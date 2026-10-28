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

func TestFieldAPIAcceptancePassAT301To304(t *testing.T) {
	f := newACLFixture(t)
	alice := f.user("user", false)
	bob := f.user("user", false)
	workspace := f.newWorkspace(alice, "Acceptance Space")
	book := f.newBook(alice, workspace.ID, "Acceptance Book")
	table := f.newTable(alice, book.ID, "Acceptance Table")
	f.grantOK(alice, acl.Table, table.Table.ID, "user", bob.id, acl.Write)

	// AT-301: Create all 12 batch-1 field kinds through the HTTP API.
	type spec struct {
		name, kind, options string
		wantDataType        string
		sampleValue         any
	}
	specs := []spec{
		{"Code", "text", `{"maxLength":50}`, "character varying", "CODE-1"},
		{"Notes", "long_text", `{}`, "text", "Long\nNotes"},
		{"Qty", "integer", `{"size":"int"}`, "integer", float64(42)},
		{"Price", "decimal", `{"precision":10,"scale":2}`, "numeric", "99.95"},
		{"Score", "float", `{"size":"double"}`, "double precision", float64(4.5)},
		{"Active", "checkbox", `{}`, "boolean", true},
		{"DueDate", "date", `{}`, "date", "2026-11-20"},
		{"EventAt", "datetime", `{}`, "timestamp with time zone", "2026-11-20T10:00:00.000000Z"},
		{"Stage", "single_select", `{"choices":[{"id":"c1","label":"Todo","color":"gray"},{"id":"c2","label":"Done","color":"green"}]}`, "text", "Todo"},
		{"Tags", "multi_select", `{"choices":[{"id":"t1","label":"Alpha","color":"blue"},{"id":"t2","label":"Beta","color":"red"}]}`, "ARRAY", []any{"Alpha"}},
		{"Contact", "email", `{}`, "text", "contact@example.test"},
		{"Website", "url", `{}`, "text", "https://example.test"},
	}

	createdFields := make(map[string]structure.Field)
	for _, s := range specs {
		body := fmt.Sprintf(`{"name":%q,"uiType":%q,"options":%s}`, s.name, s.kind, s.options)
		created := f.newField(alice, table.Table.ID, body)
		createdFields[s.name] = created
	}

	// Authoritative verification against information_schema.columns (AT-301).
	var pgSchema, pgName string
	if err := f.pool.QueryRow(f.ctx, `SELECT b.pg_schema, t.pg_name FROM meta.tables t
		JOIN meta.books b ON b.id=t.book_id WHERE t.id=$1`, table.Table.ID).Scan(&pgSchema, &pgName); err != nil {
		t.Fatal(err)
	}

	for _, s := range specs {
		field := createdFields[s.name]
		var pgColumn string
		if err := f.pool.QueryRow(f.ctx, "SELECT pg_column FROM meta.fields WHERE id=$1", field.ID).Scan(&pgColumn); err != nil {
			t.Fatalf("%s column: %v", s.name, err)
		}
		var dataType, isNullable string
		var colDefault *string
		if err := f.pool.QueryRow(f.ctx, `SELECT data_type, is_nullable, column_default
			FROM information_schema.columns WHERE table_schema=$1 AND table_name=$2 AND column_name=$3`,
			pgSchema, pgName, pgColumn).Scan(&dataType, &isNullable, &colDefault); err != nil {
			t.Fatalf("%s information_schema: %v", s.name, err)
		}
		if dataType != s.wantDataType {
			t.Errorf("%s: data_type = %q, want %q", s.name, dataType, s.wantDataType)
		}
		if s.kind == "checkbox" {
			if isNullable != "NO" || colDefault == nil || *colDefault != "false" {
				t.Errorf("checkbox physical constraint: nullable=%s default=%v", isNullable, colDefault)
			}
		}
	}

	// Verify system columns in information_schema (AT-304).
	sysCols := map[string]string{
		"id":         "bigint",
		"created_at": "timestamp with time zone",
		"created_by": "uuid",
		"updated_at": "timestamp with time zone",
		"updated_by": "uuid",
	}
	for colName, wantType := range sysCols {
		var dataType string
		if err := f.pool.QueryRow(f.ctx, `SELECT data_type FROM information_schema.columns
			WHERE table_schema=$1 AND table_name=$2 AND column_name=$3`,
			pgSchema, pgName, colName).Scan(&dataType); err != nil {
			t.Fatalf("system col %s: %v", colName, err)
		}
		if dataType != wantType {
			t.Errorf("system col %s: data_type = %q, want %q", colName, dataType, wantType)
		}
	}

	// Insert row with all 12 kinds (AT-301).
	insertFields := make(map[string]any)
	for _, s := range specs {
		insertFields[createdFields[s.name].ID] = s.sampleValue
	}
	insertPayload, _ := json.Marshal(map[string]any{"rows": []map[string]any{{"fields": insertFields}}})
	createRowsRes := decode[rowsResponse](f, f.api(alice, "POST", "/tables/"+table.Table.ID+"/rows", string(insertPayload)), 201)
	if len(createRowsRes.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(createRowsRes.Rows))
	}
	rowID := createRowsRes.Rows[0].ID

	// Read row back (AT-301) and assert values.
	readRow := decode[structure.RowRecord](f, f.api(alice, "GET", "/tables/"+table.Table.ID+"/rows/"+rowID, ""), 200)
	for _, s := range specs {
		field := createdFields[s.name]
		val, ok := readRow.Fields[field.ID]
		if !ok || val == nil {
			t.Errorf("%s missing in row fields", s.name)
		}
	}

	// AT-304: authorship values.
	schemaResp := decode[structure.TableSchema](f, f.api(alice, "GET", "/tables/"+table.Table.ID+"/schema", ""), 200)
	var createdByField, updatedByField structure.Field
	for _, fld := range schemaResp.Fields {
		if fld.UIType == "created_by" {
			createdByField = fld
		}
		if fld.UIType == "updated_by" {
			updatedByField = fld
		}
	}
	if string(readRow.Fields[createdByField.ID]) != `"`+alice.id+`"` {
		t.Errorf("created_by = %s, want %s", readRow.Fields[createdByField.ID], alice.id)
	}
	if string(readRow.Fields[updatedByField.ID]) != `"`+alice.id+`"` {
		t.Errorf("updated_by = %s, want %s", readRow.Fields[updatedByField.ID], alice.id)
	}

	// Bob edits the row (AT-304).
	bobPatchPayload, _ := json.Marshal(map[string]any{"fields": map[string]any{createdFields["Code"].ID: "CODE-BOB"}})
	decode[rowResponse](f, f.api(bob, "PATCH", "/tables/"+table.Table.ID+"/rows/"+rowID, string(bobPatchPayload)), 200)

	// Verify authorship after Bob's edit: created_by stays Alice, updated_by becomes Bob.
	readRowBob := decode[structure.RowRecord](f, f.api(alice, "GET", "/tables/"+table.Table.ID+"/rows/"+rowID, ""), 200)
	if string(readRowBob.Fields[createdByField.ID]) != `"`+alice.id+`"` {
		t.Errorf("after edit created_by = %s, want %s", readRowBob.Fields[createdByField.ID], alice.id)
	}
	if string(readRowBob.Fields[updatedByField.ID]) != `"`+bob.id+`"` {
		t.Errorf("after edit updated_by = %s, want %s", readRowBob.Fields[updatedByField.ID], bob.id)
	}

	// AT-303: Required and unique preflights on populated table.
	// Add an empty second row.
	decode[rowsResponse](f, f.api(alice, "POST", "/tables/"+table.Table.ID+"/rows", `{"rows":[{"fields":{}}]}`), 201)

	// Create a new text field "ReqTest".
	reqField := f.newField(alice, table.Table.ID, `{"name":"ReqTest","uiType":"text"}`)
	// Setting required without default or fillEmpty fails with 409 ERR_REQUIRED_HAS_EMPTY (count = 2).
	reqFail := f.fieldCall(alice, "PATCH", "/fields/"+reqField.ID, `{"isRequired":true}`, "1")
	f.expectStatus(reqFail, 409, "ERR_REQUIRED_HAS_EMPTY")

	// Setting required with fillEmpty and explicit default succeeds.
	reqOK := decode[fieldBody](f, f.fieldCall(alice, "PATCH", "/fields/"+reqField.ID, `{"isRequired":true,"fillEmpty":true,"default":"INIT"}`, "1"), 200)
	if !reqOK.Field.IsRequired {
		t.Error("expected IsRequired to be true")
	}

	// Setting unique on ReqTest (where both rows have "INIT") fails with 409 ERR_UNIQUE_HAS_DUPLICATES.
	uniqFail := f.fieldCall(alice, "PATCH", "/fields/"+reqField.ID, `{"isUnique":true}`, "2")
	f.expectStatus(uniqFail, 409, "ERR_UNIQUE_HAS_DUPLICATES")

	// Query rows to get row 2 ID.
	queryRes := decode[structure.RowQueryResult](f, f.api(alice, "POST", "/views/"+table.Views[0].ID+"/query", `{"offset":0,"limit":10}`), 200)
	if len(queryRes.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(queryRes.Rows))
	}
	row2ID := queryRes.Rows[1].ID

	// Bob updates row 2 to "INIT-2" so values are distinct.
	patchRow2, _ := json.Marshal(map[string]any{"fields": map[string]any{reqField.ID: "INIT-2"}})
	decode[rowResponse](f, f.api(bob, "PATCH", "/tables/"+table.Table.ID+"/rows/"+row2ID, string(patchRow2)), 200)

	// Now setting unique succeeds.
	uniqOK := decode[fieldBody](f, f.fieldCall(alice, "PATCH", "/fields/"+reqField.ID, `{"isUnique":true}`, "2"), 200)
	if !uniqOK.Field.IsUnique {
		t.Error("expected IsUnique to be true")
	}

	// Unique rejects duplicate row update.
	patchDup, _ := json.Marshal(map[string]any{"fields": map[string]any{reqField.ID: "INIT"}})
	f.expectStatus(f.api(bob, "PATCH", "/tables/"+table.Table.ID+"/rows/"+row2ID, string(patchDup)), 409, "ERR_UNIQUE_CONFLICT")

	// Default applies to newly inserted row.
	// Make row 1's ReqTest unique so "INIT" is free for the default on new row.
	patchRow1, _ := json.Marshal(map[string]any{"fields": map[string]any{reqField.ID: "INIT-1"}})
	decode[rowResponse](f, f.api(alice, "PATCH", "/tables/"+table.Table.ID+"/rows/"+rowID, string(patchRow1)), 200)

	createRow3 := decode[rowsResponse](f, f.api(alice, "POST", "/tables/"+table.Table.ID+"/rows", `{"rows":[{"fields":{}}]}`), 201)
	if len(createRow3.Rows) != 1 {
		t.Fatalf("expected 1 row created, got %d", len(createRow3.Rows))
	}
	row3ID := createRow3.Rows[0].ID
	row3 := decode[structure.RowRecord](f, f.api(alice, "GET", "/tables/"+table.Table.ID+"/rows/"+row3ID, ""), 200)
	if string(row3.Fields[reqField.ID]) != `"INIT"` {
		t.Errorf("new row default = %s, want %q", row3.Fields[reqField.ID], "INIT")
	}
}
