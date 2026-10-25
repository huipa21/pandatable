package acl

import (
	"context"
	"crypto/rand"
	"os"
	"strings"
	"testing"
	"time"

	"pandatable/internal/db"
	"pandatable/internal/testdb"
)

var database testdb.Suite

func TestMain(m *testing.M) { os.Exit(database.Run(m)) }

type fixture struct {
	t        *testing.T
	pool     *db.Pool
	ctx      context.Context
	resolver *Resolver
	service  *Service
	clock    time.Time
	users    []string
	groups   []string
	spaces   []string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool := database.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	f := &fixture{t: t, pool: pool, ctx: ctx, clock: time.Now()}
	f.resolver = newResolver(nil, func() time.Time { return f.clock }, DefaultTTL, DefaultMaxEntries)
	real, err := NewResolver(pool)
	if err != nil {
		t.Fatal(err)
	}
	f.resolver.load = real.load
	f.service, err = NewService(pool, f.resolver)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		f.exec(cleanup, "DELETE FROM meta.grants WHERE resource_id = ANY($1::uuid[]) OR principal_id = ANY($2::uuid[])", f.spaces, append(append([]string{}, f.users...), f.groups...))
		f.exec(cleanup, "DELETE FROM meta.workspaces WHERE id = ANY($1::uuid[])", f.spaces)
		f.exec(cleanup, "DELETE FROM meta.groups WHERE id = ANY($1::uuid[])", f.groups)
		f.exec(cleanup, "DELETE FROM meta.users WHERE id = ANY($1::uuid[])", f.users)
	})
	return f
}

func (f *fixture) exec(ctx context.Context, sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(ctx, sql, args...); err != nil {
		f.t.Fatalf("%s: %v", sql, err)
	}
}

func (f *fixture) scan(sql string, args ...any) string {
	f.t.Helper()
	var id string
	if err := f.pool.QueryRow(f.ctx, sql, args...).Scan(&id); err != nil {
		f.t.Fatalf("%s: %v", sql, err)
	}
	return id
}

func (f *fixture) user(role, state string) string {
	name := strings.ToLower(rand.Text())[:12]
	id := f.scan(`INSERT INTO meta.users (username,email,display_name,password_hash,role,status,avatar_color)
		VALUES ($1,$2,$1,'not-a-login-hash',$3,$4,'blue') RETURNING id::text`, name, name+"@acl.test", role, state)
	f.users = append(f.users, id)
	return id
}

func (f *fixture) group() string {
	id := f.scan("INSERT INTO meta.groups (name) VALUES ($1) RETURNING id::text", strings.ToLower(rand.Text())[:12])
	f.groups = append(f.groups, id)
	return id
}

func (f *fixture) member(group, user string) {
	f.exec(f.ctx, "INSERT INTO meta.group_members (group_id,user_id) VALUES ($1,$2)", group, user)
}

func (f *fixture) workspace() string {
	id := f.scan("INSERT INTO meta.workspaces (name) VALUES ('ACL fixture') RETURNING id::text")
	f.spaces = append(f.spaces, id)
	return id
}

func (f *fixture) book(workspace string) string {
	return f.scan("INSERT INTO meta.books (workspace_id,name,pg_schema,position) VALUES ($1,'Book',$2,1) RETURNING id::text",
		workspace, "b_"+strings.ToLower(rand.Text())[:10])
}

func (f *fixture) table(book string) string {
	return f.scan("INSERT INTO meta.tables (book_id,name,pg_name,position) VALUES ($1,'Table',$2,1) RETURNING id::text",
		book, "t_"+strings.ToLower(rand.Text())[:10])
}

func (f *fixture) grant(kind ResourceType, resource, principalType, principal string, level Level) {
	f.exec(f.ctx, `INSERT INTO meta.grants (resource_type,resource_id,principal_type,principal_id,level)
		VALUES ($1,$2,$3,$4,$5)`, string(kind), resource, principalType, principal, level.String())
	f.resolver.Invalidate()
}

func (f *fixture) access(user string, kind ResourceType, id string) Access {
	f.t.Helper()
	access, err := f.resolver.Access(f.ctx, user, resource(f.t, kind, id))
	if err != nil {
		f.t.Fatal(err)
	}
	return access
}

