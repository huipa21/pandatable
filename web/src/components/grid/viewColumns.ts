import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { errorMessage, request, RequestError } from '../../api/client'
import type {
  FilterNode,
  HighlightRule,
  TableSchema,
  TableView,
  ViewColumns,
  ViewMutation,
} from '../../api/types'

export const minColumnWidth = 60
export const maxColumnWidth = 1000
export const columnSaveDelay = 350
const filterOperatorsWithoutValue = new Set([
  'is_empty',
  'is_not_empty',
  'is_checked',
  'is_not_checked',
])

function hasEmptyFilterValue(value: unknown): boolean {
  if (value === null || value === undefined) return true
  if (typeof value === 'string') return value.length === 0
  if (Array.isArray(value)) return value.some(hasEmptyFilterValue)
  if (typeof value === 'object')
    return Object.values(value).some(hasEmptyFilterValue)
  return false
}

export function isCompleteFilter(node: FilterNode | null): boolean {
  if (!node) return false
  if ('items' in node)
    return node.items.length > 0 && node.items.every(isCompleteFilter)
  if (filterOperatorsWithoutValue.has(node.cmp)) return true
  return !hasEmptyFilterValue(node.value)
}

export type ColumnIntent =
  | { kind: 'width'; id: string; width: number }
  | { kind: 'visible'; id: string; visible: boolean }
  | { kind: 'order'; ids: string[] }
  | { kind: 'frozen'; count: number }

export function normalizeColumns(
  config: ViewColumns,
  schema: TableSchema,
): ViewColumns {
  const live = new Map(schema.fields.map((field) => [field.id, field]))
  const seen = new Set<string>()
  const fields: ViewColumns['fields'] = []
  for (const column of config.fields) {
    if (!live.has(column.id) || seen.has(column.id)) continue
    fields.push({ ...column })
    seen.add(column.id)
  }
  for (const field of [...schema.fields].sort(
    (a, b) => a.position - b.position,
  )) {
    if (!seen.has(field.id))
      fields.push({ id: field.id, visible: !field.isSystem })
  }
  const sortable = new Set(
    schema.fields
      .filter((field) => field.uiType !== 'link')
      .map((field) => field.id),
  )
  const seenSort = new Set<string>()
  const sort = (config.sort ?? []).filter((item) => {
    if (
      !sortable.has(item.field) ||
      seenSort.has(item.field) ||
      !['asc', 'desc'].includes(item.dir)
    )
      return false
    seenSort.add(item.field)
    return true
  })
  return {
    fields,
    frozen: Math.max(
      0,
      Math.min(config.frozen, fields.filter((field) => field.visible).length),
    ),
    ...(config.filter !== undefined ? { filter: config.filter } : {}),
    ...(config.rowHeight !== undefined ? { rowHeight: config.rowHeight } : {}),
    ...(config.sort ? { sort } : {}),
    ...(config.groupBy ? { groupBy: [...config.groupBy] } : {}),
    ...(config.aggregates
      ? { aggregates: config.aggregates.map((item) => ({ ...item })) }
      : {}),
    ...(config.highlight
      ? {
          highlight: config.highlight.map((rule) => ({
            ...rule,
            target:
              rule.target.type === 'cells'
                ? { type: 'cells' as const, fields: [...rule.target.fields] }
                : { type: 'row' as const },
          })),
        }
      : {}),
  }
}

export function applyColumnIntents(
  config: ViewColumns,
  intents: ColumnIntent[],
  schema: TableSchema,
): ViewColumns {
  const result = normalizeColumns(config, schema)
  for (const intent of intents) {
    if (intent.kind === 'order') {
      const byId = new Map(result.fields.map((field) => [field.id, field]))
      const ordered: ViewColumns['fields'] = []
      for (const id of intent.ids) {
        const field = byId.get(id)
        if (field) {
          ordered.push(field)
          byId.delete(id)
        }
      }
      result.fields = [...ordered, ...byId.values()]
    } else if (intent.kind === 'frozen') {
      result.frozen = intent.count
    } else {
      const field = result.fields.find((field) => field.id === intent.id)
      if (!field) continue
      if (intent.kind === 'width')
        field.width = Math.round(
          Math.max(minColumnWidth, Math.min(intent.width, maxColumnWidth)),
        )
      else field.visible = intent.visible
    }
    result.frozen = Math.max(
      0,
      Math.min(
        result.frozen,
        result.fields.filter((field) => field.visible).length,
      ),
    )
  }
  return result
}

