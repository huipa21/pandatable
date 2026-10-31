package structure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl"
	"pandatable/internal/structure/structuresql"
)

const (
	historyPageSize          = 50
	historyRetentionInterval = 24 * time.Hour
)

type HistoryChange struct {
	Old json.RawMessage `json:"o,omitempty"`
	New json.RawMessage `json:"n,omitempty"`
}

type RowHistory struct {
	ID         string                   `json:"id"`
	AuthorID   *string                  `json:"authorId"`
	AuthorName string                   `json:"authorName"`
	Action     string                   `json:"action"`
	Changes    map[string]HistoryChange `json:"changes"`
	CreatedAt  time.Time                `json:"createdAt"`
}

type HistoryListing struct {
	Items   []RowHistory `json:"items"`
	HasMore bool         `json:"hasMore"`
}

type historyWrite struct {
	TableID       string                     `json:"table_id"`
	RowID         int64                      `json:"row_id"`
	UserID        string                     `json:"user_id"`
	Action        string                     `json:"action"`
	Changes       map[string]HistoryChange   `json:"changes"`
	RestoreSystem map[string]json.RawMessage `json:"restore_system,omitempty"`
}

func insertHistory(ctx context.Context, tx pgx.Tx, entries []historyWrite) error {
	if len(entries) == 0 {
		return nil
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("encode row history: %w", err)
	}
	if err := structuresql.New(tx).InsertRowHistoryBatch(ctx, data); err != nil {
		return fmt.Errorf("insert row history: %w", err)
	}
	return nil
}

func snapshotHistory(table rowTable, record RowRecord, actor Actor, action string) (historyWrite, error) {
	id, err := canonicalRowID(record.ID)
	if err != nil {
		return historyWrite{}, err
	}
	changes := make(map[string]HistoryChange)
	var system map[string]json.RawMessage
	if action == "delete" {
		system = make(map[string]json.RawMessage)
	}
	for _, field := range table.fields {
		if field.system {
			if action == "delete" {
				value, ok := record.Fields[field.id]
				if !ok {
					return historyWrite{}, errors.New("row history snapshot is missing a system field")
				}
				system[field.id] = value
			}
			continue
		}
		if field.formula != nil || isComputedKind(string(field.kind)) {
			continue
		}
		value, ok := record.Fields[field.id]
		if !ok {
			return historyWrite{}, errors.New("row history snapshot is missing a stored field")
		}
		if action == "insert" || action == "restore" {
			changes[field.id] = HistoryChange{New: value}
		} else {
			changes[field.id] = HistoryChange{Old: value}
		}
	}
	return historyWrite{TableID: table.id.String(), RowID: id, UserID: actor.ID, Action: action,
		Changes: changes, RestoreSystem: system}, nil
}

func snapshotHistories(table rowTable, records []RowRecord, actor Actor, action string) ([]historyWrite, error) {
	entries := make([]historyWrite, 0, len(records))
	for _, record := range records {
		entry, err := snapshotHistory(table, record, actor, action)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func patchReturning(table rowTable, fields []rowField) string {
	columns := append(table.selectColumnListAlias(table.fields, "history_new"),
		table.selectColumnListAlias(fields, "history_old")...)
	for _, field := range fields {
		old := pgx.Identifier{"history_old", field.column}.Sanitize()
		newValue := pgx.Identifier{"history_new", field.column}.Sanitize()
		columns = append(columns, old+" IS DISTINCT FROM "+newValue)
	}
	return "RETURNING WITH (OLD AS history_old, NEW AS history_new) " + strings.Join(columns, ", ")
}

func scanPatchedRow(row pgx.Row, table rowTable, fields []rowField) (RowRecord, map[string]HistoryChange, error) {
	newCount := len(physicalRowFields(table.fields)) + 1
	oldCount := len(fields) + 1
	values := make([]any, newCount+oldCount)
	changed := make([]bool, len(fields))
	destinations := make([]any, 0, len(values)+len(changed))
	for i := range values {
		destinations = append(destinations, &values[i])
	}
	for i := range changed {
		destinations = append(destinations, &changed[i])
	}
	if err := row.Scan(destinations...); err != nil {
		return RowRecord{}, nil, err
	}
	record, err := table.encode(values[:newCount], table.fields)
	if err != nil {
		return RowRecord{}, nil, err
	}
	old, err := table.encode(values[newCount:], fields)
	if err != nil {
		return RowRecord{}, nil, err
	}
	changes := make(map[string]HistoryChange)
	for i, field := range fields {
		if changed[i] {
			changes[field.id] = HistoryChange{Old: old.Fields[field.id], New: record.Fields[field.id]}
		}
	}
	return record, changes, nil
}

func (s *Service) ListRowHistory(ctx context.Context, actor Actor, tableID, rowID string, offset int) (HistoryListing, error) {
	id, err := parsePathRowID(rowID)
	if err != nil {
		return HistoryListing{}, err
	}
	if offset < 0 || offset > maxQueryOffset {
		return HistoryListing{}, validation("History offset is outside the supported bounds")
	}
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return HistoryListing{}, acl.ErrValidation()
	}
	result := HistoryListing{Items: []RowHistory{}}
	err = s.withReadableTable(ctx, actor, tableID, func(tx pgx.Tx, table rowTable) error {
		if err := s.authorizeRows(ctx, tx, user, acl.Resource{Type: acl.Table, ID: table.id}, acl.Read); err != nil {
			return err
		}
		rows, err := structuresql.New(tx).ListRowHistory(ctx, structuresql.ListRowHistoryParams{
			TableID: table.id, RowID: id, PageOffset: int32(offset), PageLimit: historyPageSize + 1,
		})
		if err != nil {
			return fmt.Errorf("list row history: %w", err)
		}
		result.HasMore = len(rows) > historyPageSize
		if result.HasMore {
			rows = rows[:historyPageSize]
		}
		for _, row := range rows {
			item := RowHistory{
				ID: strconv.FormatInt(row.ID, 10), AuthorName: row.AuthorName,
				Action: row.Action, CreatedAt: row.CreatedAt.Time,
			}
			if row.UserID.Valid {
				authorID := row.UserID.String()
				item.AuthorID = &authorID
			}
			if err := json.Unmarshal(row.Changes, &item.Changes); err != nil {
				return fmt.Errorf("decode row history changes: %w", err)
			}
			result.Items = append(result.Items, item)
		}
		return nil
	})
	return result, err
}

func (s *Service) purgeRowHistory(ctx context.Context, now time.Time) (int64, error) {
	days, err := s.queries.ReadHistoryRetentionDays(ctx)
	if err != nil {
		return 0, fmt.Errorf("read history retention: %w", err)
	}
	if days < 1 || days > 3650 {
		return 0, errors.New("history retention is outside the supported bounds")
	}
	cutoff := now.Add(-time.Duration(days) * 24 * time.Hour)
	count, err := s.queries.PurgeRowHistory(ctx, pgtype.Timestamptz{Time: cutoff, Valid: true})
	if err != nil {
		return 0, fmt.Errorf("purge row history: %w", err)
	}
	return count, nil
}

func (s *Service) RunHistoryRetentionWorker(ctx context.Context, logger *slog.Logger) {
	ticker := time.NewTicker(historyRetentionInterval)
	defer ticker.Stop()
	for {
		if _, err := s.purgeRowHistory(ctx, time.Now()); err != nil && ctx.Err() == nil {
			logger.Error("row history retention failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
