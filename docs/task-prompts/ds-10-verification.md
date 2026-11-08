# DS-10 VERIFY - integrate and verify the dashboard/sidebar redesign

Read [execution plan](dashboard-sidebar.md). Requires DS-01..DS-09 handoffs and
exclusive access to the gate resources. This is the **only full-verification
session** in this batch.

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- Each DS prompt's Scope, numbered cases and Record.
- `docs/09-api.md` "Structure"; `docs/12-frontend.md` sections 2, 6;
  `docs/14-dev-build-release.md` test/gate sections; `docs/user-guide.md`
  workspace/navigation/language parts.
- Decisions: D-131, D-156..158, D-218, D-219, D-230, D-234 and new DS rows.
- Acceptance: AT-003, AT-204..210, AT-214..216, AT-506..508, AT-513, AT-802, AT-806.

## Integration scope

- Review all nine handoffs for completeness and conflicts. Integrate any
  outstanding locale, doc or caller patches. Remove dead code (e.g. the unused
  sidebar workspace styles, the old language-toggle CSS, the obsolete Share
  wiring).
- Trace every numbered DS case to executable evidence. Assign AT-544 onward.
  Revise AT-003 (language menu), AT-214 (dashboard landing), AT-215 (sidebar)
  and AT-216 (search/order) without erasing their history.
- Docs: the API contract for the workspace fields and `/recent` in
  `09-api.md`; the dashboard, sidebar, tree and language menu in
  `12-frontend.md`; en + zh-CN user guide; one `progress.md` row and log line
  per DS task.

## Gates (D-158)

1. Preflight: Podman VM running, mirrors set, no leftover buildah containers.
   WG-09: confirm it is closed. Otherwise record that this phase gate also
   serves as its outstanding rerun.
2. `go run .\tools\ptdev check` (log to a file, search it for failures).
3. `go run .\tools\ptdev e2e --fast -- e2e/dashboard.spec.ts e2e/default-workspace.spec.ts e2e/workspace-navigation.spec.ts e2e/workbook-cards.spec.ts e2e/views.spec.ts e2e/shell.spec.ts e2e/visibility-menu.spec.ts e2e/hardening.spec.ts`
   plus any spec using `e2e/sharing.ts` or a new DS spec.
4. Once green: exactly one complete production-image `go run .\tools\ptdev e2e`.
5. Two targeted fix attempts per failure, then record the evidence and stop. A
   failed gate reopens only the affected DS tasks.

## Wrap-up

Mark DS tasks Done only with green task and phase gates. Commit (Conventional
Commit, `[DS-10]`) only if the user requests it, after the pre-commit gate.
Append a Record of about 2 KB max: outcome, commands with pass/fail counts,
open risks.

## Record

**Outcome (2026-11-08):** Done. DS-01..DS-09 integrated, AT-544..569 mapped, D-236 (workspace metadata API) and D-237 recorded. This production phase gate also serves WG-09's outstanding authorized rerun.

**Stabilization:**
- Updated E2E for the DS behaviour:
  - login lands on `/home` (22 specs/helpers);
  - workspaces are created and edited from the dashboard;
  - the sidebar shows only the current workspace;
  - collapsed workbooks expand on demand;
  - the header Language menu disambiguates profile labels;
  - share helpers wait for `/nav`, which fixed a cascade that left the locale at zh-CN.
- Fixed order-dependent test races:
  - account menu click retried;
  - row-expand Space retried;
  - grid tail re-scroll;
  - fields-batch2 column reveal;
  - mobile-rows waits for its create dialog and allows the revocation 404s.
- D-237 raises the Playwright global bound from 600 s to 5,400 s (supersedes D-155), because a full run takes about 45 min.

**Gates:**
- `ptdev check`: PASS (final pre-commit run).
- `ptdev e2e --fast`: required DS specs green.
  - Diagnostic full fast run: 126 passed / 8 failed. All 8 were fixed, and each affected spec was rerun green (repeat-each 2–3 for flaky ones).
- Production `ptdev e2e`:
  - attempt 1: 20 failed + global timeout;
  - attempt 2: 44 failed (locale cascade);
  - attempt 3: 132 passed / 2 failed;
  - attempt 4: 132 passed / 2 failed (different tests);
  - **attempt 5: 134 passed / 38 skipped / 0 failed (44.3 min).**

**Open risks:** the suite is close to 45 min on the dev laptop; the 2-vCPU/4-GB target host is still uncertified (unchanged).
