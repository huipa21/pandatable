import { OptionSet, trimSpace } from './common'
import { TEMPORAL_AGGS, TEMPORAL_OPS } from './capabilities'
import {
  fail,
  ok,
  type FieldEnv,
  type FieldKind,
  type Result,
  type WireValue,
} from './types'

// Go RE2 `\s` is ASCII-only; mirror it exactly.
const ISO_DATE = /^([0-9]{4})-([0-9]{2})-([0-9]{2})$/
const WIRE_DATETIME =
  /^([0-9]{4})-([0-9]{2})-([0-9]{2})[Tt]([0-9]{2}):([0-9]{2}):([0-9]{2})(\.[0-9]+)?([Zz]|[+-][0-9]{2}:[0-9]{2})$/
const PASTE_DATE_PARTS = /^([0-9]{1,4})([-/.])([0-9]{1,2})([-/.])([0-9]{1,4})/
const PASTE_CJK_DATE = /^([0-9]{4})年([0-9]{1,2})月([0-9]{1,2})日/
const PASTE_TIME =
  /^(?:[Tt]|[\t\n\f\r ]+)([0-9]{1,2}):([0-9]{2})(?::([0-9]{2})(\.[0-9]+)?)?(?:[\t\n\f\r ]*([AaPp][Mm]))?(?:[\t\n\f\r ]*([Zz]|[+-][0-9]{2}(?::[0-9]{2}(?::[0-9]{2})?|[0-9]{2})))?$/
const TZ_NAME = /^[A-Za-z0-9_+-]+(\/[A-Za-z0-9_+-]+)*$/

export type DateFormat = 'iso' | 'slash' | 'us' | 'eu'
const DATE_FORMATS: readonly DateFormat[] = ['iso', 'slash', 'us', 'eu']

interface CivilDate {
  year: number
  month: number
  day: number
}

/** An instant as integer epoch seconds plus 0..999999 microseconds. */
export interface Instant {
  seconds: number
  micros: number
}

const pad = (value: number, width = 2) => String(value).padStart(width, '0')

const isLeap = (year: number) =>
  (year % 4 === 0 && year % 100 !== 0) || year % 400 === 0

function daysInMonth(year: number, month: number) {
  if (month === 2) return isLeap(year) ? 29 : 28
  return [4, 6, 9, 11].includes(month) ? 30 : 31
}

const validDate = (date: CivilDate) =>
  date.month >= 1 &&
  date.month <= 12 &&
  date.day >= 1 &&
  date.day <= daysInMonth(date.year, date.month)

function checkDate<T extends CivilDate>(
  date: T,
  invalid: 'invalid_date' | 'invalid_datetime',
): Result<T> {
  if (!validDate(date)) return fail(invalid)
  if (date.year < 1 || date.year > 9999) return fail('out_of_range')
  return ok(date)
}

/** Epoch seconds of a proleptic-Gregorian wall time read as UTC. */
function wallSeconds(date: CivilDate, hour = 0, minute = 0, second = 0) {
  const value = new Date(0)
  value.setUTCFullYear(date.year, date.month - 1, date.day)
  value.setUTCHours(hour, minute, second, 0)
  return value.getTime() / 1000
}

function utcFields(seconds: number) {
  const value = new Date(seconds * 1000)
  return {
    year: value.getUTCFullYear(),
    month: value.getUTCMonth() + 1,
    day: value.getUTCDate(),
    hour: value.getUTCHours(),
    minute: value.getUTCMinutes(),
    second: value.getUTCSeconds(),
  }
}

const MIN_SECONDS = wallSeconds({ year: 1, month: 1, day: 1 })
const MAX_SECONDS = wallSeconds({ year: 9999, month: 12, day: 31 }, 23, 59, 59)

function checkInstant(instant: Instant): Result<Instant> {
  if (instant.seconds < MIN_SECONDS || instant.seconds > MAX_SECONDS)
    return fail('out_of_range')
  return ok(instant)
}

// ---- zones ----

const formatters = new Map<string, Intl.DateTimeFormat>()

/** IANA zone check mirroring Go LoadLocation's syntax rules (D-114). */
export function loadZone(name: string): Intl.DateTimeFormat | null {
  if (
    name.length > 64 ||
    !TZ_NAME.test(name) ||
    name === 'Local' ||
    name.startsWith('posix/') ||
    name.startsWith('right/')
  )
    return null
  const cached = formatters.get(name)
  if (cached) return cached
  try {
    const formatter = new Intl.DateTimeFormat('en-US', {
      timeZone: name,
      calendar: 'gregory',
      numberingSystem: 'latn',
      era: 'short',
      year: 'numeric',
      month: 'numeric',
      day: 'numeric',
      hour: 'numeric',
      minute: 'numeric',
      second: 'numeric',
      hourCycle: 'h23',
    })
    formatters.set(name, formatter)
    return formatter
  } catch {
    return null
  }
}

