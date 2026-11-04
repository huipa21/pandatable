# Task prompt - UI-FIELD-TILES Field tiles, actions, and unified editing

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`, this prompt.
- `docs/07-field-types.md`: sections 3 (System fields), 4 (Constraints), and 5 (Type change).
- `docs/12-frontend.md`: T1.14 field panel and section 4 (Grid), field/column configuration only.
- Search `docs/decisions.md` for D-124, D-126, D-130, D-148, D-158 and subsequent field-editor/type-conversion decisions.
- Search `docs/acceptance-tests.md` for AT-301 and AT-303.

## Scope

Implement the user's Fields dropdown redesign as one cohesive issue. "Title" is interpreted as the field tile/name drag handle, not the table name or a column-header drag redesign.

1. Each field is a compact, visibly distinct draggable tile: visibility checkbox, field name, type, then a trailing three-dots trigger. Long names must not push the trigger outside the tile.
2. The trigger opens a real anchored dropdown containing Edit, Delete, Move up, and Move down. Hide action contents when closed; opening the dropdown must not expand the tile into scattered/wrapped action buttons. Close on selection, outside interaction, and Escape; restore focus appropriately.
3. Edit opens the same form layout/components used by Add field, prefilled from the selected field. Preserve all supported options, defaults, required/unique settings, descriptions, validation, and field-kind-specific editors. Do not retain a visibly different, reduced edit form.
4. On primary-pointer press on the name/drag handle, show a lifted tile (elevation, outline/transform, grabbing cursor). During a real drag show a moving preview, source state, and clear insertion indicator. Clear all states on release, cancellation, drop, or unmount. Respect reduced motion. Ordinary clicks, checkbox use, and menu actions must not initiate reorder.
5. Drag and Move up/down use the same authoritative reorder path and reconcile to the saved schema. Verify whether explicit view-column order also needs reconciliation so the actual grid follows the intended field order after reload. Preserve other views and existing hidden-field positions.

## Contracts to respect

- Start with `web/src/components/fields/FieldPanel.vue`, `FieldTypePicker.vue`, their tests, and field-related rules in `web/src/style.css`. The existing `<details>` action UI already exists: fix its actual layout/closed-state behavior rather than adding a second menu.
- Reuse existing mutation/version/error handling and shared editor definitions. Changing a stored type must still use the existing dry-run, confirmation, and conversion API; a normal metadata PATCH is not a conversion.
- Remove the separate Change type action from the tile menu by making the safe conversion workflow reachable inside Edit. Keep Set display field available in the edit form or its advanced section rather than silently deleting that capability.
- Existing formula/link/lookup/rollup and system fields retain their restrictions and specialized configuration. Unsupported edits must be explicitly unavailable, not silently discarded.
- Preserve owner-only schema mutations, read-only visibility controls, system-field restrictions, search, select-all, Add field, and System fields footer.
- Search-filtered reorder must preserve fields not shown by the search. No-op/cancelled/invalid drops must not mutate. Boundary move actions are disabled. Keep keyboard/touch alternatives and announce successful reorder accessibly.
- Do not add a drag library unless existing primitives cannot meet the requirement and its cost is justified. No backend redesign or new dependencies by default.
- All visible copy and accessible names use en/zh-CN keys; this task owns only `field.*` additions/changes.

## Parallel ownership

- Own field components, field-specific CSS, `FieldPanel.test.ts`, and a new `web/e2e/fields-menu.spec.ts`. Do not modify Visibility or Highlight implementation.
- Other sessions may own unrelated dirty changes. Never revert, format wholesale, or commit their work. Re-read shared JSON/docs immediately before surgical edits.
- Shared locale files, documentation, and any unavoidable Grid integration edits require serialized writes. Do not run gates, builds, or commits concurrently in one checkout; use separate working copies for fully independent parallel runs. Repository master-only policy still applies; do not create branches/worktrees.

## Verification (D-158)

- Focused loop: `pnpm --dir web exec vitest run src/components/fields/FieldPanel.test.ts` plus any directly changed editor/column tests.
- Browser assertions: closed menus contain no visible actions; exactly the intended dropdown opens; editing is prefilled and uses Add's layout; safe conversion remains accessible; deletion confirms; boundary moves, actual pointer drag, press/release/cancel feedback, search, read-only/system fields, keyboard use, and reload persistence work.
- Include narrow viewport, light/dark, en/zh-CN, reduced-motion, and checkbox/menu-without-drag coverage. Assert actual order, not merely a successful response.
- Task gate, once shared-checkout work is stable: `go run .\tools\ptdev check`; `go run .\tools\ptdev e2e --fast -- e2e/fields-menu.spec.ts e2e/fields-acceptance.spec.ts e2e/grid-columns-keyboard.spec.ts`. Preflight Podman/mirrors as instructed by the repository. Do not run the full production-image suite per task.

## Wrap-up

- Update related field UX documentation and AT-301/AT-303 evidence only for verified coverage.
- Follow `docs/task-prompts/TEMPLATE.md`: progress row/change-log line, new decision only for a non-trivial choice, and a concise record appended here (about 2 KB).
- Commit only this task's changes after gates pass, using Conventional Commits and the required co-author trailer. Serialize shared-checkout commits; do not stage other sessions' changes.

## Record (2026-11-04)

**Outcome:** Done. The Fields dropdown shows compact tiles: the name is the drag handle (press/drag/drop feedback, reduced-motion aware), the checkbox toggles visibility, and an anchored ⋯ menu offers Edit, Delete and Move up/down. Edit reuses the Add layout. The type picker offers only safe conversions, the display-field choice is under Advanced settings, and relation/lookup/rollup editors keep unavailable options visible. A reorder flushes pending view-column intents, recomputes the plan, PATCHes the field `{index}` only when schema order changes, and saves the active view order. Hidden, system and filtered slots and other views are untouched. The grid is `inert` while a reorder runs, and changing the table, view or user aborts it (D-215).

**Changed:** `FieldPanel.vue`, `FieldTypePicker.vue`, `fieldForm.ts`, `viewColumns.ts` (`flush`), `DataGrid.vue` (`reorderFields`, `flushColumnChanges`), `ResourcePage.vue` (reorder hooks, origin guard, newer-view preservation), `style.css`, en/zh-CN `field.panel.*`; tests `FieldPanel.test.ts`, `viewColumns.test.ts`, `ResourcePage.field-order.test.ts`, e2e `fields-menu.spec.ts` (new), `fields-acceptance.spec.ts`, `fields.ts`; docs 12-frontend, user-guide, acceptance-tests (AT-301/303), decisions (D-215), progress.

**Verification:** focused Vitest pass (FieldPanel, viewColumns, DataGrid, ResourcePage field order). `ptdev check` PASS. `ptdev e2e --fast -- e2e/fields-menu.spec.ts e2e/fields-acceptance.spec.ts e2e/grid-columns-keyboard.spec.ts`: 10 passed, 3 intentional mobile skips. Earlier gate runs failed on a misnested test and on a system-field ordering assumption in the new spec; both were test bugs and are fixed.

**Risks / next:** Pointer reorder is browser-tested on desktop only; Move up/down covers keyboard/touch. The full production-image E2E runs at phase end (D-158).