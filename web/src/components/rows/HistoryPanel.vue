<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { errorMessage, request, RequestError } from '../../api/client'
import type {
  HistoryListing,
  RowHistory,
  RowValue,
  TableField,
} from '../../api/types'
import { useAuthStore } from '../../stores/auth'
import { useNavStore } from '../../stores/nav'
import { useRealtimeStore } from '../../stores/realtime'

const props = defineProps<{
  tableId: string
  rowId: string
  identity: string
  fields: TableField[]
}>()
const emit = defineEmits<{ schemaChanged: [] }>()
const { t, locale } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const nav = useNavStore()
const realtime = useRealtimeStore()
const items = ref<RowHistory[]>([])
const hasMore = ref(false)
const loading = ref(false)
const error = ref<unknown>(null)
const blocked = ref(false)
let offset = 0
let generation = 0
let controller: AbortController | undefined
let disposed = false

const groups = computed(() => {
  const result: {
    key: string
    author: string
    createdAt: string
    entries: RowHistory[]
  }[] = []
  const byKey = new Map<string, (typeof result)[number]>()
  for (const item of items.value) {
    const minute = Math.floor(new Date(item.createdAt).getTime() / 60_000)
    const key = `${minute}:${item.authorId ?? 'deleted'}`
    let group = byKey.get(key)
    if (!group) {
      group = {
        key,
        author: item.authorName || t('history.deletedUser'),
        createdAt: item.createdAt,
        entries: [],
      }
      byKey.set(key, group)
      result.push(group)
    }
    group.entries.push(item)
  }
  return result
})

function current(token: number) {
  return !disposed && token === generation && props.identity === auth.user?.id
}

function handleFailure(cause: unknown) {
  error.value = cause
  if (!(cause instanceof RequestError)) return
  if (cause.code === 'ERR_PASSWORD_CHANGE_REQUIRED') {
    blocked.value = true
    items.value = []
    void router.replace('/change-password')
  } else if ([401, 403, 404].includes(cause.status)) {
    blocked.value = true
    items.value = []
    if (cause.status === 401) {
      auth.setUser(null)
      void router.replace('/login')
    } else {
      void nav.refresh(true)
      emit('schemaChanged')
    }
  }
}

async function load(append = false) {
  if (props.identity !== auth.user?.id || (append && loading.value)) return
  controller?.abort()
  const own = new AbortController()
  controller = own
  const token = generation
  const suffix = append ? `?offset=${offset}` : ''
  loading.value = true
  error.value = null
  try {
    const result = await request<HistoryListing>(
      `/tables/${encodeURIComponent(props.tableId)}/rows/${encodeURIComponent(props.rowId)}/history${suffix}`,
      'GET',
      undefined,
      AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
    )
    if (!current(token) || own.signal.aborted) return
    if (append) {
      const known = new Set(items.value.map((item) => item.id))
      items.value = [
        ...items.value,
        ...result.items.filter((item) => !known.has(item.id)),
      ]
      offset += result.items.length
    } else {
      items.value = result.items
      offset = result.items.length
    }
    hasMore.value = result.hasMore
    blocked.value = false
  } catch (cause) {
    if (current(token) && !own.signal.aborted) handleFailure(cause)
  } finally {
    if (current(token) && controller === own) loading.value = false
  }
}

function actionLabel(action: RowHistory['action']) {
  switch (action) {
    case 'insert':
      return t('history.actions.insert')
    case 'update':
      return t('history.actions.update')
    case 'delete':
      return t('history.actions.delete')
    case 'link':
      return t('history.actions.link')
    case 'unlink':
      return t('history.actions.unlink')
    case 'restore':
      return t('history.actions.restore')
  }
}

function fieldLabel(id: string) {
  return (
    props.fields.find((field) => field.id === id)?.name ??
    t('history.deletedField', { id })
  )
}

