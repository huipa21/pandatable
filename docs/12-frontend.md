# 12 — Frontend

## 1. Stack

Vue 3 (Composition API, `<script setup>`), TypeScript (strict), Vite, Pinia, Vue Router, Naive UI, RevoGrid (`@revolist/vue3-datagrid`), vue-i18n, VueUse, dayjs. No CSS framework beyond Naive UI theming + small utility CSS. Target bundle: < 600 KB gzip initial (grid lazy-loaded on table route).

## 2. Layout

```
┌────────────────────────────────────────────────────────────────────┐
│ ☰ PandaTable   Workspace ▸ Book ▸ Table      🔔  [EN|中文] [☀|🌙] 👤 │  ← header (language + theme toggles side by side, all users)
├──────────────┬─────────────────────────────────────────────────────┤
│ Sidebar      │ View tabs:  Grid ▾ | My view | + View                │
│  Workspaces  │ Toolbar: Fields · Filter · Sort · Group · Highlight │
│   └ Books    │          · Row height · Search · Export · Share(👥) │
│      └ Tables│ ┌─────────────────────────────────────────────────┐ │
│              │ │ Grid / Form / Kanban …                          │ │
│ Trash        │ │                                                 │ │
│ Admin ⚙ (admin only)                                             │ │
│              │ └─────────────────────────────────────────────────┘ │
│              │ Footer: aggregates · row count · presence avatars    │
└──────────────┴─────────────────────────────────────────────────────┘
```

Routes:
- `/setup`, `/login`, `/change-password`
- `/w/:workspaceId`, `/w/:wid/b/:bookId`, `/t/:tableId/v/:viewId`, `/t/:tableId/v/:viewId/r/:rowId` (row modal deep link)
- `/admin/{dashboard,users,groups,workspaces,settings,backups}` (guarded by role)

## 3. Stores (Pinia)

| Store | State |
|---|---|
| `auth` | me, prefs (locale, theme) |
| `nav` | sidebar tree + effective levels |
| `schema` | per table: fields, relations, views, schemaVersion |
| `rows` | per view: page cache (Map offset→rows), total, group tree, stale flags |
| `realtime` | ws connection, subscriptions, presence |
| `undo` | per-tab action stack |
| `notifications` | unread list/count |

T0.5 implements only a separate `preferences` store for unauthenticated display
preferences (`pandatable.locale`, `pandatable.theme` in localStorage). The auth
store/server preference layer remains future work. `/login` is a welcome/status
surface, not a sign-in form; unknown routes have a localized not-found page.

## 4. Grid (desktop/tablet ≥ 768 px)

Our own `DataGrid` component wraps RevoGrid so the grid engine is swappable (D-005).

- Virtual rows & columns; windowed data fetch (page 100) with placeholders.
- Frozen columns, resize, reorder (drag header) → saved to view config (if permitted) else local override.
- Cell renderers/editors per `FieldKind` (in `components/fields/<kind>/`): `Renderer.vue`, `Editor.vue`, `FilterInput.vue`, `parsePaste()`, `formatCopy()`.
- Keyboard: arrows, Tab/Shift+Tab, Enter (edit / commit+down), Esc (cancel), F2, Delete/Backspace (clear), Ctrl/Cmd+C/V/X, Ctrl/Cmd+Z/Y, Ctrl+D (fill down), Ctrl+A, Shift+arrows (range), Space (expand row / toggle checkbox), Home/End, Ctrl+Home/End, PageUp/Down.
- **Copy:** range → TSV (+ HTML table) to clipboard via `formatCopy`.
- **Paste:** parse TSV (handles quoted multi-line cells from Excel/Sheets) → map to cells starting at focus → per-cell `parsePaste` → preview errors count → one bulk PATCH; if exceeding last row → confirm "add N rows" → bulk insert. Read-only/computed columns skipped.
- **Fill-down:** Ctrl+D copies top cell of selection down; drag fill handle extends the selection down (vertical only in v1).
- Grouped mode: flattened list of group headers + rows, collapsible, group header shows count & aggregates.
- Highlight: row background from `_hl_row`, cell background from `_hl_cells`; palette tokens map to light/dark-aware colors.
- Link cells: chips of display values; editor opens picker modal (search target rows, multi-select for many sides).
- Attachment cells: thumbnails; drag-drop upload onto cell; preview modal.
- Row expand modal: all fields in a form layout, comments panel, history tab.

