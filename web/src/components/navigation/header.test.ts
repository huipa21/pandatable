import { afterEach, beforeEach, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { i18n, loadLocale } from '../../i18n'
import { useAuthStore } from '../../stores/auth'
import { useNavStore } from '../../stores/nav'
import { navigation, navUser } from '../../test/navigation'
import AppBreadcrumbs from './AppBreadcrumbs.vue'
import SearchPalette from './SearchPalette.vue'
import UserMenu from '../account/UserMenu.vue'

let pinia: ReturnType<typeof createPinia>
beforeEach(async () => {
  pinia = createPinia()
  setActivePinia(pinia)
  await loadLocale('en')
  i18n.global.locale.value = 'en'
  HTMLDialogElement.prototype.showModal = function () {
    this.open = true
  }
  HTMLDialogElement.prototype.close = function () {
    this.open = false
  }
  useNavStore().tree = navigation()
})
afterEach(() => disposePinia(pinia))

async function setup(path: string) {
  const page = { template: '<div />' }
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/home', component: page },
      { path: '/w/:workspaceId', component: page },
      { path: '/w/:wid/b/:bookId', component: page },
      { path: '/t/:tableId', component: page },
      { path: '/t/:tableId/v/:viewId', component: page },
      { path: '/:pathMatch(.*)*', component: page },
    ],
  })
  await router.push(path)
  return { router, global: { plugins: [pinia, i18n, router] } }
}
function crumbs(wrapper: ReturnType<typeof mount>) {
  return wrapper.findAll('.breadcrumbs a').map((item) => item.text())
}

it('builds header breadcrumbs from the route and adds the view only when named', async () => {
  const { router, global } = await setup('/t/t')
  const wrapper = mount(AppBreadcrumbs, { global })
  try {
    const routes = [
      {
        path: '/w/w',
        texts: ['Workspace'],
        hrefs: ['/w/w'],
      },
      {
        path: '/w/w/b/b',
        texts: ['Workspace', 'Book'],
        hrefs: ['/w/w', '/w/w/b/b'],
      },
      {
        path: '/t/t',
        texts: ['Workspace', 'Book', 'Table'],
        hrefs: ['/w/w', '/w/w/b/b', '/t/t'],
      },
      {
        path: '/t/t/v/v',
        texts: ['Workspace', 'Book', 'Table', 'Grid'],
        hrefs: ['/w/w', '/w/w/b/b', '/t/t', '/t/t/v/v'],
      },
    ]
    for (const routeCase of routes) {
      await router.push(routeCase.path)
      await flushPromises()
      expect(crumbs(wrapper)).toEqual(routeCase.texts)
      const links = wrapper.findAll('.breadcrumbs a')
      expect(links.map((link) => link.attributes('href'))).toEqual(
        routeCase.hrefs,
      )
      expect(links.at(-1)!.attributes('aria-current')).toBe('page')
      expect(
        links.slice(0, -1).every((link) => !link.attributes('aria-current')),
      ).toBe(true)
    }
    expect(wrapper.find('.breadcrumb-root').exists()).toBe(false)
    expect(wrapper.find('.breadcrumb-separator-root').exists()).toBe(false)
    expect(wrapper.find('.breadcrumb-separator-workspace').exists()).toBe(true)
    expect(wrapper.get('.breadcrumb-workspace').text()).toBe('Workspace')
    await wrapper.get('.breadcrumb-workspace').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/w/w')
    await router.push('/t/t/v/v')
    expect(wrapper.get('.breadcrumb-separator-current').text()).toBe(
      i18n.global.t('nav.separator'),
    )
    useNavStore().tree!.workspaces[0]!.name = 'Renamed workspace'
    await flushPromises()
    expect(wrapper.get('.breadcrumb-workspace').text()).toBe(
      'Renamed workspace',
    )
    useNavStore().lastWorkspaceId = 'w'
    await router.push('/home')
    expect(wrapper.find('.breadcrumbs').exists()).toBe(false)
    await router.push('/profile')
    expect(wrapper.find('.breadcrumbs').exists()).toBe(false)
  } finally {
    wrapper.unmount()
  }
})

it('keeps path-only workspace and book ancestors in the breadcrumb', async () => {
  const { global } = await setup('/t/t')
  const workspace = useNavStore().tree!.workspaces[0]!
  workspace.pathOnly = true
  workspace.books[0]!.pathOnly = true
  const wrapper = mount(AppBreadcrumbs, { global })
  try {
    expect(crumbs(wrapper)).toEqual(['Workspace', 'Book', 'Table'])
    expect(wrapper.findAll('.breadcrumb-path-only')).toHaveLength(2)
    expect(wrapper.findAll('.breadcrumb-path-visible')).toHaveLength(1)
    expect(
      wrapper.findAll('.breadcrumbs a').map((link) => link.attributes('href')),
    ).toEqual(['/w/w', '/w/w/b/b', '/t/t'])
    expect(
      wrapper.findAll('.breadcrumbs a').at(-1)!.attributes('aria-current'),
    ).toBe('page')
  } finally {
    wrapper.unmount()
  }
})

it('quick-jumps to any workspace, workbook, table or view by name', async () => {
  const { router, global } = await setup('/home')
  const wrapper = mount(SearchPalette, { global, attachTo: document.body })
  try {
    document.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'k', ctrlKey: true }),
    )
    await flushPromises()
    expect(wrapper.get<HTMLDialogElement>('dialog').element.open).toBe(true)
    expect(wrapper.findAll('[role="option"]')).toHaveLength(4)
    const input = wrapper.get('input')
    await input.setValue('grid')
    expect(
      wrapper.findAll('[role="option"]').map((item) => item.text()),
    ).toEqual([expect.stringContaining('Grid')])
    await input.trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/t/t/v/v')
    expect(wrapper.get<HTMLDialogElement>('dialog').element.open).toBe(false)
  } finally {
    wrapper.unmount()
  }
})

it('opens the avatar menu with account links and shows admin links only to admins', async () => {
  const { router, global } = await setup('/home')
  const auth = useAuthStore()
  auth.setUser({ ...navUser })
  const wrapper = mount(UserMenu, {
    global: { ...global, stubs: { NotificationBell: true } },
    attachTo: document.body,
  })
  try {
    const button = wrapper.get('.avatar-button')
    expect(button.text()).toContain('A')
    expect(wrapper.get('.user-menu-panel').isVisible()).toBe(false)
    await button.trigger('click')
    expect(button.attributes('aria-expanded')).toBe('true')
    expect(
      wrapper.findAll('.user-menu-panel a').map((a) => a.attributes('href')),
    ).toEqual(['/profile', '/change-password', '/trash'])
    expect(wrapper.find('.user-submenu').exists()).toBe(false)
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(button.attributes('aria-expanded')).toBe('false')
    auth.setUser({ ...navUser, role: 'admin' })
    await flushPromises()
    expect(wrapper.find('.user-submenu').exists()).toBe(true)
    await button.trigger('click')
    await router.push('/profile')
    await flushPromises()
    expect(button.attributes('aria-expanded')).toBe('false')
  } finally {
    wrapper.unmount()
  }
})
