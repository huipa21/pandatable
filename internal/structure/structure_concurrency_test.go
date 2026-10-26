package structure

import (
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"pandatable/internal/acl"
)

// holdWorkspace opens a transaction that takes the grant-protocol workspace lock
// (FOR NO KEY UPDATE) and returns it uncommitted so a test can interleave.
func (f *fixture) holdWorkspace(workspace string) pgx.Tx {
	f.t.Helper()
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() { _ = tx.Rollback(f.ctx) })
	if _, err := tx.Exec(f.ctx, "SELECT id FROM meta.workspaces WHERE id=$1 FOR NO KEY UPDATE", workspace); err != nil {
		f.t.Fatal(err)
	}
	return tx
}

// blocked runs fn and fails unless it is still waiting after a short delay.
func blocked(t *testing.T, fn func() error) (wait func() error) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		t.Fatalf("operation completed instead of waiting for the competing lock: %v", err)
	case <-time.After(400 * time.Millisecond):
	}
	return func() error {
		select {
		case err := <-done:
			return err
		case <-time.After(15 * time.Second):
			t.Fatal("blocked operation never resumed")
			return nil
		}
	}
}

func TestGrantRevocationRacingStructureMutationIsRecheckedAfterLocking(t *testing.T) {
	f := newFixture(t)
	x := f.tree()
	bob := f.user("user", "active", "en")
	f.grant(acl.Workspace, x.workspace.ID, "user", bob.ID, acl.Owner)
	tablesBefore := scanOne[int](f, "SELECT count(*) FROM meta.tables WHERE book_id=$1", x.book.ID)

	// The revoking grant mutation holds the workspace lock; bob's pre-lock check
	// still sees the old owner grant, so only the locked recheck can stop him.
	revoke := f.holdWorkspace(x.workspace.ID)
	if _, err := revoke.Exec(f.ctx, "DELETE FROM meta.grants WHERE resource_id=$1 AND principal_id=$2", x.workspace.ID, bob.ID); err != nil {
		t.Fatal(err)
	}
	wait := blocked(t, func() error { _, err := f.service.CreateTable(f.ctx, bob, x.book.ID, "Late"); return err })
	if err := revoke.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	expectError(t, wait(), 404, "ERR_NOT_FOUND")
	if scanOne[int](f, "SELECT count(*) FROM meta.tables WHERE book_id=$1", x.book.ID) != tablesBefore {
		t.Fatal("a revoked owner created a table")
	}

	// A downgrade leaves bob a visible reader, so the same race reports 403.
	f.grant(acl.Workspace, x.workspace.ID, "user", bob.ID, acl.Owner)
	downgrade := f.holdWorkspace(x.workspace.ID)
	if _, err := downgrade.Exec(f.ctx, "UPDATE meta.grants SET level='read' WHERE resource_id=$1 AND principal_id=$2", x.workspace.ID, bob.ID); err != nil {
		t.Fatal(err)
	}
	wait = blocked(t, func() error { _, err := f.service.CreateBook(f.ctx, bob, x.workspace.ID, "Late"); return err })
	if err := downgrade.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	expectError(t, wait(), 403, "ERR_PERMISSION_DENIED")
}

func TestDeleteRacingCreateReportsMissingParentAndLeavesNothing(t *testing.T) {
	f := newFixture(t)
	x := f.tree()
	tables := scanOne[int](f, "SELECT count(*) FROM meta.tables WHERE book_id=$1", x.book.ID)
	books := scanOne[int](f, "SELECT count(*) FROM meta.books WHERE workspace_id=$1", x.workspace.ID)
	schemas := scanOne[int](f, "SELECT count(*) FROM pg_namespace WHERE nspname LIKE 'b\\_%'")

	// Soft delete of the book in flight: the table creation waits, then finds no live book.
	deleting := f.holdWorkspace(x.workspace.ID)
	if _, err := deleting.Exec(f.ctx, "UPDATE meta.books SET deleted_at=now() WHERE id=$1", x.book.ID); err != nil {
		t.Fatal(err)
	}
	wait := blocked(t, func() error { _, err := f.service.CreateTable(f.ctx, x.alice, x.book.ID, "Late"); return err })
	if err := deleting.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	expectError(t, wait(), 404, "ERR_NOT_FOUND")
	if scanOne[int](f, "SELECT count(*) FROM meta.tables WHERE book_id=$1", x.book.ID) != tables {
		t.Fatal("table created under a trashed book")
	}

	// Workspace soft delete in flight blocks book creation the same way.
	deleting = f.holdWorkspace(x.workspace.ID)
	if _, err := deleting.Exec(f.ctx, "UPDATE meta.workspaces SET deleted_at=now() WHERE id=$1", x.workspace.ID); err != nil {
		t.Fatal(err)
	}
	wait = blocked(t, func() error { _, err := f.service.CreateBook(f.ctx, x.alice, x.workspace.ID, "Late"); return err })
	if err := deleting.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	expectError(t, wait(), 404, "ERR_NOT_FOUND")
	if scanOne[int](f, "SELECT count(*) FROM meta.books WHERE workspace_id=$1", x.workspace.ID) != books ||
		scanOne[int](f, "SELECT count(*) FROM pg_namespace WHERE nspname LIKE 'b\\_%'") != schemas {
		t.Fatal("late creation left metadata or a physical schema")
	}
}

