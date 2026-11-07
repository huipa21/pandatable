# DS-07 DASH - workspace dashboard on /home

Read [execution plan](dashboard-sidebar.md). Requires DS-02 and DS-06 handoffs.
Wave 3 **coordinator** for locales, progress and decisions.

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- `docs/12-frontend.md` section 2 and "T1.11 navigation"; `docs/09-api.md`
  "UX-01 workspace preference".
- Decisions: D-218, D-219, D-230, D-234 and the DS-01/DS-02 decisions.
- Acceptance: AT-214, AT-215, AT-208.

## Scope / ownership

- Own `web/src/pages/HomePage.vue`, `HomePage.test.ts`, new
  `web/e2e/dashboard.spec.ts`, `default-workspace.spec.ts`, and the workspace
  target in `web/e2e/sharing.ts` (the helper function named in the DS-05 Record).
- `/home` no longer redirects. It renders:
  - A heading ("Workspaces") with a **New workspace** button that opens the
    DS-06 dialog.
  - A **full-width search bar** (`type="search"`, labelled, `/` or Ctrl+K
    focus is optional and must not clash with SearchPalette).
  - A list (`role="list"`) of every accessible workspace from
    `useResourceSearch`. Each row is one link to `/w/{id}`: icon (custom or
    fallback), name and, after the name, the description (truncated to one line
    on desktop, with the full text in `title`/expanded on focus). It also shows
    the existing Default marker and a path-only badge.
  - A per-row action menu (reuse NavActions items): Edit, Share, Set default,
    Delete, each permission-gated as today.
  - No query: recency order (DS-02). Query: ranked matches. The no-match state
    offers a clear-search action. The empty state (no workspaces) shows the
    New workspace call to action.
- Opening a row records the open (DS-02 hook does this via the route) and
  navigates. Keep loading, stale and retry states from the nav store.
- The `resolve` preference call still runs once for uninitialized users so the
  starter workspace is provisioned. It no longer navigates.

## Contracts

- AT-214 changes: sign-in lands on the dashboard, and explicit links still win.
  Record the AT revision text for DS-10. AT-208: path-only ancestors appear as
  rows without description or actions.
- Do not edit NavContents, ResourcePage or NavNode (wave-3 peers).
- Wave 3 coordinator: integrate the DS-08/DS-09 locale/doc/progress handoffs
  after they stop; run `i18n:check`.

## Confined verification

1. DS-07.1: rows show icon, name and description. No query gives recency order
   with never-opened rows after (default first, then server order).
2. DS-07.2: search by name/description (incl. CJK), clear, no-match and empty
   states. Hidden workspaces never appear.
3. DS-07.3: row actions match permissions. Create from the dashboard shows the
   new row on top after opening it. Keyboard and 390 px layouts work.
4. Run `vitest run src/pages/HomePage.test.ts`, touched-file ESLint,
   `typecheck` and `i18n:check`. Write the E2E cases; DS-10 runs them.

## Handoff / wrap-up

Record the changed files, counts and the AT-214/215 revision text. Leave it
**implemented, awaiting DS-10 gates**.

## Record

- Implemented `/home` as a no-redirect dashboard using authorized navigation
  rows, DS-02 recency/search ordering, the DS-06 create/edit dialog and
  permission-gated `NavActions`. Path-only rows omit descriptions and actions.
  Preference resolution still provisions/repairs the default, with visible
  loading, stale, retry and resolver-error states.
- Reused DS-08's `ResourceSearchInput`; integrated its four locale keys in both
  locales. Dashboard search is transient/local while workspace search remains
  URL-backed (D-235).
- Changed `HomePage.vue`/test, `dashboard.spec.ts`, `default-workspace.spec.ts`,
  the workspace branch of `sharing.ts`, both locale files, frontend/acceptance
  docs, and task/progress/decision records. AT-214 DS-10 revision text: “A new account owns a workspace
  named `Default`; renaming it does not recreate a starter. Signing in lands on
  `/home`, the workspace dashboard; visiting `/home` resolves/provisions or
  repairs the preferred workspace without navigating away. Explicit resource
  links win. A reader can change the preferred workspace from an accessible
  dashboard row action. A lost preference repairs once without changing other
  users.” AT-208 path-only ancestors remain visible as rows without
  descriptions/actions.
- AT-215 DS-09 revision text: “The sidebar always provides a Workspaces link to
  `/home`; on workspace and deeper routes it shows only the current workspace
  link with its custom or fallback icon. Workbook/table/view/row routes also
  show a separate tree for only the authorized active workbook. No other
  workspace names or workspace actions appear in the sidebar. Breadcrumbs start
  with a Workspaces link to `/home`; selecting the current workspace returns to
  its workbook cards.”
- DS-09's frontend/AT-215 documentation and progress handoff is integrated.
- Verification: HomePage Vitest 7/7; workbook-page Vitest 39/39; touched-file
  ESLint, typecheck and `i18n:check` pass (1,311 keys). Playwright discovered
  desktop/mobile dashboard cases; execution and consolidated gates remain with
  DS-10.
