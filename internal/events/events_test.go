package events

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

type eventExecutor struct {
	args []any
	err  error
}

func (e *eventExecutor) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	e.args = args
	return pgconn.CommandTag{}, e.err
}

func TestEmitCapsRowsAndPreservesEventID(t *testing.T) {
	ctx := WithID(context.Background(), "0123456789abcdef")
	exec := &eventExecutor{}
	ids := make([]string, MaxRowIDs+1)
	for i := range ids {
		ids[i] = "1234567890123456789"
	}
	if err := Emit(ctx, exec, Event{Type: "row.updated", TableID: "table", IDs: ids}); err != nil {
		t.Fatal(err)
	}
	var event Event
	if err := json.Unmarshal([]byte(exec.args[1].(string)), &event); err != nil {
		t.Fatal(err)
	}
	if exec.args[0] != Channel || event.Type != "row.bulk" || len(event.IDs) != 0 ||
		event.EventID != ID(ctx) || len(exec.args[1].(string)) >= MaxPayloadSize {
		t.Fatalf("unexpected bulk event: %+v", event)
	}
}

func TestEmitFallsBackBeforePayloadLimit(t *testing.T) {
	ids := make([]string, MaxRowIDs)
	for i := range ids {
		ids[i] = strings.Repeat("9", 19)
	}
	payload, err := encode(Event{Type: "row.updated", TableID: "table", IDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	var event Event
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatal(err)
	}
	if event.Type != "row.bulk" || len(payload) >= MaxPayloadSize {
		t.Fatalf("payload was not converted to bulk: %s", payload)
	}
}

func TestEmitKeepsFiveHundredShortRowIDs(t *testing.T) {
	ids := make([]string, MaxRowIDs)
	for i := range ids {
		ids[i] = "1"
	}
	payload, err := encode(Event{Type: "row.updated", TableID: "table", IDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	var event Event
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatal(err)
	}
	if event.Type != "row.updated" || len(event.IDs) != MaxRowIDs || len(payload) >= MaxPayloadSize {
		t.Fatalf("500 short IDs should remain a row event: %+v", event)
	}
}

func TestEmitPropagatesErrors(t *testing.T) {
	want := errors.New("notify failed")
	if err := Emit(context.Background(), &eventExecutor{err: want}, Event{Type: "nav.changed"}); !errors.Is(err, want) {
		t.Fatalf("Emit error = %v", err)
	}
	if _, err := encode(Event{Type: "nav.changed", ByName: strings.Repeat("x", MaxPayloadSize)}); err == nil {
		t.Fatal("oversized event succeeded")
	}
}
