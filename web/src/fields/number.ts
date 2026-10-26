import { OptionSet, trimSpace } from './common'
import { NUMERIC_AGGS, NUMERIC_OPS } from './capabilities'
import { fail, ok, type FieldKind, type Result, type WireValue } from './types'

const STRICT_INTEGER = /^-?(0|[1-9][0-9]*)$/
const JSON_NUMBER = /^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$/
const PASTE_INTEGER = /^[+-]?[0-9]+(\.0+)?$/
const PASTE_NUMBER = /^[+-]?([0-9]+\.?[0-9]*|\.[0-9]+)([eE][+-]?[0-9]+)?$/
const MAX_NUMBER_TEXT = 1000
const MAX_EXPONENT = 1e9

/** D-116 numeric paste normalization (trim, full-width forms, grouping). */
export function normalizeNumberText(input: string) {
  let result = ''
  for (const char of trimSpace(input)) {
    const code = char.codePointAt(0) ?? 0
    if (code >= 0xff10 && code <= 0xff19) result += String(code - 0xff10)
    else if (char === '＋') result += '+'
    else if (char === '－' || char === '−') result += '-'
    else if (char === '．') result += '.'
    else if (char === 'ｅ' || char === 'Ｅ') result += 'e'
    else if (!",，_ \u00a0\u202f\u2009'".includes(char)) result += char
  }
  return result
}

/** value = digits × 10^exp with digits free of leading/trailing zeros. */
interface DecimalValue {
  negative: boolean
  digits: string
  exp: number
}

export function parseDecimal(input: string): DecimalValue | null {
  if (input.length > MAX_NUMBER_TEXT) return null
  let text = input
  let negative = false
  if (text.startsWith('-') || text.startsWith('+')) {
    negative = text.startsWith('-')
    text = text.slice(1)
  }
  const lower = text.toLowerCase()
  const e = lower.indexOf('e')
  const mantissa = e >= 0 ? lower.slice(0, e) : lower
  const exponentText = e >= 0 ? lower.slice(e + 1) : ''
  const dot = mantissa.indexOf('.')
  const whole = dot >= 0 ? mantissa.slice(0, dot) : mantissa
  const fraction = dot >= 0 ? mantissa.slice(dot + 1) : ''
  let digits = whole + fraction
  let exp = -fraction.length
  if (e >= 0) {
    let parsed = Number(exponentText)
    if (!(Math.abs(parsed) <= MAX_EXPONENT)) {
      if (/[1-9]/.test(digits))
        parsed = exponentText.startsWith('-') ? -MAX_EXPONENT : MAX_EXPONENT
      else parsed = 0
    }
    exp += parsed
  }
  digits = digits.replace(/^0+/, '')
  const trimmed = digits.replace(/0+$/, '')
  exp += digits.length - trimmed.length
  if (trimmed === '') return { negative: false, digits: '', exp: 0 }
  return { negative, digits: trimmed, exp }
}

const integerDigits = (value: DecimalValue) =>
  value.digits === '' ? 0 : Math.max(0, value.digits.length + value.exp)
const fractionDigits = (value: DecimalValue) => Math.max(0, -value.exp)

function formatDecimal(value: DecimalValue, scale: number) {
  let whole = '0'
  let fraction = ''
  if (value.digits !== '') {
    if (value.exp >= 0) whole = value.digits + '0'.repeat(value.exp)
    else {
      const k = -value.exp
      if (value.digits.length > k) {
        whole = value.digits.slice(0, value.digits.length - k)
        fraction = value.digits.slice(value.digits.length - k)
      } else fraction = '0'.repeat(k - value.digits.length) + value.digits
    }
  }
  fraction += '0'.repeat(scale - fraction.length)
  const text = scale > 0 ? `${whole}.${fraction}` : whole
  return value.negative ? `-${text}` : text
}

/** Inserts ',' grouping into the integer part of a plain decimal string. */
export function groupDigits(text: string) {
  const match = /^(-?)([0-9]+)(.*)$/.exec(text)
  if (!match) return text
  const [, sign = '', whole = '', rest = ''] = match
  return sign + whole.replace(/\B(?=([0-9]{3})+(?![0-9]))/g, ',') + rest
}

// ---- integer ----

export type IntegerSize = 'small' | 'int' | 'big'
export interface IntegerOptions {
  size: IntegerSize
  thousandSep: boolean
}

const INTEGER_RANGES: Record<IntegerSize, [bigint, bigint]> = {
  small: [-32768n, 32767n],
  int: [-2147483648n, 2147483647n],
  big: [-9223372036854775808n, 9223372036854775807n],
}

