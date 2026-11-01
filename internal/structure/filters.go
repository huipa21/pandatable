package structure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl"
	"pandatable/internal/codec"
	"pandatable/internal/ddl"
	"pandatable/internal/formula"
)

const maxFilterNodes = 100

type FilterNode struct {
	Op    string          `json:"op,omitempty"`
	Items []FilterNode    `json:"items,omitempty"`
	Field string          `json:"field,omitempty"`
	Cmp   codec.Op        `json:"cmp,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
}

type filterCompiler struct {
	ctx   context.Context
	tx    pgx.Tx
	actor Actor
	table rowTable
	args  []any
	nodes int
}

var numberedSQLArgument = regexp.MustCompile(`\$([0-9]+)`)
var filterNumber = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

func shiftSQLArguments(query string, count int) string {
	if count == 0 {
		return query
	}
	return numberedSQLArgument.ReplaceAllStringFunc(query, func(match string) string {
		index, err := strconv.Atoi(match[1:])
		if err != nil {
			return match
		}
		return fmt.Sprintf("$%d", index+count)
	})
}

func compileRowFilter(ctx context.Context, tx pgx.Tx, actor Actor, table rowTable, root *FilterNode) (string, []any, error) {
	compiler := filterCompiler{ctx: ctx, tx: tx, actor: actor, table: table}
	sql, err := compiler.node(root, 1)
	return sql, compiler.args, err
}

func (compiler *filterCompiler) bind(value any) string {
	compiler.args = append(compiler.args, value)
	return fmt.Sprintf("$%d", len(compiler.args))
}

func (compiler *filterCompiler) node(node *FilterNode, depth int) (string, error) {
	if node == nil {
		return "", validation("Filter is invalid")
	}
	compiler.nodes++
	if compiler.nodes > maxFilterNodes {
		return "", validation("Filter contains too many conditions")
	}
	if node.Op != "" {
		if depth > 3 || (node.Op != "and" && node.Op != "or") ||
			len(node.Items) == 0 || node.Field != "" || node.Cmp != "" || len(node.Value) != 0 {
			return "", validation("Filter group is invalid")
		}
		parts := make([]string, len(node.Items))
		for index := range node.Items {
			part, err := compiler.node(&node.Items[index], depth+1)
			if err != nil {
				return "", err
			}
			parts[index] = "(" + part + ")"
		}
		return strings.Join(parts, " "+strings.ToUpper(node.Op)+" "), nil
	}
	if len(node.Items) != 0 || node.Field == "" || node.Cmp == "" || len(node.Value) == 0 || !json.Valid(node.Value) {
		return "", validation("Filter condition is invalid")
	}
	field, ok := compiler.table.byID[node.Field]
	if !ok {
		return "", validation("Filter field is unavailable")
	}
	ops, err := compiler.fieldOps(field)
	if err != nil {
		return "", err
	}
	if !containsFilterOp(ops, node.Cmp) {
		return "", validation("Filter operator is not supported for this field")
	}
	if field.relation != nil {
		return compiler.linkCondition(field, node.Cmp, node.Value)
	}
	expression, valueKind, err := compiler.fieldExpression(field)
	if err != nil {
		return "", err
	}
	if field.computed != nil && field.kind == "lookup" {
		return compiler.lookupCondition(field, expression, valueKind, node.Cmp, node.Value)
	}
	return compiler.condition(field, expression, valueKind, node.Cmp, node.Value)
}

func (compiler *filterCompiler) fieldOps(field rowField) ([]codec.Op, error) {
	if field.relation != nil {
		return []codec.Op{
			codec.OpHasAnyOf, codec.OpIsEmpty, codec.OpIsNotEmpty,
			codec.OpEq, codec.OpNeq, codec.OpLt, codec.OpLte, codec.OpGt, codec.OpGte, codec.OpBetween,
		}, nil
	}
	if field.codec != nil {
		return field.codec.FilterOps(), nil
	}
	if field.computed != nil {
		if field.kind == "rollup" && (field.computed.aggregate == "count" ||
			field.computed.aggregate == "sum" || field.computed.aggregate == "avg") {
			return []codec.Op{codec.OpEq, codec.OpNeq, codec.OpLt, codec.OpLte, codec.OpGt, codec.OpGte,
				codec.OpBetween, codec.OpIsEmpty, codec.OpIsNotEmpty}, nil
		}
		if field.kind == "lookup" {
			return field.computed.targetCodec.FilterOps(), nil
		}
		return field.computed.targetCodec.FilterOps(), nil
	}
	if field.formula != nil {
		switch field.formula.options.ResultType {
		case formula.Number:
			return []codec.Op{codec.OpEq, codec.OpNeq, codec.OpLt, codec.OpLte, codec.OpGt, codec.OpGte, codec.OpBetween, codec.OpIsEmpty, codec.OpIsNotEmpty}, nil
		case formula.Text:
			return []codec.Op{codec.OpIs, codec.OpIsNot, codec.OpContains, codec.OpNotContains, codec.OpStartsWith, codec.OpEndsWith, codec.OpIsEmpty, codec.OpIsNotEmpty}, nil
		case formula.Date, formula.DateTime:
			return []codec.Op{codec.OpIs, codec.OpIsNot, codec.OpBefore, codec.OpAfter, codec.OpOnOrBefore, codec.OpOnOrAfter, codec.OpBetween, codec.OpIsWithin, codec.OpIsEmpty, codec.OpIsNotEmpty}, nil
		case formula.Boolean:
			return []codec.Op{codec.OpIsChecked, codec.OpIsNotChecked}, nil
		}
	}
	return nil, validation("Filter field has no supported operators")
}

func containsFilterOp(ops []codec.Op, want codec.Op) bool {
	for _, op := range ops {
		if op == want {
			return true
		}
	}
	return false
}

func filterFieldReadable(ctx context.Context, tx pgx.Tx, user pgtype.UUID, table rowTable, field rowField, visiting map[string]bool) (bool, error) {
	if visiting[field.id] || field.error != "" {
		return false, nil
	}
	if field.computed != nil {
		access, err := acl.AccessUncached(ctx, tx, user, acl.Resource{
			Type: acl.Table, ID: field.computed.targetTable,
		})
		if err != nil {
			return false, fmt.Errorf("check computed filterability: %w", err)
		}
		return access.Visible && access.Check(acl.Read) == nil, nil
	}
	if field.formula == nil {
		return true, nil
	}
	visiting[field.id] = true
	defer delete(visiting, field.id)
	for _, dependency := range field.formula.options.FieldIDs {
		referenced, ok := table.byID[dependency]
		if !ok {
			return false, nil
		}
		readable, err := filterFieldReadable(ctx, tx, user, table, referenced, visiting)
		if err != nil || !readable {
			return readable, err
		}
	}
	return true, nil
}

func (compiler *filterCompiler) fieldExpression(field rowField) (string, codec.Kind, error) {
	if field.computed != nil {
		user, ok := acl.ParseUUID(compiler.actor.ID)
		if !ok {
			return "", "", acl.ErrValidation()
		}
		target := acl.Resource{Type: acl.Table, ID: field.computed.targetTable}
		access, err := acl.AccessUncached(compiler.ctx, compiler.tx, user, target)
		if err != nil {
			return "", "", fmt.Errorf("check filter target access: %w", err)
		}
		if !access.Visible || access.Check(acl.Read) != nil {
			return "", "", validation("Filter is unavailable for this field")
		}
		expression, err := computedExpression(compiler.table, field)
		if err != nil {
			return "", "", err
		}
		if field.kind == "lookup" {
			return expression, field.computed.targetKind, nil
		}
		if field.computed.aggregate == "count" || field.computed.aggregate == "sum" || field.computed.aggregate == "avg" {
			return rollupScalarExpression(field, expression), codec.Decimal, nil
		}
		return rollupScalarExpression(field, expression), field.computed.targetKind, nil
	}
	if field.formula != nil {
		expression, kind, err := compiler.formulaExpression(field)
		return expression, kind, err
	}
	if field.system {
		column, ok := systemColumn(field.kind)
		if !ok || field.column != column {
			return "", "", validation("Invalid system field")
		}
	} else if err := ddl.ValidateIdentifier(ddl.Column, field.column); err != nil {
		return "", "", err
	}
	return pgx.Identifier{"source", field.column}.Sanitize(), field.kind, nil
}

func rollupScalarExpression(field rowField, expression string) string {
	value := "(" + expression + ` #>> '{}' )`
	switch field.computed.aggregate {
	case "count", "sum", "avg":
		return "(" + value + ")::numeric"
	}
	switch field.computed.targetKind {
	case codec.Integer, codec.Decimal, codec.Float, codec.Currency, codec.Percent,
		codec.Rating, codec.AutoNumber:
		return "(" + value + ")::numeric"
	case codec.Date:
		return "(" + value + ")::date"
	case codec.DateTime, codec.CreatedAt, codec.UpdatedAt:
		return "(" + value + ")::timestamptz"
	case codec.Time:
		return "(" + value + ")::time"
	case codec.Duration:
		return "(" + value + ")::interval"
	default:
		return value
	}
}

