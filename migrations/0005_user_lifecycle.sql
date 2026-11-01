-- +goose Up
CREATE TABLE meta.deleted_users (
  id uuid PRIMARY KEY
);

ALTER TABLE meta.attachment_purge_queue ADD COLUMN cleanup_user_id uuid;
CREATE INDEX attachment_purge_queue_cleanup_user_idx
  ON meta.attachment_purge_queue(cleanup_user_id)
  WHERE cleanup_user_id IS NOT NULL;

-- +goose Down
DROP INDEX meta.attachment_purge_queue_cleanup_user_idx;
ALTER TABLE meta.attachment_purge_queue DROP COLUMN cleanup_user_id;
DROP TABLE meta.deleted_users;
