import type { Page } from '@playwright/test'
import type { Components } from '@revolist/revogrid'
import {
  test,
  expect,
  allowHTTPFailure,
  expectBrowserClean,
  observeBrowser,
  paceLogin,
} from './fixtures'
import type { DataRow, TableSchema } from '../src/api/types'
import en from '../src/i18n/en.json' with { type: 'json' }

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

type Frame = { t?: string; tableId?: string }
type SocketWindow = Window & { testRealtimeSocket?: WebSocket }

function trackFrames(page: Page) {
  const frames: Frame[] = []
  page.on('websocket', (socket) => {
    socket.on('framereceived', (frame) => {
      if (typeof frame.payload === 'string')
        frames.push(JSON.parse(frame.payload) as Frame)
    })
  })
  return frames
}

async function login(page: Page, username: string, password: string) {
  await paceLogin(page)
  const response = await page.request.post('/api/v1/auth/login', {
    headers: headers(),
    data: { login: username, password },
  })
  expect(response.status()).toBe(200)
}

async function createTable(page: Page, suffix: string) {
  const workspace = await page.request.post('/api/v1/workspaces', {
    headers: headers(),
    data: { name: `Realtime ${suffix}` },
  })
  expect(workspace.status()).toBe(201)
  const workspaceId = (await workspace.json()).workspace.id as string
  const book = await page.request.post(
    `/api/v1/workspaces/${workspaceId}/books`,
    { headers: headers(), data: { name: 'Realtime book' } },
  )
  expect(book.status()).toBe(201)
  const bookId = (await book.json()).book.id as string
  const table = await page.request.post(`/api/v1/books/${bookId}/tables`, {
    headers: headers(),
    data: { name: 'Realtime table' },
  })
  expect(table.status()).toBe(201)
  const tableId = (await table.json()).table.id as string
  const schemaResponse = await page.request.get(
    `/api/v1/tables/${tableId}/schema`,
  )
  expect(schemaResponse.status()).toBe(200)
  const schema = (await schemaResponse.json()) as TableSchema
  const fieldId = schema.table.displayFieldId
  if (!fieldId) throw new Error('Realtime table has no display field')
  const created = await page.request.post(`/api/v1/tables/${tableId}/rows`, {
    headers: headers(),
    data: { rows: [{ fields: { [fieldId]: 'Initial value' } }] },
  })
  expect(created.status()).toBe(201)
  const row = ((await created.json()).rows as DataRow[])[0]!
  const viewId = schema.views[0]!.id
  return { tableId, fieldId, row, path: `/t/${tableId}/v/${viewId}` }
}

async function waitForSubscription(frames: Frame[], tableId: string) {
  await expect
    .poll(() =>
      frames.some(
        (frame) => frame.t === 'subscribed' && frame.tableId === tableId,
      ),
    )
    .toBe(true)
}

async function openEditor(page: Page, fieldId: string) {
  const grid = page.locator('revo-grid')
  await grid.evaluate(async (element, id) => {
    const component = element as HTMLElement & Components.RevoGrid
    const column = (await component.getColumns()).find(
      (item) => item.prop === id,
    )
    if (!column) throw new Error(`Missing grid column ${id}`)
    const pin = (column.pin ?? 'rgCol') as 'rgCol' | 'colPinStart' | 'colPinEnd'
    const store = await component.getColumnStore(pin)
    const source = store.get('source')
    const x = store
      .get('items')
      .findIndex((index) => source[index]?.prop === id)
    if (x < 0) throw new Error(`Missing visible grid column ${id}`)
    await component.setCellsFocus({ x, y: 0 }, { x, y: 0 }, pin)
    component.focus()
    await component.setCellEdit(0, column.prop)
  }, fieldId)
  const editor = page.getByRole('textbox', {
    name: en.field.editor.value,
    exact: true,
  })
  await expect(editor).toBeVisible()
  return editor
}

