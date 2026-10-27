package httpapi

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"pandatable/internal/acl"
	"pandatable/internal/structure"
)

type rowsResponse struct {
	Rows    []structure.RowRecord `json:"rows"`
	EventID string                `json:"eventId"`
}

type rowResponse struct {
	Row     structure.RowRecord `json:"row"`
	EventID string              `json:"eventId"`
}

func TestRowsHTTPAPIQueryMutationsAndGuards(t *testing.T) {
	f := newACLFixture(t)
	owner, writer, reader, stranger, admin := f.user("user", false), f.user("user", false), f.user("user", false),
		f.user("user", false), f.user("admin", false)
	workspace := f.newWorkspace(owner, "Rows")
	book := f.newBook(owner, workspace.ID, "Book")
	table := f.newTable(owner, book.ID, "Orders")
	f.grantOK(owner, acl.Table, table.Table.ID, "user", writer.id, acl.Write)
	f.grantOK(owner, acl.Table, table.Table.ID, "user", reader.id, acl.Read)
	field := f.newField(owner, table.Table.ID, `{"name":"Label","uiType":"text"}`)
	body := fmt.Sprintf(`{"rows":[{"fields":{"%s":"first"}},{"fields":{"%s":"second"}}]}`, field.ID, field.ID)
	created := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+table.Table.ID+"/rows", body), 201)
	if len(created.Rows) != 2 || created.EventID == "" || created.Rows[0].ID == created.Rows[1].ID {
		t.Fatalf("create rows = %+v", created)
	}

	query := decode[structure.RowQueryResult](f, f.api(owner, "POST", "/views/"+table.Views[0].ID+"/query",
		`{"offset":0,"limit":1,"includeCount":true}`), 200)
	if len(query.Rows) != 1 || query.Total == nil || *query.Total != 2 ||
		len(query.FieldIDs) != 2 || query.SchemaVersion != table.SchemaVersion+1 {
		t.Fatalf("query = %+v", query)
	}
	var systemID string
	for _, field := range table.Fields {
		if field.UIType == "id" {
			systemID = field.ID
			break
		}
	}
	if systemID == "" {
		t.Fatal("table schema is missing the system id field")
	}
	requested := decode[structure.RowQueryResult](f, f.api(owner, "POST",
		"/views/"+table.Views[0].ID+"/query", fmt.Sprintf(`{"fieldIds":[%q]}`, systemID)), 200)
	if len(requested.FieldIDs) != 1 || requested.FieldIDs[0] != systemID ||
		requested.Rows[0].Fields[systemID] == nil {
		t.Fatalf("explicit system-field query = %+v", requested)
	}
	row := decode[structure.RowRecord](f, f.api(owner, "GET",
		"/tables/"+table.Table.ID+"/rows/"+created.Rows[0].ID, ""), 200)
	if row.ID != created.Rows[0].ID || string(row.Fields[field.ID]) != `"first"` {
		t.Fatalf("get row = %+v", row)
	}
	patched := decode[rowResponse](f, f.api(writer, "PATCH",
		"/tables/"+table.Table.ID+"/rows/"+row.ID, fmt.Sprintf(`{"fields":{"%s":"writer edit"}}`, field.ID)), 200)
	if patched.EventID == "" || string(patched.Row.Fields[field.ID]) != `"writer edit"` {
		t.Fatalf("patch row = %+v", patched)
	}
	deleted := decode[rowsResponse](f, f.api(owner, "DELETE", "/tables/"+table.Table.ID+"/rows",
		fmt.Sprintf(`{"ids":[%q]}`, row.ID)), 200)
	if len(deleted.Rows) != 1 || deleted.Rows[0].ID != row.ID || deleted.EventID == "" {
		t.Fatalf("delete rows = %+v", deleted)
	}
	f.expectStatus(f.api(owner, "GET", "/tables/"+table.Table.ID+"/rows/"+row.ID, ""), 404, "ERR_NOT_FOUND")

	for user, expected := range map[*aclUser]struct {
		read, write int
	}{
		reader:   {read: 200, write: 403},
		stranger: {read: 404, write: 404},
		admin:    {read: 404, write: 404},
	} {
		f.expectStatus(f.api(user, "GET", "/tables/"+table.Table.ID+"/rows/"+created.Rows[1].ID, ""), expected.read,
			map[int]string{403: "ERR_PERMISSION_DENIED", 404: "ERR_NOT_FOUND"}[expected.read])
		f.expectStatus(f.api(user, "PATCH", "/tables/"+table.Table.ID+"/rows/"+created.Rows[1].ID,
			fmt.Sprintf(`{"fields":{"%s":"denied"}}`, field.ID)), expected.write,
			map[int]string{403: "ERR_PERMISSION_DENIED", 404: "ERR_NOT_FOUND"}[expected.write])
	}

	// Admin role alone does not grant data access; hidden personal views are also 404.
	personal := f.scan(`INSERT INTO meta.views (table_id,name,type,scope,owner_id,position)
		VALUES ($1,'Personal','grid','personal',$2,99) RETURNING id::text`, table.Table.ID, owner.id)
	f.expectStatus(f.api(writer, "POST", "/views/"+personal+"/query", `{}`), 404, "ERR_NOT_FOUND")
	f.expectStatus(f.api(owner, "POST", "/views/"+table.Views[0].ID+"/query", `{"limit":101}`), 400, "ERR_VALIDATION")
	f.expectStatus(f.api(owner, "POST", "/tables/"+table.Table.ID+"/rows", `{"rows":[],"pgType":"text"}`), 400,
		"ERR_VALIDATION")

	tooLarge := `{"rows":[],"padding":"` + strings.Repeat("x", (512<<10)) + `"}`
	f.expectStatus(f.api(owner, "POST", "/tables/"+table.Table.ID+"/rows", tooLarge), 400, "ERR_VALIDATION")
}