func (compiler *filterCompiler) formulaExpression(field rowField) (string, codec.Kind, error) {
	user, ok := acl.ParseUUID(compiler.actor.ID)
	if !ok {
		return "", "", acl.ErrValidation()
	}
	stack := map[string]bool{field.id: true}
	timezone := compiler.actor.Timezone
	if timezone == "" {
		timezone = "Asia/Shanghai"
	}
	compiled, err := formula.Compile(field.formula.options.AST, func(fieldID string, child *formula.Compiler) (formula.SQLValue, error) {
		referenced, found := compiler.table.byID[fieldID]
		if !found || referenced.relation != nil {
			return formula.SQLValue{}, errors.New("formula references an unavailable field")
		}
		if referenced.formula != nil {
			if stack[fieldID] || len(stack) >= 8 {
				return formula.SQLValue{}, errors.New("formula dependency cycle or depth exceeded")
			}
			stack[fieldID] = true
			value, err := child.CompileChild(referenced.formula.options.AST)
			delete(stack, fieldID)
			return value, err
		}
		if referenced.computed != nil {
			access, err := acl.AccessUncached(compiler.ctx, compiler.tx, user,
				acl.Resource{Type: acl.Table, ID: referenced.computed.targetTable})
			if err != nil {
				return formula.SQLValue{}, fmt.Errorf("check formula filter access: %w", err)
			}
			if !access.Visible || access.Check(acl.Read) != nil {
				return formula.SQLValue{}, validation("Filter is unavailable for this field")
			}
			value, err := computedExpression(compiler.table, referenced)
			if err != nil {
				return formula.SQLValue{}, err
			}
			if referenced.kind == "lookup" {
				return formula.SQLValue{SQL: value, Type: formula.Array}, nil
			}
			return formula.SQLValue{SQL: rollupScalarExpression(referenced, value), Type: formulaFieldType(referenced)}, nil
		}
		if err := ddl.ValidateIdentifier(ddl.Column, referenced.column); err != nil {
			return formula.SQLValue{}, err
		}
		return formula.SQLValue{SQL: pgx.Identifier{"source", referenced.column}.Sanitize(),
			Type: formulaFieldType(referenced)}, nil
	}, timezone)
	if err != nil {
		return "", "", fmt.Errorf("compile formula filter: %w", err)
	}
	expression := numberedSQLArgument.ReplaceAllStringFunc(compiled.SQL, func(match string) string {
		index, _ := strconv.Atoi(match[1:])
		if index < 1 || index > len(compiled.Args) {
			return match
		}
		return compiler.bind(compiled.Args[index-1])
	})
	kind := codec.Text
	switch compiled.Type {
	case formula.Number:
		kind = codec.Decimal
	case formula.Boolean:
		kind = codec.Checkbox
	case formula.Date:
		kind = codec.Date
	case formula.DateTime:
		kind = codec.DateTime
	case formula.Duration:
		kind = codec.Duration
	}
	return expression, kind, nil
}

