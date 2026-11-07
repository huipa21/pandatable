package structure

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl"
	"pandatable/internal/ddl"
)

func mustUUID(t testing.TB, value string) pgtype.UUID {
	t.Helper()
	uuid, ok := acl.ParseUUID(value)
	if !ok {
		t.Fatalf("invalid UUID %q", value)
	}
	return uuid
}

func ptr[T any](value T) *T { return &value }

func TestAnyActiveUserCreatesWorkspaceWithAtomicDirectOwnerGrant(t *testing.T) {
	f := newFixture(t)
	for _, role := range []string{"user", "admin"} {
		user := f.user(role, "active", "en")
		before := f.resolver.Stats().Epoch
		workspace, err := f.service.CreateWorkspace(f.ctx, user, WorkspaceInput{Name: "  Team Space  ",
			Icon: ptr("🐼"), Color: ptr("#AaBbCc")})
		if err != nil {
			t.Fatal(err)
		}
		f.spaces = append(f.spaces, workspace.ID)
		if workspace.Name != "Team Space" || *workspace.Icon != "🐼" || *workspace.Color != "#aabbcc" {
			t.Fatalf("workspace was not normalized: %+v", workspace)
		}
		if f.resolver.Stats().Epoch == before {
			t.Fatal("creating a resource must invalidate the resolver")
		}
		var level string
		var granted *string
		if err := f.pool.QueryRow(f.ctx, `SELECT level, granted_by::text FROM meta.grants WHERE resource_type='workspace'
			AND resource_id=$1 AND principal_type='user' AND principal_id=$2`, workspace.ID, user.ID).Scan(&level, &granted); err != nil {
			t.Fatalf("%s creator has no direct owner grant: %v", role, err)
		}
		if level != "owner" || granted == nil || *granted != user.ID {
			t.Fatalf("grant = %s by %v", level, granted)
		}
		if got := scanOne[int](f, "SELECT count(*) FROM meta.grants WHERE resource_id=$1", workspace.ID); got != 1 {
			t.Fatalf("workspace has %d grants, want exactly the owner grant", got)
		}
		if f.level(user, acl.Workspace, workspace.ID) != acl.Owner {
			t.Fatal("creator is not effective owner")
		}
	}
}

func TestWorkspaceCreationRejectsInvalidInputAndInactiveUsers(t *testing.T) {
	f := newFixture(t)
	user := f.user("user", "active", "en")
	for _, input := range []WorkspaceInput{
		{Name: " \t "},
		{Name: strings.Repeat("a", 101)},
		{Name: "bad\x00name"},
		{Name: "bad\nname"},
		{Name: "ok", Color: ptr("red")},
		{Name: "ok", Color: ptr("#abc")},
		{Name: "ok", Icon: ptr(strings.Repeat("x", 17))},
		{Name: "ok", Icon: ptr(" ")},
		{Name: "ok", Icon: ptr("not-an-icon")},
		{Name: "ok", Description: strings.Repeat("界", 501)},
		{Name: "ok", Description: "bad\tcontrol"},
	} {
		_, err := f.service.CreateWorkspace(f.ctx, user, input)
		expectError(t, err, 400, "ERR_VALIDATION")
	}
	validDescription, err := f.service.CreateWorkspace(f.ctx, user, WorkspaceInput{
		Name: "ok", Description: strings.Repeat("界", 498) + "\n界",
	})
	if err != nil {
		t.Fatalf("500-rune description with a newline must be accepted: %v", err)
	}
	f.spaces = append(f.spaces, validDescription.ID)
	workspace, err := f.service.CreateWorkspace(f.ctx, user, WorkspaceInput{Name: strings.Repeat("名", 100)})
	if err != nil {
		t.Fatalf("100 code points must be accepted: %v", err)
	}
	f.spaces = append(f.spaces, workspace.ID)
	disabled := f.user("user", "disabled", "en")
	before := scanOne[int](f, "SELECT count(*) FROM meta.workspaces")
	_, err = f.service.CreateWorkspace(f.ctx, disabled, WorkspaceInput{Name: "No"})
	expectError(t, err, 401, "ERR_UNAUTHENTICATED")
	_, err = f.service.CreateWorkspace(f.ctx, Actor{ID: "00000000-0000-4000-8000-000000000000"}, WorkspaceInput{Name: "No"})
	expectError(t, err, 401, "ERR_UNAUTHENTICATED")
	if scanOne[int](f, "SELECT count(*) FROM meta.workspaces") != before {
		t.Fatal("rejected creations must not leave workspaces")
	}
}

