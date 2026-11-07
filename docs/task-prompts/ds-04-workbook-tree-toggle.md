# DS-04 TREE - workbook row split and collapsed-by-default tree

Read [execution plan](dashboard-sidebar.md) first. Implement only DS-04. Wave 1.

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- `docs/12-frontend.md` section 2 and "UX-05 sidebar views".
- Decisions: D-102, D-158, D-225, D-230, D-234.
- Acceptance: AT-215, AT-506..508, AT-513.
- The [WG-02 record](wg-02-workbook-tree.md#record) (contextual-tree contract).

## Scope / ownership

- Own `web/src/components/navigation/NavNode.vue`, `navigation.test.ts`,
  `viewNavigation.test.ts` and the tree assertions in
  `web/e2e/workspace-navigation.spec.ts` and `views.spec.ts`.
- Split the workbook row. The chevron is its own button
  (`aria-expanded`, label "Expand/Collapse {name}") and only toggles. The book
  icon and name are one `RouterLink` to the workbook route, with
  `aria-current` when that route is current. They emit `navigate` (closes the
  mobile drawer). Match the table row layout (toggle, then link).
- Default state: the workbook node and every table start **collapsed**.
  Auto-expand only the ancestors of the current table/view route (workbook for
  a table route; workbook and table for a view route) so the active item is
  visible. Never auto-collapse what the user expanded.
- Keep the expansion state for the active workbook while navigating inside it
  during the session (e.g. a small map keyed by node ID in the component tree or
  `sessionStorage`). Switching workbooks starts the new one collapsed, plus its
  route ancestry. Do not persist expansion to the server.

## Contracts

- Preserve drag/reorder (same parent), NavActions menus, path-only rows,
  view actions, keyboard focus order and mobile drawer behaviour.
- Do not edit NavContents, the nav store, ResourcePage or the grid.
- The new labels reuse existing keys where possible. Put proposed new keys in
  the Record for the DS-03 coordinator.

## Confined verification

1. DS-04.1: on a workbook route only the workbook row is visible (collapsed).
   Clicking the chevron reveals tables without navigating. Clicking the name
   navigates to the workbook page and does not toggle.
2. DS-04.2: a direct table/view link expands exactly its ancestry, with the
   active item highlighted. Refresh gives the same result.
3. DS-04.3: a manual expansion survives table-to-table navigation in the same
   workbook. Another workbook starts collapsed. Keyboard Enter/Space on the
   chevron toggles; Enter on the link navigates.
4. Run `pnpm --dir web exec vitest run src/components/navigation/navigation.test.ts src/components/navigation/viewNavigation.test.ts`
   and touched-file ESLint. Update the E2E specs; DS-10 runs them.

## Handoff / wrap-up

Record the changed files, counts and the release of NavNode/navigation tests to
DS-09. Leave it **implemented, awaiting DS-10 gates**.

## Record

Implemented; awaiting DS-10 gates (2026-11-07).

- Split the workbook row into a labeled chevron button and a workbook-route
  link. Workbook/table expansion starts closed, route ancestry opens as needed,
  active-workbook state survives navigation, and changing workbooks resets it.
  Reused `nav.toggle`; no locale-key handoff is needed.
- Updated `NavNode.vue`, `navigation.test.ts`, `viewNavigation.test.ts`,
  `workspace-navigation.spec.ts` and `views.spec.ts`. Navigation Vitest:
  2 files, 16 tests passed. Touched-file ESLint, Prettier, frontend typecheck
  and `i18n:check` passed. E2E was not run; DS-10 owns those specs.
- Released `NavNode.vue` and the navigation tests to DS-09. No central progress,
  locale or design-document edits; DS-03 coordinates those shared files.
