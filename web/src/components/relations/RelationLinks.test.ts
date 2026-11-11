import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { i18n, loadLocale } from '../../i18n'
import RelationLinks from './RelationLinks.vue'

let wrapper: VueWrapper

beforeEach(async () => {
  await loadLocale('en')
  i18n.global.locale.value = 'en'
})

afterEach(() => {
  wrapper?.unmount()
})

it('renders stable colored tags and emits only the selected unlink action', async () => {
  wrapper = mount(RelationLinks, {
    props: {
      links: [
        { id: '1', label: 'Alpha' },
        { id: '2', label: 'Beta' },
        { id: 'invalid', label: 4 },
      ],
      editable: true,
    },
    global: { plugins: [i18n] },
  })

  expect(wrapper.findAll('.relation-chip').map((chip) => chip.text())).toEqual([
    'Alpha',
    'Beta',
  ])
  const chips = wrapper.findAll('.relation-chip')
  const colors = chips.map((chip) => chip.attributes('style'))
  expect(colors[0]).toContain('--relation-color:')
  expect(colors[0]).not.toBe(colors[1])
  expect(chips[0]!.attributes('title')).toBe('Alpha')

  const parentClick = vi.fn()
  wrapper.element.addEventListener('click', parentClick)
  await wrapper.get('button[aria-label="Unlink “Alpha”"]').trigger('click')
  expect(wrapper.emitted('remove')).toEqual([[{ id: '1', label: 'Alpha' }]])
  expect(wrapper.emitted('edit')).toBeUndefined()
  expect(parentClick).not.toHaveBeenCalled()

  await wrapper.get('button[aria-label="Edit links"]').trigger('click')
  expect(wrapper.emitted('edit')).toHaveLength(1)
  expect(parentClick).not.toHaveBeenCalled()

  await wrapper.setProps({
    links: [
      { id: '2', label: 'Beta renamed' },
      { id: '1', label: 'Alpha' },
    ],
  })
  expect(
    wrapper.findAll('.relation-chip').map((chip) => chip.attributes('style')),
  ).toEqual([colors[1], colors[0]])
})

it('hides write controls for readers and disables them while saving', async () => {
  wrapper = mount(RelationLinks, {
    props: { links: [{ id: '1', label: 'Alpha' }], editable: true },
    global: { plugins: [i18n] },
  })
  await wrapper.setProps({ disabled: true })
  expect(
    wrapper.get('button[aria-label="Unlink “Alpha”"]').attributes('disabled'),
  ).toBeDefined()
  expect(
    wrapper.get('button[aria-label="Edit links"]').attributes('disabled'),
  ).toBeDefined()
  await wrapper.get('button[aria-label="Unlink “Alpha”"]').trigger('click')
  expect(wrapper.emitted('remove')).toBeUndefined()

  await wrapper.setProps({ editable: false, disabled: false })
  expect(wrapper.get('.relation-chip').text()).toBe('Alpha')
  expect(wrapper.find('button').exists()).toBe(false)
  await wrapper.setProps({ links: [] })
  expect(wrapper.text()).toContain('No linked rows')
  expect(wrapper.find('button').exists()).toBe(false)
})