func (compiler *filterCompiler) condition(field rowField, expression string, kind codec.Kind, op codec.Op, raw json.RawMessage) (string, error) {
	if op == codec.OpIsEmpty {
		return expression + " IS NULL", nil
	}
	if op == codec.OpIsNotEmpty {
		return expression + " IS NOT NULL", nil
	}
	if kind == codec.Date || kind == codec.DateTime || kind == codec.CreatedAt || kind == codec.UpdatedAt {
		return compiler.temporalCondition(field, expression, kind, op, raw)
	}
	if kind == codec.User || kind == codec.CreatedBy || kind == codec.UpdatedBy {
		if isRelative(raw, "me") {
			if op == codec.OpHasAnyOf {
				return expression + " = ANY(ARRAY[" + compiler.bind(compiler.actor.ID) + "::uuid])", nil
			}
			return expression + " = " + compiler.bind(compiler.actor.ID) + "::uuid", nil
		}
		if op == codec.OpHasAnyOf {
			values, err := compiler.decodeList(field, raw)
			if err != nil {
				return "", err
			}
			return expression + " = ANY(" + compiler.bind(values) + "::uuid[])", nil
		}
		value, err := decodeFilterValue(field, raw)
		if err != nil {
			return "", err
		}
		operator := "="
		if op == codec.OpIsNot {
			operator = "<>"
		}
		return expression + " " + operator + " " + compiler.bind(value) + "::uuid", nil
	}
	if kind == codec.Text || kind == codec.LongText || kind == codec.Email || kind == codec.URL ||
		kind == codec.Phone || kind == codec.SingleSelect || kind == codec.User || kind == codec.CreatedBy ||
		kind == codec.UpdatedBy || field.formula != nil && kind == codec.Text {
		return compiler.textCondition(field, expression, op, raw, kind)
	}
	if kind == codec.MultiSelect || kind == codec.TextArray || kind == codec.IntArray || kind == codec.MultiUser {
		return compiler.arrayCondition(field, expression, kind, op, raw)
	}
	if kind == codec.JSON {
		return compiler.jsonCondition(field, expression, op, raw)
	}
	if kind == codec.Inet || kind == codec.CIDR {
		if op == codec.OpContainedIn {
			value, err := decodeNetworkFilter(raw)
			if err != nil {
				return "", err
			}
			return expression + " <<= " + compiler.bind(value), nil
		}
	}
	if op == codec.OpIsChecked {
		return expression + " IS TRUE", nil
	}
	if op == codec.OpIsNotChecked {
		return expression + " IS FALSE", nil
	}
	if op == codec.OpIsAnyOf || op == codec.OpIsNoneOf {
		values, err := compiler.decodeList(field, raw)
		if err != nil {
			return "", err
		}
		cast := "::text[]"
		if kind == codec.User || kind == codec.CreatedBy || kind == codec.UpdatedBy {
			cast = "::uuid[]"
		}
		operator := "= ANY("
		if op == codec.OpIsNoneOf {
			operator = "<> ALL("
		}
		return expression + " " + operator + compiler.bind(values) + cast + ")", nil
	}
	if op == codec.OpBetween {
		values, err := compiler.decodePair(field, raw)
		if err != nil {
			return "", err
		}
		return expression + " BETWEEN " + compiler.bind(values[0]) + " AND " + compiler.bind(values[1]), nil
	}
	value, err := decodeFilterValue(field, raw)
	if err != nil {
		return "", err
	}
	operator := map[codec.Op]string{
		codec.OpEq: "=", codec.OpIs: "=", codec.OpNeq: "<>", codec.OpIsNot: "<>",
		codec.OpLt: "<", codec.OpBefore: "<", codec.OpLte: "<=", codec.OpOnOrBefore: "<=",
		codec.OpGt: ">", codec.OpAfter: ">", codec.OpGte: ">=", codec.OpOnOrAfter: ">=",
	}[op]
	if operator == "" {
		return "", validation("Filter operator is not implemented for this field")
	}
	if kind == codec.Decimal || kind == codec.Currency || kind == codec.Percent {
		return expression + " " + operator + " " + compiler.bind(value) + "::numeric", nil
	}
	return expression + " " + operator + " " + compiler.bind(value), nil
}

