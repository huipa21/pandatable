# 14 — Development, Build & Release

There is **no hosted CI** (D-048). Everything — checks, tests, image build, image export — runs on the developer laptop (Windows 11 + Podman). The resulting image archive is copied to the Linux server (Docker) and loaded there.

## 1. Developer prerequisites (Windows laptop)

| Tool | Version | Notes |
|---|---|---|
| Go | 1.27.0 (module/toolchain floor) | `GOPROXY` may be set to `https://goproxy.cn,direct` if needed |
| Node.js | >= 22.13 + pnpm 11.2.2 (via Corepack) | Tested with Node 24.15.0; npm registry mirror optional (`.npmrc`) |
| Podman Desktop / podman | ≥ 5.x, podman machine running (WSL2) | `podman compose` requires `podman-compose` or docker-compose v2 binary as provider |
| Git, OpenSSH client | built into Windows | `scp`/`ssh` for transfer |

Docker on Linux/macOS works equally (the tooling auto-detects the engine).

## 2. Cross-platform dev tool — `tools/ptdev` (D-048)

Instead of Makefiles or duplicate `.sh`/`.ps1` scripts, a small Go program provides all developer commands identically on Windows (PowerShell/cmd) and Linux:

```
go run ./tools/ptdev <command> [flags]
```

| Command | What it does |
|---|---|
| `help` / `engine` | List commands and detected engine / show selection reason |
| `db up` / `db down` / `db reset` | Run dev PostgreSQL 18 container `pt-dev-db` on `127.0.0.1:55432` (volume `pt-dev-pgdata`) |
| `db status` / `db psql` | Show running/stopped/absent + DSN / open interactive psql |
| `dev` | Build a temporary backend executable and supervise it plus Vite directly; proxy `/api` (WebSocket-ready) and `/healthz` to the backend |
| `web build` | `pnpm install --frozen-lockfile && pnpm build`, copy `web/dist` → `internal/web/dist` |
| `build [--linux]` | Build web assets if present, then `go build -trimpath -ldflags "-s -w -X main.version=…" -o bin/pandatable[.exe] ./cmd/pandatable`; `--linux` emits `bin/pandatable-linux-amd64` (`GOOS=linux GOARCH=amd64 CGO_ENABLED=0`) |
| `lint` | `gofmt -l`, `go vet`, `golangci-lint run` (if installed), `pnpm lint`, `vue-tsc --noEmit`, i18n key parity |
| `test [--no-db]` | Start dev DB and require integration tests via `PT_TEST_DATABASE_URL`; uncached `go test ./...`; `--no-db` skips DB startup and retains explicit environment; forwards `-run`, `-v`, `-count`; `pnpm test` if present |
| `e2e [--perf \| -- <args>]` | Build a disposable image and isolated stack; run the complete regression suite or the explicit real-200k browser benchmark, retain evidence, tear down only owned resources |
| `check` | `lint` → `test` → `build`, with per-step pass/fail and duration summary; any failed step causes non-zero exit (D-051) |
| `image build [--version VERSION]` | Build `localhost/pandatable:<tag>` + `:latest` for `linux/amd64`; preserve original version in the binary |
| `image save [--version VERSION] [--with-postgres]` | Export version-tagged image to `dist/pandatable-<tag>.tar.gz`; optionally pull/export pinned PG18 to `dist/postgres-18.tar.gz` |
| `deploy --host user@server --yes [--dir /opt/pandatable] [--version VERSION] [--with-postgres]` | Optional: transfer existing archive(s), load, update only `PT_VERSION`, wait for Compose + internal health; no secret/config transfer |
| `version` | `git describe --tags --dirty --always`, strip leading `v`; untagged SHA becomes `0.0.0-dev+<sha>[.dirty]`; unavailable Git/repository falls back to `0.0.0-dev` |

Run ptdev from the repository root. Through T0.7, all listed commands are
implemented. `check` deliberately excludes milestone E2E (D-051/D-065).
Build/lint/test retain the no-web-manifest skip path. With the
scaffold present, `pnpm lint` also runs Prettier, `vue-tsc` and i18n parity.
Run `pnpm --dir web install --frozen-lockfile` before the first lint/test/dev;
the build commands perform the frozen install themselves. The workspace allows
only esbuild's required dependency build script, not arbitrary install scripts.

`ptdev dev` uses `PT_DATABASE_URL` when explicitly set (without starting the
dev DB); otherwise it starts/reuses the dev DB. Defaults: backend
`PT_LISTEN=127.0.0.1:8080`, `PT_BASE_URL=http://127.0.0.1:5173`, Vite on
`127.0.0.1:5173` with strict-port binding. Explicit app settings retain precedence.
Vite's API target is derived from `PT_LISTEN` (wildcard hosts mapped to loopback);
port zero is rejected. Standalone `pnpm dev` uses `PT_DEV_API_TARGET` or
`http://127.0.0.1:8080`. No WebSocket server is implemented.

The supervisor compiles once (restart it for Go changes); Vite provides frontend
HMR. Both servers are direct children, not `go run`/`pnpm` grandchildren. Either
child exit/start failure stops the other and returns a visible error. Ctrl+C or
SIGTERM cancels both and waits for reaping before removing the temporary binary.
Interrupt is attempted where supported, with forced termination otherwise;
the backend has a 12-second wait bound (covering its 10-second HTTP drain),
Vite three seconds. Windows does not support Go's `os.Interrupt` delivery to
children, so cancellation forcibly stops the direct children there.

For backend-only development, set `PT_DATABASE_URL` and
`PT_BASE_URL` and run `go run ./cmd/pandatable serve` (optionally `--listen
127.0.0.1:8080`). Startup migrates the DB before listening. `/healthz` works
without frontend assets; UI requests return explicit 503 until `ptdev web build`
or `ptdev build` copies output into `internal/web/dist`. Generated assets remain
ignored; copying validates the source index before replacing assets and retains
the tracked `.gitkeep` so clean/failed-build checkouts still compile.
No placeholder HTML is shipped.
The HTTP unit tests use an in-memory SPA fixture. DB, CLI restart and opt-in
dev integration tests share [internal/testdb](../internal/testdb/), with one
migrated disposable PostgreSQL 18 database per package.

For the opt-in real dev-process test, set `PTDEV_TEST_DEV=1`, leave local port
5173 free, and run `go run ./tools/ptdev test -run TestDevIntegration -v`.
Alternatively supply `PT_TEST_DATABASE_URL` and run the focused Go test directly.
It tests Vite/backend startup, health/API proxying and cancellation, asserting
both listeners close; the shared helper drops its database afterwards. Ordinary
runs skip this process-heavy test explicitly.

The dev DB DSN is
`postgres://postgres:postgres@127.0.0.1:55432/postgres?sslmode=disable`.
The PostgreSQL 18 volume is mounted at `/var/lib/postgresql` (not the pre-18
`/var/lib/postgresql/data` path). Readiness is limited to 60 seconds. `db down`
keeps data; `db reset` removes the container and volume, then starts fresh,
and requires confirmation unless `--yes` is supplied.

