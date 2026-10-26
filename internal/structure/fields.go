package structure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl"
	"pandatable/internal/apperr"
	"pandatable/internal/codec"
	"pandatable/internal/ddl"
	"pandatable/internal/structure/structuresql"
)

const maxDescriptionRunes = 1000

// FieldInput creates a user field (D-123). Default nil means absent; JSON null
// means no default.
type FieldInput struct {
	Name        string
	UIType      string
	Description string
	Options     json.RawMessage
	IsRequired  bool
	IsUnique    bool
	Default     json.RawMessage
	FillEmpty   bool
}

// FieldPatch preserves omitted members. Version is the caller's If-Match value.
type FieldPatch struct {
	Version     int32
	Name        *string
	Description *string
	Options     json.RawMessage
	IsRequired  *bool
	IsUnique    *bool
	Default     json.RawMessage
	Index       *int
	FillEmpty   bool
}

// FieldResult is the authoritative schema after a field mutation.
type FieldResult struct {
	Field  *Field      `json:"field,omitempty"`
	Schema TableSchema `json:"schema"`
}

func reasonError(status int, code, message, reason string) *apperr.Error {
	err := apperr.New(status, code, message)
	err.Details = map[string]any{"reason": reason}
	return err
}

func countError(code, message string, count int64) *apperr.Error {
	err := apperr.New(409, code, message)
	err.Details = map[string]any{"count": count}
	return err
}

func systemFieldError() error {
	return reasonError(400, "ERR_VALIDATION", "System fields allow only name and description changes", "system_field")
}

func cleanDescription(value string) (string, error) {
	text := strings.TrimSpace(value)
	if !utf8.ValidString(text) || utf8.RuneCountInString(text) > maxDescriptionRunes ||
		strings.ContainsFunc(text, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\t' }) {
		return "", validation("Description must be at most 1000 characters without control characters")
	}
	return text, nil
}

func isJSONNull(raw json.RawMessage) bool { return bytes.Equal(bytes.TrimSpace(raw), []byte("null")) }

// bindUserKind binds options for a writable batch-1 kind; system kinds are
// never created or reconfigured through the field API.
func bindUserKind(kind string, options json.RawMessage) (codec.Field, error) {
	if codec.IsSystem(codec.Kind(kind)) {
		return nil, systemFieldError()
	}
	if len(options) == 0 {
		options = json.RawMessage("{}")
	}
	field, err := codec.Bind(codec.Kind(kind), options)
	if err != nil {
		return nil, codec.APIError(err)
	}
	return field, nil
}

func checkConstraints(field codec.Field, required, unique, hasDefault bool) error {
	allowed := field.Constraints()
	if required && !allowed.Required || unique && !allowed.Unique || hasDefault && !allowed.Default {
		return reasonError(400, "ERR_VALIDATION", "This field type does not support the constraint", "constraint_unsupported")
	}
	return nil
}

// fieldDefault is a validated user default: the canonical wire JSON stored in
// metadata, the typed DDL literal and the decoded parameter for fills.
type fieldDefault struct {
	stored []byte
	ddl    ddl.DefaultValue
	value  any
}

func resolveDefault(field codec.Field, raw json.RawMessage) (fieldDefault, error) {
	none := fieldDefault{ddl: field.Column().Default}
	if len(raw) == 0 || isJSONNull(raw) {
		return none, nil
	}
	value, err := field.Decode(raw)
	if err != nil {
		return fieldDefault{}, defaultError(err)
	}
	if value == nil {
		return none, nil
	}
	stored, err := field.Encode(value)
	if err != nil {
		return fieldDefault{}, err
	}
	literal, err := field.DefaultValue(stored)
	if err != nil {
		return fieldDefault{}, defaultError(err)
	}
	return fieldDefault{stored: stored, ddl: literal, value: value}, nil
}

func defaultError(err error) error {
	mapped := codec.APIError(err)
	var app *apperr.Error
	if errors.As(mapped, &app) && app.Details != nil {
		app.Details["option"] = "default"
	}
	return mapped
}

