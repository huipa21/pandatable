import { afterEach, beforeEach, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { nextTick } from 'vue'
import { i18n, loadLocale } from '../../i18n'
import { useAuthStore } from '../../stores/auth'
import { useNavStore, type NavigationItem } from '../../stores/nav'
import { navigation, navUser } from '../../test/navigation'
import WorkspaceNavMenu from './WorkspaceNavMenu.vue'

let pinia: ReturnType<typeof createPinia>

beforeEach(async () => {
  pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().setUser(navUser)
  useNavStore().tree = navigation()
  await loadLocale('en')
  await loadLocale('zh-CN')
  i18n.global.locale.value = 'en'
})

afterEach(() => {
  disposePinia(pinia)
})

function mountMenu(workspace = useNavStore().items[0]!) {
  return mount(WorkspaceNavMenu, {
    props: { workspace },
    global: { plugins: [pinia, i18n] },
    attachTo: document.body,
  })
}

async function openMenu(wrapper: ReturnType<typeof mountMenu>) {
  const details = wrapper.get('details').element as HTMLDetailsElement
  details.open = true
  details.dispatchEvent(new Event('toggle'))
  await nextTick()
  return details
}

it('shows owner actions in order with the localized accessible name', async () => {
  const wrapper = mountMenu()
  try {
    expect(wrapper.get('summary').attributes('aria-label')).toBe(
      'Workspace actions for Workspace',
    )
    await openMenu(wrapper)
    expect(
      wrapper.findAll('.nav-actions button').map((button) => button.text()),
    ).toEqual(['New book', 'Edit'])
  } finally {
    wrapper.unmount()
  }
})

it('opens the create-book editor and closes the menu without navigating', async () => {
  const nav = useNavStore()
  const wrapper = mountMenu()
  try {
    const details = await openMenu(wrapper)
    await wrapper.get('.nav-actions button').trigger('click')
    await flushPromises()
    expect(nav.editor).toEqual({
      operation: 'create',
      kind: 'book',
      parentId: 'w',
    })
    expect(details.open).toBe(false)
  } finally {
    wrapper.unmount()
  }
})

it('opens the workspace editor with its current identity and name', async () => {
  const nav = useNavStore()
  const workspace = {
    ...nav.items[0]!,
    canCreate: false,
  }
  const wrapper = mountMenu(workspace)
  try {
    await openMenu(wrapper)
    expect(
      wrapper.findAll('.nav-actions button').map((button) => button.text()),
    ).toEqual(['Edit'])
    await wrapper.get('.nav-actions button').trigger('click')
    expect(nav.editor).toEqual({
      operation: 'rename',
      kind: 'workspace',
      id: 'w',
      name: 'Workspace',
    })
  } finally {
    wrapper.unmount()
  }
})

it.each([
  ['read', 'read', false],
  ['write', 'write', false],
  ['path-only', 'none', true],
] as const)('hides the menu for %s access', (_label, level, pathOnly) => {
  const owner = useNavStore().items[0]!
  const workspace: NavigationItem = {
    ...owner,
    level,
    pathOnly,
    canCreate: false,
    canEdit: false,
  }
  const wrapper = mountMenu(workspace)
  try {
    expect(wrapper.find('details').exists()).toBe(false)
  } finally {
    wrapper.unmount()
  }
})

it('moves between actions with arrow keys and restores focus on Escape', async () => {
  const wrapper = mountMenu()
  try {
    const details = await openMenu(wrapper)
    const summary = wrapper.get('summary')
    const buttons = wrapper.findAll('.nav-actions button')
    summary.element.focus()
    await summary.trigger('keydown', { key: 'ArrowDown' })
    expect(document.activeElement).toBe(buttons[0]!.element)
    await buttons[0]!.trigger('keydown', { key: 'ArrowDown' })
    expect(document.activeElement).toBe(buttons[1]!.element)
    await buttons[1]!.trigger('keydown', { key: 'ArrowUp' })
    expect(document.activeElement).toBe(buttons[0]!.element)
    buttons[0]!.element.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }),
    )
    expect(details.open).toBe(false)
    expect(document.activeElement).toBe(summary.element)
  } finally {
    wrapper.unmount()
  }
})

it('disables actions while a navigation mutation is busy', async () => {
  useNavStore().busy = true
  const wrapper = mountMenu()
  try {
    await openMenu(wrapper)
    expect(
      wrapper
        .findAll('.nav-actions button')
        .every((button) => button.attributes('disabled') !== undefined),
    ).toBe(true)
  } finally {
    wrapper.unmount()
  }
})
