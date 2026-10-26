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
	Offset       *int     `json:"offset"`
	Limit        *int     `json:"limit"`
	Sort         *RowSort `json:"sort"`
	IncludeCount *bool    `json:"includeCount"`
}

type RowRecord struct {
	ID     string                     `json:"id"`
	Fields map[string]json.RawMessage `json:"fields"`
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
	id     string
	kind   codec.Kind
	column string
	system bool
	codec  codec.Field
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
	Sort []struct {
		Field string `json:"field"`
		Dir   string `json:"dir"`
	} `json:"sort"`
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

func loadRowTable(ctx context.Context, q *structuresql.Queries, id pgtype.UUID, schema, name string, version int32) (rowTable, error) {
	if _, err := physicalTable(schema, name); err != nil {
		return rowTable{}, err
	}
	rows, err := q.ListRowFields(ctx, id)
	if err != nil {
		return rowTable{}, fmt.Errorf("list live row fields: %w", err)
	}
	table := rowTable{
		id: id, schema: schema, name: name, schemaVersion: version,
		fields: make([]rowField, 0, len(rows)), byID: make(map[string]rowField, len(rows)),
	}
	for _, row := range rows {
		kind := codec.Kind(row.UiType)
		bound, err := codec.Bind(kind, row.Options)
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
			if bound.ReadOnly() {
				return rowTable{}, errors.New("user field has a read-only codec")
			}
		}
		field := rowField{id: row.ID.String(), kind: kind, column: column, system: row.IsSystem, codec: bound}
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
		columns = append(columns, field.sqlColumn())
	}
	return columns
}

