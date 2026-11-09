package httpapi

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"pandatable/internal/acl"
	"pandatable/internal/attachments"
)

func TestPrivateDefaultResourcesStayHiddenUntilGranted(t *testing.T) {
	f := newACLFixture(t)
	alice, bob, admin := f.user("user", false), f.user("user", false), f.user("admin", false)
	workspace := f.newWorkspace(alice, "Private workspace")
	book := f.newBook(alice, workspace.ID, "Private book")
	table := f.newTable(alice, book.ID, "Private table")
	siblingBook := f.newBook(alice, workspace.ID, "Private sibling book")
	siblingTable := f.newTable(alice, siblingBook.ID, "Private sibling table")
	view := table.Views[0]
	personalView := decode[viewResponse](f, f.api(alice, "POST", "/tables/"+table.Table.ID+"/views",
		`{"name":"Private personal view","scope":"personal"}`), 201).View
	row := decode[rowsResponse](f, f.api(alice, "POST", "/tables/"+table.Table.ID+"/rows",
		`{"rows":[{"fields":{}}]}`), 201).Rows[0]
	field := f.newField(alice, table.Table.ID,
		`{"name":"Private attachment","uiType":"attachment","options":{"maxFiles":2}}`)
	uploaded := attachmentUpload(f, alice, table.Table.ID, field.ID, "private.pdf",
		[]byte("%PDF-1.4\nprivate fixture\n%%EOF\n"))
	file := decode[attachments.File](f, uploaded, 201)
	commentsPath := "/tables/" + table.Table.ID + "/rows/" + row.ID + "/comments"
	f.expectStatus(f.api(alice, "POST", commentsPath, `{"body":"Private comment"}`), 201, "")
	f.expectStatus(f.api(alice, "GET", "/tables/"+table.Table.ID+"/rows/"+row.ID+"/history", ""), 200, "")
	if table.Table.DisplayFieldID == nil {
		t.Fatal("new table has no display field")
	}

	if got := f.scan(`SELECT count(*)::text FROM meta.grants WHERE resource_type='workspace' AND resource_id=$1`,
		workspace.ID); got != "1" {
		t.Fatalf("workspace has %s direct grants, want only Alice's owner grant", got)
	}
	if got := f.scan(`SELECT count(*)::text FROM meta.grants
		WHERE resource_type='workspace' AND resource_id=$1 AND principal_type='user'
		AND principal_id=$2 AND level='owner'`, workspace.ID, alice.id); got != "1" {
		t.Fatalf("Alice's workspace owner grants = %s, want 1", got)
	}
	for _, resource := range []struct {
		kind acl.ResourceType
		id   string
	}{{acl.Book, book.ID}, {acl.Book, siblingBook.ID}, {acl.Table, table.Table.ID}, {acl.Table, siblingTable.Table.ID}} {
		if got := f.scan("SELECT count(*)::text FROM meta.grants WHERE resource_type=$1 AND resource_id=$2",
			resource.kind, resource.id); got != "0" {
			t.Fatalf("%s %s has %s direct grants, want none", resource.kind, resource.id, got)
		}
	}

	exportTicket := prepareExportForTest(f, alice, view.ID, map[string]string{"format": "csv"})
	for _, principal := range []*aclUser{bob, admin} {
		if nav := f.nav(principal); len(nav.Workspaces) != 0 {
			t.Fatalf("%s navigation = %+v", principal.login, nav)
		}
		f.expectStatus(f.api(principal, "GET", "/grants?resource=workspace:"+workspace.ID, ""),
			404, "ERR_NOT_FOUND")
		for _, request := range []struct {
			method string
			path   string
			body   string
		}{
			{"PATCH", "/workspaces/" + workspace.ID, `{"name":"Should remain private"}`},
			{"PATCH", "/books/" + book.ID, `{"name":"Should remain private"}`},
			{"PATCH", "/tables/" + table.Table.ID, `{"name":"Should remain private"}`},
			{"GET", "/tables/" + table.Table.ID + "/schema", ""},
			{"POST", "/views/" + view.ID + "/query", `{}`},
			{"POST", "/views/" + personalView.ID + "/query", `{}`},
			{"GET", "/tables/" + table.Table.ID + "/rows/" + row.ID, ""},
			{"GET", commentsPath, ""},
			{"GET", "/tables/" + table.Table.ID + "/rows/" + row.ID + "/history", ""},
			{"GET", "/attachments/" + file.ID, ""},
			{"GET", "/attachments/" + file.ID + "/thumbnail", ""},
			{"POST", "/views/" + view.ID + "/export", `{"format":"csv"}`},
			{"GET", "/exports/" + exportTicket, ""},
		} {
			f.expectStatus(f.api(principal, request.method, request.path, request.body),
				404, "ERR_NOT_FOUND")
		}
	}

	assertPrivateDefaultRealtimeDenied(t, f, alice, bob, admin, table.Table.ID, row.ID,
		*table.Table.DisplayFieldID)

	f.grantOK(alice, acl.Book, book.ID, "user", bob.id, acl.Read)
	nav := f.nav(bob)
	if len(nav.Workspaces) != 1 || nav.Workspaces[0].ID != workspace.ID ||
		!nav.Workspaces[0].PathOnly || len(nav.Workspaces[0].Books) != 1 {
		t.Fatalf("book-only grant navigation = %+v", nav)
	}
	visibleBook := nav.Workspaces[0].Books[0]
	if visibleBook.ID != book.ID || visibleBook.PathOnly || len(visibleBook.Tables) != 1 {
		t.Fatalf("book-only grant book subtree = %+v", visibleBook)
	}
	visibleTable := visibleBook.Tables[0]
	if visibleTable.ID != table.Table.ID || len(visibleTable.Views) != 1 ||
		visibleTable.Views[0].ID != view.ID || visibleTable.Views[0].Scope != "shared" {
		t.Fatalf("book-only grant table views = %+v", visibleTable)
	}
	f.expectStatus(f.api(bob, "GET", "/tables/"+table.Table.ID+"/schema", ""), 200, "")
	f.expectStatus(f.api(bob, "GET", "/tables/"+siblingTable.Table.ID+"/schema", ""), 404, "ERR_NOT_FOUND")
	f.expectStatus(f.api(bob, "POST", "/views/"+personalView.ID+"/query", `{}`), 404, "ERR_NOT_FOUND")
}

