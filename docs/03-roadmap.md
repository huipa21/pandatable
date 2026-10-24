# 03 — Roadmap (High-Level Plan)

Release **v1.0** = M0 + M1 + M2 + M3. M4+ are post-v1 increments.

| Milestone | Theme | Goal (exit criteria) | Est. |
|---|---|---|---|
| **M0** | Foundation | Repo skeleton, `ptdev` tool, Dockerfile + compose (Podman & Docker), migrations, SPA shell with i18n + theme toggles, health endpoint. Image built on laptop runs via `podman compose`, and loads/runs on the server behind the existing Caddy. | 1 wk |
| **M1** | Core usable product | First-run setup, login, admin panel (users, groups, sysinfo), workspace/book/table CRUD, permissions, basic field types, grid with inline editing, keyboard nav, copy/paste, fill-down, row expand, mobile card list. A team can store and edit data. | 4–5 wk |
| **M2** | Data power | Relations (1:1, 1:N, M:N, self), lookup/rollup, formulas, all remaining field types incl. attachments, type change with dry-run, views (shared/personal/lock), filter/sort/group/highlight, search, footer aggregates, Form view. | 4–5 wk |
| **M3** | Collaboration & ops | Realtime + presence, comments + mentions + notifications, row history, undo/redo, trash, import/export (CSV/XLSX + meta), admin backups, ownership transfer / hard delete. **v1.0 release.** | 3–4 wk |
| **M4** | More views | Kanban, Gallery. | 2 wk |
| **M5** | Calendar & mail | Calendar view; SMTP (password reset by email, mention emails). | 2 wk |
| Backlog | Maybe later | Scheduled automatic backups with retention; restore from Admin UI; book-level export package; personal API tokens; Gantt. | — |

## Dependency order

```
M0 ─► M1 ─► M2 ─► M3 (v1.0) ─► M4 ─► M5
            │
            └─ realtime (M3) intentionally after query engine is stable
```

## Risks & mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| RevoGrid can't cover a needed behavior (e.g. fill handle, grouped rows UX) | Grid rework | Spike in M0 (T0.8); wrap grid behind our own `DataGrid` component interface so it can be swapped. |
| DDL in transactions + concurrent edits → lock waits | Edits stall during schema change | `lock_timeout = 3s` on DDL tx; schema changes are rare; client retries. |
| Formula language scope creep | Delays M2 | Fixed function list v1 (see 08-query-engine.md); extend later. |
| Mobile editing complexity | Delays M1 | Mobile uses card list + row editor (reuses row expand form), not the grid. |
| pg_dump version must match PG 18 | Backup failure | App image installs `postgresql-client-18` from PGDG; checked at startup. |
| Podman vs Docker differences (image names, HEALTHCHECK format, compose provider) | Image works on laptop but not server | Fully-qualified image names, `--format docker`, `localhost/` tag prefix, compose file limited to common features; AT-005 verifies laptop→server path. |
| Server can't pull from Docker Hub (mainland China) | Deploy blocked | `ptdev image save --with-postgres` ships postgres:18 too. |
| No hosted CI | Regressions slip in | `ptdev check` mandatory before every tag; recorded in progress.md. |
| 4 GB RAM | OOM | Tuned PG config, GOMEMLIMIT, streaming exports/imports, paginated queries. |
