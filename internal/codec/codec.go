// Package codec validates, normalizes and converts field values between the
// API JSON wire format, typed PostgreSQL parameters and clipboard text
// (D-113..D-120). It never builds SQL; native storage is described only by
// typed internal/ddl descriptors.
package codec

import (
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"time"
	"unicode/utf8"

	"pandatable/internal/apperr"
	"pandatable/internal/ddl"
)

type Kind string

const (
	Text         Kind = "text"
	LongText     Kind = "long_text"
	Integer      Kind = "integer"
	Decimal      Kind = "decimal"
	Float        Kind = "float"
	Currency     Kind = "currency"
	Percent      Kind = "percent"
	Rating       Kind = "rating"
	Checkbox     Kind = "checkbox"
	Date         Kind = "date"
	Time         Kind = "time"
	DateTime     Kind = "datetime"
	Duration     Kind = "duration"
	SingleSelect Kind = "single_select"
	MultiSelect  Kind = "multi_select"
	Email        Kind = "email"
	URL          Kind = "url"
	Phone        Kind = "phone"
	UUID         Kind = "uuid"
	JSON         Kind = "json"
	Inet         Kind = "inet"
	CIDR         Kind = "cidr"
	TextArray    Kind = "text_array"
	IntArray     Kind = "int_array"
	User         Kind = "user"
	MultiUser    Kind = "multi_user"
	AutoNumber   Kind = "autonumber"
	Attachment   Kind = "attachment"
	RowID        Kind = "id"
	CreatedAt    Kind = "created_at"
	CreatedBy    Kind = "created_by"
	UpdatedAt    Kind = "updated_at"
	UpdatedBy    Kind = "updated_by"
)

// Op is a filter operator identifier (D-120). Codecs only declare
// capabilities; compiling filters belongs to the query engine.
type Op string

const (
	OpIs           Op = "is"
	OpIsNot        Op = "is_not"
	OpContains     Op = "contains"
	OpNotContains  Op = "not_contains"
	OpStartsWith   Op = "starts_with"
	OpEndsWith     Op = "ends_with"
	OpIsEmpty      Op = "is_empty"
	OpIsNotEmpty   Op = "is_not_empty"
	OpEq           Op = "eq"
	OpNeq          Op = "neq"
	OpLt           Op = "lt"
	OpLte          Op = "lte"
	OpGt           Op = "gt"
	OpGte          Op = "gte"
	OpBetween      Op = "between"
	OpBefore       Op = "before"
	OpAfter        Op = "after"
	OpOnOrBefore   Op = "on_or_before"
	OpOnOrAfter    Op = "on_or_after"
	OpIsWithin     Op = "is_within"
	OpIsChecked    Op = "is_checked"
	OpIsNotChecked Op = "is_not_checked"
	OpIsAnyOf      Op = "is_any_of"
	OpIsNoneOf     Op = "is_none_of"
	OpHasAnyOf     Op = "has_any_of"
	OpHasAllOf     Op = "has_all_of"
	OpHasNoneOf    Op = "has_none_of"
	OpIsExactly    Op = "is_exactly"
	OpHasKey       Op = "has_key"
	OpPathEquals   Op = "path_equals"
	OpContainedIn  Op = "contained_in"
)

// Agg is a footer/rollup aggregate identifier (doc 08 §6).
type Agg string

const (
	AggNone           Agg = "none"
	AggCount          Agg = "count"
	AggCountEmpty     Agg = "count_empty"
	AggCountFilled    Agg = "count_filled"
	AggUnique         Agg = "unique"
	AggSum            Agg = "sum"
	AggAvg            Agg = "avg"
	AggMin            Agg = "min"
	AggMax            Agg = "max"
	AggEarliest       Agg = "earliest"
	AggLatest         Agg = "latest"
	AggPercentChecked Agg = "percent_checked"
)

// Env carries viewer context for text conversions. Location is the viewing
// user's effective timezone; it is required for datetime kinds unless the
// field has a fixed timezone. Callers load it after validating the name
// against PostgreSQL's catalog.
type Env struct {
	Location *time.Location
}

// Column is the native storage descriptor of a bound field.
type Column struct {
	Type ddl.DataType
	// NotNull and Default are forced by the kind (checkbox), independent of the
	// user's required/default settings.
	NotNull bool
	Default ddl.DefaultValue
	// System columns are installed by ddl.CreateTable and never added.
	System        bool
	HasRangeCheck bool
	CheckMin      int64
	CheckMax      int64
	UserReference bool
	AutoNumber    bool
}

// Constraints reports which user constraints the kind supports (doc 07 §4).
type Constraints struct {
	Required bool
	Unique   bool
	Default  bool
}

type Codec interface {
	Kind() Kind
	Bind(options json.RawMessage) (Field, error)
}

