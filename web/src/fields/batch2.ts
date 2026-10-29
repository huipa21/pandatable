import {
  codePoints,
  isPlainObject,
  OptionSet,
  trimSpace,
  validString,
} from './common'
import {
  BASIC_AGGS,
  NUMERIC_AGGS,
  NUMERIC_OPS,
  TEMPORAL_AGGS,
  TEMPORAL_OPS,
  TEXT_OPS,
  USER_OPS,
} from './capabilities'
import { normalizeNumberText, parseDecimal } from './number'
import {
  fail,
  ok,
  type Agg,
  type FieldKind,
  type JsonValue,
  type Kind,
  type Op,
  type Result,
  type WireValue,
} from './types'

const STRICT_INTEGER = /^-?(0|[1-9][0-9]*)$/
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/
const PHONE = /^[+0-9(). xX#-]+$/
const TIME = /^([0-9]{2}):([0-9]{2}):([0-9]{2})(\.[0-9]{1,6})?$/
const DURATION =
  /^(-?)PT(?:(0|[1-9][0-9]*)H)?(?:(0|[1-9][0-9]*)M)?(?:(0|[1-9][0-9]*)(\.[0-9]{1,6})?S)?$/
const INT64_MIN = -(1n << 63n)
const INT64_MAX = (1n << 63n) - 1n
const MAX_JSON_BYTES = 64 * 1024
const MAX_ARRAY_ITEMS = 1000
const MAX_ARRAY_TEXT_LENGTH = 10000

interface CurrencyOptions {
  currency: string
  scale: number
}

interface PercentOptions {
  displayDecimals: number
}

interface RatingOptions {
  max: number
  icon: 'star'
}

interface FormatOptions {
  format: string
}

interface UUIDOptions {
  autoGenerate: boolean
}

interface AutoNumberOptions {
  prefix: string
  padding: number
}

function isString(value: unknown): value is string {
  return typeof value === 'string'
}

function fixedDecimal(
  value: unknown,
  precision: number,
  scale: number,
): Result<WireValue> {
  if (value === null || value === '') return ok(null)
  if (
    !isString(value) ||
    (!STRICT_INTEGER.test(value.replace(/\.[0-9]+$/, '')) &&
      !/^-?(0|[1-9][0-9]*)(\.[0-9]+)?$/.test(value))
  )
    return fail('invalid_decimal')
  const text = value
  const parsed = parseDecimal(text)
  if (!parsed) return fail('invalid_decimal')
  if (Math.max(0, -parsed.exp) > scale) return fail('too_many_decimals')
  const digits = parsed.digits === '' ? '0' : parsed.digits
  const integerPlaces = Math.max(0, digits.length + parsed.exp)
  if (integerPlaces > precision - scale) return fail('out_of_range')
  let whole = '0'
  let fraction = ''
  if (digits !== '0') {
    if (parsed.exp >= 0) whole = digits + '0'.repeat(parsed.exp)
    else {
      const position = digits.length + parsed.exp
      whole = position > 0 ? digits.slice(0, position) : '0'
      fraction =
        position > 0 ? digits.slice(position) : '0'.repeat(-position) + digits
    }
  }
  fraction += '0'.repeat(scale - fraction.length)
  return ok(
    `${parsed.negative && digits !== '0' ? '-' : ''}${whole}${scale ? `.${fraction}` : ''}`,
  )
}

function formatIntlDecimal(formatter: Intl.NumberFormat, value: string) {
  return Reflect.apply(formatter.format, formatter, [value])
}

function parseDecimalPaste(text: string, precision: number, scale: number) {
  const normalized = normalizeNumberText(text)
  if (!normalized) return ok(null)
  const signless = normalized.replace(/^[+-]/, '')
  if (!/^(?:[0-9]+\.?[0-9]*|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$/.test(signless))
    return fail('invalid_decimal')
  const parsed = parseDecimal(normalized)
  if (!parsed) return fail('invalid_decimal')
  if (Math.max(0, -parsed.exp) > scale) return fail('too_many_decimals')
  const exact =
    parsed.digits === ''
      ? '0'
      : parsed.exp >= 0
        ? `${parsed.negative ? '-' : ''}${parsed.digits}${'0'.repeat(parsed.exp)}`
        : (() => {
            const position = parsed.digits.length + parsed.exp
            const whole = position > 0 ? parsed.digits.slice(0, position) : '0'
            const fraction =
              position > 0
                ? parsed.digits.slice(position)
                : '0'.repeat(-position) + parsed.digits
            return `${parsed.negative ? '-' : ''}${whole}.${fraction}`
          })()
  return fixedDecimal(exact, precision, scale)
}

function parseTime(text: string): string | null {
  const match = TIME.exec(text)
  if (!match) return null
  const hour = Number(match[1])
  const minute = Number(match[2])
  const second = Number(match[3])
  if (hour > 23 || minute > 59 || second > 59) return null
  const fraction = (match[4]?.slice(1) ?? '').padEnd(6, '0').replace(/0+$/, '')
  return `${match[1]}:${match[2]}:${match[3]}${fraction ? `.${fraction}` : ''}`
}

function formatDuration(text: string) {
  const match = DURATION.exec(text)
  if (!match || (!match[2] && !match[3] && !match[4])) return null
  const hours = BigInt(match[2] ?? '0')
  const minutes = BigInt(match[3] ?? '0')
  const seconds = BigInt(match[4] ?? '0')
  if (minutes > 59n || seconds > 59n) return null
  const micros = BigInt((match[5]?.slice(1) ?? '').padEnd(6, '0') || '0')
  let total =
    hours * 3_600_000_000n +
    minutes * 60_000_000n +
    seconds * 1_000_000n +
    micros
  if (match[1] === '-') total = -total
  if (total < INT64_MIN || total > INT64_MAX) return null
  const negative = total < 0
  let magnitude = negative ? -total : total
  const h = magnitude / 3_600_000_000n
  magnitude %= 3_600_000_000n
  const m = magnitude / 60_000_000n
  magnitude %= 60_000_000n
  const s = magnitude / 1_000_000n
  const fraction = String(magnitude % 1_000_000n)
    .padStart(6, '0')
    .replace(/0+$/, '')
  let result = negative ? '-PT' : 'PT'
  if (h) result += `${h}H`
  if (m) result += `${m}M`
  if (fraction) result += `${s}.${fraction}S`
  else if (s || (!h && !m)) result += `${s}S`
  return result
}

function canonicalJSON(value: unknown, depth = 0): Result<JsonValue> {
  if (depth > 128) return fail('invalid_json')
  if (value === null || typeof value === 'string' || typeof value === 'boolean')
    return ok(value)
  if (typeof value === 'number')
    return Number.isFinite(value) ? ok(value) : fail('invalid_json')
  if (Array.isArray(value)) {
    const items: JsonValue[] = []
    for (const item of value) {
      const normalized = canonicalJSON(item, depth + 1)
      if (!normalized.ok) return normalized
      items.push(normalized.value)
    }
    return ok(items)
  }
  if (isPlainObject(value)) {
    const object: { [key: string]: JsonValue } = {}
    for (const key of Object.keys(value).sort()) {
      const normalized = canonicalJSON(value[key], depth + 1)
      if (!normalized.ok) return normalized
      object[key] = normalized.value
    }
    return ok(object)
  }
  return fail('invalid_json')
}

function jsonSize(value: JsonValue) {
  return new TextEncoder().encode(JSON.stringify(value)).length
}

function parseIPv4(text: string): bigint | null {
  const parts = text.split('.')
  if (parts.length !== 4) return null
  let result = 0n
  for (const part of parts) {
    if (!/^(0|[1-9][0-9]{0,2})$/.test(part)) return null
    const octet = Number(part)
    if (octet > 255) return null
    result = (result << 8n) | BigInt(octet)
  }
  return result
}

function formatIPv4(value: bigint) {
  return [24n, 16n, 8n, 0n]
    .map((shift) => Number((value >> shift) & 255n))
    .join('.')
}

function parseIPv6(text: string): bigint | null {
  if (!text || text.includes('%') || (text.match(/::/g) ?? []).length > 1)
    return null
  let source = text.toLowerCase()
  if (source.includes('.')) {
    const split = source.lastIndexOf(':')
    const ipv4 = parseIPv4(source.slice(split + 1))
    if (ipv4 === null || split < 0) return null
    source = `${source.slice(0, split)}:${((ipv4 >> 16n) & 0xffffn).toString(16)}:${(ipv4 & 0xffffn).toString(16)}`
  }
  const halves = source.split('::')
  const left = halves[0] ? halves[0]!.split(':') : []
  const right = halves[1] ? halves[1]!.split(':') : []
  if (halves.length === 1 && left.length !== 8) return null
  const zeros = 8 - left.length - right.length
  if ((halves.length === 2 && zeros < 1) || zeros < 0) return null
  const groups = [...left, ...Array(zeros).fill('0'), ...right]
  if (
    groups.length !== 8 ||
    groups.some((group) => !/^[0-9a-f]{1,4}$/.test(group))
  )
    return null
  return groups.reduce(
    (result, group) => (result << 16n) | BigInt(`0x${group}`),
    0n,
  )
}

function formatIPv6(value: bigint) {
  const groups = Array.from({ length: 8 }, (_, index) =>
    Number((value >> BigInt((7 - index) * 16)) & 0xffffn),
  )
  let start = -1
  let length = 1
  for (let index = 0; index < groups.length;) {
    if (groups[index] !== 0) {
      index++
      continue
    }
    let end = index
    while (end < groups.length && groups[end] === 0) end++
    if (end - index > length) {
      start = index
      length = end - index
    }
    index = end
  }
  const values = groups.map((group) => group.toString(16))
  if (start < 0) return values.join(':')
  const left = values.slice(0, start).join(':')
  const right = values.slice(start + length).join(':')
  return `${left}::${right}`
}

function canonicalIP(text: string, cidr: boolean): string | null {
  if (cidr) {
    const slash = text.lastIndexOf('/')
    if (slash < 0 || text.indexOf('/') !== slash) return null
    const address = text.slice(0, slash)
    const prefix = text.slice(slash + 1)
    if (!/^(0|[1-9][0-9]*)$/.test(prefix)) return null
    const ipv4 = parseIPv4(address)
    if (ipv4 !== null) {
      const bits = Number(prefix)
      if (bits > 32) return null
      const mask =
        bits === 0 ? 0n : ((1n << BigInt(bits)) - 1n) << BigInt(32 - bits)
      if ((ipv4 & mask) !== ipv4) return null
      return `${formatIPv4(ipv4)}/${bits}`
    }
    const ipv6 = parseIPv6(address)
    const bits = Number(prefix)
    if (ipv6 === null || bits > 128) return null
    const mask =
      bits === 0 ? 0n : ((1n << BigInt(bits)) - 1n) << BigInt(128 - bits)
    if ((ipv6 & mask) !== ipv6) return null
    return `${formatIPv6(ipv6)}/${bits}`
  }
  const ipv4 = parseIPv4(text)
  if (ipv4 !== null) return formatIPv4(ipv4)
  const ipv6 = parseIPv6(text)
  return ipv6 === null ? null : formatIPv6(ipv6)
}

function simpleKind<O>(
  kind: Kind,
  presentation: FieldKind<O>['presentation'],
  constraints: FieldKind<O>['constraints'],
  ops: readonly Op[],
  aggs: readonly Agg[],
  normalizeOptions: FieldKind<O>['normalizeOptions'],
  normalize: FieldKind<O>['normalize'],
  parsePaste: FieldKind<O>['parsePaste'],
  formatCopy: FieldKind<O>['formatCopy'],
  formatDisplay: FieldKind<O>['formatDisplay'],
): FieldKind<O> {
  return {
    kind,
    readOnly: kind === 'autonumber',
    presentation,
    constraints,
    normalizeOptions,
    normalize,
    parsePaste,
    formatCopy,
    formatDisplay,
    filterOps: () => [...ops],
    aggregates: () => [...aggs],
  }
}

const noOptions = (raw: unknown) => {
  const set = OptionSet.parse(raw)
  if (!set.ok) return set
  const done = set.value.done()
  return done.ok ? ok({}) : done
}

export const timeKind: FieldKind<FormatOptions> = simpleKind(
  'time',
  'text',
  { required: true, unique: true, default: true },
  TEMPORAL_OPS,
  TEMPORAL_AGGS,
  (raw) => {
    const set = OptionSet.parse(raw)
    if (!set.ok) return set
    const format = set.value.enum<'24h' | '12h'>('format', '24h', [
      '24h',
      '12h',
    ])
    if (!format.ok) return format
    const done = set.value.done()
    return done.ok ? ok({ format: format.value }) : done
  },
  (value) => {
    if (value === null || value === '') return ok(null)
    if (!isString(value)) return fail('invalid_type')
    const normalized = parseTime(value)
    return normalized ? ok(normalized) : fail('invalid_time')
  },
  (text) => {
    const value = trimSpace(text)
    if (!value) return ok(null)
    const direct = parseTime(value)
    if (direct) return ok(direct)
    const match =
      /^([0-9]{1,2}):([0-9]{2})(?::([0-9]{2}))?\s*([AaPp][Mm])$/.exec(value)
    if (!match) return fail('invalid_time')
    let hour = Number(match[1])
    const minute = Number(match[2])
    const second = Number(match[3] ?? '0')
    if (hour < 1 || hour > 12 || minute > 59 || second > 59)
      return fail('invalid_time')
    hour = (hour % 12) + (match[4]!.toLowerCase() === 'pm' ? 12 : 0)
    return ok(
      `${String(hour).padStart(2, '0')}:${match[2]}:${String(second).padStart(2, '0')}`,
    )
  },
  (value) => (typeof value === 'string' ? value : ''),
  (value, options) => {
    if (typeof value !== 'string' || options.format === '24h')
      return typeof value === 'string' ? value : ''
    const match = TIME.exec(value)
    if (!match) return value
    const hour = Number(match[1])
    return `${String(hour % 12 || 12).padStart(2, '0')}:${match[2]}:${match[3]}${hour < 12 ? ' AM' : ' PM'}`
  },
)

export const durationKind: FieldKind<FormatOptions> = simpleKind(
  'duration',
  'text',
  { required: true, unique: false, default: true },
  NUMERIC_OPS,
  NUMERIC_AGGS,
  (raw) => {
    const set = OptionSet.parse(raw)
    if (!set.ok) return set
    const format = set.value.enum<'h:mm' | 'h:mm:ss' | 'human'>(
      'format',
      'h:mm',
      ['h:mm', 'h:mm:ss', 'human'],
    )
    if (!format.ok) return format
    const done = set.value.done()
    return done.ok ? ok({ format: format.value }) : done
  },
  (value) => {
    if (value === null || value === '') return ok(null)
    if (!isString(value)) return fail('invalid_type')
    const canonical = formatDuration(value)
    return canonical ? ok(canonical) : fail('invalid_duration')
  },
  (text) => {
    const value = trimSpace(text)
    if (!value) return ok(null)
    const canonical = formatDuration(value)
    return canonical ? ok(canonical) : fail('invalid_duration')
  },
  (value) => (typeof value === 'string' ? value : ''),
  (value, options) => {
    if (typeof value !== 'string') return ''
    const match = DURATION.exec(value)
    if (!match) return value
    const sign = match[1] ? '-' : ''
    const hours = BigInt(match[2] ?? '0')
    const minutes = BigInt(match[3] ?? '0')
    const seconds = BigInt(match[4] ?? '0')
    const fraction = match[5] ?? ''
    if (options.format === 'human')
      return (
        [
          hours && `${hours}h`,
          minutes && `${minutes}m`,
          (seconds || fraction) && `${seconds}${fraction} s`,
        ]
          .filter(Boolean)
          .join(' ') || '0s'
      )
    const total = hours * 3600n + minutes * 60n + seconds
    const h = total / 3600n
    const m = Number((total / 60n) % 60n)
    const s = Number(total % 60n)
    const secondsText = `${String(s).padStart(2, '0')}${fraction}`
    return options.format === 'h:mm:ss'
      ? `${sign}${h}:${String(m).padStart(2, '0')}:${secondsText}`
      : `${sign}${h}:${String(m).padStart(2, '0')}`
  },
)

export const currencyKind: FieldKind<CurrencyOptions> = simpleKind(
  'currency',
  'number',
  { required: true, unique: true, default: true },
  NUMERIC_OPS,
  NUMERIC_AGGS,
  (raw) => {
    const set = OptionSet.parse(raw)
    if (!set.ok) return set
    const currency = set.value.take('currency')
    if (
      currency !== undefined &&
      (typeof currency !== 'string' || !/^[A-Z]{3}$/.test(currency))
    )
      return fail('invalid_options', 'currency')
    const scale = set.value.integer('scale', 0, 18)
    if (!scale.ok) return scale
    const done = set.value.done()
    return done.ok
      ? ok({
          currency: typeof currency === 'string' ? currency : 'CNY',
          scale: scale.value ?? 2,
        })
      : done
  },
  (value, options) => fixedDecimal(value, 18, options.scale),
  (text, options) => parseDecimalPaste(text, 18, options.scale),
  (value) => (typeof value === 'string' ? value : ''),
  (value, options, env) =>
    typeof value === 'string'
      ? formatIntlDecimal(
          new Intl.NumberFormat(env.locale ?? 'en', {
            style: 'currency',
            currency: options.currency,
            minimumFractionDigits: options.scale,
            maximumFractionDigits: options.scale,
          }),
          value,
        )
      : '',
)

export const percentKind: FieldKind<PercentOptions> = simpleKind(
  'percent',
  'number',
  { required: true, unique: true, default: true },
  NUMERIC_OPS,
  NUMERIC_AGGS,
  (raw) => {
    const set = OptionSet.parse(raw)
    if (!set.ok) return set
    const decimals = set.value.integer('displayDecimals', 0, 4)
    if (!decimals.ok) return decimals
    const done = set.value.done()
    return done.ok ? ok({ displayDecimals: decimals.value ?? 2 }) : done
  },
  (value) => fixedDecimal(value, 7, 4),
  (text) => parseDecimalPaste(text, 7, 4),
  (value) => (typeof value === 'string' ? value : ''),
  (value, options, env) =>
    typeof value === 'string'
      ? formatIntlDecimal(
          new Intl.NumberFormat(env.locale ?? 'en', {
            style: 'percent',
            minimumFractionDigits: options.displayDecimals,
            maximumFractionDigits: options.displayDecimals,
          }),
          value,
        )
      : '',
)

export const ratingKind: FieldKind<RatingOptions> = simpleKind(
  'rating',
  'number',
  { required: true, unique: true, default: true },
  NUMERIC_OPS,
  NUMERIC_AGGS,
  (raw) => {
    const set = OptionSet.parse(raw)
    if (!set.ok) return set
    const max = set.value.integer('max', 1, 10)
    if (!max.ok) return max
    const icon = set.value.enum<'star'>('icon', 'star', ['star'])
    if (!icon.ok) return icon
    const done = set.value.done()
    return done.ok ? ok({ max: max.value ?? 5, icon: icon.value }) : done
  },
  (value, options) => {
    if (value === null || value === '') return ok(null)
    const text =
      typeof value === 'string'
        ? value
        : typeof value === 'number' && Number.isSafeInteger(value)
          ? String(value)
          : ''
    if (!/^(0|[1-9][0-9]*)$/.test(text)) return fail('invalid_type')
    const number = Number(text)
    return number <= options.max ? ok(number) : fail('out_of_range')
  },
  (text, options) => {
    const value = trimSpace(normalizeNumberText(text))
    if (!value) return ok(null)
    if (!/^(0|[1-9][0-9]*)$/.test(value)) return fail('invalid_rating')
    const number = Number(value)
    return number <= options.max ? ok(number) : fail('out_of_range')
  },
  (value) => (typeof value === 'number' ? String(value) : ''),
  (value) => (typeof value === 'number' ? '★'.repeat(value) : ''),
)

export const phoneKind: FieldKind<Record<string, never>> = simpleKind(
  'phone',
  'text',
  { required: true, unique: true, default: true },
  TEXT_OPS,
  BASIC_AGGS,
  noOptions,
  (value) => {
    if (value === null || value === '') return ok(null)
    if (!isString(value)) return fail('invalid_type')
    const text = trimSpace(value)
    if (!text) return ok(null)
    return validString(text, false, false) &&
      codePoints(text) <= 64 &&
      PHONE.test(text) &&
      /[0-9]/.test(text)
      ? ok(text)
      : fail('invalid_phone')
  },
  (text) => phoneKind.normalize(text, {}),
  (value) => (typeof value === 'string' ? value : ''),
  (value) => (typeof value === 'string' ? value : ''),
)

export const uuidKind: FieldKind<UUIDOptions> = simpleKind(
  'uuid',
  'text',
  { required: true, unique: true, default: true },
  ['is', 'is_not', 'is_empty', 'is_not_empty'],
  BASIC_AGGS,
  (raw) => {
    const set = OptionSet.parse(raw)
    if (!set.ok) return set
    const autoGenerate = set.value.boolean('autoGenerate', false)
    if (!autoGenerate.ok) return autoGenerate
    const done = set.value.done()
    return done.ok ? ok({ autoGenerate: autoGenerate.value }) : done
  },
  (value) => {
    if (value === null || value === '') return ok(null)
    return typeof value === 'string' && UUID.test(value)
      ? ok(value)
      : fail('invalid_uuid')
  },
  (text) => uuidKind.normalize(text, { autoGenerate: false }),
  (value) => (typeof value === 'string' ? value : ''),
  (value) => (typeof value === 'string' ? value : ''),
)

export const jsonKind: FieldKind<Record<string, never>> = simpleKind(
  'json',
  'multiline',
  { required: true, unique: false, default: true },
  ['is', 'is_not', 'is_empty', 'is_not_empty'],
  BASIC_AGGS,
  noOptions,
  (value) => {
    if (value === null) return ok(null)
    const normalized = canonicalJSON(value)
    if (!normalized.ok) return normalized
    return jsonSize(normalized.value) <= MAX_JSON_BYTES
      ? ok(normalized.value)
      : fail('too_long')
  },
  (text) => {
    if (trimSpace(text) === '') return ok(null)
    let value: unknown
    try {
      value = JSON.parse(text)
    } catch (error) {
      if (error instanceof SyntaxError) return fail('invalid_json')
      throw error
    }
    return jsonKind.normalize(value, {})
  },
  (value) => JSON.stringify(value ?? null),
  (value) => JSON.stringify(value ?? null),
)

function ipKind(cidr: boolean): FieldKind<Record<string, never>> {
  const kind = cidr ? 'cidr' : 'inet'
  const normalize = (value: unknown): Result<WireValue> => {
    if (value === null || value === '') return ok(null)
    if (!isString(value)) return fail('invalid_type')
    const canonical = canonicalIP(value, cidr)
    return canonical
      ? ok(canonical)
      : fail(cidr ? 'invalid_cidr' : 'invalid_ip')
  }
  return simpleKind(
    kind,
    'text',
    { required: true, unique: !cidr, default: true },
    ['is', 'is_not', 'is_empty', 'is_not_empty'],
    BASIC_AGGS,
    noOptions,
    normalize,
    (text) => normalize(trimSpace(text)),
    (value) => (typeof value === 'string' ? value : ''),
    (value) => (typeof value === 'string' ? value : ''),
  )
}

export const inetKind = ipKind(false)
export const cidrKind = ipKind(true)

function arrayKind(
  kind: 'text_array' | 'int_array',
): FieldKind<Record<string, never>> {
  const textArray = kind === 'text_array'
  const normalize = (value: unknown): Result<WireValue> => {
    if (value === null) return ok(null)
    if (!Array.isArray(value)) return fail('invalid_array')
    if (value.length === 0) return ok(null)
    if (value.length > MAX_ARRAY_ITEMS) return fail('too_long')
    if (textArray) {
      for (const item of value)
        if (
          typeof item !== 'string' ||
          !validString(item, true, false) ||
          codePoints(item) > MAX_ARRAY_TEXT_LENGTH
        )
          return fail('invalid_array')
      const items: string[] = []
      for (const item of value) {
        if (typeof item !== 'string') return fail('invalid_array')
        items.push(item)
      }
      return ok(items)
    }
    const result: string[] = []
    for (const item of value) {
      if (typeof item !== 'string' || !STRICT_INTEGER.test(item))
        return fail('invalid_array')
      const number = BigInt(item)
      if (number < INT64_MIN || number > INT64_MAX) return fail('out_of_range')
      result.push(number.toString())
    }
    return ok(result)
  }
  return simpleKind(
    kind,
    'text',
    { required: true, unique: false, default: true },
    ['has_any_of', 'has_all_of', 'has_none_of', 'is_empty', 'is_not_empty'],
    BASIC_AGGS,
    noOptions,
    normalize,
    (text) => {
      const input = trimSpace(text)
      if (!input) return ok(null)
      return normalize(
        input
          .split(',')
          .map((item) => item.trim())
          .map((item) => (textArray ? item : item)),
      )
    },
    (value) => (Array.isArray(value) ? value.join(', ') : ''),
    (value) => (Array.isArray(value) ? value.join(', ') : ''),
  )
}

export const textArrayKind = arrayKind('text_array')
export const intArrayKind = arrayKind('int_array')

export const userKind: FieldKind<Record<string, never>> = simpleKind(
  'user',
  'user',
  { required: true, unique: false, default: true },
  USER_OPS,
  BASIC_AGGS,
  noOptions,
  (value) => {
    if (value === null || value === '') return ok(null)
    return typeof value === 'string' && UUID.test(value)
      ? ok(value)
      : fail('invalid_uuid')
  },
  (text) => userKind.normalize(trimSpace(text), {}),
  (value) => (typeof value === 'string' ? value : ''),
  (value) => (typeof value === 'string' ? value : ''),
)

export const multiUserKind: FieldKind<Record<string, never>> = simpleKind(
  'multi_user',
  'user',
  { required: true, unique: false, default: true },
  ['has_any_of', 'has_all_of', 'has_none_of', 'is_empty', 'is_not_empty'],
  BASIC_AGGS,
  noOptions,
  (value) => {
    if (value === null) return ok(null)
    if (!Array.isArray(value)) return fail('invalid_array')
    if (value.length === 0) return ok(null)
    if (value.length > MAX_ARRAY_ITEMS) return fail('too_long')
    if (!value.every((item) => typeof item === 'string' && UUID.test(item)))
      return fail('invalid_array')
    if (new Set(value).size !== value.length) return fail('duplicate_choice')
    return ok(value)
  },
  (text) => {
    const value = trimSpace(text)
    if (!value) return ok(null)
    return multiUserKind.normalize(
      value.split(',').map((item) => trimSpace(item)),
      {},
    )
  },
  (value) => (Array.isArray(value) ? value.join(', ') : ''),
  (value) => (Array.isArray(value) ? value.join(', ') : ''),
)

export const autoNumberKind: FieldKind<AutoNumberOptions> = simpleKind(
  'autonumber',
  'number',
  { required: false, unique: false, default: false },
  ['eq', 'neq', 'lt', 'lte', 'gt', 'gte', 'between'],
  NUMERIC_AGGS,
  (raw) => {
    const set = OptionSet.parse(raw)
    if (!set.ok) return set
    const prefix = set.value.take('prefix')
    if (
      prefix !== undefined &&
      (typeof prefix !== 'string' ||
        !validString(prefix, false, false) ||
        codePoints(prefix) > 32)
    )
      return fail('invalid_options', 'prefix')
    const padding = set.value.integer('padding', 0, 20)
    if (!padding.ok) return padding
    const done = set.value.done()
    return done.ok
      ? ok({
          prefix: typeof prefix === 'string' ? prefix : '',
          padding: padding.value ?? 0,
        })
      : done
  },
  () => fail('read_only'),
  () => fail('read_only'),
  (value) => (typeof value === 'string' ? value : ''),
  (value, options) => {
    if (typeof value !== 'string' || !STRICT_INTEGER.test(value)) return ''
    const number = BigInt(value)
    const digits = number < 0 ? value.slice(1) : value
    return `${options.prefix}${number < 0 ? '-' : ''}${digits.padStart(options.padding, '0')}`
  },
)

export const batch2Kinds: Partial<Record<Kind, FieldKind<unknown>>> = {
  time: timeKind as FieldKind<unknown>,
  duration: durationKind as FieldKind<unknown>,
  currency: currencyKind as FieldKind<unknown>,
  percent: percentKind as FieldKind<unknown>,
  rating: ratingKind as FieldKind<unknown>,
  phone: phoneKind as FieldKind<unknown>,
  uuid: uuidKind as FieldKind<unknown>,
  json: jsonKind as FieldKind<unknown>,
  inet: inetKind as FieldKind<unknown>,
  cidr: cidrKind as FieldKind<unknown>,
  text_array: textArrayKind as FieldKind<unknown>,
  int_array: intArrayKind as FieldKind<unknown>,
  user: userKind as FieldKind<unknown>,
  multi_user: multiUserKind as FieldKind<unknown>,
  autonumber: autoNumberKind as FieldKind<unknown>,
}
