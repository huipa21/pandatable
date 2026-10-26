package codec

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/ddl"
)

var (
	isoDate        = regexp.MustCompile(`^([0-9]{4})-([0-9]{2})-([0-9]{2})$`)
	wireDateTime   = regexp.MustCompile(`^([0-9]{4})-([0-9]{2})-([0-9]{2})[Tt]([0-9]{2}):([0-9]{2}):([0-9]{2})(\.[0-9]+)?([Zz]|[+-][0-9]{2}:[0-9]{2})$`)
	pasteDateParts = regexp.MustCompile(`^([0-9]{1,4})([-/.])([0-9]{1,2})([-/.])([0-9]{1,4})`)
	pasteCJKDate   = regexp.MustCompile(`^([0-9]{4})年([0-9]{1,2})月([0-9]{1,2})日`)
	pasteTime      = regexp.MustCompile(`^(?:[Tt]|\s+)([0-9]{1,2}):([0-9]{2})(?::([0-9]{2})(\.[0-9]+)?)?(?:\s*([AaPp][Mm]))?(?:\s*([Zz]|[+-][0-9]{2}(?::[0-9]{2}(?::[0-9]{2})?|[0-9]{2})))?$`)
	tzNamePattern  = regexp.MustCompile(`^[A-Za-z0-9_+\-]+(/[A-Za-z0-9_+\-]+)*$`)

	minInstant = time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)
	maxInstant = time.Date(9999, 12, 31, 23, 59, 59, 999999000, time.UTC)

	locations sync.Map
)

// LoadLocation loads an IANA zone name after syntax checks shared with
// fixedTz validation; callers still verify PostgreSQL catalog membership.
func LoadLocation(name string) (*time.Location, error) {
	if len(name) > 64 || !tzNamePattern.MatchString(name) || name == "Local" ||
		strings.HasPrefix(name, "posix/") || strings.HasPrefix(name, "right/") {
		return nil, fail(ReasonInvalidOptions)
	}
	if cached, ok := locations.Load(name); ok {
		return cached.(*time.Location), nil
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, fail(ReasonInvalidOptions)
	}
	locations.Store(name, location)
	return location, nil
}

type civilDate struct{ year, month, day int }

