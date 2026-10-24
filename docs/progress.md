# Progress

Status legend: ⬜ Not started · 🟨 In progress · ✅ Done · ⏸ Blocked · ❌ Dropped

## Milestones

| Milestone | Status | Started | Finished | Tag | Notes |
|---|---|---|---|---|---|
| Planning | ✅ | 2026-10-24 | 2026-10-24 | — | Interview completed; docs created |
| M0 Foundation | 🟨 | 2026-10-24 | | | T0.1–T0.3 scoped checks complete; T0.4/T0.6/T0.7 implementations verified; T0.5/T0.8 done. D-005 confirmed by the isolated spike. AT-002/003/004 and AT-802 pass; full AT-001/005/006 remain pending Linux/Docker and an approved server. |
| M1 Core | 🟨 | 2026-10-24 | | v0.1.0 | T1.1 Done; T1.2 backend auth verified, but explicitly backend-only: browser AT-102/116 and full AT-117 remain pending. AT-103/803 pass. T1.3 setup prompt ready; no setup/ACL/admin/business services yet. |
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
| T0.4 | HTTP server | 🟨 | 2026-10-24 | Implementation and scoped verification complete; real embedded SPA/Compose/browser/CSP/history checks pass on Windows/Podman. Kept in progress until linked AT-001 also passes on Linux/Docker. |
| T0.5 | Web scaffold (i18n, theme toggles) | ✅ | 2026-10-24 | AT-003 passes in the real embedded production SPA; AT-802 passes lint/parity with negative fixtures. Vue/TS shell, local preferences, direct-process dev supervisor, web build/copy, ten frontend tests, real dev proxy/cleanup, ptdev check and Linux compilation verified. |
| T0.6 | Image & deploy files | 🟨 | 2026-10-24 | Implementation verified: AT-004 image contents pass (161,964,427 bytes); isolated Windows/Podman Compose/browser/restart/persistence and gzip export/load checks pass. Full AT-001/005 await Linux/Docker and an approved real Caddy destination; not Done. |
| T0.7 | Test harness & local gate | 🟨 | 2026-10-24 | Implementation and Windows/Podman verification complete: shared PG18 helper, four real shell E2E checks, failure/cancellation/partial-start cleanup and ptdev check pass. Linked AT-006 still requires Linux bash/Docker; not Done. |
| T0.8 | RevoGrid spike | ✅ | 2026-10-24 | Scoped objectives demonstrated: opt-in lazy DataGrid, synthetic 200k window with <=300 row cache, real clipboard/editor/range/fill/group/freeze/theme and emulated tablet checks; D-066..068 confirm D-005. Windows check and image-backed E2E pass; no production query/mobile acceptance claimed. |
| T1.1 | Meta migrations | ✅ | 2026-10-24 | Forward 0002 adds all 14 remaining metadata tables; 0001 unchanged. D-069..071 clarify composite FKs, identifier/name rules, polymorphic grants and history. Real PG18 catalog/default/constraint/delete/upgrade/down-up tests and AT-002 restarts pass; Windows check and 8-check image E2E pass (2 intentional mobile exclusions). T1.2 handoff recorded. |
| T1.2 | Auth service | 🟨 | 2026-10-25 | Backend verified: bounded argon2id, hash-only PG sessions/expiry/revocation, auth/profile/timezone APIs, alias/IP limits, trusted proxies, CSRF/cookies. Windows check and image E2E pass; AT-103/803 pass. User chose backend-only: browser AT-102/116 and full AT-117 remain pending, so not Done. T1.3 handoff created. |
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
| AT-0xx Foundation | 6 | 3 | AT-002/003/004 pass. AT-001 Windows/Podman automated Compose/E2E passes; Linux/Docker pending. AT-005 export/manifests/Podman load pass, but Docker load/server/Caddy unverified. AT-006 shared harness, unavailable-DB skips and Windows gate pass; Linux/Docker pending. |
| AT-1xx Auth & Admin | 17 | 1 | AT-103 passes on real HTTP/PG18 with alias-shared lock and controllable time. AT-102/116 API evidence only; browser UI deferred by user. AT-117 profile/catalog/default persistence only, not real datetime/filter/admin behavior. T1.2 remains in progress. |
| AT-2xx Structure & Permissions | 13 | 0 | |
| AT-3xx Fields & Grid | 20 | 0 | |
| AT-4xx Relations & Computed | 11 | 0 | |
| AT-5xx Views | 12 | 0 | |
| AT-6xx Collaboration | 10 | 0 | |
| AT-7xx Import/Export/Backup | 8 | 0 | |
| AT-8xx Non-functional | 6 | 2 | AT-802 preserved: 61 matching keys, lint/negative fixtures. AT-803 passes on real HTTP/PG18 plus focused cookie/proxy/CSRF tests, with the user-approved literal-loopback HTTP exception (D-073). No NFR-2/3 or full-load AT-801 claim. |
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
| 2026-10-25 | T1.2 backend implementation verified and handed off; D-072..077, static sqlc auth queries, API/types/testing docs and T1.3 prompt added. Windows check, required PG18 and final 8/2 image E2E pass. Kept T1.2 in progress because the user explicitly deferred auth/profile UI and full AT-117. No shared/remote deployment or unrelated task completion. |
| 2026-10-24 | Initial planning docs created from interview (Q1–Q51). |
| 2026-10-24 | T1.1 started with a clean worktree; required design/runtime/test docs inspected. Started the existing stopped Podman machine and retained dev DB without reset. User confirmed grant-reference and history clarifications before coding. |
| 2026-10-24 | T1.1 Done: reversible metadata 0002, exact PG18 catalog/default/constraint/delete tests, sentinel-preserving concurrent upgrades and populated down/up, three real serve lifecycles with unchanged migration records (AT-002). Windows check, shell/spike E2E and AT-004 pass. D-069..071, data-model/testing docs, completion record and T1.2 prompt added; unrelated platform acceptance remains pending. |
| 2026-10-24 | T0.8 started with a clean worktree; pinned engine/adapter APIs and isolated prototype design inspected. |
| 2026-10-24 | T0.8 scoped spike complete: 200k window/cache, custom editor/renderer, quoted TSV and fill, grouped sample, frozen columns, translations/themes and emulated tablet demonstrated under unchanged CSP. D-066..068 confirm D-005; 20 unit tests, Windows local gate and eight image-backed browser checks pass (two intentional mobile exclusions). T1.1 handoff created; unrelated platform acceptance remains pending. |
| 2026-10-24 | Resolved OQ-1..4: external Caddy, local Podman/Docker build + image transfer, `ptdev` tool, timezone & currency defaults. Added 14-dev-build-release.md, D-046..D-050, AT-005/006/117/326. |
| 2026-10-24 | T0.1: Go scaffold + stdlib ptdev commands and unit tests; D-051; local gate and Podman PG18 lifecycle verified; T0.2 handoff added. Existing stopped Podman machine started. |
| 2026-10-24 | T0.2: implementation started. |
| 2026-10-24 | T0.7: implementation started after reading the T0.6 handoff, binding decisions and current tooling; worktree clean. |
| 2026-10-24 | T0.7 implementation/scoped verification complete: shared migrated package databases with bounded skips and fail-closed gate, pinned Playwright and isolated Compose runner, real success/failure/cancellation cleanup, D-063..D-065 and T0.8 handoff. Full AT-006 awaits Linux/Docker, so T0.7/M0 remain in progress. |
| 2026-10-24 | T0.2: config precedence/validation, CLI routing, JSON logging, and T0.3 task prompt implemented; focused tests, `ptdev check`, and PowerShell/Git Bash CLI smoke checks pass. `golangci-lint` unavailable; full AT-006 remains pending. |
| 2026-10-24 | T0.3: implementation started. |
| 2026-10-24 | T0.3: pgx pool/retries, safe transaction helper, advisory-locked embedded Goose migration, and migrate CLI complete. PostgreSQL 18.6 integration, repeated CLI migrations, and `ptdev check` pass; `golangci-lint` unavailable. Full AT-006 remains pending. |
| 2026-10-24 | : implementation started. |
| 2026-10-24 | T0.4 implementation verified: chi HTTP foundation, safe JSON logs/errors, embedded-asset boundary, automatic migrations and bounded shutdown. Focused tests, PostgreSQL 18.6 restarts, ptdev check and Linux cross-build pass; D-053..D-055 and T0.5 handoff added. AT-001 remains pending, so T0.4 and M0 stay in progress. |
| 2026-10-24 | T0.5 started: inspected build/HTTP boundaries and Node/pnpm compatibility; implemented localized Vue shell and direct-process dev workflow. |
| 2026-10-24 | T0.5 complete: AT-003 embedded-browser and AT-802 local gate pass; ten frontend tests, Go checks, production build, real dev proxy/cancellation and Linux compilation verified. Added D-056/D-057 and T0.6 prompt; M0/T0.4 remain in progress pending stack acceptance. |
| 2026-10-24 | T0.6 implementation and scoped verification: source-allowlisted image, executable healthcheck, Compose/Caddy examples, image export and opt-in remote tooling. AT-004 and Windows/Podman stack/browser/persistence checks pass; full AT-001/005/006 remain pending. D-058..D-062 and T0.7 handoff added. |

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

