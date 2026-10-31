import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { nextTick } from 'vue'
import type { TrashItem } from '../api/types'
import { i18n, loadLocale } from '../i18n'
import { useAuthStore } from '../stores/auth'
import { useNavStore } from '../stores/nav'
import { navUser } from '../test/navigation'
import TrashPage from './TrashPage.vue'

let pinia: ReturnType<typeof createPinia>

const field: TrashItem = {
  kind: 'field',
  id: 'field-id',
  name: '<Notes>',
  workspaceId: 'workspace-id',
  workspaceName: 'Workspace',
  parentId: 'table-id',
  parentName: 'Table',
  deletedAt: '2026-10-01T10:00:00Z',
  expiresAt: '2026-10-31T10:00:00Z',
}
const table: TrashItem = {
  ...field,
  kind: 'table',
  id: 'table-id',
  name: 'Orders',
  parentId: 'book-id',
  parentName: 'Book',
}

beforeEach(async () => {
  pinia = createPinia()
  setActivePinia(pinia)
  await loadLocale('en')
  await loadLocale('zh-CN')
  i18n.global.locale.value = 'en'
  useAuthStore().setUser(navUser)
})
afterEach(() => {
  disposePinia(pinia)
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  i18n.global.locale.value = 'en'
})

async function setup() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/trash', component: TrashPage },
      { path: '/login', component: { template: '<div />' } },
      { path: '/change-password', component: { template: '<div />' } },
    ],
  })
  await router.push('/trash')
  const nav = useNavStore()
  const refresh = vi.spyOn(nav, 'refresh').mockResolvedValue(true)
  const wrapper = mount(TrashPage, {
    global: { plugins: [pinia, i18n, router] },
  })
  await flushPromises()
  return { wrapper, router, refresh }
}

it('shows localized resource context and expiry, then refreshes authoritative Trash and navigation', async () => {
  let items = [field, table]
  const fetcher = vi.fn((_path: string, options: RequestInit) => {
    if (options.method === 'POST') {
      items = items.filter((item) => item.id !== table.id)
      return Promise.resolve(Response.json({ eventId: 'event' }))
    }
    return Promise.resolve(Response.json({ items }))
  })
  vi.stubGlobal('fetch', fetcher)
  const { wrapper, refresh } = await setup()
  try {
    expect(wrapper.get('h1').text()).toBe('Trash')
    const fieldCard = wrapper.get('[data-trash-id="field-id"]')
    expect(fieldCard.text()).toContain('<Notes>')
    expect(fieldCard.text()).toContain('Workspace: Workspace')
    expect(fieldCard.text()).toContain('Parent: Table')
    expect(fieldCard.find('notes').exists()).toBe(false)
    expect(
      fieldCard.findAll('time').map((time) => time.attributes('datetime')),
    ).toEqual([field.deletedAt, field.expiresAt])
    await wrapper.get('[data-trash-id="table-id"] button').trigger('click')
    await flushPromises()
    expect(fetcher).toHaveBeenCalledWith(
      '/api/v1/trash/table/table-id/restore',
      expect.objectContaining({ method: 'POST', body: '{}' }),
    )
    expect(refresh).toHaveBeenCalledWith(true)
    expect(wrapper.find('[data-trash-id="table-id"]').exists()).toBe(false)
    expect(wrapper.get('[role="status"]').text()).toBe('Restored Orders.')
    i18n.global.locale.value = 'zh-CN'
    await nextTick()
    expect(wrapper.get('h1').text()).toBe('回收站')
    expect(wrapper.get('[data-trash-id="field-id"]').text()).toContain('字段')
    expect(
      wrapper.get('[data-trash-id="field-id"] button').attributes('aria-label'),
    ).toContain('恢复字段')
  } finally {
    wrapper.unmount()
  }
})