type column struct {
	Name, Type, Nullable string
	Default              *string
	Identity             string
}

func (f *fixture) columns(schema, table string) []column {
	f.t.Helper()
	rows, err := f.pool.Query(f.ctx, `SELECT column_name, data_type, is_nullable, column_default, is_identity
		FROM information_schema.columns WHERE table_schema = $1 AND table_name = $2 ORDER BY ordinal_position`, schema, table)
	if err != nil {
		f.t.Fatal(err)
	}
	defer rows.Close()
	var result []column
	for rows.Next() {
		var c column
		if err := rows.Scan(&c.Name, &c.Type, &c.Nullable, &c.Default, &c.Identity); err != nil {
			f.t.Fatal(err)
		}
		result = append(result, c)
	}
	if err := rows.Err(); err != nil {
		f.t.Fatal(err)
	}
	return result
}

func TestCreateHierarchyWritesExactPhysicalAndMetadataState(t *testing.T) {
	f := newFixture(t)
	alice := f.user("user", "active", "en")
	workspace := f.workspace(alice)
	book := f.book(alice, workspace.ID)

	schema := scanOne[string](f, "SELECT pg_schema FROM meta.books WHERE id=$1", book.ID)
	if err := ddl.ValidateIdentifier(ddl.Book, schema); err != nil || !f.schemaExists(schema) {
		t.Fatalf("book schema %q valid=%v exists=%v", schema, err, f.schemaExists(schema))
	}
	if book.Position != 1024 || book.WorkspaceID != workspace.ID {
		t.Fatalf("book = %+v", book)
	}

	created := f.table(alice, book.ID)
	_, physical := f.physicalNames(created.Table.ID)
	if err := ddl.ValidateIdentifier(ddl.Table, physical); err != nil {
		t.Fatal(err)
	}
	nameColumn := scanOne[string](f, "SELECT pg_column FROM meta.fields WHERE table_id=$1 AND NOT is_system", created.Table.ID)
	if err := ddl.ValidateIdentifier(ddl.Column, nameColumn); err != nil {
		t.Fatal(err)
	}

	columns := f.columns(schema, physical)
	wantNames := []string{"id", "created_at", "created_by", "updated_at", "updated_by", nameColumn}
	if len(columns) != len(wantNames) {
		t.Fatalf("physical columns = %+v", columns)
	}
	for index, want := range wantNames {
		if columns[index].Name != want {
			t.Fatalf("column %d = %s, want %s", index, columns[index].Name, want)
		}
	}
	if columns[0].Identity != "YES" || columns[0].Type != "bigint" || columns[1].Nullable != "NO" ||
		columns[1].Default == nil || *columns[1].Default != "now()" || columns[2].Type != "uuid" ||
		columns[2].Nullable != "YES" || columns[5].Type != "text" || columns[5].Nullable != "YES" {
		t.Fatalf("physical definitions = %+v", columns)
	}
	if got := scanOne[int](f, `SELECT count(*) FROM pg_constraint c JOIN pg_class r ON r.oid=c.conrelid
		JOIN pg_namespace n ON n.oid=r.relnamespace WHERE n.nspname=$1 AND r.relname=$2 AND c.contype='f'`, schema, physical); got != 0 {
		t.Fatalf("%d physical authorship foreign keys", got)
	}

	schemaDTO, err := f.service.Schema(f.ctx, alice, created.Table.ID)
	if err != nil {
		t.Fatal(err)
	}
	if schemaDTO.MyLevel != "owner" || schemaDTO.SchemaVersion != 1 || schemaDTO.Table.SchemaVersion != 1 ||
		schemaDTO.Table.Position != 1024 || len(schemaDTO.Relations) != 0 {
		t.Fatalf("schema = %+v", schemaDTO)
	}
	encoded, _ := json.Marshal(schemaDTO)
	for _, leaked := range []string{physical, nameColumn, schema} {
		if strings.Contains(string(encoded), leaked) {
			t.Fatalf("DTO leaks physical identifier %s: %s", leaked, encoded)
		}
	}

	type fieldWant struct {
		name, uiType     string
		system, required bool
	}
	want := []fieldWant{{"Name", "text", false, false}, {"ID", "id", true, true}, {"Created at", "created_at", true, true},
		{"Created by", "created_by", true, false}, {"Updated at", "updated_at", true, true}, {"Updated by", "updated_by", true, false}}
	if len(schemaDTO.Fields) != len(want) {
		t.Fatalf("fields = %+v", schemaDTO.Fields)
	}
	for index, field := range schemaDTO.Fields {
		if field.Name != want[index].name || field.UIType != want[index].uiType || field.IsSystem != want[index].system ||
			field.IsRequired != want[index].required || field.IsUnique || field.Version != 1 || string(field.Options) != "{}" {
			t.Fatalf("field %d = %+v, want %+v", index, field, want[index])
		}
	}
	if schemaDTO.Table.DisplayFieldID == nil || *schemaDTO.Table.DisplayFieldID != schemaDTO.Fields[0].ID {
		t.Fatalf("display field = %v, want Name %s", schemaDTO.Table.DisplayFieldID, schemaDTO.Fields[0].ID)
	}
	rows, err := f.pool.Query(f.ctx, `SELECT pg_column, pg_type FROM meta.fields WHERE table_id=$1 AND is_system ORDER BY position`, created.Table.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	wantTypes := [][2]string{{"id", "bigint"}, {"created_at", "timestamptz"}, {"created_by", "uuid"}, {"updated_at", "timestamptz"}, {"updated_by", "uuid"}}
	for _, expected := range wantTypes {
		var column, kind string
		if !rows.Next() {
			t.Fatal("missing system field")
		}
		if err := rows.Scan(&column, &kind); err != nil || column != expected[0] || kind != expected[1] {
			t.Fatalf("system field %s %s, want %v (%v)", column, kind, expected, err)
		}
	}
	rows.Close()

	if len(schemaDTO.Views) != 1 || schemaDTO.Views[0].Name != "Grid" || schemaDTO.Views[0].Type != "grid" ||
		schemaDTO.Views[0].Scope != "shared" || schemaDTO.Views[0].Locked {
		t.Fatalf("views = %+v", schemaDTO.Views)
	}
	var config struct {
		Fields []struct {
			ID      string `json:"id"`
			Visible bool   `json:"visible"`
			Width   int    `json:"width"`
		} `json:"fields"`
		Frozen    int    `json:"frozen"`
		RowHeight string `json:"rowHeight"`
	}
	raw := scanOne[[]byte](f, "SELECT config FROM meta.views WHERE id=$1", schemaDTO.Views[0].ID)
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	if config.Frozen != 1 || config.RowHeight != "short" || len(config.Fields) != 6 || config.Fields[0].ID != schemaDTO.Fields[0].ID ||
		!config.Fields[0].Visible || config.Fields[0].Width != 240 {
		t.Fatalf("grid config = %s", raw)
	}
	for index, field := range config.Fields[1:] {
		if field.Visible || field.ID != schemaDTO.Fields[index+1].ID {
			t.Fatalf("system field %d must be hidden in order: %s", index, raw)
		}
	}

	second := f.table(alice, book.ID)
	if second.Table.Position != 2048 {
		t.Fatalf("second table position = %v", second.Table.Position)
	}
	if _, secondPhysical := f.physicalNames(second.Table.ID); secondPhysical == physical {
		t.Fatal("tables must receive distinct physical names")
	}
}

func TestDefaultNamesFollowCreatorLocale(t *testing.T) {
	f := newFixture(t)
	alice := f.user("user", "active", "zh-CN")
	table := f.table(alice, f.book(alice, f.workspace(alice).ID).ID)
	var names []string
	for _, field := range table.Fields {
		names = append(names, field.Name)
	}
	if strings.Join(names, ",") != "名称,ID,创建时间,创建人,更新时间,更新人" || table.Views[0].Name != "表格" {
		t.Fatalf("zh-CN defaults = %v / %s", names, table.Views[0].Name)
	}
	// Unknown profile locales fall back to English.
	bob := f.user("user", "active", "en")
	bob.Locale = "fr"
	if got := f.table(bob, f.book(bob, f.workspace(bob).ID).ID); got.Fields[0].Name != "Name" {
		t.Fatalf("fallback name = %s", got.Fields[0].Name)
	}
}

func TestRenamesAreMetadataOnlyAndPreserveData(t *testing.T) {
	f := newFixture(t)
	alice := f.user("user", "active", "en")
	workspace := f.workspace(alice)
	book := f.book(alice, workspace.ID)
	table := f.table(alice, book.ID)
	schema, physical := f.physicalNames(table.Table.ID)
	f.exec("INSERT INTO " + schema + "." + physical + " DEFAULT VALUES")

	names := []string{"客户 订单 表", "payload'); DROP TABLE meta.users; --", "a \"quoted\" name", " 😀 emoji "}
	for _, name := range names {
		updatedBook, err := f.service.UpdateBook(f.ctx, alice, book.ID, BookPatch{Name: &name})
		if err != nil || updatedBook.Name != strings.TrimSpace(name) {
			t.Fatalf("book rename = %+v %v", updatedBook, err)
		}
		updatedTable, err := f.service.UpdateTable(f.ctx, alice, table.Table.ID, TablePatch{Name: &name})
		if err != nil || updatedTable.Name != strings.TrimSpace(name) {
			t.Fatalf("table rename = %+v %v", updatedTable, err)
		}
		updatedWorkspace, err := f.service.UpdateWorkspace(f.ctx, alice, workspace.ID, WorkspacePatch{Name: &name})
		if err != nil || updatedWorkspace.Name != strings.TrimSpace(name) {
			t.Fatalf("workspace rename = %+v %v", updatedWorkspace, err)
		}
	}
	afterSchema, afterPhysical := f.physicalNames(table.Table.ID)
	if afterSchema != schema || afterPhysical != physical {
		t.Fatalf("physical names changed: %s.%s -> %s.%s", schema, physical, afterSchema, afterPhysical)
	}
	if got := scanOne[int](f, "SELECT count(*) FROM "+schema+"."+physical); got != 1 {
		t.Fatalf("rename lost physical data: %d rows", got)
	}
	if scanOne[int](f, "SELECT count(*) FROM meta.users") < 1 {
		t.Fatal("payload text must remain inert data")
	}
	if scanOne[int](f, "SELECT schema_version FROM meta.tables WHERE id=$1", table.Table.ID) != 1 {
		t.Fatal("a table rename is not a field change and must not bump schemaVersion")
	}

	// Icon and color are independently settable and clearable.
	icon, color := "★", "#102030"
	updated, err := f.service.UpdateWorkspace(f.ctx, alice, workspace.ID, WorkspacePatch{
		Icon: OptionalText{Set: true, Value: &icon}, Color: OptionalText{Set: true, Value: &color}})
	if err != nil || updated.Icon == nil || *updated.Color != "#102030" {
		t.Fatalf("%+v %v", updated, err)
	}
	updated, err = f.service.UpdateWorkspace(f.ctx, alice, workspace.ID, WorkspacePatch{Icon: OptionalText{Set: true}})
	if err != nil || updated.Icon != nil || updated.Color == nil {
		t.Fatalf("clearing icon must keep color: %+v %v", updated, err)
	}
	_, err = f.service.UpdateWorkspace(f.ctx, alice, workspace.ID, WorkspacePatch{})
	expectError(t, err, 400, "ERR_VALIDATION")
	_, err = f.service.UpdateBook(f.ctx, alice, book.ID, BookPatch{})
	expectError(t, err, 400, "ERR_VALIDATION")
	_, err = f.service.UpdateTable(f.ctx, alice, table.Table.ID, TablePatch{Name: ptr("")})
	expectError(t, err, 400, "ERR_VALIDATION")
}

func (f *fixture) bookOrder(workspace string) []string {
	f.t.Helper()
	rows, err := f.pool.Query(f.ctx, "SELECT id::text FROM meta.books WHERE workspace_id=$1 AND deleted_at IS NULL ORDER BY position, id", workspace)
	if err != nil {
		f.t.Fatal(err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			f.t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func TestReorderIsDeterministicAndScopedToLiveSiblings(t *testing.T) {
	f := newFixture(t)
	alice := f.user("user", "active", "en")
	workspace := f.workspace(alice)
	other := f.workspace(alice)
	var ids []string
	for range 4 {
		ids = append(ids, f.book(alice, workspace.ID).ID)
	}
	foreign := f.book(alice, other.ID)
	move := func(id string, index int) {
		t.Helper()
		if _, err := f.service.UpdateBook(f.ctx, alice, id, BookPatch{Index: &index}); err != nil {
			t.Fatal(err)
		}
	}
	equal := func(got, want []string) {
		t.Helper()
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
	equal(f.bookOrder(workspace.ID), ids)
	move(ids[3], 0)
	equal(f.bookOrder(workspace.ID), []string{ids[3], ids[0], ids[1], ids[2]})
	move(ids[3], 3)
	equal(f.bookOrder(workspace.ID), []string{ids[0], ids[1], ids[2], ids[3]})
	move(ids[0], 2)
	equal(f.bookOrder(workspace.ID), []string{ids[1], ids[2], ids[0], ids[3]})
	before := scanOne[float64](f, "SELECT position FROM meta.books WHERE id=$1", ids[0])
	move(ids[0], 2)
	if after := scanOne[float64](f, "SELECT position FROM meta.books WHERE id=$1", ids[0]); after != before {
		t.Fatalf("moving to the current index changed the position %v -> %v", before, after)
	}
	for _, index := range []int{4, 99, -1} {
		_, err := f.service.UpdateBook(f.ctx, alice, ids[0], BookPatch{Index: &index})
		expectError(t, err, 400, "ERR_VALIDATION")
	}
	if scanOne[float64](f, "SELECT position FROM meta.books WHERE id=$1", foreign.ID) != 1024 {
		t.Fatal("reordering must not touch another workspace")
	}

	// Trashed siblings do not count for indexes but keep their stored position.
	if err := f.service.DeleteBook(f.ctx, alice, ids[1]); err != nil {
		t.Fatal(err)
	}
	trashedPosition := scanOne[float64](f, "SELECT position FROM meta.books WHERE id=$1", ids[1])
	move(ids[3], 0)
	equal(f.bookOrder(workspace.ID), []string{ids[3], ids[2], ids[0]})
	if scanOne[float64](f, "SELECT position FROM meta.books WHERE id=$1", ids[1]) != trashedPosition {
		t.Fatal("a trashed sibling keeps its position")
	}
	added := f.book(alice, workspace.ID)
	if scanOne[float64](f, "SELECT position FROM meta.books WHERE id=$1", added.ID) <=
		scanOne[float64](f, "SELECT max(position) FROM meta.books WHERE workspace_id=$1 AND id<>$2", workspace.ID, added.ID) {
		t.Fatal("new books go after every sibling including trashed ones")
	}

	// Equal stored positions renumber the whole live set deterministically.
	f.exec("UPDATE meta.books SET position = 5 WHERE workspace_id=$1", workspace.ID)
	live := f.bookOrder(workspace.ID)
	move(live[len(live)-1], 1)
	got := f.bookOrder(workspace.ID)
	if got[1] != live[len(live)-1] {
		t.Fatalf("tied positions must still honor the index: %v from %v", got, live)
	}
	rows, err := f.pool.Query(f.ctx, "SELECT position FROM meta.books WHERE workspace_id=$1 AND deleted_at IS NULL ORDER BY position", workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for index := 1; rows.Next(); index++ {
		var position float64
		if err := rows.Scan(&position); err != nil || position != float64(index)*1024 {
			t.Fatalf("renumbered position %d = %v (%v)", index, position, err)
		}
	}
	rows.Close()

	// Tables use the same scope rule within their book.
	book := f.book(alice, workspace.ID)
	var tables []TableSchema
	for range 3 {
		tables = append(tables, f.table(alice, book.ID))
	}
	index := 0
	if _, err := f.service.UpdateTable(f.ctx, alice, tables[2].Table.ID, TablePatch{Index: &index}); err != nil {
		t.Fatal(err)
	}
	first := scanOne[string](f, "SELECT id::text FROM meta.tables WHERE book_id=$1 ORDER BY position, id LIMIT 1", book.ID)
	if first != tables[2].Table.ID {
		t.Fatalf("table order head = %s", first)
	}
}