func (f *fixture) expectLevel(user string, kind ResourceType, id string, want Level) {
	f.t.Helper()
	if got := f.access(user, kind, id).Level; got != want {
		f.t.Fatalf("effective level on %s=%s want %s", kind, got, want)
	}
}

func (f *fixture) put(caller string, kind ResourceType, id, principalType, principal string, level Level) (PutResult, error) {
	return f.service.Put(f.ctx, caller, PutInput{ResourceType: string(kind), ResourceID: id,
		PrincipalType: principalType, PrincipalID: principal, Level: level.String()})
}

func (f *fixture) mustPut(caller string, kind ResourceType, id, principalType, principal string, level Level) PutResult {
	f.t.Helper()
	result, err := f.put(caller, kind, id, principalType, principal, level)
	if err != nil {
		f.t.Fatalf("put %s grant: %v", level, err)
	}
	return result
}

func (f *fixture) expectError(err error, wantStatus int, wantCode string) {
	f.t.Helper()
	if err == nil {
		f.t.Fatalf("expected %d %s, got success", wantStatus, wantCode)
	}
	if code, name := status(f.t, err); code != wantStatus || name != wantCode {
		f.t.Fatalf("got %d %s, want %d %s (%v)", code, name, wantStatus, wantCode, err)
	}
}

func (f *fixture) grantCount(kind ResourceType, id string) (count int) {
	f.t.Helper()
	if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM meta.grants WHERE resource_type=$1 AND resource_id=$2",
		string(kind), id).Scan(&count); err != nil {
		f.t.Fatal(err)
	}
	return count
}

// tree builds workspace -> book1{table1,table2} and book2{table3}.
type tree struct{ w, b1, b2, t1, t2, t3 string }

func (f *fixture) tree() tree {
	w := f.workspace()
	b1, b2 := f.book(w), f.book(w)
	return tree{w: w, b1: b1, b2: b2, t1: f.table(b1), t2: f.table(b1), t3: f.table(b2)}
}

func TestEffectiveLevelOrderingAndInheritance(t *testing.T) {
	f := newFixture(t)
	x := f.tree()
	bob := f.user("user", "active")

	f.expectLevel(bob, Workspace, x.w, None)
	f.grant(Workspace, x.w, "user", bob, Read)
	f.grant(Table, x.t1, "user", bob, Write)
	for _, check := range []struct {
		kind  ResourceType
		id    string
		level Level
	}{{Workspace, x.w, Read}, {Book, x.b1, Read}, {Table, x.t1, Write}, {Table, x.t2, Read},
		{Book, x.b2, Read}, {Table, x.t3, Read}} {
		f.expectLevel(bob, check.kind, check.id, check.level)
	}

	// AT-206: a lower lower-level grant never restricts inherited access.
	carol := f.user("user", "active")
	f.grant(Workspace, x.w, "user", carol, Write)
	f.grant(Table, x.t1, "user", carol, Read)
	f.expectLevel(carol, Table, x.t1, Write)
	f.grant(Book, x.b1, "user", carol, Read)
	f.expectLevel(carol, Book, x.b1, Write)

	// Owner inherited downward; a book grant does not flow upward.
	erin := f.user("user", "active")
	f.grant(Book, x.b2, "user", erin, Owner)
	f.expectLevel(erin, Table, x.t3, Owner)
	f.expectLevel(erin, Book, x.b2, Owner)
	f.expectLevel(erin, Workspace, x.w, None)
	f.expectLevel(erin, Book, x.b1, None)
	f.expectLevel(erin, Table, x.t1, None)
}

