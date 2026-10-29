package structure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl"
	"pandatable/internal/apperr"
	"pandatable/internal/codec"
	"pandatable/internal/ddl"
	"pandatable/internal/formula"
)

type formulaOptions struct {
	Expression string        `json:"expr"`
	AST        *formula.Node `json:"ast"`
	FieldIDs   []string      `json:"fieldIds"`
	ResultType formula.Type  `json:"resultType"`
}

type formulaField struct {
	options formulaOptions
}

type FormulaCheckResult struct {
	Type     formula.Type
	Position int
	Message  string
}

func cleanFormulaOptions(raw json.RawMessage, table rowTable, replacingID string, tx pgx.Tx, ctx context.Context) (json.RawMessage, formulaOptions, error) {
	var input struct {
		Expression string `json:"expr"`
	}
	if len(raw) == 0 || isJSONNull(raw) || json.Unmarshal(raw, &input) != nil || strings.TrimSpace(input.Expression) == "" {
		return nil, formulaOptions{}, validation("Formula expression is required")
	}
	ast, err := formula.Parse(input.Expression)
	if err != nil {
		return nil, formulaOptions{}, formulaValidation(err)
	}
	byName := make(map[string]rowField, len(table.fields))
	for _, field := range table.fields {
		if field.name != "" {
			byName[field.name] = field
		}
	}
	if err := formula.BindFields(ast, func(name string) (string, formula.Type, error) {
		field, ok := byName[name]
		if !ok {
			return "", formula.Null, fmt.Errorf("Unknown field %q", name)
		}
		if field.relation != nil {
			return "", formula.Null, errors.New("Relation fields cannot be referenced directly")
		}
		typeOf := formulaFieldType(field)
		if typeOf == formula.Null && field.formula == nil {
			return "", formula.Null, errors.New("This field type is not supported in formulas")
		}
		return field.id, typeOf, nil
	}); err != nil {
		return nil, formulaOptions{}, formulaValidation(err)
	}
	resultType, err := inferFormulaType(ast, table, replacingID, nil)
	if err != nil {
		return nil, formulaOptions{}, formulaValidation(err)
	}
	options := formulaOptions{
		Expression: input.Expression, AST: ast, FieldIDs: formula.FieldIDs(ast), ResultType: resultType,
	}
	if err := validateFormulaGraph(ctx, tx, table, replacingID, ast); err != nil {
		return nil, formulaOptions{}, err
	}
	encoded, err := json.Marshal(options)
	if err != nil {
		return nil, formulaOptions{}, fmt.Errorf("encode formula options: %w", err)
	}
	return encoded, options, nil
}

func formulaValidation(err error) error {
	var formulaError *formula.Error
	if !errors.As(err, &formulaError) {
		return validation("Formula is invalid")
	}
	result := apperr.New(400, "ERR_VALIDATION", formulaError.Message)
	result.Details = map[string]any{"position": formulaError.Position, "reason": "formula_invalid"}
	return result
}

func inferFormulaType(ast *formula.Node, table rowTable, current string, stack map[string]bool) (formula.Type, error) {
	if stack == nil {
		stack = make(map[string]bool)
	}
	return formula.Infer(ast, func(fieldID string) (formula.Type, error) {
		field, ok := table.byID[fieldID]
		if !ok {
			return formula.Null, errors.New("Referenced field is unavailable")
		}
		if field.formula != nil {
			if fieldID == current || stack[fieldID] {
				return formula.Null, errors.New("Formula dependency cycle")
			}
			stack[fieldID] = true
			result, err := inferFormulaType(field.formula.options.AST, table, current, stack)
			delete(stack, fieldID)
			return result, err
		}
		return formulaFieldType(field), nil
	})
}

func formulaFieldType(field rowField) formula.Type {
	if field.formula != nil {
		return field.formula.options.ResultType
	}
	if field.computed != nil {
		switch field.kind {
		case "lookup":
			return formula.Array
		case "rollup":
			switch field.computed.aggregate {
			case "count", "sum", "avg":
				return formula.Number
			case "concat":
				return formula.Text
			default:
				return formulaCodecType(field.computed.targetKind)
			}
		}
	}
	if field.relation != nil {
		return formula.Array
	}
	return formulaCodecType(field.kind)
}

