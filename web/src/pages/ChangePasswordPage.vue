<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { NAlert, NButton, NFormItem, NInput } from 'naive-ui'
import { useAuthStore } from '../stores/auth'
import { errorMessage, request, RequestError } from '../api/client'

const { t } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const current = ref('')
const next = ref('')
const confirm = ref('')
const busy = ref(false)
const error = ref<unknown>(null)
const mismatch = ref(false)

async function submit() {
  if (busy.value) return
  mismatch.value = next.value !== confirm.value
  error.value = null
  if (mismatch.value) return
  busy.value = true
  try {
    await request('/auth/password', 'POST', {
      current: current.value,
      new: next.value,
    })
    current.value = next.value = confirm.value = ''
    auth.setUser(null)
    auth.passwordChanged = true
    await router.replace('/login')
  } catch (cause) {
    if (cause instanceof RequestError && cause.code === 'ERR_UNAUTHENTICATED') {
      auth.setUser(null)
      await router.replace('/login')
    } else error.value = cause
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <section class="setup">
    <h1>{{ t('auth.changePassword') }}</h1>
    <p>{{ t('auth.changeNotice') }}</p>
    <p>{{ t('auth.passwordPolicy') }}</p>
    <form class="auth-form" @submit.prevent="submit">
      <NFormItem :label="t('auth.currentPassword')">
        <NInput
          v-model:value="current"
          type="password"
          :input-props="{
            'aria-label': t('auth.currentPassword'),
            autocomplete: 'current-password',
            required: true,
          }"
        />
      </NFormItem>
      <NFormItem :label="t('auth.newPassword')">
        <NInput
          v-model:value="next"
          type="password"
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
        {{ t('auth.changePassword') }}
      </NButton>
    </form>
  </section>
</template>
