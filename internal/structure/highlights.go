package structure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
)

const maxHighlightRules = 20

var validHighlightID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

var highlightColorTokens = map[string]bool{
	"red-2": true, "orange-2": true, "amber-3": true,
	"green-2": true, "blue-2": true, "purple-2": true,
}

type compiledHighlightRule struct {
	rule      HighlightRule
	condition string
}

func rowFieldIDs(table rowTable) map[string]bool {
	result := make(map[string]bool, len(table.fields))
	for _, field := range table.fields {
		result[field.id] = true
	}
	return result
}

func compileHighlightRules(
	ctx context.Context,
	tx pgx.Tx,
	actor Actor,
	table rowTable,
	rules []HighlightRule,
) (string, string, []any, error) {
	if len(rules) > maxHighlightRules {
		return "", "", nil, validation("A view can have at most 20 highlight rules")
	}

	compiled := make([]compiledHighlightRule, 0, len(rules))
	args := make([]any, 0)
	seenIDs := make(map[string]bool, len(rules))
	for _, rule := range rules {
		if !validHighlightID.MatchString(rule.ID) || seenIDs[rule.ID] {
			return "", "", nil, validation("Highlight rule ids must be unique and valid")
		}
		seenIDs[rule.ID] = true
		if !highlightColorTokens[rule.Color] {
			return "", "", nil, validation("Highlight color is not supported")
		}
		switch rule.Target.Type {
		case "row":
			if len(rule.Target.Fields) != 0 {
				return "", "", nil, validation("Row highlight targets cannot name fields")
			}
		case "cells":
			if len(rule.Target.Fields) == 0 {
				return "", "", nil, validation("Cell highlight targets require fields")
			}
			seenFields := make(map[string]bool, len(rule.Target.Fields))
			for _, id := range rule.Target.Fields {
				if _, ok := table.byID[id]; !ok || seenFields[id] {
					return "", "", nil, validation("Cell highlight fields must be unique live fields")
				}
				seenFields[id] = true
			}
		default:
			return "", "", nil, validation("Highlight target type is not supported")
		}
		condition, conditionArgs, err := compileRowFilter(ctx, tx, actor, table, rule.When)
		if err != nil {
			return "", "", nil, err
		}
		compiled = append(compiled, compiledHighlightRule{
			rule: rule, condition: shiftSQLArguments(condition, len(args)),
		})
		args = append(args, conditionArgs...)
	}

	rowParts := make([]string, 0, len(compiled))
	for _, item := range compiled {
		if item.rule.Target.Type != "row" {
			continue
		}
		color := fmt.Sprintf("$%d::text", len(args)+1)
		args = append(args, item.rule.Color)
		rowParts = append(rowParts, "WHEN "+item.condition+" THEN "+color)
	}
	rowSQL := `NULL::text AS "_hl_row"`
	if len(rowParts) > 0 {
		rowSQL = "CASE " + strings.Join(rowParts, " ") + ` ELSE NULL::text END AS "_hl_row"`
	}

	cellFields := make([]string, 0)
	seenCellFields := make(map[string]bool)
	for _, item := range compiled {
		if item.rule.Target.Type != "cells" {
			continue
		}
		for _, id := range item.rule.Target.Fields {
			if seenCellFields[id] {
				continue
			}
			seenCellFields[id] = true
			cellFields = append(cellFields, id)
		}
	}
	cellParts := make([]string, 0, len(cellFields))
	for _, fieldID := range cellFields {
		key := fmt.Sprintf("$%d", len(args)+1)
		args = append(args, fieldID)
		branches := make([]string, 0)
		for _, item := range compiled {
			if item.rule.Target.Type != "cells" || !containsString(item.rule.Target.Fields, fieldID) {
				continue
			}
			color := fmt.Sprintf("$%d::text", len(args)+1)
			args = append(args, item.rule.Color)
			branches = append(branches, "WHEN "+item.condition+" THEN "+color)
		}
		value := "NULL::text"
		if len(branches) > 0 {
			value = "CASE " + strings.Join(branches, " ") + " ELSE NULL::text END"
		}
		cellParts = append(cellParts, key+"::text, "+value)
	}
	cellSQL := `'{}'::jsonb::text AS "_hl_cells"`
	if len(cellParts) > 0 {
		cellSQL = "jsonb_strip_nulls(jsonb_build_object(" + strings.Join(cellParts, ", ") +
			`))::text AS "_hl_cells"`
	}
	return rowSQL, cellSQL, args, nil
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func scanHighlightedRows(rows pgx.Rows, table rowTable, fields []rowField) ([]RowRecord, error) {
	defer rows.Close()
	physicalCount := len(physicalRowFields(fields)) + 1
	result := make([]RowRecord, 0)
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, fmt.Errorf("read highlighted row values: %w", err)
		}
		if len(values) != physicalCount+2 {
			return nil, errors.New("highlighted row scan returned an unexpected column count")
		}
		record, err := table.encode(values[:physicalCount], fields)
		if err != nil {
			return nil, err
		}
		if value := values[physicalCount]; value != nil {
			color, ok := value.(string)
			if bytes, byteValue := value.([]byte); byteValue {
				color = string(bytes)
				ok = true
			}
			if !ok {
				return nil, errors.New("row highlight returned an unexpected value")
			}
			record.HighlightRow = &color
		}
		cells, err := jsonMapString(values[physicalCount+1])
		if err != nil {
			return nil, fmt.Errorf("decode cell highlights: %w", err)
		}
		if len(cells) > 0 {
			record.HighlightCells = cells
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate highlighted rows: %w", err)
	}
	return result, nil
}

func jsonMapString(value any) (map[string]string, error) {
	switch raw := value.(type) {
	case []byte:
		var result map[string]string
		err := json.Unmarshal(raw, &result)
		return result, err
	case string:
		var result map[string]string
		err := json.Unmarshal([]byte(raw), &result)
		return result, err
	default:
		return nil, errors.New("cell highlights returned an unexpected value")
	}
}
