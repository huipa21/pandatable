import { afterEach, expect, it } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { i18n, loadLocale } from '../../i18n'
import type { FilterNode } from '../../api/types'
import { tableSchema } from '../../test/navigation'
import FilterBuilder from './FilterBuilder.vue'

let wrapper: VueWrapper | undefined

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
})

async function setup() {
  await loadLocale('en')
  i18n.global.locale.value = 'en'
  const schema = tableSchema()
  schema.fields.push(
    {
      ...schema.fields[0]!,
      id: 'status',
      name: 'Status',
      uiType: 'single_select',
      options: {
        choices: [
          { id: 'open', label: 'Open', color: 'green' },
          { id: 'blocked', label: 'Blocked', color: 'red' },
        ],
      },
      filterOps: [
        'is',
        'is_not',
        'is_any_of',
        'is_none_of',
        'is_empty',
        'is_not_empty',
      ],
      position: 100,
    },
    {
      ...schema.fields[0]!,
      id: 'due',
      name: 'Due',
      uiType: 'date',
      filterOps: ['is', 'is_not', 'before', 'after', 'between', 'is_within'],
      position: 200,
    },
    {
      ...schema.fields[0]!,
      id: 'secret',
      name: 'Hidden computed',
      uiType: 'formula',
      filterable: false,
      filterOps: [],
      position: 300,
    },
  )
  wrapper = mount(FilterBuilder, {
    props: { schema, modelValue: null },
    global: { plugins: [i18n] },
  })
  return { wrapper, schema }
}

it('offers only server-declared, type-aware fields and operators', async () => {
  const { wrapper } = await setup()
  await wrapper.find('.filter-actions button')!.trigger('click')
  let value = wrapper.emitted('update:modelValue')?.at(-1)?.[0] as FilterNode
  await wrapper.setProps({ modelValue: value })
  await flushPromises()
  const fieldSelect = wrapper.findAll('select')[1]!
  expect(fieldSelect.findAll('option').map((option) => option.text())).toEqual([
    'Status',
    'Due',
  ])
  expect(fieldSelect.text()).not.toContain('Hidden computed')

  await fieldSelect.setValue('status')
  value = wrapper.emitted('update:modelValue')?.at(-1)?.[0] as FilterNode
  await wrapper.setProps({ modelValue: value })
  await flushPromises()
  const operatorSelect = wrapper.findAll('select')[2]!
  expect(
    operatorSelect.findAll('option').map((option) => option.element.value),
  ).toEqual([
    'is',
    'is_not',
    'is_any_of',
    'is_none_of',
    'is_empty',
    'is_not_empty',
  ])
})

it('builds nested AND/OR groups without persisting view configuration', async () => {
  const { wrapper, schema } = await setup()
  await wrapper.get('button').trigger('click')
  let value = wrapper.emitted('update:modelValue')?.at(-1)?.[0] as FilterNode
  await wrapper.setProps({ modelValue: value })
  await wrapper.get('.filter-actions button:nth-child(2)').trigger('click')
  value = wrapper.emitted('update:modelValue')?.at(-1)?.[0] as FilterNode
  await wrapper.setProps({ modelValue: value })
  expect(wrapper.findAll('.filter-group')).toHaveLength(2)
  expect((value as { items: FilterNode[] }).items[0]).toHaveProperty('field')
  expect((value as { items: FilterNode[] }).items[1]).toHaveProperty('items')
  expect(schema.views[0]!.config).toEqual({ fields: [], frozen: 0 })
})

it('does not offer groups beyond the supported depth', async () => {
  const { wrapper } = await setup()
  const modelValue: FilterNode = {
    op: 'and',
    items: [
      {
        op: 'or',
        items: [
          {
            op: 'and',
            items: [{ field: 'status', cmp: 'is', value: 'Open' }],
          },
        ],
      },
    ],
  }
  await wrapper.setProps({ modelValue })
  expect(wrapper.findAll('.filter-group')).toHaveLength(3)
  expect(
    wrapper.findAll('.filter-group')[2]!.find('.filter-actions').text(),
  ).not.toContain('Add nested group')
})

it('edits JSON paths and labels relation count operators', async () => {
  const { wrapper, schema } = await setup()
  schema.fields.push(
    {
      ...schema.fields[0]!,
      id: 'payload',
      name: 'Payload',
      uiType: 'json',
      filterOps: [
        'is',
        'is_not',
        'is_empty',
        'is_not_empty',
        'has_key',
        'path_equals',
      ],
      position: 250,
    },
    {
      ...schema.fields[0]!,
      id: 'related',
      name: 'Related',
      uiType: 'link',
      filterOps: [
        'has_any_of',
        'is_empty',
        'is_not_empty',
        'eq',
        'neq',
        'lt',
        'lte',
        'gt',
        'gte',
        'between',
      ],
      position: 300,
    },
  )
  await wrapper.setProps({ schema })
  await wrapper.find('.filter-actions button')!.trigger('click')
  let value = wrapper.emitted('update:modelValue')?.at(-1)?.[0] as FilterNode
  await wrapper.setProps({ modelValue: value })
  const fieldSelect = wrapper.findAll('select')[1]!
  await fieldSelect.setValue('payload')
  value = wrapper.emitted('update:modelValue')?.at(-1)?.[0] as FilterNode
  await wrapper.setProps({ modelValue: value })
  const operatorSelect = wrapper.findAll('select')[2]!
  await operatorSelect.setValue('path_equals')
  value = wrapper.emitted('update:modelValue')?.at(-1)?.[0] as FilterNode
  await wrapper.setProps({ modelValue: value })
  const pathInput = wrapper
    .findAll('label')
    .find((label) => label.text().includes('JSON path'))!
    .find('input')
  await pathInput.setValue('project.name')
  value = wrapper.emitted('update:modelValue')?.at(-1)?.[0] as FilterNode
  await wrapper.setProps({ modelValue: value })
  const expectedInput = wrapper
    .findAll('label')
    .find((label) => label.text().includes('Expected JSON value'))!
    .find('input')
  await expectedInput.setValue('Panda')
  value = wrapper.emitted('update:modelValue')?.at(-1)?.[0] as FilterNode
  expect(value).toMatchObject({
    items: [
      {
        field: 'payload',
        cmp: 'path_equals',
        value: { path: ['project', 'name'], value: 'Panda' },
      },
    ],
  })

  await fieldSelect.setValue('related')
  value = wrapper.emitted('update:modelValue')?.at(-1)?.[0] as FilterNode
  await wrapper.setProps({ modelValue: value })
  await wrapper.findAll('select')[2]!.setValue('eq')
  value = wrapper.emitted('update:modelValue')?.at(-1)?.[0] as FilterNode
  await wrapper.setProps({ modelValue: value })
  await flushPromises()
  expect(
    wrapper
      .findAll('select')[2]!
      .findAll('option')
      .find((option) => option.element.value === 'eq')
      ?.text(),
  ).toBe('Count equals')
  expect(wrapper.find('input[type="number"]').exists()).toBe(true)
})
