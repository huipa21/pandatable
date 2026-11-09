package httpapi

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"pandatable/internal/acl"
	"pandatable/internal/events"
)

func TestBookChangedEventsRespectReadAccess(t *testing.T) {
	f := newACLFixture(t)
	owner, reader := f.user("user", false), f.user("user", false)
	pathOnly, stranger := f.user("user", false), f.user("user", false)
	bookReader, admin := f.user("user", false), f.user("admin", false)
	workspace := f.newWorkspace(owner, "Book event workspace")
	existing := f.newBook(owner, workspace.ID, "Existing")
	table := f.newTable(owner, existing.ID, "Child")
	f.grantOK(owner, acl.Workspace, workspace.ID, "user", reader.id, acl.Read)
	f.grantOK(owner, acl.Table, table.Table.ID, "user", pathOnly.id, acl.Read)
	directGrant := f.grantOK(owner, acl.Book, existing.ID, "user", bookReader.id, acl.Read)

	server, _ := realtimeServer(t, f)
	ownerSocket := dialRealtime(t, f, server, owner, server.URL)
	readerSocket := dialRealtime(t, f, server, reader, server.URL)
	pathSocket := dialRealtime(t, f, server, pathOnly, server.URL)
	strangerSocket := dialRealtime(t, f, server, stranger, server.URL)
	bookSocket := dialRealtime(t, f, server, bookReader, server.URL)
	adminSocket := dialRealtime(t, f, server, admin, server.URL)

	readNext := func(socket *websocket.Conn) events.Event {
		t.Helper()
		ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
		defer cancel()
		var event events.Event
		if err := wsjson.Read(ctx, socket, &event); err != nil {
			t.Fatal(err)
		}
		return event
	}
	assertEvents := func(bookID, description, eventID string, directRead bool) {
		t.Helper()
		authorized := []*websocket.Conn{ownerSocket, readerSocket}
		unauthorized := []*websocket.Conn{pathSocket, strangerSocket, adminSocket}
		if directRead {
			authorized = append(authorized, bookSocket)
		} else {
			unauthorized = append(unauthorized, bookSocket)
		}
		for _, socket := range authorized {
			event := readNext(socket)
			if event.Type != "book.changed" || event.BookID != bookID || event.Description == nil ||
				*event.Description != description || event.EventID != eventID {
				t.Fatalf("authorized book event = %+v", event)
			}
			nav := readNext(socket)
			if nav.Type != "nav.changed" || nav.BookID != "" || nav.Description != nil || nav.EventID != eventID {
				t.Fatalf("navigation invalidation = %+v", nav)
			}
		}
		for _, socket := range unauthorized {
			nav := readNext(socket)
			if nav.Type != "nav.changed" || nav.BookID != "" || nav.Description != nil || nav.EventID != eventID {
				t.Fatalf("book metadata leaked to an unauthorized socket: %+v", nav)
			}
			sendRealtime(t, f, socket, "ping", "")
			if next := readNext(socket); next.Type != "pong" {
				t.Fatalf("unexpected event after navigation invalidation: %+v", next)
			}
		}
	}

	created := decode[bookBody](f, f.api(owner, "POST", "/workspaces/"+workspace.ID+"/books",
		`{"name":"New","description":"  create 😀  "}`), http.StatusCreated)
	assertEvents(created.Book.ID, "create 😀", created.EventID, false)

	patched := decode[bookBody](f, f.api(owner, "PATCH", "/books/"+existing.ID,
		`{"description":"  update  "}`), http.StatusOK)
	assertEvents(existing.ID, "update", patched.EventID, true)

	renamed := decode[bookBody](f, f.api(owner, "PATCH", "/books/"+existing.ID,
		`{"name":"Renamed"}`), http.StatusOK)
	assertEvents(existing.ID, "update", renamed.EventID, true)

	cleared := decode[bookBody](f, f.api(owner, "PATCH", "/books/"+existing.ID,
		`{"description":""}`), http.StatusOK)
	assertEvents(existing.ID, "", cleared.EventID, true)

	f.expectStatus(f.api(owner, "DELETE", "/grants/"+directGrant.Grant.ID, ""), http.StatusOK, "")
	for _, socket := range []*websocket.Conn{ownerSocket, readerSocket, pathSocket, strangerSocket, bookSocket, adminSocket} {
		event := readNext(socket)
		if event.Type != "nav.changed" || event.BookID != "" || event.Description != nil {
			t.Fatalf("grant revocation invalidation = %+v", event)
		}
	}
	revoked := decode[bookBody](f, f.api(owner, "PATCH", "/books/"+existing.ID,
		`{"description":"after revocation"}`), http.StatusOK)
	assertEvents(existing.ID, "after revocation", revoked.EventID, false)

	f.expectStatus(f.api(owner, "PATCH", "/books/"+existing.ID,
		`{"name":"not committed","description":"transient","index":999}`), http.StatusBadRequest, "ERR_VALIDATION")
	for _, socket := range []*websocket.Conn{ownerSocket, readerSocket, pathSocket, strangerSocket, bookSocket, adminSocket} {
		sendRealtime(t, f, socket, "ping", "")
		if event := readNext(socket); event.Type != "pong" {
			t.Fatalf("rolled-back mutation emitted an event: %+v", event)
		}
	}
	nav := f.nav(owner)
	if len(nav.Workspaces) != 1 || len(nav.Workspaces[0].Books) != 2 ||
		nav.Workspaces[0].Books[0].Name != "Renamed" ||
		nav.Workspaces[0].Books[0].Description != "after revocation" {
		t.Fatalf("failed patch changed book metadata: %+v", nav)
	}

	secret := "must not appear in a global invalidation"
	payload := mustJSON(t, events.Event{Type: "nav.changed", BookID: existing.ID, Description: &secret})
	if _, err := f.pool.Exec(f.ctx, "SELECT pg_notify($1, $2)", events.Channel, payload); err != nil {
		t.Fatal(err)
	}
	for _, socket := range []*websocket.Conn{ownerSocket, readerSocket, pathSocket, strangerSocket, bookSocket, adminSocket} {
		event := readNext(socket)
		if event.Type != "nav.changed" || event.BookID != "" || event.Description != nil {
			t.Fatalf("global invalidation was not sanitized: %+v", event)
		}
	}
}
