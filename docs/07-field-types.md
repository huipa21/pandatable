# 07 — Field Types

Each field has a **UI type** (how it behaves in the app) backed by a **PostgreSQL type** (how it's stored). Every UI type is implemented by a Go `Codec` (validate/parse/format/SQL cast) and a frontend `FieldKind` (renderer, editor, filter input, paste parser).

## 1. Stored types

| UI type | `ui_type` | PG type | Options (`fields.options`) | Notes |
|---|---|---|---|---|
| Single line text | `text` | `text` or `varchar(n)` | `maxLength?` | `maxLength` → `varchar(n)` |
| Long text | `long_text` | `text` | `richText: false` (v1 plain, markdown later) | |
| Integer | `integer` | `smallint` / `integer` / `bigint` | `size: small\|int\|big`, `thousandSep` | |
| Decimal | `decimal` | `numeric(p,s)` | `precision`, `scale` | |
| Float | `float` | `real` / `double precision` | `size: real\|double`, `displayDecimals` | |
| Currency | `currency` | `numeric(18,2)` (scale configurable) | `currency` (ISO 4217, defaults to app `default_currency` = CNY at creation, changeable per field), `scale` | formatted with `Intl.NumberFormat(userLocale, {currency})` |
| Percent | `percent` | `numeric(7,4)` | `displayDecimals` | stored as fraction (0.25 = 25%) |
| Rating | `rating` | `smallint` | `max: 5`, `icon: star` | CHECK 0..max |
| Checkbox | `checkbox` | `boolean` | | NOT NULL DEFAULT false |
| Date | `date` | `date` | `dateFormat: iso\|slash\|us\|eu` | |
| Time | `time` | `time` | `format: 24h\|12h` | |
| Date & time | `datetime` | `timestamptz` | `dateFormat`, `timeFormat: 24h\|12h`, `timezone: user\|fixed`, `fixedTz?` | stored UTC; displayed in the viewing user's timezone (default Asia/Shanghai) unless field has a fixed timezone |
| Duration | `duration` | `interval` | `format: h:mm\|h:mm:ss\|human` | |
| Single select | `single_select` | `text` | `choices: [{id, label, color}]` | value stored = label; rename choice → `UPDATE` values in same tx |
| Multi select | `multi_select` | `text[]` | `choices` | GIN index when used in filters on large tables |
| Email | `email` | `text` | | validated |
| URL | `url` | `text` | | validated, clickable |
| Phone | `phone` | `text` | | loose validation |
| UUID | `uuid` | `uuid` | `autoGenerate: bool` | default `uuidv7()` if auto |
| JSON | `json` | `jsonb` | | code editor |
| IP address | `inet` | `inet` | | |
| Network | `cidr` | `cidr` | | |
| Text list | `text_array` | `text[]` | | tag-style editor |
| Integer list | `int_array` | `bigint[]` | | |
| User | `user` | `uuid` | `notifyOnAssign` (M5) | FK → `meta.users(id)` ON DELETE SET NULL |
| Multi user | `multi_user` | `uuid[]` | | no FK; deleted users filtered on read |
| Attachment | `attachment` | `jsonb` | `accept`: up to 20 comma-separated MIME types/wildcards (default: any sniffed type); `maxFiles: 1..20` (default: 10) | array of `{id,name,mime,size,thumb}`; files in `meta.attachments`; upload limit applies per file (D-163); image decode capped by admin `imageMaxMegapixels` (D-164); tightened options do not hide stored values (D-168) |
| Auto number | `autonumber` | `bigint` | `prefix?`, `padding?` | own sequence `s_<short>`, read-only |

## 2. Link & computed types (virtual or FK-backed)

| UI type | `ui_type` | Storage | Options |
|---|---|---|---|
| Link | `link` | FK column in child (1:1, 1:N) or junction (M:N); parent side virtual | `relationId`, `side: parent\|child` |
| Lookup | `lookup` | virtual | `linkFieldId`, `targetFieldId` |
| Rollup | `rollup` | virtual | `linkFieldId`, `targetFieldId`, `fn: count\|count_distinct\|sum\|avg\|min\|max\|earliest\|latest\|concat` |
| Formula | `formula` | virtual | `expr`, `resultType` (inferred), display format |

## 3. System fields (hidden by default, can be shown)

| Field | Column | Type |
|---|---|---|
| Row ID | `id` | `bigint` |
| Created at | `created_at` | `timestamptz` |
| Created by | `created_by` | `uuid` (rendered as user) |
| Updated at | `updated_at` | `timestamptz` |
| Updated by | `updated_by` | `uuid` (rendered as user) |

System fields are read-only, cannot be deleted, can be renamed (display only), filtered, sorted, grouped.

## 4. Constraints

| Constraint | Implementation | Applies to |
|---|---|---|
| Required | `NOT NULL` (+ default required if table has rows, or dry-run reports null count) | stored types |
| Unique | `UNIQUE` index (dry-run reports duplicates) | text, integer, decimal, float, currency, percent, rating, date/time, uuid, email, url, phone, inet |
| Default | `DEFAULT <literal>`; dynamic: `now()`, `current_date`, `uuidv7()`, current user (app-level) | stored types |

The T1.9 physical boundary uses typed `ddl.DataType`/`ColumnSpec` and
`DefaultValue`, not raw `PGType` output or user expressions. See
[data model transaction contract](05-data-model.md#t19-caller-transaction-contract-d-095d-096).
Future codecs/services must construct an allowlisted descriptor and enforce
UI-specific constraint eligibility/options. Removing required/unique/default
does not rewrite values. UNIQUE allows multiple NULLs; SET NOT NULL does not
repair existing NULLs; changing a default affects future writes only.
Physical DROP is purge; field soft deletion retains the column. T1.9 itself
added no field API; T1.14 added field management, and T2.1 adds the typed
user-reference and autonumber sequence descriptors. Type conversion remains
T2.12; relation policy is outside these stored codecs.

## 5. Type change (FR-F5)

1. Owner picks new type → `POST /fields/{id}/convert:dry-run`.
2. Server checks each non-null value with `pg_input_is_valid(c::text, '<newtype>')` (PG16+) or a type-specific converter (e.g. multi_select → single_select takes first element; checkbox → text 'true'/'false'). Returns `{total, convertible, failing, samples:[{id, value}]}`.
3. User chooses **Abort** or **Convert, set failing values to NULL**.
4. Execute in one tx: `ALTER TABLE … ALTER COLUMN c TYPE newtype USING CASE WHEN pg_input_is_valid(c::text,'newtype') THEN c::text::newtype END` (or specialised USING expression). Row history records a single `type_change` event per table (not per row).
5. Dependent formulas/lookups/rollups are re-type-checked; broken ones are flagged (`options.error`) and render `#ERROR` rather than blocking the change.

Conversions to/from `link`, `lookup`, `rollup`, `formula`, `attachment` are not allowed (create a new field instead).

## 6. Value codec contract (Go)

```go
type Codec interface {
    Kind() Kind
    Bind(options json.RawMessage) (Field, error)
}
type Field interface {
    Kind() Kind
    Options() json.RawMessage
    ReadOnly() bool
    Column() Column
    Constraints() Constraints
    Decode(raw json.RawMessage) (any, error)
    Encode(value any) (json.RawMessage, error)
    ParsePaste(text string, env Env) (any, error)
    FormatText(value any, env Env) (string, error)
    DefaultValue(raw json.RawMessage) (ddl.DefaultValue, error)
    FilterOps() []Op
    Aggregates() []Agg
}
```

T1.13 implemented this contract in [internal/codec](../internal/codec/) for the
12 batch-1 writable kinds and all five system kinds (D-113..D-121). T2.1 adds
the remaining stored kinds and their typed PostgreSQL descriptors. `Column`
contains a typed `ddl.DataType`, forced NOT NULL/default (checkbox), system
marker and typed range/reference/autonumber metadata. There is no `PGType`
string or SQL interpolation.

### Batch-1 normalization and wire contract

- Options are strict objects: unknown/duplicate members and wrong types fail.
  Absent/null options mean `{}`. Defaults and exact bounds are D-114:
  text maxLength 1..10000; long text plain only, max 100000 code points;
  integer int with small/int/big ranges; decimal precision 1..38 (default 18),
  scale 0..precision (default min(2, precision)); float double by default,
  optional displayDecimals 0..10. thousandSep defaults true on numeric kinds.
  Checkbox is forced NOT NULL DEFAULT false.
- Bigint (including row id) is a canonical decimal **string**, decimal is a
  fixed-scale string, small/int are JSON numbers, floats finite JSON numbers.
  Real rounds to float32 and uses its shortest round-trip decimal.
  Unsafe integer number inputs and decimal number tokens with more than 15
  significant digits fail; use strings for exact larger values. Excess nonzero
  fractional digits fail rather than silently rounding numeric values.
- Empty strings become NULL; multi-select `[]` becomes NULL. Checkbox
  null/empty becomes false. Text is UTF-8/no lone surrogates or NUL; tab is
  allowed, LF only in long text. Paste normalizes line breaks/grouping/full-width
  numeric forms; strict Decode is separate. See D-116 for accepted boolean text.
- Date is `YYYY-MM-DD`, years 0001..9999, real proleptic Gregorian dates.
  Datetime is a UTC RFC3339 instant with seconds and up to six fraction digits;
  input requires an explicit offset. Offset-free paste requires the viewer's
  effective zone or fixedTz; DST gaps fail, overlaps choose the earlier instant.
  Copy includes the full time, microseconds and offset (including historical
  offset seconds) so it re-pastes exactly. Display is not storage.
  At UTC range boundaries, copy uses +00:00 if the display zone would produce
  local year 0000/10000 (D-122), keeping the bounded paste parser reversible.
  Fixed zones additionally pass PG `pg_timezone_names` validation in the T1.14
  field service (D-128).
- At most 500 choices, ids `[A-Za-z0-9_-]{1,64}`, trimmed 1..100-code-point
  labels, no controls/comma, unique ids and lower-case labels; closed 13-color
  palette (D-118). Values are exact labels; multi-select is in choice order.
  Paste matches exact then lower-case and de-duplicates; never creates options.
  Stored removed labels survive Encode and render neutrally.
- Email is conservative ASCII dot-atom/domain validation (254 bytes maximum).
  URL is absolute http(s), at most 2048 code points, ASCII LDH host or valid
  bracketed IPv6, bounded port, no userinfo/whitespace/controls/backslashes or
  malformed percent escapes. Paste may prepend https://; dangerous schemes
  fail. Frontend links revalidate, escape mailto address delimiters and use
  noopener/noreferrer/nofollow; text is never interpreted as HTML.
- System Decode/paste/defaults fail read_only, with no constraint mutation or
  editor. Authorship UUIDs are lowercase; a caller resolves user labels without
  reusing the owner-only sharing directory. Unknown users are localized.

Codec unit/vector/PG tests are lower-layer evidence; browser acceptance for
field creation, row editing, persistence and grid integration is recorded by
the linked acceptance tests.

### Batch-2 normalization and wire contract (D-159..D-162)

- `time` uses `HH:MM:SS[.ffffff]`; options select 24-hour or 12-hour display.
  `duration` uses signed ISO-8601 time-only `PT#H#M#S`, with seconds and
  minutes below 60 and no calendar components. Copy preserves canonical wire
  values.
- `currency` is fixed-scale `numeric(18,scale)` (scale 0..18), with an ISO
  4217 code stored in field options. Creation snapshots the current app default
  when the code is omitted; later setting changes do not affect that field.
  `percent` is a fraction in `numeric(7,4)` and formats with `Intl`.
  `rating` is `smallint`, max 1..10 (default 5), and carries a typed range
  check; `phone` is bounded to 64 code points and the D-160 loose character
  set.
- `uuid` and `user` values are lowercase UUID strings. UUID generation is a
  typed PostgreSQL v7 default when configured. `json` uses JSONB, canonical
  key ordering and 64 KiB pre/post-encoding and depth-128 bounds; duplicate
  keys and invalid Unicode escapes fail. `inet` and `cidr` use canonical
  address/network strings.
- `text_array`, `int_array` and `multi_user` contain at most 1,000 items;
  text items are at most 10,000 code points. Bigint array values and
  autonumber use decimal strings. Empty arrays become SQL NULL. A single-user
  single-user fields are nullable unless marked required and reference
  `meta.users`; multi-user IDs are resolved
  through the table-scoped directory routes documented in [09-api.md](09-api.md).
- Autonumber uses a typed per-field sequence, ascending row-ID backfill,
  `nextval` default and column ownership. It is read-only in row editors.
  D-161 keeps the existing owner-only sharing directory unchanged while
  allowing table writers to search active users and table readers to resolve
  only submitted user IDs.

### T1.14 field service rules (D-126..D-129)

- Required/unique/default use the codec's `Constraints()`; ineligible
  settings fail `constraint_unsupported`. Required on NULL cells reports
  `ERR_REQUIRED_HAS_EMPTY{count}` unless resent with `fillEmpty` plus a default
  (one parameterized fill, then `SET NOT NULL`). Unique with duplicates reports
  `ERR_UNIQUE_HAS_DUPLICATES{count}`. Defaults are typed literals only.
- Option edits that would change the native column fail `storage_change`
  (conversion is T2.12). Select label changes, matched by choice id, rewrite
  stored values and the default in the same transaction. Removed choices keep
  stored labels.
- Soft delete keeps column and data, drops NOT NULL (except checkbox) and the
  physical default, and reassigns a deleted display field to the first live
  eligible field (`ERR_DISPLAY_FIELD_REQUIRED` if none). Checkbox and system
  kinds are not display-eligible.
