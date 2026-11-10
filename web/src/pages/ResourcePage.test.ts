import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { defineAsyncComponent, defineComponent, type Component } from 'vue'
import { i18n, loadLocale } from '../i18n'
import { useNavStore } from '../stores/nav'
import { useAuthStore } from '../stores/auth'
import { useRealtimeStore } from '../stores/realtime'
import { useSharingStore } from '../stores/sharing'
import { useUndoStore } from '../stores/undo'
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
async function setup(
  path: string,
  extraStubs: Record<string, Component> = {},
  attachTo?: HTMLElement,
) {
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
    attachTo,
    global: {
      plugins: [pinia, i18n, router],
      stubs: {
        DataGrid: {
          props: ['schema', 'fieldsOpen', 'filterOpen', 'filterCount'],
          emits: ['toggleFields', 'toggleFilter', 'createField'],
          methods: {
            exportQuery: () => ({
              fieldIds: ['name'],
              sorts: [{ fieldId: 'name', direction: 'desc' }],
              groupBy: [],
              search: 'active',
            }),
            toggleField: vi.fn(),
          },
          template: `
            <div class="grid-toolbar" data-testid="production-grid" :data-filter-count="filterCount" :data-schema-version="schema.schemaVersion">
              <slot name="toolbar-leading" />
              <button class="grid-fields-trigger" @click="$emit('toggleFields')">Fields</button>
              <button class="grid-filter-trigger" @click="$emit('toggleFilter')">Filter</button>
              <button class="grid-create-field" @click="$emit('createField')">Add field</button>
              <details class="grid-highlights"><summary>Colour</summary></details>
              <slot name="after-highlight" />
              <details class="grid-format"><summary>Format</summary></details>
              <div class="grid-toolbar-end">
                <slot name="toolbar-actions" />
                <details class="grid-search"><summary>Search</summary></details>
                <slot name="toolbar-end" />
              </div>
            </div>`,
        },
        MobileCardList: {
          methods: {
            exportQuery: () => ({ fieldIds: ['mobile-field'], groupBy: [] }),
          },
          template:
            '<div data-testid="mobile-cards"><button @click="$emit(\'add\')">+</button></div>',
        },
        RowExpandDialog: {
          props: ['rowId', 'focusComment'],
          template:
            '<div data-testid="row-dialog" :data-focus-comment="focusComment">{{ rowId }}</div>',
        },
        ...extraStubs,
      },
    },
  })

  await flushPromises()
  return { wrapper, router, nav }
}

it('exports the active desktop view as an XLSX package using a native link', async () => {
  useAuthStore().setUser(navUser)
  const schema = tableSchema()
  schema.views[0]!.config.filter = {
    field: 'name',
    cmp: 'contains',
    value: 'active',
  }
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) =>
    String(input).endsWith('/export') && init?.method === 'POST'
      ? Response.json({ ticket: 'A'.repeat(26) }, { status: 201 })
      : Response.json(schema),
  )
  vi.stubGlobal('fetch', fetcher)
  const links: string[] = []
  const click = vi
    .spyOn(HTMLAnchorElement.prototype, 'click')
    .mockImplementation(function (this: HTMLAnchorElement) {
      links.push(this.getAttribute('href') ?? '')
    })
  const { wrapper } = await setup('/t/t/v/v')
  try {
    expect(wrapper.find('.view-create').exists()).toBe(false)
    expect(wrapper.find('.view-links').exists()).toBe(false)
    expect(wrapper.find('.view-export p').exists()).toBe(false)
    await wrapper
      .findAll('.view-export button')
      .find(
        (button) => button.text() === i18n.global.t('gridPolish.gp06.export'),
      )!
      .trigger('click')
    await flushPromises()
    const packageRequest = fetcher.mock.calls.find(([path]) =>
      String(path).endsWith('/export'),
    )?.[1]
    expect(JSON.parse(String(packageRequest?.body))).toEqual({ format: 'xlsx' })
    expect(links).toEqual([`/api/v1/exports/${'A'.repeat(26)}`])
    i18n.global.locale.value = 'zh-CN'
    await flushPromises()
    expect(wrapper.get('.view-export summary').attributes('aria-label')).toBe(
      i18n.global.t('gridPolish.gp06.trigger'),
    )
  } finally {
    wrapper.unmount()
    click.mockRestore()
  }
})

