import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { nextTick } from 'vue'
import { i18n, loadLocale } from '../../i18n'
import { useNavStore } from '../../stores/nav'
import { useAuthStore } from '../../stores/auth'
import { usePreferencesStore } from '../../stores/preferences'
import { navigation, navUser } from '../../test/navigation'
import { themeMedia } from '../../test/setup'
import NavContents from './NavContents.vue'
import NavSidebar from './NavSidebar.vue'
import StructureDialog from './StructureDialog.vue'

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

it('renders all workspaces, separate workspace and workbook toggles, views and owner-only reorder', async () => {
  const { global } = await setup()
  const nav = useNavStore()
  const tree = navigation()
  const b = tree.workspaces[0]!.books[0]!
  b.tables.push({ ...b.tables[0]!, id: 't2', name: 'Second' })
  tree.workspaces.push({ ...tree.workspaces[0]!, id: 'w2', books: [] })
  nav.tree = tree
  const reorder = vi.spyOn(nav, 'reorder').mockResolvedValue()
  const wrapper = mount(NavContents, { global })
  try {
    const box = wrapper.get('[data-nav-id="w"] > .workspace-box a')
    expect(box.attributes('href')).toBe('/w/w')
    await box.trigger('click')
    expect(wrapper.emitted('navigate')).toHaveLength(1)
    expect(wrapper.find('[data-nav-id="w2"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-nav-id]')).toHaveLength(5)
    expect(wrapper.get('.workspace-heading h2').text()).toBe('Workspaces')
    await wrapper.get('.workspace-heading button').trigger('click')
    expect(nav.editor).toEqual({ operation: 'create', kind: 'workspace' })
    nav.editor = null
    const workspaceToggle = wrapper.get('[data-nav-id="w"] .workspace-toggle')
    expect(workspaceToggle.attributes('aria-expanded')).toBe('true')
    await workspaceToggle.trigger('click')
    expect(workspaceToggle.attributes('aria-expanded')).toBe('false')
    expect(wrapper.get('#workspace-children-w').attributes('style')).toContain(
      'display: none',
    )
    await workspaceToggle.trigger('click')
    expect(wrapper.find('[data-nav-id="w"] > .nav-row').exists()).toBe(false)
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
    const book = wrapper.get('[data-nav-id="b"] .book-toggle')
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
    expect(wrapper.find('[data-nav-id="w"] > .node-actions').exists()).toBe(
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

it('distinguishes the default from the selected deep-link workspace and excludes path-only actions', async () => {
  const { global, router } = await setup('/t/t/v/v/r/42')
  const nav = useNavStore()
  const tree = navigation()
  tree.workspaces.push(
    {
      ...tree.workspaces[0]!,
      id: 'w2',
      name: 'Shared workspace',
      level: 'read',
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
  const setDefault = vi
    .spyOn(nav, 'setPreferredWorkspace')
    .mockImplementation(async (id) => {
      nav.preferredWorkspaceId = id
      return id
    })
  const wrapper = mount(NavContents, { global, attachTo: document.body })
  try {
    expect(wrapper.findAll('.workspace-root > li')).toHaveLength(3)
    expect(
      wrapper.get('[data-nav-id="w"] > .workspace-box').classes(),
    ).toContain('current')
    expect(
      wrapper
        .get('[data-nav-id="w"] .workspace-link')
        .attributes('aria-current'),
    ).toBe('location')
    expect(
      wrapper
        .get('[data-nav-id="w"] .workspace-default')
        .attributes('aria-label'),
    ).toBe('Default workspace: Workspace')
    expect(
      wrapper.get('[data-nav-id="w"] .workspace-default').attributes('role'),
    ).toBe('img')
    expect(wrapper.find('[data-nav-id="w3"] > .node-actions').exists()).toBe(
      false,
    )
    const sharedActions = wrapper.get('[data-nav-id="w2"] > .node-actions')
    expect(sharedActions.text()).toContain('Set as default')
    expect(sharedActions.text()).not.toContain('Rename')
    expect(sharedActions.text()).not.toContain('Move to Trash')
    await wrapper.get('[data-nav-id="w2"] .workspace-link').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/w/w2')
    expect(nav.preferredWorkspaceId).toBe('w')
    expect(setDefault).not.toHaveBeenCalled()
    expect(
      wrapper.get('[data-nav-id="w2"] > .workspace-box').classes(),
    ).toContain('current')
    i18n.global.locale.value = 'zh-CN'
    await nextTick()
    expect(wrapper.get('.workspace-heading h2').text()).toBe('工作区')
    expect(sharedActions.text()).toContain('设为默认工作区')
    await sharedActions.get('button').trigger('click')
    await flushPromises()
    expect(setDefault).toHaveBeenCalledWith('w2')
    expect(document.activeElement).toBe(sharedActions.get('summary').element)
    expect(sharedActions.get('button').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-nav-id="w2"] .workspace-default').text()).toBe(
      '默认',
    )
    expect(wrapper.find('[data-nav-id="w"] .workspace-default').exists()).toBe(
      false,
    )
    await router.push('/t/t/v/v/r/42')
    expect(nav.preferredWorkspaceId).toBe('w2')
    expect(
      wrapper.get('[data-nav-id="w"] > .workspace-box').classes(),
    ).toContain('current')
    nav.lastWorkspaceId = 'w'
    await router.push('/t/missing')
    expect(wrapper.find('.workspace-box.current').exists()).toBe(false)
    expect(nav.preferredWorkspaceId).toBe('w2')
  } finally {
    wrapper.unmount()
    i18n.global.locale.value = 'en'
  }
})

it('keeps one structure action menu open and closes it outside or on Escape', async () => {
  const { global } = await setup()
  const wrapper = mount(NavContents, { global, attachTo: document.body })
  try {
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
