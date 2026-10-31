package structure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"pandatable/internal/attachments"
)

type retryPurgeFiles struct {
	store    *attachments.Service
	failNext bool
}

func (files *retryPurgeFiles) RemovePurgedFile(ctx context.Context, id, path string, thumb bool) error {
	if files.failNext {
		files.failNext = false
		return errors.New("simulated attachment removal failure")
	}
	return files.store.RemovePurgedFile(ctx, id, path, thumb)
}

type unusedPurgeFiles struct{}

func (unusedPurgeFiles) RemovePurgedFile(context.Context, string, string, bool) error {
	return errors.New("unexpected attachment removal")
}

func uploadTrashImage(t *testing.T, f *fixture, owner Actor, tableID, fieldID string) (*attachments.Service, attachments.File, string) {
	t.Helper()
	root := t.TempDir()
	store, err := attachments.New(f.pool, f.resolver, root)
	if err != nil {
		t.Fatal(err)
	}
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	file, err := store.Upload(f.ctx, owner.ID, tableID, fieldID, "image.png", &imageBytes, 1<<20, 16)
	if err != nil || file.Thumb == nil {
		t.Fatalf("upload image = %+v, %v", file, err)
	}
	path := scanOne[string](f, `SELECT storage_path FROM meta.attachments WHERE id=$1`, file.ID)
	return store, file, filepath.Join(root, filepath.FromSlash(path))
}

func assertPathExists(t *testing.T, path string, exists bool) {
	t.Helper()
	_, err := os.Stat(path)
	if exists && err != nil || !exists && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("path %q exists=%t: %v", path, exists, err)
	}
}

