import type { Page } from '@playwright/test'
import type { Components } from '@revolist/revogrid'
import {
  allowHTTPFailure,
  expect,
  expectBrowserClean,
  observeBrowser,
  paceLogin,
  test,
} from './fixtures'
import { setLocale, setTheme } from './app-shell'
import { openFieldPanel } from './table-controls'
import type { FieldMutation, TableSchema, ViewMutation } from '../src/api/types'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

async function signIn(page: Page) {
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
}

async function create<T>(page: Page, path: string, data: unknown): Promise<T> {
  const response = await page.request.post(path, { headers: headers(), data })
  expect(response.status()).toBe(201)
  return response.json()
}

async function createTable(page: Page, suffix: string) {
  const { workspace } = await create<{ workspace: { id: string } }>(
    page,
    '/api/v1/workspaces',
    { name: `Field tiles ${suffix}` },
  )
  const { book } = await create<{ book: { id: string } }>(
    page,
    `/api/v1/workspaces/${workspace.id}/books`,
    { name: `Field tiles book ${suffix}` },
  )
  return create<TableSchema>(page, `/api/v1/books/${book.id}/tables`, {
    name: `Field tiles table ${suffix}`,
  })
}

async function schemaOf(page: Page, tableId: string) {
  const response = await page.request.get(`/api/v1/tables/${tableId}/schema`)
  expect(response.status()).toBe(200)
  return (await response.json()) as TableSchema
}

async function addField(page: Page, tableId: string, data: object) {
  const result = await create<FieldMutation>(
    page,
    `/api/v1/tables/${tableId}/fields`,
    data,
  )
  if (!result.field) throw new Error('Created field was not returned')
  return result.field
}

async function openActions(page: Page, id: string) {
  const tile = page.locator(`[data-field-id="${id}"]`)
  const details = tile.locator('details.field-actions')
  if ((await details.getAttribute('open')) === null)
    await details.locator('summary').click()
  return details
}

