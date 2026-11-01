package structure

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"pandatable/internal/codec"
)

func packageViewTestTable(t *testing.T) (rowTable, map[string]bool) {
	t.Helper()
	rowID, err := codec.Bind(codec.RowID, nil)
	if err != nil {
		t.Fatal(err)
	}
	text, err := codec.Bind(codec.Text, nil)
	if err != nil {
		t.Fatal(err)
	}
	fields := []rowField{
		{id: "row-id", kind: codec.RowID, column: "id", system: true, codec: rowID},
		{id: "name", kind: codec.Text, column: "c_aaaaaaaaaa", codec: text},
		{id: "missing", kind: "rollup", error: "dependency_missing"},
	}
	table := rowTable{fields: fields, byID: make(map[string]rowField, len(fields))}
	for _, field := range fields {
		table.byID[field.id] = field
	}
	return table, map[string]bool{"missing": true}
}

func TestPackageViewValidationPreservesUnavailableConfiguration(t *testing.T) {
	table, unavailable := packageViewTestTable(t)
	config := ViewColumns{
		Fields:    []ViewColumn{{ID: "name", Visible: true}, {ID: "missing", Visible: true}},
		Filter:    json.RawMessage(`{"op":"and","items":[{"field":"missing","cmp":"gte","value":1},{"field":"name","cmp":"contains","value":"open"}]}`),
		FilterSet: true,
		Sort:      []ViewSort{{Field: "missing", Dir: "desc"}},
		GroupBy:   []string{"missing"},
		Highlight: []HighlightRule{{
			ID: "rule", When: &FilterNode{Field: "missing", Cmp: codec.OpGte, Value: json.RawMessage(`1`)},
			Target: HighlightTarget{Type: "cells", Fields: []string{"name"}}, Color: "amber-3",
		}},
	}
	original, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	checked, affected, err := packageViewValidationConfig(config, table, unavailable)
	if err != nil || !affected {
		t.Fatalf("validation copy = %+v, affected %t, %#v", checked, affected, err)
	}
	if _, _, err := normalizeManagedViewConfig(context.Background(), nil, Actor{}, table, []byte(`{}`), checked); err != nil {
		t.Fatalf("validate masked configuration: %#v", err)
	}
	after, err := json.Marshal(config)
	if err != nil || !reflect.DeepEqual(after, original) {
		t.Fatalf("original view was changed: %s, %v", after, err)
	}
	if strings.Contains(string(checked.Filter), `"field":"missing"`) ||
		checked.Highlight[0].When.Field != "row-id" || config.Highlight[0].When.Field != "missing" {
		t.Fatalf("validation predicates were not isolated: %+v", checked)
	}
}

func TestPackageViewValidationReportsSortAndGroup(t *testing.T) {
	table, unavailable := packageViewTestTable(t)
	for _, config := range []ViewColumns{
		{Sort: []ViewSort{{Field: "missing", Dir: "asc"}}},
		{GroupBy: []string{"missing"}},
	} {
		_, affected, err := packageViewValidationConfig(config, table, unavailable)
		if err != nil || !affected {
			t.Errorf("affected view = %t, %#v", affected, err)
		}
	}
	_, affected, err := packageViewValidationConfig(ViewColumns{
		Sort: []ViewSort{{Field: "name", Dir: "asc"}},
	}, table, unavailable)
	if err != nil || affected {
		t.Fatalf("unaffected view = %t, %#v", affected, err)
	}
}

