package backup

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"pandatable/internal/apperr"
	"pandatable/internal/backup/backupsql"
	"pandatable/internal/db"
	"pandatable/internal/testdb"
)

var backupDatabase testdb.Suite

func TestMain(m *testing.M) { os.Exit(backupDatabase.Run(m)) }

type backupFixture struct {
	ctx     context.Context
	pool    *db.Pool
	dir     string
	userID  string
	manager *Manager
}

func newBackupFixture(t *testing.T) *backupFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	pool := backupDatabase.Open(t)
	user := strings.ToLower(rand.Text())
	var userID string
	if err := pool.QueryRow(ctx, `INSERT INTO meta.users
		(username, display_name, password_hash, role, avatar_color)
		VALUES ($1, 'Backup owner', 'fixture', 'admin', 'blue') RETURNING id::text`, user).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := pool.Exec(cleanup, "DELETE FROM meta.backups WHERE created_by=$1", userID); err != nil {
			t.Error(err)
		}
		if _, err := pool.Exec(cleanup, "DELETE FROM meta.users WHERE id=$1", userID); err != nil {
			t.Error(err)
		}
	})
	dir := t.TempDir()
	manager, err := New(ctx, pool, backupDatabase.URL(t), dir, "0.0.0-dev", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if err := manager.Close(cleanup); err != nil {
			t.Error(err)
		}
	})
	return &backupFixture{ctx: ctx, pool: pool, dir: dir, userID: userID, manager: manager}
}

func waitBackup(t *testing.T, f *backupFixture, id string) Record {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		record, err := f.manager.Get(f.ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if record.Status != "running" {
			return record
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("backup did not finish")
	return Record{}
}

func requireBackupCode(t *testing.T, err error, code string) {
	t.Helper()
	var app *apperr.Error
	if !errors.As(err, &app) || app.Code != code {
		t.Fatalf("error=%v, want %s", err, code)
	}
}

func setBackupDatabaseIdleTimeout(t *testing.T, f *backupFixture) {
	t.Helper()
	if err := backupDatabase.CheckPoolOwnership(f.ctx, f.pool); err != nil {
		t.Fatal(err)
	}
	var name string
	if err := f.pool.QueryRow(f.ctx, "SELECT current_database()").Scan(&name); err != nil {
		t.Fatal(err)
	}
	database := pgx.Identifier{name}.Sanitize()
	if _, err := f.pool.Exec(f.ctx, "ALTER DATABASE "+database+" SET idle_in_transaction_session_timeout = '1s'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := f.pool.Exec(context.Background(), "ALTER DATABASE "+database+" RESET idle_in_transaction_session_timeout"); err != nil {
			t.Error(err)
		}
	})
}

func TestBackupSnapshotSurvivesIdleTimeout(t *testing.T) {
	f := newBackupFixture(t)
	setBackupDatabaseIdleTimeout(t, f)
	f.manager.dump = func(ctx context.Context, path, _ string) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
			return os.WriteFile(path, []byte("fake custom dump"), 0o600)
		}
	}
	record, err := f.manager.Start(f.ctx, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	if done := waitBackup(t, f, record.ID); done.Status != "done" {
		t.Fatalf("snapshot was lost during a slow dump: %+v", done)
	}
}

