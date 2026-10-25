<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, reactive, ref } from 'vue'
import { NAlert, NButton, NCard, NFormItem, NInput, NSpin } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { errorMessage, request, RequestError } from '../api/client'
import type { AdminUser, AdminUserMutation, UserListing } from '../api/types'
import { useAuthStore } from '../stores/auth'

const { t } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const users = ref<AdminUser[]>([])
const total = ref(0)
const q = ref('')
const appliedQ = ref('')
const status = ref('')
const limit = ref(50)
const offset = ref(0)
const loading = ref(false)
const busy = ref(false)
const error = ref<unknown>(null)
const saved = ref(false)
const mode = ref<'create' | 'edit' | 'reset' | 'status' | null>(null)
const target = ref<AdminUser | null>(null)
const formTitle = ref<HTMLElement>()
const form = reactive({
  username: '',
  email: '',
  displayName: '',
  role: 'user' as AdminUser['role'],
  password: '',
})
const title = computed(() => {
  switch (mode.value) {
    case 'create':
      return t('admin.create')
    case 'edit':
      return t('admin.edit')
    case 'reset':
      return t('admin.reset')
    default:
      return target.value?.status === 'active'
        ? t('admin.disable')
        : t('admin.enable')
  }
})
let controller: AbortController | undefined
let listGeneration = 0

