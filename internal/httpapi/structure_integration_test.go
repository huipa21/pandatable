package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"pandatable/internal/acl"
	"pandatable/internal/admin"
	"pandatable/internal/structure"
)

type workspaceBody struct {
	Workspace structure.Workspace `json:"workspace"`
	EventID   string              `json:"eventId"`
}

type bookBody struct {
	Book    structure.Book `json:"book"`
	EventID string         `json:"eventId"`
}

type tableBody struct {
	structure.TableSchema
	EventID string `json:"eventId"`
}

func decode[T any](f *aclFixture, response *httptest.ResponseRecorder, status int) T {
	f.t.Helper()
	f.expectStatus(response, status, "")
	var value T
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		f.t.Fatalf("decode %s: %v", response.Body, err)
	}
	return value
}

func (f *aclFixture) newWorkspace(user *aclUser, name string) structure.Workspace {
	f.t.Helper()
	body, _ := json.Marshal(map[string]string{"name": name})
	created := decode[workspaceBody](f, f.api(user, "POST", "/workspaces", string(body)), 201)
	if created.EventID == "" || created.Workspace.ID == "" {
		f.t.Fatalf("workspace response = %+v", created)
	}
	f.spaces = append(f.spaces, created.Workspace.ID)
	return created.Workspace
}

func (f *aclFixture) newBook(user *aclUser, workspace, name string) structure.Book {
	f.t.Helper()
	body, _ := json.Marshal(map[string]string{"name": name})
	return decode[bookBody](f, f.api(user, "POST", "/workspaces/"+workspace+"/books", string(body)), 201).Book
}

func (f *aclFixture) newTable(user *aclUser, book, name string) tableBody {
	f.t.Helper()
	body, _ := json.Marshal(map[string]string{"name": name})
	return decode[tableBody](f, f.api(user, "POST", "/books/"+book+"/tables", string(body)), 201)
}

func (f *aclFixture) nav(user *aclUser) structure.Nav {
	f.t.Helper()
	return decode[structure.Nav](f, f.api(user, "GET", "/nav", ""), 200)
}

func navTables(nav structure.Nav) map[string]structure.NavTable {
	tables := map[string]structure.NavTable{}
	for _, workspace := range nav.Workspaces {
		for _, book := range workspace.Books {
			for _, table := range book.Tables {
				tables[table.ID] = table
			}
		}
	}
	return tables
}

func TestWorkspaceDetailsAPI(t *testing.T) {
	f := newACLFixture(t)
	owner, reader := f.user("user", false), f.user("user", false)
	body, err := json.Marshal(map[string]string{
		"name": "Workspace details", "icon": "calendar", "description": "  first line\nsecond line  ",
	})
	if err != nil {
		t.Fatal(err)
	}
	created := decode[workspaceBody](f, f.api(owner, "POST", "/workspaces", string(body)), http.StatusCreated).Workspace
	f.spaces = append(f.spaces, created.ID)
	if created.Description != "first line\nsecond line" || created.Icon == nil || *created.Icon != "calendar" ||
		!isWorkspaceFallbackIcon(created.FallbackIcon) {
		t.Fatalf("workspace create = %+v", created)
	}
	f.grantOK(owner, acl.Workspace, created.ID, "user", reader.id, acl.Read)
	readerNav := f.nav(reader)
	if len(readerNav.Workspaces) != 1 || readerNav.Workspaces[0].Description != created.Description ||
		readerNav.Workspaces[0].FallbackIcon != created.FallbackIcon {
		t.Fatalf("reader nav = %+v", readerNav)
	}

	for _, invalidBody := range []string{
		`{"name":"Bad icon","icon":"not-an-icon"}`,
		`{"name":"Long description","description":"` + strings.Repeat("界", 501) + `"}`,
		`{"name":"Control character","description":"bad\tcontrol"}`,
	} {
		f.expectStatus(f.api(owner, "POST", "/workspaces", invalidBody), http.StatusBadRequest, "ERR_VALIDATION")
	}
	f.expectStatus(f.api(owner, "PATCH", "/workspaces/"+created.ID, `{"description":null}`),
		http.StatusBadRequest, "ERR_VALIDATION")

	book := f.newBook(owner, created.ID, "Path only book")
	table := f.newTable(owner, book.ID, "Path only table")
	pathOnly := f.user("user", false)
	f.grantOK(owner, acl.Table, table.Table.ID, "user", pathOnly.id, acl.Read)
	pathOnlyNav := f.nav(pathOnly)
	if len(pathOnlyNav.Workspaces) != 1 || !pathOnlyNav.Workspaces[0].PathOnly ||
		pathOnlyNav.Workspaces[0].Description != "" {
		t.Fatalf("path-only workspace nav = %+v", pathOnlyNav)
	}

	updatedBody := `{"icon":null,"description":""}`
	updated := decode[workspaceBody](f, f.api(owner, "PATCH", "/workspaces/"+created.ID, updatedBody), http.StatusOK).Workspace
	if updated.Description != "" || updated.Icon != nil || updated.FallbackIcon != created.FallbackIcon {
		t.Fatalf("workspace clear = %+v", updated)
	}
	f.expectStatus(f.api(reader, "PATCH", "/workspaces/"+created.ID, `{"description":"unauthorized"}`),
		http.StatusForbidden, "ERR_PERMISSION_DENIED")

	withoutDetails := f.newWorkspace(owner, "No details")
	if withoutDetails.Description != "" || !isWorkspaceFallbackIcon(withoutDetails.FallbackIcon) {
		t.Fatalf("workspace defaults = %+v", withoutDetails)
	}
}

