package admin

import (
	"context"
	"errors"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"pandatable/internal/apperr"
	"pandatable/internal/auth"
)

type Metric[T any] struct {
	Status      string    `json:"status"`
	Reason      string    `json:"reason,omitempty"`
	CollectedAt time.Time `json:"collectedAt"`
	Value       *T        `json:"value"`
}

func ready[T any](at time.Time, value T) Metric[T] {
	return Metric[T]{Status: "ready", CollectedAt: at, Value: &value}
}

func unavailable[T any](at time.Time, reason string) Metric[T] {
	return Metric[T]{Status: "unavailable", Reason: reason, CollectedAt: at}
}

type LoadValues struct {
	One     float64 `json:"one"`
	Five    float64 `json:"five"`
	Fifteen float64 `json:"fifteen"`
}

type MemoryValues struct {
	TotalBytes     uint64 `json:"totalBytes"`
	UsedBytes      uint64 `json:"usedBytes"`
	AvailableBytes uint64 `json:"availableBytes"`
}

type DiskValues struct {
	TotalBytes uint64 `json:"totalBytes"`
	UsedBytes  uint64 `json:"usedBytes"`
	FreeBytes  uint64 `json:"freeBytes"`
}

type ContainerMemory struct {
	CurrentBytes uint64  `json:"currentBytes"`
	LimitBytes   *uint64 `json:"limitBytes"`
	Unlimited    bool    `json:"unlimited"`
}

type DatabaseValues struct {
	SizeBytes          int64 `json:"sizeBytes"`
	Connections        int64 `json:"connections"`
	ActiveConnections  int64 `json:"activeConnections"`
	SharedBuffersBytes int64 `json:"sharedBuffersBytes"`
}

type SystemVersions struct {
	App       string         `json:"app"`
	Go        string         `json:"go"`
	Postgres  Metric[string] `json:"postgres"`
	Migration Metric[int64]  `json:"migration"`
}

type SystemSnapshot struct {
	CollectedAt   time.Time               `json:"collectedAt"`
	CPUPercent    Metric[float64]         `json:"cpuPercent"`
	Load          Metric[LoadValues]      `json:"load"`
	Memory        Metric[MemoryValues]    `json:"memory"`
	AppRSSBytes   Metric[uint64]          `json:"appRssBytes"`
	Container     Metric[ContainerMemory] `json:"container"`
	Disk          Metric[DiskValues]      `json:"disk"`
	Database      Metric[DatabaseValues]  `json:"database"`
	FilesBytes    Metric[uint64]          `json:"filesBytes"`
	BackupsBytes  Metric[uint64]          `json:"backupsBytes"`
	Versions      SystemVersions          `json:"versions"`
	UptimeSeconds float64                 `json:"uptimeSeconds"`
}

type systemFlight struct {
	done   chan struct{}
	result SystemSnapshot
	err    error
}

// SystemMonitor owns at most one collector, including while all its callers cancel.
type SystemMonitor struct {
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	flight  *systemFlight
	cached  *SystemSnapshot
	expires time.Time
	now     func() time.Time
	collect func(context.Context) SystemSnapshot
}

func NewSystemMonitor(ctx context.Context, service *Service, dataDir, version string, started time.Time, logger *slog.Logger) (*SystemMonitor, error) {
	if ctx == nil || service == nil || dataDir == "" || version == "" || started.IsZero() || logger == nil {
		return nil, errors.New("system monitor requires lifecycle, service, storage, version, start time and logger")
	}
	lifetime, cancel := context.WithCancel(ctx)
	source := &systemSource{service: service, dataDir: dataDir, version: version, started: started, logger: logger}
	return &SystemMonitor{ctx: lifetime, cancel: cancel, now: time.Now, collect: source.collect}, nil
}

func (monitor *SystemMonitor) Close() {
	monitor.cancel()
	monitor.mu.Lock()
	flight := monitor.flight
	monitor.mu.Unlock()
	if flight != nil {
		<-flight.done
	}
}

