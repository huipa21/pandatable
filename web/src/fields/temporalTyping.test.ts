import { describe, expect, it } from 'vitest'
import { timeKind } from './batch2'
import { dateKind, datetimeKind } from './temporal'
import {
  isCompleteTyped,
  normalizeTyped,
  typedPlaceholder,
  type TypedContext,
} from './temporalTyping'

const today = { year: 2026, month: 11, day: 10 }
const iso: TypedContext = { dateFormat: 'iso', twelveHours: false, today }
const us: TypedContext = { ...iso, dateFormat: 'us' }
const eu: TypedContext = { ...iso, dateFormat: 'eu' }
const env = { timeZone: 'UTC' }

describe('CES-01.1 typed date', () => {
  it.each([
    ['20261115', iso, '2026-11-15'],
    ['11/15', us, '2026-11-15'],
    ['11-15', iso, '2026-11-15'],
    ['11.15', iso, '2026-11-15'],
    ['15/11', eu, '2026-11-15'],
    ['2026/1/5', iso, '2026-01-05'],
    ['2026-11-15', iso, '2026-11-15'],
    ['11/15/2026', us, '2026-11-15'],
    ['15/11/2026', eu, '2026-11-15'],
    ['2026年1月5日', iso, '2026-01-05'],
    ['11月15日', iso, '2026-11-15'],
    ['２０２６／１１／１５', iso, '2026-11-15'],
    ['  2026-11-15  ', iso, '2026-11-15'],
  ] as const)('%s → %s', (text, ctx, expected) => {
    expect(normalizeTyped('date', text, ctx)).toBe(expected)
    expect(
      dateKind.parsePaste(expected, { dateFormat: ctx.dateFormat }, env),
    ).toEqual({ ok: true, value: expected })
  })

  it.each([
    ['abc', iso],
    ['', iso],
    ['11/15/2026', iso],
    ['2026-11/15', iso],
    ['13/15', us],
    ['2026111', iso],
    ['2026-00-10', iso],
  ] as const)('%s is rejected', (text, ctx) => {
    expect(normalizeTyped('date', text, ctx)).toBeNull()
  })
})

describe('CES-01.2 typed time', () => {
  it.each([
    ['9', '09:00:00'],
    ['09', '09:00:00'],
    ['930', '09:30:00'],
    ['1345', '13:45:00'],
    ['9:30', '09:30:00'],
    ['13:45:30', '13:45:30'],
    ['13:45:30.250', '13:45:30.25'],
    ['1:45pm', '13:45:00'],
    ['1:45 PM', '13:45:00'],
    ['9a', '09:00:00'],
    ['930p', '21:30:00'],
    ['12am', '00:00:00'],
    ['12pm', '12:00:00'],
    ['１３：４５', '13:45:00'],
  ])('%s → %s', (text, expected) => {
    expect(normalizeTyped('time', text, iso)).toBe(expected)
    const parsed = timeKind.parsePaste(expected, { format: '24h' }, env)
    expect(parsed.ok).toBe(true)
  })

  it.each([
    '24:00',
    '2400',
    '13:60',
    '13:45:60',
    '13:4',
    '13pm',
    '0am',
    '12345',
    '13:45:30.1234567',
    'noon',
  ])('%s is rejected', (text) => {
    expect(normalizeTyped('time', text, iso)).toBeNull()
  })
})

describe('CES-01.3 typed datetime', () => {
  it.each([
    ['20261115 1345', iso, '2026-11-15 13:45:00'],
    ['2026-11-15T09:30', iso, '2026-11-15 09:30:00'],
    ['2026-11-15', iso, '2026-11-15'],
    ['11/15 1:45 pm', us, '2026-11-15 13:45:00'],
    ['2026年11月15日13:45', iso, '2026-11-15 13:45:00'],
    ['2026-11-15 09:30Z', iso, '2026-11-15 09:30:00Z'],
    ['2026-11-15 09:30 +08:00', iso, '2026-11-15 09:30:00+08:00'],
  ] as const)('%s → %s', (text, ctx, expected) => {
    expect(normalizeTyped('datetime', text, ctx)).toBe(expected)
    const options = {
      dateFormat: ctx.dateFormat,
      timeFormat: '24h',
      timezone: 'user',
    } as const
    expect(datetimeKind.parsePaste(expected, options, env).ok).toBe(true)
  })

  it.each(['2026-11-15T', '2026-11-15 25:00', 'T09:30', 'abc 09:30'])(
    '%s is rejected',
    (text) => {
      expect(normalizeTyped('datetime', text, iso)).toBeNull()
    },
  )
})

describe('CES-01.4 completeness and placeholders', () => {
  it.each([
    ['time', '13:4', false],
    ['time', '13:45', true],
    ['time', '1345', true],
    ['time', '9', false],
    ['time', '13', false],
    ['time', '1:45pm', true],
    ['time', 'abc', false],
    ['date', '2026-11-15', true],
    ['date', '2026-1-1', false],
    ['date', '20261115', true],
    ['datetime', '2026-11-15 13:4', false],
    ['datetime', '2026-11-15 13:45', true],
  ] as const)('%s %s complete=%s', (kind, text, expected) => {
    expect(isCompleteTyped(kind, text, iso)).toBe(expected)
  })

  it('formats placeholders for every date and time format', () => {
    expect(typedPlaceholder('date', iso)).toBe('YYYY-MM-DD')
    expect(typedPlaceholder('date', { ...iso, dateFormat: 'slash' })).toBe(
      'YYYY/MM/DD',
    )
    expect(typedPlaceholder('date', us)).toBe('MM/DD/YYYY')
    expect(typedPlaceholder('date', eu)).toBe('DD/MM/YYYY')
    expect(typedPlaceholder('time', iso)).toBe('HH:mm')
    expect(typedPlaceholder('time', { ...iso, twelveHours: true })).toBe(
      'hh:mm a',
    )
    expect(typedPlaceholder('datetime', iso)).toBe('YYYY-MM-DD HH:mm')
    expect(typedPlaceholder('datetime', { ...us, twelveHours: true })).toBe(
      'MM/DD/YYYY hh:mm a',
    )
  })
})
