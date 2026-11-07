import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useNavLocation } from './useNavLocation'
import { useAuthStore } from '../stores/auth'
import { useNavStore } from '../stores/nav'

function param(value: unknown) {
  return typeof value === 'string' ? value : null
}

export function useRecentOpen() {
  const route = useRoute()
  const auth = useAuthStore()
  const nav = useNavStore()
  const { book } = useNavLocation()
  const lastIdentity = ref<string | null>(null)

  const location = computed(() => {
    const workspaceParam =
      param(route.params.workspaceId) ?? param(route.params.wid)
    const bookParam = param(route.params.bookId)
    const tableParam = param(route.params.tableId)
    if (!workspaceParam && !bookParam && !tableParam) return null

    const workspaceId = workspaceParam ?? book.value?.parentId ?? null
    const bookId = bookParam ?? (tableParam ? (book.value?.id ?? null) : null)
    if (tableParam && (!workspaceId || !bookId)) return null

    return {
      identity: JSON.stringify([
        auth.user?.id ?? null,
        route.path,
        workspaceParam,
        bookParam,
        tableParam,
        param(route.params.viewId),
        param(route.params.rowId),
      ]),
      workspaceId,
      bookId,
    }
  })

  watch(
    location,
    (resolved) => {
      const userId = auth.user?.id
      if (!userId || !resolved) {
        lastIdentity.value = null
        return
      }
      const identity = `${userId}:${resolved.identity}`
      if (identity === lastIdentity.value) return
      lastIdentity.value = identity
      if (resolved.workspaceId) nav.recordOpen('workspace', resolved.workspaceId)
      if (resolved.bookId) nav.recordOpen('book', resolved.bookId)
    },
    { immediate: true, flush: 'sync' },
  )
}
