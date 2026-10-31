import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { useSharingStore, validShareSearch, type ShareChange } from './sharing'
import { useNavStore } from './nav'
import { useAuthStore } from './auth'
import { navigation, navUser } from '../test/navigation'
import type { GrantListing } from '../api/types'

const listing = (): GrantListing => ({
  resource: { type: 'table', id: 't' },
  effectiveLevel: 'owner',
  items: [],
  inherited: [],
})
const change: ShareChange = {
  principalType: 'user',
  principalId: 'bob',
  name: 'Bob',
  level: 'read',
  inheritedLevel: null,
}
let pinia: ReturnType<typeof createPinia>
beforeEach(() => {
  pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().setUser(navUser)
  useNavStore().tree = navigation()
})
afterEach(() => {
  disposePinia(pinia)
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})
async function setup() {
  const fetch = vi.fn((path: string, options?: RequestInit) =>
    options?.method === 'PUT' || options?.method === 'DELETE'
      ? Promise.resolve(Response.json({ eventId: 'opaque' }))
      : Promise.resolve(
          Response.json(
            path.endsWith('/nav')
              ? navigation()
              : path.includes('/directory/')
                ? {
                    items: [{ id: 'bob', name: 'Bob', username: 'bob' }],
                    hasMore: false,
                  }
                : listing(),
          ),
        ),
  )
  vi.stubGlobal('fetch', fetch)
  const share = useSharingStore()
  share.open({ kind: 'table', id: 't', name: 'Table' })
  await flushPromises()
  return { share, fetch }
}

it('validates the exact code-point/UTF-8/literal search bounds', () => {
  for (const value of [
    'ab',
    '  团队  ',
    '界'.repeat(84),
    'a'.repeat(100),
    '%_',
  ])
    expect(validShareSearch(value)).toBe(true)
  for (const value of ['', 'a', 'a'.repeat(101), '界'.repeat(85), 'ab\tcd'])
    expect(validShareSearch(value)).toBe(false)
})

it('aborts and ignores list reads from an abandoned target or identity', async () => {
  const releases: ((response: Response) => void)[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(() => new Promise<Response>((resolve) => releases.push(resolve))),
  )
  const share = useSharingStore()
  share.open({ kind: 'table', id: 't', name: 'First' })
  share.open({ kind: 'book', id: 'b', name: 'Second' })
  releases[1]!(
    Response.json({ ...listing(), resource: { type: 'book', id: 'b' } }),
  )
  await flushPromises()
  releases[0]!(Response.json(listing()))
  await flushPromises()
  expect(share.listing?.resource.id).toBe('b')
  const pending = share.load()
  useAuthStore().setUser({ ...navUser, id: 'new' })
  releases[2]!(Response.json(listing()))
  await pending
  expect(share.target).toBeNull()
  expect(share.listing).toBeNull()
})

it('clears stale picker results immediately and ignores old query/type responses', async () => {
  const { share, fetch } = await setup()
  share.query = 'Bob'
  await share.search()
  expect(share.results?.items[0]?.id).toBe('bob')
  expect(fetch).toHaveBeenLastCalledWith(
    '/api/v1/directory/users?resource=table%3At&q=Bob',
    expect.objectContaining({
      cache: 'no-store',
      signal: expect.any(AbortSignal),
    }),
  )
  const releases: ((response: Response) => void)[] = []
  fetch.mockImplementation(
    () => new Promise<Response>((resolve) => releases.push(resolve)),
  )
  const old = share.search()
  share.query = 'Carol'
  expect(share.results).toBeNull()
  const newer = share.search()
  releases[1]!(Response.json({ items: [{ id: 'carol' }], hasMore: false }))
  await newer
  releases[0]!(Response.json({ items: [{ id: 'bob' }], hasMore: false }))
  await old
  expect(share.results?.items[0]?.id).toBe('carol')
  share.principalType = 'group'
  expect(share.results).toBeNull()
})

it('serializes reviewed writes, sends CSRF, reconciles nav and reloads without optimistic grants', async () => {
  const { share, fetch } = await setup()
  let release!: (response: Response) => void
  fetch.mockImplementation((path, options?: RequestInit) =>
    options?.method === 'PUT'
      ? new Promise<Response>((resolve) => {
          release = resolve
        })
      : Promise.resolve(
          Response.json(path.endsWith('/nav') ? navigation() : listing()),
        ),
  )
  share.review = change
  const pending = share.commit()
  const duplicate = share.commit()
  share.close()
  expect(share.busy).toBe(true)
  expect(share.target).not.toBeNull()
  expect(share.listing?.items).toEqual([])
  expect(
    fetch.mock.calls.filter(([, options]) => options?.method === 'PUT'),
  ).toHaveLength(1)
  release(Response.json({ eventId: 'opaque' }))
  await pending
  await duplicate
  expect(share.review).toBeNull()
  expect(share.busy).toBe(false)
  expect(fetch).toHaveBeenCalledWith(
    '/api/v1/grants',
    expect.objectContaining({
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        'X-PT-CSRF': '1',
        'X-PT-Event-ID': expect.stringMatching(/^[0-9a-f]{32}$/),
      },
      body: JSON.stringify({
        resourceType: 'table',
        resourceId: 't',
        principalType: 'user',
        principalId: 'bob',
        level: 'read',
      }),
    }),
  )
  expect(fetch.mock.calls.slice(-2).map(([path]) => path)).toEqual([
    '/api/v1/nav',
    '/api/v1/grants?resource=table:t',
  ])
})

