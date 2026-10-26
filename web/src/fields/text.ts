import {
  codePoints,
  hasLoneSurrogate,
  isControl,
  OptionSet,
  trimSpace,
  validString,
} from './common'
import { BASIC_AGGS, TEXT_OPS } from './capabilities'
import {
  fail,
  ok,
  type FieldKind,
  type Kind,
  type Result,
  type WireValue,
} from './types'

export const MAX_TEXT_LENGTH = 10000
export const MAX_LONG_TEXT_LENGTH = 100000
const MAX_EMAIL_BYTES = 254
const MAX_EMAIL_LOCAL = 64
const MAX_URL_LENGTH = 2048

const LINE_BREAKS = /\r\n|\r|\n/g
const EMAIL_LOCAL =
  /^[A-Za-z0-9!#$%&'*+/=?^_`{|}~-]+(\.[A-Za-z0-9!#$%&'*+/=?^_`{|}~-]+)*$/
const DOMAIN_LABEL = /^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$/
const TOP_LABEL = /^([A-Za-z]{2,63}|[Xx][Nn]--[A-Za-z0-9-]{1,59})$/
const SCHEME = /^[A-Za-z][A-Za-z0-9+-]*:/
const HTTP_PREFIX = /^[Hh][Tt][Tt][Pp][Ss]?:\/\//
const IPV6_HOST = /^\[[0-9A-Fa-f:.]+\]$/
const PORT = /^[0-9]{1,5}$/
const ASCII_LABEL = /^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?$/
const BAD_PERCENT = /%([^0-9A-Fa-f]|[0-9A-Fa-f][^0-9A-Fa-f]|[0-9A-Fa-f]?$)/
const SPACE = /[\s\u0085]/

function hasSpaceOrControl(text: string) {
  for (let index = 0; index < text.length; index++) {
    const code = text.charCodeAt(index)
    if (isControl(code) || code === 0x5c) return true
  }
  return SPACE.test(text)
}

/** Conservative ASCII address rule of D-119 for an already-trimmed value. */
export function validEmail(text: string) {
  if (/[^\x20-\x7e]/.test(text) || text.length > MAX_EMAIL_BYTES) return false
  const parts = text.split('@')
  if (parts.length !== 2) return false
  const [local = '', domain = ''] = parts
  if (local.length > MAX_EMAIL_LOCAL || !EMAIL_LOCAL.test(local)) return false
  const labels = domain.split('.')
  if (labels.length < 2 || !labels.every((label) => DOMAIN_LABEL.test(label)))
    return false
  return TOP_LABEL.test(labels[labels.length - 1] ?? '')
}

function validHostname(host: string) {
  if (!host || host.length > 253) return false
  return host
    .split('.')
    .every((label) => label.length <= 63 && ASCII_LABEL.test(label))
}

/** Validates a trimmed absolute http(s) URL; returns a reason or null. */
export function checkUrl(
  text: string,
): 'too_long' | 'invalid_url' | 'url_scheme' | null {
  if (codePoints(text) > MAX_URL_LENGTH) return 'too_long'
  if (!HTTP_PREFIX.test(text))
    return SCHEME.test(text) ? 'url_scheme' : 'invalid_url'
  if (hasSpaceOrControl(text) || hasLoneSurrogate(text)) return 'invalid_url'
  if (BAD_PERCENT.test(text)) return 'invalid_url'
  const rest = text.slice(text.indexOf('://') + 3)
  const end = rest.search(/[/?#]/)
  const authority = end >= 0 ? rest.slice(0, end) : rest
  if (!authority || authority.includes('@')) return 'invalid_url'
  let host = authority
  let port: string | null = null
  if (authority.startsWith('[')) {
    const close = authority.indexOf(']')
    if (close < 0) return 'invalid_url'
    host = authority.slice(0, close + 1)
    const suffix = authority.slice(close + 1)
    if (suffix) {
      if (!suffix.startsWith(':')) return 'invalid_url'
      port = suffix.slice(1)
    }
    if (!IPV6_HOST.test(host)) return 'invalid_url'
    try {
      new URL(`http://${host}`)
    } catch (error) {
      if (error instanceof TypeError) return 'invalid_url'
      throw error
    }
  } else {
    const colon = authority.lastIndexOf(':')
    if (colon >= 0) {
      host = authority.slice(0, colon)
      port = authority.slice(colon + 1)
    }
    if (!validHostname(host)) return 'invalid_url'
  }
  if (port !== null) {
    if (!PORT.test(port)) return 'invalid_url'
    const number = Number(port)
    if (number < 1 || number > 65535) return 'invalid_url'
  }
  return null
}

/** Returns an href only for values that re-validate (D-119). */
export function safeHref(kind: Kind, value: WireValue): string | null {
  if (typeof value !== 'string') return null
  if (kind === 'email')
    return validEmail(value)
      ? `mailto:${encodeURIComponent(value).replace('%40', '@')}`
      : null
  if (kind === 'url') return checkUrl(value) === null ? value : null
  return null
}

export interface TextOptions {
  maxLength?: number
  richText?: false
}

function validateText(
  kind: Kind,
  options: TextOptions,
  input: string,
  lenient: boolean,
): Result<WireValue> {
  let text = input
  switch (kind) {
    case 'text': {
      if (lenient) text = text.replace(LINE_BREAKS, ' ')
      if (text === '') return ok(null)
      if (!validString(text, true, false)) return fail('invalid_text')
      if (codePoints(text) > (options.maxLength ?? MAX_TEXT_LENGTH))
        return fail('too_long')
      return ok(text)
    }
    case 'long_text': {
      text = text.replace(LINE_BREAKS, '\n')
      if (text === '') return ok(null)
      if (!validString(text, true, true)) return fail('invalid_text')
      if (codePoints(text) > MAX_LONG_TEXT_LENGTH) return fail('too_long')
      return ok(text)
    }
    case 'email': {
      text = trimSpace(text)
      if (text === '') return ok(null)
      return validEmail(text) ? ok(text) : fail('invalid_email')
    }
    default: {
      text = trimSpace(text)
      if (text === '') return ok(null)
      if (lenient && !text.includes('://') && !SCHEME.test(text))
        text = `https://${text}`
      const reason = checkUrl(text)
      return reason ? fail(reason) : ok(text)
    }
  }
}

function textKind(
  kind: 'text' | 'long_text' | 'email' | 'url',
): FieldKind<TextOptions> {
  return {
    kind,
    readOnly: false,
    presentation:
      kind === 'long_text' ? 'multiline' : kind === 'text' ? 'text' : kind,
    constraints: {
      required: true,
      unique: kind !== 'long_text',
      default: true,
    },
    normalizeOptions(raw) {
      const set = OptionSet.parse(raw)
      if (!set.ok) return set
      const options: TextOptions = {}
      if (kind === 'text') {
        const length = set.value.integer('maxLength', 1, MAX_TEXT_LENGTH)
        if (!length.ok) return length
        if (length.value !== undefined) options.maxLength = length.value
      }
      if (kind === 'long_text') {
        const rich = set.value.boolean('richText', false)
        if (!rich.ok) return rich
        if (rich.value) return fail('invalid_options', 'richText')
        options.richText = false
      }
      const done = set.value.done()
      return done.ok ? ok(options) : done
    },
    normalize(value, options) {
      if (value === null) return ok(null)
      if (typeof value !== 'string' || hasLoneSurrogate(value))
        return fail('invalid_type')
      return validateText(kind, options, value, false)
    },
    parsePaste: (text, options) => validateText(kind, options, text, true),
    formatCopy: (value) => (typeof value === 'string' ? value : ''),
    formatDisplay: (value) => (typeof value === 'string' ? value : ''),
    filterOps: () => [...TEXT_OPS],
    aggregates: () => [...BASIC_AGGS],
  }
}

export const textKinds = {
  text: textKind('text'),
  long_text: textKind('long_text'),
  email: textKind('email'),
  url: textKind('url'),
}
