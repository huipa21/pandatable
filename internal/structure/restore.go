package structure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl"
	"pandatable/internal/apperr"
	"pandatable/internal/codec"
	"pandatable/internal/structure/structuresql"
)

const maxRestoreRows = 10_000

type restoreLinkPlan struct {
	table    rowTable
	target   rowTable
	field    rowField
	sourceID int64
	targets  []int64
}

type restoreSystem struct {
	createdAt time.Time
	createdBy pgtype.UUID
	updatedAt time.Time
	updatedBy pgtype.UUID
}

func restoreUnavailable() error {
	return apperr.New(409, "ERR_UNDO_UNAVAILABLE", "The deleted snapshot is no longer available")
}

func equalJSONValue(left, right any) bool {
	switch left := left.(type) {
	case nil:
		return right == nil
	case bool:
		value, ok := right.(bool)
		return ok && left == value
	case string:
		value, ok := right.(string)
		return ok && left == value
	case json.Number:
		value, ok := right.(json.Number)
		if !ok {
			return false
		}
		a, aok := new(big.Rat).SetString(string(left))
		b, bok := new(big.Rat).SetString(string(value))
		return aok && bok && a.Cmp(b) == 0
	case []any:
		values, ok := right.([]any)
		if !ok || len(left) != len(values) {
			return false
		}
		for i := range left {
			if !equalJSONValue(left[i], values[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		values, ok := right.(map[string]any)
		if !ok || len(left) != len(values) {
			return false
		}
		for key, value := range left {
			other, found := values[key]
			if !found || !equalJSONValue(value, other) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func sameJSON(left, right []byte) (bool, error) {
	values := make([]any, 2)
	for i, raw := range [][]byte{left, right} {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&values[i]); err != nil {
			return false, fmt.Errorf("decode snapshot JSON: %w", err)
		}
	}
	return equalJSONValue(values[0], values[1]), nil
}

func resolveRestoreSnapshot(ctx context.Context, tx pgx.Tx, table rowTable, actor Actor, record RowRecord) (RowRecord, error) {
	id, err := canonicalRowID(record.ID)
	if err != nil {
		return RowRecord{}, err
	}
	history, err := structuresql.New(tx).ListRowHistory(ctx, structuresql.ListRowHistoryParams{
		TableID: table.id, RowID: id, PageOffset: 0, PageLimit: 1,
	})
	if err != nil {
		return RowRecord{}, fmt.Errorf("read restore snapshot history: %w", err)
	}
	if len(history) != 1 || history[0].Action != "delete" ||
		(record.RestoreVersion != "" && record.RestoreVersion != strconv.FormatInt(history[0].ID, 10)) ||
		len(history[0].RestoreSystem) == 0 {
		return RowRecord{}, restoreUnavailable()
	}
	if record.Fields == nil {
		if record.RestoreVersion == "" {
			return RowRecord{}, restoreUnavailable()
		}
		var changes map[string]HistoryChange
		var system map[string]json.RawMessage
		if err := json.Unmarshal(history[0].Changes, &changes); err != nil {
			return RowRecord{}, fmt.Errorf("decode delete history snapshot: %w", err)
		}
		if err := json.Unmarshal(history[0].RestoreSystem, &system); err != nil {
			return RowRecord{}, fmt.Errorf("decode delete system snapshot: %w", err)
		}
		record.Fields = make(map[string]json.RawMessage, len(changes)+len(system))
		for fieldID, change := range changes {
			if len(change.Old) == 0 {
				return RowRecord{}, restoreUnavailable()
			}
			record.Fields[fieldID] = change.Old
		}
		for fieldID, value := range system {
			record.Fields[fieldID] = value
		}
	}
	entry, err := snapshotHistory(table, record, actor, "delete")
	if err != nil {
		return RowRecord{}, restoreUnavailable()
	}
	expected, err := json.Marshal(entry.Changes)
	if err != nil {
		return RowRecord{}, fmt.Errorf("encode restore snapshot: %w", err)
	}
	expectedSystem, err := json.Marshal(entry.RestoreSystem)
	if err != nil {
		return RowRecord{}, fmt.Errorf("encode restore system snapshot: %w", err)
	}
	matches, err := sameJSON(expected, history[0].Changes)
	if err != nil {
		return RowRecord{}, err
	}
	if !matches {
		return RowRecord{}, restoreUnavailable()
	}
	matches, err = sameJSON(expectedSystem, history[0].RestoreSystem)
	if err != nil {
		return RowRecord{}, err
	}
	if !matches {
		return RowRecord{}, restoreUnavailable()
	}
	return record, nil
}

func decodeRestoreSystem(table rowTable, record RowRecord) (restoreSystem, error) {
	var result restoreSystem
	seen := make(map[codec.Kind]bool)
	for _, field := range table.fields {
		if !field.system {
			continue
		}
		raw, found := record.Fields[field.id]
		if !found || seen[field.kind] {
			return restoreSystem{}, restoreUnavailable()
		}
		seen[field.kind] = true
		switch field.kind {
		case codec.RowID:
			var id string
			if err := json.Unmarshal(raw, &id); err != nil || id != record.ID {
				return restoreSystem{}, restoreUnavailable()
			}
		case codec.CreatedAt, codec.UpdatedAt:
			var text string
			if err := json.Unmarshal(raw, &text); err != nil {
				return restoreSystem{}, restoreUnavailable()
			}
			value, err := time.Parse(time.RFC3339Nano, text)
			if err != nil {
				return restoreSystem{}, restoreUnavailable()
			}
			if field.kind == codec.CreatedAt {
				result.createdAt = value
			} else {
				result.updatedAt = value
			}
		case codec.CreatedBy, codec.UpdatedBy:
			var text *string
			if err := json.Unmarshal(raw, &text); err != nil {
				return restoreSystem{}, restoreUnavailable()
			}
			var value pgtype.UUID
			if text != nil {
				var ok bool
				value, ok = acl.ParseUUID(*text)
				if !ok || value.String() != *text {
					return restoreSystem{}, restoreUnavailable()
				}
			}
			if field.kind == codec.CreatedBy {
				result.createdBy = value
			} else {
				result.updatedBy = value
			}
		default:
			return restoreSystem{}, errors.New("unknown system field in restore snapshot")
		}
	}
	for _, kind := range []codec.Kind{
		codec.RowID, codec.CreatedAt, codec.CreatedBy, codec.UpdatedAt, codec.UpdatedBy,
	} {
		if !seen[kind] {
			return restoreSystem{}, restoreUnavailable()
		}
	}
	return result, nil
}

func insertRestoredRow(ctx context.Context, tx pgx.Tx, table rowTable, record RowRecord) error {
	id, err := canonicalRowID(record.ID)
	if err != nil {
		return err
	}
	system, err := decodeRestoreSystem(table, record)
	if err != nil {
		return err
	}
	values := make(map[string]json.RawMessage)
	autoFields := make([]rowField, 0)
	autoValues := make([]int64, 0)
	for _, field := range table.fields {
		if field.system || field.relation != nil || field.computed != nil || field.formula != nil {
			continue
		}
		raw, found := record.Fields[field.id]
		if !found {
			return restoreUnavailable()
		}
		if field.kind == codec.AutoNumber {
			var text string
			if err := json.Unmarshal(raw, &text); err != nil {
				return validation("Autonumber snapshot is invalid")
			}
			value, err := canonicalRowID(text)
			if err != nil {
				return err
			}
			autoFields = append(autoFields, field)
			autoValues = append(autoValues, value)
		} else {
			values[field.id] = raw
		}
	}
	fields, decoded, err := sortedFieldValues(ctx, structuresql.New(tx), table, values)
	if err != nil {
		return err
	}
	columns := []string{`"id"`}
	arguments := []any{id}
	for i, field := range fields {
		columns = append(columns, field.sqlColumn())
		arguments = append(arguments, decoded[i])
	}
	for i, field := range autoFields {
		columns = append(columns, field.sqlColumn())
		arguments = append(arguments, autoValues[i])
	}
	columns = append(columns, `"created_at"`, `"created_by"`, `"updated_at"`, `"updated_by"`)
	arguments = append(arguments, system.createdAt, system.createdBy, system.updatedAt, system.updatedBy)
	query, args, err := sq.Insert(table.qualifiedName()).Columns(columns...).Values(arguments...).
		PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return fmt.Errorf("build row restore: %w", err)
	}
	if _, err := tx.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("insert restored row: %w", err)
	}
	return nil
}

func resetRestoredSystem(ctx context.Context, tx pgx.Tx, table rowTable, record RowRecord) error {
	id, err := canonicalRowID(record.ID)
	if err != nil {
		return err
	}
	system, err := decodeRestoreSystem(table, record)
	if err != nil {
		return err
	}
	query, args, err := sq.Update(table.qualifiedName()).
		Set("created_at", system.createdAt).Set("created_by", system.createdBy).
		Set("updated_at", system.updatedAt).Set("updated_by", system.updatedBy).
		Where(sq.Eq{`"id"`: id}).PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return fmt.Errorf("build restored system fields update: %w", err)
	}
	result, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("restore system fields: %w", err)
	}
	if result.RowsAffected() != 1 {
		return errors.New("restored row is missing when restoring system fields")
	}
	return nil
}

func restoreLinks(ctx context.Context, tx pgx.Tx, actor Actor, user pgtype.UUID, plan restoreLinkPlan) error {
	relation := *plan.field.relation
	if plan.table.id == plan.target.id {
		ids := uniqueSortedIDs(append(append([]int64{}, plan.targets...), plan.sourceID))
		if err := lockRelationRows(ctx, tx, plan.table, ids); err != nil {
			return err
		}
	} else if plan.table.id.String() < plan.target.id.String() {
		if err := lockRelationRows(ctx, tx, plan.table, []int64{plan.sourceID}); err != nil {
			return err
		}
		if err := lockRelationRows(ctx, tx, plan.target, uniqueSortedIDs(plan.targets)); err != nil {
			return err
		}
	} else {
		if err := lockRelationRows(ctx, tx, plan.target, uniqueSortedIDs(plan.targets)); err != nil {
			return err
		}
		if err := lockRelationRows(ctx, tx, plan.table, []int64{plan.sourceID}); err != nil {
			return err
		}
	}
	if (relation.kind == "one_one" || relation.kind == "one_many" && plan.field.relationSide == "child") &&
		len(plan.targets) > 1 {
		return validation("The restored relation accepts one linked row")
	}
	var edges []relationEdge
	switch relation.kind {
	case "many_many":
		if !relation.junction.Valid {
			return errors.New("restored relation is missing its junction")
		}
		if err := changeJunctionLinks(ctx, tx, relation, plan.field.relationSide,
			plan.sourceID, plan.targets, nil, user, &edges); err != nil {
			return err
		}
	case "one_one":
		if !relation.fkColumn.Valid {
			return errors.New("restored relation is missing its foreign key")
		}
		if err := changeOneToOneLinks(ctx, tx, plan.table, plan.target, relation, plan.field.relationSide,
			plan.sourceID, plan.targets, nil, false, user, &edges); err != nil {
			return err
		}
	case "one_many":
		if !relation.fkColumn.Valid {
			return errors.New("restored relation is missing its foreign key")
		}
		if plan.field.relationSide == "parent" {
			for _, targetID := range plan.targets {
				current, err := rowLinkValue(ctx, tx, plan.target, targetID, relation.fkColumn.String)
				if err != nil {
					return err
				}
				if current != nil && *current != plan.sourceID {
					return relationConflictError(strconv.FormatInt(*current, 10))
				}
			}
		}
		if err := changeOneToManyLinks(ctx, tx, plan.table, relation, plan.field.relationSide,
			plan.sourceID, plan.targets, nil, user, &edges); err != nil {
			return err
		}
	default:
		return errors.New("restored relation has an unknown kind")
	}
	if err := recordRelationHistory(ctx, tx, actor, relation, edges); err != nil {
		return err
	}
	return emitRelatedRowInvalidation(ctx, tx, plan.target.id, plan.table.id)
}

func (s *Service) RestoreRows(ctx context.Context, actor Actor, tableID string, input []RowRecord) ([]RowRecord, error) {
	if len(input) == 0 || len(input) > maxBulkRows {
		return nil, validation("Restore row count is outside the supported bounds")
	}
	groups := map[string][]RowRecord{tableID: {}}
	seen := make(map[string]map[string]bool)
	add := func(id string, record RowRecord) error {
		uuid, ok := acl.ParseUUID(id)
		if !ok || uuid.String() != id {
			return validation("Restore table id is invalid")
		}
		if _, err := canonicalRowID(record.ID); err != nil {
			return err
		}
		if record.Fields == nil && record.RestoreVersion == "" {
			return validation("Restore row fields or a snapshot version are required")
		}
		if seen[id] == nil {
			seen[id] = make(map[string]bool)
		}
		if seen[id][record.ID] {
			return validation("Restore row ids must be unique within a table")
		}
		seen[id][record.ID] = true
		record.Cascaded = nil
		groups[id] = append(groups[id], record)
		return nil
	}
	total := 0
	for _, row := range input {
		if err := add(tableID, row); err != nil {
			return nil, err
		}
		total++
		for _, group := range row.Cascaded {
			for _, dependent := range group.Rows {
				if len(dependent.Cascaded) != 0 {
					return nil, validation("Nested restore snapshots are invalid")
				}
				if err := add(group.TableID, dependent); err != nil {
					return nil, err
				}
				total++
				if total > maxRestoreRows {
					return nil, validation("Restore row count is outside the supported bounds")
				}
			}
		}
	}
	tableIDs := make([]string, 0, len(groups))
	for id := range groups {
		tableIDs = append(tableIDs, id)
	}
	sort.Strings(tableIDs)
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return nil, acl.ErrValidation()
	}
	var result []RowRecord
	err := s.withWritableTable(ctx, actor, tableID, func(tx pgx.Tx, primary rowTable) error {
		tables := map[string]rowTable{tableID: primary}
		load := func(id pgtype.UUID) (rowTable, error) {
			key := id.String()
			if table, found := tables[key]; found {
				return table, nil
			}
			target := acl.Resource{Type: acl.Table, ID: id}
			if _, err := acl.LockPath(ctx, tx, target); err != nil {
				return rowTable{}, err
			}
			if err := s.authorizeRows(ctx, tx, user, target, acl.Write); err != nil {
				return rowTable{}, err
			}
			table, err := relationRowTable(ctx, tx, id)
			if err != nil {
				return rowTable{}, err
			}
			tables[key] = table
			return table, nil
		}
		for _, id := range tableIDs {
			uuid, _ := acl.ParseUUID(id)
			table, err := load(uuid)
			if err != nil {
				return err
			}
			for index, record := range groups[id] {
				resolved, err := resolveRestoreSnapshot(ctx, tx, table, actor, record)
				if err != nil {
					return err
				}
				groups[id][index] = resolved
			}
		}
		for _, id := range tableIDs {
			table := tables[id]
			for _, record := range groups[id] {
				if err := insertRestoredRow(ctx, tx, table, record); err != nil {
					return err
				}
			}
		}
		edges := make(map[string]bool)
		plans := make([]restoreLinkPlan, 0)
		for _, id := range tableIDs {
			table := tables[id]
			for _, record := range groups[id] {
				sourceID, _ := canonicalRowID(record.ID)
				for _, field := range table.fields {
					if field.relation == nil {
						continue
					}
					var links []LinkValue
					if err := json.Unmarshal(record.Fields[field.id], &links); err != nil {
						return validation("Restore relation snapshot is invalid")
					}
					if len(links) == 0 {
						continue
					}
					targetID := field.relation.parentTable
					if field.relationSide == "parent" {
						targetID = field.relation.childTable
					}
					target, err := load(targetID)
					if err != nil {
						return err
					}
					plan := restoreLinkPlan{table: table, target: target, field: field, sourceID: sourceID}
					for _, link := range links {
						linkedID, err := canonicalRowID(link.ID)
						if err != nil {
							return err
						}
						parentID, childID := sourceID, linkedID
						if field.relationSide == "child" {
							parentID, childID = linkedID, sourceID
						}
						key := fmt.Sprintf("%s:%d:%d", field.relation.id, parentID, childID)
						if !edges[key] {
							edges[key] = true
							plan.targets = append(plan.targets, linkedID)
						}
					}
					if len(plan.targets) > 0 {
						plans = append(plans, plan)
					}
				}
			}
		}
		for _, plan := range plans {
			if err := restoreLinks(ctx, tx, actor, user, plan); err != nil {
				return err
			}
		}
		for _, id := range tableIDs {
			for _, record := range groups[id] {
				if err := resetRestoredSystem(ctx, tx, tables[id], record); err != nil {
					return err
				}
			}
		}
		restored := make(map[string]map[string]RowRecord, len(groups))
		for _, id := range tableIDs {
			table := tables[id]
			ids := make([]int64, 0, len(groups[id]))
			for _, record := range groups[id] {
				rowID, _ := canonicalRowID(record.ID)
				ids = append(ids, rowID)
			}
			query, args, err := sq.Select(table.selectColumnList(table.fields)...).
				From(table.qualifiedName()).Where(sq.Expr(`"id" = ANY(?::bigint[])`, ids)).
				OrderBy(`"id" ASC`).PlaceholderFormat(sq.Dollar).ToSql()
			if err != nil {
				return fmt.Errorf("build restored rows query: %w", err)
			}
			rows, err := queryUncachedRows(ctx, tx, query, args...)
			if err != nil {
				return fmt.Errorf("query restored rows: %w", err)
			}
			records, err := scanRows(rows, table, table.fields)
			if err != nil {
				return err
			}
			if len(records) != len(ids) {
				return errors.New("restored rows are missing")
			}
			if err := enrichRowLinks(ctx, tx, table, table.fields, records); err != nil {
				return err
			}
			if err := enrichRowComputed(ctx, tx, actor, table, table.fields, records); err != nil {
				return err
			}
			if err := enrichRowFormulas(ctx, tx, actor, table, table.fields, records); err != nil {
				return err
			}
			if err := enrichCommentCounts(ctx, tx, table.id, records); err != nil {
				return err
			}
			history, err := snapshotHistories(table, records, actor, "restore")
			if err != nil {
				return err
			}
			if err := insertHistory(ctx, tx, history); err != nil {
				return err
			}
			if err := emitRows(ctx, tx, actor, id, "row.created", records, nil); err != nil {
				return err
			}
			restored[id] = make(map[string]RowRecord, len(records))
			for _, record := range records {
				restored[id][record.ID] = record
			}
		}
		result = make([]RowRecord, len(input))
		for i, record := range input {
			result[i] = restored[tableID][record.ID]
		}
		return nil
	})
	return result, err
}
