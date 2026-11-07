# WG-05 FOOT - column-aligned aggregate pickers

Read [execution plan](workspace-grid-refinement.md).
Requires WG-03 footer geometry and WG-04 aggregate contract handoffs.

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- `docs/08-query-engine.md` sections 5, 6.
- `docs/12-frontend.md`: search footer aggregates and column persistence.
- Decisions: D-158, D-222..224, D-230; WG-04's new decision if added.
- Acceptance: AT-311, AT-504, AT-507..510, AT-330.

## Scope / ownership

- Own DataGrid aggregate/footer rendering and local styles, necessary
  `viewColumns.ts` changes and focused tests, including a task-specific
  `columnFooters.test.ts`.
- Own `web/e2e/search-aggregates.spec.ts`. WG-02 owns navigation files/specs.
- Replace the current detached aggregate list with a summary strip aligned to
  **every visible data column**, even before a function is selected. Each
  summary cell opens its own accessible type-aware function picker and shows
  the selected function/result. None clears it. Offer WG-04's percentages.
- Use the grid's existing/public layout and column information where possible.
  Scrolling, resize, reorder, visibility and frozen columns must move each
  summary with its column. Do not independently guess widths from field names,
  query private shadow internals unnecessarily or render/load the entire table.
- Keep full-width append row distinct, immediately after data; summaries below
  it must not overwrite/hide it or become synthetic records.
- Remove only the now-duplicated aggregate settings subsection of grid-more;
  WG-06 removes the rest. Preserve footer API, filter/search overrides,
  cancellation, schema guards, realtime refresh, error/loading state and
  persisted/local/locked-view behavior. No client-only aggregate arithmetic.
- Do not edit field/API contracts, navigation, transfer component or global CSS.

## Confined verification

1. WG-05.1: each visible field has an aligned summary trigger; switching
   count/empty/filled/unique/percentage/numeric/date functions yields correct
   API-backed values; clearing removes config; hidden fields stay hidden.
2. WG-05.2: widths/order/freeze/horizontal scroll/hidden columns preserve
   alignment, including empty and grouped grids with the append action.
3. WG-05.3: filters/search apply to whole results; loading/error/retry and stale
   replies are explicit; readers/locked views keep local-only changes.
4. Run focused footer/viewColumns tests and relevant DataGrid footer cases in
   one Vitest command; touched-file ESLint. Add browser geometry/value cases to
   the owned spec for WG-09. No broad check, E2E run or image build here.

## Handoff / wrap-up

Record changed files, test counts, layout/operation contract and what remains
in grid-more for WG-06. Send exact shared translation/docs proposals via own
Record to WG-02. Leave **implemented, awaiting WG-09 gates**, no commit.

## Record

Implemented 2026-11-06; **implemented, awaiting WG-09 gates**; no commit.

- Replaced the detached aggregate list with a summary strip in RevoGrid's
  footer, immediately after the distinct append action. It renders a picker for
  every visible data column (and an empty aligned cell for the grid's pinned row
  selector), using the live column order, widths and pin definitions. Frozen
  cells stick at their rendered offsets; hidden fields are omitted. No summary
  is represented as a data row.
- Pickers use each bound field's supported operations. Count, empty/filled,
  unique, numeric, date and percentage results remain API-backed; percentages
  are exact 0..1 server ratios formatted as percentages, with no client
  aggregate arithmetic. Clearing removes that field's config. Existing
  filter/search requests, abort/generation guards, schema checks, retry/error
  messaging, realtime refresh, and local-only read/locked-view overrides remain.
- Added localized labels: `Percent empty` / `空值百分比`,
  `Percent filled` / `非空值百分比`, `Percent unique` / `唯一值百分比`, and
  `Retry footer aggregates` / `重试底部汇总`.
- WG-02 shared-doc handoff: WG-04's proposed `docs/08-query-engine.md` §6
  contract lists `percent_empty`, `percent_filled`, and `percent_unique`, their
  0..1 ratios, zero-row behavior, and field-type capabilities;
  `docs/09-api.md` documents footer ratios as exact JSON strings (e.g. `"0.25"`).
  WG-09 recorded the ratio/zero-row rule in D-233; D-179 already covers exact
  footer strings. WG-05 made no shared-doc or decision edits.
- `grid-more` retains quick search, column options, grouping, sorting,
  highlights and row-height controls for WG-06. Removed only its duplicate
  aggregate-settings subsection.
- Changed `DataGrid.vue`, `columnFooters.ts` and its test,
  `DataGrid.test.ts`, `search-aggregates.spec.ts`, and both locale files.
  Added unit and browser cases for values, picker clearing, stale replies,
  retries, hidden/reordered/wide/frozen columns, horizontal scroll, grouped
  results, and the empty append state.
- Checks: focused Vitest — **126 passed** across three files; touched-file ESLint,
  `vue-tsc --noEmit`, Prettier and i18n parity pass. Browser E2E is not run here;
  WG-09 owns its execution and the deferred task/phase gates.
