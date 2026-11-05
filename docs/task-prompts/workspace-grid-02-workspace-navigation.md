# UX-02 - Workspace list and default landing

Requires UX-01's concrete API handoff. Can run alongside UX-03 and UX-08.

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- [Execution index](workspace-grid-redesign.md) and UX-01's Record only.
- `docs/12-frontend.md` sections 2 and the T1.11 navigation subsection.
- Search decisions D-104..D-107, D-209, D-217.
- Search acceptance AT-201, AT-204, AT-208, AT-213, AT-802, AT-806.

## Scope

Replace workspace selection/landing navigation, not workbook presentation.

- In the sidebar show a non-interactive `Workspaces` title with Add (+) at its
  right end. List **all accessible** workspace contexts, not just the current
  one. Clicking an eligible workspace opens its page; expand/collapse is a
  separate keyboard-accessible control. Keep books/tables/views underneath.
- Reuse StructureDialog for create/rename. Workspace actions include setting
  the user's preferred default and a visible/accessibly named default marker.
  Keep owner-only rename/delete and path-only restrictions.
- `/home` becomes a loading/error-aware entry resolver to UX-01's preferred
  workspace. Login, root and logo navigation reach it. Remove the old welcome,
  signed-in-as, chooser, instructional copy and content New Workspace action.
  Keep auth/setup/password-change/admin/profile routes intact.
- Explicit deep links never get replaced with the default. Selected workspace
  and default are distinct: browsing another workspace does not change preference.
  Identity changes clear cached selection; failures have retry, not fake empty UI.

## Ownership and boundaries

Own `HomePage.vue`, navigation components/stores/tests, `useNavLocation.ts`,
router entry behavior and the AppLayout selection watcher if needed.
Leave ResourcePage, book icons, DataGrid, table-header composition and view-create
actions to their owners. UX-03 owns API types and generated backend contracts
in this wave; consume UX-01's DTOs without independently modifying them.
Global CSS/locales/central docs go through the wave coordinator.

Existing code currently clears `lastWorkspaceId` on `/home`, renders only one
workspace in NavContents and places table actions in NavActions. Replace those
assumptions together; do not regress sidebar collapse, mobile drawer or avatar.
Keep server ordering, path-only ancestry and structure-owner checks.

## Acceptance scenarios

1. Empty/new-user launch reaches `Default` without displaying the old landing
   copy. Returning launch opens the preferred workspace. No navigation loop.
2. With three accessible workspaces, all three appear under one static heading;
   title-line Add opens creation and the new workspace appears after success.
3. Owner rename persists. Changing default and signing out/in opens the new
   preferred workspace; visiting a different workspace alone does not.
4. Deep-link a table/view/expanded row in a non-default workspace; it stays there
   and highlights the correct workspace. Back/forward navigation still works.
5. A table-only grant shows only authorized ancestry/descendants, not siblings
   or owner controls. It cannot be selected as an eligible preferred workspace.
6. Desktop collapsed sidebar and 390 px drawer remain usable; keyboard activation,
   Escape, focus return, both languages/themes and stale/retry states work.

## Verification and wrap-up

Focused checks: `nav.test.ts`, navigation/header tests and router tests affected
by the change. Extend UX-01's `default-workspace.spec.ts` and shell/navigation
coverage. Gates, evidence and serialized commits follow the execution index.
Preserve separate scenarios even if the existing multi-scenario browser fixture
needs selectors updated later by UX-11.

## Record

Started 2026-11-05: implementing the accessible workspace sidebar and
preferred-workspace entry navigation using UX-01's API handoff.

Implementation handoff: the sidebar renders all authorized contexts in server
order with separate workspace selection/expansion, owner structure controls and
a named default marker/action for eligible readers. `/home` resolves and replaces
itself with the preferred workspace; explicit links, history and identity changes
are guarded against stale requests. Preference and navigation failures retain
retry affordances even with the sidebar collapsed or the mobile drawer open
(D-219). Shared UX-03 navigation fixture now supplies its required icon fields.

Verification: focused navigation/sharing/header/home/router Vitest checks passed
33 tests. The combined `ptdev check` passed lint, required PG18 tests and build
(816 frontend tests; 1,269 matching locale keys). The requested UX-03 PG18 icon,
upgrade, API and migration-regression selectors also passed. Fast browser
coverage for `default-workspace`, `workspace-navigation`, `workbook-icons`,
`grid-inline-editing` and `shell` passed 12 tests, with 3 intentional mobile
skips (two desktop-grid scenarios and viewport-independent icon metadata).
The focused default/navigation browser rerun passed 5 desktop/mobile cases.

Stabilization: the first check exposed pre-0008 migration expectations, corrected
by the UX-03 owner. The first matched browser batch had three failures from
test timing and leaked shared preference; the scenarios now wait for the exact
creation response and restore preference/workspaces even after failure. The
first browser invocation also matched no tests with backslash path selectors;
spec-name selectors were used for the passing runs.

Status: task gate passed. UX-04 may consume the navigation handoff. Some older
multi-scenario browser fixtures still expect the retired `/home` chooser and
need coordinated selector updates in UX-11. The complete production-image
phase gate and coordinated commits remain deferred to UX-11 (D-158).
