# 02 — Architecture (High-Level Structure)

## 1. System context

```
            Users (≤10)  — desktop & mobile browsers
                    │ HTTPS
            ┌───────▼────────────────┐
            │ Caddy (existing, shared│  TLS, zstd/gzip, HTTP/2
            │ container, not ours)   │  external network "caddy"
            └───────┬────────────────┘
                    │ HTTP pandatable:8080  (REST + WebSocket)
   ┌────────────────▼─────────────────────────────────────────┐
   │                pandatable  (single Go binary)            │
   │  embedded Vue SPA (go:embed)                             │
   │                                                          │
   │  httpapi ─ middleware: session, CSRF, ACL, i18n, logging │
   │     │                                                    │
   │  ┌──┴──────┬─────────┬─────────┬──────────┬───────────┐  │
   │  │ auth    │ admin   │ meta    │ data     │ realtime  │  │
   │  │ setup   │ users   │ ws/book │ rows     │ ws hub    │  │
   │  │ session │ groups  │ table   │ query    │ presence  │  │
   │  │         │ sysinfo │ field   │ formula  │ LISTEN/   │  │
   │  │         │ backup  │ view    │ relation │ NOTIFY    │  │
   │  │         │         │ acl     │ history  │           │  │
   │  └─────────┴─────────┴────┬────┴──────────┴───────────┘  │
   │      ddl (physical schema ops) │ jobs (purge, retention, │
   │      files (attachments)       │ backup, stats cache)    │
   └───────────────┬────────────────┴──────────┬──────────────┘
                   │ pgx pool (≤ 20 conns)      │ local FS
          ┌────────▼─────────┐        ┌────────▼────────┐
          │ PostgreSQL 18    │        │ /data/files     │
          │  schema meta     │        │ /data/backups   │
          │  schema b_<id>…  │        └─────────────────┘
          └──────────────────┘
```

## 2. Key architectural choices (see decisions.md)

| Topic | Choice | Ref |
|---|---|---|
| User data storage | One **real PostgreSQL table per user table**, one PG schema per book | D-001 |
| Identifiers | Physical names are generated (`b_<id>`, `t_<id>`, `c_<id>`); user names live only in meta | D-002 |
| Backend libs | chi, pgx/v5, squirrel (dynamic SQL), sqlc (meta queries), goose (migrations) | D-003 |
| Frontend | Vue 3 + TS + Vite + Pinia + Naive UI + RevoGrid + vue-i18n | D-004, D-005 |
| Auth | Username/email + password (argon2id), server-side sessions in PG, HttpOnly cookie | D-007 |
| Realtime | WebSocket hub fed by PG `LISTEN/NOTIFY`; last-write-wins per cell | D-015 |
| Formulas/lookups/rollups | Compiled to SQL expressions at query time (not stored) | D-018 |
| Deployment | Compose: app + postgres:18; joins existing Caddy network; image built locally (Podman/Docker), transferred as tar | D-006, D-046, D-047 |
| Dev tooling | Cross-platform Go dev tool `tools/ptdev` (no Makefile, no hosted CI) | D-048 |
| Time | Stored in UTC; converted per user timezone (default Asia/Shanghai) | D-049 |

## 3. Backend module responsibilities

