// Package doctor checks metadata against the PostgreSQL catalog without repairs.
package doctor

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"pandatable/internal/db"
	"pandatable/migrations"
)

// Finding describes one observed mismatch; names are physical database identifiers.
type Finding struct {
	Kind     string
	Object   string
	Expected string
	Actual   string
}

// Check examines a consistent, read-only snapshot, including objects in Trash.
// It does not validate indexes, constraints, defaults, row data, or stored files.
func Check(ctx context.Context, pool *db.Pool) ([]Finding, error) {
	if pool == nil || pool.Pool == nil {
		return nil, errors.New("doctor requires a database pool")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("begin doctor snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	findings, err := checkMigrations(ctx, tx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, driftQuery)
	if err != nil {
		return nil, fmt.Errorf("read metadata and catalog: %w", err)
	}
	for rows.Next() {
		var finding Finding
		if err := rows.Scan(&finding.Kind, &finding.Object, &finding.Expected, &finding.Actual); err != nil {
			rows.Close()
			return nil, fmt.Errorf("read drift finding: %w", err)
		}
		findings = append(findings, finding)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("finish reading drift findings: %w", err)
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("finish doctor snapshot: %w", err)
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Kind == findings[j].Kind {
			return findings[i].Object < findings[j].Object
		}
		return findings[i].Kind < findings[j].Kind
	})
	return findings, nil
}

func checkMigrations(ctx context.Context, tx pgx.Tx) ([]Finding, error) {
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}
	expected := make(map[int64]bool)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		prefix, _, ok := strings.Cut(entry.Name(), "_")
		version, parseErr := strconv.ParseInt(prefix, 10, 64)
		if !ok || parseErr != nil || version < 1 || expected[version] {
			return nil, fmt.Errorf("invalid embedded migration name %q", entry.Name())
		}
		expected[version] = true
	}
	if len(expected) == 0 {
		return nil, errors.New("no embedded migrations found")
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT ON (version_id) version_id, is_applied
		FROM public.goose_db_version WHERE version_id > 0 ORDER BY version_id, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("read migration history: %w", err)
	}
	applied := make(map[int64]bool)
	for rows.Next() {
		var version int64
		var installed bool
		if err := rows.Scan(&version, &installed); err != nil {
			rows.Close()
			return nil, fmt.Errorf("read migration version: %w", err)
		}
		applied[version] = installed
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("finish reading migration history: %w", err)
	}
	rows.Close()
	var findings []Finding
	for version := range expected {
		if !applied[version] {
			findings = append(findings, Finding{"migration", strconv.FormatInt(version, 10), "applied", "missing or reverted"})
		}
	}
	for version, installed := range applied {
		if installed && !expected[version] {
			findings = append(findings, Finding{"migration", strconv.FormatInt(version, 10), "known to this binary", "unknown applied version"})
		}
	}
	return findings, nil
}

