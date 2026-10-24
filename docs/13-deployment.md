# 13 — Deployment & Operations

Build happens on the developer laptop (Windows + Podman); the image is saved to a tar archive, copied to the server and loaded into Docker. See [14-dev-build-release.md](14-dev-build-release.md) for the build side. This doc covers the **server** side.

## 1. Target host

Tencent Cloud Lighthouse-class: 2 vCPU, 4 GB RAM, Ubuntu 24.04 LTS x86_64, Docker Engine + Compose plugin. An **existing, independent Caddy container** already reverse-proxies all apps on the host (D-046).

## 2. Containers

| Service | Image | Networks | Notes |
|---|---|---|---|
| `db` | `docker.io/library/postgres:18.6-bookworm` | `default` (private) | volume `pgdata`; custom `postgresql.conf`; no published ports |
| `app` | `localhost/pandatable:${PT_VERSION}` (loaded from tar) | `default` + external `proxy` | Debian slim + `postgresql-client-18` (pg_dump/pg_restore); volume `/data`; no published ports |

Caddy is **not** part of this stack. The app joins Caddy's existing network so Caddy can reach it by alias `pandatable:8080`.

### Compose files and invocation paths

The executable configuration is [deploy/compose.yaml](../deploy/compose.yaml),
with [compose.local.yaml](../deploy/compose.local.yaml) for laptop smoke tests.
Both engines use simple `depends_on`; the application's existing bounded DB
retry policy handles startup readiness. App stop grace is 15 s (HTTP drain is
10 s); DB stop grace is 30 s. Memory limits are 512 MB and 1600 MB.

From the **repository root**, copy `deploy/.env.example` to `deploy/.env`,
fill the required password, then run:

```
podman compose --env-file deploy/.env -p pandatable -f deploy/compose.yaml -f deploy/compose.local.yaml up -d
```

