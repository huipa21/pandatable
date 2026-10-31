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
	"pandatable/internal/acl/aclsql"
	"pandatable/internal/apperr"
	"pandatable/internal/codec"
	"pandatable/internal/ddl"
	"pandatable/internal/structure/structuresql"
)

const (
	maxQueryOffset = 1_000_000
	maxQueryLimit  = 100
	maxBulkRows    = 1_000
	maxBulkCells   = 5_000
)

type RowSort struct {
	FieldID   string `json:"fieldId"`
	Direction string `json:"direction"`
}

type RowQueryRequest struct {
	Offset       *int              `json:"offset"`
	Limit        *int              `json:"limit"`
	IDs          *[]string         `json:"ids"`
	Sort         *RowSort          `json:"sort"`
	Sorts        *[]RowSort        `json:"sorts"`
	IncludeCount *bool             `json:"includeCount"`
	Search       *string           `json:"search"`
	FieldIDs     *[]string         `json:"fieldIds"`
	Filter       *FilterNode       `json:"filter"`
	GroupPath    []json.RawMessage `json:"groupPath,omitempty"`
}

type RowRecord struct {
	ID             string                     `json:"id"`
	Fields         map[string]json.RawMessage `json:"fields"`
	CommentCount   int64                      `json:"commentCount,omitempty"`
	HighlightRow   *string                    `json:"_hl_row,omitempty"`
	HighlightCells map[string]string          `json:"_hl_cells,omitempty"`
}

type RowQueryResult struct {
	Rows          []RowRecord `json:"rows"`
	FieldIDs      []string    `json:"fieldIds"`
	Total         *int64      `json:"total"`
	SchemaVersion int32       `json:"schemaVersion"`
}

type RowInsert struct {
	Fields map[string]json.RawMessage `json:"fields"`
}

type RowPatch struct {
	ID     string                     `json:"id"`
	Fields map[string]json.RawMessage `json:"fields"`
}

type rowField struct {
	id           string
	name         string
	kind         codec.Kind
	column       string
	system       bool
	codec        codec.Field
	relation     *relationRecord
	relationSide string
	computed     *computedField
	formula      *formulaField
	error        string
}

type rowTable struct {
	id            pgtype.UUID
	schema        string
	name          string
	schemaVersion int32
	fields        []rowField
	byID          map[string]rowField
}

type rowViewConfig struct {
	Fields []struct {
		ID      string `json:"id"`
		Visible bool   `json:"visible"`
	} `json:"fields"`
	Sort       []ViewSort      `json:"sort"`
	GroupBy    []string        `json:"groupBy"`
	Aggregates []ViewAggregate `json:"aggregates"`
	Highlight  []HighlightRule `json:"highlight"`
}

// Physical row projections change result OIDs after ALTER TYPE, so keep them out of pgx's statement cache.
func queryUncachedRows(ctx context.Context, tx pgx.Tx, query string, args ...any) (pgx.Rows, error) {
	return tx.Query(ctx, query, append([]any{pgx.QueryExecModeDescribeExec}, args...)...)
}

func queryUncachedRow(ctx context.Context, tx pgx.Tx, query string, args ...any) pgx.Row {
	return tx.QueryRow(ctx, query, append([]any{pgx.QueryExecModeDescribeExec}, args...)...)
}

func physicalTable(schema, name string) (string, error) {
	if err := ddl.ValidateIdentifier(ddl.Book, schema); err != nil {
		return "", fmt.Errorf("validate physical schema: %w", err)
	}
	if err := ddl.ValidateIdentifier(ddl.Table, name); err != nil {
		return "", fmt.Errorf("validate physical table: %w", err)
	}
	return pgx.Identifier{schema, name}.Sanitize(), nil
}

func systemColumn(kind codec.Kind) (string, bool) {
	switch kind {
	case codec.RowID:
		return "id", true
	case codec.CreatedAt:
		return "created_at", true
	case codec.CreatedBy:
		return "created_by", true
	case codec.UpdatedAt:
		return "updated_at", true
	case codec.UpdatedBy:
		return "updated_by", true
	default:
		return "", false
	}
}