### T0.6 verification evidence (2026-10-24)

- Environment: Go 1.27.0, Node 24.15.0, pnpm 11.2.2, Podman client/server
  5.8.2/5.8.4, Compose provider 2.40.3. Docker and golangci-lint are not on PATH.
  No remote destination/authorization was supplied; no server/shared Caddy
  resources were changed.
- Focused CLI/config/ptdev tests PASS; `ptdev check` PASS (lint, Go + ten frontend
  tests, native build). Go vet/format, TypeScript, translation lint/parity pass.
  Latest cleanup-only adjustment additionally passes tooling tests/vet.
  Linux/amd64 ptdev cross-compilation passes. Optional golangci-lint not run.
- The POSIX generated deployment scripts execute successfully in an isolated
  Linux/Podman Go container using a fake Docker executable, including directory
  spaces/apostrophes, preservation of the other `.env` lines, and explicit
  preflight/load/start/probe failures with no later commands after failure.
  These are shell/unit tests, **not** a real remote or Linux/Docker deployment.
- `ptdev image build` PASS with both version and latest tags. Binary reports
  `0.0.0-dev+d710743.dirty`; tag is `localhost/pandatable:0.0.0-dev-d710743.dirty`.
  Image ID `151798547521a5843869fd2125fc684533ea6cfec42c824a5a591d877ca7f0e4`:
  linux/amd64, UID/GID 10001, executable HEALTHCHECK present in inspection and
  Docker archive config. Final post-load uncompressed size **161,964,427 bytes**
  (<200,000,000); layer tar members total 161,957,376 bytes.
  `pg_dump --version`: **18.6 (Debian 18.6-1.pgdg12+2)**. CA/timezone data present;
  no Node, Go or curl in runtime. **AT-004 PASS** via Podman + archive inspection.
