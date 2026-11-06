# Workspace and grid redesign - execution index

Status: UX-01..UX-10 implemented and integrated; UX-11 assigned acceptance IDs
and attempted phase validation with an extended timeout. The canonical phase
gate remains open (see its [Record](workspace-grid-11-integration.md#record)).
Run one numbered prompt per session. Read this index before its task prompt.

## Requested outcome and interpretations

- Replace the sidebar workspace placeholder with a static **Workspaces** heading,
  a right-aligned Add (+) button, and every accessible workspace beneath it.
  Keep the book/table/view hierarchy under its workspace; distinguish selection
  from expansion. Workspace rename and creation remain available.
- A new user's starter workspace is initially named **Default**, but can be
  renamed. "Default" is an initial name, not a reserved name or a name that must
  be recreated after rename. The preferred startup workspace is **per user**,
  persisted server-side, and can be changed to another accessible workspace.
  Existing users are bootstrapped without changing their existing data.
- Opening the app, logging in, or visiting `/home` opens the user's preferred
  workspace. Explicit workspace/book/table/view/row links still win.
- Selecting a workspace shows its accessible workbooks as cards in a responsive
  grid. A user-set workbook icon wins; otherwise use a stable randomly assigned
  icon from the existing local icon set, not a new icon on every render.
- Remove the home welcome title, signed-in-as text, organization instruction,
  workspace chooser and content-area New Workspace button. Keep the avatar menu.
- The outer content header starts with the workspace name immediately to the
  right of the sidebar toggle; retain useful book/table/view context.
  The inner table header contains the existing table tools plus Import/Export
  and, at its right edge, New Entry. Keep one inner tools row rather than an
  additional mostly empty band.
- Remove **Views and table options** completely, only after moving its useful
  actions. Visibility remains the existing ACL-sharing menu (D-214), not field
  visibility or personal/shared view scope. Fields still controls columns.
- Create views from the table's sidebar ellipsis menu. List visible views under
  that table; retain existing view-management capabilities in contextual menus.
  Deleting a table makes all its views unavailable, including direct links.
  Preserve Trash/restore/purge; this is not a request for immediate hard deletion.
- Grid text/scalar cells can enter editing on a single click and commit valid
  drafts when focus leaves the editor. Double-click editing remains supported.
  Checkbox toggles on one click; single/multi-select and tags open options on
  one click. Preserve keyboard selection, range operations and read-only fields.
- The small plus directly after the last row creates one row without a form.
  New Entry opens the existing creation form. Both use authoritative validation.
  A blank-row request can legitimately fail for required fields without defaults;
  show the error and offer New Entry, never fabricate required data.

These are execution assumptions for unspecified details, not extra features.
Do not add public sharing, a new tags field type, custom icon uploads, external
services, new view types, or unrelated backend redesigns.

## Tasks and dependencies

| ID | One primary outcome | Requires | Prompt |
|---|---|---|---|
| UX-01 | Persistent default-workspace lifecycle/API | None | [01](workspace-grid-01-default-workspace.md) |
| UX-02 | Workspace sidebar and default landing navigation | UX-01 | [02](workspace-grid-02-workspace-navigation.md) |
| UX-03 | Workbook icon metadata and stable random fallback | UX-01 | [03](workspace-grid-03-workbook-icons.md) |
| UX-04 | Workspace workbook-card grid | UX-02, UX-03 | [04](workspace-grid-04-workbook-cards.md) |
| UX-05 | Sidebar view creation and contextual view actions | UX-04 | [05](workspace-grid-05-view-navigation.md) |
| UX-06 | Consolidated table header; remove options panel | UX-05 | [06](workspace-grid-06-table-header.md) |
| UX-07 | Single-click scalar editing and commit on leave | None | [07](workspace-grid-07-inline-editing.md) |
| UX-08 | One-click checkbox/select/tag interactions | UX-07 | [08](workspace-grid-08-choice-editing.md) |
| UX-09 | Plus action directly after final grid row | UX-08 | [09](workspace-grid-09-append-row.md) |
| UX-10 | Rightmost New Entry form action | UX-06, UX-09 | [10](workspace-grid-10-new-entry.md) |
| UX-11 | Combined acceptance, documentation and phase gate | UX-10 | [11](workspace-grid-11-integration.md) |

UX-03 follows UX-01 because both can change schema/API contracts and migrations.
UX-04 waits for UX-02 because both can touch navigation mapping.
UX-04/05/06/10 are serialized because they touch ResourcePage. UX-07/08/09
are serialized because they share DataGrid and its editor lifecycle.
UX-10 waits for UX-09 so the two creation paths are verified together.
All dependencies, including UX-07/08, reach UX-11 transitively.

## Safe launch waves

| Wave | Run simultaneously | Start only after |
|---|---|---|
| 1 | UX-01 + UX-07 | Nothing |
| 2 | UX-02 + UX-03 + UX-08 | Wave 1 implementation handoffs |
| 3 | UX-04 + UX-09 | Wave 2 implementation handoffs |
| 4 | UX-05 alone | UX-04 |
| 5 | UX-06 alone | UX-05 |
| 6 | UX-10 alone | UX-06 and UX-09 |
| 7 | UX-11 alone | Every implementation handoff |

"Alone" means no other task from this batch is editing shared surfaces.
Wave barriers are conservative; do not parallelize tasks merely because their
feature names sound unrelated. Parallel means implementation and focused unit
checks, **not concurrent commits, generators, DB gates or image builds**.

## Shared-worktree rules (binding for these prompts)

The repository requires linear `master` history (D-131). Do not create branches
or worktrees to evade that rule. Parallel sessions share the working tree:

1. Use only the source/test ownership named in your prompt. Inspect the worktree
   first; never discard peer changes. New tests should have task-specific files
   where feasible. Do not edit a peer-owned source file; hand off the required
   contract and wait for its owner.
2. Before starting a wave, designate one session as its coordinator. That session
   is the only writer to `en.json`, `zh-CN.json`, global styles, shared test helpers,
   and central docs (`progress`, `decisions`, acceptance, design docs, user guide).
   Other sessions put exact proposed additions in their own prompt's Record.
   The coordinator integrates them serially; no one calls a task Done with
   missing translations. Use disjoint locale namespaces `workspaceGrid.uxNN`.
   Reuse existing keys when their meaning already matches.
3. Migration/sqlc/API contract edits belong to UX-01, then UX-03. Do not run
   schema generators concurrently. UX-02 consumes UX-01's contract; grid tasks
   must not change shared API types.
4. Each task records its start and implement/stabilize handoff in its own Record.
   The coordinator updates one central progress row and one start/finish log
   line per task, without duplicating detailed evidence.
5. Parallel sessions run focused unit checks only. They leave the status
   **implemented, awaiting integration gates**, not Done. The coordinator alone
   runs required PG18 checks, `ptdev check`, affected fast E2E and commits, with
   other writers stopped. No `git add .`, reset, clean or mixed peer commits.
6. Either serialize each task's stabilization/commit, or use the conservative
   default: integrate the complete batch in UX-11, run one full task-gate batch
   on the combined state, record per-task scenarios, then make the coordinated
   Conventional Commit(s) on `master` with task IDs and the required trailer.
   Do not claim independently verified intermediate commits.

## Verification and completion

Each prompt's numbered scenarios are new acceptance obligations; linked AT IDs
are existing regression obligations, not proof that the new UX already passes.
UX-11 assigns unused AT IDs for the new scenarios in the central acceptance doc.

Inner loop: focused Vitest; focused Go tests only with the coordinator's DB slot.
Before long gates follow the engine/mirror preflight in `AGENTS.md`, log output,
and respect the two-attempt retry budget. Task gate:
`go run .\tools\ptdev check` plus
`go run .\tools\ptdev e2e --fast -- <affected specs>`.
Paths passed to the Go/Node tools may follow their CLI conventions.
Do not run the full production-image suite per task. UX-11 runs it once at this
redesign's phase end (D-158); failures reopen affected tasks.

Wrap-up for every task: record outcome, changed files, commands/counts, risks,
and next handoff in its own prompt (about 1-2 KB). Central docs/commits are
serialized as above. Keep existing unrelated milestone failures visible.
