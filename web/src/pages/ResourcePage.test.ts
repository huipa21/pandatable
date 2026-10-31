import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { i18n, loadLocale } from '../i18n'
import { useNavStore } from '../stores/nav'
import { useAuthStore } from '../stores/auth'
import { useRealtimeStore } from '../stores/realtime'
import { navigation, navUser, tableSchema } from '../test/navigation'
import ResourcePage from './ResourcePage.vue'
import { errorMessage, RequestError } from '../api/client'
import { ThemeMedia } from '../test/setup'

let pinia: ReturnType<typeof createPinia>
let media: ThemeMedia
beforeEach(async () => {
  pinia = createPinia()
  setActivePinia(pinia)
  await loadLocale('en')
  await loadLocale('zh-CN')
  i18n.global.locale.value = 'en'
  media = new ThemeMedia()
  media.matches = true
  vi.stubGlobal('matchMedia', () => media)
})
afterEach(() => {
  disposePinia(pinia)
  vi.unstubAllGlobals()
})
async function setup(path: string) {
  const nav = useNavStore()
  nav.tree = navigation()
  nav.loaded = true
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/home', component: { template: '<div />' } },
      { path: '/w/:workspaceId', component: ResourcePage },
      { path: '/w/:wid/b/:bookId', component: ResourcePage },
      { path: '/t/:tableId/v/:viewId', component: ResourcePage },
      { path: '/t/:tableId/v/:viewId/r/:rowId', component: ResourcePage },
      { path: '/t/:tableId', component: ResourcePage },
    ],
  })
  await router.push(path)
  const wrapper = mount(ResourcePage, {
    global: {
      plugins: [pinia, i18n, router],
      stubs: {
        DataGrid: { template: '<div data-testid="production-grid" />' },
        MobileCardList: {
          template:
            '<div data-testid="mobile-cards"><button @click="$emit(\'add\')">+</button></div>',
        },
        RowExpandDialog: {
          props: ['rowId'],
          template: '<div data-testid="row-dialog">{{ rowId }}</div>',
        },
      },
    },
  })

  await flushPromises()
  return { wrapper, router, nav }
}

it('reports table opening and view changes without changing subscriptions', async () => {
  useAuthStore().setUser(navUser)
  const realtime = useRealtimeStore()
  const setView = vi.spyOn(realtime, 'setView')
  const schema = tableSchema()
  schema.views.push({ ...schema.views[0]!, id: 'v2', name: 'Other' })
  vi.stubGlobal(
    'fetch',
    vi.fn(() => Promise.resolve(Response.json(schema))),
  )
  const { wrapper, router } = await setup('/t/t/v/v')
  try {
    expect(setView).toHaveBeenLastCalledWith('t', 'v')
    await router.push('/t/t/v/v2')
    await flushPromises()
    expect(setView).toHaveBeenLastCalledWith('t', 'v2')
  } finally {
    wrapper.unmount()
    setView.mockRestore()
  }
})

