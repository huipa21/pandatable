package structure

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"pandatable/internal/codec"
	"pandatable/internal/formula"
)

func validatePackageViewFilter(raw json.RawMessage) error {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	var filter FilterNode
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&filter); err != nil {
		return validation("The package view filter is invalid")
	}
	return nil
}

func packageUnavailableViewFields(table rowTable, manifest *tablePackageManifest, keys map[string]string) map[string]bool {
	unavailable := make(map[string]bool)
	for _, field := range table.fields {
		if field.error != "" {
			unavailable[field.id] = true
		}
	}
	for _, source := range manifest.Fields {
		field, ok := table.byID[keys[source.Key]]
		if source.UIType == "link" && ok && field.kind == codec.Text {
			unavailable[field.id] = true
		}
	}
	return unavailable
}

func packageViewValidationConfig(config ViewColumns, table rowTable, unavailable map[string]bool) (ViewColumns, bool, error) {
	seenSort := make(map[string]bool, len(config.Sort))
	for _, sort := range config.Sort {
		field, ok := table.byID[sort.Field]
		if !ok || field.relation != nil || seenSort[sort.Field] || sort.Dir != "asc" && sort.Dir != "desc" {
			return ViewColumns{}, false, validation("The package view sort is invalid")
		}
		seenSort[sort.Field] = true
	}
	if len(unavailable) == 0 {
		return config, false, nil
	}
	rowID := ""
	for _, field := range table.fields {
		if field.kind == codec.RowID && field.system {
			rowID = field.id
			break
		}
	}
	if rowID == "" {
		return ViewColumns{}, false, errors.New("package table has no row id field")
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return ViewColumns{}, false, fmt.Errorf("copy package view for validation: %w", err)
	}
	var checked ViewColumns
	if err := json.Unmarshal(raw, &checked); err != nil {
		return ViewColumns{}, false, fmt.Errorf("decode package view for validation: %w", err)
	}
	checked.FilterSet, checked.FormSet = config.FilterSet, config.FormSet
	affected := false
	if len(checked.Filter) > 0 && !bytes.Equal(bytes.TrimSpace(checked.Filter), []byte("null")) {
		var filter FilterNode
		if err := json.Unmarshal(checked.Filter, &filter); err != nil {
			return ViewColumns{}, false, validation("View filter is invalid")
		}
		count := 0
		changed, err := maskUnavailableFilter(&filter, table, unavailable, rowID, 1, &count)
		if err != nil {
			return ViewColumns{}, false, err
		}
		if changed {
			checked.Filter, err = json.Marshal(filter)
			if err != nil {
				return ViewColumns{}, false, fmt.Errorf("encode package view filter: %w", err)
			}
			affected = true
		}
	}
	for index := range checked.Highlight {
		count := 0
		changed, err := maskUnavailableFilter(checked.Highlight[index].When, table, unavailable, rowID, 1, &count)
		if err != nil {
			return ViewColumns{}, false, err
		}
		affected = affected || changed
	}
	for _, sort := range checked.Sort {
		affected = affected || unavailable[sort.Field]
	}
	for _, field := range checked.GroupBy {
		affected = affected || unavailable[field]
	}
	return checked, affected, nil
}

func maskUnavailableFilter(node *FilterNode, table rowTable, unavailable map[string]bool, rowID string,
	depth int, count *int,
) (bool, error) {
	if node == nil {
		return false, validation("Filter is invalid")
	}
	*count = *count + 1
	if *count > maxFilterNodes {
		return false, validation("Filter contains too many conditions")
	}
	if node.Op != "" {
		if depth > 3 || (node.Op != "and" && node.Op != "or") ||
			len(node.Items) == 0 || node.Field != "" || node.Cmp != "" || len(node.Value) != 0 {
			return false, validation("Filter group is invalid")
		}
		affected := false
		for index := range node.Items {
			changed, err := maskUnavailableFilter(&node.Items[index], table, unavailable, rowID, depth+1, count)
			if err != nil {
				return false, err
			}
			affected = affected || changed
		}
		return affected, nil
	}
	if len(node.Items) != 0 || node.Field == "" || node.Cmp == "" || len(node.Value) == 0 || !json.Valid(node.Value) {
		return false, validation("Filter condition is invalid")
	}
	field, ok := table.byID[node.Field]
	if !ok {
		return false, validation("Filter field is unavailable")
	}
	if unavailable[node.Field] {
		if err := validateUnavailablePackageCondition(field, node); err != nil {
			return false, err
		}
		// The caller stores the original predicate, not this validation-only substitute.
		*node = FilterNode{Field: rowID, Cmp: codec.OpGte, Value: json.RawMessage(`"1"`)}
		return true, nil
	}
	return false, nil
}

