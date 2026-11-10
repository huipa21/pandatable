import type { DateFormat } from './temporal'

// Frontend-only pre-normalizer for typed temporal text (CES-01). Output is
// canonical paste text for the existing kind.parsePaste, so the Go/TS paste
// grammar (D-116) is unchanged; null means "fall back to parsePaste(text)".

export type TypedKind = 'date' | 'datetime' | 'time'
export interface TypedContext {
  dateFormat: DateFormat
  twelveHours: boolean
  today: { year: number; month: number; day: number }
}

const FULL_DATE = /^([0-9]{4})([-/.])([0-9]{1,2})\2([0-9]{1,2})$/
const YEAR_LAST_DATE = /^([0-9]{1,2})([-/.])([0-9]{1,2})\2([0-9]{4})$/
const COMPACT_DATE = /^([0-9]{4})([0-9]{2})([0-9]{2})$/
const SHORT_DATE = /^([0-9]{1,2})[-/.]([0-9]{1,2})$/
const CJK_DATE = /^(?:([0-9]{4})年)?([0-9]{1,2})月([0-9]{1,2})日?$/
const TYPED_TIME =
  /^(?:([0-9]{1,4})|([0-9]{1,2}):([0-9]{2})(?::([0-9]{2})(?:\.([0-9]{1,6}))?)?) ?([ap]m?)?$/i
const TYPED_OFFSET =
  / ?([Zz]|[+-][0-9]{2}(?::[0-9]{2}(?::[0-9]{2})?|[0-9]{2})?)$/

const pad = (value: number, width = 2) => String(value).padStart(width, '0')

/** Full-width ASCII (digits, `：／－．`, letters) → ASCII; trim and collapse whitespace. */
function preprocess(text: string) {
  return text
    .replace(/[\uFF01-\uFF5E]/g, (char) =>
      String.fromCharCode(char.charCodeAt(0) - 0xfee0),
    )
    .replace(/\s+/g, ' ')
    .trim()
}

function ymd(year: number, month: number, day: number) {
  if (month < 1 || month > 12 || day < 1 || day > 31) return null
  return `${pad(year, 4)}-${pad(month)}-${pad(day)}`
}

function normalizeDate(text: string, ctx: TypedContext): string | null {
  let match = FULL_DATE.exec(text)
  if (match) return ymd(Number(match[1]), Number(match[3]), Number(match[4]))
  match = COMPACT_DATE.exec(text)
  if (match) return ymd(Number(match[1]), Number(match[2]), Number(match[3]))
  match = YEAR_LAST_DATE.exec(text)
  if (match) {
    const first = Number(match[1])
    const second = Number(match[3])
    const year = Number(match[4])
    if (ctx.dateFormat === 'us') return ymd(year, first, second)
    if (ctx.dateFormat === 'eu') return ymd(year, second, first)
    return null
  }
  match = SHORT_DATE.exec(text)
  if (match) {
    const first = Number(match[1])
    const second = Number(match[2])
    return ctx.dateFormat === 'eu'
      ? ymd(ctx.today.year, second, first)
      : ymd(ctx.today.year, first, second)
  }
  match = CJK_DATE.exec(text)
  if (match) {
    const year = match[1] === undefined ? ctx.today.year : Number(match[1])
    return ymd(year, Number(match[2]), Number(match[3]))
  }
  return null
}

function normalizeTime(text: string): string | null {
  const match = TYPED_TIME.exec(text)
  if (!match) return null
  let hour: number
  let minute: number
  let second = 0
  let fraction = ''
  const compact = match[1]
  if (compact !== undefined) {
    const split = compact.length <= 2 ? compact.length : compact.length - 2
    hour = Number(compact.slice(0, split))
    minute = Number(compact.slice(split) || '0')
  } else {
    hour = Number(match[2])
    minute = Number(match[3])
    second = Number(match[4] ?? '0')
    fraction = (match[5] ?? '').replace(/0+$/, '')
  }
  const meridiem = match[6]?.toLowerCase()
  if (meridiem !== undefined) {
    if (hour < 1 || hour > 12) return null
    hour = (hour % 12) + (meridiem.startsWith('p') ? 12 : 0)
  }
  if (hour > 23 || minute > 59 || second > 59) return null
  return `${pad(hour)}:${pad(minute)}:${pad(second)}${fraction ? `.${fraction}` : ''}`
}

function normalizeDateTime(text: string, ctx: TypedContext): string | null {
  let datePart = text
  let timePart = ''
  const at = text.search(/[Tt ]/)
  if (at >= 0) {
    datePart = text.slice(0, at)
    timePart = text.slice(at + 1).trim()
    if (timePart === '') return null
  } else {
    const cjk = /^(.*?日)(.+)$/.exec(text)
    if (cjk) [, datePart = '', timePart = ''] = cjk
  }
  const date = normalizeDate(datePart, ctx)
  if (date === null) return null
  if (timePart === '') return date
  let offset = ''
  const offsetMatch = TYPED_OFFSET.exec(timePart)
  if (offsetMatch) {
    offset = offsetMatch[1]!.toUpperCase()
    timePart = timePart.slice(0, offsetMatch.index)
  }
  const time = normalizeTime(timePart)
  return time === null ? null : `${date} ${time}${offset}`
}

/** Lenient typed text → canonical paste text, or null when unrecognized. */
export function normalizeTyped(
  kind: TypedKind,
  text: string,
  ctx: TypedContext,
): string | null {
  const input = preprocess(text)
  if (input === '') return null
  switch (kind) {
    case 'date':
      return normalizeDate(input, ctx)
    case 'time':
      return normalizeTime(input)
    case 'datetime':
      return normalizeDateTime(input, ctx)
  }
}

/**
 * True when the text normalizes and appending more digits cannot produce a
 * different accepted value (e.g. `13:4` or `9` are still being typed).
 */
export function isCompleteTyped(
  kind: TypedKind,
  text: string,
  ctx: TypedContext,
): boolean {
  if (normalizeTyped(kind, text, ctx) === null) return false
  const base = preprocess(text)
  // Two extra digits cover every longer form (e.g. `9` → `930`).
  for (let next = 0; next < 110; next++) {
    const digits = next < 10 ? String(next) : pad(next - 10)
    if (normalizeTyped(kind, base + digits, ctx) !== null) return false
  }
  return true
}

const DATE_PLACEHOLDERS: Record<DateFormat, string> = {
  iso: 'YYYY-MM-DD',
  slash: 'YYYY/MM/DD',
  us: 'MM/DD/YYYY',
  eu: 'DD/MM/YYYY',
}

/** Format hint shown in the empty editor input. */
export function typedPlaceholder(kind: TypedKind, ctx: TypedContext): string {
  const time = ctx.twelveHours ? 'hh:mm a' : 'HH:mm'
  if (kind === 'time') return time
  const date = DATE_PLACEHOLDERS[ctx.dateFormat]
  return kind === 'date' ? date : `${date} ${time}`
}
