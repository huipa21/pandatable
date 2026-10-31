package structure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl"
	"pandatable/internal/acl/aclsql"
	"pandatable/internal/apperr"
	"pandatable/internal/codec"
	"pandatable/internal/ddl"
	"pandatable/internal/events"
	"pandatable/internal/structure/structuresql"
)

type TrashItem struct {
	Kind          string    `json:"kind"`
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	WorkspaceID   string    `json:"workspaceId"`
	WorkspaceName string    `json:"workspaceName"`
	ParentID      string    `json:"parentId"`
	ParentName    string    `json:"parentName"`
	DeletedAt     time.Time `json:"deletedAt"`
	ExpiresAt     time.Time `json:"expiresAt"`
	scope         string
}

type TrashListing struct {
	Items []TrashItem `json:"items"`
}

type TrashRestoreOptions struct {
	AllowOptional bool `json:"allowOptional"`
}

func trashRetention(ctx context.Context, conn aclsql.DBTX, now time.Time) (time.Time, time.Duration, error) {
	days, err := structuresql.New(conn).ReadTrashRetentionDays(ctx)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("read trash retention: %w", err)
	}
	if days < 1 || days > 3650 {
		return time.Time{}, 0, errors.New("trash retention is outside the supported bounds")
	}
	retention := time.Duration(days) * 24 * time.Hour
	return now.UTC().Add(-retention), retention, nil
}

func queryTrash(ctx context.Context, conn aclsql.DBTX, user pgtype.UUID, cutoff time.Time,
	retention time.Duration, kind string, id pgtype.UUID) ([]TrashItem, error) {
	rows, err := structuresql.New(conn).ListTrashItems(ctx, structuresql.ListTrashItemsParams{
		UserID: user, Cutoff: pgtype.Timestamptz{Time: cutoff, Valid: true}, ItemKind: kind, ItemID: id,
	})
	if err != nil {
		return nil, fmt.Errorf("list trash: %w", err)
	}
	items := make([]TrashItem, 0, len(rows))
	for _, row := range rows {
		if !row.DeletedAt.Valid {
			return nil, errors.New("trash item has no deletion time")
		}
		item := TrashItem{Kind: row.Kind, ID: row.ID.String(), Name: row.Name,
			WorkspaceID: row.WorkspaceID.String(), WorkspaceName: row.WorkspaceName,
			ParentID: row.ParentID.String(), ParentName: row.ParentName,
			DeletedAt: row.DeletedAt.Time.UTC(), scope: row.Scope}
		item.ExpiresAt = item.DeletedAt.Add(retention)
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) ListTrash(ctx context.Context, actor Actor) (TrashListing, error) {
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return TrashListing{}, acl.ErrValidation()
	}
	cutoff, retention, err := trashRetention(ctx, s.pool, time.Now())
	if err != nil {
		return TrashListing{}, err
	}
	items, err := queryTrash(ctx, s.pool, user, cutoff, retention, "", pgtype.UUID{})
	if err != nil {
		return TrashListing{}, err
	}
	return TrashListing{Items: items}, nil
}

func trashItem(ctx context.Context, conn aclsql.DBTX, user pgtype.UUID, kind string, id pgtype.UUID,
	now time.Time) (TrashItem, time.Time, error) {
	cutoff, retention, err := trashRetention(ctx, conn, now)
	if err != nil {
		return TrashItem{}, time.Time{}, err
	}
	items, err := queryTrash(ctx, conn, user, cutoff, retention, kind, id)
	if err != nil {
		return TrashItem{}, time.Time{}, err
	}
	if len(items) != 1 {
		return TrashItem{}, time.Time{}, acl.ErrNotFound()
	}
	return items[0], cutoff, nil
}

func trashParent(kind string) (acl.ResourceType, bool) {
	switch kind {
	case "workspace":
		return acl.Workspace, true
	case "book":
		return acl.Workspace, true
	case "table":
		return acl.Book, true
	case "field", "view":
		return acl.Table, true
	default:
		return "", false
	}
}

func trashDependencyError() error {
	return apperr.New(409, "ERR_TRASH_DEPENDENCY", "A required resource is unavailable or changed")
}

