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
| `PT_BASE_URL` | — | public URL; Origin/CSRF check, Secure cookies for HTTPS; HTTP auth restricted to literal localhost/loopback |
| `PT_TRUSTED_PROXIES` | `10.0.0.0/8,172.16.0.0/12,192.168.0.0/16` | CIDRs whose `X-Forwarded-*` headers are trusted |
| `PT_DB_MAX_CONNS` | `20` | pgx pool size |
| `PT_LOG_LEVEL` | `info` | slog level, JSON logs to stdout |
| `PT_DB_CONNECT_RETRIES` | `30` | startup retries (2 s apart) while DB comes up |

Server-side `.env` (next to compose.yaml): `PT_VERSION`, `PT_DB_PASSWORD`, `PT_BASE_URL`, `PT_PROXY_NETWORK`.

Compose also accepts optional `PT_DB_NAME` (default `pandatable`), used by the
T1.20 isolated performance workflow for its uniquely named disposable seed DB.
Do not change it for an existing volume: it does not rename/reset the database.
The [developer seed](14-dev-build-release.md#t120-safe-seed-and-production-browser-baseline-d-143d-144)
is explicit CLI-only tooling, never a production startup action or HTTP route.

App-level defaults configured in Admin → Settings (stored in DB, not env): `default_timezone` (Asia/Shanghai), `default_currency` (CNY), `default_locale`, upload limit, retention days (D-049, D-050).

T1.8 implements these settings plus app name without a migration, restart or
new dependency. See [API contracts](09-api.md#t18-settings-and-workspace-inventory-d-091d-093)
for bounds/defaults and serialized atomic PATCH. Public no-store `/meta/app`
exposes only app name/default language, so every user's header/title follows
after reload without exposing privileged settings. Existing explicit user/local
preferences stay unchanged; NULL user timezones follow the new default.
Upload limit is **MiB** (default 20, range 1..100); history/trash retention
defaults are 90/30 days (range 1..3650). Upload enforcement uses this per-file
limit (T2.2). Both retention jobs run at startup and daily; Trash purge commits
database changes before retryable attachment-file cleanup and logs failures.
The image decode cap is `imageMaxMegapixels` (default 16, range 1..16). Settings
reads perform no filesystem walk and do not invalidate the dashboard's hourly
directory caches.

Admin workspace sizes are separately labeled **recorded logical attachment
bytes**, including retained Trash descendants, not the dashboard's disk/PG
measurements. The read-only metadata inventory never traverses attachment
paths or grants an administrator resource access.

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

Implemented in T1.7 (D-088..D-090) using pinned BSD-3-Clause
`github.com/shirou/gopsutil/v4 v4.25.9` and static sqlc PG queries. No privileged
host mounts, external collector, new config or database migration is required.
The seven indirect platform dependencies support portable native observations;
only used packages are linked. Dependency/image/collection costs are recorded
in the task completion evidence, not inferred as target-host load certification.

- CPU busy % uses deltas of two nonblocking aggregate CPU counter samples,
  excluding idle/I/O wait and avoiding double-counted guest time. The initial
  sample or counter reset is unavailable, not fake zero. Host-visible load
  averages cover 1/5/15 minutes. Local Podman observes its Linux VM, not Windows
  host CPU/RAM; these are not container CPU quota/utilization measurements.
- Host-visible total/used/available RAM and app RSS are distinct from Linux
  cgroup v2 memory.current and the minimum finite memory.max of visible ancestors.
  Mount/membership mapping respects the process namespace; hidden host ancestors
  cannot be certified. Unlimited is explicit; cgroup v1/non-Linux are unsupported.
  Windows load is unsupported to avoid a process-global gopsutil sampling worker.
- Current-database `pg_database_size`, total/active connections (including the
  collector), cluster `shared_buffers` converted to bytes; actual server version,
  latest applied Goose level, build-injected app version, Go runtime and
  app-process uptime. No workspace enumeration or implicit resource access.
- Filesystem capacity/used/free-to-app is sampled for **PT_DATA_DIR**, not a
  hard-coded path or the PG volume. In Compose this is `/data`; it describes the
  filesystem backing the app volume, not necessarily a dedicated partition.
  Reserved filesystem blocks may make total differ from used + free-to-app.
- Attachment/backup logical regular-file totals come from its `files`/`backups`
  children. Existing empty directories are zero; missing roots are unavailable,
  and collection never creates directories or changes permissions. Each scan
  rejects symlink roots, skips symlink entries, rejects special files and counts
  hardlinks once per entry. Rooted access and directory identity checks prevent
  symlink traversal outside the scanned root. Totals are observations, not an
  atomic filesystem snapshot.

One on-demand collector is shared; no idle polling. Snapshots cache five seconds;
directory successes **and errors** cache independently for one hour and carry
their original timestamps. Each directory scan uses batches of 128, at most
100,000 entries/depth 64 and a one-second cooperative deadline, within a
four-second collection context. No partial totals on permission, I/O, unsupported
entry, cancellation or bounds failure. **Local storage only:** OS filesystem
syscalls cannot be forcibly interrupted. A blocked syscall keeps the one worker
occupied rather than spawning unbounded replacement workers. Shutdown waits for
that worker; do not mount network/FUSE storage with unbounded I/O waits.

Each metric has ready/unavailable, nullable value and safe reason code. Source
failures are logged only by fixed section/reason, never raw errors/paths/DSNs.
An unavailable source does not make healthy sources disappear. The UI refreshes
every ten seconds while visible, stopping/canceling when hidden/unmounted;
failed HTTP refreshes keep a visibly stale snapshot with retry.

## 9. Security checklist

- No published ports: DB reachable only on the private compose network; app reachable only via Caddy's network.
- HTTPS terminated by Caddy; `PT_BASE_URL` must be `https://…` in production (cookies `Secure`).
- Firewall: only 22/80/443.
- Setup bootstrap token is process-local, valid 24 hours, and rotated/logged on
  restart while no user exists; only the dedicated `SETUP_TOKEN=…` entry is
  permitted to expose it. Setup is transaction-serialized and the endpoint
  returns 410 after completion.
- Login rate limits (per IP from trusted `X-Forwarded-For`, per account); session idle timeout 7 days, absolute 30 days; sessions revoked on password reset/disable.
- Upload MIME sniffing; only PNG, JPEG, GIF, and WebP are served inline and thumbnailed. All other types are served with `Content-Disposition: attachment`; responses set `X-Content-Type-Options: nosniff`; CSP on SPA.

### T1.2 auth operating policy

Production requires HTTPS in `PT_BASE_URL` even though Caddy's upstream socket
is HTTP. `pt_session` is host-only, Path=/, HttpOnly, Secure and SameSite=Lax.
Neither `X-Forwarded-Proto` nor the request socket changes these flags.
For the existing Vite and isolated Compose development paths only, literal
`http://localhost` / loopback-IP base URLs omit Secure and emit an explicit
startup warning (D-073). Non-loopback HTTP auth configuration fails startup.
Do not expose the local override as a public deployment.

All unsafe API requests need **both** the exact configured Origin and
`X-PT-CSRF: 1`, including login/logout. For Vite use its origin (normally
`http://127.0.0.1:5173`), not the backend port; curl/API tests must set the
Origin explicitly. A base URL path is not part of an Origin; default :80/:443
ports are omitted. No arbitrary forwarded-host/proto trust or CORS is added.
Prefer narrow actual proxy CIDRs; an explicitly empty trusted-proxy list
trusts none. Trusted X-Forwarded-For chains are evaluated right-to-left and
stop at the nearest untrusted hop, not at an attacker-supplied leftmost IP.

Passwords use 8-32 UTF-8 bytes and argon2id (64 MiB, one iteration, two lanes),
with one fail-fast KDF worker per app auth service. Busy work returns 503;
no unbounded CPU/memory queue is created. Normal password changes revoke all
sessions and require re-login. Future setup/admin hashing must reuse this
worker; future disable/reset must serialize on the user row and revoke sessions
in the same transaction. T1.2 does not implement those services.

Rate limits are bounded in-memory state for the single instance (D-045/D-077);
restart resets limits but not PG sessions. Do not use multiple app replicas.
Sessions store only token digests, slide with conditional UPDATE (never
recreate revoked rows), and check seven-day idle / 30-day absolute expiry
independently of the stored expiry. All app DB connections force UTC.
Profile timezone validation uses PG's catalog; NULL follows the current team
default. Real datetime/filter/export timezone behavior is later work.
