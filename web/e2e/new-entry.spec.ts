import type { Locator, Page } from '@playwright/test'
import {
  expect,
  expectBrowserClean,
  expectHTTPFailure,
  observeBrowser,
  paceLogin,
  test,
} from './fixtures'
import { setLocale, setTheme } from './app-shell'
import type {
  DataRow,
  FieldMutation,
  FilterNode,
  RowMutation,
  RowQueryResult,
  TableField,
  TableSchema,
  ViewColumns,
  ViewMutation,
} from '../src/api/types'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

async function login(page: Page) {
  await paceLogin(page)
  const response = await page.request.post('/api/v1/auth/login', {
    headers: headers(),
    data: { login: 'first-admin', password: 'Setup-password-123' },
  })
  expect(response.status()).toBe(200)
}

async function create<T>(page: Page, path: string, data: unknown): Promise<T> {
  const response = await page.request.post(path, { headers: headers(), data })
  expect(response.status()).toBe(201)
  return (await response.json()) as T
}

async function createTable(page: Page, label: string) {
  const { workspace } = await create<{ workspace: { id: string } }>(
    page,
    '/api/v1/workspaces',
    { name: `New entry ${label} ${Date.now()}` },
  )
  const { book } = await create<{ book: { id: string } }>(
    page,
    `/api/v1/workspaces/${workspace.id}/books`,
    { name: 'New entry book' },
  )
  const schema = await create<TableSchema>(
    page,
    `/api/v1/books/${book.id}/tables`,
    { name: 'New entry table' },
  )
  const view = schema.views[0]
  const name = schema.fields.find(
    (field) => field.id === schema.table.displayFieldId,
  )
  if (!view || !name) throw new Error('Missing grid view or display field')
  return {
    tableId: schema.table.id,
    viewId: view.id,
    name,
    path: `/t/${schema.table.id}/v/${view.id}`,
  }
}

async function createField(
  page: Page,
  tableId: string,
  data: Record<string, unknown>,
) {
  const result = await create<FieldMutation>(
    page,
    `/api/v1/tables/${tableId}/fields`,
    data,
  )
  if (!result.field) throw new Error('Field creation returned no field')
  return result.field
}

async function schemaOf(page: Page, tableId: string) {
  const response = await page.request.get(`/api/v1/tables/${tableId}/schema`)
  expect(response.status()).toBe(200)
  return (await response.json()) as TableSchema
}

async function query(page: Page, viewId: string, filter?: FilterNode) {
  const response = await page.request.post(`/api/v1/views/${viewId}/query`, {
    headers: headers(),
    data: {
      offset: 0,
      limit: 100,
      includeCount: true,
      ...(filter ? { filter } : {}),
    },
  })
  expect(response.status()).toBe(200)
  return (await response.json()) as RowQueryResult
}

async function insertRows(
  page: Page,
  tableId: string,
  rows: { fields: Record<string, string | number | null> }[],
) {
  return create<RowMutation>(page, `/api/v1/tables/${tableId}/rows`, { rows })
}

async function configureView(
  page: Page,
  tableId: string,
  viewId: string,
  changes: Partial<ViewColumns>,
) {
  const schema = await schemaOf(page, tableId)
  const view = schema.views.find((item) => item.id === viewId)
  if (!view) throw new Error('Grid view is missing')
  const response = await page.request.patch(`/api/v1/views/${viewId}`, {
    headers: { ...headers(), 'If-Match': String(view.version) },
    data: { config: { ...view.config, ...changes } },
  })
  expect(response.status()).toBe(200)
}

async function openGrid(page: Page, path: string) {
  await page.goto(path)
  if ((await page.locator('html').getAttribute('lang')) === 'zh-CN')
    await setLocale(page, 'en')
  await expect(page.locator('revo-grid')).toBeVisible()
}

function fieldSection(dialog: Locator, field: TableField) {
  return dialog.locator(`[data-field-id="${field.id}"]`)
}

async function editField(dialog: Locator, field: TableField) {
  const section = fieldSection(dialog, field)
  await expect(section.locator('.field-editor')).toBeVisible()
  return section
}

