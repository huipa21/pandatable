import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia, type Pinia } from 'pinia'
import { nextTick } from 'vue'
import { i18n, loadLocale } from '../i18n'
import type { AuthUser, AuthUserMutation } from '../api/types'
import { useAuthStore } from '../stores/auth'
import { usePreferencesStore } from '../stores/preferences'
import { themeMedia } from '../test/setup'
import ProfilePage from './ProfilePage.vue'

const replace = vi.fn()
vi.mock('vue-router', () => ({ useRouter: () => ({ replace }) }))
let pinia: Pinia

const profile: AuthUser = {
  id: 'alice-id',
  username: 'alice',
  email: 'alice@example.test',
  displayName: 'Alice',
  role: 'user',
  mustChangePassword: false,
  locale: 'en',
  theme: 'system',
  timezone: 'Asia/Tokyo',
  effectiveTimezone: 'Asia/Tokyo',
  avatarColor: 'teal',
  mentionEmail: false,
}

beforeEach(async () => {
  vi.stubGlobal('matchMedia', () => themeMedia)
  pinia = createPinia()
  setActivePinia(pinia)
  await loadLocale('en')
  await loadLocale('zh-CN')
  i18n.global.locale.value = 'en'
})

afterEach(() => {
  disposePinia(pinia)
  vi.unstubAllGlobals()
  localStorage.clear()
  replace.mockReset()
})

function responseFor(path: string, value: AuthUser = profile) {
  if (path.endsWith('/auth/me')) return Response.json(value)
  return Response.json({
    items: ['Asia/Shanghai', 'Asia/Tokyo', 'Europe/London'],
  })
}

it('loads the own profile, sends only changed fields, and saves through the existing API', async () => {
  let updated = profile
  const fetch = vi.fn((path: string, options: RequestInit) => {
    if (options.method === 'PATCH') {
      const patch = JSON.parse(String(options.body)) as Partial<AuthUser>
      updated = { ...updated, ...patch }
      return Promise.resolve(
        Response.json({
          user: updated,
          eventId: 'event',
        } satisfies AuthUserMutation),
      )
    }
    return Promise.resolve(responseFor(path, updated))
  })
  vi.stubGlobal('fetch', fetch)
  const wrapper = mount(ProfilePage, {
    global: { plugins: [pinia, i18n], stubs: { RouterLink: true } },
  })
  try {
    await flushPromises()
    expect(wrapper.get('h1').text()).toBe('Profile')
    expect(
      wrapper.get('input[aria-label="Display name"]').element,
    ).toHaveProperty('value', 'Alice')
    await wrapper.get('input[aria-label="Display name"]').setValue('Alice A.')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(fetch).toHaveBeenLastCalledWith(
      '/api/v1/auth/me',
      expect.objectContaining({
        method: 'PATCH',
        headers: {
          'Content-Type': 'application/json',
          'X-PT-CSRF': '1',
          'X-PT-Event-ID': expect.stringMatching(/^[0-9a-f]{32}$/),
        },
        body: JSON.stringify({ displayName: 'Alice A.' }),
      }),
    )
    expect(wrapper.get('[role="status"]').text()).toBe('Profile saved.')
    expect(useAuthStore().user?.displayName).toBe('Alice A.')
  } finally {
    wrapper.unmount()
  }
})

it('saves the opt-in mention email preference without changing other profile fields', async () => {
  const fetch = vi.fn((path: string, options: RequestInit) => {
    if (options.method === 'PATCH')
      return Promise.resolve(
        Response.json({
          user: { ...profile, mentionEmail: true },
          eventId: 'event',
        }),
      )
    return Promise.resolve(responseFor(path))
  })
  vi.stubGlobal('fetch', fetch)
  const wrapper = mount(ProfilePage, {
    global: { plugins: [pinia, i18n], stubs: { RouterLink: true } },
  })
  try {
    await flushPromises()
    await wrapper.get('input[type="checkbox"]').setValue(true)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(fetch).toHaveBeenLastCalledWith(
      '/api/v1/auth/me',
      expect.objectContaining({ body: '{"mentionEmail":true}' }),
    )
    expect(useAuthStore().user?.mentionEmail).toBe(true)
  } finally {
    wrapper.unmount()
  }
})

it('sends null for app default and applies saved locale and theme through shared preferences', async () => {
  let updated = profile
  const fetch = vi.fn((path: string, options: RequestInit) => {
    if (options.method === 'PATCH') {
      const patch = JSON.parse(String(options.body)) as Partial<AuthUser>
      updated = { ...updated, ...patch, effectiveTimezone: 'Asia/Shanghai' }
      return Promise.resolve(
        Response.json({
          user: updated,
          eventId: 'event',
        } satisfies AuthUserMutation),
      )
    }
    return Promise.resolve(responseFor(path, updated))
  })
  vi.stubGlobal('fetch', fetch)
  const wrapper = mount(ProfilePage, {
    global: { plugins: [pinia, i18n], stubs: { RouterLink: true } },
  })
  try {
    await flushPromises()
    await wrapper
      .get('select[aria-label="Time zone"]')
      .setValue('__app_default__')
    await wrapper.get('select[aria-label="Language"]').setValue('zh-CN')
    await wrapper.get('select[aria-label="Theme"]').setValue('dark')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    await nextTick()

    expect(fetch).toHaveBeenLastCalledWith(
      '/api/v1/auth/me',
      expect.objectContaining({
        method: 'PATCH',
        body: JSON.stringify({
          locale: 'zh-CN',
          theme: 'dark',
          timezone: null,
        }),
      }),
    )
    const preferences = usePreferencesStore(pinia)
    expect(preferences.locale).toBe('zh-CN')
    expect(preferences.theme).toBe('dark')
    expect(localStorage.getItem('pandatable.locale')).toBe('zh-CN')
    expect(localStorage.getItem('pandatable.theme')).toBe('dark')
    expect(document.documentElement.lang).toBe('zh-CN')
    expect(document.documentElement.dataset.theme).toBe('dark')
    expect(wrapper.get('[role="status"]').text()).toBe('个人资料已保存。')
  } finally {
    wrapper.unmount()
    i18n.global.locale.value = 'en'
  }
})

it('shows load failures with retry and redirects an expired session', async () => {
  let failInitialLoad = true
  let expired = false
  const fetch = vi.fn((path: string) => {
    if (path.endsWith('/auth/me') && failInitialLoad) {
      failInitialLoad = false
      return Promise.reject(new TypeError('offline'))
    }
    if (path.endsWith('/auth/me') && expired)
      return Promise.resolve(
        Response.json(
          { error: { code: 'ERR_UNAUTHENTICATED' } },
          { status: 401 },
        ),
      )
    return Promise.resolve(responseFor(path))
  })
  vi.stubGlobal('fetch', fetch)
  const wrapper = mount(ProfilePage, {
    global: { plugins: [pinia, i18n], stubs: { RouterLink: true } },
  })
  try {
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('Try again')
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(wrapper.find('form').exists()).toBe(true)

    expired = true
    const reload = wrapper
      .findAll('button')
      .find((button) => button.text() === 'Reload profile')
    await reload?.trigger('click')
    await flushPromises()
    expect(replace).toHaveBeenCalledWith('/login')
  } finally {
    wrapper.unmount()
  }
})
