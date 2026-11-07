# Dashboard/sidebar redesign - execution plan (DS-01..DS-10)

Planning only. Run DS-01..DS-10 in fresh sessions. Read this index, then the
selected prompt. No application changes were made when this plan was written.

## Requested outcome / execution assumptions

1. **Workspace dashboard (`/home`).** The content area has a full-width search
   bar. Below it, every accessible workspace is one row: icon (custom, or
   a stable random fallback), name, then description. Without a query, rows
   are ordered by the caller's most recently opened workspace first. Never-opened
   workspaces follow: the preferred/default workspace first, then server order.
   `/home` **stops auto-redirecting** to the preferred workspace (supersedes that
   part of D-218/AT-214). The starter `Default` workspace and default marker stay.
2. **Create workspace** asks for Name (required), Icon (optional picker; if
   none, the server assigns a stable random fallback like books, D-219), and
   Description (optional, plain text). Owners can edit all three later.
3. **Sidebar** no longer lists all workspaces. It shows a "Workspaces" link back
   to the dashboard, the current workspace (when in context) and, on
   workbook/table/view/row routes, the active-workbook tree (D-230). Workspace
   actions (new, edit, share, set default, delete) move to dashboard rows.
4. **Workspace page** shows its workbooks with the same search bar and the same
   recency/search ordering. Existing workbook cards (AT-216) are kept; only
   search and ordering are added.
5. **Workbook tree collapsed by default.** The workbook node and every table
   start collapsed. Only the ancestors of the current table/view route
   auto-expand so the active item stays visible. Manual expansion survives
   navigation inside the same workbook during the session.
6. **Workbook row split.** The chevron alone expands/collapses. Clicking the
   workbook icon or name navigates to the workbook page. Tables already behave
   this way and are unchanged.
7. **No content-area Share button** on workspace/workbook pages (image 2).
   Sharing remains in the resource action menus (sidebar for workbooks/tables,
   dashboard rows for workspaces) and in the table toolbar visibility menu.
8. **Language selector** is a borderless translate icon (文A glyph) in the
   header. Clicking it opens a menu listing each supported locale by native
   name, with its localized name and a check on the current one (image 3).
   With two locales (en, zh-CN), no language search box is added.
9. Search is client-side over the already-authorized `/nav` data. It is
   case-insensitive and Unicode-normalized, and matches name and description.
   It adds no new search API and never reveals hidden resources. Ranking: name
   prefix, then name contains, then description contains; ties use the
   no-query order.
10. Images are visual references only. Keep PandaTable branding and scope
    (`01-requirements.md` Out of Scope). Workbooks get no description field.

## Tasks

| ID / short name | Bounded deliverable | Prerequisites | Prompt |
|---|---|---|---|
| DS-01 WSMETA | Workspace description + random fallback icon API, nav fields, store payloads | None | [01](ds-01-workspace-metadata.md) |
| DS-02 RECENT | Per-user recently-opened workspaces/workbooks API, record hook, shared search/order composable | DS-01 | [02](ds-02-recent-resources.md) |
| DS-03 LANG | Borderless translate-icon language menu | None | [03](ds-03-language-menu.md) |
| DS-04 TREE | Workbook row split (chevron vs. link), collapsed-by-default tree | None | [04](ds-04-workbook-tree-toggle.md) |
| DS-05 NOSHARE | Remove content-area Share button; re-route tests to action menus | None | [05](ds-05-remove-content-share.md) |
| DS-06 WSFORM | Workspace create/edit dialog with icon picker and description | DS-01 | [06](ds-06-workspace-dialog.md) |
| DS-07 DASH | `/home` workspace dashboard: search, rows, recency, row actions | DS-02, DS-06 | [07](ds-07-workspace-dashboard.md) |
| DS-08 BOOKS | Workbook search + recency ordering on the workspace page | DS-02, DS-05 | [08](ds-08-workbook-search.md) |
| DS-09 SIDE | Remove the workspace list from the sidebar; dashboard link and breadcrumbs | DS-04, DS-06 | [09](ds-09-sidebar-context.md) |
| DS-10 VERIFY | Integration, AT mapping, docs, consolidated and phase gates | DS-01..DS-09 | [10](ds-10-verification.md) |

### Subtask map (request → task)

| Request | Subtasks (owner) |
|---|---|
| 1 Dashboard | migration/DTO (01); recents table, API, nav `lastOpenedAt` (02); `useResourceSearch` (02); dashboard page, rows, empty/no-match states, row actions, landing change (07) |
| 2 Create workspace | API validation/fallback (01); `IconPicker` + dialog fields + edit mode (06) |
| 3 Sidebar | remove list, add dashboard link, breadcrumb root (09); relocate actions (07) |
| 4 Workbook search | composable reuse, search bar, ordering, record open (08 + 02 hook) |
| 5 Collapsed tree / 6 split row | NavNode state and controls (04) |
| 7 Share removal | ResourcePage button, `canShare` cleanup, E2E helpers (05); workspace share via dashboard (07) |
| 8 Language | `language` AppIcon, `LanguageMenu.vue`, AppLayout wiring, locale keys (03) |
| Acceptance/docs | AT-544+ assignment, AT-214/215/216 revisions, 09/12 docs, user guide (10) |