func assertPrivateDefaultRealtimeDenied(t *testing.T, f *aclFixture, alice, bob, admin *aclUser,
	tableID, rowID, fieldID string,
) {
	t.Helper()
	server, _ := realtimeServer(t, f)
	aliceSocket := dialRealtime(t, f, server, alice, server.URL)
	bobSocket := dialRealtime(t, f, server, bob, server.URL)
	adminSocket := dialRealtime(t, f, server, admin, server.URL)
	sendRealtime(t, f, aliceSocket, "sub", tableID)
	readRealtime(t, f, aliceSocket, "subscribed")
	for _, socket := range []*websocket.Conn{bobSocket, adminSocket} {
		sendRealtime(t, f, socket, "sub", tableID)
		denied := readRealtime(t, f, socket, "sub.denied")
		if denied.Code != "ERR_NOT_FOUND" {
			t.Fatalf("ungranted realtime subscription = %+v", denied)
		}
	}

	f.expectStatus(f.api(alice, "PATCH", "/tables/"+tableID+"/rows/"+rowID,
		fmt.Sprintf(`{"fields":{%q:"private update"}}`, fieldID)), 200, "")
	readRealtime(t, f, aliceSocket, "row.updated")
	for _, socket := range []*websocket.Conn{bobSocket, adminSocket} {
		expectNoPrivateRealtimeEvent(t, f, socket)
	}
}

func expectNoPrivateRealtimeEvent(t *testing.T, f *aclFixture, socket *websocket.Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(f.ctx, 250*time.Millisecond)
	defer cancel()
	var frame struct {
		Type string `json:"t"`
	}
	if err := wsjson.Read(ctx, socket, &frame); err == nil {
		t.Fatalf("ungranted socket received realtime event %q", frame.Type)
	} else if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting for unauthorized realtime event: %v", err)
	}
}