export function useViewColumns(options: {
  schema: () => TableSchema
  viewId: () => string
  identity: () => string
  canWrite: () => boolean
  failure: (error: unknown) => void
  updated: (view: TableView) => void
  message: (error: unknown) => string
}) {
  const view = () =>
    options.schema().views.find((view) => view.id === options.viewId())
  let liveSchema = options.schema()
  let authoritative = view()
  const config = shallowRef(
    normalizeColumns(
      authoritative?.config ?? { fields: [], frozen: 0 },
      options.schema(),
    ),
  )
  const saving = ref(false)
  const failure = ref('')
  const retryable = ref(false)
  const localQuery = ref(false)
  const canPersist = computed(() => options.canWrite() && !!view())
  let intents: ColumnIntent[] = []
  let pendingHighlights: HighlightRule[] | undefined
  let pendingAggregates: ViewColumns['aggregates'] | undefined
  let timer: ReturnType<typeof setTimeout> | undefined
  let controller: AbortController | undefined
  let generation = 0
  let disposed = false
  let conflictRetried = false
  let reconciling = false
  const valid = (own: number) =>
    !disposed && own === generation && canPersist.value

  function cancel() {
    generation++
    clearTimeout(timer)
    controller?.abort()
    controller = undefined
    saving.value = false
  }

  function schedule() {
    clearTimeout(timer)
    if (canPersist.value && !saving.value && !retryable.value)
      timer = setTimeout(() => void save(), columnSaveDelay)
  }

  function change(intent: ColumnIntent) {
    if (disposed || !options.identity()) return
    if (
      (intent.kind === 'width' && !Number.isFinite(intent.width)) ||
      (intent.kind === 'frozen' && !Number.isInteger(intent.count))
    ) {
      failure.value = options.message(new RequestError('ERR_VALIDATION', 400))
      return
    }
    localQuery.value = true
    if (!canPersist.value) {
      config.value = applyColumnIntents(config.value, [intent], liveSchema)
      return
    }
    intents.push(intent)
    if (!retryable.value && !reconciling)
      config.value = applyColumnIntents(config.value, [intent], liveSchema)
    schedule()
  }

  function changeHighlights(rules: HighlightRule[]) {
    if (disposed || !options.identity()) return
    const copy = rules.map((rule) => ({
      ...rule,
      target:
        rule.target.type === 'cells'
          ? { type: 'cells' as const, fields: [...rule.target.fields] }
          : { type: 'row' as const },
    }))
    localQuery.value = true
    if (!canPersist.value) {
      config.value = { ...config.value, highlight: copy }
      return
    }
    pendingHighlights = copy
    config.value = { ...config.value, highlight: copy }
    if (!retryable.value && !reconciling) schedule()
  }

  function changeAggregates(
    aggregates: NonNullable<ViewColumns['aggregates']>,
  ) {
    if (disposed || !options.identity()) return
    const copy = aggregates.map((item) => ({ ...item }))
    localQuery.value = true
    if (!canPersist.value) {
      config.value = { ...config.value, aggregates: copy }
      return
    }
    pendingAggregates = copy
    config.value = { ...config.value, aggregates: copy }
    if (!retryable.value && !reconciling) schedule()
  }

  async function save() {
    if (
      !canPersist.value ||
      (!intents.length &&
        pendingHighlights === undefined &&
        pendingAggregates === undefined) ||
      saving.value ||
      !authoritative
    )
      return
    clearTimeout(timer)
    const own = generation
    const batch = [...intents]
    const batchHighlights = pendingHighlights
    const batchAggregates = pendingAggregates
    const target = applyColumnIntents(authoritative.config, batch, liveSchema)
    if (batchHighlights !== undefined)
      target.highlight = batchHighlights.map((rule) => ({
        ...rule,
        target:
          rule.target.type === 'cells'
            ? { type: 'cells', fields: [...rule.target.fields] }
            : { type: 'row' },
      }))
    if (batchAggregates !== undefined)
      target.aggregates = batchAggregates.map((item) => ({ ...item }))
    saving.value = true
    retryable.value = false
    controller = new AbortController()
    const signal = AbortSignal.any([
      controller.signal,
      AbortSignal.timeout(10_000),
    ])
    try {
      const body = { config: target }
      if (new TextEncoder().encode(JSON.stringify(body)).byteLength > 4096)
        throw new RequestError('ERR_VALIDATION', 400)
      const result = await request<ViewMutation>(
        `/views/${options.viewId()}`,
        'PATCH',
        body,
        signal,
        { ifMatch: authoritative.version },
      )
      if (!valid(own)) return
      authoritative = result.view
      intents.splice(0, batch.length)
      if (pendingHighlights === batchHighlights) pendingHighlights = undefined
      if (pendingAggregates === batchAggregates) pendingAggregates = undefined
      config.value = applyColumnIntents(result.view.config, intents, liveSchema)
      if (pendingHighlights !== undefined)
        config.value = { ...config.value, highlight: pendingHighlights }
      if (pendingAggregates !== undefined)
        config.value = { ...config.value, aggregates: pendingAggregates }
      if (pendingAggregates !== undefined)
        config.value = { ...config.value, aggregates: pendingAggregates }
      conflictRetried = false
      reconciling = false
      options.updated(result.view)
    } catch (error) {
      if (!valid(own)) return
      failure.value = options.message(error)
      reconciling = true
      config.value = normalizeColumns(authoritative.config, options.schema())
      if (pendingHighlights !== undefined)
        config.value = { ...config.value, highlight: pendingHighlights }
      if (pendingAggregates !== undefined)
        config.value = { ...config.value, aggregates: pendingAggregates }
      try {
        controller = new AbortController()
        const latest = await request<TableSchema>(
          `/tables/${options.schema().table.id}/schema`,
          'GET',
          undefined,
          AbortSignal.any([controller.signal, AbortSignal.timeout(10_000)]),
        )
        if (!valid(own)) return
        const next = latest.views.find((view) => view.id === options.viewId())
        if (!next) throw new RequestError('ERR_NOT_FOUND', 404)
        liveSchema = latest
        authoritative = next
        config.value = normalizeColumns(next.config, latest)
        if (pendingHighlights !== undefined)
          config.value = { ...config.value, highlight: pendingHighlights }
        if (pendingAggregates !== undefined)
          config.value = { ...config.value, aggregates: pendingAggregates }
        if (!(
          next.scope === 'personal' ||
          latest.myLevel === 'owner' ||
          (latest.myLevel === 'write' && !next.locked)
        )) {
          intents = []
          options.failure(new RequestError('ERR_PERMISSION_DENIED', 403))
        } else if (
          error instanceof RequestError &&
          error.code === 'ERR_VERSION_CONFLICT' &&
          !conflictRetried
        ) {
          conflictRetried = true
        } else retryable.value = true
        options.updated(next)
      } catch (reloadError) {
        if (!valid(own)) return
        failure.value = `${options.message(error)} ${errorMessage(reloadError)}`
        retryable.value = true
        options.failure(reloadError)
      }
      options.failure(error)
    } finally {
      if (valid(own)) {
        saving.value = false
        if (
          (intents.length ||
            pendingHighlights !== undefined ||
            pendingAggregates !== undefined) &&
          !retryable.value
        )
          schedule()
      }
    }
  }

  watch(
    () => [options.viewId(), options.identity(), canPersist.value],
    () => {
      cancel()
      authoritative = view()
      liveSchema = options.schema()
      intents = []
      pendingHighlights = undefined
      pendingAggregates = undefined
      retryable.value = false
      failure.value = ''
      reconciling = false
      config.value = normalizeColumns(
        authoritative?.config ?? { fields: [], frozen: 0 },
        options.schema(),
      )
    },
  )
  watch(
    () => view()?.version,
    () => {
      const next = view()
      if (
        !next ||
        !authoritative ||
        next.version <= authoritative.version ||
        saving.value
      )
        return
      authoritative = next
      liveSchema = options.schema()
      config.value = applyColumnIntents(next.config, intents, liveSchema)
      if (pendingHighlights !== undefined)
        config.value = { ...config.value, highlight: pendingHighlights }
    },
  )
  onBeforeUnmount(() => {
    disposed = true
    cancel()
  })
  return {
    config,
    saving,
    failure,
    retryable,
    canPersist,
    localQuery,
    change,
    changeHighlights,
    changeAggregates,
    retry: () => {
      retryable.value = false
      void save()
    },
  }
}
