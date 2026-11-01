package structure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"pandatable/internal/acl"
	"pandatable/internal/codec"
	"pandatable/internal/structure/structuresql"
)

const maxGroupBuckets = 1000

type RowGroupRequest struct {
	Path     []json.RawMessage `json:"path"`
	Filter   *FilterNode       `json:"filter"`
	Search   *string           `json:"search"`
	FieldIDs *[]string         `json:"fieldIds"`
}

type RowGroupAggregate struct {
	FieldID   string          `json:"fieldId"`
	Aggregate codec.Agg       `json:"aggregate"`
	Value     json.RawMessage `json:"value"`
}

type RowGroupBucket struct {
	Key        json.RawMessage     `json:"key"`
	Count      int64               `json:"count"`
	Aggregates []RowGroupAggregate `json:"aggregates"`
}

type RowGroupResult struct {
	Groups        []RowGroupBucket `json:"groups"`
	SchemaVersion int32            `json:"schemaVersion"`
}

func viewGroupFields(viewType string, config rowViewConfig, table rowTable) []string {
	if viewType != "kanban" {
		return config.GroupBy
	}
	if config.Kanban == nil || config.Kanban.GroupField == nil {
		return nil
	}
	field, ok := table.byID[*config.Kanban.GroupField]
	if !ok || (field.kind != codec.SingleSelect && field.kind != codec.User) {
		return nil
	}
	return []string{field.id}
}

func canonicalGroupKey(field rowField, expression string) string {
	raw := "to_jsonb(" + expression + ")"
	if field.kind == codec.Text && field.codec != nil &&
		field.computed == nil && field.formula == nil && field.relation == nil {
		return "COALESCE(" + raw + ", 'null'::jsonb)"
	}
	return `(CASE
		WHEN ` + raw + ` IS NULL OR ` + raw + ` = 'null'::jsonb OR ` + raw + ` = '[]'::jsonb
			THEN 'null'::jsonb
		WHEN jsonb_typeof(` + raw + `) = 'array' THEN
			(SELECT COALESCE(jsonb_agg(items.value ORDER BY items.value), '[]'::jsonb)
			 FROM jsonb_array_elements(` + raw + `) AS items(value))
		ELSE ` + raw + `
	END)`
}

func compileGroupPath(
	ctx context.Context,
	tx pgx.Tx,
	actor Actor,
	table rowTable,
	groupBy []string,
	path []json.RawMessage,
) (string, []any, error) {
	if len(path) > 3 || len(path) > len(groupBy) {
		return "", nil, validation("Group path exceeds the configured grouping levels")
	}
	compiler := filterCompiler{ctx: ctx, tx: tx, actor: actor, table: table}
	conditions := make([]string, 0, len(path))
	for index, value := range path {
		if len(value) == 0 || !json.Valid(value) {
			return "", nil, validation("Group path contains an invalid key")
		}
		field, ok := table.byID[groupBy[index]]
		if !ok || field.relation != nil {
			return "", nil, validation("Group path field is unavailable")
		}
		expression, _, err := compiler.fieldExpression(field)
		if err != nil {
			return "", nil, err
		}
		key := compiler.bind(string(value))
		conditions = append(conditions, canonicalGroupKey(field, expression)+" IS NOT DISTINCT FROM "+key+"::jsonb")
	}
	return strings.Join(conditions, " AND "), compiler.args, nil
}

func groupAggregateExpression(
	compiler *filterCompiler,
	field rowField,
	aggregate codec.Agg,
) (string, error) {
	expression, _, err := compiler.fieldExpression(field)
	if err != nil {
		return "", err
	}
	key := canonicalGroupKey(field, expression)
	switch aggregate {
	case codec.AggCount:
		return "count(*)", nil
	case codec.AggCountEmpty:
		return "count(*) FILTER (WHERE " + key + " = 'null'::jsonb)", nil
	case codec.AggCountFilled:
		return "count(*) FILTER (WHERE " + key + " <> 'null'::jsonb)", nil
	case codec.AggUnique:
		return "count(DISTINCT " + key + ") FILTER (WHERE " + key + " <> 'null'::jsonb)", nil
	case codec.AggSum:
		return "sum(" + expression + ")", nil
	case codec.AggAvg:
		return "avg(" + expression + ")", nil
	case codec.AggMin:
		return "min(" + expression + ")", nil
	case codec.AggMax:
		return "max(" + expression + ")", nil
	case codec.AggEarliest:
		switch field.kind {
		case codec.Date:
			return "min(" + expression + ")", nil
		case codec.DateTime, codec.CreatedAt, codec.UpdatedAt:
			return "min(" + expression + ")", nil
		default:
			return "", validation("Earliest aggregate requires a date field")
		}
	case codec.AggLatest:
		switch field.kind {
		case codec.Date:
			return "max(" + expression + ")", nil
		case codec.DateTime, codec.CreatedAt, codec.UpdatedAt:
			return "max(" + expression + ")", nil
		default:
			return "", validation("Latest aggregate requires a date field")
		}
	case codec.AggPercentChecked:
		return "avg(CASE WHEN " + expression + " THEN 1.0 ELSE 0.0 END)", nil
	default:
		return "", validation("Configured aggregate is unsupported")
	}
}