it('switches at 768px without mounting the grid for mobile cards, preserves row links and closes new drafts on navigation', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(Response.json(tableSchema())),
  )
  media.matches = false
  const { wrapper, router } = await setup('/t/t/v/v')
  try {
    expect(wrapper.find('[data-testid="production-grid"]').exists()).toBe(false)
    await wrapper.get('[data-testid="mobile-cards"] button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="row-dialog"]').exists()).toBe(true)
    await router.push('/t/t/v/v/r/9007199254740993')
    await flushPromises()
    expect(wrapper.get('[data-testid="row-dialog"]').text()).toBe(
      '9007199254740993',
    )
    media.setDark(true)
    await flushPromises()
    expect(wrapper.find('[data-testid="mobile-cards"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="production-grid"]').exists()).toBe(true)
  } finally {
    wrapper.unmount()
  }
})
it('opens row deep links without reloading schema or remounting the grid, and rejects invalid views', async () => {
  const fetcher = vi.fn().mockResolvedValue(Response.json(tableSchema()))
  vi.stubGlobal('fetch', fetcher)
  const { wrapper, router } = await setup('/t/t/v/v')
  try {
    const grid = wrapper.get('[data-testid="production-grid"]').element
    await router.push('/t/t/v/v/r/9007199254740993')
    await flushPromises()
    expect(wrapper.get('[data-testid="row-dialog"]').text()).toBe(
      '9007199254740993',
    )
    expect(wrapper.get('[data-testid="production-grid"]').element).toBe(grid)
    expect(fetcher).toHaveBeenCalledTimes(1)
    await router.push('/t/t/v/hidden/r/1')
    await flushPromises()
    expect(wrapper.find('[data-testid="row-dialog"]').exists()).toBe(false)
  } finally {
    wrapper.unmount()
  }
})
it('shows real schema metadata, breadcrumbs, levels, view links and the production grid', async () => {
  const fetch = vi.fn().mockResolvedValue(Response.json(tableSchema()))
  vi.stubGlobal('fetch', fetch)
  const { wrapper } = await setup('/t/t/v/v')
  try {
    expect(wrapper.get('h1').text()).toBe('Table')
    expect(wrapper.get('.schema-summary').text()).toContain('Name')
    expect(wrapper.get('.schema-summary').text()).toContain('Owner')
    expect(
      wrapper.findAll('.breadcrumbs a').map((item) => item.text()),
    ).toEqual(['Your PandaTable', 'Workspace', 'Book', 'Table'])
    expect(wrapper.find('[data-testid="production-grid"]').exists()).toBe(true)
    expect(fetch).toHaveBeenCalledWith(
      '/api/v1/tables/t/schema',
      expect.objectContaining({
        cache: 'no-store',
        signal: expect.any(AbortSignal),
      }),
    )
  } finally {
    wrapper.unmount()
  }
})

it('refreshes unchanged-route schema on nav revision and removes sharing when the server reports non-owner', async () => {
  let owner = true
  const fetch = vi.fn(() =>
    Promise.resolve(
      Response.json({
        ...tableSchema(),
        myLevel: owner ? 'owner' : 'read',
      }),
    ),
  )
  vi.stubGlobal('fetch', fetch)
  const { wrapper, nav } = await setup('/t/t/v/v')
  try {
    expect(
      wrapper.findAll('button').some((button) => button.text() === 'Share'),
    ).toBe(true)
    owner = false
    nav.revision++
    await flushPromises()
    expect(fetch).toHaveBeenCalledTimes(2)
    expect(wrapper.get('.schema-summary').text()).toContain('Read')
    expect(
      wrapper.findAll('button').some((button) => button.text() === 'Share'),
    ).toBe(false)
  } finally {
    wrapper.unmount()
  }
})

it('preserves the open row dialog when navigation reports a permission change', async () => {
  let level: 'owner' | 'read' = 'owner'
  const fetcher = vi.fn(() =>
    Promise.resolve(Response.json({ ...tableSchema(), myLevel: level })),
  )
  vi.stubGlobal('fetch', fetcher)
  const { wrapper, nav } = await setup('/t/t/v/v/r/1')
  try {
    const dialog = wrapper.get('[data-testid="row-dialog"]').element
    level = 'read'
    nav.tree!.workspaces[0]!.books[0]!.tables[0]!.level = level
    await flushPromises()
    expect(fetcher).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-testid="row-dialog"]').element).toBe(dialog)
    expect(wrapper.get('.schema-summary').text()).toContain('Read')
  } finally {
    wrapper.unmount()
  }
})

