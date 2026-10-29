# Archived per-task verification notes (M0–M1)

Moved verbatim from [14-dev-build-release.md](../14-dev-build-release.md) on 2026-10-29 (D-158). Historical focused-gate commands per task; search by task ID when needed.

### Default-parallel gate (M1 review, D-145/D-146)

Since 2026-10-27 both gates pass with plain `go run ./tools/ptdev check` and
`go run ./tools/ptdev e2e`; the `GOFLAGS=-p=1` workaround required by earlier
task records is no longer needed and must not be used to hide a new failure.
The Suite teardown now has a 60 s cleanup budget separate from its unchanged
3 s connect attempt and retries the forced drop only on `55006` (D-145); the
E2E settings helper restores instance-wide settings on failure and asserts the
real shared workspace inventory instead of a globally empty one (D-146).
Earlier task records keep their original `GOFLAGS=-p=1` evidence as written.

T1.21's focused required PG18 gate (explicit `PT_TEST_DATABASE_URL` and
`PT_TEST_DATABASE_REQUIRED=1`) is
`go test .\internal\structure .\internal\httpapi -run 'ViewColumns' -count=1`.
It uses the existing package Suites and covers strict column payloads,
live/deleted/unknown/duplicate IDs, widths/frozen bounds, missing/stale/valid
If-Match, ACL/personal/locked views, transport, preservation/cleanup and
concurrent saves with exactly one version winner. Frontend focused tests:
`pnpm --dir web exec vitest run src/components/grid/DataGrid.test.ts src/components/grid/viewColumns.test.ts src/components/grid/remoteWindow.test.ts`.
The production-image AT-305/311 scenario is
`go run .\tools\ptdev e2e -- e2e/grid-columns-keyboard.spec.ts --project=desktop-chromium`.
It exercises every documented key in both locales and themes, real header
resize/drag, accessible hide/reorder/freeze, reload/shared writer persistence,
reader local-only state and a real stale-version reconciliation. Existing
clipboard fixtures explicitly select an unfrozen viewport through the public
view API, preserving their complete original range/value assertions now that
the default stored frozen count is honored. Run lint/bundle, focused PG18,
then complete default-parallel check and the unfiltered production E2E suite;
no scenario/global/assertion deadline, retries, worker or resource budget changes.
The keyboard fixture has 401 real rows, so Ctrl+End must load an uncached page;
PageUp/Down asserts the exact viewport row count and all four Shift+arrow
directions are covered. D-151 keeps logical source length stable during jumps.
The mobile ACL helper waits for its own downgrade-triggered schema/card reload
and pending queries before final grant removal, avoiding a stray read across
the second revocation without ignoring any HTTP diagnostic.

T1.13's focused lower-layer gate is `go test ./internal/codec -count=1`
with explicit `PT_TEST_DATABASE_URL` and `PT_TEST_DATABASE_REQUIRED=1`, plus
`pnpm --dir web exec vitest run src/fields/fields.test.ts src/components/fields/fields.test.ts`.
Shared vectors cover options, exact wire values, paste/copy and capabilities.
Suite-owned PG18 tests create columns through actual typed DDL descriptors,
check native catalog types/forced constraints, round-trip typed pgx parameters
and exercise defaults/range failures/system columns. They are not full
AT-301/302/304 evidence without real field/row/grid APIs/UI.
Record default-parallel Suite cleanup failures separately from command-scoped
`GOFLAGS=-p=1` results; never relax the helper's assertions or deadlines.

T1.14's focused gate is
`go test ./internal/structure ./internal/httpapi -run 'Field|CleanDescription|Choice|PGType' -count=1`
(same explicit DSN/required flags) plus
`pnpm --dir web exec vitest run src/components/fields/FieldPanel.test.ts src/pages/ResourcePage.test.ts`.
The production `ptdev e2e` exercises real owner Field UI/API flows from
`web/e2e/fields.ts` inside the existing navigation scenario.

