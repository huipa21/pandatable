import type { CellPatch, GridRow, WindowSource } from './model'

export const pageSize = 100
export const cachePages = 3
export const editLimit = 5000
export const placeholder: Readonly<GridRow> = Object.freeze({
  id: 0,
  name: '',
  note: '',
  quantity: 0,
  group: '',
})

export interface WindowRequest {
  id: number
  offset: number
  limit: number
  ms: number
  status: 'pending' | 'loaded' | 'cancelled' | 'stale' | 'failed'
}
export interface WindowStats {
  total: number
  activePage: number
  cachedRows: number
  cachedPages: number[]
  pending: number
  editedCells: number
  disposed: boolean
  requests: WindowRequest[]
}

export class GridInputError extends Error {}

export class RowWindow {
  readonly slots: GridRow[]
  private cache = new Map<number, GridRow[]>()
  private edits = new Map<number, Partial<GridRow>>()
  private pending = new Map<number, AbortController>()
  private requests: WindowRequest[] = []
  private activePage = -1
  private generation = 0
  private nextRequest = 0
  private disposed = false

  constructor(
    private source: WindowSource,
    private changed: () => void,
    private failed: (error: unknown) => void,
  ) {
    if (!Number.isInteger(source.total) || source.total < 1) {
      throw new GridInputError('Invalid row total')
    }
    // RevoGrid needs addressable slots, not eagerly materialized row objects.
    this.slots = Array<GridRow>(source.total).fill(placeholder)
  }

  snapshot(): WindowStats {
    return {
      total: this.source.total,
      activePage: this.activePage,
      cachedRows: [...this.cache.values()].reduce(
        (n, rows) => n + rows.length,
        0,
      ),
      cachedPages: [...this.cache.keys()].sort((a, b) => a - b),
      pending: this.pending.size,
      editedCells: [...this.edits.values()].reduce(
        (n, patch) => n + Object.keys(patch).length,
        0,
      ),
      disposed: this.disposed,
      requests: this.requests.map((request) => ({ ...request })),
    }
  }

  load(index: number) {
    if (this.disposed) throw new GridInputError('Window disposed')
    if (!Number.isInteger(index) || index < 0 || index >= this.source.total) {
      throw new GridInputError('Invalid window index')
    }
    const page = Math.floor(index / pageSize)
    if (page === this.activePage) return
    this.activePage = page
    const generation = ++this.generation
    for (const controller of this.pending.values()) controller.abort()
    const wanted = [page - 1, page, page + 1].filter(
      (p) => p >= 0 && p * pageSize < this.source.total,
    )
    for (const [cached, rows] of this.cache) {
      if (wanted.includes(cached)) continue
      this.slots.fill(
        placeholder,
        cached * pageSize,
        cached * pageSize + rows.length,
      )
      this.cache.delete(cached)
    }
    for (const p of wanted) {
      if (!this.cache.has(p)) void this.fetch(p, generation)
    }
    this.changed()
  }

  private async fetch(page: number, generation: number) {
    const controller = new AbortController()
    const id = ++this.nextRequest
    this.pending.set(id, controller)
    const offset = page * pageSize
    const limit = Math.min(pageSize, this.source.total - offset)
    const request: WindowRequest = {
      id,
      offset,
      limit,
      ms: 0,
      status: 'pending',
    }
    this.requests.push(request)
    this.requests = this.requests.slice(-20)
    const started = performance.now()
    try {
      const rows = await this.source.fetch(offset, limit, controller.signal)
      if (this.disposed || generation !== this.generation) {
        request.status = 'stale'
        return
      }
      if (
        rows.length !== limit ||
        rows.some((row, i) => row.id !== offset + i + 1)
      ) {
        throw new GridInputError('Window response does not match request')
      }
      this.cache.set(page, rows)
      rows.forEach((row, i) => {
        Object.assign(row, this.edits.get(offset + i))
        this.slots[offset + i] = row
      })
      request.status = 'loaded'
    } catch (error) {
      if (controller.signal.aborted) {
        request.status = 'cancelled'
      } else {
        request.status = 'failed'
        if (!this.disposed) this.failed(error)
      }
    } finally {
      request.ms = Math.round((performance.now() - started) * 10) / 10
      this.pending.delete(id)
      if (!this.disposed) this.changed()
    }
  }

  apply(patches: CellPatch[]) {
    if (this.disposed || patches.length > editLimit) {
      throw new GridInputError('Spike edit limit exceeded or window disposed')
    }
    const next = new Map(
      [...this.edits].map(([index, patch]) => [index, { ...patch }]),
    )
    for (const patch of patches) {
      const row = this.slots[patch.index]
      if (
        !row ||
        row === placeholder ||
        !['name', 'note', 'quantity'].includes(patch.field) ||
        (patch.field === 'quantity'
          ? typeof patch.value !== 'number' || !Number.isFinite(patch.value)
          : typeof patch.value !== 'string' || patch.value.length > 10_000)
      ) {
        throw new GridInputError('Edit requires a loaded, writable, valid cell')
      }
      const values = next.get(patch.index) ?? {}
      Object.assign(values, { [patch.field]: patch.value })
      next.set(patch.index, values)
    }
    const count = [...next.values()].reduce(
      (n, patch) => n + Object.keys(patch).length,
      0,
    )
    if (count > editLimit) throw new GridInputError('Spike edit limit exceeded')
    this.edits = next
    for (const { index, field, value } of patches) {
      Object.assign(this.slots[index]!, { [field]: value })
    }
    this.changed()
  }

  dispose() {
    this.disposed = true
    this.generation++
    for (const controller of this.pending.values()) controller.abort()
    for (const request of this.requests) {
      if (request.status === 'pending') request.status = 'cancelled'
    }
    this.pending.clear()
    this.cache.clear()
    this.edits.clear()
    this.slots.length = 0
  }
}