func TestBackupJobExclusionProgressAndLifecycle(t *testing.T) {
	f := newBackupFixture(t)
	if err := os.MkdirAll(filepath.Join(f.dir, "files", "2026", "11"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.dir, "files", "2026", "11", "file"), []byte("attachment"), 0o600); err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	f.manager.dump = func(ctx context.Context, path, snapshot string) error {
		if snapshot == "" {
			return errors.New("missing exported snapshot")
		}
		close(started)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-release:
			return os.WriteFile(path, []byte("fake custom dump"), 0o600)
		}
	}
	record, err := f.manager.Start(f.ctx, f.userID)
	if err != nil || record.Status != "running" || record.Progress == nil || record.Progress.Stage != "dumping" {
		t.Fatalf("start=%+v err=%v", record, err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("backup dump did not start")
	}
	if _, err := f.manager.Start(f.ctx, f.userID); err == nil {
		t.Fatal("second local backup was accepted")
	} else {
		requireBackupCode(t, err, "ERR_BACKUP_BUSY")
	}
	other, err := New(f.ctx, f.pool, backupDatabase.URL(t), f.dir, "0.0.0-dev", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Close(context.Background()) }()
	if _, err := other.Start(f.ctx, f.userID); err == nil {
		t.Fatal("second manager bypassed the database backup lock")
	} else {
		requireBackupCode(t, err, "ERR_BACKUP_BUSY")
	}
	listing, err := f.manager.List(f.ctx)
	if err != nil || len(listing.Items) != 1 || listing.Items[0].Progress == nil ||
		listing.Items[0].Progress.Stage != "dumping" || listing.FreeBytes == 0 {
		t.Fatalf("running listing=%+v err=%v", listing, err)
	}
	if err := f.manager.Delete(f.ctx, record.ID); err == nil {
		t.Fatal("running backup deleted")
	}
	close(release)
	done := waitBackup(t, f, record.ID)
	if done.Status != "done" || done.SizeBytes == nil || *done.SizeBytes <= 0 || done.FinishedAt == nil {
		t.Fatalf("finished record=%+v", done)
	}
	file, opened, err := f.manager.Open(f.ctx, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	info, err := file.Stat()
	if err != nil || info.Size() != *opened.SizeBytes {
		t.Fatalf("download size=%v err=%v", info, err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.manager.Delete(f.ctx, record.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(f.dir, "backups", record.FileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backup file remains: %v", err)
	}
	if _, err := f.manager.Get(f.ctx, record.ID); err == nil {
		t.Fatal("deleted metadata remains")
	}
}

func TestBackupDiskGuardAndFailedCleanup(t *testing.T) {
	f := newBackupFixture(t)
	f.manager.freeSpace = func(context.Context, string) (uint64, error) { return 0, nil }
	if _, err := f.manager.Start(f.ctx, f.userID); err == nil {
		t.Fatal("insufficient disk space accepted")
	} else {
		requireBackupCode(t, err, "ERR_BACKUP_DISK_SPACE")
	}
	var count int
	if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM meta.backups").Scan(&count); err != nil || count != 0 {
		t.Fatalf("disk refusal created a record: count=%d err=%v", count, err)
	}
	f.manager.freeSpace = func(context.Context, string) (uint64, error) { return math.MaxUint64, nil }
	f.manager.dump = func(context.Context, string, string) error { return errors.New("forced dump failure") }
	record, err := f.manager.Start(f.ctx, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	failed := waitBackup(t, f, record.ID)
	if failed.Status != "failed" || failed.ErrorCode != "ERR_BACKUP_FAILED" || failed.SizeBytes != nil {
		t.Fatalf("failed record=%+v", failed)
	}
	for _, path := range []string{
		filepath.Join(f.dir, "backups", record.FileName),
		filepath.Join(f.dir, "backups", "."+record.FileName+".partial"),
	} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("failed backup left a file: %v", err)
		}
	}
	if err := f.manager.Delete(f.ctx, record.ID); err != nil {
		t.Fatal(err)
	}
}

func TestBackupRecoveryMarksInterruptedJobsFailed(t *testing.T) {
	f := newBackupFixture(t)
	name := "pandatable-20261101-010203.zip"
	var id string
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO meta.backups(file_name,status,created_by)
		VALUES ($1,'running',$2) RETURNING id::text`, name, f.userID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{name, "." + name + ".partial"} {
		if err := os.WriteFile(filepath.Join(f.dir, "backups", path), []byte("partial"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	recovered, err := New(f.ctx, f.pool, backupDatabase.URL(t), f.dir, "0.0.0-dev", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = recovered.Close(context.Background()) }()
	record, err := recovered.Get(f.ctx, id)
	if err != nil || record.Status != "failed" || record.ErrorCode != "ERR_BACKUP_INTERRUPTED" {
		t.Fatalf("recovered=%+v err=%v", record, err)
	}
	for _, path := range []string{name, "." + name + ".partial"} {
		if _, err := os.Stat(filepath.Join(f.dir, "backups", path)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("interrupted backup file remains: %v", err)
		}
	}
}

func TestBackupCompletionResponseLossPreservesCommittedArchive(t *testing.T) {
	f := newBackupFixture(t)
	f.manager.dump = func(_ context.Context, path, _ string) error {
		return os.WriteFile(path, []byte("fake custom dump"), 0o600)
	}
	f.manager.finish = func(ctx context.Context, conn *pgx.Conn, params backupsql.FinishBackupParams) (int64, error) {
		if _, err := backupsql.New(conn).FinishBackup(ctx, params); err != nil {
			return 0, err
		}
		return 0, errors.New("simulated lost completion response")
	}
	record, err := f.manager.Start(f.ctx, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	done := waitBackup(t, f, record.ID)
	if err := f.manager.Close(f.ctx); err != nil {
		t.Fatal(err)
	}
	file, _, err := f.manager.Open(f.ctx, record.ID)
	if err != nil || done.Status != "done" {
		t.Fatalf("committed archive was not retained: status=%s err=%v", done.Status, err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestBackupCompletionFailureRemovesUncommittedArchive(t *testing.T) {
	f := newBackupFixture(t)
	f.manager.dump = func(_ context.Context, path, _ string) error {
		return os.WriteFile(path, []byte("fake custom dump"), 0o600)
	}
	f.manager.finish = func(context.Context, *pgx.Conn, backupsql.FinishBackupParams) (int64, error) {
		return 0, errors.New("simulated completion failure")
	}
	record, err := f.manager.Start(f.ctx, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	failed := waitBackup(t, f, record.ID)
	if err := f.manager.Close(f.ctx); err != nil {
		t.Fatal(err)
	}
	if failed.Status != "failed" || failed.ErrorCode != "ERR_BACKUP_FAILED" {
		t.Fatalf("unexpected failed backup: %+v", failed)
	}
	if _, err := os.Stat(filepath.Join(f.dir, "backups", record.FileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("uncommitted archive remains: %v", err)
	}
}

func TestBackupRecoveryReconcilesDeletionTombstones(t *testing.T) {
	f := newBackupFixture(t)
	const retainedName = "pandatable-20261101-010201.zip"
	const removedName = "pandatable-20261101-010202.zip"
	const failedName = "pandatable-20261101-010203.zip"
	var retainedID, removedID, failedID string
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO meta.backups(file_name,status,size_bytes,created_by,finished_at)
		VALUES ($1,'done',7,$2,now()) RETURNING id::text`, retainedName, f.userID).Scan(&retainedID); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(f.ctx, "SELECT gen_random_uuid()::text").Scan(&removedID); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO meta.backups(file_name,status,created_by,finished_at)
		VALUES ($1,'failed',$2,now()) RETURNING id::text`, failedName, f.userID).Scan(&failedID); err != nil {
		t.Fatal(err)
	}
	tombstones := []string{
		"." + retainedName + ".deleting-" + retainedID,
		"." + removedName + ".deleting-" + removedID,
		"." + failedName + ".deleting-" + failedID,
	}
	for _, name := range tombstones {
		if err := os.WriteFile(filepath.Join(f.dir, "backups", name), []byte("archive"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	recovered, err := New(f.ctx, f.pool, backupDatabase.URL(t), f.dir, "0.0.0-dev", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = recovered.Close(context.Background()) }()
	file, _, err := recovered.Open(f.ctx, retainedID)
	if err != nil {
		t.Fatalf("rolled-back deletion not restored: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	for _, name := range tombstones {
		if _, err := os.Stat(filepath.Join(f.dir, "backups", name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("deletion tombstone remains: %v", err)
		}
	}
}

func TestBackupRecoveryPreservesConflictingDeletionFiles(t *testing.T) {
	f := newBackupFixture(t)
	name := "pandatable-20261101-010201.zip"
	var id string
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO meta.backups(file_name,status,size_bytes,created_by,finished_at)
		VALUES ($1,'done',7,$2,now()) RETURNING id::text`, name, f.userID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	paths := []string{name, "." + name + ".deleting-" + id}
	for _, path := range paths {
		if err := os.WriteFile(filepath.Join(f.dir, "backups", path), []byte("archive"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := New(f.ctx, f.pool, backupDatabase.URL(t), f.dir, "0.0.0-dev", slog.New(slog.NewTextHandler(io.Discard, nil))); err == nil {
		t.Fatal("conflicting backup deletion files were accepted")
	}
	for _, path := range paths {
		if _, err := os.Stat(filepath.Join(f.dir, "backups", path)); err != nil {
			t.Fatalf("conflicting file removed: %v", err)
		}
	}
}

func TestBackupShutdownCancelsJob(t *testing.T) {
	f := newBackupFixture(t)
	started := make(chan struct{})
	f.manager.dump = func(ctx context.Context, _, _ string) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}
	record, err := f.manager.Start(f.ctx, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("backup dump did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := f.manager.Close(ctx); err != nil {
		t.Fatal(err)
	}
	failed, err := f.manager.Get(f.ctx, record.ID)
	if err != nil || failed.Status != "failed" {
		t.Fatalf("shutdown backup=%+v err=%v", failed, err)
	}
}
