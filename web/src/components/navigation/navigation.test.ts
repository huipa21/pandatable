import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { nextTick } from 'vue'
import { RequestError } from '../../api/client'
import { i18n, loadLocale } from '../../i18n'
import { useNavStore } from '../../stores/nav'
import { useAuthStore } from '../../stores/auth'
import { usePreferencesStore } from '../../stores/preferences'
import { navigation, navUser } from '../../test/navigation'
import { themeMedia } from '../../test/setup'
import NavContents from './NavContents.vue'
import NavActions from './NavActions.vue'
import NavSidebar from './NavSidebar.vue'
import StructureDialog from './StructureDialog.vue'
import IconPicker from '../common/IconPicker.vue'

let pinia: ReturnType<typeof createPinia>
beforeEach(async () => {
  pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().setUser(navUser)
  await loadLocale('en')
  await loadLocale('zh-CN')
  i18n.global.locale.value = 'en'
  vi.stubGlobal('matchMedia', () => themeMedia)
  themeMedia.setDark(false)
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockImplementation((path: string) =>
        Promise.resolve(
          Response.json(
            path.endsWith('/nav') ? navigation() : { workspaceId: 'w' },
          ),
        ),
      ),
  )
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
  vi.useRealTimers()
})
async function setup(path = '/w/w') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/w/:workspaceId', component: { template: '<div />' } },
      { path: '/w/:wid/b/:bookId', component: { template: '<div />' } },
      {
        path: '/t/:tableId/v/:viewId/r/:rowId',
        component: { template: '<div />' },
      },
      { path: '/t/:tableId/v/:viewId', component: { template: '<div />' } },
      { path: '/t/:tableId', component: { template: '<div />' } },
      { path: '/:pathMatch(.*)*', component: { template: '<div />' } },
    ],
  })
  await router.push(path)
  await useNavStore().refresh()
  return { global: { plugins: [pinia, i18n, router] }, router }
}

