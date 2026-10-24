<script setup lang="ts">
import { NButton, NButtonGroup } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { usePreferencesStore } from '../stores/preferences'

const { t } = useI18n()
const preferences = usePreferencesStore()
</script>

<template>
  <div class="app-shell">
    <header class="app-header">
      <RouterLink class="brand" to="/login">
        <span class="brand-mark" aria-hidden="true">{{
          t('common.mark')
        }}</span>
        <span>{{ t('common.appName') }}</span>
      </RouterLink>
      <div class="preferences" :aria-label="t('common.preferences')">
        <fieldset>
          <legend>{{ t('common.language') }}</legend>
          <NButtonGroup>
            <NButton
              :aria-pressed="preferences.locale === 'en'"
              :type="preferences.locale === 'en' ? 'primary' : 'default'"
              size="small"
              @click="preferences.setLocale('en')"
            >
              {{ t('common.languages.en') }}
            </NButton>
            <NButton
              :aria-pressed="preferences.locale === 'zh-CN'"
              :type="preferences.locale === 'zh-CN' ? 'primary' : 'default'"
              size="small"
              @click="preferences.setLocale('zh-CN')"
            >
              {{ t('common.languages.zhCN') }}
            </NButton>
          </NButtonGroup>
        </fieldset>
        <fieldset>
          <legend>{{ t('common.theme') }}</legend>
          <NButtonGroup>
            <NButton
              :aria-pressed="preferences.theme === 'light'"
              :type="preferences.theme === 'light' ? 'primary' : 'default'"
              size="small"
              @click="preferences.setTheme('light')"
            >
              {{ t('common.themes.light') }}
            </NButton>
            <NButton
              :aria-pressed="preferences.theme === 'dark'"
              :type="preferences.theme === 'dark' ? 'primary' : 'default'"
              size="small"
              @click="preferences.setTheme('dark')"
            >
              {{ t('common.themes.dark') }}
            </NButton>
            <NButton
              :aria-pressed="preferences.theme === 'system'"
              :type="preferences.theme === 'system' ? 'primary' : 'default'"
              size="small"
              @click="preferences.setTheme('system')"
            >
              {{ t('common.themes.system') }}
            </NButton>
          </NButtonGroup>
        </fieldset>
      </div>
    </header>
    <main class="app-main">
      <slot />
    </main>
    <footer class="app-footer">{{ t('common.tagline') }}</footer>
  </div>
</template>
