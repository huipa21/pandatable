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

The real embedded shell is checked on desktop and mobile Chromium (one worker,
zero retries, 120 s global bound): health/JS/history/404/HEAD/method boundaries,
English/Chinese and light/dark/system persistence, actual Naive UI styles,
adjacent controls, no page overflow, and no console/page/CSP errors. API fixtures
are ready for later real user seeding, but auth/setup/business/seed APIs and their
acceptance tests are intentionally not implemented or claimed.

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