test('field tiles keep menus, editors, visibility and restrictions accessible', async ({
  page,
  browser,
  isMobile,
}) => {
  test.skip(
    isMobile,
    'The desktop project exercises a narrow desktop viewport.',
  )
  test.setTimeout(180_000)
  await page.setViewportSize({ width: 780, height: 760 })
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await signIn(page)
  await setTheme(page, 'light')
  const suffix = String(Date.now())
  const schema = await createTable(page, suffix)
  const tableId = schema.table.id
  const viewId = schema.views[0]!.id
  const quantity = await addField(page, tableId, {
    name: 'Quantity',
    description: 'Existing details',
    uiType: 'integer',
    options: { size: 'int', thousandSep: true },
    isRequired: true,
    isUnique: true,
    default: 7,
  })
  const stage = await addField(page, tableId, {
    name: 'Stage',
    uiType: 'single_select',
    options: {
      choices: [
        { id: 'todo', label: 'Todo', color: 'green' },
        { id: 'done', label: 'Done', color: 'gray' },
      ],
    },
    default: 'Todo',
  })
  const deletable = await addField(page, tableId, {
    name: 'Delete me',
    uiType: 'text',
  })
  const longName = `Long ${'field name '.repeat(8).trim()}`
  const longField = await addField(page, tableId, {
    name: longName,
    uiType: 'text',
  })
  const fieldPatches: string[] = []
  page.on('request', (request) => {
    const path = new URL(request.url()).pathname
    if (request.method() === 'PATCH' && path.startsWith('/api/v1/fields/'))
      fieldPatches.push(path)
  })
  allowHTTPFailure(page, 'POST', `/api/v1/views/${viewId}/query`, 409)
  await page.goto(`/t/${tableId}/v/${viewId}`)
  const panel = await openFieldPanel(page)
  await expect(panel.locator('.field-menu:visible')).toHaveCount(0)
  const quantityTile = panel.locator(`[data-field-id="${quantity.id}"]`)
  const quantityHandle = quantityTile.locator('.field-drag-handle')
  const handleBox = await quantityHandle.boundingBox()
  if (!handleBox) throw new Error('Field drag handle is not visible')
  await page.mouse.move(handleBox.x + 8, handleBox.y + handleBox.height / 2)
  await page.mouse.down()
  await expect(quantityTile).toHaveClass(/field-tile-pressed/)
  expect(
    await quantityTile.evaluate(
      (element) => getComputedStyle(element).transform,
    ),
  ).toBe('none')
  await page.mouse.up()
  await expect(quantityTile).not.toHaveClass(/field-tile-pressed/)
  await page.mouse.down()
  await page.evaluate(() =>
    window.dispatchEvent(new PointerEvent('pointercancel', { pointerId: 1 })),
  )
  await expect(quantityTile).not.toHaveClass(/field-tile-pressed/)
  await page.mouse.up()
  expect(fieldPatches).toEqual([])

  const checkbox = panel.getByLabel(
    en.field.panel.showField.replace('{name}', quantity.name),
    { exact: true },
  )
  const visibilitySave = page.waitForResponse(
    (response) =>
      response.request().method() === 'PATCH' &&
      new URL(response.url()).pathname === `/api/v1/views/${viewId}` &&
      response.status() === 200,
  )
  await checkbox.uncheck()
  await checkbox.check()
  await visibilitySave
  await expect
    .poll(
      async () =>
        (await schemaOf(page, tableId)).views
          .find((view) => view.id === viewId)!
          .config.fields.find((field) => field.id === quantity.id)?.visible,
    )
    .toBe(true)
  expect(fieldPatches).toEqual([])

  const longTile = panel.locator(`[data-field-id="${longField.id}"]`)
  const lightTileColor = await longTile.evaluate(
    (element) => getComputedStyle(element).backgroundColor,
  )
  const longSummary = longTile.locator('summary')
  const tileBox = await longTile.boundingBox()
  const summaryBox = await longSummary.boundingBox()
  if (!tileBox || !summaryBox) throw new Error('Long field tile is not visible')
  expect(summaryBox.x + summaryBox.width).toBeLessThanOrEqual(
    tileBox.x + tileBox.width,
  )
  await longSummary.focus()
  await page.keyboard.press('Enter')
  await expect(panel.locator('.field-menu:visible')).toHaveCount(1)
  const menuBox = await longTile.locator('.field-menu').boundingBox()
  if (!menuBox) throw new Error('Field action menu is not visible')
  expect(menuBox.x).toBeGreaterThanOrEqual(0)
  expect(menuBox.x + menuBox.width).toBeLessThanOrEqual(780)
  expect(menuBox.y + menuBox.height).toBeLessThanOrEqual(760)
  await page.keyboard.press('Escape')
  await expect(longSummary).toBeFocused()
  await expect(panel.locator('.field-menu:visible')).toHaveCount(0)

  const quantityActions = await openActions(page, quantity.id)
  await expect(panel.locator('.field-menu:visible')).toHaveCount(1)
  await quantityActions
    .getByRole('button', { name: en.field.panel.edit, exact: true })
    .click()
  const editor = page.getByRole('dialog', {
    name: en.field.panel.editTitle,
    exact: true,
  })
  await expect(editor.locator('.field-create-form')).toBeVisible()
  await expect(editor.locator('.field-type-selected')).toContainText(
    en.field.kind.integer,
  )
  await expect(
    editor.getByRole('textbox', { name: en.field.panel.name, exact: true }),
  ).toHaveValue('Quantity')
  await expect(
    editor.getByRole('textbox', {
      name: en.field.panel.defaultValue,
      exact: true,
    }),
  ).toHaveValue('7')
  await expect(
    editor.getByRole('checkbox', {
      name: en.field.panel.required,
      exact: true,
    }),
  ).toBeChecked()
  await expect(
    editor.getByRole('checkbox', { name: en.field.panel.unique, exact: true }),
  ).toBeChecked()
  await expect(
    editor.getByRole('textbox', {
      name: en.field.panel.description,
      exact: true,
    }),
  ).toHaveValue('Existing details')
  await editor
    .getByRole('button', { name: en.field.panel.kind, exact: true })
    .click()
  await editor.locator('[data-kind="text"]').click()
  const conversion = page.getByRole('dialog', {
    name: en.field.panel.typeChangeTitle,
    exact: true,
  })
  await expect(conversion).toBeVisible()
  await conversion
    .getByRole('button', { name: en.field.panel.checkConversion, exact: true })
    .click()
  await expect(conversion.locator('.type-change-summary')).toContainText('0')
  await conversion
    .getByRole('button', { name: en.field.panel.abortConversion, exact: true })
    .click()
  await expect(editor).toBeVisible()
  expect(fieldPatches).toEqual([])
  await editor
    .getByRole('button', { name: en.field.panel.cancel, exact: true })
    .click()

  await (
    await openActions(page, stage.id)
  )
    .getByRole('button', { name: en.field.panel.edit, exact: true })
    .click()
  await expect(editor.locator('.field-create-form')).toBeVisible()
  await expect(editor.locator('.field-choice input')).toHaveCount(2)
  await expect(
    editor.getByRole('combobox', {
      name: en.field.panel.defaultValue,
      exact: true,
    }),
  ).toHaveValue('todo')
  await editor
    .getByText(en.field.panel.advancedSettings, { exact: true })
    .click()
  await editor
    .getByRole('button', { name: en.field.panel.setDisplay, exact: true })
    .click()
  await expect
    .poll(async () => (await schemaOf(page, tableId)).table.displayFieldId)
    .toBe(stage.id)
  await editor
    .getByRole('button', { name: en.field.panel.cancel, exact: true })
    .click()

  await (
    await openActions(page, deletable.id)
  )
    .getByRole('button', { name: en.field.panel.delete, exact: true })
    .click()
  const confirmation = page.getByRole('dialog', {
    name: en.field.panel.confirm,
    exact: true,
  })
  await expect(confirmation).toContainText('Delete me')
  await confirmation
    .getByRole('button', { name: en.field.panel.cancel, exact: true })
    .click()
  expect(
    (await schemaOf(page, tableId)).fields.some((f) => f.id === deletable.id),
  ).toBe(true)
  await (
    await openActions(page, deletable.id)
  )
    .getByRole('button', { name: en.field.panel.delete, exact: true })
    .click()
  await confirmation
    .getByRole('button', { name: en.field.panel.confirm, exact: true })
    .click()
  await expect(panel.locator(`[data-field-id="${deletable.id}"]`)).toHaveCount(
    0,
  )

  await panel.getByRole('button', { name: en.field.panel.systemFields }).click()
  const system = (await schemaOf(page, tableId)).fields.find(
    (field) => field.isSystem,
  )!
  const systemTile = panel.locator(`[data-field-id="${system.id}"]`)
  await expect(systemTile.locator('.field-drag-handle')).toHaveCount(0)
  const systemActions = await openActions(page, system.id)
  await expect(
    systemActions.getByRole('button', { name: en.field.panel.edit }),
  ).toBeVisible()
  await expect(
    systemActions.getByRole('button', { name: en.field.panel.delete }),
  ).toHaveCount(0)
  await panel.getByLabel(en.field.panel.search, { exact: true }).click()
  await expect(panel.locator('.field-menu:visible')).toHaveCount(0)

  await page.setViewportSize({ width: 1280, height: 900 })
  await setTheme(page, 'dark')
  await setLocale(page, 'zh-CN')
  await page.locator('.grid-fields-trigger').click()
  const localizedPanel = page.locator('.field-panel-menu')
  await expect(localizedPanel).toBeVisible()
  expect(
    await localizedPanel
      .locator(`[data-field-id="${longField.id}"]`)
      .evaluate((element) => getComputedStyle(element).backgroundColor),
  ).not.toBe(lightTileColor)
  await expect(
    localizedPanel.getByLabel(
      zh.field.panel.dragField.replace('{name}', quantity.name),
      { exact: true },
    ),
  ).toBeVisible()
  await (
    await openActions(page, quantity.id)
  )
    .getByRole('button', { name: zh.field.panel.edit, exact: true })
    .click()
  await expect(
    page.getByRole('dialog', { name: zh.field.panel.editTitle, exact: true }),
  ).toBeVisible()
  await page
    .getByRole('dialog', { name: zh.field.panel.editTitle, exact: true })
    .getByRole('button', { name: zh.field.panel.cancel, exact: true })
    .click()

  const username = `tile-reader-${suffix}`
  const user = await create<{ user: { id: string } }>(
    page,
    '/api/v1/admin/users',
    {
      username,
      displayName: 'Field reader',
      role: 'user',
      password: 'Temporary-password-123',
    },
  )
  const grant = await page.request.put('/api/v1/grants', {
    headers: headers(),
    data: {
      resourceType: 'table',
      resourceId: tableId,
      principalType: 'user',
      principalId: user.user.id,
      level: 'read',
    },
  })
  expect(grant.status()).toBe(201)
  const context = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL,
    locale: 'en-US',
    viewport: { width: 1280, height: 900 },
  })
  try {
    await paceLogin()
    expect(
      (
        await context.request.post('/api/v1/auth/login', {
          headers: headers(),
          data: { login: username, password: 'Temporary-password-123' },
        })
      ).status(),
    ).toBe(200)
    expect(
      (
        await context.request.post('/api/v1/auth/password', {
          headers: headers(),
          data: {
            current: 'Temporary-password-123',
            new: 'Reader-password-123',
          },
        })
      ).status(),
    ).toBe(200)
    await paceLogin()
    expect(
      (
        await context.request.post('/api/v1/auth/login', {
          headers: headers(),
          data: { login: username, password: 'Reader-password-123' },
        })
      ).status(),
    ).toBe(200)
    const reader = await context.newPage()
    const diagnostics = await observeBrowser(reader)
    await reader.goto(`/t/${tableId}/v/${viewId}`)
    if ((await reader.locator('html').getAttribute('lang')) === 'zh-CN')
      await setLocale(reader, 'en')
    const readerPanel = await openFieldPanel(reader)
    await expect(readerPanel).toContainText(en.field.panel.readOnly)
    await expect(readerPanel.locator('.field-actions')).toHaveCount(0)
    await expect(readerPanel.locator('.field-drag-handle')).toHaveCount(0)
    await readerPanel
      .getByLabel(en.field.panel.showField.replace('{name}', quantity.name))
      .uncheck()
    expectBrowserClean(reader, diagnostics)
  } finally {
    await context.close()
  }
})

