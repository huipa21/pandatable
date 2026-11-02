package httpapi

import (
	"fmt"
	"testing"

	"pandatable/internal/acl"
	"pandatable/internal/structure"
)

func TestGalleryHTTPConfigurationAndPermissions(t *testing.T) {
	f := newACLFixture(t)
	owner, writer, reader, stranger := f.user("user", false), f.user("user", false),
		f.user("user", false), f.user("user", false)
	workspace := f.newWorkspace(owner, "Gallery")
	book := f.newBook(owner, workspace.ID, "Book")
	table := f.newTable(owner, book.ID, "Images")
	f.grantOK(owner, acl.Table, table.Table.ID, "user", writer.id, acl.Write)
	f.grantOK(owner, acl.Table, table.Table.ID, "user", reader.id, acl.Read)
	photo := f.newField(owner, table.Table.ID, `{"name":"Photo","uiType":"attachment"}`)
	notes := f.newField(owner, table.Table.ID, `{"name":"Notes","uiType":"text"}`)
	title := *table.Table.DisplayFieldID

	created := decode[viewResponse](f, f.api(owner, "POST", "/tables/"+table.Table.ID+"/views",
		`{"name":"Gallery","type":"gallery","scope":"shared"}`), 201)
	view := created.View
	if view.Type != "gallery" || view.Config.Gallery == nil || view.Config.Gallery.CoverField != nil ||
		len(view.Config.Gallery.Fields) != 3 {
		t.Fatalf("new Gallery view = %+v", view)
	}
	path := "/views/" + view.ID
	config := fmt.Sprintf(`{"config":{"gallery":{"coverField":%q,"fields":[%q,%q]}}}`,
		photo.ID, title, notes.ID)
	for _, invalid := range []string{
		`{"config":{"gallery":null}}`,
		`{"config":{"gallery":{"fields":[]}}}`,
		`{"config":{"gallery":{"coverField":null,"fields":null}}}`,
		`{"config":{"gallery":{"coverField":true,"fields":[]}}}`,
		`{"config":{"gallery":{"coverField":null,"fields":[],"extra":1}}}`,
		fmt.Sprintf(`{"config":{"gallery":{"coverField":%q,"fields":[]}}}`, notes.ID),
		fmt.Sprintf(`{"config":{"gallery":{"coverField":%q,"fields":[%q,%q]}}}`, photo.ID, title, title),
		`{"config":{"gallery":{"coverField":"00000000-0000-0000-0000-000000000001","fields":[]}}}`,
		`{"config":{"kanban":{"groupField":null,"fields":[]}}}`,
	} {
		f.expectStatus(f.fieldCall(owner, "PATCH", path, invalid, "1"), 400, "ERR_VALIDATION")
	}
	f.expectStatus(f.fieldCall(reader, "PATCH", path, config, "1"), 403, "ERR_PERMISSION_DENIED")
	f.expectStatus(f.fieldCall(stranger, "PATCH", path, config, "1"), 404, "ERR_NOT_FOUND")
	saved := decode[viewResponse](f, f.fieldCall(writer, "PATCH", path, config, "1"), 200)
	if saved.View.Version != 2 || saved.View.Config.Gallery == nil ||
		*saved.View.Config.Gallery.CoverField != photo.ID || len(saved.View.Config.Gallery.Fields) != 2 {
		t.Fatalf("saved Gallery = %+v", saved.View)
	}
	f.expectStatus(f.fieldCall(owner, "PATCH", path, config, "1"), 409, "ERR_VERSION_CONFLICT")

	rows := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+table.Table.ID+"/rows",
		fmt.Sprintf(`{"rows":[{"fields":{%q:"First",%q:"Detail"}}]}`, title, notes.ID)), 201).Rows
	result := decode[structure.RowQueryResult](f, f.api(reader, "POST", path+"/query",
		fmt.Sprintf(`{"fieldIds":[%q,%q,%q],"limit":20}`, title, photo.ID, notes.ID)), 200)
	if len(result.Rows) != 1 || result.Rows[0].ID != rows[0].ID {
		t.Fatalf("Gallery rows = %+v", result.Rows)
	}
	duplicate := decode[viewResponse](f, f.api(owner, "POST", path+"/duplicate",
		`{"name":"Gallery copy","scope":"shared"}`), 201)
	if duplicate.View.Config.Gallery == nil || *duplicate.View.Config.Gallery.CoverField != photo.ID {
		t.Fatalf("duplicated Gallery = %+v", duplicate.View)
	}
	f.exec("UPDATE meta.views SET locked=true WHERE id=$1", view.ID)
	f.expectStatus(f.fieldCall(writer, "PATCH", path, config, "2"), 403, "ERR_PERMISSION_DENIED")
	decode[viewResponse](f, f.fieldCall(owner, "PATCH", path, config, "2"), 200)

	personal := decode[viewResponse](f, f.api(reader, "POST", "/tables/"+table.Table.ID+"/views",
		`{"name":"Personal gallery","type":"gallery","scope":"personal"}`), 201)
	decode[viewResponse](f, f.fieldCall(reader, "PATCH", "/views/"+personal.View.ID, config, "1"), 200)
	f.expectStatus(f.fieldCall(owner, "PATCH", "/views/"+personal.View.ID, config, "2"), 404, "ERR_NOT_FOUND")
}
