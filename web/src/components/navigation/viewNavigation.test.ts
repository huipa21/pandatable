import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { defineComponent } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import type { Navigation, TableView } from '../../api/types'
import { i18n, loadLocale } from '../../i18n'
import { useAuthStore } from '../../stores/auth'
import { useNavStore } from '../../stores/nav'
import { useViewManagementStore } from '../../stores/viewManagement'
import { navigation, navUser, tableSchema } from '../../test/navigation'
import NavContents from './NavContents.vue'
import ViewManagementDialog from './ViewManagementDialog.vue'

const Harness = defineComponent({
  components: { NavContents, ViewManagementDialog },
  template: '<div><NavContents /><ViewManagementDialog /></div>',
})

let pinia: ReturnType<typeof createPinia>

beforeEach(async () => {
  pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().setUser(navUser)
  await loadLocale('en')
  await loadLocale('zh-CN')
  i18n.global.locale.value = 'en'
  HTMLDialogElement.prototype.showModal = function () {
    this.open = true
  }
  HTMLDialogElement.prototype.close = function () {
    this.open = false
  }
})

afterEach(() => {
  disposePinia(pinia)
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

async function setup(tree: Navigation, path = '/t/t/v/v') {
  const nav = useNavStore()
  nav.tree = tree
  nav.loaded = true
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/w/:workspaceId', component: { template: '<div />' } },
      { path: '/w/:wid/b/:bookId', component: { template: '<div />' } },
      { path: '/t/:tableId/v/:viewId', component: { template: '<div />' } },
      { path: '/t/:tableId', component: { template: '<div />' } },
    ],
  })
  await router.push(path)
  const wrapper = mount(Harness, {
    attachTo: document.body,
    global: { plugins: [pinia, i18n, router] },
  })
  await flushPromises()
  return { wrapper, router, nav }
}

async function openMenu(wrapper: VueWrapper, selector: string) {
  const menu = wrapper.get(selector).element as HTMLDetailsElement
  menu.open = true
  menu.dispatchEvent(new Event('toggle'))
  await flushPromises()
  return menu
}

function secondTable(tree: Navigation, level: 'read' | 'write' | 'owner') {
  const book = tree.workspaces[0]!.books[0]!
  book.level = level === 'owner' ? 'owner' : 'none'
  book.pathOnly = level !== 'owner'
  tree.workspaces[0]!.level = level === 'owner' ? 'owner' : 'none'
  tree.workspaces[0]!.pathOnly = level !== 'owner'
  book.tables[0]!.level = level
  book.tables.push({
    ...book.tables[0]!,
    id: 't2',
    name: 'Other table',
    views: [{ id: 'v2', name: 'Other grid', type: 'grid', scope: 'shared' }],
  })
  const schema = tableSchema()
  schema.table.id = 't2'
  schema.table.name = 'Other table'
  schema.myLevel = level
  schema.views[0]!.id = 'v2'
  schema.views[0]!.name = 'Other grid'
  return schema
}