T1.15's required row/query integration gate uses the same explicit PG18 DSN
and `PT_TEST_DATABASE_REQUIRED=1`:
`go test ./internal/structure ./internal/httpapi -run 'Rows|Query' -count=1`.
It covers batch-1 codecs against real physical columns, permissions,
constraint/default handling, atomic rollback and schema DDL serialization,
plus real HTTP CSRF/body/response behavior. The opt-in
`PT_RUN_ROW_PERF=1 go test ./internal/structure -run '^TestRowQueryPerformance200k$' -count=1 -v`
seeds a Suite-owned table with `generate_series(1,200000)` for API paging/count
timings. It covers all 12 writable batch-1 kinds through real columns,
precision/offset/select rejection, defaults, required/unique errors, owner/
writer/reader/stranger/admin permissions, atomic rollback, DDL serialization,
and actual HTTP query/CRUD, body bounds, CSRF and foreign-Origin rejection.
The common protected-handler test verifies deadlines. This is a documented
performance fixture, not browser acceptance for AT-312. Squirrel v1.5.4 is
the convention-required SQL builder; identifiers remain validated metadata
and values remain bound parameters.

T1.17 frontend/browser verification:

```powershell
pnpm -C web test
pnpm -C web lint
pnpm -C web build
go run .\tools\ptdev e2e -- e2e/grid.spec.ts --project=desktop-chromium
go run .\tools\ptdev e2e -- e2e/grid.spec.ts --project=mobile-chromium
```

The real production-image grid scenario verifies AT-306..309, including the
one-request valid-cell bulk patch, invalid-cell reporting, confirmed appended
rows, clipboard formatting, Ctrl+D and vertical fill handle. Unit/API tests
alone do not establish these acceptance results. The complete `ptdev e2e`
suite is still required and unrelated failures must be recorded separately.
AT-312 requires browser measurements on the documented 2-vCPU/4-GB target;
Podman VM timings or API-only page timings are not substitutes.

T1.18 focused verification:

```powershell
pnpm -C web exec vitest run RowExpandDialog DataGrid ResourcePage fields.test
pnpm -C web lint
pnpm -C web build
go run .\tools\ptdev e2e -- row-expand --project=desktop-chromium
go run .\tools\ptdev e2e -- row-expand --project=mobile-chromium
```

The production-image AT-310 scenario creates real metadata and a row via the
grid, opens from both the icon and a genuinely focused grid cell's Space key,
edits every writable kind, checks exact API values, reloads direct row URLs,
and exercises hidden/system/deleted fields, invalid/deleted resources,
read/write/revoked access, late responses, locales/themes and mobile fallback.
Fixtures never bypass row ACL. Failed HTTP responses must be registered before
navigation/mutation via `expectHTTPFailure` (exactly once each). Only a
background revalidation that races a deliberate revocation may use the per-page
`allowHTTPFailure`, which tolerates zero or more matching failures on that one
page; there is no suite-wide tolerance (D-157). Logins go through the shared
`paceLogin` helper so the per-IP login limit holds across scenarios.
Component tests also release
responses after route/schema/identity changes even when fetch ignores abort.
The same uncached required PG18 row/query command documented above and both
default/command-scoped serial-package full checks remain mandatory, as does
the complete production-image `go run .\tools\ptdev e2e`. Do not infer AT-312
or unrelated full field/sharing acceptance from the row dialog.

T1.19 focused verification:

```powershell
pnpm -C web exec vitest run MobileCardList RowExpandDialog ResourcePage fields.test
pnpm -C web lint
pnpm -C web build
go run .\tools\ptdev e2e -- mobile-rows --project=mobile-chromium
```

The AT-313 production-image scenario explicitly uses **390x844**, real accounts,
field/row APIs and existing codecs. It covers twelve editors, exact values,
required creation failure, omitted physical defaults, save/deep-link/reload,
hidden display-field retrieval, 100-row paging and <=300 cached rows, keyboard
card activation/Escape/focus return, full-screen layout/overflow, both locales/
themes, reader/writer/revoked/hidden access and real held read/write responses.
Component tests additionally exercise route/schema/identity/unmount guards even
when providers ignore abort. The complete production-image E2E suite and both
default and command-scoped serial full checks remain required; failed expected
HTTP requests are registered before the operation. Existing mobile fallback
assertions now check the real cards, not the superseded notice, without removing
the desktop AT-310/302/306..309 assertions or earlier evidence. No D-112 budget
or retry/worker/assertion/performance limit changes. These checks do not certify
AT-312 or NFR-1 on the required 2-vCPU/4-GB target.

### T1.2 backend auth checks

