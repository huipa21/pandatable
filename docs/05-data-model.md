# 05 — Data Model

Two layers:

1. **`meta` schema** — application metadata (users, workspaces, fields, views, grants, …). Managed by goose migrations.
2. **Book schemas `b_<id>`** — one physical PostgreSQL table per user table, created/altered at runtime by `internal/ddl`.

## 1. Identifiers & naming (D-002)

| Object | Meta ID | Physical name |
|---|---|---|
| Workspace | `uuid` (uuidv7) | — |
| Book | `uuid` | schema `b_<short>` |
| Table | `uuid` | table `b_<short>.t_<short>` |
| Field | `uuid` | column `c_<short>` |
| M:N junction | — | table `b_<short>.j_<short>` |
| Auto-number seq | — | sequence `b_<short>.s_<short>` |

`<short>` = 10-char lowercase base32 from a random 50-bit value, generated once, stored in meta (`pg_name`) and **never derived from user input**. Renames are metadata-only. Row IDs are `bigint` identities (stable, compact, good for URLs and links).

API uses meta UUIDs for structure and bigint `id` for rows.

## 2. Meta schema

```sql
CREATE SCHEMA meta;

-- ── Users & auth ───────────────────────────────────────────────
CREATE TABLE meta.users (
  id                    uuid PRIMARY KEY DEFAULT uuidv7(),
  username              text NOT NULL UNIQUE,         -- lowercase, [a-z0-9._-]{3,32}
  email                 text UNIQUE,                  -- optional (SMTP later)
  display_name          text NOT NULL,
  password_hash         text NOT NULL,                -- argon2id PHC string
  role                  text NOT NULL CHECK (role IN ('admin','user')),
  status                text NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
  must_change_password  boolean NOT NULL DEFAULT false,
  locale                text NOT NULL DEFAULT 'en' CHECK (locale IN ('en','zh-CN')),
  theme                 text NOT NULL DEFAULT 'system' CHECK (theme IN ('light','dark','system')),
  timezone              text,                         -- IANA name; NULL = app default_timezone (Asia/Shanghai)
  avatar_color          text NOT NULL,
  last_login_at         timestamptz,
  created_at            timestamptz NOT NULL DEFAULT now(),
  updated_at            timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE meta.sessions (
  id_hash       bytea PRIMARY KEY,        -- sha256(token); token only in cookie
  user_id       uuid NOT NULL REFERENCES meta.users(id) ON DELETE CASCADE,
  created_at    timestamptz NOT NULL DEFAULT now(),
  last_seen_at  timestamptz NOT NULL DEFAULT now(),
  expires_at    timestamptz NOT NULL,
  ip            inet,
  user_agent    text
);
CREATE INDEX ON meta.sessions(user_id);

CREATE TABLE meta.groups (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  name        text NOT NULL UNIQUE,
  description text NOT NULL DEFAULT '',
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE meta.group_members (
  group_id uuid NOT NULL REFERENCES meta.groups(id) ON DELETE CASCADE,
  user_id  uuid NOT NULL REFERENCES meta.users(id)  ON DELETE CASCADE,
  PRIMARY KEY (group_id, user_id)
);
CREATE INDEX ON meta.group_members(user_id);

-- ── Structure ──────────────────────────────────────────────────
CREATE TABLE meta.workspaces (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  name        text NOT NULL,
  icon        text,
  color       text,
  created_by  uuid REFERENCES meta.users(id) ON DELETE SET NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  deleted_at  timestamptz,                -- trash
  deleted_by  uuid REFERENCES meta.users(id) ON DELETE SET NULL
);

CREATE TABLE meta.books (
  id            uuid PRIMARY KEY DEFAULT uuidv7(),
  workspace_id  uuid NOT NULL REFERENCES meta.workspaces(id) ON DELETE CASCADE,
  name          text NOT NULL,
  pg_schema     text NOT NULL UNIQUE,     -- b_xxxxxxxxxx
  position      double precision NOT NULL,
  created_by    uuid REFERENCES meta.users(id) ON DELETE SET NULL,
  created_at    timestamptz NOT NULL DEFAULT now(),
  deleted_at    timestamptz,
  deleted_by    uuid REFERENCES meta.users(id) ON DELETE SET NULL
);

CREATE TABLE meta.tables (
  id                uuid PRIMARY KEY DEFAULT uuidv7(),
  book_id           uuid NOT NULL REFERENCES meta.books(id) ON DELETE CASCADE,
  name              text NOT NULL,
  pg_name           text NOT NULL,        -- t_xxxxxxxxxx (unique within schema)
  display_field_id  uuid,                 -- FK added after meta.fields
  position          double precision NOT NULL,
  schema_version    integer NOT NULL DEFAULT 1,   -- bumped on any field/relation change
  created_by        uuid REFERENCES meta.users(id) ON DELETE SET NULL,
  created_at        timestamptz NOT NULL DEFAULT now(),
  deleted_at        timestamptz,
  deleted_by        uuid REFERENCES meta.users(id) ON DELETE SET NULL
);

CREATE TABLE meta.fields (
  id           uuid PRIMARY KEY DEFAULT uuidv7(),
  table_id     uuid NOT NULL REFERENCES meta.tables(id) ON DELETE CASCADE,
  name         text NOT NULL,
  description  text NOT NULL DEFAULT '',
  ui_type      text NOT NULL,             -- see 07-field-types.md
  pg_column    text,                      -- NULL for virtual fields (formula/lookup/rollup, link on non-FK side)
  pg_type      text,                      -- e.g. 'numeric(12,2)', 'text[]'
  options      jsonb NOT NULL DEFAULT '{}',  -- type-specific options
  is_system    boolean NOT NULL DEFAULT false,
  is_required  boolean NOT NULL DEFAULT false,
  is_unique    boolean NOT NULL DEFAULT false,
  default_json jsonb,                     -- default value (typed via codec)
  position     double precision NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  deleted_at   timestamptz                -- soft-deleted: column kept until purge
);
CREATE UNIQUE INDEX ON meta.fields(table_id, lower(name)) WHERE deleted_at IS NULL;
ALTER TABLE meta.tables ADD FOREIGN KEY (display_field_id) REFERENCES meta.fields(id) ON DELETE SET NULL;

CREATE TABLE meta.relations (
  id              uuid PRIMARY KEY DEFAULT uuidv7(),
  kind            text NOT NULL CHECK (kind IN ('one_one','one_many','many_many')),
  -- "one" side = parent, "many" side = child (for one_one: child holds FK)
  parent_table_id uuid NOT NULL REFERENCES meta.tables(id) ON DELETE CASCADE,
  child_table_id  uuid NOT NULL REFERENCES meta.tables(id) ON DELETE CASCADE,
  parent_field_id uuid NOT NULL REFERENCES meta.fields(id) ON DELETE CASCADE, -- link field shown in parent
  child_field_id  uuid NOT NULL REFERENCES meta.fields(id) ON DELETE CASCADE, -- link field shown in child
  fk_column       text,                   -- in child table, for one_one/one_many
  junction_table  text,                   -- for many_many
  on_delete       text NOT NULL DEFAULT 'set_null' CHECK (on_delete IN ('set_null','cascade','restrict')),
  created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE meta.views (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  table_id    uuid NOT NULL REFERENCES meta.tables(id) ON DELETE CASCADE,
  name        text NOT NULL,
  type        text NOT NULL CHECK (type IN ('grid','form','kanban','gallery','calendar')),
  scope       text NOT NULL CHECK (scope IN ('shared','personal')),
  owner_id    uuid REFERENCES meta.users(id) ON DELETE CASCADE,  -- personal views
  locked      boolean NOT NULL DEFAULT false,
  position    double precision NOT NULL,
  config      jsonb NOT NULL DEFAULT '{}',   -- see 08-query-engine.md
  version     integer NOT NULL DEFAULT 1,    -- optimistic concurrency for config edits
  created_by  uuid REFERENCES meta.users(id) ON DELETE SET NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  deleted_at  timestamptz
);

-- ── Permissions ────────────────────────────────────────────────
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
CREATE INDEX ON meta.grants(principal_type, principal_id);

-- ── Collaboration ──────────────────────────────────────────────
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
CREATE INDEX ON meta.comments(table_id, row_id);

CREATE TABLE meta.notifications (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  user_id     uuid NOT NULL REFERENCES meta.users(id) ON DELETE CASCADE,
  kind        text NOT NULL,              -- 'mention', 'ownership_transferred', ...
  payload     jsonb NOT NULL,
  read_at     timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON meta.notifications(user_id, read_at);

CREATE TABLE meta.row_history (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  table_id    uuid NOT NULL,
  row_id      bigint NOT NULL,
  user_id     uuid REFERENCES meta.users(id) ON DELETE SET NULL,
  action      text NOT NULL CHECK (action IN ('insert','update','delete','link','unlink','restore')),
  changes     jsonb NOT NULL,             -- {fieldId: {"o": old, "n": new}}
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON meta.row_history(table_id, row_id, id DESC);
CREATE INDEX ON meta.row_history(created_at);    -- retention purge

-- ── Files ──────────────────────────────────────────────────────
CREATE TABLE meta.attachments (
  id           uuid PRIMARY KEY DEFAULT uuidv7(),
  table_id     uuid NOT NULL REFERENCES meta.tables(id) ON DELETE CASCADE,
  field_id     uuid NOT NULL REFERENCES meta.fields(id) ON DELETE CASCADE,
  file_name    text NOT NULL,
  mime_type    text NOT NULL,
  size_bytes   bigint NOT NULL,
  sha256       bytea NOT NULL,
  storage_path text NOT NULL,             -- relative to PT_DATA_DIR/files
  has_thumb    boolean NOT NULL DEFAULT false,
  uploaded_by  uuid REFERENCES meta.users(id) ON DELETE SET NULL,
  created_at   timestamptz NOT NULL DEFAULT now()
);

-- ── Admin ──────────────────────────────────────────────────────
CREATE TABLE meta.settings (
  key    text PRIMARY KEY,
  value  jsonb NOT NULL
);
-- keys: app_name, default_timezone ('Asia/Shanghai'), default_currency ('CNY'), default_locale ('en'),
--       upload_max_mb, history_retention_days, trash_retention_days, setup_completed
-- timezone values validated against PostgreSQL pg_timezone_names

CREATE TABLE meta.backups (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  file_name   text NOT NULL UNIQUE,
  size_bytes  bigint,
  status      text NOT NULL CHECK (status IN ('running','done','failed')),
  error       text,
  created_by  uuid REFERENCES meta.users(id) ON DELETE SET NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  finished_at timestamptz
);
```

