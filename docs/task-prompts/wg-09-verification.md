# WG-09 VERIFY - integrate and verify the whole refinement

Read [execution plan](workspace-grid-refinement.md). Requires WG-01..WG-08
implementation/locale handoffs and exclusive access to shared gate resources.
This is the **only full-verification session** in this batch.

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- Each WG prompt's Scope, numbered cases and Record; do not read older task
  prompts/archive unless a specific failure requires their evidence.
- `docs/12-frontend.md` touched navigation, toolbar and grid sections;
  `docs/08-query-engine.md` sections 5, 6; `docs/09-api.md` changed contracts.
- `docs/14-dev-build-release.md` test/gate/environment sections.
- Decisions: D-131, D-156..158, D-219, D-222..230 and new WG decisions.
- Acceptance: AT-214..217, AT-301..311, AT-313, AT-327..331, AT-504,
  AT-506..510, AT-513, AT-701..705; new IDs assigned below.

## Integration scope

- Review all eight handoffs for completeness and conflicts, not just their
  pass summaries. Integrate outstanding locale/caller/docs patches; clean up
  WG-08 compatibility props and obsolete callers/tests/styles when safe.
- Trace all numbered WG cases to executable tests. Assign unused AT IDs for
  new obligations; revise current AT-215/327/330/510 criteria for the requested
  behavior without deleting old evidence or presenting old tests as new proof.
- Update frontend navigation sketch and narrative, query/API contracts and
  bilingual user-guide workflows. Add decisions for actual non-trivial choices
  and supersession notes only where appropriate. Preserve prior UX-11 failures
  unless canonical new evidence resolves them.
- Repair regressions caused by this batch and tightly coupled prerequisites.
  Do not add new features or broadly fix unrelated historical acceptance.

## Required verification order

1. Stop other writers/builds. Preflight running Podman VM and known gate
   resources, persisted mirror variables (`APT_MIRROR`, `PGDG_MIRROR`,
   `NPM_REGISTRY`, `GOPROXY`) and external build leftovers. Do not stop/delete
   resources belonging to other sessions. Resolve environment faults before
   starting gates; log long commands to specific session log files.
2. Run focused remaining/reproduction tests first, then
   `go run .\tools\ptdev check` once for the integrated fix batch. This covers
   required PG18 Go tests, frontend unit/lint/typecheck/i18n and bundle checks.
3. Consolidated affected fast task gate:
   `go run .\tools\ptdev e2e --fast -- e2e/workspace-navigation.spec.ts e2e/workbook-cards.spec.ts e2e/views.spec.ts e2e/grid-append-row.spec.ts e2e/search-aggregates.spec.ts e2e/grid-columns-keyboard.spec.ts e2e/table-header.spec.ts e2e/bulk-row-actions.spec.ts e2e/import.spec.ts e2e/export.spec.ts e2e/new-entry.spec.ts e2e/grid-inline-editing.spec.ts e2e/mobile-rows.spec.ts e2e/undo.spec.ts e2e/visibility-menu.spec.ts`.
   Add any other actually touched specs; use bounded batches only if required
   by the existing harness, with no duplicate scenarios.
4. Use browser tooling on the real app to verify geometry, not only DOM:
   flat workspace landing/card grid; active-book-only tree; full-width append
   hit target; footer alignment across resize/freeze/horizontal scroll;
   Format, selection bar and single transfer trigger. Check normal desktop,
   wide/long-name content and 390px mobile, en/zh-CN, light/dark, writer/reader.
   Mobile may retain the existing card layout; do not require desktop grid
   footer or desktop row-selection features where that layout is absent.
5. Run **one** canonical complete production-image phase gate:
   `go run .\tools\ptdev e2e`.
   No timeout increase, retry/worker changes, softened assertions, broad
   expected-error allowances, disabled tests or partial-suite substitutions.
   Do not tag or claim target-host performance/deployment certification.
6. If the phase gate fails, retain the failure evidence, reopen affected tasks
   and use at most two targeted fixes/reproductions per same failure. Do not
   automatically start another full production build. Report the exact
   remaining blocker/next step; a focused pass is not a phase-gate pass.

