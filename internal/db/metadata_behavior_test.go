package db_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"pandatable/internal/db"
)

type metadataDB interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type metadataFixture struct {
	user, group, workspace, book, otherBook, table, child, otherTable string
	field, childField, otherField, parentLink, childLink              string
	relation, sharedView, personalView, grant, comment, notification  string
	attachment, backup                                                string
}

func metadataTx(t *testing.T, ctx context.Context, pool *db.Pool) pgx.Tx {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tx.Rollback(cleanupCtx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error(err)
		}
	})
	return tx
}

func execSQL(t *testing.T, ctx context.Context, database metadataDB, query string, args ...any) {
	t.Helper()
	if _, err := database.Exec(ctx, query, args...); err != nil {
		t.Fatal(err)
	}
}

func assertSQL(t *testing.T, ctx context.Context, database metadataDB, query string, args ...any) {
	t.Helper()
	var ok bool
	if err := database.QueryRow(ctx, query, args...).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("database assertion failed: %s", query)
	}
}

func expectSQLState(t *testing.T, ctx context.Context, tx pgx.Tx, state, query string, args ...any) {
	t.Helper()
	execSQL(t, ctx, tx, "SAVEPOINT expected_failure")
	_, err := tx.Exec(ctx, query, args...)
	execSQL(t, ctx, tx, "ROLLBACK TO SAVEPOINT expected_failure")
	execSQL(t, ctx, tx, "RELEASE SAVEPOINT expected_failure")
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != state {
		t.Fatalf("SQLSTATE = %v, want %s for %s", err, state, query)
	}
}

