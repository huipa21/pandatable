package codec

import (
	"encoding/json"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/ddl"
)

const maxSafeInteger = 1<<53 - 1

var (
	strictInteger = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)
	jsonNumber    = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)
	pasteInteger  = regexp.MustCompile(`^[+-]?[0-9]+(\.0+)?$`)
	pasteNumber   = regexp.MustCompile(`^[+-]?([0-9]+\.?[0-9]*|\.[0-9]+)([eE][+-]?[0-9]+)?$`)
)

// normalizeNumberText implements D-116's lenient numeric paste normalization:
// trim, map full-width forms and drop grouping separators.
func normalizeNumberText(text string) string {
	text = strings.TrimFunc(text, isTrimSpace)
	var builder strings.Builder
	for _, r := range text {
		switch {
		case r >= '０' && r <= '９':
			builder.WriteRune('0' + r - '０')
		case r == '＋':
			builder.WriteByte('+')
		case r == '－' || r == '−':
			builder.WriteByte('-')
		case r == '．':
			builder.WriteByte('.')
		case r == 'ｅ' || r == 'Ｅ':
			builder.WriteByte('e')
		case r == ',' || r == '，' || r == '_' || r == ' ' || r == '\u00A0' || r == '\u202F' || r == '\u2009' || r == '\'':
		default:
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

// decimalValue is value = digits × 10^exp with digits free of leading and
// trailing zeros ("" means zero).
type decimalValue struct {
	negative bool
	digits   string
	exp      int64
}

const maxNumberText = 1000

// parseDecimal parses [+-]digits[.digits][e±digits] (grammar already checked).
func parseDecimal(text string) (decimalValue, bool) {
	if len(text) > maxNumberText {
		return decimalValue{}, false
	}
	var value decimalValue
	if text != "" && (text[0] == '-' || text[0] == '+') {
		value.negative = text[0] == '-'
		text = text[1:]
	}
	mantissa, exponent, hasExp := strings.Cut(strings.ToLower(text), "e")
	whole, fraction, _ := strings.Cut(mantissa, ".")
	digits := whole + fraction
	exp := -int64(len(fraction))
	if hasExp {
		parsed, err := strconv.ParseInt(exponent, 10, 64)
		if err != nil || parsed > 1e9 || parsed < -1e9 {
			// Bounded so later arithmetic cannot overflow; such magnitudes are
			// rejected by range/scale checks unless the mantissa is zero.
			if strings.Trim(digits, "0") != "" {
				if strings.HasPrefix(exponent, "-") {
					parsed = -1e9
				} else {
					parsed = 1e9
				}
			} else {
				parsed = 0
			}
		}
		exp += parsed
	}
	digits = strings.TrimLeft(digits, "0")
	trimmed := strings.TrimRight(digits, "0")
	exp += int64(len(digits) - len(trimmed))
	value.digits = trimmed
	if value.digits == "" {
		value.negative = false
		value.exp = 0
	} else {
		value.exp = exp
	}
	return value, true
}

// integerDigits counts digits left of the decimal point.
func (value decimalValue) integerDigits() int64 {
	if value.digits == "" {
		return 0
	}
	return max(0, int64(len(value.digits))+value.exp)
}

func (value decimalValue) fractionDigits() int64 { return max(0, -value.exp) }

// format renders the value with exactly scale fraction digits; callers have
// checked fractionDigits() <= scale and a bounded integer part.
func (value decimalValue) format(scale int) string {
	var whole, fraction string
	switch {
	case value.digits == "":
		whole = "0"
	case value.exp >= 0:
		whole = value.digits + strings.Repeat("0", int(value.exp))
	default:
		k := int(-value.exp)
		if len(value.digits) > k {
			whole, fraction = value.digits[:len(value.digits)-k], value.digits[len(value.digits)-k:]
		} else {
			whole, fraction = "0", strings.Repeat("0", k-len(value.digits))+value.digits
		}
	}
	fraction += strings.Repeat("0", scale-len(fraction))
	result := whole
	if scale > 0 {
		result += "." + fraction
	}
	if value.negative {
		result = "-" + result
	}
	return result
}

func (value decimalValue) significantDigits() int { return len(value.digits) }

func (value decimalValue) isInteger() bool { return value.digits == "" || value.exp >= 0 }

func (value decimalValue) bigInt() *big.Int {
	result, _ := new(big.Int).SetString(value.format(0), 10)
	return result
}

// ---- integer ----

type integerCodec struct{}

func (integerCodec) Kind() Kind { return Integer }

type integerOptions struct {
	Size        string `json:"size"`
	ThousandSep bool   `json:"thousandSep"`
}

func (integerCodec) Bind(raw json.RawMessage) (Field, error) {
	set, err := parseOptions(raw)
	if err != nil {
		return nil, err
	}
	size, err := set.enum("size", "int", "small", "int", "big")
	if err != nil {
		return nil, err
	}
	separator, err := set.boolean("thousandSep", true)
	if err != nil {
		return nil, err
	}
	if err := set.done(); err != nil {
		return nil, err
	}
	field := &integerField{options: integerOptions{Size: size, ThousandSep: separator}}
	switch size {
	case "small":
		field.low, field.high, field.dataType = math.MinInt16, math.MaxInt16, ddl.SmallInt
	case "int":
		field.low, field.high, field.dataType = math.MinInt32, math.MaxInt32, ddl.Integer
	default:
		field.low, field.high, field.dataType = math.MinInt64, math.MaxInt64, ddl.BigInt
	}
	return field, nil
}

type integerField struct {
	options   integerOptions
	low, high int64
	dataType  ddl.TypeKind
}

func (field *integerField) Kind() Kind     { return Integer }
func (field *integerField) ReadOnly() bool { return false }
func (field *integerField) Options() json.RawMessage {
	encoded, _ := json.Marshal(field.options)
	return encoded
}
func (field *integerField) Column() Column { return Column{Type: ddl.DataType{Kind: field.dataType}} }
func (field *integerField) Constraints() Constraints {
	return Constraints{Required: true, Unique: true, Default: true}
}
func (field *integerField) FilterOps() []Op   { return clone(numericOps) }
func (field *integerField) Aggregates() []Agg { return clone(numericAggs) }

func (field *integerField) Decode(raw json.RawMessage) (any, error) {
	if isNull(raw) {
		return nil, nil
	}
	trimmed := string(trimJSONSpace(raw))
	if trimmed[0] == '"' {
		text, _, err := decodeString(raw)
		if err != nil {
			return nil, err
		}
		if text == "" {
			return nil, nil
		}
		if !strictInteger.MatchString(text) {
			return nil, fail(ReasonInvalidInteger)
		}
		return field.fromDecimal(text)
	}
	if !jsonNumber.MatchString(trimmed) {
		return nil, fail(ReasonInvalidType)
	}
	value, ok := parseDecimal(trimmed)
	if !ok {
		return nil, fail(ReasonInvalidInteger)
	}
	if !value.isInteger() {
		return nil, fail(ReasonInvalidInteger)
	}
	if value.integerDigits() > 16 || value.bigInt().CmpAbs(big.NewInt(maxSafeInteger)) > 0 {
		return nil, fail(ReasonImpreciseNumber)
	}
	return field.fromDecimal(value.format(0))
}

func (field *integerField) ParsePaste(text string, _ Env) (any, error) {
	text = normalizeNumberText(text)
	if text == "" {
		return nil, nil
	}
	if !pasteInteger.MatchString(text) {
		return nil, fail(ReasonInvalidInteger)
	}
	value, ok := parseDecimal(text)
	if !ok {
		return nil, fail(ReasonInvalidInteger)
	}
	return field.fromDecimal(value.format(0))
}

// fromDecimal range-checks canonical integer digits and returns the typed
// parameter for the column size.
func (field *integerField) fromDecimal(text string) (any, error) {
	if len(strings.TrimLeft(strings.TrimPrefix(text, "-"), "0")) > 19 {
		return nil, fail(ReasonOutOfRange)
	}
	parsed, err := strconv.ParseInt(text, 10, 64)
	if err != nil || parsed < field.low || parsed > field.high {
		return nil, fail(ReasonOutOfRange)
	}
	switch field.dataType {
	case ddl.SmallInt:
		return int16(parsed), nil
	case ddl.Integer:
		return int32(parsed), nil
	}
	return parsed, nil
}

func (field *integerField) Encode(value any) (json.RawMessage, error) {
	number, ok, err := scanInteger(value)
	if err != nil || !ok {
		return nullJSON, err
	}
	if field.dataType == ddl.BigInt {
		return marshal(strconv.FormatInt(number, 10))
	}
	return json.RawMessage(strconv.FormatInt(number, 10)), nil
}

func (field *integerField) FormatText(value any, _ Env) (string, error) {
	number, ok, err := scanInteger(value)
	if err != nil || !ok {
		return "", err
	}
	return strconv.FormatInt(number, 10), nil
}

func (field *integerField) DefaultValue(raw json.RawMessage) (ddl.DefaultValue, error) {
	value, err := field.Decode(raw)
	if err != nil || value == nil {
		return ddl.DefaultValue{}, err
	}
	number, _, _ := scanInteger(value)
	return ddl.DefaultValue{Kind: ddl.Literal, Literal: strconv.FormatInt(number, 10)}, nil
}

func scanInteger(value any) (int64, bool, error) {
	switch typed := value.(type) {
	case nil:
		return 0, false, nil
	case int16:
		return int64(typed), true, nil
	case int32:
		return int64(typed), true, nil
	case int64:
		return typed, true, nil
	case int:
		return int64(typed), true, nil
	case pgtype.Int2:
		return int64(typed.Int16), typed.Valid, nil
	case pgtype.Int4:
		return int64(typed.Int32), typed.Valid, nil
	case pgtype.Int8:
		return typed.Int64, typed.Valid, nil
	}
	return 0, false, fail(ReasonUnsupportedDBType)
}

// ---- decimal ----

type decimalCodec struct{}

func (decimalCodec) Kind() Kind { return Decimal }

type decimalOptions struct {
	Precision   int  `json:"precision"`
	Scale       int  `json:"scale"`
	ThousandSep bool `json:"thousandSep"`
}

const maxDecimalPrecision = 38

func (decimalCodec) Bind(raw json.RawMessage) (Field, error) {
	set, err := parseOptions(raw)
	if err != nil {
		return nil, err
	}
	precision, present, err := set.integer("precision", 1, maxDecimalPrecision)
	if err != nil {
		return nil, err
	}
	if !present {
		precision = 18
	}
	scale, present, err := set.integer("scale", 0, maxDecimalPrecision)
	if err != nil {
		return nil, err
	}
	if !present {
		scale = min(2, precision)
	}
	if scale > precision {
		return nil, failOption("scale")
	}
	separator, err := set.boolean("thousandSep", true)
	if err != nil {
		return nil, err
	}
	if err := set.done(); err != nil {
		return nil, err
	}
	return &decimalField{options: decimalOptions{Precision: precision, Scale: scale, ThousandSep: separator}}, nil
}

type decimalField struct{ options decimalOptions }

func (field *decimalField) Kind() Kind     { return Decimal }
func (field *decimalField) ReadOnly() bool { return false }
func (field *decimalField) Options() json.RawMessage {
	encoded, _ := json.Marshal(field.options)
	return encoded
}
func (field *decimalField) Column() Column {
	return Column{Type: ddl.DataType{Kind: ddl.Numeric, Precision: field.options.Precision, Scale: field.options.Scale}}
}
func (field *decimalField) Constraints() Constraints {
	return Constraints{Required: true, Unique: true, Default: true}
}
func (field *decimalField) FilterOps() []Op   { return clone(numericOps) }
func (field *decimalField) Aggregates() []Agg { return clone(numericAggs) }

func (field *decimalField) Decode(raw json.RawMessage) (any, error) {
	if isNull(raw) {
		return nil, nil
	}
	trimmed := string(trimJSONSpace(raw))
	if trimmed[0] == '"' {
		text, _, err := decodeString(raw)
		if err != nil {
			return nil, err
		}
		if text == "" {
			return nil, nil
		}
		if !jsonNumber.MatchString(text) {
			return nil, fail(ReasonInvalidDecimal)
		}
		return field.check(text, false)
	}
	if !jsonNumber.MatchString(trimmed) {
		return nil, fail(ReasonInvalidType)
	}
	return field.check(trimmed, true)
}

func (field *decimalField) ParsePaste(text string, _ Env) (any, error) {
	text = normalizeNumberText(text)
	if text == "" {
		return nil, nil
	}
	if !pasteNumber.MatchString(text) {
		return nil, fail(ReasonInvalidDecimal)
	}
	return field.check(text, false)
}

func (field *decimalField) check(text string, fromNumber bool) (any, error) {
	value, ok := parseDecimal(text)
	if !ok {
		return nil, fail(ReasonInvalidDecimal)
	}
	if fromNumber && value.significantDigits() > 15 {
		return nil, fail(ReasonImpreciseNumber)
	}
	if value.fractionDigits() > int64(field.options.Scale) {
		return nil, fail(ReasonTooManyDecimals)
	}
	if value.integerDigits() > int64(field.options.Precision-field.options.Scale) {
		return nil, fail(ReasonOutOfRange)
	}
	return field.numeric(value), nil
}

func (field *decimalField) numeric(value decimalValue) pgtype.Numeric {
	scaled, _ := new(big.Int).SetString(strings.ReplaceAll(strings.TrimPrefix(value.format(field.options.Scale), "-"), ".", ""), 10)
	if value.negative {
		scaled.Neg(scaled)
	}
	return pgtype.Numeric{Int: scaled, Exp: int32(-field.options.Scale), Valid: true}
}

func (field *decimalField) canonical(value any) (string, bool, error) {
	var parsed decimalValue
	switch typed := value.(type) {
	case nil:
		return "", false, nil
	case pgtype.Numeric:
		if !typed.Valid {
			return "", false, nil
		}
		if typed.NaN || typed.InfinityModifier != pgtype.Finite || typed.Int == nil {
			return "", false, fail(ReasonUnsupportedDBType)
		}
		text := typed.Int.String() + "e" + strconv.FormatInt(int64(typed.Exp), 10)
		parsed, _ = parseDecimal(text)
	case string:
		if !pasteNumber.MatchString(typed) {
			return "", false, fail(ReasonUnsupportedDBType)
		}
		parsed, _ = parseDecimal(typed)
	default:
		return "", false, fail(ReasonUnsupportedDBType)
	}
	if parsed.fractionDigits() > int64(field.options.Scale) || parsed.integerDigits() > maxDecimalPrecision {
		return "", false, fail(ReasonUnsupportedDBType)
	}
	return parsed.format(field.options.Scale), true, nil
}

func (field *decimalField) Encode(value any) (json.RawMessage, error) {
	text, ok, err := field.canonical(value)
	if err != nil || !ok {
		return nullJSON, err
	}
	return marshal(text)
}

func (field *decimalField) FormatText(value any, _ Env) (string, error) {
	text, _, err := field.canonical(value)
	return text, err
}

func (field *decimalField) DefaultValue(raw json.RawMessage) (ddl.DefaultValue, error) {
	value, err := field.Decode(raw)
	if err != nil || value == nil {
		return ddl.DefaultValue{}, err
	}
	text, _, err := field.canonical(value)
	return ddl.DefaultValue{Kind: ddl.Literal, Literal: text}, err
}

// ---- float ----

type floatCodec struct{}

func (floatCodec) Kind() Kind { return Float }

type floatOptions struct {
	Size            string `json:"size"`
	DisplayDecimals *int   `json:"displayDecimals,omitempty"`
	ThousandSep     bool   `json:"thousandSep"`
}

func (floatCodec) Bind(raw json.RawMessage) (Field, error) {
	set, err := parseOptions(raw)
	if err != nil {
		return nil, err
	}
	size, err := set.enum("size", "double", "real", "double")
	if err != nil {
		return nil, err
	}
	options := floatOptions{Size: size}
	if raw, ok := set.values["displayDecimals"]; ok && isNull(raw) {
		set.take("displayDecimals")
	} else if decimals, present, err := set.integer("displayDecimals", 0, 10); err != nil {
		return nil, err
	} else if present {
		options.DisplayDecimals = &decimals
	}
	if options.ThousandSep, err = set.boolean("thousandSep", true); err != nil {
		return nil, err
	}
	if err := set.done(); err != nil {
		return nil, err
	}
	return &floatField{options: options}, nil
}

type floatField struct{ options floatOptions }

func (field *floatField) Kind() Kind     { return Float }
func (field *floatField) ReadOnly() bool { return false }
func (field *floatField) Options() json.RawMessage {
	encoded, _ := json.Marshal(field.options)
	return encoded
}
func (field *floatField) real() bool { return field.options.Size == "real" }
func (field *floatField) Column() Column {
	if field.real() {
		return Column{Type: ddl.DataType{Kind: ddl.Real}}
	}
	return Column{Type: ddl.DataType{Kind: ddl.Double}}
}
func (field *floatField) Constraints() Constraints {
	return Constraints{Required: true, Unique: true, Default: true}
}
func (field *floatField) FilterOps() []Op   { return clone(numericOps) }
func (field *floatField) Aggregates() []Agg { return clone(numericAggs) }

func (field *floatField) Decode(raw json.RawMessage) (any, error) {
	if isNull(raw) {
		return nil, nil
	}
	trimmed := string(trimJSONSpace(raw))
	if !jsonNumber.MatchString(trimmed) {
		return nil, fail(ReasonInvalidType)
	}
	return field.parse(trimmed)
}

func (field *floatField) ParsePaste(text string, _ Env) (any, error) {
	text = normalizeNumberText(text)
	if text == "" {
		return nil, nil
	}
	if !pasteNumber.MatchString(text) || len(text) > maxNumberText {
		return nil, fail(ReasonInvalidNumber)
	}
	return field.parse(text)
}

func (field *floatField) parse(text string) (any, error) {
	if len(text) > maxNumberText {
		return nil, fail(ReasonInvalidNumber)
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil && !math.IsInf(value, 0) {
		value, err = 0, nil // underflow rounds to zero like JavaScript Number()
	}
	if err != nil || math.IsInf(value, 0) {
		return nil, fail(ReasonOutOfRange)
	}
	if value == 0 {
		value = 0
	}
	if field.real() {
		single := float32(value)
		if math.IsInf(float64(single), 0) {
			return nil, fail(ReasonOutOfRange)
		}
		if single == 0 {
			single = 0
		}
		return single, nil
	}
	return value, nil
}

func (field *floatField) scan(value any) (float64, int, bool, error) {
	switch typed := value.(type) {
	case nil:
		return 0, 0, false, nil
	case float32:
		return float64(typed), 32, true, nil
	case float64:
		return typed, 64, true, nil
	case pgtype.Float4:
		return float64(typed.Float32), 32, typed.Valid, nil
	case pgtype.Float8:
		return typed.Float64, 64, typed.Valid, nil
	}
	return 0, 0, false, fail(ReasonUnsupportedDBType)
}

func (field *floatField) text(value any) (string, bool, error) {
	number, bits, ok, err := field.scan(value)
	if err != nil || !ok {
		return "", false, err
	}
	if math.IsNaN(number) || math.IsInf(number, 0) {
		return "", false, fail(ReasonUnsupportedDBType)
	}
	if field.real() {
		bits = 32
	}
	return FormatNumber(number, bits), true, nil
}

func (field *floatField) Encode(value any) (json.RawMessage, error) {
	text, ok, err := field.text(value)
	if err != nil || !ok {
		return nullJSON, err
	}
	return json.RawMessage(text), nil
}

func (field *floatField) FormatText(value any, _ Env) (string, error) {
	text, _, err := field.text(value)
	return text, err
}

func (field *floatField) DefaultValue(raw json.RawMessage) (ddl.DefaultValue, error) {
	value, err := field.Decode(raw)
	if err != nil || value == nil {
		return ddl.DefaultValue{}, err
	}
	text, _, err := field.text(value)
	return ddl.DefaultValue{Kind: ddl.Literal, Literal: text}, err
}

// FormatNumber prints the shortest round-tripping decimal for a float of the
// given bit size using JavaScript Number#toString notation (exponent form
// only when the decimal exponent is < -6 or >= 21), so Go and the frontend
// produce identical text.
func FormatNumber(value float64, bits int) string {
	if value == 0 {
		return "0"
	}
	text := strconv.FormatFloat(value, 'e', -1, bits)
	mantissa, exponentText, _ := strings.Cut(text, "e")
	exponent, _ := strconv.Atoi(exponentText)
	sign := ""
	if strings.HasPrefix(mantissa, "-") {
		sign, mantissa = "-", mantissa[1:]
	}
	digits := strings.Replace(mantissa, ".", "", 1)
	if exponent < -6 || exponent >= 21 {
		result := sign + digits[:1]
		if len(digits) > 1 {
			result += "." + digits[1:]
		}
		if exponent >= 0 {
			return result + "e+" + strconv.Itoa(exponent)
		}
		return result + "e" + strconv.Itoa(exponent)
	}
	point := exponent + 1
	switch {
	case point <= 0:
		return sign + "0." + strings.Repeat("0", -point) + digits
	case point >= len(digits):
		return sign + digits + strings.Repeat("0", point-len(digits))
	}
	return sign + digits[:point] + "." + digits[point:]
}
