package codec

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/apperr"
	"pandatable/internal/ddl"
)

type kindVector struct {
	Options     json.RawMessage `json:"options"`
	ReadOnly    bool            `json:"readOnly"`
	Column      string          `json:"column"`
	Constraints [3]bool         `json:"constraints"`
	FilterOps   []Op            `json:"filterOps"`
	Aggregates  []Agg           `json:"aggregates"`
}

type caseVector struct {
	Kind      Kind            `json:"kind"`
	Options   json.RawMessage `json:"options"`
	Input     json.RawMessage `json:"input"`
	Canonical json.RawMessage `json:"canonical"`
	Raw       string          `json:"raw"`
	Text      string          `json:"text"`
	Wire      json.RawMessage `json:"wire"`
	Zone      string          `json:"zone"`
	Output    json.RawMessage `json:"output"`
	Reason    string          `json:"reason"`
	Option    string          `json:"option"`
	GoOnly    bool            `json:"goOnly"`
}

type vectorFile struct {
	Kinds   map[Kind]kindVector `json:"kinds"`
	Options []caseVector        `json:"options"`
	Decode  []caseVector        `json:"decode"`
	Paste   []caseVector        `json:"paste"`
	Format  []caseVector        `json:"format"`
}

func loadVectors(t *testing.T) vectorFile {
	t.Helper()
	data, err := os.ReadFile("testdata/vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors vectorFile
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	return vectors
}

func columnName(column Column) string {
	switch column.Type.Kind {
	case ddl.Numeric:
		return fmt.Sprintf("numeric(%d,%d)", column.Type.Precision, column.Type.Scale)
	case ddl.Varchar:
		return fmt.Sprintf("varchar(%d)", column.Type.Length)
	}
	return string(column.Type.Kind)
}

func sameJSON(t *testing.T, got, want json.RawMessage) bool {
	t.Helper()
	var left, right any
	if err := json.Unmarshal(got, &left); err != nil {
		t.Fatalf("invalid JSON %s: %v", got, err)
	}
	if len(want) == 0 {
		want = json.RawMessage("null")
	}
	if err := json.Unmarshal(want, &right); err != nil {
		t.Fatalf("invalid JSON %s: %v", want, err)
	}
	return reflect.DeepEqual(left, right)
}

func reasonOf(err error) string {
	var codecErr *Error
	if errors.As(err, &codecErr) {
		return codecErr.Reason
	}
	if err != nil {
		return "unexpected:" + err.Error()
	}
	return ""
}

func mustBind(t *testing.T, kind Kind, options json.RawMessage) Field {
	t.Helper()
	field, err := Bind(kind, options)
	if err != nil {
		t.Fatalf("bind %s %s: %v", kind, options, err)
	}
	return field
}

func envFor(t *testing.T, zone string) Env {
	t.Helper()
	if zone == "" {
		return Env{}
	}
	location, err := LoadLocation(zone)
	if err != nil {
		t.Fatalf("zone %s: %v", zone, err)
	}
	return Env{Location: location}
}

func checkResult(t *testing.T, label string, field Field, value any, err error, vector caseVector) {
	t.Helper()
	if vector.Reason != "" {
		if got := reasonOf(err); got != vector.Reason {
			t.Errorf("%s: reason %q, want %q (value %#v)", label, got, vector.Reason, value)
		}
		return
	}
	if err != nil {
		t.Errorf("%s: unexpected error %v", label, err)
		return
	}
	encoded, err := field.Encode(value)
	if err != nil {
		t.Errorf("%s: encode %#v: %v", label, value, err)
		return
	}
	if !sameJSON(t, encoded, vector.Output) {
		t.Errorf("%s: got %s, want %s", label, encoded, vector.Output)
	}
}

func TestVectorsKinds(t *testing.T) {
	vectors := loadVectors(t)
	var names []string
	for kind := range vectors.Kinds {
		names = append(names, string(kind))
	}
	sort.Strings(names)
	var registered []string
	for _, kind := range Kinds() {
		registered = append(registered, string(kind))
	}
	if !reflect.DeepEqual(names, registered) {
		t.Fatalf("registry %v, vectors %v", registered, names)
	}
	for kind, want := range vectors.Kinds {
		field := mustBind(t, kind, nil)
		if field.Kind() != kind || field.ReadOnly() != want.ReadOnly || field.ReadOnly() != IsSystem(kind) {
			t.Errorf("%s: kind/readOnly mismatch", kind)
		}
		if !sameJSON(t, field.Options(), want.Options) {
			t.Errorf("%s: options %s, want %s", kind, field.Options(), want.Options)
		}
		if got := columnName(field.Column()); got != want.Column {
			t.Errorf("%s: column %s, want %s", kind, got, want.Column)
		}
		if _, err := field.Column().Type.SQL(); err != nil {
			t.Errorf("%s: column descriptor rejected by ddl: %v", kind, err)
		}
		constraints := field.Constraints()
		if got := [3]bool{constraints.Required, constraints.Unique, constraints.Default}; got != want.Constraints {
			t.Errorf("%s: constraints %v, want %v", kind, got, want.Constraints)
		}
		if !reflect.DeepEqual(field.FilterOps(), want.FilterOps) {
			t.Errorf("%s: ops %v, want %v", kind, field.FilterOps(), want.FilterOps)
		}
		if !reflect.DeepEqual(field.Aggregates(), want.Aggregates) {
			t.Errorf("%s: aggregates %v, want %v", kind, field.Aggregates(), want.Aggregates)
		}
		if field.Column().System != IsSystem(kind) {
			t.Errorf("%s: system column flag", kind)
		}
	}
}

func TestVectorsOptions(t *testing.T) {
	for index, vector := range loadVectors(t).Options {
		label := fmt.Sprintf("options[%d] %s %s", index, vector.Kind, vector.Input)
		field, err := Bind(vector.Kind, vector.Input)
		if vector.Reason != "" {
			var codecErr *Error
			if !errors.As(err, &codecErr) || codecErr.Reason != vector.Reason || codecErr.Detail != vector.Option {
				t.Errorf("%s: got %v, want %s/%s", label, err, vector.Reason, vector.Option)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", label, err)
			continue
		}
		if !sameJSON(t, field.Options(), vector.Canonical) {
			t.Errorf("%s: canonical %s, want %s", label, field.Options(), vector.Canonical)
		}
		// Canonical options must re-bind to themselves.
		again := mustBind(t, vector.Kind, field.Options())
		if string(again.Options()) != string(field.Options()) {
			t.Errorf("%s: canonical options not stable", label)
		}
	}
}

func TestVectorsDecode(t *testing.T) {
	for index, vector := range loadVectors(t).Decode {
		label := fmt.Sprintf("decode[%d] %s %s", index, vector.Kind, vector.Raw)
		field := mustBind(t, vector.Kind, vector.Options)
		value, err := field.Decode(json.RawMessage(vector.Raw))
		checkResult(t, label, field, value, err, vector)
		if vector.Reason == "" && value != nil && !field.ReadOnly() {
			// Encoded output must decode to the same canonical value.
			encoded, _ := field.Encode(value)
			again, err := field.Decode(encoded)
			if err != nil {
				t.Errorf("%s: canonical %s does not decode: %v", label, encoded, err)
			} else if reencoded, _ := field.Encode(again); string(reencoded) != string(encoded) {
				t.Errorf("%s: canonical not stable %s vs %s", label, encoded, reencoded)
			}
		}
	}
}

func TestVectorsPaste(t *testing.T) {
	for index, vector := range loadVectors(t).Paste {
		label := fmt.Sprintf("paste[%d] %s %q", index, vector.Kind, vector.Text)
		field := mustBind(t, vector.Kind, vector.Options)
		value, err := field.ParsePaste(vector.Text, envFor(t, vector.Zone))
		checkResult(t, label, field, value, err, vector)
	}
}

func TestVectorsFormat(t *testing.T) {
	for index, vector := range loadVectors(t).Format {
		label := fmt.Sprintf("format[%d] %s %s", index, vector.Kind, vector.Wire)
		field := mustBind(t, vector.Kind, vector.Options)
		env := envFor(t, vector.Zone)
		value, err := field.Decode(vector.Wire)
		if err != nil {
			t.Errorf("%s: decode: %v", label, err)
			continue
		}
		text, err := field.FormatText(value, env)
		if err != nil || text != vector.Text {
			t.Errorf("%s: got %q (%v), want %q", label, text, err, vector.Text)
			continue
		}
		// Copy output re-pastes to the same value (D-117).
		if value == nil {
			continue
		}
		pasted, err := field.ParsePaste(text, env)
		if err != nil {
			t.Errorf("%s: re-paste %q: %v", label, text, err)
			continue
		}
		left, _ := field.Encode(value)
		right, _ := field.Encode(pasted)
		if string(left) != string(right) {
			t.Errorf("%s: re-paste %s != %s", label, right, left)
		}
	}
}

func TestUnknownKindAndAPIError(t *testing.T) {
	for _, kind := range []Kind{"formula", "attachment", "user", "", "TEXT"} {
		if _, err := Bind(kind, nil); reasonOf(err) != ReasonUnknownKind {
			t.Errorf("%q: %v", kind, err)
		}
	}
	_, err := Bind(Decimal, json.RawMessage(`{"precision":2,"scale":3}`))
	apiErr, ok := APIError(err).(*apperr.Error)
	if !ok || apiErr.Status != 400 || apiErr.Code != "ERR_VALIDATION" ||
		apiErr.Details["reason"] != ReasonInvalidOptions || apiErr.Details["option"] != "scale" {
		t.Fatalf("api error %#v", APIError(err))
	}
	_, err = mustBind(t, URL, nil).Decode(json.RawMessage(`"javascript:alert(1)"`))
	apiErr, ok = APIError(err).(*apperr.Error)
	if !ok || apiErr.Details["reason"] != ReasonURLScheme || apiErr.Details["option"] != nil {
		t.Fatalf("api error %#v", APIError(err))
	}
	if strings.Contains(apiErr.Message, "javascript") {
		t.Fatal("API error echoes input")
	}
	_, err = mustBind(t, Integer, nil).Encode("x")
	if APIError(err) != err {
		t.Fatal("unsupported DB values must stay server errors")
	}
	plain := errors.New("boom")
	if APIError(plain) != plain {
		t.Fatal("non-codec errors pass through")
	}
}

func TestOptionsRejectMalformed(t *testing.T) {
	for _, raw := range []string{`{"maxLength":1,"maxLength":2}`, `{`, `{"maxLength":1} {}`, `"x"`, `1`} {
		if _, err := Bind(Text, json.RawMessage(raw)); reasonOf(err) != ReasonInvalidOptions {
			t.Errorf("%s: %v", raw, err)
		}
	}
	field := mustBind(t, Text, json.RawMessage(`{"maxLength":255}`))
	if field.Column().Type != (ddl.DataType{Kind: ddl.Varchar, Length: 255}) {
		t.Fatalf("varchar column %+v", field.Column())
	}
}

func TestTextLimits(t *testing.T) {
	text := mustBind(t, Text, nil)
	quote := func(value string) json.RawMessage { encoded, _ := json.Marshal(value); return encoded }
	if _, err := text.Decode(quote(strings.Repeat("界", 10000))); err != nil {
		t.Fatal(err)
	}
	if _, err := text.Decode(quote(strings.Repeat("a", 10001))); reasonOf(err) != ReasonTooLong {
		t.Fatal(err)
	}
	long := mustBind(t, LongText, nil)
	if _, err := long.Decode(quote(strings.Repeat("😀", 100000))); err != nil {
		t.Fatal(err)
	}
	if _, err := long.Decode(quote(strings.Repeat("a", 100001))); reasonOf(err) != ReasonTooLong {
		t.Fatal(err)
	}
	if _, err := text.Decode(json.RawMessage("\"\xff\"")); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
	email := mustBind(t, Email, nil)
	local := strings.Repeat("a", 64)
	domain := strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 57) + ".com"
	if len(local)+1+len(domain) != 254 {
		t.Fatalf("fixture length %d", len(local)+1+len(domain))
	}
	if _, err := email.Decode(quote(local + "@" + domain)); err != nil {
		t.Fatal(err)
	}
	if _, err := email.Decode(quote(local + "a@example.com")); reasonOf(err) != ReasonInvalidEmail {
		t.Fatal("65-byte local part accepted")
	}
	if _, err := email.Decode(quote(local + "@" + domain + "m")); reasonOf(err) != ReasonInvalidEmail {
		t.Fatal("255-byte address accepted")
	}
	url := mustBind(t, URL, nil)
	base := "https://example.com/"
	if _, err := url.Decode(quote(base + strings.Repeat("a", 2048-len(base)))); err != nil {
		t.Fatal(err)
	}
	if _, err := url.Decode(quote(base + strings.Repeat("a", 2049-len(base)))); reasonOf(err) != ReasonTooLong {
		t.Fatal("2049-character URL accepted")
	}
}

func TestChoiceLimitsAndDefaults(t *testing.T) {
	choices := make([]Choice, 501)
	for index := range choices {
		choices[index] = Choice{ID: fmt.Sprintf("c%d", index), Label: fmt.Sprintf("L%d", index)}
	}
	encode := func(items []Choice) json.RawMessage {
		encoded, _ := json.Marshal(map[string]any{"choices": items})
		return encoded
	}
	if _, err := Bind(SingleSelect, encode(choices[:500])); err != nil {
		t.Fatal(err)
	}
	if _, err := Bind(SingleSelect, encode(choices)); reasonOf(err) != ReasonInvalidOptions {
		t.Fatal("501 choices accepted")
	}
	field := mustBind(t, MultiSelect, json.RawMessage(`{"choices":[{"id":"a","label":"say \"hi\""},{"id":"b","label":"back\\slash"},{"id":"c","label":"{x}"}]}`))
	value, err := field.DefaultValue(json.RawMessage(`["{x}","back\\slash","say \"hi\""]`))
	if err != nil || value != (ddl.DefaultValue{Kind: ddl.Literal, Literal: `{"say \"hi\"","back\\slash","{x}"}`}) {
		t.Fatalf("default %+v %v", value, err)
	}
	if value, err := field.DefaultValue(json.RawMessage(`null`)); err != nil || value.Kind != ddl.NoDefault {
		t.Fatalf("null default %+v %v", value, err)
	}
	// Stored labels that are no longer configured are still encoded.
	if encoded, err := field.Encode([]any{"Removed"}); err != nil || string(encoded) != `["Removed"]` {
		t.Fatalf("stale label %s %v", encoded, err)
	}
	if encoded, err := field.Encode([]string{}); err != nil || string(encoded) != "null" {
		t.Fatalf("empty array %s %v", encoded, err)
	}
	if text, _ := field.FormatText([]any{"x", "y"}, Env{}); text != "x, y" {
		t.Fatal(text)
	}
}

func TestDefaultValues(t *testing.T) {
	cases := []struct {
		kind    Kind
		options string
		raw     string
		literal string
		reason  string
	}{
		{Text, "", `"hello 'quoted'"`, "hello 'quoted'", ""},
		{Integer, `{"size":"big"}`, `"-9223372036854775808"`, "-9223372036854775808", ""},
		{Decimal, "", `"1.5"`, "1.50", ""},
		{Float, "", `0.1`, "0.1", ""},
		{Checkbox, "", `true`, "true", ""},
		{Checkbox, "", `null`, "false", ""},
		{Date, "", `"2024-02-29"`, "2024-02-29", ""},
		{DateTime, "", `"2026-10-26T16:30:00+08:00"`, "2026-10-26T08:30:00Z", ""},
		{SingleSelect, `{"choices":[{"id":"a","label":"A"}]}`, `"A"`, "A", ""},
		{Email, "", `"bad"`, "", ReasonInvalidEmail},
		{RowID, "", `"1"`, "", ReasonReadOnly},
		{CreatedBy, "", `null`, "", ReasonReadOnly},
	}
	for _, test := range cases {
		field := mustBind(t, test.kind, json.RawMessage(test.options))
		value, err := field.DefaultValue(json.RawMessage(test.raw))
		if test.reason != "" {
			if reasonOf(err) != test.reason {
				t.Errorf("%s %s: %v", test.kind, test.raw, err)
			}
			continue
		}
		if err != nil || value != (ddl.DefaultValue{Kind: ddl.Literal, Literal: test.literal}) {
			t.Errorf("%s %s: %+v %v", test.kind, test.raw, value, err)
		}
	}
	checkbox := mustBind(t, Checkbox, nil).Column()
	if !checkbox.NotNull || checkbox.Default != (ddl.DefaultValue{Kind: ddl.Literal, Literal: "false"}) {
		t.Fatalf("checkbox column %+v", checkbox)
	}
}

func TestEncodeScannedTypes(t *testing.T) {
	encode := func(kind Kind, options string, value any) string {
		t.Helper()
		encoded, err := mustBind(t, kind, json.RawMessage(options)).Encode(value)
		if err != nil {
			t.Fatalf("%s %#v: %v", kind, value, err)
		}
		return string(encoded)
	}
	checks := []struct{ got, want string }{
		{encode(Integer, `{"size":"small"}`, int16(-5)), "-5"},
		{encode(Integer, "", pgtype.Int4{Int32: 7, Valid: true}), "7"},
		{encode(Integer, `{"size":"big"}`, int64(math.MaxInt64)), `"9223372036854775807"`},
		{encode(Integer, `{"size":"big"}`, pgtype.Int8{}), "null"},
		{encode(Decimal, "", pgtype.Numeric{Int: big.NewInt(-1230), Exp: -3, Valid: true}), `"-1.23"`},
		{encode(Decimal, "", pgtype.Numeric{Int: big.NewInt(0), Exp: 0, Valid: true}), `"0.00"`},
		{encode(Decimal, "", pgtype.Numeric{Int: big.NewInt(5), Exp: 3, Valid: true}), `"5000.00"`},
		{encode(Float, `{"size":"real"}`, float32(0.1)), "0.1"},
		{encode(Float, "", float64(-0.0)), "0"},
		{encode(Checkbox, "", nil), "false"},
		{encode(Date, "", time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC)), `"2024-02-29"`},
		{encode(DateTime, "", time.Date(2026, 1, 1, 8, 0, 0, 1500, time.FixedZone("x", 8*3600))), `"2026-01-01T00:00:00.000001Z"`},
		{encode(MultiSelect, "", pgtype.FlatArray[string]{"a"}), `["a"]`},
		{encode(RowID, "", int64(9007199254740993)), `"9007199254740993"`},
		{encode(CreatedAt, "", pgtype.Timestamptz{Time: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), Valid: true}), `"2026-01-02T03:04:05Z"`},
		{encode(CreatedBy, "", [16]byte{0xAB, 0xCD, 15: 1}), `"abcd0000-0000-0000-0000-000000000001"`},
		{encode(UpdatedBy, "", pgtype.UUID{}), "null"},
		{encode(UpdatedBy, "", nil), "null"},
	}
	for index, check := range checks {
		if check.got != check.want {
			t.Errorf("case %d: got %s, want %s", index, check.got, check.want)
		}
	}
	failures := []struct {
		kind  Kind
		value any
	}{
		{Integer, "1"},
		{Decimal, pgtype.Numeric{NaN: true, Valid: true}},
		{Decimal, pgtype.Numeric{Int: big.NewInt(1), Exp: -3, Valid: true}},
		{Float, math.Inf(1)},
		{Float, math.NaN()},
		{Date, pgtype.Date{InfinityModifier: pgtype.Infinity, Valid: true}},
		{DateTime, pgtype.Timestamptz{InfinityModifier: pgtype.NegativeInfinity, Valid: true}},
		{Text, 12},
		{Checkbox, "true"},
		{RowID, "1"},
		{CreatedBy, "abcd"},
	}
	for _, failure := range failures {
		if _, err := mustBind(t, failure.kind, nil).Encode(failure.value); reasonOf(err) != ReasonUnsupportedDBType {
			t.Errorf("%s %#v: %v", failure.kind, failure.value, err)
		}
	}
}