/** UTC offset in seconds of `zone` at epoch `seconds`. */
export function zoneOffset(zone: Intl.DateTimeFormat, seconds: number) {
  const parts: Record<string, string> = {}
  for (const part of zone.formatToParts(new Date(seconds * 1000)))
    parts[part.type] = part.value
  let year = Number(parts.year)
  if (parts.era === 'BC' || parts.era === 'B') year = 1 - year
  const local = wallSeconds(
    { year, month: Number(parts.month), day: Number(parts.day) },
    Number(parts.hour) % 24,
    Number(parts.minute),
    Number(parts.second),
  )
  return local - seconds
}

/**
 * Maps a wall time (epoch seconds of the wall fields read as UTC) in `zone`
 * to an instant: gap times fail, overlaps resolve to the earlier instant.
 * Same algorithm as Go codec.ResolveLocal.
 */
export function resolveLocal(
  wall: number,
  zone: Intl.DateTimeFormat,
): Result<number> {
  const seen = new Set<number>()
  let best: number | undefined
  for (const hours of [-48, -24, 0, 24, 48]) {
    const offset = zoneOffset(zone, wall + hours * 3600)
    if (seen.has(offset)) continue
    seen.add(offset)
    const candidate = wall - offset
    if (zoneOffset(zone, candidate) !== offset) continue
    if (best === undefined || candidate < best) best = candidate
  }
  return best === undefined ? fail('nonexistent_local_time') : ok(best)
}

// ---- parsing/formatting ----

function parseFraction(text: string | undefined): number | null {
  if (!text) return 0
  const digits = text.slice(1)
  return digits.length > 6 ? null : Number(digits.padEnd(6, '0'))
}

/** Z, ±HH:MM[:SS] or ±HHMM (hours ≤ 23, minutes/seconds ≤ 59). */
function parseOffset(text: string): number | null {
  if (text === 'Z' || text === 'z') return 0
  const digits = text.slice(1).replaceAll(':', '')
  const hours = Number(digits.slice(0, 2))
  const minutes = Number(digits.slice(2, 4))
  const seconds = Number(digits.slice(4) || '0')
  if (hours > 23 || minutes > 59 || seconds > 59) return null
  const offset = hours * 3600 + minutes * 60 + seconds
  return text.startsWith('-') ? -offset : offset
}

export function parseWireDateTime(text: string): Result<Instant> {
  const match = WIRE_DATETIME.exec(text)
  if (!match) return fail('invalid_datetime')
  const date = {
    year: Number(match[1]),
    month: Number(match[2]),
    day: Number(match[3]),
  }
  if (!validDate(date)) return fail('invalid_datetime')
  const hour = Number(match[4])
  const minute = Number(match[5])
  const second = Number(match[6])
  const micros = parseFraction(match[7])
  const offset = parseOffset(match[8] ?? '')
  if (
    hour > 23 ||
    minute > 59 ||
    second > 59 ||
    micros === null ||
    offset === null
  )
    return fail('invalid_datetime')
  return checkInstant({
    seconds: wallSeconds(date, hour, minute, second) - offset,
    micros,
  })
}

function parsePasteDate(
  text: string,
  format: DateFormat,
  invalid: 'invalid_date' | 'invalid_datetime',
): Result<{ date: CivilDate; rest: string }> {
  const cjk = PASTE_CJK_DATE.exec(text)
  if (cjk) {
    const date = {
      year: Number(cjk[1]),
      month: Number(cjk[2]),
      day: Number(cjk[3]),
    }
    const checked = checkDate(date, invalid)
    return checked.ok ? ok({ date, rest: text.slice(cjk[0].length) }) : checked
  }
  const match = PASTE_DATE_PARTS.exec(text)
  if (!match || match[2] !== match[4]) return fail(invalid)
  const [whole, first = '', , second = '', , third = ''] = match
  let date: CivilDate
  if (first.length === 4)
    date = { year: Number(first), month: Number(second), day: Number(third) }
  else if (third.length === 4 && first.length <= 2 && format === 'us')
    date = { year: Number(third), month: Number(first), day: Number(second) }
  else if (third.length === 4 && first.length <= 2 && format === 'eu')
    date = { year: Number(third), month: Number(second), day: Number(first) }
  else return fail(invalid)
  const checked = checkDate(date, invalid)
  return checked.ok ? ok({ date, rest: text.slice(whole.length) }) : checked
}