Exit codes: 0 success, 1 operational failure, 2 usage/not implemented. Commands
are executed directly without a shell; output is streamed and command lines
are echoed to stderr. Ctrl+C cancels active children. No ANSI colors are used,
so `NO_COLOR` and redirected output need no special handling. On Windows,
pnpm npm/Corepack shims are launched through their Node entry point, not `cmd`.

### Container engine detection
1. `PT_ENGINE` env (`podman` | `docker`) if set.
2. Else `podman` if on PATH, else `docker`.
3. Compose: `<engine> compose` (both engines support the subcommand).

An invalid `PT_ENGINE` is a usage error. An explicit valid override that is
missing from PATH fails engine/DB operations without falling back. Without an
engine, help still works (reports unavailable), and version, lint, build and
`test --no-db` remain usable. Engine selection checks PATH, not daemon health;
DB operations report a stopped/unreachable machine as an operational error.

Engine-specific flags handled by ptdev:

| Concern | Podman | Docker |
|---|---|---|
| Build | `podman build --format docker --platform linux/amd64 -f deploy/Dockerfile -t localhost/pandatable:<ver> .` (`--format docker` keeps `HEALTHCHECK`) | `docker build --platform linux/amd64 -f deploy/Dockerfile -t localhost/pandatable:<ver> .` |
| Save | `podman save --format docker-archive -o …tar localhost/pandatable:<ver>` then gzip | `docker save -o …tar localhost/pandatable:<ver>` then gzip |
| Load on server | `docker load -i pandatable-<ver>.tar.gz` (gzip accepted) | same |

The `localhost/` prefix is used for both engines so names are identical after load.

Git SemVer metadata contains `+`, which is invalid in an image tag. Image commands
replace it with `-` for the tag/archive filename only; e.g.
`0.0.0-dev+abc.dirty` becomes `localhost/pandatable:0.0.0-dev-abc.dirty`.
The binary retains `0.0.0-dev+abc.dirty`. Versions must have 1-128 tag-safe
characters (plus `+`), starting with a letter/digit/underscore. Do not publish
distinct versions that collide under this mapping. Use `--version` consistently
when saving/deploying a previously built version after the worktree changes.

Exports use a private staging directory under `dist`, then gzip and rename only
after success. Failures/cancellation retain any prior final archive and remove
staging files. Each archive contains its named version tag, not an unrelated
current `latest`. The PostgreSQL archive contains
`docker.io/library/postgres:18.6-bookworm`, matching Compose.
Neither save nor deploy silently substitutes a missing app image/archive.

## 3. Tests without hosted CI

- **Unit:** `go test ./...` (fast, no DB) + Vitest.
- **Integration:** explicitly set `PT_TEST_DATABASE_URL` to a PostgreSQL 18 URL
  for an account allowed to create databases and run the existing migrations.
  There is no default connection for plain `go test`. The shared helper creates
  a fresh `pt_test_<random>` database from template0 per package and migrates it.
  Absent configuration or dial/DNS unreachability gives a clear skip after at
  most a 3 s connection attempt. Invalid config, authentication, missing database,
  TLS, SQL and migration errors fail, never skip; credentials are redacted.
  `ptdev test/check` starts/reuses the dev container and sets both the DSN and
  `PT_TEST_DATABASE_REQUIRED=1`, making unavailable DBs failures in the real gate.
  Tests default to `-count=1`; an explicit forwarded `-count` overrides it.
  No testcontainers or new Go dependencies.
- **E2E/acceptance:** pinned Playwright against the real image via both
  `deploy/compose.yaml` and `deploy/compose.local.yaml`, independently of `check`.
- Results of `ptdev check` + `ptdev e2e` are noted in `progress.md` when a milestone is closed.

### Default-parallel gate (M1 review, D-145/D-146)

Since 2026-10-27 both gates pass with plain `go run ./tools/ptdev check` and
`go run ./tools/ptdev e2e`; the `GOFLAGS=-p=1` workaround required by earlier
task records is no longer needed and must not be used to hide a new failure.
The Suite teardown now has a 60 s cleanup budget separate from its unchanged
3 s connect attempt and retries the forced drop only on `55006` (D-145); the
E2E settings helper restores instance-wide settings on failure and asserts the
real shared workspace inventory instead of a globally empty one (D-146).
Earlier task records keep their original `GOFLAGS=-p=1` evidence as written.

T1.21's focused required PG18 gate (explicit `PT_TEST_DATABASE_URL` and
`PT_TEST_DATABASE_REQUIRED=1`) is
`go test .\internal\structure .\internal\httpapi -run 'ViewColumns' -count=1`.
It uses the existing package Suites and covers strict column payloads,
live/deleted/unknown/duplicate IDs, widths/frozen bounds, missing/stale/valid
If-Match, ACL/personal/locked views, transport, preservation/cleanup and
concurrent saves with exactly one version winner. Frontend focused tests:
`pnpm --dir web exec vitest run src/components/grid/DataGrid.test.ts src/components/grid/viewColumns.test.ts src/components/grid/remoteWindow.test.ts`.
The production-image AT-305/311 scenario is
`go run .\tools\ptdev e2e -- e2e/grid-columns-keyboard.spec.ts --project=desktop-chromium`.
It exercises every documented key in both locales and themes, real header
resize/drag, accessible hide/reorder/freeze, reload/shared writer persistence,
reader local-only state and a real stale-version reconciliation. Existing
clipboard fixtures explicitly select an unfrozen viewport through the public
view API, preserving their complete original range/value assertions now that
the default stored frozen count is honored. Run lint/bundle, focused PG18,
then complete default-parallel check and the unfiltered production E2E suite;
no scenario/global/assertion deadline, retries, worker or resource budget changes.
The keyboard fixture has 401 real rows, so Ctrl+End must load an uncached page;
PageUp/Down asserts the exact viewport row count and all four Shift+arrow
directions are covered. D-151 keeps logical source length stable during jumps.
The mobile ACL helper waits for its own downgrade-triggered schema/card reload
and pending queries before final grant removal, avoiding a stray read across
the second revocation without ignoring any HTTP diagnostic.

T1.13's focused lower-layer gate is `go test ./internal/codec -count=1`
with explicit `PT_TEST_DATABASE_URL` and `PT_TEST_DATABASE_REQUIRED=1`, plus
`pnpm --dir web exec vitest run src/fields/fields.test.ts src/components/fields/fields.test.ts`.
Shared vectors cover options, exact wire values, paste/copy and capabilities.
Suite-owned PG18 tests create columns through actual typed DDL descriptors,
check native catalog types/forced constraints, round-trip typed pgx parameters
and exercise defaults/range failures/system columns. They are not full
AT-301/302/304 evidence without real field/row/grid APIs/UI.
Record default-parallel Suite cleanup failures separately from command-scoped
`GOFLAGS=-p=1` results; never relax the helper's assertions or deadlines.

T1.14's focused gate is
`go test ./internal/structure ./internal/httpapi -run 'Field|CleanDescription|Choice|PGType' -count=1`
(same explicit DSN/required flags) plus
`pnpm --dir web exec vitest run src/components/fields/FieldPanel.test.ts src/pages/ResourcePage.test.ts`.
The production `ptdev e2e` exercises real owner Field UI/API flows from
`web/e2e/fields.ts` inside the existing navigation scenario.

