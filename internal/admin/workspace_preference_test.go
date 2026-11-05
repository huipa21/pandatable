package admin

import "testing"

func TestAdminDefaultWorkspaceProvisioningRollback(t *testing.T) {
	f := newFixture(t)
	f.exec(f.ctx, `CREATE FUNCTION meta.ux01_reject_admin_preference() RETURNS trigger LANGUAGE plpgsql AS
		$$ BEGIN RAISE EXCEPTION 'forced user workspace failure'; END $$`)
	f.exec(f.ctx, `CREATE TRIGGER ux01_reject_admin_preference BEFORE INSERT ON meta.user_workspace_preferences
		FOR EACH ROW EXECUTE FUNCTION meta.ux01_reject_admin_preference()`)
	t.Cleanup(func() { f.exec(f.ctx, "DROP FUNCTION IF EXISTS meta.ux01_reject_admin_preference() CASCADE") })
	_, err := f.service.Create(f.ctx, f.token, CreateInput{
		Username: "ux01-rollback", DisplayName: "Rollback", Role: "user", Password: password,
	})
	if err == nil {
		t.Fatal("account creation succeeded after workspace preference failure")
	}
	var users, workspaces, grants, preferences int
	if err := f.pool.QueryRow(f.ctx, `SELECT
		(SELECT count(*) FROM meta.users WHERE username='ux01-rollback'),
		(SELECT count(*) FROM meta.workspaces),
		(SELECT count(*) FROM meta.grants),
		(SELECT count(*) FROM meta.user_workspace_preferences)`).
		Scan(&users, &workspaces, &grants, &preferences); err != nil {
		t.Fatal(err)
	}
	if users != 0 || workspaces != 0 || grants != 0 || preferences != 0 {
		t.Fatalf("failed creation left users=%d workspaces=%d grants=%d preferences=%d", users, workspaces, grants, preferences)
	}
}
