-- name: SearchShareUsers :many
SELECT id, display_name AS name, username
FROM meta.users
WHERE status = 'active'
  AND (strpos(lower(username), lower(sqlc.arg(search)::text)) > 0
       OR strpos(lower(display_name), lower(sqlc.arg(search)::text)) > 0)
ORDER BY username, id
LIMIT 21;

-- name: SearchMentionUsers :many
SELECT u.id, u.display_name AS name, u.username
FROM meta.users u
JOIN meta.tables t ON t.id = sqlc.arg(table_id)::uuid AND t.deleted_at IS NULL
JOIN meta.books b ON b.id = t.book_id AND b.deleted_at IS NULL
JOIN meta.workspaces w ON w.id = b.workspace_id AND w.deleted_at IS NULL
WHERE u.status = 'active'
  AND (strpos(lower(u.username), lower(sqlc.arg(search)::text)) > 0
       OR strpos(lower(u.display_name), lower(sqlc.arg(search)::text)) > 0)
  AND EXISTS (
    SELECT 1 FROM meta.grants g
    WHERE ((g.resource_type = 'workspace' AND g.resource_id = w.id)
       OR (g.resource_type = 'book' AND g.resource_id = b.id)
       OR (g.resource_type = 'table' AND g.resource_id = t.id))
      AND ((g.principal_type = 'user' AND g.principal_id = u.id)
        OR (g.principal_type = 'group' AND EXISTS (
          SELECT 1 FROM meta.group_members gm
          WHERE gm.group_id = g.principal_id AND gm.user_id = u.id
        )))
  )
ORDER BY u.username, u.id
LIMIT 21;

-- name: ResolveTableUserLabels :many
SELECT id, display_name AS name, false AS deleted
FROM meta.users WHERE id = ANY(sqlc.arg(ids)::uuid[])
UNION ALL
SELECT id, ''::text AS name, true AS deleted
FROM meta.deleted_users WHERE id = ANY(sqlc.arg(ids)::uuid[])
ORDER BY id;

-- name: SearchShareGroups :many
SELECT id, name
FROM meta.groups
WHERE strpos(lower(name), lower(sqlc.arg(search)::text)) > 0
ORDER BY name, id
LIMIT 21;

-- Access queries return the maximum applicable grant along the target path
-- (workspace -> book -> table) for the user and the user's flat groups, plus
-- whether the target is visible path-only through a grant on a descendant.
-- Disabled users and trashed targets/ancestors yield no grants at all.

-- name: WorkspaceAccess :one
WITH principals AS (
  SELECT 'user'::text AS principal_type, u.id AS principal_id
  FROM meta.users u
  WHERE u.id = sqlc.arg(user_id)::uuid AND u.status = 'active'
  UNION ALL
  SELECT 'group'::text, gm.group_id
  FROM meta.group_members gm
  JOIN meta.users u ON u.id = gm.user_id AND u.status = 'active'
  WHERE gm.user_id = sqlc.arg(user_id)::uuid
), scope AS (
  SELECT 'workspace'::text AS resource_type, w.id AS resource_id, true AS on_path
  FROM meta.workspaces w
  WHERE w.id = sqlc.arg(workspace_id)::uuid AND w.deleted_at IS NULL
  UNION ALL
  SELECT 'book'::text, b.id, false
  FROM meta.books b
  JOIN meta.workspaces w ON w.id = b.workspace_id
  WHERE w.id = sqlc.arg(workspace_id)::uuid AND w.deleted_at IS NULL AND b.deleted_at IS NULL
  UNION ALL
  SELECT 'table'::text, t.id, false
  FROM meta.tables t
  JOIN meta.books b ON b.id = t.book_id
  JOIN meta.workspaces w ON w.id = b.workspace_id
  WHERE w.id = sqlc.arg(workspace_id)::uuid AND w.deleted_at IS NULL
    AND b.deleted_at IS NULL AND t.deleted_at IS NULL
)
SELECT COALESCE(MAX(CASE g.level WHEN 'read' THEN 1 WHEN 'write' THEN 2 WHEN 'owner' THEN 3 END)
                FILTER (WHERE s.on_path), 0)::int AS level,
       COUNT(*) > 0 AS visible
FROM meta.grants g
JOIN principals p ON p.principal_type = g.principal_type AND p.principal_id = g.principal_id
JOIN scope s ON s.resource_type = g.resource_type AND s.resource_id = g.resource_id;

