-- name: LockUserAdministration :exec
SELECT pg_advisory_xact_lock(hashtextextended('pandatable.admin.users', 0));

-- name: SessionUser :one
SELECT u.id, u.role, u.status, u.must_change_password
FROM meta.sessions s JOIN meta.users u ON u.id = s.user_id
WHERE s.id_hash = $1
  AND s.created_at <= clock_timestamp() AND s.expires_at > clock_timestamp()
  AND s.last_seen_at + interval '7 days' > clock_timestamp()
  AND s.created_at + interval '30 days' > clock_timestamp();

-- name: LockUser :one
SELECT id FROM meta.users WHERE id = $1 FOR UPDATE;

-- name: LockGroupAdministrator :one
SELECT id FROM meta.users WHERE id = $1 FOR NO KEY UPDATE;

-- name: GetUser :one
SELECT id, username, email, display_name, role, status, must_change_password,
       last_login_at, created_at, updated_at
FROM meta.users WHERE id = $1;

-- name: CountUsers :one
SELECT count(*) FROM meta.users
WHERE (sqlc.arg(status)::text = '' OR status = sqlc.arg(status))
  AND (strpos(lower(username), sqlc.arg(q)::text) > 0
    OR strpos(lower(COALESCE(email, '')), sqlc.arg(q)) > 0
    OR strpos(lower(display_name), sqlc.arg(q)) > 0);

-- name: ListUsers :many
SELECT id, username, email, display_name, role, status, must_change_password,
       last_login_at, created_at, updated_at
FROM meta.users
WHERE (sqlc.arg(status)::text = '' OR status = sqlc.arg(status))
  AND (strpos(lower(username), sqlc.arg(q)::text) > 0
    OR strpos(lower(COALESCE(email, '')), sqlc.arg(q)) > 0
    OR strpos(lower(display_name), sqlc.arg(q)) > 0)
ORDER BY username, id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: IdentityExists :one
SELECT EXISTS (
  SELECT 1 FROM meta.users
  WHERE (sqlc.narg(exclude_id)::uuid IS NULL OR id <> sqlc.narg(exclude_id))
    AND (lower(username) = sqlc.arg(username)::text OR lower(email) = sqlc.arg(username)
      OR (sqlc.narg(email)::text IS NOT NULL
        AND (lower(username) = sqlc.narg(email) OR lower(email) = sqlc.narg(email))))
);

-- name: CreateUser :one
INSERT INTO meta.users (username, email, display_name, role, password_hash,
    locale, must_change_password, avatar_color)
VALUES ($1, $2, $3, $4, $5, $6, true, 'teal')
RETURNING id;

-- name: UpdateUser :exec
UPDATE meta.users SET display_name = $2, email = $3, role = $4, status = $5,
    updated_at = clock_timestamp() WHERE id = $1;

-- name: CountActiveAdmins :one
SELECT count(*) FROM meta.users WHERE role = 'admin' AND status = 'active';

-- name: ResetPassword :exec
UPDATE meta.users SET password_hash = $2, must_change_password = true,
    updated_at = clock_timestamp() WHERE id = $1;

-- name: RevokeUserSessions :exec
DELETE FROM meta.sessions WHERE user_id = $1;

-- name: CountGroups :one
SELECT count(*) FROM meta.groups g
WHERE (strpos(lower(g.name), sqlc.arg(q)::text) > 0
    OR strpos(lower(g.description), sqlc.arg(q)) > 0)
  AND (sqlc.narg(user_id)::uuid IS NULL OR EXISTS (
    SELECT 1 FROM meta.group_members m WHERE m.group_id=g.id AND m.user_id=sqlc.narg(user_id)));

-- name: ListGroups :many
SELECT g.id, g.name, g.description, g.created_at,
    (SELECT count(*) FROM meta.group_members m WHERE m.group_id=g.id) AS member_count
FROM meta.groups g
WHERE (strpos(lower(g.name), sqlc.arg(q)::text) > 0
    OR strpos(lower(g.description), sqlc.arg(q)) > 0)
  AND (sqlc.narg(user_id)::uuid IS NULL OR EXISTS (
    SELECT 1 FROM meta.group_members m WHERE m.group_id=g.id AND m.user_id=sqlc.narg(user_id)))