func loadRowTable(ctx context.Context, q *structuresql.Queries, tx pgx.Tx, id pgtype.UUID, schema, name string, version int32) (rowTable, error) {
	if _, err := physicalTable(schema, name); err != nil {
		return rowTable{}, err
	}
	rows, err := q.ListRowFields(ctx, id)
	if err != nil {
		return rowTable{}, fmt.Errorf("list live row fields: %w", err)
	}
	names, err := q.ListTableFields(ctx, id)
	if err != nil {
		return rowTable{}, fmt.Errorf("list live row field names: %w", err)
	}
	fieldNames := make(map[string]string, len(names))
	for _, field := range names {
		fieldNames[field.ID.String()] = field.Name
	}
	table := rowTable{
		id: id, schema: schema, name: name, schemaVersion: version,
		fields: make([]rowField, 0, len(rows)), byID: make(map[string]rowField, len(rows)),
	}
	relations, err := relationRows(ctx, tx, id)
	if err != nil {
		return rowTable{}, err
	}
	relationFields := make(map[string]struct {
		relation relationRecord
		side     string
	}, len(relations)*2)
	for _, relation := range relations {
		relationFields[relation.parentField.String()] = struct {
			relation relationRecord
			side     string
		}{relation, "parent"}
		relationFields[relation.childField.String()] = struct {
			relation relationRecord
			side     string
		}{relation, "child"}
	}
	for _, row := range rows {
		fieldName, ok := fieldNames[row.ID.String()]
		if !ok {
			return rowTable{}, errors.New("row field is missing its metadata name")
		}
		kind := codec.Kind(row.UiType)
		if row.UiType == "link" {
			info, ok := relationFields[row.ID.String()]
			if !ok || row.PgColumn.Valid {
				return rowTable{}, errors.New("invalid relation field metadata")
			}
			relation := info.relation
			field := rowField{id: row.ID.String(), name: fieldName, relation: &relation, relationSide: info.side}
			table.fields = append(table.fields, field)
			table.byID[field.id] = field
			continue
		}
		if row.UiType == "formula" {
			var options formulaOptions
			if err := json.Unmarshal(row.Options, &options); err != nil || options.AST == nil {
				return rowTable{}, errors.New("invalid formula field metadata")
			}
			field := rowField{id: row.ID.String(), name: fieldName, kind: kind, formula: &formulaField{options: options}, error: options.Error}
			table.fields = append(table.fields, field)
			table.byID[field.id] = field
			continue
		}
		if isComputedKind(row.UiType) {
			options, err := decodeComputedOptions(row.UiType, row.Options)
			if err != nil {
				return rowTable{}, fmt.Errorf("read computed field options: %w", err)
			}
			if options.Error != "" {
				field := rowField{id: row.ID.String(), name: fieldName, kind: kind, error: options.Error}
				table.fields = append(table.fields, field)
				table.byID[field.id] = field
				continue
			}
			info, ok := relationFields[options.RelationFieldID]
			if !ok || row.PgColumn.Valid || row.IsSystem {
				return rowTable{}, errors.New("invalid computed field metadata")
			}
			relation := info.relation
			relationField := rowField{id: options.RelationFieldID, relation: &relation, relationSide: info.side}
			computed, err := computedTarget(ctx, tx, table, row.UiType, row.Options, relationField)
			if err != nil {
				return rowTable{}, fmt.Errorf("bind computed field: %w", err)
			}
			field := rowField{id: row.ID.String(), name: fieldName, kind: kind, computed: computed}
			table.fields = append(table.fields, field)
			table.byID[field.id] = field
			continue
		}
		options, err := fieldOptionsForCodec(row.Options)
		if err != nil {
			return rowTable{}, err
		}
		bound, err := codec.Bind(kind, options)
		if err != nil {
			return rowTable{}, fmt.Errorf("bind stored row field: %w", err)
		}
		column := ""
		if row.IsSystem {
			var ok bool
			column, ok = systemColumn(kind)
			if !ok || !bound.ReadOnly() {
				return rowTable{}, errors.New("invalid system field metadata")
			}
		} else {
			if !row.PgColumn.Valid {
				return rowTable{}, errors.New("user field is missing its physical column")
			}
			column = row.PgColumn.String
			if err := ddl.ValidateIdentifier(ddl.Column, column); err != nil {
				return rowTable{}, fmt.Errorf("validate physical field column: %w", err)
			}
			if bound.ReadOnly() && kind != codec.AutoNumber {
				return rowTable{}, errors.New("user field has a read-only codec")
			}
		}
		field := rowField{id: row.ID.String(), name: fieldName, kind: kind, column: column, system: row.IsSystem, codec: bound}
		table.fields = append(table.fields, field)
		table.byID[field.id] = field
	}
	return table, nil
}

func (table rowTable) qualifiedName() string {
	return pgx.Identifier{table.schema, table.name}.Sanitize()
}

func (field rowField) sqlColumn() string {
	return pgx.Identifier{field.column}.Sanitize()
}

func (table rowTable) selectColumns(fields []rowField) string {
	return strings.Join(table.selectColumnList(fields), ", ")
}

func (table rowTable) selectColumnList(fields []rowField) []string {
	columns := make([]string, 0, len(fields)+1)
	columns = append(columns, pgx.Identifier{"id"}.Sanitize())
	for _, field := range fields {
		if field.relation != nil || field.computed != nil || field.formula != nil {
			continue
		}
		column := field.sqlColumn()
		if field.kind == codec.JSON {
			column += "::text"
		}
		columns = append(columns, column)
	}
	return columns
}

func (table rowTable) encode(values []any, fields []rowField) (RowRecord, error) {
	physicalFields := physicalRowFields(fields)
	if len(values) != len(physicalFields)+1 {
		return RowRecord{}, errors.New("row scan returned an unexpected column count")
	}
	rowID, err := codec.Bind(codec.RowID, nil)
	if err != nil {
		return RowRecord{}, err
	}
	encodedID, err := rowID.Encode(values[0])
	if err != nil {
		return RowRecord{}, fmt.Errorf("encode row id: %w", err)
	}
	var id string
	if err := json.Unmarshal(encodedID, &id); err != nil {
		return RowRecord{}, fmt.Errorf("decode canonical row id: %w", err)
	}
	record := RowRecord{ID: id, Fields: make(map[string]json.RawMessage, len(fields))}
	for index, field := range physicalFields {
		wire, err := field.codec.Encode(values[index+1])
		if err != nil {
			return RowRecord{}, fmt.Errorf("encode row field: %w", err)
		}
		if !json.Valid(wire) {
			return RowRecord{}, errors.New("codec emitted invalid row JSON")
		}
		record.Fields[field.id] = json.RawMessage(wire)
	}
	return record, nil
}

func scanRow(row pgx.Row, table rowTable, fields []rowField) (RowRecord, error) {
	fields = physicalRowFields(fields)
	values := make([]any, len(fields)+1)
	destinations := make([]any, len(values))
	for index := range values {
		destinations[index] = &values[index]
	}
	if err := row.Scan(destinations...); err != nil {
		return RowRecord{}, err
	}
	return table.encode(values, fields)
}

func physicalRowFields(fields []rowField) []rowField {
	result := make([]rowField, 0, len(fields))
	for _, field := range fields {
		if field.relation == nil && field.computed == nil && field.formula == nil {
			result = append(result, field)
		}
	}
	return result
}

