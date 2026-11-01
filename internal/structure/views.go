package structure

import (
	"bytes"
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
	Filter     json.RawMessage `json:"filter,omitempty"`
	FilterSet  bool            `json:"-"`
	RowHeight  string          `json:"rowHeight,omitempty"`
	Sort       []ViewSort      `json:"sort,omitempty"`
	GroupBy    []string        `json:"groupBy,omitempty"`
	Aggregates []ViewAggregate `json:"aggregates,omitempty"`
	Highlight  []HighlightRule `json:"highlight,omitempty"`
	Form       *FormConfig     `json:"form,omitempty"`
	FormSet    bool            `json:"-"`
}

type FormConfig struct {
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Fields      []FormField `json:"fields"`
	AfterSubmit string      `json:"afterSubmit"`
}

type FormField struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Help     string `json:"help"`
	Required bool   `json:"required"`
}

type ViewSort struct {
	Field string `json:"field"`
	Dir   string `json:"dir"`
}

type ViewAggregate struct {
	FieldID   string    `json:"fieldId"`
	Aggregate codec.Agg `json:"aggregate"`
}

type HighlightTarget struct {
	Type   string   `json:"type"`
	Fields []string `json:"fields,omitempty"`
}

type HighlightRule struct {
	ID     string          `json:"id"`
	When   *FilterNode     `json:"when"`
	Target HighlightTarget `json:"target"`
	Color  string          `json:"color"`
}

func readViewColumns(raw []byte, fields []Field) (ViewColumns, error) {
	var config ViewColumns
	if err := json.Unmarshal(raw, &config); err != nil {
		return ViewColumns{}, fmt.Errorf("decode view columns: %w", err)
	}
	config.Highlight = normalizeHighlightRules(config.Highlight, fieldIDs(fields))
	if config.Form != nil {
		form, err := normalizeFormConfig(*config.Form, fieldIDs(fields))
		if err != nil {
			return ViewColumns{}, err
		}
		config.Form = &form
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
		Filter:     append(json.RawMessage(nil), config.Filter...),
		FilterSet:  config.FilterSet,
		RowHeight:  config.RowHeight,
		GroupBy:    append([]string(nil), config.GroupBy...),
		Aggregates: append([]ViewAggregate(nil), config.Aggregates...),
		Highlight:  append([]HighlightRule(nil), config.Highlight...),
		Form:       config.Form,
	}
	sortable := make(map[string]bool, len(fields))
	for _, field := range fields {
		if field.UIType != "link" {
			sortable[field.ID] = true
		}
	}
	seenSort := make(map[string]bool, len(config.Sort))
	for _, item := range config.Sort {
		if !sortable[item.Field] || seenSort[item.Field] ||
			item.Dir != "asc" && item.Dir != "desc" {
			continue
		}
		seenSort[item.Field] = true
		result.Sort = append(result.Sort, item)
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

func normalizeFormConfig(config FormConfig, live map[string]bool) (FormConfig, error) {
	if config.AfterSubmit != "message" && config.AfterSubmit != "new" {
		return FormConfig{}, fmt.Errorf("stored form after-submit behavior is invalid")
	}
	result := FormConfig{
		Title: config.Title, Description: config.Description,
		Fields: make([]FormField, 0, len(config.Fields)), AfterSubmit: config.AfterSubmit,
	}
	seen := make(map[string]bool, len(config.Fields))
	for _, field := range config.Fields {
		if !live[field.ID] || seen[field.ID] {
			continue
		}
		seen[field.ID] = true
		result.Fields = append(result.Fields, field)
	}
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
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return View{}, acl.ErrValidation()
	}
	err = s.withViewTable(ctx, actor, view.TableID.String(), &result, func(tx pgx.Tx, table rowTable) error {
		q := structuresql.New(tx)
		stored, err := q.LockViewColumns(ctx, structuresql.LockViewColumnsParams{ID: uuid, TableID: table.id})
		if err != nil {
			return notFoundIfNoRows(err, "lock view columns")
		}
		if err := authorizeViewMutation(ctx, s, tx, actor, user, table, stored.Scope, stored.OwnerID, stored.Locked, false); err != nil {
			return err
		}
		if stored.Type != "grid" || version < 1 || input.Fields == nil {
			return validation("A grid column configuration is required")
		}
		if stored.Version != version {
			return apperr.New(409, "ERR_VERSION_CONFLICT", "The view changed; reload and retry")
		}
		raw, clean, err := normalizeManagedViewConfig(ctx, tx, actor, table, stored.Config, input)
		if err != nil {
			return err
		}
		if err := q.SaveManagedView(ctx, structuresql.SaveManagedViewParams{
			ID: uuid, Name: stored.Name, Locked: stored.Locked, Config: raw,
		}); err != nil {
			return fmt.Errorf("save view columns: %w", err)
		}
		result = View{ID: id, Name: stored.Name, Type: stored.Type, Scope: stored.Scope, Locked: stored.Locked,
			Position: stored.Position, Version: stored.Version + 1, Config: clean}
		return nil
	})
	return result, err
}

func (s *Service) UpdateViewForm(ctx context.Context, actor Actor, id string, version int32, input FormConfig) (View, error) {
	uuid, ok := acl.ParseUUID(id)
	if !ok {
		return View{}, acl.ErrNotFound()
	}
	view, err := s.queries.GetViewForQuery(ctx, uuid)
	if err != nil {
		return View{}, notFoundIfNoRows(err, "read view for form save")
	}
	if view.Scope == "personal" && (!view.OwnerID.Valid || view.OwnerID.String() != actor.ID) {
		return View{}, acl.ErrNotFound()
	}
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return View{}, acl.ErrValidation()
	}
	var result View
	err = s.withViewTable(ctx, actor, view.TableID.String(), &result, func(tx pgx.Tx, table rowTable) error {
		q := structuresql.New(tx)
		stored, err := q.LockViewColumns(ctx, structuresql.LockViewColumnsParams{ID: uuid, TableID: table.id})
		if err != nil {
			return notFoundIfNoRows(err, "lock view form")
		}
		if err := authorizeViewMutation(ctx, s, tx, actor, user, table, stored.Scope, stored.OwnerID, stored.Locked, false); err != nil {
			return err
		}
		if stored.Type != "form" || version < 1 {
			return validation("A form view configuration is required")
		}
		if stored.Version != version {
			return viewVersionConflict()
		}
		form, err := validateFormConfig(input, table)
		if err != nil {
			return err
		}
		var config map[string]json.RawMessage
		if err := json.Unmarshal(stored.Config, &config); err != nil {
			return fmt.Errorf("decode stored view config: %w", err)
		}
		if config == nil {
			return fmt.Errorf("stored view config is not an object")
		}
		encoded, err := json.Marshal(form)
		if err != nil {
			return fmt.Errorf("encode form config: %w", err)
		}
		config["form"] = encoded
		raw, err := json.Marshal(config)
		if err != nil {
			return fmt.Errorf("encode merged view config: %w", err)
		}
		if err := q.SaveManagedView(ctx, structuresql.SaveManagedViewParams{
			ID: uuid, Name: stored.Name, Locked: stored.Locked, Config: raw,
		}); err != nil {
			return fmt.Errorf("save form config: %w", err)
		}
		result, err = managedView(stored.ID, stored.Name, stored.Type, stored.Scope,
			stored.Locked, stored.Position, stored.Version+1, raw, table)
		return err
	})
	return result, err
}

