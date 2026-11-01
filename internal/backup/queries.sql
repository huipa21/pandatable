-- name: BackupEnvironment :one
SELECT pg_database_size(current_database())::bigint AS database_bytes,
       current_setting('server_version')::text AS pg_version,
       COALESCE((SELECT value #>> '{}' FROM meta.settings WHERE key = 'default_timezone'),
                'Asia/Shanghai')::text AS timezone;

-- name: BackupMigration :one
SELECT COALESCE(max(version_id), 0)::bigint AS migration
FROM (
    SELECT DISTINCT ON (version_id) version_id, is_applied
    FROM public.goose_db_version ORDER BY version_id, id DESC
) AS latest WHERE is_applied;

-- name: CreateBackup :one
INSERT INTO meta.backups (file_name, status, created_by)
VALUES ($1, 'running', $2)
RETURNING id, created_at;

-- name: FinishBackup :execrows
UPDATE meta.backups
SET status = 'done', size_bytes = $2, error = NULL, finished_at = now()
WHERE id = $1 AND status = 'running';

-- name: FailBackup :execrows
UPDATE meta.backups
SET status = 'failed', error = $2, finished_at = now()
WHERE id = $1 AND status = 'running';

-- name: ListBackups :many
SELECT b.id, b.file_name, b.size_bytes, b.status, b.error, b.created_by,
       COALESCE(u.display_name, '')::text AS created_by_name, b.created_at, b.finished_at
FROM meta.backups AS b
LEFT JOIN meta.users AS u ON u.id = b.created_by
ORDER BY b.created_at DESC, b.id DESC;

-- name: GetBackup :one
SELECT b.id, b.file_name, b.size_bytes, b.status, b.error, b.created_by,
       COALESCE(u.display_name, '')::text AS created_by_name, b.created_at, b.finished_at
FROM meta.backups AS b
LEFT JOIN meta.users AS u ON u.id = b.created_by
WHERE b.id = $1;

-- name: DeleteBackup :execrows
DELETE FROM meta.backups WHERE id = $1 AND status <> 'running';

-- name: RestoreBackupRecord :exec
INSERT INTO meta.backups (id, file_name, size_bytes, status, created_by, created_at, finished_at)
VALUES ($1, $2, $3, 'done', (SELECT u.id FROM meta.users AS u WHERE u.id = $4), $5, $6)
ON CONFLICT (file_name) DO NOTHING;

-- name: RecordRestoredArchive :exec
INSERT INTO meta.backups (file_name, size_bytes, status, created_at, finished_at)
VALUES ($1, $2, 'done', $3, now())
ON CONFLICT (file_name) DO NOTHING;