func TestRecentResourcesAPIAndCallerScopedNavTimestamps(t *testing.T) {
	f := newACLFixture(t)
	owner, reader, stranger := f.user("user", false), f.user("user", false), f.user("user", false)
	workspace := f.newWorkspace(owner, "Recent workspace")
	book := f.newBook(owner, workspace.ID, "Recent book")
	table := f.newTable(owner, book.ID, "Recent table")
	f.grantOK(owner, acl.Table, table.Table.ID, "user", reader.id, acl.Read)

	for _, target := range []map[string]string{
		{"kind": "workspace", "id": workspace.ID},
		{"kind": "book", "id": book.ID},
	} {
		f.expectStatus(f.api(reader, "POST", "/recent", mustJSON(t, target)), http.StatusNoContent, "")
	}
	readerNav := f.nav(reader)
	if len(readerNav.Workspaces) != 1 || readerNav.Workspaces[0].LastOpenedAt == nil ||
		len(readerNav.Workspaces[0].Books) != 1 || readerNav.Workspaces[0].Books[0].LastOpenedAt == nil {
		t.Fatalf("reader nav recency = %+v", readerNav)
	}
	ownerNav := f.nav(owner)
	if len(ownerNav.Workspaces) != 1 || ownerNav.Workspaces[0].LastOpenedAt != nil ||
		ownerNav.Workspaces[0].Books[0].LastOpenedAt != nil {
		t.Fatalf("reader recents leaked to owner: %+v", ownerNav)
	}

	f.expectStatus(f.api(stranger, "POST", "/recent", `{"kind":"workspace","id":"`+workspace.ID+`"}`),
		http.StatusNotFound, "ERR_NOT_FOUND")
	f.expectStatus(f.api(reader, "POST", "/recent", `{"kind":"unknown","id":"`+workspace.ID+`"}`),
		http.StatusBadRequest, "ERR_VALIDATION")
	var count int
	if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM meta.recent_resources WHERE user_id=$1", stranger.id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("hidden target left %d recent rows", count)
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func isWorkspaceFallbackIcon(icon string) bool {
	switch icon {
	case "book", "table", "view", "fields", "calendar", "star", "group", "link":
		return true
	default:
		return false
	}
}

func TestStructureAPIAcceptanceAT201AT203AT213(t *testing.T) {
	f := newACLFixture(t)
	alice, carol := f.user("user", false), f.user("user", false)

	t.Run("AT-213 any user creates a workspace and owns it", func(t *testing.T) {
		workspace := f.newWorkspace(carol, "Carol W2")
		listing := decode[acl.Listing](f, f.api(carol, "GET", "/grants?resource=workspace:"+workspace.ID, ""), 200)
		if listing.EffectiveLevel != "owner" || len(listing.Items) != 1 || listing.Items[0].PrincipalID != carol.id ||
			listing.Items[0].Level != "owner" || listing.Items[0].PrincipalType != "user" {
			t.Fatalf("creator grant = %+v", listing)
		}
		if nav := f.nav(carol); len(nav.Workspaces) != 1 || nav.Workspaces[0].ID != workspace.ID || nav.Workspaces[0].Level != "owner" {
			t.Fatalf("carol nav = %+v", nav)
		}
		f.expectStatus(f.api(alice, "GET", "/grants?resource=workspace:"+workspace.ID, ""), 404, "ERR_NOT_FOUND")
	})

	workspace := f.newWorkspace(alice, "Alice 工作区")
	book := f.newBook(alice, workspace.ID, "Book 1")
	created := f.newTable(alice, book.ID, "客户 表")

	t.Run("AT-201 hierarchy, default Grid view, Name field and physical objects", func(t *testing.T) {
		nav := f.nav(alice)
		if len(nav.Workspaces) != 1 || nav.Workspaces[0].ID != workspace.ID || len(nav.Workspaces[0].Books) != 1 ||
			nav.Workspaces[0].Books[0].ID != book.ID || len(nav.Workspaces[0].Books[0].Tables) != 1 {
			t.Fatalf("sidebar tree = %+v", nav)
		}
		navTable := nav.Workspaces[0].Books[0].Tables[0]
		if navTable.ID != created.Table.ID || navTable.Name != "客户 表" || navTable.Level != "owner" ||
			len(navTable.Views) != 1 || navTable.Views[0].Name != "Grid" || navTable.Views[0].Type != "grid" {
			t.Fatalf("nav table = %+v", navTable)
		}
		schema := decode[structure.TableSchema](f, f.api(alice, "GET", "/tables/"+created.Table.ID+"/schema", ""), 200)
		if schema.MyLevel != "owner" || schema.Fields[0].Name != "Name" || schema.Fields[0].UIType != "text" || schema.Fields[0].IsSystem ||
			schema.Table.DisplayFieldID == nil || *schema.Table.DisplayFieldID != schema.Fields[0].ID || len(schema.Fields) != 6 {
			t.Fatalf("schema = %+v", schema)
		}
		var physicalSchema, physicalTable string
		if err := f.pool.QueryRow(f.ctx, `SELECT b.pg_schema, t.pg_name FROM meta.tables t JOIN meta.books b ON b.id=t.book_id
			WHERE t.id=$1`, created.Table.ID).Scan(&physicalSchema, &physicalTable); err != nil ||
			!strings.HasPrefix(physicalSchema, "b_") || !strings.HasPrefix(physicalTable, "t_") {
			t.Fatalf("physical names %s.%s %v", physicalSchema, physicalTable, err)
		}
		var exists bool
		if err := f.pool.QueryRow(f.ctx, `SELECT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname=$1 AND tablename=$2)`,
			physicalSchema, physicalTable).Scan(&exists); err != nil || !exists {
			t.Fatalf("physical table missing: %v", err)
		}
		for _, response := range []*httptest.ResponseRecorder{
			f.api(alice, "GET", "/nav", ""), f.api(alice, "GET", "/tables/"+created.Table.ID+"/schema", "")} {
			if strings.Contains(response.Body.String(), physicalSchema) || strings.Contains(response.Body.String(), physicalTable) {
				t.Fatal("responses must not leak physical identifiers")
			}
		}
	})

	t.Run("AT-202 table rename is metadata only (field rename awaits T1.14)", func(t *testing.T) {
		before := f.scan("SELECT pg_name FROM meta.tables WHERE id=$1", created.Table.ID)
		response := f.api(alice, "PATCH", "/tables/"+created.Table.ID, `{"name":"订单 \"明细\" 表"}`)
		updated := decode[struct {
			Table structure.Table `json:"table"`
		}](f, response, 200)
		if updated.Table.Name != `订单 "明细" 表` || f.scan("SELECT pg_name FROM meta.tables WHERE id=$1", created.Table.ID) != before {
			t.Fatalf("rename = %+v", updated)
		}
	})

	t.Run("AT-203 delete hides at once and retains data and metadata", func(t *testing.T) {
		extra := f.newTable(alice, book.ID, "To delete")
		bob := f.user("user", false)
		f.grantOK(alice, acl.Workspace, workspace.ID, "user", bob.id, acl.Read)
		if _, ok := navTables(f.nav(bob))[extra.Table.ID]; !ok {
			t.Fatal("reader should see the table")
		}
		f.expectStatus(f.api(bob, "GET", "/tables/"+extra.Table.ID+"/schema", ""), 200, "")
		f.expectStatus(f.api(bob, "DELETE", "/tables/"+extra.Table.ID, ""), 403, "ERR_PERMISSION_DENIED")
		deleted := f.api(alice, "DELETE", "/tables/"+extra.Table.ID, "")
		f.expectStatus(deleted, 200, "")
		if !strings.Contains(deleted.Body.String(), "eventId") {
			t.Fatalf("delete response = %s", deleted.Body)
		}
		for _, user := range []*aclUser{alice, bob} {
			if _, ok := navTables(f.nav(user))[extra.Table.ID]; ok {
				t.Fatal("deleted table still in the sidebar")
			}
			f.expectStatus(f.api(user, "GET", "/tables/"+extra.Table.ID+"/schema", ""), 404, "ERR_NOT_FOUND")
		}
		f.expectStatus(f.api(alice, "DELETE", "/tables/"+extra.Table.ID, ""), 404, "ERR_NOT_FOUND")
		var kept int
		if err := f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM meta.fields WHERE table_id=$1) +
			(SELECT count(*) FROM meta.views WHERE table_id=$1) + (SELECT count(*) FROM meta.tables WHERE id=$1 AND deleted_at IS NOT NULL)`,
			extra.Table.ID).Scan(&kept); err != nil || kept != 6+1+1 {
			t.Fatalf("retained metadata rows = %d (%v)", kept, err)
		}
	})
}

func TestStructureAPIAdminHasNoImplicitAccessAT112(t *testing.T) {
	f := newACLFixture(t)
	alice, admin1 := f.user("user", false), f.user("admin", false)
	workspace := f.newWorkspace(alice, "Admin cannot see")
	book := f.newBook(alice, workspace.ID, "B")
	table := f.newTable(alice, book.ID, "T")

	adminService, err := admin.NewService(f.pool, f.authService, f.resolver)
	if err != nil {
		t.Fatal(err)
	}
	structureService, err := structure.New(f.pool, f.resolver)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := New(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), testAssets(), AuthOptions{Service: f.authService,
		ACL: f.aclService, Structure: structureService, Admin: adminService, BaseURL: "https://table.example"})
	if err != nil {
		t.Fatal(err)
	}
	call := func(user *aclUser, method, path string) *httptest.ResponseRecorder {
		return f.call(handler, user, method, "/api/v1"+path, "")
	}
	if nav := decode[structure.Nav](f, call(admin1, "GET", "/nav"), 200); len(nav.Workspaces) != 0 || nav.Workspaces == nil {
		t.Fatalf("admin sidebar = %+v", nav)
	}
	for _, path := range []string{"/tables/" + table.Table.ID + "/schema", "/grants?resource=workspace:" + workspace.ID} {
		f.expectStatus(call(admin1, "GET", path), 404, "ERR_NOT_FOUND")
	}
	f.expectStatus(f.call(handler, admin1, "PATCH", "/api/v1/workspaces/"+workspace.ID, `{"name":"x"}`), 404, "ERR_NOT_FOUND")
	f.expectStatus(f.call(handler, admin1, "DELETE", "/api/v1/tables/"+table.Table.ID, ""), 404, "ERR_NOT_FOUND")

	// The read-only inventory lists the real workspace with its real direct owner,
	// independently of resource access.
	inventory := decode[struct {
		Items []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Owners []struct {
				ID   string `json:"id"`
				Type string `json:"type"`
			} `json:"owners"`
			Orphaned bool `json:"orphaned"`
		} `json:"items"`
	}](f, call(admin1, "GET", "/admin/workspaces?q=Admin+cannot+see"), 200)
	found := false
	for _, item := range inventory.Items {
		if item.ID == workspace.ID {
			found = true
			if item.Name != workspace.Name || item.Orphaned || len(item.Owners) != 1 || item.Owners[0].ID != alice.id || item.Owners[0].Type != "user" {
				t.Fatalf("inventory item = %+v", item)
			}
		}
	}
	if !found {
		t.Fatalf("inventory lacks the workspace: %+v", inventory)
	}
	// Non-admins stay out of the admin namespace even as owners.
	f.expectStatus(call(alice, "GET", "/admin/workspaces"), 403, "ERR_PERMISSION_DENIED")
}

func TestStructureAPIPermissionMatrixOverHTTP(t *testing.T) {
	f := newACLFixture(t)
	alice := f.user("user", false)
	workspace := f.newWorkspace(alice, "Matrix")
	book := f.newBook(alice, workspace.ID, "Book")
	sibling := f.newBook(alice, workspace.ID, "Sibling")
	table := f.newTable(alice, book.ID, "Table")
	other := f.newTable(alice, book.ID, "Other")

	reader, writer, bookOwner, tableOnly, stranger, admin1 :=
		f.user("user", false), f.user("user", false), f.user("user", false), f.user("user", false), f.user("user", false), f.user("admin", false)
	f.grantOK(alice, acl.Workspace, workspace.ID, "user", reader.id, acl.Read)
	f.grantOK(alice, acl.Workspace, workspace.ID, "user", writer.id, acl.Write)
	f.grantOK(alice, acl.Book, book.ID, "user", bookOwner.id, acl.Owner)
	f.grantOK(alice, acl.Table, table.Table.ID, "user", tableOnly.id, acl.Read)

	type request struct{ method, path, body string }
	createBook := request{"POST", "/workspaces/" + workspace.ID + "/books", `{"name":"x"}`}
	patchWorkspace := request{"PATCH", "/workspaces/" + workspace.ID, `{"name":"x"}`}
	deleteWorkspace := request{"DELETE", "/workspaces/" + workspace.ID, ""}
	patchBook := request{"PATCH", "/books/" + book.ID, `{"name":"x"}`}
	deleteSibling := request{"DELETE", "/books/" + sibling.ID, ""}
	createTable := request{"POST", "/books/" + book.ID + "/tables", `{"name":"x"}`}
	createSiblingTable := request{"POST", "/books/" + sibling.ID + "/tables", `{"name":"x"}`}
	patchTable := request{"PATCH", "/tables/" + other.Table.ID, `{"name":"x"}`}
	schemaOther := request{"GET", "/tables/" + other.Table.ID + "/schema", ""}
	schemaTable := request{"GET", "/tables/" + table.Table.ID + "/schema", ""}

	expect := func(label string, user *aclUser, want map[request]int) {
		t.Helper()
		for call, status := range want {
			response := f.api(user, call.method, call.path, call.body)
			if response.Code != status {
				t.Fatalf("%s %s %s: HTTP %d, want %d: %s", label, call.method, call.path, response.Code, status, response.Body)
			}
		}
	}
	// Mutating calls that would succeed are excluded from the "denied" users; each
	// user is checked on every denied request and on schema reads.
	denied := func(status int, calls ...request) map[request]int {
		result := map[request]int{}
		for _, call := range calls {
			result[call] = status
		}
		return result
	}
	allWrites := []request{createBook, patchWorkspace, deleteWorkspace, patchBook, deleteSibling, createTable, createSiblingTable, patchTable}

	expect("stranger", stranger, denied(404, append(allWrites, schemaOther, schemaTable)...))
	expect("admin role", admin1, denied(404, append(allWrites, schemaOther, schemaTable)...))
	for label, user := range map[string]*aclUser{"reader": reader, "writer": writer} {
		expect(label, user, denied(403, allWrites...))
		expect(label, user, denied(200, schemaOther, schemaTable))
	}
	// A table-only reader sees the table but only that table, and cannot change anything.
	expect("table-only", tableOnly, denied(403, createBook, patchWorkspace, deleteWorkspace, patchBook, createTable))
	expect("table-only", tableOnly, denied(404, deleteSibling, createSiblingTable, schemaOther, patchTable))
	expect("table-only", tableOnly, denied(200, schemaTable))
	// A book owner manages tables in its book but not the book/workspace/siblings.
	expect("book owner", bookOwner, denied(403, createBook, patchWorkspace, deleteWorkspace, patchBook))
	expect("book owner", bookOwner, denied(404, deleteSibling, createSiblingTable))
	expect("book owner", bookOwner, denied(200, patchTable, schemaOther, schemaTable))
	expect("book owner", bookOwner, denied(201, createTable))

	// Group-derived ownership is honored.
	group := f.scan("INSERT INTO meta.groups (name) VALUES ($1) RETURNING id::text", "owners-"+strings.ToLower(f.scan("SELECT gen_random_uuid()::text"))[:8])
	f.groups = append(f.groups, group)
	member := f.user("user", false)
	f.exec("INSERT INTO meta.group_members (group_id,user_id) VALUES ($1,$2)", group, member.id)
	f.grantOK(alice, acl.Book, sibling.ID, "group", group, acl.Owner)
	expect("group book owner", member, denied(201, createSiblingTable))
	expect("group book owner", member, denied(403, patchWorkspace, deleteSibling))
	expect("group book owner", member, denied(404, createTable))
}

func TestStructureAPIAuthenticationCSRFAndBoundsClasses(t *testing.T) {
	f := newACLFixture(t)
	alice, forced, disabled := f.user("user", false), f.user("user", true), f.user("user", false)
	workspace := f.newWorkspace(alice, "Guards")
	book := f.newBook(alice, workspace.ID, "B")
	table := f.newTable(alice, book.ID, "T")
	f.exec("UPDATE meta.users SET status='disabled' WHERE id=$1", disabled.id)

	routes := []struct{ method, path, body string }{
		{"GET", "/nav", ""},
		{"POST", "/workspaces", `{"name":"x"}`},
		{"PATCH", "/workspaces/" + workspace.ID, `{"name":"x"}`},
		{"DELETE", "/workspaces/" + workspace.ID, ""},
		{"POST", "/workspaces/" + workspace.ID + "/books", `{"name":"x"}`},
		{"PATCH", "/books/" + book.ID, `{"name":"x"}`},
		{"DELETE", "/books/" + book.ID, ""},
		{"POST", "/books/" + book.ID + "/tables", `{"name":"x"}`},
		{"PATCH", "/tables/" + table.Table.ID, `{"name":"x"}`},
		{"DELETE", "/tables/" + table.Table.ID, ""},
		{"GET", "/tables/" + table.Table.ID + "/schema", ""},
	}
	workspaces := f.scan("SELECT count(*)::text FROM meta.workspaces")
	for _, route := range routes {
		label := route.method + " " + route.path
		f.expectStatus(f.api(nil, route.method, route.path, route.body), 401, "ERR_UNAUTHENTICATED")
		f.expectStatus(f.api(disabled, route.method, route.path, route.body), 401, "ERR_UNAUTHENTICATED")
		f.expectStatus(f.api(forced, route.method, route.path, route.body), 403, "ERR_PASSWORD_CHANGE_REQUIRED")
		if route.method != "GET" {
			// Missing/foreign Origin or CSRF header is rejected before any handler runs.
			for name, mutate := range map[string]func(*http.Request){
				"no csrf":      func(r *http.Request) { r.Header.Del("X-PT-CSRF") },
				"bad csrf":     func(r *http.Request) { r.Header.Set("X-PT-CSRF", "0") },
				"no origin":    func(r *http.Request) { r.Header.Del("Origin") },
				"other origin": func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") },
			} {
				request := httptest.NewRequest(route.method, "/api/v1"+route.path, strings.NewReader(route.body))
				request.RemoteAddr = "192.0.2.250:5000"
				f.decorate(request, alice)
				mutate(request)
				response := httptest.NewRecorder()
				f.handler.ServeHTTP(response, request)
				if response.Code != 403 || !strings.Contains(response.Body.String(), "ERR_CSRF") {
					t.Fatalf("%s %s: HTTP %d %s", label, name, response.Code, response.Body)
				}
			}
		}
	}
	if f.scan("SELECT count(*)::text FROM meta.workspaces") != workspaces {
		t.Fatal("rejected requests changed state")
	}
	if got := f.scan("SELECT name FROM meta.workspaces WHERE id=$1", workspace.ID); got != "Guards" {
		t.Fatalf("rejected requests renamed the workspace: %s", got)
	}

	// Strict bounded JSON, content type, query and method handling on a valid session.
	for label, test := range map[string]struct {
		method, path, body string
		status             int
		code               string
		contentType        string
	}{
		"unknown field":      {"POST", "/workspaces", `{"name":"x","owner":"y"}`, 400, "ERR_VALIDATION", "application/json"},
		"malformed":          {"POST", "/workspaces", `{"name":`, 400, "ERR_VALIDATION", "application/json"},
		"two values":         {"POST", "/workspaces", `{"name":"x"}{"name":"y"}`, 400, "ERR_VALIDATION", "application/json"},
		"too large":          {"POST", "/workspaces", `{"name":"` + strings.Repeat("a", 5000) + `"}`, 400, "ERR_VALIDATION", "application/json"},
		"name too long":      {"POST", "/workspaces", `{"name":"` + strings.Repeat("a", 101) + `"}`, 400, "ERR_VALIDATION", "application/json"},
		"missing name":       {"POST", "/workspaces", `{}`, 400, "ERR_VALIDATION", "application/json"},
		"non-string name":    {"POST", "/workspaces", `{"name":7}`, 400, "ERR_VALIDATION", "application/json"},
		"bad color":          {"POST", "/workspaces", `{"name":"x","color":"red"}`, 400, "ERR_VALIDATION", "application/json"},
		"color object":       {"POST", "/workspaces", `{"name":"x","color":{}}`, 400, "ERR_VALIDATION", "application/json"},
		"wrong content type": {"POST", "/workspaces", `{"name":"x"}`, 415, "ERR_CONTENT_TYPE", "text/plain"},
		"empty patch":        {"PATCH", "/workspaces/" + workspace.ID, `{}`, 400, "ERR_VALIDATION", "application/json"},
		"fractional index":   {"PATCH", "/books/" + book.ID, `{"index":1.5}`, 400, "ERR_VALIDATION", "application/json"},
		"negative index":     {"PATCH", "/books/" + book.ID, `{"index":-1}`, 400, "ERR_VALIDATION", "application/json"},
		"index out of range": {"PATCH", "/books/" + book.ID, `{"index":5}`, 400, "ERR_VALIDATION", "application/json"},
		"table fields":       {"POST", "/books/" + book.ID + "/tables", `{"name":"x","fields":[]}`, 400, "ERR_VALIDATION", "application/json"},
		"display field":      {"PATCH", "/tables/" + table.Table.ID, `{"displayFieldId":"x"}`, 400, "ERR_VALIDATION", "application/json"},
		"nav query":          {"GET", "/nav?x=1", "", 400, "ERR_VALIDATION", ""},
		"schema query":       {"GET", "/tables/" + table.Table.ID + "/schema?x=1", "", 400, "ERR_VALIDATION", ""},
		"bad id":             {"PATCH", "/tables/not-a-uuid", `{"name":"x"}`, 404, "ERR_NOT_FOUND", "application/json"},
		"bad id schema":      {"GET", "/tables/not-a-uuid/schema", "", 404, "ERR_NOT_FOUND", ""},
		"put method":         {"PUT", "/workspaces/" + workspace.ID, `{}`, 405, "ERR_METHOD_NOT_ALLOWED", "application/json"},
		"post nav":           {"POST", "/nav", `{}`, 405, "ERR_METHOD_NOT_ALLOWED", "application/json"},
	} {
		request := httptest.NewRequest(test.method, "/api/v1"+test.path, strings.NewReader(test.body))
		request.RemoteAddr = "192.0.2.250:5000"
		f.decorate(request, alice)
		if test.contentType != "" {
			request.Header.Set("Content-Type", test.contentType)
		}
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, request)
		f.expectStatus(response, test.status, test.code)
		_ = label
	}

	// Successful responses are no-store JSON carrying an eventId.
	response := f.api(alice, "PATCH", "/books/"+book.ID, `{"name":"Renamed","index":0}`)
	f.expectStatus(response, 200, "")
	if response.Header().Get("Content-Type") != "application/json" || !strings.Contains(response.Body.String(), `"eventId"`) {
		t.Fatalf("response = %v %s", response.Header(), response.Body)
	}
	for _, path := range []string{"/nav", "/tables/" + table.Table.ID + "/schema"} {
		if response := f.api(alice, "GET", path, ""); response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s must be no-store", path)
		}
	}
}

func TestStructureAPIReorderNameSemanticsAndSanitizedErrors(t *testing.T) {
	f := newACLFixture(t)
	alice := f.user("user", false)
	workspace := f.newWorkspace(alice, "Reorder")
	var books []structure.Book
	for index := range 3 {
		books = append(books, f.newBook(alice, workspace.ID, fmt.Sprintf("Book %d", index)))
	}
	response := f.api(alice, "PATCH", "/books/"+books[2].ID, `{"index":0}`)
	f.expectStatus(response, 200, "")
	nav := f.nav(alice)
	got := nav.Workspaces[0].Books
	if len(got) != 3 || got[0].ID != books[2].ID || got[1].ID != books[0].ID || got[2].ID != books[1].ID {
		t.Fatalf("book order = %+v", got)
	}
	// Duplicate display names are legal and independent of physical names.
	body := `{"name":"Book 0"}`
	duplicate := decode[bookBody](f, f.api(alice, "POST", "/workspaces/"+workspace.ID+"/books", body), 201)
	if duplicate.Book.Name != "Book 0" || duplicate.Book.ID == books[0].ID {
		t.Fatalf("duplicate = %+v", duplicate)
	}
	// Soft-deleted parents behave as missing for every child route.
	f.expectStatus(f.api(alice, "DELETE", "/workspaces/"+workspace.ID, ""), 200, "")
	for _, call := range [][3]string{
		{"POST", "/workspaces/" + workspace.ID + "/books", `{"name":"x"}`},
		{"PATCH", "/books/" + books[0].ID, `{"name":"x"}`},
		{"POST", "/books/" + books[0].ID + "/tables", `{"name":"x"}`},
		{"DELETE", "/workspaces/" + workspace.ID, ""},
	} {
		f.expectStatus(f.api(alice, call[0], call[1], call[2]), 404, "ERR_NOT_FOUND")
	}
	if nav := f.nav(alice); len(nav.Workspaces) != 0 {
		t.Fatalf("trashed workspace in nav: %+v", nav)
	}
}

func TestStructureAPIConcurrentCreationsAcrossUsers(t *testing.T) {
	f := newACLFixture(t)
	alice := f.user("user", false)
	workspace := f.newWorkspace(alice, "Concurrent")
	book := f.newBook(alice, workspace.ID, "B")
	var wait sync.WaitGroup
	statuses := make([]int, 8)
	for index := range statuses {
		wait.Add(1)
		go func() {
			defer wait.Done()
			path, body := "/books/"+book.ID+"/tables", `{"name":"Parallel"}`
			if index%2 == 1 {
				path, body = "/workspaces/"+workspace.ID+"/books", `{"name":"Parallel book"}`
			}
			statuses[index] = f.api(alice, "POST", path, body).Code
		}()
	}
	wait.Wait()
	for index, status := range statuses {
		if status != 201 {
			t.Fatalf("request %d returned %d", index, status)
		}
	}
	if got := f.scan("SELECT count(*)::text FROM meta.tables WHERE book_id=$1", book.ID); got != "4" {
		t.Fatalf("%s tables created, want 4", got)
	}
}