- A disposable context-copy build verifies source/workspace/lockfiles and
  `.gitkeep` are available, but node_modules, generated assets, Git metadata,
  docs/tools, deploy config, bin/dist, local env/npm config and Go tests do not
  enter the context. Empty excluded host directories were also excluded.
- Compose project `pt-t06-6417`, loopback port 18086, fresh DB/app volumes:
  health **200 in 6.534 s** after `up -d` began. Built JS, SPA/deep routes 200;
  missing assets (including extensionless ones), reserved API/health subpaths
  and dotfiles return JSON 404. Image and DB become healthy.
- Integrated browser: real embedded login and translated deep-route not-found
  render; English -> Chinese and dark mode persist across reload; **zero**
  console warnings/errors, page errors or CSP violations. AT-003/802 preserved.
  No auth/business/WebSocket endpoints or shared E2E harness added.
- App as UID 10001 writes persistent files and can write backups/tmp. Two app
  restarts preserve exactly one migration-1 record with timestamp
  `2026-10-24 07:36:36.738052`. Graceful stop exits **0 in 3.983 s**, leaving zero
  application DB connections. `down` without `-v`, then `up --wait`, preserves
  both DB migration state and the file sentinel.
- Server config has **no app/DB host ports**, external proxy network and alias
  `pandatable`; empty trusted-proxy setting stays empty. Local override binds
  only `127.0.0.1:18086`; DB volume is `/var/lib/postgresql`.
- `ptdev image save --with-postgres` PASS. Final gzip archives:
  `dist/pandatable-0.0.0-dev-d710743.dirty.tar.gz` = **54,937,045 bytes**;
  `dist/postgres-18.tar.gz` = **161,810,248 bytes**. Full gzip CRC/EOF and Docker
  `manifest.json`, referenced config/layers/tags/platform are verified, including
  the app HEALTHCHECK. Both archives load successfully with Podman.
  Docker load compatibility is structurally checked, **not runtime-certified**.
- Native missing-image export fails with exit 1 and leaves no final/staging
  archive; missing Docker override fails without fallback. Missing deploy host
  or `--yes` exits 2 without SSH. Healthcheck without listener exits 1 without
  DB config; serve still rejects missing DB config. Tests cover redirects,
  non-200, refusal, invalid flags, wildcard binds, proxy bypass and the real
  2-second deadline; export failures/cancellation preserve prior files and clean
  staging, including Windows paths with spaces.
- Disposable containers/networks/volumes/context images and temporary fixtures/
  binaries removed. Existing `pt-dev-db` and `pt-dev-pgdata` retained; no broad
  prune/reset performed. Release images/archives remain for reuse.
- **Pending:** full AT-001 on Linux/Docker; AT-005 Windows archives -> Ubuntu
  `docker load`/Compose -> approved existing Caddy HTTPS; AT-006 Linux/Docker and
  T0.7's shared harness. A Linux container under Podman is not a substitute.
  T0.4, T0.6 and M0 remain in progress. Next:
  [T0.7 test harness & local gate](task-prompts/T0.7.md).

### T0.7 verification evidence (2026-10-24)

- Implemented [internal/testdb](../internal/testdb/): lazy migrated PG18
  databases per package from template0, 3 s connection bound, explicit absent/
  dial/DNS skips, redacted operational failures, fresh bounded FORCE cleanup
  after tests/initialization failures. DB and CLI tests share the helper; the
  real dev test now uses `PTDEV_TEST_DEV=1` plus the same helper instead of
  requiring a manually provisioned database.
- `ptdev test/check` requires reachable integration DBs and defaults to
  uncached Go tests. Existing lint -> test -> build summary/exit behavior is
  preserved; E2E stays a separate milestone command. Focused unit tests cover
  gate continuation/failure/cancellation, E2E arguments, direct-child EOF/
  reaping, readiness validation and build/start/test/cleanup error propagation.
- Real PG18 integration PASS via
  `ptdev test -run "TestInitialMigrationPostgreSQL|TestServePostgreSQLRestarts|TestSuiteIsolationPostgreSQL|TestDevIntegration" -v -count=1`
  with `PTDEV_TEST_DEV=1`: separate package databases, migration reruns/concurrency,
  same-suite reuse, isolated row state, two real application restarts with an
  unchanged single migration record, zero remaining application connections,
  forced cleanup of an intentionally open connection and DB removal. Real
  backend/Vite proxy/listener cleanup also passes.
- Real auth failure SQLSTATE **28P01** and missing DB **3D000** fail without
  leaking connection details or becoming unavailable/skips. Invalid URL is a
  failing test. With `PT_TEST_DATABASE_REQUIRED=1`, a closed port is a failure.
  Plain `go test ./... -v -count=1` against closed loopback port **49704** PASS
  in **10.795 s** wall time (includes compilation/unrelated bounded unit tests);
  each DB integration reports its clear skip in **0.00-0.01 s**. Unit tests
  enforce the helper bound separately. No developer DB was stopped.
- Final `ptdev check` PASS in Windows PowerShell/Podman: lint **11.143 s**,
  test **19.119 s**, build **8.595 s**, total **38.856 s**. Includes gofmt/vet,
  real Go/PG tests, all **10** Vitest tests, strict E2E/frontend TypeScript,
  ESLint/Prettier, **21-key** locale parity/negative lint fixtures and frozen
  production build. Optional `golangci-lint` remains absent and was not run.