ORDER BY g.name, g.id LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: GetGroup :one
SELECT g.id, g.name, g.description, g.created_at,
    (SELECT count(*) FROM meta.group_members m WHERE m.group_id=g.id) AS member_count
FROM meta.groups g WHERE g.id=$1;

-- name: CreateGroup :one
INSERT INTO meta.groups(name, description) VALUES ($1,$2) RETURNING id;

-- name: UpdateGroup :exec
UPDATE meta.groups SET name=$2, description=$3 WHERE id=$1;

-- name: LockGroup :one
SELECT id FROM meta.groups WHERE id=$1 FOR UPDATE;

-- name: LockMembershipGroup :one
SELECT id FROM meta.groups WHERE id=$1 FOR KEY SHARE;

-- name: DeleteGroup :one
DELETE FROM meta.groups WHERE id=$1 RETURNING id;

-- name: DeleteGroupGrants :exec
DELETE FROM meta.grants WHERE principal_type='group' AND principal_id=$1;

-- name: CountGroupMembers :one
SELECT count(*) FROM meta.users u JOIN meta.group_members m ON m.user_id=u.id
WHERE m.group_id=sqlc.arg(group_id)
  AND (strpos(lower(u.username), sqlc.arg(q)::text) > 0
    OR strpos(lower(COALESCE(u.email,'')), sqlc.arg(q)) > 0
    OR strpos(lower(u.display_name), sqlc.arg(q)) > 0);

-- name: ListGroupMembers :many
SELECT u.id, u.username, u.email, u.display_name, u.role, u.status,
    u.must_change_password, u.last_login_at, u.created_at, u.updated_at
FROM meta.users u JOIN meta.group_members m ON m.user_id=u.id
WHERE m.group_id=sqlc.arg(group_id)
  AND (strpos(lower(u.username), sqlc.arg(q)::text) > 0
    OR strpos(lower(COALESCE(u.email,'')), sqlc.arg(q)) > 0
    OR strpos(lower(u.display_name), sqlc.arg(q)) > 0)
ORDER BY u.username,u.id LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: AddGroupMember :execrows
INSERT INTO meta.group_members(group_id,user_id) VALUES ($1,$2) ON CONFLICT DO NOTHING;

-- name: RemoveGroupMember :execrows
DELETE FROM meta.group_members WHERE group_id=$1 AND user_id=$2;

-- name: SystemDatabase :one
SELECT pg_database_size(current_database())::bigint AS size_bytes,
       count(*)::bigint AS connections,
       count(*) FILTER (WHERE state = 'active')::bigint AS active_connections,
       pg_size_bytes(current_setting('shared_buffers'))::bigint AS shared_buffers_bytes,
       current_setting('server_version')::text AS server_version
FROM pg_stat_activity WHERE datname = current_database();

-- name: SystemMigration :one
SELECT COALESCE(max(version_id), 0)::bigint AS migration
FROM (
    SELECT DISTINCT ON (version_id) version_id, is_applied
    FROM public.goose_db_version ORDER BY version_id, id DESC
) AS latest WHERE is_applied;

-- name: ReadSettings :many
SELECT key, value FROM meta.settings WHERE key = ANY($1::text[]) ORDER BY key;

-- name: WriteSetting :exec
INSERT INTO meta.settings(key,value) VALUES ($1,$2) ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value;

-- name: ValidSettingTimezone :one
SELECT EXISTS (SELECT 1 FROM pg_timezone_names WHERE name=$1 AND name NOT LIKE 'posix/%' AND name NOT LIKE 'right/%');

-- name: CountWorkspaces :one
SELECT count(*) FROM meta.workspaces WHERE deleted_at IS NULL AND strpos(lower(name), sqlc.arg(q)::text)>0;

