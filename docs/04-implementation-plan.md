# 04 — Detailed Implementation Plan

Task IDs `Tm.n` are referenced in commits and `progress.md`. "AT" = acceptance tests that must pass for the task to be Done. "Docs" = design reference.

Estimates are ideal-days (d) for one developer.

---

## M0 — Foundation (≈ 5 d)

| ID | Task | Details | Docs | AT | Est |
|---|---|---|---|---|---|
| T0.1 | Repo scaffold & ptdev | `go mod init`, layout per 02-architecture §5, `.editorconfig`, golangci-lint config, `tools/ptdev` skeleton with engine detection + `version`, `db up/down/reset`, `build`, `lint`, `test`, `check` commands (other commands stubbed until their task). Works in PowerShell and bash. | 02, 14 §2, conventions | AT-006 | 1 |
| T0.2 | Config & CLI | `internal/config` (env + flags), `cmd/pandatable` sub-commands: `serve`, `migrate`, (stubs) `restore`, `doctor`, `reset-admin-password`. slog JSON logger. | 13 §4 | — | 0.5 |
| T0.3 | DB & migrations | pgx pool, tx helper (`WithTx(ctx, fn)`), goose embedded migrations with advisory lock; migration 0001: `meta` schema, extensions (`pg_trgm`, `pg_stat_statements`), `users`, `sessions`, `settings`. | 05 §2 | AT-002 | 0.5 |
| T0.4 | HTTP server | chi router, middleware (request id, slog access log, recover, security headers, gzip), `/healthz`, `/api/v1` mount, SPA via `go:embed` with history fallback, graceful shutdown. | 09 §1 | AT-001 | 0.5 |
| T0.5 | Web scaffold | Vite + Vue 3 + TS + Pinia + Router + Naive UI + vue-i18n (en, zh-CN) + dayjs; AppLayout header with **language + theme toggles side by side**; theme light/dark/system; ESLint, Prettier, Vitest; i18n key parity script. | 12 | AT-003, AT-802 | 1 |
| T0.6 | Image & deploy files | Multi-stage `deploy/Dockerfile` (node → go → debian-slim + `postgresql-client-18`), `pandatable healthcheck` sub-command; `deploy/` contains `compose.yaml` (external Caddy network), `compose.local.yaml`, `postgresql.conf`, `Caddyfile.example`, `.env.example`; ptdev `image build`, `image save [--with-postgres]`, optional opt-in `deploy`. | 13, 14 | AT-001, AT-004, AT-005 | 1 |
| T0.7 | Test harness & local gate | Go integration test helper (`PT_TEST_DATABASE_URL`, fresh DB per package, skip if unreachable); Playwright setup with fixtures (seed users via API) and `ptdev e2e`; `ptdev check` = full local gate (no hosted CI). | 14 §3, conventions | AT-006 | 1 |
| T0.8 | **RevoGrid spike** | Prototype: 200k-row windowed remote data, custom editor/renderer, range selection, TSV copy/paste, fill handle, grouped rows, frozen columns, dark theme, touch. Record result in decisions (confirm D-005 or switch). | 12 §4 | — | 1 |

**Exit:** image built on the laptop with Podman runs via `podman compose` (local override) and, after `save`/`load`, on the server with Docker behind the existing Caddy; app shell with toggles; `ptdev check` green; spike decision recorded.

---

## M1 — Core usable product (≈ 22 d)

### Backend foundation
| ID | Task | Details | Docs | AT | Est |
|---|---|---|---|---|---|
| T1.1 | Meta migrations | groups, group_members, workspaces, books, tables, fields, relations, views, grants, attachments, comments, notifications, row_history, backups (create all now to stabilise schema). | 05 | AT-002 | 0.5 |
| T1.2 | Auth service | argon2id hashing (params: m=64MB, t=1, p=2 — tuned for 2 vCPU), session create/validate/slide/revoke, login by username or email, logout, `/auth/me`, change password, profile prefs (locale, theme, **timezone**), trusted-proxy client IP; rate limiting (per IP + per account); CSRF + Origin middleware. | 09, 13 §9 | AT-102, AT-103, AT-116, AT-117, AT-803 | 2 |
| T1.3 | Setup flow | On boot with zero users: generate token, log `SETUP_TOKEN=…`; `/setup/status`, `POST /setup` (single-use, 410 after). | D-012 | AT-101 | 0.5 |
| T1.4 | ACL resolver & grants API | `acl.Resolver` (effective level along path, user+groups), per-process cache + epoch invalidation, middleware helpers `Require(level, resource)`; grants CRUD with scope-owner rule, last-owner rule, 404-for-hidden. Unit tests for the full matrix. | 06 | AT-204…AT-212 | 2.5 |

