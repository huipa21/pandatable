package acl

import (
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestConcurrentOwnerRemovalsKeepOneActiveOwner(t *testing.T) {
	f := newFixture(t)
	for round := range 12 {
		x := f.tree()
		alice, bob := f.user("user", "active"), f.user("user", "active")
		f.grant(Workspace, x.w, "user", alice, Owner)
		f.grant(Workspace, x.w, "user", bob, Owner)
		aliceGrant := f.scan("SELECT id::text FROM meta.grants WHERE principal_id=$1", alice)
		bobGrant := f.scan("SELECT id::text FROM meta.grants WHERE principal_id=$1", bob)

		results := make([]error, 2)
		start := make(chan struct{})
		var wait sync.WaitGroup
		for index, call := range []func() error{
			func() error { return f.service.Delete(f.ctx, alice, bobGrant) },
			func() error { return f.service.Delete(f.ctx, bob, aliceGrant) },
		} {
			wait.Add(1)
			go func() {
				defer wait.Done()
				<-start
				results[index] = call()
			}()
		}
		close(start)
		wait.Wait()

		var remaining int
		if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM meta.grants g JOIN meta.users u ON u.id=g.principal_id
			WHERE g.resource_type='workspace' AND g.resource_id=$1 AND g.principal_type='user'
			AND g.level='owner' AND u.status='active'`, x.w).Scan(&remaining); err != nil {
			t.Fatal(err)
		}
		if remaining != 1 {
			t.Fatalf("round %d: %d active direct owners remain; results=%v", round, remaining, results)
		}
		succeeded := 0
		for _, err := range results {
			if err == nil {
				succeeded++
			} else if code, _ := status(t, err); code != 403 && code != 404 && code != 409 {
				t.Fatalf("round %d: unexpected error %v", round, err)
			}
		}
		if succeeded != 1 {
			t.Fatalf("round %d: exactly one removal may win, results=%v", round, results)
		}
	}
}

func TestConcurrentDowngradesAndUpsertsAreSerialized(t *testing.T) {
	f := newFixture(t)
	x := f.tree()
	alice, bob, carol, erin := f.user("user", "active"), f.user("user", "active"), f.user("user", "active"), f.user("user", "active")
	f.grant(Workspace, x.w, "user", alice, Owner)
	f.grant(Workspace, x.w, "user", bob, Owner)
	f.grant(Workspace, x.w, "user", erin, Owner)

	var wait sync.WaitGroup
	errs := make([]error, 8)
	for index := range errs {
		wait.Add(1)
		go func() {
			defer wait.Done()
			switch index % 4 {
			case 0:
				_, errs[index] = f.put(alice, Workspace, x.w, "user", bob, Read)
			case 1:
				_, errs[index] = f.put(bob, Workspace, x.w, "user", alice, Write)
			default:
				_, errs[index] = f.put(erin, Book, x.b1, "user", carol, Read)
			}
		}()
	}
	wait.Wait()
	for index, err := range errs {
		if index%4 >= 2 && err != nil {
			t.Fatalf("independent upsert %d failed: %v", index, err)
		}
	}
	var owners int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM meta.grants WHERE resource_type='workspace'
		AND resource_id=$1 AND level='owner'`, x.w).Scan(&owners); err != nil {
		t.Fatal(err)
	}
	if owners < 1 {
		t.Fatalf("workspace lost every owner: %v", errs)
	}
	if f.grantCount(Book, x.b1) != 1 {
		t.Fatalf("concurrent upserts of one pair must produce exactly one row, got %d", f.grantCount(Book, x.b1))
	}
}

// startBlocked runs fn and reports whether it is still waiting after a delay.
func startBlocked(t *testing.T, fn func() error) (wait func() error) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		t.Fatalf("operation completed instead of waiting for the hard-delete lock: %v", err)
	case <-time.After(400 * time.Millisecond):
	}
	return func() error {
		select {
		case err := <-done:
			return err
		case <-time.After(10 * time.Second):
			t.Fatal("blocked grant mutation never resumed")
			return nil
		}
	}
}

func TestGrantSerializesWithHardDeletionOfTargetsAndPrincipals(t *testing.T) {
	f := newFixture(t)
	x := f.tree()
	alice, bob := f.user("user", "active"), f.user("user", "active")
	team := f.group()
	f.grant(Workspace, x.w, "user", alice, Owner)

	hardDelete := func(sql string, id string) pgx.Tx {
		tx, err := f.pool.Begin(f.ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = tx.Rollback(f.ctx) })
		if _, err := tx.Exec(f.ctx, sql, id); err != nil {
			t.Fatal(err)
		}
		return tx
	}

	// D-070: a grant waits for an in-flight hard delete of its book, then finds no target.
	deleting := hardDelete("DELETE FROM meta.books WHERE id=$1", x.b2)
	wait := startBlocked(t, func() error { _, err := f.put(alice, Book, x.b2, "user", bob, Read); return err })
	if err := deleting.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.expectError(wait(), 404, "ERR_NOT_FOUND")
	if f.grantCount(Book, x.b2) != 0 {
		t.Fatal("no grant may reference a hard-deleted book")
	}

	deleting = hardDelete("DELETE FROM meta.tables WHERE id=$1", x.t1)
	wait = startBlocked(t, func() error { _, err := f.put(alice, Table, x.t1, "user", bob, Read); return err })
	if err := deleting.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.expectError(wait(), 404, "ERR_NOT_FOUND")

	// Principals: a user or group being hard-deleted cannot receive a dangling grant.
	deleting = hardDelete("DELETE FROM meta.groups WHERE id=$1", team)
	wait = startBlocked(t, func() error { _, err := f.put(alice, Workspace, x.w, "group", team, Read); return err })
	if err := deleting.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.expectError(wait(), 404, "ERR_PRINCIPAL_NOT_FOUND")

	deleting = hardDelete("DELETE FROM meta.users WHERE id=$1", bob)
	wait = startBlocked(t, func() error { _, err := f.put(alice, Workspace, x.w, "user", bob, Read); return err })
	if err := deleting.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.expectError(wait(), 404, "ERR_PRINCIPAL_NOT_FOUND")
	if f.grantCount(Workspace, x.w) != 1 {
		t.Fatal("only the original owner grant may remain")
	}
}

func TestFailedMutationRollsBackAndReleasesLocks(t *testing.T) {
	f := newFixture(t)
	x := f.tree()
	alice, bob := f.user("user", "active"), f.user("user", "active")
	f.grant(Workspace, x.w, "user", alice, Owner)
	f.mustPut(alice, Workspace, x.w, "user", bob, Read)
	before := f.resolver.Stats().Epoch

	_, err := f.put(alice, Workspace, x.w, "user", alice, Read)
	f.expectError(err, 409, "ERR_LAST_OWNER")
	if f.resolver.Stats().Epoch != before {
		t.Fatal("rejected mutations must not flush the cache")
	}
	var level string
	if err := f.pool.QueryRow(f.ctx, "SELECT level FROM meta.grants WHERE principal_id=$1", alice).Scan(&level); err != nil || level != "owner" {
		t.Fatalf("rolled-back downgrade changed the grant: %q %v", level, err)
	}
	// A subsequent change on the same workspace proceeds promptly (no leaked lock).
	locked := make(chan error, 1)
	go func() {
		_, err := f.put(alice, Workspace, x.w, "user", bob, Write)
		locked <- err
	}()
	select {
	case err := <-locked:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("workspace lock leaked after a rejected mutation")
	}
	if f.resolver.Stats().Epoch == before {
		t.Fatal("successful mutations must invalidate the cache")
	}
}
