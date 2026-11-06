package httpapi

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"pandatable/internal/acl"
	"pandatable/internal/structure"
)

func TestQuickSearchAndFooterAggregatesUseVisibleFilteredRows(t *testing.T) {
	f := newACLFixture(t)
	owner, reader, stranger := f.user("user", false), f.user("user", false), f.user("user", false)
	workspace := f.newWorkspace(owner, "Search")
	book := f.newBook(owner, workspace.ID, "Book")
	table := f.newTable(owner, book.ID, "Records")
	f.grantOK(owner, acl.Table, table.Table.ID, "user", reader.id, acl.Read)

	label := f.newField(owner, table.Table.ID, `{"name":"Label","uiType":"text"}`)
	note := f.newField(owner, table.Table.ID, `{"name":"Note","uiType":"text"}`)
	amount := f.newField(owner, table.Table.ID, `{"name":"Amount","uiType":"integer"}`)
	rate := f.newField(owner, table.Table.ID, `{"name":"Rate","uiType":"integer"}`)
	counter := f.newField(owner, table.Table.ID, `{"name":"Counter","uiType":"text"}`)
	tags := f.newField(owner, table.Table.ID, `{"name":"Tags","uiType":"text_array"}`)
	active := f.newField(owner, table.Table.ID, `{"name":"Active","uiType":"checkbox"}`)
	private := f.newField(owner, table.Table.ID, `{"name":"Private","uiType":"text"}`)

	rows := []map[string]any{
		{label.ID: "Acme Alpha", note.ID: "one", amount.ID: 10, rate.ID: 2, counter.ID: "x", tags.ID: []string{"a"}, active.ID: false, private.ID: "private"},
		{label.ID: "Beta", note.ID: "项目 Beta", amount.ID: 0, rate.ID: 4, counter.ID: "y", tags.ID: []string{"a"}, active.ID: true, private.ID: "acme hidden"},
		{label.ID: "Gamma", note.ID: nil, amount.ID: 10, rate.ID: nil, counter.ID: "z", tags.ID: []string{"b"}, active.ID: false, private.ID: nil},
		{label.ID: "Literal %_ token", note.ID: nil, amount.ID: nil, rate.ID: nil, counter.ID: nil, tags.ID: nil, active.ID: false, private.ID: nil},
	}
	inserts := make([]map[string]any, len(rows))
	for index, fields := range rows {
		inserts[index] = map[string]any{"fields": fields}
	}
	body, err := json.Marshal(map[string]any{"rows": inserts})
	if err != nil {
		t.Fatal(err)
	}
	f.expectStatus(f.api(owner, "POST", "/tables/"+table.Table.ID+"/rows", string(body)), 201, "")

	aggregates := []map[string]string{
		{"fieldId": amount.ID, "aggregate": "sum"},
		{"fieldId": rate.ID, "aggregate": "avg"},
		{"fieldId": note.ID, "aggregate": "count_empty"},
		{"fieldId": label.ID, "aggregate": "unique"},
		{"fieldId": counter.ID, "aggregate": "count"},
		{"fieldId": amount.ID, "aggregate": "percent_empty"},
		{"fieldId": amount.ID, "aggregate": "percent_filled"},
		{"fieldId": amount.ID, "aggregate": "percent_unique"},
		{"fieldId": note.ID, "aggregate": "percent_empty"},
		{"fieldId": note.ID, "aggregate": "percent_filled"},
		{"fieldId": note.ID, "aggregate": "percent_unique"},
		{"fieldId": tags.ID, "aggregate": "percent_empty"},
		{"fieldId": tags.ID, "aggregate": "percent_filled"},
		{"fieldId": tags.ID, "aggregate": "percent_unique"},
		{"fieldId": active.ID, "aggregate": "percent_checked"},
	}
	columns := []map[string]any{
		{"id": label.ID, "visible": true},
		{"id": note.ID, "visible": true},
		{"id": amount.ID, "visible": true},
		{"id": rate.ID, "visible": true},
		{"id": counter.ID, "visible": true},
		{"id": tags.ID, "visible": true},
		{"id": active.ID, "visible": true},
		{"id": private.ID, "visible": false},
	}
	config, err := json.Marshal(map[string]any{"config": map[string]any{
		"fields": columns, "frozen": 0, "groupBy": []string{label.ID},
		"aggregates": aggregates,
	}})
	if err != nil {
		t.Fatal(err)
	}
	f.expectStatus(f.fieldCall(owner, "PATCH", "/views/"+table.Views[0].ID, string(config), `"1"`), 200, "")
	allRows := decode[structure.RowAggregateResult](f, f.api(reader, "POST",
		"/views/"+table.Views[0].ID+"/aggregates", `{}`), 200)
	allWant := map[string]string{
		amount.ID + ":percent_empty": `"0.25"`, amount.ID + ":percent_filled": `"0.75"`, amount.ID + ":percent_unique": `"0.5"`,
		note.ID + ":percent_empty": `"0.5"`, note.ID + ":percent_filled": `"0.5"`, note.ID + ":percent_unique": `"0.5"`,
		tags.ID + ":percent_empty": `"0.25"`, tags.ID + ":percent_filled": `"0.75"`, tags.ID + ":percent_unique": `"0.5"`,
		active.ID + ":percent_checked": `"0.25"`,
	}
	for _, aggregate := range allRows.Aggregates {
		key := aggregate.FieldID + ":" + string(aggregate.Aggregate)
		if want, ok := allWant[key]; ok {
			if got := string(aggregate.Value); got != want {
				t.Errorf("all-row aggregate %s = %s, want %s", key, got, want)
			}
			delete(allWant, key)
		}
	}
	if len(allWant) != 0 {
		t.Errorf("missing all-row percentages: %v", allWant)
	}

	acme := decode[structure.RowQueryResult](f, f.api(owner, "POST",
		"/views/"+table.Views[0].ID+"/query", `{"search":"acme"}`), 200)
	if len(acme.Rows) != 1 || string(acme.Rows[0].Fields[label.ID]) != `"Acme Alpha"` {
		t.Fatalf("visible text search should not match hidden fields: %+v", acme.Rows)
	}
	chinese := decode[structure.RowQueryResult](f, f.api(owner, "POST",
		"/views/"+table.Views[0].ID+"/query", `{"search":"项目"}`), 200)
	if len(chinese.Rows) != 1 || string(chinese.Rows[0].Fields[label.ID]) != `"Beta"` {
		t.Fatalf("Chinese text search = %+v", chinese.Rows)
	}
	literalWildcard := decode[structure.RowQueryResult](f, f.api(owner, "POST",
		"/views/"+table.Views[0].ID+"/query", `{"search":"%_"}`), 200)
	if len(literalWildcard.Rows) != 1 ||
		string(literalWildcard.Rows[0].Fields[label.ID]) != `"Literal %_ token"` {
		t.Fatalf("wildcard characters should be searched literally: %+v", literalWildcard.Rows)
	}
	groups := decode[structure.RowGroupResult](f, f.api(owner, "POST",
		"/views/"+table.Views[0].ID+"/groups", `{"path":[],"search":"acme"}`), 200)
	if len(groups.Groups) != 1 || string(groups.Groups[0].Key) != `"Acme Alpha"` {
		t.Fatalf("grouped search result = %+v", groups.Groups)
	}
	hidden := decode[structure.RowQueryResult](f, f.api(owner, "POST",
		"/views/"+table.Views[0].ID+"/query", `{"search":"acme hidden"}`), 200)
	if len(hidden.Rows) != 0 {
		t.Fatalf("search matched a hidden field: %+v", hidden.Rows)
	}

	filter := fmt.Sprintf(`{"field":%q,"cmp":"eq","value":10}`, amount.ID)
	filtered := decode[structure.RowAggregateResult](f, f.api(reader, "POST",
		"/views/"+table.Views[0].ID+"/aggregates",
		fmt.Sprintf(`{"filter":%s}`, filter)), 200)
	want := map[string]string{
		amount.ID + ":sum":             `"20"`,
		rate.ID + ":avg":               `"2"`,
		note.ID + ":count_empty":       "1",
		label.ID + ":unique":           "2",
		counter.ID + ":count":          "2",
		amount.ID + ":percent_empty":   `"0"`,
		amount.ID + ":percent_filled":  `"1"`,
		amount.ID + ":percent_unique":  `"0.5"`,
		note.ID + ":percent_empty":     `"0.5"`,
		note.ID + ":percent_filled":    `"0.5"`,
		note.ID + ":percent_unique":    `"0.5"`,
		tags.ID + ":percent_empty":     `"0"`,
		tags.ID + ":percent_filled":    `"1"`,
		tags.ID + ":percent_unique":    `"1"`,
		active.ID + ":percent_checked": `"0"`,
	}
	for _, aggregate := range filtered.Aggregates {
		key := aggregate.FieldID + ":" + string(aggregate.Aggregate)
		if got, ok := want[key]; !ok || string(aggregate.Value) != got {
			t.Errorf("filtered aggregate %s = %s, want %s", key, aggregate.Value, want[key])
		}
		delete(want, key)
	}
	if len(want) != 0 {
		t.Errorf("missing filtered aggregates: %v", want)
	}

	searchFiltered := decode[structure.RowAggregateResult](f, f.api(reader, "POST",
		"/views/"+table.Views[0].ID+"/aggregates",
		fmt.Sprintf(`{"search":"acme","filter":%s}`, filter)), 200)
	searchWant := map[string]string{
		amount.ID + ":sum":             `"10"`,
		rate.ID + ":avg":               `"2"`,
		note.ID + ":count_empty":       "0",
		label.ID + ":unique":           "1",
		counter.ID + ":count":          "1",
		amount.ID + ":percent_empty":   `"0"`,
		amount.ID + ":percent_filled":  `"1"`,
		amount.ID + ":percent_unique":  `"1"`,
		note.ID + ":percent_empty":     `"0"`,
		note.ID + ":percent_filled":    `"1"`,
		note.ID + ":percent_unique":    `"1"`,
		tags.ID + ":percent_empty":     `"0"`,
		tags.ID + ":percent_filled":    `"1"`,
		tags.ID + ":percent_unique":    `"1"`,
		active.ID + ":percent_checked": `"0"`,
	}
	for _, aggregate := range searchFiltered.Aggregates {
		key := aggregate.FieldID + ":" + string(aggregate.Aggregate)
		if got, ok := searchWant[key]; !ok || string(aggregate.Value) != got {
			t.Errorf("search aggregate %s = %s, want %s", key, aggregate.Value, searchWant[key])
		}
		delete(searchWant, key)
	}
	if len(searchWant) != 0 {
		t.Errorf("missing search aggregates: %v", searchWant)
	}
	nullAggregate := decode[structure.RowAggregateResult](f, f.api(reader, "POST",
		"/views/"+table.Views[0].ID+"/aggregates", `{"search":"Gamma"}`), 200)
	foundNullAverage := false
	for _, aggregate := range nullAggregate.Aggregates {
		if aggregate.FieldID == rate.ID && aggregate.Aggregate == "avg" {
			foundNullAverage = true
			if string(aggregate.Value) != "null" {
				t.Errorf("empty average = %s, want null", aggregate.Value)
			}
			break
		}
	}
	if !foundNullAverage {
		t.Error("empty average was missing")
	}
	emptyOnly := decode[structure.RowAggregateResult](f, f.api(reader, "POST",
		"/views/"+table.Views[0].ID+"/aggregates",
		fmt.Sprintf(`{"filter":{"field":%q,"cmp":"is_empty","value":null}}`, amount.ID)), 200)
	for _, aggregate := range emptyOnly.Aggregates {
		if aggregate.FieldID == amount.ID {
			want, ok := map[string]string{"percent_empty": `"1"`, "percent_filled": `"0"`, "percent_unique": `"0"`}[string(aggregate.Aggregate)]
			if ok && string(aggregate.Value) != want {
				t.Errorf("all-empty %s = %s, want %s", aggregate.Aggregate, aggregate.Value, want)
			}
		}
	}
	noRows := decode[structure.RowAggregateResult](f, f.api(reader, "POST",
		"/views/"+table.Views[0].ID+"/aggregates", `{"search":"no matching row"}`), 200)
	for _, aggregate := range noRows.Aggregates {
		if aggregate.Aggregate != "percent_checked" &&
			strings.HasPrefix(string(aggregate.Aggregate), "percent_") && string(aggregate.Value) != `"0"` {
			t.Errorf("empty %s = %s, want zero string", aggregate.Aggregate, aggregate.Value)
		}
	}
	f.expectStatus(f.api(stranger, "POST", "/views/"+table.Views[0].ID+"/aggregates", `{}`), 404, "ERR_NOT_FOUND")
	f.expectStatus(f.api(owner, "POST", "/views/"+table.Views[0].ID+"/aggregates",
		fmt.Sprintf(`{"aggregates":[{"fieldId":%q,"aggregate":"sum"}]}`, label.ID)), 400, "ERR_VALIDATION")
	unsupported := decode[structure.RowAggregateResult](f, f.api(owner, "POST",
		"/views/"+table.Views[0].ID+"/aggregates",
		fmt.Sprintf(`{"aggregates":[{"fieldId":%q,"aggregate":"percent_empty"}]}`, private.ID)), 200)
	if len(unsupported.Aggregates) != 0 {
		t.Errorf("hidden field aggregate returned results: %+v", unsupported.Aggregates)
	}
	f.expectStatus(f.api(owner, "POST", "/views/"+table.Views[0].ID+"/aggregates",
		fmt.Sprintf(`{"aggregates":[{"fieldId":%q,"aggregate":"percent_unique"}]}`, active.ID)), 400, "ERR_VALIDATION")
	f.expectStatus(f.api(owner, "POST", "/views/"+table.Views[0].ID+"/query",
		fmt.Sprintf(`{"search":%q}`, strings.Repeat("x", 257))), 400, "ERR_VALIDATION")
}
