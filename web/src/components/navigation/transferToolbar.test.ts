import { afterEach, beforeEach, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { i18n, loadLocale } from '../../i18n'
import { tableSchema } from '../../test/navigation'
import AppIcon from '../AppIcon.vue'
import TableToolbarActions from './TableToolbarActions.vue'

let pinia: ReturnType<typeof createPinia>

beforeEach(async () => {
  pinia = createPinia()
  setActivePinia(pinia)
  await loadLocale('en')
  i18n.global.locale.value = 'en'
})

afterEach(() => {
  disposePinia(pinia)
  document.body.innerHTML = ''
})

function mountToolbar(level: 'none' | 'read' | 'write' | 'owner' = 'owner') {
  const schema = tableSchema()
  schema.myLevel = level
  return mount(TableToolbarActions, {
    props: {
      schema,
      view: schema.views[0],
      identity: 'alice',
      exportBusy: false,
      exportError: '',
    },
    global: { plugins: [i18n] },
    attachTo: document.body,
  })
}

it('shows one icon-and-text trigger and exactly two XLSX menu actions', async () => {
  const wrapper = mountToolbar()
  try {
    const trigger = wrapper.get('.transfer-menu > summary')
    expect(wrapper.findAll('.transfer-menu')).toHaveLength(1)
    expect(wrapper.find('.schema-summary').exists()).toBe(false)
    expect(trigger.attributes('aria-label')).toBe('Import/Export')
    expect(trigger.text()).toContain('Import/Export')
    expect(wrapper.findComponent(AppIcon).props('name')).toBe('transfer')

    await trigger.trigger('click')
    const buttons = wrapper.findAll('.view-export-panel button')
    expect(buttons.map((button) => button.text())).toEqual(['Import', 'Export'])
    expect(wrapper.get('.view-export-panel').text()).not.toContain('CSV')
    expect(wrapper.get('.view-export-panel').text()).not.toContain('package')
    expect(
      wrapper.findAllComponents(AppIcon).map((icon) => icon.props('name')),
    ).toEqual(['transfer', 'import', 'export'])

    await buttons[0]!.trigger('click')
    expect(wrapper.emitted('import')).toHaveLength(1)
    expect(
      (wrapper.get('.transfer-menu').element as HTMLDetailsElement).open,
    ).toBe(false)
    expect(document.activeElement).toBe(trigger.element)

    await trigger.trigger('click')
    await wrapper.get('.view-export-panel button:nth-child(2)').trigger('click')
    expect(wrapper.emitted('export')).toEqual([['xlsx']])
    expect(
      wrapper.get('.view-export-panel button').attributes('disabled'),
    ).toBe(undefined)
  } finally {
    wrapper.unmount()
  }
})

it('hides import from readers and closes on Escape or outside pointerdown', async () => {
  const wrapper = mountToolbar('read')
  try {
    const menu = wrapper.get('.transfer-menu')
    const trigger = menu.get('summary')
    await trigger.trigger('click')
    expect(menu.findAll('button').map((button) => button.text())).toEqual([
      'Export',
    ])

    await menu.trigger('keydown', { key: 'Escape' })
    expect((menu.element as HTMLDetailsElement).open).toBe(false)
    expect(document.activeElement).toBe(trigger.element)

    await trigger.trigger('click')
    document.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }))
    expect((menu.element as HTMLDetailsElement).open).toBe(false)
  } finally {
    wrapper.unmount()
  }
})

it('does not expose transfer actions without read access', () => {
  const wrapper = mountToolbar('none')
  try {
    expect(wrapper.find('.transfer-menu').exists()).toBe(false)
  } finally {
    wrapper.unmount()
  }
})