func formulaCodecType(kind codec.Kind) formula.Type {
	switch kind {
	case codec.Integer, codec.Decimal, codec.Float, codec.Currency, codec.Percent,
		codec.Rating, codec.AutoNumber, codec.RowID:
		return formula.Number
	case codec.Checkbox:
		return formula.Boolean
	case codec.Date:
		return formula.Date
	case codec.DateTime, codec.CreatedAt, codec.UpdatedAt:
		return formula.DateTime
	case codec.MultiSelect, codec.TextArray, codec.IntArray, codec.MultiUser:
		return formula.Array
	case codec.JSON, codec.Attachment:
		return formula.Array
	case codec.Duration:
		return formula.Duration
	case codec.Text, codec.LongText, codec.SingleSelect, codec.Email, codec.URL, codec.Phone, codec.UUID,
		codec.Inet, codec.CIDR, codec.User, codec.Time, codec.CreatedBy, codec.UpdatedBy:
		return formula.Text
	default:
		return formula.Null
	}
}

func validateFormulaGraph(ctx context.Context, tx pgx.Tx, table rowTable, candidateID string, candidate *formula.Node) error {
	graph := make(map[string][]string)
	for _, field := range table.fields {
		if field.formula != nil {
			graph[field.id] = field.formula.options.FieldIDs
		}
	}
	if candidateID != "" {
		graph[candidateID] = formula.FieldIDs(candidate)
	}
	var visit func(string, int, map[string]bool) error
	visit = func(id string, depth int, visiting map[string]bool) error {
		if id == candidateID && candidateID != "" && visiting[id] {
			return validation("Formula dependency cycle")
		}
		dependencies, isFormula := graph[id]
		if !isFormula {
			return nil
		}
		depth++
		if depth > 8 {
			return validation("Formula dependency depth cannot exceed 8")
		}
		if visiting[id] {
			return validation("Formula dependency cycle")
		}
		next := make(map[string]bool, len(visiting)+1)
		for key, value := range visiting {
			next[key] = value
		}
		next[id] = true
		for _, dependency := range dependencies {
			if err := visit(dependency, depth, next); err != nil {
				return err
			}
		}
		return nil
	}
	for _, id := range formula.FieldIDs(candidate) {
		if id == candidateID && candidateID != "" {
			return validation("Formula dependency cycle")
		}
		if err := visit(id, 1, map[string]bool{}); err != nil {
			return err
		}
	}
	_ = ctx
	_ = tx
	return nil
}

func (s *Service) CheckFormula(ctx context.Context, actor Actor, tableID, expression string) (FormulaCheckResult, error) {
	target, err := resourceOf(acl.Table, tableID)
	if err != nil {
		return FormulaCheckResult{}, err
	}
	var result FormulaCheckResult
	err = s.withReadableTable(ctx, actor, tableID, func(tx pgx.Tx, table rowTable) error {
		user, ok := acl.ParseUUID(actor.ID)
		if !ok {
			return acl.ErrValidation()
		}
		if err := s.authorizeRows(ctx, tx, user, target, acl.Owner); err != nil {
			return err
		}
		raw, _ := json.Marshal(map[string]string{"expr": expression})
		_, options, err := cleanFormulaOptions(raw, table, "", tx, ctx)
		if err != nil {
			var apiError *apperr.Error
			if errors.As(err, &apiError) && apiError.Details != nil {
				position, _ := apiError.Details["position"].(int)
				result.Position = position
				result.Message = apiError.Message
				return nil
			}
			return err
		}
		result.Type = options.ResultType
		return nil
	})
	return result, err
}

