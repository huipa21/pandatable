package structure

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl"
	"pandatable/internal/apperr"
	"pandatable/internal/events"
	"pandatable/internal/structure/structuresql"
)

type ViewInput struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Scope string `json:"scope"`
}

type ViewPatch struct {
	Name   *string
	Locked *bool
	Config *ViewColumns
}

func (s *Service) UpdateViewMetadata(ctx context.Context, actor Actor, id string, version int32, patch ViewPatch) (View, error) {
	viewID, ok := acl.ParseUUID(id)
	if !ok {
		return View{}, acl.ErrNotFound()
	}
	if patch.Name == nil && patch.Locked == nil {
		return View{}, validation("A view name or lock change is required")
	}
	name := ""
	if patch.Name != nil {
		var err error
		name, err = cleanName(*patch.Name)
		if err != nil {
			return View{}, err
		}
	}
	view, err := s.queries.GetViewForQuery(ctx, viewID)
	if err != nil {
		return View{}, notFoundIfNoRows(err, "read view for update")
	}
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return View{}, acl.ErrValidation()
	}
	var result View
	err = s.withViewTable(ctx, actor, view.TableID.String(), &result, func(tx pgx.Tx, table rowTable) error {
		q := structuresql.New(tx)
		stored, err := q.LockViewColumns(ctx, structuresql.LockViewColumnsParams{
			ID: viewID, TableID: table.id,
		})
		if err != nil {
			return notFoundIfNoRows(err, "lock view for update")
		}
		locking := patch.Locked != nil && *patch.Locked != stored.Locked
		if err := authorizeViewMutation(ctx, s, tx, actor, user, table,
			stored.Scope, stored.OwnerID, stored.Locked, locking); err != nil {
			return err
		}
		if stored.Scope == "personal" && patch.Locked != nil {
			return validation("Personal views cannot be locked")
		}
		if version < 1 {
			return validation("A view version is required")
		}
		if stored.Version != version {
			return viewVersionConflict()
		}
		nextName := stored.Name
		if patch.Name != nil {
			nextName = name
		}
		nextLocked := stored.Locked
		if patch.Locked != nil {
			nextLocked = *patch.Locked
		}
		if err := q.SaveManagedView(ctx, structuresql.SaveManagedViewParams{
			ID: viewID, Name: nextName, Locked: nextLocked, Config: stored.Config,
		}); err != nil {
			return fmt.Errorf("save view metadata: %w", err)
		}
		result, err = managedView(viewID, nextName, stored.Type, stored.Scope,
			nextLocked, stored.Position, stored.Version+1, stored.Config, table)
		return err
	})
	return result, err
}

func (s *Service) CreateView(ctx context.Context, actor Actor, tableID string, input ViewInput) (View, error) {
	name, err := cleanName(input.Name)
	if err != nil {
		return View{}, err
	}
	if input.Scope != "shared" && input.Scope != "personal" {
		return View{}, validation("View scope must be shared or personal")
	}
	if input.Type == "" {
		input.Type = "grid"
	}
	if input.Type != "grid" && input.Type != "form" {
		return View{}, validation("View type is invalid")
	}
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return View{}, acl.ErrValidation()
	}

	var result View
	err = s.withViewTable(ctx, actor, tableID, &result, func(tx pgx.Tx, table rowTable) error {
		target := acl.Resource{Type: acl.Table, ID: table.id}
		level := acl.Read
		if input.Scope == "shared" {
			level = acl.Write
		}
		if err := s.authorizeRows(ctx, tx, user, target, level); err != nil {
			return err
		}

		q := structuresql.New(tx)
		views, err := q.LockTableViewsForManagement(ctx, structuresql.LockTableViewsForManagementParams{
			TableID: table.id, UserID: user,
		})
		if err != nil {
			return fmt.Errorf("lock table views for creation: %w", err)
		}
		position := nextViewPosition(views)
		config, err := defaultViewConfig(table, input.Type)
		if err != nil {
			return err
		}
		owner := pgtype.UUID{}
		if input.Scope == "personal" {
			owner = user
		}
		created, err := q.InsertManagedView(ctx, structuresql.InsertManagedViewParams{
			TableID: table.id, Name: name, Type: input.Type, Scope: input.Scope, OwnerID: owner,
			Position: position, Config: config, CreatedBy: user,
		})
		if err != nil {
			return fmt.Errorf("create view: %w", err)
		}
		result, err = managedView(created.ID, created.Name, created.Type, created.Scope,
			created.Locked, created.Position, created.Version, created.Config, table)
		return err
	})
	return result, err
}

