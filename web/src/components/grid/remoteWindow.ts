import type { DataRow, RowQueryResult } from '../../api/types'

export const remotePageSize = 100
export const remoteCachePages = 3

export interface RemoteRowSource {
  fetch(
    offset: number,
    limit: number,
    includeCount: boolean,
    signal: AbortSignal,
  ): Promise<RowQueryResult>
}

export interface RemoteWindowSnapshot {
  total: number
  activePage: number
  cachedRows: number
  cachedPages: number[]
  pending: number
  disposed: boolean
}

const emptyRow: DataRow = Object.freeze({ id: '', fields: Object.freeze({}) })

export class RemoteRowsWindow {
  readonly slots: DataRow[] = []
  private cache = new Map<number, DataRow[]>()
  private pending = new Map<number, AbortController>()
  private activePage = -1
  private generation = 0
  private disposed = false
  private initialized = false

  total = 0
  fieldIds: string[] = []

  constructor(
    private readonly source: RemoteRowSource,
    private readonly changed: () => void,
    private readonly failed: (error: unknown) => void,
  ) {}

  snapshot(): RemoteWindowSnapshot {
    return {
      total: this.total,
      activePage: this.activePage,
      cachedRows: [...this.cache.values()].reduce(
        (count, rows) => count + rows.length,
        0,
      ),
      cachedPages: [...this.cache.keys()].sort((a, b) => a - b),
      pending: this.pending.size,
      disposed: this.disposed,
    }
  }

  async initialize(anchor = 0) {
    if (this.disposed) return
    this.cancelPending()
    this.generation++
    const generation = this.generation
    this.initialized = false
    this.activePage = -1
    this.cache.clear()
    this.slots.length = 0
    this.fieldIds = []
    this.total = 0
    this.changed()
    let page = Math.max(0, Math.floor(anchor / remotePageSize))
    await this.fetchPage(page, generation, true)
    if (
      this.initialized &&
      this.total > 0 &&
      page * remotePageSize >= this.total
    ) {
      page = Math.floor((this.total - 1) / remotePageSize)
      await this.fetchPage(page, generation, false)
    }
    if (this.disposed || generation !== this.generation || !this.initialized)
      return
    this.activePage = page
    this.loadNeighbors(page, generation)
  }

  load(index: number) {
    if (this.disposed || !this.initialized || !Number.isInteger(index)) return
    if (index < 0 || index >= this.total) return
    const page = Math.floor(index / remotePageSize)
    if (page !== this.activePage) {
      this.cancelPending()
      this.generation++
      this.activePage = page
      this.evictOutside(page)
    }
    this.loadNeighbors(page, this.generation)
    this.changed()
  }

  async ensureLoaded(index: number): Promise<DataRow | null> {
    if (
      this.disposed ||
      !this.initialized ||
      !Number.isInteger(index) ||
      index < 0 ||
      index >= this.total
    )
      return null
    if (this.rowAt(index)) {
      this.load(index)
      return this.rowAt(index)
    }
    // Keep logical length stable so keyboard focus is not clamped to an empty source.
    this.cancelPending()
    const generation = ++this.generation
    const page = Math.floor(index / remotePageSize)
    this.activePage = page
    this.evictOutside(page)
    this.changed()
    await this.fetchPage(page, generation, false)
    if (this.disposed || generation !== this.generation) return null
    this.loadNeighbors(page, generation)
    this.changed()
    return this.rowAt(index)
  }

  rowAt(index: number): DataRow | null {
    const row = this.slots[index]
    return row && row.id ? row : null
  }

  indexOfRow(id: string): number {
    for (const [page, rows] of this.cache) {
      const localIndex = rows.findIndex((row) => row.id === id)
      if (localIndex >= 0) return page * remotePageSize + localIndex
    }
    return -1
  }

  updateRow(index: number, row: DataRow) {
    if (this.disposed || !this.rowAt(index)) return
    this.slots[index] = row
    const page = Math.floor(index / remotePageSize)
    const rows = this.cache.get(page)
    if (rows) rows[index % remotePageSize] = row
    this.changed()
  }

  dispose() {
    if (this.disposed) return
    this.disposed = true
    this.generation++
    this.cancelPending()
    this.cache.clear()
    this.slots.length = 0
    this.total = 0
    this.fieldIds = []
    this.changed()
  }

  private loadNeighbors(page: number, generation: number) {
    const pages = [page - 1, page, page + 1].filter(
      (candidate) => candidate >= 0 && candidate * remotePageSize < this.total,
    )
    this.evictOutside(page)
    for (const candidate of pages) {
      if (!this.cache.has(candidate) && !this.pending.has(candidate))
        void this.fetchPage(candidate, generation, false)
    }
  }

  private evictOutside(page: number) {
    const keep = [page - 1, page, page + 1]
    for (const [cached, rows] of this.cache) {
      if (keep.includes(cached)) continue
      const start = cached * remotePageSize
      this.slots.fill(emptyRow, start, start + rows.length)
      this.cache.delete(cached)
    }
  }

  private async fetchPage(
    page: number,
    generation: number,
    includeCount: boolean,
  ) {
    const controller = new AbortController()
    this.pending.set(page, controller)
    const offset = page * remotePageSize
    const limit = remotePageSize
    try {
      const result = await this.source.fetch(
        offset,
        limit,
        includeCount,
        controller.signal,
      )
      if (
        this.disposed ||
        generation !== this.generation ||
        controller.signal.aborted
      )
        return
      if (includeCount) {
        if (
          result.total === null ||
          !Number.isSafeInteger(result.total) ||
          result.total < 0 ||
          result.total > 1_000_001
        )
          throw new Error('Query returned an unsupported row count')
        this.total = result.total
        this.slots.length = this.total
        this.slots.fill(emptyRow)
        this.initialized = true
        this.fieldIds = [...result.fieldIds]
      }
      if (result.total !== null && result.total !== this.total)
        throw new Error('Query count changed within the grid window')
      const expected = Math.max(0, Math.min(limit, this.total - offset))
      if (result.rows.length !== expected)
        throw new Error('Query page length does not match the row count')
      this.cache.set(page, result.rows)
      result.rows.forEach((row, index) => {
        this.slots[offset + index] = row
      })
      this.changed()
    } catch (error) {
      if (!controller.signal.aborted && !this.disposed) this.failed(error)
    } finally {
      if (this.pending.get(page) === controller) this.pending.delete(page)
      if (!this.disposed) this.changed()
    }
  }

  private cancelPending() {
    for (const controller of this.pending.values()) controller.abort()
    this.pending.clear()
  }
}