func TestEffectiveLevelGroupUnionAndMembership(t *testing.T) {
	f := newFixture(t)
	x := f.tree()
	carol := f.user("user", "active")
	teamA, teamB := f.group(), f.group()
	f.member(teamA, carol)
	f.member(teamB, carol)
	f.grant(Book, x.b1, "group", teamA, Write)
	f.grant(Workspace, x.w, "group", teamB, Read)
	f.grant(Table, x.t2, "user", carol, Read)

	f.expectLevel(carol, Table, x.t1, Write)
	f.expectLevel(carol, Table, x.t2, Write)
	f.expectLevel(carol, Table, x.t3, Read)
	f.grant(Table, x.t3, "group", teamA, Owner)
	f.resolver.Invalidate()
	f.expectLevel(carol, Table, x.t3, Owner)

	// Another group the user is not in never applies.
	outsider := f.group()
	f.grant(Workspace, x.w, "group", outsider, Owner)
	f.resolver.Invalidate()
	f.expectLevel(carol, Workspace, x.w, Read)

	// AT-207: leaving the group removes its access immediately after invalidation.
	f.exec(f.ctx, "DELETE FROM meta.group_members WHERE group_id=$1 AND user_id=$2", teamA, carol)
	f.resolver.Invalidate()
	f.expectLevel(carol, Table, x.t1, Read)
	f.expectLevel(carol, Table, x.t3, Read)
	f.exec(f.ctx, "DELETE FROM meta.group_members WHERE group_id=$1", teamB)
	f.resolver.Invalidate()
	f.expectLevel(carol, Table, x.t1, None)
}

func TestMembershipChangeWithoutInvalidationIsBoundedByTTL(t *testing.T) {
	f := newFixture(t)
	x := f.tree()
	carol := f.user("user", "active")
	team := f.group()
	f.member(team, carol)
	f.grant(Book, x.b1, "group", team, Write)
	f.expectLevel(carol, Table, x.t1, Write)

	f.exec(f.ctx, "DELETE FROM meta.group_members WHERE group_id=$1", team)
	f.clock = f.clock.Add(DefaultTTL - time.Millisecond)
	f.expectLevel(carol, Table, x.t1, Write)
	f.clock = f.clock.Add(2 * time.Millisecond)
	f.expectLevel(carol, Table, x.t1, None)
}

func TestDisabledUsersAdminsAndTrashHaveNoAccess(t *testing.T) {
	f := newFixture(t)
	x := f.tree()
	admin := f.user("admin", "active")
	f.expectLevel(admin, Workspace, x.w, None)
	if f.access(admin, Workspace, x.w).Visible {
		t.Fatal("admin role alone must not reveal a workspace")
	}

	disabled := f.user("user", "disabled")
	team := f.group()
	f.member(team, disabled)
	f.grant(Workspace, x.w, "user", disabled, Owner)
	f.grant(Workspace, x.w, "group", team, Owner)
	if got := f.access(disabled, Table, x.t1); got != (Access{}) {
		t.Fatalf("disabled user via direct and group grants: %+v", got)
	}

	bob := f.user("user", "active")
	f.grant(Table, x.t1, "user", bob, Write)
	f.grant(Workspace, x.w, "user", bob, Read)
	f.expectLevel(bob, Table, x.t1, Write)
	f.exec(f.ctx, "UPDATE meta.tables SET deleted_at=now() WHERE id=$1", x.t1)
	f.resolver.Invalidate()
	if got := f.access(bob, Table, x.t1); got != (Access{}) {
		t.Fatalf("trashed table must be hidden: %+v", got)
	}
	f.expectLevel(bob, Table, x.t2, Read)
	f.exec(f.ctx, "UPDATE meta.tables SET deleted_at=NULL WHERE id=$1", x.t1)
	f.exec(f.ctx, "UPDATE meta.books SET deleted_at=now() WHERE id=$1", x.b1)
	f.resolver.Invalidate()
	for _, check := range []struct {
		kind ResourceType
		id   string
	}{{Book, x.b1}, {Table, x.t1}, {Table, x.t2}} {
		if got := f.access(bob, check.kind, check.id); got != (Access{}) {
			t.Fatalf("%s under a trashed book must be hidden: %+v", check.kind, got)
		}
	}
	f.expectLevel(bob, Book, x.b2, Read)
	f.exec(f.ctx, "UPDATE meta.workspaces SET deleted_at=now() WHERE id=$1", x.w)
	f.resolver.Invalidate()
	if got := f.access(bob, Table, x.t3); got != (Access{}) {
		t.Fatalf("trashed workspace hides everything: %+v", got)
	}
	if got := f.access(bob, Workspace, x.w); got != (Access{}) {
		t.Fatalf("trashed workspace hidden: %+v", got)
	}
}

