<script setup lang="ts">
import { ref } from 'vue'
import { NAlert, NButton, NFormItem, NInput } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { errorMessage, request } from '../api/client'

const { t } = useI18n()
const email = ref('')
const busy = ref(false)
const sent = ref(false)
const error = ref<unknown>(null)

async function submit() {
  if (busy.value) return
  busy.value = true
  error.value = null
  sent.value = false
  try {
    await request(
      '/auth/reset/request',
      'POST',
      { email: email.value },
      AbortSignal.timeout(10_000),
    )
    sent.value = true
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
    <p v-if="sent" class="profile-saved" role="status">
      {{ t('mail.requestSent') }}
    </p>
    <form class="auth-form" @submit.prevent="submit">
      <NFormItem :label="t('mail.email')">
        <NInput
          v-model:value="email"
          :disabled="busy"
          :input-props="{
            'aria-label': t('mail.email'),
            type: 'email',
            autocomplete: 'email',
            required: true,
          }"
        />
      </NFormItem>
      <NAlert v-if="error" type="error" role="alert">
        {{ errorMessage(error) }}
      </NAlert>
      <NButton
        type="primary"
        attr-type="submit"
        :loading="busy"
        :disabled="busy"
      >
        {{ t('mail.requestReset') }}
      </NButton>
      <RouterLink to="/login">{{ t('mail.backToLogin') }}</RouterLink>
    </form>
  </section>
</template>
