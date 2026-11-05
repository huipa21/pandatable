package structure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl"
	"pandatable/internal/apperr"
	"pandatable/internal/events"
	"pandatable/internal/structure/structuresql"
	"pandatable/internal/workspace"
)

type WorkspacePreference struct {
	WorkspaceID *string `json:"workspaceId"`
}

var errPreferenceChanged = errors.New("workspace preference candidate changed")

func preferenceDTO(id pgtype.UUID) WorkspacePreference {
	if !id.Valid {
		return WorkspacePreference{}
	}
	value := id.String()
	return WorkspacePreference{WorkspaceID: &value}
}

func preferenceBusy(err error) error {
	var postgres *pgconn.PgError
	if errors.As(err, &postgres) && postgres.Code == "55P03" {
		return apperr.New(409, "ERR_WORKSPACE_PREFERENCE_BUSY", "Workspace preference is busy; retry")
	}
	return err
}

// WorkspacePreference reads without creating anything. An uninitialized,
// deleted or inaccessible preference is represented as null.
func (s *Service) WorkspacePreference(ctx context.Context, actor Actor) (WorkspacePreference, error) {
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return WorkspacePreference{}, acl.ErrValidation()
	}
	id, err := s.queries.ReadWorkspacePreference(ctx, user)
	if errors.Is(err, pgx.ErrNoRows) {
		return WorkspacePreference{}, nil
	}
	if err != nil {
		return WorkspacePreference{}, fmt.Errorf("read workspace preference: %w", err)
	}
	if !id.Valid {
		return WorkspacePreference{}, nil
	}
	access, err := acl.AccessUncached(ctx, s.pool, user, acl.Resource{Type: acl.Workspace, ID: id})
	if err != nil {
		return WorkspacePreference{}, fmt.Errorf("check preferred workspace access: %w", err)
	}
	if access.Level < acl.Read {
		return WorkspacePreference{}, nil
	}
	return preferenceDTO(id), nil
}

// lockPreferredWorkspace follows the workspace -> user -> group lock order.
// Group locks also serialize deletion of a group supplying workspace access.
func lockPreferredWorkspace(ctx context.Context, tx pgx.Tx, queries *structuresql.Queries, user, id pgtype.UUID) (acl.Access, error) {
	target := acl.Resource{Type: acl.Workspace, ID: id}
	if _, err := acl.LockPath(ctx, tx, target); err != nil {
		return acl.Access{}, err
	}
	if err := acl.LockActiveUser(ctx, tx, user); err != nil {
		return acl.Access{}, err
	}
	if _, err := queries.LockWorkspaceGrantGroups(ctx, structuresql.LockWorkspaceGrantGroupsParams{
		UserID: user, WorkspaceID: id,
	}); err != nil {
		return acl.Access{}, fmt.Errorf("lock preferred workspace groups: %w", err)
	}
	return acl.AccessUncached(ctx, tx, user, target)
}

func setWorkspacePreference(ctx context.Context, queries *structuresql.Queries, user, id pgtype.UUID) error {
	count, err := queries.UpdateWorkspacePreference(ctx, structuresql.UpdateWorkspacePreferenceParams{
		UserID: user, WorkspaceID: id,
	})
	if err != nil {
		return fmt.Errorf("update workspace preference: %w", err)
	}
	if count != 1 {
		return errors.New("workspace preference disappeared")
	}
	return nil
}

