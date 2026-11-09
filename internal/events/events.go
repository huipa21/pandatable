// Package events publishes small, transaction-bound realtime notifications.
package events

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	Channel        = "pt_events"
	MaxRowIDs      = 500
	MaxPayloadSize = 7900
)

type Event struct {
	Type          string              `json:"t"`
	TableID       string              `json:"tableId,omitempty"`
	BookID        string              `json:"bookId,omitempty"`
	Description   *string             `json:"description,omitempty"`
	IDs           []string            `json:"ids,omitempty"`
	Fields        []string            `json:"fields,omitempty"`
	Changes       map[string][]string `json:"changes,omitempty"`
	By            string              `json:"by,omitempty"`
	ByName        string              `json:"byName,omitempty"`
	EventID       string              `json:"eventId,omitempty"`
	SchemaVersion int32               `json:"schemaVersion,omitempty"`
	ViewID        string              `json:"viewId,omitempty"`
	Version       int32               `json:"version,omitempty"`
	Recipient     string              `json:"recipient,omitempty"`
	SourceTableID string              `json:"sourceTableId,omitempty"`
	Code          string              `json:"code,omitempty"`
}

type idKey struct{}

var validID = regexp.MustCompile(`^[A-Za-z0-9_-]{16,64}$`)

func ValidID(id string) bool { return validID.MatchString(id) }

// WithID installs the request's preallocated event ID. The caller validates
// any client-provided ID before calling it.
func WithID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, idKey{}, id)
}

func ID(ctx context.Context) string {
	if id, ok := ctx.Value(idKey{}).(string); ok && ValidID(id) {
		return id
	}
	return rand.Text()
}

type executor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func encode(event Event) ([]byte, error) {
	if event.Type == "" {
		return nil, errors.New("event type is required")
	}
	if event.Type == "book.changed" && (event.BookID == "" || event.Description == nil) {
		return nil, errors.New("book change requires a book ID and description")
	}
	if len(event.IDs) > MaxRowIDs {
		event.Type, event.IDs, event.Fields, event.Changes = "row.bulk", nil, nil, nil
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("encode event: %w", err)
	}
	if len(payload) >= MaxPayloadSize && event.TableID != "" && len(event.IDs) > 0 {
		event.Type, event.IDs, event.Fields, event.Changes = "row.bulk", nil, nil, nil
		payload, err = json.Marshal(event)
		if err != nil {
			return nil, fmt.Errorf("encode bulk event: %w", err)
		}
	}
	if len(payload) >= MaxPayloadSize {
		return nil, errors.New("event exceeds the PostgreSQL notification limit")
	}
	return payload, nil
}

// Emit must use the mutation's transaction; PostgreSQL delivers NOTIFY only
// when that transaction commits.
func Emit(ctx context.Context, tx executor, event Event) error {
	if tx == nil {
		return errors.New("event transaction is required")
	}
	if event.EventID == "" {
		event.EventID = ID(ctx)
	}
	payload, err := encode(event)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "SELECT pg_notify($1, $2)", Channel, string(payload)); err != nil {
		return fmt.Errorf("publish realtime event: %w", err)
	}
	return nil
}
