import type { Page } from '@playwright/test'
import type {
  AdminUserMutation,
  DataRow,
  Navigation,
  TableSchema,
  WorkspacePreference,
} from '../src/api/types'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }
import {
  expect,
  expectBrowserClean,
  expectHTTPFailure,
  observeBrowser,
  paceLogin,
  test,
} from './fixtures'
import { openNodeActions, setLocale, setTheme, signOut } from './app-shell'

const preferencePath = '/api/v1/auth/workspace-preference'
const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})
const cleanupStates = new WeakMap<
  Page,
  { original: string; workspaceIds: string[] }
>()

test.afterEach(async ({ page }) => {
  const state = cleanupStates.get(page)
  if (!state) return

  const restore = () =>
    page.request.put(preferencePath, {
      headers: headers(),
      data: { workspaceId: state.original },
    })
  let restored = await restore()
  if (restored.status() === 401) {
    await paceLogin()
    const login = await page.request.post('/api/v1/auth/login', {
      headers: headers(),
      data: { login: 'first-admin', password: 'Setup-password-123' },
    })
    expect(login.status()).toBe(200)
    restored = await restore()
  }
  expect(restored.status()).toBe(200)
  for (const id of state.workspaceIds) {
    const deleted = await page.request.delete(`/api/v1/workspaces/${id}`, {
      headers: headers(),
    })
    expect(deleted.status()).toBe(200)
  }
})

async function login(page: Page) {
  await paceLogin(page)
  await page.goto('/login')
  await page.getByLabel(en.auth.login, { exact: true }).fill('first-admin')
  await page
    .getByLabel(en.auth.password, { exact: true })
    .fill('Setup-password-123')
  await page.getByRole('button', { name: en.auth.signIn, exact: true }).click()
  await expect(page).toHaveURL(/\/w\/[^/]+$/)
}

async function tree(page: Page, mobile: boolean, locale = en) {
  if (mobile) {
    const drawer = page.locator('.nav-drawer')
    if (!(await drawer.isVisible()))
      await page
        .getByRole('button', { name: locale.nav.open, exact: true })
        .click()
    await expect(drawer).toBeVisible()
    return drawer.locator('.nav-tree')
  }
  return page.locator('.sidebar-desktop .nav-tree')
}

async function closeDrawer(page: Page, mobile: boolean) {
  if (!mobile) return
  await page.keyboard.press('Escape')
  await expect(page.locator('.nav-drawer')).not.toBeVisible()
  await expect(
    page.getByRole('button', { name: en.nav.open, exact: true }),
  ).toBeFocused()
}

