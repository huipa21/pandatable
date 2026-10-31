-- +goose Up
ALTER TABLE meta.row_history
  ADD COLUMN restore_system jsonb
  CHECK (restore_system IS NULL OR
         (action = 'delete' AND jsonb_typeof(restore_system) = 'object'));

-- +goose Down
ALTER TABLE meta.row_history DROP COLUMN restore_system;
