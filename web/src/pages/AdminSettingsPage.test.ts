import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { nextTick } from 'vue'
import type { AdminSettings } from '../api/types'
import { i18n, loadLocale } from '../i18n'
import AdminSettingsPage from './AdminSettingsPage.vue'
import { themeMedia } from '../test/setup'

const replace = vi.fn()
vi.mock('vue-router', () => ({ useRouter: () => ({ replace }) }))
beforeEach(() => vi.stubGlobal('matchMedia', () => themeMedia))
afterEach(() => {
  vi.unstubAllGlobals()
  localStorage.clear()
  replace.mockReset()
})
const settings: AdminSettings = {
  appName: 'Team',
  defaultTimezone: 'Asia/Shanghai',
  defaultCurrency: 'CNY',
  defaultLocale: 'en',
  uploadMaxMb: 20,
  imageMaxMegapixels: 16,
  historyRetentionDays: 90,
  trashRetentionDays: 30,
}

it('loads catalogs, sends only changed fields and shows persisted values/notices in both locales', async () => {
  await loadLocale('en')
  await loadLocale('zh-CN')
  i18n.global.locale.value = 'en'
  let release!: (value: Response) => void
  const fetch = vi.fn((path: string, options: RequestInit) => {
    if (options.method === 'PATCH')
      return Promise.resolve(
        Response.json({
          settings: { ...settings, appName: 'Renamed', imageMaxMegapixels: 12 },
          eventId: 'event',
        }),
      )
    if (path.endsWith('/settings'))
      return new Promise<Response>((resolve) => {
        release = resolve
      })
    return Promise.resolve(
      Response.json({
        items: path.endsWith('/timezones')
          ? ['Asia/Shanghai', 'Europe/London']
          : ['CNY', 'USD'],
      }),
    )
  })
  vi.stubGlobal('fetch', fetch)
  const wrapper = mount(AdminSettingsPage, {
    global: { plugins: [createPinia(), i18n] },
  })
  try {
    await nextTick()
    expect(wrapper.text()).toContain('Loading settings')
    release(Response.json(settings))
    await flushPromises()
    expect(wrapper.text()).toContain(
      'History and Trash retention jobs are not active yet.',
    )
    await wrapper
      .get('input[aria-label="Application name"]')
      .setValue('Renamed')
    await wrapper
      .get('input[aria-label="Image decode limit (MP)"]')
      .setValue('12')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(fetch).toHaveBeenLastCalledWith(
      '/api/v1/admin/settings',
      expect.objectContaining({
        method: 'PATCH',
        body: '{"appName":"Renamed","imageMaxMegapixels":12}',
      }),
    )
    expect(wrapper.text()).toContain('Settings saved')
    expect(
      wrapper.get('input[aria-label="Image decode limit (MP)"]').element,
    ).toHaveProperty('value', '12')
    expect(
      wrapper.get('input[aria-label="Application name"]').element,
    ).toHaveProperty('value', 'Renamed')
    i18n.global.locale.value = 'zh-CN'
    await nextTick()
    expect(wrapper.get('h1').text()).toBe('设置')
    expect(wrapper.text()).toContain('历史记录和回收站保留任务尚未启用')
  } finally {
    wrapper.unmount()
    i18n.global.locale.value = 'en'
  }
})

it('shows a failed load with retry and independently redirects denied sessions', async () => {
  await loadLocale('en')
  i18n.global.locale.value = 'en'
  const fetch = vi.fn().mockRejectedValue(new TypeError('offline'))
  vi.stubGlobal('fetch', fetch)
  const wrapper = mount(AdminSettingsPage, {
    global: { plugins: [createPinia(), i18n] },
  })
  try {
    await flushPromises()
    expect(wrapper.find('form').exists()).toBe(false)
    expect(wrapper.get('[role="alert"]').text()).toContain('Try again')
    fetch.mockImplementation((path: string) =>
      Promise.resolve(
        path.endsWith('/settings')
          ? Response.json(
              { error: { code: 'ERR_PERMISSION_DENIED' } },
              { status: 403 },
            )
          : Response.json({ items: [] }),
      ),
    )
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(replace).toHaveBeenCalledWith('/home')
  } finally {
    wrapper.unmount()
  }
})