func seedMetadata(t *testing.T, ctx context.Context, database metadataDB) metadataFixture {
	t.Helper()
	id := func(query string, args ...any) string {
		t.Helper()
		var value string
		if err := database.QueryRow(ctx, query, args...).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	var f metadataFixture
	f.user = id(`INSERT INTO meta.users (username, display_name, password_hash, role, avatar_color)
		VALUES ('t11_actor', 'Test actor', 'test-only-phc', 'user', '#abcdef') RETURNING id::text`)
	execSQL(t, ctx, database, `INSERT INTO meta.sessions (id_hash, user_id, expires_at)
		VALUES (decode(repeat('cd', 32), 'hex'), $1, now() + interval '1 day')`, f.user)
	f.group = id(`INSERT INTO meta.groups (name) VALUES ('T11 Group') RETURNING id::text`)
	execSQL(t, ctx, database, `INSERT INTO meta.group_members VALUES ($1, $2)`, f.group, f.user)
	f.workspace = id(`INSERT INTO meta.workspaces (name, created_by, deleted_by)
		VALUES ('Workspace', $1, $1) RETURNING id::text`, f.user)
	f.book = id(`INSERT INTO meta.books (workspace_id, name, pg_schema, position, created_by, deleted_by)
		VALUES ($1, 'Book', 'b_aaaaaaaaaa', 0, $2, $2) RETURNING id::text`, f.workspace, f.user)
	f.otherBook = id(`INSERT INTO meta.books (workspace_id, name, pg_schema, position)
		VALUES ($1, 'Book', 'b_bbbbbbbbbb', 1) RETURNING id::text`, f.workspace)
	f.table = id(`INSERT INTO meta.tables (book_id, name, pg_name, position, created_by, deleted_by)
		VALUES ($1, 'Table', 't_aaaaaaaaaa', 0, $2, $2) RETURNING id::text`, f.book, f.user)
	f.child = id(`INSERT INTO meta.tables (book_id, name, pg_name, position)
		VALUES ($1, 'Table', 't_bbbbbbbbbb', 1) RETURNING id::text`, f.book)
	f.otherTable = id(`INSERT INTO meta.tables (book_id, name, pg_name, position)
		VALUES ($1, 'Table', 't_aaaaaaaaaa', 0) RETURNING id::text`, f.otherBook)
	f.field = id(`INSERT INTO meta.fields (table_id, name, ui_type, pg_column, pg_type, position)
		VALUES ($1, 'Name', 'text', 'c_aaaaaaaaaa', 'text', 0) RETURNING id::text`, f.table)
	f.childField = id(`INSERT INTO meta.fields (table_id, name, ui_type, pg_column, pg_type, position)
		VALUES ($1, 'Name', 'text', 'c_aaaaaaaaaa', 'text', 0) RETURNING id::text`, f.child)
	f.otherField = id(`INSERT INTO meta.fields (table_id, name, ui_type, pg_column, pg_type, position)
		VALUES ($1, 'Name', 'text', 'c_aaaaaaaaaa', 'text', 0) RETURNING id::text`, f.otherTable)
	f.parentLink = id(`INSERT INTO meta.fields (table_id, name, ui_type, position)
		VALUES ($1, 'Children', 'link', 1) RETURNING id::text`, f.table)
	f.childLink = id(`INSERT INTO meta.fields (table_id, name, ui_type, pg_column, pg_type, position)
		VALUES ($1, 'Parent', 'link', 'c_bbbbbbbbbb', 'bigint', 1) RETURNING id::text`, f.child)
	execSQL(t, ctx, database, `UPDATE meta.tables SET display_field_id = $2 WHERE id = $1`, f.table, f.field)
	f.relation = id(`INSERT INTO meta.relations
		(book_id, kind, parent_table_id, child_table_id, parent_field_id, child_field_id, fk_column)
		VALUES ($1, 'one_many', $2, $3, $4, $5, 'c_bbbbbbbbbb') RETURNING id::text`,
		f.book, f.table, f.child, f.parentLink, f.childLink)
	f.sharedView = id(`INSERT INTO meta.views (table_id, name, type, scope, position, created_by)
		VALUES ($1, 'Grid', 'grid', 'shared', 0, $2) RETURNING id::text`, f.table, f.user)
	f.personalView = id(`INSERT INTO meta.views (table_id, name, type, scope, owner_id, position)
		VALUES ($1, 'Grid', 'grid', 'personal', $2, 1) RETURNING id::text`, f.table, f.user)
	f.grant = id(`INSERT INTO meta.grants (resource_type, resource_id, principal_type, principal_id, level, granted_by)
		VALUES ('workspace', $1, 'user', $2, 'owner', $2) RETURNING id::text`, f.workspace, f.user)
	f.comment = id(`INSERT INTO meta.comments (table_id, row_id, user_id, body)
		VALUES ($1, 999, $2, 'Test comment') RETURNING id::text`, f.table, f.user)
	f.notification = id(`INSERT INTO meta.notifications (user_id, kind, payload)
		VALUES ($1, 'mention', '{}') RETURNING id::text`, f.user)
	execSQL(t, ctx, database, `INSERT INTO meta.row_history (table_id, row_id, user_id, action, changes)
		VALUES ($1, 999, $2, 'insert', '{}')`, f.table, f.user)
	f.attachment = id(`INSERT INTO meta.attachments
		(table_id, field_id, file_name, mime_type, size_bytes, sha256, storage_path, uploaded_by)
		VALUES ($1, $2, 'test.txt', 'text/plain', 0, decode(repeat('ab', 32), 'hex'), 'test/test.txt', $3)
		RETURNING id::text`, f.table, f.field, f.user)
	f.backup = id(`INSERT INTO meta.backups (file_name, status, created_by)
		VALUES ('test-backup.zip', 'running', $1) RETURNING id::text`, f.user)
	return f
}

func TestMetadataDefaultsPostgreSQL(t *testing.T) {
	pool := database.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx := metadataTx(t, ctx, pool)
	f := seedMetadata(t, ctx, tx)
	for _, id := range []string{f.group, f.workspace, f.book, f.otherBook, f.table, f.child, f.otherTable,
		f.field, f.childField, f.otherField, f.parentLink, f.childLink, f.relation, f.sharedView,
		f.personalView, f.grant, f.comment, f.notification, f.attachment, f.backup} {
		assertSQL(t, ctx, tx, `SELECT uuid_extract_version($1::uuid) = 7`, id)
	}

	assertSQL(t, ctx, tx, `SELECT description = '' AND created_at = now() FROM meta.groups WHERE id = $1`, f.group)
	assertSQL(t, ctx, tx, `SELECT icon IS NULL AND color IS NULL AND deleted_at IS NULL AND created_at = now()
		FROM meta.workspaces WHERE id = $1`, f.workspace)
	assertSQL(t, ctx, tx, `SELECT schema_version = 1 AND created_at = now() AND deleted_at IS NULL
		FROM meta.tables WHERE id = $1`, f.table)
	assertSQL(t, ctx, tx, `SELECT description = '' AND options = '{}' AND NOT is_system AND NOT is_required
		AND NOT is_unique AND default_json IS NULL AND version = 1 AND created_at = now() AND deleted_at IS NULL
		FROM meta.fields WHERE id = $1`, f.field)
	assertSQL(t, ctx, tx, `SELECT pg_column IS NULL AND pg_type IS NULL FROM meta.fields WHERE id = $1`, f.parentLink)
	assertSQL(t, ctx, tx, `SELECT on_delete = 'set_null' AND junction_table IS NULL AND created_at = now()
		FROM meta.relations WHERE id = $1`, f.relation)
	assertSQL(t, ctx, tx, `SELECT owner_id IS NULL AND NOT locked AND config = '{}' AND version = 1
		AND created_at = now() AND updated_at = now() AND deleted_at IS NULL FROM meta.views WHERE id = $1`, f.sharedView)
	assertSQL(t, ctx, tx, `SELECT mentions = '{}'::uuid[] AND edited_at IS NULL AND deleted_at IS NULL AND created_at = now()
		FROM meta.comments WHERE id = $1`, f.comment)
	assertSQL(t, ctx, tx, `SELECT read_at IS NULL AND created_at = now() FROM meta.notifications WHERE id = $1`, f.notification)
	assertSQL(t, ctx, tx, `SELECT id > 0 AND created_at = now() FROM meta.row_history WHERE table_id = $1`, f.table)
	assertSQL(t, ctx, tx, `SELECT NOT has_thumb AND created_at = now() FROM meta.attachments WHERE id = $1`, f.attachment)
	assertSQL(t, ctx, tx, `SELECT size_bytes IS NULL AND error IS NULL AND finished_at IS NULL AND created_at = now()
		FROM meta.backups WHERE id = $1`, f.backup)
	execSQL(t, ctx, tx, `INSERT INTO meta.row_history (table_id, action, changes) VALUES ($1, 'type_change', '{}')`, f.table)
	assertSQL(t, ctx, tx, `SELECT count(DISTINCT id) = 2 FROM meta.row_history`)
	execSQL(t, ctx, tx, `INSERT INTO meta.fields (table_id, name, ui_type, position, default_json)
		VALUES ($1, 'Computed', 'formula', 2, '42')`, f.table)
	t.Log("actual UUIDv7/default/timestamp/identity values, multiple virtual columns and table-level type_change verified")
}

func TestRestoreSystemConstraintPostgreSQL(t *testing.T) {
	pool := database.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx := metadataTx(t, ctx, pool)
	f := seedMetadata(t, ctx, tx)
	expectSQLState(t, ctx, tx, "23514",
		`UPDATE meta.row_history SET restore_system = '{}' WHERE table_id = $1`, f.table)
	expectSQLState(t, ctx, tx, "23514",
		`UPDATE meta.row_history SET action = 'delete', restore_system = '[]' WHERE table_id = $1`, f.table)
	execSQL(t, ctx, tx,
		`UPDATE meta.row_history SET action = 'delete', restore_system = '{}' WHERE table_id = $1`, f.table)
}

func TestMetadataConstraintsPostgreSQL(t *testing.T) {
	pool := database.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	tx := metadataTx(t, ctx, pool)
	f := seedMetadata(t, ctx, tx)
	tests := []struct {
		name, state, query string
		args               []any
	}{
		{"group name", "23505", `INSERT INTO meta.groups (name) VALUES ('T11 Group')`, nil},
		{"membership pair", "23505", `INSERT INTO meta.group_members VALUES ($1, $2)`, []any{f.group, f.user}},
		{"physical schema", "23505", `UPDATE meta.books SET pg_schema = 'b_aaaaaaaaaa' WHERE id = $1`, []any{f.otherBook}},
		{"physical table", "23505", `UPDATE meta.tables SET pg_name = 't_aaaaaaaaaa' WHERE id = $1`, []any{f.child}},
		{"physical column", "23505", `UPDATE meta.fields SET pg_column = 'c_aaaaaaaaaa' WHERE id = $1`, []any{f.parentLink}},
		{"active field name", "23505", `UPDATE meta.fields SET name = 'nAmE' WHERE id = $1`, []any{f.parentLink}},
		{"grant pair", "23505", `INSERT INTO meta.grants (resource_type, resource_id, principal_type, principal_id, level)
			VALUES ('workspace', $1, 'user', $2, 'read')`, []any{f.workspace, f.user}},
		{"backup file", "23505", `INSERT INTO meta.backups (file_name, status) VALUES ('test-backup.zip', 'running')`, nil},
		{"relation FK column", "23505", `INSERT INTO meta.relations
			(book_id, kind, parent_table_id, child_table_id, parent_field_id, child_field_id, fk_column)
			SELECT book_id, kind, parent_table_id, child_table_id, parent_field_id, child_field_id, fk_column FROM meta.relations`, nil},
		{"missing field table", "23503", `UPDATE meta.fields SET table_id = uuidv7() WHERE id = $1`, []any{f.otherField}},
		{"cross-table display", "23503", `UPDATE meta.tables SET display_field_id = $2 WHERE id = $1`, []any{f.table, f.childField}},
		{"cross-table attachment", "23503", `UPDATE meta.attachments SET field_id = $1`, []any{f.childField}},
		{"cross-table parent field", "23503", `UPDATE meta.relations SET parent_field_id = $1`, []any{f.childField}},
		{"cross-table child field", "23503", `UPDATE meta.relations SET child_field_id = $1`, []any{f.field}},
		{"cross-book child", "23503", `UPDATE meta.relations SET child_table_id = $1, child_field_id = $2`, []any{f.otherTable, f.otherField}},
		{"cross-book parent", "23503", `UPDATE meta.relations SET parent_table_id = $1, parent_field_id = $2`, []any{f.otherTable, f.otherField}},
		{"move referenced table", "23503", `UPDATE meta.tables SET book_id = $1 WHERE id = $2`, []any{f.otherBook, f.child}},
		{"missing history table", "23503", `UPDATE meta.row_history SET table_id = uuidv7()`, nil},
		{"missing membership user", "23503", `UPDATE meta.group_members SET user_id = uuidv7()`, nil},
		{"missing membership group", "23503", `UPDATE meta.group_members SET group_id = uuidv7()`, nil},
		{"missing view owner", "23503", `UPDATE meta.views SET owner_id = uuidv7() WHERE scope = 'personal'`, nil},
		{"missing notification user", "23503", `UPDATE meta.notifications SET user_id = uuidv7()`, nil},
		{"missing authorship user", "23503", `UPDATE meta.workspaces SET created_by = uuidv7()`, nil},
		{"required position", "23502", `UPDATE meta.fields SET position = NULL`, nil},
		{"required workspace", "23502", `UPDATE meta.books SET workspace_id = NULL`, nil},
		{"group name required", "23502", `INSERT INTO meta.groups DEFAULT VALUES`, nil},
		{"relation kind", "23514", `UPDATE meta.relations SET kind = 'invalid'`, nil},
		{"relation on_delete", "23514", `UPDATE meta.relations SET on_delete = 'invalid'`, nil},
		{"relation missing FK", "23514", `UPDATE meta.relations SET fk_column = NULL`, nil},
		{"relation extra junction", "23514", `UPDATE meta.relations SET junction_table = 'j_aaaaaaaaaa'`, nil},
		{"junction shape", "23514", `UPDATE meta.relations SET kind = 'many_many'`, nil},
		{"same link field", "23514", `UPDATE meta.relations SET child_table_id = parent_table_id, child_field_id = parent_field_id`, nil},
		{"view type", "23514", `UPDATE meta.views SET type = 'invalid'`, nil},
		{"view scope", "23514", `UPDATE meta.views SET scope = 'invalid'`, nil},
		{"personal owner required", "23514", `UPDATE meta.views SET owner_id = NULL WHERE scope = 'personal'`, nil},
		{"shared owner forbidden", "23514", `UPDATE meta.views SET owner_id = $1 WHERE scope = 'shared'`, []any{f.user}},
		{"personal lock forbidden", "23514", `UPDATE meta.views SET locked = true WHERE scope = 'personal'`, nil},
		{"grant resource kind", "23514", `UPDATE meta.grants SET resource_type = 'view'`, nil},
		{"grant principal kind", "23514", `UPDATE meta.grants SET principal_type = 'everyone'`, nil},
		{"grant level", "23514", `UPDATE meta.grants SET level = 'admin'`, nil},
		{"schema version", "23514", `UPDATE meta.tables SET schema_version = 0`, nil},
		{"field version", "23514", `UPDATE meta.fields SET version = 0`, nil},
		{"view version", "23514", `UPDATE meta.views SET version = 0`, nil},
		{"field options shape", "23514", `UPDATE meta.fields SET options = '[]'`, nil},
		{"view config shape", "23514", `UPDATE meta.views SET config = 'null'`, nil},
		{"negative upload size", "23514", `UPDATE meta.attachments SET size_bytes = -1`, nil},
		{"digest length", "23514", `UPDATE meta.attachments SET sha256 = decode('ab', 'hex')`, nil},
		{"backup status", "23514", `UPDATE meta.backups SET status = 'invalid'`, nil},
		{"negative backup size", "23514", `UPDATE meta.backups SET size_bytes = -1`, nil},
		{"history action", "23514", `UPDATE meta.row_history SET action = 'invalid'`, nil},
		{"row action requires row", "23514", `UPDATE meta.row_history SET row_id = NULL`, nil},
		{"table action forbids row", "23514", `UPDATE meta.row_history SET action = 'type_change'`, nil},
		{"history identity", "428C9", `INSERT INTO meta.row_history (id, table_id, row_id, action, changes)
			VALUES (42, $1, 1, 'insert', '{}')`, []any{f.table}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expectSQLState(t, ctx, tx, test.state, test.query, test.args...)
		})
	}
	for _, kind := range []string{"one_one", "one_many"} {
		execSQL(t, ctx, tx, `UPDATE meta.relations SET kind = $1`, kind)
	}
	for _, behavior := range []string{"set_null", "cascade", "restrict"} {
		execSQL(t, ctx, tx, `UPDATE meta.relations SET on_delete = $1`, behavior)
	}
	execSQL(t, ctx, tx, `UPDATE meta.relations SET kind = 'many_many', fk_column = NULL, junction_table = 'j_aaaaaaaaaa'`)
	expectSQLState(t, ctx, tx, "23505", `INSERT INTO meta.relations
		(book_id, kind, parent_table_id, child_table_id, parent_field_id, child_field_id, junction_table)
		SELECT book_id, kind, parent_table_id, child_table_id, parent_field_id, child_field_id, junction_table FROM meta.relations`)
	for _, kind := range []string{"grid", "form", "kanban", "gallery", "calendar"} {
		execSQL(t, ctx, tx, `UPDATE meta.views SET type = $1`, kind)
	}
	for _, level := range []string{"read", "write", "owner"} {
		execSQL(t, ctx, tx, `UPDATE meta.grants SET level = $1`, level)
	}
	for _, action := range []string{"insert", "update", "delete", "link", "unlink", "restore"} {
		execSQL(t, ctx, tx, `UPDATE meta.row_history SET action = $1`, action)
	}
	for _, status := range []string{"running", "done", "failed"} {
		execSQL(t, ctx, tx, `UPDATE meta.backups SET status = $1`, status)
	}
	// Self-links remain legal, but still have two distinct field IDs.
	execSQL(t, ctx, tx, `UPDATE meta.relations SET child_table_id = $1, child_field_id = $2`, f.table, f.field)
	execSQL(t, ctx, tx, `UPDATE meta.views SET locked = true WHERE scope = 'shared'`)
	// The polymorphic target columns intentionally have no existence FK.
	execSQL(t, ctx, tx, `INSERT INTO meta.grants (resource_type, resource_id, principal_type, principal_id, level)
		VALUES ('book', uuidv7(), 'group', uuidv7(), 'read'), ('table', uuidv7(), 'user', uuidv7(), 'write')`)
	t.Logf("%d invalid writes rejected with expected SQLSTATE; all documented enum values and self-links accepted", len(tests)+1)
}