async function applyText(dialog: Locator, field: TableField, value: string) {
  const section = await editField(dialog, field)
  await section
    .locator('.field-editor input, .field-editor textarea')
    .fill(value)
}

test('desktop New Entry and the grid plus use distinct creation paths', async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, 'The mobile add control already opens the creation form.')
  test.setTimeout(150_000)
  await page.setViewportSize({ width: 1600, height: 900 })
  await login(page)
  const table = await createTable(page, 'values')
  const code = await createField(page, table.tableId, {
    name: 'Required code',
    uiType: 'text',
    isRequired: true,
    isUnique: true,
    default: 'seed-code',
  })
  const check = await createField(page, table.tableId, {
    name: 'Check',
    uiType: 'checkbox',
  })
  const choices = {
    choices: [
      { id: 'a', label: 'Alpha', color: 'red' },
      { id: 'b', label: 'Beta', color: 'blue' },
    ],
  }
  const stage = await createField(page, table.tableId, {
    name: 'Stage',
    uiType: 'single_select',
    options: choices,
  })
  const tags = await createField(page, table.tableId, {
    name: 'Tags',
    uiType: 'multi_select',
    options: choices,
  })
  const labels = await createField(page, table.tableId, {
    name: 'Labels',
    uiType: 'text_array',
  })
  const defaulted = await createField(page, table.tableId, {
    name: 'Defaulted',
    uiType: 'integer',
    default: 7,
  })
  const rowsPath = `/api/v1/tables/${table.tableId}/rows`
  const posts: Record<string, unknown>[] = []
  page.on('request', (request) => {
    if (
      request.method() === 'POST' &&
      new URL(request.url()).pathname === rowsPath
    ) {
      const body = request.postDataJSON() as {
        rows: { fields: Record<string, unknown> }[]
      }
      posts.push(body.rows[0]!.fields)
    }
  })
  await openGrid(page, `/t/${table.tableId}?draft=keep`)
  const toolbar = page.locator('.grid-toolbar')
  const action = toolbar.getByRole('button', {
    name: en.workspaceGrid.ux10.newEntry,
    exact: true,
  })
  await expect(action).toBeVisible()
  await expect(toolbar.locator('.grid-toolbar-end > :last-child')).toHaveClass(
    /new-entry/,
  )
  const actionBox = await action.boundingBox()
  const precedingBox = await toolbar
    .locator('.table-toolbar-actions')
    .boundingBox()
  if (!actionBox || !precedingBox) throw new Error('Toolbar bounds unavailable')
  expect(actionBox.x).toBeGreaterThanOrEqual(
    precedingBox.x + precedingBox.width - 1,
  )

  await page.getByRole('button', { name: en.grid.addRow, exact: true }).click()
  await expect.poll(async () => (await query(page, table.viewId)).total).toBe(1)
  await expect(page.locator('dialog.row-expand[open]')).toHaveCount(0)
  expect(posts).toEqual([{}])

  const originalURL = page.url()
  await action.focus()
  await action.press('Enter')
  const dialog = page.locator('dialog.row-expand')
  await expect(dialog).toBeVisible()
  await expect(dialog.locator('header h2')).toHaveText(en.mobileRows.newRow)
  await expect(
    dialog.getByRole('button', { name: en.rowExpand.previous }),
  ).toHaveCount(0)
  await expect(
    dialog.getByRole('button', { name: en.rowExpand.next }),
  ).toHaveCount(0)
  await expect(dialog.locator('.row-field-value[role="button"]')).toHaveCount(0)
  const nameSection = fieldSection(dialog, table.name)
  const label = nameSection.locator('h3')
  const input = nameSection.locator('.field-editor input')
  const [desktopSection, desktopLabel] = await Promise.all([
    nameSection.boundingBox(),
    label.boundingBox(),
  ])
  expect(desktopSection).not.toBeNull()
  expect(desktopLabel).not.toBeNull()
  expect(desktopLabel!.width).toBeGreaterThanOrEqual(128)
  expect(desktopLabel!.width / (desktopSection!.width - 16)).toBeCloseTo(0.3, 1)
  await page.keyboard.press('Escape')
  await expect(dialog).not.toBeVisible()
  await page.setViewportSize({ width: 480, height: 844 })
  await page.getByRole('button', { name: en.grid.addRow, exact: true }).click()
  await expect(dialog).toBeVisible()
  const [wideLabel, wideInput] = await Promise.all([
    label.boundingBox(),
    input.boundingBox(),
  ])
  expect(wideLabel).not.toBeNull()
  expect(wideInput).not.toBeNull()
  expect(wideInput!.x).toBeGreaterThanOrEqual(wideLabel!.x + wideLabel!.width)
  await page.setViewportSize({ width: 479, height: 844 })
  const [narrowLabel, narrowInput] = await Promise.all([
    label.boundingBox(),
    input.boundingBox(),
  ])
  expect(narrowLabel).not.toBeNull()
  expect(narrowInput).not.toBeNull()
  expect(narrowInput!.y).toBeGreaterThanOrEqual(
    narrowLabel!.y + narrowLabel!.height,
  )
  expect(Math.abs(narrowInput!.x - narrowLabel!.x)).toBeLessThanOrEqual(1)
  await page.keyboard.press('Escape')
  await expect(dialog).not.toBeVisible()
  await page.setViewportSize({ width: 1600, height: 900 })
  await expect(action).toBeVisible()
  await action.focus()
  await action.press('Enter')
  await expect(dialog).toBeVisible()
  const search = dialog.getByRole('searchbox', {
    name: en.rowExpand.search,
    exact: true,
  })
  await search.fill('seed-code')
  await expect(dialog.locator('.row-field:visible')).toHaveCount(0)
  await expect(dialog.getByText(en.rowExpand.noFields)).toBeVisible()
  await search.fill('req')
  await expect(dialog.locator('.row-field:visible')).toHaveCount(1)
  await search.fill('')
  await expect(search).toBeFocused()
  expect(posts).toHaveLength(1)
  await applyText(dialog, table.name, 'Discarded draft')
  await page.keyboard.press('Escape')
  await expect(dialog).not.toBeVisible()
  await expect(page).toHaveURL(originalURL)
  await expect(action).toBeFocused()
  expect(posts).toHaveLength(1)
  expect((await query(page, table.viewId)).total).toBe(1)

  await action.click()
  await expect(dialog).toBeVisible()
  await expect(fieldSection(dialog, table.name)).not.toContainText(
    'Discarded draft',
  )
  await applyText(dialog, code, 'temporary')
  await applyText(dialog, code, '')
  expectHTTPFailure(page, 'POST', rowsPath, 400)
  const submit = dialog.getByRole('button', {
    name: en.gridPolish.gp05.save,
    exact: true,
  })
  await submit.click()
  await expect(dialog.getByRole('alert')).toHaveText(
    en.errors.ERR_REQUIRED_FIELD,
  )
  expect((await query(page, table.viewId)).total).toBe(1)
  await applyText(dialog, code, 'seed-code')
  expectHTTPFailure(page, 'POST', rowsPath, 409)
  await submit.click()
  await expect(dialog.getByRole('alert')).toHaveText(
    en.errors.ERR_UNIQUE_CONFLICT,
  )
  await applyText(dialog, code, 'unique-code')
  await applyText(dialog, table.name, 'Created from form')
  const checkSection = await editField(dialog, check)
  await checkSection.locator('input').click()
  const stageSection = await editField(dialog, stage)
  await stageSection.locator('select').selectOption('Beta')
  const tagsSection = await editField(dialog, tags)
  await tagsSection.locator('select').selectOption(['Beta', 'Alpha'])
  await applyText(dialog, labels, 'alpha, beta')
  expect(posts).toHaveLength(3)

  await page.evaluate((path) => {
    const original = window.fetch
    window.fetch = (...args: Parameters<typeof fetch>) => {
      const [input, init] = args
      const url =
        typeof input === 'string'
          ? input
          : input instanceof URL
            ? input.href
            : input.url
      if (
        new URL(url, location.href).pathname === path &&
        init?.method === 'POST'
      ) {
        window.fetch = original
        return Promise.reject(new TypeError('Simulated network failure'))
      }
      return original(...args)
    }
  }, rowsPath)
  await submit.click()
  await expect(dialog.getByRole('alert')).toHaveText(en.errors.ERR_INTERNAL)
  await expect(fieldSection(dialog, code).locator('input')).toBeEnabled()
  expect(posts).toHaveLength(3)

  let release!: () => void
  const hold = new Promise<void>((resolve) => {
    release = resolve
  })
  const routePattern = `**${rowsPath}`
  await page.route(routePattern, async (route) => {
    if (route.request().method() === 'POST') await hold
    await route.continue()
  })
  await submit.evaluate((button: HTMLButtonElement) => {
    button.click()
    button.click()
  })
  await expect.poll(() => posts.length).toBe(4)
  await expect(submit).toBeDisabled()
  release()
  await expect(page).toHaveURL(/\/r\/[1-9]\d*$/)
  await page.unroute(routePattern)
  const rowId = new URL(page.url()).pathname.split('/').at(-1)!
  const rowPath = page.url()
  expect(posts).toHaveLength(4)
  expect(posts[3]).toEqual({
    [table.name.id]: 'Created from form',
    [code.id]: 'unique-code',
    [check.id]: true,
    [stage.id]: 'Beta',
    [tags.id]: ['Alpha', 'Beta'],
    [labels.id]: ['alpha', 'beta'],
  })
  expect(posts[3]).not.toHaveProperty(defaulted.id)
  const response = await page.request.get(`${rowsPath}/${rowId}`)
  expect(response.status()).toBe(200)
  const saved = (await response.json()) as DataRow
  expect(saved.fields).toMatchObject({
    ...posts[3],
    [defaulted.id]: 7,
  })
  await dialog.getByRole('button', { name: en.rowExpand.close }).click()
  await expect(dialog).not.toBeVisible()
  await expect(action).toBeFocused()
  await page.goto(rowPath)
  await page.reload()
  await expect(fieldSection(dialog, table.name)).toContainText(
    'Created from form',
  )
  await expect(fieldSection(dialog, defaulted)).toContainText('7')
  await dialog.getByRole('button', { name: en.rowExpand.close }).click()

  await setTheme(page, 'dark')
  await setLocale(page, 'zh-CN')
  const translated = toolbar.getByRole('button', {
    name: zh.workspaceGrid.ux10.newEntry,
    exact: true,
  })
  await translated.click()
  await expect(dialog).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(translated).toBeFocused()
  await setLocale(page, 'en')
  await setTheme(page, 'light')
})

