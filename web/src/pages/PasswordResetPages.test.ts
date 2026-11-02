import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { i18n, loadLocale } from '../i18n'
import { useAuthStore } from '../stores/auth'
import { themeMedia } from '../test/setup'
import ForgotPasswordPage from './ForgotPasswordPage.vue'
import ResetPasswordPage from './ResetPasswordPage.vue'

const replace = vi.fn()
vi.mock('vue-router', () => ({ useRouter: () => ({ replace }) }))

beforeEach(async () => {
  vi.stubGlobal('matchMedia', () => themeMedia)
  await loadLocale('en')
  i18n.global.locale.value = 'en'
})
afterEach(() => {
  vi.unstubAllGlobals()
  replace.mockReset()
  window.history.replaceState({}, '', '/')
})

it('shows a non-enumerating confirmation for a reset request', async () => {
  const fetch = vi.fn().mockResolvedValue(Response.json({ accepted: true }))
  vi.stubGlobal('fetch', fetch)
  const wrapper = mount(ForgotPasswordPage, {
    global: { plugins: [createPinia(), i18n], stubs: { RouterLink: true } },
  })
  try {
    await wrapper
      .get('input[aria-label="Account email address"]')
      .setValue('user@example.test')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.get('[role="status"]').text()).toContain(
      'If an active account',
    )
    expect(fetch).toHaveBeenCalledWith(
      '/api/v1/auth/reset/request',
      expect.objectContaining({ body: '{"email":"user@example.test"}' }),
    )
  } finally {
    wrapper.unmount()
  }
})

it('removes the reset token from the URL and sends it only in the completion body', async () => {
  const token = 'A'.repeat(43)
  window.history.replaceState({}, '', `/reset-password#token=${token}`)
  const fetch = vi.fn().mockResolvedValue(Response.json({ eventId: 'event' }))
  vi.stubGlobal('fetch', fetch)
  const pinia = createPinia()
  setActivePinia(pinia)
  const wrapper = mount(ResetPasswordPage, {
    global: { plugins: [pinia, i18n], stubs: { RouterLink: true } },
  })
  try {
    expect(window.location.hash).toBe('')
    await wrapper
      .get('input[aria-label="New password"]')
      .setValue('new-password-123')
    await wrapper
      .get('input[aria-label="Confirm new password"]')
      .setValue('new-password-123')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(fetch).toHaveBeenCalledWith(
      '/api/v1/auth/reset/complete',
      expect.objectContaining({
        body: JSON.stringify({ token, password: 'new-password-123' }),
      }),
    )
    expect(useAuthStore().passwordChanged).toBe(true)
    expect(replace).toHaveBeenCalledWith('/login')
  } finally {
    wrapper.unmount()
  }
})
