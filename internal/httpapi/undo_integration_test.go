package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"pandatable/internal/acl"
	"pandatable/internal/attachments"
	"pandatable/internal/structure"
)

func undoBody(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestUndoGuardedPatchLargePayload(t *testing.T) {
	f := newACLFixture(t)
	owner, reader := f.user("user", false), f.user("user", false)
	book := f.newBook(owner, f.newWorkspace(owner, "Large undo").ID, "Book")
	table := f.newTable(owner, book.ID, "Rows")
	field := f.newField(owner, table.Table.ID, `{"name":"Long text","uiType":"long_text"}`)
	f.grantOK(owner, acl.Table, table.Table.ID, "user", reader.id, acl.Read)
	before := strings.Repeat("\U0001f600", 100_000)
	after := strings.Repeat("\U0001f642", 100_000)
	path := "/tables/" + table.Table.ID + "/rows"
	create := undoBody(t, map[string]any{"rows": []any{map[string]any{
		"fields": map[string]any{field.ID: before},
	}}})
	if len(create) >= bulkRowsBodyLimit {
		t.Fatalf("initial edit fixture exceeds the normal limit: %d bytes", len(create))
	}
	row := decode[rowsResponse](f, f.api(owner, "POST", path, create), 201).Rows[0]
	edit := undoBody(t, map[string]any{"rows": []any{map[string]any{
		"id": row.ID, "fields": map[string]any{field.ID: after},
	}}})
	f.expectStatus(f.api(owner, "PATCH", path, edit), 200, "")
	guarded := undoBody(t, map[string]any{"rows": []any{map[string]any{
		"id": row.ID, "fields": map[string]any{field.ID: before},
		"expected": map[string]any{field.ID: after},
	}}})
	if len(guarded) <= bulkRowsBodyLimit || len(guarded) > guardedRowsBodyLimit {
		t.Fatalf("guarded fixture is not between request limits: %d bytes", len(guarded))
	}
	f.expectStatus(f.api(owner, "PATCH", path, guarded), 400, "ERR_VALIDATION")
	f.expectStatus(f.api(reader, "PATCH", path+":guarded", guarded), 403, "ERR_PERMISSION_DENIED")
	f.expectStatus(f.api(owner, "PATCH", path+":guarded", edit), 400, "ERR_VALIDATION")
	f.expectStatus(f.api(owner, "PATCH", path+":guarded", guarded), 200, "")
	restored := decode[structure.RowRecord](f, f.api(owner, "GET", path+"/"+row.ID, ""), 200)
	var actual string
	if err := json.Unmarshal(restored.Fields[field.ID], &actual); err != nil || actual != before {
		t.Fatalf("large guarded undo did not restore the previous value: %v", err)
	}
}

func TestUndoRestoreSnapshotsAndGuardedPatch(t *testing.T) {
	f := newACLFixture(t)
	owner, writer, reader, stranger := f.user("user", false), f.user("user", false),
		f.user("user", false), f.user("admin", false)
	book := f.newBook(owner, f.newWorkspace(owner, "Undo restore").ID, "Book")
	source := f.newTable(owner, book.ID, "Source")
	target := f.newTable(owner, book.ID, "Target")
	name := relationLabelField(source.TableSchema)
	files := f.newField(owner, source.Table.ID, `{"name":"Files","uiType":"attachment"}`)
	number := f.newField(owner, source.Table.ID, `{"name":"Number","uiType":"autonumber"}`)
	relation := decode[relationBody](f, f.api(owner, "POST", "/tables/"+source.Table.ID+"/relations",
		fmt.Sprintf(`{"name":"Targets","reverseName":"Sources","targetTableId":%q,"kind":"many_many"}`,
			target.Table.ID)), 201)
	f.grantOK(owner, acl.Table, source.Table.ID, "user", writer.id, acl.Write)
	f.grantOK(owner, acl.Table, source.Table.ID, "user", reader.id, acl.Read)
	f.grantOK(owner, acl.Table, target.Table.ID, "user", writer.id, acl.Read)

	file := decode[attachments.File](f, attachmentUpload(f, owner, source.Table.ID, files.ID,
		"fixture.pdf", []byte("%PDF-1.4\nundo fixture\n%%EOF\n")), 201)
	created := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+source.Table.ID+"/rows",
		undoBody(t, map[string]any{"rows": []any{map[string]any{"fields": map[string]any{
			name.ID: "original", files.ID: []attachments.File{file},
		}}}})), 201).Rows[0]
	linked := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+target.Table.ID+"/rows",
		`{"rows":[{"fields":{}}]}`), 201).Rows[0]
	linkPath := "/tables/" + source.Table.ID + "/rows/" + created.ID + "/links/" + relation.Relation.ParentFieldID
	f.expectStatus(f.api(owner, "POST", linkPath, undoBody(t, map[string]any{"add": []string{linked.ID}})), 200, "")
	commentPath := "/tables/" + source.Table.ID + "/rows/" + created.ID + "/comments"
	f.expectStatus(f.api(owner, "POST", commentPath, commentBody("keep this comment")), 201, "")
	deleted := decode[rowsResponse](f, f.api(owner, "DELETE", "/tables/"+source.Table.ID+"/rows",
		undoBody(t, map[string]any{"ids": []string{created.ID}})), 200)
	if len(deleted.Rows) != 1 || string(deleted.Rows[0].Fields[files.ID]) == "null" ||
		deleted.Rows[0].RestoreVersion == "" {
		t.Fatalf("delete did not return the full snapshot: %+v", deleted)
	}
	body := undoBody(t, map[string]any{"rows": deleted.Rows})
	reference := structure.RowRecord{ID: created.ID, RestoreVersion: deleted.Rows[0].RestoreVersion}
	path := "/tables/" + source.Table.ID + "/rows:restore"
	f.expectStatus(f.api(reader, "POST", path, body), 403, "ERR_PERMISSION_DENIED")
	f.expectStatus(f.api(stranger, "POST", path, body), 404, "ERR_NOT_FOUND")
	f.expectStatus(f.api(writer, "POST", path, body), 403, "ERR_PERMISSION_DENIED")
	f.expectStatus(f.api(owner, "GET", "/tables/"+source.Table.ID+"/rows/"+created.ID, ""), 404, "ERR_NOT_FOUND")

	forged := deleted.Rows[0]
	forged.Fields = make(map[string]json.RawMessage, len(deleted.Rows[0].Fields))
	for id, value := range deleted.Rows[0].Fields {
		forged.Fields[id] = value
	}
	forged.Fields[name.ID] = json.RawMessage(`"forged"`)
	f.expectStatus(f.api(owner, "POST", path, undoBody(t, map[string]any{"rows": []structure.RowRecord{forged}})),
		409, "ERR_UNDO_UNAVAILABLE")
	f.expectStatus(f.api(owner, "GET", "/tables/"+source.Table.ID+"/rows/"+created.ID, ""), 404, "ERR_NOT_FOUND")

	forgedSystem := deleted.Rows[0]
	forgedSystem.Fields = make(map[string]json.RawMessage, len(deleted.Rows[0].Fields))
	for id, value := range deleted.Rows[0].Fields {
		forgedSystem.Fields[id] = value
	}
	for _, field := range source.TableSchema.Fields {
		if field.UIType == "created_at" {
			forgedSystem.Fields[field.ID] = json.RawMessage(`"2000-01-01T00:00:00Z"`)
		}
	}
	f.expectStatus(f.api(owner, "POST", path, undoBody(t, map[string]any{"rows": []structure.RowRecord{forgedSystem}})),
		409, "ERR_UNDO_UNAVAILABLE")
	history := decode[structure.HistoryListing](f, f.api(owner, "GET",
		"/tables/"+source.Table.ID+"/rows/"+created.ID+"/history", ""), 200)
	for _, item := range history.Items {
		for _, field := range source.TableSchema.Fields {
			if field.IsSystem {
				if _, exists := item.Changes[field.ID]; exists {
					t.Fatalf("public history exposes system metadata: %+v", item)
				}
			}
		}
	}
	f.grantOK(owner, acl.Table, target.Table.ID, "user", writer.id, acl.Write)
	stale := reference
	stale.RestoreVersion = "1"
	f.expectStatus(f.api(writer, "POST", path, undoBody(t, map[string]any{"rows": []structure.RowRecord{stale}})),
		409, "ERR_UNDO_UNAVAILABLE")
	restored := decode[rowsResponse](f, f.api(writer, "POST", path,
		undoBody(t, map[string]any{"rows": []structure.RowRecord{reference}})), 200)
	if len(restored.Rows) != 1 || restored.Rows[0].ID != created.ID ||
		string(restored.Rows[0].Fields[name.ID]) != `"original"` ||
		string(restored.Rows[0].Fields[files.ID]) != string(deleted.Rows[0].Fields[files.ID]) ||
		string(restored.Rows[0].Fields[number.ID]) != string(deleted.Rows[0].Fields[number.ID]) ||
		restored.Rows[0].CommentCount != 1 {
		t.Fatalf("restored row does not match snapshot: %+v", restored)
	}
	for _, field := range source.TableSchema.Fields {
		if field.IsSystem && string(restored.Rows[0].Fields[field.ID]) != string(deleted.Rows[0].Fields[field.ID]) {
			t.Fatalf("restored system field %s = %s, want %s", field.UIType,
				restored.Rows[0].Fields[field.ID], deleted.Rows[0].Fields[field.ID])
		}
	}
	reverse := decode[structure.RowRecord](f, f.api(owner, "GET",
		"/tables/"+target.Table.ID+"/rows/"+linked.ID, ""), 200)
	if string(reverse.Fields[relation.Relation.ChildFieldID]) !=
		fmt.Sprintf(`[{"id":%q,"label":"original"}]`, created.ID) {
		t.Fatalf("reverse link was not restored: %s", reverse.Fields[relation.Relation.ChildFieldID])
	}
	comments := decode[structure.CommentListing](f, f.api(owner, "GET", commentPath, ""), 200)
	if len(comments.Items) != 1 || comments.Items[0].Body != "keep this comment" {
		t.Fatalf("restored comments = %+v", comments)
	}
	f.expectStatus(f.api(owner, "GET", "/attachments/"+file.ID, ""), 200, "")
	f.expectStatus(f.api(owner, "POST", path, body), 409, "ERR_UNDO_UNAVAILABLE")
	deletedAgain := decode[rowsResponse](f, f.api(owner, "DELETE", "/tables/"+source.Table.ID+"/rows",
		undoBody(t, map[string]any{"ids": []string{created.ID}})), 200)
	if deletedAgain.Rows[0].RestoreVersion == reference.RestoreVersion {
		t.Fatal("a later deletion reused an old history version")
	}
	f.expectStatus(f.api(writer, "POST", path, undoBody(t, map[string]any{"rows": []structure.RowRecord{reference}})),
		409, "ERR_UNDO_UNAVAILABLE")
	f.expectStatus(f.api(writer, "POST", path, undoBody(t, map[string]any{"rows": []structure.RowRecord{{
		ID: created.ID, RestoreVersion: deletedAgain.Rows[0].RestoreVersion,
	}}})), 200, "")

	patchPath := "/tables/" + source.Table.ID + "/rows"
	f.expectStatus(f.api(owner, "PATCH", patchPath, fmt.Sprintf(
		`{"rows":[{"id":%q,"fields":{%q:"new"},"expected":{%q:"stale"}}]}`,
		created.ID, name.ID, name.ID)), 409, "ERR_UNDO_CONFLICT")
	unchanged := decode[structure.RowRecord](f, f.api(owner, "GET",
		"/tables/"+source.Table.ID+"/rows/"+created.ID, ""), 200)
	if string(unchanged.Fields[name.ID]) != `"original"` {
		t.Fatalf("guarded patch changed a conflicting row: %+v", unchanged)
	}
	patch := decode[struct {
		Rows     []structure.RowRecord `json:"rows"`
		Previous []structure.RowPatch  `json:"previous"`
	}](f, f.api(owner, "PATCH", patchPath, fmt.Sprintf(
		`{"rows":[{"id":%q,"fields":{%q:"new"},"expected":{%q:"original"}}]}`,
		created.ID, name.ID, name.ID)), 200)
	if len(patch.Previous) != 1 || string(patch.Previous[0].Fields[name.ID]) != `"original"` ||
		string(patch.Rows[0].Fields[name.ID]) != `"new"` {
		t.Fatalf("guarded patch omitted authoritative previous values: %+v", patch)
	}
}

