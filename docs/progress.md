# Progress

Status legend: ⬜ Not started · 🟨 In progress · ✅ Done · ⏸ Blocked · ❌ Dropped

## Milestones

| Milestone | Status | Started | Finished | Tag | Notes |
|---|---|---|---|---|---|
| Planning | ✅ | 2026-10-24 | 2026-10-24 | — | Interview completed; docs created |
| M0 Foundation | 🟨 | 2026-10-24 | | | T0.1–T0.3 scoped checks complete; T0.4/T0.6/T0.7 implementations verified; T0.5/T0.8 done. D-005 confirmed by the isolated spike. AT-002/003/004 and AT-802 pass; full AT-001/005/006 remain pending Linux/Docker and an approved server. |
| M1 Core | 🟨 | 2026-10-24 | | v0.1.0 | T3.12's production gate failed dashboard and grid scenarios, reopening affected tasks; AT-307 also differs from the literal criterion. Target-host AT-312 remains uncertified. |
| M2 Data power | 🟨 | 2026-10-29 | | v0.2.0 | T3.12's production gate failed the AT-409 formula scenario; T2.5 is reopened. |
| M3 Collaboration & ops | 🟨 | 2026-10-31 | | — (v1.0.0 planned) | Release blocked by the failed production gate, outstanding acceptance and lack of an authorized deployment target. |
| M4 More views | 🟨 | 2026-11-02 | | v1.1.0 | T4.1 and T4.2 done; the phase-end production gate remains. |
| M5 Calendar & mail | 🟨 | 2026-11-02 | | v1.2.0 | T5.2 done; T5.1 and the phase-end production gate remain. |

## Tasks

