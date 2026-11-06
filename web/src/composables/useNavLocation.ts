import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useNavStore } from '../stores/nav'

function param(value: unknown) {
  return typeof value === 'string' ? value : undefined
}

/** Resolves the route's workspace/book/table/view against the nav tree. */
export function useNavLocation() {
  const route = useRoute()
  const nav = useNavStore()
  const table = computed(() => {
    const id = param(route.params.tableId)
    return id
      ? nav.flatItems.find((item) => item.kind === 'table' && item.id === id)
      : undefined
  })
  const book = computed(() => {
    if (table.value)
      return nav.flatItems.find(
        (item) => item.kind === 'book' && item.id === table.value!.parentId,
      )
    const id = param(route.params.bookId)
    return id
      ? nav.flatItems.find(
          (item) =>
            item.kind === 'book' &&
            item.id === id &&
            item.parentId === route.params.wid,
        )
      : undefined
  })
  const routeWorkspaceId = computed(
    () =>
      book.value?.parentId ??
      param(route.params.workspaceId) ??
      param(route.params.wid) ??
      null,
  )
  const workspace = computed(() => {
    const resourceRoute =
      typeof route.params.tableId === 'string' ||
      typeof route.params.bookId === 'string' ||
      typeof route.params.workspaceId === 'string' ||
      typeof route.params.wid === 'string'
    const id =
      routeWorkspaceId.value ?? (resourceRoute ? null : nav.lastWorkspaceId)
    return id ? nav.items.find((item) => item.id === id) : undefined
  })
  const view = computed(() => {
    const id = param(route.params.viewId)
    return id ? table.value?.views.find((item) => item.id === id) : undefined
  })
  const activeBook = computed(() => {
    if (!nav.loaded || nav.loading || nav.error) return undefined
    if (param(route.params.bookId)) return book.value
    if (!table.value) return undefined
    if (param(route.params.viewId) && !view.value) return undefined
    return book.value
  })
  return { workspace, book, table, view, routeWorkspaceId, activeBook }
}
