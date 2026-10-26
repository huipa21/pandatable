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

### T1.11 navigation and schema-only table shell (D-104..D-107)

- The authenticated shell (excluding setup and forced password change) has a
  collapsible workspace/book/table sidebar. Below 768 px it is a modal drawer
  with Escape/backdrop dismissal, focus restoration and close-on-navigation.
  Account/admin links remain separate; administrative inventory never grants or
  populates resource access.
- The memory-only `nav` store preserves `/nav` order and effective levels,
  clears/aborts on identity changes, and guards stale responses. Refresh on
  mount, focus/visibility return, every 30 seconds while visible, after structure
  mutations and resource 403/404. Ordinary reads coalesce; forced refresh aborts
  an older read. Requests have ten-second client deadlines. Failed reads keep
  previous data explicitly stale with retry; there is no realtime subsystem.
- `/home` lists accessible workspace contexts and offers creation. `/w/:workspaceId`
  and `/w/:wid/b/:bookId` show only visible children, breadcrumbs and explicit
  empty states. Path-only ancestors are labeled context, with no structure
  controls. Book creation/rename/delete/reorder needs workspace owner; table
  creation/rename/delete/reorder needs book owner. Table ownership alone never
  grants parent-structure controls.
- Native modal forms create/rename resources with trimmed 1..100-code-point
  names, no controls. Confirmed deletion honestly describes moving the item and
  descendants to Trash; restore/purge are not implemented. The Trash entry is
  disabled and scoped to M3. Same-parent book/table drag reorder sends `{index}`;
  Move up/down buttons provide keyboard and touch alternatives. No workspace
  ordering or cross-parent moves.
- Lazy `/t/:tableId/v/:viewId` reads real schema and displays table name,
  display-field name, visible views and `myLevel`. `/t/:tableId` replaces to the
  first Grid (otherwise first visible view). Missing/mismatched parents/views,
  hidden or removed resources show neutral localized not-found copy. This is a
  schema-only shell: no grid import or row/query/field/view editing. T1.12 adds
  sharing below without expanding that boundary.
- en/zh-CN use static translation keys and precompiled runtime-only messages.
  Explicit local preferences, memory-only auth, production CSP and the
  <600,000-byte/no-initial-grid bundle gate remain unchanged. Unit mocks are
  lower-layer evidence only; browser fixtures use real accounts and APIs.

### T1.12 sharing (D-108..D-112)

- Sidebar actions and resource pages expose Share only for target effective
  owner, including book/table-only owners who cannot rename/delete that resource
  without parent ownership. No Share on path-only/read/write/admin-only nodes.
- A viewport-bounded, scrollable native modal separates direct grants from
  read-only inherited grants and identifies principal type/status and source
  scope/id/level. Missing principals show an honest missing label/id; disabled
  and missing grants can be removed, not replaced. User content is interpolated,
  never HTML. Native focus trapping/Escape/return-to-opener works above the
  mobile navigation drawer; labeled native controls support keyboard/touch.
- User/group search is explicit and cancellation/generation guarded. Require
  2..100 code points/254 UTF-8 bytes without controls; return only 20 minimal
  matches plus a narrow-search notice, no empty enumeration or admin APIs.
  Changing search/type immediately clears old results and selection. The real
  server rechecks ownership and principal status; selecting a result is not a
  reservation.
- Add/replace/change/remove enters an explicit review step. The UI explains
  only-raise inheritance, active direct user last-owner protection and possible
  self-access loss. Per-principal inherited warnings never imply a lower grant
  restricts a group/inherited owner. Last-owner/disabled/missing/server/network
  failures stay explicit, translated from codes in en/zh-CN; never server text.
- Memory-only sharing state resets/aborts on scope/route/identity changes.
  Separate list/search requests and serialized writes have ten-second client
  deadlines. Failed reads disable editing and expose retry; failed writes never
  optimistically change permissions. Even uncertain writes reconcile nav.
  Successful writes re-read grants; self-access loss clears privileged state.
  Nav revisions refresh open sharing and the current table schema, so existing
  visible 30-second/focus/visibility refresh converges other tabs without
  promising realtime. No new dependency, auth persistence or CSP relaxation.
- Acceptance is real M1 grants/nav/schema/structure-denial behavior, not the
  unimplemented rows/editors/shared-view APIs in full AT-204..207. D-112 expands
  only scenario/global harness bounds to 150/360 seconds, retaining request,
  assertion, login, dashboard and PG deadlines, one worker and zero retries.

