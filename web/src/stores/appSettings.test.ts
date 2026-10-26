import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createPinia, disposePinia, setActivePinia, type Pinia } from 'pinia'
import { useAppSettingsStore } from './appSettings'
import { usePreferencesStore } from './preferences'
import { loadLocale } from '../i18n'
import { themeMedia } from '../test/setup'

let pinia: Pinia
beforeEach(async () => {
  localStorage.clear()
  vi.stubGlobal('matchMedia', () => themeMedia)
  pinia = createPinia()
  setActivePinia(pinia)
  await loadLocale('en')
  await loadLocale('zh-CN')
})
afterEach(() => {
  disposePinia(pinia)
  localStorage.clear()
  vi.unstubAllGlobals()
})

it('loads only public branding/defaults, applies fallback and preserves explicit local preferences', async () => {
  const fetch = vi
    .fn()
    .mockResolvedValue(
      Response.json({ appName: '<Team>', defaultLocale: 'zh-CN' }),
    )
  vi.stubGlobal('fetch', fetch)
  const store = useAppSettingsStore()
  await store.refresh()
  expect(store.appName).toBe('<Team>')
  expect(store.error).toBe(false)
  expect(store.loading).toBe(false)
  expect(usePreferencesStore().locale).toBe('zh-CN')
  expect(localStorage.getItem('pandatable.locale')).toBeNull()
  expect(fetch).toHaveBeenCalledWith(
    '/api/v1/meta/app',
    expect.objectContaining({ cache: 'no-store' }),
  )
  await usePreferencesStore().setLocale('en')
  await store.apply({ appName: 'Updated', defaultLocale: 'zh-CN' })
  expect(usePreferencesStore().locale).toBe('en')
})

it('surfaces public load failure and permits retry without fake success', async () => {
  const fetch = vi
    .fn()
    .mockRejectedValueOnce(new TypeError('offline'))
    .mockResolvedValueOnce(
      Response.json({ appName: 'Recovered', defaultLocale: 'en' }),
    )
  vi.stubGlobal('fetch', fetch)
  const store = useAppSettingsStore()
  await store.refresh()
  expect(store.appName).toBeNull()
  expect(store.error).toBe(true)
  expect(store.loading).toBe(false)
  await store.refresh()
  expect(store.appName).toBe('Recovered')
  expect(store.error).toBe(false)
})

it('ignores an old public response after a committed admin update', async () => {
  let finish!: (value: Response) => void
  vi.stubGlobal(
    'fetch',
    vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          finish = resolve
        }),
    ),
  )
  const store = useAppSettingsStore()
  const pending = store.refresh()
  await store.apply({ appName: 'Committed', defaultLocale: 'en' })
  finish(Response.json({ appName: 'Stale', defaultLocale: 'zh-CN' }))
  await pending
  expect(store.appName).toBe('Committed')
  expect(store.loading).toBe(false)
})