func scanRows(rows pgx.Rows, table rowTable, fields []rowField) ([]RowRecord, error) {
	defer rows.Close()
	result := make([]RowRecord, 0)
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, fmt.Errorf("read row values: %w", err)
		}
		record, err := table.encode(values, fields)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}
	return result, nil
}

func (s *Service) authorizeRows(ctx context.Context, conn aclsql.DBTX, user pgtype.UUID, target acl.Resource, level acl.Level) error {
	access, err := acl.AccessUncached(ctx, conn, user, target)
	if err != nil {
		return err
	}
	if !access.Visible {
		return acl.ErrNotFound()
	}
	return access.Check(level)
}

func (s *Service) withReadableTable(ctx context.Context, actor Actor, tableID string, run func(pgx.Tx, rowTable) error) error {
	target, err := resourceOf(acl.Table, tableID)
	if err != nil {
		return err
	}
	access, err := s.resolver.Access(ctx, actor.ID, target)
	if err != nil {
		return err
	}
	if err := access.Check(acl.Read); err != nil {
		return err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return fmt.Errorf("begin row read: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err := tx.Exec(ctx, lockTimeout); err != nil {
		return rowDatabaseError(fmt.Errorf("set row-read lock timeout: %w", err))
	}
	tableRow, err := structuresql.New(tx).LockRowTable(ctx, target.ID)
	if err != nil {
		return rowDatabaseError(notFoundIfNoRows(err, "lock row table"))
	}
	table, err := loadRowTable(ctx, structuresql.New(tx), tx, target.ID, tableRow.PgSchema, tableRow.PgName, tableRow.SchemaVersion)
	if err != nil {
		return err
	}
	if err := run(tx, table); err != nil {
		return mapRowDatabaseError(sanitizeLock(err))
	}
	if err := tx.Commit(ctx); err != nil {
		return rowDatabaseError(fmt.Errorf("commit row read: %w", err))
	}
	return nil
}

func (s *Service) withWritableTable(ctx context.Context, actor Actor, tableID string, run func(pgx.Tx, rowTable) error) error {
	return s.withTableMutation(ctx, actor, tableID, acl.Write, run)
}

func (s *Service) withTableMutation(ctx context.Context, actor Actor, tableID string, level acl.Level, run func(pgx.Tx, rowTable) error) error {
	target, err := resourceOf(acl.Table, tableID)
	if err != nil {
		return err
	}
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return acl.ErrValidation()
	}
	if err := s.authorizeRows(ctx, s.pool, user, target, level); err != nil {
		return err
	}
	err = s.pool.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, lockTimeout); err != nil {
			return fmt.Errorf("set row-write lock timeout: %w", err)
		}
		_, err := acl.LockPath(ctx, tx, target)
		if err != nil {
			return err
		}
		if err := acl.LockActiveUser(ctx, tx, user); err != nil {
			return err
		}
		if err := s.authorizeRows(ctx, tx, user, target, level); err != nil {
			return err
		}
		tableRow, err := structuresql.New(tx).LockRowTable(ctx, target.ID)
		if err != nil {
			return notFoundIfNoRows(err, "lock row table")
		}
		table, err := loadRowTable(ctx, structuresql.New(tx), tx, target.ID, tableRow.PgSchema, tableRow.PgName, tableRow.SchemaVersion)
		if err != nil {
			return err
		}
		return run(tx, table)
	})
	return rowDatabaseError(err)
}

func rowDatabaseError(err error) error { return mapRowDatabaseError(sanitizeLock(err)) }

func mapRowDatabaseError(err error) error {
	if err == nil {
		return nil
	}
	var postgres *pgconn.PgError
	if !errors.As(err, &postgres) {
		return err
	}
	switch postgres.Code {
	case "23502":
		return apperr.New(400, "ERR_REQUIRED_FIELD", "A required field is missing")
	case "23505":
		return apperr.New(409, "ERR_UNIQUE_CONFLICT", "A unique field already has this value")
	case "23503":
		return relationRestrictError()
	case "40001":
		return apperr.New(409, "ERR_SCHEMA_CHANGED", "The table schema changed; reload and retry")
	default:
		return err
	}
}

func canonicalRowID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != value {
		return 0, validation("Row id must be a positive canonical decimal string")
	}
	return id, nil
}

func parsePathRowID(value string) (int64, error) {
	id, err := canonicalRowID(value)
	if err != nil {
		return 0, acl.ErrNotFound()
	}
	return id, nil
}

