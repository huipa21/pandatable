package structure

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"pandatable/internal/codec"
)

func TestCompileHighlightRulesPreservesFirstMatchOrderPerTarget(t *testing.T) {
	bound, err := codec.Bind(codec.Text, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	field := rowField{id: "status-id", kind: codec.Text, column: "c_aaaaaaaaaa", codec: bound}
	table := rowTable{
		fields: []rowField{field},
		byID:   map[string]rowField{field.id: field},
	}
	condition := func() *FilterNode {
		return &FilterNode{
			Field: field.id, Cmp: codec.OpIs, Value: json.RawMessage(`"Overdue"`),
		}
	}
	rules := []HighlightRule{
		{ID: "row-red", When: condition(), Target: HighlightTarget{Type: "row"}, Color: "red-2"},
		{ID: "cell-amber", When: condition(), Target: HighlightTarget{
			Type: "cells", Fields: []string{field.id},
		}, Color: "amber-3"},
		{ID: "row-blue", When: condition(), Target: HighlightTarget{Type: "row"}, Color: "blue-2"},
	}
	rowSQL, cellsSQL, args, err := compileHighlightRules(
		context.Background(), nil, Actor{}, table, rules,
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(rowSQL, "THEN $4") > strings.Index(rowSQL, "THEN $5") ||
		!strings.Contains(rowSQL, `AS "_hl_row"`) ||
		!strings.Contains(cellsSQL, `AS "_hl_cells"`) {
		t.Fatalf("compiled highlight SQL lost rule order or aliases: %s / %s", rowSQL, cellsSQL)
	}
	if len(args) != 7 || args[3] != "red-2" || args[4] != "blue-2" ||
		args[5] != field.id || args[6] != "amber-3" {
		t.Fatalf("highlight arguments = %#v", args)
	}
}

func TestCompileHighlightRulesRejectsInvalidRules(t *testing.T) {
	bound, err := codec.Bind(codec.Text, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	field := rowField{id: "status-id", kind: codec.Text, column: "c_aaaaaaaaaa", codec: bound}
	table := rowTable{fields: []rowField{field}, byID: map[string]rowField{field.id: field}}
	valid := HighlightRule{
		ID: "rule-1",
		When: &FilterNode{
			Field: field.id, Cmp: codec.OpIs, Value: json.RawMessage(`"Overdue"`),
		},
		Target: HighlightTarget{Type: "row"}, Color: "red-2",
	}
	invalid := []HighlightRule{
		{ID: "rule-2", When: valid.When, Target: valid.Target, Color: "invalid"},
		{ID: "rule-2", When: valid.When, Target: HighlightTarget{Type: "cells"}, Color: "red-2"},
		{ID: "rule-2", When: valid.When, Target: HighlightTarget{Type: "row", Fields: []string{field.id}}, Color: "red-2"},
	}
	for _, rule := range invalid {
		if _, _, _, err := compileHighlightRules(context.Background(), nil, Actor{}, table, []HighlightRule{rule}); err == nil {
			t.Fatalf("invalid highlight rule was accepted: %+v", rule)
		}
	}
	if _, _, _, err := compileHighlightRules(
		context.Background(), nil, Actor{}, table, []HighlightRule{valid, valid},
	); err == nil {
		t.Fatal("duplicate highlight ids were accepted")
	}
	if _, _, _, err := compileHighlightRules(
		context.Background(), nil, Actor{}, table,
		make([]HighlightRule, maxHighlightRules+1),
	); err == nil {
		t.Fatal("too many highlight rules were accepted")
	}
}
