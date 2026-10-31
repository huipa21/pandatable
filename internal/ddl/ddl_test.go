package ddl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"pandatable/internal/apperr"
)

func assertError(t *testing.T, err error, status int, code string) {
	t.Helper()
	var app *apperr.Error
	if !errors.As(err, &app) || app.Status != status || app.Code != code {
		t.Fatalf("error = %v, want %d %s", err, status, code)
	}
}

func TestIdentifier(t *testing.T) {
	for _, kind := range []ObjectKind{Book, Table, Column, Junction, Sequence} {
		for _, input := range []byte{0, 255} {
			name, err := newIdentifier(kind, bytes.NewReader(bytes.Repeat([]byte{input}, 7)))
			if err != nil {
				t.Fatal(err)
			}
			suffix := strings.Repeat("a", 10)
			if input == 255 {
				suffix = strings.Repeat("7", 10)
			}
			if name != string(kind)+"_"+suffix {
				t.Fatalf("identifier = %q", name)
			}
			if err := ValidateIdentifier(kind, name); err != nil {
				t.Fatal(err)
			}
		}
		name, err := NewIdentifier(kind)
		if err != nil || ValidateIdentifier(kind, name) != nil {
			t.Fatalf("random identifier = %q, %v", name, err)
		}
	}
	seen := map[string]bool{}
	for position := 0; position < 10; position++ {
		for value := 0; value < 32; value++ {
			input := make([]byte, 7)
			for bit := 0; bit < 5; bit++ {
				if value&(1<<(4-bit)) != 0 {
					index := position*5 + bit
					input[index/8] |= 1 << (7 - index%8)
				}
			}
			name, err := newIdentifier(Column, bytes.NewReader(input))
			if err != nil || len(name) != 12 || name[2+position] != alphabet[value] {
				t.Fatalf("base32 position %d symbol %d: %q, %v", position, value, name, err)
			}
		}
	}
	for bit := 0; bit < 56; bit++ {
		input := make([]byte, 7)
		input[bit/8] = 1 << (7 - bit%8)
		name, err := newIdentifier(Column, bytes.NewReader(input))
		if err != nil {
			t.Fatal(err)
		}
		if bit < 50 {
			if name == "c_aaaaaaaaaa" || seen[name] {
				t.Fatalf("entropy bit %d lost", bit)
			}
			seen[name] = true
		} else if name != "c_aaaaaaaaaa" {
			t.Fatalf("discarded bit %d affected identifier", bit)
		}
	}
	for _, name := range []string{"", "b_", "b_aaaaaaaaa", "b_aaaaaaaaaaa", "B_aaaaaaaaaa",
		"b_aaaaaaaaa0", "b_aaaaaaaaa1", "b_aaaaaaaaa8", "b_aaaaaaaaa9", "b_AAAAAAAAAA",
		"t_aaaaaaaaaa", `"b_aaaaaaaaaa"`, "b_aaaa.aaaaa", "b_aaaa_aaaaa", "b_aaaaaaa\x00aa",
		"b_aaaaaaaaaa; DROP SCHEMA meta CASCADE", "b_中文中文中文中文中文"} {
		assertError(t, ValidateIdentifier(Book, name), 400, "ERR_VALIDATION")
	}
	_, err := newIdentifier(Book, bytes.NewReader(make([]byte, 6)))
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("entropy error = %v", err)
	}
	_, err = newIdentifier(ObjectKind("b; DROP"), nil)
	assertError(t, err, 400, "ERR_VALIDATION")
	assertError(t, ValidateIdentifier(ObjectKind("x"), "x_aaaaaaaaaa"), 400, "ERR_VALIDATION")
}

