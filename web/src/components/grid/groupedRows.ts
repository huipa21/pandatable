import type {
  DataRow,
  RowGroupBucket,
  RowQueryResult,
  RowValue,
} from '../../api/types'
import {
  RemoteRowsWindow,
  remotePageSize,
  type RemoteWindowSnapshot,
} from './remoteWindow'

export interface GroupedRowSource {
  groups(path: RowValue[], signal: AbortSignal): Promise<RowGroupBucket[]>
  rows(
    path: RowValue[],
    offset: number,
    limit: number,
    includeCount: boolean,
    signal: AbortSignal,
  ): Promise<RowQueryResult>
}

export interface GroupNode {
  readonly path: RowValue[]
  readonly key: RowValue
  readonly level: number
  readonly count: number
  readonly aggregates: RowGroupBucket['aggregates']
  expanded: boolean
  children?: GroupNode[]
  rows?: RemoteRowsWindow
}

export type GroupedRowEntry =
  | { kind: 'group'; group: GroupNode }
  | { kind: 'row'; group: GroupNode; rowIndex: number }

function pathKey(path: RowValue[]) {
  return JSON.stringify(path)
}

function groupNodes(
  buckets: RowGroupBucket[],
  parent: RowValue[],
  level: number,
): GroupNode[] {
  return buckets.map((bucket) => ({
    path: [...parent, bucket.key],
    key: bucket.key,
    level,
    count: bucket.count,
    aggregates: bucket.aggregates,
    expanded: false,
  }))
}

export class GroupedRowsWindow {
  roots: GroupNode[] = []
  entries: GroupedRowEntry[] = []
  private leafStarts = new Map<GroupNode, number>()
  private expanded = new Set<string>()
  private rootController: AbortController | undefined
  private groupControllers = new Set<AbortController>()
  private generation = 0
  private activePage = 0
  private disposed = false

  constructor(
    private readonly source: GroupedRowSource,
    private readonly levels: number,
    private readonly storageKey: string,
    private readonly changed: () => void,
    private readonly failed: (error: unknown) => void,
  ) {
    if (!Number.isInteger(levels) || levels < 1 || levels > 3)
      throw new Error('Grouping level count is unsupported')
  }

  get total() {
    return this.entries.length
  }

  snapshot(): RemoteWindowSnapshot {
    let cachedRows = 0
    let pending = this.rootController ? 1 : 0
    const visit = (groups: GroupNode[]) => {
      for (const group of groups) {
        if (group.rows) {
          const state = group.rows.snapshot()
          cachedRows += state.cachedRows
          pending += state.pending
        }
        if (group.children) visit(group.children)
      }
    }
    visit(this.roots)
    return {
      total: this.total,
      activePage: this.activePage,
      cachedRows,
      cachedPages: [],
      pending,
      disposed: this.disposed,
    }
  }

  async initialize() {
    if (this.disposed) return
    this.generation++
    const generation = this.generation
    this.rootController?.abort()
    for (const controller of this.groupControllers) controller.abort()
    this.groupControllers.clear()
    this.disposeRows(this.roots)
    this.expanded = new Set()
    this.roots = []
    this.entries = []
    this.leafStarts.clear()
    const controller = new AbortController()
    this.rootController = controller
    this.changed()
    try {
      this.expanded = this.readExpanded()
      const buckets = await this.source.groups([], controller.signal)
      if (!this.current(generation, controller)) return
      this.roots = groupNodes(buckets, [], 0)
      this.rebuild()
      for (const group of this.roots) {
        if (this.expanded.has(pathKey(group.path)))
          await this.expand(group, generation, false)
      }
    } catch (error) {
      if (!controller.signal.aborted && this.current(generation, controller))
        this.failed(error)
    } finally {
      if (this.rootController === controller) this.rootController = undefined
      if (!this.disposed) this.changed()
    }
  }

  async toggle(path: RowValue[]) {
    const group = this.find(path)
    if (!group || this.disposed) return
    if (group.expanded) {
      group.expanded = false
      this.expanded.delete(pathKey(group.path))
      this.persistExpanded()
      this.rebuild()
      return
    }
    this.expanded.add(pathKey(group.path))
    this.persistExpanded()
    await this.expand(group, this.generation, true)
  }

  entryAt(index: number) {
    return this.entries[index]
  }

  rowAt(index: number): DataRow | null {
    const entry = this.entries[index]
    return entry?.kind === 'row'
      ? (entry.group.rows?.rowAt(entry.rowIndex) ?? null)
      : null
  }

