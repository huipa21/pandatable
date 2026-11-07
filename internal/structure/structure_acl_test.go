package structure

import (
	"errors"
	"sync"
	"testing"

	"pandatable/internal/acl"
	"pandatable/internal/apperr"
	"pandatable/internal/ddl"
)

type tree struct {
	workspace Workspace
	book      Book
	sibling   Book
	table     TableSchema
	other     TableSchema
	alice     Actor
}

func (f *fixture) tree() tree {
	f.t.Helper()
	alice := f.user("user", "active", "en")
	workspace := f.workspace(alice)
	book := f.book(alice, workspace.ID)
	return tree{workspace: workspace, book: book, sibling: f.book(alice, workspace.ID),
		table: f.table(alice, book.ID), other: f.table(alice, book.ID), alice: alice}
}

func TestOwnerMatrixOnStructureMutations(t *testing.T) {
	f := newFixture(t)
	x := f.tree()
	rename := ptr("Renamed")

	type attempt struct {
		name string
		run  func(Actor) error
	}
	createBook := attempt{"create book", func(a Actor) error { _, err := f.service.CreateBook(f.ctx, a, x.workspace.ID, "B"); return err }}
	updateWorkspace := attempt{"update workspace", func(a Actor) error {
		_, err := f.service.UpdateWorkspace(f.ctx, a, x.workspace.ID, WorkspacePatch{Name: rename})
		return err
	}}
	updateBook := attempt{"update book", func(a Actor) error {
		_, err := f.service.UpdateBook(f.ctx, a, x.book.ID, BookPatch{Name: rename})
		return err
	}}
	createTable := attempt{"create table", func(a Actor) error { _, err := f.service.CreateTable(f.ctx, a, x.book.ID, "T"); return err }}
	updateTable := attempt{"update table", func(a Actor) error {
		_, err := f.service.UpdateTable(f.ctx, a, x.table.Table.ID, TablePatch{Name: rename})
		return err
	}}
	siblingTable := attempt{"sibling book table", func(a Actor) error { _, err := f.service.CreateTable(f.ctx, a, x.sibling.ID, "T"); return err }}
	readSchema := attempt{"schema", func(a Actor) error { _, err := f.service.Schema(f.ctx, a, x.table.Table.ID); return err }}
	all := []attempt{createBook, updateWorkspace, updateBook, createTable, updateTable, siblingTable, readSchema}

	check := func(label string, user Actor, want map[string]int) {
		t.Helper()
		for _, a := range all {
			err := a.run(user)
			status := 0
			if err != nil {
				var app *apperr.Error
				if !errors.As(err, &app) {
					t.Fatalf("%s %s: unexpected error %v", label, a.name, err)
				}
				status = app.Status
			}
			if status != want[a.name] {
				t.Fatalf("%s %s: HTTP %d, want %d (%v)", label, a.name, status, want[a.name], err)
			}
		}
	}

	stranger := f.user("user", "active", "en")
	hidden := map[string]int{}
	for _, a := range all {
		hidden[a.name] = 404
	}
	check("no grants", stranger, hidden)
	admin := f.user("admin", "active", "en")
	check("admin role", admin, hidden)

	for _, level := range []acl.Level{acl.Read, acl.Write} {
		user := f.user("user", "active", "en")
		f.grant(acl.Workspace, x.workspace.ID, "user", user.ID, level)
		check(level.String()+" on workspace", user, map[string]int{
			createBook.name: 403, updateWorkspace.name: 403, updateBook.name: 403, createTable.name: 403,
			updateTable.name: 403, siblingTable.name: 403, readSchema.name: 0})
	}

	bookOwner := f.user("user", "active", "en")
	f.grant(acl.Book, x.book.ID, "user", bookOwner.ID, acl.Owner)
	// A book owner creates tables in the book, but renaming the book itself, the
	// workspace or books elsewhere needs workspace ownership (06 action matrix).
	check("book owner", bookOwner, map[string]int{
		createBook.name: 403, updateWorkspace.name: 403, updateBook.name: 403, createTable.name: 0,
		updateTable.name: 0, siblingTable.name: 404, readSchema.name: 0})

	tableOwner := f.user("user", "active", "en")
	f.grant(acl.Table, x.table.Table.ID, "user", tableOwner.ID, acl.Owner)
	check("table owner", tableOwner, map[string]int{
		createBook.name: 403, updateWorkspace.name: 403, updateBook.name: 403, createTable.name: 403,
		updateTable.name: 403, siblingTable.name: 404, readSchema.name: 0})
	_, err := f.service.UpdateTable(f.ctx, tableOwner, x.other.Table.ID, TablePatch{Name: rename})
	expectError(t, err, 404, "ERR_NOT_FOUND")

	groupOwner := f.user("user", "active", "en")
	team := f.group()
	f.member(team, groupOwner)
	f.grant(acl.Workspace, x.workspace.ID, "group", team, acl.Owner)
	check("group workspace owner", groupOwner, map[string]int{
		createBook.name: 0, updateWorkspace.name: 0, updateBook.name: 0, createTable.name: 0,
		updateTable.name: 0, siblingTable.name: 0, readSchema.name: 0})
	f.exec("DELETE FROM meta.group_members WHERE group_id=$1", team)
	f.resolver.Invalidate()
	check("former group member", groupOwner, hidden)

	for _, text := range []string{"not-a-uuid", "", "00000000-0000-4000-8000-000000000000"} {
		_, err := f.service.CreateBook(f.ctx, x.alice, text, "B")
		expectError(t, err, 404, "ERR_NOT_FOUND")
		_, err = f.service.Schema(f.ctx, x.alice, text)
		expectError(t, err, 404, "ERR_NOT_FOUND")
	}
	if _, err := f.service.CreateWorkspace(f.ctx, admin, WorkspaceInput{Name: "Admin owned"}); err != nil {
		t.Fatalf("an administrator may create a workspace like any user: %v", err)
	}
}

