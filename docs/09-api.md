# 09 — API

Internal REST API used only by the SPA. Base path `/api/v1`. JSON in/out. Auth by session cookie.

## 1. Conventions

- **CSRF:** all non-GET requests must send header `X-PT-CSRF: 1` *and* pass Origin check against `PT_BASE_URL` (SameSite=Lax cookie as additional layer).
- **IDs:** structure objects use UUID strings; rows use bigint `id` serialized
  as a canonical decimal string, never a potentially rounded JS number (D-115).
- **Errors:**
  ```json
  { "error": { "code": "ERR_PERMISSION_DENIED", "message": "dev-readable text", "details": { } } }
  ```
  The frontend translates `code` via i18n (`errors.ERR_PERMISSION_DENIED`). HTTP status: 400 validation, 401 unauthenticated, 403 forbidden, 404 not found (also used when resource hidden by ACL), 409 conflict (version, last owner, unique), 422 conversion failure, 429 rate limit, 500.
- **Pagination:** `?offset=&limit=` (limit ≤ 500) → `{ "items": [...], "total": n }`.
- **Optimistic concurrency:** views/fields carry `version`; send `If-Match: <version>` on update.
- Every mutating response includes `"eventId"` so the originating client can ignore its own realtime echo.

### T0.4 operational routes

- `GET /healthz` returns HTTP 200, `Content-Type: application/json`, and
  `{"status":"ok"}` plus a newline. HEAD is also supported. This is process
  liveness, not a per-request database readiness check.
- `/api/v1` includes the T1.2 auth/profile/timezone endpoints below. Unknown paths in
  `/api` return JSON 404 `ERR_NOT_FOUND`, never the SPA. Unknown `/healthz/…`
  paths also return 404; unsupported methods on `/healthz` return JSON 405
  `ERR_METHOD_NOT_ALLOWED`.
- Responses include a generated `X-Request-ID`; an incoming value is not reused.
  Unexpected handler panics return JSON 500 `ERR_INTERNAL` if the response has
  not started, otherwise the connection/stream is aborted.
- UI requests return JSON 503 `ERR_SPA_UNAVAILABLE` until built SPA assets exist.
  This is a pre-T0.5 build state, not an application API success response.

T1.2 implements the auth/profile/timezone backend below. T1.3 implements
first-run setup; T1.4 adds the ACL resolver and grants API; T1.5 adds admin
users and login/forced-password-change UI; T1.6 adds groups; T1.7 adds system
observations/dashboard; T1.8 adds settings, public branding/default language,
currency catalog and read-only workspace inventory; T1.10 adds the
workspace/book/table structure API below. Other business endpoints
and profile UI remain later-task work.

T1.9 adds **no HTTP route**. Future structure/field services can propagate its
sanitized typed errors through the existing HTTP error boundary:
`ERR_VALIDATION` (400, invalid descriptors/default representations),
`ERR_DDL_CONFLICT` (409, existing data/constraint/name conflicts) and
`ERR_DDL_LOCK_TIMEOUT` (409, the three-second schema lock wait expired).
No PG message/detail/SQL/default value is included. A name collision additionally
supports `errors.Is(err, ddl.ErrIdentifierCollision)` for bounded caller-owned
whole-transaction retry; do not treat every metadata uniqueness error as a
physical collision. Unexpected driver/cancellation/missing-object/dependency
errors remain wrapped for standard correlated logging and redacted 500 handling,
not success/no-op fallbacks. API mapping is preparatory, not a new test endpoint.

