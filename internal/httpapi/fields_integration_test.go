package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"pandatable/internal/acl"
	"pandatable/internal/structure"
)

type fieldBody struct {
	Field   *structure.Field      `json:"field"`
	Schema  structure.TableSchema `json:"schema"`
	EventID string                `json:"eventId"`
}

func (f *aclFixture) fieldCall(user *aclUser, method, path, body string, ifMatch ...string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
	request.RemoteAddr = "192.0.2.250:5000"
	f.decorate(request, user)
	for _, value := range ifMatch {
		request.Header.Add("If-Match", value)
	}
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	return response
}

func (f *aclFixture) newField(user *aclUser, table, body string) structure.Field {
	f.t.Helper()
	created := decode[fieldBody](f, f.fieldCall(user, "POST", "/tables/"+table+"/fields", body), 201)
	if created.Field == nil || created.EventID == "" || created.Schema.Table.ID != table {
		f.t.Fatalf("field response = %+v", created)
	}
	return *created.Field
}

func TestFieldAPIRenameAT202KeepsPhysicalNamesAndData(t *testing.T) {
	f := newACLFixture(t)
	alice, writer, reader, stranger, admin := f.user("user", false), f.user("user", false), f.user("user", false),
		f.user("user", false), f.user("admin", false)
	workspace := f.newWorkspace(alice, "Fields")
	book := f.newBook(alice, workspace.ID, "Book")
	table := f.newTable(alice, book.ID, "Orders")
	f.grantOK(alice, acl.Table, table.Table.ID, "user", writer.id, acl.Write)
	f.grantOK(alice, acl.Table, table.Table.ID, "user", reader.id, acl.Read)

	field := f.newField(alice, table.Table.ID, `{"name":"Amount","uiType":"decimal","options":{"precision":12,"scale":2}}`)
	column := f.scan("SELECT pg_column FROM meta.fields WHERE id=$1", field.ID)
	physical := f.scan("SELECT b.pg_schema || '.' || t.pg_name FROM meta.tables t JOIN meta.books b ON b.id=t.book_id WHERE t.id=$1",
		table.Table.ID)
	f.exec("INSERT INTO " + physical + " (" + column + ") VALUES (12.5)")

	// Non-owners: visible write/read → 403, hidden/admin → 404; nothing changes.
	for user, status := range map[*aclUser]int{writer: 403, reader: 403, stranger: 404, admin: 404} {
		code := map[int]string{403: "ERR_PERMISSION_DENIED", 404: "ERR_NOT_FOUND"}[status]
		f.expectStatus(f.fieldCall(user, "PATCH", "/fields/"+field.ID, `{"name":"x"}`, "1"), status, code)
		f.expectStatus(f.fieldCall(user, "DELETE", "/fields/"+field.ID, "", "1"), status, code)
		f.expectStatus(f.fieldCall(user, "POST", "/tables/"+table.Table.ID+"/fields", `{"name":"x","uiType":"text"}`), status, code)
		f.expectStatus(f.fieldCall(user, "PATCH", "/tables/"+table.Table.ID, `{"displayFieldId":"`+field.ID+`"}`), status, code)
	}

	renamed := decode[fieldBody](f, f.fieldCall(alice, "PATCH", "/fields/"+field.ID, `{"name":"金额 (含税)"}`, `"1"`), 200)
	if renamed.Field == nil || renamed.Field.Name != "金额 (含税)" || renamed.Field.Version != 2 ||
		renamed.Schema.SchemaVersion != field.Version+2 || renamed.EventID == "" {
		t.Fatalf("rename = %+v", renamed)
	}
	if f.scan("SELECT pg_column FROM meta.fields WHERE id=$1", field.ID) != column ||
		f.scan("SELECT "+column+"::text FROM "+physical) != "12.50" {
		t.Fatal("rename changed physical name or data")
	}
	raw, _ := json.Marshal(renamed)
	for _, leaked := range []string{column, strings.Split(physical, ".")[0], strings.Split(physical, ".")[1]} {
		if strings.Contains(string(raw), leaked) {
			t.Fatalf("response leaks %s", leaked)
		}
	}
	// Readers see the renamed field through the schema.
	schema := decode[structure.TableSchema](f, f.api(reader, "GET", "/tables/"+table.Table.ID+"/schema", ""), 200)
	var seen *structure.Field
	for index := range schema.Fields {
		if schema.Fields[index].ID == field.ID {
			seen = &schema.Fields[index]
		}
	}
	if seen == nil || seen.Name != "金额 (含税)" || schema.MyLevel != "read" {
		t.Fatalf("reader schema = %+v", schema.Fields)
	}

	// Display field and soft delete through the API.
	decode[tableBody](f, f.fieldCall(alice, "PATCH", "/tables/"+table.Table.ID, `{"displayFieldId":"`+field.ID+`"}`), 200)
	if f.scan("SELECT display_field_id::text FROM meta.tables WHERE id=$1", table.Table.ID) != field.ID {
		t.Fatal("display field not set")
	}
	deleted := decode[fieldBody](f, f.fieldCall(alice, "DELETE", "/fields/"+field.ID, "", "2"), 200)
	if deleted.Field != nil || *deleted.Schema.Table.DisplayFieldID != table.Fields[0].ID {
		t.Fatalf("delete = %+v", deleted.Schema.Table)
	}
	f.expectStatus(f.fieldCall(alice, "DELETE", "/fields/"+field.ID, "", "3"), 404, "ERR_NOT_FOUND")
	if f.scan("SELECT "+column+"::text FROM "+physical) != "12.50" {
		t.Fatal("soft delete lost data")
	}
}

