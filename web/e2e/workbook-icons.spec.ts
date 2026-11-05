import type { APIRequestContext, BrowserContext } from '@playwright/test'
import type {
  AdminUserMutation,
  Book,
  NavBook,
  Navigation,
} from '../src/api/types'
import { expect, paceLogin, test } from './fixtures'

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

const fallbackIcons = [
  'book',
  'table',
  'view',
  'fields',
  'calendar',
  'star',
  'group',
  'link',
]

async function navigationBook(
  request: APIRequestContext,
  workspaceId: string,
  bookId: string,
): Promise<NavBook> {
  const response = await request.get('/api/v1/nav')
  expect(response.status()).toBe(200)
  const navigation = (await response.json()) as Navigation
  const book = navigation.workspaces
    .find((workspace) => workspace.id === workspaceId)
    ?.books.find((item) => item.id === bookId)
  if (!book) throw new Error('Expected the authorized book in navigation')
  return book
}

test('UX-03: workbook icon metadata survives reload and sharing', async ({
  page,
  browser,
  isMobile,
}) => {
  test.skip(isMobile, 'The icon metadata API is independent of viewport.')
  test.setTimeout(180_000)
  await paceLogin(page)
  const login = await page.request.post('/api/v1/auth/login', {
    headers: headers(),
    data: { login: 'first-admin', password: 'Setup-password-123' },
  })
  expect(login.status()).toBe(200)

  const stamp = Date.now()
  const workspaceResponse = await page.request.post('/api/v1/workspaces', {
    headers: headers(),
    data: { name: `UX03 icons ${stamp}` },
  })
  expect(workspaceResponse.status()).toBe(201)
  const workspaceId = (
    (await workspaceResponse.json()) as { workspace: { id: string } }
  ).workspace.id
  let readerContext: BrowserContext | undefined
  try {
    const bookResponse = await page.request.post(
      `/api/v1/workspaces/${workspaceId}/books`,
      { headers: headers(), data: { name: 'Name-only book' } },
    )
    expect(bookResponse.status()).toBe(201)
    const book = ((await bookResponse.json()) as { book: Book }).book
    expect(book).toHaveProperty('icon', null)
    expect(fallbackIcons).toContain(book.fallbackIcon)
    const fallback = book.fallbackIcon

    await page.goto(`/w/${workspaceId}`)
    await page.reload()
    expect(
      await navigationBook(page.request, workspaceId, book.id),
    ).toMatchObject({
      icon: null,
      fallbackIcon: fallback,
    })

    const username = `ux03-${stamp}`
    const userResponse = await page.request.post('/api/v1/admin/users', {
      headers: headers(),
      data: {
        username,
        displayName: 'Workbook icon reader',
        role: 'user',
        password: 'Temporary-password-123',
      },
    })
    expect(userResponse.status()).toBe(201)
    const user = ((await userResponse.json()) as AdminUserMutation).user
    const grant = await page.request.put('/api/v1/grants', {
      headers: headers(),
      data: {
        resourceType: 'workspace',
        resourceId: workspaceId,
        principalType: 'user',
        principalId: user.id,
        level: 'read',
      },
    })
    expect(grant.status()).toBe(201)

    readerContext = await browser.newContext({
      baseURL: process.env.PT_E2E_BASE_URL,
    })
    await paceLogin()
    const initialLogin = await readerContext.request.post(
      '/api/v1/auth/login',
      {
        headers: headers(),
        data: { login: username, password: 'Temporary-password-123' },
      },
    )
    expect(initialLogin.status()).toBe(200)
    const password = await readerContext.request.post('/api/v1/auth/password', {
      headers: headers(),
      data: { current: 'Temporary-password-123', new: 'Changed-password-123' },
    })
    expect(password.status()).toBe(200)
    await paceLogin()
    const readerLogin = await readerContext.request.post('/api/v1/auth/login', {
      headers: headers(),
      data: { login: username, password: 'Changed-password-123' },
    })
    expect(readerLogin.status()).toBe(200)
    expect(
      await navigationBook(readerContext.request, workspaceId, book.id),
    ).toMatchObject({ icon: null, fallbackIcon: fallback })

    const set = await page.request.patch(`/api/v1/books/${book.id}`, {
      headers: headers(),
      data: { icon: 'star' },
    })
    expect(set.status()).toBe(200)
    expect(((await set.json()) as { book: Book }).book).toMatchObject({
      icon: 'star',
      fallbackIcon: fallback,
    })
    expect(
      await navigationBook(readerContext.request, workspaceId, book.id),
    ).toMatchObject({ icon: 'star', fallbackIcon: fallback })
    const denied = await readerContext.request.patch(
      `/api/v1/books/${book.id}`,
      {
        headers: headers(),
        data: { icon: 'calendar' },
      },
    )
    expect(denied.status()).toBe(403)

    const rename = await page.request.patch(`/api/v1/books/${book.id}`, {
      headers: headers(),
      data: { name: 'Renamed book', index: 0 },
    })
    expect(rename.status()).toBe(200)
    expect(((await rename.json()) as { book: Book }).book).toMatchObject({
      name: 'Renamed book',
      icon: 'star',
      fallbackIcon: fallback,
    })
    const clear = await page.request.patch(`/api/v1/books/${book.id}`, {
      headers: headers(),
      data: { icon: null },
    })
    expect(clear.status()).toBe(200)
    expect(((await clear.json()) as { book: Book }).book).toMatchObject({
      icon: null,
      fallbackIcon: fallback,
    })
    const invalid = await page.request.patch(`/api/v1/books/${book.id}`, {
      headers: headers(),
      data: { icon: 'not-an-icon' },
    })
    expect(invalid.status()).toBe(400)
    await page.reload()
    expect(
      await navigationBook(readerContext.request, workspaceId, book.id),
    ).toMatchObject({ icon: null, fallbackIcon: fallback })
  } finally {
    await readerContext?.close()
    const deleted = await page.request.delete(
      `/api/v1/workspaces/${workspaceId}`,
      {
        headers: headers(),
      },
    )
    expect(deleted.status()).toBe(200)
  }
})
