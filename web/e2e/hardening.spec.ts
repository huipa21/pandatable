import type { Page } from '@playwright/test'
import {
  exerciseOnce,
  readMemorySample,
  scrollToLoaded,
  trackPage,
  validateGridView,
  waitForSubscription,
} from '../scripts/load-test.mjs'
import type { AuthUserMutation, DataRow, TableSchema } from '../src/api/types'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }
import { expect, paceLogin, test } from './fixtures'

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

async function login(
  page: Page,
  username = 'first-admin',
  password = 'Setup-password-123',
) {
  await paceLogin(page)
  const response = await page.request.post('/api/v1/auth/login', {
    headers: headers(),
    data: { login: username, password },
  })
  expect(response.status()).toBe(200)
  return ((await response.json()) as AuthUserMutation).user.id
}

async function fixture(page: Page, suffix: string, count: number) {
  const workspaceName = `Hardening workspace ${suffix}`
  const bookName = `Hardening book ${suffix}`
  const tableName = `Hardening table ${suffix}`
  const workspace = await page.request.post('/api/v1/workspaces', {
    headers: headers(),
    data: { name: workspaceName },
  })
  expect(workspace.status()).toBe(201)
  const workspaceId = (await workspace.json()).workspace.id as string
  const book = await page.request.post(
    `/api/v1/workspaces/${workspaceId}/books`,
    { headers: headers(), data: { name: bookName } },
  )
  expect(book.status()).toBe(201)
  const bookId = (await book.json()).book.id as string
  const table = await page.request.post(`/api/v1/books/${bookId}/tables`, {
    headers: headers(),
    data: { name: tableName },
  })
  expect(table.status()).toBe(201)
  const tableId = (await table.json()).table.id as string
  const schemaResponse = await page.request.get(
    `/api/v1/tables/${tableId}/schema`,
  )
  expect(schemaResponse.status()).toBe(200)
  const schema = (await schemaResponse.json()) as TableSchema
  const fieldId = schema.table.displayFieldId
  const viewId = schema.views[0]?.id
  if (!fieldId || !viewId) throw new Error('Fixture schema is incomplete')
  validateGridView(schema.views[0])
  const rows: DataRow[] = []
  for (let offset = 0; offset < count; offset += 200) {
    const response = await page.request.post(`/api/v1/tables/${tableId}/rows`, {
      headers: headers(),
      data: {
        rows: Array.from(
          { length: Math.min(200, count - offset) },
          (_, index) => ({
            fields: { [fieldId]: `Fixture ${offset + index}` },
          }),
        ),
      },
    })
    expect(response.status()).toBe(201)
    rows.push(...((await response.json()).rows as DataRow[]))
  }
  return {
    workspaceName,
    bookName,
    tableName,
    workspaceId,
    bookId,
    tableId,
    viewId,
    fieldId,
    rows,
  }
}

async function noPageOverflow(page: Page, path: string) {
  await expect
    .poll(
      () =>
        page.evaluate(
          () => document.documentElement.scrollWidth <= window.innerWidth,
        ),
      { message: `No horizontal page overflow on ${path}` },
    )
    .toBe(true)
}

async function reachableControls(page: Page, path: string) {
  const viewport = page.viewportSize()
  if (!viewport) throw new Error('Responsive test requires a fixed viewport')
  const controls = page.locator(
    ':is(header button, header a[href], main button, main a[href], ' +
      'main input:not([type="hidden"]), main select, main textarea):visible',
  )
  const visible = await controls.evaluateAll((elements) =>
    elements.map((element, index) => {
      element.scrollIntoView({ block: 'center', inline: 'nearest' })
      const scrolled = element.getBoundingClientRect()
      return {
        index,
        x: scrolled.x,
        width: scrolled.width,
        enabled:
          !element.matches(':disabled') &&
          !element.closest('[aria-disabled="true"]'),
      }
    }),
  )
  let enabled = 0
  for (const control of visible) {
    expect(
      control.x,
      `${path}: control ${control.index + 1} left edge`,
    ).toBeGreaterThanOrEqual(0)
    expect(
      control.x + control.width,
      `${path}: control ${control.index + 1} right edge`,
    ).toBeLessThanOrEqual(viewport.width)
    if (control.enabled) {
      enabled++
      await controls.nth(control.index).click({ trial: true, timeout: 5_000 })
    }
  }
  expect(visible.length, `${path}: visible controls`).toBeGreaterThan(0)
  expect(enabled, `${path}: enabled controls`).toBeGreaterThan(0)
}

