import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { useNavStore } from './nav'
import { useAuthStore } from './auth'
import { navigation, navUser, tableSchema } from '../test/navigation'

let pinia: ReturnType<typeof createPinia>
beforeEach(() => {
  pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().setUser(navUser)
})
afterEach(() => {
  disposePinia(pinia)
  vi.unstubAllGlobals()
})

it('coalesces reads, preserves server order and prevents old/other-identity responses', async () => {
  const releases: ((response: Response) => void)[] = []
  const fetch = vi.fn(
    () => new Promise<Response>((resolve) => releases.push(resolve)),
  )
  vi.stubGlobal('fetch', fetch)
  const nav = useNavStore()
  const first = nav.refresh()
  const coalesced = nav.refresh()
  expect(fetch).toHaveBeenCalledTimes(1)
  const newer = nav.refresh(true)
  const next = navigation()
  next.workspaces.push({ ...next.workspaces[0]!, id: 'second', name: 'A' })
  releases[1]!(Response.json(next))
  await newer
  releases[0]!(Response.json({ workspaces: [] }))
  await first
  await coalesced
  expect(nav.items.map((item) => item.id)).toEqual(['w', 'second'])
  const oldIdentity = nav.refresh()
  useAuthStore().setUser({ ...navUser, id: 'bob' })
  expect(nav.loaded).toBe(false)
  expect(nav.items).toEqual([])
  releases[2]!(Response.json(next))
  await oldIdentity
  expect(nav.items).toEqual([])
})

it('mirrors owner-of-parent and path-only controls, not the admin role', async () => {
  const tree = navigation()
  const w = tree.workspaces[0]!
  const b = w.books[0]!
  w.level = 'none'
  w.pathOnly = true
  b.level = 'owner'
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json(tree)))
  const nav = useNavStore()
  await nav.refresh()
  expect(nav.items[0]).toMatchObject({
    canEdit: false,
    canCreate: false,
    pathOnly: true,
  })
  expect(nav.items[0]!.children[0]).toMatchObject({
    canEdit: false,
    canCreate: true,
  })
  expect(nav.items[0]!.children[0]!.children[0]).toMatchObject({
    canEdit: true,
  })
  b.level = 'none'
  b.pathOnly = true
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json(tree)))
  await nav.refresh()
  expect(nav.flatItems.every((item) => !item.canCreate && !item.canEdit)).toBe(
    true,
  )
  useAuthStore().setUser({ ...navUser, id: 'admin', role: 'admin' })
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(Response.json({ workspaces: [] })),
  )
  await nav.refresh()
  expect(nav.items).toEqual([])
})

it('does not navigate or repopulate a new identity from an old mutation', async () => {
  let release!: (response: Response) => void
  const fetch = vi.fn(
    () =>
      new Promise<Response>((resolve) => {
        release = resolve
      }),
  )
  vi.stubGlobal('fetch', fetch)
  const nav = useNavStore()
  const mutation = nav.submit(
    { operation: 'create', kind: 'workspace' },
    'Old identity',
  )
  useAuthStore().setUser({ ...navUser, id: 'bob' })
  release(Response.json({ workspace: { id: 'old' }, eventId: 'opaque' }))
  await expect(mutation).rejects.toMatchObject({ name: 'AbortError' })
  expect(nav.items).toEqual([])
  expect(nav.busy).toBe(false)
  expect(fetch).toHaveBeenCalledTimes(1)
})

it('refetches after mutations/403/404, sends CSRF and index, and surfaces stale errors', async () => {
  const fetch = vi.fn((path: string, options: RequestInit) =>
    Promise.resolve(
      Response.json(
        options.method === 'GET'
          ? navigation()
          : path.endsWith('/tables')
            ? { ...tableSchema(), eventId: 'opaque' }
            : { eventId: 'opaque' },
      ),
    ),
  )
  vi.stubGlobal('fetch', fetch)
  const nav = useNavStore()
  await nav.refresh()
  expect(
    await nav.submit(
      { operation: 'create', kind: 'table', parentId: 'b' },
      '表',
    ),
  ).toBe('/t/t/v/v')
  expect(fetch).toHaveBeenCalledWith(
    '/api/v1/books/b/tables',
    expect.objectContaining({
      method: 'POST',
      body: '{"name":"表"}',
      headers: {
        'Content-Type': 'application/json',
        'X-PT-CSRF': '1',
        'X-PT-Event-ID': expect.stringMatching(/^[0-9a-f]{32}$/),
      },
      signal: expect.any(AbortSignal),
    }),
  )
  await nav.reorder(nav.flatItems[2]!, 0)
  expect(fetch).toHaveBeenCalledWith(
    '/api/v1/tables/t',
    expect.objectContaining({ method: 'PATCH', body: '{"index":0}' }),
  )
  for (const status of [403, 404]) {
    fetch.mockImplementation((_, options) =>
      Promise.resolve(
        options.method === 'GET'
          ? Response.json(navigation())
          : Response.json(
              {
                error: {
                  code:
                    status === 403 ? 'ERR_PERMISSION_DENIED' : 'ERR_NOT_FOUND',
                },
              },
              { status },
            ),
      ),
    )
    await expect(
      nav.submit({ operation: 'delete', kind: 'table', id: 't' }, ''),
    ).rejects.toMatchObject({ status })
    expect(fetch).toHaveBeenLastCalledWith(
      '/api/v1/nav',
      expect.objectContaining({ method: 'GET' }),
    )
    expect(nav.mutationError).toBeTruthy()
  }
  fetch.mockRejectedValue(new TypeError('offline'))
  expect(await nav.refresh()).toBe(false)
  expect(nav.error).toBeInstanceOf(TypeError)
  expect(nav.loaded).toBe(true)
  expect(nav.items).toHaveLength(1)
  expect(nav.busy).toBe(false)
})
