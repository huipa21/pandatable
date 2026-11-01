import type { Browser, BrowserContext, Page } from '@playwright/test'
import {
  expect,
  expectBrowserClean,
  observeBrowser,
  paceLogin,
  test,
} from './fixtures'
import type {
  AdminUser,
  DataRow,
  GrantListing,
  TableSchema,
} from '../src/api/types'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})
const temporary = 'Temporary-password-123'
const password = 'Lifecycle-password-123'

async function login(page: Page, username: string, value: string) {
  await paceLogin(page)
  const response = await page.request.post('/api/v1/auth/login', {
    headers: headers(),
    data: { login: username, password: value },
  })
  expect(response.status()).toBe(200)
}

async function participant(
  browser: Browser,
  username: string,
  isMobile: boolean,
) {
  const context = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL,
    viewport: isMobile
      ? { width: 390, height: 844 }
      : { width: 1440, height: 900 },
    isMobile,
    hasTouch: isMobile,
    locale: 'en-US',
  })
  const page = await context.newPage()
  await login(page, username, temporary)
  const changed = await page.request.post('/api/v1/auth/password', {
    headers: headers(),
    data: { current: temporary, new: password },
  })
  expect(changed.status()).toBe(200)
  await login(page, username, password)
  return { context, page }
}

async function createUser(page: Page, username: string, displayName: string) {
  const response = await page.request.post('/api/v1/admin/users', {
    headers: headers(),
    data: { username, displayName, role: 'user', password: temporary },
  })
  expect(response.status()).toBe(201)
  return (await response.json()).user as AdminUser
}

async function workspace(page: Page, name: string) {
  const created = await page.request.post('/api/v1/workspaces', {
    headers: headers(),
    data: { name },
  })
  expect(created.status()).toBe(201)
  const workspaceId = (await created.json()).workspace.id as string
  const book = await page.request.post(
    `/api/v1/workspaces/${workspaceId}/books`,
    {
      headers: headers(),
      data: { name: `${name} book` },
    },
  )
  expect(book.status()).toBe(201)
  const bookId = (await book.json()).book.id as string
  const table = await page.request.post(`/api/v1/books/${bookId}/tables`, {
    headers: headers(),
    data: { name: `${name} table` },
  })
  expect(table.status()).toBe(201)
  return { workspaceId, schema: (await table.json()) as TableSchema }
}

async function grantOwner(page: Page, workspaceId: string, userId: string) {
  const response = await page.request.put('/api/v1/grants', {
    headers: headers(),
    data: {
      resourceType: 'workspace',
      resourceId: workspaceId,
      principalType: 'user',
      principalId: userId,
      level: 'owner',
    },
  })
  expect(response.status()).toBe(201)
}

async function relinquishOwner(
  page: Page,
  workspaceId: string,
  userId: string,
) {
  const response = await page.request.get(
    `/api/v1/grants?resource=workspace:${workspaceId}`,
  )
  expect(response.status()).toBe(200)
  const grants = (await response.json()) as GrantListing
  const own = grants.items.find((item) => item.principalId === userId)
  expect(own).toBeDefined()
  const removed = await page.request.delete(`/api/v1/grants/${own!.id}`, {
    headers: headers(),
  })
  expect(removed.status()).toBe(200)
}

async function disable(page: Page, user: AdminUser) {
  const response = await page.request.patch(`/api/v1/admin/users/${user.id}`, {
    headers: headers(),
    data: { status: 'disabled' },
  })
  expect(response.status()).toBe(200)
}

