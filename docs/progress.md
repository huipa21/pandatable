# Progress

Status legend: ⬜ Not started · 🟨 In progress · ✅ Done · ⏸ Blocked · ❌ Dropped

## Milestones

| Milestone | Status | Started | Finished | Tag | Notes |
|---|---|---|---|---|---|
| Planning | ✅ | 2026-10-24 | 2026-10-24 | — | Interview completed; docs created |
| M0 Foundation | 🟨 | 2026-10-24 | | | T0.1–T0.3 scoped checks complete; T0.4/T0.6/T0.7 implementations verified; T0.5/T0.8 done. D-005 confirmed by the isolated spike. AT-002/003/004 and AT-802 pass; full AT-001/005/006 remain pending Linux/Docker and an approved server. |
| M1 Core | 🟨 | 2026-10-24 | | v0.1.0 | T1.1/T1.3/T1.4/T1.5/T1.6/T1.7/T1.10/T1.11 Done for M1 scope. Real sidebar/schema-shell AT-201/208/213/full AT-112 and M1 AT-203 removal pass alongside auth/admin tests. T1.8 still awaits full AT-111/322 upload enforcement; T1.2 AT-102/116/full AT-117, field/filter AT-202/804, sharing/grid/rows and M3 restore remain pending. ACL/grants API verified (AT-204..212); no future UI acceptance inferred. 2026-10-27 phase review: 11 tasks Done, 9 In progress; both gates are green (`ptdev check` with default parallelism, `ptdev e2e` 17 passed/0 failed/2 intentional skips) after D-145/D-146. Remaining M1 work: AT-311 column ops (no `PATCH /views/{id}`), AT-305 keyboard navigation, the missing self-profile UI for AT-116/117, the AT-102 browser scenario and full AT-301..304; AT-312/NFR-2 stay environment-blocked. |
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
| T1.3 | Setup flow | ✅ | 2026-10-25 | First-run token/status/admin/session, process-safe one-time setup and localized desktop/mobile UI. AT-101 passes in production-image Chromium; required PG18 gate and full local checks pass. D-078; no migration. |
| T1.4 | ACL resolver & grants API | ✅ | 2026-10-26 | Resolver (one SQL/lookup, flat groups, path-only visibility, LRU+epoch invalidation), grants list/PUT/DELETE with scope-owner, last-owner, hidden-404/visible-403, principal validation and workspace-lock/D-070 serialization; `require` guard. D-079/D-080, no migration. AT-204..212 pass at the API surface (209-212 fully; UI parts of 204-208 await T1.10-T1.12). Windows check and 9/2 image E2E pass. |
| T1.5 | Admin users | ✅ | 2026-10-26 | Admin-only list/search/create/PATCH/reset, normalized collision-safe identities, last-active-admin and self-management rules, atomic session revocation and ACL invalidation; responsive localized users UI and login/forced change. D-081..083, no migration/dependencies. Required PG18 tests x3, Windows check, production-image AT-104..108 desktop/mobile and 11/2 regression suite pass; T1.6 handoff recorded. |
| T1.6 | Admin groups | ✅ | 2026-10-26 | Group CRUD/member API, localized group/user detail and immediate ACL invalidation; D-084..087, no migration/dependency. Required PG18 x3 and production desktop/mobile AT-109 pass; full image suite 11/2. Full check passes with command-scoped GOFLAGS=-p=1 after existing testdb cleanup deadline failures under parallel package load; exact caveat/evidence below. T1.7 handoff recorded. |
| T1.7 | Admin dashboard | ✅ | 2026-10-26 | Read-only cached metrics and localized responsive dashboard; D-088..090, gopsutil v4.25.9, no migration. Required PG18 x3, non-root Linux filesystem tests and production desktop/mobile AT-110 pass; refresh 9904/9855.5 ms. Full image suite 11/2; complete check passes with GOFLAGS=-p=1, default-parallel cleanup caveat retained. Real post-auth RSS exceeds NFR-1 target; not certified. T1.8 handoff recorded. |
| T1.8 | Admin settings & workspaces list | 🟨 | 2026-10-26 | Seven atomic settings/default readers/public branding and read-only workspace inventory, D-091..093. T1.11 now closes full AT-112 with real Alice UI creation/admin sidebar denial/table404/populated inventory. Full AT-111/322 still awaits T2.2 upload enforcement, so In progress. Earlier SQL fixtures remain API-only evidence; original checks/handoff preserved. |
| T1.9 | DDL module | 🟨 | 2026-10-26 | Physical-only caller-pgx.Tx DDL, strict 50-bit identifiers, typed types/defaults and explicit constraint removal; D-094..096, no migration/dependency. Required PG18 x3 (100% statements), serial-package full check and production image 11/2 pass. Default-parallel cleanup failure remains; AT-202/full AT-804 await real table/field/filter APIs. T1.10 handoff recorded; not Done. |
| T1.10 | Workspace/Book/Table API | ✅ | 2026-10-26 | API implementation unchanged; T1.11 closes real M1 browser AT-201/203/213 and full AT-112. Required uncached PG18 and scoped-serial complete gate pass; default-parallel cleanup caveat persists. Done for M1 structure scope: AT-203 restoration remains M3, AT-202 field rename T1.14 and full AT-804 T2.6, not claimed. |
| T1.11 | Sidebar & navigation UI | ✅ | 2026-10-26 | D-104..107: memory-only nav refresh/identity guards, owner-of-parent tree controls, path-only breadcrumbs, native create/rename/Trash dialogs, drag/button reorder, mobile drawer and lazy schema-only table routes. Real desktop/mobile AT-201/208/213/full AT-112 and M1 AT-203 pass. 52 unit tests/304 locale keys, required PG18 and serial full check pass; image suite 11 passed/2 intentional exclusions. Initial gzip 178,390 bytes, no grid imports. Default-parallel cleanup/RSS caveats unchanged; exact record below and T1.12 prompt created. |
| T1.12 | Share dialog UI | 🟨 | 2026-10-26 | M1 sharing implementation verified: D-108..112, target-owner native dialog, direct/inherited review, scoped minimal real directory, localized failures and race-safe nav/schema refresh. Real desktop/mobile sharing AT-204..211 scope and full AT-208..211 pass; full AT-204..207 rows/editors/shared views still await T1.14..T1.19, so In progress. 66 frontend tests/348 keys; required PG18 and scoped-serial full check pass; full image suite 11 passed/2 intentional exclusions, initial gzip 184,252 bytes/no initial grid. Default-parallel Suite cleanup and NFR-1 caveats retained. Exact record below; T1.13 prompt created. | |
| T1.13 | Codec framework + batch 1 | 🟨 | 2026-10-26 | Implementation verified: 12 writable + five read-only system codecs/FieldKinds, typed DDL/defaults, exact wire/paste/copy, safe localized components, shared vectors and required uncached PG18. D-113..D-122; no migration/dependency/routes/grid integration. 384 frontend tests/397 keys, initial gzip 185755 bytes; serial full check passes, final image 11 passed/2 intentional exclusions. Default-parallel Suite cleanup and NFR-1 caveats remain. Full AT-301/302/304 awaits T1.14..T1.16; handoff recorded. |
| T1.14 | Field API & UI | 🟨 | 2026-10-26 | Implementation verified: D-123..D-130 table-owner field create/rename/description/options/required/unique/default/reorder/soft-delete/display field with If-Match, typed DDL, exact preflights, select label rewrites and same-tx schema versions; lazy FieldPanel on the real schema page. 394 frontend tests/487 keys, required PG18, serial full check and production image 11 passed/2 intentional exclusions; real AT-202 field rename passes. Default-parallel cleanup and NFR-1 caveats remain. Full AT-301/303 await T1.15/T1.16 rows/grid; handoff and T1.15 prompt recorded. |
| T1.15 | Basic query & rows API | 🟨 | 2026-10-26 | Implemented D-132..D-137 query/row routes, typed codec I/O, bounded atomic writes, ACL/schema locking, deterministic bulk-patch lock ordering, API DTOs and localized errors. Focused PG18/API suites, scoped-serial full check and production E2E pass; default-parallel Suite cleanup failures remain. 200k local API timings recorded, but AT-312 and full AT-301/302/303/304 still need the T1.16 production grid. Handoff recorded. |
| T1.16 | DataGrid component | 🟨 | 2026-10-27 | Production grid is wired on the real table/view route with bounded query paging and row edits; full browser acceptance and target-host performance remain pending. |
| T1.17 | Copy/paste & fill-down | ✅ | 2026-10-27 | Production-grid AT-306..309 pass in desktop Chromium: codec-backed TSV copy/cut/paste, one valid-cell bulk patch with invalid-cell reporting, confirmed tail-row growth, formatted date/select copy, Ctrl+D and vertical fill handle. Desktop and mobile production scenarios pass; en/zh-CN and light/dark covered. 406 frontend tests, lint/typecheck/i18n and 189,948-byte initial gzip gate pass; focused uncached PG18 row/query tests and scoped-serial full check pass. Default-parallel Suite cleanup/restart failures remain; final full E2E is 8 passed/5 failed/2 skipped (failures are outside T1.17); AT-312 target-host performance not measured. |
| T1.18 | Row expand modal | ✅ | 2026-10-27 | D-140: lazy full-row dialog, Space/icon and direct links; all 12 codec editors, exact values, read-only system/reader fields, bounded single PATCH and stale guards. Full production-image AT-310 passes (desktop 43.8 s/mobile fallback 2.5 s). 441 frontend tests, lint/528 keys, required PG18 and serial full gate pass; initial gzip 190337 bytes. Full E2E 11 pass/4 unrelated mobile fail/2 skips; default Suite cleanup and uncertified NFR/AT-312 caveats remain. T1.19 handoff recorded. |
| T1.19 | Mobile layout | ✅ | 2026-10-27 | D-141/D-142: lazy <768px cards without RevoGrid, 100-row pages/<=300 cached rows, bounded options/FieldPanel/delete, reused full-screen editors and defaults-preserving FAB creation. Full production-image AT-313 passes at 390x844 (15.6 s/17.3 s); AT-310 and AT-302/306..309 preserved. 485 frontend tests, lint/543 keys, required PG18 and serial full gate pass; initial gzip 190849 bytes. Full E2E 12 pass/5 fail/2 skips (four prior mobile failures plus desktop navigation 404 diagnostic); default Suite cleanup and uncertified AT-312/NFR-1 remain. Exact results/cleanup and T1.20 handoff in [completion record](task-prompts/T1.19.md#implementation-and-handoff-2026-10-27). |
| T1.20 | Seed & perf baseline | 🟨 | 2026-10-27 | D-143/D-144 safe CLI seed and persistent production-browser workflow implemented/tested. Final local real-200k baseline: 605.7ms first usable paint, 137.8..192.3ms middle scroll/paint, exact fresh requests/100-row pages/<=300 models; preceding strict local run failed 379.3/350.3ms and is retained. No authorized 2-vCPU/4-GB Ubuntu target; configured VM 8 CPUs/2048 MiB, engine reports a different WSL host boundary. AT-312/NFR-1 remain uncertified. 485 frontend tests, lint/543 keys, required PG18 and serial full gate pass; initial gzip 190849 bytes. Full image regression 13 pass/4 prior mobile fail/2 skips; default-parallel Suite cleanup fails separately. Exact evidence/owned cleanup in [record](task-prompts/T1.20.md#implementation-and-handoff-2026-10-27). |
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
| AT-1xx Auth & Admin | 17 | 9 | AT-101/103 and T1.5..T1.7 AT-104..110 pass (real production-image desktop/mobile plus PG18). T1.8 partial AT-111/112; uploads/workspace creation/navigation/table APIs remain blockers. Full AT-102/profile AT-116 remain pending. AT-117 includes team-default propagation, not real datetime/filter UI. T1.2/T1.8 remain in progress. |
| AT-2xx Structure & Permissions | 13 | 4 | AT-209..212 pass at the API surface. T1.10 adds real HTTP/PG18 API passes for AT-201/203/213 and the table-rename half of AT-202; their sidebar/Trash UI, field rename and AT-204..208 UI parts await T1.11/T1.12/T1.14. T1.14 adds the real field-rename half of AT-202 (see the T1.14 evidence; this summary count is not recomputed here). |
| AT-3xx Fields & Grid | 20 | 0 | |
| AT-4xx Relations & Computed | 11 | 0 | |
| AT-5xx Views | 12 | 0 | |
| AT-6xx Collaboration | 10 | 0 | |
| AT-7xx Import/Export/Backup | 8 | 0 | |
| AT-8xx Non-functional | 6 | 2 | AT-802 preserved: 261 matching keys, lint/negative fixtures. AT-803 passes on real HTTP/PG18 plus focused cookie/proxy/CSRF tests, with the user-approved literal-loopback HTTP exception (D-073). Post-auth RSS remains a known NFR-1 risk; no NFR-1/2/3 or full-load AT-801 certification. |
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
| 2026-10-26 | T1.10 implemented: workspace/book/table API (`internal/structure`), atomic metadata+DDL, system fields/Name/default Grid, reorder, soft delete, `/nav`, D-097..D-103 and sqlc `structuresql`. Required PG18 tests, serial full check and image E2E 11/2 pass; default-parallel Suite cleanup failure recorded. Sidebar UI and linked UI/field acceptance stay pending, so In progress; T1.11 prompt created. Retained Podman VM/dev DB restored stopped. |
| 2026-10-26 | T1.6 completed: group CRUD/members, linked group/user detail, D-084..087, deterministic grant/member/delete concurrency and immediate ACL invalidation. AT-109 production desktop/mobile and full 11/2 image suite pass; full check passes with GOFLAGS=-p=1 (default-parallel testdb cleanup failures recorded, not relaxed). T1.7 prompt created; original stopped DB/VM state restored. |
| 2026-10-26 | T1.4 implemented and verified: `internal/acl` resolver/grants service, `/api/v1/grants` routes, `require` guard, D-079/D-080, sqlc `aclsql`, docs and T1.5 prompt. Required PG18 tests (x3), Windows check and image E2E pass. No migration or shared/remote deployment. |
| 2026-10-25 | T1.3 started after reading the required docs and T1.2 handoff; D-078 records the confirmed setup policies. AT-101 and the required PG18/local gates now pass; exact evidence follows. No migration or shared/remote deployment. |
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
T1.3 is now complete; next is **[T1.4 ACL resolver & grants API](task-prompts/T1.4.md)**.

## T1.3 setup verification and handoff (2026-10-25)

**T1.3 Done.** D-078 records a 32-byte process-local bootstrap token, 24-hour
expiry and restart rotation; an advisory-locked transaction rechecks both the
zero-user condition and setup marker before atomically inserting the first
admin, session and `setup_completed` setting. No migration was added and
0001/0002 are unchanged. Username is trimmed/lowercased and restricted to
`[a-z0-9._-]{3,32}`. The required email is trimmed/lowercased and saved for
future SMTP/login use. Password remains 8-32 UTF-8 bytes. The setup UI is
localized in English/Simplified Chinese, redirects first-run SPA routes to
`/setup`, and ends at an authenticated identity/completion surface; `/login`
remains welcome-only.

### Verification

- Environment: Go **1.27.0**, Node **24.15.0**, pnpm **11.2.2**, Podman
  client/server **5.8.2/5.8.4**, Compose **2.40.3**, PostgreSQL **18.6**,
  sqlc **1.31.1**, Playwright **1.64.1**, Chromium **156.0.8078.4**.
  Docker, golangci-lint and an approved Ubuntu/Caddy destination remain
  unavailable.
- Focused required PG18 tests passed uncached:
  `go test ./internal/auth ./internal/httpapi -run 'TestSetup|TestAuthCSRFAndLimitsHTTPPostgreSQL' -v -count=1`
  and `go test ./internal/auth -run '^TestConcurrentSetupProcessesPostgreSQL$' -v -count=1`.
  They cover wrong/expired token, setup status, exact user/hash/session data,
  successful session authentication, rollback with no partial rows or token
  consumption, completion 410, active/disabled existing users, two independent
  services racing through PG, HTTPS cookies, CSRF/Origin, safe logging and
  restart token rotation/rejection. Suite TestMain dropped its owned databases.
- `go run ./tools/ptdev check`: **PASS**, required uncached PG18; lint **17.298 s**,
  tests **31.549 s**, build **16.374 s**, total **65.222 s**. Includes gofmt,
  vet, **20 Vitest tests**, strict TS, lint/Prettier, **80** matching locale keys
  and the native build. Optional golangci-lint was not run.
- `go run ./tools/ptdev e2e`: **PASS**, **9 passed / 2 intentional mobile
  exclusions in 45.2 s**, health **6.934 s**. AT-101 exercises the production
  image: setup redirect, wrong-token 401, desktop and Pixel 7 form layout,
  admin creation, authenticated `/auth/me`, completion status and POST 410.
  No browser/CSP errors. The isolated startup snapshot was **72.8 MB / 536.9 MB**
  and **11.63% CPU**; this is a single observation, not AT-801 load certification.
  Initial shell gzip is **138,976 bytes**, under the 600,000-byte cap.
- Pinned `go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate` succeeds;
  bindings are reproducible. The generator was not on PATH, so the pinned
  `go run` invocation was used without adding runtime dependencies.

### Resolved failures and cleanup

The first browser attempt found Naive UI's visual labels were not associated
with native inputs; translated `aria-label` input props fixed that. Chromium
reports expected API 401/410 statuses as generic console errors, so E2E now
filters only those setup-route responses and explicitly asserts both statuses;
all other console/page/CSP errors remain fatal. A provider-specific Compose
`ps` banner made resource IDs unavailable; the final snapshot uses direct
read-only engine `ps` filtered to the owned Compose project/service. No
acceptance assertions, CSP rules or locale checks were weakened.

The final E2E project, app/DB containers, volumes, networks and unique image
were removed. Suite inspection found zero remaining `pt_test_*` databases.
The existing `pt-dev-db` container is restored to stopped, with
`pt-dev-pgdata` preserved. Release images, shared networks/Caddy, generated
web assets and browser artifacts were not committed; no remote deployment was
performed. AT-001/005/006 Linux/Docker/server checks and T1.2 browser AT-102/116
plus full AT-117 remain pending.

Next: **T1.4 ACL resolver & grants API**. The prompt carries forward D-069..D-078,
session/auth contracts, metadata integrity and polymorphic-grant service rules,
PG18 test isolation, no migration unless justified, CSP/i18n/resource limits,
and the required completion record plus T1.5 prompt handoff.

## T1.4 ACL verification and handoff (2026-10-26)

**T1.4 Done at its API surface.** `internal/acl` implements the resolver and
grants service; `GET/PUT /api/v1/grants` and `DELETE /api/v1/grants/{id}` are
mounted by `serve`; `authHTTP.require(level, type, param)` is the reusable
404/403 guard for later resource routes. D-079 (single-SQL lookup with flat
groups joined in SQL, path-only visibility, LRU 4096/5 s TTL plus process epoch
`Invalidate()`; replaces doc 06's NOTIFY sketch under D-045) and D-080
(owner-only list/PUT/DELETE, hidden=404 / visible-but-low=403, workspace
`FOR NO KEY UPDATE` + key-share locks serializing with hard deletion,
principal 404/409, active-direct-user last-owner rule) are recorded. No
migration; 0001/0002 unchanged; second sqlc entry generates `internal/acl/aclsql`
byte-identically with pinned 1.31.1.

### Verification

- Go **1.27.0**, PostgreSQL **18.6**, Podman **5.8.2**, sqlc **1.31.1**;
  Docker/golangci-lint/race (cgo off) unavailable.
- `go test ./internal/acl ./internal/httpapi -count=3` with required PG18:
  **PASS** (12.9 s / 14.9 s). Covers the resolver matrix, group union and
  membership, disabled/admin/trashed, path-only visibility, cache hit/epoch/TTL/
  LRU/race-with-invalidation, validation, scope-owner boundaries, last-owner
  edge cases, orphaned workspaces, rollback/lock release, 12 rounds of
  concurrent last-owner removal, concurrent upserts and hard-delete
  interleavings, plus HTTP AT-204..212, CSRF/Origin/auth/forced-change/disabled
  and body/query/method rules.
- `go run ./tools/ptdev check`: **PASS**, lint **14.228 s**, test **26.956 s**,
  build **12.582 s**, total **53.766 s** (first run failed only gofmt on
  CRLF-created files; fixed with `gofmt -w`).
- `go run ./tools/ptdev e2e` (goproxy.cn mirror): **PASS**, **9 passed / 2
  intentional mobile skips in 39.4 s**, health **6.474 s**, shell gzip
  **138,976 bytes**, app snapshot **74.3 MB / 536.9 MB, 12.08% CPU** (startup
  observation, not AT-801).
- Cache/query behavior: one SQL statement per uncached lookup; repeated lookups
  within TTL and epoch cost zero queries; a committed mutation invalidates, a
  rejected one does not.

## T1.13 codec implementation and handoff (2026-10-26)

- D-113..D-122 define bound typed Go codecs and matching pure frontend
  FieldKinds for 12 writable and five read-only system kinds. Strict options,
  native DDL/defaults, exact bigint/decimal/microsecond wire values, nullable
  normalization, DST/date boundaries, choices and safe email/URL components
  are implemented without new dependencies, migrations or production routes.
- Shared vectors and native Suite-owned PG18 tests pass; final required
  uncached PG-only run **0.959 s**. Final frontend **384 tests / 20 files**,
  **397 matching keys**, lint/strict TS/i18n pass. Initial gzip **185755 bytes**
  (<600000), no initial grid/codec registry imports; all assets **327517 bytes**.
- Final command-scoped `GOFLAGS=-p=1` full check passes: lint **26.446 s**,
  tests **2m12.244 s**, build **13.568 s**, total **2m52.258 s**. Default full
  check separately fails only Suite cleanup in codec/db/testdb, total
  **58.936 s**; no deadline/assertion relaxation. golangci-lint absent (warning).
- Final production-image project `pt-e2e-obbw2axpyjzzmksipdtklhfnpq`:
  **11 passed / 2 intentional mobile-grid exclusions (3.5m)**. Real auth/
  admin/navigation/sharing fixtures, both locales/themes and strict diagnostics
  pass. An intermediate repeat failed the existing desktop nav drag-order
  assertion; recorded without changing/retrying assertions or harness bounds.
- Final measured RSS desktop **169738240 / 169869312**, mobile
  **181809152 / 181809152 bytes**, so NFR-1 remains **not certified**.
  Disposable stacks/tagged images removed; retained DB and VM restored stopped,
  two original containers/37 volumes/one network preserved. Six new build-stage
  cache images retained (137 versus 131), no broad prune or unrelated deletion.
- T1.13 remains **In progress** until full AT-301/302/304 real field/row/grid/
  persistence/Fields-menu scenarios pass. No codec/SQL/mock acceptance proxy.
  Exact commands, iterations/resources/cleanup and limitations are in
  [T1.13 record](task-prompts/T1.13.md#implementation-and-handoff-2026-10-26);
  [T1.14 prompt](task-prompts/T1.14.md) carries Field API/UI and T1.15 handoff.

## T1.14 field implementation and handoff (2026-10-26)

- D-123..D-130: table-owner field create/rename/description/options/required/
  unique/default/reorder/soft-delete and display-field routes. Field `If-Match`,
  512 KiB field bodies, `meta.tables` row lock, typed DDL, exact NULL/duplicate
  preflights with optional fill, select label rewrites by choice id,
  `pg_timezone_names` fixed-zone checks, and one field-version/schemaVersion
  bump per commit. No migration or dependency.
- A lazy FieldPanel on the real `/t/:tableId/v/:viewId` schema page has an
  owner-only header menu and editor dialog plus a read-only list for others.
  It serializes guarded requests and replaces or reloads the authoritative
  schema. No rows, grid or synthetic data. Initial gzip **188707 bytes**
  (<600000); FieldPanel is a 14.15 KB gzip lazy chunk.
- Final **394 frontend tests / 21 files**, **487 matching keys**. Required
  uncached PG18 field tests pass. Command-scoped `GOFLAGS=-p=1` full check:
  lint **20.389 s**, test **2m8.869 s**, build **15.431 s**, total
  **2m44.689 s**, all PASS. Default full check (twice) fails only Suite DROP
  cleanup in codec/db/testdb.
- Final production image `pt-e2e-qowqqq2peog2vkjqjydlq3ax2n`: **11 passed /
  2 intentional exclusions (3.3m)**. It includes real owner field flows
  (AT-202 Chinese rename, conflicts, select rewrite, display, reorder, delete)
  and browser reader denials (API writer/reader/stranger/admin denials in PG18). Two earlier runs failed only the existing desktop
  nav drag-order step. Desktop post-auth RSS **232652800** bytes,
  mobile **185143296**: NFR-1 **not certified**.
- T1.14 stays **In progress** until full AT-301/303 rows/grid scenarios pass
  (T1.15/T1.16). [T1.14 record](task-prompts/T1.14.md#implementation-and-handoff-2026-10-26);
  [T1.15 prompt](task-prompts/T1.15.md).

### Cleanup and next task

Zero `pt_test_*` databases/sessions remain; the E2E project, volumes, networks
and image were removed; `pt-dev-db`/`pt-dev-pgdata` retained (container stopped
again as before the task). No remote deployment. Limitations: lifecycle/
disable/membership services must call `Resolver.Invalidate()` when they arrive;
UI parts of AT-204..208 are T1.10-T1.12; T1.2 browser AT-102/116/full AT-117
and Linux/Docker AT-001/005/006 remain pending. Details:
[T1.4 record](task-prompts/T1.4.md#implementation-and-handoff-2026-10-26).

Next: **[T1.5 Admin users](task-prompts/T1.5.md)**.

## T1.5 admin users verification and handoff (2026-10-26)

**T1.5 Done; AT-104..AT-108 pass.** Started and completed 2026-10-26 after
user confirmation of the D-081 list/identity/password policy, D-082
last-active-admin/self-management and transaction protocol, and D-083 scoped
login/forced-change/admin UI. No migration or dependency additions.

### Delivered

- `internal/admin`: sanitized users DTOs, literal search/status/paging,
  normalized identities with legacy collision/ambiguity rejection, create,
  PATCH and forced password reset. Static SQL uses a third pinned-sqlc entry.
- User mutations serialize with `pandatable.admin.users`, lock/recheck the
  acting admin/session, then lock the target. Role/status changes and resets
  revoke all target sessions atomically; last-active-admin changes fail 409.
  Self-demotion/disable is allowed when another admin remains; self-reset is
  allowed. Role/status writes invalidate ACL after commit/uncertain commit.
  Last workspace owner disable is allowed; data/authorship remain.
- Admin-only HTTP namespace, shared auth worker/identity validators, strict
  request/CSRF/no-store/error contracts. SPA sign-in, minimal home, forced
  password change/re-login, admin-only menu, responsive localized user
  create/edit/role/status/reset/search forms, explicit errors and route guards.
  Existing setup completion, preferences, CSP and opt-in grid are preserved.

### Exact verification

- Versions: Go 1.27.0 windows/amd64, PostgreSQL 18.6, Podman client/server
  5.8.2/5.8.4, Node 24.15.0, pnpm 11.2.2, sqlc 1.31.1,
  Playwright 1.64.1 (Chromium). Docker/golangci-lint unavailable; cgo off.
- `go test ./internal/admin ./internal/auth ./internal/httpapi -count=3`,
  explicit `PT_TEST_DATABASE_URL` and `PT_TEST_DATABASE_REQUIRED=1`: **PASS**
  (final 8.096 s / 14.072 s / 18.190 s). Includes failed-session-delete
  rollback, actual PG lock evidence, concurrent self-demotions, concurrent
  duplicate-email updates, stale caller reauthorization and old-password login
  queued behind reset (cannot resurrect a session). Disable/enable invalidation
  is tested against a primed resolver cache, without waiting for TTL.
- `go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate`: **PASS**;
  SHA-256 before/after all three generated packages is byte-identical.
- `pnpm --dir web lint` and `pnpm --dir web test`: **PASS**, **143** matching
  keys and **27 tests / 8 files**. Guard tests include forced access to setup
  and spike, non-admin deep routes and explicit connection failures.
- Final `go run ./tools/ptdev check`: **PASS**, lint **19.224 s**,
  test **31.358 s**, build **13.518 s**, total **64.101 s**.
- Final `go run ./tools/ptdev e2e`: **PASS**, **11 passed / 2 intentional
  mobile exclusions in 57.1 s**; desktop/mobile admin scenarios
  **10.3 s / 9.9 s**. Setup and all existing shell/spike checks pass.
  Production health **6.381 s**. Shell gzip **165,751 bytes** (<600,000, no
  grid imports), all assets **294,311 bytes**. Startup app snapshot
  **74.35 MB / 536.9 MB, 13.19% CPU** (not load-test/AT-801 evidence).
- Separate owned production-image probe: **AT-004 preserved**,
  linux/amd64, **162,975,200 bytes** (<200 MB), UID/GID 10001:10001,
  executable healthcheck and `pg_dump` **18.6**. Probe tag/container removed.

### Failures, cleanup and handoff

Initial lint caught Vue formatting and an untranslated role/status separator;
fixed through existing ESLint/Prettier and a shared en/zh-CN formatting key.
The first hash-check command used a native Git wildcard that PowerShell did not
expand; corrected to enumerated `Get-FileHash` inputs. Initial E2E reached the
working password-change success UI but expected `status` instead of Naive UI's
actual `alert`; corrected the locator, then subsequent full runs passed. The first
image probe looked for Docker's Config.Healthcheck, but Podman exposes the
image's top-level HealthCheck; corrected the probe and verified actual metadata
and runtime client. No production assertion, CSP or rate limit was weakened.

All Suite-owned databases/sessions were removed (catalog counts both zero).
All four E2E projects and owned probe images/containers were removed, with
their private networks/volumes. `pt-dev-db` restored to stopped and
`pt-dev-pgdata` retained. Release images/shared networks/Caddy untouched.
Ignored generated assets and browser artifacts are not committed. No remote
deployment or hosted CI. Linux/Docker/server AT-001/005/006, T1.2
AT-102/116/full AT-117 and T1.4 UI-dependent acceptance remain pending.

Full commands, implementation details and limitations:
[T1.5 completion record](task-prompts/T1.5.md#implementation-and-handoff-2026-10-26).
Next: **[T1.6 Admin groups](task-prompts/T1.6.md)**, carrying the mandatory
Invalidate-after-membership-change and completion/next-prompt obligations.

## T1.6 admin groups verification and handoff (2026-10-26)

**T1.6 Done; production-image desktop/mobile AT-109 passes.** User confirmed
the D-084..D-086 list, name/description, membership/disabled-user, deletion and
detail-UI contracts before editing. D-087 records the lock refinement proven
by real concurrency tests. No migration, dependency, shared/remote deployment
or hosted CI was added.

### Delivered

- Admin-only group list/search/create/detail/PATCH/delete, member list/add/remove,
  user detail and user-group lists. Static SQL stays in the existing admin sqlc
  entry. Names retain case-sensitive uniqueness; bounded Unicode fields,
  literal search, stable paging/counts, typed duplicate/disabled/missing errors.
- Group deletion DELETEs principal first, cascades memberships and removes all
  polymorphic grants atomically. Group mutation actor lock is FK-compatible
  `FOR NO KEY UPDATE`; distinct member target is `FOR UPDATE` and group is
  `FOR KEY SHARE`. Existing user-admin locks are unchanged. All mutations
  reauthorize after actor lock; memberships/deletion invalidate ACL after commit
  and commit errors. Real grant/delete races exposed and fixed a deadlock.
- Responsive localized groups list/detail and linked user detail, group edits,
  confirmation/cancel deletion, active-user search/paging picker, member removal,
  error/retry and empty states. Abort/generation guards preserve auth and prevent
  stale reads. Non-admin navigation/API/deep-route denial remains independent.

### Exact verification

Environment: Go **1.27.0 windows/amd64**, cgo=0; PG **18.6**
(`18.6-1.pgdg13+2`); Podman **5.8.2/5.8.4**; Node **24.15.0**,
pnpm **11.2.2**, pinned sqlc **1.31.1**, Playwright **1.64.1** Chromium.
Docker/golangci-lint unavailable; no race-detector claim.

- Focused group/HTTP PG18 tests and real-grant/deferred-commit cases pass x3.
  `go test ./internal/admin ./internal/auth ./internal/acl ./internal/httpapi
  -count=3`: **PASS**, **20.096 / 20.286 / 20.217 / 27.989 s**. Explicit
  `PT_TEST_DATABASE_URL`, required=1, fresh Suite-owned migrated databases only.
- Pinned generator version query failed on network; cached-source invocation
  succeeds: `go -C C:\Users\huipa\go\pkg\mod\github.com\sqlc-dev\sqlc@v1.31.1
  run ./cmd/sqlc -f C:\other\pandatable\sqlc.yaml generate`. SHA-256 before/after
  all auth/acl/admin generated files is byte-identical. No version/proxy change.
- `pnpm --dir web lint`, `pnpm --dir web test`, `pnpm --dir web build`: **PASS**,
  **169** matching non-empty locale keys, **27 tests / 8 files**. Shell gzip
  **166,667 bytes**, all assets **299,014 bytes**, no initial grid imports.
- Default `go run ./tools/ptdev check` attempted twice: lint/build pass, existing
  `TestSuiteIsolationPostgreSQL` fails during cleanup under concurrent packages.
  `go test ./internal/testdb -run TestSuiteIsolationPostgreSQL -count=3 -v`
  passes (**3.606 s**). Full check with command-scoped `GOFLAGS=-p=1` passes:
  lint **14.815 s**, test **67.344 s**, build **10.882 s**, total **93.041 s**.
  No tests skipped, cached, softened or removed; shared testdb code unchanged.
  Default-parallel cleanup sensitivity is a retained limitation, not a claimed
  default-gate pass. Exact PowerShell commands are in the T1.6 record.
- Focused `go run ./tools/ptdev e2e -- admin-users.spec.ts`: **3 passed / 44.1 s**
  including setup and desktop/mobile AT-104..109. Final `go run ./tools/ptdev
  e2e`: **11 passed / 2 intentional mobile exclusions (1.0m)**, admin scenarios
  **14.0 / 14.2 s**. Real setup/admin API seeding only, unchanged ten-login
  budget. Both Bob/Carol user details, group detail, create/add/remove/edit/delete,
  bilingual duplicate error, empty state and cancel paths pass. Exact expected
  diagnostic is method/path/status-scoped and must occur; CSP/other errors fail.
- Final project `pt-e2e-hl6wlqupim5s43o25i6myqb4f3`: health **6.206 s**;
  one-shot app **73.41 MB / 536.9 MB, 13.21% CPU** (not load certification).
  Saved group/user screenshots visually inspected for desktop/mobile wrapping
  and overlap. Editor diagnostics and `git diff --check` pass.

### Failures, cleanup and handoff

Initial SQL generation could not reach the module proxy; an offline version
query also failed, so the exact cached 1.31.1 module was run directly. New tests
initially guessed ACL API names; corrected to NewResource/Access. Vue lint
caught picker punctuation and whitespace-sensitive tag layout; fixed with
translated labels and existing multiline markup, without rule changes. First
image build caught invalid multiline template assignments (typecheck missed
it); a named handler and local production build fixed it before successful E2E.
The deterministic grant/delete test reproduced an actor/granted_by FK deadlock;
D-087's narrower locks and both grant/deletion orders now pass repeatedly.
Two default-parallel full checks hit the existing shared testdb five-second
context/cleanup timing; isolated x3 and the full serial-package gate pass.

All Suite databases and sessions verified **zero**. Both successful E2E projects
removed their containers, private networks/volumes and unique image tags; failed
pre-image run had no tag to remove. Retained `pt-dev-pgdata`, release images,
unrelated containers/anonymous volumes/networks and Caddy were not removed.
`pt-dev-db` and `podman-machine-default` restored to their original **stopped**
state. Generated web assets and ignored screenshots/test artifacts are excluded
from the commit. No remote or shared deployment.

Linux/Docker/server AT-001/005/006, T1.2 AT-102/116/full AT-117 and T1.4
UI-dependent acceptance remain pending. Next: **[T1.7 Admin dashboard](task-prompts/T1.7.md)**.

## T1.7 admin dashboard verification and handoff (2026-10-26)

**T1.7 Done; production-image desktop/mobile AT-110 passes.** Started and
finished on 2026-10-26. Clarification was requested before editing; user was
unavailable, so bounded pragmatic defaults are recorded as implementation choices
in D-088..D-090, not falsely described as user-confirmed.

### Delivered

- Admin GET `/api/v1/admin/system` and responsive localized `/admin/dashboard`,
  preserving existing Admin/users/Groups routes, auth classes, CSRF/no-store,
  independent service authorization and resource isolation.
- Host-visible CPU/load/RAM, app RSS, Linux cgroup v2 memory current/visible
  effective limit, configured-volume disk, current-database PG size/connections/
  shared buffers, logical files/backups sizes, actual versions/migration/uptime.
  Explicit unavailable/reason/null and unlimited; no fake initial CPU or missing
  directory zero, raw error, host path, entry name or secret in DTOs/logs.
- One on-demand four-second collector, five-second snapshot cache, hourly
  timestamped directory success/error caches; cancelable waiters without
  canceling other clients. Rooted scans, batches 128, 100,000-entry/depth-64/
  one-second bounds, symlink/permission/special-file policy; no idle workers.
- Fixed ten-second visible-only refresh, no overlaps, abort/unmount/visibility
  cleanup, late-result guard, loading and stale/error/retry. Both locales have
  units/timestamps; grid/preferences/CSP preserved.
- Pinned BSD-3-Clause gopsutil **4.25.9**, seven indirect platform helpers.
  Existing x/sys **0.49.0** is now direct for Linux test FIFO creation, not a
  version upgrade. Existing admin sqlc entry plus generator-only catalog;
  no migration or resource/membership/grant writes.

### Verification

- Required PG18 focused system tests x3: **admin 4.574 s, HTTP 4.581 s**.
  Full restart/admin/auth/ACL/HTTP regressions x3: **25.023 / 23.521 / 23.553 /
  23.551 / 33.766 s**. Isolated shared testdb cleanup x3: **4.332 s**.
  No developer DB schema/data changes; fresh migrated Suite databases only.
- Pinned sqlc **1.31.1** cached-source regeneration byte-identical across all
  auth/ACL/admin bindings; `go mod verify` and whitespace checks pass.
- Non-root Linux tests under **2 CPUs / 512 MiB** pass, including actual
  permission failure, symlink root/entry, FIFO rejection, CPU/cache/cancellation/
  cgroup cases. Host collection **228,277 ns/op, 45,937 B/op, 338 allocs/op**;
  1001-file directory scan **3,582,176 ns/op, 545,454 B/op, 7086 allocs/op**.
  Windows observations: **138,283 ns/op / 1241 B/op / 19 allocs** host;
  **160,007,489 ns/op / 592,134 B/op / 11133 allocs** directory.
- Frontend lint/types/i18n pass, **31 tests / 10 files**, **226** matching
  non-empty keys. Initial shell gzip **168,887 bytes**, all assets **304,046**,
  no initial grid imports, <600,000 gate preserved.
- Default `ptdev check` attempted twice. First also found/fixed the restart
  fixture's missing DataDir; both hit existing testdb cleanup deadlines under
  concurrent package load. Second also reported DB-package Suite cleanup.
  Default gate is **not** claimed green. No shared harness/assertion/timeout
  edits. Final full check with command-scoped `GOFLAGS=-p=1`: **PASS**,
  lint **16.390 s**, tests **73.610 s**, build **13.029 s**, total **103.029 s**.
- Focused production image: **3 passed (1.2m)**; final full image:
  **11 passed / 2 intentional mobile-grid exclusions (1.4m)**, desktop/mobile
  admin scenarios **27.1 / 25.1 s**. AT-110 refresh observed directly in DOM
  **9904 / 9855.5 ms**, genuine 4096/2048-byte fixtures and unchanged directory
  timestamps; both locales/loading/stale/retry/nav/API/deep-route denial pass.
  Existing sign-ins stay within ten/minute/IP, no bypass or relaxed diagnostics.
- Final project `pt-e2e-g5x6setmpdnc6hapc2ewxc2mli`: health **6.575 s**;
  startup **74.32 MB / 536.9 MB, 6.49% CPU**. Image **163,282,400 bytes**,
  amd64 UID/GID 10001, exec HEALTHCHECK; retained release **161,964,427 bytes**
  (delta **1,317,973**, not attributed solely to the new dependency).
  Dashboard screenshot pairs visually inspected, no clipping/overflow.

### Failures, resources, cleanup and handoff

Initial module-proxy download timed out; explicit command-scoped goproxy.cn
retry succeeded without changing persistent Go/proxy settings. Tests caught a
misplaced new test declaration, a loading-test render race, unused test args and
template formatting; corrected before final gates. Linux test-binary copy first
lacked executable mode, then unquoted PowerShell dotted flags were misparsed;
fixed with explicit chmod in the owned container and quoted flags. All three
uniquely named helper containers were removed.

First E2E's additional experimental RSS-budget assertion failed at ~162 MiB
after auth hashing. That is **not a metrics error or AT-110 criterion**; the
assertion was removed, not silently weakened, while genuine RSS stays shown
and recorded. Final observations **170,131,456..175,009,792 bytes** exceed the
150 MB NFR-1 target. No GC forcing, hash-cost reduction, rate-limit change or
target-host/load certification. The Linux microbenchmark is a capped laptop-VM
observation, not a 2-vCPU/4-GB server load test. WSL actually exposed ~15.47 GiB
despite the Podman machine's displayed 2-GiB config.

Second E2E's coarse locator polling missed the near-ten-second render despite
trace-confirmed fresh responses. A MutationObserver now measures the exact
rendered update under the unchanged <=10,000-ms bound. Combined admin scenario
timeout is explicitly 45 s for the added real poll; suite bound/retries/CSP/error
diagnostics are unchanged. One standalone Vitest i18n-negative test timed out
during simultaneous VM startup/build load; subsequent full serial gates pass,
without changing its five-second assertion/test timeout.

All Suite databases/connections verified **zero**. All four E2E projects
(two failures/two passes) removed only their containers, private volumes/networks
and unique tags. Retained dev DB/volume, release tags, unrelated buildkit container,
anonymous volumes, networks and Caddy untouched. Dev DB and Podman VM restored
to original **stopped** state; only the owned Linux scratch binary deleted.
Generated assets/screenshots/observations/traces stay ignored, not committed.

Local-storage syscalls remain cooperatively, not forcibly, cancelable;
cgroup v1/Windows load unsupported. Linux/Docker/server AT-001/005/006,
T1.2 AT-102/116/full AT-117 and T1.4 UI-dependent acceptance stay pending.
No remote/shared deployment or hosted CI.

Exact commands and complete record:
[T1.7 completion](task-prompts/T1.7.md#implementation-and-handoff-2026-10-26).
Next: **[T1.8 settings and workspaces list](task-prompts/T1.8.md)**.

## T1.8 settings and workspace inventory (2026-10-26)

**In progress**, with the available implementation delivered. Full AT-111
requires AT-322 uploads (T2.2); full AT-112 requires real workspace creation,
navigation and table GET (T1.10/T1.11). Clarification, including AT-112's
UI-verification method, was requested; user unavailable. D-091..D-093 record
bounded implementation choices, not user-confirmed contracts.

### Delivered

- Admin settings GET/atomic partial PATCH, seven strict bounded/catalog-
  validated values, sanitized DTOs, unknown/null/empty rejection, setup marker
  preservation. Existing admin advisory lock/actor/session recheck serializes
  writes; no settings cache, ACL mutation or filesystem walks.
- Existing NULL-timezone auth/profile readers and new-user locale default
  wired. Existing explicit profile/setup/local choices stay unchanged.
  Currency-field/upload/retention consumers remain T2.1/T2.2/T3.4/T3.6.
- Public two-field appName/defaultLocale endpoint, plain-text header/title
  for all authentication classes after reload, immediate current-tab admin
  save, explicit translated load failure/retry. Team locale replaces only
  browser fallback; explicit local storage/click choices win async races.
- Live read-only paged/searched workspace names, first 50 direct owners plus
  total, disabled status, group ownership and active-direct-user orphan rule.
  One repeatable-read snapshot, recorded attachment bytes including retained
  Trash descendants, not disk/PG size. No grants, resource links, row data,
  ownership mutation, implicit sidebar access or production seed API.
- Lazy responsive en/zh-CN settings/inventory pages; independent admin API/
  service/deep-route guards, loading/empty/error/retry, request cancellation.
  Existing users/groups/dashboard/auth/ACL/CSRF/CSP/grid/preferences preserved.
  No new dependency or migration; pinned static admin SQL/catalog only.

### Verification

Versions: Go **1.27.0**, Node **24.15.0**, pnpm **11.2.2**, sqlc **1.31.1**,
PG **18.6** (dev Debian13/image Debian12 packages), Podman **5.8.2/5.8.4**,
Playwright **1.64.1** Chromium. Docker/golangci-lint absent. Commands and
exact version/package suffixes: [completion record](task-prompts/T1.8.md#implementation-and-handoff-2026-10-26).

- Required uncached Suite-owned PG18 settings/inventory/HTTP x3:
  admin **5.145 s**, HTTP **3.877 s**; extra actual-owner HTTP versus admin
  inventory/ACL regression x3 **3.442 s**. Covers catalogs/types/bounds/
  corrupt storage/full rollback/concurrent writers/session recheck/default
  readers, paging/owner cap/groups/disabled/orphan/Trash/size and independent
  auth classes. Developer DB schema/data untouched.
- Full restart/admin/auth/ACL/HTTP x3:
  **24.177 / 23.641 / 20.970 / 20.859 / 33.067 s**.
  Auth/ACL/admin pinned regeneration byte-identical.
- Frontend **40 tests / 13 files**, lint/types/parity, **261 keys** pass.
  Initial gzip **172,077 bytes**, all assets **311,436**, no initial grid.
- Default check first caught/fixed a newly nested test declaration; second
  reproduced existing `TestSuiteIsolationPostgreSQL` owned-DB cleanup deadline
  sensitivity under package concurrency. **Not** a default pass. No shared
  timeout/assertion/diagnostic changes. Initial command-scoped
  `GOFLAGS=-p=1` full gate **91.612 s**; final stable-source gate **95.856 s**
  (lint **18.259**, test **64.440**, build **13.157**) all pass.
- Focused production-image setup/admin desktop/mobile **3 passed (1.2m)**,
  admin **30.7/30.2 s**, real settings persistence/header reload/defaults/
  explicit preferences, both locales, loading/error/retry and real empty
  inventory/non-admin denial. Image **163,347,937 bytes**, non-root10001,
  exec HEALTHCHECK. Health **6.899 s**, startup **74.4 MB / 536.9 MB**;
  observed live **159.4 MB** and dashboard RSS **169,869,312 bytes**. NFR-1
  still not certified, no GC/hash weakening.
- Visual inspection caught mobile two-column settings and user-worded list
  labels after focused image success. Corrected CSS/locale keys; added exact
  one-mobile/two-desktop column assertion. First full image predated correction
  while Playwright loaded new assertion: **10 passed / 1 failed / 2 deliberate
  mobile-grid exclusions (1.6m)**, expected1/observed2. It demonstrates the
  old-image defect, not a final-source pass. Final image evidence follows.

Final stable-source production image **11 passed / 2 intentional mobile-grid
exclusions (1.8m)**, combined admin desktop/mobile **31.7 / 35.5 s**, setup
**5.9 s**. Exact one-mobile/two-desktop settings columns pass. Final English
desktop/Chinese mobile settings and Chinese mobile inventory screenshots
visually inspected; no clipping/overflow. Real persistence/reload/default/local
choices, independent guards, read-only empty inventory and retry pass.
Full AT-111/112 remain partial, not inferred from mocks or SQL fixtures.

Final project **pt-e2e-32yjkpq6jjolfkf7knhctu6mt5**: health **6.087 s**,
startup **74.12 MB / 536.9 MB, 6.18% CPU**; later **151.3 MB, 3.81% CPU**.
Image **163,347,937 bytes**, linux/amd64, UID/GID10001:10001, exec HEALTHCHECK.
Dashboard refresh **9883.4 / 9832 ms** and genuine 4096/2048-byte fixtures
preserved. Actual RSS **169,082,880 / 173,219,840 bytes**, above NFR-1 target.
VM reports **16,613,830,656 bytes** total, not target 4 GB; container limit
**536,870,912 bytes**. No normal-load/platform/server certification.

### Failures, cleanup and next task

Tests also caught/fixed a fixture field name, unrepresentable PostgreSQL JSON
NUL fixture, stale consumed mock Response, matchMedia mock lifetime and
informational-alert role. Formatting/type/lint issues fixed without rule
changes. The default-parallel shared cleanup failure and old-image mobile
failure are recorded separately above, not disguised as final green results.
An initial read-only cleanup query assumed unset POSTGRES_USER/POSTGRES_DB;
psql failed as root. Reran with repository helper's explicit postgres role,
no data changed. Final Suite databases/connections **0 / 0**.

All three E2E projects independently verified **zero private containers,
volumes, networks and unique tags**. Retained dev DB/volume, release images,
unrelated buildkit/anonymous volumes/networks/Caddy preserved. Dev DB and
Podman VM restored to original **stopped** state; dev volume still exists.
No scratch binary or broad cleanup. Generated assets/test artifacts remain
ignored, not committed. No remote/shared deployment or hosted CI.

Exact implementation/commands/failures/resources/cleanup and session log names:
[T1.8 completion](task-prompts/T1.8.md#implementation-and-handoff-2026-10-26).
Next: **[T1.9 DDL module](task-prompts/T1.9.md)**, retaining future
AT-111/112/117/322 dependencies and mandatory completion/next-prompt handoff.

## T1.9 physical DDL implementation and handoff (2026-10-26)

**In progress**, implementation verified to the available scope. AT-202 is a
real table/field rename API scenario (T1.10/T1.14); full AT-804 also requires
filter APIs (T2.6). Catalog/metadata rename and injection tests are lower-layer
evidence, not complete API/browser acceptance. Clarification was unavailable;
D-094..D-096 record bounded implementation choices, not user confirmations.

### Delivered

- `internal/ddl`: crypto/rand names with exact kind prefix + ten lowercase
  RFC 4648 a-z2-7 symbols/50 independent bits; strict metadata validation.
  Caller-owned bounded whole-transaction collision retries; no silent rename.
- pgx.Tx-only CreateSchema/CreateTable/AddColumn/DropColumn/SetNotNull/
  AddUnique/SetDefault. Each validated operation sets LOCAL lock_timeout=3s;
  no independent transactions, pool API, IF EXISTS/IF NOT EXISTS or CASCADE.
  Fixed identity/system columns; no authorship FKs/triggers or row APIs.
- Closed native-type descriptors and safe E-string/cast literals, type-matched
  now/current_date/uuidv7, no raw SQL type/default surface. Bounded dimensions;
  varchar literals reject overlength rather than silently truncate.
  Explicit required/unique/default removal preserves data; NULL-distinct UNIQUE.
- Sanitized typed validation/conflict/lock errors with bilingual keys; unexpected
  failures retain the standard logging/redaction boundary. Services still own
  ACL/locks, metadata/system fields/default Grid/Name, versions/events and commits.
  No migration, dependency, auth/admin/ACL/settings or production-route changes.

### Exact verification and limitations

Go 1.27.0, Node 24.15.0, pnpm 11.2.2, sqlc 1.31.1, PG18.6
(dev `18.6-1.pgdg13+2`, image `18.6-1.pgdg12+2`),
Podman client/server 5.8.2/5.8.4, Playwright 1.64.1/Chromium.
golangci-lint remains absent (explicit existing-tool warning); no claim that
its optional checks or a race-detector/platform certification ran.

- First focused PG18 run **failed, 4.071 s**: PG18 represents NOT NULL in
  pg_constraint (the initial PK-only count was wrong), and duplicate UNIQUE
  creation returns index-name `42P07`, not only `42710`. Corrected catalog
  assertions and explicit conflict mapping, without suppressing errors.
- Focused required uncached PG18 x3 then **10.295 s / 99.2%**; final extended
  entropy/default/no-CASCADE/UUID-version evidence x3 **10.240 s / 100.0%**
  statement coverage. Tests observe blocking PIDs before the real three-second
  lock timeout and prove metadata/version/DDL rollback plus same-session LOCAL
  reset. Types/defaults/catalog/identity/authorship/constraint removal,
  metadata-only Chinese/payload rename and both string-conformance injection
  modes pass. All DB fixtures use the package's Suite-owned disposable DB.
- All **nine** auth/ACL/admin generated Go files are byte-identical after pinned
  sqlc regeneration. Applied migrations, go.mod/go.sum and sqlc.yaml unchanged.
  New Go files are LF/gofmt; patch whitespace passes.
- Default-parallel `ptdev check`: lint **14.507 s PASS**, tests **16.499 s
  FAIL**, build **13.566 s PASS**, total **44.572 s**. DDL passes **7.096 s**;
  only the already documented TestSuiteIsolationPostgreSQL owned-DB cleanup
  deadline sensitivity fails. No shared timeouts/assertions/diagnostics changed.
- Command-scoped `GOFLAGS=-p=1` complete gate **92.705 s PASS**:
  lint **15.939 s**, test **65.474 s**, build **11.291 s**. Required uncached
  PG18 auth/admin/ACL/HTTP/restart/Suite and **40 frontend tests / 13 files**
  pass. Default-parallel cleanup is not certified. **263** locale keys;
  initial gzip **172,214 bytes** (<600,000), all assets **311,597**, no initial
  grid imports. No unrelated frontend behavior changes.
- Real production-image project **pt-e2e-35ksiy7napeb6ucg2vyeiofpij**:
  **11 passed / 2 intentional mobile-grid exclusions (1.6m)**. Setup **6.2 s**,
  combined admin desktop/mobile **31.2 / 29.8 s**. Existing CSP/browser
  diagnostics, auth/login budget, settings exact columns/default preferences,
  inventory/groups/dashboard/shell/spike tests unchanged.
  Image **163,347,936 bytes**, linux/amd64, UID/GID10001:10001, exec
  HEALTHCHECK; actual loopback health verified, ready **6.826 s**.
- Startup **73.95 MB / 536.9 MB, 6.02% CPU**, separate live observation
  **167.1 MB, 4.66% CPU**. Dashboard genuine 4096/2048-byte fixtures and
  visible refresh **9876.1 / 9846.7 ms** pass. Actual post-auth RSS
  **168,820,736 / 178,110,464 bytes** exceeds 150 MB; no forced GC/hash
  weakening or NFR-1 claim. VM-visible RAM **16,613,830,656 bytes**, container
  limit **536,870,912 bytes**, not the target 2-vCPU/4-GB host.

### Cleanup and next task

Initial worktree clean; VM and retained dev DB stopped. Initial read-only Podman
inventory failed while the socket was stopped; started the existing VM, then
inventoried it before starting the retained dev DB. No shared/developer schema/
data was changed. After tests, Suite databases/connections **0 / 0**, including
after the failed default gate. Owned E2E containers/volumes/networks/unique tag
verified absent. Retained dev DB/volume, release images and unrelated stopped
buildkit/container/storage/networks/Caddy preserved; no prune/reset.
Retained dev DB and VM restored **stopped**, dev volume confirmed present.
No scratch binary or committed generated assets/browser artifacts.

Full AT-111/112/117/322 and earlier platform/load/UI-dependent acceptance stay
pending with the exact dependencies in the next prompt. No remote/shared
deployment or hosted CI. Exact commands/results/failures/cleanup and local
log names: [T1.9 completion](task-prompts/T1.9.md#implementation-and-handoff-2026-10-26).
Next: **[T1.10 Workspace/Book/Table API](task-prompts/T1.10.md)**, including
mandatory completion record and following T1.11 prompt.

## T1.10 structure API implementation and handoff (2026-10-26)

**In progress.** Real HTTP/PG18 API evidence for AT-201/203/213 and the table-rename
half of AT-202; AT-112's admin half (empty nav, 404s, inventory owner) passes at
the API. Sidebar/browser scenarios (T1.11), field rename (T1.14), uploads
(AT-111/322), datetime/filter (AT-117) and filter injection (AT-804) are pending.
Delivered/verified: `internal/structure`, 11 routes, D-097..D-103, pinned sqlc
`structuresql` (other generated files byte-identical), no migration/dependency.
Focused PG18: 27 structure tests 86.0% statements plus HTTP `TestStructureAPI*`.
Default-parallel check fails only the existing Suite cleanup deadline in
`internal/db`/`internal/testdb` (lint 16.5 s, test 27.3 s FAIL, build 16.3 s);
command-scoped `GOFLAGS=-p=1` check **PASS** (2m15.5 s); image E2E **11 passed /
2 intentional exclusions**, startup 74.2 MB. NFR-1 remains uncertified. Exact
commands, failures, limitations and cleanup:
[T1.10 completion record](task-prompts/T1.10.md#implementation-and-handoff-2026-10-26).
Next: **[T1.11 Sidebar & navigation UI](task-prompts/T1.11.md)**.

## T1.11 sidebar/navigation implementation and handoff (2026-10-26)

**Done for the linked M1 scope**: AT-201/208/213 and full AT-112 now pass through
real UI/accounts/APIs on production-image desktop/mobile Chromium. M1 AT-203
cancel/confirmed deletion removes the table from the tree/schema immediately;
M3 Trash restoration is explicitly unimplemented. This also closes T1.10's
deferred M1 browser boundary; it does not claim field rename, uploads or filters.

- Delivered D-104..D-107: typed nav/schema DTOs, memory-only coalescing/
  invalidating nav store with ten-second deadlines and identity/stale-response
  guards; collapsible parent-owner controls and path-only contexts; native
  Unicode-name/Trash dialogs; same-parent drag and keyboard/touch move buttons;
  <768 px modal drawer/focus restoration; real guarded workspace/book/table
  routes, breadcrumbs/empty/error states and lazy schema-only table shell.
  Admin inventory remains independent; no backend, migration or dependency change.
- Required focused PG18 structure/httpapi **9.926 / 13.744 s PASS**, uncached,
  explicit URL/required=1 and Suite-owned DBs. Final full scoped-serial gate:
  lint **20.193 s**, test **1m34.177 s**, build **16.175 s**, total
  **2m10.546 s PASS**; all **52 frontend tests / 16 files**, **304** matching
  locale keys, initial gzip **178,390 bytes**, all assets **320,070 bytes**,
  no initial grid chunks. ESLint/Prettier/strict TS/go vet pass.
- Default-parallel full check: lint **21.520 s PASS**, test **26.577 s FAIL**,
  build **16.261 s PASS**, total **1m4.358 s**. Only the existing owned-DB
  cleanup sensitivity in `internal/db` and `internal/testdb` fails. No PG
  assertions/timeouts changed; default-parallel is not certified.
- Final unique production project **pt-e2e-olyqy6ymvbo3b7ptpbgwwi6z34**:
  **11 passed / 2 intentional mobile-grid exclusions (2.1m)**, combined admin/
  navigation desktop **47.3 s**, mobile **40.2 s**; setup **6.7 s**.
  Real Alice UI hierarchy/default Grid/Name, Chinese renames, both locales/
  light/dark, button/drag reorder, Bob read-only and 403, Carol table-only
  three-node tree/path-only controls and owner workspace creation, admin empty
  sidebar/404 vs populated owner inventory, deletion/cancel and concurrent 404
  all pass. Console/CSP diagnostics clean. No browser SQL or mocked nav/schema.
- Five image attempts are recorded honestly: initial setup-status selector
  collision (fixed by keeping setup sidebar-free); premature URL fixture reads
  (await actual routes and clean owned workspaces in finally); a stale-removal
  negative helper incorrectly waiting for dialog success; then the first full
  run's two obsolete unauthenticated table-route expectations (now test login
  guard plus a genuinely unknown route). Final full run passes unchanged
  acceptance assertions. D-107 changes only expanded harness bounds to
  75/180 s; request/dashboard/assertion/PG deadlines and login budget stay fixed.
- Image **163,585,503 bytes**, linux/amd64, UID/GID10001:10001. Health ready
  **6.903 s**; startup **74.01 MB / 536.9 MB, 6.47% CPU**; separate live
  observation **90.09 MB, 8.54% CPU**. Actual post-auth dashboard RSS
  **168,689,664 / 172,576,768 bytes** still exceeds 150 MB; **NFR-1 not
  certified**. Refresh **9834.1 / 9837.9 ms**, container limit 536,870,912 bytes,
  VM-visible RAM 16,613,822,464 bytes (not the target host). No forced GC/hash
  weakening, normal-load/Linux-Docker/physical-device/server certification.
- Final Suite DBs/connections **0 / 0**. All five owned E2E projects' containers,
  volumes/networks and unique image tags are absent. Retained `pt-dev-pgdata`,
  `pt-dev-db`, original latest image id `151798547521`, unrelated stopped
  buildkit/anonymous storage/networks and Caddy preserved. Dev DB and VM restored
  **stopped**. No broad prune/reset, remote deployment or hosted CI.

Go1.27.0, Node24.15.0, pnpm11.2.2, PG18.6, Podman5.8.2/5.8.4,
Vue3.5.44/Pinia3.0.4/Router4.6.4/NaiveUI2.45.3/vue-i18n11.4.16,
Vite7.3.7/TS5.9.3/Vitest4.1.12, Playwright1.64.1/Chromium156.0.8078.4.
golangci-lint remains absent (explicit warning); no optional-linter/race claim.
Exact commands/failures/log names:
[T1.11 completion record](task-prompts/T1.11.md#implementation-and-handoff-2026-10-26).
Next: **[T1.12 Share dialog UI](task-prompts/T1.12.md)**. Full AT-111/322 uploads,
AT-117 datetime/filter, AT-202 field rename and AT-804 filters remain pending.

## T1.12 sharing implementation and handoff (2026-10-26)

**M1 sharing implementation verified; task remains In progress** because full
linked AT-204..207 still requires real rows/editors/shared views from later
tasks. Full M1 AT-208..211 and the real sharing portions of AT-204..207 pass;
no permission-probe, mocked picker or SQL fixture substitutes for those missing
row/editor scenarios.

- D-108..D-112: effective target-owner Share on sidebar/resource pages, native
  responsive direct/inherited dialog, explicit review and localized failures,
  bounded owner-authorized real directory (minimal identities, first 20,
  explicit 2..100-code-point/254-byte search, no admin enumeration), cancellation/
  identity/race guards and authoritative nav/schema refresh. No migration or
  dependency; pinned sqlc generation is byte-identical across all 12 bindings.
- Final full production project **pt-e2e-24b43zcuwjbe3jlogmxisv224d**:
  **11 passed / 2 intentional mobile-grid exclusions (3.3m)**, setup **6.4 s**,
  desktop/mobile expanded scenarios **1.3m / 1.3m**. Real users/groups/scopes,
  read/write/owner/inherited levels, direct removal, last-owner rejections,
  self-access loss/restoration, principal missing/disabled races, membership
  revocation <5 s, independent admin isolation and prior real navigation all
  pass. Both locales/light-dark, mobile drawer stacking/Escape/focus/touch and
  viewport bounds pass; retained-tab browser/CSP diagnostics clean.
- **66 frontend tests / 18 files**, **348 locale keys**, strict frontend/E2E TS,
  ESLint/Prettier and Go build/vet pass. Required focused uncached PG18 ACL/HTTP
  **5.001 / 12.807 s**; serial full gate's same packages **4.683 / 14.641 s**.
  Full scoped-serial check **PASS**, lint **22.153 s**, test **2m2.255 s**,
  build **13.934 s**, total **2m38.343 s**. No PG deadline/assertion change.
- Default-parallel full check **FAIL** only on existing `internal/db` and
  `internal/testdb` Suite cleanup/drop sensitivity: lint **18.436 s PASS**,
  test **25.198 s FAIL**, build **14.990 s PASS**, total **58.624 s**.
  Reported separately, not certified. Initial browser attempt found native
  select labels included option text; translated aria-labels fixed real lookup/
  accessibility, not auth or acceptance thresholds. Its timed-out disposable
  branding cleanup also broke mobile; corrected desktop **2 passed** and final
  full run pass. Exact failures/commands are in the completion record.
- Initial gzip **184,252 bytes** (<600,000), all assets **325,999 bytes**,
  no initial grid chunks; synthetic opt-in grid unchanged. Image **163,622,369
  bytes**, linux/amd64, UID/GID10001:10001, executable HEALTHCHECK and pg_dump18.6.
  Health ready **6.648 s**; startup **74.39 MB / 536.9 MB, 6.42% CPU**;
  independent live sample **24.72 MB, 7.38% CPU**. Actual post-auth dashboard
  RSS **168,689,664 / 181,317,632 bytes**, exceeding 150 MB: **NFR-1 not
  certified**. Dashboard refresh **9884.6 / 9884.9 ms**, genuine 4096/2048-byte
  fixtures; VM RAM **16,613,826,560 bytes**, 16 visible CPUs, container limit
  **536,870,912 bytes**, not the target host. No forced GC/hash weakening.
- All three owned projects' containers/volumes/networks/tags absent; final Suite
  DBs/connections **0 / 0**. Retained pt-dev-db/pt-dev-pgdata, latest image
  `151798547521`, unrelated stopped buildkit/anonymous storage/networks/Caddy
  preserved. Dev DB and VM restored **stopped**. No broad prune/reset,
  remote/shared deployment, hosted CI or committed browser/generated assets.

Go1.27.0/Node24.15.0/pnpm11.2.2/PG18.6/Podman5.8.2-5.8.4,
Vue3.5.44/Pinia3.0.4/Router4.6.4/NaiveUI2.45.3/vue-i18n11.4.16,
Vite7.3.7/TS5.9.3/Vitest4.1.12/Playwright1.64.1/Chromium156.0.8078.4.
golangci-lint absent (explicit warning); no optional-linter/race or physical
device/virtual-keyboard/Linux-Docker/target-host certification.
Full AT-111/322, AT-117, AT-202 field rename, AT-804 filters and M3 restore remain
pending. Exact record: [T1.12](task-prompts/T1.12.md#implementation-and-handoff-2026-10-26).
Next: **[T1.13 Codec framework + batch 1](task-prompts/T1.13.md)**, including its
mandatory completion record and following T1.14 prompt.

## T1.17 copy/paste and fill-down verification (2026-10-27)

T1.17 production-grid acceptance passes for AT-306..309. Copy/cut/paste and
fill-down use the real `/t/:tableId/v/:viewId` grid, bound codec `formatCopy`
and `parsePaste`, and existing bounded row APIs; no API, dependency, migration,
or schema change was needed. D-139 records the clipboard and bounded
RevoGrid-index policy. Valid cells from the 3x4 TSV are submitted in one
bulk PATCH while invalid integer/email cells stay unpersisted and are reported.
The browser scenario covers quoted multiline/Chinese text, a 10-row tail paste
with explicit confirmation and nine inserted rows, date/select copy formatting,
Ctrl+D across six rows and vertical fill-handle extension. Both English/Chinese
and light/dark modes are exercised. The grid remains absent on unsupported
mobile; its localized fallback notice is now visible at the mobile breakpoint.
The synthetic spike remains separate.

- `pnpm -C web test`: **406 passed / 24 files**.
  `pnpm -C web lint`: **PASS**, including ESLint, Prettier, Vue typecheck and
  518 matching non-empty en/zh-CN keys. `pnpm -C web build`: **PASS**; initial
  shell gzip **189,948 bytes / 600,000-byte limit**, all assets 357,267 bytes.
- Required uncached PG18 row/query integration:
  `$env:PT_TEST_DATABASE_URL = (& go run .\tools\ptdev db status | Select-Object -Last 1); $env:PT_TEST_DATABASE_REQUIRED = '1'; go test .\internal\structure .\internal\httpapi -run 'Rows|Query' -count=1`
  **PASS** (both packages).
- `go run .\tools\ptdev check` default parallel: **FAIL** after lint/build
  passed. It reproduced Suite cleanup/drop failures in codec/db/ddl/testdb and
  `TestServePostgreSQLRestarts` observing one remaining connection; no test
  deadline/assertion was changed. Command-scoped
  `$env:GOFLAGS = '-p=1'; go run .\tools\ptdev check`: **PASS** (lint
  26.324 s, Go/frontend tests 2m21.305 s, build 15.637 s; 3m3.267 s total).
  `golangci-lint` is not installed and emitted the existing optional-linter
  warning.
- Production-image browser scenario
  `go run .\tools\ptdev e2e -- e2e/grid.spec.ts --project=desktop-chromium`:
  **2 passed** (setup + production grid, 25.5 s); grid scenario itself 15.4 s.
  Actual clipboard paste request assertion: one PATCH with 3 rows/10 valid
  supplied cells; invalid cells had no persisted values. Mobile counterpart
  `go run .\tools\ptdev e2e -- e2e/grid.spec.ts --project=mobile-chromium`:
  **2 passed** (setup + unsupported-mobile fallback, 17.0 s). Production
  app one-shot snapshots were 73.39 MB / 536.9 MB (6.40% CPU) desktop and
  73.93 MB / 536.9 MB (6.35% CPU) mobile; final full-suite snapshot was
  74.7 MB / 536.9 MB (6.77% CPU). These are not load/NFR results.
- Final `go run .\tools\ptdev e2e`: **8 passed / 5 failed / 2 skipped in
  2.6 min**. Both production-grid desktop and mobile scenarios pass (10.5 s /
  2.6 s); two mobile spike-only scenarios are intentionally skipped. Remaining
  failures are admin/navigation diagnostics (including one unexpected 404),
  mobile spike, and two mobile shell scenarios. No budgets/retries/assertions
  were relaxed. The earlier stale-selection failure in the test was resolved
  by waiting for RevoGrid's selected range before clipboard actions.
- AT-312 was not measured: the available Podman VM is **8 CPUs / 2 GiB**, not
  the documented 2-vCPU/4-GB target. No 200k-row production-browser first-paint
  or middle-page timing is claimed. NFR-1, full AT-301/303/304 and
  AT-204..207 remain pending; AT-302's invalid integer/email UI/no-persistence
  scenario is now covered by the real grid browser run.
- Versions: Go 1.27.0, Node 24.15.0, pnpm 11.2.2, PostgreSQL 18.6,
  Podman 5.8.2/5.8.4, Vitest 4.1.12, Vite 7.3.7, Playwright 1.64.1,
  Chromium 156.0.8078.4; no dependency added. Each E2E invocation used
  a unique `pt-e2e-*` Compose project and its owned resources were cleaned by
  `ptdev`. Runtime restoration and final branch/worktree verification are
  recorded below after cleanup.

T1.17's full AT-306..309 scenarios pass; unrelated full-suite failures,
default-parallel Suite cleanup sensitivity, and the unmeasured AT-312 target
remain explicitly outside that claim. See
[T1.17 implementation and handoff](task-prompts/T1.17.md#implementation-and-handoff-2026-10-27).
Next: [T1.18 row expand](task-prompts/T1.18.md).

Cleanup restored `pt-dev-db` and `podman-machine-default` to stopped (VM
configuration remains 8 CPUs / 2 GiB). All T1.17-owned E2E projects were
removed by `ptdev`; pre-existing `pt-e2e-x42ioxb5porjfzr6u36ao2bfvw` exited
containers, its volumes/networks, `buildx_buildkit_default`, `pt-dev-pgdata`,
release images and Caddy were preserved. `git branch --list` contains only
`master`; the Conventional Commit is recorded in the handoff after final
repository verification.

## T1.18 row expansion verification (2026-10-27)

T1.18's linked **AT-310 passes** in the complete production-image browser
run. The lazy full-row native dialog uses the existing GET/single PATCH
routes and FieldEditor/FieldRenderer, with exact codec wire values, all live
fields (including hidden and read-only system fields), read/write access
controls, neutral resource errors, ten-second requests and generation/identity/
schema/route guards. Space and labeled expand controls work on the real grid;
row-only URL changes retain the grid/FieldPanel, and direct reload opens the
requested row. D-140 records per-field save behavior and the unchanged
4096-byte single-PATCH limit, including explicit UTF-8 oversize rejection.
No backend API, migration, row version or dependency was added.

- Focused initial row/grid/page/editor tests: **56 pass / 4 files**;
  final row-dialog suite **33 pass**, including exact 4096/4097-byte
  boundaries, all kinds, removed labels, hidden/system values, invalid input,
  failure/no-success state, read-only/auth outcomes and stale read/write results.
- Final `pnpm -C web test` through the full gate: **441 pass / 25 files**.
  `pnpm -C web lint`: **PASS**, ESLint/Prettier, strict frontend/E2E
  typecheck and **528** matching non-empty en/zh-CN keys.
  `pnpm -C web build`: **PASS**, conservative initial gzip **190337 bytes**
  (<600000); all assets **361491 bytes**. Grid/dialog/editors stay lazy.
- Required uncached explicit-DSN PG18
  `go test .\internal\structure .\internal\httpapi -run 'Rows|Query' -count=1`
  with `PT_TEST_DATABASE_REQUIRED=1`: **PASS**, **3.154 s / 3.099 s**.
- Default `go run .\tools\ptdev check`: lint/build pass; Go tests **FAIL**
  with the existing parallel Suite DROP cleanup failures in **codec/db/testdb**
  (`TestSuiteIsolationPostgreSQL`). No deadline/assertion changed. This run
  did not reproduce the prior restart remaining-connection error.
  Final command-scoped
  `$env:GOFLAGS = '-p=1'; go run .\tools\ptdev check`: **PASS**,
  lint **22.626 s**, tests **2m11.961 s**, build **17.098 s**, total
  **2m51.685 s**. `golangci-lint` remains absent (existing warning).
- Complete `go run .\tools\ptdev e2e`: **11 passed / 4 failed / 2 skipped
  in 4.0 min**, one worker/zero retries, unchanged D-112 budgets and
  five-second assertions. **AT-310 desktop 43.8 s** covers real grid create,
  icon/Space, all 12 editor saves and exact API values, direct reload, hidden/
  system/deleted fields, invalid/deleted row/view/table, reader/writer/revoked
  access, late route/identity responses, English/Chinese and light/dark.
  Its mobile fallback passes **2.5 s**, with no RevoGrid/cards mounted.
  T1.17 real grid desktop/mobile scenarios also pass (**15.2 s / 2.9 s**).
- The four full-suite failures are outside T1.18: mobile admin workspace
  empty-search expectation (`settings.ts:164`), mobile synthetic-spike English
  notice, mobile shell English welcome, and mobile shell expecting `lang=en`
  but receiving `zh-CN`. Two spike mobile exclusions remain intentional.
  Desktop admin/navigation now passes; no unrelated fix or acceptance promotion.
- Final production app startup snapshot: **74.6 MB / 536.9 MB, 5.48% CPU**.
  Earlier focused startup snapshots were **73.95 MB / 536.9 MB (5.74%)**,
  **74.81 MB (5.64%)**, **74.66 MB (5.60%)**, **74.65 MB (6.98%)**.
  These are not load/NFR results. VM remains **8 CPUs / 2 GiB**;
  **AT-312/NFR-1 are not certified** and no 200k production-browser measurement
  on the 2-vCPU/4-GB target is claimed.

Intermediate browser-selector, pinned-index, delayed-route and expected-status
test failures were corrected without weakening assertions. Invalid GET row
IDs intentionally return neutral 404; the real server was not changed.
One attempt failed because the VM became stopped/unreachable before browser
startup; it was restarted without resetting storage. Exact versions, commands,
limitations and final cleanup are in
[T1.18 record](task-prompts/T1.18.md#implementation-and-handoff-2026-10-27).
Full AT-301/303/304, AT-204..207, uploads, restore and filter acceptance
remain pending. Prior evidence is preserved. Next:
[T1.19 mobile layout](task-prompts/T1.19.md).

Final owned-only cleanup was verified: no new E2E containers remain;
`pt-dev-pgdata`, baseline exited stack/image, release images, Caddy and
unrelated volumes/stacks are preserved. The dev DB and Podman VM are both
restored to **stopped**, with the original **8 CPUs / 2048 MiB** VM settings.
The worktree began clean; only `master` exists. The T1.18 Conventional Commit
is made directly on `master`, without generated assets or browser artifacts.
## M1 phase review and gate repair (2026-10-27)

Scope: review the whole M1 phase, verify whether it is complete, and fix what
blocks the gates. No feature, API, schema, dependency or acceptance claim was
added. Both gates were run for real before and after the change.

### Verified M1 status

- Tasks: **11 Done** (T1.1, T1.3-T1.7, T1.10, T1.11, T1.17-T1.19), **9 In
  progress** (T1.2, T1.8, T1.9, T1.12-T1.16, T1.20). Status unchanged by this
  review; no task was promoted.
- Gates **before** the fix: `go run .\tools\ptdev check` **FAIL**
  (lint PASS, test FAIL, build PASS) and `go run .\tools\ptdev e2e`
  **13 passed / 4 failed / 2 intentional skips**.
- Gates **after** the fix: `ptdev check` **PASS** with default parallelism
  (lint 27.6 s, test 1m18.8 s, build 14.8 s) and `ptdev e2e`
  **17 passed / 0 failed / 2 intentional skips in 5.0m**. No `GOFLAGS=-p=1`.

### Defects found and fixed

- **Default-parallel Suite cleanup (D-145).** Reproduced with an explicit DSN
  and `PT_TEST_DATABASE_REQUIRED=1`: `internal/codec`, `internal/devseed` and
  `internal/testdb` failed with `drop owned database failed`. The diagnostic
  carried no SQLSTATE, so it was the shared 5 s connect+drop deadline expiring,
  not a server refusal. Teardown now has its own 60 s budget and retries the
  forced drop only on `55006`; the isolation test verifies forced removal with
  a fresh deadline instead of an already spent shared one. All assertions,
  redaction and package-failing behavior are unchanged.
- **Four mobile E2E failures, one root cause (D-146).** `verifySettings`
  asserted a globally empty admin workspace inventory, which only holds for the
  first project; the desktop project legitimately leaves real workspaces. The
  mobile run therefore failed at `settings.ts` and aborted before the caller's
  `finally` restore, leaving instance-wide `defaultLocale=zh-CN` and cascading
  into the mobile spike notice, shell welcome and `lang=en` assertions. The
  helper now restores settings itself on failure and asserts the live API
  inventory, reaching the empty state through the escaped `%_` search.

### Remaining M1 work (not started here)

- **AT-311** column ops are unimplemented: there is no `PATCH /views/{id}`
  route, and the grid only has RevoGrid local resize plus a session-local field
  toggle; reorder, freeze and persistence are missing (T1.16).
- **AT-305** keyboard navigation relies on RevoGrid defaults, with no
  Delete/Backspace clear, Esc/F2, Ctrl+Home/End coverage or E2E (T1.16).
- **AT-116 / AT-117 (M1 half)**: no `/profile` page or route, so display name,
  locale, theme and timezone cannot be changed in the browser although
  `PATCH /auth/me` exists (T1.2).
- **AT-102** has no browser scenario for the translated wrong-password error or
  post-logout back-button behavior (T1.2).
- **AT-301/302/303/304** still lack the complete grid scenarios (12 types with
  `information_schema` verification, validation, required/unique/default,
  system-field display and `updated_by`).
- Deferred by design: AT-111 upload enforcement (T2.2), full AT-804 (T2.6),
  AT-203 restore (T3.6), AT-117 relative-date filters (M2). T1.8 and T1.9 stay
  In progress because their linked tests close only in M2.
- Environment-blocked and still uncertified: AT-312/NFR-2 (no authorized
  2-vCPU/4-GB Ubuntu host), AT-001/005/006 (no Linux/Docker or approved Caddy
  destination) and NFR-1 app RSS.

Owned cleanup: the temporary gate logs were removed and the E2E workflow
removed its own stack; `pt-dev-db`, `pt-dev-pgdata`, release images and
unrelated stacks are preserved. The worktree began clean; only `master` exists.