it('reports export failures and cancels stale navigation requests on mobile', async () => {
  useAuthStore().setUser(navUser)
  media.matches = false
  const schema = tableSchema()
  schema.views.push({ ...schema.views[0]!, id: 'v2', name: 'Other' })
  let release!: (response: Response) => void
  let mode: 'failure' | 'pending' = 'failure'
  let signal: AbortSignal | undefined
  const fetcher = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    if (!String(input).endsWith('/export'))
      return Promise.resolve(Response.json(schema))
    signal = init?.signal ?? undefined
    if (mode === 'failure')
      return Promise.resolve(
        Response.json(
          { error: { code: 'ERR_VALIDATION', message: 'invalid' } },
          { status: 400 },
        ),
      )
    return new Promise<Response>((resolve) => {
      release = resolve
    })
  })
  vi.stubGlobal('fetch', fetcher)
  const click = vi
    .spyOn(HTMLAnchorElement.prototype, 'click')
    .mockImplementation(() => {})
  const { wrapper, router } = await setup('/t/t/v/v')
  const exportButton = () =>
    wrapper
      .findAll('.view-export button')
      .find(
        (button) => button.text() === i18n.global.t('gridPolish.gp06.export'),
      )!
  try {
    await exportButton().trigger('click')
    await flushPromises()
    const request = fetcher.mock.calls.find(([path]) =>
      String(path).endsWith('/export'),
    )?.[1]
    expect(JSON.parse(String(request?.body))).toEqual({ format: 'xlsx' })
    expect(
      wrapper.get('.table-export-feedback[role="alert"]').text(),
    ).toContain('Export could not be started.')
    mode = 'pending'
    await exportButton().trigger('click')
    expect(wrapper.find('.table-export-feedback[role="status"]').exists()).toBe(
      true,
    )
    await router.push('/t/t/v/v2')
    await flushPromises()
    expect(signal?.aborted).toBe(true)
    release(Response.json({ ticket: 'A'.repeat(26) }, { status: 201 }))
    await flushPromises()
    expect(click).not.toHaveBeenCalled()
    expect(wrapper.find('.table-export-feedback[role="alert"]').exists()).toBe(
      false,
    )
  } finally {
    wrapper.unmount()
    click.mockRestore()
  }
})

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
  useAuthStore().setUser(navUser)
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(Response.json(tableSchema())),
  )
  media.matches = false
  const { wrapper, router } = await setup('/t/t/v/v')
  try {
    expect(wrapper.find('[data-testid="production-grid"]').exists()).toBe(false)
    expect(wrapper.find('.new-entry').exists()).toBe(false)
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

it('opens a new row from the rightmost grid action, preserves the route on cancel, and refreshes after creation', async () => {
  useAuthStore().setUser(navUser)
  const fetcher = vi.fn(() => Promise.resolve(Response.json(tableSchema())))
  const refresh = vi.fn(async () => {})
  vi.stubGlobal('fetch', fetcher)
  const gridStub = {
    methods: {
      refresh,
      exportQuery: () => ({ fieldIds: ['name'], groupBy: [] }),
    },
    template: `
      <div class="grid-toolbar">
        <div class="grid-toolbar-end">
          <slot name="toolbar-actions" />
          <slot name="toolbar-end" />
        </div>
      </div>`,
  }
  const dialogStub = {
    props: { rowId: { type: String, default: null } },
    emits: ['close', 'created'],
    template: `
      <div data-testid="new-row-dialog" :data-row-id="rowId">
        <button data-testid="close-row" @click="$emit('close')">Close</button>
        <button data-testid="create-row" @click="$emit('created', '47')">Create</button>
      </div>`,
  }
  const { wrapper, router } = await setup(
    '/t/t?draft=keep',
    { DataGrid: gridStub, RowExpandDialog: dialogStub },
    document.body,
  )
  try {
    const grid = wrapper.get('.grid-toolbar').element
    const toolbarEnd = wrapper.get('.grid-toolbar-end')
    const action = toolbarEnd.get('.new-entry')
    expect(action.text()).toBe('New')
    expect(toolbarEnd.element.lastElementChild).toBe(action.element)
    await action.trigger('click')
    await flushPromises()
    expect(
      wrapper.get('[data-testid="new-row-dialog"]').attributes('data-row-id'),
    ).toBeUndefined()
    expect(fetcher).toHaveBeenCalledTimes(1)
    expect(router.currentRoute.value.fullPath).toBe('/t/t/v/v?draft=keep')
    await wrapper.get('[data-testid="close-row"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="new-row-dialog"]').exists()).toBe(false)
    expect(router.currentRoute.value.fullPath).toBe('/t/t/v/v?draft=keep')
    expect(fetcher).toHaveBeenCalledTimes(1)

    await action.trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="create-row"]').trigger('click')
    await flushPromises()
    expect(refresh).toHaveBeenCalledTimes(1)
    expect(router.currentRoute.value.fullPath).toBe('/t/t/v/v/r/47')
    expect(fetcher).toHaveBeenCalledTimes(1)
    expect(wrapper.get('.grid-toolbar').element).toBe(grid)
    expect(
      wrapper.get('[data-testid="new-row-dialog"]').attributes('data-row-id'),
    ).toBe('47')
    await wrapper.get('[data-testid="close-row"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.fullPath).toBe('/t/t/v/v')
    await router.push('/t/t?another=1')
    await flushPromises()
    expect(router.currentRoute.value.fullPath).toBe('/t/t/v/v?another=1')
    expect(fetcher).toHaveBeenCalledTimes(1)
    expect(wrapper.get('.grid-toolbar').element).toBe(grid)
  } finally {
    wrapper.unmount()
  }
})