## 3. Physical user table layout

```sql
CREATE SCHEMA b_k3j9x0a2mq;

CREATE TABLE b_k3j9x0a2mq.t_8fh2kd9s0p (
  id          bigint GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,  -- BY DEFAULT: allows undo/restore re-insert with same id
  created_at  timestamptz NOT NULL DEFAULT now(),
  created_by  uuid,          -- no FK (users may be hard-deleted; shown as "Deleted user")
  updated_at  timestamptz NOT NULL DEFAULT now(),
  updated_by  uuid,
  -- user fields:
  c_a1b2c3d4e5  text,
  c_f6g7h8i9j0  numeric(12,2),
  c_k1l2m3n4o5  text[],                -- multi select
  c_p6q7r8s9t0  bigint REFERENCES b_k3j9x0a2mq.t_parent(id) ON DELETE SET NULL  -- link FK (child side)
);
```

- System fields map to `id, created_at, created_by, updated_at, updated_by` and exist in `meta.fields` with `is_system=true` (hidden by default in new views).
- `updated_at/updated_by` set by the app in every UPDATE (no triggers — keeps writes explicit and cheap).
- Indexes: PK; every FK column; auto-created for `is_unique`. Owners can't create arbitrary indexes in v1; the engine adds a btree index on a field when it's used in a shared view's sort/group *and* the table has > 10k rows (background, `CREATE INDEX CONCURRENTLY`). See D-020.

