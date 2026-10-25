import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from './auth'
import { errorMessage, RequestError } from '../api/client'
import { i18n, loadLocale } from '../i18n'
import type { AuthUser } from '../api/types'

const user: AuthUser = {
  id: 'user-id',
  username: 'alice',
  email: null,
  displayName: 'Alice',
  role: 'user',
  mustChangePassword: true,
  locale: 'en',
  theme: 'system',
  timezone: null,
  effectiveTimezone: 'Asia/Shanghai',
  avatarColor: 'teal',
}

beforeEach(() => setActivePinia(createPinia()))
afterEach(() => vi.unstubAllGlobals())

it('uses same-origin CSRF login, never persists credentials, and surfaces service failures', async () => {
  const fetch = vi
    .fn()
    .mockResolvedValue(Response.json({ user, eventId: 'event' }))
  vi.stubGlobal('fetch', fetch)
  const auth = useAuthStore()
  await auth.login({ login: 'alice', password: 'Temporary-123' })
  expect(auth.user).toEqual(user)
  expect(fetch).toHaveBeenCalledWith(
    '/api/v1/auth/login',
    expect.objectContaining({
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json', 'X-PT-CSRF': '1' },
      body: JSON.stringify({ login: 'alice', password: 'Temporary-123' }),
    }),
  )
  fetch.mockResolvedValue(
    Response.json({ error: { code: 'ERR_AUTH_BUSY' } }, { status: 503 }),
  )
  await expect(auth.logout()).rejects.toMatchObject({ code: 'ERR_AUTH_BUSY' })
  expect(auth.user).toEqual(user)
  expect(JSON.stringify(localStorage)).not.toContain('Temporary-123')
})

it('treats only unauthenticated responses as signed-out; fails explicitly otherwise', async () => {
  const fetch = vi
    .fn()
    .mockResolvedValue(
      Response.json(
        { error: { code: 'ERR_UNAUTHENTICATED' } },
        { status: 401 },
      ),
    )
  vi.stubGlobal('fetch', fetch)
  const auth = useAuthStore()
  auth.setUser(user)
  await auth.refresh()
  expect(auth.user).toBeNull()
  fetch.mockRejectedValue(new TypeError('network'))
  await expect(auth.refresh()).rejects.toThrow('network')
})

it('ignores an identity response that began before sign-out', async () => {
  let finish!: (value: Response) => void
  vi.stubGlobal(
    'fetch',
    vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          finish = resolve
        }),
    ),
  )
  const auth = useAuthStore()
  const refresh = auth.refresh()
  auth.setUser(null)
  finish(Response.json(user))
  await refresh
  expect(auth.user).toBeNull()
})

it('translates typed errors in both locales without rendering server messages', async () => {
  for (const locale of ['en', 'zh-CN'] as const) {
    await loadLocale(locale)
    i18n.global.locale.value = locale
    expect(errorMessage(new RequestError('ERR_LAST_ADMIN', 409))).toBe(
      i18n.global.t('errors.ERR_LAST_ADMIN'),
    )
    expect(
      errorMessage(new RequestError('<script>unknown</script>', 500)),
    ).toBe(i18n.global.t('errors.ERR_INTERNAL'))
  }
})