it('discards the new-row dialog on view and identity changes', async () => {
  const auth = useAuthStore()
  auth.setUser(navUser)
  const schema = tableSchema()
  schema.views.push({ ...schema.views[0]!, id: 'v2', name: 'Other' })
  vi.stubGlobal(
    'fetch',
    vi.fn(() => Promise.resolve(Response.json(schema))),
  )
  const { wrapper, router } = await setup('/t/t/v/v')
  try {
    await wrapper.get('.new-entry').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="row-dialog"]').exists()).toBe(true)
    await router.push('/t/t/v/v2')
    await flushPromises()
    expect(wrapper.find('[data-testid="row-dialog"]').exists()).toBe(false)
    await wrapper.get('.new-entry').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="row-dialog"]').exists()).toBe(true)
    auth.setUser({ ...navUser, id: 'someone-else' })
    await flushPromises()
    expect(wrapper.find('[data-testid="row-dialog"]').exists()).toBe(false)
  } finally {
    wrapper.unmount()
  }
})

it('does not open a new-row dialog from a stale mobile add event for readers', async () => {
  useAuthStore().setUser(navUser)
  media.matches = false
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(Response.json({ ...tableSchema(), myLevel: 'read' })),
  )
  const { wrapper } = await setup('/t/t/v/v')
  try {
    await wrapper.get('[data-testid="mobile-cards"] button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="row-dialog"]').exists()).toBe(false)
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
    await router.push('/t/t/v/v/r/9007199254740993?comment=comment-id')
    await flushPromises()
    expect(
      wrapper
        .get('[data-testid="row-dialog"]')
        .attributes('data-focus-comment'),
    ).toBe('comment-id')
    expect(fetcher).toHaveBeenCalledTimes(1)
    await router.push('/t/t/v/hidden/r/1')
    await flushPromises()
    expect(wrapper.find('[data-testid="row-dialog"]').exists()).toBe(false)
  } finally {
    wrapper.unmount()
  }
})
it('shows the authorized table route and production grid without metadata summaries', async () => {
  const fetch = vi.fn().mockResolvedValue(Response.json(tableSchema()))
  vi.stubGlobal('fetch', fetch)
  const { wrapper } = await setup('/t/t/v/v')
  try {
    expect(wrapper.get('h1').text()).toBe('Table')
    expect(wrapper.find('.schema-summary').exists()).toBe(false)
    expect(wrapper.find('.breadcrumbs').exists()).toBe(false)
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

it.each([
  { type: 'grid', desktop: true, level: 'owner' },
  { type: 'grid', desktop: true, level: 'write' },
  { type: 'grid', desktop: true, level: 'read' },
  { type: 'grid', desktop: false, level: 'owner' },
  { type: 'grid', desktop: false, level: 'read' },
  { type: 'form', desktop: true, level: 'write' },
  { type: 'kanban', desktop: false, level: 'owner' },
  { type: 'gallery', desktop: true, level: 'read' },
] as const)(
  'keeps header actions and permissions for $type at desktop=$desktop with $level access',
  async ({ type, desktop, level }) => {
    useAuthStore().setUser(navUser)
    media.matches = desktop
    const schema = tableSchema()
    schema.myLevel = level
    schema.views[0]!.type = type
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json(schema)))
    const empty = { template: '<div />' }
    const { wrapper } = await setup('/t/t/v/v', {
      FieldPanel: empty,
      FormBuilder: empty,
      FormView: empty,
      KanbanBuilder: empty,
      KanbanView: empty,
      GalleryBuilder: empty,
      GalleryView: empty,
    })
    try {
      const toolbar = wrapper.get(
        type === 'grid' && desktop
          ? '[data-testid="production-grid"]'
          : '.table-toolbar',
      )
      expect(toolbar.findAll('.table-history button')).toHaveLength(2)
      expect(toolbar.find('.schema-summary').exists()).toBe(false)
      const transferMenu = toolbar.get('.view-export')
      expect(transferMenu.get('summary').attributes('aria-label')).toBe(
        i18n.global.t('gridPolish.gp06.trigger'),
      )
      const actions = transferMenu
        .findAll('button')
        .map((button) => button.text())
      expect(actions.includes(i18n.global.t('gridPolish.gp06.import'))).toBe(
        level !== 'read',
      )
      expect(actions.includes(i18n.global.t('gridPolish.gp06.export'))).toBe(
        true,
      )
      expect(actions).toHaveLength(1 + Number(level !== 'read'))
      expect(toolbar.find('.visibility-menu').exists()).toBe(level === 'owner')
      const newEntry = toolbar.find('.new-entry')
      expect(newEntry.exists()).toBe(
        type === 'grid' && desktop && level !== 'read',
      )
      if (newEntry.exists()) {
        expect(newEntry.text()).toBe('New')
        expect(newEntry.classes()).toContain('grid-toolbar-button')
        expect(newEntry.find('svg.app-icon').exists()).toBe(true)
      }
      expect(
        toolbar.get('.view-export button:last-of-type').attributes('disabled'),
      ).toBeUndefined()
      expect(wrapper.find('.table-management').exists()).toBe(false)
      expect(wrapper.text()).not.toContain('Views and table options')
      i18n.global.locale.value = 'zh-CN'
      await flushPromises()
      expect(wrapper.text()).not.toContain('视图和数据表选项')
      expect(toolbar.get('.view-export summary').attributes('aria-label')).toBe(
        i18n.global.t('gridPolish.gp06.trigger'),
      )
      if (newEntry.exists()) expect(newEntry.text()).toBe('新建')
    } finally {
      wrapper.unmount()
    }
  },
)

it('keeps undo, redo and collaborator presence in the active table toolbar', async () => {
  useAuthStore().setUser(navUser)
  const undo = useUndoStore()
  undo.record([{ kind: 'delete', tableId: 't', ids: ['1'] }])
  undo.redoStack = [{ steps: [{ kind: 'delete', tableId: 't', ids: ['2'] }] }]
  const undoAction = vi.spyOn(undo, 'undo').mockResolvedValue()
  const redoAction = vi.spyOn(undo, 'redo').mockResolvedValue()
  useRealtimeStore().presence.t = [{ userId: 'bob', name: 'Bob' }]
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(Response.json(tableSchema())),
  )
  const { wrapper } = await setup('/t/t/v/v')
  try {
    const toolbar = wrapper.get('[data-testid="production-grid"]')
    await toolbar.get('button[aria-label="Undo"]').trigger('click')
    await toolbar.get('button[aria-label="Redo"]').trigger('click')
    expect(undoAction).toHaveBeenCalledOnce()
    expect(redoAction).toHaveBeenCalledOnce()
    expect(toolbar.get('[data-presence-user="bob"]').text()).toBe('B')
  } finally {
    wrapper.unmount()
  }
})

