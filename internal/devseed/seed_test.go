package devseed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"pandatable/internal/acl"
	"pandatable/internal/codec"
	"pandatable/internal/structure"
	"pandatable/internal/structure/structuresql"
	"pandatable/internal/testdb"
	"pandatable/internal/workspace"
)

var database testdb.Suite

func TestMain(m *testing.M) { os.Exit(database.Run(m)) }

func TestFixtureBoundsAndCodecs(t *testing.T) {
	ids := map[string]string{"name": "00000000-0000-4000-8000-000000000001"}
	for index, spec := range fieldSpecs() {
		ids[spec.key] = fmt.Sprintf("00000000-0000-4000-8000-%012d", index+2)
	}
	for _, index := range []int{1, 2, 5, 10, MaxRows} {
		row, err := fixtureRow(index, ids)
		if err != nil {
			t.Fatal(err)
		}
		again, err := fixtureRow(index, ids)
		if err != nil || !reflect.DeepEqual(row, again) {
			t.Fatal("fixture is not deterministic")
		}
		if len(row.Fields)*BatchRows > 5000 {
			t.Fatal("seed batch exceeds cell bound")
		}
		batch := make([]structure.RowInsert, BatchRows)
		for i := range batch {
			batch[i] = row
		}
		raw, err := json.Marshal(struct {
			Rows []structure.RowInsert `json:"rows"`
		}{batch})
		if err != nil || len(raw) >= 512*1024 {
			t.Fatal("seed shape exceeds body bound")
		}
		for _, spec := range fieldSpecs() {
			field, err := codec.Bind(codec.Kind(spec.input.UIType), spec.input.Options)
			if err != nil {
				t.Fatal(err)
			}
			if raw, supplied := row.Fields[ids[spec.key]]; supplied {
				if _, err := field.Decode(raw); err != nil {
					t.Fatalf("fixture %d %s: %v", index, spec.key, err)
				}
			}
		}
		if index%2 != 0 && row.Fields[ids["checkbox"]] != nil || index%5 == 0 && row.Fields[ids["single_select"]] != nil {
			t.Fatal("fixture supplied an omitted physical default")
		}
	}
}

type fakeWriter struct {
	calls  int
	failAt int
	cancel context.CancelFunc
}

func (w *fakeWriter) CreateRows(_ context.Context, _ structure.Actor, _ string, rows []structure.RowInsert) ([]structure.RowRecord, error) {
	w.calls++
	if w.calls == w.failAt {
		return nil, errors.New("batch failed")
	}
	if w.cancel != nil {
		w.cancel()
	}
	return make([]structure.RowRecord, len(rows)), nil
}

func TestBatchFailureAndCancellationCheckpoints(t *testing.T) {
	ids := map[string]string{"name": "name"}
	for _, spec := range fieldSpecs() {
		ids[spec.key] = spec.key
	}
	for _, mode := range []string{"complete", "failure", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			writer := &fakeWriter{}
			if mode == "failure" {
				writer.failAt = 2
			}
			if mode == "cancel" {
				writer.cancel = cancel
			}
			result := Manifest{RequestedRows: 601, FieldIDs: ids}
			err := writeRows(ctx, writer, structure.Actor{}, &result)
			if mode == "complete" {
				if err != nil || result.CommittedRows != 601 || writer.calls != 3 {
					t.Fatalf("complete=%+v calls=%d err=%v", result, writer.calls, err)
				}
			} else if err == nil || result.CommittedRows != BatchRows || result.Complete {
				t.Fatalf("lost failure checkpoint: %+v %v", result, err)
			}
			if mode == "failure" && result.AttemptedRows != 600 {
				t.Fatal("uncertain attempted batch was not recorded")
			}
			if mode == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation lost")
			}
		})
	}
}