function parsePasteDateTime(
  text: string,
  format: DateFormat,
  zone: Intl.DateTimeFormat | null,
): Result<Instant> {
  const parsed = parsePasteDate(text, format, 'invalid_datetime')
  if (!parsed.ok)
    return parsed.reason === 'out_of_range' ? parsed : fail('invalid_datetime')
  const { date, rest } = parsed.value
  let hour = 0
  let minute = 0
  let second = 0
  let micros = 0
  let offsetText = ''
  if (rest !== '') {
    const match = PASTE_TIME.exec(rest)
    if (!match) return fail('invalid_datetime')
    hour = Number(match[1])
    minute = Number(match[2])
    second = Number(match[3] ?? '0')
    const fraction = parseFraction(match[4])
    if (fraction === null) return fail('invalid_datetime')
    micros = fraction
    const meridiem = (match[5] ?? '').toLowerCase()
    if (meridiem !== '') {
      if (hour < 1 || hour > 12) return fail('invalid_datetime')
      hour %= 12
      if (meridiem === 'pm') hour += 12
    }
    if (hour > 23 || minute > 59 || second > 59) return fail('invalid_datetime')
    offsetText = match[6] ?? ''
  }
  const wall = wallSeconds(date, hour, minute, second)
  if (offsetText !== '') {
    const offset = parseOffset(offsetText)
    if (offset === null) return fail('invalid_datetime')
    return checkInstant({ seconds: wall - offset, micros })
  }
  if (!zone) return fail('timezone_required')
  const resolved = resolveLocal(wall, zone)
  return resolved.ok
    ? checkInstant({ seconds: resolved.value, micros })
    : resolved
}

const fractionSuffix = (micros: number) =>
  micros === 0 ? '' : '.' + pad(micros, 6).replace(/0+$/, '')

const formatYMD = (year: number, month: number, day: number) =>
  `${pad(year, 4)}-${pad(month)}-${pad(day)}`

/** Canonical UTC wire form (D-115). */
export function formatInstant(instant: Instant) {
  const f = utcFields(instant.seconds)
  return `${formatYMD(f.year, f.month, f.day)}T${pad(f.hour)}:${pad(f.minute)}:${pad(f.second)}${fractionSuffix(instant.micros)}Z`
}

function formatOffset(offset: number) {
  const sign = offset < 0 ? '-' : '+'
  const abs = Math.abs(offset)
  let text = `${sign}${pad(Math.floor(abs / 3600))}:${pad(Math.floor(abs / 60) % 60)}`
  if (abs % 60 !== 0) text += `:${pad(abs % 60)}`
  return text
}

/** Copy text in the display zone with an explicit offset (D-117); null = UTC. */
export function formatLocal(
  instant: Instant,
  zone: Intl.DateTimeFormat | null,
) {
  let offset = zone ? zoneOffset(zone, instant.seconds) : 0
  let f = utcFields(instant.seconds + offset)
  if (f.year < 1 || f.year > 9999) {
    offset = 0
    f = utcFields(instant.seconds)
  }
  return `${formatYMD(f.year, f.month, f.day)} ${pad(f.hour)}:${pad(f.minute)}:${pad(f.second)}${fractionSuffix(instant.micros)}${formatOffset(offset)}`
}

function displayDate(
  year: number,
  month: number,
  day: number,
  format: DateFormat,
) {
  const y = pad(year, 4)
  const m = pad(month)
  const d = pad(day)
  switch (format) {
    case 'slash':
      return `${y}/${m}/${d}`
    case 'us':
      return `${m}/${d}/${y}`
    case 'eu':
      return `${d}/${m}/${y}`
    default:
      return `${y}-${m}-${d}`
  }
}

function displayTime(hour: number, minute: number, format: TimeFormat) {
  if (format === '24h') return `${pad(hour)}:${pad(minute)}`
  return `${hour % 12 === 0 ? 12 : hour % 12}:${pad(minute)} ${hour < 12 ? 'AM' : 'PM'}`
}

function stringValue(value: unknown): Result<string | null> {
  if (value === null) return ok(null)
  if (typeof value !== 'string') return fail('invalid_type')
  return ok(value === '' ? null : value)
}

// ---- date ----

export interface DateOptions {
  dateFormat: DateFormat
}

