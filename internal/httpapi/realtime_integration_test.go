package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"pandatable/internal/acl"
	"pandatable/internal/admin"
	"pandatable/internal/events"
	"pandatable/internal/realtime"
	"pandatable/internal/structure"
)

func realtimeServer(t *testing.T, f *aclFixture) (*httptest.Server, *realtime.Hub) {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	hub, err := realtime.New(f.pool, f.resolver, f.authService, logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := hub.Start(f.ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(hub.Close)
	structureService, err := structure.New(f.pool, f.resolver)
	if err != nil {
		t.Fatal(err)
	}
	adminService, err := admin.NewService(f.pool, f.authService, f.resolver)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(nil)
	origin := "http://" + server.Listener.Addr().String()
	handler, err := New(logger, testAssets(), AuthOptions{
		Service: f.authService, ACL: f.aclService, Structure: structureService,
		Admin: adminService, Realtime: hub, BaseURL: origin,
	})
	if err != nil {
		t.Fatal(err)
	}
	server.Config.Handler = handler
	server.Start()
	t.Cleanup(server.Close)
	return server, hub
}

func dialRealtime(t *testing.T, f *aclFixture, server *httptest.Server, user *aclUser, origin string) *websocket.Conn {
	t.Helper()
	header := http.Header{"Origin": []string{origin}}
	if user != nil {
		header.Set("Cookie", user.cookie.Name+"="+user.cookie.Value)
	}
	conn, response, err := websocket.Dial(f.ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/api/v1/ws",
		&websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		if response != nil {
			t.Fatalf("dial WebSocket: HTTP %d: %v", response.StatusCode, err)
		}
		t.Fatalf("dial WebSocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	return conn
}

func sendRealtime(t *testing.T, f *aclFixture, conn *websocket.Conn, kind, tableID string) {
	t.Helper()
	if err := wsjson.Write(f.ctx, conn, map[string]string{"t": kind, "tableId": tableID}); err != nil {
		t.Fatal(err)
	}
}

func readRealtime(t *testing.T, f *aclFixture, conn *websocket.Conn, kind string) events.Event {
	t.Helper()
	ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
	defer cancel()
	for {
		var event events.Event
		if err := wsjson.Read(ctx, conn, &event); err != nil {
			t.Fatalf("read %s: %v", kind, err)
		}
		if event.Type == kind {
			return event
		}
	}
}

func TestRealtimeProtocolTransactionsACLAndRevocation(t *testing.T) {
	f := newACLFixture(t)
	owner, bob, stranger := f.user("admin", false), f.user("user", false), f.user("user", false)
	workspace := f.newWorkspace(owner, "Realtime")
	book := f.newBook(owner, workspace.ID, "Book")
	table := f.newTable(owner, book.ID, "Rows")
	f.grantOK(owner, acl.Table, table.Table.ID, "user", bob.id, acl.Write)
	fieldID := table.Fields[0].ID
	server, _ := realtimeServer(t, f)
	bobSocket := dialRealtime(t, f, server, bob, server.URL)
	strangerSocket := dialRealtime(t, f, server, stranger, server.URL)

	sendRealtime(t, f, strangerSocket, "sub", table.Table.ID)
	denied := readRealtime(t, f, strangerSocket, "sub.denied")
	if denied.Code != "ERR_NOT_FOUND" || denied.TableID != table.Table.ID {
		t.Fatalf("hidden subscription was not denied: %+v", denied)
	}
	sendRealtime(t, f, bobSocket, "sub", table.Table.ID)
	if ack := readRealtime(t, f, bobSocket, "subscribed"); ack.TableID != table.Table.ID {
		t.Fatalf("subscription ack = %+v", ack)
	}

	created := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+table.Table.ID+"/rows",
		fmt.Sprintf(`{"rows":[{"fields":{"%s":"initial"}}]}`, fieldID)), 201)
	event := readRealtime(t, f, bobSocket, "row.created")
	if len(event.IDs) != 1 || event.IDs[0] != created.Rows[0].ID ||
		event.EventID != created.EventID || event.By != owner.id || event.ByName != owner.login {
		t.Fatalf("row-created event = %+v; response = %+v", event, created)
	}

	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	rolledBack := events.WithID(f.ctx, "rollback_event_123456")
	if err := events.Emit(rolledBack, tx, events.Event{Type: "row.updated", TableID: table.Table.ID, IDs: []string{"1"}}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(f.ctx); err != nil {
		t.Fatal(err)
	}
	patch := decode[rowResponse](f, f.api(owner, "PATCH", "/tables/"+table.Table.ID+"/rows/"+created.Rows[0].ID,
		fmt.Sprintf(`{"fields":{"%s":"changed"}}`, fieldID)), 200)
	event = readRealtime(t, f, bobSocket, "row.updated")
	if event.EventID != patch.EventID || len(event.Fields) != 1 || event.Fields[0] != fieldID {
		t.Fatalf("rollback leaked or patch event differs: %+v", event)
	}
	query := decode[structure.RowQueryResult](f, f.api(bob, "POST", "/views/"+table.Views[0].ID+"/query",
		fmt.Sprintf(`{"ids":[%q],"limit":1,"includeCount":false}`, created.Rows[0].ID)), 200)
	if len(query.Rows) != 1 || string(query.Rows[0].Fields[fieldID]) != `"changed"` {
		t.Fatalf("batched row query = %+v", query)
	}
	f.expectStatus(f.api(bob, "POST", "/views/"+table.Views[0].ID+"/query",
		fmt.Sprintf(`{"ids":[%q,%q]}`, created.Rows[0].ID, created.Rows[0].ID)), 400, "ERR_VALIDATION")
	bulkBody := `{"rows":[` + strings.Repeat(`{"fields":{}},`, 500) + `{"fields":{}}]}`
	decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+table.Table.ID+"/rows", bulkBody), 201)
	bulk := readRealtime(t, f, bobSocket, "row.bulk")
	if bulk.TableID != table.Table.ID || len(bulk.IDs) != 0 {
		t.Fatalf("bulk notification = %+v", bulk)
	}
	requestWithID := func(ids ...string) *httptest.ResponseRecorder {
		request := httptest.NewRequest("POST", "/api/v1/tables/"+table.Table.ID+"/rows",
			strings.NewReader(`{"rows":[{"fields":{}}]}`))
		request.RemoteAddr = "192.0.2.250:5000"
		f.decorate(request, owner)
		for _, id := range ids {
			request.Header.Add("X-PT-Event-ID", id)
		}
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, request)
		return response
	}
	for _, invalid := range [][]string{{"short"}, {strings.Repeat("a", 65)}, {"0123456789abcdef", "fedcba9876543210"}} {
		f.expectStatus(requestWithID(invalid...), 400, "ERR_VALIDATION")
	}
	const clientID = "client_event_1234567890"
	createdWithID := decode[rowsResponse](f, requestWithID(clientID), 201)
	if event := readRealtime(t, f, bobSocket, "row.created"); event.EventID != clientID || createdWithID.EventID != clientID {
		t.Fatalf("preallocated event ID mismatch: event=%+v response=%+v", event, createdWithID)
	}

	f.newField(owner, table.Table.ID, `{"name":"Remote field","uiType":"text"}`)
	schema := readRealtime(t, f, bobSocket, "schema.changed")
	if schema.TableID != table.Table.ID || schema.SchemaVersion <= table.SchemaVersion {
		t.Fatalf("schema event = %+v", schema)
	}
	sendRealtime(t, f, bobSocket, "unsub", table.Table.ID)
	sendRealtime(t, f, bobSocket, "ping", "")
	readRealtime(t, f, bobSocket, "pong")
	sendRealtime(t, f, bobSocket, "sub", table.Table.ID)
	readRealtime(t, f, bobSocket, "subscribed")

	started := time.Now()
	f.expectStatus(f.api(owner, "PATCH", "/admin/users/"+bob.id, `{"status":"disabled"}`), 200, "")
	readRealtime(t, f, bobSocket, "session.revoked")
	if time.Since(started) >= 5*time.Second {
		t.Fatal("session revocation exceeded five seconds")
	}
	f.expectStatus(f.api(bob, "GET", "/auth/me", ""), 401, "ERR_UNAUTHENTICATED")
}