From **inside deploy/**, the equivalent is
`podman compose --env-file .env -p pandatable -f compose.yaml -f compose.local.yaml up -d`.
The earlier root-level `-f compose.yaml` sketch was incorrect: there are no
root-level Compose files. On the server the copied files sit together in
`/opt/pandatable`, so `docker compose up -d` runs from that directory.

The local override publishes only `127.0.0.1:${PT_LOCAL_HTTP_PORT:-8080}`,
sets the matching HTTP base URL, and creates a project-scoped private proxy
network (`<project>-proxy-local`), not the shared Caddy network. Use a unique
`-p` and local port for disposable tests. No fixed container/volume names are
used. Neither server service publishes a host port.

Notes:
- PG 18 official image keeps data under `/var/lib/postgresql/18/docker` (volume mounted at `/var/lib/postgresql`), easing future major upgrades.
- Image names are fully qualified (`docker.io/library/...`, `localhost/...`) so Podman and Docker resolve them identically (Podman does not assume Docker Hub; `podman save` → `docker load` keeps the `localhost/` prefix).
- The app uses UID/GID 10001, owns `/data/files`, `/data/backups`, `/data/tmp`,
  and initializes named volume ownership from the image. For a bind mount,
  provision that directory's ownership yourself; the app does not run as root
  or silently repair shared host directories.
- Copy [deploy/.env.example](../deploy/.env.example); it intentionally has no
  usable password. Generate a unique random password (e.g. 32 random bytes in
  hex). `PT_DB_PASSWORD` must use URL-unreserved characters only
  (`A-Z a-z 0-9 . _ ~ -`) because Compose inserts it in a PostgreSQL URL.
  Do not print `compose config` without `--quiet` or commit the completed file.
  An explicitly empty `PT_TRUSTED_PROXIES` remains empty (trust no proxies).

### Caddy configuration (in the user's existing Caddy)

```caddyfile
table.example.com {
    encode zstd gzip
    reverse_proxy pandatable:8080
}
```
- Caddy proxies WebSockets automatically; no extra config.
- If the Caddy container is not on a user-defined network yet: `docker network create caddy` and attach Caddy to it; set `PT_PROXY_NETWORK` accordingly.
- Caddy sets `X-Forwarded-For`/`-Proto`; the app trusts these only from `PT_TRUSTED_PROXIES` (used for rate limiting & session IP).
- Large uploads/imports: Caddy has no default body limit; the app enforces limits.

## 3. PostgreSQL tuning (4 GB host) — deploy/postgresql.conf

```
listen_addresses = '*'            # only reachable on the private compose network
max_connections = 40
shared_buffers = 768MB
effective_cache_size = 2GB
work_mem = 16MB
maintenance_work_mem = 256MB
wal_buffers = 16MB
checkpoint_completion_target = 0.9
max_wal_size = 2GB
random_page_cost = 1.1            # SSD cloud disk
effective_io_concurrency = 100
io_method = worker                # PG18 async I/O (default)
autovacuum = on
timezone = 'UTC'                  # storage/session in UTC; app converts per user (D-049)
idle_in_transaction_session_timeout = 60s
statement_timeout = 30s           # app overrides for import/backup/DDL
shared_preload_libraries = 'pg_stat_statements'
```
Extensions: `pg_trgm`, `pg_stat_statements` (created by migration).

## 4. App configuration

| Env | Default | Meaning |
|---|---|---|
| `PT_DATABASE_URL` | — | Postgres DSN (required) |
| `PT_DATA_DIR` | `/data` | `files/`, `backups/`, `tmp/` |
| `PT_LISTEN` | `:8080` | listen address |
| `PT_BASE_URL` | — | public URL; Origin/CSRF check, cookie `Secure` when https |
| `PT_TRUSTED_PROXIES` | `10.0.0.0/8,172.16.0.0/12,192.168.0.0/16` | CIDRs whose `X-Forwarded-*` headers are trusted |
| `PT_DB_MAX_CONNS` | `20` | pgx pool size |
| `PT_LOG_LEVEL` | `info` | slog level, JSON logs to stdout |
| `PT_DB_CONNECT_RETRIES` | `30` | startup retries (2 s apart) while DB comes up |

Server-side `.env` (next to compose.yaml): `PT_VERSION`, `PT_DB_PASSWORD`, `PT_BASE_URL`, `PT_PROXY_NETWORK`.

App-level defaults configured in Admin → Settings (stored in DB, not env): `default_timezone` (Asia/Shanghai), `default_currency` (CNY), `default_locale`, upload limit, retention days (D-049, D-050).

`serve` and `migrate` accept matching kebab-case flags (`--database-url`, `--data-dir`, `--listen`, `--base-url`, `--trusted-proxies`, `--db-max-conns`, `--log-level`, and `--db-connect-retries`). A supplied flag overrides its environment variable; environment overrides the default. An explicitly empty trusted-proxy list trusts no proxies.

`serve` connects using the existing retry policy and runs the embedded,
advisory-locked migrations before opening the HTTP listener. A migration or bind
failure exits nonzero with a JSON error; database URLs/passwords are not logged.
`GET /healthz` returns 200 with `{"status":"ok"}` once listening, without issuing
a DB query. Before T0.5 supplies a built SPA, UI routes explicitly return 503
and startup logs warn that assets are unavailable.

`pandatable healthcheck [--listen host:port]` reads only `PT_LISTEN` (default
`:8080`); unrelated/missing DB and base URL settings do not affect it. Wildcard
listeners map to IPv4/IPv6 loopback. The probe ignores HTTP proxy variables,
does not follow redirects, requires HTTP 200 from `/healthz`, and fails visibly
with exit 1 within 2 s. Invalid flags exit 2. The image uses the executable
directly, with no shell/curl dependency (10 s interval, 3 s engine timeout,
60 s start period, three retries). This is liveness, not ongoing DB readiness.

Interrupt (Ctrl+C on Windows) and SIGTERM (Linux/container stop) initiate graceful
shutdown. The server drains for up to 10 seconds without immediately canceling
active request contexts; after the deadline it cancels/closes remaining requests
and reports a shutdown failure. The database pool is then closed. Allow more
than 10 seconds in the future Compose stop grace period. HTTP header/read/write/
idle limits are 5/30/30/60 seconds, with headers limited to 1 MiB (D-055).

## 5. Server layout & first deploy

```
/opt/pandatable/
├── compose.yaml
├── postgresql.conf
├── .env
└── images/            # transferred tar.gz archives
```

1. Copy `deploy/compose.yaml`, `deploy/postgresql.conf`, `deploy/.env.example` → `/opt/pandatable/` (rename to `.env`, fill values).
2. Load images: `docker load -i images/pandatable-<ver>.tar.gz` (and `postgres-18.tar.gz` if the server cannot pull from Docker Hub — see 14 §4).
3. `docker compose up -d`.
4. `docker compose logs app | grep SETUP_TOKEN` → open `PT_BASE_URL/setup`, paste token, create first admin.
5. Add the site block to the existing Caddyfile and reload Caddy.

The setup page/token are M1 work, not available in the M0 shell. Provision
the external network and Caddy site only with the shared server owner's
approval. `ptdev deploy` never creates/changes Caddy or its network.

## 6. Upgrades

```
docker load -i images/pandatable-<newver>.tar.gz
sed -i 's/^PT_VERSION=.*/PT_VERSION=<newver>/' .env
docker compose up -d app
```
Migrations run automatically on start (goose, in tx, advisory lock). Create a backup in Admin before upgrading. Rollback = set previous `PT_VERSION` (only if no migrations ran; otherwise restore backup).

Optional tooling (from the laptop repository root):
`go run ./tools/ptdev deploy --host user@server --dir /opt/pandatable --version X.Y.Z --yes`
(add `--with-postgres` to transfer/load its existing archive too). This requires
modern OpenSSH scp (SFTP default), noninteractive SSH credentials/known host,
remote Docker + Compose with `--wait`, and pre-provisioned Compose/config/.env
and Caddy network. `--host` accepts a DNS name, IPv4 or SSH alias, optionally
with a user; configure other SSH options/ports/IPv6 in SSH config via an alias.
`--dir` is an absolute POSIX path, including spaces/apostrophes if needed.

No deployment happens without both an explicit host and `--yes`. The tool checks
existing local archives and remote files/config, uploads only image archives to a
unique staging directory, loads them, updates the single `PT_VERSION=` line in
the existing `.env`, runs `docker compose up -d --wait --wait-timeout 60 app`,
then probes internal health. It never transfers secrets or replaces server
configuration. It removes only its staged archives/directory on success/failure,
with a 15 s cleanup bound, reporting cleanup failures too.

Failures stop the sequence; no fallback after an error, app rebuild or automatic
rollback is attempted. Without `--with-postgres`, Compose may pull its pinned DB
image if it is not already present. A failed load/start can leave loaded images or an updated version
and partially started stack; inspect the server before retrying. Internal
health does not prove external TLS/proxy reachability: verify `/healthz` and
the SPA through the real Caddy domain separately (AT-005).

## 7. Restore

See [11-import-export-backup.md §3](11-import-export-backup.md#restore-cli).

## 8. Monitoring (Admin → Dashboard)

Collected by `gopsutil` + PG queries, cached 5 s:
- CPU usage % and load avg (container sees host `/proc` values for CPU/load).
- Memory: host total/used/available; container limit (cgroup) and app RSS; Postgres connection count + shared_buffers.
- Storage: `pg_database_size()`, attachments dir size (walked hourly, cached), backups dir size, free/used space of the `/data` volume (= host disk where Docker stores volumes).
- Version info: app, Go, PostgreSQL, migration level; uptime.

## 9. Security checklist

- No published ports: DB reachable only on the private compose network; app reachable only via Caddy's network.
- HTTPS terminated by Caddy; `PT_BASE_URL` must be `https://…` in production (cookies `Secure`).
- Firewall: only 22/80/443.
- Setup token single-use; setup endpoint returns 410 after completion.
- Login rate limits (per IP from trusted `X-Forwarded-For`, per account); session idle timeout 7 days, absolute 30 days; sessions revoked on password reset/disable.
- Upload MIME sniffing; attachments served with `Content-Disposition: attachment` except safe image types; `X-Content-Type-Options: nosniff`; CSP on SPA.
