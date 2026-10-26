import { ref } from 'vue'
import { defineStore } from 'pinia'
import { request } from '../api/client'
import type { AppSettings } from '../api/types'
import { usePreferencesStore } from './preferences'

export const useAppSettingsStore = defineStore('appSettings', () => {
  const preferences = usePreferencesStore()
  const appName = ref<string | null>(null)
  const error = ref(false)
  const loading = ref(false)
  let generation = 0

  async function apply(value: AppSettings) {
    generation++
    appName.value = value.appName
    error.value = false
    loading.value = false
    await preferences.applyDefaultLocale(value.defaultLocale)
  }

  async function refresh() {
    if (loading.value) return
    const current = ++generation
    loading.value = true
    error.value = false
    try {
      const value = await request<AppSettings>(
        '/meta/app',
        'GET',
        undefined,
        AbortSignal.timeout(10_000),
      )
      if (current === generation) await apply(value)
    } catch {
      if (current === generation) error.value = true
    } finally {
      if (current === generation) loading.value = false
    }
  }

  return { appName, error, loading, apply, refresh }
})
