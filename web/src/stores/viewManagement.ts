import { onScopeDispose, ref, shallowRef, watch } from 'vue'
import { defineStore } from 'pinia'
import { request, RequestError } from '../api/client'
import type {
  AccessLevel,
  TableSchema,
  TableView,
  ViewColumns,
  ViewMutation,
} from '../api/types'
import { useAuthStore } from './auth'
import { useNavStore } from './nav'

export type ViewScope = 'personal' | 'shared'
export type ViewType = 'grid' | 'form' | 'kanban' | 'gallery'

export function canManageView(view: TableView, level: AccessLevel | undefined) {
  if (!level || level === 'none') return false
  if (view.scope === 'personal') return true
  return level === 'owner' || (level === 'write' && !view.locked)
}

interface ViewTarget {
  tableId: string
  viewId: string | null
}

interface ViewEditor extends ViewTarget {
  operation: 'create' | 'rename' | 'delete'
}

interface Context extends ViewTarget {
  identity: string
  isCurrentRoute: () => boolean
}

export const useViewManagementStore = defineStore('viewManagement', () => {
  const auth = useAuthStore()
  const nav = useNavStore()
  const target = ref<ViewTarget | null>(null)
  const editor = ref<ViewEditor | null>(null)
  const schema = shallowRef<TableSchema | null>(null)
  const loading = ref(false)
  const busy = ref(false)
  const error = ref<unknown>(null)
  const loadError = ref<unknown>(null)
  let context: Context | undefined
  let controller: AbortController | undefined

  function reset() {
    context = undefined
    controller?.abort()
    controller = undefined
    target.value = null
    editor.value = null
    schema.value = null
    loading.value = busy.value = false
    error.value = loadError.value = null
  }

  watch(() => auth.user?.id, reset, { flush: 'sync' })
  onScopeDispose(reset)

  function current(own: Context, signal?: AbortSignal) {
    return (
      context === own &&
      !signal?.aborted &&
      auth.user?.id === own.identity &&
      own.isCurrentRoute()
    )
  }

  function noteAccessError(cause: unknown) {
    if (!(cause instanceof RequestError)) return
    if (
      cause.code === 'ERR_UNAUTHENTICATED' ||
      cause.code === 'ERR_PASSWORD_CHANGE_REQUIRED'
    )
      nav.mutationError = cause
    else if ([403, 404].includes(cause.status)) void nav.refresh(true)
  }

  async function load(own: Context) {
    if (!current(own)) return false
    controller?.abort()
    const requestController = new AbortController()
    controller = requestController
    loading.value = true
    loadError.value = null
    try {
      const result = await request<TableSchema>(
        `/tables/${encodeURIComponent(own.tableId)}/schema`,
        'GET',
        undefined,
        AbortSignal.any([
          requestController.signal,
          AbortSignal.timeout(10_000),
        ]),
      )
      if (!current(own, requestController.signal)) return false
      if (result.table.id !== own.tableId)
        throw new RequestError('ERR_INTERNAL', 500)
      if (own.viewId && !result.views.some((view) => view.id === own.viewId))
        throw new RequestError('ERR_NOT_FOUND', 404)
      schema.value = result
      return true
    } catch (cause) {
      if (current(own, requestController.signal)) {
        schema.value = null
        loadError.value = cause
        noteAccessError(cause)
      }
      return false
    } finally {
      if (current(own)) loading.value = false
    }
  }

  function select(
    tableId: string,
    viewId: string | null,
    isCurrentRoute: () => boolean,
  ) {
    reset()
    const identity = auth.user?.id
    if (!identity) {
      loadError.value = new RequestError('ERR_UNAUTHENTICATED', 401)
      noteAccessError(loadError.value)
      return Promise.resolve(false)
    }
    const own: Context = { tableId, viewId, identity, isCurrentRoute }
    context = own
    target.value = { tableId, viewId }
    return load(own)
  }

  function beginCreate(tableId: string, isCurrentRoute: () => boolean) {
    const pending = select(tableId, null, isCurrentRoute)
    if (context) editor.value = { operation: 'create', tableId, viewId: null }
    return pending
  }

  function beginEdit(operation: 'rename' | 'delete') {
    if (!target.value?.viewId || !schema.value) {
      error.value = new RequestError('ERR_NOT_FOUND', 404)
      return
    }
    editor.value = { ...target.value, operation }
    error.value = null
  }

  function dismissEditor() {
    editor.value = null
    error.value = null
  }

  function retry() {
    return context ? load(context) : Promise.resolve(false)
  }

  function replaceView(updated: TableView) {
    if (!schema.value) return
    const views = schema.value.views.filter((view) => view.id !== updated.id)
    views.push(updated)
    views.sort(
      (left, right) =>
        left.position - right.position || left.id.localeCompare(right.id),
    )
    schema.value = { ...schema.value, views }
  }

  function selectedView() {
    return schema.value?.views.find((view) => view.id === target.value?.viewId)
  }

  async function write<T>(
    path: string,
    method: string,
    body: unknown,
    version: number | undefined,
    apply: (result: T) => void,
  ): Promise<T | null> {
    const own = context
    if (!own || !schema.value) {
      error.value = new RequestError('ERR_NOT_FOUND', 404)
      return null
    }
    if (busy.value || loading.value || !current(own)) return null
    controller?.abort()
    const requestController = new AbortController()
    controller = requestController
    busy.value = true
    error.value = null
    try {
      const result = await request<T>(
        path,
        method,
        body,
        AbortSignal.any([
          requestController.signal,
          AbortSignal.timeout(10_000),
        ]),
        { ifMatch: version },
      )
      if (!current(own, requestController.signal)) return null
      apply(result)
      await nav.refresh(true)
      return current(own, requestController.signal) ? result : null
    } catch (cause) {
      if (current(own, requestController.signal)) {
        error.value = cause
        noteAccessError(cause)
        if (
          !(cause instanceof RequestError) ||
          !['ERR_UNAUTHENTICATED', 'ERR_PASSWORD_CHANGE_REQUIRED'].includes(
            cause.code,
          )
        )
          await Promise.all([load(own), nav.refresh(true)])
      }
      return null
    } finally {
      if (current(own)) busy.value = false
    }
  }

  function create(name: string, type: ViewType, scope: ViewScope) {
    if (!target.value) {
      error.value = new RequestError('ERR_NOT_FOUND', 404)
      return Promise.resolve(null)
    }
    return write<ViewMutation>(
      `/tables/${encodeURIComponent(target.value.tableId)}/views`,
      'POST',
      { name, type, scope },
      undefined,
      (result) => replaceView(result.view),
    )
  }

  function rename(name: string) {
    const view = selectedView()
    if (!view) {
      error.value = new RequestError('ERR_NOT_FOUND', 404)
      return Promise.resolve(null)
    }
    return write<ViewMutation>(
      `/views/${encodeURIComponent(view.id)}`,
      'PATCH',
      { name },
      view.version,
      (result) => replaceView(result.view),
    )
  }

  function duplicate(name: string, scope: ViewScope, config?: ViewColumns) {
    const view = selectedView()
    if (!view) {
      error.value = new RequestError('ERR_NOT_FOUND', 404)
      return Promise.resolve(null)
    }
    return write<ViewMutation>(
      `/views/${encodeURIComponent(view.id)}/duplicate`,
      'POST',
      { name, scope, ...(config ? { config } : {}) },
      undefined,
      (result) => replaceView(result.view),
    )
  }

  function toggleLock() {
    const view = selectedView()
    if (!view) {
      error.value = new RequestError('ERR_NOT_FOUND', 404)
      return Promise.resolve(null)
    }
    return write<ViewMutation>(
      `/views/${encodeURIComponent(view.id)}`,
      'PATCH',
      { locked: !view.locked },
      view.version,
      (result) => replaceView(result.view),
    )
  }

  function move(beforeId: string | null) {
    const view = selectedView()
    if (!view) {
      error.value = new RequestError('ERR_NOT_FOUND', 404)
      return Promise.resolve(null)
    }
    return write<ViewMutation>(
      `/views/${encodeURIComponent(view.id)}/move`,
      'POST',
      { beforeId },
      view.version,
      (result) => replaceView(result.view),
    )
  }

  async function remove() {
    const view = selectedView()
    if (!view) {
      error.value = new RequestError('ERR_NOT_FOUND', 404)
      return false
    }
    const result = await write<unknown>(
      `/views/${encodeURIComponent(view.id)}`,
      'DELETE',
      undefined,
      view.version,
      () => {
        if (schema.value)
          schema.value = {
            ...schema.value,
            views: schema.value.views.filter((item) => item.id !== view.id),
          }
      },
    )
    return result !== null
  }

  return {
    target,
    editor,
    schema,
    loading,
    busy,
    error,
    loadError,
    reset,
    select,
    beginCreate,
    beginEdit,
    dismissEditor,
    retry,
    create,
    rename,
    duplicate,
    toggleLock,
    move,
    remove,
  }
})
