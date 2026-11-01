<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import { NAlert, NButton, NInput, NSpin } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { errorMessage, request, RequestError } from '../../api/client'
import type { AdminUser, OwnershipListing } from '../../api/types'
import ActiveUserPicker from './ActiveUserPicker.vue'

const props = defineProps<{ user: AdminUser }>()
const emit = defineEmits<{ deleted: []; failure: [cause: unknown] }>()
const { t } = useI18n()
const ownerships = ref<OwnershipListing>({ items: [], total: 0 })
const offset = ref(0)
const loading = ref(false)
const busy = ref(false)
const error = ref<unknown>(null)
const saved = ref(false)
const transferring = ref(false)
const deleting = ref(false)
const cleanupPending = ref(false)
const recipient = ref('')
const confirmation = ref('')
const resourceLabels = {
  workspace: () => t('lifecycle.workspace'),
  book: () => t('lifecycle.book'),
  table: () => t('lifecycle.table'),
}
let controller: AbortController | undefined
let generation = 0
let disposed = false

onBeforeUnmount(() => {
  disposed = true
  generation++
  controller?.abort()
})

function failed(cause: unknown) {
  error.value = cause
  if (
    cause instanceof RequestError &&
    [
      'ERR_UNAUTHENTICATED',
      'ERR_PASSWORD_CHANGE_REQUIRED',
      'ERR_PERMISSION_DENIED',
    ].includes(cause.code)
  )
    emit('failure', cause)
}

async function load() {
  controller?.abort()
  controller = new AbortController()
  const own = controller
  const current = ++generation
  loading.value = true
  error.value = null
  try {
    const params = new URLSearchParams({
      offset: String(offset.value),
      limit: '50',
    })
    const result = await request<OwnershipListing>(
      `/admin/users/${props.user.id}/ownerships?${params}`,
      'GET',
      undefined,
      AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
    )
    if (current !== generation) return
    ownerships.value = result
    if (offset.value > 0 && offset.value >= result.total) {
      offset.value = Math.max(0, Math.floor((result.total - 1) / 50) * 50)
      await load()
    }
  } catch (cause) {
    if (!own.signal.aborted && current === generation) failed(cause)
  } finally {
    if (current === generation) loading.value = false
  }
}

function page(delta: number) {
  offset.value = Math.max(0, offset.value + delta * 50)
  void load()
}

async function transfer() {
  if (busy.value || !recipient.value) return
  busy.value = true
  error.value = null
  saved.value = false
  try {
    await request(
      `/admin/users/${props.user.id}/ownerships/transfer`,
      'POST',
      { userId: recipient.value },
      AbortSignal.timeout(125_000),
    )
    if (disposed) return
    transferring.value = false
    recipient.value = ''
    await load()
    if (!disposed && !error.value) saved.value = true
  } catch (cause) {
    if (disposed) return
    transferring.value = false
    recipient.value = ''
    await load()
    if (!disposed) failed(cause)
  } finally {
    if (!disposed) busy.value = false
  }
}

async function removeUser() {
  if (busy.value || confirmation.value !== props.user.username) return
  busy.value = true
  error.value = null
  try {
    await request(
      `/admin/users/${props.user.id}`,
      'DELETE',
      { username: confirmation.value },
      AbortSignal.timeout(125_000),
    )
    if (!disposed) emit('deleted')
  } catch (cause) {
    if (disposed) return
    if (
      cause instanceof RequestError &&
      cause.code === 'ERR_USER_CLEANUP_PENDING'
    )
      cleanupPending.value = true
    failed(cause)
  } finally {
    if (!disposed) busy.value = false
  }
}

async function retryCleanup() {
  if (busy.value) return
  busy.value = true
  error.value = null
  try {
    await request(
      '/admin/user-deletions/cleanup',
      'POST',
      undefined,
      AbortSignal.timeout(125_000),
    )
    if (!disposed) emit('deleted')
  } catch (cause) {
    if (!disposed) failed(cause)
  } finally {
    if (!disposed) busy.value = false
  }
}

void load()
</script>

<template>
  <section class="user-lifecycle" :aria-label="t('lifecycle.ownerships')">
    <h2>{{ t('lifecycle.ownerships') }}</h2>
    <p>{{ t('lifecycle.ownershipNotice') }}</p>
    <NAlert v-if="error" type="error" role="alert">
      {{ errorMessage(error) }}
      <NButton v-if="cleanupPending" :disabled="busy" @click="retryCleanup">
        {{ t('lifecycle.cleanupRetry') }}
      </NButton>
      <NButton v-else :disabled="busy || loading" @click="load">
        {{ t('auth.retry') }}
      </NButton>
    </NAlert>
    <NAlert v-if="saved" type="success" role="status">
      {{ t('lifecycle.transferred') }}
    </NAlert>
    <template v-if="!cleanupPending">
      <NSpin v-if="loading" size="small" />
      <template v-else-if="!error">
        <p>{{ t('lifecycle.ownershipTotal', { total: ownerships.total }) }}</p>
        <p v-if="ownerships.total === 0">{{ t('lifecycle.noOwnerships') }}</p>
        <ul v-else>
          <li v-for="item in ownerships.items" :key="item.resourceId">
            {{
              t('lifecycle.ownershipItem', {
                type: resourceLabels[item.resourceType](),
                name: item.name,
              })
            }}
            <span v-if="item.resourceType !== 'workspace'">
              {{
                t('lifecycle.ownershipWorkspace', { name: item.workspaceName })
              }}
            </span>
            <span v-if="item.trashed">{{ t('lifecycle.trashed') }}</span>
          </li>
        </ul>
        <div class="user-actions">
          <NButton :disabled="busy || offset === 0" @click="page(-1)">
            {{ t('admin.previous') }}
          </NButton>
          <NButton
            :disabled="busy || offset + 50 >= ownerships.total"
            @click="page(1)"
          >
            {{ t('admin.next') }}
          </NButton>
        </div>
        <NButton
          v-if="ownerships.total > 0 && !transferring"
          :disabled="busy || deleting"
          @click="transferring = true"
        >
          {{ t('lifecycle.transfer') }}
        </NButton>
        <div v-if="transferring">
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
              @click="transfer"
            >
              {{ t('lifecycle.transfer') }}
            </NButton>
            <NButton :disabled="busy" @click="transferring = false">
              {{ t('admin.cancel') }}
            </NButton>
          </div>
        </div>
      </template>
      <NButton
        v-if="!deleting"
        type="error"
        :disabled="busy || loading || transferring || !!error"
        @click="deleting = true"
      >
        {{ t('lifecycle.delete') }}
      </NButton>
      <form v-else @submit.prevent="removeUser">
        <NAlert type="warning">{{ t('lifecycle.deleteNotice') }}</NAlert>
        <label>
          {{ t('lifecycle.confirmUsername') }}
          <NInput
            v-model:value="confirmation"
            :input-props="{
              'aria-label': t('lifecycle.confirmUsername'),
              autocomplete: 'off',
            }"
          />
        </label>
        <div class="user-actions">
          <NButton
            type="error"
            attr-type="submit"
            :loading="busy"
            :disabled="busy || confirmation !== user.username"
          >
            {{ t('lifecycle.deleteConfirm') }}
          </NButton>
          <NButton :disabled="busy" @click="deleting = false">
            {{ t('admin.cancel') }}
          </NButton>
        </div>
      </form>
    </template>
    <RouterLink v-else to="/admin/users">
      {{ t('lifecycle.backUsers') }}
    </RouterLink>
  </section>
</template>
