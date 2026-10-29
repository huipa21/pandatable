package codec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"pandatable/internal/ddl"
	"pandatable/internal/testdb"
)

var database testdb.Suite

func TestMain(m *testing.M) { os.Exit(database.Run(m)) }

// formatType is PostgreSQL's format_type() spelling of a codec descriptor.
func formatType(column Column) string {
	switch column.Type.Kind {
	case ddl.Varchar:
		return fmt.Sprintf("character varying(%d)", column.Type.Length)
	case ddl.Numeric:
		return fmt.Sprintf("numeric(%d,%d)", column.Type.Precision, column.Type.Scale)
	case ddl.TimestampTZ:
		return "timestamp with time zone"
	case ddl.Time:
		return "time without time zone"
	}
	return string(column.Type.Kind)
}

type pgFixture struct {
	t     *testing.T
	ctx   context.Context
	tx    pgx.Tx
	table ddl.TableName
	name  string
}

func newPGFixture(t *testing.T) *pgFixture {
	t.Helper()
	pool := database.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error(err)
		}
	})
	fixture := &pgFixture{t: t, ctx: ctx, tx: tx}
	fixture.table = ddl.TableName{Schema: fixture.identifier(ddl.Book), Name: fixture.identifier(ddl.Table)}
	fixture.must(ddl.CreateSchema(ctx, tx, fixture.table.Schema))
	fixture.must(ddl.CreateTable(ctx, tx, fixture.table))
	fixture.name = pgx.Identifier{fixture.table.Schema, fixture.table.Name}.Sanitize()
	return fixture
}

func (fixture *pgFixture) must(err error) {
	fixture.t.Helper()
	if err != nil {
		fixture.t.Fatal(err)
	}
}

func (fixture *pgFixture) identifier(kind ddl.ObjectKind) string {
	fixture.t.Helper()
	name, err := ddl.NewIdentifier(kind)
	fixture.must(err)
	return name
}

// addColumn adds a column exactly as the codec describes it and checks the
// resulting catalog type.
func (fixture *pgFixture) addColumn(field Field, defaultValue ddl.DefaultValue) string {
	fixture.t.Helper()
	column := field.Column()
	if column.System {
		fixture.t.Fatalf("%s is a system column", field.Kind())
	}
	name := fixture.identifier(ddl.Column)
	if column.AutoNumber {
		fixture.must(ddl.AddAutoNumberColumn(fixture.ctx, fixture.tx, fixture.table, name))
		fixture.checkType(name, column)
		return name
	}
	if defaultValue.Kind == ddl.NoDefault {
		defaultValue = column.Default
	}
	spec := ddl.ColumnSpec{Name: name, Type: column.Type, NotNull: column.NotNull, Default: defaultValue}
	if column.HasRangeCheck {
		spec.RangeCheck = &ddl.IntRangeCheck{Min: column.CheckMin, Max: column.CheckMax}
	}
	if column.UserReference {
		spec.Reference = ddl.UserReference
	}
	fixture.must(ddl.AddColumn(fixture.ctx, fixture.tx, fixture.table, spec))
	fixture.checkType(name, column)
	return name
}

func (fixture *pgFixture) checkType(name string, column Column) {
	fixture.t.Helper()
	var typeName string
	var notNull bool
	fixture.must(fixture.tx.QueryRow(fixture.ctx, `
		SELECT pg_catalog.format_type(a.atttypid, a.atttypmod), a.attnotnull
		FROM pg_catalog.pg_attribute a
		WHERE a.attrelid = $1::pg_catalog.regclass AND a.attname = $2 AND NOT a.attisdropped`,
		fixture.name, name).Scan(&typeName, &notNull))
	if typeName != formatType(column) || notNull != column.NotNull {
		fixture.t.Fatalf("column %s: %s notNull=%v, want %s notNull=%v", name, typeName, notNull, formatType(column), column.NotNull)
	}
}

// roundTrip inserts a typed parameter and reads it back through rows.Values().
func (fixture *pgFixture) roundTrip(column string, value any) any {
	fixture.t.Helper()
	quoted := pgx.Identifier{column}.Sanitize()
	var id int64
	fixture.must(fixture.tx.QueryRow(fixture.ctx,
		"INSERT INTO "+fixture.name+" ("+quoted+") VALUES ($1) RETURNING id", value).Scan(&id))
	return fixture.read(column, id)
}

