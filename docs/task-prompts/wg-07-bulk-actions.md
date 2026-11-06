# WG-07 BULK - selected-row Delete and Edit

Read [execution plan](workspace-grid-refinement.md). Requires WG-06.

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- `docs/09-api.md` "Rows & queries", bulk/guarded mutation and undo contracts.
- `docs/12-frontend.md` section 4, row editing and undo sections.
- Decisions: D-115, D-158, D-228, D-230; search guarded bulk/undo decisions.
- Acceptance: AT-302, AT-303, AT-305, AT-309, AT-313; search undo acceptance.

## Scope / ownership

- Own DataGrid selection/action wiring and focused tests. Add a small bulk-edit
  component only if it keeps typed editor logic clearer; reuse the existing
  field binding/renderer/editor helpers rather than duplicate value parsing.
- Own a new focused `web/e2e/bulk-row-actions.spec.ts`.
- A non-empty row selection shows a visible count, Delete and Edit without
  opening another menu. No controls for non-writers. Keep cell-range selection
  distinct from row-checkbox selection.
- Delete confirms and deletes exactly the selected IDs, including selected rows
  outside the current virtual page. Reuse existing DELETE, snapshots and undo.
- Edit opens a field picker and typed value editor, then applies that same
  normalized value to the chosen field on every selected row in one existing
  atomic bulk PATCH. Use actual selected IDs, never row positions or just the
  loaded page. Preview count/field and permit cancel without writes.
- Reuse authoritative previous values/guarded writes where applicable for
  accurate undo and conflict handling. Exclude read-only/system/computed fields;
  use established capability restrictions for relation/attachment-specific
  mutations, explaining unsupported choices rather than offering broken edits.
  Cover scalar, checkbox, select, date and other ordinary bulk-writable fields.
- Retain server row/cell/payload limits. Prevent unsupported selection growth
  where practical and always report limit violations before mutation. Never
  silently slice IDs, loop partial chunks or report partial writes as success.
  Accepted selections are all-or-nothing; limits must be visible to the user.
- Guard route/view/user/schema/permission changes and pending cell drafts.
  Handle errors without losing selection/draft; reset selection only after
  confirmed success or context change. Refresh rows, groups and summaries.
- No backend redesign, new API, migration or relaxed authorization.

## Confined verification

1. WG-07.1: select nonadjacent rows spanning loaded pages; only these IDs are
   deleted/edited; deselection/count/cancel and undo work.
2. WG-07.2: typed text, numeric, false, select and clear/null values preserve
   codecs; invalid/required/unique values fail atomically with visible errors.
3. WG-07.3: conflicts, request-size/row limits, removed fields, route/identity
   changes, revoked access, pending writes and duplicate submit are safe.
4. WG-07.4: grouped/filtered results and summaries refresh; row selection is not
   confused with range fill; reader controls and unauthorized writes absent.
5. Run focused new bulk-action unit tests and affected selection/undo DataGrid
   selectors; touched-file ESLint, i18n check. Add real API-backed E2E cases for
   WG-09; no broad check, image build, E2E run or commit.

## Handoff / wrap-up

As the sole wave-4 session, integrate both locales and related docs. Record
supported fields, explicit limits, focused counts and any unresolved risk.
Leave **implemented, awaiting WG-09 gates**.

## Record

Not started.
