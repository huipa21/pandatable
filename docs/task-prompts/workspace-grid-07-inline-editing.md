# UX-07 - Single-click scalar editing and commit on leave

No prerequisite. Can run alongside UX-01. Stop editing before UX-08 starts.

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- [Execution index](workspace-grid-redesign.md).
- `docs/12-frontend.md` section 4; `docs/07-field-types.md` validation/codec
  contracts; `docs/09-api.md` Rows & queries.
- Search decisions D-138, D-139, D-215, D-217.
- Search acceptance AT-204, AT-301, AT-302, AT-305, AT-306, AT-309, AT-320.

## Scope

Implement the grid editor activation/commit lifecycle for text and scalar types.
Checkbox/select/tag-specific controls belong to UX-08; row addition to UX-09.

- A normal single click on an editable data cell focuses/activates its editor.
  Double-click and existing keyboard entry still work.
- Valid edits persist when focus leaves the editor, without needing Enter.
  Enter/Tab commit once; Escape cancels. A click into another cell commits the
  previous draft before editing the new cell.
- Use registry parsing/normalization and the current authoritative PATCH/undo/
  realtime pipeline. Do not directly mutate renderer data to simulate saving.
- Invalid drafts remain visible with translated validation; rejected saves
  cannot look persisted. Keep a recoverable draft/error rather than silently
  dropping the value on outside click.

## Ownership and lifecycle contract

Own DataGrid/GridCellEditor/GridCellContent activation glue, grid-specific tests
and the minimum FieldEditor extension needed. Do not change backend API/types,
navigation, ResourcePage, row-plus layout or global styles.
The existing FieldEditor is shared by forms, row modals and mobile. Introduce
an explicit grid-only opt-in for commit-on-leave if needed; retain explicit
Save semantics elsewhere. Do not globally turn every form blur into a write.

Use the installed RevoGrid API/events, including its type declarations, rather
than guessed DOM event dispatch. Handle focus moving inside the editor/popover
as internal, not an outside click. Preserve multi-line input, IME composition,
date/time precision, decimal/bigint strings and computed/system read-only fields.
Clicks/dragging on utility columns, range selection, links, group headers and
relation/attachment controls must retain their intended behavior.
Guard route/user/unmount changes and prevent duplicate or out-of-order commits
to the same cell. Do not unmount an unsaved draft before a commit is captured.

## Acceptance scenarios

1. Single-click text, type, click another cell: one PATCH, value remains and
   survives reload. Double-click editing also persists. Repeat for typed scalar
   fields with exact wire values (including bigint/decimal).
2. Enter/Tab plus the resulting blur does not save twice; Escape does not save.
   Chinese IME composition and multiline editing do not commit prematurely.
3. Invalid integer/email and required/unique/server failures remain explicit;
   no false saved state. Slow successive saves cannot overwrite a later value.
4. Reader and computed/system cells never enter a writable editor; permission
   revocation and route/identity changes cannot write to a new table/user.
5. Keyboard navigation, range drag/copy/paste/fill, row expansion and link/relation
   actions remain usable. Row-modal/form/mobile Save behavior is unchanged.
6. Editing does not mount controls for the entire table or break bounded
   virtualization; undo/redo and realtime still observe successful writes.

## Verification and wrap-up

Focused `DataGrid.test.ts`, field editor tests and new editor-lifecycle tests;
add `web/e2e/grid-inline-editing.spec.ts` with real pointer/keyboard/blur actions
and reload/API persistence assertions. Record the activation and editor-boundary
contract so UX-08 extends it without a second event pipeline.
Task gates and central docs follow the execution index.

## Record

### 2026-11-05 - Started

Started the grid-only editor lifecycle alongside UX-01; central docs, shared
gates and commits remain coordinator-owned.

### 2026-11-05 - Implement/stabilize handoff

**Implemented; source writes stopped; awaiting the coordinator's shared
`ptdev check` and central-doc integration.** UX-08 should extend the existing
editor boundary, not introduce another save pipeline. Changed DataGrid,
GridCellEditor, GridCellContent, the scalar predicate, grid-only FieldEditor
opt-in, focused tests and the new inline-editing browser spec. The row-expand
browser test now leaves the active scalar editor before using Space. No
locale keys, dependencies or backend contracts were changed.

Proposed decision/design addition: single-click scalar activation uses
RevoGrid's `beforecellfocus` mouse event and a primary release without
modifiers or a drag over five pixels, then `setCellEdit` using row identity.
Interactive and non-scalar controls retain their own behavior. Wrap the Vue
adapter's `beforeDisconnect` with one leave boundary, handling internal focus
and IME; retain recoverable drafts/errors, serialize cell PATCHes and guard
table/view/user/schema/permission changes. Defer authoritative local rows
without clearing the window; reconcile overlapping collaborator changes and
queue a fresh reload if updates arrive during window loading.

Verification:

- Six focused grid/editor/window/realtime/field Vitest files: **442 passed**.
  Frontend type-check, targeted ESLint and Prettier checks: **passed**.
- `ptdev e2e --fast` selected desktop scenarios in inline-editing,
  fields-acceptance, fields-batch2, grid-columns-keyboard, realtime,
  row-expand and undo: **8 passed**, including setup. This resolved the six
  previously failing browser scenarios.
- After the final loading guard, `ptdev e2e --fast -- grid-inline-editing.spec.ts
  realtime.spec.ts --grep <inline and realtime scenarios>`: **4 passed,
  3 intentional desktop/mobile skips**, including setup.

Coordinator handoff: run the serialized `go run .\tools\ptdev check` on this
final source state; integrate the proposed decision and progress/design
evidence for AT-204, AT-301, AT-302, AT-305, AT-306, AT-309 and AT-320.
Preserve the uncommitted UX-01/planning changes and avoid mixed commits. The
full production-image gate remains reserved for UX-11; physical IME behavior
has not been claimed as tested.
