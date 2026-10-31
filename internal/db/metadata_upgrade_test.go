package db_test

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"pandatable/internal/db"
	"pandatable/migrations"
)

func TestMetadataUpgradeRollbackPostgreSQL(t *testing.T) {
	// This pool is Suite-owned, never the database named by the developer's URL.
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
	t.Cleanup(func() {
		restoreCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := provider.Up(restoreCtx); err != nil {
			t.Errorf("restore latest schema in owned database: %v", err)
		}
		if _, err := pool.Exec(restoreCtx, `DROP TABLE IF EXISTS meta.t11_unrelated`); err != nil {
			t.Error(err)
		}
		if _, err := pool.Exec(restoreCtx, `DELETE FROM meta.users WHERE username IN ('t11_actor', 't11_sentinel');
			DELETE FROM meta.settings WHERE key = 't11_sentinel'`); err != nil {
			t.Error(err)
		}
	})
	if _, err := provider.DownTo(ctx, 1); err != nil {
		t.Fatalf("prepare 0001-only database: %v", err)
	}
	execSQL(t, ctx, pool, `
		INSERT INTO meta.users
		  (username, email, display_name, password_hash, role, status, must_change_password,
		   locale, theme, timezone, avatar_color, last_login_at, created_at, updated_at)
		VALUES ('t11_sentinel', 'sentinel@example.invalid', 'Sentinel', 'test-only-phc', 'admin',
		        'disabled', true, 'zh-CN', 'dark', 'Europe/London', '#123456',
		        '2026-01-01 01:00:00+00', '2026-01-01 00:00:00+00', '2026-01-01 02:00:00+00');
		INSERT INTO meta.sessions (id_hash, user_id, created_at, last_seen_at, expires_at, ip, user_agent)
		SELECT decode(repeat('ab', 32), 'hex'), id, '2026-01-01 00:00:00+00',
		       '2026-01-01 01:00:00+00', '2027-01-01 00:00:00+00', '192.0.2.1', 'test sentinel'
		FROM meta.users WHERE username = 't11_sentinel';
		INSERT INTO meta.settings (key, value) VALUES
		  ('t11_sentinel', '{"nested":{"keep":[1,true,null]},"timezone":"Asia/Shanghai"}');
		CREATE TABLE meta.t11_unrelated (value text NOT NULL);
		INSERT INTO meta.t11_unrelated VALUES ('not owned by migration 0002');
	`)
	const snapshotSQL = `SELECT jsonb_build_object(
		'users', (SELECT jsonb_agg(to_jsonb(u) ORDER BY id) FROM meta.users u),
		'sessions', (SELECT jsonb_agg(to_jsonb(s) ORDER BY id_hash) FROM meta.sessions s),
		'settings', (SELECT jsonb_agg(to_jsonb(s) ORDER BY key) FROM meta.settings s),
		'version1', (SELECT jsonb_agg(to_jsonb(v) ORDER BY id) FROM public.goose_db_version v WHERE version_id = 1),
		'extensions', (SELECT jsonb_agg(jsonb_build_array(oid, extname, extversion) ORDER BY extname)
		               FROM pg_extension WHERE extname IN ('pg_trgm','pg_stat_statements')),
		'unrelated', (SELECT jsonb_agg(value) FROM meta.t11_unrelated)
	)::text`
	var before string
	if err := pool.QueryRow(ctx, snapshotSQL).Scan(&before); err != nil {
		t.Fatal(err)
	}
	for cycle := range 2 {
		results := make(chan error, 2)
		for range 2 {
			go func() { results <- db.RunMigrations(ctx, pool) }()
		}
		for range 2 {
			if err := <-results; err != nil {
				t.Fatalf("cycle %d concurrent upgrade: %v", cycle+1, err)
			}
		}
		beforeRerun := migrationRecords(t, ctx, migrationDB)
		if err := db.RunMigrations(ctx, pool); err != nil {
			t.Fatal(err)
		}
		if afterRerun := migrationRecords(t, ctx, migrationDB); beforeRerun != afterRerun {
			t.Fatal("repeated migration changed version records")
		}
		assertSQL(t, ctx, pool, `SELECT count(*) = 4 AND count(*) FILTER (WHERE version_id = 1 AND is_applied) = 1
			AND count(*) FILTER (WHERE version_id = 2 AND is_applied) = 1
			AND count(*) FILTER (WHERE version_id = 3 AND is_applied) = 1 FROM public.goose_db_version`)
		var after string
		if err := pool.QueryRow(ctx, snapshotSQL).Scan(&after); err != nil || before != after {
			t.Fatalf("cycle %d upgrade changed 0001 values/record/extensions/unrelated object (error=%v)", cycle+1, err)
		}
		// Populate dependent objects and the display-field cycle before exercising Down.
		tx := metadataTx(t, ctx, pool)
		seedMetadata(t, ctx, tx)
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		assertSQL(t, ctx, pool, `SELECT count(*) = 1 AND bool_and(restore_system IS NULL) FROM meta.row_history`)
		if _, err := provider.DownTo(ctx, 2); err != nil {
			t.Fatalf("cycle %d rollback to 0002: %v", cycle+1, err)
		}
		assertSQL(t, ctx, pool, `SELECT count(*) = 1 FROM meta.row_history`)
		if _, err := provider.Up(ctx); err != nil {
			t.Fatalf("cycle %d reapply 0003: %v", cycle+1, err)
		}
		assertSQL(t, ctx, pool, `SELECT count(*) = 1 AND bool_and(restore_system IS NULL) FROM meta.row_history`)
		if _, err := provider.DownTo(ctx, 1); err != nil {
			t.Fatalf("cycle %d rollback to 0001: %v", cycle+1, err)
		}
		var tables []string
		if err := pool.QueryRow(ctx, `SELECT array_agg(tablename::text ORDER BY tablename)
			FROM pg_tables WHERE schemaname = 'meta'`).Scan(&tables); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(tables, []string{"sessions", "settings", "t11_unrelated", "users"}) {
			t.Fatalf("0002 rollback left unexpected tables: %v", tables)
		}
		assertSQL(t, ctx, pool, `SELECT to_regclass('meta.row_history_id_seq') IS NULL`)
		// seedMetadata adds one user/session; remove only those owned fixture rows.
		execSQL(t, ctx, pool, `DELETE FROM meta.users WHERE username = 't11_actor'`)
		if err := pool.QueryRow(ctx, snapshotSQL).Scan(&after); err != nil || before != after {
			t.Fatalf("cycle %d rollback changed 0001 sentinel data (error=%v)", cycle+1, err)
		}
		t.Logf("cycle %d: 0001 upgrade, concurrent/repeated apply once, populated Down, sentinel data/extension OIDs/version1 unchanged", cycle+1)
	}
	if err := db.RunMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}
}

func migrationRecords(t *testing.T, ctx context.Context, database *sql.DB) string {
	t.Helper()
	var records string
	if err := database.QueryRowContext(ctx, `SELECT jsonb_agg(to_jsonb(v) ORDER BY id)::text
		FROM public.goose_db_version v`).Scan(&records); err != nil {
		t.Fatal(err)
	}
	return records
}
