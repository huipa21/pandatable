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
    expect(crumbs(wrapper)).toEqual(['Workspace', 'Book', 'Table'])
    expect(
      wrapper.findAll('.breadcrumbs a').at(-1)!.attributes('aria-current'),
    ).toBe('page')
    expect(wrapper.get('.breadcrumb-workspace').text()).toBe('Workspace')
    await router.push('/t/t/v/v')
    expect(crumbs(wrapper)).toEqual(['Workspace', 'Book', 'Table', 'Grid'])
    expect(wrapper.get('.breadcrumb-separator-current').text()).toBe(
      i18n.global.t('nav.separator'),
    )
    await router.push('/w/w/b/b')
    expect(crumbs(wrapper)).toEqual(['Workspace', 'Book'])
    await router.push('/w/w')
    expect(crumbs(wrapper)).toEqual(['Workspace'])
    useNavStore().tree!.workspaces[0]!.name = 'Renamed workspace'
    await flushPromises()
    expect(wrapper.get('.breadcrumb-workspace').text()).toBe(
      'Renamed workspace',
    )
    await router.push('/home')
    expect(wrapper.find('.breadcrumbs').exists()).toBe(false)
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