[Auth tests](../internal/auth/service_test.go) and
[HTTP tests](../internal/httpapi/auth_integration_test.go) each use their own
Suite-owned PG18 database and bounded fixture cleanup. They insert real PHC
hashes only in those owned databases, never in the developer DB. The real gate
requires live PG18 and uncached execution; an ordinary skip is not acceptance.

```text
go run ./tools/ptdev test -run "TestSession|TestLogin|TestPassword|TestProfile|TestConcurrent|TestAuth|TestForced|TestPHC|TestRateLimit|TestLimitCapacity|TestCSRF|TestTrustedProxy|TestCookie" -v -count=1
go test ./internal/auth -run "^$" -bench BenchmarkPasswordHash -benchtime=3x -benchmem -cpu=2
```

Rate/session tests inject a clock rather than sleeping for deadlines. A
password-change race test waits for actual PostgreSQL lock evidence before
committing the changed hash and revocation. Session validators race a revoke
and verify the row cannot reappear. Expiry tests deliberately extend the stored
expiry to prove idle/absolute limits are checked independently.

Static auth queries are generated locally using pinned `sqlc` **1.31.1**:
`sqlc generate` (also generates `internal/acl/aclsql`, `internal/admin/adminsql`
and `internal/structure/structuresql` from the other `sqlc.yaml` entries). Commit its Go bindings; generation must produce no diff on
rerun. [catalog.sql](../internal/auth/catalog.sql) declares the PG timezone
catalog only for the offline generator; no runtime SQL or migration executes it.
The application image uses existing Go source allowlists and does not include
the generator. Runtime additions are only `x/crypto` (argon2id) and `x/sys`.

By user-confirmed scope, no login/profile frontend or auth browser fixture is
shipped in T1.2. Existing `ptdev e2e` remains the real shell/spike gate; it is
**not** browser acceptance for AT-102/116. API coverage and stored profile
timezone/default checks do not complete full AT-117's real datetime/filter/
admin-default scenarios. T1.2 remains in progress until linked acceptance passes.

### T1.22 self-profile and auth browser acceptance

The `ProfilePage` unit tests cover changed-only `/auth/me` patches, timezone
catalog/default handling (`null`, not an empty string), shared locale/theme
preference persistence, load retry and expired-session redirection. The
production-image `profile.spec.ts` scenario owns its uniquely suffixed users
and restores the previous instance-wide admin settings in `finally`. It covers
username/email login, translated bad credentials, logout/back navigation,
display-name/password persistence and the M1 real datetime/timezone path; it
must use real APIs and the real grid/row editor. Preserve the existing
150-second scenario, 600-second global, five-second assertion, one-worker and
zero-retry limits set by D-155. T1.22 is not accepted unless this scenario
passes in the complete production-image run; the M2 relative-date filter half
of AT-117 is not part of this test. The complete scenario runs once in desktop
Chromium; inside it, a separate 390x844 touch-enabled Chromium context checks
the mobile profile route, native controls, password link and horizontal overflow.
The mobile project does not replay the full profile scenario.

### T1.3 setup checks

The auth and HTTP setup tests use separate Suite-owned PG18 databases and clean
their first-admin/settings fixtures with bounded cleanup. The required local
gate supplies `PT_TEST_DATABASE_REQUIRED=1`; setup tests cover token expiry and
restart rotation, normalized identity/password boundaries, rollback, existing
active/disabled users, session storage, CSRF/cookie policy and two independently
initialized services racing through the PostgreSQL setup lock.

```text
go run ./tools/ptdev test -run "TestSetup|TestConcurrentSetup|TestSetupHTTP|TestServePostgreSQLRestarts" -v -count=1
go run ./tools/ptdev e2e
```

The production-image AT-101 test obtains its token only from the uniquely owned
app container's startup log. The harness sends it to Playwright through a
private child environment variable and redacts it from retained Compose logs.
The E2E run also prints a read-only app-container memory/CPU snapshot. No
production seed endpoint or real developer account is used.

### T1.4 ACL and grants checks

`internal/acl` tests are Suite-owned PG18 tests (own `TestMain`); unit tests
need no database. Resolver matrix, group union/membership, disabled/trashed,
path-only visibility, TTL bound, cache hit counts, validation, scope-owner and
last-owner rules, rollback, concurrent owner removals and the D-070 hard-delete
lock interleavings are in `internal/acl`; AT-204..212 at the HTTP surface
(including `authHTTP.require` probe routes) are in `internal/httpapi`.

