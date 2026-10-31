package structure

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"pandatable/internal/acl"
)

func trashContains(items []TrashItem, kind, id string) bool {
	for _, item := range items {
		if item.Kind == kind && item.ID == id {
			return true
		}
	}
	return false
}

func mustTrash(t *testing.T, f *fixture, actor Actor) TrashListing {
	t.Helper()
	listing, err := f.service.ListTrash(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	return listing
}

func mustRestore(t *testing.T, f *fixture, actor Actor, kind, id string) {
	t.Helper()
	if err := f.service.RestoreTrash(f.ctx, actor, kind, id, TrashRestoreOptions{}); err != nil {
		t.Fatalf("restore %s: %v", kind, err)
	}
}

func TestTrashHierarchyRestoreVisibilityAndOwnership(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	reader := f.user("user", "active", "en")
	admin := f.user("admin", "active", "en")
	workspace := f.workspace(owner)
	book := f.book(owner, workspace.ID)
	table := f.table(owner, book.ID)
	f.grant(acl.Workspace, workspace.ID, "user", reader.ID, acl.Read)
	fieldResult, err := f.service.CreateField(f.ctx, owner, table.Table.ID, FieldInput{
		Name: "Notes", UIType: "text", Description: "Preserved configuration", Default: json.RawMessage(`"default"`),
	})
	if err != nil {
		t.Fatal(err)
	}
	field := *fieldResult.Field
	view, err := f.service.CreateView(f.ctx, owner, table.Table.ID, ViewInput{Name: "Extra", Type: "grid", Scope: "shared"})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := f.service.CreateRows(f.ctx, owner, table.Table.ID,
		[]RowInsert{{Fields: map[string]json.RawMessage{field.ID: json.RawMessage(`"kept"`)}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.DeleteField(f.ctx, owner, field.ID, field.Version); err != nil {
		t.Fatal(err)
	}
	if err := f.service.DeleteView(f.ctx, owner, view.ID, view.Version); err != nil {
		t.Fatal(err)
	}
	ownerTrash := mustTrash(t, f, owner)
	if !trashContains(ownerTrash.Items, "field", field.ID) || !trashContains(ownerTrash.Items, "view", view.ID) {
		t.Fatalf("field/view trash = %+v", ownerTrash)
	}
	for _, item := range ownerTrash.Items {
		if item.ExpiresAt.Sub(item.DeletedAt) != 30*24*time.Hour {
			t.Fatalf("default expiry = %+v", item)
		}
	}
	for _, actor := range []Actor{reader, admin} {
		if len(mustTrash(t, f, actor).Items) != 0 {
			t.Fatal("reader or unrelated administrator saw owner trash")
		}
		expectError(t, f.service.RestoreTrash(f.ctx, actor, "field", field.ID, TrashRestoreOptions{}), 404, "ERR_NOT_FOUND")
	}
	if err := f.service.DeleteTable(f.ctx, owner, table.Table.ID); err != nil {
		t.Fatal(err)
	}
	if listing := mustTrash(t, f, owner); !trashContains(listing.Items, "table", table.Table.ID) ||
		trashContains(listing.Items, "field", field.ID) || trashContains(listing.Items, "view", view.ID) {
		t.Fatalf("table trash visibility = %+v", listing)
	}
	if _, err := f.service.Schema(f.ctx, owner, table.Table.ID); err == nil {
		t.Fatal("deleted table schema remains accessible")
	}
	expectError(t, f.service.RestoreTrash(f.ctx, owner, "field", field.ID, TrashRestoreOptions{}), 404, "ERR_NOT_FOUND")
	if err := f.service.DeleteBook(f.ctx, owner, book.ID); err != nil {
		t.Fatal(err)
	}
	if listing := mustTrash(t, f, owner); !trashContains(listing.Items, "book", book.ID) ||
		trashContains(listing.Items, "table", table.Table.ID) {
		t.Fatalf("book trash visibility = %+v", listing)
	}
	if err := f.service.DeleteWorkspace(f.ctx, owner, workspace.ID); err != nil {
		t.Fatal(err)
	}
	if listing := mustTrash(t, f, owner); !trashContains(listing.Items, "workspace", workspace.ID) ||
		trashContains(listing.Items, "book", book.ID) {
		t.Fatalf("workspace trash visibility = %+v", listing)
	}
	mustRestore(t, f, owner, "workspace", workspace.ID)
	mustRestore(t, f, owner, "book", book.ID)
	mustRestore(t, f, owner, "table", table.Table.ID)
	mustRestore(t, f, owner, "field", field.ID)
	mustRestore(t, f, owner, "view", view.ID)
	if len(mustTrash(t, f, owner).Items) != 0 {
		t.Fatal("restored resources remain in trash")
	}
	schema, err := f.service.Schema(f.ctx, owner, table.Table.ID)
	if err != nil {
		t.Fatal(err)
	}
	var restored *Field
	for index := range schema.Fields {
		if schema.Fields[index].ID == field.ID {
			restored = &schema.Fields[index]
		}
	}
	if restored == nil || restored.Description != field.Description || string(restored.Default) != `"default"` {
		t.Fatalf("restored field configuration = %+v", restored)
	}
	row, err := f.service.GetRow(f.ctx, owner, table.Table.ID, rows[0].ID)
	if err != nil || string(row.Fields[field.ID]) != `"kept"` {
		t.Fatalf("restored row = %+v, %v", row, err)
	}
	foundView := false
	for _, item := range schema.Views {
		foundView = foundView || item.ID == view.ID
	}
	if !foundView {
		t.Fatal("restored view identity is missing")
	}
}

func TestTrashFieldRequiredDefaultConflictAndAutonumber(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	ft := f.fieldTable(owner)
	field := ft.create(FieldInput{Name: "Required", UIType: "text", IsRequired: true, Default: json.RawMessage(`"fallback"`)})
	initial, err := f.service.CreateRows(f.ctx, owner, ft.id(),
		[]RowInsert{{Fields: map[string]json.RawMessage{field.ID: json.RawMessage(`"original"`)}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.DeleteField(f.ctx, owner, field.ID, field.Version); err != nil {
		t.Fatal(err)
	}
	newRows, err := f.service.CreateRows(f.ctx, owner, ft.id(), []RowInsert{{Fields: map[string]json.RawMessage{}}})
	if err != nil {
		t.Fatal(err)
	}
	expectError(t, f.service.RestoreTrash(f.ctx, owner, "field", field.ID, TrashRestoreOptions{}), 409, "ERR_REQUIRED_HAS_EMPTY")
	conflict := ft.create(FieldInput{Name: "Required", UIType: "text"})
	expectError(t, f.service.RestoreTrash(f.ctx, owner, "field", field.ID, TrashRestoreOptions{AllowOptional: true}),
		409, "ERR_FIELD_NAME_CONFLICT")
	renamed := "Another name"
	if _, err := ft.update(conflict, FieldPatch{Name: &renamed}); err != nil {
		t.Fatal(err)
	}
	if err := f.service.RestoreTrash(f.ctx, owner, "field", field.ID, TrashRestoreOptions{AllowOptional: true}); err != nil {
		t.Fatal(err)
	}
	schema, err := f.service.Schema(f.ctx, owner, ft.id())
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range schema.Fields {
		if item.ID == field.ID && (item.IsRequired || string(item.Default) != `"fallback"`) {
			t.Fatalf("optional restore lost default or remained required: %+v", item)
		}
	}
	original, err := f.service.GetRow(f.ctx, owner, ft.id(), initial[0].ID)
	if err != nil || string(original.Fields[field.ID]) != `"original"` {
		t.Fatalf("original value = %+v, %v", original, err)
	}
	hiddenInsert, err := f.service.GetRow(f.ctx, owner, ft.id(), newRows[0].ID)
	if err != nil || string(hiddenInsert.Fields[field.ID]) != "null" {
		t.Fatalf("row inserted while hidden = %+v, %v", hiddenInsert, err)
	}
	after, err := f.service.CreateRows(f.ctx, owner, ft.id(), []RowInsert{{Fields: map[string]json.RawMessage{}}})
	if err != nil || string(after[0].Fields[field.ID]) != `"fallback"` {
		t.Fatalf("restored default = %+v, %v", after, err)
	}

	auto := ft.create(FieldInput{Name: "Auto", UIType: "autonumber", Options: json.RawMessage(`{"prefix":"A","padding":3}`)})
	beforeAuto, err := f.service.GetRow(f.ctx, owner, ft.id(), initial[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.DeleteField(f.ctx, owner, auto.ID, auto.Version); err != nil {
		t.Fatal(err)
	}
	inserted, err := f.service.CreateRows(f.ctx, owner, ft.id(), []RowInsert{{Fields: map[string]json.RawMessage{}}})
	if err != nil {
		t.Fatalf("inserting while autonumber is trashed: %v", err)
	}
	mustRestore(t, f, owner, "field", auto.ID)
	oldAuto, err := f.service.GetRow(f.ctx, owner, ft.id(), initial[0].ID)
	if err != nil || string(oldAuto.Fields[auto.ID]) != string(beforeAuto.Fields[auto.ID]) {
		t.Fatalf("restored autonumber changed existing value: %+v, %v", oldAuto, err)
	}
	newAuto, err := f.service.GetRow(f.ctx, owner, ft.id(), inserted[0].ID)
	if err != nil || string(newAuto.Fields[auto.ID]) == "null" {
		t.Fatalf("restored autonumber did not fill hidden row: %+v, %v", newAuto, err)
	}
	if _, err := f.service.CreateRows(f.ctx, owner, ft.id(), []RowInsert{{Fields: map[string]json.RawMessage{}}}); err != nil {
		t.Fatalf("restored autonumber default failed: %v", err)
	}
}

func TestTrashPersonalViewsAndScopedOwners(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	reader := f.user("user", "active", "en")
	tableOwner := f.user("user", "active", "en")
	groupOwner := f.user("user", "active", "en")
	workspace := f.workspace(owner)
	book := f.book(owner, workspace.ID)
	table := f.table(owner, book.ID)
	group := f.group()
	f.member(group, groupOwner)
	f.grant(acl.Book, book.ID, "group", group, acl.Owner)
	f.grant(acl.Table, table.Table.ID, "user", reader.ID, acl.Read)
	f.grant(acl.Table, table.Table.ID, "user", tableOwner.ID, acl.Owner)
	personal, err := f.service.CreateView(f.ctx, reader, table.Table.ID, ViewInput{Name: "Private", Scope: "personal"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.DeleteView(f.ctx, reader, personal.ID, personal.Version); err != nil {
		t.Fatal(err)
	}
	if !trashContains(mustTrash(t, f, reader).Items, "view", personal.ID) ||
		trashContains(mustTrash(t, f, owner).Items, "view", personal.ID) {
		t.Fatal("personal trash privacy or reader ownership was not preserved")
	}
	expectError(t, f.service.RestoreTrash(f.ctx, owner, "view", personal.ID, TrashRestoreOptions{}), 404, "ERR_NOT_FOUND")
	mustRestore(t, f, reader, "view", personal.ID)

	if err := f.service.DeleteTable(f.ctx, owner, table.Table.ID); err != nil {
		t.Fatal(err)
	}
	if trashContains(mustTrash(t, f, tableOwner).Items, "table", table.Table.ID) {
		t.Fatal("table owner may not restore a table without owning its parent book")
	}
	expectError(t, f.service.RestoreTrash(f.ctx, tableOwner, "table", table.Table.ID, TrashRestoreOptions{}), 404, "ERR_NOT_FOUND")
	if !trashContains(mustTrash(t, f, groupOwner).Items, "table", table.Table.ID) {
		t.Fatal("group book owner could not see table trash")
	}
	mustRestore(t, f, groupOwner, "table", table.Table.ID)
}

func TestTrashRelationRejectsMissingPhysicalStorage(t *testing.T) {
	for _, kind := range []string{"one_many", "many_many"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			owner := f.user("user", "active", "en")
			workspace := f.workspace(owner)
			book := f.book(owner, workspace.ID)
			parent, _ := relationTable(t, f, owner, book.ID, "Projects")
			child, _ := relationTable(t, f, owner, book.ID, "Tasks")
			side := "child"
			if kind == "many_many" {
				side = "parent"
			}
			created, err := f.service.CreateRelation(f.ctx, owner, child.Table.ID, RelationInput{
				Name: "Projects", ReverseName: "Tasks", TargetTableID: parent.Table.ID,
				Kind: kind, SourceSide: side, OnDelete: "set_null",
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.service.DeleteField(f.ctx, owner, created.Field.ID, created.Field.Version); err != nil {
				t.Fatal(err)
			}
			schema, table := f.physicalNames(child.Table.ID)
			var column, junction *string
			if err := f.pool.QueryRow(f.ctx, `SELECT fk_column,junction_table FROM meta.relations WHERE id=$1`,
				created.Relation.ID).Scan(&column, &junction); err != nil {
				t.Fatal(err)
			}
			if column != nil {
				f.exec("ALTER TABLE " + pgx.Identifier{schema, table}.Sanitize() +
					" DROP COLUMN " + pgx.Identifier{*column}.Sanitize())
			} else if junction != nil {
				f.exec("DROP TABLE " + pgx.Identifier{schema, *junction}.Sanitize())
			} else {
				t.Fatal("relation has no physical storage")
			}
			expectError(t, f.service.RestoreTrash(f.ctx, owner, "field", created.Field.ID, TrashRestoreOptions{}),
				409, "ERR_TRASH_DEPENDENCY")
			if count := scanOne[int](f, `SELECT count(*) FROM meta.fields
				WHERE id=ANY($1::uuid[]) AND deleted_at IS NOT NULL`,
				[]string{created.Relation.ParentFieldID, created.Relation.ChildFieldID}); count != 2 {
				t.Fatalf("failed restore reactivated %d relation fields", 2-count)
			}
		})
	}
}

func TestTrashLegacyFieldRequiresExplicitOptionalRestore(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	ft := f.fieldTable(owner)
	field := ft.create(FieldInput{Name: "Legacy", UIType: "text", IsRequired: true, Default: json.RawMessage(`"value"`)})
	if _, err := f.service.DeleteField(f.ctx, owner, field.ID, field.Version); err != nil {
		t.Fatal(err)
	}
	f.exec(`UPDATE meta.fields SET trash_required=NULL WHERE id=$1`, field.ID)
	expectError(t, f.service.RestoreTrash(f.ctx, owner, "field", field.ID, TrashRestoreOptions{}),
		409, "ERR_TRASH_REQUIRED_UNKNOWN")
	if err := f.service.RestoreTrash(f.ctx, owner, "field", field.ID, TrashRestoreOptions{AllowOptional: true}); err != nil {
		t.Fatal(err)
	}
	schema, err := f.service.Schema(f.ctx, owner, ft.id())
	if err != nil {
		t.Fatal(err)
	}
	for _, restored := range schema.Fields {
		if restored.ID == field.ID {
			if restored.IsRequired || string(restored.Default) != `"value"` {
				t.Fatalf("legacy restore = %+v", restored)
			}
			return
		}
	}
	t.Fatal("legacy field was not restored")
}