func (s *Service) RestoreTrash(ctx context.Context, actor Actor, kind, id string, input TrashRestoreOptions) error {
	parentKind, valid := trashParent(kind)
	itemID, ok := acl.ParseUUID(id)
	if !valid || !ok {
		return acl.ErrNotFound()
	}
	if input.AllowOptional && kind != "field" {
		return validation("Optional restoration applies only to fields")
	}
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return acl.ErrValidation()
	}
	item, _, err := trashItem(ctx, s.pool, user, kind, itemID, time.Now())
	if err != nil {
		return err
	}
	parentID, ok := acl.ParseUUID(item.ParentID)
	if !ok {
		return errors.New("trash item has invalid parent")
	}
	written := false
	err = s.pool.WithTx(ctx, func(tx pgx.Tx) error {
		queries := s.queries.WithTx(tx)
		if _, err := tx.Exec(ctx, lockTimeout); err != nil {
			return fmt.Errorf("set trash restoration lock timeout: %w", err)
		}
		if kind == "workspace" {
			if _, err := queries.LockTrashWorkspace(ctx, itemID); err != nil {
				return notFoundIfNoRows(err, "lock trashed workspace")
			}
		} else if _, err := acl.LockPath(ctx, tx, acl.Resource{Type: parentKind, ID: parentID}); err != nil {
			return err
		}
		if err := acl.LockActiveUser(ctx, tx, user); err != nil {
			return err
		}
		current, cutoff, err := trashItem(ctx, tx, user, kind, itemID, time.Now())
		if err != nil {
			return err
		}
		if current.ParentID != item.ParentID {
			return acl.ErrNotFound()
		}
		c := &txn{tx: tx, q: queries, user: user, actor: actor}
		switch kind {
		case "workspace":
			err = queries.RestoreTrashWorkspace(ctx, itemID)
		case "book":
			err = queries.RestoreTrashBook(ctx, itemID)
		case "table":
			if err = queries.RestoreTrashTable(ctx, itemID); err == nil {
				err = notifyRelatedSchemaChanges(ctx, tx, actor, itemID)
			}
		case "field":
			err = s.restoreTrashField(ctx, c, parentID, itemID, cutoff, input.AllowOptional)
		case "view":
			var version int32
			version, err = queries.RestoreTrashView(ctx, itemID)
			if err == nil {
				recipient := ""
				if current.scope == "personal" {
					recipient = actor.ID
				}
				err = events.Emit(ctx, tx, events.Event{Type: "view.changed", TableID: parentID.String(),
					ViewID: id, Version: version, By: actor.ID, Recipient: recipient})
			}
		}
		if err != nil {
			return fieldNameConflict(fmt.Errorf("restore %s: %w", kind, err))
		}
		written = true
		recipient := ""
		if kind == "view" && current.scope == "personal" {
			recipient = actor.ID
		}
		return events.Emit(ctx, tx, events.Event{Type: "nav.changed", By: actor.ID, Recipient: recipient})
	})
	if written {
		s.resolver.Invalidate()
	}
	return sanitizeLock(err)
}

