package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"pandatable/internal/db"
	"pandatable/migrations"
)

func TestWorkspaceDetailsMigrationUpDownPostgreSQL(t *testing.T) {
	pool := database.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	migrationDB := stdlib.OpenDB(*pool.Config().ConnConfig.Copy())
	t.Cleanup(func() {
		if err := migrationDB.Close(); err != nil {
			t.Error(err)
		}
	})
	provider, err := goose.NewProvider(goose.DialectPostgres, migrationDB, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	var userID, workspaceID string
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		if _, err := provider.Up(cleanup); err != nil {
			t.Error(err)
		}
		if workspaceID != "" {
			if _, err := pool.Exec(cleanup, `DELETE FROM meta.workspaces WHERE id=$1`, workspaceID); err != nil {
				t.Error(err)
			}
		}
		if userID != "" {
			if _, err := pool.Exec(cleanup, `DELETE FROM meta.users WHERE id=$1`, userID); err != nil {
				t.Error(err)
			}
		}
	})
	if _, err := provider.DownTo(ctx, 8); err != nil {
		t.Fatal(err)
	}

	const username = "ds01-workspace-details-upgrade"
	if err := pool.QueryRow(ctx, `INSERT INTO meta.users (username,display_name,password_hash,role,avatar_color)
		VALUES ($1,'Upgrade user','test-only','user','blue') RETURNING id::text`, username).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO meta.workspaces (name,icon,created_by)
		VALUES ('Legacy free-text icon','legacy-icon',$1) RETURNING id::text`, userID).Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}

	if err := db.RunMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var icon *string
	var description, fallback string
	if err := pool.QueryRow(ctx, `SELECT icon,description,fallback_icon FROM meta.workspaces WHERE id=$1`,
		workspaceID).Scan(&icon, &description, &fallback); err != nil {
		t.Fatal(err)
	}
	if icon != nil || description != "" || !isWorkspaceFallbackIcon(fallback) {
		t.Fatalf("upgraded workspace icon=%v description=%q fallback=%q", icon, description, fallback)
	}
	if _, err := pool.Exec(ctx, `UPDATE meta.workspaces SET icon='legacy-icon' WHERE id=$1`, workspaceID); err == nil {
		t.Fatal("workspace icon constraint accepted a legacy free-text icon")
	}

	if _, err := provider.DownTo(ctx, 8); err != nil {
		t.Fatal(err)
	}
	var detailsColumns int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema='meta' AND table_name='workspaces'
		  AND column_name IN ('description','fallback_icon')`).Scan(&detailsColumns); err != nil {
		t.Fatal(err)
	}
	if detailsColumns != 0 {
		t.Fatalf("down migration left %d workspace detail columns", detailsColumns)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT fallback_icon FROM meta.workspaces WHERE id=$1`, workspaceID).Scan(&fallback); err != nil {
		t.Fatal(err)
	}
	if !isWorkspaceFallbackIcon(fallback) {
		t.Fatalf("re-upgraded fallback icon %q is invalid", fallback)
	}
}

func isWorkspaceFallbackIcon(icon string) bool {
	switch icon {
	case "book", "table", "view", "fields", "calendar", "star", "group", "link":
		return true
	default:
		return false
	}
}
