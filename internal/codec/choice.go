package codec

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/ddl"
)

const (
	maxChoices     = 500
	maxChoiceLabel = 100
)

var (
	choiceIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	// ChoiceColors are the allowed choice color tokens; the first is the default.
	ChoiceColors = []string{"gray", "red", "orange", "amber", "yellow", "lime", "green", "teal", "cyan", "blue", "indigo", "purple", "pink"}
)

type Choice struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Color string `json:"color"`
}

type choiceOptions struct {
	Choices []Choice `json:"choices"`
}

type choiceCodec struct{ multi bool }

func (codec choiceCodec) Kind() Kind {
	if codec.multi {
		return MultiSelect
	}
	return SingleSelect
}

func (codec choiceCodec) Bind(raw json.RawMessage) (Field, error) {
	set, err := parseOptions(raw)
	if err != nil {
		return nil, err
	}
	field := &choiceField{multi: codec.multi, options: choiceOptions{Choices: []Choice{}}, byLabel: map[string]int{}, byFold: map[string]int{}}
	if rawChoices, ok := set.take("choices"); ok && !isNull(rawChoices) {
		if !jsonIs(rawChoices, '[') {
			return nil, failOption("choices")
		}
		var items []json.RawMessage
		if json.Unmarshal(rawChoices, &items) != nil || len(items) > maxChoices {
			return nil, failOption("choices")
		}
		ids := map[string]bool{}
		for index, item := range items {
			choice, err := parseChoice(item)
			if err != nil {
				return nil, err
			}
			fold := strings.ToLower(choice.Label)
			if ids[choice.ID] {
				return nil, failOption("choices")
			}
			if _, duplicate := field.byFold[fold]; duplicate {
				return nil, failOption("choices")
			}
			ids[choice.ID] = true
			field.byFold[fold] = index
			field.byLabel[choice.Label] = index
			field.options.Choices = append(field.options.Choices, choice)
		}
	}
	if err := set.done(); err != nil {
		return nil, err
	}
	return field, nil
}

func parseChoice(raw json.RawMessage) (Choice, error) {
	set, err := parseOptions(raw)
	if err != nil || !jsonIs(raw, '{') {
		return Choice{}, failOption("choices")
	}
	read := func(key string, required bool) (string, error) {
		value, ok := set.take(key)
		if !ok || isNull(value) {
			if required {
				return "", failOption("choices")
			}
			return "", nil
		}
		text, _, err := decodeString(value)
		if err != nil {
			return "", failOption("choices")
		}
		return text, nil
	}
	var choice Choice
	if choice.ID, err = read("id", true); err != nil {
		return Choice{}, err
	}
	if choice.Label, err = read("label", true); err != nil {
		return Choice{}, err
	}
	if choice.Color, err = read("color", false); err != nil {
		return Choice{}, err
	}
	if err := set.done(); err != nil {
		return Choice{}, failOption("choices")
	}
	choice.Label = strings.TrimFunc(choice.Label, isTrimSpace)
	if choice.Color == "" {
		choice.Color = ChoiceColors[0]
	}
	if !choiceIDPattern.MatchString(choice.ID) || !validChoiceLabel(choice.Label) || !validColor(choice.Color) {
		return Choice{}, failOption("choices")
	}
	return choice, nil
}

func validChoiceLabel(label string) bool {
	count := utf8.RuneCountInString(label)
	return count >= 1 && count <= maxChoiceLabel && validString(label, false, false) && !strings.Contains(label, ",")
}

func validColor(color string) bool {
	for _, candidate := range ChoiceColors {
		if candidate == color {
			return true
		}
	}
	return false
}

type choiceField struct {
	multi   bool
	options choiceOptions
	byLabel map[string]int
	byFold  map[string]int
}

func (field *choiceField) Kind() Kind {
	if field.multi {
		return MultiSelect
	}
	return SingleSelect
}
func (field *choiceField) ReadOnly() bool { return false }
func (field *choiceField) Options() json.RawMessage {
	encoded, _ := json.Marshal(field.options)
	return encoded
}

// Choices returns a copy of the configured choices.
func (field *choiceField) Choices() []Choice { return clone(field.options.Choices) }

func (field *choiceField) Column() Column {
	if field.multi {
		return Column{Type: ddl.DataType{Kind: ddl.TextArray}}
	}
	return Column{Type: ddl.DataType{Kind: ddl.Text}}
}
func (field *choiceField) Constraints() Constraints {
	return Constraints{Required: true, Default: true}
}
func (field *choiceField) FilterOps() []Op {
	if field.multi {
		return []Op{OpHasAnyOf, OpHasAllOf, OpHasNoneOf, OpIsExactly, OpIsEmpty, OpIsNotEmpty}
	}
	return []Op{OpIs, OpIsNot, OpIsAnyOf, OpIsNoneOf, OpIsEmpty, OpIsNotEmpty}
}
func (field *choiceField) Aggregates() []Agg {
	if field.multi {
		return []Agg{AggNone, AggCount, AggCountEmpty, AggCountFilled, AggPercentEmpty, AggPercentFilled}
	}
	return clone(basicAggs)
}