func TestPathOnlyVisibilityAndHiddenResources(t *testing.T) {
	f := newFixture(t)
	x := f.tree()
	carol := f.user("user", "active")
	dave := f.user("user", "active")
	f.grant(Table, x.t1, "user", carol, Read)

	f.expectLevel(carol, Table, x.t1, Read)
	for _, path := range []struct {
		kind ResourceType
		id   string
	}{{Workspace, x.w}, {Book, x.b1}} {
		access := f.access(carol, path.kind, path.id)
		if access.Level != None || !access.Visible {
			t.Fatalf("%s must be path-only visible: %+v", path.kind, access)
		}
		f.expectError(access.Check(Read), 403, "ERR_PERMISSION_DENIED")
	}
	for _, hidden := range []struct {
		kind ResourceType
		id   string
	}{{Table, x.t2}, {Book, x.b2}, {Table, x.t3}} {
		f.expectError(f.access(carol, hidden.kind, hidden.id).Check(Read), 404, "ERR_NOT_FOUND")
	}
	f.expectError(f.access(dave, Table, x.t1).Check(Read), 404, "ERR_NOT_FOUND")
	f.expectError(f.access(dave, Workspace, x.w).Check(Read), 404, "ERR_NOT_FOUND")
	missing := "018f0000-0000-7000-8000-0000000000ff"
	for _, kind := range []ResourceType{Workspace, Book, Table} {
		f.expectError(f.access(carol, kind, missing).Check(Read), 404, "ERR_NOT_FOUND")
	}
	// A workspace grant from a trashed descendant is not path-visible.
	f.exec(f.ctx, "UPDATE meta.tables SET deleted_at=now() WHERE id=$1", x.t1)
	f.resolver.Invalidate()
	if f.access(carol, Workspace, x.w).Visible {
		t.Fatal("grant on a trashed table must not reveal the workspace")
	}
}

func TestCacheServesRepeatedLookupsAndRefreshesAfterGrantMutation(t *testing.T) {
	f := newFixture(t)
	x := f.tree()
	owner, bob := f.user("user", "active"), f.user("user", "active")
	f.grant(Workspace, x.w, "user", owner, Owner)
	f.mustPut(owner, Table, x.t1, "user", bob, Read)

	f.expectLevel(bob, Table, x.t1, Read)
	loads := f.resolver.Stats().Loads
	for range 5 {
		f.expectLevel(bob, Table, x.t1, Read)
	}
	if got := f.resolver.Stats().Loads; got != loads {
		t.Fatalf("cached lookups issued %d extra queries", got-loads)
	}
	f.mustPut(owner, Table, x.t1, "user", bob, Write)
	f.expectLevel(bob, Table, x.t1, Write)
	grant := f.scan("SELECT id::text FROM meta.grants WHERE principal_id=$1", bob)
	if err := f.service.Delete(f.ctx, owner, grant); err != nil {
		t.Fatal(err)
	}
	f.expectLevel(bob, Table, x.t1, None)
}

