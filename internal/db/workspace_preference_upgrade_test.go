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

func TestWorkspacePreferenceMigrationUpgradePostgreSQL(t *testing.T) {
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
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		if _, err := provider.Up(cleanup); err != nil {
			t.Error(err)
		}
		if _, err := pool.Exec(cleanup, `DELETE FROM meta.grants WHERE principal_id IN
			(SELECT id FROM meta.users WHERE username='ux01-upgrade');
			DELETE FROM meta.workspaces WHERE created_by IN
			(SELECT id FROM meta.users WHERE username='ux01-upgrade');
			DELETE FROM meta.users WHERE username='ux01-upgrade'`); err != nil {
			t.Error(err)
		}
	})
	if _, err := provider.DownTo(ctx, 6); err != nil {
		t.Fatal(err)
	}
	var user, workspace string
	if err := pool.QueryRow(ctx, `INSERT INTO meta.users (username,display_name,password_hash,role,avatar_color)
		VALUES ('ux01-upgrade','Existing user','test-only','user','blue') RETURNING id::text`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO meta.workspaces (name,created_by)
		VALUES ('Existing workspace',$1) RETURNING id::text`, user).Scan(&workspace); err != nil {
		t.Fatal(err)
	}
	execSQL(t, ctx, pool, `INSERT INTO meta.grants (resource_type,resource_id,principal_type,principal_id,level)
		VALUES ('workspace',$1,'user',$2,'owner')`, workspace, user)
	const snapshot = `SELECT jsonb_build_object(
		'user',(SELECT to_jsonb(u) FROM meta.users u WHERE id=$1),
		'workspace',(SELECT jsonb_build_object(
			'id',w.id,'name',w.name,'icon',w.icon,'color',w.color,'created_by',w.created_by,
			'created_at',w.created_at,'deleted_at',w.deleted_at,'deleted_by',w.deleted_by
		) FROM meta.workspaces w WHERE id=$2),
		'grant',(SELECT to_jsonb(g) FROM meta.grants g WHERE resource_id=$2))::text`
	var before, after string
	if err := pool.QueryRow(ctx, snapshot, user, workspace).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := db.RunMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := db.RunMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, snapshot, user, workspace).Scan(&after); err != nil || after != before {
		t.Fatalf("migration altered existing user data: equal=%t err=%v", before == after, err)
	}
	assertSQL(t, ctx, pool, `SELECT NOT EXISTS (SELECT 1 FROM meta.user_workspace_preferences WHERE user_id=$1)`, user)
	assertSQL(t, ctx, pool, `SELECT count(*)=1 FROM public.goose_db_version WHERE version_id=7 AND is_applied`)
	execSQL(t, ctx, pool, `INSERT INTO meta.user_workspace_preferences (user_id,workspace_id) VALUES ($1,$2)`, user, workspace)
	execSQL(t, ctx, pool, `DELETE FROM meta.workspaces WHERE id=$1`, workspace)
	assertSQL(t, ctx, pool, `SELECT workspace_id IS NULL FROM meta.user_workspace_preferences WHERE user_id=$1`, user)
	execSQL(t, ctx, pool, `DELETE FROM meta.grants WHERE principal_id=$1`, user)
	execSQL(t, ctx, pool, `DELETE FROM meta.users WHERE id=$1`, user)
	assertSQL(t, ctx, pool, `SELECT NOT EXISTS (SELECT 1 FROM meta.user_workspace_preferences WHERE user_id=$1)`, user)
}
