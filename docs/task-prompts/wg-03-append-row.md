# WG-03 ADD - full-width append action

Read [execution plan](workspace-grid-refinement.md). No prerequisites.

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- `docs/12-frontend.md`: search "append", "blank-row" and section 4.
- Decisions: D-158, D-222..224, D-227, D-230.
- Acceptance: AT-305, AT-313, AT-330, AT-331.

## Scope / ownership

- Own DataGrid's append-action markup/scoped geometry and task-specific cases
  in `web/src/components/grid/appendRowLayout.test.ts`.
- Own focused updates to `web/e2e/grid-append-row.spec.ts`.
- Make the action a single complete row beneath the real data, spanning the
  rendered table width rather than only the first column. Cover at least the
  viewport width when columns are narrower, and the full rendered column span
  when wider; clipping follows the grid's normal horizontal viewport.
- Keep the plus visually at the leading edge and make the entire row the
  accessible click target. Do not duplicate the button, invent a data row or
  replace native grid virtualization.
- Reuse the native footer integration and existing addRow mutation, permissions,
  end-of-data visibility, pending state, defaults, undo and New Entry fallback.
- Do not change aggregate logic/markup, toolbar, API/field contracts or global
  CSS. Hand off footer geometry details to WG-05.

## Confined verification

1. WG-03.1: empty and two-column tables have a full-width clickable append row.
2. WG-03.2: many columns, resize/reorder/freeze, horizontal scrolling and grouped
   tails preserve row width/alignment without covering data or headers.
3. WG-03.3: one click creates exactly one row; pending prevents duplicates;
   readers see none; required-field failure remains explicit; long datasets
   show it only at the existing logical end; keyboard activation works.
4. Run the new focused unit file and existing DataGrid append tests using
   Vitest file/name selectors. Add browser bounding-box/click-target assertions
   for WG-09; do not claim layout verified from unit/CSS tests alone.
5. Touched-file ESLint only; no full checks, image build or E2E execution here.

## Handoff / wrap-up

Put exact en/zh-CN/style/doc proposals in own Record for WG-01; do not edit
shared files concurrently. Record test counts and footer integration contract.
Leave **implemented, awaiting WG-09 gates**, no commit.

## Record

### Implementation and handoff

Status: **Implemented, awaiting WG-09 gates**; no commit. `DataGrid` keeps the
native RevoGrid footer and existing add-row mutation, while sizing its single
button to the larger of the grid viewport and the visible rendered column
widths. The width is refreshed on column changes and by the existing grid
render/resize observation; RevoGrid continues to clip overflow through its
horizontal viewport. No locale, API, aggregate or shared-document changes were
needed.

The focused unit file covers viewport-width, wide-column, and reorder/freeze
width cases. The append E2E spec adds full-row bounds/alignment assertions,
clicks the trailing visible edge, and covers resized wide columns, persisted
reordering/freezing, horizontal scrolling and an expanded grouped tail. Browser
E2E was not run in this quick task; WG-09 owns its geometry execution and the
deferred gates.

Checks: focused Vitest — **20 passed** across the layout and DataGrid files;
touched-file ESLint and Prettier pass. Browser E2E remains queued for WG-09.
