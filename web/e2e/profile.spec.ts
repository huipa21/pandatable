import type { Browser, BrowserContext, Page } from '@playwright/test'
import {
  test,
  expect,
  expectBrowserClean,
  expectHTTPFailure,
  observeBrowser,
  paceLogin,
} from './fixtures'
import type {
  AdminSettings,
  AuthUser,
  RowQueryResult,
  TableSchema,
} from '../src/api/types'
import { setLocale, setTheme, signOut } from './app-shell'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }
import { restoreSettings } from './settings'

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

async function useEnglish(page: Page) {
  if ((await page.locator('html').getAttribute('lang')) === 'zh-CN')
    await setLocale(page, 'en')
}

async function signIn(page: Page, login: string, password: string) {
  await paceLogin(page)
  await page.goto('/login')
  await useEnglish(page)
  await page.getByLabel(en.auth.login, { exact: true }).fill(login)
  await page.getByLabel(en.auth.password, { exact: true }).fill(password)
  await page.getByRole('button', { name: en.auth.signIn, exact: true }).click()
}

async function setNewPassword(page: Page, current: string, next: string) {
  await expect(page).toHaveURL(/\/change-password$/)
  await useEnglish(page)
  await page.getByLabel(en.auth.currentPassword, { exact: true }).fill(current)
  await page.getByLabel(en.auth.newPassword, { exact: true }).fill(next)
  await page.getByLabel(en.auth.confirmPassword, { exact: true }).fill(next)
  await page
    .getByRole('button', { name: en.auth.changePassword, exact: true })
    .click()
  await expect(page).toHaveURL(/\/login$/)
  await expect(
    page.getByText(en.auth.passwordChanged, { exact: true }),
  ).toBeVisible()
}

async function noOverflow(page: Page) {
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true)
}

async function createUser(
  page: Page,
  username: string,
  email: string,
  displayName: string,
) {
  const response = await page.request.post('/api/v1/admin/users', {
    headers: headers(),
    data: {
      username,
      email,
      displayName,
      role: 'user',
      password: 'Temporary-password-123',
    },
  })
  expect(response.status()).toBe(201)
  return (await response.json()).user as Pick<AuthUser, 'id' | 'username'>
}

async function openProfile(page: Page) {
  await page.goto('/profile')
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
  await useEnglish(page)
  await expect(
    page.getByRole('heading', { name: en.profile.title }),
  ).toBeVisible()
  await expect(page.getByLabel(en.profile.displayName)).toBeVisible()
  await noOverflow(page)
}

async function editDateTime(
  page: Page,
  fieldId: string,
  rowId: string,
  mobile: boolean,
) {
  if (mobile) {
    await page
      .getByRole('button', {
        name: en.rowExpand.open.replace('{id}', rowId),
        exact: true,
      })
      .click()
    const dialog = page.getByRole('dialog', {
      name: en.rowExpand.title.replace('{id}', rowId),
      exact: true,
    })
    const section = dialog.locator(`[data-field-id="${fieldId}"]`)
    await section
      .getByRole('button', {
        name: en.rowExpand.edit.replace('{name}', 'Moment'),
        exact: true,
      })
      .click()
    const input = section.getByRole('textbox', {
      name: en.field.editor.value,
      exact: true,
    })
    await input.fill('2026-01-01 00:30')
    await input.press('Enter')
    await expect(
      dialog.getByText(en.rowExpand.saved, { exact: true }),
    ).toBeVisible()
    return
  }

  const grid = page.locator('revo-grid')
  const header = grid.getByRole('columnheader', {
    name: 'Moment',
    exact: true,
  })
  const row = grid.getByRole('checkbox', {
    name: en.grid.selectRow.replace('{id}', rowId),
    exact: true,
  })
  const [gridBox, headerBox, rowBox] = await Promise.all([
    grid.boundingBox(),
    header.boundingBox(),
    row.boundingBox(),
  ])
  expect(gridBox).not.toBeNull()
  expect(headerBox).not.toBeNull()
  expect(rowBox).not.toBeNull()
  await grid.click({
    position: {
      x: headerBox!.x + headerBox!.width / 2 - gridBox!.x,
      y: rowBox!.y + rowBox!.height / 2 - gridBox!.y,
    },
  })
  const editor = page.getByRole('textbox', {
    name: en.field.editor.value,
    exact: true,
  })
  await expect(editor).toBeVisible()
  await editor.fill('2026-01-01 00:30')
  await editor.press('Enter')
}

