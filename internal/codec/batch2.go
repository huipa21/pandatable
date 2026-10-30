package codec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/text/currency"

	"pandatable/internal/ddl"
)

const (
	maxJSONBytes       = 64 * 1024
	maxJSONDepth       = 128
	maxArrayItems      = 1000
	maxArrayTextLength = 10000
	maxPhoneLength     = 64
)

var (
	wireTimePattern = regexp.MustCompile(`^([0-9]{2}):([0-9]{2}):([0-9]{2})(\.[0-9]{1,6})?$`)
	wireUUIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	phonePattern    = regexp.MustCompile(`^[+0-9(). xX#-]+$`)
	durationPattern = regexp.MustCompile(`^(-?)PT(?:(0|[1-9][0-9]*)H)?(?:(0|[1-9][0-9]*)M)?(?:(0|[1-9][0-9]*)(\.[0-9]{1,6})?S)?$`)
	uuidOps         = []Op{OpIs, OpIsNot, OpIsEmpty, OpIsNotEmpty}
	arrayOps        = []Op{OpHasAnyOf, OpHasAllOf, OpHasNoneOf, OpIsEmpty, OpIsNotEmpty}
	addressOps      = []Op{OpIs, OpIsNot, OpIsEmpty, OpIsNotEmpty, OpContainedIn}
)

type batch2Field struct {
	kind        Kind
	options     json.RawMessage
	column      Column
	constraints Constraints
	ops         []Op
	aggs        []Agg
	decode      func(json.RawMessage) (any, error)
	paste       func(string) (any, error)
	encode      func(any) (json.RawMessage, error)
	format      func(any) (string, error)
	defaultText func(any) (string, error)
}

func (field *batch2Field) Kind() Kind { return field.kind }
func (field *batch2Field) Options() json.RawMessage {
	return append(json.RawMessage(nil), field.options...)
}
func (field *batch2Field) ReadOnly() bool           { return field.column.AutoNumber }
func (field *batch2Field) Column() Column           { return field.column }
func (field *batch2Field) Constraints() Constraints { return field.constraints }
func (field *batch2Field) FilterOps() []Op          { return clone(field.ops) }
func (field *batch2Field) Aggregates() []Agg        { return clone(field.aggs) }
func (field *batch2Field) Decode(raw json.RawMessage) (any, error) {
	return field.decode(raw)
}
func (field *batch2Field) ParsePaste(text string, _ Env) (any, error) {
	return field.paste(text)
}
func (field *batch2Field) Encode(value any) (json.RawMessage, error) {
	return field.encode(value)
}
func (field *batch2Field) FormatText(value any, _ Env) (string, error) {
	return field.format(value)
}
func (field *batch2Field) DefaultValue(raw json.RawMessage) (ddl.DefaultValue, error) {
	value, err := field.decode(raw)
	if err != nil || value == nil {
		return ddl.DefaultValue{}, err
	}
	if field.ReadOnly() {
		return ddl.DefaultValue{}, fail(ReasonReadOnly)
	}
	text, err := field.defaultText(value)
	if err != nil {
		return ddl.DefaultValue{}, err
	}
	return ddl.DefaultValue{Kind: ddl.Literal, Literal: text}, nil
}

type currencyCodec struct{}

func (currencyCodec) Kind() Kind { return Currency }
func (codec currencyCodec) Bind(raw json.RawMessage) (Field, error) {
	set, err := parseOptions(raw)
	if err != nil {
		return nil, err
	}
	code := "CNY"
	if rawCode, ok := set.take("currency"); ok {
		if !jsonIs(rawCode, '"') || json.Unmarshal(rawCode, &code) != nil {
			return nil, failOption("currency")
		}
		if _, err := currency.ParseISO(code); err != nil {
			return nil, failOption("currency")
		}
	}
	scale, present, err := set.integer("scale", 0, 18)
	if err != nil {
		return nil, err
	}
	if !present {
		scale = 2
	}
	if err := set.done(); err != nil {
		return nil, err
	}
	options := struct {
		Currency string `json:"currency"`
		Scale    int    `json:"scale"`
	}{Currency: code, Scale: scale}
	field := decimalBackedField(Currency, options, 18, scale)
	field.column.Type = ddl.DataType{Kind: ddl.Numeric, Precision: 18, Scale: scale}
	field.constraints = Constraints{Required: true, Unique: true, Default: true}
	field.ops, field.aggs = clone(numericOps), clone(numericAggs)
	return field, nil
}

type percentCodec struct{}

func (percentCodec) Kind() Kind { return Percent }
func (percentCodec) Bind(raw json.RawMessage) (Field, error) {
	set, err := parseOptions(raw)
	if err != nil {
		return nil, err
	}
	display, present, err := set.integer("displayDecimals", 0, 4)
	if err != nil {
		return nil, err
	}
	if !present {
		display = 2
	}
	if err := set.done(); err != nil {
		return nil, err
	}
	options := struct {
		DisplayDecimals int `json:"displayDecimals"`
	}{DisplayDecimals: display}
	field := decimalBackedField(Percent, options, 7, 4)
	field.column.Type = ddl.DataType{Kind: ddl.Numeric, Precision: 7, Scale: 4}
	field.constraints = Constraints{Required: true, Unique: true, Default: true}
	field.ops, field.aggs = clone(numericOps), clone(numericAggs)
	return field, nil
}

