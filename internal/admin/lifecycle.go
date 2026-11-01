package admin

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl"
	"pandatable/internal/admin/adminsql"
	"pandatable/internal/apperr"
	"pandatable/internal/auth"
	"pandatable/internal/events"
	"pandatable/internal/structure"
)

type Ownership struct {
	ResourceType  string `json:"resourceType"`
	ResourceID    string `json:"resourceId"`
	Name          string `json:"name"`
	WorkspaceID   string `json:"workspaceId"`
	WorkspaceName string `json:"workspaceName"`
	Trashed       bool   `json:"trashed"`
}

type OwnershipListing struct {
	Items []Ownership `json:"items"`
	Total int64       `json:"total"`
}

func lifecycleID(value string) (pgtype.UUID, error) {
	id, ok := acl.ParseUUID(value)
	if !ok {
		return pgtype.UUID{}, apperr.New(404, "ERR_NOT_FOUND", "Resource not found")
	}
	return id, nil
}

func disabledUser(ctx context.Context, queries *adminsql.Queries, id pgtype.UUID) (adminsql.GetUserRow, error) {
	user, err := queries.GetUser(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return user, apperr.New(404, "ERR_NOT_FOUND", "User not found")
	}
	if err != nil {
		return user, fmt.Errorf("read lifecycle user: %w", err)
	}
	if user.Status != "disabled" {
		return user, apperr.New(409, "ERR_USER_NOT_DISABLED", "User must be disabled first")
	}
	return user, nil
}

func (service *Service) Ownerships(ctx context.Context, token, userID string, input ListInput) (OwnershipListing, error) {
	id, err := lifecycleID(userID)
	if err != nil {
		return OwnershipListing{}, err
	}
	if input.Q != "" || input.Status != "" || input.Offset < 0 || input.Limit < 1 || input.Limit > 500 {
		return OwnershipListing{}, validation()
	}
	result := OwnershipListing{Items: []Ownership{}}
	err = service.groupSnapshot(ctx, token, func(queries *adminsql.Queries) error {
		if _, err := disabledUser(ctx, queries, id); err != nil {
			return err
		}
		if err := checkOwnershipIntegrity(ctx, queries, id); err != nil {
			return err
		}
		var err error
		result.Total, err = queries.CountUserOwnerships(ctx, id)
		if err != nil {
			return fmt.Errorf("count user ownerships: %w", err)
		}
		rows, err := queries.ListUserOwnerships(ctx, adminsql.ListUserOwnershipsParams{
			UserID: id, PageOffset: input.Offset, PageLimit: input.Limit,
		})
		if err != nil {
			return fmt.Errorf("list user ownerships: %w", err)
		}
		for _, row := range rows {
			result.Items = append(result.Items, Ownership{
				ResourceType: row.ResourceType, ResourceID: row.ResourceID.String(), Name: row.Name,
				WorkspaceID: row.WorkspaceID.String(), WorkspaceName: row.WorkspaceName, Trashed: row.Trashed,
			})
		}
		return nil
	})
	return result, err
}

func checkOwnershipIntegrity(ctx context.Context, queries *adminsql.Queries, id pgtype.UUID) error {
	count, err := queries.CountDanglingUserOwnerships(ctx, id)
	if err != nil {
		return fmt.Errorf("check ownership references: %w", err)
	}
	if count != 0 {
		return apperr.New(409, "ERR_LIFECYCLE_INCONSISTENT", "User ownership references a missing resource")
	}
	return nil
}

type lifecycleLocks func(*adminsql.Queries) error
type lifecycleWrite func(pgx.Tx, *adminsql.Queries, pgtype.UUID, *bool) error

func (service *Service) lifecycleMutation(ctx context.Context, token string, locks lifecycleLocks, write lifecycleWrite) error {
	hash, err := auth.SessionHash(token)
	if err != nil {
		return err
	}
	if _, err := authorize(ctx, service.queries, hash); err != nil {
		return err
	}
	invalidate := false
	err = service.pool.WithTx(ctx, func(tx pgx.Tx) error {
		queries := service.queries.WithTx(tx)
		if err := queries.LockUserAdministration(ctx); err != nil {
			return fmt.Errorf("lock user administration: %w", err)
		}
		actor, err := authorize(ctx, queries, hash)
		if err != nil {
			return err
		}
		// ACL mutations lock workspaces before user principals.
		if err := locks(queries); err != nil {
			return err
		}
		if _, err := queries.LockUser(ctx, actor); err != nil {
			return fmt.Errorf("lock acting administrator: %w", err)
		}
		if _, err := authorize(ctx, queries, hash); err != nil {
			return err
		}
		if err := write(tx, queries, actor, &invalidate); err != nil {
			return err
		}
		if invalidate {
			return events.Emit(ctx, tx, events.Event{Type: "nav.changed", By: actor.String()})
		}
		return nil
	})
	if invalidate {
		// A failed commit can have an uncertain outcome.
		service.resolver.Invalidate()
	}
	return err
}

func lockManagedUser(ctx context.Context, queries *adminsql.Queries, id, actor pgtype.UUID) (adminsql.GetUserRow, error) {
	if id != actor {
		if _, err := queries.LockUser(ctx, id); errors.Is(err, pgx.ErrNoRows) {
			return adminsql.GetUserRow{}, apperr.New(404, "ERR_NOT_FOUND", "User not found")
		} else if err != nil {
			return adminsql.GetUserRow{}, fmt.Errorf("lock managed user: %w", err)
		}
	}
	return disabledUser(ctx, queries, id)
}

