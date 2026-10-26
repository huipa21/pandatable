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
[progress.md, T0.6](progress.md#t06-verification-evidence-2026-10-24).

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
[progress.md, T0.7](progress.md#t07-verification-evidence-2026-10-24).
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
conclusion: [progress](progress.md#t08-verification-evidence-2026-10-24) and
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
[progress](progress.md#t11-verification-evidence-2026-10-24) and the
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
[progress](progress.md#t12-backend-verification-and-handoff-2026-10-25).

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
[progress](progress.md#t13-setup-verification-and-handoff-2026-10-25) and the
[T1.3 completion record](task-prompts/T1.3.md#implementation-and-handoff-2026-10-25).

### T1.5 admin users evidence (2026-10-26)

| Test | Status | Evidence |
|---|---|---|
| AT-104 | Pass | No registration UI; real non-admin create request gets 403 `ERR_PERMISSION_DENIED` (unauthenticated 401). |
| AT-105 | Pass | Desktop/mobile production UI creates normalized users with temporary passwords; login/deep routes force `/change-password`; successful change revokes sessions and requires re-login. |
| AT-106 | Pass | Admin reset through the real UI revokes the existing browser session (me=401); reset password login requires change. PG18 verifies all-session atomicity/rollback and old-password login race. |
| AT-107 | Pass | Disable logs the open tab out on its next request; login is denied; retained identity is visible and re-enables. PG18 verifies retained workspace/creator/grants and immediate cache invalidation (no TTL wait). |
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
| AT-111 | Partial | Real admin settings persistence and production-image desktop/mobile header/title reload for admin, ordinary user and unauthenticated browser. Public DTO exposes only appName/defaultLocale; explicit local language choices remain intact. Full test includes upload enforcement (AT-322), which awaits T2.2. |
| AT-112 | Partial | Required Suite-owned PG18 inventory/owner/orphan/size/paging and owner-only grants versus admin 404/ACL denial pass. Production browser verifies the real empty read-only inventory, navigation/guards and no resource links. No workspace creation/navigation/table APIs exist; SQL fixtures and unit-populated cards are not the full alice-create/sidebar/table-GET UI scenario. Await T1.10/T1.11. |
| AT-117 | Partial | Real settings changes reach existing NULL-timezone auth/profile readers and new-user defaults; explicit timezone/locale choices survive. Datetime display, relative filters and profile UI remain deferred. |
| AT-322 | Pending | Upload limit persists with strict integer/MiB bounds, but no attachment upload consumer exists. No mock/reject-all endpoint or upload pass is claimed. |
| AT-113..115 | Pending, unchanged | Read-only orphan flag follows active direct user owners; rescue/transfer/hard deletion remain M3. |

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
[progress](progress.md#t14-acl-verification-and-handoff-2026-10-26).

- **AT-201 — Create hierarchy** · M1 · alice creates workspace W, book B, table T → sidebar shows W ▸ B ▸ T; T has a default Grid view and a primary "Name" field; DB has schema `b_*` and table `t_*`.
- **AT-202 — Rename is metadata only** · M1 *(API)* · Renaming T/field to Chinese names with spaces works; physical names unchanged.
- **AT-203 — Delete goes to trash** · M1/M3 · alice deletes T → disappears from sidebar; (M3) appears in W's Trash and can be restored with data intact.
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

## AT-3xx Fields & Grid (M1, M2)

- **AT-301 — Basic field types** · M1 · Owner adds each M1 type (text, long text, integer, decimal, float, checkbox, date, datetime, single select, multi select, email, URL); values entered via grid persist after reload and are stored in the expected PG type (API check via information_schema).
- **AT-302 — Validation** · M1 · Entering "abc" in integer, "x@" in email → cell shows error and value is not saved.
- **AT-303 — Required / unique / default** · M1 · Setting `required` on a field with empty cells reports count and requires a default or abort; unique rejects duplicates with translated error; default applied to new rows.
- **AT-304 — System fields** · M1 · New views hide id/created/updated fields; "Fields" menu can show them; values correct (updated_by = last editor).
- **AT-305 — Keyboard navigation** · M1 · Arrows/Tab/Enter/Esc/F2/Delete/Ctrl+Home/End behave per 12-frontend.md §4.
- **AT-306 — Copy/paste from Excel** · M1 · Copying a 3×4 range from Excel/Google Sheets (incl. a quoted multi-line cell and Chinese text) and pasting at a cell updates 12 cells in one request; invalid cells reported, others saved.
- **AT-307 — Paste beyond end adds rows** · M1 · Pasting 10 rows at the second-to-last row prompts "Add 9 rows?"; confirming creates them.
- **AT-308 — Copy to Excel** · M1 · Copying a range from the grid and pasting into Excel produces the same cells (select labels, dates formatted).
- **AT-309 — Fill-down** · M1 · Select cell + 5 below, Ctrl+D → all 6 equal top value; drag fill handle down 3 rows → same.
- **AT-310 — Row expand** · M1 · Space or expand icon opens modal with all fields editable; deep link URL `/t/:id/v/:id/r/:rowId` opens it directly.
- **AT-311 — Column ops** · M1 · Resize, reorder, hide column, freeze first 2 columns → persisted in view for all users (write) / local only (read).
- **AT-312 — Large table scroll** · M1 · With 200k seeded rows, first paint < 1.5 s, scrolling to the middle loads rows < 300 ms per page (NFR-2).
- **AT-313 — Mobile card list** · M1 · On a 390×844 viewport, table shows card list; tap opens row editor; edit and save works; "+" adds a row.
- **AT-320 — Remaining field types** · M2 · time, duration, currency, percent, rating, phone, uuid, json, inet, cidr, text list, integer list, user, multi-user, auto-number, varchar(n) round-trip correctly; user field picker lists active users; auto-number read-only and increasing.
- **AT-321 — Attachments** · M2 · Upload image & PDF to attachment cell (drag-drop and picker) → thumbnail for image, download works; user without access to table gets 404 for file URL.
- **AT-322 — Upload limit** · M2 · File larger than admin limit is rejected with translated error.
- **AT-323 — Type change dry-run** · M2 · Text field with values ["1","2","x"] → convert to integer: dry-run shows 1 failing with sample "x"; "Abort" changes nothing; "Convert, set to NULL" results in [1,2,NULL].
- **AT-324 — Select choice rename** · M2 · Renaming choice "Open"→"Active" updates all cells and filters using it.
- **AT-325 — Field soft delete** · M2/M3 · Deleting a field hides it everywhere; restoring from Trash brings values back.
- **AT-326 — Currency default** · M2 · New currency field defaults to CNY (¥ formatting); owner switches it to USD and values display as $; admin changes default currency to EUR → new currency fields default to EUR, existing fields unchanged.

## AT-4xx Relations & Computed (M2)

- **AT-401 — 1:N link** · M2 · Owner creates link Tasks→Projects (many tasks per project) → both tables get link fields; linking a task to project P shows task in P's link cell and vice versa.
- **AT-402 — 1:1 link** · M2 · Linking row A1 to B1 then A2 to B1 prompts "B1 is already linked to A1, replace?" and enforces uniqueness.
- **AT-403 — M:N link** · M2 · Tasks↔Tags many-to-many; one task with 3 tags, one tag on 2 tasks; both sides consistent.
- **AT-404 — Self link** · M2 · Employees "Manager" (N:1 self) works; reverse field "Reports" lists subordinates.
- **AT-405 — On-delete behaviours** · M2 · set_null: deleting project clears tasks' link; cascade: deleting project deletes its tasks; restrict: deleting project with tasks fails with translated error.
- **AT-406 — Same-book only** · M2 · Link target picker lists only tables in the same book (self included).
- **AT-407 — Lookup** · M2 · Tasks.lookup(Project.Owner) shows owner of linked project; updates when project owner changes.
- **AT-408 — Rollup** · M2 · Projects.rollup(sum Tasks.Hours) correct; count, avg, min, max, earliest, latest, concat also correct.
- **AT-409 — Formula** · M2 · `{Price} * {Qty}`, `IF({Status}="Done","✓","")`, `DATETIME_DIFF(TODAY(), {Due}, "days")`, `CONCAT({First}," ",{Last})` compute correctly; invalid formula shows error with position; renaming a referenced field keeps the formula working.
- **AT-410 — Computed fields filter/sort** · M2 · Filter and sort on a formula and a rollup work.
- **AT-411 — Cross-permission masking** · M2 · bob has read on Tasks but none on Projects → link cell shows project names only (no open), lookup cells show 🔒, filter on lookup not offered.

## AT-5xx Views (M2)

- **AT-501 — Nested filter** · M2 · Filter `Status is any of [Open, Blocked] AND (Due before today OR Owner is me)` returns correct rows; depth > 3 not allowed by UI.
- **AT-502 — Type-aware operators** · M2 · Operators offered match 08-query-engine.md §2 for each field type; relative dates (`today`, `past 7 days`) evaluate in user timezone.
- **AT-503 — Multi sort** · M2 · Sort by Status asc, Due desc produces correct order; stable paging (no duplicates/missing rows when scrolling).
- **AT-504 — Group 3 levels** · M2 · Group by Project › Status › Owner shows nested headers with counts and sum aggregates; collapse/expand works; `(empty)` group present.
- **AT-505 — Highlight** · M2 · Rule 1 (row red if Overdue), rule 2 (cell amber on Due if within 3 days) → colors correct; reordering rules changes outcome per first-match; colors readable in dark mode.
- **AT-506 — Shared vs personal views** · M2 · bob (write) creates personal view → alice can't see it; bob creates shared view → alice sees it; read-only carol can create personal views but not shared.
- **AT-507 — Locked view** · M2 · Owner locks shared view → bob (write) can't save changes (may apply temporary overrides with "save as personal view" prompt).
- **AT-508 — Concurrent view edit** · M2 · Two users edit the same view config → second save gets conflict, reloads, and re-applies change.
- **AT-509 — Quick search** · M2 · Searching "acme" finds rows matching in any visible text field, incl. Chinese text search "项目".
- **AT-510 — Footer aggregates** · M2 · Sum/avg/count/empty/unique values correct and respect filters.
- **AT-511 — Form view** · M2 · Owner builds form with custom labels/help/required; bob (write) submits → row created; carol (read) sees form disabled; form works on mobile viewport.
- **AT-512 — Filter/group performance** · M2 · On 200k rows: filter + sort + group each respond < 1 s (NFR-3).

## AT-6xx Collaboration (M3)

- **AT-601 — Realtime rows** · M3 · alice and bob view T; bob edits a cell → alice sees new value < 1 s; bob adds/deletes rows → alice's grid updates.
- **AT-602 — LWW on same cell** · M3 · alice and bob edit same cell; last saved wins; both end with same value; editor shows "changed by bob" badge while alice is editing.
- **AT-603 — Schema realtime** · M3 · alice adds field → bob's grid shows new column without reload.
- **AT-604 — Presence** · M3 · bob opens T → alice sees bob's avatar; bob focuses a cell → alice sees outline with bob's name; bob leaves → avatar gone < 60 s.
- **AT-605 — Reconnect** · M3 · Disconnecting network for 30 s and reconnecting resyncs the visible window (changes made meanwhile appear).
- **AT-606 — Comments & mentions** · M3 · alice comments "@bob please check" on a row → bob gets bell notification live; clicking opens row modal at comment; mention list only offers users with access.
- **AT-607 — Row history** · M3 · Edits, paste, link/unlink appear in the row's History tab with user, time, old → new values; history older than retention deleted by job *(API, time-travel)*.
- **AT-608 — Undo/redo** · M3 · Edit cell, paste range, fill-down, delete 3 rows, link change → Ctrl+Z undoes each in reverse order (deleted rows return with same ids, links, comments); Ctrl+Y redoes.
- **AT-609 — Undo conflict prompt** · M3 · alice edits cell, bob changes it, alice presses Ctrl+Z → confirm dialog "changed by bob since".
- **AT-610 — Trash** · M3 · Deleted table/view/field/book restorable by owner within retention; purge job removes physical objects after retention *(API, time-travel)*.

## AT-7xx Import / Export / Backup (M3)

- **AT-701 — CSV export** · M3 · Export view → CSV opens correctly in Excel with Chinese text (BOM), respecting view filter/sort/visible fields.
- **AT-702 — XLSX package round-trip** · M3 · Export table T (all types, views, a formula, a link) as PandaTable package → import as new table T' → T' has identical fields (types, options, constraints), views, and data; link becomes re-linked (if target exists and user chooses) or text.
- **AT-703 — Plain import new table** · M3 · Import a 10k-row CSV (GBK encoded) → wizard guesses types (integer, date, single select, text); user adjusts; table created; invalid cells reported.
- **AT-704 — Import append** · M3 · Import XLSX into existing table with column mapping → rows appended; write users allowed, read users not.
- **AT-705 — Large import memory** · M3 · Import 200k rows CSV: completes, app RSS stays < 300 MB.
- **AT-706 — Backup create/download/delete** · M3 · Admin creates backup → status running → done; zip contains `db.dump`, `files/`, `backup.json`; download works; delete removes file.
- **AT-707 — Restore** · M3 *(manual)* · Restore backup on a fresh host via CLI → all users, workspaces, data, attachments present.
- **AT-708 — Backup disk guard** · M3 *(API)* · Insufficient free disk → backup refused with translated error.

## AT-8xx Non-functional (all)

- **AT-801 — Memory budget** · M3 · Under a 10-user simulated load (k6/script: editing, scrolling, realtime) for 15 min, app RSS < 150 MB, host memory < 3.2 GB used.
- **AT-802 — i18n completeness** · M0+ *(local gate)* · `en.json` and `zh-CN.json` key sets identical; no hard-coded strings (lint rule).
- **AT-803 — Security headers & CSRF** · M1 *(API)* · Requests without `X-PT-CSRF` or with foreign Origin rejected; cookies HttpOnly+Secure+SameSite=Lax; CSP, nosniff present.
- **AT-804 — SQL injection** · M1/M2 *(API)* · Field/table names and filter values containing `"; DROP TABLE` etc. are stored/handled as data.
- **AT-805 — Browser matrix** · M3 *(manual)* · Smoke test on Chrome, Edge, Firefox, Safari desktop, iOS Safari, Android Chrome.
- **AT-806 — Responsive** · M3 · All main pages usable at 390 px width (no horizontal page scroll except grid area).

## AT-9xx Post-v1 (M4, M5)

- **AT-901 — Kanban** · M4 · Kanban by single select; drag card between columns updates field; realtime.
- **AT-902 — Gallery** · M4 · Gallery cards with cover attachment and selected fields.
- **AT-903 — Calendar** · M5 · Calendar by date/datetime field (month/week), drag to reschedule.
- **AT-904 — SMTP** · M5 · Admin configures SMTP and sends test email; user self-service password reset by email; mention emails (opt-in).
