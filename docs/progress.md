# Progress

Status legend: ⬜ Not started · 🟨 In progress · ✅ Done · ⏸ Blocked · ❌ Dropped

## Milestones

| Milestone | Status | Started | Finished | Tag | Notes |
|---|---|---|---|---|---|
| Planning | ✅ | 2026-10-24 | 2026-10-24 | — | Interview completed; docs created |
| M0 Foundation | 🟨 | 2026-10-24 | | | T0.1–T0.3 scoped checks complete; T0.4 implementation verified; T0.5 done. AT-002/003 and AT-802 pass; full AT-001 and AT-006 remain pending. |
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
| T0.4 | HTTP server | 🟨 | 2026-10-24 | Implementation and scoped verification complete; real embedded T0.5 SPA now passes browser/CSP/history checks. Kept in progress until linked AT-001 passes with T0.6 Compose and Linux/Docker. |
| T0.5 | Web scaffold (i18n, theme toggles) | ✅ | 2026-10-24 | AT-003 passes in the real embedded production SPA; AT-802 passes lint/parity with negative fixtures. Vue/TS shell, local preferences, direct-process dev supervisor, web build/copy, ten frontend tests, real dev proxy/cleanup, ptdev check and Linux compilation verified. |
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
| AT-0xx Foundation | 6 | 2 | AT-002 and AT-003 pass. Real embedded SPA, health, assets, history and CSP pass locally; full AT-001 still awaits Compose/Linux-Docker. Full AT-006 remains pending Linux bash/Docker + T0.7 harness checks. |
| AT-1xx Auth & Admin | 17 | 0 | |
| AT-2xx Structure & Permissions | 13 | 0 | |
| AT-3xx Fields & Grid | 20 | 0 | |
| AT-4xx Relations & Computed | 11 | 0 | |
| AT-5xx Views | 12 | 0 | |
| AT-6xx Collaboration | 10 | 0 | |
| AT-7xx Import/Export/Backup | 8 | 0 | |
| AT-8xx Non-functional | 6 | 1 | AT-802 passes for the T0.5 scaffold: 21 matching keys, untranslated-text/attribute lint and negative fixtures. Remains a continuous local gate. |
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
| 2026-10-24 | T0.4: implementation started. |
| 2026-10-24 | T0.4 implementation verified: chi HTTP foundation, safe JSON logs/errors, embedded-asset boundary, automatic migrations and bounded shutdown. Focused tests, PostgreSQL 18.6 restarts, ptdev check and Linux cross-build pass; D-053..D-055 and T0.5 handoff added. AT-001 remains pending, so T0.4 and M0 stay in progress. |
| 2026-10-24 | T0.5 started: inspected build/HTTP boundaries and Node/pnpm compatibility; implemented localized Vue shell and direct-process dev workflow. |
| 2026-10-24 | T0.5 complete: AT-003 embedded-browser and AT-802 local gate pass; ten frontend tests, Go checks, production build, real dev proxy/cancellation and Linux compilation verified. Added D-056/D-057 and T0.6 prompt; M0/T0.4 remain in progress pending stack acceptance. |

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

### T0.4 verification evidence (2026-10-24)

- Focused HTTP/web/CLI/config tests pass. HTTP routes and middleware also pass
  without `PT_TEST_DATABASE_URL`; HTTP statement coverage is 96.2%, embedded-web
  package coverage 100%. Health requires no DB or frontend assets.
- Tests cover deterministic GET/HEAD health, reserved API/health namespaces
  (including encoded paths), JSON errors, actual static assets and history
  fallback using an in-memory SPA fixture, missing/empty assets (503), generated
  IDs, log redaction/levels, security headers, gzip, panic recovery/partial-response
  abort, graceful drain, forced shutdown deadline, and listener failure.
- `TestServePostgreSQLRestarts` passes on PostgreSQL 18.6
  (`server_version_num=180006`): two real `serve` startups each return health 200
  and API 404, stop gracefully, retain exactly one unchanged migration record,
  and leave zero DB connections. An occupied port fails explicitly. Disposable
  databases are dropped; a follow-up query found zero `pt_test_t04_*` databases.
  This verifies AT-002's application restart scenario.
