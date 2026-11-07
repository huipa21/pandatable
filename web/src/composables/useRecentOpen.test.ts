import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { h } from 'vue'
import { mount, type VueWrapper } from '@vue/test-utils'
import {
  createMemoryHistory,
  createRouter,
  RouterView,
} from 'vue-router'
import { useRecentOpen } from './useRecentOpen'
import { useAuthStore } from '../stores/auth'
import { useNavStore } from '../stores/nav'
import { navUser, navigation } from '../test/navigation'

let pinia: ReturnType<typeof createPinia>
let router: ReturnType<typeof createRouter>
let wrapper: VueWrapper | undefined
const page = { render: () => null }
const shell = {
  setup() {
    useRecentOpen()
    return () => h(RouterView)
  },
}

beforeEach(async () => {
  pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().setUser(navUser)
  const nav = useNavStore()
  nav.tree = navigation()
  nav.loaded = true
  router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/home', component: page },
      { path: '/w/:workspaceId', component: page },
      { path: '/w/:wid/b/:bookId', component: page },
      { path: '/t/:tableId', component: page },
      { path: '/t/:tableId/v/:viewId', component: page },
      { path: '/t/:tableId/v/:viewId/r/:rowId', component: page },
    ],
  })
  await router.push('/home')
  await router.isReady()
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  disposePinia(pinia)
  vi.unstubAllGlobals()
})

it('records each resolved workspace/workbook route once, including table/view/row routes', async () => {
  const fetch = vi
    .fn()
    .mockResolvedValue(new Response(null, { status: 204 }))
  vi.stubGlobal('fetch', fetch)
  wrapper = mount(
    shell,
    { global: { plugins: [pinia, router] } },
  )

  await router.push('/w/w')
  await router.push('/w/w/b/b')
  await router.push('/t/t')
  await router.push('/t/t/v/v')
  await router.push('/t/t/v/v/r/1')

  expect(fetch.mock.calls.map(([path, options]) => [path, options?.body])).toEqual([
    ['/api/v1/recent', '{"kind":"workspace","id":"w"}'],
    ['/api/v1/recent', '{"kind":"workspace","id":"w"}'],
    ['/api/v1/recent', '{"kind":"book","id":"b"}'],
    ['/api/v1/recent', '{"kind":"workspace","id":"w"}'],
    ['/api/v1/recent', '{"kind":"book","id":"b"}'],
    ['/api/v1/recent', '{"kind":"workspace","id":"w"}'],
    ['/api/v1/recent', '{"kind":"book","id":"b"}'],
    ['/api/v1/recent', '{"kind":"workspace","id":"w"}'],
    ['/api/v1/recent', '{"kind":"book","id":"b"}'],
  ])
})

it('does not post again when the same resolved route recomputes', async () => {
  const fetch = vi
    .fn()
    .mockResolvedValue(new Response(null, { status: 204 }))
  vi.stubGlobal('fetch', fetch)
  wrapper = mount(
    shell,
    { global: { plugins: [pinia, router] } },
  )

  await router.push('/t/t/v/v')
  const count = fetch.mock.calls.length
  useNavStore().tree = navigation()
  expect(fetch).toHaveBeenCalledTimes(count)
})

it('records the current route for a newly authenticated user', async () => {
  const fetch = vi
    .fn()
    .mockResolvedValue(new Response(null, { status: 204 }))
  vi.stubGlobal('fetch', fetch)
  wrapper = mount(shell, { global: { plugins: [pinia, router] } })

  await router.push('/w/w')
  useAuthStore().setUser({ ...navUser, id: 'bob' })
  expect(fetch).toHaveBeenCalledTimes(2)
})