func TestPutValidatesTargetsAndPrincipals(t *testing.T) {
	f := newFixture(t)
	x := f.tree()
	owner, bob := f.user("user", "active"), f.user("user", "active")
	disabled := f.user("user", "disabled")
	team := f.group()
	f.grant(Workspace, x.w, "user", owner, Owner)
	missing := "018f0000-0000-7000-8000-0000000000fe"

	f.expectError(func() error { _, err := f.put(owner, Table, missing, "user", bob, Read); return err }(), 404, "ERR_NOT_FOUND")
	f.expectError(func() error { _, err := f.put(owner, Workspace, x.w, "user", missing, Read); return err }(), 404, "ERR_PRINCIPAL_NOT_FOUND")
	f.expectError(func() error { _, err := f.put(owner, Workspace, x.w, "group", missing, Read); return err }(), 404, "ERR_PRINCIPAL_NOT_FOUND")
	f.expectError(func() error { _, err := f.put(owner, Workspace, x.w, "user", disabled, Read); return err }(), 409, "ERR_PRINCIPAL_DISABLED")
	f.expectError(func() error { _, err := f.put(owner, Workspace, x.w, "group", bob, Read); return err }(), 404, "ERR_PRINCIPAL_NOT_FOUND")
	f.expectError(func() error { _, err := f.put(owner, Workspace, x.w, "user", team, Read); return err }(), 404, "ERR_PRINCIPAL_NOT_FOUND")

	for _, input := range []PutInput{
		{"view", x.w, "user", bob, "read"}, {"workspace", "nope", "user", bob, "read"},
		{"workspace", x.w, "role", bob, "read"}, {"workspace", x.w, "user", "nope", "read"},
		{"workspace", x.w, "user", bob, "admin"}, {"workspace", x.w, "user", bob, "none"},
		{"workspace", x.w, "user", bob, ""}, {"", "", "", "", ""},
	} {
		_, err := f.service.Put(f.ctx, owner, input)
		f.expectError(err, 400, "ERR_VALIDATION")
	}
	if _, err := f.service.Put(f.ctx, "not-a-uuid", PutInput{"workspace", x.w, "user", bob, "read"}); err == nil {
		t.Fatal("malformed caller must be rejected")
	}

	// Trashed targets are indistinguishable from missing ones, even for an owner.
	f.exec(f.ctx, "UPDATE meta.tables SET deleted_at=now() WHERE id=$1", x.t1)
	f.resolver.Invalidate()
	f.expectError(func() error { _, err := f.put(owner, Table, x.t1, "user", bob, Read); return err }(), 404, "ERR_NOT_FOUND")
	f.exec(f.ctx, "UPDATE meta.books SET deleted_at=now() WHERE id=$1", x.b2)
	f.resolver.Invalidate()
	f.expectError(func() error { _, err := f.put(owner, Book, x.b2, "group", team, Read); return err }(), 404, "ERR_NOT_FOUND")
	f.expectError(func() error { _, err := f.put(owner, Table, x.t3, "group", team, Read); return err }(), 404, "ERR_NOT_FOUND")
	if f.grantCount(Table, x.t1)+f.grantCount(Book, x.b2)+f.grantCount(Table, x.t3) != 0 {
		t.Fatal("rejected requests must not leave grants behind")
	}

	created := f.mustPut(owner, Workspace, x.w, "group", team, Write)
	if !created.Created || created.Grant.Level != "write" || created.Grant.Principal == nil || created.Grant.Principal.Name == "" {
		t.Fatalf("group grant result: %+v", created)
	}
	if created.Grant.GrantedBy == nil || *created.Grant.GrantedBy != owner {
		t.Fatal("grantedBy must record the caller")
	}
}

func TestPutUpsertReplacesLevelAndIsIdempotent(t *testing.T) {
	f := newFixture(t)
	x := f.tree()
	owner, other, bob := f.user("user", "active"), f.user("user", "active"), f.user("user", "active")
	f.grant(Workspace, x.w, "user", owner, Owner)
	f.grant(Workspace, x.w, "user", other, Owner)

	first := f.mustPut(owner, Book, x.b1, "user", bob, Read)
	if !first.Created {
		t.Fatal("first put creates")
	}
	again := f.mustPut(other, Book, x.b1, "user", bob, Read)
	if again.Created || again.Grant.ID != first.Grant.ID || again.Grant.GrantedBy == nil || *again.Grant.GrantedBy != owner {
		t.Fatalf("same-level put must be an unchanged no-op: %+v", again)
	}
	upgraded := f.mustPut(other, Book, x.b1, "user", bob, Owner)
	if upgraded.Created || upgraded.Grant.ID != first.Grant.ID || upgraded.Grant.Level != "owner" || *upgraded.Grant.GrantedBy != other {
		t.Fatalf("level change replaces in place: %+v", upgraded)
	}
	if f.grantCount(Book, x.b1) != 1 {
		t.Fatal("one grant per resource/principal pair")
	}
	f.expectLevel(bob, Book, x.b1, Owner)
	f.mustPut(owner, Book, x.b1, "user", bob, Read)
	f.expectLevel(bob, Book, x.b1, Read)
}