// checkTimezone verifies a datetime fixedTz against PostgreSQL's catalog
// (D-114) in the mutation transaction.
func checkTimezone(ctx context.Context, q *structuresql.Queries, field codec.Field) error {
	if field.Kind() != codec.DateTime {
		return nil
	}
	var options struct {
		FixedTz string `json:"fixedTz"`
	}
	if err := json.Unmarshal(field.Options(), &options); err != nil {
		return fmt.Errorf("read datetime options: %w", err)
	}
	if options.FixedTz == "" {
		return nil
	}
	valid, err := q.ValidFieldTimezone(ctx, options.FixedTz)
	if err != nil {
		return fmt.Errorf("check fixed timezone: %w", err)
	}
	if !valid {
		return codec.APIError(&codec.Error{Reason: codec.ReasonInvalidOptions, Detail: "fixedTz"})
	}
	return nil
}

func pgTypeName(dataType ddl.DataType) string {
	switch dataType.Kind {
	case ddl.Varchar:
		return fmt.Sprintf("varchar(%d)", dataType.Length)
	case ddl.Numeric:
		return fmt.Sprintf("numeric(%d,%d)", dataType.Precision, dataType.Scale)
	}
	return string(dataType.Kind)
}

// displayEligible: live, non-system and not checkbox (D-129).
func displayEligible(kind string, system bool) bool {
	return !system && !codec.IsSystem(codec.Kind(kind)) && codec.Kind(kind) != codec.Checkbox
}

// fieldNameConflict maps the live case-insensitive name index (D-126).
func fieldNameConflict(err error) error {
	var postgres *pgconn.PgError
	if errors.As(err, &postgres) && postgres.Code == "23505" && postgres.ConstraintName == "fields_table_id_lower_name_idx" {
		return apperr.New(409, "ERR_FIELD_NAME_CONFLICT", "Another field already uses this name")
	}
	return err
}

type lockedTable struct {
	structuresql.LockFieldTableRow
	physical ddl.TableName
}

func lockTable(ctx context.Context, c *txn, id pgtype.UUID) (lockedTable, error) {
	row, err := c.q.LockFieldTable(ctx, id)
	if err != nil {
		return lockedTable{}, notFoundIfNoRows(err, "lock table")
	}
	return lockedTable{LockFieldTableRow: row, physical: ddl.TableName{Schema: row.PgSchema, Name: row.PgName}}, nil
}

// columnState is the constraint/default state of one physical column.
type columnState struct {
	required, unique bool
	stored           []byte
}

// applyColumn moves a column from one state to another: default first, then
// required (preflight, optional fill), then unique (preflight). All in c.tx.
func applyColumn(ctx context.Context, c *txn, target physicalColumn, dataType ddl.DataType, from, to columnState,
	value fieldDefault, fill, forceDefault bool) error {
	if forceDefault || !bytes.Equal(from.stored, to.stored) {
		if err := ddl.SetDefault(ctx, c.tx, target.table, target.column, dataType, value.ddl); err != nil {
			return err
		}
	}
	switch {
	case to.required && !from.required:
		empty, err := target.emptyCount(ctx, c.tx)
		if err != nil {
			return err
		}
		if empty > 0 {
			if !fill || value.value == nil {
				return countError("ERR_REQUIRED_HAS_EMPTY", "Some rows are empty; provide a default and fill them, or cancel", empty)
			}
			if err := target.fillEmpty(ctx, c.tx, value.value); err != nil {
				return err
			}
		}
		if err := ddl.SetNotNull(ctx, c.tx, target.table, target.column, true); err != nil {
			return err
		}
	case !to.required && from.required:
		if err := ddl.SetNotNull(ctx, c.tx, target.table, target.column, false); err != nil {
			return err
		}
	}
	switch {
	case to.unique && !from.unique:
		duplicates, err := target.duplicateCount(ctx, c.tx)
		if err != nil {
			return err
		}
		if duplicates > 0 {
			return countError("ERR_UNIQUE_HAS_DUPLICATES", "Some values are duplicated", duplicates)
		}
		return ddl.AddUnique(ctx, c.tx, target.table, target.column, true)
	case !to.unique && from.unique:
		return ddl.AddUnique(ctx, c.tx, target.table, target.column, false)
	}
	return nil
}

