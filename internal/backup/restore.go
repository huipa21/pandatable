package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/backup/backupsql"
	"pandatable/internal/db"
)

type RestoreOptions struct {
	Pool        *db.Pool
	DatabaseURL string
	DataDir     string
	AppVersion  string
	Logger      *slog.Logger
}

// Restore requires the application to be stopped and the archive to be trusted.
func Restore(ctx context.Context, archivePath string, options RestoreOptions) (result error) {
	if ctx == nil || options.Pool == nil || options.Pool.Pool == nil || options.DatabaseURL == "" ||
		options.DataDir == "" || options.AppVersion == "" || options.Logger == nil || archivePath == "" {
		return errors.New("restore requires a database, storage, archive, version and logger")
	}
	filesDir, err := storageDirectory(options.DataDir, "files")
	if err != nil {
		return err
	}
	backupsDir, err := storageDirectory(options.DataDir, "backups")
	if err != nil {
		return err
	}
	archiveAbs, err := filepath.Abs(archivePath)
	if err != nil {
		return fmt.Errorf("resolve backup archive: %w", err)
	}
	filesAbs, err := filepath.Abs(filesDir)
	if err != nil {
		return fmt.Errorf("resolve attachment directory: %w", err)
	}
	if relative, err := filepath.Rel(filesAbs, archiveAbs); err != nil || filepath.IsLocal(relative) {
		return errors.New("backup archive must be outside the attachment directory")
	}
	conn, locked, err := acquireBackupLock(ctx, options.Pool)
	if err != nil {
		return err
	}
	if !locked {
		return errors.New("a backup or restore is already running")
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		result = errors.Join(result, conn.Close(closeCtx))
	}()
	var targetPostgres string
	if err := conn.QueryRow(ctx, "SELECT current_setting('server_version')").Scan(&targetPostgres); err != nil {
		return fmt.Errorf("read target PostgreSQL version: %w", err)
	}
	stage, err := os.MkdirTemp(options.DataDir, ".restore-")
	if err != nil {
		return fmt.Errorf("create restore staging directory: %w", err)
	}
	databaseReplaced := false
	defer func() {
		if databaseReplaced && result != nil {
			result = fmt.Errorf("restore did not finish; keep the application stopped and retry using the archive; recovery files: %s: %w", stage, result)
			return
		}
		result = errors.Join(result, os.RemoveAll(stage))
	}()
	metadata, err := stageArchive(ctx, archiveAbs, stage, options.AppVersion, targetPostgres)
	if err != nil {
		return fmt.Errorf("validate backup: %w", err)
	}
	dump := filepath.Join(stage, "db.dump")
	list := exec.CommandContext(ctx, "pg_restore", "--list", dump)
	if err := runPG(list, "validate pg_restore archive"); err != nil {
		return err
	}
	existing, err := localBackupRecords(ctx, conn, backupsDir, options.Logger)
	if err != nil {
		return err
	}
	databaseReplaced, err = restoreDatabase(ctx, options.DatabaseURL, dump)
	if err != nil {
		return err
	}
	previous := filepath.Join(stage, "previous-files")
	if err := os.Rename(filesDir, previous); err != nil {
		return fmt.Errorf("stage existing attachment directory: %w", err)
	}
	if err := os.Rename(filepath.Join(stage, "files"), filesDir); err != nil {
		return errors.Join(fmt.Errorf("install restored attachments: %w", err), os.Rename(previous, filesDir))
	}
	options.Pool.Reset()
	if err := db.RunMigrations(ctx, options.Pool); err != nil {
		return fmt.Errorf("migrate restored database: %w", err)
	}
	if err := restoreBackupCatalog(ctx, options.Pool, existing, backupsDir, archiveAbs, metadata); err != nil {
		return fmt.Errorf("restore local backup catalog: %w", err)
	}
	if err := os.RemoveAll(previous); err != nil {
		return fmt.Errorf("remove previous attachments: %w", err)
	}
	return nil
}

const restorePreamble = `BEGIN;
SET LOCAL idle_in_transaction_session_timeout = 0;
SET LOCAL statement_timeout = 0;
SET LOCAL lock_timeout = '10s';
DO $pandatable$
DECLARE item record;
BEGIN
  FOR item IN SELECT nspname FROM pg_catalog.pg_namespace
    WHERE nspname NOT LIKE 'pg\_%' ESCAPE '\' AND nspname <> 'information_schema'
  LOOP
    EXECUTE format('DROP SCHEMA %I CASCADE', item.nspname);
  END LOOP;
END
$pandatable$;
CREATE SCHEMA public;
`

