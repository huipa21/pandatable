-- name: FieldForUpload :one
SELECT f.options
FROM meta.fields f
JOIN meta.tables t ON t.id = f.table_id AND t.deleted_at IS NULL
WHERE f.id = $1 AND f.table_id = $2 AND f.ui_type = 'attachment' AND f.deleted_at IS NULL;

-- name: Insert :one
INSERT INTO meta.attachments (id, table_id, field_id, file_name, mime_type, size_bytes, sha256,
                              storage_path, has_thumb, uploaded_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id, table_id, field_id, file_name, mime_type, size_bytes, sha256, storage_path, has_thumb;

-- name: Get :one
SELECT id, table_id, field_id, file_name, mime_type, size_bytes, sha256, storage_path, has_thumb
FROM meta.attachments
WHERE id = $1;

-- name: FieldStorage :one
SELECT b.pg_schema, t.pg_name, f.pg_column
FROM meta.fields f
JOIN meta.tables t ON t.id = f.table_id AND t.deleted_at IS NULL
JOIN meta.books b ON b.id = t.book_id AND b.deleted_at IS NULL
WHERE f.id = $1 AND f.table_id = $2 AND f.ui_type = 'attachment' AND f.deleted_at IS NULL;

-- name: Delete :exec
DELETE FROM meta.attachments
WHERE id = $1 AND table_id = $2 AND field_id = $3;