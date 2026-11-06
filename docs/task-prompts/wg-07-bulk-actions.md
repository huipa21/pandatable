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

Implemented; awaiting WG-09 gates.

- `DataGrid` keeps checkbox row selection independent of RevoGrid cell/range
  selection. Writer-only toolbar actions expose a live count, direct Edit and
  confirmed Delete. Delete sends the exact selected IDs to the existing atomic
  DELETE API; Edit uses `FieldEditor`/bound codecs, previews count, field and
  normalized display value, then sends one atomic bulk PATCH for those IDs.
- Editable choices include all bound writable ordinary fields, including
  scalar/text, number, checkbox, select, date and table-scoped user fields.
  System, computed/read-only, relation and attachment fields are excluded;
  the dialog explains the relation/attachment restriction. Delete retains the
  1,000-ID/64-KiB API bound. Edit retains 1,000 rows/5,000 cells/512 KiB and
  reports a visible limit error without slicing or chunking.
- Both operations use existing delete snapshots or authoritative bulk-patch
  `previous` values for undo. Success clears selection and refreshes rows,
  grouping and footer aggregates; cancel/failure retains selection. Pending
  cell drafts and active editors block the actions. Route, view, identity,
  schema and permission changes reset selection, close dialogs and abort a
  pending bulk request.
- Focused verification: **118 Vitest tests passed** across DataGrid and field
  editor suites; strict frontend typecheck, touched-file ESLint, i18n parity
  (1,291 keys) and formatting pass. Added
  `web/e2e/bulk-row-actions.spec.ts` for API-backed typed edits, cross-page
  selected IDs, null/false/choice/date codecs, exact deletion and undo. Per
  WG-07 scope, the E2E was not run; WG-09 must run it and the consolidated
  task gates. Server-side validation/conflict behavior and real browser
  pagination remain to be confirmed there.