func TestInheritedLevelWarningAndRaiseOnly(t *testing.T) {
	f := newFixture(t)
	x := f.tree()
	owner, bob := f.user("user", "active"), f.user("user", "active")
	f.grant(Workspace, x.w, "user", owner, Owner)
	f.mustPut(owner, Workspace, x.w, "user", bob, Write)

	result := f.mustPut(owner, Table, x.t1, "user", bob, Read)
	if result.Grant.InheritedLevel == nil || *result.Grant.InheritedLevel != "write" {
		t.Fatalf("lower direct grant must report the higher inherited level: %+v", result.Grant)
	}
	f.expectLevel(bob, Table, x.t1, Write)

	listing, err := f.service.List(f.ctx, owner, resource(t, Table, x.t1))
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Items) != 1 || listing.Items[0].InheritedLevel == nil || len(listing.Inherited) != 2 {
		t.Fatalf("table listing: %+v", listing)
	}
	for _, entry := range listing.Inherited {
		if entry.ResourceType != "workspace" || entry.ResourceID != x.w {
			t.Fatalf("inherited entries must come from ancestors: %+v", entry)
		}
	}
	if listing.EffectiveLevel != "owner" || listing.Resource.Type != "table" {
		t.Fatalf("listing header: %+v", listing)
	}
	book, err := f.service.List(f.ctx, owner, resource(t, Book, x.b1))
	if err != nil || len(book.Items) != 0 || len(book.Inherited) != 2 {
		t.Fatalf("book listing: %+v %v", book, err)
	}
	space, err := f.service.List(f.ctx, owner, resource(t, Workspace, x.w))
	if err != nil || len(space.Items) != 2 || len(space.Inherited) != 0 {
		t.Fatalf("workspace listing: %+v %v", space, err)
	}
}

func TestListRequiresOwnerAndHidesUnknownResources(t *testing.T) {
	f := newFixture(t)
	x := f.tree()
	owner, writer, reader, stranger := f.user("user", "active"), f.user("user", "active"), f.user("user", "active"), f.user("user", "active")
	f.grant(Workspace, x.w, "user", owner, Owner)
	f.grant(Workspace, x.w, "user", writer, Write)
	f.grant(Table, x.t1, "user", reader, Read)

	_, err := f.service.List(f.ctx, writer, resource(t, Workspace, x.w))
	f.expectError(err, 403, "ERR_PERMISSION_DENIED")
	_, err = f.service.List(f.ctx, reader, resource(t, Table, x.t1))
	f.expectError(err, 403, "ERR_PERMISSION_DENIED")
	_, err = f.service.List(f.ctx, reader, resource(t, Workspace, x.w))
	f.expectError(err, 403, "ERR_PERMISSION_DENIED")
	_, err = f.service.List(f.ctx, reader, resource(t, Table, x.t2))
	f.expectError(err, 404, "ERR_NOT_FOUND")
	_, err = f.service.List(f.ctx, stranger, resource(t, Workspace, x.w))
	f.expectError(err, 404, "ERR_NOT_FOUND")
	_, err = f.service.List(f.ctx, owner, resource(t, Table, "018f0000-0000-7000-8000-0000000000fd"))
	f.expectError(err, 404, "ERR_NOT_FOUND")
}

