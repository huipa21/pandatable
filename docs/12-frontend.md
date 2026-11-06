# 12 — Frontend

## 1. Stack

Vue 3 (Composition API, `<script setup>`), TypeScript (strict), Vite, Pinia, Vue Router, Naive UI, RevoGrid (`@revolist/vue3-datagrid`), vue-i18n, VueUse, dayjs. No CSS framework beyond Naive UI theming + small utility CSS. Target bundle: < 600 KB gzip initial (grid lazy-loaded on table route).

## 2. Layout

Shell per D-209 (supersedes the earlier sketch):

```
┌────────────────────────────────────────────────────────────────────────┐
│ PT PandaTable ⇤ │ Workspace › Book › Table [› View]   🔍  中文/EN  ☀/🌙 │  ← header
├─────────────────┬──────────────────────────────────────────────────────┤
│ Workspaces    + │ ↶ ↷ Fields · Filter · Group · … · Import · Export   │
│ ▾ Workspace  ⋯ │                                                      │
│   ▾ 📘 Book   ⋯ │                                                      │
│      ▦ Table  ⋯ │ ┌──────────────────────────────────────────────────┐ │
│      ▾ Table 2  │ │ Grid / Form / Kanban …                           │ │
│         ◫ View ⋯│ │                                                  │ │
│ ▾ Workspace 2 ⋯ │ └──────────────────────────────────────────────────┘ │
│ (A)³ avatar menu│                                                      │
└─────────────────┴──────────────────────────────────────────────────────┘
```

- Workspace = company/program; book ("workbook") = project holding tables and
  their relations; table = sheet; view = filtered/sorted table.
- Header: logo → `/home`; ⇤ hides/shows the sidebar (remembered) or opens the
  mobile drawer; breadcrumb (view segment only when the URL names one); 🔍
  quick-jump palette (Ctrl/Cmd+K, client-side over the nav tree); language and
  light↔dark toggles. The workspace remains visible beside the sidebar toggle on
  narrow screens, with the current resource retained as space permits. When the
  sidebar is hidden the avatar moves to the header.
- Sidebar: a static Workspaces heading and Add action precede every accessible
  workspace in server order. A workspace link selects it; a separate control
  expands its books, tables and views. The selected route/last-visited context is
  independent of the visibly marked per-user default. Eligible workspace actions
  set that default; path-only ancestors remain context without owner controls.
  Books toggle their tables; hovering/focusing a table's icon turns it into the
  views dropdown; the table name opens `/t/:tableId` (default view, no redirect).
  Table "⋯" menus include view creation, and each view has a context menu for
  its permitted actions alongside the sharing and structure menus.
- Avatar menu (unread badge): notifications, Profile, Change password, Trash,
  Admin submenu (admins), Sign out. No footer.
- `/home`: loading/error-aware resolver to the preferred workspace. Retry is
  explicit; navigating to a resource directly never invokes the resolver.
- `/w/:workspaceId`: accessible workbooks appear in server order as responsive
  links with their custom icon or persisted fallback. Path-only books are
  labeled as context; long names wrap within their cards. Workspace owners
  retain sharing and first-book creation. A failed navigation refresh is marked
  stale with retry, including while a retry is pending; a stale empty list is
  not presented as a confirmed empty workspace.

Routes:
- `/setup`, `/login`, `/change-password`
- `/w/:workspaceId`, `/w/:wid/b/:bookId`, `/t/:tableId` (default view), `/t/:tableId/v/:viewId`, `/t/:tableId/v/:viewId/r/:rowId` (row modal deep link)
- `/admin/{dashboard,users,groups,workspaces,settings,backups}` (guarded by role)

## 3. Stores (Pinia)

| Store | State |
|---|---|
| `auth` | me, prefs (locale, theme) |
| `nav` | sidebar tree + effective levels |
| `viewManagement` | authorized target-table schema and contextual view mutations |
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
- The sidebar lists all accessible workspace contexts and offers workspace
  creation; `/home` resolves the per-user default (D-219). `/w/:workspaceId` and
  `/w/:wid/b/:bookId` show only visible children, breadcrumbs and explicit empty
  states. Path-only ancestors are labeled context, with no structure controls or
  default selection. Book creation/rename/delete/reorder needs workspace owner;
  table creation/rename/delete/reorder needs book owner. Table ownership alone
  never grants parent-structure controls.
