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

T1.9/D-094 fixes the exact alphabet to RFC 4648 lowercase `a-z2-7`.
Each operation checks its own
`b_`, `t_`, `c_`, `j_` or `s_` prefix plus **exactly ten** alphabet characters.
Both new and stored identifiers use that strict validation; older illustrative
names below are not authoritative grammar (some contain `0/1/8/9`).
`ddl.NewIdentifier` uses crypto/rand, never display names or UUID truncation.
No catalog lookup or silent regeneration: a physical creation collision returns
`errors.Is(err, ddl.ErrIdentifierCollision)`. The future service distinguishes
physical-name metadata unique constraints from display-name conflicts and may
retry at most three **whole** transactions with fresh names. Existing objects
are never renamed and trashed physical-name reservations are never reused.

## 2. Meta schema

The SQL below describes the current schema; the executable source is
[`0001_initial.sql`](../migrations/0001_initial.sql) plus the forward-only
[`0002_metadata.sql`](../migrations/0002_metadata.sql). Never edit an applied
migration. See D-069..D-071 for the T1.1 integrity clarifications.

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
  schema_version    integer NOT NULL DEFAULT 1 CHECK (schema_version > 0), -- bumped on any field/relation change
  created_by        uuid REFERENCES meta.users(id) ON DELETE SET NULL,
  created_at        timestamptz NOT NULL DEFAULT now(),
  deleted_at        timestamptz,
  deleted_by        uuid REFERENCES meta.users(id) ON DELETE SET NULL,
  UNIQUE (book_id, pg_name),
  UNIQUE (book_id, id)                    -- composite FK target for same-book relations
);

CREATE TABLE meta.fields (
  id           uuid PRIMARY KEY DEFAULT uuidv7(),
  table_id     uuid NOT NULL REFERENCES meta.tables(id) ON DELETE CASCADE,
  name         text NOT NULL,
  description  text NOT NULL DEFAULT '',
  ui_type      text NOT NULL,             -- see 07-field-types.md
  pg_column    text,                      -- NULL for virtual fields (formula/lookup/rollup, link on non-FK side)
  pg_type      text,                      -- e.g. 'numeric(12,2)', 'text[]'
  options      jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(options) = 'object'),
  is_system    boolean NOT NULL DEFAULT false,
  is_required  boolean NOT NULL DEFAULT false,
  is_unique    boolean NOT NULL DEFAULT false,
  default_json jsonb,                     -- default value (typed via codec)
  position     double precision NOT NULL,
  version      integer NOT NULL DEFAULT 1 CHECK (version > 0), -- API If-Match
  created_at   timestamptz NOT NULL DEFAULT now(),
  deleted_at   timestamptz,               -- soft-deleted: column kept until purge
  UNIQUE (table_id, id),
  UNIQUE (table_id, pg_column)
);
CREATE UNIQUE INDEX ON meta.fields(table_id, lower(name)) WHERE deleted_at IS NULL;
ALTER TABLE meta.tables ADD CONSTRAINT tables_display_field_fk
  FOREIGN KEY (id, display_field_id) REFERENCES meta.fields(table_id, id)
  ON DELETE SET NULL (display_field_id);

CREATE TABLE meta.relations (
  id              uuid PRIMARY KEY DEFAULT uuidv7(),
  book_id         uuid NOT NULL,
  kind            text NOT NULL CHECK (kind IN ('one_one','one_many','many_many')),
  -- "one" side = parent, "many" side = child (for one_one: child holds FK)
  parent_table_id uuid NOT NULL,
  child_table_id  uuid NOT NULL,
  parent_field_id uuid NOT NULL,           -- link field shown in parent
  child_field_id  uuid NOT NULL,           -- link field shown in child
  fk_column       text,                   -- in child table, for one_one/one_many
  junction_table  text,                   -- for many_many
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

CREATE TABLE meta.views (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  table_id    uuid NOT NULL REFERENCES meta.tables(id) ON DELETE CASCADE,
  name        text NOT NULL,
  type        text NOT NULL CHECK (type IN ('grid','form','kanban','gallery','calendar')),
  scope       text NOT NULL CHECK (scope IN ('shared','personal')),
  owner_id    uuid REFERENCES meta.users(id) ON DELETE CASCADE,  -- personal views
  locked      boolean NOT NULL DEFAULT false,
  position    double precision NOT NULL,
  config      jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(config) = 'object'), -- see 08-query-engine.md
  version     integer NOT NULL DEFAULT 1 CHECK (version > 0),
  created_by  uuid REFERENCES meta.users(id) ON DELETE SET NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  deleted_at  timestamptz,
  CHECK ((scope = 'shared' AND owner_id IS NULL) OR (scope = 'personal' AND owner_id IS NOT NULL)),
  CHECK (NOT locked OR scope = 'shared')
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
  table_id    uuid NOT NULL REFERENCES meta.tables(id) ON DELETE CASCADE,
  row_id      bigint,                     -- NULL only for table-level type_change
  user_id     uuid REFERENCES meta.users(id) ON DELETE SET NULL,
  action      text NOT NULL CHECK (action IN ('insert','update','delete','link','unlink','restore','type_change')),
  changes     jsonb NOT NULL,             -- {fieldId: {"o": old, "n": new}}
  created_at  timestamptz NOT NULL DEFAULT now(),
  CHECK ((action = 'type_change') = (row_id IS NULL))
);
CREATE INDEX ON meta.row_history(table_id, row_id, id DESC);
CREATE INDEX ON meta.row_history(created_at);    -- retention purge