func (s *Service) fieldResult(ctx context.Context, c *txn, tableID pgtype.UUID, fieldID *pgtype.UUID) (FieldResult, error) {
	if err := c.q.BumpSchemaVersion(ctx, tableID); err != nil {
		return FieldResult{}, fmt.Errorf("bump schema version: %w", err)
	}
	schema, err := loadSchema(ctx, c.q, tableID, c.user, acl.Owner)
	if err != nil {
		return FieldResult{}, err
	}
	result := FieldResult{Schema: schema}
	if fieldID != nil {
		for index := range schema.Fields {
			if schema.Fields[index].ID == fieldID.String() {
				field := schema.Fields[index]
				result.Field = &field
			}
		}
	}
	return result, nil
}

// CreateField adds a typed native column and its metadata atomically. Only an
// effective owner of the live table may do so (D-123).
func (s *Service) CreateField(ctx context.Context, actor Actor, tableID string, input FieldInput) (FieldResult, error) {
	target, err := resourceOf(acl.Table, tableID)
	if err != nil {
		return FieldResult{}, err
	}
	name, err := cleanName(input.Name)
	if err != nil {
		return FieldResult{}, err
	}
	description, err := cleanDescription(input.Description)
	if err != nil {
		return FieldResult{}, err
	}
	bound, err := bindUserKind(input.UIType, input.Options)
	if err != nil {
		return FieldResult{}, err
	}
	value, err := resolveDefault(bound, input.Default)
	if err != nil {
		return FieldResult{}, err
	}
	if err := checkConstraints(bound, input.IsRequired, input.IsUnique, value.stored != nil); err != nil {
		return FieldResult{}, err
	}
	column := bound.Column()
	var result FieldResult
	err = retryCollisions(func() error {
		return s.run(ctx, actor, scope{target: target}, func(c *txn) error {
			table, err := lockTable(ctx, c, target.ID)
			if err != nil {
				return err
			}
			if err := checkTimezone(ctx, c.q, bound); err != nil {
				return err
			}
			physicalName, err := s.newName(ddl.Column)
			if err != nil {
				return err
			}
			last, err := c.q.MaxUserFieldPosition(ctx, target.ID)
			if err != nil {
				return fmt.Errorf("read field position: %w", err)
			}
			id, err := c.q.InsertUserField(ctx, structuresql.InsertUserFieldParams{TableID: target.ID, Name: name,
				Description: description, UiType: input.UIType, PgColumn: pgText(physicalName),
				PgType: pgText(pgTypeName(column.Type)), Options: bound.Options(), IsRequired: input.IsRequired,
				IsUnique: input.IsUnique, DefaultJson: value.stored, Position: last + positionStep})
			if err != nil {
				return fieldNameConflict(fmt.Errorf("insert field: %w", err))
			}
			physical := physicalColumn{table: table.physical, column: physicalName}
			if err := ddl.AddColumn(ctx, c.tx, table.physical, ddl.ColumnSpec{Name: physicalName, Type: column.Type,
				NotNull: column.NotNull, Default: column.Default}); err != nil {
				return err
			}
			if err := applyColumn(ctx, c, physical, column.Type, columnState{},
				columnState{required: input.IsRequired, unique: input.IsUnique, stored: value.stored},
				value, input.FillEmpty, false); err != nil {
				return err
			}
			result, err = s.fieldResult(ctx, c, target.ID, &id)
			return err
		})
	})
	return result, err
}