test('AT-601/602/603/605/107: live grid, LWW, reconnect and revocation', async ({
  page,
  browser,
  isMobile,
}) => {
  test.skip(isMobile, 'desktop grid acceptance')
  test.setTimeout(180_000)
  const h = headers()
  await login(page, 'first-admin', 'Setup-password-123')
  const { tableId, fieldId, row, path } = await createTable(
    page,
    String(Date.now()),
  )
  const username = `realtime-bob-${Date.now()}`
  const createUser = await page.request.post('/api/v1/admin/users', {
    headers: h,
    data: {
      username,
      displayName: 'Bob',
      role: 'user',
      password: 'Temporary-password-123',
    },
  })
  expect(createUser.status()).toBe(201)
  const userId = (await createUser.json()).user.id as string
  const grant = await page.request.put('/api/v1/grants', {
    headers: h,
    data: {
      resourceType: 'table',
      resourceId: tableId,
      principalType: 'user',
      principalId: userId,
      level: 'write',
    },
  })
  expect(grant.status()).toBe(201)

  const bobContext = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL,
    viewport: { width: 1440, height: 900 },
  })
  const bob = await bobContext.newPage()
  const bobDiagnostics = await observeBrowser(bob)
  let offlineContext: Awaited<ReturnType<typeof browser.newContext>> | undefined
  try {
    await login(bob, username, 'Temporary-password-123')
    const changePassword = await bobContext.request.post(
      '/api/v1/auth/password',
      {
        headers: h,
        data: {
          current: 'Temporary-password-123',
          new: 'Changed-password-123',
        },
      },
    )
    expect(changePassword.status()).toBe(200)
    await login(bob, username, 'Changed-password-123')
    const aliceFrames = trackFrames(page)
    const bobFrames = trackFrames(bob)
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto(path)
    await bob.goto(path)
    await waitForSubscription(aliceFrames, tableId)
    await waitForSubscription(bobFrames, tableId)
    await expect(page.getByText('Initial value', { exact: true })).toBeVisible()
    await expect(bob.getByText('Initial value', { exact: true })).toBeVisible()

    const start = Date.now()
    const update = await bobContext.request.patch(
      `/api/v1/tables/${tableId}/rows/${row.id}`,
      { headers: h, data: { fields: { [fieldId]: 'Bob first' } } },
    )
    expect(update.status()).toBe(200)
    await expect(page.getByText('Bob first', { exact: true })).toBeVisible({
      timeout: 1000,
    })
    expect(Date.now() - start).toBeLessThan(1000)

    const aliceEditor = await openEditor(page, fieldId)
    await aliceEditor.fill('Alice wins')
    const bobEditor = await openEditor(bob, fieldId)
    await bobEditor.fill('Bob saved')
    await bobEditor.press('Enter')
    await expect(
      page.getByRole('status').filter({ hasText: 'Changed by Bob' }),
    ).toBeVisible()
    await expect(aliceEditor).toHaveValue('Alice wins')
    await aliceEditor.press('Enter')
    await expect(page.getByText('Alice wins', { exact: true })).toBeVisible()
    await expect(bob.getByText('Alice wins', { exact: true })).toBeVisible()
    const persisted = await bobContext.request.get(
      `/api/v1/tables/${tableId}/rows/${row.id}`,
    )
    expect(((await persisted.json()) as DataRow).fields[fieldId]).toBe(
      'Alice wins',
    )

    const cancelEditor = await openEditor(page, fieldId)
    await cancelEditor.fill('Discard me')
    const remoteEdit = await bobContext.request.patch(
      `/api/v1/tables/${tableId}/rows/${row.id}`,
      { headers: h, data: { fields: { [fieldId]: 'Bob after escape' } } },
    )
    expect(remoteEdit.status()).toBe(200)
    await expect(
      page.getByRole('status').filter({ hasText: 'Changed by Bob' }),
    ).toBeVisible()
    await cancelEditor.press('Escape')
    await expect(
      page.getByText('Bob after escape', { exact: true }),
    ).toBeVisible()

    const added = await bobContext.request.post(
      `/api/v1/tables/${tableId}/rows`,
      {
        headers: h,
        data: { rows: [{ fields: { [fieldId]: 'Added remotely' } }] },
      },
    )
    expect(added.status()).toBe(201)
    const addedRow = ((await added.json()).rows as DataRow[])[0]!
    await expect(
      page.getByText('Added remotely', { exact: true }),
    ).toBeVisible()
    const deleted = await bobContext.request.delete(
      `/api/v1/tables/${tableId}/rows`,
      {
        headers: h,
        data: { ids: [addedRow.id] },
      },
    )
    expect(deleted.status()).toBe(200)
    await expect(page.getByText('Added remotely', { exact: true })).toHaveCount(
      0,
    )

    let navigations = 0
    bob.on('framenavigated', (frame) => {
      if (frame === bob.mainFrame()) navigations++
    })
    const field = await page.request.post(`/api/v1/tables/${tableId}/fields`, {
      headers: h,
      data: { name: 'Remote field', uiType: 'text' },
    })
    expect(field.status()).toBe(201)
    await expect(
      bob.locator('revo-grid').getByRole('columnheader', {
        name: 'Remote field',
        exact: true,
      }),
    ).toBeVisible()
    expect(navigations).toBe(0)

    offlineContext = await browser.newContext({
      baseURL: process.env.PT_E2E_BASE_URL,
      viewport: { width: 1440, height: 900 },
      storageState: await page.context().storageState(),
    })
    const offline = await offlineContext.newPage()
    const offlineDiagnostics = await observeBrowser(offline)
    await offline.addInitScript(() => {
      const NativeWebSocket = window.WebSocket
      window.WebSocket = class extends NativeWebSocket {
        constructor(url: string | URL, protocols?: string | string[]) {
          super(url, protocols)
          ;(window as SocketWindow).testRealtimeSocket = this
        }
      }
    })
    const offlineFrames = trackFrames(offline)
    await offline.goto(path)
    await waitForSubscription(offlineFrames, tableId)
    await expect(
      offline.getByText('Bob after escape', { exact: true }),
    ).toBeVisible()
    await offlineContext.setOffline(true)
    await offline.evaluate(() => {
      const socket = (window as SocketWindow).testRealtimeSocket
      if (!socket) throw new Error('Realtime socket was not created')
      socket.close()
    })
    const whileOffline = await bobContext.request.patch(
      `/api/v1/tables/${tableId}/rows/${row.id}`,
      { headers: h, data: { fields: { [fieldId]: 'Changed while offline' } } },
    )
    expect(whileOffline.status()).toBe(200)
    await new Promise((resolve) => setTimeout(resolve, 30_000))
    await expect(
      offline.getByText('Bob after escape', { exact: true }),
    ).toBeVisible()
    await offlineContext.setOffline(false)
    await expect(
      offline.getByText('Changed while offline', { exact: true }),
    ).toBeVisible({ timeout: 35_000 })
    expect(
      offlineDiagnostics.errors.filter(
        (error) =>
          !error.includes('net::ERR_INTERNET_DISCONNECTED') &&
          !error.includes('WebSocket connection to'),
      ),
    ).toEqual([])

    allowHTTPFailure(bob, 'GET', '/api/v1/nav', 401)
    const disabledAt = Date.now()
    const disable = await page.request.patch(`/api/v1/admin/users/${userId}`, {
      headers: h,
      data: { status: 'disabled' },
    })
    expect(disable.status()).toBe(200)
    await expect(bob).toHaveURL(/\/login$/, { timeout: 5000 })
    expect(Date.now() - disabledAt).toBeLessThan(5000)
    expect(bobFrames.some((frame) => frame.t === 'session.revoked')).toBe(true)
    expectBrowserClean(bob, bobDiagnostics)
  } finally {
    await offlineContext?.close()
    await bobContext.close()
  }
})

