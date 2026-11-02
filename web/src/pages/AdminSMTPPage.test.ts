import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { i18n, loadLocale } from '../i18n'
import type { SMTPSettings } from '../api/types'
import { themeMedia } from '../test/setup'
import AdminSMTPPage from './AdminSMTPPage.vue'

const replace = vi.fn()
vi.mock('vue-router', () => ({ useRouter: () => ({ replace }) }))

const settings: SMTPSettings = {
  enabled: true,
  host: 'localhost',
  port: 2525,
  security: 'none',
  hasUsername: true,
  hasPassword: true,
  fromAddress: 'sender@example.test',
}

beforeEach(async () => {
  vi.stubGlobal('matchMedia', () => themeMedia)
  await loadLocale('en')
  i18n.global.locale.value = 'en'
})
afterEach(() => {
  vi.unstubAllGlobals()
  replace.mockReset()
})

it('preserves omitted credentials and reports test delivery failures', async () => {
  const fetch = vi.fn((path: string, options: RequestInit) => {
    if (options.method === 'PATCH')
      return Promise.resolve(
        Response.json({
          settings: { ...settings, host: '127.0.0.1' },
          eventId: 'event',
        }),
      )
    if (path.endsWith('/test'))
      return Promise.resolve(
        Response.json({ error: { code: 'ERR_SMTP_SEND' } }, { status: 502 }),
      )
    return Promise.resolve(Response.json(settings))
  })
  vi.stubGlobal('fetch', fetch)
  const wrapper = mount(AdminSMTPPage, {
    global: { plugins: [createPinia(), i18n], stubs: { RouterLink: true } },
  })
  try {
    await flushPromises()
    expect(wrapper.text()).toContain('A username is stored.')
    expect(wrapper.text()).toContain('A password is stored.')
    await wrapper.get('input[aria-label="SMTP host"]').setValue('127.0.0.1')
    await wrapper.findAll('form')[0]!.trigger('submit')
    await flushPromises()
    expect(fetch).toHaveBeenLastCalledWith(
      '/api/v1/admin/smtp',
      expect.objectContaining({ body: '{"host":"127.0.0.1"}' }),
    )
    await wrapper
      .get('input[aria-label="Test recipient email"]')
      .setValue('recipient@example.test')
    await wrapper.findAll('form')[1]!.trigger('submit')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain(
      'The email could not be sent',
    )
  } finally {
    wrapper.unmount()
  }
})

it('sends explicitly entered credentials only on update', async () => {
  const fetch = vi.fn((_: string, options: RequestInit) =>
    Promise.resolve(
      options.method === 'PATCH'
        ? Response.json({ settings, eventId: 'event' })
        : Response.json(settings),
    ),
  )
  vi.stubGlobal('fetch', fetch)
  const wrapper = mount(AdminSMTPPage, {
    global: { plugins: [createPinia(), i18n], stubs: { RouterLink: true } },
  })
  try {
    await flushPromises()
    await wrapper
      .get('input[aria-label="New SMTP username"]')
      .setValue('test-user')
    await wrapper
      .get('input[aria-label="New SMTP password"]')
      .setValue('test-secret')
    await wrapper.findAll('form')[0]!.trigger('submit')
    await flushPromises()
    expect(fetch).toHaveBeenLastCalledWith(
      '/api/v1/admin/smtp',
      expect.objectContaining({
        body: '{"username":"test-user","password":"test-secret"}',
      }),
    )
    expect(
      wrapper.get('input[aria-label="New SMTP username"]').element,
    ).toHaveProperty('value', '')
    expect(
      wrapper.get('input[aria-label="New SMTP password"]').element,
    ).toHaveProperty('value', '')
  } finally {
    wrapper.unmount()
  }
})

it('allows stored credentials to be explicitly removed', async () => {
  const fetch = vi.fn((_: string, options: RequestInit) =>
    Promise.resolve(
      options.method === 'PATCH'
        ? Response.json({
            settings: { ...settings, hasUsername: false, hasPassword: false },
            eventId: 'event',
          })
        : Response.json(settings),
    ),
  )
  vi.stubGlobal('fetch', fetch)
  const wrapper = mount(AdminSMTPPage, {
    global: { plugins: [createPinia(), i18n], stubs: { RouterLink: true } },
  })
  try {
    await flushPromises()
    const labels = wrapper.findAll('label')
    await labels
      .find((label) => label.text().includes('Remove the stored username'))!
      .get('input')
      .setValue(true)
    await labels
      .find((label) => label.text().includes('Remove the stored password'))!
      .get('input')
      .setValue(true)
    await wrapper.findAll('form')[0]!.trigger('submit')
    await flushPromises()
    expect(fetch).toHaveBeenLastCalledWith(
      '/api/v1/admin/smtp',
      expect.objectContaining({ body: '{"username":"","password":""}' }),
    )
    expect(wrapper.text()).not.toContain('A username is stored.')
    expect(wrapper.text()).not.toContain('A password is stored.')
  } finally {
    wrapper.unmount()
  }
})