// fieldScope resolves a live field to its live table without locks; missing
// fields and hidden tables are indistinguishable 404s.
func (s *Service) fieldScope(ctx context.Context, id string) (pgtype.UUID, acl.Resource, error) {
	fieldID, ok := acl.ParseUUID(id)
	if !ok {
		return pgtype.UUID{}, acl.Resource{}, acl.ErrNotFound()
	}
	tableID, err := s.queries.FieldTable(ctx, fieldID)
	if err != nil {
		return pgtype.UUID{}, acl.Resource{}, notFoundIfNoRows(err, "read field table")
	}
	return fieldID, acl.Resource{Type: acl.Table, ID: tableID}, nil
}

func (c *txn) lockedField(ctx context.Context, table lockedTable, id pgtype.UUID, version int32) (structuresql.GetFieldRow, error) {
	field, err := c.q.GetField(ctx, structuresql.GetFieldParams{ID: id, TableID: table.ID})
	if err != nil {
		return field, notFoundIfNoRows(err, "read field")
	}
	if field.Version != version {
		return field, apperr.New(409, "ERR_VERSION_CONFLICT", "The field changed; reload and retry")
	}
	return field, nil
}

// choiceRenames maps changed labels of choices kept by id.
func choiceRenames(before, after codec.Field) (from, to []string) {
	type chooser interface{ Choices() []codec.Choice }
	old, okOld := before.(chooser)
	updated, okNew := after.(chooser)
	if !okOld || !okNew {
		return nil, nil
	}
	labels := map[string]string{}
	for _, choice := range old.Choices() {
		labels[choice.ID] = choice.Label
	}
	for _, choice := range updated.Choices() {
		if previous, ok := labels[choice.ID]; ok && previous != choice.Label {
			from, to = append(from, previous), append(to, choice.Label)
		}
	}
	return from, to
}

// renameDefault rewrites the labels of a stored select default.
func renameDefault(stored []byte, from, to []string) []byte {
	if len(stored) == 0 || len(from) == 0 {
		return stored
	}
	mapping := map[string]string{}
	for index := range from {
		mapping[from[index]] = to[index]
	}
	var single string
	if json.Unmarshal(stored, &single) == nil {
		if renamed, ok := mapping[single]; ok {
			encoded, _ := json.Marshal(renamed)
			return encoded
		}
		return stored
	}
	var labels []string
	if json.Unmarshal(stored, &labels) != nil {
		return stored
	}
	for index, label := range labels {
		if renamed, ok := mapping[label]; ok {
			labels[index] = renamed
		}
	}
	encoded, _ := json.Marshal(labels)
	return encoded
}