it('restarts an in-flight schema read when the navigation permission changes', async () => {
  let release!: (response: Response) => void
  const fetcher = vi
    .fn()
    .mockImplementationOnce(
      () =>
        new Promise<Response>((resolve) => {
          release = resolve
        }),
    )
    .mockResolvedValue(Response.json({ ...tableSchema(), myLevel: 'read' }))
  vi.stubGlobal('fetch', fetcher)
  const { wrapper, nav } = await setup('/t/t/v/v')
  try {
    nav.tree!.workspaces[0]!.books[0]!.tables[0]!.level = 'read'
    await flushPromises()
    expect(fetcher).toHaveBeenCalledTimes(2)
    release(Response.json(tableSchema()))
    await flushPromises()
    expect(wrapper.get('.schema-summary').text()).toContain('Read')
  } finally {
    wrapper.unmount()
  }
})

it('replaces a table shortcut with its real Grid id and ignores schema reads from an abandoned route', async () => {
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockImplementation(() => Promise.resolve(Response.json(tableSchema()))),
  )
  const first = await setup('/t/t')
  try {
    expect(first.router.currentRoute.value.path).toBe('/t/t/v/v')
  } finally {
    first.wrapper.unmount()
  }
  let release!: (response: Response) => void
  vi.stubGlobal(
    'fetch',
    vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          release = resolve
        }),
    ),
  )
  const { wrapper, router } = await setup('/t/t/v/v')
  try {
    await router.push('/w/w')
    await flushPromises()
    release(Response.json(tableSchema()))
    await flushPromises()
    expect(wrapper.get('h1').text()).toBe('Workspace')
    expect(wrapper.find('.schema-summary').exists()).toBe(false)
  } finally {
    wrapper.unmount()
  }
})

it('shows a failed initial navigation read as an error, not perpetual loading or an empty success', async () => {
  const { wrapper, nav } = await setup('/w/w')
  try {
    nav.loaded = false
    nav.error = new TypeError('offline')
    await flushPromises()
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
    expect(wrapper.get('[role="alert"]').text()).toContain('Try again')
    expect(wrapper.text()).not.toContain('no visible books')
  } finally {
    wrapper.unmount()
  }
})

it('renders path-only context without controls and does not accept mismatched parents or view ids', async () => {
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockImplementation(() => Promise.resolve(Response.json(tableSchema()))),
  )
  const { wrapper, router, nav } = await setup('/w/w')
  try {
    nav.tree.workspaces[0]!.level = 'none'
    nav.tree.workspaces[0]!.pathOnly = true
    await flushPromises()
    expect(wrapper.text()).toContain('Only accessible descendants')
    expect(wrapper.findAll('button')).toHaveLength(0)
    await router.push('/w/hidden/b/b')
    await flushPromises()
    expect(wrapper.get('h1').text()).toContain('not found or removed')
    await router.push('/t/t/v/hidden')
    await flushPromises()
    expect(wrapper.get('h1').text()).toContain('not found or removed')
    expect(wrapper.find('.schema-summary').exists()).toBe(false)
  } finally {
    wrapper.unmount()
  }
})

it('refetches navigation once on schema denial, keeps failures explicit and translates resource-neutral errors', async () => {
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(
        Response.json({ error: { code: 'ERR_NOT_FOUND' } }, { status: 404 }),
      ),
  )
  const refresh = vi.spyOn(useNavStore(), 'refresh').mockResolvedValue(true)
  const { wrapper } = await setup('/t/t/v/v')
  try {
    expect(refresh).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[role="alert"]').text()).toContain(
      'may have been removed',
    )
    for (const locale of ['en', 'zh-CN'] as const) {
      i18n.global.locale.value = locale
      expect(errorMessage(new RequestError('ERR_NOT_FOUND', 404))).not.toMatch(
        /user or group|用户或用户组/,
      )
      expect(
        errorMessage(new RequestError('ERR_DDL_LOCK_TIMEOUT', 409)),
      ).not.toBe(errorMessage(new Error()))
    }
  } finally {
    wrapper.unmount()
    i18n.global.locale.value = 'en'
  }
})