## 5. Mobile (< 768 px) — D-016

- Sidebar becomes drawer; toolbar collapses into a "View options" bottom sheet (filter/sort/group/highlight builders are the same components in drawers).
- Grid replaced by **Card list**: each card shows display field + first 3–5 visible fields, highlight color stripe, group headers sticky.
- Tap card → full-screen **Row editor** (same as row expand modal) with full editing, comments, history.
- Add row via floating “+” button (opens row editor).
- Copy/paste and fill-down are desktop features; bulk select + delete available via long-press.
- Form view works natively on mobile.

## 6. i18n

- `vue-i18n` with `src/i18n/en.json` and `src/i18n/zh-CN.json`, lazy loaded. Key namespaces: `common.*`, `nav.*`, `grid.*`, `field.*`, `filter.*`, `admin.*`, `errors.<CODE>`.
- Locale order: user pref (server) → localStorage → browser language → `en`.
- Naive UI locale + dateLocale switched accordingly; dayjs locale too.
- Key parity check (both files identical key sets) runs in `ptdev lint`.

T0.5 precompiles locale JSON using `@intlify/unplugin-vue-i18n` and uses the
runtime-only vue-i18n build: no runtime code generation or CSP relaxation.
English is loaded once at startup for fallback/error text; Chinese is fetched
on demand (or on startup when saved/browser-selected). The store updates
`document.lang`, Naive UI locale/dateLocale, and dayjs together. Storage and
locale-chunk failures log explicitly and show translated alerts; a failed
language switch retains the active language. No server preferences exist yet.

## 7. Theme

- Naive UI `darkTheme` + custom theme overrides; `light | dark | system`.
- Highlight and select-choice colors defined as token pairs (light/dark) so data colors remain readable in both themes.
- Toggle in header next to the language toggle; persisted like locale.

System mode listens to `prefers-color-scheme` changes; explicit light/dark
overrides them. Store disposal removes the listener. All three choices persist
across reload, independently of language.

## 7a. Timezone & currency display

- User menu → Profile: timezone picker (searchable IANA list, "Use team default (Asia/Shanghai)" option).
- All datetime rendering via one `formatDateTime(value, field, userTz)` helper (dayjs + `utc`/`timezone` plugins); editors interpret typed input in the user's timezone.
- Currency values formatted with `Intl.NumberFormat(locale, { style: 'currency', currency: field.options.currency })`; field editor offers currency selection with app default (CNY) preselected.

## 8. Quality

- ESLint + Prettier, `vue-tsc --noEmit` in `ptdev lint` (local gate, no hosted CI).
- Unit tests: Vitest for paste parser, formatters, filter builder, undo stack.
- E2E: Playwright (desktop Chrome + mobile emulation) implementing acceptance tests.

T0.5 local commands (from `web/`):

```text
pnpm lint
pnpm test
pnpm build
```

`lint` includes ESLint 10, Prettier, strict `vue-tsc`, and the nested JSON
key-parity/non-empty-message check. The i18n ESLint rule rejects untranslated
template text, interpolated literals, and visible attributes (including labels,
placeholders and accessible names). Unit tests exercise the actual rule with
negative fixtures as well as real locale files, preference restoration, system
changes, and Naive UI locale/theme wiring. Vitest is capped at two workers.
`pnpm format` applies formatting. The shared Playwright harness is still T0.7;
T0.5's scoped browser verification is recorded in its task prompt.
