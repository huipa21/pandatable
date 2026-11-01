package backup

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"pandatable/internal/acl"
	"pandatable/internal/attachments"
	"pandatable/internal/structure"
)

func TestBackupRestorePostgres18(t *testing.T) {
	for _, tool := range []string{"pg_dump", "pg_restore", "psql"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("PostgreSQL 18 client %s is unavailable; run this test in the PostgreSQL client image", tool)
		}
	}
	f := newBackupFixture(t)
	if err := backupDatabase.CheckPoolOwnership(f.ctx, f.pool); err != nil {
		t.Fatal(err)
	}
	resolver, err := acl.NewResolver(f.pool)
	if err != nil {
		t.Fatal(err)
	}
	service, err := structure.New(f.pool, resolver)
	if err != nil {
		t.Fatal(err)
	}
	actor := structure.Actor{ID: f.userID, Locale: "en", Timezone: "UTC"}
	workspace, err := service.CreateWorkspace(f.ctx, actor, structure.WorkspaceInput{Name: "Original workspace"})
	if err != nil {
		t.Fatal(err)
	}
	book, err := service.CreateBook(f.ctx, actor, workspace.ID, "Original book")
	if err != nil {
		t.Fatal(err)
	}
	table, err := service.CreateTable(f.ctx, actor, book.ID, "Original table")
	if err != nil {
		t.Fatal(err)
	}
	field, err := service.CreateField(f.ctx, actor, table.Table.ID, structure.FieldInput{Name: "Files", UIType: "attachment"})
	if err != nil || field.Field == nil {
		t.Fatalf("create attachment field: %v", err)
	}
	attachmentService, err := attachments.New(f.pool, resolver, f.dir)
	if err != nil {
		t.Fatal(err)
	}
	attachment, err := attachmentService.Upload(f.ctx, f.userID, table.Table.ID, field.Field.ID,
		"proof.txt", strings.NewReader("original attachment"), 1<<20, 16)
	if err != nil {
		t.Fatal(err)
	}
	attachmentJSON, err := json.Marshal([]attachments.File{attachment})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.CreateRows(f.ctx, actor, table.Table.ID, []structure.RowInsert{{Fields: map[string]json.RawMessage{
		table.Fields[0].ID: json.RawMessage(`"original row"`), field.Field.ID: attachmentJSON,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	var schemaName, tableName, column, storagePath string
	if err := f.pool.QueryRow(f.ctx, `SELECT b.pg_schema, t.pg_name, f.pg_column
		FROM meta.tables t JOIN meta.books b ON b.id=t.book_id
		JOIN meta.fields f ON f.id=t.display_field_id WHERE t.id=$1`, table.Table.ID).
		Scan(&schemaName, &tableName, &column); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(f.ctx, "SELECT storage_path FROM meta.attachments WHERE id=$1", attachment.ID).
		Scan(&storagePath); err != nil {
		t.Fatal(err)
	}
	record, err := f.manager.Start(f.ctx, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	done := waitBackup(t, f, record.ID)
	if done.Status != "done" {
		t.Fatalf("real pg_dump backup failed: %+v", done)
	}
	if err := f.manager.Close(f.ctx); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(f.dir, "backups", record.FileName)
	qualified := pgx.Identifier{schemaName, tableName}.Sanitize()
	display := pgx.Identifier{column}.Sanitize()
	for _, statement := range []string{
		"UPDATE meta.users SET display_name='Changed owner' WHERE id=$1",
		"UPDATE meta.workspaces SET name='Changed workspace' WHERE id=$2",
		"UPDATE " + qualified + " SET " + display + "='changed row'",
		"CREATE SCHEMA b_restore_extra",
		"CREATE TABLE b_restore_extra.after_backup(value text)",
	} {
		var args []any
		switch {
		case strings.Contains(statement, "$1"):
			args = []any{f.userID}
		case strings.Contains(statement, "$2"):
			statement = strings.ReplaceAll(statement, "$2", "$1")
			args = []any{workspace.ID}
		}
		if _, err := f.pool.Exec(f.ctx, statement, args...); err != nil {
			t.Fatal(err)
		}
	}
	attachmentPath := filepath.Join(f.dir, filepath.FromSlash(storagePath))
	if err := os.WriteFile(attachmentPath, []byte("changed attachment"), 0o600); err != nil {
		t.Fatal(err)
	}
	extraFile := filepath.Join(f.dir, "files", "after-backup")
	if err := os.WriteFile(extraFile, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := RestoreOptions{Pool: f.pool, DatabaseURL: backupDatabase.URL(t), DataDir: f.dir,
		AppVersion: "0.0.0-dev", Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if runtime.GOOS != "windows" {
		restorePath, err := exec.LookPath("pg_restore")
		if err != nil {
			t.Fatal(err)
		}
		shimDir := t.TempDir()
		shim := "#!/bin/sh\nif [ \"$1\" = \"--list\" ]; then exec \"$PT_TEST_REAL_PG_RESTORE\" \"$@\"; fi\n" +
			"\"$PT_TEST_REAL_PG_RESTORE\" \"$@\" || exit $?\nprintf '\\nSELECT 1 / 0;\\n'\n"
		if err := os.WriteFile(filepath.Join(shimDir, "pg_restore"), []byte(shim), 0o700); err != nil {
			t.Fatal(err)
		}
		originalPath := os.Getenv("PATH")
		t.Setenv("PT_TEST_REAL_PG_RESTORE", restorePath)
		t.Setenv("PATH", shimDir+string(os.PathListSeparator)+originalPath)
		if err := Restore(f.ctx, archive, options); err == nil {
			t.Fatal("failing restore transaction reported success")
		}
		t.Setenv("PATH", originalPath)
		assertRestoredValues(t, f.ctx, f, workspace.ID, qualified, display, attachmentPath,
			"Changed owner", "Changed workspace", "changed row", "changed attachment", true)
		setBackupDatabaseIdleTimeout(t, f)
		shim = "#!/bin/sh\nif [ \"$1\" = \"--list\" ]; then exec \"$PT_TEST_REAL_PG_RESTORE\" \"$@\"; fi\n" +
			"sleep 2\nexec \"$PT_TEST_REAL_PG_RESTORE\" \"$@\"\n"
		if err := os.WriteFile(filepath.Join(shimDir, "pg_restore"), []byte(shim), 0o700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", shimDir+string(os.PathListSeparator)+originalPath)
	}
	if cli := os.Getenv("PT_TEST_RESTORE_CLI"); cli != "" {
		t.Setenv("PT_DATABASE_URL", backupDatabase.URL(t))
		t.Setenv("PT_DATA_DIR", f.dir)
		t.Setenv("PT_BASE_URL", "http://127.0.0.1:8080")
		t.Setenv("PT_DB_CONNECT_RETRIES", "0")
		command := exec.CommandContext(f.ctx, cli, "restore", archive, "--yes")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("restore valid backup with CLI: %v\n%s", err, output)
		}
		f.pool.Reset()
	} else if err := Restore(f.ctx, archive, options); err != nil {
		t.Fatalf("restore valid backup: %v", err)
	}

	assertRestoredValues(t, f.ctx, f, workspace.ID, qualified, display, attachmentPath,
		"Backup owner", "Original workspace", "original row", "original attachment", false)
	if _, err := os.Stat(extraFile); !os.IsNotExist(err) {
		t.Fatalf("post-backup attachment survived restore: %v", err)
	}
	var count int
	if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM meta.backups WHERE id=$1 AND status='done'", record.ID).
		Scan(&count); err != nil || count != 1 {
		t.Fatalf("local backup catalog not retained: count=%d err=%v", count, err)
	}
}

func assertRestoredValues(t *testing.T, ctx context.Context, f *backupFixture, workspaceID, table, column, file,
	userWant, workspaceWant, rowWant, fileWant string, extraSchema bool) {
	t.Helper()
	var userName, workspaceName, rowValue string
	var extraExists bool
	if err := f.pool.QueryRow(ctx, "SELECT display_name FROM meta.users WHERE id=$1", f.userID).Scan(&userName); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, "SELECT name FROM meta.workspaces WHERE id=$1", workspaceID).Scan(&workspaceName); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, "SELECT "+column+" FROM "+table+" LIMIT 1").Scan(&rowValue); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, "SELECT to_regnamespace('b_restore_extra') IS NOT NULL").Scan(&extraExists); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(file)
	if err != nil || userName != userWant || workspaceName != workspaceWant || rowValue != rowWant ||
		string(content) != fileWant || extraExists != extraSchema {
		t.Fatalf("restored values user=%q workspace=%q row=%q file=%q extraSchema=%v err=%v",
			userName, workspaceName, rowValue, content, extraExists, err)
	}
}