```text
go run ./tools/ptdev test -run "Test(Effective|Membership|Disabled|PathOnly|Cache|Put|Inherited|List|Scope|Delete|LastOwner|Orphaned|Concurrent|Grant|Failed|Resolver|Access|Level|Resource)" -v -count=1
```

T1.5 supplies real admin user seeding; workspace fixtures still require T1.10.
These ACL checks remain API/HTTP-level evidence.

### T1.5 admin users checks

`internal/admin` owns a separate Suite-managed PG18 database. Tests cover
normalization/legacy ambiguous identities, literal search/pagination, typed
validation/authorization, temporary-password/self-reset flows, all-session
revocation, status/role rules, last-active-admin concurrency, rollback on failed
session revocation, real row-lock waits and caller reauthorization. A cached
last workspace owner's access disappears immediately on disable and returns
on enable; workspace/authorship rows remain. `internal/httpapi` covers the
admin namespace, strict bodies/query parameters, CSRF and DTO/error contracts.
Static queries use the third sqlc 1.31.1 entry (`internal/admin/adminsql`);
all three generated packages must stay byte-identical on regeneration.

```text
go test ./internal/admin ./internal/auth ./internal/httpapi -count=3
pnpm --dir web lint
pnpm --dir web test
go run ./tools/ptdev check
go run ./tools/ptdev e2e
```

The focused Go command requires explicit `PT_TEST_DATABASE_URL` and
`PT_TEST_DATABASE_REQUIRED=1`; `ptdev check` supplies them automatically.
Browser AT-104..108 uses only logged-token setup, actual login/admin APIs and
the production UI. One scenario per desktop/mobile creates normalized users,
edits roles in both locales, searches, changes a forced password, tests
non-admin API/deep-route denial, resets/revokes, disables an open session on
its next request and re-enables the retained identity. Ten total login attempts
fit the unchanged ten/minute/IP limit; the tests do not bypass/reset rate state.
Expected 401 `/auth/me` probes are matched narrowly to Chromium's HTTP console
diagnostics, alongside AT-101's explicitly asserted negative setup responses;
all other warnings/errors/CSP violations still fail the suite.

### T1.6 admin groups checks

`internal/admin/groups_test.go` reuses the package Suite, fixture and real ACL
resolver. Coverage includes Unicode/control bounds, trim/case-sensitive names,
literal search/paging/counts, duplicate/missing/disabled membership, retained
disabled members, atomic deletion/grant cleanup, rollback, all auth classes,
caller reauthorization and concurrent duplicate adds/status changes. Primed ACL
entries gain and lose access immediately, including group deletion. A deferred
constraint-trigger failure proves commit errors still invalidate the cache.
Deterministic gates exercise the real grant service before/after deletion and
when the grant creator is the concurrently added member. D-087 records the
deadlock exposed and corrected by that evidence; no migration is needed.

```text
go test ./internal/admin ./internal/httpapi -run "TestGroup|TestAdminGroups|TestMembershipCommitError" -count=3
go test ./internal/admin ./internal/auth ./internal/acl ./internal/httpapi -count=3
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate
pnpm --dir web lint
pnpm --dir web test
go run ./tools/ptdev check
go run ./tools/ptdev e2e
```

All Go integration commands require explicit `PT_TEST_DATABASE_URL` and
`PT_TEST_DATABASE_REQUIRED=1`; tests remain uncached and use only Suite-owned
disposable databases. Verify pinned regeneration byte identity for all generated
packages. If the version query is unreachable but pinned source is cached, run
that exact module's `cmd/sqlc` with an absolute `-f` repository config and record
the fallback command, never silently change versions or proxy configuration.

The existing `admin-users.spec.ts` scenario now covers AT-104..109 on desktop
and mobile with the same sign-ins: Bob/Carol seed only through the real admin
API, `team-a` creation, both membership-detail views, add/remove, en/zh-CN
edit, exact duplicate-name error, empty search/membership, deletion cancel and
confirmed cleanup. Group/user-detail screenshots are retained under ignored
results. No auth bypass or extra login budget is introduced.