### Admin
| ID | Task | Details | Docs | AT | Est |
|---|---|---|---|---|---|
| T1.5 | Admin users | API + UI: list/search, create, edit, role, disable/enable (revoke sessions), reset password (forced change). | 09 | AT-104…AT-108 | 1.5 |
| T1.6 | Admin groups | API + UI: CRUD, members management. | 09 | AT-109 | 0.5 |
| T1.7 | Admin dashboard | gopsutil CPU/mem/disk, PG sizes, files size (cached walker), versions, uptime; UI cards auto-refresh 10 s. | 13 §6 | AT-110 | 1 |
| T1.8 | Admin settings & workspaces list | settings API/UI (app name, default timezone, default currency, default language, upload limit, retention days); read-only workspaces list with owners (transfer in M3). | 09 | AT-111, AT-112 | 0.5 |

### Structure
| ID | Task | Details | Docs | AT | Est |
|---|---|---|---|---|---|
| T1.9 | DDL module | Identifier generator, `CreateSchema`, `CreateTable` (system columns), `AddColumn`, `DropColumn`, `SetNotNull`, `AddUnique`, `SetDefault`; all within caller tx with `lock_timeout`. | 05 §3,§5 | AT-202, AT-804 | 1 |
| T1.10 | Workspace/Book/Table API | CRUD + reorder + soft delete; creating table creates default grid view + "Name" text field as display field; `/nav` tree filtered by ACL. | 09 | AT-201, AT-203, AT-213 | 1.5 |
| T1.11 | Sidebar & navigation UI | Tree with create/rename/delete/reorder (drag), path-only nodes, breadcrumbs, empty states, routes. | 12 §2 | AT-201, AT-208 | 1.5 |
| T1.12 | Share dialog UI | Per scope: list direct grants + inherited summary, add user/group with level, change/remove, warnings (inherited higher, last owner). | 06 | AT-204…AT-211 | 1 |

### Fields & rows
| ID | Task | Details | Docs | AT | Est |
|---|---|---|---|---|---|
| T1.13 | Codec framework + batch 1 | Go `Codec` interface + registry; codecs: text, long_text, integer, decimal, float, checkbox, date, datetime, single_select, multi_select, email, url, system fields. Frontend `FieldKind` registry with renderer/editor/parsePaste/formatCopy for same. | 07 | AT-301, AT-302, AT-304 | 2.5 |
| T1.14 | Field API & UI | Create/rename/options/description/required/unique/default/soft-delete/reorder; field header menu + field editor popover; display field setting. | 07 §4 | AT-301, AT-303 | 1.5 |
| T1.15 | Basic query & rows API | `/views/{id}/query` with paging + count + view field config + simple column sort; row get/create/patch/bulk patch/bulk insert/delete; `updated_at/by`. | 08 §3, §9 | AT-312 | 1.5 |
| T1.16 | DataGrid component | RevoGrid wrapper: windowed fetch, renderers/editors per FieldKind, keyboard nav, column resize/reorder/hide/freeze persisted to view config (or local when read-only), add row, select & delete rows. | 12 §4 | AT-305, AT-311 | 3 |
| T1.17 | Copy/paste & fill-down | TSV/HTML clipboard, multi-line quoted cell parsing, per-cell parse with error report, bulk PATCH, add-rows prompt; Ctrl+D & drag fill handle. | 12 §4 | AT-306…AT-309 | 1.5 |
| T1.18 | Row expand modal | All-fields form layout, deep link route; reused as mobile row editor. | 12 §4 | AT-310 | 1 |
| T1.19 | Mobile layout | Drawer sidebar, card list for grid views, full-screen row editor, FAB add, bottom-sheet toolbar. | 12 §5 | AT-313 | 1.5 |
| T1.20 | Seed & perf baseline | `pandatable dev seed --rows 200000` (dev-only command) + measure NFR-2. | 01 NFR | AT-312 | 0.5 |

