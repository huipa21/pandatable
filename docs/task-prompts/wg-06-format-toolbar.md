# WG-06 FMT - Format and removal of toolbar ellipsis

Read [execution plan](workspace-grid-refinement.md). Requires WG-05.

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- `docs/12-frontend.md` section 4 and table-toolbar/column-persistence sections.
- `docs/08-query-engine.md` sections 1 and 5.
- Decisions: D-158, D-214, D-226, D-229, D-230.
- Acceptance: AT-305, AT-311, AT-327, AT-509, AT-510.

## Scope / ownership

- Own DataGrid toolbar/layout and relevant viewColumns code/tests; add a focused
  `formatToolbar.test.ts`. Own relevant updates in
  `web/e2e/grid-columns-keyboard.spec.ts` and `table-header.spec.ts`.
- Replace row-height-only trigger with a localized **Format** control containing
  existing row-height presets and per-field width settings with current bounds
  and explicit invalid-input feedback.
- Remove grid-more trigger/panel completely. Footer settings have moved in
  WG-05. Move quick search to an accessible dedicated search trigger/input,
  keeping its debounce/filter/query semantics and keyboard behavior.
- Preserve column order/freezing with their existing controls under Format;
  use existing Fields UI for visibility rather than another duplicate list.
  Retain drag/resize and keyboard/touch alternatives.
- Remove the old hidden Delete control; WG-07 supplies visible selection
  actions next. Do not implement bulk edit or remove backend deletion.
- Keep sidebar/table/field context menus, ACL Visibility, history, highlight,
  grouping, sorting, Fields, New Entry, WG-08 transfer icon and presence.
- Touch ResourcePage only if a proven popup/toolbar coordination issue requires
  it; preserve WG-01 cards. Do not edit TableToolbarActions.

## Contracts

- Width/height config, writer persistence, reader local overrides, locked shared
  views, save-as-personal, optimistic conflict/retry remain unchanged.
- Menu dismissal, focus return and mutual exclusion still work; no clipped
  panel or unnecessary second mostly-empty header row at normal desktop width.
- No new sizing system, API or dependency. As the sole wave-3 session, integrate
  both translations/docs serially under the plan's coordination rules.

## Confined verification

1. WG-06.1: Format changes height and width, survives reload for writers and
   stays local for readers/locked views; invalid widths show errors.
2. WG-06.2: no table-toolbar ellipsis or old hidden contents remain; search,
   Fields visibility, order/freeze and footer pickers are reachable.
3. WG-06.3: keyboard navigation/PageDown, resizing and popup close/focus behavior
   remain; existing field/sidebar ellipses are unaffected.
4. Run focused Format/viewColumns and affected DataGrid unit selectors, plus
   touched-file ESLint and i18n check. Add owned browser cases for WG-09.
   No full check, E2E execution, image builds or commits.

## Handoff / wrap-up

Update own Record (counts, changed files, selection-action insertion point),
progress and directly related frontend docs. Leave
**implemented, awaiting WG-09 gates**.

## Record

Not started.
