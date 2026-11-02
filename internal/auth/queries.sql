-- name: FindLoginUsers :many
SELECT id, password_hash, status FROM meta.users
WHERE username = lower(sqlc.arg(login)::text) OR lower(email) = lower(sqlc.arg(login)::text)
LIMIT 2;

-- name: LockUser :one
SELECT id, password_hash, status FROM meta.users WHERE id = $1 FOR UPDATE;

-- name: GetUser :one
SELECT id, username, email, display_name, role, must_change_password,
       locale, theme, timezone, avatar_color, mention_email
FROM meta.users WHERE id = $1 AND status = 'active';

-- name: CreateSession :exec
INSERT INTO meta.sessions (id_hash, user_id, created_at, last_seen_at, expires_at, ip, user_agent)
VALUES (sqlc.arg(id_hash), sqlc.arg(user_id), sqlc.arg(now)::timestamptz,
        sqlc.arg(now)::timestamptz, sqlc.arg(expires_at)::timestamptz,
        sqlc.narg(ip)::inet, sqlc.arg(user_agent));

-- name: TouchSession :one
UPDATE meta.sessions s
SET last_seen_at = GREATEST(s.last_seen_at, sqlc.arg(now)::timestamptz),
    expires_at = LEAST(s.created_at + interval '30 days', GREATEST(s.last_seen_at, sqlc.arg(now)::timestamptz) + interval '7 days')
FROM meta.users u
WHERE s.id_hash = sqlc.arg(id_hash) AND u.id = s.user_id AND u.status = 'active'
  AND s.created_at <= sqlc.arg(now)::timestamptz
  AND s.expires_at > sqlc.arg(now)::timestamptz
  AND s.last_seen_at + interval '7 days' > sqlc.arg(now)::timestamptz
  AND s.created_at + interval '30 days' > sqlc.arg(now)::timestamptz
RETURNING s.user_id, s.expires_at;

-- name: CheckSession :one
SELECT user_id FROM meta.sessions
WHERE id_hash = sqlc.arg(id_hash) AND user_id = sqlc.arg(user_id)
  AND created_at <= sqlc.arg(now)::timestamptz
  AND expires_at > sqlc.arg(now)::timestamptz
  AND last_seen_at + interval '7 days' > sqlc.arg(now)::timestamptz
  AND created_at + interval '30 days' > sqlc.arg(now)::timestamptz
FOR UPDATE;

-- name: RevokeSession :exec
DELETE FROM meta.sessions WHERE id_hash = $1;

-- name: RevokeUserSessions :exec
DELETE FROM meta.sessions WHERE user_id = $1;

-- name: RecordLogin :exec
UPDATE meta.users SET last_login_at = $2 WHERE id = $1;

-- name: ChangePassword :exec
UPDATE meta.users SET password_hash = $2, must_change_password = false, updated_at = $3 WHERE id = $1;

-- name: UpdateProfile :exec
UPDATE meta.users
SET display_name = $2, locale = $3, theme = $4, timezone = $5, updated_at = $6,
    mention_email = $7
WHERE id = $1;

-- name: FindResetUsers :many
SELECT id FROM meta.users
WHERE lower(email) = sqlc.arg(email)::text AND status = 'active'
LIMIT 2;

-- name: StorePasswordReset :exec
INSERT INTO meta.password_resets (user_id, token_hash, expires_at)
VALUES (sqlc.arg(user_id), sqlc.arg(token_hash), sqlc.arg(expires_at))
ON CONFLICT (user_id) DO UPDATE
SET token_hash = EXCLUDED.token_hash, expires_at = EXCLUDED.expires_at;

-- name: FindPasswordReset :one
SELECT user_id FROM meta.password_resets
WHERE token_hash = sqlc.arg(token_hash) AND expires_at > sqlc.arg(now)::timestamptz;

-- name: LockPasswordReset :one
SELECT user_id FROM meta.password_resets
WHERE token_hash = sqlc.arg(token_hash) AND user_id = sqlc.arg(user_id)
  AND expires_at > sqlc.arg(now)::timestamptz
FOR UPDATE;

-- name: DefaultTimezone :one
SELECT COALESCE((SELECT CASE WHEN jsonb_typeof(value) = 'string' THEN value #>> '{}' ELSE '' END
                 FROM meta.settings WHERE key = 'default_timezone'), 'Asia/Shanghai')::text;

-- name: ValidTimezone :one
SELECT EXISTS (SELECT 1 FROM pg_timezone_names WHERE name = $1 AND name NOT LIKE 'posix/%' AND name NOT LIKE 'right/%');

-- name: SetupNeeded :one
SELECT NOT EXISTS (SELECT 1 FROM meta.users)
   AND NOT EXISTS (SELECT 1 FROM meta.settings WHERE key = 'setup_completed');

-- name: LockSetup :exec
SELECT pg_advisory_xact_lock(hashtextextended('pandatable.setup', 0));

-- name: CreateSetupUser :one
INSERT INTO meta.users (username, email, display_name, password_hash, role, locale,
      avatar_color, last_login_at, updated_at)
VALUES (sqlc.arg(username), sqlc.arg(email), sqlc.arg(display_name), sqlc.arg(password_hash),
  'admin', sqlc.arg(locale), sqlc.arg(avatar_color), sqlc.arg(now)::timestamptz,
  sqlc.arg(now)::timestamptz)
RETURNING id;

-- name: CompleteSetup :exec
INSERT INTO meta.settings (key, value) VALUES ('setup_completed', 'true'::jsonb);

-- name: Timezones :many
SELECT name::text FROM pg_timezone_names WHERE name NOT LIKE 'posix/%' AND name NOT LIKE 'right/%' ORDER BY name;
