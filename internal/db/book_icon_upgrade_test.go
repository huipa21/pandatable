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

func TestBookIconMigrationBackfillsStableFallbacksPostgreSQL(t *testing.T) {
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
			if _, err := pool.Exec(cleanup, `DELETE FROM meta.grants WHERE resource_id=$1`, workspaceID); err != nil {
				t.Error(err)
			}
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
	if _, err := provider.DownTo(ctx, 7); err != nil {
		t.Fatal(err)
	}

	const username = "ux03-book-icon-upgrade"
	if err := pool.QueryRow(ctx, `INSERT INTO meta.users (username,display_name,password_hash,role,avatar_color)
		VALUES ($1,'Upgrade user','test-only','user','blue') RETURNING id::text`, username).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO meta.workspaces (name,created_by)
		VALUES ('Existing icon workspace',$1) RETURNING id::text`, userID).Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO meta.grants (resource_type,resource_id,principal_type,principal_id,level)
		VALUES ('workspace',$1,'user',$2,'owner')`, workspaceID, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO meta.books (workspace_id,name,pg_schema,position,created_by)
		VALUES ($1,'Legacy one','b_aaaaaaaaaa',1024,$2),($1,'Legacy two','b_bbbbbbbbbb',2048,$2)`, workspaceID, userID); err != nil {
		t.Fatal(err)
	}
	const snapshot = `SELECT jsonb_build_object(
		'books',(SELECT jsonb_agg(jsonb_build_object('id',id,'name',name,'pg_schema',pg_schema,'position',position) ORDER BY id)
			FROM meta.books WHERE workspace_id=$1),
		'grants',(SELECT jsonb_agg(to_jsonb(g) ORDER BY g.id) FROM meta.grants g WHERE resource_id=$1))::text`
	var before, after string
	if err := pool.QueryRow(ctx, snapshot, workspaceID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := db.RunMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}
	assertLegacyBooksHaveFallbacks := func() map[string]string {
		t.Helper()
		rows, err := pool.Query(ctx, `SELECT name,fallback_icon FROM meta.books WHERE workspace_id=$1 ORDER BY name`, workspaceID)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		result := make(map[string]string)
		for rows.Next() {
			var name, icon string
			if err := rows.Scan(&name, &icon); err != nil {
				t.Fatal(err)
			}
			result[name] = icon
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if len(result) != 2 {
			t.Fatalf("backfilled %d books, want 2", len(result))
		}
		for name, icon := range result {
			if !isFallbackIcon(icon) {
				t.Errorf("%s has invalid fallback icon %q", name, icon)
			}
		}
		return result
	}
	first := assertLegacyBooksHaveFallbacks()
	if err := db.RunMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}
	second := assertLegacyBooksHaveFallbacks()
	if first["Legacy one"] != second["Legacy one"] || first["Legacy two"] != second["Legacy two"] {
		t.Fatalf("migration rerun changed fallbacks: before=%v after=%v", first, second)
	}
	if err := pool.QueryRow(ctx, snapshot, workspaceID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("migration changed existing name/order/schema/grants: before=%s after=%s", before, after)
	}
	var invalidFallbacks int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM meta.books
		WHERE workspace_id=$1 AND fallback_icon NOT IN ('book','table','view','fields','calendar','star','group')`,
		workspaceID).Scan(&invalidFallbacks); err != nil {
		t.Fatal(err)
	}
	if invalidFallbacks != 0 {
		t.Fatalf("migration assigned %d fallback icons outside the local palette", invalidFallbacks)
	}

}

func isFallbackIcon(icon string) bool {
	switch icon {
	case "book", "table", "view", "fields", "calendar", "star", "group":
		return true
	default:
		return false
	}
}
