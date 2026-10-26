import { fail, ok, type Result } from './types'

// Go unicode.IsSpace plus U+FEFF: JavaScript \s already covers U+FEFF and the
// Unicode Zs/line separators but not U+0085.
const TRIM_START = /^[\s\u0085]+/
const TRIM_END = /[\s\u0085]+$/

export const trimSpace = (text: string) =>
  text.replace(TRIM_START, '').replace(TRIM_END, '')

export const codePoints = (text: string) => [...text].length

export const isControl = (code: number) =>
  code < 0x20 || (code >= 0x7f && code <= 0x9f)

export function hasLoneSurrogate(text: string) {
  for (let index = 0; index < text.length; index++) {
    const code = text.charCodeAt(index)
    if (code >= 0xd800 && code <= 0xdbff) {
      const next = text.charCodeAt(index + 1)
      if (next >= 0xdc00 && next <= 0xdfff) {
        index++
        continue
      }
      return true
    }
    if (code >= 0xdc00 && code <= 0xdfff) return true
  }
  return false
}

/** No lone surrogates and no control characters except allowed tab/LF. */
export function validString(text: string, allowTab: boolean, allowLF: boolean) {
  if (hasLoneSurrogate(text)) return false
  for (let index = 0; index < text.length; index++) {
    const code = text.charCodeAt(index)
    if (!isControl(code)) continue
    if ((code === 9 && allowTab) || (code === 10 && allowLF)) continue
    return false
  }
  return true
}

/** Go strings.ToLower uses simple per-code-point case mapping. */
export const lowerSimple = (text: string) =>
  [...text]
    .map((char) => {
      const lower = char.toLowerCase()
      return codePoints(lower) === 1 ? lower : ([...lower][0] ?? char)
    })
    .join('')

export const isPlainObject = (
  value: unknown,
): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value)

/** Strict option reader mirroring internal/codec optionSet. */
export class OptionSet {
  private used = new Set<string>()
  private constructor(private values: Record<string, unknown>) {}

  static parse(raw: unknown): Result<OptionSet> {
    if (raw === null || raw === undefined) return ok(new OptionSet({}))
    if (!isPlainObject(raw)) return fail('invalid_options')
    return ok(new OptionSet(raw))
  }

  has(key: string) {
    return Object.hasOwn(this.values, key)
  }

  take(key: string): unknown {
    if (!this.has(key)) return undefined
    this.used.add(key)
    return this.values[key]
  }

  done(): Result<true> {
    for (const key of Object.keys(this.values)) {
      if (!this.used.has(key)) return fail('invalid_options', key)
    }
    return ok(true)
  }

  enum<T extends string>(
    key: string,
    fallback: T,
    allowed: readonly T[],
  ): Result<T> {
    if (!this.has(key)) return ok(fallback)
    const value = this.take(key)
    return typeof value === 'string' &&
      (allowed as readonly string[]).includes(value)
      ? ok(value as T)
      : fail('invalid_options', key)
  }

  boolean(key: string, fallback: boolean): Result<boolean> {
    if (!this.has(key)) return ok(fallback)
    const value = this.take(key)
    return typeof value === 'boolean' ? ok(value) : fail('invalid_options', key)
  }

  integer(key: string, low: number, high: number): Result<number | undefined> {
    if (!this.has(key)) return ok(undefined)
    const value = this.take(key)
    return typeof value === 'number' &&
      Number.isInteger(value) &&
      value >= low &&
      value <= high
      ? ok(value)
      : fail('invalid_options', key)
  }
}
