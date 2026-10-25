<script setup lang="ts">
import { ref } from 'vue'
import { NAlert, NButton, NCard, NFormItem, NInput, NTag } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { errorMessage } from '../api/client'

const { t } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const login = ref('')
const password = ref('')
const busy = ref(false)
const error = ref<unknown>(null)

async function submit() {
  if (busy.value) return
  busy.value = true
  error.value = null
  try {
    await auth.login({ login: login.value, password: password.value })
    password.value = ''
    await router.replace(
      auth.user?.mustChangePassword ? '/change-password' : '/home',
    )
  } catch (cause) {
    error.value = cause
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <section class="welcome">
    <p class="eyebrow">{{ t('login.eyebrow') }}</p>
    <h1>{{ t('login.title') }}</h1>
    <p class="welcome-description">{{ t('login.description') }}</p>
    <NCard class="welcome-card">
      <NTag type="success" round>{{ t('login.status') }}</NTag>
      <p>{{ t('login.notice') }}</p>
      <NAlert v-if="auth.passwordChanged" type="success" role="status">
        {{ t('auth.passwordChanged') }}
      </NAlert>
      <form class="auth-form" @submit.prevent="submit">
        <NFormItem :label="t('auth.login')">
          <NInput
            v-model:value="login"
            :input-props="{
              'aria-label': t('auth.login'),
              autocomplete: 'username',
              required: true,
            }"
          />
        </NFormItem>
        <NFormItem :label="t('auth.password')">
          <NInput
            v-model:value="password"
            type="password"
            show-password-on="click"
            :input-props="{
              'aria-label': t('auth.password'),
              autocomplete: 'current-password',
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
          {{ t('auth.signIn') }}
        </NButton>
      </form>
    </NCard>
  </section>
</template>
