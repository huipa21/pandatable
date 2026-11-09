package structure

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
	"unicode/utf16"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/xuri/excelize/v2"

	"pandatable/internal/acl"
	"pandatable/internal/codec"
	"pandatable/internal/structure/structuresql"
)

const (
	// XLSXMetaSheet and XLSXFormatVersion identify the v2 metadata contract.
	XLSXMetaSheet     = "_meta"
	XLSXFormatVersion = "pandatable.xlsx/v2"
	// XLSXMetaHeaderRow and XLSXMetaFirstFieldRow locate the field table.
	XLSXMetaHeaderRow     = 5
	XLSXMetaFirstFieldRow = XLSXMetaHeaderRow + 1
	// XLSXAttachmentMode identifies attachment references scoped to the source instance.
	XLSXAttachmentMode = "same-instance-id"
)

// XLSXMetaField represents one field row in the v2 metadata sheet.
type XLSXMetaField struct {
	Column                  string
	FieldID                 string
	Name                    string
	UIType                  string
	PGType                  string
	Required                bool
	Unique                  bool
	Default                 string
	OptionsJSON             string
	RelationTargetTableID   string
	RelationTargetTableName string
	RelationDisplayField    string
	RelationCardinality     string
	AttachmentMode          string
	FormulaOrExpression     string
	Description             string
	System                  bool
}

// XLSXMetadata describes the workbook source and ordered field rows.
type XLSXMetadata struct {
	FormatVersion    string
	SourceInstanceID string
	TableID          string
	TableName        string
	SchemaVersion    int32
	ExportedAt       time.Time
	DatetimeTimezone string
	Fields           []XLSXMetaField
}

// XLSXAttachmentReference is an attachment identity and name, not file data.
type XLSXAttachmentReference struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// XLSXMetaColumns returns the field-table headers in their required order.
func XLSXMetaColumns() []string {
	return []string{
		"column", "field_id", "name", "ui_type", "pg_type", "required", "unique",
		"default", "options_json", "relation_target_table_id", "relation_target_table_name",
		"relation_display_field", "relation_cardinality", "attachment_mode",
		"formula_or_expression", "description", "system",
	}
}

// Cells returns the exact ordered values used for one metadata field row.
func (field XLSXMetaField) Cells() []any {
	return []any{
		field.Column, field.FieldID, field.Name, field.UIType, field.PGType,
		field.Required, field.Unique, field.Default, field.OptionsJSON,
		field.RelationTargetTableID, field.RelationTargetTableName, field.RelationDisplayField,
		field.RelationCardinality, field.AttachmentMode, field.FormulaOrExpression,
		field.Description, field.System,
	}
}

// XLSXMetaSHA256 hashes the JSON array of field rows, with native booleans and
// all other cells represented as strings. Header and source-info rows are excluded.
func XLSXMetaSHA256(fields []XLSXMetaField) (string, error) {
	hasher := sha256.New()
	write := func(data []byte) error {
		if _, err := hasher.Write(data); err != nil {
			return fmt.Errorf("hash spreadsheet metadata: %w", err)
		}
		return nil
	}
	if err := write([]byte("[")); err != nil {
		return "", err
	}
	for index, field := range fields {
		if index > 0 {
			if err := write([]byte(",")); err != nil {
				return "", err
			}
		}
		encoded, err := json.Marshal(field.Cells())
		if err != nil {
			return "", fmt.Errorf("encode spreadsheet metadata for hashing: %w", err)
		}
		if err := write(encoded); err != nil {
			return "", err
		}
	}
	if err := write([]byte("]")); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func (s *Service) xlsxSourceInstanceID(ctx context.Context) (string, error) {
	read := func() ([]byte, error) {
		var raw []byte
		err := s.pool.QueryRow(ctx, `SELECT value FROM meta.settings WHERE key = 'instance_id'`).Scan(&raw)
		return raw, err
	}
	raw, err := read()
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err := s.pool.Exec(ctx, `INSERT INTO meta.settings (key, value)
			VALUES ('instance_id', to_jsonb(gen_random_uuid()::text)) ON CONFLICT (key) DO NOTHING`); err != nil {
			return "", fmt.Errorf("create spreadsheet source instance id: %w", err)
		}
		raw, err = read()
	}
	if err != nil {
		return "", fmt.Errorf("read spreadsheet source instance id: %w", err)
	}
	var id string
	if err := json.Unmarshal(raw, &id); err != nil {
		return "", errors.New("stored spreadsheet source instance id is invalid")
	}
	parsed, ok := acl.ParseUUID(id)
	if !ok || parsed.String() != id {
		return "", errors.New("stored spreadsheet source instance id is invalid")
	}
	return id, nil
}

