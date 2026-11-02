-- name: ClaimMentionMail :one
SELECT notification_id, attempts FROM meta.mention_mail_jobs
WHERE next_attempt_at <= sqlc.arg(now)::timestamptz
ORDER BY next_attempt_at, notification_id
LIMIT 1 FOR UPDATE SKIP LOCKED;

-- name: MentionMailTarget :one
SELECT u.id AS user_id, u.email, u.locale, c.table_id
FROM meta.notifications n
JOIN meta.users u ON u.id = n.user_id AND u.status = 'active' AND u.mention_email
JOIN meta.comments c ON c.id::text = n.payload->>'commentId' AND c.deleted_at IS NULL
JOIN meta.tables t ON t.id = c.table_id AND t.deleted_at IS NULL
JOIN meta.books b ON b.id = t.book_id AND b.deleted_at IS NULL
JOIN meta.workspaces w ON w.id = b.workspace_id AND w.deleted_at IS NULL
WHERE n.id = sqlc.arg(notification_id)::uuid AND n.kind = 'mention'
  AND u.email IS NOT NULL AND u.id = ANY(c.mentions);

-- name: DeleteMentionMail :exec
DELETE FROM meta.mention_mail_jobs WHERE notification_id = $1;

-- name: RetryMentionMail :exec
UPDATE meta.mention_mail_jobs
SET attempts = attempts + 1, next_attempt_at = sqlc.arg(next_attempt_at)
WHERE notification_id = sqlc.arg(notification_id);