func decimalBackedField(kind Kind, options any, precision, scale int) *batch2Field {
	encoded, _ := json.Marshal(options)
	decimal := &decimalField{options: decimalOptions{Precision: precision, Scale: scale}}
	field := &batch2Field{
		kind: kind, options: encoded,
		column:      Column{Type: ddl.DataType{Kind: ddl.Numeric, Precision: precision, Scale: scale}},
		constraints: Constraints{Required: true, Unique: true, Default: true},
		ops:         numericOps, aggs: numericAggs,
	}
	field.decode = decimal.Decode
	field.paste = func(text string) (any, error) { return decimal.ParsePaste(text, Env{}) }
	field.encode = decimal.Encode
	field.format = func(value any) (string, error) { return decimal.FormatText(value, Env{}) }
	field.defaultText = func(value any) (string, error) {
		encoded, err := decimal.Encode(value)
		if err != nil || isNull(encoded) {
			return "", err
		}
		var text string
		err = json.Unmarshal(encoded, &text)
		return text, err
	}
	return field
}

type ratingCodec struct{}

func (ratingCodec) Kind() Kind { return Rating }
func (ratingCodec) Bind(raw json.RawMessage) (Field, error) {
	set, err := parseOptions(raw)
	if err != nil {
		return nil, err
	}
	maxValue, present, err := set.integer("max", 1, 10)
	if err != nil {
		return nil, err
	}
	if !present {
		maxValue = 5
	}
	icon, err := set.enum("icon", "star", "star")
	if err != nil {
		return nil, err
	}
	if err := set.done(); err != nil {
		return nil, err
	}
	options := struct {
		Max  int    `json:"max"`
		Icon string `json:"icon"`
	}{Max: maxValue, Icon: icon}
	return newRatingField(options), nil
}

func newRatingField(options struct {
	Max  int    `json:"max"`
	Icon string `json:"icon"`
}) Field {
	encoded, _ := json.Marshal(options)
	return &batch2Field{
		kind: Rating, options: encoded,
		column:      Column{Type: ddl.DataType{Kind: ddl.SmallInt}, HasRangeCheck: true, CheckMin: 0, CheckMax: int64(options.Max)},
		constraints: Constraints{Required: true, Unique: true, Default: true},
		ops:         numericOps, aggs: numericAggs,
		decode: func(raw json.RawMessage) (any, error) {
			value, err := ratingValue(raw, options.Max, false)
			return value, err
		},
		paste: func(text string) (any, error) {
			if strings.TrimFunc(text, isTrimSpace) == "" {
				return nil, nil
			}
			return ratingValue(json.RawMessage(strconv.Quote(strings.TrimFunc(text, isTrimSpace))), options.Max, true)
		},
		encode:      encodeRating,
		format:      formatRating,
		defaultText: formatRating,
	}
}

func ratingValue(raw json.RawMessage, maxValue int, paste bool) (any, error) {
	if isNull(raw) {
		return nil, nil
	}
	text := string(trimJSONSpace(raw))
	if paste {
		pasteText, _, err := decodeString(raw)
		if err != nil {
			return nil, fail(ReasonInvalidRating)
		}
		text = pasteText
		text = normalizeNumberText(text)
		if !pasteInteger.MatchString(text) {
			return nil, fail(ReasonInvalidRating)
		}
		value, ok := parseDecimal(text)
		if !ok || !value.isInteger() {
			return nil, fail(ReasonInvalidRating)
		}
		text = value.format(0)
	} else if strings.HasPrefix(text, `"`) {
		textValue, _, err := decodeString(raw)
		if err != nil || !strictInteger.MatchString(textValue) {
			return nil, fail(ReasonInvalidRating)
		}
		text = textValue
	} else if !strictInteger.MatchString(text) {
		return nil, fail(ReasonInvalidType)
	}
	value, err := strconv.ParseInt(text, 10, 16)
	if err != nil || value < 0 || value > int64(maxValue) {
		return nil, fail(ReasonOutOfRange)
	}
	return int16(value), nil
}

func encodeRating(value any) (json.RawMessage, error) {
	number, ok, err := scanInteger(value)
	if err != nil || !ok {
		return nullJSON, err
	}
	return json.RawMessage(strconv.FormatInt(number, 10)), nil
}

func formatRating(value any) (string, error) {
	number, ok, err := scanInteger(value)
	if err != nil || !ok {
		return "", err
	}
	return strconv.FormatInt(number, 10), nil
}

type timeCodec struct{}

