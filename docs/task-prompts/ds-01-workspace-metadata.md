# DS-01 WSMETA - workspace description and random fallback icon

Read [execution plan](dashboard-sidebar.md) first. Implement only DS-01. Wave 1.

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- `docs/05-data-model.md` workspaces/books; `docs/09-api.md` "Structure",
  "T1.10 structure contract".
- Decisions: D-097..D-103, D-158, D-218, D-219, D-234.
- Acceptance: AT-214, AT-216.

## Scope / ownership

- Own `migrations/0009_workspace_details.sql`, `internal/structure/`
  (queries.sql, regenerated `structuresql`, workspaces.go, nav.go, dto,
  validate, tests) and the HTTP handler/codec for workspace create/patch.
  In the frontend, own `web/src/stores/nav.ts` types and mapping plus the
  create/update request payload helpers and their unit tests.
- Migration:
  - `description text NOT NULL DEFAULT ''`. Validate in Go: trimmed, max 500
    runes, newlines allowed, other control characters rejected.
  - `fallback_icon text NOT NULL` with a random default from the same 8-name
    vocabulary as books (copy the 0008 pattern), backfilled for existing rows.
  - Constrain `icon` to the book AppIcon vocabulary (reuse `cleanBookIcon`).
    The migration nulls any existing non-vocabulary icon values (they were
    never rendered) before adding the CHECK. Add a Down section.
- API:
  - `POST /workspaces {name, icon?, color?, description?}`.
  - `PATCH /workspaces/{id}`: `description` may be `""`; `null` clears `icon`.
  - Workspace DTOs and `NavWorkspace` gain `description` and `fallbackIcon`.
    Clearing `icon` reveals the same `fallbackIcon` for every reader.
- Store: `NavigationItem` gains `description: string` and reuses `bookIcon`
  (or a renamed `icon`) as `icon ?? fallbackIcon` for workspaces. The
  `nav.create/update` helpers accept the optional `icon` and `description`.
  Do not build the dialog UI (DS-06).

## Contracts

- Path-only workspaces expose name and icon only. Their description is `""`
  in `/nav`, like the other withheld sibling content (06-permissions).
- Admin inventory, trash/restore and default-workspace provisioning keep
  working. Starter `Default` gets a fallback icon and an empty description.
- Events/realtime payloads carry the new fields where workspace DTOs are
  already emitted. No new endpoints.
- Record one `D-xxx` row for the vocabulary restriction and description limits
  as a handoff to the DS-03 coordinator.

## Confined verification

1. DS-01.1: create with/without icon/description. The fallback is stable across
   reads and users, the custom icon overrides it, and clearing restores the
   same fallback.
2. DS-01.2: invalid icon, a description longer than 500 runes or containing
   control characters → `ERR_VALIDATION`. Non-owners cannot PATCH. The
   path-only nav hides the description.
3. DS-01.3: migration Up/Down on a seeded DB with legacy free-text icons.
4. Run `go build ./...`, then focused `go test -run 'TestWorkspace|TestNav' ./internal/structure -count=1`
   (PG18 DSN, `PT_TEST_DATABASE_REQUIRED=1`). Then
   `pnpm --dir web exec vitest run src/stores/nav.test.ts` and touched-file ESLint.

## Handoff / wrap-up

Record the changed files, test counts, the exact DTO shape for DS-02/DS-06 and
the decision row text. Release `stores/nav.ts` and the structure package to
DS-02. Leave it **implemented, awaiting DS-10 gates**.

## Record

- Implemented migration 0009, workspace validation/DTO/nav fields, create/patch
  handling, sqlc bindings, store mapping/payloads, and focused migration, API,
  structure and store tests. Files changed:
  `migrations/0009_workspace_details.sql`;
  `internal/db/{migrate_test.go,metadata_upgrade_test.go,workspace_preference_upgrade_test.go,workspace_details_upgrade_test.go}`;
  `internal/httpapi/{structure.go,structure_integration_test.go}`;
  `internal/structure/{book_icons_test.go,dto.go,nav.go,queries.sql,structure_acl_test.go,structure_pg_test.go,structuresql/queries.sql.go,validate.go,workspaces.go}`;
  `web/src/{api/types.ts,stores/nav.ts,stores/nav.test.ts,test/navigation.ts}`;
  this task prompt's Record.
- DTO handoff: workspace mutation responses include
  `{id,name,icon,color,description,fallbackIcon,createdAt}`. `/nav` workspace
  items include `{id,name,icon,color,description,fallbackIcon,createdAt,level,
  pathOnly,books}`; path-only descriptions are `""`. The store exposes the
  displayed workspace icon as `icon ?? fallbackIcon`; create/update payload
  helpers accept optional `{icon,description}`.
- Verification: `go build ./...` passed; focused PostgreSQL Go tests passed
  (18 tests across structure, httpapi and db, including migration Up/Down);
  `pnpm --dir web exec vitest run src/stores/nav.test.ts` passed (12 tests);
  touched-file ESLint, frontend typecheck and `i18n:check` passed. Task remains
  implemented, awaiting DS-10 gates.
- Central-doc handoff (owned by the wave coordinator): update `05-data-model.md`
  with workspace description/fallback columns and icon vocabulary; update
  `09-api.md` with create/patch/nav shapes, validation and path-only description
  masking; append this decision row to `decisions.md`:

  | D-235 | 2026-11-07 | Workspace descriptions are trimmed plain text, at most 500 Unicode code points; LF and CR newlines are allowed, while other Unicode control characters are rejected. A non-null workspace icon must use the `BookIconName` vocabulary; its persisted fallback is selected once from the same eight-name palette as books and is unchanged when an override is cleared. | Aligns validation and icon constraints while keeping fallback presentation stable across readers. | DS-01 implementation | Handoff to DS-03 coordinator |
