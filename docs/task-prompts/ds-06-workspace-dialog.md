# DS-06 WSFORM - workspace create/edit dialog with icon and description

Read [execution plan](dashboard-sidebar.md). Requires the DS-01 handoff. Wave 2
**coordinator** for locales, progress and decisions.

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- `docs/12-frontend.md` "T1.11 navigation"; `docs/09-api.md` "Structure".
- Decisions: D-102, D-158, D-219, D-234 and the DS-01 decision.
- Acceptance: AT-214, AT-216.

## Scope / ownership

- Own `web/src/components/navigation/StructureDialog.vue` (workspace branch),
  `NavActions.vue`, a new reusable `web/src/components/common/IconPicker.vue`
  and their unit tests.
- Create-workspace dialog fields, in this order:
  1. **Workspace name**: required, existing validation.
  2. **Icon**: optional `IconPicker` (a grid of the allowed vocabulary, a
     radio-group/listbox with arrow-key navigation, a "Random" choice selected
     by default). With "Random" no `icon` is sent, so the server's stable
     fallback applies.
  3. **Description (optional)**: textarea, 500-character counter, plain text.
- Owners get **Edit workspace** (replacing Rename for workspaces) with the same
  form prefilled. They can clear the icon (back to the fallback) or the
  description. Send only changed fields through the DS-01 store helpers.
- Show the server's field errors inline. Keep focus trap, Esc and busy states.
- Book create/rename dialogs are unchanged. Make `IconPicker` reusable for them
  later, but do not wire it.

## Contracts

- Do not edit `stores/nav.ts` (DS-02 owns it in this wave). If a helper is
  missing, record an exact handoff.
- NavActions keeps share/default/delete semantics and permissions. DS-07 reuses
  NavActions (or its menu items) for dashboard rows, so keep its API
  item-driven and free of sidebar-specific DOM assumptions.
- Wave 2 coordinator: after DS-02 stops, integrate the locale/doc/progress
  handoffs serially and run `pnpm --dir web i18n:check`.

## Confined verification

1. DS-06.1: create with name only gives a fallback icon and an empty
   description. Create with icon and description persists both.
2. DS-06.2: edit changes icon/description, and clearing restores the fallback.
   Non-owners see no Edit. Validation errors (more than 500 characters) render
   inline.
3. DS-06.3: keyboard-only completion. The picker is announced as a group with
   the selected state. 390 px layout fits.
4. Run `vitest run` on the dialog/picker/NavActions tests, touched-file ESLint,
   `typecheck` and `i18n:check`. Add the E2E case to `workspace-navigation.spec.ts`
   create flow (or a new `workspace-dialog.spec.ts`); DS-10 runs it.

## Handoff / wrap-up

Record the component APIs for DS-07 (how to open create/edit), counts and
integrated handoffs. Leave it **implemented, awaiting DS-10 gates**.

## Record (2026-11-07)

- Implemented workspace create/edit fields in `StructureDialog.vue`; random
  sends no icon, editing submits only changed icon/description details, and
  oversized descriptions show an inline counter error. Added reusable
  `IconPicker.vue` with all server-allowed icons and roving arrow-key selection.
- `NavActions.vue` now exposes owner-only **Edit workspace** while preserving
  other resource rename and sharing/default/delete behavior. DS-07 opens create
  with `{ operation: 'create', kind: 'workspace' }` and edit with
  `{ operation: 'rename', kind: 'workspace', id, name }`. `IconPicker` exposes
  `v-model` as `BookIconName | null`. The existing `nav.submit`
  `WorkspaceDetails` helper was sufficient; `stores/nav.ts` was not changed.
- Added unit coverage for permissions, keyboard selection, changed/cleared
  metadata and inline validation. `workspace-navigation.spec.ts` covers
  name-only/custom creation, metadata editing and clearing on desktop/mobile.
- Checks: focused Vitest 12/12; touched-file ESLint; typecheck; i18n check
  (1,301 matching keys); Prettier; production frontend build; Playwright spec
  discovery — all pass. Browser E2E and consolidated gates remain deferred to
  DS-10; implementation awaits those gates.
- Wave 2 coordination: integrated the completed DS-02 progress handoff; its API
  documentation and acceptance mapping remain with DS-10.
