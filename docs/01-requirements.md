# 01 — Requirements

## 1. Vision

A lightweight, self-hosted collaborative table app (NocoDB/Airtable-like) for a team of ~10 people, running comfortably on a 2 vCPU / 4 GB RAM server. Keep the essentials, drop enterprise weight.

## 2. Constraints

| ID | Constraint |
|---|---|
| C-1 | Backend Go, frontend Vue 3, database PostgreSQL 18.x |
| C-2 | Host: 2 vCPU, 4 GB RAM, Ubuntu 24.04 x86_64 (Tencent Lighthouse-class) |
| C-3 | Scale: ≤ ~10 users, ≤ 10 concurrent editors, ≤ 200k rows per table |
| C-4 | Built-in PostgreSQL only; no external DB connectors |
| C-5 | Built locally (Windows laptop with Podman; Docker also supported), image exported as tar and loaded on the server; deployed with Docker Compose (app + Postgres) behind the host's existing independent Caddy container |
| C-6 | No hosted CI; quality gate runs locally (`ptdev check`) |

## 3. Out of Scope (explicitly NOT built)

- Integrations with other apps (webhooks, Slack, automations, plugins, app store)
- External / multiple database types or connectors
- Enterprise audit logs (a lightweight per-row change history IS in scope)
- SSO / OAuth / LDAP / SAML
- Data sync to internal or external sources
- User self-registration
- Public/anonymous share links and public forms
- Personal API tokens / public API (internal REST API only) — v1
- Row-, column- or cell-level permissions
- Full CRDT/OT co-editing
- Email (SMTP) — deferred to a later phase (see roadmap)

## 4. Functional Requirements

### 4.1 Users, roles, admin (FR-U)

| ID | Requirement |
|---|---|
| FR-U1 | App roles: `admin`, `user`. Admin has all normal user functions plus the Admin Panel. Admin Panel is visible only to admins. |
| FR-U2 | No self-registration. Admin creates, edits, disables, re-enables users; resets passwords (user must change at next login). |
| FR-U3 | First run: web setup page creates the first admin; only available while no user exists and protected by a one-time setup token printed to the server log (D-012). |
| FR-U4 | Admin manages groups and group membership. |
| FR-U5 | Disabling a user keeps their data and authorship. Admin can then (a) transfer the user's ownerships to another user, or (b) hard-delete the user **and everything they solely own** (workspaces where they are the only owner, their personal views, comments, attachments they uploaded in deleted scopes). Authorship references on remaining data become "Deleted user". |
| FR-U6 | Admin has **no implicit access** to workspace data. Admin can see the list of workspaces + owners and transfer ownership of orphaned workspaces (no active owner). |
| FR-U7 | Admin dashboard shows CPU, memory, storage (DB size, attachment size, disk free/used). |
| FR-U8 | Admin settings: app name, default timezone (Asia/Shanghai), default currency (CNY), default language, upload size limit, history retention days, trash retention days, backup management. |
| FR-U9 | Every user can toggle language (English / 简体中文) and light/dark theme; both toggles sit side by side in the header; preference persists per user (and in localStorage before login). |
| FR-U10 | Login with username or email + password. Users can change their own password and display name. |
| FR-U11 | Each user can set their own timezone (default = app default timezone, Asia/Shanghai). Datetime display, relative date filters (`today`, `this week`…), formulas `NOW()/TODAY()` and exports use the user's timezone. |

### 4.2 Workspace / Book / Table (FR-S)

| ID | Requirement |
|---|---|
| FR-S1 | Hierarchy: Workspace → Book → Table → View. |
| FR-S2 | Any user can create a workspace and becomes its first owner. |
| FR-S3 | Create/rename/reorder/delete books and tables (owner of the scope). Deleted items go to Trash for 30 days (configurable) and can be restored. |
| FR-S4 | Each table has a configurable display (primary) field. |

### 4.3 Permissions (FR-P) — see [06-permissions.md](06-permissions.md)

| ID | Requirement |
|---|---|
| FR-P1 | Levels: `read`, `write` (read/write), `owner`. Grantable on workspace, book, table to users or groups. |
| FR-P2 | Inheritance downward; lower levels may only **raise** permission. Effective = max of all applicable grants (user + groups, along path). |
| FR-P3 | An owner of a scope can manage grants on that scope and its sub-levels (including granting `owner`). |
| FR-P4 | A workspace must always keep ≥ 1 active user owner. |
| FR-P5 | Schema changes (tables, fields, relations, type changes) require `owner`. `write` users edit rows and shared views. |

### 4.4 Fields (FR-F) — see [07-field-types.md](07-field-types.md)

