package admin

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"pandatable/internal/acl"
)

func TestGroupNormalization(t *testing.T) {
	name, err := normalizeGroup("  Team-A  ", "first\nsecond\tcolumn")
	if err != nil || name != "Team-A" {
		t.Fatalf("name=%q err=%v", name, err)
	}
	for _, input := range [][2]string{{" ", ""}, {"bad\x00", ""}, {strings.Repeat("a", 101), ""},
		{"valid", strings.Repeat("a", 1001)}, {"valid", "bad\x01"}, {"\xff", ""}, {"valid", "\xff"}} {
		_, err := normalizeGroup(input[0], input[1])
		code(t, err, "ERR_VALIDATION")
	}
	if _, err := normalizeGroup(strings.Repeat("界", 100), strings.Repeat("界", 1000)); err != nil {
		t.Fatal(err)
	}
	for _, input := range []ListInput{{Limit: 0}, {Limit: 501}, {Limit: 1, Offset: -1},
		{Limit: 1, Q: "\x01"}, {Limit: 1, Q: strings.Repeat("a", 255)}, {Limit: 1, Status: "active"}} {
		_, err := groupListInput(input)
		code(t, err, "ERR_VALIDATION")
	}
}

func fixtureGroup(t *testing.T, f *fixture, name string) Group {
	t.Helper()
	group, err := f.service.CreateGroup(f.ctx, f.token, name, "Literal %_ description")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		f.exec(ctx, "DELETE FROM meta.grants WHERE principal_type='group' AND principal_id=$1", group.ID)
		f.exec(ctx, "DELETE FROM meta.groups WHERE id=$1", group.ID)
	})
	return group
}

func TestGroupsCRUDMembershipAndImmediateACL(t *testing.T) {
	f := newFixture(t)
	group := fixtureGroup(t, f, " team-a ")
	user, _ := f.account("user", "active", false)
	disabled, _ := f.account("user", "disabled", false)
	if group.Name != "team-a" || group.MemberCount != 0 {
		t.Fatalf("unexpected group %+v", group)
	}
	_, err := f.service.CreateGroup(f.ctx, f.token, "team-a", "")
	code(t, err, "ERR_GROUP_NAME_CONFLICT")
	fixtureGroup(t, f, "Team-A")
	listing, err := f.service.Groups(f.ctx, f.token, "", ListInput{Limit: 1, Q: " TEAM-A "})
	if err != nil || listing.Total != 2 || len(listing.Items) != 1 {
		t.Fatalf("listing=%+v err=%v", listing, err)
	}
	listing, err = f.service.Groups(f.ctx, f.token, "", ListInput{Limit: 1, Offset: 2, Q: "%_"})
	if err != nil || listing.Total != 2 || len(listing.Items) != 0 || listing.Items == nil {
		t.Fatalf("empty page=%+v err=%v", listing, err)
	}
	var workspace string
	if err := f.pool.QueryRow(f.ctx, "INSERT INTO meta.workspaces(name) VALUES ('group access') RETURNING id::text").Scan(&workspace); err != nil {
		t.Fatal(err)
	}
	f.workspaces = append(f.workspaces, workspace)
	f.exec(f.ctx, `INSERT INTO meta.grants(resource_type,resource_id,principal_type,principal_id,level)
		VALUES ('workspace',$1,'group',$2,'owner')`, workspace, group.ID)
	resource, err := acl.NewResource("workspace", workspace)
	if err != nil {
		t.Fatal(err)
	}
	check := func(want acl.Level) {
		t.Helper()
		access, err := f.resolver.Access(f.ctx, user, resource)
		if err != nil || access.Level != want {
			t.Fatalf("access=%+v want=%v err=%v", access, want, err)
		}
	}
	check(acl.None)
	if err := f.service.ChangeMember(f.ctx, f.token, group.ID, user, true); err != nil {
		t.Fatal(err)
	}
	check(acl.Owner)
	code(t, f.service.ChangeMember(f.ctx, f.token, group.ID, user, true), "ERR_MEMBERSHIP_EXISTS")
	code(t, f.service.ChangeMember(f.ctx, f.token, group.ID, disabled, true), "ERR_PRINCIPAL_DISABLED")
	code(t, f.service.ChangeMember(f.ctx, f.token, group.ID, "00000000-0000-4000-8000-000000000001", true), "ERR_NOT_FOUND")
	members, err := f.service.Members(f.ctx, f.token, group.ID, ListInput{Limit: 50})
	if err != nil || members.Total != 1 || len(members.Items) != 1 || members.Items[0].ID != user {
		t.Fatalf("members=%+v err=%v", members, err)
	}
	listing, err = f.service.Groups(f.ctx, f.token, user, ListInput{Limit: 50})
	if err != nil || listing.Total != 1 || listing.Items[0].MemberCount != 1 {
		t.Fatalf("user groups=%+v err=%v", listing, err)
	}
	if err := f.service.ChangeMember(f.ctx, f.token, group.ID, user, false); err != nil {
		t.Fatal(err)
	}
	check(acl.None)
	if err := f.service.ChangeMember(f.ctx, f.token, group.ID, user, false); err != nil {
		t.Fatal(err)
	}
	if err := f.service.ChangeMember(f.ctx, f.token, group.ID, user, true); err != nil {
		t.Fatal(err)
	}
	check(acl.Owner)
	if _, err := f.service.Update(f.ctx, f.token, user, UserPatch{Status: ptr("disabled")}); err != nil {
		t.Fatal(err)
	}
	members, err = f.service.Members(f.ctx, f.token, group.ID, ListInput{Limit: 50})
	if err != nil || members.Total != 1 || members.Items[0].Status != "disabled" {
		t.Fatalf("disabled retained=%+v err=%v", members, err)
	}
	if err := f.service.ChangeMember(f.ctx, f.token, group.ID, user, false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Update(f.ctx, f.token, user, UserPatch{Status: ptr("active")}); err != nil {
		t.Fatal(err)
	}
	if err := f.service.ChangeMember(f.ctx, f.token, group.ID, user, true); err != nil {
		t.Fatal(err)
	}
	check(acl.Owner)
	updated, err := f.service.UpdateGroup(f.ctx, f.token, group.ID, GroupPatch{Name: ptr(" team-b "), Description: ptr("")})
	if err != nil || updated.Name != "team-b" || updated.Description != "" {
		t.Fatalf("updated=%+v err=%v", updated, err)
	}
	if err := f.service.DeleteGroup(f.ctx, f.token, group.ID); err != nil {
		t.Fatal(err)
	}
	check(acl.None)
	var dangling int
	if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM meta.grants WHERE principal_id=$1", group.ID).Scan(&dangling); err != nil || dangling != 0 {
		t.Fatalf("dangling=%d err=%v", dangling, err)
	}
	_, err = f.service.Group(f.ctx, f.token, group.ID)
	code(t, err, "ERR_NOT_FOUND")
}

