# Progress

Status legend: ⬜ Not started · 🟨 In progress · ✅ Done · ⏸ Blocked · ❌ Dropped

## Milestones

| Milestone | Status | Started | Finished | Tag | Notes |
|---|---|---|---|---|---|
| Planning | ✅ | 2026-10-24 | 2026-10-24 | — | Interview completed; docs created |
| M0 Foundation | 🟨 | 2026-10-24 | | | T0.1–T0.3 complete for their scoped checks. AT-002 migration idempotence verified; full AT-006 remains pending. |
| M1 Core | ⬜ | | | v0.1.0 | |
| M2 Data power | ⬜ | | | v0.2.0 | |
| M3 Collaboration & ops | ⬜ | | | v1.0.0 | |
| M4 More views | ⬜ | | | v1.1.0 | |
| M5 Calendar & mail | ⬜ | | | v1.2.0 | |

## Tasks

| Task | Title | Status | Updated | Notes |
|---|---|---|---|---|
| T0.1 | Repo scaffold & ptdev | ✅ | 2026-10-24 | Scoped completion approved: scaffold, stdlib ptdev, unit tests, Windows/Podman check, PowerShell + Git Bash entry points, native + Linux builds. Go 1.27.0; Podman client/server 5.8.2/5.8.4; Node 24.15.0; PostgreSQL 18.6. golangci-lint absent (warning). Full AT-006 pending Linux/Docker + T0.7 harness. |
| T0.2 | Config & CLI | ✅ | 2026-10-24 | Config flags/validation and JSON CLI routing; unit tests, ptdev check, and PowerShell/Git Bash smoke pass. Full AT-006 remains pending. |
| T0.3 | DB & migrations | ✅ | 2026-10-24 | pgx pool, transaction helper, and embedded migration complete; PostgreSQL 18 integration and repeated CLI migrations pass. AT-002 migration behavior verified; full AT-006 remains pending. |
| T0.4 | HTTP server | ⬜ | | |
| T0.5 | Web scaffold (i18n, theme toggles) | ⬜ | | |
| T0.6 | Image & deploy files | ⬜ | | |
| T0.7 | Test harness & local gate | ⬜ | | |
| T0.8 | RevoGrid spike | ⬜ | | |
| T1.1 | Meta migrations | ⬜ | | |
| T1.2 | Auth service | ⬜ | | |
| T1.3 | Setup flow | ⬜ | | |
| T1.4 | ACL resolver & grants API | ⬜ | | |
| T1.5 | Admin users | ⬜ | | |
| T1.6 | Admin groups | ⬜ | | |
| T1.7 | Admin dashboard | ⬜ | | |
| T1.8 | Admin settings & workspaces list | ⬜ | | |
| T1.9 | DDL module | ⬜ | | |
| T1.10 | Workspace/Book/Table API | ⬜ | | |
| T1.11 | Sidebar & navigation UI | ⬜ | | |
| T1.12 | Share dialog UI | ⬜ | | |
| T1.13 | Codec framework + batch 1 | ⬜ | | |
| T1.14 | Field API & UI | ⬜ | | |
| T1.15 | Basic query & rows API | ⬜ | | |
| T1.16 | DataGrid component | ⬜ | | |
| T1.17 | Copy/paste & fill-down | ⬜ | | |
| T1.18 | Row expand modal | ⬜ | | |
| T1.19 | Mobile layout | ⬜ | | |
| T1.20 | Seed & perf baseline | ⬜ | | |
| T2.1 | Codecs batch 2 | ⬜ | | |
| T2.2 | Attachments | ⬜ | | |
| T2.3 | Relations | ⬜ | | |
| T2.4 | Lookup & rollup | ⬜ | | |
| T2.5 | Formula engine | ⬜ | | |
| T2.6 | Filter engine & UI | ⬜ | | |
| T2.7 | Sort | ⬜ | | |
| T2.8 | Grouping | ⬜ | | |
| T2.9 | Highlight | ⬜ | | |
| T2.10 | View management | ⬜ | | |
| T2.11 | Search & aggregates | ⬜ | | |
| T2.12 | Type change | ⬜ | | |
| T2.13 | Form view | ⬜ | | |
| T2.14 | Auto-index job | ⬜ | | |
| T2.15 | Perf pass | ⬜ | | |
| T3.1 | Realtime hub | ⬜ | | |
| T3.2 | Presence | ⬜ | | |
| T3.3 | Comments & notifications | ⬜ | | |
| T3.4 | Row history | ⬜ | | |
| T3.5 | Undo/redo | ⬜ | | |
| T3.6 | Trash & purge | ⬜ | | |
| T3.7 | Export | ⬜ | | |
| T3.8 | Import | ⬜ | | |
| T3.9 | Backups | ⬜ | | |
| T3.10 | User lifecycle admin | ⬜ | | |
| T3.11 | Hardening | ⬜ | | |
| T3.12 | Release v1.0 | ⬜ | | |
| T4.1 | Kanban view | ⬜ | | |
| T4.2 | Gallery view | ⬜ | | |
| T5.1 | Calendar view | ⬜ | | |
| T5.2 | SMTP | ⬜ | | |

## Acceptance test status

Summary updated at each milestone end.