function integerFrom(text: string, options: IntegerOptions): Result<WireValue> {
  if (text.replace(/^-/, '').replace(/^0+/, '').length > 19)
    return fail('out_of_range')
  const value = BigInt(text)
  const [low, high] = INTEGER_RANGES[options.size]
  if (value < low || value > high) return fail('out_of_range')
  return ok(options.size === 'big' ? value.toString() : Number(value))
}

const integerText = (value: WireValue) =>
  typeof value === 'number' || typeof value === 'string' ? String(value) : ''

export const integerKind: FieldKind<IntegerOptions> = {
  kind: 'integer',
  readOnly: false,
  presentation: 'number',
  constraints: { required: true, unique: true, default: true },
  normalizeOptions(raw) {
    const set = OptionSet.parse(raw)
    if (!set.ok) return set
    const size = set.value.enum<IntegerSize>('size', 'int', [
      'small',
      'int',
      'big',
    ])
    if (!size.ok) return size
    const thousandSep = set.value.boolean('thousandSep', true)
    if (!thousandSep.ok) return thousandSep
    const done = set.value.done()
    return done.ok
      ? ok({ size: size.value, thousandSep: thousandSep.value })
      : done
  },
  normalize(value, options) {
    if (value === null) return ok(null)
    if (typeof value === 'string') {
      if (value === '') return ok(null)
      if (!STRICT_INTEGER.test(value)) return fail('invalid_integer')
      return integerFrom(value, options)
    }
    if (typeof value !== 'number') return fail('invalid_type')
    if (!Number.isInteger(value)) return fail('invalid_integer')
    if (!Number.isSafeInteger(value)) return fail('imprecise_number')
    return integerFrom(String(Object.is(value, -0) ? 0 : value), options)
  },
  parsePaste(input, options) {
    const text = normalizeNumberText(input)
    if (text === '') return ok(null)
    if (!PASTE_INTEGER.test(text)) return fail('invalid_integer')
    const value = parseDecimal(text)
    if (!value) return fail('invalid_integer')
    return integerFrom(formatDecimal(value, 0), options)
  },
  formatCopy: integerText,
  formatDisplay(value, options) {
    const text = integerText(value)
    return options.thousandSep ? groupDigits(text) : text
  },
  filterOps: () => [...NUMERIC_OPS],
  aggregates: () => [...NUMERIC_AGGS],
}

// ---- decimal ----

export interface DecimalOptions {
  precision: number
  scale: number
  thousandSep: boolean
}

function checkDecimal(
  text: string,
  options: DecimalOptions,
  fromNumber: boolean,
): Result<WireValue> {
  const value = parseDecimal(text)
  if (!value) return fail('invalid_decimal')
  if (fromNumber && value.digits.length > 15) return fail('imprecise_number')
  if (fractionDigits(value) > options.scale) return fail('too_many_decimals')
  if (integerDigits(value) > options.precision - options.scale)
    return fail('out_of_range')
  return ok(formatDecimal(value, options.scale))
}

export const decimalKind: FieldKind<DecimalOptions> = {
  kind: 'decimal',
  readOnly: false,
  presentation: 'number',
  constraints: { required: true, unique: true, default: true },
  normalizeOptions(raw) {
    const set = OptionSet.parse(raw)
    if (!set.ok) return set
    const precision = set.value.integer('precision', 1, 38)
    if (!precision.ok) return precision
    const scale = set.value.integer('scale', 0, 38)
    if (!scale.ok) return scale
    const p = precision.value ?? 18
    const s = scale.value ?? Math.min(2, p)
    if (s > p) return fail('invalid_options', 'scale')
    const thousandSep = set.value.boolean('thousandSep', true)
    if (!thousandSep.ok) return thousandSep
    const done = set.value.done()
    return done.ok
      ? ok({ precision: p, scale: s, thousandSep: thousandSep.value })
      : done
  },
  normalize(value, options) {
    if (value === null) return ok(null)
    if (typeof value === 'string') {
      if (value === '') return ok(null)
      if (!JSON_NUMBER.test(value)) return fail('invalid_decimal')
      return checkDecimal(value, options, false)
    }
    if (typeof value !== 'number') return fail('invalid_type')
    if (!Number.isFinite(value)) return fail('invalid_number')
    return checkDecimal(String(value), options, true)
  },
  parsePaste(input, options) {
    const text = normalizeNumberText(input)
    if (text === '') return ok(null)
    if (!PASTE_NUMBER.test(text)) return fail('invalid_decimal')
    return checkDecimal(text, options, false)
  },
  formatCopy: (value) => (typeof value === 'string' ? value : ''),
  formatDisplay(value, options) {
    if (typeof value !== 'string') return ''
    return options.thousandSep ? groupDigits(value) : value
  },
  filterOps: () => [...NUMERIC_OPS],
  aggregates: () => [...NUMERIC_AGGS],
}