func TestRealtimeRejectsCrossOriginAndUnauthenticatedSockets(t *testing.T) {
	f := newACLFixture(t)
	user := f.user("user", false)
	server, _ := realtimeServer(t, f)
	for _, test := range []struct {
		name   string
		origin string
		user   *aclUser
		status int
	}{
		{"missing session", server.URL, nil, http.StatusUnauthorized},
		{"foreign origin", "https://attacker.example", user, http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			headers := http.Header{"Origin": []string{test.origin}}
			if test.user != nil {
				headers.Set("Cookie", test.user.cookie.Name+"="+test.user.cookie.Value)
			}
			conn, response, err := websocket.Dial(f.ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/api/v1/ws",
				&websocket.DialOptions{HTTPHeader: headers})
			if conn != nil {
				_ = conn.CloseNow()
			}
			if err == nil || response == nil || response.StatusCode != test.status {
				t.Fatalf("upgrade response = %v, error %v; want %d", response, err, test.status)
			}
		})
	}
}

func TestRealtimePersonalViewIsolationAndGlobalNavigation(t *testing.T) {
	f := newACLFixture(t)
	owner, writer := f.user("admin", false), f.user("user", false)
	workspace := f.newWorkspace(owner, "Views")
	book := f.newBook(owner, workspace.ID, "Book")
	table := f.newTable(owner, book.ID, "Rows")
	f.grantOK(owner, acl.Table, table.Table.ID, "user", writer.id, acl.Write)
	server, _ := realtimeServer(t, f)
	ownerSocket := dialRealtime(t, f, server, owner, server.URL)
	writerSocket := dialRealtime(t, f, server, writer, server.URL)
	for _, conn := range []*websocket.Conn{ownerSocket, writerSocket} {
		sendRealtime(t, f, conn, "sub", table.Table.ID)
		readRealtime(t, f, conn, "subscribed")
	}

	f.expectStatus(f.api(writer, "POST", "/tables/"+table.Table.ID+"/views",
		`{"name":"Private","scope":"personal"}`), 201, "")
	if event := readRealtime(t, f, writerSocket, "view.changed"); event.ViewID == "" {
		t.Fatalf("personal view event = %+v", event)
	}
	readRealtime(t, f, writerSocket, "nav.changed")
	sendRealtime(t, f, ownerSocket, "ping", "")
	var next events.Event
	ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
	defer cancel()
	if err := wsjson.Read(ctx, ownerSocket, &next); err != nil || next.Type != "pong" {
		t.Fatalf("personal view leaked to another user: %+v, %v", next, err)
	}

	sendRealtime(t, f, ownerSocket, "unsub", table.Table.ID)
	sendRealtime(t, f, ownerSocket, "ping", "")
	readRealtime(t, f, ownerSocket, "pong")
	f.expectStatus(f.api(writer, "POST", "/tables/"+table.Table.ID+"/views",
		`{"name":"Shared","scope":"shared"}`), 201, "")
	readRealtime(t, f, writerSocket, "view.changed")
	if event := readRealtime(t, f, ownerSocket, "nav.changed"); event.TableID != "" || event.ViewID != "" {
		t.Fatalf("global navigation event exposed resource IDs: %+v", event)
	}
}

