# UX-05 - Create and manage views from the sidebar

Requires UX-04 (and transitively UX-02). Run without another navigation or
ResourcePage writer.

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- [Execution index](workspace-grid-redesign.md).
- `docs/12-frontend.md` navigation; `docs/09-api.md` Views;
  `docs/08-query-engine.md` view visibility/configuration.
- Search decisions D-036, D-104, D-209, D-217.
- Search acceptance AT-203..AT-205, AT-506..AT-508, AT-511.

## Scope

Relocate view creation and view management, preserving existing semantics.

- The ellipsis beside a table includes Create view. Open an accessible dialog
  for name, supported type and personal/shared scope. A table's menu must remain
  available to a reader who can create personal views even if they cannot
  rename/delete the table; do not reuse `item.canEdit` as view authorization.
- Existing/newly created visible views appear beneath the table. Expand/select
  the relevant table on successful creation and navigate to the new view.
- Move rename, duplicate, delete, reorder, shared lock/unlock and save-as-personal
  affordances to appropriate view-context menus or active-view actions. Preserve
  personal privacy, shared-write permissions, locks and version-conflict handling.
- Remove the old view list/create form from ResourcePage's management panel
  after relocation. UX-06 removes the remaining options panel itself.
- Confirm deleting a table also removes access to **all** its shared/personal
  views, including deep links and open sessions. Preserve soft delete,
  restoration, purge and applicable realtime invalidation.

## Ownership and handoff

Own NavActions/NavNode view affordances, a focused view-management dialog/store
or composable only if needed to reuse existing logic, ResourcePage view-management
extraction, and focused tests. Reuse existing APIs in `view_management.go`.
Do not create a second divergent view CRUD implementation in navigation.

The current NavNode already lists `item.views`; extend it rather than replacing
it. Current view mutations live in ResourcePage and call nav refresh. Handle
creation from a table menu even when another table/workspace page is open:
fetch only the needed authorized schema, guard route/user changes and restore
focus. Backend changes are limited to an actual failing descendant-access test,
not speculative hard-cascade deletion or granting parent ownership.

Hand off the remaining options-panel contents and relocated commands to UX-06.

## Acceptance scenarios

1. Create each supported view type from the table ellipsis; it appears beneath
   the correct table immediately and after refresh. No content-area create form.
2. Reader can create personal but not shared views; writer can create shared;
   another user cannot see a personal view. Locked shared behavior is unchanged.
3. Rename, duplicate, reorder, lock/unlock and delete remain reachable with
   correct authorization, focus and error/conflict feedback.
4. Create while a different table is open; no mutation or stale result lands on
   the wrong table/user. Table menu works by pointer, keyboard and touch.
5. Trash a table containing shared and two users' personal views: nav entries
   disappear, schema/query/view mutation/direct links fail appropriately for
   every former viewer. Restore the table and views/data return consistently.

## Verification and wrap-up

Focused view/navigation/ResourcePage tests; extend `views.spec.ts` and
`trash.spec.ts`. Run targeted PG18 tests only if a real service issue is changed.
Gates, translations, documentation and commits follow the execution index.

## Record

Not started.
