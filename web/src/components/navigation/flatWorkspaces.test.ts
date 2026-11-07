import { afterEach, beforeEach, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { i18n, loadLocale } from '../../i18n'
import { useAuthStore } from '../../stores/auth'
import { useNavStore } from '../../stores/nav'
import { navigation, navUser } from '../../test/navigation'
import NavContents from './NavContents.vue'

let pinia: ReturnType<typeof createPinia>

beforeEach(async () => {
  pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().setUser(navUser)
  await loadLocale('en')
  await loadLocale('zh-CN')
  i18n.global.locale.value = 'en'
})

afterEach(() => {
  disposePinia(pinia)
})

it('shows only the dashboard link and current workspace context', async () => {
  const nav = useNavStore()
  const tree = navigation()
  tree.workspaces.push(
    { ...tree.workspaces[0]!, id: 'w2', name: 'Sibling workspace' },
    {
      ...tree.workspaces[0]!,
      id: 'w3',
      name: 'Path context',
      level: 'none',
      pathOnly: true,
    },
  )
  nav.tree = tree
  nav.loaded = true

  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/home', component: { template: '<div />' } },
      { path: '/w/:workspaceId', component: { template: '<div />' } },
      { path: '/w/:wid/b/:bookId', component: { template: '<div />' } },
      { path: '/t/:tableId', component: { template: '<div />' } },
      { path: '/t/:tableId/v/:viewId', component: { template: '<div />' } },
    ],
  })
  await router.push('/home')
  const wrapper = mount(NavContents, {
    global: { plugins: [pinia, i18n, router] },
  })

  try {
    const home = wrapper.get('.workspace-root li[data-kind="dashboard"] a')
    expect(home.text()).toBe('Workspaces')
    expect(home.attributes('href')).toBe('/home')
    expect(home.attributes('aria-current')).toBe('page')
    expect(wrapper.find('.workspace-heading').exists()).toBe(false)
    expect(
      wrapper.find('.workspace-root li[data-kind="workspace"]').exists(),
    ).toBe(false)
    expect(wrapper.text()).not.toContain('Sibling workspace')
    expect(wrapper.find('.node-actions').exists()).toBe(false)

    await router.push('/w/w')
    await flushPromises()
    const entries = wrapper.findAll('.workspace-root > li')
    expect(entries).toHaveLength(2)
    expect(entries[0]!.text()).toBe('Workspaces')
    expect(entries[1]!.attributes('data-nav-id')).toBe('w')
    expect(entries[1]!.get('.workspace-link').text()).toContain('Workspace')
    expect(entries[1]!.find('.workspace-link .app-icon').exists()).toBe(true)
    expect(entries[1]!.get('.workspace-link').attributes('aria-current')).toBe(
      'page',
    )
    expect(wrapper.text()).not.toContain('Sibling workspace')
    expect(wrapper.text()).not.toContain('Path context')
    expect(wrapper.find('.workbook-root').exists()).toBe(false)

    await router.push('/w/w/b/b')
    await flushPromises()
    expect(wrapper.findAll('.workspace-root > li')).toHaveLength(2)
    expect(
      wrapper
        .get('.workspace-root [data-nav-id="w"] .workspace-link')
        .attributes('aria-current'),
    ).toBe('location')
    expect(wrapper.get('.workbook-root').attributes('data-context-book')).toBe(
      'b',
    )
    expect(wrapper.find('.workspace-root [data-kind="book"]').exists()).toBe(
      false,
    )

    await wrapper
      .get('.workspace-root li[data-kind="dashboard"] a')
      .trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/home')
    expect(wrapper.emitted('navigate')).toHaveLength(1)
  } finally {
    wrapper.unmount()
  }
})
