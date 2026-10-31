package structure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl"
	"pandatable/internal/codec"
	"pandatable/internal/ddl"
)

const (
	autoIndexThreshold = 10_000
	autoIndexInterval  = time.Minute
)

type automaticIndexKind string

const (
	automaticBtree   automaticIndexKind = "btree"
	automaticTrigram automaticIndexKind = "trigram"
)

type autoIndexNeed struct {
	btree   bool
	trigram bool
}

type autoIndexField struct {
	id      string
	tableID string
	uiType  codec.Kind
	column  string
	pgType  string
	options []byte
	system  bool
	schema  string
	table   string
}

type autoIndexView struct {
	id      string
	tableID string
	config  []byte
}

type automaticIndexConn interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *Service) RunAutoIndexWorker(ctx context.Context, logger *slog.Logger) {
	ticker := time.NewTicker(autoIndexInterval)
	defer ticker.Stop()

	for {
		if err := s.scanAutoIndexes(ctx); err != nil && ctx.Err() == nil {
			logger.Error("automatic index scan failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) scanAutoIndexes(ctx context.Context) error {
	viewRows, err := s.pool.Query(ctx, `
		SELECT v.id::text, v.table_id::text, v.config
		FROM meta.views v
		JOIN meta.tables t ON t.id = v.table_id AND t.deleted_at IS NULL
		JOIN meta.books b ON b.id = t.book_id AND b.deleted_at IS NULL
		WHERE v.deleted_at IS NULL AND v.scope = 'shared' AND v.type = 'grid'`)
	if err != nil {
		return fmt.Errorf("list shared views for automatic indexing: %w", err)
	}
	views := make([]autoIndexView, 0)
	tableIDs := make(map[string]struct{})
	for viewRows.Next() {
		var view autoIndexView
		if err := viewRows.Scan(&view.id, &view.tableID, &view.config); err != nil {
			viewRows.Close()
			return fmt.Errorf("scan shared view for automatic indexing: %w", err)
		}
		views = append(views, view)
		tableIDs[view.tableID] = struct{}{}
	}
	if err := viewRows.Err(); err != nil {
		viewRows.Close()
		return fmt.Errorf("iterate shared views for automatic indexing: %w", err)
	}
	viewRows.Close()
	if len(tableIDs) == 0 {
		return nil
	}

	fieldRows, err := s.pool.Query(ctx, `
		SELECT f.id::text, f.table_id::text, f.ui_type, f.pg_column, f.pg_type,
		       f.options, f.is_system, b.pg_schema, t.pg_name
		FROM meta.fields f
		JOIN meta.tables t ON t.id = f.table_id AND t.deleted_at IS NULL
		JOIN meta.books b ON b.id = t.book_id AND b.deleted_at IS NULL
		WHERE f.deleted_at IS NULL
		  AND EXISTS (
		    SELECT 1 FROM meta.views v
		    WHERE v.table_id = f.table_id AND v.deleted_at IS NULL
		      AND v.scope = 'shared' AND v.type = 'grid')`)
	if err != nil {
		return fmt.Errorf("list shared-view fields for automatic indexing: %w", err)
	}
	fieldsByTable := make(map[string]map[string]autoIndexField)
	for fieldRows.Next() {
		var field autoIndexField
		var column, pgType pgtype.Text
		if err := fieldRows.Scan(&field.id, &field.tableID, &field.uiType, &column, &pgType,
			&field.options, &field.system, &field.schema, &field.table); err != nil {
			fieldRows.Close()
			return fmt.Errorf("scan shared-view field for automatic indexing: %w", err)
		}
		if column.Valid {
			field.column = column.String
		}
		if pgType.Valid {
			field.pgType = pgType.String
		}
		if fieldsByTable[field.tableID] == nil {
			fieldsByTable[field.tableID] = make(map[string]autoIndexField)
		}
		fieldsByTable[field.tableID][field.id] = field
	}
	if err := fieldRows.Err(); err != nil {
		fieldRows.Close()
		return fmt.Errorf("iterate shared-view fields for automatic indexing: %w", err)
	}
	fieldRows.Close()

	viewsByTable := make(map[string][]autoIndexView)
	for _, view := range views {
		viewsByTable[view.tableID] = append(viewsByTable[view.tableID], view)
	}
	sortedTableIDs := make([]string, 0, len(viewsByTable))
	for tableID := range viewsByTable {
		sortedTableIDs = append(sortedTableIDs, tableID)
	}
	sort.Strings(sortedTableIDs)

	var scanErrors []error
	for _, tableID := range sortedTableIDs {
		tableFields := fieldsByTable[tableID]
		needs := make(map[string]autoIndexNeed)
		for _, view := range viewsByTable[tableID] {
			viewNeeds, err := sharedViewIndexNeeds(view.config, tableFields)
			if err != nil {
				scanErrors = append(scanErrors,
					fmt.Errorf("read automatic-index uses from shared view %s: %w", view.id, err))
				continue
			}
			mergeAutoIndexNeeds(needs, viewNeeds)
		}

		pending := make(map[string]autoIndexNeed, len(needs))
		for fieldID, need := range needs {
			field := tableFields[fieldID]
			missing, err := missingAutomaticIndexKinds(field.options, need)
			if err != nil {
				scanErrors = append(scanErrors, fmt.Errorf("read automatic-index state for field %s: %w", fieldID, err))
				missing = need
			}
			if missing.btree || missing.trigram {
				pending[fieldID] = missing
			}
		}
		eligibleIDs := make([]string, 0, len(pending))
		for fieldID := range pending {
			eligibleIDs = append(eligibleIDs, fieldID)
		}
		sort.Strings(eligibleIDs)
		if len(eligibleIDs) == 0 {
			continue
		}
		tableField := tableFields[eligibleIDs[0]]
		tableName, err := physicalTable(tableField.schema, tableField.table)
		if err != nil {
			scanErrors = append(scanErrors, fmt.Errorf("validate automatic-index table: %w", err))
			continue
		}
		var rowCount int64
		if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM "+tableName).Scan(&rowCount); err != nil {
			scanErrors = append(scanErrors, fmt.Errorf("count rows for automatic-index table %s: %w", tableID, err))
			continue
		}
		if rowCount <= autoIndexThreshold {
			continue
		}
		for _, fieldID := range eligibleIDs {
			field := tableFields[fieldID]
			if err := s.ensureAutomaticIndexes(ctx, field, pending[fieldID]); err != nil {
				scanErrors = append(scanErrors, fmt.Errorf("index field %s: %w", fieldID, err))
			}
		}
	}
	return errors.Join(scanErrors...)
}

func missingAutomaticIndexKinds(options []byte, need autoIndexNeed) (autoIndexNeed, error) {
	var values map[string]json.RawMessage
	if err := json.Unmarshal(options, &values); err != nil {
		return need, fmt.Errorf("decode field options: %w", err)
	}
	indexedJSON, exists := values["indexed"]
	if !exists {
		return need, nil
	}
	var indexed map[string]bool
	if err := json.Unmarshal(indexedJSON, &indexed); err != nil || indexed == nil {
		return need, errors.New("field options indexed member must be an object")
	}
	return autoIndexNeed{
		btree:   need.btree && !indexed[string(automaticBtree)],
		trigram: need.trigram && !indexed[string(automaticTrigram)],
	}, nil
}

func sharedViewIndexNeeds(raw []byte, fields map[string]autoIndexField) (map[string]autoIndexNeed, error) {
	var config struct {
		Fields  []ViewColumn    `json:"fields"`
		Sort    []ViewSort      `json:"sort"`
		GroupBy []string        `json:"groupBy"`
		Filter  json.RawMessage `json:"filter"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, fmt.Errorf("decode view config: %w", err)
	}
	needs := make(map[string]autoIndexNeed)
	addBtree := func(id string) {
		field, ok := fields[id]
		if ok && !field.system && field.column != "" && supportsBtreeType(field.pgType) {
			need := needs[id]
			need.btree = true
			needs[id] = need
		}
	}
	for _, item := range config.Sort {
		addBtree(item.Field)
	}
	for _, id := range config.GroupBy {
		addBtree(id)
	}
	if len(config.Filter) > 0 && string(config.Filter) != "null" {
		var filter FilterNode
		if err := json.Unmarshal(config.Filter, &filter); err != nil {
			return nil, fmt.Errorf("decode view filter: %w", err)
		}
		if err := collectFilterIndexNeeds(&filter, fields, needs); err != nil {
			return nil, err
		}
	}

	visible := make(map[string]bool, len(config.Fields))
	for _, column := range config.Fields {
		visible[column.ID] = column.Visible
	}
	for id, field := range fields {
		isVisible, configured := visible[id]
		if (!configured && !field.system || configured && isVisible) && supportsTrigramType(field) {
			need := needs[id]
			need.trigram = true
			needs[id] = need
		}
	}
	return needs, nil
}

func collectFilterIndexNeeds(node *FilterNode, fields map[string]autoIndexField, needs map[string]autoIndexNeed) error {
	if node == nil {
		return errors.New("stored view filter is empty")
	}
	if node.Op != "" {
		if node.Op != "and" && node.Op != "or" || len(node.Items) == 0 {
			return errors.New("stored view filter group is invalid")
		}
		for index := range node.Items {
			if err := collectFilterIndexNeeds(&node.Items[index], fields, needs); err != nil {
				return err
			}
		}
		return nil
	}
	if node.Field == "" || node.Cmp == "" {
		return errors.New("stored view filter condition is invalid")
	}
	field, ok := fields[node.Field]
	if ok && !field.system && field.column != "" && supportsBtreeFilter(field.pgType, node.Cmp) {
		need := needs[node.Field]
		need.btree = true
		needs[node.Field] = need
	}
	return nil
}

func mergeAutoIndexNeeds(target, source map[string]autoIndexNeed) {
	for id, sourceNeed := range source {
		need := target[id]
		need.btree = need.btree || sourceNeed.btree
		need.trigram = need.trigram || sourceNeed.trigram
		target[id] = need
	}
}

func supportsBtreeType(pgType string) bool {
	switch pgType {
	case "text", "varchar", "smallint", "int2", "integer", "int4", "bigint", "int8",
		"numeric", "real", "float4", "double precision", "float8", "boolean", "bool",
		"date", "time", "timestamptz", "interval", "uuid", "inet", "cidr",
		"text[]", "bigint[]", "int8[]", "uuid[]":
		return true
	default:
		return strings.HasPrefix(pgType, "varchar(") || strings.HasPrefix(pgType, "numeric(")
	}
}

func supportsBtreeFilter(pgType string, operator codec.Op) bool {
	if !supportsBtreeType(pgType) {
		return false
	}
	if operator == codec.OpIsEmpty || operator == codec.OpIsNotEmpty {
		return true
	}
	switch pgType {
	case "smallint", "int2", "integer", "int4", "bigint", "int8", "numeric",
		"real", "float4", "double precision", "float8",
		"date", "time", "timestamptz", "interval":
		switch operator {
		case codec.OpEq, codec.OpNeq, codec.OpLt, codec.OpLte, codec.OpGt, codec.OpGte,
			codec.OpBetween, codec.OpIs, codec.OpIsNot, codec.OpBefore, codec.OpAfter,
			codec.OpOnOrBefore, codec.OpOnOrAfter:
			return true
		}
	case "boolean", "bool":
		return operator == codec.OpIsChecked || operator == codec.OpIsNotChecked
	case "uuid", "inet", "cidr":
		return operator == codec.OpIs || operator == codec.OpIsNot ||
			operator == codec.OpEq || operator == codec.OpNeq ||
			operator == codec.OpLt || operator == codec.OpLte ||
			operator == codec.OpGt || operator == codec.OpGte
	}
	return false
}

func supportsTrigramType(field autoIndexField) bool {
	if field.system || field.column == "" || field.pgType != "text" {
		return false
	}
	switch field.uiType {
	case codec.Text, codec.LongText, codec.SingleSelect, codec.Email, codec.URL, codec.Phone:
		return true
	default:
		return false
	}
}

func (s *Service) ensureAutomaticIndexes(ctx context.Context, field autoIndexField, need autoIndexNeed) error {
	if err := ddl.ValidateIdentifier(ddl.Book, field.schema); err != nil {
		return fmt.Errorf("validate schema: %w", err)
	}
	if err := ddl.ValidateIdentifier(ddl.Table, field.table); err != nil {
		return fmt.Errorf("validate table: %w", err)
	}
	if err := ddl.ValidateIdentifier(ddl.Column, field.column); err != nil {
		return fmt.Errorf("validate column: %w", err)
	}
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire automatic-index connection: %w", err)
	}
	defer conn.Release()

	lockName := field.tableID + ":" + field.id
	var locked bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1, 0))", lockName).Scan(&locked); err != nil {
		return fmt.Errorf("acquire automatic-index advisory lock: %w", err)
	}
	if !locked {
		return nil
	}

	var indexErrors []error
	if need.btree {
		if err := s.ensureAutomaticIndex(ctx, conn, field, automaticBtree); err != nil {
			indexErrors = append(indexErrors, fmt.Errorf("create btree index: %w", err))
		}
	}
	if need.trigram {
		if err := s.ensureAutomaticIndex(ctx, conn, field, automaticTrigram); err != nil {
			indexErrors = append(indexErrors, fmt.Errorf("create trigram index: %w", err))
		}
	}

	unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	var unlocked bool
	unlockErr := conn.QueryRow(unlockCtx, "SELECT pg_advisory_unlock(hashtextextended($1, 0))", lockName).Scan(&unlocked)
	if unlockErr == nil && !unlocked {
		unlockErr = errors.New("automatic-index advisory lock was not held")
	}
	if unlockErr != nil {
		indexErrors = append(indexErrors, fmt.Errorf("release automatic-index advisory lock: %w", unlockErr))
		closeCtx, cancelClose := context.WithTimeout(context.Background(), 5*time.Second)
		if err := conn.Conn().Close(closeCtx); err != nil {
			indexErrors = append(indexErrors, fmt.Errorf("close connection with automatic-index lock: %w", err))
		}
		cancelClose()
	}
	return errors.Join(indexErrors...)
}

func (s *Service) ensureAutomaticIndex(ctx context.Context, conn automaticIndexConn, field autoIndexField, kind automaticIndexKind) error {
	indexName, err := automaticIndexName(field.id, kind)
	if err != nil {
		return err
	}
	exists, valid, err := inspectAutomaticIndex(ctx, conn, field.schema, indexName, field.column, kind)
	if err != nil {
		return err
	}
	index := pgx.Identifier{field.schema, indexName}.Sanitize()
	if exists && !valid {
		if _, err := conn.Exec(ctx, "DROP INDEX CONCURRENTLY "+index); err != nil {
			return fmt.Errorf("drop invalid concurrent index: %w", err)
		}
		exists = false
	}
	if !exists {
		statement, err := automaticIndexDDL(field, indexName, kind)
		if err != nil {
			return err
		}
		if _, err := conn.Exec(ctx, statement); err != nil {
			return fmt.Errorf("execute concurrent index creation: %w", err)
		}
	}
	if err := markAutomaticIndex(ctx, conn, field, kind); err != nil {
		return err
	}
	return nil
}

func inspectAutomaticIndex(
	ctx context.Context,
	conn automaticIndexConn,
	schema, indexName, column string,
	kind automaticIndexKind,
) (bool, bool, error) {
	var exists bool
	if err := conn.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = $1 AND c.relname = $2)`, schema, indexName).Scan(&exists); err != nil {
		return false, false, fmt.Errorf("check automatic index existence: %w", err)
	}
	if !exists {
		return false, false, nil
	}
	var valid pgtype.Bool
	var method, indexedColumn, opclass pgtype.Text
	var simple bool
	err := conn.QueryRow(ctx, `
		SELECT i.indisvalid, am.amname, a.attname, opc.opcname,
		       i.indnkeyatts = 1 AND i.indnatts = 1
		         AND i.indpred IS NULL AND i.indexprs IS NULL
		FROM pg_class idx
		JOIN pg_namespace n ON n.oid = idx.relnamespace
		JOIN pg_index i ON i.indexrelid = idx.oid
		JOIN pg_am am ON am.oid = idx.relam
		LEFT JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = i.indkey[0]
		LEFT JOIN pg_opclass opc ON opc.oid = i.indclass[0]
		WHERE n.nspname = $1 AND idx.relname = $2`, schema, indexName).
		Scan(&valid, &method, &indexedColumn, &opclass, &simple)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, false, fmt.Errorf("automatic index name %s is occupied by a non-index relation", indexName)
	}
	if err != nil {
		return false, false, fmt.Errorf("inspect automatic index: %w", err)
	}
	expectedMethod := "btree"
	if kind == automaticTrigram {
		expectedMethod = "gin"
	}
	expectedOpclass := kind == automaticTrigram && opclass.Valid && opclass.String == "gin_trgm_ops"
	if !valid.Valid || !method.Valid || !indexedColumn.Valid || !simple ||
		method.String != expectedMethod || indexedColumn.String != column ||
		kind == automaticTrigram && !expectedOpclass {
		return true, false, fmt.Errorf("automatic index %s does not match the requested definition", indexName)
	}
	return true, valid.Bool, nil
}

func automaticIndexName(fieldID string, kind automaticIndexKind) (string, error) {
	id, ok := acl.ParseUUID(fieldID)
	if !ok {
		return "", errors.New("automatic index field id is invalid")
	}
	suffix := "b"
	if kind == automaticTrigram {
		suffix = "t"
	} else if kind != automaticBtree {
		return "", errors.New("automatic index kind is invalid")
	}
	return "i_" + strings.ReplaceAll(id.String(), "-", "") + "_" + suffix, nil
}

func automaticIndexDDL(field autoIndexField, indexName string, kind automaticIndexKind) (string, error) {
	table, err := physicalTable(field.schema, field.table)
	if err != nil {
		return "", err
	}
	index := pgx.Identifier{indexName}.Sanitize()
	column := pgx.Identifier{field.column}.Sanitize()
	switch kind {
	case automaticBtree:
		return "CREATE INDEX CONCURRENTLY IF NOT EXISTS " + index +
			" ON " + table + " (" + column + ")", nil
	case automaticTrigram:
		return "CREATE INDEX CONCURRENTLY IF NOT EXISTS " + index +
			" ON " + table + " USING gin (" + column + " gin_trgm_ops)", nil
	default:
		return "", errors.New("automatic index kind is invalid")
	}
}

func markAutomaticIndex(ctx context.Context, conn automaticIndexConn, field autoIndexField, kind automaticIndexKind) error {
	tag, err := conn.Exec(ctx, `
		UPDATE meta.fields
		SET options = jsonb_set(
			options, '{indexed}',
			COALESCE(options->'indexed', '{}'::jsonb) || jsonb_build_object($3::text, true),
			true)
		WHERE id = $1 AND table_id = $2 AND deleted_at IS NULL
		  AND (options->'indexed' IS NULL OR jsonb_typeof(options->'indexed') = 'object')`,
		field.id, field.tableID, string(kind))
	if err != nil {
		return fmt.Errorf("save automatic-index metadata: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	var options []byte
	var indexedType pgtype.Text
	err = conn.QueryRow(ctx, `SELECT options FROM meta.fields
		WHERE id = $1 AND table_id = $2 AND deleted_at IS NULL`, field.id, field.tableID).Scan(&options)
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.New("field was deleted before automatic-index metadata was saved")
	}
	if err != nil {
		return fmt.Errorf("check automatic-index metadata after update: %w", err)
	}
	var optionValues map[string]json.RawMessage
	if err := json.Unmarshal(options, &optionValues); err != nil {
		return fmt.Errorf("decode field options after automatic-index metadata update: %w", err)
	}
	if err := conn.QueryRow(ctx, `SELECT jsonb_typeof(options->'indexed') FROM meta.fields
		WHERE id = $1 AND table_id = $2 AND deleted_at IS NULL`, field.id, field.tableID).Scan(&indexedType); err != nil {
		return fmt.Errorf("read automatic-index metadata type: %w", err)
	}
	if indexedType.Valid && indexedType.String != "object" {
		return errors.New("field options indexed member must be an object")
	}
	return errors.New("automatic-index metadata update affected no rows")
}