func TestRealtimeListenerDisconnectsAndRecovers(t *testing.T) {
	f := newACLFixture(t)
	owner := f.user("admin", false)
	workspace := f.newWorkspace(owner, "Recovery")
	book := f.newBook(owner, workspace.ID, "Book")
	table := f.newTable(owner, book.ID, "Rows")
	server, _ := realtimeServer(t, f)
	socket := dialRealtime(t, f, server, owner, server.URL)
	sendRealtime(t, f, socket, "sub", table.Table.ID)
	readRealtime(t, f, socket, "subscribed")

	var listenerPID int
	if err := f.pool.QueryRow(f.ctx, `SELECT pid FROM pg_stat_activity
		WHERE datname = current_database() AND query = $1 AND pid <> pg_backend_pid()`,
		"LISTEN "+events.Channel).Scan(&listenerPID); err != nil {
		t.Fatal(err)
	}
	var terminated bool
	if err := f.pool.QueryRow(f.ctx, `SELECT pg_terminate_backend($1)`, listenerPID).Scan(&terminated); err != nil || !terminated {
		t.Fatalf("terminate test listener: %v, terminated=%t", err, terminated)
	}
	readCtx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
	defer cancel()
	var event events.Event
	if err := wsjson.Read(readCtx, socket, &event); err == nil || readCtx.Err() != nil {
		t.Fatalf("socket did not close promptly after listener loss: event=%+v err=%v", event, err)
	}

	reconnectCtx, stop := context.WithTimeout(f.ctx, 10*time.Second)
	defer stop()
	headers := http.Header{"Origin": []string{server.URL}}
	headers.Set("Cookie", owner.cookie.Name+"="+owner.cookie.Value)
	var reconnected *websocket.Conn
	for reconnected == nil {
		var response *http.Response
		var err error
		reconnected, response, err = websocket.Dial(reconnectCtx,
			"ws"+strings.TrimPrefix(server.URL, "http")+"/api/v1/ws", &websocket.DialOptions{HTTPHeader: headers})
		if err == nil {
			break
		}
		if reconnectCtx.Err() != nil || (response != nil && response.StatusCode != http.StatusServiceUnavailable) {
			t.Fatalf("listener did not recover: response=%v err=%v", response, err)
		}
		select {
		case <-reconnectCtx.Done():
			t.Fatal("listener did not recover before timeout")
		case <-time.After(100 * time.Millisecond):
		}
	}

	defer func() { _ = reconnected.CloseNow() }()
	sendRealtime(t, f, reconnected, "sub", table.Table.ID)
	readRealtime(t, f, reconnected, "subscribed")
	decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+table.Table.ID+"/rows",
		`{"rows":[{"fields":{}}]}`), 201)
	if event := readRealtime(t, f, reconnected, "row.created"); event.TableID != table.Table.ID {
		t.Fatalf("post-reconnect event = %+v", event)
	}
}

