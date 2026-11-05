package db_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"pandatable/internal/db"
	"pandatable/internal/testdb"
	"pandatable/migrations"
)

var database testdb.Suite

func TestMain(m *testing.M) {
	os.Exit(database.Run(m))
}

func TestEmbeddedMigrationOrdering(t *testing.T) {
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	if want := []string{"0001_initial.sql", "0002_metadata.sql", "0003_restore_system.sql", "0004_trash.sql", "0005_user_lifecycle.sql", "0006_mail.sql", "0007_workspace_preferences.sql", "0008_book_icons.sql"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("embedded migration files = %v, want %v", names, want)
	}
}

func TestInitialMigrationPostgreSQL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := database.Open(t)

	results := make(chan error, 2)
	for range 2 {
		go func() {
			results <- db.RunMigrations(ctx, pool)
		}()
	}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("concurrent migration run failed: %v", err)
		}
	}
	if err := db.RunMigrations(ctx, pool); err != nil {
		t.Fatalf("rerun migrations failed: %v", err)
	}

	var users, sessions, settings string
	if err := pool.QueryRow(ctx, `
		SELECT to_regclass('meta.users')::text,
		       to_regclass('meta.sessions')::text,
		       to_regclass('meta.settings')::text`).Scan(&users, &sessions, &settings); err != nil {
		t.Fatal(err)
	}
	if users != "meta.users" || sessions != "meta.sessions" || settings != "meta.settings" {
		t.Fatalf("initial tables: users=%q sessions=%q settings=%q", users, sessions, settings)
	}

	expectedColumns := map[string][]string{
		"users":    {"id", "username", "email", "display_name", "password_hash", "role", "status", "must_change_password", "locale", "theme", "timezone", "avatar_color", "last_login_at", "created_at", "updated_at", "mention_email"},
		"sessions": {"id_hash", "user_id", "created_at", "last_seen_at", "expires_at", "ip", "user_agent"},
		"settings": {"key", "value"},
	}
	rows, err := pool.Query(ctx, `
		SELECT table_name, column_name
		FROM information_schema.columns
		WHERE table_schema = 'meta'
		  AND table_name IN ('users', 'sessions', 'settings')`)
	if err != nil {
		t.Fatal(err)
	}
	actualColumns := make(map[string]map[string]bool)
	for rows.Next() {
		var tableName, columnName string
		if err := rows.Scan(&tableName, &columnName); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		if actualColumns[tableName] == nil {
			actualColumns[tableName] = make(map[string]bool)
		}
		actualColumns[tableName][columnName] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	rows.Close()
	for tableName, columns := range expectedColumns {
		for _, columnName := range columns {
			if !actualColumns[tableName][columnName] {
				t.Errorf("meta.%s is missing column %q", tableName, columnName)
			}
		}
	}

	var extensionCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_extension WHERE extname IN ('pg_trgm', 'pg_stat_statements')`).Scan(&extensionCount); err != nil {
		t.Fatal(err)
	}
	if extensionCount != 2 {
		t.Errorf("installed extensions = %d, want 2", extensionCount)
	}

	var appliedCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM public.goose_db_version WHERE version_id = 1 AND is_applied`).Scan(&appliedCount); err != nil {
		t.Fatal(err)
	}
	if appliedCount != 1 {
		t.Errorf("successful initial migration records = %d, want 1", appliedCount)
	}

	var transactionID [16]byte
	if _, err := rand.Read(transactionID[:]); err != nil {
		t.Fatal(err)
	}
	suffix := hex.EncodeToString(transactionID[:])
	committedKey := "t03_transaction_commit_" + suffix
	if err := pool.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO meta.settings (key, value) VALUES ($1, 'true')`, committedKey)
		return err
	}); err != nil {
		t.Fatalf("transaction commit failed: %v", err)
	}
	var committedCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM meta.settings WHERE key = $1`, committedKey).Scan(&committedCount); err != nil {
		t.Fatal(err)
	}
	if committedCount != 1 {
		t.Fatalf("committed setting count = %d, want 1", committedCount)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM meta.settings WHERE key = $1`, committedKey); err != nil {
		t.Fatalf("remove committed test setting: %v", err)
	}

	rolledBackKey := "t03_transaction_rollback_" + suffix
	callbackErr := errors.New("force rollback")
	if err := pool.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO meta.settings (key, value) VALUES ($1, 'true')`, rolledBackKey); err != nil {
			return err
		}
		return callbackErr
	}); !errors.Is(err, callbackErr) {
		t.Fatalf("transaction callback error = %v, want original callback error", err)
	}
	var rolledBackCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM meta.settings WHERE key = $1`, rolledBackKey).Scan(&rolledBackCount); err != nil {
		t.Fatal(err)
	}
	if rolledBackCount != 0 {
		t.Errorf("rolled back setting count = %d, want 0", rolledBackCount)
	}
}