T1.15's required row/query integration gate uses the same explicit PG18 DSN
and `PT_TEST_DATABASE_REQUIRED=1`:
`go test ./internal/structure ./internal/httpapi -run 'Rows|Query' -count=1`.
It covers batch-1 codecs against real physical columns, permissions,
constraint/default handling, atomic rollback and schema DDL serialization,
plus real HTTP CSRF/body/response behavior. The opt-in
`PT_RUN_ROW_PERF=1 go test ./internal/structure -run '^TestRowQueryPerformance200k$' -count=1 -v`
seeds a Suite-owned table with `generate_series(1,200000)` for API paging/count
timings. It covers all 12 writable batch-1 kinds through real columns,
precision/offset/select rejection, defaults, required/unique errors, owner/
writer/reader/stranger/admin permissions, atomic rollback, DDL serialization,
and actual HTTP query/CRUD, body bounds, CSRF and foreign-Origin rejection.
The common protected-handler test verifies deadlines. This is a documented
performance fixture, not browser acceptance for AT-312. Squirrel v1.5.4 is
the convention-required SQL builder; identifiers remain validated metadata
and values remain bound parameters.

T1.17 frontend/browser verification:

```powershell
pnpm -C web test
pnpm -C web lint
pnpm -C web build
go run .\tools\ptdev e2e -- e2e/grid.spec.ts --project=desktop-chromium
go run .\tools\ptdev e2e -- e2e/grid.spec.ts --project=mobile-chromium
```

The real production-image grid scenario verifies AT-306..309, including the
one-request valid-cell bulk patch, invalid-cell reporting, confirmed appended
rows, clipboard formatting, Ctrl+D and vertical fill handle. Unit/API tests
alone do not establish these acceptance results. The complete `ptdev e2e`
suite is still required and unrelated failures must be recorded separately.
AT-312 requires browser measurements on the documented 2-vCPU/4-GB target;
Podman VM timings or API-only page timings are not substitutes.

T1.18 focused verification:

```powershell
pnpm -C web exec vitest run RowExpandDialog DataGrid ResourcePage fields.test
pnpm -C web lint
pnpm -C web build
go run .\tools\ptdev e2e -- row-expand --project=desktop-chromium
go run .\tools\ptdev e2e -- row-expand --project=mobile-chromium
```

The production-image AT-310 scenario creates real metadata and a row via the
grid, opens from both the icon and a genuinely focused grid cell's Space key,
edits every writable kind, checks exact API values, reloads direct row URLs,
and exercises hidden/system/deleted fields, invalid/deleted resources,
read/write/revoked access, late responses, locales/themes and mobile fallback.
Fixtures never bypass row ACL. Failed HTTP responses must be registered before
navigation/mutation via `expectHTTPFailure` (exactly once each). Only a
background revalidation that races a deliberate revocation may use the per-page
`allowHTTPFailure`, which tolerates zero or more matching failures on that one
page; there is no suite-wide tolerance (D-157). Logins go through the shared
`paceLogin` helper so the per-IP login limit holds across scenarios.
Component tests also release
responses after route/schema/identity changes even when fetch ignores abort.
The same uncached required PG18 row/query command documented above and both
default/command-scoped serial-package full checks remain mandatory, as does
the complete production-image `go run .\tools\ptdev e2e`. Do not infer AT-312
or unrelated full field/sharing acceptance from the row dialog.

T1.19 focused verification:

```powershell
pnpm -C web exec vitest run MobileCardList RowExpandDialog ResourcePage fields.test
pnpm -C web lint
pnpm -C web build
go run .\tools\ptdev e2e -- mobile-rows --project=mobile-chromium
```

The AT-313 production-image scenario explicitly uses **390x844**, real accounts,
field/row APIs and existing codecs. It covers twelve editors, exact values,
required creation failure, omitted physical defaults, save/deep-link/reload,
hidden display-field retrieval, 100-row paging and <=300 cached rows, keyboard
card activation/Escape/focus return, full-screen layout/overflow, both locales/
themes, reader/writer/revoked/hidden access and real held read/write responses.
Component tests additionally exercise route/schema/identity/unmount guards even
when providers ignore abort. The complete production-image E2E suite and both
default and command-scoped serial full checks remain required; failed expected
HTTP requests are registered before the operation. Existing mobile fallback
assertions now check the real cards, not the superseded notice, without removing
the desktop AT-310/302/306..309 assertions or earlier evidence. No D-112 budget
or retry/worker/assertion/performance limit changes. These checks do not certify
AT-312 or NFR-1 on the required 2-vCPU/4-GB target.

### T1.20 safe seed and production-browser baseline (D-143/D-144)

The application command is **`pandatable dev seed --rows 200000`**, not a
production HTTP endpoint or a database reset. Its additional safety flags are
mandatory:

```powershell
# Provision/reserve your own unique PG18 disposable database first.
# Migrate it with the existing CLI and complete normal web setup/password change.
# Use the existing active user's UUID from the normal /auth/me response.
$env:PT_DATABASE_URL = '<explicit disposable PostgreSQL connection URL>'
.\bin\pandatable.exe dev seed --rows 200000 `
  --confirm-database pt_seed_local_unique --owner-id <existing-user-uuid> `
  --yes-dev-seed
```

`--database-url` overrides `PT_DATABASE_URL` by the normal CLI convention.
The configured database must match `--confirm-database` exactly and match
`pt_seed_` or Suite-owned `pt_test_` followed by 1..48 lowercase letters,
digits or underscores. No implicit localhost/developer DSN. The command checks
the connected database name, major 18, no workspaces **including Trash**, an
existing active owner without forced password change, and a nonblocking
database advisory session lock on a reserved connection (no idle transaction
between batches), with independent bounded unlock/close on every exit.
Reserve the DB exclusively; do not make concurrent
ordinary UI mutations while seeding. An admin role is neither required nor an
ACL bypass. The command does **not** create a database/user/session, run
migrations, infer credentials, alter an existing table, reset, resume or purge.

The fixture uses the ordinary authorized services: one workspace with a real
direct user owner grant, book/physical schema, table/default shared Grid and
system fields. The Name display field is required and unique. Twelve additional
fields exercise long text, ordinary integer, bigint, precision-38/scale-6
decimal, real float, checkbox, date, datetime, single/multi-select, email and
URL. Values are deterministic by one-based index (`Seed row 000001`, etc.);
bigints above 2^53 and decimal/microsecond strings retain exact precision.
Every tenth row has explicit optional NULLs; odd checkboxes omit their forced
false default; every fifth status omits its physical `"Todo"` default.
Metadata UUIDs/physical identifiers, actor UUIDs and transaction timestamps
are genuinely generated, not deterministic snapshots. No new codec.