func (table rowTable) encode(values []any, fields []rowField) (RowRecord, error) {
	if len(values) != len(fields)+1 {
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
	for index, field := range fields {
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
	table, err := loadRowTable(ctx, structuresql.New(tx), target.ID, tableRow.PgSchema, tableRow.PgName, tableRow.SchemaVersion)
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
	target, err := resourceOf(acl.Table, tableID)
	if err != nil {
		return err
	}
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return acl.ErrValidation()
	}
	if err := s.authorizeRows(ctx, s.pool, user, target, acl.Write); err != nil {
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
		if err := s.authorizeRows(ctx, tx, user, target, acl.Write); err != nil {
			return err
		}
		tableRow, err := structuresql.New(tx).LockRowTable(ctx, target.ID)
		if err != nil {
			return notFoundIfNoRows(err, "lock row table")
		}
		table, err := loadRowTable(ctx, structuresql.New(tx), target.ID, tableRow.PgSchema, tableRow.PgName, tableRow.SchemaVersion)
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

func sortedFieldValues(table rowTable, input map[string]json.RawMessage) ([]rowField, []any, error) {
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
		if !ok || !valid || parsed.String() != id || field.system {
			return nil, nil, validation("A supplied field is not writable on this table")
		}
		value, err := field.codec.Decode(input[id])
		if err != nil {
			return nil, nil, codec.APIError(err)
		}
		fields = append(fields, field)
		values = append(values, value)
	}
	return fields, values, nil
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
		record, err := scanRow(tx.QueryRow(ctx, query, args...), table, table.fields)
		if errors.Is(err, pgx.ErrNoRows) {
			return acl.ErrNotFound()
		}
		if err != nil {
			return fmt.Errorf("read row: %w", err)
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
	table, err := loadRowTable(ctx, q, lockedView.TableID, lockedView.PgSchema, lockedView.PgName, lockedView.SchemaVersion)
	if err != nil {
		return RowQueryResult{}, err
	}
	config := rowViewConfig{}
	if err := json.Unmarshal(lockedView.Config, &config); err != nil {
		return RowQueryResult{}, fmt.Errorf("decode stored view config: %w", err)
	}
	fields := visibleRowFields(table, config.Fields)
	sortField, direction, err := configuredRowSort(table, config.Sort, request.Sort)
	if err != nil {
		return RowQueryResult{}, err
	}
	builder := sq.Select(table.selectColumnList(fields)...).From(table.qualifiedName())
	if sortField != nil {
		builder = builder.OrderBy(sortField.sqlColumn()+" "+direction+" NULLS LAST", `"id" ASC`)
	} else {
		builder = builder.OrderBy(`"id" ASC`)
	}
	sql, args, err := builder.Limit(uint64(limit)).Offset(uint64(offset)).
		PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return RowQueryResult{}, fmt.Errorf("build view query: %w", err)
	}
	result := RowQueryResult{Rows: make([]RowRecord, 0, limit), SchemaVersion: table.schemaVersion}
	result.FieldIDs = make([]string, len(fields))
	for index, field := range fields {
		result.FieldIDs[index] = field.id
	}
	if includeCount {
		var total int64
		countQuery, countArgs, err := sq.Select("count(*)").From(table.qualifiedName()).
			PlaceholderFormat(sq.Dollar).ToSql()
		if err != nil {
			return RowQueryResult{}, fmt.Errorf("build row count query: %w", err)
		}
		if err := tx.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
			return RowQueryResult{}, rowDatabaseError(fmt.Errorf("count view rows: %w", err))
		}
		result.Total = &total
	}
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return RowQueryResult{}, rowDatabaseError(fmt.Errorf("query view rows: %w", err))
	}
	result.Rows, err = scanRows(rows, table, fields)
	if err != nil {
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
}) []rowField {
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

func configuredRowSort(table rowTable, configured []struct {
	Field string `json:"field"`
	Dir   string `json:"dir"`
}, override *RowSort) (*rowField, string, error) {
	if override != nil {
		field, ok := table.byID[override.FieldID]
		direction := strings.ToUpper(override.Direction)
		if !ok || direction != "ASC" && direction != "DESC" {
			return nil, "", validation("Sort must name a live field and use asc or desc")
		}
		return &field, direction, nil
	}
	if len(configured) == 0 {
		return nil, "", nil
	}
	field, ok := table.byID[configured[0].Field]
	direction := strings.ToUpper(configured[0].Dir)
	if !ok || direction != "ASC" && direction != "DESC" {
		return nil, "", nil
	}
	return &field, direction, nil
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
		for _, item := range input {
			record, err := insertRow(ctx, tx, table, actor, item.Fields)
			if err != nil {
				return err
			}
			result = append(result, record)
		}
		return nil
	})
	return result, err
}

func insertRow(ctx context.Context, tx pgx.Tx, table rowTable, actor Actor, input map[string]json.RawMessage) (RowRecord, error) {
	fields, values, err := sortedFieldValues(table, input)
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
	record, err := scanRow(tx.QueryRow(ctx, query, args...), table, table.fields)
	if err != nil {
		return RowRecord{}, fmt.Errorf("insert row: %w", err)
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
		record, err := patchRow(ctx, tx, table, actor, id, input)
		if err != nil {
			return err
		}
		result = record
		return nil
	})
	return result, err
}

func patchRow(ctx context.Context, tx pgx.Tx, table rowTable, actor Actor, id int64, input map[string]json.RawMessage) (RowRecord, error) {
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
	fields, values, err := sortedFieldValues(table, input)
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
	record, err := scanRow(tx.QueryRow(ctx, query, args...), table, table.fields)
	if err != nil {
		return RowRecord{}, fmt.Errorf("patch row: %w", err)
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
			record, err := patchRow(ctx, tx, table, actor, ids[index], item.Fields)
			if err != nil {
				return err
			}
			result[index] = record
		}
		return nil
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
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("lock rows for deletion: %w", err)
		}
		records, err := scanRows(rows, table, table.fields)
		if err != nil {
			return err
		}
		if len(records) != len(ids) {
			return acl.ErrNotFound()
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
		return nil
	})
	return result, err
}