### T1.7 system dashboard checks

`internal/admin/system*_test.go` covers CPU percent/counter reset/availability,
exact five-second cache expiry, coalescing, waiter/lifecycle cancellation,
hourly directory cache, zero/missing/error/bounds policy, rooted symlink skipping,
cgroup namespace/ancestor/unlimited/unsupported parsing, real PG18 size/settings/
versions/migration queries and secret-free error/DTO output. The Linux-only
filesystem test exercises unreadable directories as non-root and special files
without opening them. Windows may skip symlink creation when the account lacks
that privilege; run the compiled tests non-root in a uniquely owned Linux
container as well, not as a substitute for full Linux/Docker acceptance.

```text
go test ./internal/admin ./internal/httpapi -run "TestSystem|TestAdminSystem" -count=3
go test ./internal/admin ./internal/auth ./internal/acl ./internal/httpapi -count=3
go test ./internal/admin -run "^$" -bench BenchmarkSystem -benchtime=1s
pnpm --dir web exec vitest run src/composables/useSystemMetrics.test.ts src/pages/AdminDashboardPage.test.ts src/router/guards.test.ts
go run ./tools/ptdev check
go run ./tools/ptdev e2e -- admin-users.spec.ts
go run ./tools/ptdev e2e
```

All PG commands still require explicit URL/required=1 and Suite-owned databases.
Static system queries use the existing admin sqlc entry with a generator-only
catalog declaration, never an applied migration. Pinned 1.31.1 regeneration must
be byte-identical across auth/acl/admin packages.

AT-110 extends the existing desktop/mobile admin scenario without extra sign-ins.
`ptdev e2e` creates actual **4096-byte attachment and 2048-byte backup fixture
files** as the app UID only inside its unique disposable volume, before the
first observation. These are deliberately not attachment metadata/backup jobs
or a production seed API; the entire owned volume is removed by existing cleanup.
Metrics remain real: CPU/RAM/DB/disk/RSS/cgroup/version values and exact file
totals are asserted, followed by an updated snapshot within ten seconds and
unchanged hourly scan timestamps. Both locales, loading, an exact method/path/
503-scoped transport-failure fixture with explicit stale/retry UI, screenshots,
navigation and independent API/deep-route denial are covered.

The combined admin test allows 45 seconds because it now includes a real
ten-second poll in addition to users/groups/auth flows; its refresh assertion
remains ten seconds, no retries, and the suite's 120-second bound is unchanged.
All warnings/errors/CSP diagnostics remain fatal except narrowly expected,
explicitly asserted HTTP failures. Unit tests additionally prove hidden/unmount
cancellation, no overlaps, request timeouts, late-result rejection and recovery.
Record default-parallel check separately from any command-scoped `GOFLAGS=-p=1`
resource workaround; do not change shared testdb assertions/timeouts.

### T1.8 settings and workspace checks

Required uncached PG18 tests in `internal/admin/settings_test.go`,
`workspaces_test.go` and `internal/httpapi/settings_integration_test.go` cover
catalog/default/bound validation, sanitized public/admin DTOs, independent auth
classes, CSRF/body/query validation, atomic rollback and concurrent patches,
actor-lock recheck, actual auth/profile/default-locale creation readers,
workspace search/page/order, live/Trash/disabled/group-owner orphan rules,
50-owner payload cap and retained-attachment metadata sums. An actual owner
can read grants while a listing-only admin gets 404 and still has no ACL access.
These SQL fixtures exist **only in Suite-owned databases**, not developer data
or browser acceptance seeds.

```text
go test ./internal/admin ./internal/httpapi -run "TestSettings|TestWorkspaces|TestAdminSettings|TestAdminInventory" -count=3
go test ./cmd/pandatable ./internal/admin ./internal/auth ./internal/acl ./internal/httpapi -count=3
pnpm --dir web exec vitest run src/stores/preferences.test.ts src/stores/appSettings.test.ts src/pages/AdminSettingsPage.test.ts src/pages/AdminWorkspacesPage.test.ts src/router/guards.test.ts
go run ./tools/ptdev check
go run ./tools/ptdev e2e -- admin-users.spec.ts
go run ./tools/ptdev e2e
```

