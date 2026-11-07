import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { i18n, loadLocale } from '../i18n'
import { useAuthStore } from '../stores/auth'
import { useNavStore } from '../stores/nav'
import { navigation, navUser } from '../test/navigation'
import { ThemeMedia } from '../test/setup'
import ResourcePage from './ResourcePage.vue'

let pinia: ReturnType<typeof createPinia>

beforeEach(async () => {
  pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().setUser(navUser)
  await loadLocale('en')
  await loadLocale('zh-CN')
  i18n.global.locale.value = 'en'
  vi.stubGlobal('matchMedia', () => new ThemeMedia())
})

afterEach(() => {
  disposePinia(pinia)
  vi.unstubAllGlobals()
})

async function setup(path = '/w/w') {
  const nav = useNavStore()
  nav.tree = navigation()
  nav.loaded = true
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/home', component: { template: '<div />' } },
      { path: '/w/:workspaceId', component: ResourcePage },
      { path: '/w/:wid/b/:bookId', component: ResourcePage },
    ],
  })
  await router.push(path)
  const wrapper = mount(ResourcePage, {
    global: { plugins: [pinia, i18n, router] },
  })
  await flushPromises()
  return { wrapper, router, nav }
}

it('renders ordered workbook links with effective icons and switches workspaces without retaining old cards', async () => {
  const { wrapper, router, nav } = await setup()
  try {
    const workspace = nav.tree.workspaces[0]!
    const first = workspace.books[0]!
    first.fallbackIcon = 'view'
    workspace.books.push({
      ...first,
      id: 'second',
      name: 'Second workbook',
      icon: 'star',
      tables: [],
    })
    nav.tree.workspaces.push({
      ...workspace,
      id: 'other',
      name: 'Other workspace',
      books: [
        {
          ...first,
          id: 'other-book',
          name: 'Other workbook',
          fallbackIcon: 'calendar',
          tables: [],
        },
      ],
    })
    await flushPromises()

    let cards = wrapper.findAll('.workbook-card')
    expect(cards.map((card) => card.text())).toEqual([
      'Book',
      'Second workbook',
    ])
    expect(cards.map((card) => card.attributes('href'))).toEqual([
      '/w/w/b/b',
      '/w/w/b/second',
    ])
    expect(cards[0]!.get('svg').attributes('aria-hidden')).toBe('true')
    expect(cards[0]!.get('svg path').attributes('d')).toBe(
      'M4 6h16M7 12h10M10 18h4',
    )
    expect(cards[1]!.get('svg path').attributes('d')).toBe(
      'M12 3l3 6 6 1-4.5 4.5 1 6.5-5.5-3-5.5 3 1-6.5L3 10l6-1z',
    )

    first.icon = 'star'
    await flushPromises()
    expect(
      wrapper.findAll('.workbook-card')[0]!.get('svg path').attributes('d'),
    ).toBe('M12 3l3 6 6 1-4.5 4.5 1 6.5-5.5-3-5.5 3 1-6.5L3 10l6-1z')
    first.icon = null
    await flushPromises()
    expect(
      wrapper.findAll('.workbook-card')[0]!.get('svg path').attributes('d'),
    ).toBe('M4 6h16M7 12h10M10 18h4')

    await router.push('/w/other')
    await flushPromises()
    cards = wrapper.findAll('.workbook-card')
    expect(cards).toHaveLength(1)
    expect(cards[0]!.text()).toBe('Other workbook')
    expect(cards[0]!.attributes('href')).toBe('/w/other/b/other-book')
    expect(wrapper.text()).not.toContain('Second workbook')
    await cards[0]!.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/w/other/b/other-book')
    expect(wrapper.find('.workbook-grid').exists()).toBe(false)
    expect(wrapper.text()).toContain('This book has no visible tables.')
  } finally {
    wrapper.unmount()
  }
})

it('orders workbooks by recency and filters them with a URL-backed search', async () => {
  const { wrapper, router, nav } = await setup('/w/w?q=Book')
  try {
    const workspace = nav.tree.workspaces[0]!
    const book = workspace.books[0]!
    workspace.books.push(
      {
        ...book,
        id: 'planner',
        name: 'Planner',
        lastOpenedAt: '2026-11-06T12:00:00Z',
        tables: [],
      },
      {
        ...book,
        id: 'cjk',
        name: '项目工作簿',
        lastOpenedAt: '2026-11-07T12:00:00Z',
        tables: [],
      },
    )
    nav.tree.workspaces.push({
      ...workspace,
      id: 'other',
      name: 'Other workspace',
      books: [{ ...book, id: 'other-book', name: 'Other book', tables: [] }],
    })
    await flushPromises()

    const search = wrapper.get('input[type="search"]')
    expect((search.element as HTMLInputElement).value).toBe('Book')
    expect(
      wrapper.findAll('.workbook-card').map((card) => card.text()),
    ).toEqual(['Book'])
    expect(router.currentRoute.value.query.q).toBe('Book')

    await search.setValue('项目')
    await flushPromises()
    expect(
      wrapper.findAll('.workbook-card').map((card) => card.text()),
    ).toEqual(['项目工作簿'])
    expect(router.currentRoute.value.query.q).toBe('项目')

    await search.setValue('no matching book')
    await flushPromises()
    expect(wrapper.findAll('.workbook-card')).toHaveLength(0)
    expect(wrapper.find('.workbook-no-matches').exists()).toBe(true)
    await wrapper.get('.resource-search-clear').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query.q).toBeUndefined()
    expect(
      wrapper.findAll('.workbook-card').map((card) => card.text()),
    ).toEqual(['项目工作簿', 'Planner', 'Book'])

    await search.setValue('Planner')
    await flushPromises()
    await router.push('/w/other')
    await flushPromises()
    expect((search.element as HTMLInputElement).value).toBe('')
    expect(router.currentRoute.value.query.q).toBeUndefined()

    await router.back()
    await flushPromises()
    expect(router.currentRoute.value.fullPath).toBe('/w/w?q=Planner')
    expect((search.element as HTMLInputElement).value).toBe('Planner')
  } finally {
    wrapper.unmount()
  }
})

