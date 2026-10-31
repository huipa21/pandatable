<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, watch, watchEffect } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import {
  NAlert,
  NConfigProvider,
  NGlobalStyle,
  darkTheme,
  dateEnUS,
  dateZhCN,
  enUS,
  zhCN,
} from 'naive-ui'
import { usePreferencesStore } from './stores/preferences'
import { useAppSettingsStore } from './stores/appSettings'
import { useAuthStore } from './stores/auth'
import { useNavStore } from './stores/nav'
import { useRealtimeStore } from './stores/realtime'
import { useUndoStore } from './stores/undo'
import { errorMessage } from './api/client'
import { NButton } from 'naive-ui'
import AppLayout from './layouts/AppLayout.vue'

const preferences = usePreferencesStore()
const appSettings = useAppSettingsStore()
const auth = useAuthStore()
const nav = useNavStore()
const realtime = useRealtimeStore()
const undo = useUndoStore()
const router = useRouter()
const { t } = useI18n()
const uiLocale = computed(() => (preferences.locale === 'zh-CN' ? zhCN : enUS))
const dateLocale = computed(() =>
  preferences.locale === 'zh-CN' ? dateZhCN : dateEnUS,
)
const themeOverrides = {
  common: { primaryColor: '#25836b', primaryColorHover: '#31977d' },
}
watchEffect(() => {
  document.title = appSettings.appName ?? t('common.appName')
})
const removeRealtimeListener = realtime.onGlobal((event) => {
  if (event.t === 'session.revoked') {
    auth.setUser(null)
    void router.replace('/login')
  } else if (event.t === 'nav.changed' || event.t === 'view.changed') {
    void nav.refresh(true)
  }
})

function undoKeydown(event: KeyboardEvent) {
  if (
    event.defaultPrevented ||
    event.isComposing ||
    event.altKey ||
    !(event.ctrlKey || event.metaKey) ||
    !auth.user ||
    auth.user.mustChangePassword
  )
    return
  const key = event.key.toLowerCase()
  if (key !== 'z' && key !== 'y') return
  if (
    event
      .composedPath()
      .some(
        (target) =>
          target instanceof Element &&
          target.matches('input, textarea, select, [contenteditable="true"]'),
      )
  )
    return
  event.preventDefault()
  event.stopPropagation()
  if (key === 'y' || event.shiftKey) void undo.redo()
  else void undo.undo()
}

onMounted(() => document.addEventListener('keydown', undoKeydown, true))
watch(
  () => (auth.user?.mustChangePassword ? null : (auth.user?.id ?? null)),
  (id) => realtime.start(id),
  { immediate: true, flush: 'sync' },
)
onBeforeUnmount(() => {
  document.removeEventListener('keydown', undoKeydown, true)
  removeRealtimeListener()
  realtime.stop()
})
</script>

<template>
  <NConfigProvider
    :theme="preferences.isDark ? darkTheme : null"
    :theme-overrides="themeOverrides"
    :locale="uiLocale"
    :date-locale="dateLocale"
  >
    <NGlobalStyle />
    <AppLayout>
      <NAlert
        v-if="realtime.status === 'retrying'"
        type="warning"
        role="status"
      >
        {{ t('realtime.reconnecting') }}
      </NAlert>
      <NAlert v-if="appSettings.error" type="error" role="alert">
        {{ t('settings.publicError') }}
        <NButton :loading="appSettings.loading" @click="appSettings.refresh">
          {{ t('auth.retry') }}
        </NButton>
      </NAlert>
      <NAlert v-if="preferences.storageError" type="warning" role="alert">
        {{ t('common.storageError') }}
      </NAlert>
      <NAlert v-if="preferences.localeError" type="error" role="alert">
        {{ t('common.localeError') }}
      </NAlert>
      <NAlert v-if="undo.inconsistent" type="error" role="alert">
        {{ t('undo.partialFailure') }}
      </NAlert>
      <NAlert v-else-if="undo.error" type="error" role="alert">
        {{ errorMessage(undo.error) }}
      </NAlert>
      <RouterView />
    </AppLayout>
  </NConfigProvider>
</template>
