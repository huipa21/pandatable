package structure

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"pandatable/internal/codec"
	"pandatable/internal/formula"
)

func TestExportManifestChunksAndZipEntryLimits(t *testing.T) {
	text := strings.Repeat("a", manifestChunkChars-1) + "😀" +
		strings.Repeat("中", manifestChunkChars) + "😀"
	chunks := splitPackageManifest([]byte(text))
	if len(chunks) != 3 || strings.Join(chunks, "") != text {
		t.Fatalf("manifest chunks do not reassemble: %d chunks", len(chunks))
	}
	for _, chunk := range chunks {
		if got := len(utf16.Encode([]rune(chunk))); got > manifestChunkChars {
			t.Fatalf("manifest chunk has %d UTF-16 units", got)
		}
	}

	var output bytes.Buffer
	entry := &boundedZipEntry{writer: &output, size: maxZipEntryBytes - 2}
	if n, err := entry.Write([]byte("abc")); n != 0 || err == nil || output.Len() != 0 {
		t.Fatalf("oversized entry write = %d, %v", n, err)
	}
	if n, err := entry.Write([]byte("ok")); n != 2 || err != nil || output.String() != "ok" {
		t.Fatalf("bounded entry write = %d, %v", n, err)
	}
	failed := &boundedZipEntry{writer: failingExportWriter{}, size: 0}
	if _, err := failed.Write([]byte("x")); err == nil {
		t.Fatal("ZIP entry swallowed a write error")
	}
}

type failingExportWriter struct{}

func (failingExportWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestSpreadsheetNumbersAndComputedTypes(t *testing.T) {
	for _, test := range []struct {
		input string
		want  any
	}{
		{"0.001", 0.001},
		{"123456789012345", float64(123456789012345)},
		{"9007199254740993", "9007199254740993"},
	} {
		got, err := spreadsheetNumber(test.input)
		if err != nil || got != test.want {
			t.Errorf("spreadsheetNumber(%q) = %v, %v; want %v", test.input, got, err, test.want)
		}
	}
	if _, err := spreadsheetNumber("not-a-number"); err == nil {
		t.Fatal("invalid number was accepted")
	}

	auto, err := codec.Bind(codec.AutoNumber, json.RawMessage(`{"prefix":"","padding":6}`))
	if err != nil {
		t.Fatal(err)
	}
	plan := exportPlan{location: time.UTC}
	row := exportRow{
		record: RowRecord{Fields: map[string]json.RawMessage{
			"auto": json.RawMessage(`42`), "rollup": json.RawMessage(`12.5`),
			"formula": json.RawMessage(`true`),
		}},
		values: map[string]any{"auto": int64(42)},
	}
	fields := []struct {
		field rowField
		want  any
	}{
		{rowField{id: "auto", kind: codec.AutoNumber, codec: auto}, "000042"},
		{rowField{id: "rollup", kind: "rollup", computed: &computedField{aggregate: "sum"}}, 12.5},
		{rowField{id: "formula", formula: &formulaField{options: formulaOptions{ResultType: formula.Boolean}}}, true},
	}
	for _, test := range fields {
		got, err := plan.spreadsheetValue(row, test.field, nil)
		if err != nil || got != test.want {
			t.Errorf("spreadsheetValue(%s) = %v, %v; want %v", test.field.id, got, err, test.want)
		}
	}
}

func TestSpreadsheetListAndLinkValuesPreserveSeparators(t *testing.T) {
	list, err := codec.Bind(codec.TextArray, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan := exportPlan{location: time.UTC}
	row := exportRow{
		record: RowRecord{Fields: map[string]json.RawMessage{
			"list": json.RawMessage(`["first, second"," spaced ","\"quoted\""]`),
			"link": json.RawMessage(`[{"id":"1","label":"A, B"},{"id":"2","label":" C "}]`),
		}},
		values: map[string]any{"list": []string{"first, second", " spaced ", `"quoted"`}},
	}
	for _, test := range []struct {
		field rowField
		want  []string
	}{
		{rowField{id: "list", kind: codec.TextArray, codec: list}, []string{"first, second", " spaced ", `"quoted"`}},
		{rowField{id: "link", relation: &relationRecord{}}, []string{"A, B", " C "}},
	} {
		value, err := plan.spreadsheetValue(row, test.field, nil)
		if err != nil {
			t.Fatal(err)
		}
		text, ok := value.(string)
		if !ok {
			t.Fatalf("%s package value is not text: %T", test.field.id, value)
		}
		var got []string
		if err := json.Unmarshal([]byte(text), &got); err != nil || !reflect.DeepEqual(got, test.want) {
			t.Fatalf("%s package value = %q, %v; want %v", test.field.id, text, err, test.want)
		}
	}
}
