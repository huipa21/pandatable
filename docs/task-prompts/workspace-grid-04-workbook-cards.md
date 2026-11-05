# UX-04 - Workbook cards for the selected workspace

Requires UX-02 and UX-03. Can run alongside UX-09.

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- [Execution index](workspace-grid-redesign.md), UX-03's API/icon handoff.
- `docs/12-frontend.md` section 2 and T1.11 navigation.
- Search decisions D-104, D-105, D-209, D-217.
- Search acceptance AT-201, AT-204, AT-208, AT-802, AT-806.

## Scope

Change only the selected workspace's child-book presentation to a card grid.

- `/w/:workspaceId` shows its accessible workbooks in a responsive grid of
  clickable cards with workbook name and UX-03's effective icon.
- Use the existing book route when opening a card. Distinguish workbook cards
  from table rows/cards and the old workspace chooser.
- Preserve create-book and workspace-share actions with existing permissions;
  keep empty, loading, stale, not-found and path-only states honest.
- No extra welcome copy, workspace chooser or content-area New Workspace button.
  Do not remove the ability to create the first workbook.

## Ownership and boundaries

Own ResourcePage's **non-table workspace branch**, a small workbook-card
component if useful, tests and scoped card styles. Map icon DTOs into navigation
in `stores/nav.ts` if needed now that UX-02 is finished.
Do not change table rendering, AppLayout/header, book metadata APIs or DataGrid.
UX-09 owns grid source/tests in this wave. Shared locales/CSS/docs follow the
coordinator rule; prefer scoped card CSS.

Preserve server ordering and access-filtered children. Path-only books may show
safe navigation context but must not acquire rename/create/share powers.
Use real links, focus indicators, accessible names and ellipsis/wrapping for
long Chinese or Latin names. No heavy grid/card library.

## Acceptance scenarios

1. Select workspaces with zero, one and several books. Correct cards appear;
   switching workspaces never leaves cards from the old workspace behind.
2. Cards navigate to the correct book; custom and fallback icons survive reload.
   Long names do not overlap neighboring cards.
3. Only accessible books are shown. Context-only ancestors remain labeled;
   readers cannot see owner mutation controls.
4. First-book creation and workspace sharing remain reachable for the authorized
   user; failed nav refresh remains visibly stale with retry.
5. At desktop/tablet/390 px the grid adapts without horizontal page overflow,
   and works by keyboard, in both languages and light/dark themes.

## Verification and wrap-up

Focused ResourcePage/card tests; add `web/e2e/workbook-cards.spec.ts` using real
workspace/book APIs, including UX-03 icon scenarios. Gates, central docs and
commits follow the execution index. Hand off ResourcePage to UX-05 only after
editing has stopped.

## Record

Not started.