it('keeps workspace context singular and preserves the active workbook tree controls', async () => {
  const { global, router } = await setup()
  const nav = useNavStore()
  const tree = navigation()
  const b = tree.workspaces[0]!.books[0]!
  b.tables.push({ ...b.tables[0]!, id: 't2', name: 'Second' })
  tree.workspaces.push({ ...tree.workspaces[0]!, id: 'w2', books: [] })
  nav.tree = tree
  const reorder = vi.spyOn(nav, 'reorder').mockResolvedValue()
  const wrapper = mount(NavContents, { global })
  try {
    const box = wrapper.get('.workspace-root li[data-kind="workspace"] a')
    expect(box.attributes('href')).toBe('/w/w')
    expect(box.attributes('aria-current')).toBe('page')
    expect(box.find('.app-icon').exists()).toBe(false)
    await box.trigger('click')
    expect(wrapper.emitted('navigate')).toHaveLength(1)
    expect(wrapper.findAll('.workspace-root > li')).toHaveLength(2)
    expect(wrapper.find('[data-nav-id="w2"]').exists()).toBe(false)
    expect(wrapper.find('.workbook-root').exists()).toBe(false)
    expect(wrapper.find('.workspace-heading').exists()).toBe(false)
    expect(wrapper.find('.workspace-nav-menu').exists()).toBe(true)
    await router.push('/w/w/b/b')
    await nextTick()
    expect(
      wrapper
        .get('.workspace-root [data-nav-id="w"] .workspace-link')
        .attributes('aria-current'),
    ).toBe('location')
    expect(wrapper.get('.workbook-root').attributes('data-context-book')).toBe(
      'b',
    )
    expect(wrapper.get('[data-nav-id="b"] > .nav-row').classes()).toContain(
      'current',
    )
    const book = wrapper.get('[data-nav-id="b"] .book-toggle')
    const bookLink = wrapper.get('[data-nav-id="b"] .book-link')
    expect(book.attributes('aria-expanded')).toBe('false')
    expect(book.attributes('aria-label')).toBe('Expand or collapse Book')
    expect(bookLink.attributes('href')).toBe('/w/w/b/b')
    expect(bookLink.attributes('aria-current')).toBe('page')
    await book.trigger('click')
    expect(router.currentRoute.value.path).toBe('/w/w/b/b')
    expect(book.attributes('aria-expanded')).toBe('true')
    expect(wrapper.find('[data-nav-id="t"]').exists()).toBe(true)
    await bookLink.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/w/w/b/b')
    expect(book.attributes('aria-expanded')).toBe('true')
    await router.push('/t/t2')
    await nextTick()
    expect(book.attributes('aria-expanded')).toBe('true')
    expect(
      wrapper
        .get('[data-nav-id="t2"] .table-toggle')
        .attributes('aria-expanded'),
    ).toBe('false')
    await router.push('/w/w/b/b')
    await nextTick()
    expect(book.attributes('aria-expanded')).toBe('true')
    expect(wrapper.get('[data-nav-id="t"] a.nav-name').attributes('href')).toBe(
      '/t/t',
    )
    expect(wrapper.find('.view-list').exists()).toBe(false)
    const views = wrapper.get('[data-nav-id="t"] .table-toggle')
    expect(views.attributes('aria-expanded')).toBe('false')
    await views.trigger('click')
    expect(
      wrapper.get('[data-nav-id="t"] .view-list a').attributes('href'),
    ).toBe('/t/t/v/v')
    await views.trigger('click')
    expect(wrapper.find('.view-list').exists()).toBe(false)
    expect(book.attributes('aria-expanded')).toBe('true')
    await book.trigger('click')
    expect(wrapper.find('[data-nav-id="t"]').exists()).toBe(false)
    await book.trigger('click')
    const row = wrapper.get('[data-nav-id="t"]')
    await row.get('summary').trigger('click')
    await row
      .findAll('button')
      .find((button) => button.text() === 'Move down')!
      .trigger('click')
    expect(reorder).toHaveBeenLastCalledWith(
      expect.objectContaining({ id: 't' }),
      1,
    )
    await row.get('.nav-row').trigger('dragstart')
    await wrapper.get('[data-nav-id="t2"] .nav-row').trigger('drop')
    expect(reorder).toHaveBeenCalledTimes(2)
    await row.get('.nav-row').trigger('dragstart')
    await wrapper.get('[data-nav-id="b"] > .nav-row').trigger('drop')
    expect(reorder).toHaveBeenCalledTimes(2)
    tree.workspaces[0]!.level = 'none'
    tree.workspaces[0]!.pathOnly = true
    b.level = 'none'
    b.pathOnly = true
    b.tables.forEach((table) => {
      table.level = 'read'
    })
    nav.tree = structuredClone(tree)
    await nextTick()
    expect(wrapper.find('[data-nav-id="w"] .workspace-nav-menu').exists()).toBe(
      false,
    )
    expect(wrapper.find('[data-nav-id="b"] > .node-actions').exists()).toBe(
      false,
    )
    const readerActions = wrapper.get('[data-nav-id="t"] > .node-actions')
    expect(readerActions.text()).toContain('Create view')
    expect(readerActions.text()).not.toContain('Rename')
    expect(readerActions.text()).not.toContain('Move to Trash')
    expect(wrapper.text()).toContain('Path only')
  } finally {
    wrapper.unmount()
  }
})