Bounds are 1..200000 rows (default 200000), 300 rows and at most 3900 supplied
cells per atomic batch, ten seconds per batch and ten minutes overall. Metadata
and batches commit separately; this is **not** a single atomic 200k operation.
The terminal JSON manifest reports database, owned IDs/field mapping, stage,
requested/confirmed-committed/attempted counts and `complete`. Failed batches
roll back, cancellation propagates, no automatic retry/split of a failed
operation. A canceled/lost commit can be uncertain: confirmed count is a lower
bound, attempted count identifies the uncertain batch. Do not retry/resume.
An occupied target is refused, including after partial failure. Preserve the
manifest/explicit target; inspect it and drop **only that operator-provisioned
disposable database** when finished. No broad name-pattern cleanup. SIGKILL or
power loss may prevent a final manifest; the explicitly reserved DB remains
the ownership boundary. Never seed an unrelated/production database.

Focused required PG18 verification (explicit Suite admin DSN as above):

```powershell
$env:PT_TEST_DATABASE_REQUIRED = '1'
go test .\cmd\pandatable .\internal\devseed .\tools\ptdev -count=1
```

Tests cover CLI refusal/redaction, exact serialized row/cell/body bounds,
existing codecs, ordinary non-admin ownership, physical defaults and exact
values, PG18 concurrent-seed exclusion, occupied/mismatched/disabled/forced
targets, atomic unique failure and real lock-wait cancellation. The existing
Suite owns and drops its whole test DB, including physical schemas; no shared
dev database cleanup or relaxed deadline.

Run the real production-browser workflow:

```powershell
go run .\tools\ptdev e2e --perf
```

On an **authorized target host** the same Go command works with Docker and
the documented Go/Node/pnpm/Chromium prerequisites (POSIX uses `./tools/ptdev`).
Do not reconfigure a shared VM to approximate the target. `--perf` selects a
fixed scenario; it cannot be combined with arbitrary Playwright arguments.
The regular complete `ptdev e2e` remains unchanged and required separately.
The perf harness owns one unique `pt-e2e-*` stack/image and `pt_seed_*` DB.
Compose's optional `PT_DB_NAME` defaults to `pandatable`; only this workflow
selects its unique DB name. Do not change this setting for an existing volume.
Normal setup browser acceptance runs first, the owner is obtained by normal
login, and the production binary inside the owned app runs the CLI seed.
Only after all 200000 rows are confirmed does the benchmark start. No
seed-only UI, fake rows, HTTP seed endpoint, session-storage artifact or
production authorization shortcut. Preparation has the seed's own deadline;
browser budgets remain 150 seconds/scenario, 600 global, one worker, zero
retries, five-second assertions.

Measurement boundaries:

- Fresh authenticated context, no preceding SPA visit/asset prewarming.
  Start at document navigation (`performance.timeOrigin`), including auth/nav/
  schema requests, lazy grid/editor chunks and first query. End only when the
  exact first real Name cell is in the grid viewport and an enabled native row
  action exists across two consecutive animation frames. This includes a real
  rendering opportunity, not a compositor/GPU trace or generic FCP of a spinner.
- Middle samples start **before** public `scrollToRow(globalIndex)` at offsets
  100000, 100500, 99900, 101000, 100100; nonadjacent jumps force real page loads.
  End at the matching exact visible row/action across two frames. Match the
  actual query's global offset, 100 limit and exact decimal-string first ID;
  never use viewport/RevoGrid DOM indexes as data offsets (D-139).
- Separately record fetch start, headers and cloned response-body completion,
  plus navigation and asset resource timing. Assert first usable paint
  **<1500 ms**, **every** middle scroll-to-paint **<300 ms** and every measured
  middle request/body **<300 ms**. No averaging, adjusted thresholds, synthetic
  spike timers or API-only substitution. Reports are attached before threshold
  assertions, including failures.
- Verify 200000 addressable source slots and **<=300 loaded row models** at
  each sample; existing window tests preserve active +/-1 and eviction. No
  production telemetry API or initial-bundle import is added.

Ignored per-project results include `seed.json`, `host.json` (engine-visible
CPU/RAM/OS/kernel, not guessed container quota), timestamped app+DB
`resources.jsonl` samples during browser load, and `performance.json` with
request/render/asset boundaries (also attached as `performance-baseline`).
Harness evidence lives outside the browser output subdirectory so Playwright
cannot erase it between the setup and measured invocations. The workflow fails
explicitly if the standalone browser report was not persisted.
Record the image, tool versions, physical/VM CPU allocation and RAM separately,
browser machine/network topology, viewport, cold-context and database-cache
conditions, and other host load. Seeded DB buffers are warm; browser assets
are cold. Resource samples are container usage, **not app RSS or NFR-1
certification**. Build/startup snapshots alone are not normal-load evidence.
An engine-visible host with 2 CPUs does not prove a two-vCPU allocation; on a
VM record the hypervisor configuration as well. A timing pass on the local
8-CPU/2048-MiB Podman VM is not target-host acceptance. T1.20/AT-312 stay
pending until the complete strict scenario passes on the documented authorized
2-vCPU/4-GB Ubuntu 24.04 host. No approved target is currently provided.

Existing owned-project teardown runs on seed/browser/measurement failure or
cancellation and removes only that project's volumes/containers/network/image.
The original dev DB/volume, shared Caddy, release images and unrelated stacks
are never targets. Preserve explicit port-collision/startup failures, strict
browser diagnostics, default-parallel Suite cleanup failures and regression
failures in the handoff; do not hide them with retries or allowlists.

### Shared integration helper

Declare a package-level `testdb.Suite`, call `os.Exit(database.Run(m))` in
`TestMain`, and obtain `database.Open(t)` or `database.URL(t)` in integration
tests. The first use creates/migrates the database; later tests reuse that
package's database. Tests own their rows/transactions; separate packages never
share data. Use external `package db_test` when importing the helper from DB
tests to avoid an import cycle.

Pools opened by the helper close through `t.Cleanup`. After all tests (including
failures), TestMain reconnects with a 5 s bound and drops **only** its recorded
random database using `WITH (FORCE)` to reap forgotten connections. It also
cleans after migration/initialization failures or an ambiguous timed-out CREATE.
Cleanup errors make the package fail and identify the owned database for manual
recovery. Do not use broad `pt_test_*` cleanup queries. As with any in-process
test cleanup, forcibly killing the entire test executable cannot run TestMain.

The DB package's T1.1 tests inspect the real PG18 catalog and exercise defaults,
constraints, Trash/name rules, composite FKs and deletion behavior. Its
`TestMetadataUpgradeRollbackPostgreSQL` temporarily runs Goose Down **only to
0001, only inside its Suite-owned database**, seeds sentinel users/sessions/
settings, and verifies concurrent/repeated 0002 upgrades and populated down/up
cycles. It preserves extension OIDs, the initial version record and an unrelated
metadata object. Production `serve`/`migrate` remain strictly forward-only.
`TestServePostgreSQLRestarts` performs an initial serve plus two actual restarts,
checking all version records (including timestamps) remain unchanged, health,
logs and connection cleanup. A focused live-DB run is:

```text
go run ./tools/ptdev test -run "TestMetadata|TestInitialMigrationPostgreSQL|TestServePostgreSQLRestarts" -v -count=1
```