func TestRowsHTTPAPIRequiresCSRFAndMapsCodecAndConstraintErrors(t *testing.T) {
	f := newACLFixture(t)
	owner := f.user("user", false)
	workspace := f.newWorkspace(owner, "Row errors")
	book := f.newBook(owner, workspace.ID, "Book")
	table := f.newTable(owner, book.ID, "Items")
	number := f.newField(owner, table.Table.ID, `{"name":"Number","uiType":"integer"}`)
	required := f.newField(owner, table.Table.ID, `{"name":"Required","uiType":"text","isRequired":true}`)
	first := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+table.Table.ID+"/rows",
		fmt.Sprintf(`{"rows":[{"fields":{"%s":1,"%s":"value"}}]}`, number.ID, required.ID)), 201)

	request := httptest.NewRequest("PATCH", "/api/v1/tables/"+table.Table.ID+"/rows/"+first.Rows[0].ID,
		strings.NewReader(fmt.Sprintf(`{"fields":{"%s":"invalid"}}`, number.ID)))
	request.RemoteAddr = "192.0.2.250:5000"
	f.decorate(request, owner)
	request.Header.Del("X-PT-CSRF")
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	f.expectStatus(response, 403, "ERR_CSRF")

	foreignOrigin := httptest.NewRequest("POST", "/api/v1/tables/"+table.Table.ID+"/rows",
		strings.NewReader(`{"rows":[]}`))
	foreignOrigin.RemoteAddr = "192.0.2.250:5000"
	f.decorate(foreignOrigin, owner)
	foreignOrigin.Header.Set("Origin", "https://evil.example")
	foreignResponse := httptest.NewRecorder()
	f.handler.ServeHTTP(foreignResponse, foreignOrigin)
	f.expectStatus(foreignResponse, 403, "ERR_CSRF")

	f.expectStatus(f.api(owner, "PATCH", "/tables/"+table.Table.ID+"/rows/"+first.Rows[0].ID,
		fmt.Sprintf(`{"fields":{"%s":"invalid"}}`, number.ID)), 400, "ERR_VALIDATION")
	f.expectStatus(f.api(owner, "POST", "/tables/"+table.Table.ID+"/rows",
		fmt.Sprintf(`{"rows":[{"fields":{"%s":null}}]}`, required.ID)), 400, "ERR_REQUIRED_FIELD")
	unchanged := decode[structure.RowRecord](f, f.api(owner, "GET",
		"/tables/"+table.Table.ID+"/rows/"+first.Rows[0].ID, ""), 200)
	if string(unchanged.Fields[number.ID]) != "1" {
		t.Fatalf("invalid integer patch persisted: %s", unchanged.Fields[number.ID])
	}
}
