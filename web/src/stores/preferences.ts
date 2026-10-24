import { computed, onScopeDispose, ref, watchEffect } from 'vue'
import { defineStore } from 'pinia'
import dayjs from 'dayjs'
import 'dayjs/locale/zh-cn'
import { i18n, loadLocale, type Locale } from '../i18n'

export type Theme = 'light' | 'dark' | 'system'

const localeKey = 'pandatable.locale'
const themeKey = 'pandatable.theme'

export const usePreferencesStore = defineStore('preferences', () => {
  const storageError = ref(false)
  const localeError = ref(false)

  function storage(action: 'get' | 'set', key: string, value = '') {
    try {
      if (action === 'get') return localStorage.getItem(key)
      localStorage.setItem(key, value)
    } catch (error) {
      if (
        !(error instanceof DOMException) ||
        !['SecurityError', 'QuotaExceededError'].includes(error.name)
      ) {
        throw error
      }
      console.warn('Display preference storage is unavailable', error.name)
      storageError.value = true
    }
    return null
  }

  const storedLocale = storage('get', localeKey)
  const storedTheme = storage('get', themeKey)
  const browserLocale = navigator.languages[0] ?? navigator.language
  const locale = ref<Locale>(
    storedLocale === 'en' || storedLocale === 'zh-CN'
      ? storedLocale
      : browserLocale.toLowerCase().startsWith('zh')
        ? 'zh-CN'
        : 'en',
  )
  const theme = ref<Theme>(
    storedTheme === 'light' ||
      storedTheme === 'dark' ||
      storedTheme === 'system'
      ? storedTheme
      : 'system',
  )
  if (storedLocale && storedLocale !== 'en' && storedLocale !== 'zh-CN') {
    console.warn('Ignoring unsupported saved locale')
  }
  if (storedTheme && !['light', 'dark', 'system'].includes(storedTheme)) {
    console.warn('Ignoring unsupported saved theme')
  }

  const media = window.matchMedia('(prefers-color-scheme: dark)')
  const systemDark = ref(media.matches)
  function onSystemTheme(event: MediaQueryListEvent) {
    systemDark.value = event.matches
  }
  media.addEventListener('change', onSystemTheme)
  onScopeDispose(() => media.removeEventListener('change', onSystemTheme))
  const isDark = computed(
    () =>
      theme.value === 'dark' || (theme.value === 'system' && systemDark.value),
  )
  watchEffect(() => {
    document.documentElement.dataset.theme = isDark.value ? 'dark' : 'light'
    document.documentElement.lang = locale.value
    dayjs.locale(locale.value === 'zh-CN' ? 'zh-cn' : 'en')
  })

  let localeRequest = 0
  async function setLocale(value: Locale, persist = true) {
    const request = ++localeRequest
    try {
      await loadLocale(value)
    } catch (error) {
      // Dynamic imports can fail on an interrupted connection or a stale deployment.
      console.error('Failed to load display language', error)
      if (request === localeRequest) {
        localeError.value = true
        locale.value = i18n.global.locale.value === 'zh-CN' ? 'zh-CN' : 'en'
      }
      return
    }
    if (request !== localeRequest) return
    localeError.value = false
    i18n.global.locale.value = value
    locale.value = value
    if (persist) storage('set', localeKey, value)
  }

  function setTheme(value: Theme) {
    theme.value = value
    storage('set', themeKey, value)
  }

  return {
    locale,
    theme,
    isDark,
    storageError,
    localeError,
    setLocale,
    setTheme,
  }
})
