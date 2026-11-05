package structure

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"pandatable/internal/acl"
	"pandatable/internal/structure/structuresql"
)

func resolvePreference(t *testing.T, f *fixture, actor Actor) string {
	t.Helper()
	preference, err := f.service.ResolveWorkspacePreference(f.ctx, actor)
	if err != nil || preference.WorkspaceID == nil {
		t.Fatalf("resolve preference: %+v %v", preference, err)
	}
	return *preference.WorkspaceID
}

func assertWorkspacePreference(t *testing.T, f *fixture, actor Actor, want string) {
	t.Helper()
	preference, err := f.service.WorkspacePreference(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	if want == "" {
		if preference.WorkspaceID != nil {
			t.Fatalf("preference = %+v, want null", preference)
		}
		return
	}
	if preference.WorkspaceID == nil || *preference.WorkspaceID != want {
		t.Fatalf("preference = %+v, want %s", preference, want)
	}
}

func parallelResolve(t *testing.T, f *fixture, actor Actor) string {
	t.Helper()
	start := make(chan struct{})
	results := make([]WorkspacePreference, 2)
	errs := make([]error, len(results))
	var workers sync.WaitGroup
	for index := range results {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			results[index], errs[index] = f.service.ResolveWorkspacePreference(f.ctx, actor)
		}()
	}
	close(start)
	workers.Wait()
	for index, err := range errs {
		if err != nil || results[index].WorkspaceID == nil {
			t.Fatalf("parallel resolution %d = %+v, %v", index, results[index], err)
		}
	}
	if *results[0].WorkspaceID != *results[1].WorkspaceID {
		t.Fatalf("parallel resolutions differ: %+v", results)
	}
	return *results[0].WorkspaceID
}

func TestWorkspacePreferenceBootstrapConcurrentRestartAndRename(t *testing.T) {
	f := newFixture(t)
	user := f.user("user", "active", "en")
	existing := f.workspace(user)
	assertWorkspacePreference(t, f, user, "")

	starter := parallelResolve(t, f, user)
	f.spaces = append(f.spaces, starter)
	if starter == existing.ID {
		t.Fatal("an existing workspace was reused instead of creating the starter")
	}
	if count := scanOne[int](f, "SELECT count(*) FROM meta.workspaces WHERE created_by=$1", user.ID); count != 2 {
		t.Fatalf("existing user has %d workspaces, want original and one starter", count)
	}
	if !scanOne[bool](f, `SELECT EXISTS (
		SELECT 1 FROM meta.workspaces w JOIN meta.grants g ON g.resource_id=w.id
		JOIN meta.user_workspace_preferences p ON p.workspace_id=w.id
		WHERE w.id=$1 AND w.name='Default' AND w.created_by=$2 AND w.deleted_at IS NULL
		  AND g.resource_type='workspace' AND g.principal_type='user' AND g.principal_id=$2
		  AND g.level='owner' AND p.user_id=$2)`, starter, user.ID) {
		t.Fatal("starter, direct owner and persisted preference are not consistent")
	}
	if name := scanOne[string](f, "SELECT name FROM meta.workspaces WHERE id=$1", existing.ID); name != existing.Name {
		t.Fatalf("original workspace changed: %q", name)
	}

	restarted, err := New(f.pool, f.resolver)
	if err != nil {
		t.Fatal(err)
	}
	f.service = restarted
	name := "Renamed starter"
	if _, err := f.service.UpdateWorkspace(f.ctx, user, starter, WorkspacePatch{Name: &name}); err != nil {
		t.Fatal(err)
	}
	if got := resolvePreference(t, f, user); got != starter {
		t.Fatalf("rename or restart recreated starter: %s", got)
	}
	if _, err := f.service.SetWorkspacePreference(f.ctx, user, existing.ID); err != nil {
		t.Fatal(err)
	}
	other := f.user("user", "active", "zh-CN")
	otherStarter := resolvePreference(t, f, other)
	f.spaces = append(f.spaces, otherStarter)
	f.service, err = New(f.pool, f.resolver)
	if err != nil {
		t.Fatal(err)
	}
	assertWorkspacePreference(t, f, user, existing.ID)
	assertWorkspacePreference(t, f, other, otherStarter)
	if got := resolvePreference(t, f, user); got != existing.ID {
		t.Fatalf("selection did not survive restart: %s", got)
	}
}