func TestDataType(t *testing.T) {
	cases := []struct {
		input DataType
		want  string
	}{
		{DataType{Kind: Text}, "pg_catalog.text"},
		{DataType{Kind: Varchar, Length: 1}, "pg_catalog.varchar(1)"},
		{DataType{Kind: Varchar, Length: 10485760}, "pg_catalog.varchar(10485760)"},
		{DataType{Kind: Numeric, Precision: 1}, "pg_catalog.numeric(1,0)"},
		{DataType{Kind: Numeric, Precision: 1000, Scale: 1000}, "pg_catalog.numeric(1000,1000)"},
		{DataType{Kind: SmallInt}, "pg_catalog.int2"}, {DataType{Kind: Integer}, "pg_catalog.int4"},
		{DataType{Kind: BigInt}, "pg_catalog.int8"}, {DataType{Kind: Real}, "pg_catalog.float4"},
		{DataType{Kind: Double}, "pg_catalog.float8"}, {DataType{Kind: Boolean}, "pg_catalog.bool"},
		{DataType{Kind: Date}, "pg_catalog.date"}, {DataType{Kind: Time}, "pg_catalog.time"},
		{DataType{Kind: TimestampTZ}, "pg_catalog.timestamptz"}, {DataType{Kind: Interval}, "pg_catalog.interval"},
		{DataType{Kind: UUID}, "pg_catalog.uuid"}, {DataType{Kind: JSONB}, "pg_catalog.jsonb"},
		{DataType{Kind: Inet}, "pg_catalog.inet"}, {DataType{Kind: CIDR}, "pg_catalog.cidr"},
		{DataType{Kind: TextArray}, "pg_catalog.text[]"}, {DataType{Kind: BigIntArray}, "pg_catalog.int8[]"},
		{DataType{Kind: UUIDArray}, "pg_catalog.uuid[]"},
	}
	for _, tc := range cases {
		got, err := tc.input.SQL()
		if err != nil || got != tc.want {
			t.Fatalf("%+v SQL = %q, %v; want %q", tc.input, got, err, tc.want)
		}
	}
	for _, input := range []DataType{
		{}, {Kind: "text); DROP SCHEMA meta CASCADE;--"}, {Kind: "TEXT"}, {Kind: "public.text"},
		{Kind: Varchar}, {Kind: Varchar, Length: -1}, {Kind: Varchar, Length: 10485761},
		{Kind: Varchar, Length: 1, Scale: 1}, {Kind: Text, Length: 1},
		{Kind: Text, Precision: 1}, {Kind: Text, Scale: -1},
		{Kind: Numeric}, {Kind: Numeric, Precision: 1001}, {Kind: Numeric, Precision: -1},
		{Kind: Numeric, Precision: 2, Scale: -1}, {Kind: Numeric, Precision: 2, Scale: 3},
		{Kind: Numeric, Precision: 2, Length: 1},
	} {
		_, err := input.SQL()
		assertError(t, err, 400, "ERR_VALIDATION")
	}
}

func TestDefault(t *testing.T) {
	for _, tc := range []struct {
		value DefaultValue
		kind  TypeKind
		want  string
	}{
		{DefaultValue{}, Text, ""},
		{DefaultValue{Kind: Literal}, Text, "E''::pg_catalog.text"},
		{DefaultValue{Kind: Literal, Literal: "NULL"}, Text, "E'NULL'::pg_catalog.text"},
		{DefaultValue{Kind: Literal, Literal: `a'\'; DROP TABLE meta.users;--`}, Text, `E'a''\\''; DROP TABLE meta.users;--'::pg_catalog.text`},
		{DefaultValue{Kind: Now}, TimestampTZ, "pg_catalog.now()"},
		{DefaultValue{Kind: CurrentDate}, Date, "CURRENT_DATE"},
		{DefaultValue{Kind: UUIDv7}, UUID, "pg_catalog.uuidv7()"},
	} {
		got, err := tc.value.sql(DataType{Kind: tc.kind})
		if err != nil || got != tc.want {
			t.Fatalf("%+v default = %q, %v; want %q", tc.value, got, err, tc.want)
		}
	}
	for _, value := range []DefaultValue{
		{Kind: "now(); DROP SCHEMA meta CASCADE"}, {Kind: Now}, {Kind: CurrentDate}, {Kind: UUIDv7},
		{Literal: "unexpected"}, {Kind: Now, Literal: "ignored"},
		{Kind: Literal, Literal: "\x00"}, {Kind: Literal, Literal: "\xff"},
	} {
		_, err := value.sql(DataType{Kind: Text})
		assertError(t, err, 400, "ERR_VALIDATION")
	}
	_, err := (DefaultValue{}).sql(DataType{})
	assertError(t, err, 400, "ERR_VALIDATION")
	_, err = (DefaultValue{Kind: Literal, Literal: "abc"}).sql(DataType{Kind: Varchar, Length: 2})
	assertError(t, err, 400, "ERR_VALIDATION")
	got, err := (DefaultValue{Kind: Literal, Literal: "中文"}).sql(DataType{Kind: Varchar, Length: 2})
	if err != nil || got != "E'中文'::pg_catalog.varchar(2)" {
		t.Fatalf("varchar character bound = %q, %v", got, err)
	}
}

type recordingTx struct {
	pgx.Tx
	queries []string
	args    [][]any
	err     error
	failAt  int
}

func (tx *recordingTx) Exec(_ context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	tx.queries = append(tx.queries, query)
	tx.args = append(tx.args, args)
	if len(tx.queries) == tx.failAt {
		return pgconn.CommandTag{}, tx.err
	}
	return pgconn.NewCommandTag("ALTER TABLE"), nil
}

