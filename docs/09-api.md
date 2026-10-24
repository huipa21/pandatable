# 09 — API

Internal REST API used only by the SPA. Base path `/api/v1`. JSON in/out. Auth by session cookie.

## 1. Conventions

- **CSRF:** all non-GET requests must send header `X-PT-CSRF: 1` *and* pass Origin check against `PT_BASE_URL` (SameSite=Lax cookie as additional layer).
- **IDs:** structure objects use UUID strings; rows use integer `id`.
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

T1.2 implements the auth/profile/timezone backend below. Setup, admin, ACL and
business endpoints remain later-task work. The welcome page is still not a
sign-in UI; browser auth/profile acceptance remains pending by explicit scope choice.

## 2. Endpoints

### Setup & auth
| Method | Path | Notes |
|---|---|---|
| GET | `/setup/status` | `{ needed: bool }` |
| POST | `/setup` | `{ token, username, displayName, password, locale }` → first admin; 410 once done |
| POST | `/auth/login` | `{ login, password }` (username or email); rate-limited 10/min/IP, 5 failures → 1 min lock per account |
| POST | `/auth/logout` | |
| GET | `/auth/me` | user, role, prefs, mustChangePassword |
| POST | `/auth/password` | `{ current, new }` |
| PATCH | `/auth/me` | `{ displayName, locale, theme, timezone }` (timezone: IANA name or `null` = app default) |

#### T1.2 auth contract (D-072..D-077)

- Passwords are **8-32 UTF-8 bytes**, without trimming, normalization or composition
  rules. Login trims surrounding whitespace from the username/email only, and
  matches username/email case-insensitively. Multiple matching legacy identities
  fail closed. Future setup/admin creation must normalize and reject ambiguous
  identities; no schema or account-creation service is introduced here.
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
  Frontend consumers must translate codes when that UI is implemented.

### Admin (role=admin)
| Method | Path | Notes |
|---|---|---|
| GET/POST | `/admin/users` | list (search, status) / create `{username, email?, displayName, role, password}` |
| PATCH | `/admin/users/{id}` | displayName, email, role, status |
| POST | `/admin/users/{id}/reset-password` | `{ password }` → sets must_change_password |
| GET | `/admin/users/{id}/ownerships` | workspaces/books/tables where user is direct owner + sole-owner flag |
| POST | `/admin/users/{id}/transfer` | `{ toUserId }` transfers all direct owner grants |
| DELETE | `/admin/users/{id}` | hard delete; `?confirm=<username>`; deletes sole-owned workspaces (to trash bypassed → purge) |
| CRUD | `/admin/groups`, `/admin/groups/{id}/members` | |
| GET | `/admin/workspaces` | name, owners, size, orphaned |
| POST | `/admin/workspaces/{id}/owner` | `{ userId }` (orphan rescue) |
| GET/PATCH | `/admin/settings` | app name, default timezone, default currency, default locale, upload limit, retention days |
| GET | `/meta/timezones` | IANA timezone list (from `pg_timezone_names`) — any user |
| GET | `/meta/currencies` | ISO 4217 codes — any user |
| GET | `/admin/system` | CPU %, load, mem used/total, disk used/free, db size, files size, backups size, app RSS, uptime, versions |
| GET/POST | `/admin/backups` | list / create (async job) |
| GET | `/admin/backups/{id}/download` | streamed zip |
| DELETE | `/admin/backups/{id}` | |

### Directory (any user — for pickers)
| GET | `/directory/users?q=` | active users: id, displayName, username, avatarColor |
| GET | `/directory/groups?q=` | id, name |

### Structure
| Method | Path | Notes |
|---|---|---|
| GET | `/nav` | full sidebar tree the user may see (workspaces → books → tables → views) with effective levels |
| POST | `/workspaces` | |
| PATCH/DELETE | `/workspaces/{id}` | delete → trash |
| POST | `/workspaces/{id}/books` | |
| PATCH/DELETE | `/books/{id}` | |
| POST | `/books/{id}/tables` | `{ name, fields?: [...] }` |
| PATCH/DELETE | `/tables/{id}` | name, displayFieldId, position |
| GET | `/tables/{id}/schema` | fields, relations, views, schemaVersion, myLevel |
| POST | `/tables/{id}/fields` | create field (incl. link → creates relation + reverse field) |
| PATCH/DELETE | `/fields/{id}` | rename/options/constraints; delete → trash |
| POST | `/fields/{id}/convert:dry-run` | `{ uiType, pgType?, options }` |
| POST | `/fields/{id}/convert` | `{ …, onFail: "abort"\|"null" }` |
| POST | `/formula:check` | `{ tableId, expr }` → type / error position |
| GET | `/trash?scope=workspace:{id}` | |
| POST | `/trash/{type}/{id}/restore` | |
| DELETE | `/trash/{type}/{id}` | purge now |

### Permissions
| GET | `/grants?resource=workspace:{id}` | direct grants on resource + inherited summary |
| PUT | `/grants` | `{ resourceType, resourceId, principalType, principalId, level }` (upsert) |
| DELETE | `/grants/{id}` | |

### Views
| POST | `/tables/{id}/views` | `{ name, type, scope }` |
| PATCH/DELETE | `/views/{id}` | name, config (If-Match), locked, position |
| POST | `/views/{id}/duplicate` | |

### Rows & queries
| Method | Path | Notes |
|---|---|---|
| POST | `/views/{id}/query` | `{ offset, limit, search?, overrides?: {filter, sort}, groupPath? }` → `{ rows, total }` |
| POST | `/views/{id}/groups` | `{ path, search?, overrides? }` → buckets |
| POST | `/views/{id}/aggregates` | footer values |
| GET | `/tables/{id}/rows/{rowId}` | full row (all fields) |
| POST | `/tables/{id}/rows` | `{ rows: [{fields}] }` (≤ 1000) |
| PATCH | `/tables/{id}/rows` | bulk `{ rows: [{id, fields}] }` (paste/fill-down, ≤ 5000 cells) |
| PATCH | `/tables/{id}/rows/{rowId}` | `{ fields }` |
| DELETE | `/tables/{id}/rows` | `{ ids: [...] }` → returns deleted rows snapshot (for undo) |
| POST | `/tables/{id}/rows:restore` | `{ rows: [snapshot] }` (undo of delete; same ids) |
| POST | `/tables/{id}/rows/{rowId}/links/{fieldId}` | `{ add: [ids], remove: [ids] }` |
| GET | `/tables/{id}/link-candidates/{fieldId}?q=&offset=` | rows of target table for picker |
| POST | `/views/{id}/form-submit` | form view submission |

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
