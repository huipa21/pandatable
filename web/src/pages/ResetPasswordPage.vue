<script setup lang="ts">
import { ref } from 'vue'
import { NAlert, NButton, NFormItem, NInput } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { errorMessage, request } from '../api/client'
import { useAuthStore } from '../stores/auth'

const { t } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const parameters = new URLSearchParams(window.location.hash.slice(1))
const tokens = parameters.getAll('token')
const token = ref(
  tokens.length === 1 && /^[A-Za-z0-9_-]{43}$/.test(tokens[0] ?? '')
    ? tokens[0]!
    : '',
)
window.history.replaceState(
  window.history.state,
  '',
  window.location.pathname + window.location.search,
)
const password = ref('')
const confirm = ref('')
const mismatch = ref(false)
const busy = ref(false)
const error = ref<unknown>(null)

async function submit() {
  if (busy.value || !token.value) return
  mismatch.value = password.value !== confirm.value
  error.value = null
  if (mismatch.value) return
  busy.value = true
  try {
    await request(
      '/auth/reset/complete',
      'POST',
      { token: token.value, password: password.value },
      AbortSignal.timeout(10_000),
    )
    token.value = password.value = confirm.value = ''
    auth.setUser(null)
    auth.passwordChanged = true
    await router.replace('/login')
  } catch (cause) {
    error.value = cause
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <section class="setup">
    <h1>{{ t('mail.forgotTitle') }}</h1>
    <NAlert v-if="!token" type="error" role="alert">
      {{ t('mail.invalidLink') }}
    </NAlert>
    <template v-else>
      <p>{{ t('mail.resetNotice') }}</p>
      <p>{{ t('auth.passwordPolicy') }}</p>
      <form class="auth-form" @submit.prevent="submit">
        <NFormItem :label="t('auth.newPassword')">
          <NInput
            v-model:value="password"
            type="password"
            :disabled="busy"
            :input-props="{
              'aria-label': t('auth.newPassword'),
              autocomplete: 'new-password',
              required: true,
            }"
          />
        </NFormItem>
        <NFormItem :label="t('auth.confirmPassword')">
          <NInput
            v-model:value="confirm"
            type="password"
            :disabled="busy"
            :input-props="{
              'aria-label': t('auth.confirmPassword'),
              autocomplete: 'new-password',
              required: true,
            }"
          />
        </NFormItem>
        <NAlert v-if="mismatch" type="error" role="alert">
          {{ t('auth.passwordMismatch') }}
        </NAlert>
        <NAlert v-if="error" type="error" role="alert">
          {{ errorMessage(error) }}
        </NAlert>
        <NButton
          type="primary"
          attr-type="submit"
          :loading="busy"
          :disabled="busy"
        >
          {{ t('mail.resetSubmit') }}
        </NButton>
      </form>
    </template>
    <p>
      <RouterLink to="/forgot-password">
        {{ t('mail.requestReset') }}
      </RouterLink>
    </p>
  </section>
</template>
