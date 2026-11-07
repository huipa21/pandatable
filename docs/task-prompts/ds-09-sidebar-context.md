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