func (timeCodec) Kind() Kind { return Time }
func (timeCodec) Bind(raw json.RawMessage) (Field, error) {
	set, err := parseOptions(raw)
	if err != nil {
		return nil, err
	}
	format, err := set.enum("format", "24h", "24h", "12h")
	if err != nil {
		return nil, err
	}
	if err := set.done(); err != nil {
		return nil, err
	}
	options := struct {
		Format string `json:"format"`
	}{Format: format}
	encoded, _ := json.Marshal(options)
	return &batch2Field{kind: Time, options: encoded, column: Column{Type: ddl.DataType{Kind: ddl.Time}},
		constraints: Constraints{Required: true, Unique: true, Default: true},
		ops:         clone(timeOps), aggs: clone(temporalAggs),
		decode: decodeTime, paste: parseTimePaste, encode: encodeTime, format: formatTime, defaultText: formatTime}, nil
}

func decodeTime(raw json.RawMessage) (any, error) {
	text, ok, err := decodeString(raw)
	if err != nil || !ok || text == "" {
		return nil, err
	}
	return parseTimeValue(text)
}

func parseTimePaste(text string) (any, error) {
	text = strings.TrimFunc(text, isTrimSpace)
	if text == "" {
		return nil, nil
	}
	if _, err := parseTimeValue(text); err == nil {
		return parseTimeValue(text)
	}
	for _, layout := range []string{"3:04:05 PM", "3:04 PM"} {
		if value, err := time.Parse(layout, strings.ToUpper(text)); err == nil {
			return pgtype.Time{Microseconds: int64(value.Hour()*3600+value.Minute()*60+value.Second()) * 1_000_000, Valid: true}, nil
		}
	}
	return nil, fail(ReasonInvalidTime)
}

func parseTimeValue(text string) (pgtype.Time, error) {
	match := wireTimePattern.FindStringSubmatch(text)
	if match == nil {
		return pgtype.Time{}, fail(ReasonInvalidTime)
	}
	hour, _ := strconv.Atoi(match[1])
	minute, _ := strconv.Atoi(match[2])
	second, _ := strconv.Atoi(match[3])
	if hour > 23 || minute > 59 || second > 59 {
		return pgtype.Time{}, fail(ReasonInvalidTime)
	}
	fraction := strings.TrimPrefix(match[4], ".")
	fraction += strings.Repeat("0", 6-len(fraction))
	microsecond, _ := strconv.Atoi(fraction)
	micros := int64(((hour*60+minute)*60+second)*1_000_000 + microsecond)
	return pgtype.Time{Microseconds: micros, Valid: true}, nil
}

func scanTime(value any) (int64, bool, error) {
	switch typed := value.(type) {
	case nil:
		return 0, false, nil
	case pgtype.Time:
		return typed.Microseconds, typed.Valid, nil
	case time.Time:
		return int64(((typed.Hour()*60+typed.Minute())*60+typed.Second())*1_000_000 + typed.Nanosecond()/1_000), true, nil
	}
	return 0, false, fail(ReasonUnsupportedDBType)
}

func encodeTime(value any) (json.RawMessage, error) {
	text, err := formatTime(value)
	if err != nil || text == "" {
		return nullJSON, err
	}
	return marshal(text)
}

