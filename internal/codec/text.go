package codec

import (
	"encoding/json"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/ddl"
)

const (
	maxTextLength     = 10000
	maxLongTextLength = 100000
	maxEmailBytes     = 254
	maxEmailLocal     = 64
	maxURLLength      = 2048
)

// textCodec implements the string-stored kinds text, long_text, email and url.
type textCodec struct{ kind Kind }

func (codec textCodec) Kind() Kind { return codec.kind }

type textOptions struct {
	MaxLength *int `json:"maxLength,omitempty"`
}

type longTextOptions struct {
	RichText bool `json:"richText"`
}

func (codec textCodec) Bind(raw json.RawMessage) (Field, error) {
	set, err := parseOptions(raw)
	if err != nil {
		return nil, err
	}
	field := &textField{kind: codec.kind}
	switch codec.kind {
	case Text:
		length, present, err := set.integer("maxLength", 1, maxTextLength)
		if err != nil {
			return nil, err
		}
		if present {
			field.maxLength = length
		}
	case LongText:
		rich, err := set.boolean("richText", false)
		if err != nil {
			return nil, err
		}
		if rich {
			return nil, failOption("richText")
		}
	}
	if err := set.done(); err != nil {
		return nil, err
	}
	return field, nil
}

type textField struct {
	kind      Kind
	maxLength int
}

func (field *textField) Kind() Kind     { return field.kind }
func (field *textField) ReadOnly() bool { return false }

func (field *textField) Options() json.RawMessage {
	var options any = struct{}{}
	switch field.kind {
	case Text:
		value := textOptions{}
		if field.maxLength > 0 {
			length := field.maxLength
			value.MaxLength = &length
		}
		options = value
	case LongText:
		options = longTextOptions{}
	}
	encoded, _ := json.Marshal(options)
	return encoded
}

func (field *textField) Column() Column {
	if field.kind == Text && field.maxLength > 0 {
		return Column{Type: ddl.DataType{Kind: ddl.Varchar, Length: field.maxLength}}
	}
	return Column{Type: ddl.DataType{Kind: ddl.Text}}
}

func (field *textField) Constraints() Constraints {
	return Constraints{Required: true, Unique: field.kind != LongText, Default: true}
}

func (field *textField) FilterOps() []Op   { return clone(textOps) }
func (field *textField) Aggregates() []Agg { return clone(basicAggs) }

func (field *textField) Decode(raw json.RawMessage) (any, error) {
	text, ok, err := decodeString(raw)
	if err != nil || !ok {
		return nil, err
	}
	return field.validate(text, false)
}

func (field *textField) ParsePaste(text string, _ Env) (any, error) {
	return field.validate(text, true)
}

// validate returns nil (SQL NULL) for empty input or the normalized string.
func (field *textField) validate(text string, lenient bool) (any, error) {
	switch field.kind {
	case Text:
		if lenient {
			text = lineBreaks.ReplaceAllString(text, " ")
		}
		if text == "" {
			return nil, nil
		}
		if !validString(text, true, false) {
			return nil, fail(ReasonInvalidText)
		}
		limit := maxTextLength
		if field.maxLength > 0 {
			limit = field.maxLength
		}
		if utf8.RuneCountInString(text) > limit {
			return nil, fail(ReasonTooLong)
		}
		return text, nil
	case LongText:
		text = lineBreaks.ReplaceAllString(text, "\n")
		if text == "" {
			return nil, nil
		}
		if !validString(text, true, true) {
			return nil, fail(ReasonInvalidText)
		}
		if utf8.RuneCountInString(text) > maxLongTextLength {
			return nil, fail(ReasonTooLong)
		}
		return text, nil
	case Email:
		text = strings.TrimFunc(text, isTrimSpace)
		if text == "" {
			return nil, nil
		}
		if !ValidEmail(text) {
			return nil, fail(ReasonInvalidEmail)
		}
		return text, nil
	default:
		text = strings.TrimFunc(text, isTrimSpace)
		if text == "" {
			return nil, nil
		}
		if lenient && !strings.Contains(text, "://") && !schemePattern.MatchString(text) {
			text = "https://" + text
		}
		if reason := checkURL(text); reason != "" {
			return nil, fail(reason)
		}
		return text, nil
	}
}

var lineBreaks = regexp.MustCompile(`\r\n|\r|\n`)

func (field *textField) Encode(value any) (json.RawMessage, error) {
	text, ok, err := scanText(value)
	if err != nil || !ok {
		return nullJSON, err
	}
	return marshal(text)
}

func (field *textField) FormatText(value any, _ Env) (string, error) {
	text, _, err := scanText(value)
	return text, err
}

