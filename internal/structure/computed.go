package structure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl"
	"pandatable/internal/codec"
	"pandatable/internal/ddl"
)

type computedOptions struct {
	RelationFieldID string `json:"relationFieldId"`
	TargetFieldID   string `json:"targetFieldId"`
	Aggregate       string `json:"aggregate,omitempty"`
	Error           string `json:"error,omitempty"`
}

type computedField struct {
	options      computedOptions
	relation     relationRecord
	relationSide string
	targetTable  pgtype.UUID
	targetSchema string
	targetName   string
	targetColumn string
	targetKind   codec.Kind
	targetCodec  codec.Field
	aggregate    string
}

func decodeComputedOptions(kind string, raw json.RawMessage) (computedOptions, error) {
	var options computedOptions
	if len(raw) == 0 {
		return options, validation("Computed field options are required")
	}
	if err := json.Unmarshal(raw, &options); err != nil {
		return options, validation("Computed field options are invalid")
	}
	relationFieldID, ok := acl.ParseUUID(options.RelationFieldID)
	if !ok || relationFieldID.String() != options.RelationFieldID {
		return options, validation("Computed field relation is invalid")
	}
	targetFieldID, ok := acl.ParseUUID(options.TargetFieldID)
	if !ok || targetFieldID.String() != options.TargetFieldID {
		return options, validation("Computed field target is invalid")
	}
	if kind == "lookup" {
		if options.Aggregate != "" {
			return options, validation("Lookup fields do not have an aggregate")
		}
		return options, nil
	}
	switch options.Aggregate {
	case "count", "sum", "avg", "min", "max", "earliest", "latest", "concat":
		return options, nil
	default:
		return options, validation("Rollup aggregate is invalid")
	}
}

func computedTarget(ctx context.Context, tx pgx.Tx, table rowTable, kind string, raw json.RawMessage, sourceField rowField) (*computedField, error) {
	options, err := decodeComputedOptions(kind, raw)
	if err != nil {
		return nil, err
	}
	if sourceField.id != options.RelationFieldID || sourceField.relation == nil {
		return nil, validation("Computed field relation is invalid")
	}
	relation := *sourceField.relation
	var targetTable pgtype.UUID
	if table.id == relation.parentTable {
		targetTable = relation.childTable
	} else if table.id == relation.childTable {
		targetTable = relation.parentTable
	} else {
		return nil, errors.New("computed relation does not include source table")
	}
	if sourceField.relationSide != "parent" && sourceField.relationSide != "child" {
		return nil, errors.New("computed relation has invalid source side")
	}
	var schema, name, uiType string
	var column pgtype.Text
	var fieldOptions []byte
	var system bool
	targetFieldID, ok := acl.ParseUUID(options.TargetFieldID)
	if !ok || targetFieldID.String() != options.TargetFieldID {
		return nil, validation("Computed field target is invalid")
	}
	err = tx.QueryRow(ctx, `SELECT b.pg_schema,t.pg_name,f.ui_type,f.pg_column,f.options,f.is_system
		FROM meta.fields f JOIN meta.tables t ON t.id=f.table_id
		JOIN meta.books b ON b.id=t.book_id
		WHERE f.id=$1 AND f.table_id=$2 AND f.deleted_at IS NULL AND t.deleted_at IS NULL`,
		targetFieldID, targetTable).Scan(&schema, &name, &uiType, &column, &fieldOptions, &system)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, validation("Computed field target is invalid")
	}
	if err != nil {
		return nil, fmt.Errorf("read computed target field: %w", err)
	}
	if uiType == "link" || uiType == "lookup" || uiType == "rollup" {
		return nil, validation("Computed field target must be a stored value field")
	}
	targetKind := codec.Kind(uiType)
	targetCodec, err := codec.Bind(targetKind, fieldOptions)
	if err != nil {
		return nil, fmt.Errorf("bind computed target field: %w", err)
	}
	targetColumn := ""
	if system {
		systemColumnName, found := systemColumn(targetKind)
		if !found {
			return nil, errors.New("computed target system field has no column")
		}
		targetColumn = systemColumnName
	} else {
		if !column.Valid {
			return nil, errors.New("computed target field is missing its column")
		}
		targetColumn = column.String
	}
	if err := ddl.ValidateIdentifier(ddl.Book, schema); err != nil {
		return nil, fmt.Errorf("validate computed target schema: %w", err)
	}
	if err := ddl.ValidateIdentifier(ddl.Table, name); err != nil {
		return nil, fmt.Errorf("validate computed target table: %w", err)
	}
	if err := ddl.ValidateIdentifier(ddl.Column, targetColumn); err != nil {
		return nil, fmt.Errorf("validate computed target column: %w", err)
	}
	aggregate := ""
	if kind == "rollup" {
		aggregate = options.Aggregate
		if err := validateRollupAggregate(aggregate, targetKind); err != nil {
			return nil, err
		}
	}
	return &computedField{
		options: options, relation: relation, relationSide: sourceField.relationSide,
		targetTable: targetTable, targetSchema: schema, targetName: name,
		targetColumn: targetColumn, targetKind: targetKind, targetCodec: targetCodec, aggregate: aggregate,
	}, nil
}

