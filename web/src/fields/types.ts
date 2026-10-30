// Shared types for the batch-1 field registry (D-121). Wire values follow
// D-115 exactly; the server codec in internal/codec is authoritative.

export type Kind =
  | 'text'
  | 'long_text'
  | 'integer'
  | 'decimal'
  | 'float'
  | 'checkbox'
  | 'date'
  | 'datetime'
  | 'single_select'
  | 'multi_select'
  | 'time'
  | 'duration'
  | 'currency'
  | 'percent'
  | 'rating'
  | 'phone'
  | 'uuid'
  | 'json'
  | 'inet'
  | 'cidr'
  | 'text_array'
  | 'int_array'
  | 'user'
  | 'multi_user'
  | 'autonumber'
  | 'attachment'
  | 'email'
  | 'url'
  | 'lookup'
  | 'rollup'
  | 'formula'
  | 'id'
  | 'created_at'
  | 'created_by'
  | 'updated_at'
  | 'updated_by'

export type Reason =
  | 'unknown_kind'
  | 'invalid_options'
  | 'invalid_type'
  | 'invalid_text'
  | 'too_long'
  | 'invalid_integer'
  | 'invalid_decimal'
  | 'invalid_number'
  | 'imprecise_number'
  | 'out_of_range'
  | 'too_many_decimals'
  | 'invalid_boolean'
  | 'invalid_date'
  | 'invalid_datetime'
  | 'nonexistent_local_time'
  | 'timezone_required'
  | 'unknown_choice'
  | 'duplicate_choice'
  | 'invalid_email'
  | 'invalid_url'
  | 'url_scheme'
  | 'invalid_time'
  | 'invalid_duration'
  | 'invalid_rating'
  | 'invalid_phone'
  | 'invalid_uuid'
  | 'invalid_json'
  | 'invalid_ip'
  | 'invalid_cidr'
  | 'invalid_array'
  | 'read_only'

export type Op =
  | 'is'
  | 'is_not'
  | 'contains'
  | 'not_contains'
  | 'starts_with'
  | 'ends_with'
  | 'is_empty'
  | 'is_not_empty'
  | 'eq'
  | 'neq'
  | 'lt'
  | 'lte'
  | 'gt'
  | 'gte'
  | 'between'
  | 'before'
  | 'after'
  | 'on_or_before'
  | 'on_or_after'
  | 'is_within'
  | 'is_checked'
  | 'is_not_checked'
  | 'is_any_of'
  | 'is_none_of'
  | 'has_any_of'
  | 'has_all_of'
  | 'has_none_of'
  | 'is_exactly'
  | 'has_key'
  | 'path_equals'
  | 'contained_in'

export type Agg =
  | 'none'
  | 'count'
  | 'count_empty'
  | 'count_filled'
  | 'unique'
  | 'sum'
  | 'avg'
  | 'min'
  | 'max'
  | 'earliest'
  | 'latest'
  | 'percent_checked'

/** Canonical JSON wire value (D-115); null is SQL NULL. */
export type JsonValue =
  null | string | number | boolean | JsonValue[] | { [key: string]: JsonValue }

export type WireValue = JsonValue

export function isWireValue(value: unknown, depth = 0): value is WireValue {
  if (depth > 128) return false
  if (value === null || typeof value === 'string' || typeof value === 'boolean')
    return true
  if (typeof value === 'number') return Number.isFinite(value)
  if (Array.isArray(value))
    return value.every((item) => isWireValue(item, depth + 1))
  if (
    typeof value === 'object' &&
    Object.getPrototypeOf(value) === Object.prototype
  )
    return Object.values(value).every((item) => isWireValue(item, depth + 1))
  return false
}

export type Result<T> =
  { ok: true; value: T } | { ok: false; reason: Reason; option?: string }

export interface FieldEnv {
  /** Viewer's effective profile/app-default IANA timezone, D-117. */
  timeZone?: string
  /** UI locale for display formatting only. */
  locale?: string
}

/** Rendering/editing family used by FieldRenderer/FieldEditor. */
export type Presentation =
  | 'text'
  | 'multiline'
  | 'number'
  | 'checkbox'
  | 'date'
  | 'datetime'
  | 'select'
  | 'email'
  | 'url'
  | 'user'
  | 'attachment'

export interface Constraints {
  required: boolean
  unique: boolean
  default: boolean
}

export interface Choice {
  id: string
  label: string
  color: string
}

/** A kind definition; `O` is the normalized options shape. */
export interface FieldKind<O = unknown> {
  kind: Kind
  readOnly: boolean
  presentation: Presentation
  constraints: Constraints
  normalizeOptions(raw: unknown): Result<O>
  /** Strict validation of an API/editor value, mirroring Go Decode+Encode. */
  normalize(value: unknown, options: O): Result<WireValue>
  parsePaste(text: string, options: O, env: FieldEnv): Result<WireValue>
  formatCopy(value: WireValue, options: O, env: FieldEnv): string
  formatDisplay(value: WireValue, options: O, env: FieldEnv): string
  filterOps(options: O): Op[]
  aggregates(options: O): Agg[]
}

export const ok = <T>(value: T): Result<T> => ({ ok: true, value })
export const fail = <T = never>(reason: Reason, option?: string): Result<T> =>
  option === undefined ? { ok: false, reason } : { ok: false, reason, option }