test('AT-806 main pages remain usable at 390px in both languages', async ({
  page,
}, testInfo) => {
  test.setTimeout(150_000)
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/login')
  if ((await page.locator('html').getAttribute('lang')) === 'zh-CN')
    await page
      .getByRole('button', { name: zh.common.languages.en, exact: true })
      .click()
  await expect(
    page.getByRole('heading', { name: en.login.title }),
  ).toBeVisible()
  await expect(page.getByLabel(en.auth.login, { exact: true })).toBeVisible()
  await page.waitForLoadState('networkidle')
  await noPageOverflow(page, '/login')
  await reachableControls(page, '/login')

  await login(page)
  const data = await fixture(page, `${testInfo.project.name}-${Date.now()}`, 1)
  const routes = [
    { path: '/home', en: en.auth.homeTitle, zh: zh.auth.homeTitle },
    { path: '/setup', en: en.setup.title, zh: zh.setup.title },
    {
      path: `/w/${data.workspaceId}`,
      en: data.workspaceName,
      zh: data.workspaceName,
    },
    {
      path: `/w/${data.workspaceId}/b/${data.bookId}`,
      en: data.bookName,
      zh: data.bookName,
    },
    {
      path: `/t/${data.tableId}/v/${data.viewId}`,
      en: data.tableName,
      zh: data.tableName,
    },
    { path: '/profile', en: en.profile.title, zh: zh.profile.title },
    {
      path: '/change-password',
      en: en.auth.changePassword,
      zh: zh.auth.changePassword,
    },
    { path: '/trash', en: en.trash.title, zh: zh.trash.title },
    {
      path: '/admin/dashboard',
      en: en.dashboard.title,
      zh: zh.dashboard.title,
    },
    { path: '/admin/users', en: en.admin.users, zh: zh.admin.users },
    { path: '/admin/groups', en: en.groups.title, zh: zh.groups.title },
    {
      path: '/admin/settings',
      en: en.settings.title,
      zh: zh.settings.title,
    },
    {
      path: '/admin/workspaces',
      en: en.workspaces.title,
      zh: zh.workspaces.title,
    },
    { path: '/admin/backups', en: en.backups.title, zh: zh.backups.title },
    {
      path: '/connection-error',
      en: en.auth.connectionError,
      zh: zh.auth.connectionError,
    },
    {
      path: '/not-yet-implemented',
      en: en.nav.notFoundTitle,
      zh: zh.nav.notFoundTitle,
    },
  ]
  try {
    for (const locale of ['en', 'zh'] as const) {
      if (locale === 'zh')
        await page
          .getByRole('button', { name: en.common.languages.zhCN, exact: true })
          .click()
      for (const route of routes) {
        await page.goto(route.path)
        await expect(page.locator('main h1')).toHaveText(route[locale])
        await page.waitForLoadState('networkidle')
        await noPageOverflow(page, route.path)
        if (route.path.startsWith('/t/')) {
          await expect(
            page.getByRole('region', {
              name: locale === 'en' ? en.mobileRows.title : zh.mobileRows.title,
              exact: true,
            }),
          ).toBeVisible()
        }
        await reachableControls(page, route.path)
        await noPageOverflow(page, route.path)
      }
    }
  } finally {
    if (
      !page.isClosed() &&
      (await page.locator('html').getAttribute('lang')) === 'zh-CN'
    ) {
      await page
        .getByRole('button', { name: zh.common.languages.en, exact: true })
        .click()
      await expect(page.locator('html')).toHaveAttribute('lang', 'en')
    }
  }
})

test('load harness waits for the grid source before scrolling', async ({
  page,
}) => {
  await page.setContent(
    '<revo-grid style="display:block;width:100%;height:300px">' +
      '<revogr-data col-type="colPinStart"><button></button></revogr-data>' +
      '</revo-grid>',
  )
  const grid = page.locator('revo-grid')
  await grid.evaluate(
    (element, rowLabel) => {
      const source = () =>
        element.hasAttribute('data-test-source-ready')
          ? Array.from({ length: 301 }, (_, index) =>
              index === 300 ? { __rowId: 'test-row' } : {},
            )
          : []
      const button = element.querySelector('button')
      if (!button) throw new Error('Missing test row button')
      button.setAttribute('aria-label', rowLabel)
      Object.assign(element, {
        async getSource() {
          element.setAttribute('data-test-source-read', 'true')
          return source()
        },
        async scrollToRow(target: number) {
          element.setAttribute('data-test-source-read', 'true')
          const rowCount = source().length
          if (rowCount <= target)
            throw new Error('Load harness scrolled before grid initialization')
          element.setAttribute('data-test-scroll-count', String(rowCount))
        },
      })
    },
    en.rowExpand.open.replace('{id}', 'test-row'),
  )
  const result = scrollToLoaded(page, 300, 'test-row').then(
    () => null,
    (error: unknown) => error,
  )
  try {
    await expect(grid).toHaveAttribute('data-test-source-read', 'true')
  } finally {
    await grid.evaluate((element) =>
      element.setAttribute('data-test-source-ready', 'true'),
    )
  }
  expect(await result).toBeNull()
  await expect(grid).toHaveAttribute('data-test-scroll-count', '301')
})