Explicit URL/required=1, fresh migrated Suite databases, pinned sqlc **1.31.1**
byte-identical auth/ACL/admin regeneration and the default-parallel versus
command-scoped `GOFLAGS=-p=1` distinction still apply. No assertion, diagnostic
or shared cleanup timeout is weakened to claim a pass.

The existing production-image admin desktop/mobile scenario uses real settings
GET/PATCH persistence, admin/ordinary/unauthenticated header reloads, title/text
safety, both locales, explicit-local-language preservation, new-user default
locale/effective timezone, real empty inventory, loading and narrow asserted
error/retry cases. No extra sign-in budget, mock successful acceptance values,
seed API, SQL browser workspace fixture or resource-route implementation.
Populated inventory rendering is a **unit test**, not full AT-112.
Full AT-111/322 awaits real uploads (T2.2); full AT-112 awaits real workspace
creation/navigation/table APIs (T1.10/T1.11); full AT-117 still awaits datetime/
filter UI. T1.8 stays In progress until linked acceptance really passes.

### T1.9 physical DDL checks

`internal/ddl` uses the same package-level Suite and no test-only production
endpoint. Required uncached focused validation (PowerShell):

```powershell
$env:PT_TEST_DATABASE_URL = (& go run .\tools\ptdev db status | Select-Object -Last 1)
$env:PT_TEST_DATABASE_REQUIRED = '1'
go test .\internal\ddl -count=3 -cover
```

Tests cover exact prefix/alphabet/50-bit entropy boundaries and rejected
identifiers/types/defaults; emitted DDL grammar and sanitized error shapes;
real PG18 native types/defaults/catalogs (including PG18 NOT NULL constraints);
BY DEFAULT identity/explicit restore IDs, nullable authorship/no FK/no trigger;
add/drop/set/remove data preservation, NULL-distinct UNIQUE, no CASCADE;
combined metadata/DDL commit and rollback and metadata-only rename evidence.
Injection literals are exercised with standard_conforming_strings on **and**
off. Varchar defaults are character-bounded before explicit casting to avoid
PostgreSQL's otherwise silent cast truncation.

The contention test first observes the blocking PID using pg_blocking_pids,
then requires the actual three-second lock timeout and verifies rollback of
both pending field metadata/schema version and physical column. A same-session
test verifies LOCAL timeout reset on commit and rollback. All physical objects
are confined to the Suite-owned disposable DB, not the developer DB.

