# UX-09 - Small plus directly after the last grid row

Requires UX-08. Can run alongside UX-04.

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- [Execution index](workspace-grid-redesign.md).
- `docs/12-frontend.md` section 4; `docs/09-api.md` Rows & queries.
- Search decisions D-138, D-139, D-217.
- Search acceptance AT-204, AT-205, AT-303, AT-307, AT-313, AT-504.

## Scope

Position and verify the existing plus-row action, not a second row-creation API.

- DataGrid already has `addRow()` and `.grid-add-row`. Reuse them.
  The existing button follows a fixed-height RevoGrid element, leaving a gap
  below short tables; fix the final-row placement rather than adding a duplicate.
- Show a compact plus strip immediately after the last displayed data row,
  including an empty grid. A click sends one ordinary blank-row creation request,
  without opening a form. Repeated activation while saving cannot duplicate it.
- Treat the plus as an action, never a fake row in query counts, clipboard,
  sorting, selection, grouping, aggregate or export data.
- In a large virtualized table, put it at the logical end of the result, not at
  the end of each fetched page. Keep DOM and data windows bounded. No sticky
  bottom-of-viewport button misrepresented as "after the last row".

## Ownership and edge cases

Own DataGrid plus-row positioning/layout and append behavior tests only.
UX-04 owns ResourcePage and navigation mapping. Do not edit those surfaces.
No scalar/choice interaction changes or New Entry header work here.

Preserve existing mutation, undo, realtime and re-query behavior. Required fields
without defaults may reject a blank create; show a localized error and point to
the upcoming New Entry action, without fabricated values or silently opening a
different workflow. Defaults are server-applied.

Filters/sort/group mean a new blank record may not appear in the visible query;
requery and communicate this honestly. Do not append a row that violates the
active query or fabricate an insertion position. For grouped grids use one
table-level append action after the final displayed group, not one per header.
Collapsed groups, variable row heights and aggregate footers must remain correct.

## Acceptance scenarios

1. For 0, 1 and 5 rows, measure plus top against the final row/header bottom:
   no unused viewport-height gap (at most the normal border/spacing, <= 2 CSS px).
   Use multiple configured row heights and viewport sizes.
2. One click creates exactly one row directly; it survives reload, receives
   server defaults and participates in undo/realtime. Saving disables repeats.
3. Scroll through a multi-page virtualized result to its actual end. The action
   is not inserted after a loaded page or included in row counts/range operations.
4. Filtered/sorted/grouped/empty results show truthful outcomes and one append
   action; required/unique/network failures show errors without fake rows.
5. Readers cannot append. Keyboard focus/activation and permission changes work.
   Mobile's existing form/FAB creation is unchanged.

## Verification and wrap-up

Focused DataGrid/window/group tests; add `web/e2e/grid-append-row.spec.ts` with
bounding-box assertions, a paged result and exact API row-count checks.
Do not rewrite the unrelated existing AT-307 nine-vs-eight criterion.
Task gates/central docs/commits follow the execution index.

## Record (2026-11-05)

**Completed; coordinated task gates pass.** Changed `DataGrid.vue`,
`remoteWindow.ts`, their unit tests and `e2e/grid-append-row.spec.ts`.
The native footer reuses blank create/undo, never becomes data, and waits for
the writer's loaded visible logical tail. Both locales, frontend section 4
and D-222..224 are integrated.

Real-stack paged append exposed a viewport reset. Two timing-only fixes failed;
retaining empty placeholder geometry during authoritative ungrouped/grouped
requery fixed the reset without retaining stale rows. Grouped reveal loads
the anchored leaf page and checks identity. Failed counts clear placeholders.
Browser assertions wait for actual source/viewport readiness, not just ARIA.

Verification after both writers stopped:
- Focused grid/window/group Vitest: **102 passed**.
- Final `go run .\tools\ptdev check`: lint, required PG18/backend tests,
  **841 frontend tests** and build passed.
- `ptdev e2e --fast --` serialized affected-spec batches:
  `grid`, `grouping`, `grid-append-row`: **11 passed, 6 skipped**;
  `grid-columns-keyboard`, `grid-inline-editing`, `undo`, `mobile-rows`:
  **10 passed, 3 skipped**;
  `hardening`, `fields-acceptance`, `realtime` (desktop):
  **18 passed, 1 skipped**;
  `attachments`, `export`, `fields-batch2`, `row-expand`,
  `search-aggregates`, `sort`, `table-workspace`: **13 passed, 6 skipped**.
  Mobile realtime also passed in the preceding query/realtime batch.

Regression tests now follow workspace landing/sidebar creation, visibility
sharing and grid listboxes. Session-disable diagnostics allow only the exact
schema 401 race; row-popup timeout accommodates existing login pacing.
Interrupted infrastructure runs and earlier red batches were superseded by
the gates above. No production-image gate or commit was run.

UX-11 owns phase verification, acceptance IDs and commits. AT-307's literal
nine-versus-eight discrepancy and UX-08's recorded gaps remain unchanged.
