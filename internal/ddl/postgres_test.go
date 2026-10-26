package ddl

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"pandatable/internal/db"
	"pandatable/internal/testdb"
)

var database testdb.Suite

func TestMain(m *testing.M) { os.Exit(database.Run(m)) }

func identifier(t *testing.T, kind ObjectKind) string {
	t.Helper()
	name, err := NewIdentifier(kind)
	if err != nil {
		t.Fatal(err)
	}
	return name
}

func transaction(t *testing.T, ctx context.Context, pool *db.Pool) pgx.Tx {
	t.Helper()
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
	return tx
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func exec(t *testing.T, ctx context.Context, tx pgx.Tx, sql string, args ...any) {
	t.Helper()
	_, err := tx.Exec(ctx, sql, args...)
	must(t, err)
}

func check(t *testing.T, ctx context.Context, tx pgx.Tx, sql string, args ...any) {
	t.Helper()
	var ok bool
	must(t, tx.QueryRow(ctx, sql, args...).Scan(&ok))
	if !ok {
		t.Fatalf("catalog/data assertion failed: %s", sql)
	}
}

func physicalTable(t *testing.T, ctx context.Context, tx pgx.Tx) (TableName, string) {
	t.Helper()
	table := TableName{Schema: identifier(t, Book), Name: identifier(t, Table)}
	must(t, CreateSchema(ctx, tx, table.Schema))
	must(t, CreateTable(ctx, tx, table))
	sql, err := table.sql()
	must(t, err)
	return table, sql
}

func expectedFailure(t *testing.T, ctx context.Context, tx pgx.Tx, code string, status int, call func() error) {
	t.Helper()
	exec(t, ctx, tx, "SAVEPOINT expected_failure")
	err := call()
	assertError(t, err, status, code)
	exec(t, ctx, tx, "ROLLBACK TO SAVEPOINT expected_failure")
	exec(t, ctx, tx, "RELEASE SAVEPOINT expected_failure")
}

func TestDDLSystemColumnsPostgreSQL(t *testing.T) {
	pool := database.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx := transaction(t, ctx, pool)
	table, sql := physicalTable(t, ctx, tx)
	rows, err := tx.Query(ctx, `SELECT column_name, data_type, is_nullable, is_identity,
		COALESCE(identity_generation,''), COALESCE(column_default,'')
		FROM information_schema.columns WHERE table_schema=$1 AND table_name=$2 ORDER BY ordinal_position`,
		table.Schema, table.Name)
	must(t, err)
	got, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) ([6]string, error) {
		var values [6]string
		err := row.Scan(&values[0], &values[1], &values[2], &values[3], &values[4], &values[5])
		return values, err
	})
	must(t, err)
	want := [][6]string{
		{"id", "bigint", "NO", "YES", "BY DEFAULT", ""},
		{"created_at", "timestamp with time zone", "NO", "NO", "", "now()"},
		{"created_by", "uuid", "YES", "NO", "", ""},
		{"updated_at", "timestamp with time zone", "NO", "NO", "", "now()"},
		{"updated_by", "uuid", "YES", "NO", "", ""},
	}
	if len(got) != len(want) {
		t.Fatalf("system columns = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("column %d = %v, want %v", i, got[i], want[i])
		}
	}
	check(t, ctx, tx, `SELECT count(*) FILTER (WHERE contype='p')=1
		AND bool_and(conname=$2) FILTER (WHERE contype='p')
		AND count(*) FILTER (WHERE contype='n')=3
		AND count(*) FILTER (WHERE contype NOT IN ('p','n'))=0
		FROM pg_constraint WHERE conrelid=$1::regclass`, sql, table.Name+"_pkey")
	check(t, ctx, tx, `SELECT count(*)=0 FROM pg_trigger WHERE tgrelid=$1::regclass`, sql)
	exec(t, ctx, tx, "INSERT INTO "+sql+" DEFAULT VALUES")
	exec(t, ctx, tx, "INSERT INTO "+sql+` (id,created_by,updated_by) VALUES (100,'00000000-0000-0000-0000-000000000001','00000000-0000-0000-0000-000000000002')`)
	exec(t, ctx, tx, "INSERT INTO "+sql+" DEFAULT VALUES")
	check(t, ctx, tx, "SELECT array_agg(id ORDER BY id)=ARRAY[1,2,100]::bigint[] AND bool_and(created_at=now() AND updated_at=now()) FROM "+sql)
	exec(t, ctx, tx, "UPDATE "+sql+" SET updated_at='2001-01-01Z' WHERE id=1")
	exec(t, ctx, tx, "UPDATE "+sql+" SET created_by='00000000-0000-0000-0000-000000000003' WHERE id=1")
	check(t, ctx, tx, "SELECT updated_at='2001-01-01Z' AND updated_by IS NULL FROM "+sql+" WHERE id=1")
	check(t, ctx, tx, "SELECT current_setting('lock_timeout')='3s' AND current_setting('TimeZone')='UTC'")
}