func formatTime(value any) (string, error) {
	micros, ok, err := scanTime(value)
	if err != nil || !ok {
		return "", err
	}
	if micros < 0 || micros >= 24*60*60*1_000_000 {
		return "", fail(ReasonUnsupportedDBType)
	}
	seconds, fraction := micros/1_000_000, micros%1_000_000
	text := fmt.Sprintf("%02d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
	if fraction != 0 {
		text += "." + strings.TrimRight(fmt.Sprintf("%06d", fraction), "0")
	}
	return text, nil
}

type durationCodec struct{}

func (durationCodec) Kind() Kind { return Duration }
func (durationCodec) Bind(raw json.RawMessage) (Field, error) {
	set, err := parseOptions(raw)
	if err != nil {
		return nil, err
	}
	format, err := set.enum("format", "h:mm", "h:mm", "h:mm:ss", "human")
	if err != nil {
		return nil, err
	}
	if err := set.done(); err != nil {
		return nil, err
	}
	options := struct {
		Format string `json:"format"`
	}{Format: format}
	encoded, _ := json.Marshal(options)
	return &batch2Field{kind: Duration, options: encoded, column: Column{Type: ddl.DataType{Kind: ddl.Interval}},
		constraints: Constraints{Required: true, Default: true}, ops: clone(numericOps), aggs: clone(numericAggs),
		decode: decodeDuration, paste: parseDurationPaste, encode: encodeDuration, format: formatDuration, defaultText: formatDuration}, nil
}

func decodeDuration(raw json.RawMessage) (any, error) {
	text, ok, err := decodeString(raw)
	if err != nil || !ok || text == "" {
		return nil, err
	}
	return parseDurationValue(text)
}

func parseDurationPaste(text string) (any, error) {
	text = strings.TrimFunc(text, isTrimSpace)
	if text == "" {
		return nil, nil
	}
	return parseDurationValue(text)
}

func parseDurationValue(text string) (pgtype.Interval, error) {
	match := durationPattern.FindStringSubmatch(text)
	if match == nil || match[2] == "" && match[3] == "" && match[4] == "" {
		return pgtype.Interval{}, fail(ReasonInvalidDuration)
	}
	hours, minutes, seconds := int64(0), int64(0), int64(0)
	var err error
	if match[2] != "" {
		hours, err = strconv.ParseInt(match[2], 10, 64)
		if err != nil {
			return pgtype.Interval{}, fail(ReasonOutOfRange)
		}
	}
	if match[3] != "" {
		minutes, err = strconv.ParseInt(match[3], 10, 64)
		if err != nil || minutes > 59 {
			return pgtype.Interval{}, fail(ReasonInvalidDuration)
		}
	}
	if match[4] != "" {
		seconds, err = strconv.ParseInt(match[4], 10, 64)
		if err != nil || seconds > 59 {
			return pgtype.Interval{}, fail(ReasonInvalidDuration)
		}
	}
	fraction := strings.TrimPrefix(match[5], ".")
	fraction += strings.Repeat("0", 6-len(fraction))
	microsFraction, _ := strconv.ParseInt(fraction, 10, 64)
	if hours > math.MaxInt64/3_600_000_000 {
		return pgtype.Interval{}, fail(ReasonOutOfRange)
	}
	total := hours * 3_600_000_000
	if minutes > (math.MaxInt64-total)/60_000_000 {
		return pgtype.Interval{}, fail(ReasonOutOfRange)
	}
	total += minutes * 60_000_000
	if seconds > (math.MaxInt64-total)/1_000_000 {
		return pgtype.Interval{}, fail(ReasonOutOfRange)
	}
	total += seconds * 1_000_000
	if microsFraction > math.MaxInt64-total {
		return pgtype.Interval{}, fail(ReasonOutOfRange)
	}
	total += microsFraction
	if match[1] == "-" {
		total = -total
	}
	return pgtype.Interval{Microseconds: total, Valid: true}, nil
}

func scanDuration(value any) (int64, bool, error) {
	switch typed := value.(type) {
	case nil:
		return 0, false, nil
	case pgtype.Interval:
		if !typed.Valid || typed.Months != 0 || typed.Days != 0 {
			return 0, false, fail(ReasonUnsupportedDBType)
		}
		return typed.Microseconds, true, nil
	}
	return 0, false, fail(ReasonUnsupportedDBType)
}

func encodeDuration(value any) (json.RawMessage, error) {
	text, err := formatDuration(value)
	if err != nil || text == "" {
		return nullJSON, err
	}
	return marshal(text)
}

func formatDuration(value any) (string, error) {
	micros, ok, err := scanDuration(value)
	if err != nil || !ok {
		return "", err
	}
	negative := micros < 0
	magnitude := uint64(micros)
	if negative {
		magnitude = uint64(-(micros + 1)) + 1
	}
	hours, rest := magnitude/3_600_000_000, magnitude%3_600_000_000
	minutes, rest := rest/60_000_000, rest%60_000_000
	seconds, fraction := rest/1_000_000, rest%1_000_000
	text := "PT"
	if negative {
		text = "-PT"
	}
	if hours != 0 {
		text += strconv.FormatUint(hours, 10) + "H"
	}
	if minutes != 0 {
		text += strconv.FormatUint(minutes, 10) + "M"
	}
	if fraction == 0 {
		if seconds != 0 || hours == 0 && minutes == 0 {
			text += strconv.FormatUint(seconds, 10) + "S"
		}
	} else {
		text += strconv.FormatUint(seconds, 10) + "." + strings.TrimRight(fmt.Sprintf("%06d", fraction), "0") + "S"
	}
	return text, nil
}

type phoneCodec struct{}

func (phoneCodec) Kind() Kind { return Phone }
func (phoneCodec) Bind(raw json.RawMessage) (Field, error) {
	return bindPlainText(Phone, raw, Constraints{Required: true, Unique: true, Default: true}, textOps, basicAggs)
}

type uuidCodec struct{}

func (uuidCodec) Kind() Kind { return UUID }
func (uuidCodec) Bind(raw json.RawMessage) (Field, error) {
	set, err := parseOptions(raw)
	if err != nil {
		return nil, err
	}
	autoGenerate, err := set.boolean("autoGenerate", false)
	if err != nil {
		return nil, err
	}
	if err := set.done(); err != nil {
		return nil, err
	}
	field, err := bindNoOptions(UUID, nil, ddl.DataType{Kind: ddl.UUID},
		Constraints{Required: true, Unique: true, Default: true}, uuidOps, basicAggs)
	if err != nil {
		return nil, err
	}
	options := struct {
		AutoGenerate bool `json:"autoGenerate"`
	}{AutoGenerate: autoGenerate}
	bound := field.(*batch2Field)
	bound.options, _ = json.Marshal(options)
	if autoGenerate {
		bound.column.Default = ddl.DefaultValue{Kind: ddl.UUIDv7}
	}
	return bound, nil
}

type jsonCodec struct{}

func (jsonCodec) Kind() Kind { return JSON }
func (jsonCodec) Bind(raw json.RawMessage) (Field, error) {
	return bindNoOptions(JSON, raw, ddl.DataType{Kind: ddl.JSONB}, Constraints{Required: true, Default: true},
		[]Op{OpIs, OpIsNot, OpIsEmpty, OpIsNotEmpty, OpHasKey, OpPathEquals}, basicAggs)
}

type inetCodec struct{}

func (inetCodec) Kind() Kind { return Inet }
func (inetCodec) Bind(raw json.RawMessage) (Field, error) {
	return bindNoOptions(Inet, raw, ddl.DataType{Kind: ddl.Inet},
		Constraints{Required: true, Unique: true, Default: true}, addressOps, basicAggs)
}

type cidrCodec struct{}

func (cidrCodec) Kind() Kind { return CIDR }
func (cidrCodec) Bind(raw json.RawMessage) (Field, error) {
	return bindNoOptions(CIDR, raw, ddl.DataType{Kind: ddl.CIDR},
		Constraints{Required: true, Default: true}, addressOps, basicAggs)
}

type arrayCodec struct{ kind Kind }

func (codec arrayCodec) Kind() Kind { return codec.kind }
func (codec arrayCodec) Bind(raw json.RawMessage) (Field, error) {
	typ := ddl.DataType{Kind: ddl.TextArray}
	if codec.kind == IntArray {
		typ.Kind = ddl.BigIntArray
	}
	return bindNoOptions(codec.kind, raw, typ, Constraints{Required: true, Default: true}, arrayOps, basicAggs)
}

type userCodec struct{ multi bool }

func (codec userCodec) Kind() Kind {
	if codec.multi {
		return MultiUser
	}
	return User
}
func (codec userCodec) Bind(raw json.RawMessage) (Field, error) {
	kind := User
	typ := ddl.DataType{Kind: ddl.UUID}
	reference := true
	ops, aggs := clone(userOps), clone(basicAggs)
	if codec.multi {
		kind, typ, reference = MultiUser, ddl.DataType{Kind: ddl.UUIDArray}, false
		ops = clone(arrayOps)
	}
	field, err := bindNoOptions(kind, raw, typ, Constraints{Required: true, Default: true}, ops, aggs)
	if err != nil {
		return nil, err
	}
	bound := field.(*batch2Field)
	bound.column.UserReference = reference
	return bound, nil
}

type autoNumberCodec struct{}

func (autoNumberCodec) Kind() Kind { return AutoNumber }
func (autoNumberCodec) Bind(raw json.RawMessage) (Field, error) {
	set, err := parseOptions(raw)
	if err != nil {
		return nil, err
	}
	prefixRaw, hasPrefix := set.take("prefix")
	prefix := ""
	if hasPrefix && (isNull(prefixRaw) || !jsonIs(prefixRaw, '"') || json.Unmarshal(prefixRaw, &prefix) != nil ||
		!validString(prefix, false, false) || utf8.RuneCountInString(prefix) > 32) {
		return nil, failOption("prefix")
	}
	padding, present, err := set.integer("padding", 0, 20)
	if err != nil {
		return nil, err
	}
	if !present {
		padding = 0
	}
	if err := set.done(); err != nil {
		return nil, err
	}
	options := struct {
		Prefix  string `json:"prefix,omitempty"`
		Padding int    `json:"padding,omitempty"`
	}{Prefix: prefix, Padding: padding}
	encoded, _ := json.Marshal(options)
	bound := &batch2Field{kind: AutoNumber, options: encoded, column: Column{
		Type: ddl.DataType{Kind: ddl.BigInt}, NotNull: true, AutoNumber: true,
	}, ops: withoutEmpty(numericOps), aggs: clone(numericAggs)}
	bound.encode = encodeBigintString
	bound.format = func(value any) (string, error) {
		number, ok, err := scanInteger(value)
		if err != nil || !ok {
			return "", err
		}
		text := strconv.FormatInt(number, 10)
		if padding > len(text) {
			text = strings.Repeat("0", padding-len(text)) + text
		}
		return prefix + text, nil
	}
	bound.defaultText = bound.format
	bound.decode = func(json.RawMessage) (any, error) { return nil, fail(ReasonReadOnly) }
	bound.paste = func(string) (any, error) { return nil, fail(ReasonReadOnly) }
	return bound, nil
}

func bindPlainText(kind Kind, raw json.RawMessage, constraints Constraints, ops []Op, aggs []Agg) (Field, error) {
	return bindNoOptions(kind, raw, ddl.DataType{Kind: ddl.Text}, constraints, ops, aggs)
}

func bindNoOptions(kind Kind, raw json.RawMessage, typ ddl.DataType, constraints Constraints, ops []Op, aggs []Agg) (Field, error) {
	set, err := parseOptions(raw)
	if err != nil {
		return nil, err
	}
	if err := set.done(); err != nil {
		return nil, err
	}
	encoded, _ := json.Marshal(struct{}{})
	field := &batch2Field{kind: kind, options: encoded,
		column: Column{Type: typ}, constraints: constraints, ops: clone(ops), aggs: clone(aggs)}
	field.decode = func(value json.RawMessage) (any, error) { return decodeBatch2Value(kind, value) }
	field.paste = func(value string) (any, error) { return parseBatch2Paste(kind, value) }
	field.encode = func(value any) (json.RawMessage, error) { return encodeBatch2Value(kind, value) }
	field.format = func(value any) (string, error) { return formatBatch2Value(kind, value) }
	field.defaultText = func(value any) (string, error) { return defaultBatch2Value(kind, value) }
	return field, nil
}

func decodeBatch2Value(kind Kind, raw json.RawMessage) (any, error) {
	if isNull(raw) {
		return nil, nil
	}
	switch kind {
	case Phone:
		text, _, err := decodeString(raw)
		if err != nil {
			return nil, err
		}
		return validatePhone(text)
	case UUID, User:
		text, _, err := decodeString(raw)
		if err != nil {
			return nil, err
		}
		if text == "" {
			return nil, nil
		}
		return parseUUID(text)
	case JSON:
		canonical, err := canonicalJSON(raw)
		if err != nil {
			return nil, err
		}
		return []byte(canonical), nil
	case Inet:
		text, _, err := decodeString(raw)
		if err != nil {
			return nil, err
		}
		if text == "" {
			return nil, nil
		}
		address, err := netip.ParseAddr(text)
		if err != nil || address.Zone() != "" {
			return nil, fail(ReasonInvalidIP)
		}
		return address, nil
	case CIDR:
		text, _, err := decodeString(raw)
		if err != nil {
			return nil, err
		}
		if text == "" {
			return nil, nil
		}
		prefix, err := netip.ParsePrefix(text)
		if err != nil || prefix.Addr().Zone() != "" || prefix != prefix.Masked() {
			return nil, fail(ReasonInvalidCIDR)
		}
		return prefix, nil
	case TextArray:
		return decodeTextArray(raw)
	case IntArray:
		return decodeIntArray(raw)
	case MultiUser:
		return decodeUserArray(raw)
	}
	return nil, fail(ReasonUnknownKind)
}

func parseBatch2Paste(kind Kind, text string) (any, error) {
	text = strings.TrimFunc(text, isTrimSpace)
	if text == "" {
		return nil, nil
	}
	switch kind {
	case Phone:
		return validatePhone(text)
	case UUID, User:
		return parseUUID(text)
	case JSON:
		return canonicalJSON(json.RawMessage(text))
	case Inet:
		return decodeBatch2Value(Inet, mustJSON(text))
	case CIDR:
		return decodeBatch2Value(CIDR, mustJSON(text))
	case TextArray:
		parts := strings.Split(text, ",")
		for i := range parts {
			parts[i] = strings.TrimFunc(parts[i], isTrimSpace)
		}
		raw, _ := json.Marshal(parts)
		return decodeTextArray(raw)
	case IntArray:
		parts := strings.Split(text, ",")
		items := make([]string, len(parts))
		for i, part := range parts {
			items[i] = strings.TrimFunc(part, isTrimSpace)
		}
		raw, _ := json.Marshal(items)
		return decodeIntArray(raw)
	case MultiUser:
		parts := strings.Split(text, ",")
		items := make([]string, len(parts))
		for i, part := range parts {
			items[i] = strings.TrimFunc(part, isTrimSpace)
		}
		raw, _ := json.Marshal(items)
		return decodeUserArray(raw)
	}
	return nil, fail(ReasonUnknownKind)
}

func mustJSON(value string) json.RawMessage {
	raw, _ := json.Marshal(value)
	return raw
}

func validatePhone(text string) (any, error) {
	text = strings.TrimFunc(text, isTrimSpace)
	if text == "" {
		return nil, nil
	}
	if !validString(text, false, false) || utf8.RuneCountInString(text) > maxPhoneLength ||
		!phonePattern.MatchString(text) || !strings.ContainsAny(text, "0123456789") {
		return nil, fail(ReasonInvalidPhone)
	}
	return text, nil
}

func parseUUID(text string) (any, error) {
	if !wireUUIDPattern.MatchString(text) {
		return nil, fail(ReasonInvalidUUID)
	}
	var value pgtype.UUID
	if err := value.Scan(text); err != nil {
		return nil, fail(ReasonInvalidUUID)
	}
	return value, nil
}

func decodeTextArray(raw json.RawMessage) (any, error) {
	var values []json.RawMessage
	if !jsonIs(raw, '[') || json.Unmarshal(raw, &values) != nil {
		return nil, fail(ReasonInvalidArray)
	}
	if len(values) == 0 {
		return nil, nil
	}
	if len(values) > maxArrayItems {
		return nil, fail(ReasonTooLong)
	}
	result := make([]string, len(values))
	for i, item := range values {
		text, ok, err := decodeString(item)
		if err != nil || !ok {
			return nil, fail(ReasonInvalidArray)
		}
		if !validString(text, true, false) || utf8.RuneCountInString(text) > maxArrayTextLength {
			return nil, fail(ReasonInvalidArray)
		}
		result[i] = text
	}
	return result, nil
}

func decodeIntArray(raw json.RawMessage) (any, error) {
	var values []json.RawMessage
	if !jsonIs(raw, '[') || json.Unmarshal(raw, &values) != nil {
		return nil, fail(ReasonInvalidArray)
	}
	if len(values) == 0 {
		return nil, nil
	}
	if len(values) > maxArrayItems {
		return nil, fail(ReasonTooLong)
	}
	result := make([]int64, len(values))
	for i, item := range values {
		text, ok, err := decodeString(item)
		if err != nil || !ok || !strictInteger.MatchString(text) {
			return nil, fail(ReasonInvalidArray)
		}
		number, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return nil, fail(ReasonOutOfRange)
		}
		result[i] = number
	}
	return result, nil
}

