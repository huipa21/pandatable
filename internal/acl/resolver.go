package acl

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl/aclsql"
	"pandatable/internal/db"
)

const (
	// DefaultTTL bounds staleness from changes made outside this process's
	// Invalidate calls (for example manual SQL); services never rely on it.
	DefaultTTL        = 5 * time.Second
	DefaultMaxEntries = 4096
)

type cacheKey struct {
	user     pgtype.UUID
	resource Resource
}

type cacheEntry struct {
	key     cacheKey
	access  Access
	epoch   uint64
	expires time.Time
}

type loader func(ctx context.Context, user pgtype.UUID, resource Resource) (Access, error)

// Stats reports cache behavior for diagnostics and tests.
type Stats struct {
	Hits, Misses, Loads uint64
	Entries             int
	Epoch               uint64
}

// Resolver computes effective permissions with a bounded per-process LRU
// cache. Every grant, group-membership, user-status or resource-lifecycle
// mutation must call Invalidate after it commits (or may have committed): an
// epoch bump discards all entries and refuses to store loads that began before
// the bump, so a stale result can never be served for longer than the race
// between commit and the bump itself (D-079).
type Resolver struct {
	load loader
	now  func() time.Time
	ttl  time.Duration
	max  int

	mu      sync.Mutex
	epoch   uint64
	entries map[cacheKey]*list.Element
	order   *list.List
	stats   Stats
}

func NewResolver(pool *db.Pool) (*Resolver, error) {
	if pool == nil || pool.Pool == nil {
		return nil, errors.New("acl resolver requires a database pool")
	}
	queries := aclsql.New(pool)
	return newResolver(func(ctx context.Context, user pgtype.UUID, resource Resource) (Access, error) {
		return queryAccess(ctx, queries, user, resource)
	}, time.Now, DefaultTTL, DefaultMaxEntries), nil
}

func newResolver(load loader, now func() time.Time, ttl time.Duration, max int) *Resolver {
	return &Resolver{load: load, now: now, ttl: ttl, max: max,
		entries: make(map[cacheKey]*list.Element), order: list.New()}
}

// queryAccess is the single per-request SQL lookup: it joins the user's flat
// group memberships with the grants along the path of the live resource.
func queryAccess(ctx context.Context, queries *aclsql.Queries, user pgtype.UUID, resource Resource) (Access, error) {
	var level int32
	var visible bool
	switch resource.Type {
	case Workspace:
		row, err := queries.WorkspaceAccess(ctx, aclsql.WorkspaceAccessParams{UserID: user, WorkspaceID: resource.ID})
		level, visible = row.Level, row.Visible
		if err != nil {
			return Access{}, fmt.Errorf("resolve workspace access: %w", err)
		}
	case Book:
		row, err := queries.BookAccess(ctx, aclsql.BookAccessParams{UserID: user, BookID: resource.ID})
		level, visible = row.Level, row.Visible
		if err != nil {
			return Access{}, fmt.Errorf("resolve book access: %w", err)
		}
	case Table:
		row, err := queries.TableAccess(ctx, aclsql.TableAccessParams{UserID: user, TableID: resource.ID})
		level, visible = row.Level, row.Visible
		if err != nil {
			return Access{}, fmt.Errorf("resolve table access: %w", err)
		}
	default:
		return Access{}, ErrValidation()
	}
	if level < int32(None) || level > int32(Owner) {
		return Access{}, errors.New("unexpected access level")
	}
	return Access{Level: Level(level), Visible: visible || level > 0}, nil
}

// Access returns the caller's cached effective access. Disabled or unknown
// users, hidden and trashed resources yield the zero Access.
func (resolver *Resolver) Access(ctx context.Context, userID string, resource Resource) (Access, error) {
	user, ok := ParseUUID(userID)
	if !ok || !resource.ID.Valid {
		return Access{}, ErrValidation()
	}
	key := cacheKey{user: user, resource: resource}
	now := resolver.now()

	resolver.mu.Lock()
	if element, found := resolver.entries[key]; found {
		entry := element.Value.(*cacheEntry)
		if entry.epoch == resolver.epoch && now.Before(entry.expires) {
			resolver.order.MoveToFront(element)
			resolver.stats.Hits++
			access := entry.access
			resolver.mu.Unlock()
			return access, nil
		}
		resolver.remove(element)
	}
	resolver.stats.Misses++
	epoch := resolver.epoch
	resolver.mu.Unlock()

	access, err := resolver.load(ctx, user, resource)
	if err != nil {
		return Access{}, err
	}

	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	resolver.stats.Loads++
	// A bump during the load may predate the data we read, so do not store it.
	if epoch == resolver.epoch && resolver.max > 0 {
		if element, found := resolver.entries[key]; found {
			resolver.remove(element)
		}
		resolver.entries[key] = resolver.order.PushFront(&cacheEntry{
			key: key, access: access, epoch: epoch, expires: resolver.now().Add(resolver.ttl)})
		for resolver.order.Len() > resolver.max {
			resolver.remove(resolver.order.Back())
		}
	}
	return access, nil
}

func (resolver *Resolver) remove(element *list.Element) {
	resolver.order.Remove(element)
	delete(resolver.entries, element.Value.(*cacheEntry).key)
}

// Require resolves access and applies Access.Check.
func (resolver *Resolver) Require(ctx context.Context, userID string, required Level, resource Resource) error {
	access, err := resolver.Access(ctx, userID, resource)
	if err != nil {
		return err
	}
	return access.Check(required)
}

// Invalidate discards every cached decision. Call it after any commit that can
// change authorization: grants, group membership, user status/lifecycle,
// resource trash/restore/delete.
func (resolver *Resolver) Invalidate() {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	resolver.epoch++
	resolver.entries = make(map[cacheKey]*list.Element)
	resolver.order.Init()
}

func (resolver *Resolver) Stats() Stats {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	stats := resolver.stats
	stats.Entries = len(resolver.entries)
	stats.Epoch = resolver.epoch
	return stats
}