### T1.2 backend auth checks

[Auth tests](../internal/auth/service_test.go) and
[HTTP tests](../internal/httpapi/auth_integration_test.go) each use their own
Suite-owned PG18 database and bounded fixture cleanup. They insert real PHC
hashes only in those owned databases, never in the developer DB. The real gate
requires live PG18 and uncached execution; an ordinary skip is not acceptance.

```text
go run ./tools/ptdev test -run "TestSession|TestLogin|TestPassword|TestProfile|TestConcurrent|TestAuth|TestForced|TestPHC|TestRateLimit|TestLimitCapacity|TestCSRF|TestTrustedProxy|TestCookie" -v -count=1
go test ./internal/auth -run "^$" -bench BenchmarkPasswordHash -benchtime=3x -benchmem -cpu=2
```

Rate/session tests inject a clock rather than sleeping for deadlines. A
password-change race test waits for actual PostgreSQL lock evidence before
committing the changed hash and revocation. Session validators race a revoke
and verify the row cannot reappear. Expiry tests deliberately extend the stored
expiry to prove idle/absolute limits are checked independently.

Static auth queries are generated locally using pinned `sqlc` **1.31.1**:
`sqlc generate` (also generates `internal/acl/aclsql`, `internal/admin/adminsql`
and `internal/structure/structuresql` from the other `sqlc.yaml` entries). Commit its Go bindings; generation must produce no diff on
rerun. [catalog.sql](../internal/auth/catalog.sql) declares the PG timezone
catalog only for the offline generator; no runtime SQL or migration executes it.
The application image uses existing Go source allowlists and does not include
the generator. Runtime additions are only `x/crypto` (argon2id) and `x/sys`.

By user-confirmed scope, no login/profile frontend or auth browser fixture is
shipped in T1.2. Existing `ptdev e2e` remains the real shell/spike gate; it is
**not** browser acceptance for AT-102/116. API coverage and stored profile
timezone/default checks do not complete full AT-117's real datetime/filter/
admin-default scenarios. T1.2 remains in progress until linked acceptance passes.

### T1.22 self-profile and auth browser acceptance

The `ProfilePage` unit tests cover changed-only `/auth/me` patches, timezone
catalog/default handling (`null`, not an empty string), shared locale/theme
preference persistence, load retry and expired-session redirection. The
production-image `profile.spec.ts` scenario owns its uniquely suffixed users
and restores the previous instance-wide admin settings in `finally`. It covers
username/email login, translated bad credentials, logout/back navigation,
display-name/password persistence and the M1 real datetime/timezone path; it
must use real APIs and the real grid/row editor. Preserve the existing
150-second scenario, 600-second global, five-second assertion, one-worker and
zero-retry limits set by D-155. T1.22 is not accepted unless this scenario
passes in the complete production-image run; the M2 relative-date filter half
of AT-117 is not part of this test. The complete scenario runs once in desktop
Chromium; inside it, a separate 390x844 touch-enabled Chromium context checks
the mobile profile route, native controls, password link and horizontal overflow.
The mobile project does not replay the full profile scenario.

### T1.3 setup checks

The auth and HTTP setup tests use separate Suite-owned PG18 databases and clean
their first-admin/settings fixtures with bounded cleanup. The required local
gate supplies `PT_TEST_DATABASE_REQUIRED=1`; setup tests cover token expiry and
restart rotation, normalized identity/password boundaries, rollback, existing
active/disabled users, session storage, CSRF/cookie policy and two independently
initialized services racing through the PostgreSQL setup lock.

```text
go run ./tools/ptdev test -run "TestSetup|TestConcurrentSetup|TestSetupHTTP|TestServePostgreSQLRestarts" -v -count=1
go run ./tools/ptdev e2e
```

The production-image AT-101 test obtains its token only from the uniquely owned
app container's startup log. The harness sends it to Playwright through a
private child environment variable and redacts it from retained Compose logs.
The E2E run also prints a read-only app-container memory/CPU snapshot. No
production seed endpoint or real developer account is used.

### T1.4 ACL and grants checks

`internal/acl` tests are Suite-owned PG18 tests (own `TestMain`); unit tests
need no database. Resolver matrix, group union/membership, disabled/trashed,
path-only visibility, TTL bound, cache hit counts, validation, scope-owner and
last-owner rules, rollback, concurrent owner removals and the D-070 hard-delete
lock interleavings are in `internal/acl`; AT-204..212 at the HTTP surface
(including `authHTTP.require` probe routes) are in `internal/httpapi`.

```text
go run ./tools/ptdev test -run "Test(Effective|Membership|Disabled|PathOnly|Cache|Put|Inherited|List|Scope|Delete|LastOwner|Orphaned|Concurrent|Grant|Failed|Resolver|Access|Level|Resource)" -v -count=1
```

T1.5 supplies real admin user seeding; workspace fixtures still require T1.10.
These ACL checks remain API/HTTP-level evidence.

### T1.5 admin users checks

`internal/admin` owns a separate Suite-managed PG18 database. Tests cover
normalization/legacy ambiguous identities, literal search/pagination, typed
validation/authorization, temporary-password/self-reset flows, all-session
revocation, status/role rules, last-active-admin concurrency, rollback on failed
session revocation, real row-lock waits and caller reauthorization. A cached
last workspace owner's access disappears immediately on disable and returns
on enable; workspace/authorship rows remain. `internal/httpapi` covers the
admin namespace, strict bodies/query parameters, CSRF and DTO/error contracts.
Static queries use the third sqlc 1.31.1 entry (`internal/admin/adminsql`);
all three generated packages must stay byte-identical on regeneration.

```text
go test ./internal/admin ./internal/auth ./internal/httpapi -count=3
pnpm --dir web lint
pnpm --dir web test
go run ./tools/ptdev check
go run ./tools/ptdev e2e
```

The focused Go command requires explicit `PT_TEST_DATABASE_URL` and
`PT_TEST_DATABASE_REQUIRED=1`; `ptdev check` supplies them automatically.
Browser AT-104..108 uses only logged-token setup, actual login/admin APIs and
the production UI. One scenario per desktop/mobile creates normalized users,
edits roles in both locales, searches, changes a forced password, tests
non-admin API/deep-route denial, resets/revokes, disables an open session on
its next request and re-enables the retained identity. Ten total login attempts
fit the unchanged ten/minute/IP limit; the tests do not bypass/reset rate state.
Expected 401 `/auth/me` probes are matched narrowly to Chromium's HTTP console
diagnostics, alongside AT-101's explicitly asserted negative setup responses;
all other warnings/errors/CSP violations still fail the suite.

### T1.6 admin groups checks

`internal/admin/groups_test.go` reuses the package Suite, fixture and real ACL
resolver. Coverage includes Unicode/control bounds, trim/case-sensitive names,
literal search/paging/counts, duplicate/missing/disabled membership, retained
disabled members, atomic deletion/grant cleanup, rollback, all auth classes,
caller reauthorization and concurrent duplicate adds/status changes. Primed ACL
entries gain and lose access immediately, including group deletion. A deferred
constraint-trigger failure proves commit errors still invalidate the cache.
Deterministic gates exercise the real grant service before/after deletion and
when the grant creator is the concurrently added member. D-087 records the
deadlock exposed and corrected by that evidence; no migration is needed.

