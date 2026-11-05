# UX-11 - Integrate and certify the workspace/grid redesign

Requires every UX-01..UX-10 implementation handoff (transitively UX-10).
Run alone; all peer writers and gate runners must be stopped.

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- [Execution index](workspace-grid-redesign.md) and UX-01..UX-10 Records.
- Relevant changed sections of `docs/05-data-model.md`, `docs/09-api.md`,
  `docs/12-frontend.md`, and `docs/user-guide.md`.
- Search decisions D-131, D-158, D-209..D-217; affected acceptance IDs listed
  in implementation prompts. Do not read historical task records/archive.

## Scope

Integrate this batch, validate its exact user-visible outcomes, and update docs.
This is not a new feature task or a full audit of unrelated milestone problems.

1. Verify all implementations and handoffs exist. Inspect shared-worktree
   changes without resetting anything; reconcile missing locale additions,
   shared API mocks, global styles and test selectors/helpers.
2. Update affected browser tests that used the removed options panel or landing
   copy. Keep the same assertions through the new entry points; no skip/timeout
   inflation or assertion removal to make gates green.
3. Record new default/icon technical details and scoped supersession of old
   frontend rules in decisions; update API/data/frontend docs and the bilingual
   user guide. D-217 records the requested direction, not implementation proof.
4. Assign unused AT IDs to the numbered new scenarios, map them to concrete
   executable tests and preserve existing acceptance history. New passes must
   include the default launch, all-workspace sidebar, stable icons/card grid,
   header/actions relocation, view/table deletion, both row-create paths and
   actual single-click/blur editing with reload persistence.
5. Update one progress row and one finish log line per task only after its
   scenarios and task gates pass. Keep detailed counts in each task's Record.
   Failed/unrun tasks remain open with exact blockers.

## Gate sequence

Follow `AGENTS.md` engine/mirror preflight and write long output to named logs.
Do not run concurrent check/E2E/build commands against the shared environment.

1. Focused frontend tests for changed navigation, header, ResourcePage, grid
   editors, RowExpandDialog and cards; targeted required-PG18 tests for default
   provisioning, book icons, ACL, view lifecycle and migrations.
2. `go run .\tools\ptdev check`.
3. `go run .\tools\ptdev e2e --fast --` with the affected specs. At minimum include
   the new default-workspace, workbook-cards, grid-inline-editing, grid-append-row,
   new-entry specs and existing shell/table-workspace/views/visibility-menu,
   row-expand/mobile-rows, grid/grid-columns-keyboard, import/export, trash,
   grouping/undo/realtime coverage. Discover other affected specs from the diff;
   run in bounded batches if necessary without omitting failures.
4. Browser inspection at desktop and 390 px, en/zh-CN, light/dark: confirm layout,
   final-row plus spacing, rightmost New Entry, absence of removed text/button,
   menu accessibility and no unexpected console errors.
5. Once at this phase end:
   `go run .\tools\ptdev e2e` (complete production-image gate, D-158).
   Its failure reopens affected tasks. Keep unrelated pre-existing failures
   distinct; never call the whole batch Done while its scenarios are unverified.

Respect the two-targeted-fix retry budget. After exhaustion record the evidence
and change approach or report the blocker, not repeated long reruns.

## Final deliverable and commit

- No unrelated features/dependencies, no secret/log artifacts committed.
- Record commands and pass/fail counts plus any unresolved risks; no environment
  inventory. Documentation-only planning did not imply implementation acceptance.
- Commit the coordinated changes on `master` using Conventional Commits and
  UX task IDs, with the required Co-authored-by trailer. Stage explicit paths;
  never include unrelated user changes.
- Report the completed task IDs, gate results and any remaining blockers.

## Record

Not started.