func TestSoftDeleteHidesDescendantsAndRetainsPhysicalData(t *testing.T) {
	f := newFixture(t)
	x := f.tree()
	reader := f.user("user", "active", "en")
	f.grant(acl.Workspace, x.workspace.ID, "user", reader.ID, acl.Read)
	schema, physical := f.physicalNames(x.table.Table.ID)
	f.exec("INSERT INTO " + schema + "." + physical + " DEFAULT VALUES")
	fieldsBefore := scanOne[int](f, "SELECT count(*) FROM meta.fields WHERE table_id=$1", x.table.Table.ID)
	grantsBefore := scanOne[int](f, "SELECT count(*) FROM meta.grants WHERE resource_id=$1", x.workspace.ID)

	// Prime the cache, then prove the delete is visible without waiting for the TTL.
	if f.level(reader, acl.Table, x.table.Table.ID) != acl.Read {
		t.Fatal("reader should see the table")
	}
	if err := f.service.DeleteTable(f.ctx, x.alice, x.table.Table.ID); err != nil {
		t.Fatal(err)
	}
	if access, err := f.resolver.Access(f.ctx, reader.ID, acl.Resource{Type: acl.Table, ID: mustUUID(t, x.table.Table.ID)}); err != nil || access.Visible {
		t.Fatalf("cached access survived a table delete: %+v %v", access, err)
	}
	_, err := f.service.Schema(f.ctx, reader, x.table.Table.ID)
	expectError(t, err, 404, "ERR_NOT_FOUND")
	err = f.service.DeleteTable(f.ctx, x.alice, x.table.Table.ID)
	expectError(t, err, 404, "ERR_NOT_FOUND")
	_, err = f.service.UpdateTable(f.ctx, x.alice, x.table.Table.ID, TablePatch{Name: ptr("x")})
	expectError(t, err, 404, "ERR_NOT_FOUND")
	if scanOne[int](f, "SELECT count(*) FROM "+schema+"."+physical) != 1 ||
		scanOne[int](f, "SELECT count(*) FROM meta.fields WHERE table_id=$1", x.table.Table.ID) != fieldsBefore ||
		scanOne[int](f, "SELECT count(*) FROM meta.views WHERE table_id=$1", x.table.Table.ID) != 1 ||
		scanOne[int](f, "SELECT count(*) FROM meta.tables WHERE id=$1 AND deleted_at IS NOT NULL AND deleted_by=$2", x.table.Table.ID, x.alice.ID) != 1 {
		t.Fatal("soft delete must retain physical data, fields, views and authorship")
	}

	// The remaining table is unaffected; deleting its book hides it and the book.
	if _, err := f.service.Schema(f.ctx, reader, x.other.Table.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.service.DeleteBook(f.ctx, x.alice, x.book.ID); err != nil {
		t.Fatal(err)
	}
	_, err = f.service.Schema(f.ctx, x.alice, x.other.Table.ID)
	expectError(t, err, 404, "ERR_NOT_FOUND")
	_, err = f.service.CreateTable(f.ctx, x.alice, x.book.ID, "late")
	expectError(t, err, 404, "ERR_NOT_FOUND")
	_, err = f.service.UpdateBook(f.ctx, x.alice, x.book.ID, BookPatch{Name: ptr("late")})
	expectError(t, err, 404, "ERR_NOT_FOUND")
	if !f.schemaExists(schema) {
		t.Fatal("soft deletion must keep the physical schema")
	}

	if err := f.service.DeleteWorkspace(f.ctx, x.alice, x.workspace.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{x.workspace.ID} {
		_, err = f.service.CreateBook(f.ctx, x.alice, id, "late")
		expectError(t, err, 404, "ERR_NOT_FOUND")
		_, err = f.service.UpdateWorkspace(f.ctx, x.alice, id, WorkspacePatch{Name: ptr("late")})
		expectError(t, err, 404, "ERR_NOT_FOUND")
		err = f.service.DeleteWorkspace(f.ctx, x.alice, id)
		expectError(t, err, 404, "ERR_NOT_FOUND")
	}
	_, err = f.service.CreateBook(f.ctx, x.alice, x.workspace.ID, "late")
	expectError(t, err, 404, "ERR_NOT_FOUND")
	if scanOne[int](f, "SELECT count(*) FROM meta.grants WHERE resource_id=$1", x.workspace.ID) != grantsBefore {
		t.Fatal("soft deletion retains grants for the later Trash restore")
	}
	nav, err := f.service.Nav(f.ctx, x.alice)
	if err != nil || len(nav.Workspaces) != 0 {
		t.Fatalf("trashed workspace in nav: %+v %v", nav, err)
	}
}

// names scripts generated identifiers so collisions are deterministic.
type names struct {
	mu    sync.Mutex
	queue map[ddl.ObjectKind][]string
	calls map[ddl.ObjectKind]int
}

func (f *fixture) script(queue map[ddl.ObjectKind][]string) *names {
	scripted := &names{queue: queue, calls: map[ddl.ObjectKind]int{}}
	f.service.newName = func(kind ddl.ObjectKind) (string, error) {
		scripted.mu.Lock()
		defer scripted.mu.Unlock()
		scripted.calls[kind]++
		if values := scripted.queue[kind]; len(values) > 0 {
			scripted.queue[kind] = values[1:]
			return values[0], nil
		}
		return ddl.NewIdentifier(kind)
	}
	return scripted
}

func TestGeneratedCollisionsRetryWholeTransactionsAtMostThreeTimes(t *testing.T) {
	f := newFixture(t)
	x := f.tree()
	existing := scanOne[string](f, "SELECT pg_schema FROM meta.books WHERE id=$1", x.book.ID)

	// A physical schema collision, then a metadata-only reservation collision, then success.
	f.exec("INSERT INTO meta.books (workspace_id,name,pg_schema,position) VALUES ($1,'Reserved','b_aaaaaaaaaa',9)", x.workspace.ID)
	script := f.script(map[ddl.ObjectKind][]string{ddl.Book: {existing, "b_aaaaaaaaaa"}})
	book, err := f.service.CreateBook(f.ctx, x.alice, x.workspace.ID, "Retried")
	if err != nil {
		t.Fatal(err)
	}
	if script.calls[ddl.Book] != 3 {
		t.Fatalf("generated %d names, want 3 whole-transaction attempts", script.calls[ddl.Book])
	}
	if f.schemaExists("b_aaaaaaaaaa") {
		t.Fatal("the rolled-back attempt must not leave its physical schema")
	}
	if got := scanOne[string](f, "SELECT pg_schema FROM meta.books WHERE id=$1", book.ID); got == existing || got == "b_aaaaaaaaaa" {
		t.Fatalf("final schema %s reused a colliding name", got)
	}

	// Three collisions exhaust the bound: sanitized 409 and nothing persisted.
	before := scanOne[int](f, "SELECT count(*) FROM meta.books WHERE workspace_id=$1", x.workspace.ID)
	script = f.script(map[ddl.ObjectKind][]string{ddl.Book: {existing, existing, existing, existing}})
	_, err = f.service.CreateBook(f.ctx, x.alice, x.workspace.ID, "Never")
	expectError(t, err, 409, "ERR_DDL_CONFLICT")
	if script.calls[ddl.Book] != 3 {
		t.Fatalf("retry bound violated: %d attempts", script.calls[ddl.Book])
	}
	if scanOne[int](f, "SELECT count(*) FROM meta.books WHERE workspace_id=$1", x.workspace.ID) != before {
		t.Fatal("exhausted retries left metadata")
	}

	// Display names are not reservations: duplicates succeed without any retry.
	script = f.script(nil)
	for range 2 {
		if _, err := f.service.CreateBook(f.ctx, x.alice, x.workspace.ID, "Same display name"); err != nil {
			t.Fatal(err)
		}
	}
	if script.calls[ddl.Book] != 2 {
		t.Fatalf("duplicate display names must not trigger retries: %d", script.calls[ddl.Book])
	}

	// Table names: metadata reservation in the same book, then a physical-only table.
	schema, existingTable := f.physicalNames(x.table.Table.ID)
	f.exec("CREATE TABLE " + schema + ".t_zzzzzzzzzz (id bigint)")
	script = f.script(map[ddl.ObjectKind][]string{ddl.Table: {existingTable, "t_zzzzzzzzzz"}})
	created, err := f.service.CreateTable(f.ctx, x.alice, x.book.ID, "Retried table")
	if err != nil {
		t.Fatal(err)
	}
	if script.calls[ddl.Table] != 3 || script.calls[ddl.Column] != 3 {
		t.Fatalf("table/column names generated %d/%d times", script.calls[ddl.Table], script.calls[ddl.Column])
	}
	if _, physical := f.physicalNames(created.Table.ID); physical == existingTable || physical == "t_zzzzzzzzzz" {
		t.Fatalf("final table reused %s", physical)
	}
	if scanOne[int](f, "SELECT count(*) FROM meta.tables WHERE book_id=$1 AND name='Retried table'", x.book.ID) != 1 {
		t.Fatal("retries must leave exactly one table row")
	}

	// A physical table collision on every attempt persists nothing at all.
	before = scanOne[int](f, "SELECT count(*) FROM meta.tables WHERE book_id=$1", x.book.ID)
	f.script(map[ddl.ObjectKind][]string{ddl.Table: {"t_zzzzzzzzzz", "t_zzzzzzzzzz", "t_zzzzzzzzzz"}})
	_, err = f.service.CreateTable(f.ctx, x.alice, x.book.ID, "Never")
	expectError(t, err, 409, "ERR_DDL_CONFLICT")
	if scanOne[int](f, "SELECT count(*) FROM meta.tables WHERE book_id=$1", x.book.ID) != before {
		t.Fatal("exhausted table retries left metadata")
	}
}

func TestFailedDDLRollsBackMetadataAndPhysicalObjects(t *testing.T) {
	f := newFixture(t)
	x := f.tree()
	schema, _ := f.physicalNames(x.table.Table.ID)
	tablesBefore := scanOne[int](f, "SELECT count(*) FROM meta.tables WHERE book_id=$1", x.book.ID)
	physicalBefore := scanOne[int](f, "SELECT count(*) FROM pg_tables WHERE schemaname=$1", schema)
	fieldsBefore := scanOne[int](f, "SELECT count(*) FROM meta.fields f JOIN meta.tables t ON t.id=f.table_id WHERE t.book_id=$1", x.book.ID)
	viewsBefore := scanOne[int](f, "SELECT count(*) FROM meta.views v JOIN meta.tables t ON t.id=v.table_id WHERE t.book_id=$1", x.book.ID)

	// The Name column fails validation after the table metadata and CREATE TABLE ran.
	script := f.script(map[ddl.ObjectKind][]string{ddl.Column: {"c_INVALID!!"}})
	_, err := f.service.CreateTable(f.ctx, x.alice, x.book.ID, "Broken")
	expectError(t, err, 400, "ERR_VALIDATION")
	if script.calls[ddl.Table] != 1 {
		t.Fatalf("a validation failure must not be retried: %d attempts", script.calls[ddl.Table])
	}
	if scanOne[int](f, "SELECT count(*) FROM meta.tables WHERE book_id=$1", x.book.ID) != tablesBefore ||
		scanOne[int](f, "SELECT count(*) FROM pg_tables WHERE schemaname=$1", schema) != physicalBefore ||
		scanOne[int](f, "SELECT count(*) FROM meta.fields f JOIN meta.tables t ON t.id=f.table_id WHERE t.book_id=$1", x.book.ID) != fieldsBefore ||
		scanOne[int](f, "SELECT count(*) FROM meta.views v JOIN meta.tables t ON t.id=v.table_id WHERE t.book_id=$1", x.book.ID) != viewsBefore {
		t.Fatal("failed DDL left metadata or a physical table behind")
	}

	// An invalid schema name fails after nothing durable; no retry, no rows.
	books := scanOne[int](f, "SELECT count(*) FROM meta.books WHERE workspace_id=$1", x.workspace.ID)
	script = f.script(map[ddl.ObjectKind][]string{ddl.Book: {"b_bad name!"}})
	_, err = f.service.CreateBook(f.ctx, x.alice, x.workspace.ID, "Broken")
	expectError(t, err, 400, "ERR_VALIDATION")
	if script.calls[ddl.Book] != 1 || scanOne[int](f, "SELECT count(*) FROM meta.books WHERE workspace_id=$1", x.workspace.ID) != books {
		t.Fatal("invalid schema must be a single failed attempt with no metadata")
	}

	// Generation errors propagate without a success or partial write.
	f.service.newName = func(ddl.ObjectKind) (string, error) { return "", errors.New("entropy unavailable") }
	if _, err := f.service.CreateTable(f.ctx, x.alice, x.book.ID, "Broken"); err == nil || err.Error() != "entropy unavailable" {
		t.Fatalf("generation error = %v", err)
	}
	if scanOne[int](f, "SELECT count(*) FROM meta.tables WHERE book_id=$1", x.book.ID) != tablesBefore {
		t.Fatal("generation failure left a table")
	}
}

func TestNavReturnsOnlyPermittedLivePathsWithoutSiblingLeakage(t *testing.T) {
	f := newFixture(t)
	x := f.tree()
	otherWorkspace := f.workspace(x.alice)
	secretBook := f.book(x.alice, otherWorkspace.ID)
	f.table(x.alice, secretBook.ID)

	nav, err := f.service.Nav(f.ctx, x.alice)
	if err != nil || len(nav.Workspaces) < 2 {
		t.Fatalf("owner nav = %+v %v", nav, err)
	}
	var owned NavWorkspace
	for _, workspace := range nav.Workspaces {
		if workspace.ID == x.workspace.ID {
			owned = workspace
		}
	}
	if owned.Level != "owner" || owned.PathOnly || len(owned.Books) != 2 || owned.Books[0].ID != x.book.ID ||
		owned.Books[1].ID != x.sibling.ID || len(owned.Books[0].Tables) != 2 || owned.Books[0].Tables[0].Level != "owner" ||
		len(owned.Books[0].Tables[0].Views) != 1 || owned.Books[0].Tables[0].Views[0].Name != "Grid" {
		t.Fatalf("owner tree = %+v", owned)
	}

	// Table-only grant: path-only workspace/book holding exactly that table.
	description := "workspace secret"
	if _, err := f.service.UpdateWorkspace(f.ctx, x.alice, x.workspace.ID, WorkspacePatch{
		Description: &description,
	}); err != nil {
		t.Fatal(err)
	}
	carol := f.user("user", "active", "en")
	f.grant(acl.Table, x.other.Table.ID, "user", carol.ID, acl.Read)
	nav, err = f.service.Nav(f.ctx, carol)
	if err != nil || len(nav.Workspaces) != 1 {
		t.Fatalf("carol nav = %+v %v", nav, err)
	}
	w := nav.Workspaces[0]
	if !w.PathOnly || w.Level != "none" || len(w.Books) != 1 || !w.Books[0].PathOnly || w.Books[0].Level != "none" ||
		w.Description != "" || len(w.Books[0].Tables) != 1 || w.Books[0].Tables[0].ID != x.other.Table.ID ||
		w.Books[0].Tables[0].Level != "read" {
		t.Fatalf("path-only tree = %+v", w)
	}

	// Raise-only: workspace read plus a table write shows write for that table only.
	dave := f.user("user", "active", "en")
	f.grant(acl.Workspace, x.workspace.ID, "user", dave.ID, acl.Read)
	f.grant(acl.Table, x.table.Table.ID, "user", dave.ID, acl.Write)
	nav, err = f.service.Nav(f.ctx, dave)
	if err != nil || len(nav.Workspaces) != 1 || len(nav.Workspaces[0].Books) != 2 {
		t.Fatalf("dave nav = %+v %v", nav, err)
	}
	levels := map[string]string{}
	for _, book := range nav.Workspaces[0].Books {
		levels[book.ID] = book.Level
		for _, table := range book.Tables {
			levels[table.ID] = table.Level
		}
	}
	if levels[x.table.Table.ID] != "write" || levels[x.other.Table.ID] != "read" || levels[x.book.ID] != "read" {
		t.Fatalf("levels = %v", levels)
	}

	// Disabled users, admins without grants and trashed ancestors/tables get nothing.
	admin := f.user("admin", "active", "en")
	if nav, err = f.service.Nav(f.ctx, admin); err != nil || len(nav.Workspaces) != 0 || nav.Workspaces == nil {
		t.Fatalf("admin nav = %+v %v", nav, err)
	}
	f.exec("UPDATE meta.users SET status='disabled' WHERE id=$1", dave.ID)
	if nav, err = f.service.Nav(f.ctx, dave); err != nil || len(nav.Workspaces) != 0 {
		t.Fatalf("disabled nav = %+v %v", nav, err)
	}
	if err := f.service.DeleteTable(f.ctx, x.alice, x.other.Table.ID); err != nil {
		t.Fatal(err)
	}
	if nav, err = f.service.Nav(f.ctx, carol); err != nil || len(nav.Workspaces) != 0 {
		t.Fatalf("trashed table must remove its path-only ancestors: %+v %v", nav, err)
	}

	// Every nav node agrees with the resolver.
	for _, user := range []Actor{x.alice, carol} {
		nav, err := f.service.Nav(f.ctx, user)
		if err != nil {
			t.Fatal(err)
		}
		for _, workspace := range nav.Workspaces {
			if got := f.level(user, acl.Workspace, workspace.ID).String(); got != workspace.Level {
				t.Fatalf("workspace level %s vs resolver %s", workspace.Level, got)
			}
			for _, book := range workspace.Books {
				if got := f.level(user, acl.Book, book.ID).String(); got != book.Level {
					t.Fatalf("book level %s vs resolver %s", book.Level, got)
				}
				for _, table := range book.Tables {
					if got := f.level(user, acl.Table, table.ID).String(); got != table.Level {
						t.Fatalf("table level %s vs resolver %s", table.Level, got)
					}
				}
			}
		}
	}
}

func TestPersonalViewsAppearOnlyForTheirOwner(t *testing.T) {
	f := newFixture(t)
	x := f.tree()
	bob := f.user("user", "active", "en")
	f.grant(acl.Workspace, x.workspace.ID, "user", bob.ID, acl.Read)
	f.exec(`INSERT INTO meta.views (table_id,name,type,scope,owner_id,position,config)
		VALUES ($1,'Mine','grid','personal',$2,2048,'{}')`, x.table.Table.ID, bob.ID)
	for _, check := range []struct {
		user Actor
		want int
	}{{x.alice, 1}, {bob, 2}} {
		schema, err := f.service.Schema(f.ctx, check.user, x.table.Table.ID)
		if err != nil || len(schema.Views) != check.want {
			t.Fatalf("schema views = %+v %v", schema.Views, err)
		}
		nav, err := f.service.Nav(f.ctx, check.user)
		if err != nil {
			t.Fatal(err)
		}
		for _, workspace := range nav.Workspaces {
			for _, book := range workspace.Books {
				for _, table := range book.Tables {
					if table.ID == x.table.Table.ID && len(table.Views) != check.want {
						t.Fatalf("nav views = %+v", table.Views)
					}
				}
			}
		}
	}
}
