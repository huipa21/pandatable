import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { bindField, isKind, KINDS, type BoundField } from './registry'
import {
  parseWireDateTime,
  formatLocal,
  loadZone,
  resolveLocal,
} from './temporal'
import { safeHref } from './text'
import { shortestFloat32 } from './number'
import type { FieldEnv, Kind, Result, WireValue } from './types'

// Shared Go/TypeScript vectors (D-121); Go runs the same file in
// internal/codec/codec_test.go. goOnly entries rely on raw-JSON details
// (duplicate keys, number tokens) that JSON.parse cannot observe.
interface KindVector {
  options?: unknown
  readOnly: boolean
  constraints: [boolean, boolean, boolean]
  filterOps: string[]
  aggregates: string[]
}
interface CaseVector {
  kind: Kind
  options?: unknown
  input?: unknown
  canonical?: unknown
  raw?: string
  text?: string
  wire?: unknown
  zone?: string
  output?: unknown
  reason?: string
  option?: string
  goOnly?: boolean
}
interface Vectors {
  kinds: Record<Kind, KindVector>
  options: CaseVector[]
  decode: CaseVector[]
  paste: CaseVector[]
  format: CaseVector[]
}

const vectors = JSON.parse(
  readFileSync(
    resolve(
      dirname(fileURLToPath(import.meta.url)),
      '../../../internal/codec/testdata/vectors.json',
    ),
    'utf8',
  ),
) as Vectors

const shared = (cases: CaseVector[]) => cases.filter((vector) => !vector.goOnly)

function bind(kind: Kind, options: unknown): BoundField {
  const bound = bindField(kind, options ?? null)
  if (!bound.ok)
    throw new Error(`bind ${kind} ${JSON.stringify(options)}: ${bound.reason}`)
  return bound.value
}

const envFor = (zone?: string): FieldEnv => (zone ? { timeZone: zone } : {})

function expectResult(result: Result<WireValue>, vector: CaseVector) {
  if (vector.reason) {
    expect(
      result.ok ? `ok:${JSON.stringify(result.value)}` : result.reason,
    ).toBe(vector.reason)
    return
  }
  if (!result.ok) throw new Error(`unexpected ${result.reason}`)
  expect(result.value).toEqual(vector.output ?? null)
}

const label = (vector: CaseVector, value: unknown) =>
  `${vector.kind} ${JSON.stringify(value)}${vector.zone ? ` @${vector.zone}` : ''}`

describe('shared codec vectors', () => {
  it('declares every Go kind with the same capabilities', () => {
    expect([...KINDS].sort()).toEqual(Object.keys(vectors.kinds).sort())
    for (const [kind, want] of Object.entries(vectors.kinds) as [
      Kind,
      KindVector,
    ][]) {
      const field = bind(kind, want.options)
      expect(field.readOnly, kind).toBe(want.readOnly)
      const { required, unique } = field.constraints
      expect([required, unique, field.constraints.default], kind).toEqual(
        want.constraints,
      )
      expect(field.filterOps(), kind).toEqual(want.filterOps)
      expect(field.aggregates(), kind).toEqual(want.aggregates)
    }
  })

  it.each(
    shared(vectors.options).map((vector) => [
      label(vector, vector.input),
      vector,
    ]),
  )('options %s', (_, vector) => {
    const bound = bindField(vector.kind, vector.input ?? null)
    if (vector.reason) {
      expect(bound.ok ? 'ok' : `${bound.reason}/${bound.option ?? ''}`).toBe(
        `${vector.reason}/${vector.option ?? ''}`,
      )
      return
    }
    if (!bound.ok) throw new Error(`unexpected ${bound.reason}/${bound.option}`)
    expect(JSON.parse(JSON.stringify(bound.value.options))).toEqual(
      vector.canonical,
    )
  })

  it.each(
    shared(vectors.decode).map((vector) => [label(vector, vector.raw), vector]),
  )('decode %s', (_, vector) => {
    const field = bind(vector.kind, vector.options)
    const result = field.normalize(JSON.parse(vector.raw ?? 'null'))
    expectResult(result, vector)
    if (result.ok && result.value !== null && !field.readOnly)
      expect(field.normalize(result.value)).toEqual(result)
  })

  it.each(
    shared(vectors.paste).map((vector) => [label(vector, vector.text), vector]),
  )('paste %s', (_, vector) => {
    const field = bind(vector.kind, vector.options)
    expectResult(
      field.parsePaste(vector.text ?? '', envFor(vector.zone)),
      vector,
    )
  })

  it.each(
    shared(vectors.format).map((vector) => [
      label(vector, vector.wire),
      vector,
    ]),
  )('format %s', (_, vector) => {
    const field = bind(vector.kind, vector.options)
    const env = envFor(vector.zone)
    const value = field.normalize(vector.wire ?? null)
    if (!value.ok) throw new Error(`decode ${value.reason}`)
    const text = field.formatCopy(value.value, env)
    expect(text).toBe(vector.text)
    if (value.value === null) return
    expect(field.parsePaste(text, env)).toEqual(value)
  })
})