// ---- float ----

export interface FloatOptions {
  size: 'real' | 'double'
  displayDecimals?: number
  thousandSep: boolean
}

/** Shortest decimal that round-trips through float32, as a JS number. */
export function shortestFloat32(value: number) {
  const single = Math.fround(value)
  if (single === 0) return 0
  for (let digits = 1; digits <= 9; digits++) {
    const candidate = Number(single.toPrecision(digits))
    if (Math.fround(candidate) === single) return candidate
  }
  return single
}

function floatValue(value: number, options: FloatOptions): Result<WireValue> {
  if (!Number.isFinite(value)) return fail('out_of_range')
  if (options.size === 'real') {
    if (!Number.isFinite(Math.fround(value))) return fail('out_of_range')
    return ok(shortestFloat32(value))
  }
  return ok(value === 0 ? 0 : value)
}

export const floatKind: FieldKind<FloatOptions> = {
  kind: 'float',
  readOnly: false,
  presentation: 'number',
  constraints: { required: true, unique: true, default: true },
  normalizeOptions(raw) {
    const set = OptionSet.parse(raw)
    if (!set.ok) return set
    const size = set.value.enum<'real' | 'double'>('size', 'double', [
      'real',
      'double',
    ])
    if (!size.ok) return size
    const options: FloatOptions = { size: size.value, thousandSep: true }
    if (
      set.value.has('displayDecimals') &&
      set.value.take('displayDecimals') === null
    ) {
      // null means "not set"
    } else {
      const decimals = set.value.integer('displayDecimals', 0, 10)
      if (!decimals.ok) return decimals
      if (decimals.value !== undefined) options.displayDecimals = decimals.value
    }
    const thousandSep = set.value.boolean('thousandSep', true)
    if (!thousandSep.ok) return thousandSep
    options.thousandSep = thousandSep.value
    const done = set.value.done()
    return done.ok ? ok(options) : done
  },
  normalize(value, options) {
    if (value === null) return ok(null)
    if (typeof value !== 'number') return fail('invalid_type')
    return floatValue(value, options)
  },
  parsePaste(input, options) {
    const text = normalizeNumberText(input)
    if (text === '') return ok(null)
    if (!PASTE_NUMBER.test(text) || text.length > MAX_NUMBER_TEXT)
      return fail('invalid_number')
    return floatValue(Number(text), options)
  },
  formatCopy: (value) => (typeof value === 'number' ? String(value) : ''),
  formatDisplay(value, options) {
    if (typeof value !== 'number') return ''
    const text =
      options.displayDecimals === undefined
        ? String(value)
        : value.toFixed(options.displayDecimals)
    return options.thousandSep && !/e/.test(text) ? groupDigits(text) : text
  },
  filterOps: () => [...NUMERIC_OPS],
  aggregates: () => [...NUMERIC_AGGS],
}

// ---- checkbox ----

const CHECKBOX_TRUE = [
  'true',
  '1',
  'yes',
  'y',
  'on',
  'checked',
  'x',
  '✓',
  '✔',
  '☑',
  '是',
  '对',
]
const CHECKBOX_FALSE = [
  'false',
  '0',
  'no',
  'n',
  'off',
  'unchecked',
  '☐',
  '否',
  '错',
]

export const checkboxKind: FieldKind<Record<string, never>> = {
  kind: 'checkbox',
  readOnly: false,
  presentation: 'checkbox',
  constraints: { required: false, unique: false, default: true },
  normalizeOptions(raw) {
    const set = OptionSet.parse(raw)
    if (!set.ok) return set
    const done = set.value.done()
    return done.ok ? ok({}) : done
  },
  normalize(value) {
    if (value === null) return ok(false)
    return typeof value === 'boolean' ? ok(value) : fail('invalid_type')
  },
  parsePaste(input) {
    const text = trimSpace(input).toLowerCase()
    if (text === '' || CHECKBOX_FALSE.includes(text)) return ok(false)
    if (CHECKBOX_TRUE.includes(text)) return ok(true)
    return fail('invalid_boolean')
  },
  formatCopy: (value) => (value === true ? 'true' : 'false'),
  formatDisplay: (value) => (value === true ? 'true' : 'false'),
  filterOps: () => ['is_checked', 'is_not_checked'],
  aggregates: () => ['none', 'count', 'percent_checked'],
}
