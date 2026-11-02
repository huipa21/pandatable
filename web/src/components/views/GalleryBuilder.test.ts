import { afterEach, expect, it } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { i18n, loadLocale } from '../../i18n'
import { tableSchema } from '../../test/navigation'
import GalleryBuilder from './GalleryBuilder.vue'

let wrapper: VueWrapper | undefined

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
})

it('saves an attachment cover and card fields while preserving a conflicting draft', async () => {
  await loadLocale('en')
  i18n.global.locale.value = 'en'
  const schema = tableSchema()
  schema.fields.push(
    {
      ...schema.fields[0]!,
      id: 'photo',
      name: 'Photo',
      uiType: 'attachment',
      position: 2048,
    },
    {
      ...schema.fields[0]!,
      id: 'notes',
      name: 'Notes',
      uiType: 'text',
      position: 3072,
    },
  )
  const view = {
    ...schema.views[0]!,
    type: 'gallery',
    config: {
      fields: [],
      frozen: 0,
      gallery: { coverField: null, fields: ['name'] },
    },
  }
  wrapper = mount(GalleryBuilder, {
    props: { schema, view },
    global: { plugins: [i18n] },
  })
  expect(wrapper.findAll('select option')).toHaveLength(2)
  await wrapper.get('select').setValue('photo')
  await wrapper.findAll('.field-choice input')[2]!.setValue(true)
  await wrapper.get('form').trigger('submit')
  expect(wrapper.emitted('save')?.[0]?.[0]).toEqual({
    coverField: 'photo',
    fields: ['name', 'notes'],
  })

  await wrapper.setProps({
    view: {
      ...view,
      version: 2,
      config: {
        ...view.config,
        gallery: { coverField: null, fields: ['name', 'photo'] },
      },
    },
  })
  expect((wrapper.get('select').element as HTMLSelectElement).value).toBe(
    'photo',
  )
  await wrapper.get('form').trigger('submit')
  expect(wrapper.emitted('save')?.[1]?.[0]).toEqual({
    coverField: 'photo',
    fields: ['name', 'notes'],
  })
})
