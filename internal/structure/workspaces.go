package structure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl"
	"pandatable/internal/events"
	"pandatable/internal/structure/structuresql"
)

type WorkspaceInput struct {
	Name  string
	Icon  *string
	Color *string
}

// OptionalText distinguishes an absent PATCH member from an explicit null.
type OptionalText struct {
	Set   bool
	Value *string
}

type WorkspacePatch struct {
	Name  *string
	Icon  OptionalText
	Color OptionalText
}

func workspaceDTO(id pgtype.UUID, name string, icon, color pgtype.Text, created pgtype.Timestamptz) Workspace {
	return Workspace{ID: id.String(), Name: name, Icon: textPointer(icon), Color: textPointer(color), CreatedAt: timeOf(created)}
}

func cleanOptional(value *string, clean func(string) (string, error)) (*string, error) {
	if value == nil {
		return nil, nil
	}
	cleaned, err := clean(*value)
	if err != nil {
		return nil, err
	}
	return &cleaned, nil
}

func notFoundIfNoRows(err error, operation string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return acl.ErrNotFound()
	}
	return fmt.Errorf("%s: %w", operation, err)
}

// CreateWorkspace lets any active user create a workspace; the workspace and
// the creator's direct owner grant commit atomically (D-010, D-023).
func (s *Service) CreateWorkspace(ctx context.Context, actor Actor, input WorkspaceInput) (Workspace, error) {
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return Workspace{}, acl.ErrValidation()
	}
	name, err := cleanName(input.Name)
	if err != nil {
		return Workspace{}, err
	}
	icon, err := cleanOptional(input.Icon, cleanIcon)
	if err != nil {
		return Workspace{}, err
	}
	color, err := cleanOptional(input.Color, cleanColor)
	if err != nil {
		return Workspace{}, err
	}
	var created Workspace
	written := false
	err = s.pool.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, lockTimeout); err != nil {
			return fmt.Errorf("set structure lock timeout: %w", err)
		}
		// The grant protocol's principal lock: a concurrent disable/delete of the
		// creator finishes first, so a disabled user never becomes an owner.
		if err := acl.LockActiveUser(ctx, tx, user); err != nil {
			return err
		}
		queries := s.queries.WithTx(tx)
		row, err := queries.InsertWorkspace(ctx, structuresql.InsertWorkspaceParams{
			Name: name, Icon: textValue(icon), Color: textValue(color), CreatedBy: user})
		if err != nil {
			return fmt.Errorf("insert workspace: %w", err)
		}
		if err := queries.InsertWorkspaceOwnerGrant(ctx, structuresql.InsertWorkspaceOwnerGrantParams{
			WorkspaceID: row.ID, UserID: user}); err != nil {
			return fmt.Errorf("insert workspace owner grant: %w", err)
		}
		created = workspaceDTO(row.ID, row.Name, row.Icon, row.Color, row.CreatedAt)
		written = true
		return events.Emit(ctx, tx, events.Event{Type: "nav.changed", By: actor.ID})
	})
	if written {
		s.resolver.Invalidate()
	}
	if err != nil {
		return Workspace{}, sanitizeLock(err)
	}
	return created, nil
}

func (s *Service) UpdateWorkspace(ctx context.Context, actor Actor, id string, patch WorkspacePatch) (Workspace, error) {
	target, err := resourceOf(acl.Workspace, id)
	if err != nil {
		return Workspace{}, err
	}
	if patch.Name == nil && !patch.Icon.Set && !patch.Color.Set {
		return Workspace{}, validation("At least one workspace field is required")
	}
	var name string
	if patch.Name != nil {
		if name, err = cleanName(*patch.Name); err != nil {
			return Workspace{}, err
		}
	}
	icon, err := cleanOptional(patch.Icon.Value, cleanIcon)
	if err != nil {
		return Workspace{}, err
	}
	color, err := cleanOptional(patch.Color.Value, cleanColor)
	if err != nil {
		return Workspace{}, err
	}
	var updated Workspace
	err = s.run(ctx, actor, scope{target: target}, func(c *txn) error {
		current, err := c.q.GetWorkspace(ctx, target.ID)
		if err != nil {
			return notFoundIfNoRows(err, "read workspace")
		}
		params := structuresql.UpdateWorkspaceParams{ID: target.ID, Name: current.Name, Icon: current.Icon, Color: current.Color}
		if patch.Name != nil {
			params.Name = name
		}
		if patch.Icon.Set {
			params.Icon = textValue(icon)
		}
		if patch.Color.Set {
			params.Color = textValue(color)
		}
		row, err := c.q.UpdateWorkspace(ctx, params)
		if err != nil {
			return notFoundIfNoRows(err, "update workspace")
		}
		updated = workspaceDTO(row.ID, row.Name, row.Icon, row.Color, row.CreatedAt)
		return nil
	})
	return updated, err
}

// DeleteWorkspace is a soft delete: metadata, physical objects and grants stay
// for the later Trash service while the ACL hides the workspace and everything
// beneath it immediately.
func (s *Service) DeleteWorkspace(ctx context.Context, actor Actor, id string) error {
	target, err := resourceOf(acl.Workspace, id)
	if err != nil {
		return err
	}
	return s.run(ctx, actor, scope{target: target, affectsACL: true}, func(c *txn) error {
		if _, err := c.q.TrashWorkspace(ctx, structuresql.TrashWorkspaceParams{ID: target.ID, DeletedBy: c.user}); err != nil {
			return notFoundIfNoRows(err, "trash workspace")
		}
		return nil
	})
}
