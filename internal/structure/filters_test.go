package structure

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"pandatable/internal/codec"
)

func TestFilterCompilerEscapesTextAndRejectsUnsupportedTree(t *testing.T) {
	bound, err := codec.Bind(codec.Text, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	field := rowField{id: "text-id", kind: codec.Text, column: "c_aaaaaaaaaa", codec: bound}
	if _, err := bound.Decode(json.RawMessage(`"50%_done\\ok"`)); err != nil {
		t.Fatalf("decode text filter: %v", err)
	}
	table := rowTable{fields: []rowField{field}, byID: map[string]rowField{field.id: field}}
	node := &FilterNode{Field: field.id, Cmp: codec.OpContains, Value: json.RawMessage(`"50%_done\\ok"`)}
	sql, args, err := compileRowFilter(context.Background(), nil, Actor{}, table, node)
	if err != nil {
		t.Fatalf("compile row filter: %#v", err)
	}
	if !strings.Contains(sql, `"source"."c_aaaaaaaaaa" ILIKE $1 ESCAPE '\'`) {
		t.Fatalf("compiled SQL = %q", sql)
	}
	if got, want := args[0], `%50\%\_done\\ok%`; got != want {
		t.Fatalf("pattern argument = %q, want %q", got, want)
	}

	unsupported := &FilterNode{Field: field.id, Cmp: codec.OpEq, Value: json.RawMessage(`"x"`)}
	if _, _, err := compileRowFilter(context.Background(), nil, Actor{}, table, unsupported); err == nil {
		t.Fatal("unsupported operation was accepted")
	}
	tooDeep := &FilterNode{Op: "and", Items: []FilterNode{{
		Op: "or", Items: []FilterNode{{Op: "and", Items: []FilterNode{{Op: "or", Items: []FilterNode{{
			Field: field.id, Cmp: codec.OpIs, Value: json.RawMessage(`"x"`),
		}}}}}},
	}}}
	if _, _, err := compileRowFilter(context.Background(), nil, Actor{}, table, tooDeep); err == nil {
		t.Fatal("filter nesting deeper than three groups was accepted")
	}
}

func TestJSONFilterOperatorsBindKeysAndPaths(t *testing.T) {
	bound, err := codec.Bind(codec.JSON, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	field := rowField{id: "json-id", kind: codec.JSON, column: "c_aaaaaaaaaa", codec: bound}
	table := rowTable{fields: []rowField{field}, byID: map[string]rowField{field.id: field}}
	key := &FilterNode{Field: field.id, Cmp: codec.OpHasKey, Value: json.RawMessage(`"owner"`)}
	sql, args, err := compileRowFilter(context.Background(), nil, Actor{}, table, key)
	if err != nil || !strings.Contains(sql, `jsonb_exists("source"."c_aaaaaaaaaa", $1)`) || len(args) != 1 || args[0] != "owner" {
		t.Fatalf("key predicate = %q, %#v, %v", sql, args, err)
	}

	path := &FilterNode{
		Field: field.id, Cmp: codec.OpPathEquals,
		Value: json.RawMessage(`{"path":["project","name"],"value":"Panda"}`),
	}
	sql, args, err = compileRowFilter(context.Background(), nil, Actor{}, table, path)
	if err != nil || !strings.Contains(sql, `#>> $1::text[] = $2`) || len(args) != 2 {
		t.Fatalf("path predicate = %q, %#v, %v", sql, args, err)
	}
	segments, ok := args[0].([]string)
	if !ok || strings.Join(segments, "/") != "project/name" || args[1] != "Panda" {
		t.Fatalf("path arguments = %#v", args)
	}

	invalid := []*FilterNode{
		{Field: field.id, Cmp: codec.OpPathEquals, Value: json.RawMessage(`{"path":[],"value":"x"}`)},
		{Field: field.id, Cmp: codec.OpPathEquals, Value: json.RawMessage(`{"path":["x"],"value":null}`)},
		{Field: field.id, Cmp: codec.OpPathEquals, Value: json.RawMessage(`{"path":["x"],"value":"x","extra":true}`)},
	}
	for _, node := range invalid {
		if _, _, err := compileRowFilter(context.Background(), nil, Actor{}, table, node); err == nil {
			t.Fatalf("invalid JSON filter accepted: %s", node.Value)
		}
	}
}

func TestFilterRelativeDayUsesRequestTimezone(t *testing.T) {
	now, err := time.Parse(time.RFC3339, "2026-03-08T12:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	start, end, err := relativeDay("today", "America/Los_Angeles", now)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := start.Format("2006-01-02T15:04:05Z07:00"), "2026-03-08T00:00:00-08:00"; got != want {
		t.Fatalf("start = %s, want %s", got, want)
	}
	if got := end.Sub(start).Hours(); got != 23 {
		t.Fatalf("DST day length = %v hours, want 23", got)
	}

	start, _, err = relativeDay("today", "Pacific/Kiritimati", now)
	if err != nil {
		t.Fatal(err)
	}
	param, ok := temporalParam(codec.Date, start).(pgtype.Date)
	if !ok {
		t.Fatalf("date parameter type = %T, want pgtype.Date", temporalParam(codec.Date, start))
	}
	if got, want := param.Time.Format("2006-01-02"), "2026-03-09"; got != want {
		t.Fatalf("date parameter = %s, want local calendar date %s", got, want)
	}
}