func TestFieldAPIIfMatchBodiesAndGuards(t *testing.T) {
	f := newACLFixture(t)
	alice, forced, disabled := f.user("user", false), f.user("user", true), f.user("user", false)
	table := f.newTable(alice, f.newBook(alice, f.newWorkspace(alice, "Guards").ID, "B").ID, "T")
	field := f.newField(alice, table.Table.ID, `{"name":"Code","uiType":"text"}`)
	f.exec("UPDATE meta.users SET status='disabled' WHERE id=$1", disabled.id)

	for _, route := range []struct{ method, path, body string }{
		{"POST", "/tables/" + table.Table.ID + "/fields", `{"name":"x","uiType":"text"}`},
		{"PATCH", "/fields/" + field.ID, `{"name":"x"}`},
		{"DELETE", "/fields/" + field.ID, ""},
	} {
		f.expectStatus(f.fieldCall(nil, route.method, route.path, route.body, "1"), 401, "ERR_UNAUTHENTICATED")
		f.expectStatus(f.fieldCall(disabled, route.method, route.path, route.body, "1"), 401, "ERR_UNAUTHENTICATED")
		f.expectStatus(f.fieldCall(forced, route.method, route.path, route.body, "1"), 403, "ERR_PASSWORD_CHANGE_REQUIRED")
		request := httptest.NewRequest(route.method, "/api/v1"+route.path, strings.NewReader(route.body))
		request.RemoteAddr = "192.0.2.250:5000"
		f.decorate(request, alice)
		request.Header.Del("X-PT-CSRF")
		request.Header.Set("If-Match", "1")
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, request)
		if response.Code != 403 || !strings.Contains(response.Body.String(), "ERR_CSRF") {
			t.Fatalf("%s %s without CSRF: %d", route.method, route.path, response.Code)
		}
	}

	// If-Match is required, a single positive canonical integer (optionally quoted).
	for _, header := range [][]string{nil, {""}, {"abc"}, {"0"}, {"-1"}, {"01"}, {"W/\"1\""}, {"1", "1"}, {"99999999999"}} {
		f.expectStatus(f.fieldCall(alice, "PATCH", "/fields/"+field.ID, `{"name":"x"}`, header...), 400, "ERR_VALIDATION")
		f.expectStatus(f.fieldCall(alice, "DELETE", "/fields/"+field.ID, "", header...), 400, "ERR_VALIDATION")
	}
	f.expectStatus(f.fieldCall(alice, "PATCH", "/fields/"+field.ID, `{"name":"x"}`, "7"), 409, "ERR_VERSION_CONFLICT")
	if f.scan("SELECT name FROM meta.fields WHERE id=$1", field.ID) != "Code" {
		t.Fatal("rejected requests changed the field")
	}

	// Strict bodies: unknown members, missing name/type, null options.
	for _, body := range []string{`{"name":"x","uiType":"text","pgType":"text"}`, `{"name":"x"}`, `{"uiType":"text"}`,
		`{"name":"x","uiType":"text","options":"{}"}`, `{"name":"x","uiType":"text","isRequired":"yes"}`} {
		f.expectStatus(f.fieldCall(alice, "POST", "/tables/"+table.Table.ID+"/fields", body), 400, "ERR_VALIDATION")
	}
	f.expectStatus(f.fieldCall(alice, "PATCH", "/fields/"+field.ID, `{"options":null}`, "1"), 400, "ERR_VALIDATION")
	f.expectStatus(f.fieldCall(alice, "PATCH", "/fields/"+field.ID, `{"uiType":"integer"}`, "1"), 400, "ERR_VALIDATION")

	// D-125: field bodies accept 500 choices (well above 4096 bytes) but stay bounded.
	choices := make([]map[string]string, 500)
	for index := range choices {
		choices[index] = map[string]string{"id": fmt.Sprintf("c%03d", index), "label": fmt.Sprintf("选项 %03d %s", index,
			strings.Repeat("x", 60)), "color": "blue"}
	}
	options, _ := json.Marshal(map[string]any{"choices": choices})
	body := `{"name":"Many","uiType":"single_select","options":` + string(options) + `}`
	if len(body) < 4096*8 {
		t.Fatalf("500-choice body only %d bytes", len(body))
	}
	many := f.newField(alice, table.Table.ID, body)
	if !strings.Contains(string(many.Options), "c499") {
		t.Fatal("500 choices not stored")
	}
	tooMany := strings.Replace(body, `"name":"Many"`, `"name":"Many2","description":"`+strings.Repeat("a", 512<<10)+`"`, 1)
	f.expectStatus(f.fieldCall(alice, "POST", "/tables/"+table.Table.ID+"/fields", tooMany), 400, "ERR_VALIDATION")
	// Other routes keep the 4096-byte limit.
	f.expectStatus(f.fieldCall(alice, "PATCH", "/tables/"+table.Table.ID, `{"name":"`+strings.Repeat("a", 5000)+`"}`),
		400, "ERR_VALIDATION")

	// Codec failures surface a safe reason/option, never server internals.
	response := f.fieldCall(alice, "POST", "/tables/"+table.Table.ID+"/fields", `{"name":"x","uiType":"integer","default":1.5}`)
	f.expectStatus(response, 400, "ERR_VALIDATION")
	if !strings.Contains(response.Body.String(), `"option":"default"`) {
		t.Fatalf("default error details: %s", response.Body)
	}
}
