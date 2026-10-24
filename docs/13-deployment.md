# 13 — Deployment & Operations

Build happens on the developer laptop (Windows + Podman); the image is saved to a tar archive, copied to the server and loaded into Docker. See [14-dev-build-release.md](14-dev-build-release.md) for the build side. This doc covers the **server** side.

## 1. Target host

Tencent Cloud Lighthouse-class: 2 vCPU, 4 GB RAM, Ubuntu 24.04 LTS x86_64, Docker Engine + Compose plugin. An **existing, independent Caddy container** already reverse-proxies all apps on the host (D-046).

## 2. Containers

| Service | Image | Networks | Notes |
|---|---|---|---|
| `db` | `postgres:18` (Debian) | `default` (private) | volume `pgdata`; custom `postgresql.conf`; no published ports |
| `app` | `localhost/pandatable:${PT_VERSION}` (loaded from tar) | `default` + external `proxy` | Debian slim + `postgresql-client-18` (pg_dump/pg_restore); volume `/data`; no published ports |

Caddy is **not** part of this stack. The app joins Caddy's existing network so Caddy can reach it by alias `pandatable:8080`.

### deploy/compose.yaml (sketch)

Must work with both `docker compose` (server) and `podman compose` (laptop smoke tests) — D-047. Therefore: no Docker-only extensions, app retries DB connection on startup instead of relying solely on `depends_on: condition`.

```yaml
name: pandatable

services:
  db:
    image: docker.io/library/postgres:18
    restart: unless-stopped
    environment:
      POSTGRES_DB: pandatable
      POSTGRES_USER: pandatable
      POSTGRES_PASSWORD: ${PT_DB_PASSWORD:?set PT_DB_PASSWORD}
      TZ: UTC
    volumes:
      - pgdata:/var/lib/postgresql
      - ./postgresql.conf:/etc/postgresql/postgresql.conf:ro
    command: ["postgres", "-c", "config_file=/etc/postgresql/postgresql.conf"]
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U pandatable -d pandatable"]
      interval: 10s
      timeout: 5s
      retries: 10
    mem_limit: 1600m
    networks: [default]

  app:
    image: localhost/pandatable:${PT_VERSION:-latest}
    restart: unless-stopped
    depends_on: [db]
    environment:
      PT_DATABASE_URL: postgres://pandatable:${PT_DB_PASSWORD}@db:5432/pandatable?sslmode=disable
      PT_DATA_DIR: /data
      PT_LISTEN: ":8080"
      PT_BASE_URL: ${PT_BASE_URL:?set PT_BASE_URL}          # e.g. https://table.example.com
      PT_TRUSTED_PROXIES: ${PT_TRUSTED_PROXIES:-10.0.0.0/8,172.16.0.0/12,192.168.0.0/16}
      GOMEMLIMIT: 300MiB
      TZ: UTC
    volumes:
      - ptdata:/data
    mem_limit: 512m
    networks:
      default: {}
      proxy:
        aliases: [pandatable]

networks:
  default: {}
  proxy:
    external: true
    name: ${PT_PROXY_NETWORK:-caddy}      # name of the existing Caddy network on the server

volumes:
  pgdata: {}
  ptdata: {}
```

`deploy/compose.local.yaml` (override for laptop smoke tests, no Caddy available):
```yaml
services:
  app:
    ports: ["127.0.0.1:8080:8080"]
networks:
  proxy:
    external: false
    name: pandatable-proxy-local
```
Usage: `podman compose -f compose.yaml -f compose.local.yaml up -d`.

Notes:
- PG 18 official image keeps data under `/var/lib/postgresql/18/docker` (volume mounted at `/var/lib/postgresql`), easing future major upgrades.
- Image names are fully qualified (`docker.io/library/...`, `localhost/...`) so Podman and Docker resolve them identically (Podman does not assume Docker Hub; `podman save` → `docker load` keeps the `localhost/` prefix).

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

## 6. Upgrades

```
docker load -i images/pandatable-<newver>.tar.gz
sed -i 's/^PT_VERSION=.*/PT_VERSION=<newver>/' .env
docker compose up -d app
```
Migrations run automatically on start (goose, in tx, advisory lock). Create a backup in Admin before upgrading. Rollback = set previous `PT_VERSION` (only if no migrations ran; otherwise restore backup).

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