-- name: ListWorkspaces :many
WITH page AS (
    SELECT id,name FROM meta.workspaces
    WHERE deleted_at IS NULL AND strpos(lower(name), sqlc.arg(q)::text)>0
    ORDER BY name,id LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset)
)
SELECT p.id,p.name,
    NOT EXISTS (SELECT 1 FROM meta.grants g JOIN meta.users u ON u.id=g.principal_id
        WHERE g.resource_type='workspace' AND g.resource_id=p.id
          AND g.principal_type='user' AND g.level='owner' AND u.status='active') AS orphaned,
    (SELECT count(*) FROM meta.grants g
        LEFT JOIN meta.users u ON g.principal_type='user' AND u.id=g.principal_id
        LEFT JOIN meta.groups gr ON g.principal_type='group' AND gr.id=g.principal_id
        WHERE g.resource_type='workspace' AND g.resource_id=p.id AND g.level='owner'
          AND (u.id IS NOT NULL OR gr.id IS NOT NULL)) AS owner_total,
    (SELECT COALESCE(sum(a.size_bytes),0)::bigint FROM meta.attachments a
        JOIN meta.tables t ON t.id=a.table_id JOIN meta.books b ON b.id=t.book_id
        WHERE b.workspace_id=p.id) AS attachment_bytes
FROM page p ORDER BY p.name,p.id;

-- name: ListWorkspaceOwners :many
WITH owners AS (
    SELECT g.resource_id,g.principal_id,g.principal_type,
        COALESCE(u.display_name,gr.name)::text AS name,
        u.username,u.status,
        row_number() OVER (PARTITION BY g.resource_id
            ORDER BY g.principal_type,COALESCE(u.display_name,gr.name),g.principal_id) AS position
    FROM meta.grants g
    LEFT JOIN meta.users u ON g.principal_type='user' AND u.id=g.principal_id
    LEFT JOIN meta.groups gr ON g.principal_type='group' AND gr.id=g.principal_id
    WHERE g.resource_type='workspace' AND g.resource_id=ANY($1::uuid[]) AND g.level='owner'
      AND (u.id IS NOT NULL OR gr.id IS NOT NULL)
)
SELECT resource_id,principal_id,principal_type,name,username,status
FROM owners WHERE position<=50 ORDER BY resource_id,position;

-- name: LockUserRelatedWorkspaces :many
SELECT w.id FROM meta.workspaces w
WHERE EXISTS (
    SELECT 1 FROM meta.grants g
    WHERE g.principal_type='user' AND g.principal_id=sqlc.arg(user_id)::uuid
      AND ((g.resource_type='workspace' AND g.resource_id=w.id)
        OR (g.resource_type='book' AND EXISTS (
          SELECT 1 FROM meta.books b WHERE b.id=g.resource_id AND b.workspace_id=w.id))
        OR (g.resource_type='table' AND EXISTS (
          SELECT 1 FROM meta.tables t JOIN meta.books b ON b.id=t.book_id
          WHERE t.id=g.resource_id AND b.workspace_id=w.id)))
) OR EXISTS (
    SELECT 1 FROM meta.views v JOIN meta.tables t ON t.id=v.table_id
    JOIN meta.books b ON b.id=t.book_id
    WHERE v.owner_id=sqlc.arg(user_id)::uuid AND b.workspace_id=w.id
)
ORDER BY w.id FOR NO KEY UPDATE OF w;

-- name: LockLifecycleWorkspace :one
SELECT id FROM meta.workspaces WHERE id=$1 AND deleted_at IS NULL FOR NO KEY UPDATE;

-- name: WorkspaceIsOrphaned :one
SELECT NOT EXISTS (
    SELECT 1 FROM meta.grants g JOIN meta.users u ON u.id=g.principal_id
    WHERE g.resource_type='workspace' AND g.resource_id=$1
      AND g.principal_type='user' AND g.level='owner' AND u.status='active'
);

-- name: CountUserOwnerships :one
SELECT count(*) FROM meta.grants
WHERE principal_type='user' AND principal_id=$1 AND level='owner';

-- name: CountDanglingUserOwnerships :one
SELECT count(*) FROM meta.grants g
WHERE g.principal_type='user' AND g.principal_id=$1 AND g.level='owner'
  AND NOT ((g.resource_type='workspace' AND EXISTS (
        SELECT 1 FROM meta.workspaces w WHERE w.id=g.resource_id))
    OR (g.resource_type='book' AND EXISTS (
        SELECT 1 FROM meta.books b JOIN meta.workspaces w ON w.id=b.workspace_id WHERE b.id=g.resource_id))
    OR (g.resource_type='table' AND EXISTS (
        SELECT 1 FROM meta.tables t JOIN meta.books b ON b.id=t.book_id
        JOIN meta.workspaces w ON w.id=b.workspace_id WHERE t.id=g.resource_id)));

