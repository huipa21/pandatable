package structure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl"
	"pandatable/internal/apperr"
	"pandatable/internal/codec"
	"pandatable/internal/ddl"
	"pandatable/internal/structure/structuresql"
)

type TypeChangeTarget struct {
	UIType           string
	Options          json.RawMessage
	Default          json.RawMessage
	SetFailingToNull bool
}

type TypeChangeSample struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

type TypeChangeDryRun struct {
	Total       int64              `json:"total"`
	Convertible int64              `json:"convertible"`
	Failing     int64              `json:"failing"`
	Samples     []TypeChangeSample `json:"samples"`
}

type typeChangePlan struct {
	source codec.Field
	target codec.Field
	mode   ddl.CastMode
}

func typeChangeable(kind string) bool {
	switch kind {
	case "link", "lookup", "rollup", "formula", "attachment", "autonumber":
		return false
	default:
		return !codec.IsSystem(codec.Kind(kind))
	}
}

func makeTypeChangePlan(ctx context.Context, c *txn, field structuresql.GetFieldRow, input TypeChangeTarget) (typeChangePlan, error) {
	if field.IsSystem || !field.PgColumn.Valid || !typeChangeable(field.UiType) ||
		!typeChangeable(input.UIType) || field.UiType == input.UIType {
		return typeChangePlan{}, reasonError(400, "ERR_VALIDATION", "This field type cannot be converted", "type_change_unsupported")
	}
	sourceOptions, err := fieldOptionsForCodec(field.Options)
	if err != nil {
		return typeChangePlan{}, err
	}
	source, err := bindUserKind(field.UiType, sourceOptions)
	if err != nil {
		return typeChangePlan{}, err
	}
	options := input.Options
	if len(options) == 0 {
		options = json.RawMessage("{}")
		if (input.UIType == string(codec.SingleSelect) || input.UIType == string(codec.MultiSelect)) &&
			(field.UiType == string(codec.SingleSelect) || field.UiType == string(codec.MultiSelect)) {
			options = source.Options()
		}
	}
	if isJSONNull(options) {
		return typeChangePlan{}, validation("Options must be an object")
	}
	target, err := bindUserKind(input.UIType, options)
	if err != nil {
		return typeChangePlan{}, err
	}
	if target.Kind() == codec.Currency {
		target, err = bindCreationCurrency(ctx, c.q, options, target)
		if err != nil {
			return typeChangePlan{}, err
		}
	}
	if err := checkTimezone(ctx, c.q, target); err != nil {
		return typeChangePlan{}, err
	}
	mode := ddl.CastText
	if source.Kind() == codec.MultiSelect && target.Kind() == codec.SingleSelect {
		mode = ddl.CastFirstArrayElement
	}
	return typeChangePlan{source: source, target: target, mode: mode}, nil
}

func conversionDescriptor(field codec.Field, mode ddl.CastMode, setFailingToNull bool) ddl.TypeConversion {
	column := field.Column()
	return ddl.TypeConversion{
		Type: column.Type, Mode: mode, RangeCheck: codecRangeCheck(column),
		Reference: codecReference(column), SetFailuresToNil: setFailingToNull,
	}
}

func codecRangeCheck(column codec.Column) *ddl.IntRangeCheck {
	if !column.HasRangeCheck {
		return nil
	}
	return &ddl.IntRangeCheck{Min: column.CheckMin, Max: column.CheckMax}
}

func codecReference(column codec.Column) ddl.Reference {
	if column.UserReference {
		return ddl.UserReference
	}
	return ddl.NoReference
}