### M:N junction

```sql
CREATE TABLE b_x.j_<short> (
  parent_id bigint NOT NULL REFERENCES b_x.t_parent(id) ON DELETE CASCADE,
  child_id  bigint NOT NULL REFERENCES b_x.t_child(id)  ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  created_by uuid,
  PRIMARY KEY (parent_id, child_id)
);
CREATE INDEX ON b_x.j_<short>(child_id);
```
For M:N, `on_delete` = `cascade` deletes the linked rows on the other side (app-level, in same tx); `restrict` blocks deletion if links exist; `set_null` just removes junction rows (default).

## 4. Trash & purge

- Soft-deleted objects: workspace, book, table, field, view (`deleted_at`).
- Soft-deleted field: column stays physically; excluded from queries; name freed.
- Purge job (daily): objects older than `trash_retention_days` (default 30) → DROP SCHEMA / DROP TABLE / ALTER TABLE DROP COLUMN, delete attachments files, delete meta rows.
- Rows are hard-deleted (undo within session re-inserts using the same id — see 10-collaboration.md).

## 5. Transactions & DDL

Every structural change = **one transaction** containing both meta changes and DDL, with `SET LOCAL lock_timeout = '3s'`. On success → `schema_version++` and NOTIFY `schema.changed`. PostgreSQL's transactional DDL guarantees meta and physical schema never diverge.

A startup self-check (`pandatable doctor`, also run at boot in warn-only mode) compares meta vs `information_schema` and reports drift.