## Final report / persistence

- Each WG task can be marked Done only when its focused cases and consolidated
  task gates pass; a phase failure reopens the affected tasks per D-158.
- Update progress with one row and concise log lines per task, acceptance
  evidence/status and the task Records (about 1-2 KB each). No giant gate logs
  in docs; keep detailed logs in session storage.
- Report pass/fail/intentional-skip counts and unresolved pre-existing failures,
  especially AT-305/311 and AT-705. Never imply earlier failed phases passed.
- No commit unless requested; if requested, use Conventional Commits on master,
  explicit scoped staging after inspecting peer changes, the required trailer,
  and a green normal pre-commit check. Do not tag or create branches.

## Record

### 2026-11-07 — task gates pass; phase gate failed and awaits one rerun

- Integrated the WG-01..08 handoffs, removed the obsolete toolbar compatibility
  prop/helper, mapped numbered cases to AT-519..543, documented D-233 and
  updated the frontend, query/API and bilingual workflow documentation.
- Corrected card and append geometry, view-deletion focus, cross-page keyboard
  focus and footer alignment. RevoGrid can update its column viewport after its
  synthetic scroll event and independently of its virtual scrollbar; the footer
  now follows the viewport's native scroll event and rebinds across resizing.
- A non-reactive pending-write counter left selected-row actions disabled after
  saving; it is now reactive, with successful-save and rejected-draft coverage.
  The AT-608 browser reproduction passed 2 tests. A transient missing
  bounding box after paged append led to polling the same 2px geometry bounds;
  its focused browser reproduction passed 2.
- Final `ptdev check` passed lint, required PG18 Go tests, 65 frontend files /
  896 tests, type/i18n and build; `golangci-lint` was unavailable and warned.
  An earlier check failed formatting and one PG cleanup; formatting was fixed,
  the isolated doctor test passed, and subsequent full checks passed.
- The non-overlapping fast gate passed: navigation 8/1 skipped, grid 13/8,
  aggregates/transfer 8/3, entry/editing 13/8 and grid/admin 5/0. Across the
  five batches this is 47 passed and 20 intentional skips, including five
  required setup runs. Real-browser bounds cover card layout, append width and
  footer alignment; the toolbar and permission cases cover desktop/mobile,
  long names, both locales/themes and writer/reader access.
- An earlier grid batch had 11 passed, 8 skips and 2 failures; a later pre-final
  batch had 13 passed and 8 skips. Two footer batches each had 7 passed, 3 skips
  and 1 alignment failure; after the native-scroll fix the same batch passed
  8 tests with 3 intentional skips. These are not the final consolidated gate.
- The single production-image phase gate failed: 28 passed, 2 failed, 1
  skipped and 135 did not run. Both Fields-menu failures expected `/home`
  after login although the preferred workspace correctly resolved to `/w/...`;
  the 600-second suite and teardown limits also expired (135 scenarios never
  ran, so the phase result covers only the first 31).
- Post-phase repair: stale `/home` login assertions now expect `/w/<id>` in
  fields-menu, backup, history, lookups, profile and relations specs; the
  relations Delete no longer opens Format first. The profile trace showed its
  extra Enter re-opening an already-open single-click editor, which then
  ignored the next draft (a real data-loss path). `GridCellEditor` now re-arms
  a retained editor when its draft changes after a commit and preserves that
  draft on unmount; Escape still discards. New unit and browser regressions
  cover it. Focused fast E2E: Fields-menu 4/3 skipped, history+relations 4/1,
  profile 2/0, backup 3/0, rapid re-entry 2/0. Final `ptdev check`: 65 files /
  899 frontend tests, PG18 Go, lint, type/i18n, build pass (`golangci-lint`
  absent, warning only).
- Next step: one separately authorized `go run .\tools\ptdev e2e` phase run.
  Its 600 s global budget is now likely too small for 166 scenarios; that is a
  harness decision (D-155) for the user, not something to relax silently.
  UX-11 PageDown (AT-305/311) and AT-705 memory remain open, unrerun. WG-01..09
  stay In progress until the phase passes. Logs remain in session storage.
