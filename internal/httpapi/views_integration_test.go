package httpapi

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"pandatable/internal/acl"
	"pandatable/internal/structure"
)

type viewResponse struct {
	View    structure.View `json:"view"`
	EventID string         `json:"eventId"`
}

func TestViewColumnsHTTPPayloadVersionAndPermissions(t *testing.T) {
	f := newACLFixture(t)
	owner, writer, reader, stranger, admin := f.user("user", false), f.user("user", false),
		f.user("user", false), f.user("user", false), f.user("admin", false)
	workspace := f.newWorkspace(owner, "Columns")
	book := f.newBook(owner, workspace.ID, "Book")
	table := f.newTable(owner, book.ID, "Items")
	f.grantOK(owner, acl.Table, table.Table.ID, "user", writer.id, acl.Write)
	f.grantOK(owner, acl.Table, table.Table.ID, "user", reader.id, acl.Read)
	id, field := table.Views[0].ID, table.Table.DisplayFieldID
	if field == nil {
		t.Fatal("missing display field")
	}
	path := "/views/" + id
	body := fmt.Sprintf(`{"config":{"fields":[{"id":%q,"visible":true,"width":240}],"frozen":1}}`, *field)
	for user, status := range map[*aclUser]int{reader: 403, stranger: 404, admin: 404} {
		f.expectStatus(f.fieldCall(user, "PATCH", path, body, "1"), status,
			map[int]string{403: "ERR_PERMISSION_DENIED", 404: "ERR_NOT_FOUND"}[status])
	}
	for _, match := range [][]string{nil, {""}, {"0"}, {"01"}, {"-1"}, {"1", "2"}, {"W/\"1\""}} {
		f.expectStatus(f.fieldCall(owner, "PATCH", path, body, match...), 400, "ERR_VALIDATION")
	}
	invalid := []string{
		`{}`, `null`, `{"config":null}`, `{"config":{}}`,
		`{"config":{"fields":null,"frozen":0}}`,
		fmt.Sprintf(`{"config":{"fields":[{"id":%q}],"frozen":0}}`, *field),
		fmt.Sprintf(`{"config":{"fields":[{"id":%q,"visible":true,"extra":1}],"frozen":0}}`, *field),
		fmt.Sprintf(`{"config":{"fields":[{"id":%q,"visible":true,"width":59}],"frozen":0}}`, *field),
		fmt.Sprintf(`{"config":{"fields":[{"id":%q,"visible":true,"width":null}],"frozen":0}}`, *field),
		fmt.Sprintf(`{"config":{"fields":[{"id":%q,"visible":true,"width":1001}],"frozen":0}}`, *field),
		fmt.Sprintf(`{"config":{"fields":[{"id":%q,"visible":true},{"id":%q,"visible":false}],"frozen":0}}`, *field, *field),
		`{"config":{"fields":[{"id":"00000000-0000-0000-0000-000000000001","visible":true}],"frozen":0}}`,
		`{"config":{"fields":[],"frozen":-1}}`, `{"config":{"fields":[],"frozen":999}}`,
		body + `{}`, strings.Repeat(" ", 4096) + body,
	}
	for _, member := range []string{"name", "scope", "locked", "position", "filter", "sort", "group", "highlight", "aggregates", "form", "kanban", "calendar", "rowHeight"} {
		invalid = append(invalid, fmt.Sprintf(`{"%s":null,"config":{"fields":[],"frozen":0}}`, member),
			fmt.Sprintf(`{"config":{"%s":null,"fields":[],"frozen":0}}`, member))
	}
	for i, rejected := range invalid {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			f.expectStatus(f.fieldCall(owner, "PATCH", path, rejected, "1"), 400, "ERR_VALIDATION")
		})
	}
	accepted := decode[viewResponse](f, f.fieldCall(writer, "PATCH", path, body, `"1"`), 200)
	if accepted.View.Version != 2 || accepted.EventID == "" || accepted.View.Config.Frozen != 1 ||
		accepted.View.Config.Fields[0].Width == nil || *accepted.View.Config.Fields[0].Width != 240 {
		t.Fatalf("column response = %+v", accepted)
	}
	f.expectStatus(f.fieldCall(owner, "PATCH", path, body, "1"), 409, "ERR_VERSION_CONFLICT")
	schema := decode[structure.TableSchema](f, f.api(reader, "GET", "/tables/"+table.Table.ID+"/schema", ""), 200)
	if schema.Views[0].Version != 2 || schema.Views[0].Config.Fields[0].ID != *field {
		t.Fatal("schema did not retain columns")
	}
	f.exec("UPDATE meta.views SET locked=true WHERE id=$1", id)
	for _, user := range []*aclUser{owner, writer} {
		f.expectStatus(f.fieldCall(user, "PATCH", path, body, "2"), 403, "ERR_PERMISSION_DENIED")
	}
	personal := f.scan(`INSERT INTO meta.views(table_id,name,type,scope,owner_id,position)
		VALUES($1,'Personal','grid','personal',$2,99) RETURNING id::text`, table.Table.ID, writer.id)
	f.expectStatus(f.fieldCall(owner, "PATCH", "/views/"+personal, body, "1"), 404, "ERR_NOT_FOUND")
	decode[viewResponse](f, f.fieldCall(writer, "PATCH", "/views/"+personal, body, "1"), 200)
	f.exec("UPDATE meta.views SET deleted_at=now() WHERE id=$1", personal)
	f.expectStatus(f.fieldCall(writer, "PATCH", "/views/"+personal, body, "2"), 404, "ERR_NOT_FOUND")
	f.exec("UPDATE meta.workspaces SET deleted_at=now() WHERE id=$1", workspace.ID)
	f.expectStatus(f.fieldCall(owner, "PATCH", path, body, "2"), 404, "ERR_NOT_FOUND")
}

