-- Structure metadata for workspaces, books, tables, system fields and views
-- (T1.10, D-097..D-103). Authorization/lock queries live in internal/acl.
-- Physical DDL is never expressed here; see internal/ddl.

-- name: ReadDefaultCurrency :one
SELECT COALESCE(
    (SELECT value #>> '{}' FROM meta.settings WHERE key='default_currency'),
    'CNY'
);

-- name: InsertWorkspace :one
INSERT INTO meta.workspaces (name, icon, color, created_by)
VALUES ($1, $2, $3, $4)
RETURNING id, name, icon, color, created_at;

-- name: InsertWorkspaceOwnerGrant :exec
INSERT INTO meta.grants (resource_type, resource_id, principal_type, principal_id, level, granted_by)
VALUES ('workspace', sqlc.arg(workspace_id)::uuid, 'user', sqlc.arg(user_id)::uuid, 'owner', sqlc.arg(user_id)::uuid);

-- name: GetWorkspace :one
SELECT id, name, icon, color, created_at
FROM meta.workspaces
WHERE id = $1 AND deleted_at IS NULL;

-- name: UpdateWorkspace :one
UPDATE meta.workspaces SET name = $2, icon = $3, color = $4
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, name, icon, color, created_at;

-- name: TrashWorkspace :one
UPDATE meta.workspaces SET deleted_at = now(), deleted_by = $2
WHERE id = $1 AND deleted_at IS NULL
RETURNING id;

-- name: MaxBookPosition :one
SELECT COALESCE(MAX(position), 0)::float8 AS position FROM meta.books WHERE workspace_id = $1;

-- name: ListLiveBookOrder :many
SELECT id, position FROM meta.books
WHERE workspace_id = $1 AND deleted_at IS NULL
ORDER BY position, id;

-- name: InsertBook :one
INSERT INTO meta.books (workspace_id, name, pg_schema, position, created_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, workspace_id, name, position, created_at;

-- name: GetBook :one
SELECT id, workspace_id, name, pg_schema, position, created_at
FROM meta.books
WHERE id = $1 AND deleted_at IS NULL;

-- name: RenameBook :one
UPDATE meta.books SET name = $2
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, workspace_id, name, position, created_at;

-- name: SetBookPosition :one
UPDATE meta.books SET position = $2
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, workspace_id, name, position, created_at;

-- name: RenumberBooks :exec
UPDATE meta.books SET position = array_position(sqlc.arg(ids)::uuid[], id)::float8 * 1024
WHERE id = ANY(sqlc.arg(ids)::uuid[]) AND deleted_at IS NULL;

-- name: TrashBook :one
UPDATE meta.books SET deleted_at = now(), deleted_by = $2
WHERE id = $1 AND deleted_at IS NULL
RETURNING id;

-- name: MaxTablePosition :one
SELECT COALESCE(MAX(position), 0)::float8 AS position FROM meta.tables WHERE book_id = $1;

-- name: ListLiveTableOrder :many
SELECT id, position FROM meta.tables
WHERE book_id = $1 AND deleted_at IS NULL
ORDER BY position, id;

-- name: InsertTable :one
INSERT INTO meta.tables (book_id, name, pg_name, position, created_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: SetTableDisplayField :one
UPDATE meta.tables SET display_field_id = $2
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, book_id, name, display_field_id, position, schema_version, created_at;

-- name: GetTable :one
SELECT id, book_id, name, display_field_id, position, schema_version, created_at
FROM meta.tables
WHERE id = $1 AND deleted_at IS NULL;

-- name: RenameTable :one
UPDATE meta.tables SET name = $2
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, book_id, name, display_field_id, position, schema_version, created_at;

-- name: SetTablePosition :one
UPDATE meta.tables SET position = $2
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, book_id, name, display_field_id, position, schema_version, created_at;

-- name: RenumberTables :exec
UPDATE meta.tables SET position = array_position(sqlc.arg(ids)::uuid[], id)::float8 * 1024
WHERE id = ANY(sqlc.arg(ids)::uuid[]) AND deleted_at IS NULL;

-- name: TrashTable :one
UPDATE meta.tables SET deleted_at = now(), deleted_by = $2
WHERE id = $1 AND deleted_at IS NULL
RETURNING id;

-- name: InsertField :one
INSERT INTO meta.fields (table_id, name, ui_type, pg_column, pg_type, is_system, is_required, position)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id;

-- name: ListTableFields :many
SELECT id, name, description, ui_type, options, is_system, is_required, is_unique, default_json, position, version
FROM meta.fields
WHERE table_id = $1 AND deleted_at IS NULL
ORDER BY position, id;

-- Field API (T1.14, D-123..D-129). Physical columns are changed only through
-- internal/ddl; data preflight/rewrites use validated identifiers in Go.

-- name: FieldTable :one
SELECT f.table_id
FROM meta.fields f
JOIN meta.tables t ON t.id = f.table_id AND t.deleted_at IS NULL
WHERE f.id = $1 AND f.deleted_at IS NULL;

-- name: LockFieldTable :one
SELECT t.id, t.pg_name, b.pg_schema, t.display_field_id, t.schema_version
FROM meta.tables t
JOIN meta.books b ON b.id = t.book_id
WHERE t.id = $1 AND t.deleted_at IS NULL
FOR NO KEY UPDATE OF t;

-- name: GetField :one
SELECT id, table_id, name, description, ui_type, pg_column, options, is_system, is_required, is_unique,
       default_json, position, version
FROM meta.fields
WHERE id = $1 AND table_id = $2 AND deleted_at IS NULL;

-- name: MaxUserFieldPosition :one
SELECT COALESCE(MAX(position), 0)::float8 AS position
FROM meta.fields
WHERE table_id = $1 AND NOT is_system AND deleted_at IS NULL;

-- name: ListLiveUserFieldOrder :many
SELECT id, position FROM meta.fields
WHERE table_id = $1 AND NOT is_system AND deleted_at IS NULL
ORDER BY position, id;

-- name: InsertUserField :one
INSERT INTO meta.fields (table_id, name, description, ui_type, pg_column, pg_type, options,
                         is_required, is_unique, default_json, position)
VALUES (sqlc.arg(table_id), sqlc.arg(name), sqlc.arg(description), sqlc.arg(ui_type), sqlc.arg(pg_column),
        sqlc.arg(pg_type), sqlc.arg(options)::jsonb, sqlc.arg(is_required), sqlc.arg(is_unique),
        sqlc.narg(default_json)::jsonb, sqlc.arg(position))
RETURNING id;

-- name: UpdateField :one
UPDATE meta.fields
SET name = sqlc.arg(name), description = sqlc.arg(description), options = sqlc.arg(options)::jsonb,
    is_required = sqlc.arg(is_required), is_unique = sqlc.arg(is_unique),
    default_json = sqlc.narg(default_json)::jsonb, version = version + 1
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING version;

-- name: SetFieldPosition :exec
UPDATE meta.fields SET position = $2, version = version + 1
WHERE id = $1 AND deleted_at IS NULL;

-- name: RenumberFields :exec
UPDATE meta.fields SET position = array_position(sqlc.arg(ids)::uuid[], id)::float8 * 1024
WHERE id = ANY(sqlc.arg(ids)::uuid[]) AND deleted_at IS NULL;

-- name: TrashField :exec
UPDATE meta.fields SET deleted_at = now(), is_required = false, version = version + 1
WHERE id = $1 AND deleted_at IS NULL;

-- name: BumpSchemaVersion :exec
UPDATE meta.tables SET schema_version = schema_version + 1
WHERE id = $1 AND deleted_at IS NULL;

-- name: ValidFieldTimezone :one
SELECT EXISTS (SELECT 1 FROM pg_timezone_names
               WHERE name = $1 AND name NOT LIKE 'posix/%' AND name NOT LIKE 'right/%');

-- name: InsertDefaultGridView :one
INSERT INTO meta.views (table_id, name, type, scope, position, config, created_by)
VALUES ($1, $2, 'grid', 'shared', 1024, sqlc.arg(config)::jsonb, $3)
RETURNING id;

-- Shared views plus the caller's own personal views.

-- name: ListTableViews :many
SELECT id, name, type, scope, locked, position, version, config
FROM meta.views
WHERE table_id = sqlc.arg(table_id)::uuid AND deleted_at IS NULL
  AND (scope = 'shared' OR owner_id = sqlc.arg(user_id)::uuid)
ORDER BY position, id;

-- name: LockTableViewsForManagement :many
SELECT id, name, type, scope, owner_id, locked, position, version, config
FROM meta.views
WHERE table_id = sqlc.arg(table_id)::uuid AND deleted_at IS NULL
  AND (scope = 'shared' OR owner_id = sqlc.arg(user_id)::uuid)
ORDER BY position, id
FOR NO KEY UPDATE;

-- name: InsertManagedView :one
INSERT INTO meta.views (table_id, name, type, scope, owner_id, position, config, created_by)
VALUES (sqlc.arg(table_id)::uuid, sqlc.arg(name), sqlc.arg(type), sqlc.arg(scope),
        sqlc.narg(owner_id)::uuid, sqlc.arg(position), sqlc.arg(config)::jsonb,
        sqlc.arg(created_by)::uuid)
RETURNING id, name, type, scope, locked, position, version, config;

-- name: LockViewColumns :one
SELECT id, name, type, scope, owner_id, locked, position, version, config
FROM meta.views
WHERE id = $1 AND table_id = $2 AND deleted_at IS NULL
FOR NO KEY UPDATE;

-- name: SaveManagedView :exec
UPDATE meta.views
SET name = sqlc.arg(name), locked = sqlc.arg(locked), config = sqlc.arg(config)::jsonb,
    version = version + 1, updated_at = now()
WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: MoveManagedView :exec
UPDATE meta.views
SET position = sqlc.arg(position), version = version + 1, updated_at = now()
WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: DeleteManagedView :exec
UPDATE meta.views
SET deleted_at = now(), version = version + 1, updated_at = now()
WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- T1.15 row/query metadata. Physical names are validated in Go before quoting.

-- name: GetRowTable :one
SELECT t.id, t.pg_name, b.pg_schema, t.schema_version
FROM meta.tables t
JOIN meta.books b ON b.id = t.book_id AND b.deleted_at IS NULL
WHERE t.id = $1 AND t.deleted_at IS NULL;

-- name: LockRowTable :one
SELECT t.id, t.pg_name, b.pg_schema, t.schema_version
FROM meta.tables t
JOIN meta.books b ON b.id = t.book_id AND b.deleted_at IS NULL
WHERE t.id = $1 AND t.deleted_at IS NULL
FOR SHARE OF t;

-- name: GetViewForQuery :one
SELECT v.id, v.table_id, v.type, v.scope, v.owner_id, v.config,
       t.pg_name, b.pg_schema, t.schema_version
FROM meta.views v
JOIN meta.tables t ON t.id = v.table_id AND t.deleted_at IS NULL
JOIN meta.books b ON b.id = t.book_id AND b.deleted_at IS NULL
WHERE v.id = $1 AND v.deleted_at IS NULL;

-- name: LockViewForQuery :one
SELECT v.id, v.table_id, v.type, v.scope, v.owner_id, v.config,
       t.pg_name, b.pg_schema, t.schema_version
FROM meta.views v
JOIN meta.tables t ON t.id = v.table_id AND t.deleted_at IS NULL
JOIN meta.books b ON b.id = t.book_id AND b.deleted_at IS NULL
WHERE v.id = $1 AND v.deleted_at IS NULL
FOR SHARE OF v, t;

-- name: ListRowFields :many
SELECT id, ui_type, pg_column, options, is_system
FROM meta.fields
WHERE table_id = $1 AND deleted_at IS NULL
ORDER BY is_system, position, id;

-- name: ListAttachmentMetadata :many
SELECT id, file_name, mime_type, size_bytes, has_thumb
FROM meta.attachments
WHERE table_id = sqlc.arg(table_id)::uuid
  AND field_id = sqlc.arg(field_id)::uuid
  AND id = ANY(sqlc.arg(ids)::uuid[]);

-- Navigation: the user's effective grants and the live workspaces they touch.
-- Level arithmetic and path-only pruning happen in Go (internal/structure/nav.go)
-- and are tested against the resolver. Disabled users and trashed ancestors
-- yield nothing, exactly like the resolver queries.

-- name: NavGrants :many
WITH principals AS (
  SELECT 'user'::text AS principal_type, u.id AS principal_id
  FROM meta.users u
  WHERE u.id = sqlc.arg(user_id)::uuid AND u.status = 'active'
  UNION ALL
  SELECT 'group'::text, gm.group_id
  FROM meta.group_members gm
  JOIN meta.users u ON u.id = gm.user_id AND u.status = 'active'
  WHERE gm.user_id = sqlc.arg(user_id)::uuid
)
SELECT g.resource_type, g.resource_id,
       MAX(CASE g.level WHEN 'read' THEN 1 WHEN 'write' THEN 2 WHEN 'owner' THEN 3 END)::int AS level
FROM meta.grants g
JOIN principals p ON p.principal_type = g.principal_type AND p.principal_id = g.principal_id
GROUP BY g.resource_type, g.resource_id;

-- name: NavGrantedWorkspaces :many
WITH principals AS (
  SELECT 'user'::text AS principal_type, u.id AS principal_id
  FROM meta.users u
  WHERE u.id = sqlc.arg(user_id)::uuid AND u.status = 'active'
  UNION ALL
  SELECT 'group'::text, gm.group_id
  FROM meta.group_members gm
  JOIN meta.users u ON u.id = gm.user_id AND u.status = 'active'
  WHERE gm.user_id = sqlc.arg(user_id)::uuid
), granted AS (
  SELECT g.resource_type, g.resource_id
  FROM meta.grants g
  JOIN principals p ON p.principal_type = g.principal_type AND p.principal_id = g.principal_id
)
SELECT w.id
FROM meta.workspaces w
WHERE w.deleted_at IS NULL AND (
  EXISTS (SELECT 1 FROM granted g WHERE g.resource_type = 'workspace' AND g.resource_id = w.id)
  OR EXISTS (SELECT 1 FROM granted g JOIN meta.books b ON b.id = g.resource_id
             WHERE g.resource_type = 'book' AND b.workspace_id = w.id AND b.deleted_at IS NULL)
  OR EXISTS (SELECT 1 FROM granted g JOIN meta.tables t ON t.id = g.resource_id
             JOIN meta.books b ON b.id = t.book_id
             WHERE g.resource_type = 'table' AND b.workspace_id = w.id
               AND b.deleted_at IS NULL AND t.deleted_at IS NULL)
);

-- name: NavWorkspaces :many
SELECT id, name, icon, color, created_at
FROM meta.workspaces
WHERE id = ANY(sqlc.arg(ids)::uuid[]) AND deleted_at IS NULL;

-- name: NavBooks :many
SELECT id, workspace_id, name, position
FROM meta.books
WHERE workspace_id = ANY(sqlc.arg(workspace_ids)::uuid[]) AND deleted_at IS NULL;

-- name: NavTables :many
SELECT t.id, t.book_id, t.name, t.position, t.schema_version
FROM meta.tables t
JOIN meta.books b ON b.id = t.book_id
WHERE b.workspace_id = ANY(sqlc.arg(workspace_ids)::uuid[]) AND b.deleted_at IS NULL AND t.deleted_at IS NULL;

-- name: NavViews :many
SELECT id, table_id, name, type, scope, position
FROM meta.views
WHERE table_id = ANY(sqlc.arg(table_ids)::uuid[]) AND deleted_at IS NULL
  AND (scope = 'shared' OR owner_id = sqlc.arg(user_id)::uuid)
ORDER BY position, id;
-- name: TryDevSeedLock :one
SELECT pg_try_advisory_lock(hashtextextended('pandatable.dev.seed', 0))::boolean;

-- name: ReleaseDevSeedLock :one
SELECT pg_advisory_unlock(hashtextextended('pandatable.dev.seed', 0))::boolean;

-- name: InspectDevSeedTarget :one
SELECT current_database()::text AS database_name,
       current_setting('server_version_num')::integer AS server_version,
       NOT EXISTS (SELECT 1 FROM meta.workspaces) AS empty;
