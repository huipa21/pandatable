# 08 — Query Engine (Views, Filter, Sort, Group, Highlight, Formulas)

All row reads go through `internal/query`, which compiles a **view config** (+ ad-hoc overrides such as quick search) into a single parameterised SQL statement. Identifiers come only from meta (`pg_name`, `pg_column`); values are always bound parameters.

## 1. View config (`meta.views.config`)

```jsonc
{
  "fields": [                         // order = display order
    { "id": "fld_…", "visible": true, "width": 180 },
    { "id": "fld_…", "visible": false }
  ],
  "frozen": 1,                         // number of frozen leading columns
  "rowHeight": "short",                // short | medium | tall | extra
  "filter": {                          // nested, max depth 3
    "op": "and",
    "items": [
      { "field": "fld_status", "cmp": "is_any_of", "value": ["Open","Blocked"] },
      { "op": "or", "items": [
          { "field": "fld_due", "cmp": "before", "value": { "rel": "today" } },
          { "field": "fld_owner", "cmp": "is", "value": { "rel": "me" } }
      ]}
    ]
  },
  "sort":  [ { "field": "fld_due", "dir": "asc" } ],
  "group": [ { "field": "fld_status", "dir": "asc" } ],   // max 3
  "highlight": [                                          // ordered, first match wins per target
    { "id": "hl1", "when": { /* filter tree */ }, "target": { "type": "row" }, "color": "red-2" },
    { "id": "hl2", "when": { /* … */ }, "target": { "type": "cells", "fields": ["fld_due"] }, "color": "amber-3" }
  ],
  "aggregates": { "fld_amount": "sum", "fld_id": "count" },
  // form view
  "form": { "title": "", "description": "", "fields": [ { "id": "…", "label": "…", "help": "…", "required": true } ], "afterSubmit": "message|new" },
  // kanban (M4)
  "kanban": { "groupField": "fld_status", "coverField": null },
  // calendar (M5)
  "calendar": { "dateField": "fld_due", "endField": null }
}
```

The config is validated server-side against current fields; references to deleted fields are dropped silently on read and cleaned on next save. View config updates use optimistic concurrency (`version`); conflicts → client reloads and re-applies.

T1.21 introduced the Grid column subset (D-148): ordered
`fields:[{id,visible,width?}]` and `frozen`. T2.7–T2.9 add configured sort,
grouping/aggregates, and highlights. Schema/view responses include the
sanitized supported subset, not arbitrary future config. Live configured
fields retain their order; omitted user fields append visible in metadata
order, omitted system fields append hidden. Read drops unknown/deleted/
duplicate column references and clamps the frozen count after removals. PATCH
rejects non-live or duplicate IDs, validates integer widths 60..1000 and a
frozen count from zero to the effective visible-column count. Missing widths
render at 180 px; the existing default display field retains its stored 240 px.

`PATCH /views/{id}` keeps the versioned `{config:{fields,frozen}}` column
contract and also accepts view metadata edits. Every persisted existing-view
edit requires the authoritative version in `If-Match`, increments that view's
version once, and leaves table schema version unchanged. Config updates merge
supported members into stored JSON, preserving untouched values such as
`filter`, `rowHeight`, sort, grouping, highlights, aggregates, and future
members. D-178 supersedes D-148/D-149's narrower T1.21 persistence rules:
readers can persist their own personal views; shared edits need table write
access unless locked, when only owners may edit. Only owners may lock/unlock
shared views. Personal views remain owner-only 404 to other users.

T2.9 (D-177) stores up to 20 ordered highlight rules in `highlight`. The
`when` tree uses the same server filter validation/compiler and bound values.
Row and per-cell-field CASE expressions resolve independently, with the first
matching rule winning for each target. Targets must name unique live fields;
colors must be one of the six palette tokens. Incomplete editor drafts remain
local until the rule is valid.

**Personal overrides:** readers and users of locked shared views may apply
temporary in-memory filter, sort, column, and highlight changes. “Save as
personal view” duplicates the supported source config plus the current
overrides without writing to the shared view. The D-138 transient query
override remains available and does not persist anything.

## 2. Filter operators

| Field kinds | Operators |
|---|---|
| text-like (text, long_text, email, url, phone) | is, is_not, contains, not_contains, starts_with, ends_with, is_empty, is_not_empty |
| numeric (integer, decimal, float, currency, percent, rating, autonumber) | eq, neq, lt, lte, gt, gte, between, is_empty, is_not_empty |
| date/datetime | is, is_not, before, after, on_or_before, on_or_after, between, is_within (past/next N days, this week/month/year), is_empty, is_not_empty |
| time, duration | =, <, >, between, is_empty |
| checkbox | is_checked, is_not_checked |
| single_select | is, is_not, is_any_of, is_none_of, is_empty, is_not_empty |
| multi_select, text_array, int_array | has_any_of, has_all_of, has_none_of, is_exactly, is_empty, is_not_empty |
| user / multi_user | is / has_any_of (incl. `{rel:"me"}`), is_empty |
| uuid, inet, cidr | is, is_not, is_empty; inet/cidr: `contained_in` (`<<=`) |
| json | is_empty, is_not_empty, has_key, path_equals (`#>>`) |
| link | has_any_of (row ids), is_empty, is_not_empty, count cmp |
| lookup / rollup / formula | operators of the result type |

