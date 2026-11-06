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

it('lists workspaces flat in server order and keeps selection, default, and owner actions distinct', async () => {
  const nav = useNavStore()
  const tree = navigation()
  const first = tree.workspaces[0]!
  tree.workspaces.push(
    { ...first, id: 'w2', name: 'Current workspace' },
    {
      ...first,
      id: 'w3',
      name: 'Path context',
      level: 'none',
      pathOnly: true,
    },
  )
  nav.tree = tree
  nav.loaded = true
  nav.preferenceLoaded = true
  nav.preferredWorkspaceId = first.id

  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/w/:workspaceId', component: { template: '<div />' } }],
  })
  await router.push('/w/w2')
  const wrapper = mount(NavContents, {
    global: { plugins: [pinia, i18n, router] },
  })

  try {
    const entries = wrapper.findAll('.workspace-root > li')
    expect(entries.map((entry) => entry.attributes('data-nav-id'))).toEqual([
      'w',
      'w2',
      'w3',
    ])
    expect(
      wrapper.findAll('[data-kind="book"], [data-kind="table"]'),
    ).toHaveLength(0)
    expect(wrapper.find('.workspace-toggle').exists()).toBe(false)
    expect(wrapper.find('.workspace-section').exists()).toBe(false)
    expect(wrapper.find('.view-list').exists()).toBe(false)
    expect(wrapper.text()).not.toContain(i18n.global.t('nav.emptyBooks'))

    expect(
      wrapper.get('[data-nav-id="w2"] > .workspace-box').classes(),
    ).toContain('current')
    expect(
      wrapper
        .get('[data-nav-id="w2"] .workspace-link')
        .attributes('aria-current'),
    ).toBe('page')
    expect(
      wrapper
        .get('[data-nav-id="w"] .workspace-default')
        .attributes('aria-label'),
    ).toBe('Default workspace: Workspace')
    expect(wrapper.find('[data-nav-id="w2"] .workspace-default').exists()).toBe(
      false,
    )

    const actions = wrapper.get('[data-nav-id="w2"] > .node-actions')
    expect(actions.text()).toContain('Share')
    expect(actions.text()).toContain('New book')
    const createBook = actions
      .findAll('button')
      .find((button) => button.text() === 'New book')
    expect(createBook).toBeDefined()
    await createBook!.trigger('click')
    expect(nav.editor).toEqual({
      operation: 'create',
      kind: 'book',
      parentId: 'w2',
    })

    expect(wrapper.find('[data-nav-id="w3"] > .node-actions').exists()).toBe(
      false,
    )
    expect(wrapper.get('[data-nav-id="w3"] .path-only').text()).toBe(
      'Path only',
    )

    await wrapper.get('[data-nav-id="w"] .workspace-link').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/w/w')
    expect(wrapper.emitted('navigate')).toHaveLength(1)
  } finally {
    wrapper.unmount()
  }
})