T0.5 introduced a separate `preferences` store for unauthenticated display
preferences (`pandatable.locale`, `pandatable.theme` in localStorage). T1.2 provides the
auth/profile backend and mirrored [API types](../web/src/api/types.ts); T1.3
adds only the localized `/setup` form and initial-setup route redirect. Setup
success shows the authenticated first-admin identity without adding a sign-in
form. T1.5 adds the auth store and login/forced-change/admin surfaces below;
profile UI and full server preference synchronization remain deferred. T1.8
adds only the explicit team-default fallback described below.
Unknown routes have a localized not-found page after setup.

### T1.5 authentication and admin users (D-081..D-083)

- `/login` preserves the welcome copy/card and adjacent local preference controls
  and adds username/email/password sign-in. `/home` is only a signed-in identity
  and future-workspace notice, not a workspace dashboard.
- Router navigation checks setup, then `/auth/me`. Missing sessions are expected
  401s; network/server errors render `/connection-error` with an explicit retry,
  never a success-shaped bypass. Forced users go to `/change-password` before
  any other post-setup route (including the spike). That form requires current,
  new and confirmation passwords, then sends the existing auth API request,
  clears local identity and requires re-login after all-session revocation.
- Active admins see **Admin** -> `/admin/users`. Others see no Admin link;
  `/admin/*` redirects to `/home` (or `/login` without a session). API authorization
  is independent of the client guard. Auth identity is memory-only; credentials
  and session tokens are never stored in localStorage.
- Admin users supports literal search/status, offset paging (50 default, <=500),
  create, edit/displayName/email/role, confirmed disable/enable and password
  reset. Inline, focusable forms use temporary-password policy/revocation
  notices. Desktop uses compact horizontal records; below 768 px they become
  stacked cards with wrapping actions and single-column forms. Every new string
  and error is translated in en/zh-CN. A revoked/disabled open tab redirects to
  login on its next request; M1 adds no realtime/polling promises.
- Actual role/status changes or reset of oneself clear the identity and redirect
  to login. Rejected last-admin changes and other errors stay visible, translated
  from error codes, never server text. User-list GETs are abortable and
  generation-guarded; stale identity reads cannot overwrite a newer sign-out.

The no-inline/no-eval CSP, lazy synthetic-only grid lab, local preference
semantics and <600,000-byte initial gzip/no-grid gate are unchanged. T1.2
profile/timezone UI, AT-116 and full AT-117 remain deferred; sign-in capability
alone does not certify all of AT-102.

### T1.8 settings and read-only workspace inventory (D-091..D-093)

- Lazy admin-only `/admin/settings` and `/admin/workspaces` preserve all
  existing navigation and independently guarded APIs. Settings use localized
  responsive forms, live timezone/currency catalogs, documented integer bounds,
  loading/error/retry and saved confirmation. Save sends **only changed fields**,
  avoiding accidental overwrites from stale forms. Reload discards unsaved edits.
  Upload enforcement/field creation/purge consumers are explicitly labeled as
  future work, not presented as already enforced.
- The shell fetches public `/meta/app` once at bootstrap with a ten-second
  client deadline, then uses its plain-text name for the header/document title.
  Admin success updates this tab immediately; other users/unauthenticated
  browsers see the persisted name after reload. Failed public reads retain the
  product/previous value with a visible translated error and retry, never fake
  success or exposure of privileged settings.
- Team default language replaces **browser fallback only**, without writing
  localStorage. An explicit stored or newly clicked local language choice
  always wins, including asynchronous races. Theme, OS following and existing
  server user preferences remain unchanged. Setup sends the actually chosen
  locale; no profile synchronization or forced-change bypass is added.
- Workspace cards show names, active-direct-user orphan state, direct owners
  (disabled status and group caveat), truncation notice and recorded attachment
  bytes. The size explanation includes retained Trash descendants and excludes
  PostgreSQL tables, thumbnails/backups and unrecorded files. Search/offset
  paging, loading, empty/error/retry and both locales are supported. No resource
  link, grant, transfer button or sidebar entry is created by the inventory.
- Reads are abortable, deadline-bounded and generation-guarded; unmount cancels
  pending work. Auth failures redirect using existing error codes. Brand and
  record names wrap on mobile and render as text, never HTML. Runtime-only
  i18n, no-inline/no-eval CSP and the no-grid initial gzip gate remain intact.
  Full AT-111/112 and upload AT-322 remain pending the real planned APIs.

### T1.7 system dashboard (D-088..D-090)

- Active admins see a separate **System dashboard** link at `/admin/dashboard`;
  existing Admin/users and Groups links/routes are preserved. Non-admin deep
  routes redirect independently of server-side `/admin/system` authorization.
