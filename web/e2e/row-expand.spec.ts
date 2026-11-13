import type { Page } from '@playwright/test'
import type { Components } from '@revolist/revogrid'
import {
  test,
  expect,
  expectHTTPFailure,
  observeBrowser,
  expectBrowserClean,
  paceLogin,
} from './fixtures'
import type { DataRow, TableSchema } from '../src/api/types'
import { setLocale, setTheme, signOut } from './app-shell'
import { openFieldPanel } from './table-controls'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

type HeldResponseWindow = Window & {
  rowResponseHeld?: boolean
  releaseRowResponse?: () => void
}

async function holdRowResponse(page: Page, path: string) {
  await page.evaluate((path) => {
    const state = window as HeldResponseWindow
    const original = window.fetch
    state.rowResponseHeld = false
    let release!: () => void
    const held = new Promise<void>((resolve) => {
      release = resolve
    })
    state.releaseRowResponse = () => {
      window.fetch = original
      release()
    }
    window.fetch = async (...args: Parameters<typeof fetch>) => {
      const response = await original(...args)
      if (new URL(response.url).pathname === path) {
        state.rowResponseHeld = true
        await held
      }
      return response
    }
  }, path)
}

async function login(page: Page) {
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

test('row workspace controls, side activity and unsaved duplication persist only on confirmation', async ({
  page,
  isMobile,
}) => {
  test.setTimeout(90_000)
  await login(page)
  const h = headers()
  const workspace = await page.request.post('/api/v1/workspaces', {
    headers: h,
    data: { name: `Popup ${Date.now()}` },
  })
  expect(workspace.status()).toBe(201)
  const wid = (await workspace.json()).workspace.id as string
  const book = await page.request.post(`/api/v1/workspaces/${wid}/books`, {
    headers: h,
    data: { name: 'Popup book' },
  })
  expect(book.status()).toBe(201)
  const bid = (await book.json()).book.id as string
  const table = await page.request.post(`/api/v1/books/${bid}/tables`, {
    headers: h,
    data: { name: 'Popup table' },
  })
  expect(table.status()).toBe(201)
  const tid = (await table.json()).table.id as string
  const schemaResponse = await page.request.get(`/api/v1/tables/${tid}/schema`)
  const schema = (await schemaResponse.json()) as TableSchema
  const name = schema.fields.find((field) => !field.isSystem)!
  const viewId = schema.views[0]!.id
  const inserted = await page.request.post(`/api/v1/tables/${tid}/rows`, {
    headers: h,
    data: {
      rows: [
        { fields: { [name.id]: 'First entry' } },
        { fields: { [name.id]: 'Second entry' } },
      ],
    },
  })
  expect(inserted.status()).toBe(201)
  const originals = (await inserted.json()).rows as DataRow[]
  const base = `/t/${tid}/v/${viewId}`
  const queryRows = async () => {
    const result = await page.request.post(`/api/v1/views/${viewId}/query`, {
      headers: h,
      data: { limit: 100 },
    })
    expect(result.status()).toBe(200)
    return (await result.json()).rows as DataRow[]
  }
  await page.goto(base)
  if (isMobile)
    await page
      .locator(`[data-row-id="${originals[0]!.id}"] button.mobile-card`)
      .click()
  else {
    await expect(
      page
        .locator('revo-grid')
        .getByRole('columnheader', { name: 'ID', exact: true }),
    ).toHaveCount(0)
    await page
      .getByRole('button', {
        name: en.rowExpand.open.replace('{id}', originals[0]!.id),
        exact: true,
      })
      .click()
  }
  const dialog = page.locator('dialog.row-expand')
  await expect(dialog.locator(':scope > header h2')).toHaveText('First entry')
  const save = dialog.getByRole('button', {
    name: en.gridPolish.gp05.save,
    exact: true,
  })
  await expect(save).toBeDisabled()
  await expect(dialog.locator('header .row-search')).toBeVisible()
  await expect(dialog.locator('.row-toolbar')).toHaveCount(0)
  const nameInput = dialog.locator(
    `[data-field-id="${name.id}"] .field-editor input`,
  )
  const labelBox = await dialog
    .locator(`[data-field-id="${name.id}"] h3`)
    .boundingBox()
  const inputBox = await nameInput.boundingBox()
  expect(labelBox).not.toBeNull()
  expect(inputBox).not.toBeNull()
  expect(inputBox!.y).toBeGreaterThanOrEqual(labelBox!.y + labelBox!.height)
  const search = dialog.getByRole('searchbox', {
    name: en.rowExpand.search,
    exact: true,
  })
  await search.fill('First entry')
  await nameInput.fill('Unsaved entry')
  await expect(nameInput).toBeVisible()
  await dialog.locator(':scope > header h2').click()
  await expect(save).toBeEnabled()
  expect(
    (await queryRows()).find((row) => row.id === originals[0]!.id)?.fields[
      name.id
    ],
  ).toBe('First entry')
  await nameInput.fill('First entry')
  await dialog.locator(':scope > header h2').click()
  await expect(save).toBeDisabled()
  await search.fill('Name')
  await expect(dialog.locator('.row-field:visible')).toHaveCount(1)
  await search.fill('')
  await expect(dialog.locator('.row-field')).toHaveCount(schema.fields.length)
  await expect(
    dialog.getByRole('button', { name: 'Toggle compact view', exact: true }),
  ).toHaveCount(0)
  for (const label of [en.comments.title, en.history.title]) {
    await dialog.getByRole('tab', { name: label, exact: true }).click()
    await expect(dialog.locator('.row-form')).toBeVisible()
    await expect(dialog.locator('.row-activity')).toBeVisible()
  }
  if (!isMobile) {
    const formBox = await dialog.locator('.row-form').boundingBox()
    const activityBox = await dialog.locator('.row-activity').boundingBox()
    expect(formBox).not.toBeNull()
    expect(activityBox).not.toBeNull()
    expect(activityBox!.x).toBeGreaterThanOrEqual(
      formBox!.x + formBox!.width - 1,
    )
  }
  expect(
    await dialog.evaluate(
      (element) => element.scrollWidth <= element.clientWidth,
    ),
  ).toBe(true)
  await dialog
    .getByRole('button', { name: en.rowExpand.collapseActivity, exact: true })
    .click()
  await expect(dialog.locator('.row-activity')).not.toBeVisible()
  await dialog
    .getByRole('button', { name: en.rowExpand.expandActivity, exact: true })
    .click()
  await expect(dialog.locator('.row-activity')).toBeVisible()
  if (!isMobile) {
    await dialog
      .getByRole('button', { name: en.rowExpand.fullscreen, exact: true })
      .click()
    const area = await page.locator('.row-workspace').boundingBox()
    const popup = await dialog.boundingBox()
    expect(area).not.toBeNull()
    expect(popup).not.toBeNull()
    for (const key of ['x', 'y', 'width', 'height'] as const)
      expect(Math.abs(area![key] - popup![key])).toBeLessThan(2)
    await dialog
      .getByRole('button', { name: en.rowExpand.fullscreen, exact: true })
      .click()
  }
  await dialog
    .getByRole('button', { name: en.rowExpand.next, exact: true })
    .click()
  await expect(dialog.locator(':scope > header h2')).toHaveText('Second entry')
  await dialog
    .getByRole('button', { name: en.rowExpand.previous, exact: true })
    .click()
  await expect(dialog.locator(':scope > header h2')).toHaveText('First entry')
  await dialog
    .getByRole('button', { name: en.rowExpand.duplicate, exact: true })
    .click()
  expect(await queryRows()).toHaveLength(2)
  const field = dialog.locator(`[data-field-id="${name.id}"]`)
  await field.locator('.field-editor input').fill('Draft copy')
  expect(await queryRows()).toHaveLength(2)
  await save.click()
  await expect(dialog.locator(':scope > header h2')).toHaveText('Draft copy')
  await expect(save).toBeDisabled()
  const rows = await queryRows()
  expect(rows).toHaveLength(3)
  const copy = rows.find((row) => row.fields[name.id] === 'Draft copy')!
  expect(originals.map((row) => row.id)).not.toContain(copy.id)
  await dialog
    .getByRole('button', { name: en.rowExpand.delete, exact: true })
    .click()
  const confirmation = page.locator('.row-delete-dialog')
  await expect(confirmation).toBeVisible()
  expect(await queryRows()).toHaveLength(3)
  await confirmation
    .getByRole('button', { name: en.admin.cancel, exact: true })
    .click()
  expect(await queryRows()).toHaveLength(3)
  await dialog
    .getByRole('button', { name: en.rowExpand.delete, exact: true })
    .click()
  await confirmation
    .getByRole('button', { name: en.rowExpand.delete, exact: true })
    .click()
  await expect(dialog).toHaveCount(0)
  expect((await queryRows()).map((row) => row.id)).toEqual(
    originals.map((row) => row.id),
  )
})

test('AT-310 real row expand, exact editors, deep links, ACL and stale responses', async ({
  page,
  browser,
  isMobile,
}) => {
  test.setTimeout(150_000)
  await login(page)
  const h = headers()
  const suffix = Date.now()
  const workspaceResponse = await page.request.post('/api/v1/workspaces', {
    headers: h,
    data: { name: `Rows ${suffix}` },
  })
  expect(workspaceResponse.status()).toBe(201)
  const wid = (await workspaceResponse.json()).workspace.id as string
  const bookResponse = await page.request.post(
    `/api/v1/workspaces/${wid}/books`,
    { headers: h, data: { name: 'Rows book' } },
  )
  expect(bookResponse.status()).toBe(201)
  const bid = (await bookResponse.json()).book.id as string
  const tableResponse = await page.request.post(`/api/v1/books/${bid}/tables`, {
    headers: h,
    data: { name: 'Rows table' },
  })
  expect(tableResponse.status()).toBe(201)
  const tid = (await tableResponse.json()).table.id as string
  const schemaOf = async () => {
    const response = await page.request.get(`/api/v1/tables/${tid}/schema`)
    expect(response.status()).toBe(200)
    return (await response.json()) as TableSchema
  }
  const initial = await schemaOf()
  const viewId = initial.views[0]!.id
  expect(
    (
      await page.request.patch(`/api/v1/views/${viewId}`, {
        headers: { ...h, 'If-Match': String(initial.views[0]!.version) },
        data: { config: { ...initial.views[0]!.config, frozen: 0 } },
      })
    ).status(),
  ).toBe(200)
  const base = `/t/${tid}/v/${viewId}`
  const choices = {
    choices: [
      { id: 'a', label: 'Alpha', color: 'red' },
      { id: 'b', label: 'Beta', color: 'blue' },
    ],
  }
  const specs = [
    {
      name: 'Notes',
      uiType: 'long_text',
      options: {},
      input: '第一行\nsecond line',
      value: '第一行\nsecond line',
    },
    {
      name: 'Big',
      uiType: 'integer',
      options: { size: 'big' },
      input: '9223372036854775807',
      value: '9223372036854775807',
    },
    {
      name: 'Exact',
      uiType: 'decimal',
      options: { precision: 38, scale: 6 },
      input: '123456789012345678901234567890.123456',
      value: '123456789012345678901234567890.123456',
    },
    {
      name: 'Float',
      uiType: 'float',
      options: { size: 'real' },
      input: '1.23456789',
      value: 1.2345679,
    },
    { name: 'Check', uiType: 'checkbox', options: {}, input: '', value: true },
    {
      name: 'Date',
      uiType: 'date',
      options: {},
      input: '2026-10-27',
      value: '2026-10-27',
    },
    {
      name: 'Instant',
      uiType: 'datetime',
      options: {},
      input: '2026-10-27T15:00:00.123456+08:00',
      value: '2026-10-27T07:00:00.123456Z',
    },
    {
      name: 'Stage',
      uiType: 'single_select',
      options: choices,
      input: 'Beta',
      value: 'Beta',
    },
    {
      name: 'Tags',
      uiType: 'multi_select',
      options: choices,
      input: '',
      value: ['Alpha', 'Beta'],
    },
    {
      name: 'Email',
      uiType: 'email',
      options: {},
      input: 'valid@example.com',
      value: 'valid@example.com',
    },
    {
      name: 'URL',
      uiType: 'url',
      options: {},
      input: 'https://example.com/a',
      value: 'https://example.com/a',
    },
  ]
  for (const spec of specs) {
    const response = await page.request.post(`/api/v1/tables/${tid}/fields`, {
      headers: h,
      data: { name: spec.name, uiType: spec.uiType, options: spec.options },
    })
    expect(response.status()).toBe(201)
  }
  await page.goto(base)
  if (isMobile) {
    await expect(page.locator('revo-grid')).toHaveCount(0)
    await expect(
      page.getByRole('region', { name: en.mobileRows.title, exact: true }),
    ).toBeVisible()
    return
  }
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.getByRole('button', { name: en.grid.addRow, exact: true }).click()
  const schema = await schemaOf()
  const fields = await openFieldPanel(page)
  await fields
    .getByLabel(en.field.panel.showField.replace('{name}', 'Notes'), {
      exact: true,
    })
    .uncheck()
  await page.locator('.grid-fields-trigger').click()
  const query = await page.request.post(`/api/v1/views/${viewId}/query`, {
    headers: h,
    data: { limit: 100 },
  })
  expect(query.status()).toBe(200)
  const row = (await query.json()).rows[0] as DataRow
  const rowPath = `${base}/r/${row.id}`
  const apiRowPath = `/api/v1/tables/${tid}/rows/${row.id}`
  const openButton = page.getByRole('button', {
    name: en.rowExpand.open.replace('{id}', row.id),
    exact: true,
  })
  await expect(openButton).toBeVisible()
  await openButton.click()
  await expect(page).toHaveURL(new RegExp(`/r/${row.id}$`))
  const dialog = page.locator('dialog.row-expand')
  await expect(dialog).toBeVisible()
  await expect(dialog.locator('.row-field')).toHaveCount(17)
  await expect(
    dialog.getByText(en.rowExpand.system, { exact: true }),
  ).toHaveCount(5)
  await dialog.getByRole('button', { name: en.rowExpand.close }).click()
  await expect(dialog).not.toBeVisible()

  const gridElement = page.locator('revo-grid')
  const [gridBox, headerBox, rowBox] = await Promise.all([
    gridElement.boundingBox(),
    gridElement
      .getByRole('columnheader', { name: 'Name', exact: true })
      .boundingBox(),
    gridElement
      .getByRole('checkbox', {
        name: en.grid.selectRow.replace('{id}', row.id),
        exact: true,
      })
      .boundingBox(),
  ])
  expect(gridBox).not.toBeNull()
  expect(headerBox).not.toBeNull()
  expect(rowBox).not.toBeNull()
  await gridElement.click({
    position: {
      x: headerBox!.x + headerBox!.width / 2 - gridBox!.x,
      y: rowBox!.y + rowBox!.height / 2 - gridBox!.y,
    },
  })
  await expect
    .poll(() =>
      gridElement.evaluate((element) =>
        (element as HTMLElement & Components.RevoGrid).getSelectedRange(),
      ),
    )
    .toMatchObject({ x: 0, y: 0 })
  const inlineEditor = gridElement.getByRole('textbox', {
    name: en.field.editor.value,
    exact: true,
  })
  await expect(inlineEditor).toBeVisible()
  await inlineEditor.press('Escape')
  await expect(inlineEditor).toBeHidden()
  // Grid keyboard focus settles asynchronously after the editor closes.
  await expect(async () => {
    if (!(await dialog.isVisible())) await page.keyboard.press('Space')
    await expect(dialog).toBeVisible({ timeout: 1_500 })
  }).toPass({ timeout: 10_000 })
  const nameField = schema.fields.find((field) => field.uiType === 'text')!
  const edit = (fieldId: string) =>
    dialog.locator(`[data-field-id="${fieldId}"]`)
  const save = dialog.getByRole('button', {
    name: en.gridPolish.gp05.save,
    exact: true,
  })
  await expect(save).toBeDisabled()
  let section = edit(nameField.id)
  await section.locator('.field-editor input').fill('Row 中文')
  await dialog.locator(':scope > header h2').click()
  await expect(save).toBeEnabled()
  expect(
    (await (await page.request.get(apiRowPath)).json()).fields[nameField.id],
  ).not.toBe('Row 中文')
  for (const spec of specs) {
    const field = schema.fields.find((field) => field.name === spec.name)!
    section = edit(field.id)
    if (spec.uiType === 'checkbox') await section.getByRole('checkbox').check()
    else if (spec.uiType === 'single_select')
      await section.locator('select').selectOption('Beta')
    else {
      if (spec.uiType === 'multi_select')
        await section.locator('select').selectOption(['Beta', 'Alpha'])
      else
        await section
          .locator('.field-editor input, .field-editor textarea')
          .fill(spec.input)
    }
    await expect(section.locator('.field-editor')).toHaveCount(1)
  }
  const patches: Record<string, unknown>[] = []
  page.on('request', (request) => {
    if (
      new URL(request.url()).pathname === apiRowPath &&
      request.method() === 'PATCH'
    )
      patches.push(request.postDataJSON() as Record<string, unknown>)
  })
  await save.click()
  await expect(save).toBeDisabled()
  expect(patches).toHaveLength(1)
  for (const spec of specs) {
    const field = schema.fields.find((field) => field.name === spec.name)!
    const persisted = await page.request.get(apiRowPath)
    expect(persisted.status()).toBe(200)
    expect((await persisted.json()).fields[field.id]).toEqual(spec.value)
  }
  await page.reload()
  await expect(dialog).toBeVisible()
  await expect(
    dialog.locator(`[data-field-id="${nameField.id}"] input`),
  ).toHaveValue('Row 中文')
  await expect(dialog.locator('.row-field')).toHaveCount(17)
  const exactField = schema.fields.find((field) => field.name === 'Exact')!
  section = edit(exactField.id)
  await expect(section.locator('input')).toHaveValue(
    '123456789012345678901234567890.123456',
  )
  const bigField = schema.fields.find((field) => field.name === 'Big')!
  section = edit(bigField.id)
  await section.locator('input').fill('invalid')
  await save.click()
  await expect(section.getByRole('alert')).toBeVisible()
  expect(
    (await (await page.request.get(apiRowPath)).json()).fields[bigField.id],
  ).toBe('9223372036854775807')
  await dialog.getByRole('button', { name: en.rowExpand.close }).click()

  await setLocale(page, 'zh-CN')
  await setTheme(page, 'dark')
  await page
    .getByRole('button', {
      name: zh.rowExpand.open.replace('{id}', row.id),
      exact: true,
    })
    .click()
  await expect(
    dialog.getByRole('heading', {
      name: 'Row 中文',
      exact: true,
    }),
  ).toBeVisible()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await dialog.getByRole('button', { name: zh.rowExpand.close }).click()
  await setLocale(page, 'en')
  await setTheme(page, 'light')

  // Direct links still authorize the table/view and fetch the full row.
  expectHTTPFailure(page, 'GET', `/api/v1/tables/${tid}/rows/0`, 404)
  await page.goto(`${base}/r/0`)
  await expect(dialog.getByRole('alert')).toContainText(en.errors.ERR_NOT_FOUND)
  await expect(dialog.locator('.row-field')).toHaveCount(0)
  await page.goto(
    `/t/${tid}/v/00000000-0000-0000-0000-000000000000/r/${row.id}`,
  )
  await expect(
    page.getByRole('heading', { name: en.nav.removedTitle, exact: true }),
  ).toBeVisible()
  await expect(dialog).toHaveCount(0)

  // Hold delivery after a real authorized HTTP read, even when abort is ignored.
  await page.goto(base)
  await holdRowResponse(page, apiRowPath)
  await openButton.click()
  await expect
    .poll(() =>
      page.evaluate(() => (window as HeldResponseWindow).rowResponseHeld),
    )
    .toBe(true)
  await dialog.getByRole('button', { name: en.rowExpand.close }).click()
  await expect(page).toHaveURL(new RegExp(`${base}$`))
  await page.evaluate(() =>
    (window as HeldResponseWindow).releaseRowResponse?.(),
  )
  await expect(dialog).toHaveCount(0)
  await expect(page.locator('revo-grid')).toBeVisible()

  await holdRowResponse(page, apiRowPath)
  await openButton.click()
  await expect
    .poll(() =>
      page.evaluate(() => (window as HeldResponseWindow).rowResponseHeld),
    )
    .toBe(true)
  await dialog.getByRole('button', { name: en.rowExpand.close }).click()
  await signOut(page)
  await expect(page).toHaveURL(/\/login$/)
  await page.evaluate(() =>
    (window as HeldResponseWindow).releaseRowResponse?.(),
  )
  await expect(dialog).toHaveCount(0)
  await login(page)

  const createdUser = await page.request.post('/api/v1/admin/users', {
    headers: h,
    data: {
      username: `row-reader-${suffix}`,
      displayName: 'Row reader',
      password: 'Temporary-password-123',
      role: 'user',
    },
  })
  expect(createdUser.status()).toBe(201)
  const user = (await createdUser.json()).user as {
    id: string
    username: string
  }
  const grant = await page.request.put('/api/v1/grants', {
    headers: h,
    data: {
      resourceType: 'table',
      resourceId: tid,
      principalType: 'user',
      principalId: user.id,
      level: 'read',
    },
  })
  expect(grant.status()).toBe(201)
  const readerContext = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL,
    locale: 'en-US',
    viewport: { width: 1280, height: 900 },
  })
  try {
    expect(
      (
        await readerContext.request.post('/api/v1/auth/login', {
          headers: h,
          data: { login: user.username, password: 'Temporary-password-123' },
        })
      ).status(),
    ).toBe(200)
    expect(
      (
        await readerContext.request.post('/api/v1/auth/password', {
          headers: h,
          data: {
            current: 'Temporary-password-123',
            new: 'Reader-password-123',
          },
        })
      ).status(),
    ).toBe(200)
    expect(
      (
        await readerContext.request.post('/api/v1/auth/login', {
          headers: h,
          data: { login: user.username, password: 'Reader-password-123' },
        })
      ).status(),
    ).toBe(200)
    const reader = await readerContext.newPage()
    const diagnostics = await observeBrowser(reader)
    await reader.goto(rowPath)
    const readerDialog = reader.locator('dialog.row-expand')
    await expect(readerDialog).toBeVisible()
    await expect(
      readerDialog.getByText(en.rowExpand.readOnly, { exact: true }),
    ).toBeVisible()
    await expect(
      readerDialog.locator('.row-field-value[role="button"]'),
    ).toHaveCount(0)
    expect(
      (
        await reader.request.patch(apiRowPath, {
          headers: h,
          data: { fields: { [nameField.id]: 'denied' } },
        })
      ).status(),
    ).toBe(403)

    // Permission changes are authoritative at save time, even with an open editor.
    const grantData = {
      resourceType: 'table',
      resourceId: tid,
      principalType: 'user',
      principalId: user.id,
    }
    expect(
      (
        await page.request.put('/api/v1/grants', {
          headers: h,
          data: { ...grantData, level: 'write' },
        })
      ).status(),
    ).toBe(200)
    await reader.reload()
    const readerName = readerDialog.locator(`[data-field-id="${nameField.id}"]`)
    await readerName.locator('input').fill('Writer saved')
    const readerSave = readerDialog.getByRole('button', {
      name: en.gridPolish.gp05.save,
      exact: true,
    })
    await readerSave.click()
    await expect(readerSave).toBeDisabled()
    expect(
      (await (await page.request.get(apiRowPath)).json()).fields[nameField.id],
    ).toBe('Writer saved')
    await readerName.locator('input').fill('must not save')
    expect(
      (
        await page.request.put('/api/v1/grants', {
          headers: h,
          data: { ...grantData, level: 'read' },
        })
      ).status(),
    ).toBe(200)
    await expect(
      readerDialog.getByText(en.rowExpand.readOnly, { exact: true }),
    ).toBeVisible()
    await expect(readerDialog.getByRole('alert')).toContainText(
      en.errors.ERR_PERMISSION_DENIED,
    )
    expect(
      (
        await reader.request.patch(apiRowPath, {
          headers: h,
          data: { fields: { [nameField.id]: 'must not save' } },
        })
      ).status(),
    ).toBe(403)
    await expect(
      readerDialog.getByText(en.rowExpand.saved, { exact: true }),
    ).toHaveCount(0)
    await readerDialog.getByRole('button', { name: en.rowExpand.close }).click()
    expect(
      (
        await page.request.delete(
          `/api/v1/grants/${(await grant.json()).grant.id}`,
          { headers: h },
        )
      ).status(),
    ).toBe(200)
    await reader.goto(rowPath)
    await expect(
      reader.getByRole('heading', { name: en.nav.removedTitle, exact: true }),
    ).toBeVisible()
    await expect(readerDialog).toHaveCount(0)
    expect((await reader.request.get(apiRowPath)).status()).toBe(404)
    expectBrowserClean(reader, diagnostics)
  } finally {
    await readerContext.close()
  }

  const removedField = schema.fields.find((field) => field.name === 'URL')!
  expect(
    (
      await page.request.delete(`/api/v1/fields/${removedField.id}`, {
        headers: { ...h, 'If-Match': String(removedField.version) },
      })
    ).status(),
  ).toBe(200)
  await page.goto(rowPath)
  await expect(dialog.locator('.row-field')).toHaveCount(16)
  await expect(
    dialog.locator(`[data-field-id="${removedField.id}"]`),
  ).toHaveCount(0)
  await dialog.getByRole('button', { name: en.rowExpand.close }).click()

  expect(
    (
      await page.request.delete(`/api/v1/tables/${tid}/rows`, {
        headers: h,
        data: { ids: [row.id] },
      })
    ).status(),
  ).toBe(200)
  expectHTTPFailure(page, 'GET', apiRowPath, 404)
  await page.goto(rowPath)
  await expect(dialog.getByRole('alert')).toContainText(en.errors.ERR_NOT_FOUND)
  await expect(dialog.locator('.row-field')).toHaveCount(0)
  await dialog.getByRole('button', { name: en.rowExpand.close }).click()
  expect(
    (
      await page.request.delete(`/api/v1/tables/${tid}`, { headers: h })
    ).status(),
  ).toBe(200)
  await page.goto(rowPath)
  await expect(
    page.getByRole('heading', { name: en.nav.removedTitle, exact: true }),
  ).toBeVisible()
  await expect(dialog).toHaveCount(0)
})