func decodeUserArray(raw json.RawMessage) (any, error) {
	var values []json.RawMessage
	if !jsonIs(raw, '[') || json.Unmarshal(raw, &values) != nil {
		return nil, fail(ReasonInvalidArray)
	}
	if len(values) == 0 {
		return nil, nil
	}
	if len(values) > maxArrayItems {
		return nil, fail(ReasonTooLong)
	}
	result := make([]pgtype.UUID, len(values))
	seen := map[string]bool{}
	for i, item := range values {
		text, ok, err := decodeString(item)
		if err != nil || !ok {
			return nil, fail(ReasonInvalidArray)
		}
		value, err := parseUUID(text)
		if err != nil {
			return nil, err
		}
		if seen[text] {
			return nil, fail(ReasonDuplicateChoice)
		}
		seen[text] = true
		result[i] = value.(pgtype.UUID)
	}
	return result, nil
}

func encodeBatch2Value(kind Kind, value any) (json.RawMessage, error) {
	if value == nil {
		return nullJSON, nil
	}
	switch kind {
	case Phone:
		text, ok, err := scanText(value)
		if err != nil || !ok {
			return nullJSON, err
		}
		return marshal(text)
	case UUID, User:
		text, ok, err := scanUUID(value)
		if err != nil || !ok {
			return nullJSON, err
		}
		return marshal(text)
	case JSON:
		raw, err := scannedJSON(value)
		if err != nil {
			return nil, err
		}
		return raw, nil
	case Inet:
		address, ok, err := scanAddress(value)
		if err != nil || !ok {
			return nullJSON, err
		}
		return marshal(address.String())
	case CIDR:
		prefix, ok, err := scanPrefix(value)
		if err != nil || !ok {
			return nullJSON, err
		}
		return marshal(prefix.String())
	case TextArray:
		items, ok, err := scanArray[string](value)
		if err != nil || !ok {
			return nullJSON, err
		}
		if len(items) == 0 {
			return nullJSON, nil
		}
		return marshal(items)
	case IntArray:
		items, ok, err := scanArray[int64](value)
		if err != nil || !ok {
			return nullJSON, err
		}
		if len(items) == 0 {
			return nullJSON, nil
		}
		result := make([]string, len(items))
		for i, item := range items {
			result[i] = strconv.FormatInt(item, 10)
		}
		return marshal(result)
	case MultiUser:
		items, ok, err := scanArray[pgtype.UUID](value)
		if err != nil || !ok {
			return nullJSON, err
		}
		if len(items) == 0 {
			return nullJSON, nil
		}
		result := make([]string, len(items))
		for i, item := range items {
			text, valid, err := scanUUID(item)
			if err != nil || !valid {
				return nullJSON, err
			}
			result[i] = text
		}
		return marshal(result)
	case AutoNumber:
		return encodeBigintString(value)
	}
	return nil, fail(ReasonUnknownKind)
}

