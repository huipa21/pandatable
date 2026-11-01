package httpapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pandatable/internal/apperr"
	"pandatable/internal/structure"
)

func importTemporaryFile(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("private import data"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func nonemptyImportDirectory(t *testing.T) (string, string) {
	t.Helper()
	directory := t.TempDir()
	child := filepath.Join(directory, "retained")
	if err := os.WriteFile(child, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	return directory, child
}

func readyImportJob(t *testing.T, jobs *importJobs, path string) *importJob {
	t.Helper()
	job, err := jobs.reserve("user", structure.ImportTarget{Kind: "book", ID: "book"})
	if err != nil {
		t.Fatal(err)
	}
	jobs.mu.Lock()
	job.uploadPath, job.status = path, "ready"
	jobs.mu.Unlock()
	jobs.active.Done()
	return job
}

func requireImportFileRemoved(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary file %q still exists or cannot be checked: %v", path, err)
	}
}

func TestImportShutdownWaitsForUpload(t *testing.T) {
	jobs := newImportJobs(testLogger())
	path := importTemporaryFile(t, "upload.csv")
	job, err := jobs.reserve("user", structure.ImportTarget{Kind: "book", ID: "book"})
	if err != nil {
		t.Fatal(err)
	}
	jobs.mu.Lock()
	job.uploadPath = path
	jobs.mu.Unlock()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := jobs.shutdown(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("shutdown while upload is active = %v", err)
	}
	if _, err := jobs.reserve("another", structure.ImportTarget{Kind: "book", ID: "book"}); err == nil {
		t.Fatal("shutdown accepted a new upload")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("active upload was removed too early: %v", err)
	}
	jobs.active.Done()
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if err := jobs.shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	requireImportFileRemoved(t, path)
	if err := jobs.shutdown(ctx); err != nil {
		t.Fatalf("repeated shutdown = %v", err)
	}
}

func TestImportShutdownCancelsRunningJobAndRemovesReport(t *testing.T) {
	jobs := newImportJobs(testLogger())
	upload := importTemporaryFile(t, "upload.csv")
	report := importTemporaryFile(t, "report.csv")
	job := readyImportJob(t, jobs, upload)
	if _, _, err := jobs.start(job.id, job.userID); err != nil {
		t.Fatal(err)
	}
	finished := make(chan struct{})
	go func() {
		<-jobs.ctx.Done()
		jobs.finish(job.id, structure.ImportResult{}, report, "ERR_INTERNAL")
		jobs.active.Done()
		close(finished)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := jobs.shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, finished)
	requireImportFileRemoved(t, upload)
	requireImportFileRemoved(t, report)
	if _, _, err := jobs.status(job.id, job.userID); err == nil {
		t.Fatal("completed job remained accessible after shutdown")
	}
}

func TestImportShutdownReportsCleanupFailure(t *testing.T) {
	jobs := newImportJobs(testLogger())
	directory, _ := nonemptyImportDirectory(t)
	readyImportJob(t, jobs, directory)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := jobs.shutdown(ctx); err == nil || !strings.Contains(err.Error(), "remove import temporary file") {
		t.Fatalf("cleanup failure = %v", err)
	}
}

func TestImportDiscardRetriesCleanupFailure(t *testing.T) {
	jobs := newImportJobs(testLogger())
	directory, child := nonemptyImportDirectory(t)
	job := readyImportJob(t, jobs, directory)
	if err := jobs.discard(job.id, job.userID); err == nil {
		t.Fatal("discard succeeded despite a cleanup failure")
	}
	if status, _, err := jobs.status(job.id, job.userID); err != nil || status.Status != "ready" {
		t.Fatalf("job was not retained for retry: %+v, %v", status, err)
	}
	if err := os.Remove(child); err != nil {
		t.Fatal(err)
	}
	if err := jobs.discard(job.id, job.userID); err != nil {
		t.Fatal(err)
	}
	requireImportFileRemoved(t, directory)
	if err := jobs.shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestImportExpiryRetriesCleanupFailure(t *testing.T) {
	jobs := newImportJobs(testLogger())
	directory, child := nonemptyImportDirectory(t)
	job := readyImportJob(t, jobs, directory)
	jobs.expire(job.id)
	if status, _, err := jobs.status(job.id, job.userID); err != nil || status.Status != "ready" {
		t.Fatalf("expired job was not retained for retry: %+v, %v", status, err)
	}
	if err := os.Remove(child); err != nil {
		t.Fatal(err)
	}
	jobs.expire(job.id)
	requireImportFileRemoved(t, directory)
	if _, _, err := jobs.status(job.id, job.userID); err == nil {
		t.Fatal("expired job remained after successful cleanup")
	}
	if err := jobs.shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestImportFinishRetainsUploadAfterCleanupFailure(t *testing.T) {
	jobs := newImportJobs(testLogger())
	directory, child := nonemptyImportDirectory(t)
	job := readyImportJob(t, jobs, directory)
	if _, _, err := jobs.start(job.id, job.userID); err != nil {
		t.Fatal(err)
	}
	jobs.finish(job.id, structure.ImportResult{}, "", "")
	jobs.active.Done()
	if status, _, err := jobs.status(job.id, job.userID); err != nil || status.Status != "done" {
		t.Fatalf("finished job status = %+v, %v", status, err)
	}
	if err := os.Remove(child); err != nil {
		t.Fatal(err)
	}
	if err := jobs.discard(job.id, job.userID); err != nil {
		t.Fatal(err)
	}
	requireImportFileRemoved(t, directory)
	if err := jobs.shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestImportDiscardConcurrentWithShutdown(t *testing.T) {
	for range 50 {
		jobs := newImportJobs(testLogger())
		path := importTemporaryFile(t, "pending.csv")
		job := readyImportJob(t, jobs, path)
		start := make(chan struct{})
		discarded := make(chan error, 1)
		go func() {
			<-start
			discarded <- jobs.discard(job.id, job.userID)
		}()
		close(start)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := jobs.shutdown(ctx)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		if err := waitError(t, discarded); err != nil {
			var app *apperr.Error
			if !errors.As(err, &app) || app.Code != "ERR_IMPORT_BUSY" {
				t.Fatalf("concurrent discard = %v", err)
			}
		}
		requireImportFileRemoved(t, path)
	}
}

func TestRunShutsDownImportJobs(t *testing.T) {
	jobs := newImportJobs(testLogger())
	path := importTemporaryFile(t, "pending.csv")
	readyImportJob(t, jobs, path)
	handler, err := New(testLogger(), testAssets())
	if err != nil {
		t.Fatal(err)
	}
	handler.(*managedHandler).imports = jobs
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Run(ctx, testListener(t), handler, testLogger()); err != nil {
		t.Fatal(err)
	}
	requireImportFileRemoved(t, path)
}

func TestRunReportsImportCleanupFailure(t *testing.T) {
	jobs := newImportJobs(testLogger())
	directory, _ := nonemptyImportDirectory(t)
	readyImportJob(t, jobs, directory)
	handler, err := New(testLogger(), testAssets())
	if err != nil {
		t.Fatal(err)
	}
	handler.(*managedHandler).imports = jobs
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Run(ctx, testListener(t), handler, testLogger()); err == nil ||
		!strings.Contains(err.Error(), "shutdown HTTP handler") {
		t.Fatalf("server cleanup failure = %v", err)
	}
}