Relative values: `today`, `yesterday`, `tomorrow`, `me`, `n_days_ago:N`, `n_days_from_now:N`, evaluated in the **requesting user's timezone** (user setting, default app `default_timezone` = Asia/Shanghai), e.g. `today` on a datetime field → `c >= ($tz_midnight) AND c < ($tz_midnight + 1 day)` computed in Go and bound as timestamptz; `date` fields compare against the user's local date. DB sessions run in UTC (D-049).

Text comparisons are case-insensitive (`ILIKE` / `lower()`), with `%`/`_` escaped.

T1.13 only declares batch-1 `FilterOps`/`Aggregates` in the Go and frontend
registries (D-120); no query/filter/aggregate compiler is implemented.
System id uses numeric capabilities, system timestamps temporal capabilities,
and authorship UUIDs use is/is_not/has_any_of/is_empty/is_not_empty.
NOT NULL system columns omit empty operators. Consumers must intersect
operations with the bound field's declared capabilities; do not infer support
from a raw PG type. Bigint/decimal filter values must retain their string wire
precision, and relative temporal compilation still requires the user's zone.

## 3. Generated SQL shape

```sql
SELECT t.id, t.c_a, t.c_b, …,
       /* link (parent side, display values) */
       (SELECT coalesce(jsonb_agg(jsonb_build_array(c.id, c.c_disp) ORDER BY c.id), '[]')
          FROM b_x.t_child c WHERE c.c_fk = t.id LIMIT 50) AS f_link,
       /* rollup */
       (SELECT sum(c.c_amount) FROM b_x.t_child c WHERE c.c_fk = t.id) AS f_total,
       /* formula */
       (CASE WHEN t.c_qty > 0 THEN t.c_price * t.c_qty ELSE 0 END) AS f_formula,
       /* highlight */
       (CASE WHEN <hl1> THEN 'red-2'::text END) AS _hl_row,
       jsonb_strip_nulls(jsonb_build_object(
         'fld_due', CASE WHEN <hl2> THEN 'amber-3'::text END
       ))::text AS _hl_cells
FROM b_x.t_main t
WHERE <filter> AND <quick search>
ORDER BY <group keys>, <sort keys>, t.id
LIMIT $n OFFSET $m;
```

- Every ORDER BY ends with `t.id` for deterministic paging.
- Paging: offset/limit, page size 100 (grid fetches visible window ± 1 page). Fine for ≤ 200k rows (D-019).
- Total count: separate `count(*)` with the same WHERE, cached per (view, filter hash, data version) for 5 s.
- Link cells return at most 50 display items + total count.
- `_hl_row` is a palette token or null; `_hl_cells` is returned as an object
  from field UUIDs to palette tokens, omitting unmatched fields.

### T1.15 basic query implementation (D-132..D-135)

`POST /views/{id}/query` implements a single-table page and optional exact
count. Search, grouping, aggregates, relations and data-version caching remain
deferred. It accepts `offset` (0..1,000,000, default 0), `limit` (1..100,
default 100), the compatible single-column
`sort:{fieldId,direction}` or ordered `sorts:[{fieldId,direction},…]` override
(not both), and `includeCount` (default true). The ordered sort list overrides
the view's configured sort; omitting both uses the configured list. Duplicate
criteria are ignored, invalid explicit criteria are rejected, and invalid or
deleted configured criteria are ignored. Formula, lookup and rollup
expressions are sortable when their referenced data is readable to the caller;
configured computed sorts that are not readable are ignored so normal masked
row values remain available. NULLS LAST applies in either direction, followed
by row ID ascending for deterministic paging. The result is
`{rows:[{id,fields}],fieldIds,total,schemaVersion}`; `total` is null when
`includeCount:false`. Field IDs preserve the view's visible column order.
Configured visible live fields retain their order; deleted/unknown/duplicate
IDs are ignored, omitted live user fields append visible in metadata order,
and system fields are hidden unless explicitly enabled in the view config.
Sort uses the bound field descriptor and metadata column, NULLS LAST for both
directions, then row id ascending. Count is a separate exact `count(*)` over
the same snapshot and is optional so callers that only need a page can avoid
that work.

T1.16 adds an optional `fieldIds` query member for a transient display override
used by the Fields menu. Omitting it preserves the stored view configuration.
When supplied, it is an ordered list of live field UUIDs (including explicitly
selected system fields); duplicate, unknown and deleted IDs are ignored. The
override is not persisted. Table read permission and metadata-derived SQL
identifier validation are unchanged (D-138).

