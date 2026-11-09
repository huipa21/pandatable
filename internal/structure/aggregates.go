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

type RowAggregateRequest struct {
	Filter      *FilterNode      `json:"filter"`
	Search      *string          `json:"search"`
	SearchField *string          `json:"searchField"`
	FieldIDs    *[]string        `json:"fieldIds"`
	Aggregates  *[]ViewAggregate `json:"aggregates"`
}

type RowAggregate struct {
	FieldID   string          `json:"fieldId"`
	Aggregate codec.Agg       `json:"aggregate"`
	Value     json.RawMessage `json:"value"`
}

type RowAggregateResult struct {
	Aggregates    []RowAggregate `json:"aggregates"`
	SchemaVersion int32          `json:"schemaVersion"`
}

func footerAggregateValueExpression(field rowField, aggregate codec.Agg, expression string) string {
	if aggregateValueIsText(field, aggregate) {
		return "(" + expression + ")::text"
	}
	return "to_jsonb(" + expression + ")"
}

func footerAggregateText(value any) (json.RawMessage, error) {
	var text string
	switch typed := value.(type) {
	case nil:
		return json.RawMessage("null"), nil
	case []byte:
		text = string(typed)
	case string:
		text = typed
	default:
		return nil, fmt.Errorf("footer aggregate returned unexpected text value (%T)", value)
	}
	if strings.Contains(text, ".") {
		text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
		if text == "-0" {
			text = "0"
		}
	}
	encoded, err := json.Marshal(text)
	if err != nil {
		return nil, fmt.Errorf("encode exact footer aggregate value: %w", err)
	}
	return encoded, nil
}