func decodeNetworkFilter(raw json.RawMessage) (netip.Prefix, error) {
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return netip.Prefix{}, validation("Network filter value is invalid")
	}
	prefix, err := netip.ParsePrefix(text)
	if err != nil {
		return netip.Prefix{}, validation("Network filter value is invalid")
	}
	return prefix.Masked(), nil
}

func (compiler *filterCompiler) textCondition(field rowField, expression string, op codec.Op, raw json.RawMessage, kind codec.Kind) (string, error) {
	if op == codec.OpIsAnyOf || op == codec.OpIsNoneOf {
		values, err := compiler.decodeList(field, raw)
		if err != nil {
			return "", err
		}
		comparison := "lower(" + expression + ") = ANY(ARRAY(SELECT lower(x) FROM unnest(" + compiler.bind(values) + "::text[]) x))"
		if op == codec.OpIsNoneOf {
			comparison = "NOT (" + comparison + ")"
		}
		return comparison, nil
	}
	value, err := decodeFilterValue(field, raw)
	if err != nil {
		return "", err
	}
	text, ok := value.(string)
	if !ok {
		return "", validation("Text filter value must be a string")
	}
	pattern := escapeLike(text)
	switch op {
	case codec.OpContains, codec.OpNotContains:
		pattern = "%" + pattern + "%"
	case codec.OpStartsWith:
		pattern += "%"
	case codec.OpEndsWith:
		pattern = "%" + pattern
	default:
		comparison := "lower(" + expression + ") = lower(" + compiler.bind(text) + "::text)"
		if op == codec.OpIsNot {
			comparison = "NOT (" + comparison + ")"
		}
		return comparison, nil
	}
	comparison := expression + " ILIKE " + compiler.bind(pattern) + " ESCAPE '\\'"
	if op == codec.OpNotContains {
		comparison = "NOT (" + comparison + ")"
	}
	_ = kind
	return comparison, nil
}

func escapeLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}

func (compiler *filterCompiler) jsonCondition(field rowField, expression string, op codec.Op, raw json.RawMessage) (string, error) {
	switch op {
	case codec.OpHasKey:
		var key string
		if json.Unmarshal(raw, &key) != nil || len(key) > 256 {
			return "", validation("JSON key filter value is invalid")
		}
		return "jsonb_exists(" + expression + ", " + compiler.bind(key) + ")", nil
	case codec.OpPathEquals:
		var value struct {
			Path  []string        `json:"path"`
			Value json.RawMessage `json:"value"`
		}
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&value) != nil || len(value.Path) == 0 || len(value.Path) > 32 {
			return "", validation("JSON path filter value is invalid")
		}
		for _, segment := range value.Path {
			if len(segment) > 256 {
				return "", validation("JSON path filter value is invalid")
			}
		}
		text, err := decodeJSONScalar(value.Value)
		if err != nil {
			return "", err
		}
		return expression + " #>> " + compiler.bind(value.Path) + "::text[] = " + compiler.bind(text), nil
	case codec.OpIs, codec.OpIsNot:
		value, err := decodeFilterValue(field, raw)
		if err != nil {
			return "", err
		}
		operator := "="
		if op == codec.OpIsNot {
			operator = "<>"
		}
		return expression + " " + operator + " " + compiler.bind(value) + "::jsonb", nil
	default:
		return "", validation("JSON filter operator is invalid")
	}
}

