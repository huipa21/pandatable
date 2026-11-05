package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
)

func TestSystemCPUUnitsAndAvailability(t *testing.T) {
	at := time.Now()
	before := cpuCounters(cpu.TimesStat{User: 10, System: 20, Idle: 60, Iowait: 10, Guest: 100})
	if before.total != 100 || before.idle != 70 {
		t.Fatal("guest double-counting or incorrect idle accounting")
	}
	initial := cpuDelta(at, nil, before)
	if initial.Status != "unavailable" || initial.Reason != "initializing" || initial.Value != nil {
		t.Fatal("first sample must not fabricate CPU")
	}
	after := cpuSample{total: 200, idle: 90}
	result := cpuDelta(at, &before, after)
	if result.Value == nil || *result.Value != 80 {
		t.Fatalf("CPU must be percent, not fraction: %+v", result)
	}
	idle := cpuDelta(at, &before, cpuSample{total: 200, idle: 170})
	if idle.Value == nil || *idle.Value != 0 {
		t.Fatal("genuine idle zero is valid")
	}
	for _, invalid := range []cpuSample{before, {total: 99, idle: 70}, {total: 200, idle: 69},
		{total: 200, idle: 171}, {total: math.NaN()}, {total: math.Inf(1)}, {total: 200, idle: math.NaN()}} {
		if cpuDelta(at, &before, invalid).Value != nil {
			t.Fatal("counter resets and invalid samples must be unavailable")
		}
	}
}

func testMonitor(collect func(context.Context) SystemSnapshot) *SystemMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	return &SystemMonitor{ctx: ctx, cancel: cancel, now: time.Now, collect: collect}
}

func TestSystemCacheCoalescingAndRequestCancellation(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	at := time.Now().UTC()
	monitor := testMonitor(func(ctx context.Context) SystemSnapshot {
		if calls.Add(1) == 1 {
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > 4*time.Second {
				t.Error("collector deadline missing")
			}
			close(started)
			<-release
		}
		return SystemSnapshot{CollectedAt: at}
	})
	defer monitor.Close()
	canceled, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { _, err := monitor.Snapshot(canceled); result <- err }()
	<-started
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("request cancellation=%v", err)
	}
	var workers sync.WaitGroup
	for range 32 {
		workers.Go(func() {
			value, err := monitor.Snapshot(context.Background())
			if err != nil || !value.CollectedAt.Equal(at) {
				t.Errorf("coalesced result=%+v err=%v", value, err)
			}
		})
	}
	close(release)
	workers.Wait()
	if calls.Load() != 1 {
		t.Fatalf("cache/coalescing collected %d times", calls.Load())
	}
	monitor.mu.Lock()
	monitor.expires = time.Time{}
	monitor.mu.Unlock()
	if _, err := monitor.Snapshot(context.Background()); err != nil || calls.Load() != 2 {
		t.Fatal("expired cache did not collect again", err)
	}
}

func TestSystemLifecycleCancellation(t *testing.T) {
	started := make(chan struct{})
	monitor := testMonitor(func(ctx context.Context) SystemSnapshot {
		close(started)
		<-ctx.Done()
		return SystemSnapshot{}
	})
	done := make(chan error, 1)
	go func() { _, err := monitor.Snapshot(context.Background()); done <- err }()
	<-started
	monitor.Close()
	code(t, <-done, "ERR_SYSTEM_UNAVAILABLE")
	_, err := monitor.Snapshot(context.Background())
	code(t, err, "ERR_SYSTEM_UNAVAILABLE")
}

func TestSystemFiveSecondCacheBoundary(t *testing.T) {
	var clock atomic.Int64
	var calls atomic.Int32
	clock.Store(time.Now().UnixNano())
	monitor := testMonitor(func(context.Context) SystemSnapshot {
		calls.Add(1)
		return SystemSnapshot{}
	})
	monitor.now = func() time.Time { return time.Unix(0, clock.Load()) }
	defer monitor.Close()
	if _, err := monitor.Snapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	clock.Add(int64(5*time.Second - time.Nanosecond))
	if _, err := monitor.Snapshot(context.Background()); err != nil || calls.Load() != 1 {
		t.Fatal("snapshot expired before five seconds", err)
	}
	clock.Add(1)
	if _, err := monitor.Snapshot(context.Background()); err != nil || calls.Load() != 2 {
		t.Fatal("snapshot must expire at five seconds", err)
	}
}

