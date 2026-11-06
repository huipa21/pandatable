# WG-04 AGG - percentage summary operations

Read [execution plan](workspace-grid-refinement.md). No prerequisites.

## Read (only these)

- `AGENTS.md`, `docs/README.md`, `docs/conventions.md`.
- `docs/08-query-engine.md` sections 1, 5, 6.
- `docs/09-api.md` "Rows & queries", aggregate endpoint only.
- Decisions: D-115, D-158, D-230; search aggregate-related decisions as needed.
- Acceptance: AT-504, AT-509, AT-510.

## Scope / ownership

- Own aggregate capability/contracts in `internal/codec`, aggregate SQL and
  validation in `internal/structure`, focused aggregate tests in those packages
  and `internal/httpapi/search_aggregates_integration_test.go`.
- Own matching `web/src/fields` aggregate types/capabilities and tests;
  `web/src/api/types.ts` only if an actual DTO change is needed.
- Existing operations include count/empty/filled/unique and typed numeric,
  temporal and checkbox operations. Add the missing percentage operations
  required by the reference: `percent_empty`, `percent_filled`, `percent_unique`.
  Search existing code/SQL helpers first; reuse their empty/distinct semantics.
- Percent Empty/Filled use their existing count numerator divided by all
  matching rows. Percent Unique uses existing unique-count semantics divided
  by all matching rows. For zero matching rows return a documented zero
  percentage, not NaN/divide-by-zero. Match percent_checked's scale and exact
  JSON representation; hand that contract to WG-05.
- Advertise operations only for compatible types, consistently in Go and TS.
  Keep existing operations/backward-compatible saved configs working.
- Compute server-side over the complete authorized filtered/searched dataset;
  never approximate from loaded pages or use JS floating-point conversion.
- No DataGrid, toolbar, navigation, locale JSON or E2E-spec edits. No new
  dependency/migration or broad rollup redesign; update shared validation only
  where required to accept and safely evaluate these operations.

## Confined verification

1. WG-04.1: fixture with 4 rows (1 empty, 3 filled, 2 distinct non-empty
   values) produces 25%, 75%, 50%; retain established type-specific emptiness.
2. WG-04.2: zero rows, all empty, duplicates, numeric zero/false, applicable
   array/text types, filtered/search subsets and hidden fields are correct.
3. WG-04.3: unsupported field/operation rejected; saved config round-trips;
   unauthorized/personal views remain protected; existing exact sum/avg and
   grouped operations remain unchanged.
4. Run focused codec and aggregate Vitest tests. Run focused Go unit tests;
   for SQL/API cases use explicit PG18 DSN and
   `PT_TEST_DATABASE_REQUIRED=1`, selecting only relevant tests in
   `./internal/structure` and `./internal/httpapi`. Use existing dev DB startup
   command if needed, not a full check/build. Do not accept DB skips as pass.
5. No broad suite, `ptdev check`, E2E or image builds here.

## Handoff / wrap-up

Record exact operation/scale/empty contracts and passed test selectors for
WG-05. Hand WG-01 proposed query/API docs, decision entry and both translations
if any. Leave **implemented, awaiting WG-09 gates**, no commit.

## Record

Implemented 2026-11-06; awaiting WG-09 gates.

- Added `percent_empty`, `percent_filled`, and `percent_unique` to the Go and
  TypeScript aggregate contracts. They are advertised only where the matching
  empty/filled and distinct operations exist; multi-select omits percent-unique.
- Results are ratios from 0 to 1 over the complete matching dataset. Empty,
  filled and distinct non-empty counts are divided by all matching rows; zero
  rows return numeric zero for the new operations. Footer values are exact JSON
  strings normalized only for trailing fractional zeroes. Grouped aggregate
  values retain their existing JSON-number representation. Existing
  `percent_checked` empty-result behavior remains null.
- WG-05 contract: `percent_empty`, `percent_filled`, `percent_unique`; footer
  wire values are decimal strings such as `"0.25"`, not JavaScript numbers.
- WG-01 shared-doc handoff: in `docs/08-query-engine.md` §6 list the three
  operations and define their 0..1, zero-row and type-capability semantics; in
  `docs/09-api.md` document their exact-string footer values. No API DTO change
  was needed. Propose D-231 to record the ratio/zero-row contract; D-179
  already governs exact footer strings. WG-05 should add picker labels to both
  locales: “Percent empty” / “空值百分比”, “Percent filled” / “非空值百分比”,
  and “Percent unique” / “唯一值百分比”. No shared docs, decisions or locale
  files were edited here.
- Passed: `go test ./internal/codec`; PG18 with
  `PT_TEST_DATABASE_REQUIRED=1`, selectors
  `TestGroupingTextPathsAndArraySemanticsPostgreSQL` and
  `TestQuickSearchAndFooterAggregatesUseVisibleFilteredRows` in
  `./internal/structure ./internal/httpapi`; `pnpm --dir web exec vitest run
  src/fields/fields.test.ts` (340 tests); focused ESLint, Prettier and `go vet`.
  `golangci-lint` was unavailable. No broad task or phase gates were run.
