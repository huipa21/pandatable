package httpapi

import (
	"encoding/json"
	"testing"

	"pandatable/internal/structure"
)

func TestSQLInjectionStringsRemainDataThroughHTTPAT804(t *testing.T) {
	f := newACLFixture(t)
	owner := f.user("user", false)
	workspace := f.newWorkspace(owner, "Injection acceptance")
	book := f.newBook(owner, workspace.ID, "Book")

	tableName := `表 "); DROP TABLE meta.users; --`
	fieldName := `字段 '); DROP TABLE meta.users; --`
	needle := `"; DROP TABLE meta.users; -- %_`
	value := "prefix " + needle + " suffix"
	decoy := `prefix "; DROP TABLE meta.users; -- XY suffix`

	table := f.newTable(owner, book.ID, tableName)
	if table.Table.Name != tableName || f.scan("SELECT name FROM meta.tables WHERE id=$1", table.Table.ID) != tableName {
		t.Fatal("table name was not stored literally")
	}
	fieldBody, err := json.Marshal(map[string]string{"name": fieldName, "uiType": "text"})
	if err != nil {
		t.Fatal(err)
	}
	field := f.newField(owner, table.Table.ID, string(fieldBody))
	if field.Name != fieldName || f.scan("SELECT name FROM meta.fields WHERE id=$1", field.ID) != fieldName {
		t.Fatal("field name was not stored literally")
	}

	body, err := json.Marshal(map[string]any{
		"rows": []map[string]any{
			{"fields": map[string]string{field.ID: value}},
			{"fields": map[string]string{field.ID: decoy}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	created := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+table.Table.ID+"/rows", string(body)), 201)
	if len(created.Rows) != 2 {
		t.Fatalf("created rows = %+v", created.Rows)
	}

	for _, filter := range []struct {
		operator string
		value    string
	}{{"is", value}, {"contains", needle}} {
		body, err := json.Marshal(map[string]any{
			"filter":       map[string]string{"field": field.ID, "cmp": filter.operator, "value": filter.value},
			"includeCount": true,
		})
		if err != nil {
			t.Fatal(err)
		}
		result := decode[structure.RowQueryResult](f, f.api(owner, "POST", "/views/"+table.Views[0].ID+"/query", string(body)), 200)
		if result.Total == nil || *result.Total != 1 || len(result.Rows) != 1 || result.Rows[0].ID != created.Rows[0].ID {
			t.Fatalf("%s filter matched unexpected rows: %+v", filter.operator, result)
		}
		var actual string
		if err := json.Unmarshal(result.Rows[0].Fields[field.ID], &actual); err != nil || actual != value {
			t.Fatalf("%s filter returned %q, want %q (error: %v)", filter.operator, actual, value, err)
		}
		if f.scan("SELECT to_regclass('meta.users')::text") != "meta.users" {
			t.Fatal("filter changed the users table")
		}
	}
	f.expectStatus(f.api(owner, "GET", "/auth/me", ""), 200, "")
}
