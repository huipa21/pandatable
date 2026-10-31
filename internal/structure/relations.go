package structure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl"
	"pandatable/internal/apperr"
	"pandatable/internal/ddl"
	"pandatable/internal/structure/structuresql"
)

type Relation struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	ParentTableID string `json:"parentTableId"`
	ChildTableID  string `json:"childTableId"`
	ParentFieldID string `json:"parentFieldId"`
	ChildFieldID  string `json:"childFieldId"`
	OnDelete      string `json:"onDelete"`
}

type RelationInput struct {
	Name          string
	ReverseName   string
	TargetTableID string
	Kind          string
	SourceSide    string
	OnDelete      string
}

type RelationResult struct {
	Field    *Field      `json:"field,omitempty"`
	Schema   TableSchema `json:"schema"`
	Relation Relation    `json:"relation"`
}

type LinkChange struct {
	Add     []string
	Remove  []string
	Replace bool
}

type relationRecord struct {
	id, kind, onDelete string
	bookID             pgtype.UUID
	parentTable        pgtype.UUID
	childTable         pgtype.UUID
	parentField        pgtype.UUID
	childField         pgtype.UUID
	fkColumn           pgtype.Text
	junction           pgtype.Text
}

func relationDTO(row relationRecord) Relation {
	return Relation{
		ID: row.id, Kind: row.kind, ParentTableID: row.parentTable.String(),
		ChildTableID: row.childTable.String(), ParentFieldID: row.parentField.String(),
		ChildFieldID: row.childField.String(), OnDelete: row.onDelete,
	}
}

func relationRows(ctx context.Context, tx pgx.Tx, tableID pgtype.UUID) ([]relationRecord, error) {
	rows, err := tx.Query(ctx, `SELECT id::text, book_id, kind, parent_table_id, child_table_id,
		parent_field_id, child_field_id, fk_column, junction_table, on_delete
		FROM meta.relations WHERE parent_table_id=$1 OR child_table_id=$1 ORDER BY id`, tableID)
	if err != nil {
		return nil, fmt.Errorf("list table relations: %w", err)
	}
	defer rows.Close()
	result := make([]relationRecord, 0)
	for rows.Next() {
		var row relationRecord
		if err := rows.Scan(&row.id, &row.bookID, &row.kind, &row.parentTable, &row.childTable,
			&row.parentField, &row.childField, &row.fkColumn, &row.junction, &row.onDelete); err != nil {
			return nil, fmt.Errorf("scan table relation: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate table relations: %w", err)
	}
	return result, nil
}

func relationForField(ctx context.Context, tx pgx.Tx, fieldID pgtype.UUID) (relationRecord, error) {
	var row relationRecord
	err := tx.QueryRow(ctx, `SELECT id::text, book_id, kind, parent_table_id, child_table_id,
		parent_field_id, child_field_id, fk_column, junction_table, on_delete
		FROM meta.relations WHERE parent_field_id=$1 OR child_field_id=$1`, fieldID).
		Scan(&row.id, &row.bookID, &row.kind, &row.parentTable, &row.childTable,
			&row.parentField, &row.childField, &row.fkColumn, &row.junction, &row.onDelete)
	if errors.Is(err, pgx.ErrNoRows) {
		return relationRecord{}, pgx.ErrNoRows
	}
	if err != nil {
		return relationRecord{}, fmt.Errorf("read field relation: %w", err)
	}
	return row, nil
}

func cleanRelationInput(input RelationInput) (string, string, string, error) {
	name, err := cleanName(input.Name)
	if err != nil {
		return "", "", "", err
	}
	reverse, err := cleanName(input.ReverseName)
	if err != nil {
		return "", "", "", err
	}
	kind := input.Kind
	if kind != "one_one" && kind != "one_many" && kind != "many_many" {
		return "", "", "", validation("Relation kind is invalid")
	}
	onDelete := input.OnDelete
	if onDelete == "" {
		onDelete = "set_null"
	}
	if onDelete != "set_null" && onDelete != "cascade" && onDelete != "restrict" {
		return "", "", "", validation("Relation delete behavior is invalid")
	}
	sourceSide := input.SourceSide
	if sourceSide == "" {
		sourceSide = "parent"
	}
	if sourceSide != "parent" && sourceSide != "child" {
		return "", "", "", validation("Relation source side is invalid")
	}
	if kind == "many_many" && sourceSide != "parent" {
		return "", "", "", validation("Many-to-many source side must be parent")
	}
	return name, reverse, onDelete, nil
}

func insertRelationField(ctx context.Context, tx pgx.Tx, tableID pgtype.UUID, name string, position float64) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := tx.QueryRow(ctx, `INSERT INTO meta.fields
		(table_id,name,ui_type,pg_column,pg_type,options,is_system,is_required,is_unique,position)
		VALUES ($1,$2,'link',NULL,NULL,'{}',false,false,false,$3) RETURNING id`,
		tableID, name, position).Scan(&id)
	if err != nil {
		return pgtype.UUID{}, fieldNameConflict(fmt.Errorf("insert relation field: %w", err))
	}
	return id, nil
}