func TestWorkspacePreferenceAuthorizationFallbackAndReplacement(t *testing.T) {
	f := newFixture(t)
	owner, reader, admin := f.user("user", "active", "en"), f.user("user", "active", "en"), f.user("admin", "active", "en")
	shared := f.workspace(owner)
	starter := resolvePreference(t, f, reader)
	f.spaces = append(f.spaces, starter)
	alternate := f.workspace(reader)
	f.grant(acl.Workspace, shared.ID, "user", reader.ID, acl.Read)
	if _, err := f.service.SetWorkspacePreference(f.ctx, reader, shared.ID); err != nil {
		t.Fatalf("workspace reader could not select it: %v", err)
	}
	assertWorkspacePreference(t, f, reader, shared.ID)

	hidden := f.workspace(owner)
	pathOnly := f.workspace(owner)
	book := f.book(owner, pathOnly.ID)
	table := f.table(owner, book.ID)
	f.grant(acl.Table, table.Table.ID, "user", reader.ID, acl.Read)
	for _, test := range []struct {
		id     string
		status int
		code   string
	}{{"not-a-uuid", 404, "ERR_NOT_FOUND"}, {hidden.ID, 404, "ERR_NOT_FOUND"},
		{pathOnly.ID, 403, "ERR_PERMISSION_DENIED"}} {
		_, err := f.service.SetWorkspacePreference(f.ctx, reader, test.id)
		expectError(t, err, test.status, test.code)
	}
	_, err := f.service.SetWorkspacePreference(f.ctx, admin, shared.ID)
	expectError(t, err, 404, "ERR_NOT_FOUND")
	assertWorkspacePreference(t, f, admin, "")
	assertWorkspacePreference(t, f, reader, shared.ID)

	f.grant(acl.Workspace, hidden.ID, "user", reader.ID, acl.Read)
	if err := f.service.DeleteWorkspace(f.ctx, owner, hidden.ID); err != nil {
		t.Fatal(err)
	}
	_, err = f.service.SetWorkspacePreference(f.ctx, reader, hidden.ID)
	expectError(t, err, 404, "ERR_NOT_FOUND")

	f.exec("DELETE FROM meta.grants WHERE resource_type='workspace' AND resource_id=$1 AND principal_id=$2", shared.ID, reader.ID)
	assertWorkspacePreference(t, f, reader, "")
	if got := resolvePreference(t, f, reader); got != starter {
		t.Fatalf("fallback = %s, want earliest eligible %s", got, starter)
	}
	if err := f.service.DeleteWorkspace(f.ctx, reader, starter); err != nil {
		t.Fatal(err)
	}
	if got := resolvePreference(t, f, reader); got != alternate.ID {
		t.Fatalf("fallback after trash = %s, want %s", got, alternate.ID)
	}
	if err := f.service.DeleteWorkspace(f.ctx, reader, alternate.ID); err != nil {
		t.Fatal(err)
	}
	assertWorkspacePreference(t, f, reader, "")
	replacement := parallelResolve(t, f, reader)
	f.spaces = append(f.spaces, replacement)
	if replacement == starter || replacement == alternate.ID {
		t.Fatal("a deleted or path-only workspace was selected")
	}
	if got := scanOne[int](f, `SELECT count(*) FROM meta.workspaces WHERE created_by=$1 AND deleted_at IS NULL`, reader.ID); got != 1 {
		t.Fatalf("concurrent replacement created %d live workspaces", got)
	}
	if got := scanOne[string](f, "SELECT name FROM meta.workspaces WHERE id=$1", replacement); got != "Default" {
		t.Fatalf("replacement name = %q", got)
	}
}

func TestWorkspacePreferenceSetBeforeBootstrap(t *testing.T) {
	f := newFixture(t)
	owner, reader := f.user("user", "active", "en"), f.user("user", "active", "en")
	shared, hidden := f.workspace(owner), f.workspace(owner)
	f.grant(acl.Workspace, shared.ID, "user", reader.ID, acl.Read)
	_, err := f.service.SetWorkspacePreference(f.ctx, reader, hidden.ID)
	expectError(t, err, 404, "ERR_NOT_FOUND")
	if count := scanOne[int](f, `SELECT count(*) FROM meta.user_workspace_preferences WHERE user_id=$1`, reader.ID); count != 0 {
		t.Fatal("invalid selection initialized the preference")
	}
	result, err := f.service.SetWorkspacePreference(f.ctx, reader, shared.ID)
	if err != nil || result.WorkspaceID == nil || *result.WorkspaceID != shared.ID {
		t.Fatalf("valid initial selection = %+v %v", result, err)
	}
	starter := scanOne[string](f, "SELECT id::text FROM meta.workspaces WHERE created_by=$1", reader.ID)
	f.spaces = append(f.spaces, starter)
	if name := scanOne[string](f, "SELECT name FROM meta.workspaces WHERE id=$1", starter); name != "Default" {
		t.Fatalf("initial selection did not create starter: %q", name)
	}
	assertWorkspacePreference(t, f, reader, shared.ID)
	if got := resolvePreference(t, f, reader); got != shared.ID {
		t.Fatalf("initial selection was not retained: %s", got)
	}
}

