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
| `test [--no-db]` | Start dev DB and set `PT_TEST_DATABASE_URL` for `go test ./...`; `--no-db` skips DB startup; forwards `-run`, `-v`, `-count`; `pnpm test` if present |
| `e2e` | Build image, start stack with `compose.local.yaml`, run Playwright, tear down |
| `check` | `lint` → `test` → `build`, with per-step pass/fail and duration summary; any failed step causes non-zero exit (D-051) |
| `image build [--version VERSION]` | Build `localhost/pandatable:<tag>` + `:latest` for `linux/amd64`; preserve original version in the binary |
| `image save [--version VERSION] [--with-postgres]` | Export version-tagged image to `dist/pandatable-<tag>.tar.gz`; optionally pull/export pinned PG18 to `dist/postgres-18.tar.gz` |
| `deploy --host user@server --yes [--dir /opt/pandatable] [--version VERSION] [--with-postgres]` | Optional: transfer existing archive(s), load, update only `PT_VERSION`, wait for Compose + internal health; no secret/config transfer |
| `version` | `git describe --tags --dirty --always`, strip leading `v`; untagged SHA becomes `0.0.0-dev+<sha>[.dirty]`; unavailable Git/repository falls back to `0.0.0-dev` |

Run ptdev from the repository root. Through T0.6, help, engine, version, DB,
build, lint, test, check, dev, standalone web build, image and deploy are implemented.
E2E still returns `not implemented yet (T0.7)` with exit 2.
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
The HTTP unit tests use an in-memory SPA fixture, and the CLI restart integration
test creates/drops its own disposable PostgreSQL 18 database. The shared
cross-package integration harness remains T0.7 work.

For the opt-in real dev-process test, create a disposable PostgreSQL 18 database
named `pt_test_*`, set `PTDEV_TEST_DEV_DATABASE_URL` to its URL, leave local port
5173 free, and run `go test ./tools/ptdev -run TestDevIntegration -v -count=1`.
It tests Vite/backend startup, health/API proxying and cancellation, asserting
both listeners close. Drop the disposable database afterwards. Ordinary runs
skip this test explicitly; it does not replace the T0.7 shared harness.

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
- **Integration:** tests needing PostgreSQL read `PT_TEST_DATABASE_URL` (default `postgres://postgres:postgres@127.0.0.1:55432/postgres`). The helper creates a fresh database `pt_test_<random>` per test package and drops it afterwards. If the DB is unreachable, integration tests are **skipped with a clear message** (so `go test` never hangs) — `ptdev test` starts the DB first, so the gate is real. No testcontainers (avoids Podman-socket issues on Windows).
- **E2E/acceptance:** Playwright against the real image via `compose.local.yaml`.
- Results of `ptdev check` + `ptdev e2e` are noted in `progress.md` when a milestone is closed.

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