// UpdateField changes name, description, options, constraints, default and/or
// position under the table owner's lock, with If-Match versioning (D-124).
func (s *Service) UpdateField(ctx context.Context, actor Actor, id string, patch FieldPatch) (FieldResult, error) {
	fieldID, target, err := s.fieldScope(ctx, id)
	if err != nil {
		return FieldResult{}, err
	}
	if patch.Name == nil && patch.Description == nil && patch.Options == nil && patch.IsRequired == nil &&
		patch.IsUnique == nil && patch.Default == nil && patch.Index == nil {
		return FieldResult{}, validation("At least one field property is required")
	}
	var name, description string
	if patch.Name != nil {
		if name, err = cleanName(*patch.Name); err != nil {
			return FieldResult{}, err
		}
	}
	if patch.Description != nil {
		if description, err = cleanDescription(*patch.Description); err != nil {
			return FieldResult{}, err
		}
	}
	if patch.Options != nil && isJSONNull(patch.Options) {
		return FieldResult{}, validation("Options must be an object")
	}
	if patch.Index != nil && *patch.Index < 0 {
		return FieldResult{}, validation("Index is outside the sibling range")
	}
	var result FieldResult
	err = s.run(ctx, actor, scope{target: target}, func(c *txn) error {
		table, err := lockTable(ctx, c, target.ID)
		if err != nil {
			return err
		}
		field, err := c.lockedField(ctx, table, fieldID, patch.Version)
		if err != nil {
			return err
		}
		if patch.Name == nil {
			name = field.Name
		}
		if patch.Description == nil {
			description = field.Description
		}
		structural := patch.Options != nil || patch.IsRequired != nil || patch.IsUnique != nil || patch.Default != nil || patch.Index != nil
		if field.IsSystem {
			if structural {
				return systemFieldError()
			}
			if _, err := c.q.UpdateField(ctx, structuresql.UpdateFieldParams{ID: field.ID, Name: name, Description: description,
				Options: field.Options, IsRequired: field.IsRequired, IsUnique: field.IsUnique, DefaultJson: field.DefaultJson}); err != nil {
				return fieldNameConflict(fmt.Errorf("update field: %w", err))
			}
			result, err = s.fieldResult(ctx, c, table.ID, &field.ID)
			return err
		}
		before, err := codec.Bind(codec.Kind(field.UiType), field.Options)
		if err != nil {
			return fmt.Errorf("bind stored field options: %w", err)
		}
		after := before
		if patch.Options != nil {
			if after, err = bindUserKind(field.UiType, patch.Options); err != nil {
				return err
			}
			if after.Column() != before.Column() {
				return reasonError(400, "ERR_VALIDATION", "Options cannot change the stored column type", "storage_change")
			}
			if err := checkTimezone(ctx, c.q, after); err != nil {
				return err
			}
		}
		from, to := choiceRenames(before, after)
		rawDefault := json.RawMessage(renameDefault(field.DefaultJson, from, to))
		if patch.Default != nil {
			rawDefault = patch.Default
		}
		value, err := resolveDefault(after, rawDefault)
		if err != nil {
			return err
		}
		required, unique := field.IsRequired, field.IsUnique
		if patch.IsRequired != nil {
			required = *patch.IsRequired
		}
		if patch.IsUnique != nil {
			unique = *patch.IsUnique
		}
		// Constraints already set stay legal; only enabling is checked.
		if err := checkConstraints(after, required && !field.IsRequired, unique && !field.IsUnique,
			value.stored != nil && !bytes.Equal(value.stored, field.DefaultJson)); err != nil {
			return err
		}
		physical := physicalColumn{table: table.physical, column: field.PgColumn.String}
		if len(from) > 0 {
			if err := physical.renameLabels(ctx, c.tx, after.Kind() == codec.MultiSelect, from, to); err != nil {
				return err
			}
		}
		if err := applyColumn(ctx, c, physical, after.Column().Type,
			columnState{required: field.IsRequired, unique: field.IsUnique, stored: field.DefaultJson},
			columnState{required: required, unique: unique, stored: value.stored}, value, patch.FillEmpty, false); err != nil {
			return err
		}
		if _, err := c.q.UpdateField(ctx, structuresql.UpdateFieldParams{ID: field.ID, Name: name, Description: description,
			Options: after.Options(), IsRequired: required, IsUnique: unique, DefaultJson: value.stored}); err != nil {
			return fieldNameConflict(fmt.Errorf("update field: %w", err))
		}
		if patch.Index != nil {
			if err := moveField(ctx, c, table.ID, field.ID, *patch.Index); err != nil {
				return err
			}
		}
		result, err = s.fieldResult(ctx, c, table.ID, &field.ID)
		return err
	})
	return result, err
}

func moveField(ctx context.Context, c *txn, tableID, fieldID pgtype.UUID, index int) error {
	rows, err := c.q.ListLiveUserFieldOrder(ctx, tableID)
	if err != nil {
		return fmt.Errorf("list field order: %w", err)
	}
	siblings := make([]sibling, len(rows))
	for position, row := range rows {
		siblings[position] = sibling{ID: row.ID, Position: row.Position}
	}
	return reorder(siblings, fieldID, index, func(position float64) error {
		return c.q.SetFieldPosition(ctx, structuresql.SetFieldPositionParams{ID: fieldID, Position: position})
	}, func(order []pgtype.UUID) error {
		return c.q.RenumberFields(ctx, order)
	})
}