func (fixture *pgFixture) read(column string, id int64) any {
	fixture.t.Helper()
	rows, err := fixture.tx.Query(fixture.ctx, "SELECT "+pgx.Identifier{column}.Sanitize()+" FROM "+fixture.name+" WHERE id = $1", id)
	fixture.must(err)
	defer rows.Close()
	if !rows.Next() {
		fixture.t.Fatalf("row %d missing", id)
	}
	values, err := rows.Values()
	fixture.must(err)
	fixture.must(rows.Err())
	return values[0]
}

func (fixture *pgFixture) insertDefault() int64 {
	fixture.t.Helper()
	var id int64
	fixture.must(fixture.tx.QueryRow(fixture.ctx, "INSERT INTO "+fixture.name+" DEFAULT VALUES RETURNING id").Scan(&id))
	return id
}

func (fixture *pgFixture) expectSQLState(state string, query string, args ...any) {
	fixture.t.Helper()
	_, err := fixture.tx.Exec(fixture.ctx, "SAVEPOINT expected_failure")
	fixture.must(err)
	_, err = fixture.tx.Exec(fixture.ctx, query, args...)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != state {
		fixture.t.Fatalf("%s: got %v, want SQLSTATE %s", query, err, state)
	}
	_, err = fixture.tx.Exec(fixture.ctx, "ROLLBACK TO SAVEPOINT expected_failure")
	fixture.must(err)
}

// TestCodecVectorsRoundTripPostgreSQL stores every successful shared decode
// and paste vector in a real native column created from the codec descriptor
// and verifies the scanned value encodes and formats identically.
func TestCodecVectorsRoundTripPostgreSQL(t *testing.T) {
	fixture := newPGFixture(t)
	vectors := loadVectors(t)
	columns := map[string]string{}
	fields := map[string]Field{}
	checked := map[Kind]int{}
	run := func(label string, vector caseVector, value any, err error) {
		t.Helper()
		if vector.Reason != "" || err != nil || IsSystem(vector.Kind) {
			return
		}
		key := string(vector.Kind) + string(vector.Options)
		field, ok := fields[key]
		if !ok {
			field = mustBind(t, vector.Kind, vector.Options)
			fields[key] = field
			columns[key] = fixture.addColumn(field, ddl.DefaultValue{})
		}
		want, err := field.Encode(value)
		fixture.must(err)
		if value == nil && vector.Kind != Checkbox && vector.Kind != User {
			// NULL is the same for every nullable kind; still store it once.
			if checked[vector.Kind] > 0 {
				return
			}
		}
		scanned := fixture.roundTrip(columns[key], value)
		got, err := field.Encode(scanned)
		if err != nil || string(got) != string(want) {
			t.Errorf("%s: scanned %#v encodes %s (%v), want %s", label, scanned, got, err, want)
		}
		env := envFor(t, vector.Zone)
		wantText, _ := field.FormatText(value, env)
		if gotText, err := field.FormatText(scanned, env); err != nil || gotText != wantText {
			t.Errorf("%s: scanned text %q (%v), want %q", label, gotText, err, wantText)
		}
		checked[vector.Kind]++
	}
	for index, vector := range vectors.Decode {
		field := mustBind(t, vector.Kind, vector.Options)
		value, err := field.Decode(json.RawMessage(vector.Raw))
		run(fmt.Sprintf("decode[%d] %s %s", index, vector.Kind, vector.Raw), vector, value, err)
	}
	for index, vector := range vectors.Paste {
		field := mustBind(t, vector.Kind, vector.Options)
		value, err := field.ParsePaste(vector.Text, envFor(t, vector.Zone))
		run(fmt.Sprintf("paste[%d] %s %q", index, vector.Kind, vector.Text), vector, value, err)
	}
	for _, kind := range Kinds() {
		if !IsSystem(kind) && kind != AutoNumber && checked[kind] < 2 {
			t.Errorf("%s: only %d PostgreSQL round trips", kind, checked[kind])
		}
	}
}

