import type { Page } from '@playwright/test'
import type { Components } from '@revolist/revogrid'
import {
  expect,
  expectBrowserClean,
  expectHTTPFailure,
  observeBrowser,
  paceLogin,
  test,
} from './fixtures'
import { setLocale } from './app-shell'
import type {
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
    { name: `Append ${label} ${Date.now()}` },
  )
  const { book } = await create<{ book: { id: string } }>(
    page,
    `/api/v1/workspaces/${workspace.id}/books`,
    { name: 'Append book' },
  )
  const schema = await create<TableSchema>(
    page,
    `/api/v1/books/${book.id}/tables`,
    { name: 'Append table' },
  )
  const view = schema.views[0]
  if (!view || !schema.table.displayFieldId)
    throw new Error('Append table is missing its grid view or display field')
  return {
    tableId: schema.table.id,
    viewId: view.id,
    nameId: schema.table.displayFieldId,
    path: `/t/${schema.table.id}/v/${view.id}`,
  }
}

async function createField(
  page: Page,
  tableId: string,
  data: Record<string, unknown>,
): Promise<TableField> {
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

async function configureView(
  page: Page,
  tableId: string,
  viewId: string,
  changes: Partial<ViewColumns>,
) {
  const schema = await schemaOf(page, tableId)
  const view = schema.views.find((item) => item.id === viewId)
  if (!view) throw new Error('Append test view is missing')
  const response = await page.request.patch(`/api/v1/views/${viewId}`, {
    headers: { ...headers(), 'If-Match': String(view.version) },
    data: { config: { ...view.config, ...changes } },
  })
  expect(response.status()).toBe(200)
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
  const response = await page.request.post(`/api/v1/tables/${tableId}/rows`, {
    headers: headers(),
    data: { rows },
  })
  expect(response.status()).toBe(201)
  return (await response.json()) as RowMutation
}

async function openGrid(page: Page, path: string) {
  await page.goto(path)
  await expect(page.locator('revo-grid')).toBeVisible()
  if ((await page.locator('html').getAttribute('lang')) === 'zh-CN')
    await setLocale(page, 'en')
}

async function expectPlusAtEnd(page: Page, count: number) {
  const grid = page.locator('revo-grid')
  await expect(grid).toHaveAttribute('aria-rowcount', String(count))
  const plus = page.getByRole('button', { name: en.grid.addRow, exact: true })
  await expect(plus).toBeVisible()
  const end =
    count === 0
      ? grid.locator('.header-rgRow.actual-rgRow').first()
      : grid.locator(`revogr-data .rgRow[data-rgrow="${count - 1}"]`).first()
  await expect(end).toBeVisible()
  const rowBox = await end.boundingBox()
  const plusBox = await plus.boundingBox()
  if (!rowBox || !plusBox) throw new Error('Grid bounds are unavailable')
  expect(Math.abs(plusBox.y - (rowBox.y + rowBox.height))).toBeLessThanOrEqual(
    2,
  )
}

function rowCreateResponse(page: Page, tableId: string) {
  return page.waitForResponse(
    (response) =>
      response.request().method() === 'POST' &&
      new URL(response.url()).pathname === `/api/v1/tables/${tableId}/rows`,
  )
}

test('short grids append directly, use defaults, and preserve undo and realtime', async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, 'The desktop grid has a separate mobile creation form.')
  test.setTimeout(180_000)
  await page.setViewportSize({ width: 1440, height: 900 })
  await login(page)
  const table = await createTable(page, 'short')
  const defaultField = await createField(page, table.tableId, {
    name: 'Defaulted',
    uiType: 'text',
    default: 'server default',
  })

  const peer = await page.context().newPage()
  const peerDiagnostics = await observeBrowser(peer)
  const subscriptions: string[] = []
  peer.on('websocket', (socket) =>
    socket.on('framereceived', ({ payload }) => {
      if (typeof payload !== 'string') return
      const frame = JSON.parse(payload) as { t?: string; tableId?: string }
      if (frame.t === 'subscribed' && frame.tableId)
        subscriptions.push(frame.tableId)
    }),
  )
  try {
    await openGrid(peer, table.path)
    await expect.poll(() => subscriptions.includes(table.tableId)).toBe(true)
    await openGrid(page, table.path)
    await expectPlusAtEnd(page, 0)

    const first = rowCreateResponse(page, table.tableId)
    const plus = page.getByRole('button', { name: en.grid.addRow, exact: true })
    await plus.focus()
    await page.keyboard.press('Enter')
    const response = await first
    expect(response.status()).toBe(201)
    expect(response.request().postDataJSON()).toEqual({
      rows: [{ fields: {} }],
    })
    const created = (await response.json()) as RowMutation
    expect(created.rows).toHaveLength(1)
    expect(created.rows[0]?.fields[defaultField.id]).toBe('server default')
    await expect
      .poll(async () => (await query(page, table.viewId)).total)
      .toBe(1)
    await expect(peer.locator('revo-grid')).toHaveAttribute(
      'aria-rowcount',
      '1',
    )
    await expectPlusAtEnd(page, 1)

    await expect(
      page.getByRole('button', { name: en.undo.undo, exact: true }),
    ).toBeEnabled()
    await page.getByRole('button', { name: en.undo.undo, exact: true }).click()
    await expect
      .poll(async () => (await query(page, table.viewId)).total)
      .toBe(0)
    await page.getByRole('button', { name: en.undo.redo, exact: true }).click()
    await expect
      .poll(async () => (await query(page, table.viewId)).total)
      .toBe(1)
    await page.reload()
    await expectPlusAtEnd(page, 1)
    expect(
      (await query(page, table.viewId)).rows[0]?.fields[defaultField.id],
    ).toBe('server default')

    let release!: () => void
    const held = new Promise<void>((resolve) => {
      release = resolve
    })
    let creates = 0
    const routePattern = `**/api/v1/tables/${table.tableId}/rows`
    await page.route(routePattern, async (route) => {
      if (route.request().method() === 'POST') {
        creates++
        await held
      }
      await route.continue()
    })
    try {
      await plus.click()
      await expect(plus).toBeDisabled()
      await plus.dispatchEvent('click')
      await expect.poll(() => creates).toBe(1)
    } finally {
      release()
    }
    await expect
      .poll(async () => (await query(page, table.viewId)).total)
      .toBe(2)
    await page.unroute(routePattern)
    expect(creates).toBe(1)

    await insertRows(
      page,
      table.tableId,
      Array.from({ length: 3 }, (_, index) => ({
        fields: { [table.nameId]: `Extra ${index}` },
      })),
    )
    for (const [height, width, viewportHeight] of [
      ['short', 1440, 900],
      ['medium', 1200, 720],
      ['extra', 1440, 1000],
    ] as const) {
      await page.setViewportSize({ width, height: viewportHeight })
      await configureView(page, table.tableId, table.viewId, {
        rowHeight: height,
      })
      await page.reload()
      await expectPlusAtEnd(page, 5)
    }
    expect((await query(page, table.viewId)).total).toBe(5)
    expectBrowserClean(peer, peerDiagnostics)
  } finally {
    await peer.close()
  }
})

