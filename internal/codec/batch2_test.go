package codec

import (
	"encoding/json"
	"math"
	"net/netip"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/ddl"
)

func TestBatch2CodecValues(t *testing.T) {
	tests := []struct {
		kind    Kind
		options string
		wire    string
		want    any
		encoded string
	}{
		{Time, `{}`, `"12:34:56.1234"`, pgtype.Time{Microseconds: 45_296_123_400, Valid: true}, `"12:34:56.1234"`},
		{Duration, `{}`, `"-PT2H3M4.5S"`, pgtype.Interval{Microseconds: -7_384_500_000, Valid: true}, `"-PT2H3M4.5S"`},
		{Currency, `{"currency":"USD","scale":2}`, `"99.95"`, pgtype.Numeric{}, `"99.95"`},
		{Percent, `{}`, `"0.2500"`, pgtype.Numeric{}, `"0.2500"`},
		{Rating, `{"max":10}`, `10`, int16(10), `10`},
		{Phone, `{}`, `"+1 (212) 555-0100 x5"`, "+1 (212) 555-0100 x5", `"+1 (212) 555-0100 x5"`},
		{UUID, `{}`, `"00000000-0000-0000-0000-000000000001"`, pgtype.UUID{}, `"00000000-0000-0000-0000-000000000001"`},
		{User, `{}`, `"00000000-0000-0000-0000-000000000001"`, pgtype.UUID{}, `"00000000-0000-0000-0000-000000000001"`},
		{JSON, `{}`, `{"z":1.0,"a":[true]}`, []byte(`{"a":[true],"z":1.0}`), `{"a":[true],"z":1.0}`},
		{Inet, `{}`, `"2001:db8::1"`, netip.MustParseAddr("2001:db8::1"), `"2001:db8::1"`},
		{CIDR, `{}`, `"192.0.2.0/24"`, netip.MustParsePrefix("192.0.2.0/24"), `"192.0.2.0/24"`},
		{TextArray, `{}`, `["one","two"]`, []string{"one", "two"}, `["one","two"]`},
		{IntArray, `{}`, `["9223372036854775807","-2"]`, []int64{math.MaxInt64, -2}, `["9223372036854775807","-2"]`},
		{MultiUser, `{}`, `["00000000-0000-0000-0000-000000000001"]`, []pgtype.UUID{{Bytes: [16]byte{15: 1}, Valid: true}}, `["00000000-0000-0000-0000-000000000001"]`},
		{AutoNumber, `{}`, `"9223372036854775807"`, int64(math.MaxInt64), `"9223372036854775807"`},
	}

	for _, test := range tests {
		t.Run(string(test.kind), func(t *testing.T) {
			field := mustBind(t, test.kind, json.RawMessage(test.options))
			if test.kind == AutoNumber {
				if !field.ReadOnly() || reasonOf(mustDecodeErr(field)) != ReasonReadOnly {
					t.Fatal("autonumber must be read-only")
				}
				encoded, err := field.Encode(int64(42))
				if err != nil || string(encoded) != `"42"` {
					t.Fatalf("autonumber wire = %s, %v", encoded, err)
				}
				formatted, err := field.FormatText(int64(42), Env{})
				if err != nil || formatted != "42" {
					t.Fatalf("autonumber text = %q, %v", formatted, err)
				}
				return
			}
			value, err := field.Decode(json.RawMessage(test.wire))
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			switch test.kind {
			case Currency, Percent:
				if value == nil {
					t.Fatal("expected a numeric value")
				}
			case UUID, User:
				if _, ok := value.(pgtype.UUID); !ok {
					t.Fatalf("Decode type %T", value)
				}
			default:
				if !reflect.DeepEqual(value, test.want) {
					t.Fatalf("Decode = %#v (%T), want %#v (%T)", value, value, test.want, test.want)
				}
			}
			encoded, err := field.Encode(value)
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}
			if string(encoded) != test.encoded {
				t.Fatalf("Encode = %s, want %s", encoded, test.encoded)
			}
			formatted, err := field.FormatText(value, Env{})
			if err != nil {
				t.Fatalf("FormatText: %v", err)
			}
			if formatted == "" {
				t.Fatal("FormatText unexpectedly empty")
			}
		})
	}
}

func mustDecodeErr(field Field) error {
	_, err := field.Decode(json.RawMessage(`"1"`))
	return err
}