```text
go test ./internal/admin ./internal/httpapi -run "TestGroup|TestAdminGroups|TestMembershipCommitError" -count=3
go test ./internal/admin ./internal/auth ./internal/acl ./internal/httpapi -count=3
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate
pnpm --dir web lint
pnpm --dir web test
go run ./tools/ptdev check
go run ./tools/ptdev e2e
```

All Go integration commands require explicit `PT_TEST_DATABASE_URL` and
`PT_TEST_DATABASE_REQUIRED=1`; tests remain uncached and use only Suite-owned
disposable databases. Verify pinned regeneration byte identity for all generated
packages. If the version query is unreachable but pinned source is cached, run
that exact module's `cmd/sqlc` with an absolute `-f` repository config and record
the fallback command, never silently change versions or proxy configuration.

The existing `admin-users.spec.ts` scenario now covers AT-104..109 on desktop
and mobile with the same sign-ins: Bob/Carol seed only through the real admin
API, `team-a` creation, both membership-detail views, add/remove, en/zh-CN
edit, exact duplicate-name error, empty search/membership, deletion cancel and
confirmed cleanup. Group/user-detail screenshots are retained under ignored
results. No auth bypass or extra login budget is introduced.

### T1.7 system dashboard checks

`internal/admin/system*_test.go` covers CPU percent/counter reset/availability,
exact five-second cache expiry, coalescing, waiter/lifecycle cancellation,
hourly directory cache, zero/missing/error/bounds policy, rooted symlink skipping,
cgroup namespace/ancestor/unlimited/unsupported parsing, real PG18 size/settings/
versions/migration queries and secret-free error/DTO output. The Linux-only
filesystem test exercises unreadable directories as non-root and special files
without opening them. Windows may skip symlink creation when the account lacks
that privilege; run the compiled tests non-root in a uniquely owned Linux
container as well, not as a substitute for full Linux/Docker acceptance.

```text
go test ./internal/admin ./internal/httpapi -run "TestSystem|TestAdminSystem" -count=3
go test ./internal/admin ./internal/auth ./internal/acl ./internal/httpapi -count=3
go test ./internal/admin -run "^$" -bench BenchmarkSystem -benchtime=1s
pnpm --dir web exec vitest run src/composables/useSystemMetrics.test.ts src/pages/AdminDashboardPage.test.ts src/router/guards.test.ts
go run ./tools/ptdev check
go run ./tools/ptdev e2e -- admin-users.spec.ts
go run ./tools/ptdev e2e
```

All PG commands still require explicit URL/required=1 and Suite-owned databases.
Static system queries use the existing admin sqlc entry with a generator-only
catalog declaration, never an applied migration. Pinned 1.31.1 regeneration must
be byte-identical across auth/acl/admin packages.

AT-110 extends the existing desktop/mobile admin scenario without extra sign-ins.
`ptdev e2e` creates actual **4096-byte attachment and 2048-byte backup fixture
files** as the app UID only inside its unique disposable volume, before the
first observation. These are deliberately not attachment metadata/backup jobs
or a production seed API; the entire owned volume is removed by existing cleanup.
Metrics remain real: CPU/RAM/DB/disk/RSS/cgroup/version values and exact file
totals are asserted, followed by an updated snapshot within ten seconds and
unchanged hourly scan timestamps. Both locales, loading, an exact method/path/
503-scoped transport-failure fixture with explicit stale/retry UI, screenshots,
navigation and independent API/deep-route denial are covered.

The combined admin test allows 45 seconds because it now includes a real
ten-second poll in addition to users/groups/auth flows; its refresh assertion
remains ten seconds, no retries, and the suite's 120-second bound is unchanged.
All warnings/errors/CSP diagnostics remain fatal except narrowly expected,
explicitly asserted HTTP failures. Unit tests additionally prove hidden/unmount
cancellation, no overlaps, request timeouts, late-result rejection and recovery.
Record default-parallel check separately from any command-scoped `GOFLAGS=-p=1`
resource workaround; do not change shared testdb assertions/timeouts.

### T1.8 settings and workspace checks

Required uncached PG18 tests in `internal/admin/settings_test.go`,
`workspaces_test.go` and `internal/httpapi/settings_integration_test.go` cover
catalog/default/bound validation, sanitized public/admin DTOs, independent auth
classes, CSRF/body/query validation, atomic rollback and concurrent patches,
actor-lock recheck, actual auth/profile/default-locale creation readers,
workspace search/page/order, live/Trash/disabled/group-owner orphan rules,
50-owner payload cap and retained-attachment metadata sums. An actual owner
can read grants while a listing-only admin gets 404 and still has no ACL access.
These SQL fixtures exist **only in Suite-owned databases**, not developer data
or browser acceptance seeds.

```text
go test ./internal/admin ./internal/httpapi -run "TestSettings|TestWorkspaces|TestAdminSettings|TestAdminInventory" -count=3
go test ./cmd/pandatable ./internal/admin ./internal/auth ./internal/acl ./internal/httpapi -count=3
pnpm --dir web exec vitest run src/stores/preferences.test.ts src/stores/appSettings.test.ts src/pages/AdminSettingsPage.test.ts src/pages/AdminWorkspacesPage.test.ts src/router/guards.test.ts
go run ./tools/ptdev check
go run ./tools/ptdev e2e -- admin-users.spec.ts
go run ./tools/ptdev e2e
```

Explicit URL/required=1, fresh migrated Suite databases, pinned sqlc **1.31.1**
byte-identical auth/ACL/admin regeneration and the default-parallel versus
command-scoped `GOFLAGS=-p=1` distinction still apply. No assertion, diagnostic
or shared cleanup timeout is weakened to claim a pass.

The existing production-image admin desktop/mobile scenario uses real settings
GET/PATCH persistence, admin/ordinary/unauthenticated header reloads, title/text
safety, both locales, explicit-local-language preservation, new-user default
locale/effective timezone, real empty inventory, loading and narrow asserted
error/retry cases. No extra sign-in budget, mock successful acceptance values,
seed API, SQL browser workspace fixture or resource-route implementation.
Populated inventory rendering is a **unit test**, not full AT-112.
Full AT-111/322 awaits real uploads (T2.2); full AT-112 awaits real workspace
creation/navigation/table APIs (T1.10/T1.11); full AT-117 still awaits datetime/
filter UI. T1.8 stays In progress until linked acceptance really passes.

### T1.9 physical DDL checks

`internal/ddl` uses the same package-level Suite and no test-only production
endpoint. Required uncached focused validation (PowerShell):

```powershell
$env:PT_TEST_DATABASE_URL = (& go run .\tools\ptdev db status | Select-Object -Last 1)
$env:PT_TEST_DATABASE_REQUIRED = '1'
go test .\internal\ddl -count=3 -cover
```

