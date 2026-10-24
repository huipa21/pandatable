# Decision Log

Binding decisions. Never edit history; to change a decision add a new entry and mark the old one `Superseded by D-xxx`.
Source "Qn" = interview question number (2026-10-24 planning interview).

| ID | Date | Decision | Rationale / notes | Source | Status |
|---|---|---|---|---|---|
| D-001 | 2026-10-24 | Each user table is a **real PostgreSQL table**; each book is a PG schema; metadata in schema `meta`. | Native types, real FKs, SQL filtering/grouping, best performance at 200k rows. | Q1 | Accepted |
| D-002 | 2026-10-24 | Physical identifiers are generated (`b_/t_/c_/j_/s_` + 10-char base32); user-visible names only in meta. | No SQL injection via names, renames are metadata-only, no quoting issues with Chinese names. | — | Accepted |
| D-003 | 2026-10-24 | Backend: Go, chi router, pgx/v5, squirrel (dynamic SQL), sqlc (meta queries), goose (migrations), slog. | Lightweight, idiomatic, no ORM magic. | Q2 | Accepted |
| D-004 | 2026-10-24 | Frontend: Vue 3 + TS + Vite + Pinia + **Naive UI** + vue-i18n. | TS-first, dense UI, built-in dark theme & zh-CN locale. | Q3 | Accepted |
| D-005 | 2026-10-24 | Grid engine: **RevoGrid**, wrapped behind own `DataGrid` component interface. | MIT, virtual scroll, editing, free row grouping. Wrapper keeps it swappable; spike in M0 validates fill handle, range copy/paste, grouping. | Q4 | Accepted (pending spike T0.8) |
| D-006 | 2026-10-24 | Deploy with Docker Compose: `postgres:18` + single app image (SPA embedded) + optional Caddy. | Simple ops on 2C/4G Ubuntu 24.04 host. | Q5, Q6 | Superseded by D-046/D-047 (Caddy external; build via Podman) |
| D-007 | 2026-10-24 | Auth: username/email + password (argon2id), server-side sessions in PG, HttpOnly/Secure/SameSite=Lax cookie + CSRF header + Origin check. | Revocable sessions, no JWT complexity. | Q7 | Accepted |
| D-008 | 2026-10-24 | Password reset by admin only, forced change at next login. SMTP (self-service reset, mention emails) in M5. | No mail server needed for v1. | Q8 | Accepted |
| D-009 | 2026-10-24 | Admin has no implicit data access; can list workspaces/owners and transfer ownership of orphaned workspaces. | Separation of user management and data access. | Q10 | Accepted |
| D-010 | 2026-10-24 | Any user can create workspaces and becomes first owner. | | Q11 | Accepted |
| D-011 | 2026-10-24 | Grants can target users or groups; effective = max of all applicable grants. Groups are flat. | | Q12 | Accepted |
| D-012 | 2026-10-24 | First admin created via **web setup page**, available only while no users exist, protected by a one-time **setup token printed to server log**. | User chose web setup (Q9=2); token prevents hijack of a fresh public cloud instance. | Q9 | Accepted |
| D-013 | 2026-10-24 | Permission inheritance downward; lower-level grants can only **raise**. | Simple mental model. | Q14 | Accepted |
| D-014 | 2026-10-24 | An owner (effective) of any scope can manage grants on that scope and its sub-levels, including granting owner. | Q15=2 (refines original "only workspace owner" statement). | Q15 | Accepted |
| D-015 | 2026-10-24 | Realtime via WebSocket; events from PG `LISTEN/NOTIFY` (delivered on commit); last-write-wins per cell; field-level PATCH. | Cheap, correct ordering after commit, no CRDT. | Q36 | Accepted |
| D-016 | 2026-10-24 | Fully responsive: desktop grid; mobile (< 768 px) uses card list + full-screen row editor; copy/paste & fill-down desktop-only. | Q46=2 with feasible effort. | Q46 | Accepted |
| D-017 | 2026-10-24 | Relations are bidirectional link fields, within the same book, self-links allowed; on-delete per relation: set_null (default) / cascade / restrict. 1:1 = unique FK, 1:N = FK on child, M:N = junction table. | | Q24, Q25, Q27, Q28 | Accepted |
| D-018 | 2026-10-24 | Formulas, lookups, rollups are compiled to SQL expressions **at query time**; not stored; not PG generated columns. | PG18 virtual columns can't use NOW()/subqueries and require DDL to change; uniform approach. Revisit for perf. | Q20, Q26 | Accepted |
| D-019 | 2026-10-24 | Row paging via OFFSET/LIMIT (page 100) with `id` tie-breaker; separate cached count. | Adequate for ≤ 200k rows; simplest with arbitrary sorts. | Q6 | Accepted |
| D-020 | 2026-10-24 | Auto-indexing policy: index FK columns and unique fields always; btree/GIN/trigram indexes created concurrently in background when a field is used in shared-view sort/group/filter/search on tables > 10k rows. No manual index UI in v1. | Keeps NFR-3 without exposing DBA concepts. | — | Accepted |
| D-021 | 2026-10-24 | Leaving users are disabled (soft). Admin can transfer all their ownerships or hard-delete the user and everything they solely own; remaining authorship shows "Deleted user". | | Q13 | Accepted |
| D-022 | 2026-10-24 | Schema changes (tables, fields, relations, type changes) require `owner`; `write` edits rows and shared views. Three levels kept. | | Q16 | Accepted |
| D-023 | 2026-10-24 | Workspace must keep ≥ 1 active **direct user** owner (group ownership doesn't count). | | Q17 | Accepted |
| D-024 | 2026-10-24 | UI types layered over native PG types (see 07-field-types.md); supported PG types: text, varchar(n), smallint/integer/bigint, numeric(p,s), real/double, boolean, date, time, timestamptz, interval, uuid, jsonb, inet/cidr, text[]/bigint[]. | | Q18, Q19 | Accepted |
| D-025 | 2026-10-24 | System fields (id, created/updated at/by) always exist, hidden by default, can be shown. | | Q21 | Accepted |
| D-026 | 2026-10-24 | Attachments on local disk volume (`/data/files`), admin-configurable size limit. | | Q22 | Accepted |
| D-027 | 2026-10-24 | Field type change allowed with dry-run (`pg_input_is_valid`) and choice abort / set-failed-to-null. | | Q23 | Accepted |
| D-028 | 2026-10-24 | View types: Grid + Form (v1), Kanban + Gallery (M4), Calendar (M5). | | Q29 | Accepted |
| D-029 | 2026-10-24 | Filters nested AND/OR depth 3; group up to 3 levels server-side; highlight = ordered rules on row or cells, first match wins. | | Q31–Q33 | Accepted |
| D-030 | 2026-10-24 | Shared + personal views; owners can lock shared views. No public share links or public forms. | | Q34, Q35 | Accepted |
| D-031 | 2026-10-24 | Presence (avatars + cell outline), row comments with @mentions and in-app notifications. | | Q37, Q38 | Accepted |
| D-032 | 2026-10-24 | Minimal per-row change history captured with PG18 `RETURNING OLD/NEW`, retention 90 days (setting). Not an audit log. | | Q39 | Accepted |
| D-033 | 2026-10-24 | Client-side undo/redo, last 50 actions per tab; row delete returns snapshots for restore with same ids (identity `BY DEFAULT`). | | Q40 | Accepted |
| D-034 | 2026-10-24 | Export CSV (view data) and XLSX; XLSX "PandaTable package" includes all rows, views and a hidden `_pandatable` manifest sheet with field types so re-import recreates exact structure. | | Q41 | Accepted |
| D-035 | 2026-10-24 | No personal API tokens / public API in v1. | | Q42 | Accepted |
| D-036 | 2026-10-24 | Trash for workspace/book/table/field/view, 30 days (setting). Rows hard-deleted (undo covers). | | Q43 | Accepted |
| D-037 | 2026-10-24 | Backups from Admin UI: pg_dump (custom format) + attachments zipped into `/data/backups`; create/list/download/delete in UI; restore via CLI. App image ships `pg_dump` 18. | | Q44 | Accepted |
| D-038 | 2026-10-24 | i18n: English + Simplified Chinese from day one. Language and theme toggles side by side in header for all users. | | Q45, Q49 | Accepted |
| D-039 | 2026-10-24 | Dark mode via Naive UI theming (light/dark/system). | | Q47 | Accepted |
| D-040 | 2026-10-24 | Product name: PandaTable. | | Q48 | Accepted |
| D-041 | 2026-10-24 | Admin dashboard shows CPU, memory, storage (DB, files, backups, disk) via gopsutil + PG size functions, cached 5 s. | | Q49 | Accepted |
| D-042 | 2026-10-24 | Grid must support Excel/Sheets-compatible copy/paste (TSV) and fill-down (Ctrl+D + drag handle). | | Q51 | Accepted |
| D-043 | 2026-10-24 | Permission levels for comments/history/export: `read` users may comment, view history and export. | Commenting is collaboration, not data editing. | — | Accepted |
| D-044 | 2026-10-24 | Cross-table visibility: without read access on a linked table, link cells show only display values; lookups/rollups are masked. | Minimal leakage, keeps links usable. | — | Accepted |
| D-045 | 2026-10-24 | Single app instance (no horizontal scaling); presence kept in memory. | Team of ~10; keeps design simple. | Q6 | Accepted |
| D-046 | 2026-10-24 | No Caddy in our stack. The app joins the server's existing Caddy network (external compose network, name via `PT_PROXY_NETWORK`, alias `pandatable`); no host ports published. App trusts `X-Forwarded-*` only from `PT_TRUSTED_PROXIES`. | User runs an independent Caddy container for all apps. | OQ-1 | Accepted |
| D-047 | 2026-10-24 | Images are built on the Windows laptop with Podman (Docker also supported), tagged `localhost/pandatable:<ver>`, saved as gzip tar (`--format docker`/`docker-archive`) and `docker load`-ed on the server. Dockerfile/compose use only features common to Podman and Docker, with fully qualified image names. Optional bundling of `postgres:18` image for servers without Docker Hub access. | Build locally, transfer images; laptop = Podman, server = Docker. | OQ-2 | Accepted |
| D-048 | 2026-10-24 | No hosted CI. A cross-platform Go dev tool `tools/ptdev` (run via `go run ./tools/ptdev …`) replaces Makefile/shell scripts: dev DB, dev servers, lint, test, check, e2e, image build/save, optional deploy. Integration tests use a dev PG18 container via `PT_TEST_DATABASE_URL` (no testcontainers) and skip when unavailable. | Works identically in PowerShell and bash; avoids Podman-socket issues on Windows. | OQ-2 | Accepted |
| D-049 | 2026-10-24 | All timestamps stored and DB sessions in UTC. App setting `default_timezone` = `Asia/Shanghai`; each user may override (`users.timezone`). Display, relative-date filters, `NOW()/TODAY()` formulas and exports use the requesting user's timezone. | Team in China; per-user flexibility. | OQ-3 | Accepted |
| D-050 | 2026-10-24 | App setting `default_currency` = `CNY` (admin-adjustable); each currency field stores its own ISO 4217 code, preselected from the default at creation and changeable per field. | | OQ-4 | Accepted |
| D-051 | 2026-10-24 | T0.1 local gate runs lint, test, then build; reports every attempted step and fails if any step fails. The private Go module is `pandatable`, using the installed Go 1.27.0 toolchain. ptdev runs from the repo root and uses stdlib-only direct subprocesses. Scope gosec G204/G304 exclusions to `tools/ptdev/execx.go`, which executes developer-controlled commands and copies repo-local build assets; do not relax application checks. | Completes the requested T0.1 gate, preserves engine-free commands, and avoids shell-specific automation. Ctrl+C uses context cancellation, interrupt where supported, forced termination otherwise, and a bounded wait. | T0.1 | Accepted |
| D-052 | 2026-10-24 | App configuration flags use kebab-case names matching the `PT_` environment settings; precedence is flags, environment, then documented defaults. An explicitly empty trusted-proxy list trusts no proxies. | Keeps local overrides predictable and makes opting out of proxy trust fail closed. | T0.2 | Accepted |
