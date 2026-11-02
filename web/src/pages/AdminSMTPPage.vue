<script setup lang="ts">
import { onBeforeUnmount, reactive, ref } from 'vue'
import { NAlert, NButton, NFormItem, NInput, NSpin } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { errorMessage, request, RequestError } from '../api/client'
import type { SMTPPatch, SMTPSettings } from '../api/types'
import { useAuthStore } from '../stores/auth'

const { t } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const original = ref<SMTPSettings | null>(null)
const form = reactive<SMTPSettings>({
  enabled: false,
  host: '',
  port: 587,
  security: 'starttls',
  hasUsername: false,
  hasPassword: false,
  fromAddress: '',
})
const username = ref('')
const clearUsername = ref(false)
const password = ref('')
const clearPassword = ref(false)
const recipient = ref('')
const loading = ref(false)
const busy = ref(false)
const saved = ref(false)
const sent = ref(false)
const error = ref<unknown>(null)
let controller: AbortController | undefined
let generation = 0

onBeforeUnmount(() => {
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

function nextRequest() {
  controller?.abort()
  controller = new AbortController()
  const own = controller
  return {
    own,
    current: ++generation,
    signal: AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
  }
}

async function load() {
  const { own, current, signal } = nextRequest()
  loading.value = true
  error.value = null
  saved.value = sent.value = false
  try {
    const settings = await request<SMTPSettings>(
      '/admin/smtp',
      'GET',
      undefined,
      signal,
    )
    if (current !== generation) return
    original.value = settings
    Object.assign(form, settings)
    username.value = ''
    clearUsername.value = false
    password.value = ''
    clearPassword.value = false
  } catch (cause) {
    if (!own.signal.aborted && current === generation) await failed(cause)
  } finally {
    if (current === generation) loading.value = false
  }
}

async function save() {
  const previous = original.value
  if (busy.value || !previous) return
  const patch: SMTPPatch = {}
  if (form.enabled !== previous.enabled) patch.enabled = form.enabled
  if (form.host !== previous.host) patch.host = form.host
  if (form.port !== previous.port) patch.port = form.port
  if (form.security !== previous.security) patch.security = form.security
  if (clearUsername.value) patch.username = ''
  else if (username.value) patch.username = username.value
  if (form.fromAddress !== previous.fromAddress)
    patch.fromAddress = form.fromAddress
  if (clearPassword.value) patch.password = ''
  else if (password.value) patch.password = password.value
  saved.value = sent.value = false
  if (Object.keys(patch).length === 0) return
  busy.value = true
  error.value = null
  const { own, current, signal } = nextRequest()
  try {
    const result = await request<{ settings: SMTPSettings }>(
      '/admin/smtp',
      'PATCH',
      patch,
      signal,
    )
    if (current !== generation) return
    original.value = result.settings
    Object.assign(form, result.settings)
    username.value = ''
    clearUsername.value = false
    password.value = ''
    clearPassword.value = false
    saved.value = true
  } catch (cause) {
    if (!own.signal.aborted && current === generation) await failed(cause)
  } finally {
    if (current === generation) busy.value = false
  }
}

async function sendTest() {
  if (busy.value) return
  busy.value = true
  error.value = null
  saved.value = sent.value = false
  const { own, current, signal } = nextRequest()
  try {
    await request('/admin/smtp/test', 'POST', { to: recipient.value }, signal)
    if (current === generation) sent.value = true
  } catch (cause) {
    if (!own.signal.aborted && current === generation) await failed(cause)
  } finally {
    if (current === generation) busy.value = false
  }
}

void load()
</script>

<template>
  <section class="admin-settings">
    <h1>{{ t('mail.adminTitle') }}</h1>
    <p>{{ t('mail.adminNotice') }}</p>
    <NAlert v-if="error" type="error" role="alert">
      {{ errorMessage(error) }}
      <NButton :disabled="busy || loading" @click="load">
        {{ t('auth.retry') }}
      </NButton>
    </NAlert>
    <p v-if="saved" class="profile-saved" role="status">
      {{ t('mail.saved') }}
    </p>
    <p v-if="sent" class="profile-saved" role="status">
      {{ t('mail.testSent') }}
    </p>
    <NSpin v-if="loading" :description="t('mail.loading')" />
    <template v-else-if="original">
      <form class="settings-form" @submit.prevent="save">
        <label class="form-wide">
          <input v-model="form.enabled" type="checkbox" :disabled="busy" />
          {{ t('mail.enabled') }}
        </label>
        <NFormItem :label="t('mail.host')">
          <NInput
            v-model:value="form.host"
            :disabled="busy"
            :input-props="{ 'aria-label': t('mail.host') }"
          />
        </NFormItem>
        <NFormItem :label="t('mail.port')">
          <input
            v-model.number="form.port"
            type="number"
            min="1"
            max="65535"
            required
            :disabled="busy"
            :aria-label="t('mail.port')"
          />
        </NFormItem>
        <NFormItem :label="t('mail.security')">
          <select
            v-model="form.security"
            :disabled="busy"
            :aria-label="t('mail.security')"
          >
            <option value="starttls">{{ t('mail.starttls') }}</option>
            <option value="tls">{{ t('mail.tls') }}</option>
            <option value="none">{{ t('mail.none') }}</option>
          </select>
        </NFormItem>
        <NFormItem :label="t('mail.username')">
          <NInput
            v-model:value="username"
            :disabled="busy || clearUsername"
            :input-props="{
              'aria-label': t('mail.username'),
              autocomplete: 'off',
            }"
          />
        </NFormItem>
        <div>
          <p>{{ t('mail.usernameNotice') }}</p>
          <p v-if="form.hasUsername">{{ t('mail.usernameStored') }}</p>
          <label v-if="form.hasUsername">
            <input v-model="clearUsername" type="checkbox" :disabled="busy" />
            {{ t('mail.clearUsername') }}
          </label>
        </div>
        <NFormItem :label="t('mail.password')">
          <NInput
            v-model:value="password"
            type="password"
            :disabled="busy || clearPassword"
            :input-props="{
              'aria-label': t('mail.password'),
              autocomplete: 'new-password',
            }"
          />
        </NFormItem>
        <div>
          <p>{{ t('mail.passwordNotice') }}</p>
          <p v-if="form.hasPassword">{{ t('mail.passwordStored') }}</p>
          <label v-if="form.hasPassword">
            <input v-model="clearPassword" type="checkbox" :disabled="busy" />
            {{ t('mail.clearPassword') }}
          </label>
        </div>
        <NFormItem :label="t('mail.fromAddress')">
          <NInput
            v-model:value="form.fromAddress"
            :disabled="busy"
            :input-props="{
              'aria-label': t('mail.fromAddress'),
              type: 'email',
            }"
          />
        </NFormItem>
        <div class="user-actions form-wide">
          <NButton
            type="primary"
            attr-type="submit"
            :loading="busy"
            :disabled="busy"
          >
            {{ t('mail.save') }}
          </NButton>
          <NButton :disabled="busy" @click="load">
            {{ t('settings.reload') }}
          </NButton>
        </div>
      </form>
      <form class="auth-form" @submit.prevent="sendTest">
        <NFormItem :label="t('mail.testAddress')">
          <NInput
            v-model:value="recipient"
            :disabled="busy"
            :input-props="{
              'aria-label': t('mail.testAddress'),
              type: 'email',
              required: true,
            }"
          />
        </NFormItem>
        <NButton attr-type="submit" :loading="busy" :disabled="busy">
          {{ t('mail.sendTest') }}
        </NButton>
      </form>
    </template>
    <p>
      <RouterLink to="/admin/settings">{{ t('settings.title') }}</RouterLink>
    </p>
  </section>
</template>