func sortedFieldValues(ctx context.Context, queries *structuresql.Queries, table rowTable,
	input map[string]json.RawMessage) ([]rowField, []any, error) {
	keys := make([]string, 0, len(input))
	for id := range input {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	fields := make([]rowField, 0, len(keys))
	values := make([]any, 0, len(keys))
	for _, id := range keys {
		field, ok := table.byID[id]
		parsed, valid := acl.ParseUUID(id)
		if !ok || !valid || parsed.String() != id || field.system || field.relation != nil || field.computed != nil || field.formula != nil {
			return nil, nil, validation("A supplied field is not writable on this table")
		}
		value, err := field.codec.Decode(input[id])
		if err != nil {
			return nil, nil, codec.APIError(err)
		}
		if field.kind == codec.Attachment {
			value, err = canonicalAttachmentValue(ctx, queries, table.id, field, value)
			if err != nil {
				return nil, nil, err
			}
		}
		fields = append(fields, field)
		values = append(values, value)
	}
	return fields, values, nil
}

func canonicalAttachmentValue(ctx context.Context, queries *structuresql.Queries, tableID pgtype.UUID,
	field rowField, rawValue any) (any, error) {
	raw, ok := rawValue.(json.RawMessage)
	if !ok {
		return nil, errors.New("attachment codec returned an unexpected value")
	}
	var submitted []codec.AttachmentValue
	if err := json.Unmarshal(raw, &submitted); err != nil {
		return nil, codec.APIError(err)
	}
	if len(submitted) == 0 {
		return raw, nil
	}
	fieldID, ok := acl.ParseUUID(field.id)
	if !ok {
		return nil, errors.New("attachment field has an invalid id")
	}
	ids := make([]pgtype.UUID, len(submitted))
	for index, value := range submitted {
		id, valid := acl.ParseUUID(value.ID)
		if !valid || id.String() != value.ID {
			return nil, validation("Attachment reference is invalid")
		}
		ids[index] = id
	}
	metadata, err := queries.ListAttachmentMetadata(ctx, structuresql.ListAttachmentMetadataParams{
		TableID: tableID, FieldID: fieldID, Ids: ids,
	})
	if err != nil {
		return nil, fmt.Errorf("load attachment cell metadata: %w", err)
	}
	byID := make(map[string]structuresql.ListAttachmentMetadataRow, len(metadata))
	for _, item := range metadata {
		byID[item.ID.String()] = item
	}
	canonical := make([]codec.AttachmentValue, len(submitted))
	for index, value := range submitted {
		item, found := byID[value.ID]
		if !found {
			return nil, validation("Attachment does not belong to this field")
		}
		var thumb *string
		if item.HasThumb {
			url := codec.ThumbnailURL(item.ID.String())
			thumb = &url
		}
		canonical[index] = codec.AttachmentValue{
			ID: item.ID.String(), Name: item.FileName, MIME: item.MimeType,
			Size: item.SizeBytes, Thumb: thumb,
		}
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return nil, fmt.Errorf("encode attachment cell metadata: %w", err)
	}
	value, err := field.codec.Decode(encoded)
	if err != nil {
		return nil, codec.APIError(err)
	}
	return value, nil
}

func (s *Service) GetRow(ctx context.Context, actor Actor, tableID, rowID string) (RowRecord, error) {
	id, err := parsePathRowID(rowID)
	if err != nil {
		return RowRecord{}, err
	}
	var result RowRecord
	err = s.withReadableTable(ctx, actor, tableID, func(tx pgx.Tx, table rowTable) error {
		query, args, err := sq.Select(table.selectColumnList(table.fields)...).
			From(table.qualifiedName()).
			Where(sq.Eq{pgx.Identifier{"id"}.Sanitize(): id}).
			PlaceholderFormat(sq.Dollar).ToSql()
		if err != nil {
			return fmt.Errorf("build row read query: %w", err)
		}
		record, err := scanRow(queryUncachedRow(ctx, tx, query, args...), table, table.fields)
		if errors.Is(err, pgx.ErrNoRows) {
			return acl.ErrNotFound()
		}
		if err != nil {
			return fmt.Errorf("read row: %w", err)
		}
		if err := enrichRowLinks(ctx, tx, table, table.fields, []RowRecord{record}); err != nil {
			return err
		}
		if err := enrichRowComputed(ctx, tx, actor, table, table.fields, []RowRecord{record}); err != nil {
			return err
		}
		if err := enrichRowFormulas(ctx, tx, actor, table, table.fields, []RowRecord{record}); err != nil {
			return err
		}
		if err := enrichSingleCommentCount(ctx, tx, table.id, &record); err != nil {
			return err
		}
		result = record
		return nil
	})
	return result, err
}

func (s *Service) QueryRows(ctx context.Context, actor Actor, viewID string, request RowQueryRequest) (RowQueryResult, error) {
	viewUUID, ok := acl.ParseUUID(viewID)
	if !ok {
		return RowQueryResult{}, acl.ErrNotFound()
	}
	offset := 0
	if request.Offset != nil {
		offset = *request.Offset
	}
	limit := maxQueryLimit
	if request.Limit != nil {
		limit = *request.Limit
	}
	if offset < 0 || offset > maxQueryOffset || limit < 1 || limit > maxQueryLimit {
		return RowQueryResult{}, validation("Query paging is outside the supported bounds")
	}
	includeCount := request.IncludeCount == nil || *request.IncludeCount
	view, err := s.queries.GetViewForQuery(ctx, viewUUID)
	if err != nil {
		return RowQueryResult{}, notFoundIfNoRows(err, "read view")
	}
	if view.Scope == "personal" && (!view.OwnerID.Valid || view.OwnerID.String() != actor.ID) {
		return RowQueryResult{}, acl.ErrNotFound()
	}
	target := acl.Resource{Type: acl.Table, ID: view.TableID}
	access, err := s.resolver.Access(ctx, actor.ID, target)
	if err != nil {
		return RowQueryResult{}, err
	}
	if err := access.Check(acl.Read); err != nil {
		return RowQueryResult{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return RowQueryResult{}, fmt.Errorf("begin view query: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err := tx.Exec(ctx, lockTimeout); err != nil {
		return RowQueryResult{}, rowDatabaseError(fmt.Errorf("set query lock timeout: %w", err))
	}
	q := structuresql.New(tx)
	lockedView, err := q.LockViewForQuery(ctx, viewUUID)
	if err != nil {
		return RowQueryResult{}, rowDatabaseError(notFoundIfNoRows(err, "lock view"))
	}
	if lockedView.Scope == "personal" && (!lockedView.OwnerID.Valid || lockedView.OwnerID.String() != actor.ID) {
		return RowQueryResult{}, acl.ErrNotFound()
	}
	table, err := loadRowTable(ctx, q, tx, lockedView.TableID, lockedView.PgSchema, lockedView.PgName, lockedView.SchemaVersion)
	if err != nil {
		return RowQueryResult{}, err
	}
	config := rowViewConfig{}
	if err := json.Unmarshal(lockedView.Config, &config); err != nil {
		return RowQueryResult{}, fmt.Errorf("decode stored view config: %w", err)
	}
	if err := validateGroupConfig(ViewColumns{
		GroupBy: config.GroupBy, Aggregates: config.Aggregates,
	}, table); err != nil {
		return RowQueryResult{}, err
	}
	config.Highlight = normalizeHighlightRules(config.Highlight, rowFieldIDs(table))
	highlightRowSQL, highlightCellsSQL, highlightArgs, err := compileHighlightRules(
		ctx, tx, actor, table, config.Highlight,
	)
	if err != nil {
		return RowQueryResult{}, err
	}
	fields := visibleRowFields(table, config.Fields, request.FieldIDs)
	if err := validateQuickSearch(request.Search); err != nil {
		return RowQueryResult{}, err
	}
	sortKeys, explicitSort, err := configuredRowSorts(table, config.Sort, request.Sort, request.Sorts)
	if err != nil {
		return RowQueryResult{}, err
	}
	var sortArgs []any
	sortExpressions := make([]string, 0, len(sortKeys))
	for _, key := range sortKeys {
		compiler := filterCompiler{ctx: ctx, tx: tx, actor: actor, table: table}
		expression, _, err := compiler.fieldExpression(key.field)
		if err != nil {
			var apiError *apperr.Error
			if !explicitSort && errors.As(err, &apiError) && apiError.Code == "ERR_VALIDATION" {
				continue
			}
			return RowQueryResult{}, err
		}
		sortExpressions = append(sortExpressions,
			shiftSQLArguments(expression, len(sortArgs))+" "+key.direction+" NULLS LAST")
		sortArgs = append(sortArgs, compiler.args...)
	}
	filterSQL := ""
	var filterArgs []any
	if request.Filter != nil {
		filterSQL, filterArgs, err = compileRowFilter(ctx, tx, actor, table, request.Filter)
		if err != nil {
			return RowQueryResult{}, err
		}
	}
	searchSQL, searchArgs, err := compileQuickSearch(ctx, tx, actor, table, fields, request.Search)
	if err != nil {
		return RowQueryResult{}, err
	}
	conditionSQL, conditionArgs := combineQueryConditions(filterSQL, filterArgs, searchSQL, searchArgs)
	if request.IDs != nil {
		if len(*request.IDs) == 0 || len(*request.IDs) > maxQueryLimit {
			return RowQueryResult{}, validation("Query row IDs are outside the supported bounds")
		}
		ids := make([]int64, 0, len(*request.IDs))
		seen := make(map[int64]bool, len(*request.IDs))
		for _, raw := range *request.IDs {
			id, err := canonicalRowID(raw)
			if err != nil || seen[id] {
				return RowQueryResult{}, validation("Query row IDs must be unique and valid")
			}
			seen[id] = true
			ids = append(ids, id)
		}
		if conditionSQL != "" {
			conditionSQL += " AND "
		}
		conditionSQL += fmt.Sprintf(`"source"."id" = ANY($%d::bigint[])`, len(conditionArgs)+1)
		conditionArgs = append(conditionArgs, ids)
	}
	groupSQL, groupArgs, err := compileGroupPath(ctx, tx, actor, table, config.GroupBy, request.GroupPath)
	if err != nil {
		return RowQueryResult{}, err
	}
	whereSQL := conditionSQL
	if groupSQL != "" {
		if whereSQL != "" {
			whereSQL += " AND "
		}
		whereSQL += "(" + shiftSQLArguments(groupSQL, len(conditionArgs)) + ")"
	}
	from := table.qualifiedName() + ` AS "source"`
	builder := sq.Select(table.selectColumnList(fields)...).From(from)
	sql, args, err := builder.OrderBy(`"source"."id" ASC`).Limit(uint64(limit)).Offset(uint64(offset)).
		PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return RowQueryResult{}, fmt.Errorf("build view query: %w", err)
	}
	argumentPrefix := len(conditionArgs) + len(groupArgs) + len(sortArgs)
	sql = shiftSQLArguments(sql, argumentPrefix+len(highlightArgs))
	fromAt := strings.Index(sql, " FROM ")
	if fromAt < 0 {
		return RowQueryResult{}, errors.New("view query is missing its FROM clause")
	}
	highlightSelect := shiftSQLArguments(highlightRowSQL, argumentPrefix) + ", " +
		shiftSQLArguments(highlightCellsSQL, argumentPrefix)
	sql = sql[:fromAt] + ", " + highlightSelect + sql[fromAt:]
	orderKeys := make([]string, 0, len(sortExpressions)+1)
	for _, expression := range sortExpressions {
		orderKeys = append(orderKeys, shiftSQLArguments(expression, len(conditionArgs)+len(groupArgs)))
	}
	orderKeys = append(orderKeys, `"source"."id" ASC`)
	sql = strings.Replace(sql, `ORDER BY "source"."id" ASC`,
		"ORDER BY "+strings.Join(orderKeys, ", "), 1)
	if whereSQL != "" {
		orderAt := strings.Index(sql, " ORDER BY ")
		if orderAt < 0 {
			return RowQueryResult{}, errors.New("view query is missing its order clause")
		}
		sql = sql[:orderAt] + " WHERE " + whereSQL + sql[orderAt:]
	}
	args = append(append(append(append(append([]any(nil), conditionArgs...), groupArgs...), sortArgs...),
		highlightArgs...), args...)
	result := RowQueryResult{Rows: make([]RowRecord, 0, limit), SchemaVersion: table.schemaVersion}
	result.FieldIDs = make([]string, len(fields))
	for index, field := range fields {
		result.FieldIDs[index] = field.id
	}
	if includeCount {
		var total int64
		countBuilder := sq.Select("count(*)").From(from)
		if whereSQL != "" {
			countBuilder = countBuilder.Where(sq.Expr(whereSQL))
		}
		countQuery, countArgs, err := countBuilder.
			PlaceholderFormat(sq.Dollar).ToSql()
		if err != nil {
			return RowQueryResult{}, fmt.Errorf("build row count query: %w", err)
		}
		countArgs = append(append(append([]any(nil), conditionArgs...), groupArgs...), countArgs...)
		if err := tx.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
			return RowQueryResult{}, rowDatabaseError(fmt.Errorf("count view rows: %w", err))
		}
		result.Total = &total
	}
	rows, err := queryUncachedRows(ctx, tx, sql, args...)
	if err != nil {
		return RowQueryResult{}, rowDatabaseError(fmt.Errorf("query view rows: %w", err))
	}
	result.Rows, err = scanHighlightedRows(rows, table, fields)
	if err != nil {
		return RowQueryResult{}, rowDatabaseError(err)
	}
	if err := enrichRowLinks(ctx, tx, table, fields, result.Rows); err != nil {
		return RowQueryResult{}, rowDatabaseError(err)
	}
	if err := enrichRowComputed(ctx, tx, actor, table, fields, result.Rows); err != nil {
		return RowQueryResult{}, rowDatabaseError(err)
	}
	if err := enrichRowFormulas(ctx, tx, actor, table, fields, result.Rows); err != nil {
		return RowQueryResult{}, rowDatabaseError(err)
	}
	if err := enrichCommentCounts(ctx, tx, table.id, result.Rows); err != nil {
		return RowQueryResult{}, rowDatabaseError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return RowQueryResult{}, rowDatabaseError(fmt.Errorf("commit view query: %w", err))
	}
	return result, nil
}

func visibleRowFields(table rowTable, config []struct {
	ID      string `json:"id"`
	Visible bool   `json:"visible"`
}, override *[]string) []rowField {
	if override != nil {
		visible := make([]rowField, 0, len(*override))
		seen := make(map[string]bool, len(*override))
		for _, id := range *override {
			if seen[id] {
				continue
			}
			field, ok := table.byID[id]
			if !ok {
				continue
			}
			seen[id] = true
			visible = append(visible, field)
		}
		return visible
	}
	visible := make([]rowField, 0, len(table.fields))
	seen := make(map[string]bool, len(table.fields))
	for _, item := range config {
		if seen[item.ID] {
			continue
		}
		field, ok := table.byID[item.ID]
		if !ok {
			continue
		}
		seen[item.ID] = true
		if item.Visible {
			visible = append(visible, field)
		}
	}
	for _, field := range table.fields {
		if field.system || seen[field.id] {
			continue
		}
		visible = append(visible, field)
	}
	return visible
}

type rowSortKey struct {
	field     rowField
	direction string
}

func configuredRowSorts(table rowTable, configured []ViewSort, override *RowSort, overrides *[]RowSort) ([]rowSortKey, bool, error) {
	if override != nil && overrides != nil {
		return nil, true, validation("Only one sort request format may be provided")
	}
	explicit := override != nil || overrides != nil
	requested := make([]RowSort, 0, len(configured))
	if override != nil {
		requested = append(requested, *override)
	} else if overrides != nil {
		requested = append(requested, *overrides...)
	} else {
		for _, item := range configured {
			requested = append(requested, RowSort{FieldID: item.Field, Direction: item.Dir})
		}
	}
	keys := make([]rowSortKey, 0, len(requested))
	seen := make(map[string]bool, len(requested))
	for _, item := range requested {
		field, ok := table.byID[item.FieldID]
		direction := strings.ToUpper(item.Direction)
		if !ok || field.relation != nil || direction != "ASC" && direction != "DESC" {
			if explicit {
				return nil, true, validation("Sort must name live fields and use asc or desc")
			}
			continue
		}
		if seen[item.FieldID] {
			continue
		}
		seen[item.FieldID] = true
		keys = append(keys, rowSortKey{field: field, direction: direction})
	}
	return keys, explicit, nil
}

func (s *Service) CreateRows(ctx context.Context, actor Actor, tableID string, input []RowInsert) ([]RowRecord, error) {
	if len(input) == 0 || len(input) > maxBulkRows {
		return nil, validation("Insert row count is outside the supported bounds")
	}
	cells := 0
	for _, row := range input {
		cells += len(row.Fields)
	}
	if cells > maxBulkCells {
		return nil, validation("Insert cell count is outside the supported bounds")
	}
	result := make([]RowRecord, 0, len(input))
	err := s.withWritableTable(ctx, actor, tableID, func(tx pgx.Tx, table rowTable) error {
		queries := s.queries.WithTx(tx)
		for _, item := range input {
			record, err := insertRow(ctx, tx, queries, table, actor, item.Fields)
			if err != nil {
				return err
			}
			result = append(result, record)
		}
		return emitRows(ctx, tx, actor, table.id.String(), "row.created", result, nil)
	})
	return result, err
}

func (s *Service) SubmitForm(ctx context.Context, actor Actor, viewID string, input map[string]json.RawMessage) (RowRecord, error) {
	if input == nil {
		return RowRecord{}, validation("Form fields must be an object")
	}
	uuid, ok := acl.ParseUUID(viewID)
	if !ok {
		return RowRecord{}, acl.ErrNotFound()
	}
	view, err := s.queries.GetViewForQuery(ctx, uuid)
	if err != nil {
		return RowRecord{}, notFoundIfNoRows(err, "read form view")
	}
	if view.Scope == "personal" && (!view.OwnerID.Valid || view.OwnerID.String() != actor.ID) {
		return RowRecord{}, acl.ErrNotFound()
	}
	if len(input) > maxBulkCells {
		return RowRecord{}, validation("Form submission contains too many fields")
	}

	var result RowRecord
	err = s.withWritableTable(ctx, actor, view.TableID.String(), func(tx pgx.Tx, table rowTable) error {
		q := structuresql.New(tx)
		stored, err := q.LockViewColumns(ctx, structuresql.LockViewColumnsParams{ID: uuid, TableID: table.id})
		if err != nil {
			return notFoundIfNoRows(err, "lock form view")
		}
		if stored.Scope == "personal" && (!stored.OwnerID.Valid || stored.OwnerID.String() != actor.ID) {
			return acl.ErrNotFound()
		}
		if stored.Type != "form" {
			return validation("Rows can only be submitted to a form view")
		}
		var config struct {
			Form *FormConfig `json:"form"`
		}
		if err := json.Unmarshal(stored.Config, &config); err != nil {
			return fmt.Errorf("decode stored form config: %w", err)
		}
		if config.Form == nil {
			return validation("Form configuration is invalid")
		}
		form, err := validateFormConfig(*config.Form, table)
		if err != nil {
			return err
		}
		configured := make(map[string]FormField, len(form.Fields))
		for _, field := range form.Fields {
			configured[field.ID] = field
		}
		for id := range input {
			if _, ok := configured[id]; !ok {
				return validation("Submitted fields must belong to this form")
			}
		}
		for _, field := range form.Fields {
			if field.Required {
				value, exists := input[field.ID]
				if !exists || emptyFormValue(value) {
					return apperr.New(400, "ERR_REQUIRED_FIELD", "A required form field is missing")
				}
			}
		}
		record, err := insertRow(ctx, tx, q, table, actor, input)
		if err != nil {
			return err
		}
		result = record
		return emitRows(ctx, tx, actor, table.id.String(), "row.created", []RowRecord{record}, nil)
	})
	return result, err
}

func emptyFormValue(raw json.RawMessage) bool {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return false
	}
	switch value := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(value) == ""
	case []any:
		return len(value) == 0
	case map[string]any:
		return len(value) == 0
	default:
		return false
	}
}