**Exit:** team can log in, organise workspaces, share, define basic fields, and edit data in grid/mobile. Tag `v0.1.0`.

---

## M2 — Data power (≈ 24 d)

| ID | Task | Details | Docs | AT | Est |
|---|---|---|---|---|---|
| T2.1 | Codecs batch 2 | time, duration, currency (default = app default currency CNY, per-field ISO 4217 code), percent, rating, phone, uuid, json, inet, cidr, text_array, int_array, user, multi_user (+ `/directory/users`), autonumber (sequence), varchar maxLength. FE kinds for each. | 07 | AT-320, AT-326 | 2.5 |
| T2.2 | Attachments | Upload (multipart, size limit, MIME sniff), store `files/<yyyy>/<mm>/<uuid>`, image thumbnails (Go `image` + `x/image/draw`, ≤ 256px), ACL-checked serving, cell UI with drag-drop + preview. | 07, 13 §9 | AT-321, AT-322 | 2 |
| T2.3 | Relations | Link field creation (kind, target, on_delete) → FK/junction DDL + reverse field; link/unlink API; 1:1 replacement confirm; cascade/restrict logic in row delete; link cell chips + picker modal; same-book restriction. | 05 §3, 08 §9 | AT-401…AT-406 | 3 |
| T2.4 | Lookup & rollup | Compile correlated subqueries (1:N, 1:1, M:N); rollup fns; config UI; masking when no access to target. | 08 §8, 06 §6 | AT-407, AT-408, AT-411 | 1.5 |
| T2.5 | Formula engine | Lexer, Pratt parser, AST, type checker, SQL compiler, function library v1, cycle detection, `/formula:check`; editor UI with field/function autocomplete + inline error. | 08 §7 | AT-409 | 3 |
| T2.6 | Filter engine & UI | Filter tree validation + compiler (all ops per type, relative dates in user TZ, ILIKE escaping, array ops, lookup/rollup/formula); filter builder UI (nested groups ≤ 3). | 08 §2 | AT-501, AT-502, AT-410 | 2.5 |
| T2.7 | Sort | Multi-sort compile (NULLS LAST, id tie-breaker) + UI. | 08 §3 | AT-503 | 0.5 |
| T2.8 | Grouping | `/views/{id}/groups` buckets with counts/aggregates; grid grouped rendering (header rows, collapse state); mobile sticky headers. | 08 §4 | AT-504 | 2 |
| T2.9 | Highlight | Rule compile to `_hl_row/_hl_cells`; rules UI (ordered, drag), light/dark palette tokens. | 08 §1 | AT-505 | 1 |
| T2.10 | View management | Create/rename/duplicate/delete/reorder views; shared vs personal; lock; temporary overrides + "save as personal view"; optimistic version conflicts. | 08 §1 | AT-506…AT-508 | 1.5 |
| T2.11 | Search & aggregates | Quick search; footer aggregates endpoint + UI. | 08 §5–6 | AT-509, AT-510 | 1 |
| T2.12 | Type change | Dry-run + convert with USING expressions; specialised conversions; dependent formula re-check; select choice rename propagation; UI wizard. | 07 §5 | AT-323, AT-324 | 1.5 |
| T2.13 | Form view | Form builder (field selection, labels, help, required), submit endpoint, mobile-friendly. | 08 §1 | AT-511 | 1.5 |
| T2.14 | Auto-index job | Background `CREATE INDEX CONCURRENTLY` per D-020; tracking in meta (`fields.options.indexed`). | D-020 | AT-512 | 0.5 |
| T2.15 | Perf pass | Seed 200k rows with links; EXPLAIN key queries; tune. | 01 NFR | AT-312, AT-512 | 1 |