func TestWorkspacePreferenceDisableRace(t *testing.T) {
	f := newFixture(t)
	user := f.user("user", "active", "en")
	disabling, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = disabling.Rollback(context.Background()) })
	if _, err := disabling.Exec(f.ctx, "UPDATE meta.users SET status='disabled' WHERE id=$1", user.ID); err != nil {
		t.Fatal(err)
	}
	wait := blocked(t, func() error {
		_, err := f.service.ResolveWorkspacePreference(f.ctx, user)
		return err
	})
	if err := disabling.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	expectError(t, wait(), 401, "ERR_UNAUTHENTICATED")
	if got := scanOne[int](f, `SELECT
		(SELECT count(*) FROM meta.workspaces WHERE created_by=$1) +
		(SELECT count(*) FROM meta.user_workspace_preferences WHERE user_id=$1)`, user.ID); got != 0 {
		t.Fatalf("disabled user received %d workspace/preference rows", got)
	}
}

func TestWorkspacePreferenceRevocationRaces(t *testing.T) {
	f := newFixture(t)
	owner, reader := f.user("user", "active", "en"), f.user("user", "active", "en")
	shared := f.workspace(owner)
	starter := resolvePreference(t, f, reader)
	f.spaces = append(f.spaces, starter)
	f.grant(acl.Workspace, shared.ID, "user", reader.ID, acl.Read)
	if _, err := f.service.SetWorkspacePreference(f.ctx, reader, shared.ID); err != nil {
		t.Fatal(err)
	}
	revoking := f.holdWorkspace(shared.ID)
	if _, err := revoking.Exec(f.ctx, "DELETE FROM meta.grants WHERE resource_id=$1 AND principal_id=$2", shared.ID, reader.ID); err != nil {
		t.Fatal(err)
	}
	var result WorkspacePreference
	wait := blocked(t, func() error {
		var err error
		result, err = f.service.ResolveWorkspacePreference(f.ctx, reader)
		return err
	})
	if err := revoking.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := wait(); err != nil || result.WorkspaceID == nil || *result.WorkspaceID != starter {
		t.Fatalf("resolve after revocation = %+v %v", result, err)
	}

	f.grant(acl.Workspace, shared.ID, "user", reader.ID, acl.Read)
	revoking = f.holdWorkspace(shared.ID)
	if _, err := revoking.Exec(f.ctx, "DELETE FROM meta.grants WHERE resource_id=$1 AND principal_id=$2", shared.ID, reader.ID); err != nil {
		t.Fatal(err)
	}
	wait = blocked(t, func() error {
		_, err := f.service.SetWorkspacePreference(f.ctx, reader, shared.ID)
		return err
	})
	if err := revoking.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	expectError(t, wait(), 404, "ERR_NOT_FOUND")
	assertWorkspacePreference(t, f, reader, starter)
}

func TestWorkspacePreferenceGroupDeletionRace(t *testing.T) {
	f := newFixture(t)
	owner, reader := f.user("user", "active", "en"), f.user("user", "active", "en")
	shared := f.workspace(owner)
	starter := resolvePreference(t, f, reader)
	f.spaces = append(f.spaces, starter)
	group := f.group()
	f.member(group, reader)
	f.grant(acl.Workspace, shared.ID, "group", group, acl.Read)
	if _, err := f.service.SetWorkspacePreference(f.ctx, reader, shared.ID); err != nil {
		t.Fatal(err)
	}
	deleting, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = deleting.Rollback(context.Background()) })
	if _, err := deleting.Exec(f.ctx, "DELETE FROM meta.groups WHERE id=$1", group); err != nil {
		t.Fatal(err)
	}
	if _, err := deleting.Exec(f.ctx, "DELETE FROM meta.grants WHERE principal_type='group' AND principal_id=$1", group); err != nil {
		t.Fatal(err)
	}
	var result WorkspacePreference
	wait := blocked(t, func() error {
		var err error
		result, err = f.service.ResolveWorkspacePreference(f.ctx, reader)
		return err
	})
	if err := deleting.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := wait(); err != nil || result.WorkspaceID == nil || *result.WorkspaceID != starter {
		t.Fatalf("resolve after group deletion = %+v %v", result, err)
	}
}