-- name: BookAccess :one
WITH principals AS (
  SELECT 'user'::text AS principal_type, u.id AS principal_id
  FROM meta.users u
  WHERE u.id = sqlc.arg(user_id)::uuid AND u.status = 'active'
  UNION ALL
  SELECT 'group'::text, gm.group_id
  FROM meta.group_members gm
  JOIN meta.users u ON u.id = gm.user_id AND u.status = 'active'
  WHERE gm.user_id = sqlc.arg(user_id)::uuid
), scope AS (
  SELECT 'workspace'::text AS resource_type, w.id AS resource_id, true AS on_path
  FROM meta.books b
  JOIN meta.workspaces w ON w.id = b.workspace_id
  WHERE b.id = sqlc.arg(book_id)::uuid AND w.deleted_at IS NULL AND b.deleted_at IS NULL
  UNION ALL
  SELECT 'book'::text, b.id, true
  FROM meta.books b
  JOIN meta.workspaces w ON w.id = b.workspace_id
  WHERE b.id = sqlc.arg(book_id)::uuid AND w.deleted_at IS NULL AND b.deleted_at IS NULL
  UNION ALL
  SELECT 'table'::text, t.id, false
  FROM meta.tables t
  JOIN meta.books b ON b.id = t.book_id
  JOIN meta.workspaces w ON w.id = b.workspace_id
  WHERE b.id = sqlc.arg(book_id)::uuid AND w.deleted_at IS NULL
    AND b.deleted_at IS NULL AND t.deleted_at IS NULL
)
SELECT COALESCE(MAX(CASE g.level WHEN 'read' THEN 1 WHEN 'write' THEN 2 WHEN 'owner' THEN 3 END)
                FILTER (WHERE s.on_path), 0)::int AS level,
       COUNT(*) > 0 AS visible
FROM meta.grants g
JOIN principals p ON p.principal_type = g.principal_type AND p.principal_id = g.principal_id
JOIN scope s ON s.resource_type = g.resource_type AND s.resource_id = g.resource_id;

-- name: TableAccess :one
WITH principals AS (
  SELECT 'user'::text AS principal_type, u.id AS principal_id
  FROM meta.users u
  WHERE u.id = sqlc.arg(user_id)::uuid AND u.status = 'active'
  UNION ALL
  SELECT 'group'::text, gm.group_id
  FROM meta.group_members gm
  JOIN meta.users u ON u.id = gm.user_id AND u.status = 'active'
  WHERE gm.user_id = sqlc.arg(user_id)::uuid
), scope AS (
  SELECT 'workspace'::text AS resource_type, w.id AS resource_id
  FROM meta.tables t
  JOIN meta.books b ON b.id = t.book_id
  JOIN meta.workspaces w ON w.id = b.workspace_id
  WHERE t.id = sqlc.arg(table_id)::uuid
    AND w.deleted_at IS NULL AND b.deleted_at IS NULL AND t.deleted_at IS NULL
  UNION ALL
  SELECT 'book'::text, b.id
  FROM meta.tables t
  JOIN meta.books b ON b.id = t.book_id
  JOIN meta.workspaces w ON w.id = b.workspace_id
  WHERE t.id = sqlc.arg(table_id)::uuid
    AND w.deleted_at IS NULL AND b.deleted_at IS NULL AND t.deleted_at IS NULL
  UNION ALL
  SELECT 'table'::text, t.id
  FROM meta.tables t
  JOIN meta.books b ON b.id = t.book_id
  JOIN meta.workspaces w ON w.id = b.workspace_id
  WHERE t.id = sqlc.arg(table_id)::uuid
    AND w.deleted_at IS NULL AND b.deleted_at IS NULL AND t.deleted_at IS NULL
)
SELECT COALESCE(MAX(CASE g.level WHEN 'read' THEN 1 WHEN 'write' THEN 2 WHEN 'owner' THEN 3 END), 0)::int AS level,
       COUNT(*) > 0 AS visible
FROM meta.grants g
JOIN principals p ON p.principal_type = g.principal_type AND p.principal_id = g.principal_id
JOIN scope s ON s.resource_type = g.resource_type AND s.resource_id = g.resource_id;

-- Live (untrashed) path lookups; no row means the target is hidden for everyone.

-- name: WorkspaceTarget :one
SELECT w.id AS workspace_id
FROM meta.workspaces w
WHERE w.id = $1 AND w.deleted_at IS NULL;

-- name: BookTarget :one
SELECT w.id AS workspace_id, b.id AS book_id
FROM meta.books b
JOIN meta.workspaces w ON w.id = b.workspace_id
WHERE b.id = $1 AND w.deleted_at IS NULL AND b.deleted_at IS NULL;