func (s *Service) DuplicateView(ctx context.Context, actor Actor, id, name, scope string, config *ViewColumns) (View, error) {
	viewID, ok := acl.ParseUUID(id)
	if !ok {
		return View{}, acl.ErrNotFound()
	}
	cleaned, err := cleanName(name)
	if err != nil {
		return View{}, err
	}
	if scope != "shared" && scope != "personal" {
		return View{}, validation("View scope must be shared or personal")
	}
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return View{}, acl.ErrValidation()
	}
	source, err := s.queries.GetViewForQuery(ctx, viewID)
	if err != nil {
		return View{}, notFoundIfNoRows(err, "read source view")
	}
	var result View
	err = s.withViewTable(ctx, actor, source.TableID.String(), &result, func(tx pgx.Tx, table rowTable) error {
		target := acl.Resource{Type: acl.Table, ID: table.id}
		if source.Scope == "personal" && source.OwnerID.String() != actor.ID {
			return acl.ErrNotFound()
		}
		level := acl.Read
		if scope == "shared" {
			level = acl.Write
		}
		if err := s.authorizeRows(ctx, tx, user, target, level); err != nil {
			return err
		}

		q := structuresql.New(tx)
		views, err := q.LockTableViewsForManagement(ctx, structuresql.LockTableViewsForManagementParams{
			TableID: table.id, UserID: user,
		})
		if err != nil {
			return fmt.Errorf("lock table views for duplication: %w", err)
		}
		var sourceView structuresql.LockTableViewsForManagementRow
		found := false
		for _, candidate := range views {
			if candidate.ID == viewID {
				sourceView = candidate
				found = true
				break
			}
		}
		if !found {
			return acl.ErrNotFound()
		}
		if sourceView.Scope == "personal" && sourceView.OwnerID.String() != actor.ID {
			return acl.ErrNotFound()
		}
		sourceConfig := sourceView.Config
		if config != nil {
			if sourceView.Type != "grid" {
				return validation("Only grid view configuration can be overridden")
			}
			sourceConfig, _, err = normalizeManagedViewConfig(ctx, tx, actor, table, sourceView.Config, *config)
			if err != nil {
				return err
			}
		}
		owner := pgtype.UUID{}
		if scope == "personal" {
			owner = user
		}
		created, err := q.InsertManagedView(ctx, structuresql.InsertManagedViewParams{
			TableID: table.id, Name: cleaned, Type: sourceView.Type, Scope: scope, OwnerID: owner,
			Position: nextViewPosition(views), Config: sourceConfig, CreatedBy: user,
		})
		if err != nil {
			return fmt.Errorf("duplicate view: %w", err)
		}
		result, err = managedView(created.ID, created.Name, created.Type, created.Scope,
			created.Locked, created.Position, created.Version, created.Config, table)
		return err
	})
	return result, err
}

