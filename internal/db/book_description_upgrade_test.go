package db_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"pandatable/migrations"
)

func TestBookDescriptionMigrationUpDownPostgreSQL(t *testing.T) {
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
	var workspaceID, otherWorkspaceID string
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		if _, err := provider.Up(cleanup); err != nil {
			t.Error(err)
		}
		if _, err := pool.Exec(cleanup, `DELETE FROM meta.workspaces
			WHERE id::text = ANY($1::text[]) OR name = 'wmb01 new default'`,
			[]string{workspaceID, otherWorkspaceID}); err != nil {
			t.Error(err)
		}
	})
	if _, err := provider.DownTo(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO meta.workspaces (name,icon,fallback_icon)
		VALUES ('WMB link','link','link') RETURNING id::text`).Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO meta.workspaces (name,icon,fallback_icon)
		VALUES ('WMB stable','link','star') RETURNING id::text`).Scan(&otherWorkspaceID); err != nil {
		t.Fatal(err)
	}
	var bookID, otherBookID string
	if err := pool.QueryRow(ctx, `INSERT INTO meta.books (workspace_id,name,pg_schema,position,icon,fallback_icon)
		VALUES ($1,'WMB link','b_wmb_link',1,'link','link') RETURNING id::text`, workspaceID).Scan(&bookID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO meta.books (workspace_id,name,pg_schema,position,icon,fallback_icon)
		VALUES ($1,'WMB stable','b_wmb_stable',2,'link','star') RETURNING id::text`, workspaceID).Scan(&otherBookID); err != nil {
		t.Fatal(err)
	}

	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	var workspaceFallback, bookFallback, otherWorkspaceFallback, otherBookFallback string
	var workspaceIcon, bookIcon, description string
	if err := pool.QueryRow(ctx, `SELECT icon,fallback_icon FROM meta.workspaces WHERE id=$1`, workspaceID).
		Scan(&workspaceIcon, &workspaceFallback); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT icon,fallback_icon,description FROM meta.books WHERE id=$1`, bookID).
		Scan(&bookIcon, &bookFallback, &description); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT fallback_icon FROM meta.workspaces WHERE id=$1`, otherWorkspaceID).
		Scan(&otherWorkspaceFallback); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT fallback_icon FROM meta.books WHERE id=$1`, otherBookID).
		Scan(&otherBookFallback); err != nil {
		t.Fatal(err)
	}
	if workspaceIcon != "link" || bookIcon != "link" || description != "" ||
		!isFallbackIcon(workspaceFallback) || !isFallbackIcon(bookFallback) ||
		otherWorkspaceFallback != "star" || otherBookFallback != "star" {
		t.Fatalf("upgrade changed custom or stable icons: workspace=%s/%s book=%s/%s description=%q stable=%s/%s",
			workspaceIcon, workspaceFallback, bookIcon, bookFallback, description, otherWorkspaceFallback, otherBookFallback)
	}

	assertNoLinkFallbacks := func() {
		t.Helper()
		var count int
		if err := pool.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM meta.workspaces WHERE fallback_icon='link') +
			(SELECT count(*) FROM meta.books WHERE fallback_icon='link')`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%d link fallbacks remain", count)
		}
		for _, table := range []string{"workspaces", "books"} {
			var defaultValue string
			if err := pool.QueryRow(ctx, `SELECT column_default FROM information_schema.columns
				WHERE table_schema='meta' AND table_name=$1 AND column_name='fallback_icon'`, table).
				Scan(&defaultValue); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(defaultValue, "link") || !strings.Contains(defaultValue, "random()") {
				t.Fatalf("%s fallback default = %s", table, defaultValue)
			}
		}
	}
	assertNoLinkFallbacks()
	for _, update := range []struct {
		table, id, query string
	}{
		{"workspaces", workspaceID, "UPDATE meta.workspaces SET fallback_icon='link' WHERE id=$1"},
		{"books", bookID, "UPDATE meta.books SET fallback_icon='link' WHERE id=$1"},
	} {
		_, err := pool.Exec(ctx, update.query, update.id)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
			t.Fatalf("%s accepted a link fallback: %v", update.table, err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO meta.workspaces (name)
		SELECT 'wmb01 new default' FROM generate_series(1,128)`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO meta.books (workspace_id,name,pg_schema,position)
		SELECT $1, 'Default book', 'b_wmb_default_' || n::text, n FROM generate_series(1,128) AS n`, workspaceID); err != nil {
		t.Fatal(err)
	}
	assertNoLinkFallbacks()

	if _, err := provider.DownTo(ctx, 10); err != nil {
		t.Fatal(err)
	}
	var columnCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema='meta' AND table_name='books' AND column_name='description'`).Scan(&columnCount); err != nil {
		t.Fatal(err)
	}
	if columnCount != 0 {
		t.Fatalf("down migration retained %d description columns", columnCount)
	}
	var downWorkspace, downBook string
	if err := pool.QueryRow(ctx, `SELECT fallback_icon FROM meta.workspaces WHERE id=$1`, workspaceID).Scan(&downWorkspace); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT fallback_icon FROM meta.books WHERE id=$1`, bookID).Scan(&downBook); err != nil {
		t.Fatal(err)
	}
	if downWorkspace != workspaceFallback || downBook != bookFallback {
		t.Fatalf("down migration changed remapped fallbacks: %q/%q", downWorkspace, downBook)
	}
	for _, update := range []struct {
		table, id, query string
	}{
		{"workspaces", otherWorkspaceID, "UPDATE meta.workspaces SET fallback_icon='link' WHERE id=$1"},
		{"books", otherBookID, "UPDATE meta.books SET fallback_icon='link' WHERE id=$1"},
	} {
		if _, err := pool.Exec(ctx, update.query, update.id); err != nil {
			t.Fatalf("down migration did not restore %s constraint: %v", update.table, err)
		}
		var defaultValue string
		if err := pool.QueryRow(ctx, `SELECT column_default FROM information_schema.columns
			WHERE table_schema='meta' AND table_name=$1 AND column_name='fallback_icon'`, update.table).
			Scan(&defaultValue); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(defaultValue, "link") {
			t.Fatalf("down migration did not restore %s default: %s", update.table, defaultValue)
		}
	}

	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	assertNoLinkFallbacks()
	if err := pool.QueryRow(ctx, `SELECT fallback_icon FROM meta.workspaces WHERE id=$1`, workspaceID).Scan(&downWorkspace); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT fallback_icon,description FROM meta.books WHERE id=$1`, bookID).
		Scan(&downBook, &description); err != nil {
		t.Fatal(err)
	}
	if downWorkspace != workspaceFallback || downBook != bookFallback || description != "" {
		t.Fatalf("up/down/up changed stable fallbacks or default description: %q/%q %q", downWorkspace, downBook, description)
	}
}
