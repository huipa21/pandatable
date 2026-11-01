# Acceptance Tests

Format: **ID — Title** · *Milestone* · Given / When / Then. Automated as Playwright E2E (`web/e2e/`) unless marked *(API)* (Go integration test) or *(manual)*.
Test users used throughout: `admin` (role admin), `alice`, `bob`, `carol` (role user); group `team-a` = {bob, carol}.

## AT-0xx Foundation (M0)

- **AT-001 — Stack starts** · M0 · Given the laptop with Podman and a completed `deploy/.env`, from the repository root run `podman compose --env-file deploy/.env -f deploy/compose.yaml -f deploy/compose.local.yaml up -d`; then `GET http://127.0.0.1:8080/healthz` returns 200 within 60 s and the SPA loads. Same with Docker on Linux. Disposable checks may choose a unique `-p` project and `PT_LOCAL_HTTP_PORT`.
- **AT-002 — Migrations idempotent** · M0 *(API)* · When the app restarts twice, then migrations run once and no error is logged.
- **AT-003 — Language & theme toggles** · M0 · Given the login page, when the user clicks 中文 then 🌙, then UI text switches to Chinese and dark theme applies immediately; after reload, both persist.
- **AT-004 — Image contents** · M0 *(manual)* · `<engine> run --rm --entrypoint pg_dump localhost/pandatable:<ver> --version` prints 18.x; image is linux/amd64; size < 200 MB; runs as non-root; HEALTHCHECK present (`docker inspect`).
- **AT-005 — Laptop → server path** · M0 *(manual)* · `ptdev image build` + `ptdev image save --with-postgres` on Windows/Podman → copy archives → `docker load` on Ubuntu → `docker compose up -d` with `PT_PROXY_NETWORK` = Caddy's network → app reachable at `https://<domain>` through the existing Caddy, WebSocket upgrade succeeds (`/api/v1/ws` once implemented; `/healthz` in M0); app and db publish no host ports.
- **AT-006 — Local gate cross-platform** · M0 *(manual)* · `go run ./tools/ptdev check` succeeds in Windows PowerShell (Podman) and in Linux bash (Docker); with the DB stopped, `go test ./...` skips integration tests with a clear message instead of failing/hanging.

### T0.6 scoped evidence (2026-10-24)

| Test | Status | Evidence / remaining requirement |
|---|---|---|
| AT-001 | Partial | Windows/Podman isolated Compose: health 200 in 6.534 s, real SPA/assets/deep route/browser/CSP pass. Linux/Docker still unavailable. |
| AT-004 | Pass | linux/amd64, UID/GID 10001, executable HEALTHCHECK (inspection + archive), pg_dump 18.6, uncompressed image 161,964,427 bytes (<200 MB); persistent storage writable. |
| AT-005 | Partial | Both gzip Docker archives have complete manifests/layers and load with Podman. No Docker executable or approved Ubuntu/Caddy destination: actual transfer, Docker load/start and external HTTPS remain unverified. |
| AT-006 | Pending | Windows local gate passes; Linux tooling cross-compiles and POSIX deploy-script unit tests pass under Podman. Linux/Docker runtime and T0.7 shared harness remain required. |

