# UX-10 - Rightmost New Entry opens the creation form

Requires UX-06 and UX-09. Run independently of other ResourcePage/header writers.

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- [Execution index](workspace-grid-redesign.md), UX-06's action-slot handoff.
- `docs/12-frontend.md` T1.18 row expansion and T1.19 mobile grid;
  `docs/09-api.md` Rows & queries.
- Search decisions D-140, D-141, D-211, D-217.
- Search acceptance AT-204, AT-205, AT-303, AT-310, AT-313, AT-320.

## Scope

Wire the rightmost table-header New Entry button to the existing creation form.

- In table Grid view, New Entry is the rightmost action in UX-06's inner content
  header. Use the existing schema-aware RowExpandDialog/new-row mode.
- The current ResourcePage already sets `newRow = true` from MobileCardList and
  renders RowExpandDialog. Reuse that path; no separate public form or saved Form
  view creation is needed. A table's saved Form view remains a different feature.
- Writer/owner can open and submit; readers cannot create. Required/type/default/
  unique rules and supported complex field editors remain authoritative.
- After successful creation, refresh visible rows through existing hooks and
  retain current query semantics. Preserve the existing created-row modal/deep
  link behavior and accessible focus return.

## Ownership and boundaries

Own ResourcePage's New Entry wiring, small header action additions and creation
dialog tests. Change RowExpandDialog only if a concrete integration gap requires
it. No DataGrid editor/plus modifications, backend changes, or view CRUD changes.

Keep Cancel/Escape safe: no create request and no accidental route/view switch.
During save prevent duplicate submissions. A rejected request leaves fields
editable with visible errors. Preserve omitted server defaults rather than
turning every untouched input into an explicit null. Do not reuse per-cell
commit-on-blur to submit the whole creation form.
At 390 px do not duplicate/conflict with mobile's existing add action.

## Acceptance scenarios

1. New Entry is rightmost in the inner header. Click opens a blank schema-aware
   creation form, not a saved-view dialog and not an immediate row POST.
2. Enter required text, checkbox/select/list and other representative supported
   fields; submit creates one row with exact values and omitted server defaults.
   Reload confirms persistence; cancel creates nothing.
3. Required/unique/network errors remain visible in the form. Repeated submit
   while saving does not create duplicates; read-only users are denied.
4. Creation refreshes filtered/sorted/grouped views correctly even when the new
   row is excluded. No fabricated visible row or lost active view configuration.
5. Route/identity changes, focus return, keyboard/Escape, both languages/themes,
   desktop and mobile remain correct. Existing row expansion/deep links work.
6. Exercise both creation paths: plus creates directly and New Entry opens a
   form. Neither is accidentally wired to the other's handler.

## Verification and wrap-up

Focused ResourcePage/RowExpandDialog/mobile tests; add
`web/e2e/new-entry.spec.ts`, preserving `row-expand.spec.ts` and mobile coverage.
Gates/docs/commits follow the execution index. Leave a complete handoff for UX-11.

## Record

Not started.
