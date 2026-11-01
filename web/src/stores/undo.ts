import { computed, ref, shallowRef, watch } from 'vue'
import { defineStore } from 'pinia'
import { request, RequestError } from '../api/client'
import type {
  DataRow,
  HistoryListing,
  LinkMutation,
  RelationDelta,
  RowMutation,
  RowValue,
  TableSchema,
  TableView,
  ViewColumns,
  ViewMutation,
} from '../api/types'
import { i18n } from '../i18n'
import { useAuthStore } from './auth'
import { useRealtimeStore } from './realtime'

export interface RowValues {
  id: string
  fields: Record<string, RowValue>
}

interface GuardedRow extends RowValues {
  expected: Record<string, RowValue>
}

interface RestoreSnapshot {
  id: string
  fields?: Record<string, RowValue>
  restoreVersion?: string
  cascaded?: { tableId: string; rows: RestoreSnapshot[] }[]
}

export type UndoStep =
  | { kind: 'patch'; tableId: string; rows: GuardedRow[] }
  | { kind: 'delete'; tableId: string; ids: string[] }
  | { kind: 'restore'; tableId: string; rows: RestoreSnapshot[] }
  | {
      kind: 'link'
      tableId: string
      rowId: string
      fieldId: string
      add: string[]
      remove: string[]
    }
  | {
      kind: 'view'
      tableId: string
      viewId: string
      viewType: 'grid' | 'form' | 'kanban'
      config: ViewColumns
      expected: ViewColumns
    }

interface UndoAction {
  steps: UndoStep[]
}

const maxGuardedPatchBytes = 2 * 1024 * 1024

class UndoCancelled extends Error {}

function sameValue(left: unknown, right: unknown): boolean {
  if (Object.is(left, right)) return true
  if (Array.isArray(left) && Array.isArray(right))
    return (
      left.length === right.length &&
      left.every((item, index) => sameValue(item, right[index]))
    )
  if (
    typeof left !== 'object' ||
    left === null ||
    typeof right !== 'object' ||
    right === null ||
    Array.isArray(left) ||
    Array.isArray(right)
  )
    return false
  const first = left as Record<string, unknown>
  const second = right as Record<string, unknown>
  const keys = Object.keys(first)
  return (
    keys.length === Object.keys(second).length &&
    keys.every(
      (key) => Object.hasOwn(second, key) && sameValue(first[key], second[key]),
    )
  )
}

export function patchUndoStep(
  tableId: string,
  previous: RowValues[],
  result: DataRow[],
): UndoStep | null {
  const rowsById = new Map(result.map((row) => [row.id, row]))
  const rows: GuardedRow[] = []
  for (const before of previous) {
    const after = rowsById.get(before.id)
    if (!after) throw new Error('Mutation response is missing a row')
    const fields: Record<string, RowValue> = {}
    const expected: Record<string, RowValue> = {}
    for (const [fieldId, value] of Object.entries(before.fields)) {
      if (!Object.hasOwn(after.fields, fieldId))
        throw new Error('Mutation response is missing a field')
      const current = after.fields[fieldId]!
      if (!sameValue(value, current)) {
        fields[fieldId] = value
        expected[fieldId] = current
      }
    }
    if (Object.keys(fields).length)
      rows.push({ id: before.id, fields, expected })
  }
  return rows.length ? { kind: 'patch', tableId, rows } : null
}

export function linkUndoSteps(changes: RelationDelta[]): UndoStep[] {
  const steps: UndoStep[] = []
  for (const change of [...changes].reverse()) {
    const last = steps.at(-1)
    const step =
      last?.kind === 'link' &&
      last.tableId === change.tableId &&
      last.rowId === change.rowId &&
      last.fieldId === change.fieldId
        ? last
        : {
            kind: 'link' as const,
            tableId: change.tableId,
            rowId: change.rowId,
            fieldId: change.fieldId,
            add: [],
            remove: [],
          }
    if (step !== last) steps.push(step)
    if (change.action === 'link') step.remove.push(change.targetId)
    else if (change.action === 'unlink') step.add.push(change.targetId)
    else throw new Error('Mutation response has an invalid link change')
  }
  return steps
}