func (s *Service) CreateRelation(ctx context.Context, actor Actor, tableID string, input RelationInput) (RelationResult, error) {
	source, err := resourceOf(acl.Table, tableID)
	if err != nil {
		return RelationResult{}, err
	}
	target, err := resourceOf(acl.Table, input.TargetTableID)
	if err != nil {
		return RelationResult{}, err
	}
	name, reverseName, onDelete, err := cleanRelationInput(input)
	if err != nil {
		return RelationResult{}, err
	}
	if name == reverseName && source.ID == target.ID {
		return RelationResult{}, validation("Self-relation fields must have different names")
	}
	sourceSide := input.SourceSide
	if sourceSide == "" {
		sourceSide = "parent"
	}

	var result RelationResult
	err = retryCollisions(func() error {
		return s.run(ctx, actor, scope{target: source, schemaChanged: true}, func(c *txn) error {
			sourceTable, err := lockTable(ctx, c, source.ID)
			if err != nil {
				return err
			}
			targetTable := sourceTable
			targetBookID := pgtype.UUID{}
			if source.ID != target.ID {
				targetTable, err = lockTable(ctx, c, target.ID)
				if err != nil {
					return err
				}
				targetMeta, err := c.q.GetTable(ctx, target.ID)
				if err != nil {
					return notFoundIfNoRows(err, "read relation target table")
				}
				targetBookID = targetMeta.BookID
			} else {
				sourceMeta, err := c.q.GetTable(ctx, source.ID)
				if err != nil {
					return notFoundIfNoRows(err, "read relation source table")
				}
				targetBookID = sourceMeta.BookID
			}
			sourceMeta, err := c.q.GetTable(ctx, source.ID)
			if err != nil {
				return notFoundIfNoRows(err, "read relation source table")
			}
			if sourceMeta.BookID != targetBookID || sourceTable.physical.Schema != targetTable.physical.Schema {
				return validation("Relation targets must be in the same live book")
			}
			if source.ID != target.ID {
				if _, err := acl.LockPath(ctx, c.tx, target); err != nil {
					return err
				}
				access, err := acl.AccessUncached(ctx, c.tx, c.user, target)
				if err != nil {
					return err
				}
				if !access.Visible {
					return acl.ErrNotFound()
				}
				if err := access.Check(acl.Owner); err != nil {
					return err
				}
			}

			parentTable, childTable := sourceTable, targetTable
			parentTableID, childTableID := source.ID, target.ID
			parentName, childName := name, reverseName
			if (input.Kind == "one_many" || input.Kind == "one_one") && sourceSide == "child" {
				parentTable, childTable = targetTable, sourceTable
				parentTableID, childTableID = target.ID, source.ID
				parentName, childName = reverseName, name
			}
			parentPosition, err := c.q.MaxUserFieldPosition(ctx, parentTableID)
			if err != nil {
				return fmt.Errorf("read parent relation field position: %w", err)
			}
			parentFieldID, err := insertRelationField(ctx, c.tx, parentTableID, parentName, parentPosition+positionStep)
			if err != nil {
				return err
			}
			childPosition, err := c.q.MaxUserFieldPosition(ctx, childTableID)
			if err != nil {
				return fmt.Errorf("read child relation field position: %w", err)
			}
			if parentTableID == childTableID {
				childPosition += positionStep
			}
			childFieldID, err := insertRelationField(ctx, c.tx, childTableID, childName, childPosition+positionStep)
			if err != nil {
				return err
			}

			var fkColumn, junction any
			switch input.Kind {
			case "one_one", "one_many":
				fkColumn, err = s.newName(ddl.Column)
				if err != nil {
					return err
				}
				action := map[string]ddl.DeleteAction{
					"set_null": ddl.SetNull, "cascade": ddl.Cascade, "restrict": ddl.Restrict,
				}[onDelete]
				if err := ddl.AddRelationColumn(ctx, c.tx, childTable.physical, fkColumn.(string),
					parentTable.physical, input.Kind == "one_one", action); err != nil {
					return err
				}
			case "many_many":
				junctionName, err := s.newName(ddl.Junction)
				if err != nil {
					return err
				}
				junction = junctionName
				if err := ddl.CreateJunction(ctx, c.tx, parentTable.physical.Schema, junctionName,
					parentTable.physical, childTable.physical); err != nil {
					return err
				}
			}
			var relationID pgtype.UUID
			err = c.tx.QueryRow(ctx, `INSERT INTO meta.relations
				(book_id,kind,parent_table_id,child_table_id,parent_field_id,child_field_id,fk_column,junction_table,on_delete)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
				sourceMeta.BookID, input.Kind, parentTableID, childTableID, parentFieldID,
				childFieldID, fkColumn, junction, onDelete).Scan(&relationID)
			if err != nil {
				return fmt.Errorf("insert relation metadata: %w", err)
			}
			parentOptions, _ := json.Marshal(map[string]string{
				"relationId": relationID.String(), "targetTableId": childTableID.String(), "side": "parent",
			})
			childOptions, _ := json.Marshal(map[string]string{
				"relationId": relationID.String(), "targetTableId": parentTableID.String(), "side": "child",
			})
			for _, update := range []struct {
				id      pgtype.UUID
				options []byte
			}{{parentFieldID, parentOptions}, {childFieldID, childOptions}} {
				if _, err := c.tx.Exec(ctx, "UPDATE meta.fields SET options=$2::jsonb WHERE id=$1", update.id, update.options); err != nil {
					return fmt.Errorf("store relation field options: %w", err)
				}
			}
			if _, err := c.tx.Exec(ctx, "UPDATE meta.tables SET schema_version=schema_version+1 WHERE id=$1 OR id=$2",
				parentTableID, childTableID); err != nil {
				return fmt.Errorf("bump relation schema versions: %w", err)
			}
			schema, err := loadSchema(ctx, c.q, c.tx, source.ID, c.user, acl.Owner)
			if err != nil {
				return err
			}
			relation := Relation{ID: relationID.String(), Kind: input.Kind, ParentTableID: parentTableID.String(),
				ChildTableID: childTableID.String(), ParentFieldID: parentFieldID.String(),
				ChildFieldID: childFieldID.String(), OnDelete: onDelete}
			var fieldID pgtype.UUID
			if parentTableID == source.ID {
				fieldID = parentFieldID
			} else {
				fieldID = childFieldID
			}
			var created *Field
			for index := range schema.Fields {
				if schema.Fields[index].ID == fieldID.String() {
					created = &schema.Fields[index]
					break
				}
			}
			if created == nil {
				return errors.New("created relation field missing from schema")
			}
			result = RelationResult{Field: created, Schema: schema, Relation: relation}
			if target.ID != source.ID {
				return emitSchema(ctx, c.tx, actor, target.ID)
			}
			return nil
		})
	})
	return result, err
}

func lockRelationTables(ctx context.Context, c *txn, relation relationRecord, sourceID pgtype.UUID) (lockedTable, lockedTable, error) {
	if sourceID != relation.parentTable && sourceID != relation.childTable {
		return lockedTable{}, lockedTable{}, errors.New("source table is outside relation")
	}
	source, err := lockTable(ctx, c, sourceID)
	if err != nil {
		return lockedTable{}, lockedTable{}, err
	}
	otherID := relation.parentTable
	if sourceID == relation.parentTable {
		otherID = relation.childTable
	}
	other := source
	if otherID != sourceID {
		other, err = lockTable(ctx, c, otherID)
		if err != nil {
			return lockedTable{}, lockedTable{}, err
		}
	}
	if sourceID == relation.parentTable {
		return source, other, nil
	}
	return other, source, nil
}

func (s *Service) deleteRelationField(ctx context.Context, c *txn, source lockedTable, field structuresql.GetFieldRow) (FieldResult, error) {
	relation, err := relationForField(ctx, c.tx, field.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return FieldResult{}, errors.New("link field has no relation metadata")
	}
	if err != nil {
		return FieldResult{}, err
	}
	otherID := relation.parentTable
	if relation.parentTable == source.ID {
		otherID = relation.childTable
	}
	other := source
	if otherID != source.ID {
		other, err = lockTable(ctx, c, otherID)
		if err != nil {
			return FieldResult{}, err
		}
		target := acl.Resource{Type: acl.Table, ID: otherID}
		if _, err := acl.LockPath(ctx, c.tx, target); err != nil {
			return FieldResult{}, err
		}
		access, err := acl.AccessUncached(ctx, c.tx, c.user, target)
		if err != nil {
			return FieldResult{}, err
		}
		if !access.Visible {
			return FieldResult{}, acl.ErrNotFound()
		}
		if err := access.Check(acl.Owner); err != nil {
			return FieldResult{}, err
		}
	}
	parentTable, childTable := source, other
	if relation.parentTable != source.ID {
		parentTable, childTable = other, source
	}
	if relation.fkColumn.Valid {
		if err := ddl.DropColumn(ctx, c.tx, childTable.physical, relation.fkColumn.String); err != nil {
			return FieldResult{}, err
		}
	} else if relation.junction.Valid {
		if err := ddl.DropJunction(ctx, c.tx, parentTable.physical.Schema, relation.junction.String); err != nil {
			return FieldResult{}, err
		}
	} else {
		return FieldResult{}, errors.New("relation metadata has no physical storage")
	}
	for _, fieldID := range []pgtype.UUID{relation.parentField, relation.childField} {
		if err := c.q.TrashField(ctx, fieldID); err != nil {
			return FieldResult{}, fmt.Errorf("trash relation field: %w", err)
		}
	}
	if _, err := c.tx.Exec(ctx, "DELETE FROM meta.relations WHERE id=$1::uuid", relation.id); err != nil {
		return FieldResult{}, fmt.Errorf("delete relation metadata: %w", err)
	}
	if _, err := c.tx.Exec(ctx, "UPDATE meta.tables SET schema_version=schema_version+1 WHERE id=$1 OR id=$2",
		relation.parentTable, relation.childTable); err != nil {
		return FieldResult{}, fmt.Errorf("bump relation schema versions: %w", err)
	}
	schema, err := loadSchema(ctx, c.q, c.tx, source.ID, c.user, acl.Owner)
	if err != nil {
		return FieldResult{}, err
	}
	if otherID != source.ID {
		if err := emitSchema(ctx, c.tx, c.actor, otherID); err != nil {
			return FieldResult{}, err
		}
	}
	return FieldResult{Schema: schema}, nil
}

func fieldRelationOptions(relationID, targetTableID, side string) []byte {
	value, _ := json.Marshal(map[string]string{
		"relationId": relationID, "targetTableId": targetTableID, "side": side,
	})
	return value
}

func sameName(a, b string) bool { return strings.EqualFold(a, b) }

func relationRestrictError() error {
	return apperr.New(409, "ERR_RESTRICT_VIOLATION", "This row is linked and cannot be deleted")
}

func relationConflictError(rowID string) error {
	err := apperr.New(409, "ERR_RELATION_CONFLICT", "This row is already linked")
	err.Details = map[string]any{"relatedRowId": rowID}
	return err
}

func relationIDs(values []string) ([]int64, error) {
	if len(values) > maxBulkRows {
		return nil, validation("Relation link count is outside the supported bounds")
	}
	result := make([]int64, len(values))
	seen := make(map[int64]bool, len(values))
	for index, value := range values {
		id, err := canonicalRowID(value)
		if err != nil {
			return nil, err
		}
		if seen[id] {
			return nil, validation("Relation row ids must be unique")
		}
		seen[id] = true
		result[index] = id
	}
	return result, nil
}

func lockRelationRows(ctx context.Context, tx pgx.Tx, table rowTable, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	query := "SELECT \"id\" FROM " + table.qualifiedName() +
		" WHERE \"id\"=ANY($1::bigint[]) ORDER BY \"id\" FOR UPDATE"
	rows, err := tx.Query(ctx, query, ids)
	if err != nil {
		return fmt.Errorf("lock relation rows: %w", err)
	}
	count := 0
	for rows.Next() {
		count++
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate locked relation rows: %w", err)
	}
	rows.Close()
	if count != len(ids) {
		return acl.ErrNotFound()
	}
	return nil
}

func relationExec(ctx context.Context, tx pgx.Tx, query string, args ...any) error {
	if _, err := tx.Exec(ctx, query, args...); err != nil {
		var postgres *pgconn.PgError
		if errors.As(err, &postgres) && postgres.Code == "23505" {
			return apperr.New(409, "ERR_RELATION_CONFLICT", "This row is already linked")
		}
		return fmt.Errorf("change relation links: %w", err)
	}
	return nil
}

func updateForeignLinks(ctx context.Context, tx pgx.Tx, table rowTable, column string, ids []int64,
	target *int64, actor pgtype.UUID) error {
	if err := ddl.ValidateIdentifier(ddl.Column, column); err != nil {
		return err
	}
	value := any(nil)
	if target != nil {
		value = *target
	}
	query := "UPDATE " + table.qualifiedName() + " SET " + pgx.Identifier{column}.Sanitize() +
		"=$1, \"updated_at\"=transaction_timestamp(), \"updated_by\"=$2 WHERE \"id\"=ANY($3::bigint[])"
	return relationExec(ctx, tx, query, value, actor, ids)
}

func rowLinkValue(ctx context.Context, tx pgx.Tx, table rowTable, id int64, column string) (*int64, error) {
	if err := ddl.ValidateIdentifier(ddl.Column, column); err != nil {
		return nil, err
	}
	var value *int64
	query := "SELECT " + pgx.Identifier{column}.Sanitize() + " FROM " + table.qualifiedName() + ` WHERE "id"=$1`
	if err := tx.QueryRow(ctx, query, id).Scan(&value); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, acl.ErrNotFound()
		}
		return nil, fmt.Errorf("read relation link: %w", err)
	}
	return value, nil
}

func relatedRowFor(ctx context.Context, tx pgx.Tx, table rowTable, column string, id int64) (int64, bool, error) {
	if err := ddl.ValidateIdentifier(ddl.Column, column); err != nil {
		return 0, false, err
	}
	var related int64
	query := "SELECT \"id\" FROM " + table.qualifiedName() + " WHERE " +
		pgx.Identifier{column}.Sanitize() + "=$1 ORDER BY \"id\" LIMIT 1"
	err := tx.QueryRow(ctx, query, id).Scan(&related)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("read conflicting relation row: %w", err)
	}
	return related, true, nil
}

func (s *Service) ChangeLinks(ctx context.Context, actor Actor, tableID, rowID, fieldID string, change LinkChange) (RowRecord, error) {
	sourceID, err := canonicalRowID(rowID)
	if err != nil {
		return RowRecord{}, err
	}
	fieldUUID, ok := acl.ParseUUID(fieldID)
	if !ok || fieldUUID.String() != fieldID {
		return RowRecord{}, validation("Relation field id is invalid")
	}
	add, err := relationIDs(change.Add)
	if err != nil {
		return RowRecord{}, err
	}
	remove, err := relationIDs(change.Remove)
	if err != nil {
		return RowRecord{}, err
	}
	for _, added := range add {
		for _, removed := range remove {
			if added == removed {
				return RowRecord{}, validation("Relation ids cannot be added and removed together")
			}
		}
	}
	if len(add)+len(remove) == 0 {
		return RowRecord{}, validation("At least one relation link change is required")
	}

	var result RowRecord
	err = s.withWritableTable(ctx, actor, tableID, func(tx pgx.Tx, table rowTable) error {
		field, found := table.byID[fieldID]
		if !found || field.relation == nil {
			return validation("Field is not a live relation on this table")
		}
		relation := *field.relation
		if (relation.kind == "one_one" || relation.kind == "one_many" && field.relationSide == "child") && len(add) > 1 {
			return validation("This relation field accepts one linked row")
		}
		targetTableID := relation.parentTable
		if field.relationSide == "parent" {
			targetTableID = relation.childTable
		}
		targetTable := table
		if targetTableID != table.id {
			target := acl.Resource{Type: acl.Table, ID: targetTableID}
			if _, err := acl.LockPath(ctx, tx, target); err != nil {
				return err
			}
			user, valid := acl.ParseUUID(actor.ID)
			if !valid {
				return acl.ErrValidation()
			}
			if err := s.authorizeRows(ctx, tx, user, target, acl.Write); err != nil {
				return err
			}
			targetMeta, err := structuresql.New(tx).LockRowTable(ctx, targetTableID)
			if err != nil {
				return notFoundIfNoRows(err, "lock related row table")
			}
			targetTable, err = loadRowTable(ctx, structuresql.New(tx), tx, targetTableID,
				targetMeta.PgSchema, targetMeta.PgName, targetMeta.SchemaVersion)
			if err != nil {
				return err
			}
		}
		if relation.kind == "many_many" && !relation.junction.Valid ||
			relation.kind != "many_many" && !relation.fkColumn.Valid {
			return errors.New("relation metadata has invalid physical storage")
		}

		otherIDs := append(append([]int64{}, add...), remove...)
		if table.id == targetTable.id {
			otherIDs = append(otherIDs, sourceID)
			otherIDs = uniqueSortedIDs(otherIDs)
			if err := lockRelationRows(ctx, tx, table, otherIDs); err != nil {
				return err
			}
		} else if table.id.String() < targetTable.id.String() {
			if err := lockRelationRows(ctx, tx, table, []int64{sourceID}); err != nil {
				return err
			}
			if err := lockRelationRows(ctx, tx, targetTable, otherIDs); err != nil {
				return err
			}
		} else {
			if err := lockRelationRows(ctx, tx, targetTable, otherIDs); err != nil {
				return err
			}
			if err := lockRelationRows(ctx, tx, table, []int64{sourceID}); err != nil {
				return err
			}
		}

		user, valid := acl.ParseUUID(actor.ID)
		if !valid {
			return acl.ErrValidation()
		}
		if relation.kind == "many_many" {
			if err := changeJunctionLinks(ctx, tx, relation, field.relationSide, sourceID, add, remove, user); err != nil {
				return err
			}
		} else if relation.kind == "one_one" {
			if err := changeOneToOneLinks(ctx, tx, table, targetTable, relation, field.relationSide,
				sourceID, add, remove, change.Replace, user); err != nil {
				return err
			}
		} else if err := changeOneToManyLinks(ctx, tx, table, relation, field.relationSide,
			sourceID, add, remove, user); err != nil {
			return err
		}

		query, args, err := sq.Select(table.selectColumnList(table.fields)...).From(table.qualifiedName()).
			Where(sq.Eq{pgx.Identifier{"id"}.Sanitize(): sourceID}).
			PlaceholderFormat(sq.Dollar).ToSql()
		if err != nil {
			return fmt.Errorf("build updated relation row query: %w", err)
		}
		record, err := scanRow(queryUncachedRow(ctx, tx, query, args...), table, table.fields)
		if errors.Is(err, pgx.ErrNoRows) {
			return acl.ErrNotFound()
		}
		if err != nil {
			return fmt.Errorf("read updated relation row: %w", err)
		}
		if err := enrichRowLinks(ctx, tx, table, table.fields, []RowRecord{record}); err != nil {
			return err
		}
		if err := enrichRowComputed(ctx, tx, actor, table, table.fields, []RowRecord{record}); err != nil {
			return err
		}
		result = record
		if err := emitRows(ctx, tx, actor, table.id.String(), "row.updated", []RowRecord{record}, []string{fieldID}); err != nil {
			return err
		}
		return emitRelatedRowInvalidation(ctx, tx, targetTable.id, table.id)
	})
	return result, err
}

func uniqueSortedIDs(ids []int64) []int64 {
	seen := make(map[int64]bool, len(ids))
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func changeJunctionLinks(ctx context.Context, tx pgx.Tx, relation relationRecord, side string,
	sourceID int64, add, remove []int64, actor pgtype.UUID) error {
	if err := ddl.ValidateIdentifier(ddl.Junction, relation.junction.String); err != nil {
		return err
	}
	schema, _, err := relationDisplayColumn(ctx, tx, relation.parentTable)
	if err != nil {
		return err
	}
	junction := pgx.Identifier{schema.Schema, relation.junction.String}.Sanitize()
	sourceColumn, targetColumn := "parent_id", "child_id"
	if side == "child" {
		sourceColumn, targetColumn = "child_id", "parent_id"
	}
	for _, id := range remove {
		query := "DELETE FROM " + junction + " WHERE " + pgx.Identifier{sourceColumn}.Sanitize() +
			"=$1 AND " + pgx.Identifier{targetColumn}.Sanitize() + "=$2"
		if _, err := tx.Exec(ctx, query, sourceID, id); err != nil {
			return fmt.Errorf("remove relation junction link: %w", err)
		}
	}
	for _, id := range add {
		query := "INSERT INTO " + junction + " (" + pgx.Identifier{sourceColumn}.Sanitize() + "," +
			pgx.Identifier{targetColumn}.Sanitize() + `,"created_by") VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`
		if _, err := tx.Exec(ctx, query, sourceID, id, actor); err != nil {
			return fmt.Errorf("add relation junction link: %w", err)
		}
	}
	return nil
}

func changeOneToManyLinks(ctx context.Context, tx pgx.Tx, table rowTable, relation relationRecord, side string,
	sourceID int64, add, remove []int64, actor pgtype.UUID) error {
	if err := ddl.ValidateIdentifier(ddl.Column, relation.fkColumn.String); err != nil {
		return err
	}
	child := table
	if side == "parent" {
		var err error
		child, err = relationRowTable(ctx, tx, relation.childTable)
		if err != nil {
			return err
		}
		for _, id := range remove {
			if err := updateForeignLinks(ctx, tx, child, relation.fkColumn.String, []int64{id}, nil, actor); err != nil {
				return err
			}
		}
		if len(add) > 0 {
			if err := updateForeignLinks(ctx, tx, child, relation.fkColumn.String, add, &sourceID, actor); err != nil {
				return err
			}
		}
		return nil
	}
	if len(remove) > 0 {
		current, err := rowLinkValue(ctx, tx, table, sourceID, relation.fkColumn.String)
		if err != nil {
			return err
		}
		if current != nil && containsID(remove, *current) {
			if err := updateForeignLinks(ctx, tx, table, relation.fkColumn.String, []int64{sourceID}, nil, actor); err != nil {
				return err
			}
		}
	}
	if len(add) > 0 {
		parent := add[0]
		if err := updateForeignLinks(ctx, tx, table, relation.fkColumn.String, []int64{sourceID}, &parent, actor); err != nil {
			return err
		}
	}
	return nil
}

func changeOneToOneLinks(ctx context.Context, tx pgx.Tx, table, target rowTable, relation relationRecord, side string,
	sourceID int64, add, remove []int64, replace bool, actor pgtype.UUID) error {
	if err := ddl.ValidateIdentifier(ddl.Column, relation.fkColumn.String); err != nil {
		return err
	}
	child := table
	if side == "parent" {
		child = target
	}
	for _, id := range remove {
		if side == "parent" {
			if err := tx.QueryRow(ctx, `SELECT "id" FROM `+child.qualifiedName()+` WHERE "id"=$1 AND `+
				pgx.Identifier{relation.fkColumn.String}.Sanitize()+`=$2`, id, sourceID).Scan(new(int64)); errors.Is(err, pgx.ErrNoRows) {
				continue
			} else if err != nil {
				return fmt.Errorf("check one-to-one link: %w", err)
			}
			if err := updateForeignLinks(ctx, tx, child, relation.fkColumn.String, []int64{id}, nil, actor); err != nil {
				return err
			}
		} else {
			current, err := rowLinkValue(ctx, tx, table, sourceID, relation.fkColumn.String)
			if err != nil {
				return err
			}
			if current != nil && *current == id {
				if err := updateForeignLinks(ctx, tx, table, relation.fkColumn.String, []int64{sourceID}, nil, actor); err != nil {
					return err
				}
			}
		}
	}
	if len(add) == 0 {
		return nil
	}
	if side == "parent" {
		childID := add[0]
		oldParent, err := rowLinkValue(ctx, tx, child, childID, relation.fkColumn.String)
		if err != nil {
			return err
		}
		if oldParent != nil && *oldParent != sourceID && !replace {
			return relationConflictError(strconv.FormatInt(*oldParent, 10))
		}
		if otherChild, exists, err := relatedRowFor(ctx, tx, child, relation.fkColumn.String, sourceID); err != nil {
			return err
		} else if exists && otherChild != childID && !replace {
			return relationConflictError(strconv.FormatInt(otherChild, 10))
		}
		if replace {
			if oldParent != nil && *oldParent != sourceID {
				if err := updateForeignLinks(ctx, tx, child, relation.fkColumn.String, []int64{childID}, nil, actor); err != nil {
					return err
				}
			}
			if otherChild, exists, err := relatedRowFor(ctx, tx, child, relation.fkColumn.String, sourceID); err != nil {
				return err
			} else if exists && otherChild != childID {
				if err := updateForeignLinks(ctx, tx, child, relation.fkColumn.String, []int64{otherChild}, nil, actor); err != nil {
					return err
				}
			}
		}
		if err := updateForeignLinks(ctx, tx, child, relation.fkColumn.String, []int64{childID}, &sourceID, actor); err != nil {
			return err
		}
		return nil
	}
	parentID := add[0]
	if linkedChild, exists, err := relatedRowFor(ctx, tx, child, relation.fkColumn.String, parentID); err != nil {
		return err
	} else if exists && linkedChild != sourceID && !replace {
		return relationConflictError(strconv.FormatInt(linkedChild, 10))
	}
	if replace {
		if linkedChild, exists, err := relatedRowFor(ctx, tx, child, relation.fkColumn.String, parentID); err != nil {
			return err
		} else if exists && linkedChild != sourceID {
			if err := updateForeignLinks(ctx, tx, child, relation.fkColumn.String, []int64{linkedChild}, nil, actor); err != nil {
				return err
			}
		}
	}
	return updateForeignLinks(ctx, tx, table, relation.fkColumn.String, []int64{sourceID}, &parentID, actor)
}

func relationRowTable(ctx context.Context, tx pgx.Tx, tableID pgtype.UUID) (rowTable, error) {
	row, err := structuresql.New(tx).LockRowTable(ctx, tableID)
	if err != nil {
		return rowTable{}, notFoundIfNoRows(err, "lock relation row table")
	}
	return loadRowTable(ctx, structuresql.New(tx), tx, tableID, row.PgSchema, row.PgName, row.SchemaVersion)
}

func relationReadTable(ctx context.Context, tx pgx.Tx, tableID pgtype.UUID) (rowTable, error) {
	row, err := structuresql.New(tx).GetRowTable(ctx, tableID)
	if err != nil {
		return rowTable{}, notFoundIfNoRows(err, "read relation row table")
	}
	return loadRowTable(ctx, structuresql.New(tx), tx, tableID, row.PgSchema, row.PgName, row.SchemaVersion)
}

func containsID(ids []int64, value int64) bool {
	for _, id := range ids {
		if id == value {
			return true
		}
	}
	return false
}

func linkedRowIDs(ctx context.Context, tx pgx.Tx, relation relationRecord, side string,
	table rowTable, ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var query string
	if relation.fkColumn.Valid {
		if side != "parent" {
			return nil, nil
		}
		if err := ddl.ValidateIdentifier(ddl.Column, relation.fkColumn.String); err != nil {
			return nil, err
		}
		query = "SELECT \"id\" FROM " + table.qualifiedName() + " WHERE " +
			pgx.Identifier{relation.fkColumn.String}.Sanitize() + "=ANY($1::bigint[]) ORDER BY \"id\""
	} else if relation.junction.Valid {
		if err := ddl.ValidateIdentifier(ddl.Junction, relation.junction.String); err != nil {
			return nil, err
		}
		sourceColumn, targetColumn := "parent_id", "child_id"
		if side == "child" {
			sourceColumn, targetColumn = "child_id", "parent_id"
		}
		parent, _, err := relationDisplayColumn(ctx, tx, relation.parentTable)
		if err != nil {
			return nil, err
		}
		query = "SELECT DISTINCT " + pgx.Identifier{"links", targetColumn}.Sanitize() +
			" FROM " + pgx.Identifier{parent.Schema, relation.junction.String}.Sanitize() +
			` AS "links" WHERE ` + pgx.Identifier{"links", sourceColumn}.Sanitize() +
			"=ANY($1::bigint[]) ORDER BY " + pgx.Identifier{"links", targetColumn}.Sanitize()
	} else {
		return nil, errors.New("relation metadata has no physical storage")
	}
	rows, err := tx.Query(ctx, query, ids)
	if err != nil {
		return nil, fmt.Errorf("list linked rows: %w", err)
	}
	defer rows.Close()
	result := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan linked row id: %w", err)
		}
		result = append(result, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate linked rows: %w", err)
	}
	return result, nil
}

func (s *Service) deleteRelatedRows(ctx context.Context, tx pgx.Tx, actor Actor,
	table rowTable, ids []int64, visited map[string]map[int64]bool) error {
	if len(ids) == 0 {
		return nil
	}
	tableID := table.id.String()
	if visited[tableID] == nil {
		visited[tableID] = make(map[int64]bool)
	}
	pending := make([]int64, 0, len(ids))
	for _, id := range ids {
		if !visited[tableID][id] {
			visited[tableID][id] = true
			pending = append(pending, id)
		}
	}
	if len(pending) == 0 {
		return nil
	}
	relations, err := relationRows(ctx, tx, table.id)
	if err != nil {
		return err
	}
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return acl.ErrValidation()
	}
	for _, relation := range relations {
		side := ""
		targetID := pgtype.UUID{}
		if relation.parentTable == table.id {
			side, targetID = "parent", relation.childTable
		} else if relation.childTable == table.id {
			side, targetID = "child", relation.parentTable
		}
		if side == "" {
			continue
		}
		if relation.fkColumn.Valid && side == "child" {
			if err := emitRelatedRowInvalidation(ctx, tx, targetID, table.id); err != nil {
				return err
			}
			continue
		}
		target, err := relationReadTable(ctx, tx, targetID)
		if err != nil {
			return err
		}
		linked, err := linkedRowIDs(ctx, tx, relation, side, target, pending)
		if err != nil {
			return err
		}
		if relation.onDelete == "restrict" && len(linked) > 0 {
			return relationRestrictError()
		}
		if relation.onDelete == "cascade" && len(linked) > 0 {
			targetResource := acl.Resource{Type: acl.Table, ID: targetID}
			if _, err := acl.LockPath(ctx, tx, targetResource); err != nil {
				return err
			}
			if err := s.authorizeRows(ctx, tx, user, targetResource, acl.Write); err != nil {
				return err
			}
			target, err = relationRowTable(ctx, tx, targetID)
			if err != nil {
				return err
			}
			if err := lockRelationRows(ctx, tx, target, linked); err != nil {
				return err
			}
			if err := s.deleteRelatedRows(ctx, tx, actor, target, linked, visited); err != nil {
				return err
			}
			if err := deletePhysicalRows(ctx, tx, target, linked); err != nil {
				return err
			}
		}
		if relation.fkColumn.Valid && relation.onDelete == "set_null" && len(linked) > 0 {
			targetResource := acl.Resource{Type: acl.Table, ID: targetID}
			if _, err := acl.LockPath(ctx, tx, targetResource); err != nil {
				return err
			}
			if err := s.authorizeRows(ctx, tx, user, targetResource, acl.Write); err != nil {
				return err
			}
			target, err = relationRowTable(ctx, tx, targetID)
			if err != nil {
				return err
			}
			if err := lockRelationRows(ctx, tx, target, linked); err != nil {
				return err
			}
			if err := updateForeignLinks(ctx, tx, target, relation.fkColumn.String, linked, nil, user); err != nil {
				return err
			}
		}
		if len(linked) > 0 {
			if err := emitRelatedRowInvalidation(ctx, tx, targetID, table.id); err != nil {
				return err
			}
		}
	}
	return nil
}

func deletePhysicalRows(ctx context.Context, tx pgx.Tx, table rowTable, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	query := "DELETE FROM " + table.qualifiedName() + ` WHERE "id"=ANY($1::bigint[])`
	if _, err := tx.Exec(ctx, query, ids); err != nil {
		return fmt.Errorf("delete linked rows: %w", err)
	}
	return nil
}

type LinkValue struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

func relationDisplayColumn(ctx context.Context, tx pgx.Tx, tableID pgtype.UUID) (ddl.TableName, string, error) {
	var table ddl.TableName
	var column pgtype.Text
	err := tx.QueryRow(ctx, `SELECT b.pg_schema,t.pg_name,f.pg_column
		FROM meta.tables t
		JOIN meta.books b ON b.id=t.book_id AND b.deleted_at IS NULL
		LEFT JOIN meta.fields f ON f.id=t.display_field_id AND f.table_id=t.id AND f.deleted_at IS NULL
		WHERE t.id=$1 AND t.deleted_at IS NULL`, tableID).Scan(&table.Schema, &table.Name, &column)
	if errors.Is(err, pgx.ErrNoRows) {
		return ddl.TableName{}, "", acl.ErrNotFound()
	}
	if err != nil {
		return ddl.TableName{}, "", fmt.Errorf("read relation display field: %w", err)
	}
	if err := ddl.ValidateIdentifier(ddl.Book, table.Schema); err != nil {
		return ddl.TableName{}, "", err
	}
	if err := ddl.ValidateIdentifier(ddl.Table, table.Name); err != nil {
		return ddl.TableName{}, "", err
	}
	if !column.Valid {
		return table, "", nil
	}
	if err := ddl.ValidateIdentifier(ddl.Column, column.String); err != nil {
		return ddl.TableName{}, "", err
	}
	return table, column.String, nil
}

func relationLabelExpression(alias, column string) string {
	if column == "" {
		return alias + `."id"::text`
	}
	quoted := pgx.Identifier{column}.Sanitize()
	return "COALESCE(" + alias + "." + quoted + "::text, " + alias + `."id"::text)`
}

func relationTableName(table ddl.TableName) (string, error) {
	if err := ddl.ValidateIdentifier(ddl.Book, table.Schema); err != nil {
		return "", err
	}
	if err := ddl.ValidateIdentifier(ddl.Table, table.Name); err != nil {
		return "", err
	}
	return pgx.Identifier{table.Schema, table.Name}.Sanitize(), nil
}

func enrichRowLinks(ctx context.Context, tx pgx.Tx, table rowTable, fields []rowField, records []RowRecord) error {
	if len(records) == 0 {
		return nil
	}
	sourceIDs := make([]int64, len(records))
	byID := make(map[int64]*RowRecord, len(records))
	for index := range records {
		id, err := canonicalRowID(records[index].ID)
		if err != nil {
			return err
		}
		sourceIDs[index] = id
		byID[id] = &records[index]
	}
	for _, field := range fields {
		relation := field.relation
		if relation == nil {
			continue
		}
		for _, record := range records {
			if record.Fields == nil {
				record.Fields = make(map[string]json.RawMessage)
			}
			record.Fields[field.id] = json.RawMessage("[]")
		}

		sourceTable, _, err := relationDisplayColumn(ctx, tx, table.id)
		if err != nil {
			return err
		}
		sourceName, err := relationTableName(sourceTable)
		if err != nil {
			return err
		}
		targetID := relation.parentTable
		if field.relationSide == "parent" {
			targetID = relation.childTable
		}
		targetTable, targetColumn, err := relationDisplayColumn(ctx, tx, targetID)
		if err != nil {
			return err
		}
		targetName, err := relationTableName(targetTable)
		if err != nil {
			return err
		}
		targetLabel := relationLabelExpression("target", targetColumn)
		sourceField := pgx.Identifier{"source", "id"}.Sanitize()
		targetField := pgx.Identifier{"target", "id"}.Sanitize()
		var query string
		switch {
		case relation.fkColumn.Valid && field.relationSide == "child":
			if err := ddl.ValidateIdentifier(ddl.Column, relation.fkColumn.String); err != nil {
				return err
			}
			fk := pgx.Identifier{"source", relation.fkColumn.String}.Sanitize()
			query = "SELECT " + sourceField + "::text, " + targetField + "::text, " + targetLabel +
				" FROM " + sourceName + ` AS "source" JOIN ` + targetName + ` AS "target" ON ` + fk + "=" + targetField +
				` WHERE "source"."id"=ANY($1::bigint[]) ORDER BY "source"."id"`
		case relation.fkColumn.Valid:
			if err := ddl.ValidateIdentifier(ddl.Column, relation.fkColumn.String); err != nil {
				return err
			}
			fk := pgx.Identifier{"target", relation.fkColumn.String}.Sanitize()
			query = "SELECT " + sourceField + "::text, " + targetField + "::text, " + targetLabel +
				" FROM " + targetName + ` AS "target" JOIN ` + sourceName + ` AS "source" ON ` + fk + "=" + sourceField +
				` WHERE "source"."id"=ANY($1::bigint[]) ORDER BY "source"."id","target"."id"`
		case relation.junction.Valid:
			if err := ddl.ValidateIdentifier(ddl.Junction, relation.junction.String); err != nil {
				return err
			}
			junction := pgx.Identifier{table.schema, relation.junction.String}.Sanitize()
			sourceColumn, targetColumnName := "parent_id", "child_id"
			if field.relationSide == "child" {
				sourceColumn, targetColumnName = "child_id", "parent_id"
			}
			join := pgx.Identifier{"links", targetColumnName}.Sanitize()
			filter := pgx.Identifier{"links", sourceColumn}.Sanitize()
			query = "SELECT " + filter + "::text, " + join + "::text, " + targetLabel +
				" FROM " + junction + ` AS "links" JOIN ` + targetName + ` AS "target" ON ` + join + "=" + targetField +
				" WHERE " + filter + "=ANY($1::bigint[]) ORDER BY " + filter + "," + join
		default:
			return errors.New("relation metadata has no physical storage")
		}
		rows, err := tx.Query(ctx, query, sourceIDs)
		if err != nil {
			return fmt.Errorf("query relation cell values: %w", err)
		}
		links := make(map[int64][]LinkValue, len(records))
		for rows.Next() {
			var sourceText, targetText, label string
			if err := rows.Scan(&sourceText, &targetText, &label); err != nil {
				rows.Close()
				return fmt.Errorf("scan relation cell value: %w", err)
			}
			sourceID, err := strconv.ParseInt(sourceText, 10, 64)
			if err != nil {
				rows.Close()
				return fmt.Errorf("parse relation source row id: %w", err)
			}
			if _, found := byID[sourceID]; !found {
				rows.Close()
				return errors.New("relation query returned an unrelated source row")
			}
			if strings.HasPrefix(label, `"`) {
				var decoded string
				if json.Unmarshal([]byte(label), &decoded) == nil {
					label = decoded
				}
			}
			links[sourceID] = append(links[sourceID], LinkValue{ID: targetText, Label: label})
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("iterate relation cell values: %w", err)
		}
		rows.Close()
		for sourceID, items := range links {
			encoded, err := json.Marshal(items)
			if err != nil {
				return fmt.Errorf("encode relation cell values: %w", err)
			}
			byID[sourceID].Fields[field.id] = encoded
		}
	}
	return nil
}
