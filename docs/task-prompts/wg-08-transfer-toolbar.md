# WG-08 XFER - unified Import/Export icon

Read [execution plan](workspace-grid-refinement.md). No prerequisites.

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- `docs/12-frontend.md`: table toolbar and import/export controls only.
- `docs/11-import-export-backup.md`: import/export UI contracts only.
- Decisions: D-158, D-226, D-229, D-230.
- Acceptance: AT-327; search AT-701..705 for existing transfer regressions.

## Scope / ownership

- Own `web/src/components/navigation/TableToolbarActions.vue` and a new focused
  `transferToolbar.test.ts` beside it. Own focused assertions in
  `web/e2e/export.spec.ts`; do not rewrite import/export backend tests.
- Remove the visible display-field/access summary in all table/view toolbar
  variants. Keep schema, access checks and presence. Remove dead imports and
  component-local styles; avoid unrelated global cleanup.
- One header icon trigger opens Import, Export CSV and Export workbook actions
  according to existing permission/view/query readiness. Use an existing local
  icon; an accessible localized name/title is required even if text is hidden.
- Reuse emitted import/export events and caller workflows; do not change the
  public prop/event contract during wave 1, even if a now-unused compatibility
  prop remains for WG-09 to clean up with callers.
- Import remains write/owner only. Export options retain current view and
  csvReady restrictions, pending disabled state, package notice and feedback.
  Close on action, outside click, Escape and route/identity change, with correct
  focus behavior. Feedback/errors remain visible after the menu closes.
- Do not edit ResourcePage, DataGrid, AppIcon, global styles, API or shared
  header tests. Hand off any icon/style/caller change exactly to WG-01.

## Confined verification

1. WG-08.1: exactly one transfer trigger; no display-field/access copy or
   separate Import/Export triggers on desktop, mobile or non-grid variants.
2. WG-08.2: writer import and allowed CSV/workbook export emit the same events;
   readers cannot import; disabled/loading and error states remain correct.
3. WG-08.3: keyboard focus, Escape/outside dismissal, route/identity changes,
   translated name and presence remain; menu feedback is not lost.
4. Run `pnpm --dir web exec vitest run src/components/navigation/transferToolbar.test.ts`
   and touched-file ESLint. Add focused browser assertions for WG-09; do not
   run complete header/export suites, `ptdev check`, E2E or image builds.

## Handoff / wrap-up

Record test counts and precise proposed locale/docs/caller patches for WG-01
and WG-09. Leave **implemented, awaiting WG-09 gates**, no commit.

## Record

Implemented, awaiting WG-09 gates.

- Added `transfer.title` to both locale catalogs (`en`: “Import or
  export”; `zh-CN`: “导入或导出”).
- `TableToolbarActions` now owns one accessible `duplicate` icon trigger;
  import remains write/owner-only, CSV remains grid/query-ready-only, and
  XLSX remains available wherever a view exists. Existing `import` and
  `export` events/callers remain unchanged; no caller patch is proposed.
- Added `transferToolbar.test.ts` (2 tests, 2 passing) and one focused
  `export.spec.ts` assertion block for WG-09. Typecheck, i18n check and
  touched-file ESLint pass. Full E2E and `ptdev` gates remain with WG-09.
