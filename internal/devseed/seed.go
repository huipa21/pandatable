// Package devseed builds a disposable developer fixture through the ordinary
// authorized services. It is not reachable from the HTTP server.
package devseed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"pandatable/internal/acl"
	"pandatable/internal/auth/authsql"
	"pandatable/internal/db"
	"pandatable/internal/structure"
	"pandatable/internal/structure/structuresql"
)

const (
	MaxRows   = 200_000
	BatchRows = 300
	Timeout   = 10 * time.Minute
)

var databaseName = regexp.MustCompile(`^pt_(seed|test)_[a-z0-9_]{1,48}$`)

func ValidDatabase(name string) bool { return databaseName.MatchString(name) }

type Refusal string

func (err Refusal) Error() string { return string(err) }

type Manifest struct {
	Database      string            `json:"database"`
	WorkspaceID   string            `json:"workspaceId,omitempty"`
	BookID        string            `json:"bookId,omitempty"`
	TableID       string            `json:"tableId,omitempty"`
	ViewID        string            `json:"viewId,omitempty"`
	FieldIDs      map[string]string `json:"fieldIds,omitempty"`
	RequestedRows int               `json:"requestedRows"`
	CommittedRows int               `json:"committedRows"`
	AttemptedRows int               `json:"attemptedRows"`
	Stage         string            `json:"stage"`
	Complete      bool              `json:"complete"`
}

type rowWriter interface {
	CreateRows(context.Context, structure.Actor, string, []structure.RowInsert) ([]structure.RowRecord, error)
}

