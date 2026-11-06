# WG-02 BOOK - current-workbook navigation

Read [execution plan](workspace-grid-refinement.md). Requires WG-01 handoff.

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- `docs/12-frontend.md` section 2 and "UX-05 sidebar views".
- Decisions: D-102, D-158, D-219, D-225, D-230.
- Acceptance: AT-214..217, AT-506..508, AT-513.

## Scope / ownership

- Own NavContents, NavNode, `useNavLocation.ts` and task-specific navigation
  tests; adjust existing navigation/view-navigation tests only where behavior
  intentionally changed. Own relevant assertions in
  `web/e2e/workspace-navigation.spec.ts` and `views.spec.ts`.
- On workbook, table, explicit view and row-deep-link routes, render a separate
  contextual tree for **only the active workbook**: workbook > table > view.
  Keep the all-workspace section flat. Selecting a workspace leaves workbook
  context; returning through a card restores the correct tree.
- Reuse existing route resolution, NavNode, structure actions and view menus.
  Preserve create/rename/delete/reorder/share and view-management capabilities;
  move workbook-level actions into the contextual section if necessary.
- Show an accessible way back to workspace cards. Do not add a new route,
  auto-open a table, or fetch every workbook schema.
- Do not edit ResourcePage, DataGrid, viewManagement store contracts or grid tests.
  New scoped styles should remain local; global edits go through coordinator.

## Contracts

- Resolve context from authorized nav ancestry, never solely remembered IDs.
  During loading/stale/403/404 or identity changes, do not flash another
  workbook's tree or retain unauthorized context.
- Preserve server ordering, path-only restrictions, same-parent reorder,
  current-route highlighting, mobile drawer behavior and deep-link URLs.
- Preserve personal/shared/locked views and effective-owner rules.
- WG-05 runs concurrently: do not touch its grid/aggregate files or spec.

## Confined verification

1. WG-02.1: opening a card shows its workbook/tables/views only; other workspace
   entries have no nested children.
2. WG-02.2: direct table, view and row links select/expand the right context;
   switching workspaces/back clears context; refresh preserves route identity.
3. WG-02.3: view creation/rename/lock and authorized structure controls remain
   reachable; readers/path-only users cannot gain controls; stale loads cannot
   cross routes/users.
4. Run focused navigation/view-navigation unit files plus the new contextual
   cases in one Vitest command; touched-file ESLint. Add focused E2E cases for
   WG-09; no full gates, image builds or commits.

## Handoff / wrap-up

Record source ownership released, exact test counts and next steps.
Coordinate wave-2 shared translations/docs after WG-05 stops editing. Update
frontend navigation docs to the new two-context behavior, preserving history.
Leave **implemented, awaiting WG-09 gates**.

## Record

Implemented; awaiting WG-09 gates (2026-11-06).

- Added a route-resolved contextual tree beside the flat workspace list. It
  renders only the authorized active workbook on workbook/table/view/row routes,
  hides during navigation loading or stale errors, and clears on identity
  changes. Reused `NavNode` and existing structure/view actions, preserving
  workspace links, same-parent reorder, path-only controls and route highlights.
- Updated navigation/view unit coverage and `workspace-navigation.spec.ts` for
  route selection, refresh, workspace-card return, and restricted ancestry.
  Updated the frontend navigation contract. No locale keys were needed.
- Vitest: 3 files, 17 tests passed. Touched-file ESLint and Prettier checks
  passed. E2E was not run, per the refinement plan; WG-09 owns its execution
  with the consolidated task gates and phase gate.
- Source ownership released: `NavContents.vue`, `NavNode.vue`,
  `useNavLocation.ts`, navigation/view tests, and workspace-navigation E2E.
  Next: WG-09 runs the focused E2E and consolidated D-158 gates; leave changes
  uncommitted.
