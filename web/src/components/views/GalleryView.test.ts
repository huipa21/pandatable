import { afterEach, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import type { DataRow, RowQueryRequest } from '../../api/types'
import { i18n, loadLocale } from '../../i18n'
import { useAuthStore } from '../../stores/auth'
import { useRealtimeStore } from '../../stores/realtime'
import { navUser, tableSchema } from '../../test/navigation'
import GalleryView from './GalleryView.vue'

let wrapper: VueWrapper | undefined
let pinia: ReturnType<typeof createPinia>

const imageId = '123e4567-e89b-12d3-a456-426614174000'
const pdfId = '123e4567-e89b-12d3-a456-426614174001'
const thumbnail = `/api/v1/attachments/${imageId}/thumbnail`
const image = {
  id: imageId,
  name: 'photo.png',
  mime: 'image/png',
  size: 12,
  thumb: thumbnail,
}
const pdf = {
  id: pdfId,
  name: 'document.pdf',
  mime: 'application/pdf',
  size: 20,
  thumb: null,
}

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  if (pinia) disposePinia(pinia)
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

async function setup() {
  await loadLocale('en')
  i18n.global.locale.value = 'en'
  pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().setUser(navUser)
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
    {
      ...schema.fields[0]!,
      id: 'hidden',
      name: 'Hidden',
      uiType: 'text',
      position: 4096,
    },
  )
  const view = schema.views[0]!
  view.type = 'gallery'
  view.config.gallery = { coverField: 'photo', fields: ['name', 'notes'] }
  const state = {
    rows: [
      {
        id: '1',
        fields: {
          name: 'First',
          photo: [pdf, image],
          notes: 'Visible detail',
          hidden: 'Secret detail',
        },
      },
      {
        id: '2',
        fields: { name: 'Second', photo: [pdf], notes: null },
      },
      {
        id: '3',
        fields: {
          name: 'Invalid cover',
          photo: [{ ...image, thumb: 'https://example.test/unsafe.png' }],
          notes: null,
        },
      },
    ] as DataRow[],
    total: null as number | null,
    fail: false,
  }
  const fetcher = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      if (!String(input).endsWith('/query'))
        throw new Error(`Unexpected request: ${String(input)}`)
      if (state.fail)
        return Response.json(
          { error: { code: 'ERR_INTERNAL' } },
          { status: 500 },
        )
      const body = JSON.parse(String(init?.body)) as RowQueryRequest
      return Response.json({
        rows: state.rows.slice(
          body.offset ?? 0,
          (body.offset ?? 0) + (body.limit ?? 20),
        ),
        fieldIds: body.fieldIds,
        total: state.total ?? state.rows.length,
        schemaVersion: 1,
      })
    },
  )
  vi.stubGlobal('fetch', fetcher)
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/t/:tableId/v/:viewId', component: GalleryView }],
  })
  await router.push('/t/t/v/v')
  wrapper = mount(GalleryView, {
    props: { schema, view, identity: navUser.id },
    global: { plugins: [pinia, i18n, router] },
  })
  await flushPromises()
  return { wrapper, schema, view, state, fetcher }
}

it('renders the first safe image cover and selected fields with explicit fallbacks', async () => {
  const { fetcher } = await setup()
  const first = wrapper!.get('[data-row-id="1"]')
  expect(first.get('img').attributes()).toMatchObject({
    src: thumbnail,
    alt: 'photo.png',
  })
  expect(first.text()).toContain('Visible detail')
  expect(first.text()).not.toContain('Secret detail')
  expect(wrapper!.get('[data-row-id="2"]').text()).toContain('No cover image')
  expect(wrapper!.get('[data-row-id="3"]').text()).toContain(
    'Cover image unavailable',
  )
  expect(wrapper!.find('img[src^="https:"]').exists()).toBe(false)
  await first.get('button').trigger('click')
  expect(wrapper!.emitted('expand')?.[0]).toEqual(['1'])
  expect(fetcher).toHaveBeenCalledWith(
    '/api/v1/views/v/query',
    expect.objectContaining({
      body: JSON.stringify({
        offset: 0,
        limit: 20,
        includeCount: true,
        fieldIds: ['name', 'photo', 'notes'],
      }),
    }),
  )
})

it('reports thumbnail failures and retries covers after a refresh', async () => {
  await setup()
  await wrapper!.get('[data-row-id="1"] img').trigger('error')
  expect(wrapper!.get('[data-row-id="1"]').text()).toContain(
    'Cover image unavailable',
  )
  await wrapper!.vm.$.exposed!.refresh()
  await flushPromises()
  expect(wrapper!.get('[data-row-id="1"] img').attributes('src')).toBe(
    thumbnail,
  )
})

it('pages bounded results and refreshes after realtime changes and deletions', async () => {
  const { state, fetcher } = await setup()
  state.rows = Array.from({ length: 25 }, (_, index) => ({
    id: String(index + 1),
    fields: { name: `Item ${index + 1}`, photo: null, notes: 'Detail' },
  }))
  vi.useFakeTimers()
  useRealtimeStore().invalidateLocal('t')
  await vi.advanceTimersByTimeAsync(100)
  await flushPromises()
  expect(wrapper!.findAll('.gallery-card')).toHaveLength(20)
  await wrapper!
    .findAll('button')
    .find((button) => button.text() === 'Next page')!
    .trigger('click')
  await flushPromises()
  expect(wrapper!.findAll('.gallery-card')).toHaveLength(5)
  expect(wrapper!.text()).toContain('Item 25')
  expect(
    fetcher.mock.calls.map(([, init]) => JSON.parse(String(init?.body))),
  ).toContainEqual(expect.objectContaining({ offset: 20, limit: 20 }))
  state.rows = state.rows.slice(0, 5)
  useRealtimeStore().invalidateLocal('t')
  await vi.advanceTimersByTimeAsync(100)
  await flushPromises()
  expect(wrapper!.findAll('.gallery-card')).toHaveLength(5)
  expect(wrapper!.text()).toContain('Item 1')
  expect(wrapper!.text()).not.toContain('Item 25')
})

it('shows load failures and an explicit query paging limit', async () => {
  const { state } = await setup()
  state.total = 1_000_021
  await wrapper!.vm.$.exposed!.refresh()
  await flushPromises()
  expect(wrapper!.text()).toContain('1000020')
  state.fail = true
  await wrapper!.vm.$.exposed!.refresh()
  await flushPromises()
  expect(wrapper!.get('[role="alert"]').text()).toContain(
    'Gallery could not be refreshed',
  )
})