func TestMetadataTrashAndNamesPostgreSQL(t *testing.T) {
	pool := database.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx := metadataTx(t, ctx, pool)
	f := seedMetadata(t, ctx, tx)
	execSQL(t, ctx, tx, `UPDATE meta.workspaces SET deleted_at = now();
		UPDATE meta.books SET deleted_at = now(); UPDATE meta.tables SET deleted_at = now();
		UPDATE meta.fields SET deleted_at = now(); UPDATE meta.views SET deleted_at = now();`)
	assertSQL(t, ctx, tx, `SELECT (SELECT count(*) = 1 FROM meta.relations) AND (SELECT count(*) = 1 FROM meta.attachments)
		AND (SELECT count(*) = 1 FROM meta.comments) AND (SELECT count(*) = 1 FROM meta.row_history)
		AND (SELECT count(*) = 1 FROM meta.grants) AND (SELECT count(*) = 2 FROM meta.views)`)
	execSQL(t, ctx, tx, `INSERT INTO meta.fields (table_id, name, ui_type, pg_column, position)
		VALUES ($1, 'nAmE', 'text', 'c_cccccccccc', 3)`, f.table)
	expectSQLState(t, ctx, tx, "23505", `UPDATE meta.fields SET deleted_at = NULL WHERE id = $1`, f.field)
	expectSQLState(t, ctx, tx, "23505", `INSERT INTO meta.fields (table_id, name, ui_type, pg_column, position)
		VALUES ($1, 'New field', 'text', 'c_aaaaaaaaaa', 4)`, f.table)
	expectSQLState(t, ctx, tx, "23505", `INSERT INTO meta.tables (book_id, name, pg_name, position)
		VALUES ($1, 'New table', 't_aaaaaaaaaa', 2)`, f.book)
	expectSQLState(t, ctx, tx, "23505", `INSERT INTO meta.books (workspace_id, name, pg_schema, position)
		VALUES ($1, 'New book', 'b_aaaaaaaaaa', 2)`, f.workspace)
	execSQL(t, ctx, tx, `INSERT INTO meta.groups (name) VALUES ('t11 group')`)
	execSQL(t, ctx, tx, `UPDATE meta.fields SET name = 'Restored name', deleted_at = NULL WHERE id = $1`, f.field)
	assertSQL(t, ctx, tx, `SELECT display_field_id = $2 AND deleted_at IS NOT NULL FROM meta.tables WHERE id = $1`, f.table, f.field)
	t.Log("soft delete retains metadata/references; display names freed, physical names reserved; conflicting restore rejected")
}

