import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { i18n, loadLocale } from '../../i18n'
import { errorMessage, RequestError } from '../../api/client'
import type { DirectGrant, GrantListing } from '../../api/types'
import { useNavStore } from '../../stores/nav'
import { useSharingStore } from '../../stores/sharing'
import { useAuthStore } from '../../stores/auth'
import { navigation, navUser } from '../../test/navigation'
import ShareDialog from './ShareDialog.vue'
import DirectGrantRow from './DirectGrantRow.vue'
import NavContents from '../navigation/NavContents.vue'

const grant = (): DirectGrant => ({
  id: 'g',
  resourceType: 'table',
  resourceId: 't',
  principalType: 'user',
  principalId: 'bob',
  level: 'read',
  grantedBy: null,
  createdAt: '2026-10-26T00:00:00Z',
  principal: { name: 'Bob <script>', username: 'bob', status: 'active' },
  inheritedLevel: 'write',
})
const listing = (): GrantListing => ({
  resource: { type: 'table', id: 't' },
  effectiveLevel: 'owner',
  items: [grant()],
  inherited: [
    {
      ...grant(),
      id: 'parent',
      resourceType: 'workspace',
      resourceId: 'w',
      level: 'write',
    },
  ],
})
let pinia: ReturnType<typeof createPinia>
beforeEach(async () => {
  pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().setUser(navUser)
  useNavStore().tree = navigation()
  useNavStore().loaded = true
  await loadLocale('en')
  await loadLocale('zh-CN')
  i18n.global.locale.value = 'en'
  HTMLDialogElement.prototype.showModal = function () {
    this.open = true
  }
  HTMLDialogElement.prototype.close = function () {
    this.open = false
  }
  vi.stubGlobal(
    'fetch',
    vi.fn(() => Promise.resolve(Response.json(listing()))),
  )
})
afterEach(() => {
  disposePinia(pinia)
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  i18n.global.locale.value = 'en'
})
async function setup() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/w/:workspaceId', component: { template: '<div />' } },
      { path: '/t/:tableId/v/:viewId', component: { template: '<div />' } },
      { path: '/:pathMatch(.*)*', component: { template: '<div />' } },
    ],
  })
  await router.push('/t/t/v/v')
  return { plugins: [pinia, i18n, router] }
}

it('renders distinct direct/inherited sources and warning, reviews removals without issuing a mutation, supports Escape', async () => {
  const wrapper = mount(ShareDialog, {
    global: await setup(),
    attachTo: document.body,
  })
  const share = useSharingStore()
  try {
    share.open({ kind: 'table', id: 't', name: '<script>' })
    await flushPromises()
    expect(wrapper.get('dialog').element.open).toBe(true)
    expect(wrapper.find('script').exists()).toBe(false)
    expect(wrapper.text()).toContain('Inherited level is higher (Write)')
    expect(wrapper.text()).toContain('Workspace w: Write')
    expect(wrapper.get('.share-search select').attributes('aria-label')).toBe(
      'Principal type',
    )
    expect(
      wrapper.findAll('[aria-label="Inherited grants (read-only)"] button'),
    ).toHaveLength(0)
    await wrapper
      .get('[data-principal-id="bob"] button:last-child')
      .trigger('click')
    expect(share.review).toMatchObject({ removeId: 'g' })
    await flushPromises()
    expect(document.activeElement).toBe(wrapper.get('h3').element)
    expect(wrapper.text()).toContain('still inherits Write')
    expect(vi.mocked(fetch)).toHaveBeenCalledTimes(1)
    i18n.global.locale.value = 'zh-CN'
    await flushPromises()
    expect(wrapper.text()).toContain('移除 Bob <script> 的直接授权')
    await wrapper.get('.share-review button:last-child').trigger('click')
    expect(share.review).toBeNull()
    await wrapper.get('dialog').trigger('cancel')
    await flushPromises()
    expect(wrapper.get('dialog').element.open).toBe(false)
  } finally {
    wrapper.unmount()
  }
})

it('labels dangling/disabled grants honestly and permits removal but not replacement', async () => {
  const global = await setup()
  for (const principal of [
    null,
    { name: 'Disabled', username: 'disabled', status: 'disabled' as const },
  ]) {
    const wrapper = mount(DirectGrantRow, {
      props: { grant: { ...grant(), principal }, disabled: false },
      global,
    })
    try {
      expect(wrapper.get('select').attributes('disabled')).toBeDefined()
      expect(wrapper.findAll('button')[0]!.attributes('disabled')).toBeDefined()
      expect(
        wrapper.findAll('button')[1]!.attributes('disabled'),
      ).toBeUndefined()
      expect(wrapper.text()).toContain(
        principal ? 'Disabled' : 'Missing principal (bob)',
      )
    } finally {
      wrapper.unmount()
    }
  }
})

it('offers Share to book/table effective owners independently of structure ownership, never to path-only/read/write/admin-only', async () => {
  const global = await setup()
  const nav = useNavStore()
  const tree = navigation()
  tree.workspaces[0]!.level = 'none'
  tree.workspaces[0]!.pathOnly = true
  const b = tree.workspaces[0]!.books[0]!
  b.level = 'owner'
  nav.tree = tree
  const wrapper = mount(NavContents, { global })
  try {
    expect(
      wrapper.get('.workbook-root [data-nav-id="b"] > .node-actions').text(),
    ).toContain('Share')
    expect(
      wrapper.get('.workbook-root [data-nav-id="b"] > .node-actions').text(),
    ).not.toContain('Rename')
    b.level = 'none'
    b.pathOnly = true
    nav.tree = structuredClone(tree)
    await flushPromises()
    expect(
      wrapper.get('.workbook-root [data-nav-id="t"] > .node-actions').text(),
    ).toContain('Share')
    expect(
      wrapper.get('.workbook-root [data-nav-id="t"] > .node-actions').text(),
    ).not.toContain('Rename')
    for (const level of ['read', 'write'] as const) {
      b.tables[0]!.level = level
      nav.tree = structuredClone(tree)
      await flushPromises()
      expect(
        wrapper
          .get('.workbook-root [data-nav-id="t"] > .node-actions')
          .findAll('button')
          .map((button) => button.text()),
      ).toEqual(['Create view'])
    }
    useAuthStore().setUser({ ...navUser, role: 'admin', id: 'admin' })
    await flushPromises()
    expect(wrapper.findAll('.node-actions')).toHaveLength(0)
  } finally {
    wrapper.unmount()
  }
})

it('shows localized last-owner/disabled/missing and redacted failures with retry, not server messages', async () => {
  const wrapper = mount(ShareDialog, { global: await setup() })
  const share = useSharingStore()
  try {
    vi.mocked(fetch).mockRejectedValue(new Error('private server detail'))
    share.open({ kind: 'workspace', id: 'w', name: 'Workspace' })
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).not.toContain(
      'private server detail',
    )
    expect(wrapper.get('[role="alert"] button').text()).toBe('Try again')
    expect(wrapper.find('.share-search').exists()).toBe(false)
    for (const locale of ['en', 'zh-CN'] as const) {
      i18n.global.locale.value = locale
      for (const code of [
        'ERR_LAST_OWNER',
        'ERR_PRINCIPAL_DISABLED',
        'ERR_PRINCIPAL_NOT_FOUND',
      ])
        expect(errorMessage(new RequestError(code, 409))).not.toBe(
          errorMessage(new Error()),
        )
    }
    vi.mocked(fetch).mockImplementation(() =>
      Promise.resolve(Response.json(listing())),
    )
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(wrapper.find('.share-search').exists()).toBe(true)
  } finally {
    wrapper.unmount()
  }
})