// TestCodecDefaultsPostgreSQL applies typed DefaultValue literals through
// internal/ddl and checks PostgreSQL stores exactly the decoded value.
func TestCodecDefaultsPostgreSQL(t *testing.T) {
	fixture := newPGFixture(t)
	cases := []struct {
		kind    Kind
		options string
		raw     string
	}{
		{Text, "", `"it's \\ \"quoted\" 世界"`},
		{Text, `{"maxLength":3}`, `"😀😀😀"`},
		{LongText, "", `"line1\nline2"`},
		{Integer, `{"size":"small"}`, `-32768`},
		{Integer, "", `2147483647`},
		{Integer, `{"size":"big"}`, `"-9223372036854775808"`},
		{Decimal, `{"precision":38,"scale":10}`, `"-1234567890123456789012345678.0123456789"`},
		{Decimal, "", `"0.1"`},
		{Float, `{"size":"real"}`, `0.1`},
		{Float, "", `1.7976931348623157e308`},
		{Float, "", `5e-324`},
		{Checkbox, "", `true`},
		{Date, "", `"0001-01-01"`},
		{DateTime, "", `"9999-12-31T23:59:59.999999Z"`},
		{DateTime, "", `"2026-03-08T02:30:00.000001-05:00"`},
		{SingleSelect, `{"choices":[{"id":"a","label":"It's"}]}`, `"It's"`},
		{MultiSelect, `{"choices":[{"id":"a","label":"say \"hi\""},{"id":"b","label":"back\\slash"},{"id":"c","label":"{NULL}"},{"id":"d","label":"NULL"}]}`, `["NULL","{NULL}","back\\slash","say \"hi\""]`},
		{Email, "", `"o'brien@example.com"`},
		{URL, "", `"https://example.com/?q='x'&r=%5C"`},
		{Currency, `{"currency":"USD","scale":2}`, `"99.95"`},
		{Percent, "", `"0.25"`},
		{Rating, `{"max":10}`, `10`},
		{Time, "", `"12:34:56.1234"`},
		{Duration, "", `"PT1H2M3.5S"`},
		{Phone, "", `"+1 (212) 555-0100 x5"`},
		{UUID, "", `"00000000-0000-0000-0000-000000000001"`},
		{JSON, "", `{"z":1,"a":true}`},
		{Inet, "", `"192.0.2.1"`},
		{CIDR, "", `"192.0.2.0/24"`},
		{TextArray, "", `["one","quote \" and \\ slash"]`},
		{IntArray, "", `["9223372036854775807","-2"]`},
	}
	for _, test := range cases {
		field := mustBind(t, test.kind, json.RawMessage(test.options))
		value, err := field.Decode(json.RawMessage(test.raw))
		fixture.must(err)
		defaultValue, err := field.DefaultValue(json.RawMessage(test.raw))
		if err != nil {
			t.Fatalf("%s default: %v", test.kind, err)
		}
		column := fixture.addColumn(field, defaultValue)
		scanned := fixture.read(column, fixture.insertDefault())
		got, err := field.Encode(scanned)
		fixture.must(err)
		want, _ := field.Encode(value)
		if string(got) != string(want) {
			t.Errorf("%s default %s: got %s, want %s", test.kind, test.raw, got, want)
		}
	}
	autoUUID := mustBind(t, UUID, json.RawMessage(`{"autoGenerate":true}`))
	autoColumn := fixture.addColumn(autoUUID, ddl.DefaultValue{})
	generated := fixture.read(autoColumn, fixture.insertDefault())
	encoded, err := autoUUID.Encode(generated)
	fixture.must(err)
	if _, err := autoUUID.Decode(encoded); err != nil {
		t.Fatalf("generated UUID wire value %s: %v", encoded, err)
	}
}

