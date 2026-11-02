<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  isNavigationFailure,
  NavigationFailureType,
  useRouter,
} from 'vue-router'
import { errorMessage, request, RequestError } from '../../api/client'
import type {
  MentionNotification,
  NotificationListing,
  TableSchema,
} from '../../api/types'
import { useAuthStore } from '../../stores/auth'
import { useNavStore } from '../../stores/nav'
import { useRealtimeStore } from '../../stores/realtime'

const { t, locale } = useI18n()
const emit = defineEmits<{ count: [value: number] }>()
const router = useRouter()
const auth = useAuthStore()
const nav = useNavStore()
const realtime = useRealtimeStore()
const opened = ref(false)
const loading = ref(false)
const busy = ref(false)
const items = ref<MentionNotification[]>([])
const unreadCount = ref(0)
const hasMore = ref(false)
const error = ref<unknown>(null)
const navigationError = ref('')
let generation = 0
let loadController: AbortController | undefined
let navigationController: AbortController | undefined
let disposed = false

function current(token: number, userId: string) {
  return !disposed && token === generation && auth.user?.id === userId
}

async function load(append = false) {
  const userId = auth.user?.id
  if (!userId || auth.user?.mustChangePassword || (append && loading.value))
    return
  loadController?.abort()
  const own = new AbortController()
  loadController = own
  const token = generation
  loading.value = true
  error.value = null
  try {
    const result = await request<NotificationListing>(
      `/notifications${append ? `?offset=${items.value.length}` : ''}`,
      'GET',
      undefined,
      AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
    )
    if (!current(token, userId) || own.signal.aborted) return
    if (append) {
      const known = new Set(items.value.map((item) => item.id))
      items.value = [
        ...items.value,
        ...result.items.filter((item) => !known.has(item.id)),
      ]
    } else items.value = result.items
    unreadCount.value = result.unreadCount
    hasMore.value = result.hasMore
  } catch (cause) {
    if (!current(token, userId) || own.signal.aborted) return
    error.value = cause
    if (cause instanceof RequestError && cause.code === 'ERR_UNAUTHENTICATED') {
      auth.setUser(null)
      void router.replace('/login')
    } else if (
      cause instanceof RequestError &&
      cause.code === 'ERR_PASSWORD_CHANGE_REQUIRED'
    ) {
      void router.replace('/change-password')
    }
  } finally {
    if (current(token, userId) && loadController === own) loading.value = false
  }
}

async function openNotification(notification: MentionNotification) {
  const userId = auth.user?.id
  if (!userId || busy.value) return
  const own = new AbortController()
  navigationController = own
  const token = generation
  busy.value = true
  error.value = null
  navigationError.value = ''
  try {
    const schema = await request<TableSchema>(
      `/tables/${encodeURIComponent(notification.tableId)}/schema`,
      'GET',
      undefined,
      AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
    )
    if (!current(token, userId) || own.signal.aborted) return
    const view = schema.views.find((candidate) => candidate.type === 'grid')
    if (!view) {
      navigationError.value = t('notifications.noGrid')
      return
    }
    const failure = await router.push({
      path: `/t/${encodeURIComponent(notification.tableId)}/v/${encodeURIComponent(view.id)}/r/${encodeURIComponent(notification.rowId)}`,
      query: { comment: notification.commentId },
    })
    if (!current(token, userId) || own.signal.aborted) return
    if (
      failure &&
      !isNavigationFailure(failure, NavigationFailureType.duplicated)
    ) {
      navigationError.value = t('notifications.navigationError')
      return
    }
    await request(
      '/notifications/read',
      'POST',
      { id: notification.id },
      AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
    )
    if (!current(token, userId) || own.signal.aborted) return
    opened.value = false
    await load()
  } catch (cause) {
    if (!current(token, userId) || own.signal.aborted) return
    error.value = cause
    if (cause instanceof RequestError && [403, 404].includes(cause.status))
      void nav.refresh(true)
  } finally {
    if (current(token, userId) && navigationController === own)
      busy.value = false
  }
}

