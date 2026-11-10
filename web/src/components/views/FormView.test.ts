import { afterEach, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { i18n, loadLocale } from '../../i18n'
import type { TableSchema } from '../../api/types'
import { tableSchema, navUser } from '../../test/navigation'
import { useAuthStore } from '../../stores/auth'
import FieldEditor from '../fields/FieldEditor.vue'
import FormView from './FormView.vue'

let wrapper: VueWrapper | undefined
let pinia: ReturnType<typeof createPinia>

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  if (pinia) disposePinia(pinia)
  vi.unstubAllGlobals()
})

async function setup(
  level: 'read' | 'write' | 'owner',
  afterSubmit: 'message' | 'new' = 'message',
) {
  await loadLocale('en')
  i18n.global.locale.value = 'en'
  pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().setUser(navUser)
  const schema = tableSchema()
  schema.myLevel = level
  const view = schema.views[0]!
  view.type = 'form'
  view.config.form = {
    title: 'Request form',
    description: '',
    fields: [{ id: 'name', label: 'Topic', help: '', required: true }],
    afterSubmit,
  }
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/t/:tableId/v/:viewId', component: FormView }],
  })
  await router.push('/t/t/v/v')
  const form = mount(FormView, {
    props: { schema, view, identity: navUser.id },
    global: { plugins: [pinia, i18n, router] },
  })
  wrapper = form
  return form
}

it('disables read-only submission and validates required values before sending', async () => {
  const fetcher = vi.fn()
  vi.stubGlobal('fetch', fetcher)
  const readOnly = await setup('read')
  expect(readOnly.get('button[type="submit"]').element).toHaveProperty(
    'disabled',
    true,
  )
  expect(readOnly.get('p.read-only').text()).toContain('do not have permission')
  readOnly.unmount()
  wrapper = undefined
  disposePinia(pinia)

  const writable = await setup('write')
  await writable.get('form').trigger('submit')
  expect(writable.get('[role="alert"]').text()).toContain('Topic is required')
  expect(fetcher).not.toHaveBeenCalled()
})

it('submits live field values and shows the configured confirmation message', async () => {
  const fetcher = vi.fn().mockResolvedValue(Response.json({ row: { id: '1' } }))
  vi.stubGlobal('fetch', fetcher)
  const form = await setup('owner')
  await form.get('input[aria-label="Topic"]').setValue('Access request')
  await form.get('form').trigger('submit')
  await flushPromises()
  expect(fetcher).toHaveBeenCalledWith(
    '/api/v1/views/v/form-submit',
    expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ fields: { name: 'Access request' } }),
    }),
  )
  expect(form.get('[role="status"]').text()).toBe(
    'Your response was submitted.',
  )
})

it('renders a checkbox icon and requires a checked value when configured', async () => {
  const fetcher = vi.fn().mockResolvedValue(Response.json({ row: { id: '1' } }))
  vi.stubGlobal('fetch', fetcher)
  const form = await setup('owner')
  const schema = JSON.parse(JSON.stringify(form.props('schema'))) as TableSchema
  schema.fields[0]!.uiType = 'checkbox'
  schema.fields[0]!.options = { icon: 'heart' }
  schema.fields[0]!.default = false
  schema.schemaVersion++
  await form.setProps({ schema, view: schema.views[0]! })
  const checkbox = form.get('[role="checkbox"][aria-label="Topic"]')
  expect(checkbox.attributes('aria-required')).toBe('true')
  expect(checkbox.get('.field-check').attributes('data-icon')).toBe('heart')
  await form.get('form').trigger('submit')
  expect(form.get('[role="alert"]').text()).toContain('Topic is required')
  expect(fetcher).not.toHaveBeenCalled()
  await checkbox.trigger('click')
  await form.get('form').trigger('submit')
  await flushPromises()
  expect(fetcher).toHaveBeenCalledWith(
    '/api/v1/views/v/form-submit',
    expect.objectContaining({
      body: JSON.stringify({ fields: { name: true } }),
    }),
  )
})