export function viewUndoStep(
  tableId: string,
  before: TableView,
  after: TableView,
): UndoStep | null {
  if (
    before.type !== 'grid' &&
    before.type !== 'form' &&
    before.type !== 'kanban'
  )
    throw new Error('Unsupported view type in undo history')
  const previous = viewConfigMember(before.type, before.config)
  const current = viewConfigMember(before.type, after.config)
  if (sameValue(previous, current)) return null
  return {
    kind: 'view',
    tableId,
    viewId: before.id,
    viewType: before.type,
    config: before.config,
    expected: after.config,
  }
}

function viewConfigMember(
  type: 'grid' | 'form' | 'kanban',
  config: ViewColumns,
) {
  if (type === 'form') return config.form
  if (type === 'kanban') return config.kanban
  return config
}

function pathForRows(tableId: string) {
  return `/tables/${encodeURIComponent(tableId)}/rows`
}

function compactSnapshot(row: RestoreSnapshot): RestoreSnapshot {
  if (!row.restoreVersion) return row
  return {
    id: row.id,
    restoreVersion: row.restoreVersion,
    ...(row.cascaded?.length
      ? {
          cascaded: row.cascaded.map((group) => ({
            tableId: group.tableId,
            rows: group.rows.map(compactSnapshot),
          })),
        }
      : {}),
  }
}

function boundedSteps(step: UndoStep): UndoStep[] {
  if (step.kind !== 'patch') return [step]
  const result: UndoStep[] = []
  let rows: GuardedRow[] = []
  let size = 0
  for (const row of step.rows) {
    let current: GuardedRow | undefined
    for (const [fieldId, value] of Object.entries(row.fields)) {
      const expected = row.expected[fieldId]
      if (expected === undefined)
        throw new Error('Undo patch is missing its expected value')
      const item: GuardedRow = {
        id: row.id,
        fields: { [fieldId]: value },
        expected: { [fieldId]: expected },
      }
      const bytes = new TextEncoder().encode(
        JSON.stringify({ rows: [item] }),
      ).length
      if (bytes > maxGuardedPatchBytes)
        throw new Error('Undo cell exceeds the guarded patch limit')
      if (size + bytes > maxGuardedPatchBytes && rows.length) {
        result.push({ ...step, rows })
        rows = []
        size = 0
        current = undefined
      }
      if (!current) {
        current = { id: row.id, fields: {}, expected: {} }
        rows.push(current)
      }
      current.fields[fieldId] = value
      current.expected[fieldId] = expected
      size += bytes
    }
  }
  if (rows.length) result.push({ ...step, rows })
  return result
}

async function mapConcurrent<T, R>(
  values: T[],
  run: (value: T) => Promise<R>,
): Promise<R[]> {
  const result = new Array<R>(values.length)
  let next = 0
  await Promise.all(
    Array.from({ length: Math.min(8, values.length) }, async () => {
      while (next < values.length) {
        const index = next++
        result[index] = await run(values[index]!)
      }
    }),
  )
  return result
}