test('UX-02: workspace navigation, preference, deep links and restricted ancestry', async ({
  page,
  browser,
  isMobile,
}, testInfo) => {
  test.setTimeout(300_000)
  if (isMobile) await page.setViewportSize({ width: 390, height: 844 })
  await login(page)
  const original = (
    (await (
      await page.request.get(preferencePath)
    ).json()) as WorkspacePreference
  ).workspaceId
  expect(original).toBeTruthy()
  if (!original) throw new Error('Expected a preferred workspace after login')
  const cleanup: { original: string; workspaceIds: string[] } = {
    original,
    workspaceIds: [],
  }
  cleanupStates.set(page, cleanup)
  expect(new URL(page.url()).pathname).toBe(`/w/${original}`)
  await expect(page.locator('main')).not.toContainText(en.auth.homeTitle)
  await expect(page.locator('main')).not.toContainText(en.nav.chooseWorkspace)

  const suffix = `${Date.now()}-${testInfo.project.name.startsWith('mobile') ? 'm' : 'd'}`
  const created = await page.request.post('/api/v1/workspaces', {
    headers: headers(),
    data: { name: `UX02 first ${suffix}` },
  })
  expect(created.status()).toBe(201)
  const first = ((await created.json()) as { workspace: { id: string } })
    .workspace.id
  cleanup.workspaceIds.push(first)
  await page.reload()
  let sidebar = await tree(page, isMobile)
  await expect(sidebar.locator('.workspace-heading h2')).toHaveText(
    en.workspaceGrid.ux02.workspaces,
  )
  if (isMobile) {
    const add = await sidebar.locator('.workspace-heading button').boundingBox()
    const expand = await sidebar
      .locator(`[data-nav-id="${first}"] .workspace-toggle`)
      .boundingBox()
    expect(add?.width).toBeGreaterThanOrEqual(44)
    expect(add?.height).toBeGreaterThanOrEqual(44)
    expect(expand?.width).toBeGreaterThanOrEqual(44)
    expect(expand?.height).toBeGreaterThanOrEqual(44)
  }
  await sidebar
    .locator('.workspace-heading')
    .getByRole('button', { name: en.nav.create.workspace, exact: true })
    .click()
  const structure = page.getByRole('dialog', { name: en.nav.dialogTitle })
  await structure
    .getByRole('textbox', { name: en.nav.name })
    .fill(`UX02 second ${suffix}`)
  const workspaceCreated = page.waitForResponse(
    (response) =>
      response.request().method() === 'POST' &&
      new URL(response.url()).pathname === '/api/v1/workspaces',
  )
  await structure.getByRole('button', { name: en.admin.confirm }).click()
  const secondResponse = await workspaceCreated
  expect(secondResponse.status()).toBe(201)
  const second = (
    (await secondResponse.json()) as { workspace: { id: string } }
  ).workspace.id
  cleanup.workspaceIds.push(second)
  await expect(structure).not.toBeVisible()
  await expect(page).toHaveURL(new RegExp(`/w/${second}$`))
  if (isMobile) await expect(page.locator('.nav-drawer')).not.toBeVisible()
  expect(second).not.toBe(first)

  sidebar = await tree(page, isMobile)
  const serverTree = (await (
    await page.request.get('/api/v1/nav')
  ).json()) as Navigation
  await expect(sidebar.locator('.workspace-root > li')).toHaveCount(
    serverTree.workspaces.length,
  )
  const listed = await sidebar
    .locator('.workspace-root > li')
    .evaluateAll((nodes) =>
      nodes.map((node) => node.getAttribute('data-nav-id')),
    )
  expect(listed).toEqual(serverTree.workspaces.map((item) => item.id))
  for (const id of [original, first, second])
    await expect(sidebar.locator(`[data-nav-id="${id}"]`)).toBeVisible()

  const renamed = `UX02 renamed ${suffix}`
  const actions = sidebar.locator(`[data-nav-id="${first}"] > .node-actions`)
  await openNodeActions(actions)
  await actions
    .getByRole('button', { name: en.nav.rename, exact: true })
    .click()
  await structure.getByRole('textbox', { name: en.nav.name }).fill(renamed)
  await structure.getByRole('button', { name: en.admin.confirm }).click()
  await expect(structure).not.toBeVisible()
  await expect(
    sidebar.locator(`[data-nav-id="${first}"] .workspace-link`),
  ).toHaveText(renamed)
  await openNodeActions(actions)
  await actions
    .getByRole('button', {
      name: en.workspaceGrid.ux02.setDefault,
      exact: true,
    })
    .click()
  await expect
    .poll(
      async () =>
        (
          (await (
            await page.request.get(preferencePath)
          ).json()) as WorkspacePreference
        ).workspaceId,
    )
    .toBe(first)
  await expect(
    sidebar.locator(`[data-nav-id="${first}"] .workspace-default`),
  ).toHaveAttribute(
    'aria-label',
    en.workspaceGrid.ux02.defaultMarker.replace('{name}', renamed),
  )
  await expect(
    sidebar.locator(`[data-nav-id="${second}"] > .workspace-box`),
  ).toHaveClass(/current/)
  expect(new URL(page.url()).pathname).toBe(`/w/${second}`)
  await closeDrawer(page, isMobile)

  if (!isMobile) {
    await page.getByRole('button', { name: en.nav.collapseSidebar }).click()
    await expect(page.locator('.sidebar-desktop')).not.toBeVisible()
    await expect(page.locator('.avatar-button')).toBeVisible()
    await page.getByRole('button', { name: en.nav.expandSidebar }).click()
  }
  await page.locator('.brand').click()
  await expect(page).toHaveURL(new RegExp(`/w/${first}$`))
  await signOut(page)
  await expect(page).toHaveURL(/\/login$/)
  await login(page)
  await expect(page).toHaveURL(new RegExp(`/w/${first}$`))

  await setLocale(page, 'zh-CN')
  await setTheme(page, 'dark')
  sidebar = await tree(page, isMobile, zh)
  await expect(sidebar.locator('.workspace-heading h2')).toHaveText(
    zh.workspaceGrid.ux02.workspaces,
  )
  await expect(
    sidebar.locator(`[data-nav-id="${first}"] .workspace-default`),
  ).toHaveText(zh.workspaceGrid.ux02.default)
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true)
  if (isMobile)
    await page.getByRole('button', { name: zh.nav.close, exact: true }).click()
  await setLocale(page, 'en')
  await setTheme(page, 'light')

  let rejectResolve = true
  await page.route(`**${preferencePath}/resolve`, async (route) => {
    if (rejectResolve) {
      rejectResolve = false
      await route.fulfill({
        status: 503,
        contentType: 'application/json',
        body: JSON.stringify({ error: { code: 'ERR_INTERNAL' } }),
      })
    } else await route.continue()
  })
  expectHTTPFailure(page, 'POST', `${preferencePath}/resolve`, 503)
  await page.goto('/home')
  await expect(page.locator('.home-page [role="alert"]')).toBeVisible()
  await expect(page).toHaveURL(/\/home$/)
  if (isMobile) {
    await tree(page, true)
    await expect(page.locator('.nav-drawer .nav-error')).toBeVisible()
    await closeDrawer(page, true)
  } else {
    await page.getByRole('button', { name: en.nav.collapseSidebar }).click()
    await expect(page.locator('.nav-error-floating')).toBeVisible()
  }
  await page.locator('.home-page [role="alert"] button').click()
  await expect(page).toHaveURL(new RegExp(`/w/${first}$`))
  await page.unroute(`**${preferencePath}/resolve`)
  if (!isMobile)
    await page.getByRole('button', { name: en.nav.expandSidebar }).click()

  const bookResponse = await page.request.post(
    `/api/v1/workspaces/${second}/books`,
    {
      headers: headers(),
      data: { name: 'Visible book' },
    },
  )
  expect(bookResponse.status()).toBe(201)
  const book = ((await bookResponse.json()) as { book: { id: string } }).book.id
  const siblingBookResponse = await page.request.post(
    `/api/v1/workspaces/${second}/books`,
    { headers: headers(), data: { name: 'Hidden sibling book' } },
  )
  expect(siblingBookResponse.status()).toBe(201)
  const siblingBook = (
    (await siblingBookResponse.json()) as { book: { id: string } }
  ).book.id
  const tableResponse = await page.request.post(
    `/api/v1/books/${book}/tables`,
    {
      headers: headers(),
      data: { name: 'Visible table' },
    },
  )
  expect(tableResponse.status()).toBe(201)
  const schema = (await tableResponse.json()) as TableSchema
  const table = schema.table.id
  const view = schema.views[0]!.id
  const nameField = schema.table.displayFieldId
  if (!nameField) throw new Error('Expected the default display field')
  const siblingTableResponse = await page.request.post(
    `/api/v1/books/${book}/tables`,
    {
      headers: headers(),
      data: { name: 'Hidden sibling table' },
    },
  )
  expect(siblingTableResponse.status()).toBe(201)
  const siblingTable = ((await siblingTableResponse.json()) as TableSchema)
    .table.id
  const rows = await page.request.post(`/api/v1/tables/${table}/rows`, {
    headers: headers(),
    data: { rows: [{ fields: { [nameField]: 'Deep-linked row' } }] },
  })
  expect(rows.status()).toBe(201)
  const row = ((await rows.json()) as { rows: DataRow[] }).rows[0]!.id
  const viewPath = `/t/${table}/v/${view}`
  const rowPath = `${viewPath}/r/${row}`
  await page.goto(rowPath)
  await expect(page).toHaveURL(new RegExp(`${rowPath}$`))
  await expect(page.locator('dialog.row-expand')).toBeVisible()
  await page
    .locator('dialog.row-expand')
    .getByRole('button', { name: en.rowExpand.close })
    .click()
  await expect(page).toHaveURL(new RegExp(`${viewPath}$`))
  sidebar = await tree(page, isMobile)
  const currentWorkspace = sidebar.locator(
    `[data-nav-id="${second}"] > .workspace-box`,
  )
  await expect(currentWorkspace).toHaveClass(/current/)
  await expect(currentWorkspace.locator('.workspace-link')).toHaveAttribute(
    'aria-current',
    'location',
  )
  const toggle = currentWorkspace.getByRole('button', {
    name: en.nav.toggle.replace('{name}', `UX02 second ${suffix}`),
  })
  await toggle.focus()
  await page.keyboard.press('Space')
  await expect(toggle).toHaveAttribute('aria-expanded', 'false')
  expect(new URL(page.url()).pathname).toBe(viewPath)
  await page.keyboard.press('Space')
  await expect(toggle).toHaveAttribute('aria-expanded', 'true')
  await sidebar.locator(`[data-nav-id="${first}"] .workspace-link`).click()
  await expect(page).toHaveURL(new RegExp(`/w/${first}$`))
  await page.goBack()
  await expect(page).toHaveURL(new RegExp(`${viewPath}$`))
  await page.goForward()
  await expect(page).toHaveURL(new RegExp(`/w/${first}$`))

  const username = `ux02-${suffix}`
  const userResponse = await page.request.post('/api/v1/admin/users', {
    headers: headers(),
    data: {
      username,
      displayName: 'Restricted navigation user',
      role: 'user',
      password: 'Temporary-password-123',
    },
  })
  expect(userResponse.status()).toBe(201)
  const user = ((await userResponse.json()) as AdminUserMutation).user
  const grant = await page.request.put('/api/v1/grants', {
    headers: headers(),
    data: {
      resourceType: 'table',
      resourceId: table,
      principalType: 'user',
      principalId: user.id,
      level: 'read',
    },
  })
  expect(grant.status()).toBe(201)
  const userContext = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL,
    viewport: isMobile
      ? { width: 390, height: 844 }
      : { width: 1440, height: 900 },
    isMobile,
    hasTouch: isMobile,
    locale: 'en-US',
  })
  const userPage = await userContext.newPage()
  const diagnostics = await observeBrowser(userPage)
  try {
    await paceLogin()
    const initialLogin = await userContext.request.post('/api/v1/auth/login', {
      headers: headers(),
      data: { login: username, password: 'Temporary-password-123' },
    })
    expect(initialLogin.status()).toBe(200)
    const password = await userContext.request.post('/api/v1/auth/password', {
      headers: headers(),
      data: { current: 'Temporary-password-123', new: 'Changed-password-123' },
    })
    expect(password.status()).toBe(200)
    await paceLogin()
    const finalLogin = await userContext.request.post('/api/v1/auth/login', {
      headers: headers(),
      data: { login: username, password: 'Changed-password-123' },
    })
    expect(finalLogin.status()).toBe(200)
    await userPage.goto(viewPath)
    await expect(userPage).toHaveURL(new RegExp(`${viewPath}$`))
    const userTree = await tree(userPage, isMobile)
    const context = userTree.locator(`[data-nav-id="${second}"]`)
    await expect(
      context.locator(':scope > .workspace-box .path-only'),
    ).toHaveText(en.nav.pathOnly)
    await expect(context.locator(':scope > .node-actions')).toHaveCount(0)
    await expect(
      context.locator(`[data-nav-id="${book}"] > .node-actions`),
    ).toHaveCount(0)
    await expect(context.locator(`[data-nav-id="${table}"]`)).toBeVisible()
    await expect(context.locator(`[data-nav-id="${siblingBook}"]`)).toHaveCount(
      0,
    )
    await expect(
      context.locator(`[data-nav-id="${siblingTable}"]`),
    ).toHaveCount(0)
    expect(
      (
        await userContext.request.put(preferencePath, {
          headers: headers(),
          data: { workspaceId: second },
        })
      ).status(),
    ).toBe(403)
    expectBrowserClean(userPage, diagnostics)
  } finally {
    await userContext.close()
  }
})