func decodeJSONScalar(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || !json.Valid(raw) {
		return "", validation("JSON path comparison value is invalid")
	}
	switch raw[0] {
	case '"':
		var value string
		if json.Unmarshal(raw, &value) != nil {
			return "", validation("JSON path comparison value is invalid")
		}
		return value, nil
	case 't', 'f':
		var value bool
		if json.Unmarshal(raw, &value) != nil {
			return "", validation("JSON path comparison value is invalid")
		}
		return strconv.FormatBool(value), nil
	default:
		text := string(raw)
		if !filterNumber.MatchString(text) {
			return "", validation("JSON path comparison value is invalid")
		}
		return text, nil
	}
}

func (compiler *filterCompiler) arrayCondition(field rowField, expression string, kind codec.Kind, op codec.Op, raw json.RawMessage) (string, error) {
	values, err := compiler.decodeList(field, raw)
	if err != nil {
		return "", err
	}
	cast := ""
	switch kind {
	case codec.MultiSelect, codec.TextArray:
		cast = "::text[]"
	case codec.IntArray:
		cast = "::bigint[]"
	case codec.MultiUser:
		cast = "::uuid[]"
	}
	bound := compiler.bind(values) + cast
	switch op {
	case codec.OpHasAnyOf:
		return expression + " && " + bound, nil
	case codec.OpHasAllOf:
		return expression + " @> " + bound, nil
	case codec.OpHasNoneOf:
		return "NOT (" + expression + " && " + bound + ")", nil
	case codec.OpIsExactly:
		return expression + " = " + bound, nil
	default:
		_ = kind
		return "", validation("Array filter operator is invalid")
	}
}

func (compiler *filterCompiler) temporalCondition(field rowField, expression string, kind codec.Kind, op codec.Op, raw json.RawMessage) (string, error) {
	if op == codec.OpIsWithin {
		start, end, err := relativeWindow(raw, compiler.actor.Timezone, time.Now())
		if err != nil {
			return "", err
		}
		return expression + " >= " + compiler.bind(temporalParam(kind, start)) + " AND " + expression + " < " +
			compiler.bind(temporalParam(kind, end)), nil
	}
	if relative, ok := relativeValue(raw); ok {
		start, end, err := relativeDay(relative, compiler.actor.Timezone, time.Now())
		if err != nil {
			return "", err
		}
		switch op {
		case codec.OpIs:
			return expression + " >= " + compiler.bind(temporalParam(kind, start)) + " AND " + expression + " < " +
				compiler.bind(temporalParam(kind, end)), nil
		case codec.OpIsNot:
			return "NOT (" + expression + " >= " + compiler.bind(temporalParam(kind, start)) + " AND " + expression + " < " +
				compiler.bind(temporalParam(kind, end)) + ")", nil
		case codec.OpBefore:
			return expression + " < " + compiler.bind(temporalParam(kind, start)), nil
		case codec.OpAfter:
			return expression + " >= " + compiler.bind(temporalParam(kind, end)), nil
		case codec.OpOnOrBefore:
			return expression + " < " + compiler.bind(temporalParam(kind, end)), nil
		case codec.OpOnOrAfter:
			return expression + " >= " + compiler.bind(temporalParam(kind, start)), nil
		}
		return "", validation("Relative date value is not supported for this operator")
	}
	if op == codec.OpBetween {
		values, err := compiler.decodePair(field, raw)
		if err != nil {
			return "", err
		}
		return expression + " BETWEEN " + compiler.bind(values[0]) + " AND " + compiler.bind(values[1]), nil
	}
	value, err := decodeTemporalValue(field, kind, raw)
	if err != nil {
		return "", err
	}
	operator := map[codec.Op]string{
		codec.OpIs: "=", codec.OpIsNot: "<>", codec.OpBefore: "<", codec.OpAfter: ">",
		codec.OpOnOrBefore: "<=", codec.OpOnOrAfter: ">=",
	}[op]
	if operator == "" {
		return "", validation("Temporal filter operator is invalid")
	}
	return expression + " " + operator + " " + compiler.bind(value), nil
}

func temporalParam(kind codec.Kind, value time.Time) any {
	if kind == codec.Date {
		date := time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
		return pgtype.Date{Time: date, Valid: true}
	}
	return value.UTC()
}

func relativeValue(raw json.RawMessage) (string, bool) {
	var value struct {
		Rel string `json:"rel"`
	}
	if json.Unmarshal(raw, &value) != nil || value.Rel == "" {
		return "", false
	}
	return value.Rel, true
}

func isRelative(raw json.RawMessage, want string) bool {
	value, ok := relativeValue(raw)
	return ok && value == want
}