-- name: ListUserOwnerships :many
SELECT g.resource_type,g.resource_id,
    CASE g.resource_type WHEN 'workspace' THEN w.name WHEN 'book' THEN b.name ELSE t.name END::text AS name,
    w.id AS workspace_id,w.name AS workspace_name,
    (w.deleted_at IS NOT NULL OR b.deleted_at IS NOT NULL OR t.deleted_at IS NOT NULL)::boolean AS trashed
FROM meta.grants g
LEFT JOIN meta.tables t ON g.resource_type='table' AND t.id=g.resource_id
LEFT JOIN meta.books b ON (g.resource_type='book' AND b.id=g.resource_id) OR b.id=t.book_id
JOIN meta.workspaces w ON (g.resource_type='workspace' AND w.id=g.resource_id) OR w.id=b.workspace_id
WHERE g.principal_type='user' AND g.principal_id=sqlc.arg(user_id)::uuid AND g.level='owner'
ORDER BY w.name,w.id,g.resource_type,g.resource_id
LIMIT sqlc.arg(page_limit)::int OFFSET sqlc.arg(page_offset)::int;

-- name: TransferUserOwnerships :execrows
INSERT INTO meta.grants(resource_type,resource_id,principal_type,principal_id,level,granted_by)
SELECT resource_type,resource_id,'user',sqlc.arg(recipient_id)::uuid,'owner',sqlc.arg(actor_id)::uuid
FROM meta.grants
WHERE principal_type='user' AND principal_id=sqlc.arg(user_id)::uuid AND level='owner'
ON CONFLICT(resource_type,resource_id,principal_type,principal_id)
DO UPDATE SET level='owner',granted_by=EXCLUDED.granted_by;

-- name: RemoveUserOwnerships :exec
DELETE FROM meta.grants WHERE principal_type='user' AND principal_id=$1 AND level='owner';

-- name: SetWorkspaceOwner :exec
INSERT INTO meta.grants(resource_type,resource_id,principal_type,principal_id,level,granted_by)
VALUES ('workspace',sqlc.arg(workspace_id)::uuid,'user',sqlc.arg(user_id)::uuid,'owner',sqlc.arg(actor_id)::uuid)
ON CONFLICT(resource_type,resource_id,principal_type,principal_id)
DO UPDATE SET level='owner',granted_by=EXCLUDED.granted_by;

-- name: ListSoleOwnedWorkspaces :many
SELECT w.id FROM meta.workspaces w JOIN meta.grants g
  ON g.resource_type='workspace' AND g.resource_id=w.id AND g.principal_type='user'
  AND g.principal_id=$1 AND g.level='owner'
WHERE NOT EXISTS (
    SELECT 1 FROM meta.grants other
    WHERE other.resource_type='workspace' AND other.resource_id=w.id
      AND other.level='owner' AND other.id<>g.id
)
ORDER BY w.id;

-- name: RemoveAllUserGrants :exec
DELETE FROM meta.grants WHERE principal_type='user' AND principal_id=$1;

-- name: RecordDeletedUser :exec
INSERT INTO meta.deleted_users(id) VALUES ($1);

-- name: HardDeleteUser :exec
DELETE FROM meta.users WHERE id=$1;
-- name: ReadSMTPSettings :one
SELECT enabled, host, port, security, username, password_cipher, from_address
FROM meta.smtp_settings WHERE singleton = true;

-- name: WriteSMTPSettings :exec
UPDATE meta.smtp_settings
SET enabled = sqlc.arg(enabled), host = sqlc.arg(host), port = sqlc.arg(port),
    security = sqlc.arg(security), username = sqlc.arg(username),
    password_cipher = sqlc.narg(password_cipher), from_address = sqlc.arg(from_address)
WHERE singleton = true;