func TestMetadataDeletesPostgreSQL(t *testing.T) {
	pool := database.Open(t)
	for _, target := range []string{"user", "group", "display field", "relation field", "table", "book", "workspace"} {
		t.Run(target, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			tx := metadataTx(t, ctx, pool)
			f := seedMetadata(t, ctx, tx)
			switch target {
			case "user":
				execSQL(t, ctx, tx, `DELETE FROM meta.users WHERE id = $1`, f.user)
				assertSQL(t, ctx, tx, `SELECT (SELECT count(*) = 0 FROM meta.sessions)
					AND (SELECT count(*) = 0 FROM meta.group_members) AND (SELECT count(*) = 0 FROM meta.notifications)
					AND (SELECT count(*) = 0 FROM meta.views WHERE scope = 'personal')
					AND (SELECT created_by IS NULL FROM meta.views WHERE scope = 'shared')
					AND (SELECT created_by IS NULL AND deleted_by IS NULL FROM meta.workspaces)
					AND (SELECT bool_and(created_by IS NULL AND deleted_by IS NULL) FROM meta.books)
					AND (SELECT bool_and(created_by IS NULL AND deleted_by IS NULL) FROM meta.tables)
					AND (SELECT user_id IS NULL FROM meta.comments) AND (SELECT user_id IS NULL FROM meta.row_history)
					AND (SELECT uploaded_by IS NULL FROM meta.attachments) AND (SELECT created_by IS NULL FROM meta.backups)
					AND (SELECT granted_by IS NULL AND principal_id = $1 FROM meta.grants)`, f.user)
			case "group":
				execSQL(t, ctx, tx, `INSERT INTO meta.grants (resource_type, resource_id, principal_type, principal_id, level)
					VALUES ('workspace', $1, 'group', $2, 'read')`, f.workspace, f.group)
				execSQL(t, ctx, tx, `DELETE FROM meta.groups WHERE id = $1`, f.group)
				assertSQL(t, ctx, tx, `SELECT (SELECT count(*) = 0 FROM meta.group_members)
					AND (SELECT count(*) = 1 FROM meta.users WHERE id = $1)
					AND (SELECT count(*) = 1 FROM meta.grants WHERE principal_type = 'group')`, f.user)
			case "display field":
				execSQL(t, ctx, tx, `DELETE FROM meta.fields WHERE id = $1`, f.field)
				assertSQL(t, ctx, tx, `SELECT id = $1 AND display_field_id IS NULL FROM meta.tables WHERE id = $1`, f.table)
				assertSQL(t, ctx, tx, `SELECT (SELECT count(*) = 0 FROM meta.attachments)
					AND (SELECT count(*) = 1 FROM meta.comments) AND (SELECT count(*) = 1 FROM meta.row_history)`)
			case "relation field":
				execSQL(t, ctx, tx, `DELETE FROM meta.fields WHERE id = $1`, f.parentLink)
				assertSQL(t, ctx, tx, `SELECT (SELECT count(*) = 0 FROM meta.relations)
					AND (SELECT count(*) = 1 FROM meta.fields WHERE id = $1)`, f.childLink)
			case "table", "book", "workspace":
				switch target {
				case "table":
					execSQL(t, ctx, tx, `DELETE FROM meta.tables WHERE id = $1`, f.table)
				case "book":
					execSQL(t, ctx, tx, `DELETE FROM meta.books WHERE id = $1`, f.book)
				case "workspace":
					execSQL(t, ctx, tx, `DELETE FROM meta.workspaces WHERE id = $1`, f.workspace)
				}
				assertSQL(t, ctx, tx, `SELECT (SELECT count(*) = 0 FROM meta.fields WHERE table_id = $1)
					AND (SELECT count(*) = 0 FROM meta.views) AND (SELECT count(*) = 0 FROM meta.relations)
					AND (SELECT count(*) = 0 FROM meta.comments) AND (SELECT count(*) = 0 FROM meta.row_history)
					AND (SELECT count(*) = 0 FROM meta.attachments) AND (SELECT count(*) = 1 FROM meta.backups)
					AND (SELECT count(*) = 1 FROM meta.grants)`, f.table)
				if target == "workspace" {
					assertSQL(t, ctx, tx, `SELECT (SELECT count(*) = 0 FROM meta.books) AND (SELECT count(*) = 0 FROM meta.tables)`)
				} else {
					assertSQL(t, ctx, tx, `SELECT count(*) = 1 FROM meta.tables WHERE id = $1`, f.otherTable)
				}
			}
		})
	}
}
