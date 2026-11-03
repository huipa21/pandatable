import type { Page } from '@playwright/test'
import {
  test,
  expect,
  observeBrowser,
  expectBrowserClean,
  paceLogin,
} from './fixtures'
import type { DataRow, TableSchema } from '../src/api/types'
import { openAccountMenu } from './app-shell'
import en from '../src/i18n/en.json' with { type: 'json' }

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

async function login(page: Page, username: string, password: string) {
  await paceLogin(page)
  const response = await page.request.post('/api/v1/auth/login', {
    headers: headers(),
    data: { login: username, password },
  })
  expect(response.status()).toBe(200)
}

test('AT-606: accessible mentions arrive live and open the exact row comment', async ({
  page,
  browser,
  isMobile,
}) => {
  test.setTimeout(180_000)
  await login(page, 'first-admin', 'Setup-password-123')
  const workspace = await page.request.post('/api/v1/workspaces', {
    headers: headers(),
    data: { name: `Comments ${isMobile ? 'mobile' : 'desktop'}` },
  })
  expect(workspace.status()).toBe(201)
  const workspaceId = (await workspace.json()).workspace.id as string
  const book = await page.request.post(
    `/api/v1/workspaces/${workspaceId}/books`,
    { headers: headers(), data: { name: 'Comments book' } },
  )
  expect(book.status()).toBe(201)
  const bookId = (await book.json()).book.id as string
  const tableResponse = await page.request.post(
    `/api/v1/books/${bookId}/tables`,
    { headers: headers(), data: { name: 'Comments table' } },
  )
  expect(tableResponse.status()).toBe(201)
  const tableId = (await tableResponse.json()).table.id as string
  const schemaResponse = await page.request.get(
    `/api/v1/tables/${tableId}/schema`,
  )
  expect(schemaResponse.status()).toBe(200)
  const schema = (await schemaResponse.json()) as TableSchema
  const viewId = schema.views[0]!.id
  const fieldId = schema.table.displayFieldId
  if (!fieldId) throw new Error('Comments table has no display field')
  const createdRow = await page.request.post(`/api/v1/tables/${tableId}/rows`, {
    headers: headers(),
    data: { rows: [{ fields: { [fieldId]: 'Review this' } }] },
  })
  expect(createdRow.status()).toBe(201)
  const row = ((await createdRow.json()).rows as DataRow[])[0]!

  const bobName = isMobile ? 'bob-mention-mobile' : 'bob'
  const outsiderName = isMobile ? 'bobby-mention-mobile' : 'bobby'
  const createUser = async (username: string, displayName: string) => {
    const response = await page.request.post('/api/v1/admin/users', {
      headers: headers(),
      data: {
        username,
        displayName,
        role: 'user',
        password: 'Temporary-password-123',
      },
    })
    expect(response.status()).toBe(201)
    return (await response.json()).user.id as string
  }
  const bobId = await createUser(bobName, 'Bob')
  await createUser(outsiderName, 'Bobby without access')
  const grant = await page.request.put('/api/v1/grants', {
    headers: headers(),
    data: {
      resourceType: 'table',
      resourceId: tableId,
      principalType: 'user',
      principalId: bobId,
      level: 'read',
    },
  })
  expect(grant.status()).toBe(201)

  const bobContext = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL,
    viewport: isMobile
      ? { width: 390, height: 844 }
      : { width: 1440, height: 900 },
    isMobile,
    hasTouch: isMobile,
    locale: 'en-US',
  })
  const bob = await bobContext.newPage()
  const bobDiagnostics = await observeBrowser(bob)
  const received: { t?: string; tableId?: string }[] = []
  const sent: { t?: string; tableId?: string }[] = []
  let connected = false
  bob.on('websocket', (socket) => {
    connected = true
    socket.on('framereceived', (frame) => {
      if (typeof frame.payload === 'string')
        received.push(JSON.parse(frame.payload))
    })
    socket.on('framesent', (frame) => {
      if (typeof frame.payload === 'string')
        sent.push(JSON.parse(frame.payload))
    })
  })
  try {
    await login(bob, bobName, 'Temporary-password-123')
    const changed = await bob.request.post('/api/v1/auth/password', {
      headers: headers(),
      data: { current: 'Temporary-password-123', new: 'Changed-password-123' },
    })
    expect(changed.status()).toBe(200)
    await login(bob, bobName, 'Changed-password-123')
    await bob.goto('/home')
    await expect.poll(() => connected).toBe(true)
    await openAccountMenu(bob)
    const bell = bob.getByRole('button', {
      name: en.notifications.button.replace('{count}', '0'),
    })
    await expect(bell).toBeVisible()

    await page.goto(`/t/${tableId}/v/${viewId}`)
    if (isMobile) {
      await page.locator(`[data-row-id="${row.id}"] button.mobile-card`).click()
    } else {
      await page
        .getByRole('button', {
          name: en.rowExpand.open.replace('{id}', row.id),
          exact: true,
        })
        .click()
    }
    const aliceDialog = page.locator('dialog.row-expand')
    await aliceDialog
      .getByRole('tab', { name: en.comments.title, exact: true })
      .click()
    const draft = aliceDialog.getByLabel(en.comments.compose, { exact: true })
    await draft.fill('@bo')
    const choices = aliceDialog.locator('[role="option"]')
    await expect(choices.filter({ hasText: 'Bob (' })).toHaveCount(1)
    await expect(
      choices.filter({ hasText: 'Bobby without access' }),
    ).toHaveCount(0)
    await choices.filter({ hasText: 'Bob (' }).click()
    await expect(draft).toHaveValue(`@${bobName} `)
    await draft.fill(`@${bobName} please check`)
    await aliceDialog
      .getByRole('button', { name: en.comments.post, exact: true })
      .click()
    const aliceComment = aliceDialog
      .locator('.comment-list [data-comment-id]')
      .first()
    await expect(aliceComment.locator('.comment-body a')).toHaveText(
      `@${bobName}`,
    )
    const commentId = await aliceComment.getAttribute('data-comment-id')
    expect(commentId).toBeTruthy()
    await expect(
      page.locator(
        `[aria-label="${en.comments.count.replace('{count}', '1')}"]`,
      ),
    ).toHaveCount(1)

    await expect
      .poll(() =>
        received.some(
          (frame) => frame.t === 'notification' && frame.tableId === tableId,
        ),
      )
      .toBe(true)
    expect(
      sent.some((frame) => frame.t === 'sub' && frame.tableId === tableId),
    ).toBe(false)
    const unreadBell = bob.getByRole('button', {
      name: en.notifications.button.replace('{count}', '1'),
    })
    await expect(
      bob.getByRole('button', {
        name: en.account.menuUnread
          .replace('{name}', 'Bob')
          .replace('{count}', '1'),
        exact: true,
      }),
    ).toBeVisible()
    await openAccountMenu(bob)
    await expect(unreadBell).toBeVisible()
    await unreadBell.click()
    await bob.locator('.notification-list li button').click()
    await expect(bob).toHaveURL(
      new RegExp(
        `/t/${tableId}/v/${viewId}/r/${row.id}\\?comment=${commentId}`,
      ),
    )
    const bobDialog = bob.locator('dialog.row-expand')
    await expect(bobDialog).toBeVisible()
    const focused = bobDialog.locator(`[data-comment-id="${commentId}"]`)
    await expect(focused).toBeFocused()
    await expect(focused).toContainText(`@${bobName} please check`)
    // The row dialog is modal, so read the avatar's unread summary instead.
    await expect(bob.locator('.avatar-button')).toHaveAttribute(
      'aria-label',
      en.account.menu.replace('{name}', 'Bob'),
    )
    const notifications = await bob.request.get('/api/v1/notifications')
    expect((await notifications.json()).unreadCount).toBe(0)

    await bobDialog
      .getByLabel(en.comments.compose, { exact: true })
      .fill('Thanks')
    await bobDialog
      .getByRole('button', { name: en.comments.post, exact: true })
      .click()
    await expect(aliceDialog.locator('.comment-list')).toContainText('Thanks')
    await expect(
      page.locator(
        `[aria-label="${en.comments.count.replace('{count}', '2')}"]`,
      ),
    ).toHaveCount(1)
    expectBrowserClean(bob, bobDiagnostics)
  } finally {
    await bobContext.close()
  }
})