func (field *textField) DefaultValue(raw json.RawMessage) (ddl.DefaultValue, error) {
	value, err := field.Decode(raw)
	if err != nil || value == nil {
		return ddl.DefaultValue{}, err
	}
	text, _ := value.(string)
	return ddl.DefaultValue{Kind: ddl.Literal, Literal: text}, nil
}

func scanText(value any) (string, bool, error) {
	switch typed := value.(type) {
	case nil:
		return "", false, nil
	case string:
		return typed, true, nil
	case *string:
		if typed == nil {
			return "", false, nil
		}
		return *typed, true, nil
	case pgtype.Text:
		return typed.String, typed.Valid, nil
	}
	return "", false, fail(ReasonUnsupportedDBType)
}

var (
	emailLocal  = regexp.MustCompile("^[A-Za-z0-9!#$%&'*+/=?^_`{|}~-]+(\\.[A-Za-z0-9!#$%&'*+/=?^_`{|}~-]+)*$")
	domainLabel = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)
	topLabel    = regexp.MustCompile(`^([A-Za-z]{2,63}|[Xx][Nn]--[A-Za-z0-9-]{1,59})$`)
)

// ValidEmail applies the conservative ASCII address rule of D-119 to an
// already-trimmed value.
func ValidEmail(text string) bool {
	if len(text) > maxEmailBytes || strings.Count(text, "@") != 1 {
		return false
	}
	local, domain, _ := strings.Cut(text, "@")
	if len(local) > maxEmailLocal || !emailLocal.MatchString(local) {
		return false
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if !domainLabel.MatchString(label) {
			return false
		}
	}
	return topLabel.MatchString(labels[len(labels)-1])
}

var (
	schemePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+-]*:`)
	httpPrefix    = regexp.MustCompile(`^[Hh][Tt][Tt][Pp][Ss]?://`)
	ipv6Host      = regexp.MustCompile(`^\[[0-9A-Fa-f:.]+\]$`)
	portPattern   = regexp.MustCompile(`^[0-9]{1,5}$`)
	asciiLabel    = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?$`)
	percentEscape = regexp.MustCompile(`%([^0-9A-Fa-f]|[0-9A-Fa-f][^0-9A-Fa-f]|[0-9A-Fa-f]?$)`)
)

// checkURL validates an already-trimmed absolute http(s) URL (D-119) and
// returns an error reason or "".
func checkURL(text string) string {
	if utf8.RuneCountInString(text) > maxURLLength {
		return ReasonTooLong
	}
	if !httpPrefix.MatchString(text) {
		if schemePattern.MatchString(text) {
			return ReasonURLScheme
		}
		return ReasonInvalidURL
	}
	for _, r := range text {
		if isControl(r) || isTrimSpace(r) || r == '\\' {
			return ReasonInvalidURL
		}
	}
	if percentEscape.MatchString(text) {
		return ReasonInvalidURL
	}
	rest := text[strings.Index(text, "://")+3:]
	authority := rest
	if end := strings.IndexAny(rest, "/?#"); end >= 0 {
		authority = rest[:end]
	}
	if authority == "" || strings.Contains(authority, "@") {
		return ReasonInvalidURL
	}
	host, port, hasPort := authority, "", false
	if strings.HasPrefix(authority, "[") {
		end := strings.Index(authority, "]")
		if end < 0 {
			return ReasonInvalidURL
		}
		host = authority[:end+1]
		if suffix := authority[end+1:]; suffix != "" {
			if !strings.HasPrefix(suffix, ":") {
				return ReasonInvalidURL
			}
			port, hasPort = suffix[1:], true
		}
		if !ipv6Host.MatchString(host) {
			return ReasonInvalidURL
		}
		if address, err := netip.ParseAddr(host[1 : len(host)-1]); err != nil || !address.Is6() {
			return ReasonInvalidURL
		}
	} else {
		if index := strings.LastIndex(authority, ":"); index >= 0 {
			host, port, hasPort = authority[:index], authority[index+1:], true
		}
		if !validHostname(host) {
			return ReasonInvalidURL
		}
	}
	if hasPort {
		if !portPattern.MatchString(port) {
			return ReasonInvalidURL
		}
		if number, err := strconv.Atoi(port); err != nil || number < 1 || number > 65535 {
			return ReasonInvalidURL
		}
	}
	return ""
}

// validHostname accepts dot-separated ASCII LDH labels without edge hyphens;
// Unicode hosts must be entered as punycode (D-119).
func validHostname(host string) bool {
	if host == "" || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) > 63 || !asciiLabel.MatchString(label) {
			return false
		}
	}
	return true
}