func restoreDatabase(ctx context.Context, databaseURL, dump string) (bool, error) {
	psql, err := pgCommand(ctx, "psql", databaseURL, "--no-psqlrc", "--set=ON_ERROR_STOP=1", "--file=-")
	if err != nil {
		return false, err
	}
	reader, writer := io.Pipe()
	psql.Stdin = reader
	type producerResult struct {
		commitSent bool
		err        error
	}
	producer := make(chan producerResult, 1)
	go func() {
		commitSent := false
		_, err := io.WriteString(writer, restorePreamble)
		if err == nil {
			restore := exec.CommandContext(ctx, "pg_restore", "--clean", "--if-exists", "--no-owner", "--no-acl", "--file=-", dump)
			restore.Stdout = writer
			err = runPG(restore, "pg_restore")
		}
		if err == nil {
			_, err = io.WriteString(writer, "\nCOMMIT;\n")
			commitSent = err == nil
		}
		_ = writer.CloseWithError(err)
		producer <- producerResult{commitSent: commitSent, err: err}
	}()
	psqlErr := runPG(psql, "restore database transaction")
	_ = reader.CloseWithError(psqlErr)
	produced := <-producer
	if err := errors.Join(psqlErr, produced.err); err != nil {
		if produced.commitSent {
			return true, fmt.Errorf("restore database commit outcome is uncertain: %w", err)
		}
		return false, fmt.Errorf("restore database transaction failed before commit: %w", err)
	}
	return true, nil
}

func localBackupRecords(ctx context.Context, conn *pgx.Conn, dir string, logger *slog.Logger) ([]backupsql.ListBackupsRow, error) {
	var exists bool
	if err := conn.QueryRow(ctx, "SELECT to_regclass('meta.backups') IS NOT NULL").Scan(&exists); err != nil {
		return nil, fmt.Errorf("inspect existing backup catalog: %w", err)
	}
	if !exists {
		return nil, nil
	}
	rows, err := backupsql.New(conn).ListBackups(ctx)
	if err != nil {
		return nil, fmt.Errorf("read existing backup catalog: %w", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("open existing backup directory: %w", err)
	}
	defer func() { _ = root.Close() }()
	var retained []backupsql.ListBackupsRow
	for _, row := range rows {
		if row.Status != "done" {
			continue
		}
		if !backupNamePattern.MatchString(row.FileName) || !row.SizeBytes.Valid {
			return nil, errors.New("existing backup catalog contains invalid metadata")
		}
		file, info, err := openRegular(root, row.FileName)
		if err != nil || info.Size() != row.SizeBytes.Int64 {
			if file != nil {
				_ = file.Close()
			}
			logger.Warn("missing or changed local backup omitted from restored catalog", "backup_id", row.ID.String())
			continue
		}
		if err := file.Close(); err != nil {
			return nil, fmt.Errorf("close existing backup archive: %w", err)
		}
		retained = append(retained, row)
	}
	return retained, nil
}

func restoreBackupCatalog(ctx context.Context, pool *db.Pool, rows []backupsql.ListBackupsRow,
	backupsDir, archivePath string, metadata Metadata) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	queries := backupsql.New(tx)
	for _, row := range rows {
		if err := queries.RestoreBackupRecord(ctx, backupsql.RestoreBackupRecordParams{
			ID: row.ID, FileName: row.FileName, SizeBytes: row.SizeBytes, ID_2: row.CreatedBy,
			CreatedAt: row.CreatedAt, FinishedAt: row.FinishedAt,
		}); err != nil {
			return err
		}
	}
	backupAbs, err := filepath.Abs(backupsDir)
	if err != nil {
		return err
	}
	archiveDir, archiveName := filepath.Split(archivePath)
	archiveDir = filepath.Clean(archiveDir)
	if archiveDir == filepath.Clean(backupAbs) && backupNamePattern.MatchString(archiveName) {
		info, err := os.Lstat(archivePath)
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("restored archive is no longer a regular file")
		}
		if err := queries.RecordRestoredArchive(ctx, backupsql.RecordRestoredArchiveParams{
			FileName: archiveName, SizeBytes: pgtype.Int8{Int64: info.Size(), Valid: true},
			CreatedAt: pgtype.Timestamptz{Time: metadata.CreatedAt, Valid: true},
		}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
