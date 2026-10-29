import { afterEach, beforeEach, expect, it } from 'vitest'
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

it('renders linked labels as chips and exposes an edit control only when editable', async () => {
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
  await wrapper.get('button[aria-label="Edit links"]').trigger('click')
  expect(wrapper.emitted('edit')).toHaveLength(1)

  await wrapper.setProps({ links: [], editable: false })
  expect(wrapper.text()).toContain('No linked rows')
  expect(wrapper.find('button').exists()).toBe(false)
})
