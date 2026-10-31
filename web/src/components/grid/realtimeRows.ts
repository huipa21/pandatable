import type { DataRow, RowQueryResult } from '../../api/types'
import { remotePageSize } from './remoteWindow'

interface RowWindow {
  snapshot(): { activePage: number }
  indexOfRow(id: string): number
  invalidateOffscreen(ids: readonly string[]): void
}

interface Options {
  window(): RowWindow | undefined
  epoch(): number
  fetch(ids: string[], signal: AbortSignal): Promise<RowQueryResult>
  merge(row: DataRow): void
  reload(): void
  needsReload(): boolean
  failed(error: unknown): void
}

export class RealtimeRowSync {
  private readonly pending = new Set<string>()
  private controller = new AbortController()
  private running = false
  private disposed = false

  constructor(private readonly options: Options) {}

  update(ids: readonly string[]) {
    if (this.disposed) return
    this.options.window()?.invalidateOffscreen(ids)
    for (const id of ids) this.pending.add(id)
    if (!this.running) {
      this.running = true
      queueMicrotask(() => void this.drain())
    }
  }

  dispose() {
    this.disposed = true
    this.pending.clear()
    this.controller.abort()
  }

  hasWork() {
    return this.running || this.pending.size > 0
  }

  private async drain() {
    try {
      while (!this.disposed && this.pending.size) {
        const ids = [...this.pending]
        this.pending.clear()
        const window = this.options.window()
        if (!window) continue
        const epoch = this.options.epoch()
        const page = window.snapshot().activePage
        const visible = ids.filter((id) => {
          const index = window.indexOfRow(id)
          return (
            page >= 0 &&
            index >= 0 &&
            index >= page * remotePageSize &&
            index < (page + 1) * remotePageSize
          )
        })
        let missing = false
        for (
          let offset = 0;
          offset < visible.length;
          offset += remotePageSize
        ) {
          const batch = visible.slice(offset, offset + remotePageSize)
          const result = await this.options.fetch(batch, this.controller.signal)
          if (this.disposed || epoch !== this.options.epoch()) break
          const returned = new Set(result.rows.map((row) => row.id))
          missing ||= batch.some((id) => !returned.has(id))
          for (const row of result.rows) this.options.merge(row)
        }
        if (
          !this.disposed &&
          epoch === this.options.epoch() &&
          (missing || this.options.needsReload())
        )
          this.options.reload()
      }
    } catch (error) {
      if (!this.disposed && !this.controller.signal.aborted)
        this.options.failed(error)
    } finally {
      this.running = false
      if (!this.disposed && this.pending.size) {
        this.running = true
        queueMicrotask(() => void this.drain())
      }
    }
  }
}