func TestViewColumnsHTTPCleanupConcurrentAndTransport(t *testing.T) {
	f := newACLFixture(t)
	owner := f.user("user", false)
	workspace := f.newWorkspace(owner, "Column concurrency")
	book := f.newBook(owner, workspace.ID, "Book")
	table := f.newTable(owner, book.ID, "Items")
	field := f.newField(owner, table.Table.ID, `{"name":"Deleted","uiType":"text"}`)
	id := table.Views[0].ID
	path := "/views/" + id
	f.exec("UPDATE meta.fields SET deleted_at=now() WHERE id=$1", field.ID)
	f.exec(`UPDATE meta.views SET config=jsonb_build_object('fields',jsonb_build_array(
		jsonb_build_object('id',$2::text,'visible',true)), 'frozen',1,'rowHeight','short') WHERE id=$1`, id, field.ID)
	schema := decode[structure.TableSchema](f, f.api(owner, "GET", "/tables/"+table.Table.ID+"/schema", ""), 200)
	for _, column := range schema.Views[0].Config.Fields {
		if column.ID == field.ID {
			t.Fatal("deleted column leaked")
		}
	}
	deleted := fmt.Sprintf(`{"config":{"fields":[{"id":%q,"visible":true}],"frozen":0}}`, field.ID)
	f.expectStatus(f.fieldCall(owner, "PATCH", path, deleted, "1"), 400, "ERR_VALIDATION")
	body := fmt.Sprintf(`{"config":{"fields":[{"id":%q,"visible":true,"width":60}],"frozen":0}}`, *table.Table.DisplayFieldID)
	for _, header := range []string{"Origin", "X-PT-CSRF"} {
		req := httptest.NewRequest("PATCH", "/api/v1"+path, strings.NewReader(body))
		f.decorate(req, owner)
		req.Header.Set("If-Match", "1")
		req.Header.Del(header)
		rec := httptest.NewRecorder()
		f.handler.ServeHTTP(rec, req)
		f.expectStatus(rec, 403, "ERR_CSRF")
	}
	f.expectStatus(f.fieldCall(nil, "PATCH", path, body, "1"), 401, "ERR_UNAUTHENTICATED")
	var wg sync.WaitGroup
	responses := make(chan *httptest.ResponseRecorder, 2)
	start := make(chan struct{})
	for range 2 {
		wg.Go(func() { <-start; responses <- f.fieldCall(owner, "PATCH", path, body, "1") })
	}
	close(start)
	wg.Wait()
	close(responses)
	statuses := map[int]int{}
	for response := range responses {
		statuses[response.Code]++
		if response.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("missing no-store")
		}
	}
	if statuses[200] != 1 || statuses[409] != 1 {
		t.Fatalf("concurrent saves = %v", statuses)
	}
	stored := f.scan("SELECT config::text FROM meta.views WHERE id=$1", id)
	if strings.Contains(stored, field.ID) || !strings.Contains(stored, `"rowHeight": "short"`) {
		t.Fatalf("cleanup/preservation failed: %s", stored)
	}
	if f.scan("SELECT version::text FROM meta.views WHERE id=$1", id) != "2" {
		t.Fatal("version bumped more than once")
	}
}
