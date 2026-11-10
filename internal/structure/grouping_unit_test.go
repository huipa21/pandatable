package structure

import (
	"reflect"
	"testing"
)

func TestQueryGroupFieldsValidatesTransientOverrides(t *testing.T) {
	table := rowTable{byID: map[string]rowField{
		"a":    {id: "a"},
		"b":    {id: "b"},
		"link": {id: "link", relation: &relationRecord{}},
	}}
	config := rowViewConfig{GroupBy: []string{"a"}}
	got, err := queryGroupFields("grid", config, table, nil)
	if err != nil || !reflect.DeepEqual(got, []string{"a"}) {
		t.Fatalf("saved grouping = %v, %v", got, err)
	}
	override := []string{"b"}
	got, err = queryGroupFields("grid", config, table, &override)
	if err != nil || !reflect.DeepEqual(got, override) {
		t.Fatalf("override grouping = %v, %v", got, err)
	}
	for _, invalid := range [][]string{
		{"missing"}, {"a", "a"}, {"link"}, {"a", "b", "a", "b"},
	} {
		if _, err := queryGroupFields("grid", config, table, &invalid); err == nil {
			t.Errorf("accepted invalid grouping %v", invalid)
		}
	}
	if _, err := queryGroupFields("kanban", config, table, &override); err == nil {
		t.Error("accepted a grouping override for a non-grid view")
	}
}
