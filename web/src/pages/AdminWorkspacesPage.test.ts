import { afterEach, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { i18n, loadLocale } from '../i18n'
import AdminWorkspacesPage from './AdminWorkspacesPage.vue'

const replace = vi.fn()
vi.mock('vue-router', () => ({ useRouter: () => ({ replace }) }))
afterEach(() => {
  vi.unstubAllGlobals()
  replace.mockReset()
})

it('renders owner/orphan/size metadata, paging/search/empty and both locales', async () => {
  await loadLocale('en')
  await loadLocale('zh-CN')
  i18n.global.locale.value = 'en'
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(
      Response.json({
        items: [
          {
            id: 'workspace',
            name: '<Workspace>',
            orphaned: true,
            attachmentBytes: 4113,
            ownerTotal: 52,
            owners: [
              {
                id: 'u',
                type: 'user',
                name: 'Alice',
                username: 'alice',
                status: 'disabled',
              },
              {
                id: 'g',
                type: 'group',
                name: 'Group',
                username: null,
                status: null,
              },
            ],
          },
        ],
        total: 60,
      }),
    )
    .mockImplementation(() =>
      Promise.resolve(Response.json({ items: [], total: 0 })),
    )
  vi.stubGlobal('fetch', fetch)
  const wrapper = mount(AdminWorkspacesPage, {
    global: { plugins: [createPinia(), i18n] },
  })

  try {
    await flushPromises()
    expect(wrapper.text()).toContain('<Workspace>')
    expect(wrapper.text()).toContain('4,113 bytes')
    expect(wrapper.text()).toContain('Disabled')
    expect(wrapper.text()).toContain('does not prevent orphan')
    expect(wrapper.text()).toContain('50 more owners')
    expect(wrapper.findAll('a')).toHaveLength(0)
    await wrapper.get('.pagination button:last-child').trigger('click')
    await flushPromises()
    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining('offset=50'),
      expect.anything(),
    )
    expect(wrapper.text()).toContain('No live workspaces')
    await wrapper.get('input').setValue('%_')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(fetch).toHaveBeenLastCalledWith(
      expect.stringContaining('q=%25_&offset=0'),
      expect.anything(),
    )
    i18n.global.locale.value = 'zh-CN'
    await flushPromises()
    expect(wrapper.get('h1').text()).toBe('工作区')
    expect(wrapper.text()).toContain('不会授予数据访问权限')
  } finally {
    wrapper.unmount()
    i18n.global.locale.value = 'en'
  }
})

it('rescues an orphan only after selecting an active user and reloads the inventory', async () => {
  await loadLocale('en')
  i18n.global.locale.value = 'en'
  let rescued = false
  const fetch = vi.fn(async (url: string, init?: RequestInit) => {
    if (url.includes('/admin/workspaces/workspace/rescue')) {
      expect(init?.method).toBe('POST')
      expect(JSON.parse(String(init?.body))).toEqual({ userId: 'bob' })
      rescued = true
      return Response.json({ eventId: 'event' })
    }
    if (url.includes('/admin/users?'))
      return Response.json({
        items: [
          {
            id: 'bob',
            username: 'bob',
            displayName: 'Bob',
            status: 'active',
          },
        ],
        total: 1,
      })
    if (url.includes('/admin/workspaces?'))
      return Response.json({
        items: [
          {
            id: 'workspace',
            name: 'W',
            orphaned: !rescued,
            attachmentBytes: 0,
            ownerTotal: 0,
            owners: [],
          },
        ],
        total: 1,
      })
    throw new Error(`Unexpected request ${url}`)
  })
  vi.stubGlobal('fetch', fetch)
  const wrapper = mount(AdminWorkspacesPage, {
    global: { plugins: [createPinia(), i18n] },
  })
  try {
    await flushPromises()
    await wrapper
      .findAll('button')
      .find((item) => item.text() === 'Assign owner')!
      .trigger('click')
    await flushPromises()
    const confirm = wrapper
      .findAll('button')
      .find((item) => item.text() === 'Assign owner')!
    expect(confirm.attributes('disabled')).toBeDefined()
    await wrapper.get('[aria-label="Select an active user"]').setValue('bob')
    await confirm.trigger('click')
    await flushPromises()
    expect(rescued).toBe(true)
    expect(wrapper.text()).toContain('An active owner has been assigned.')
    expect(wrapper.text()).toContain('Has an active direct user owner')
  } finally {
    wrapper.unmount()
  }
})