test('creation refreshes filtered, sorted and grouped views without inventing visible rows', async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, 'Grouped desktop grid behavior is tested here.')
  test.setTimeout(120_000)
  await page.setViewportSize({ width: 1440, height: 900 })
  await login(page)
  const table = await createTable(page, 'query')
  const status = await createField(page, table.tableId, {
    name: 'Status',
    uiType: 'text',
  })
  const amount = await createField(page, table.tableId, {
    name: 'Amount',
    uiType: 'integer',
  })
  const allView = await create<ViewMutation>(
    page,
    `/api/v1/tables/${table.tableId}/views`,
    { name: 'All rows', type: 'grid', scope: 'shared' },
  )
  await insertRows(page, table.tableId, [
    { fields: { [table.name.id]: 'A', [status.id]: 'Open', [amount.id]: 3 } },
    { fields: { [table.name.id]: 'B', [status.id]: 'Open', [amount.id]: 4 } },
    { fields: { [table.name.id]: 'C', [status.id]: 'Closed', [amount.id]: 5 } },
  ])
  const filter: FilterNode = { field: status.id, cmp: 'is', value: 'Open' }
  await configureView(page, table.tableId, table.viewId, {
    filter,
    sort: [{ field: table.name.id, dir: 'desc' }],
    groupBy: [status.id],
    aggregates: [{ fieldId: amount.id, aggregate: 'sum' }],
    rowHeight: 'tall',
  })
  const before = (await schemaOf(page, table.tableId)).views.find(
    (view) => view.id === table.viewId,
  )!
  await openGrid(page, table.path)
  const grid = page.locator('revo-grid')
  const action = page.getByRole('button', {
    name: en.workspaceGrid.ux10.newEntry,
    exact: true,
  })
  const dialog = page.locator('dialog.row-expand')
  const amountSummary = page.locator(
    `.grid-summary-cell[data-field-id="${amount.id}"] output`,
  )
  await expect(grid).toHaveAttribute('aria-rowcount', '1')
  await expect(amountSummary).toContainText('7')

  await action.click()
  await applyText(dialog, table.name, 'Hidden')
  await applyText(dialog, status, 'Closed')
  await applyText(dialog, amount, '9')
  await dialog.getByRole('button', { name: en.gridPolish.gp05.save }).click()
  await expect(page).toHaveURL(/\/r\/[1-9]\d*$/)
  await dialog.getByRole('button', { name: en.rowExpand.close }).click()
  await expect
    .poll(async () => (await query(page, allView.view.id)).total)
    .toBe(4)
  expect((await query(page, table.viewId, filter)).total).toBe(2)
  await expect(grid).toHaveAttribute('aria-rowcount', '1')
  await expect(grid.getByText('Hidden', { exact: true })).toHaveCount(0)
  await expect(amountSummary).toContainText('7')

  await action.click()
  await applyText(dialog, table.name, 'Z')
  await applyText(dialog, status, 'Open')
  await applyText(dialog, amount, '9')
  await dialog.getByRole('button', { name: en.gridPolish.gp05.save }).click()
  await expect(page).toHaveURL(/\/r\/[1-9]\d*$/)
  await dialog.getByRole('button', { name: en.rowExpand.close }).click()
  await expect
    .poll(async () =>
      (await query(page, table.viewId, filter)).rows.map(
        (row) => row.fields[table.name.id],
      ),
    )
    .toEqual(['Z', 'B', 'A'])
  await expect(amountSummary).toContainText('16')
  await page
    .getByRole('button', { name: 'Expand Status: Open', exact: false })
    .click()
  await expect(grid).toHaveAttribute('aria-rowcount', '4')
  await expect(grid.getByText('Z', { exact: true })).toBeVisible()
  await expect(grid.getByText('Hidden', { exact: true })).toHaveCount(0)
  const after = (await schemaOf(page, table.tableId)).views.find(
    (view) => view.id === table.viewId,
  )!
  expect(after.config).toEqual(before.config)
  expect(after.version).toBe(before.version)
})

