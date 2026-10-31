import { describe, expect, it, vi } from 'vitest'
import { RealtimeRowSync } from './realtimeRows'

describe('realtime row merging', () => {
  it('batches visible IDs, invalidates offscreen data and reloads missing rows', async () => {
    const invalidateOffscreen = vi.fn()
    const merge = vi.fn()
    const reload = vi.fn()
    const fetch = vi.fn(async (ids: string[]) => ({
      rows: [{ id: ids[0]!, fields: { value: 'new' } }],
      fieldIds: ['value'],
      total: null,
      schemaVersion: 1,
    }))
    const indexes: Record<string, number> = { a: 100, b: 101, c: 1 }
    const sync = new RealtimeRowSync({
      window: () => ({
        snapshot: () => ({ activePage: 1 }),
        indexOfRow: (id) => indexes[id] ?? -1,
        invalidateOffscreen,
      }),
      epoch: () => 1,
      fetch,
      merge,
      reload,
      needsReload: () => false,
      failed: vi.fn(),
    })
    sync.update(['a', 'c'])
    sync.update(['b'])
    await vi.waitFor(() => expect(reload).toHaveBeenCalledOnce())
    expect(fetch).toHaveBeenCalledOnce()
    expect(fetch.mock.calls[0]?.[0]).toEqual(['a', 'b'])
    expect(merge).toHaveBeenCalledWith({ id: 'a', fields: { value: 'new' } })
    expect(invalidateOffscreen).toHaveBeenCalledWith(['a', 'c'])
    sync.dispose()
  })
})