func TestGroupsAuthorizationAndConcurrentMembership(t *testing.T) {
	f := newFixture(t)
	group := fixtureGroup(t, f, "authorization")
	user, ordinary := f.account("user", "active", false)
	_, disabled := f.account("admin", "disabled", false)
	_, forced := f.account("admin", "active", true)
	for _, caller := range []struct{ token, code string }{{"bad", "ERR_UNAUTHENTICATED"},
		{ordinary, "ERR_PERMISSION_DENIED"}, {disabled, "ERR_UNAUTHENTICATED"}, {forced, "ERR_PASSWORD_CHANGE_REQUIRED"}} {
		_, err := f.service.Groups(f.ctx, caller.token, "", ListInput{Limit: 50})
		code(t, err, caller.code)
		_, err = f.service.CreateGroup(f.ctx, caller.token, "blocked", "")
		code(t, err, caller.code)
		code(t, f.service.ChangeMember(f.ctx, caller.token, group.ID, user, true), caller.code)
		code(t, f.service.DeleteGroup(f.ctx, caller.token, group.ID), caller.code)
	}
	start := make(chan struct{})
	done := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			done <- f.service.ChangeMember(f.ctx, f.token, group.ID, user, true)
		}()
	}
	close(start)
	first, second := <-done, <-done
	if (first == nil) == (second == nil) {
		t.Fatalf("one duplicate add must fail: %v / %v", first, second)
	}
	if first != nil {
		code(t, first, "ERR_MEMBERSHIP_EXISTS")
	} else {
		code(t, second, "ERR_MEMBERSHIP_EXISTS")
	}
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(f.ctx, "SELECT id FROM meta.users WHERE id=$1 FOR UPDATE", user); err != nil {
		t.Fatal(err)
	}
	go func() { done <- f.service.ChangeMember(f.ctx, f.token, group.ID, user, true) }()
	awaitBlocked(t, f)
	if _, err := tx.Exec(f.ctx, "UPDATE meta.users SET status='disabled' WHERE id=$1", user); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	code(t, <-done, "ERR_PRINCIPAL_DISABLED")
	if err := f.service.ChangeMember(f.ctx, f.token, group.ID, user, false); err != nil {
		t.Fatal(err)
	}
}

