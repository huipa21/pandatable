package backup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shirou/gopsutil/v4/disk"

	"pandatable/internal/acl"
	"pandatable/internal/apperr"
	"pandatable/internal/backup/backupsql"
	"pandatable/internal/db"
)

const (
	backupLockKey  int64 = 0x5054424B
	cleanupLockKey int64 = 0x50544243
)

var backupNamePattern = regexp.MustCompile(`^pandatable-[0-9]{8}-[0-9]{6}\.zip$`)
var tombstonePattern = regexp.MustCompile(`^\.(pandatable-[0-9]{8}-[0-9]{6}\.zip)\.deleting-([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)

type Progress struct {
	Stage   string `json:"stage"`
	Percent int    `json:"percent"`
}

type Record struct {
	ID            string     `json:"id"`
	FileName      string     `json:"fileName"`
	SizeBytes     *int64     `json:"sizeBytes"`
	Status        string     `json:"status"`
	ErrorCode     string     `json:"errorCode,omitempty"`
	CreatedBy     *string    `json:"createdBy"`
	CreatedByName string     `json:"createdByName"`
	CreatedAt     time.Time  `json:"createdAt"`
	FinishedAt    *time.Time `json:"finishedAt"`
	Progress      *Progress  `json:"progress,omitempty"`
}

type Listing struct {
	Items     []Record `json:"items"`
	FreeBytes uint64   `json:"freeBytes"`
}

type activeJob struct {
	id       string
	progress Progress
}

type Manager struct {
	pool        *db.Pool
	databaseURL string
	version     string
	filesDir    string
	backupsDir  string
	tmpDir      string
	logger      *slog.Logger
	freeSpace   func(context.Context, string) (uint64, error)
	dump        func(context.Context, string, string) error
	finish      func(context.Context, *pgx.Conn, backupsql.FinishBackupParams) (int64, error)

	ctx          context.Context
	cancel       context.CancelFunc
	mu           sync.Mutex
	fileMu       sync.Mutex
	active       *activeJob
	reserving    bool
	closed       bool
	jobs         sync.WaitGroup
	shutdownOnce sync.Once
	shutdownDone chan struct{}
}

func New(ctx context.Context, pool *db.Pool, databaseURL, dataDir, version string, logger *slog.Logger) (*Manager, error) {
	if ctx == nil || pool == nil || pool.Pool == nil || databaseURL == "" || dataDir == "" || version == "" || logger == nil {
		return nil, errors.New("backup manager requires a database, storage, version and logger")
	}
	dirs := make([]string, 0, 3)
	for _, name := range []string{"files", "backups", "tmp"} {
		dir, err := storageDirectory(dataDir, name)
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, dir)
	}
	lifetime, cancel := context.WithCancel(context.Background())
	manager := &Manager{
		pool: pool, databaseURL: databaseURL, version: version, filesDir: dirs[0], backupsDir: dirs[1], tmpDir: dirs[2],
		logger: logger, ctx: lifetime, cancel: cancel, shutdownDone: make(chan struct{}),
		freeSpace: func(ctx context.Context, dir string) (uint64, error) {
			usage, err := disk.UsageWithContext(ctx, dir)
			if err != nil {
				return 0, err
			}
			return usage.Free, nil
		},
	}
	manager.dump = manager.dumpSnapshot
	manager.finish = func(ctx context.Context, conn *pgx.Conn, params backupsql.FinishBackupParams) (int64, error) {
		return backupsql.New(conn).FinishBackup(ctx, params)
	}
	conn, locked, err := manager.lock(ctx)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("check interrupted backups: %w", err)
	}
	if locked {
		err = manager.recover(ctx, conn)
		closeErr := conn.Close(ctx)
		if err = errors.Join(err, closeErr); err != nil {
			cancel()
			return nil, fmt.Errorf("recover interrupted backups: %w", err)
		}
	}
	return manager, nil
}

func (manager *Manager) lock(ctx context.Context) (*pgx.Conn, bool, error) {
	return acquireBackupLock(ctx, manager.pool)
}

func acquireBackupLock(ctx context.Context, pool *db.Pool) (*pgx.Conn, bool, error) {
	conn, err := pgx.ConnectConfig(ctx, pool.Config().ConnConfig.Copy())
	if err != nil {
		return nil, false, errors.New("connect to PostgreSQL for backup lock failed")
	}
	var locked bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", backupLockKey).Scan(&locked); err != nil {
		_ = conn.Close(context.Background())
		return nil, false, fmt.Errorf("acquire backup lock: %w", err)
	}
	if !locked {
		if err := conn.Close(ctx); err != nil {
			return nil, false, fmt.Errorf("close backup lock connection: %w", err)
		}
		return nil, false, nil
	}
	return conn, true, nil
}

func (manager *Manager) recover(ctx context.Context, conn *pgx.Conn) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin backup recovery: %w", err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", cleanupLockKey); err != nil {
		return fmt.Errorf("lock backup recovery: %w", err)
	}
	queries := backupsql.New(tx)
	rows, err := queries.ListBackups(ctx)
	if err != nil {
		return fmt.Errorf("list interrupted backups: %w", err)
	}
	if err := manager.recoverTombstones(rows); err != nil {
		return err
	}
	for _, row := range rows {
		if row.Status != "running" && row.Status != "failed" {
			continue
		}
		if !backupNamePattern.MatchString(row.FileName) {
			return errors.New("interrupted backup has an invalid filename")
		}
		for _, name := range []string{"." + row.FileName + ".partial", row.FileName} {
			if err := removeIfPresent(filepath.Join(manager.backupsDir, name)); err != nil {
				return fmt.Errorf("remove interrupted backup: %w", err)
			}
		}
		if row.Status == "running" {
			updated, err := queries.FailBackup(ctx, backupsql.FailBackupParams{
				ID: row.ID, Error: pgtype.Text{String: "ERR_BACKUP_INTERRUPTED", Valid: true},
			})
			if err != nil || updated != 1 {
				return fmt.Errorf("mark interrupted backup failed: %w", errors.Join(err, nonOne(updated)))
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit backup recovery: %w", err)
	}
	return nil
}

func (manager *Manager) recoverTombstones(rows []backupsql.ListBackupsRow) error {
	byID := make(map[string]backupsql.ListBackupsRow, len(rows))
	for _, row := range rows {
		byID[row.ID.String()] = row
	}
	entries, err := os.ReadDir(manager.backupsDir)
	if err != nil {
		return fmt.Errorf("inspect backup deletion recovery: %w", err)
	}
	for _, entry := range entries {
		match := tombstonePattern.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		tombstone := filepath.Join(manager.backupsDir, entry.Name())
		info, err := os.Lstat(tombstone)
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("invalid backup deletion recovery file: %s", entry.Name())
		}
		row, exists := byID[match[2]]
		if exists && row.FileName != match[1] {
			return fmt.Errorf("backup deletion recovery conflicts with catalog: %s", entry.Name())
		}
		if !exists || row.Status == "failed" {
			if err := removeIfPresent(tombstone); err != nil {
				return fmt.Errorf("remove completed backup deletion: %w", err)
			}
			continue
		}
		if row.Status != "done" {
			return fmt.Errorf("backup deletion recovery conflicts with catalog: %s", entry.Name())
		}
		final := filepath.Join(manager.backupsDir, row.FileName)
		if err := os.Link(tombstone, final); err != nil {
			existing, statErr := os.Lstat(final)
			if !errors.Is(err, os.ErrExist) || statErr != nil || !existing.Mode().IsRegular() || !os.SameFile(info, existing) {
				return fmt.Errorf("restore interrupted backup deletion: %w", err)
			}
		}
		if err := removeIfPresent(tombstone); err != nil {
			return fmt.Errorf("remove recovered backup deletion link: %w", err)
		}
	}
	return nil
}

func (manager *Manager) recoverDeletions(ctx context.Context) error {
	tx, err := manager.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin backup deletion recovery: %w", err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", cleanupLockKey); err != nil {
		return fmt.Errorf("lock backup deletion recovery: %w", err)
	}
	rows, err := backupsql.New(tx).ListBackups(ctx)
	if err != nil {
		return fmt.Errorf("list backup deletion recovery: %w", err)
	}
	if err := manager.recoverTombstones(rows); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func nonOne(count int64) error {
	if count != 1 {
		return fmt.Errorf("expected one backup record, got %d", count)
	}
	return nil
}

func removeIfPresent(name string) error {
	if err := os.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (manager *Manager) Close(ctx context.Context) error {
	manager.shutdownOnce.Do(func() {
		manager.mu.Lock()
		manager.closed = true
		manager.cancel()
		manager.mu.Unlock()
		go func() {
			manager.jobs.Wait()
			close(manager.shutdownDone)
		}()
	})
	select {
	case <-manager.shutdownDone:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for backup shutdown: %w", ctx.Err())
	}
}

func (manager *Manager) Start(ctx context.Context, userID string) (record Record, result error) {
	user, ok := acl.ParseUUID(userID)
	if !ok {
		return Record{}, apperr.New(400, "ERR_VALIDATION", "Invalid backup creator")
	}
	manager.mu.Lock()
	if manager.closed || manager.reserving || manager.active != nil {
		manager.mu.Unlock()
		return Record{}, apperr.New(409, "ERR_BACKUP_BUSY", "A backup is already running")
	}
	manager.reserving = true
	manager.jobs.Add(1)
	manager.mu.Unlock()
	handedOff := false
	defer func() {
		if !handedOff {
			manager.mu.Lock()
			manager.reserving = false
			manager.mu.Unlock()
			manager.jobs.Done()
		}
	}()
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(manager.ctx, cancel)
	defer func() { stop(); cancel() }()
	conn, locked, err := manager.lock(ctx)
	if err != nil {
		return Record{}, err
	}
	if !locked {
		return Record{}, apperr.New(409, "ERR_BACKUP_BUSY", "A backup is already running")
	}
	defer func() {
		if !handedOff {
			closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer closeCancel()
			result = errors.Join(result, conn.Close(closeCtx))
		}
	}()
	if err := manager.recover(ctx, conn); err != nil {
		return Record{}, err
	}
	queries := backupsql.New(conn)
	environment, err := queries.BackupEnvironment(ctx)
	if err != nil {
		return Record{}, fmt.Errorf("read backup database size: %w", err)
	}
	filesBytes, _, err := fileSize(ctx, manager.filesDir)
	if err != nil {
		return Record{}, fmt.Errorf("measure backup attachments: %w", err)
	}
	free, err := manager.freeSpace(ctx, manager.backupsDir)
	if err != nil {
		return Record{}, fmt.Errorf("measure available backup disk space: %w", err)
	}
	if environment.DatabaseBytes < 0 || !hasBackupSpace(free, uint64(environment.DatabaseBytes), filesBytes) {
		return Record{}, apperr.New(409, "ERR_BACKUP_DISK_SPACE", "Insufficient disk space for backup")
	}
	location, err := time.LoadLocation(environment.Timezone)
	if err != nil {
		return Record{}, fmt.Errorf("load backup timezone: %w", err)
	}
	name := "pandatable-" + time.Now().In(location).Format("20060102-150405") + ".zip"
	for _, path := range []string{filepath.Join(manager.backupsDir, name), filepath.Join(manager.backupsDir, "."+name+".partial")} {
		if _, err := os.Lstat(path); err == nil {
			return Record{}, apperr.New(409, "ERR_BACKUP_NAME_CONFLICT", "A backup already exists for this second")
		} else if !errors.Is(err, os.ErrNotExist) {
			return Record{}, fmt.Errorf("inspect backup destination: %w", err)
		}
	}
	created, err := queries.CreateBackup(ctx, backupsql.CreateBackupParams{FileName: name, CreatedBy: user})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Record{}, apperr.New(409, "ERR_BACKUP_NAME_CONFLICT", "A backup already exists for this second")
		}
		return Record{}, fmt.Errorf("create backup record: %w", err)
	}
	record = Record{ID: created.ID.String(), FileName: name, Status: "running",
		CreatedBy: &userID, CreatedAt: created.CreatedAt.Time.UTC(), Progress: &Progress{Stage: "dumping", Percent: 5}}
	manager.mu.Lock()
	manager.active = &activeJob{id: record.ID, progress: *record.Progress}
	manager.reserving = false
	manager.mu.Unlock()
	handedOff = true
	go manager.run(conn, created.ID, record)
	return record, nil
}

func hasBackupSpace(free, database, files uint64) bool {
	if math.MaxUint64-database < files {
		return false
	}
	total := database + files
	return total <= math.MaxUint64/2 && free >= total*2
}

func (manager *Manager) setProgress(id, stage string, percent int) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.active != nil && manager.active.id == id {
		manager.active.progress = Progress{Stage: stage, Percent: percent}
	}
}

func (manager *Manager) run(conn *pgx.Conn, id pgtype.UUID, record Record) {
	defer manager.jobs.Done()
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := conn.Close(closeCtx); err != nil {
			manager.logger.Error("backup lock connection cleanup failed", "backup_id", record.ID)
		}
		manager.mu.Lock()
		manager.active = nil
		manager.mu.Unlock()
	}()
	partial := filepath.Join(manager.backupsDir, "."+record.FileName+".partial")
	final := filepath.Join(manager.backupsDir, record.FileName)
	published := false
	err := runLowPriority(func() error {
		return manager.create(manager.ctx, conn, id, record, partial, final, &published)
	})
	if err == nil {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	queries := backupsql.New(manager.pool)
	updated, updateErr := queries.FailBackup(cleanupCtx, backupsql.FailBackupParams{
		ID: id, Error: pgtype.Text{String: "ERR_BACKUP_FAILED", Valid: true},
	})
	cleanupErr := removeIfPresent(partial)
	if published && updateErr == nil && updated == 1 {
		cleanupErr = errors.Join(cleanupErr, removeIfPresent(final))
	}
	if published && updateErr == nil && updated == 0 {
		row, readErr := queries.GetBackup(cleanupCtx, id)
		if readErr == nil && row.Status == "done" {
			manager.logger.Warn("backup completed despite a lost completion response", "backup_id", record.ID)
			if cleanupErr != nil {
				manager.logger.Error("backup partial cleanup failed", "backup_id", record.ID)
			}
			return
		}
	}
	if updateErr != nil || updated != 1 || cleanupErr != nil {
		manager.logger.Error("backup failure cleanup failed", "backup_id", record.ID)
	}
	manager.logger.Error("backup job failed", "backup_id", record.ID, "stage", manager.stage(record.ID),
		"error", redactError(err, manager.databaseURL))
}

func (manager *Manager) stage(id string) string {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.active != nil && manager.active.id == id {
		return manager.active.progress.Stage
	}
	return "unknown"
}

func (manager *Manager) create(ctx context.Context, conn *pgx.Conn, id pgtype.UUID, record Record,
	partial, final string, published *bool) (result error) {
	temp, err := os.CreateTemp(manager.tmpDir, "backup-*.dump")
	if err != nil {
		return fmt.Errorf("create temporary database dump: %w", err)
	}
	dumpPath := temp.Name()
	if err := temp.Close(); err != nil {
		_ = removeIfPresent(dumpPath)
		return fmt.Errorf("close temporary database dump: %w", err)
	}
	defer func() {
		if dumpPath != "" {
			result = errors.Join(result, removeIfPresent(dumpPath))
		}
	}()
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return fmt.Errorf("begin backup snapshot: %w", err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if _, err := tx.Exec(ctx, "SET LOCAL idle_in_transaction_session_timeout = 0"); err != nil {
		return fmt.Errorf("configure backup snapshot idle timeout: %w", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL statement_timeout = 0"); err != nil {
		return fmt.Errorf("configure backup snapshot statement timeout: %w", err)
	}
	queries := backupsql.New(tx)
	environment, err := queries.BackupEnvironment(ctx)
	if err != nil {
		return fmt.Errorf("read backup snapshot environment: %w", err)
	}
	migration, err := queries.BackupMigration(ctx)
	if err != nil {
		return fmt.Errorf("read backup snapshot migration: %w", err)
	}
	var snapshot string
	if err := tx.QueryRow(ctx, "SELECT pg_export_snapshot()").Scan(&snapshot); err != nil {
		return fmt.Errorf("export backup snapshot: %w", err)
	}
	if err := manager.dump(ctx, dumpPath, snapshot); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("finish backup snapshot: %w", err)
	}
	manager.setProgress(record.ID, "archiving", 10)
	metadata := Metadata{AppVersion: manager.version, SchemaMigration: migration, CreatedAt: record.CreatedAt,
		PgVersion: environment.PgVersion}
	size, err := writeArchive(ctx, partial, dumpPath, manager.filesDir, metadata, func(percent int) {
		manager.setProgress(record.ID, "archiving", percent)
	})
	if err != nil {
		return err
	}
	if err := removeIfPresent(dumpPath); err != nil {
		return fmt.Errorf("remove temporary database dump: %w", err)
	}
	dumpPath = ""
	manager.setProgress(record.ID, "finalizing", 96)
	if err := os.Link(partial, final); err != nil {
		return fmt.Errorf("publish backup archive: %w", err)
	}
	*published = true
	if err := os.Remove(partial); err != nil {
		return fmt.Errorf("remove partial backup link: %w", err)
	}
	updated, err := manager.finish(ctx, conn, backupsql.FinishBackupParams{
		ID: id, SizeBytes: pgtype.Int8{Int64: size, Valid: true},
	})
	if err != nil || updated != 1 {
		return fmt.Errorf("finish backup record: %w", errors.Join(err, nonOne(updated)))
	}
	return nil
}

func (manager *Manager) dumpSnapshot(ctx context.Context, path, snapshot string) error {
	cmd, err := pgCommand(ctx, "pg_dump", manager.databaseURL, "--format=custom", "--no-owner",
		"--exclude-table-data=meta.backups", "--snapshot="+snapshot, "--file="+path)
	if err != nil {
		return err
	}
	return runPG(cmd, "pg_dump")
}

func recordFromRow(row backupsql.GetBackupRow) Record {
	record := Record{ID: row.ID.String(), FileName: row.FileName, Status: row.Status,
		CreatedByName: row.CreatedByName, CreatedAt: row.CreatedAt.Time.UTC()}
	if row.SizeBytes.Valid {
		size := row.SizeBytes.Int64
		record.SizeBytes = &size
	}
	if row.Error.Valid {
		record.ErrorCode = row.Error.String
	}
	if row.CreatedBy.Valid {
		user := row.CreatedBy.String()
		record.CreatedBy = &user
	}
	if row.FinishedAt.Valid {
		finished := row.FinishedAt.Time.UTC()
		record.FinishedAt = &finished
	}
	return record
}

func (manager *Manager) withProgress(record *Record) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if record.Status == "running" && manager.active != nil && manager.active.id == record.ID {
		progress := manager.active.progress
		record.Progress = &progress
	}
}

func (manager *Manager) List(ctx context.Context) (Listing, error) {
	rows, err := backupsql.New(manager.pool).ListBackups(ctx)
	if err != nil {
		return Listing{}, fmt.Errorf("list backups: %w", err)
	}
	free, err := manager.freeSpace(ctx, manager.backupsDir)
	if err != nil {
		return Listing{}, fmt.Errorf("measure available backup disk space: %w", err)
	}
	result := Listing{Items: make([]Record, 0, len(rows)), FreeBytes: free}
	for _, row := range rows {
		record := recordFromRow(backupsql.GetBackupRow(row))
		manager.withProgress(&record)
		result.Items = append(result.Items, record)
	}
	return result, nil
}

func (manager *Manager) Get(ctx context.Context, id string) (Record, error) {
	uuid, ok := acl.ParseUUID(id)
	if !ok {
		return Record{}, acl.ErrNotFound()
	}
	row, err := backupsql.New(manager.pool).GetBackup(ctx, uuid)
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, acl.ErrNotFound()
	}
	if err != nil {
		return Record{}, fmt.Errorf("read backup: %w", err)
	}
	record := recordFromRow(row)
	manager.withProgress(&record)
	return record, nil
}

func (manager *Manager) Open(ctx context.Context, id string) (*os.File, Record, error) {
	record, err := manager.Get(ctx, id)
	if err != nil {
		return nil, Record{}, err
	}
	if record.Status != "done" || record.SizeBytes == nil || !backupNamePattern.MatchString(record.FileName) {
		return nil, Record{}, apperr.New(409, "ERR_BACKUP_UNAVAILABLE", "Backup is not available for download")
	}
	root, err := os.OpenRoot(manager.backupsDir)
	if err != nil {
		return nil, Record{}, fmt.Errorf("open backup directory: %w", err)
	}
	file, info, openErr := openRegular(root, record.FileName)
	closeErr := root.Close()
	if err := errors.Join(openErr, closeErr); err != nil {
		if file != nil {
			_ = file.Close()
		}
		return nil, Record{}, fmt.Errorf("open backup archive: %w", err)
	}
	if info.Size() != *record.SizeBytes {
		_ = file.Close()
		return nil, Record{}, apperr.New(409, "ERR_BACKUP_UNAVAILABLE", "Backup archive size does not match metadata")
	}
	return file, record, nil
}

func (manager *Manager) Delete(ctx context.Context, id string) (result error) {
	uuid, ok := acl.ParseUUID(id)
	if !ok {
		return acl.ErrNotFound()
	}
	manager.fileMu.Lock()
	defer manager.fileMu.Unlock()
	tx, err := manager.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin backup deletion: %w", err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", cleanupLockKey); err != nil {
		return fmt.Errorf("lock backup deletion: %w", err)
	}
	queries := backupsql.New(tx)
	rows, err := queries.ListBackups(ctx)
	if err != nil {
		return fmt.Errorf("list backup deletion recovery: %w", err)
	}
	if err := manager.recoverTombstones(rows); err != nil {
		return err
	}
	row, err := queries.GetBackup(ctx, uuid)
	if errors.Is(err, pgx.ErrNoRows) {
		return acl.ErrNotFound()
	}
	if err != nil {
		return fmt.Errorf("read backup for deletion: %w", err)
	}
	if row.Status == "running" {
		return apperr.New(409, "ERR_BACKUP_BUSY", "Backup is still running")
	}
	if !backupNamePattern.MatchString(row.FileName) {
		return errors.New("backup record has an invalid filename")
	}
	final := filepath.Join(manager.backupsDir, row.FileName)
	tombstone := filepath.Join(manager.backupsDir, "."+row.FileName+".deleting-"+uuid.String())
	renamed := false
	if err := os.Rename(final, tombstone); err == nil {
		renamed = true
	} else if !errors.Is(err, os.ErrNotExist) || row.Status == "done" {
		return fmt.Errorf("stage backup deletion: %w", err)
	}
	defer func() {
		if renamed {
			result = errors.Join(result, os.Rename(tombstone, final))
		}
	}()
	deleted, err := queries.DeleteBackup(ctx, uuid)
	if err != nil || deleted != 1 {
		return fmt.Errorf("delete backup record: %w", errors.Join(err, nonOne(deleted)))
	}
	if err := tx.Commit(ctx); err != nil {
		renamed = false
		recoveryCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		recoveryErr := manager.recoverDeletions(recoveryCtx)
		return fmt.Errorf("commit backup deletion outcome uncertain: %w", errors.Join(err, recoveryErr))
	}
	renamed = false
	if err := removeIfPresent(tombstone); err != nil {
		return fmt.Errorf("remove deleted backup archive: %w", err)
	}
	if err := removeIfPresent(filepath.Join(manager.backupsDir, "."+row.FileName+".partial")); err != nil {
		return fmt.Errorf("remove partial backup archive: %w", err)
	}
	return nil
}
