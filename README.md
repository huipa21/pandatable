# PandaTable

A lightweight, self-hosted, NocoDB/Airtable-like collaborative table app for small teams (~10 users).

- **Backend:** Go (single binary, frontend embedded)
- **Frontend:** Vue 3 + TypeScript + Vite + Pinia + Naive UI + RevoGrid
- **Database:** PostgreSQL 18 (built-in, the only supported database)
- **Target host:** 2 vCPU / 4 GB RAM / Ubuntu 24.04 x86_64, Docker Compose, behind an existing Caddy container
- **Build:** locally (Windows + Podman, or Docker) via `go run ./tools/ptdev …`; images transferred as tar archives

Structure: **Workspace → Book → Table → Views**, with read / read-write / owner permissions on each level.

## Documentation

All project documentation lives in [`docs/`](docs/README.md). Start there.

| Doc | Purpose |
|---|---|
| [docs/01-requirements.md](docs/01-requirements.md) | Scope, in/out, functional requirements |
| [docs/02-architecture.md](docs/02-architecture.md) | High-level structure |
| [docs/03-roadmap.md](docs/03-roadmap.md) | High-level plan / milestones |
| [docs/04-implementation-plan.md](docs/04-implementation-plan.md) | Detailed task-level plan |
| [docs/decisions.md](docs/decisions.md) | Decision log |
| [docs/progress.md](docs/progress.md) | Progress tracking |
| [docs/acceptance-tests.md](docs/acceptance-tests.md) | Acceptance tests |
| [docs/14-dev-build-release.md](docs/14-dev-build-release.md) | Dev setup, build & release |

## Status

M0 foundation in progress: CLI, migrations, HTTP server, and localized web shell
implemented through T0.5. Deployment is next (T0.6). See [docs/progress.md](docs/progress.md).

## Local Development

Requires Go 1.27.0, Node.js >= 22.13, pnpm 11.2.2 (Corepack), and Podman or
Docker for the dev database. Run from the repo root:

```text
go run ./tools/ptdev help
pnpm --dir web install --frozen-lockfile
go run ./tools/ptdev db up
go run ./tools/ptdev dev
```

The UI is at `http://127.0.0.1:5173`, with `/api` (including future WebSockets)
proxied to the backend. Ctrl+C stops both direct child processes. The welcome
page is a scaffold, not an authentication implementation.

```text
go run ./tools/ptdev check
go run ./tools/ptdev web build
go run ./tools/ptdev build --linux
go run ./tools/ptdev db down
```

Without a container engine, use `go run ./tools/ptdev test --no-db`.
`serve` requires `PT_DATABASE_URL` and `PT_BASE_URL`; it migrates before binding
HTTP. Build the frontend before building the binary to embed the SPA. A clean
Go-only build still works, but UI requests return explicit 503 until assets exist.
See [docs/14-dev-build-release.md](docs/14-dev-build-release.md) for command details.
