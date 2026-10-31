import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import type { RowHistory } from '../../api/types'
import { i18n, loadLocale } from '../../i18n'
import { useAuthStore } from '../../stores/auth'
import { useRealtimeStore, type RealtimeEvent } from '../../stores/realtime'
import { navUser, tableSchema } from '../../test/navigation'
import HistoryPanel from './HistoryPanel.vue'

let pinia: ReturnType<typeof createPinia>
let wrapper: VueWrapper | undefined
let onEvent: ((event: RealtimeEvent) => void) | undefined
let entries: RowHistory[]

beforeEach(async () => {
  pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().setUser(navUser)
  await loadLocale('en')
  await loadLocale('zh-CN')
  i18n.global.locale.value = 'en'
  onEvent = undefined
  entries = [
    {
      id: '3',
      authorId: 'alice',
      authorName: 'Alice',
      action: 'update',
      changes: { name: { o: 'Old', n: 'New' } },
      createdAt: '2026-10-31T12:00:45Z',
    },
    {
      id: '2',
      authorId: 'alice',
      authorName: 'Alice',
      action: 'link',
      changes: {
        relation: {
          n: [{ id: '9007199254740993', label: 'Project' }],
        },
      },
      createdAt: '2026-10-31T12:00:02Z',
    },
    {
      id: '1',
      authorId: null,
      authorName: '',
      action: 'delete',
      changes: { removed: { o: '<script>literal</script>' } },
      createdAt: '2026-10-31T11:59:59Z',
    },
  ]
  vi.spyOn(useRealtimeStore(), 'subscribe').mockImplementation(
    (_table, listener) => {
      onEvent = listener
      return () => {
        onEvent = undefined
      }
    },
  )
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  disposePinia(pinia)
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

async function setup() {
  const fetcher = vi.fn<typeof fetch>(async (input) => {
    if (String(input).includes('?offset=3'))
      return Response.json({
        items: [
          {
            ...entries[2]!,
            id: '0',
            createdAt: '2026-10-31T11:59:20Z',
          },
        ],
        hasMore: false,
      })
    return Response.json({ items: entries, hasMore: true })
  })
  vi.stubGlobal('fetch', fetcher)
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }],
  })
  await router.push('/t/t/v/v/r/1')
  const fields = tableSchema().fields
  fields.push({
    ...fields[0]!,
    id: 'relation',
    name: 'Projects',
    uiType: 'link',
  })
  wrapper = mount(HistoryPanel, {
    props: { tableId: 't', rowId: '1', identity: navUser.id, fields },
    global: { plugins: [pinia, i18n, router] },
    attachTo: document.body,
  })
  await flushPromises()
  return { wrapper, fetcher }
}

it('groups authors and minutes, renders exact changes, and pages older history', async () => {
  const { wrapper, fetcher } = await setup()
  expect(wrapper.findAll('.history-group')).toHaveLength(2)
  expect(wrapper.findAll('.history-group')[0]!.text()).toContain('Alice')
  expect(wrapper.get('time').attributes('datetime')).toBe(
    '2026-10-31T12:00:45Z',
  )
  expect(wrapper.get('time').text()).not.toBe('')
  expect(wrapper.findAll('.history-group')[0]!.text()).toContain('Old → New')
  expect(wrapper.text()).toContain('No value → Project (row 9007199254740993)')
  expect(wrapper.text()).toContain('Deleted user')
  expect(wrapper.text()).toContain('Deleted field (removed)')
  expect(wrapper.text()).toContain('<script>literal</script>')
  expect(wrapper.find('script').exists()).toBe(false)
  await wrapper.get('button').trigger('click')
  await flushPromises()
  expect(fetcher).toHaveBeenLastCalledWith(
    '/api/v1/tables/t/rows/1/history?offset=3',
    expect.objectContaining({ method: 'GET' }),
  )
  expect(wrapper.findAll('.history-group')).toHaveLength(2)
  expect(wrapper.findAll('[data-history-id]')).toHaveLength(4)
  expect(
    fetcher.mock.calls.every(([url]) => String(url).includes('/history')),
  ).toBe(true)
  i18n.global.locale.value = 'zh-CN'
  await flushPromises()
  expect(wrapper.text()).toContain('已删除的用户')
  expect(wrapper.text()).toContain('Project（行 9007199254740993）')
})

it('refreshes for existing row events and surfaces failed refreshes as stale', async () => {
  const { wrapper, fetcher } = await setup()
  entries[0]!.changes.name = { o: 'New', n: 'Remote' }
  onEvent?.({ t: 'row.updated', tableId: 't', ids: ['1'] })
  await flushPromises()
  expect(wrapper.text()).toContain('New → Remote')
  fetcher.mockResolvedValueOnce(
    Response.json({ error: { code: 'ERR_INTERNAL' } }, { status: 500 }),
  )
  onEvent?.({ t: 'row.bulk', tableId: 't' })
  await flushPromises()
  expect(wrapper.get('[role="alert"]').text()).toContain(
    'Previously loaded history may be out of date.',
  )
  expect(wrapper.text()).toContain('New → Remote')
})

it('discards old history when the viewer identity changes', async () => {
  const { wrapper } = await setup()
  expect(wrapper.text()).toContain('Old → New')
  useAuthStore().setUser(null)
  await flushPromises()
  expect(wrapper.findAll('[data-history-id]')).toHaveLength(0)
  expect(wrapper.text()).not.toContain('Old → New')
})