Exact versions, image/archive identifiers, cleanup and failure-path evidence:
[progress.md, T0.6](archive/progress-m0-m1.md#t06-verification-evidence-2026-10-24).

### T0.7 scoped evidence (2026-10-24)

| Test | Status | Evidence / remaining requirement |
|---|---|---|
| AT-001 | Partial | Automated Windows/Podman image/Compose run: health in 6.238 s; four desktop/mobile shell checks pass. Linux/Docker remains unavailable. |
| AT-002 | Pass preserved | Shared migrated disposable package databases; concurrent/rerun migrations and two real app restarts retain one unchanged migration record. Owned DBs removed afterwards. |
| AT-003 | Pass preserved | Embedded production shell on desktop/mobile Chromium: en/zh-CN and light/dark/system changes/reload/live OS preferences pass with no browser/CSP errors. No actual login/auth flow is claimed. |
| AT-004 | Pass preserved | Original release image reinspection: linux/amd64, UID/GID 10001, executable HEALTHCHECK, pg_dump 18.6, 161,964,427 bytes. |
| AT-005 | Partial, unchanged | No Docker executable or approved Ubuntu/Caddy destination; no actual remote load/HTTPS checks performed. |
| AT-006 | Partial | Windows PowerShell/Podman `ptdev check` passes; shared PG18 isolation/cleanup and bounded closed-port skips pass. Invalid URL/auth/missing DB and required-DB unreachability fail instead of skipping. Linux bash/Docker gate remains required. |
| AT-802 | Pass preserved | Strict frontend/E2E TS, ESLint/Prettier, 21-key locale parity and negative i18n fixtures pass in the local gate. |

Real browser failure/cancellation/partial-start cleanup and exact commands:
[progress.md, T0.7](archive/progress-m0-m1.md#t07-verification-evidence-2026-10-24).
T0.7 remains in progress until its linked **full AT-006** passes.

### T0.8 scoped evidence (2026-10-24)

| Test | Status | Evidence / remaining requirement |
|---|---|---|
| AT-001 | Partial, preserved | Windows/Podman isolated image/Compose health in 6.414 s; real production shell plus spike suite passes. Linux/Docker still pending. |
| AT-002 | Pass preserved | Required live PG18 integration tests and application restart/migration tests pass in `ptdev check`; T1.1's additional meta schema is not yet implemented. |
| AT-003 | Pass preserved | Existing four desktop/mobile shell checks pass, including language and light/dark/system persistence; spike also verifies localized grid headers, light/dark styles and mobile exclusion. |
| AT-004 | Pass preserved | New disposable image: linux/amd64, UID/GID 10001:10001, executable HEALTHCHECK, 162,373,088 bytes (<200 MB); runtime `pg_dump --version` probe succeeds. Original T0.6 release retained. |
| AT-005 / AT-006 | Partial, unchanged | No approved server/Caddy destination or Linux/Docker runtime check. Windows `ptdev check` passes; no remote changes. |
| AT-802 | Pass preserved | ESLint/i18n negative fixtures, strict frontend/E2E TS, Prettier and **61** matching non-empty locale keys pass. |
| AT-305..313 / NFR-2..3 | Not claimed | Synthetic provider/window/editing/grouping/touch feasibility is not real table/query/seed, persisted editing or mobile-card acceptance. |

T0.8 has no linked AT row; its explicitly scoped spike demonstrations are
complete. `ptdev e2e` reports **8 passed, 2 intentionally skipped in 27.3 s**:
four existing shell checks, desktop windowing, desktop interactions, mobile
isolation and a tablet observation. Exact measurements, limits and D-005
conclusion: [progress](archive/progress-m0-m1.md#t08-verification-evidence-2026-10-24) and
[T0.8 record](task-prompts/T0.8.md#implementation-and-handoff-2026-10-24).
M0/T0.4/T0.6/T0.7 remain in progress for their unrelated pending platform checks.

### T1.1 scoped evidence (2026-10-24)

| Test | Status | Evidence / remaining requirement |
|---|---|---|
| AT-002 | Pass, T1.1 complete | New metadata 0002 plus unchanged 0001: real PG18 clean catalog/default/constraint/FK tests, concurrent/repeated upgrade, two populated down/up cycles with preserved user/session/settings sentinels, extension OIDs and migration-1 record. Initial serve plus two real restarts retain all version records exactly (versions 1/2 each applied once), no errors and zero remaining app connections. Owned DBs dropped. |
| AT-001 | Partial, preserved | Windows/Podman isolated image/Compose health in 6.354 s; production shell and spike checks pass. Linux/Docker remains pending. |
| AT-003 | Pass preserved | All four existing desktop/mobile shell checks plus spike checks pass with no browser/CSP errors. No actual login/auth flow is claimed. |
| AT-004 | Pass preserved | New disposable image: linux/amd64, UID/GID 10001:10001, executable HEALTHCHECK, pg_dump 18.6, 162,381,278 bytes (<200 MB); inspected/probed before owned teardown. |
| AT-005 / AT-006 | Partial, unchanged | No Docker executable or approved server/Caddy destination. Required live PG18 Windows check passes; Linux/Docker and remote acceptance remain pending. |
| AT-802 | Pass preserved | ESLint/i18n negative fixtures, strict frontend/E2E TS, Prettier and 61 matching non-empty locale keys pass. |

Final image-backed E2E: **8 passed, 2 intentional mobile exclusions in 32.9 s**.
The earlier supplemental image probe's engine-format error, corrected rerun,
exact commands, assertions and cleanup are recorded in
[progress](archive/progress-m0-m1.md#t11-verification-evidence-2026-10-24) and the
[T1.1 completion record](task-prompts/T1.1.md#implementation-and-handoff-2026-10-24).
T1.1 is Done; unrelated M0/platform and future auth/business acceptance are not.

## AT-1xx Auth & Admin (M1, M3)

- **AT-101 — First-run setup requires token** · M1 · Given an empty DB, visiting any page redirects to `/setup`; submitting a wrong token fails; submitting the logged token creates admin and logs in; afterwards `/setup` returns "already set up" (410).
- **AT-102 — Login / logout** · M1 · Login by username and by email succeeds; wrong password shows translated error; logout invalidates the session (back button can't access data).
- **AT-103 — Login rate limit** · M1 *(API)* · 5 wrong passwords for `alice` → next attempt within 1 min returns 429 even with correct password.
- **AT-104 — No self-registration** · M1 · No sign-up link exists; `POST /api/v1/admin/users` as non-admin → 403.
- **AT-105 — Admin creates user, forced password change** · M1 · Admin creates `alice` with temp password; alice logs in and is forced to `/change-password` before anything else.
- **AT-106 — Admin resets password** · M1 · Admin resets bob's password → bob's existing sessions are revoked; bob must change password at next login.
- **AT-107 — Disable user** · M1 · Admin disables carol → carol's open tab gets logged out within 5 s (M3: via `session.revoked`; M1: on next request); carol cannot log in; carol's data/authorship remains.
- **AT-108 — Admin panel visibility** · M1 · `alice` sees no Admin menu and `/admin/*` routes redirect; API returns 403.
- **AT-109 — Groups** · M1 · Admin creates `team-a`, adds bob & carol; membership visible in group detail and in user detail.
- **AT-110 — Dashboard metrics** · M1 · Admin dashboard shows CPU %, memory used/total, DB size, files size, disk free; values refresh (≤ 10 s) and are non-zero.
- **AT-111 — Settings** · M1 · Admin changes app name → header shows new name for all users after reload; upload limit change is enforced (AT-322).
- **AT-112 — Admin has no implicit data access** · M1 · alice creates workspace W; admin does not see W in sidebar; `GET` W's tables as admin → 404; Admin → Workspaces lists W with owner alice.
- **AT-113 — Orphaned workspace rescue** · M3 · alice (sole owner of W) is disabled → W flagged orphaned in Admin → Workspaces; admin assigns bob as owner → bob sees W with owner rights.
- **AT-114 — Transfer ownerships** · M3 · Admin opens disabled alice → "Ownerships" lists W; "Transfer to bob" → bob is owner of everything alice owned directly.
- **AT-115 — Hard delete user** · M3 · Admin hard-deletes alice after typing her username → workspaces where she was sole owner are gone (incl. physical schemas); in shared workspaces her rows show created by "Deleted user"; her personal views are gone.
- **AT-116 — Self profile** · M1 · alice changes display name and password; new password works, old doesn't.
- **AT-117 — Timezone** · M1/M2 · Default timezone is Asia/Shanghai for new users; a datetime saved as 2026-01-01 00:30 by alice (Asia/Shanghai) displays as 2025-12-31 16:30 for bob after he sets Europe/London; (M2) filter `is today` uses each user's own timezone; admin changing default timezone affects users without their own setting.

### T1.2 backend evidence (2026-10-25)

| Test | Status | Evidence / remaining requirement |
|---|---|---|
| AT-102 | Partial | Real HTTP/PG18 username/email login, generic errors, logout and revoked-cookie rejection pass. User explicitly deferred auth frontend: translated errors and browser/back-button behavior remain pending. |
| AT-103 | Pass | Five bad credentials across username/email/case aliases cause the same one-minute account lock; a correct password is rejected. Controllable-time tests cover exact expiry/reset. Ten/minute/IP also counts malformed login requests; spoofed untrusted forwarding cannot bypass it. |
| AT-116 | Partial | Actual HTTP/PG profile/password changes persist; new password works, old fails, every session is revoked. Profile UI/browser acceptance remains pending by user choice. |
| AT-117 | Partial | PG-catalog timezone validation, NULL/team default (Asia/Shanghai if absent), persistence and UTC DB sessions pass. No real datetime field/display/filter or admin-default UI acceptance is claimed. |
| AT-803 | Pass | Actual auth HTTP/PG18 tests reject missing CSRF header, missing/foreign Origin; HTTPS cookie flags, no-store, CSP/nosniff and proxy boundaries pass. D-073 explicitly permits non-Secure cookies only for literal-local HTTP development and rejects other HTTP auth configuration. |
| AT-002/003/004/802 | Pass preserved | Required PG18 migration/restart gate, unchanged shell/spike E2E, 61-key locale parity, non-root 162,627,041-byte linux/amd64 image with exec-form HEALTHCHECK and pg_dump 18.6. |
| AT-001/005/006 | Partial, unchanged | Windows/Podman gate passes; Linux/Docker and approved Ubuntu/Caddy checks remain pending. |

Final image E2E: **8 passed / 2 intentional mobile exclusions in 34.7 s**,
health **8.003 s**, no browser/CSP errors. This is the preserved shell/spike
suite, **not auth UI acceptance**. T1.2 remains in progress, not Done.
Exact commands, failure corrections, CPU/memory observations and owned cleanup:
[progress](archive/progress-m0-m1.md#t12-backend-verification-and-handoff-2026-10-25).

### T1.3 scoped evidence (2026-10-25)

| Test | Status | Evidence / remaining requirement |
|---|---|---|
| AT-101 | Pass | Production-image Chromium setup: empty-instance redirect, wrong-token 401, desktop and Pixel 7 form layout, first-admin creation, authenticated `/auth/me`, completed status and POST 410. Browser/CSP diagnostics clean. |
| AT-002/003/004/802 | Pass preserved | Required PG18 migration/restart gate; shell/spike image checks; initial shell gzip 138,976 bytes (<600,000); 80 matching non-empty locale keys. |
| AT-103/803 | Pass preserved | Existing real HTTP/PG18 rate-limit, CSRF/Origin, cookie and security-header coverage remains green in the required gate. |
| AT-102/116 and full AT-117 | Partial, unchanged | Login/profile UI and real datetime/filter/admin-default behavior remain outside T1.3. |
| AT-001/005/006 | Partial, unchanged | Windows/Podman local evidence only; Linux/Docker and approved server/Caddy checks remain pending. |

The required `ptdev check` passes with uncached PG18. Final image E2E reports
**9 passed / 2 intentional mobile exclusions in 45.2 s**, health **6.934 s**,
and no browser/CSP errors. The one-shot app snapshot was **72.8 MB / 536.9 MB**
at **11.63% CPU**; this is not AT-801 load certification. Exact commands,
versions, resolved failures and owned cleanup are recorded in
[progress](archive/progress-m0-m1.md#t13-setup-verification-and-handoff-2026-10-25) and the
[T1.3 completion record](task-prompts/T1.3.md#implementation-and-handoff-2026-10-25).

### T1.5 admin users evidence (2026-10-26)

| Test | Status | Evidence |
|---|---|---|
| AT-104 | Pass | No registration UI; real non-admin create request gets 403 `ERR_PERMISSION_DENIED` (unauthenticated 401). |
| AT-105 | Pass | Desktop/mobile production UI creates normalized users with temporary passwords; login/deep routes force `/change-password`; successful change revokes sessions and requires re-login. |
| AT-106 | Pass | Admin reset through the real UI revokes the existing browser session (me=401); reset password login requires change. PG18 verifies all-session atomicity/rollback and old-password login race. |
| AT-107 | Pass | Disable sends `session.revoked` and logs the open tab out within 5 s; login is denied; retained identity is visible and re-enables. PG18 verifies retained workspace/creator/grants and immediate cache invalidation (no TTL wait). |
| AT-108 | Pass | Non-admin sees no Admin link; `/admin/users` and unknown `/admin/*` deep routes redirect home; independent API denial and forced/disabled authorization verified. |
| AT-002/003/004/101/103/802/803 | Preserved | Required PG18 Windows gate and production setup/shell/spike suite pass; image 162,975,200 bytes, linux/amd64, UID/GID 10001, healthcheck and PG18 client; 143 locale keys, CSP unchanged. |

Final production-image suite: **11 passed / 2 intentional mobile exclusions**.
Both admin scenarios cover role edits, search, en/zh-CN and responsive cards/forms.
No full AT-102/116/117, Linux/Docker/server, realtime or load acceptance is
inferred. Exact commands, failures and cleanup:
[T1.5 record](task-prompts/T1.5.md#implementation-and-handoff-2026-10-26).

### T1.6 admin groups evidence (2026-10-26)

| Test | Status | Evidence |
|---|---|---|
| AT-109 | Pass | Production-image desktop/mobile creates `team-a`, adds Bob/Carol through UI, verifies group detail and both user details, removes membership, edits in en/zh-CN, confirms/cancels deletion, checks empty/search/error states and responsive layouts. Fixtures seed only via real setup/admin APIs. |
| AT-104..108 | Preserved | Same production scenario retains forced change, role/reset/disable/re-enable and admin visibility; group/user-detail deep routes and API independently deny non-admins. No extra sign-ins or rate-limit bypass. |
| AT-207 API behavior | Preserved | Actual admin membership add/remove/deletion invalidates primed ACL caches immediately, not after TTL. PG18 proves rollback, concurrency with real grants, principal cleanup and commit-error invalidation. Remaining resource/editor/sidebar UI stays pending T1.10-T1.12. |
| AT-101/103/802/803 | Preserved | Full setup/shell/spike image suite, required PG18 auth/CSRF tests, 169 matching locale keys and no-inline/no-eval CSP pass. Shell gzip 166,667 bytes; no grid imports. |

Final image suite: **11 passed / 2 intentional mobile exclusions**. Full local
check passes with command-scoped `GOFLAGS=-p=1`; default-parallel checks failed
twice in existing testdb cleanup, with unchanged assertions/timeouts and isolated
x3 pass. This caveat is not a default-gate pass or load/platform certification.
Exact verification, failures, resources and cleanup:
[T1.6 completion record](task-prompts/T1.6.md#implementation-and-handoff-2026-10-26).

### T1.7 admin dashboard evidence (2026-10-26)

| Test | Status | Evidence |
|---|---|---|
| AT-110 | Pass | Real production-image desktop/mobile admin dashboard shows actual CPU, RAM, PG/database connections/shared buffers, disk, RSS/cgroup, exact 4096-byte attachment and 2048-byte backup fixture totals, runtime/server/migration versions and uptime. Rendered refresh measured 9904 / 9855.5 ms (both <=10 s); hourly directory timestamps unchanged. en/zh-CN, loading, explicit stale/error/retry, navigation and independent non-admin deep-route/API denial pass. |
| AT-101/104..109/802/803 | Preserved | Full production setup/admin/shell/spike suite, required PG18 auth/admin/ACL regressions, 226 matching locale keys and unchanged no-inline/no-eval CSP. Initial gzip 168,887 bytes, no grid imports. Empty directory zero and first CPU sample unavailable are explicitly tested, never fabricated. |
| AT-801 / NFR-1 | Not certified | Linux collector microbenchmarks are small, but image-backed post-auth RSS is 170,131,456..175,009,792 bytes, above the 150 MB normal-load target. No target-host/load certification or forced GC/password weakening to hide this observation. |

Final full image suite: **11 passed / 2 intentional mobile-grid exclusions
(1.4m)**. Required PG18 regressions pass x3. Final complete local gate passes
with command-scoped `GOFLAGS=-p=1`; two default-parallel attempts reproduce
existing Suite cleanup deadline failures, not claimed green. Linux filesystem
tests additionally pass non-root under a 2-CPU/512-MiB cap, including symlink,
permission and special-file policies; this is not full Linux/Docker acceptance.
Exact commands, failures, costs and cleanup:
[T1.7 completion record](task-prompts/T1.7.md#implementation-and-handoff-2026-10-26).

### T1.8 available-scope evidence (2026-10-26)

| Test | Status | Evidence / remaining requirement |
|---|---|---|
| AT-111 | Pass | Combined `admin-users.spec.ts` production-image scenario passes desktop and mobile: app name/settings persist, ordinary and unauthenticated browser headers reload, and the public DTO exposes only appName/defaultLocale. AT-322 enforcement is verified by `attachments.spec.ts`; T1.8 remains In Progress for AT-112/117. |
| AT-112 | Partial | Required Suite-owned PG18 inventory/owner/orphan/size/paging and owner-only grants versus admin 404/ACL denial pass. Production browser verifies the real empty read-only inventory, navigation/guards and no resource links. No workspace creation/navigation/table APIs exist; SQL fixtures and unit-populated cards are not the full alice-create/sidebar/table-GET UI scenario. Await T1.10/T1.11. |
| AT-117 | Partial | Real settings changes reach existing NULL-timezone auth/profile readers and new-user defaults; explicit timezone/locale choices survive. Datetime display, relative filters and profile UI remain deferred. |
| AT-321 | Pass | `attachments.spec.ts` production-runtime desktop scenario creates the field in Field UI, uploads a PNG by picker and PDF by drag/drop, persists/reloads both, verifies thumbnail/download, and confirms an unauthorized user gets 404. Mobile is an intentional spec skip. |
| AT-322 | Pass | `attachments.spec.ts` lowers persisted uploadMaxMb and verifies an over-limit upload returns 413 with the localized error; `admin-users.spec.ts` also passes the full settings browser scenario on desktop/mobile. |
| AT-113 | Pass | T3.10 desktop/mobile browser and PG18 tests verify the orphan flag, active-user rescue, owner access and no implicit admin data access; disabled or group-only owners do not prevent orphan status. |
| AT-114 | Pass | T3.10 desktop/mobile browser and PG18 tests list disabled-user ownerships and transfer all direct workspace/book/table grants, including Trash; the new owner can restore and access the transferred workspace. |
| AT-115 | Pass | T3.10 desktop/mobile browser verifies exact username confirmation, retained shared rows and live localized deleted-user authorship. PG18 checks physical schema and personal-view removal, shared data/attachments, atomic rollback, localized export and durable cleanup failure/retry. |

T1.8 remains **In progress**, not Done. Clarification and the requested AT-112
UI-verification question were asked; user unavailable, so D-091..D-093 record
bounded implementation choices and honest future-API dependencies.
Exact verification, resource caveats and cleanup:
[T1.8 completion record](task-prompts/T1.8.md#implementation-and-handoff-2026-10-26).

## AT-2xx Structure & Permissions (M1)

### T1.4 API-surface evidence (2026-10-26)

| Test | Status | Evidence / remaining requirement |
|---|---|---|
| AT-209..212 | Pass (API) | Real HTTP/PG18: book owner manages B/T only (403 on W, 404 on another book), workspace owner grants owner, last owner `ERR_LAST_OWNER` then allowed after a second owner, no-grant users and admin role get 404. |
| AT-204..208 | Partial | Resolver/grants API behavior passes (read/write probes and 403s, raise-only with `inheritedLevel` warning data, group grant removal with invalidation + 5 s TTL bound, table-only API visibility). Disabled editors/menus, warning banner, tab behavior and sidebar presentation await T1.10-T1.12. |

Tests: `internal/acl` and `internal/httpapi` (required PG18, `-count=3`);
membership removal is simulated by SQL plus `Invalidate()` until T1.6. See
[progress](archive/progress-m0-m1.md#t14-acl-verification-and-handoff-2026-10-26).

- **AT-201 — Create hierarchy** · M1 · alice creates workspace W, book B, table T → sidebar shows W ▸ B ▸ T; T has a default Grid view and a primary "Name" field; DB has schema `b_*` and table `t_*`.
- **AT-202 — Rename is metadata only** · M1 *(API)* · Renaming T/field to Chinese names with spaces works; physical names unchanged.
- **AT-203 — Delete goes to trash** · M1/M3 · alice deletes T → disappears from sidebar; (M3) appears in W's Trash and can be restored with data intact.
  T3.6: **Pass** — fast desktop/mobile browser deletion removes the sidebar node and schema access; Trash restores the table and retained row. PG18 also checks owner filtering, nested restoration and retained attachments. [Record](task-prompts/T3.6.md#record-2026-11-01).
- **AT-204 — Workspace grant read** · M1 · alice grants bob `read` on W → bob sees W, B, T; can view rows; cannot edit cells (editors disabled), cannot add rows, cannot see field/schema menus; API write → 403.
- **AT-205 — Write level** · M1 · bob with `write` on B can add/edit/delete rows in T and create a shared view, but cannot add/delete fields or tables (403).
- **AT-206 — Raise-only inheritance** · M1 · bob `read` on W + `write` on T → bob can edit T only; other tables read-only. Setting bob `read` on T when he has `write` on W shows warning "inherited level higher" and bob still writes.
- **AT-207 — Group grants** · M1 · `team-a` gets `write` on B → carol can edit B's tables; removing carol from team-a removes access within 5 s.
- **AT-208 — Table-only grant visibility** · M1 · carol granted `read` only on T → sidebar shows W ▸ B ▸ T only (no sibling tables/books); W/B settings inaccessible.
- **AT-209 — Scope owner manages sub-scope** · M1 · alice grants bob `owner` on B → bob can grant carol `write` on B or T, but cannot manage grants on W (403) or on another book.
- **AT-210 — Owner can grant owner** · M1 · alice grants bob `owner` on W → bob can manage all grants in W, including granting `owner` to carol.
- **AT-211 — Last owner protected** · M1 · alice (only owner of W) cannot remove or downgrade her own owner grant (`ERR_LAST_OWNER`); after granting bob owner, she can.
- **AT-212 — Hidden resources are 404** · M1 *(API)* · dave (no grants) requesting T's rows gets 404, not 403.
- **AT-213 — Any user creates workspaces** · M1 · carol creates W2 and is its owner.

### T1.9 physical-only evidence (2026-10-26)

| Test | Status | Evidence / remaining requirement |
|---|---|---|
| AT-202 | Pending API scenario | Required Suite-owned PG18 tests demonstrate parameterized Chinese/space/payload display-name metadata changes leave physical names/data unchanged. Real table and field rename APIs await T1.10/T1.14; this is not an API acceptance pass. |
| AT-804 | Partial lower-layer evidence only | Exact identifier/type/default rejection and safe E-string/cast literals, real catalog/injection tests with both string-conformance modes pass. Real table/field-name and filter-value API scenarios await T1.10/T1.14/T2.6. |

T1.9 remains **In progress**, not Done. Unit/PG18 DDL tests additionally cover
identity/system columns, typed native storage/defaults, constraint removal,
metadata+DDL rollback, observed lock contention/three-second timeout and LOCAL
reset. No test production endpoint, codec, filter engine or browser seed was
introduced. Full AT-111/112/117/322 dependencies are unchanged. See
[T1.9 completion record](task-prompts/T1.9.md#implementation-and-handoff-2026-10-26).

### T1.10 API evidence (2026-10-26)

| Test | Status | Evidence / remaining requirement |
|---|---|---|
| AT-201 | API pass; UI pending | Real HTTP/PG18: alice creates W ▸ B ▸ T; `/nav` shows the tree; the table has a default Grid view, the Name text display field and five hidden system fields; catalog shows `b_*` schema and `t_*` table. The sidebar rendering is T1.11. |
| AT-202 | Table API pass; field rename pending | A real `PATCH /tables/{id}` renames to Chinese/space/quote/payload text with physical names and data unchanged. Field rename needs T1.14. |
| AT-203 | API pass; Trash UI pending | `DELETE` hides the table from nav/schema/ACL (primed cache invalidated at once) and retains physical data, fields, views and grants. M3 restore and sidebar removal are future work. |
| AT-213 | API pass | Any user (including the admin role) creates a workspace with an atomic direct owner grant; the creator lists the grant as owner. UI part is T1.11. |
| AT-112 | Partial | Real alice-created workspace: admin `/nav` is empty, schema/grants/PATCH/DELETE are 404 and the admin inventory lists the workspace with owner alice. The browser sidebar scenario awaits T1.11. |
| AT-204..212 | Preserved | Resolver/grants/API matrix unchanged; structure routes add owner/visibility matrices for read/write/book-owner/table-only/group/admin/stranger. |

T1.10 stays **In progress** until the UI parts pass. Unit and required PG18 tests do
not complete AT-111/117/322, AT-202 field rename or full AT-804. See the
[T1.10 completion record](task-prompts/T1.10.md#implementation-and-handoff-2026-10-26).

### T1.11 real browser evidence (2026-10-26)

| Test | Status | Evidence / remaining requirement |
|---|---|---|
| AT-201 | Pass | Alice creates W/B/T through the real UI on desktop/mobile; sidebar hierarchy and real schema show Grid/Name, owner access, five system fields. Required PG18 API/catalog assertions also pass. No SQL browser fixtures or mocked resources. |
| AT-208 | Pass | Real table-only read grant to Carol: exactly W/B/T, both ancestors path-only, no sibling books/tables or settings/structure controls. Direct path-only context opens only visible children. |
| AT-213 | Pass | Carol creates W2 through UI with her real account; actual `/nav` reports direct resulting owner access. Fixture cleanup uses her own API rights. |
| AT-112 | Pass | Alice creates W; independent admin sidebar excludes it and table-schema GET returns 404; populated read-only admin inventory lists W and Alice's ownership without links/controls. Replaces the earlier empty-inventory/API-only boundary. |
| AT-203 | M1 pass; M3 pending | Cancel and confirmed real UI Trash deletion; tree node disappears immediately, table route is removed/not-found and schema GET is 404. Physical retention verified by PG18. Restore with data intact remains M3, not claimed. |
| AT-202 | Table rename pass; field rename pending | Real UI Chinese workspace/book/table metadata renames plus required PG18 table-name/data preservation tests pass. Field rename requires T1.14. |
| AT-802/803 | Preserved | 304 matching keys, negative inline-string fixtures, strict TS, no-inline/no-eval production CSP and real auth/CSRF regressions pass. |
| AT-111/117/322/804 | Dependencies retained | Upload enforcement T2.2, real datetime/filter behavior and field/filter API injection scenarios remain pending. Sharing/editor UI parts of AT-204..211 remain T1.12 and later grid tasks. |

T1.11 and T1.10 are Done for their M1 scope; AT-203's M3 portion stays pending.
Full image suite **11 passed / 2 intentional mobile-grid exclusions (2.1m)**;
52 frontend tests, required uncached PG18 and command-scoped serial full gate pass.
Default-parallel Suite cleanup still fails separately and NFR-1 is not certified.
Exact commands, failures, resources and cleanup:
[T1.11 record](task-prompts/T1.11.md#implementation-and-handoff-2026-10-26).

### T1.12 real M1 sharing evidence (2026-10-26)

| Test | Status | Real production-image desktop/mobile evidence / remaining boundary |
|---|---|---|
| AT-204 | Sharing scope pass; full test pending | Alice uses the real user picker to grant/replace workspace read. Bob sees the hierarchy and schema read level, no Share, and cannot use the owner directory. Rows/editors/add-row/schema-menu/write-row behavior awaits actual field/row/grid APIs/UI; no proxy credit. |
| AT-205 | Sharing scope pass; full test pending | Book write reaches actual table schema; Bob cannot create a table (403) or share. Row CRUD/shared-view/field mutation acceptance awaits those APIs/UI. |
| AT-206 | Sharing scope pass; full test pending | Workspace read + table write leaves the sibling read; workspace write + direct table read warns using real inheritedLevel and preserves actual schema write. Removing the direct grant retains inherited access. Persisted cell editing is still pending. |
| AT-207 | Sharing scope pass; full test pending | Real group picker grants book write to Carol; real admin membership removal immediately denies schema (404, measured <5 s), and focus refresh removes the resource. Row editing remains pending. |
| AT-208 | Pass preserved and extended | Real table-only read pruning/path-only contexts remain; table-only owner can share the table but cannot rename it without book ownership. Read/write/path-only/admin-only nodes have no Share. |
| AT-209 | Pass | Book-only owner Bob shares his book/table with Carol; parent workspace grants 403, hidden sibling book grants 404. Book rename still requires workspace owner. |
| AT-210 | Pass | Workspace owner Bob grants Carol owner through the real picker/UI; inherited/direct scope controls remain authoritative. |
| AT-211 | Pass | Real last active direct user owner removal and downgrade return localized ERR_LAST_OWNER. After another active direct user owner exists, Alice can downgrade herself; Share disappears and privileged state clears. Bob restores her through the same real UI. |
| AT-112/201/203/213 | Pass preserved, M1 boundaries unchanged | Existing real navigation/admin isolation/creation/Trash-removal scenario still passes. M3 restoration remains pending. |
| AT-111/117/202/322/804 | Dependencies retained | Upload enforcement, real datetime/filter behavior, field rename and filter APIs are not implemented by sharing. |

Final full image project `pt-e2e-24b43zcuwjbe3jlogmxisv224d`: **11 passed /
2 intentional mobile-grid exclusions (3.3m)**; both expanded real account/
navigation/sharing scenarios pass. Additional real principal deletion/disable
between selection and confirmation returns localized errors; disabled users
disappear from search. Both locales/themes, modal keyboard/focus/drawer/touch,
viewport bounds and strict console/CSP diagnostics pass. No mocked production
picker, SQL/browser seed, extra sign-in budget, row/editor proxy or auth shortcut.

Required uncached PG18 and scoped-serial full check pass; **66 frontend tests /
18 files**, **348 matching locale keys**, initial gzip **184,252 bytes** without
initial grid imports. Default-parallel Suite cleanup still fails separately;
post-auth RSS still exceeds 150 MB, so NFR-1 is not certified. T1.12 remains
**In progress** for its linked full AT-204..207 dependencies, not a claim that
future row/editor functionality passed. Exact commands, failures/resources and
owned-only cleanup: [T1.12 record](task-prompts/T1.12.md#implementation-and-handoff-2026-10-26).

### T1.13 codec evidence (2026-10-26; lower-layer only)

| Test | Status | Evidence / remaining boundary |
|---|---|---|
| AT-301 | Codec/PG scope passes; full pending | All 12 batch-1 writable kinds and five system kinds bind with matching Go/frontend options/capabilities. Suite-owned native PG18 columns round-trip real typed values and validate descriptors/defaults/constraints. Real owner field creation/grid entry/persistence/reload requires T1.14..T1.16. |
| AT-302 | Codec/component scope passes; full pending | Shared invalid-input/precision/range/Unicode/date/DST/select/email/URL vectors, localized native editor failures, IME handling and escaping pass. No real row API exists to prove invalid grid values are not saved. |
| AT-304 | Codec/component scope passes; full pending | System fields reject writes/defaults, expose no editor and encode actual PG system columns. Existing table schema creates hidden metadata; Fields-menu visibility/last-editor persistence awaits real fields/rows/grid. |
| AT-303 | Preparatory evidence only | Typed literal defaults, checkbox forced NOT NULL/default and native range failures pass. No real required/unique/default Field UI or populated-row workflow yet (T1.14+). |
| AT-112/201/203/208..211/213 | Existing M1 evidence preserved | Final unchanged production-image account/navigation/sharing/admin suite passes. Full row/editor AT-204..207 and M3 restore remain pending. |
| AT-111/117/202/322/804 | Dependencies retained | Upload enforcement T2.2, real datetime/filter behavior, T1.14 field rename and T2.6 filter/API injection scenarios are not passed by codecs. |

Final **384 frontend tests / 20 files**, **397 locale keys**, required uncached
PG18, serial full check and production image **11 passed / 2 intentional
mobile-grid exclusions (3.5m)**. Initial gzip **185755 bytes**, no initial grid
imports. Default-parallel Suite cleanup failures and post-auth RSS above 150 MB
remain explicit; NFR-1 is not certified. An intermediate browser run failed the
existing desktop nav drag-order assertion; no tests/budgets were weakened.
T1.13 stays **In progress**. See the
[exact completion record](task-prompts/T1.13.md#implementation-and-handoff-2026-10-26).

### T1.14 field evidence (2026-10-26)

| Test | Status | Evidence / remaining boundary |
|---|---|---|
| AT-202 | Field rename pass (API + UI) | In the real production image, the owner renames a field to Chinese through the FieldPanel; the version bumps and no physical name appears in the schema DTO. The real-HTTP PG18 test renames to `金额 (含税)` (Chinese, space, parentheses) with `pg_column` and data unchanged. Writer/reader get 403, stranger/admin 404; browser read-level participants also see 403. Together with the T1.10/T1.11 table rename, M1 AT-202 passes. |
| AT-301 | Field creation scope passes; full pending | All 12 batch-1 kinds are created through the field service in Suite-owned PG18 tests with native types/descriptors checked. The real owner UI creates integer and single-select fields with options/defaults. Grid value entry, persistence after reload and information_schema checks need T1.15/T1.16. |
| AT-303 | Constraint API/UI scope passes; full pending | Required on NULL cells reports `ERR_REQUIRED_HAS_EMPTY{count}` and fills only with an explicit default; unique reports `ERR_UNIQUE_HAS_DUPLICATES{count}`; defaults are typed physical literals (PG18 Suite fixtures; the UI translates them). Populated-row browser workflows and "default applied to new rows" via the row API need T1.15/T1.16. |
| AT-304 | Partial | System fields allow rename/description only (real API 400 `system_field`). Fields menu/last-editor values need T1.15/T1.16. |
| AT-112/201/203/208..211/213 | Existing M1 evidence preserved | Unchanged account/navigation/sharing/admin scenarios pass in the same image run. |
| AT-111/117/204..207/322/804 | Dependencies retained | T2.2 uploads, real datetime/filter, rows/editors/shared views and T2.6 filter APIs remain pending. |

Default-parallel Suite cleanup failures and post-auth RSS above 150 MB
(desktop 232652800 bytes) remain explicit; NFR-1 is not certified. T1.14 stays
**In progress**. See the
[exact completion record](task-prompts/T1.14.md#implementation-and-handoff-2026-10-26).

### T1.15 row/query evidence (2026-10-26)

| Test | Status | Evidence / remaining boundary |
|---|---|---|
| AT-301 | API scope passes; full pending | All 12 batch-1 writable kinds round-trip through native PG18 columns using codecs; inserts apply typed physical defaults when omitted. Owner grid entry, reload persistence and information_schema browser checks remain T1.16. |
| AT-302 | API scope passes; full pending | Real row writes reject decimal excess nonzero precision, offset-free datetime input and unknown select labels with translated `ERR_VALIDATION`; the real HTTP route rejects an invalid integer. Visible cell error/no-save behavior remains T1.16. |
| AT-303 | API scope passes; full pending | Required/unique errors map safely; default-on-insert is verified through real PG18 columns. Populated-table field setup and browser new-row defaults remain T1.16. |
| AT-304 | Partial | Reads return read-only system fields; insert/patch author UUIDs and timestamps are maintained. Fields menu and correct UI last-editor values remain T1.16. |
| AT-312 | API performance evidence only; full pending | Suite-owned PG18 `generate_series(1,200000)` fixture: seed 579.6752 ms; 100-row page at offset 100,000 with count disabled 26.1817 ms; page plus exact count 20.394 ms. Not the 2-vCPU/4-GB target-host result or browser first-paint/scroll acceptance; production grid is T1.16. |
| HTTP security/rollback | API tests pass | Real protected HTTP tests cover query/CRUD, row permissions, bounded bodies, CSRF and foreign-Origin rejection; shared middleware deadline behavior is asserted in `auth_test.go`. PG18 tests cover atomic bulk rollback and field-DDL serialization. |

Focused command `go run .\tools\ptdev test -run 'Rows|Query' -v` passed
with required PG18 integration enabled (seven row/query top-level tests pass;
the opt-in performance test is skipped unless enabled). Full gate, browser,
resource and cleanup results are recorded in the
[T1.15 completion record](task-prompts/T1.15.md#implementation-and-handoff-2026-10-26).
T1.15 remains **In progress**: no full AT-301/302/303/304 or AT-312 claims are
made before their real persisted-grid scenarios.

### T1.17 copy/paste and fill-down evidence (2026-10-27)

| Test | Status | Evidence / remaining boundary |
|---|---|---|
| AT-302 | Pass for specified invalid integer/email grid behavior | Production-grid browser scenario shows both localized cell errors; invalid values are not persisted. Full AT-301 kind coverage remains pending. |
| AT-306 | Pass | The original production-image scenario verifies invalid-cell reporting and a 10-valid-cell bulk PATCH. T3.12 additionally copies a real Excel 3x4 range with Chinese and quoted multiline text: one PATCH contains 12 valid cells and all values persist; a second paste reports two invalid cells. |
| AT-307 | Fail | The prior nine-row prompt starts at the last existing row. In the literal T3.12 second-to-last-row scenario, the application offers eight rows rather than the specified nine. |
| AT-308 | Pass | T3.12 copies a date and select label from the grid into real Excel and verifies both resulting cells. |
| AT-309 | Pass | Ctrl+D fills the selected six-row vertical range; dragging the fill handle down extends persisted values to the target rows. Horizontal fill is not supported. |
| Mobile, locale/theme | Pass for T1.17 scope | Production-image mobile scenario confirms no grid is mounted and the localized fallback is visible. Desktop scenario exercises English/Chinese and light/dark. |
| AT-301/303/304/312 | Pending / partial preserved | The 12-kind create/type-catalog, populated required/unique/default, all system-field display and 200k target-host browser performance criteria are not claimed here. The VM is 8 CPU/2 GiB rather than the 2-vCPU/4-GB target. |

Focused production-image browser runs:
`go run .\tools\ptdev e2e -- e2e/grid.spec.ts --project=desktop-chromium`
**2 passed** (setup + grid, 25.5 s) and the corresponding
`--project=mobile-chromium` **2 passed** (17.0 s). The full production
`go run .\tools\ptdev e2e` run on the final code reported **8 passed / 5 failed /
2 skipped in 2.6 min**; both production-grid desktop and mobile scenarios pass
(10.5 s / 2.6 s). The remaining failures are admin/navigation diagnostics
(including an unexpected 404), mobile spike and two mobile shell scenarios;
two mobile-only spike scenarios are skipped by the existing configuration.
The earlier mobile notice CSS-order issue and stale browser selection were
fixed and verified. See the
[T1.17 completion record](task-prompts/T1.17.md#implementation-and-handoff-2026-10-27).

### T1.18 row expansion evidence (2026-10-27)

| Test | Status | Evidence / remaining boundary |
|---|---|---|
| AT-310 | Pass | Complete production-image suite: real grid row creation, expand icon and Space, all 12 writable codec editors with exact persisted values, full-field direct row URL/reload, hidden/system/deleted fields, invalid/deleted row/view/table, reader/writer/revoked ACL, delayed route/identity responses, en/zh-CN and light/dark. Desktop scenario 43.8 s; system fields/readers remain read-only by contract. |
| AT-306..309 / AT-302 | Prior evidence preserved | Existing real production-grid desktop scenario passes in 15.2 s; mobile fallback 2.9 s. Modal coverage does not replace the original clipboard/validation assertions. |
| Mobile, T1.18 boundary | Pass | New scenario's mobile fallback passes in 2.5 s with no RevoGrid/cards mounted. T1.19 still owns full-screen/mobile cards, FAB and AT-313. |
| AT-301/303/304, AT-204..207 | Pending full scenarios preserved | Modal kind saves/system renderers and row API denial are not the complete grid/type-catalog/constraints/sharing acceptance tests. |
| AT-312 / NFR-1 | Not certified | No real 200k-row production-browser first-paint/middle-page measurement on the documented 2-vCPU/4-GB host. Available VM remains 8 CPUs/2 GiB. |

`go run .\tools\ptdev e2e` final result: **11 passed / 4 failed / 2 skipped
in 4.0 min**. All T1.18 scenarios and existing production-grid scenarios pass.
Unrelated failures: mobile admin workspace empty-search expectation, mobile
synthetic-spike English notice, and two mobile shell locale/welcome assertions.
Two existing spike mobile exclusions remain intentional. Request/assertion/
scenario/global deadlines, one worker and zero retries are unchanged.
The neutral **404** for invalid GET row IDs is verified without bypassing ACL;
intentional browser HTTP failures are pre-registered.

Lower-layer checks: **441 frontend tests / 25 files**, lint/typecheck and
**528** matching locale keys pass; gzip initial shell **190337 bytes**.
Explicit required uncached PG18 row/query tests pass. Default full check
reproduces known parallel Suite DROP cleanup errors in codec/db/testdb;
command-scoped `GOFLAGS=-p=1` full check passes. Exact failures, versions,
resource snapshots and cleanup:
[T1.18 record](task-prompts/T1.18.md#implementation-and-handoff-2026-10-27).

### T1.19 mobile layout evidence (2026-10-27)

| Test | Status | Evidence / remaining boundary |
|---|---|---|
| AT-313 | Pass | Complete production-image suite at exactly 390x844: real cards (no RevoGrid), display plus four visible fields, hidden display-field query, required creation failure, twelve shared codec editors/exact API values, omitted physical defaults, FAB creation, tap/keyboard full-screen editing/save/direct-link/reload, 301-row 100-row paging/<=300 cached rows, confirmed deletion, readers/writers/revoked/hidden ACL, held real read/write/identity responses, focus/Escape/overflow, both locales and light/dark. Desktop-project emulated phone 15.6 s; mobile project 17.3 s. |
| AT-310 | Pass preserved | Existing complete desktop production-image scenario passes 48.3 s, without removing its exact editors, hidden/system/deleted fields, direct links, permissions or stale-response checks. Its mobile smoke now verifies real cards/no RevoGrid (3.0 s), superseding only the former no-cards fallback behavior. |
| AT-302 / AT-306..309 | Prior evidence preserved | Existing production-grid scenario passes 14.1 s desktop; mobile cards smoke 3.8 s. Original desktop validation/clipboard/fill assertions remain; card/editor evidence is not substituted for them. |
| AT-301/303/304, AT-204..207 | Pending full scenarios preserved | Mobile exact editors/defaults and scoped ACL checks are not complete grid/type-catalog/constraints/sharing acceptance. AT-203 restore remains M3; AT-111/322 await uploads, AT-117 real datetime/filter UI, AT-804 T2.6. |
| AT-312 / NFR-1 | Not certified | No real 200k-row production-browser first-paint/middle-page measurement on the documented 2-vCPU/4-GB host. Available VM stays 8 CPUs/2048 MiB; startup snapshots are not load/performance evidence. |

Final complete `go run .\tools\ptdev e2e`: **12 passed / 5 failed / 2 skipped
in 5.0 min**. All AT-313/310 and existing production-grid scenarios pass.
The four earlier mobile failures remain: admin workspace empty-search,
synthetic-spike English notice, shell English welcome and locale. The additional
desktop admin/navigation failure is an unregistered 404 console diagnostic
during ACL revocation (trace includes the existing view query); the preceding
complete run passed desktop admin/navigation but failed the isolated desktop
spike's cancellation-observation assertion. Neither desktop path was relaxed
or rewritten. Two existing mobile spike exclusions stay intentional.
One worker, zero retries, five-second assertions and D-112 budgets are unchanged.

Lower layers: **485 frontend tests / 26 files**; lint/strict types and **543**
matching locale keys; initial gzip **190849 bytes** (<600000); required explicit-
DSN uncached PG18 row/query gate all pass. Command-scoped serial full check passes.
Default-parallel full check reproduces Suite DROP cleanup failures in
codec/db/ddl/testdb, recorded separately without weaker deadlines. Unit/component
coverage includes late table/view/schema/permission/identity/unmount reads and
writes and exact 4096/4097-byte single PATCH and 524288/524289-byte atomic create
bounds. Versions, exact commands, intermediate failures, resource observations
and owned-only cleanup: [T1.19 record](task-prompts/T1.19.md#implementation-and-handoff-2026-10-27).

### T1.20 seed and performance workflow (2026-10-27; target acceptance pending)

The CLI-only seed and explicit production-image measurement workflow are
implemented under D-143/D-144. Focused Suite-owned PG18 tests verify the
ordinary non-admin owner's real metadata/schema/query/typed rows, all writable
codecs and exact bigint/decimal/microsecond values, physical omitted defaults,
required/unique behavior, explicit target/rerun/concurrent refusal, atomic
failed batches and bounded cancellation/session-lock cleanup. These are seed/
service tests, not full field/grid/sharing acceptance.

The benchmark must confirm 200000 physical rows before starting a fresh
authenticated production browser. It records navigation/request/header/body/
usable-render boundaries and verifies five exact global middle offsets,
100-row pages and <=300 loaded row models. It asserts the unchanged **<1500ms**
first usable paint and **<300ms per middle page** thresholds, not averaged or
API-only timers. The normal complete production-image suite remains separate.
Exact invocation results, limitations, versions and owned cleanup are recorded
in the [T1.20 implementation/handoff](task-prompts/T1.20.md).

Final local production-image workflow: **200000 rows** confirmed, seed
**1m0.962s**; fresh 1440x900 Chromium first usable paint **605.7ms**.
Five fresh global middle pages (100000, 100500, 99900, 101000, 100100) render in
**192.3/157.1/141.6/137.8/155.8ms**; request-to-body
**169.6/118.1/104.4/99.7/108.3ms**. Every exact first ID/100-row page,
fresh-request boundary and <=300 loaded-model assertion passes. A preceding
strict run rendered two pages in **379.3/350.3ms**, failing the unchanged
300ms budget; that failure is not erased by the final pass. Both are local
observations, not target-host acceptance. The final standalone JSON report,
navigation/40 asset timings, seed/host JSON and seven resource samples persist
outside Playwright's cleared browser-output subdirectory (ignored, not tracked).

Complete regression `ptdev e2e`: **13 passed / 4 failed / 2 intentional skips
in 4.3m**. AT-310 desktop **44.3s**, AT-313 **14.3s/13.7s**, existing production
grid **12.4s/3.0s** pass. Four earlier mobile empty-search/English notice/
welcome/locale failures remain. Desktop nav and spike cancellation pass in this
run; their earlier intermittent failures remain recorded, not allowlisted.
Final scoped-serial full check passes (**485** frontend tests, strict types/
**543** locale keys and required uncached PG18); initial gzip **190849 bytes**.
Default-parallel cleanup failures in codec/db/devseed/testdb are reported
separately, without relaxed deadlines; exact failed DB IDs were checked absent.

**AT-312 and T1.20 remain pending/In progress.** No authorized 2-vCPU/4-GB
Ubuntu 24.04 host was supplied; the available Podman VM remains 8 CPUs /
2048 MiB, unchanged. Local browser timing/resource observations, even passing
ones, do not certify target-host NFR-2 or normal-load app-RSS NFR-1. Preserve
all prior AT-310/302/306..309/313 evidence and reported failures. Full
AT-301/303/304 and AT-204..207 remain pending unless their complete linked
scenarios pass; AT-203 restore is M3, AT-111/322 await T2.2 uploads, AT-117
awaits datetime/filter UI, AT-804 awaits T2.6. No promotion from seed/API tests.

## AT-3xx Fields & Grid (M1, M2)

- **AT-301 — Basic field types** · M1 · Owner adds each M1 type (text, long text, integer, decimal, float, checkbox, date, datetime, single select, multi select, email, URL); values entered via grid persist after reload and are stored in the expected PG type (API check via information_schema).
- **AT-302 — Validation** · M1 · Entering "abc" in integer, "x@" in email → cell shows error and value is not saved.
- **AT-303 — Required / unique / default** · M1 · Setting `required` on a field with empty cells reports count and requires a default or abort; unique rejects duplicates with translated error; default applied to new rows.
- **AT-304 — System fields** · M1 · New views hide id/created/updated fields; "Fields" menu can show them; values correct (updated_by = last editor).
- **AT-305 — Keyboard navigation** · M1 · Arrows/Tab/Enter/Esc/F2/Delete/Ctrl+Home/End behave per 12-frontend.md §4.
- **AT-306 — Copy/paste from Excel** · M1 · Copying a 3×4 range from Excel/Google Sheets (incl. a quoted multi-line cell and Chinese text) and pasting at a cell updates 12 cells in one request; invalid cells reported, others saved.
  - **T3.12 evidence: Pass.** A headed Chrome/real Excel check saves 12 valid cells in one PATCH, including quoted multiline and Chinese text; a second paste reports invalid cells. The earlier production-image scenario verifies that valid cells still save alongside invalid cells.
- **AT-307 — Paste beyond end adds rows** · M1 · Pasting 10 rows at the second-to-last row prompts "Add 9 rows?"; confirming creates them.
  - **T3.12 evidence: Fail.** The literal second-to-last-row scenario offers eight additional rows, not nine. The older nine-row result started at the last row; the criterion has not been changed.
- **AT-308 — Copy to Excel** · M1 · Copying a range from the grid and pasting into Excel produces the same cells (select labels, dates formatted).
  - **T3.12 evidence: Pass.** A headed Chrome check pastes a grid date and select label into real Excel and verifies the resulting cell values.
- **AT-309 — Fill-down** · M1 · Select cell + 5 below, Ctrl+D → all 6 equal top value; drag fill handle down 3 rows → same.
- **AT-310 — Row expand** · M1 · Space or expand icon opens modal with all fields editable; deep link URL `/t/:id/v/:id/r/:rowId` opens it directly.
- **AT-311 — Column ops** · M1 · Resize, reorder, hide column, freeze first 2 columns → persisted in view for all users (write) / local only (read).
- **AT-312 — Large table scroll** · M1 · With 200k seeded rows, first paint < 1.5 s, scrolling to the middle loads rows < 300 ms per page (NFR-2).
- **AT-313 — Mobile card list** · M1 · On a 390×844 viewport, table shows card list; tap opens row editor; edit and save works; "+" adds a row.
- **AT-320 — Remaining field types** · M2 · time, duration, currency, percent, rating, phone, uuid, json, inet, cidr, text list, integer list, user, multi-user, auto-number, varchar(n) round-trip correctly; user field picker lists active users; auto-number read-only and increasing.
- **AT-321 — Attachments** · M2 · Upload image & PDF to attachment cell (drag-drop and picker) → thumbnail for image, download works; user without access to table gets 404 for file URL.
- **AT-322 — Upload limit** · M2 · File larger than admin limit is rejected with translated error.
- **AT-323 — Type change dry-run** · M2 · Text field with values ["1","2","x"] → convert to integer: dry-run shows 1 failing with sample "x"; "Abort" changes nothing; "Convert, set to NULL" results in [1,2,NULL]. Evidence: PG18/API/FieldPanel tests and production-browser E2E; `ptdev check` passes and field acceptance is 5/5.
- **AT-324 — Select choice rename** · M2 · Renaming choice "Open"→"Active" updates all cells and filters using it. Evidence: PG18 verifies cells, defaults, and shared/personal filters/highlights; production-browser E2E verifies the active saved filter; field acceptance is 5/5.
- **AT-325 — Field soft delete** · M2/M3 · Deleting a field hides it everywhere; restoring from Trash brings values back.
  T3.6: **Pass** — desktop/mobile browser deletion hides the field and restores its original value after explicit optional confirmation; PG18 verifies retained configuration, defaults, required-state conflicts and relation storage. [Record](task-prompts/T3.6.md#record-2026-11-01).
- **AT-326 — Currency default** · M2 · New currency field defaults to CNY (¥ formatting); owner switches it to USD and values display as $; admin changes default currency to EUR → new currency fields default to EUR, existing fields unchanged.

## AT-4xx Relations & Computed (M2)

- **AT-401 — 1:N link** · M2 · Owner creates link Tasks→Projects (many tasks per project) → both tables get link fields; linking a task to project P shows task in P's link cell and vice versa.
- **AT-402 — 1:1 link** · M2 · Linking row A1 to B1 then A2 to B1 prompts "B1 is already linked to A1, replace?" and enforces uniqueness.
- **AT-403 — M:N link** · M2 · Tasks↔Tags many-to-many; one task with 3 tags, one tag on 2 tasks; both sides consistent.
- **AT-404 — Self link** · M2 · Employees "Manager" (N:1 self) works; reverse field "Reports" lists subordinates.
- **AT-405 — On-delete behaviours** · M2 · set_null: deleting project clears tasks' link; cascade: deleting project deletes its tasks; restrict: deleting project with tasks fails with translated error.
- **AT-406 — Same-book only** · M2 · Link target picker lists only tables in the same book (self included).
- **AT-407 — Lookup** · M2 · Tasks.lookup(Project.Owner) shows owner of linked project; updates when project owner changes. **Evidence: PASS** — PG18 `TestLookupComputedValueUpdatesWithRelatedRow` and browser `lookups-rollups.spec.ts`.
- **AT-408 — Rollup** · M2 · Projects.rollup(sum Tasks.Hours) correct; count, avg, min, max, earliest, latest, concat also correct. **Evidence: PASS** — PG18 `TestRollupComputedAggregates` and browser `lookups-rollups.spec.ts` cover all eight aggregates.
- **AT-409 — Formula** · M2 · `{Price} * {Qty}`, `IF({Status}="Done","✓","")`, `DATETIME_DIFF(TODAY(), {Due}, "days")`, `CONCAT({First}," ",{Last})` compute correctly; invalid formula shows error with position; renaming a referenced field keeps the formula working. **Evidence: PASS** — PG18 `TestFormulaQueryValuesStableReferencesAndDependencyValidation` and `TestFormulaFunctionSQLExecutesOnPostgres`, API `TestFormulaCheckAPIRequiresOwnerAndReportsPosition`, and browser `formulas.spec.ts` cover the four expressions, positioned errors, and rename stability.
- **AT-410 — Computed fields filter/sort** · M2 · Filter and sort on a formula and a rollup work. **Evidence: PASS** — PG18 `TestFormulaQueryValuesStableReferencesAndDependencyValidation` and `TestRollupComputedAggregates` verify formula and rollup filters; `TestQueryRowsSortsFormulaValues` and `TestQueryRowsSortsRollupValues` verify their sorting.
- **AT-411 — Cross-permission masking** · M2 · bob has read on Tasks but none on Projects → link cell shows project names only (no open), lookup cells show 🔒, filter on lookup not offered. **Evidence: PASS** — PG18 `TestComputedValuesMaskedWithoutTargetRead` and browser `lookups-rollups.spec.ts` verify masking and retained link labels; computed filtering is unavailable until T2.6.

## AT-5xx Views (M2)

- **AT-501 — Nested filter** · M2 · Filter `Status is any of [Open, Blocked] AND (Due before today OR Owner is me)` returns correct rows; depth > 3 not allowed by UI. **Evidence: PASS** — `FilterBuilder.test.ts` verifies nested group limits; production-browser `filters.spec.ts` verifies the nested result and transient request filter (3/3 fast E2E tests pass).
- **AT-502 — Type-aware operators** · M2 · Operators offered match 08-query-engine.md §2 for each field type; relative dates (`today`, `past 7 days`) evaluate in user timezone. **Evidence: PASS** — `FilterBuilder.test.ts` checks server-declared operators; PG18 relative-date boundary tests and production-browser `filters.spec.ts` verify actor-timezone evaluation (3/3 fast E2E tests pass).
- **AT-503 — Multi sort** · M2 · Sort by Status asc, Due desc produces correct order; stable paging (no duplicates/missing rows when scrolling). **Evidence: PASS** — PG18 `TestQueryRowsMultiSortKeepsNullsLastAndPagesDeterministically`, grid `DataGrid.test.ts`, and fast production-browser `sort.spec.ts` (2 passed, 1 intentional mobile skip) verify priority controls, NULLS LAST, row-id tie-breaking, complete API paging, and desktop scroll fetching.
- **AT-504 — Group 3 levels** · M2 · Group by Project › Status › Owner shows nested headers with counts and sum aggregates; collapse/expand works; `(empty)` group present. **Evidence: PASS** — PG18 `TestGroupedRowsHTTPAPIUsesFilteredBucketsAndGroupPaths` covers filtered buckets, nested counts/sums, empty and sorted multi-select keys, and group-path rows; `grouping.spec.ts` verifies the three-level production grid, collapse/expand, and sticky mobile headers (3/3 fast E2E tests). `ptdev check` passes.
- **AT-505 — Highlight** · M2 · Rule 1 (row red if Overdue), rule 2 (cell amber on Due if within 3 days) → colors correct; reordering rules changes outcome per first-match; colors readable in dark mode. **Evidence: PASS** — PG18 `TestQueryRowsReturnsOrderedRowAndCellHighlights`, `viewColumns.test.ts`, and production-browser `highlight.spec.ts` verify persisted rule editing, independent row/cell first-match ordering, keyboard reorder, and ≥4.5:1 active amber text contrast in light/dark themes (2 fast E2E passed, 1 intentional mobile skip); `ptdev check` passes.
- **AT-506 — Shared vs personal views** · M2 · bob (write) creates personal view → alice can't see it; bob creates shared view → alice sees it; read-only carol can create personal views but not shared. **Evidence: PASS** — PG18 `TestViewManagementHTTPAT506AT507` and fast production-browser `views.spec.ts` (2 passed, 1 intentional mobile skip) verify private views, reader personal creation, shared visibility/denial, duplication, rename, reorder, and delete.
- **AT-507 — Locked view** · M2 · Owner locks shared view → bob (write) can't save changes (may apply temporary overrides with "save as personal view" prompt). **Evidence: PASS** — PG18 `TestViewManagementHTTPAT506AT507` and production-browser `views.spec.ts` verify locking and denied changes. T3.12 additionally verifies that a writer saves a temporary filter as a private view without changing the locked shared view.
- **AT-508 — Concurrent view edit** · M2 · Two users edit the same view config → second save gets conflict, reloads, and re-applies change. **Evidence: PASS** — PG18 `TestViewManagementHTTPAT508ConcurrentSaves` and production-browser `views.spec.ts` verify one 200/one 409 for same-version edits, then reload/reapply and confirm the second edit is retained.
- **AT-509 — Quick search** · M2 · Searching "acme" finds rows matching in any visible text field, incl. Chinese text search "项目". **Evidence: PASS** — PG18 `TestQuickSearchAndFooterAggregatesUseVisibleFilteredRows` and production-browser `search-aggregates.spec.ts` verify visible-field English/Chinese search, hidden-field exclusion, and transient filtering (2 passed, 1 intentional mobile skip); `ptdev check` passes.
- **AT-510 — Footer aggregates** · M2 · Sum/avg/count/empty/unique values correct and respect filters. **Evidence: PASS** — PG18 `TestQuickSearchAndFooterAggregatesUseVisibleFilteredRows` and production-browser `search-aggregates.spec.ts` verify filtered and search-scoped sum/avg/count/empty/unique values, empty averages, and UI configuration (2 passed, 1 intentional mobile skip); `ptdev check` passes.
- **AT-511 — Form view** · M2 · Owner builds form with custom labels/help/required; bob (write) submits → row created; carol (read) sees form disabled; form works on mobile viewport. **Evidence: PASS** — Go API/structure tests, `FormBuilder.test.ts` and `FormView.test.ts` (4/4), `ptdev check`, and production-browser `form.spec.ts` (3/3 fast E2E) verify configuration persistence/reload, required validation, owner/writer row creation, read-only submission denial, and overflow-free 390 px layout.
- **AT-512 — Filter/group performance** · M2 · On 200k rows: filter + sort + group each respond < 1 s (NFR-3). **Auto-index correctness PASS; target-host performance pending** — PG18 integration verifies strict >10k eligibility, btree/trigram selection, restart/idempotent scheduling, metadata preservation, and retry after metadata failure (T2.14). T2.15's authenticated loopback HTTP first/warm samples were filter 42.8/26.6 ms, sort 65.5/59.4 ms, group 81.2/71.2 ms, all locally <1 s. The 8-vCPU/2-GiB VM is not the 2-vCPU/4-GB target; TLS, reverse-proxy and browser overhead are excluded. Fixture, parameters and plans: [T2.15 record](task-prompts/T2.15.md#record-2026-10-31).

## AT-6xx Collaboration (M3)

- **AT-601 — Realtime rows** · M3 · alice and bob view T; bob edits a cell → alice sees new value < 1 s; bob adds/deletes rows → alice's grid updates. **Evidence: PASS** — `realtime.spec.ts` measures sub-second propagation and verifies create/delete on desktop and mobile; PG18 tests cover transaction rollback, ACLs and the 501-row bulk fallback.
- **AT-602 — LWW on same cell** · M3 · alice and bob edit same cell; last saved wins; both end with same value; editor shows "changed by bob" badge while alice is editing. **Evidence: PASS** — `realtime.spec.ts` checks the remote-editor badge, retained draft, last-save convergence and Escape; mobile row-dialog editing is also covered.
- **AT-603 — Schema realtime** · M3 · alice adds field → bob's grid shows new column without reload. **Evidence: PASS** — `realtime.spec.ts` verifies the new column without navigation; PG18 tests cover dependent schema notifications.
- **AT-604 — Presence** · M3 · bob opens T → alice sees bob's avatar; bob focuses a cell → alice sees outline with bob's name; bob leaves → avatar gone < 60 s. **Evidence: PASS** — Fast `realtime.spec.ts` verifies entry, matching outline/name, view isolation and departure on desktop and mobile; PG18 `TestRealtimePresenceLifecycleAndACL` covers table access, personal-view spoofing, multiple tabs and disconnect, while hub tests cover throttling and the expiry deadline.
- **AT-605 — Reconnect** · M3 · Disconnecting network for 30 s and reconnecting resyncs the visible window (changes made meanwhile appear). **Evidence: PASS** — `realtime.spec.ts` holds a browser offline for 30 s, changes a row, and verifies visible-window resync after reconnect; Go tests cover listener recovery.
- **AT-606 — Comments & mentions** · M3 · alice comments "@bob please check" on a row → bob gets bell notification live; clicking opens row modal at comment; mention list only offers users with access. **Evidence: PASS** — `comments.spec.ts` verifies the live unsubscribed recipient, filtered picker, exact focused deep link, unread state, and row counts on desktop and mobile; PG18 tests cover permissions, rollback, revocation and pagination.
- **AT-607 — Row history** · M3 · Edits, paste, link/unlink appear in the row's History tab with user, time, old → new values; history older than retention deleted by job *(API, time-travel)*. **Evidence: PASS** — Fast `history.spec.ts` verifies edits, desktop paste (mobile bulk equivalent), bidirectional link/unlink and the English/Chinese timeline on desktop and mobile; PG18 tests verify the strict retention cutoff, worker, permissions and deleted-row history; component tests cover author/time grouping and pagination.
- **AT-608 — Undo/redo** · M3 · Edit cell, paste range, fill-down, delete 3 rows, link change → Ctrl+Z undoes each in reverse order (deleted rows return with same ids, links, comments); Ctrl+Y redoes. **Evidence: PASS** — Fast `undo.spec.ts` verifies the complete reverse/redo sequence plus row insertion and view config; PG18 tests verify exact IDs, system fields, links, comments, attachment references, cascades and authorization. Store tests cover 50-action bounds, grouped actions, redo invalidation and bounded replay.
- **AT-609 — Undo conflict prompt** · M3 · alice edits cell, bob changes it, alice presses Ctrl+Z → confirm dialog "changed by bob since". **Evidence: PASS** — Fast `undo.spec.ts` verifies Bob's named prompt, cancellation, confirmation and redo on desktop/mobile; PG18 and store tests verify locked comparisons, intervening races and compensation. `ptdev check` passes.
- **AT-610 — Trash** · M3 · Deleted table/view/field/book restorable by owner within retention; purge job removes physical objects after retention *(API, time-travel)*.
  T3.6: **Pass** — browser restores each resource kind; required PG18 time-travel tests cover strict cutoff, nested protection, physical schema/table/column removal, attachment cleanup and failure retry. [Record](task-prompts/T3.6.md#record-2026-11-01).

## AT-7xx Import / Export / Backup (M3)

- **AT-701 — CSV export** · M3 · Export view → CSV opens correctly in Excel with Chinese text (BOM), respecting view filter/sort/visible fields.
  - **T3.7 evidence (2026-11-01): Pass.** Desktop and mobile browser exports verify filtering, sorting, grouping and visible fields; PostgreSQL tests verify quoting and actor timezone. Microsoft Excel opened an actual BOM export with Chinese text, quotation marks and an embedded comma.
- **AT-702 — XLSX package round-trip** · M3 · Export table T (all types, views, a formula, a link) as PandaTable package → import as new table T' → T' has identical fields (types, options, constraints), views, and data; link becomes re-linked (if target exists and user chooses) or text.
  - **T3.7 evidence (2026-11-01): Partial — export artifact only.** Browser download and PostgreSQL package tests verify the very-hidden manifest, field/view metadata, typed cells, list separators, formulas, links and access masking. Microsoft Excel opened a generated package with Chinese text, a numeric cell and its very-hidden manifest intact.
  - **T3.8 evidence (2026-11-01): Partial.** Desktop/mobile browser and PG18 tests verify representative data, field options/constraints, shared-view configuration, formulas, lookups, rollups, re-linking and text fallback; unavailable computed dependencies and view settings are preserved and reported for repair. PG18 exercises every writable type with an empty attachment. Nonempty attachment names cannot recover file identities/content; distinct timestamps can collide as Excel serials; view locks are absent from the manifest. The literal all-types, data-identical contract remains blocked (D-190).
- **AT-703 — Plain import new table** · M3 · Import a 10k-row CSV (GBK encoded) → wizard guesses types (integer, date, single select, text); user adjusts; table created; invalid cells reported.
  - **T3.8 evidence (2026-11-01): Pass.** Desktop/mobile browser tests import 10,000 GBK rows, verify guesses and adjusted mapping, and download the invalid-cell report; PG18 tests verify NULLs, source-order issue limits and history.
- **AT-704 — Import append** · M3 · Import XLSX into existing table with column mapping → rows appended; write users allowed, read users not.
  - **T3.8 evidence (2026-11-01): Pass.** Desktop/mobile browser tests map a plain XLSX as a writer, deny a reader and verify package auto-mapping; PG18 tests cover package append, reader denial, unchanged views and partially committed batches.
- **AT-705 — Large import memory** · M3 · Import 200k rows CSV: completes, app RSS stays < 300 MB.
  - **T3.8 evidence (2026-11-01): Pass.** The focused browser gate imported 200,000 rows and measured the app process lifetime high-water RSS at 268,263,424 bytes, below 300,000,000 bytes.
- **AT-706 — Backup create/download/delete** · M3 · Admin creates backup → status running → done; zip contains `db.dump`, `files/`, `backup.json`; download works; delete removes file.
  - **T3.9 evidence (2026-11-01): Pass.** Desktop and mobile browser tests create a real backup, inspect the downloaded ZIP and metadata, and verify deletion of both the archive and record; PG18 tests cover progress, failures, recovery and authorization.
- **AT-707 — Restore** · M3 *(manual)* · Restore backup on a fresh host via CLI → all users, workspaces, data, attachments present.
  - **T3.9 automated evidence (2026-11-01): Pass; manual acceptance pending.** The real CLI restored a real PG18 dump and attachments into a disposable database/data directory, recovering users, workspace and row data; tests also verify rollback and slow-restore handling. A separate fresh-host manual restore has not been performed.
- **AT-708 — Backup disk guard** · M3 *(API)* · Insufficient free disk → backup refused with translated error.
  - **T3.9 evidence (2026-11-01): Pass.** Desktop and mobile browser tests use a sparse file in the owned disposable data volume to trigger the real guard, verify English and Chinese errors, and confirm no backup record is created.

## AT-8xx Non-functional (all)

- **AT-801 — Memory budget** · M3 · Under a 10-user simulated load (k6/script: editing, scrolling, realtime) for 15 min, app RSS < 150 MB, host memory < 3.2 GB used.
  - **T3.11 evidence (2026-11-01): Not certified.** The driver validates duration, per-user activity, fresh samples and strict thresholds; a real two-user browser smoke covers scrolling, edits, peer events and both metrics. No 15-minute run was made on the non-equivalent local VM. The earlier above-target app RSS observation remains open.
- **AT-802 — i18n completeness** · M0+ *(local gate)* · `en.json` and `zh-CN.json` key sets identical; no hard-coded strings (lint rule).
- **AT-803 — Security headers & CSRF** · M1 *(API)* · Requests without `X-PT-CSRF` or with foreign Origin rejected; cookies HttpOnly+Secure+SameSite=Lax; CSP, nosniff present.
  - **T3.11 evidence (2026-11-01): Pass rechecked.** Real HTTP/PG18 tests cover failed CSRF, malformed/unsupported login requests and untrusted forwarding, plus sniffed uploads, metadata mismatch and sandbox headers; browser and router tests cover the stricter SPA CSP. Existing D-073 cookie/Origin evidence remains valid.
- **AT-804 — SQL injection** · M1/M2 *(API)* · Field/table names and filter values containing `"; DROP TABLE` etc. are stored/handled as data.
  - **T3.11 evidence (2026-11-01): Partial, unchanged.** The new read-only doctor checks metadata/catalog drift, not injection scenarios; the remaining table/field-name and filter-value API evidence is still required.
  - **T3.12 evidence: Pass.** Required PG18 HTTP test `TestSQLInjectionStringsRemainDataThroughHTTPAT804` stores injection-shaped table and field names, filters literal values with equality and containment, and verifies the users table and session remain intact.
- **AT-805 — Browser matrix** · M3 *(manual)* · Smoke test on Chrome, Edge, Firefox, Safari desktop, iOS Safari, Android Chrome.
  - **T3.11 evidence (2026-11-01): Pending.** No manual six-browser/device matrix was performed. Chromium desktop/mobile emulation is not a substitute.
  - **T3.12 evidence: Partial.** Installed Chrome and Edge pass the desktop smoke; Playwright Firefox and WebKit engines also pass, but do not certify stock Firefox/Safari or physical iOS/Android devices.
- **AT-806 — Responsive** · M3 · All main pages usable at 390 px width (no horizontal page scroll except grid area).
  - **T3.11 evidence (2026-11-01): Partial automated coverage.** Desktop and mobile Chromium navigate 16 main routes at 390 px in both languages, check headings and page overflow, and verify the mobile row list is visible. This does not exhaustively exercise every page's controls.

## AT-9xx Post-v1 (M4, M5)

- **AT-901 — Kanban** · M4 · Kanban by single select; drag card between columns updates field; realtime.
- **AT-902 — Gallery** · M4 · Gallery cards with cover attachment and selected fields.
- **AT-903 — Calendar** · M5 · Calendar by date/datetime field (month/week), drag to reschedule.
- **AT-904 — SMTP** · M5 · Admin configures SMTP and sends test email; user self-service password reset by email; mention emails (opt-in).

## M1 phase review evidence (2026-10-27)

Whole-M1 review of the real gates, not a task completion. No acceptance result
was promoted and no prior evidence was removed.

| Test | Status | Evidence / remaining requirement |
|---|---|---|
| AT-006 (default-parallel gate) | Improved, still M0-partial | `go run .\tools\ptdev check` previously failed with Suite DROP cleanup errors in `internal/codec`, `internal/devseed` and `internal/testdb`; it now passes with default parallelism (lint 27.6 s, test 1m18.8 s, build 14.8 s) after D-145, so `GOFLAGS=-p=1` is no longer required. Linux bash/Docker remains outstanding. |
| Full production-image suite | 17 passed / 0 failed / 2 intentional skips (5.0m) | Previously 13 passed / 4 failed. The four mobile failures had one root cause: a globally-empty workspace-inventory assumption aborted the run before the caller restored instance-wide settings, leaving `defaultLocale=zh-CN` (D-146). All AT-104..110/112/201/208/213, AT-310, AT-313, AT-302 and AT-306..309 scenarios pass unchanged. |
| AT-305 / AT-311 | Not implemented | No `PATCH /views/{id}` route exists, so column width/order/hidden/frozen cannot persist to the view; the grid has RevoGrid local resize and a session-local field toggle only. Keyboard navigation relies on RevoGrid defaults with no Delete/Esc/F2/Ctrl+Home/End coverage or E2E. T1.16 remains In progress; tracked as [T1.21](task-prompts/T1.21.md). |
| AT-102 / AT-116 / AT-117 | Partial, unchanged | `PATCH /auth/me` exists, but there is no `/profile` page or route, so display name, locale, theme and timezone cannot be changed in a browser and the alice/bob timezone scenario cannot run. No browser scenario asserts the translated wrong-password error or post-logout back-button behavior. Tracked as [T1.22](task-prompts/T1.22.md). |
| AT-301/302/303/304 | Partial, preserved | Existing grid, modal and card evidence stands; the complete 12-type creation with `information_schema` verification, validation, required/unique/default and system-field/`updated_by` scenarios are still missing. Tracked as [T1.23](task-prompts/T1.23.md). |
| AT-111 / AT-203 / AT-804 | Deferred by design | Upload enforcement is T2.2, restore is T3.6 and filter-value injection is T2.6; T1.8 and T1.9 stay In progress until those milestones close them. |
| AT-312 / NFR-1 / NFR-2 | Not certified, unchanged | Still no authorized 2-vCPU/4-GB Ubuntu host; no new performance measurement was taken in this review. |
| AT-001 / AT-005 | Partial, unchanged | No Linux/Docker runtime or approved Caddy destination was used. |

Exact commands, root-cause analysis and cleanup:
[progress.md, M1 review](archive/progress-m0-m1.md#m1-phase-review-and-gate-repair-2026-10-27).

## T1.21 completed column/keyboard evidence (2026-10-28)

This superseded the earlier AT-305/AT-311 "not implemented" verdict at the
time. See the T3.12 release audit below for the reopened task status.

| Test | Status | Evidence / remaining boundary |
|---|---|---|
| AT-305 | Pass | Real production Grid: all four arrows, Tab/Shift+Tab, Enter edit/commit+down, Esc/F2, Delete/Backspace codec clear, Ctrl+C/V/X/D/A, all four Shift+arrow directions, Space checkbox/row, Home/End, Ctrl+Home/End and exact viewport PageUp/Down. Paired en/light and zh-CN/dark run every key; earlier focused four-combination evidence is retained. A 401-row fixture forces uncached first/last jumps, exact row IDs and bounded source/cache; no viewport index is treated as row identity. Read-only/system clears are skipped. |
| AT-311 | Pass | Real pointer header resize/drag plus accessible width/order/hide/show/freeze controls; first two data columns frozen (utility column excluded), persisted config/readback/reload for owner and another user promoted to write. Reader changes width/order/visibility/freeze locally with zero column PATCHes and no server version/config change, then reload restores the shared config. A real stale If-Match produces the pre-registered 409, authoritative reload and intent rebase preserving another writer's width. Both locales and light/dark are exercised; locked/personal/hidden/permission/concurrent-save cases also pass required PG18 and focused component tests. |
| AT-302 / AT-306..309 | Browser assertions preserved | Unfiltered production suite: original validation, exact TSV/invalid-cell atomic patch/tail insert/readback/fill-handle assertions pass **14.7 s desktop / 3.0 s mobile cards smoke**. Fixtures explicitly select an unfrozen viewport through the public API; no original assertion or budget was removed. See the T3.12 audit for real Excel checks and the AT-307 mismatch. |
| AT-310 / AT-313 | Pass preserved | Complete original modal/card, exact codecs/defaults/deep links/ACL/stale-response scenarios pass **47.1 s / 2.6 s** and **15.4 s / 16.7 s**, respectively. The mobile helper drains its own downgrade-triggered queries before another revocation, without ignoring 404s or loosening diagnostics. |
| Full gates | Pass | Default-parallel `ptdev check`: **504 frontend tests / 27 files**, required Go/PG18, lint/typecheck/**553** locale keys and build; **2m0.738 s**. Unfiltered production `ptdev e2e`: **19 passed / 0 failed / 2 intentional spike skips in 5.6m**, unchanged 150 s scenario/360 s global/5 s assertion budgets, one worker and zero retries. Initial shell **191255 bytes gzip** with no grid/registry import. |
| Remaining acceptance | Unchanged | T1.22 owns AT-102/116/M1 AT-117, T1.23 full AT-301..304; uploads AT-111/322 T2.2, full AT-804 T2.6, restore AT-203 M3. AT-312/NFR-1/2 remain uncertified without the authorized 2-vCPU/4-GB host; no target performance claim. |

All failed iterations, exact versions/commands, startup-only observations,
owned cleanup and handoff are retained in the
[T1.21 record](task-prompts/T1.21.md#implementation-and-verification-record-2026-10-27).

## T1.22 implementation checkpoint (2026-10-28)

No acceptance verdict is promoted by this implementation checkpoint. The
authenticated self-profile route and page, focused unit tests, and a
production-browser scenario for AT-102, AT-116, and the M1 portion of AT-117
are implemented. The focused profile/router tests pass (6/6); lint, strict
typecheck, locale-key validation (563 matching keys), and the production
bundle gate pass. Playwright discovers the desktop and 390px mobile scenarios,
but neither has run against the production image. AT-102, AT-116, and AT-117
therefore remain Partial until their complete browser scenarios pass. The
Podman VM was visible but its socket refused connections; `pt-dev-db` state
could not be inspected, so no database-dependent gate or E2E stack was started.
No shared container, image, or volume was changed. See the
[T1.22 implementation checkpoint](task-prompts/T1.22.md#implementation-checkpoint-2026-10-28)
for exact commands, versions, limitations, and handoff.

## T1.23 field acceptance evidence (2026-10-28)

This supersedes only the earlier AT-301/303/304 "pending/partial" verdicts.
Prior evidence above remains. **T1.23, T1.13, T1.14 and T1.15 are Done.**

| Test | Status | Evidence / remaining boundary |
|---|---|---|
| AT-301 | Pass | Production image, desktop, real login and real workspace/book/table creation. The owner adds the 12 M1 kinds through the real Field panel (Name is the table's default text field; an extra `text` field is also added). Values are typed into the real localized grid editors, committed with Enter, read back through the view query API and again after a page reload, all exact (`42`, `"99.95"`, `4.5`, `true`, ISO date, UTC instant, select label, ordered multi-select array, email, URL). The PostgreSQL storage type of each kind (`character varying`/`text`/`integer`/`numeric`/`double precision`/`boolean`/`date`/`timestamp with time zone`/`ARRAY`) and checkbox `NOT NULL DEFAULT false` are checked against `information_schema.columns` in the required uncached PG18 test `TestFieldAPIAcceptancePassAT301To304`. No browser endpoint exposes the catalog, so that check is API-level only (D-157). |
| AT-302 | Pass preserved and re-run | Same scenario: `abc` in the integer cell and `x@` in the email cell show the localized cell errors and leave the stored values unchanged. The original T1.16 validation scenario still passes. |
| AT-303 | Pass | Real Field panel on a populated table: setting `required` with two empty cells shows the translated `ERR_REQUIRED_HAS_EMPTY` count of 2 (pre-registered 409). Retrying with an explicit default and `fillEmpty` succeeds. Setting `unique` with one duplicate shows `ERR_UNIQUE_HAS_DUPLICATES` with count 1 (409); after the duplicate is resolved, unique is accepted. A duplicate grid write shows the translated `ERR_UNIQUE_CONFLICT`. A newly added row receives the physical default. The same preflights pass in the PG18 test. |
| AT-304 | Pass | A new table's default view hides `id`, `created_at`, `created_by`, `updated_at` and `updated_by`; the Fields control reveals all five. `created_by` and `updated_by` equal the creator. An admin-created second account (forced password change, write grant) edits a cell through its own real grid; afterwards `updated_by` is that account and `created_by` is still the creator, in the API and after an owner reload. PG18 verifies the system column types and the same authorship transitions. |
| Full gates | Pass | Default-parallel `go run .\tools\ptdev check` (no `GOFLAGS`): 507 frontend tests, required Go/PG18, lint/strict typecheck/**563** keys, bundle gate, **1m47s**; initial shell **191482 bytes gzip** (< 600000). Complete production `go run .\tools\ptdev e2e`: **23 passed / 0 failed / 2 intentional spike skips, 7.6 min** Playwright time with unchanged 150 s per-scenario, one worker, zero retries and five-second assertions. Field scenarios took 11.7 s / 6.1 s / 7.1 s. Earlier evidence (AT-202, AT-302, AT-305..311, AT-313, admin/auth/sharing/profile) passes unchanged. |
| Remaining | Unchanged | AT-312/NFR-1/2 remain uncertified without the authorized host; AT-111/322 await T2.2, full AT-804 awaits T2.6, AT-203 restore is M3. |

Exact commands, failed attempts, cleanup and handoff are in the
[T1.23 record](task-prompts/T1.23.md#implementation-and-verification-record-2026-10-28).

## T2.1 field evidence (2026-10-29)

| Test | Status | Evidence / remaining boundary |
|---|---|---|
| AT-320 | Pass (desktop) | The real Field panel creates each batch-2 kind; grid values save and persist after reload, the user picker exposes only active users, and autonumber is read-only/increasing. In the fast browser gate, 6 scenarios pass and the 2 mobile-only exclusions are intentional. |
| AT-326 | Pass (desktop) | CNY formats and persists on the initial field; changing it to USD updates that field, while an admin default change to EUR affects new fields only. The test restores the instance setting. |

## T2.3 relation evidence (2026-10-29)

| Test | Status | Evidence / remaining boundary |
|---|---|---|
| AT-401 | Pass (desktop) | Owner creates both 1:N fields; the grid picker links a task to a project, and the reverse row value contains the task label. |
| AT-402 | Pass (desktop) | 1:1 uniqueness returns the expected conflict, asks before replacement, and leaves the old row unlinked while linking the new row. |
| AT-403 | Pass (desktop) | M:N links support multiple tags per task and multiple tasks per tag; reverse values agree. |
| AT-404 | Pass (desktop) | Employee self-relation links a report to its manager; the reverse Reports field returns the subordinate. |
| AT-405 | Pass | PG18 verifies set_null, cascade, and restrict. Browser coverage verifies set_null and cascade results and the translated restrict error through the grid. |
| AT-406 | Pass (desktop) | Relation target selector includes same-book tables and the current table, and excludes tables from another book. |

## T3.12 release audit (2026-11-01)

The complete production-image milestone gate failed with 25 passed, 4 failed
and 53 not run at the unchanged 600-second global timeout. It is not replaced
by focused fast runs; the affected tasks are reopened. These verdicts supersede
conflicting historical summaries above.

| Test | Status | Evidence / remaining boundary |
|---|---|---|
| AT-004 | Pass | The milestone image is linux/amd64, 171,849,346 bytes, UID/GID 10001:10001, with an executable healthcheck and PostgreSQL 18 `pg_dump`. |
| AT-110 | Reopened | The production dashboard scenario expected migration 2 but observed 5. Its test now derives the latest migration from the actual files; focused dashboard assertions passed before later sharing failures. |
| AT-102/116 | Pass (focused) | The profile browser scenario verifies username/email login, translated errors, logout/back navigation, display-name changes and password replacement. It passed in the fast runtime image; the milestone run timed out before it. |
| AT-117 | Pass (focused) | The profile browser scenario verifies Shanghai/London datetime display and default-timezone changes. Required PG18 HTTP evidence verifies `is today` for users on different calendar dates. The milestone profile scenario did not run. |
| AT-209 / sharing | Reopened | Focused admin scenarios failed on a missing-principal alert and an expected revocation 404. Subsequent browser failures prevented a conclusive retest; the existing sharing test is unchanged. |
| AT-305/311 | Reopened | The production stale-version scenario did not observe its expected 409. A test-route experiment failed and was removed; the original conflict scenario remains unresolved. |
| AT-306/308 | Pass | A headed Chrome/real Excel check verifies both directions, including the 12-cell single request, invalid-cell reporting, date and select label. |
| AT-307 | Fail | Pasting ten rows at the second-to-last row offers eight additional rows, not the nine required by the literal criterion. |
| AT-409 | Reopened | The production formula scenario failed after another scenario changed the shared user timezone. Test cleanup and actor-timezone assertions were corrected; both focused formula projects pass, but the milestone gate remains failed. |
| AT-507 | Pass | A writer's temporary override of a locked shared view saves privately without changing that shared view. |
| AT-804 | Pass | The required PG18 HTTP injection test verifies literal table/field names and filter values and confirms the users table and session remain intact. |
| AT-805/806 | Partial | Chrome and Edge desktop smoke and Playwright Firefox/WebKit engines pass; stock Firefox/Safari, physical iOS/Android, and exhaustive responsive controls remain unverified. |
| AT-001/005/006, AT-312/512/801, AT-707 | Partial | Linux/Docker, authorized Ubuntu/Caddy deployment, target-host performance and fresh-host restore checks remain unavailable. |
| AT-702 | Partial | Package round-trip fidelity remains blocked by attachment identity/content, possible timestamp collisions and missing view locks. |

The production grid scenario also reported an unexpected browser-console 409;
both focused grid projects passed, but its original cause was not established.
See the [T3.12 record](task-prompts/T3.12.md#record-2026-11-01) for focused
reruns, gate results and release blockers.
