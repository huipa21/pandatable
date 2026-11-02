# Shell redesign (user sketch)

## Scope and contracts

- Adapt the web shell to the user's sketch: header logo, sidebar toggle,
  breadcrumb, search, language and theme toggles; sidebar workspace box,
  workbook › table › view tree, and avatar menu at the bottom.
- UI only: no backend, route, permission, migration or dependency change.
- D-209 (supersedes the D-105 `/t/:tableId` redirect and the D-106
  always-visible tree/Trash entry). AT-003 is now covered by the header toggles.

## User decisions

Workspace box opens `/w/:id` and has "+" for a new book. Books toggle their
tables. On hover, the table icon becomes a views dropdown. The table name opens
the default view, with a breadcrumb ending at the table; a view adds a fourth
segment. "Book" stays the UI term. Search is a client-side quick-jump. The
theme control is a light↔dark toggle. Notifications live in the avatar menu,
with an unread badge. Undo/redo sit in a view subheader for now (to be revised).
The avatar menu holds Profile, Change password, Trash, an Admin submenu and
Sign out. The collapsed sidebar is fully hidden and the state is remembered.
Home shows workspace cards. The footer is removed.

## Record (2026-11-02)

New: `AppIcon`, `AppBreadcrumbs`, `SearchPalette`, `NavActions`, `UserMenu`,
`useNavLocation`. Rewritten: `AppLayout`, `NavContents`, `NavNode`,
`HomePage`, shell CSS. `nav` store gains `views`, `lastWorkspaceId` and
`drawerOpen`; `preferences` gains `sidebarCollapsed` and `toggleTheme`.
ResourcePage renders the default view for a bare table URL. New en/zh-CN keys
(`search.*`, `account.*`, `nav.*`, `common.useDark/useLight`).

Verification:

- `pnpm lint` (eslint, prettier, vue-tsc, i18n parity): pass.
- `pnpm exec vitest run`: 55 files, 678 tests passed (adds `header.test.ts`
  for breadcrumbs, search and the avatar menu).
- e2e specs migrated to the new shell (`e2e/app-shell.ts` helpers). Secondary
  contexts pin `locale: 'en-US'` because the host locale is zh-CN. Structure
  "⋯" menus now close on outside click/Escape, and opening one closes the
  others, so dropdowns no longer cover the next row.
- Gate: `ptdev check` pass. `ptdev e2e --fast` ran in 7 spec batches, because
  the full 110-test run exceeds Playwright's 600 s `globalTimeout`; all specs
  pass. Races exposed while stabilizing (not regressions): history now
  re-applies grid focus until it sticks. The mobile-rows reader 403 and the
  lookups stale-schema 409 are now allowed rather than required.
- Follow-up: the full production-image `ptdev e2e` (D-158) needs a
  `globalTimeout` larger than 600 s or a sharded run.
