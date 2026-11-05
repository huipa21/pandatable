# 14 — Development, Build & Release

There is **no hosted CI** (D-048). Everything — checks, tests, image build, image export — runs on the developer laptop (Windows 11 + Podman). The resulting image archive is copied to the Linux server (Docker) and loaded there.

## 1. Developer prerequisites (Windows laptop)

| Tool | Version | Notes |
|---|---|---|
| Go | 1.27.0 (module/toolchain floor) | `GOPROXY` may be set to `https://goproxy.cn,direct` if needed |
| Node.js | >= 22.13 + pnpm 11.2.2 (via Corepack) | Tested with Node 24.15.0; npm registry mirror optional (`.npmrc`) |
| Podman Desktop / podman | ≥ 5.x, WSL2 available | `ptdev` starts the machine automatically; `podman compose` requires `podman-compose` or docker-compose v2 binary as provider |
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
| `db up` / `db down` / `db reset` | Run dev PostgreSQL 18 container `pt-dev-db` on IPv4 loopback (prefers port `55432`, automatically selects a free port; volume `pt-dev-pgdata`) |
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
dev DB); otherwise it starts/reuses the dev DB and passes its actual published
port to the backend. `test`/`check` likewise pass the resolved DSN to integration
tests; no manual environment changes are needed after a DB port fallback. Defaults: backend
`PT_LISTEN=127.0.0.1:8080`, `PT_BASE_URL=http://127.0.0.1:5173`, Vite on
`127.0.0.1:5173` with strict-port binding. Explicit app settings retain precedence.
Vite's API target is derived from `PT_LISTEN` (wildcard hosts mapped to loopback);
port zero is rejected. Standalone `pnpm dev` uses `PT_DEV_API_TARGET` or
`http://127.0.0.1:8080`. No WebSocket server is implemented.

Before DB startup or compilation, `ptdev dev` checks that the backend and Vite
addresses can be bound. An occupied address fails with an instruction to stop
the existing listener; it never kills another dev session or silently changes
ports. Stop the earlier `ptdev dev` with Ctrl+C in its terminal before restarting.
On Windows, identify listeners with
`Get-NetTCPConnection -LocalPort 8080,5173 -State Listen | Select-Object LocalPort,OwningProcess`.
Check those PIDs before stopping anything; an existing dev session may already
be serving the UI. The probes release their ports immediately, so a later bind
race still fails through normal child supervision.

Vite redirects browser page navigation between loopback aliases (`localhost`,
`127.0.0.1`, `[::1]`) to the exact local HTTP `PT_BASE_URL` origin (D-208).
`ptdev dev` passes the resolved base URL to both processes and prints that URL.
Standalone `pnpm dev` must use the same `PT_BASE_URL` as the backend (default
`http://127.0.0.1:5173`). Reload an already-open page after changing this setting.
API requests and their Origin headers are never rewritten: a mismatched origin
still returns `403 ERR_CSRF`; setup displays the localized security-check error.

The supervisor compiles once (restart it for Go changes); Vite provides frontend
HMR. Both servers are direct children, not `go run`/`pnpm` grandchildren. Either
child exit/start failure stops the other and returns a visible error. Ctrl+C or
SIGTERM cancels both and waits for reaping before removing the temporary binary.
Interrupt is attempted where supported, with forced termination otherwise;
the backend has a 12-second wait bound (covering its 10-second HTTP drain),
Vite three seconds. Windows does not support Go's `os.Interrupt` delivery to
children, so cancellation forcibly stops the direct children there.
After reaping, temporary-directory deletion retries Windows sharing/lock and permission errors
for up to two seconds because executable image mappings can briefly remain
locked. Persistent cleanup failures are still reported with the directory path.
On Windows, `ptdev dev` also places itself in a kill-on-close Job Object that its
children inherit, so closing or killing the terminal (or the `go run` wrapper)
terminates the backend and Vite instead of leaving them holding the ports.
Automatic engine startup happens before joining that job, so stopping the dev
supervisor does not terminate a newly launched container engine.

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

The default dev DB DSN is
`postgres://postgres:postgres@127.0.0.1:55432/postgres?sslmode=disable`.
`db up` prints the actual DSN; `db status` reads it from the existing container
(and prints no DSN when absent). A running DB is reused without probing its
already-bound port. A stopped DB keeps its current port if available; otherwise
`ptdev` recreates only its container with the same image and named data volume,
never deleting the volume. Automatic recreation refuses unexpected mounts.
New containers prefer `55432`, then use an OS-selected free loopback port.
Bind races reported by the engine trigger at most three startup attempts;
unrelated failures remain errors. No existing listener is terminated.
These fallback rules apply to the database port, not backend/Vite ports, whose
explicit conflict handling above is unchanged (D-213/D-216).

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
`test --no-db` remain usable. Help and `engine` only check PATH and never start services. Before DB, image or
E2E operations, `ptdev` checks `<engine> info` (five-second probe timeout).
If unavailable, it automatically starts the selected local engine and waits
up to two minutes, respecting Ctrl+C (D-216):

- Podman on Windows/macOS: start the default machine (or the only machine);
  initialize one first if none exists. An already-running or starting machine is given time
  to reconnect, not restarted. Multiple machines without a default fail explicitly.