Tests cover exact prefix/alphabet/50-bit entropy boundaries and rejected
identifiers/types/defaults; emitted DDL grammar and sanitized error shapes;
real PG18 native types/defaults/catalogs (including PG18 NOT NULL constraints);
BY DEFAULT identity/explicit restore IDs, nullable authorship/no FK/no trigger;
add/drop/set/remove data preservation, NULL-distinct UNIQUE, no CASCADE;
combined metadata/DDL commit and rollback and metadata-only rename evidence.
Injection literals are exercised with standard_conforming_strings on **and**
off. Varchar defaults are character-bounded before explicit casting to avoid
PostgreSQL's otherwise silent cast truncation.

The contention test first observes the blocking PID using pg_blocking_pids,
then requires the actual three-second lock timeout and verifies rollback of
both pending field metadata/schema version and physical column. A same-session
test verifies LOCAL timeout reset on commit and rollback. All physical objects
are confined to the Suite-owned disposable DB, not the developer DB.

Then run the full `ptdev check` and `ptdev e2e` gates. Record a default-parallel
cleanup failure separately from a command-scoped `GOFLAGS=-p=1` workaround;
never change shared cleanup timeouts/assertions to produce a pass.
Catalog tests are not real AT-202 or full AT-804 API acceptance: table/field
rename and filter injection require T1.10/T1.14/T2.6. See the exact results,
resource observations and cleanup in the
[T1.9 completion record](task-prompts/T1.9.md#implementation-and-handoff-2026-10-26).

### T1.10 structure API checks

`internal/structure` has its own Suite-owned PG18 database; `internal/httpapi`
adds real HTTP scenarios through the production router (no seed or test route).
Required uncached focused validation (PowerShell):

```powershell
$env:PT_TEST_DATABASE_URL = (& go run .\tools\ptdev db status | Select-Object -Last 1)
$env:PT_TEST_DATABASE_REQUIRED = '1'
go test .\internal\structure -count=1 -cover
go test .\internal\httpapi -count=1 -run TestStructureAPI -v
```

Service tests cover strict inputs, exact physical/metadata state (catalog columns,
identity/defaults, no FKs, six fields, display field, Grid config, locale labels,
no physical names in DTOs), metadata-only Chinese/payload renames with data
preserved, deterministic reorder (including renumbering and trashed siblings),
the owner/visibility matrix, soft-delete retention and immediate cache
invalidation, scripted collision retry bounds (physical and metadata) and
rollback after deliberately invalid DDL. Concurrency tests hold the workspace
lock in a separate transaction to prove grant revoke/downgrade, book/workspace
delete and user-disable races are rechecked after locking; a real three-second
lock timeout; serialized parallel creations; and deferred-trigger commit
failures that must still invalidate and persist nothing. HTTP tests cover
AT-201/203/213 at the API surface, admin without implicit access plus inventory
owner, the full level matrix, group-derived ownership, and the unauthenticated/
disabled/forced-change/CSRF/no-store/bounded-JSON/query/method classes.

Sidebar UI parts of AT-201/203/213, full AT-112, AT-202 field rename and full
AT-804 are **not** claimed; see
[T1.10 completion record](task-prompts/T1.10.md#implementation-and-handoff-2026-10-26).

### T1.11 navigation checks

The navigation store/component/resource-route tests cover coalescing, forced
refresh, stale responses and identity resets, owner-of-parent controls,
path-only contexts, same-parent drag and button reorder, Unicode name bounds,
localized failures, mobile drawer lifecycle, schema shortcut/view validation
and explicit initial-load errors. Run from the root:

```powershell
pnpm --dir web exec vitest run src\stores\nav.test.ts src\components\navigation\navigation.test.ts src\pages\ResourcePage.test.ts src\router\guards.test.ts
pnpm --dir web lint
pnpm --dir web build
go run .\tools\ptdev check
$env:GOFLAGS = '-p=1'; go run .\tools\ptdev check
$env:GOFLAGS = '-p=1'; go run .\tools\ptdev e2e
```

The expanded existing admin scenario reuses real Alice/admin sessions and the
real Bob/Carol accounts, activating participants through login/password/login.
No SQL, seeded workspace, mocked nav/schema/grant, auth bypass or changed login
budget is used. Alice creates W/B/T via UI, reads real Grid/Name schema, renames
to Chinese, reorders through drag or mobile buttons, and confirms/cancels Trash
deletion. Bob's read-only controls, Carol's table-only ancestor pruning and
workspace creation, admin sidebar/table-GET denial versus populated read-only
inventory, and concurrent removal errors are checked on desktop/mobile.

D-107 raises only the expanded scenario/global **harness** bounds to 75/180 s
for additional serial actions. Existing request, assertion, dashboard <=10 s,
login-budget and PG Suite cleanup deadlines are unchanged; one worker/no retries
and fatal unexpected console/CSP diagnostics remain. Report default-parallel
cleanup sensitivity separately from the command-scoped serial pass.
Full AT-111/322, AT-117, AT-202 field rename and AT-804 filter APIs remain pending;
M3 owns AT-203 restoration, not this M1 deletion scenario.

### T1.12 sharing checks

T1.12 extends the same real account contexts with
[sharing scenarios](../web/e2e/sharing.ts). D-112 supersedes D-107's harness
bounds only (150 s expanded scenario, 360 s suite); all other thresholds remain.

```powershell
$env:PT_TEST_DATABASE_URL = (& go run .\tools\ptdev db status | Select-Object -Last 1)
$env:PT_TEST_DATABASE_REQUIRED = '1'
go test .\internal\acl .\internal\httpapi -count=1
pnpm --dir web exec vitest run src\stores\sharing.test.ts src\components\sharing\sharing.test.ts src\components\navigation\navigation.test.ts src\pages\ResourcePage.test.ts
pnpm --dir web lint
pnpm --dir web test
pnpm --dir web build
go run .\tools\ptdev check
$env:GOFLAGS = '-p=1'; go run .\tools\ptdev check
$env:GOFLAGS = '-p=1'; go run .\tools\ptdev e2e -- admin-users.spec.ts
$env:GOFLAGS = '-p=1'; go run .\tools\ptdev e2e
```

The directory test uses `internal/testdb.Suite` with real PG18 for owner-only
privacy/bounds, literal search, active filtering, uncached grant revocation,
hidden/visible/forced/unauthenticated errors and trashed ancestors. SQL fixtures
here are lower-layer evidence, not browser acceptance. Static queries regenerate
with the existing pinned sqlc 1.31.1 command; no migration/dependency.

Store/component tests cover stale target/query/type/identity responses, exact
search bounds, CSRF/review/write serialization, uncertain-write reconciliation,
last-owner/principal errors, redaction/retry, disabled/dangling grants and
target-owner versus parent-owner controls. The production-image scenario uses
real users/groups/grants and membership APIs with no mocked route, SQL seed,
additional login budget or row/editor proxy acceptance. Browser diagnostics
remain strict for all retained user tabs; only explicitly expected status errors
are consumed. Default-parallel Suite cleanup sensitivity must still be reported
separately from command-scoped serial results.

### Browser setup and isolated E2E

From the root, after installing frontend dependencies:

```text
pnpm --dir web e2e:install
go run ./tools/ptdev e2e
go run ./tools/ptdev e2e -- --project=desktop-chromium --grep theme
```

Chromium is installed explicitly into Playwright's laptop cache; Linux may also
need Playwright's documented browser OS libraries. Missing tools/browser binaries
fail visibly with installation guidance. Firefox/WebKit/full browser-matrix
certification is not part of M0.

Each run builds its own `0.0.0-pt-e2e-*` tag (never overwrites `latest`), chooses
a random project/password and available loopback port, and supplies an empty
private env file so no `deploy/.env` secrets are read. The local override creates
only project-scoped networks/volumes. Startup plus exact `/healthz` JSON readiness
is bounded to 60 s; probes bypass proxies and reject redirects. A port race is
reported as startup failure, not hidden by connecting to another app.

adjacent controls, no page overflow, and no console/page/CSP errors. API fixtures
The real embedded shell is checked on desktop and mobile Chromium (one worker,
zero retries, 600 s global bound per D-155): health/JS/history/404/HEAD/method boundaries,
English/Chinese and light/dark/system persistence, actual Naive UI styles,
adjacent controls, no page overflow, and no console/page/CSP errors. AT-101 now
uses the production image and the logged token from the unique owned app
container; ptdev passes it only to Playwright and redacts it from failure logs.
T1.5 adds login/forced-change and admin-user browser acceptance; profile and
workspace/business flows remain deferred. Browser users are seeded only via the
real admin API, never SQL or a test-only production endpoint.

T0.8 adds [grid-spike.spec.ts](../web/e2e/grid-spike.spec.ts) to this same
production-image suite. It verifies opt-in isolation, 200k logical-row
window/cache/DOM bounds, real clipboard editing/fill, groups, frozen columns,
locales/themes, cancellation/remount behavior and one explicit tablet touch
observation. Mobile verifies **no grid mounted**; desktop-only clipboard and
the duplicate tablet case in the mobile project are intentionally skipped.
Chromium CDP records actual JS heap, shared listener inventories and touch
input. These observations are not Firefox/WebKit, physical-device or real
database/query acceptance. Run the smaller suite during grid work with
`go run ./tools/ptdev e2e -- grid-spike.spec.ts --project=desktop-chromium`.

Every `pnpm build` now runs [check-bundle.mjs](../web/scripts/check-bundle.mjs)
against Vite's manifest. It includes both locales, welcome-route imports and
CSS in a conservative initial gzip total, requires **<600,000 bytes**, and
rejects engine chunks in that initial dependency set. The all-assets total
includes lazy engine/optional plugin chunks; it is reported separately.
The image-context allowlist includes only that required build script from
`web/scripts/`; it still excludes tests, local dependencies and secrets.
Pinned engine/adapter 4.29.1 add only two packages (MIT, no other runtime
dependencies); the workspace records their exact-version release-age exceptions.
Build tooling and browsers do not enter the runtime image.

The direct Node wrapper receives EOF from a private parent pipe on cancellation,
allowing Playwright to close its workers and browsers on Windows too (20 s
forced-stop fallback). Teardown uses independent contexts: diagnostics 10 s,
project `down --volumes` 60 s, unique image cleanup 15 s. Partial startup and
browser failures still run teardown; cleanup failures are visible and nonzero.
It never prunes, resets the dev DB or removes shared networks. Hard-killing the
ptdev process/machine is not a cooperative cancellation.

Ignored `web/test-results/<project>/` retains failure traces/screenshots,
browser diagnostics and redacted `compose.log`; command output includes the
project, URL, timings and results path. A direct `pnpm --dir web e2e` requires
`PT_E2E_BASE_URL` for an already-owned HTTP `127.0.0.1` stack; ptdev normally
sets this and the results directory automatically.

For real failure/cancellation regressions, first ensure `ptdev image build` has
provided `localhost/pandatable:latest`, then set `PTDEV_TEST_E2E=1` and run:

```text
go test ./tools/ptdev -run "TestE2EFailureAndCancellationIntegration|TestE2EStartupFailureIntegration" -v -count=1
```

These opt-in tests deliberately fail a browser assertion, cancel an active
browser run, and cause a real partial Compose startup failure with an occupied
port. They assert owned resources/listeners and recorded runner/worker/browser
PIDs are gone. Their temporary config lives under ignored test-results, not
frontend source; the deliberate lifecycle probe is not selected by normal E2E.
Run resource-heavy lifecycle checks separately from the fast gate on a small
host. Ordinary unit tests cover arguments, child EOF/reaping, readiness,
step ordering, cancellation and cleanup-error propagation without containers.

## 4. Release procedure

1. `go run ./tools/ptdev check` → green.
2. `go run ./tools/ptdev e2e` → green (milestone releases).
3. `git tag vX.Y.Z` (SemVer, see conventions).
4. `go run ./tools/ptdev image build && go run ./tools/ptdev image save` (add `--with-postgres` on first deploy or when the server can't reach Docker Hub — common for mainland-China hosts).
5. Transfer: `scp dist/pandatable-X.Y.Z.tar.gz user@server:/opt/pandatable/images/` (or `ptdev deploy`).
6. On server: see [13-deployment.md §6](13-deployment.md#6-upgrades).
7. Update `progress.md` (milestone/tag) and change log.

## 5. Image build (`deploy/Dockerfile`)

[deploy/Dockerfile](../deploy/Dockerfile) is the source of truth, not a separate
copy/paste sketch. It builds Vite with Node 24.15.0 + pnpm 11.2.2, copies
`package.json`, `pnpm-lock.yaml` **and** `pnpm-workspace.yaml` before frozen
installation, then compiles with Go 1.27.0 (`GOTOOLCHAIN=local`,
`CGO_ENABLED=0`, `linux/amd64`, stripped/trimmed, version embedded).

The Debian bookworm-slim runtime installs PostgreSQL client **major 18** from
the signed PGDG HTTPS repository, plus CA/timezone data. Client patch/security
updates are deliberately obtained at build time. A separate throwaway stage
fetches PGDG's public signing key; curl, Node, Go and build dependencies do not
enter the runtime. The process runs as UID/GID 10001 with owned persistent
`/data` subdirectories and an executable-only healthcheck.

[.dockerignore](../.dockerignore) is an allowlist of app/migration/frontend source
and manifests, with additional secret/generated/test exclusions. Host assets,
node_modules, data, docs, tools, Git metadata and local configuration are not
the build context. Real assets come only from the frontend stage.

Optional `GOPROXY`, `NPM_REGISTRY`, `APT_MIRROR`, `PGDG_MIRROR` environment
variables are passed as named build arguments without echoing their values in
ptdev's command lines. `APT_MIRROR` replaces Debian's base repository URL
(default `http://deb.debian.org/debian`); `PGDG_MIRROR` replaces the PGDG apt
repository URL (default `https://apt.postgresql.org/pub/repos/apt`) while the
signing key is still fetched from postgresql.org and enforced by `signed-by`
(D-156).
Use public/credential-free mirror URLs: build args are not secret storage.
No dependencies or hosted CI are added.