it('retains last-owner/disabled/missing errors and reconciles even uncertain writes', async () => {
  const { share, fetch } = await setup()
  for (const code of [
    'ERR_LAST_OWNER',
    'ERR_PRINCIPAL_DISABLED',
    'ERR_PRINCIPAL_NOT_FOUND',
    'network',
  ]) {
    fetch.mockImplementation((path, options?: RequestInit) =>
      options?.method === 'PUT'
        ? code === 'network'
          ? Promise.reject(new TypeError('private network details'))
          : Promise.resolve(
              Response.json(
                { error: { code } },
                { status: code === 'ERR_PRINCIPAL_NOT_FOUND' ? 404 : 409 },
              ),
            )
        : Promise.resolve(
            Response.json(path.endsWith('/nav') ? navigation() : listing()),
          ),
    )
    share.review = change
    await share.commit()
    expect(share.writeError).not.toBeNull()
    expect(share.review).not.toBeNull()
    expect(share.listing?.items).toEqual([])
    expect(fetch.mock.calls.slice(-2).map(([path]) => path)).toEqual([
      '/api/v1/nav',
      '/api/v1/grants?resource=table:t',
    ])
  }
})

it('clears permissions on failed list and nav ownership loss, without recursive forbidden refresh', async () => {
  const { share, fetch } = await setup()
  fetch.mockImplementation((path) =>
    Promise.resolve(
      path.endsWith('/nav')
        ? Response.json(navigation())
        : Response.json(
            { error: { code: 'ERR_PERMISSION_DENIED' } },
            { status: 403 },
          ),
    ),
  )
  await share.load()
  expect(share.error).toMatchObject({ code: 'ERR_PERMISSION_DENIED' })
  expect(share.listing).toBeNull()
  expect(fetch.mock.calls).toHaveLength(3)
  const nav = useNavStore()
  nav.tree.workspaces[0]!.books[0]!.tables[0]!.level = 'read'
  nav.revision++
  await flushPromises()
  expect(share.editable).toBe(false)
  expect(share.error).toMatchObject({ code: 'ERR_PERMISSION_DENIED' })
})

it('does not leak a late mutation into a new identity and revocation clears authentication', async () => {
  const { share, fetch } = await setup()
  let release!: (response: Response) => void
  fetch.mockImplementation(
    () =>
      new Promise<Response>((resolve) => {
        release = resolve
      }),
  )
  share.review = change
  const pending = share.commit()
  useAuthStore().setUser({ ...navUser, id: 'new' })
  release(Response.json({ eventId: 'opaque' }))
  await pending
  expect(share.target).toBeNull()
  expect(fetch.mock.calls).toHaveLength(2)
  fetch.mockImplementation(() =>
    Promise.resolve(
      Response.json(
        { error: { code: 'ERR_UNAUTHENTICATED' } },
        { status: 401 },
      ),
    ),
  )
  share.open({ kind: 'table', id: 't', name: 'Table' })
  await flushPromises()
  expect(useAuthStore().user).toBeNull()
  expect(share.target).toBeNull()
})

it('reconciles an aborted same-identity mutation after route closure without reopening stale sharing', async () => {
  const { share, fetch } = await setup()
  const refresh = vi.spyOn(useNavStore(), 'refresh').mockResolvedValue(true)
  let release!: (response: Response) => void
  fetch.mockImplementation(
    () =>
      new Promise<Response>((resolve) => {
        release = resolve
      }),
  )
  share.review = change
  const pending = share.commit()
  const signal = fetch.mock.calls.at(-1)![1]!.signal
  share.close(true)
  expect(signal?.aborted).toBe(true)
  release(Response.json({ eventId: 'opaque' }))
  await pending
  expect(refresh).toHaveBeenCalledTimes(1)
  expect(share.target).toBeNull()
  expect(share.listing).toBeNull()
  expect(share.writeError).toBeNull()
})

it('marks a forced-password session for the existing authentication guard', async () => {
  const { share, fetch } = await setup()
  fetch.mockImplementation(() =>
    Promise.resolve(
      Response.json(
        {
          error: { code: 'ERR_PASSWORD_CHANGE_REQUIRED' },
        },
        { status: 403 },
      ),
    ),
  )
  await share.load()
  expect(useAuthStore().user?.mustChangePassword).toBe(true)
  expect(share.editable).toBe(false)
  expect(share.listing).toBeNull()
})