// Field is a codec bound to validated, normalized options.
type Field interface {
	Kind() Kind
	Options() json.RawMessage
	ReadOnly() bool
	Column() Column
	Constraints() Constraints
	// Decode validates an API JSON value and returns a typed pgx parameter;
	// nil means SQL NULL.
	Decode(raw json.RawMessage) (any, error)
	// Encode converts a value scanned by pgx (typed or rows.Values()) to API JSON.
	Encode(value any) (json.RawMessage, error)
	// ParsePaste leniently parses clipboard or typed text to a typed parameter.
	ParsePaste(text string, env Env) (any, error)
	// FormatText renders a scanned DB value for copy/CSV export.
	FormatText(value any, env Env) (string, error)
	// DefaultValue converts a wire value into a typed DDL literal default;
	// JSON null means no default.
	DefaultValue(raw json.RawMessage) (ddl.DefaultValue, error)
	FilterOps() []Op
	Aggregates() []Agg
}

// Error reasons shared with the frontend registry (web/src/fields).
const (
	ReasonUnknownKind       = "unknown_kind"
	ReasonInvalidOptions    = "invalid_options"
	ReasonInvalidType       = "invalid_type"
	ReasonInvalidText       = "invalid_text"
	ReasonTooLong           = "too_long"
	ReasonInvalidInteger    = "invalid_integer"
	ReasonInvalidDecimal    = "invalid_decimal"
	ReasonInvalidNumber     = "invalid_number"
	ReasonImpreciseNumber   = "imprecise_number"
	ReasonOutOfRange        = "out_of_range"
	ReasonTooManyDecimals   = "too_many_decimals"
	ReasonInvalidBoolean    = "invalid_boolean"
	ReasonInvalidDate       = "invalid_date"
	ReasonInvalidDateTime   = "invalid_datetime"
	ReasonNonexistentLocal  = "nonexistent_local_time"
	ReasonTimezoneRequired  = "timezone_required"
	ReasonUnknownChoice     = "unknown_choice"
	ReasonDuplicateChoice   = "duplicate_choice"
	ReasonInvalidEmail      = "invalid_email"
	ReasonInvalidURL        = "invalid_url"
	ReasonURLScheme         = "url_scheme"
	ReasonInvalidTime       = "invalid_time"
	ReasonInvalidDuration   = "invalid_duration"
	ReasonInvalidPhone      = "invalid_phone"
	ReasonInvalidUUID       = "invalid_uuid"
	ReasonInvalidJSON       = "invalid_json"
	ReasonInvalidIP         = "invalid_ip"
	ReasonInvalidCIDR       = "invalid_cidr"
	ReasonInvalidArray      = "invalid_array"
	ReasonInvalidRating     = "invalid_rating"
	ReasonReadOnly          = "read_only"
	ReasonUnsupportedDBType = "unsupported_db_value"
)

// Error is a codec validation failure. Detail names the offending option or
// is empty; it never contains the rejected value.
type Error struct {
	Reason string
	Detail string
}

func (err *Error) Error() string {
	if err.Detail != "" {
		return "codec: " + err.Reason + " (" + err.Detail + ")"
	}
	return "codec: " + err.Reason
}

func fail(reason string) error { return &Error{Reason: reason} }

func failOption(detail string) error { return &Error{Reason: ReasonInvalidOptions, Detail: detail} }

// APIError maps a codec validation error to the API error boundary (400
// ERR_VALIDATION with details.reason). Unsupported scanned DB values are
// server faults and, like other errors, are returned unchanged.
func APIError(err error) error {
	var codecErr *Error
	if !errors.As(err, &codecErr) || codecErr.Reason == ReasonUnsupportedDBType {
		return err
	}
	apiErr := apperr.New(400, "ERR_VALIDATION", "Field value or options are invalid")
	apiErr.Details = map[string]any{"reason": codecErr.Reason}
	if codecErr.Detail != "" {
		apiErr.Details["option"] = codecErr.Detail
	}
	return apiErr
}

var registry = map[Kind]Codec{}

func init() {
	for _, codec := range []Codec{
		textCodec{kind: Text}, textCodec{kind: LongText}, textCodec{kind: Email}, textCodec{kind: URL},
		integerCodec{}, decimalCodec{}, floatCodec{}, checkboxCodec{},
		currencyCodec{}, percentCodec{}, ratingCodec{},
		dateCodec{}, timeCodec{}, dateTimeCodec{}, durationCodec{},
		phoneCodec{}, uuidCodec{}, jsonCodec{}, inetCodec{}, cidrCodec{},
		arrayCodec{kind: TextArray}, arrayCodec{kind: IntArray},
		userCodec{multi: false}, userCodec{multi: true}, autoNumberCodec{},
		attachmentCodec{},
		choiceCodec{multi: false}, choiceCodec{multi: true},
		systemCodec{kind: RowID}, systemCodec{kind: CreatedAt}, systemCodec{kind: CreatedBy},
		systemCodec{kind: UpdatedAt}, systemCodec{kind: UpdatedBy},
	} {
		registry[codec.Kind()] = codec
	}
}