func TestDDLTypesDefaultsPostgreSQL(t *testing.T) {
	pool := database.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx := transaction(t, ctx, pool)
	table, sql := physicalTable(t, ctx, tx)
	for _, tc := range []struct {
		dataType DataType
		value    string
		format   string
	}{
		{DataType{Kind: Text}, "plain", "text"},
		{DataType{Kind: Varchar, Length: 20}, "bounded", "character varying(20)"},
		{DataType{Kind: SmallInt}, "-32768", "smallint"},
		{DataType{Kind: Integer}, "2147483647", "integer"},
		{DataType{Kind: BigInt}, "9223372036854775807", "bigint"},
		{DataType{Kind: Numeric, Precision: 12, Scale: 2}, "123456.25", "numeric(12,2)"},
		{DataType{Kind: Real}, "1.25", "real"},
		{DataType{Kind: Double}, "2.5", "double precision"},
		{DataType{Kind: Boolean}, "false", "boolean"},
		{DataType{Kind: Date}, "2026-10-26", "date"},
		{DataType{Kind: Time}, "12:34:56", "time without time zone"},
		{DataType{Kind: TimestampTZ}, "2026-10-26T12:34:56+08:00", "timestamp with time zone"},
		{DataType{Kind: Interval}, "2 hours", "interval"},
		{DataType{Kind: UUID}, "00000000-0000-0000-0000-000000000001", "uuid"},
		{DataType{Kind: JSONB}, `{"a":"b"}`, "jsonb"},
		{DataType{Kind: Inet}, "192.0.2.1", "inet"},
		{DataType{Kind: CIDR}, "192.0.2.0/24", "cidr"},
		{DataType{Kind: TextArray}, `{"a","b"}`, "text[]"},
		{DataType{Kind: BigIntArray}, "{1,2}", "bigint[]"},
		{DataType{Kind: UUIDArray}, "{00000000-0000-0000-0000-000000000001}", "uuid[]"},
	} {
		column := identifier(t, Column)
		must(t, AddColumn(ctx, tx, table, ColumnSpec{Name: column, Type: tc.dataType, Default: DefaultValue{Kind: Literal, Literal: tc.value}}))
		check(t, ctx, tx, `SELECT format_type(atttypid,atttypmod)=$3 AND atthasdef
			FROM pg_attribute WHERE attrelid=$1::regclass AND attname=$2`, sql, column, tc.format)
		exec(t, ctx, tx, "INSERT INTO "+sql+" DEFAULT VALUES")
		typeSQL, err := tc.dataType.SQL()
		must(t, err)
		quoted := pgx.Identifier{column}.Sanitize()
		check(t, ctx, tx, "SELECT "+quoted+" IS NOT DISTINCT FROM $1::"+typeSQL+" FROM "+sql+" ORDER BY id DESC LIMIT 1", tc.value)
	}
	for _, tc := range []struct {
		dataType DataType
		value    DefaultValue
		testSQL  string
	}{
		{DataType{Kind: TimestampTZ}, DefaultValue{Kind: Now}, "= now()"},
		{DataType{Kind: Date}, DefaultValue{Kind: CurrentDate}, "= CURRENT_DATE"},
		{DataType{Kind: UUID}, DefaultValue{Kind: UUIDv7}, "=7"},
	} {
		column := identifier(t, Column)
		must(t, AddColumn(ctx, tx, table, ColumnSpec{Name: column, Type: tc.dataType, Default: tc.value}))
		exec(t, ctx, tx, "INSERT INTO "+sql+" DEFAULT VALUES")
		expression := pgx.Identifier{column}.Sanitize()
		if tc.dataType.Kind == UUID {
			expression = "pg_catalog.uuid_extract_version(" + expression + ")"
		}
		check(t, ctx, tx, "SELECT "+expression+tc.testSQL+" FROM "+sql+" ORDER BY id DESC LIMIT 1")
	}
	for _, tc := range []struct {
		dataType DataType
		value    string
	}{
		{DataType{Kind: Integer}, "not an integer"},
		{DataType{Kind: SmallInt}, "32768"},
		{DataType{Kind: Varchar, Length: 2}, "too long"},
		{DataType{Kind: Numeric, Precision: 2}, "100"},
		{DataType{Kind: JSONB}, "{invalid"},
		{DataType{Kind: UUID}, "'; DROP TABLE meta.users; --"},
		{DataType{Kind: Date}, "not a date"},
	} {
		column := identifier(t, Column)
		expectedFailure(t, ctx, tx, "ERR_VALIDATION", 400, func() error {
			return AddColumn(ctx, tx, table, ColumnSpec{Name: column, Type: tc.dataType, Default: DefaultValue{Kind: Literal, Literal: tc.value}})
		})
		check(t, ctx, tx, "SELECT count(*)=0 FROM pg_attribute WHERE attrelid=$1::regclass AND attname=$2", sql, column)
	}
}