T1.13 also adds **no HTTP route**. Future field/row services bind
`codec.Bind(uiType, options)` and use typed Column/DefaultValue descriptors,
Decode for request values and Encode for scanned pgx values.
`codec.APIError` maps validation failures to 400 `ERR_VALIDATION` with
`details.reason` and optional `details.option` (never the rejected value).
Unsupported scanned types remain server faults for correlated redaction.
Translate reasons through static frontend keys, never server messages.
Bigint/decimal are strings; date/datetime/select/null contracts are specified
in [field types](07-field-types.md#batch-1-normalization-and-wire-contract).
No row/field DTO or endpoint is invented to stand in for acceptance.

## 2. Endpoints

### Setup & auth
| Method | Path | Notes |
|---|---|---|
| GET | `/setup/status` | `{ needed: bool }` |
| POST | `/setup` | `{ token, username, email, displayName, password, locale }` → first admin + session; 410 once done |
| POST | `/auth/login` | `{ login, password }` (username or email); rate-limited 10/min/IP, 5 failures → 1 min lock per account |
| POST | `/auth/logout` | |
| GET | `/auth/me` | user, role, prefs, mustChangePassword |
| POST | `/auth/password` | `{ current, new }` |
| PATCH | `/auth/me` | `{ displayName, locale, theme, timezone }` (timezone: IANA name or `null` = app default) |

#### T1.2 auth contract (D-072..D-077)

- Passwords are **8-32 UTF-8 bytes**, without trimming, normalization or composition
  rules. Login trims surrounding whitespace from the username/email only, and
  matches username/email case-insensitively. Multiple matching legacy identities
  fail closed. T1.3 setup normalizes its first username/email; future admin
  creation must preserve these normalization and ambiguous-identity safeguards.
- Login returns `{ user, eventId }`; `GET /auth/me` returns the user directly;
  profile PATCH returns `{ user, eventId }`. Logout/password success returns
  `{ eventId }`. Password change requires the current password and a different
  new password, clears `mustChangePassword`, revokes **all** sessions including
  the current one, and expires the cookie. Re-login is required.
- User DTO: `id`, `username`, nullable `email`, `displayName`, `role`,
  `mustChangePassword`, `locale`, `theme`, nullable `timezone`,
  `effectiveTimezone`, `avatarColor`. Password/hash/token/session metadata
  never enter DTOs. The raw session token appears only in `Set-Cookie`.
- Cookie `pt_session`: host-only, Path=/, HttpOnly, SameSite=Lax, Secure for
  HTTPS. Persistent cookie expiry follows the server's sliding expiry:
  seven days idle, capped at 30 days from creation. Successful identity/profile/
  timezone reads refresh it; server-side validation always checks both limits.
  Logout of a canonical already-revoked token is idempotent.
- Literal localhost/loopback HTTP is an explicit development exception: cookie
  omits Secure and startup logs a warning. Other HTTP auth configuration is
  rejected. Never use this exception for production (see deployment section 9).
- Unsafe API methods, **including login/logout**, require both `X-PT-CSRF: 1`
  and one exact Origin matching the configured base URL's canonical scheme/
  host/port (no path/trailing slash; default ports omitted). GET/HEAD/OPTIONS
  are safe methods. There is no CORS or forwarded-host/proto trust.
- Auth responses use `Cache-Control: no-store`. JSON requests require
  `Content-Type: application/json`, a single JSON value, known fields only,
  and at most 4096 body bytes. Handler contexts have a ten-second deadline.
- PATCH fields are optional; omitted fields are preserved. Only timezone
  accepts JSON null. At least one field is required. Display name must be
  nonblank, at most 100 Unicode code points, without control characters.
  Locale is `en|zh-CN`, theme `light|dark|system`. Timezone must exactly match
  the PG18 `pg_timezone_names` catalog (excluding `posix/` and `right/`).
  NULL follows `meta.settings.default_timezone`, default `Asia/Shanghai` if
  unset. Invalid stored defaults fail explicitly; app connections use UTC.
- `GET /meta/timezones` returns `{ items: string[] }` from that same catalog,
  available to active authenticated users. A forced-change user can only
  read `/auth/me`, change password or log out; profile/timezone-list access
  returns 403 until the password changes. Future protected routes must enforce
  this flag too. Disabled users are denied even if a session row remains;
  future admin disable/reset must lock the user and revoke sessions atomically.
- Ten login requests/minute/IP after CSRF/client-IP validation, including malformed
  JSON/unsupported content types; five consecutive failures for a resolved
  user ID cause a one-minute lock, rejecting even a correct password and shared
  across username/email/case aliases. Bad password lengths count as failures.
  Success/completed lock resets failures. State is single-instance in memory,
  capped at 4096 IP/account keys; saturation fails closed, not by evicting live
  limits. Restart resets rate state, not sessions. Same-account parallel login
  attempts return 429; the single occupied password worker returns 503.
- Client IP starts with the socket peer. Only configured trusted proxy peers
  may supply X-Forwarded-For; walk right-to-left until the first untrusted hop.
  Empty trust means ignore forwarding. Trusted chains are limited to 32 IPs /
  4096 bytes and malformed chains fail 400. X-Real-IP and forwarded host/proto
  are not used. Cookie/Origin policy depends only on PT_BASE_URL.
- Typed auth errors: 400 `ERR_VALIDATION`, 401 `ERR_UNAUTHENTICATED` or
  `ERR_INVALID_CREDENTIALS`, 403 `ERR_CSRF` or `ERR_PASSWORD_CHANGE_REQUIRED`,
  415 `ERR_CONTENT_TYPE`, 429 `ERR_RATE_LIMITED` (`Retry-After: 60`),
  503 `ERR_AUTH_BUSY` (`Retry-After: 1`), 500 `ERR_INTERNAL`. Internal errors
  log only a fixed message and request ID, never driver/hash/credential content.
  The setup UI translates setup errors; future auth consumers must do the same.

#### T1.3 setup contract (D-078)

- `GET /setup/status` is public and returns `{ needed: bool }` with no-store.
  When needed, SPA navigation to any route except `/setup` redirects to
  `/setup`. T1.5 adds a sign-in form to the existing `/login` welcome surface.
- When no users exist and `setup_completed` is absent from `meta.settings`,
  each process boot creates a 32-byte random base64url token in memory, valid
  for 24 hours, and emits the dedicated `SETUP_TOKEN=…` log message. The token
  is never stored or included in access/request logs. Restarting while setup is
  incomplete invalidates the prior token and logs a replacement.
- POST requires exact Origin plus `X-PT-CSRF: 1`, bounded JSON and trusted-proxy
  client IP policy. Username is trimmed/lowercased and must match
  `[a-z0-9._-]{3,32}`. Email is required, trimmed/lowercased and validated as
  one address for future SMTP/login use. Password remains 8-32 UTF-8 bytes
  without normalization.
- `ERR_SETUP_TOKEN` is 401 for invalid/expired tokens; `ERR_SETUP_COMPLETE` is
  410 when a user exists or completion is recorded. Setup serializes across
  processes with a PostgreSQL transaction advisory lock, rechecks zero users
  and completion state in the transaction, then atomically creates the admin,
  session and `setup_completed` setting. Rollback leaves no partial rows and
  does not consume the process token.
- Success returns `{ user, eventId }` and sets the existing secure
  server-side `pt_session` cookie. Raw session tokens appear only in `Set-Cookie`.
  The localized UI remains on a completion state showing the authenticated
  identity; T1.3 does not add or claim a login form.

### Admin (role=admin)
| Method | Path | Notes |
|---|---|---|
| GET/POST | `/admin/users` | list (search, status) / create `{username, email?, displayName, role, password}` |
| GET | `/admin/users/{id}` | sanitized admin user detail |
| GET | `/admin/users/{id}/groups` | paged/searchable group memberships |
| PATCH | `/admin/users/{id}` | displayName, email, role, status |
| POST | `/admin/users/{id}/reset-password` | `{ password }` → sets must_change_password |
| GET | `/admin/users/{id}/ownerships` | workspaces/books/tables where user is direct owner + sole-owner flag |
| POST | `/admin/users/{id}/transfer` | `{ toUserId }` transfers all direct owner grants |
| DELETE | `/admin/users/{id}` | hard delete; `?confirm=<username>`; deletes sole-owned workspaces (to trash bypassed → purge) |
| GET/POST | `/admin/groups` | list/search / create `{name, description?}` |
| GET/PATCH/DELETE | `/admin/groups/{id}` | detail / edit name or description / permanent deletion |
| GET/POST | `/admin/groups/{id}/members` | paged/searchable members / add `{userId}` |
| DELETE | `/admin/groups/{id}/members/{userId}` | remove membership, absent membership is a no-op |
| GET | `/admin/workspaces` | name, owners, size, orphaned |
| POST | `/admin/workspaces/{id}/owner` | `{ userId }` (orphan rescue) |
| GET/PATCH | `/admin/settings` | app name, default timezone, default currency, default locale, upload limit, retention days |
| GET | `/meta/app` | public `{appName,defaultLocale}` only; no-store |
| GET | `/meta/timezones` | IANA timezone list (from `pg_timezone_names`) — any user |
| GET | `/meta/currencies` | ISO 4217 codes — any user |
| GET | `/admin/system` | CPU %, load, mem used/total, disk used/free, db size, files size, backups size, app RSS, uptime, versions |
| GET/POST | `/admin/backups` | list / create (async job) |
| GET | `/admin/backups/{id}/download` | streamed zip |
| DELETE | `/admin/backups/{id}` | |

#### T1.5 users contract (D-081..D-083)

T1.5 implements users list/create/PATCH/reset; T1.6 adds groups and user detail
as specified below. T1.8 adds settings/workspace inventory; ownership transfer,
user deletion and backups remain later tasks. The entire `/admin/*` API namespace requires an active admin:
401 unauthenticated/disabled, 403 `ERR_PASSWORD_CHANGE_REQUIRED` for a forced
user, otherwise 403 `ERR_PERMISSION_DENIED` for a non-admin. Admin status does
not grant resource access. All routes reuse auth's exact CSRF/Origin, no-store,
4096-byte strict JSON, typed errors and ten-second deadline.

- List: `q` (trimmed, case-insensitive **literal substring** of username,
  email or displayName; at most 254 UTF-8 bytes, no control characters),
  `status` (empty/all, `active`, `disabled`), `offset` (default 0, nonnegative
  int32), `limit` (default 50, 1..500). Duplicate/unknown parameters fail 400.
  Returns `{items,total}` from one repeatable-read snapshot, ordered by username
  then id. Empty/out-of-range pages have `items: []` and the filtered total.
- Admin user DTO: `id, username, email, displayName, role, status,
  mustChangePassword, lastLoginAt, createdAt, updatedAt`; email and lastLoginAt
  are nullable. No password/hash/session metadata is returned.
- Create: username is immutable, trimmed/lowercased `[a-z0-9._-]{3,32}`;
  email is optional/null/blank or normalized/validated exactly as setup.
  Display name follows the existing 100-code-point/nonblank/no-controls rule;
  role is required (`admin|user`). Admin supplies the password (8..32 UTF-8
  bytes, unmodified); new users are active with `mustChangePassword: true`.
  Returns 201 `{user,eventId}` without creating a session for the new user.
- PATCH preserves omitted fields; only email accepts null (null/blank clears
  it); username/password are not patchable. At least one field is required.
  Returns `{user,sessionsRevoked,eventId}`. Actual role or status changes revoke
  **all** sessions in the same user-row-locked transaction, including on enable.
  An unchanged role/status does not revoke sessions.
- Identity checks reject case-insensitive duplicates and cross-column
  username/email ambiguity, including legacy rows: 409 `ERR_IDENTITY_CONFLICT`.
  Missing/malformed target id is 404 `ERR_NOT_FOUND`.
- Demoting/disabling the last active admin is 409 `ERR_LAST_ADMIN`; disabled
  admins do not count. Self-demotion/disable is allowed if another active admin
  remains. Self-reset is allowed. Reset returns `{eventId}`, sets forced change
  and atomically revokes all sessions; disabled users remain disabled.
  Self-revoking success also expires the caller's cookie.
- Mutations share a PostgreSQL transaction advisory lock
  `pandatable.admin.users`, then lock/recheck the actor and lock the target user
  before updating. Caller session/status/role/forced-change is rechecked after
  the actor lock. Last-admin and identity checks are serialized across processes.
  Password hashing reuses the existing auth worker, with busy=503.
  Role/status writes invalidate ACL after commit, even if commit is uncertain.
  Disabling a last workspace owner is allowed and retains all data/authorship;
  orphan rescue/transfer remains deferred.

#### T1.6 groups contract (D-084..D-087)

All paths use the same active-admin namespace guard, exact Origin + CSRF,
no-store, strict 4096-byte JSON and ten-second deadline as users. Groups are flat;
administration never grants implicit resource access.

- Group lists, group members and user memberships return `{items,total}` from
  one repeatable-read snapshot. `offset` defaults 0, nonnegative int32; `limit`
  defaults 50, 1..500. `q` uses the same trimmed, case-insensitive literal-substring
  and 254-byte/control validation as users. Groups search name/description and
  order by name/id; members search username/email/displayName and order by
  username/id. Unknown/duplicate/malformed parameters fail 400. Empty pages have
  `items: []` and the filtered total; missing target user/group is 404.
- Group DTO: `id,name,description,createdAt,memberCount` (counts include disabled
  members). GET detail returns `{group}`; user GET detail returns `{user}` using
  the existing sanitized user DTO. No sessions/hashes/passwords are exposed.
- POST requires name; omitted description defaults to `""`. Name is trimmed,
  case-preserving, 1..100 Unicode characters, no controls, with the existing
  **case-sensitive** uniqueness. Description is at most 1000 Unicode characters;
  newline, carriage return and tab are allowed, other controls are rejected.
  No Unicode normalization or case-folded name uniqueness is introduced.
  Create returns 201 `{group,eventId}`; duplicate name is 409
  `ERR_GROUP_NAME_CONFLICT`. PATCH preserves omitted fields, rejects null/empty
  patches, and returns 200 `{group,eventId}`.
- POST members requires an existing active `userId`; missing/malformed ID is
  404 `ERR_NOT_FOUND`, disabled is 409 `ERR_PRINCIPAL_DISABLED`, duplicate is
  409 `ERR_MEMBERSHIP_EXISTS`. Success is 201 `{eventId}`. DELETE requires an
  existing group/user and returns 200 `{eventId}` even if the membership is absent.
  Existing disabled memberships remain visible and removable; disable does not
  purge memberships. There is no bulk replacement or nested-group endpoint.
- Group DELETE returns 200 `{eventId}` after deleting the principal row first,
  cascading memberships and deleting every polymorphic group grant in the same
  transaction. UI confirmation is required; no resource/data deletion or
  last-direct-user-owner exception is added. Missing group is 404.
- All group mutations share `pandatable.admin.users`, then lock/recheck the
  acting admin/session with actor `FOR NO KEY UPDATE`. Membership locks a distinct
  target user `FOR UPDATE`, checks status under that lock, then locks its group
  `FOR KEY SHARE`. Edit uses group `FOR UPDATE`; deletion takes the DELETE lock
  before grant cleanup. This composes with user status/auth and D-080 principal
  locks without blocking grant `granted_by` FK key-share on the actor. The original
  user-admin actor lock remains unchanged. Membership writes/deletion invalidate
  ACL after commit, also if commit reports an uncertain outcome/error; failed
  or rolled-back operations may conservatively invalidate without changing data.

#### T1.8 settings and workspace inventory (D-091..D-093)

All admin routes independently enforce the active-admin/session/forced-change
contract above, including after taking mutation locks. Existing no-store, exact
Origin + CSRF, 4096-byte strict JSON and ten-second deadlines remain unchanged.
No settings/workspace endpoint grants access to resource data.

`GET /admin/settings` accepts no query parameters and returns:

```json
{
  "appName": "PandaTable",
  "defaultTimezone": "Asia/Shanghai",
  "defaultCurrency": "CNY",
  "defaultLocale": "en",
  "uploadMaxMb": 20,
  "historyRetentionDays": 90,
  "trashRetentionDays": 30
}
```

`PATCH /admin/settings` accepts any nonempty subset of these seven fields;
unknown fields, nulls, wrong types and query parameters fail validation.
`appName` is trimmed, nonblank, at most 100 Unicode code points, without
controls. Timezone exactly matches PG's catalog excluding `posix/`/`right/`;
currency exactly matches the checked-in current ISO 4217 list
([SIX list one](https://www.six-group.com/dam/download/financial-information/data-center/iso-currrency/lists/list-one.xml),
2026-09-17, including fund/special codes); locale is `en|zh-CN`. Numeric fields
are integers: upload 1..100 **MiB** (1,048,576 bytes each), retention 1..3650
days. Missing rows use the documented defaults; corrupt stored values fail,
not silently default. `setup_completed` is never exposed or editable.

The complete candidate is validated before writing supplied keys in a single
transaction. Writes share `pandatable.admin.users`, lock the acting admin
`FOR NO KEY UPDATE`, then recheck its session. Disjoint concurrent patches
compose; same-field writes serialize, last committed write wins (no settings
version/If-Match). Response: `{settings,eventId}`. No settings cache, restart,
filesystem traversal or ACL invalidation is needed: no ACL state changes.

Existing NULL-profile-timezone auth readers observe the new team default on
their next read; explicit timezones remain unchanged. New admin-created users
receive the current default locale, but existing users' locale/theme/timezone
are not rewritten. Setup retains its explicit chosen locale. Upload enforcement,
currency-field creation and history/trash purge consumers do not exist yet:
these values persist for T2.2/T2.1/T3.4/T3.6, not completed AT-322.

`GET /meta/app` is public and returns **only** `{appName,defaultLocale}`. It
does not read privileged settings. No query parameters; no-store, same bounded
request context and sanitized errors. `GET /meta/currencies` returns
`{items:string[]}` from the validation catalog to active, password-cleared
authenticated users, including non-admins; it accepts no query parameters.
There is no runtime network catalog fetch or new seed/auth endpoint.

`GET /admin/workspaces`: `q`, `offset`, `limit` follow group-list bounds
(trimmed case-insensitive literal name substring, <=254 UTF-8 bytes/no controls,
offset nonnegative int32, limit default 50/1..500). Duplicate, malformed and
unknown query parameters fail. One repeatable-read snapshot returns
`{items,total}`, ordered by name then UUID; empty/out-of-range pages use `[]`.
Only live workspaces are listed. Each item is:

```json
{
  "id": "workspace-uuid",
  "name": "Team",
  "owners": [{
    "id": "principal-uuid",
    "type": "user",
    "name": "Alice",
    "username": "alice",
    "status": "active"
  }],
  "ownerTotal": 1,
  "orphaned": false,
  "attachmentBytes": 0
}
```

Owners are direct workspace owner grants to existing users/groups, ordered by
type, name, UUID; first 50 per workspace, full `ownerTotal` for truncation.
Disabled users remain visible with status. Groups have null username/status.
Dangling polymorphic principals are omitted. `orphaned` means **no active direct
user owner**; a group owner/member or disabled owner never prevents it.
`attachmentBytes` sums logical `meta.attachments.size_bytes` across retained
descendants (including trashed books/tables/fields), not PostgreSQL relation
size, thumbnails/backups or filesystem usage. No path, row data, email,
physical identifier, ownership mutation or sidebar entry is exposed/created.
This is not the future workspace/table CRUD API.

#### T1.7 system observations (D-088..D-090)

`GET /admin/system` is implemented. It accepts no query parameters and is
read-only. The existing active-admin namespace guard, independent service
session check, no-store and ten-second request deadline apply on every request,
including cache hits. No resource access, grant/membership changes or ACL
invalidation is performed. A stopped/unconfigured collector returns 503
`ERR_SYSTEM_UNAVAILABLE`; authorization failures retain the existing 401/403 codes.

The response is a `SystemSnapshot`, mirrored in the frontend API types:

- `collectedAt` (UTC RFC3339), `uptimeSeconds` (app-process lifetime);
- `cpuPercent`, `load: {one,five,fifteen}`, host-visible
  `memory: {totalBytes,usedBytes,availableBytes}`, `appRssBytes`;
- `container: {currentBytes,limitBytes,unlimited}` for Linux cgroup v2 memory;
- `disk: {totalBytes,usedBytes,freeBytes}` for the configured data filesystem;
- `database: {sizeBytes,connections,activeConnections,sharedBuffersBytes}` for
  the current database (connections include the collector; shared buffers is
  cluster configuration);
- `filesBytes`, `backupsBytes` (logical regular-file bytes, not allocated blocks);
- `versions: {app,go,postgres,migration}`. App/Go are strings; PG/migration use
  metric envelopes from actual server queries, not hard-coded versions.

Every metric above except app/Go/uptime uses
`{status:"ready"|"unavailable", collectedAt, value, reason?}`. Unavailable
`value` is **null**, not zero. Reason codes are `initializing`, `unsupported`,
`notFound`, `permission`, `io`, `limit`, `timeout`, `invalid`, `query`; never
raw OS/PG error text. Genuine zero is valid, including empty directories and
idle CPU. Initial/reset CPU samples are unavailable until a second sample.
An unlimited cgroup has a ready value with `unlimited:true,limitBytes:null`.
The finite limit is the minimum of visible ancestors, not a claim about hidden
host ancestors. Cgroup v1/non-Linux are unsupported; native Windows load is
unsupported rather than starting gopsutil's global load sampler.

Partial source failures are explicit in a 200 snapshot, with safe section/reason
logging; a failed total is never a partial number. No DSN, hostname, storage path,
entry name, environment or credential appears in the response or metric logs.
One on-demand collection is shared and cached five seconds, with a four-second
application-lifetime deadline. Canceling a waiter does not cancel other waiters.
Directory results, including failures and their scan timestamps, cache separately
for one hour. See deployment monitoring for scan safety and local-storage limits.

### Sharing directory (T1.12, D-109)

| Method | Path | Response |
|---|---|---|
| GET | `/directory/users?resource=table:<uuid>&q=alice` | `{items:[{id,name,username}],hasMore}`; active users only |
| GET | `/directory/groups?resource=book:<uuid>&q=team` | `{items:[{id,name,username:null}],hasMore}` |

Both routes require active, password-change-cleared authentication and effective
**owner of the supplied scope**, not admin role. `resource` follows the grants
reference grammar (workspace/book/table); missing, hidden or trashed paths are
404, visible non-owner/path-only is 403. Authorization and search use one
read-only repeatable-read snapshot with uncached ownership. Grant mutations still
recheck principal existence/status and ownership independently.

Exactly one `resource` and one `q`, no other query parameters. Search is trimmed,
2..100 Unicode code points, <=254 UTF-8 bytes, valid UTF-8 without control
characters. Malformed/duplicate/unknown parameters and invalid references are
400 `ERR_VALIDATION`. Match a literal case-insensitive substring of username or
display name (users), name (groups); `%`, `_` and quotes are ordinary characters.
Return at most 20 matches, users ordered by username/id, groups by name/id.
`hasMore` means refine the query; there is no total, paging or empty browse.
Only `id`, `name` and nullable `username` are exposed: no email, role, status,
membership or admin enumeration. This intentionally scoped minimal identity
discovery replaces the earlier unimplemented general-directory sketch.
Responses are no-store with the normal ten-second deadline/redacted errors.

### Structure
| Method | Path | Notes |
|---|---|---|
| GET | `/nav` | full sidebar tree the user may see (workspaces → books → tables → views) with effective levels |
| POST | `/workspaces` | `{ name, icon?, color? }` → 201 `{ workspace, eventId }`; creator becomes direct owner |
| PATCH/DELETE | `/workspaces/{id}` | PATCH `{ name?, icon?, color? }` (null clears icon/color); delete → trash |
| POST | `/workspaces/{id}/books` | `{ name }` → 201 `{ book, eventId }` |
| PATCH/DELETE | `/books/{id}` | PATCH `{ name?, index? }`; delete → trash |
| POST | `/books/{id}/tables` | `{ name }` → 201 table schema + `eventId` |
| PATCH/DELETE | `/tables/{id}` | PATCH `{ name?, index?, displayFieldId? }` (display field needs table owner, T1.14); delete → trash |
| GET | `/tables/{id}/schema` | table, fields, relations, views, schemaVersion, myLevel |
| POST | `/tables/{id}/fields` | create field (T1.14 batch-1 kinds; link → relation + reverse field arrives with T2.3) |
| PATCH/DELETE | `/fields/{id}` | rename/description/options/constraints/default/reorder; delete → soft delete; `If-Match` required |
| POST | `/fields/{id}/convert:dry-run` | `{ uiType, pgType?, options }` |
| POST | `/fields/{id}/convert` | `{ …, onFail: "abort"\|"null" }` |
| POST | `/formula:check` | `{ tableId, expr }` → type / error position |
| GET | `/trash?scope=workspace:{id}` | |
| POST | `/trash/{type}/{id}/restore` | |
| DELETE | `/trash/{type}/{id}` | purge now |

#### T1.10 structure contract (D-097..D-103)

- Routes exist only when the server has the structure service (always in
  `serve`) and use the T1.2 protections: session + disabled-user 401,
  `must_change_password` 403, no-store, ten-second deadline, exact Origin and
  `X-PT-CSRF: 1` on unsafe methods, JSON-only bodies of at most 4096 bytes with
  unknown/duplicate members rejected, and no query string on `/nav` or
  `/tables/{id}/schema`. A malformed UUID path is 404 `ERR_NOT_FOUND`.
- **Who may do what:** any active user may `POST /workspaces`; the creator gets a
  direct `owner` grant atomically. Workspace PATCH/DELETE, book creation and book
  PATCH/DELETE need effective owner of the **workspace**; table creation and
  table PATCH/DELETE need effective owner of the **book** (workspace owners
  inherit it). The admin role gives
  nothing. A hidden/missing/trashed target is 404 (checked first), a visible or
  path-only target without the required parent ownership is 403.
- **Inputs:** `name` is trimmed, 1..100 Unicode code points, no control
  characters; `icon` (nullable) ≤16 code points; `color` (nullable) `#RRGGBB`.
  `index` is a zero-based integer among the *other live siblings* under the same
  parent (D-101); out of range is 400 and an unchanged index is a 200 no-op.
  PATCH requires at least one known member. Display names are not unique.
- **Responses** carry an opaque `eventId` (D-103). `POST /books/{id}/tables`
  returns the full table schema: `table`, `fields` (Name + five system fields,
  no physical names), `relations: []`, `views` (default Grid), `schemaVersion`
  1 and `myLevel`. `GET /tables/{id}/schema` (read level) returns the same shape
  for the caller (shared views plus their personal views).
- **Soft delete:** DELETE sets `deleted_at/by` on the target only; descendants,
  physical objects, grants, fields and views are retained for the Trash service
  (M3) while the ACL hides the target and everything below it immediately
  (the resolver is invalidated before the response). A second delete is 404.
- **Errors:** validation 400; unauthenticated/disabled actor 401; hidden 404;
  insufficient 403; physical-name retries exhausted or DDL conflict 409
  `ERR_DDL_CONFLICT`; a lock wait over three seconds 409 `ERR_DDL_LOCK_TIMEOUT`.
  Unexpected database/driver/cancellation errors are redacted 500s with the
  standard correlated log.
- **`GET /nav`** → `{ workspaces: [{ id, name, icon, color, createdAt, level,
  pathOnly, books: [{ id, name, position, level, pathOnly, tables: [{ id, name,
  position, schemaVersion, level, views: [{ id, name, type, scope }] }] }] }] }`.
  `level` is `none|read|write|owner` (max grant on the path, flat groups,
  active users); `none`/`pathOnly` workspaces and books hold only visible
  descendants. Order: workspaces by creation, books/tables by position then id.
  Trashed resources never appear and the admin role is not special.

T1.11 consumes these routes without adding or changing HTTP contracts. Its
memory-only sidebar refetches `/nav` after mutations and resource 403/404, on
focus/visibility return and at a visible-tab 30-second interval (D-104).
`eventId` is not a realtime subscription. The schema-only table route reads
`GET /tables/{id}/schema`; no row/query/field/view mutation route is implied.
Both locales translate `ERR_NOT_FOUND` without naming a resource type or
distinguishing hidden from removed resources.

#### T1.14 field contract (D-123..D-130)

- `POST /tables/{id}/fields` `{name, uiType, description?, options?,
  isRequired?, isUnique?, default?, fillEmpty?}` → 201; `PATCH /fields/{id}`
  `{name?, description?, options?, isRequired?, isUnique?, default?, index?,
  fillEmpty?}` → 200; `DELETE /fields/{id}` → 200. PATCH/DELETE require
  `If-Match: <field version>`: missing/malformed is 400 `ERR_VALIDATION`, stale
  409 `ERR_VERSION_CONFLICT`. Responses are `{field?, schema, eventId}` with the
  full authoritative schema (no physical names). `PATCH /tables/{id}` accepts
  `displayFieldId` (live, non-system, non-checkbox field of that table).
- Every field and display-field mutation requires **effective owner of the
  target table**, rechecked uncached under the D-080 locks plus
  `meta.tables … FOR NO KEY UPDATE`. The admin role has no bypass. A
  hidden/missing/trashed table or field is 404; visible non-owner is 403.
- Field create/patch bodies use a separate strict **512 KiB** limit (D-125); all
  other routes keep 4096 bytes. Origin/CSRF/session/deadline checks still apply.
- `uiType` is a batch-1 writable kind; options/defaults are validated by the
  bound codec (D-113..D-118) and `fixedTz` additionally by `pg_timezone_names`.
  Names: trimmed 1..100 code points, unique among live fields under `lower()`
  → 409 `ERR_FIELD_NAME_CONFLICT`. Descriptions ≤1000 code points.
  `index` counts live non-system fields only.
- Errors: 400 `ERR_VALIDATION` with `details.reason` (`system_field`,
  `constraint_unsupported`, `storage_change`, `display_ineligible`, codec reasons)
  and optional `details.option`; 409 `ERR_REQUIRED_HAS_EMPTY` /
  `ERR_UNIQUE_HAS_DUPLICATES` with `details.count`; 409
  `ERR_DISPLAY_FIELD_REQUIRED`; DDL conflicts/lock timeouts as T1.10. Resending
  with `fillEmpty:true` and a default fills NULL cells in the same transaction.
- Every successful mutation bumps the field `version` (if a field changed) and
  `schemaVersion` once, with DDL, metadata and data rewrites in one transaction.
  Field delete is soft (column/data kept, NOT NULL and physical default dropped,
  display field reassigned). Restore/purge are M3; conversion is T2.12.

### Permissions
| Method | Path | Notes |
|---|---|---|
| GET | `/grants?resource=workspace:{id}` | direct grants on resource + inherited summary |
| PUT | `/grants` | `{ resourceType, resourceId, principalType, principalId, level }` (upsert) |
| DELETE | `/grants/{id}` | |

#### T1.4 grants contract (D-079, D-080)

- All three routes use the T1.2 protections: session + disabled-user denial,
  `must_change_password` → 403, no-store, ten-second deadline, exact Origin and
  `X-PT-CSRF: 1` on PUT/DELETE, JSON-only bodies of at most 4096 bytes with known
  fields. They exist only when the server is configured with the ACL service
  (always in `serve`).
- `resource` is `workspace:<uuid>`, `book:<uuid>` or `table:<uuid>` (canonical
  UUID, exactly one value). Malformed references are 400 `ERR_VALIDATION`.
- **Authorization:** list, PUT and DELETE all require the caller to be an
  *effective owner* of the grant's resource (direct or inherited), which is the
  D-014 scope-owner rule: an owner of S manages S and descendants, including
  `owner` grants, and nothing outside S. Admin role gives nothing. Mutations
  re-evaluate ownership uncached inside the locked transaction.
- **404 vs 403:** a resource with no effective grant for the caller, no grant on
  any live descendant (path-only visibility) and not visible through a grant is
  hidden: 404 `ERR_NOT_FOUND`, also for missing/trashed targets and ancestors.
  A resource that is visible (effective level below the requirement, or
  path-only) is 403 `ERR_PERMISSION_DENIED`. Example (AT-209): a book owner
  gets 403 on the parent workspace, 404 on a sibling book.
- `GET` returns `{ resource, effectiveLevel, items, inherited }`. `items` are
  direct grants on the resource; `inherited` are grants on its workspace/book
  ancestors (workspace first). A grant is
  `{ id, resourceType, resourceId, principalType, principalId, level, grantedBy,
  createdAt, principal }`, where `principal` is `{ name, username, status }` for
  users (display name), `{ name, username: null, status: null }` for groups, or
  `null` for a dangling principal. `items[]` also carry `inheritedLevel`
  (nullable): the highest level the *same principal* inherits from ancestors, so
  clients can warn that a lower direct level has no effect.
- `PUT` validates level (`read|write|owner`), types, UUIDs and the principal:
  missing user/group 404 `ERR_PRINCIPAL_NOT_FOUND`, disabled user 409
  `ERR_PRINCIPAL_DISABLED`. Returns `{ grant, created, eventId }`: 201 when
  created, 200 when the level was replaced; an identical level is an unchanged
  200 no-op. Lowering the *direct* level below an inherited one is allowed and
  has no effect (D-013).
- `DELETE` returns `{ eventId }`; unknown ids are 404, malformed ids 400.
- **Last owner:** removing or downgrading the direct `owner` grant of an
  *active user* on a workspace when no other active direct user owner exists is
  409 `ERR_LAST_OWNER`. Group ownership and disabled owners do not count; owner
  grants on books/tables are unprotected.
- Concurrent mutations in one workspace serialize on its row lock; mutations
  also hold key-share locks on book/table/user/group targets so they serialize
  with hard deletion. After any committed change the resolver cache is
  invalidated before the response is sent.
- Server-side route guard: `authHTTP.require(level, type, urlParam)` returns
  404/403 as above and is how later resource routes (`/tables/{id}/…`) enforce
  ACLs. Resolver decisions are cached per process; see
  [06](06-permissions.md#implementation).

### Views
| POST | `/tables/{id}/views` | `{ name, type, scope }` |
| PATCH/DELETE | `/views/{id}` | name, config (If-Match), locked, position |
| POST | `/views/{id}/duplicate` | |

### Rows & queries
| Method | Path | Notes |
|---|---|---|
| POST | `/views/{id}/query` | T1.15: `{ offset?, limit?, sort?: { fieldId, direction }, includeCount? }` → `{ rows: [{id, fields}], fieldIds, total, schemaVersion }`; limit ≤100, offset ≤1,000,000; total is null when count is disabled. T1.16 optionally accepts `fieldIds` as a transient visibility override (D-138), without changing stored view config. |
| POST | `/views/{id}/groups` | `{ path, search?, overrides? }` → buckets |
| POST | `/views/{id}/aggregates` | footer values |
| GET | `/tables/{id}/rows/{rowId}` | T1.15: `{ id, fields }`, all live fields including read-only system values; row ID is a canonical bigint decimal string. |
| POST | `/tables/{id}/rows` | T1.15: `{ rows: [{ fields: { fieldId: value } }] }`; ≤1,000 rows/5,000 cells, 512 KiB request. Physical defaults apply to omitted values. |
| PATCH | `/tables/{id}/rows` | T1.15 atomic bulk `{ rows: [{ id, fields }] }`; ≤1,000 rows/5,000 cells, 512 KiB request. |
| PATCH | `/tables/{id}/rows/{rowId}` | T1.15: `{ fields: { fieldId: value } }`; 4,096-byte request. |
| DELETE | `/tables/{id}/rows` | T1.15 atomic `{ ids: [...] }`, ≤1,000 IDs/64 KiB; returns deleted row snapshots. |
| POST | `/tables/{id}/rows:restore` | `{ rows: [snapshot] }` (undo of delete; same ids) |
| POST | `/tables/{id}/rows/{rowId}/links/{fieldId}` | `{ add: [ids], remove: [ids] }` |
| GET | `/tables/{id}/link-candidates/{fieldId}?q=&offset=` | rows of target table for picker |
| POST | `/views/{id}/form-submit` | form view submission |

T1.15 read requires effective table read and mutation requires effective table
write/owner; admin role does not bypass data grants. Bulk writes are all-or-
nothing. `created_by`/`updated_by` are UUIDs, not resolved user names.
Bulk patch locks use ascending numeric row ID order while response order
matches the request.
The production grid's T1.17 paste path sends valid edits to existing rows in a
single bounded bulk PATCH; codec-invalid cells are reported and omitted from
that request. After explicit confirmation, additional tail rows use the
bounded bulk-create route. The client must respect the row/cell/body limits
above and report oversize input rather than silently truncating it; no API
endpoint or authorization rule is added for clipboard operations (D-139).
NOT NULL and unique violations return sanitized `ERR_REQUIRED_FIELD` and
`ERR_UNIQUE_CONFLICT`; schema lock/serialization outcomes are
`ERR_DDL_LOCK_TIMEOUT` or `ERR_SCHEMA_CHANGED`. These implemented routes do
not include the deferred filters/search, restore, links, groups or aggregates.

T1.18's row-expand deep link uses the existing GET full-row response and
single-row PATCH `{fields:{fieldId:value}}`/`{row,eventId}` response, without
any new route or row-version requirement. The frontend edits one field at a
time and rejects JSON bodies exceeding the existing 4096-byte UTF-8 limit;
it never silently truncates long text or falls back to the larger bulk route.
The view in the URL must remain present in the caller's authorized table
schema before the dialog mounts; the row endpoint still independently enforces
table ACL. Hidden grid columns are not a permission boundary: every live field
is readable through the full-row endpoint, with system fields read-only.

T1.19 mobile cards use the same query and row contracts. Their hidden display
field is requested using transient fieldIds only; no view config is persisted.
The FAB opens a local draft in the existing row dialog; Apply does not issue
writes. Create sends exactly one row through POST `/tables/{id}/rows`, omitting
all untouched fields so PostgreSQL physical defaults apply. Explicit nulls
remain supplied values. The client checks the existing 5000-cell/512-KiB bounds
without truncation or splitting; existing-row saves remain single PATCH with
the 4096-byte bound. Failures never create optimistic rows. The new row URL
uses only the successful authoritative response ID. No mobile-specific API,
row version, validator, migration or relaxed authorization/body limit exists.

### Collaboration
| GET/POST | `/tables/{id}/rows/{rowId}/comments` | |
| PATCH/DELETE | `/comments/{id}` | author only (owner can delete) |
| GET | `/tables/{id}/rows/{rowId}/history` | paged |
| GET | `/notifications` · POST `/notifications/read` | |

### Files
| POST | `/tables/{id}/fields/{fieldId}/attachments` | multipart; size limit from settings |
| GET | `/files/{attachmentId}` · `/files/{attachmentId}/thumb` | ACL-checked; `Content-Disposition` |

### Import / export
| GET | `/views/{id}/export?format=csv\|xlsx` | streamed |
| POST | `/imports:analyze` | multipart file → sheets, columns, guessed types, PandaTable meta detected? |
| POST | `/imports` | `{ uploadId, target: {newTable: {bookId,name}} \| {tableId}, mapping }` → job |
| GET | `/imports/{jobId}` | progress / result |

## 3. WebSocket `/api/v1/ws`

Single connection per tab. JSON messages `{ "t": type, ... }`.

Client → server:
| t | Payload |
|---|---|
| `sub` | `{ tableId }` (server checks read access) |
| `unsub` | `{ tableId }` |
| `presence` | `{ tableId, viewId, cell?: { rowId, fieldId } \| null }` |
| `ping` | |

Server → client:
| t | Payload |
|---|---|
| `row.created` / `row.updated` / `row.deleted` | `{ tableId, ids, fields?, by, eventId }` — client refetches affected rows if visible |
| `schema.changed` | `{ tableId, schemaVersion }` — client reloads schema |
| `view.changed` | `{ viewId, version }` |
| `nav.changed` | sidebar must refresh (structure or grants changed for this user) |
| `presence` | `{ tableId, users: [{ userId, viewId, cell }] }` |
| `comment.created` | `{ tableId, rowId, count }` |
| `notification` | `{ id, kind, payload }` |
| `pong` | |
| `session.revoked` | user disabled / logged out elsewhere |