func (monitor *SystemMonitor) Snapshot(ctx context.Context) (SystemSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return SystemSnapshot{}, err
	}
	monitor.mu.Lock()
	if monitor.ctx.Err() != nil {
		monitor.mu.Unlock()
		return SystemSnapshot{}, apperr.New(503, "ERR_SYSTEM_UNAVAILABLE", "System collector stopped")
	}
	if monitor.cached != nil && monitor.now().Before(monitor.expires) {
		result := *monitor.cached
		monitor.mu.Unlock()
		return result, nil
	}
	flight := monitor.flight
	if flight == nil {
		flight = &systemFlight{done: make(chan struct{})}
		monitor.flight = flight
		go monitor.run(flight)
	}
	monitor.mu.Unlock()
	select {
	case <-ctx.Done():
		return SystemSnapshot{}, ctx.Err()
	case <-flight.done:
		return flight.result, flight.err
	}
}

func (monitor *SystemMonitor) run(flight *systemFlight) {
	ctx, cancel := context.WithTimeout(monitor.ctx, 4*time.Second)
	defer cancel()
	result := monitor.collect(ctx)
	monitor.mu.Lock()
	defer monitor.mu.Unlock()
	if monitor.ctx.Err() != nil {
		flight.err = apperr.New(503, "ERR_SYSTEM_UNAVAILABLE", "System collector stopped")
	} else {
		flight.result = result
		monitor.cached = &result
		monitor.expires = monitor.now().Add(5 * time.Second)
	}
	monitor.flight = nil
	close(flight.done)
}

// Authorization is deliberately outside the metrics cache.
func (service *Service) System(ctx context.Context, token string, monitor *SystemMonitor) (SystemSnapshot, error) {
	hash, err := auth.SessionHash(token)
	if err != nil {
		return SystemSnapshot{}, err
	}
	if _, err := authorize(ctx, service.queries, hash); err != nil {
		return SystemSnapshot{}, err
	}
	if monitor == nil {
		return SystemSnapshot{}, apperr.New(503, "ERR_SYSTEM_UNAVAILABLE", "System collector unavailable")
	}
	return monitor.Snapshot(ctx)
}

type systemSource struct {
	service     *Service
	dataDir     string
	version     string
	started     time.Time
	logger      *slog.Logger
	previous    *cpuSample
	directories directoryCache
}

func (source *systemSource) collect(ctx context.Context) SystemSnapshot {
	at := time.Now().UTC()
	result := SystemSnapshot{CollectedAt: at, UptimeSeconds: time.Since(source.started).Seconds(),
		Versions: SystemVersions{App: source.version, Go: runtime.Version()}}
	source.host(ctx, &result)
	row, err := source.service.queries.SystemDatabase(ctx)
	if err != nil {
		result.Database = unavailable[DatabaseValues](at, "query")
		result.Versions.Postgres = unavailable[string](at, "query")
		source.logger.WarnContext(ctx, "system metric unavailable", "section", "database", "reason", "query")
	} else {
		result.Database = ready(at, DatabaseValues{SizeBytes: row.SizeBytes, Connections: row.Connections,
			ActiveConnections: row.ActiveConnections, SharedBuffersBytes: row.SharedBuffersBytes})
		result.Versions.Postgres = ready(at, row.ServerVersion)
	}
	level, err := source.service.queries.SystemMigration(ctx)
	if err != nil {
		result.Versions.Migration = unavailable[int64](at, "query")
		source.logger.WarnContext(ctx, "system metric unavailable", "section", "migration", "reason", "query")
	} else {
		result.Versions.Migration = ready(at, level)
	}
	result.FilesBytes, result.BackupsBytes = source.directories.sizes(ctx, source.dataDir)
	for _, item := range []struct {
		name   string
		reason string
	}{{"files", result.FilesBytes.Reason}, {"backups", result.BackupsBytes.Reason}} {
		if item.reason != "" {
			source.logger.WarnContext(ctx, "system metric unavailable", "section", item.name, "reason", item.reason)
		}
	}
	return result
}