test('the plus stays at the logical end of a bounded paged result', async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, 'The desktop grid has a separate mobile creation form.')
  test.setTimeout(180_000)
  await page.setViewportSize({ width: 1280, height: 800 })
  await login(page)
  const table = await createTable(page, 'paged')
  const extra = await createField(page, table.tableId, {
    name: 'Wide',
    uiType: 'text',
  })
  const schema = await schemaOf(page, table.tableId)
  await configureView(page, table.tableId, table.viewId, {
    fields: schema.fields.map((field) => ({
      id: field.id,
      visible: field.id === table.nameId || field.id === extra.id,
      width: 1000,
    })),
    frozen: 0,
  })
  await insertRows(
    page,
    table.tableId,
    Array.from({ length: 450 }, (_, index) => ({
      fields: { [table.nameId]: `Row ${String(index).padStart(3, '0')}` },
    })),
  )
  const offsets: number[] = []
  page.on('request', (request) => {
    if (
      request.method() !== 'POST' ||
      new URL(request.url()).pathname !== `/api/v1/views/${table.viewId}/query`
    )
      return
    const body = request.postDataJSON() as { offset?: number }
    if (typeof body.offset === 'number') offsets.push(body.offset)
  })
  await openGrid(page, table.path)
  const grid = page.locator('revo-grid')
  const plus = page.locator('.grid-add-row')
  await expect(grid).toHaveAttribute('aria-rowcount', '450')
  expect(
    await grid
      .locator('revogr-scroll-virtual.horizontal')
      .evaluate((element) => element.scrollWidth > element.clientWidth),
  ).toBe(true)
  await expect(plus).toBeHidden()
  await expect(
    grid.locator('revogr-data .rgRow[data-rgrow="0"]').first(),
  ).toBeVisible()
  await grid.evaluate(async (element) => {
    await (element as HTMLElement & Components.RevoGrid).scrollToRow(99)
  })
  await expect(
    grid.locator('revogr-data .rgRow[data-rgrow="99"]').first(),
  ).toBeVisible()
  await expect(plus).toBeHidden()
  await grid.evaluate(async (element) => {
    await (element as HTMLElement & Components.RevoGrid).scrollToRow(200)
  })
  await expect.poll(() => offsets.includes(200)).toBe(true)
  await expect(
    grid.locator('revogr-data .rgRow[data-rgrow="200"]').first(),
  ).toBeVisible()
  await expect(plus).toBeHidden()

  let releaseTail!: () => void
  const heldTail = new Promise<void>((resolve) => {
    releaseTail = resolve
  })
  let tailRequests = 0
  const queryRoute = `**/api/v1/views/${table.viewId}/query`
  await page.route(queryRoute, async (route) => {
    const body = route.request().postDataJSON() as { offset?: number }
    if (body.offset === 400) {
      tailRequests++
      await heldTail
    }
    await route.continue()
  })
  try {
    await grid.evaluate(async (element) => {
      await (element as HTMLElement & Components.RevoGrid).scrollToRow(449)
    })
    await expect.poll(() => tailRequests).toBe(1)
    await expect(
      grid.locator('revogr-data .rgRow[data-rgrow="449"]').first(),
    ).toBeVisible()
    await page.evaluate(
      () =>
        new Promise<void>((resolve) =>
          requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
        ),
    )
    await expect(plus).toBeHidden()
  } finally {
    releaseTail()
    await page.unrouteAll({ behavior: 'wait' })
  }
  await expectPlusAtEnd(page, 450)
  const source = await grid.evaluate(async (element) => {
    const rows = await (
      element as HTMLElement & Components.RevoGrid
    ).getSource()
    return {
      length: rows.length,
      loaded: rows.filter((row) => row.__rowId).length,
      lastLoaded: Boolean(rows.at(-1)?.__rowId),
    }
  })
  expect(source).toMatchObject({ length: 450, lastLoaded: true })
  expect(source.loaded).toBeGreaterThan(0)
  expect(source.loaded).toBeLessThanOrEqual(300)
  expect(await grid.locator('revogr-data .rgRow').count()).toBeLessThanOrEqual(
    120,
  )

  const created = rowCreateResponse(page, table.tableId)
  await plus.focus()
  await page.keyboard.press('Space')
  const response = await created
  expect(response.status()).toBe(201)
  expect(response.request().postDataJSON()).toEqual({ rows: [{ fields: {} }] })
  await expect
    .poll(async () => (await query(page, table.viewId)).total)
    .toBe(451)
  await expectPlusAtEnd(page, 451)
  expect(
    await grid.evaluate(
      async (element) =>
        (await (element as HTMLElement & Components.RevoGrid).getSource())
          .length,
    ),
  ).toBe(451)
})