-- ── Files ──────────────────────────────────────────────────────
CREATE TABLE meta.attachments (
  id           uuid PRIMARY KEY DEFAULT uuidv7(),
  table_id     uuid NOT NULL REFERENCES meta.tables(id) ON DELETE CASCADE,
  field_id     uuid NOT NULL,
  file_name    text NOT NULL,
  mime_type    text NOT NULL,
  size_bytes   bigint NOT NULL CHECK (size_bytes >= 0),
  sha256       bytea NOT NULL CHECK (octet_length(sha256) = 32),
  storage_path text NOT NULL,             -- relative to PT_DATA_DIR/files
  has_thumb    boolean NOT NULL DEFAULT false,
  uploaded_by  uuid REFERENCES meta.users(id) ON DELETE SET NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (table_id, field_id) REFERENCES meta.fields(table_id, id) ON DELETE CASCADE
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
  size_bytes  bigint CHECK (size_bytes >= 0),
  status      text NOT NULL CHECK (status IN ('running','done','failed')),
  error       text,
  created_by  uuid REFERENCES meta.users(id) ON DELETE SET NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  finished_at timestamptz
);
```

### T1.8 settings and inventory readers (D-091..D-093)

No migration: the original `meta.settings(key,value jsonb)` already supports
the seven allowlisted keys below. The API uses camelCase names; only present
PATCH fields are upserted, atomically, under the shared admin advisory lock and
actor `FOR NO KEY UPDATE`/session recheck. No settings cache or directory scan.
Corrupt stored types/values fail explicitly; absent rows use defaults.

| Key | JSON type / default | Bounds / existing consumer |
|---|---|---|
| app_name | string / PandaTable | trimmed 1..100 code points, no controls; public header/title |
| default_timezone | string / Asia/Shanghai | exact PG catalog, no posix/right; auth effectiveTimezone when profile NULL |
| default_currency | string / CNY | checked-in ISO 4217; field creation deferred T2.1 |
| default_locale | string / en | en or zh-CN; new admin-created users and non-explicit browser fallback |
| upload_max_mb | integer / 20 | 1..100 MiB; enforcement deferred T2.2 |
| history_retention_days | integer / 90 | 1..3650; consumer deferred T3.4 |
| trash_retention_days | integer / 30 | 1..3650; consumer deferred T3.6 |

Setup's marker remains private/unpatchable. Existing user preferences are not
rewritten; NULL timezone keeps following the current default. Settings are not
permissions, so they neither invalidate ACL nor create grants.

Admin workspace inventory is static sqlc over live workspaces, direct owner
grants and existing principals, with a repeatable-read count/page/owner snapshot.
Disabled/group owner visibility is independent of orphan status, which uses
the same active-direct-user-owner rule as D-023/D-080. Owner DTOs cap at 50 per
workspace plus a total. Recorded attachment bytes include retained descendants
in Trash; no physical-table query, row data or filesystem traversal. Missing
principal targets are omitted, not rescued or repaired by this read-only API.

### Integrity and service boundaries (D-069..D-071)

T1.2 auth and T1.3 setup (D-072..D-078) reuse the original users/sessions/settings
with **no schema migration**. Setup stores only the `setup_completed` boolean
setting after atomically creating the first admin and session. Its one-time
bootstrap token remains process-local; it is not persisted. Session `id_hash` is SHA-256 of a canonical base64url token
made from 32 random bytes. `expires_at` is the earlier of seven days from
`last_seen_at` and 30 days from `created_at`; validation checks both independently,
requires an active user, and sliding uses conditional UPDATE, never UPSERT.
Application connections force UTC. Profile NULL timezone follows the current
validated default setting (Asia/Shanghai when absent). Password change locks
the user and revokes all sessions atomically; login rechecks the hash under the
same user lock before inserting a session. Disable/reset/lifecycle services
are still future work and must use that lock/revocation contract.

- `books.pg_schema` is globally unique; table names are unique within a book,
  field columns within a table, junction names within a book, and relation FK
  columns within a child table. These reservations include trashed metadata:
  physical objects survive soft deletion. Nullable field columns allow multiple
  virtual fields. System fields use their documented fixed column names.
  Identifier generation/validation belongs to T1.9; this migration does not pick
  a new base32 alphabet or create physical book schemas/tables.
- Only active **field** names have case-insensitive uniqueness; deleting a field
  frees its display name, not its physical column name. Group names and backup
  filenames retain ordinary case-sensitive uniqueness. No additional
  workspace/book/table/view display-name uniqueness is imposed by the sketch.
  Restore of a field with a conflicting active name must be resolved by the
  future Trash service, never by overwriting another field.
- Composite FKs enforce display-field/table, attachment-field/table and both
  relation-field/table matches. `relations.book_id` enforces same-book targets
  through two composite table FKs. Self-links use two distinct fields.
  Hard deletion clears only `display_field_id`, never the table ID.
  Soft deletion retains references; future structural services must reassign a
  deleted display field and hide/clean affected link/computed definitions.
  Deleting a relation field cascades the relation record, not the other link
  field or physical DDL; those changes belong in the future service transaction.
- Positive schema/field/view versions, JSON-object field options/view config,
  personal-view ownership (shared views have no owner), shared-only locking,
  relation storage shape, nonnegative file sizes and 32-byte attachment digests
  are checked in PostgreSQL. Full codecs/config validation remains app work.
- Polymorphic `grants.resource_id`/`principal_id` deliberately have **no FK or
  trigger** (user-confirmed D-070). Future grant/lifecycle services must validate
  existence/type and remove affected grants in the same transaction as a hard
  deletion, serializing with competing grant creation. Soft deletion retains
  grants; ACL must hide trashed resources. Direct SQL can leave dangling grants.
  The T1.4 grant service implements the serialization half: every grant
  mutation locks the workspace row `FOR NO KEY UPDATE`, then the book/table
  `FOR KEY SHARE`, then a user/group principal `FOR SHARE`, in that order, each
  as its own statement. Future hard-delete services must `DELETE` the target
  (or principal) row *first* (taking the conflicting lock), then delete
  `meta.grants` rows for that type/id in the same transaction, then invalidate
  the ACL cache after commit.
  Last-owner enforcement, orphan rescue and sole-owned workspace/file purge
  remain service work, not automatic effects of deleting a user row.
- User hard deletion cascades sessions, memberships, notifications and personal
  views. All nullable metadata authorship references become NULL ("Deleted
  user"); shared views/comments/history/attachments/backups survive within
  surviving scopes. Mention arrays and notification payloads are snapshots, not
  FKs. Future reads filter deleted users where appropriate.
- T1.5 disable/enable and role changes are updates, not deletion: user IDs,
  data, grants and authorship stay intact. D-082 serializes user-admin
  mutations with `pandatable.admin.users`, then locks/rechecks the actor and
  locks the target user before writing and revoking all target sessions in the
  same transaction. The last active admin cannot be disabled/demoted; the last
  workspace owner may be disabled (orphaning). Password resets also lock the
  target and revoke all sessions atomically. Role/status writes invalidate ACL
  after commit/uncertain commit; no schema change is needed.
- T1.6 group administration (D-084..D-087) reuses 0001 groups/members without a
  migration. Case-sensitive group uniqueness is retained; service validation
  trims/bounds names and bounds descriptions. Mutations share the D-082 advisory
  lock, use actor `FOR NO KEY UPDATE` and recheck its session, then membership
  target user `FOR UPDATE` and group `FOR KEY SHARE` (one statement per lock).
  Group edits use group `FOR UPDATE`. The narrower actor lock allows grant
  `granted_by` FK checks; the narrower membership group lock allows concurrent
  principal `FOR SHARE` while preventing deletion. Deletion DELETEs the group
  first, cascades memberships, then removes polymorphic grants transactionally.
  Disable retains memberships; adding disabled users is rejected, removal is
  allowed. Membership/deletion commits and uncertain commits invalidate ACL.
- Comments/history reference metadata tables, not physical rows, so row deletion
  does not destroy undo/history context. Table purge cascades both. History's
  `type_change` action has NULL `row_id`; all six row actions require a row ID.
  Retention still applies to both kinds of event.
- In addition to the indexes shown above, 0002 indexes book workspace,
  relation parent/child field pairs, view table/owner and attachment table/field
  lookups; composite unique indexes cover table/field parent traversal and
  polymorphic resource lookup. Authorship-only indexes are not added speculatively.
- Startup remains forward-only under the existing advisory lock. 0002 Down
  explicitly removes its tables and the display-field cycle constraint in
  dependency order, without CASCADE, schema/extension drops or changes to
  0001 users/sessions/settings. Down/up tests run only in Suite-owned databases.

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

### T1.15 row persistence (D-133..D-135)

T1.20's CLI-only developer seed (D-143) creates real metadata and physical
typed rows through these same authorized services, without a migration,
alternate codec or seed HTTP route. It is restricted to an explicitly
confirmed, otherwise workspace-empty disposable database. See the
[seed/workflow operational contract](14-dev-build-release.md#t120-safe-seed-and-production-browser-baseline-d-143d-144)
for deterministic fixture characteristics, atomic-batch/partial-failure
boundaries and owned-only disposal.

Row writes resolve only live non-system field UUIDs through `meta.fields`,
decode each supplied value with its bound codec, and use parameterized SQL
against generated/validated physical identifiers. Inserts omit unspecified
user columns so their physical typed defaults apply; they set `created_at` and
`updated_at` to one transaction timestamp and both authorship columns to the
actor UUID. Patches set `updated_at`/`updated_by` and preserve creation values.
Reads encode every live field (including system fields) through its codec;
deleted fields never enter row I/O. Author UUIDs are returned as UUID values,
not expanded through the owner-only sharing directory.

Mutations use the D-080 workspace/book/table/actor lock order, reauthorize
uncached, then hold a table `FOR SHARE` schema lock for the transaction.
Read/query snapshots also hold `FOR SHARE`; this conflicts with field DDL's
table `FOR NO KEY UPDATE`. A request therefore sees one stable schema or a
sanitized lock/schema-conflict error, never a partially changed column layout.
Bulk insert/patch/delete is one transaction and any invalid cell, constraint
failure, or missing row rolls back the whole request. Row IDs remain canonical
positive bigint decimal strings; there is no persisted row-version column.
Bulk patches lock rows in ascending numeric ID order to prevent overlapping
batches from taking row locks in opposite order, while returning rows in the
request's original order (D-137).

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

### T1.9 caller-transaction contract (D-095/D-096)

`internal/ddl` implements physical operations only. Every function takes
`context.Context` and **pgx.Tx**, never a pool/connection; it neither begins nor
commits/rolls back transactions. After input validation, each operation executes
`SET LOCAL lock_timeout = '3s'`, then DDL on that same transaction. This setting
also applies to subsequent statements until commit/rollback and does not leak to
the pooled session. A service needing a bound on its earlier authorization/
metadata locks must set that bound before those locks itself. Do not catch a
failed DDL and try to commit partial metadata; return the error and roll back.

| Operation | Contract |
|---|---|
| `CreateSchema` | A validated `b_` schema, no IF NOT EXISTS. |
| `CreateTable` | Validated `b_` + `t_`, exactly the five fixed system columns above, named `<table>_pkey`; bigint BY DEFAULT identity. No user fields/metadata/view creation. |
| `AddColumn` | Generated `c_` name, typed native type, optional NOT NULL/UNIQUE/default; no raw type or expression strings. PostgreSQL's ordinary ADD default behavior applies to existing rows. |
| `DropColumn` | Physical purge only, never field soft delete. No CASCADE/IF EXISTS; dependent objects explicitly block removal. |
| `SetNotNull(..., bool)` | SET/DROP NOT NULL; existing NULLs conflict rather than being backfilled. |
| `AddUnique(..., bool)` | ADD/DROP the named `<table>_<column>_key` constraint. Ordinary UNIQUE permits multiple NULLs; duplicates conflict. No concurrent/background index work. |
| `SetDefault` | Set a safe typed literal/dynamic default, or `DefaultValue{}` to DROP DEFAULT. Existing data is never rewritten. |

Alter/drop functions accept generated user-column names only, protecting `id`,
`created_at/by`, `updated_at/by`. Authorship UUIDs are nullable, without physical
FKs; deleted users are interpreted by readers. T1.15 owns explicit updated-at/by
writes. Explicit restored identity IDs do **not** advance the identity sequence;
future restore/import services must handle sequence coordination if necessary.

The closed `DataType` enum covers doc 07 stored native types, including `uuid[]`.
`SQL()` validates unused dimensions and returns only allowlisted, pg_catalog-
qualified syntax: varchar length 1..10485760; numeric precision 1..1000 and
scale 0..precision. Arbitrary SQL types and negative-scale numeric are not an
input surface. Codecs will construct descriptors, not pass metadata `pg_type`
through as SQL.

`DefaultValue` is no-default, `Literal` textual PostgreSQL input, or only
type-matched `Now`/`CurrentDate`/`UUIDv7`. Literals are valid UTF-8 without NUL,
escaped as E-strings (both quotes and backslashes), explicitly cast to the
validated type. Varchar literals exceeding the character bound are rejected
before execution, not silently truncated. PostgreSQL checks remaining literal
representations/ranges; numeric scale follows native semantics. DDL grammar does
not accept ordinary bind placeholders here; this is explicit safe construction,
not purported parameterization. `Literal: "NULL"` is text, not SQL NULL;
no-default removes the expression and leaves normal NULL/NOT NULL behavior.
Current-user defaults remain app-level.

T1.10/T1.14 services still own uncached ACL authorization and resource locking,
metadata rows (including all five `is_system=true` fields), default Grid/Name
display field, schema versions, NOTIFY/event IDs and transaction completion.
Lifecycle commits/uncertain commits must also invalidate ACL. This package does
not implement doctor, orchestration, relation policy, conversion or row APIs.

### T1.10 structure orchestration (D-097..D-103)

`internal/structure` is the service that owns what `internal/ddl` does not; no
migration was needed (0001/0002 are unchanged). Everything below happens in one
transaction under the D-099 lock protocol.

- **Book creation:** a fresh `b_` name is generated, `ddl.CreateSchema` runs and
  `meta.books` is inserted (position = max over all siblings including trashed
  + 1024). Only metadata uniqueness on `pg_schema` or the physical-name
  collision is retried (at most three whole transactions, D-100).
- **Table creation:** `meta.tables` (`schema_version` 1, `pg_name` unique per
  book), `ddl.CreateTable`, one nullable `text` Name column, then the field and
  view rows below. A failure at any step (including deliberately invalid DDL)
  rolls back metadata and physical objects together.

| Field | `ui_type` | `pg_column` / `pg_type` | `is_system` | `is_required` | position |
|---|---|---|---|---|---|
| Name (display field) | `text` | generated `c_…` / `text` | no | no | 1024 |
| ID | `id` | `id` / `bigint` | yes | yes | 1e9 |
| Created at | `created_at` | `created_at` / `timestamptz` | yes | yes | 1e9+1 |
| Created by | `created_by` | `created_by` / `uuid` | yes | no | 1e9+2 |
| Updated at | `updated_at` | `updated_at` / `timestamptz` | yes | yes | 1e9+3 |
| Updated by | `updated_by` | `updated_by` / `uuid` | yes | no | 1e9+4 |

`options` is `{}`, `description` empty, `version` 1, `is_unique` false and
`default_json` NULL (physical defaults belong to the system columns). The
display-field FK points at Name. Field names are the creator's-locale labels
(en: Name, ID, Created at/by, Updated at/by; zh-CN: 名称, ID, 创建时间, 创建人,
更新时间, 更新人) and may be renamed like any display name. User fields added
by T1.14 append after the highest **non-system** position (D-126). T1.14
mutations increment the field `version` and table `schema_version` once per
committed change, store user defaults in `default_json` (canonical wire JSON)
plus a typed physical default, and soft-delete via `deleted_at` while keeping
the physical column (D-124/D-127/D-129). No migration was needed.

The default view is a shared `grid` named Grid (表格 in zh-CN), position 1024,
unlocked, `created_by` the creator, with config
`{"fields":[{id:<Name>,visible:true,width:240},{id:<each system field>,visible:false}…],"frozen":1,"rowHeight":"short"}`.

Display names of workspaces/books/tables are not unique and renames never touch
`pg_schema`, `pg_name` or `pg_column`. Reorders update only `position`;
soft deletes set only `deleted_at/deleted_by` on the target and keep the
physical schema/table/data, descendants' metadata, views and grants. Workspaces
have no `position`; they are listed by `created_at, id`.