| Package | Responsibility |
|---|---|
| `cmd/pandatable` | Entry point: `serve`, `migrate`, `restore`, `reset-admin-password` sub-commands |
| `internal/config` | Env/flag config (`PT_DATABASE_URL`, `PT_DATA_DIR`, `PT_LISTEN`, `PT_BASE_URL`, …) |
| `internal/db` | pgx pool, tx helpers, goose migrations for `meta` schema |
| `internal/auth` | Setup flow, login/logout, sessions, password hashing, rate limit |
| `internal/admin` | Users, groups, ownership transfer, hard delete, settings, sysinfo |
| `internal/acl` | Grant CRUD, effective-permission resolution + per-request cache |
| `internal/meta` | Workspaces, books, tables, fields, views, relations (metadata) |
| `internal/ddl` | Generates & executes DDL for physical schema changes (in same tx as meta change) |
| `internal/query` | View config → SQL (filter, sort, group, highlight, search, aggregates) |
| `internal/formula` | Formula language: lexer, parser, type-checker, SQL compiler |
| `internal/data` | Row CRUD, bulk ops (paste / fill-down), link/unlink |
| `internal/history` | Row change capture + retention |
| `internal/comment` | Comments, mentions, notifications |
| `internal/realtime` | WebSocket hub, subscriptions, presence, PG LISTEN/NOTIFY bridge |
| `internal/files` | Attachment upload/download, thumbnails (images only, small) |
| `internal/xfer` | CSV/XLSX import & export |
| `internal/backup` | Backup jobs (pg_dump + zip), listing, deletion, restore CLI |
| `internal/jobs` | Lightweight scheduler (trash purge, history retention, stats cache) |
| `internal/httpapi` | Router, handlers, middleware, error mapping, static SPA serving |
| `internal/i18n` | Error-code catalogue (messages rendered in frontend) |

Layering rule: `httpapi → services (auth/admin/meta/data/…) → acl/query/ddl → db`. Services never import `httpapi`.

## 4. Request flow example — edit a cell

1. Client `PATCH /api/v1/tables/{tid}/rows/{rid}` `{ "fields": { "fld_x": 42 } }`.
2. Middleware: session → user; ACL resolves `write` on table.
3. `data` validates value via field type codec → builds `UPDATE b_x.t_y SET c_z=$1, updated_at=now(), updated_by=$2 WHERE id=$3 RETURNING OLD.c_z, NEW.*`.
4. Same tx: insert `meta.row_history`; `pg_notify('pt_events', {table, op:"update", ids, fields, by, txid})`.
5. Commit → response with new row.
6. Realtime hub receives NOTIFY → pushes `row.updated` to subscribers of the table → clients refetch/merge row.

## 5. Repository layout

```
pandatable/
├── cmd/pandatable/            main.go
├── internal/                  (packages listed above)
│   └── web/                   embed.go + dist/ (built SPA copied here)
├── migrations/                goose SQL for meta schema (embedded)
├── web/                       Vue 3 app
│   ├── src/
│   │   ├── api/               typed REST client + ws client
│   │   ├── stores/            Pinia stores
│   │   ├── router/
│   │   ├── i18n/              en.json, zh-CN.json
│   │   ├── layouts/           AppLayout, AuthLayout, AdminLayout
│   │   ├── pages/             Login, Setup, Workspace, Table, Admin/*
│   │   ├── components/
│   │   │   ├── grid/          RevoGrid wrapper, editors, renderers
│   │   │   ├── fields/        per-type cell renderer/editor/filter input
│   │   │   ├── views/         toolbar, filter/sort/group/highlight builders
│   │   │   ├── mobile/        card list, row editor
│   │   │   └── common/
│   │   └── composables/
│   └── e2e/                   Playwright acceptance tests
├── tools/ptdev/               cross-platform dev/build/release tool (go run ./tools/ptdev …)
├── deploy/
│   ├── Dockerfile             multi-stage, linux/amd64, works with podman & docker
│   ├── compose.yaml           server stack (app + db, external caddy network)
│   ├── compose.local.yaml     laptop override (publish 127.0.0.1:8080, local network)
│   ├── postgresql.conf        tuned for 4 GB host
│   ├── Caddyfile.example      site block to paste into the existing Caddy
│   └── .env.example
├── dist/                      (git-ignored) image archives for transfer
├── docs/
└── AGENTS.md
```

## 6. Runtime resources (target 4 GB host)

| Component | Budget |
|---|---|
| PostgreSQL 18 | shared_buffers 768 MB, work_mem 16 MB, max_connections 40 → ~1.2 GB peak |
| pandatable | < 150 MB RSS (GOMEMLIMIT=300MiB) |
| Caddy | external, shared with other apps (not budgeted here) |
| OS + page cache | remainder (~2.5 GB) |

Details in [13-deployment.md](13-deployment.md).