- `go run ./tools/ptdev check`: lint, test and native build PASS on Windows/Podman.
  Formatting and `go vet` pass. `golangci-lint` is unavailable and was not run.
- `go run ./tools/ptdev build --linux` passes (linux/amd64, CGO disabled).
  This is compilation evidence, not Linux/Docker runtime acceptance.
- The built Linux binary also passes two startups in a disposable Linux/Podman
  container against a disposable PostgreSQL 18.6 DB: health 200, unchanged
  migration record, SIGTERM shutdown with exit 0, no ERROR events or connection
  URLs in JSON logs. The smoke container and database were removed. This is
  direct-binary smoke coverage, not the future Docker/Compose stack acceptance.
- The new serve integration test clearly skips with an unreachable PostgreSQL
  URL; the no-DB run also passes. The existing dev database/container was retained.
- AT-001 is only partially verified: local health/routing and fixture SPA tests
  pass. The real Vue SPA awaits T0.5, Compose awaits T0.6, and Linux/Docker stack
  verification remains pending. Full AT-006 remains pending Linux/Docker and the
  T0.7 harness. No deployment files, frontend scaffold or application services
  were added. The combined `ptdev dev` supervisor remains for T0.5.

### T0.5 verification evidence (2026-10-24)

- Environment: Windows, Go 1.27.0, Node 24.15.0, pnpm 11.2.2, PostgreSQL 18.6.
  Vite 7 and the i18n build plugin support the documented Node >=22.13 floor.
  Dependencies are lockfile-pinned; esbuild is the only allowed install script.
- `pnpm lint`: ESLint (including translated-text/visible-attribute rules),
  Prettier, strict `vue-tsc`, and 21-key en/zh-CN parity PASS. Negative fixtures
  reject template text, interpolated strings, static labels/placeholders/accessible
  names, and bound string-literal attributes. Locale failure and blocked-storage
  tests verify explicit error state instead of a false applied/saved result.
- `pnpm test`: 3 files / 10 tests PASS. Covers actual Naive UI theme/locale/date
  locale wiring, dayjs/document language, browser/saved preference precedence,
  both languages, all three theme modes, restoration, OS changes and disposal.
- `ptdev web build`, production build/copy and `ptdev check` PASS. Generated
  files stay ignored; `.gitkeep` survives copying. Go-only tests/build pass both
  before and after real assets exist. Source validation tests preserve old assets
  for missing/empty indexes. All emitted HTML/JS/CSS total about 130 KB gzip,
  below the initial bundle budget even before excluding lazy chunks.
- Real embedded production browser check at loopback: English → Chinese and
  light → dark update immediately; reload restores both. Naive UI card background
  changes from white to `rgb(24, 24, 28)`. Light/system also persist; system follows
  emulated OS changes, explicit light overrides dark OS. Desktop and 375px mobile
  controls are adjacent, with no horizontal overflow. AT-003 PASS.
- The actual Go server returns SPA/history 200, built JS 200, missing asset 404,
  health JSON 200 and `/api/v1/missing` JSON 404. Deep client route renders the
  translated not-found surface and returns to welcome. Browser console has no
  warnings/errors and the production CSP reports zero violations; no CSP changes.
- Opt-in `TestDevIntegration` PASS against disposable `pt_test_t05_47ba5bb1`:
  direct backend/Vite startup, health/API proxy and cancellation; both listeners
  close and the temporary executable directory is removed. Repeated supervisor
  tests cover cancellation, nonzero/zero unexpected exits and start failures.
  Disposable DB dropped after verification with zero remaining connections;
  no verification server remains. Existing dev DB/container retained.
- Linux/amd64, CGO-disabled compilation of app and ptdev PASS. This is not
  Linux/Docker runtime acceptance. `golangci-lint` is absent and was not run.
  Full AT-001 awaits T0.6 Compose/Linux-Docker; AT-006 awaits Linux/Docker and
  T0.7's shared harness. No auth, grid, deployment files or shared harness added.
- Next: [T0.6 image & deploy prompt](task-prompts/T0.6.md).