- Native modal forms create/rename resources with trimmed 1..100-code-point
  names, no controls. Confirmed deletion honestly describes moving the item and
  descendants to Trash; restore/purge are not implemented. The Trash entry is
  disabled and scoped to M3. Same-parent book/table drag reorder sends `{index}`;
  Move up/down buttons provide keyboard and touch alternatives. No workspace
  ordering or cross-parent moves.
- Lazy `/t/:tableId/v/:viewId` reads real schema and displays table name,
  display-field name, visible views and `myLevel`. `/t/:tableId` replaces to the
  first Grid (otherwise first visible view); since D-209 it renders that default
  view in place without redirecting. Missing/mismatched parents/views,
  hidden or removed resources show neutral localized not-found copy. This is a
  schema-only shell: no grid import or row/query/field/view editing. T1.12 adds
  sharing below without expanding that boundary. T1.14 added the field panel;
  T1.16 later integrated the production grid and row APIs.
- en/zh-CN use static translation keys and precompiled runtime-only messages.
  Explicit local preferences, memory-only auth, production CSP and the
  <600,000-byte/no-initial-grid bundle gate remain unchanged. Unit mocks are
  lower-layer evidence only; browser fixtures use real accounts and APIs.

### UX-05 sidebar views (D-225)

The table menu opens a dialog to create a personal or shared Grid, Form, Kanban
or Gallery view; readers can create personal views without structure-edit
permission. Selecting an explicit view expands its table in the sidebar. Each
view menu offers the authorized rename, duplicate, reorder, lock/unlock and
delete actions. The active-view action for saving temporary shared-view changes
as a personal view remains available on the table page.

All these operations share a route- and identity-guarded management store. It
loads the target table's authorized schema for permissions, lock state and
versions, refreshes navigation after writes, and retains explicit retry/error
feedback on conflicts and uncertain results. Table deletion continues to use
Trash: descendant views and deep links are unavailable until restoration.

### T1.12 sharing (D-108..D-112)

- Sidebar actions and resource pages expose Share only for target effective
  owner, including book/table-only owners who cannot rename/delete that resource
  without parent ownership. No Share on path-only/read/write/admin-only nodes.
- Table pages host Share in the eye-icon Visibility menu (D-214): inside the
  desktop Grid toolbar after Highlight, otherwise (mobile, non-Grid views) in
  the table toolbar. It launches the same dialog and returns focus to the
  menu opener; workspace/book pages keep their direct Share button.
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
  only scenario/global harness bounds to 150/600 seconds, retaining request,
  assertion, login, dashboard and PG deadlines, one worker and zero retries.

### T1.14 field panel (D-123..D-130)

- The schema page lazily loads `components/fields/FieldPanel.vue` (its own
  chunk; initial shell has no grid/registry imports). It lists live fields in
  position order (system fields last, marked), with type, required/unique and
  display-field badges. Readers/writers get the read-only list and a notice.
- Table owners get "Add field" and a per-field native `<details>` header menu:
  Edit, Move up/down (non-system), Use as display field (eligible non-system,
  non-checkbox kinds) and Delete (confirm dialog, soft delete). System fields
  allow name/description edits only.
- Creation starts with a name and a searchable, height-bounded icon list of all
  supported types, including Link, Lookup, Rollup and Formula (D-212). Selecting
  a type opens its settings; the type picker remains available and preserves
  the name when switching. There is no separate "More field types" section.
  Link uses four cardinality radio choices and a same-book target selector;
  reverse-field naming and deletion policy live under Advanced settings.
  Lookup/Rollup use relation and target selectors, with compatible aggregates
  for Rollup. Formula shows the supported function list even before typing.
- The native modal editor (`fieldForm.ts` keeps pure draft/body logic) shows
  name, kind (create only), description, kind options from the registry
  (length/size/precision/scale, date/time formats, zone, choices with label/
  color/remove/order), required/unique/default and an explicit "fill empty
  cells with default" option. PATCH sends only changed members.
