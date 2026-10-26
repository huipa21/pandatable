package codec

import (
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/ddl"
)

type checkboxCodec struct{}

func (checkboxCodec) Kind() Kind { return Checkbox }

func (checkboxCodec) Bind(raw json.RawMessage) (Field, error) {
	set, err := parseOptions(raw)
	if err != nil {
		return nil, err
	}
	if err := set.done(); err != nil {
		return nil, err
	}
	return checkboxField{}, nil
}

type checkboxField struct{}

var (
	checkboxTrue  = map[string]bool{"true": true, "1": true, "yes": true, "y": true, "on": true, "checked": true, "x": true, "✓": true, "✔": true, "☑": true, "是": true, "对": true}
	checkboxFalse = map[string]bool{"false": true, "0": true, "no": true, "n": true, "off": true, "unchecked": true, "☐": true, "否": true, "错": true}
)

func (checkboxField) Kind() Kind               { return Checkbox }
func (checkboxField) ReadOnly() bool           { return false }
func (checkboxField) Options() json.RawMessage { return json.RawMessage(`{}`) }
func (checkboxField) Column() Column {
	return Column{Type: ddl.DataType{Kind: ddl.Boolean}, NotNull: true, Default: ddl.DefaultValue{Kind: ddl.Literal, Literal: "false"}}
}
func (checkboxField) Constraints() Constraints { return Constraints{Default: true} }
func (checkboxField) FilterOps() []Op          { return []Op{OpIsChecked, OpIsNotChecked} }
func (checkboxField) Aggregates() []Agg        { return []Agg{AggNone, AggCount, AggPercentChecked} }

// Decode maps JSON null to false: the column is NOT NULL DEFAULT false.
func (checkboxField) Decode(raw json.RawMessage) (any, error) {
	switch string(trimJSONSpace(raw)) {
	case "", "null", "false":
		return false, nil
	case "true":
		return true, nil
	}
	return nil, fail(ReasonInvalidType)
}

func (checkboxField) ParsePaste(text string, _ Env) (any, error) {
	text = strings.ToLower(strings.TrimFunc(text, isTrimSpace))
	switch {
	case text == "" || checkboxFalse[text]:
		return false, nil
	case checkboxTrue[text]:
		return true, nil
	}
	return nil, fail(ReasonInvalidBoolean)
}

func scanBool(value any) (bool, error) {
	switch typed := value.(type) {
	case nil:
		return false, nil
	case bool:
		return typed, nil
	case pgtype.Bool:
		return typed.Valid && typed.Bool, nil
	}
	return false, fail(ReasonUnsupportedDBType)
}

func (checkboxField) Encode(value any) (json.RawMessage, error) {
	checked, err := scanBool(value)
	if err != nil {
		return nullJSON, err
	}
	return marshal(checked)
}

func (checkboxField) FormatText(value any, _ Env) (string, error) {
	checked, err := scanBool(value)
	if err != nil {
		return "", err
	}
	if checked {
		return "true", nil
	}
	return "false", nil
}

func (field checkboxField) DefaultValue(raw json.RawMessage) (ddl.DefaultValue, error) {
	value, err := field.Decode(raw)
	if err != nil {
		return ddl.DefaultValue{}, err
	}
	if value.(bool) {
		return ddl.DefaultValue{Kind: ddl.Literal, Literal: "true"}, nil
	}
	return ddl.DefaultValue{Kind: ddl.Literal, Literal: "false"}, nil
}
