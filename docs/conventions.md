# Conventions

## Repository & branches
- `master` is always releasable. Feature branches `m1/t1.4-login` → merge with squash.
- **No hosted CI**: run `go run ./tools/ptdev check` before every merge to `master` and before every tag; `ptdev e2e` before milestone tags (D-048).
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
  - Dynamic data queries via `squirrel` with **identifiers only from meta** (quoted with `pgx.Identifier{}.Sanitize()`), values always as parameters.
  - No string concatenation of user input into SQL — enforced by code review + gosec.
- Tests: `go test ./...`; integration tests use [internal/testdb](../internal/testdb/) with a package-level Suite and TestMain cleanup, real PostgreSQL 18 from explicit `PT_TEST_DATABASE_URL`, and a fresh migrated database per package. Absent/unreachable DBs clearly skip within a bounded connection attempt; config/auth/SQL/migration errors fail. `ptdev test/check` starts/reuses the dev DB and requires integration tests (no unavailable-DB skips). No testcontainers. See [14, tests](14-dev-build-release.md#3-tests-without-hosted-ci).
- Cross-platform: code and tooling must work on Windows (dev) and Linux (prod). Use `filepath`, no shell-specific scripts; dev automation goes into `tools/ptdev`.
- Container engine neutrality: Dockerfile/compose must work with both Podman and Docker (fully qualified image names, no engine-specific features).

## Frontend
- `<script setup lang="ts">`, strict TS, no `any` without comment.
- Components PascalCase; composables `useXxx`; stores `useXxxStore`.
- All visible strings via `t('…')`; keys in both `en.json` and `zh-CN.json`.
- API types in `src/api/types.ts` mirror Go DTOs (kept in sync manually; consider generation later).

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
- Design changes → update the relevant `05–13` doc in the same PR as code.