it('resolves contextual trees only from fresh authorized route ancestry', async () => {
  const { global, router } = await setup('/w/w/b/b')
  const nav = useNavStore()
  const tree = navigation()
  const secondWorkspace = structuredClone(tree.workspaces[0]!)
  secondWorkspace.id = 'w2'
  secondWorkspace.name = 'Second workspace'
  secondWorkspace.books[0]!.id = 'b2'
  secondWorkspace.books[0]!.name = 'Second book'
  secondWorkspace.books[0]!.tables[0]!.id = 't2'
  secondWorkspace.books[0]!.tables[0]!.name = 'Second table'
  secondWorkspace.books[0]!.tables[0]!.views[0]!.id = 'v2'
  tree.workspaces.push(secondWorkspace)
  nav.tree = tree
  const wrapper = mount(NavContents, { global })
  try {
    expect(wrapper.findAll('.workspace-root > li')).toHaveLength(2)
    expect(wrapper.find('.workspace-root [data-kind="table"]').exists()).toBe(
      false,
    )
    expect(wrapper.get('.workbook-root').attributes('data-context-book')).toBe(
      'b',
    )
    expect(wrapper.findAll('.workbook-root [data-kind="book"]')).toHaveLength(1)
    expect(wrapper.find('[data-nav-id="b2"]').exists()).toBe(false)

    await router.push('/t/t')
    await nextTick()
    expect(
      wrapper.get('[data-nav-id="b"] .book-toggle').attributes('aria-expanded'),
    ).toBe('true')
    expect(wrapper.find('[data-nav-id="t"]').exists()).toBe(true)
    expect(wrapper.get('[data-nav-id="t"] > .nav-row').classes()).toContain(
      'current',
    )
    expect(
      wrapper
        .get('[data-nav-id="t"] .table-toggle')
        .attributes('aria-expanded'),
    ).toBe('false')

    await router.push('/t/t/v/v')
    await nextTick()
    expect(wrapper.get('[data-view-id="v"] a').attributes('aria-current')).toBe(
      'page',
    )
    expect(
      wrapper
        .get('[data-nav-id="t"] .table-toggle')
        .attributes('aria-expanded'),
    ).toBe('true')

    await router.push('/t/t/v/v/r/42')
    await nextTick()
    expect(wrapper.get('.workbook-root').attributes('data-context-book')).toBe(
      'b',
    )
    expect(wrapper.get('[data-view-id="v"] a').attributes('aria-current')).toBe(
      'page',
    )

    await router.push('/w/w')
    await nextTick()
    expect(wrapper.find('.workbook-root').exists()).toBe(false)
    expect(wrapper.find('.workspace-root [data-kind="book"]').exists()).toBe(
      false,
    )

    await router.push('/t/t/v/missing')
    await nextTick()
    expect(wrapper.find('.workbook-root').exists()).toBe(false)

    await router.push('/w/w2/b/b2')
    await nextTick()
    expect(wrapper.get('.workbook-root').attributes('data-context-book')).toBe(
      'b2',
    )
    expect(wrapper.find('[data-nav-id="b"]').exists()).toBe(false)
    expect(
      wrapper
        .get('[data-nav-id="b2"] .book-toggle')
        .attributes('aria-expanded'),
    ).toBe('false')

    nav.loading = true
    await nextTick()
    expect(wrapper.find('.workbook-root').exists()).toBe(false)
    nav.loading = false
    nav.error = new Error('stale navigation')
    await nextTick()
    expect(wrapper.find('.workbook-root').exists()).toBe(false)

    useAuthStore().setUser({ ...navUser, id: 'bob' })
    await nextTick()
    expect(wrapper.find('.workbook-root').exists()).toBe(false)
    expect(
      wrapper.findAll('.workspace-root > li[data-kind="workspace"]'),
    ).toHaveLength(0)
    expect(
      wrapper.find('.workspace-root li[data-kind="dashboard"] a').exists(),
    ).toBe(true)
  } finally {
    wrapper.unmount()
  }
})

