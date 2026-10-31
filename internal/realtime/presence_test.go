package realtime

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"pandatable/internal/acl"
	"pandatable/internal/auth"
)

const (
	testTable = "018f0000-0000-7000-8000-000000000001"
	testView  = "018f0000-0000-7000-8000-000000000002"
	otherView = "018f0000-0000-7000-8000-000000000003"
	testField = "018f0000-0000-7000-8000-000000000004"
)

type presenceAuthorizer struct {
	denied map[string]bool
}

func (a *presenceAuthorizer) Require(_ context.Context, userID string, _ acl.Level, _ acl.Resource) error {
	if a.denied[userID] {
		return acl.ErrNotFound()
	}
	return nil
}

func (*presenceAuthorizer) Invalidate() {}

func presenceTestHub() *Hub {
	return &Hub{
		acl:          &presenceAuthorizer{denied: make(map[string]bool)},
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		clients:      make(map[*client]struct{}),
		presence:     make(map[string]*presenceTable),
		presenceWake: make(chan struct{}, 1),
	}
}

func presenceTestClient(t *testing.T, h *Hub, id, name string) *client {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	c := &client{
		hub: h, ctx: ctx, cancel: cancel, user: auth.User{ID: id, DisplayName: name},
		out: make(chan outgoing, 32), subs: map[string]struct{}{testTable: {}},
	}
	h.clients[c] = struct{}{}
	return c
}

func receivePresence(t *testing.T, c *client) presenceSnapshot {
	t.Helper()
	select {
	case message := <-c.out:
		if message.presence == nil {
			t.Fatalf("expected presence, got %+v", message.event)
		}
		return *message.presence
	default:
		t.Fatal("presence snapshot was not sent")
		return presenceSnapshot{}
	}
}

func TestPresenceValidationAndSubscription(t *testing.T) {
	tests := []struct {
		name    string
		viewID  string
		cell    *presenceCell
		wantOK  bool
		wantRow string
	}{
		{"table only", "", nil, true, ""},
		{"view", strings.ToUpper(testView), nil, true, ""},
		{"bigint row", testView, &presenceCell{RowID: "9007199254740993", FieldID: testField}, true, "9007199254740993"},
		{"invalid view", "bad", nil, false, ""},
		{"cell without view", "", &presenceCell{RowID: "1", FieldID: testField}, false, ""},
		{"invalid field", testView, &presenceCell{RowID: "1", FieldID: "bad"}, false, ""},
		{"zero row", testView, &presenceCell{RowID: "0", FieldID: testField}, false, ""},
		{"noncanonical row", testView, &presenceCell{RowID: "01", FieldID: testField}, false, ""},
		{"overflow row", testView, &presenceCell{RowID: "9223372036854775808", FieldID: testField}, false, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			view, cell, ok := normalizedPresence(incoming{ViewID: test.viewID, Cell: test.cell})
			if ok != test.wantOK {
				t.Fatalf("valid = %v, want %v", ok, test.wantOK)
			}
			if ok && test.viewID != "" && view != testView {
				t.Fatalf("normalized view = %q", view)
			}
			if test.wantRow != "" && (cell == nil || cell.RowID != test.wantRow) {
				t.Fatalf("normalized cell = %+v", cell)
			}
		})
	}

	h := presenceTestHub()
	c := presenceTestClient(t, h, "alice", "Alice")
	c.unsubscribe(testTable)
	c.reportPresence(incoming{TableID: testTable, ViewID: testView})
	message := <-c.out
	if message.event.Type != "error" || message.event.Code != "ERR_VALIDATION" || len(h.presence) != 0 {
		t.Fatalf("unsubscribed presence was accepted: %+v", message)
	}
}

func TestPresenceSchedulingAndHeartbeatExpiry(t *testing.T) {
	h := presenceTestHub()
	alice := presenceTestClient(t, h, "alice", "Alice")
	bob := presenceTestClient(t, h, "bob", "Bob")
	charlie := presenceTestClient(t, h, "charlie", "Charlie")
	start := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)

	h.setPresence(alice, testTable, testView, nil, start)
	due, _ := h.duePresence(start)
	if len(due) != 1 || due[0] != testTable {
		t.Fatalf("initial broadcast = %v", due)
	}
	table := h.presence[testTable]
	table.dirty = false
	table.lastBroadcast = start
	initialActivity := table.entries[alice].activity
	h.setPresence(alice, testTable, testView, nil, start.Add(time.Second))
	if table.dirty || table.entries[alice].activity != initialActivity {
		t.Fatal("heartbeat changed the visible presence or activity order")
	}
	for i := 1; i <= 20; i++ {
		h.setPresence(alice, testTable, testView,
			&presenceCell{RowID: strconv.Itoa(i), FieldID: testField}, start.Add(time.Duration(i)*time.Millisecond))
	}
	due, next := h.duePresence(start.Add(249 * time.Millisecond))
	if len(due) != 0 || !next.Equal(start.Add(presenceBroadcastInterval)) {
		t.Fatalf("broadcast was not throttled: due=%v next=%v", due, next)
	}
	due, _ = h.duePresence(start.Add(presenceBroadcastInterval))
	if len(due) != 1 || table.entries[alice].cell.RowID != "20" {
		t.Fatalf("latest focus was not coalesced: due=%v cell=%+v", due, table.entries[alice].cell)
	}

	h.setPresence(alice, testTable, testView, nil, start)
	h.setPresence(bob, testTable, testView, nil, start.Add(59*time.Second+800*time.Millisecond))
	h.setPresence(charlie, testTable, testView, nil, start.Add(100*time.Millisecond))
	table.lastBroadcast = start.Add(59*time.Second + 700*time.Millisecond)
	table.dirty = true
	due, next = h.duePresence(start.Add(59*time.Second + 800*time.Millisecond))
	deadline := start.Add(presenceTimeout)
	if len(due) != 0 || !next.Equal(deadline) {
		t.Fatalf("expiry slot was not reserved: due=%v next=%v", due, next)
	}
	due, _ = h.duePresence(deadline.Add(-time.Millisecond))
	if len(due) != 0 || len(table.entries) != 3 {
		t.Fatalf("presence expired before its reserved slot: due=%v entries=%d", due, len(table.entries))
	}
	due, _ = h.duePresence(deadline)
	if len(due) != 1 || len(table.entries) != 1 || table.entries[bob].viewID != testView {
		t.Fatalf("stale entries did not expire together by their deadlines: due=%v entries=%+v", due, table.entries)
	}
}