func insertRow(ctx context.Context, tx pgx.Tx, queries *structuresql.Queries, table rowTable, actor Actor,
	input map[string]json.RawMessage) (RowRecord, error) {
	fields, values, err := sortedFieldValues(ctx, queries, table, input)
	if err != nil {
		return RowRecord{}, err
	}
	columns := make([]string, 0, len(fields)+4)
	builder := sq.Insert(table.qualifiedName()).PlaceholderFormat(sq.Dollar)
	valuesWithDefaults := make([]any, 0, len(fields)+4)
	for index, field := range fields {
		columns = append(columns, field.sqlColumn())
		valuesWithDefaults = append(valuesWithDefaults, values[index])
	}
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return RowRecord{}, acl.ErrValidation()
	}
	columns = append(columns, `"created_at"`, `"created_by"`, `"updated_at"`, `"updated_by"`)
	valuesWithDefaults = append(valuesWithDefaults, sq.Expr("transaction_timestamp()"), user,
		sq.Expr("transaction_timestamp()"), user)
	query, args, err := builder.Columns(columns...).
		Values(valuesWithDefaults...).
		Suffix("RETURNING " + table.selectColumns(table.fields)).
		ToSql()
	if err != nil {
		return RowRecord{}, fmt.Errorf("build row insert: %w", err)
	}
	record, err := scanRow(queryUncachedRow(ctx, tx, query, args...), table, table.fields)
	if err != nil {
		return RowRecord{}, fmt.Errorf("insert row: %w", err)
	}
	if err := enrichRowLinks(ctx, tx, table, table.fields, []RowRecord{record}); err != nil {
		return RowRecord{}, err
	}
	if err := enrichRowComputed(ctx, tx, actor, table, table.fields, []RowRecord{record}); err != nil {
		return RowRecord{}, err
	}
	if err := enrichRowFormulas(ctx, tx, actor, table, table.fields, []RowRecord{record}); err != nil {
		return RowRecord{}, err
	}
	return record, nil
}

