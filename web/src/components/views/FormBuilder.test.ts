import { afterEach, expect, it } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { i18n, loadLocale } from '../../i18n'
import { tableSchema } from '../../test/navigation'
import type { FormConfig, TableView } from '../../api/types'
import FormBuilder from './FormBuilder.vue'

let wrapper: VueWrapper | undefined

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
})

it('selects, orders, labels, and saves writable form fields', async () => {
  await loadLocale('en')
  i18n.global.locale.value = 'en'
  const schema = tableSchema()
  schema.fields.push(
    {
      ...schema.fields[0]!,
      id: 'status',
      name: 'Status',
      position: 2048,
    },
    {
      ...schema.fields[0]!,
      id: 'computed',
      name: 'Computed',
      uiType: 'formula',
      position: 3072,
    },
    {
      ...schema.fields[0]!,
      id: 'system',
      name: 'System',
      isSystem: true,
      position: 4096,
    },
  )
  const view: TableView = {
    ...schema.views[0]!,
    type: 'form',
    config: {
      fields: [],
      frozen: 0,
      form: { title: '', description: '', fields: [], afterSubmit: 'message' },
    },
  }
  wrapper = mount(FormBuilder, {
    props: { schema, view },
    global: { plugins: [i18n] },
  })

  await wrapper.get('summary').trigger('click')
  expect(wrapper.findAll('.field-choice')).toHaveLength(2)
  await wrapper.findAll('.field-choice input')[0]!.setValue(true)
  await wrapper.findAll('.field-choice input')[1]!.setValue(true)
  await flushPromises()
  expect(wrapper.findAll('.selected-field')).toHaveLength(2)

  const selected = wrapper.findAll('.selected-field')
  await selected[0]!.get('[aria-label="Move Name down"]').trigger('click')
  const nameField = wrapper
    .findAll('.selected-field')
    .find((item) => item.get('h3').text() === 'Name')!
  await nameField.get('label input').setValue('Service request')
  await nameField.get('textarea').setValue('Describe the request.')
  await nameField.get('input[type="checkbox"]').setValue(true)
  const fields: FormConfig['fields'] = [
    { id: 'status', label: 'Status', help: '', required: false },
    {
      id: 'name',
      label: 'Service request',
      help: 'Describe the request.',
      required: true,
    },
  ]
  await wrapper.get('form').trigger('submit')
  expect(wrapper.emitted('save')?.[0]?.[0]).toMatchObject({
    title: '',
    fields,
    afterSubmit: 'message',
  })
})