// match resolves pasted text exactly, then by unique lower-case match.
func (field *choiceField) match(text string) (int, bool) {
	if index, ok := field.byLabel[text]; ok {
		return index, true
	}
	index, ok := field.byFold[strings.ToLower(text)]
	return index, ok
}

// ordered returns labels for the selected indexes in choice order.
func (field *choiceField) ordered(selected map[int]bool) []string {
	labels := make([]string, 0, len(selected))
	for index, choice := range field.options.Choices {
		if selected[index] {
			labels = append(labels, choice.Label)
		}
	}
	return labels
}

func (field *choiceField) Decode(raw json.RawMessage) (any, error) {
	if isNull(raw) {
		return nil, nil
	}
	if !field.multi {
		text, _, err := decodeString(raw)
		if err != nil || text == "" {
			return nil, err
		}
		if _, ok := field.byLabel[text]; !ok {
			return nil, fail(ReasonUnknownChoice)
		}
		return text, nil
	}
	if !jsonIs(raw, '[') {
		return nil, fail(ReasonInvalidType)
	}
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return nil, fail(ReasonInvalidType)
	}
	if len(items) > maxChoices {
		return nil, fail(ReasonUnknownChoice)
	}
	selected := map[int]bool{}
	for _, item := range items {
		if isNull(item) {
			return nil, fail(ReasonInvalidType)
		}
		text, _, err := decodeString(item)
		if err != nil {
			return nil, err
		}
		index, ok := field.byLabel[text]
		if !ok {
			return nil, fail(ReasonUnknownChoice)
		}
		if selected[index] {
			return nil, fail(ReasonDuplicateChoice)
		}
		selected[index] = true
	}
	if len(selected) == 0 {
		return nil, nil
	}
	return field.ordered(selected), nil
}

func (field *choiceField) ParsePaste(text string, _ Env) (any, error) {
	text = strings.TrimFunc(text, isTrimSpace)
	if text == "" {
		return nil, nil
	}
	if !field.multi {
		index, ok := field.match(text)
		if !ok {
			return nil, fail(ReasonUnknownChoice)
		}
		return field.options.Choices[index].Label, nil
	}
	selected := map[int]bool{}
	for _, part := range strings.Split(text, ",") {
		part = strings.TrimFunc(part, isTrimSpace)
		if part == "" {
			continue
		}
		index, ok := field.match(part)
		if !ok {
			return nil, fail(ReasonUnknownChoice)
		}
		selected[index] = true
	}
	if len(selected) == 0 {
		return nil, nil
	}
	return field.ordered(selected), nil
}

// scanLabels accepts scanned text / text[] values; labels are returned as
// stored even when no longer configured.
func (field *choiceField) scanLabels(value any) ([]string, bool, error) {
	if value == nil {
		return nil, false, nil
	}
	if !field.multi {
		text, ok, err := scanText(value)
		if err != nil || !ok {
			return nil, false, err
		}
		return []string{text}, true, nil
	}
	switch typed := value.(type) {
	case []string:
		return typed, true, nil
	case []any:
		labels := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, false, fail(ReasonUnsupportedDBType)
			}
			labels = append(labels, text)
		}
		return labels, true, nil
	case pgtype.FlatArray[string]:
		return typed, true, nil
	}
	return nil, false, fail(ReasonUnsupportedDBType)
}

func (field *choiceField) Encode(value any) (json.RawMessage, error) {
	labels, ok, err := field.scanLabels(value)
	if err != nil || !ok {
		return nullJSON, err
	}
	if !field.multi {
		return marshal(labels[0])
	}
	if len(labels) == 0 {
		return nullJSON, nil
	}
	return marshal(labels)
}

func (field *choiceField) FormatText(value any, _ Env) (string, error) {
	labels, _, err := field.scanLabels(value)
	if err != nil {
		return "", err
	}
	return strings.Join(labels, ", "), nil
}

func (field *choiceField) DefaultValue(raw json.RawMessage) (ddl.DefaultValue, error) {
	value, err := field.Decode(raw)
	if err != nil || value == nil {
		return ddl.DefaultValue{}, err
	}
	if !field.multi {
		return ddl.DefaultValue{Kind: ddl.Literal, Literal: value.(string)}, nil
	}
	return ddl.DefaultValue{Kind: ddl.Literal, Literal: arrayLiteral(value.([]string))}, nil
}

// arrayLiteral builds a PostgreSQL text[] input literal with every element
// double-quoted and backslash-escaped.
func arrayLiteral(items []string) string {
	var builder strings.Builder
	builder.WriteByte('{')
	for index, item := range items {
		if index > 0 {
			builder.WriteByte(',')
		}
		builder.WriteByte('"')
		builder.WriteString(strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(item))
		builder.WriteByte('"')
	}
	builder.WriteByte('}')
	return builder.String()
}