-- name: TableTarget :one
SELECT w.id AS workspace_id, b.id AS book_id, t.id AS table_id
FROM meta.tables t
JOIN meta.books b ON b.id = t.book_id
JOIN meta.workspaces w ON w.id = b.workspace_id
WHERE t.id = $1 AND w.deleted_at IS NULL AND b.deleted_at IS NULL AND t.deleted_at IS NULL;

-- Mutation locks are taken one statement at a time in a fixed order
-- (workspace, book, table, principal) so they cannot deadlock each other.
-- The workspace lock serializes every grant mutation in that workspace and
-- conflicts with hard deletion; key-share locks conflict only with hard deletion.

-- name: LockWorkspace :one
SELECT id FROM meta.workspaces WHERE id = $1 AND deleted_at IS NULL FOR NO KEY UPDATE;

-- name: LockBook :one
SELECT id FROM meta.books
WHERE id = sqlc.arg(book_id)::uuid AND workspace_id = sqlc.arg(workspace_id)::uuid AND deleted_at IS NULL
FOR KEY SHARE;

-- name: LockTable :one
SELECT id FROM meta.tables
WHERE id = sqlc.arg(table_id)::uuid AND book_id = sqlc.arg(book_id)::uuid AND deleted_at IS NULL
FOR KEY SHARE;

-- name: LockUserPrincipal :one
SELECT status FROM meta.users WHERE id = $1 FOR SHARE;

-- name: LockGroupPrincipal :one
SELECT id FROM meta.groups WHERE id = $1 FOR SHARE;

-- name: GetGrant :one
SELECT id, resource_type, resource_id, principal_type, principal_id, level, granted_by, created_at
FROM meta.grants WHERE id = $1;

-- name: LockGrant :one
SELECT id, resource_type, resource_id, principal_type, principal_id, level, granted_by, created_at
FROM meta.grants WHERE id = $1 FOR UPDATE;

-- name: LockGrantByPair :one
SELECT id, resource_type, resource_id, principal_type, principal_id, level, granted_by, created_at
FROM meta.grants
WHERE resource_type = $1 AND resource_id = $2 AND principal_type = $3 AND principal_id = $4
FOR UPDATE;

-- name: InsertGrant :one
INSERT INTO meta.grants (resource_type, resource_id, principal_type, principal_id, level, granted_by)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, resource_type, resource_id, principal_type, principal_id, level, granted_by, created_at;

-- name: UpdateGrantLevel :one
UPDATE meta.grants SET level = $2, granted_by = $3 WHERE id = $1
RETURNING id, resource_type, resource_id, principal_type, principal_id, level, granted_by, created_at;

-- name: DeleteGrant :exec
DELETE FROM meta.grants WHERE id = $1;

-- Active direct user owners of a workspace other than the grant being changed.
-- Group-only ownership never counts (D-023).

-- name: CountOtherWorkspaceOwners :one
SELECT count(*)::bigint
FROM meta.grants g
JOIN meta.users u ON u.id = g.principal_id
WHERE g.resource_type = 'workspace' AND g.resource_id = sqlc.arg(workspace_id)::uuid
  AND g.principal_type = 'user' AND g.level = 'owner' AND u.status = 'active'
  AND g.id <> sqlc.arg(excluded_grant_id)::uuid;

-- name: ListDirectGrants :many
SELECT g.id, g.resource_type, g.resource_id, g.principal_type, g.principal_id, g.level,
       g.granted_by, g.created_at,
       u.username AS user_username, u.display_name AS user_display_name, u.status AS user_status,
       gr.name AS group_name
FROM meta.grants g
LEFT JOIN meta.users u ON g.principal_type = 'user' AND u.id = g.principal_id
LEFT JOIN meta.groups gr ON g.principal_type = 'group' AND gr.id = g.principal_id
WHERE g.resource_type = sqlc.arg(resource_type)::text AND g.resource_id = sqlc.arg(resource_id)::uuid
ORDER BY g.created_at, g.id;

-- name: ListInheritedGrants :many
SELECT g.id, g.resource_type, g.resource_id, g.principal_type, g.principal_id, g.level,
       g.granted_by, g.created_at,
       u.username AS user_username, u.display_name AS user_display_name, u.status AS user_status,
       gr.name AS group_name
FROM meta.grants g
LEFT JOIN meta.users u ON g.principal_type = 'user' AND u.id = g.principal_id
LEFT JOIN meta.groups gr ON g.principal_type = 'group' AND gr.id = g.principal_id
WHERE (g.resource_type = 'workspace' AND g.resource_id = sqlc.narg(workspace_id)::uuid)
   OR (g.resource_type = 'book' AND g.resource_id = sqlc.narg(book_id)::uuid)
ORDER BY g.resource_type DESC, g.created_at, g.id;
