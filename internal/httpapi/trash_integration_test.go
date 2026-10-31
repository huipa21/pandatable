package httpapi

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"pandatable/internal/acl"
	"pandatable/internal/structure"
)

func TestTrashHTTPAuthorizationRestoreAndValidation(t *testing.T) {
	f := newACLFixture(t)
	owner, reader, admin := f.user("user", false), f.user("user", false), f.user("admin", false)
	workspace := f.newWorkspace(owner, "Recovery")
	book := f.newBook(owner, workspace.ID, "Book")
	table := f.newTable(owner, book.ID, "Orders")
	f.grantOK(owner, acl.Table, table.Table.ID, "user", reader.id, acl.Read)
	field := f.newField(owner, table.Table.ID,
		`{"name":"Notes","uiType":"text","isRequired":true,"default":"fallback"}`)
	created := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+table.Table.ID+"/rows",
		fmt.Sprintf(`{"rows":[{"fields":{%q:"kept"}}]}`, field.ID)), 201)
	rowID := created.Rows[0].ID
	f.expectStatus(f.fieldCall(owner, "DELETE", "/fields/"+field.ID, "", fmt.Sprint(field.Version)), 200, "")
	f.expectStatus(f.api(owner, "POST", "/tables/"+table.Table.ID+"/rows", `{"rows":[{"fields":{}}]}`), 201, "")
	path := "/trash/field/" + field.ID + "/restore"

	f.expectStatus(f.api(nil, "GET", "/trash", ""), 401, "ERR_UNAUTHENTICATED")
	f.expectStatus(f.api(owner, "GET", "/trash?unused=1", ""), 400, "ERR_VALIDATION")
	for _, user := range []*aclUser{reader, admin} {
		listing := decode[structure.TrashListing](f, f.api(user, "GET", "/trash", ""), 200)
		if len(listing.Items) != 0 {
			t.Fatalf("non-owner saw trash: %+v", listing)
		}
		f.expectStatus(f.api(user, "POST", path, `{}`), 404, "ERR_NOT_FOUND")
	}
	listing := decode[structure.TrashListing](f, f.api(owner, "GET", "/trash", ""), 200)
	if len(listing.Items) != 1 || listing.Items[0].Kind != "field" || listing.Items[0].ID != field.ID ||
		listing.Items[0].WorkspaceID != workspace.ID || listing.Items[0].ParentID != table.Table.ID ||
		listing.Items[0].ExpiresAt.Sub(listing.Items[0].DeletedAt).Hours() != 30*24 {
		t.Fatalf("owner trash response = %+v", listing)
	}
	f.expectStatus(f.api(owner, "POST", path, `{"unexpected":true}`), 400, "ERR_VALIDATION")
	f.expectStatus(f.api(owner, "POST", path, `{}`), 409, "ERR_REQUIRED_HAS_EMPTY")
	request := httptest.NewRequest("POST", "/api/v1"+path, strings.NewReader(`{"allowOptional":true}`))
	request.AddCookie(owner.cookie)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://table.example")
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	f.expectStatus(response, 403, "ERR_CSRF")
	restored := decode[struct {
		EventID string `json:"eventId"`
	}](f, f.api(owner, "POST", path, `{"allowOptional":true}`), 200)
	if restored.EventID == "" {
		t.Fatal("restore did not return an event ID")
	}
	row := decode[structure.RowRecord](f, f.api(owner, "GET", "/tables/"+table.Table.ID+"/rows/"+rowID, ""), 200)
	if string(row.Fields[field.ID]) != `"kept"` {
		t.Fatalf("restored HTTP row = %+v", row)
	}
	f.expectStatus(f.api(owner, "POST", path, `{}`), 404, "ERR_NOT_FOUND")

	f.expectStatus(f.api(owner, "DELETE", "/tables/"+table.Table.ID, ""), 200, "")
	if _, ok := navTables(f.nav(owner))[table.Table.ID]; ok {
		t.Fatal("deleted table remained in navigation")
	}
	f.expectStatus(f.api(reader, "GET", "/tables/"+table.Table.ID+"/schema", ""), 404, "ERR_NOT_FOUND")
	tablePath := "/trash/table/" + table.Table.ID + "/restore"
	f.expectStatus(f.api(admin, "POST", tablePath, `{}`), 404, "ERR_NOT_FOUND")
	f.expectStatus(f.api(owner, "POST", tablePath, `{"allowOptional":true}`), 400, "ERR_VALIDATION")
	f.expectStatus(f.api(owner, "POST", tablePath, `{}`), 200, "")
	row = decode[structure.RowRecord](f, f.api(owner, "GET", "/tables/"+table.Table.ID+"/rows/"+rowID, ""), 200)
	if string(row.Fields[field.ID]) != `"kept"` {
		t.Fatalf("table restore lost row data: %+v", row)
	}
	f.expectStatus(f.api(owner, "POST", "/trash/unknown/"+table.Table.ID+"/restore", `{}`), 404, "ERR_NOT_FOUND")
	f.expectStatus(f.api(owner, "POST", "/trash/table/not-a-uuid/restore", `{}`), 404, "ERR_NOT_FOUND")
}
