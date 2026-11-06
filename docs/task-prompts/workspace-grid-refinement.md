# Workspace/grid refinement - execution plan

Planning only. Run WG-01..WG-09 in fresh sessions; do not repeat UX-01..UX-11.
Read this index, then the selected prompt. No application changes were made
when this plan was written.

## Requested outcome / execution assumptions

1. Workspace browsing: show **every accessible workspace**, in server order,
   as a flat sidebar list. No workbook/table/view expansion, book counts or
   empty-book messages inside those workspace entries. Selecting a workspace
   opens its existing responsive workbook-card grid (image 2).
2. Workbook browsing: a card opens the existing workbook route. Keep the
   workspace section flat; show a **separate current-workbook tree** containing
   that workbook, its tables and their views. Do not mount trees for other
   workbooks/workspaces. Table/view/row deep links resolve the same context.
   This is the interpretation of "workbook view shows nested structure";
   it is not a second all-workspace tree.
3. The append (+) action is one full-width row immediately after the displayed
   data, not a button covering only the first cell (image 3). Preserve its
   existing blank-row mutation, validation, undo and end-of-data behavior.
4. Each visible data column gets an aligned bottom summary cell, with its own
   type-aware aggregate picker (image 4). Include Count, Empty, Filled, Unique,
   Percent Empty, Percent Filled and Percent Unique where supported, plus
   existing numeric/date/checkbox operations. Results cover the whole matching
   dataset, not just loaded rows. "None" clears a summary.
5. Remove the **table toolbar's** ellipsis menu and all controls inside it
   (image 5), not field/table/sidebar context menus. Relocate, do not delete,
   useful functions: search to a dedicated search affordance; dimensions to
   Format; ordering/freezing to Format and visibility to existing Fields;
   aggregates to column footers; row actions to the selection action bar.
6. Replace row-height-only control with **Format**, offering column widths and
   row heights, retaining existing persisted/local and locked-view semantics.
7. Selecting rows reveals **Delete** and **Edit** plus the count. Delete removes
   exactly all selected IDs after confirmation. Edit chooses a writable field
   and assigns one typed value to that field on every selected row; repeat to
   change another field. This is not a full multi-record form or cell-range
   fill-down. Preserve existing atomic API limits; show explicit limit errors,
   never silently truncate or partly process a selection.
8. Remove visible "Display field: Name" and "Your access: Owner" metadata from
   table/view toolbars. Keep their schema/permission behavior internally.
   Replace separate Import/Export triggers with one accessible icon trigger
   containing Import, Export CSV and Export workbook, subject to existing rules.
9. Images are visual references, not requests for NocoDB's plans, teams,
   integrations, audits or settings. Preserve PandaTable branding and scope.
   These descriptions are sufficient without access to attachment snapshots.

## Tasks

| ID / short name | Bounded deliverable | Prerequisites | Prompt |
|---|---|---|---|
| WG-01 FLAT | Flat workspace sidebar; reuse/verify workbook cards | None | [01](wg-01-flat-workspaces.md) |
| WG-02 BOOK | Active-workbook/table/view contextual tree | WG-01 | [02](wg-02-workbook-tree.md) |
| WG-03 ADD | Full-width append row | None | [03](wg-03-append-row.md) |
| WG-04 AGG | Missing percentage aggregate contracts/engine | None | [04](wg-04-aggregate-contracts.md) |
| WG-05 FOOT | Column-aligned summary cells and pickers | WG-03, WG-04 | [05](wg-05-column-footers.md) |
| WG-06 FMT | Format, search relocation, remove toolbar ellipsis | WG-05 | [06](wg-06-format-toolbar.md) |
| WG-07 BULK | Visible selection actions; typed bulk edit/delete | WG-06 | [07](wg-07-bulk-actions.md) |
| WG-08 XFER | One transfer icon; remove toolbar metadata | None | [08](wg-08-transfer-toolbar.md) |
| WG-09 VERIFY | Integration, acceptance mapping and full gates | WG-01..WG-08 | [09](wg-09-verification.md) |