test('an expanded multi-page group reveals its newly appended tail row', async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, 'The desktop grid has a separate mobile creation form.')
  test.setTimeout(150_000)
  await page.setViewportSize({ width: 1280, height: 800 })
  await login(page)
  const table = await createTable(page, 'grouped-paged')
  const status = await createField(page, table.tableId, {
    name: 'Status',
    uiType: 'text',
  })
  await insertRows(
    page,
    table.tableId,
    Array.from({ length: 250 }, (_, index) => ({
      fields: { [table.nameId]: `Row ${String(index).padStart(3, '0')}` },
    })),
  )
  await configureView(page, table.tableId, table.viewId, {
    groupBy: [status.id],
  })
  await openGrid(page, table.path)
  await expectPlusAtEnd(page, 1)
  await page
    .getByRole('button', { name: 'Expand Status: (empty)', exact: false })
    .click()
  const grid = page.locator('revo-grid')
  await expect(grid).toHaveAttribute('aria-rowcount', '251')
  await expect
    .poll(() =>
      grid.evaluate(async (element) => {
        const rows = await (
          element as HTMLElement & Components.RevoGrid
        ).getSource()
        const viewport = element.querySelector<HTMLElement>(
          'revogr-viewport-scroll.rgCol .vertical-inner',
        )
        return (
          rows.length === 251 &&
          !!viewport &&
          viewport.scrollHeight > viewport.clientHeight
        )
      }),
    )
    .toBe(true)
  await grid.evaluate(async (element) => {
    await (element as HTMLElement & Components.RevoGrid).scrollToRow(250)
  })
  await expectPlusAtEnd(page, 251)
  const created = rowCreateResponse(page, table.tableId)
  await page.getByRole('button', { name: en.grid.addRow, exact: true }).focus()
  await page.keyboard.press('Space')
  expect((await created).status()).toBe(201)
  await expect
    .poll(async () => (await query(page, table.viewId)).total)
    .toBe(251)
  await expectPlusAtEnd(page, 252)
  const source = await grid.evaluate(async (element) => {
    const rows = await (
      element as HTMLElement & Components.RevoGrid
    ).getSource()
    return {
      length: rows.length,
      loaded: rows.filter((row) => row.__rowId).length,
      lastLoaded: Boolean(rows.at(-1)?.__rowId),
    }
  })
  expect(source).toMatchObject({ length: 252, lastLoaded: true })
  expect(source.loaded).toBeLessThanOrEqual(300)
})