it('uses the newly selected table and view for the next export', async () => {
  useAuthStore().setUser(navUser)
  const first = tableSchema()
  const second = tableSchema()
  second.table.id = 't2'
  second.views[0]!.id = 'v2'
  second.views[0]!.config.filter = {
    field: 'name',
    cmp: 'contains',
    value: 'second',
  }
  const fetcher = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'POST' && String(input).endsWith('/export'))
        return Response.json({ ticket: 'A'.repeat(26) }, { status: 201 })
      return Response.json(
        String(input).includes('/tables/t2/') ? second : first,
      )
    },
  )
  vi.stubGlobal('fetch', fetcher)
  const click = vi
    .spyOn(HTMLAnchorElement.prototype, 'click')
    .mockImplementation(() => {})
  const { wrapper, router, nav } = await setup('/t/t/v/v')
  try {
    const tables = nav.tree!.workspaces[0]!.books[0]!.tables
    tables.push({
      ...tables[0]!,
      id: 't2',
      name: 'Second table',
      views: [{ ...tables[0]!.views[0]!, id: 'v2', name: 'Second view' }],
    })
    await router.push('/t/t2/v/v2')
    await flushPromises()
    await wrapper
      .findAll('.view-export button')
      .find(
        (button) => button.text() === i18n.global.t('gridPolish.gp06.export'),
      )!
      .trigger('click')
    await flushPromises()
    const exportCall = fetcher.mock.calls.find(([path]) =>
      String(path).endsWith('/export'),
    )
    expect(String(exportCall?.[0])).toBe('/api/v1/views/v2/export')
    expect(JSON.parse(String(exportCall?.[1]?.body))).toEqual({
      format: 'xlsx',
    })
    expect(click).toHaveBeenCalledOnce()
  } finally {
    wrapper.unmount()
    click.mockRestore()
  }
})