func TestCodecConstraintsPostgreSQL(t *testing.T) {
	fixture := newPGFixture(t)

	// Checkbox is NOT NULL DEFAULT false regardless of user defaults.
	checkbox := mustBind(t, Checkbox, nil)
	checkboxColumn := fixture.addColumn(checkbox, ddl.DefaultValue{})
	if encoded, _ := checkbox.Encode(fixture.read(checkboxColumn, fixture.insertDefault())); string(encoded) != "false" {
		t.Fatalf("checkbox default %s", encoded)
	}
	fixture.expectSQLState("23502", "INSERT INTO "+fixture.name+" ("+pgx.Identifier{checkboxColumn}.Sanitize()+") VALUES (NULL)")

	rating := mustBind(t, Rating, json.RawMessage(`{"max":5}`))
	ratingColumn := fixture.addColumn(rating, ddl.DefaultValue{})
	fixture.expectSQLState("23514", "INSERT INTO "+fixture.name+" ("+pgx.Identifier{ratingColumn}.Sanitize()+") VALUES (6)")

	// The codec maxLength equals the varchar limit in code points.
	text := mustBind(t, Text, json.RawMessage(`{"maxLength":3}`))
	textColumn := fixture.addColumn(text, ddl.DefaultValue{})
	quoted := pgx.Identifier{textColumn}.Sanitize()
	if _, err := text.Decode(json.RawMessage(`"😀😀😀😀"`)); reasonOf(err) != ReasonTooLong {
		t.Fatalf("codec accepted 4 code points: %v", err)
	}
	fixture.expectSQLState("22001", "INSERT INTO "+fixture.name+" ("+quoted+") VALUES ($1)", "😀😀😀😀")

	// Decimal bounds match numeric(p,s) so PostgreSQL never rounds silently.
	decimal := mustBind(t, Decimal, json.RawMessage(`{"precision":5,"scale":2}`))
	decimalColumn := fixture.addColumn(decimal, ddl.DefaultValue{})
	max, err := decimal.Decode(json.RawMessage(`"999.99"`))
	fixture.must(err)
	if encoded, _ := decimal.Encode(fixture.roundTrip(decimalColumn, max)); string(encoded) != `"999.99"` {
		t.Fatalf("decimal max %s", encoded)
	}
	if _, err := decimal.Decode(json.RawMessage(`"1000"`)); reasonOf(err) != ReasonOutOfRange {
		t.Fatalf("decimal overflow accepted: %v", err)
	}
	fixture.expectSQLState("22003", "INSERT INTO "+fixture.name+" ("+pgx.Identifier{decimalColumn}.Sanitize()+") VALUES ($1::text::pg_catalog.numeric)", "1000")

	// Integer sizes match int2/int4/int8 bounds.
	for _, test := range []struct{ options, max, over string }{
		{`{"size":"small"}`, `32767`, "32768"},
		{`{"size":"int"}`, `2147483647`, "2147483648"},
		{`{"size":"big"}`, `"9223372036854775807"`, "9223372036854775808"},
	} {
		field := mustBind(t, Integer, json.RawMessage(test.options))
		column := fixture.addColumn(field, ddl.DefaultValue{})
		value, err := field.Decode(json.RawMessage(test.max))
		fixture.must(err)
		got, _ := field.Encode(fixture.roundTrip(column, value))
		want, _ := field.Encode(value)
		if string(got) != string(want) {
			t.Fatalf("%s max %s != %s", test.options, got, want)
		}
		if _, err := field.ParsePaste(test.over, Env{}); reasonOf(err) != ReasonOutOfRange {
			t.Fatalf("%s overflow accepted", test.options)
		}
		fixture.expectSQLState("22003", "INSERT INTO "+fixture.name+" ("+pgx.Identifier{column}.Sanitize()+") VALUES ($1::text::"+formatType(field.Column())+")", test.over)
	}
}

// TestSystemColumnsPostgreSQL checks the five system kinds against the
// columns ddl.CreateTable installs and encodes scanned values.
func TestSystemColumnsPostgreSQL(t *testing.T) {
	fixture := newPGFixture(t)
	for _, kind := range []Kind{RowID, CreatedAt, CreatedBy, UpdatedAt, UpdatedBy} {
		fixture.checkType(string(kind), mustBind(t, kind, nil).Column())
	}
	actor := "0190f5d2-7b3c-7d4e-8f00-0123456789ab"
	var id int64
	fixture.must(fixture.tx.QueryRow(fixture.ctx, "INSERT INTO "+fixture.name+
		" (created_at, created_by) VALUES ('2026-10-26 08:30:00.123456+00', $1::pg_catalog.uuid) RETURNING id", actor).Scan(&id))
	expected := map[Kind]string{
		RowID:     fmt.Sprintf("%q", fmt.Sprint(id)),
		CreatedAt: `"2026-10-26T08:30:00.123456Z"`,
		CreatedBy: `"` + actor + `"`,
		UpdatedBy: "null",
	}
	for _, kind := range []Kind{RowID, CreatedAt, CreatedBy, UpdatedAt, UpdatedBy} {
		field := mustBind(t, kind, nil)
		encoded, err := field.Encode(fixture.read(string(kind), id))
		fixture.must(err)
		if want, ok := expected[kind]; ok && string(encoded) != want {
			t.Errorf("%s: %s, want %s", kind, encoded, want)
		}
		if kind == UpdatedAt {
			var instant string
			if json.Unmarshal(encoded, &instant) != nil || len(instant) < 20 || instant[len(instant)-1] != 'Z' {
				t.Errorf("updated_at %s", encoded)
			}
		}
	}
}