func TestPresenceSnapshotsIsolateViewsAndAggregateConnections(t *testing.T) {
	h := presenceTestHub()
	alice := presenceTestClient(t, h, "alice", "Alice")
	bob := presenceTestClient(t, h, "bob", "Bob")
	bobOtherTab := presenceTestClient(t, h, "bob", "Bob")
	denied := presenceTestClient(t, h, "denied", "Hidden")
	h.acl.(*presenceAuthorizer).denied["denied"] = true
	now := time.Now()
	h.setPresence(alice, testTable, testView, nil, now)
	h.setPresence(bob, testTable, testView, &presenceCell{RowID: "1", FieldID: testField}, now)
	h.setPresence(bobOtherTab, testTable, otherView, &presenceCell{RowID: "2", FieldID: testField}, now)
	h.setPresence(denied, testTable, testView, nil, now)

	h.broadcastPresence(testTable)
	aliceSnapshot := receivePresence(t, alice)
	otherSnapshot := receivePresence(t, bobOtherTab)
	_ = receivePresence(t, bob)
	if len(aliceSnapshot.Users) != 2 || aliceSnapshot.Users[1].UserID != "bob" ||
		aliceSnapshot.Users[1].Cell != nil || aliceSnapshot.ViewID != testView {
		t.Fatalf("wrong view leaked or user aggregation failed: %+v", aliceSnapshot)
	}

	if len(otherSnapshot.Users) != 2 || otherSnapshot.Users[1].Cell == nil ||
		otherSnapshot.Users[1].Cell.RowID != "2" {
		t.Fatalf("matching view did not receive focus: %+v", otherSnapshot)
	}
	if message := <-denied.out; message.event.Type != "sub.denied" || denied.subscribed(testTable) {
		t.Fatalf("denied subscriber retained presence: %+v", message)
	}
	payload, err := json.Marshal(aliceSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), otherView) || strings.Contains(string(payload), "Hidden") ||
		strings.Contains(string(payload), "username") || strings.Contains(string(payload), "email") {
		t.Fatalf("snapshot included unrelated details: %s", payload)
	}

	bobOtherTab.unsubscribe(testTable)
	h.presence[testTable].lastBroadcast = time.Now().Add(-presenceBroadcastInterval)
	h.broadcastPresence(testTable)
	aliceSnapshot = receivePresence(t, alice)
	_ = receivePresence(t, bob)
	if len(aliceSnapshot.Users) != 2 || aliceSnapshot.Users[1].Cell == nil ||
		aliceSnapshot.Users[1].Cell.RowID != "1" {
		t.Fatalf("disconnecting one tab removed the other or lost its focus: %+v", aliceSnapshot)
	}

	bob.unsubscribe(testTable)
	h.presence[testTable].lastBroadcast = time.Now().Add(-presenceBroadcastInterval)
	h.broadcastPresence(testTable)
	aliceSnapshot = receivePresence(t, alice)
	if len(aliceSnapshot.Users) != 1 || aliceSnapshot.Users[0].UserID != "alice" {
		t.Fatalf("departure remained visible: %+v", aliceSnapshot)
	}
}

func TestPresenceBroadcastRate(t *testing.T) {
	h := presenceTestHub()
	c := presenceTestClient(t, h, "alice", "Alice")
	ctx, cancel := context.WithCancel(context.Background())
	h.ctx = ctx
	h.presenceDone = make(chan struct{})
	go h.runPresence()
	var mu sync.Mutex
	var sent []time.Time
	collectorDone := make(chan struct{})
	go func() {
		defer close(collectorDone)
		for {
			select {
			case message := <-c.out:
				if message.presence != nil {
					mu.Lock()
					sent = append(sent, time.Now())
					mu.Unlock()
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	for i := 1; i <= 60; i++ {
		h.setPresence(c, testTable, testView,
			&presenceCell{RowID: strconv.Itoa(i), FieldID: testField}, time.Now())
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(presenceBroadcastInterval)
	cancel()
	<-h.presenceDone
	<-collectorDone
	mu.Lock()
	defer mu.Unlock()
	if len(sent) < 4 {
		t.Fatalf("too few coalesced broadcasts: %d", len(sent))
	}
	for i := 1; i < len(sent); i++ {
		if interval := sent[i].Sub(sent[i-1]); interval < 230*time.Millisecond {
			t.Fatalf("broadcasts were only %v apart", interval)
		}
	}
	for i := range sent {
		count := 0
		for j := i; j < len(sent) && sent[j].Sub(sent[i]) < 980*time.Millisecond; j++ {
			count++
		}
		if count > 4 {
			t.Fatalf("%d broadcasts in a 980 ms window", count)
		}
	}
}