test('AT-601: mobile cards receive live row changes', async ({
  page,
  browser,
  isMobile,
}) => {
  test.skip(!isMobile, 'mobile card acceptance')
  test.setTimeout(90_000)
  await login(page, 'first-admin', 'Setup-password-123')
  const { tableId, fieldId, row, path } = await createTable(
    page,
    `mobile-${Date.now()}`,
  )
  const context = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL,
    viewport: { width: 390, height: 844 },
    isMobile: true,
    hasTouch: true,
    storageState: await page.context().storageState(),
  })
  const other = await context.newPage()
  const diagnostics = await observeBrowser(other)
  try {
    const frames = trackFrames(page)
    await page.goto(path)
    await other.goto(path)
    await waitForSubscription(frames, tableId)
    const card = page.locator(`[data-row-id="${row.id}"]`)
    await expect(card).toContainText('Initial value')
    const update = await other.request.patch(
      `/api/v1/tables/${tableId}/rows/${row.id}`,
      { headers: headers(), data: { fields: { [fieldId]: 'Mobile updated' } } },
    )
    expect(update.status()).toBe(200)
    await expect(card).toContainText('Mobile updated')
    await card.locator('button.mobile-card').click()
    const dialog = page.locator('dialog.row-expand')
    const field = dialog.locator(`[data-field-id="${fieldId}"]`)
    await field.getByRole('button').click()
    const editor = field.locator('.field-editor input')
    await editor.fill('Mobile draft')
    const remoteEdit = await other.request.patch(
      `/api/v1/tables/${tableId}/rows/${row.id}`,
      {
        headers: headers(),
        data: { fields: { [fieldId]: 'Mobile remote' } },
      },
    )
    expect(remoteEdit.status()).toBe(200)
    await expect(field.getByRole('status')).toContainText(
      'Changed by First Administrator',
    )
    await expect(editor).toHaveValue('Mobile draft')
    await field
      .getByRole('button', { name: en.admin.cancel, exact: true })
      .click()
    await expect(field).toContainText('Mobile remote')
    await dialog
      .getByRole('button', { name: en.rowExpand.close, exact: true })
      .click()
    await expect(card).toContainText('Mobile remote')
    const added = await other.request.post(`/api/v1/tables/${tableId}/rows`, {
      headers: headers(),
      data: { rows: [{ fields: { [fieldId]: 'Mobile added' } }] },
    })
    expect(added.status()).toBe(201)
    const addedRow = ((await added.json()).rows as DataRow[])[0]!
    await expect(page.locator(`[data-row-id="${addedRow.id}"]`)).toContainText(
      'Mobile added',
    )
    const deleted = await other.request.delete(
      `/api/v1/tables/${tableId}/rows`,
      {
        headers: headers(),
        data: { ids: [addedRow.id] },
      },
    )
    expect(deleted.status()).toBe(200)
    await expect(page.locator(`[data-row-id="${addedRow.id}"]`)).toHaveCount(0)
    expect(diagnostics.errors).toEqual([])
  } finally {
    await context.close()
  }
})
