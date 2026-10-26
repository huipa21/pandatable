import { computed, onScopeDispose, ref, watch } from 'vue'
import { defineStore } from 'pinia'
import { request, RequestError } from '../api/client'
import type {
  DirectoryListing,
  GrantLevel,
  GrantListing,
  PrincipalType,
  ResourceKind,
} from '../api/types'
import { useAuthStore } from './auth'
import { useNavStore } from './nav'

export interface ShareTarget {
  kind: ResourceKind
  id: string
  name: string
}

export interface ShareChange {
  principalType: PrincipalType
  principalId: string
  name: string
  level: GrantLevel
  removeId?: string
  inheritedLevel: GrantLevel | null
}

export function validShareSearch(value: string) {
  const search = value.trim()
  const length = [...search].length
  return (
    length >= 2 &&
    length <= 100 &&
    new TextEncoder().encode(search).length <= 254 &&
    !/\p{Cc}/u.test(search)
  )
}

export const useSharingStore = defineStore('sharing', () => {
  const auth = useAuthStore()
  const nav = useNavStore()
  const target = ref<ShareTarget | null>(null)
  const listing = ref<GrantListing | null>(null)
  const loading = ref(false)
  const error = ref<unknown>(null)
  const busy = ref(false)
  const writeError = ref<unknown>(null)
  const review = ref<ShareChange | null>(null)
  const query = ref('')
  const principalType = ref<PrincipalType>('user')
  const results = ref<DirectoryListing | null>(null)
  const searching = ref(false)
  const searchError = ref<unknown>(null)
  let generation = 0
  let listGeneration = 0
  let searchGeneration = 0
  let listController: AbortController | undefined
  let searchController: AbortController | undefined
  let writeController: AbortController | undefined

  const editable = computed(
    () =>
      listing.value?.effectiveLevel === 'owner' &&
      !loading.value &&
      !error.value,
  )
  const signal = (own: AbortController) =>
    AbortSignal.any([own.signal, AbortSignal.timeout(10_000)])

  function clearSearch() {
    searchGeneration++
    searchController?.abort()
    results.value = null
    searching.value = false
    searchError.value = null
  }
  watch([query, principalType], clearSearch, { flush: 'sync' })

  function close(force = false) {
    if (busy.value && !force) return
    generation++
    listGeneration++
    listController?.abort()
    writeController?.abort()
    clearSearch()
    target.value = listing.value = review.value = null
    error.value = writeError.value = null
    loading.value = busy.value = false
    query.value = ''
    principalType.value = 'user'
  }
  watch(
    () => auth.user?.id,
    () => close(true),
    { flush: 'sync' },
  )
  onScopeDispose(() => close(true))

  async function handleError(cause: unknown) {
    if (!(cause instanceof RequestError)) return
    if (cause.code === 'ERR_UNAUTHENTICATED') auth.setUser(null)
    else if (cause.code === 'ERR_PASSWORD_CHANGE_REQUIRED' && auth.user)
      auth.setUser({ ...auth.user, mustChangePassword: true })
    else if ([403, 404].includes(cause.status)) await nav.refresh(true)
  }

  async function load() {
    const scope = target.value
    if (!scope || busy.value) return
    listController?.abort()
    const own = new AbortController()
    listController = own
    const current = ++listGeneration
    const session = generation
    loading.value = true
    error.value = null
    try {
      const result = await request<GrantListing>(
        `/grants?resource=${scope.kind}:${scope.id}`,
        'GET',
        undefined,
        signal(own),
      )
      if (session !== generation || current !== listGeneration) return
      listing.value = result
    } catch (cause) {
      if (
        session !== generation ||
        current !== listGeneration ||
        own.signal.aborted
      )
        return
      listing.value = review.value = null
      clearSearch()
      error.value = cause
      await handleError(cause)
    } finally {
      if (session === generation && current === listGeneration)
        loading.value = false
    }
  }

  function open(scope: ShareTarget) {
    close(true)
    target.value = scope
    void load()
  }

  async function search() {
    if (!target.value || !editable.value || busy.value) return
    clearSearch()
    if (!validShareSearch(query.value)) {
      searchError.value = new RequestError('ERR_VALIDATION', 400)
      return
    }
    const own = new AbortController()
    searchController = own
    const current = searchGeneration
    const session = generation
    const scope = target.value
    searching.value = true
    try {
      const result = await request<DirectoryListing>(
        `/directory/${principalType.value}s?${new URLSearchParams({
          resource: `${scope.kind}:${scope.id}`,
          q: query.value.trim(),
        })}`,
        'GET',
        undefined,
        signal(own),
      )
      if (session === generation && current === searchGeneration)
        results.value = result
    } catch (cause) {
      if (
        session !== generation ||
        current !== searchGeneration ||
        own.signal.aborted
      )
        return
      searchError.value = cause
      await handleError(cause)
    } finally {
      if (session === generation && current === searchGeneration)
        searching.value = false
    }
  }

  async function commit() {
    const scope = target.value
    const change = review.value
    if (!scope || !change || busy.value || !editable.value) return
    const own = new AbortController()
    writeController = own
    const session = generation
    const userId = auth.user?.id
    busy.value = true
    writeError.value = null
    let succeeded = false
    try {
      await request(
        change.removeId ? `/grants/${change.removeId}` : '/grants',
        change.removeId ? 'DELETE' : 'PUT',
        change.removeId
          ? undefined
          : {
              resourceType: scope.kind,
              resourceId: scope.id,
              principalType: change.principalType,
              principalId: change.principalId,
              level: change.level,
            },
        signal(own),
      )
      succeeded = true
    } catch (cause) {
      if (session === generation && !own.signal.aborted) {
        writeError.value = cause
        await handleError(cause)
      }
    } finally {
      // A timed-out/aborted write may have committed. Reconcile, never retry it automatically.
      if (auth.user?.id === userId) await nav.refresh(true)
      if (session === generation) {
        busy.value = false
        if (succeeded) {
          review.value = null
          clearSearch()
          query.value = ''
        }
        await load()
      }
    }
  }

  watch(
    () => nav.revision,
    () => {
      if (!target.value || busy.value) return
      const node = nav.flatItems.find(
        (item) =>
          item.id === target.value?.id && item.kind === target.value?.kind,
      )
      if (node?.level === 'owner' && !node.pathOnly) {
        if (!loading.value) void load()
      } else {
        listGeneration++
        listController?.abort()
        loading.value = false
        listing.value = review.value = null
        clearSearch()
        error.value = new RequestError(
          node ? 'ERR_PERMISSION_DENIED' : 'ERR_NOT_FOUND',
          node ? 403 : 404,
        )
      }
    },
  )

  return {
    target,
    listing,
    loading,
    error,
    busy,
    writeError,
    review,
    query,
    principalType,
    results,
    searching,
    searchError,
    editable,
    open,
    close,
    load,
    search,
    commit,
  }
})