func enrichRowFormulas(ctx context.Context, tx pgx.Tx, actor Actor, table rowTable, fields []rowField, rows []RowRecord) error {
	if len(rows) == 0 {
		return nil
	}
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return acl.ErrValidation()
	}
	ids := make([]int64, len(rows))
	byID := make(map[int64]int, len(rows))
	for index, row := range rows {
		id, err := parsePathRowID(row.ID)
		if err != nil {
			return err
		}
		ids[index] = id
		byID[id] = index
	}
	for _, field := range fields {
		if field.formula == nil {
			continue
		}
		stack := make(map[string]bool)
		stack[field.id] = true
		timezone := actor.Timezone
		if timezone == "" {
			timezone = "Asia/Shanghai"
		}
		value, err := formula.Compile(field.formula.options.AST, func(fieldID string, compiler *formula.Compiler) (formula.SQLValue, error) {
			referenced, found := table.byID[fieldID]
			if !found {
				return formula.SQLValue{}, errors.New("formula references a missing field")
			}
			if referenced.formula != nil {
				if stack[fieldID] || len(stack) >= 8 {
					return formula.SQLValue{}, errors.New("formula dependency cycle or depth exceeded")
				}
				stack[fieldID] = true
				nested, err := compiler.CompileChild(referenced.formula.options.AST)
				delete(stack, fieldID)
				return nested, err
			}
			if referenced.computed != nil {
				access, err := acl.AccessUncached(ctx, tx, user, acl.Resource{Type: acl.Table, ID: referenced.computed.targetTable})
				if err != nil {
					return formula.SQLValue{}, fmt.Errorf("check formula dependency access: %w", err)
				}
				if !access.Visible || access.Check(acl.Read) != nil {
					return formula.SQLValue{SQL: "NULL", Type: formulaFieldType(referenced), Masked: true}, nil
				}
				expression, err := computedExpression(table, referenced)
				if err != nil {
					return formula.SQLValue{}, err
				}
				if referenced.kind == "rollup" {
					expression = "(" + expression + ` #>> '{}' )`
					switch referenced.computed.aggregate {
					case "count", "sum", "avg":
						expression = "(" + expression + ")::numeric"
					default:
						switch referenced.computed.targetKind {
						case codec.Integer, codec.Decimal, codec.Float, codec.Currency, codec.Percent, codec.Rating:
							expression = "(" + expression + ")::numeric"
						case codec.Date:
							expression = "(" + expression + ")::date"
						case codec.DateTime:
							expression = "(" + expression + ")::timestamptz"
						}
					}
				}
				return formula.SQLValue{SQL: expression, Type: formulaFieldType(referenced)}, nil
			}
			if referenced.relation != nil {
				return formula.SQLValue{}, errors.New("formula cannot directly reference a link field")
			}
			if err := ddl.ValidateIdentifier(ddl.Column, referenced.column); err != nil {
				return formula.SQLValue{}, err
			}
			return formula.SQLValue{
				SQL: pgx.Identifier{"source", referenced.column}.Sanitize(), Type: formulaFieldType(referenced),
			}, nil
		}, timezone)
		if err != nil {
			return fmt.Errorf("compile formula field %s: %w", field.id, err)
		}
		if value.Masked {
			for index := range rows {
				rows[index].Fields[field.id] = json.RawMessage(`"🔒"`)
			}
			continue
		}
		if err := ddl.ValidateIdentifier(ddl.Book, table.schema); err != nil {
			return err
		}
		if err := ddl.ValidateIdentifier(ddl.Table, table.name); err != nil {
			return err
		}
		rowIDArg := len(value.Args) + 1
		args := append(value.Args, ids)
		source := pgx.Identifier{table.schema, table.name}.Sanitize()
		formulaSQL := value.SQL
		if value.Type == formula.Null {
			formulaSQL = "NULL::text"
		}
		query := `SELECT "source"."id", COALESCE(to_jsonb(` + formulaSQL + `), 'null'::jsonb) FROM ` + source +
			` AS "source" WHERE "source"."id" = ANY($` + fmt.Sprint(rowIDArg) + `::bigint[])`
		result, err := tx.Query(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("query formula field: %w", err)
		}
		for result.Next() {
			var id int64
			var raw []byte
			if err := result.Scan(&id, &raw); err != nil {
				result.Close()
				return fmt.Errorf("scan formula value: %w", err)
			}
			index, found := byID[id]
			if !found || !json.Valid(raw) {
				result.Close()
				return errors.New("formula query returned an invalid row or value")
			}
			rows[index].Fields[field.id] = append(json.RawMessage(nil), raw...)
		}
		if err := result.Err(); err != nil {
			result.Close()
			return fmt.Errorf("iterate formula values: %w", err)
		}
		result.Close()
	}
	return nil
}

func formulaFieldUses(ctx context.Context, tx pgx.Tx, fieldIDs []pgtype.UUID) (bool, error) {
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
		WHERE deleted_at IS NULL AND ui_type='formula'
		AND options->'fieldIds' ?| $1::text[]
	)`, ids).Scan(&used)
	if err != nil {
		return false, fmt.Errorf("check formula dependencies: %w", err)
	}
	return used, nil
}