func (s *Service) DeleteView(ctx context.Context, actor Actor, id string, version int32) error {
	viewID, ok := acl.ParseUUID(id)
	if !ok {
		return acl.ErrNotFound()
	}
	view, err := s.queries.GetViewForQuery(ctx, viewID)
	if err != nil {
		return notFoundIfNoRows(err, "read view for deletion")
	}
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return acl.ErrValidation()
	}
	notification := View{ID: id, Scope: view.Scope, Version: version}
	return s.withViewTable(ctx, actor, view.TableID.String(), &notification, func(tx pgx.Tx, table rowTable) error {
		q := structuresql.New(tx)
		stored, err := q.LockViewColumns(ctx, structuresql.LockViewColumnsParams{
			ID: viewID, TableID: table.id,
		})
		if err != nil {
			return notFoundIfNoRows(err, "lock view for deletion")
		}
		if err := authorizeViewMutation(ctx, s, tx, actor, user, table, stored.Scope, stored.OwnerID, stored.Locked, false); err != nil {
			return err
		}
		if version < 1 {
			return validation("A view version is required")
		}
		if stored.Version != version {
			return viewVersionConflict()
		}
		if err := q.DeleteManagedView(ctx, viewID); err != nil {
			return fmt.Errorf("delete view: %w", err)
		}
		return nil
	})
}

func (s *Service) MoveView(ctx context.Context, actor Actor, id string, version int32, beforeID *string) (View, error) {
	viewID, ok := acl.ParseUUID(id)
	if !ok {
		return View{}, acl.ErrNotFound()
	}
	view, err := s.queries.GetViewForQuery(ctx, viewID)
	if err != nil {
		return View{}, notFoundIfNoRows(err, "read view for reordering")
	}
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return View{}, acl.ErrValidation()
	}
	var result View
	err = s.withViewTable(ctx, actor, view.TableID.String(), &result, func(tx pgx.Tx, table rowTable) error {
		q := structuresql.New(tx)
		views, err := q.LockTableViewsForManagement(ctx, structuresql.LockTableViewsForManagementParams{
			TableID: table.id, UserID: user,
		})
		if err != nil {
			return fmt.Errorf("lock table views for reordering: %w", err)
		}
		targetIndex := -1
		for i := range views {
			if views[i].ID == viewID {
				targetIndex = i
				break
			}
		}
		if targetIndex < 0 {
			return acl.ErrNotFound()
		}
		stored := views[targetIndex]
		if err := authorizeViewMutation(ctx, s, tx, actor, user, table, stored.Scope, stored.OwnerID, stored.Locked, false); err != nil {
			return err
		}
		if version < 1 {
			return validation("A view version is required")
		}
		if stored.Version != version {
			return viewVersionConflict()
		}

		ordered := append([]structuresql.LockTableViewsForManagementRow(nil), views[:targetIndex]...)
		ordered = append(ordered, views[targetIndex+1:]...)
		insertAt := len(ordered)
		if beforeID != nil {
			before, valid := acl.ParseUUID(*beforeID)
			if !valid {
				return validation("The destination view is invalid")
			}
			insertAt = -1
			for i := range ordered {
				if ordered[i].ID == before {
					insertAt = i
					break
				}
			}
			if insertAt < 0 {
				return acl.ErrNotFound()
			}
		}
		position := positionAt(ordered, insertAt)
		if math.IsNaN(position) || math.IsInf(position, 0) {
			return validation("Views cannot be reordered further")
		}
		if position != stored.Position {
			if err := q.MoveManagedView(ctx, structuresql.MoveManagedViewParams{
				ID: viewID, Position: position,
			}); err != nil {
				return fmt.Errorf("save view order: %w", err)
			}
			stored.Position = position
			stored.Version++
		}
		result, err = managedView(stored.ID, stored.Name, stored.Type, stored.Scope,
			stored.Locked, stored.Position, stored.Version, stored.Config, table)
		return err
	})
	return result, err
}

