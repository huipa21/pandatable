<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NFormItem, NInput, NSpin } from 'naive-ui'
import { usePreferencesStore } from '../stores/preferences'
import type { APIError, AuthUserMutation } from '../api/types'

const { t } = useI18n()
const preferences = usePreferencesStore()
const setupErrorCodes = [
  'ERR_VALIDATION',
  'ERR_SETUP_TOKEN',
  'ERR_SETUP_COMPLETE',
  'ERR_AUTH_BUSY',
] as const
type SetupErrorCode = (typeof setupErrorCodes)[number] | 'ERR_INTERNAL'

const state = ref<'loading' | 'form' | 'complete' | 'success' | 'error'>(
  'loading',
)
const busy = ref(false)
const showError = ref(false)
const errorCode = ref<SetupErrorCode>('ERR_INTERNAL')
const user = ref<AuthUserMutation['user']>()
const form = reactive({
  token: '',
  username: '',
  email: '',
  displayName: '',
  password: '',
})
const errorMessage = computed(() => {
  switch (errorCode.value) {
    case 'ERR_VALIDATION':
      return t('errors.ERR_VALIDATION')
    case 'ERR_SETUP_TOKEN':
      return t('errors.ERR_SETUP_TOKEN')
    case 'ERR_SETUP_COMPLETE':
      return t('errors.ERR_SETUP_COMPLETE')
    case 'ERR_AUTH_BUSY':
      return t('errors.ERR_AUTH_BUSY')
    default:
      return t('errors.ERR_INTERNAL')
  }
})

function safeErrorCode(code: string): SetupErrorCode {
  return setupErrorCodes.includes(code as (typeof setupErrorCodes)[number])
    ? (code as (typeof setupErrorCodes)[number])
    : 'ERR_INTERNAL'
}

async function loadStatus() {
  try {
    const response = await fetch('/api/v1/setup/status', { cache: 'no-store' })
    if (!response.ok) throw new Error('setup status unavailable')
    const result = (await response.json()) as { needed: boolean }
    state.value = result.needed ? 'form' : 'complete'
  } catch {
    errorCode.value = 'ERR_INTERNAL'
    state.value = 'error'
  }
}

async function submit() {
  busy.value = true
  showError.value = false
  try {
    const response = await fetch('/api/v1/setup', {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json', 'X-PT-CSRF': '1' },
      body: JSON.stringify({
        ...form,
        displayName: form.displayName,
        locale: preferences.locale,
      }),
    })
    const result = (await response.json()) as AuthUserMutation | APIError
    if (response.status === 410) {
      state.value = 'complete'
      return
    }
    if (!response.ok) {
      errorCode.value =
        'error' in result ? safeErrorCode(result.error.code) : 'ERR_INTERNAL'
      showError.value = true
      return
    }
    user.value = (result as AuthUserMutation).user
    form.token = ''
    form.password = ''
    state.value = 'success'
  } catch {
    errorCode.value = 'ERR_INTERNAL'
    showError.value = true
  } finally {
    busy.value = false
  }
}

void loadStatus()
</script>

<template>
  <section class="setup" aria-live="polite">
    <p class="eyebrow">{{ t('setup.eyebrow') }}</p>
    <h1 v-if="state === 'success'">{{ t('setup.successTitle') }}</h1>
    <h1 v-else>{{ t('setup.title') }}</h1>
    <p class="setup-description">
      <template v-if="state === 'form'">{{ t('setup.description') }}</template>
      <template v-else>{{ t('setup.statusDescription') }}</template>
    </p>

    <NSpin v-if="state === 'loading'" size="small" />
    <form
      v-else-if="state === 'form'"
      class="setup-form"
      @submit.prevent="submit"
    >
      <NFormItem :label="t('setup.token')">
        <NInput
          v-model:value="form.token"
          :input-props="{ 'aria-label': t('setup.token'), autocomplete: 'off' }"
          required
        />
      </NFormItem>
      <NFormItem :label="t('setup.username')">
        <NInput
          v-model:value="form.username"
          :input-props="{
            'aria-label': t('setup.username'),
            autocomplete: 'username',
          }"
          required
        />
      </NFormItem>
      <NFormItem :label="t('setup.email')">
        <NInput
          v-model:value="form.email"
          :input-props="{
            'aria-label': t('setup.email'),
            autocomplete: 'email',
          }"
          required
        />
      </NFormItem>
      <NFormItem :label="t('setup.displayName')">
        <NInput
          v-model:value="form.displayName"
          :input-props="{
            'aria-label': t('setup.displayName'),
            autocomplete: 'name',
          }"
          required
        />
      </NFormItem>
      <NFormItem :label="t('setup.password')">
        <NInput
          v-model:value="form.password"
          type="password"
          :input-props="{
            'aria-label': t('setup.password'),
            autocomplete: 'new-password',
          }"
          show-password-on="click"
          required
        />
      </NFormItem>
      <NAlert v-if="showError" type="error" role="alert">
        {{ errorMessage }}
      </NAlert>
      <NButton
        class="setup-submit"
        type="primary"
        attr-type="submit"
        :loading="busy"
      >
        {{ t('setup.submit') }}
      </NButton>
    </form>

    <div
      v-else-if="state === 'success' && user"
      class="setup-result"
      role="status"
    >
      <p>{{ t('setup.signedInAs', { name: user.displayName }) }}</p>
      <p class="setup-identity">
        {{
          t('setup.identity', { username: user.username, email: user.email })
        }}
      </p>
    </div>
    <div v-else-if="state === 'complete'" class="setup-result" role="status">
      <p>{{ t('setup.alreadyComplete') }}</p>
    </div>
    <NAlert v-else-if="state === 'error'" type="error" role="alert">
      {{ errorMessage }}
    </NAlert>
  </section>
</template>
