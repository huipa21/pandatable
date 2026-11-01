package structure

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"pandatable/internal/codec"
	"pandatable/internal/formula"
)

func TestPackageFormulaOptionsWithMissingDependency(t *testing.T) {
	bound, err := codec.Bind(codec.Integer, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	missing := rowField{id: "missing", name: "Missing", kind: "rollup", error: "dependency_missing"}
	known := rowField{id: "known", name: "Known", kind: codec.Integer, codec: bound}
	table := rowTable{
		fields: []rowField{missing, known},
		byID:   map[string]rowField{missing.id: missing, known.id: known},
	}
	encoded, err := packageFormulaOptions(context.Background(), nil,
		json.RawMessage(`{"expr":"{Missing} + {Known}"}`), table, nil)
	if err != nil {
		t.Fatal(err)
	}
	var options formulaOptions
	if err := json.Unmarshal(encoded, &options); err != nil {
		t.Fatal(err)
	}
	if options.Expression != "{Missing} + {Known}" || options.AST == nil ||
		!reflect.DeepEqual(options.FieldIDs, []string{"missing", "known"}) ||
		options.ResultType != formula.Null || options.Error != "dependency_missing" {
		t.Fatalf("missing formula options = %+v", options)
	}

	encoded, err = packageFormulaOptions(context.Background(), nil,
		json.RawMessage(`{"expr":"{Known} + 2"}`), table, nil)
	options = formulaOptions{}
	if err != nil || json.Unmarshal(encoded, &options) != nil ||
		options.ResultType != formula.Number || options.Error != "" {
		t.Fatalf("normal formula options = %s, %v", encoded, err)
	}

	for _, expression := range []string{
		`{"expr":"{Missing} + {Unknown}"}`,
		`{"expr":"{Missing} +"}`,
		`{"expr":"{Known} + \"invalid\""}`,
		`{"expr":"{Missing} + \"invalid\""}`,
		`{"expr":"UNKNOWN({Missing})"}`,
		`{"expr":"REPLACE({Missing}, 1, 2)"}`,
	} {
		if _, err := packageFormulaOptions(context.Background(), nil, json.RawMessage(expression), table, nil); err == nil {
			t.Errorf("invalid formula accepted: %s", expression)
		}
	}
}

func TestPackageFormulaOptionsWithDifferentMissingTypes(t *testing.T) {
	first := rowField{id: "first", name: "First", kind: "lookup", error: "dependency_missing"}
	second := rowField{id: "second", name: "Second", kind: "rollup", error: "dependency_missing"}
	table := rowTable{
		fields: []rowField{first, second},
		byID:   map[string]rowField{first.id: first, second.id: second},
	}
	raw := json.RawMessage(`{"expr":"IF({First} = \"yes\", {Second} * 2, 0)"}`)
	encoded, err := packageFormulaOptions(context.Background(), nil, raw, table, nil)
	if err != nil {
		t.Fatal(err)
	}
	var options formulaOptions
	if err := json.Unmarshal(encoded, &options); err != nil ||
		options.Error != "dependency_missing" || !reflect.DeepEqual(options.FieldIDs, []string{"first", "second"}) {
		t.Fatalf("formula options = %s, %v", encoded, err)
	}
}

func TestPackageFormulaOptionsWithSeveralMissingTypes(t *testing.T) {
	table := rowTable{byID: make(map[string]rowField)}
	for _, name := range []string{"First", "Second", "Third", "Fourth", "Fifth"} {
		field := rowField{id: name, name: name, kind: "rollup", error: "dependency_missing"}
		table.fields = append(table.fields, field)
		table.byID[field.id] = field
	}
	raw := json.RawMessage(`{"expr":"IF({First} = \"yes\", {Second} + {Third} + {Fourth} + {Fifth}, 0)"}`)
	if _, err := packageFormulaOptions(context.Background(), nil, raw, table, nil); err != nil {
		t.Fatalf("valid mixed-type formula was rejected: %#v", err)
	}
}

func TestPackageFormulaOptionsRejectsCyclesWithMissingDependency(t *testing.T) {
	missing := rowField{id: "missing", name: "Missing", kind: "lookup", error: "dependency_missing"}
	first := rowField{id: "first", name: "First", kind: "formula", formula: &formulaField{options: formulaOptions{
		AST: &formula.Node{Kind: "field", FieldID: "second"}, FieldIDs: []string{"second"}, ResultType: formula.Number,
	}}}
	second := rowField{id: "second", name: "Second", kind: "formula", formula: &formulaField{options: formulaOptions{
		AST: &formula.Node{Kind: "field", FieldID: "first"}, FieldIDs: []string{"first"}, ResultType: formula.Number,
	}}}
	table := rowTable{
		fields: []rowField{missing, first, second},
		byID:   map[string]rowField{missing.id: missing, first.id: first, second.id: second},
	}
	if _, err := packageFormulaOptions(context.Background(), nil,
		json.RawMessage(`{"expr":"{Missing} + {First}"}`), table, nil); err == nil {
		t.Fatal("cyclic formula graph was accepted")
	}
}

func TestPackageFormulaRejectsConvertedLinkReferences(t *testing.T) {
	bound, err := codec.Bind(codec.Text, nil)
	if err != nil {
		t.Fatal(err)
	}
	link := rowField{id: "link", name: "Link", kind: codec.Text, codec: bound}
	missing := rowField{id: "missing", name: "Missing", kind: "lookup", error: "dependency_missing"}
	table := rowTable{
		fields: []rowField{link, missing},
		byID:   map[string]rowField{link.id: link, missing.id: missing},
	}
	for _, raw := range []string{
		`{"expr":"{Link} & \"text\""}`,
		`{"expr":"{Link} & {Missing}"}`,
	} {
		if _, err := packageFormulaOptions(context.Background(), nil, json.RawMessage(raw), table,
			map[string]bool{"link": true}); err == nil {
			t.Errorf("direct reference to converted link was accepted: %s", raw)
		}
	}
}
