import { computed, onScopeDispose, ref, watch } from 'vue'
import { defineStore } from 'pinia'
import { request, RequestError } from '../api/client'
import type {
  AccessLevel,
  AuthMutation,
  Navigation,
  NavView,
  ResourceKind,
  TableSchema,
  WorkspacePreference,
  WorkspacePreferenceMutation,
} from '../api/types'
import { useAuthStore } from './auth'

export interface NavigationItem {
  id: string
  name: string
  kind: ResourceKind
  path: string
  parentId: string | null
  level: AccessLevel
  pathOnly: boolean
  canEdit: boolean
  childKind: 'book' | 'table' | null
  canCreate: boolean
  index: number
  siblingCount: number
  views: NavView[]
  children: NavigationItem[]
}

export interface StructureAction {
  operation: 'create' | 'rename' | 'delete'
  kind: ResourceKind
  id?: string
  parentId?: string
  name?: string
}

export function tablePath(id: string, views: NavView[]) {
  const view = views.find((item) => item.type === 'grid') ?? views[0]
  return view ? `/t/${id}/v/${view.id}` : `/t/${id}`
}

export const useNavStore = defineStore('nav', () => {
  const auth = useAuthStore()
  const tree = ref<Navigation>({ workspaces: [] })
  const loaded = ref(false)
  const loading = ref(false)
  const busy = ref(false)
  const error = ref<unknown>(null)
  const mutationError = ref<unknown>(null)
  const editor = ref<StructureAction | null>(null)
  const revision = ref(0)
  // The last visited workspace is distinct from the server-side preference.
  const lastWorkspaceId = ref<string | null>(null)
  const preferredWorkspaceId = ref<string | null>(null)
  const preferenceLoaded = ref(false)
  const preferenceLoading = ref(false)
  const preferenceBusy = ref(false)
  const preferenceError = ref<unknown>(null)
  const drawerOpen = ref(false)
  let generation = 0
  let controller: AbortController | undefined
  let mutationController: AbortController | undefined
  let pending: Promise<boolean> | undefined
  let preferenceGeneration = 0
  let preferenceReadController: AbortController | undefined
  let preferenceWriteController: AbortController | undefined
  let preferencePending: Promise<boolean> | undefined
  let preferenceWrite: Promise<string> | undefined

  function reset() {
    generation++
    preferenceGeneration++
    controller?.abort()
    mutationController?.abort()
    preferenceReadController?.abort()
    preferenceWriteController?.abort()
    pending = undefined
    preferencePending = undefined
    preferenceWrite = undefined
    tree.value = { workspaces: [] }
    loaded.value = loading.value = busy.value = false
    error.value = mutationError.value = editor.value = null
    lastWorkspaceId.value = null
    preferredWorkspaceId.value = null
    preferenceLoaded.value =
      preferenceLoading.value =
      preferenceBusy.value =
        false
    preferenceError.value = null
    drawerOpen.value = false
    revision.value++
  }
  watch(() => auth.user?.id, reset, { flush: 'sync' })
  onScopeDispose(reset)

  function refresh(force = false): Promise<boolean> {
    if (pending && !force) return pending
    controller?.abort()
    const own = new AbortController()
    controller = own
    const current = ++generation
    loading.value = true
    error.value = null
    pending = (async () => {
      try {
        const result = await request<Navigation>(
          '/nav',
          'GET',
          undefined,
          AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
        )
        if (generation !== current) return false
        tree.value = result
        loaded.value = true
        revision.value++
        return true
      } catch (cause) {
        if (!own.signal.aborted && generation === current) error.value = cause
        return false
      } finally {
        if (generation === current) {
          loading.value = false
          pending = undefined
        }
      }
    })()
    return pending
  }

  function refreshPreference(force = false): Promise<boolean> {
    if (preferenceWrite)
      return preferenceWrite.then(
        () => true,
        () => false,
      )
    if (preferencePending && !force) return preferencePending
    preferenceReadController?.abort()
    const own = new AbortController()
    preferenceReadController = own
    const current = ++preferenceGeneration
    preferenceLoading.value = true
    preferenceError.value = null
    preferencePending = (async () => {
      try {
        const result = await request<WorkspacePreference>(
          '/auth/workspace-preference',
          'GET',
          undefined,
          AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
        )
        if (own.signal.aborted || preferenceGeneration !== current) return false
        preferredWorkspaceId.value = result.workspaceId
        preferenceLoaded.value = true
        return true
      } catch (cause) {
        if (!own.signal.aborted && preferenceGeneration === current)
          preferenceError.value = cause
        return false
      } finally {
        if (preferenceGeneration === current) {
          preferenceLoading.value = false
          preferencePending = undefined
        }
      }
    })()
    return preferencePending
  }

  async function writePreference(
    path: string,
    method: 'POST' | 'PUT',
    body: unknown,
    signal?: AbortSignal,
  ) {
    const userId = auth.user?.id
    while (preferenceWrite) {
      try {
        await preferenceWrite
      } catch (cause) {
        // A cancelled earlier navigation must not cancel the next resolver.
        if (
          !(cause instanceof DOMException) ||
          cause.name !== 'AbortError' ||
          signal?.aborted ||
          auth.user?.id !== userId
        )
          throw cause
      }
    }
    if (!userId || auth.user?.id !== userId || signal?.aborted)
      throw new DOMException('Aborted', 'AbortError')
    preferenceReadController?.abort()
    preferencePending = undefined
    preferenceLoading.value = false
    const own = new AbortController()
    preferenceWriteController = own
    const current = ++preferenceGeneration
    preferenceBusy.value = true
    preferenceError.value = null
    const signals = [own.signal, AbortSignal.timeout(10_000)]
    if (signal) signals.push(signal)
    const operation = (async () => {
      try {
        const result = await request<WorkspacePreferenceMutation>(
          path,
          method,
          body,
          AbortSignal.any(signals),
        )
        if (
          own.signal.aborted ||
          signal?.aborted ||
          preferenceGeneration !== current ||
          auth.user?.id !== userId
        )
          throw new DOMException('Aborted', 'AbortError')
        preferredWorkspaceId.value = result.workspaceId
        preferenceLoaded.value = true
        return result.workspaceId
      } catch (cause) {
        if (
          !own.signal.aborted &&
          !signal?.aborted &&
          preferenceGeneration === current &&
          auth.user?.id === userId
        ) {
          preferenceError.value = cause
          if (
            cause instanceof RequestError &&
            [403, 404].includes(cause.status)
          )
            void refresh(true)
        }
        throw cause
      }
    })()
    preferenceWrite = operation
    try {
      return await operation
    } finally {
      if (preferenceWrite === operation) {
        preferenceWrite = undefined
        preferenceBusy.value = false
        if (signal?.aborted && !own.signal.aborted && auth.user?.id === userId)
          void refreshPreference(true)
      }
    }
  }

  function resolvePreference(signal?: AbortSignal) {
    return writePreference(
      '/auth/workspace-preference/resolve',
      'POST',
      {},
      signal,
    )
  }

  function setPreferredWorkspace(workspaceId: string) {
    return writePreference('/auth/workspace-preference', 'PUT', { workspaceId })
  }

  async function mutate<T>(path: string, method: string, body?: unknown) {
    busy.value = true
    mutationError.value = null
    const own = new AbortController()
    mutationController = own
    const userId = auth.user?.id
    try {
      const result = await request<T>(
        path,
        method,
        body,
        AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
      )
      if (own.signal.aborted || auth.user?.id !== userId)
        throw new DOMException('Aborted', 'AbortError')
      if (!own.signal.aborted && auth.user?.id === userId) await refresh(true)
      return result
    } catch (cause) {
      if (!own.signal.aborted && auth.user?.id === userId) {
        mutationError.value = cause
        if (cause instanceof RequestError && [403, 404].includes(cause.status))
          await refresh(true)
      }
      throw cause
    } finally {
      if (mutationController === own) busy.value = false
    }
  }

  async function submit(action: StructureAction, name: string) {
    const plural = `${action.kind}s`
    if (action.operation === 'create') {
      if (action.kind === 'table') {
        const result = await mutate<TableSchema & AuthMutation>(
          `/books/${action.parentId}/tables`,
          'POST',
          { name },
        )
        return tablePath(result.table.id, result.views)
      }
      if (action.kind === 'book') {
        const result = await mutate<{ book: { id: string } } & AuthMutation>(
          `/workspaces/${action.parentId}/books`,
          'POST',
          { name },
        )
        return `/w/${action.parentId}/b/${result.book.id}`
      }
      const result = await mutate<{ workspace: { id: string } } & AuthMutation>(
        '/workspaces',
        'POST',
        { name },
      )
      return `/w/${result.workspace.id}`
    }
    await mutate(
      `/${plural}/${action.id}`,
      action.operation === 'delete' ? 'DELETE' : 'PATCH',
      action.operation === 'rename' ? { name } : undefined,
    )
    return null
  }

  async function reorder(item: NavigationItem, index: number) {
    await mutate(`/${item.kind}s/${item.id}`, 'PATCH', { index })
  }

  const items = computed<NavigationItem[]>(() =>
    tree.value.workspaces.map((w, wi, workspaces) => ({
      id: w.id,
      name: w.name,
      kind: 'workspace',
      path: `/w/${w.id}`,
      parentId: null,
      level: w.level,
      pathOnly: w.pathOnly,
      canEdit: !w.pathOnly && w.level === 'owner',
      canCreate: !w.pathOnly && w.level === 'owner',
      childKind: 'book',
      index: wi,
      siblingCount: workspaces.length,
      views: [],
      children: w.books.map((b, bi, books) => ({
        id: b.id,
        name: b.name,
        kind: 'book',
        path: `/w/${w.id}/b/${b.id}`,
        parentId: w.id,
        level: b.level,
        pathOnly: b.pathOnly,
        canEdit: !b.pathOnly && w.level === 'owner',
        canCreate: !b.pathOnly && b.level === 'owner',
        childKind: 'table',
        index: bi,
        siblingCount: books.length,
        views: [],
        children: b.tables.map((table, ti, tables) => ({
          id: table.id,
          name: table.name,
          kind: 'table',
          // The bare table route shows the default view (breadcrumb without a view).
          path: `/t/${table.id}`,
          parentId: b.id,
          level: table.level,
          pathOnly: false,
          canEdit: b.level === 'owner',
          canCreate: false,
          childKind: null,
          index: ti,
          siblingCount: tables.length,
          views: table.views,
          children: [],
        })),
      })),
    })),
  )
  const flatItems = computed(() =>
    items.value.flatMap((w) => [
      w,
      ...w.children.flatMap((b) => [b, ...b.children]),
    ]),
  )

  return {
    tree,
    items,
    flatItems,
    loaded,
    loading,
    busy,
    error,
    mutationError,
    editor,
    revision,
    lastWorkspaceId,
    preferredWorkspaceId,
    preferenceLoaded,
    preferenceLoading,
    preferenceBusy,
    preferenceError,
    drawerOpen,
    reset,
    refresh,
    refreshPreference,
    resolvePreference,
    setPreferredWorkspace,
    submit,
    reorder,
  }
})