func TestScopeOwnerBoundaries(t *testing.T) {
	f := newFixture(t)
	x := f.tree()
	alice, bob, carol := f.user("user", "active"), f.user("user", "active"), f.user("user", "active")
	f.grant(Workspace, x.w, "user", alice, Owner)

	// AT-209: a book owner manages that book and its tables only.
	f.mustPut(alice, Book, x.b1, "user", bob, Owner)
	f.mustPut(bob, Book, x.b1, "user", carol, Write)
	f.mustPut(bob, Table, x.t1, "user", carol, Read)
	f.mustPut(bob, Table, x.t2, "user", carol, Read)
	_, err := f.put(bob, Workspace, x.w, "user", carol, Read)
	f.expectError(err, 403, "ERR_PERMISSION_DENIED")
	_, err = f.put(bob, Book, x.b2, "user", carol, Read)
	f.expectError(err, 404, "ERR_NOT_FOUND")
	_, err = f.put(bob, Table, x.t3, "user", carol, Read)
	f.expectError(err, 404, "ERR_NOT_FOUND")
	// Owners may grant owner within scope (D-014) but cannot reach outside it.
	f.mustPut(bob, Book, x.b1, "user", carol, Owner)
	f.expectLevel(carol, Table, x.t1, Owner)
	f.expectLevel(carol, Book, x.b2, None)
	_, err = f.put(carol, Workspace, x.w, "user", carol, Owner)
	f.expectError(err, 403, "ERR_PERMISSION_DENIED")

	// A table owner manages just the table.
	dave := f.user("user", "active")
	f.mustPut(alice, Table, x.t3, "user", dave, Owner)
	f.mustPut(dave, Table, x.t3, "user", carol, Write)
	_, err = f.put(dave, Book, x.b2, "user", carol, Write)
	f.expectError(err, 403, "ERR_PERMISSION_DENIED")

	// AT-210: a workspace owner manages everything below, including owner grants.
	erin := f.user("user", "active")
	f.mustPut(alice, Workspace, x.w, "user", bob, Owner)
	f.mustPut(bob, Workspace, x.w, "user", erin, Owner)
	f.mustPut(bob, Table, x.t3, "user", erin, Read)
	f.expectLevel(erin, Workspace, x.w, Owner)

	// Lower levels and outsiders cannot manage grants.
	frank := f.user("user", "active")
	f.grant(Workspace, x.w, "user", frank, Write)
	_, err = f.put(frank, Table, x.t1, "user", carol, Read)
	f.expectError(err, 403, "ERR_PERMISSION_DENIED")
	admin, stranger := f.user("admin", "active"), f.user("user", "active")
	_, err = f.put(admin, Table, x.t1, "user", carol, Read)
	f.expectError(err, 404, "ERR_NOT_FOUND")
	_, err = f.put(stranger, Workspace, x.w, "user", stranger, Owner)
	f.expectError(err, 404, "ERR_NOT_FOUND")
	disabled := f.user("user", "disabled")
	f.grant(Workspace, x.w, "user", disabled, Owner)
	_, err = f.put(disabled, Workspace, x.w, "user", disabled, Owner)
	f.expectError(err, 404, "ERR_NOT_FOUND")
}

func TestDeleteGrantRules(t *testing.T) {
	f := newFixture(t)
	x := f.tree()
	alice, bob, carol := f.user("user", "active"), f.user("user", "active"), f.user("user", "active")
	f.grant(Workspace, x.w, "user", alice, Owner)
	f.mustPut(alice, Book, x.b1, "user", bob, Owner)
	onTable := f.mustPut(bob, Table, x.t1, "user", carol, Write).Grant.ID
	onOther := f.mustPut(alice, Book, x.b2, "user", carol, Write).Grant.ID

	f.expectError(f.service.Delete(f.ctx, alice, "nope"), 400, "ERR_VALIDATION")
	f.expectError(f.service.Delete(f.ctx, alice, "018f0000-0000-7000-8000-0000000000fc"), 404, "ERR_NOT_FOUND")
	f.expectError(f.service.Delete(f.ctx, carol, onTable), 403, "ERR_PERMISSION_DENIED")
	f.expectError(f.service.Delete(f.ctx, bob, onOther), 404, "ERR_NOT_FOUND")
	f.expectLevel(carol, Book, x.b2, Write)
	if err := f.service.Delete(f.ctx, bob, onTable); err != nil {
		t.Fatal(err)
	}
	f.expectError(f.service.Delete(f.ctx, bob, onTable), 404, "ERR_NOT_FOUND")
	f.expectLevel(carol, Table, x.t1, None)
	if err := f.service.Delete(f.ctx, alice, onOther); err != nil {
		t.Fatal(err)
	}
}

