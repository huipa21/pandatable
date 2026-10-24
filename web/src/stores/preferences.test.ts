import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, disposePinia, setActivePinia, type Pinia } from 'pinia'
import { nextTick } from 'vue'
import dayjs from 'dayjs'
import { i18n, loadLocale } from '../i18n'
import * as localization from '../i18n'
import { themeMedia } from '../test/setup'
import { usePreferencesStore } from './preferences'

let pinia: Pinia
beforeEach(async () => {
  localStorage.clear()
  themeMedia.setDark(false)
  pinia = createPinia()
  setActivePinia(pinia)
  await loadLocale('en')
  i18n.global.locale.value = 'en'
})
afterEach(() => {
  disposePinia(pinia)
  vi.restoreAllMocks()
})

describe('AT-003 display preferences', () => {
  it('switches translations and persists language and dark theme across a fresh store', async () => {
    const store = usePreferencesStore()
    await store.setLocale('zh-CN')
    store.setTheme('dark')
    await nextTick()
    expect(i18n.global.t('login.title')).toBe('欢迎使用 PandaTable')
    expect(document.documentElement.lang).toBe('zh-CN')
    expect(document.documentElement.dataset.theme).toBe('dark')
    expect(dayjs.locale()).toBe('zh-cn')

    disposePinia(pinia)
    pinia = createPinia()
    setActivePinia(pinia)
    const restored = usePreferencesStore()
    await restored.setLocale(restored.locale, false)
    expect(restored.locale).toBe('zh-CN')
    expect(restored.theme).toBe('dark')
    expect(restored.isDark).toBe(true)

    await restored.setLocale('en')
    await nextTick()
    expect(i18n.global.t('login.title')).toBe('Welcome to PandaTable')
    expect(dayjs.locale()).toBe('en')
    expect(document.documentElement.lang).toBe('en')
  })

  it('follows system changes only in system mode and persists all three modes', async () => {
    const store = usePreferencesStore()
    expect(store.theme).toBe('system')
    expect(store.isDark).toBe(false)
    themeMedia.setDark(true)
    expect(store.isDark).toBe(true)
    store.setTheme('light')
    expect(store.isDark).toBe(false)
    expect(localStorage.getItem('pandatable.theme')).toBe('light')
    store.setTheme('dark')
    themeMedia.setDark(false)
    expect(store.isDark).toBe(true)
    expect(localStorage.getItem('pandatable.theme')).toBe('dark')
    store.setTheme('system')
    expect(store.isDark).toBe(false)
    expect(localStorage.getItem('pandatable.theme')).toBe('system')
    await nextTick()
    expect(document.documentElement.dataset.theme).toBe('light')
  })

  it('uses browser language when no locale is saved, but respects a saved choice', () => {
    vi.spyOn(navigator, 'languages', 'get').mockReturnValue(['zh-CN'])
    expect(usePreferencesStore().locale).toBe('zh-CN')
    disposePinia(pinia)
    pinia = createPinia()
    setActivePinia(pinia)
    localStorage.setItem('pandatable.locale', 'en')
    expect(usePreferencesStore().locale).toBe('en')
  })

  it('warns on unsupported saved values and falls back to browser/system', () => {
    vi.spyOn(navigator, 'languages', 'get').mockReturnValue(['en-US'])
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    localStorage.setItem('pandatable.locale', 'xx')
    localStorage.setItem('pandatable.theme', 'sepia')
    const store = usePreferencesStore()
    expect(store.locale).toBe('en')
    expect(store.theme).toBe('system')
    expect(warn).toHaveBeenCalledTimes(2)
  })

  it('surfaces blocked storage instead of breaking the shell', async () => {
    vi.spyOn(console, 'warn').mockImplementation(() => {})
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('Blocked', 'QuotaExceededError')
    })
    const store = usePreferencesStore()
    store.setTheme('dark')
    await store.setLocale('zh-CN')
    expect(store.storageError).toBe(true)
    expect(store.isDark).toBe(true)
    expect(store.locale).toBe('zh-CN')
  })

  it('removes the system listener when its store is disposed', () => {
    const remove = vi.spyOn(themeMedia, 'removeEventListener')
    usePreferencesStore()
    disposePinia(pinia)
    expect(remove).toHaveBeenCalledWith('change', expect.any(Function))
  })

  it('reports locale load failures and keeps the active language consistent', async () => {
    localStorage.setItem('pandatable.locale', 'zh-CN')
    vi.spyOn(localization, 'loadLocale').mockRejectedValueOnce(
      new TypeError('Language chunk unavailable'),
    )
    const error = vi.spyOn(console, 'error').mockImplementation(() => {})
    const store = usePreferencesStore()
    await store.setLocale(store.locale, false)
    expect(store.localeError).toBe(true)
    expect(store.locale).toBe('en')
    expect(i18n.global.locale.value).toBe('en')
    expect(error).toHaveBeenCalledOnce()
  })
})