  load(index: number) {
    if (this.disposed || !Number.isInteger(index)) return
    const entry = this.entries[index]
    if (entry?.kind !== 'row') return
    this.activePage = Math.floor(index / remotePageSize)
    entry.group.rows?.load(entry.rowIndex)
  }

  async ensureLoaded(index: number): Promise<DataRow | null> {
    if (this.disposed || !Number.isInteger(index)) return null
    const entry = this.entries[index]
    if (entry?.kind !== 'row') return null
    this.activePage = Math.floor(index / remotePageSize)
    return (await entry.group.rows?.ensureLoaded(entry.rowIndex)) ?? null
  }

  indexOfRow(id: string) {
    if (!id) return -1
    for (const [group, start] of this.leafStarts) {
      const localIndex = group.rows?.indexOfRow(id) ?? -1
      if (localIndex >= 0) return start + localIndex
    }
    return -1
  }

  updateRow(index: number, row: DataRow) {
    const entry = this.entries[index]
    if (entry?.kind === 'row') entry.group.rows?.updateRow(entry.rowIndex, row)
  }

  dispose() {
    if (this.disposed) return
    this.disposed = true
    this.generation++
    this.rootController?.abort()
    this.rootController = undefined
    for (const controller of this.groupControllers) controller.abort()
    this.groupControllers.clear()
    this.disposeRows(this.roots)
    this.roots = []
    this.entries = []
    this.leafStarts.clear()
    this.changed()
  }

  private current(generation: number, controller?: AbortController) {
    return (
      !this.disposed &&
      generation === this.generation &&
      !controller?.signal.aborted
    )
  }

  private find(path: RowValue[]) {
    const wanted = pathKey(path)
    const visit = (groups: GroupNode[]): GroupNode | undefined => {
      for (const group of groups) {
        if (pathKey(group.path) === wanted) return group
        const nested = group.children && visit(group.children)
        if (nested) return nested
      }
      return undefined
    }
    return visit(this.roots)
  }

  private async expand(group: GroupNode, generation: number, notify: boolean) {
    if (!this.current(generation)) return
    group.expanded = true
    if (group.level === this.levels - 1) {
      if (!group.rows) {
        group.rows = new RemoteRowsWindow(
          {
            fetch: (offset, limit, includeCount, signal) =>
              this.source.rows(group.path, offset, limit, includeCount, signal),
          },
          () => this.changed(),
          (error) => this.failed(error),
        )
        await group.rows.initialize()
        if (group.rows.total !== group.count)
          this.failed(new Error('Grouped row count changed while expanding'))
      }
    } else if (!group.children) {
      const controller = new AbortController()
      this.groupControllers.add(controller)
      try {
        const buckets = await this.source.groups(group.path, controller.signal)
        if (!this.current(generation, controller)) return
        group.children = groupNodes(buckets, group.path, group.level + 1)
      } catch (error) {
        if (!controller.signal.aborted && this.current(generation))
          this.failed(error)
        return
      } finally {
        this.groupControllers.delete(controller)
      }
    }
    if (!this.current(generation)) return
    this.rebuild()
    if (group.children) {
      for (const child of group.children) {
        if (this.expanded.has(pathKey(child.path)))
          await this.expand(child, generation, false)
      }
    }
    if (notify) this.changed()
  }

  private rebuild() {
    const entries: GroupedRowEntry[] = []
    const leafStarts = new Map<GroupNode, number>()
    const visit = (groups: GroupNode[]) => {
      for (const group of groups) {
        entries.push({ kind: 'group', group })
        if (!group.expanded) continue
        if (group.children) visit(group.children)
        else if (group.level === this.levels - 1) {
          leafStarts.set(group, entries.length)
          for (let rowIndex = 0; rowIndex < group.count; rowIndex++)
            entries.push({ kind: 'row', group, rowIndex })
        }
      }
    }
    visit(this.roots)
    this.entries = entries
    this.leafStarts = leafStarts
    this.changed()
  }

  private disposeRows(groups: GroupNode[]) {
    for (const group of groups) {
      group.rows?.dispose()
      group.rows = undefined
      if (group.children) this.disposeRows(group.children)
    }
  }

  private readExpanded() {
    const stored = window.localStorage.getItem(this.storageKey)
    if (stored === null) return new Set<string>()
    const values: unknown = JSON.parse(stored)
    if (
      !Array.isArray(values) ||
      values.some((value) => typeof value !== 'string')
    )
      throw new Error('Stored grouping state is invalid')
    return new Set(values)
  }

  private persistExpanded() {
    window.localStorage.setItem(
      this.storageKey,
      JSON.stringify([...this.expanded]),
    )
  }
}
