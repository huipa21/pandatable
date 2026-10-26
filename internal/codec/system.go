package codec

import (
	"encoding/hex"
	"encoding/json"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/ddl"
)

type systemCodec struct{ kind Kind }

func (codec systemCodec) Kind() Kind { return codec.kind }

func (codec systemCodec) Bind(raw json.RawMessage) (Field, error) {
	set, err := parseOptions(raw)
	if err != nil {
		return nil, err
	}
	if err := set.done(); err != nil {
		return nil, err
	}
	return systemField{kind: codec.kind}, nil
}

type systemField struct{ kind Kind }

func (field systemField) Kind() Kind               { return field.kind }
func (field systemField) ReadOnly() bool           { return true }
func (field systemField) Options() json.RawMessage { return json.RawMessage(`{}`) }
func (field systemField) Constraints() Constraints { return Constraints{} }

func (field systemField) Column() Column {
	switch field.kind {
	case RowID:
		return Column{Type: ddl.DataType{Kind: ddl.BigInt}, NotNull: true, System: true}
	case CreatedAt, UpdatedAt:
		return Column{Type: ddl.DataType{Kind: ddl.TimestampTZ}, NotNull: true, System: true}
	}
	return Column{Type: ddl.DataType{Kind: ddl.UUID}, System: true}
}

func (field systemField) FilterOps() []Op {
	switch field.kind {
	case RowID:
		return withoutEmpty(numericOps)
	case CreatedAt, UpdatedAt:
		return withoutEmpty(temporalOps)
	}
	return clone(userOps)
}

func (field systemField) Aggregates() []Agg {
	switch field.kind {
	case RowID:
		return []Agg{AggNone, AggCount, AggMin, AggMax}
	case CreatedAt, UpdatedAt:
		return []Agg{AggNone, AggCount, AggEarliest, AggLatest}
	}
	return clone(basicAggs)
}

func (field systemField) Decode(json.RawMessage) (any, error) { return nil, fail(ReasonReadOnly) }
func (field systemField) ParsePaste(string, Env) (any, error) { return nil, fail(ReasonReadOnly) }
func (field systemField) DefaultValue(json.RawMessage) (ddl.DefaultValue, error) {
	return ddl.DefaultValue{}, fail(ReasonReadOnly)
}

func (field systemField) Encode(value any) (json.RawMessage, error) {
	text, ok, err := field.text(value, Env{}, true)
	if err != nil || !ok {
		return nullJSON, err
	}
	return marshal(text)
}

func (field systemField) FormatText(value any, env Env) (string, error) {
	text, _, err := field.text(value, env, false)
	return text, err
}

func (field systemField) text(value any, env Env, wire bool) (string, bool, error) {
	switch field.kind {
	case RowID:
		number, ok, err := scanInteger(value)
		if err != nil || !ok {
			return "", false, err
		}
		return strconv.FormatInt(number, 10), true, nil
	case CreatedAt, UpdatedAt:
		instant, ok, err := scanInstant(value)
		if err != nil || !ok {
			return "", false, err
		}
		if wire {
			return FormatInstant(instant), true, nil
		}
		return FormatLocal(instant, env.Location), true, nil
	}
	return scanUUID(value)
}

func scanUUID(value any) (string, bool, error) {
	var bytes [16]byte
	switch typed := value.(type) {
	case nil:
		return "", false, nil
	case [16]byte:
		bytes = typed
	case pgtype.UUID:
		if !typed.Valid {
			return "", false, nil
		}
		bytes = typed.Bytes
	default:
		return "", false, fail(ReasonUnsupportedDBType)
	}
	text := hex.EncodeToString(bytes[:])
	return text[:8] + "-" + text[8:12] + "-" + text[12:16] + "-" + text[16:20] + "-" + text[20:], true, nil
}