func TestUndoRestoreUnavailableWithoutDeleteMetadata(t *testing.T) {
	f := newACLFixture(t)
	owner := f.user("user", false)
	book := f.newBook(owner, f.newWorkspace(owner, "Old undo snapshot").ID, "Book")
	table := f.newTable(owner, book.ID, "Rows")
	row := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+table.Table.ID+"/rows",
		`{"rows":[{"fields":{}}]}`), 201).Rows[0]
	deleted := decode[rowsResponse](f, f.api(owner, "DELETE", "/tables/"+table.Table.ID+"/rows",
		undoBody(t, map[string]any{"ids": []string{row.ID}})), 200)
	f.exec(`UPDATE meta.row_history SET restore_system=NULL WHERE table_id=$1 AND row_id=$2::bigint AND action='delete'`,
		table.Table.ID, row.ID)
	f.expectStatus(f.api(owner, "POST", "/tables/"+table.Table.ID+"/rows:restore",
		undoBody(t, map[string]any{"rows": deleted.Rows})), 409, "ERR_UNDO_UNAVAILABLE")
	f.expectStatus(f.api(owner, "GET", "/tables/"+table.Table.ID+"/rows/"+row.ID, ""), 404, "ERR_NOT_FOUND")
}

func TestUndoRestoreCascadeAndRelationConflicts(t *testing.T) {
	f := newACLFixture(t)
	owner := f.user("user", false)
	book := f.newBook(owner, f.newWorkspace(owner, "Undo cascades").ID, "Book")
	parents := f.newTable(owner, book.ID, "Parents")
	children := f.newTable(owner, book.ID, "Children")
	relation := decode[relationBody](f, f.api(owner, "POST", "/tables/"+parents.Table.ID+"/relations",
		fmt.Sprintf(`{"name":"Children","reverseName":"Parent","targetTableId":%q,"kind":"one_many","sourceSide":"parent","onDelete":"cascade"}`,
			children.Table.ID)), 201)
	parent := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+parents.Table.ID+"/rows",
		`{"rows":[{"fields":{}}]}`), 201).Rows[0]
	child := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+children.Table.ID+"/rows",
		`{"rows":[{"fields":{}}]}`), 201).Rows[0]
	f.expectStatus(f.api(owner, "POST", "/tables/"+parents.Table.ID+"/rows/"+parent.ID+
		"/links/"+relation.Relation.ParentFieldID, undoBody(t, map[string]any{"add": []string{child.ID}})), 200, "")
	childComments := "/tables/" + children.Table.ID + "/rows/" + child.ID + "/comments"
	f.expectStatus(f.api(owner, "POST", childComments, commentBody("cascade comment")), 201, "")
	deleted := decode[rowsResponse](f, f.api(owner, "DELETE", "/tables/"+parents.Table.ID+"/rows",
		undoBody(t, map[string]any{"ids": []string{parent.ID}})), 200)
	if len(deleted.Rows) != 1 || len(deleted.Rows[0].Cascaded) != 1 ||
		deleted.Rows[0].Cascaded[0].TableID != children.Table.ID ||
		len(deleted.Rows[0].Cascaded[0].Rows) != 1 ||
		deleted.Rows[0].Cascaded[0].Rows[0].ID != child.ID {
		t.Fatalf("cascade snapshots = %+v", deleted.Rows)
	}
	path := "/tables/" + parents.Table.ID + "/rows:restore"
	incomplete := deleted.Rows[0]
	incomplete.Cascaded = nil
	f.expectStatus(f.api(owner, "POST", path,
		undoBody(t, map[string]any{"rows": []structure.RowRecord{incomplete}})), 404, "ERR_NOT_FOUND")
	f.expectStatus(f.api(owner, "GET", "/tables/"+parents.Table.ID+"/rows/"+parent.ID, ""), 404, "ERR_NOT_FOUND")
	cascade := deleted.Rows[0].Cascaded[0]
	f.expectStatus(f.api(owner, "POST", path, undoBody(t, map[string]any{"rows": []structure.RowRecord{{
		ID: deleted.Rows[0].ID, RestoreVersion: deleted.Rows[0].RestoreVersion,
		Cascaded: []structure.RowRestoreGroup{{
			TableID: cascade.TableID,
			Rows: []structure.RowRecord{{
				ID: cascade.Rows[0].ID, RestoreVersion: cascade.Rows[0].RestoreVersion,
			}},
		}},
	}}})), 200, "")
	restoredChild := decode[structure.RowRecord](f, f.api(owner, "GET",
		"/tables/"+children.Table.ID+"/rows/"+child.ID, ""), 200)
	if restoredChild.CommentCount != 1 {
		t.Fatalf("cascade comment was not retained: %+v", restoredChild)
	}
	childSnapshot := deleted.Rows[0].Cascaded[0].Rows[0]
	for _, field := range children.TableSchema.Fields {
		if field.IsSystem && string(restoredChild.Fields[field.ID]) != string(childSnapshot.Fields[field.ID]) {
			t.Fatalf("restored cascade system field %s = %s, want %s", field.UIType,
				restoredChild.Fields[field.ID], childSnapshot.Fields[field.ID])
		}
	}
	comments := decode[structure.CommentListing](f, f.api(owner, "GET", childComments, ""), 200)
	if len(comments.Items) != 1 || comments.Items[0].Body != "cascade comment" {
		t.Fatalf("cascade comments = %+v", comments)
	}
	f.expectStatus(f.api(owner, http.MethodGet, "/tables/"+parents.Table.ID+"/rows/"+parent.ID, ""), 200, "")
}