it.each(['/t/t/v/v', '/w/w/b/b'] as const)(
  'creates a reader personal view from %s, expands it, and restores focus on cancel',
  async (path) => {
    const tree = navigation()
    tree.workspaces.push({
      ...navigation().workspaces[0]!,
      id: 'other',
      name: 'Other workspace',
      books: [],
    })
    const schema = secondTable(tree, 'read')
    const created: TableView = {
      ...schema.views[0]!,
      id: 'new-view',
      name: 'Private form',
      type: 'form',
      scope: 'personal',
      position: 2048,
    }
    const fetcher = vi.fn(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const path = String(input)
        if (path.endsWith('/nav')) return Response.json(tree)
        if (path.endsWith('/tables/t2/schema')) return Response.json(schema)
        if (path.endsWith('/tables/t2/views') && init?.method === 'POST') {
          schema.views.push(created)
          tree.workspaces[0]!.books[0]!.tables[1]!.views.push(created)
          return Response.json(
            { view: created, eventId: 'event' },
            { status: 201 },
          )
        }
        throw new Error(`Unexpected request: ${path}`)
      },
    )
    vi.stubGlobal('fetch', fetcher)
    const { wrapper, router } = await setup(tree, path)
    try {
      if (path === '/w/w/b/b')
        await wrapper.get('[data-nav-id="b"] .book-toggle').trigger('click')
      const selector = '[data-nav-id="t2"] > .node-actions'
      const menu = await openMenu(wrapper, selector)
      const summary = menu.querySelector('summary')!
      summary.focus()
      await wrapper.get(`${selector} button`).trigger('click')
      await flushPromises()
      expect(wrapper.get<HTMLDialogElement>('.view-dialog').element.open).toBe(
        true,
      )
      expect(wrapper.get('.view-dialog input').attributes('aria-label')).toBe(
        'View name',
      )
      expect(document.activeElement).toBe(
        wrapper.get('.view-dialog input').element,
      )
      i18n.global.locale.value = 'zh-CN'
      await flushPromises()
      expect(wrapper.get('.view-dialog h2').text()).toBe('创建视图')
      expect(wrapper.get('.view-dialog input').attributes('aria-label')).toBe(
        '视图名称',
      )
      i18n.global.locale.value = 'en'
      await flushPromises()
      expect(
        wrapper
          .findAll('.view-dialog select')
          .at(0)!
          .find('option[value="shared"]')
          .attributes('disabled'),
      ).toBeDefined()
      expect(
        wrapper
          .findAll('.view-dialog select')
          .at(1)!
          .findAll('option')
          .map((item) => item.attributes('value')),
      ).toEqual(['grid', 'form', 'kanban', 'gallery'])
      await wrapper
        .get('.view-dialog button[type="button"]:last-child')
        .trigger('click')
      await flushPromises()
      expect(document.activeElement).toBe(summary)

      await openMenu(wrapper, selector)
      await wrapper.get(`${selector} button`).trigger('click')
      await flushPromises()
      await wrapper.get('.view-dialog input').setValue('😀'.repeat(100))
      expect(
        wrapper
          .get('.view-dialog button[type="submit"]')
          .attributes('disabled'),
      ).toBeUndefined()
      await wrapper.get('.view-dialog input').setValue('😀'.repeat(101))
      expect(
        wrapper
          .get('.view-dialog button[type="submit"]')
          .attributes('disabled'),
      ).toBeDefined()
      await wrapper.get('.view-dialog input').setValue('bad\u0001name')
      expect(
        wrapper
          .get('.view-dialog button[type="submit"]')
          .attributes('disabled'),
      ).toBeDefined()
      await wrapper.get('.view-dialog input').setValue('Private form')
      await wrapper.findAll('.view-dialog select').at(1)!.setValue('form')
      await wrapper.get('.view-dialog form').trigger('submit')
      await flushPromises()
      expect(router.currentRoute.value.path).toBe('/t/t2/v/new-view')
      expect(
        JSON.parse(
          String(
            fetcher.mock.calls.find(([path]) =>
              String(path).endsWith('/tables/t2/views'),
            )?.[1]?.body,
          ),
        ),
      ).toEqual({ name: 'Private form', type: 'form', scope: 'personal' })
      expect(
        wrapper
          .get('[data-nav-id="t2"] .table-toggle')
          .attributes('aria-expanded'),
      ).toBe('true')
      expect(wrapper.get('[data-view-id="new-view"] a').text()).toContain(
        'Private form',
      )
      expect(
        fetcher.mock.calls.some(([path]) =>
          String(path).endsWith('/tables/t/schema'),
        ),
      ).toBe(false)
    } finally {
      wrapper.unmount()
    }
  },
)

it('aborts an in-flight target schema read when the route changes', async () => {
  const tree = navigation()
  const schema = secondTable(tree, 'owner')
  let release!: (response: Response) => void
  let signal: AbortSignal | undefined
  const fetcher = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    if (String(input).endsWith('/tables/t2/schema')) {
      signal = init?.signal ?? undefined
      return new Promise<Response>((resolve) => {
        release = resolve
      })
    }
    return Promise.resolve(Response.json(tree))
  })
  vi.stubGlobal('fetch', fetcher)
  const { wrapper, router } = await setup(tree)
  try {
    await openMenu(wrapper, '[data-nav-id="t2"] > .node-actions')
    await wrapper
      .findAll('[data-nav-id="t2"] > .node-actions button')
      .find((button) => button.text() === 'Create view')!
      .trigger('click')
    await router.push('/w/w')
    expect(signal?.aborted).toBe(true)
    release(Response.json(schema))
    await flushPromises()
    expect(useViewManagementStore().schema).toBeNull()
    expect(useViewManagementStore().editor).toBeNull()
    expect(router.currentRoute.value.path).toBe('/w/w')
    expect(fetcher.mock.calls.some(([, init]) => init?.method === 'POST')).toBe(
      false,
    )
  } finally {
    wrapper.unmount()
  }
})