// Lookup returns the codec for a ui_type.
func Lookup(kind Kind) (Codec, bool) {
	codec, ok := registry[kind]
	return codec, ok
}

// Bind looks up a kind and binds its options.
func Bind(kind Kind, options json.RawMessage) (Field, error) {
	codec, ok := Lookup(kind)
	if !ok {
		return nil, fail(ReasonUnknownKind)
	}
	return codec.Bind(options)
}

// Kinds lists all registered kinds in a stable order.
func Kinds() []Kind {
	kinds := make([]Kind, 0, len(registry))
	for kind := range registry {
		kinds = append(kinds, kind)
	}
	sort.Slice(kinds, func(i, j int) bool { return kinds[i] < kinds[j] })
	return kinds
}

// IsSystem reports whether kind is one of the five read-only system kinds.
func IsSystem(kind Kind) bool {
	switch kind {
	case RowID, CreatedAt, CreatedBy, UpdatedAt, UpdatedBy:
		return true
	}
	return false
}

var (
	textOps     = []Op{OpIs, OpIsNot, OpContains, OpNotContains, OpStartsWith, OpEndsWith, OpIsEmpty, OpIsNotEmpty}
	numericOps  = []Op{OpEq, OpNeq, OpLt, OpLte, OpGt, OpGte, OpBetween, OpIsEmpty, OpIsNotEmpty}
	temporalOps = []Op{OpIs, OpIsNot, OpBefore, OpAfter, OpOnOrBefore, OpOnOrAfter, OpBetween, OpIsWithin, OpIsEmpty, OpIsNotEmpty}
	timeOps     = []Op{OpIs, OpIsNot, OpBefore, OpAfter, OpOnOrBefore, OpOnOrAfter, OpBetween, OpIsEmpty, OpIsNotEmpty}
	userOps     = []Op{OpIs, OpIsNot, OpHasAnyOf, OpIsEmpty, OpIsNotEmpty}

	basicAggs    = []Agg{AggNone, AggCount, AggCountEmpty, AggCountFilled, AggUnique}
	numericAggs  = []Agg{AggNone, AggCount, AggCountEmpty, AggCountFilled, AggUnique, AggSum, AggAvg, AggMin, AggMax}
	temporalAggs = []Agg{AggNone, AggCount, AggCountEmpty, AggCountFilled, AggUnique, AggEarliest, AggLatest}
)

func withoutEmpty(ops []Op) []Op {
	result := make([]Op, 0, len(ops))
	for _, op := range ops {
		if op != OpIsEmpty && op != OpIsNotEmpty {
			result = append(result, op)
		}
	}
	return result
}

func clone[T any](items []T) []T { return append([]T(nil), items...) }

func trimJSONSpace(raw []byte) []byte {
	start, end := 0, len(raw)
	for start < end && isJSONSpace(raw[start]) {
		start++
	}
	for end > start && isJSONSpace(raw[end-1]) {
		end--
	}
	return raw[start:end]
}

func isJSONSpace(b byte) bool { return b == ' ' || b == '\t' || b == '\n' || b == '\r' }

func isNull(raw json.RawMessage) bool {
	trimmed := trimJSONSpace(raw)
	return len(trimmed) == 0 || string(trimmed) == "null"
}

var nullJSON = json.RawMessage("null")

func marshal(value any) (json.RawMessage, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

// decodeString decodes a JSON string; ok is false for JSON null.
func decodeString(raw json.RawMessage) (string, bool, error) {
	if isNull(raw) {
		return "", false, nil
	}
	trimmed := trimJSONSpace(raw)
	if trimmed[0] != '"' || !utf8.Valid(trimmed) || hasLoneSurrogateEscape(trimmed) {
		return "", false, fail(ReasonInvalidType)
	}
	var text string
	if err := json.Unmarshal(trimmed, &text); err != nil {
		return "", false, fail(ReasonInvalidType)
	}
	return text, true, nil
}

// hasLoneSurrogateEscape detects \uD800..\uDFFF escapes that are not a valid
// high/low pair; encoding/json would silently replace them with U+FFFD.
func hasLoneSurrogateEscape(raw []byte) bool {
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\\' || i+1 >= len(raw) {
			continue
		}
		if raw[i+1] != 'u' {
			i++
			continue
		}
		code, ok := hexEscape(raw, i)
		if !ok {
			return false
		}
		switch {
		case code >= 0xDC00 && code <= 0xDFFF:
			return true
		case code >= 0xD800 && code <= 0xDBFF:
			low, ok := hexEscape(raw, i+6)
			if !ok || low < 0xDC00 || low > 0xDFFF {
				return true
			}
			i += 11
		default:
			i += 5
		}
	}
	return false
}

func hexEscape(raw []byte, at int) (uint64, bool) {
	if at+6 > len(raw) || raw[at] != '\\' || raw[at+1] != 'u' {
		return 0, false
	}
	code, err := strconv.ParseUint(string(raw[at+2:at+6]), 16, 16)
	return code, err == nil
}