it('requires an explicit choice before restoring a required field as optional', async () => {
  let restored = false
  const fetcher = vi.fn((_path: string, options: RequestInit) => {
    if (options.method === 'POST') {
      if (options.body === '{}')
        return Promise.resolve(
          Response.json(
            {
              error: { code: 'ERR_REQUIRED_HAS_EMPTY', details: { count: 3 } },
            },
            { status: 409 },
          ),
        )
      restored = true
      return Promise.resolve(Response.json({ eventId: 'event' }))
    }
    return Promise.resolve(Response.json({ items: restored ? [] : [field] }))
  })
  vi.stubGlobal('fetch', fetcher)
  const { wrapper } = await setup()
  try {
    await wrapper.get('[data-trash-id="field-id"] button').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('3 empty rows')
    expect(
      fetcher.mock.calls.filter(([, options]) => options.method === 'POST'),
    ).toHaveLength(1)
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(fetcher).toHaveBeenCalledWith(
      '/api/v1/trash/field/field-id/restore',
      expect.objectContaining({
        method: 'POST',
        body: '{"allowOptional":true}',
      }),
    )
    expect(wrapper.text()).toContain('There are no items available to restore.')
  } finally {
    wrapper.unmount()
  }
})

it('explains unknown legacy required settings before allowing an optional restore', async () => {
  let restored = false
  const fetcher = vi.fn((_path: string, options: RequestInit) => {
    if (options.method === 'POST') {
      if (options.body === '{}')
        return Promise.resolve(
          Response.json(
            { error: { code: 'ERR_TRASH_REQUIRED_UNKNOWN' } },
            { status: 409 },
          ),
        )
      restored = true
      return Promise.resolve(Response.json({ eventId: 'event' }))
    }
    return Promise.resolve(Response.json({ items: restored ? [] : [field] }))
  })
  vi.stubGlobal('fetch', fetcher)
  const { wrapper } = await setup()
  try {
    await wrapper.get('[data-trash-id="field-id"] button').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain(
      'original required setting was not recorded',
    )
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(fetcher).toHaveBeenCalledWith(
      '/api/v1/trash/field/field-id/restore',
      expect.objectContaining({ body: '{"allowOptional":true}' }),
    )
  } finally {
    wrapper.unmount()
  }
})

it('reports load, dependency and refresh failures without claiming restoration failed or succeeded', async () => {
  let getFails = true
  let postCount = 0
  const fetcher = vi.fn((_path: string, options: RequestInit) => {
    if (options.method === 'POST') {
      postCount++
      return Promise.resolve(
        postCount === 1
          ? Response.json(
              { error: { code: 'ERR_TRASH_DEPENDENCY' } },
              { status: 409 },
            )
          : Response.json({ eventId: 'event' }),
      )
    }
    return getFails
      ? Promise.reject(new TypeError('offline'))
      : Promise.resolve(Response.json({ items: [field] }))
  })
  vi.stubGlobal('fetch', fetcher)
  const { wrapper, refresh } = await setup()
  try {
    expect(wrapper.get('[role="alert"]').text()).toContain('Try again')
    expect(wrapper.text()).not.toContain('offline')
    getFails = false
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    await wrapper.get('[data-trash-id="field-id"] button').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain(
      'needed for restoration',
    )
    refresh.mockResolvedValue(false)
    await wrapper.get('[data-trash-id="field-id"] button').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('item was restored')
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
  } finally {
    wrapper.unmount()
  }
})

it('discards an old account response and redirects an expired session', async () => {
  let release!: (response: Response) => void
  const fetcher = vi
    .fn()
    .mockImplementationOnce(
      () =>
        new Promise<Response>((resolve) => {
          release = resolve
        }),
    )
    .mockResolvedValueOnce(Response.json({ items: [] }))
    .mockResolvedValueOnce(
      Response.json(
        { error: { code: 'ERR_UNAUTHENTICATED' } },
        { status: 401 },
      ),
    )
  vi.stubGlobal('fetch', fetcher)
  const { wrapper, router } = await setup()
  try {
    useAuthStore().setUser({ ...navUser, id: 'bob' })
    await flushPromises()
    release(Response.json({ items: [field] }))
    await flushPromises()
    expect(wrapper.text()).not.toContain('<Notes>')
    expect(wrapper.text()).toContain('There are no items available to restore.')
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/login')
    expect(useAuthStore().user).toBeNull()
  } finally {
    wrapper.unmount()
  }
})