- Requests are serialized, ten-second, aborted on unmount and send `If-Match`.
  Success replaces state with the returned schema. A version conflict reloads
  and re-seeds the open editor. 401/403/404 and uncertain outcomes (5xx/network)
  reload the schema, and 403/404 also refresh nav. Errors are translated from
  code/reason/option/count, never server text. Nav-revision refreshes keep the
  current schema visible and only replace it with a same-table, not-older one.
- UI-FIELD-TILES (D-215) replaces the list rows with compact tiles inside the
  grid **Fields** dropdown: the field name is the pointer drag handle (press,
  drag and drop-target feedback; reduced-motion aware), the checkbox toggles
  visibility, and a **⋯** button opens an anchored, viewport-clamped actions
  menu (Edit, Delete, Move up/down). "Use as display field" moves to Edit's
  Advanced settings. Edit reuses the Add
  layout; the type picker offers only the safe conversions for the field and
  type change requires saving pending edits first. Relation/lookup/rollup edit
  their own settings with unavailable options kept visible. Reorder flushes
  pending view-column changes, PATCHes the field index only when schema order
  changes, then saves the active view order (`DataGrid.reorderFields`).

### T1.15 row/query API boundary (D-132..D-135)

`web/src/api/types.ts` mirrors the row/query DTOs: canonical bigint row IDs
remain strings, row values are keyed by field UUID, and `fieldIds` carries the
visible view order separately from the value map. Row constraint and schema-
change codes are translated through static en/zh-CN keys. This task adds no
grid, store, route, optimistic row state or initial grid import; production
editing remains T1.16 and copy/paste remains T1.17.

T0.5 introduced a separate `preferences` store for unauthenticated display
preferences (`pandatable.locale`, `pandatable.theme` in localStorage). T1.2
provides the auth/profile backend and mirrored [API types](../web/src/api/types.ts);
T1.3 adds the localized `/setup` form and initial-setup route redirect. Setup
success shows the authenticated first-admin identity without adding a sign-in
form. T1.5 adds the auth store and login/forced-change/admin surfaces below.
T1.22 adds the authenticated self-profile page, using the existing preference
store rather than a second mechanism. T1.8 adds the explicit team-default
fallback described below.
Unknown routes have a localized not-found page after setup.

### T1.5 authentication and admin users (D-081..D-083)

- `/login` preserves the welcome copy/card and adjacent local preference controls
  and adds username/email/password sign-in. Its original `/home` identity notice
  was replaced by the preferred-workspace resolver in D-219.
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
semantics and <600,000-byte initial gzip/no-grid gate are unchanged. T1.2's
backend remains unchanged; AT-102/116 and the real datetime/default-timezone
browser scenarios are tracked by T1.22 and must not be inferred from the
login/profile APIs alone.

### T1.22 self profile (D-153)

- `/profile` is a lazy authenticated route available to ordinary users and
  admins alike. It can only read the current user's `/auth/me`; there is no
  user-id parameter or admin-style profile access.
- The form loads the live `/meta/timezones` catalog, keeps edits in memory,
  sends only changed values to `PATCH /auth/me`, and reloads canonical server
  values on entry/retry. Its explicit "Use app default" option sends `null`,
  never an empty timezone string. Password changes link to the existing
  `/change-password` screen, which clears memory auth and requires sign-in
  after server-wide session revocation.
- After successful profile persistence, locale/theme update through the
  existing preferences store and its `pandatable.locale` /
  `pandatable.theme` values; the shared header and profile therefore use the
  same runtime state. An unsubmitted profile draft is not stored. Profile
  requests use the established no-store same-origin API helper with a ten-
  second deadline and visible load/error/retry/saved states.
- Native labeled controls and the single-column form at <=767 px preserve
  keyboard/touch use and avoid adding initial grid imports. Both locale catalogs
  contain matching profile strings.
- T5.2 adds the default-off mention-email checkbox to this same profile form.
  It sends only a changed `mentionEmail` value and does not disable in-app
  notifications.

### T5.2 email and password recovery (D-205..D-207)

- Admin Settings links to a localized SMTP form. Saved settings are reloaded
  without revealing either credential; blank username and password inputs
  preserve their stored values unless the administrator explicitly removes
  them. Test mail uses saved settings and reports delivery errors.