func (s *Service) PatchRow(ctx context.Context, actor Actor, tableID, rowID string, input map[string]json.RawMessage) (RowRecord, error) {
	id, err := canonicalRowID(rowID)
	if err != nil {
		return RowRecord{}, err
	}
	if len(input) == 0 {
		return RowRecord{}, validation("At least one field value is required")
	}
	var result RowRecord
	err = s.withWritableTable(ctx, actor, tableID, func(tx pgx.Tx, table rowTable) error {
		record, err := patchRow(ctx, tx, s.queries.WithTx(tx), table, actor, id, input)
		if err != nil {
			return err
		}
		if err := enrichSingleCommentCount(ctx, tx, table.id, &record); err != nil {
			return err
		}
		result = record
		if err := emitRows(ctx, tx, actor, table.id.String(), "row.updated", []RowRecord{record},
			changedFields([]RowPatch{{Fields: input}})); err != nil {
			return err
		}
		return emitRelatedRowInvalidations(ctx, tx, table.id)
	})
	return result, err
}

func patchRow(ctx context.Context, tx pgx.Tx, queries *structuresql.Queries, table rowTable, actor Actor,
	id int64, input map[string]json.RawMessage) (RowRecord, error) {
	if len(input) == 0 {
		return RowRecord{}, validation("At least one field value is required")
	}
	lockQuery, lockArgs, err := sq.Select(pgx.Identifier{"id"}.Sanitize()).
		From(table.qualifiedName()).
		Where(sq.Eq{pgx.Identifier{"id"}.Sanitize(): id}).
		Suffix("FOR UPDATE").
		PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return RowRecord{}, fmt.Errorf("build row lock query: %w", err)
	}
	if err := tx.QueryRow(ctx, lockQuery, lockArgs...).Scan(new(int64)); errors.Is(err, pgx.ErrNoRows) {
		return RowRecord{}, acl.ErrNotFound()
	} else if err != nil {
		return RowRecord{}, fmt.Errorf("lock row for patch: %w", err)
	}
	fields, values, err := sortedFieldValues(ctx, queries, table, input)
	if err != nil {
		return RowRecord{}, err
	}
	builder := sq.Update(table.qualifiedName()).PlaceholderFormat(sq.Dollar)
	for index, field := range fields {
		builder = builder.Set(field.sqlColumn(), values[index])
	}
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return RowRecord{}, acl.ErrValidation()
	}
	query, args, err := builder.
		Set("updated_at", sq.Expr("transaction_timestamp()")).
		Set("updated_by", user).
		Where(sq.Eq{pgx.Identifier{"id"}.Sanitize(): id}).
		Suffix("RETURNING " + table.selectColumns(table.fields)).
		ToSql()
	if err != nil {
		return RowRecord{}, fmt.Errorf("build row patch: %w", err)
	}
	record, err := scanRow(queryUncachedRow(ctx, tx, query, args...), table, table.fields)
	if err != nil {
		return RowRecord{}, fmt.Errorf("patch row: %w", err)
	}
	if err := enrichRowLinks(ctx, tx, table, table.fields, []RowRecord{record}); err != nil {
		return RowRecord{}, err
	}
	if err := enrichRowComputed(ctx, tx, actor, table, table.fields, []RowRecord{record}); err != nil {
		return RowRecord{}, err
	}
	if err := enrichRowFormulas(ctx, tx, actor, table, table.fields, []RowRecord{record}); err != nil {
		return RowRecord{}, err
	}
	return record, nil
}