func TestUndoRestoreDoesNotOverwriteReparentedRows(t *testing.T) {
	f := newACLFixture(t)
	owner := f.user("user", false)
	book := f.newBook(owner, f.newWorkspace(owner, "Undo relation conflicts").ID, "Book")
	parents := f.newTable(owner, book.ID, "Parents")
	children := f.newTable(owner, book.ID, "Children")
	relation := decode[relationBody](f, f.api(owner, "POST", "/tables/"+parents.Table.ID+"/relations",
		fmt.Sprintf(`{"name":"Children","reverseName":"Parent","targetTableId":%q,"kind":"one_many","sourceSide":"parent","onDelete":"set_null"}`,
			children.Table.ID)), 201)
	parentRows := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+parents.Table.ID+"/rows",
		`{"rows":[{"fields":{}},{"fields":{}}]}`), 201).Rows
	child := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+children.Table.ID+"/rows",
		`{"rows":[{"fields":{}}]}`), 201).Rows[0]
	linkPath := func(id string) string {
		return "/tables/" + parents.Table.ID + "/rows/" + id + "/links/" + relation.Relation.ParentFieldID
	}
	link := undoBody(t, map[string]any{"add": []string{child.ID}})
	f.expectStatus(f.api(owner, "POST", linkPath(parentRows[0].ID), link), 200, "")
	deleted := decode[rowsResponse](f, f.api(owner, "DELETE", "/tables/"+parents.Table.ID+"/rows",
		undoBody(t, map[string]any{"ids": []string{parentRows[0].ID}})), 200)
	f.expectStatus(f.api(owner, "POST", linkPath(parentRows[1].ID), link), 200, "")
	f.expectStatus(f.api(owner, "POST", "/tables/"+parents.Table.ID+"/rows:restore",
		undoBody(t, map[string]any{"rows": deleted.Rows})), 409, "ERR_RELATION_CONFLICT")
	f.expectStatus(f.api(owner, "GET", "/tables/"+parents.Table.ID+"/rows/"+parentRows[0].ID, ""), 404, "ERR_NOT_FOUND")
	unchanged := decode[structure.RowRecord](f, f.api(owner, "GET",
		"/tables/"+children.Table.ID+"/rows/"+child.ID, ""), 200)
	var links []structure.LinkValue
	if err := json.Unmarshal(unchanged.Fields[relation.Relation.ChildFieldID], &links); err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || links[0].ID != parentRows[1].ID {
		t.Fatalf("restore overwrote another user's link: %+v", links)
	}
}