func TestDDLConstraintRemovalPostgreSQL(t *testing.T) {
	pool := database.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx := transaction(t, ctx, pool)
	table, sql := physicalTable(t, ctx, tx)
	column := identifier(t, Column)
	quoted := pgx.Identifier{column}.Sanitize()
	exec(t, ctx, tx, "INSERT INTO "+sql+" DEFAULT VALUES")
	must(t, AddColumn(ctx, tx, table, ColumnSpec{Name: column, Type: DataType{Kind: Text}}))
	check(t, ctx, tx, "SELECT "+quoted+" IS NULL FROM "+sql+" WHERE id=1")
	expectedFailure(t, ctx, tx, "ERR_DDL_CONFLICT", 409, func() error { return SetNotNull(ctx, tx, table, column, true) })
	must(t, SetDefault(ctx, tx, table, column, DataType{Kind: Text}, DefaultValue{Kind: Literal, Literal: "first"}))
	check(t, ctx, tx, "SELECT "+quoted+" IS NULL FROM "+sql+" WHERE id=1")
	exec(t, ctx, tx, "INSERT INTO "+sql+" DEFAULT VALUES")
	check(t, ctx, tx, "SELECT "+quoted+"='first' FROM "+sql+" WHERE id=2")
	must(t, SetDefault(ctx, tx, table, column, DataType{Kind: Text}, DefaultValue{}))
	exec(t, ctx, tx, "INSERT INTO "+sql+" DEFAULT VALUES")
	check(t, ctx, tx, "SELECT "+quoted+" IS NULL FROM "+sql+" WHERE id=3")
	must(t, AddUnique(ctx, tx, table, column, true))
	check(t, ctx, tx, `SELECT count(*)=1 AND bool_and(contype='u' AND NOT condeferrable)
		FROM pg_constraint WHERE conrelid=$1::regclass AND conname=$2`, sql, table.Name+"_"+column+"_key")
	check(t, ctx, tx, `SELECT NOT indnullsnotdistinct FROM pg_index
		WHERE indexrelid=(SELECT conindid FROM pg_constraint WHERE conrelid=$1::regclass AND conname=$2)`,
		sql, table.Name+"_"+column+"_key")
	must(t, AddUnique(ctx, tx, table, column, false))
	exec(t, ctx, tx, "INSERT INTO "+sql+" ("+quoted+") VALUES ('first')")
	expectedFailure(t, ctx, tx, "ERR_DDL_CONFLICT", 409, func() error { return AddUnique(ctx, tx, table, column, true) })
	exec(t, ctx, tx, "UPDATE "+sql+" SET "+quoted+"=id::text")
	must(t, SetNotNull(ctx, tx, table, column, true))
	must(t, SetNotNull(ctx, tx, table, column, false))
	exec(t, ctx, tx, "INSERT INTO "+sql+" DEFAULT VALUES")
	check(t, ctx, tx, "SELECT count(*)=5 AND count("+quoted+")=4 FROM "+sql)
	other := identifier(t, Column)
	must(t, AddColumn(ctx, tx, table, ColumnSpec{Name: other, Type: DataType{Kind: Integer}, NotNull: true, Default: DefaultValue{Kind: Literal, Literal: "7"}}))
	check(t, ctx, tx, "SELECT bool_and("+pgx.Identifier{other}.Sanitize()+"=7) FROM "+sql)
	must(t, DropColumn(ctx, tx, table, other))
	check(t, ctx, tx, "SELECT count(*)=5 AND bool_and("+quoted+"=id::text OR "+quoted+" IS NULL) FROM "+sql)
	check(t, ctx, tx, `SELECT NOT EXISTS(SELECT FROM pg_attribute WHERE attrelid=$1::regclass AND attname=$2 AND NOT attisdropped)`, sql, other)
	view := pgx.Identifier{table.Schema, "dependent_view"}.Sanitize()
	exec(t, ctx, tx, "CREATE VIEW "+view+" AS SELECT "+quoted+" FROM "+sql)
	exec(t, ctx, tx, "SAVEPOINT dependent_column")
	err := DropColumn(ctx, tx, table, column)
	var postgres interface{ SQLState() string }
	if !errors.As(err, &postgres) || postgres.SQLState() != "2BP01" {
		t.Fatalf("dependent column should fail without CASCADE: %v", err)
	}
	exec(t, ctx, tx, "ROLLBACK TO SAVEPOINT dependent_column")
	check(t, ctx, tx, "SELECT count(*)=5 FROM "+view)
}

