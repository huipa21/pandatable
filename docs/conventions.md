# Conventions

## Repository & branches
- Single linear history on `master` (D-131): commit every task directly on
  `master`; do not create feature/task branches, merge commits or squash
  merges. Run `git switch master` before starting if needed; `git branch`
  must list only `master`.
- **No hosted CI**: run `go run ./tools/ptdev check` before every commit to `master` and before every tag; `ptdev e2e` before milestone tags (D-048).
- Commits: Conventional Commits — `feat(grid): fill-down`, `fix(acl): …`, `docs: …`, `chore: …`. Reference task IDs: `feat(auth): login endpoint [T1.3]`.
- Versioning: SemVer; `v0.x` until M3 complete → `v1.0.0`.

## Go
- Go ≥ 1.25, modules, `gofmt` + `golangci-lint` (govet, staticcheck, errcheck, gosec, revive).
- `internal/` only; no exported packages.
- Errors: wrap with `fmt.Errorf("…: %w", err)`; domain errors are typed `apperr.Error{Code, Status, Details}` and mapped once in `httpapi`.
- Logging: `log/slog` JSON; request id per request; never log passwords, tokens, cell values.
- Context: every DB call takes `ctx`; handlers have timeouts.
- SQL:
  - Static meta queries via `sqlc`.
    Auth queries use `sqlc generate` with pinned **1.31.1**, repository
    [configuration](../sqlc.yaml) and [query source](../internal/auth/queries.sql);
    commit the generated Go bindings. The offline PG catalog declaration is
    generator-only, never a migration. The generator is not a runtime dependency.
    The same configuration also generates ACL, admin and structure bindings from their
    respective `queries.sql` files; a pinned regeneration must be byte-identical
    for all four generated packages. Admin's `catalog.sql` likewise describes
    monitoring catalogs/Goose metadata for generation only, never runtime DDL.
  - Dynamic data queries via `squirrel` with **identifiers only from meta** (quoted with `pgx.Identifier{}.Sanitize()`), values always as parameters.
  - No string concatenation of user input into SQL — enforced by code review + gosec.
  - Runtime physical DDL uses `internal/ddl` with caller `pgx.Tx` only
    (D-094..D-096). Its type/default allowlist handles DDL's lack of ordinary
    bind placeholders; metadata identifiers must still pass exact kind/alphabet/
    length validation before pgx quoting. Never pass raw `pg_type`, display
    names or default expressions as SQL. Services own ACL, metadata/version/event
    writes, collision retry and transaction completion; physical DROP is purge.
- Tests: `go test ./...`; integration tests use [internal/testdb](../internal/testdb/) with a package-level Suite and TestMain cleanup, real PostgreSQL 18 from explicit `PT_TEST_DATABASE_URL`, and a fresh migrated database per package. Absent/unreachable DBs clearly skip within a bounded connection attempt; config/auth/SQL/migration errors fail. `ptdev test/check` starts/reuses the dev DB and requires integration tests (no unavailable-DB skips). No testcontainers. See [14, tests](14-dev-build-release.md#3-tests-without-hosted-ci).
- Cross-platform: code and tooling must work on Windows (dev) and Linux (prod). Use `filepath`, no shell-specific scripts; dev automation goes into `tools/ptdev`.
- Container engine neutrality: Dockerfile/compose must work with both Podman and Docker (fully qualified image names, no engine-specific features).

## Frontend
- `<script setup lang="ts">`, strict TS, no `any` without comment.
- Components PascalCase; composables `useXxx`; stores `useXxxStore`.
- All visible strings via `t('…')`; keys in both `en.json` and `zh-CN.json`.
- API types in `src/api/types.ts` mirror Go DTOs (kept in sync manually; consider generation later).
- Navigation preserves server order/levels; structure controls use owner of the
  parent, while sharing uses owner of the target scope. Path-only ancestors
  never gain controls from visible descendants or the admin role.
- Resource reads/mutations are deadline-bounded; nav/schema requests cancel or
  ignore stale responses on route/identity changes. Failed refreshes are visibly
  stale, never an empty success. Native dialogs and drag alternatives must remain
  keyboard/touch accessible; names are validated by Unicode code points.
- Sharing uses the target's effective ownership, not parent-structure ownership.
  Pickers use scoped `/directory` search, never admin enumeration; no email or
  membership disclosure. Direct mutations require review and authoritative
  re-fetch, including after uncertain writes. Keep inherited grants read-only
  and distinguish per-principal inheritance from user/group effective access.
- Batch-1 fields bind pure definitions in `web/src/fields`; Go binds the
  matching `internal/codec` registry. Never coerce bigint/decimal strings or
  datetime microseconds through JS numbers. Keep both implementations aligned
  with shared `internal/codec/testdata/vectors.json`; raw JSON-only cases are
  explicitly Go-only. Native field components use static localized errors,
  interpolation, revalidated links and escaped mailto address delimiters.
  Codecs are not permission checks or a reason to wire synthetic grid routes.

## Naming
- API JSON: camelCase. DB: snake_case. Physical user objects: `b_`, `t_`, `c_`, `j_`, `s_` prefixes + 10-char base32.
- Error codes: `ERR_UPPER_SNAKE` (e.g. `ERR_LAST_OWNER`, `ERR_PERMISSION_DENIED`, `ERR_VALIDATION`, `ERR_VERSION_CONFLICT`, `ERR_CONVERSION_FAILED`, `ERR_RESTRICT_VIOLATION`).

## Testing levels
| Level | Tool | Scope |
|---|---|---|
| Unit | go test, Vitest | codecs, formula compiler, filter compiler, ACL resolver, paste parser |
| Integration | go test + PG18 (dev container) | services + DDL + queries against real DB |
| E2E / Acceptance | Playwright via `ptdev e2e` | AT-xxx from acceptance-tests.md |
| Performance | Go bench + seed script (200k rows) | NFR-2/3 |

## Docs
- Update `progress.md` on task start/finish.
- New decisions → `decisions.md`.
- Design changes → update the relevant `05–13` doc in the same commit as code.