func relativeDay(value, timezone string, now time.Time) (time.Time, time.Time, error) {
	location, err := filterLocation(timezone)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	local := now.In(location)
	offset := 0
	switch value {
	case "today":
	case "yesterday":
		offset = -1
	case "tomorrow":
		offset = 1
	default:
		if strings.HasPrefix(value, "n_days_ago:") {
			offset, err = strconv.Atoi(strings.TrimPrefix(value, "n_days_ago:"))
			offset = -offset
		} else if strings.HasPrefix(value, "n_days_from_now:") {
			offset, err = strconv.Atoi(strings.TrimPrefix(value, "n_days_from_now:"))
		} else {
			return time.Time{}, time.Time{}, validation("Relative date value is invalid")
		}
		if err != nil || offset < -36500 || offset > 36500 {
			return time.Time{}, time.Time{}, validation("Relative date value is invalid")
		}
	}
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location).AddDate(0, 0, offset)
	return day, day.AddDate(0, 0, 1), nil
}

func relativeWindow(raw json.RawMessage, timezone string, now time.Time) (time.Time, time.Time, error) {
	var value struct {
		Rel   string `json:"rel"`
		Days  int    `json:"days"`
		Count int    `json:"count"`
	}
	if json.Unmarshal(raw, &value) != nil {
		return time.Time{}, time.Time{}, validation("Relative date range is invalid")
	}
	location, err := filterLocation(timezone)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	local := now.In(location)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	switch value.Rel {
	case "past", "next":
		days := value.Days
		if days == 0 {
			days = value.Count
		}
		if days < 1 || days > 36500 {
			return time.Time{}, time.Time{}, validation("Relative date range is invalid")
		}
		if value.Rel == "past" {
			start = start.AddDate(0, 0, -days+1)
			return start, time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location).AddDate(0, 0, 1), nil
		}
		return start, start.AddDate(0, 0, days), nil
	case "this_week":
		start = start.AddDate(0, 0, -((int(start.Weekday()) + 6) % 7))
		return start, start.AddDate(0, 0, 7), nil
	case "this_month":
		start = time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, location)
		return start, start.AddDate(0, 1, 0), nil
	case "this_year":
		start = time.Date(local.Year(), 1, 1, 0, 0, 0, 0, location)
		return start, start.AddDate(1, 0, 0), nil
	default:
		return time.Time{}, time.Time{}, validation("Relative date range is invalid")
	}
}

func filterLocation(timezone string) (*time.Location, error) {
	if timezone == "" {
		timezone = "Asia/Shanghai"
	}
	location, err := codec.LoadLocation(timezone)
	if err != nil {
		return nil, validation("User timezone is invalid")
	}
	return location, nil
}

func decodeTemporalValue(field rowField, kind codec.Kind, raw json.RawMessage) (any, error) {
	if field.codec != nil && !field.codec.ReadOnly() {
		value, err := field.codec.Decode(raw)
		if err != nil {
			return nil, validation("Temporal filter value is invalid")
		}
		return value, nil
	}
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return nil, validation("Temporal filter value is invalid")
	}
	if kind == codec.Date {
		value, err := time.Parse("2006-01-02", text)
		if err != nil {
			return nil, validation("Temporal filter value is invalid")
		}
		return pgtype.Date{Time: value, Valid: true}, nil
	}
	value, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return nil, validation("Temporal filter value is invalid")
	}
	return value, nil
}

func decodeFilterValue(field rowField, raw json.RawMessage) (any, error) {
	if field.codec != nil && !field.codec.ReadOnly() {
		value, err := field.codec.Decode(raw)
		if err != nil {
			return nil, validation("Filter value is invalid")
		}
		return value, nil
	}
	if field.formula != nil {
		switch field.formula.options.ResultType {
		case formula.Number:
			return decodeNumericFilter(raw)
		case formula.Text:
			var value string
			if json.Unmarshal(raw, &value) != nil {
				return nil, validation("Filter value is invalid")
			}
			return value, nil
		case formula.Boolean:
			var value bool
			if json.Unmarshal(raw, &value) != nil {
				return nil, validation("Filter value is invalid")
			}
			return value, nil
		case formula.Date, formula.DateTime:
			return decodeTemporalValue(field, map[formula.Type]codec.Kind{
				formula.Date: codec.Date, formula.DateTime: codec.DateTime,
			}[field.formula.options.ResultType], raw)
		}
	}
	if field.computed != nil {
		targetField := rowField{kind: field.computed.targetKind, codec: field.computed.targetCodec}
		if field.computed.aggregate == "count" || field.computed.aggregate == "sum" || field.computed.aggregate == "avg" {
			targetField.kind = codec.Decimal
			targetField.codec = nil
			return decodeNumericFilter(raw)
		}
		return decodeFilterValue(targetField, raw)
	}
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return nil, validation("Filter value is invalid")
	}
	switch field.kind {
	case codec.RowID:
		value, err := canonicalRowID(text)
		if err != nil {
			return nil, validation("Filter value is invalid")
		}
		return value, nil
	case codec.CreatedAt, codec.UpdatedAt:
		value, err := time.Parse(time.RFC3339Nano, text)
		if err != nil {
			return nil, validation("Filter value is invalid")
		}
		return value, nil
	case codec.CreatedBy, codec.UpdatedBy, codec.User:
		value, ok := acl.ParseUUID(text)
		if !ok {
			return nil, validation("Filter value is invalid")
		}
		return value.String(), nil
	}
	return nil, validation("Filter value is invalid")
}