- Login links to a generic email reset request. The reset page removes the
  token fragment from the browser address immediately, keeps it in memory
  until submission, and requires sign-in again after completion.

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

- UX-06 (D-226) puts undo/redo, presence, table metadata, Import and Export in
  the existing grid toolbar; mobile and non-grid views use one equivalent table
  toolbar. Visibility continues to mean resource ACL sharing, while Fields
  controls columns and display-field editing. Export progress and failures stay
  visible outside its menu; CSV uses the active view's filter, sort, search and
  visible columns. Toolbar popovers stay above row content and close other menus
  when activated. Field and filter popovers remain outside the schema-keyed Grid
  and track the wrapping toolbar's height, preserving editor feedback on schema
  refresh. The trailing action is UX-10's New Entry button (D-227). In the
  Grid toolbar the display-field/access summary only fills space left by the
  actions, so the desktop toolbar stays one row (D-229).
- Virtual rows & columns; windowed data fetch (page 100) with placeholders.
- Frozen columns, resize, reorder (drag header) → saved to view config (if permitted) else local override.
- Batch-1 and batch-2 pure `FieldKind` definitions live in `src/fields`, bound through
  `bindField(kind, options)`; shared presentation-family components are
  `components/fields/FieldRenderer.vue` and `FieldEditor.vue` (D-121).
  The contract includes normalizeOptions/normalize/parsePaste/formatCopy/
  formatDisplay/readOnly/constraints/filterOps/aggregates. Filter inputs remain
  later work. System and autonumber editors render nothing. Native controls
  commit validated values, preserve IME input and show localized errors; Escape
  cancels, multiline uses Ctrl/Meta+Enter, select/checkbox use native changes.
  Currency/percent keep exact decimal strings through `Intl.NumberFormat`;
  table-scoped user pickers search active users and render user labels through
  the separate, permission-checked label resolver (D-161/D-162).
  The real table route lazy-loads these components through T1.16's production
  grid. The synthetic spike stays isolated in `SpikeDataGrid.vue`.
- UX-07's grid-only scalar editing (D-228) opens the cell editor on a single
  unmodified primary click of a writable scalar cell (not a drag); double-click,
  Enter and F2 still open it. Leaving the editor commits a valid draft once
  through the existing serialized cell PATCH and undo; invalid drafts stay open
  with their localized error, Escape cancels, and IME composition is never
  committed mid-compose. Read-only, computed, system and non-scalar cells keep
  their own behavior. Pending writes are guarded against table, view, user,
  schema and permission changes; overlapping realtime updates reconcile
  without clearing the loaded window. Row forms keep explicit Save.
- UX-08's grid-only choice interactions (D-221) toggle a writable checkbox once
  when its box is clicked; clicking elsewhere selects the cell and Space also
  toggles it. One click opens a single- or multi-select listbox. A single choice
  or clear commits immediately; multiple choices remain open and commit once on
  leaving the editor or Enter, while Escape cancels. Existing unavailable
  labels remain visible. Choice wire values are labels in configured order;
  text/integer lists retain their codec editor and row forms keep explicit Save.
- T1.16's production grid fetches 100-row query pages and keeps only the active
  page plus its two neighbors in the row cache. It guards queries and writes
  against route/schema/identity changes, uses table `myLevel` for write
  controls, validates editor commits with the bound `FieldKind`, and persists
  successful cell edits through row PATCH. Add/delete row actions use the row
  APIs and show failures without optimistic success. T1.21 connects the Fields
  menu and native header resize/drag to versioned column config (D-148/D-149).
  A transient ordered `fieldIds` query override (D-138) still supports immediate
  local visibility, including explicit system fields. T1.17 enables
  copy/cut/paste and vertical fill-down on this production grid: copy uses each
  bound codec's `formatCopy`; paste parses TSV and each cell with `parsePaste`,
  reports invalid cells, and submits valid existing-row cells in one bounded
  bulk patch. Pasting beyond the loaded table tail requires confirmation before
  bounded bulk insertion. Ctrl+D and the fill handle copy the selection's top
  value down; horizontal fill is not supported. The synthetic spike remains a
  separate lab-only implementation (D-139).