func TestSystemFormatText(t *testing.T) {
	shanghai := envFor(t, "Asia/Shanghai")
	instant := time.Date(2026, 10, 26, 8, 30, 0, 0, time.UTC)
	if text, _ := mustBind(t, UpdatedAt, nil).FormatText(instant, shanghai); text != "2026-10-26 16:30:00+08:00" {
		t.Fatal(text)
	}
	if text, _ := mustBind(t, RowID, nil).FormatText(int64(42), shanghai); text != "42" {
		t.Fatal(text)
	}
	for _, kind := range []Kind{RowID, CreatedAt, CreatedBy, UpdatedAt, UpdatedBy} {
		field := mustBind(t, kind, nil)
		if _, err := field.ParsePaste("", shanghai); reasonOf(err) != ReasonReadOnly {
			t.Errorf("%s paste: %v", kind, err)
		}
	}
}

func TestLocalTimeLMTOffsets(t *testing.T) {
	// Historical zones can have second-level offsets; copy text keeps them so
	// it re-pastes exactly.
	env := envFor(t, "America/New_York")
	field := mustBind(t, DateTime, nil)
	instant := time.Date(1800, 1, 1, 12, 0, 0, 0, time.UTC)
	text, err := field.FormatText(instant, env)
	if err != nil || text != "1800-01-01 07:03:58-04:56:02" {
		t.Fatalf("%q %v", text, err)
	}
	pasted, err := field.ParsePaste(text, Env{})
	if err != nil || !pasted.(time.Time).Equal(instant) {
		t.Fatalf("%v %v", pasted, err)
	}
	resolved, err := ResolveLocal(time.Date(1800, 1, 1, 7, 3, 58, 0, time.UTC), env.Location)
	if err != nil || !resolved.Equal(instant) {
		t.Fatalf("%v %v", resolved, err)
	}
}

func TestFormatNumber(t *testing.T) {
	cases := []struct {
		value float64
		bits  int
		want  string
	}{
		{0, 64, "0"}, {1, 64, "1"}, {-1.5, 64, "-1.5"}, {1e20, 64, "100000000000000000000"},
		{1e21, 64, "1e+21"}, {1.25e-7, 64, "1.25e-7"}, {1e-7, 64, "1e-7"}, {9.99e-8, 64, "9.99e-8"},
		{1e-6, 64, "0.000001"}, {-1.5e-6, 64, "-0.0000015"}, {123.456, 64, "123.456"}, {-2.5e22, 64, "-2.5e+22"},
		{5e-324, 64, "5e-324"}, {math.MaxFloat64, 64, "1.7976931348623157e+308"},
		{float64(float32(0.1)), 32, "0.1"}, {float64(float32(1) / 3), 32, "0.33333334"},
	}
	for _, test := range cases {
		if got := FormatNumber(test.value, test.bits); got != test.want {
			t.Errorf("%v/%d: %s, want %s", test.value, test.bits, got, test.want)
		}
	}
}
