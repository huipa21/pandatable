import { afterEach, describe, expect, it, vi } from 'vitest'
import { syntheticSource } from '../../spikes/synthetic'
import type { GridRow, WindowSource } from './model'
import { GridInputError, RowWindow, placeholder } from './window'

const synthetic = (total = 200_000) =>
  syntheticSource(
    total,
    (id) => `Row ${id}`,
    (group) => `Group ${group}`,
  )

afterEach(() => vi.useRealTimers())

describe('bounded window over 200k logical rows', () => {
  it('loads start/middle/end with at most three 100-row pages and shared placeholders', async () => {
    vi.useFakeTimers()
    const failed = vi.fn()
    const window = new RowWindow(synthetic(), vi.fn(), failed)
    expect(window.slots).toHaveLength(200_000)
    expect(new Set(window.slots).size).toBe(1)
    for (const index of [0, 100_000, 199_999, 0]) {
      window.load(index)
      await vi.runAllTimersAsync()
      expect(window.snapshot().cachedRows).toBeLessThanOrEqual(300)
      expect(new Set(window.slots).size).toBeLessThanOrEqual(301)
      expect(window.slots[index]?.id).toBe(index + 1)
    }
    expect(window.snapshot().requests.every((r) => r.limit <= 100)).toBe(true)
    expect(failed).not.toHaveBeenCalled()
    window.dispose()
    expect(window.slots).toHaveLength(0)
    expect(window.snapshot()).toMatchObject({
      disposed: true,
      cachedRows: 0,
      pending: 0,
    })
  })

  it('ignores out-of-order results even when a provider ignores AbortSignal', async () => {
    const pending: {
      offset: number
      resolve: (rows: GridRow[]) => void
      signal: AbortSignal
    }[] = []
    const source: WindowSource = {
      total: 200_000,
      fetch: (offset, limit, signal) =>
        new Promise((resolve) => {
          expect(limit).toBe(100)
          pending.push({ offset, resolve, signal })
        }),
    }
    const changed = vi.fn()
    const window = new RowWindow(source, changed, vi.fn())
    window.load(0)
    window.load(100_000)
    const makeRows = (offset: number) =>
      Array.from({ length: 100 }, (_, i) => ({
        id: offset + i + 1,
        name: '',
        note: '',
        quantity: 0,
        group: '',
      }))
    for (const request of pending.slice(2).reverse())
      request.resolve(makeRows(request.offset))
    await Promise.resolve()
    expect(window.snapshot().activePage).toBe(1000)
    expect(window.slots[100_000]?.id).toBe(100_001)
    for (const request of pending.slice(0, 2)) {
      expect(request.signal.aborted).toBe(true)
      request.resolve(makeRows(request.offset))
    }
    await Promise.resolve()
    expect(window.slots[0]).toBe(placeholder)
    expect(window.snapshot().cachedPages).toEqual([999, 1000, 1001])
    expect(
      window.snapshot().requests.filter((r) => r.status === 'stale'),
    ).toHaveLength(2)
    window.load(199_999)
    window.dispose()
    changed.mockClear()
    for (const request of pending.slice(5))
      request.resolve(makeRows(request.offset))
    await Promise.resolve()
    expect(changed).not.toHaveBeenCalled()
    expect(window.snapshot().pending).toBe(0)
    expect(window.slots).toHaveLength(0)
  })

  it('cancels timers, bounds logs and stops notifications after unmount', async () => {
    vi.useFakeTimers()
    const changed = vi.fn()
    const window = new RowWindow(synthetic(), changed, vi.fn())
    for (let page = 0; page < 30; page++) {
      window.load(page * 100)
      await vi.runAllTimersAsync()
    }
    expect(window.snapshot().requests).toHaveLength(20)
    window.load(100_000)
    window.dispose()
    changed.mockClear()
    await vi.runAllTimersAsync()
    expect(vi.getTimerCount()).toBe(0)
    expect(window.snapshot().pending).toBe(0)
    expect(changed).not.toHaveBeenCalled()
    expect(() => window.load(0)).toThrow(GridInputError)
  })

  it('applies edits atomically, skips no invalid cell and retains bounded edits across eviction', async () => {
    vi.useFakeTimers()
    const window = new RowWindow(synthetic(), vi.fn(), vi.fn())
    window.load(0)
    await vi.runAllTimersAsync()
    expect(() =>
      window.apply([
        { index: 0, field: 'name', value: 'must not apply' },
        { index: 10_000, field: 'note', value: 'not loaded' },
      ]),
    ).toThrow(GridInputError)
    expect(window.slots[0]?.name).toBe('Row 1')
    expect(() =>
      window.apply([{ index: 0, field: 'quantity', value: Infinity }]),
    ).toThrow()
    window.apply([{ index: 0, field: 'note', value: 'a\n"b"' }])
    window.load(100_000)
    await vi.runAllTimersAsync()
    expect(window.slots[0]).toBe(placeholder)
    window.load(0)
    await vi.runAllTimersAsync()
    expect(window.slots[0]?.note).toBe('a\n"b"')
    expect(window.snapshot().editedCells).toBe(1)
    expect(() =>
      window.apply(
        Array.from({ length: 5001 }, () => ({
          index: 0,
          field: 'name',
          value: 'x',
        })),
      ),
    ).toThrow(GridInputError)
    window.dispose()
    expect(window.snapshot().editedCells).toBe(0)
  })

  it('enforces the cumulative 5,000-cell overlay cap across evicted windows', async () => {
    vi.useFakeTimers()
    const window = new RowWindow(synthetic(6000), vi.fn(), vi.fn())
    for (let page = 0; page < 50; page++) {
      window.load(page * 100)
      await vi.runAllTimersAsync()
      window.apply(
        Array.from({ length: 100 }, (_, i) => ({
          index: page * 100 + i,
          field: 'name',
          value: 'edited',
        })),
      )
    }
    expect(window.snapshot().editedCells).toBe(5000)
    expect(window.snapshot().cachedRows).toBe(300)
    window.load(5000)
    await vi.runAllTimersAsync()
    expect(() =>
      window.apply([{ index: 5000, field: 'name', value: 'over limit' }]),
    ).toThrow(GridInputError)
    expect(window.slots[5000]?.name).toBe('Row 5001')
    expect(window.snapshot().editedCells).toBe(5000)
    window.dispose()
  })

  it('reports provider failures and malformed responses, never fills with fake success data', async () => {
    const failed = vi.fn()
    const source: WindowSource = { total: 1, fetch: async () => [] }
    const window = new RowWindow(source, vi.fn(), failed)
    window.load(0)
    await Promise.resolve()
    expect(failed).toHaveBeenCalledOnce()
    expect(window.snapshot().requests[0]?.status).toBe('failed')
    expect(window.slots[0]).toBe(placeholder)
    expect(() => window.load(-1)).toThrow()
    window.dispose()
  })
})