function timestamp(value: string) {
  return new Date(value).toLocaleString(locale.value, {
    timeZone: auth.user?.effectiveTimezone,
  })
}

const removeListener = realtime.onGlobal((event) => {
  if (['notification', 'notification.changed', 'nav.changed'].includes(event.t))
    void load()
})
watch(
  () => auth.user?.id,
  (id) => {
    generation++
    loadController?.abort()
    navigationController?.abort()
    items.value = []
    unreadCount.value = 0
    hasMore.value = opened.value = loading.value = busy.value = false
    error.value = null
    navigationError.value = ''
    if (id) void load()
  },
  { immediate: true, flush: 'sync' },
)
watch(
  () => realtime.status,
  (status) => {
    if (status === 'connected') void load()
  },
)
watch(unreadCount, (count) => emit('count', count), { immediate: true })
onBeforeUnmount(() => {
  disposed = true
  generation++
  loadController?.abort()
  navigationController?.abort()
  removeListener()
})
</script>

<template>
  <div class="notification-bell">
    <button
      type="button"
      :aria-label="
        error
          ? t('notifications.unavailable')
          : t('notifications.button', { count: unreadCount })
      "
      :aria-expanded="opened"
      aria-controls="notification-list"
      @click="opened = !opened"
    >
      <svg
        aria-hidden="true"
        width="18"
        height="18"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
      >
        <path d="M6 9a6 6 0 0 1 12 0v5l2 3H4l2-3z" />
        <path d="M10 20h4" />
      </svg>
      <span aria-hidden="true">{{ t('notifications.title') }}</span>
      <span v-if="unreadCount" class="unread-count">{{ unreadCount }}</span>
    </button>
    <span v-if="error && !opened" role="alert">{{
      t('notifications.unavailable')
    }}</span>
    <section
      v-if="opened"
      id="notification-list"
      class="notification-list"
      :aria-label="t('notifications.title')"
    >
      <h2>{{ t('notifications.title') }}</h2>
      <p v-if="loading" role="status">{{ t('notifications.loading') }}</p>
      <div v-if="error" role="alert">
        {{ errorMessage(error) }}
        <span v-if="items.length">{{ t('notifications.stale') }}</span>
        <button type="button" @click="load()">{{ t('auth.retry') }}</button>
      </div>
      <p v-if="navigationError" role="alert">{{ navigationError }}</p>
      <p v-if="!loading && !error && !items.length">
        {{ t('notifications.empty') }}
      </p>
      <ul>
        <li v-for="notification in items" :key="notification.id">
          <button
            type="button"
            :disabled="busy"
            @click="openNotification(notification)"
          >
            <span>
              {{
                t('notifications.mention', {
                  name:
                    notification.authorName || t('notifications.unknownAuthor'),
                  table: notification.tableName,
                })
              }}
            </span>
            <span v-if="!notification.readAt">{{
              t('notifications.unread')
            }}</span>
            <time :datetime="notification.createdAt">{{
              timestamp(notification.createdAt)
            }}</time>
          </button>
        </li>
      </ul>
      <button
        v-if="hasMore"
        type="button"
        :disabled="loading"
        @click="load(true)"
      >
        {{ t('notifications.loadMore') }}
      </button>
    </section>
  </div>
</template>

<style scoped>
.notification-bell > button {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
}
.unread-count {
  margin-left: auto;
  min-width: 20px;
  padding: 0 6px;
  border-radius: 10px;
  background: var(--accent);
  color: var(--surface);
  font-size: 12px;
  text-align: center;
}
.notification-list {
  max-height: min(50dvh, 400px);
  overflow: auto;
  margin: 4px 0;
  padding: 8px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface);
  color: var(--text);
}
.notification-list h2 {
  margin: 0 0 8px;
  font-size: 1rem;
}
.notification-list ul {
  display: grid;
  gap: 8px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.notification-list li button {
  display: grid;
  gap: 4px;
  width: 100%;
  text-align: left;
}
</style>