func validateUnavailablePackageCondition(field rowField, node *FilterNode) error {
	if !packageFilterOperator(node.Cmp) {
		return validation("Filter operator is not supported")
	}
	if field.error == "" {
		compiler := filterCompiler{}
		ops, err := compiler.fieldOps(rowField{relation: &relationRecord{}})
		if err != nil {
			return err
		}
		if !containsFilterOp(ops, node.Cmp) {
			return validation("Filter operator is not supported for a link")
		}
		switch node.Cmp {
		case codec.OpIsEmpty, codec.OpIsNotEmpty:
			return nil
		case codec.OpHasAnyOf:
			var ids []string
			if json.Unmarshal(node.Value, &ids) != nil || len(ids) == 0 || len(ids) > 500 {
				return validation("Link filter requires row ids")
			}
			for _, id := range ids {
				if _, err := canonicalRowID(id); err != nil {
					return validation("Link filter row id is invalid")
				}
			}
			return nil
		case codec.OpBetween:
			_, err := decodeCountPair(node.Value)
			return err
		default:
			_, err := decodeCount(node.Value)
			return err
		}
	}
	if field.kind == "formula" {
		for _, candidate := range []struct {
			result formula.Type
			kind   codec.Kind
		}{
			{formula.Number, codec.Decimal}, {formula.Text, codec.Text}, {formula.Boolean, codec.Checkbox},
			{formula.Date, codec.Date}, {formula.DateTime, codec.DateTime},
		} {
			compiler := filterCompiler{actor: Actor{Timezone: "UTC"}}
			testField := rowField{kind: "formula", formula: &formulaField{options: formulaOptions{ResultType: candidate.result}}}
			ops, err := compiler.fieldOps(testField)
			if err != nil {
				return err
			}
			if containsFilterOp(ops, node.Cmp) {
				if _, err := compiler.condition(testField, "value", candidate.kind, node.Cmp, node.Value); err == nil {
					return nil
				}
			}
		}
		return validation("Formula filter operator or value is invalid")
	}
	switch node.Cmp {
	case codec.OpEq, codec.OpNeq, codec.OpLt, codec.OpLte, codec.OpGt, codec.OpGte:
		_, err := decodeNumericFilter(node.Value)
		return err
	case codec.OpBetween:
		var values []json.RawMessage
		if json.Unmarshal(node.Value, &values) != nil || len(values) != 2 {
			return validation("Between requires exactly two values")
		}
	}
	return nil
}

func packageFilterOperator(op codec.Op) bool {
	switch op {
	case codec.OpIs, codec.OpIsNot, codec.OpContains, codec.OpNotContains, codec.OpStartsWith, codec.OpEndsWith,
		codec.OpIsEmpty, codec.OpIsNotEmpty, codec.OpEq, codec.OpNeq, codec.OpLt, codec.OpLte, codec.OpGt, codec.OpGte,
		codec.OpBetween, codec.OpBefore, codec.OpAfter, codec.OpOnOrBefore, codec.OpOnOrAfter, codec.OpIsWithin,
		codec.OpIsChecked, codec.OpIsNotChecked, codec.OpIsAnyOf, codec.OpIsNoneOf, codec.OpHasAnyOf,
		codec.OpHasAllOf, codec.OpHasNoneOf, codec.OpIsExactly, codec.OpHasKey, codec.OpPathEquals, codec.OpContainedIn:
		return true
	default:
		return false
	}
}