export const useUndoStore = defineStore('undo', () => {
  const auth = useAuthStore()
  const realtime = useRealtimeStore()
  const undoStack = shallowRef<UndoAction[]>([])
  const redoStack = shallowRef<UndoAction[]>([])
  const busy = ref(false)
  const error = shallowRef<unknown>(null)
  const inconsistent = ref(false)
  const canUndo = computed(() => undoStack.value.length > 0 && !busy.value)
  const canRedo = computed(() => redoStack.value.length > 0 && !busy.value)
  let pending: UndoAction[] = []
  let generation = 0
  let controller: AbortController | undefined

  function reset() {
    generation++
    controller?.abort()
    controller = undefined
    undoStack.value = []
    redoStack.value = []
    pending = []
    busy.value = false
    error.value = null
    inconsistent.value = false
  }

  watch(() => auth.user?.id, reset, { flush: 'sync' })

  function push(action: UndoAction) {
    undoStack.value = [...undoStack.value, action].slice(-50)
    redoStack.value = []
  }

  function record(steps: (UndoStep | null)[]) {
    const valid = steps.filter((step): step is UndoStep => step !== null)
    if (!auth.user || !valid.length) return
    const bounded = valid.flatMap((step) =>
      boundedSteps(
        step.kind === 'restore'
          ? { ...step, rows: step.rows.map(compactSnapshot) }
          : step,
      ),
    )
    if (!bounded.length) return
    const action = {
      steps: structuredClone(bounded),
    }
    if (busy.value) pending.push(action)
    else push(action)
  }

  function signal() {
    if (!controller) throw new Error('Undo operation is not active')
    return AbortSignal.any([controller.signal, AbortSignal.timeout(10_000)])
  }

  async function conflictRows(step: Extract<UndoStep, { kind: 'patch' }>) {
    const current = await mapConcurrent(step.rows, (row) =>
      request<DataRow>(
        `${pathForRows(step.tableId)}/${encodeURIComponent(row.id)}`,
        'GET',
        undefined,
        signal(),
      ),
    )
    const changed = step.rows
      .map((row, index) => ({
        row,
        current: current[index]!,
        fields: Object.keys(row.expected).filter(
          (fieldId) =>
            !Object.hasOwn(current[index]!.fields, fieldId) ||
            !sameValue(row.expected[fieldId], current[index]!.fields[fieldId]),
        ),
      }))
      .filter((item) => item.fields.length)
    if (!changed.length) throw new RequestError('ERR_UNDO_CONFLICT', 409)
    const first = changed[0]!
    const history = await request<HistoryListing>(
      `${pathForRows(step.tableId)}/${encodeURIComponent(first.row.id)}/history`,
      'GET',
      undefined,
      signal(),
    )
    const author = history.items.find((item) =>
      first.fields.some((fieldId) => Object.hasOwn(item.changes, fieldId)),
    )?.authorName
    const name = author || i18n.global.t('undo.unknownEditor')
    if (!window.confirm(i18n.global.t('undo.conflict', { name })))
      throw new UndoCancelled()
    return step.rows.map((row, index) => {
      const expected: Record<string, RowValue> = {}
      for (const fieldId of Object.keys(row.fields)) {
        const value = current[index]!.fields[fieldId]
        if (value === undefined)
          throw new RequestError('ERR_SCHEMA_CHANGED', 409)
        expected[fieldId] = value
      }
      return { ...row, expected }
    })
  }

  async function applyStep(
    step: UndoStep,
    allowConfirmation: boolean,
  ): Promise<UndoStep[]> {
    if (step.kind === 'patch') {
      let rows = step.rows
      let result: RowMutation
      try {
        result = await request<RowMutation>(
          `${pathForRows(step.tableId)}:guarded`,
          'PATCH',
          { rows },
          signal(),
        )
      } catch (cause) {
        if (
          !allowConfirmation ||
          !(cause instanceof RequestError) ||
          cause.code !== 'ERR_UNDO_CONFLICT'
        )
          throw cause
        rows = await conflictRows(step)
        const chunks = boundedSteps({ ...step, rows })
        if (chunks.length > 1) {
          const inverse: UndoStep[] = []
          try {
            for (const chunk of chunks)
              inverse.unshift(...(await applyStep(chunk, false)))
          } catch (failure) {
            if (failure instanceof RequestError) {
              try {
                for (const previous of inverse) await applyStep(previous, false)
              } catch (compensationFailure) {
                throw new Error('Undo compensation failed', {
                  cause: compensationFailure,
                })
              }
            }
            throw failure
          }
          return inverse
        }
        result = await request<RowMutation>(
          `${pathForRows(step.tableId)}:guarded`,
          'PATCH',
          { rows },
          signal(),
        )
      }
      if (
        result.rows.length !== rows.length ||
        result.rows.some((row, index) => row.id !== rows[index]?.id)
      )
        throw new Error('Undo patch returned an unsupported response')
      const inverse = patchUndoStep(
        step.tableId,
        rows.map((row) => ({ id: row.id, fields: row.expected })),
        result.rows,
      )
      return inverse ? [inverse] : []
    }
    if (step.kind === 'delete') {
      const result = await request<RowMutation>(
        pathForRows(step.tableId),
        'DELETE',
        { ids: step.ids },
        signal(),
      )
      if (
        result.rows.length !== step.ids.length ||
        step.ids.some((id) => !result.rows.some((row) => row.id === id))
      )
        throw new Error('Undo delete returned an unsupported response')
      return [
        {
          kind: 'restore',
          tableId: step.tableId,
          rows: result.rows.map(compactSnapshot),
        },
      ]
    }
    if (step.kind === 'restore') {
      const result = await request<RowMutation>(
        `${pathForRows(step.tableId)}:restore`,
        'POST',
        { rows: step.rows },
        signal(),
      )
      if (
        result.rows.length !== step.rows.length ||
        result.rows.some((row, index) => row.id !== step.rows[index]?.id)
      )
        throw new Error('Undo restore returned an unsupported response')
      return [
        {
          kind: 'delete',
          tableId: step.tableId,
          ids: result.rows.map((row) => row.id),
        },
      ]
    }
    if (step.kind === 'link') {
      const result = await request<LinkMutation>(
        `${pathForRows(step.tableId)}/${encodeURIComponent(step.rowId)}/links/${encodeURIComponent(step.fieldId)}`,
        'POST',
        { add: step.add, remove: step.remove },
        signal(),
      )
      if (!Array.isArray(result.changes))
        throw new Error('Undo link returned an unsupported response')
      return linkUndoSteps(result.changes)
    }
    const schema = await request<TableSchema>(
      `/tables/${encodeURIComponent(step.tableId)}/schema`,
      'GET',
      undefined,
      signal(),
    )
    const view = schema.views.find((item) => item.id === step.viewId)
    if (!view) throw new RequestError('ERR_NOT_FOUND', 404)
    if (view.type !== step.viewType)
      throw new RequestError('ERR_VERSION_CONFLICT', 409)
    const current = viewConfigMember(step.viewType, view.config)
    const expected = viewConfigMember(step.viewType, step.expected)
    if (!sameValue(current, expected))
      throw new RequestError('ERR_VERSION_CONFLICT', 409)
    const config =
      step.viewType === 'form'
        ? { form: step.config.form }
        : step.viewType === 'kanban'
          ? { kanban: step.config.kanban }
          : step.config
    const result = await request<ViewMutation>(
      `/views/${encodeURIComponent(step.viewId)}`,
      'PATCH',
      { config },
      signal(),
      { ifMatch: view.version },
    )
    return [
      {
        ...step,
        config: view.config,
        expected: result.view.config,
      },
    ]
  }

  function invalidate(steps: UndoStep[]) {
    const rows = new Set<string>()
    const views = new Map<string, Set<string>>()
    for (const step of steps) {
      if (step.kind === 'view') {
        const ids = views.get(step.tableId) ?? new Set<string>()
        ids.add(step.viewId)
        views.set(step.tableId, ids)
      } else {
        rows.add(step.tableId)
        if (step.kind === 'restore')
          for (const row of step.rows)
            for (const group of row.cascaded ?? []) rows.add(group.tableId)
      }
    }
    for (const tableId of rows) realtime.invalidateLocal(tableId)
    for (const [tableId, ids] of views)
      for (const viewId of ids) realtime.invalidateLocal(tableId, viewId)
  }

  async function replay(direction: 'undo' | 'redo') {
    const source = direction === 'undo' ? undoStack : redoStack
    const target = direction === 'undo' ? redoStack : undoStack
    const action = source.value.at(-1)
    if (!action || busy.value || !auth.user) return
    const own = generation
    controller = new AbortController()
    busy.value = true
    error.value = null
    inconsistent.value = false
    const inverse: UndoStep[] = []
    try {
      for (const step of action.steps) {
        if (generation !== own) return
        inverse.unshift(...(await applyStep(step, true)))
      }
      if (generation !== own) return
      source.value = source.value.slice(0, -1)
      if (inverse.length)
        target.value = [...target.value, { steps: inverse }].slice(-50)
      invalidate([...action.steps, ...inverse])
    } catch (cause) {
      if (generation !== own) return
      let uncertain = !(
        cause instanceof RequestError || cause instanceof UndoCancelled
      )
      if (!uncertain) {
        try {
          for (const step of inverse) await applyStep(step, false)
        } catch {
          uncertain = true
        }
      }
      if (uncertain) {
        undoStack.value = []
        redoStack.value = []
        inconsistent.value = true
      } else if (!(cause instanceof UndoCancelled)) error.value = cause
      invalidate([...action.steps, ...inverse])
    } finally {
      if (generation === own) {
        controller = undefined
        busy.value = false
        for (const action of pending) push(action)
        pending = []
      }
    }
  }

  return {
    undoStack,
    redoStack,
    busy,
    error,
    inconsistent,
    canUndo,
    canRedo,
    record,
    undo: () => replay('undo'),
    redo: () => replay('redo'),
    reset,
  }
})