func TestLastOwnerInvariant(t *testing.T) {
	f := newFixture(t)
	x := f.tree()
	alice, bob := f.user("user", "active"), f.user("user", "active")
	team := f.group()
	f.member(team, bob)
	f.grant(Workspace, x.w, "user", alice, Owner)
	aliceGrant := f.scan("SELECT id::text FROM meta.grants WHERE principal_id=$1", alice)

	// AT-211: the only direct owner cannot remove or downgrade herself.
	f.expectError(f.service.Delete(f.ctx, alice, aliceGrant), 409, "ERR_LAST_OWNER")
	for _, level := range []Level{Write, Read} {
		_, err := f.put(alice, Workspace, x.w, "user", alice, level)
		f.expectError(err, 409, "ERR_LAST_OWNER")
	}
	f.expectLevel(alice, Workspace, x.w, Owner)

	// Group ownership never counts, so it cannot substitute for a direct owner.
	f.mustPut(alice, Workspace, x.w, "group", team, Owner)
	f.expectLevel(bob, Workspace, x.w, Owner)
	f.expectError(f.service.Delete(f.ctx, alice, aliceGrant), 409, "ERR_LAST_OWNER")
	_, err := f.put(alice, Workspace, x.w, "user", alice, Write)
	f.expectError(err, 409, "ERR_LAST_OWNER")
	// Re-asserting owner is not a downgrade.
	if result := f.mustPut(alice, Workspace, x.w, "user", alice, Owner); result.Created {
		t.Fatal("owner re-put must be a no-op")
	}
	// Lower scopes have no last-owner rule.
	book := f.mustPut(alice, Book, x.b1, "user", alice, Owner)
	if err := f.service.Delete(f.ctx, alice, book.Grant.ID); err != nil {
		t.Fatalf("book owner grants are not protected: %v", err)
	}

	// A disabled owner does not count; once another active direct owner exists, alice may step down.
	dave := f.user("user", "active")
	f.mustPut(alice, Workspace, x.w, "user", dave, Owner)
	f.exec(f.ctx, "UPDATE meta.users SET status='disabled' WHERE id=$1", dave)
	f.resolver.Invalidate()
	f.expectError(f.service.Delete(f.ctx, alice, aliceGrant), 409, "ERR_LAST_OWNER")
	f.exec(f.ctx, "UPDATE meta.users SET status='active' WHERE id=$1", dave)
	f.resolver.Invalidate()
	if _, err := f.put(alice, Workspace, x.w, "user", alice, Write); err != nil {
		t.Fatalf("after granting another direct owner she may downgrade: %v", err)
	}
	f.expectLevel(alice, Workspace, x.w, Write)
}

func TestOrphanedWorkspaceCanDropDisabledOwnerAndRecover(t *testing.T) {
	f := newFixture(t)
	x := f.tree()
	alice, bob, carol := f.user("user", "active"), f.user("user", "active"), f.user("user", "active")
	team := f.group()
	f.member(team, bob)
	f.grant(Workspace, x.w, "user", alice, Owner)
	f.grant(Workspace, x.w, "group", team, Owner)
	aliceGrant := f.scan("SELECT id::text FROM meta.grants WHERE principal_id=$1", alice)

	// D-014: disabling the last direct owner is allowed and orphans the workspace.
	f.exec(f.ctx, "UPDATE meta.users SET status='disabled' WHERE id=$1", alice)
	f.resolver.Invalidate()
	f.expectLevel(alice, Workspace, x.w, None)
	f.expectLevel(bob, Workspace, x.w, Owner)
	// Removing the already-inactive owner does not reduce the active-owner count.
	if err := f.service.Delete(f.ctx, bob, aliceGrant); err != nil {
		t.Fatalf("removing a disabled owner: %v", err)
	}
	f.mustPut(bob, Workspace, x.w, "user", carol, Owner)
	f.mustPut(bob, Workspace, x.w, "user", bob, Owner)
	groupGrant := f.scan("SELECT id::text FROM meta.grants WHERE principal_type='group' AND principal_id=$1", team)
	if err := f.service.Delete(f.ctx, carol, groupGrant); err != nil {
		t.Fatalf("group owner grants are not protected: %v", err)
	}
}
