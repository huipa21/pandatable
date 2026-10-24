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

Planning phase. See [docs/progress.md](docs/progress.md).
