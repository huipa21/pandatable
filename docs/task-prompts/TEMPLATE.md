# Task prompt - Tx.y <title>

Keep the prompt under about 6 KB and the final record under about 2 KB (D-158).

## Read (only these)

- `AGENTS.md`, `docs/README.md`.
- Named sections only, e.g. `docs/07-field-types.md` §3, `docs/09-api.md` "Rows".
- Decisions by ID (search, do not read the whole log): D-xxx, D-yyy.
- Acceptance by ID (search): AT-xxx.

## Scope

- Goal in 2-4 bullets; linked AT-xxx.
- Out of scope: list only what is likely to be confused with this task.

## Contracts to respect

- Short bullets of binding behaviour this task touches (no restating whole docs).

## Verification (D-158)

1. Inner loop: `go build ./...`; focused `go test -run <Name> ./internal/<pkg> -count=1`
   (explicit PG18 DSN, `PT_TEST_DATABASE_REQUIRED=1`); focused `vitest run <file>`.
2. Task gate: `go run .\tools\ptdev check` and
   `go run .\tools\ptdev e2e --fast -- e2e/<spec>.ts`.
3. Do **not** run the complete production-image `ptdev e2e`; it runs once at phase end.
   Exception: changes to `deploy/Dockerfile`, Compose, `go.mod`/`go.sum` or
   `pnpm-lock.yaml` also build the release target once.

## Wrap-up

- `progress.md`: one task-table row and one change-log line.
- `acceptance-tests.md`: update only the affected AT-xxx evidence lines.
- New non-trivial choices: one `D-xxx` row.
- Append a record below (about 2 KB max): outcome, changed files, commands with
  pass/fail counts, open risks, next step. No version dumps or VM inventories.
- Conventional Commit on `master` referencing the task.

## Record (YYYY-MM-DD)