test('drag and menu moves persist the active grid order without disturbing hidden fields or other views', async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, 'Pointer reorder is covered on desktop.')
  test.setTimeout(150_000)
  await page.setViewportSize({ width: 1280, height: 900 })
  await signIn(page)
  const initial = await createTable(page, `${Date.now()}-order`)
  const tableId = initial.table.id
  const viewId = initial.views[0]!.id
  const first = await addField(page, tableId, {
    name: 'Alpha one',
    uiType: 'text',
  })
  const hidden = await addField(page, tableId, {
    name: 'Hidden',
    uiType: 'text',
  })
  const second = await addField(page, tableId, {
    name: 'Alpha two',
    uiType: 'text',
  })
  const other = await addField(page, tableId, { name: 'Other', uiType: 'text' })
  const third = await addField(page, tableId, {
    name: 'Alpha three',
    uiType: 'text',
  })
  const secondView = await create<ViewMutation>(
    page,
    `/api/v1/tables/${tableId}/views`,
    { name: 'Other grid', type: 'grid', scope: 'shared' },
  )
  const before = await schemaOf(page, tableId)
  const name = before.fields.find(
    (field) => field.id === before.table.displayFieldId,
  )!
  const system = before.fields.filter((field) => field.isSystem)
  const ids = [
    name.id,
    first.id,
    hidden.id,
    second.id,
    other.id,
    third.id,
    ...system.map((field) => field.id),
  ]
  const active = before.views.find((view) => view.id === viewId)!
  const untouched = before.views.find((view) => view.id === secondView.view.id)!
  const response = await page.request.patch(`/api/v1/views/${viewId}`, {
    headers: { ...headers(), 'If-Match': String(active.version) },
    data: {
      config: {
        ...active.config,
        frozen: 0,
        fields: ids.map((id) => ({
          id,
          visible: id !== hidden.id && !system.some((field) => field.id === id),
        })),
      },
    },
  })
  expect(response.status()).toBe(200)
  allowHTTPFailure(page, 'POST', `/api/v1/views/${viewId}/query`, 409)
  await page.goto(`/t/${tableId}/v/${viewId}`)
  const panel = await openFieldPanel(page)
  await panel.getByLabel(en.field.panel.search, { exact: true }).fill('Alpha')
  const firstTile = panel.locator(`[data-field-id="${first.id}"]`)
  const thirdTile = panel.locator(`[data-field-id="${third.id}"]`)
  const firstHandle = firstTile.locator('.field-drag-handle')
  const start = await firstHandle.boundingBox()
  const target = await thirdTile.boundingBox()
  if (!start || !target) throw new Error('Drag tiles are not visible')
  await page.mouse.move(start.x + 12, start.y + start.height / 2)
  await page.mouse.down()
  await expect(firstTile).toHaveClass(/field-tile-pressed/)
  await page.mouse.move(target.x + 30, target.y + target.height - 4, {
    steps: 12,
  })
  await expect(firstTile).toHaveClass(/field-tile-dragging/)
  await expect(thirdTile).toHaveClass(/field-drop-after/)
  await page.mouse.up()
  await expect(firstTile).not.toHaveClass(/field-tile-dragging/)
  const afterDrag = [
    name.id,
    second.id,
    hidden.id,
    third.id,
    other.id,
    first.id,
    ...system.map((field) => field.id),
  ]
  await expect
    .poll(async () =>
      (await schemaOf(page, tableId)).views
        .find((view) => view.id === viewId)!
        .config.fields.map((field) => field.id),
    )
    .toEqual(afterDrag)
  await expect(panel.locator('.field-status')).toContainText('Alpha one')
  const afterSchema = await schemaOf(page, tableId)
  expect(
    afterSchema.views.find((view) => view.id === secondView.view.id),
  ).toEqual(untouched)
  expect(
    afterSchema.fields
      .filter((field) => !field.isSystem)
      .sort((a, b) => a.position - b.position)
      .map((field) => field.id),
  ).toEqual([name.id, hidden.id, second.id, other.id, third.id, first.id])

  const firstActions = await openActions(page, second.id)
  await expect(
    firstActions.getByRole('button', {
      name: en.field.panel.moveUp,
      exact: true,
    }),
  ).toBeDisabled()
  await page.keyboard.press('Escape')
  const lastActions = await openActions(page, first.id)
  await expect(
    lastActions.getByRole('button', {
      name: en.field.panel.moveDown,
      exact: true,
    }),
  ).toBeDisabled()
  await lastActions
    .getByRole('button', { name: en.field.panel.moveUp, exact: true })
    .click()
  const afterMove = [
    name.id,
    second.id,
    hidden.id,
    first.id,
    other.id,
    third.id,
    ...system.map((field) => field.id),
  ]
  await expect
    .poll(async () =>
      (await schemaOf(page, tableId)).views
        .find((view) => view.id === viewId)!
        .config.fields.map((field) => field.id),
    )
    .toEqual(afterMove)
  expect(
    (await schemaOf(page, tableId)).views.find(
      (view) => view.id === secondView.view.id,
    ),
  ).toEqual(untouched)
  await page.reload()
  const visible = [name.id, second.id, first.id, other.id, third.id]
  await expect
    .poll(async () =>
      page.locator('revo-grid').evaluate(async (element, expected) => {
        const grid = element as HTMLElement & Components.RevoGrid
        return (await grid.getColumns())
          .map((column) => String(column.prop))
          .filter((id) => expected.includes(id))
      }, visible),
    )
    .toEqual(visible)
})

