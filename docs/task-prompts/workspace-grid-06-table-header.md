# UX-06 - Consolidate table headers and remove the options panel

Requires UX-05. Run independently of all other header/ResourcePage/grid writers.

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- [Execution index](workspace-grid-redesign.md), UX-05's handoff.
- `docs/12-frontend.md` sections 2 and 4 and T1.12 sharing.
- Search decisions D-209, D-210, D-214, D-215, D-217.
- Search acceptance AT-204, AT-305, AT-311, AT-506..AT-508, AT-701, AT-704.

## Scope

One coherent header layout; no new entry form or grid-editing changes.

- Outer header, immediately after the sidebar collapse area: show selected
  workspace name with useful book/table/view context. It must also work on the
  workspace page and when the sidebar is hidden.
- Consolidate the inner table header into the existing Fields, Filter, Group,
  Sort, Highlight, Visibility, row-height and other table-tools surface. Retain
  undo/redo, presence and current search/aggregate/column controls without a
  redundant empty band. Reserve the rightmost action position for UX-10.
- Put Import/Export in this inner content header, not a hidden table-options
  panel. Reuse the existing import dialog and export functions/query context.
- Keep ACL sharing in the existing Visibility menu. Do not conflate this with
  Fields column visibility or view personal/shared scope. Retain mobile and
  non-grid visibility/import/export access where already supported.
- Completely remove the **Views and table options** trigger and its panel,
  obsolete empty containers/styles and dead state. Do not leave it hidden in CSS.

## Ownership and preservation audit

Own AppLayout/AppBreadcrumbs, ResourcePage header composition, DataGrid toolbar
slots only (not editors/row creation), affected styles and tests.
Inventory every old panel capability before deletion: display-field/schema
controls remain in Fields/Edit/Advanced; access information must not become
misleading; UX-05 owns view commands; Import/Export move here. No lost actions.
Prefer existing slots/small components over a global command registry.

CSV must retain current view filters, sort and visible columns; package export
retains its existing semantics. Import permissions remain write/owner; export
remains available to authorized readers. Errors/progress stay visible.
Long names and toolbar overflow must not hide critical actions on small screens.
Do not redesign export formats, field visibility or sharing authorization.

## Acceptance scenarios

1. Workspace name appears next to the collapse region on workspace/book/table
   routes and updates on navigation; no duplicate welcome heading.
2. No English/Chinese options-panel trigger exists, via mouse or keyboard.
   All surviving tools and UX-05's view actions are still reachable.
3. Owner's Visibility opens the existing share dialog and returns focus.
   Non-owners cannot gain grants UI; field visibility remains in Fields.
4. Import works from the header for writers and is denied for readers.
   CSV exports exactly the active filtered/sorted/visible columns; XLSX and
   progress/failure handling still work. Repeat after changing view/table.
5. Grid/Form/Kanban/Gallery and mobile preserve their applicable actions.
   Undo/redo, presence, column tools and avatar/language/theme controls still work.
6. Both languages/themes, keyboard and 390 px layouts have no obscured popovers
   or horizontal page overflow beyond the grid itself.

## Verification and wrap-up

Focused header/ResourcePage/toolbar tests, plus affected fast specs:
`shell.spec.ts`, `table-workspace.spec.ts`, `visibility-menu.spec.ts`,
`import.spec.ts`, `export.spec.ts`, `views.spec.ts`.
Update narrow selectors/helpers without reducing assertions; UX-11 handles the
remaining batch-wide helper migration. Follow index gates/docs/commit rules.
Publish the exact rightmost action slot/props for UX-10.

## Record

Not started.