func validateFormConfig(config FormConfig, table rowTable) (FormConfig, error) {
	if config.AfterSubmit != "message" && config.AfterSubmit != "new" {
		return FormConfig{}, validation("Form after-submit behavior is invalid")
	}
	if config.Fields == nil || len(config.Fields) > len(table.fields) {
		return FormConfig{}, validation("Form fields are invalid")
	}
	seen := make(map[string]bool, len(config.Fields))
	for _, item := range config.Fields {
		field, live := table.byID[item.ID]
		parsed, valid := acl.ParseUUID(item.ID)
		if !live || !valid || parsed.String() != item.ID || seen[item.ID] {
			return FormConfig{}, validation("Form fields must reference unique live table fields")
		}
		if field.system || field.relation != nil || field.computed != nil || field.formula != nil ||
			field.codec == nil || field.codec.ReadOnly() {
			return FormConfig{}, validation("Form fields must be writable")
		}
		seen[item.ID] = true
	}
	return config, nil
}

func normalizeManagedViewConfig(ctx context.Context, tx pgx.Tx, actor Actor, table rowTable,
	storedConfig []byte, input ViewColumns,
) ([]byte, ViewColumns, error) {
	if input.Fields == nil {
		return nil, ViewColumns{}, validation("A grid column configuration is required")
	}
	var existing ViewColumns
	if err := json.Unmarshal(storedConfig, &existing); err != nil {
		return nil, ViewColumns{}, fmt.Errorf("decode stored view config: %w", err)
	}
	if input.GroupBy == nil {
		input.GroupBy = existing.GroupBy
	}
	if input.Aggregates == nil {
		input.Aggregates = existing.Aggregates
	}
	if input.Sort == nil {
		input.Sort = existing.Sort
	}
	if !input.FilterSet {
		input.Filter = existing.Filter
		input.FilterSet = existing.FilterSet
	}
	if input.RowHeight == "" {
		input.RowHeight = existing.RowHeight
	} else if input.RowHeight != "short" && input.RowHeight != "medium" &&
		input.RowHeight != "tall" && input.RowHeight != "extra" {
		return nil, ViewColumns{}, validation("Row height is invalid")
	}
	if input.Highlight == nil {
		input.Highlight = normalizeHighlightRules(existing.Highlight, rowFieldIDs(table))
	}
	if err := validateGroupConfig(input, table); err != nil {
		return nil, ViewColumns{}, err
	}
	if _, _, _, err := compileHighlightRules(ctx, tx, actor, table, input.Highlight); err != nil {
		return nil, ViewColumns{}, err
	}
	if input.FilterSet && !bytes.Equal(bytes.TrimSpace(input.Filter), []byte("null")) {
		var filter FilterNode
		decoder := json.NewDecoder(bytes.NewReader(input.Filter))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&filter); err != nil {
			return nil, ViewColumns{}, validation("View filter is invalid")
		}
		if _, _, err := compileRowFilter(ctx, tx, actor, table, &filter); err != nil {
			return nil, ViewColumns{}, err
		}
	}
	seen := make(map[string]bool, len(input.Fields))
	visible := 0
	for _, column := range input.Fields {
		if _, live := table.byID[column.ID]; !live || seen[column.ID] {
			return nil, ViewColumns{}, validation("Columns must reference unique live table fields")
		}
		if column.Width != nil && (*column.Width < MinColumnWidth || *column.Width > MaxColumnWidth) {
			return nil, ViewColumns{}, validation("Column width is outside bounds")
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
		return nil, ViewColumns{}, validation("Frozen columns exceed the visible column count")
	}
	fields := make([]Field, len(table.fields))
	for index, field := range table.fields {
		fields[index] = Field{ID: field.id, IsSystem: field.system}
	}
	clean, err := normalizeViewColumns(input, fields)
	if err != nil {
		return nil, ViewColumns{}, err
	}
	var stored map[string]json.RawMessage
	if err := json.Unmarshal(storedConfig, &stored); err != nil {
		return nil, ViewColumns{}, fmt.Errorf("decode stored view config: %w", err)
	}
	if stored == nil {
		stored = make(map[string]json.RawMessage)
	}
	for key, value := range map[string]any{
		"fields": clean.Fields, "frozen": clean.Frozen, "sort": clean.Sort,
		"groupBy": clean.GroupBy, "aggregates": clean.Aggregates, "highlight": clean.Highlight,
	} {
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, ViewColumns{}, fmt.Errorf("encode view config member %s: %w", key, err)
		}
		stored[key] = encoded
	}
	if input.FilterSet {
		filter := input.Filter
		if len(filter) == 0 {
			filter = json.RawMessage("null")
		}
		stored["filter"] = append(json.RawMessage(nil), filter...)
	}
	if input.RowHeight != "" {
		encoded, err := json.Marshal(input.RowHeight)
		if err != nil {
			return nil, ViewColumns{}, fmt.Errorf("encode view row height: %w", err)
		}
		stored["rowHeight"] = encoded
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		return nil, ViewColumns{}, fmt.Errorf("encode merged view config: %w", err)
	}
	return raw, clean, nil
}

func fieldIDs(fields []Field) map[string]bool {
	result := make(map[string]bool, len(fields))
	for _, field := range fields {
		result[field.ID] = true
	}
	return result
}

func normalizeHighlightRules(rules []HighlightRule, live map[string]bool) []HighlightRule {
	result := make([]HighlightRule, 0, len(rules))
	for _, rule := range rules {
		if !filterReferencesLive(rule.When, live) {
			continue
		}
		if rule.Target.Type == "cells" {
			fields := make([]string, 0, len(rule.Target.Fields))
			for _, id := range rule.Target.Fields {
				if live[id] {
					fields = append(fields, id)
				}
			}
			if len(fields) == 0 {
				continue
			}
			rule.Target.Fields = fields
		}
		result = append(result, rule)
	}
	return result
}

func filterReferencesLive(node *FilterNode, live map[string]bool) bool {
	if node == nil {
		return true
	}
	if node.Op != "" {
		for index := range node.Items {
			if !filterReferencesLive(&node.Items[index], live) {
				return false
			}
		}
		return true
	}
	return node.Field == "" || live[node.Field]
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