it('shows only path-only workspace ancestry for a table-only grantee', async () => {
  const { global, router } = await setup('/t/t/v/v/r/42')
  const nav = useNavStore()
  const tree = navigation()
  tree.workspaces[0]!.level = 'none'
  tree.workspaces[0]!.pathOnly = true
  tree.workspaces[0]!.books[0]!.level = 'none'
  tree.workspaces[0]!.books[0]!.pathOnly = true
  tree.workspaces[0]!.books[0]!.tables[0]!.level = 'read'
  tree.workspaces.push(
    {
      ...tree.workspaces[0]!,
      id: 'w2',
      name: 'Shared workspace',
      level: 'read',
      pathOnly: false,
      books: [],
    },
    {
      ...tree.workspaces[0]!,
      id: 'w3',
      name: 'Path context',
      level: 'none',
      pathOnly: true,
      books: [],
    },
  )
  nav.tree = tree
  nav.preferenceLoaded = true
  nav.preferredWorkspaceId = 'w'
  const wrapper = mount(NavContents, { global, attachTo: document.body })
  try {
    expect(wrapper.findAll('.workspace-root > li')).toHaveLength(2)
    expect(
      wrapper.get('[data-nav-id="w"] > .workspace-box').classes(),
    ).toContain('current')
    expect(
      wrapper
        .get('[data-nav-id="w"] .workspace-link')
        .attributes('aria-current'),
    ).toBe('location')
    expect(wrapper.get('[data-nav-id="w"] .path-only').text()).toBe('Path only')
    expect(wrapper.get('.workbook-root').attributes('data-context-book')).toBe(
      'b',
    )
    expect(wrapper.find('[data-nav-id="w2"]').exists()).toBe(false)
    expect(
      wrapper.find('[data-nav-id="w3"] .workspace-nav-menu').exists(),
    ).toBe(false)
    i18n.global.locale.value = 'zh-CN'
    await nextTick()
    expect(
      wrapper.get('.workspace-root li[data-kind="dashboard"] a').text(),
    ).toBe('工作区')
    expect(wrapper.get('[data-nav-id="w"] .path-only').text()).toBe('仅路径')
    await router.push('/t/t/v/v/r/42')
    nav.lastWorkspaceId = 'w'
    await router.push('/t/missing')
    await nextTick()
    expect(
      wrapper.find('.workspace-root [data-kind="workspace"]').exists(),
    ).toBe(false)
    expect(wrapper.find('.workbook-root').exists()).toBe(false)
  } finally {
    wrapper.unmount()
    i18n.global.locale.value = 'en'
  }
})

it('keeps one structure action menu open and closes it outside or on Escape', async () => {
  const { global } = await setup('/w/w/b/b')
  const wrapper = mount(NavContents, { global, attachTo: document.body })
  try {
    await wrapper.get('[data-nav-id="b"] .book-toggle').trigger('click')
    const menu = (id: string) =>
      wrapper.get(`[data-nav-id="${id}"] > .node-actions`)
        .element as HTMLDetailsElement
    const open = async (id: string) => {
      menu(id).open = true
      menu(id).dispatchEvent(new Event('toggle'))
      await nextTick()
    }
    await open('b')
    await open('t')
    expect(menu('b').open).toBe(false)
    expect(menu('t').open).toBe(true)
    const dialog = document.createElement('dialog')
    document.body.append(dialog)
    dialog.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    document.body.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }),
    )
    expect(menu('t').open).toBe(true)
    dialog.remove()
    menu('t')
      .querySelector('button')!
      .dispatchEvent(
        new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }),
      )
    expect(menu('t').open).toBe(false)
    expect(document.activeElement).toBe(menu('t').querySelector('summary'))
    await open('t')
    document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    expect(menu('t').open).toBe(false)
  } finally {
    wrapper.unmount()
  }
})

it('validates code points, retains localized mutation errors and closes confirmed dialogs', async () => {
  const { global } = await setup()
  const nav = useNavStore()
  const wrapper = mount(StructureDialog, { global, attachTo: document.body })
  try {
    nav.editor = { operation: 'create', kind: 'workspace' }
    await flushPromises()
    expect(wrapper.get('dialog').element.open).toBe(true)
    const input = wrapper.get('input')
    await input.setValue('😀'.repeat(100))
    expect(
      wrapper.get('button[type="submit"]').attributes('disabled'),
    ).toBeUndefined()
    await input.setValue('😀'.repeat(101))
    expect(
      wrapper.get('button[type="submit"]').attributes('disabled'),
    ).toBeDefined()
    await input.setValue('   ')
    expect(
      wrapper.get('button[type="submit"]').attributes('disabled'),
    ).toBeDefined()
    await input.setValue('Valid')
    vi.spyOn(nav, 'submit').mockRejectedValue(new TypeError('offline'))
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).not.toContain('offline')
    i18n.global.locale.value = 'zh-CN'
    await nextTick()
    expect(wrapper.get('h2').text()).toBe('新建工作区')
    nav.editor = {
      operation: 'delete',
      kind: 'workspace',
      id: 'w',
      name: '<script>',
    }
    await flushPromises()
    expect(wrapper.text()).toContain('回收站')
    expect(wrapper.find('script').exists()).toBe(false)
    vi.mocked(nav.submit).mockResolvedValue(null)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(nav.editor).toBeNull()
    expect(wrapper.get('dialog').element.open).toBe(false)
  } finally {
    wrapper.unmount()
    i18n.global.locale.value = 'en'
  }
})

