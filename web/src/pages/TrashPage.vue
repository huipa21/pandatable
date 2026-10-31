<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { errorMessage, request, RequestError } from '../api/client'
import type { TrashItem, TrashListing } from '../api/types'
import { trashKindLabel } from '../i18n'
import { useAuthStore } from '../stores/auth'
import { useNavStore } from '../stores/nav'

const { t, locale } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const nav = useNavStore()
const items = ref<TrashItem[]>([])
const loading = ref(false)
const busy = ref(false)
const loaded = ref(false)
const listError = ref<unknown>(null)
const mutationError = ref<unknown>(null)
const optional = ref<TrashItem | null>(null)
const optionalCount = ref(0)
const optionalReason = ref<'empty' | 'unknown'>('empty')
const restoredName = ref('')
const refreshFailed = ref(false)
let generation = 0
let identity = auth.user?.id
let controller: AbortController | undefined
let mutationController: AbortController | undefined

async function redirectSession(cause: unknown): Promise<boolean> {
  if (!(cause instanceof RequestError)) return false
  if (cause.code === 'ERR_UNAUTHENTICATED') {
    auth.setUser(null)
    await router.replace('/login')
    return true
  }
  if (cause.code === 'ERR_PASSWORD_CHANGE_REQUIRED') {
    await router.replace('/change-password')
    return true
  }
  return false
}

function reset() {
  generation++
  controller?.abort()
  mutationController?.abort()
  items.value = []
  loading.value = busy.value = loaded.value = false
  listError.value = mutationError.value = optional.value = null
  restoredName.value = ''
  refreshFailed.value = false
}

async function load(): Promise<boolean> {
  controller?.abort()
  const own = new AbortController()
  controller = own
  const current = ++generation
  loading.value = true
  listError.value = null
  try {
    const listing = await request<TrashListing>(
      '/trash',
      'GET',
      undefined,
      AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
    )
    if (current !== generation || own.signal.aborted) return false
    items.value = listing.items
    if (
      optional.value &&
      !listing.items.some(
        (item) =>
          item.id === optional.value?.id && item.kind === optional.value.kind,
      )
    )
      optional.value = null
    loaded.value = true
    return true
  } catch (cause) {
    if (current !== generation || own.signal.aborted) return false
    items.value = []
    loaded.value = false
    if (await redirectSession(cause)) return false
    listError.value = cause
    return false
  } finally {
    if (current === generation) loading.value = false
  }
}

async function refresh() {
  optional.value = null
  mutationError.value = null
  restoredName.value = ''
  refreshFailed.value = false
  await load()
}

async function restore(item: TrashItem, allowOptional = false) {
  if (busy.value || !auth.user) return
  const userId = auth.user.id
  const own = new AbortController()
  mutationController = own
  busy.value = true
  mutationError.value = null
  optional.value = null
  restoredName.value = ''
  refreshFailed.value = false
  try {
    await request(
      `/trash/${encodeURIComponent(item.kind)}/${encodeURIComponent(item.id)}/restore`,
      'POST',
      allowOptional ? { allowOptional: true } : {},
      AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
    )
    if (own.signal.aborted || auth.user?.id !== userId) return
    const [trashLoaded, navLoaded] = await Promise.all([
      load(),
      nav.refresh(true),
    ])
    if (own.signal.aborted || auth.user?.id !== userId) return
    if (trashLoaded && navLoaded) restoredName.value = item.name
    else refreshFailed.value = true
  } catch (cause) {
    if (own.signal.aborted || auth.user?.id !== userId) return
    if (await redirectSession(cause)) return
    if (
      cause instanceof RequestError &&
      ['ERR_REQUIRED_HAS_EMPTY', 'ERR_TRASH_REQUIRED_UNKNOWN'].includes(
        cause.code,
      ) &&
      item.kind === 'field' &&
      !allowOptional
    ) {
      optional.value = item
      optionalCount.value = cause.details.count ?? 0
      optionalReason.value =
        cause.code === 'ERR_TRASH_REQUIRED_UNKNOWN' ? 'unknown' : 'empty'
    } else {
      mutationError.value = cause
      await Promise.all([load(), nav.refresh(true)])
    }
  } finally {
    if (mutationController === own) busy.value = false
  }
}