func TestSeedPostgreSQL(t *testing.T) {
	pool := database.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	name := pool.Config().ConnConfig.Database
	var owner string
	if err := pool.QueryRow(ctx, `INSERT INTO meta.users (username,display_name,password_hash,avatar_color,role)
		VALUES ('seed-owner','Seed owner','not-a-login-hash','blue','user') RETURNING id::text`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	for _, input := range []struct {
		name, owner string
		rows        int
	}{{"production", owner, 1}, {name, owner, 0}, {name, owner, MaxRows + 1}, {name, "bad", 1},
		{name + "x", owner, 1}, {name, "00000000-0000-4000-8000-000000000001", 1}} {
		result, err := Run(ctx, pool, input.name, input.owner, input.rows)
		if err == nil || result.Complete || result.WorkspaceID != "" {
			t.Fatalf("unsafe seed accepted: %+v %v", result, err)
		}
	}
	if _, err := pool.Exec(ctx, "UPDATE meta.users SET must_change_password=true WHERE id=$1", owner); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(ctx, pool, name, owner, 1); err == nil {
		t.Fatal("seed accepted forced-password owner")
	}
	if _, err := pool.Exec(ctx, "UPDATE meta.users SET must_change_password=false, status='disabled' WHERE id=$1", owner); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(ctx, pool, name, owner, 1); err == nil {
		t.Fatal("seed accepted disabled owner")
	}
	if _, err := pool.Exec(ctx, "UPDATE meta.users SET status='active' WHERE id=$1", owner); err != nil {
		t.Fatal(err)
	}
	lock, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lock.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended('pandatable.dev.seed', 0))"); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(ctx, pool, name, owner, 1); err == nil {
		t.Fatal("concurrent seed accepted")
	}
	if err := lock.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err := withSeedLock(ctx, pool, func(_ *structuresql.Queries) error {
		var idleTransaction bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM pg_stat_activity a JOIN pg_locks l ON l.pid=a.pid
			WHERE l.locktype='advisory' AND a.datname=current_database() AND a.state='idle in transaction'
		)`).Scan(&idleTransaction); err != nil {
			t.Fatal(err)
		}
		if idleTransaction {
			t.Fatal("seed coordination holds an idle transaction between batches")
		}
		if _, err := Run(ctx, pool, name, owner, 1); err == nil {
			t.Fatal("session seed lock did not exclude a concurrent seed")
		}
		return context.Canceled
	}); !errors.Is(err, context.Canceled) {
		t.Fatal("coordination error was swallowed")
	}
	if err := withSeedLock(ctx, pool, func(_ *structuresql.Queries) error { return nil }); err != nil {
		t.Fatalf("seed lock leaked after failure: %v", err)
	}
	canceled, stop := context.WithCancel(ctx)
	stop()
	if _, err := Run(canceled, pool, name, owner, 1); err == nil {
		t.Fatal("canceled seed accepted")
	}
	userID, ok := acl.ParseUUID(owner)
	if !ok {
		t.Fatal("fixture owner ID is invalid")
	}
	var starter string
	if err := pool.WithTx(ctx, func(tx pgx.Tx) error {
		id, err := workspace.CreateStarter(ctx, structuresql.New(tx), userID)
		starter = id.String()
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, mutation string
	}{
		{"icon", "UPDATE meta.workspaces SET icon='x' WHERE id=$1"},
		{"color", "UPDATE meta.workspaces SET color='#123456' WHERE id=$1"},
		{"trashed", "UPDATE meta.workspaces SET deleted_at=now() WHERE id=$1"},
		{"other workspace", "INSERT INTO meta.workspaces (name,created_by) SELECT 'Existing',created_by FROM meta.workspaces WHERE id=$1"},
		{"book", "INSERT INTO meta.books (workspace_id,name,pg_schema,position) VALUES ($1,'Existing','b_abcdefghij',1)"},
		{"changed grant", "UPDATE meta.grants SET level='read' WHERE resource_id=$1"},
	} {
		t.Run("reject "+test.name, func(t *testing.T) {
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(ctx) }()
			if _, err := tx.Exec(ctx, test.mutation, starter); err != nil {
				t.Fatal(err)
			}
			pristine, err := structuresql.New(tx).IsOnlyPristineStarter(ctx, userID)
			if err != nil || pristine {
				t.Fatalf("modified starter accepted: pristine=%t err=%v", pristine, err)
			}
		})
	}
	if _, err := pool.Exec(ctx, "UPDATE meta.workspaces SET name='User data' WHERE id=$1", starter); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(ctx, pool, name, owner, 1); err == nil {
		t.Fatal("seed accepted a modified starter workspace")
	}
	if _, err := pool.Exec(ctx, "UPDATE meta.workspaces SET name='Default' WHERE id=$1", starter); err != nil {
		t.Fatal(err)
	}
	result, err := Run(ctx, pool, name, owner, 601)
	if err != nil || !result.Complete || result.CommittedRows != 601 || len(result.FieldIDs) != 13 {
		t.Fatalf("real seed=%+v err=%v", result, err)
	}
	var starterName string
	if err := pool.QueryRow(ctx, "SELECT name FROM meta.workspaces WHERE id=$1", starter).Scan(&starterName); err != nil || starterName != "Default" {
		t.Fatalf("seed changed the starter workspace: name=%q err=%v", starterName, err)
	}
	resolver, err := acl.NewResolver(pool)
	if err != nil {
		t.Fatal(err)
	}
	service, err := structure.New(pool, resolver)
	if err != nil {
		t.Fatal(err)
	}
	actor := structure.Actor{ID: owner, Locale: "en"}
	schema, err := service.Schema(ctx, actor, result.TableID)
	if err != nil || schema.MyLevel != "owner" {
		t.Fatalf("schema/authorization: %+v %v", schema, err)
	}
	page, err := service.QueryRows(ctx, actor, result.ViewID, structure.RowQueryRequest{})
	if err != nil || page.Total == nil || *page.Total != 601 || len(page.Rows) != 100 {
		t.Fatalf("real query: %+v %v", page, err)
	}
	first := page.Rows[0]
	for key, expected := range map[string]string{
		"name": `"Seed row 000001"`, "bigint": `"9007199254740994"`,
		"decimal": `"123456789012345678901234.000001"`, "datetime": `"2024-01-02T03:04:05.123456Z"`,
		"checkbox": `false`, "single_select": `"Done"`, "multi_select": `["Todo","Done"]`,
	} {
		if string(first.Fields[result.FieldIDs[key]]) != expected {
			t.Errorf("%s=%s, want %s", key, first.Fields[result.FieldIDs[key]], expected)
		}
	}
	if string(page.Rows[4].Fields[result.FieldIDs["single_select"]]) != `"Todo"` ||
		string(page.Rows[9].Fields[result.FieldIDs["decimal"]]) != `null` {
		t.Fatal("physical defaults/null fixture mismatch")
	}
	for _, field := range schema.Fields {
		if field.UIType == "created_by" || field.UIType == "updated_by" {
			if string(first.Fields[field.ID]) != `"`+owner+`"` {
				// Query correctly hides system fields; inspect the full row.
				row, err := service.GetRow(ctx, actor, result.TableID, "1")
				if err != nil || string(row.Fields[field.ID]) != `"`+owner+`"` {
					t.Fatalf("seed authorship: %+v %v", row, err)
				}
			}
		}
	}
	if again, err := Run(ctx, pool, name, owner, 601); err == nil || again.Complete || again.WorkspaceID != "" {
		t.Fatal("rerun reset or reused an occupied target")
	}
	// A real failed atomic batch leaves the original rows untouched.
	failed := result
	failed.CommittedRows, failed.AttemptedRows, failed.Complete = 0, 0, false
	if err := writeRows(ctx, service, actor, &failed); err == nil || failed.CommittedRows != 0 {
		t.Fatal("duplicate/unique failure was not atomic and explicit")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "SELECT id FROM meta.workspaces WHERE id=$1 FOR NO KEY UPDATE", result.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	bounded, end := context.WithTimeout(ctx, 50*time.Millisecond)
	start := time.Now()
	err = writeRows(bounded, service, actor, &failed)
	end()
	if err == nil || time.Since(start) > 2*time.Second || failed.CommittedRows != 0 {
		t.Fatalf("blocked batch cancellation: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		t.Fatal(err)
	}
	page, err = service.QueryRows(ctx, actor, result.ViewID, structure.RowQueryRequest{})
	if err != nil || page.Total == nil || *page.Total != 601 {
		t.Fatal("failure/cancellation changed committed data")
	}
	// Suite owns and drops this entire database, including physical schemas.
}