WG-05 follows WG-03 because both change grid footer geometry. WG-06 follows
WG-05 so it removes the aggregate settings only after the replacement exists.
WG-07 follows WG-06 so removal of the old hidden Delete precedes its replacement.
All four edit DataGrid; do not run them together. WG-04 owns field/API aggregate
contracts, not DataGrid. WG-08 owns the separate transfer component, not its
ResourcePage callers.

## Safe launch waves

| Wave | Sessions that can run in parallel | Barrier / coordinator |
|---|---|---|
| 1 | **WG-01 + WG-03 + WG-04 + WG-08** | WG-01 coordinates shared-file handoffs |
| 2 | **WG-02 + WG-05** | All wave-1 source/locale handoffs integrated; WG-02 coordinates |
| 3 | **WG-06** | Wave 2 integrated |
| 4 | **WG-07** | WG-06 integrated |
| 5 | **WG-09** | Every implementation handoff complete; no concurrent writers |

Parallel means disjoint implementation and confined unit tests, **not**
simultaneous commits, shared-file edits, DB gates or builds. Use these
conservative barriers even if an individual prerequisite finishes early.

## Shared-worktree and quick-task protocol

- D-131 still applies: single `master`, no branches/worktrees or automatic
  commits. Inspect `git status`; preserve peer/unrelated changes. Never stage
  the entire worktree or discard changes. Only WG-09 may commit, and only if
  requested, after the normal pre-commit gate.
- Stay inside each prompt's source/test ownership. A needed peer-file change
  becomes an exact handoff in your own Record, not an overlapping edit.
- One coordinator writes shared files at a time: both locale JSON files,
  AppIcon, global styles, shared helpers, progress, decisions, acceptance,
  design docs and user guide. Peers put exact proposed additions/patches in
  their own Record. After source writers stop, the coordinator applies these
  serially and records completion. Use existing translations first; new keys
  use `workspaceGrid.wgNN`. Both locales must be integrated before a wave closes.
- Coordinators record one progress row and concise start/handoff log lines per
  task, plus necessary decision/design updates. Detailed evidence belongs only
  in the task's Record (about 1-2 KB); do not duplicate it in central docs.
- WG-01..WG-08 run only their listed focused tests and touched-file lint.
  Add/update their focused E2E cases, but execution can wait for WG-09. No
  `ptdev check`, broad suites, production build or complete `ptdev e2e` in
  these sessions. No intermediate commits: D-158's task gates are **deferred,
  not waived**, and these tasks remain **implemented, awaiting WG-09 gates**,
  never Done based on unit tests alone.
- The final session runs the consolidated required task gates and exactly one
  complete production-image phase gate. A failed gate leaves affected tasks
  open. Existing UX-11 AT-305/311 and AT-705 failures are not assumed resolved.
- No new dependencies, migrations, scope expansion or test-harness relaxations
  are expected. If a substantial new contract is needed, record the reason and
  bounded next step rather than inventing a broad redesign.
- Two targeted fix attempts per same failure, then report evidence/change
  approach. Do not loop on flaky gates.

## Acceptance and finish

Each prompt's numbered cases are new obligations. Existing linked AT IDs are
regressions, not evidence that these changes already pass. WG-09 assigns unused
AT IDs to the new cases, updates superseded criteria without erasing historical
evidence, and maps every case to executable evidence. In particular, AT-215's
workspace expansion and AT-327's metadata/separate transfer controls must change.
WG-09 must test actual geometry in a browser, not only CSS classes or snapshots.

## Planning record (2026-11-06)

- Inspected current navigation, workbook cards, DataGrid, transfer toolbar,
  aggregate capabilities/API and test layout.
- Cards, blank-row creation, bulk DELETE and atomic bulk PATCH already exist.
  Missing percentage operations require a bounded contract/engine task.
- Added nine execution prompts with disjoint wave ownership and deferred gates.
- This is a new follow-up; prior UX phase failures remain open.