func TestDDLCollisionsPostgreSQL(t *testing.T) {
	pool := database.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx := transaction(t, ctx, pool)
	table, sql := physicalTable(t, ctx, tx)
	column := identifier(t, Column)
	spec := ColumnSpec{Name: column, Type: DataType{Kind: Text}, Unique: true}
	must(t, AddColumn(ctx, tx, table, spec))
	for _, call := range []func() error{
		func() error { return CreateSchema(ctx, tx, table.Schema) },
		func() error { return CreateTable(ctx, tx, table) },
		func() error { return AddColumn(ctx, tx, table, spec) },
	} {
		expectedFailure(t, ctx, tx, "ERR_DDL_CONFLICT", 409, func() error {
			err := call()
			if !errors.Is(err, ErrIdentifierCollision) {
				t.Fatalf("collision lost: %v", err)
			}
			return err
		})
	}
	expectedFailure(t, ctx, tx, "ERR_DDL_CONFLICT", 409, func() error { return AddUnique(ctx, tx, table, column, true) })
	must(t, AddUnique(ctx, tx, table, column, false))
	check(t, ctx, tx, "SELECT count(*)=6 FROM pg_attribute WHERE attrelid=$1::regclass AND attnum>0 AND NOT attisdropped", sql)
}

func seedMetadata(t *testing.T, ctx context.Context, tx pgx.Tx, table TableName, column string) string {
	t.Helper()
	var workspace, book, id string
	must(t, tx.QueryRow(ctx, `INSERT INTO meta.workspaces(name) VALUES ('DDL fixture') RETURNING id::text`).Scan(&workspace))
	must(t, tx.QueryRow(ctx, `INSERT INTO meta.books(workspace_id,name,pg_schema,position)
		VALUES ($1,'Book',$2,0) RETURNING id::text`, workspace, table.Schema).Scan(&book))
	must(t, tx.QueryRow(ctx, `INSERT INTO meta.tables(book_id,name,pg_name,position)
		VALUES ($1,'Table',$2,0) RETURNING id::text`, book, table.Name).Scan(&id))
	exec(t, ctx, tx, `INSERT INTO meta.fields(table_id,name,ui_type,pg_column,pg_type,position)
		VALUES ($1,'Name','text',$2,'text',5)`, id, column)
	return id
}