it('redirects when the rescue picker loses administrator authorization', async () => {
  await loadLocale('en')
  i18n.global.locale.value = 'en'
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url.includes('/admin/users?'))
        return Response.json(
          { error: { code: 'ERR_PERMISSION_DENIED' } },
          { status: 403 },
        )
      if (url.includes('/admin/workspaces?'))
        return Response.json({
          items: [
            {
              id: 'workspace',
              name: 'W',
              orphaned: true,
              attachmentBytes: 0,
              ownerTotal: 0,
              owners: [],
            },
          ],
          total: 1,
        })
      throw new Error(`Unexpected request ${url}`)
    }),
  )
  const wrapper = mount(AdminWorkspacesPage, {
    global: { plugins: [createPinia(), i18n] },
  })
  try {
    await flushPromises()
    await wrapper
      .findAll('button')
      .find((item) => item.text() === 'Assign owner')!
      .trigger('click')
    await flushPromises()
    expect(replace).toHaveBeenCalledWith('/home')
  } finally {
    wrapper.unmount()
  }
})

it('reconciles an orphan changed concurrently after a failed rescue', async () => {
  await loadLocale('en')
  i18n.global.locale.value = 'en'
  let orphaned = true
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url.includes('/rescue')) {
        orphaned = false
        return Response.json(
          { error: { code: 'ERR_WORKSPACE_NOT_ORPHANED' } },
          { status: 409 },
        )
      }
      if (url.includes('/admin/users?'))
        return Response.json({
          items: [{ id: 'bob', username: 'bob', displayName: 'Bob' }],
          total: 1,
        })
      if (url.includes('/admin/workspaces?'))
        return Response.json({
          items: [
            {
              id: 'workspace',
              name: 'W',
              orphaned,
              attachmentBytes: 0,
              ownerTotal: 0,
              owners: [],
            },
          ],
          total: 1,
        })
      throw new Error(`Unexpected request ${url}`)
    }),
  )
  const wrapper = mount(AdminWorkspacesPage, {
    global: { plugins: [createPinia(), i18n] },
  })
  try {
    await flushPromises()
    await wrapper
      .findAll('button')
      .find((item) => item.text() === 'Assign owner')!
      .trigger('click')
    await flushPromises()
    await wrapper.get('[aria-label="Select an active user"]').setValue('bob')
    await wrapper
      .findAll('button')
      .find((item) => item.text() === 'Assign owner')!
      .trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain(
      'already has an active direct owner',
    )
    expect(wrapper.text()).not.toContain('An active owner has been assigned.')
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('Has an active direct user owner')
    expect(wrapper.find('[aria-label="Select an active user"]').exists()).toBe(
      false,
    )
  } finally {
    wrapper.unmount()
  }
})

it('ignores a rescue response after leaving the workspace inventory', async () => {
  await loadLocale('en')
  i18n.global.locale.value = 'en'
  let finish!: (response: Response) => void
  let inventoryLoads = 0
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url.includes('/rescue'))
        return new Promise<Response>((resolve) => (finish = resolve))
      if (url.includes('/admin/users?'))
        return Response.json({
          items: [{ id: 'bob', username: 'bob', displayName: 'Bob' }],
          total: 1,
        })
      if (url.includes('/admin/workspaces?')) {
        inventoryLoads++
        return Response.json({
          items: [
            {
              id: 'workspace',
              name: 'W',
              orphaned: true,
              attachmentBytes: 0,
              ownerTotal: 0,
              owners: [],
            },
          ],
          total: 1,
        })
      }
      throw new Error(`Unexpected request ${url}`)
    }),
  )
  const wrapper = mount(AdminWorkspacesPage, {
    global: { plugins: [createPinia(), i18n] },
  })
  await flushPromises()
  await wrapper
    .findAll('button')
    .find((item) => item.text() === 'Assign owner')!
    .trigger('click')
  await flushPromises()
  await wrapper.get('[aria-label="Select an active user"]').setValue('bob')
  await wrapper
    .findAll('button')
    .find((item) => item.text() === 'Assign owner')!
    .trigger('click')
  await flushPromises()
  wrapper.unmount()
  finish(Response.json({ eventId: 'event' }))
  await flushPromises()
  expect(inventoryLoads).toBe(1)
  expect(replace).not.toHaveBeenCalled()
})

it('shows retry on error and rejects late responses after unmount', async () => {
  await loadLocale('en')
  i18n.global.locale.value = 'en'
  let finish!: (response: Response) => void
  const fetch = vi
    .fn()
    .mockRejectedValueOnce(new TypeError('offline'))
    .mockImplementationOnce(
      () =>
        new Promise<Response>((resolve) => {
          finish = resolve
        }),
    )
  vi.stubGlobal('fetch', fetch)
  const wrapper = mount(AdminWorkspacesPage, {
    global: { plugins: [createPinia(), i18n] },
  })
  await flushPromises()
  expect(wrapper.get('[role="alert"]').text()).toContain('Try again')
  await wrapper.get('[role="alert"] button').trigger('click')
  const options = fetch.mock.calls[1]![1] as RequestInit
  wrapper.unmount()
  expect(options.signal?.aborted).toBe(true)
  finish(Response.json({ items: [], total: 0 }))
  await flushPromises()
  expect(replace).not.toHaveBeenCalled()
})
