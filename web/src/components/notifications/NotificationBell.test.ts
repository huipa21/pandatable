import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import type { NotificationListing } from '../../api/types'
import { i18n, loadLocale } from '../../i18n'
import { useAuthStore } from '../../stores/auth'
import { useRealtimeStore, type RealtimeEvent } from '../../stores/realtime'
import { navUser, tableSchema } from '../../test/navigation'
import NotificationBell from './NotificationBell.vue'

let pinia: ReturnType<typeof createPinia>
let wrapper: VueWrapper | undefined
let onEvent: ((event: RealtimeEvent) => void) | undefined
let listing: NotificationListing

beforeEach(async () => {
  pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().setUser(navUser)
  await loadLocale('en')
  i18n.global.locale.value = 'en'
  listing = {
    unreadCount: 1,
    hasMore: false,
    items: [
      {
        id: 'notification-1',
        tableId: 't',
        tableName: 'Table',
        rowId: '9007199254740993',
        commentId: 'comment-1',
        authorName: 'Bob',
        createdAt: '2026-10-31T12:00:00Z',
        readAt: null,
      },
    ],
  }
  onEvent = undefined
  vi.spyOn(useRealtimeStore(), 'onGlobal').mockImplementation((listener) => {
    onEvent = listener
    return () => {
      onEvent = undefined
    }
  })
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  disposePinia(pinia)
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

async function setup(hasGrid = true) {
  const fetcher = vi.fn<typeof fetch>(async (input, init) => {
    const url = String(input)
    if (url === '/api/v1/notifications') return Response.json(listing)
    if (url === '/api/v1/tables/t/schema') {
      const schema = tableSchema()
      if (!hasGrid) schema.views[0]!.type = 'form'
      return Response.json(schema)
    }
    if (url === '/api/v1/notifications/read' && init?.method === 'POST') {
      listing.unreadCount = 0
      listing.items[0]!.readAt = '2026-10-31T12:05:00Z'
      return Response.json({ eventId: 'event' })
    }
    throw new Error(`Unexpected request: ${url}`)
  })
  vi.stubGlobal('fetch', fetcher)
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }],
  })
  await router.push('/home')
  wrapper = mount(NotificationBell, {
    global: { plugins: [pinia, i18n, router] },
    attachTo: document.body,
  })
  await flushPromises()
  return { wrapper, fetcher, router }
}

it('refreshes the live unread count and opens the correct row and comment before marking read', async () => {
  const { wrapper, fetcher, router } = await setup()
  expect(wrapper.get('button').attributes('aria-label')).toBe(
    'Notifications: 1 unread',
  )
  listing.unreadCount = 2
  onEvent?.({ t: 'notification', tableId: 't' })
  await flushPromises()
  expect(wrapper.get('button').attributes('aria-label')).toBe(
    'Notifications: 2 unread',
  )
  await wrapper.get('button').trigger('click')
  expect(wrapper.text()).toContain('Bob mentioned you in Table')
  await wrapper.get('.notification-list li button').trigger('click')
  await flushPromises()
  expect(router.currentRoute.value.path).toBe('/t/t/v/v/r/9007199254740993')
  expect(router.currentRoute.value.query.comment).toBe('comment-1')
  expect(fetcher).toHaveBeenCalledWith(
    '/api/v1/notifications/read',
    expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ id: 'notification-1' }),
    }),
  )
  expect(wrapper.get('button').attributes('aria-label')).toBe(
    'Notifications: 0 unread',
  )
})

it('reports when no accessible grid exists and does not mark the notification read', async () => {
  const { wrapper, fetcher, router } = await setup(false)
  await wrapper.get('button').trigger('click')
  await wrapper.get('.notification-list li button').trigger('click')
  await flushPromises()
  expect(wrapper.text()).toContain('No accessible grid view')
  expect(router.currentRoute.value.path).toBe('/home')
  expect(fetcher).not.toHaveBeenCalledWith(
    '/api/v1/notifications/read',
    expect.anything(),
  )
})

it('ignores a late schema response after the signed-in identity changes', async () => {
  const { wrapper, fetcher, router } = await setup()
  let resolveSchema: ((value: Response) => void) | undefined
  const pendingSchema = new Promise<Response>((resolve) => {
    resolveSchema = resolve
  })
  fetcher.mockImplementation(async (input) => {
    const url = String(input)
    if (url === '/api/v1/tables/t/schema') return pendingSchema
    throw new Error(`Unexpected request: ${url}`)
  })
  await wrapper.get('button').trigger('click')
  await wrapper.get('.notification-list li button').trigger('click')
  useAuthStore().setUser(null)
  resolveSchema?.(Response.json(tableSchema()))
  await flushPromises()
  expect(router.currentRoute.value.path).toBe('/home')
  expect(fetcher).not.toHaveBeenCalledWith(
    '/api/v1/notifications/read',
    expect.anything(),
  )
})
