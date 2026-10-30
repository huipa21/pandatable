package structure

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"pandatable/internal/acl"
	"pandatable/internal/apperr"
	"pandatable/internal/codec"
	"pandatable/internal/structure/structuresql"
)

const (
	MinColumnWidth = 60
	MaxColumnWidth = 1000
)

type ViewColumn struct {
	ID      string `json:"id"`
	Visible bool   `json:"visible"`
	Width   *int   `json:"width,omitempty"`
}

type ViewColumns struct {
	Fields     []ViewColumn    `json:"fields"`
	Frozen     int             `json:"frozen"`
	GroupBy    []string        `json:"groupBy,omitempty"`
	Aggregates []ViewAggregate `json:"aggregates,omitempty"`
}

type ViewAggregate struct {
	FieldID   string    `json:"fieldId"`
	Aggregate codec.Agg `json:"aggregate"`
}

func readViewColumns(raw []byte, fields []Field) (ViewColumns, error) {
	var config ViewColumns
	if err := json.Unmarshal(raw, &config); err != nil {
		return ViewColumns{}, fmt.Errorf("decode view columns: %w", err)
	}
	return normalizeViewColumns(config, fields)
}

func normalizeViewColumns(config ViewColumns, fields []Field) (ViewColumns, error) {
	live := make(map[string]bool, len(fields))
	for _, field := range fields {
		live[field.ID] = true
	}
	result := ViewColumns{
		Fields:     make([]ViewColumn, 0, len(fields)),
		GroupBy:    append([]string(nil), config.GroupBy...),
		Aggregates: append([]ViewAggregate(nil), config.Aggregates...),
	}
	seen := make(map[string]bool, len(fields))
	visible := 0
	for _, column := range config.Fields {
		if !live[column.ID] || seen[column.ID] {
			continue
		}
		if column.Width != nil && (*column.Width < MinColumnWidth || *column.Width > MaxColumnWidth) {
			return ViewColumns{}, fmt.Errorf("stored view column width is outside bounds")
		}
		seen[column.ID] = true
		result.Fields = append(result.Fields, column)
		if column.Visible {
			visible++
		}
	}
	for _, field := range fields {
		if seen[field.ID] {
			continue
		}
		result.Fields = append(result.Fields, ViewColumn{ID: field.ID, Visible: !field.IsSystem})
		if !field.IsSystem {
			visible++
		}
	}
	result.Frozen = max(0, min(config.Frozen, visible))
	return result, nil
}

func (s *Service) UpdateViewColumns(ctx context.Context, actor Actor, id string, version int32, input ViewColumns) (View, error) {
	uuid, ok := acl.ParseUUID(id)
	if !ok {
		return View{}, acl.ErrNotFound()
	}
	view, err := s.queries.GetViewForQuery(ctx, uuid)
	if err != nil {
		return View{}, notFoundIfNoRows(err, "read view for column save")
	}
	if view.Scope == "personal" && view.OwnerID.String() != actor.ID {
		return View{}, acl.ErrNotFound()
	}
	var result View
	err = s.withWritableTable(ctx, actor, view.TableID.String(), func(tx pgx.Tx, table rowTable) error {
		q := structuresql.New(tx)
		stored, err := q.LockViewColumns(ctx, structuresql.LockViewColumnsParams{ID: uuid, TableID: table.id})
		if err != nil {
			return notFoundIfNoRows(err, "lock view columns")
		}
		if stored.Scope == "personal" && stored.OwnerID.String() != actor.ID {
			return acl.ErrNotFound()
		}
		if stored.Locked {
			return acl.ErrPermissionDenied()
		}
		if stored.Type != "grid" || version < 1 || input.Fields == nil {
			return validation("A grid column configuration is required")
		}
		if stored.Version != version {
			return apperr.New(409, "ERR_VERSION_CONFLICT", "The view changed; reload and retry")
		}
		var existing ViewColumns
		if err := json.Unmarshal(stored.Config, &existing); err != nil {
			return fmt.Errorf("decode stored view config: %w", err)
		}
		if input.GroupBy == nil {
			input.GroupBy = existing.GroupBy
		}
		if input.Aggregates == nil {
			input.Aggregates = existing.Aggregates
		}
		if err := validateGroupConfig(input, table); err != nil {
			return err
		}
		seen := make(map[string]bool, len(input.Fields))
		visible := 0
		for _, column := range input.Fields {
			if _, live := table.byID[column.ID]; !live || seen[column.ID] {
				return validation("Columns must reference unique live table fields")
			}
			if column.Width != nil && (*column.Width < MinColumnWidth || *column.Width > MaxColumnWidth) {
				return validation("Column width is outside bounds")
			}
			seen[column.ID] = true
			if column.Visible {
				visible++
			}
		}
		for _, field := range table.fields {
			if !field.system && !seen[field.id] {
				visible++
			}
		}
		if input.Frozen < 0 || input.Frozen > visible {
			return validation("Frozen columns exceed the visible column count")
		}
		fields := make([]Field, len(table.fields))
		for index, field := range table.fields {
			fields[index] = Field{ID: field.id, IsSystem: field.system}
		}
		clean, err := normalizeViewColumns(input, fields)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(clean)
		if err != nil {
			return fmt.Errorf("encode cleaned config: %w", err)
		}
		if err := q.SaveViewColumns(ctx, structuresql.SaveViewColumnsParams{ID: uuid, Columns: raw}); err != nil {
			return fmt.Errorf("save view columns: %w", err)
		}
		result = View{ID: id, Name: stored.Name, Type: stored.Type, Scope: stored.Scope, Locked: stored.Locked,
			Position: stored.Position, Version: stored.Version + 1, Config: clean}
		return nil
	})
	return result, err
}

func validateGroupConfig(config ViewColumns, table rowTable) error {
	if len(config.GroupBy) > 3 {
		return validation("Grouping supports at most three fields")
	}
	seenGroups := make(map[string]bool, len(config.GroupBy))
	for _, id := range config.GroupBy {
		field, ok := table.byID[id]
		if !ok || field.relation != nil || seenGroups[id] {
			return validation("Grouping fields must be unique live non-link fields")
		}
		seenGroups[id] = true
	}
	seenAggregates := make(map[string]bool, len(config.Aggregates))
	for _, aggregate := range config.Aggregates {
		field, ok := table.byID[aggregate.FieldID]
		if !ok || field.codec == nil {
			return validation("Aggregate fields must be live stored fields")
		}
		supported := false
		for _, candidate := range field.codec.Aggregates() {
			if candidate == aggregate.Aggregate && candidate != codec.AggNone {
				supported = true
				break
			}
		}
		key := aggregate.FieldID + "\x00" + string(aggregate.Aggregate)
		if !supported || seenAggregates[key] {
			return validation("Aggregate is unsupported or duplicated for its field")
		}
		seenAggregates[key] = true
	}
	return nil
}
