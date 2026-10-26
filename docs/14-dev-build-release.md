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
| `e2e [-- <args>]` | Build a disposable image, start an isolated stack with the local override, run Playwright, retain failure artifacts, tear down only owned resources |
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
`sqlc generate` (also generates `internal/acl/aclsql` from the second
`sqlc.yaml` entry). Commit its Go bindings; generation must produce no diff on
rerun. [catalog.sql](../internal/auth/catalog.sql) declares the PG timezone
catalog only for the offline generator; no runtime SQL or migration executes it.
The application image uses existing Go source allowlists and does not include
the generator. Runtime additions are only `x/crypto` (argon2id) and `x/sys`.

By user-confirmed scope, no login/profile frontend or auth browser fixture is
shipped in T1.2. Existing `ptdev e2e` remains the real shell/spike gate; it is
**not** browser acceptance for AT-102/116. API coverage and stored profile
timezone/default checks do not complete full AT-117's real datetime/filter/
admin-default scenarios. T1.2 remains in progress until linked acceptance passes.

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
zero retries, 120 s global bound): health/JS/history/404/HEAD/method boundaries,
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

Optional `GOPROXY`, `NPM_REGISTRY`, `APT_MIRROR` environment variables are
passed as named build arguments without echoing their values in ptdev's
command lines. `APT_MIRROR` replaces Debian's base repository URL (default
`http://deb.debian.org/debian`); PGDG remains its official HTTPS repository.
Use public/credential-free mirror URLs: build args are not secret storage.
No dependencies or hosted CI are added.