test('readers cannot open New Entry, while writers can create through it', async ({
  page,
  browser,
  isMobile,
}) => {
  test.skip(isMobile, 'Desktop header permissions are tested here.')
  test.setTimeout(150_000)
  await login(page)
  const table = await createTable(page, 'permissions')
  const { user } = await create<{ user: { id: string; username: string } }>(
    page,
    '/api/v1/admin/users',
    {
      username: `entry-reader-${Date.now()}`,
      displayName: 'Entry reader',
      password: 'Temporary-password-123',
      role: 'user',
    },
  )
  const grant = async (level: 'read' | 'write') => {
    const response = await page.request.put('/api/v1/grants', {
      headers: headers(),
      data: {
        resourceType: 'table',
        resourceId: table.tableId,
        principalType: 'user',
        principalId: user.id,
        level,
      },
    })
    expect(response.ok()).toBe(true)
  }
  await grant('read')
  const context = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL,
    locale: 'en-US',
    viewport: { width: 1440, height: 900 },
  })
  try {
    await paceLogin(page)
    const first = await context.request.post('/api/v1/auth/login', {
      headers: headers(),
      data: { login: user.username, password: 'Temporary-password-123' },
    })
    expect(first.status()).toBe(200)
    const password = await context.request.post('/api/v1/auth/password', {
      headers: headers(),
      data: {
        current: 'Temporary-password-123',
        new: 'Collaborator-password-123',
      },
    })
    expect(password.status()).toBe(200)
    await paceLogin(page)
    const relogin = await context.request.post('/api/v1/auth/login', {
      headers: headers(),
      data: { login: user.username, password: 'Collaborator-password-123' },
    })
    expect(relogin.status()).toBe(200)
    const reader = await context.newPage()
    const diagnostics = await observeBrowser(reader)
    await openGrid(reader, table.path)
    await expect(reader.locator('.new-entry')).toHaveCount(0)
    await expect(reader.locator('.grid-add-row')).toHaveCount(0)
    const denied = await context.request.post(
      `/api/v1/tables/${table.tableId}/rows`,
      { headers: headers(), data: { rows: [{ fields: {} }] } },
    )
    expect(denied.status()).toBe(403)

    await grant('write')
    await reader.reload()
    const action = reader.getByRole('button', {
      name: en.workspaceGrid.ux10.newEntry,
      exact: true,
    })
    await expect(action).toBeVisible()
    await action.click()
    const dialog = reader.locator('dialog.row-expand')
    await applyText(dialog, table.name, 'Writer entry')
    await dialog.getByRole('button', { name: en.gridPolish.gp05.save }).click()
    await expect(reader).toHaveURL(/\/r\/[1-9]\d*$/)
    await expect(fieldSection(dialog, table.name)).toContainText('Writer entry')
    await dialog.getByRole('button', { name: en.rowExpand.close }).click()
    await expect(action).toBeFocused()
    expectBrowserClean(reader, diagnostics)
  } finally {
    await context.close()
  }
})

