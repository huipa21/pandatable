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
	"pandatable/internal/structure/structuresql"
)

type packageField struct {
	Key      string          `json:"key"`
	Name     string          `json:"name"`
	UIType   string          `json:"uiType"`
	PGType   string          `json:"pgType,omitempty"`
	Options  json.RawMessage `json:"options"`
	Required bool            `json:"required"`
	Unique   bool            `json:"unique"`
	Default  json.RawMessage `json:"default"`
}

type packageView struct {
	Name   string      `json:"name"`
	Type   string      `json:"type"`
	Config ViewColumns `json:"config"`
}

type tablePackageManifest struct {
	Format        string         `json:"format"`
	FormatVersion int            `json:"formatVersion"`
	AppVersion    string         `json:"appVersion"`
	ExportedAt    time.Time      `json:"exportedAt"`
	Table         packageTable   `json:"table"`
	Fields        []packageField `json:"fields"`
	Views         []packageView  `json:"views"`
	RowIDColumn   bool           `json:"rowIdColumn"`
}

type packageTable struct {
	Name         string  `json:"name"`
	DisplayField *string `json:"displayField"`
}

func packageManifest(ctx context.Context, tx pgx.Tx, actor Actor, schema TableSchema, table rowTable, version string) ([]byte, error) {
	if version == "" {
		return nil, errors.New("package export requires an application version")
	}
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return nil, acl.ErrValidation()
	}
	keys := make(map[string]string, len(schema.Fields))
	for index, field := range schema.Fields {
		keys[field.ID] = fmt.Sprintf("f%d", index+1)
	}
	manifest := tablePackageManifest{
		Format: "pandatable.table", FormatVersion: 1, AppVersion: version,
		ExportedAt: time.Now().UTC(), Table: packageTable{Name: schema.Table.Name},
		Fields: make([]packageField, 0, len(schema.Fields)),
		Views:  make([]packageView, 0, len(schema.Views)), RowIDColumn: true,
	}
	if schema.Table.DisplayFieldID != nil {
		key, exists := keys[*schema.Table.DisplayFieldID]
		if !exists {
			return nil, errors.New("package display field is unavailable")
		}
		manifest.Table.DisplayField = &key
	}
	for _, field := range schema.Fields {
		rowField, exists := table.byID[field.ID]
		if !exists {
			return nil, errors.New("package field metadata is unavailable")
		}
		options, err := packageFieldOptions(ctx, tx, user, field, rowField, keys)
		if err != nil {
			return nil, err
		}
		entry := packageField{
			Key: keys[field.ID], Name: field.Name, UIType: field.UIType,
			Options: options, Required: field.IsRequired, Unique: field.IsUnique,
			Default: field.Default,
		}
		if len(entry.Default) == 0 {
			entry.Default = json.RawMessage("null")
		}
		if rowField.codec != nil {
			entry.PGType = pgTypeName(rowField.codec.Column().Type)
		}
		manifest.Fields = append(manifest.Fields, entry)
	}
	for _, view := range schema.Views {
		if view.Scope != "shared" {
			continue
		}
		config, err := packageViewConfig(view.Config, keys)
		if err != nil {
			return nil, err
		}
		manifest.Views = append(manifest.Views, packageView{Name: view.Name, Type: view.Type, Config: config})
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("encode package manifest: %w", err)
	}
	return encoded, nil
}

func packageFieldOptions(ctx context.Context, tx pgx.Tx, user pgtype.UUID, field Field, row rowField, keys map[string]string) (json.RawMessage, error) {
	if row.relation != nil {
		target := row.relation.parentTable
		if row.relationSide == "parent" {
			target = row.relation.childTable
		}
		name, readable, err := packageTargetName(ctx, tx, user, target)
		if err != nil {
			return nil, err
		}
		options := struct {
			Kind        string  `json:"kind"`
			Side        string  `json:"side"`
			TargetTable *string `json:"targetTable"`
			OnDelete    string  `json:"onDelete"`
		}{Kind: row.relation.kind, Side: row.relationSide, OnDelete: row.relation.onDelete}
		if readable {
			options.TargetTable = &name
		}
		return json.Marshal(options)
	}
	if row.formula != nil {
		return json.Marshal(struct {
			Expression string `json:"expr"`
		}{Expression: row.formula.options.Expression})
	}
	if row.computed == nil {
		return append(json.RawMessage(nil), field.Options...), nil
	}
	options := make(map[string]json.RawMessage)
	if err := json.Unmarshal(field.Options, &options); err != nil || options == nil {
		return nil, errors.New("computed package field options are invalid")
	}
	relationKey, exists := keys[row.computed.options.RelationFieldID]
	if !exists {
		return nil, errors.New("computed package relation is unavailable")
	}
	encodedKey, err := json.Marshal(relationKey)
	if err != nil {
		return nil, err
	}
	options["relationFieldId"] = encodedKey
	delete(options, "targetFieldId")
	targetName, readable, err := packageTargetName(ctx, tx, user, row.computed.targetTable)
	if err != nil {
		return nil, err
	}
	if readable {
		fields, err := structuresql.New(tx).ListTableFields(ctx, row.computed.targetTable)
		if err != nil {
			return nil, fmt.Errorf("read package target fields: %w", err)
		}
		for _, target := range fields {
			if target.ID.String() == row.computed.options.TargetFieldID {
				options["targetField"], err = json.Marshal(target.Name)
				if err != nil {
					return nil, err
				}
				options["targetTable"], err = json.Marshal(targetName)
				if err != nil {
					return nil, err
				}
				break
			}
		}
	}
	return json.Marshal(options)
}