Then run the full `ptdev check` and `ptdev e2e` gates. Record a default-parallel
cleanup failure separately from a command-scoped `GOFLAGS=-p=1` workaround;
never change shared cleanup timeouts/assertions to produce a pass.
Catalog tests are not real AT-202 or full AT-804 API acceptance: table/field
rename and filter injection require T1.10/T1.14/T2.6. See the exact results,
resource observations and cleanup in the
[T1.9 completion record](task-prompts/T1.9.md#implementation-and-handoff-2026-10-26).

### T1.10 structure API checks

`internal/structure` has its own Suite-owned PG18 database; `internal/httpapi`
adds real HTTP scenarios through the production router (no seed or test route).
Required uncached focused validation (PowerShell):

```powershell
$env:PT_TEST_DATABASE_URL = (& go run .\tools\ptdev db status | Select-Object -Last 1)
$env:PT_TEST_DATABASE_REQUIRED = '1'
go test .\internal\structure -count=1 -cover
go test .\internal\httpapi -count=1 -run TestStructureAPI -v
```

Service tests cover strict inputs, exact physical/metadata state (catalog columns,
identity/defaults, no FKs, six fields, display field, Grid config, locale labels,
no physical names in DTOs), metadata-only Chinese/payload renames with data
preserved, deterministic reorder (including renumbering and trashed siblings),
the owner/visibility matrix, soft-delete retention and immediate cache
invalidation, scripted collision retry bounds (physical and metadata) and
rollback after deliberately invalid DDL. Concurrency tests hold the workspace
lock in a separate transaction to prove grant revoke/downgrade, book/workspace
delete and user-disable races are rechecked after locking; a real three-second
lock timeout; serialized parallel creations; and deferred-trigger commit
failures that must still invalidate and persist nothing. HTTP tests cover
AT-201/203/213 at the API surface, admin without implicit access plus inventory
owner, the full level matrix, group-derived ownership, and the unauthenticated/
disabled/forced-change/CSRF/no-store/bounded-JSON/query/method classes.

Sidebar UI parts of AT-201/203/213, full AT-112, AT-202 field rename and full
AT-804 are **not** claimed; see
[T1.10 completion record](task-prompts/T1.10.md#implementation-and-handoff-2026-10-26).

### T1.11 navigation checks

The navigation store/component/resource-route tests cover coalescing, forced
refresh, stale responses and identity resets, owner-of-parent controls,
path-only contexts, same-parent drag and button reorder, Unicode name bounds,
localized failures, mobile drawer lifecycle, schema shortcut/view validation
and explicit initial-load errors. Run from the root:

```powershell
pnpm --dir web exec vitest run src\stores\nav.test.ts src\components\navigation\navigation.test.ts src\pages\ResourcePage.test.ts src\router\guards.test.ts
pnpm --dir web lint
pnpm --dir web build
go run .\tools\ptdev check
$env:GOFLAGS = '-p=1'; go run .\tools\ptdev check
$env:GOFLAGS = '-p=1'; go run .\tools\ptdev e2e
```

The expanded existing admin scenario reuses real Alice/admin sessions and the
real Bob/Carol accounts, activating participants through login/password/login.
No SQL, seeded workspace, mocked nav/schema/grant, auth bypass or changed login
budget is used. Alice creates W/B/T via UI, reads real Grid/Name schema, renames
to Chinese, reorders through drag or mobile buttons, and confirms/cancels Trash
deletion. Bob's read-only controls, Carol's table-only ancestor pruning and
workspace creation, admin sidebar/table-GET denial versus populated read-only
inventory, and concurrent removal errors are checked on desktop/mobile.

D-107 raises only the expanded scenario/global **harness** bounds to 75/180 s
for additional serial actions. Existing request, assertion, dashboard <=10 s,
login-budget and PG Suite cleanup deadlines are unchanged; one worker/no retries
and fatal unexpected console/CSP diagnostics remain. Report default-parallel
cleanup sensitivity separately from the command-scoped serial pass.
Full AT-111/322, AT-117, AT-202 field rename and AT-804 filter APIs remain pending;
M3 owns AT-203 restoration, not this M1 deletion scenario.

### T1.12 sharing checks

T1.12 extends the same real account contexts with
[sharing scenarios](../web/e2e/sharing.ts). D-112 supersedes D-107's harness
bounds only (150 s expanded scenario, 360 s suite); all other thresholds remain.

```powershell
$env:PT_TEST_DATABASE_URL = (& go run .\tools\ptdev db status | Select-Object -Last 1)
$env:PT_TEST_DATABASE_REQUIRED = '1'
go test .\internal\acl .\internal\httpapi -count=1
pnpm --dir web exec vitest run src\stores\sharing.test.ts src\components\sharing\sharing.test.ts src\components\navigation\navigation.test.ts src\pages\ResourcePage.test.ts
pnpm --dir web lint
pnpm --dir web test
pnpm --dir web build
go run .\tools\ptdev check
$env:GOFLAGS = '-p=1'; go run .\tools\ptdev check
$env:GOFLAGS = '-p=1'; go run .\tools\ptdev e2e -- admin-users.spec.ts
$env:GOFLAGS = '-p=1'; go run .\tools\ptdev e2e
```

The directory test uses `internal/testdb.Suite` with real PG18 for owner-only
privacy/bounds, literal search, active filtering, uncached grant revocation,
hidden/visible/forced/unauthenticated errors and trashed ancestors. SQL fixtures
here are lower-layer evidence, not browser acceptance. Static queries regenerate
with the existing pinned sqlc 1.31.1 command; no migration/dependency.

Store/component tests cover stale target/query/type/identity responses, exact
search bounds, CSRF/review/write serialization, uncertain-write reconciliation,
last-owner/principal errors, redaction/retry, disabled/dangling grants and
target-owner versus parent-owner controls. The production-image scenario uses
real users/groups/grants and membership APIs with no mocked route, SQL seed,
additional login budget or row/editor proxy acceptance. Browser diagnostics
remain strict for all retained user tabs; only explicitly expected status errors
are consumed. Default-parallel Suite cleanup sensitivity must still be reported
separately from command-scoped serial results.
