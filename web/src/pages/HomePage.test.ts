import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { i18n, loadLocale } from '../i18n'
import type { NavWorkspace } from '../api/types'
import { useAuthStore } from '../stores/auth'
import { useNavStore } from '../stores/nav'
import { navUser, navigation } from '../test/navigation'
import HomePage from './HomePage.vue'

let pinia: ReturnType<typeof createPinia>

beforeEach(async () => {
  pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().setUser(navUser)
  await loadLocale('en')
  i18n.global.locale.value = 'en'
})

afterEach(() => {
  disposePinia(pinia)
  vi.restoreAllMocks()
})

function workspace(overrides: Partial<NavWorkspace> = {}): NavWorkspace {
  return {
    ...navigation().workspaces[0]!,
    books: [],
    ...overrides,
  }
}

async function setup() {
  const page = { template: '<div />' }
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/home', component: HomePage },
      { path: '/w/:workspaceId', component: page },
      { path: '/t/:tableId/v/:viewId/r/:rowId', component: page },
    ],
  })
  await router.push('/home')
  const wrapper = mount(
    { template: '<RouterView />' },
    { global: { plugins: [pinia, i18n, router] } },
  )
  return { router, wrapper }
}

function mockResolution(workspaceId = 'w') {
  const nav = useNavStore()
  const resolve = vi
    .spyOn(nav, 'resolvePreference')
    .mockImplementation(async () => {
      nav.preferredWorkspaceId = workspaceId
      nav.preferenceLoaded = true
      return workspaceId
    })
  const refresh = vi.spyOn(nav, 'refresh').mockImplementation(async () => {
    nav.loaded = true
    nav.error = null
    return true
  })
  return { nav, resolve, refresh }
}

it('keeps /home on the workspace dashboard and orders rows by recency then default', async () => {
  const { nav, resolve, refresh } = mockResolution('default')
  nav.tree.workspaces = [
    workspace({ id: 'default', name: 'Default workspace' }),
    workspace({
      id: 'older',
      name: 'Older workspace',
      lastOpenedAt: '2025-01-01T00:00:00Z',
      fallbackIcon: 'star',
    }),
    workspace({
      id: 'recent',
      name: 'Recent workspace',
      lastOpenedAt: '2025-02-01T00:00:00Z',
      icon: 'book',
      description: 'Recent description',
    }),
  ]
  const { router, wrapper } = await setup()
  try {
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/home')
    expect(resolve).toHaveBeenCalledTimes(1)
    expect(refresh).toHaveBeenCalledWith(true)
    const rows = wrapper.findAll('.workspace-dashboard-row')
    expect(rows.map((row) => row.attributes('data-nav-id'))).toEqual([
      'recent',
      'older',
      'default',
    ])
    expect(rows[0]!.text()).toContain('Recent description')
    expect(
      rows[0]!.find('.workspace-dashboard-description').attributes('title'),
    ).toBe('Recent description')
    expect(rows[0]!.find('svg').exists()).toBe(true)
    expect(
      rows[2]!.find('.workspace-dashboard-default').attributes('aria-label'),
    ).toBe('Default workspace: Default workspace')
  } finally {
    wrapper.unmount()
  }
})

it('searches names and descriptions and offers a clear action for no matches', async () => {
  const { nav } = mockResolution()
  nav.loaded = true
  nav.tree.workspaces = [
    workspace({
      id: 'name-match',
      name: '东京项目',
      description: 'Planning',
    }),
    workspace({
      id: 'description-match',
      name: 'Other project',
      description: '东京团队',
    }),
    workspace({ id: 'hidden-from-query', name: 'Unrelated' }),
  ]
  const { wrapper } = await setup()
  try {
    await flushPromises()
    const search = wrapper.get('input[type="search"]')
    await search.setValue('团队')
    expect(
      wrapper
        .findAll('.workspace-dashboard-row')
        .map((row) => row.attributes('data-nav-id')),
    ).toEqual(['description-match'])
    await search.setValue('missing')
    expect(wrapper.text()).toContain('No workspaces match your search.')
    await wrapper.get('.resource-search-clear').trigger('click')
    expect((search.element as HTMLInputElement).value).toBe('')
    expect(wrapper.findAll('.workspace-dashboard-row')).toHaveLength(3)
  } finally {
    wrapper.unmount()
  }
})

