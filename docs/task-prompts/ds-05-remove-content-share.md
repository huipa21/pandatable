# DS-05 NOSHARE - remove content-area Share button

Read [execution plan](dashboard-sidebar.md) first. Implement only DS-05. Wave 1.

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- `docs/12-frontend.md` "T1.12 sharing".
- Decisions: D-108..D-112, D-158, D-234.
- Acceptance: AT-204..AT-210 (grants/sharing), AT-216.

## Scope / ownership

- Own `web/src/pages/ResourcePage.vue` (only the Share button and its
  now-unused `canShare`/`sharing` wiring), `ResourcePage.test.ts` and
  `ResourcePage.workbook-cards.test.ts` share assertions, and the E2E helpers
  that click it: `web/e2e/sharing.ts`, `hardening.spec.ts`,
  `workbook-cards.spec.ts` (share steps only).
- Remove the `Share` button rendered under workspace and workbook headings
  (`v-if="canShare && kind !== 'table'"`). Do not touch the table toolbar
  visibility menu, NavActions' Share item or `ShareDialog`.
- Re-route the tests to the remaining entry points:
  - workbook and table sharing: open the sidebar resource action menu → Share.
  - workspace sharing: the sidebar workspace action menu → Share (still
    present in wave 1; DS-07 re-points this helper to the dashboard row menu
    in wave 3).
  - Put the helper's target lookup in one function so DS-07 changes one place.

## Contracts

- Sharing permissions are unchanged. A user who could share before can still
  reach Share for every resource kind through a menu, including on mobile
  (drawer menu).
- Do not change ResourcePage layout beyond removing the button. DS-08 takes
  ResourcePage in wave 3.

## Confined verification

1. DS-05.1: workspace and workbook pages render no Share button for owners.
   The heading, create and import controls are unchanged.
2. DS-05.2: an owner can still open the Share dialog for workspace, workbook
   and table through menus. Readers cannot.
3. Run `pnpm --dir web exec vitest run src/pages/ResourcePage.test.ts src/pages/ResourcePage.workbook-cards.test.ts src/components/sharing/sharing.test.ts`,
   touched-file ESLint and `pnpm --dir web typecheck`. Do not run the E2E;
   DS-10 does.

## Handoff / wrap-up

Record the changed files, counts and the helper function name for DS-07. Release
ResourcePage to DS-08. Leave it **implemented, awaiting DS-10 gates**.

## Record

- Implemented, awaiting DS-10 gates; released `ResourcePage.vue` to DS-08.
- Changed: `ResourcePage.vue`, `ResourcePage.workbook-cards.test.ts`,
  `e2e/sharing.ts`, `e2e/hardening.spec.ts`, `e2e/workbook-cards.spec.ts`,
  `docs/progress.md`, and this record. The page-level Share button and its
  sharing-store wiring are removed; `canShare` remains for the existing table
  Visibility menus.
- `shareAction` in `e2e/sharing.ts` centralizes sidebar target lookup; DS-07
  should reroute its workspace branch here.
- Vitest: 3 files, 42 tests passed. Touched-file ESLint, Prettier and web
  typecheck passed. E2E was intentionally deferred to DS-10.
