package acl

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/apperr"
)

const (
	userA = "018f0000-0000-7000-8000-00000000000a"
	userB = "018f0000-0000-7000-8000-00000000000b"
	wsID  = "018f0000-0000-7000-8000-0000000000a1"
)

func status(t *testing.T, err error) (int, string) {
	t.Helper()
	if err == nil {
		return 0, ""
	}
	var app *apperr.Error
	if !errors.As(err, &app) {
		t.Fatalf("expected typed error, got %v", err)
	}
	return app.Status, app.Code
}

func TestLevelOrderingAndParsing(t *testing.T) {
	if !(None < Read && Read < Write && Write < Owner) {
		t.Fatal("levels must be ordered none < read < write < owner")
	}
	for _, name := range []string{"read", "write", "owner"} {
		level, ok := ParseLevel(name)
		if !ok || level.String() != name {
			t.Fatalf("ParseLevel(%q)=%v,%v", name, level, ok)
		}
	}
	for _, name := range []string{"", "none", "admin", "READ", "owner ", "Read"} {
		if _, ok := ParseLevel(name); ok {
			t.Fatalf("ParseLevel(%q) must fail: only the three grantable levels are valid", name)
		}
	}
}

func TestResourceParsing(t *testing.T) {
	resource, err := ParseResourceRef("table:" + wsID)
	if err != nil || resource.Type != Table || resource.ID.String() != wsID {
		t.Fatalf("valid reference rejected: %v %v", resource, err)
	}
	upper, err := NewResource("workspace", "018F0000-0000-7000-8000-0000000000A1")
	if err != nil || upper.ID.String() != wsID {
		t.Fatalf("UUIDs must be canonicalized: %v %v", upper, err)
	}
	for _, bad := range []string{"", "table", "table:", ":" + wsID, "view:" + wsID, "table:not-a-uuid",
		"table:018f000000007000800000000000a1aa", "table:{" + wsID + "}", "table:" + wsID + " ",
		"workspace:" + wsID + ":x", "Workspace:" + wsID} {
		if _, err := ParseResourceRef(bad); err == nil {
			t.Fatalf("ParseResourceRef(%q) must fail", bad)
		} else if code, _ := status(t, err); code != 400 {
			t.Fatalf("ParseResourceRef(%q) status=%d", bad, code)
		}
	}
}

func TestAccessCheckMatrix(t *testing.T) {
	hidden := Access{}
	pathOnly := Access{Visible: true}
	cases := []struct {
		name     string
		access   Access
		required Level
		status   int
		code     string
	}{
		{"hidden read", hidden, Read, 404, "ERR_NOT_FOUND"},
		{"hidden owner", hidden, Owner, 404, "ERR_NOT_FOUND"},
		{"path-only read", pathOnly, Read, 403, "ERR_PERMISSION_DENIED"},
		{"path-only owner", pathOnly, Owner, 403, "ERR_PERMISSION_DENIED"},
		{"read satisfies read", Access{Read, true}, Read, 0, ""},
		{"read lacks write", Access{Read, true}, Write, 403, "ERR_PERMISSION_DENIED"},
		{"read lacks owner", Access{Read, true}, Owner, 403, "ERR_PERMISSION_DENIED"},
		{"write satisfies read", Access{Write, true}, Read, 0, ""},
		{"write satisfies write", Access{Write, true}, Write, 0, ""},
		{"write lacks owner", Access{Write, true}, Owner, 403, "ERR_PERMISSION_DENIED"},
		{"owner satisfies all", Access{Owner, true}, Owner, 0, ""},
		{"owner satisfies write", Access{Owner, true}, Write, 0, ""},
		{"none requirement is at least read", hidden, None, 404, "ERR_NOT_FOUND"},
	}
	for _, test := range cases {
		code, name := status(t, test.access.Check(test.required))
		if code != test.status || name != test.code {
			t.Errorf("%s: got %d %s want %d %s", test.name, code, name, test.status, test.code)
		}
	}
}

type fakeLoader struct {
	calls  atomic.Int64
	result func(user pgtype.UUID, resource Resource) (Access, error)
}

func (fake *fakeLoader) load(_ context.Context, user pgtype.UUID, resource Resource) (Access, error) {
	fake.calls.Add(1)
	return fake.result(user, resource)
}