onBeforeUnmount(() => {
  listGeneration++
  controller?.abort()
  form.password = ''
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
  const signal = controller.signal
  const generation = ++listGeneration
  loading.value = true
  error.value = null
  try {
    const params = new URLSearchParams({
      q: appliedQ.value,
      status: status.value,
      offset: String(offset.value),
      limit: String(limit.value),
    })
    const result = await request<UserListing>(
      `/admin/users?${params}`,
      'GET',
      undefined,
      signal,
    )
    if (generation !== listGeneration) return
    users.value = result.items
    total.value = result.total
  } catch (cause) {
    if (!signal.aborted && generation === listGeneration) await failed(cause)
  } finally {
    if (generation === listGeneration) loading.value = false
  }
}

function search() {
  offset.value = 0
  appliedQ.value = q.value
  saved.value = false
  void load()
}

function page(delta: number) {
  offset.value = Math.max(0, offset.value + delta * limit.value)
  void load()
}

async function open(
  action: NonNullable<typeof mode.value>,
  user: AdminUser | null = null,
) {
  target.value = user
  form.username = user?.username ?? ''
  form.email = user?.email ?? ''
  form.displayName = user?.displayName ?? ''
  form.role = user?.role ?? 'user'
  form.password = ''
  error.value = null
  saved.value = false
  mode.value = action
  await nextTick()
  formTitle.value?.focus()
}

function close() {
  mode.value = null
  target.value = null
  form.password = ''
}

async function submit() {
  if (busy.value) return
  busy.value = true
  error.value = null
  saved.value = false
  const user = target.value
  try {
    let revokeSelf = false
    if (mode.value === 'create') {
      await request<AdminUserMutation>('/admin/users', 'POST', { ...form })
    } else if (user && mode.value === 'reset') {
      await request(`/admin/users/${user.id}/reset-password`, 'POST', {
        password: form.password,
      })
      revokeSelf = user.id === auth.user?.id
    } else if (user) {
      const patch =
        mode.value === 'status'
          ? { status: user.status === 'active' ? 'disabled' : 'active' }
          : {
              displayName: form.displayName,
              email: form.email || null,
              role: form.role,
            }
      const result = await request<AdminUserMutation>(
        `/admin/users/${user.id}`,
        'PATCH',
        patch,
      )
      revokeSelf = user.id === auth.user?.id && result.sessionsRevoked === true
    }
    close()
    if (revokeSelf) {
      auth.setUser(null)
      await router.replace('/login')
      return
    }
    saved.value = true
    await load()
  } catch (cause) {
    await failed(cause)
  } finally {
    form.password = ''
    busy.value = false
  }
}

void load()
</script>

<template>
  <section class="admin-users">
    <div class="admin-heading">
      <h1>{{ t('admin.users') }}</h1>
      <NButton type="primary" :disabled="busy" @click="open('create')">
        {{ t('admin.create') }}
      </NButton>
    </div>
    <NAlert v-if="error" type="error" role="alert">
      {{ errorMessage(error) }}
    </NAlert>
    <NAlert v-if="saved" type="success" role="status">
      {{ t('admin.saved') }}
    </NAlert>
    <NCard v-if="mode" class="admin-editor">
      <h2 ref="formTitle" tabindex="-1">{{ title }}</h2>
      <p v-if="target">{{ target.username }}</p>
      <form class="user-form" @submit.prevent="submit">
        <template v-if="mode === 'create' || mode === 'edit'">
          <NFormItem v-if="mode === 'create'" :label="t('admin.username')">
            <NInput
              v-model:value="form.username"
              :input-props="{
                'aria-label': t('admin.username'),
                autocomplete: 'off',
                required: true,
              }"
            />
          </NFormItem>
          <NFormItem :label="t('admin.displayName')">
            <NInput
              v-model:value="form.displayName"
              :input-props="{
                'aria-label': t('admin.displayName'),
                autocomplete: 'off',
                required: true,
              }"
            />
          </NFormItem>
          <NFormItem :label="t('admin.email')">
            <NInput
              v-model:value="form.email"
              :input-props="{
                'aria-label': t('admin.email'),
                autocomplete: 'off',
              }"
            />
          </NFormItem>
          <NFormItem :label="t('admin.role')">
            <select v-model="form.role" :aria-label="t('admin.role')">
              <option value="user">{{ t('admin.regularUser') }}</option>
              <option value="admin">{{ t('admin.administrator') }}</option>
            </select>
          </NFormItem>
        </template>
        <template v-if="mode === 'create' || mode === 'reset'">
          <NFormItem :label="t('admin.temporaryPassword')">
            <NInput
              v-model:value="form.password"
              type="password"
              show-password-on="click"
              :input-props="{
                'aria-label': t('admin.temporaryPassword'),
                autocomplete: 'new-password',
                required: true,
              }"
            />
          </NFormItem>
          <p class="form-wide">
            {{ t('auth.passwordPolicy') }} {{ t('admin.forcedNotice') }}
          </p>
        </template>
        <p v-if="mode === 'edit'" class="form-wide">
          {{ t('admin.roleNotice') }}
        </p>
        <p v-if="mode === 'status'" class="form-wide">
          {{ t('admin.statusNotice') }}
        </p>
        <div class="user-actions form-wide">
          <NButton
            type="primary"
            attr-type="submit"
            :loading="busy"
            :disabled="busy"
          >
            {{ t('admin.confirm') }}
          </NButton>
          <NButton :disabled="busy" @click="close">
            {{ t('admin.cancel') }}
          </NButton>
        </div>
      </form>
    </NCard>
    <form class="user-search" @submit.prevent="search">
      <NInput
        v-model:value="q"
        :placeholder="t('admin.searchHint')"
        :input-props="{ 'aria-label': t('admin.searchHint') }"
      />
      <select v-model="status" :aria-label="t('admin.status')" @change="search">
        <option value="">{{ t('admin.allStatuses') }}</option>
        <option value="active">{{ t('admin.active') }}</option>
        <option value="disabled">{{ t('admin.disabled') }}</option>
      </select>
      <select
        v-model="limit"
        :aria-label="t('admin.pageSize')"
        @change="search"
      >
        <option :value="10">{{ t('admin.size10') }}</option>
        <option :value="50">{{ t('admin.size50') }}</option>
        <option :value="100">{{ t('admin.size100') }}</option>
        <option :value="500">{{ t('admin.size500') }}</option>
      </select>
      <NButton attr-type="submit" :disabled="busy">
        {{ t('admin.search') }}
      </NButton>
    </form>
    <NSpin v-if="loading" size="small" />
    <p>{{ t('admin.total', { total }) }}</p>
    <div class="user-list" :aria-busy="loading">
      <article
        v-for="user in users"
        :key="user.id"
        class="user-card"
        :data-user="user.username"
      >
        <div class="user-details">
          <h2>{{ user.displayName }}</h2>
          <p>{{ user.username }}</p>
          <p v-if="user.email">{{ user.email }}</p>
          <p>
            {{
              t('admin.roleStatus', {
                role:
                  user.role === 'admin'
                    ? t('admin.administrator')
                    : t('admin.regularUser'),
                status:
                  user.status === 'active'
                    ? t('admin.active')
                    : t('admin.disabled'),
              })
            }}
          </p>
          <p v-if="user.mustChangePassword">{{ t('admin.mustChange') }}</p>
        </div>
        <div class="user-actions">
          <NButton size="small" :disabled="busy" @click="open('edit', user)">
            {{ t('admin.edit') }}
          </NButton>
          <NButton size="small" :disabled="busy" @click="open('status', user)">
            {{
              user.status === 'active' ? t('admin.disable') : t('admin.enable')
            }}
          </NButton>
          <NButton size="small" :disabled="busy" @click="open('reset', user)">
            {{ t('admin.reset') }}
          </NButton>
        </div>
      </article>
      <p v-if="!loading && !users.length">{{ t('admin.empty') }}</p>
    </div>
    <div class="user-actions pagination">
      <NButton :disabled="loading || offset === 0" @click="page(-1)">
        {{ t('admin.previous') }}
      </NButton>
      <NButton :disabled="loading || offset + limit >= total" @click="page(1)">
        {{ t('admin.next') }}
      </NButton>
    </div>
  </section>
</template>