The query loads only the selected live table/view and field metadata, quotes
validated generated identifiers, and binds all values with Squirrel. A
repeatable-read transaction holds a table `FOR SHARE` lock for the view and
table, preventing concurrent field DDL from changing columns mid-query.

The opt-in Suite-owned PG18 fixture seeded 200,000 `generate_series` rows in
579.7 ms; a 100-row page at offset 100,000 without count took 26.2 ms, and a
100-row page plus exact count took 20.4 ms. These local-fixture timings are API
evidence only, not the 2-vCPU/4-GB target-host NFR-2 result or full AT-312;
that acceptance still requires T1.16's production grid and browser scenario.

## 4. Grouping (server-side)

1. `POST /views/{id}/groups` with `path` (e.g. `[]` for top level, `["Open"]` for level-2 inside "Open") returns group buckets: `{key, count, aggregates}` using `GROUP BY` on the group expression under the full filter.
2. Rows for an expanded leaf group are fetched with the group path added to WHERE, paginated as usual.
3. Empty values form a `(empty)` group. Multi-select groups by the full array value (sorted) — v1 simplification.
4. Client renders a virtual flattened list: group header rows + data rows; collapsed state stored per user in localStorage.

## 5. Quick search

`ILIKE '%term%'` OR'ed across visible text-like fields, `::text` cast for numbers/dates. Uses `pg_trgm` GIN index if the table is large and search is used (auto-index policy, D-020).

## 6. Footer aggregates

Per visible field: none, count, count_empty, count_filled, unique, sum, avg, min, max, earliest, latest, percent_checked. One `SELECT agg(...), agg(...)` with the view's WHERE.

## 7. Formula language (D-018)

Airtable-like, compiled to SQL (never evaluated in Go per-row).

```
{Price} * {Qty}
IF({Status} = "Done", "✓", "")
CONCAT({First}, " ", {Last})
DATETIME_DIFF(NOW(), {Created at}, "days")
```

- **Lexer/parser** → AST → **type checker** (number, text, bool, date, datetime, duration, array) → **SQL compiler** emitting expression with bound literals.
- Field refs `{Field name}` stored internally as field ids (renames safe).
- Functions v1:
  - Logic: `IF, AND, OR, NOT, SWITCH, BLANK, ISBLANK, COALESCE`
  - Math: `ROUND, ROUNDUP, ROUNDDOWN, ABS, CEILING, FLOOR, MOD, POWER, SQRT, MIN, MAX, SUM, AVERAGE, INT`
  - Text: `CONCAT, LEN, LEFT, RIGHT, MID, UPPER, LOWER, TRIM, SUBSTITUTE, FIND, REPLACE, T, VALUE, REGEX_MATCH`
  - Date: `NOW, TODAY, YEAR, MONTH, DAY, WEEKDAY, HOUR, MINUTE, DATEADD, DATETIME_DIFF, DATETIME_FORMAT, DATE`
  - Operators: `+ - * / % & = != < <= > >= ( )`
- Division by zero → NULL (`NULLIF`). Errors at runtime (bad cast) are prevented by type checker; `VALUE()` uses `pg_input_is_valid` guard.
- `NOW()`/`TODAY()`/`DATETIME_FORMAT()`/`YEAR()`… on datetimes use the requesting user's timezone (`AT TIME ZONE $tz`), so two users in different timezones may see different `TODAY()` results — intended.
- Formulas may reference other formulas, lookups and rollups (inlined; cycle detection; max depth 8).
- Formulas are filterable/sortable/groupable (they are just SQL expressions).
- Why not PG18 virtual generated columns: they require immutable expressions on the same row (no `NOW()`, no lookups/rollups), and altering them is DDL; a uniform query-time approach is simpler. Revisit if performance needs it.

## 8. Lookups & rollups

- Compiled as correlated subqueries (shown above); for M:N via junction join.
- Lookup returns an ordered JSON array of the target field values; rollup returns a scalar using `count`, `sum`, `avg`, `min`, `max`, `earliest`, `latest` or `concat`.
- Values are computed in row queries and masked when the caller lacks read access to the target table. Filtering and sorting computed fields remain for T2.6 and must preserve this permission boundary.
- A live computed field may reference only a stored target field and a live relation. Deletion of a referenced field or relation is rejected until the dependent computed field is deleted (D-171).
- Indexes on FK columns and junction `(child_id)` keep these fast at target scale.

## 9. Writes

- `PATCH` row: only provided fields; codec-validated; `updated_at/by` set; history captured via `RETURNING OLD.*, NEW.*` (PG18).
- Bulk (paste/fill-down/import): one request, one transaction, `UPDATE … FROM (VALUES …)` / `COPY` for inserts; ≤ 5,000 cells per paste request (client splits larger).
- Link changes: `POST /rows/{id}/links/{fieldId}` `{add:[ids], remove:[ids]}` — 1:N sets FK on child rows; 1:1 enforces uniqueness (replacing old link after confirmation); M:N inserts/deletes junction rows.
