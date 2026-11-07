-- +goose Up
CREATE TABLE meta.recent_resources (
  user_id      uuid NOT NULL REFERENCES meta.users(id) ON DELETE CASCADE,
  workspace_id uuid REFERENCES meta.workspaces(id) ON DELETE CASCADE,
  book_id      uuid REFERENCES meta.books(id) ON DELETE CASCADE,
  opened_at    timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT recent_resources_exactly_one_target CHECK (
    (workspace_id IS NOT NULL) <> (book_id IS NOT NULL)
  )
);

CREATE UNIQUE INDEX recent_resources_user_workspace_key
  ON meta.recent_resources (user_id, workspace_id)
  WHERE workspace_id IS NOT NULL;
CREATE UNIQUE INDEX recent_resources_user_book_key
  ON meta.recent_resources (user_id, book_id)
  WHERE book_id IS NOT NULL;

-- +goose Down
DROP TABLE meta.recent_resources;