func storageFixture(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"files", "backups"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "files", "owned-fixture"), make([]byte, 4096), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestSystemDirectoryBoundsCacheAndErrors(t *testing.T) {
	dir := storageFixture(t)
	ctx := context.Background()
	bounds := scanBounds{entries: 100, depth: 4}
	size, err := directorySize(ctx, dir, "files", bounds)
	if err != nil || size != 4096 {
		t.Fatalf("logical bytes=%d err=%v", size, err)
	}
	size, err = directorySize(ctx, dir, "backups", bounds)
	if err != nil || size != 0 {
		t.Fatal("existing empty directory must be genuine zero", err)
	}
	for _, tc := range []struct {
		name   string
		bounds scanBounds
		want   string
	}{{"missing", bounds, "notFound"}, {"files", scanBounds{entries: 0, depth: 4}, "limit"}} {
		_, err := directorySize(ctx, dir, tc.name, tc.bounds)
		if err == nil || filesystemReason(err) != tc.want {
			t.Fatalf("error=%v want=%s", err, tc.want)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "files", "a", "b"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := directorySize(ctx, dir, "files", scanBounds{entries: 100, depth: 1}); !errors.Is(err, errScanLimit) {
		t.Fatal("depth bound not enforced", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := directorySize(canceled, dir, "files", bounds); !errors.Is(err, context.Canceled) {
		t.Fatal("filesystem cancellation not enforced", err)
	}
	var cache directoryCache
	files, backups := cache.sizes(ctx, dir)
	if files.Value == nil || *files.Value != 4096 || backups.Value == nil || *backups.Value != 0 ||
		time.Until(cache.expires) < 59*time.Minute {
		t.Fatal("incorrect independent hourly directory cache")
	}
	if err := os.WriteFile(filepath.Join(dir, "files", "new"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	cached, _ := cache.sizes(ctx, dir)
	if *cached.Value != 4096 || !cached.CollectedAt.Equal(files.CollectedAt) {
		t.Fatal("five-second snapshot refresh must not rescan directories")
	}
	cache.expires = time.Time{}
	rescanned, _ := cache.sizes(ctx, dir)
	if rescanned.Value == nil || *rescanned.Value != 4099 {
		t.Fatal("hourly cache expiration did not rescan")
	}
	var missing directoryCache
	notFound, _ := missing.sizes(ctx, filepath.Join(dir, "absent"))
	if notFound.Value != nil || notFound.Reason != "notFound" {
		t.Fatal("missing directory cannot be presented as empty")
	}
	if filesystemReason(os.ErrPermission) != "permission" {
		t.Fatal("permission errors must remain explicit")
	}
}

func TestSystemDirectorySymlinks(t *testing.T) {
	dir := storageFixture(t)
	if err := os.Symlink(filepath.Join(dir, "files", "owned-fixture"), filepath.Join(dir, "files", "link")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("Windows account cannot create symlinks; Linux image test covers this policy")
		}
		t.Fatal(err)
	}
	size, err := directorySize(context.Background(), dir, "files", scanBounds{entries: 100, depth: 4})
	if err != nil || size != 4096 {
		t.Fatal("symlinks must not contribute bytes", size, err)
	}
	if err := os.Symlink(filepath.Join(dir, "files"), filepath.Join(dir, "linked-root")); err != nil {
		t.Fatal(err)
	}
	if _, err := directorySize(context.Background(), dir, "linked-root", scanBounds{entries: 100, depth: 4}); !errors.Is(err, errFileType) {
		t.Fatal("linked root must not be traversed", err)
	}
}

func TestSystemCgroupV2Semantics(t *testing.T) {
	files := fstest.MapFS{
		"proc/self/cgroup":                 {Data: []byte("0::/app\n")},
		"proc/self/mountinfo":              {Data: []byte("1 0 0:1 / /sys/fs/cgroup rw - cgroup2 cgroup rw\n")},
		"sys/fs/cgroup/app/memory.current": {Data: []byte("1024\n")},
		"sys/fs/cgroup/app/memory.max":     {Data: []byte("max\n")},
		"sys/fs/cgroup/memory.max":         {Data: []byte("4096\n")},
	}
	at := time.Now()
	value := readContainerMemory(context.Background(), at, files)
	if value.Value == nil || value.Value.CurrentBytes != 1024 || value.Value.Unlimited ||
		value.Value.LimitBytes == nil || *value.Value.LimitBytes != 4096 {
		t.Fatalf("visible ancestor limits not applied: %+v", value)
	}
	files["sys/fs/cgroup/memory.max"].Data = []byte("max")
	value = readContainerMemory(context.Background(), at, files)
	if value.Value == nil || !value.Value.Unlimited || value.Value.LimitBytes != nil {
		t.Fatal("unlimited is not zero or unavailable")
	}
	files["sys/fs/cgroup/app/memory.max"].Data = []byte("secret-invalid-path")
	value = readContainerMemory(context.Background(), at, files)
	if value.Value != nil || value.Reason != "invalid" {
		t.Fatal("invalid cgroup value must fail explicitly")
	}
	files["proc/self/cgroup"].Data = []byte("5:memory:/app")
	if readContainerMemory(context.Background(), at, files).Reason != "unsupported" {
		t.Fatal("cgroup v1 must be explicitly unsupported")
	}
	files["proc/self/cgroup"].Data = []byte(strings.Repeat("x", 4097))
	if readContainerMemory(context.Background(), at, files).Reason != "limit" {
		t.Fatal("cgroup reads must be bounded")
	}
	for _, tc := range []struct{ group, mount, wantMount, wantRelative string }{
		{"0::/", "1 0 0:1 / /sys/fs/cgroup rw - cgroup2 cgroup rw", "sys/fs/cgroup", "."},
		{"0::/parent/app", "1 0 0:1 /parent /cg rw - cgroup2 cgroup rw", "cg", "app"},
		{"0::/app", `1 0 0:1 / /space\040name rw - cgroup2 cgroup rw`, "space name", "app"},
	} {
		mount, relative, err := cgroupLocation(tc.group, tc.mount)
		if err != nil || mount != tc.wantMount || relative != tc.wantRelative {
			t.Fatal("cgroup namespace mapping", mount, relative, err)
		}
	}
}

func TestSystemPostgreSQLObservationsAndAuthorization(t *testing.T) {
	f := newFixture(t)
	dir := storageFixture(t)
	monitor, err := NewSystemMonitor(f.ctx, f.service, dir, "test-build", time.Now().Add(-time.Minute),
		slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}

	defer monitor.Close()
	result, err := f.service.System(f.ctx, f.token, monitor)
	if err != nil {
		t.Fatal(err)
	}
	if result.Database.Value == nil || result.Database.Value.SizeBytes <= 0 || result.Database.Value.Connections < 1 ||
		result.Database.Value.ActiveConnections < 1 || result.Database.Value.SharedBuffersBytes <= 0 ||
		result.Memory.Value == nil || result.Memory.Value.TotalBytes == 0 || result.AppRSSBytes.Value == nil ||
		*result.AppRSSBytes.Value == 0 || result.Disk.Value == nil || result.Disk.Value.FreeBytes == 0 ||
		result.FilesBytes.Value == nil || *result.FilesBytes.Value != 4096 {
		t.Fatalf("actual observations missing: %+v", result)
	}
	if result.Versions.App != "test-build" || result.Versions.Go != runtime.Version() || result.UptimeSeconds < 60 ||
		result.Versions.Postgres.Value == nil || !strings.HasPrefix(*result.Versions.Postgres.Value, "18.") ||
		result.Versions.Migration.Value == nil || *result.Versions.Migration.Value != 7 {
		t.Fatal("versions/uptime not actual runtime/database values")
	}
	var shared, size int64
	if err := f.pool.QueryRow(f.ctx, `SELECT pg_size_bytes(current_setting('shared_buffers')), pg_database_size(current_database())`).Scan(&shared, &size); err != nil {
		t.Fatal(err)
	}
	if shared != result.Database.Value.SharedBuffersBytes || size != result.Database.Value.SizeBytes {
		t.Fatal("database units must be bytes from current database")
	}
	for _, state := range []struct {
		role, status string
		forced       bool
		want         string
	}{
		{"user", "active", false, "ERR_PERMISSION_DENIED"},
		{"admin", "active", true, "ERR_PASSWORD_CHANGE_REQUIRED"},
		{"admin", "disabled", false, "ERR_UNAUTHENTICATED"},
	} {
		_, token := f.account(state.role, state.status, state.forced)
		_, err := f.service.System(f.ctx, token, monitor)
		code(t, err, state.want)
	}
	f.exec(f.ctx, "UPDATE meta.users SET role='user' WHERE id=$1", f.admin)
	_, err = f.service.System(f.ctx, f.token, monitor)
	code(t, err, "ERR_PERMISSION_DENIED")
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{dir, "owned-fixture", f.token, f.phc, "password", "postgres://"} {
		if strings.Contains(string(payload), secret) {
			t.Fatal("metrics leaked a path or credential")
		}
	}
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(f.ctx) }()
	if _, err := tx.Exec(f.ctx, `INSERT INTO public.goose_db_version(version_id,is_applied) VALUES (999,true),(999,false)`); err != nil {
		t.Fatal(err)
	}
	level, err := f.service.queries.WithTx(tx).SystemMigration(f.ctx)
	if err != nil || level != 7 {
		t.Fatal("latest migration state must honor a reverted version", level, err)
	}
}

func TestSystemSourceFailuresAreExplicitAndSanitized(t *testing.T) {
	f := newFixture(t)
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(f.ctx); err != nil {
		t.Fatal(err)
	}
	broken := *f.service
	broken.queries = f.service.queries.WithTx(tx)
	var logs bytes.Buffer
	dir := filepath.Join(t.TempDir(), "private-missing-source")
	source := &systemSource{service: &broken, dataDir: dir, version: "test-build", started: time.Now(),
		logger: slog.New(slog.NewJSONHandler(&logs, nil))}
	result := source.collect(f.ctx)
	if result.Database.Value != nil || result.Database.Reason != "query" ||
		result.Versions.Postgres.Value != nil || result.Versions.Postgres.Reason != "query" ||
		result.Versions.Migration.Value != nil || result.Versions.Migration.Reason != "query" ||
		result.FilesBytes.Value != nil || result.FilesBytes.Reason != "notFound" ||
		result.Disk.Value != nil {
		t.Fatal("failed sources must not return success-shaped values")
	}
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{dir, "private-missing-source", "tx is closed", f.token, f.phc, "postgres://"} {
		if strings.Contains(string(payload), secret) || strings.Contains(logs.String(), secret) {
			t.Fatal("source errors leaked private details")
		}
	}
	if !strings.Contains(logs.String(), `"section":"database"`) {
		t.Fatal("failed collection must be logged with safe source context")
	}
}

func BenchmarkSystemHostCollection(b *testing.B) {
	source := &systemSource{dataDir: b.TempDir(), logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		source.host(context.Background(), &SystemSnapshot{CollectedAt: time.Now().UTC()})
	}
}

func BenchmarkSystemDirectoryScan(b *testing.B) {
	dir := storageFixture(b)
	for index := range 1000 {
		if err := os.WriteFile(filepath.Join(dir, "files", strconv.Itoa(index)), []byte("fixture"), 0o600); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := directorySize(context.Background(), dir, "files", scanBounds{entries: 100_000, depth: 64}); err != nil {
			b.Fatal(err)
		}
	}
}
