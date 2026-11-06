package structure

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"pandatable/internal/codec"
)

func TestTextGroupKeyMatchesGenericPostgreSQL(t *testing.T) {
	f := newFixture(t)
	bound, err := codec.Bind(codec.Text, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	physical := rowField{kind: codec.Text, codec: bound}
	fast := canonicalGroupKey(physical, "value")
	generic := canonicalGroupKey(rowField{}, "value")
	if strings.Contains(fast, "jsonb_array_elements") || !strings.Contains(generic, "jsonb_array_elements") {
		t.Fatalf("unexpected scalar/fallback expressions: %s / %s", fast, generic)
	}
	for _, field := range []rowField{
		{kind: codec.Text}, {kind: codec.TextArray}, {kind: codec.JSON},
		{kind: codec.Text, codec: bound, computed: &computedField{}},
		{kind: codec.Text, codec: bound, formula: &formulaField{}},
		{kind: codec.Text, codec: bound, relation: &relationRecord{}},
	} {
		if got := canonicalGroupKey(field, "value"); got != generic {
			t.Fatalf("nonphysical or nonscalar field %s used scalar expression: %s", field.kind, got)
		}
	}
	values := []*string{nil}
	for _, value := range []string{"", "null", "[]", "[1,2]", `{"a":1}`, `a"b`, `a\b`, "text \u4e2d"} {
		values = append(values, &value)
	}
	for _, value := range values {
		var got, want string
		err := f.pool.QueryRow(f.ctx, "SELECT ("+fast+")::text, ("+generic+
			")::text FROM (SELECT $1::text AS value) AS input", value).Scan(&got, &want)
		if err != nil || got != want {
			t.Fatalf("value %v: scalar=%q generic=%q, err=%v", value, got, want, err)
		}
	}
}

func TestGroupingTextPathsAndArraySemanticsPostgreSQL(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	table := f.fieldTable(owner)
	text := table.create(FieldInput{Name: "Group", UIType: "text"})
	tags := table.create(FieldInput{Name: "Tags", UIType: "text_array"})
	physical, err := physicalTable(table.pgSchema, table.pgName)
	if err != nil {
		t.Fatal(err)
	}
	textColumn := pgx.Identifier{table.column(text)}.Sanitize()
	tagsColumn := pgx.Identifier{table.column(tags)}.Sanitize()
	f.exec(fmt.Sprintf(`INSERT INTO %s (%s, %s) VALUES
		(NULL, ARRAY['a','b']), (NULL, ARRAY['b','a']),
		('', ARRAY[]::text[]),
		('[]', ARRAY['b','a']), ('[]', ARRAY['a','b']),
		('null', NULL), ('a"b', NULL), ('a"b', ARRAY[]::text[]), ($1, NULL)`,
		physical, textColumn, tagsColumn), "text \u4e2d")

	view := table.schema.Views[0]
	aggregates := []ViewAggregate{
		{FieldID: text.ID, Aggregate: codec.AggCountEmpty},
		{FieldID: text.ID, Aggregate: codec.AggCountFilled},
		{FieldID: text.ID, Aggregate: codec.AggUnique},
		{FieldID: text.ID, Aggregate: codec.AggPercentEmpty},
		{FieldID: text.ID, Aggregate: codec.AggPercentFilled},
		{FieldID: text.ID, Aggregate: codec.AggPercentUnique},
	}
	_, err = f.service.UpdateViewColumns(f.ctx, owner, view.ID, view.Version, ViewColumns{
		Fields:  []ViewColumn{{ID: text.ID, Visible: true}, {ID: tags.ID, Visible: true}},
		GroupBy: []string{text.ID, tags.ID}, Aggregates: aggregates,
	})
	if err != nil {
		t.Fatal(err)
	}
	top, err := f.service.QueryGroups(f.ctx, owner, view.ID, RowGroupRequest{})
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]int64{"<NULL>": 2, "": 1, "[]": 2, "null": 1, `a"b`: 2, "text \u4e2d": 1}
	if len(top.Groups) != len(expected) {
		t.Fatalf("top groups = %+v", top.Groups)
	}
	for _, bucket := range top.Groups {
		var groupKey *string
		if err := json.Unmarshal(bucket.Key, &groupKey); err != nil {
			t.Fatal(err)
		}
		if groupKey == nil {
			want := []float64{2, 0, 0, 1, 0, 0}
			for index, aggregate := range bucket.Aggregates {
				var got float64
				if err := json.Unmarshal(aggregate.Value, &got); err != nil || got != want[index] {
					t.Errorf("null-group aggregate %s = %s, want numeric %v", aggregate.Aggregate, aggregate.Value, want[index])
				}
			}
		}
	}
	for _, bucket := range top.Groups {
		var key *string
		if err := json.Unmarshal(bucket.Key, &key); err != nil {
			t.Fatal(err)
		}
		value := "<NULL>"
		if key != nil {
			value = *key
		}
		if count, ok := expected[value]; !ok || count != bucket.Count {
			t.Fatalf("unexpected group %s: %d", bucket.Key, bucket.Count)
		}
		delete(expected, value)
	}
	if len(expected) != 0 {
		t.Fatalf("missing groups: %v", expected)
	}

	for _, test := range []struct {
		name, path, key string
		ids             []string
	}{
		{"null text", `null`, `["a","b"]`, []string{"1", "2"}},
		{"array-like text", `"[]"`, `["a","b"]`, []string{"4", "5"}},
		{"escaped text", `"a\"b"`, `null`, []string{"7", "8"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := []json.RawMessage{json.RawMessage(test.path)}
			nested, err := f.service.QueryGroups(f.ctx, owner, view.ID, RowGroupRequest{Path: path})
			if err != nil || len(nested.Groups) != 1 || nested.Groups[0].Count != 2 {
				t.Fatalf("nested group = %+v, %v", nested, err)
			}
			var compact bytes.Buffer
			if err := json.Compact(&compact, nested.Groups[0].Key); err != nil || compact.String() != test.key {
				t.Fatalf("nested group key = %s, want %s: %v", nested.Groups[0].Key, test.key, err)
			}
			path = append(path, json.RawMessage(test.key))
			rows, err := f.service.QueryRows(f.ctx, owner, view.ID, RowQueryRequest{GroupPath: path})
			if err != nil || rows.Total == nil || *rows.Total != 2 {
				t.Fatalf("group path rows = %+v, %v", rows, err)
			}
			ids := make([]string, len(rows.Rows))
			for index, row := range rows.Rows {
				ids[index] = row.ID
			}
			if !reflect.DeepEqual(ids, test.ids) {
				t.Fatalf("group path ids = %v, want %v", ids, test.ids)
			}
		})
	}

	footer, err := f.service.QueryAggregates(f.ctx, owner, view.ID, RowAggregateRequest{})
	if err != nil || len(footer.Aggregates) != len(aggregates) {
		t.Fatalf("footer aggregates = %+v, %v", footer, err)
	}
	for index, want := range []string{
		"2", "7", "5", `"0.22222222222222222222"`, `"0.77777777777777777778"`, `"0.55555555555555555556"`,
	} {
		got := footer.Aggregates[index]
		if got.FieldID != text.ID || got.Aggregate != aggregates[index].Aggregate || string(got.Value) != want {
			t.Fatalf("footer aggregate %d = %+v, want %s", index, got, want)
		}
	}

	outsider := f.user("user", "active", "en")
	_, err = f.service.QueryGroups(f.ctx, outsider, view.ID, RowGroupRequest{})
	expectError(t, err, 404, "ERR_NOT_FOUND")
	_, err = f.service.QueryRows(f.ctx, outsider, view.ID, RowQueryRequest{GroupPath: []json.RawMessage{json.RawMessage(`null`)}})
	expectError(t, err, 404, "ERR_NOT_FOUND")
	_, err = f.service.QueryAggregates(f.ctx, outsider, view.ID, RowAggregateRequest{})
	expectError(t, err, 404, "ERR_NOT_FOUND")
}