func lockActiveRecipient(ctx context.Context, queries *adminsql.Queries, id, actor pgtype.UUID) error {
	if id != actor {
		if _, err := queries.LockUser(ctx, id); errors.Is(err, pgx.ErrNoRows) {
			return apperr.New(404, "ERR_PRINCIPAL_NOT_FOUND", "Recipient not found")
		} else if err != nil {
			return fmt.Errorf("lock ownership recipient: %w", err)
		}
	}
	user, err := queries.GetUser(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.New(404, "ERR_PRINCIPAL_NOT_FOUND", "Recipient not found")
	}
	if err != nil {
		return fmt.Errorf("read ownership recipient: %w", err)
	}
	if user.Status != "active" {
		return apperr.New(409, "ERR_PRINCIPAL_DISABLED", "Recipient must be active")
	}
	return nil
}

func (service *Service) TransferOwnerships(ctx context.Context, token, userID, recipientID string) (int64, error) {
	user, err := lifecycleID(userID)
	if err != nil {
		return 0, err
	}
	recipient, err := lifecycleID(recipientID)
	if err != nil {
		return 0, err
	}
	var transferred int64
	err = service.lifecycleMutation(ctx, token, func(queries *adminsql.Queries) error {
		_, err := queries.LockUserRelatedWorkspaces(ctx, user)
		if err != nil {
			return fmt.Errorf("lock ownership workspaces: %w", err)
		}
		return nil
	}, func(_ pgx.Tx, queries *adminsql.Queries, actor pgtype.UUID, invalidate *bool) error {
		if _, err := lockManagedUser(ctx, queries, user, actor); err != nil {
			return err
		}
		if err := lockActiveRecipient(ctx, queries, recipient, actor); err != nil {
			return err
		}
		if err := checkOwnershipIntegrity(ctx, queries, user); err != nil {
			return err
		}
		*invalidate = true
		var err error
		transferred, err = queries.TransferUserOwnerships(ctx, adminsql.TransferUserOwnershipsParams{
			UserID: user, RecipientID: recipient, ActorID: actor,
		})
		if err != nil {
			return fmt.Errorf("transfer user ownerships: %w", err)
		}
		if err := queries.RemoveUserOwnerships(ctx, user); err != nil {
			return fmt.Errorf("remove transferred ownerships: %w", err)
		}
		return nil
	})
	return transferred, err
}

func (service *Service) RescueWorkspace(ctx context.Context, token, workspaceID, recipientID string) error {
	workspace, err := lifecycleID(workspaceID)
	if err != nil {
		return err
	}
	recipient, err := lifecycleID(recipientID)
	if err != nil {
		return err
	}
	return service.lifecycleMutation(ctx, token, func(queries *adminsql.Queries) error {
		if _, err := queries.LockLifecycleWorkspace(ctx, workspace); errors.Is(err, pgx.ErrNoRows) {
			return apperr.New(404, "ERR_NOT_FOUND", "Workspace not found")
		} else if err != nil {
			return fmt.Errorf("lock orphaned workspace: %w", err)
		}
		return nil
	}, func(_ pgx.Tx, queries *adminsql.Queries, actor pgtype.UUID, invalidate *bool) error {
		if err := lockActiveRecipient(ctx, queries, recipient, actor); err != nil {
			return err
		}
		orphaned, err := queries.WorkspaceIsOrphaned(ctx, workspace)
		if err != nil {
			return fmt.Errorf("check orphaned workspace: %w", err)
		}
		if !orphaned {
			return apperr.New(409, "ERR_WORKSPACE_NOT_ORPHANED", "Workspace already has an active direct owner")
		}
		*invalidate = true
		if err := queries.SetWorkspaceOwner(ctx, adminsql.SetWorkspaceOwnerParams{
			WorkspaceID: workspace, UserID: recipient, ActorID: actor,
		}); err != nil {
			return fmt.Errorf("rescue orphaned workspace: %w", err)
		}
		return nil
	})
}

func (service *Service) HardDeleteUser(ctx context.Context, token, userID, confirmation string) error {
	user, err := lifecycleID(userID)
	if err != nil {
		return err
	}
	return service.lifecycleMutation(ctx, token, func(queries *adminsql.Queries) error {
		_, err := queries.LockUserRelatedWorkspaces(ctx, user)
		if err != nil {
			return fmt.Errorf("lock user workspaces: %w", err)
		}
		return nil
	}, func(tx pgx.Tx, queries *adminsql.Queries, actor pgtype.UUID, invalidate *bool) error {
		target, err := lockManagedUser(ctx, queries, user, actor)
		if err != nil {
			return err
		}
		if confirmation != target.Username {
			return apperr.New(400, "ERR_USERNAME_CONFIRMATION", "Username confirmation does not match")
		}
		workspaces, err := queries.ListSoleOwnedWorkspaces(ctx, user)
		if err != nil {
			return fmt.Errorf("list solely owned workspaces: %w", err)
		}
		*invalidate = true
		for _, workspace := range workspaces {
			if err := structure.PurgeOwnedWorkspace(ctx, tx, workspace, user); err != nil {
				return fmt.Errorf("purge solely owned workspace %s: %w", workspace.String(), err)
			}
		}
		if err := queries.RemoveAllUserGrants(ctx, user); err != nil {
			return fmt.Errorf("remove deleted user grants: %w", err)
		}
		if err := queries.RecordDeletedUser(ctx, user); err != nil {
			return fmt.Errorf("record deleted user attribution: %w", err)
		}
		if err := queries.HardDeleteUser(ctx, user); err != nil {
			return fmt.Errorf("hard-delete user: %w", err)
		}
		return events.Emit(ctx, tx, events.Event{Type: "session.check", Recipient: user.String()})
	})
}
