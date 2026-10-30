package structure

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"pandatable/internal/codec"
)

const maxQuickSearchLength = 256

func validateQuickSearch(search *string) error {
	if search != nil && utf8.RuneCountInString(*search) > maxQuickSearchLength {
		return validation("Quick search exceeds the supported length")
	}
	return nil
}

func searchableQuickSearchKind(kind codec.Kind) bool {
	switch kind {
	case codec.Text, codec.LongText, codec.SingleSelect, codec.MultiSelect,
		codec.Email, codec.URL, codec.Phone,
		codec.Integer, codec.Decimal, codec.Float, codec.Currency, codec.Percent,
		codec.Rating, codec.AutoNumber, codec.Duration,
		codec.Date, codec.Time, codec.DateTime, codec.CreatedAt, codec.UpdatedAt:
		return true
	default:
		return false
	}
}

func compileQuickSearch(
	ctx context.Context,
	tx pgx.Tx,
	actor Actor,
	table rowTable,
	fields []rowField,
	search *string,
) (string, []any, error) {
	if search == nil || *search == "" {
		return "", nil, nil
	}
	compiler := filterCompiler{ctx: ctx, tx: tx, actor: actor, table: table}
	clauses := make([]string, 0, len(fields))
	pattern := "%" + escapeLike(*search) + "%"
	for _, field := range fields {
		if field.codec == nil || !searchableQuickSearchKind(field.kind) {
			continue
		}
		expression, _, err := compiler.fieldExpression(field)
		if err != nil {
			return "", nil, err
		}
		clauses = append(clauses,
			"COALESCE(("+expression+")::text, '') ILIKE "+compiler.bind(pattern)+` ESCAPE '\'`)
	}
	if len(clauses) == 0 {
		return "FALSE", nil, nil
	}
	return "(" + strings.Join(clauses, " OR ") + ")", compiler.args, nil
}

func combineQueryConditions(
	filterSQL string,
	filterArgs []any,
	searchSQL string,
	searchArgs []any,
) (string, []any) {
	if searchSQL != "" {
		searchSQL = shiftSQLArguments(searchSQL, len(filterArgs))
		if filterSQL == "" {
			filterSQL = searchSQL
		} else {
			filterSQL += " AND " + searchSQL
		}
	}
	args := append([]any(nil), filterArgs...)
	args = append(args, searchArgs...)
	return filterSQL, args
}