func TestDDLMetadataAtomicityPostgreSQL(t *testing.T) {
	pool := database.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	table := TableName{Schema: identifier(t, Book), Name: identifier(t, Table)}
	column := identifier(t, Column)
	sentinel := errors.New("service abort")
	for _, rollback := range []bool{true, false} {
		err := pool.WithTx(ctx, func(tx pgx.Tx) error {
			id := seedMetadata(t, ctx, tx, table, column)
			must(t, CreateSchema(ctx, tx, table.Schema))
			must(t, CreateTable(ctx, tx, table))
			must(t, AddColumn(ctx, tx, table, ColumnSpec{Name: column, Type: DataType{Kind: Text}}))
			exec(t, ctx, tx, `UPDATE meta.tables SET schema_version=schema_version+1 WHERE id=$1`, id)
			if rollback {
				return sentinel
			}
			return nil
		})
		if rollback && !errors.Is(err, sentinel) || !rollback && err != nil {
			t.Fatalf("transaction result = %v", err)
		}
		var present bool
		must(t, pool.QueryRow(ctx, `SELECT EXISTS(SELECT FROM pg_namespace WHERE nspname=$1)
			AND EXISTS(SELECT FROM meta.books WHERE pg_schema=$1)`, table.Schema).Scan(&present))
		if present == rollback {
			t.Fatalf("metadata/DDL presence after rollback=%t: %t", rollback, present)
		}
		if rollback {
			var absent bool
			must(t, pool.QueryRow(ctx, `SELECT NOT EXISTS(SELECT FROM pg_namespace WHERE nspname=$1)
				AND NOT EXISTS(SELECT FROM meta.books WHERE pg_schema=$1)`, table.Schema).Scan(&absent))
			if !absent {
				t.Fatal("partial metadata or DDL after rollback")
			}
		}
	}
	sql, err := table.sql()
	must(t, err)
	_, err = pool.Exec(ctx, "INSERT INTO "+sql+" ("+pgx.Identifier{column}.Sanitize()+") VALUES ('retained')")
	must(t, err)
	tx := transaction(t, ctx, pool)
	// Metadata-only renames are catalog evidence, not the future AT-202 API.
	exec(t, ctx, tx, `UPDATE meta.tables SET name=$2 WHERE pg_name=$1`, table.Name, "中文 table ; DROP TABLE meta.users;--")
	exec(t, ctx, tx, `UPDATE meta.fields SET name=$2 WHERE pg_column=$1`, column, "中文 field with spaces")
	check(t, ctx, tx, `SELECT t.pg_name=$1 AND f.pg_column=$2 AND t.schema_version=2
		FROM meta.tables t JOIN meta.fields f ON f.table_id=t.id WHERE t.pg_name=$1`, table.Name, column)
	check(t, ctx, tx, "SELECT "+pgx.Identifier{column}.Sanitize()+"='retained' FROM "+sql)
	must(t, AddUnique(ctx, tx, table, column, true))
	must(t, SetNotNull(ctx, tx, table, column, true))
	must(t, SetDefault(ctx, tx, table, column, DataType{Kind: Text}, DefaultValue{Kind: Literal, Literal: "temporary"}))
	must(t, DropColumn(ctx, tx, table, column))
	exec(t, ctx, tx, `UPDATE meta.tables SET schema_version=3 WHERE pg_name=$1`, table.Name)
	must(t, tx.Rollback(ctx))
	tx = transaction(t, ctx, pool)
	check(t, ctx, tx, "SELECT "+pgx.Identifier{column}.Sanitize()+"='retained' FROM "+sql+" WHERE id=1")
	check(t, ctx, tx, `SELECT name='Table' AND schema_version=2 FROM meta.tables WHERE pg_name=$1`, table.Name)
	check(t, ctx, tx, `SELECT NOT attnotnull AND NOT atthasdef FROM pg_attribute WHERE attrelid=$1::regclass AND attname=$2`, sql, column)
	check(t, ctx, tx, `SELECT count(*)=1 FROM pg_constraint WHERE conrelid=$1::regclass AND contype<>'n'`, sql)
}

func TestDDLInjectionPostgreSQL(t *testing.T) {
	pool := database.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx := transaction(t, ctx, pool)
	table, sql := physicalTable(t, ctx, tx)
	for _, mode := range []string{"on", "off"} {
		exec(t, ctx, tx, "SELECT set_config('standard_conforming_strings',$1,true)", mode)
		for _, payload := range []string{`'; DROP TABLE meta.users CASCADE; --`, `\'; DROP TABLE meta.users CASCADE; --`, `"; SELECT pg_sleep(30);--`, "中文'\n\\\\$1"} {
			column := identifier(t, Column)
			must(t, AddColumn(ctx, tx, table, ColumnSpec{Name: column, Type: DataType{Kind: Text}, Default: DefaultValue{Kind: Literal, Literal: payload}}))
			exec(t, ctx, tx, "INSERT INTO "+sql+" DEFAULT VALUES")
			check(t, ctx, tx, "SELECT "+pgx.Identifier{column}.Sanitize()+"=$1 FROM "+sql+" ORDER BY id DESC LIMIT 1", payload)
		}
	}
	payload := `c_aaaa"; DROP TABLE meta.users CASCADE;--`
	assertError(t, DropColumn(ctx, tx, table, payload), 400, "ERR_VALIDATION")
	assertError(t, CreateSchema(ctx, tx, `b_aaaa; DROP SCHEMA meta CASCADE`), 400, "ERR_VALIDATION")
	assertError(t, AddColumn(ctx, tx, table, ColumnSpec{Name: identifier(t, Column), Type: DataType{Kind: "text; SELECT pg_sleep(30)"}}), 400, "ERR_VALIDATION")
	check(t, ctx, tx, "SELECT to_regclass('meta.users') IS NOT NULL AND to_regclass('meta.fields') IS NOT NULL")
}