it.each(['text', 'checkbox'])(
  'enforces schema-level requiredness for a %s even when the form makes it optional',
  async (kind) => {
    const fetcher = vi
      .fn()
      .mockResolvedValue(Response.json({ row: { id: '1' } }))
    vi.stubGlobal('fetch', fetcher)
    const form = await setup('owner')
    const schema = JSON.parse(
      JSON.stringify(form.props('schema')),
    ) as TableSchema
    schema.fields[0]!.uiType = kind
    schema.fields[0]!.isRequired = true
    schema.fields[0]!.default = kind === 'checkbox' ? false : null
    schema.views[0]!.config.form!.fields[0]!.required = false
    schema.schemaVersion++
    await form.setProps({ schema, view: schema.views[0]! })

    expect(form.find('.required-mark').exists()).toBe(true)
    expect(form.findComponent(FieldEditor).props('required')).toBe(true)
    await form.get('form').trigger('submit')
    expect(form.get('[role="alert"]').text()).toContain('Topic is required')
    expect(fetcher).not.toHaveBeenCalled()

    if (kind === 'checkbox')
      await form.get('[role="checkbox"]').trigger('click')
    else await form.get('input[aria-label="Topic"]').setValue('Required value')
    await form.get('form').trigger('submit')
    await flushPromises()
    expect(fetcher).toHaveBeenCalledWith(
      '/api/v1/views/v/form-submit',
      expect.objectContaining({
        body: JSON.stringify({
          fields: { name: kind === 'checkbox' ? true : 'Required value' },
        }),
      }),
    )
  },
)

it('preserves an in-progress draft when the same schema and view are refreshed', async () => {
  const fetcher = vi.fn().mockResolvedValue(Response.json({ row: { id: '1' } }))
  vi.stubGlobal('fetch', fetcher)
  const form = await setup('owner')
  await form.get('input[aria-label="Topic"]').setValue('Access request')
  const refreshed = JSON.parse(
    JSON.stringify(form.props('schema')),
  ) as TableSchema
  await form.setProps({ schema: refreshed, view: refreshed.views[0]! })
  await form.get('form').trigger('submit')
  await flushPromises()
  expect(fetcher).toHaveBeenCalledWith(
    '/api/v1/views/v/form-submit',
    expect.objectContaining({
      body: JSON.stringify({ fields: { name: 'Access request' } }),
    }),
  )
  expect(form.get('[role="status"]').text()).toBe(
    'Your response was submitted.',
  )
  const secondRefresh = JSON.parse(JSON.stringify(refreshed)) as TableSchema
  await form.setProps({ schema: secondRefresh, view: secondRefresh.views[0]! })
  expect(form.get('[role="status"]').text()).toBe(
    'Your response was submitted.',
  )
})

it.each(['view', 'schema'])(
  'resets the field editor when the %s version changes',
  async (change) => {
    const fetcher = vi.fn()
    vi.stubGlobal('fetch', fetcher)
    const form = await setup('owner')
    await form.get('input[aria-label="Topic"]').setValue('Stale draft')
    const refreshed = JSON.parse(
      JSON.stringify(form.props('schema')),
    ) as TableSchema
    if (change === 'view') refreshed.views[0]!.version++
    else refreshed.schemaVersion++
    await form.setProps({ schema: refreshed, view: refreshed.views[0]! })
    expect(form.get('input[aria-label="Topic"]').element).toHaveProperty(
      'value',
      '',
    )
    await form.get('form').trigger('submit')
    expect(form.get('[role="alert"]').text()).toContain('Topic is required')
    expect(fetcher).not.toHaveBeenCalled()
  },
)

it('clears values after a successful new-form submission', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(Response.json({ row: { id: '1' } })),
  )
  const form = await setup('owner', 'new')
  const input = form.get('input[aria-label="Topic"]')
  await input.setValue('Another request')
  await form.get('form').trigger('submit')
  await flushPromises()
  expect(form.get('input[aria-label="Topic"]').element).toHaveProperty(
    'value',
    '',
  )
  expect(form.find('[role="status"]').exists()).toBe(false)
})
