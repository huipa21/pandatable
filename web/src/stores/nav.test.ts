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

it('ignores a stale preference read after setting a new default and preserves the request contract', async () => {
  let releaseRead!: (response: Response) => void
  const fetch = vi.fn((_path: string, options: RequestInit) => {
    if (options.method === 'PUT')
      return Promise.resolve(
        Response.json({ workspaceId: 'w2', eventId: 'opaque' }),
      )
    return new Promise<Response>((resolve) => {
      releaseRead = resolve
    })
  })
  vi.stubGlobal('fetch', fetch)
  const nav = useNavStore()
  const read = nav.refreshPreference()
  const write = nav.setPreferredWorkspace('w2')
  expect(await write).toBe('w2')
  releaseRead(Response.json({ workspaceId: 'w' }))
  expect(await read).toBe(false)
  expect(nav.preferredWorkspaceId).toBe('w2')
  expect(nav.preferenceLoaded).toBe(true)
  expect(nav.preferenceLoading).toBe(false)
  expect(fetch).toHaveBeenCalledWith(
    '/api/v1/auth/workspace-preference',
    expect.objectContaining({
      method: 'PUT',
      body: '{"workspaceId":"w2"}',
      headers: expect.objectContaining({ 'X-PT-CSRF': '1' }),
      signal: expect.any(AbortSignal),
    }),
  )
})

it('surfaces preference failures, retries authoritative reads and clears state on identity change', async () => {
  const fetch = vi
    .fn()
    .mockRejectedValueOnce(new TypeError('offline'))
    .mockResolvedValueOnce(Response.json({ workspaceId: 'w' }))
  vi.stubGlobal('fetch', fetch)
  const nav = useNavStore()
  nav.preferenceLoaded = true
  nav.preferredWorkspaceId = 'old'
  expect(await nav.refreshPreference()).toBe(false)
  expect(nav.preferenceError).toBeInstanceOf(TypeError)
  expect(nav.preferredWorkspaceId).toBe('old')
  expect(await nav.refreshPreference(true)).toBe(true)
  expect(nav.preferenceError).toBeNull()
  expect(nav.preferredWorkspaceId).toBe('w')
  nav.lastWorkspaceId = 'other'
  useAuthStore().setUser({ ...navUser, id: 'bob' })
  expect(nav.preferenceLoaded).toBe(false)
  expect(nav.preferredWorkspaceId).toBeNull()
  expect(nav.lastWorkspaceId).toBeNull()
})

it('surfaces a rejected default change and refreshes access without inventing a preference', async () => {
  const fetch = vi.fn((path: string) =>
    Promise.resolve(
      path.endsWith('/nav')
        ? Response.json(navigation())
        : Response.json(
            { error: { code: 'ERR_PERMISSION_DENIED' } },
            { status: 403 },
          ),
    ),
  )
  vi.stubGlobal('fetch', fetch)
  const nav = useNavStore()
  nav.preferenceLoaded = true
  nav.preferredWorkspaceId = 'w'
  await expect(nav.setPreferredWorkspace('path-only')).rejects.toMatchObject({
    code: 'ERR_PERMISSION_DENIED',
  })
  expect(nav.preferenceError).toMatchObject({ code: 'ERR_PERMISSION_DENIED' })
  expect(nav.preferredWorkspaceId).toBe('w')
  expect(fetch).toHaveBeenCalledWith(
    '/api/v1/nav',
    expect.objectContaining({ method: 'GET' }),
  )
})

it('aborts a pending resolve on identity change and never applies its result', async () => {
  let release!: (response: Response) => void
  const fetch = vi.fn(
    () =>
      new Promise<Response>((resolve) => {
        release = resolve
      }),
  )
  vi.stubGlobal('fetch', fetch)
  const nav = useNavStore()
  const resolution = nav.resolvePreference()
  expect(fetch).toHaveBeenCalledWith(
    '/api/v1/auth/workspace-preference/resolve',
    expect.objectContaining({ method: 'POST', body: '{}' }),
  )
  useAuthStore().setUser({ ...navUser, id: 'bob' })
  release(Response.json({ workspaceId: 'old', eventId: 'opaque' }))
  await expect(resolution).rejects.toMatchObject({ name: 'AbortError' })
  expect(nav.preferredWorkspaceId).toBeNull()
  expect(nav.preferenceLoaded).toBe(false)
  expect(nav.preferenceBusy).toBe(false)
  expect(nav.preferenceError).toBeNull()
})

it('reconciles an interrupted resolve without applying its late response', async () => {
  let release!: (response: Response) => void
  const fetch = vi.fn((_path: string, options: RequestInit) =>
    options.method === 'POST'
      ? new Promise<Response>((resolve) => {
          release = resolve
        })
      : Promise.resolve(Response.json({ workspaceId: 'actual' })),
  )
  vi.stubGlobal('fetch', fetch)
  const nav = useNavStore()
  const controller = new AbortController()
  const resolution = nav.resolvePreference(controller.signal)
  controller.abort()
  release(Response.json({ workspaceId: 'late', eventId: 'opaque' }))
  await expect(resolution).rejects.toMatchObject({ name: 'AbortError' })
  await vi.waitFor(() => expect(nav.preferredWorkspaceId).toBe('actual'))
  expect(nav.preferenceError).toBeNull()
  expect(fetch).toHaveBeenCalledTimes(2)
})

it('allows a new home resolve after an earlier navigation cancels its request', async () => {
  let release!: (response: Response) => void
  let posts = 0
  const fetch = vi.fn((_path: string, options: RequestInit) => {
    if (options.method !== 'POST')
      return Promise.resolve(Response.json({ workspaceId: 'stale' }))
    posts++
    if (posts === 1)
      return new Promise<Response>((resolve) => {
        release = resolve
      })
    return Promise.resolve(
      Response.json({ workspaceId: 'current', eventId: 'next' }),
    )
  })
  vi.stubGlobal('fetch', fetch)
  const nav = useNavStore()
  const controller = new AbortController()
  const first = nav.resolvePreference(controller.signal)
  const cancelled = expect(first).rejects.toMatchObject({ name: 'AbortError' })
  controller.abort()
  const second = nav.resolvePreference()
  release(Response.json({ workspaceId: 'late', eventId: 'first' }))
  await cancelled
  expect(await second).toBe('current')
  expect(nav.preferredWorkspaceId).toBe('current')
  expect(nav.preferenceError).toBeNull()
  expect(posts).toBe(2)
})

it('serializes a default change before a later home resolve', async () => {
  let release!: (response: Response) => void
  const fetch = vi.fn((path: string, options: RequestInit) => {
    if (options.method === 'PUT')
      return new Promise<Response>((resolve) => {
        release = resolve
      })
    expect(path).toBe('/api/v1/auth/workspace-preference/resolve')
    return Promise.resolve(
      Response.json({ workspaceId: 'w2', eventId: 'next' }),
    )
  })
  vi.stubGlobal('fetch', fetch)
  const nav = useNavStore()
  const write = nav.setPreferredWorkspace('w2')
  const resolve = nav.resolvePreference()
  expect(fetch).toHaveBeenCalledTimes(1)
  release(Response.json({ workspaceId: 'w2', eventId: 'first' }))
  expect(await write).toBe('w2')
  expect(await resolve).toBe('w2')
  expect(fetch).toHaveBeenCalledTimes(2)
  expect(nav.preferenceBusy).toBe(false)
})
