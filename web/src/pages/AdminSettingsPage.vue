<script setup lang="ts">
import { onBeforeUnmount, reactive, ref } from 'vue'
import { NAlert, NButton, NFormItem, NInput, NSpin } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { errorMessage, request, RequestError } from '../api/client'
import type { AdminSettings, SettingsMutation } from '../api/types'
import { useAuthStore } from '../stores/auth'
import { useAppSettingsStore } from '../stores/appSettings'

const { t } = useI18n()
const auth = useAuthStore()
const app = useAppSettingsStore()
const router = useRouter()
const original = ref<AdminSettings | null>(null)
const form = reactive<AdminSettings>({
  appName: '',
  defaultTimezone: '',
  defaultCurrency: '',
  defaultLocale: 'en',
  uploadMaxMb: 20,
  imageMaxMegapixels: 16,
  historyRetentionDays: 90,
  trashRetentionDays: 30,
})
const timezones = ref<string[]>([])
const currencies = ref<string[]>([])
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
  } else if (cause.code === 'ERR_PERMISSION_DENIED') {
    await router.replace('/home')
  }
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
    const [settings, zones, codes] = await Promise.all([
      request<AdminSettings>('/admin/settings', 'GET', undefined, signal),
      request<{ items: string[] }>('/meta/timezones', 'GET', undefined, signal),
      request<{ items: string[] }>(
        '/meta/currencies',
        'GET',
        undefined,
        signal,
      ),
    ])
    if (current !== generation) return
    original.value = settings
    Object.assign(form, settings)
    timezones.value = zones.items
    currencies.value = codes.items
  } catch (cause) {
    if (!own.signal.aborted && current === generation) await failed(cause)
  } finally {
    if (current === generation) loading.value = false
  }
}

async function save() {
  if (busy.value || !original.value) return
  const previous = original.value
  const patch: Partial<AdminSettings> = {}
  if (form.appName !== previous.appName) patch.appName = form.appName
  if (form.defaultTimezone !== previous.defaultTimezone)
    patch.defaultTimezone = form.defaultTimezone
  if (form.defaultCurrency !== previous.defaultCurrency)
    patch.defaultCurrency = form.defaultCurrency
  if (form.defaultLocale !== previous.defaultLocale)
    patch.defaultLocale = form.defaultLocale
  if (form.uploadMaxMb !== previous.uploadMaxMb)
    patch.uploadMaxMb = form.uploadMaxMb
  if (form.imageMaxMegapixels !== previous.imageMaxMegapixels)
    patch.imageMaxMegapixels = form.imageMaxMegapixels
  if (form.historyRetentionDays !== previous.historyRetentionDays)
    patch.historyRetentionDays = form.historyRetentionDays
  if (form.trashRetentionDays !== previous.trashRetentionDays)
    patch.trashRetentionDays = form.trashRetentionDays
  saved.value = false
  if (Object.keys(patch).length === 0) return
  busy.value = true
  error.value = null
  controller?.abort()
  controller = new AbortController()
  const own = controller
  const current = ++generation
  try {
    const result = await request<SettingsMutation>(
      '/admin/settings',
      'PATCH',
      patch,
      AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
    )
    if (current !== generation) return
    original.value = result.settings
    Object.assign(form, result.settings)
    await app.apply(result.settings)
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
  <section class="admin-settings">
    <div class="admin-heading">
      <h1>{{ t('settings.title') }}</h1>
    </div>
    <p>{{ t('settings.defaultsNotice') }}</p>
    <p class="settings-deferred">{{ t('settings.deferredNotice') }}</p>
    <NAlert v-if="error" type="error" role="alert">
      {{ errorMessage(error) }}
      <NButton :disabled="busy || loading" @click="load">
        {{ t('auth.retry') }}
      </NButton>
    </NAlert>
    <NAlert v-if="saved" type="success" role="status">
      {{ t('settings.saved') }}
    </NAlert>
    <NSpin v-if="loading" :description="t('settings.loading')" />
    <form v-else-if="original" class="settings-form" @submit.prevent="save">
      <NFormItem :label="t('settings.appName')">
        <NInput
          v-model:value="form.appName"
          :disabled="busy"
          :input-props="{ 'aria-label': t('settings.appName'), required: true }"
        />
      </NFormItem>
      <NFormItem :label="t('settings.defaultTimezone')">
        <select
          v-model="form.defaultTimezone"
          :aria-label="t('settings.defaultTimezone')"
          :disabled="busy"
          required
        >
          <option v-for="zone in timezones" :key="zone" :value="zone">
            {{ zone }}
          </option>
        </select>
      </NFormItem>
      <NFormItem :label="t('settings.defaultCurrency')">
        <select
          v-model="form.defaultCurrency"
          :aria-label="t('settings.defaultCurrency')"
          :disabled="busy"
          required
        >
          <option v-for="code in currencies" :key="code" :value="code">
            {{ code }}
          </option>
        </select>
      </NFormItem>
      <NFormItem :label="t('settings.defaultLocale')">
        <select
          v-model="form.defaultLocale"
          :aria-label="t('settings.defaultLocale')"
          :disabled="busy"
        >
          <option value="en">{{ t('common.languages.en') }}</option>
          <option value="zh-CN">{{ t('common.languages.zhCN') }}</option>
        </select>
      </NFormItem>
      <NFormItem :label="t('settings.uploadMaxMb')">
        <input
          v-model.number="form.uploadMaxMb"
          type="number"
          min="1"
          max="100"
          step="1"
          required
          :aria-label="t('settings.uploadMaxMb')"
          :disabled="busy"
        />
      </NFormItem>
      <NFormItem :label="t('settings.imageMaxMegapixels')">
        <input
          v-model.number="form.imageMaxMegapixels"
          type="number"
          min="1"
          max="16"
          step="1"
          required
          :aria-label="t('settings.imageMaxMegapixels')"
          :disabled="busy"
        />
      </NFormItem>
      <NFormItem :label="t('settings.historyRetentionDays')">
        <input
          v-model.number="form.historyRetentionDays"
          type="number"
          min="1"
          max="3650"
          step="1"
          required
          :aria-label="t('settings.historyRetentionDays')"
          :disabled="busy"
        />
      </NFormItem>
      <NFormItem :label="t('settings.trashRetentionDays')">
        <input
          v-model.number="form.trashRetentionDays"
          type="number"
          min="1"
          max="3650"
          step="1"
          required
          :aria-label="t('settings.trashRetentionDays')"
          :disabled="busy"
        />
      </NFormItem>
      <div class="user-actions form-wide">
        <NButton
          attr-type="submit"
          type="primary"
          :loading="busy"
          :disabled="busy"
        >
          {{ t('settings.save') }}
        </NButton>
        <NButton :disabled="busy" @click="load">
          <span>{{ t('settings.reload') }}</span>
        </NButton>
      </div>
    </form>
  </section>
</template>
