# Setup origin 403 fix

## Scope and contracts

- Fix development setup POST returning 403 when browsing localhost.
- Preserve exact backend CSRF Origin checks and setup-token validation.
- AT-101; D-007 and D-208. No migrations or dependencies.

## Record (2026-11-02)

Root cause: the browser used `http://localhost:5173`, while the default backend
base URL was `http://127.0.0.1:5173`. Identical setup requests returned
`403 ERR_CSRF` with the first Origin and `401 ERR_SETUP_TOKEN` with the second.
Setup's error allowlist also mapped `ERR_CSRF` to the generic internal error.

Vite now redirects HTML page navigation between loopback aliases to the
configured local HTTP origin, preserving path/query. API requests, mutations
and Origin headers remain untouched. `ptdev dev` passes the resolved base URL
to Vite as well as the backend and prints that URL. Setup uses the existing
English/Chinese CSRF error. Development documentation and D-208 record the rule.

Verification:

- Focused Vitest origin/UI tests: 8 passed.
- Focused Go dev config/API target and HTTP CSRF/origin tests: pass.
- `pnpm --dir web typecheck`: pass.
- `go run .\tools\ptdev check`: lint, test and build pass; 54 frontend files,
  675 tests passed; required PostgreSQL integration tests pass.
- `go run .\tools\ptdev e2e --fast -- setup.spec.ts`: 1 passed (AT-101).
- Live browser: localhost setup redirects to 127.0.0.1; a deliberately empty
  setup submission reaches token validation (401), not CSRF rejection.
  No administrator was created in the user's development database.

Reload the already-open setup page before retrying. Standalone Vite and backend
processes must still use matching `PT_BASE_URL` settings. No phase gate claimed.