async function createTimezoneTable(page: Page) {
  const suffix = Date.now()
  const workspaceResponse = await page.request.post('/api/v1/workspaces', {
    headers: headers(),
    data: { name: `Timezone workspace ${suffix}` },
  })
  expect(workspaceResponse.status()).toBe(201)
  const workspaceId = (await workspaceResponse.json()).workspace.id as string
  const bookResponse = await page.request.post(
    `/api/v1/workspaces/${workspaceId}/books`,
    { headers: headers(), data: { name: `Timezone book ${suffix}` } },
  )
  expect(bookResponse.status()).toBe(201)
  const bookId = (await bookResponse.json()).book.id as string
  const tableResponse = await page.request.post(
    `/api/v1/books/${bookId}/tables`,
    { headers: headers(), data: { name: `Timezone table ${suffix}` } },
  )
  expect(tableResponse.status()).toBe(201)
  const tableId = (await tableResponse.json()).table.id as string
  const schemaResponse = await page.request.get(
    `/api/v1/tables/${tableId}/schema`,
  )
  expect(schemaResponse.status()).toBe(200)
  const schema = (await schemaResponse.json()) as TableSchema
  const fieldResponse = await page.request.post(
    `/api/v1/tables/${tableId}/fields`,
    {
      headers: headers(),
      data: {
        name: 'Moment',
        uiType: 'datetime',
        options: {
          dateFormat: 'iso',
          timeFormat: '24h',
          timezone: 'user',
        },
      },
    },
  )
  expect(fieldResponse.status()).toBe(201)
  const rowResponse = await page.request.post(
    `/api/v1/tables/${tableId}/rows`,
    {
      headers: headers(),
      data: {
        rows: [
          { fields: { [schema.table.displayFieldId!]: 'Timezone sample' } },
        ],
      },
    },
  )
  expect(rowResponse.status()).toBe(201)
  const rowId = ((await rowResponse.json()).rows[0] as { id: string }).id
  const result = await page.request.post(
    `/api/v1/views/${schema.views[0]!.id}/query`,
    {
      headers: headers(),
      data: { offset: 0, limit: 10, includeCount: true },
    },
  )
  expect(result.status()).toBe(200)
  const rows = (await result.json()) as RowQueryResult
  expect(rows.rows).toHaveLength(1)
  return {
    tableId,
    viewId: schema.views[0]!.id,
    rowId,
    momentId: (await fieldResponse.json()).field.id as string,
  }
}

async function newBrowserPage(
  browser: Browser,
  page: Page,
  isMobile: boolean,
  viewport = page.viewportSize() ?? undefined,
): Promise<{
  context: BrowserContext
  page: Page
  diagnostics: Awaited<ReturnType<typeof observeBrowser>>
}> {
  const context = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL,
    locale: 'en-US',
    viewport,
    isMobile,
    hasTouch: isMobile,
  })
  const child = await context.newPage()
  const diagnostics = await observeBrowser(child)
  return { context, page: child, diagnostics }
}

