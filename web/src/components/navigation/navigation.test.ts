import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { nextTick } from 'vue'
import { i18n, loadLocale } from '../../i18n'
import { useNavStore } from '../../stores/nav'
import { navigation } from '../../test/navigation'
import { themeMedia } from '../../test/setup'
import NavContents from './NavContents.vue'
import NavSidebar from './NavSidebar.vue'
import StructureDialog from './StructureDialog.vue'

let pinia: ReturnType<typeof createPinia>
beforeEach(async () => {
  pinia = createPinia()
  setActivePinia(pinia)
  await loadLocale('en')
  await loadLocale('zh-CN')
  i18n.global.locale.value = 'en'
  vi.stubGlobal('matchMedia', () => themeMedia)
  themeMedia.setDark(false)
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockImplementation(() => Promise.resolve(Response.json(navigation()))),
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
async function setup() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }],
  })
  await router.push('/home')
  await useNavStore().refresh()
  return { plugins: [pinia, i18n, router] }
}

it('renders nested links and exposes same-parent drag plus keyboard/touch reorder only for owners', async () => {
  const global = await setup()
  const nav = useNavStore()
  const tree = navigation()
  const b = tree.workspaces[0]!.books[0]!
  b.tables.push({ ...b.tables[0]!, id: 't2', name: 'Second' })
  nav.tree = tree
  const reorder = vi.spyOn(nav, 'reorder').mockResolvedValue()
  const wrapper = mount(NavContents, { global })
  try {
    expect(wrapper.findAll('[data-nav-id]')).toHaveLength(4)
    expect(
      wrapper.get('[data-nav-id="w"] > .nav-row').attributes('draggable'),
    ).toBe('false')
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
    expect(wrapper.findAll('summary')).toHaveLength(0)
    expect(wrapper.text()).toContain('Path only')
  } finally {
    wrapper.unmount()
  }
})

it('validates code points, retains localized mutation errors and closes confirmed dialogs', async () => {
  const global = await setup()
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

it('refreshes on focus/visible interval, stops hidden/unmounted and closes the mobile drawer on Escape', async () => {
  const global = await setup()
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
    await wrapper.get('.drawer-toggle').trigger('click')
    await flushPromises()
    expect(wrapper.get<HTMLDialogElement>('.nav-drawer').element.open).toBe(
      true,
    )
    await wrapper.get('.nav-drawer').trigger('cancel')
    expect(wrapper.get<HTMLDialogElement>('.nav-drawer').element.open).toBe(
      false,
    )
  } finally {
    wrapper.unmount()
    vi.restoreAllMocks()
  }
  const count = refresh.mock.calls.length
  await vi.advanceTimersByTimeAsync(60_000)
  window.dispatchEvent(new Event('focus'))
  expect(refresh).toHaveBeenCalledTimes(count)
})