it.each(['identity', 'route'] as const)(
  'ignores a late creation response after a %s change',
  async (change) => {
    const tree = navigation()
    const schema = secondTable(tree, 'owner')
    let release!: (response: Response) => void
    let signal: AbortSignal | undefined
    const fetcher = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input)
      if (path.endsWith('/tables/t2/schema'))
        return Promise.resolve(Response.json(schema))
      if (path.endsWith('/tables/t2/views')) {
        signal = init?.signal ?? undefined
        return new Promise<Response>((resolve) => {
          release = resolve
        })
      }
      return Promise.resolve(Response.json(tree))
    })
    vi.stubGlobal('fetch', fetcher)
    const { wrapper, router, nav } = await setup(tree)
    const refresh = vi.spyOn(nav, 'refresh')
    try {
      await openMenu(wrapper, '[data-nav-id="t2"] > .node-actions')
      await wrapper
        .findAll('[data-nav-id="t2"] > .node-actions button')
        .find((button) => button.text() === 'Create view')!
        .trigger('click')
      await flushPromises()
      await wrapper.get('.view-dialog input').setValue('Pending')
      await wrapper.get('.view-dialog form').trigger('submit')
      expect(signal).toBeDefined()
      if (change === 'identity')
        useAuthStore().setUser({ ...navUser, id: 'bob' })
      else await router.push('/w/w')
      expect(signal?.aborted).toBe(true)
      release(
        Response.json({
          view: { ...schema.views[0], id: 'late-view', name: 'Pending' },
          eventId: 'event',
        }),
      )
      await flushPromises()
      expect(router.currentRoute.value.path).toBe(
        change === 'identity' ? '/t/t/v/v' : '/w/w',
      )
      expect(useViewManagementStore().schema).toBeNull()
      expect(refresh).not.toHaveBeenCalled()
    } finally {
      wrapper.unmount()
    }
  },
)

it.each(['read', 'write'] as const)(
  'allows a %s user to duplicate a locked shared view only as personal',
  async (level) => {
    const tree = navigation()
    const schema = secondTable(tree, level)
    schema.views[0]!.locked = true
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => Response.json(schema)),
    )
    const { wrapper } = await setup(tree, '/t/t2/v/v2')
    try {
      await openMenu(wrapper, '[data-view-id="v2"] > .node-actions')
      const menu = wrapper.get('[data-view-id="v2"] > .node-actions')
      expect(menu.text()).toContain('Duplicate as personal')
      expect(menu.text()).not.toContain('Duplicate as shared')
      expect(menu.text()).not.toContain('Rename view')
      expect(menu.text()).not.toContain('Delete view')
      expect(menu.text()).not.toContain('Unlock shared view')
    } finally {
      wrapper.unmount()
    }
  },
)

it('reloads a conflicted view and retains the rename draft for an explicit retry', async () => {
  const tree = navigation()
  const schema = tableSchema()
  const versions: string[] = []
  let conflict = true
  const fetcher = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input)
      if (path.endsWith('/nav')) return Response.json(tree)
      if (path.endsWith('/tables/t/schema')) return Response.json(schema)
      if (path.endsWith('/views/v') && init?.method === 'PATCH') {
        versions.push((init.headers as Record<string, string>)['If-Match']!)
        if (conflict) {
          conflict = false
          schema.views[0]!.version = 2
          return Response.json(
            { error: { code: 'ERR_VERSION_CONFLICT' } },
            { status: 409 },
          )
        }
        schema.views[0]!.name = 'My rename'
        schema.views[0]!.version = 3
        tree.workspaces[0]!.books[0]!.tables[0]!.views[0]!.name = 'My rename'
        return Response.json({ view: schema.views[0], eventId: 'event' })
      }
      throw new Error(`Unexpected request: ${path}`)
    },
  )
  vi.stubGlobal('fetch', fetcher)
  const { wrapper } = await setup(tree)
  try {
    await openMenu(wrapper, '[data-view-id="v"] > .node-actions')
    await wrapper
      .findAll('[data-view-id="v"] button')
      .find((button) => button.text() === 'Rename view')!
      .trigger('click')
    await flushPromises()
    await wrapper.get('.view-dialog input').setValue('My rename')
    await wrapper.get('.view-dialog form').trigger('submit')
    await flushPromises()
    expect(wrapper.get('.view-dialog [role="alert"]').text()).toBe(
      i18n.global.t('errors.ERR_VERSION_CONFLICT'),
    )
    expect(
      (wrapper.get('.view-dialog input').element as HTMLInputElement).value,
    ).toBe('My rename')
    await wrapper.get('.view-dialog form').trigger('submit')
    await flushPromises()
    expect(versions).toEqual(['1', '2'])
    expect(wrapper.get('[data-view-id="v"] a').text()).toContain('My rename')
    expect(wrapper.get<HTMLDialogElement>('.view-dialog').element.open).toBe(
      false,
    )
  } finally {
    wrapper.unmount()
  }
})