func TestGroupDeletionPrincipalLocksAndRollback(t *testing.T) {
	f := newFixture(t)
	group := fixtureGroup(t, f, "delete-lock")
	user, _ := f.account("user", "active", false)
	if err := f.service.ChangeMember(f.ctx, f.token, group.ID, user, true); err != nil {
		t.Fatal(err)
	}
	var workspace string
	if err := f.pool.QueryRow(f.ctx, "INSERT INTO meta.workspaces(name) VALUES ('retained') RETURNING id::text").Scan(&workspace); err != nil {
		t.Fatal(err)
	}
	f.workspaces = append(f.workspaces, workspace)
	f.exec(f.ctx, `INSERT INTO meta.grants(resource_type,resource_id,principal_type,principal_id,level)
		VALUES ('workspace',$1,'group',$2,'read')`, workspace, group.ID)
	f.exec(f.ctx, `CREATE FUNCTION meta.fail_group_cleanup() RETURNS trigger LANGUAGE plpgsql AS
		$$ BEGIN RAISE EXCEPTION 'fixture rollback'; END $$`)
	f.exec(f.ctx, `CREATE TRIGGER fail_group_cleanup BEFORE DELETE ON meta.grants
		FOR EACH ROW WHEN (OLD.principal_type='group') EXECUTE FUNCTION meta.fail_group_cleanup()`)
	err := f.service.DeleteGroup(f.ctx, f.token, group.ID)
	f.exec(f.ctx, "DROP TRIGGER fail_group_cleanup ON meta.grants")
	f.exec(f.ctx, "DROP FUNCTION meta.fail_group_cleanup()")
	if err == nil {
		t.Fatal("cleanup failure must roll back deletion")
	}
	retained, err := f.service.Group(f.ctx, f.token, group.ID)
	if err != nil || retained.MemberCount != 1 {
		t.Fatalf("rollback lost group/membership: %+v %v", retained, err)
	}
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(f.ctx, "SELECT id FROM meta.groups WHERE id=$1 FOR SHARE", group.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 2)
	go func() { done <- f.service.DeleteGroup(f.ctx, f.token, group.ID) }()
	awaitBlocked(t, f)
	memberDone := make(chan error, 1)
	go func() { memberDone <- f.service.ChangeMember(f.ctx, f.token, group.ID, user, true) }()
	awaitBlockedCount(t, f, 2)
	if err := tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	code(t, <-memberDone, "ERR_NOT_FOUND")
	tx, err = f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var target string
	if err := tx.QueryRow(f.ctx, "SELECT id::text FROM meta.groups WHERE id=$1 FOR SHARE", group.ID).Scan(&target); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("deleted principal still grantable: %v", err)
	}
}

func TestGroupMutationRechecksActingAdmin(t *testing.T) {
	f := newFixture(t)
	group := fixtureGroup(t, f, "reauthorize")
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(f.ctx, "SELECT id FROM meta.users WHERE id=$1 FOR UPDATE", f.admin); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- f.service.DeleteGroup(f.ctx, f.token, group.ID) }()
	awaitBlocked(t, f)
	if _, err := tx.Exec(f.ctx, "UPDATE meta.users SET role='user' WHERE id=$1", f.admin); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	code(t, <-done, "ERR_PERMISSION_DENIED")
}

