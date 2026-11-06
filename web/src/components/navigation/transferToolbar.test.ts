import { afterEach, beforeEach, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { i18n, loadLocale } from '../../i18n'
import { tableSchema } from '../../test/navigation'
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

function mountToolbar(level: 'read' | 'write' | 'owner' = 'owner') {
  const schema = tableSchema()
  schema.myLevel = level
  return mount(TableToolbarActions, {
    props: {
      schema,
      view: schema.views[0],
      identity: 'alice',
      csvReady: true,
      exportBusy: false,
      exportError: '',
    },
    global: { plugins: [i18n] },
    attachTo: document.body,
  })
}

it('uses one localized transfer trigger and emits import/export actions', async () => {
  const wrapper = mountToolbar()
  try {
    const trigger = wrapper.get('.transfer-menu > summary')
    expect(wrapper.findAll('.transfer-menu')).toHaveLength(1)
    expect(wrapper.find('.schema-summary').exists()).toBe(false)
    expect(trigger.attributes('aria-label')).toBe('Import or export')

    await trigger.trigger('click')
    await wrapper.get('.view-export-panel button').trigger('click')
    expect(wrapper.emitted('import')).toHaveLength(1)
    expect(
      (wrapper.get('.transfer-menu').element as HTMLDetailsElement).open,
    ).toBe(false)
    expect(document.activeElement).toBe(trigger.element)

    await trigger.trigger('click')
    await wrapper.get('.view-export-panel button:nth-child(2)').trigger('click')
    expect(wrapper.emitted('export')).toEqual([['csv']])
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
    expect(menu.text()).not.toContain('Import into this table')
    expect(menu.findAll('button')).toHaveLength(2)

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
