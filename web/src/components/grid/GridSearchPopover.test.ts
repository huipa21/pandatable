import { afterEach, beforeEach, expect, it } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
import { i18n, loadLocale } from '../../i18n'
import GridSearchPopover from './GridSearchPopover.vue'
import type { QuickSearch, QuickSearchField } from './quickSearch'

let wrapper: VueWrapper | undefined

beforeEach(async () => {
  await loadLocale('en')
  i18n.global.locale.value = 'en'
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
})

const fields: QuickSearchField[] = [
  { id: 'name', name: 'Name', uiType: 'text' },
  { id: 'amount', name: 'Amount', uiType: 'integer' },
]

function mountPopover(modelValue: QuickSearch = { term: '', fieldId: null }) {
  wrapper = mount(GridSearchPopover, {
    props: {
      fields,
      modelValue,
      'onUpdate:modelValue': (value: QuickSearch) =>
        wrapper!.setProps({ modelValue: value }),
    },
    global: { plugins: [i18n] },
    attachTo: document.body,
  })
  return wrapper
}

function emitted(view: VueWrapper) {
  return (view.emitted('update:modelValue') ?? []).map(
    ([value]) => value as QuickSearch,
  )
}

it('lists all fields first and emits a field-scoped search term', async () => {
  const view = mountPopover()
  const details = view.get('details').element as HTMLDetailsElement
  details.open = true
  await view.get('.grid-search-field-button').trigger('click')
  const options = view.findAll('[role="option"]')
  expect(options.map((option) => option.text())).toEqual([
    i18n.global.t('gridPolish.gp10.allFields'),
    'Name',
    'Amount',
  ])
  expect(options[0]!.attributes('aria-selected')).toBe('true')
  expect(options.every((option) => option.find('svg').exists())).toBe(true)

  await options[1]!.trigger('click')
  await view.get('input[type="search"]').setValue('us')
  expect(emitted(view).at(-1)).toEqual({ term: 'us', fieldId: 'name' })
  expect(view.get('.grid-search-field-name').text()).toBe('Name')
  expect(view.find('.grid-search-dot').exists()).toBe(true)
})

it('chooses fields by keyboard and keeps the popover open on list Escape', async () => {
  const view = mountPopover()
  const details = view.get('details').element as HTMLDetailsElement
  details.open = true
  await view.get('.grid-search-field-button').trigger('click')
  const list = view.get('[role="listbox"]')
  await list.trigger('keydown', { key: 'End' })
  await list.trigger('keydown', { key: 'Enter' })
  expect(emitted(view).at(-1)).toEqual({ term: '', fieldId: 'amount' })

  await view.get('.grid-search-field-button').trigger('click')
  await view.get('[role="listbox"]').trigger('keydown', { key: 'Escape' })
  expect(view.get('[role="listbox"]').isVisible()).toBe(false)
  expect(details.open).toBe(true)
})

it('closes on Escape and clears only the term with the clear button', async () => {
  const view = mountPopover({ term: 'acme', fieldId: 'name' })
  const details = view.get('details').element as HTMLDetailsElement
  details.open = true
  await view.get('.grid-search-clear').trigger('click')
  expect(emitted(view).at(-1)).toEqual({ term: '', fieldId: 'name' })
  await nextTick()
  expect(view.find('.grid-search-clear').exists()).toBe(false)
  expect(view.find('.grid-search-dot').exists()).toBe(false)

  await view.get('input[type="search"]').trigger('keydown', { key: 'Escape' })
  expect(details.open).toBe(false)
  expect(document.activeElement).toBe(view.get('summary').element)
})
