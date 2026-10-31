-- +goose Up
ALTER TABLE meta.fields ADD COLUMN trash_required boolean;

CREATE TABLE meta.attachment_purge_queue (
  id           uuid PRIMARY KEY,
  storage_path text NOT NULL,
  has_thumb    boolean NOT NULL
);

-- +goose Down
DROP TABLE meta.attachment_purge_queue;
ALTER TABLE meta.fields DROP COLUMN trash_required;