test('AT-102/116/117 M1: self profile, auth browser behavior, and per-user datetime zones', async ({
  page,
  browser,
}) => {
  test.setTimeout(150_000)
  await page.setViewportSize({ width: 1440, height: 900 })
  await signIn(page, 'first-admin', 'Setup-password-123')
  await expect(page).toHaveURL(/\/home$/)

  const settingsResponse = await page.request.get('/api/v1/admin/settings')
  expect(settingsResponse.status()).toBe(200)
  const previousSettings = (await settingsResponse.json()) as AdminSettings
  const aliceName = `alice-${Date.now().toString(36)}`
  const bobName = `bob-${Date.now().toString(36)}`
  await createUser(page, aliceName, `${aliceName}@example.test`, 'Alice')
  const bob = await createUser(page, bobName, `${bobName}@example.test`, 'Bob')
  const aliceBrowser = await newBrowserPage(browser, page, false)
  const bobBrowser = await newBrowserPage(browser, page, false)
  const alicePage = aliceBrowser.page
  const bobPage = bobBrowser.page

  try {
    const defaultResponse = await page.request.patch('/api/v1/admin/settings', {
      headers: headers(),
      data: { defaultTimezone: 'Asia/Shanghai' },
    })
    expect(defaultResponse.status()).toBe(200)

    await signIn(alicePage, aliceName, 'Temporary-password-123')
    await setNewPassword(
      alicePage,
      'Temporary-password-123',
      'Alice-password-123',
    )
    await signIn(alicePage, `${aliceName}@example.test`, 'Alice-password-123')
    await expect(alicePage).toHaveURL(/\/home$/)
    expect(
      (await (await alicePage.request.get('/api/v1/auth/me')).json())
        .effectiveTimezone,
    ).toBe('Asia/Shanghai')

    await alicePage.goto('/profile')
    await expect(alicePage.getByLabel(en.profile.displayName)).toHaveValue(
      'Alice',
    )
    await signOut(alicePage)
    await expect(alicePage).toHaveURL(/\/login$/)
    expect((await alicePage.request.get('/api/v1/auth/me')).status()).toBe(401)
    await alicePage.goBack()
    await expect(alicePage).toHaveURL(/\/login$/)
    await expect(alicePage.getByLabel(en.profile.displayName)).toHaveCount(0)

    expectHTTPFailure(alicePage, 'POST', '/api/v1/auth/login', 401)
    await paceLogin(alicePage)
    await alicePage.getByLabel(en.auth.login).fill(aliceName)
    await alicePage.getByLabel(en.auth.password).fill('Wrong-password-123')
    await alicePage.getByRole('button', { name: en.auth.signIn }).click()
    await expect(
      alicePage
        .getByRole('alert')
        .filter({ hasText: en.errors.ERR_INVALID_CREDENTIALS }),
    ).toContainText(en.errors.ERR_INVALID_CREDENTIALS)
    await signIn(alicePage, aliceName, 'Alice-password-123')
    await expect(alicePage).toHaveURL(/\/home$/)
    await openProfile(alicePage)

    const profileForm = alicePage.locator('main')
    await profileForm.getByLabel(en.profile.displayName).fill('Alice Profile')
    await profileForm
      .getByLabel(en.profile.language, { exact: true })
      .selectOption('zh-CN')
    await profileForm
      .getByLabel(en.profile.theme, { exact: true })
      .selectOption('dark')
    await profileForm
      .getByLabel(en.profile.timezone)
      .selectOption('Asia/Shanghai')
    await alicePage.getByRole('button', { name: en.profile.save }).click()
    await expect(
      alicePage.getByRole('status').filter({ hasText: zh.profile.saved }),
    ).toBeVisible()
    await expect(alicePage.locator('html')).toHaveAttribute('lang', 'zh-CN')
    await expect(alicePage.locator('html')).toHaveAttribute(
      'data-theme',
      'dark',
    )
    expect(
      await alicePage.evaluate(() => ({
        locale: localStorage.getItem('pandatable.locale'),
        theme: localStorage.getItem('pandatable.theme'),
      })),
    ).toEqual({ locale: 'zh-CN', theme: 'dark' })
    await alicePage.reload()
    await expect(alicePage.getByLabel(zh.profile.displayName)).toHaveValue(
      'Alice Profile',
    )
    await setLocale(alicePage, 'en')
    await setTheme(alicePage, 'light')
    await expect(alicePage.locator('html')).toHaveAttribute('lang', 'en')
    await expect(alicePage.locator('html')).toHaveAttribute(
      'data-theme',
      'light',
    )
    await noOverflow(alicePage)

    await alicePage.goto('/change-password')
    await setNewPassword(
      alicePage,
      'Alice-password-123',
      'Alice-new-password-456',
    )
    expectHTTPFailure(alicePage, 'POST', '/api/v1/auth/login', 401)
    await paceLogin(alicePage)
    await alicePage.getByLabel(en.auth.login).fill(aliceName)
    await alicePage.getByLabel(en.auth.password).fill('Alice-password-123')
    await alicePage.getByRole('button', { name: en.auth.signIn }).click()
    await expect(
      alicePage
        .getByRole('alert')
        .filter({ hasText: en.errors.ERR_INVALID_CREDENTIALS }),
    ).toContainText(en.errors.ERR_INVALID_CREDENTIALS)
    await signIn(alicePage, aliceName, 'Alice-new-password-456')
    await expect(alicePage).toHaveURL(/\/home$/)
    await openProfile(alicePage)
    await expect(alicePage.getByLabel(en.profile.displayName)).toHaveValue(
      'Alice Profile',
    )

    await signIn(bobPage, bobName, 'Temporary-password-123')
    await setNewPassword(bobPage, 'Temporary-password-123', 'Bob-password-123')
    await signIn(bobPage, `${bobName}@example.test`, 'Bob-password-123')
    await expect(bobPage).toHaveURL(/\/home$/)
    const bobBeforeDefaultChange = (await (
      await bobPage.request.get('/api/v1/auth/me')
    ).json()) as AuthUser
    expect(bobBeforeDefaultChange).toMatchObject({
      timezone: null,
      effectiveTimezone: 'Asia/Shanghai',
    })

    const table = await createTimezoneTable(alicePage)
    const grant = await alicePage.request.put('/api/v1/grants', {
      headers: headers(),
      data: {
        resourceType: 'table',
        resourceId: table.tableId,
        principalType: 'user',
        principalId: bob.id,
        level: 'read',
      },
    })
    expect(grant.status()).toBe(201)
    await alicePage.goto(`/t/${table.tableId}/v/${table.viewId}`)
    await expect(
      alicePage.getByRole('region', {
        name: en.grid.title,
      }),
    ).toBeVisible()
    await editDateTime(alicePage, table.momentId, table.rowId, false)
    await expect
      .poll(async () => {
        const response = await alicePage.request.post(
          `/api/v1/views/${table.viewId}/query`,
          {
            headers: headers(),
            data: { offset: 0, limit: 10, includeCount: true },
          },
        )
        if (!response.ok()) return null
        const rows = (await response.json()) as RowQueryResult
        return rows.rows[0]?.fields[table.momentId] ?? null
      })
      .toBe('2025-12-31T16:30:00Z')
    const savedRows = await alicePage.request.post(
      `/api/v1/views/${table.viewId}/query`,
      {
        headers: headers(),
        data: { offset: 0, limit: 10, includeCount: true },
      },
    )
    expect(savedRows.status()).toBe(200)
    const savedResult = (await savedRows.json()) as RowQueryResult
    expect(savedResult.rows[0]?.fields[table.momentId]).toBe(
      '2025-12-31T16:30:00Z',
    )

    const changedDefault = await page.request.patch('/api/v1/admin/settings', {
      headers: headers(),
      data: { defaultTimezone: 'Europe/London' },
    })
    expect(changedDefault.status()).toBe(200)
    const movedDefault = (await (
      await bobPage.request.get('/api/v1/auth/me')
    ).json()) as AuthUser
    expect(movedDefault).toMatchObject({
      timezone: null,
      effectiveTimezone: 'Europe/London',
    })
    const aliceOwnZone = (await (
      await alicePage.request.get('/api/v1/auth/me')
    ).json()) as AuthUser
    expect(aliceOwnZone).toMatchObject({
      timezone: 'Asia/Shanghai',
      effectiveTimezone: 'Asia/Shanghai',
    })

    await openProfile(bobPage)
    await bobPage.getByLabel(en.profile.timezone).selectOption('Europe/London')
    await bobPage.getByRole('button', { name: en.profile.save }).click()
    await expect(bobPage.getByRole('status')).toContainText(en.profile.saved)
    const bobOwnZone = (await (
      await bobPage.request.get('/api/v1/auth/me')
    ).json()) as AuthUser
    expect(bobOwnZone).toMatchObject({
      timezone: 'Europe/London',
      effectiveTimezone: 'Europe/London',
    })

    const rowUrl = `/t/${table.tableId}/v/${table.viewId}/r/${table.rowId}`
    await bobPage.goto(rowUrl)
    const rowDialog = bobPage.getByRole('dialog', {
      name: en.rowExpand.title.replace('{id}', table.rowId),
    })
    await expect(rowDialog).toBeVisible()
    await expect(
      rowDialog
        .locator(`[data-field-id="${table.momentId}"]`)
        .locator('.field-value'),
    ).toHaveText('2025-12-31 16:30')
    await noOverflow(bobPage)

    const mobileBrowser = await newBrowserPage(browser, alicePage, true, {
      width: 390,
      height: 844,
    })
    try {
      await mobileBrowser.context.addCookies(
        await alicePage.context().cookies(),
      )
      await openProfile(mobileBrowser.page)
      await expect(
        mobileBrowser.page.getByLabel(en.profile.timezone),
      ).toBeVisible()
      await expect(
        mobileBrowser.page.getByRole('main').getByRole('link', {
          name: en.auth.changePassword,
        }),
      ).toBeVisible()
      await noOverflow(mobileBrowser.page)
      await expectBrowserClean(mobileBrowser.page, mobileBrowser.diagnostics)
    } finally {
      await mobileBrowser.context.close()
    }

    await expectBrowserClean(alicePage, aliceBrowser.diagnostics)
    await expectBrowserClean(bobPage, bobBrowser.diagnostics)
  } finally {
    await restoreSettings(page, previousSettings)
    await Promise.all([
      aliceBrowser.context.close(),
      bobBrowser.context.close(),
    ])
  }
})