// DeleteField soft-deletes a user field: metadata is hidden, the column and its
// data stay, NOT NULL is relaxed and a deleted display field is replaced (D-129).
func (s *Service) DeleteField(ctx context.Context, actor Actor, id string, version int32) (FieldResult, error) {
	fieldID, target, err := s.fieldScope(ctx, id)
	if err != nil {
		return FieldResult{}, err
	}
	var result FieldResult
	err = s.run(ctx, actor, scope{target: target}, func(c *txn) error {
		table, err := lockTable(ctx, c, target.ID)
		if err != nil {
			return err
		}
		field, err := c.lockedField(ctx, table, fieldID, version)
		if err != nil {
			return err
		}
		if field.IsSystem {
			return systemFieldError()
		}
		if table.DisplayFieldID == field.ID {
			fields, err := c.q.ListTableFields(ctx, table.ID)
			if err != nil {
				return fmt.Errorf("list table fields: %w", err)
			}
			var replacement pgtype.UUID
			for _, candidate := range fields {
				if candidate.ID != field.ID && displayEligible(candidate.UiType, candidate.IsSystem) {
					replacement = candidate.ID
					break
				}
			}
			if !replacement.Valid {
				return apperr.New(409, "ERR_DISPLAY_FIELD_REQUIRED", "Add another eligible field before deleting the display field")
			}
			if _, err := c.q.SetTableDisplayField(ctx, structuresql.SetTableDisplayFieldParams{ID: table.ID,
				DisplayFieldID: replacement}); err != nil {
				return notFoundIfNoRows(err, "replace display field")
			}
		}
		bound, err := codec.Bind(codec.Kind(field.UiType), field.Options)
		if err != nil {
			return fmt.Errorf("bind stored field options: %w", err)
		}
		// A hidden column must not receive user defaults: with a unique
		// constraint they would make every later row insert collide.
		if field.DefaultJson != nil {
			if err := ddl.SetDefault(ctx, c.tx, table.physical, field.PgColumn.String, bound.Column().Type,
				bound.Column().Default); err != nil {
				return err
			}
		}
		if field.IsRequired {
			if !bound.Column().NotNull {
				if err := ddl.SetNotNull(ctx, c.tx, table.physical, field.PgColumn.String, false); err != nil {
					return err
				}
			}
		}
		if err := c.q.TrashField(ctx, field.ID); err != nil {
			return fmt.Errorf("trash field: %w", err)
		}
		result, err = s.fieldResult(ctx, c, table.ID, nil)
		return err
	})
	return result, err
}

// setDisplayField validates and stores the display field inside an
// UpdateTable transaction that already holds the owner locks.
func setDisplayField(ctx context.Context, c *txn, tableID pgtype.UUID, fieldID string) error {
	id, ok := acl.ParseUUID(fieldID)
	if !ok {
		return validation("Display field must be a live field of this table")
	}
	table, err := lockTable(ctx, c, tableID)
	if err != nil {
		return err
	}
	field, err := c.q.GetField(ctx, structuresql.GetFieldParams{ID: id, TableID: table.ID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return validation("Display field must be a live field of this table")
		}
		return fmt.Errorf("read display field: %w", err)
	}
	if !displayEligible(field.UiType, field.IsSystem) {
		return reasonError(400, "ERR_VALIDATION", "This field cannot be the display field", "display_ineligible")
	}
	if table.DisplayFieldID == field.ID {
		return nil
	}
	if _, err := c.q.SetTableDisplayField(ctx, structuresql.SetTableDisplayFieldParams{ID: table.ID, DisplayFieldID: field.ID}); err != nil {
		return notFoundIfNoRows(err, "set display field")
	}
	if err := c.q.BumpSchemaVersion(ctx, table.ID); err != nil {
		return fmt.Errorf("bump schema version: %w", err)
	}
	return nil
}
