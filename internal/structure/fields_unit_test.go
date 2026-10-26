package structure

import (
	"encoding/json"
	"strings"
	"testing"

	"pandatable/internal/codec"
	"pandatable/internal/ddl"
)

func TestCleanDescriptionBounds(t *testing.T) {
	for _, tc := range []struct {
		input, want string
		ok          bool
	}{
		{"  line one\n\tline two  ", "line one\n\tline two", true},
		{strings.Repeat("字", 1000), strings.Repeat("字", 1000), true},
		{strings.Repeat("字", 1001), "", false},
		{"bad\rline", "", false},
		{"bad\x00", "", false},
		{"bad\u0085line", "", false},
		{string([]byte{0xff}), "", false},
	} {
		got, err := cleanDescription(tc.input)
		if (err == nil) != tc.ok || got != tc.want {
			t.Fatalf("cleanDescription(%q) = %q, %v", tc.input, got, err)
		}
	}
}

func bind(t *testing.T, kind codec.Kind, options string) codec.Field {
	t.Helper()
	field, err := codec.Bind(kind, json.RawMessage(options))
	if err != nil {
		t.Fatal(err)
	}
	return field
}

func TestChoiceRenamesMatchByIDAndRenameDefaults(t *testing.T) {
	before := bind(t, codec.MultiSelect, `{"choices":[{"id":"a","label":"A"},{"id":"b","label":"B"},{"id":"c","label":"C"}]}`)
	after := bind(t, codec.MultiSelect, `{"choices":[{"id":"b","label":"A"},{"id":"a","label":"B"},{"id":"d","label":"C"}]}`)
	from, to := choiceRenames(before, after)
	if strings.Join(from, ",") != "B,A" || strings.Join(to, ",") != "A,B" {
		t.Fatalf("renames %v -> %v", from, to)
	}
	if got := string(renameDefault([]byte(`["A","C","B"]`), from, to)); got != `["B","C","A"]` {
		t.Fatalf("multi default = %s", got)
	}
	if got := string(renameDefault([]byte(`"A"`), from, to)); got != `"B"` {
		t.Fatalf("single default = %s", got)
	}
	if got := string(renameDefault([]byte(`"Z"`), from, to)); got != `"Z"` {
		t.Fatalf("unrelated default = %s", got)
	}
	if renameDefault(nil, from, to) != nil {
		t.Fatal("absent default must stay absent")
	}
	if from, _ := choiceRenames(bind(t, codec.Text, `{}`), bind(t, codec.Text, `{}`)); from != nil {
		t.Fatal("non-select kinds have no renames")
	}
}

func TestPGTypeNamesAndDisplayEligibility(t *testing.T) {
	for kind, want := range map[codec.Kind]string{codec.Text: "text", codec.Integer: "integer", codec.Decimal: "numeric(18,2)",
		codec.Float: "double precision", codec.Checkbox: "boolean", codec.DateTime: "timestamptz", codec.MultiSelect: "text[]"} {
		if got := pgTypeName(bind(t, kind, `{}`).Column().Type); got != want {
			t.Fatalf("%s: %s, want %s", kind, got, want)
		}
	}
	if got := pgTypeName(ddl.DataType{Kind: ddl.Varchar, Length: 20}); got != "varchar(20)" {
		t.Fatal(got)
	}
	for kind, want := range map[string]bool{"text": true, "integer": true, "single_select": true, "checkbox": false,
		"id": false, "created_by": false} {
		if displayEligible(kind, false) != want {
			t.Fatalf("%s eligibility", kind)
		}
	}
	if displayEligible("text", true) {
		t.Fatal("system fields are never display fields")
	}
}
