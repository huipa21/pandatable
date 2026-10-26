// Package structure implements the workspace/book/table API services (T1.10):
// atomic metadata plus physical DDL in one caller-owned transaction, uncached
// owner authorization under the D-079/D-080 lock protocol, bounded
// whole-transaction retry of generated-name collisions, and the permitted
// navigation tree.
package structure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl"
	"pandatable/internal/acl/aclsql"
	"pandatable/internal/apperr"
	"pandatable/internal/db"
	"pandatable/internal/ddl"
	"pandatable/internal/structure/structuresql"
)

const (
	// maxAttempts bounds whole-transaction retries of generated physical names.
	maxAttempts = 3
	// Earlier ACL/metadata lock waits are bounded here; DDL later resets the same
	// LOCAL setting to its own three seconds.
	lockTimeout = "SET LOCAL lock_timeout = '3s'"
)

// Actor is the authenticated caller. Locale selects the language of the default
// display names written into new tables.
type Actor struct {
	ID     string
	Locale string
}

type Service struct {
	pool     *db.Pool
	queries  *structuresql.Queries
	resolver *acl.Resolver
	newName  func(ddl.ObjectKind) (string, error)
}

func New(pool *db.Pool, resolver *acl.Resolver) (*Service, error) {
	if pool == nil || pool.Pool == nil || resolver == nil {
		return nil, errors.New("structure service requires a database pool and ACL resolver")
	}
	return &Service{pool: pool, queries: structuresql.New(pool), resolver: resolver, newName: ddl.NewIdentifier}, nil
}

// scope names the resource a mutation targets and who may perform it. owner
// selects the resource the caller must own; nil means the target itself.
// Target visibility is always checked first so hidden resources stay 404 even
// when a sibling or ancestor is visible.
type scope struct {
	target     acl.Resource
	owner      func(acl.Path) acl.Resource
	affectsACL bool
}

type txn struct {
	tx    pgx.Tx
	q     *structuresql.Queries
	user  pgtype.UUID
	actor Actor
	path  acl.Path
}

func resourceOf(kind acl.ResourceType, id string) (acl.Resource, error) {
	uuid, ok := acl.ParseUUID(id)
	if !ok {
		// A malformed id can never name a resource.
		return acl.Resource{}, acl.ErrNotFound()
	}
	return acl.Resource{Type: kind, ID: uuid}, nil
}

func authorize(ctx context.Context, conn aclsql.DBTX, user pgtype.UUID, sc scope, path acl.Path) error {
	access, err := acl.AccessUncached(ctx, conn, user, sc.target)
	if err != nil {
		return err
	}
	if !access.Visible {
		return acl.ErrNotFound()
	}
	if sc.owner == nil {
		return access.Check(acl.Owner)
	}
	parent, err := acl.AccessUncached(ctx, conn, user, sc.owner(path))
	if err != nil {
		return err
	}
	return parent.Check(acl.Owner)
}

// run authorizes cheaply without locks (so non-owners cannot contend), then
// repeats the check uncached after taking the workspace/book/table and actor
// locks, runs fn and commits. Resource lifecycle commits, including uncertain
// ones, invalidate the resolver.
func (s *Service) run(ctx context.Context, actor Actor, sc scope, fn func(*txn) error) error {
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return acl.ErrValidation()
	}
	path, err := acl.ResolvePath(ctx, s.pool, sc.target)
	if err != nil {
		return err
	}
	if err := authorize(ctx, s.pool, user, sc, path); err != nil {
		return err
	}
	written := false
	err = s.pool.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, lockTimeout); err != nil {
			return fmt.Errorf("set structure lock timeout: %w", err)
		}
		locked, err := acl.LockPath(ctx, tx, sc.target)
		if err != nil {
			return err
		}
		if err := acl.LockActiveUser(ctx, tx, user); err != nil {
			return err
		}
		if err := authorize(ctx, tx, user, sc, locked); err != nil {
			return err
		}
		if err := fn(&txn{tx: tx, q: s.queries.WithTx(tx), user: user, actor: actor, path: locked}); err != nil {
			return err
		}
		written = sc.affectsACL
		return nil
	})
	if written {
		s.resolver.Invalidate()
	}
	return sanitizeLock(err)
}

// sanitizeLock maps a lock wait expiry before DDL starts to the same sanitized
// 409 the DDL module returns, never exposing the PostgreSQL message.
func sanitizeLock(err error) error {
	var postgres *pgconn.PgError
	var app *apperr.Error
	if err != nil && errors.As(err, &postgres) && postgres.Code == "55P03" && !errors.As(err, &app) {
		return apperr.New(409, "ERR_DDL_LOCK_TIMEOUT", "Schema is busy; retry the transaction later")
	}
	return err
}

// generatedCollision recognizes only reservations of freshly generated physical
// names: the DDL creation collision or the two metadata uniqueness constraints
// on those names. Display-name or other 23505 errors never match.
func generatedCollision(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ddl.ErrIdentifierCollision) {
		return true
	}
	var postgres *pgconn.PgError
	return errors.As(err, &postgres) && postgres.Code == "23505" &&
		(postgres.ConstraintName == "books_pg_schema_key" || postgres.ConstraintName == "tables_book_id_pg_name_key")
}

// retryCollisions runs complete transactions with fresh names, at most
// maxAttempts times; nothing is retried inside an aborted transaction.
func retryCollisions(attempt func() error) error {
	for range maxAttempts {
		if err := attempt(); !generatedCollision(err) {
			return err
		}
	}
	return apperr.New(409, "ERR_DDL_CONFLICT", "Physical identifier is already reserved")
}
