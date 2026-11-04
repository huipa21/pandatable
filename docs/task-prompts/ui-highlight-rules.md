# Task prompt - UI-HIGHLIGHT Rename Colour and clarify multiple highlight rules

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`, this prompt.
- `docs/08-query-engine.md` section 1: highlight config, ordering, limits, and personal overrides.
- `docs/12-frontend.md`: highlight/Grid configuration sections only; locate headings first.
- Search `docs/decisions.md` for D-029, D-158, D-177, D-178.
- Search `docs/acceptance-tests.md` for AT-505.

## Scope

1. Rename the Grid toolbar's Colour/Color menu to **Highlight**, including its accessible name and directly related help/user documentation, with equivalent zh-CN copy. Retain the highlighter icon.
2. Make the menu clearly describe conditional formatting: an ordered list of independent highlight rules, each with a condition, row or selected-cell target, and palette color. There can be several rules, not one global color setting.
3. Preserve and make discoverable Add rule, edit, remove, drag reorder, and Move up/down alternatives. Show clear rule boundaries and ordering. If the current UI already meets these requirements, make only the necessary naming/help improvements instead of gratuitously rebuilding it.
4. Verify that multiple rules survive reload and render the correct row/cell highlights. Fix only defects tightly coupled to this workflow.

This is Excel-like conditional formatting using PandaTable's existing condition language and palette, not Excel feature parity. No formula language, icon sets, gradients, data bars, unlimited rules, or public sharing.

## Existing implementation and contracts

- `web/src/components/grid/DataGrid.vue` already has `.grid-highlights`, `highlightDrafts`, an ordered rule list, `FilterBuilder`, Add/remove/reorder actions, and a 20-rule limit. The current trigger uses `t('grid.colour')`; inspect callers before replacing/removing that key.
- Start with that component, its tests, `web/e2e/highlight.spec.ts`, and existing column-state/highlight helpers. Reuse the implementation rather than creating a second config or evaluator.
- D-177: maximum 20 ordered rules; six semantic palette tokens; first matching rule wins independently for each row/cell target. Preserve row/cell precedence, light/dark contrast, bigint/date filter precision, supported field validation, and server-side parameterization.
- Reuse existing view-version persistence, permissions, local overrides, and conflict/error reporting. Do not expand shared-view write access to expose the UI. Incomplete drafts stay local and must not corrupt saved rules.
- Leave color-picker labels meaning Color/Colour: only the feature/menu name becomes Highlight. Keep persisted `highlight`, `color`, and palette token contracts unchanged; the internal `colour` icon name need not change.
- All visible labels/help/errors and accessible names use en/zh-CN keys. Own a dedicated `grid.highlightMenuLabel` key if a new trigger label is needed, plus highlight-specific keys; do not edit the Visibility namespace.

## Parallel ownership

- Own only the highlight subtree/logic/styles in `DataGrid.vue`, highlight-related tests in `DataGrid.test.ts`, and `web/e2e/highlight.spec.ts`. Keep CSS scoped or use existing highlight-specific selectors.
- A separate task adds a Visibility slot/menu immediately AFTER the highlight block, before row height. Do not move, remove, or implement that slot/menu and do not edit `ResourcePage.vue`.
- A separate task owns field tiles/forms/drag. Do not generalize this task into field-menu changes.
- Locale files/docs are shared: re-read before surgical edits and serialize writes. No concurrent gates/builds/commits in the same checkout. Fully independent runs need separate working copies, not new branches/worktrees contrary to repository policy. Never revert or stage another session's changes.

## Verification (D-158)

- Focused loop: `pnpm --dir web exec vitest run src/components/grid/DataGrid.test.ts` and directly affected highlight/helper tests.
- Extend `web/e2e/highlight.spec.ts` to assert the visible Highlight trigger and accessible name, adding/editing/removing several rules, saved order after drag and Move up/down, reload persistence, row and selected-cell targeting, independent first-match precedence, and unchanged unrelated view config.
- Cover incomplete drafts, 20-rule boundary, permissions/conflicts, light/dark and both locales. Reuse existing focused coverage rather than duplicating broad suites.
- Task gate after shared changes stabilize: `go run .\tools\ptdev check`; `go run .\tools\ptdev e2e --fast -- e2e/highlight.spec.ts`. Preflight Podman/mirrors; do not run the full production-image suite per task.

## Wrap-up

- Update directly related frontend/user-guide wording and AT-505 evidence for verified results. Do not add a new architectural decision for a label-only change; record any actual non-trivial choice.
- Follow `docs/task-prompts/TEMPLATE.md`: progress row/change-log line and a concise record appended here (about 2 KB).
- Commit only this task's changes after gates pass, using Conventional Commits and the required co-author trailer. Serialize shared-checkout commits; never stage another session's changes.

## Record (2026-11-04)

- Outcome: renamed the Grid menu to Highlight in English and Chinese and
  explained independent ordered rules, row/cell targets and cell precedence.
  Preserved the existing palette, permissions, view persistence and 20-rule cap.
- Corrected cell-over-row styling and contrast for highlighted content. Kept
  incomplete list/range/JSON-path drafts local, retained drafts across saves,
  and serialized value-less conditions as explicit `null` for server validation.
- Verification: focused Grid and view-column suites passed 80/80;
  `pnpm --dir web i18n:check` passed with 1,260 matching keys. The Chromium
  matrix covered all 36 row/cell palette combinations in both themes.
  `go run .\tools\ptdev check` passed lint, test and build;
  `go run .\tools\ptdev e2e --fast -- highlight.spec.ts` passed 2 with
  1 intentional mobile skip.
- An additional `table-workspace.spec.ts` run passed 2 with 1 intentional
  skip after the field-tiles follow-up fixed its stale type-picker selector.
  The complete production-image E2E remains a phase-end gate under D-158.
