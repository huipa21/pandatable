package structure

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/jackc/pgx/v5/pgtype"

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

func TestExportSpreadsheetTimeAndAttachmentReferences(t *testing.T) {
	clock, err := codec.Bind(codec.Time, nil)
	if err != nil {
		t.Fatal(err)
	}

	files, err := codec.Bind(codec.Attachment, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan := exportPlan{location: time.UTC}
	for _, test := range []struct {
		text  string
		micro int64
	}{
		{"00:00:00", 0},
		{"12:34:56.1234", 45296123400},
		{"23:59:59.999999", 86399999999},
	} {
		raw, err := json.Marshal(test.text)
		if err != nil {
			t.Fatal(err)
		}
		value, err := clock.Decode(raw)
		if err != nil {
			t.Fatal(err)
		}
		row := exportRow{record: RowRecord{Fields: map[string]json.RawMessage{"clock": raw}},
			values: map[string]any{"clock": value}}
		got, err := plan.spreadsheetValue(row, rowField{id: "clock", kind: codec.Time, codec: clock}, nil)
		fraction, ok := got.(float64)
		want := float64(test.micro) / (24 * 60 * 60 * 1_000_000)
		if err != nil || !ok || math.Abs(fraction-want) > 1e-14 {
			t.Errorf("time %s = %v (%T), %v; want %g", test.text, got, got, err, want)
		}
	}

	raw := json.RawMessage(`[{"id":"00000000-0000-0000-0000-000000000001","name":"report, \"final\".pdf","mime":"application/pdf","size":10,"thumb":null},{"id":"00000000-0000-0000-0000-000000000002","name":"report, \"final\".pdf","mime":"application/pdf","size":20,"thumb":null}]`)
	row := exportRow{record: RowRecord{Fields: map[string]json.RawMessage{"files": raw}},
		values: map[string]any{"files": raw}}
	got, err := plan.spreadsheetValue(row, rowField{id: "files", kind: codec.Attachment, codec: files}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var references []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	text, ok := got.(string)
	if !ok || json.Unmarshal([]byte(text), &references) != nil ||
		len(references) != 2 || references[0].ID == references[1].ID ||
		references[0].Name != `report, "final".pdf` || references[1].Name != references[0].Name ||
		strings.Contains(text, `"mime"`) || strings.Contains(text, `"size"`) {
		t.Fatalf("attachment references = %q", got)
	}
	column := importColumnPlan{
		field:  rowField{id: "files", kind: codec.Attachment, codec: files},
		source: ImportColumn{UIType: "attachment"},
	}
	if value, err := parseImportCell(column, text, true, codec.Env{Location: time.UTC}); err == nil || value != nil {
		t.Fatalf("unimplemented attachment relink was silently accepted: %v, %v", value, err)
	}
}

func TestExportTimeSerialImportCompatibility(t *testing.T) {
	bound, err := codec.Bind(codec.Time, nil)
	if err != nil {
		t.Fatal(err)
	}
	column := importColumnPlan{field: rowField{kind: codec.Time, codec: bound}, source: ImportColumn{UIType: "time"}}
	for _, test := range []struct {
		text  string
		micro int64
	}{
		{"0", 0},
		{"0.5", 43200000000},
		{"0.5242606875", 45296123400},
		{"0.9999999999884259", 86399999999},
		{"12:34:56.1234", 45296123400},
	} {
		value, err := parseImportCell(column, test.text, true, codec.Env{Location: time.UTC})
		clock, ok := value.(pgtype.Time)
		if err != nil || !ok || !clock.Valid || clock.Microseconds != test.micro {
			t.Errorf("import time %q = %v, %v; want %d microseconds", test.text, value, err, test.micro)
		}
	}
	for _, text := range []string{"-0.1", "1", "NaN", "+Inf", "0.9999999999999999"} {
		if value, err := parseImportCell(column, text, true, codec.Env{Location: time.UTC}); err == nil {
			t.Errorf("invalid time serial %q accepted as %v", text, value)
		}
	}
}