it('keeps first-book creation available only to workspace owners', async () => {
  const { wrapper, nav } = await setup()
  try {
    const workspace = nav.tree.workspaces[0]!
    workspace.books = []
    await flushPromises()
    expect(wrapper.text()).toContain('This workspace has no visible books.')
    expect(wrapper.find('.workbook-grid').exists()).toBe(false)
    const buttons = wrapper.findAll('button')
    expect(buttons.map((button) => button.text())).toEqual(['New book'])

    await buttons[0]!.trigger('click')
    expect(nav.editor).toEqual({
      operation: 'create',
      kind: 'book',
      parentId: 'w',
    })

    workspace.level = 'read'
    await flushPromises()
    expect(wrapper.findAll('button')).toHaveLength(0)
    workspace.level = 'none'
    workspace.pathOnly = true
    await flushPromises()
    expect(wrapper.text()).toContain('Only accessible descendants')
    expect(wrapper.findAll('button')).toHaveLength(0)
  } finally {
    wrapper.unmount()
  }
})

it('omits page-level sharing while retaining workspace and workbook controls', async () => {
  const { wrapper, router, nav } = await setup()
  try {
    const workspace = nav.tree.workspaces[0]!
    const book = workspace.books[0]!
    expect(wrapper.get('h1').text()).toBe(workspace.name)
    expect(
      wrapper.findAll('button').map((button) => button.text()),
    ).not.toContain('Share')
    expect(wrapper.findAll('button').map((button) => button.text())).toContain(
      'New book',
    )

    await router.push(`/w/${workspace.id}/b/${book.id}`)
    await flushPromises()

    expect(wrapper.get('h1').text()).toBe(book.name)
    const buttons = wrapper.findAll('button').map((button) => button.text())
    expect(buttons).not.toContain('Share')
    expect(buttons).toContain('New table')
    expect(buttons).toContain(i18n.global.t('import.newTable'))
  } finally {
    wrapper.unmount()
  }
})

it('labels path-only workbook cards and does not give their book page mutation controls', async () => {
  const { wrapper, router, nav } = await setup()
  try {
    const book = nav.tree.workspaces[0]!.books[0]!
    book.level = 'none'
    book.pathOnly = true
    await flushPromises()
    const card = wrapper.get('.workbook-card')
    expect(card.text()).toContain('Book')
    expect(card.text()).toContain('Path only')
    expect(nav.items[0]!.children[0]).toMatchObject({
      canEdit: false,
      canCreate: false,
    })
    i18n.global.locale.value = 'zh-CN'
    await flushPromises()
    expect(card.text()).toContain('仅路径')
    await card.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/w/w/b/b')
    expect(wrapper.findAll('button')).toHaveLength(0)
    expect(wrapper.text()).toContain('只显示你可访问的下级资源')
  } finally {
    wrapper.unmount()
  }
})

it('distinguishes loading, missing and stale workspaces and retries an authoritative refresh', async () => {
  const { wrapper, router, nav } = await setup()
  try {
    nav.loaded = false
    await flushPromises()
    expect(wrapper.get('[role="status"]').text()).toBe('Loading navigation')
    nav.error = new TypeError('offline')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('Try again')
    expect(wrapper.text()).not.toContain('This workspace has no visible books.')

    nav.loaded = true
    await flushPromises()
    expect(wrapper.get('.workbook-stale').text()).toContain(
      'Previously loaded items may be out of date.',
    )
    expect(wrapper.findAll('.workbook-card')).toHaveLength(1)
    nav.tree.workspaces[0]!.books = []
    await flushPromises()
    expect(wrapper.text()).not.toContain('This workspace has no visible books.')
    await router.push('/w/unknown')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain(
      'Previously loaded items may be out of date.',
    )
    expect(wrapper.text()).not.toContain('not found or removed')

    await router.push('/w/w')
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
    await wrapper.get('.workbook-stale button').trigger('click')
    expect(
      wrapper.get('.workbook-stale button').attributes('disabled'),
    ).toBeDefined()
    expect(wrapper.text()).not.toContain('This workspace has no visible books.')
    release(Response.json(navigation()))
    await flushPromises()
    expect(nav.error).toBeNull()
    expect(wrapper.find('.workbook-stale').exists()).toBe(false)
    expect(wrapper.findAll('.workbook-card')).toHaveLength(1)

    await router.push('/w/unknown')
    await flushPromises()
    expect(wrapper.get('h1').text()).toContain('not found or removed')
    expect(wrapper.find('.workbook-card').exists()).toBe(false)
  } finally {
    wrapper.unmount()
  }
})
