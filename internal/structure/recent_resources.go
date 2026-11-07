package structure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"pandatable/internal/acl"
	"pandatable/internal/structure/structuresql"
)

// RecordRecent records a workspace or book open without changing navigation
// events or permissions.
func (s *Service) RecordRecent(ctx context.Context, actor Actor, kind, id string) error {
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return acl.ErrValidation()
	}
	switch kind {
	case "workspace", "book":
	default:
		return acl.ErrValidation()
	}
	target, err := resourceOf(acl.ResourceType(kind), id)
	if err != nil {
		return err
	}
	return s.pool.WithTx(ctx, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		if err := q.LockRecentResources(ctx, user); err != nil {
			return fmt.Errorf("lock recent resources: %w", err)
		}
		var writeErr error
		switch kind {
		case "workspace":
			_, writeErr = q.UpsertRecentWorkspace(ctx, structuresql.UpsertRecentWorkspaceParams{
				UserID: user, WorkspaceID: target.ID,
			})
		case "book":
			_, writeErr = q.UpsertRecentBook(ctx, structuresql.UpsertRecentBookParams{
				UserID: user, BookID: target.ID,
			})
		}
		if errors.Is(writeErr, pgx.ErrNoRows) {
			return acl.ErrNotFound()
		}
		if writeErr != nil {
			return fmt.Errorf("upsert recent %s: %w", kind, writeErr)
		}
		if err := q.PruneRecentResources(ctx, user); err != nil {
			return fmt.Errorf("prune recent resources: %w", err)
		}
		return nil
	})
}
