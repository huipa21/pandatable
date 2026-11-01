import { afterEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { i18n, loadLocale } from '../../i18n'
import type { AdminUser } from '../../api/types'
import UserLifecyclePanel from './UserLifecyclePanel.vue'

const user: AdminUser = {
  id: 'alice-id',
  username: 'alice',
  displayName: 'Alice',
  email: null,
  role: 'user',
  status: 'disabled',
  mustChangePassword: false,
  lastLoginAt: null,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}
const bob = {
  ...user,
  id: 'bob-id',
  username: 'bob',
  displayName: 'Bob',
  status: 'active',
}

afterEach(() => {
  vi.unstubAllGlobals()
  i18n.global.locale.value = 'en'
})

async function setup(fetch: typeof globalThis.fetch) {
  await loadLocale('en')
  await loadLocale('zh-CN')
  i18n.global.locale.value = 'en'
  vi.stubGlobal('fetch', fetch)
  const wrapper = mount(UserLifecyclePanel, {
    props: { user },
    global: { plugins: [i18n], stubs: { RouterLink: true } },
  })
  await flushPromises()
  return wrapper
}

function button(wrapper: ReturnType<typeof mount>, text: string) {
  const found = wrapper.findAll('button').find((item) => item.text() === text)
  if (!found) throw new Error(`Missing button ${text}`)
  return found
}

it('lists all direct scopes, transfers to an active user, and refreshes the authoritative list', async () => {
  let transferred = false
  const fetch = vi.fn(async (url: string, init?: RequestInit) => {
    if (url.includes('/ownerships/transfer')) {
      expect(init?.method).toBe('POST')
      expect(JSON.parse(String(init?.body))).toEqual({ userId: 'bob-id' })
      transferred = true
      return Response.json({ transferred: 2, eventId: 'event' })
    }
    if (url.includes('/ownerships?'))
      return Response.json({
        items: transferred
          ? []
          : [
              {
                resourceType: 'workspace',
                resourceId: 'w',
                name: 'Workspace W',
                workspaceId: 'w',
                workspaceName: 'Workspace W',
                trashed: false,
              },
              {
                resourceType: 'book',
                resourceId: 'b',
                name: 'Old book',
                workspaceId: 'w',
                workspaceName: 'Workspace W',
                trashed: true,
              },
            ],
        total: transferred ? 0 : 2,
      })
    if (url.includes('/admin/users?'))
      return Response.json({ items: [bob], total: 1 })
    throw new Error(`Unexpected request ${url}`)
  })
  const wrapper = await setup(fetch as typeof globalThis.fetch)
  try {
    expect(wrapper.text()).toContain('Workspace W')
    expect(wrapper.text()).toContain('Old book')
    expect(wrapper.text()).toContain('In Trash')
    await button(wrapper, 'Transfer all ownerships').trigger('click')
    await flushPromises()
    await wrapper.get('select').setValue('bob-id')
    await button(wrapper, 'Transfer all ownerships').trigger('click')
    await flushPromises()
    expect(transferred).toBe(true)
    expect(wrapper.text()).toContain('Ownerships transferred.')
    expect(wrapper.text()).toContain('No direct ownerships.')
    i18n.global.locale.value = 'zh-CN'
    await wrapper.vm.$nextTick()
    expect(wrapper.text()).toContain('所有权已转移。')
  } finally {
    wrapper.unmount()
  }
})

it('requires the exact username and exposes failed deletion cleanup with a retry', async () => {
  const fetch = vi.fn(async (url: string, init?: RequestInit) => {
    if (url.includes('/ownerships?'))
      return Response.json({ items: [], total: 0 })
    if (url.endsWith('/admin/users/alice-id')) {
      expect(init?.method).toBe('DELETE')
      expect(JSON.parse(String(init?.body))).toEqual({ username: 'alice' })
      return Response.json(
        { error: { code: 'ERR_USER_CLEANUP_PENDING' } },
        { status: 503 },
      )
    }
    if (url.endsWith('/admin/user-deletions/cleanup')) {
      expect(init?.method).toBe('POST')
      return Response.json({ eventId: 'event' })
    }
    throw new Error(`Unexpected request ${url}`)
  })
  const wrapper = await setup(fetch as typeof globalThis.fetch)
  try {
    await button(wrapper, 'Hard-delete user').trigger('click')
    const submit = button(wrapper, 'Permanently delete user')
    expect(submit.attributes('disabled')).toBeDefined()
    await wrapper.get('input').setValue('Alice')
    expect(submit.attributes('disabled')).toBeDefined()
    await wrapper.get('input').setValue('alice')
    expect(submit.attributes('disabled')).toBeUndefined()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain(
      'some files could not be removed',
    )
    expect(wrapper.emitted('deleted')).toBeUndefined()
    await button(wrapper, 'Retry file cleanup').trigger('click')
    await flushPromises()
    expect(wrapper.emitted('deleted')).toHaveLength(1)
  } finally {
    wrapper.unmount()
  }
})

it('reconciles ownerships after an uncertain transfer without displaying success', async () => {
  let transferred = false
  const fetch = vi.fn(async (url: string) => {
    if (url.includes('/ownerships/transfer')) {
      transferred = true
      throw new TypeError('response lost')
    }
    if (url.includes('/ownerships?'))
      return Response.json({
        items: transferred
          ? []
          : [
              {
                resourceType: 'workspace',
                resourceId: 'w',
                name: 'Workspace W',
                workspaceId: 'w',
                workspaceName: 'Workspace W',
                trashed: false,
              },
            ],
        total: transferred ? 0 : 1,
      })
    if (url.includes('/admin/users?'))
      return Response.json({ items: [bob], total: 1 })
    throw new Error(`Unexpected request ${url}`)
  })
  const wrapper = await setup(fetch as typeof globalThis.fetch)
  try {
    await button(wrapper, 'Transfer all ownerships').trigger('click')
    await flushPromises()
    await wrapper.get('select').setValue('bob-id')
    await button(wrapper, 'Transfer all ownerships').trigger('click')
    await flushPromises()
    expect(transferred).toBe(true)
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('Ownerships transferred.')
    await button(wrapper, 'Try again').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('No direct ownerships.')
    expect(wrapper.find('select').exists()).toBe(false)
  } finally {
    wrapper.unmount()
  }
})

it('ignores a transfer response after the user detail is closed', async () => {
  let finish!: (response: Response) => void
  let ownershipLoads = 0
  const fetch = vi.fn(async (url: string) => {
    if (url.includes('/ownerships/transfer'))
      return new Promise<Response>((resolve) => (finish = resolve))
    if (url.includes('/ownerships?')) {
      ownershipLoads++
      return Response.json({
        items: [
          {
            resourceType: 'workspace',
            resourceId: 'w',
            name: 'Workspace W',
            workspaceId: 'w',
            workspaceName: 'Workspace W',
            trashed: false,
          },
        ],
        total: 1,
      })
    }
    if (url.includes('/admin/users?'))
      return Response.json({ items: [bob], total: 1 })
    throw new Error(`Unexpected request ${url}`)
  })
  const wrapper = await setup(fetch as typeof globalThis.fetch)
  await button(wrapper, 'Transfer all ownerships').trigger('click')
  await flushPromises()
  await wrapper.get('select').setValue('bob-id')
  await button(wrapper, 'Transfer all ownerships').trigger('click')
  await flushPromises()
  wrapper.unmount()
  finish(Response.json({ transferred: 1, eventId: 'event' }))
  await flushPromises()
  expect(ownershipLoads).toBe(1)
})

it('does not navigate from a late hard-deletion response', async () => {
  let finish!: (response: Response) => void
  const fetch = vi.fn(async (url: string) => {
    if (url.includes('/ownerships?'))
      return Response.json({ items: [], total: 0 })
    if (url.endsWith('/admin/users/alice-id'))
      return new Promise<Response>((resolve) => (finish = resolve))
    throw new Error(`Unexpected request ${url}`)
  })
  const wrapper = await setup(fetch as typeof globalThis.fetch)
  await button(wrapper, 'Hard-delete user').trigger('click')
  await wrapper.get('input').setValue('alice')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  wrapper.unmount()
  finish(Response.json({ eventId: 'event' }))
  await flushPromises()
  expect(wrapper.emitted('deleted')).toBeUndefined()
})