// ResolveWorkspacePreference initializes existing users once, then retains the
// preferred live workspace or selects the first eligible workspace in nav order.
func (s *Service) ResolveWorkspacePreference(ctx context.Context, actor Actor) (WorkspacePreference, error) {
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return WorkspacePreference{}, acl.ErrValidation()
	}
	for range 4 {
		var selected pgtype.UUID
		created := false
		err := s.pool.WithTx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, lockTimeout); err != nil {
				return fmt.Errorf("set workspace preference lock timeout: %w", err)
			}
			queries := s.queries.WithTx(tx)
			if err := queries.LockWorkspacePreference(ctx, user); err != nil {
				return fmt.Errorf("lock workspace preference: %w", err)
			}
			preferred, err := queries.ReadWorkspacePreference(ctx, user)
			if errors.Is(err, pgx.ErrNoRows) {
				if err := acl.LockActiveUser(ctx, tx, user); err != nil {
					return err
				}
				selected, err = workspace.CreateStarter(ctx, queries, user)
				if err != nil {
					return err
				}
				created = true
				return events.Emit(ctx, tx, events.Event{Type: "nav.changed", By: actor.ID})
			}
			if err != nil {
				return fmt.Errorf("read workspace preference for resolution: %w", err)
			}
			eligible, err := queries.EligibleWorkspaces(ctx, user)
			if err != nil {
				return fmt.Errorf("list eligible workspaces: %w", err)
			}
			if len(eligible) == 0 {
				if err := acl.LockActiveUser(ctx, tx, user); err != nil {
					return err
				}
				eligible, err = queries.EligibleWorkspaces(ctx, user)
				if err != nil {
					return fmt.Errorf("recheck eligible workspaces: %w", err)
				}
				if len(eligible) != 0 {
					return errPreferenceChanged
				}
				row, err := workspace.CreateOwned(ctx, queries, structuresql.InsertWorkspaceParams{
					Name: workspace.StarterName, CreatedBy: user,
				})
				if err != nil {
					return err
				}
				created = true
				selected = row.ID
				if err := setWorkspacePreference(ctx, queries, user, selected); err != nil {
					return err
				}
				return events.Emit(ctx, tx, events.Event{Type: "nav.changed", By: actor.ID})
			}
			selected = eligible[0]
			for _, id := range eligible {
				if id == preferred {
					selected = id
					break
				}
			}
			access, err := lockPreferredWorkspace(ctx, tx, queries, user, selected)
			if err != nil {
				var app *apperr.Error
				if errors.As(err, &app) && app.Code == "ERR_NOT_FOUND" {
					return errPreferenceChanged
				}
				return err
			}
			if access.Level < acl.Read {
				return errPreferenceChanged
			}
			if selected != preferred {
				return setWorkspacePreference(ctx, queries, user, selected)
			}
			return nil
		})
		if created {
			s.resolver.Invalidate()
		}
		if err == errPreferenceChanged {
			continue
		}
		if err != nil {
			return WorkspacePreference{}, preferenceBusy(err)
		}
		return preferenceDTO(selected), nil
	}
	return WorkspacePreference{}, apperr.New(409, "ERR_WORKSPACE_PREFERENCE_BUSY", "Workspace preference is busy; retry")
}

// SetWorkspacePreference never grants access and checks authorization both
// before and after the existing workspace/principal locks.
func (s *Service) SetWorkspacePreference(ctx context.Context, actor Actor, workspaceID string) (WorkspacePreference, error) {
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return WorkspacePreference{}, acl.ErrValidation()
	}
	target, err := resourceOf(acl.Workspace, workspaceID)
	if err != nil {
		return WorkspacePreference{}, err
	}
	access, err := acl.AccessUncached(ctx, s.pool, user, target)
	if err != nil {
		return WorkspacePreference{}, err
	}
	if err := access.Check(acl.Read); err != nil {
		return WorkspacePreference{}, err
	}
	created := false
	err = s.pool.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, lockTimeout); err != nil {
			return fmt.Errorf("set workspace preference lock timeout: %w", err)
		}
		queries := s.queries.WithTx(tx)
		if err := queries.LockWorkspacePreference(ctx, user); err != nil {
			return fmt.Errorf("lock workspace preference: %w", err)
		}
		access, err := lockPreferredWorkspace(ctx, tx, queries, user, target.ID)
		if err != nil {
			return err
		}
		if err := access.Check(acl.Read); err != nil {
			return err
		}
		current, err := queries.ReadWorkspacePreference(ctx, user)
		if errors.Is(err, pgx.ErrNoRows) {
			if _, err := workspace.CreateStarter(ctx, queries, user); err != nil {
				return err
			}
			created = true
		} else if err != nil {
			return fmt.Errorf("read workspace preference for update: %w", err)
		} else if current == target.ID {
			return nil
		}
		if err := setWorkspacePreference(ctx, queries, user, target.ID); err != nil {
			return err
		}
		if created {
			return events.Emit(ctx, tx, events.Event{Type: "nav.changed", By: actor.ID})
		}
		return nil
	})
	if created {
		s.resolver.Invalidate()
	}
	if err != nil {
		return WorkspacePreference{}, preferenceBusy(err)
	}
	return preferenceDTO(target.ID), nil
}
