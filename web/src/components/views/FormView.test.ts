import { afterEach, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { i18n, loadLocale } from '../../i18n'
import { tableSchema, navUser } from '../../test/navigation'
import { useAuthStore } from '../../stores/auth'
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
  wrapper = mount(FormView, {
    props: { schema, view, identity: navUser.id },
    global: { plugins: [pinia, i18n, router] },
  })
  return wrapper
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
