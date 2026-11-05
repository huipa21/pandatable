-- +goose Up
CREATE TABLE meta.user_workspace_preferences (
  user_id      uuid PRIMARY KEY REFERENCES meta.users(id) ON DELETE CASCADE,
  workspace_id uuid REFERENCES meta.workspaces(id) ON DELETE SET NULL
);
CREATE INDEX user_workspace_preferences_workspace_idx
  ON meta.user_workspace_preferences(workspace_id) WHERE workspace_id IS NOT NULL;

-- +goose Down
DROP TABLE meta.user_workspace_preferences;
