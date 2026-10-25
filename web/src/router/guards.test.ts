import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { router } from './index'

beforeEach(async () => {
  setActivePinia(createPinia())
  await router.push('/connection-error')
})
afterEach(() => vi.unstubAllGlobals())

function responses(user: { role: string; mustChangePassword: boolean } | null) {
  vi.stubGlobal(
    'fetch',
    vi.fn((path: string) =>
      Promise.resolve(
        path.endsWith('/setup/status')
          ? Response.json({ needed: false })
          : user
            ? Response.json(user)
            : Response.json(
                { error: { code: 'ERR_UNAUTHENTICATED' } },
                { status: 401 },
              ),
      ),
    ),
  )
}

it('redirects unauthenticated and non-admin deep admin routes', async () => {
  responses(null)
  for (const path of [
    '/admin/users',
    '/admin/groups',
    '/admin/groups/group-id',
    '/admin/users/user-id',
  ]) {
    await router.push(path)
    expect(router.currentRoute.value.path).toBe('/login')
  }
  responses({ role: 'user', mustChangePassword: false })
  for (const path of [
    '/admin/future',
    '/admin/groups',
    '/admin/groups/group-id',
    '/admin/users/user-id',
  ]) {
    await router.push(path)
    expect(router.currentRoute.value.path).toBe('/home')
  }
})

it('forces password change before every other client route, even the spike', async () => {
  responses({ role: 'admin', mustChangePassword: true })
  await router.push('/_spikes/revogrid')
  expect(router.currentRoute.value.path).toBe('/change-password')
  await router.push('/setup')
  expect(router.currentRoute.value.path).toBe('/change-password')
})

it('lets cleared admins access the user panel and exposes network failures', async () => {
  responses({ role: 'admin', mustChangePassword: false })
  await router.push('/admin/users')
  expect(router.currentRoute.value.path).toBe('/admin/users')
  vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('offline')))
  await router.push('/home')
  expect(router.currentRoute.value.path).toBe('/connection-error')
})