func TestBatch2OptionsAndPhysicalDescriptors(t *testing.T) {
	for _, test := range []struct {
		kind    Kind
		options string
		column  Column
	}{
		{Currency, `{"currency":"JPY","scale":0}`, Column{Type: ddlNumeric(18, 0)}},
		{Percent, `{"displayDecimals":4}`, Column{Type: ddlNumeric(7, 4)}},
		{Rating, `{"max":10}`, Column{Type: ddlSmallInt(), HasRangeCheck: true, CheckMin: 0, CheckMax: 10}},
		{UUID, `{"autoGenerate":true}`, Column{Type: ddlUUID(), Default: ddl.DefaultValue{Kind: ddl.UUIDv7}}},
		{User, `{}`, Column{Type: ddlUUID(), UserReference: true}},
		{MultiUser, `{}`, Column{Type: ddlUUIDArray()}},
		{AutoNumber, `{"prefix":"INV-","padding":6}`, Column{Type: ddlBigInt(), NotNull: true, AutoNumber: true}},
	} {
		field := mustBind(t, test.kind, json.RawMessage(test.options))
		if field.Column() != test.column {
			t.Errorf("%s column %#v, want %#v", test.kind, field.Column(), test.column)
		}
		if got, err := mustBind(t, AutoNumber, json.RawMessage(`{"prefix":"INV-","padding":6}`)).FormatText(int64(42), Env{}); err != nil || got != "INV-000042" {
			t.Fatalf("autonumber display = %q, %v", got, err)
		}
		rebound, err := Bind(test.kind, field.Options())
		if err != nil || string(rebound.Options()) != string(field.Options()) {
			t.Errorf("%s options not canonical: %s (%v)", test.kind, field.Options(), err)
		}
	}

	for _, test := range []struct {
		kind Kind
		raw  string
	}{
		{Currency, `{"currency":"ZZZ"}`},
		{Currency, `{"scale":19}`},
		{Percent, `{"displayDecimals":5}`},
		{Rating, `{"max":0}`},
		{Rating, `{"max":5,"unknown":true}`},
		{Duration, `{"format":"months"}`},
		{UUID, `{"autoGenerate":"yes"}`},
		{AutoNumber, `{"padding":21}`},
	} {
		if _, err := Bind(test.kind, json.RawMessage(test.raw)); reasonOf(err) != ReasonInvalidOptions {
			t.Errorf("%s %s: got %v", test.kind, test.raw, err)
		}
	}
}

func TestBatch2InvalidValues(t *testing.T) {
	for _, test := range []struct {
		kind Kind
		raw  string
		want string
	}{
		{Time, `"24:00:00"`, ReasonInvalidTime},
		{Duration, `"P1D"`, ReasonInvalidDuration},
		{Rating, `11`, ReasonOutOfRange},
		{Phone, `"not a phone"`, ReasonInvalidPhone},
		{UUID, `"00000000-0000-0000-0000-00000000000A"`, ReasonInvalidUUID},
		{JSON, `{"x":1,"x":2}`, ReasonInvalidJSON},
		{Inet, `"fe80::1%eth0"`, ReasonInvalidIP},
		{CIDR, `"192.0.2.1/24"`, ReasonInvalidCIDR},
		{TextArray, `[1]`, ReasonInvalidArray},
		{IntArray, `[1]`, ReasonInvalidArray},
		{IntArray, `["9223372036854775808"]`, ReasonOutOfRange},
		{MultiUser, `["00000000-0000-0000-0000-000000000001","00000000-0000-0000-0000-000000000001"]`, ReasonDuplicateChoice},
	} {
		field := mustBind(t, test.kind, nil)
		if _, err := field.Decode(json.RawMessage(test.raw)); reasonOf(err) != test.want {
			t.Errorf("%s %s: reason %q, want %q", test.kind, test.raw, reasonOf(err), test.want)
		}
	}
	if _, err := mustBind(t, AutoNumber, nil).DefaultValue(json.RawMessage(`"1"`)); reasonOf(err) != ReasonReadOnly {
		t.Fatalf("autonumber default: %v", err)
	}
}

func TestBatch2EmptyAndCanonicalJSON(t *testing.T) {
	for _, kind := range []Kind{TextArray, IntArray, MultiUser} {
		field := mustBind(t, kind, nil)
		value, err := field.Decode(json.RawMessage(`[]`))
		if err != nil || value != nil {
			t.Errorf("%s empty array = %#v, %v", kind, value, err)
		}
	}
	jsonField := mustBind(t, JSON, nil)
	canonical, err := jsonField.Decode(json.RawMessage(`{"b":2,"a":{"y":true,"x":null}}`))
	if err != nil || string(canonical.([]byte)) != `{"a":{"x":null,"y":true},"b":2}` {
		t.Fatalf("canonical JSON = %s, %v", canonical, err)
	}
	tooLarge := json.RawMessage(`"` + strings.Repeat("x", maxJSONBytes) + `"`)
	if _, err := jsonField.Decode(tooLarge); reasonOf(err) != ReasonTooLong {
		t.Fatalf("oversized JSON: %v", err)
	}
	escapedTooLarge := json.RawMessage(`"` + strings.Repeat("<", maxJSONBytes/6+1) + `"`)
	if _, err := jsonField.Decode(escapedTooLarge); reasonOf(err) != ReasonTooLong {
		t.Fatalf("oversized canonical JSON: %v", err)
	}
	deep := json.RawMessage(strings.Repeat("[", maxJSONDepth+1) + "0" + strings.Repeat("]", maxJSONDepth+1))
	if _, err := jsonField.Decode(deep); reasonOf(err) != ReasonInvalidJSON {
		t.Fatalf("overly nested JSON: %v", err)
	}
}

func ddlNumeric(precision, scale int) ddl.DataType {
	return ddl.DataType{Kind: ddl.Numeric, Precision: precision, Scale: scale}
}

func ddlSmallInt() ddl.DataType { return ddl.DataType{Kind: ddl.SmallInt} }
func ddlUUID() ddl.DataType     { return ddl.DataType{Kind: ddl.UUID} }
func ddlUUIDArray() ddl.DataType {
	return ddl.DataType{Kind: ddl.UUIDArray}
}
func ddlBigInt() ddl.DataType { return ddl.DataType{Kind: ddl.BigInt} }