const driftQuery = `
WITH expected_tables AS (
  SELECT b.pg_schema AS schema_name, t.pg_name AS table_name, 'table' AS kind
  FROM meta.tables t JOIN meta.books b ON b.id = t.book_id
  UNION ALL
  SELECT b.pg_schema, r.junction_table, 'junction'
  FROM meta.relations r JOIN meta.books b ON b.id = r.book_id
  WHERE r.junction_table IS NOT NULL
), expected_columns AS (
  SELECT b.pg_schema AS schema_name, t.pg_name AS table_name,
         f.pg_column AS column_name, f.pg_type AS type_name
  FROM meta.fields f JOIN meta.tables t ON t.id = f.table_id
  JOIN meta.books b ON b.id = t.book_id
  WHERE f.pg_column IS NOT NULL AND f.pg_type IS NOT NULL
  UNION ALL
  SELECT b.pg_schema, t.pg_name, r.fk_column, 'bigint'
  FROM meta.relations r JOIN meta.books b ON b.id = r.book_id
  JOIN meta.tables t ON t.id = r.child_table_id
  WHERE r.fk_column IS NOT NULL
  UNION ALL
  SELECT b.pg_schema, r.junction_table, cols.column_name, cols.type_name
  FROM meta.relations r JOIN meta.books b ON b.id = r.book_id
  CROSS JOIN (VALUES ('parent_id','bigint'), ('child_id','bigint'),
                     ('created_at','timestamptz'), ('created_by','uuid')) AS cols(column_name,type_name)
  WHERE r.junction_table IS NOT NULL
), findings AS (
  SELECT 'missing_schema' AS kind, pg_catalog.format('%I', b.pg_schema) AS object,
         'book schema' AS expected, 'missing' AS actual
  FROM meta.books b LEFT JOIN pg_catalog.pg_namespace n ON n.nspname = b.pg_schema
  WHERE n.oid IS NULL
  UNION ALL
  SELECT 'unexpected_schema', pg_catalog.format('%I', n.nspname), 'registered book schema', 'unregistered'
  FROM pg_catalog.pg_namespace n
  WHERE n.nspname ~ '^b_[a-z2-7]{10}$'
    AND NOT EXISTS (SELECT 1 FROM meta.books b WHERE b.pg_schema = n.nspname)
  UNION ALL
  SELECT 'missing_table', pg_catalog.format('%I.%I', e.schema_name, e.table_name),
         e.kind, 'missing or not a regular table'
  FROM expected_tables e
  LEFT JOIN pg_catalog.pg_namespace n ON n.nspname = e.schema_name
  LEFT JOIN pg_catalog.pg_class c ON c.relnamespace = n.oid AND c.relname = e.table_name AND c.relkind = 'r'
  WHERE c.oid IS NULL
  UNION ALL
  SELECT 'unexpected_table', pg_catalog.format('%I.%I', b.pg_schema, c.relname),
         'registered table or junction', 'unregistered'
  FROM meta.books b JOIN pg_catalog.pg_namespace n ON n.nspname = b.pg_schema
  JOIN pg_catalog.pg_class c ON c.relnamespace = n.oid AND c.relkind IN ('r','p')
  WHERE NOT EXISTS (SELECT 1 FROM expected_tables e
                    WHERE e.schema_name = b.pg_schema AND e.table_name = c.relname)
  UNION ALL
  SELECT 'field_metadata', pg_catalog.format('%I.%I field %s', b.pg_schema, t.pg_name, f.id),
         CASE WHEN f.ui_type IN ('link','formula','lookup','rollup') THEN 'virtual field' ELSE 'stored field' END,
         'inconsistent physical column or type'
  FROM meta.fields f JOIN meta.tables t ON t.id = f.table_id JOIN meta.books b ON b.id = t.book_id
  WHERE (f.ui_type IN ('link','formula','lookup','rollup') AND (f.pg_column IS NOT NULL OR f.pg_type IS NOT NULL))
     OR (f.ui_type NOT IN ('link','formula','lookup','rollup') AND
         (f.pg_column IS NULL OR f.pg_column = '' OR f.pg_type IS NULL OR f.pg_type = ''))
  UNION ALL
  SELECT CASE WHEN a.attnum IS NULL THEN 'missing_column' ELSE 'column_type' END,
         pg_catalog.format('%I.%I.%I', e.schema_name, e.table_name, e.column_name),
         e.type_name, COALESCE(pg_catalog.format_type(a.atttypid, a.atttypmod), 'missing')
  FROM expected_columns e
  LEFT JOIN pg_catalog.pg_namespace n ON n.nspname = e.schema_name
  LEFT JOIN pg_catalog.pg_class c ON c.relnamespace = n.oid AND c.relname = e.table_name AND c.relkind = 'r'
  LEFT JOIN pg_catalog.pg_attribute a ON a.attrelid = c.oid AND a.attname = e.column_name
    AND a.attnum > 0 AND NOT a.attisdropped
  WHERE a.attnum IS NULL OR pg_catalog.format_type(a.atttypid, a.atttypmod) <>
    CASE WHEN e.type_name = 'timestamptz' THEN 'timestamp with time zone'
         WHEN e.type_name = 'time' THEN 'time without time zone'
         WHEN e.type_name LIKE 'varchar(%' THEN 'character varying' || substring(e.type_name FROM 8)
         ELSE e.type_name END
  UNION ALL
  SELECT 'unexpected_column', pg_catalog.format('%I.%I.%I', e.schema_name, e.table_name, a.attname),
         'registered field or relation column', 'unregistered'
  FROM expected_tables e
  JOIN pg_catalog.pg_namespace n ON n.nspname = e.schema_name
  JOIN pg_catalog.pg_class c ON c.relnamespace = n.oid AND c.relname = e.table_name AND c.relkind = 'r'
  JOIN pg_catalog.pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
  WHERE NOT EXISTS (SELECT 1 FROM expected_columns x
                    WHERE x.schema_name = e.schema_name AND x.table_name = e.table_name AND x.column_name = a.attname)
)
SELECT kind, object, expected, actual FROM findings ORDER BY kind, object`