test('390px mobile keeps one add action and its existing creation form', async ({
  page,
  isMobile,
}) => {
  test.skip(!isMobile, 'This checks the mobile card-list action.')
  test.setTimeout(120_000)
  await page.setViewportSize({ width: 390, height: 844 })
  await login(page)
  const table = await createTable(page, 'mobile')
  const rowsPath = `/api/v1/tables/${table.tableId}/rows`
  let posts = 0
  page.on('request', (request) => {
    if (
      request.method() === 'POST' &&
      new URL(request.url()).pathname === rowsPath
    )
      posts++
  })
  await page.goto(table.path)
  if ((await page.locator('html').getAttribute('lang')) === 'zh-CN')
    await setLocale(page, 'en')
  await expect(page.locator('.mobile-rows')).toBeVisible()
  await expect(page.locator('revo-grid')).toHaveCount(0)
  await expect(page.locator('.new-entry')).toHaveCount(0)
  const add = page.getByRole('button', { name: en.grid.addRow, exact: true })
  await expect(add).toHaveCount(1)
  await add.click()
  const dialog = page.locator('dialog.row-expand')
  await expect(dialog).toBeVisible()
  expect(posts).toBe(0)
  await applyText(dialog, table.name, 'Discard mobile')
  await page.keyboard.press('Escape')
  await expect(add).toBeFocused()
  expect(posts).toBe(0)
  await setTheme(page, 'dark')
  await setLocale(page, 'zh-CN')
  const translatedAdd = page.getByRole('button', {
    name: zh.grid.addRow,
    exact: true,
  })
  await expect(translatedAdd).toHaveCount(1)
  await translatedAdd.click()
  await expect(dialog).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(translatedAdd).toBeFocused()
  await setLocale(page, 'en')
  await setTheme(page, 'light')
  await add.click()
  await expect(fieldSection(dialog, table.name)).not.toContainText(
    'Discard mobile',
  )
  await applyText(dialog, table.name, 'Mobile entry')
  await dialog.getByRole('button', { name: en.gridPolish.gp05.save }).click()
  await expect(page).toHaveURL(/\/r\/[1-9]\d*$/)
  expect(posts).toBe(1)
  await expect(fieldSection(dialog, table.name)).toContainText('Mobile entry')
  await dialog.getByRole('button', { name: en.rowExpand.close }).click()
  await expect(page.locator('.mobile-card')).toContainText('Mobile entry')
  expect(
    await page.evaluate(
      () =>
        document.documentElement.scrollWidth <=
        document.documentElement.clientWidth,
    ),
  ).toBe(true)
})