func (s *Service) QueryGroups(
	ctx context.Context,
	actor Actor,
	viewID string,
	request RowGroupRequest,
) (RowGroupResult, error) {
	viewUUID, ok := acl.ParseUUID(viewID)
	if !ok {
		return RowGroupResult{}, acl.ErrNotFound()
	}
	view, err := s.queries.GetViewForQuery(ctx, viewUUID)
	if err != nil {
		return RowGroupResult{}, notFoundIfNoRows(err, "read grouped view")
	}
	if view.Scope == "personal" && (!view.OwnerID.Valid || view.OwnerID.String() != actor.ID) {
		return RowGroupResult{}, acl.ErrNotFound()
	}
	access, err := s.resolver.Access(ctx, actor.ID, acl.Resource{Type: acl.Table, ID: view.TableID})
	if err != nil {
		return RowGroupResult{}, err
	}
	if err := access.Check(acl.Read); err != nil {
		return RowGroupResult{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return RowGroupResult{}, fmt.Errorf("begin grouped view query: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err := tx.Exec(ctx, lockTimeout); err != nil {
		return RowGroupResult{}, rowDatabaseError(fmt.Errorf("set grouped query lock timeout: %w", err))
	}
	q := structuresql.New(tx)
	lockedView, err := q.LockViewForQuery(ctx, viewUUID)
	if err != nil {
		return RowGroupResult{}, rowDatabaseError(notFoundIfNoRows(err, "lock grouped view"))
	}
	if lockedView.Scope == "personal" && (!lockedView.OwnerID.Valid || lockedView.OwnerID.String() != actor.ID) {
		return RowGroupResult{}, acl.ErrNotFound()
	}
	table, err := loadRowTable(ctx, q, tx, lockedView.TableID, lockedView.PgSchema, lockedView.PgName, lockedView.SchemaVersion)
	if err != nil {
		return RowGroupResult{}, err
	}
	var config rowViewConfig
	if err := json.Unmarshal(lockedView.Config, &config); err != nil {
		return RowGroupResult{}, fmt.Errorf("decode stored view config: %w", err)
	}
	groupBy := viewGroupFields(lockedView.Type, config, table)
	viewConfig := ViewColumns{GroupBy: groupBy, Aggregates: config.Aggregates}
	if err := validateGroupConfig(viewConfig, table); err != nil {
		return RowGroupResult{}, err
	}
	if len(request.Path) > len(groupBy) || len(request.Path) > 3 {
		return RowGroupResult{}, validation("Group path exceeds the configured grouping levels")
	}
	if len(groupBy) == 0 || len(request.Path) == len(groupBy) {
		return RowGroupResult{
			Groups: []RowGroupBucket{}, SchemaVersion: table.schemaVersion,
		}, nil
	}

	filter := filterCompiler{ctx: ctx, tx: tx, actor: actor, table: table}
	filterSQL := ""
	if request.Filter != nil {
		filterSQL, err = filter.node(request.Filter, 1)
		if err != nil {
			return RowGroupResult{}, err
		}
	}
	if err := validateQuickSearch(request.Search); err != nil {
		return RowGroupResult{}, err
	}
	searchSQL, searchArgs, err := compileQuickSearch(ctx, tx, actor, table,
		visibleRowFields(table, config.Fields, request.FieldIDs), request.Search)
	if err != nil {
		return RowGroupResult{}, err
	}
	conditionSQL, conditionArgs := combineQueryConditions(filterSQL, filter.args, searchSQL, searchArgs)
	groupSQL, groupArgs, err := compileGroupPath(ctx, tx, actor, table, groupBy, request.Path)
	if err != nil {
		return RowGroupResult{}, err
	}
	where := conditionSQL
	if groupSQL != "" {
		if where != "" {
			where += " AND "
		}
		where += "(" + shiftSQLArguments(groupSQL, len(conditionArgs)) + ")"
	}

	field, ok := table.byID[groupBy[len(request.Path)]]
	if !ok || field.relation != nil {
		return RowGroupResult{}, validation("Grouping field is unavailable")
	}
	expressionCompiler := filterCompiler{ctx: ctx, tx: tx, actor: actor, table: table}
	groupExpression, _, err := expressionCompiler.fieldExpression(field)
	if err != nil {
		return RowGroupResult{}, err
	}
	groupExpression = canonicalGroupKey(field, groupExpression)
	aggregateFields := make([]rowField, len(config.Aggregates))
	aggregateExpressions := make([]string, len(config.Aggregates))
	for index, aggregate := range config.Aggregates {
		aggregateField := table.byID[aggregate.FieldID]
		aggregateFields[index] = aggregateField
		aggregateExpressions[index], err = groupAggregateExpression(&expressionCompiler, aggregateField, aggregate.Aggregate)
		if err != nil {
			return RowGroupResult{}, err
		}
	}
	offset := len(conditionArgs) + len(groupArgs)
	selectItems := []string{
		"(" + shiftSQLArguments(groupExpression, offset) + `)::text AS "key"`,
		`count(*)::bigint AS "count"`,
	}
	for index, expression := range aggregateExpressions {
		selectItems = append(selectItems,
			"to_jsonb("+shiftSQLArguments(expression, offset)+") AS "+
				pgx.Identifier{fmt.Sprintf("agg_%d", index)}.Sanitize())
	}
	query := "SELECT " + strings.Join(selectItems, ", ") + " FROM " + table.qualifiedName() + ` AS "source"`
	if where != "" {
		query += " WHERE " + where
	}
	query += ` GROUP BY "key" ORDER BY "key" ASC LIMIT ` + fmt.Sprint(maxGroupBuckets+1)
	args := append(append(append([]any(nil), conditionArgs...), groupArgs...), expressionCompiler.args...)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return RowGroupResult{}, rowDatabaseError(fmt.Errorf("query grouped buckets: %w", err))
	}
	defer rows.Close()
	result := RowGroupResult{
		Groups: make([]RowGroupBucket, 0), SchemaVersion: table.schemaVersion,
	}
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return RowGroupResult{}, rowDatabaseError(fmt.Errorf("read grouped bucket: %w", err))
		}
		if len(values) != 2+len(aggregateFields) {
			return RowGroupResult{}, errors.New("group query returned an unexpected column count")
		}
		key, err := rawJSONValue(values[0])
		if err != nil {
			return RowGroupResult{}, err
		}
		count, ok := values[1].(int64)
		if !ok {
			return RowGroupResult{}, errors.New("group query returned an invalid row count")
		}
		bucket := RowGroupBucket{Key: key, Count: count, Aggregates: make([]RowGroupAggregate, len(aggregateFields))}
		for index, aggregate := range config.Aggregates {
			value, err := rawJSONValue(values[index+2])
			if err != nil {
				return RowGroupResult{}, err
			}
			bucket.Aggregates[index] = RowGroupAggregate{
				FieldID: aggregate.FieldID, Aggregate: aggregate.Aggregate, Value: value,
			}
		}
		result.Groups = append(result.Groups, bucket)
		if len(result.Groups) > maxGroupBuckets {
			return RowGroupResult{}, validation("Group path contains too many buckets")
		}
	}
	if err := rows.Err(); err != nil {
		return RowGroupResult{}, rowDatabaseError(fmt.Errorf("read grouped buckets: %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return RowGroupResult{}, rowDatabaseError(fmt.Errorf("commit grouped query: %w", err))
	}
	return result, nil
}

func rawJSONValue(value any) (json.RawMessage, error) {
	switch typed := value.(type) {
	case nil:
		return json.RawMessage("null"), nil
	case []byte:
		if !json.Valid(typed) {
			return nil, fmt.Errorf("group query returned invalid JSON (%T)", value)
		}
		return json.RawMessage(typed), nil
	case string:
		if !json.Valid([]byte(typed)) {
			return nil, fmt.Errorf("group query returned invalid JSON (%T)", value)
		}
		return json.RawMessage(typed), nil
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return nil, fmt.Errorf("encode grouped value: %w", err)
		}
		return json.RawMessage(encoded), nil
	}
}