**Exit:** relations, computed fields, full view features. Tag `v0.2.0`.

---

## M3 — Collaboration & ops → v1.0 (≈ 20 d)

| ID | Task | Details | Docs | AT | Est |
|---|---|---|---|---|---|
| T3.1 | Realtime hub | Dedicated LISTEN conn, `pg_notify` emission helper used by all mutations (rows, schema, views, nav, grants), WS endpoint (`coder/websocket`), subscribe with ACL check, eventId echo suppression, heartbeat, client store integration + reconnect resync, `session.revoked`. | 10 §1, 09 §3 | AT-601…AT-603, AT-605, AT-107 | 3 |
| T3.2 | Presence | In-memory presence per table, throttled broadcast, avatar stack + cell outline UI. | 10 §2 | AT-604 | 1 |
| T3.3 | Comments & notifications | Comments API/UI in row modal, @mention autocomplete (users with access), notifications table + bell + WS push, comment count badge. | 10 §3 | AT-606 | 2 |
| T3.4 | Row history | Capture via `RETURNING OLD/NEW` in all row/link mutations; history API + timeline UI; retention job. | 10 §4 | AT-607 | 1.5 |
| T3.5 | Undo/redo | Client action stack with inverses; `rows:restore` endpoint (same ids, links, attachments refs); conflict confirm. | 10 §5 | AT-608, AT-609 | 2 |
| T3.6 | Trash & purge | Trash API/UI per scope; restore; daily purge job (DROP physical objects, delete files). | 05 §4 | AT-203, AT-325, AT-610 | 1.5 |
| T3.7 | Export | Streaming CSV (BOM) and XLSX (excelize StreamWriter); PandaTable package with `_pandatable` manifest. | 11 §1 | AT-701, AT-702 | 1.5 |
| T3.8 | Import | Analyze (encoding incl. GBK, delimiter, type guess), wizard UI, job runner with progress, COPY batches, error report; package import (structure + views + relink). | 11 §2 | AT-702…AT-705 | 3 |
| T3.9 | Backups | Job (pg_dump → zip with files + backup.json), disk guard, list/download/delete API + UI; `restore` CLI. | 11 §3 | AT-706…AT-708 | 1.5 |
| T3.10 | User lifecycle admin | Ownerships view, transfer all, orphaned workspace rescue, hard delete with sole-owned cascade. | 06 §7, D-021 | AT-113…AT-115 | 1 |
| T3.11 | Hardening | CSP, upload sniffing, rate limits review, `doctor` drift check, error-path i18n review, load test script. | 13 §9 | AT-801, AT-803…AT-806 | 1.5 |
| T3.12 | Release v1.0 | Full acceptance run, deploy to target server, short user guide (`docs/user-guide.md`, EN + ZH), tag `v1.0.0`. | — | all M0–M3 | 1 |

---

## M4 — More views (≈ 8 d)

| ID | Task | AT | Est |
|---|---|---|---|
| T4.1 | Kanban view (group by single select/user; drag between columns; card fields; realtime) | AT-901 | 5 |
| T4.2 | Gallery view (cover attachment, card fields) | AT-902 | 3 |

## M5 — Calendar & mail (≈ 8 d)

| ID | Task | AT | Est |
|---|---|---|---|
| T5.1 | Calendar view (month/week, drag reschedule, date range fields) | AT-903 | 5 |
| T5.2 | SMTP: settings + test mail, self-service password reset, opt-in mention emails | AT-904 | 3 |

---

## Cross-cutting checklist (every task)

- [ ] ACL enforced server-side for every new endpoint (+ test).
- [ ] Realtime events emitted for every mutation (from M3 on; earlier tasks leave `// TODO(T3.1)` hook via a shared `events.Emit` no-op).
- [ ] i18n keys added in `en` + `zh-CN`; error codes translated.
- [ ] Works in dark mode and at 390 px width.
- [ ] Docs updated (design doc, decisions if needed, progress).
