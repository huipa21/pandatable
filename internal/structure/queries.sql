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

-- name: ConvertField :one
UPDATE meta.fields
SET ui_type = sqlc.arg(ui_type), pg_type = sqlc.arg(pg_type),
    options = sqlc.arg(options)::jsonb, is_required = sqlc.arg(is_required),
    is_unique = sqlc.arg(is_unique), default_json = sqlc.narg(default_json)::jsonb,
    version = version + 1
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING version;

-- name: ListComputedDependents :many
SELECT f.id, f.table_id, f.ui_type, f.options
FROM meta.fields f
JOIN meta.tables t ON t.id = f.table_id AND t.deleted_at IS NULL
JOIN meta.books b ON b.id = t.book_id AND b.deleted_at IS NULL
WHERE f.deleted_at IS NULL AND f.ui_type IN ('lookup', 'rollup')
  AND f.options->>'targetFieldId' = sqlc.arg(field_id)::text
ORDER BY f.table_id, f.id;

-- name: LockDependencyTable :one
SELECT id FROM meta.tables
WHERE id = $1 AND deleted_at IS NULL
FOR NO KEY UPDATE;

-- name: BumpDependencySchemaVersion :exec
UPDATE meta.tables SET schema_version = schema_version + 1
WHERE id = $1 AND deleted_at IS NULL;

-- name: SaveDependencyFieldOptions :exec
UPDATE meta.fields
SET options = sqlc.arg(options)::jsonb, version = version + 1
WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: InsertTypeChangeHistory :exec
INSERT INTO meta.row_history (table_id, user_id, action, changes)
VALUES (sqlc.arg(table_id)::uuid, sqlc.arg(user_id)::uuid, 'type_change',
        sqlc.arg(changes)::jsonb);

-- name: InsertRowHistoryBatch :exec
INSERT INTO meta.row_history (table_id, row_id, user_id, action, changes, restore_system)
SELECT (item->>'table_id')::uuid, (item->>'row_id')::bigint,
       (item->>'user_id')::uuid, item->>'action', item->'changes', item->'restore_system'
FROM jsonb_array_elements(sqlc.arg(entries)::jsonb) AS item;

-- name: InsertDeleteHistoryBatch :many
INSERT INTO meta.row_history (table_id, row_id, user_id, action, changes, restore_system)
SELECT (item->>'table_id')::uuid, (item->>'row_id')::bigint,
       (item->>'user_id')::uuid, item->>'action', item->'changes', item->'restore_system'
FROM jsonb_array_elements(sqlc.arg(entries)::jsonb) AS item
RETURNING id, row_id;

-- name: ListRowHistory :many
SELECT h.id, h.user_id, COALESCE(u.display_name, '')::text AS author_name,
       h.action, h.changes, h.created_at, h.restore_system
FROM meta.row_history h
LEFT JOIN meta.users u ON u.id = h.user_id
WHERE h.table_id = sqlc.arg(table_id)::uuid AND h.row_id = sqlc.arg(row_id)::bigint
ORDER BY h.id DESC
LIMIT sqlc.arg(page_limit)::int OFFSET sqlc.arg(page_offset)::int;

