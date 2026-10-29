# PandaTable Documentation Index

| # | Document | What it answers |
|---|---|---|
| 01 | [Requirements](01-requirements.md) | What we build and what we deliberately do NOT build |
| 02 | [Architecture](02-architecture.md) | High-level structure: components, runtime, repo layout |
| 03 | [Roadmap](03-roadmap.md) | High-level plan: milestones and their goals |
| 04 | [Implementation Plan](04-implementation-plan.md) | Detailed task breakdown per milestone (task IDs) |
| 05 | [Data Model](05-data-model.md) | Meta schema, physical user-table layout, naming |
| 06 | [Permissions](06-permissions.md) | Roles, grants, inheritance, action matrix, algorithm |
| 07 | [Field Types](07-field-types.md) | UI types ↔ PostgreSQL types, options, type changes |
| 08 | [Query Engine](08-query-engine.md) | Views config, filter/sort/group/highlight, formulas, lookups |
| 09 | [API](09-api.md) | REST endpoints, WebSocket protocol, error format |
| 10 | [Collaboration](10-collaboration.md) | Realtime, presence, comments, history, undo |
| 11 | [Import / Export / Backup](11-import-export-backup.md) | File formats, backup jobs |
| 12 | [Frontend](12-frontend.md) | UI structure, grid behavior, responsive, i18n, theme |
| 13 | [Deployment & Operations](13-deployment.md) | Server compose stack, existing Caddy, PG tuning, upgrades, monitoring |
| 14 | [Dev, Build & Release](14-dev-build-release.md) | Laptop setup (Podman/Docker), `ptdev` tool, tests, image build/save/transfer |
| — | [Conventions](conventions.md) | Coding, naming, commits, testing |
| — | [Decisions](decisions.md) | Decision log (binding) |
| — | [Progress](progress.md) | Status tracking |
| — | [Acceptance Tests](acceptance-tests.md) | Testable acceptance criteria (AT-xxx) |
| — | [Glossary](glossary.md) | Terms used across docs |
| — | [Task prompts](task-prompts/) | Ready-to-use prompts for starting each task in a fresh agent session ([template](task-prompts/TEMPLATE.md)) |
| — | [Archive](archive/) | Historical M0–M1 verification evidence; search by task/AT ID, never read at task start |

## Reading budget (D-158)

At task start read only `AGENTS.md`, this index, the task prompt and the doc
sections it names. Search `decisions.md` and `acceptance-tests.md` by ID
(`Select-String -Path docs\decisions.md -Pattern 'D-145'`) instead of reading
them whole. Every extra file is re-sent on each model call for the rest of the session.

## How the docs relate

```
requirements ──► architecture ──► roadmap ──► implementation-plan ──► progress
      │                │                              │
      └──► acceptance-tests ◄─────────────────────────┘
decisions  ← referenced by all (D-xxx)
05..13     ← detailed design referenced by implementation-plan tasks
```
