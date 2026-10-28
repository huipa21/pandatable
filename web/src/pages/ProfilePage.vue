<script setup lang="ts">
import { onBeforeUnmount, reactive, ref } from 'vue'
import { NAlert, NButton, NFormItem, NInput, NSpin } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { errorMessage, request, RequestError } from '../api/client'
import type { AuthUser, AuthUserMutation, ProfilePatch } from '../api/types'
import { useAuthStore } from '../stores/auth'
import { usePreferencesStore, type Theme } from '../stores/preferences'

const appDefaultTimezone = '__app_default__'
const { t } = useI18n()
const auth = useAuthStore()
const preferences = usePreferencesStore()
const router = useRouter()
const original = ref<AuthUser | null>(null)
const form = reactive({
  displayName: '',
  locale: 'en' as AuthUser['locale'],
  theme: 'system' as Theme,
  timezone: null as string | null,
})
const timezones = ref<string[]>([])
const loading = ref(false)
const busy = ref(false)
const error = ref<unknown>(null)
const saved = ref(false)
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
  }
}

function assignUser(user: AuthUser) {
  original.value = user
  form.displayName = user.displayName
  form.locale = user.locale
  form.theme = user.theme
  form.timezone = user.timezone
  auth.setUser(user)
}

async function load() {
  controller?.abort()
  controller = new AbortController()
  const own = controller
  const signal = AbortSignal.any([own.signal, AbortSignal.timeout(10_000)])
  const current = ++generation
  loading.value = true
  error.value = null
  saved.value = false
  try {
    const [user, zones] = await Promise.all([
      request<AuthUser>('/auth/me', 'GET', undefined, signal),
      request<{ items: string[] }>('/meta/timezones', 'GET', undefined, signal),
    ])
    if (
      current !== generation ||
      (auth.user !== null && auth.user.id !== user.id)
    )
      return
    assignUser(user)
    timezones.value = zones.items
  } catch (cause) {
    if (!own.signal.aborted && current === generation) await failed(cause)
  } finally {
    if (current === generation) loading.value = false
  }
}

function changeTimezone(value: string) {
  form.timezone = value === appDefaultTimezone ? null : value
}

async function save() {
  const previous = original.value
  if (busy.value || !previous) return
  const patch: ProfilePatch = {}
  if (form.displayName !== previous.displayName)
    patch.displayName = form.displayName
  if (form.locale !== previous.locale) patch.locale = form.locale
  if (form.theme !== previous.theme) patch.theme = form.theme
  if (form.timezone !== previous.timezone) patch.timezone = form.timezone
  saved.value = false
  if (Object.keys(patch).length === 0) return

  busy.value = true
  error.value = null
  controller?.abort()
  controller = new AbortController()
  const own = controller
  const current = ++generation
  try {
    const result = await request<AuthUserMutation>(
      '/auth/me',
      'PATCH',
      patch,
      AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
    )
    if (current !== generation || auth.user?.id !== previous.id) return
    assignUser(result.user)
    await preferences.setLocale(result.user.locale)
    preferences.setTheme(result.user.theme)
    saved.value = true
  } catch (cause) {
    if (!own.signal.aborted && current === generation) await failed(cause)
  } finally {
    if (current === generation) busy.value = false
  }
}

void load()
</script>

<template>
  <section class="profile-page">
    <div class="admin-heading">
      <h1>{{ t('profile.title') }}</h1>
      <RouterLink to="/change-password">
        {{ t('auth.changePassword') }}
      </RouterLink>
    </div>
    <NAlert v-if="error" type="error" role="alert">
      {{ errorMessage(error) }}
      <NButton :disabled="busy || loading" @click="load">
        {{ t('auth.retry') }}
      </NButton>
    </NAlert>
    <p v-if="saved" class="profile-saved" role="status">
      {{ t('profile.saved') }}
    </p>
    <NSpin v-if="loading" :description="t('profile.loading')" />
    <form v-else-if="original" class="profile-form" @submit.prevent="save">
      <NFormItem :label="t('profile.displayName')">
        <NInput
          v-model:value="form.displayName"
          :disabled="busy"
          :input-props="{
            'aria-label': t('profile.displayName'),
            required: true,
          }"
        />
      </NFormItem>
      <NFormItem :label="t('profile.language')">
        <select
          v-model="form.locale"
          :aria-label="t('profile.language')"
          :disabled="busy"
        >
          <option value="en">{{ t('common.languages.en') }}</option>
          <option value="zh-CN">{{ t('common.languages.zhCN') }}</option>
        </select>
      </NFormItem>
      <NFormItem :label="t('profile.theme')">
        <select
          v-model="form.theme"
          :aria-label="t('profile.theme')"
          :disabled="busy"
        >
          <option value="light">{{ t('common.themes.light') }}</option>
          <option value="dark">{{ t('common.themes.dark') }}</option>
          <option value="system">{{ t('common.themes.system') }}</option>
        </select>
      </NFormItem>
      <NFormItem :label="t('profile.timezone')">
        <select
          :value="form.timezone ?? appDefaultTimezone"
          :aria-label="t('profile.timezone')"
          :disabled="busy"
          @change="changeTimezone(($event.target as HTMLSelectElement).value)"
        >
          <option :value="appDefaultTimezone">
            {{ t('profile.useAppDefault') }}
          </option>
          <option v-for="zone in timezones" :key="zone" :value="zone">
            {{ zone }}
          </option>
        </select>
      </NFormItem>
      <div class="user-actions form-wide">
        <NButton
          attr-type="submit"
          type="primary"
          :loading="busy"
          :disabled="busy"
        >
          {{ t('profile.save') }}
        </NButton>
        <NButton :disabled="busy" @click="load">
          {{ t('profile.reload') }}
        </NButton>
      </div>
    </form>
  </section>
</template>