func TestUndoLinkReplacementReturnsActualDeltas(t *testing.T) {
	f := newACLFixture(t)
	owner := f.user("user", false)
	book := f.newBook(owner, f.newWorkspace(owner, "Undo link deltas").ID, "Book")
	left := f.newTable(owner, book.ID, "Left")
	right := f.newTable(owner, book.ID, "Right")
	relation := decode[relationBody](f, f.api(owner, "POST", "/tables/"+left.Table.ID+"/relations",
		fmt.Sprintf(`{"name":"Right","reverseName":"Left","targetTableId":%q,"kind":"one_one","sourceSide":"parent"}`,
			right.Table.ID)), 201)
	parents := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+left.Table.ID+"/rows",
		`{"rows":[{"fields":{}},{"fields":{}}]}`), 201).Rows
	child := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+right.Table.ID+"/rows",
		`{"rows":[{"fields":{}}]}`), 201).Rows[0]
	linkPath := func(id string) string {
		return "/tables/" + left.Table.ID + "/rows/" + id + "/links/" + relation.Relation.ParentFieldID
	}
	f.expectStatus(f.api(owner, "POST", linkPath(parents[0].ID),
		undoBody(t, map[string]any{"add": []string{child.ID}})), 200, "")
	result := decode[struct {
		Row     structure.RowRecord       `json:"row"`
		Changes []structure.RelationDelta `json:"changes"`
	}](f, f.api(owner, "POST", linkPath(parents[1].ID),
		undoBody(t, map[string]any{"add": []string{child.ID}, "replace": true})), 200)
	if len(result.Changes) != 2 || result.Changes[0].Action != "unlink" ||
		result.Changes[0].RowID != parents[0].ID || result.Changes[1].Action != "link" ||
		result.Changes[1].RowID != parents[1].ID {
		t.Fatalf("replacement deltas = %+v", result.Changes)
	}
	for i := len(result.Changes) - 1; i >= 0; i-- {
		change := result.Changes[i]
		operation := "add"
		if change.Action == "link" {
			operation = "remove"
		}
		f.expectStatus(f.api(owner, "POST", linkPath(change.RowID),
			undoBody(t, map[string]any{operation: []string{change.TargetID}})), 200, "")
	}
	restored := decode[structure.RowRecord](f, f.api(owner, "GET",
		"/tables/"+left.Table.ID+"/rows/"+parents[0].ID, ""), 200)
	var links []structure.LinkValue
	if err := json.Unmarshal(restored.Fields[relation.Relation.ParentFieldID], &links); err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || links[0].ID != child.ID {
		t.Fatalf("displaced one-to-one link was not restored: %+v", links)
	}
	noop := decode[struct {
		Changes []structure.RelationDelta `json:"changes"`
	}](f, f.api(owner, "POST", linkPath(parents[0].ID),
		undoBody(t, map[string]any{"add": []string{child.ID}})), 200)
	if noop.Changes == nil || len(noop.Changes) != 0 {
		t.Fatalf("no-op link returned changes: %+v", noop.Changes)
	}
}

