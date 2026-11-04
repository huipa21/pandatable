# Task prompt - UI-VISIBILITY Move sharing into the Visibility toolbar menu

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`, this prompt.
- `docs/12-frontend.md`: Layout and T1.12 sharing sections.
- `docs/06-permissions.md`: target ownership and sharing rules only; locate headings first.
- Search `docs/decisions.md` for D-108 through D-112, D-158, D-209.
- Search `docs/acceptance-tests.md` for AT-204 through AT-207 and the sharing acceptance definitions.

## Scope

1. On the desktop Grid toolbar, add a Visibility menu with an eye icon immediately after the current Colour/highlighter control (renamed Highlight by a separate task), before row height and overflow.
2. Move the table-page Share action from the upper utility area into this Visibility dropdown. The menu exposes the existing Share action and launches the existing sharing dialog, not a new permission editor.
3. Use the spelling "Visibility" and an appropriate zh-CN translation. The menu must be compact, keyboard/touch operable, viewport-bounded, and dismiss on outside interaction or Escape.
4. Remove the displaced table-page Share button without losing sharing access on mobile or non-Grid table views. Preserve workspace/book sharing and sidebar Share actions.

Interpretation: Visibility is the home for the existing resource-sharing action. It is not a request for public sharing, a new visibility API, or a change to personal/shared view scope.

## Contracts to respect

- Start at `web/src/pages/ResourcePage.vue`: the table utility area currently calls `sharing.open(node)`. `DataGrid.vue` owns the toolbar and currently renders `.grid-highlights` before `.grid-row-height`.
- Keep resource authorization and dialog opening in the page/sharing layer. Prefer a small named slot, `after-highlight`, rendered immediately after the highlight details block, with page-provided Visibility content. Inspect existing patterns before implementing; do not import the sharing store into the Grid.
- The new slot is only a suggested integration contract, not permission to restructure the toolbar. Preserve the existing outside-dismissal and focus behavior.
- Reuse `AppIcon`'s existing `eye` icon. Do not rename/remove the `colour` icon: it remains the highlighter symbol.
- Only effective owners of the target may Share. Parent ownership, admin status, and view edit rights must not grant sharing. Hide unavailable actions; do not expose an empty menu to non-owners unless an existing permitted action belongs there.
- Do not change directory lookups, grant review, inherited grants, last-owner safeguards, or authoritative refresh after uncertain mutations.
- Close the dropdown before opening the dialog and restore focus to a surviving opener on dialog dismissal. Handle route changes, loss of access, narrow screens, and non-Grid views.
- Do not move view-scope selectors from Views and table options or conflate view scope with resource ACL visibility.
- Use both locales; own a new `grid.visibilityMenu` namespace and reuse existing `sharing.*` labels.

## Parallel ownership

- Own `ResourcePage.vue`, relevant tests in `ResourcePage.test.ts`, and new `web/e2e/visibility-menu.spec.ts`. In `DataGrid.vue`, own only the new slot insertion between highlight and row-height controls.
- The Highlight task owns the highlight block, its label, rule logic, styles, and tests. Do not edit that block or rename Colour yourself. The placement must work before and after the independent rename.
- The Fields task owns field components/CSS. Avoid broad shared-file formatting. Re-read locale/docs before surgical edits and serialize shared writes.
- Do not run gates/builds/commits concurrently in one checkout. Use separate working copies for fully independent parallel execution, without introducing branches/worktrees against repository policy. Preserve all unrelated dirty changes.

## Verification (D-158)

- Focused loop: `pnpm --dir web exec vitest run src/pages/ResourcePage.test.ts` and directly affected sharing tests.
- Browser assertions: actual toolbar order is Highlight/Colour, Visibility, row height; eye icon and accessible name are present; the old table utility Share button is absent; the existing dialog opens from Visibility; outside/Escape dismissal and focus return work.
- Verify target owner vs non-owner/admin-only access, narrow/mobile and non-Grid access, and unchanged book/workspace/sidebar sharing. Check both locales and themes.
- Task gate after shared changes stabilize: `go run .\tools\ptdev check`; `go run .\tools\ptdev e2e --fast -- e2e/visibility-menu.spec.ts e2e/shell.spec.ts`. Add the existing sharing acceptance spec if discovered; `web/e2e/sharing.ts` is a helper, not a spec. Preflight Podman/mirrors; do not run the full production-image suite per task.

## Wrap-up

- Update the frontend sharing/layout documentation and only acceptance evidence actually exercised. Do not claim all AT-204..207 passed from toolbar-only tests.
- Follow `docs/task-prompts/TEMPLATE.md`: progress row/change-log line, decision for a non-trivial choice, and a record appended here (about 2 KB).
- Commit only this task's changes after gates pass, using Conventional Commits and the required co-author trailer. Serialize shared-checkout commits and never stage other sessions' work.

## Record (2026-11-04)

- Outcome: Done (D-214). New `components/sharing/VisibilityMenu.vue` (eye icon,
  `grid.visibilityMenu.*` en/zh-CN, existing `sharing.open` label) launches the
  existing sharing dialog. `ResourcePage.vue` renders it through DataGrid's new
  `after-highlight` slot on desktop Grid views and in `.table-utility` on mobile
  and non-Grid views; the old utility Share button is gone. Visibility is gated by
  the unchanged `canShare` (effective target owner). Workspace/book page and
  sidebar Share actions are unchanged.
- Menu closes before the dialog opens and refocuses its summary when the dialog
  closes; outside pointer and Escape dismiss it. DataGrid's capture keydown
  handler now ignores `summary` targets, so Enter/Space open toolbar menus from
  the keyboard (previously swallowed for every toolbar menu).
- Tests: `ResourcePage.test.ts` +2 (order/icon/name, Escape/outside/focus
  return, mobile/non-owner/zh-CN); new `e2e/visibility-menu.spec.ts` (toolbar
  order, keyboard, viewport bounds, dialog + focus return, zh-CN/dark, 390 px and
  form view, write-level admin without ownership sees no Share). `e2e/sharing.ts`
  `open()` uses the Visibility menu on table paths.
- Gates on HEAD + this change only (temporary clone, other sessions' WIP
  excluded): `ptdev check` PASS (707/707 vitest, lint, build);
  `ptdev e2e --fast -- e2e/visibility-menu.spec.ts e2e/shell.spec.ts` 6 passed,
  1 skipped (mobile project; the spec covers 390 px itself).
- Open risk: `e2e/admin-users.spec.ts` (which runs `verifySharing`) times out at
  HEAD in `e2e/fields.ts:230` ("Save field" in the New field dialog), before the
  sharing steps; the updated `sharing.ts` helper is therefore unexercised by it.
  Only toolbar/dialog sharing access is evidenced here; AT-204..207 are not
  re-claimed.
- Next: rerun admin-users once the field-creation e2e helper is fixed.
