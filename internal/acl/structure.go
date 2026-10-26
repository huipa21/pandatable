package acl

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl/aclsql"
	"pandatable/internal/apperr"
)

// Path is the live workspace/book ancestry of a resource. Book is invalid for
// workspaces; for a table it is the table's parent book.
type Path struct {
	Workspace, Book pgtype.UUID
}

// ResolvePath finds the live ancestors of the resource; a missing or trashed
// target or ancestor is reported as hidden (404).
func ResolvePath(ctx context.Context, db aclsql.DBTX, resource Resource) (Path, error) {
	path, err := resolveTarget(ctx, aclsql.New(db), resource)
	return Path{Workspace: path.workspace, Book: path.book}, err
}

// LockPath takes the D-070/D-080 serialization locks of the resource inside the
// caller's transaction: workspace FOR NO KEY UPDATE, then book/table FOR KEY
// SHARE, one statement per lock. Structure services use the same protocol as
// grant mutations so the two serialize per workspace.
func LockPath(ctx context.Context, tx pgx.Tx, resource Resource) (Path, error) {
	path, err := lockTarget(ctx, aclsql.New(tx), resource)
	return Path{Workspace: path.workspace, Book: path.book}, err
}

// LockActiveUser locks the acting user FOR SHARE (the principal step of the
// grant protocol), so a concurrent disable or hard deletion finishes first.
// A missing or disabled user is unauthenticated, not a hidden resource.
func LockActiveUser(ctx context.Context, tx pgx.Tx, user pgtype.UUID) error {
	status, err := aclsql.New(tx).LockUserPrincipal(ctx, user)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.New(401, "ERR_UNAUTHENTICATED", "Authentication required")
	}
	if err != nil {
		return fmt.Errorf("lock acting user: %w", err)
	}
	if status != "active" {
		return apperr.New(401, "ERR_UNAUTHENTICATED", "Authentication required")
	}
	return nil
}

// AccessUncached evaluates effective access without the resolver cache. Mutation
// authorization must use it inside the locked transaction.
func AccessUncached(ctx context.Context, db aclsql.DBTX, user pgtype.UUID, resource Resource) (Access, error) {
	if !user.Valid || !resource.ID.Valid {
		return Access{}, ErrValidation()
	}
	return queryAccess(ctx, aclsql.New(db), user, resource)
}