func TestUndoGuardedBulkPatchIsAtomic(t *testing.T) {
	f := newACLFixture(t)
	owner := f.user("user", false)
	table := f.newTable(owner, f.newBook(owner, f.newWorkspace(owner, "Undo atomic").ID, "Book").ID, "Rows")
	field := relationLabelField(table.TableSchema)
	created := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+table.Table.ID+"/rows",
		fmt.Sprintf(`{"rows":[{"fields":{%q:"one"}},{"fields":{%q:"two"}}]}`, field.ID, field.ID)), 201).Rows
	patch := fmt.Sprintf(`{"rows":[{"id":%q,"fields":{%q:"changed"},"expected":{%q:"one"}},`+
		`{"id":%q,"fields":{%q:"changed"},"expected":{%q:"stale"}}]}`,
		created[0].ID, field.ID, field.ID, created[1].ID, field.ID, field.ID)
	f.expectStatus(f.api(owner, "PATCH", "/tables/"+table.Table.ID+"/rows", patch), 409, "ERR_UNDO_CONFLICT")
	for index, want := range []string{`"one"`, `"two"`} {
		row := decode[structure.RowRecord](f, f.api(owner, "GET",
			"/tables/"+table.Table.ID+"/rows/"+created[index].ID, ""), 200)
		if string(row.Fields[field.ID]) != want {
			t.Fatalf("guard failure committed row %s: %+v", row.ID, row)
		}
	}
}