func validateRollupAggregate(aggregate string, kind codec.Kind) error {
	numeric := map[codec.Kind]bool{
		codec.Integer: true, codec.Decimal: true, codec.Float: true,
		codec.Currency: true, codec.Percent: true, codec.Rating: true,
	}
	temporal := map[codec.Kind]bool{codec.Date: true, codec.DateTime: true}
	switch aggregate {
	case "sum", "avg":
		if !numeric[kind] {
			return validation("Sum and average require a numeric target field")
		}
	case "earliest", "latest":
		if !temporal[kind] {
			return validation("Earliest and latest require a date or datetime target field")
		}
	case "min", "max":
		orderable := map[codec.Kind]bool{
			codec.Text: true, codec.LongText: true, codec.Email: true, codec.URL: true,
			codec.SingleSelect: true, codec.Integer: true, codec.Decimal: true,
			codec.Float: true, codec.Currency: true, codec.Percent: true, codec.Rating: true,
			codec.Date: true, codec.DateTime: true, codec.Time: true, codec.AutoNumber: true,
		}
		if !orderable[kind] {
			return validation("Minimum and maximum do not support this target field")
		}
	}
	return nil
}

func (field *computedField) relatedFrom(source rowTable) (string, error) {
	if err := ddl.ValidateIdentifier(ddl.Book, field.targetSchema); err != nil {
		return "", err
	}
	target := pgx.Identifier{"target"}.Sanitize()
	targetName := pgx.Identifier{field.targetSchema, field.targetName}.Sanitize()
	sourceID := pgx.Identifier{"source", "id"}.Sanitize()
	switch field.relation.kind {
	case "one_one", "one_many":
		if !field.relation.fkColumn.Valid {
			return "", errors.New("computed relation is missing its foreign key")
		}
		if err := ddl.ValidateIdentifier(ddl.Column, field.relation.fkColumn.String); err != nil {
			return "", err
		}
		fk := pgx.Identifier{"target", field.relation.fkColumn.String}.Sanitize()
		if field.relationSide == "parent" {
			return "FROM " + targetName + " AS " + target + " WHERE " + fk + "=" + sourceID, nil
		}
		sourceFK := pgx.Identifier{"source", field.relation.fkColumn.String}.Sanitize()
		return "FROM " + targetName + " AS " + target + " WHERE " + sourceFK + "=" + pgx.Identifier{"target", "id"}.Sanitize(), nil
	case "many_many":
		if !field.relation.junction.Valid {
			return "", errors.New("computed relation is missing its junction")
		}
		if err := ddl.ValidateIdentifier(ddl.Junction, field.relation.junction.String); err != nil {
			return "", err
		}
		linkName := pgx.Identifier{field.targetSchema, field.relation.junction.String}.Sanitize()
		sourceColumn, targetColumn := "parent_id", "child_id"
		if field.relationSide == "child" {
			sourceColumn, targetColumn = "child_id", "parent_id"
		}
		return "FROM " + targetName + " AS " + target + " JOIN " + linkName +
			` AS "links" ON ` + pgx.Identifier{"links", targetColumn}.Sanitize() + "=" +
			pgx.Identifier{"target", "id"}.Sanitize() + " WHERE " +
			pgx.Identifier{"links", sourceColumn}.Sanitize() + "=" + sourceID, nil
	default:
		return "", errors.New("computed relation kind is invalid")
	}
}

