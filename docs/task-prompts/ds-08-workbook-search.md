# DS-08 BOOKS - workbook search and recency on the workspace page

Read [execution plan](dashboard-sidebar.md). Requires DS-02 and DS-05 handoffs.
Wave 3.

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- `docs/12-frontend.md` section 2.
- Decisions: D-219, D-230, D-234 and the DS-02 decision.
- Acceptance: AT-216, AT-208.

## Scope / ownership

- Own `web/src/pages/ResourcePage.vue` (workspace branch only),
  `ResourcePage.workbook-cards.test.ts` and `web/e2e/workbook-cards.spec.ts`.
- On `/w/{id}`, add the same full-width search bar above the existing
  workbook card grid (reuse the DS-07 markup/style pattern; extract a small
  shared `ResourceSearchInput.vue` only if DS-07 has not, and record the
  ownership in your Record). Feed the cards from `useResourceSearch` over the
  workspace's visible books (name only; books have no description).
- No query: recently opened workbooks first, then server order. Query: ranked
  matches, a no-match state with clear-search, and the existing empty state
  when the workspace has no books. Keep "New book" and import controls.
- Clear the query when switching workspaces, and keep it in the URL query
  (`?q=`) so back/refresh restore it (and also on the dashboard if DS-07
  agrees; record the choice).

## Contracts

- Card visuals, icons and path-only labels stay as in AT-216. Do not
  reintroduce the Share button. Do not touch the table branch, grid or toolbar.
- Hidden books never appear. Stale/retry states are unchanged.

## Confined verification

1. DS-08.1: no query gives recency order; opening a workbook moves it to the
   top after returning.
2. DS-08.2: search filters cards (incl. CJK), and no-match/clear work. Switching
   workspaces resets the query; refresh restores `?q=`.
3. DS-08.3: keyboard reaches the search input, then the cards. 390 px layout fits.
4. Run `vitest run src/pages/ResourcePage.workbook-cards.test.ts src/pages/ResourcePage.test.ts`,
   touched-file ESLint and `typecheck`. Write the E2E cases; DS-10 runs them.

## Handoff / wrap-up

Record the changed files, counts and locale handoffs to DS-07. Leave it
**implemented, awaiting DS-10 gates**.

## Record

- Implemented, awaiting DS-10 gates. Added URL-backed workbook search on
  workspace pages using `useResourceSearch`; the cards retain server order for
  equal/absent recency, path-only metadata, and the original empty/stale states.
  Search resets when the destination workspace has no `q`, while browser back
  restores the prior workspace query. Added reusable `ResourceSearchInput.vue`
  for DS-07 to adopt; DS-08 owns the component.
- Changed: `ResourcePage.vue`, new `components/ResourceSearchInput.vue`,
  `ResourcePage.workbook-cards.test.ts`, `e2e/workbook-cards.spec.ts`, and this
  record. Unit coverage includes URL sync, CJK matching, clear/no-match,
  recency and workspace navigation. E2E covers open-to-top recency, CJK search,
  no-match/clear, refresh/back restoration, keyboard order, and 390 px layout.
- DS-07 integrated the four `dashboard.ds08.*` keys in both locales and adopted
  the shared `ResourceSearchInput.vue`. The dashboard query remains transient
  while the workspace-page query is URL-backed (D-235).
- Verification: focused Vitest passed (2 files, 39 tests). ESLint passed for
  the component, page, tests and E2E spec after the locale handoff; `typecheck`
  and i18n checks pass. E2E and consolidated/phase gates remain with DS-10.