func (s *Service) restoreTrashField(ctx context.Context, c *txn, tableID, fieldID pgtype.UUID,
	cutoff time.Time, allowOptional bool) error {
	table, err := lockTable(ctx, c, tableID)
	if err != nil {
		return err
	}
	field, err := c.q.LockTrashField(ctx, structuresql.LockTrashFieldParams{
		ID: fieldID, TableID: tableID, DeletedAt: pgtype.Timestamptz{Time: cutoff, Valid: true},
	})
	if err != nil {
		return notFoundIfNoRows(err, "lock trashed field")
	}
	kind, column, options, defaultJSON := field.UiType, field.PgColumn, field.Options, field.DefaultJson
	priorRequired, unique, system := field.TrashRequired, field.IsUnique, field.IsSystem
	required := priorRequired.Valid && priorRequired.Bool
	if system {
		return systemFieldError()
	}
	if kind == "link" {
		if allowOptional {
			return validation("Link fields cannot be restored as optional")
		}
		return s.restoreTrashRelation(ctx, c, table, fieldID, cutoff)
	}
	if kind == "formula" || isComputedKind(kind) {
		if allowOptional {
			return validation("Computed fields cannot be restored as optional")
		}
		if err := validateRestoredComputed(ctx, c, table, fieldID, kind, options); err != nil {
			return err
		}
	} else {
		if !column.Valid {
			return errors.New("trashed stored field has no physical column")
		}
		cleanOptions, err := fieldOptionsForCodec(options)
		if err != nil {
			return err
		}
		bound, err := codec.Bind(codec.Kind(kind), cleanOptions)
		if err != nil {
			return fmt.Errorf("bind trashed field options: %w", err)
		}
		unknownRequired := !priorRequired.Valid && bound.Constraints().Required
		if unknownRequired && !allowOptional {
			return apperr.New(409, "ERR_TRASH_REQUIRED_UNKNOWN",
				"The previous required setting was not recorded; restore as optional or cancel")
		}
		value, err := resolveDefault(bound, defaultJSON)
		if err != nil {
			return fmt.Errorf("validate trashed field default: %w", err)
		}
		if err := checkConstraints(bound, required, unique, value.stored != nil); err != nil {
			return err
		}
		if err := checkTimezone(ctx, c.q, bound); err != nil {
			return err
		}
		physical := physicalColumn{table: table.physical, column: column.String}
		if bound.Column().AutoNumber {
			if allowOptional {
				return validation("Autonumber fields cannot be restored as optional")
			}
			if err := ddl.RestoreAutoNumberColumn(ctx, c.tx, table.physical, column.String); err != nil {
				return err
			}
		} else {
			if required {
				empty, err := physical.emptyCount(ctx, c.tx)
				if err != nil {
					return err
				}
				if empty > 0 && !allowOptional {
					return countError("ERR_REQUIRED_HAS_EMPTY", "Some rows are empty; restore as optional or cancel", empty)
				}
				if empty > 0 {
					required = false
				}
			} else if allowOptional && !unknownRequired {
				return validation("This field was not required")
			}
			if err := ddl.SetDefault(ctx, c.tx, table.physical, column.String, bound.Column().Type, value.ddl); err != nil {
				return err
			}
			if required && !bound.Column().NotNull {
				if err := ddl.SetNotNull(ctx, c.tx, table.physical, column.String, true); err != nil {
					return err
				}
			}
		}
	}
	if err := c.q.RestoreTrashField(ctx, structuresql.RestoreTrashFieldParams{
		ID: fieldID, IsRequired: required,
	}); err != nil {
		return fieldNameConflict(fmt.Errorf("restore field metadata: %w", err))
	}
	if err := c.q.BumpSchemaVersion(ctx, tableID); err != nil {
		return fmt.Errorf("bump restored field schema version: %w", err)
	}
	return emitSchema(ctx, c.tx, c.actor, tableID)
}

func validateRestoredComputed(ctx context.Context, c *txn, table lockedTable, fieldID pgtype.UUID,
	kind string, options []byte) error {
	current, err := loadRowTable(ctx, c.q, c.tx, table.ID, table.physical.Schema, table.physical.Name, table.SchemaVersion)
	if err != nil {
		return err
	}
	if kind == "formula" {
		var stored formulaOptions
		if err := json.Unmarshal(options, &stored); err != nil || stored.AST == nil {
			return errors.New("invalid trashed formula options")
		}
		result, err := inferFormulaType(stored.AST, current, fieldID.String(), nil)
		if err != nil || result != stored.ResultType {
			return trashDependencyError()
		}
		if err := validateFormulaGraph(ctx, c.tx, current, fieldID.String(), stored.AST); err != nil {
			return trashDependencyError()
		}
		return nil
	}
	stored, err := decodeComputedOptions(kind, options)
	if err != nil {
		return fmt.Errorf("decode trashed computed field: %w", err)
	}
	relationField, ok := current.byID[stored.RelationFieldID]
	if !ok || relationField.relation == nil {
		return trashDependencyError()
	}
	if _, err := computedTarget(ctx, c.tx, current, kind, options, relationField); err != nil {
		var app *apperr.Error
		if errors.As(err, &app) {
			return trashDependencyError()
		}
		return err
	}
	return nil
}