| Task | Title | Status | Updated | Notes |
|---|---|---|---|---|
| WG-01 | Flat workspace browsing | 🟨 | 2026-11-06 | D-230; flat ordered sidebar with existing workbook cards and access states retained. Focused Vitest/ESLint pass; E2E and consolidated task gates deferred to WG-09. [Record](task-prompts/wg-01-flat-workspaces.md). |
| WG-08 | Unified import/export toolbar trigger | 🟨 | 2026-11-06 | D-230; one localized transfer icon replaces separate import/export triggers and removes toolbar metadata while preserving permissions, readiness, dismissal and feedback. Focused Vitest/ESLint, typecheck and i18n checks pass; WG-09 gates deferred. [Record](task-prompts/wg-08-transfer-toolbar.md). |
| WG-PLAN | Workspace/grid refinement execution prompts | ✅ | 2026-11-06 | D-230; nine bounded prompts, five safe waves, confined implementation tests and final-only consolidated/full gates. WG-01 implemented awaiting WG-09 gates. [Plan](task-prompts/workspace-grid-refinement.md). |
| UX-11 | Workspace/grid integration, acceptance and phase gate | 🟨 | 2026-11-06 | D-228/D-229; fast checks and visual inspection pass, but the production-image runs failed and used an extended global timeout. AT-305/311 and AT-705 remain unresolved. [Record](task-prompts/workspace-grid-11-integration.md). |
| UX-10 | Rightmost New Entry form action | ✅ | 2026-11-06 | D-227; task gates passed. The UX-11 phase gate remains open. [Record](task-prompts/workspace-grid-10-new-entry.md). |
| UX-06 | Consolidated table header and options-panel removal | 🟨 | 2026-11-06 | Reopened: the linked AT-305/311 PageDown regression failed in the UX-11 production run; focused repetitions passed, but the phase gate remains red. [Record](task-prompts/workspace-grid-06-table-header.md). |
| UX-05 | Sidebar view creation and management | ✅ | 2026-11-06 | Task gates passed; ResourcePage released to UX-06. [Record](task-prompts/workspace-grid-05-view-navigation.md). |
| UX-09 | Plus action after the final grid row | ✅ | 2026-11-05 | Coordinated task gates passed; the UX-11 phase gate remains open. [Record](task-prompts/workspace-grid-09-append-row.md). |
| UX-04 | Workspace workbook-card grid | ✅ | 2026-11-05 | Coordinated task gates passed; ResourcePage released to UX-05. [Record](task-prompts/workspace-grid-04-workbook-cards.md). |
| UX-08 | One-click checkbox and choice editing | 🟨 | 2026-11-06 | D-221; focused choice scenarios pass, but linked AT-305/311 failed in the UX-11 production regression. [Record](task-prompts/workspace-grid-08-choice-editing.md). |
| UX-03 | Workbook icon metadata and stable fallback | ✅ | 2026-11-05 | Coordinated task gate passed; card rendering follows in UX-04. [Record](task-prompts/workspace-grid-03-workbook-icons.md). |
| UX-02 | Workspace sidebar and default landing navigation | 🟨 | 2026-11-06 | Reopened: the UX-11 production mobile admin/navigation scenario failed on a paginated inventory selector; the corrected search passes focused E2E, but the phase gate remains red. [Record](task-prompts/workspace-grid-02-workspace-navigation.md). |
| UX-01 | Persistent default-workspace lifecycle | ✅ | 2026-11-05 | Task gate passed; UX-02 API handoff in [record](task-prompts/workspace-grid-01-default-workspace.md). |
| UX-07 | Single-click scalar editing and commit on leave | 🟨 | 2026-11-06 | D-228; focused scalar scenarios pass, but linked AT-305/311 failed in the UX-11 production regression. [Record](task-prompts/workspace-grid-07-inline-editing.md). |
| Dev DB startup recovery | Automatic engine readiness and free database port | ✅ | 2026-11-05 | D-216; full check, live port-conflict recovery and fast shell browser gate pass. [Record](task-prompts/dev-startup-recovery.md). |
| Field creation redesign | Searchable icon picker and type-specific settings | ✅ | 2026-11-04 | D-212; repository gate and affected fast browser gate pass. [Record](task-prompts/field-creation-redesign.md). |
| Shell redesign | Sketch-based header/sidebar/avatar menu | ✅ | 2026-11-02 | D-209 shell layout; `ptdev check` and the fast e2e gate (all 37 specs, run in batches) pass. [Record](task-prompts/shell-redesign.md). |
| Setup origin fix | Local dev setup 403 | ✅ | 2026-11-02 | D-208 canonical dev navigation and localized CSRF feedback; full check, AT-101 fast E2E and live browser verification pass. [Record](task-prompts/fix-setup-origin.md). |
| T0.1 | Repo scaffold & ptdev | ✅ | 2026-10-24 | Scoped completion approved: scaffold, stdlib ptdev, unit tests, Windows/Podman check, PowerShell + Git Bash entry points, native + Linux builds. Go 1.27.0; Podman client/server 5.8.2/5.8.4; Node 24.15.0; PostgreSQL 18.6. golangci-lint absent (warning). Full AT-006 pending Linux/Docker + T0.7 harness. |
| T0.2 | Config & CLI | ✅ | 2026-10-24 | Config flags/validation and JSON CLI routing; unit tests, ptdev check, and PowerShell/Git Bash smoke pass. Full AT-006 remains pending. |
| T0.3 | DB & migrations | ✅ | 2026-10-24 | pgx pool, transaction helper, and embedded migration complete; PostgreSQL 18 integration and repeated CLI migrations pass. AT-002 migration behavior verified; full AT-006 remains pending. |
| T0.4 | HTTP server | 🟨 | 2026-10-24 | Implementation and scoped verification complete; real embedded SPA/Compose/browser/CSP/history checks pass on Windows/Podman. Kept in progress until linked AT-001 also passes on Linux/Docker. |
| T0.5 | Web scaffold (i18n, theme toggles) | ✅ | 2026-10-24 | AT-003 passes in the real embedded production SPA; AT-802 passes lint/parity with negative fixtures. Vue/TS shell, local preferences, direct-process dev supervisor, web build/copy, ten frontend tests, real dev proxy/cleanup, ptdev check and Linux compilation verified. |
| T0.6 | Image & deploy files | 🟨 | 2026-10-24 | Implementation verified: AT-004 image contents pass (161,964,427 bytes); isolated Windows/Podman Compose/browser/restart/persistence and gzip export/load checks pass. Full AT-001/005 await Linux/Docker and an approved real Caddy destination; not Done. |
| T0.7 | Test harness & local gate | 🟨 | 2026-10-24 | Implementation and Windows/Podman verification complete: shared PG18 helper, four real shell E2E checks, failure/cancellation/partial-start cleanup and ptdev check pass. Linked AT-006 still requires Linux bash/Docker; not Done. |
| T0.8 | RevoGrid spike | ✅ | 2026-10-24 | Scoped objectives demonstrated: opt-in lazy DataGrid, synthetic 200k window with <=300 row cache, real clipboard/editor/range/fill/group/freeze/theme and emulated tablet checks; D-066..068 confirm D-005. Windows check and image-backed E2E pass; no production query/mobile acceptance claimed. |
| T1.1 | Meta migrations | ✅ | 2026-10-24 | Forward 0002 adds all 14 remaining metadata tables; 0001 unchanged. D-069..071 clarify composite FKs, identifier/name rules, polymorphic grants and history. Real PG18 catalog/default/constraint/delete/upgrade/down-up tests and AT-002 restarts pass; Windows check and 8-check image E2E pass (2 intentional mobile exclusions). T1.2 handoff recorded. |
| T1.2 | Auth service | 🟨 | 2026-11-01 | Backend and AT-103/803 evidence remain; the deferred profile UI now passes focused AT-102/116/117 browser checks. The production milestone timed out before that scenario, so this task remains open. |
| T1.3 | Setup flow | ✅ | 2026-10-25 | First-run token/status/admin/session, process-safe one-time setup and localized desktop/mobile UI. AT-101 passes in production-image Chromium; required PG18 gate and full local checks pass. D-078; no migration. |
| T1.4 | ACL resolver & grants API | ✅ | 2026-10-26 | Resolver (one SQL/lookup, flat groups, path-only visibility, LRU+epoch invalidation), grants list/PUT/DELETE with scope-owner, last-owner, hidden-404/visible-403, principal validation and workspace-lock/D-070 serialization; `require` guard. D-079/D-080, no migration. AT-204..212 pass at the API surface (209-212 fully; UI parts of 204-208 await T1.10-T1.12). Windows check and 9/2 image E2E pass. |
| T1.5 | Admin users | 🟨 | 2026-11-06 | Reopened: the UX-11 production mobile admin/navigation test failed on a paginated workspace inventory selector; the corrected search passes focused E2E, but the phase gate remains red. Earlier AT-104..108 and task-gate evidence is preserved. |
| T1.6 | Admin groups | ✅ | 2026-10-26 | Group CRUD/member API, localized group/user detail and immediate ACL invalidation; D-084..087, no migration/dependency. Required PG18 x3 and production desktop/mobile AT-109 pass; full image suite 11/2. Full check passes with command-scoped GOFLAGS=-p=1 after existing testdb cleanup deadline failures under parallel package load; exact caveat/evidence below. T1.7 handoff recorded. |
| T1.7 | Admin dashboard | 🟨 | 2026-11-01 | Reopened: T3.12's production scenario expected migration 2 but observed 5. The test now derives the latest migration; prior dashboard evidence remains in the [record](task-prompts/T1.7.md), while the milestone gate remains failed. |
| T1.8 | Admin settings & workspaces list | 🟨 | 2026-11-01 | D-091..093 settings/default readers and workspace inventory remain; T2.2 covers AT-111/322. T3.12 adds focused AT-117 browser/PG18 evidence, but the milestone profile scenario did not run and the release gate remains failed. |
| T1.9 | DDL module | 🟨 | 2026-11-01 | Physical-only caller-pgx.Tx DDL and D-094..096 remain; T3.12 now verifies full AT-804 via real table, field and filter HTTP/PG18 requests. Earlier checks and the remaining task gate are retained in the [record](task-prompts/T1.9.md). |
| T1.10 | Workspace/Book/Table API | ✅ | 2026-10-26 | API implementation unchanged; T1.11 closes real M1 browser AT-201/203/213 and full AT-112. Required uncached PG18 and scoped-serial complete gate pass; default-parallel cleanup caveat persists. Done for M1 structure scope: AT-203 restoration remains M3, AT-202 field rename T1.14 and full AT-804 T2.6, not claimed. |
| T1.11 | Sidebar & navigation UI | ✅ | 2026-10-26 | D-104..107: memory-only nav refresh/identity guards, owner-of-parent tree controls, path-only breadcrumbs, native create/rename/Trash dialogs, drag/button reorder, mobile drawer and lazy schema-only table routes. Real desktop/mobile AT-201/208/213/full AT-112 and M1 AT-203 pass. 52 unit tests/304 locale keys, required PG18 and serial full check pass; image suite 11 passed/2 intentional exclusions. Initial gzip 178,390 bytes, no grid imports. Default-parallel cleanup/RSS caveats unchanged; exact record below and T1.12 prompt created. |
| T1.12 | Share dialog UI | 🟨 | 2026-11-01 | Prior D-108..112 evidence remains. T3.12's focused desktop/mobile sharing regression now passes after test-race corrections; the production milestone gate remains failed. [Record](task-prompts/T1.12.md). |
| T1.13 | Codec framework + batch 1 | ✅ | 2026-10-28 | Done via T1.23 (full AT-301..304 pass on the production image). Earlier: implementation verified: 12 writable + five read-only system codecs/FieldKinds, typed DDL/defaults, exact wire/paste/copy, safe localized components, shared vectors and required uncached PG18. D-113..D-122; no migration/dependency/routes/grid integration. 384 frontend tests/397 keys, initial gzip 185755 bytes; serial full check passes, final image 11 passed/2 intentional exclusions. NFR-1 caveat remains. |
| T1.14 | Field API & UI | ✅ | 2026-10-28 | Done via T1.23 (full AT-301/303 pass through the real Field UI, including required-empty count/fillEmpty, unique duplicate/conflict and default on new row). Earlier: D-123..D-130 table-owner field create/rename/description/options/required/unique/default/reorder/soft-delete/display field with If-Match, typed DDL, exact preflights, select label rewrites and same-tx schema versions; lazy FieldPanel; real AT-202 field rename passes. NFR-1 caveat remains. |
| T1.15 | Basic query & rows API | ✅ | 2026-10-28 | Done via T1.23 (full AT-301..304 grid/row persistence, validation and authorship pass on the production image). Earlier: D-132..D-137 query/row routes, typed codec I/O, bounded atomic writes, ACL/schema locking, deterministic bulk-patch lock ordering, API DTOs and localized errors. AT-312 remains uncertified (no target host). |
| T1.16 | DataGrid component | 🟨 | 2026-11-01 | Reopened: T3.12's production AT-305/311 conflict scenario and grid diagnostics failed. The focused conflict scenario now passes; the milestone gate and grid diagnostic remain unresolved. Prior evidence is in the [record](task-prompts/T1.16.md). |
| T1.17 | Copy/paste & fill-down | 🟨 | 2026-11-01 | Reopened: real Excel AT-306/308 pass, but the literal AT-307 second-to-last-row paste offers eight rows rather than nine; the production grid scenario also reported an unexpected 409. See [T3.12](task-prompts/T3.12.md#record-2026-11-01). |
| T1.18 | Row expand modal | ✅ | 2026-10-27 | D-140: lazy full-row dialog, Space/icon and direct links; all 12 codec editors, exact values, read-only system/reader fields, bounded single PATCH and stale guards. Full production-image AT-310 passes (desktop 43.8 s/mobile fallback 2.5 s). 441 frontend tests, lint/528 keys, required PG18 and serial full gate pass; initial gzip 190337 bytes. Full E2E 11 pass/4 unrelated mobile fail/2 skips; default Suite cleanup and uncertified NFR/AT-312 caveats remain. T1.19 handoff recorded. |
| T1.19 | Mobile layout | ✅ | 2026-10-27 | D-141/D-142: lazy <768px cards without RevoGrid, 100-row pages/<=300 cached rows, bounded options/FieldPanel/delete, reused full-screen editors and defaults-preserving FAB creation. Full production-image AT-313 passes at 390x844 (15.6 s/17.3 s); AT-310 and AT-302/306..309 preserved. 485 frontend tests, lint/543 keys, required PG18 and serial full gate pass; initial gzip 190849 bytes. Full E2E 12 pass/5 fail/2 skips (four prior mobile failures plus desktop navigation 404 diagnostic); default Suite cleanup and uncertified AT-312/NFR-1 remain. Exact results/cleanup and T1.20 handoff in [completion record](task-prompts/T1.19.md#implementation-and-handoff-2026-10-27). |
| T1.20 | Seed & perf baseline | 🟨 | 2026-10-27 | D-143/D-144 safe CLI seed and persistent production-browser workflow implemented/tested. Final local real-200k baseline: 605.7ms first usable paint, 137.8..192.3ms middle scroll/paint, exact fresh requests/100-row pages/<=300 models; preceding strict local run failed 379.3/350.3ms and is retained. No authorized 2-vCPU/4-GB Ubuntu target; configured VM 8 CPUs/2048 MiB, engine reports a different WSL host boundary. AT-312/NFR-1 remain uncertified. 485 frontend tests, lint/543 keys, required PG18 and serial full gate pass; initial gzip 190849 bytes. Full image regression 13 pass/4 prior mobile fail/2 skips; default-parallel Suite cleanup fails separately. Exact evidence/owned cleanup in [record](task-prompts/T1.20.md#implementation-and-handoff-2026-10-27). |
| T1.21 | Grid column ops & keyboard | 🟨 | 2026-11-06 | Reopened: T3.12's stale-version conflict and UX-11's PageDown focus assertion failed in production gates; focused reruns pass, but the PageDown cause is unproven and the phase gate remains red. Prior evidence is in the [record](task-prompts/T1.21.md#implementation-and-verification-record-2026-10-27). |
| T1.22 | Self profile UI | 🟨 | 2026-11-01 | The T3.12 focused AT-102/116/117 browser scenario and PG18/check gate pass; the production milestone timed out before the profile scenario and the release gate remains failed. See [T3.12](task-prompts/T3.12.md#record-2026-11-01). |
| T1.23 | Field acceptance pass | ✅ | 2026-10-28 | Full AT-301, AT-302, AT-303 and AT-304 pass on the production image; **T1.13, T1.14 and T1.15 flip to Done** in the same change. No product-code fix was needed; harness/build changes only (D-156 `PGDG_MIRROR`, D-157 login pacing/per-page optional failures/editor opening). Required PG18 `information_schema` types/system columns/authorship pass; default-parallel `ptdev check` passes (507 frontend tests, 563 keys, initial gzip 191482 bytes); complete `ptdev e2e` **23 passed / 0 failed / 2 intentional skips**. M1 still cannot close: T1.8 (T2.2 uploads), T1.9 (T2.6 filters), T1.22 and environment-blocked AT-312/NFR-2 remain. [Record](task-prompts/T1.23.md#implementation-and-verification-record-2026-10-28). |
| T2.1 | Codecs batch 2 | ✅ | 2026-10-29 | AT-320/326 and task gates pass; batch-2 codecs, typed DDL, field UI, user picker, docs and browser coverage complete. See [T2.1 record](task-prompts/T2.1.md#record-2026-10-29-phase-c-complete). |
| T2.2 | Attachments | ✅ | 2026-10-29 | AT-111/321/322 and task gates pass. Local storage, bounded MIME/image processing, ACL serving, row metadata integrity, attachment UI, and settings are complete; D-163..D-169 record confirmed contracts. |
| T2.3 | Relations | ✅ | 2026-10-29 | AT-401..406 and task gates pass: bidirectional 1:1/1:N/M:N/self links, transactional FK/junction DDL, ACL and delete policies, relation picker/rendering; D-170 records endpoint access. [Record](task-prompts/T2.3.md#record-2026-10-29). |
| T2.4 | Lookup & rollup | 🟨 | 2026-11-06 | Reopened: the UX-11 production AT-407/408/411 scenario failed at its stale `/home` landing assertion; the corrected focused test passes, but the phase gate remains red. Earlier task-gate evidence is preserved. |
| T2.5 | Formula engine | 🟨 | 2026-11-01 | Reopened: T3.12's production AT-409 scenario failed after another test changed the shared user timezone. Test cleanup and actor-timezone assertions were corrected; the production milestone gate remains failed. Prior [record](task-prompts/T2.5.md#record-2026-10-29). |
| T2.6 | Filter engine & UI | ✅ | 2026-10-30 | Server-side filter compiler and localized nested UI pass AT-501/502; formula/rollup filter and sort evidence now completes AT-410 with T2.7. Task gates pass. [Record](task-prompts/T2.6.md#record-2026-10-30). |
| T2.7 | Sort | ✅ | 2026-10-30 | AT-503/410 and task gates pass: ordered transient sort controls, NULLS LAST deterministic paging, and formula/rollup sorting; D-175 preserves the legacy request. [Record](task-prompts/T2.7.md#record-2026-10-30). |
| T2.8 | Grouping | ✅ | 2026-10-30 | AT-504 and task gates pass: filtered three-level buckets, empty and sorted multi-select keys, grouped row paging, localized desktop/mobile headers, and persistent expansion; D-176 records view-config storage. [Record](task-prompts/T2.8.md#record-2026-10-30). |
| T2.9 | Highlight | ✅ | 2026-10-30 | D-177: ordered, filter-backed row/cell rules use first-match semantics and light/dark palette tokens. AT-505 and both task gates pass. [Record](task-prompts/T2.9.md#record-2026-10-30). |
| UI-HIGHLIGHT | Highlight rules clarity | ✅ | 2026-11-04 | Highlight menu clarifies ordered rules; persistence, precedence, contrast and task gates pass. [Record](task-prompts/ui-highlight-rules.md#record-2026-11-04). |
| UI-FIELD-TILES | Fields dropdown tiles | ✅ | 2026-11-04 | D-215: compact draggable field tiles, anchored ⋯ menu, Edit reuses the Add layout with safe type conversion, and reorder persists schema plus active-view order after flushing pending column changes. Check and fast browser gate pass. [Record](task-prompts/ui-field-tiles.md#record-2026-11-04). |
| UI-VISIBILITY | Sharing in Visibility menu | ✅ | 2026-11-04 | D-214: table Share moves into an eye-icon Visibility menu after Highlight; mobile/non-Grid keep it in the table utility area. Check and fast browser gate pass. [Record](task-prompts/ui-visibility-sharing.md#record-2026-11-04). |
| T2.10 | View management | ✅ | 2026-10-30 | D-178; AT-506–508 and task gates pass. Shared/personal lifecycle, locks, ordering, local overrides, and version conflicts are implemented; no migration. |
| T2.11 | Search & aggregates | ✅ | 2026-10-30 | D-179; AT-509/510 and task gates pass. Visible-field quick search and filtered footer aggregates are complete. [Record](task-prompts/T2.11.md#record-2026-10-30). |
| T2.12 | Type change | ✅ | 2026-10-31 | D-180; AT-323/324 and both task gates pass. Owner-only atomic conversions, dependency error propagation, choice/default/filter rewrites, and localized wizard are complete. [Record](task-prompts/T2.12.md#record-2026-10-31). |
| T2.13 | Form view | ✅ | 2026-10-31 | D-178; AT-511 and both task gates pass. Versioned form configuration, localized builder/rendering, and permission-checked atomic row submission are complete. [Record](task-prompts/T2.13.md#record-2026-10-31). |
| T2.14 | Auto-index job | ✅ | 2026-10-31 | D-020 auto-index job and metadata tracking pass PG18 tests and task gates; AT-512's 200k filter/sort/group performance criterion remains pending for T2.15. |
| T2.15 | Perf pass | 🟨 | 2026-10-31 | Local benchmark and task gates pass; AT-512 target-host acceptance remains pending. [Record](task-prompts/T2.15.md#record-2026-10-31). |
| T3.1 | Realtime hub | ✅ | 2026-10-31 | D-183; AT-601/602/603/605/107 and task gates pass. [Record](task-prompts/T3.1.md#record-2026-10-31). |
| T3.2 | Presence | ✅ | 2026-10-31 | D-184; AT-604 and task gates pass. [Record](task-prompts/T3.2.md#record-2026-10-31). |
| T3.3 | Comments & notifications | ✅ | 2026-10-31 | D-185; AT-606 and task gates pass. [Record](task-prompts/T3.3.md#record-2026-10-31). |
| T3.4 | Row history | ✅ | 2026-10-31 | D-186; AT-607 and task gates pass. [Record](task-prompts/T3.4.md#record-2026-10-31). |
| T3.5 | Undo/redo | ✅ | 2026-11-01 | D-187; AT-608/609 and both task gates pass. [Record](task-prompts/T3.5.md#record-2026-11-01). |
| T3.6 | Trash & purge | ✅ | 2026-11-01 | D-188; AT-203/325/610 and both task gates pass. [Record](task-prompts/T3.6.md#record-2026-11-01). |
| T3.7 | Export | 🟨 | 2026-11-01 | AT-701 and task gates pass; AT-702 is blocked by package fidelity. [Record](task-prompts/T3.7.md#record-2026-11-01). |
| T3.8 | Import | ⏸ | 2026-11-06 | D-190–193; earlier AT-703–705 task-gate passes remain, but the latest UX-11 production AT-705 failed its process-lifetime RSS limit before import began. AT-702 remains blocked by package fidelity. [Record](task-prompts/T3.8.md#record-2026-11-01); [UX-11 evidence](task-prompts/workspace-grid-11-integration.md#record). |
| T3.9 | Backups | 🟨 | 2026-11-01 | D-194; task gates and AT-706/708 pass; fresh-host manual AT-707 remains pending. [Record](task-prompts/T3.9.md#record-2026-11-01). |
| T3.10 | User lifecycle admin | ✅ | 2026-11-01 | D-195; AT-113–115 and both task gates pass. [Record](task-prompts/T3.10.md#record-2026-11-01). |
| T3.11 | Hardening | 🟨 | 2026-11-01 | D-196–198; load-driver regressions, AT-804 HTTP/PG18 and selected Chromium/Firefox/WebKit responsive checks pass. Target-host AT-801, full browser matrix AT-805 and complete AT-806 remain open. [Record](task-prompts/T3.11.md#record-2026-11-01). |
| T3.12 | Release v1.0 | ⏸ | 2026-11-01 | Guide, audit and cross-engine, responsive/grid/load follow-ups recorded; production gate, remaining acceptance and an authorized target block tagging and deployment. [Record](task-prompts/T3.12.md#record-2026-11-01). |
| T4.1 | Kanban view | ✅ | 2026-11-02 | D-203; AT-901 and both task gates pass. [Record](task-prompts/T4.1.md#record-2026-11-02). |
| T4.2 | Gallery view | ✅ | 2026-11-02 | D-204; AT-902 and both task gates pass. [Record](task-prompts/T4.2.md#record-2026-11-02). |
| T5.1 | Calendar view | ⬜ | | |
| T5.2 | SMTP | ✅ | 2026-11-02 | D-205..D-207; AT-904 and both task gates pass. [Record](task-prompts/T5.2.md#record-2026-11-02). |

## Acceptance test status

The counts below are historical and have not been recomputed after the T3.12
attempt; current affected verdicts are in the T3.12 acceptance evidence.

| Area | Total | Passing | Notes |
|---|---|---|---|
| AT-0xx Foundation | 6 | 3 | AT-002/003/004 pass. AT-001 Windows/Podman automated Compose/E2E passes; Linux/Docker pending. AT-005 export/manifests/Podman load pass, but Docker load/server/Caddy unverified. AT-006 shared harness, unavailable-DB skips and Windows gate pass; Linux/Docker pending. |
| AT-1xx Auth & Admin | 17 | 9 | Historical count; T3.12 reopens AT-110 after the production dashboard failure. Focused AT-102/116/117 browser and per-user relative-today PG18 checks pass; the production milestone profile scenario did not run. |
| AT-2xx Structure & Permissions | 13 | 4 | Historical count; focused desktop/mobile sharing scenarios now pass after T3.12's test-race corrections, but the production milestone gate remains failed. |
| AT-3xx Fields & Grid | 20 | 12 | Historical count; T3.12 reopens AT-305/311, verifies real Excel AT-306/308, and observes an AT-307 mismatch. AT-312 remains uncertified without the authorized 2-vCPU/4-GB host. |
| AT-4xx Relations & Computed | 11 | 6 | AT-401..406 pass; AT-407..411 remain for later M2 tasks. |
| AT-5xx Views | 12 | 0 | |
| AT-6xx Collaboration | 10 | 0 | |
| AT-7xx Import/Export/Backup | 8 | 0 | |
| AT-8xx Non-functional | 6 | 2 | Historical count; T3.12 adds HTTP/PG18 AT-804 and selected 390-pixel menu, dialog and workflow checks. AT-805/806 remain partial, and target-host NFR-1/2/3 and full-load AT-801 are uncertified. |
| AT-9xx Post-v1 | 4 | 3 | AT-901, AT-902 and AT-904 pass their task gates. |

## Open questions / follow-ups

| # | Question | Raised | Status |
|---|---|---|---|
| OQ-1 | Domain name & HTTPS: use bundled Caddy, or an existing proxy on the server? | 2026-10-24 | Resolved: existing independent Caddy container → D-046 |
| OQ-2 | Where will CI run (GitHub Actions, Gitea, local only)? | 2026-10-24 | Resolved: local builds (Podman on Windows / Docker on Linux), images transferred → D-047, D-048 |
| OQ-3 | Default timezone for the team (for "today" filters, display): Asia/Shanghai? | 2026-10-24 | Resolved: Asia/Shanghai default, per-user override → D-049 |
| OQ-4 | Currency default for currency fields (CNY?) | 2026-10-24 | Resolved: CNY default, admin-adjustable, per-field override → D-050 |

## Change log

Newest first: add new lines directly below the header row.

| Date | Change |
|---|---|
| 2026-11-06 | Started and implemented WG-08: unified transfer icon with focused behavior coverage; locale additions and export browser assertions are ready for WG-09 gates. |
| 2026-11-06 | Started and implemented WG-01 flat workspace browsing; focused Vitest/ESLint pass, with E2E and consolidated gates deferred to WG-09. |
| 2026-11-06 | Planned WG-01..09 workspace/grid refinement with source ownership, quick-task handoffs and one final full-verification session; implementation and earlier phase failures unchanged. |
| 2026-11-06 | Reopened UX-06 and kept UX-07/08 open: the UX-11 production AT-305/311 PageDown regression failed; focused repetitions pass, phase verification remains open. |
| 2026-11-06 | Reopened UX-02, T1.5 and T2.4: production navigation/landing assertions failed; corrected focused tests pass, phase verification remains open. |
| 2026-11-06 | UX-11 integrated D-228/D-229 and acceptance mappings; production gate and AT-705 remain open. |
| 2026-11-06 | Started UX-11: workspace/grid integration, acceptance and phase gate. |
| 2026-11-06 | Completed UX-10: New Entry creation form and active-view refresh; task gates passed. |
| 2026-11-06 | Started UX-10: wire the rightmost New Entry form action. |
| 2026-11-06 | Completed UX-06: consolidated the table header and removed the options panel (D-226); task gates passed. |
| 2026-11-06 | Started UX-06: consolidate the table header and remove the options panel. |
| 2026-11-06 | Completed UX-05: sidebar view creation and contextual management (D-225); task gates passed and UX-06 handoff released. |
| 2026-11-06 | Started UX-05: sidebar view creation and contextual management. |
| 2026-11-05 | Completed UX-09: native grid-tail append and bounded requery stabilization (D-222..224); coordinated task gates passed. |
| 2026-11-05 | Completed UX-04: responsive workbook cards; coordinated task gates passed and UX-05 handoff released. |
| 2026-11-05 | Started UX-09: blank-row action after the grid tail (D-222). |
| 2026-11-05 | Started UX-04: workspace workbook-card grid. |
| 2026-11-05 | Completed UX-03: workbook icon metadata and stable fallback (D-220); task gate passed. |
| 2026-11-05 | Completed UX-02: workspace sidebar and preferred-workspace entry navigation (D-219); task gate passed. |
| 2026-11-05 | Started UX-08: grid checkbox and choice interactions. |
| 2026-11-05 | Started UX-03: workbook icon metadata and stable fallback. |
| 2026-11-05 | Started UX-02: workspace sidebar and preferred-workspace entry navigation. |
| 2026-11-05 | Completed UX-01: default-workspace lifecycle and preference API (D-218); task gate passed. |
| 2026-11-05 | Started UX-01: persistent per-user default-workspace lifecycle and API. |
| 2026-11-05 | Started UX-07: grid-only scalar editor activation and commit-on-leave. |
| 2026-11-05 | Completed dev DB startup recovery (D-216); engine readiness, volume-preserving port fallback and resolved DSNs verified. [Record](task-prompts/dev-startup-recovery.md). |
| 2026-11-05 | Started dev DB startup recovery: automatic engine readiness and database port conflict handling. |
| 2026-11-04 | UI-HIGHLIGHT Done: clarify Highlight rules and verify persistence, precedence and contrast. |
| 2026-11-04 | UI-FIELD-TILES Done: Fields dropdown uses draggable tiles with a ⋯ menu and unified Add/Edit; reorder saves schema and active-view order (D-215); check and fast browser gate pass. |
| 2026-11-04 | UI-VISIBILITY Done: table sharing moves into the Visibility toolbar menu (D-214); check and fast browser gate pass. |
| 2026-11-04 | Completed field creation redesign (D-212); check and affected fast browser gate pass. |
| 2026-11-02 | Shell redesign Done: sketch-based header, workspace sidebar tree and avatar menu; e2e specs migrated and both task gates pass (D-209). | loopback navigation uses the configured origin; strict CSRF is preserved and both task gates pass (D-208). |
| 2026-11-02 | T5.2 Done: SMTP, self-service password reset and opt-in mention email pass AT-904 and both task gates. |
| 2026-11-02 | T4.2 Done: Gallery covers and selected card fields pass AT-902 and both task gates. |
| 2026-11-02 | T4.1 Done: Kanban grouping, accessible moves and realtime pass AT-901 and both task gates. |
| 2026-11-01 | T3.12 blocked: audit, guide and cross-engine/responsive/load follow-ups recorded; production gate, acceptance and authorized deployment remain open. |
| 2026-11-01 | T3.11 implemented and task gates pass; target-host load, remaining injection evidence, browser matrix and complete responsive usability remain open. |
| 2026-11-01 | T3.10 Done: ownership transfer, orphan rescue and hard-delete lifecycle pass AT-113–115 and task gates. |
| 2026-11-01 | T3.9 implemented and automated gates pass; fresh-host restore acceptance remains pending. |
| 2026-11-01 | T3.8 follow-up: stabilize the i18n check fixture; both task gates pass, while AT-702 remains blocked. |
| 2026-11-01 | T3.8 follow-up: preserve dependent formulas and views; the browser gate passes, but AT-702 and the latest check remain blocked. |
| 2026-11-01 | T3.8 follow-up: view and package-append behavior verified; missing view locks disclosed and AT-702 remains blocked. |
| 2026-11-01 | T3.8 follow-up: graceful import cleanup verified; AT-702 remains blocked by package fidelity. |
| 2026-11-01 | T3.8 implemented: plain imports pass AT-703–705; AT-702 remains partial pending the package fidelity contract. |
| 2026-11-01 | T3.8 started: import analysis, mapping, execution and package round-trip. |
| 2026-11-01 | T3.7 implemented: CSV and PandaTable package export pass task gates; AT-702 round-trip awaits T3.8. |
| 2026-11-01 | T3.6 Done: authorized Trash, reversible restoration and retryable retention purge pass AT-203/325/610 and task gates. |
| 2026-11-01 | T3.5 Done: per-tab undo/redo, guarded conflicts and exact-ID restoration pass AT-608/609 and task gates. |
| 2026-10-31 | T3.4 Done: transactional row/link history, localized timeline and retention pass AT-607 and task gates. |
| 2026-10-31 | T3.3 Done: comments, access-filtered mentions and live notifications pass AT-606 and task gates. |
| 2026-10-31 | T3.2 Done: per-table presence, localized avatars and focused-cell indicators pass AT-604 and task gates. |
| 2026-10-31 | T3.1 Done: transactional realtime delivery, reconnect/resync, access revocation and focused task gates pass. |
| 2026-10-31 | T3.1 realtime hub implementation started. |
| 2026-10-31 | T2.15 implementation and task gates pass locally; target-host performance acceptance remains pending. |
| 2026-10-31 | T2.14 Done: restart-safe shared-view auto-indexing, metadata retries, `ptdev check`, and focused search E2E pass; AT-512 performance remains pending for T2.15. |
| 2026-10-31 | T2.13 Done: AT-511, `ptdev check`, and fast Form-view E2E pass (3/3). Versioned configuration, owner/writer submission, read-only denial, and 390 px layout are verified; details in the [T2.13 record](task-prompts/T2.13.md#record-2026-10-31). |
| 2026-10-31 | T2.12 Done: D-180, AT-323/324, `ptdev check`, and focused field-acceptance E2E pass (5/5). Atomic owner-only conversion, dependency rechecks, and choice rename propagation are verified; details in the [T2.12 record](task-prompts/T2.12.md#record-2026-10-31). |
| 2026-10-30 | T2.6 Done: AT-501/502 and AT-410 pass after T2.7 adds computed-field sorting; server filter coverage and task gates were already green. |
| 2026-10-30 | T2.11 Done: D-179, AT-509/510, `ptdev check` (562 tests), and focused production-browser E2E (2 passed, 1 intentional mobile skip) pass; details in the [T2.11 record](task-prompts/T2.11.md#record-2026-10-30). |
| 2026-10-30 | T2.10 Done: D-178, AT-506–508, live-PG `ptdev check`, and focused production-browser E2E pass; shared/personal lifecycle, permissions, locking, ordering, temporary overrides, and optimistic conflicts verified. [T2.10 record](task-prompts/T2.10.md#record-2026-10-30). |
| 2026-10-30 | T2.9 Done: AT-505 and both task gates pass; ordered highlight query/compiler, production rule editor and reorder controls, independent row/cell rendering, and light/dark palette checks complete. D-177 records the bounded rule and color contract; details in the [T2.9 record](task-prompts/T2.9.md#record-2026-10-30). |
| 2026-10-30 | T2.8 Done: AT-504 and both task gates pass; filtered grouped queries, canonical group keys, aggregates, virtual desktop/mobile group rows, and collapse persistence are complete. D-176 records view-config storage; details in the [T2.8 record](task-prompts/T2.8.md#record-2026-10-30). |
| 2026-10-30 | T2.7 Done: AT-503/410 pass; transient multi-sort controls, computed-field sorting, NULLS LAST and deterministic paging pass task gates. D-175 extends the query contract compatibly; detailed evidence in the T2.7 record. |
| 2026-10-30 | T2.6 filter implementation and task gates pass; AT-501/502 pass and AT-410 filter evidence is partial pending T2.7 computed-field sorting. D-173/D-174 record the operator contracts; PG18 regression covers timezone-relative date, lookup, formula, and rollup filters. |
| 2026-10-29 | T2.5 Done: AT-409 and both task gates pass; formula parsing, query-time computation, API checks and localized editor are complete. D-172 records implementation choices. |
| 2026-10-29 | T2.4 Done: AT-407/408/411 and both task gates pass. Lookup/rollup computation, localized configuration, cross-table masking, relation-kind coverage and D-171 dependency protection are complete. |
| 2026-10-29 | T2.3 Done: AT-401..406 and both task gates pass. Relation metadata/DDL, link mutations, delete policies, field/grid UI and localized picker are covered by PG18 and real-browser tests; D-170 records ACL policy. |
| 2026-10-29 | T2.2 Done: AT-111/321/322 pass; attachment APIs, local storage/thumbnails, ACL, row integrity, UI, and admin limits pass `ptdev check` and fast browser gates. D-163..D-169 record confirmed contracts. T1.8 stays In Progress for AT-112/117. |
| 2026-10-29 | D-158 gate levels: per task `ptdev check` + `ptdev e2e --fast` (host-built binary on the shared runtime stage); complete production-image E2E once per phase. Dockerfile Go/pnpm cache mounts, compact Playwright reporter, per-task evidence moved to `docs/archive/`, task-prompt template and reading budget. |
| 2026-10-29 | T2.1 Done: AT-320/326 and task gates pass; batch-2 codecs, typed DDL, field UI/user picker, docs and browser coverage complete. D-159..D-162 record the contracts; M2 remains In Progress. |
| 2026-10-28 | T1.23 Done: full AT-301..304 production-browser scenarios plus required PG18 catalog/authorship checks pass; T1.13/T1.14/T1.15 flip to Done. D-156 configurable PGDG mirror (fixes repeated image-build stalls), D-157 E2E harness pacing/optional-failure/editor-opening rules. Default-parallel check and complete E2E 23/0/2 pass. M1 stays open (T1.8, T1.9, T1.22, AT-312/NFR-2). |
| 2026-10-26 | T1.10 implemented: workspace/book/table API (`internal/structure`), atomic metadata+DDL, system fields/Name/default Grid, reorder, soft delete, `/nav`, D-097..D-103 and sqlc `structuresql`. Required PG18 tests, serial full check and image E2E 11/2 pass; default-parallel Suite cleanup failure recorded. Sidebar UI and linked UI/field acceptance stay pending, so In progress; T1.11 prompt created. Retained Podman VM/dev DB restored stopped. |
| 2026-10-26 | T1.6 completed: group CRUD/members, linked group/user detail, D-084..087, deterministic grant/member/delete concurrency and immediate ACL invalidation. AT-109 production desktop/mobile and full 11/2 image suite pass; full check passes with GOFLAGS=-p=1 (default-parallel testdb cleanup failures recorded, not relaxed). T1.7 prompt created; original stopped DB/VM state restored. |
| 2026-10-26 | T1.4 implemented and verified: `internal/acl` resolver/grants service, `/api/v1/grants` routes, `require` guard, D-079/D-080, sqlc `aclsql`, docs and T1.5 prompt. Required PG18 tests (x3), Windows check and image E2E pass. No migration or shared/remote deployment. |
| 2026-10-25 | T1.3 started after reading the required docs and T1.2 handoff; D-078 records the confirmed setup policies. AT-101 and the required PG18/local gates now pass; exact evidence follows. No migration or shared/remote deployment. |
| 2026-10-25 | T1.2 backend implementation verified and handed off; D-072..077, static sqlc auth queries, API/types/testing docs and T1.3 prompt added. Windows check, required PG18 and final 8/2 image E2E pass. Kept T1.2 in progress because the user explicitly deferred auth/profile UI and full AT-117. No shared/remote deployment or unrelated task completion. |
| 2026-10-24 | T0.6 implementation and scoped verification: source-allowlisted image, executable healthcheck, Compose/Caddy examples, image export and opt-in remote tooling. AT-004 and Windows/Podman stack/browser/persistence checks pass; full AT-001/005/006 remain pending. D-058..D-062 and T0.7 handoff added. |
| 2026-10-24 | T0.5 complete: AT-003 embedded-browser and AT-802 local gate pass; ten frontend tests, Go checks, production build, real dev proxy/cancellation and Linux compilation verified. Added D-056/D-057 and T0.6 prompt; M0/T0.4 remain in progress pending stack acceptance. |
| 2026-10-24 | T0.5 started: inspected build/HTTP boundaries and Node/pnpm compatibility; implemented localized Vue shell and direct-process dev workflow. |
| 2026-10-24 | T0.4 implementation verified: chi HTTP foundation, safe JSON logs/errors, embedded-asset boundary, automatic migrations and bounded shutdown. Focused tests, PostgreSQL 18.6 restarts, ptdev check and Linux cross-build pass; D-053..D-055 and T0.5 handoff added. AT-001 remains pending, so T0.4 and M0 stay in progress. |
| 2026-10-24 | : implementation started. |
| 2026-10-24 | T0.3: pgx pool/retries, safe transaction helper, advisory-locked embedded Goose migration, and migrate CLI complete. PostgreSQL 18.6 integration, repeated CLI migrations, and `ptdev check` pass; `golangci-lint` unavailable. Full AT-006 remains pending. |
| 2026-10-24 | T0.3: implementation started. |
| 2026-10-24 | T0.2: config precedence/validation, CLI routing, JSON logging, and T0.3 task prompt implemented; focused tests, `ptdev check`, and PowerShell/Git Bash CLI smoke checks pass. `golangci-lint` unavailable; full AT-006 remains pending. |
| 2026-10-24 | T0.7 implementation/scoped verification complete: shared migrated package databases with bounded skips and fail-closed gate, pinned Playwright and isolated Compose runner, real success/failure/cancellation cleanup, D-063..D-065 and T0.8 handoff. Full AT-006 awaits Linux/Docker, so T0.7/M0 remain in progress. |
| 2026-10-24 | T0.7: implementation started after reading the T0.6 handoff, binding decisions and current tooling; worktree clean. |
| 2026-10-24 | T0.2: implementation started. |
| 2026-10-24 | T0.1: Go scaffold + stdlib ptdev commands and unit tests; D-051; local gate and Podman PG18 lifecycle verified; T0.2 handoff added. Existing stopped Podman machine started. |
| 2026-10-24 | Resolved OQ-1..4: external Caddy, local Podman/Docker build + image transfer, `ptdev` tool, timezone & currency defaults. Added 14-dev-build-release.md, D-046..D-050, AT-005/006/117/326. |
| 2026-10-24 | T0.8 scoped spike complete: 200k window/cache, custom editor/renderer, quoted TSV and fill, grouped sample, frozen columns, translations/themes and emulated tablet demonstrated under unchanged CSP. D-066..068 confirm D-005; 20 unit tests, Windows local gate and eight image-backed browser checks pass (two intentional mobile exclusions). T1.1 handoff created; unrelated platform acceptance remains pending. |
| 2026-10-24 | T0.8 started with a clean worktree; pinned engine/adapter APIs and isolated prototype design inspected. |
| 2026-10-24 | T1.1 Done: reversible metadata 0002, exact PG18 catalog/default/constraint/delete tests, sentinel-preserving concurrent upgrades and populated down/up, three real serve lifecycles with unchanged migration records (AT-002). Windows check, shell/spike E2E and AT-004 pass. D-069..071, data-model/testing docs, completion record and T1.2 prompt added; unrelated platform acceptance remains pending. |
| 2026-10-24 | T1.1 started with a clean worktree; required design/runtime/test docs inspected. Started the existing stopped Podman machine and retained dev DB without reset. User confirmed grant-reference and history clarifications before coding. |
| 2026-10-24 | Initial planning docs created from interview (Q1–Q51). |

Per-task verification evidence for M0–M1 lives in [archive/progress-m0-m1.md](archive/progress-m0-m1.md) and the task-prompt records. Keep new entries here to one table row plus one change-log line (D-158).