func TestRealtimeTypeConversionNotifiesDependentSchema(t *testing.T) {
	f := newACLFixture(t)
	owner := f.user("admin", false)
	workspace := f.newWorkspace(owner, "Dependencies")
	book := f.newBook(owner, workspace.ID, "Book")
	target := f.newTable(owner, book.ID, "Projects")
	source := f.newTable(owner, book.ID, "Tasks")
	value := f.newField(owner, target.Table.ID, `{"name":"Amount","uiType":"integer"}`)
	relation := decode[relationBody](f, f.api(owner, "POST", "/tables/"+source.Table.ID+"/relations",
		fmt.Sprintf(`{"name":"Project","reverseName":"Tasks","targetTableId":%q,"kind":"one_many","sourceSide":"child"}`,
			target.Table.ID)), 201)
	f.newField(owner, source.Table.ID, fmt.Sprintf(`{"name":"Total","uiType":"rollup","options":`+
		`{"relationFieldId":%q,"targetFieldId":%q,"aggregate":"sum"}}`, relation.Relation.ChildFieldID, value.ID))
	before := decode[structure.TableSchema](f, f.api(owner, "GET", "/tables/"+source.Table.ID+"/schema", ""), 200)

	server, _ := realtimeServer(t, f)
	socket := dialRealtime(t, f, server, owner, server.URL)
	sendRealtime(t, f, socket, "sub", source.Table.ID)
	readRealtime(t, f, socket, "subscribed")
	converted := decode[fieldBody](f, f.fieldCall(owner, "POST", "/fields/"+value.ID+"/convert",
		`{"uiType":"text"}`, "1"), 200)
	event := readRealtime(t, f, socket, "schema.changed")
	if event.TableID != source.Table.ID || event.SchemaVersion <= before.SchemaVersion ||
		event.EventID != converted.EventID {
		t.Fatalf("dependent schema notification = %+v, previous version=%d", event, before.SchemaVersion)
	}
}