func packageTargetName(ctx context.Context, tx pgx.Tx, user, target pgtype.UUID) (string, bool, error) {
	access, err := acl.AccessUncached(ctx, tx, user, acl.Resource{Type: acl.Table, ID: target})
	if err != nil {
		return "", false, err
	}
	if !access.Visible || access.Check(acl.Read) != nil {
		return "", false, nil
	}
	table, err := structuresql.New(tx).GetTable(ctx, target)
	if err != nil {
		return "", false, notFoundIfNoRows(err, "read package relation target")
	}
	return table.Name, true, nil
}

func packageViewConfig(source ViewColumns, keys map[string]string) (ViewColumns, error) {
	raw, err := json.Marshal(source)
	if err != nil {
		return ViewColumns{}, fmt.Errorf("copy package view: %w", err)
	}
	var config ViewColumns
	if err := json.Unmarshal(raw, &config); err != nil {
		return ViewColumns{}, fmt.Errorf("decode package view: %w", err)
	}
	key := func(id *string) error {
		mapped, exists := keys[*id]
		if !exists {
			return errors.New("package view references an unavailable field")
		}
		*id = mapped
		return nil
	}
	for index := range config.Fields {
		if err := key(&config.Fields[index].ID); err != nil {
			return ViewColumns{}, err
		}
	}
	for index := range config.Sort {
		if err := key(&config.Sort[index].Field); err != nil {
			return ViewColumns{}, err
		}
	}
	for index := range config.GroupBy {
		if err := key(&config.GroupBy[index]); err != nil {
			return ViewColumns{}, err
		}
	}
	for index := range config.Aggregates {
		if err := key(&config.Aggregates[index].FieldID); err != nil {
			return ViewColumns{}, err
		}
	}
	if len(config.Filter) > 0 && string(config.Filter) != "null" {
		var filter FilterNode
		if err := json.Unmarshal(config.Filter, &filter); err != nil {
			return ViewColumns{}, fmt.Errorf("decode package view filter: %w", err)
		}
		if err := remapPackageFilter(&filter, key); err != nil {
			return ViewColumns{}, err
		}
		config.Filter, err = json.Marshal(filter)
		if err != nil {
			return ViewColumns{}, err
		}
	}
	for index := range config.Highlight {
		rule := &config.Highlight[index]
		if rule.When != nil {
			if err := remapPackageFilter(rule.When, key); err != nil {
				return ViewColumns{}, err
			}
		}
		for field := range rule.Target.Fields {
			if err := key(&rule.Target.Fields[field]); err != nil {
				return ViewColumns{}, err
			}
		}
	}
	if config.Form != nil {
		for index := range config.Form.Fields {
			if err := key(&config.Form.Fields[index].ID); err != nil {
				return ViewColumns{}, err
			}
		}
	}
	if config.Kanban != nil {
		if config.Kanban.GroupField != nil {
			if err := key(config.Kanban.GroupField); err != nil {
				return ViewColumns{}, err
			}
		}
		for index := range config.Kanban.Fields {
			if err := key(&config.Kanban.Fields[index]); err != nil {
				return ViewColumns{}, err
			}
		}
	}
	if config.Gallery != nil {
		if config.Gallery.CoverField != nil {
			if err := key(config.Gallery.CoverField); err != nil {
				return ViewColumns{}, err
			}
		}
		for index := range config.Gallery.Fields {
			if err := key(&config.Gallery.Fields[index]); err != nil {
				return ViewColumns{}, err
			}
		}
	}
	return config, nil
}

func remapPackageFilter(node *FilterNode, key func(*string) error) error {
	if node.Field != "" {
		if err := key(&node.Field); err != nil {
			return err
		}
	}
	for index := range node.Items {
		if err := remapPackageFilter(&node.Items[index], key); err != nil {
			return err
		}
	}
	return nil
}