func TestDisableRacingWorkspaceCreationFinishesFirst(t *testing.T) {
	f := newFixture(t)
	user := f.user("user", "active", "en")
	before := scanOne[int](f, "SELECT count(*) FROM meta.workspaces")
	disabling, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = disabling.Rollback(f.ctx) })
	if _, err := disabling.Exec(f.ctx, "UPDATE meta.users SET status='disabled' WHERE id=$1", user.ID); err != nil {
		t.Fatal(err)
	}
	wait := blocked(t, func() error {
		_, err := f.service.CreateWorkspace(f.ctx, user, WorkspaceInput{Name: "Late"})
		return err
	})
	if err := disabling.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	expectError(t, wait(), 401, "ERR_UNAUTHENTICATED")
	if scanOne[int](f, "SELECT count(*) FROM meta.workspaces") != before {
		t.Fatal("a disabled user created a workspace")
	}

	// A disable racing a structure mutation behaves the same way.
	x := f.tree()
	disabling, err = f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = disabling.Rollback(f.ctx) })
	if _, err := disabling.Exec(f.ctx, "UPDATE meta.users SET status='disabled' WHERE id=$1", x.alice.ID); err != nil {
		t.Fatal(err)
	}
	wait = blocked(t, func() error { _, err := f.service.CreateBook(f.ctx, x.alice, x.workspace.ID, "Late"); return err })
	if err := disabling.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	expectError(t, wait(), 401, "ERR_UNAUTHENTICATED")
}

func TestConcurrentTableCreationsAreSerializedAndDistinct(t *testing.T) {
	f := newFixture(t)
	x := f.tree()
	var wait sync.WaitGroup
	results := make([]TableSchema, 6)
	errs := make([]error, 6)
	for index := range results {
		wait.Add(1)
		go func() {
			defer wait.Done()
			results[index], errs[index] = f.service.CreateTable(f.ctx, x.alice, x.book.ID, "Parallel")
		}()
	}
	wait.Wait()
	positions := map[float64]bool{}
	physical := map[string]bool{}
	for index, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
		positions[results[index].Table.Position] = true
		_, name := f.physicalNames(results[index].Table.ID)
		physical[name] = true
	}
	if len(positions) != 6 || len(physical) != 6 {
		t.Fatalf("concurrent creations collided: positions=%v physical=%v", positions, physical)
	}
}

func TestStructureLockWaitIsBoundedAndSanitized(t *testing.T) {
	f := newFixture(t)
	x := f.tree()
	tables := scanOne[int](f, "SELECT count(*) FROM meta.tables WHERE book_id=$1", x.book.ID)
	f.holdWorkspace(x.workspace.ID)
	started := time.Now()
	_, err := f.service.CreateTable(f.ctx, x.alice, x.book.ID, "Waiting")
	expectError(t, err, 409, "ERR_DDL_LOCK_TIMEOUT")
	if elapsed := time.Since(started); elapsed < 2500*time.Millisecond || elapsed > 8*time.Second {
		t.Fatalf("lock wait took %v, want about three seconds", elapsed)
	}
	if scanOne[int](f, "SELECT count(*) FROM meta.tables WHERE book_id=$1", x.book.ID) != tables {
		t.Fatal("timed-out creation left a table")
	}
}

// A deferred constraint trigger makes COMMIT itself fail after every statement
// succeeded, the uncertain-outcome case that must still invalidate the resolver.
func (f *fixture) failCommitsOn(table string) {
	f.t.Helper()
	f.exec(`CREATE FUNCTION meta.fail_structure_commit() RETURNS trigger LANGUAGE plpgsql AS
		$$ BEGIN RAISE EXCEPTION 'fixture commit failure'; END $$`)
	f.exec("CREATE CONSTRAINT TRIGGER fail_structure_commit AFTER INSERT OR UPDATE ON " + table +
		" DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION meta.fail_structure_commit()")
	f.t.Cleanup(func() { f.exec("DROP FUNCTION meta.fail_structure_commit() CASCADE") })
}

