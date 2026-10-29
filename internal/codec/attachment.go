package codec

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"regexp"
	"strings"
	"unicode/utf8"

	"pandatable/internal/ddl"
)

const (
	maxAttachmentFiles     = 20
	defaultAttachmentFiles = 10
	maxAttachmentAccept    = 20
)

var attachmentMediaType = regexp.MustCompile(`^[a-z0-9!#$&^_.+-]+/(?:[a-z0-9!#$&^_.+-]+|\*)$`)

// AttachmentOptions are the strict, normalized options for an attachment field.
type AttachmentOptions struct {
	Accept   string `json:"accept,omitempty"`
	MaxFiles int    `json:"maxFiles"`
}

// AttachmentValue is the canonical wire value stored in an attachment cell.
type AttachmentValue struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	MIME  string  `json:"mime"`
	Size  int64   `json:"size"`
	Thumb *string `json:"thumb"`
}

// ThumbnailURL returns the authenticated endpoint for a stored thumbnail.
func ThumbnailURL(id string) string { return "/api/v1/attachments/" + id + "/thumbnail" }

type attachmentCodec struct{}

func (attachmentCodec) Kind() Kind { return Attachment }
func (attachmentCodec) Bind(raw json.RawMessage) (Field, error) {
	set, err := parseOptions(raw)
	if err != nil {
		return nil, err
	}
	accept := ""
	if value, present := set.take("accept"); present {
		if !jsonIs(value, '"') || json.Unmarshal(value, &accept) != nil {
			return nil, failOption("accept")
		}
		accept, err = normalizeAttachmentAccept(accept)
		if err != nil {
			return nil, err
		}
	}
	maxFiles, present, err := set.integer("maxFiles", 1, maxAttachmentFiles)
	if err != nil {
		return nil, err
	}
	if !present {
		maxFiles = defaultAttachmentFiles
	}
	if err := set.done(); err != nil {
		return nil, err
	}
	options, _ := json.Marshal(AttachmentOptions{Accept: accept, MaxFiles: maxFiles})
	return attachmentField{options: options}, nil
}

func normalizeAttachmentAccept(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	parts := strings.Split(value, ",")
	if len(parts) > maxAttachmentAccept {
		return "", failOption("accept")
	}
	seen := make(map[string]bool, len(parts))
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.ToLower(strings.TrimSpace(part))
		if !attachmentMediaType.MatchString(item) || item == "*/*" || seen[item] {
			return "", failOption("accept")
		}
		if !strings.HasSuffix(item, "/*") {
			parsed, _, err := mime.ParseMediaType(item)
			if err != nil || parsed != item {
				return "", failOption("accept")
			}
		}
		seen[item] = true
		normalized = append(normalized, item)
	}
	return strings.Join(normalized, ", "), nil
}

type attachmentField struct{ options json.RawMessage }

func (field attachmentField) Kind() Kind { return Attachment }
func (field attachmentField) Options() json.RawMessage {
	return append(json.RawMessage(nil), field.options...)
}
func (attachmentField) ReadOnly() bool { return false }
func (attachmentField) Column() Column {
	return Column{Type: ddl.DataType{Kind: ddl.JSONB}}
}
func (attachmentField) Constraints() Constraints { return Constraints{} }
func (attachmentField) FilterOps() []Op          { return []Op{} }
func (attachmentField) Aggregates() []Agg        { return []Agg{} }
func (field attachmentField) Decode(raw json.RawMessage) (any, error) {
	return field.decode(raw, true)
}

func (field attachmentField) decode(raw json.RawMessage, enforceMaxFiles bool) (any, error) {
	if string(trimJSONSpace(raw)) == "null" {
		return nil, nil
	}
	canonicalRaw, err := canonicalJSON(raw)
	if err != nil {
		return nil, fail(ReasonInvalidArray)
	}
	raw = canonicalRaw
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var values []AttachmentValue
	if err := decoder.Decode(&values); err != nil || values == nil {
		return nil, fail(ReasonInvalidArray)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, fail(ReasonInvalidArray)
	}
	var shapes []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &shapes); err != nil || len(shapes) != len(values) {
		return nil, fail(ReasonInvalidArray)
	}
	for _, shape := range shapes {
		if len(shape) != 5 {
			return nil, fail(ReasonInvalidArray)
		}
		for _, key := range []string{"id", "name", "mime", "size", "thumb"} {
			if _, present := shape[key]; !present {
				return nil, fail(ReasonInvalidArray)
			}
		}
	}
	var options AttachmentOptions
	if err := json.Unmarshal(field.options, &options); err != nil {
		return nil, fail(ReasonInvalidOptions)
	}
	if enforceMaxFiles && len(values) > options.MaxFiles {
		return nil, fail(ReasonOutOfRange)
	}
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if !wireUUIDPattern.MatchString(value.ID) || seen[value.ID] ||
			value.Name == "" || !validString(value.Name, false, false) || utf8.RuneCountInString(value.Name) > 255 ||
			strings.ContainsAny(value.Name, `/\\`) || value.Size < 0 {
			return nil, fail(ReasonInvalidArray)
		}
		mediaType, _, err := mime.ParseMediaType(value.MIME)
		if err != nil || mediaType != value.MIME || strings.ToLower(value.MIME) != value.MIME {
			return nil, fail(ReasonInvalidArray)
		}
		if value.Thumb != nil && (*value.Thumb != ThumbnailURL(value.ID) || !thumbnailMIME(value.MIME)) {
			return nil, fail(ReasonInvalidArray)
		}
		seen[value.ID] = true
	}
	canonical, err := json.Marshal(values)
	if err != nil {
		return nil, fail(ReasonInvalidArray)
	}
	return json.RawMessage(canonical), nil
}

func thumbnailMIME(value string) bool {
	switch value {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

func (field attachmentField) Encode(value any) (json.RawMessage, error) {
	if value == nil {
		return json.RawMessage("null"), nil
	}
	var raw json.RawMessage
	switch typed := value.(type) {
	case []byte:
		raw = typed
	case string:
		raw = json.RawMessage(typed)
	case json.RawMessage:
		raw = typed
	case []AttachmentValue:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return nil, err
		}
		raw = encoded
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return nil, fail(ReasonUnsupportedDBType)
		}
		raw = encoded
	}
	decoded, err := field.decode(raw, false)
	if err != nil {
		return nil, err
	}
	return json.Marshal(decoded)
}

func (field attachmentField) ParsePaste(string, Env) (any, error) {
	return nil, fail(ReasonInvalidType)
}
func (field attachmentField) FormatText(value any, _ Env) (string, error) {
	if value == nil {
		return "", nil
	}
	encoded, err := field.Encode(value)
	if err != nil {
		return "", err
	}
	var values []AttachmentValue
	if err := json.Unmarshal(encoded, &values); err != nil {
		return "", err
	}
	names := make([]string, len(values))
	for index := range values {
		names[index] = values[index].Name
	}
	return strings.Join(names, ", "), nil
}
func (field attachmentField) DefaultValue(raw json.RawMessage) (ddl.DefaultValue, error) {
	if len(raw) == 0 || string(trimJSONSpace(raw)) == "null" {
		return ddl.DefaultValue{}, nil
	}
	return ddl.DefaultValue{}, fail(ReasonInvalidOptions)
}