describe('field registry', () => {
  it('rejects unknown kinds and malformed options', () => {
    for (const kind of ['', 'TEXT', '__proto__', 'toString'])
      expect(isKind(kind)).toBe(false)
    expect(isKind('formula')).toBe(true)
    expect(bindField('formula', { expr: '1 + 1' }).ok).toBe(true)
    expect(bindField('text', 'x')).toEqual({
      ok: false,
      reason: 'invalid_options',
    })
    expect(bindField('text', [])).toEqual({
      ok: false,
      reason: 'invalid_options',
    })
    expect(bindField('text', { maxLength: 1, extra: true })).toEqual({
      ok: false,
      reason: 'invalid_options',
      option: 'extra',
    })
  })

  it('keeps system kinds read-only', () => {
    for (const kind of [
      'id',
      'created_at',
      'created_by',
      'updated_at',
      'updated_by',
    ] as const) {
      const field = bind(kind, {})
      expect(field.readOnly).toBe(true)
      expect(field.constraints).toEqual({
        required: false,
        unique: false,
        default: false,
      })
      expect(field.normalize('1')).toEqual({ ok: false, reason: 'read_only' })
      expect(field.parsePaste('1', {})).toEqual({
        ok: false,
        reason: 'read_only',
      })
    }
    expect(bind('id', {}).formatCopy('9223372036854775807', {})).toBe(
      '9223372036854775807',
    )
    expect(
      bind('created_at', {}).formatCopy(
        '2026-03-08T07:30:00.5Z',
        envFor('America/New_York'),
      ),
    ).toBe('2026-03-08 03:30:00.5-04:00')
    expect(bind('created_at', {}).formatCopy('2026-03-08T07:30:00Z', {})).toBe(
      '2026-03-08 07:30:00+00:00',
    )
  })
})

describe('exact numbers', () => {
  it('keeps bigint and decimal values as exact strings', () => {
    const big = bind('integer', { size: 'big' })
    expect(big.normalize('-9223372036854775808')).toEqual({
      ok: true,
      value: '-9223372036854775808',
    })
    expect(big.normalize('9223372036854775808')).toEqual({
      ok: false,
      reason: 'out_of_range',
    })
    expect(big.normalize(2 ** 53)).toEqual({
      ok: false,
      reason: 'imprecise_number',
    })
    expect(big.parsePaste('9,223,372,036,854,775,807', {})).toEqual({
      ok: true,
      value: '9223372036854775807',
    })
    expect(big.formatDisplay('-1234567', {})).toBe('-1,234,567')
    const decimal = bind('decimal', { precision: 38, scale: 10 })
    expect(
      decimal.normalize('1234567890123456789012345678.1234567890'),
    ).toEqual({
      ok: true,
      value: '1234567890123456789012345678.1234567890',
    })
    expect(decimal.normalize(0.1)).toEqual({ ok: true, value: '0.1000000000' })
    for (const value of [NaN, Infinity, -Infinity])
      expect(decimal.normalize(value)).toEqual({
        ok: false,
        reason: 'invalid_number',
      })
    expect(decimal.normalize(0.30000000000000004)).toEqual({
      ok: false,
      reason: 'imprecise_number',
    })
    expect(decimal.parsePaste('１２３．５', {})).toEqual({
      ok: true,
      value: '123.5000000000',
    })
  })

  it('formats currency and percent decimal strings without Number rounding', () => {
    expect(
      bind('currency', { currency: 'USD', scale: 2 }).formatDisplay(
        '9999999999999999.99',
        { locale: 'en-US' },
      ),
    ).toBe('$9,999,999,999,999,999.99')
    expect(
      bind('percent', { displayDecimals: 2 }).formatDisplay('9.9999', {
        locale: 'en-US',
      }),
    ).toBe('999.99%')
  })

  it('rounds real values to the shortest float32 text', () => {
    expect(shortestFloat32(0.1)).toBe(0.1)
    expect(shortestFloat32(16777217)).toBe(16777216)
    expect(bind('float', { size: 'real' }).normalize(3.4e39)).toEqual({
      ok: false,
      reason: 'out_of_range',
    })
    expect(bind('float', {}).parsePaste('1e-400', {})).toEqual({
      ok: true,
      value: 0,
    })
    expect(
      bind('float', { displayDecimals: 2 }).formatDisplay(1234.5, {}),
    ).toBe('1,234.50')
  })
})

