package ddl

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

type TypeKind string

const (
	Text        TypeKind = "text"
	Varchar     TypeKind = "varchar"
	SmallInt    TypeKind = "smallint"
	Integer     TypeKind = "integer"
	BigInt      TypeKind = "bigint"
	Numeric     TypeKind = "numeric"
	Real        TypeKind = "real"
	Double      TypeKind = "double precision"
	Boolean     TypeKind = "boolean"
	Date        TypeKind = "date"
	Time        TypeKind = "time"
	TimestampTZ TypeKind = "timestamptz"
	Interval    TypeKind = "interval"
	UUID        TypeKind = "uuid"
	JSONB       TypeKind = "jsonb"
	Inet        TypeKind = "inet"
	CIDR        TypeKind = "cidr"
	TextArray   TypeKind = "text[]"
	BigIntArray TypeKind = "bigint[]"
	UUIDArray   TypeKind = "uuid[]"
)

type DataType struct {
	Kind      TypeKind
	Length    int
	Precision int
	Scale     int
}

func (dataType DataType) SQL() (string, error) {
	if dataType.Kind != Varchar && dataType.Length != 0 ||
		dataType.Kind != Numeric && (dataType.Precision != 0 || dataType.Scale != 0) {
		return "", validation("Unexpected type dimensions")
	}
	switch dataType.Kind {
	case Varchar:
		if dataType.Length < 1 || dataType.Length > 10485760 {
			return "", validation("Invalid varchar length")
		}
		return "pg_catalog.varchar(" + strconv.Itoa(dataType.Length) + ")", nil
	case Numeric:
		if dataType.Precision < 1 || dataType.Precision > 1000 || dataType.Scale < 0 || dataType.Scale > dataType.Precision {
			return "", validation("Invalid numeric precision or scale")
		}
		return fmt.Sprintf("pg_catalog.numeric(%d,%d)", dataType.Precision, dataType.Scale), nil
	case Text, Date, Time, TimestampTZ, Interval, UUID, JSONB, Inet, CIDR:
		return "pg_catalog." + string(dataType.Kind), nil
	case SmallInt:
		return "pg_catalog.int2", nil
	case Integer:
		return "pg_catalog.int4", nil
	case BigInt:
		return "pg_catalog.int8", nil
	case Real:
		return "pg_catalog.float4", nil
	case Double:
		return "pg_catalog.float8", nil
	case Boolean:
		return "pg_catalog.bool", nil
	case TextArray:
		return "pg_catalog.text[]", nil
	case BigIntArray:
		return "pg_catalog.int8[]", nil
	case UUIDArray:
		return "pg_catalog.uuid[]", nil
	default:
		return "", validation("Invalid column type")
	}
}

type DefaultKind string

const (
	NoDefault   DefaultKind = ""
	Literal     DefaultKind = "literal"
	Now         DefaultKind = "now"
	CurrentDate DefaultKind = "current_date"
	UUIDv7      DefaultKind = "uuidv7"
)

// DefaultValue.Literal is a textual PostgreSQL input value, never an expression.
// The server validates its representation and range through an explicit type cast.
type DefaultValue struct {
	Kind    DefaultKind
	Literal string
}

func (value DefaultValue) sql(dataType DataType) (string, error) {
	typeSQL, err := dataType.SQL()
	if err != nil {
		return "", err
	}
	if value.Kind != Literal && value.Literal != "" {
		return "", validation("Unexpected default literal")
	}
	switch value.Kind {
	case NoDefault:
		return "", nil
	case Literal:
		if !utf8.ValidString(value.Literal) || strings.ContainsRune(value.Literal, 0) {
			return "", validation("Invalid default literal")
		}
		if dataType.Kind == Varchar && utf8.RuneCountInString(value.Literal) > dataType.Length {
			return "", validation("Default exceeds varchar length")
		}
		escaped := strings.ReplaceAll(value.Literal, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, "'", "''")
		return "E'" + escaped + "'::" + typeSQL, nil
	case Now:
		if dataType.Kind == TimestampTZ {
			return "pg_catalog.now()", nil
		}
	case CurrentDate:
		if dataType.Kind == Date {
			return "CURRENT_DATE", nil
		}
	case UUIDv7:
		if dataType.Kind == UUID {
			return "pg_catalog.uuidv7()", nil
		}
	}
	return "", validation("Invalid default for column type")
}
