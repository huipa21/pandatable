# WG-01 FLAT - flat workspace browsing

Read [execution plan](workspace-grid-refinement.md) first. Implement only WG-01.

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- `docs/12-frontend.md` section 2 and "T1.11 navigation".
- Decisions by ID: D-102, D-131, D-158, D-217, D-219, D-230.
- Acceptance by ID: AT-214..216, AT-327.

## Scope / ownership

- Own `web/src/components/navigation/NavContents.vue`, a new focused
  `flatWorkspaces.test.ts` beside it, and
  `web/src/pages/ResourcePage.workbook-cards.test.ts`.
- Reuse existing cards/styles in ResourcePage; modify ResourcePage only if
  needed to satisfy card navigation/empty/error cases, not grid/header behavior.
- Own focused updates to `web/e2e/workspace-navigation.spec.ts` and
  `web/e2e/workbook-cards.spec.ts`. WG-02 takes navigation files after handoff.
- Remove per-workspace expansion, nested trees, book counts and book-empty
  messages from workspace browsing. Keep every authorized workspace in server
  order, current/default markers and permitted workspace actions.
- Selecting a workspace must show only its visible workbook cards; clicking a
  card opens the existing workbook route. Reuse stable icons and creation/share
  controls. Do not redesign cards already meeting the reference.
- Hand off a clear contextual-tree insertion point to WG-02. Do not implement
  the workbook tree here or remove NavNode from the repository.

## Contracts

- Preserve explicit links, default landing, path-only ancestry, owner controls,
  keyboard/mobile navigation, stale/retry states and inaccessible-resource UX.
- Do not fetch an admin workspace inventory or expose hidden workbooks.
- Do not change nav/API models, DataGrid, field capabilities or transfer toolbar.
- Coordinate wave 1 shared locale/docs updates only after peer writers stop;
  follow the execution plan's shared-worktree protocol.

## Confined verification

1. WG-01.1: several workspaces appear once in server order, with no descendant
   book/table/view DOM in workspace-browsing mode; selection/default differ.
2. WG-01.2: empty, one-book and many-book workspaces render correct cards;
   switching never leaks the previous list; hidden books stay absent.
3. WG-01.3: keyboard card activation reaches the workbook route; direct links,
   mobile drawer dismissal, stale/retry and read/path-only controls survive.
4. Run `pnpm --dir web exec vitest run src/components/navigation/flatWorkspaces.test.ts src/pages/ResourcePage.workbook-cards.test.ts`.
   Run touched-file ESLint from `web`. Add focused E2E assertions; do not run
   broad tests, `ptdev check` or image builds. Execution belongs to WG-09.

## Handoff / wrap-up

Update own Record with changed files, counts and WG-02 insertion contract.
As wave coordinator, serially integrate peer translations in en/zh-CN, central
progress and decision/design additions; run `pnpm --dir web i18n:check`.
Leave status **implemented, awaiting WG-09 gates**, no commit.

## Record

Implemented; awaiting WG-09 gates (2026-11-06).

- Changed `NavContents.vue` to show only ordered workspace entries, preserving
  route/default markers and permitted workspace actions. Added
  `flatWorkspaces.test.ts`; updated the workspace-navigation and workbook-cards
  E2E specs, `docs/12-frontend.md` and `docs/progress.md`. Existing ResourcePage
  cards, empty/stale/retry states and card tests already met the route contract
  and needed no code changes.
- Focused Vitest: 2 files, 5 tests passed. Touched-file ESLint, Prettier and
  i18n parity passed. E2E was not run, per the refinement plan; acceptance and
  full gates remain WG-09 work.
- WG-02 insertion contract: put the active-workbook-only tree outside
  `.workspace-root`; mount it only for workbook/table/view/row context. Keep the
  flat entries, route/default markers and mobile navigation handlers unchanged.
- Handoff: `navigation.test.ts` still asserts workspace expansion and nested
  descendants in `NavContents`. WG-02 should replace those assertions while
  adding contextual-tree coverage; `NavNode.vue` remains available for reuse.