func (s *Service) PatchRows(ctx context.Context, actor Actor, tableID string, input []RowPatch) ([]RowRecord, error) {
	if len(input) == 0 || len(input) > maxBulkRows {
		return nil, validation("Patch row count is outside the supported bounds")
	}
	cells, seen := 0, make(map[string]bool, len(input))
	ids := make([]int64, len(input))
	for index, row := range input {
		cells += len(row.Fields)
		if len(row.Fields) == 0 || seen[row.ID] {
			return nil, validation("Bulk patch rows must be unique and contain field values")
		}
		seen[row.ID] = true
		id, err := canonicalRowID(row.ID)
		if err != nil {
			return nil, err
		}
		ids[index] = id
	}
	if cells > maxBulkCells {
		return nil, validation("Patch cell count is outside the supported bounds")
	}
	result := make([]RowRecord, len(input))
	order := make([]int, len(input))
	for index := range order {
		order[index] = index
	}
	sort.Slice(order, func(left, right int) bool {
		return ids[order[left]] < ids[order[right]]
	})
	err := s.withWritableTable(ctx, actor, tableID, func(tx pgx.Tx, table rowTable) error {
		for _, index := range order {
			item := input[index]
			record, err := patchRow(ctx, tx, s.queries.WithTx(tx), table, actor, ids[index], item.Fields)
			if err != nil {
				return err
			}
			result[index] = record
		}
		if err := enrichCommentCounts(ctx, tx, table.id, result); err != nil {
			return err
		}
		if err := emitPatches(ctx, tx, actor, table.id.String(), result, input); err != nil {
			return err
		}
		return emitRelatedRowInvalidations(ctx, tx, table.id)
	})
	return result, err
}

