# DS-09 SIDE - sidebar without the workspace list

Read [execution plan](dashboard-sidebar.md). Requires DS-04 and DS-06 handoffs.
Wave 3.

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- `docs/12-frontend.md` section 2, "T1.11 navigation", "UX-05 sidebar views".
- Decisions: D-102, D-219, D-225, D-230, D-234.
- Acceptance: AT-208, AT-215, AT-513.

## Scope / ownership

- Own `web/src/components/navigation/NavContents.vue`, `NavSidebar.vue`,
  `AppBreadcrumbs.vue`, `flatWorkspaces.test.ts`, `navigation.test.ts`,
  `header.test.ts`, `web/e2e/workspace-navigation.spec.ts` and `shell.spec.ts`.
- Remove the "Workspaces" heading, the + button and the full workspace list
  from the sidebar. Show instead:
  - A **Workspaces** link to `/home` (`aria-current` on the dashboard).
  - On workspace or deeper routes, the current workspace as a link with its
    icon (custom or fallback), so users can return to its workbook cards.
  - On workbook/table/view/row routes, the existing active-workbook tree
    (DS-04 behaviour), unchanged.
- Breadcrumbs start with a "Workspaces" root crumb linking to `/home`.
- Delete `flatWorkspaces.test.ts` or repurpose it for the new contract. Keep
  NavActions for workbook/table nodes. Workspace actions now live on the
  dashboard (DS-07).

## Contracts

- AT-208: a table-only grantee still sees the W > B > T path (current workspace
  link, then the workbook tree) and no siblings.
- Preserve mobile drawer open/close-on-navigate, loading/stale/empty states,
  route highlighting and the sidebar collapse toggle.
- Do not edit HomePage, ResourcePage or NavNode. Locale handoffs go to DS-07.

## Confined verification

1. DS-09.1: dashboard route: the sidebar shows only the Workspaces link (current).
   Workspace route: Workspaces + current workspace. Workbook route: + tree.
2. DS-09.2: no other workspace names appear anywhere in the sidebar DOM.
   Breadcrumb root navigates to `/home`.
3. DS-09.3: path-only and table-only grants, deep links, refresh and the mobile
   drawer behave correctly.
4. Run `vitest run` on the navigation/header/flat tests, touched-file ESLint
   and `typecheck`. Update the E2E specs; DS-10 runs them.

## Handoff / wrap-up

Record the changed files, counts, the AT-215 revision text and locale handoffs.
Leave it **implemented, awaiting DS-10 gates**.

## Record

- Implemented, awaiting DS-10 gates. The sidebar now always links to the
  Workspaces dashboard and shows only the route's current workspace, including
  its custom/fallback icon; the active-workbook tree and NavActions remain
  scoped to workbook/table nodes. Breadcrumbs begin with the dashboard link,
  kept visible on mobile alongside the current crumb.
- Changed (8 files including this record): `NavContents.vue`,
  `AppBreadcrumbs.vue`, `style.css`, `flatWorkspaces.test.ts`,
  `navigation.test.ts`, `header.test.ts`, and `workspace-navigation.spec.ts`.
  Replaced flat-list tests with dashboard, workspace-context, breadcrumb and
  table-only ancestry coverage.
- AT-215 revision handoff: **AT-215 — Contextual workspace sidebar** · UX ·
  The sidebar always provides a **Workspaces** link to `/home`; on workspace
  and deeper routes it shows only the current workspace link with its custom or
  fallback icon. Workbook/table/view/row routes also show a separate tree for
  only the authorized active workbook. No other workspace names or workspace
  actions appear in the sidebar. Breadcrumbs start with a **Workspaces** link
  to `/home`; selecting the current workspace returns to its workbook cards.
  Executable: `workspace-navigation.spec.ts`; AT-208 covers table-only
  ancestry.
- Documentation handoff integrated by DS-07: `docs/12-frontend.md` now
  describes the dashboard link, current-workspace context and root breadcrumb;
  AT-215 is revised and its gate is pending DS-10. No locale keys were needed:
  reuse `workspaceGrid.ux02.workspaces` from both catalogs.
- Verification: focused Vitest passed (3 files, 16 tests); typecheck and ESLint
  passed for the six TS/Vue files; Prettier passed for all seven code/style
  files; i18n check passed (1,311 matching keys). Playwright discovered the
  desktop and mobile DS-09 cases; browser E2E and consolidated task gates remain
  with DS-10.
