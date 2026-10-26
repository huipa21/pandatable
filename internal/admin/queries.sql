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
    must_change_password, avatar_color)
VALUES ($1, $2, $3, $4, $5, true, 'teal')
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
