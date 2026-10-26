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
| Date | `date` | `date` | `format` | |
| Time | `time` | `time` | `format: 24h\|12h` | |
| Date & time | `datetime` | `timestamptz` | `format`, `timezone: user\|fixed`, `fixedTz?` | stored UTC; displayed in the viewing user's timezone (default Asia/Shanghai) unless field has a fixed timezone |
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
| Attachment | `attachment` | `jsonb` | `accept: mime list`, `maxFiles` | array of `{id,name,mime,size,thumb}`; files in `meta.attachments` |
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
| Unique | `UNIQUE` index (dry-run reports duplicates) | text, numeric, date/time, uuid, email, url, phone, inet |
| Default | `DEFAULT <literal>`; dynamic: `now()`, `current_date`, `uuidv7()`, current user (app-level) | stored types |

The T1.9 physical boundary uses typed `ddl.DataType`/`ColumnSpec` and
`DefaultValue`, not raw `PGType` output or user expressions. See
[data model transaction contract](05-data-model.md#t19-caller-transaction-contract-d-095d-096).
Future codecs/services must construct an allowlisted descriptor and enforce
UI-specific constraint eligibility/options. Removing required/unique/default
does not rewrite values. UNIQUE allows multiple NULLs; SET NOT NULL does not
repair existing NULLs; changing a default affects future writes only.
Physical DROP is purge; field soft deletion retains the column. No field API,
type conversion, FK/relation policy or auto-number sequence orchestration is
implemented by T1.9.

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
    PGType(opts Options) string                      // e.g. "numeric(12,2)"
    Decode(raw json.RawMessage, opts Options) (any, error) // API JSON → Go value (validated)
    Encode(v any, opts Options) (any, error)        // DB value → API JSON
    ParsePaste(s string, opts Options) (any, error) // clipboard text → value (lenient)
    FormatText(v any, opts Options, loc Locale) string // for CSV export / copy
    FilterOps() []Op                                // allowed filter operators
    Aggregates() []Agg                              // allowed footer/rollup aggregates
}
```