- UX-09's blank-row action (D-222) sits in RevoGrid's native footer directly
  after the loaded visible tail, including an empty grid and grouped results.
  It is available to desktop writers only once the query is ready and the end
  is visible. It uses the existing row API and undo, then requeries and reveals
  the created row if present; filters, sorting or collapsed groups may affect
  its visibility. Required/unique failures show the server error and point to
  the New Entry form rather than fabricating values.
  Append requery keeps the previous logical length as empty, non-data
  placeholders until authoritative rows arrive, avoiding virtual-scroll resets
  (D-223). Grouped requery likewise retains geometry and loads the anchored
  leaf page before checking the created row's identity (D-224). Failed queries
  clear retained placeholders and surface the error; placeholders never become
  export, clipboard or selection data.
- Keyboard (T1.21/D-150):
  - Arrows move between cells; Tab/Shift+Tab move forward/back.
  - Enter starts editing; validated editor Enter commits and moves down.
    Multiline keeps plain Enter as a newline and uses Ctrl/Cmd+Enter to commit.
    F2 opens the existing editor; Esc cancels its draft without a row write.
  - Delete/Backspace clear the selected bounded loaded range using each
    writable field's ordinary empty-value codec (nullable values become null,
    checkbox false). Read-only/computed/system columns are skipped. The atomic
    bulk PATCH retains 1000 rows/5000 cells/512 KiB and normal required/unique
    failures; no row version or alternate validator.
  - Ctrl/Cmd+C/V/X use codec-backed clipboard operations; Ctrl/Cmd+D fills down.
    Ctrl/Cmd+A selects the current column viewport's data range;
    Shift+arrows extend a range in that viewport. Frozen and unpinned columns
    are distinct RevoGrid selection viewports; cross-viewport range copying
    is not implied. Existing bounded clipboard limits remain explicit.
  - Space toggles a writable checkbox cell; on other data cells it opens the
    row dialog. A read-only checkbox is untouched. Native controls/IME keep
    their own Space, navigation, editing and clipboard behavior.
  - Home/End select the first/last visible data column of the current row.
    Ctrl/Cmd+Home/End select the first/last data cell in the view, loading the
    destination's bounded real page when needed. PageUp/Down move one visible
    viewport of rows, clamped to the first/last row.
  - Global jumps resolve row identity through RemoteRowsWindow before calling
    public global-coordinate grid methods. DOM/afterfocus indexes remain
    virtual, never assumed to be global offsets (D-139). Native RevoGrid
    arrows/Tab/range/clipboard behavior is exercised on the production grid,
    not inferred from defaults. Undo/redo Ctrl/Cmd+Z/Y remains later work.
    D-151's awaited bounded destination load preserves logical source length
    while fetching, so a far jump cannot clamp focus against an empty grid.
- **Copy:** range → TSV (+ HTML table) to clipboard via `formatCopy`.
- **Paste:** parse TSV (handles quoted multi-line cells from Excel/Sheets) → map to cells starting at focus → per-cell `parsePaste` → preview errors count → one bulk PATCH; if exceeding last row → confirm "add N rows" → bulk insert. Read-only/computed columns skipped.
- **Fill-down:** Ctrl+D copies top cell of selection down; drag fill handle extends the selection down (vertical only in v1).
- Grouped mode: flattened list of group headers + rows, collapsible, group header shows count & aggregates.
- The desktop Grid's **Highlight** menu edits up to 20 ordered, independent,
  filter-backed rules, each targeting a row or selected live cells. Rule
  headings are drag handles; labeled Move up/Move down buttons provide keyboard
  and touch reordering. Incomplete rules remain local until valid; saves retain
  other view-config members. Query `_hl_row` and `_hl_cells` apply the first
  matching rule independently to each target; a cell match overrides its row
  color.
- Highlight row/cell backgrounds resolve six palette tokens through light/
  dark theme CSS pairs; highlighted text, links, and validation messages remain
  readable in both themes.