- Pinned dev-only `@playwright/test` **1.64.1** adds only its three lockfile
  packages; Chromium provisioning (`pnpm --dir web e2e:install`) and frozen
  installation PASS. Workspace manifest remains alongside lockfile/package
  manifest in the real Dockerfile. No new Go/runtime dependencies.
- Final `ptdev e2e` PASS: project **pt-e2e-23xngb5vnvv2v7dbyhm462kzbx**,
  loopback **53175**, health ready in **6.238 s** after startup began (<60 s).
  **4 tests PASS in 16.1 s**, one worker, desktop/mobile Chromium: actual
  embedded SPA/JS, health JSON/security headers, deep-route/reload/history,
  missing static/extensionless asset and reserved API/health/dotfile boundaries,
  HEAD/405, locale and all three theme modes/reload/live OS changes, actual
  card colors, adjacent controls and no overflow. No console warnings/errors,
  page errors or CSP violations; script policy unchanged. AT-003/802 preserved.
- Final opt-in lifecycle regressions PASS:
  `PTDEV_TEST_E2E=1 go test ./tools/ptdev -run "TestE2EFailureAndCancellationIntegration|TestE2EStartupFailureIntegration" -v -count=1`
  (set the environment variable using the host shell's syntax).
  Deliberate browser assertion failure at **57479** (project
  **pt-e2e-o7soh6ojypzlugpri2xi3hzymj**, health **7.445 s**) and active-browser
  cancellation at **56989** (project **pt-e2e-3uf6xxuotolkjlidu7cho7xenx**,
  health **7.693 s**) both report failure and remove owned containers, volumes,
  networks, image tag and HTTP listener. Recorded Node runner, worker and Chrome
  PIDs are gone. Cancellation retains its context cause. Real occupied-port
  partial Compose startup also fails before browser execution and removes all
  owned stack resources. Redacted logs/traces/screenshots remain ignored under
  `web/test-results/`; temporary configs are removed.
- An initial concurrent gate/probe run exposed a generated-probe source/lint
  collision and CPU-sensitive existing 5 s lint-test timeout. Probes now use
  a committed non-acceptance `.probe.ts` plus configs only under ignored
  test-results; no source generation or timeout relaxation. The final gate
  and lifecycle checks were run separately and pass.
- Prior AT-004 release image reinspection still gives linux/amd64,
  UID/GID **10001:10001**, executable HEALTHCHECK, **161,964,427 bytes** and
  `pg_dump 18.6`. New E2E tags are disposable and do not overwrite `latest` or
  the original `0.0.0-dev+d710743.dirty` release.
- Linux/amd64 app/tooling remain buildable; ptdev executable and its tests
  cross-compile PASS. This is **not Linux/Docker runtime acceptance**. Docker
  is absent from PATH and no approved Ubuntu/Caddy destination exists.
  AT-001 Linux/Docker, full AT-005 and AT-006 Linux bash/Docker are unverified;
  T0.4/T0.6/T0.7/M0 stay in progress. No auth/setup/business/seed/WebSocket
  endpoints or future acceptance passes are claimed.
- Only owned disposable resources were removed. Developer `pt-dev-db`/
  `pt-dev-pgdata`, unrelated engine resources and shared/remote Caddy were not
  reset or removed. No verification server remains.
- Next: [T0.8 RevoGrid spike](task-prompts/T0.8.md), using the new gates and
  preserving CSP, translations, isolation and honest platform limitations.

### T0.8 verification evidence (2026-10-24)

- **Done for the scoped spike; D-005 confirmed by D-068.** The unlinked
  `/_spikes/revogrid` lab requires explicit mount at >=768 px. The engine is
  behind [DataGrid](../web/src/components/grid/DataGrid.vue); welcome and
  unknown/deep routes remain unchanged. No auth, query/seed/business endpoints,
  database performance results, mobile-card editor or production grid shipped.
- Pin MIT engine/adapter **4.29.1**: exactly two added packages, no other runtime
  dependencies. Verified installed declarations/source and official
  [Vue API](https://rv-grid.com/guide/vue3/),
  [editor](https://rv-grid.com/guide/vue3/editor),
  [clipboard](https://rv-grid.com/guide/clipboard) and
  [grouping](https://rv-grid.com/guide/row/grouping) guides before wiring.
  The package TSV splitter does not handle quoted multiline cells; the wrapper
  supplies a tested parser/formatter rather than changing production CSP.
- Synthetic 200,000-row provider: 100-row requests, current page +/-1, **<=300
  cached row objects**, one shared immutable placeholder for other slots,
  last-20 requests, <=5,000 transient edited cells. Generation guards ignore
  out-of-order results even if a provider ignores AbortSignal. Request timers
  cancel on navigation/unmount; edits validate atomically, persist only in the
  bounded mount-local overlay and survive page eviction. Engine slot/index
  arrays are **O(N)**; this is not an O(visible rows) engine-memory claim.
- Focused provider/TSV tests passed before the full gate. Final
  `go run ./tools/ptdev check` PASS: lint **11.196 s**, test **21.454 s**,
  build **11.914 s**, total **44.563 s**. Includes required live PG18
  integration/restart tests, **20 Vitest tests in six files**, strict Vue/E2E
  TS, ESLint/Prettier, **61-key** parity/negative fixtures and frozen build.
  Optional `golangci-lint` is absent and was not run.
- Final `go run ./tools/ptdev e2e` PASS: project
  **pt-e2e-lcv32fpwafg4pxv32hn7bbthtg**, loopback **61528**, health **6.414 s**,
  **8 passed / 2 intentional skips in 27.3 s**. All four original desktop/mobile
  shell checks remain green. The two skips exclude desktop clipboard/fill and
  a duplicate tablet observation from the mobile project; mobile no-grid
  isolation is actually tested. Browser warning/error/page/CSP diagnostics are
  empty. No inline/eval script policy relaxation.
- Real Chromium at **1440x1000**, grid **1392x440**:

  | Position | Requested zero-based ranges | Cached rows | DOM row fragments / cells / descendants | Navigation observation |
  |---|---|---|---|---|
  | Start | 0-99, 100-199 | 200 | 32 / 80 / 199 | First visible row after mount: 302 ms; already-loaded start check: 125 ms |
  | Middle | 99900-99999, 100000-100099, 100100-100199 | 300 | 32 / 80 / 199 | 240 ms |
  | End | 199800-199899, 199900-199999 | 200 | 30 / 75 / 191 | 223 ms |
  | Return to start | 0-99, 100-199 fetched again | 200 | 32 / 80 / 199 | 226 ms |

  Pinned and main row fragments represent the same visible logical rows, not
  twice as much data. Engine source length stays 200,000 with only 201/301
  unique object references including the shared placeholder. Observed completed
  request durations **42.5-137.2 ms** include deliberate 40/120 ms timers and
  browser scheduling; navigation observations include Playwright overhead.
  These are **not NFR-2/3 or AT-312 server/query measurements**.
- Chromium CDP JS used heap: pre-mount after GC **3,457,500 bytes**;
  mounted retained heap after GC **9,348,768 / 9,692,588 / 9,762,312 /
  9,795,748 bytes** at the four positions; unmounted after GC **6,134,812
  bytes**. Warm engine modules/shared helpers remain resident. Window/document
  listener inventories do not grow across two additional mount/unmount cycles;
  engine process-lifetime delegated listeners are not claimed to disappear.
  Unit tests separately verify responsive-listener removal, request/timer
  cancellation, no callbacks after dispose, stale responses and the exact
  cumulative 5,000-cell overlay cap.
- Actual production-CSP interactions PASS: Vue multiline note editor with
  Enter save/Escape cancel, quantity renderer, Shift+arrow and mouse-drag
  rectangular selection, system clipboard Ctrl+C/V round-trip of tabs,
  embedded newlines and doubled quotes, wrapper Ctrl+D, built-in drag fill,
  translated oversized/unloaded-copy rejection, frozen-column position during
  horizontal scrolling, en/zh-CN headers and light/dark grid colors/screenshots.
  A separate **100-row read-only grouping sample** collapses both groups to
  two headers and expands one to 52 visible items. It does not group partially
  loaded 200k data or implement server counts/aggregates.
- Emulated touch at **1024x1000**: tap focuses cell **(0,0)**; actual CDP
  touch swipe moves the grid to **scrollTop 216 px**. Resizing to 700 px
  unmounts the grid and displays the mobile notice. No page overflow; this is
  not physical iPad/Android/Safari certification. Clipboard tests grant Chromium
  read/write permission on loopback; browser/OS permissions and secure contexts
  still apply, and Excel/Sheets applications were not directly automated.
- Build gzip gate PASS: conservative initial shell (including both locales
  and welcome imports/CSS) **135,087 bytes <600,000**; **all assets 255,685
  bytes**, including lazy engine and optional plugin chunks. Initial shell
  resources contain no grid chunks. No bundle-budget exception required.
  Image-context allowlist includes the required build checker, not test tools.
- New disposable image: **162,373,088 bytes**, linux/amd64,
  UID/GID **10001:10001**, executable HEALTHCHECK and successful runtime
  `pg_dump --version` probe. Startup `podman stats` snapshot: app **5.308 MB**,
  DB **97.68 MB**; neither is a production-load/NFR-1 certification. The grid
  runs in the browser, not in server RAM; no new Go/server dependencies.
  AT-003/004/802 preserved.
- The successful E2E runner removed only its named containers, volumes,
  networks and unique image tag. Native verification server on **18088** was
  stopped before the final gate; no verification listeners remain. Developer
  DB/volume, original release tags/archives, shared Caddy and unrelated resources
  were not reset/deleted. On evening continuation Podman is stopped/unreachable;
  no engine was restarted merely to repeat inspection. Earlier successful
  teardown is the cleanup evidence, not a new engine inspection.
- **Remaining:** production FieldKind codecs, transactional bulk writes,
  add-row/HTML clipboard UX, vertical-only fill policy, server-flattened group
  rows/counts/aggregates, complete keyboard/undo behavior and full browser/device
  matrix. No AT-305..313 pass claimed. Full AT-001/005/006 Linux/Docker/server
  checks remain pending; T0.4/T0.6/T0.7/M0 remain in progress.
- Full implementation, interaction matrix, caveats and next task:
  [T0.8 completion](task-prompts/T0.8.md#implementation-and-handoff-2026-10-24).
  Next: [T1.1 Meta migrations](task-prompts/T1.1.md).

### T1.1 verification evidence (2026-10-24)

- **T1.1 Done; linked AT-002 passes.** Added
  [0002_metadata.sql](../migrations/0002_metadata.sql) with all 14 remaining
  metadata tables. Existing 0001 is byte-for-byte unchanged in Git; it still
  owns users/sessions/settings and extensions. No new dependencies, application
  APIs, real accounts, physical book/user tables, ACL/lifecycle/collaboration/
  backup services or frontend changes.
- D-069..D-071 and [data model](05-data-model.md#integrity-and-service-boundaries-d-069d-071)
  record composite field/table and same-book FKs, physical-name reservations
  through Trash, active-field case-insensitive names, field If-Match version,
  view ownership/storage-shape/scalar checks and the explicit rollback order.
  User confirmed polymorphic grants remain service-validated (no target FKs or
  triggers), and table-level type_change history uses NULL row_id with a table
  purge FK. Those future services must handle grant races/cleanup, last-owner/
  orphan rules and physical/file/reverse-link cleanup; this is not implemented
  or implied by metadata cascades.
- Environment: Windows/amd64, **Go 1.27.0**, **Node 24.15.0**, **pnpm 11.2.2**,
  **Podman client/server 5.8.2/5.8.4**, **Compose 2.40.3**,
  **PostgreSQL 18.6** (`180006`, Debian `18.6-1.pgdg13+2` on the retained dev
  container), **Playwright 1.64.1**, **Chromium 156.0.8078.4**. The existing
  stopped Podman machine and dev DB were started without resetting anything.
  Docker and golangci-lint remain absent; no approved server/Caddy destination.
- Focused PowerShell commands from the repository root:

  ```text
  go run .\tools\ptdev test -run 'TestEmbeddedMigrationOrdering|TestInitialMigrationPostgreSQL|TestMetadata|TestServePostgreSQLRestarts' -v -count=1
  go run .\tools\ptdev test -run 'TestMetadata|TestServePostgreSQLRestarts' -v -count=2
  ```

  Both PASS with **live, required PG18**, not skipped integration evidence.
  [Catalog tests](../internal/db/metadata_catalog_test.go) verify all 17 meta
  tables, exact new column order/types/nullability/default expressions, ALWAYS
  identity, validated constraints and valid/ready unique/lookup indexes.
  [Behavior tests](../internal/db/metadata_behavior_test.go) check real UUIDv7,
  timestamps, JSON/boolean/version/NULL defaults, multiple virtual columns,
  table-level history and **54** expected SQLSTATE failures in the constraint
  suite (unique/FK/NOT NULL/check/identity), plus Trash/restore/name checks.
  Valid enums and same-book self-links pass. Seven deletion scenarios cover
  user, group, display/relation field, table, book and workspace, including
  authorship SET NULL, personal-view/session/member/notification cascades,
  surviving sibling scopes, and intentionally service-owned grant references.
- [Upgrade tests](../internal/db/metadata_upgrade_test.go) run two populated
  Down-to-0001/up cycles only inside the Suite-owned DB. Sentinel users
  (including explicit auth/preferences/timestamps), session bytea/IP/expiry/
  user-agent and nested JSON settings compare unchanged. Extension OIDs/
  versions, migration-1 record and an unrelated metadata table/data survive.
  Each upgrade has two concurrent advisory-locked callers, followed by a rerun;
  versions 1/2 each have exactly one applied record, with unchanged records on
  the rerun. Down removes all 14 new tables and the identity sequence, not the
  original schema/data/extensions/unrelated object. The `-count=2` run also
  verifies the fixture cleanup permits repeating tests in the same package DB.
- **AT-002:** [serve test](../cmd/pandatable/serve_test.go) performs an initial
  real serve plus **two restarts**, each with health 200, API 404, graceful
  shutdown, no ERROR/connection-URL logs and zero remaining app connections.
  All Goose records, including IDs/timestamps and both applied versions, remain
  exactly unchanged across all three lifecycles. The occupied-port case remains
  an explicit failure. Package TestMain cleanup drops the owned databases.
- Final `go run .\tools\ptdev check`: **PASS**, lint **22.715 s**, test
  **26.926 s**, build **18.824 s**, total **68.472 s**. Includes gofmt/vet,
  required uncached Go/PG18 tests, **20 Vitest tests in six files**, strict
  frontend/E2E TS, ESLint/Prettier, **61** matching non-empty locale keys and
  negative i18n fixtures, frozen build and native binary. Optional
  golangci-lint was not run. Initial shell gzip stays **135,087 bytes
  <600,000**, all assets **255,685 bytes**; no grid in initial imports.
- Final `go run .\tools\ptdev e2e` with a read-only image-inspection/short-lived
  pg_dump probe at health readiness: **PASS**, project
  **pt-e2e-qllx7olo7expau2swdbpjam2jt**, loopback **50903**, health
  **6.354 s**, **8 passed / 2 intentional mobile exclusions in 32.9 s**.
  All shell/spike checks remain green; browser/CSP checks report no errors.
  AT-003/802 preserved; no frontend CSP/i18n/test-policy changes.
  **AT-004 preserved:** image **162,381,278 bytes <200,000,000**,
  linux/amd64, UID/GID **10001:10001**, executable
  `CMD pandatable healthcheck`; runtime pg_dump **18.6**
  (`18.6-1.pgdg12+2`).
- An earlier identical browser run also passed 8/2 (health **9.695 s**,
  **39.9 s** browser duration), but the supplemental PowerShell probe failed
  because it read Docker's `Config.Healthcheck` instead of Podman's top-level
  `Healthcheck`. That wrapper failure was not hidden or an app regression:
  inspected the engine format, corrected only the ephemeral probe, and reran
  the full combined verification successfully. No application/assertion changes.
- Ordinary no-DB compatibility was checked separately using
  `go test .\internal\db .\cmd\pandatable -run 'TestMetadata|TestServePostgreSQLRestarts' -v -count=1`
  with the URL unset, then an explicit closed loopback port 1. Every DB test
  clearly skipped in **0.00-0.01 s**; these skips are not counted as integration
  passes. No shared PostgreSQL container was stopped for that check.
- Cleanup verified read-only: **zero** remaining `pt_test_*` databases; both
  named E2E projects have no containers/volumes/networks/tag, and neither
  **55806** nor **50903** has a listener. The runtime probes/browser version
  probe exited and reaped themselves. Dev DB is still running; `pt-dev-pgdata`
  and original release image are retained. No broad prune/reset, shared Caddy/
  network/remote changes, persistent verification server or committed test
  artifacts. Ignored browser results/build outputs remain local.
- **Pending, unchanged:** full AT-001 Linux/Docker, AT-005 approved Ubuntu/
  Caddy transfer/load/HTTPS and AT-006 Linux bash/Docker. Linux image builds
  under Podman do not satisfy these. M0/T0.4/T0.6/T0.7 stay in progress.
  No auth/setup/ACL/production data-grid or future acceptance passes claimed.
- Implementation/limitations/handoff: [T1.1 record](task-prompts/T1.1.md#implementation-and-handoff-2026-10-24).
  Next: [T1.2 Auth service](task-prompts/T1.2.md).

## T1.2 backend verification and handoff (2026-10-25)

**Backend delivered and verified; T1.2 remains in progress, not Done.** Started
2026-10-24. User explicitly selected backend-only scope, passwords of 8-32
UTF-8 bytes, literal-loopback HTTP cookie exception, and all-session revocation
after password change. No browser AT-102/116 or full AT-117 pass is claimed.
M0/T0.4/T0.6/T0.7 and their Linux/Docker/server checks remain unchanged.

### Implementation and security state

- [Auth service](../internal/auth/service.go), password/token/limit helpers and
  [HTTP handlers](../internal/httpapi/auth.go): username/email login, logout,
  identity, password and profile preferences, plus authenticated PG timezone
  catalog. Mirrored [API types](../web/src/api/types.ts); no auth frontend.
  D-072..D-077 and the [API contract](09-api.md#t12-auth-contract-d-072d-077)
  record policy, request/response/error shapes and future service obligations.
- Argon2id m=65536 KiB,t=1,p=2, random 16-byte salts, 32-byte keys, exact bounded
  canonical PHC validation and constant-time key comparison. A single fail-fast
  worker bounds active KDF work; contention returns explicit 503. Added only
  x/crypto **v0.58.0** and transitive x/sys **v0.49.0**, not an auth framework.
- Sessions use 32 random bytes as canonical base64url; only SHA-256 of the
  encoded token is stored. Real PG tests inspect digest/IP/user-agent/timestamps,
  UTC connections and last_login_at. Seven-day idle and 30-day absolute expiry
  are independently enforced even with artificially extended expires_at.
  Monotonic conditional UPDATE cannot resurrect a revoked row.
- Eight concurrent validators vs logout leave zero session rows. A separate
  test observes a real blocked PG user-row lock, commits a password change and
  revocation, then proves the already-verified old-password login cannot insert
  a surviving session. Change-password clears must_change_password and revokes
  all sessions atomically; the old password fails and the new one works.
- Five bad credentials, including invalid password lengths, lock the resolved
  account for a minute; username/email/case aliases share that ID. Correct
  passwords are rejected during lock; deterministic deadline tests verify reset
  at the exact boundary. Ten/minute/IP includes malformed JSON and unsupported
  content types after CSRF/IP validation (D-077 supersedes D-075's exemption).
  Maps cap at 4096 and fail closed rather than evicting live protection.
- Trusted X-Forwarded-For is parsed right-to-left, stopping at an untrusted hop;
  empty trust ignores spoofed forwarding. Both exact configured Origin and
  X-PT-CSRF: 1 are required for unsafe requests. HTTPS cookies are host-only,
  Path=/, HttpOnly, Secure, SameSite=Lax; only literal-local HTTP omits Secure
  with a startup warning. Non-loopback HTTP auth configuration is rejected.
- Typed errors, no-store responses, bounded JSON/DB contexts and accurate API
  405 Allow headers are tested. No passwords, PHC strings, session tokens or
  email identifiers appear in access/error logs; internal diagnostics use a
  fixed message and request ID. Existing no-inline/no-eval-script CSP remains.
- Profile display name/locale/theme/timezone persist across service recreation.
  Timezone validates against real PG18 catalog; NULL follows the current team
  default (Asia/Shanghai when absent). Invalid stored strings/JSON null/types
  fail explicitly. Forced-change users cannot edit profile/list timezones.
  Disabled users cannot authenticate even if a row remains; future admin
  disable/reset must serialize and revoke in the same transaction.
- **0001/0002 unchanged, no new migration.** Pinned development-only sqlc
  **1.31.1** generates static typed bindings; regeneration is byte-identical.
  Its offline timezone catalog declaration is never executed/migrated or
  included as a runtime schema change. Build-context allowlists are unchanged.

### Exact verification

- Versions: Go **1.27.0 windows/amd64**, Node **24.15.0**, pnpm **11.2.2**,
  Podman client/server **5.8.2/5.8.4**, Compose **2.40.3**, PostgreSQL **18.6**
  (`18.6-1.pgdg13+2` dev DB), Playwright **1.64.1**, actual Chromium
  **156.0.8078.4**, sqlc **1.31.1**. Docker/golangci-lint and an approved
  Ubuntu/Caddy destination remain unavailable.
- Required live PG18, uncached focused command:
  `go test .\internal\auth .\internal\httpapi -count=1 -v`, with explicit
  PT_TEST_DATABASE_URL and PT_TEST_DATABASE_REQUIRED=1: **PASS**.
  Subsequent focused expiry/blocked-login tests with `-count=2` also pass;
  fixture rows cleanly repeat in one Suite-owned database.
- Final extra boundary command with required PG18:
  `go test .\internal\auth .\internal\httpapi -run 'TestLoginLimitsAndAliasesPostgreSQL|TestAuthHTTPPostgreSQL|TestMalformedLoginIPLimitHTTPPostgreSQL|TestAuthInputAndInternalErrorsHTTPPostgreSQL' -v -count=1`:
  **PASS**. Ten malformed requests with forged untrusted forwarding consume the
  socket-IP budget; a correct eleventh request returns 429. API 405 responses
  advertise the actual methods, not the shell's GET/HEAD-only allowance.
- `go test .\internal\auth -run '^$' -fuzz '^FuzzParsePHC$' -fuzztime=3s -parallel=2`:
  **PASS**, **76,057** executions, seven new interesting inputs, no crash.
- `go test .\internal\auth -run '^$' -bench BenchmarkPasswordHash -benchtime=3x -benchmem -cpu=2`:
  **59,977,533 ns/op**, **67,113,466 B/op**, **30 allocs/op**, three iterations
  on local Intel i7-11850H with two Go CPUs. This is a local KDF observation,
  not certification of full target-host load/RSS.
- Final `go run .\tools\ptdev check`: **PASS**, lint **15.987 s**, test
  **25.454 s**, build **12.651 s**, total **54.093 s**. Includes gofmt/vet,
  required uncached Go/PG18 tests, AT-002 initial serve plus two restarts with
  unchanged migration records, **20 Vitest tests / six files**, strict
  frontend/E2E TS, ESLint/Prettier, **61** matching non-empty locale keys and
  negative fixtures, frozen web build and native binary. Optional golangci-lint
  was not run. Initial gzip **135,087 <600,000** bytes, all assets **255,685**;
  no grid in the initial imports.
- Final `go run .\tools\ptdev e2e` with GOPROXY set to the public mirror:
  **PASS**, project **pt-e2e-kzzrsztolv6nch4wydf6rw75j6**, loopback **50236**,
  health **8.003 s**, **8 passed / 2 intentional mobile exclusions in 34.7 s**.
  Existing shell/spike assertions and browser/CSP diagnostics pass unchanged.
  This suite is not auth/profile browser acceptance.
- Supplemental read-only inspection while that owned stack was ready:
  AT-004 image **162,627,041 bytes <200,000,000**, linux/amd64, UID/GID
  **10001:10001**, exec-form `CMD pandatable healthcheck`; runtime pg_dump
  **18.6** (`18.6-1.pgdg12+2`). Podman startup snapshot: **74.1 MB / 536.9 MB**
  container memory, **4.42% CPU**, **314.591 ms CPU time**, **13 pids**.
  This is a startup/shell observation, not AT-801's 15-minute ten-user workload.
- Earlier pre-boundary image run also passed **8/2 in 32.5 s**, health
  **7.25 s**, image **162,622,944 bytes**, project
  **pt-e2e-yvbqikgte7573wtc2lcop3y4un**, loopback **52567**. The final run above
  verifies the stricter request limit and method-header implementation.
- Ordinary absent-URL and closed-loopback-port compatibility:
  `go test .\internal\auth .\internal\httpapi -run 'TestSessionLifecyclePostgreSQL|TestAuthHTTPPostgreSQL' -v -count=1`
  clearly skips the DB tests in **0.00-0.01 s**. Those skips are not integration
  passes. Shared PostgreSQL was not stopped.
- Failures resolved without weakening assertions: default Go proxy lookup timed
  out, so public-module retrieval used goproxy.cn; first sqlc generation could
  not resolve PG's timezone catalog, fixed by its generator-only declaration;
  initial check failed only Prettier on the new API type file, fixed using the
  existing formatter. Final complete gate and E2E both pass.

### Acceptance, cleanup and next task

AT-103 and AT-803 pass at their specified API surface. AT-102/116 have real
HTTP/PG service evidence only; translated errors, login/profile UI and
back-button/browser behavior remain pending by user scope choice. AT-117 has
profile/catalog/default persistence evidence only; actual datetime fields,
per-user display/filter behavior and admin default-change UI are not claimed.
AT-002/003/004/802 are preserved. Full AT-001/005/006 Linux/Docker/approved
server acceptance remains pending; no shared/remote deployment.

TestMain dropped all owned databases: read-only verification found **zero**
remaining pt_test_* DBs. Both named E2E projects have zero containers, volumes,
networks or image tag; **50236/52567** have no listener. Short-lived pg_dump/
browser-version probes exited. Dev DB remains running with pt-dev-pgdata and
existing release images retained. No prune/reset/shared Caddy/network changes.
Generated web assets, ignored browser results and temporary tools/logs are not
committed.

Implementation/limitations: [T1.2 record](task-prompts/T1.2.md#implementation-and-handoff-2026-10-25).
Next prompt: **[T1.3 Setup flow](task-prompts/T1.3.md)**. It carries forward
single-worker hashing, session/CSRF/cookie/proxy constraints, owned PG18 tests,
unchanged migrations/CSP/resources and the requirement to record completion
and create T1.4's prompt. T1.3 itself is not started by this handoff.