func TestPackageViewValidationRejectsInvalidMetadata(t *testing.T) {
	table, unavailable := packageViewTestTable(t)
	for _, sorts := range [][]ViewSort{
		{{Field: "missing", Dir: "invalid"}},
		{{Field: "missing", Dir: "asc"}, {Field: "missing", Dir: "desc"}},
		{{Field: "unknown", Dir: "asc"}},
	} {
		if _, _, err := packageViewValidationConfig(ViewColumns{Sort: sorts}, table, unavailable); err == nil {
			t.Errorf("invalid sort accepted: %+v", sorts)
		}
	}
	conditions := make([]string, maxFilterNodes)
	for index := range conditions {
		conditions[index] = `{"field":"missing","cmp":"gte","value":1}`
	}
	for _, raw := range []string{
		`{"field":"missing","cmp":"not_an_operator","value":1}`,
		`{"field":"missing","cmp":"gte","value":"not-a-number"}`,
		`{"field":"missing","cmp":"between","value":[1]}`,
		`{"field":"missing","value":1}`,
		`{"op":"and","items":[]}`,
		`{"field":"unknown","cmp":"is","value":"x"}`,
		`{"op":"and","items":[{"op":"and","items":[{"op":"and","items":[{"op":"and","items":[{"field":"missing","cmp":"gte","value":1}]}]}]}]}`,
		fmt.Sprintf(`{"op":"and","items":[%s]}`, strings.Join(conditions, ",")),
	} {
		config := ViewColumns{Filter: json.RawMessage(raw), FilterSet: true}
		if _, _, err := packageViewValidationConfig(config, table, unavailable); err == nil {
			t.Errorf("invalid filter accepted: %s", raw)
		}
	}
	if err := validatePackageViewFilter(json.RawMessage(
		`{"field":"missing","cmp":"gte","value":1,"extra":true}`)); err == nil {
		t.Fatal("unknown filter member was accepted")
	}
	config := ViewColumns{
		Fields:    []ViewColumn{{ID: "name", Visible: true}},
		Filter:    json.RawMessage(`{"op":"and","items":[{"field":"missing","cmp":"gte","value":1},{"field":"name","cmp":"gte","value":1}]}`),
		FilterSet: true,
	}
	checked, _, err := packageViewValidationConfig(config, table, unavailable)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := normalizeManagedViewConfig(context.Background(), nil, Actor{}, table, []byte(`{}`), checked); err == nil {
		t.Fatal("invalid unaffected condition was accepted")
	}
	checked.Filter = nil
	checked.FilterSet = false
	checked.Highlight = []HighlightRule{{
		ID: "rule", When: &FilterNode{Field: "row-id", Cmp: codec.OpGte, Value: json.RawMessage(`"1"`)},
		Target: HighlightTarget{Type: "row"}, Color: "invalid",
	}}
	if _, _, err := normalizeManagedViewConfig(context.Background(), nil, Actor{}, table, []byte(`{}`), checked); err == nil {
		t.Fatal("invalid highlight metadata was accepted")
	}
}

func TestUnavailablePackageConditionValidatesKnownSourceTypes(t *testing.T) {
	link := rowField{kind: codec.Text}
	missingFormula := rowField{kind: "formula", error: "dependency_missing"}
	for _, test := range []struct {
		field rowField
		node  FilterNode
		valid bool
	}{
		{link, FilterNode{Cmp: codec.OpHasAnyOf, Value: json.RawMessage(`["1"]`)}, true},
		{link, FilterNode{Cmp: codec.OpHasAnyOf, Value: json.RawMessage(`["invalid"]`)}, false},
		{link, FilterNode{Cmp: codec.OpContains, Value: json.RawMessage(`"text"`)}, false},
		{link, FilterNode{Cmp: codec.OpBetween, Value: json.RawMessage(`[0,2]`)}, true},
		{link, FilterNode{Cmp: codec.OpGte, Value: json.RawMessage(`-1`)}, false},
		{missingFormula, FilterNode{Cmp: codec.OpGte, Value: json.RawMessage(`1`)}, true},
		{missingFormula, FilterNode{Cmp: codec.OpIs, Value: json.RawMessage(`"text"`)}, true},
		{missingFormula, FilterNode{Cmp: codec.OpIsChecked, Value: json.RawMessage(`null`)}, true},
		{missingFormula, FilterNode{Cmp: codec.OpGte, Value: json.RawMessage(`"bad"`)}, false},
		{missingFormula, FilterNode{Cmp: codec.OpHasKey, Value: json.RawMessage(`"key"`)}, false},
	} {
		err := validateUnavailablePackageCondition(test.field, &test.node)
		if (err == nil) != test.valid {
			t.Errorf("%s %s valid = %t, error %#v", test.field.kind, test.node.Cmp, test.valid, err)
		}
	}
}

func TestFormConfigRejectsUnavailableField(t *testing.T) {
	id := "00000000-0000-0000-0000-000000000001"
	field := rowField{id: id, kind: "lookup", error: "dependency_missing"}
	table := rowTable{fields: []rowField{field}, byID: map[string]rowField{id: field}}
	if _, err := validateFormConfig(FormConfig{
		AfterSubmit: "message", Fields: []FormField{{ID: id}},
	}, table); err == nil {
		t.Fatal("unavailable form field was accepted")
	}
}

func TestPackageFilterOperatorsCoverCodecCapabilities(t *testing.T) {
	for _, kind := range codec.Kinds() {
		field, err := codec.Bind(kind, nil)
		if err != nil {
			t.Fatalf("bind %s defaults: %v", kind, err)
		}
		for _, op := range field.FilterOps() {
			if !packageFilterOperator(op) {
				t.Errorf("operator %s for %s is missing", op, kind)
			}
		}
	}
}