- Responsive cards show host-visible CPU/load/RAM, app RSS, cgroup v2 memory,
  configured-volume disk, current PG size/connections/shared buffers, logical
  attachment/backup totals, actual versions/migration level and process uptime.
  The explanatory copy distinguishes host/VM from container observations.
- `useSystemMetrics` fetches immediately then on a fixed ten-second schedule
  without overlapping requests. Hidden/unmount aborts the request, clears timers
  and removes the listener; becoming visible fetches immediately. Client waits
  also have a ten-second abort deadline. A generation guard rejects late results.
- Both locales include loading, source-unavailable reasons, genuine zeros,
  localized numbers/IEC byte units, snapshot and hourly-directory timestamps.
  Failed refreshes retain data with an explicit stale alert, retry action and
  continued visible-page scheduled retries. Auth errors use existing redirects.
  No raw server error messages, secrets or filesystem paths are displayed.
- The dashboard is lazy, has no grid imports and does not change local shell
  preferences, runtime-only i18n, no-inline/no-eval CSP or initial bundle limits.
  Settings/workspaces and profile/default-timezone behavior remain later work.

### T1.6 groups and user detail (D-084..D-087)

- Active admins also see Groups navigation. `/admin/groups` provides paged
  literal search, counts and create; `/admin/groups/:id` provides name/description
  editing, confirmed deletion and paged/searchable member add/remove. The active
  user picker uses the existing admin users API with search and paging.
- User display-name links open `/admin/users/:userId`, showing sanitized identity,
  status and paged/searchable linked group memberships. Group members link back
  to user detail; both sides satisfy AT-109. Existing user editing/reset stays
  on `/admin/users`. Disabled memberships are shown and removable.
- Route changes abort/generation-guard list and picker reads; typed errors and
  explicit retries remain visible. No implicit admin access or new auth path.
  Both locales, empty/error states, desktop/mobile wrapping and deletion cancel
  are covered by the production-image admin scenario. Explicit expected HTTP
  diagnostics match method/path/status and must occur; unrelated console/CSP
  diagnostics still fail. The unchanged ten/minute/IP budget covers the suite.
- No dependencies or grid imports are added. Initial shell remains below
  600,000 gzip bytes; preferences, CSP and the opt-in lab are preserved.

## 4. Grid (desktop/tablet ≥ 768 px)

Our own `DataGrid` component wraps RevoGrid so the grid engine is swappable (D-005).

- Virtual rows & columns; windowed data fetch (page 100) with placeholders.
- Frozen columns, resize, reorder (drag header) → saved to view config (if permitted) else local override.
- Batch-1 pure `FieldKind` definitions live in `src/fields`, bound through
  `bindField(kind, options)`; shared presentation-family components are
  `components/fields/FieldRenderer.vue` and `FieldEditor.vue` (D-121).
  The contract includes normalizeOptions/normalize/parsePaste/formatCopy/
  formatDisplay/readOnly/constraints/filterOps/aggregates. Filter inputs remain
  later work. System editors render nothing. Native controls commit validated
  values, preserve IME input and show localized errors; Escape cancels,
  multiline uses Ctrl/Meta+Enter, select/checkbox use native changes.
  These components/registry have no current route imports. T1.16 owns real
  grid integration; the synthetic spike stays isolated.
- Keyboard: arrows, Tab/Shift+Tab, Enter (edit / commit+down), Esc (cancel), F2, Delete/Backspace (clear), Ctrl/Cmd+C/V/X, Ctrl/Cmd+Z/Y, Ctrl+D (fill down), Ctrl+A, Shift+arrows (range), Space (expand row / toggle checkbox), Home/End, Ctrl+Home/End, PageUp/Down.
- **Copy:** range → TSV (+ HTML table) to clipboard via `formatCopy`.
- **Paste:** parse TSV (handles quoted multi-line cells from Excel/Sheets) → map to cells starting at focus → per-cell `parsePaste` → preview errors count → one bulk PATCH; if exceeding last row → confirm "add N rows" → bulk insert. Read-only/computed columns skipped.
- **Fill-down:** Ctrl+D copies top cell of selection down; drag fill handle extends the selection down (vertical only in v1).
- Grouped mode: flattened list of group headers + rows, collapsible, group header shows count & aggregates.
- Highlight: row background from `_hl_row`, cell background from `_hl_cells`; palette tokens map to light/dark-aware colors.
- Link cells: chips of display values; editor opens picker modal (search target rows, multi-select for many sides).
- Attachment cells: thumbnails; drag-drop upload onto cell; preview modal.
- Row expand modal: all fields in a form layout, comments panel, history tab.