func (s *Service) QueryAggregates(
	ctx context.Context,
	actor Actor,
	viewID string,
	request RowAggregateRequest,
) (RowAggregateResult, error) {
	viewUUID, ok := acl.ParseUUID(viewID)
	if !ok {
		return RowAggregateResult{}, acl.ErrNotFound()
	}
	view, err := s.queries.GetViewForQuery(ctx, viewUUID)
	if err != nil {
		return RowAggregateResult{}, notFoundIfNoRows(err, "read aggregate view")
	}
	if view.Scope == "personal" && (!view.OwnerID.Valid || view.OwnerID.String() != actor.ID) {
		return RowAggregateResult{}, acl.ErrNotFound()
	}
	access, err := s.resolver.Access(ctx, actor.ID, acl.Resource{Type: acl.Table, ID: view.TableID})
	if err != nil {
		return RowAggregateResult{}, err
	}
	if err := access.Check(acl.Read); err != nil {
		return RowAggregateResult{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return RowAggregateResult{}, fmt.Errorf("begin footer aggregate query: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err := tx.Exec(ctx, lockTimeout); err != nil {
		return RowAggregateResult{}, rowDatabaseError(fmt.Errorf("set aggregate lock timeout: %w", err))
	}
	q := structuresql.New(tx)
	lockedView, err := q.LockViewForQuery(ctx, viewUUID)
	if err != nil {
		return RowAggregateResult{}, rowDatabaseError(notFoundIfNoRows(err, "lock aggregate view"))
	}
	if lockedView.Scope == "personal" && (!lockedView.OwnerID.Valid || lockedView.OwnerID.String() != actor.ID) {
		return RowAggregateResult{}, acl.ErrNotFound()
	}
	table, err := loadRowTable(ctx, q, tx, lockedView.TableID, lockedView.PgSchema, lockedView.PgName, lockedView.SchemaVersion)
	if err != nil {
		return RowAggregateResult{}, err
	}
	var config rowViewConfig
	if err := json.Unmarshal(lockedView.Config, &config); err != nil {
		return RowAggregateResult{}, fmt.Errorf("decode stored aggregate view config: %w", err)
	}
	aggregates := config.Aggregates
	if request.Aggregates != nil {
		aggregates = *request.Aggregates
	}
	if err := validateGroupConfig(ViewColumns{Aggregates: aggregates}, table); err != nil {
		return RowAggregateResult{}, err
	}
	if err := validateQuickSearch(request.Search); err != nil {
		return RowAggregateResult{}, err
	}

	fields := visibleRowFields(table, config.Fields, request.FieldIDs)
	visible := make(map[string]bool, len(fields))
	for _, field := range fields {
		visible[field.id] = true
	}
	effective := make([]ViewAggregate, 0, len(aggregates))
	expressions := make([]string, 0, len(aggregates))
	compiler := filterCompiler{ctx: ctx, tx: tx, actor: actor, table: table}
	for _, aggregate := range aggregates {
		if !visible[aggregate.FieldID] {
			continue
		}
		field := table.byID[aggregate.FieldID]
		expression, err := groupAggregateExpression(&compiler, field, aggregate.Aggregate)
		if err != nil {
			return RowAggregateResult{}, err
		}
		effective = append(effective, aggregate)
		expressions = append(expressions, expression)
	}

	filterSQL := ""
	var filterArgs []any
	if request.Filter != nil {
		filterSQL, filterArgs, err = compileRowFilter(ctx, tx, actor, table, request.Filter)
		if err != nil {
			return RowAggregateResult{}, err
		}
	}
	searchSQL, searchArgs, err := compileQuickSearch(ctx, tx, actor, table, fields, request.Search, request.SearchField)
	if err != nil {
		return RowAggregateResult{}, err
	}
	where, conditionArgs := combineQueryConditions(filterSQL, filterArgs, searchSQL, searchArgs)

	result := RowAggregateResult{
		Aggregates:    make([]RowAggregate, 0, len(effective)),
		SchemaVersion: table.schemaVersion,
	}
	if len(effective) > 0 {
		selectItems := make([]string, len(expressions))
		for index, expression := range expressions {
			aggregate := effective[index]
			field := table.byID[aggregate.FieldID]
			valueExpression := footerAggregateValueExpression(field, aggregate.Aggregate, expression)
			selectItems[index] = shiftSQLArguments(valueExpression, len(conditionArgs)) + " AS " +
				pgx.Identifier{fmt.Sprintf("agg_%d", index)}.Sanitize()
		}
		query := "SELECT " + strings.Join(selectItems, ", ") + " FROM " +
			table.qualifiedName() + ` AS "source"`
		if where != "" {
			query += " WHERE " + where
		}
		args := append(append([]any(nil), conditionArgs...), compiler.args...)
		rows, err := queryUncachedRows(ctx, tx, query, args...)
		if err != nil {
			return RowAggregateResult{}, rowDatabaseError(fmt.Errorf("query footer aggregates: %w", err))
		}
		if !rows.Next() {
			err := rows.Err()
			rows.Close()
			if err != nil {
				return RowAggregateResult{}, rowDatabaseError(fmt.Errorf("read footer aggregates: %w", err))
			}
			return RowAggregateResult{}, errors.New("footer aggregate query returned no result")
		}
		values, err := rows.Values()
		if err != nil {
			rows.Close()
			return RowAggregateResult{}, rowDatabaseError(fmt.Errorf("read footer aggregate values: %w", err))
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return RowAggregateResult{}, rowDatabaseError(fmt.Errorf("read footer aggregates: %w", err))
		}
		rows.Close()
		if len(values) != len(effective) {
			return RowAggregateResult{}, errors.New("footer aggregate query returned an unexpected column count")
		}
		for index, aggregate := range effective {
			field := table.byID[aggregate.FieldID]
			var value json.RawMessage
			if aggregateValueIsText(field, aggregate.Aggregate) {
				value, err = footerAggregateText(values[index])
			} else {
				value, err = rawJSONValue(values[index])
			}
			if err != nil {
				return RowAggregateResult{}, err
			}
			result.Aggregates = append(result.Aggregates, RowAggregate{
				FieldID: aggregate.FieldID, Aggregate: aggregate.Aggregate, Value: value,
			})
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return RowAggregateResult{}, rowDatabaseError(fmt.Errorf("commit footer aggregate query: %w", err))
	}
	return result, nil
}

func aggregateValueIsText(field rowField, aggregate codec.Agg) bool {
	switch aggregate {
	case codec.AggPercentChecked, codec.AggPercentEmpty, codec.AggPercentFilled, codec.AggPercentUnique:
		return true
	}
	switch field.kind {
	case codec.Integer, codec.Decimal, codec.Float, codec.Currency, codec.Percent,
		codec.Rating, codec.AutoNumber:
		switch aggregate {
		case codec.AggSum, codec.AggAvg, codec.AggMin, codec.AggMax:
			return true
		}
	}
	return false
}
