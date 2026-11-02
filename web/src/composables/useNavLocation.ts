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
    const id =
      routeWorkspaceId.value ??
      (route.path === '/home' ? null : nav.lastWorkspaceId)
    return id ? nav.items.find((item) => item.id === id) : undefined
  })
  const view = computed(() => {
    const id = param(route.params.viewId)
    return id ? table.value?.views.find((item) => item.id === id) : undefined
  })
  return { workspace, book, table, view, routeWorkspaceId }
}