func (date civilDate) valid() bool {
	if date.month < 1 || date.month > 12 || date.day < 1 {
		return false
	}
	return date.day <= time.Date(date.year, time.Month(date.month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func (date civilDate) time() time.Time {
	return time.Date(date.year, time.Month(date.month), date.day, 0, 0, 0, 0, time.UTC)
}

func atoi(text string) int {
	value, _ := strconv.Atoi(text)
	return value
}

// checkDate validates a calendar date; reason is invalidReason or out_of_range.
func checkDate(date civilDate, invalidReason string) error {
	if !date.valid() {
		return fail(invalidReason)
	}
	if date.year < 1 || date.year > 9999 {
		return fail(ReasonOutOfRange)
	}
	return nil
}

// parsePasteDate reads a leading date in ISO-like, CJK or field-ordered
// us/eu form and returns the remaining text.
func parsePasteDate(text, format, invalidReason string) (civilDate, string, error) {
	if match := pasteCJKDate.FindStringSubmatch(text); match != nil {
		date := civilDate{atoi(match[1]), atoi(match[2]), atoi(match[3])}
		return date, text[len(match[0]):], checkDate(date, invalidReason)
	}
	match := pasteDateParts.FindStringSubmatch(text)
	if match == nil || match[2] != match[4] {
		return civilDate{}, "", fail(invalidReason)
	}
	var date civilDate
	switch {
	case len(match[1]) == 4:
		date = civilDate{atoi(match[1]), atoi(match[3]), atoi(match[5])}
	case len(match[5]) == 4 && len(match[1]) <= 2 && format == "us":
		date = civilDate{atoi(match[5]), atoi(match[1]), atoi(match[3])}
	case len(match[5]) == 4 && len(match[1]) <= 2 && format == "eu":
		date = civilDate{atoi(match[5]), atoi(match[3]), atoi(match[1])}
	default:
		return civilDate{}, "", fail(invalidReason)
	}
	return date, text[len(match[0]):], checkDate(date, invalidReason)
}

func scanDate(value any) (time.Time, bool, error) {
	switch typed := value.(type) {
	case nil:
		return time.Time{}, false, nil
	case time.Time:
		return typed, true, nil
	case pgtype.Date:
		if !typed.Valid {
			return time.Time{}, false, nil
		}
		if typed.InfinityModifier != pgtype.Finite {
			return time.Time{}, false, fail(ReasonUnsupportedDBType)
		}
		return typed.Time, true, nil
	}
	return time.Time{}, false, fail(ReasonUnsupportedDBType)
}

func formatYear(year int) string { return fmt.Sprintf("%04d", year) }

func formatDate(value time.Time) (string, error) {
	if value.Year() < 1 || value.Year() > 9999 {
		return "", fail(ReasonUnsupportedDBType)
	}
	return formatYear(value.Year()) + value.Format("-01-02"), nil
}

// ---- date ----

type dateCodec struct{}

func (dateCodec) Kind() Kind { return Date }

type dateOptions struct {
	DateFormat string `json:"dateFormat"`
}

var dateFormats = []string{"iso", "slash", "us", "eu"}

func (dateCodec) Bind(raw json.RawMessage) (Field, error) {
	set, err := parseOptions(raw)
	if err != nil {
		return nil, err
	}
	format, err := set.enum("dateFormat", "iso", dateFormats...)
	if err != nil {
		return nil, err
	}
	if err := set.done(); err != nil {
		return nil, err
	}
	return &dateField{options: dateOptions{DateFormat: format}}, nil
}

type dateField struct{ options dateOptions }

func (field *dateField) Kind() Kind     { return Date }
func (field *dateField) ReadOnly() bool { return false }
func (field *dateField) Options() json.RawMessage {
	encoded, _ := json.Marshal(field.options)
	return encoded
}
func (field *dateField) Column() Column { return Column{Type: ddl.DataType{Kind: ddl.Date}} }
func (field *dateField) Constraints() Constraints {
	return Constraints{Required: true, Unique: true, Default: true}
}
func (field *dateField) FilterOps() []Op   { return clone(temporalOps) }
func (field *dateField) Aggregates() []Agg { return clone(temporalAggs) }

func (field *dateField) Decode(raw json.RawMessage) (any, error) {
	text, ok, err := decodeString(raw)
	if err != nil || !ok || text == "" {
		return nil, err
	}
	match := isoDate.FindStringSubmatch(text)
	if match == nil {
		return nil, fail(ReasonInvalidDate)
	}
	date := civilDate{atoi(match[1]), atoi(match[2]), atoi(match[3])}
	if err := checkDate(date, ReasonInvalidDate); err != nil {
		return nil, err
	}
	return pgtype.Date{Time: date.time(), Valid: true}, nil
}

func (field *dateField) ParsePaste(text string, _ Env) (any, error) {
	text = strings.TrimFunc(text, isTrimSpace)
	if text == "" {
		return nil, nil
	}
	date, rest, err := parsePasteDate(text, field.options.DateFormat, ReasonInvalidDate)
	if err != nil {
		return nil, err
	}
	if rest != "" {
		return nil, fail(ReasonInvalidDate)
	}
	return pgtype.Date{Time: date.time(), Valid: true}, nil
}

func (field *dateField) FormatText(value any, _ Env) (string, error) {
	date, ok, err := scanDate(value)
	if err != nil || !ok {
		return "", err
	}
	return formatDate(date)
}

func (field *dateField) Encode(value any) (json.RawMessage, error) {
	text, err := field.FormatText(value, Env{})
	if err != nil || text == "" {
		return nullJSON, err
	}
	return marshal(text)
}

func (field *dateField) DefaultValue(raw json.RawMessage) (ddl.DefaultValue, error) {
	value, err := field.Decode(raw)
	if err != nil || value == nil {
		return ddl.DefaultValue{}, err
	}
	text, err := field.FormatText(value, Env{})
	return ddl.DefaultValue{Kind: ddl.Literal, Literal: text}, err
}

// ---- datetime ----

type dateTimeCodec struct{}

func (dateTimeCodec) Kind() Kind { return DateTime }

type dateTimeOptions struct {
	DateFormat string `json:"dateFormat"`
	TimeFormat string `json:"timeFormat"`
	Timezone   string `json:"timezone"`
	FixedTz    string `json:"fixedTz,omitempty"`
}

func (dateTimeCodec) Bind(raw json.RawMessage) (Field, error) {
	set, err := parseOptions(raw)
	if err != nil {
		return nil, err
	}
	var options dateTimeOptions
	if options.DateFormat, err = set.enum("dateFormat", "iso", dateFormats...); err != nil {
		return nil, err
	}
	if options.TimeFormat, err = set.enum("timeFormat", "24h", "24h", "12h"); err != nil {
		return nil, err
	}
	if options.Timezone, err = set.enum("timezone", "user", "user", "fixed"); err != nil {
		return nil, err
	}
	field := &dateTimeField{}
	rawZone, hasZone := set.take("fixedTz")
	if hasZone && isNull(rawZone) {
		hasZone = false
	}
	switch {
	case options.Timezone == "fixed" && !hasZone, options.Timezone == "user" && hasZone:
		return nil, failOption("fixedTz")
	case hasZone:
		if !jsonIs(rawZone, '"') || json.Unmarshal(rawZone, &options.FixedTz) != nil {
			return nil, failOption("fixedTz")
		}
		if field.location, err = LoadLocation(options.FixedTz); err != nil {
			return nil, failOption("fixedTz")
		}
	}
	if err := set.done(); err != nil {
		return nil, err
	}
	field.options = options
	return field, nil
}

type dateTimeField struct {
	options  dateTimeOptions
	location *time.Location
}

func (field *dateTimeField) Kind() Kind     { return DateTime }
func (field *dateTimeField) ReadOnly() bool { return false }
func (field *dateTimeField) Options() json.RawMessage {
	encoded, _ := json.Marshal(field.options)
	return encoded
}
func (field *dateTimeField) Column() Column { return Column{Type: ddl.DataType{Kind: ddl.TimestampTZ}} }
func (field *dateTimeField) Constraints() Constraints {
	return Constraints{Required: true, Unique: true, Default: true}
}
func (field *dateTimeField) FilterOps() []Op   { return clone(temporalOps) }
func (field *dateTimeField) Aggregates() []Agg { return clone(temporalAggs) }

// zone is the display/paste zone: fixedTz, else the viewer's zone.
func (field *dateTimeField) zone(env Env) *time.Location {
	if field.location != nil {
		return field.location
	}
	return env.Location
}

func (field *dateTimeField) Decode(raw json.RawMessage) (any, error) {
	text, ok, err := decodeString(raw)
	if err != nil || !ok || text == "" {
		return nil, err
	}
	instant, err := parseWireDateTime(text)
	if err != nil {
		return nil, err
	}
	return instant, nil
}

func (field *dateTimeField) ParsePaste(text string, env Env) (any, error) {
	text = strings.TrimFunc(text, isTrimSpace)
	if text == "" {
		return nil, nil
	}
	return parsePasteDateTime(text, field.options.DateFormat, field.zone(env))
}

func (field *dateTimeField) Encode(value any) (json.RawMessage, error) {
	instant, ok, err := scanInstant(value)
	if err != nil || !ok {
		return nullJSON, err
	}
	return marshal(FormatInstant(instant))
}

func (field *dateTimeField) FormatText(value any, env Env) (string, error) {
	instant, ok, err := scanInstant(value)
	if err != nil || !ok {
		return "", err
	}
	return FormatLocal(instant, field.zone(env)), nil
}

func (field *dateTimeField) DefaultValue(raw json.RawMessage) (ddl.DefaultValue, error) {
	value, err := field.Decode(raw)
	if err != nil || value == nil {
		return ddl.DefaultValue{}, err
	}
	return ddl.DefaultValue{Kind: ddl.Literal, Literal: FormatInstant(value.(time.Time))}, nil
}

func checkInstant(instant time.Time) (time.Time, error) {
	if instant.Before(minInstant) || instant.After(maxInstant) {
		return time.Time{}, fail(ReasonOutOfRange)
	}
	return instant.UTC(), nil
}

func parseFraction(text string) (int, bool) {
	if text == "" {
		return 0, true
	}
	digits := text[1:]
	if len(digits) > 6 {
		return 0, false
	}
	return atoi(digits + strings.Repeat("0", 9-len(digits))), true
}

// parseOffset parses Z, ±HH:MM[:SS] or ±HHMM (hours ≤ 23, minutes/seconds ≤ 59).
func parseOffset(text string) (int, bool) {
	if text == "Z" || text == "z" {
		return 0, true
	}
	digits := strings.ReplaceAll(text[1:], ":", "")
	hours, minutes, seconds := atoi(digits[:2]), atoi(digits[2:4]), atoi(digits[4:])
	if hours > 23 || minutes > 59 || seconds > 59 {
		return 0, false
	}
	offset := hours*3600 + minutes*60 + seconds
	if text[0] == '-' {
		offset = -offset
	}
	return offset, true
}

func parseWireDateTime(text string) (time.Time, error) {
	match := wireDateTime.FindStringSubmatch(text)
	if match == nil {
		return time.Time{}, fail(ReasonInvalidDateTime)
	}
	date := civilDate{atoi(match[1]), atoi(match[2]), atoi(match[3])}
	if !date.valid() {
		return time.Time{}, fail(ReasonInvalidDateTime)
	}
	hour, minute, second := atoi(match[4]), atoi(match[5]), atoi(match[6])
	nanos, okFraction := parseFraction(match[7])
	offset, okOffset := parseOffset(match[8])
	if hour > 23 || minute > 59 || second > 59 || !okFraction || !okOffset {
		return time.Time{}, fail(ReasonInvalidDateTime)
	}
	wall := time.Date(date.year, time.Month(date.month), date.day, hour, minute, second, nanos, time.UTC)
	return checkInstant(wall.Add(-time.Duration(offset) * time.Second))
}

func parsePasteDateTime(text, format string, location *time.Location) (any, error) {
	date, rest, err := parsePasteDate(text, format, ReasonInvalidDateTime)
	if err != nil {
		if codecErr, ok := err.(*Error); ok && codecErr.Reason == ReasonOutOfRange {
			return nil, err
		}
		return nil, fail(ReasonInvalidDateTime)
	}
	hour, minute, second, nanos := 0, 0, 0, 0
	offsetText := ""
	if rest != "" {
		match := pasteTime.FindStringSubmatch(rest)
		if match == nil {
			return nil, fail(ReasonInvalidDateTime)
		}
		hour, minute, second = atoi(match[1]), atoi(match[2]), atoi(match[3])
		var ok bool
		if nanos, ok = parseFraction(match[4]); !ok {
			return nil, fail(ReasonInvalidDateTime)
		}
		if meridiem := strings.ToLower(match[5]); meridiem != "" {
			if hour < 1 || hour > 12 {
				return nil, fail(ReasonInvalidDateTime)
			}
			hour %= 12
			if meridiem == "pm" {
				hour += 12
			}
		}
		if hour > 23 || minute > 59 || second > 59 {
			return nil, fail(ReasonInvalidDateTime)
		}
		offsetText = match[6]
	}
	wall := time.Date(date.year, time.Month(date.month), date.day, hour, minute, second, nanos, time.UTC)
	if offsetText != "" {
		offset, ok := parseOffset(offsetText)
		if !ok {
			return nil, fail(ReasonInvalidDateTime)
		}
		return checkInstant(wall.Add(-time.Duration(offset) * time.Second))
	}
	if location == nil {
		return nil, fail(ReasonTimezoneRequired)
	}
	instant, err := ResolveLocal(wall, location)
	if err != nil {
		return nil, err
	}
	return checkInstant(instant)
}

// ResolveLocal maps a wall-clock time (given as UTC fields) in location to an
// instant: nonexistent (gap) times fail, ambiguous (overlap) times resolve to
// the earlier instant. The frontend implements the same algorithm.
func ResolveLocal(wall time.Time, location *time.Location) (time.Time, error) {
	seen := map[int]bool{}
	var best time.Time
	found := false
	for _, hours := range []int{-48, -24, 0, 24, 48} {
		_, offset := wall.Add(time.Duration(hours) * time.Hour).In(location).Zone()
		if seen[offset] {
			continue
		}
		seen[offset] = true
		candidate := wall.Add(-time.Duration(offset) * time.Second)
		local := candidate.In(location)
		if !time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), local.Minute(), local.Second(), local.Nanosecond(), time.UTC).Equal(wall) {
			continue
		}
		if !found || candidate.Before(best) {
			best, found = candidate, true
		}
	}
	if !found {
		return time.Time{}, fail(ReasonNonexistentLocal)
	}
	return best.UTC(), nil
}

func scanInstant(value any) (time.Time, bool, error) {
	switch typed := value.(type) {
	case nil:
		return time.Time{}, false, nil
	case time.Time:
		return typed.UTC(), true, nil
	case pgtype.Timestamptz:
		if !typed.Valid {
			return time.Time{}, false, nil
		}
		if typed.InfinityModifier != pgtype.Finite {
			return time.Time{}, false, fail(ReasonUnsupportedDBType)
		}
		return typed.Time.UTC(), true, nil
	}
	return time.Time{}, false, fail(ReasonUnsupportedDBType)
}

func fractionSuffix(instant time.Time) string {
	micros := instant.Nanosecond() / 1000
	if micros == 0 {
		return ""
	}
	return "." + strings.TrimRight(fmt.Sprintf("%06d", micros), "0")
}

// FormatInstant renders the canonical UTC wire form (D-115).
func FormatInstant(instant time.Time) string {
	instant = instant.UTC()
	return formatYear(instant.Year()) + instant.Format("-01-02T15:04:05") + fractionSuffix(instant) + "Z"
}

// FormatLocal renders copy text in location with an explicit offset (D-117);
// a nil location uses UTC.
func FormatLocal(instant time.Time, location *time.Location) string {
	if location == nil {
		location = time.UTC
	}
	local := instant.In(location)
	if local.Year() < 1 || local.Year() > 9999 {
		local = instant.UTC()
	}
	_, offset := local.Zone()
	sign := "+"
	if offset < 0 {
		sign, offset = "-", -offset
	}
	zone := fmt.Sprintf("%s%02d:%02d", sign, offset/3600, offset/60%60)
	if offset%60 != 0 {
		zone += fmt.Sprintf(":%02d", offset%60)
	}
	return formatYear(local.Year()) + local.Format("-01-02 15:04:05") + fractionSuffix(local) + zone
}
