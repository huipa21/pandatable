# DS-02 RECENT - recently opened workspaces/workbooks and shared search

Read [execution plan](dashboard-sidebar.md). Requires the DS-01 handoff. Wave 2.

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- `docs/05-data-model.md` meta schema; `docs/09-api.md` "Structure" and
  "UX-01 workspace preference" (pattern for per-user rows).
- Decisions: D-158, D-218, D-234 and the DS-01 decision.
- Acceptance: AT-214, AT-215, AT-216.

## Scope / ownership

- Backend: own `migrations/0010_recent_resources.sql`, the structure package
  queries/sqlc/nav code and the new handler.
  - Table `meta.recent_resources(user_id, workspace_id NULL, book_id NULL,
    opened_at)`. Exactly one target is set (CHECK). FKs use `ON DELETE
    CASCADE`. Use a unique index per (user, target).
  - `POST /recent {kind:'workspace'|'book', id}` → 204. Upsert `opened_at =
    now()`. Require effective read or path-only reachability through the same
    ACL lookup as `/nav`. Return a uniform 404 for hidden, trashed or missing
    targets. Prune each user's rows to the newest 200 in the same transaction.
  - `/nav` adds `lastOpenedAt: string|null` per workspace and book for the
    caller only (LEFT JOIN). Trashed resources never appear.
- Frontend:
  - Own `stores/nav.ts` (`lastOpenedAt` mapping and a `recordOpen(kind, id)`
    action that updates the local value optimistically, sends the POST
    without blocking and ignores 404).
  - Add a new `web/src/composables/useRecentOpen.ts`, wired once in
    `AppLayout.vue`. When a workspace or workbook route resolves (including
    table/view/row routes, which also record their workbook and workspace),
    call `recordOpen` at most once per route identity.
  - Add a new `web/src/composables/useResourceSearch.ts` with unit tests:
    - input: items with `{name, description?, lastOpenedAt, index}`, a query
      ref and an optional pinned ID (the preferred workspace).
    - no query: opened items by `lastOpenedAt` desc, then the pinned ID, then
      server `index`.
    - query: NFKC + case-folded match on name/description. Rank name prefix >
      name contains > description contains; ties use the no-query order.
    - return plain ordered items (no match highlighting); DS-07/DS-08 render them.

## Contracts

- Recording never changes permissions, events or the realtime nav refresh
  loop. Do not emit a nav-changed event for recents.
- Users never learn other users' recents. Deleting a user or resource cascades.
- Must work with stale nav (errors are ignored and retried on the next route).

## Confined verification

1. DS-02.1 (PG18): upsert/order, 200-row pruning, hidden/trashed → 404 with no
   row, cascade on delete, per-user isolation, `/nav` `lastOpenedAt`.
2. DS-02.2 (Vitest): composable ordering with/without a query, CJK and
   accented names, description match, empty query whitespace, pinned fallback.
3. DS-02.3: route changes record once. Rapid route flips do not double-post;
   a 404 is silent.
4. Run `go build ./...`, then focused `go test -run 'TestRecent|TestNav' ./internal/structure -count=1`
   (PG18). Run `vitest run` on the composable/store/hook tests, touched-file
   ESLint and `pnpm --dir web typecheck`.

## Handoff / wrap-up

Record the API/DTO shape, composable signature and counts. Put the API doc
patch in the Record for DS-10. Leave it **implemented, awaiting DS-10 gates**.

## Record