it('keeps toolbar popovers exclusive when another control is activated', async () => {
  useAuthStore().setUser(navUser)
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(Response.json(tableSchema())),
  )
  const { wrapper } = await setup('/t/t/v/v', {
    FieldPanel: { template: '<div />' },
  })
  try {
    const toolbar = wrapper.get('[data-testid="production-grid"]')
    const exportMenu = toolbar.get('.view-export')
    const visibility = toolbar.get('.visibility-menu')
    const fields = wrapper.get('.grid-field-popover')
    const exportDetails = exportMenu.element as HTMLDetailsElement
    const visibilityDetails = visibility.element as HTMLDetailsElement
    const fieldPopover = fields.element as HTMLElement

    exportDetails.open = true
    await toolbar.get('.grid-fields-trigger').trigger('click')
    expect(exportDetails.open).toBe(false)
    expect(fieldPopover.style.display).not.toBe('none')

    await visibility.get('summary').trigger('click')
    expect(fieldPopover.style.display).toBe('none')
    visibilityDetails.open = true
    await exportMenu.get('summary').trigger('click')
    expect(visibilityDetails.open).toBe(false)
  } finally {
    wrapper.unmount()
  }
})

it('retains the field panel when a schema change remounts the grid', async () => {
  useAuthStore().setUser(navUser)
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(Response.json(tableSchema())),
  )
  const mounted = vi.fn()
  const unmounted = vi.fn()
  const panel = {
    props: ['schema'],
    emits: ['update'],
    mounted,
    unmounted,
    template: `
      <button data-testid="advance-schema"
        @click="$emit('update', { ...schema, schemaVersion: schema.schemaVersion + 1 })">
        Update
      </button>`,
  }
  const { wrapper } = await setup('/t/t/v/v', { FieldPanel: panel })
  try {
    await wrapper.get('.grid-fields-trigger').trigger('click')
    const previousGrid = wrapper.get('[data-testid="production-grid"]')
    const previousVersion = Number(
      previousGrid.attributes('data-schema-version'),
    )
    await wrapper.get('[data-testid="advance-schema"]').trigger('click')
    await flushPromises()

    const nextGrid = wrapper.get('[data-testid="production-grid"]')
    expect(nextGrid.element).not.toBe(previousGrid.element)
    expect(Number(nextGrid.attributes('data-schema-version'))).toBe(
      previousVersion + 1,
    )
    expect(mounted).toHaveBeenCalledOnce()
    expect(unmounted).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="advance-schema"]').exists()).toBe(true)
  } finally {
    wrapper.unmount()
  }
})