func TestOperationsGrammar(t *testing.T) {
	table := TableName{Schema: "b_aaaaaaaaaa", Name: "t_bbbbbbbbbb"}
	column := "c_cccccccccc"
	target := `"b_aaaaaaaaaa"."t_bbbbbbbbbb"`
	constraint := `"t_bbbbbbbbbb_c_cccccccccc_key"`
	for _, tc := range []struct {
		name string
		call func(pgx.Tx) error
		want string
	}{
		{"schema", func(tx pgx.Tx) error { return CreateSchema(context.Background(), tx, table.Schema) }, `CREATE SCHEMA "b_aaaaaaaaaa"`},
		{"drop schema", func(tx pgx.Tx) error { return DropSchema(context.Background(), tx, table.Schema) }, `DROP SCHEMA "b_aaaaaaaaaa"`},
		{"table", func(tx pgx.Tx) error { return CreateTable(context.Background(), tx, table) }, `CREATE TABLE ` + target + ` (
		"id" pg_catalog.int8 GENERATED BY DEFAULT AS IDENTITY CONSTRAINT "t_bbbbbbbbbb_pkey" PRIMARY KEY,
		"created_at" pg_catalog.timestamptz NOT NULL DEFAULT pg_catalog.now(),
		"created_by" pg_catalog.uuid,
		"updated_at" pg_catalog.timestamptz NOT NULL DEFAULT pg_catalog.now(),
		"updated_by" pg_catalog.uuid
	)`},
		{"drop table", func(tx pgx.Tx) error { return DropTable(context.Background(), tx, table) }, `DROP TABLE ` + target},
		{"add", func(tx pgx.Tx) error {
			return AddColumn(context.Background(), tx, table, ColumnSpec{Name: column, Type: DataType{Kind: Text}, NotNull: true, Unique: true, Default: DefaultValue{Kind: Literal, Literal: "a'b"}})
		}, `ALTER TABLE ` + target + ` ADD COLUMN "c_cccccccccc" pg_catalog.text DEFAULT E'a''b'::pg_catalog.text NOT NULL CONSTRAINT ` + constraint + ` UNIQUE`},
		{"drop", func(tx pgx.Tx) error { return DropColumn(context.Background(), tx, table, column) }, `ALTER TABLE ` + target + ` DROP COLUMN "c_cccccccccc"`},
		{"required", func(tx pgx.Tx) error { return SetNotNull(context.Background(), tx, table, column, true) }, `ALTER TABLE ` + target + ` ALTER COLUMN "c_cccccccccc" SET NOT NULL`},
		{"nullable", func(tx pgx.Tx) error { return SetNotNull(context.Background(), tx, table, column, false) }, `ALTER TABLE ` + target + ` ALTER COLUMN "c_cccccccccc" DROP NOT NULL`},
		{"unique", func(tx pgx.Tx) error { return AddUnique(context.Background(), tx, table, column, true) }, `ALTER TABLE ` + target + ` ADD CONSTRAINT ` + constraint + ` UNIQUE ("c_cccccccccc")`},
		{"not unique", func(tx pgx.Tx) error { return AddUnique(context.Background(), tx, table, column, false) }, `ALTER TABLE ` + target + ` DROP CONSTRAINT ` + constraint},
		{"default", func(tx pgx.Tx) error {
			return SetDefault(context.Background(), tx, table, column, DataType{Kind: Boolean}, DefaultValue{Kind: Literal, Literal: "false"})
		}, `ALTER TABLE ` + target + ` ALTER COLUMN "c_cccccccccc" SET DEFAULT E'false'::pg_catalog.bool`},
		{"no default", func(tx pgx.Tx) error {
			return SetDefault(context.Background(), tx, table, column, DataType{Kind: Boolean}, DefaultValue{})
		}, `ALTER TABLE ` + target + ` ALTER COLUMN "c_cccccccccc" DROP DEFAULT`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := &recordingTx{}
			if err := tc.call(tx); err != nil {
				t.Fatal(err)
			}
			if len(tx.queries) != 2 || tx.queries[0] != "SET LOCAL lock_timeout = '3s'" || tx.queries[1] != tc.want {
				t.Fatalf("queries = %#v", tx.queries)
			}
			for _, args := range tx.args {
				if len(args) != 0 {
					t.Fatalf("DDL unexpectedly uses bind arguments: %v", args)
				}
			}
		})
	}
}