test('moving fields preserves pending view changes and exposes retry on a failed view save', async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, 'Field tiles are exercised in the desktop grid.')
  test.setTimeout(150_000)
  await page.setViewportSize({ width: 1280, height: 900 })
  await signIn(page)
  const initial = await createTable(page, `${Date.now()}-pending`)
  const tableId = initial.table.id
  const viewId = initial.views[0]!.id
  const first = await addField(page, tableId, {
    name: 'First',
    uiType: 'text',
  })
  const second = await addField(page, tableId, {
    name: 'Second',
    uiType: 'text',
  })
  const third = await addField(page, tableId, {
    name: 'Third',
    uiType: 'text',
  })
  const hidden = await addField(page, tableId, {
    name: 'Hide me',
    uiType: 'text',
  })
  const fieldPatches: string[] = []
  page.on('request', (request) => {
    const path = new URL(request.url()).pathname
    if (request.method() === 'PATCH' && path.startsWith('/api/v1/fields/'))
      fieldPatches.push(path)
  })
  allowHTTPFailure(page, 'POST', `/api/v1/views/${viewId}/query`, 409)
  await page.goto(`/t/${tableId}/v/${viewId}`)
  const panel = await openFieldPanel(page)

  let release!: () => void
  const heldSave = new Promise<void>((resolve) => {
    release = resolve
  })
  let holding = false
  let failNext = false
  await page.route(`**/api/v1/views/${viewId}`, async (route) => {
    if (route.request().method() !== 'PATCH') {
      await route.continue()
      return
    }
    if (!holding) {
      holding = true
      await heldSave
    } else if (failNext) {
      failNext = false
      await route.fulfill({
        status: 503,
        json: {
          error: { code: 'ERR_STORAGE', message: 'Temporary failure' },
        },
      })
      return
    }
    await route.continue()
  })
  try {
    await panel
      .getByLabel(en.field.panel.showField.replace('{name}', hidden.name))
      .uncheck()
    await expect.poll(() => holding).toBe(true)
    await (
      await openActions(page, first.id)
    )
      .getByRole('button', { name: en.field.panel.moveDown, exact: true })
      .click()
    await expect(panel.getByLabel(en.field.panel.search)).toBeDisabled()
    expect(fieldPatches).toEqual([])
  } finally {
    release()
  }
  await expect(panel.getByRole('status')).toContainText(
    en.field.panel.moved
      .replace('{name}', first.name)
      .replace('{position}', '3')
      .replace('{total}', '4'),
  )
  const afterFirst = await schemaOf(page, tableId)
  const name = afterFirst.table.displayFieldId
  const displayName = afterFirst.fields.find((field) => field.id === name)!.name
  const systemIds = afterFirst.fields
    .filter((field) => field.isSystem)
    .map((field) => field.id)
  const userOrder = (view: { config: { fields: { id: string }[] } }) =>
    view.config.fields
      .map((field) => field.id)
      .filter((id) => !systemIds.includes(id))
  const firstView = afterFirst.views.find((view) => view.id === viewId)!
  expect(userOrder(firstView)).toEqual([
    name,
    second.id,
    first.id,
    third.id,
    hidden.id,
  ])
  expect(
    firstView.config.fields.find((field) => field.id === hidden.id)?.visible,
  ).toBe(false)

  failNext = true
  allowHTTPFailure(page, 'PATCH', `/api/v1/views/${viewId}`, 503)
  await (
    await openActions(page, first.id)
  )
    .getByRole('button', { name: en.field.panel.moveDown, exact: true })
    .click()
  await expect(panel.getByRole('alert')).toHaveText(
    en.field.panel.viewOrderFailed,
  )
  await page.locator('.grid-fields-trigger').click()
  await page
    .getByRole('button', { name: en.grid.columnsRetry, exact: true })
    .click()
  await expect
    .poll(async () =>
      userOrder(
        (await schemaOf(page, tableId)).views.find(
          (view) => view.id === viewId,
        )!,
      ),
    )
    .toEqual([name, second.id, third.id, first.id, hidden.id])
  await page.reload()
  const reloaded = await openFieldPanel(page)
  await expect(reloaded.locator('.field-drag-handle .field-name')).toHaveText([
    displayName,
    second.name,
    third.name,
    first.name,
    hidden.name,
  ])
})