| ID | Requirement |
|---|---|
| FR-F1 | Field types map to native PostgreSQL 18 types: text, varchar(n), smallint/integer/bigint, numeric(p,s), real/double, boolean, date, time, timestamptz, interval, uuid, jsonb, inet/cidr, text[]/int[]. |
| FR-F2 | UI types on top: single/multi select, email, URL, phone, checkbox, rating, currency, percent, long text, user, multi-user, attachment, auto-number. |
| FR-F3 | Computed: formula, lookup, rollup. |
| FR-F4 | System fields: id, created_at, created_by, updated_at, updated_by — hidden by default, can be shown. |
| FR-F5 | Field type change allowed with dry-run showing number of values that would fail conversion; user chooses abort or set-failed-to-null. |
| FR-F6 | Field constraints: required (NOT NULL), unique, default value. |
| FR-F7 | Attachments stored on local disk volume; admin-configurable size limit. |

### 4.5 Relations (FR-R)

| ID | Requirement |
|---|---|
| FR-R1 | Link field types: 1:1, 1:N (N:1 is the reverse side), M:N. Always bidirectional (both tables get a link field). |
| FR-R2 | Relations only between tables in the same book; self-links allowed. |
| FR-R3 | On-delete behaviour chosen per relation: set null (default), cascade, restrict. |
| FR-R4 | Lookup (show field of linked rows) and rollup (count, count-distinct, sum, avg, min, max, earliest, latest, concat). |

### 4.6 Views (FR-V) — see [08-query-engine.md](08-query-engine.md)

| ID | Requirement |
|---|---|
| FR-V1 | View types: v1 Grid + Form; v2 Kanban + Gallery; v3 Calendar. |
| FR-V2 | View config: field visibility/order/width, frozen columns, filter, sort, group, highlight, row height, footer aggregates. |
| FR-V3 | Filter: nested AND/OR groups up to 3 levels; type-aware operators. |
| FR-V4 | Sort: multi-field. |
| FR-V5 | Group: up to 3 levels, server-side, with counts/aggregates, collapsible. |
| FR-V6 | Highlight: ordered rules (condition → row or cells → color); first match wins per target. |
| FR-V7 | Shared views (visible to all with table access; editable by `write` unless locked by owner) and personal views (creator only). |
| FR-V8 | Quick search across visible fields. |
| FR-V9 | Form view: logged-in users with `write` submit new rows; configurable field labels/help/required. |

### 4.7 Grid editing (FR-G)

| ID | Requirement |
|---|---|
| FR-G1 | Inline editing, keyboard navigation (arrows, Tab, Enter, Esc, Home/End, PageUp/Down). |
| FR-G2 | Copy/paste ranges compatible with Excel / Google Sheets (TSV); paste beyond last row offers to add rows. |
| FR-G3 | Fill-down (Ctrl+D and drag fill handle). |
| FR-G4 | Row expand modal (all fields, comments, history). |
| FR-G5 | Bulk delete selected rows; add row; insert via form. |
| FR-G6 | Undo/redo of the user's last 50 actions in the session. |

### 4.8 Collaboration (FR-C) — see [10-collaboration.md](10-collaboration.md)

| ID | Requirement |
|---|---|
| FR-C1 | Realtime row create/update/delete propagation via WebSocket; last write wins per cell. |
| FR-C2 | Presence: avatars of viewers per table; outline on cell being edited by others. |
| FR-C3 | Row comments with @mentions; in-app notifications. |
| FR-C4 | Per-row change history (who/when/field/old/new), retention default 90 days. |
| FR-C5 | Schema/view changes propagate live to open clients. |

### 4.9 Import / Export / Backup (FR-X) — see [11-import-export-backup.md](11-import-export-backup.md)

| ID | Requirement |
|---|---|
| FR-X1 | Export a view as CSV (data only) or XLSX (data + views + hidden meta sheet describing field types). |
| FR-X2 | Import CSV/XLSX: plain files via mapping wizard + type guessing; PandaTable XLSX (with meta sheet) restores structure and views exactly. Import into new table or append to existing. |
| FR-X3 | Admin backup: create (pg_dump + attachments → zip on server filesystem), list, download, delete. Restore via CLI. |

### 4.10 Non-functional (NFR)

| ID | Requirement |
|---|---|
| NFR-1 | App process RSS < 150 MB under normal load; whole stack fits in 4 GB with headroom. |
| NFR-2 | Grid initial load of a 200k-row table < 1.5 s (first page); scroll page fetch < 300 ms. |
| NFR-3 | Filter/sort/group on 200k rows < 1 s on target host (with indexes where sensible). |
| NFR-4 | Realtime update visible to other clients < 1 s. |
| NFR-5 | Fully responsive: usable editing on mobile (card list + row editor) and desktop (grid). |
| NFR-6 | i18n: English and Simplified Chinese from day one. |
| NFR-7 | Security: argon2id passwords, HttpOnly/Secure/SameSite cookies, CSRF protection, login rate limiting, all SQL identifiers generated (never user-supplied), all values parameterised. |
| NFR-8 | Single Go binary with embedded frontend; one OCI image (linux/amd64) buildable with Podman or Docker, deployable with Docker. |
| NFR-9 | Browsers: latest Chrome, Edge, Firefox, Safari (desktop + iOS/Android). |
