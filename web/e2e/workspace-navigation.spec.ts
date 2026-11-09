import type { Page } from '@playwright/test'
import type {
  AdminUserMutation,
  DataRow,
  TableSchema,
  WorkspacePreference,
} from '../src/api/types'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }
import {
  expect,
  expectBrowserClean,
  observeBrowser,
  paceLogin,
  test,
} from './fixtures'
import { setLocale } from './app-shell'
import { closeMobileNavigation } from './sharing'

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
  await expect(page).toHaveURL(/\/(?:home|w\/[^/]+)$/)
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

async function closeRow(page: Page) {
  const dialog = page.locator('dialog.row-expand')
  const viewPath = new URL(page.url()).pathname.replace(/\/r\/[^/]+$/, '')
  await dialog.getByRole('button', { name: en.rowExpand.close }).click()
  await expect(dialog).not.toBeVisible()
  await expect(page).toHaveURL(viewPath)
}

test('DS-09: dashboard link, current workspace context, breadcrumbs and table-only ancestry', async ({
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
  const diagnostics = await observeBrowser(page)
  const suffix = `${Date.now()}-${testInfo.project.name.startsWith('mobile') ? 'm' : 'd'}`

  const siblingResponse = await page.request.post('/api/v1/workspaces', {
    headers: headers(),
    data: { name: `Hidden workspace ${suffix}` },
  })
  expect(siblingResponse.status()).toBe(201)
  const sibling = (
    (await siblingResponse.json()) as { workspace: { id: string } }
  ).workspace.id
  cleanup.workspaceIds.push(sibling)

  const workspaceResponse = await page.request.post('/api/v1/workspaces', {
    headers: headers(),
    data: { name: `Current workspace ${suffix}`, icon: 'star' },
  })
  expect(workspaceResponse.status()).toBe(201)
  const workspace = (
    (await workspaceResponse.json()) as {
      workspace: { id: string }
    }
  ).workspace.id
  cleanup.workspaceIds.push(workspace)

  const bookResponse = await page.request.post(
    `/api/v1/workspaces/${workspace}/books`,
    { headers: headers(), data: { name: 'Visible book' } },
  )
  expect(bookResponse.status()).toBe(201)
  const book = ((await bookResponse.json()) as { book: { id: string } }).book.id
  const siblingBookResponse = await page.request.post(
    `/api/v1/workspaces/${workspace}/books`,
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
    { headers: headers(), data: { name: 'Hidden sibling table' } },
  )
  expect(siblingTableResponse.status()).toBe(201)
  const siblingTable = ((await siblingTableResponse.json()) as TableSchema)
    .table.id
  const rowResponse = await page.request.post(`/api/v1/tables/${table}/rows`, {
    headers: headers(),
    data: { rows: [{ fields: { [nameField]: 'Deep-linked row' } }] },
  })
  expect(rowResponse.status()).toBe(201)
  const row = ((await rowResponse.json()) as { rows: DataRow[] }).rows[0]!.id
  const bookPath = `/w/${workspace}/b/${book}`
  const viewPath = `/t/${table}/v/${view}`
  const rowPath = `${viewPath}/r/${row}`

  await page.goto('/home')
  let sidebar = await tree(page, isMobile)
  await expect(
    sidebar.locator('.workspace-root li[data-kind="dashboard"] a'),
  ).toHaveAttribute('aria-current', 'page')
  await expect(sidebar.locator('.workspace-root > li')).toHaveCount(1)
  await expect(
    sidebar.locator('.workspace-heading, .workspace-toggle'),
  ).toHaveCount(0)
  await expect(page.locator('.breadcrumbs a')).toHaveCount(0)
  await expect(sidebar).not.toContainText(`Current workspace ${suffix}`)
  await expect(sidebar).not.toContainText(`Hidden workspace ${suffix}`)

  await page.goto(`/w/${workspace}`)
  sidebar = await tree(page, isMobile)
  const dashboardLink = sidebar.locator(
    '.workspace-root li[data-kind="dashboard"] .workspace-link',
  )
  const workspaceLink = sidebar.locator(
    `.workspace-root li[data-nav-id="${workspace}"] .workspace-link`,
  )
  await expect(dashboardLink).toHaveText(en.workspaceGrid.ux02.workspaces)
  await expect(dashboardLink).toHaveAttribute('href', '/home')
  await expect(dashboardLink).not.toHaveAttribute('aria-current', 'page')
  await expect(sidebar.locator('.workspace-root > li')).toHaveCount(2)
  await expect(workspaceLink).toContainText(`Current workspace ${suffix}`)
  await expect(workspaceLink.locator('.app-icon')).toHaveCount(0)
  await expect(workspaceLink).toHaveAttribute('aria-current', 'page')
  await expect(sidebar).not.toContainText(`Hidden workspace ${suffix}`)
  const workspaceActions = sidebar.locator(
    `[data-nav-id="${workspace}"] .workspace-nav-menu`,
  )
  await expect(workspaceActions).toHaveCount(1)
  await expect(workspaceActions.locator('summary')).toHaveAttribute(
    'aria-label',
    en.wmb.workspaceActions.replace('{name}', `Current workspace ${suffix}`),
  )

  await page.goto(bookPath)
  sidebar = await tree(page, isMobile)
  await expect(sidebar.locator('.workspace-root > li')).toHaveCount(2)
  await expect(
    sidebar.locator(
      `.workspace-root [data-nav-id="${workspace}"] .workspace-link`,
    ),
  ).toHaveAttribute('aria-current', 'location')
  await expect(sidebar.locator('.workbook-root')).toHaveAttribute(
    'data-context-book',
    book,
  )
  await expect(sidebar).not.toContainText(`Hidden workspace ${suffix}`)
  await expect(sidebar.locator('.workbook-root > li')).toHaveCount(1)
  await expect(sidebar.locator('.workbook-root')).not.toContainText(
    'Hidden sibling book',
  )
  await expect(page.locator('.breadcrumbs a').first()).toHaveText(
    `Current workspace ${suffix}`,
  )
  await expect(page.locator('.breadcrumbs a').first()).toHaveAttribute(
    'href',
    `/w/${workspace}`,
  )

  await page.goto(rowPath)
  await expect(page.locator('dialog.row-expand')).toBeVisible()
  await expect(page.locator('.breadcrumbs a').first()).toHaveText(
    `Current workspace ${suffix}`,
  )
  await expect(page.locator('.breadcrumbs a').first()).toHaveAttribute(
    'href',
    `/w/${workspace}`,
  )
  await expect(page.locator('.breadcrumb-workspace')).toHaveText(
    `Current workspace ${suffix}`,
  )
  await closeRow(page)
  sidebar = await tree(page, isMobile)
  await expect(sidebar.locator('.workbook-root')).toHaveAttribute(
    'data-context-book',
    book,
  )
  await expect(sidebar).not.toContainText(`Hidden workspace ${suffix}`)
  await expect(sidebar.locator('.workbook-root')).not.toContainText(
    'Hidden sibling book',
  )
  if (isMobile) await closeMobileNavigation(page, en)
  await page.locator('.breadcrumbs a').first().click()
  await expect(page).toHaveURL(new RegExp(`/w/${workspace}$`))
  if (isMobile) await expect(page.locator('.nav-drawer')).not.toBeVisible()

  await page.goto(rowPath)
  await expect(page.locator('dialog.row-expand')).toBeVisible()
  await closeRow(page)
  sidebar = await tree(page, isMobile)
  await sidebar
    .locator(`.workspace-root [data-nav-id="${workspace}"] .workspace-link`)
    .click()
  await expect(page).toHaveURL(new RegExp(`/w/${workspace}$`))
  if (isMobile) await expect(page.locator('.nav-drawer')).not.toBeVisible()
  sidebar = await tree(page, isMobile)
  await expect(sidebar.locator('.workbook-root')).toHaveCount(0)
  await expect(
    page.locator('main .workbook-card').filter({ hasText: 'Visible book' }),
  ).toBeVisible()
  await page.goto(rowPath)
  await expect(page.locator('dialog.row-expand')).toBeVisible()
  await closeRow(page)
  await page.locator('.breadcrumbs a').first().click()
  await expect(page).toHaveURL(new RegExp(`/w/${workspace}$`))
  await page.goto(rowPath)
  await expect(page.locator('dialog.row-expand')).toBeVisible()
  await page.reload()
  await expect(page).toHaveURL(new RegExp(`${rowPath}$`))
  await expect(page.locator('dialog.row-expand')).toBeVisible()
  await closeRow(page)
  sidebar = await tree(page, isMobile)
  await expect(sidebar.locator('.workspace-root > li')).toHaveCount(2)
  await expect(sidebar.locator('.workbook-root')).toHaveAttribute(
    'data-context-book',
    book,
  )

  await sidebar
    .locator('.workspace-root li[data-kind="dashboard"] .workspace-link')
    .click()
  await expect(page).toHaveURL(/\/home$/)
  if (isMobile) await expect(page.locator('.nav-drawer')).not.toBeVisible()
  else {
    await page.getByRole('button', { name: en.nav.collapseSidebar }).click()
    await expect(page.locator('.sidebar-desktop')).not.toBeVisible()
    await page.getByRole('button', { name: en.nav.expandSidebar }).click()
  }

  await setLocale(page, 'zh-CN')
  sidebar = await tree(page, isMobile, zh)
  await expect(
    sidebar.locator('.workspace-root li[data-kind="dashboard"] a'),
  ).toHaveText(zh.workspaceGrid.ux02.workspaces)
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true)
  if (isMobile) await page.getByRole('button', { name: zh.nav.close }).click()
  await setLocale(page, 'en')

  const username = `ds09-${suffix}`
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
  const userDiagnostics = await observeBrowser(userPage)
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
    await userPage.goto(rowPath)
    await expect(userPage).toHaveURL(new RegExp(`${rowPath}$`))
    await expect(userPage.locator('dialog.row-expand')).toBeVisible()
    await closeRow(userPage)
    const userTree = await tree(userPage, isMobile)
    await expect(userTree.locator('.workspace-root > li')).toHaveCount(2)
    await expect(userTree).not.toContainText(`Hidden workspace ${suffix}`)
    await expect(
      userTree.locator('.workspace-root [data-kind="workspace"]'),
    ).toHaveCount(1)
    const context = userTree.locator(`[data-nav-id="${workspace}"]`)
    await expect(
      context.locator(':scope > .workspace-box .path-only'),
    ).toHaveText(en.nav.pathOnly)
    await expect(context.locator('.workspace-link .app-icon')).toHaveCount(0)
    await expect(context.locator('.workspace-nav-menu')).toHaveCount(0)
    const userBookTree = userTree.locator('.workbook-root')
    await expect(userBookTree).toHaveAttribute('data-context-book', book)
    await expect(userBookTree.locator(`[data-nav-id="${book}"]`)).toBeVisible()
    await expect(
      userBookTree.locator(`[data-nav-id="${siblingBook}"]`),
    ).toHaveCount(0)
    await expect(
      userBookTree.locator(`[data-nav-id="${siblingTable}"]`),
    ).toHaveCount(0)
    await expect(userBookTree.locator(`[data-nav-id="${table}"]`)).toBeVisible()
    await expect(userPage.locator('.breadcrumbs a').first()).toHaveAttribute(
      'href',
      `/w/${workspace}`,
    )
    expect(
      (
        await userContext.request.put(preferencePath, {
          headers: headers(),
          data: { workspaceId: workspace },
        })
      ).status(),
    ).toBe(403)
    expectBrowserClean(userPage, userDiagnostics)
  } finally {
    await userContext.close()
  }
  expectBrowserClean(page, diagnostics)
})