func typeChangeDryRun(ctx context.Context, tx pgx.Tx, table ddl.TableName, column string, target codec.Field, mode ddl.CastMode) (TypeChangeDryRun, error) {
	physical := physicalColumn{table: table, column: column}
	tableSQL, columnSQL, err := physical.sql()
	if err != nil {
		return TypeChangeDryRun{}, err
	}
	descriptor := conversionDescriptor(target, mode, false)
	valid, err := ddl.ConversionValidity(column, descriptor)
	if err != nil {
		return TypeChangeDryRun{}, err
	}
	query := fmt.Sprintf(`SELECT count(*) FILTER (WHERE %[1]s IS NOT NULL),
		count(*) FILTER (WHERE %[1]s IS NOT NULL AND (%[2]s))
		FROM %[3]s`, columnSQL, valid, tableSQL)
	var result TypeChangeDryRun
	if err := tx.QueryRow(ctx, query).Scan(&result.Total, &result.Convertible); err != nil {
		return TypeChangeDryRun{}, fmt.Errorf("count type conversion values: %w", err)
	}
	result.Failing = result.Total - result.Convertible
	result.Samples = make([]TypeChangeSample, 0, 5)
	if result.Failing == 0 {
		return result, nil
	}
	sampleQuery := fmt.Sprintf(`SELECT "id", %[1]s::text FROM %[2]s
		WHERE %[1]s IS NOT NULL AND NOT (%[3]s) ORDER BY "id" LIMIT 5`, columnSQL, tableSQL, valid)
	rows, err := tx.Query(ctx, sampleQuery)
	if err != nil {
		return TypeChangeDryRun{}, fmt.Errorf("read type conversion samples: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var value string
		if err := rows.Scan(&id, &value); err != nil {
			return TypeChangeDryRun{}, fmt.Errorf("scan type conversion sample: %w", err)
		}
		result.Samples = append(result.Samples, TypeChangeSample{ID: strconv.FormatInt(id, 10), Value: value})
	}
	if err := rows.Err(); err != nil {
		return TypeChangeDryRun{}, fmt.Errorf("iterate type conversion samples: %w", err)
	}
	return result, nil
}

func (s *Service) DryRunTypeChange(ctx context.Context, actor Actor, id string, version int32, input TypeChangeTarget) (TypeChangeDryRun, error) {
	fieldID, target, err := s.fieldScope(ctx, id)
	if err != nil {
		return TypeChangeDryRun{}, err
	}
	var result TypeChangeDryRun
	err = s.run(ctx, actor, scope{target: target, readOnly: true}, func(c *txn) error {
		table, err := lockTable(ctx, c, target.ID)
		if err != nil {
			return err
		}
		field, err := c.lockedField(ctx, table, fieldID, version)
		if err != nil {
			return err
		}
		plan, err := makeTypeChangePlan(ctx, c, field, input)
		if err != nil {
			return err
		}
		result, err = typeChangeDryRun(ctx, c.tx, table.physical, field.PgColumn.String, plan.target, plan.mode)
		return err
	})
	return result, err
}

func convertDefaultRaw(source, target codec.Field, raw []byte, mode ddl.CastMode) (json.RawMessage, error) {
	if len(raw) == 0 || isJSONNull(raw) {
		return nil, nil
	}
	value, err := source.Decode(raw)
	if err != nil {
		return nil, fmt.Errorf("decode existing field default: %w", err)
	}
	if mode == ddl.CastFirstArrayElement {
		labels, ok := value.([]string)
		if !ok {
			return nil, errors.New("multi-select default has an unexpected value")
		}
		if len(labels) == 0 {
			return nil, nil
		}
		value = labels[0]
	}
	text := fmt.Sprint(value)
	var converted json.RawMessage
	switch target.Kind() {
	case codec.Integer, codec.Decimal, codec.Float, codec.Currency, codec.Percent, codec.Rating:
		if !json.Valid([]byte(text)) {
			return nil, validation("The existing default cannot be converted to the new field type")
		}
		converted = json.RawMessage(text)
	case codec.Checkbox:
		boolean := strings.ToLower(text)
		switch boolean {
		case "true", "t", "yes", "y", "on", "1":
			converted = json.RawMessage("true")
		case "false", "f", "no", "n", "off", "0":
			converted = json.RawMessage("false")
		default:
			return nil, validation("The existing default cannot be converted to the new field type")
		}
	case codec.JSON:
		if rawValue, ok := value.(json.RawMessage); ok && json.Valid(rawValue) {
			converted = append(json.RawMessage(nil), rawValue...)
		} else if json.Valid([]byte(text)) {
			converted = json.RawMessage(text)
		} else {
			return nil, validation("The existing default cannot be converted to the new field type")
		}
	case codec.MultiSelect, codec.TextArray, codec.IntArray, codec.MultiUser:
		if _, ok := value.([]string); ok {
			converted, err = json.Marshal(value)
		} else {
			converted, err = json.Marshal([]string{text})
		}
		if err != nil {
			return nil, fmt.Errorf("encode converted field default: %w", err)
		}
	default:
		converted, err = json.Marshal(text)
		if err != nil {
			return nil, fmt.Errorf("encode converted field default: %w", err)
		}
	}
	decoded, err := target.Decode(converted)
	if err != nil {
		return nil, validation("The existing default cannot be converted to the new field type")
	}
	return target.Encode(decoded)
}

func conversionFailure(stats TypeChangeDryRun) error {
	result := apperr.New(409, "ERR_CONVERSION_FAILED", "Some values cannot be converted")
	result.Details = map[string]any{"count": stats.Failing, "samples": stats.Samples}
	return result
}

func (s *Service) ConvertFieldType(ctx context.Context, actor Actor, id string, version int32, input TypeChangeTarget) (FieldResult, error) {
	fieldID, target, err := s.fieldScope(ctx, id)
	if err != nil {
		return FieldResult{}, err
	}
	var result FieldResult
	err = s.run(ctx, actor, scope{target: target, schemaChanged: true}, func(c *txn) error {
		table, err := lockTable(ctx, c, target.ID)
		if err != nil {
			return err
		}
		field, err := c.lockedField(ctx, table, fieldID, version)
		if err != nil {
			return err
		}
		plan, err := makeTypeChangePlan(ctx, c, field, input)
		if err != nil {
			return err
		}
		stats, err := typeChangeDryRun(ctx, c.tx, table.physical, field.PgColumn.String, plan.target, plan.mode)
		if err != nil {
			return err
		}
		if stats.Failing > 0 && !input.SetFailingToNull {
			return conversionFailure(stats)
		}
		targetColumn := plan.target.Column()
		if stats.Failing > 0 && input.SetFailingToNull && targetColumn.NotNull {
			return reasonError(409, "ERR_CONVERSION_FAILED", "This field type cannot store failed values as NULL", "required_type")
		}
		required := field.IsRequired && plan.target.Constraints().Required
		if stats.Failing > 0 && input.SetFailingToNull {
			required = false
		}
		unique := field.IsUnique && plan.target.Constraints().Unique
		rawDefault := input.Default
		if rawDefault == nil {
			rawDefault, err = convertDefaultRaw(plan.source, plan.target, field.DefaultJson, plan.mode)
			if err != nil {
				return err
			}
		}
		defaultValue, err := resolveDefault(plan.target, rawDefault)
		if err != nil {
			return err
		}
		physical := physicalColumn{table: table.physical, column: field.PgColumn.String}
		sourceColumn := plan.source.Column()
		if field.IsUnique {
			if err := ddl.AddUnique(ctx, c.tx, table.physical, field.PgColumn.String, false); err != nil {
				return err
			}
		}
		if sourceColumn.HasRangeCheck {
			if err := ddl.SetRangeCheck(ctx, c.tx, table.physical, field.PgColumn.String, nil); err != nil {
				return err
			}
		}
		if sourceColumn.UserReference {
			if err := ddl.SetUserReference(ctx, c.tx, table.physical, field.PgColumn.String, false); err != nil {
				return err
			}
		}
		if err := ddl.SetDefault(ctx, c.tx, table.physical, field.PgColumn.String, sourceColumn.Type, ddl.DefaultValue{}); err != nil {
			return err
		}
		if sourceColumn.NotNull || field.IsRequired {
			if err := ddl.SetNotNull(ctx, c.tx, table.physical, field.PgColumn.String, false); err != nil {
				return err
			}
		}
		descriptor := conversionDescriptor(plan.target, plan.mode, input.SetFailingToNull)
		if err := ddl.AlterColumnType(ctx, c.tx, table.physical, field.PgColumn.String, descriptor); err != nil {
			return err
		}
		if check := codecRangeCheck(targetColumn); check != nil {
			if err := ddl.SetRangeCheck(ctx, c.tx, table.physical, field.PgColumn.String, check); err != nil {
				return err
			}
		}
		if targetColumn.UserReference {
			if err := ddl.SetUserReference(ctx, c.tx, table.physical, field.PgColumn.String, true); err != nil {
				return err
			}
		}
		if targetColumn.NotNull || required {
			if err := ddl.SetNotNull(ctx, c.tx, table.physical, field.PgColumn.String, true); err != nil {
				return err
			}
		}
		if defaultValue.ddl.Kind != ddl.NoDefault {
			if err := ddl.SetDefault(ctx, c.tx, table.physical, field.PgColumn.String, targetColumn.Type, defaultValue.ddl); err != nil {
				return err
			}
		}
		if unique {
			duplicates, err := physical.duplicateCount(ctx, c.tx)
			if err != nil {
				return err
			}
			if duplicates > 0 {
				return countError("ERR_UNIQUE_HAS_DUPLICATES", "Some converted values are duplicated", duplicates)
			}
			if err := ddl.AddUnique(ctx, c.tx, table.physical, field.PgColumn.String, true); err != nil {
				return err
			}
		}
		options := plan.target.Options()
		if _, err := c.q.ConvertField(ctx, structuresql.ConvertFieldParams{
			ID: field.ID, UiType: input.UIType, PgType: pgtype.Text{String: pgTypeName(targetColumn.Type), Valid: true},
			Options: options, IsRequired: required, IsUnique: unique, DefaultJson: defaultValue.stored,
		}); err != nil {
			return fmt.Errorf("save converted field metadata: %w", err)
		}
		if err := recheckTypeDependencies(ctx, c, table, field.ID, codec.Kind(input.UIType)); err != nil {
			return err
		}
		changes, err := json.Marshal(map[string]string{
			"fieldId": field.ID.String(), "from": field.UiType, "to": input.UIType,
		})
		if err != nil {
			return fmt.Errorf("encode type change history: %w", err)
		}
		if err := c.q.InsertTypeChangeHistory(ctx, structuresql.InsertTypeChangeHistoryParams{
			TableID: table.ID, UserID: c.user, Changes: changes,
		}); err != nil {
			return fmt.Errorf("record type change history: %w", err)
		}
		result, err = s.fieldResult(ctx, c, table.ID, &field.ID)
		return err
	})
	return result, err
}

func recheckTypeDependencies(ctx context.Context, c *txn, table lockedTable, fieldID pgtype.UUID, targetKind codec.Kind) error {
	dependents, err := c.q.ListComputedDependents(ctx, fieldID.String())
	if err != nil {
		return fmt.Errorf("list computed type dependencies: %w", err)
	}
	type pendingDependency struct {
		id      pgtype.UUID
		options []byte
	}
	pending := make([]pendingDependency, 0)
	tableIDs := make(map[string]pgtype.UUID)
	for _, field := range dependents {
		tableIDs[field.TableID.String()] = field.TableID
		var options computedOptions
		if err := json.Unmarshal(field.Options, &options); err != nil {
			return fmt.Errorf("decode computed options: %w", err)
		}
		if options.TargetFieldID != fieldID.String() {
			continue
		}
		next := options
		if field.UiType == "rollup" {
			if err := validateRollupAggregate(options.Aggregate, targetKind); err != nil {
				next.Error = "dependency_type"
			} else if next.Error == "dependency_type" {
				next.Error = ""
			}
		} else if next.Error == "dependency_type" {
			next.Error = ""
		}
		if next == options {
			continue
		}
		encoded, err := json.Marshal(next)
		if err != nil {
			return fmt.Errorf("encode computed dependency status: %w", err)
		}
		pending = append(pending, pendingDependency{id: field.ID, options: encoded})
	}
	orderedTableIDs := make([]string, 0, len(tableIDs))
	for id := range tableIDs {
		orderedTableIDs = append(orderedTableIDs, id)
	}
	sort.Strings(orderedTableIDs)
	for _, id := range orderedTableIDs {
		if _, err := c.q.LockDependencyTable(ctx, tableIDs[id]); err != nil {
			return fmt.Errorf("lock computed dependency table: %w", err)
		}
	}
	for _, item := range pending {
		if err := c.q.SaveDependencyFieldOptions(ctx, structuresql.SaveDependencyFieldOptionsParams{
			ID: item.id, Options: item.options,
		}); err != nil {
			return fmt.Errorf("save computed dependency status: %w", err)
		}
	}
	for _, id := range orderedTableIDs {
		tableID := tableIDs[id]
		if tableID == table.ID {
			continue
		}
		if err := c.q.BumpDependencySchemaVersion(ctx, tableID); err != nil {
			return fmt.Errorf("bump computed dependency schema version: %w", err)
		}
	}
	if err := recheckFormulaTable(ctx, c, table.ID, table.PgSchema, table.PgName, table.SchemaVersion); err != nil {
		return err
	}
	for _, id := range orderedTableIDs {
		tableID := tableIDs[id]
		if tableID == table.ID {
			continue
		}
		rowTable, err := c.q.GetRowTable(ctx, tableID)
		if err != nil {
			return fmt.Errorf("read computed dependency table: %w", err)
		}
		if err := recheckFormulaTable(ctx, c, rowTable.ID, rowTable.PgSchema, rowTable.PgName, rowTable.SchemaVersion); err != nil {
			return err
		}
		if err := emitSchema(ctx, c.tx, c.actor, tableID); err != nil {
			return err
		}
	}
	return nil
}

func recheckFormulaTable(ctx context.Context, c *txn, tableID pgtype.UUID, schema, name string, version int32) error {
	rowTable, err := loadRowTable(ctx, c.q, c.tx, tableID, schema, name, version)
	if err != nil {
		return err
	}
	for index := range rowTable.fields {
		field := rowTable.fields[index]
		if field.formula == nil {
			continue
		}
		next := field.formula.options
		resultType, typeErr := inferFormulaType(next.AST, rowTable, field.id, nil)
		if typeErr != nil {
			next.Error = "dependency_type"
		} else {
			next.ResultType = resultType
			if next.Error == "dependency_type" {
				next.Error = ""
			}
		}
		if reflect.DeepEqual(next, field.formula.options) {
			continue
		}
		encoded, err := json.Marshal(next)
		if err != nil {
			return fmt.Errorf("encode formula dependency status: %w", err)
		}
		id, ok := acl.ParseUUID(field.id)
		if !ok {
			return errors.New("formula field has an invalid metadata id")
		}
		if err := c.q.SaveDependencyFieldOptions(ctx, structuresql.SaveDependencyFieldOptionsParams{
			ID: id, Options: encoded,
		}); err != nil {
			return fmt.Errorf("save formula dependency status: %w", err)
		}
		field.formula.options = next
		field.error = next.Error
		rowTable.fields[index] = field
		rowTable.byID[field.id] = field
	}
	return nil
}
