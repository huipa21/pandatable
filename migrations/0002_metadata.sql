-- +goose Up
CREATE TABLE meta.groups (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  name        text NOT NULL UNIQUE,
  description text NOT NULL DEFAULT '',
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE meta.group_members (
  group_id uuid NOT NULL REFERENCES meta.groups(id) ON DELETE CASCADE,
  user_id  uuid NOT NULL REFERENCES meta.users(id) ON DELETE CASCADE,
  PRIMARY KEY (group_id, user_id)
);
CREATE INDEX group_members_user_id_idx ON meta.group_members(user_id);

CREATE TABLE meta.workspaces (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  name        text NOT NULL,
  icon        text,
  color       text,
  created_by  uuid REFERENCES meta.users(id) ON DELETE SET NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  deleted_at  timestamptz,
  deleted_by  uuid REFERENCES meta.users(id) ON DELETE SET NULL
);

CREATE TABLE meta.books (
  id            uuid PRIMARY KEY DEFAULT uuidv7(),
  workspace_id  uuid NOT NULL REFERENCES meta.workspaces(id) ON DELETE CASCADE,
  name          text NOT NULL,
  pg_schema     text NOT NULL UNIQUE,
  position      double precision NOT NULL,
  created_by    uuid REFERENCES meta.users(id) ON DELETE SET NULL,
  created_at    timestamptz NOT NULL DEFAULT now(),
  deleted_at    timestamptz,
  deleted_by    uuid REFERENCES meta.users(id) ON DELETE SET NULL
);
CREATE INDEX books_workspace_id_idx ON meta.books(workspace_id);

CREATE TABLE meta.tables (
  id                uuid PRIMARY KEY DEFAULT uuidv7(),
  book_id           uuid NOT NULL REFERENCES meta.books(id) ON DELETE CASCADE,
  name              text NOT NULL,
  pg_name           text NOT NULL,
  display_field_id  uuid,
  position          double precision NOT NULL,
  schema_version    integer NOT NULL DEFAULT 1 CHECK (schema_version > 0),
  created_by        uuid REFERENCES meta.users(id) ON DELETE SET NULL,
  created_at        timestamptz NOT NULL DEFAULT now(),
  deleted_at        timestamptz,
  deleted_by        uuid REFERENCES meta.users(id) ON DELETE SET NULL,
  UNIQUE (book_id, pg_name),
  UNIQUE (book_id, id)
);

CREATE TABLE meta.fields (
  id           uuid PRIMARY KEY DEFAULT uuidv7(),
  table_id     uuid NOT NULL REFERENCES meta.tables(id) ON DELETE CASCADE,
  name         text NOT NULL,
  description  text NOT NULL DEFAULT '',
  ui_type      text NOT NULL,
  pg_column    text,
  pg_type      text,
  options      jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(options) = 'object'),
  is_system    boolean NOT NULL DEFAULT false,
  is_required  boolean NOT NULL DEFAULT false,
  is_unique    boolean NOT NULL DEFAULT false,
  default_json jsonb,
  position     double precision NOT NULL,
  version      integer NOT NULL DEFAULT 1 CHECK (version > 0),
  created_at   timestamptz NOT NULL DEFAULT now(),
  deleted_at   timestamptz,
  UNIQUE (table_id, id),
  UNIQUE (table_id, pg_column)
);
CREATE UNIQUE INDEX fields_table_id_lower_name_idx ON meta.fields(table_id, lower(name)) WHERE deleted_at IS NULL;
ALTER TABLE meta.tables ADD CONSTRAINT tables_display_field_fk
  FOREIGN KEY (id, display_field_id) REFERENCES meta.fields(table_id, id)
  ON DELETE SET NULL (display_field_id);

CREATE TABLE meta.relations (
  id              uuid PRIMARY KEY DEFAULT uuidv7(),
  book_id         uuid NOT NULL,
  kind            text NOT NULL CHECK (kind IN ('one_one','one_many','many_many')),
  parent_table_id uuid NOT NULL,
  child_table_id  uuid NOT NULL,
  parent_field_id uuid NOT NULL,
  child_field_id  uuid NOT NULL,
  fk_column       text,
  junction_table  text,
  on_delete       text NOT NULL DEFAULT 'set_null' CHECK (on_delete IN ('set_null','cascade','restrict')),
  created_at      timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (book_id, parent_table_id) REFERENCES meta.tables(book_id, id) ON DELETE CASCADE,
  FOREIGN KEY (book_id, child_table_id) REFERENCES meta.tables(book_id, id) ON DELETE CASCADE,
  FOREIGN KEY (parent_table_id, parent_field_id) REFERENCES meta.fields(table_id, id) ON DELETE CASCADE,
  FOREIGN KEY (child_table_id, child_field_id) REFERENCES meta.fields(table_id, id) ON DELETE CASCADE,
  CHECK (parent_field_id <> child_field_id),
  CHECK (
    (kind IN ('one_one','one_many') AND fk_column IS NOT NULL AND junction_table IS NULL)
    OR (kind = 'many_many' AND fk_column IS NULL AND junction_table IS NOT NULL)
  ),
  UNIQUE (book_id, junction_table),
  UNIQUE (child_table_id, fk_column)
);
CREATE INDEX relations_parent_field_idx ON meta.relations(parent_table_id, parent_field_id);
CREATE INDEX relations_child_field_idx ON meta.relations(child_table_id, child_field_id);