it('shows accessible path-only rows without descriptions or actions', async () => {
  const { nav } = mockResolution()
  nav.loaded = true
  nav.tree.workspaces = [
    workspace({
      id: 'path',
      name: 'Shared context',
      description: 'Must not be exposed',
      pathOnly: true,
      level: 'read',
    }),
    workspace({
      id: 'reader',
      name: 'Reader workspace',
      level: 'read',
    }),
  ]
  const { wrapper } = await setup()
  try {
    await flushPromises()
    const pathRow = wrapper.get('[data-nav-id="path"]')
    expect(pathRow.text()).toContain('Path only')
    expect(pathRow.text()).not.toContain('Must not be exposed')
    expect(pathRow.find('.node-actions').exists()).toBe(false)
    const readerActions = wrapper.get('[data-nav-id="reader"] .node-actions')
    await readerActions.get('summary').trigger('click')
    expect(readerActions.text()).toContain('Set as default')
    expect(readerActions.text()).not.toContain('Share')
    expect(readerActions.text()).not.toContain('Delete')
    expect(readerActions.text()).not.toContain('Create book')
  } finally {
    wrapper.unmount()
  }
})

it('opens the workspace create dialog and shows an empty-state call to action', async () => {
  const { nav } = mockResolution()
  nav.loaded = true
  const { wrapper } = await setup()
  try {
    await flushPromises()
    expect(wrapper.text()).toContain('No workspaces yet.')
    await wrapper.get('button').trigger('click')
    expect(nav.editor).toEqual({
      operation: 'create',
      kind: 'workspace',
    })
  } finally {
    wrapper.unmount()
  }
})

it('keeps stale rows visible and lets the user retry navigation', async () => {
  const { nav, refresh } = mockResolution()
  nav.tree.workspaces = [workspace()]
  nav.error = new TypeError('offline')
  refresh.mockImplementationOnce(async () => {
    nav.loaded = true
    return true
  })
  const { wrapper } = await setup()
  try {
    await flushPromises()
    expect(wrapper.text()).toContain('Navigation could not be refreshed.')
    expect(wrapper.findAll('.workspace-dashboard-row')).toHaveLength(1)
    await wrapper.get('.dashboard-stale button').trigger('click')
    await flushPromises()
    expect(nav.error).toBeNull()
    expect(wrapper.find('.dashboard-stale').exists()).toBe(false)
  } finally {
    wrapper.unmount()
  }
})

it('keeps resolution errors retryable without navigating away from /home', async () => {
  const nav = useNavStore()
  const resolve = vi
    .spyOn(nav, 'resolvePreference')
    .mockRejectedValueOnce(new TypeError('private server detail'))
    .mockImplementationOnce(async () => 'w')
  vi.spyOn(nav, 'refresh').mockResolvedValue(true)
  const { router, wrapper } = await setup()
  try {
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/home')
    expect(wrapper.get('[role="alert"]').text()).not.toContain(
      'private server detail',
    )
    expect(wrapper.get('[role="alert"]').text()).toContain(
      'Unable to prepare your workspace dashboard',
    )
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(resolve).toHaveBeenCalledTimes(2)
    expect(router.currentRoute.value.path).toBe('/home')
  } finally {
    wrapper.unmount()
  }
})

it('aborts resolution when an explicit resource route starts', async () => {
  const nav = useNavStore()
  let releaseResolve!: (id: string) => void
  const resolve = vi.spyOn(nav, 'resolvePreference').mockImplementation(
    () =>
      new Promise<string>((done) => {
        releaseResolve = done
      }),
  )
  const refresh = vi.spyOn(nav, 'refresh').mockResolvedValue(true)
  const { router, wrapper } = await setup()
  try {
    await flushPromises()
    expect(wrapper.get('[role="status"]').text()).toBe(
      'Preparing your workspace dashboard',
    )
    const navigationPromise = router.push('/w/explicit')
    await flushPromises()
    expect(resolve.mock.calls[0]![0]?.aborted).toBe(true)
    releaseResolve('default')
    await navigationPromise
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/w/explicit')
    expect(refresh).not.toHaveBeenCalled()
  } finally {
    wrapper.unmount()
  }
})