func buildXLSXMetadata(ctx context.Context, tx pgx.Tx, user pgtype.UUID, schema TableSchema,
	table rowTable, instanceID string, exportedAt time.Time,
) (XLSXMetadata, error) {
	metadata := XLSXMetadata{
		FormatVersion: XLSXFormatVersion, SourceInstanceID: instanceID,
		TableID: schema.Table.ID, TableName: schema.Table.Name,
		SchemaVersion: schema.SchemaVersion, ExportedAt: exportedAt.UTC(),
		DatetimeTimezone: "UTC", Fields: make([]XLSXMetaField, 0, len(schema.Fields)),
	}
	keys := make(map[string]string, len(schema.Fields))
	for index, field := range schema.Fields {
		keys[field.ID] = fmt.Sprintf("f%d", index+1)
	}
	for index, field := range schema.Fields {
		row, exists := table.byID[field.ID]
		if !exists {
			return XLSXMetadata{}, errors.New("spreadsheet field metadata is unavailable")
		}
		column, err := excelize.ColumnNumberToName(index + 2)
		if err != nil {
			return XLSXMetadata{}, fmt.Errorf("address spreadsheet metadata column: %w", err)
		}
		entry := XLSXMetaField{
			Column: column, FieldID: field.ID, Name: field.Name, UIType: field.UIType,
			Required: field.IsRequired, Unique: field.IsUnique,
			Description: field.Description, System: field.IsSystem,
		}
		if row.codec != nil {
			entry.PGType = pgTypeName(row.codec.Column().Type)
		}
		defaultValue := field.Default
		if len(defaultValue) == 0 {
			defaultValue = json.RawMessage("null")
		}
		entry.Default, err = canonicalXLSXJSON(defaultValue)
		if err != nil {
			return XLSXMetadata{}, fmt.Errorf("encode spreadsheet field default: %w", err)
		}
		options := field.Options
		if row.relation != nil {
			options, err = packageFieldOptions(ctx, tx, user, field, row, keys)
			if err != nil {
				return XLSXMetadata{}, err
			}
			target := row.relation.parentTable
			if row.relationSide == "parent" {
				target = row.relation.childTable
			}
			targetTable, readable, err := packageTargetTable(ctx, tx, user, target)
			if err != nil {
				return XLSXMetadata{}, err
			}
			entry.RelationCardinality = row.relation.kind
			if readable {
				entry.RelationTargetTableID = target.String()
				entry.RelationTargetTableName = targetTable.Name
				if targetTable.DisplayFieldID.Valid {
					fields, err := structuresql.New(tx).ListTableFields(ctx, target)
					if err != nil {
						return XLSXMetadata{}, fmt.Errorf("read spreadsheet relation display field: %w", err)
					}
					for _, targetField := range fields {
						if targetField.ID == targetTable.DisplayFieldID {
							entry.RelationDisplayField = targetField.Name
							break
						}
					}
				}
			}
		} else if row.computed != nil {
			_, readable, err := packageTargetTable(ctx, tx, user, row.computed.targetTable)
			if err != nil {
				return XLSXMetadata{}, err
			}
			if !readable {
				options, err = packageFieldOptions(ctx, tx, user, field, row, keys)
				if err != nil {
					return XLSXMetadata{}, err
				}
			}
		}
		entry.OptionsJSON, err = canonicalXLSXJSON(options)
		if err != nil {
			return XLSXMetadata{}, fmt.Errorf("encode spreadsheet field options: %w", err)
		}
		if field.UIType == string(codec.Attachment) {
			entry.AttachmentMode = XLSXAttachmentMode
		}
		if row.formula != nil {
			entry.FormulaOrExpression = row.formula.options.Expression
		}
		for _, cell := range entry.Cells() {
			if text, ok := cell.(string); ok && len(utf16.Encode([]rune(text))) > maxSpreadsheetCellChars {
				return XLSXMetadata{}, validation("A field definition is too long for an XLSX metadata cell")
			}
		}
		metadata.Fields = append(metadata.Fields, entry)
	}
	return metadata, nil
}

func canonicalXLSXJSON(raw json.RawMessage) (string, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return "", errors.New("multiple JSON values")
		}
		return "", err
	}
	encoded, err := json.Marshal(value)
	return string(encoded), err
}
