import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { errorMessage, request, RequestError } from '../../api/client'
import type {
  TableSchema,
  TableView,
  ViewColumns,
  ViewMutation,
} from '../../api/types'

export const minColumnWidth = 60
export const maxColumnWidth = 1000
export const columnSaveDelay = 350

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
  return {
    fields,
    frozen: Math.max(
      0,
      Math.min(config.frozen, fields.filter((field) => field.visible).length),
    ),
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
  const config = ref(
    normalizeColumns(
      authoritative?.config ?? { fields: [], frozen: 0 },
      options.schema(),
    ),
  )
  const saving = ref(false)
  const failure = ref('')
  const retryable = ref(false)
  const localQuery = ref(false)
  const canPersist = computed(
    () => options.canWrite() && !!view() && !view()!.locked,
  )
  let intents: ColumnIntent[] = []
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

  async function save() {
    if (!canPersist.value || !intents.length || saving.value || !authoritative)
      return
    clearTimeout(timer)
    const own = generation
    const batch = [...intents]
    const target = applyColumnIntents(authoritative.config, batch, liveSchema)
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
      config.value = applyColumnIntents(result.view.config, intents, liveSchema)
      conflictRetried = false
      reconciling = false
      options.updated(result.view)
    } catch (error) {
      if (!valid(own)) return
      failure.value = options.message(error)
      reconciling = true
      config.value = normalizeColumns(authoritative.config, options.schema())
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
        if (next.locked || !['write', 'owner'].includes(latest.myLevel)) {
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
        if (intents.length && !retryable.value) schedule()
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
    retry: () => {
      retryable.value = false
      void save()
    },
  }
}
