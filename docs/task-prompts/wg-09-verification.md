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

Not started.