func TestCommitFailureStillInvalidatesAndPersistsNothing(t *testing.T) {
	f := newFixture(t)
	x := f.tree()
	epoch := f.resolver.Stats().Epoch
	f.failCommitsOn("meta.books")
	books := scanOne[int](f, "SELECT count(*) FROM meta.books WHERE workspace_id=$1", x.workspace.ID)
	schemas := scanOne[int](f, "SELECT count(*) FROM pg_namespace WHERE nspname LIKE 'b\\_%'")
	if _, err := f.service.CreateBook(f.ctx, x.alice, x.workspace.ID, "Doomed"); err == nil {
		t.Fatal("the deferred commit failure must be returned")
	}
	if f.resolver.Stats().Epoch <= epoch {
		t.Fatal("an uncertain commit must still invalidate the resolver")
	}
	if scanOne[int](f, "SELECT count(*) FROM meta.books WHERE workspace_id=$1", x.workspace.ID) != books ||
		scanOne[int](f, "SELECT count(*) FROM pg_namespace WHERE nspname LIKE 'b\\_%'") != schemas {
		t.Fatal("failed commit leaked metadata or a physical schema")
	}

	// Soft deletion fails at commit too; primed cache entries are flushed regardless.
	if f.level(x.alice, acl.Book, x.book.ID) != acl.Owner {
		t.Fatal("expected owner access")
	}
	epoch = f.resolver.Stats().Epoch
	if err := f.service.DeleteBook(f.ctx, x.alice, x.book.ID); err == nil {
		t.Fatal("the deferred commit failure must be returned")
	}
	if f.resolver.Stats().Epoch <= epoch {
		t.Fatal("an uncertain delete must invalidate the resolver")
	}
	if scanOne[bool](f, "SELECT deleted_at IS NULL FROM meta.books WHERE id=$1", x.book.ID) != true {
		t.Fatal("a failed commit must not trash the book")
	}
}

func TestWorkspaceCommitFailureInvalidatesAndLeavesNoGrant(t *testing.T) {
	f := newFixture(t)
	user := f.user("user", "active", "en")
	f.failCommitsOn("meta.workspaces")
	epoch := f.resolver.Stats().Epoch
	workspaces := scanOne[int](f, "SELECT count(*) FROM meta.workspaces")
	grants := scanOne[int](f, "SELECT count(*) FROM meta.grants WHERE principal_id=$1", user.ID)
	if _, err := f.service.CreateWorkspace(f.ctx, user, WorkspaceInput{Name: "Doomed"}); err == nil {
		t.Fatal("the deferred commit failure must be returned")
	}
	if f.resolver.Stats().Epoch <= epoch {
		t.Fatal("an uncertain workspace creation must invalidate the resolver")
	}
	if scanOne[int](f, "SELECT count(*) FROM meta.workspaces") != workspaces ||
		scanOne[int](f, "SELECT count(*) FROM meta.grants WHERE principal_id=$1", user.ID) != grants {
		t.Fatal("failed commit left a workspace or owner grant")
	}
}

func TestRejectedAndRenameOnlyMutationsDoNotFlushTheCache(t *testing.T) {
	f := newFixture(t)
	x := f.tree()
	stranger := f.user("user", "active", "en")
	epoch := f.resolver.Stats().Epoch
	if _, err := f.service.CreateBook(f.ctx, stranger, x.workspace.ID, "No"); err == nil {
		t.Fatal("stranger must be rejected")
	}
	if _, err := f.service.UpdateBook(f.ctx, x.alice, x.book.ID, BookPatch{Name: ptr("Renamed")}); err != nil {
		t.Fatal(err)
	}
	index := 1
	if _, err := f.service.UpdateBook(f.ctx, x.alice, x.book.ID, BookPatch{Index: &index}); err != nil {
		t.Fatal(err)
	}
	if f.resolver.Stats().Epoch != epoch {
		t.Fatal("rejections, renames and reorders do not change authorization")
	}
	if _, err := f.service.CreateBook(f.ctx, x.alice, x.workspace.ID, "New"); err != nil {
		t.Fatal(err)
	}
	if f.resolver.Stats().Epoch == epoch {
		t.Fatal("creation must flush the cache")
	}
}