func Run(ctx context.Context, pool *db.Pool, confirmedDatabase, owner string, count int) (Manifest, error) {
	result := Manifest{Database: confirmedDatabase, RequestedRows: count, Stage: "preflight"}
	if !ValidDatabase(confirmedDatabase) || count < 1 || count > MaxRows {
		return result, Refusal("seed requires a disposable database and 1..200000 rows")
	}
	userID, ok := acl.ParseUUID(owner)
	if !ok {
		return result, Refusal("seed requires an existing active owner UUID")
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	err := withSeedLock(ctx, pool, func(q *structuresql.Queries) error {
		target, err := q.InspectDevSeedTarget(ctx)
		if err != nil {
			return fmt.Errorf("inspect developer seed target: %w", err)
		}
		if target.DatabaseName != confirmedDatabase || target.ServerVersion < 180000 || target.ServerVersion >= 190000 ||
			!target.Empty {
			return Refusal("seed requires the confirmed PG18 database with no workspaces (including Trash)")
		}
		user, err := authsql.New(pool).GetUser(ctx, userID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Refusal("seed owner must already exist and be active")
			}
			return fmt.Errorf("read seed owner: %w", err)
		}
		if user.MustChangePassword {
			return Refusal("seed owner must complete the normal password change first")
		}
		resolver, err := acl.NewResolver(pool)
		if err != nil {
			return err
		}
		service, err := structure.New(pool, resolver)
		if err != nil {
			return err
		}
		actor := structure.Actor{ID: user.ID.String(), Locale: user.Locale}
		result.Stage = "workspace"
		workspace, err := service.CreateWorkspace(ctx, actor, structure.WorkspaceInput{Name: "T1.20 developer seed"})
		if err != nil {
			return err
		}
		result.WorkspaceID = workspace.ID
		result.Stage = "book"
		book, err := service.CreateBook(ctx, actor, workspace.ID, "Performance baseline")
		if err != nil {
			return err
		}
		result.BookID = book.ID
		result.Stage = "table"
		schema, err := service.CreateTable(ctx, actor, book.ID, "Seed rows")
		if err != nil {
			return err
		}
		result.TableID = schema.Table.ID
		if schema.Table.DisplayFieldID == nil || len(schema.Views) != 1 {
			return errors.New("created seed table has an unexpected schema")
		}
		result.ViewID = schema.Views[0].ID
		result.FieldIDs = map[string]string{"name": *schema.Table.DisplayFieldID}
		result.Stage = "fields"
		required, unique := true, true
		if _, err := service.UpdateField(ctx, actor, result.FieldIDs["name"], structure.FieldPatch{
			Version: 1, IsRequired: &required, IsUnique: &unique,
		}); err != nil {
			return err
		}
		for _, spec := range fieldSpecs() {
			added, err := service.CreateField(ctx, actor, result.TableID, spec.input)
			if err != nil {
				return err
			}
			if added.Field == nil {
				return errors.New("created seed field is missing")
			}
			result.FieldIDs[spec.key] = added.Field.ID
		}
		result.Stage = "rows"
		if err := writeRows(ctx, service, actor, &result); err != nil {
			return err
		}
		result.Stage = "verify"
		page, err := service.QueryRows(ctx, actor, result.ViewID, structure.RowQueryRequest{})
		if err != nil {
			return err
		}
		if page.Total == nil || *page.Total != int64(count) || len(page.Rows) != min(count, 100) || page.Rows[0].ID != "1" {
			return errors.New("seed query verification did not match the requested fixture")
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	result.Stage, result.Complete = "complete", true
	return result, nil
}

func withSeedLock(ctx context.Context, pool *db.Pool, fn func(*structuresql.Queries) error) (err error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire seed coordination connection: %w", err)
	}
	q := structuresql.New(conn)
	locked, err := q.TryDevSeedLock(ctx)
	if err != nil {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		return errors.Join(fmt.Errorf("acquire developer seed lock: %w", err), conn.Hijack().Close(closeCtx))
	}
	if !locked {
		conn.Release()
		return Refusal("another developer seed is running")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		unlocked, unlockErr := q.ReleaseDevSeedLock(cleanup)
		if unlockErr != nil || !unlocked {
			err = errors.Join(err, errors.New("release developer seed lock failed"), unlockErr, conn.Hijack().Close(cleanup))
			return
		}
		conn.Release()
	}()
	return fn(q)
}

func writeRows(ctx context.Context, service rowWriter, actor structure.Actor, result *Manifest) error {
	for start := 1; start <= result.RequestedRows; start += BatchRows {
		if err := ctx.Err(); err != nil {
			return err
		}
		batch := make([]structure.RowInsert, 0, BatchRows)
		for index := start; index <= min(start+BatchRows-1, result.RequestedRows); index++ {
			row, err := fixtureRow(index, result.FieldIDs)
			if err != nil {
				return err
			}
			batch = append(batch, row)
		}
		result.AttemptedRows = result.CommittedRows + len(batch)
		batchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		rows, err := service.CreateRows(batchCtx, actor, result.TableID, batch)
		cancel()
		if err != nil {
			return fmt.Errorf("write seed batch: %w", err)
		}
		if len(rows) != len(batch) {
			return errors.New("seed batch returned an unexpected row count")
		}
		result.CommittedRows += len(rows)
	}
	return nil
}

type fieldSpec struct {
	key   string
	input structure.FieldInput
}

func fieldSpecs() []fieldSpec {
	choices := json.RawMessage(`{"choices":[{"id":"todo","label":"Todo","color":"gray"},{"id":"done","label":"Done","color":"green"}]}`)
	return []fieldSpec{
		{"notes", structure.FieldInput{Name: "Notes", UIType: "long_text"}},
		{"quantity", structure.FieldInput{Name: "Quantity", UIType: "integer"}},
		{"bigint", structure.FieldInput{Name: "Exact bigint", UIType: "integer", Options: json.RawMessage(`{"size":"big"}`)}},
		{"decimal", structure.FieldInput{Name: "Exact decimal", UIType: "decimal", Options: json.RawMessage(`{"precision":38,"scale":6}`)}},
		{"float", structure.FieldInput{Name: "Real", UIType: "float", Options: json.RawMessage(`{"size":"real"}`)}},
		{"checkbox", structure.FieldInput{Name: "Checked", UIType: "checkbox"}},
		{"date", structure.FieldInput{Name: "Date", UIType: "date"}},
		{"datetime", structure.FieldInput{Name: "Instant", UIType: "datetime"}},
		{"single_select", structure.FieldInput{Name: "Status", UIType: "single_select", Options: choices, Default: json.RawMessage(`"Todo"`)}},
		{"multi_select", structure.FieldInput{Name: "Tags", UIType: "multi_select", Options: choices}},
		{"email", structure.FieldInput{Name: "Email", UIType: "email"}},
		{"url", structure.FieldInput{Name: "URL", UIType: "url"}},
	}
}

func fixtureRow(index int, ids map[string]string) (structure.RowInsert, error) {
	values := map[string]any{
		"name":         fmt.Sprintf("Seed row %06d", index),
		"notes":        "Deterministic seed\nrow " + strconv.Itoa(index),
		"quantity":     index % 1000,
		"bigint":       strconv.FormatInt(9_007_199_254_740_993+int64(index), 10),
		"decimal":      fmt.Sprintf("123456789012345678901234.%06d", index),
		"float":        float32(index%1000) / 8,
		"date":         time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, index%366).Format("2006-01-02"),
		"datetime":     "2024-01-02T03:04:05.123456Z",
		"multi_select": []string{"Todo", "Done"},
		"email":        fmt.Sprintf("seed%06d@example.test", index),
		"url":          fmt.Sprintf("https://example.test/seed/%d", index),
	}
	if index%2 == 0 {
		values["checkbox"] = true
	}
	if index%5 != 0 {
		values["single_select"] = "Done"
	}
	if index%10 == 0 {
		for _, key := range []string{"notes", "bigint", "decimal", "float", "date", "datetime", "multi_select", "email", "url"} {
			values[key] = nil
		}
	}
	fields := make(map[string]json.RawMessage, len(values))
	for key, value := range values {
		raw, err := json.Marshal(value)
		if err != nil {
			return structure.RowInsert{}, fmt.Errorf("encode seed fixture: %w", err)
		}
		fields[ids[key]] = raw
	}
	return structure.RowInsert{Fields: fields}, nil
}
