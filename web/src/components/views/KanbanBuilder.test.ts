import { afterEach, expect, it } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { i18n, loadLocale } from '../../i18n'
import { tableSchema } from '../../test/navigation'
import KanbanBuilder from './KanbanBuilder.vue'

let wrapper: VueWrapper | undefined

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
})

it('saves grouping and card fields and preserves a draft after a version conflict', async () => {
  await loadLocale('en')
  i18n.global.locale.value = 'en'
  const schema = tableSchema()
  schema.fields.push(
    {
      ...schema.fields[0]!,
      id: 'status',
      name: 'Status',
      uiType: 'single_select',
      position: 2048,
    },
    {
      ...schema.fields[0]!,
      id: 'owner',
      name: 'Owner',
      uiType: 'user',
      position: 3072,
    },
  )
  const view = {
    ...schema.views[0]!,
    type: 'kanban',
    config: {
      fields: [],
      frozen: 0,
      kanban: { groupField: null, fields: ['name'] },
    },
  }
  wrapper = mount(KanbanBuilder, {
    props: { schema, view },
    global: { plugins: [i18n] },
  })
  expect(wrapper.findAll('select option')).toHaveLength(3)
  await wrapper.get('select').setValue('status')
  await wrapper.findAll('.field-choice input')[1]!.setValue(true)
  await wrapper.get('form').trigger('submit')
  expect(wrapper.emitted('save')?.[0]?.[0]).toEqual({
    groupField: 'status',
    fields: ['name', 'status'],
  })

  await wrapper.setProps({
    view: {
      ...view,
      version: 2,
      config: {
        ...view.config,
        kanban: { groupField: 'owner', fields: ['name'] },
      },
    },
  })
  expect((wrapper.get('select').element as HTMLSelectElement).value).toBe(
    'status',
  )
  await wrapper.get('form').trigger('submit')
  expect(wrapper.emitted('save')?.[1]?.[0]).toEqual({
    groupField: 'status',
    fields: ['name', 'status'],
  })
})