func decodeNumericFilter(raw json.RawMessage) (string, error) {
	var value string
	if raw[0] == '"' {
		if json.Unmarshal(raw, &value) != nil {
			return "", validation("Numeric filter value is invalid")
		}
	} else {
		value = string(raw)
	}
	if len(value) > 1000 || !filterNumber.MatchString(value) {
		return "", validation("Numeric filter value is invalid")
	}
	return value, nil
}

func (compiler *filterCompiler) decodeList(field rowField, raw json.RawMessage) (any, error) {
	switch field.kind {
	case codec.MultiSelect, codec.TextArray, codec.IntArray, codec.MultiUser:
		if field.codec == nil {
			return nil, validation("Filter value is invalid")
		}
		value, err := field.codec.Decode(raw)
		if err != nil || value == nil {
			return nil, validation("Filter value must be a non-empty list")
		}
		return value, nil
	}
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil || len(items) == 0 || len(items) > 500 {
		return nil, validation("Filter value must be a non-empty list")
	}
	if field.kind == codec.RowID {
		values := make([]int64, len(items))
		for index, item := range items {
			var text string
			if json.Unmarshal(item, &text) != nil {
				return nil, validation("Filter value list is invalid")
			}
			value, err := canonicalRowID(text)
			if err != nil {
				return nil, validation("Filter value list is invalid")
			}
			values[index] = value
		}
		return values, nil
	}
	values := make([]string, len(items))
	for index, item := range items {
		var value string
		if json.Unmarshal(item, &value) != nil || value == "" {
			return nil, validation("Filter value list is invalid")
		}
		if field.kind == codec.SingleSelect && field.codec != nil {
			decoded, err := field.codec.Decode(item)
			if err != nil {
				return nil, validation("Filter value list is invalid")
			}
			value, _ = decoded.(string)
		}
		values[index] = value
	}
	return values, nil
}

func (compiler *filterCompiler) decodePair(field rowField, raw json.RawMessage) ([]any, error) {
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil || len(items) != 2 {
		return nil, validation("Between requires exactly two values")
	}
	values := make([]any, 2)
	for index, item := range items {
		value, err := decodeFilterValue(field, item)
		if err != nil {
			return nil, err
		}
		values[index] = value
	}
	return values, nil
}

func (compiler *filterCompiler) linkCondition(field rowField, op codec.Op, raw json.RawMessage) (string, error) {
	if op != codec.OpHasAnyOf {
		if op == codec.OpIsEmpty {
			sql, err := compiler.linkExists(field, false)
			return sql, err
		}
		if op == codec.OpIsNotEmpty {
			sql, err := compiler.linkExists(field, true)
			return sql, err
		}
		return compiler.linkCountCondition(field, op, raw)
	}
	var ids []string
	if json.Unmarshal(raw, &ids) != nil || len(ids) == 0 || len(ids) > 500 {
		return "", validation("Link filter requires row ids")
	}
	parsed := make([]int64, len(ids))
	for index, id := range ids {
		value, err := canonicalRowID(id)
		if err != nil {
			return "", validation("Link filter row id is invalid")
		}
		parsed[index] = value
	}
	exists, err := compiler.linkSubquery(field, parsed)
	if err != nil {
		return "", err
	}
	return "EXISTS (" + exists + ")", nil
}

func (compiler *filterCompiler) linkCountCondition(field rowField, op codec.Op, raw json.RawMessage) (string, error) {
	operator := map[codec.Op]string{
		codec.OpEq: "=", codec.OpNeq: "<>", codec.OpLt: "<", codec.OpLte: "<=",
		codec.OpGt: ">", codec.OpGte: ">=",
	}[op]
	subquery, err := compiler.linkSubquery(field, nil)
	if err != nil {
		return "", err
	}
	count := "(SELECT count(*) FROM (" + subquery + `) AS "link_count_rows")`
	if op == codec.OpBetween {
		values, err := decodeCountPair(raw)
		if err != nil {
			return "", err
		}
		return count + " BETWEEN " + compiler.bind(values[0]) + " AND " + compiler.bind(values[1]), nil
	}
	if operator == "" {
		return "", validation("Link count operator is invalid")
	}
	value, err := decodeCount(raw)
	if err != nil {
		return "", err
	}
	return count + " " + operator + " " + compiler.bind(value), nil
}

func decodeCountPair(raw json.RawMessage) ([2]int64, error) {
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil || len(items) != 2 {
		return [2]int64{}, validation("Between requires exactly two link counts")
	}
	left, err := decodeCount(items[0])
	if err != nil {
		return [2]int64{}, err
	}
	right, err := decodeCount(items[1])
	if err != nil {
		return [2]int64{}, err
	}
	return [2]int64{left, right}, nil
}