it('offers owner-only workspace editing instead of Rename', async () => {
  const { global } = await setup()
  const nav = useNavStore()
  const item = nav.items[0]!
  const wrapper = mount(NavActions, {
    global,
    props: { item },
  })
  try {
    expect(wrapper.text()).toContain('Edit workspace')
    expect(wrapper.text()).not.toContain('Rename')
    await wrapper.setProps({ item: { ...item, canEdit: false, level: 'read' } })
    expect(wrapper.text()).not.toContain('Edit workspace')
  } finally {
    wrapper.unmount()
  }
})

it('announces icon choices as a radio group and supports arrow-key selection', async () => {
  const { global } = await setup()
  const wrapper = mount(IconPicker, {
    global,
    props: { modelValue: null },
    attachTo: document.body,
  })
  try {
    expect(wrapper.get('[role="radiogroup"]').attributes('aria-label')).toBe(
      'Icon',
    )
    expect(
      wrapper
        .get('[aria-label="Random icon (use the workspace fallback)"]')
        .attributes('aria-checked'),
    ).toBe('true')
    await wrapper
      .get('[aria-label="Random icon (use the workspace fallback)"]')
      .trigger('keydown', { key: 'ArrowRight' })
    await nextTick()
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual(['panelClose'])
    expect(document.activeElement).toBe(
      wrapper.get('[aria-label="Select panelClose icon"]').element,
    )
  } finally {
    wrapper.unmount()
  }
})

it('submits only changed workspace metadata and supports clearing fields', async () => {
  const { global } = await setup()
  const nav = useNavStore()
  nav.tree.workspaces[0]!.icon = 'star'
  nav.tree.workspaces[0]!.description = 'Before'
  const submit = vi.spyOn(nav, 'submit').mockResolvedValue(null)
  const wrapper = mount(StructureDialog, { global, attachTo: document.body })
  try {
    const action = {
      operation: 'rename' as const,
      kind: 'workspace' as const,
      id: 'w',
      name: 'Workspace',
    }
    nav.editor = action
    await flushPromises()
    expect(wrapper.get('h2').text()).toBe('Edit workspace')
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe(
      'Before',
    )
    await wrapper.get('[aria-label="Select book icon"]').trigger('click')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(submit).toHaveBeenLastCalledWith(action, 'Workspace', {
      icon: 'book',
    })

    nav.editor = { ...action }
    await flushPromises()
    await wrapper
      .get('[aria-label="Random icon (use the workspace fallback)"]')
      .trigger('click')
    await wrapper.get('textarea').setValue('')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(submit).toHaveBeenLastCalledWith(action, 'Workspace', {
      icon: null,
      description: '',
    })
  } finally {
    wrapper.unmount()
  }
})

it('renders overlong descriptions and server validation errors inline', async () => {
  const { global } = await setup()
  const nav = useNavStore()
  const submit = vi.spyOn(nav, 'submit')
  const wrapper = mount(StructureDialog, { global, attachTo: document.body })
  try {
    nav.editor = { operation: 'create', kind: 'workspace' }
    await flushPromises()
    await wrapper.get('textarea').setValue('x'.repeat(501))
    expect(wrapper.get('#workspace-description-counter').text()).toBe(
      '501/500 characters',
    )
    expect(wrapper.get('#workspace-description-error').text()).toBe(
      'Description must be 500 characters or fewer.',
    )
    expect(
      wrapper.get('button[type="submit"]').attributes('disabled'),
    ).toBeDefined()
    expect(submit).not.toHaveBeenCalled()

    await wrapper.get('textarea').setValue('')
    await wrapper.get('input').setValue('Workspace')
    submit.mockRejectedValue(new RequestError('ERR_VALIDATION', 400))
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(submit).toHaveBeenLastCalledWith(
      { operation: 'create', kind: 'workspace' },
      'Workspace',
      {},
    )
    expect(wrapper.get('[role="alert"]').text()).toBe(
      'Check the workspace name, icon, and description.',
    )
  } finally {
    wrapper.unmount()
  }
})