test('filtered, sorted, and grouped results retain one truthful table action', async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, 'The desktop grid has a separate mobile creation form.')
  test.setTimeout(150_000)
  await page.setViewportSize({ width: 1440, height: 900 })
  await login(page)
  const table = await createTable(page, 'grouped')
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
    { fields: { [table.nameId]: 'A', [status.id]: 'Open', [amount.id]: 3 } },
    { fields: { [table.nameId]: 'B', [status.id]: 'Open', [amount.id]: 4 } },
    { fields: { [table.nameId]: 'C', [status.id]: 'Closed', [amount.id]: 5 } },
  ])
  const filter: FilterNode = { field: status.id, cmp: 'is', value: 'Open' }
  await configureView(page, table.tableId, table.viewId, {
    filter,
    sort: [{ field: table.nameId, dir: 'desc' }],
    groupBy: [status.id],
    aggregates: [{ fieldId: amount.id, aggregate: 'sum' }],
    rowHeight: 'tall',
  })
  expect(
    (await query(page, table.viewId, filter)).rows.map(
      (row) => row.fields[table.nameId],
    ),
  ).toEqual(['B', 'A'])
  await openGrid(page, table.path)
  await expectPlusAtEnd(page, 1)
  await expect(page.locator('.grid-aggregate-footer')).toContainText('7')
  await page
    .getByRole('button', { name: 'Expand Status: Open', exact: false })
    .click()
  await expectPlusAtEnd(page, 3)
  await page
    .getByRole('button', { name: 'Collapse Status: Open', exact: false })
    .click()
  await expectPlusAtEnd(page, 1)
  await page.getByRole('button', { name: en.grid.addRow, exact: true }).click()
  await expect
    .poll(async () => (await query(page, allView.view.id)).total)
    .toBe(4)
  expect((await query(page, table.viewId, filter)).total).toBe(2)
  await expect(page.locator('revo-grid')).toHaveAttribute('aria-rowcount', '1')
  await expect(page.locator('.grid-add-row')).toHaveCount(1)
  await expect(
    page.getByRole('status').filter({
      hasText:
        'Row created. Filters, sorting, or collapsed groups may affect where it appears.',
    }),
  ).toBeVisible()
  await expect(page.locator('.grid-aggregate-footer')).toContainText('7')
})