func (s *Service) withViewTable(ctx context.Context, actor Actor, tableID string, changed *View, run func(pgx.Tx, rowTable) error) error {
	target, err := resourceOf(acl.Table, tableID)
	if err != nil {
		return err
	}
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return acl.ErrValidation()
	}
	if err := s.authorizeRows(ctx, s.pool, user, target, acl.Read); err != nil {
		return err
	}
	err = s.pool.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, lockTimeout); err != nil {
			return fmt.Errorf("set view lock timeout: %w", err)
		}
		if _, err := acl.LockPath(ctx, tx, target); err != nil {
			return err
		}
		if err := acl.LockActiveUser(ctx, tx, user); err != nil {
			return err
		}
		if err := s.authorizeRows(ctx, tx, user, target, acl.Read); err != nil {
			return err
		}
		tableRow, err := structuresql.New(tx).LockRowTable(ctx, target.ID)
		if err != nil {
			return notFoundIfNoRows(err, "lock view table")
		}
		table, err := loadRowTable(ctx, structuresql.New(tx), tx, target.ID,
			tableRow.PgSchema, tableRow.PgName, tableRow.SchemaVersion)
		if err != nil {
			return err
		}
		if err := run(tx, table); err != nil {
			return err
		}
		if changed == nil || changed.ID == "" {
			return fmt.Errorf("changed view is missing from realtime event")
		}
		recipient := ""
		if changed.Scope == "personal" {
			recipient = actor.ID
		}
		if err := events.Emit(ctx, tx, events.Event{Type: "view.changed", TableID: table.id.String(),
			ViewID: changed.ID, Version: changed.Version, By: actor.ID, Recipient: recipient}); err != nil {
			return err
		}
		return events.Emit(ctx, tx, events.Event{Type: "nav.changed", By: actor.ID, Recipient: recipient})
	})
	return rowDatabaseError(err)
}

func authorizeViewMutation(ctx context.Context, s *Service, tx pgx.Tx, actor Actor, user pgtype.UUID,
	table rowTable, scope string, ownerID pgtype.UUID, locked, locking bool,
) error {
	if scope == "personal" {
		if ownerID.String() != actor.ID {
			return acl.ErrNotFound()
		}
		return nil
	}
	level := acl.Write
	if locked || locking {
		level = acl.Owner
	}
	return s.authorizeRows(ctx, tx, user, acl.Resource{Type: acl.Table, ID: table.id}, level)
}

func defaultViewConfig(table rowTable, viewType string) ([]byte, error) {
	config := ViewColumns{Fields: make([]ViewColumn, 0, len(table.fields))}
	for _, field := range table.fields {
		config.Fields = append(config.Fields, ViewColumn{ID: field.id, Visible: !field.system})
	}
	if viewType == "form" {
		config.Form = &FormConfig{
			Fields: []FormField{}, AfterSubmit: "message",
		}
	}
	return json.Marshal(config)
}

func nextViewPosition(views []structuresql.LockTableViewsForManagementRow) float64 {
	position := 0.0
	for _, view := range views {
		position = max(position, view.Position)
	}
	return position + 1024
}

func positionAt(views []structuresql.LockTableViewsForManagementRow, index int) float64 {
	if len(views) == 0 {
		return 1024
	}
	if index == 0 {
		return views[0].Position - 1024
	}
	if index >= len(views) {
		return views[len(views)-1].Position + 1024
	}
	left, right := views[index-1].Position, views[index].Position
	return left + (right-left)/2
}

func managedView(id pgtype.UUID, name, viewType, scope string, locked bool, position float64,
	version int32, raw []byte, table rowTable,
) (View, error) {
	fields := make([]Field, len(table.fields))
	for i, field := range table.fields {
		fields[i] = Field{ID: field.id, IsSystem: field.system}
	}
	config, err := readViewColumns(raw, fields)
	if err != nil {
		return View{}, err
	}
	return View{ID: id.String(), Name: name, Type: viewType, Scope: scope, Locked: locked,
		Position: position, Version: version, Config: config}, nil
}

func viewVersionConflict() error {
	return apperr.New(409, "ERR_VERSION_CONFLICT", "The view changed; reload and retry")
}
