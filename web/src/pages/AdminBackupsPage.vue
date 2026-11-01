<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { NAlert, NButton } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { errorMessage, request, RequestError } from '../api/client'
import type { BackupListing, BackupRecord } from '../api/types'
import { useAuthStore } from '../stores/auth'

const { t, n, locale } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const items = ref<BackupRecord[]>([])
const freeBytes = ref<number | null>(null)
const loading = ref(false)
const busy = ref(false)
const loaded = ref(false)
const listError = ref<unknown>(null)
const mutationError = ref<unknown>(null)
const deleting = ref<BackupRecord | null>(null)
const running = computed(() =>
  items.value.some((item) => item.status === 'running'),
)
let generation = 0
let controller: AbortController | undefined
let mutationController: AbortController | undefined
let timer: ReturnType<typeof setTimeout> | undefined

function reset() {
  generation++
  controller?.abort()
  mutationController?.abort()
  clearTimeout(timer)
  items.value = []
  freeBytes.value = null
  loading.value = busy.value = loaded.value = false
  listError.value = mutationError.value = deleting.value = null
}

async function redirectSession(cause: unknown) {
  if (!(cause instanceof RequestError)) return false
  if (cause.code === 'ERR_UNAUTHENTICATED') {
    auth.setUser(null)
    await router.replace('/login')
  } else if (cause.code === 'ERR_PASSWORD_CHANGE_REQUIRED') {
    await router.replace('/change-password')
  } else if (cause.code === 'ERR_PERMISSION_DENIED') {
    await router.replace('/home')
  } else return false
  return true
}

async function load() {
  clearTimeout(timer)
  controller?.abort()
  const own = new AbortController()
  controller = own
  const current = ++generation
  loading.value = true
  listError.value = null
  try {
    const listing = await request<BackupListing>(
      '/admin/backups',
      'GET',
      undefined,
      AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
    )
    if (own.signal.aborted || current !== generation) return
    items.value = listing.items
    freeBytes.value = listing.freeBytes
    loaded.value = true
    if (
      deleting.value &&
      !items.value.some((item) => item.id === deleting.value?.id)
    )
      deleting.value = null
  } catch (cause) {
    if (own.signal.aborted || current !== generation) return
    if (!(await redirectSession(cause))) listError.value = cause
  } finally {
    if (current === generation) {
      loading.value = false
      if (running.value) timer = setTimeout(() => void load(), 1500)
    }
  }
}

async function mutate(
  action: (signal: AbortSignal) => Promise<unknown>,
  timeout = 10_000,
) {
  if (busy.value || !auth.user) return
  const identity = auth.user.id
  const own = new AbortController()
  mutationController = own
  busy.value = true
  mutationError.value = null
  try {
    await action(AbortSignal.any([own.signal, AbortSignal.timeout(timeout)]))
  } catch (cause) {
    if (
      !own.signal.aborted &&
      auth.user?.id === identity &&
      !(await redirectSession(cause))
    )
      mutationError.value = cause
  } finally {
    if (!own.signal.aborted && auth.user?.id === identity) {
      await load()
      if (mutationController === own) busy.value = false
    }
  }
}

function create() {
  void mutate(
    (signal) => request('/admin/backups', 'POST', {}, signal),
    120_000,
  )
}

function remove(item: BackupRecord) {
  void mutate(async (signal) => {
    await request(
      `/admin/backups/${encodeURIComponent(item.id)}`,
      'DELETE',
      undefined,
      signal,
    )
    deleting.value = null
  })
}

function status(item: BackupRecord) {
  return {
    running: t('backups.running'),
    done: t('backups.done'),
    failed: t('backups.failed'),
  }[item.status]
}

function stage(item: BackupRecord) {
  const labels = {
    dumping: t('backups.dumping'),
    archiving: t('backups.archiving'),
    finalizing: t('backups.finalizing'),
  }
  return item.progress ? labels[item.progress.stage] : ''
}

function timestamp(value: string) {
  return new Date(value).toLocaleString(locale.value, {
    timeZone: auth.user?.effectiveTimezone,
  })
}

watch(
  () => auth.user?.id,
  (id) => {
    reset()
    if (id) void load()
  },
  { immediate: true, flush: 'sync' },
)
onBeforeUnmount(reset)
</script>

<template>
  <section class="admin-backups" :aria-busy="loading || busy">
    <h1>{{ t('backups.title') }}</h1>
    <p>{{ t('backups.notice') }}</p>
    <p v-if="freeBytes !== null">
      {{ t('backups.free', { bytes: n(freeBytes) }) }}
    </p>
    <NButton
      :disabled="!loaded || !!listError || running || busy || loading"
      @click="create"
    >
      {{ t('backups.create') }}
    </NButton>
    <NButton :disabled="loading || busy" @click="load">
      {{ t('backups.refresh') }}
    </NButton>
    <p v-if="loading && !loaded" role="status">{{ t('backups.loading') }}</p>
    <NAlert v-if="listError" type="error" role="alert">
      <p v-if="loaded">{{ t('backups.stale') }}</p>
      <p>{{ errorMessage(listError) }}</p>
      <NButton :disabled="loading || busy" @click="load">
        <span>{{ t('auth.retry') }}</span>
      </NButton>
    </NAlert>
    <NAlert v-if="mutationError" type="error" role="alert">
      {{ errorMessage(mutationError) }}
    </NAlert>
    <p v-if="loaded && items.length === 0">{{ t('backups.empty') }}</p>
    <div v-if="loaded" class="user-list">
      <article
        v-for="item in items"
        :key="item.id"
        class="backup-card"
        :data-backup="item.id"
      >
        <h2>{{ item.fileName }}</h2>
        <p>{{ t('backups.status', { status: status(item) }) }}</p>
        <p v-if="item.progress" role="status">
          {{
            t('backups.progress', {
              stage: stage(item),
              percent: n(item.progress.percent),
            })
          }}
        </p>
        <p v-if="item.errorCode" role="alert">
          {{ errorMessage(new RequestError(item.errorCode, 500)) }}
        </p>
        <p>
          {{
            item.sizeBytes === null
              ? t('backups.sizePending')
              : t('backups.size', { bytes: n(item.sizeBytes) })
          }}
        </p>
        <p>{{ t('backups.createdAt', { time: timestamp(item.createdAt) }) }}</p>
        <p>
          {{
            t('backups.createdBy', {
              name: item.createdByName || t('backups.unknownCreator'),
            })
          }}
        </p>
        <a
          v-if="item.status === 'done'"
          :href="`/api/v1/admin/backups/${encodeURIComponent(item.id)}/download`"
          target="_blank"
          rel="noopener"
          download
        >
          <span>{{ t('backups.download') }}</span>
        </a>
        <NButton
          v-if="item.status !== 'running'"
          :aria-label="t('backups.deleteItem', { name: item.fileName })"
          :disabled="busy || !!listError"
          @click="deleting = item"
        >
          <span>{{ t('backups.delete') }}</span>
        </NButton>
        <div v-if="deleting?.id === item.id">
          <p>{{ t('backups.deleteNotice', { name: item.fileName }) }}</p>
          <NButton :disabled="busy || !!listError" @click="remove(item)">
            <span>{{ t('backups.confirmDelete') }}</span>
          </NButton>
          <NButton :disabled="busy" @click="deleting = null">
            <span>{{ t('backups.cancel') }}</span>
          </NButton>
        </div>
      </article>
    </div>
  </section>
</template>