func (s *Service) restoreTrashRelation(ctx context.Context, c *txn, table lockedTable,
	fieldID pgtype.UUID, cutoff time.Time) error {
	relation, err := relationForAnyField(ctx, c.tx, fieldID)
	if errors.Is(err, pgx.ErrNoRows) {
		return trashDependencyError()
	}
	if err != nil {
		return err
	}
	otherID := relation.parentTable
	if otherID == table.ID {
		otherID = relation.childTable
	}
	if otherID != table.ID {
		target := acl.Resource{Type: acl.Table, ID: otherID}
		if _, err := acl.LockPath(ctx, c.tx, target); err != nil {
			var app *apperr.Error
			if errors.As(err, &app) && app.Status == 404 {
				return trashDependencyError()
			}
			return err
		}
		if err := s.authorizeRows(ctx, c.tx, c.user, target, acl.Owner); err != nil {
			return err
		}
		if _, err := lockTable(ctx, c, otherID); err != nil {
			return err
		}
	}
	available, err := c.q.CountRestorableRelationFields(ctx, structuresql.CountRestorableRelationFieldsParams{
		Ids:    []pgtype.UUID{relation.parentField, relation.childField},
		Cutoff: pgtype.Timestamptz{Time: cutoff, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("check trashed relation fields: %w", err)
	}
	if available != 2 {
		return trashDependencyError()
	}
	if err := validateRelationStorage(ctx, c.tx, relation); err != nil {
		return err
	}
	if err := c.q.RestoreTrashRelationFields(ctx, []pgtype.UUID{relation.parentField, relation.childField}); err != nil {
		return fieldNameConflict(fmt.Errorf("restore relation fields: %w", err))
	}
	for _, id := range []pgtype.UUID{table.ID, otherID} {
		if err := c.q.BumpSchemaVersion(ctx, id); err != nil {
			return fmt.Errorf("bump restored relation schema version: %w", err)
		}
		if err := emitSchema(ctx, c.tx, c.actor, id); err != nil {
			return err
		}
		if otherID == table.ID {
			break
		}
	}
	return nil
}

func validateRelationStorage(ctx context.Context, tx pgx.Tx, relation relationRecord) error {
	var exists bool
	queries := structuresql.New(tx)
	switch {
	case relation.fkColumn.Valid && !relation.junction.Valid:
		storage, err := queries.ReadTrashRelationTable(ctx, relation.childTable)
		if err != nil {
			return notFoundIfNoRows(err, "read trashed relation storage")
		}
		table := ddl.TableName{Schema: storage.PgSchema, Name: storage.PgName}
		if err := ddl.ValidateIdentifier(ddl.Book, table.Schema); err != nil {
			return fmt.Errorf("validate trashed relation schema: %w", err)
		}
		if err := ddl.ValidateIdentifier(ddl.Table, table.Name); err != nil {
			return fmt.Errorf("validate trashed relation table: %w", err)
		}
		if err := ddl.ValidateIdentifier(ddl.Column, relation.fkColumn.String); err != nil {
			return fmt.Errorf("validate trashed relation column: %w", err)
		}
		err = tx.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM pg_catalog.pg_attribute a
			JOIN pg_catalog.pg_class c ON c.oid=a.attrelid
			JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
			WHERE n.nspname=$1 AND c.relname=$2 AND c.relkind IN ('r','p')
			  AND a.attname=$3 AND a.attnum>0 AND NOT a.attisdropped)`,
			table.Schema, table.Name, relation.fkColumn.String).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check trashed relation column: %w", err)
		}
	case relation.junction.Valid && !relation.fkColumn.Valid:
		schema, err := queries.ReadTrashRelationSchema(ctx, relation.bookID)
		if err != nil {
			return notFoundIfNoRows(err, "read trashed relation schema")
		}
		if err := ddl.ValidateIdentifier(ddl.Book, schema); err != nil {
			return fmt.Errorf("validate trashed relation schema: %w", err)
		}
		if err := ddl.ValidateIdentifier(ddl.Junction, relation.junction.String); err != nil {
			return fmt.Errorf("validate trashed junction: %w", err)
		}
		err = tx.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM pg_catalog.pg_class c
			JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
			WHERE n.nspname=$1 AND c.relname=$2 AND c.relkind IN ('r','p')
			  AND (SELECT count(*) FROM pg_catalog.pg_attribute a
			       WHERE a.attrelid=c.oid AND a.attname IN ('parent_id','child_id')
			         AND a.attnum>0 AND NOT a.attisdropped)=2)`,
			schema, relation.junction.String).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check trashed junction: %w", err)
		}
	default:
		return trashDependencyError()
	}
	if !exists {
		return trashDependencyError()
	}
	return nil
}
