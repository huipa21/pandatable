package httpapi

import (
	"fmt"
	"testing"

	"pandatable/internal/acl"
	"pandatable/internal/structure"
)

func TestKanbanHTTPConfigurationGroupingAndPermissions(t *testing.T) {
	f := newACLFixture(t)
	owner, writer, reader, stranger := f.user("user", false), f.user("user", false),
		f.user("user", false), f.user("user", false)
	workspace := f.newWorkspace(owner, "Kanban")
	book := f.newBook(owner, workspace.ID, "Book")
	table := f.newTable(owner, book.ID, "Tasks")
	f.grantOK(owner, acl.Table, table.Table.ID, "user", writer.id, acl.Write)
	f.grantOK(owner, acl.Table, table.Table.ID, "user", reader.id, acl.Read)
	status := f.newField(owner, table.Table.ID,
		`{"name":"Status","uiType":"single_select","options":{"choices":[{"id":"todo","label":"Todo"},{"id":"done","label":"Done"}]}}`)
	assignee := f.newField(owner, table.Table.ID, `{"name":"Assignee","uiType":"user"}`)
	text := f.newField(owner, table.Table.ID, `{"name":"Notes","uiType":"text"}`)
	title := *table.Table.DisplayFieldID

	created := decode[viewResponse](f, f.api(owner, "POST", "/tables/"+table.Table.ID+"/views",
		`{"name":"Board","type":"kanban","scope":"shared"}`), 201)
	view := created.View
	if view.Type != "kanban" || view.Config.Kanban == nil || view.Config.Kanban.GroupField != nil ||
		len(view.Config.Kanban.Fields) != 4 {
		t.Fatalf("new Kanban view = %+v", view)
	}
	path := "/views/" + view.ID
	config := fmt.Sprintf(`{"config":{"kanban":{"groupField":%q,"fields":[%q,%q]}}}`,
		status.ID, title, text.ID)
	for _, invalid := range []string{
		`{"config":{"kanban":null}}`,
		`{"config":{"kanban":{"fields":[]}}}`,
		`{"config":{"kanban":{"groupField":null,"fields":null}}}`,
		`{"config":{"kanban":{"groupField":true,"fields":[]}}}`,
		`{"config":{"kanban":{"groupField":null,"fields":[],"extra":1}}}`,
		fmt.Sprintf(`{"config":{"kanban":{"groupField":%q,"fields":[]}}}`, text.ID),
		fmt.Sprintf(`{"config":{"kanban":{"groupField":%q,"fields":[%q,%q]}}}`, status.ID, title, title),
		`{"config":{"kanban":{"groupField":"00000000-0000-0000-0000-000000000001","fields":[]}}}`,
	} {
		f.expectStatus(f.fieldCall(owner, "PATCH", path, invalid, "1"), 400, "ERR_VALIDATION")
	}
	f.expectStatus(f.fieldCall(reader, "PATCH", path, config, "1"), 403, "ERR_PERMISSION_DENIED")
	f.expectStatus(f.fieldCall(stranger, "PATCH", path, config, "1"), 404, "ERR_NOT_FOUND")
	saved := decode[viewResponse](f, f.fieldCall(writer, "PATCH", path, config, "1"), 200)
	if saved.View.Version != 2 || saved.View.Config.Kanban == nil ||
		*saved.View.Config.Kanban.GroupField != status.ID || len(saved.View.Config.Kanban.Fields) != 2 {
		t.Fatalf("saved Kanban = %+v", saved.View)
	}
	f.expectStatus(f.fieldCall(owner, "PATCH", path, config, "1"), 409, "ERR_VERSION_CONFLICT")

	rows := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+table.Table.ID+"/rows",
		fmt.Sprintf(`{"rows":[{"fields":{%q:"First",%q:"Todo",%q:%q}},{"fields":{%q:"Second",%q:"Done"}},{"fields":{%q:"Third"}}]}`,
			title, status.ID, assignee.ID, writer.id, title, status.ID, title)), 201).Rows
	groups := decode[structure.RowGroupResult](f, f.api(reader, "POST", path+"/groups", `{"path":[]}`), 200)
	counts := make(map[string]int64)
	for _, group := range groups.Groups {
		counts[string(group.Key)] = group.Count
	}
	if counts[`"Todo"`] != 1 || counts[`"Done"`] != 1 || counts["null"] != 1 {
		t.Fatalf("select groups = %+v", counts)
	}
	todo := decode[structure.RowQueryResult](f, f.api(reader, "POST", path+"/query",
		`{"groupPath":["Todo"],"limit":20}`), 200)
	if len(todo.Rows) != 1 || todo.Rows[0].ID != rows[0].ID {
		t.Fatalf("Todo rows = %+v", todo.Rows)
	}
	patch := fmt.Sprintf(`{"fields":{%q:"Done"}}`, status.ID)
	f.expectStatus(f.api(reader, "PATCH", "/tables/"+table.Table.ID+"/rows/"+rows[0].ID, patch),
		403, "ERR_PERMISSION_DENIED")
	f.expectStatus(f.api(writer, "PATCH", "/tables/"+table.Table.ID+"/rows/"+rows[0].ID, patch), 200, "")
	done := decode[structure.RowQueryResult](f, f.api(reader, "POST", path+"/query",
		`{"groupPath":["Done"],"limit":20}`), 200)
	if len(done.Rows) != 2 {
		t.Fatalf("Done rows after move = %+v", done.Rows)
	}

	userConfig := fmt.Sprintf(`{"config":{"kanban":{"groupField":%q,"fields":[%q]}}}`, assignee.ID, title)
	userView := decode[viewResponse](f, f.fieldCall(owner, "PATCH", path, userConfig, "2"), 200)
	userRows := decode[structure.RowQueryResult](f, f.api(reader, "POST", path+"/query",
		fmt.Sprintf(`{"groupPath":[%q],"limit":20}`, writer.id)), 200)
	if userView.View.Version != 3 || len(userRows.Rows) != 1 || userRows.Rows[0].ID != rows[0].ID {
		t.Fatalf("user grouping = %+v, rows %+v", userView.View.Config.Kanban, userRows.Rows)
	}
	duplicate := decode[viewResponse](f, f.api(owner, "POST", path+"/duplicate",
		`{"name":"Board copy","scope":"shared"}`), 201)
	if duplicate.View.Config.Kanban == nil || *duplicate.View.Config.Kanban.GroupField != assignee.ID {
		t.Fatalf("duplicated Kanban = %+v", duplicate.View)
	}
	f.exec("UPDATE meta.views SET locked=true WHERE id=$1", view.ID)
	f.expectStatus(f.fieldCall(writer, "PATCH", path, userConfig, "3"), 403, "ERR_PERMISSION_DENIED")
	decode[viewResponse](f, f.fieldCall(owner, "PATCH", path, userConfig, "3"), 200)

	personal := decode[viewResponse](f, f.api(reader, "POST", "/tables/"+table.Table.ID+"/views",
		`{"name":"Personal board","type":"kanban","scope":"personal"}`), 201)
	decode[viewResponse](f, f.fieldCall(reader, "PATCH", "/views/"+personal.View.ID, config, "1"), 200)
	f.expectStatus(f.fieldCall(owner, "PATCH", "/views/"+personal.View.ID, config, "2"), 404, "ERR_NOT_FOUND")
}
