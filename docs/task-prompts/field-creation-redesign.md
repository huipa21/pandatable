# Field Creation Redesign

## Scope

- Match the supplied name/type-picker screenshots using existing field APIs.
- Include Link, Lookup, Rollup and Formula in one searchable, scrollable icon list.
- Show directional relation choices, related-table/field selectors and formula functions.
- Keep unsupported enterprise controls and unsupported aggregates out of scope.

## Contracts

Existing owner authorization, schema reconciliation, localization and bounded
requests remain unchanged. D-212 records the UI choices; no new dependencies.

## Verification

Focused FieldPanel tests; `ptdev check`; fast field, relation, computed and
formula browser specs. No full production-image milestone run.

## Record (2026-11-04)

Implemented the searchable, scrollable 32-type icon picker in
`FieldTypePicker.vue`, reused by `FieldPanel.vue`. Removed separate advanced
type actions; Link has four cardinality choices and advanced reverse/deletion
settings. Lookup/Rollup arrange dependency selectors together; Formula shows
the supported function catalog before typing. Switching types preserves the
name and correctly collapses the picker when returning to a stored type.
Existing AppIcon mappings, both locales, styles and browser flows were updated.

Verification:
- Focused `vitest run src/components/fields/FieldPanel.test.ts`: 24 passed.
- Final `go run ./tools/ptdev check`: lint, tests and build passed.
- Final `ptdev e2e --fast -- e2e/fields-acceptance.spec.ts
	e2e/fields-batch2.spec.ts e2e/relations.spec.ts e2e/lookups-rollups.spec.ts
	e2e/formulas.spec.ts`: 11 passed, 4 intentional skips.
- Initial gate including `e2e/hardening.spec.ts`: 31 passed, 5 skipped, one
	batch-2 user-name cell visibility failure after successful saved-value
	assertions. The final affected-spec gate above passed that scenario.
- Live in-memory fixture: all five editor types fit 1280px and 390px viewports
	without horizontal overflow; picker has 32 icons and a 290px scrolling area;
	search isolates Rollup. Desktop Link and mobile Formula screenshots checked.

No API, dependency or migration changes. Unsupported enterprise-only controls,
formula formatting and distinct aggregates in the reference are not added.
Dev UI remains running at http://127.0.0.1:5173.