func TestRealtimeRelatedRowsRespectSourceACL(t *testing.T) {
	f := newACLFixture(t)
	owner, reader, full := f.user("admin", false), f.user("user", false), f.user("user", false)
	workspace := f.newWorkspace(owner, "Related")
	book := f.newBook(owner, workspace.ID, "Book")
	target := f.newTable(owner, book.ID, "Projects")
	source := f.newTable(owner, book.ID, "Tasks")
	targetLabel, sourceLabel := relationLabelField(target.TableSchema), relationLabelField(source.TableSchema)
	relation := decode[relationBody](f, f.api(owner, "POST", "/tables/"+source.Table.ID+"/relations",
		fmt.Sprintf(`{"name":"Project","reverseName":"Tasks","targetTableId":%q,"kind":"one_many","sourceSide":"child"}`,
			target.Table.ID)), 201)
	lookup := f.newField(owner, source.Table.ID, fmt.Sprintf(`{"name":"Project name","uiType":"lookup","options":`+
		`{"relationFieldId":%q,"targetFieldId":%q}}`, relation.Relation.ChildFieldID, targetLabel.ID))
	projects := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+target.Table.ID+"/rows",
		fmt.Sprintf(`{"rows":[{"fields":{%q:"Before"}}]}`, targetLabel.ID)), 201)
	tasks := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+source.Table.ID+"/rows",
		fmt.Sprintf(`{"rows":[{"fields":{%q:"Task"}}]}`, sourceLabel.ID)), 201)
	f.expectStatus(f.api(owner, "POST", "/tables/"+source.Table.ID+"/rows/"+tasks.Rows[0].ID+
		"/links/"+relation.Relation.ChildFieldID, fmt.Sprintf(`{"add":[%q]}`, projects.Rows[0].ID)), 200, "")
	f.grantOK(owner, acl.Table, source.Table.ID, "user", reader.id, acl.Read)
	f.grantOK(owner, acl.Table, source.Table.ID, "user", full.id, acl.Read)
	f.grantOK(owner, acl.Table, target.Table.ID, "user", full.id, acl.Read)
	server, _ := realtimeServer(t, f)
	readerSocket := dialRealtime(t, f, server, reader, server.URL)
	fullSocket := dialRealtime(t, f, server, full, server.URL)
	for _, socket := range []*websocket.Conn{readerSocket, fullSocket} {
		sendRealtime(t, f, socket, "sub", source.Table.ID)
		readRealtime(t, f, socket, "subscribed")
	}

	patch := decode[rowResponse](f, f.api(owner, "PATCH", "/tables/"+target.Table.ID+"/rows/"+projects.Rows[0].ID,
		fmt.Sprintf(`{"fields":{%q:"After"}}`, targetLabel.ID)), 200)
	event := readRealtime(t, f, fullSocket, "row.bulk")
	if event.TableID != source.Table.ID || event.EventID != patch.EventID || event.SourceTableID != "" || event.By != "" {
		t.Fatalf("related-table invalidation leaked source details: %+v", event)
	}
	query := decode[structure.RowQueryResult](f, f.api(full, "POST", "/views/"+source.Views[0].ID+"/query", `{}`), 200)
	if len(query.Rows) != 1 || string(query.Rows[0].Fields[lookup.ID]) != `["After"]` {
		t.Fatalf("related lookup after update = %+v", query.Rows)
	}
	assertHidden := func(operation string) {
		t.Helper()
		sendRealtime(t, f, readerSocket, "ping", "")
		var next events.Event
		ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
		defer cancel()
		if err := wsjson.Read(ctx, readerSocket, &next); err != nil || next.Type != "pong" {
			t.Fatalf("hidden related %s leaked to reader: %+v, %v", operation, next, err)
		}
	}
	assertHidden("update")

	path := "/tables/" + target.Table.ID + "/rows/" + projects.Rows[0].ID +
		"/links/" + relation.Relation.ParentFieldID
	for _, action := range []string{"remove", "add"} {
		change := decode[rowResponse](f, f.api(owner, "POST", path,
			fmt.Sprintf(`{%q:[%q]}`, action, tasks.Rows[0].ID)), 200)
		event = readRealtime(t, f, fullSocket, "row.bulk")
		if event.TableID != source.Table.ID || event.EventID != change.EventID || event.SourceTableID != "" || event.By != "" {
			t.Fatalf("related link invalidation leaked source details: %+v", event)
		}
		assertHidden("link change")
	}

	deleted := decode[rowsResponse](f, f.api(owner, "DELETE", "/tables/"+target.Table.ID+"/rows",
		fmt.Sprintf(`{"ids":[%q]}`, projects.Rows[0].ID)), 200)
	event = readRealtime(t, f, fullSocket, "row.bulk")
	if event.TableID != source.Table.ID || event.EventID != deleted.EventID || event.SourceTableID != "" || event.By != "" {
		t.Fatalf("related deletion invalidation leaked source details: %+v", event)
	}
	assertHidden("deletion")
}