func TestGroupDeletionRacesRealGrantCreation(t *testing.T) {
	for _, scenario := range []struct {
		name                   string
		grantFirst, membership bool
	}{{"grant-first", true, false}, {"delete-first", false, false}, {"grant-versus-membership", true, true}} {
		t.Run(scenario.name, func(t *testing.T) {
			f := newFixture(t)
			group := fixtureGroup(t, f, "grant-race")
			owner := f.admin
			if scenario.membership {
				owner, _ = f.account("user", "active", false)
			}
			var workspace string
			if err := f.pool.QueryRow(f.ctx, "INSERT INTO meta.workspaces(name) VALUES ('grant race') RETURNING id::text").Scan(&workspace); err != nil {
				t.Fatal(err)
			}
			f.workspaces = append(f.workspaces, workspace)
			f.exec(f.ctx, `INSERT INTO meta.grants(resource_type,resource_id,principal_type,principal_id,level)
				VALUES ('workspace',$1,'user',$2,'owner')`, workspace, owner)
			grants, err := acl.NewService(f.pool, f.resolver)
			if err != nil {
				t.Fatal(err)
			}
			tx, err := f.pool.Begin(f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(context.Background()) }()
			if scenario.grantFirst {
				f.exec(f.ctx, `CREATE FUNCTION meta.gate_group_grant() RETURNS trigger LANGUAGE plpgsql AS
					$$ BEGIN PERFORM pg_advisory_xact_lock(81106); RETURN NEW; END $$`)
				f.exec(f.ctx, `CREATE TRIGGER gate_group_grant BEFORE INSERT ON meta.grants
					FOR EACH ROW WHEN (NEW.principal_type='group') EXECUTE FUNCTION meta.gate_group_grant()`)
				defer f.exec(f.ctx, "DROP FUNCTION meta.gate_group_grant() CASCADE")
				if _, err := tx.Exec(f.ctx, "SELECT pg_advisory_xact_lock(81106)"); err != nil {
					t.Fatal(err)
				}
			} else if _, err := tx.Exec(f.ctx, "SELECT id FROM meta.workspaces WHERE id=$1 FOR NO KEY UPDATE", workspace); err != nil {
				t.Fatal(err)
			}
			grantDone := make(chan error, 1)
			go func() {
				_, err := grants.Put(f.ctx, owner, acl.PutInput{ResourceType: "workspace", ResourceID: workspace,
					PrincipalType: "group", PrincipalID: group.ID, Level: "read"})
				grantDone <- err
			}()
			awaitBlocked(t, f)
			deleteDone := make(chan error, 1)
			go func() {
				if scenario.membership {
					deleteDone <- f.service.ChangeMember(f.ctx, f.token, group.ID, owner, true)
				} else {
					deleteDone <- f.service.DeleteGroup(f.ctx, f.token, group.ID)
				}
			}()
			if scenario.grantFirst && !scenario.membership {
				awaitBlockedCount(t, f, 2)
			} else if err := <-deleteDone; err != nil {
				t.Fatal(err)
			}
			if err := tx.Commit(f.ctx); err != nil {
				t.Fatal(err)
			}
			if scenario.grantFirst {
				if err := <-grantDone; err != nil {
					t.Fatalf("locked grant should commit before deletion: %v", err)
				}
				if !scenario.membership {
					if err := <-deleteDone; err != nil {
						t.Fatal(err)
					}
				}
			} else {
				code(t, <-grantDone, "ERR_PRINCIPAL_NOT_FOUND")
			}
			var remaining int
			want := 0
			if scenario.membership {
				want = 1
			}
			if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM meta.grants WHERE principal_id=$1", group.ID).Scan(&remaining); err != nil || remaining != want {
				t.Fatalf("dangling grants=%d err=%v", remaining, err)
			}
		})
	}
}

func TestMembershipCommitErrorInvalidatesACL(t *testing.T) {
	f := newFixture(t)
	group := fixtureGroup(t, f, "commit-error")
	user, _ := f.account("user", "active", false)
	f.exec(f.ctx, `CREATE FUNCTION meta.fail_membership_commit() RETURNS trigger LANGUAGE plpgsql AS
		$$ BEGIN RAISE EXCEPTION 'fixture commit failure'; END $$`)
	f.exec(f.ctx, `CREATE CONSTRAINT TRIGGER fail_membership_commit AFTER INSERT ON meta.group_members
		DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION meta.fail_membership_commit()`)
	defer f.exec(f.ctx, "DROP FUNCTION meta.fail_membership_commit() CASCADE")
	before := f.resolver.Stats().Epoch
	if err := f.service.ChangeMember(f.ctx, f.token, group.ID, user, true); err == nil {
		t.Fatal("deferred commit failure must be returned")
	}
	if f.resolver.Stats().Epoch <= before {
		t.Fatal("commit error must still invalidate ACL")
	}
	members, err := f.service.Members(f.ctx, f.token, group.ID, ListInput{Limit: 50})
	if err != nil || members.Total != 0 {
		t.Fatalf("commit failure leaked membership: %+v %v", members, err)
	}
}