## Safe launch waves

| Wave | Sessions that can run in parallel | Barrier / coordinator |
|---|---|---|
| 1 | **DS-01 ∥ DS-03 ∥ DS-04 ∥ DS-05** | DS-03 coordinates locales, AppIcon and central docs |
| 2 | **DS-02 ∥ DS-06** | Wave 1 integrated; DS-06 coordinates |
| 3 | **DS-07 ∥ DS-08 ∥ DS-09** | Wave 2 integrated; DS-07 coordinates |
| 4 | **DS-10** | All handoffs complete; no concurrent writers |

```
W1                 W2                 W3                W4
DS-01 WSMETA ──┬─► DS-02 RECENT ──┬─► DS-07 DASH ──┐
               └─► DS-06 WSFORM ──┼─► DS-07, DS-09 │
DS-04 TREE ───────────────────────┼─► DS-09 SIDE ──┼─► DS-10 VERIFY
DS-05 NOSHARE ────────────────────┴─► DS-08 BOOKS ─┤   (DS-08 also needs DS-02)
DS-03 LANG ────────────────────────────────────────┘
```

### Why the barriers

- DS-01 and DS-02 both add migrations, `queries.sql`, sqlc output, `nav.go`
  and `stores/nav.ts`, so they are serialized (0009, then 0010).
- DS-06 needs DS-01's API fields. It owns `StructureDialog.vue`,
  `NavActions.vue` and new `IconPicker.vue`, and does not edit `stores/nav.ts`.
- DS-07, DS-08 and DS-09 own disjoint pages (`HomePage.vue`, `ResourcePage.vue`
  and `NavContents.vue`/`AppBreadcrumbs.vue`). All three consume DS-02's
  composable and DS-06's dialog.
- DS-05 releases `ResourcePage.vue` before DS-08. DS-04 releases `NavNode.vue`
  and `navigation.test.ts` before DS-09.
- DS-03 owns `AppLayout.vue` in wave 1. DS-02 adds its record hook there in wave 2.

### Shared-file ownership by wave

| File | W1 | W2 | W3 |
|---|---|---|---|
| `migrations/`, `internal/structure/*`, sqlc | DS-01 | DS-02 | — |
| `web/src/stores/nav.ts` | DS-01 | DS-02 | — |
| `layouts/AppLayout.vue`, `AppIcon.vue` | DS-03 | DS-02 (hook only) | — |
| `NavNode.vue`, `navigation.test.ts` | DS-04 | — | DS-09 |
| `ResourcePage.vue`, `e2e/sharing.ts` | DS-05 | — | DS-08 / DS-07 (`sharing.ts` workspace target) |
| `StructureDialog.vue`, `NavActions.vue` | — | DS-06 | — |
| `HomePage.vue`, `default-workspace.spec.ts` | — | — | DS-07 |
| `en.json`/`zh-CN.json`, progress, decisions | DS-03 | DS-06 | DS-07 |

## Shared-worktree and quick-task protocol

Same as the [WG plan](workspace-grid-refinement.md) protocol (D-131/D-158):

- Single `master`, no branches or automatic commits. Preserve peer changes.
  Only DS-10 may commit, and only if the user asks.
- Stay inside your ownership. Write a needed peer-file change as an exact
  handoff in your own Record. The wave coordinator applies locale, doc and
  progress patches serially. New locale keys use `dashboard.dsNN.*`.
- DS-01..DS-09 run only focused Go/Vitest tests, touched-file lint and
  `i18n:check`. They add or update focused E2E cases, which DS-10 runs.
  DS-01/DS-02 also run their focused PG18 tests (explicit DSN,
  `PT_TEST_DATABASE_REQUIRED=1`). Each ends **implemented, awaiting DS-10
  gates**, not Done.
- No new dependencies. Migrations are limited to DS-01 (0009) and DS-02 (0010).
- Two targeted fix attempts per failure, then record the evidence and stop.
- WG-09 still needs one authorized production-phase rerun. DS-10 must either
  start after WG-09 closes or record that its single phase gate also covers
  that rerun.

## Acceptance and finish

The numbered cases in each prompt are new obligations. DS-10 assigns AT-544
onward and revises AT-214 (landing), AT-215 (sidebar) and AT-216 (search and
order on cards) without erasing their history. It updates `09-api.md`,
`12-frontend.md` and the user guide (en + zh-CN).

## Planning record (2026-11-07)

- Inspected the workspace schema (has `icon`, lacks description and fallback),
  book fallback icons (0008), nav DTO/store, NavContents/NavNode, the
  ResourcePage Share button, the AppLayout language toggle and the E2E helpers
  that depend on the Share button.
- Added ten bounded prompts in four waves with disjoint ownership. Decision D-234.