func encodeBigintString(value any) (json.RawMessage, error) {
	number, ok, err := scanInteger(value)
	if err != nil || !ok {
		return nullJSON, err
	}
	return marshal(strconv.FormatInt(number, 10))
}

func scanArray[T any](value any) ([]T, bool, error) {
	switch typed := value.(type) {
	case []T:
		return typed, true, nil
	case pgtype.FlatArray[T]:
		return []T(typed), true, nil
	case []any:
		result := make([]T, len(typed))
		for i, item := range typed {
			converted := item
			if bytes, ok := item.([16]byte); ok {
				converted = pgtype.UUID{Bytes: bytes, Valid: true}
			}
			value, ok := converted.(T)
			if !ok {
				return nil, false, fail(ReasonUnsupportedDBType)
			}
			result[i] = value
		}
		return result, true, nil
	}
	return nil, false, fail(ReasonUnsupportedDBType)
}

func scanAddress(value any) (netip.Addr, bool, error) {
	switch typed := value.(type) {
	case netip.Addr:
		return typed, true, nil
	case netip.Prefix:
		return typed.Addr(), true, nil
	}
	return netip.Addr{}, false, fail(ReasonUnsupportedDBType)
}

func scanPrefix(value any) (netip.Prefix, bool, error) {
	switch typed := value.(type) {
	case netip.Prefix:
		return typed, true, nil
	}
	return netip.Prefix{}, false, fail(ReasonUnsupportedDBType)
}