-- name: ReadHistoryRetentionDays :one
SELECT COALESCE(
    (SELECT CASE WHEN jsonb_typeof(value) = 'number' THEN (value #>> '{}')::int ELSE 0 END
     FROM meta.settings WHERE key = 'history_retention_days'),
    90
)::int;

-- name: PurgeRowHistory :execrows
DELETE FROM meta.row_history WHERE created_at < sqlc.arg(cutoff)::timestamptz;

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

-- name: ListViewConfigsForTable :many
SELECT id, config
FROM meta.views
WHERE table_id = sqlc.arg(table_id)::uuid AND deleted_at IS NULL
ORDER BY id;

-- name: SaveViewConfigForTypeChange :exec
UPDATE meta.views
SET config = sqlc.arg(config)::jsonb, version = version + 1, updated_at = now()
WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

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

-- name: ListRowComments :many
SELECT c.id, c.user_id, COALESCE(u.display_name, '')::text AS author_name,
       c.body, c.mentions, c.created_at, c.edited_at
FROM meta.comments c
LEFT JOIN meta.users u ON u.id = c.user_id
WHERE c.table_id = sqlc.arg(table_id)::uuid AND c.row_id = sqlc.arg(row_id)::bigint
  AND c.deleted_at IS NULL
ORDER BY (c.id = sqlc.narg(focus_id)::uuid) DESC NULLS LAST, c.created_at DESC, c.id DESC
LIMIT sqlc.arg(page_limit)::int OFFSET sqlc.arg(page_offset)::int;

-- name: FindRowComment :one
SELECT c.id, c.table_id, c.row_id, c.user_id, COALESCE(u.display_name, '')::text AS author_name,
       c.body, c.mentions, c.created_at, c.edited_at
FROM meta.comments c
LEFT JOIN meta.users u ON u.id = c.user_id
WHERE c.id = sqlc.arg(id)::uuid AND c.deleted_at IS NULL;

-- name: LockRowComment :one
SELECT id, table_id, row_id, user_id, mentions
FROM meta.comments
WHERE id = sqlc.arg(id)::uuid AND deleted_at IS NULL
FOR UPDATE;

-- name: InsertRowComment :one
INSERT INTO meta.comments (table_id, row_id, user_id, body, mentions)
VALUES (sqlc.arg(table_id)::uuid, sqlc.arg(row_id)::bigint, sqlc.arg(user_id)::uuid,
        sqlc.arg(body)::text, sqlc.arg(mentions)::uuid[])
RETURNING id;

-- name: UpdateRowComment :exec
UPDATE meta.comments SET body = sqlc.arg(body)::text, mentions = sqlc.arg(mentions)::uuid[], edited_at = now()
WHERE id = sqlc.arg(id)::uuid AND deleted_at IS NULL;

-- name: DeleteRowComment :exec
UPDATE meta.comments SET deleted_at = now()
WHERE id = sqlc.arg(id)::uuid AND deleted_at IS NULL;

-- name: CountRowComments :many
SELECT row_id, count(*)::bigint AS comment_count
FROM meta.comments
WHERE table_id = sqlc.arg(table_id)::uuid AND row_id = ANY(sqlc.arg(row_ids)::bigint[])
  AND deleted_at IS NULL
GROUP BY row_id;

-- name: LockMentionUsers :many
SELECT id, username, display_name AS name
FROM meta.users
WHERE username = ANY(sqlc.arg(usernames)::text[]) AND status = 'active'
ORDER BY id
FOR SHARE;

-- name: GetMentionUsers :many
SELECT id, username, display_name AS name
FROM meta.users
WHERE id = ANY(sqlc.arg(ids)::uuid[])
ORDER BY id;

-- name: InsertMentionNotification :one
INSERT INTO meta.notifications (user_id, kind, payload)
VALUES (sqlc.arg(user_id)::uuid, 'mention', jsonb_build_object('commentId', sqlc.arg(comment_id)::uuid))
RETURNING id;

-- name: DeleteMentionNotifications :exec
DELETE FROM meta.notifications
WHERE kind = 'mention' AND user_id = ANY(sqlc.arg(user_ids)::uuid[])
  AND payload->>'commentId' = sqlc.arg(comment_id)::text;

-- name: ListMentionNotifications :one
WITH visible AS (
  SELECT n.id, n.created_at, n.read_at, c.id AS comment_id, c.row_id,
         t.id AS table_id, t.name AS table_name, COALESCE(u.display_name, '')::text AS author_name
  FROM meta.notifications n
  JOIN meta.comments c ON c.id::text = n.payload->>'commentId' AND c.deleted_at IS NULL
  JOIN meta.tables t ON t.id = c.table_id AND t.deleted_at IS NULL
  JOIN meta.books b ON b.id = t.book_id AND b.deleted_at IS NULL
  JOIN meta.workspaces w ON w.id = b.workspace_id AND w.deleted_at IS NULL
  JOIN meta.users viewer ON viewer.id = n.user_id AND viewer.status = 'active'
  LEFT JOIN meta.users u ON u.id = c.user_id
  WHERE n.user_id = sqlc.arg(user_id)::uuid AND n.kind = 'mention'
    AND n.user_id = ANY(c.mentions)
    AND EXISTS (
      SELECT 1 FROM meta.grants g
      WHERE ((g.resource_type = 'workspace' AND g.resource_id = w.id)
         OR (g.resource_type = 'book' AND g.resource_id = b.id)
         OR (g.resource_type = 'table' AND g.resource_id = t.id))
        AND ((g.principal_type = 'user' AND g.principal_id = n.user_id)
          OR (g.principal_type = 'group' AND EXISTS (
            SELECT 1 FROM meta.group_members gm
            WHERE gm.group_id = g.principal_id AND gm.user_id = n.user_id
          )))
    )
), page AS (
  SELECT * FROM visible
  ORDER BY created_at DESC, id DESC
  LIMIT sqlc.arg(page_limit)::int OFFSET sqlc.arg(page_offset)::int
)
SELECT (SELECT count(*)::bigint FROM visible WHERE read_at IS NULL) AS unread_count,
       COALESCE(jsonb_agg(jsonb_build_object(
         'id', id, 'tableId', table_id, 'tableName', table_name, 'rowId', row_id::text,
         'commentId', comment_id, 'authorName', author_name, 'createdAt', created_at, 'readAt', read_at
       ) ORDER BY created_at DESC, id DESC), '[]'::jsonb)::text AS items
FROM page;

-- name: MarkMentionNotificationRead :execrows
UPDATE meta.notifications SET read_at = now()
WHERE id = sqlc.arg(id)::uuid AND user_id = sqlc.arg(user_id)::uuid
  AND kind = 'mention' AND read_at IS NULL;
