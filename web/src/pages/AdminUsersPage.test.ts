import { afterEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { i18n, loadLocale } from '../i18n'
import AdminUsersPage from './AdminUsersPage.vue'

const replace = vi.fn()
vi.mock('vue-router', () => ({ useRouter: () => ({ replace }) }))

afterEach(() => {
  vi.unstubAllGlobals()
  replace.mockReset()
})

it('loads pending cleanup on entry and reports failures until retry succeeds', async () => {
  await loadLocale('en')
  i18n.global.locale.value = 'en'
  let pending = 2
  let attempts = 0
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      if (url.includes('/admin/users?'))
        return Response.json({ items: [], total: 0 })
      if (url.endsWith('/admin/user-deletions/cleanup')) {
        if (init?.method === 'POST') {
          attempts++
          if (attempts === 1)
            return Response.json(
              { error: { code: 'ERR_USER_CLEANUP_PENDING' } },
              { status: 503 },
            )
          pending = 0
          return Response.json({ eventId: 'event' })
        }
        return Response.json({ pending })
      }
      throw new Error(`Unexpected request ${url}`)
    }),
  )
  const wrapper = mount(AdminUsersPage, {
    global: { plugins: [createPinia(), i18n], stubs: { RouterLink: true } },
  })
  try {
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain(
      '2 files from deleted users still need cleanup.',
    )
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain(
      'some files could not be removed',
    )
    expect(wrapper.get('[role="alert"]').text()).toContain(
      'Could not complete pending file cleanup.',
    )
    expect(wrapper.text()).not.toContain('Pending file cleanup completed.')
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(wrapper.text()).not.toContain('files from deleted users still need')
    expect(wrapper.text()).toContain('Pending file cleanup completed.')
  } finally {
    wrapper.unmount()
  }
})

it('retries a failed cleanup status check without leaving a stale page error', async () => {
  await loadLocale('en')
  i18n.global.locale.value = 'en'
  let checks = 0
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url.includes('/admin/users?'))
        return Response.json({ items: [], total: 0 })
      if (url.endsWith('/admin/user-deletions/cleanup')) {
        checks++
        return checks === 1
          ? Response.json(
              { error: { code: 'ERR_LIFECYCLE_UNAVAILABLE' } },
              { status: 503 },
            )
          : Response.json({ pending: 0 })
      }
      throw new Error(`Unexpected request ${url}`)
    }),
  )
  const wrapper = mount(AdminUsersPage, {
    global: { plugins: [createPinia(), i18n], stubs: { RouterLink: true } },
  })
  try {
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain(
      'Could not check pending deletion cleanup.',
    )
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(checks).toBe(2)
  } finally {
    wrapper.unmount()
  }
})

it('ignores a cleanup retry response after leaving user management', async () => {
  await loadLocale('en')
  i18n.global.locale.value = 'en'
  let finish!: (response: Response) => void
  let checks = 0
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      if (url.includes('/admin/users?'))
        return Response.json({ items: [], total: 0 })
      if (url.endsWith('/admin/user-deletions/cleanup')) {
        if (init?.method === 'POST')
          return new Promise<Response>((resolve) => (finish = resolve))
        checks++
        return Response.json({ pending: 1 })
      }
      throw new Error(`Unexpected request ${url}`)
    }),
  )
  const wrapper = mount(AdminUsersPage, {
    global: { plugins: [createPinia(), i18n], stubs: { RouterLink: true } },
  })
  await flushPromises()
  await wrapper.get('[role="alert"] button').trigger('click')
  await flushPromises()
  wrapper.unmount()
  finish(Response.json({ eventId: 'event' }))
  await flushPromises()
  expect(checks).toBe(1)
})