func (s *Service) DeleteRows(ctx context.Context, actor Actor, tableID string, rowIDs []string) ([]RowRecord, error) {
	if len(rowIDs) == 0 || len(rowIDs) > maxBulkRows {
		return nil, validation("Delete row count is outside the supported bounds")
	}
	ids := make([]int64, len(rowIDs))
	seen := make(map[string]bool, len(rowIDs))
	for index, value := range rowIDs {
		if seen[value] {
			return nil, validation("Delete row ids must be unique")
		}
		seen[value] = true
		id, err := canonicalRowID(value)
		if err != nil {
			return nil, err
		}
		ids[index] = id
	}
	sorted := append([]int64(nil), ids...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	var result []RowRecord
	err := s.withWritableTable(ctx, actor, tableID, func(tx pgx.Tx, table rowTable) error {
		query, args, err := sq.Select(table.selectColumnList(table.fields)...).
			From(table.qualifiedName()).
			Where(sq.Expr(`"id" = ANY(?::bigint[])`, sorted)).
			OrderBy(`"id" ASC`).
			Suffix("FOR UPDATE").
			PlaceholderFormat(sq.Dollar).ToSql()
		if err != nil {
			return fmt.Errorf("build rows delete snapshot query: %w", err)
		}
		rows, err := queryUncachedRows(ctx, tx, query, args...)
		if err != nil {
			return fmt.Errorf("lock rows for deletion: %w", err)
		}
		records, err := scanRows(rows, table, table.fields)
		if err != nil {
			return err
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
		if len(records) != len(ids) {
			return acl.ErrNotFound()
		}
		if err := s.deleteRelatedRows(ctx, tx, actor, table, sorted, make(map[string]map[int64]bool)); err != nil {
			return err
		}
		deleteQuery, deleteArgs, err := sq.Delete(table.qualifiedName()).
			Where(sq.Expr(`"id" = ANY(?::bigint[])`, sorted)).
			PlaceholderFormat(sq.Dollar).ToSql()
		if err != nil {
			return fmt.Errorf("build row delete query: %w", err)
		}
		if _, err := tx.Exec(ctx, deleteQuery, deleteArgs...); err != nil {
			return fmt.Errorf("delete rows: %w", err)
		}
		result = records
		return emitRows(ctx, tx, actor, table.id.String(), "row.deleted", records, nil)
	})
	return result, err
}
