# Task record - Dev startup recovery

## Scope

Automatically ready the selected container engine and recover dev PostgreSQL
port conflicts without deleting database data or terminating other listeners.
Preserve explicit `PT_ENGINE`/`PT_DATABASE_URL` settings and D-213's backend/Vite
port policy.

## Outcome (2026-11-05)

- Added bounded engine health probes/startup for DB, image and E2E operations.
  Podman machines already starting/running are allowed to reconnect.
- Reuse running DBs and available stopped-container mappings; otherwise select
  a free loopback port and recreate only a verified stopped container, retaining
  its image and named volume. Engine bind races have three startup attempts;
  unrelated failures and unexpected mounts fail explicitly.
- Container metadata supplies the actual DSN to dev, test/check and status.
  Windows engine startup precedes the supervisor's kill-on-close Job Object.
- Updated [developer documentation](../14-dev-build-release.md) and D-216 in
  [decisions](../decisions.md). No dependencies or application behavior changed.

## Verification

- `go test ./tools/ptdev -count=1` and focused readiness/recovery regressions:
  pass (engine selection/startup, cancellation, bind races, reuse, image/volume
  retention, unsafe mounts and error propagation).
- `PTDEV_TEST_DEV=1` with `go run .\tools\ptdev check`: lint, required PG18 Go
  tests (including real dev startup/proxy/shutdown), 759 frontend tests in 56
  files, and build pass. Optional golangci-lint remains unavailable.
- `go run .\tools\ptdev e2e --fast -- e2e/shell.spec.ts`: 5 passed; owned stack
  and image cleaned up.
- Live Podman test held the previous DB port open: startup selected another
  port, preserved PostgreSQL's cluster system identifier, and status/check used
  the new mapping. The dev database remains running on its resolved port.

## Limits

Docker startup and offline-machine paths use fake-command regression tests;
the shared Podman VM was not stopped. Full AT-006 Linux/Docker acceptance is
still pending, not claimed by this fix.