- Docker on Windows/macOS: `docker desktop start` (requires Docker Desktop
  with its Desktop CLI installed).
- Docker Engine on Linux: `systemctl start docker`, using existing permissions;
  no automatic elevation or `sudo`.

Linux Podman is daemonless, so a failed health check reports a configuration/
remote-connection error rather than starting an unrelated service. Missing
executables, startup/permission errors, unreachable remote engines and readiness
timeouts remain explicit errors; `ptdev` does not install engines, switch
connections or fall back from an explicit `PT_ENGINE`.

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

### Gate levels (D-158)

| Level | When | Commands |
|---|---|---|
| Inner loop | every edit | `go build ./...`; `go test -run <Name> ./internal/<pkg> -count=1` with explicit `PT_TEST_DATABASE_URL` + `PT_TEST_DATABASE_REQUIRED=1` (`ptdev db up` first); `pnpm --dir web exec vitest run <file>` |
| Task gate | end of each task | `go run ./tools/ptdev check` (default parallelism, never `GOFLAGS=-p=1`, D-145/D-146) and `go run ./tools/ptdev e2e --fast -- <touched specs>` |
| Phase gate | once at phase/milestone end, before any tag | complete production-image `go run ./tools/ptdev e2e` |

`e2e --fast` runs `ptdev build --linux` on the host (web build, embedded
assets, Linux binary with warm caches), then builds the Dockerfile's
`prebuilt` target, which copies `bin/pandatable-linux-amd64` onto the same
`runtime` stage as the release image. Everything after the build — disposable
tag, isolated Compose stack, health readiness, setup token, Playwright specs
and teardown — is identical to the full run. It cannot be combined with
`--perf`. A task that changes `deploy/Dockerfile`, Compose files,
`go.mod`/`go.sum` or `pnpm-lock.yaml` must also build the release target once
(the full `ptdev e2e`, or `podman build -f deploy/Dockerfile` with a throwaway
tag). A phase-gate failure reopens the affected tasks.

Write long gate output to a log file and search it; Playwright prints the
compact `dot` reporter (failures in full) and writes per-test durations to
`web/test-results/<project>/report.json`.

Per-task focused gate commands for M0–M1 are archived in
[archive/dev-checks-m0-m1.md](archive/dev-checks-m0-m1.md).

### T2.15 linked-query performance (D-182)

With an explicitly configured PG18 test database administrator URL, run:

```powershell
$env:PT_TEST_DATABASE_URL = '<explicit PG18 test administrator URL>'
$env:PT_TEST_DATABASE_REQUIRED = '1'
$env:PT_RUN_QUERY_PERF = '1'
go test .\internal\httpapi -run '^TestLinkedQueryHTTPPerformance200k$' -count=1 -v
go test .\internal\structure -run '^(TestTracedQueryBoundArgs|TestQueryPerformance200k)$' -bench '^BenchmarkQueryPerformance200k$' -benchtime=1x -count=1 -v
Remove-Item Env:\PT_RUN_QUERY_PERF
```

The Suite creates and drops its own isolated database. Real structure and
relation services create metadata/FKs, deterministic SQL seeds 200,000 tasks
and 1,000 projects, and the shared-view index scan creates applicable indexes
before measurement. Run the two commands sequentially to avoid competing
loads. The HTTP test measures first and repeated authenticated loopback
requests through the complete response-body read, including server
serialization; the service test and Go benchmark measure the underlying
calls separately. Both tests validate counts, ordering, group sums and visible
links, strictly compare individual samples to <1 s, and print actual data,
count and link statements, bound arguments and `EXPLAIN (ANALYZE, BUFFERS)`.
The Go benchmark also validates results, reports allocations and excludes
setup. Seeding, index creation and `ANALYZE` warm caches; no cold-cache result
is claimed. Loopback measurements exclude TLS, reverse proxy and browser
overhead and are not AT-312/`e2e --perf`; target-host acceptance requires the
documented 2-vCPU/4-GB host.


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
the connected database name, major 18, no workspaces **including Trash** (or
only the owner's untouched, unshared `Default` starter with no books), an
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


### Browser setup and isolated E2E

From the root, after installing frontend dependencies:

```text
pnpm --dir web e2e:install
go run ./tools/ptdev e2e
go run ./tools/ptdev e2e --fast -- grid.spec.ts --project=desktop-chromium
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

The Go build cache and pnpm store use `RUN --mount=type=cache` (Podman/buildah
and Docker BuildKit), so source edits recompile incrementally. The runtime is a
named `runtime` stage; the release image is the final (default) stage. The
`prebuilt` stage is only for `ptdev e2e --fast` (D-158) and is the one reason
`bin/pandatable-linux-amd64` is allowlisted; the release stage never reads it.

Optional `GOPROXY`, `NPM_REGISTRY`, `APT_MIRROR`, `PGDG_MIRROR` environment
variables are passed as named build arguments without echoing their values in
ptdev's command lines. `APT_MIRROR` replaces Debian's base repository URL
(default `http://deb.debian.org/debian`); `PGDG_MIRROR` replaces the PGDG apt
repository URL (default `https://apt.postgresql.org/pub/repos/apt`) while the
signing key is still fetched from postgresql.org and enforced by `signed-by`
(D-156).
Use public/credential-free mirror URLs: build args are not secret storage.
No dependencies or hosted CI are added.
