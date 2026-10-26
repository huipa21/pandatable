package structure

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"pandatable/internal/ddl"
)

// physicalColumn names one generated user column. Every data statement in this
// file validates the three generated identifiers before quoting them; values
// are always bind parameters (D-127, D-128).
type physicalColumn struct {
	table  ddl.TableName
	column string
}

func (target physicalColumn) sql() (table, column string, err error) {
	if err := ddl.ValidateIdentifier(ddl.Book, target.table.Schema); err != nil {
		return "", "", err
	}
	if err := ddl.ValidateIdentifier(ddl.Table, target.table.Name); err != nil {
		return "", "", err
	}
	if err := ddl.ValidateIdentifier(ddl.Column, target.column); err != nil {
		return "", "", err
	}
	return pgx.Identifier{target.table.Schema, target.table.Name}.Sanitize(), pgx.Identifier{target.column}.Sanitize(), nil
}

func (target physicalColumn) count(ctx context.Context, tx pgx.Tx, operation, query string) (int64, error) {
	table, column, err := target.sql()
	if err != nil {
		return 0, err
	}
	var count int64
	if err := tx.QueryRow(ctx, fmt.Sprintf(query, table, column)).Scan(&count); err != nil {
		return 0, fmt.Errorf("%s: %w", operation, err)
	}
	return count, nil
}

// emptyCount is the number of rows whose column is NULL.
func (target physicalColumn) emptyCount(ctx context.Context, tx pgx.Tx) (int64, error) {
	return target.count(ctx, tx, "count empty cells", "SELECT count(*) FROM %[1]s WHERE %[2]s IS NULL")
}

// duplicateCount is the number of distinct non-NULL values stored more than once.
func (target physicalColumn) duplicateCount(ctx context.Context, tx pgx.Tx) (int64, error) {
	return target.count(ctx, tx, "count duplicate values",
		"SELECT count(*) FROM (SELECT 1 FROM %[1]s WHERE %[2]s IS NOT NULL GROUP BY %[2]s HAVING count(*) > 1) AS d")
}

// fillEmpty writes value (a codec-decoded typed parameter) into NULL cells.
func (target physicalColumn) fillEmpty(ctx context.Context, tx pgx.Tx, value any) error {
	table, column, err := target.sql()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE "+table+" SET "+column+" = $1 WHERE "+column+" IS NULL", value); err != nil {
		return fmt.Errorf("fill empty cells: %w", err)
	}
	return nil
}

// renameLabels rewrites select labels old[i] → new[i] simultaneously. Multi
// arrays keep element order, map every element and drop later duplicates
// produced by merging into an orphan label.
func (target physicalColumn) renameLabels(ctx context.Context, tx pgx.Tx, multi bool, from, to []string) error {
	if len(from) == 0 {
		return nil
	}
	table, column, err := target.sql()
	if err != nil {
		return err
	}
	query := "UPDATE " + table + " AS r SET " + column + " = m.new FROM unnest($1::text[], $2::text[]) AS m(old, new) WHERE r." +
		column + " = m.old"
	if multi {
		query = "UPDATE " + table + " AS r SET " + column + ` = (
			SELECT array_agg(d.x ORDER BY d.o) FROM (
				SELECT COALESCE(m.new, u.e) AS x, min(u.o) AS o
				FROM unnest(r.` + column + `) WITH ORDINALITY AS u(e, o)
				LEFT JOIN unnest($1::text[], $2::text[]) AS m(old, new) ON m.old = u.e
				GROUP BY 1) AS d)
			WHERE r.` + column + " && $1::text[]"
	}
	if _, err := tx.Exec(ctx, query, from, to); err != nil {
		return fmt.Errorf("rename select labels: %w", err)
	}
	return nil
}