it('opens compact field and filter menus, applies a draft, and dismisses them with Escape', async () => {
  useAuthStore().setUser(navUser)
  const schema = tableSchema()
  schema.fields[0]!.filterable = true
  schema.fields[0]!.filterOps = ['contains', 'is_empty']
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json(schema)))
  const { wrapper } = await setup('/t/t/v/v')
  try {
    expect(wrapper.find('[role="tab"]').exists()).toBe(false)
    expect(wrapper.find('.field-panel').exists()).toBe(false)
    expect(
      (wrapper.get('.grid-field-popover').element as HTMLElement).style.display,
    ).toBe('none')
    await wrapper.get('.grid-fields-trigger').trigger('click')
    await vi.dynamicImportSettled()
    await flushPromises()
    expect(
      (wrapper.get('.grid-field-popover').element as HTMLElement).style.display,
    ).not.toBe('none')
    expect(wrapper.get('.grid-field-popover').text()).toContain('Name')
    await wrapper.get('.grid-filter-trigger').trigger('click')
    await vi.dynamicImportSettled()
    await flushPromises()
    expect(
      (wrapper.get('.grid-field-popover').element as HTMLElement).style.display,
    ).toBe('none')
    expect(
      (wrapper.get('.grid-filter-popover').element as HTMLElement).style
        .display,
    ).not.toBe('none')
    await wrapper
      .get('.grid-filter-popover .filter-actions button')
      .trigger('click')
    await wrapper
      .get('.grid-filter-popover .filter-item input')
      .setValue('Panda')
    await wrapper
      .findAll('.grid-filter-popover button')
      .find((button) => button.text() === 'Apply filters')!
      .trigger('click')
    expect(
      wrapper
        .get('[data-testid="production-grid"]')
        .attributes('data-filter-count'),
    ).toBe('1')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(
      (wrapper.get('.grid-filter-popover').element as HTMLElement).style
        .display,
    ).toBe('none')
  } finally {
    wrapper.unmount()
  }
})

it('cancels a pending field editor when another menu opens before it loads', async () => {
  useAuthStore().setUser(navUser)
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(Response.json(tableSchema())),
  )
  let release!: (component: Component) => void
  const openCreate = vi.fn()
  const lazyPanel = defineAsyncComponent(
    () =>
      new Promise<Component>((resolve) => {
        release = resolve
      }),
  )
  const { wrapper } = await setup('/t/t/v/v', { FieldPanel: lazyPanel })
  try {
    await wrapper.get('.grid-create-field').trigger('click')
    expect(release).toBeTypeOf('function')
    await wrapper.get('.grid-filter-trigger').trigger('click')
    release(
      defineComponent({
        setup(_props, { expose }) {
          expose({ openCreate })
          return () => null
        },
      }),
    )
    await vi.dynamicImportSettled()
    await flushPromises()
    expect(openCreate).not.toHaveBeenCalled()
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
    expect(wrapper.find('.schema-summary').exists()).toBe(false)
    expect(
      wrapper
        .findAll('.view-export button')
        .some((button) => button.text() === i18n.global.t('import.append')),
    ).toBe(false)
    expect(
      wrapper.findAll('button').some((button) => button.text() === 'Share'),
    ).toBe(false)
  } finally {
    wrapper.unmount()
  }
})

