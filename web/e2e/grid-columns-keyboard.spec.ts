import type { Page, Route } from '@playwright/test'
import type { Components } from '@revolist/revogrid'
import {
  test,
  expect,
  expectHTTPFailure,
  observeBrowser,
  expectBrowserClean,
  paceLogin,
} from './fixtures'
import type { DataRow, TableSchema, ViewColumns } from '../src/api/types'
import { setLocale, setTheme } from './app-shell'
import {
  observeViewQuerySchemaConflicts,
  openGridFormat,
} from './table-controls'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})
const poll = <T>(callback: () => T | Promise<T>) =>
  expect.poll(callback, { intervals: [50, 50, 100] })

test('AT-305/AT-311 production keyboard and shared/local column configuration', async ({
  page,
  browser,
  isMobile,
}) => {
  test.setTimeout(150_000)
  await paceLogin(page)
  await page.goto('/login')
  if ((await page.locator('html').getAttribute('lang')) === 'zh-CN')
    await setLocale(page, 'en')
  await page.getByLabel(en.auth.login, { exact: true }).fill('first-admin')
  await page
    .getByLabel(en.auth.password, { exact: true })
    .fill('Setup-password-123')
  await page.getByRole('button', { name: en.auth.signIn, exact: true }).click()
  await expect(page).toHaveURL(/\/home$/)
  const h = headers()
  const suffix = Date.now()
  const workspace = await page.request.post('/api/v1/workspaces', {
    headers: h,
    data: { name: `Columns ${suffix}` },
  })
  expect(workspace.status()).toBe(201)
  const wid = (await workspace.json()).workspace.id as string
  const book = await page.request.post(`/api/v1/workspaces/${wid}/books`, {
    headers: h,
    data: { name: 'Column book' },
  })
  expect(book.status()).toBe(201)
  const bid = (await book.json()).book.id as string
  const table = await page.request.post(`/api/v1/books/${bid}/tables`, {
    headers: h,
    data: { name: 'Keyboard table' },
  })
  expect(table.status()).toBe(201)
  const tid = (await table.json()).table.id as string
  for (const [name, uiType] of [
    ['Qty', 'integer'],
    ['Check', 'checkbox'],
    ['Note', 'text'],
  ] as const) {
    const response = await page.request.post(`/api/v1/tables/${tid}/fields`, {
      headers: h,
      data: { name, uiType },
    })
    expect(response.status()).toBe(201)
  }
  const schemaOf = async () => {
    const response = await page.request.get(`/api/v1/tables/${tid}/schema`)
    expect(response.status()).toBe(200)
    return (await response.json()) as TableSchema
  }
  const schema = await schemaOf()
  const viewId = schema.views[0]!.id
  // Rapid visibility changes can race a view save under snapshot isolation.
  const checkQueryConflicts = observeViewQuerySchemaConflicts(page, viewId)
  expect(
    (
      await page.request.patch(`/api/v1/views/${viewId}`, {
        headers: { ...h, 'If-Match': String(schema.views[0]!.version) },
        data: { config: { ...schema.views[0]!.config, frozen: 0 } },
      })
    ).status(),
  ).toBe(200)
  const name = schema.fields.find(
    (field) => field.id === schema.table.displayFieldId,
  )!
  const qty = schema.fields.find((field) => field.name === 'Qty')!
  const check = schema.fields.find((field) => field.name === 'Check')!
  const note = schema.fields.find((field) => field.name === 'Note')!
  const system = schema.fields.find((field) => field.uiType === 'id')!
  const created = await page.request.post(`/api/v1/tables/${tid}/rows`, {
    headers: h,
    data: {
      rows: Array.from({ length: 401 }, (_, i) => ({
        fields: { [name.id]: `Row ${i}`, [qty.id]: i, [note.id]: `Note ${i}` },
      })),
    },
  })
  expect(created.status()).toBe(201)
  const data = (await created.json()).rows as DataRow[]
  const base = `/t/${tid}/v/${viewId}`
  await page.goto(base)
  if (isMobile) {
    await expect(page.locator('revo-grid')).toHaveCount(0)
    await expect(
      page.getByRole('region', { name: en.mobileRows.title, exact: true }),
    ).toBeVisible()
    await expect(
      page.getByLabel(en.grid.frozenColumns, { exact: true }),
    ).toHaveCount(0)
    await checkQueryConflicts()
    return
  }
  await page.setViewportSize({ width: 1440, height: 900 })
  const grid = page.locator('revo-grid')
  await expect(grid).toBeVisible()
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
  const value = async (index: number, id: string) => {
    const response = await page.request.get(
      `/api/v1/tables/${tid}/rows/${data[index]!.id}`,
    )
    expect(response.status()).toBe(200)
    return ((await response.json()) as DataRow).fields[id]
  }
  const configOf = async () => (await schemaOf()).views[0]!
  const focus = async (
    fieldId: string,
    row: number,
    toRow = row,
    toId = fieldId,
  ) => {
    await grid.evaluate(
      async (element, { fieldId, row, toRow, toId }) => {
        const grid = element as HTMLElement & Components.RevoGrid
        await grid.scrollToRow(row)
        const column = (await grid.getColumns()).find(
          (column) => column.prop === fieldId,
        )!
        const type = column.pin ?? 'rgCol'
        const store = await grid.getColumnStore(type)
        const source = store.get('source')
        const items = store.get('items')
        const x = items.findIndex((index) => source[index]?.prop === fieldId)
        const x1 = items.findIndex((index) => source[index]?.prop === toId)
        if (x < 0 || x1 < 0)
          throw new Error('Focus columns are not in the same live viewport')
        await grid.setCellsFocus({ x, y: row }, { x: x1, y: toRow }, type)
        grid.focus()
      },
      { fieldId, row, toRow, toId },
    )
    await poll(async () =>
      grid.evaluate(async (element) => {
        const focused = await (
          element as HTMLElement & Components.RevoGrid
        ).getFocused()
        return focused?.model?.__rowId
      }),
    ).toBe(data[row]!.id)
  }
  const focused = async () =>
    grid.evaluate(async (element) => {
      const focused = await (
        element as HTMLElement & Components.RevoGrid
      ).getFocused()
      return { id: focused?.model?.__rowId, field: focused?.column?.prop }
    })
  const expectFocus = async (row: number, field: string) =>
    poll(focused).toEqual({ id: data[row]!.id, field })
  const range = async () =>
    grid.evaluate(async (element) =>
      (element as HTMLElement & Components.RevoGrid).getSelectedRange(),
    )
  const formatMenu = async (target: Page, messages: typeof en | typeof zh) => {
    const details = await openGridFormat(target)
    await expect(
      details.getByText(messages.grid.columnOptions, { exact: true }),
    ).toBeVisible()
    return details.locator('.grid-format-menu')
  }
  const fieldsMenu = async (target: Page) => {
    const trigger = target.locator('.grid-fields-trigger')
    if ((await trigger.getAttribute('aria-expanded')) !== 'true')
      await trigger.click()
    const panel = target.locator('.grid-field-popover')
    await expect(panel).toBeVisible()
    return panel
  }

  for (const [language, theme] of [
    ['en', 'light'],
    ['zh-CN', 'dark'],
  ] as const) {
    const messages = language === 'en' ? en : zh
    await setLocale(page, language)
    await setTheme(page, theme)
    await focus(name.id, 0)
    for (const [key, row, field] of [
      ['ArrowRight', 0, qty.id],
      ['ArrowDown', 1, qty.id],
      ['ArrowLeft', 1, name.id],
      ['ArrowUp', 0, name.id],
      ['Tab', 0, qty.id],
      ['Shift+Tab', 0, name.id],
    ] as const) {
      await page.keyboard.press(key)
      await expectFocus(row, field)
    }
    await page.keyboard.press('Enter')
    const editor = grid.getByLabel(messages.field.editor.value, { exact: true })
    await expect(editor).toBeVisible()
    const text = `${language}-${theme}`
    await editor.fill(text)
    await editor.press('Enter')
    await poll(() => value(0, name.id)).toBe(text)
    await expectFocus(1, name.id)
    await grid.focus()
    await page.keyboard.press('F2')
    await expect(editor).toBeVisible()
    await editor.fill('discarded draft')
    await editor.press('Escape')
    await expect(editor).toHaveCount(0)
    expect(await value(1, name.id)).not.toBe('discarded draft')
    await focus(name.id, 2)
    await page.keyboard.press('Delete')
    await poll(() => value(2, name.id)).toBeNull()
    await focus(name.id, 3)
    await page.keyboard.press('Backspace')
    await poll(() => value(3, name.id)).toBeNull()
    await focus(qty.id, 5)
    await page.keyboard.press('Home')
    await expectFocus(5, name.id)
    await page.keyboard.press('End')
    await expectFocus(5, note.id)
    await page.keyboard.press('Control+End')
    await expectFocus(data.length - 1, note.id)
    await page.keyboard.press('F2')
    await expect(editor).toBeVisible()
    await editor.press('Escape')
    await expect(editor).toHaveCount(0)
    await grid.focus()
    await page.keyboard.press('Control+Home')
    await expectFocus(0, name.id)
    await page.keyboard.press('PageDown')
    const pageRows = await grid.evaluate((element) =>
      Math.max(1, Math.floor((element.clientHeight - 32) / 32)),
    )
    await expectFocus(pageRows, name.id)
    await page.keyboard.press('PageUp')
    await expectFocus(0, name.id)
    await page.keyboard.press('Shift+ArrowDown')
    await poll(range).toMatchObject({ x: 0, y: 0, x1: 0, y1: 1 })
    await page.keyboard.press('Shift+ArrowRight')
    await poll(range).toMatchObject({ x: 0, y: 0, x1: 1, y1: 1 })
    await page.keyboard.press('Shift+ArrowLeft')
    await poll(range).toMatchObject({ x: 0, y: 0, x1: 0, y1: 1 })
    await page.keyboard.press('Shift+ArrowUp')
    await poll(range).toMatchObject({ x: 0, y: 0, x1: 0, y1: 0 })
    await focus(name.id, 0)
    await page.keyboard.press('Control+a')
    await poll(range).toMatchObject({ x: 0, y: 0, x1: 3, y1: data.length - 1 })
    await focus(note.id, 0)
    await page.keyboard.press('ArrowRight')
    await expectFocus(0, note.id)
    await page.keyboard.press('Shift+ArrowRight')
    await poll(range).toMatchObject({ x: 3, y: 0, x1: 3, y1: 0 })
    await focus(name.id, 0, 0, note.id)
    await page.keyboard.press('Control+c')
    await poll(
      async () =>
        (await page.evaluate(() => navigator.clipboard.readText()))
          .split('\n')[0]!
          .split('\t').length,
    ).toBe(4)
    await focus(name.id, 0)
    await page.keyboard.press('Control+c')
    await poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(
      text,
    )
    await page.evaluate(
      (text) => navigator.clipboard.writeText(text),
      `paste-${text}`,
    )
    await page.keyboard.press('Control+v')
    await poll(() => value(0, name.id)).toBe(`paste-${text}`)
    await page.keyboard.press('Control+x')
    await poll(() => value(0, name.id)).toBeNull()
    await page.evaluate(
      (text) => navigator.clipboard.writeText(text),
      `fill-${text}`,
    )
    await page.keyboard.press('Control+v')
    await poll(() => value(0, name.id)).toBe(`fill-${text}`)
    await focus(name.id, 0, 2)
    await page.keyboard.press('Control+d')
    await poll(() => value(2, name.id)).toBe(`fill-${text}`)
    await focus(check.id, 0)
    const before = await value(0, check.id)
    await page.keyboard.press('Space')
    await poll(() => value(0, check.id)).toBe(!before)
    await focus(name.id, 0)
    await page.keyboard.press('Space')
    const dialog = page.getByRole('dialog', {
      name: messages.rowExpand.title.replace('{id}', data[0]!.id),
      exact: true,
    })
    await expect(dialog).toBeVisible()
    await dialog
      .getByRole('button', { name: messages.rowExpand.close, exact: true })
      .click()
    await expect(dialog).toHaveCount(0)
    const localizedMenu = await formatMenu(page, messages)
    const widthLabel = messages.grid.columnWidth.replace('{name}', name.name)
    const widthInput = localizedMenu.getByLabel(widthLabel, { exact: true })
    await widthInput.fill('1001')
    await widthInput.press('Tab')
    await expect(widthInput).toHaveAttribute('aria-invalid', 'true')
    await expect(
      localizedMenu.getByText(
        messages.grid.columnWidthInvalid
          .replace('{min}', '60')
          .replace('{max}', '1000'),
        { exact: true },
      ),
    ).toBeVisible()
    await widthInput.fill('250')
    await widthInput.press('Tab')
    await expect(widthInput).not.toHaveAttribute('aria-invalid', 'true')
    await localizedMenu
      .getByRole('button', {
        name: messages.grid.columnRight.replace('{name}', name.name),
        exact: true,
      })
      .click()
    await localizedMenu
      .getByLabel(messages.grid.frozenColumns, { exact: true })
      .fill('2')
    await localizedMenu
      .getByLabel(messages.grid.frozenColumns, { exact: true })
      .press('Tab')
    await poll(async () => {
      const config = (await configOf()).config
      return {
        ids: config.fields
          .filter((field) => field.visible)
          .map((field) => field.id),
        frozen: config.frozen,
        width: config.fields.find((field) => field.id === name.id)?.width,
      }
    }).toEqual({
      ids: [qty.id, name.id, check.id, note.id],
      frozen: 2,
      width: 250,
    })
    await localizedMenu
      .getByRole('button', {
        name: messages.grid.columnLeft.replace('{name}', name.name),
        exact: true,
      })
      .click()
    await localizedMenu
      .getByLabel(messages.grid.frozenColumns, { exact: true })
      .fill('0')
    await localizedMenu
      .getByLabel(messages.grid.frozenColumns, { exact: true })
      .press('Tab')
    await poll(async () => {
      const config = (await configOf()).config
      return {
        ids: config.fields
          .filter((field) => field.visible)
          .map((field) => field.id),
        frozen: config.frozen,
      }
    }).toEqual({ ids: [name.id, qty.id, check.id, note.id], frozen: 0 })
    await localizedMenu.locator('input[type="number"]').first().press('Escape')
    await expect(page.locator('.grid-format')).not.toHaveAttribute('open', '')

    await page.locator('.grid-fields-trigger').click()
    await expect(page.locator('.grid-field-popover')).toBeVisible()
    await page.locator('.grid-fields-trigger').click()
    await expect(page.locator('.grid-field-popover')).not.toBeVisible()
  }
  await setLocale(page, 'en')
  await setTheme(page, 'light')

  await expect(
    page
      .getByRole('region', { name: en.grid.title })
      .getByRole('button', { name: en.auth.retry, exact: true }),
  ).toHaveCount(0)
  await openGridFormat(page)
  const format = page.locator('.grid-format')
  await expect(format).toHaveAttribute('open', '')
  await page.locator('.grid-fields-trigger').click()
  await expect(format).not.toHaveAttribute('open', '')
  await page.locator('.grid-fields-trigger').click()

  const nameHeader = grid.getByRole('columnheader', {
    name: name.name,
    exact: true,
  })
  const widthBeforeResize = (await configOf()).config.fields.find(
    (field) => field.id === name.id,
  )!.width!
  await nameHeader.scrollIntoViewIfNeeded()
  const resizeHandle = await nameHeader.locator('.resizable-r').boundingBox()
  if (!resizeHandle)
    throw new Error('Production column resize handle is absent')
  await page.mouse.move(
    resizeHandle.x + resizeHandle.width / 2,
    resizeHandle.y + resizeHandle.height / 2,
  )
  await page.mouse.down()
  await page.mouse.move(
    resizeHandle.x + 45,
    resizeHandle.y + resizeHandle.height / 2,
    { steps: 8 },
  )
  await page.mouse.up()
  await poll(
    async () =>
      (await configOf()).config.fields.find((field) => field.id === name.id)
        ?.width ?? 0,
  ).toBeGreaterThan(widthBeforeResize)
  const noteHeader = await grid
    .getByRole('columnheader', { name: note.name, exact: true })
    .boundingBox()
  const checkHeader = await grid
    .getByRole('columnheader', { name: check.name, exact: true })
    .boundingBox()
  if (!noteHeader || !checkHeader)
    throw new Error('Production drag headers are absent')
  await page.mouse.move(
    noteHeader.x + noteHeader.width / 2,
    noteHeader.y + noteHeader.height / 2,
  )
  await page.mouse.down()
  await page.mouse.move(
    checkHeader.x + 10,
    checkHeader.y + checkHeader.height / 2,
    { steps: 12 },
  )
  await page.mouse.up()
  await poll(async () =>
    (await configOf()).config.fields
      .filter((field) => field.visible)
      .map((field) => field.id),
  ).toEqual([name.id, qty.id, note.id, check.id])
  const menu = await formatMenu(page, en)
  await menu
    .getByLabel(en.grid.columnWidth.replace('{name}', name.name), {
      exact: true,
    })
    .fill('260')
  await menu
    .getByLabel(en.grid.columnWidth.replace('{name}', name.name), {
      exact: true,
    })
    .press('Tab')
  await menu
    .getByRole('button', {
      name: en.grid.columnRight.replace('{name}', name.name),
      exact: true,
    })
    .click()
  const fields = await fieldsMenu(page)
  await fields
    .getByLabel(en.field.panel.showField.replace('{name}', note.name))
    .uncheck()
  const freezeMenu = await formatMenu(page, en)
  await freezeMenu.getByLabel(en.grid.frozenColumns, { exact: true }).fill('2')
  await freezeMenu
    .getByLabel(en.grid.frozenColumns, { exact: true })
    .press('Tab')
  await poll(async () => {
    const view = await configOf()
    return {
      visible: view.config.fields
        .filter((field) => field.visible)
        .map((field) => field.id),
      frozen: view.config.frozen,
      width: view.config.fields.find((field) => field.id === name.id)?.width,
    }
  }).toEqual({ visible: [qty.id, name.id, check.id], frozen: 2, width: 260 })
  await page.reload()
  await poll(async () =>
    grid.evaluate(async (element) =>
      (await (element as HTMLElement & Components.RevoGrid).getColumns())
        .filter((column) => column.pin === 'colPinStart')
        .map((column) => column.prop),
    ),
  ).toEqual(['__selected', qty.id, name.id])
  const systemFields = await fieldsMenu(page)
  await systemFields
    .getByRole('button', { name: en.field.panel.systemFields, exact: true })
    .click()
  await systemFields
    .getByLabel(en.field.panel.showField.replace('{name}', system.name))
    .check()
  await poll(
    async () =>
      (await configOf()).config.fields.find((field) => field.id === system.id)
        ?.visible,
  ).toBe(true)
  await focus(system.id, 0)
  const beforeSystem = await value(0, system.id)
  await page.keyboard.press('Delete')
  expect(await value(0, system.id)).toBe(beforeSystem)

  const userResponse = await page.request.post('/api/v1/admin/users', {
    headers: h,
    data: {
      username: `columns-${suffix}`,
      displayName: 'Column collaborator',
      password: 'Temporary-password-123',
      role: 'user',
    },
  })
  expect(userResponse.status()).toBe(201)
  const user = (await userResponse.json()).user as {
    id: string
    username: string
  }
  const grantBody = {
    resourceType: 'table',
    resourceId: tid,
    principalType: 'user',
    principalId: user.id,
    level: 'read',
  }
  expect(
    (
      await page.request.put('/api/v1/grants', { headers: h, data: grantBody })
    ).status(),
  ).toBe(201)
  const context = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL,
    locale: 'en-US',
    viewport: { width: 1440, height: 900 },
  })
  try {
    expect(
      (
        await context.request.post('/api/v1/auth/login', {
          headers: h,
          data: { login: user.username, password: 'Temporary-password-123' },
        })
      ).status(),
    ).toBe(200)
    expect(
      (
        await context.request.post('/api/v1/auth/password', {
          headers: h,
          data: {
            current: 'Temporary-password-123',
            new: 'Collaborator-password-123',
          },
        })
      ).status(),
    ).toBe(200)
    expect(
      (
        await context.request.post('/api/v1/auth/login', {
          headers: h,
          data: { login: user.username, password: 'Collaborator-password-123' },
        })
      ).status(),
    ).toBe(200)
    const collaborator = await context.newPage()
    const diagnostics = await observeBrowser(collaborator)
    await collaborator.goto(base)
    await expect(collaborator.locator('revo-grid')).toBeVisible()
    const requests: string[] = []
    collaborator.on('request', (request) => {
      if (
        request.method() === 'PATCH' &&
        new URL(request.url()).pathname === `/api/v1/views/${viewId}`
      )
        requests.push(request.postData() ?? '')
    })
    const stored = await configOf()
    const localFields = await fieldsMenu(collaborator)
    await expect(
      collaborator.getByText(en.grid.columnsLocal, { exact: true }),
    ).toBeVisible()
    await localFields
      .getByLabel(en.field.panel.showField.replace('{name}', name.name))
      .uncheck()
    const local = await formatMenu(collaborator, en)
    await local
      .getByLabel(en.grid.columnWidth.replace('{name}', qty.name), {
        exact: true,
      })
      .fill('340')
    await local
      .getByLabel(en.grid.columnWidth.replace('{name}', qty.name), {
        exact: true,
      })
      .press('Tab')
    await local
      .getByRole('button', {
        name: en.grid.columnRight.replace('{name}', qty.name),
        exact: true,
      })
      .click()
    await local.getByLabel(en.grid.frozenColumns, { exact: true }).fill('1')
    await local.getByLabel(en.grid.frozenColumns, { exact: true }).press('Tab')
    await poll(async () =>
      collaborator
        .locator('revo-grid')
        .evaluate(
          async (element, id) =>
            (
              await (element as HTMLElement & Components.RevoGrid).getColumns()
            ).some((column) => column.prop === id),
          name.id,
        ),
    ).toBe(false)
    expect(requests).toEqual([])
    expect((await configOf()).version).toBe(stored.version)
    expect((await configOf()).config).toEqual(stored.config)
    await collaborator.reload()
    await poll(async () =>
      collaborator
        .locator('revo-grid')
        .evaluate(
          async (element, id) =>
            (
              await (element as HTMLElement & Components.RevoGrid).getColumns()
            ).some((column) => column.prop === id),
          name.id,
        ),
    ).toBe(true)
    expect(
      (
        await page.request.put('/api/v1/grants', {
          headers: h,
          data: { ...grantBody, level: 'write' },
        })
      ).status(),
    ).toBe(200)
    await collaborator.reload()
    const writable = await formatMenu(collaborator, en)
    await writable
      .getByLabel(en.grid.columnWidth.replace('{name}', qty.name), {
        exact: true,
      })
      .fill('310')
    await writable
      .getByLabel(en.grid.columnWidth.replace('{name}', qty.name), {
        exact: true,
      })
      .press('Tab')
    await poll(
      async () =>
        (await configOf()).config.fields.find((field) => field.id === qty.id)
          ?.width,
    ).toBe(310)
    expect(requests.length).toBeGreaterThan(0)
    await page.reload()
    await poll(async () =>
      grid.evaluate(
        async (element) =>
          (
            await (element as HTMLElement & Components.RevoGrid).getColumns()
          ).find((column) => column.name === 'Qty')?.size,
      ),
    ).toBe(310)
    expectBrowserClean(collaborator, diagnostics)
  } finally {
    await context.close()
  }

  const beforeConflict = await configOf()
  const concurrent: ViewColumns = {
    ...beforeConflict.config,
    fields: beforeConflict.config.fields.map((field) =>
      field.id === qty.id ? { ...field, width: 320 } : field,
    ),
  }
  expect(
    (
      await page.request.patch(`/api/v1/views/${viewId}`, {
        headers: { ...h, 'If-Match': String(beforeConflict.version) },
        data: { config: concurrent },
      })
    ).status(),
  ).toBe(200)
  const viewRoute = `**/api/v1/views/${viewId}`
  let stalePatchSent = false
  const forceStaleVersion = async (route: Route) => {
    if (route.request().method() === 'PATCH' && !stalePatchSent) {
      // Reproduce a delayed realtime notification even if it arrives first.
      stalePatchSent = true
      await route.continue({
        headers: {
          ...route.request().headers(),
          'if-match': String(beforeConflict.version),
        },
      })
    } else {
      await route.continue()
    }
  }
  await page.route(viewRoute, forceStaleVersion)
  try {
    expectHTTPFailure(page, 'PATCH', `/api/v1/views/${viewId}`, 409)
    const conflictMenu = await formatMenu(page, en)
    await conflictMenu
      .getByLabel(en.grid.columnWidth.replace('{name}', name.name), {
        exact: true,
      })
      .fill('285')
    await conflictMenu
      .getByLabel(en.grid.columnWidth.replace('{name}', name.name), {
        exact: true,
      })
      .press('Tab')
    await poll(async () => {
      const config = (await configOf()).config
      return [
        config.fields.find((field) => field.id === name.id)?.width,
        config.fields.find((field) => field.id === qty.id)?.width,
      ]
    }).toEqual([285, 320])
    await expect(
      page
        .getByRole('alert')
        .filter({ hasText: en.errors.ERR_VERSION_CONFLICT }),
    ).toBeVisible()
    expect(stalePatchSent).toBe(true)
    const sourceCount = () =>
      grid.evaluate(
        async (element) =>
          (
            await (element as HTMLElement & Components.RevoGrid).getSource()
          ).filter((row) => row.__rowId).length,
      )
    await poll(sourceCount).toBeGreaterThan(0)
    expect(await sourceCount()).toBeLessThanOrEqual(300)
    await expect(
      page
        .getByRole('region', { name: en.grid.title })
        .getByRole('button', { name: en.auth.retry, exact: true }),
    ).toHaveCount(0)
  } finally {
    await page.unroute(viewRoute, forceStaleVersion)
  }
  await checkQueryConflicts()
})
