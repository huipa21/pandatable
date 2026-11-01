import { ref } from 'vue'
import { request } from '../api/client'
import type { DirectoryListing } from '../api/types'

const labels = new Map<string, string>()
const deleted = new Set<string>()
const loaded = new Set<string>()
const failures = new Set<string>()
const inflight = new Set<string>()
const queued = new Map<string, Set<string>>()
const revision = ref(0)
let activeIdentity: string | null = null
let generation = 0

const cacheKey = (identity: string, tableId: string, id: string) =>
  `${identity}:${tableId}:${id}`
const scopeKey = (identity: string, tableId: string) => `${identity}:${tableId}`

function resetForIdentity(identity: string) {
  if (activeIdentity === identity) return
  activeIdentity = identity
  invalidateUserLabels()
}

export function invalidateUserLabels() {
  generation++
  labels.clear()
  deleted.clear()
  loaded.clear()
  failures.clear()
  inflight.clear()
  queued.clear()
  revision.value++
}

async function loadBatch(identity: string, tableId: string, ids: string[]) {
  const current = generation
  try {
    const result = await request<{
      items: { id: string; name: string; deleted?: boolean }[]
    }>(
      `/tables/${encodeURIComponent(tableId)}/directory/user-labels`,
      'POST',
      { ids },
      AbortSignal.timeout(10_000),
    )
    if (identity !== activeIdentity || current !== generation) return
    for (const id of ids) {
      const key = cacheKey(identity, tableId, id)
      inflight.delete(key)
      loaded.add(key)
      failures.delete(key)
      labels.delete(key)
      deleted.delete(key)
    }
    for (const item of result.items) {
      const key = cacheKey(identity, tableId, item.id)
      if (item.deleted) deleted.add(key)
      else labels.set(key, item.name)
    }
    while (labels.size > 2000) {
      const oldest = labels.keys().next().value
      if (oldest === undefined) break
      labels.delete(oldest)
      loaded.delete(oldest)
      failures.delete(oldest)
      deleted.delete(oldest)
    }
    revision.value++
  } catch {
    if (identity !== activeIdentity || current !== generation) return
    for (const id of ids) {
      const key = cacheKey(identity, tableId, id)
      inflight.delete(key)
      loaded.add(key)
      failures.add(key)
    }
    revision.value++
  }
}

function enqueue(identity: string, tableId: string, ids: string[]) {
  resetForIdentity(identity)
  const scope = scopeKey(identity, tableId)
  let pending = queued.get(scope)
  if (!pending) {
    pending = new Set()
    queued.set(scope, pending)
  }
  for (const id of ids) {
    const key = cacheKey(identity, tableId, id)
    if (id && !loaded.has(key) && !inflight.has(key)) pending.add(id)
  }
  while (loaded.size > 2000) {
    const oldest = loaded.values().next().value
    if (oldest === undefined) break
    loaded.delete(oldest)
    failures.delete(oldest)
    labels.delete(oldest)
    deleted.delete(oldest)
  }
  if (!pending.size) return
  queueMicrotask(() => {
    const values = queued.get(scope)
    if (!values || identity !== activeIdentity) return
    queued.delete(scope)
    const batch = [...values]
    for (let index = 0; index < batch.length; index += 100) {
      const ids = batch.slice(index, index + 100)
      for (const id of ids) inflight.add(cacheKey(identity, tableId, id))
      void loadBatch(identity, tableId, ids)
    }
  })
}

export function queueUserLabels(
  identity: string | undefined,
  tableId: string | undefined,
  ids: string[],
) {
  if (!identity || !tableId || !ids.length) return
  enqueue(identity, tableId, ids)
}

export function userLabel(
  identity: string | undefined,
  tableId: string | undefined,
  id: string,
) {
  void revision.value
  if (!identity || !tableId) return null
  resetForIdentity(identity)
  enqueue(identity, tableId, [id])
  return labels.get(cacheKey(identity, tableId, id)) ?? null
}

export function userLabelDeleted(
  identity: string | undefined,
  tableId: string | undefined,
  id: string,
) {
  void revision.value
  if (!identity || !tableId) return false
  return deleted.has(cacheKey(identity, tableId, id))
}

export function userLabelFailed(
  identity: string | undefined,
  tableId: string | undefined,
  id: string,
) {
  void revision.value
  if (!identity || !tableId) return false
  return failures.has(cacheKey(identity, tableId, id))
}

export async function searchTableUsers(
  tableId: string,
  query: string,
  signal: AbortSignal,
) {
  return request<DirectoryListing>(
    `/tables/${encodeURIComponent(tableId)}/directory/users?${new URLSearchParams(
      {
        q: query,
      },
    )}`,
    'GET',
    undefined,
    signal,
  )
}