func TestOperationsRejectBeforeExecution(t *testing.T) {
	ctx := context.Background()
	table := TableName{Schema: "b_aaaaaaaaaa", Name: "t_bbbbbbbbbb"}
	for _, column := range []string{"id", "created_at", "created_by", "updated_at", "updated_by", `c_aaaaaaaaaa"; DROP TABLE meta.users;--`} {
		for _, call := range []func(pgx.Tx) error{
			func(tx pgx.Tx) error {
				return AddColumn(ctx, tx, table, ColumnSpec{Name: column, Type: DataType{Kind: Text}})
			},
			func(tx pgx.Tx) error { return DropColumn(ctx, tx, table, column) },
			func(tx pgx.Tx) error { return RestoreAutoNumberColumn(ctx, tx, table, column) },
			func(tx pgx.Tx) error { return SetNotNull(ctx, tx, table, column, false) },
			func(tx pgx.Tx) error { return AddUnique(ctx, tx, table, column, true) },
			func(tx pgx.Tx) error { return SetDefault(ctx, tx, table, column, DataType{Kind: Text}, DefaultValue{}) },
		} {
			tx := &recordingTx{}
			assertError(t, call(tx), 400, "ERR_VALIDATION")
			if len(tx.queries) != 0 {
				t.Fatalf("invalid column executed SQL: %v", tx.queries)
			}
		}
	}
	for _, call := range []func(pgx.Tx) error{
		func(tx pgx.Tx) error { return CreateSchema(ctx, tx, `"b_aaaaaaaaaa"`) },
		func(tx pgx.Tx) error { return DropSchema(ctx, tx, `"b_aaaaaaaaaa"`) },
		func(tx pgx.Tx) error { return CreateTable(ctx, tx, TableName{Schema: "meta", Name: table.Name}) },
		func(tx pgx.Tx) error { return DropTable(ctx, tx, TableName{Schema: "meta", Name: table.Name}) },
		func(tx pgx.Tx) error {
			return CreateTable(ctx, tx, TableName{Schema: table.Schema, Name: "t_aaaaaaaaa0"})
		},
		func(tx pgx.Tx) error {
			return AddColumn(ctx, tx, table, ColumnSpec{Name: "c_aaaaaaaaaa", Type: DataType{Kind: "text; DROP TABLE meta.users"}})
		},
		func(tx pgx.Tx) error {
			return SetDefault(ctx, tx, table, "c_aaaaaaaaaa", DataType{Kind: Text}, DefaultValue{Kind: "1; SELECT 1"})
		},
	} {
		tx := &recordingTx{}
		assertError(t, call(tx), 400, "ERR_VALIDATION")
		if len(tx.queries) != 0 {
			t.Fatalf("invalid input executed SQL: %v", tx.queries)
		}
	}
	assertError(t, CreateSchema(ctx, nil, table.Schema), 400, "ERR_VALIDATION")
}

func TestErrors(t *testing.T) {
	for _, tc := range []struct {
		state     string
		creating  bool
		status    int
		code      string
		collision bool
	}{
		{"42P06", true, 409, "ERR_DDL_CONFLICT", true},
		{"42P07", true, 409, "ERR_DDL_CONFLICT", true},
		{"42701", true, 409, "ERR_DDL_CONFLICT", true},
		{"23502", false, 409, "ERR_DDL_CONFLICT", false},
		{"23505", false, 409, "ERR_DDL_CONFLICT", false},
		{"42710", false, 409, "ERR_DDL_CONFLICT", false},
		{"42P07", false, 409, "ERR_DDL_CONFLICT", false},
		{"55P03", false, 409, "ERR_DDL_LOCK_TIMEOUT", false},
		{"22P02", false, 400, "ERR_VALIDATION", false},
		{"22003", false, 400, "ERR_VALIDATION", false},
	} {
		raw := &pgconn.PgError{Code: tc.state, Message: "secret default", Detail: "private row"}
		err := databaseError(raw, tc.creating)
		assertError(t, err, tc.status, tc.code)
		if errors.Is(err, ErrIdentifierCollision) != tc.collision {
			t.Fatalf("collision = %v for %s", err, tc.state)
		}
		var app *apperr.Error
		if !errors.As(err, &app) {
			t.Fatal(err)
		}
		encoded, marshalErr := json.Marshal(app)
		if marshalErr != nil || strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "private") ||
			strings.Contains(string(encoded), "Status") || strings.Contains(string(encoded), tc.state) {
			t.Fatalf("unsafe error JSON = %s, %v", encoded, marshalErr)
		}
	}
	raw := errors.New("connection failure")
	if databaseError(raw, false) != raw {
		t.Fatal("lost underlying error")
	}
	for _, state := range []string{"", "x", "42P01", "57014", "XX000"} {
		err := &pgconn.PgError{Code: state}
		if databaseError(err, false) != err {
			t.Fatalf("unexpected classification of %s", state)
		}
	}
	for _, failAt := range []int{1, 2} {
		tx := &recordingTx{failAt: failAt, err: raw}
		err := CreateSchema(context.Background(), tx, "b_aaaaaaaaaa")
		if !errors.Is(err, raw) || len(tx.queries) != failAt {
			t.Fatalf("failure %d: queries=%v err=%v", failAt, tx.queries, err)
		}
	}
}