function formatValue(value: RowValue | undefined): string {
  if (value === undefined || value === null) return t('history.noValue')
  if (typeof value === 'string') return value || t('history.emptyText')
  if (typeof value === 'boolean')
    return value ? t('history.yes') : t('history.no')
  if (typeof value === 'number') return String(value)
  if (Array.isArray(value))
    return value.length
      ? value.map((item) => formatValue(item)).join(t('history.separator'))
      : t('history.emptyList')
  if (typeof value.id === 'string' && typeof value.label === 'string')
    return t('history.linkValue', { label: value.label, id: value.id })
  return JSON.stringify(value)
}

function timestamp(value: string) {
  return new Date(value).toLocaleString(locale.value, {
    timeZone: auth.user?.effectiveTimezone,
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  })
}

watch(
  [
    () => props.tableId,
    () => props.rowId,
    () => props.identity,
    () => auth.user?.id,
  ],
  ([tableId, rowId, identity, userId], _previous, onCleanup) => {
    generation++
    controller?.abort()
    items.value = []
    offset = 0
    hasMore.value = loading.value = blocked.value = false
    error.value = null
    if (!tableId || !rowId || !identity || identity !== userId) return
    void load()
    onCleanup(
      realtime.subscribe(tableId, (event) => {
        if (event.t === 'sub.denied') {
          generation++
          controller?.abort()
          handleFailure(new RequestError(event.code ?? 'ERR_NOT_FOUND', 404))
        } else if (
          event.t === 'resync' ||
          event.t === 'row.bulk' ||
          (['row.created', 'row.updated', 'row.deleted'].includes(event.t) &&
            event.ids?.includes(rowId))
        )
          void load()
      }),
    )
  },
  { immediate: true },
)
onBeforeUnmount(() => {
  disposed = true
  generation++
  controller?.abort()
})
</script>

<template>
  <section class="history" :aria-label="t('history.title')">
    <h3>{{ t('history.title') }}</h3>
    <p v-if="loading" role="status">{{ t('history.loading') }}</p>
    <div v-if="error" role="alert">
      {{ errorMessage(error) }}
      <span v-if="items.length">{{ t('history.stale') }}</span>
      <button v-if="!blocked" type="button" @click="load()">
        {{ t('auth.retry') }}
      </button>
    </div>
    <p v-if="!loading && !error && !items.length">{{ t('history.empty') }}</p>
    <section v-for="group in groups" :key="group.key" class="history-group">
      <div class="history-meta">
        <strong>{{ group.author }}</strong>
        <time :datetime="group.createdAt">{{
          timestamp(group.createdAt)
        }}</time>
      </div>
      <ol>
        <li
          v-for="item in group.entries"
          :key="item.id"
          :data-history-id="item.id"
        >
          <strong>{{ actionLabel(item.action) }}</strong>
          <dl>
            <template v-for="(change, id) in item.changes" :key="id">
              <dt>{{ fieldLabel(id) }}</dt>
              <dd>
                {{
                  t('history.transition', {
                    old: formatValue(change.o),
                    new: formatValue(change.n),
                  })
                }}
              </dd>
            </template>
          </dl>
        </li>
      </ol>
    </section>
    <button
      v-if="hasMore && !blocked"
      type="button"
      :disabled="loading"
      @click="load(true)"
    >
      {{ t('history.loadMore') }}
    </button>
  </section>
</template>

<style scoped>
.history {
  padding: 16px 0;
}
.history-group {
  padding: 12px 0;
  border-top: 1px solid var(--border);
}
.history-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 16px;
}
ol {
  padding-left: 24px;
}
li {
  margin: 12px 0;
}
dl {
  margin: 8px 0;
}
dt {
  font-weight: 600;
}
dd {
  margin: 0 0 8px;
  overflow-wrap: anywhere;
  white-space: pre-wrap;
}
button {
  min-height: 36px;
  padding: 6px 12px;
  color: var(--text);
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 4px;
  cursor: pointer;
}
@media (max-width: 767px) {
  button {
    min-height: 44px;
    font-size: 16px;
  }
}
</style>
