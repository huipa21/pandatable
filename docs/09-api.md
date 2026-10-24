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