func TestDDLLockTimeoutPostgreSQL(t *testing.T) {
	pool := database.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	setup := transaction(t, ctx, pool)
	table, sql := physicalTable(t, ctx, setup)
	initialColumn := identifier(t, Column)
	id := seedMetadata(t, ctx, setup, table, initialColumn)
	must(t, AddColumn(ctx, setup, table, ColumnSpec{Name: initialColumn, Type: DataType{Kind: Text}}))
	must(t, setup.Commit(ctx))
	blocker := transaction(t, ctx, pool)
	exec(t, ctx, blocker, "LOCK TABLE "+sql+" IN ACCESS SHARE MODE")
	waiter := transaction(t, ctx, pool)
	column := identifier(t, Column)
	exec(t, ctx, waiter, "UPDATE meta.tables SET schema_version=2 WHERE id=$1", id)
	exec(t, ctx, waiter, `INSERT INTO meta.fields(table_id,name,ui_type,pg_column,pg_type,position)
		VALUES ($1,'New field','text',$2,'text',6)`, id, column)
	result := make(chan error, 1)
	start := time.Now()
	go func() {
		result <- AddColumn(ctx, waiter, table, ColumnSpec{Name: column, Type: DataType{Kind: Text}})
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	observe, stop := context.WithTimeout(ctx, 2*time.Second)
	defer stop()
	for {
		var blocked bool
		err := pool.QueryRow(observe, "SELECT $1::integer=ANY(pg_blocking_pids($2::integer))",
			blocker.Conn().PgConn().PID(), waiter.Conn().PgConn().PID()).Scan(&blocked)
		must(t, err)
		if blocked {
			break
		}
		select {
		case err := <-result:
			t.Fatalf("DDL returned before observed contention: %v", err)
		case <-ticker.C:
		case <-observe.Done():
			t.Fatal("DDL never reached the expected table lock")
		}
	}
	err := <-result
	elapsed := time.Since(start)
	assertError(t, err, 409, "ERR_DDL_LOCK_TIMEOUT")
	if elapsed < 2500*time.Millisecond || elapsed > 4500*time.Millisecond {
		t.Fatalf("3s lock_timeout elapsed = %s", elapsed)
	}
	must(t, waiter.Rollback(ctx))
	must(t, blocker.Rollback(ctx))
	tx := transaction(t, ctx, pool)
	check(t, ctx, tx, "SELECT schema_version=1 FROM meta.tables WHERE id=$1", id)
	check(t, ctx, tx, "SELECT count(*)=0 FROM pg_attribute WHERE attrelid=$1::regclass AND attname=$2", sql, column)
	check(t, ctx, tx, "SELECT count(*)=0 FROM meta.fields WHERE table_id=$1 AND pg_column=$2", id, column)
	must(t, AddColumn(ctx, tx, table, ColumnSpec{Name: column, Type: DataType{Kind: Text}}))
	check(t, ctx, tx, "SELECT current_setting('lock_timeout')='3s'")
}

func TestDDLLocalTimeoutResetPostgreSQL(t *testing.T) {
	pool := database.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pool.Acquire(ctx)
	must(t, err)
	defer conn.Release()
	var initial, final string
	must(t, conn.QueryRow(ctx, "SHOW lock_timeout").Scan(&initial))
	for _, commit := range []bool{false, true} {
		tx, err := conn.Begin(ctx)
		must(t, err)
		name := identifier(t, Book)
		must(t, CreateSchema(ctx, tx, name))
		check(t, ctx, tx, "SELECT current_setting('lock_timeout')='3s'")
		if commit {
			must(t, tx.Commit(ctx))
		} else {
			must(t, tx.Rollback(ctx))
		}
		must(t, conn.QueryRow(ctx, "SHOW lock_timeout").Scan(&final))
		if strings.TrimSpace(final) != strings.TrimSpace(initial) {
			t.Fatalf("lock timeout leaked: %q -> %q", initial, final)
		}
	}
}
