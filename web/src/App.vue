<script setup lang="ts">
import { computed, onBeforeUnmount, watch, watchEffect } from 'vue'
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
import { NButton } from 'naive-ui'
import AppLayout from './layouts/AppLayout.vue'

const preferences = usePreferencesStore()
const appSettings = useAppSettingsStore()
const auth = useAuthStore()
const nav = useNavStore()
const realtime = useRealtimeStore()
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
watch(
  () => (auth.user?.mustChangePassword ? null : (auth.user?.id ?? null)),
  (id) => realtime.start(id),
  { immediate: true, flush: 'sync' },
)
onBeforeUnmount(() => {
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
      <RouterView />
    </AppLayout>
  </NConfigProvider>
</template>