describe('time zones', () => {
  it('resolves DST gaps and overlaps like Go', () => {
    const zone = loadZone('America/New_York')
    if (!zone) throw new Error('zone')
    const wall = (text: string) => Date.parse(`${text}Z`) / 1000
    expect(resolveLocal(wall('2026-03-08T02:30:00'), zone)).toEqual({
      ok: false,
      reason: 'nonexistent_local_time',
    })
    expect(resolveLocal(wall('2026-11-01T01:30:00'), zone)).toEqual({
      ok: true,
      value: wall('2026-11-01T05:30:00'),
    })
    expect(loadZone('Local')).toBeNull()
    expect(loadZone('posix/UTC')).toBeNull()
    expect(loadZone('Not/AZone')).toBeNull()
  })

  it('keeps microseconds and year boundaries exact', () => {
    const min = parseWireDateTime('0001-01-01T00:00:00Z')
    if (!min.ok) throw new Error('min')
    expect(formatLocal(min.value, loadZone('America/New_York'))).toBe(
      '0001-01-01 00:00:00+00:00',
    )
    expect(parseWireDateTime('9999-12-31T23:59:59.999999Z').ok).toBe(true)
    expect(parseWireDateTime('9999-12-31T23:59:59.999999-00:01')).toEqual({
      ok: false,
      reason: 'out_of_range',
    })
    expect(
      bind('datetime', {}).normalize('2026-01-02T03:04:05.120000+08:00'),
    ).toEqual({
      ok: true,
      value: '2026-01-01T19:04:05.12Z',
    })
  })
})

describe('link safety', () => {
  it('only links re-validated http(s) and mail addresses', () => {
    expect(safeHref('url', 'https://example.com/a?b=c')).toBe(
      'https://example.com/a?b=c',
    )
    for (const value of [
      'javascript:alert(1)',
      'JAVASCRIPT:alert(1)',
      'data:text/html,x',
      'vbscript:x',
      'file:///etc/passwd',
      '//example.com',
      'https://user:pass@example.com',
      'https://exa mple.com',
      'https://example.com/\u0000',
    ])
      expect(safeHref('url', value), value).toBeNull()
    expect(safeHref('email', 'a.b@example.com')).toBe('mailto:a.b@example.com')
    expect(safeHref('email', 'a@example.com?cc=b@example.com')).toBeNull()
    expect(safeHref('text', 'https://example.com')).toBeNull()
  })
})

describe('computed field display', () => {
  it('formats lookup arrays and remains read-only', () => {
    const lookup = bind('lookup', {
      relationFieldId: 'relation',
      targetFieldId: 'value',
    })
    expect(lookup.readOnly).toBe(true)
    expect(lookup.formatDisplay(['Alice', 'Bob'], {})).toBe('Alice, Bob')
    expect(lookup.formatDisplay('🔒', {})).toBe('🔒')
    expect(lookup.parsePaste('ignored', {})).toEqual({
      ok: false,
      reason: 'read_only',
    })
  })
})