func TestTrashPurgeBoundaryAndDurableAttachmentRetry(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	ft := f.fieldTable(owner)
	field := ft.create(FieldInput{Name: "Photo", UIType: "attachment"})
	columnName := scanOne[string](f, `SELECT pg_column FROM meta.fields WHERE id=$1`, field.ID)
	store, file, path := uploadTrashImage(t, f, owner, ft.id(), field.ID)
	encoded, err := json.Marshal([]attachments.File{file})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := f.service.CreateRows(f.ctx, owner, ft.id(),
		[]RowInsert{{Fields: map[string]json.RawMessage{field.ID: encoded}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.DeleteField(f.ctx, owner, field.ID, field.Version); err != nil {
		t.Fatal(err)
	}
	_, err = store.Open(f.ctx, owner.ID, file.ID, false)
	expectError(t, err, 404, "ERR_NOT_FOUND")
	mustRestore(t, f, owner, "field", field.ID)
	opened, err := store.Open(f.ctx, owner.ID, file.ID, true)
	if err != nil {
		t.Fatalf("restored attachment thumbnail: %v", err)
	}
	if err := opened.Reader.Close(); err != nil {
		t.Fatal(err)
	}
	row, err := f.service.GetRow(f.ctx, owner, ft.id(), rows[0].ID)
	if err != nil || !strings.Contains(string(row.Fields[field.ID]), file.ID) {
		t.Fatalf("restored attachment cell = %+v, %v", row, err)
	}
	schema, err := f.service.Schema(f.ctx, owner, ft.id())
	if err != nil {
		t.Fatal(err)
	}
	var restoredVersion int32
	for _, current := range schema.Fields {
		if current.ID == field.ID {
			restoredVersion = current.Version
		}
	}
	if restoredVersion == 0 {
		t.Fatal("restored attachment field missing")
	}
	if _, err := f.service.DeleteField(f.ctx, owner, field.ID, restoredVersion); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	cutoff := now.Add(-30 * 24 * time.Hour)
	f.exec(`UPDATE meta.fields SET deleted_at=$2 WHERE id=$1`, field.ID, cutoff)
	files := &retryPurgeFiles{store: store, failNext: true}
	if count, err := f.service.PurgeTrash(f.ctx, now, files); err != nil || count != 0 {
		t.Fatalf("purge exactly at cutoff = %d, %v", count, err)
	}
	assertPathExists(t, path, true)
	assertPathExists(t, path+".thumb.png", true)
	f.exec(`UPDATE meta.fields SET deleted_at=$2 WHERE id=$1`, field.ID, cutoff.Add(-time.Microsecond))
	count, err := f.service.PurgeTrash(f.ctx, now, files)
	if count != 1 || err == nil || !strings.Contains(err.Error(), "simulated attachment removal failure") {
		t.Fatalf("purge with file failure = %d, %v", count, err)
	}
	if scanOne[int](f, `SELECT count(*) FROM meta.fields WHERE id=$1`, field.ID) != 0 ||
		scanOne[int](f, `SELECT count(*) FROM meta.attachments WHERE id=$1`, file.ID) != 0 ||
		scanOne[int](f, `SELECT count(*) FROM meta.attachment_purge_queue WHERE id=$1`, file.ID) != 1 {
		t.Fatal("purged metadata or durable attachment queue is inconsistent")
	}
	schemaName, tableName := f.physicalNames(ft.id())
	if scanOne[bool](f, `SELECT EXISTS (SELECT 1 FROM information_schema.columns
		WHERE table_schema=$1 AND table_name=$2 AND column_name=$3)`, schemaName, tableName, columnName) {
		t.Fatal("purged field column still exists")
	}
	assertPathExists(t, path, true)
	assertPathExists(t, path+".thumb.png", true)
	if count, err := f.service.PurgeTrash(f.ctx, now, files); err != nil || count != 0 {
		t.Fatalf("retry queued file removal = %d, %v", count, err)
	}
	if scanOne[int](f, `SELECT count(*) FROM meta.attachment_purge_queue WHERE id=$1`, file.ID) != 0 {
		t.Fatal("successful file retry left a queue entry")
	}
	assertPathExists(t, path, false)
	assertPathExists(t, path+".thumb.png", false)
}

func TestTrashPurgeNestedRetentionAndDDLRollback(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	workspace := f.workspace(owner)
	book := f.book(owner, workspace.ID)
	table := f.table(owner, book.ID)
	fieldResult, err := f.service.CreateField(f.ctx, owner, table.Table.ID, FieldInput{Name: "Photo", UIType: "attachment"})
	if err != nil {
		t.Fatal(err)
	}
	field := *fieldResult.Field
	store, file, path := uploadTrashImage(t, f, owner, table.Table.ID, field.ID)
	view, err := f.service.CreateView(f.ctx, owner, table.Table.ID, ViewInput{Name: "Retained", Scope: "shared"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.DeleteView(f.ctx, owner, view.ID, view.Version); err != nil {
		t.Fatal(err)
	}
	if err := f.service.DeleteWorkspace(f.ctx, owner, workspace.ID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	cutoff := now.Add(-30 * 24 * time.Hour)
	f.exec(`UPDATE meta.workspaces SET deleted_at=$2 WHERE id=$1`, workspace.ID, cutoff.Add(-time.Hour))
	f.exec(`UPDATE meta.views SET deleted_at=$2 WHERE id=$1`, view.ID, cutoff)
	if count, err := f.service.PurgeTrash(f.ctx, now, store); err != nil || count != 0 {
		t.Fatalf("purge protected parent = %d, %v", count, err)
	}
	schemaName, tableName := f.physicalNames(table.Table.ID)
	if !f.schemaExists(schemaName) || scanOne[int](f, `SELECT count(*) FROM meta.workspaces WHERE id=$1`, workspace.ID) != 1 {
		t.Fatal("newer child did not protect parent")
	}
	assertPathExists(t, path, true)
	f.exec(`UPDATE meta.views SET deleted_at=$2 WHERE id=$1`, view.ID, cutoff.Add(-time.Microsecond))
	blocker := pgx.Identifier{schemaName, "purge_blocker"}.Sanitize()
	f.exec("CREATE TABLE " + blocker + " (id integer)")
	candidate := purgeCandidate{kind: "workspace", id: mustUUID(t, workspace.ID), workspace: mustUUID(t, workspace.ID)}
	if done, err := f.service.purgeCandidate(f.ctx, candidate, now); err == nil || done {
		t.Fatalf("blocked schema purge = %t, %v", done, err)
	}
	if !f.schemaExists(schemaName) ||
		!scanOne[bool](f, `SELECT EXISTS (SELECT 1 FROM information_schema.tables
			WHERE table_schema=$1 AND table_name=$2)`, schemaName, tableName) ||
		scanOne[int](f, `SELECT count(*) FROM meta.attachments WHERE id=$1`, file.ID) != 1 ||
		scanOne[int](f, `SELECT count(*) FROM meta.attachment_purge_queue WHERE id=$1`, file.ID) != 0 {
		t.Fatal("failed DDL did not roll back physical and metadata changes")
	}
	assertPathExists(t, path, true)
	f.exec("DROP TABLE " + blocker)
	if count, err := f.service.PurgeTrash(f.ctx, now, store); err != nil || count != 1 {
		t.Fatalf("retry nested purge = %d, %v", count, err)
	}
	if f.schemaExists(schemaName) ||
		scanOne[int](f, `SELECT count(*) FROM meta.workspaces WHERE id=$1`, workspace.ID) != 0 ||
		scanOne[int](f, `SELECT count(*) FROM meta.grants WHERE resource_id=ANY($1::uuid[])`,
			[]string{workspace.ID, book.ID, table.Table.ID}) != 0 {
		t.Fatal("nested purge left physical or metadata resources")
	}
	assertPathExists(t, path, false)
	assertPathExists(t, path+".thumb.png", false)
}

func TestTrashRelatedTableRestoreAndPurge(t *testing.T) {
	for _, kind := range []string{"one_many", "many_many"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			owner := f.user("user", "active", "en")
			workspace := f.workspace(owner)
			book := f.book(owner, workspace.ID)
			target, targetLabel := relationTable(t, f, owner, book.ID, "Projects")
			source, sourceLabel := relationTable(t, f, owner, book.ID, "Tasks")
			side := "child"
			if kind == "many_many" {
				side = "parent"
			}
			created, err := f.service.CreateRelation(f.ctx, owner, source.Table.ID, RelationInput{
				Name: "Project", ReverseName: "Tasks", TargetTableID: target.Table.ID,
				Kind: kind, SourceSide: side, OnDelete: "set_null",
			})
			if err != nil {
				t.Fatal(err)
			}
			targetRow := relationRow(t, f, owner, target.Table.ID, targetLabel.ID, "Project")
			sourceRow := relationRow(t, f, owner, source.Table.ID, sourceLabel.ID, "Task")
			if _, err := f.service.ChangeLinks(f.ctx, owner, source.Table.ID, sourceRow, created.Field.ID,
				LinkChange{Add: []string{targetRow}}); err != nil {
				t.Fatal(err)
			}
			if err := f.service.DeleteTable(f.ctx, owner, target.Table.ID); err != nil {
				t.Fatal(err)
			}
			hiddenSchema, err := f.service.Schema(f.ctx, owner, source.Table.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, field := range hiddenSchema.Fields {
				if field.ID == created.Field.ID {
					t.Fatal("relation to deleted table remains visible")
				}
			}
			hiddenRow, err := f.service.GetRow(f.ctx, owner, source.Table.ID, sourceRow)
			if err != nil {
				t.Fatal(err)
			}
			if _, visible := hiddenRow.Fields[created.Field.ID]; visible {
				t.Fatal("relation to deleted table remains queryable")
			}
			mustRestore(t, f, owner, "table", target.Table.ID)
			restoredRow, err := f.service.GetRow(f.ctx, owner, source.Table.ID, sourceRow)
			if err != nil {
				t.Fatal(err)
			}
			links := relationValues(t, restoredRow, created.Field.ID)
			if len(links) != 1 || links[0].ID != targetRow {
				t.Fatalf("restored table links = %+v", links)
			}
			if err := f.service.DeleteTable(f.ctx, owner, target.Table.ID); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC().Truncate(time.Microsecond)
			f.exec(`UPDATE meta.tables SET deleted_at=$2 WHERE id=$1`, target.Table.ID, now.Add(-31*24*time.Hour))
			if count, err := f.service.PurgeTrash(f.ctx, now, unusedPurgeFiles{}); err != nil || count != 1 {
				t.Fatalf("purge related table = %d, %v", count, err)
			}
			if scanOne[int](f, `SELECT count(*) FROM meta.relations WHERE id=$1`, created.Relation.ID) != 0 ||
				scanOne[int](f, `SELECT count(*) FROM meta.fields WHERE id=$1`, created.Field.ID) != 0 {
				t.Fatal("purged relation metadata remained on surviving table")
			}
			if _, err := f.service.GetRow(f.ctx, owner, source.Table.ID, sourceRow); err != nil {
				t.Fatalf("surviving table unusable after relation purge: %v", err)
			}
		})
	}
}

func TestTrashPurgesBothRelationFieldsAndTheirStorage(t *testing.T) {
	for _, kind := range []string{"one_many", "many_many"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			owner := f.user("user", "active", "en")
			workspace := f.workspace(owner)
			book := f.book(owner, workspace.ID)
			target, _ := relationTable(t, f, owner, book.ID, "Projects")
			source, _ := relationTable(t, f, owner, book.ID, "Tasks")
			side := "child"
			if kind == "many_many" {
				side = "parent"
			}
			created, err := f.service.CreateRelation(f.ctx, owner, source.Table.ID, RelationInput{
				Name: "Project", ReverseName: "Tasks", TargetTableID: target.Table.ID,
				Kind: kind, SourceSide: side, OnDelete: "set_null",
			})
			if err != nil {
				t.Fatal(err)
			}
			var column, junction *string
			if err := f.pool.QueryRow(f.ctx, `SELECT fk_column,junction_table FROM meta.relations WHERE id=$1`,
				created.Relation.ID).Scan(&column, &junction); err != nil {
				t.Fatal(err)
			}
			schema, table := f.physicalNames(source.Table.ID)
			if _, err := f.service.DeleteField(f.ctx, owner, created.Field.ID, created.Field.Version); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC().Truncate(time.Microsecond)
			ids := []string{created.Relation.ParentFieldID, created.Relation.ChildFieldID}
			f.exec(`UPDATE meta.fields SET deleted_at=$2 WHERE id=ANY($1::uuid[])`, ids, now.Add(-31*24*time.Hour))
			if count, err := f.service.PurgeTrash(f.ctx, now, unusedPurgeFiles{}); err != nil || count != 1 {
				t.Fatalf("purge relation fields = %d, %v", count, err)
			}
			if scanOne[int](f, `SELECT count(*) FROM meta.fields WHERE id=ANY($1::uuid[])`, ids) != 0 ||
				scanOne[int](f, `SELECT count(*) FROM meta.relations WHERE id=$1`, created.Relation.ID) != 0 {
				t.Fatal("relation metadata remained after purge")
			}
			if column != nil && scanOne[bool](f, `SELECT EXISTS (SELECT 1 FROM information_schema.columns
				WHERE table_schema=$1 AND table_name=$2 AND column_name=$3)`, schema, table, *column) {
				t.Fatal("relation FK column remained after purge")
			}
			if junction != nil && scanOne[bool](f, `SELECT EXISTS (SELECT 1 FROM information_schema.tables
				WHERE table_schema=$1 AND table_name=$2)`, schema, *junction) {
				t.Fatal("relation junction remained after purge")
			}
		})
	}
}

func TestTrashWorkerStopsOnCancellation(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithCancel(f.ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		f.service.RunTrashRetentionWorker(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), unusedPurgeFiles{})
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("trash retention worker did not stop after cancellation")
	}
}
