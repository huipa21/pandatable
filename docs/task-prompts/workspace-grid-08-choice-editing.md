# UX-08 - Direct checkbox, select and tag editing

Requires UX-07. Can run alongside UX-02 and UX-03.

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- [Execution index](workspace-grid-redesign.md), UX-07's lifecycle handoff.
- `docs/07-field-types.md` checkbox, single/multi select and list types;
  `docs/12-frontend.md` section 4.
- Search decisions D-139, D-217.
- Search acceptance AT-204, AT-301, AT-302, AT-305, AT-320, AT-324.

## Scope

Specialize UX-07's grid interaction for existing boolean and choice/tag types.

- One click toggles a writable checkbox on/off, without a prior double click
  or a second click just to activate the editor.
- One click on a single-select cell opens its choices; picking an option
  commits through the ordinary mutation path.
- One click on multi-select/tag-style cells opens options; add/remove multiple
  values without closing after each item. Commit valid selection on leaving
  the whole editor; Escape cancels the draft.
- Reuse existing option definitions, labels/colors, validation and stable
  choice IDs. "Tags" means the existing choice/list presentations, not a new
  backend field type or auto-creating global choices.

## Ownership and boundaries

Own the grid's choice/checkbox renderer-editor glue and necessary FieldEditor
grid-only behavior, plus focused tests. UX-02 owns navigation and UX-03 owns
API types/schema; do not edit either. No header/ResourcePage/row-add work.
Reuse UX-07's activation, commit and request-error handling.

Inspect registered type presentations before mapping "tags": existing
multi-select fields should show their configured choices. For free-text list
types, preserve supported entry semantics and show existing suggestions only
where an authoritative source exists; do not invent a global tag catalog.
Document the exact existing types covered.

Popover focus/option clicks must not trigger an early blur commit. Checkbox
clicks must not toggle twice through both renderer and editor handlers.
Support keyboard/touch and retain read-only/nullable behavior. Do not change
row-modal/form multi-select Save semantics. Keep user/relation/attachment
pickers usable through UX-07's lifecycle without expanding this task into
a picker redesign.

## Acceptance scenarios

1. A single physical click on a checkbox toggles once; second click toggles off.
   Each persists after reload, participates in undo and shows failures honestly.
2. A single click shows single-select options; selection and supported clearing
   persist, with labels displayed but IDs sent to the server.
3. Multi-select/tag options open with one click; several additions/removals stay
   in the same editor. Outside click commits once; internal clicks do not close;
   Escape cancels. Reload retains exact selections.
4. Existing unavailable/renamed choices and null values are not silently replaced.
   Free-text list inputs retain their original codec rules.
5. Read-only cells cannot mutate via pointer or keyboard; failure/revocation/
   identity guards and range selection still work.
6. Keyboard option navigation, focus restoration, both languages/themes and
   unchanged row-modal/form behavior are covered.

## Verification and wrap-up

Focused grid/editor/field tests; extend `grid-inline-editing.spec.ts` from UX-07
with actual one-click assertions and API/reload verification.
Gates/docs/commits follow the execution index. Hand off editor sources to UX-09.

## Record

Status: In progress. Gap coverage was added (see the follow-up below) and
awaits a coordinated gate re-run. Commit and production-image phase gate are
deferred to UX-11. Central docs are coordinator-owned.

- Covered: `checkbox` toggles once per primary click on its box (a click
  elsewhere in the cell only selects; Space toggles). `single_select` and
  `multi_select` open a grid-only listbox (`GridChoiceEditor.vue`) on one click.
  Single pick and None commit at once. Multi stays open, commits once on
  leave/Enter and cancels on Escape. Unconfigured stored labels stay visible and
  are never silently replaced. `text_array`/`int_array` keep UX-07's codec
  editor. User, relation and attachment pickers are unchanged.
- Writes reuse UX-07: `commitCell` (former `edit`) gives the serialized PATCH,
  undo, drafts, guards and focus. `toggleCell` flips the newest intent, so rapid
  toggles serialize. The popover is viewport-fixed and flips above bottom rows.
- Correction: the wire value is the exact label in choice order (07 codec), not
  the choice ID as scenario 2 says.
- Fixed `CellCommit.val` optional (`BeforeSaveDataDetails`), DataGrid.vue typecheck.
- Files: `web/src/components/grid/{DataGrid,GridCellEditor,GridCellContent,
  GridChoiceEditor}.vue`, `inlineEditing.ts`, `DataGrid.test.ts`,
  `GridCellEditor.test.ts`, `web/e2e/grid-inline-editing.spec.ts`.
- Evidence: typecheck, eslint and prettier clean; vitest 815 passed (full web suite);
  `ptdev e2e --fast -- e2e/grid-inline-editing.spec.ts` 3 passed, 2 mobile skipped.
  Coordinated gate (after the login URL moved to `/w/<id>`): `ptdev check` passed
  (816 frontend tests); combined `ptdev e2e --fast` batch 12 passed, 3 intentional
  mobile skips (both UX-08/UX-07 grid scenarios are desktop-only).
- Proposed decision (coordinator): one-click choice/checkbox editing is grid-only;
  F2/Enter on a checkbox still opens the codec editor; no new locale keys.
- Gap follow-up (after the coordinator kept UX-08 In progress): added
  DataGrid unit tests for checkbox undo (one guarded `patch` step,
  `false`→`true` expected) and a rejected toggle (403 error shown, no undo step,
  stored value kept so the next toggle again requests `true`). Extended the
  browser spec with a zh-CN/dark pass: None option is `zh.field.editor.none`,
  and the popover has a dark surface and light text. Locale and theme are
  restored afterwards. Focused checks: vitest grid+fields 194 passed;
  typecheck/eslint/prettier clean; `ptdev e2e --fast --
  e2e/grid-inline-editing.spec.ts` 3 passed, 2 mobile skipped. Needs a
  coordinated gate re-run. Physical touch is untested.
  Handoff: editor sources to UX-09.
- 2026-11-06 (UX-11): gap coverage passed in the coordinated `ptdev check`
  (DataGrid undo/rejected-toggle tests), the fast grid batch and both
  production-image runs (zh-CN/dark choice scenario, AT-329). The later
  production run failed the linked AT-305/311 PageDown regression; this task
  remains open. See the [UX-11 record](workspace-grid-11-integration.md#record).
