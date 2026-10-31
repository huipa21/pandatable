-- name: ReadTrashRetentionDays :one
SELECT COALESCE(
  (SELECT CASE WHEN jsonb_typeof(value) = 'number' THEN (value #>> '{}')::int ELSE 0 END
   FROM meta.settings WHERE key = 'trash_retention_days'), 30)::int;

-- name: ListTrashItems :many
WITH principals AS (
  SELECT 'user'::text AS kind, u.id FROM meta.users u
  WHERE u.id=sqlc.arg(user_id)::uuid AND u.status='active'
  UNION ALL
  SELECT 'group'::text, gm.group_id FROM meta.group_members gm
  JOIN meta.users u ON u.id=gm.user_id AND u.status='active'
  WHERE gm.user_id=sqlc.arg(user_id)::uuid
), granted AS (
  SELECT g.resource_type, g.resource_id,
         MAX(CASE g.level WHEN 'owner' THEN 3 WHEN 'write' THEN 2 WHEN 'read' THEN 1 ELSE 0 END) AS level
  FROM meta.grants g JOIN principals p ON p.kind=g.principal_type AND p.id=g.principal_id
  GROUP BY g.resource_type, g.resource_id
), items AS (
  SELECT 'workspace'::text AS kind, w.id, w.name, w.id AS workspace_id,
         w.name AS workspace_name, w.id AS parent_id, ''::text AS parent_name,
         w.deleted_at, ''::text AS scope
  FROM meta.workspaces w
  JOIN granted gw ON gw.resource_type='workspace' AND gw.resource_id=w.id AND gw.level=3
  WHERE w.deleted_at >= sqlc.arg(cutoff)::timestamptz
  UNION ALL
  SELECT 'book', b.id, b.name, w.id, w.name, w.id, w.name, b.deleted_at, ''
  FROM meta.books b JOIN meta.workspaces w ON w.id=b.workspace_id AND w.deleted_at IS NULL
  JOIN granted gw ON gw.resource_type='workspace' AND gw.resource_id=w.id AND gw.level=3
  WHERE b.deleted_at >= sqlc.arg(cutoff)::timestamptz
  UNION ALL
  SELECT 'table', t.id, t.name, w.id, w.name, b.id, b.name, t.deleted_at, ''
  FROM meta.tables t JOIN meta.books b ON b.id=t.book_id AND b.deleted_at IS NULL
  JOIN meta.workspaces w ON w.id=b.workspace_id AND w.deleted_at IS NULL
  LEFT JOIN granted gw ON gw.resource_type='workspace' AND gw.resource_id=w.id
  LEFT JOIN granted gb ON gb.resource_type='book' AND gb.resource_id=b.id
  WHERE t.deleted_at >= sqlc.arg(cutoff)::timestamptz AND GREATEST(COALESCE(gw.level,0),COALESCE(gb.level,0))=3
  UNION ALL
  SELECT 'field', f.id, f.name, w.id, w.name, t.id, t.name, f.deleted_at, ''
  FROM meta.fields f JOIN meta.tables t ON t.id=f.table_id AND t.deleted_at IS NULL
  JOIN meta.books b ON b.id=t.book_id AND b.deleted_at IS NULL
  JOIN meta.workspaces w ON w.id=b.workspace_id AND w.deleted_at IS NULL
  LEFT JOIN granted gw ON gw.resource_type='workspace' AND gw.resource_id=w.id
  LEFT JOIN granted gb ON gb.resource_type='book' AND gb.resource_id=b.id
  LEFT JOIN granted gt ON gt.resource_type='table' AND gt.resource_id=t.id
  WHERE f.deleted_at >= sqlc.arg(cutoff)::timestamptz AND NOT f.is_system
    AND GREATEST(COALESCE(gw.level,0),COALESCE(gb.level,0),COALESCE(gt.level,0))=3
  UNION ALL
  SELECT 'view', v.id, v.name, w.id, w.name, t.id, t.name, v.deleted_at, v.scope
  FROM meta.views v JOIN meta.tables t ON t.id=v.table_id AND t.deleted_at IS NULL
  JOIN meta.books b ON b.id=t.book_id AND b.deleted_at IS NULL
  JOIN meta.workspaces w ON w.id=b.workspace_id AND w.deleted_at IS NULL
  LEFT JOIN granted gw ON gw.resource_type='workspace' AND gw.resource_id=w.id
  LEFT JOIN granted gb ON gb.resource_type='book' AND gb.resource_id=b.id
  LEFT JOIN granted gt ON gt.resource_type='table' AND gt.resource_id=t.id
  WHERE v.deleted_at >= sqlc.arg(cutoff)::timestamptz
    AND ((v.scope='shared' AND GREATEST(COALESCE(gw.level,0),COALESCE(gb.level,0),COALESCE(gt.level,0))=3)
      OR (v.scope='personal' AND v.owner_id=sqlc.arg(user_id)::uuid
        AND GREATEST(COALESCE(gw.level,0),COALESCE(gb.level,0),COALESCE(gt.level,0))>=1))
)
SELECT kind, id, name, workspace_id, workspace_name, parent_id, parent_name, deleted_at, scope
FROM items WHERE (sqlc.arg(item_kind)::text='' OR kind=sqlc.arg(item_kind)::text)
  AND (sqlc.narg(item_id)::uuid IS NULL OR id=sqlc.narg(item_id)::uuid)
ORDER BY deleted_at DESC, kind, id;

-- name: LockTrashWorkspace :one
SELECT id FROM meta.workspaces WHERE id=$1 FOR NO KEY UPDATE;

-- name: RestoreTrashWorkspace :exec
UPDATE meta.workspaces SET deleted_at=NULL, deleted_by=NULL WHERE id=$1;

-- name: RestoreTrashBook :exec
UPDATE meta.books SET deleted_at=NULL, deleted_by=NULL WHERE id=$1;

-- name: RestoreTrashTable :exec
UPDATE meta.tables SET deleted_at=NULL, deleted_by=NULL WHERE id=$1;

-- name: RestoreTrashView :one
UPDATE meta.views SET deleted_at=NULL, version=version+1, updated_at=now()
WHERE id=$1 RETURNING version;

-- name: LockTrashField :one
SELECT ui_type, pg_column, options, default_json, trash_required, is_unique, is_system
FROM meta.fields WHERE id=$1 AND table_id=$2 AND deleted_at >= $3 FOR UPDATE;

-- name: RestoreTrashField :exec
UPDATE meta.fields SET deleted_at=NULL, trash_required=NULL, is_required=$2, version=version+1 WHERE id=$1;

-- name: SaveTrashFieldRequired :exec
UPDATE meta.fields SET trash_required=is_required WHERE id=$1 AND deleted_at IS NULL;

-- name: CountRestorableRelationFields :one
SELECT count(*) FROM meta.fields WHERE id=ANY(sqlc.arg(ids)::uuid[]) AND deleted_at >= sqlc.arg(cutoff)::timestamptz;

-- name: RestoreTrashRelationFields :exec
UPDATE meta.fields SET deleted_at=NULL, trash_required=NULL, is_required=false, version=version+1
WHERE id=ANY(sqlc.arg(ids)::uuid[]);

-- name: ReadTrashRelationTable :one
SELECT b.pg_schema,t.pg_name FROM meta.tables t JOIN meta.books b ON b.id=t.book_id WHERE t.id=$1;

-- name: ReadTrashRelationSchema :one
SELECT pg_schema FROM meta.books WHERE id=$1;

-- name: IsFieldVisible :one
SELECT f.ui_type <> 'link' OR EXISTS (
  SELECT 1 FROM meta.relations r
  JOIN meta.fields pf ON pf.id=r.parent_field_id AND pf.deleted_at IS NULL
  JOIN meta.fields cf ON cf.id=r.child_field_id AND cf.deleted_at IS NULL
  JOIN meta.tables pt ON pt.id=r.parent_table_id AND pt.deleted_at IS NULL
  JOIN meta.tables ct ON ct.id=r.child_table_id AND ct.deleted_at IS NULL
  WHERE r.parent_field_id=f.id OR r.child_field_id=f.id)
FROM meta.fields f WHERE f.id=$1 AND f.deleted_at IS NULL;

-- name: ListRelatedLiveSchemas :many
SELECT DISTINCT t.id FROM meta.relations r
JOIN meta.tables t ON t.id=CASE WHEN r.parent_table_id=$1 THEN r.child_table_id ELSE r.parent_table_id END
WHERE (r.parent_table_id=$1 OR r.child_table_id=$1) AND t.id<>$1 AND t.deleted_at IS NULL
ORDER BY t.id;

-- name: ListTableRelations :many
SELECT r.* FROM meta.relations r
JOIN meta.fields pf ON pf.id=r.parent_field_id
JOIN meta.fields cf ON cf.id=r.child_field_id
JOIN meta.tables pt ON pt.id=r.parent_table_id
JOIN meta.tables ct ON ct.id=r.child_table_id
WHERE (r.parent_table_id=sqlc.arg(table_id)::uuid OR r.child_table_id=sqlc.arg(table_id)::uuid)
  AND (NOT sqlc.arg(live_only)::boolean OR
       (pf.deleted_at IS NULL AND cf.deleted_at IS NULL AND pt.deleted_at IS NULL AND ct.deleted_at IS NULL))
ORDER BY r.id;

-- name: FindFieldRelation :one
SELECT r.* FROM meta.relations r
JOIN meta.fields pf ON pf.id=r.parent_field_id
JOIN meta.fields cf ON cf.id=r.child_field_id
JOIN meta.tables pt ON pt.id=r.parent_table_id
JOIN meta.tables ct ON ct.id=r.child_table_id
WHERE (r.parent_field_id=sqlc.arg(field_id)::uuid OR r.child_field_id=sqlc.arg(field_id)::uuid)
  AND (NOT sqlc.arg(live_only)::boolean OR
       (pf.deleted_at IS NULL AND cf.deleted_at IS NULL AND pt.deleted_at IS NULL AND ct.deleted_at IS NULL));

-- name: ListExpiredTrash :many
SELECT kind, id, workspace_id FROM (
  SELECT 1 AS priority, 'workspace'::text AS kind, w.id, w.id AS workspace_id
    FROM meta.workspaces w WHERE w.deleted_at < sqlc.arg(cutoff)::timestamptz
  UNION ALL
  SELECT 2, 'book', b.id, b.workspace_id FROM meta.books b WHERE b.deleted_at < sqlc.arg(cutoff)::timestamptz
  UNION ALL
  SELECT 3, 'table', t.id, b.workspace_id FROM meta.tables t
    JOIN meta.books b ON b.id=t.book_id WHERE t.deleted_at < sqlc.arg(cutoff)::timestamptz
  UNION ALL
  SELECT 4, 'field', f.id, b.workspace_id FROM meta.fields f
    JOIN meta.tables t ON t.id=f.table_id JOIN meta.books b ON b.id=t.book_id
    WHERE f.deleted_at < sqlc.arg(cutoff)::timestamptz
  UNION ALL
  SELECT 5, 'view', v.id, b.workspace_id FROM meta.views v
    JOIN meta.tables t ON t.id=v.table_id JOIN meta.books b ON b.id=t.book_id
    WHERE v.deleted_at < sqlc.arg(cutoff)::timestamptz
) candidates ORDER BY priority, id;

-- name: TrashCandidateExpired :one
SELECT EXISTS (
  SELECT 1 FROM meta.workspaces WHERE sqlc.arg(kind)::text='workspace'
    AND id=sqlc.arg(id)::uuid AND deleted_at < sqlc.arg(cutoff)::timestamptz
  UNION ALL SELECT 1 FROM meta.books WHERE sqlc.arg(kind)::text='book'
    AND id=sqlc.arg(id)::uuid AND deleted_at < sqlc.arg(cutoff)::timestamptz
  UNION ALL SELECT 1 FROM meta.tables WHERE sqlc.arg(kind)::text='table'
    AND id=sqlc.arg(id)::uuid AND deleted_at < sqlc.arg(cutoff)::timestamptz
  UNION ALL SELECT 1 FROM meta.fields WHERE sqlc.arg(kind)::text='field'
    AND id=sqlc.arg(id)::uuid AND deleted_at < sqlc.arg(cutoff)::timestamptz
  UNION ALL SELECT 1 FROM meta.views WHERE sqlc.arg(kind)::text='view'
    AND id=sqlc.arg(id)::uuid AND deleted_at < sqlc.arg(cutoff)::timestamptz
);

-- name: TrashHasRetainedDescendants :one
SELECT EXISTS (
  SELECT 1 FROM meta.books b WHERE sqlc.arg(kind)::text='workspace'
    AND b.workspace_id=sqlc.arg(id)::uuid AND b.deleted_at >= sqlc.arg(cutoff)::timestamptz
  UNION ALL SELECT 1 FROM meta.tables t JOIN meta.books b ON b.id=t.book_id
    WHERE ((sqlc.arg(kind)::text='workspace' AND b.workspace_id=sqlc.arg(id)::uuid)
        OR (sqlc.arg(kind)::text='book' AND t.book_id=sqlc.arg(id)::uuid))
      AND t.deleted_at >= sqlc.arg(cutoff)::timestamptz
  UNION ALL SELECT 1 FROM meta.fields f JOIN meta.tables t ON t.id=f.table_id
    JOIN meta.books b ON b.id=t.book_id
    WHERE ((sqlc.arg(kind)::text='workspace' AND b.workspace_id=sqlc.arg(id)::uuid)
        OR (sqlc.arg(kind)::text='book' AND t.book_id=sqlc.arg(id)::uuid)
        OR (sqlc.arg(kind)::text='table' AND f.table_id=sqlc.arg(id)::uuid))
      AND f.deleted_at >= sqlc.arg(cutoff)::timestamptz
  UNION ALL SELECT 1 FROM meta.views v JOIN meta.tables t ON t.id=v.table_id
    JOIN meta.books b ON b.id=t.book_id
    WHERE ((sqlc.arg(kind)::text='workspace' AND b.workspace_id=sqlc.arg(id)::uuid)
        OR (sqlc.arg(kind)::text='book' AND t.book_id=sqlc.arg(id)::uuid)
        OR (sqlc.arg(kind)::text='table' AND v.table_id=sqlc.arg(id)::uuid))
      AND v.deleted_at >= sqlc.arg(cutoff)::timestamptz
);

-- name: ListPurgeBooks :many
SELECT id, pg_schema FROM meta.books
WHERE (sqlc.arg(kind)::text='workspace' AND workspace_id=sqlc.arg(parent_id)::uuid)
   OR (sqlc.arg(kind)::text='book' AND id=sqlc.arg(parent_id)::uuid)
ORDER BY id;

-- name: ListPurgeTables :many
SELECT t.id, b.pg_schema, t.pg_name FROM meta.tables t JOIN meta.books b ON b.id=t.book_id
WHERE (sqlc.arg(kind)::text='workspace' AND b.workspace_id=sqlc.arg(parent_id)::uuid)
   OR (sqlc.arg(kind)::text='book' AND b.id=sqlc.arg(parent_id)::uuid)
   OR (sqlc.arg(kind)::text='table' AND t.id=sqlc.arg(parent_id)::uuid)
ORDER BY t.id;

-- name: DeletePurgedGrants :exec
DELETE FROM meta.grants WHERE resource_type=sqlc.arg(resource_type)
  AND resource_id=ANY(sqlc.arg(ids)::uuid[]);

-- name: DeletePurgedWorkspace :exec
DELETE FROM meta.workspaces WHERE id=$1;

-- name: DeletePurgedBook :exec
DELETE FROM meta.books WHERE id=$1;

-- name: DeletePurgedTable :exec
DELETE FROM meta.tables WHERE id=$1;

-- name: DeletePurgedView :exec
DELETE FROM meta.views WHERE id=$1;

-- name: QueuePurgedTableAttachments :exec
INSERT INTO meta.attachment_purge_queue (id,storage_path,has_thumb)
SELECT id,storage_path,has_thumb FROM meta.attachments WHERE table_id=ANY(sqlc.arg(ids)::uuid[]);

-- name: QueuePurgedFieldAttachments :exec
INSERT INTO meta.attachment_purge_queue (id,storage_path,has_thumb)
SELECT id,storage_path,has_thumb FROM meta.attachments WHERE field_id=ANY(sqlc.arg(ids)::uuid[]);

-- name: ListPurgeRelations :many
SELECT r.* FROM meta.relations r
WHERE r.parent_table_id=ANY(sqlc.arg(table_ids)::uuid[]) OR r.child_table_id=ANY(sqlc.arg(table_ids)::uuid[])
ORDER BY r.id;

-- name: ListPurgeFieldIDs :many
SELECT id FROM meta.fields WHERE table_id=ANY(sqlc.arg(table_ids)::uuid[]);

-- name: LockPurgeFieldDeletionTime :one
SELECT deleted_at FROM meta.fields WHERE id=$1 FOR UPDATE;

-- name: DeletePurgedRelation :exec
DELETE FROM meta.relations WHERE id=$1;

-- name: DeletePurgedFields :exec
DELETE FROM meta.fields WHERE id=ANY(sqlc.arg(ids)::uuid[]);

-- name: MarkPurgedDependencies :many
UPDATE meta.fields SET
  options=jsonb_set(options,'{error}','"dependency_missing"'::jsonb), version=version+1
WHERE table_id<>ALL(sqlc.arg(table_ids)::uuid[]) AND deleted_at IS NULL AND ui_type IN ('lookup','rollup')
  AND (options->>'relationFieldId'=ANY(sqlc.arg(field_ids)::text[])
       OR options->>'targetFieldId'=ANY(sqlc.arg(field_ids)::text[]))
RETURNING table_id;

-- name: BumpPurgedSchemaVersion :one
UPDATE meta.tables SET schema_version=schema_version+1
WHERE id=$1 AND deleted_at IS NULL RETURNING schema_version;

-- name: LockPurgedField :one
SELECT f.table_id, f.ui_type, f.pg_column, b.pg_schema, t.pg_name
FROM meta.fields f JOIN meta.tables t ON t.id=f.table_id JOIN meta.books b ON b.id=t.book_id
WHERE f.id=$1 FOR UPDATE OF f;

-- name: CountExpiredRelationFields :one
SELECT count(*) FROM meta.fields WHERE id=ANY(sqlc.arg(ids)::uuid[])
  AND deleted_at < sqlc.arg(cutoff)::timestamptz;

-- name: ListQueuedAttachmentRemovals :many
SELECT id, storage_path, has_thumb FROM meta.attachment_purge_queue ORDER BY id;

-- name: AcknowledgeAttachmentRemoval :exec
DELETE FROM meta.attachment_purge_queue WHERE id=$1 AND storage_path=$2 AND has_thumb=$3;