func TestRealtimeSelfRelationInvalidatesOtherRows(t *testing.T) {
	f := newACLFixture(t)
	owner := f.user("admin", false)
	workspace := f.newWorkspace(owner, "Self relation")
	book := f.newBook(owner, workspace.ID, "Book")
	table := f.newTable(owner, book.ID, "Rows")
	label := relationLabelField(table.TableSchema)
	relation := decode[relationBody](f, f.api(owner, "POST", "/tables/"+table.Table.ID+"/relations",
		fmt.Sprintf(`{"name":"Parent","reverseName":"Children","targetTableId":%q,"kind":"one_many","sourceSide":"child"}`,
			table.Table.ID)), 201)
	created := decode[rowsResponse](f, f.api(owner, "POST", "/tables/"+table.Table.ID+"/rows",
		fmt.Sprintf(`{"rows":[{"fields":{%q:"Before"}},{"fields":{%q:"Parent"}}]}`, label.ID, label.ID)), 201)
	server, _ := realtimeServer(t, f)
	socket := dialRealtime(t, f, server, owner, server.URL)
	sendRealtime(t, f, socket, "sub", table.Table.ID)
	readRealtime(t, f, socket, "subscribed")
	patch := decode[rowResponse](f, f.api(owner, "PATCH", "/tables/"+table.Table.ID+"/rows/"+created.Rows[0].ID,
		fmt.Sprintf(`{"fields":{%q:"After"}}`, label.ID)), 200)
	readRealtime(t, f, socket, "row.updated")
	event := readRealtime(t, f, socket, "row.bulk")
	if event.TableID != table.Table.ID || event.EventID != patch.EventID || event.By != "" {
		t.Fatalf("self-relation invalidation = %+v", event)
	}
	link := decode[rowResponse](f, f.api(owner, "POST", "/tables/"+table.Table.ID+"/rows/"+created.Rows[0].ID+
		"/links/"+relation.Relation.ChildFieldID, fmt.Sprintf(`{"add":[%q]}`, created.Rows[1].ID)), 200)
	readRealtime(t, f, socket, "row.updated")
	event = readRealtime(t, f, socket, "row.bulk")
	if event.TableID != table.Table.ID || event.EventID != link.EventID || event.By != "" {
		t.Fatalf("self-relation link invalidation = %+v", event)
	}
	deleted := decode[rowsResponse](f, f.api(owner, "DELETE", "/tables/"+table.Table.ID+"/rows",
		fmt.Sprintf(`{"ids":[%q]}`, created.Rows[1].ID)), 200)
	event = readRealtime(t, f, socket, "row.bulk")
	if event.TableID != table.Table.ID || event.EventID != deleted.EventID || event.By != "" {
		t.Fatalf("self-relation deletion invalidation = %+v", event)
	}
}
