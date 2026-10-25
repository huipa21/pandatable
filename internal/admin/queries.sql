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