it('keeps stale preference errors and retry accessible in expanded, collapsed and mobile navigation', async () => {
  const { global } = await setup()
  const nav = useNavStore()
  const refreshPreference = vi
    .spyOn(nav, 'refreshPreference')
    .mockResolvedValue(false)
  const preferences = usePreferencesStore()
  preferences.sidebarCollapsed = false
  const wrapper = mount(NavSidebar, { global, attachTo: document.body })
  try {
    nav.preferenceLoaded = true
    nav.preferenceError = new TypeError('offline')
    await flushPromises()
    expect(wrapper.get('.sidebar-desktop .nav-error').text()).toContain(
      'Try again',
    )
    expect(wrapper.get('.sidebar-desktop .nav-error').text()).toContain(
      'Unable to load or update your default workspace',
    )
    expect(wrapper.get('.nav-tree').text()).toContain(
      'The default workspace could not be refreshed',
    )
    await wrapper.get('.sidebar-desktop .nav-error button').trigger('click')
    expect(refreshPreference).toHaveBeenLastCalledWith(true)
    preferences.sidebarCollapsed = true
    await nextTick()
    expect(wrapper.get('.nav-error-floating button').isVisible()).toBe(true)
    themeMedia.setDark(true)
    await nextTick()
    nav.drawerOpen = true
    await flushPromises()
    expect(wrapper.get('.nav-drawer .nav-error button').isVisible()).toBe(true)
    expect(wrapper.find('.nav-error-floating').exists()).toBe(false)
    await wrapper.get('.nav-drawer').trigger('cancel')
    await nextTick()
    expect(wrapper.get('.nav-error-floating button').isVisible()).toBe(true)
  } finally {
    wrapper.unmount()
    preferences.sidebarCollapsed = false
  }
})

it('does not abort the new identity when the previous sidebar unmounts', async () => {
  const { global } = await setup()
  const nav = useNavStore()
  const wrapper = mount(NavSidebar, { global, attachTo: document.body })
  await flushPromises()
  useAuthStore().setUser({ ...navUser, id: 'bob' })
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
  const resolution = nav.resolvePreference()
  wrapper.unmount()
  release(Response.json({ workspaceId: 'bob-default', eventId: 'opaque' }))
  expect(await resolution).toBe('bob-default')
  expect(nav.preferredWorkspaceId).toBe('bob-default')
})

it('refreshes on focus/visible interval, stops hidden/unmounted and closes the mobile drawer on Escape', async () => {
  const { global } = await setup()
  vi.useFakeTimers()
  const nav = useNavStore()
  const refresh = vi.spyOn(nav, 'refresh').mockResolvedValue(true)
  const wrapper = mount(NavSidebar, { global, attachTo: document.body })
  try {
    expect(refresh).toHaveBeenCalledTimes(1)
    window.dispatchEvent(new Event('focus'))
    expect(refresh).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(30_000)
    expect(refresh).toHaveBeenCalledTimes(3)
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden')
    document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(60_000)
    expect(refresh).toHaveBeenCalledTimes(3)
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
    document.dispatchEvent(new Event('visibilitychange'))
    expect(refresh).toHaveBeenCalledTimes(4)
    themeMedia.setDark(true)
    await nextTick()
    // The header's sidebar toggle opens the drawer through the nav store.
    nav.drawerOpen = true
    await flushPromises()
    expect(wrapper.get<HTMLDialogElement>('.nav-drawer').element.open).toBe(
      true,
    )
    await wrapper.get('.nav-drawer').trigger('cancel')
    expect(wrapper.get<HTMLDialogElement>('.nav-drawer').element.open).toBe(
      false,
    )
    expect(nav.drawerOpen).toBe(false)
  } finally {
    wrapper.unmount()
    vi.restoreAllMocks()
  }
  const count = refresh.mock.calls.length
  await vi.advanceTimersByTimeAsync(60_000)
  window.dispatchEvent(new Event('focus'))
  expect(refresh).toHaveBeenCalledTimes(count)
})