CREATE TABLE meta.views (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  table_id    uuid NOT NULL REFERENCES meta.tables(id) ON DELETE CASCADE,
  name        text NOT NULL,
  type        text NOT NULL CHECK (type IN ('grid','form','kanban','gallery','calendar')),
  scope       text NOT NULL CHECK (scope IN ('shared','personal')),
  owner_id    uuid REFERENCES meta.users(id) ON DELETE CASCADE,
  locked      boolean NOT NULL DEFAULT false,
  position    double precision NOT NULL,
  config      jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(config) = 'object'),
  version     integer NOT NULL DEFAULT 1 CHECK (version > 0),
  created_by  uuid REFERENCES meta.users(id) ON DELETE SET NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  deleted_at  timestamptz,
  CHECK ((scope = 'shared' AND owner_id IS NULL) OR (scope = 'personal' AND owner_id IS NOT NULL)),
  CHECK (NOT locked OR scope = 'shared')
);
CREATE INDEX views_table_id_idx ON meta.views(table_id);
CREATE INDEX views_owner_id_idx ON meta.views(owner_id);

-- Polymorphic references are validated/removed by future services (D-070).
CREATE TABLE meta.grants (
  id              uuid PRIMARY KEY DEFAULT uuidv7(),
  resource_type   text NOT NULL CHECK (resource_type IN ('workspace','book','table')),
  resource_id     uuid NOT NULL,
  principal_type  text NOT NULL CHECK (principal_type IN ('user','group')),
  principal_id    uuid NOT NULL,
  level           text NOT NULL CHECK (level IN ('read','write','owner')),
  granted_by      uuid REFERENCES meta.users(id) ON DELETE SET NULL,
  created_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (resource_type, resource_id, principal_type, principal_id)
);
CREATE INDEX grants_principal_idx ON meta.grants(principal_type, principal_id);

CREATE TABLE meta.comments (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  table_id    uuid NOT NULL REFERENCES meta.tables(id) ON DELETE CASCADE,
  row_id      bigint NOT NULL,
  user_id     uuid REFERENCES meta.users(id) ON DELETE SET NULL,
  body        text NOT NULL,
  mentions    uuid[] NOT NULL DEFAULT '{}',
  created_at  timestamptz NOT NULL DEFAULT now(),
  edited_at   timestamptz,
  deleted_at  timestamptz
);
CREATE INDEX comments_table_row_idx ON meta.comments(table_id, row_id);

CREATE TABLE meta.notifications (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  user_id     uuid NOT NULL REFERENCES meta.users(id) ON DELETE CASCADE,
  kind        text NOT NULL,
  payload     jsonb NOT NULL,
  read_at     timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX notifications_user_read_idx ON meta.notifications(user_id, read_at);

CREATE TABLE meta.row_history (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  table_id    uuid NOT NULL REFERENCES meta.tables(id) ON DELETE CASCADE,
  row_id      bigint,
  user_id     uuid REFERENCES meta.users(id) ON DELETE SET NULL,
  action      text NOT NULL CHECK (action IN ('insert','update','delete','link','unlink','restore','type_change')),
  changes     jsonb NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  CHECK ((action = 'type_change') = (row_id IS NULL))
);
CREATE INDEX row_history_table_row_id_idx ON meta.row_history(table_id, row_id, id DESC);
CREATE INDEX row_history_created_at_idx ON meta.row_history(created_at);

CREATE TABLE meta.attachments (
  id           uuid PRIMARY KEY DEFAULT uuidv7(),
  table_id     uuid NOT NULL REFERENCES meta.tables(id) ON DELETE CASCADE,
  field_id     uuid NOT NULL,
  file_name    text NOT NULL,
  mime_type    text NOT NULL,
  size_bytes   bigint NOT NULL CHECK (size_bytes >= 0),
  sha256       bytea NOT NULL CHECK (octet_length(sha256) = 32),
  storage_path text NOT NULL,
  has_thumb    boolean NOT NULL DEFAULT false,
  uploaded_by  uuid REFERENCES meta.users(id) ON DELETE SET NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (table_id, field_id) REFERENCES meta.fields(table_id, id) ON DELETE CASCADE
);
CREATE INDEX attachments_table_field_idx ON meta.attachments(table_id, field_id);

CREATE TABLE meta.backups (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  file_name   text NOT NULL UNIQUE,
  size_bytes  bigint CHECK (size_bytes >= 0),
  status      text NOT NULL CHECK (status IN ('running','done','failed')),
  error       text,
  created_by  uuid REFERENCES meta.users(id) ON DELETE SET NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  finished_at timestamptz
);

-- +goose Down
DROP TABLE meta.backups;
DROP TABLE meta.attachments;
DROP TABLE meta.row_history;
DROP TABLE meta.notifications;
DROP TABLE meta.comments;
DROP TABLE meta.grants;
DROP TABLE meta.views;
DROP TABLE meta.relations;
ALTER TABLE meta.tables DROP CONSTRAINT tables_display_field_fk;
DROP TABLE meta.fields;
DROP TABLE meta.tables;
DROP TABLE meta.books;
DROP TABLE meta.workspaces;
DROP TABLE meta.group_members;
DROP TABLE meta.groups;