func scannedJSON(value any) (json.RawMessage, error) {
	var raw []byte
	switch typed := value.(type) {
	case []byte:
		raw = typed
	case json.RawMessage:
		raw = typed
	case string:
		raw = []byte(typed)
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, fail(ReasonUnsupportedDBType)
		}
		raw = encoded
	}
	if isNull(raw) {
		return nullJSON, nil
	}
	return canonicalJSON(raw)
}

func canonicalJSON(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) > maxJSONBytes || !json.Valid(raw) || hasLoneSurrogateEscape(raw) {
		if len(raw) > maxJSONBytes {
			return nil, fail(ReasonTooLong)
		}
		return nil, fail(ReasonInvalidJSON)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := readJSONValue(decoder, 0)
	if err != nil {
		return nil, fail(ReasonInvalidJSON)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, fail(ReasonInvalidJSON)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fail(ReasonInvalidJSON)
	}
	if len(encoded) > maxJSONBytes {
		return nil, fail(ReasonTooLong)
	}
	return encoded, nil
}

func readJSONValue(decoder *json.Decoder, depth int) (any, error) {
	if depth > maxJSONDepth {
		return nil, errors.New("JSON nesting exceeds limit")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch typed := token.(type) {
	case json.Delim:
		switch typed {
		case '{':
			object := map[string]any{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, errors.New("invalid object key")
				}
				if _, exists := object[key]; exists {
					return nil, errors.New("duplicate object key")
				}
				value, err := readJSONValue(decoder, depth+1)
				if err != nil {
					return nil, err
				}
				object[key] = value
			}
			_, err := decoder.Token()
			return object, err
		case '[':
			items := []any{}
			for decoder.More() {
				value, err := readJSONValue(decoder, depth+1)
				if err != nil {
					return nil, err
				}
				items = append(items, value)
			}
			_, err := decoder.Token()
			return items, err
		}
	case json.Number, string, bool, nil:
		return typed, nil
	}
	return nil, errors.New("invalid JSON value")
}

