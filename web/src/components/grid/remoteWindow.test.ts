import { afterEach, describe, expect, it, vi } from 'vitest'
import type { DataRow, RowQueryResult } from '../../api/types'
import {
  RemoteRowsWindow,
  remoteCachePages,
  remotePageSize,
} from './remoteWindow'

const row = (id: number): DataRow => ({
  id: String(10_000_000_000 + id),
  fields: { value: id },
})

function source(total = 200_000) {
  const requests: {
    offset: number
    limit: number
    includeCount: boolean
    signal: AbortSignal
  }[] = []
  return {
    requests,
    fetch: vi.fn(
      async (
        offset: number,
        limit: number,
        includeCount: boolean,
        signal: AbortSignal,
      ): Promise<RowQueryResult> => {
        requests.push({ offset, limit, includeCount, signal })
        const rows = Array.from(
          { length: Math.min(limit, total - offset) },
          (_, index) => row(offset + index),
        )
        return {
          rows,
          fieldIds: ['value'],
          total: includeCount ? total : null,
          schemaVersion: 1,
        }
      },
    ),
  }
}

afterEach(() => vi.useRealTimers())

describe('production query window', () => {
  it('loads an uncached keyboard destination without resetting row count, source length or count queries', async () => {
    const provider = source(401)
    const lengths: number[] = []
    const window = new RemoteRowsWindow(
      provider,
      () => lengths.push(window.slots.length),
      vi.fn(),
    )
    await window.initialize()
    await Promise.resolve()
    expect(window.rowAt(400)).toBeNull()
    lengths.length = 0
    expect((await window.ensureLoaded(400))?.id).toBe('10000000400')
    await Promise.resolve()
    expect(lengths.length).toBeGreaterThan(0)
    expect(lengths.every((length) => length === 401)).toBe(true)
    expect(
      provider.requests.filter((request) => request.includeCount),
    ).toHaveLength(1)
    expect(provider.requests.every((request) => request.limit === 100)).toBe(
      true,
    )
    expect(window.rowAt(0)).toBeNull()
    expect(window.snapshot().cachedRows).toBeLessThanOrEqual(300)
    expect((await window.ensureLoaded(0))?.id).toBe('10000000000')
    expect(window.rowAt(400)).toBeNull()
    window.dispose()
  })

  it('keeps only the active page and its neighbors for 200k arbitrary bigint IDs', async () => {
    const provider = source()
    const changed = vi.fn()
    const window = new RemoteRowsWindow(provider, changed, vi.fn())
    await window.initialize()
    await Promise.resolve()
    expect(window.total).toBe(200_000)
    expect(window.rowAt(0)?.id).toBe('10000000000')
    expect(window.indexOfRow('10000000025')).toBe(25)
    expect(window.snapshot().cachedRows).toBeLessThanOrEqual(
      remotePageSize * remoteCachePages,
    )
    window.load(100_000)
    await Promise.resolve()
    expect(window.rowAt(100_000)?.id).toBe('10000100000')
    expect(window.indexOfRow('10000100025')).toBe(100_025)
    expect(window.indexOfRow('10000000025')).toBe(-1)
    expect(window.snapshot().cachedPages).toEqual([999, 1000, 1001])
    expect(provider.requests.every((item) => item.limit <= 100)).toBe(true)
    expect(provider.requests.filter((item) => item.includeCount)).toHaveLength(
      1,
    )
    window.dispose()
    expect(window.slots).toHaveLength(0)
    expect(window.snapshot()).toMatchObject({
      disposed: true,
      cachedRows: 0,
      pending: 0,
    })
  })

  it('ignores late results even when a provider ignores abort', async () => {
    const pending: {
      offset: number
      resolve: (result: RowQueryResult) => void
      signal: AbortSignal
    }[] = []
    const window = new RemoteRowsWindow(
      {
        fetch: (offset, limit, includeCount, signal) =>
          new Promise((resolve) => {
            pending.push({
              offset,
              signal,
              resolve: (result) => resolve(result),
            })
            if (includeCount) {
              pending.at(-1)!.resolve = (result) => resolve(result)
            }
            void limit
          }),
      },
      vi.fn(),
      vi.fn(),
    )
    const initial = window.initialize()
    pending[0]!.resolve({
      rows: Array.from({ length: 100 }, (_, index) => row(index)),
      fieldIds: ['value'],
      total: 200_000,
      schemaVersion: 1,
    })
    await initial
    window.load(100_000)
    const stale = pending.filter((item) => item.offset === 100).at(0)
    expect(stale?.signal.aborted).toBe(true)
    window.dispose()
    for (const item of pending) {
      item.resolve({
        rows: Array.from({ length: 100 }, (_, index) =>
          row(item.offset + index),
        ),
        fieldIds: ['value'],
        total: null,
        schemaVersion: 1,
      })
    }
    await Promise.resolve()
    expect(window.snapshot().pending).toBe(0)
    expect(window.slots).toHaveLength(0)
  })

  it('marks affected cached neighbor pages stale without discarding the active page', async () => {
    const provider = source(400)
    const window = new RemoteRowsWindow(provider, vi.fn(), vi.fn())
    await window.initialize(100)
    await Promise.resolve()
    expect(window.snapshot().cachedPages).toEqual([0, 1, 2])
    window.invalidateOffscreen(['10000000001', '10000000101'])
    expect(window.snapshot().cachedPages).toEqual([1, 2])
    expect(window.rowAt(1)).toBeNull()
    expect(window.rowAt(101)?.id).toBe('10000000101')
    window.dispose()
  })

  it('cancels stale in-flight pages and reloads an active page after a remote edit', async () => {
    const pending: {
      offset: number
      signal: AbortSignal
      resolve: (result: RowQueryResult) => void
    }[] = []
    const result = (offset: number, includeCount: boolean): RowQueryResult => ({
      rows: Array.from({ length: 100 }, (_, index) => row(offset + index)),
      fieldIds: ['value'],
      total: includeCount ? 400 : null,
      schemaVersion: 1,
    })
    const window = new RemoteRowsWindow(
      {
        fetch: (offset, _limit, includeCount, signal) =>
          includeCount
            ? Promise.resolve(result(offset, true))
            : new Promise((resolve) =>
                pending.push({ offset, signal, resolve }),
              ),
      },
      vi.fn(),
      vi.fn(),
    )
    await window.initialize()
    const neighbor = pending.find((item) => item.offset === 100)!
    window.load(200)
    const staleActive = pending.find((item) => item.offset === 200)!
    window.invalidateOffscreen(['10000000201'])
    expect(neighbor.signal.aborted).toBe(true)
    expect(staleActive.signal.aborted).toBe(true)
    const refreshed = pending.filter((item) => item.offset === 200).at(-1)!
    expect(refreshed).not.toBe(staleActive)
    neighbor.resolve(result(100, false))
    staleActive.resolve(result(200, false))
    const fresh = result(200, false)
    fresh.rows[1]!.fields.value = 'new'
    refreshed.resolve(fresh)
    await vi.waitFor(() => expect(window.rowAt(201)?.fields.value).toBe('new'))
    expect(window.rowAt(101)).toBeNull()
    window.dispose()
  })
})