test('blank-create errors and permission changes do not fabricate rows', async ({
  page,
  browser,
  isMobile,
}) => {
  test.skip(isMobile, 'The desktop grid has a separate mobile creation form.')
  test.setTimeout(180_000)
  await page.setViewportSize({ width: 1440, height: 900 })
  await login(page)
  const table = await createTable(page, 'constraints')
  const field = await createField(page, table.tableId, {
    name: 'Required code',
    uiType: 'text',
    isRequired: true,
    isUnique: true,
  })
  await openGrid(page, table.path)
  await expectPlusAtEnd(page, 0)
  const rowsPath = `/api/v1/tables/${table.tableId}/rows`
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
  await page.getByRole('button', { name: en.grid.addRow, exact: true }).click()
  await expect(page.getByRole('alert')).toContainText(en.errors.ERR_INTERNAL)
  expect((await query(page, table.viewId)).total).toBe(0)

  expectHTTPFailure(page, 'POST', rowsPath, 400)
  await page.getByRole('button', { name: en.grid.addRow, exact: true }).click()
  await expect(page.getByRole('alert')).toContainText(
    en.errors.ERR_REQUIRED_FIELD,
  )
  await expect(page.getByRole('alert')).toContainText(
    'Use New Entry to provide the required or unique values.',
  )
  await expect(page.locator('dialog[open]')).toHaveCount(0)
  await setLocale(page, 'zh-CN')
  await expect(page.getByRole('alert')).toContainText(
    zh.errors.ERR_REQUIRED_FIELD,
  )
  await expect(page.getByRole('alert')).toContainText(
    '请使用“新建条目”填写必填值或其他唯一值。',
  )
  await setLocale(page, 'en')
  expect((await query(page, table.viewId)).total).toBe(0)

  const updated = await page.request.patch(`/api/v1/fields/${field.id}`, {
    headers: { ...headers(), 'If-Match': String(field.version) },
    data: { default: 'unique default' },
  })
  expect(updated.status()).toBe(200)
  await page.reload()
  await expectPlusAtEnd(page, 0)
  await page.getByRole('button', { name: en.grid.addRow, exact: true }).click()
  await expect.poll(async () => (await query(page, table.viewId)).total).toBe(1)
  expect((await query(page, table.viewId)).rows[0]?.fields[field.id]).toBe(
    'unique default',
  )
  expectHTTPFailure(page, 'POST', rowsPath, 409)
  await page.getByRole('button', { name: en.grid.addRow, exact: true }).click()
  await expect(page.getByRole('alert')).toContainText(
    en.errors.ERR_UNIQUE_CONFLICT,
  )
  expect((await query(page, table.viewId)).total).toBe(1)

  const { user } = await create<{ user: { id: string; username: string } }>(
    page,
    '/api/v1/admin/users',
    {
      username: `append-reader-${Date.now()}`,
      displayName: 'Append reader',
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
  const readerContext = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL,
    locale: 'en-US',
    viewport: { width: 1440, height: 900 },
  })
  try {
    await paceLogin(page)
    const initialLogin = await readerContext.request.post(
      '/api/v1/auth/login',
      {
        headers: headers(),
        data: { login: user.username, password: 'Temporary-password-123' },
      },
    )
    expect(initialLogin.status()).toBe(200)
    const password = await readerContext.request.post('/api/v1/auth/password', {
      headers: headers(),
      data: {
        current: 'Temporary-password-123',
        new: 'Collaborator-password-123',
      },
    })
    expect(password.status()).toBe(200)
    await paceLogin(page)
    const relogin = await readerContext.request.post('/api/v1/auth/login', {
      headers: headers(),
      data: { login: user.username, password: 'Collaborator-password-123' },
    })
    expect(relogin.status()).toBe(200)
    const reader = await readerContext.newPage()
    const diagnostics = await observeBrowser(reader)
    await openGrid(reader, table.path)
    await expect(reader.locator('.grid-add-row')).toHaveCount(0)
    await grant('write')
    await reader.reload()
    await expectPlusAtEnd(reader, 1)
    await grant('read')
    await reader.reload()
    await expect(reader.locator('.grid-add-row')).toHaveCount(0)
    expectBrowserClean(reader, diagnostics)
  } finally {
    await readerContext.close()
  }
})

test('mobile retains its existing form-based creation', async ({
  page,
  isMobile,
}) => {
  test.skip(!isMobile, 'This checks the existing mobile creation form.')
  test.setTimeout(120_000)
  await login(page)
  const table = await createTable(page, 'mobile')
  await page.goto(table.path)
  if ((await page.locator('html').getAttribute('lang')) === 'zh-CN')
    await setLocale(page, 'en')
  await expect(page.locator('revo-grid')).toHaveCount(0)
  await page.getByRole('button', { name: en.grid.addRow, exact: true }).click()
  const dialog = page.locator('dialog.row-expand')
  await expect(dialog).toBeVisible()
  expect((await query(page, table.viewId)).total).toBe(0)
  await dialog
    .getByRole('button', { name: en.mobileRows.create, exact: true })
    .click()
  await expect.poll(async () => (await query(page, table.viewId)).total).toBe(1)
})