func formatBatch2Value(kind Kind, value any) (string, error) {
	if value == nil {
		return "", nil
	}
	switch kind {
	case Phone:
		text, ok, err := scanText(value)
		if err != nil || !ok {
			return "", err
		}
		return text, nil
	case UUID, User:
		text, ok, err := scanUUID(value)
		if err != nil || !ok {
			return "", err
		}
		return text, nil
	case JSON:
		raw, err := scannedJSON(value)
		return string(raw), err
	case Inet:
		address, ok, err := scanAddress(value)
		if err != nil || !ok {
			return "", err
		}
		return address.String(), nil
	case CIDR:
		prefix, ok, err := scanPrefix(value)
		if err != nil || !ok {
			return "", err
		}
		return prefix.String(), nil
	case TextArray:
		items, ok, err := scanArray[string](value)
		if err != nil || !ok {
			return "", err
		}
		return strings.Join(items, ", "), nil
	case IntArray:
		items, ok, err := scanArray[int64](value)
		if err != nil || !ok {
			return "", err
		}
		parts := make([]string, len(items))
		for i, item := range items {
			parts[i] = strconv.FormatInt(item, 10)
		}
		return strings.Join(parts, ", "), nil
	case MultiUser:
		encoded, err := encodeBatch2Value(kind, value)
		if err != nil || isNull(encoded) {
			return "", err
		}
		var parts []string
		err = json.Unmarshal(encoded, &parts)
		return strings.Join(parts, ", "), err
	case AutoNumber:
		number, ok, err := scanInteger(value)
		if err != nil || !ok {
			return "", err
		}
		return strconv.FormatInt(number, 10), nil
	}
	return "", fail(ReasonUnknownKind)
}

func defaultBatch2Value(kind Kind, value any) (string, error) {
	switch kind {
	case JSON:
		raw, err := scannedJSON(value)
		return string(raw), err
	case TextArray:
		items, ok, err := scanArray[string](value)
		if err != nil || !ok {
			return "", err
		}
		quoted := make([]string, len(items))
		for i, item := range items {
			item = strings.ReplaceAll(item, `\`, `\\`)
			item = strings.ReplaceAll(item, `"`, `\"`)
			quoted[i] = `"` + item + `"`
		}
		return "{" + strings.Join(quoted, ",") + "}", nil
	case IntArray:
		items, ok, err := scanArray[int64](value)
		if err != nil || !ok {
			return "", err
		}
		parts := make([]string, len(items))
		for i, item := range items {
			parts[i] = strconv.FormatInt(item, 10)
		}
		return "{" + strings.Join(parts, ",") + "}", nil
	case MultiUser:
		items, ok, err := scanArray[pgtype.UUID](value)
		if err != nil || !ok {
			return "", err
		}
		parts := make([]string, len(items))
		for i, item := range items {
			text, valid, err := scanUUID(item)
			if err != nil || !valid {
				return "", err
			}
			parts[i] = text
		}
		return "{" + strings.Join(parts, ",") + "}", nil
	case UUID, User:
		return formatBatch2Value(kind, value)
	default:
		return formatBatch2Value(kind, value)
	}
}