- T2.11 adds an accessible quick-search input and footer aggregate controls to
  the production grid. Search stays transient and matches only effectively
  visible searchable fields; paging, filters, groups, and footer results share
  the same term. Per-visible-field aggregate choices use the field codec's
  supported operations, persist through the versioned merged view config when
  allowed, and remain local overrides for read-only/locked views. The footer
  refreshes on search, filter, visibility, and aggregate changes, formats
  values with the field codec, and surfaces request failures.
- Link cells: chips of display values; editor opens picker modal (search target rows, multi-select for many sides).
- Attachment cells: thumbnails; drag-drop upload onto cell; preview modal.
- Row expand modal: all live fields in a form layout. T1.18 implements the
  field form/deep link below; comments/history remain later collaboration work.

T1.20 adds an opt-in **production-image** browser measurement, not a second
grid/provider or synthetic seed UI. The
[performance workflow](14-dev-build-release.md#t120-safe-seed-and-production-browser-baseline-d-143d-144)
measures fresh authenticated document navigation to an exact usable real row
across a rendering opportunity and five real global middle-page jumps.
Request/header/body/render boundaries are independent; page size 100,
<=300 loaded models, D-139 identity/global mapping and all existing lazy
grid/editor/card imports remain unchanged. Timing results on a non-target VM
do not certify AT-312 or NFR-1.

### T1.21 desktop column state (D-148/D-149)

The ordered column config drives width, visibility and leading frozen user
columns; the selection/expand utility column is pinned separately and does
not count toward frozen. Header drag/resize have labeled numeric width/frozen
and left/right button alternatives in the Fields menu. Moving a visible
column skips hidden fields; hiding/showing retains its configured position.
Number-input drafts survive unrelated asynchronous renders until commit.

Write/owner users on unlocked views send debounced (350 ms), coalesced,
serialized column-only PATCHes with the current view version. Saves do not
disable row editing or scrolling. New intents during an in-flight save are
retained for the next version. Reads, writes and reconciliation have independent
abort/deadline/generation/identity guards. A failed save restores authoritative
columns, reloads schema and rebases retained intents against live fields.
One version conflict retries automatically; repeated conflicts or other
failures retain the authoritative display with an explicit translated Retry.
No failed save leaves optimistic column state or a pretend saved marker.

Readers, locked views (including owners), and non-writable views never send
column PATCHes and show a local-only notice. Their memory overrides disappear
on reload/navigation. Below 768 px cards remain the provider, with only their
existing transient visibility: no RevoGrid or desktop column controls mount.
Grid, editors and registry stay lazy; the initial shell budget is unchanged.

### T1.18 row expansion (D-140)

The lazy [RowExpandDialog](../web/src/components/rows/RowExpandDialog.vue)
opens at `/t/:tableId/v/:viewId/r/:rowId`. The real grid exposes a labeled
expand icon for each loaded row; Space on a focused loaded cell opens that row
without treating virtual row indexes as identities. Native inputs/buttons and
IME retain their own keyboard behavior. Row-only route changes leave the grid
and FieldPanel mounted. Closing replaces the URL with the containing view;
browser Back/Forward and direct reload use the same schema-authorized route.

The dialog reads the full row, not just visible columns or cached page values.
The pinned `#` grid gutter is a view-position ordinal, not the database ID;
the unique system ID stays hidden by default. The gutter contains selection
and a labeled expand icon. The popup heading uses the first non-system field
in metadata order (D-211), with a localized fallback for blank primary values.
Search beneath the heading matches all field names and formatted values.
Normal mode uses bordered field values; compact mode removes those borders.
Comments and history replace the form area, never sit beside it. Their close
control or Escape returns to the preserved form. Notification links open the
comments overlay directly. Fullscreen fills only the table workspace and tracks
its resize/scroll geometry.

Previous/next use the active view order and existing paging; boundaries are
reported, and controls are disabled while editing, saving, or duplicating.
Rows outside the loaded view report unavailable navigation. Kanban navigation
is limited to currently displayed cards. Writers can duplicate editable values
into a local draft and explicitly save a new identity; system, computed and
relation fields are not copied, and unique values must be reviewed. Closing
discards the duplicate without an insert. Deletion requires confirmation and
records restore snapshots for undo. History displays author avatars, timestamps,
field cards, removed values in red and added values in green.

All live fields are listed in metadata order; system fields use read-only
renderers, and authorship stays an unresolved UUID (no sharing-directory lookup).
Readers have no edit controls. Writers/owners edit one field at a time with the
existing codec-bound FieldEditor. Text has an explicit Save button or Enter;
long text uses Ctrl/Meta+Enter, checkbox/single-select changes save immediately,
and multi-select has explicit Save. Cancel/Escape discards the active editor.
Closing never implicitly saves a draft.

On desktop, Grid writers can open the same local creation draft from **New
Entry**, the rightmost toolbar action. This is separate from the grid-tail plus,
which creates directly using defaults. Applying a field only updates the draft;
untouched fields remain omitted from the atomic create request. Cancelling a
new draft leaves the current route unchanged. Success refreshes the active query
and opens the authoritative created-row deep link. Resolving a bare table route
to that same default view does not remount the grid. Mobile retains its single
add-row FAB for this form; readers have neither creation action (D-227).

Each save uses single-row PATCH, preserves exact bigint/decimal/microsecond
wire values and rejects UTF-8 bodies over 4096 bytes visibly. Values/status
change only on a successful authoritative response; the grid's active bounded
window refreshes after save. Access failures clear row content and refresh
nav/schema, schema conflicts reload metadata, and auth/forced-change outcomes
use existing login/password routes. Reads/writes have ten-second deadlines
and generation/abort/identity guards, including implementations that ignore
cancellation. Identical schema refreshes do not restart failed row reads.
Locale/theme changes stay reactive; the dialog is viewport-bounded and native
focus trapping/Escape work without a new dependency. T1.19 adds cards,
full-screen row layout and FAB as described in section 5; T3.3 adds comments as
described below. Row history uses the field-change overlay described above.

### T3.3 comments and notifications (D-185)

The row dialog loads a paginated, plain-text comment panel for readers as well
as writers. It offers access-filtered mention autocomplete and links resolved
mentions to their comment; typed ineligible names are visibly not notified.
Authors can edit/delete and table owners can delete any comment. Desktop row
headers and mobile cards display counts and refresh on comment changes.

The authenticated shell's bell loads persisted unread counts, updates on
recipient-only realtime events even outside the table, and opens an accessible
grid's row link with `?comment={id}`. The requested comment is pinned and
focused, including when older than the first normal page. Errors, a missing
grid, and failed saves are shown rather than silently discarded. Both
desktop and mobile use the same localized components and server ACL.

### T0.8 feasibility lab (not the production grid)

Open `/_spikes/revogrid` directly and choose **Mount prototype**. This route
is deliberately unlinked from the welcome/table navigation. It loads the engine
only after explicit opt-in at >=768 px; smaller viewports show a translated
mobile-card-intent notice, not a grid or an M1 card editor. Welcome and other
deep-route behavior remain unchanged.

[SpikeDataGrid.vue](../web/src/components/grid/SpikeDataGrid.vue) owns the
synthetic lab's engine imports,
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
or full keyboard/undo coverage. The spike's built-in fill behavior is
lab-only and does not define the production grid's vertical-only policy. Real
clipboard round-trips were tested in permission-granted Chromium on loopback,
not inside Excel/Sheets or every OS/browser. Tablet touch is emulated, not
physical-device certification. See the [exact results](archive/progress-m0-m1.md#t08-verification-evidence-2026-10-24)
and [T0.8 completion/handoff](task-prompts/T0.8.md#implementation-and-handoff-2026-10-24).

### T4.1 Kanban view (D-203)

The board groups cards by an active single-select or user field and renders the
display field plus selected card fields. It keeps unassigned rows and existing
unavailable choices visible, includes empty single-select choices, and lets
users search for additional user destinations within the table directory.
The board loads group metadata, then displays up to eight columns with 20 cards
per column at a time; paging is explicit.
Writers can drag cards or use the keyboard/touch-accessible move form. Readers
can inspect cards but not move them. Realtime notifications refresh the board;
row edits and configuration changes use the existing permission, error, and
undo paths.

### T4.2 Gallery view (D-204)

Gallery cards show the display field, selected card fields and the first
attachment with a canonical thumbnail in the configured cover field. Missing
images and invalid or failed previews have explicit placeholders. Cards open
the existing row dialog; configuration uses the existing permission, version
and undo paths. Realtime changes refresh the current page. Pages contain at
most 20 cards, and the UI reports the existing query offset limit when it
prevents access to additional rows.

## 5. Mobile (< 768 px) — D-016

- Sidebar becomes drawer; toolbar collapses into a "View options" bottom sheet (filter/sort/group/highlight builders are the same components in drawers).
- Grid replaced by **Card list**: each card shows display field + first 3–5 visible fields, highlight color stripe, group headers sticky.
- Tap card → full-screen **Row editor** (same as row expand modal) with full editing, comments, history.
- Add row via floating “+” button (opens row editor).
- Copy/paste and fill-down are desktop features; bulk select + delete available via long-press.
- Form view works natively on mobile.

T2.13's form builder and submission view use the same accessible field editors
on desktop and mobile. The form remains a single-column layout at 390 px,
supports keyboard/touch controls, and shows read access without an enabled
submit action. Refreshing an unchanged schema or view preserves in-progress
values and submission feedback; an actual view or schema revision resets the
draft and its field editors together.

### T1.19 implemented mobile grid (D-141/D-142)

The bullets above describe the eventual mobile product; T1.19 implements only
existing M1 operations. Below 768 px the real authorized table/view route
lazy-loads MobileCardList instead of DataGrid, with no RevoGrid engine/provider
mount. At 768 px and above the existing production grid/clipboard behavior is
preserved. The synthetic spike remains isolated.

- Cards render the display value plus the first four other visible fields via
  FieldRenderer. A hidden display field is fetched using the existing transient
  fieldIds override, without changing stored visibility. Card buttons suppress
  nested links; full-row rendering still offers validated links.
- Reuse RemoteRowsWindow, 100 rows per page, active plus adjacent pages and
  <=300 cached row objects. Only the active page is rendered. Previous/Next
  controls announce page/count and focus the heading. Page changes/refresh clear
  selection; explicit labeled checkboxes support touch and keyboard without
  long-press-only behavior.
- A bounded native bottom sheet offers reload, transient field visibility,
  the existing FieldPanel and confirmed selected-row deletion. There are no
  working-looking filter/group/highlight controls or persisted view mutations.
  The existing navigation drawer/sharing remain available independently.
- Tapping/keyboard-activating a card uses the same full-row deep link and
  RowExpandDialog. Below 768 px the dialog fills the viewport, with a sticky
  header, safe-area padding, scrolling and 44px editor/button targets.
- The write/owner FAB opens that dialog in local-draft mode. Existing codec
  editors Apply one value at a time, then Create sends one atomic row POST.
  Untouched fields are omitted so physical defaults apply; explicit nulls are
  retained. There is no auto-insert on FAB/open/field Apply or close.
  Creation is bounded to 5000 supplied cells/512 KiB; existing-row saves retain
  the 4096-byte single-PATCH limit. Failed writes do not show saved state;
  uncertain creation instructs checking the row list before retry.
- All state is memory-only. Abort/generation/identity/schema guards discard
  late reads/writes; schema changes reset the new draft with a translated
  notice. Success opens the authoritative new row URL and refreshes the bounded
  list. Readers have no FAB/selection/write controls; server ACL remains final.

T1.19 itself did not add mobile comments/history, uploads, relations/formulas,
search/filter/group/highlight builders, realtime, restore/purge or out-of-scope
features; later tasks add the supported features described elsewhere.
Locale/theme changes use static matching en/zh-CN keys and existing tokens.

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
- Highlight and select-choice colors defined as token pairs (light/dark) so data colors remain readable in both themes; AT-505 checks the active rule colors in both modes.
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
- Currency values remain fixed-scale decimal strings and are passed directly
  to `Intl.NumberFormat` for exact display (no JavaScript `Number` conversion).
  The field editor lets owners set a per-field ISO code or leave it blank to
  snapshot the app default (CNY) at creation; later setting changes do not
  change existing fields. Percent stores a fraction as `numeric(7,4)` and
  formats it with `Intl.NumberFormat` percent style.

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