test('AT-113..115: rescue, transfer, and irreversible deletion preserve shared data', async ({
  page,
  browser,
  isMobile,
}) => {
  test.setTimeout(240_000)
  await login(page, 'first-admin', 'Setup-password-123')
  const me = await page.request.get('/api/v1/auth/me')
  expect(me.status()).toBe(200)
  const adminId = (await me.json()).id as string
  const suffix = `${isMobile ? 'm' : 'd'}-${Date.now()}`
  const transferUser = await createUser(
    page,
    `life-transfer-${suffix}`,
    'Transfer owner',
  )
  const alice = await createUser(
    page,
    `life-alice-${suffix}`,
    'Alice Lifecycle',
  )
  const bob = await createUser(page, `life-bob-${suffix}`, 'Bob Lifecycle')

  const rescueName = `Lifecycle rescue ${suffix}`
  const transferName = `Lifecycle transfer ${suffix}`
  const rescueWorkspace = await workspace(page, rescueName)
  const transferWorkspace = await workspace(page, transferName)
  for (const item of [rescueWorkspace, transferWorkspace]) {
    await grantOwner(page, item.workspaceId, transferUser.id)
    await relinquishOwner(page, item.workspaceId, adminId)
    expect(
      (
        await page.request.get(`/api/v1/tables/${item.schema.table.id}/schema`)
      ).status(),
    ).toBe(404)
  }

  let aliceContext: BrowserContext | undefined
  let bobContext: BrowserContext | undefined
  try {
    const aliceSession = await participant(browser, alice.username, isMobile)
    aliceContext = aliceSession.context
    const bobSession = await participant(browser, bob.username, isMobile)
    bobContext = bobSession.context
    const alicePage = aliceSession.page
    const bobPage = bobSession.page
    const bobDiagnostics = await observeBrowser(bobPage)

    const soleName = `Lifecycle sole ${suffix}`
    const sharedName = `Lifecycle shared ${suffix}`
    const sole = await workspace(alicePage, soleName)
    const shared = await workspace(alicePage, sharedName)
    await grantOwner(alicePage, shared.workspaceId, bob.id)
    const fieldId = shared.schema.table.displayFieldId
    if (!fieldId) throw new Error('Shared table has no display field')
    const created = await alicePage.request.post(
      `/api/v1/tables/${shared.schema.table.id}/rows`,
      {
        headers: headers(),
        data: { rows: [{ fields: { [fieldId]: 'shared data survives' } }] },
      },
    )
    expect(created.status()).toBe(201)
    const row = ((await created.json()).rows as DataRow[])[0]!
    const personal = await alicePage.request.post(
      `/api/v1/tables/${shared.schema.table.id}/views`,
      {
        headers: headers(),
        data: { name: 'Alice personal view', scope: 'personal', type: 'grid' },
      },
    )
    expect(personal.status()).toBe(201)
    const personalId = (await personal.json()).view.id as string

    await disable(page, transferUser)
    expect(
      (
        await bobPage.request.get(
          `/api/v1/admin/users/${transferUser.id}/ownerships`,
        )
      ).status(),
    ).toBe(403)
    await page.goto('/admin/workspaces')
    await page
      .getByLabel(en.workspaces.searchHint, { exact: true })
      .fill(rescueName)
    await page
      .getByRole('button', { name: en.admin.search, exact: true })
      .click()
    const card = page.locator(
      `[data-workspace="${rescueWorkspace.workspaceId}"]`,
    )
    await expect(card).toContainText(en.workspaces.orphaned)
    await card
      .getByRole('button', { name: en.lifecycle.rescue, exact: true })
      .click()
    await card
      .getByLabel(en.lifecycle.selectUser, { exact: true })
      .selectOption(bob.id)
    await card
      .getByRole('button', { name: en.lifecycle.rescue, exact: true })
      .click()
    await expect(card).toContainText(en.workspaces.owned)
    await expect(
      page.getByText(en.lifecycle.rescued, { exact: true }),
    ).toBeVisible()
    const rescuedSchema = await bobPage.request.get(
      `/api/v1/tables/${rescueWorkspace.schema.table.id}/schema`,
    )
    expect(rescuedSchema.status()).toBe(200)
    expect((await rescuedSchema.json()).myLevel).toBe('owner')
    expect(
      (
        await page.request.get(
          `/api/v1/tables/${rescueWorkspace.schema.table.id}/schema`,
        )
      ).status(),
    ).toBe(404)

    await page.goto(`/admin/users/${transferUser.id}`)
    const ownerships = page.getByRole('region', {
      name: en.lifecycle.ownerships,
    })
    await expect(ownerships).toContainText(rescueName)
    await expect(ownerships).toContainText(transferName)
    await ownerships
      .getByRole('button', { name: en.lifecycle.transfer, exact: true })
      .click()
    await ownerships
      .getByLabel(en.lifecycle.selectUser, { exact: true })
      .selectOption(bob.id)
    await ownerships
      .getByRole('button', { name: en.lifecycle.transfer, exact: true })
      .click()
    await expect(ownerships).toContainText(en.lifecycle.transferred)
    await expect(ownerships).toContainText(en.lifecycle.noOwnerships)
    const transferredSchema = await bobPage.request.get(
      `/api/v1/tables/${transferWorkspace.schema.table.id}/schema`,
    )
    expect(transferredSchema.status()).toBe(200)
    expect((await transferredSchema.json()).myLevel).toBe('owner')

    let connected = false
    bobPage.on('websocket', () => {
      connected = true
    })
    const viewId = shared.schema.views[0]!.id
    const base = `/t/${shared.schema.table.id}/v/${viewId}`
    await bobPage.goto(base)
    await expect.poll(() => connected).toBe(true)
    if (isMobile) {
      await bobPage
        .locator(`[data-row-id="${row.id}"] button.mobile-card`)
        .click()
    } else {
      await bobPage
        .getByRole('button', {
          name: en.rowExpand.open.replace('{id}', row.id),
          exact: true,
        })
        .click()
    }
    const dialog = bobPage.locator('dialog.row-expand')
    const createdBy = shared.schema.fields.find(
      (field) => field.uiType === 'created_by',
    )
    if (!createdBy) throw new Error('Shared table has no created-by field')
    const author = dialog.locator(`[data-field-id="${createdBy.id}"]`)
    await expect(author).toContainText('Alice Lifecycle')
    await disable(page, alice)
    expect((await alicePage.request.get('/api/v1/auth/me')).status()).toBe(401)
    await page.goto(`/admin/users/${alice.id}`)
    const deleting = page.getByRole('region', { name: en.lifecycle.ownerships })
    await expect(deleting).toContainText(soleName)
    await expect(deleting).toContainText(sharedName)
    await deleting
      .getByRole('button', { name: en.lifecycle.delete, exact: true })
      .click()
    const confirmation = deleting.getByLabel(en.lifecycle.confirmUsername, {
      exact: true,
    })
    const confirmButton = deleting.getByRole('button', {
      name: en.lifecycle.deleteConfirm,
      exact: true,
    })
    await confirmation.fill(alice.username.toUpperCase())
    await expect(confirmButton).toBeDisabled()
    await confirmation.fill(alice.username)
    await expect(confirmButton).toBeEnabled()
    await confirmButton.click()
    await expect(page).toHaveURL(/\/admin\/users$/)
    await expect(author).toContainText(en.field.user.deleted, {
      timeout: 15_000,
    })

    const surviving = await bobPage.request.get(
      `/api/v1/tables/${shared.schema.table.id}/rows/${row.id}`,
    )
    expect(surviving.status()).toBe(200)
    expect(((await surviving.json()) as DataRow).fields[fieldId]).toBe(
      'shared data survives',
    )
    const survivingSchema = await bobPage.request.get(
      `/api/v1/tables/${shared.schema.table.id}/schema`,
    )
    expect(survivingSchema.status()).toBe(200)
    expect(
      ((await survivingSchema.json()) as TableSchema).views.map(
        (view) => view.id,
      ),
    ).not.toContain(personalId)
    const soleInventory = await page.request.get(
      `/api/v1/admin/workspaces?q=${encodeURIComponent(soleName)}`,
    )
    expect(soleInventory.status()).toBe(200)
    expect((await soleInventory.json()).items).toEqual([])
    expect(
      (await page.request.get(`/api/v1/admin/users/${alice.id}`)).status(),
    ).toBe(404)
    expect(
      (
        await page.request.get(
          `/api/v1/tables/${shared.schema.table.id}/schema`,
        )
      ).status(),
    ).toBe(404)
    expect(
      (
        await bobPage.request.get(
          `/api/v1/tables/${sole.schema.table.id}/schema`,
        )
      ).status(),
    ).toBe(404)

    await dialog
      .getByRole('button', { name: en.rowExpand.close, exact: true })
      .click()
    await bobPage
      .getByRole('button', { name: en.common.languages.zhCN, exact: true })
      .click()
    await bobPage.goto(`${base}/r/${row.id}`)
    await expect(
      bobPage.locator(`dialog.row-expand [data-field-id="${createdBy.id}"]`),
    ).toContainText(zh.field.user.deleted)
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true)
    expectBrowserClean(bobPage, bobDiagnostics)
  } finally {
    await aliceContext?.close()
    await bobContext?.close()
  }
})