export const dateKind: FieldKind<DateOptions> = {
  kind: 'date',
  readOnly: false,
  presentation: 'date',
  constraints: { required: true, unique: true, default: true },
  normalizeOptions(raw) {
    const set = OptionSet.parse(raw)
    if (!set.ok) return set
    const dateFormat = set.value.enum('dateFormat', 'iso', DATE_FORMATS)
    if (!dateFormat.ok) return dateFormat
    const done = set.value.done()
    return done.ok ? ok({ dateFormat: dateFormat.value }) : done
  },
  normalize(value) {
    const text = stringValue(value)
    if (!text.ok || text.value === null) return text
    const match = ISO_DATE.exec(text.value)
    if (!match) return fail('invalid_date')
    const date = {
      year: Number(match[1]),
      month: Number(match[2]),
      day: Number(match[3]),
    }
    const checked = checkDate(date, 'invalid_date')
    return checked.ok ? ok(text.value) : checked
  },
  parsePaste(input, options) {
    const text = trimSpace(input)
    if (text === '') return ok(null)
    const parsed = parsePasteDate(text, options.dateFormat, 'invalid_date')
    if (!parsed.ok) return parsed
    if (parsed.value.rest !== '') return fail('invalid_date')
    const { year, month, day } = parsed.value.date
    return ok(formatYMD(year, month, day))
  },
  formatCopy: (value) => (typeof value === 'string' ? value : ''),
  formatDisplay(value, options) {
    if (typeof value !== 'string') return ''
    const match = ISO_DATE.exec(value)
    if (!match) return value
    return displayDate(
      Number(match[1]),
      Number(match[2]),
      Number(match[3]),
      options.dateFormat,
    )
  },
  filterOps: () => [...TEMPORAL_OPS],
  aggregates: () => [...TEMPORAL_AGGS],
}

// ---- datetime ----

export type TimeFormat = '24h' | '12h'
export interface DateTimeOptions {
  dateFormat: DateFormat
  timeFormat: TimeFormat
  timezone: 'user' | 'fixed'
  fixedTz?: string
}

/** fixedTz, else the viewer's zone; null when neither is usable. */
export function displayZone(options: { fixedTz?: string }, env: FieldEnv) {
  const name = options.fixedTz ?? env.timeZone
  return name === undefined ? null : loadZone(name)
}

/** Datetime display: field date/time format in the display zone (presentation only). */
export function formatInstantDisplay(
  wire: WireValue,
  options: { dateFormat: DateFormat; timeFormat: TimeFormat; fixedTz?: string },
  env: FieldEnv,
) {
  if (typeof wire !== 'string') return ''
  const instant = parseWireDateTime(wire)
  if (!instant.ok) return wire
  const zone = displayZone(options, env)
  const offset = zone ? zoneOffset(zone, instant.value.seconds) : 0
  const f = utcFields(instant.value.seconds + offset)
  const text = `${displayDate(f.year, f.month, f.day, options.dateFormat)} ${displayTime(f.hour, f.minute, options.timeFormat)}`
  return zone ? text : `${text} UTC`
}

export function formatInstantCopy(
  wire: WireValue,
  options: { fixedTz?: string },
  env: FieldEnv,
) {
  if (typeof wire !== 'string') return ''
  const instant = parseWireDateTime(wire)
  return instant.ok ? formatLocal(instant.value, displayZone(options, env)) : ''
}

export const datetimeKind: FieldKind<DateTimeOptions> = {
  kind: 'datetime',
  readOnly: false,
  presentation: 'datetime',
  constraints: { required: true, unique: true, default: true },
  normalizeOptions(raw) {
    const set = OptionSet.parse(raw)
    if (!set.ok) return set
    const dateFormat = set.value.enum('dateFormat', 'iso', DATE_FORMATS)
    if (!dateFormat.ok) return dateFormat
    const timeFormat = set.value.enum<TimeFormat>('timeFormat', '24h', [
      '24h',
      '12h',
    ])
    if (!timeFormat.ok) return timeFormat
    const timezone = set.value.enum<'user' | 'fixed'>('timezone', 'user', [
      'user',
      'fixed',
    ])
    if (!timezone.ok) return timezone
    const options: DateTimeOptions = {
      dateFormat: dateFormat.value,
      timeFormat: timeFormat.value,
      timezone: timezone.value,
    }
    const zone = set.value.take('fixedTz')
    const hasZone = zone !== undefined && zone !== null
    if (timezone.value === 'fixed' ? !hasZone : hasZone)
      return fail('invalid_options', 'fixedTz')
    if (hasZone) {
      if (typeof zone !== 'string' || !loadZone(zone))
        return fail('invalid_options', 'fixedTz')
      options.fixedTz = zone
    }
    const done = set.value.done()
    return done.ok ? ok(options) : done
  },
  normalize(value) {
    const text = stringValue(value)
    if (!text.ok || text.value === null) return text
    const instant = parseWireDateTime(text.value)
    return instant.ok ? ok(formatInstant(instant.value)) : instant
  },
  parsePaste(input, options, env) {
    const text = trimSpace(input)
    if (text === '') return ok(null)
    const instant = parsePasteDateTime(
      text,
      options.dateFormat,
      displayZone(options, env),
    )
    return instant.ok ? ok(formatInstant(instant.value)) : instant
  },
  formatCopy: formatInstantCopy,
  formatDisplay: formatInstantDisplay,
  filterOps: () => [...TEMPORAL_OPS],
  aggregates: () => [...TEMPORAL_AGGS],
}
