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
  let generation = 0
  let controller: AbortController | undefined
  let mutationController: AbortController | undefined
  let pending: Promise<boolean> | undefined

  function reset() {
    generation++
    controller?.abort()
    mutationController?.abort()
    pending = undefined
    tree.value = { workspaces: [] }
    loaded.value = loading.value = busy.value = false
    error.value = mutationError.value = editor.value = null
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
        children: b.tables.map((table, ti, tables) => ({
          id: table.id,
          name: table.name,
          kind: 'table',
          path: tablePath(table.id, table.views),
          parentId: b.id,
          level: table.level,
          pathOnly: false,
          canEdit: b.level === 'owner',
          canCreate: false,
          childKind: null,
          index: ti,
          siblingCount: tables.length,
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
    reset,
    refresh,
    submit,
    reorder,
  }
})
