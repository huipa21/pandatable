# UX-01 - Persistent default-workspace lifecycle

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- [Execution index](workspace-grid-redesign.md), including shared-write rules.
- `docs/01-requirements.md` sections 4.1-4.3; `docs/05-data-model.md` users,
  workspaces and grants; `docs/09-api.md` Setup & auth and Structure.
- Search decisions D-009, D-010, D-023, D-079, D-104, D-131, D-158, D-217.
- Search acceptance AT-101, AT-201, AT-203, AT-208, AT-212, AT-213.

## Scope

Implement only the backend/data contract for a user's startup workspace.
No sidebar, home-page, grid, icon or card work.

- Provision an owned starter workspace named `Default` for new users and
  idempotently bootstrap existing users without altering their old workspaces.
  Renaming a starter must never cause another starter to appear.
- Persist a per-user preferred workspace ID. Expose read/set through the
  existing authenticated preference/API patterns; synchronize Go and TS DTOs.
  It is a preference, not a workspace-wide flag affecting other users.
- Allow choosing an accessible, live, non-path-only workspace with read access;
  changing this preference does not require workspace ownership. It grants no
  access. Reject hidden/deleted/context-only targets with standard errors.
- When a preferred workspace is deleted or access is revoked, resolve to a
  remaining eligible workspace in deterministic server order. If none exists,
  safely provision an owned `Default`. Do not treat network/DB failures as
  an empty workspace list or silently create data after a failed read.

## Ownership and handoff

Own default-related auth/structure services, HTTP handlers, SQL, migration,
generated bindings, focused tests and `web/src/api/types.ts` additions.
Inspect `internal/structure/workspaces.go`, `nav.go`, auth creation/setup/admin
user flows, and their existing transaction/ACL locks before deciding where to
bootstrap. Use forward migrations and pinned sqlc generation where applicable.
UX-03 waits for your handoff before touching migrations/API types.

Define and record the exact preference/bootstrap endpoint, response shape,
null semantics and resolution rules for UX-02. Do not leave those to be guessed.
No eager mass rewrite or access to every user's data from a normal-user request.
No side-effecting unauthenticated request or workspace creation on every poll.
Coordinate service boundaries without circular dependencies or duplicated
workspace/grant creation code.

## Acceptance scenarios

1. First use by a new normal user and setup admin yields a live `Default`,
   its creator owner grant and a persisted preferred ID.
2. Two concurrent first-use requests and a restart still yield one starter for
   that user. Repeat for an existing populated user; old data is unchanged.
3. Rename the starter; refresh/bootstrap does not recreate the old name.
   Set another workspace as preferred; `/me` or the agreed read API retains it
   across sessions. A second user's preference is unaffected.
4. Hidden, malformed, trashed and path-only targets fail without preference or
   grant changes. An admin receives no implicit access to others' workspaces.
5. Delete/revoke the selected target: fallback resolves once without duplicates,
   exposing inaccessible IDs, changing ownership, or a redirect loop. No eligible
   workspace causes one safe owned replacement, including under concurrency.
6. Failed transactions leave no orphan workspace/grant/preference combination;
   validation and authorization errors use existing API conventions.

## Verification and wrap-up

Run focused Go/HTTP integration tests with required PG18 under the serialized DB
slot; verify migration upgrade and pinned generator consistency if touched.
Add browser/API coverage in `web/e2e/default-workspace.spec.ts` for UX-02 to extend.
Full task gates and central docs follow the execution index; do not run the
production-image gate here. Record contracts and evidence below, not in new notes.

## Record

Started 2026-11-05: implementing the authenticated, per-user default-workspace
lifecycle and its focused verification. UI/navigation changes remain for UX-02.

Implementation handoff: migration 0007 stores a preference row per user. Setup
and admin account creation provision `Default` and the direct owner atomically;
legacy users initialize on the first explicit resolve. A renamed starter is not
recreated. No normal-user request scans or rewrites other users.

UX-02 contract: `GET /api/v1/auth/workspace-preference` returns
`{workspaceId: string|null}` without side effects; null also hides a stale,
trashed or inaccessible selection. `POST .../resolve` with `{}` returns
`{workspaceId: string,eventId}` and initializes/repairs once. Call it when
entering `/home`, not on `/nav` polls. `PUT /api/v1/auth/workspace-preference`
with exactly `{workspaceId: string}` returns the same mutation shape. Live
workspace read access (including group grants) suffices; hidden/deleted/malformed
IDs are 404, path-only is 403, invalid/null JSON is 400, contention is 409
`ERR_WORKSPACE_PREFERENCE_BUSY`. All use normal session/forced-password,
no-store and CSRF rules. If the selection is lost, resolve retains the earliest
eligible workspace in nav creation/UUID order, or atomically creates one owned
replacement. Database errors never trigger fallback creation.

Focused Go/PG18 tests pass for concurrent legacy bootstrap/replacement, restart
and rename, reader/group selection, revocation/deletion races, disabled users,
rollback/failed reads, setup/admin provisioning, HTTP authorization and migration
upgrade. A real advisory-lock timeout returns the localized 409 without partial
data, and pinned sqlc generation is byte-identical.

Stabilized 2026-11-05: `go run .\tools\ptdev check` passes required PG18,
lint/typecheck, 782 frontend tests and build (the optional golangci-lint was
unavailable). `go run .\tools\ptdev e2e --fast -- default-workspace.spec.ts
admin-users.spec.ts trash.spec.ts user-lifecycle.spec.ts` passes all 9 setup,
desktop and mobile tests. Initial checks exposed stale migration expectations
and a missing client error mapping; both were fixed and retested.

Changed surfaces: migration, shared transactional workspace creation,
setup/admin provisioning, preference service/HTTP/SQL/TS DTOs, localized error,
seed preflight and focused tests. UI landing/sidebar work belongs to UX-02;
the production-image phase gate and coordinated commits remain for UX-11.
The task gates ran on the shared tree with UX-07 source writes paused.