it('moves table sharing into the Grid Visibility menu with Escape, outside dismissal and focus return', async () => {
  useAuthStore().setUser(navUser)
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL) =>
      Promise.resolve(
        String(input).includes('/grants')
          ? Response.json({
              effectiveLevel: 'owner',
              direct: [],
              inherited: [],
            })
          : Response.json(tableSchema()),
      ),
    ),
  )
  const { wrapper } = await setup('/t/t/v/v', {}, document.body)
  const sharing = useSharingStore()
  try {
    expect(wrapper.find('.table-toolbar .visibility-menu').exists()).toBe(false)
    expect(
      wrapper
        .findAll('.table-toolbar > button')
        .some((button) => button.text() === 'Share'),
    ).toBe(false)
    const toolbar = wrapper.get('[data-testid="production-grid"]')
    expect(
      [...toolbar.element.children]
        .filter((element) => element.tagName === 'DETAILS')
        .map((element) => element.className),
    ).toEqual([
      'grid-highlights',
      'visibility-menu visibility-menu--toolbar',
      'grid-format',
    ])
    expect(
      toolbar.get('.grid-toolbar-end > details.grid-search').element
        .nextElementSibling,
    ).toBe(toolbar.get('.new-entry').element)
    const menu = toolbar.get('.visibility-menu')
    const details = menu.element as HTMLDetailsElement
    const summary = menu.get('summary')
    expect(summary.attributes('aria-label')).toBe('Visibility')
    expect(summary.find('svg.app-icon').exists()).toBe(true)

    details.open = true
    await menu.trigger('keydown', { key: 'Escape' })
    expect(details.open).toBe(false)
    expect(document.activeElement).toBe(summary.element)

    details.open = true
    document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    expect(details.open).toBe(false)

    details.open = true
    await menu.get('button').trigger('click')
    expect(details.open).toBe(false)
    expect(sharing.target).toEqual({ kind: 'table', id: 't', name: 'Table' })
    ;(document.activeElement as HTMLElement | null)?.blur()
    sharing.close(true)
    await flushPromises()
    expect(document.activeElement).toBe(summary.element)
  } finally {
    wrapper.unmount()
  }
})

it('keeps Visibility sharing on mobile and non-Grid views and hides it from non-owners', async () => {
  useAuthStore().setUser(navUser)
  let level = 'owner'
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve(Response.json({ ...tableSchema(), myLevel: level })),
    ),
  )
  media.matches = false
  const { wrapper, nav } = await setup('/t/t/v/v')
  try {
    expect(wrapper.find('[data-testid="mobile-cards"]').exists()).toBe(true)
    const menu = wrapper.get('.table-toolbar .visibility-menu--utility')
    expect(menu.get('summary').attributes('aria-label')).toBe('Visibility')
    i18n.global.locale.value = 'zh-CN'
    await flushPromises()
    expect(menu.get('summary').attributes('aria-label')).toBe('可见性')
    expect(menu.get('button').text()).toBe('共享')
    i18n.global.locale.value = 'en'
    level = 'write'
    nav.revision++
    await flushPromises()
    expect(wrapper.find('.visibility-menu').exists()).toBe(false)
  } finally {
    wrapper.unmount()
  }
})

it('dismisses the mobile filter on outside pointer or Escape and restores focus', async () => {
  useAuthStore().setUser(navUser)
  media.matches = false
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(Response.json(tableSchema())),
  )
  const { wrapper } = await setup('/t/t/v/v', {}, document.body)
  try {
    const menu = wrapper.get<HTMLDetailsElement>('.view-filter')
    const summary = menu.get('summary')
    menu.element.open = true
    summary.element.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    expect(menu.element.open).toBe(true)
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(menu.element.open).toBe(false)
    expect(document.activeElement).toBe(summary.element)
    menu.element.open = true
    document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    expect(menu.element.open).toBe(false)
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
    expect(wrapper.find('.schema-summary').exists()).toBe(false)
    expect(
      wrapper
        .findAll('.view-export button')
        .some((button) => button.text() === i18n.global.t('import.append')),
    ).toBe(false)
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
    expect(wrapper.find('.schema-summary').exists()).toBe(false)
    expect(
      wrapper
        .findAll('.view-export button')
        .some((button) => button.text() === i18n.global.t('import.append')),
    ).toBe(false)
  } finally {
    wrapper.unmount()
  }
})

it('normalizes a bare table route to its default view and ignores schema reads from an abandoned route', async () => {
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockImplementation(() => Promise.resolve(Response.json(tableSchema()))),
  )
  const first = await setup('/t/t')
  try {
    expect(first.router.currentRoute.value.path).toBe('/t/t/v/v')
    expect(first.wrapper.find('[data-testid="production-grid"]').exists()).toBe(
      true,
    )
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