### T0.8 feasibility lab (not the production grid)

Open `/_spikes/revogrid` directly and choose **Mount prototype**. This route
is deliberately unlinked from the welcome/table navigation. It loads the engine
only after explicit opt-in at >=768 px; smaller viewports show a translated
mobile-card-intent notice, not a grid or an M1 card editor. Welcome and other
deep-route behavior remain unchanged.

[DataGrid.vue](../web/src/components/grid/DataGrid.vue) owns all engine imports,
event adaptation, source-store updates, renderer/editor, TSV and fill-down
handling. Its external boundary accepts an engine-independent `WindowSource`
(`total`, abortable `fetch(offset, limit, signal)`) and emits window statistics.
The isolated [synthetic provider](../web/src/spikes/synthetic.ts) has 200,000
logical rows, page size 100, deliberately varied 40/120 ms timers and **no
HTTP/database calls**. The active page and its two neighbors retain at most
300 row objects. Remaining engine slots share one immutable placeholder;
RevoGrid still has O(N) reference/index arrays, not O(visible rows) memory.
Page generation checks ignore out-of-order responses even from providers
which ignore cancellation. Unmount aborts work and clears cache/edit overlays.
Only the last 20 request records and at most 5,000 transient changed cells
are retained.

Verified built-ins: virtual scrolling, Enter/Escape editing hooks, Shift+arrow
and drag ranges, drag fill handle, column pinning, compact light/dark themes,
and collapsible grouping on a **separate complete 100-row read-only sample**.
The wrapper supplies a Vue multiline note editor, a quantity renderer,
keyboard focus after pointer input, Ctrl+D, atomic loaded-cell edits and
quoted multiline TSV parse/format (the engine's plain TSV splitter is
insufficient). Copy/paste bounds and invalid edits show translated errors;
placeholder data is not copied. Clipboard is disabled for the grouped sample.

D-066..D-068 confirm D-005 for further implementation. This lab does not provide
production `FieldKind` registries, server writes, ACL, view persistence,
flattened remote grouping/counts/aggregates, HTML clipboard, add-row confirmation
or full keyboard/undo coverage. Built-in drag fill can extend horizontally;
the vertical-only v1 policy still needs enforcement in T1.17. Real clipboard
round-trips were tested in permission-granted Chromium on loopback, not inside
Excel/Sheets or every OS/browser. Tablet touch is emulated, not physical-device
certification. See the [exact results](progress.md#t08-verification-evidence-2026-10-24)
and [T0.8 completion/handoff](task-prompts/T0.8.md#implementation-and-handoff-2026-10-24).

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
language switch retains the active language. Server profile preferences exist
from T1.2, but this unauthenticated shell does not read or write them yet.

## 7. Theme

- Naive UI `darkTheme` + custom theme overrides; `light | dark | system`.
- Highlight and select-choice colors defined as token pairs (light/dark) so data colors remain readable in both themes.
- Toggle in header next to the language toggle; persisted like locale.

System mode listens to `prefers-color-scheme` changes; explicit light/dark
overrides them. Store disposal removes the listener. All three choices persist
across reload, independently of language.

## 7a. Timezone & currency display

- User menu → Profile: timezone picker (searchable IANA list, "Use team default (Asia/Shanghai)" option).
- Batch-1 datetime formatting/parsing uses `src/fields/temporal.ts` with
  `Intl.DateTimeFormat` zone offsets and exact integer seconds/microseconds
  (D-117/D-121), not lossy JS Date/number wire values or dayjs timezone plugins.
  Offset-free typed input needs the viewer/fixed zone; DST gaps fail, overlaps
  use the earlier instant. Copy retains an explicit offset and microseconds.
  Display without a usable zone explicitly labels UTC. Callers must supply
  effectiveTimezone from auth when the real grid is integrated.
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
`pnpm format` applies formatting. T0.7 adds pinned Playwright under `web/e2e/`:
desktop/mobile Chromium, reusable real API request fixtures, automatic browser/
CSP diagnostics, failure traces/screenshots and production-shell coverage.
`ptdev e2e` builds and tears down an isolated real image/Compose stack; it is
separate from the fast gate. Vitest selects only `src/**/*.test.ts`, while
Playwright selects E2E specs. Both configs and E2E TypeScript participate in
strict type-check/lint. The opt-in lifecycle failure probe is not an acceptance
spec. Auth/setup/seed flows remain M1 work. See
[14, browser setup](14-dev-build-release.md#browser-setup-and-isolated-e2e).