test('load harness scrolls, edits, receives peer events, and reads both metrics', async ({
  page,
  browser,
  isMobile,
}, testInfo) => {
  test.skip(isMobile, 'Load harness drives the desktop grid')
  test.setTimeout(240_000)
  await page.setViewportSize({ width: 1440, height: 900 })
  const adminId = await login(page)
  const data = await fixture(page, `load-smoke-${testInfo.project.name}`, 600)
  const username = `load-smoke-${Date.now()}`
  const created = await page.request.post('/api/v1/admin/users', {
    headers: headers(),
    data: {
      username,
      displayName: 'Load smoke',
      role: 'user',
      password: 'Temporary-password-123',
    },
  })
  expect(created.status()).toBe(201)
  const peerId = ((await created.json()) as AuthUserMutation).user.id
  const grant = await page.request.put('/api/v1/grants', {
    headers: headers(),
    data: {
      resourceType: 'table',
      resourceId: data.tableId,
      principalType: 'user',
      principalId: peerId,
      level: 'write',
    },
  })
  expect(grant.status()).toBe(201)
  const peerContext = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL,
    viewport: { width: 1440, height: 900 },
    locale: 'en-US',
  })
  const admin = {
    userId: adminId,
    subscribed: false,
    stopping: false,
    failure: '',
    received: 0,
  }
  const peer = {
    userId: peerId,
    subscribed: false,
    stopping: false,
    failure: '',
    received: 0,
  }
  try {
    const peerPage = await peerContext.newPage()
    await login(peerPage, username, 'Temporary-password-123')
    const changed = await peerPage.request.post('/api/v1/auth/password', {
      headers: headers(),
      data: {
        current: 'Temporary-password-123',
        new: 'Changed-password-123',
      },
    })
    expect(changed.status()).toBe(200)
    expect(await login(peerPage, username, 'Changed-password-123')).toBe(peerId)
    trackPage(page, admin, data.tableId)
    trackPage(peerPage, peer, data.tableId)
    const path = `/t/${data.tableId}/v/${data.viewId}`
    await Promise.all([page.goto(path), peerPage.goto(path)])
    await Promise.all([waitForSubscription(admin), waitForSubscription(peer)])
    for (const [rowIndex, scrollIndex, value] of [
      [0, 300, 'load-smoke'],
      [9, 500, 'load-smoke-2'],
      [0, 400, 'load-smoke-3'],
    ] as const) {
      const row = data.rows[rowIndex]
      if (!row) throw new Error('Fixture did not create the load-test row')
      await exerciseOnce(rowIndex === 9 ? peerPage : page, {
        tableId: data.tableId,
        fieldId: data.fieldId,
        rowIndex,
        rowId: row.id,
        scrollIndex,
        value,
      })
      const saved = await page.request.get(
        `/api/v1/tables/${data.tableId}/rows/${row.id}`,
      )
      expect(saved.status()).toBe(200)
      expect(((await saved.json()) as DataRow).fields[data.fieldId]).toBe(value)
    }
    await expect.poll(() => admin.received).toBeGreaterThan(0)
    await expect.poll(() => peer.received).toBeGreaterThan(0)
    expect(admin.failure).toBe('')
    expect(peer.failure).toBe('')
    const metrics = await page.request.get('/api/v1/admin/system')
    expect(metrics.status()).toBe(200)
    const sample = readMemorySample(await metrics.json(), 0)
    expect(sample.appRssBytes).toBeGreaterThan(0)
    expect(sample.hostUsedBytes).toBeGreaterThan(0)
  } finally {
    admin.stopping = true
    peer.stopping = true
    await peerContext.close()
  }
})