function timestamp(value: string) {
  return new Date(value).toLocaleString(locale.value, {
    timeZone: auth.user?.effectiveTimezone,
  })
}

watch(
  () => nav.revision,
  () => {
    if (identity && auth.user?.id === identity && !busy.value) void load()
  },
  { flush: 'sync' },
)
watch(
  () => auth.user?.id,
  (id) => {
    reset()
    identity = id
    if (id) void load()
  },
  { immediate: true, flush: 'sync' },
)
onBeforeUnmount(() => {
  reset()
})
</script>

<template>
  <section class="trash-page">
    <h1>{{ t('trash.title') }}</h1>
    <p>{{ t('trash.notice') }}</p>
    <button type="button" :disabled="loading || busy" @click="refresh">
      {{ t('trash.refresh') }}
    </button>
    <p v-if="loading" role="status">{{ t('trash.loading') }}</p>
    <div v-if="listError" role="alert">
      <p>{{ errorMessage(listError) }}</p>
      <button type="button" :disabled="loading || busy" @click="refresh">
        {{ t('auth.retry') }}
      </button>
    </div>
    <p v-if="mutationError" role="alert">{{ errorMessage(mutationError) }}</p>
    <div v-if="optional" role="alert">
      <p v-if="optionalReason === 'unknown'">
        {{ t('trash.requiredUnknownNotice') }}
      </p>
      <p v-else>{{ t('trash.requiredNotice', { count: optionalCount }) }}</p>
      <button
        type="button"
        :aria-label="t('trash.restoreOptionalItem', { name: optional.name })"
        :disabled="busy || loading"
        @click="restore(optional, true)"
      >
        {{ t('trash.restoreOptional') }}
      </button>
    </div>
    <p v-if="refreshFailed" role="alert">{{ t('trash.refreshFailed') }}</p>
    <p v-if="restoredName" role="status">
      {{ t('trash.restored', { name: restoredName }) }}
    </p>
    <p v-if="loaded && !loading && items.length === 0">
      {{ t('trash.empty') }}
    </p>
    <ul v-if="loaded && items.length" class="trash-list">
      <li
        v-for="item in items"
        :key="`${item.kind}:${item.id}`"
        :data-trash-id="item.id"
      >
        <div>
          <strong>{{ item.name }}</strong>
          <span class="trash-kind">{{ trashKindLabel(item.kind) }}</span>
          <p v-if="item.kind !== 'workspace'">
            {{ t('trash.workspace', { name: item.workspaceName }) }}
          </p>
          <p v-if="item.parentId !== item.workspaceId">
            {{ t('trash.parent', { name: item.parentName }) }}
          </p>
          <p>
            <time :datetime="item.deletedAt">
              {{ t('trash.deleted', { date: timestamp(item.deletedAt) }) }}
            </time>
          </p>
          <p>
            <time :datetime="item.expiresAt">
              {{ t('trash.expires', { date: timestamp(item.expiresAt) }) }}
            </time>
          </p>
        </div>
        <button
          type="button"
          :aria-label="
            t('trash.restoreItem', {
              kind: trashKindLabel(item.kind),
              name: item.name,
            })
          "
          :disabled="busy || loading"
          @click="restore(item)"
        >
          {{ t('trash.restore') }}
        </button>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.trash-list {
  display: grid;
  gap: 12px;
  list-style: none;
  padding: 0;
}

.trash-list li {
  display: flex;
  flex-wrap: wrap;
  align-items: start;
  justify-content: space-between;
  gap: 12px;
  padding: 12px;
  border: 1px solid var(--border);
  border-radius: 6px;
  overflow-wrap: anywhere;
}

.trash-list p {
  margin: 4px 0;
}

.trash-kind {
  margin-inline-start: 8px;
  color: var(--muted);
}

.trash-page button {
  min-height: 44px;
  font: inherit;
  color: var(--text);
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 6px 12px;
  cursor: pointer;
}
</style>
