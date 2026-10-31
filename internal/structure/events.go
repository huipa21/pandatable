package structure

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/events"
	"pandatable/internal/structure/structuresql"
)

func emitRows(ctx context.Context, tx pgx.Tx, actor Actor, tableID, kind string, rows []RowRecord, fields []string) error {
	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	return events.Emit(ctx, tx, events.Event{
		Type: kind, TableID: tableID, IDs: ids, Fields: fields, By: actor.ID, ByName: actor.Name,
	})
}

func emitPatches(ctx context.Context, tx pgx.Tx, actor Actor, tableID string, rows []RowRecord, patches []RowPatch) error {
	ids := make([]string, len(rows))
	changes := make(map[string][]string, len(patches))
	for i, row := range rows {
		ids[i] = row.ID
	}
	for _, patch := range patches {
		changes[patch.ID] = changedFields([]RowPatch{patch})
	}
	return events.Emit(ctx, tx, events.Event{Type: "row.updated", TableID: tableID, IDs: ids,
		Fields: changedFields(patches), Changes: changes, By: actor.ID, ByName: actor.Name})
}

func emitSchema(ctx context.Context, tx pgx.Tx, actor Actor, tableID pgtype.UUID) error {
	var version int32
	if err := tx.QueryRow(ctx, "SELECT schema_version FROM meta.tables WHERE id = $1", tableID).Scan(&version); err != nil {
		return fmt.Errorf("read realtime schema version: %w", err)
	}
	return events.Emit(ctx, tx, events.Event{
		Type: "schema.changed", TableID: tableID.String(), SchemaVersion: version, By: actor.ID, ByName: actor.Name,
	})
}

func emitRelatedRowInvalidation(ctx context.Context, tx pgx.Tx, tableID, sourceID pgtype.UUID) error {
	return events.Emit(ctx, tx, events.Event{
		Type: "row.bulk", TableID: tableID.String(), SourceTableID: sourceID.String(),
	})
}

func notifyRelatedSchemaChanges(ctx context.Context, tx pgx.Tx, actor Actor, tableID pgtype.UUID) error {
	queries := structuresql.New(tx)
	related, err := queries.ListRelatedLiveSchemas(ctx, tableID)
	if err != nil {
		return fmt.Errorf("list related schemas: %w", err)
	}
	for _, id := range related {
		if err := queries.BumpDependencySchemaVersion(ctx, id); err != nil {
			return fmt.Errorf("bump related schema version: %w", err)
		}
		if err := emitSchema(ctx, tx, actor, id); err != nil {
			return err
		}
	}
	return nil
}

func emitRelatedRowInvalidations(ctx context.Context, tx pgx.Tx, tableID pgtype.UUID) error {
	rows, err := tx.Query(ctx, `SELECT DISTINCT
		CASE WHEN parent_table_id = $1 THEN child_table_id ELSE parent_table_id END
		FROM meta.relations WHERE parent_table_id = $1 OR child_table_id = $1
		ORDER BY 1`, tableID)
	if err != nil {
		return fmt.Errorf("list realtime relation dependencies: %w", err)
	}
	var related []pgtype.UUID
	for rows.Next() {
		var id pgtype.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("read realtime relation dependency: %w", err)
		}
		related = append(related, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read realtime relation dependencies: %w", err)
	}
	for _, id := range related {
		if err := emitRelatedRowInvalidation(ctx, tx, id, tableID); err != nil {
			return err
		}
	}
	return nil
}

func changedFields(patches []RowPatch) []string {
	unique := make(map[string]struct{})
	for _, patch := range patches {
		for id := range patch.Fields {
			unique[id] = struct{}{}
		}
	}
	result := make([]string, 0, len(unique))
	for id := range unique {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}
