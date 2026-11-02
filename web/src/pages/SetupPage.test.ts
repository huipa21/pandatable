import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { i18n, loadLocale } from '../i18n'
import { themeMedia } from '../test/setup'
import SetupPage from './SetupPage.vue'

beforeEach(() => vi.stubGlobal('matchMedia', () => themeMedia))
afterEach(() => vi.unstubAllGlobals())

it.each(['en', 'zh-CN'] as const)(
  'shows the localized CSRF error rather than an internal error (%s)',
  async (locale) => {
    await loadLocale(locale)
    i18n.global.locale.value = locale
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(Response.json({ needed: true }))
      .mockResolvedValueOnce(
        Response.json(
          { error: { code: 'ERR_CSRF', message: 'untrusted server message' } },
          { status: 403 },
        ),
      )
    vi.stubGlobal('fetch', fetch)
    const wrapper = mount(SetupPage, {
      global: { plugins: [createPinia(), i18n] },
    })
    try {
      await flushPromises()
      await wrapper.get('form').trigger('submit')
      await flushPromises()
      expect(wrapper.get('[role="alert"]').text()).toContain(
        i18n.global.t('errors.ERR_CSRF'),
      )
      expect(wrapper.text()).not.toContain('untrusted server message')
      expect(fetch).toHaveBeenLastCalledWith(
        '/api/v1/setup',
        expect.objectContaining({
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-PT-CSRF': '1' },
        }),
      )
    } finally {
      wrapper.unmount()
    }
  },
)
