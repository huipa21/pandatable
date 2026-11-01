<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import { NAlert, NButton, NInput, NSpin } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { errorMessage, request, RequestError } from '../api/client'
import type { AdminWorkspace, WorkspaceListing } from '../api/types'
import ActiveUserPicker from '../components/admin/ActiveUserPicker.vue'
import { useAuthStore } from '../stores/auth'

const { t, n } = useI18n()
const auth = useAuthStore()
const router = useRouter()
const items = ref<AdminWorkspace[]>([])
const total = ref(0)
const q = ref('')
const appliedQ = ref('')
const offset = ref(0)
const limit = ref(50)
const loading = ref(false)
const busy = ref(false)
const error = ref<unknown>(null)
const saved = ref(false)
const rescueId = ref('')
const recipient = ref('')
let controller: AbortController | undefined
let generation = 0
let disposed = false
onBeforeUnmount(() => {
  disposed = true
  generation++
  controller?.abort()
})

async function failed(cause: unknown) {
  error.value = cause
  if (!(cause instanceof RequestError)) return
  if (cause.code === 'ERR_UNAUTHENTICATED') {
    auth.setUser(null)
    await router.replace('/login')
  } else if (cause.code === 'ERR_PASSWORD_CHANGE_REQUIRED') {
    await router.replace('/change-password')
  } else if (cause.code === 'ERR_PERMISSION_DENIED') {
    await router.replace('/home')
  }
}

async function load() {
  controller?.abort()
  controller = new AbortController()
  const own = controller
  const current = ++generation
  loading.value = true
  error.value = null
  const params = new URLSearchParams({
    q: appliedQ.value,
    offset: String(offset.value),
    limit: String(limit.value),
  })
  try {
    const result = await request<WorkspaceListing>(
      `/admin/workspaces?${params}`,
      'GET',
      undefined,
      AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
    )
    if (current !== generation) return
    items.value = result.items
    total.value = result.total
    if (offset.value > 0 && offset.value >= result.total) {
      offset.value = Math.max(
        0,
        Math.floor((result.total - 1) / limit.value) * limit.value,
      )
      await load()
    }
  } catch (cause) {
    if (!own.signal.aborted && current === generation) {
      await failed(cause)
    }
  } finally {
    if (current === generation) loading.value = false
  }
}

function search() {
  appliedQ.value = q.value
  offset.value = 0
  void load()
}
function page(delta: number) {
  offset.value = Math.max(0, offset.value + delta * limit.value)
  void load()
}

function openRescue(id: string) {
  rescueId.value = id
  recipient.value = ''
  saved.value = false
}

async function rescue() {
  if (busy.value || !rescueId.value || !recipient.value) return
  busy.value = true
  error.value = null
  saved.value = false
  try {
    await request(
      `/admin/workspaces/${rescueId.value}/rescue`,
      'POST',
      { userId: recipient.value },
      AbortSignal.timeout(10_000),
    )
    if (disposed) return
    rescueId.value = ''
    recipient.value = ''
    await load()
    if (!disposed && !error.value) saved.value = true
  } catch (cause) {
    if (disposed) return
    rescueId.value = ''
    recipient.value = ''
    await load()
    if (!disposed) await failed(cause)
  } finally {
    if (!disposed) busy.value = false
  }
}
void load()
</script>

<template>
  <section class="admin-workspaces">
    <div class="admin-heading">
      <h1>{{ t('workspaces.title') }}</h1>
    </div>
    <p>{{ t('workspaces.notice') }}</p>
    <p>{{ t('workspaces.sizeNotice') }}</p>
    <form class="user-search" @submit.prevent="search">
      <NInput
        v-model:value="q"
        :input-props="{ 'aria-label': t('workspaces.searchHint') }"
      />
      <NButton attr-type="submit" :disabled="loading">
        <span>{{ t('admin.search') }}</span>
      </NButton>
      <select
        v-model.number="limit"
        :aria-label="t('workspaces.pageSize')"
        :disabled="loading"
        @change="search"
      >
        <option :value="10">{{ t('workspaces.size10') }}</option>
        <option :value="50">{{ t('workspaces.size50') }}</option>
        <option :value="100">{{ t('workspaces.size100') }}</option>
        <option :value="500">{{ t('workspaces.size500') }}</option>
      </select>
    </form>
    <NAlert v-if="error" type="error" role="alert">
      {{ errorMessage(error) }}
      <NButton :disabled="loading" @click="load">{{ t('auth.retry') }}</NButton>
    </NAlert>
    <NAlert v-if="saved" type="success" role="status">
      {{ t('lifecycle.rescued') }}
    </NAlert>
    <NSpin v-if="loading" :description="t('workspaces.loading')" />
    <template v-else-if="!error">
      <p>{{ t('workspaces.total', { total: n(total) }) }}</p>
      <p v-if="items.length === 0">{{ t('workspaces.empty') }}</p>
      <div class="user-list">
        <article
          v-for="item in items"
          :key="item.id"
          class="workspace-card"
          :data-workspace="item.id"
        >
          <h2>{{ item.name }}</h2>
          <p>
            {{
              item.orphaned ? t('workspaces.orphaned') : t('workspaces.owned')
            }}
          </p>
          <p>
            {{
              t('workspaces.attachmentBytes', {
                bytes: n(item.attachmentBytes),
              })
            }}
          </p>
          <h3>{{ t('workspaces.owners') }}</h3>
          <p v-if="item.owners.length === 0">{{ t('workspaces.noOwners') }}</p>
          <ul v-else>
            <li v-for="owner in item.owners" :key="`${owner.type}:${owner.id}`">
              {{ owner.name }}
              <span v-if="owner.username">{{
                t('workspaces.username', { username: owner.username })
              }}</span>
              {{
                owner.type === 'group'
                  ? t('workspaces.groupOwner')
                  : owner.status === 'disabled'
                    ? t('admin.disabled')
                    : t('admin.active')
              }}
            </li>
          </ul>
          <p v-if="item.ownerTotal > item.owners.length">
            {{
              t('workspaces.moreOwners', {
                total: n(item.ownerTotal - item.owners.length),
              })
            }}
          </p>
          <NButton
            v-if="item.orphaned && rescueId !== item.id"
            :disabled="busy || !!rescueId"
            @click="openRescue(item.id)"
          >
            {{ t('lifecycle.rescue') }}
          </NButton>
          <div v-if="rescueId === item.id">
            <ActiveUserPicker
              v-model="recipient"
              :disabled="busy"
              @failure="failed"
            />
            <div class="user-actions">
              <NButton
                type="primary"
                :loading="busy"
                :disabled="busy || !recipient"
                @click="rescue"
              >
                {{ t('lifecycle.rescue') }}
              </NButton>
              <NButton :disabled="busy" @click="rescueId = ''">
                {{ t('admin.cancel') }}
              </NButton>
            </div>
          </div>
        </article>
      </div>
      <div class="user-actions pagination">
        <NButton :disabled="offset === 0" @click="page(-1)">
          <span>{{ t('admin.previous') }}</span>
        </NButton>
        <NButton :disabled="offset + limit >= total" @click="page(1)">
          <span>{{ t('admin.next') }}</span>
        </NButton>
      </div>
    </template>
  </section>
</template>