func TestWorkspacePreferenceLockContention(t *testing.T) {
	f := newFixture(t)
	user := f.user("user", "active", "en")
	userID, ok := acl.ParseUUID(user.ID)
	if !ok {
		t.Fatal("invalid fixture user ID")
	}
	lock, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Rollback(context.Background()) })
	if err := structuresql.New(lock).LockWorkspacePreference(f.ctx, userID); err != nil {
		t.Fatal(err)
	}
	_, err = f.service.ResolveWorkspacePreference(f.ctx, user)
	expectError(t, err, 409, "ERR_WORKSPACE_PREFERENCE_BUSY")
	if got := scanOne[int](f, `SELECT
		(SELECT count(*) FROM meta.workspaces WHERE created_by=$1) +
		(SELECT count(*) FROM meta.user_workspace_preferences WHERE user_id=$1)`, user.ID); got != 0 {
		t.Fatalf("contended resolution left %d workspace/preference rows", got)
	}
	if err := lock.Rollback(f.ctx); err != nil {
		t.Fatal(err)
	}
	starter := resolvePreference(t, f, user)
	f.spaces = append(f.spaces, starter)
}

func TestWorkspacePreferenceFailureLeavesNoPartialData(t *testing.T) {
	f := newFixture(t)
	user := f.user("user", "active", "en")
	f.exec(`CREATE FUNCTION meta.ux01_reject_preference() RETURNS trigger LANGUAGE plpgsql AS
		$$ BEGIN RAISE EXCEPTION 'forced preference failure'; END $$`)
	f.exec(`CREATE TRIGGER ux01_reject_preference BEFORE INSERT ON meta.user_workspace_preferences
		FOR EACH ROW EXECUTE FUNCTION meta.ux01_reject_preference()`)
	t.Cleanup(func() { f.exec("DROP FUNCTION IF EXISTS meta.ux01_reject_preference() CASCADE") })
	if _, err := f.service.ResolveWorkspacePreference(f.ctx, user); err == nil {
		t.Fatal("forced preference failure unexpectedly succeeded")
	}
	if got := scanOne[int](f, `SELECT
		(SELECT count(*) FROM meta.workspaces WHERE created_by=$1) +
		(SELECT count(*) FROM meta.grants WHERE principal_type='user' AND principal_id=$1) +
		(SELECT count(*) FROM meta.user_workspace_preferences WHERE user_id=$1)`, user.ID); got != 0 {
		t.Fatalf("failed transaction left %d workspace, grant or preference rows", got)
	}
	f.exec("DROP FUNCTION meta.ux01_reject_preference() CASCADE")
	f.exec(`CREATE FUNCTION meta.ux01_reject_preference_commit() RETURNS trigger LANGUAGE plpgsql AS
		$$ BEGIN RAISE EXCEPTION 'forced preference commit failure'; END $$`)
	f.exec(`CREATE CONSTRAINT TRIGGER ux01_reject_preference_commit AFTER INSERT ON meta.user_workspace_preferences
		DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION meta.ux01_reject_preference_commit()`)
	t.Cleanup(func() { f.exec("DROP FUNCTION IF EXISTS meta.ux01_reject_preference_commit() CASCADE") })
	epoch := f.resolver.Stats().Epoch
	if _, err := f.service.ResolveWorkspacePreference(f.ctx, user); err == nil {
		t.Fatal("forced preference commit failure unexpectedly succeeded")
	}
	if f.resolver.Stats().Epoch <= epoch {
		t.Fatal("uncertain workspace grant commit did not invalidate the ACL cache")
	}
	if got := scanOne[int](f, `SELECT
		(SELECT count(*) FROM meta.workspaces WHERE created_by=$1) +
		(SELECT count(*) FROM meta.grants WHERE principal_type='user' AND principal_id=$1) +
		(SELECT count(*) FROM meta.user_workspace_preferences WHERE user_id=$1)`, user.ID); got != 0 {
		t.Fatalf("failed commit left %d workspace, grant or preference rows", got)
	}
	f.exec("DROP FUNCTION meta.ux01_reject_preference_commit() CASCADE")
	starter := resolvePreference(t, f, user)
	f.spaces = append(f.spaces, starter)

	if err := f.service.DeleteWorkspace(f.ctx, user, starter); err != nil {
		t.Fatal(err)
	}
	f.exec("ALTER TABLE meta.grants RENAME TO ux01_test_grants")
	renamed := true
	t.Cleanup(func() {
		if renamed {
			f.exec("ALTER TABLE meta.ux01_test_grants RENAME TO grants")
		}
	})
	if _, err := f.service.ResolveWorkspacePreference(f.ctx, user); err == nil || errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("failed eligibility read was treated as an empty list: %v", err)
	}
	if got := scanOne[int](f, "SELECT count(*) FROM meta.workspaces WHERE created_by=$1", user.ID); got != 1 {
		t.Fatalf("failed read created another workspace: count=%d", got)
	}
	f.exec("ALTER TABLE meta.ux01_test_grants RENAME TO grants")
	renamed = false
}
