# 14 — Development, Build & Release

There is **no hosted CI** (D-048). Everything — checks, tests, image build, image export — runs on the developer laptop (Windows 11 + Podman). The resulting image archive is copied to the Linux server (Docker) and loaded there.

## 1. Developer prerequisites (Windows laptop)

| Tool | Version | Notes |
|---|---|---|
| Go | ≥ 1.25 | `GOPROXY` may be set to `https://goproxy.cn,direct` if needed |
| Node.js | 22 LTS + pnpm (via corepack) | npm registry mirror optional (`.npmrc`) |
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
| `db up` / `db down` / `db reset` | Run dev PostgreSQL 18 container `pt-dev-db` on `127.0.0.1:55432` (volume `pt-dev-pgdata`) |
| `dev` | Start backend (`go run ./cmd/pandatable serve`) with `PT_DATABASE_URL` pointing to dev DB, and Vite dev server (proxy `/api` → `:8080`, ws included) |
| `web build` | `pnpm install --frozen-lockfile && pnpm build`, copy `web/dist` → `internal/web/dist` |
| `build` | `web build` then `go build -trimpath -ldflags "-s -w -X main.version=…" -o bin/pandatable[.exe] ./cmd/pandatable` |
| `lint` | `gofmt -l`, `go vet`, `golangci-lint run` (if installed), `pnpm lint`, `vue-tsc --noEmit`, i18n key parity |
| `test` | `go test ./...` (unit + integration; integration uses dev DB, see §3), `pnpm test` (Vitest) |
| `e2e` | Build image, start stack with `compose.local.yaml`, run Playwright, tear down |
| `check` | `lint` + `test` (the "CI" gate before tagging/release) |
| `image build` | Build `localhost/pandatable:<ver>` + `:latest` for `linux/amd64` |
| `image save [--with-postgres]` | Export image(s) to `dist/pandatable-<ver>.tar.gz` (and `dist/postgres-18.tar.gz`) |
| `deploy --host user@server [--dir /opt/pandatable]` | Optional: `scp` archive(s) + `ssh docker load` + update `PT_VERSION` + `docker compose up -d app` |
| `version` | Print computed version (`git describe --tags --dirty`, fallback `0.0.0-dev`) |

### Container engine detection
1. `PT_ENGINE` env (`podman` | `docker`) if set.
2. Else `podman` if on PATH, else `docker`.
3. Compose: `<engine> compose` (both engines support the subcommand).

Engine-specific flags handled by ptdev:

| Concern | Podman | Docker |
|---|---|---|
| Build | `podman build --format docker --platform linux/amd64 -f deploy/Dockerfile -t localhost/pandatable:<ver> .` (`--format docker` keeps `HEALTHCHECK`) | `docker build --platform linux/amd64 -f deploy/Dockerfile -t localhost/pandatable:<ver> .` |
| Save | `podman save --format docker-archive -o …tar localhost/pandatable:<ver>` then gzip | `docker save -o …tar localhost/pandatable:<ver>` then gzip |
| Load on server | `docker load -i pandatable-<ver>.tar.gz` (gzip accepted) | same |

The `localhost/` prefix is used for both engines so names are identical after load.

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

## 5. Dockerfile outline (`deploy/Dockerfile`)

```dockerfile
# 1) web
FROM docker.io/library/node:22-bookworm-slim AS web
WORKDIR /src/web
COPY web/package.json web/pnpm-lock.yaml ./
RUN corepack enable && pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

# 2) go
FROM docker.io/library/golang:1.25-bookworm AS go
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./internal/web/dist
ARG VERSION=0.0.0-dev
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/pandatable ./cmd/pandatable

# 3) runtime (pg_dump 18 from PGDG for backups)
FROM docker.io/library/debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl gnupg tzdata \
 && install -d /usr/share/postgresql-common/pgdg \
 && curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc -o /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc \
 && echo "deb [signed-by=/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc] http://apt.postgresql.org/pub/repos/apt bookworm-pgdg main" > /etc/apt/sources.list.d/pgdg.list \
 && apt-get update && apt-get install -y --no-install-recommends postgresql-client-18 \
 && apt-get purge -y curl gnupg && apt-get autoremove -y && rm -rf /var/lib/apt/lists/*
RUN useradd -r -u 10001 -d /data pandatable && mkdir -p /data && chown pandatable /data
COPY --from=go /out/pandatable /usr/local/bin/pandatable
USER pandatable
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD ["pandatable", "healthcheck"]
ENTRYPOINT ["pandatable"]
CMD ["serve"]
```

Notes: `pandatable healthcheck` sub-command does an HTTP GET to `/healthz` (no curl needed). Base images are fully qualified for Podman. Build args for mirrors (`GOPROXY`, `NPM_REGISTRY`, `APT_MIRROR`) supported by ptdev via env passthrough.