func decodeCount(raw json.RawMessage) (int64, error) {
	var text string
	if len(raw) > 0 && raw[0] == '"' {
		if json.Unmarshal(raw, &text) != nil {
			return 0, validation("Link count value is invalid")
		}
	} else {
		text = string(raw)
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil || value < 0 {
		return 0, validation("Link count value is invalid")
	}
	return value, nil
}

func (compiler *filterCompiler) linkExists(field rowField, positive bool) (string, error) {
	query, err := compiler.linkSubquery(field, nil)
	if err != nil {
		return "", err
	}
	exists := "EXISTS (" + query + ")"
	if !positive {
		return "NOT " + exists, nil
	}
	return exists, nil
}

func (compiler *filterCompiler) linkSubquery(field rowField, ids []int64) (string, error) {
	relation := field.relation
	source := pgx.Identifier{"source", "id"}.Sanitize()
	whereIDs := ""
	if ids != nil {
		whereIDs = " AND "
		if relation.kind == "one_one" || relation.kind == "one_many" {
			if field.relationSide == "child" {
				whereIDs += pgx.Identifier{"source", relation.fkColumn.String}.Sanitize() + " = ANY(" + compiler.bind(ids) + "::bigint[])"
			} else {
				whereIDs += pgx.Identifier{"child", "id"}.Sanitize() + " = ANY(" + compiler.bind(ids) + "::bigint[])"
			}
		} else {
			targetColumn := "child_id"
			if field.relationSide == "child" {
				targetColumn = "parent_id"
			}
			whereIDs += pgx.Identifier{"links", targetColumn}.Sanitize() + " = ANY(" + compiler.bind(ids) + "::bigint[])"
		}
	}
	switch relation.kind {
	case "one_one", "one_many":
		if relation.fkColumn.Valid {
			targetID := relation.childTable
			if field.relationSide == "child" {
				targetID = relation.parentTable
			}
			table, err := targetTableName(compiler.ctx, compiler.tx, targetID)
			if err != nil {
				return "", err
			}
			if field.relationSide == "parent" {
				target := pgx.Identifier{"child", relation.fkColumn.String}.Sanitize()
				return "SELECT 1 FROM " + table + ` AS "child" WHERE ` + target + "=" + source + whereIDs, nil
			}
			target := pgx.Identifier{"parent", "id"}.Sanitize()
			return "SELECT 1 FROM " + table + ` AS "parent" WHERE ` +
				pgx.Identifier{"source", relation.fkColumn.String}.Sanitize() + "=" + target + whereIDs, nil
		}
	case "many_many":
		if relation.junction.Valid {
			junction := pgx.Identifier{compiler.table.schema, relation.junction.String}.Sanitize()
			sourceColumn, targetColumn := "parent_id", "child_id"
			if field.relationSide == "child" {
				sourceColumn, targetColumn = "child_id", "parent_id"
			}
			query := "SELECT 1 FROM " + junction + ` AS "links" WHERE ` +
				pgx.Identifier{"links", sourceColumn}.Sanitize() + "=" + source + " AND " +
				pgx.Identifier{"links", targetColumn}.Sanitize() + " IS NOT NULL"
			if ids != nil {
				query = strings.TrimSuffix(query, " IS NOT NULL") + whereIDs
			}
			return query, nil
		}
	}
	return "", errors.New("relation filter metadata is invalid")
}

func targetTableName(ctx context.Context, tx pgx.Tx, tableID pgtype.UUID) (string, error) {
	var schema, name string
	if err := tx.QueryRow(ctx, `SELECT b.pg_schema,t.pg_name FROM meta.tables t
		JOIN meta.books b ON b.id=t.book_id WHERE t.id=$1 AND t.deleted_at IS NULL`, tableID).Scan(&schema, &name); err != nil {
		return "", fmt.Errorf("read relation filter table: %w", err)
	}
	if ddl.ValidateIdentifier(ddl.Book, schema) != nil || ddl.ValidateIdentifier(ddl.Table, name) != nil {
		return "", errors.New("relation filter table metadata is invalid")
	}
	return pgx.Identifier{schema, name}.Sanitize(), nil
}

func (compiler *filterCompiler) lookupCondition(field rowField, expression string, kind codec.Kind, op codec.Op, raw json.RawMessage) (string, error) {
	if op == codec.OpIsEmpty || op == codec.OpIsNotEmpty {
		condition := expression + " = '[]'::jsonb"
		if op == codec.OpIsNotEmpty {
			condition = expression + " <> '[]'::jsonb"
		}
		return condition, nil
	}
	alias := `"lookup_value"."value" #>> '{}'`
	if kind == codec.JSON {
		alias = `"lookup_value"."value"`
	}
	target := rowField{kind: kind, codec: field.computed.targetCodec}
	condition, err := compiler.condition(target, alias, kind, op, raw)
	if err != nil {
		return "", err
	}
	return "EXISTS (SELECT 1 FROM jsonb_array_elements(" + expression + `) AS "lookup_value"("value") WHERE ` + condition + ")", nil
}
