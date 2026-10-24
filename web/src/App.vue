<script setup lang="ts">
import { computed, watchEffect } from 'vue'
import { useI18n } from 'vue-i18n'
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
import AppLayout from './layouts/AppLayout.vue'

const preferences = usePreferencesStore()
const { t } = useI18n()
const uiLocale = computed(() => (preferences.locale === 'zh-CN' ? zhCN : enUS))
const dateLocale = computed(() =>
  preferences.locale === 'zh-CN' ? dateZhCN : dateEnUS,
)
const themeOverrides = {
  common: { primaryColor: '#25836b', primaryColorHover: '#31977d' },
}
watchEffect(() => {
  document.title = t('common.appName')
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