func computedExpression(source rowTable, field rowField) (string, error) {
	if field.computed == nil {
		return "", errors.New("field is not computed")
	}
	related, err := field.computed.relatedFrom(source)
	if err != nil {
		return "", err
	}
	column := pgx.Identifier{"target", field.computed.targetColumn}.Sanitize()
	var expression string
	if field.kind == "lookup" {
		value := "to_jsonb(" + column + ")"
		if field.computed.targetKind == codec.Decimal || field.computed.targetKind == codec.Currency ||
			field.computed.targetKind == codec.Percent {
			value = "to_jsonb(" + column + "::text)"
		}
		expression = "(SELECT COALESCE(jsonb_agg(" + value + " ORDER BY \"target\".\"id\"), '[]'::jsonb) " + related + ")"
	} else {
		aggregate := ""
		switch field.computed.aggregate {
		case "count":
			aggregate = "count(*)"
		case "sum":
			aggregate = "sum(" + column + ")"
		case "avg":
			aggregate = "avg(" + column + ")"
		case "min", "earliest":
			aggregate = "min(" + column + ")"
		case "max", "latest":
			aggregate = "max(" + column + ")"
		case "concat":
			aggregate = "string_agg(" + column + "::text, ', ' ORDER BY \"target\".\"id\")"
		default:
			return "", errors.New("computed rollup aggregate is invalid")
		}
		expression = "(SELECT COALESCE(to_jsonb(" + aggregate + "), 'null'::jsonb) " + related + ")"
	}
	return expression, nil
}

func enrichRowComputed(ctx context.Context, tx pgx.Tx, actor Actor, table rowTable, fields []rowField, rows []RowRecord) error {
	if len(rows) == 0 {
		return nil
	}
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return acl.ErrValidation()
	}
	ids := make([]int64, len(rows))
	for index, row := range rows {
		id, err := parsePathRowID(row.ID)
		if err != nil {
			return err
		}
		ids[index] = id
	}
	for _, field := range fields {
		if field.computed == nil {
			if field.error != "" {
				for index := range rows {
					rows[index].Fields[field.id] = json.RawMessage(`"#ERROR"`)
				}
			}
			continue
		}
		target := acl.Resource{Type: acl.Table, ID: field.computed.targetTable}
		access, err := acl.AccessUncached(ctx, tx, user, target)
		if err != nil {
			return fmt.Errorf("check computed target access: %w", err)
		}
		if !access.Visible || access.Check(acl.Read) != nil {
			for index := range rows {
				rows[index].Fields[field.id] = json.RawMessage(`"🔒"`)
			}
			continue
		}
		expression, err := computedExpression(table, field)
		if err != nil {
			return fmt.Errorf("compile computed field: %w", err)
		}
		source := pgx.Identifier{table.schema, table.name}.Sanitize()
		query := "SELECT \"source\".\"id\", " + expression +
			" FROM " + source + ` AS "source" WHERE "source"."id" = ANY($1::bigint[])`
		byID := make(map[int64]int, len(rows))
		for index, id := range ids {
			byID[id] = index
		}
		result, err := tx.Query(ctx, query, ids)
		if err != nil {
			return fmt.Errorf("query computed field: %w", err)
		}
		for result.Next() {
			var id int64
			var value []byte
			if err := result.Scan(&id, &value); err != nil {
				result.Close()
				return fmt.Errorf("scan computed value: %w", err)
			}
			index, found := byID[id]
			if !found || !json.Valid(value) {
				result.Close()
				return errors.New("computed query returned an invalid row or value")
			}
			rows[index].Fields[field.id] = append(json.RawMessage(nil), value...)
		}
		if err := result.Err(); err != nil {
			result.Close()
			return fmt.Errorf("iterate computed values: %w", err)
		}
		result.Close()
	}
	return nil
}

func cleanComputedOptions(kind string, raw json.RawMessage) (json.RawMessage, error) {
	options, err := decodeComputedOptions(kind, raw)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(options)
	if err != nil {
		return nil, fmt.Errorf("encode computed options: %w", err)
	}
	return encoded, nil
}

func computedFieldUses(ctx context.Context, tx pgx.Tx, fieldIDs []pgtype.UUID) (bool, error) {
	if len(fieldIDs) == 0 {
		return false, nil
	}
	ids := make([]string, len(fieldIDs))
	for index, id := range fieldIDs {
		ids[index] = id.String()
	}
	var used bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM meta.fields
		WHERE deleted_at IS NULL AND ui_type IN ('lookup','rollup')
		AND (options->>'relationFieldId'=ANY($1::text[]) OR options->>'targetFieldId'=ANY($1::text[]))
	)`, ids).Scan(&used)
	if err != nil {
		return false, fmt.Errorf("check computed field dependencies: %w", err)
	}
	return used, nil
}

func isComputedKind(kind string) bool { return kind == "lookup" || kind == "rollup" }