func resource(t *testing.T, kind ResourceType, id string) Resource {
	t.Helper()
	value, err := NewResource(string(kind), id)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestResolverCacheHitInvalidateAndTTL(t *testing.T) {
	level := Read
	fake := &fakeLoader{result: func(pgtype.UUID, Resource) (Access, error) { return Access{level, true}, nil }}
	clock := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	resolver := newResolver(fake.load, func() time.Time { return clock }, 5*time.Second, 8)
	target := resource(t, Table, wsID)
	ctx := context.Background()

	for range 3 {
		access, err := resolver.Access(ctx, userA, target)
		if err != nil || access.Level != Read {
			t.Fatalf("access=%v err=%v", access, err)
		}
	}
	if fake.calls.Load() != 1 {
		t.Fatalf("repeated lookups must hit the cache, loads=%d", fake.calls.Load())
	}
	if _, err := resolver.Access(ctx, userB, target); err != nil || fake.calls.Load() != 2 {
		t.Fatal("cache must be keyed by user")
	}

	level = Owner
	if access, _ := resolver.Access(ctx, userA, target); access.Level != Read {
		t.Fatal("unchanged epoch inside the TTL serves the cached decision")
	}
	resolver.Invalidate()
	if access, _ := resolver.Access(ctx, userA, target); access.Level != Owner {
		t.Fatal("Invalidate must force a fresh decision")
	}
	if stats := resolver.Stats(); stats.Epoch != 1 || stats.Entries != 1 {
		t.Fatalf("stats=%+v", stats)
	}

	level = Write
	clock = clock.Add(4*time.Second + 900*time.Millisecond)
	if access, _ := resolver.Access(ctx, userA, target); access.Level != Owner {
		t.Fatal("entry must live until the TTL")
	}
	clock = clock.Add(200 * time.Millisecond)
	if access, _ := resolver.Access(ctx, userA, target); access.Level != Write {
		t.Fatal("TTL expiry bounds staleness from out-of-band changes")
	}
}

func TestResolverDoesNotStoreLoadsRacingInvalidation(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	level := Read
	var calls atomic.Int64
	resolver := newResolver(func(context.Context, pgtype.UUID, Resource) (Access, error) {
		calls.Add(1)
		observed := level
		if calls.Load() == 1 {
			close(started)
			<-release
		}
		return Access{observed, true}, nil
	}, time.Now, time.Minute, 8)
	target := resource(t, Workspace, wsID)

	done := make(chan Access, 1)
	go func() {
		access, _ := resolver.Access(context.Background(), userA, target)
		done <- access
	}()
	<-started
	// The mutation commits and invalidates while the earlier load is in flight.
	level = None
	resolver.Invalidate()
	close(release)
	if stale := <-done; stale.Level != Read {
		t.Fatalf("in-flight caller observed pre-commit data: %v", stale)
	}
	if access, _ := resolver.Access(context.Background(), userA, target); access.Level != None {
		t.Fatal("a load that raced an invalidation must not be cached")
	}
	if resolver.Stats().Entries != 1 {
		t.Fatal("only the fresh decision may be cached")
	}
}

func TestResolverBoundedLRUAndErrors(t *testing.T) {
	var fail atomic.Bool
	fake := &fakeLoader{result: func(pgtype.UUID, Resource) (Access, error) {
		if fail.Load() {
			return Access{}, errors.New("database unavailable")
		}
		return Access{Read, true}, nil
	}}
	resolver := newResolver(fake.load, time.Now, time.Minute, 2)
	ctx := context.Background()
	first := resource(t, Workspace, "018f0000-0000-7000-8000-000000000001")
	second := resource(t, Workspace, "018f0000-0000-7000-8000-000000000002")
	third := resource(t, Workspace, "018f0000-0000-7000-8000-000000000003")
	for _, target := range []Resource{first, second, first, third} {
		if _, err := resolver.Access(ctx, userA, target); err != nil {
			t.Fatal(err)
		}
	}
	if resolver.Stats().Entries != 2 {
		t.Fatalf("cache must stay bounded, entries=%d", resolver.Stats().Entries)
	}
	calls := fake.calls.Load()
	if _, _ = resolver.Access(ctx, userA, first); fake.calls.Load() != calls {
		t.Fatal("most recently used entry must survive eviction")
	}
	if _, _ = resolver.Access(ctx, userA, second); fake.calls.Load() != calls+1 {
		t.Fatal("least recently used entry must have been evicted")
	}

	resolver.Invalidate()
	fail.Store(true)
	if _, err := resolver.Access(ctx, userA, first); err == nil {
		t.Fatal("loader failures must surface, never become an allow/deny decision")
	}
	if resolver.Stats().Entries != 0 {
		t.Fatal("failed loads must not be cached")
	}
	if _, err := resolver.Access(ctx, "not-a-uuid", first); err == nil {
		t.Fatal("malformed user ids are rejected")
	}
}

func TestResolverRequire(t *testing.T) {
	access := Access{Write, true}
	resolver := newResolver(func(context.Context, pgtype.UUID, Resource) (Access, error) { return access, nil },
		time.Now, time.Minute, 4)
	target := resource(t, Table, wsID)
	if err := resolver.Require(context.Background(), userA, Write, target); err != nil {
		t.Fatal(err)
	}
	if code, name := status(t, resolver.Require(context.Background(), userA, Owner, target)); code != 403 || name != "ERR_PERMISSION_DENIED" {
		t.Fatalf("owner on a write grant: %d %s", code, name)
	}
}
