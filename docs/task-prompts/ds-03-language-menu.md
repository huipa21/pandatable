# DS-03 LANG - translate-icon language menu

Read [execution plan](dashboard-sidebar.md) first. Implement only DS-03. Wave 1
**coordinator** for locales, AppIcon, progress and decisions.

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- `docs/12-frontend.md` sections 2, 6 (i18n) and 7 (theme).
- Decisions: D-153, D-158, D-234; search others by keyword `locale`.
- Acceptance: AT-003 (its "clicks 中文" wording becomes "opens the language
  menu and picks 简体中文"; propose the revision for DS-10), AT-102, AT-116,
  AT-802, AT-806.

## Scope / ownership

- Own `web/src/layouts/AppLayout.vue` (language control only), a new
  `web/src/components/account/LanguageMenu.vue` and a test file next to it,
  and `web/src/components/AppIcon.vue` (add a `language` glyph: the 文A
  translate symbol drawn in the existing stroke style). Own the language
  assertions in `web/e2e/shell.spec.ts`.
- Replace the text toggle with an icon-only button: no border, no background
  at rest, hover/focus treatment like the other header icons, visible focus
  ring. Use `aria-label`/`title` "Language", `aria-haspopup="menu"` and
  `aria-expanded`.
- Clicking it opens a menu anchored under the icon. Each row shows the
  locale's **native name** (`English`, `简体中文`) with the `lang` attribute,
  the name in the current UI language on the right, and a check on the
  active locale. Selecting a row calls `preferences.setLocale` (persists as
  today) and closes the menu. Esc, an outside click or a selection closes it
  and returns focus. Arrow keys move between items.
- Build the list from the supported-locale constant, not hard-coded markup,
  so a future locale is one entry. No search box with two locales.

## Contracts

- Keep the persisted/explicit locale semantics in `stores/preferences.ts`
  (do not edit the store). The menu works signed-out on the login/setup pages
  and stays within the viewport at 390 px.
- Wave 1 coordinator: after DS-01/DS-04/DS-05 stop, apply their locale, doc,
  decision and progress handoffs serially. Record DS-01's decision, then the
  plan decision D-234 if it is not already present. Run `pnpm --dir web i18n:check`.

## Confined verification

1. DS-03.1: the icon has no border. The menu lists en and zh-CN with native
   names, a check on the current locale and correct `lang` attributes.
2. DS-03.2: switching updates the UI and persists after reload (signed in),
   and also works signed out. Keyboard: open, arrows, Enter, Esc returns focus.
3. DS-03.3: mobile 390 px: the menu is fully visible and the header does not
   overflow.
4. Run `pnpm --dir web exec vitest run src/components/account/LanguageMenu.test.ts src/App.test.ts`,
   touched-file ESLint and `i18n:check`. Add the E2E cases; DS-10 runs them.

## Handoff / wrap-up

Record the changed files, counts, integrated peer handoffs and release of
`AppLayout.vue`/`AppIcon.vue` to wave 2. Leave it **implemented, awaiting DS-10 gates**.

## Record

Implemented the supported-locale-driven header menu with a borderless 文A icon,
native and current-UI names, active-locale check, persisted selection, arrow
navigation, Escape/outside dismissal and focus restoration. The menu remains in
the unauthenticated shell and is right-anchored within narrow viewports.

Changed `LanguageMenu.vue` and its unit test, `AppIcon.vue`, `AppLayout.vue`,
`style.css`, `i18n/index.ts`, both locale files, `App.test.ts` and the language
cases and shared locale helper in `shell.spec.ts`/`app-shell.ts`; updated this
record and `progress.md`. Focused Vitest
(3 tests), touched-file ESLint, `i18n:check` and the production frontend build
pass. `vue-tsc` still reports two
pre-existing errors in navigation and workbook-card tests; the project type
check is therefore not green. The production-shell E2E cases and consolidated
task gates are intentionally deferred to DS-10. D-234 already records the plan;
no DS-01/04/05 handoff was available to integrate in this session. Propose the
AT-003 wording change described above during DS-10. `AppLayout.vue` and
`AppIcon.vue` are released to wave 2; no conflicting changes were found.
