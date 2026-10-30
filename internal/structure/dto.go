package structure

import (
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/codec"
)

// DTOs deliberately omit physical PostgreSQL names (b_/t_/c_); clients address
// structure by UUID only (D-002).

type Workspace struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Icon      *string   `json:"icon"`
	Color     *string   `json:"color"`
	CreatedAt time.Time `json:"createdAt"`
}

type Book struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspaceId"`
	Name        string    `json:"name"`
	Position    float64   `json:"position"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Table struct {
	ID             string    `json:"id"`
	BookID         string    `json:"bookId"`
	Name           string    `json:"name"`
	DisplayFieldID *string   `json:"displayFieldId"`
	Position       float64   `json:"position"`
	SchemaVersion  int32     `json:"schemaVersion"`
	CreatedAt      time.Time `json:"createdAt"`
}

type Field struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	UIType      string          `json:"uiType"`
	Options     json.RawMessage `json:"options"`
	Filterable  bool            `json:"filterable"`
	FilterOps   []codec.Op      `json:"filterOps"`
	IsSystem    bool            `json:"isSystem"`
	IsRequired  bool            `json:"isRequired"`
	IsUnique    bool            `json:"isUnique"`
	// Default is the canonical wire default (D-127) or null.
	Default  json.RawMessage `json:"default"`
	Position float64         `json:"position"`
	Version  int32           `json:"version"`
}

type View struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Type     string      `json:"type"`
	Scope    string      `json:"scope"`
	Locked   bool        `json:"locked"`
	Position float64     `json:"position"`
	Version  int32       `json:"version"`
	Config   ViewColumns `json:"config"`
}

type TableSchema struct {
	Table         Table      `json:"table"`
	Fields        []Field    `json:"fields"`
	Relations     []Relation `json:"relations"`
	Views         []View     `json:"views"`
	SchemaVersion int32      `json:"schemaVersion"`
	MyLevel       string     `json:"myLevel"`
}

func timeOf(value pgtype.Timestamptz) time.Time { return value.Time.UTC() }

func textPointer(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func uuidPointer(value pgtype.UUID) *string {
	if !value.Valid {
		return nil
	}
	text := value.String()
	return &text
}

func textValue(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func tableDTO(id, book, display pgtype.UUID, name string, position float64, version int32, created pgtype.Timestamptz) Table {
	return Table{ID: id.String(), BookID: book.String(), Name: name, DisplayFieldID: uuidPointer(display),
		Position: position, SchemaVersion: version, CreatedAt: timeOf(created)}
}

func bookDTO(id, workspace pgtype.UUID, name string, position float64, created pgtype.Timestamptz) Book {
	return Book{ID: id.String(), WorkspaceID: workspace.String(), Name: name, Position: position, CreatedAt: timeOf(created)}
}