| Area | Total | Passing | Notes |
|---|---|---|---|
| AT-0xx Foundation | 6 | 0 | AT-002 migration idempotence verified with PostgreSQL 18 and repeated CLI runs. Full AT-006 remains pending Linux bash/Docker + T0.7 harness checks. |
| AT-1xx Auth & Admin | 17 | 0 | |
| AT-2xx Structure & Permissions | 13 | 0 | |
| AT-3xx Fields & Grid | 20 | 0 | |
| AT-4xx Relations & Computed | 11 | 0 | |
| AT-5xx Views | 12 | 0 | |
| AT-6xx Collaboration | 10 | 0 | |
| AT-7xx Import/Export/Backup | 8 | 0 | |
| AT-8xx Non-functional | 6 | 0 | |
| AT-9xx Post-v1 | 4 | 0 | |

## Open questions / follow-ups

| # | Question | Raised | Status |
|---|---|---|---|
| OQ-1 | Domain name & HTTPS: use bundled Caddy, or an existing proxy on the server? | 2026-10-24 | Resolved: existing independent Caddy container → D-046 |
| OQ-2 | Where will CI run (GitHub Actions, Gitea, local only)? | 2026-10-24 | Resolved: local builds (Podman on Windows / Docker on Linux), images transferred → D-047, D-048 |
| OQ-3 | Default timezone for the team (for "today" filters, display): Asia/Shanghai? | 2026-10-24 | Resolved: Asia/Shanghai default, per-user override → D-049 |
| OQ-4 | Currency default for currency fields (CNY?) | 2026-10-24 | Resolved: CNY default, admin-adjustable, per-field override → D-050 |

## Change log

| Date | Change |
|---|---|
| 2026-10-24 | Initial planning docs created from interview (Q1–Q51). |
| 2026-10-24 | Resolved OQ-1..4: external Caddy, local Podman/Docker build + image transfer, `ptdev` tool, timezone & currency defaults. Added 14-dev-build-release.md, D-046..D-050, AT-005/006/117/326. |
| 2026-10-24 | T0.1: Go scaffold + stdlib ptdev commands and unit tests; D-051; local gate and Podman PG18 lifecycle verified; T0.2 handoff added. Existing stopped Podman machine started. |
| 2026-10-24 | T0.2: implementation started. |
| 2026-10-24 | T0.2: config precedence/validation, CLI routing, JSON logging, and T0.3 task prompt implemented; focused tests, `ptdev check`, and PowerShell/Git Bash CLI smoke checks pass. `golangci-lint` unavailable; full AT-006 remains pending. |
| 2026-10-24 | T0.3: implementation started. |
| 2026-10-24 | T0.3: pgx pool/retries, safe transaction helper, advisory-locked embedded Goose migration, and migrate CLI complete. PostgreSQL 18.6 integration, repeated CLI migrations, and `ptdev check` pass; `golangci-lint` unavailable. Full AT-006 remains pending. |

### T0.1 verification evidence (2026-10-24)

- `go env GOVERSION GOOS GOARCH`: `go1.27.0`, `windows`, `amd64` (direct `go version` produced no terminal output; installed `Go/VERSION` independently agrees).
- `podman version`: client 5.8.2, server 5.8.4; `node -v`: v24.15.0.
- Fresh Windows PowerShell and Git Bash: `help`, `version`, `engine` all exit 0; engine is Podman (PATH preference).
- `ptdev check`: lint PASS, test PASS, build PASS. `gofmt -l .` empty and `go vet ./...` clean; golangci-lint not installed, so its checks remain unverified.
- `db up`, `db status`, `db down`, `db status`, `db up`: container created, running, stopped and restarted correctly; volume retained. `podman exec pt-dev-db psql -U postgres -c "select version()"` returns PostgreSQL 18.6.
- `build --linux`: creates `bin/pandatable-linux-amd64`; native `bin/pandatable.exe version` equals ptdev's computed Git version.
- Direct ptdev executable: missing Docker override exits 1 with a PATH error; invalid `PT_ENGINE=foo` exits 2; future commands exit 2 with their task IDs. `go run` reports these child exit codes through its own wrapper.
- With Podman removed from PATH: version, lint, build and `test --no-db -run TestFormatVersion -v -count=1` pass.
- No integration tests, HTTP/config packages, frontend scaffold, deployment files or third-party Go dependencies added. Full AT-006 remains pending, not claimed as passed.

### T0.3 verification evidence (2026-10-24)

- PostgreSQL 18.6: disposable-database test passed with two concurrent migration invocations, an additional rerun, schema/extension checks, and real transaction commit/rollback checks.
- Ran `go run ./cmd/pandatable migrate` twice against the disposable database; both runs logged `database migrations complete` with no errors. A separate no-database run clearly skipped the integration test when `PT_TEST_DATABASE_URL` was unset.
- `go run ./tools/ptdev check`: lint, tests, and build PASS; `gofmt` and `go vet` clean. `golangci-lint` is unavailable, so its checks were not run.
- AT-002's migration idempotence behavior is verified; the application restart scenario will be exercised once the T0.4 server startup path exists. Full AT-006 remains pending Linux/Docker and T0.7 harness checks.
