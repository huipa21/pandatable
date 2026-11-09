import type { Locator, Page, Route } from '@playwright/test'
import type { Components } from '@revolist/revogrid'
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
  TableField,
  TableSchema,
} from '../src/api/types'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

async function create<T>(page: Page, path: string, data: unknown): Promise<T> {
  const response = await page.request.post(path, { headers: headers(), data })
  expect(response.status()).toBe(201)
  return (await response.json()) as T
}

async function gridCell(
  page: Page,
  fieldId: string,
  row: number,
): Promise<Locator> {
  const grid = page.locator('revo-grid')
  const position = await grid.evaluate(
    async (element, { fieldId, row }) => {
      const component = element as HTMLElement & Components.RevoGrid
      await component.scrollToRow(row)
      const column = (await component.getColumns()).find(
        (item) => item.prop === fieldId,
      )
      if (!column) throw new Error(`Missing grid column ${fieldId}`)
      const type = column.pin ?? 'rgCol'
      if (type === 'rgCol') await component.scrollToColumnProp(fieldId)
      await new Promise<void>((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
      )
      const store = await component.getColumnStore(type)
      const source = store.get('source')
      const x = store
        .get('items')
        .findIndex((index) => source[index]?.prop === fieldId)
      if (x < 0) throw new Error(`Grid column ${fieldId} is not visible`)
      return { type, x }
    },
    { fieldId, row },
  )
  const cell = grid.locator(
    `revogr-data[col-type="${position.type}"] .rgCell[data-rgrow="${row}"][data-rgcol="${position.x}"]`,
  )
  await expect(cell).toBeVisible()
  return cell
}

test('rapid Enter after a scalar click preserves the next edit', async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, 'Inline grid editing is a desktop interaction.')
  await page.setViewportSize({ width: 1440, height: 900 })
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

  const { workspace } = await create<{ workspace: { id: string } }>(
    page,
    '/api/v1/workspaces',
    { name: `Rapid grid edit ${Date.now()}` },
  )
  const { book } = await create<{ book: { id: string } }>(
    page,
    `/api/v1/workspaces/${workspace.id}/books`,
    { name: 'Rapid grid edit book' },
  )
  const schema = await create<TableSchema>(
    page,
    `/api/v1/books/${book.id}/tables`,
    { name: 'Rapid grid edit table' },
  )
  const { field } = await create<FieldMutation>(
    page,
    `/api/v1/tables/${schema.table.id}/fields`,
    { name: 'Moment', uiType: 'datetime' },
  )
  const view = schema.views[0]
  if (!field || !view) throw new Error('The field or view was not returned')
  const { rows } = await create<{ rows: DataRow[] }>(
    page,
    `/api/v1/tables/${schema.table.id}/rows`,
    { rows: [{ fields: {} }] },
  )
  const row = rows[0]
  if (!row) throw new Error('The row was not returned')

  await page.goto(`/t/${schema.table.id}/v/${view.id}`)
  const grid = page.locator('revo-grid')
  const cell = await gridCell(page, field.id, 0)
  const [gridBox, cellBox] = await Promise.all([
    grid.boundingBox(),
    cell.boundingBox(),
  ])
  if (!gridBox || !cellBox) throw new Error('The grid cell is not visible')
  await grid.click({
    position: {
      x: cellBox.x + cellBox.width / 2 - gridBox.x,
      y: cellBox.y + cellBox.height / 2 - gridBox.y,
    },
  })
  await page.keyboard.press('Enter')
  const editor = page.getByRole('textbox', {
    name: en.field.editor.value,
    exact: true,
  })
  await expect(editor).toBeFocused()
  await page.keyboard.type('2026-01-01T00:30:00Z')
  await expect(editor).toHaveValue('2026-01-01T00:30:00Z')
  await page.keyboard.press('Enter')
  await expect
    .poll(async () => {
      const response = await page.request.get(
        `/api/v1/tables/${schema.table.id}/rows/${row.id}`,
      )
      expect(response.status()).toBe(200)
      return ((await response.json()) as DataRow).fields[field.id]
    })
    .toBe('2026-01-01T00:30:00Z')
})

test('scalar grid editing commits on leave and retains rejected drafts', async ({
  page,
  browser,
  isMobile,
}) => {
  test.skip(isMobile, 'Inline grid editing is a desktop interaction.')
  test.setTimeout(240_000)
  page.setDefaultTimeout(10_000)
  await page.setViewportSize({ width: 1440, height: 900 })
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

  const suffix = Date.now()
  const { workspace } = await create<{ workspace: { id: string } }>(
    page,
    '/api/v1/workspaces',
    { name: `Inline editing ${suffix}` },
  )
  const { book } = await create<{ book: { id: string } }>(
    page,
    `/api/v1/workspaces/${workspace.id}/books`,
    { name: 'Inline editing book' },
  )
  const initial = await create<TableSchema>(
    page,
    `/api/v1/books/${book.id}/tables`,
    { name: 'Inline editing table' },
  )
  const tableId = initial.table.id
  const fields = new Map<string, TableField>()
  for (const data of [
    { name: 'Note', uiType: 'text' },
    { name: 'Big', uiType: 'integer', options: { size: 'big' } },
    {
      name: 'Exact',
      uiType: 'decimal',
      options: { precision: 38, scale: 6 },
    },
    { name: 'Email', uiType: 'email' },
    { name: 'Notes', uiType: 'long_text' },
    { name: 'Website', uiType: 'url' },
    { name: 'Moment', uiType: 'datetime' },
    { name: 'Code', uiType: 'text', isRequired: true, isUnique: true },
  ]) {
    const result = await create<FieldMutation>(
      page,
      `/api/v1/tables/${tableId}/fields`,
      data,
    )
    if (!result.field) throw new Error(`Field ${data.name} was not returned`)
    fields.set(data.name, result.field)
  }
  const field = (name: string) => {
    const item = fields.get(name)
    if (!item) throw new Error(`Missing field ${name}`)
    return item.id
  }
  const schemaResponse = await page.request.get(
    `/api/v1/tables/${tableId}/schema`,
  )
  expect(schemaResponse.status()).toBe(200)
  const schema = (await schemaResponse.json()) as TableSchema
  const name = schema.table.displayFieldId
  const system = schema.fields.find((item) => item.uiType === 'id')
  const view = schema.views[0]
  if (!name || !system || !view)
    throw new Error('The table is missing its display field, system ID or view')
  const systemColumn = view.config.fields.find((item) => item.id === system.id)
  if (!systemColumn) throw new Error('The system ID column is not configured')
  const configured = await page.request.patch(`/api/v1/views/${view.id}`, {
    headers: { ...headers(), 'If-Match': String(view.version) },
    data: {
      config: {
        ...view.config,
        frozen: 0,
        fields: [
          ...view.config.fields.filter((item) => item.id !== system.id),
          { ...systemColumn, visible: true },
        ],
      },
    },
  })
  expect(configured.status()).toBe(200)
  const inserted = await create<{ rows: DataRow[] }>(
    page,
    `/api/v1/tables/${tableId}/rows`,
    {
      rows: [
        {
          fields: {
            [name]: 'Row one',
            [field('Note')]: 'First note',
            [field('Big')]: '5',
            [field('Exact')]: '1.000000',
            [field('Email')]: 'first@example.com',
            [field('Notes')]: 'Initial text',
            [field('Website')]: 'https://example.com',
            [field('Code')]: 'alpha',
          },
        },
        {
          fields: {
            [name]: 'Row two',
            [field('Note')]: 'Second note',
            [field('Big')]: '6',
            [field('Code')]: 'bravo',
          },
        },
      ],
    },
  )
  const first = inserted.rows[0]
  if (!first) throw new Error('The first row was not returned')
  const rowPath = `/api/v1/tables/${tableId}/rows/${first.id}`
  const base = `/t/${tableId}/v/${view.id}`
  const value = async (fieldId: string) => {
    const response = await page.request.get(rowPath)
    expect(response.status()).toBe(200)
    return ((await response.json()) as DataRow).fields[fieldId]
  }
  const patches: { fields: Record<string, unknown> }[] = []
  page.on('request', (request) => {
    if (
      request.method() === 'PATCH' &&
      new URL(request.url()).pathname === rowPath
    )
      patches.push(
        request.postDataJSON() as { fields: Record<string, unknown> },
      )
  })
  const nextPatch = () =>
    page.waitForResponse(
      (response) =>
        response.request().method() === 'PATCH' &&
        new URL(response.url()).pathname === rowPath,
    )

  await page.goto(base)
  const grid = page.locator('revo-grid')
  const editor = page.getByRole('textbox', {
    name: en.field.editor.value,
    exact: true,
  })
  const outside = page.locator('.grid-toolbar')
  const leave = () => outside.click({ position: { x: 2, y: 2 } })
  await expect(grid).toBeVisible()
  await expect(editor).toHaveCount(0)

  const noteCell = await gridCell(page, field('Note'), 0)
  const nameCell = await gridCell(page, name, 0)
  await expect(nameCell).toContainText('Row one')
  await nameCell.click()
  await expect(editor).toHaveValue('Row one')
  await editor.fill('Single click')
  const nameSaved = nextPatch()
  await noteCell.click()
  expect((await nameSaved).status()).toBe(200)
  await expect.poll(() => value(name)).toBe('Single click')
  await expect(editor).toHaveValue('First note')
  expect(patches).toHaveLength(1)

  await editor.fill('Tabbed note')
  const tabSaved = nextPatch()
  await editor.press('Tab')
  expect((await tabSaved).status()).toBe(200)
  await expect.poll(() => value(field('Note'))).toBe('Tabbed note')
  await expect
    .poll(() =>
      grid.evaluate(async (element) => {
        const focused = await (
          element as HTMLElement & Components.RevoGrid
        ).getFocused()
        return focused?.column?.prop
      }),
    )
    .toBe(field('Big'))
  expect(patches).toHaveLength(2)

  const bigCell = await gridCell(page, field('Big'), 0)
  await bigCell.click()
  await editor.fill('discarded')
  await editor.press('Escape')
  await expect(editor).toHaveCount(0)
  expect(patches).toHaveLength(2)
  expect(await value(field('Big'))).toBe('5')

  await bigCell.click()
  await editor.fill('not an integer')
  await leave()
  await expect(bigCell).toContainText(en.field.error.invalid_integer)
  await expect(bigCell.locator('.grid-cell-draft')).toHaveText('not an integer')
  expect(patches).toHaveLength(2)
  await bigCell.click()
  await expect(editor).toHaveValue('not an integer')
  await editor.fill('9223372036854775807')
  const bigSaved = nextPatch()
  await editor.press('Enter')
  expect((await bigSaved).status()).toBe(200)
  await expect.poll(() => value(field('Big'))).toBe('9223372036854775807')
  expect(patches).toHaveLength(3)

  const exact = '123456789012345678901234567890.123456'
  await (await gridCell(page, field('Exact'), 0)).click()
  await editor.fill(exact)
  const exactSaved = nextPatch()
  await leave()
  expect((await exactSaved).status()).toBe(200)
  expect(patches.at(-1)?.fields[field('Exact')]).toBe(exact)
  await expect.poll(() => value(field('Exact'))).toBe(exact)

  const emailCell = await gridCell(page, field('Email'), 0)
  const beforeInvalidEmail = patches.length
  await emailCell.click({ position: { x: 4, y: 4 } })
  await expect(editor).toBeVisible()
  await editor.fill('invalid@')
  await leave()
  await expect(emailCell).toContainText(en.field.error.invalid_email)
  expect(patches).toHaveLength(beforeInvalidEmail)
  await emailCell.click()
  await expect(editor).toHaveValue('invalid@')
  await editor.fill('fixed@example.com')
  const emailSaved = nextPatch()
  await leave()
  expect((await emailSaved).status()).toBe(200)
  await expect.poll(() => value(field('Email'))).toBe('fixed@example.com')

  const notesCell = await gridCell(page, field('Notes'), 0)
  await notesCell.click()
  await editor.fill('one')
  const beforeMultiline = patches.length
  await editor.press('Enter')
  await page.keyboard.insertText('中文')
  await expect(editor).toHaveValue('one\n中文')
  expect(patches).toHaveLength(beforeMultiline)
  const notesSaved = nextPatch()
  await leave()
  expect((await notesSaved).status()).toBe(200)
  await expect.poll(() => value(field('Notes'))).toBe('one\n中文')

  await (await gridCell(page, field('Moment'), 0)).click()
  await editor.fill('2026-10-27T15:00:00.123456+08:00')
  const momentSaved = nextPatch()
  await leave()
  expect((await momentSaved).status()).toBe(200)
  expect(patches.at(-1)?.fields[field('Moment')]).toBe(
    '2026-10-27T07:00:00.123456Z',
  )
  await expect
    .poll(() => value(field('Moment')))
    .toBe('2026-10-27T07:00:00.123456Z')

  const codeCell = await gridCell(page, field('Code'), 0)
  await codeCell.click()
  await editor.fill('bravo')
  expectHTTPFailure(page, 'PATCH', rowPath, 409)
  const duplicate = nextPatch()
  await leave()
  expect((await duplicate).status()).toBe(409)
  await expect(codeCell).toContainText(en.errors.ERR_UNIQUE_CONFLICT)
  await expect(codeCell.locator('.grid-cell-draft')).toHaveText('bravo')
  expect(await value(field('Code'))).toBe('alpha')
  await codeCell.click()
  await expect(editor).toHaveValue('bravo')
  await editor.fill('')
  expectHTTPFailure(page, 'PATCH', rowPath, 400)
  const required = nextPatch()
  await leave()
  expect((await required).status()).toBe(400)
  await expect(codeCell).toContainText(en.errors.ERR_REQUIRED_FIELD)
  expect(await value(field('Code'))).toBe('alpha')
  await codeCell.click()
  await expect(editor).toHaveValue('')
  await editor.fill('charlie')
  const codeSaved = nextPatch()
  await leave()
  expect((await codeSaved).status()).toBe(200)
  await expect.poll(() => value(field('Code'))).toBe('charlie')

  await (await gridCell(page, name, 0)).dblclick()
  await expect(editor).toBeVisible()
  await editor.fill('Double click')
  const doubleSaved = nextPatch()
  await leave()
  expect((await doubleSaved).status()).toBe(200)
  await expect.poll(() => value(name)).toBe('Double click')

  await (await gridCell(page, name, 0)).click()
  await editor.fill('IME 中文')
  const beforeComposition = patches.length
  await editor.dispatchEvent('compositionstart', { data: '' })
  await editor.dispatchEvent('keydown', { key: 'Enter', isComposing: true })
  await leave()
  await expect(editor).toHaveValue('IME 中文')
  expect(patches).toHaveLength(beforeComposition)
  const compositionSaved = nextPatch()
  await editor.dispatchEvent('compositionend', { data: '中文' })
  expect((await compositionSaved).status()).toBe(200)
  await expect.poll(() => value(name)).toBe('IME 中文')

  let release!: () => void
  let held = false
  const hold = new Promise<void>((resolve) => {
    release = resolve
  })
  const pattern = `**${rowPath}`
  const holdFirst = async (route: Route) => {
    if (route.request().method() === 'PATCH' && !held) {
      held = true
      await hold
    }
    await route.continue()
  }
  await page.route(pattern, holdFirst)
  try {
    const start = patches.length
    const target = await gridCell(page, name, 0)
    await target.click()
    await editor.fill('Slow first')
    await leave()
    await expect.poll(() => held).toBe(true)
    await target.click()
    await expect(editor).toHaveValue('Slow first')
    await editor.fill('Slow second')
    await leave()
    expect(patches).toHaveLength(start + 1)
    release()
    await expect.poll(() => value(name)).toBe('Slow second')
    await expect(
      page.getByRole('status').filter({ hasText: en.grid.saving }),
    ).toHaveCount(0)
    expect(patches.slice(start).map((patch) => patch.fields[name])).toEqual([
      'Slow first',
      'Slow second',
    ])
  } finally {
    release()
    await page.unroute(pattern, holdFirst)
  }

  const undo = page.getByRole('button', { name: en.undo.undo, exact: true })
  const redo = page.getByRole('button', { name: en.undo.redo, exact: true })
  await expect(undo).toBeEnabled()
  await undo.click()
  await expect.poll(() => value(name)).toBe('Slow first')
  await expect(redo).toBeEnabled()
  await redo.click()
  await expect.poll(() => value(name)).toBe('Slow second')

  const rangeEndCell = await gridCell(page, field('Note'), 1)
  const rangeStartCell = await gridCell(page, name, 0)
  const rangeStart = await rangeStartCell.boundingBox()
  const rangeEnd = await rangeEndCell.boundingBox()
  if (!rangeStart || !rangeEnd) throw new Error('Range cells are not visible')
  await page.mouse.move(rangeStart.x + 20, rangeStart.y + 15)
  await page.mouse.down()
  await page.mouse.move(rangeEnd.x + 20, rangeEnd.y + 15, { steps: 8 })
  await page.mouse.up()
  await expect(editor).toHaveCount(0)
  await expect
    .poll(() =>
      grid.evaluate(async (element) => {
        const range = await (
          element as HTMLElement & Components.RevoGrid
        ).getSelectedRange()
        return range ? { y: range.y, y1: range.y1 } : null
      }),
    )
    .toEqual({ y: 0, y1: 1 })
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
  await grid.evaluate((element) => (element as HTMLElement).focus())
  await page.keyboard.press('Control+c')
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toContain('Slow second\tTabbed note')

  const website = await gridCell(page, field('Website'), 0)
  const link = website.locator('a.field-link')
  await link.evaluate((element) =>
    element.addEventListener('click', (event) => {
      event.preventDefault()
      element.setAttribute('data-test-clicked', 'true')
    }),
  )
  await link.click()
  await expect(link).toHaveAttribute('data-test-clicked', 'true')
  await expect(editor).toHaveCount(0)
  await (await gridCell(page, system.id, 0)).click()
  await page.keyboard.press('Enter')
  await expect(editor).toHaveCount(0)

  await page.reload()
  await expect(await gridCell(page, name, 0)).toContainText('Slow second')
  expect(await value(field('Big'))).toBe('9223372036854775807')
  expect(await value(field('Exact'))).toBe(exact)
  expect(await value(field('Notes'))).toBe('one\n中文')
  expect(await value(field('Moment'))).toBe('2026-10-27T07:00:00.123456Z')
  expect(await value(field('Code'))).toBe('charlie')

  const { user } = await create<{ user: { id: string; username: string } }>(
    page,
    '/api/v1/admin/users',
    {
      username: `inline-reader-${suffix}`,
      displayName: 'Inline reader',
      password: 'Temporary-password-123',
      role: 'user',
    },
  )
  const grant = await page.request.put('/api/v1/grants', {
    headers: headers(),
    data: {
      resourceType: 'table',
      resourceId: tableId,
      principalType: 'user',
      principalId: user.id,
      level: 'read',
    },
  })
  expect(grant.status()).toBe(201)
  const readerContext = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL,
    locale: 'en-US',
    viewport: { width: 1440, height: 900 },
  })
  try {
    await paceLogin(page)
    const login = await readerContext.request.post('/api/v1/auth/login', {
      headers: headers(),
      data: { login: user.username, password: 'Temporary-password-123' },
    })
    expect(login.status()).toBe(200)
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
    const readerPatches: string[] = []
    reader.on('request', (request) => {
      if (
        request.method() === 'PATCH' &&
        new URL(request.url()).pathname === rowPath
      )
        readerPatches.push(request.postData() ?? '')
    })
    await reader.goto(base)
    await (await gridCell(reader, name, 0)).dblclick()
    await reader.keyboard.press('Enter')
    await expect(
      reader.getByRole('textbox', { name: en.field.editor.value, exact: true }),
    ).toHaveCount(0)
    expect(readerPatches).toEqual([])
    expect(await value(name)).toBe('Slow second')
    expectBrowserClean(reader, diagnostics)
  } finally {
    await readerContext.close()
  }
})

test('choice cells toggle and pick with one click and commit once', async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, 'Inline grid editing is a desktop interaction.')
  test.setTimeout(180_000)
  page.setDefaultTimeout(10_000)
  await page.setViewportSize({ width: 1440, height: 700 })
  const diagnostics = await observeBrowser(page)
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

  const suffix = Date.now()
  const { workspace } = await create<{ workspace: { id: string } }>(
    page,
    '/api/v1/workspaces',
    { name: `Choice editing ${suffix}` },
  )
  const { book } = await create<{ book: { id: string } }>(
    page,
    `/api/v1/workspaces/${workspace.id}/books`,
    { name: 'Choice editing book' },
  )
  const initial = await create<TableSchema>(
    page,
    `/api/v1/books/${book.id}/tables`,
    { name: 'Choice editing table' },
  )
  const tableId = initial.table.id
  const choices = (labels: string[]) => ({
    choices: labels.map((label, index) => ({
      id: `c${index}`,
      label,
      color: 'gray',
    })),
  })
  const fieldIds = new Map<string, string>()
  for (const data of [
    { name: 'Done', uiType: 'checkbox' },
    {
      name: 'Status',
      uiType: 'single_select',
      options: choices(['Todo', 'Doing', 'Finished']),
    },
    {
      name: 'Tags',
      uiType: 'multi_select',
      options: choices(['Alpha', 'Beta', 'Gamma']),
    },
  ]) {
    const result = await create<FieldMutation>(
      page,
      `/api/v1/tables/${tableId}/fields`,
      data,
    )
    if (!result.field) throw new Error(`Field ${data.name} was not returned`)
    fieldIds.set(data.name, result.field.id)
  }
  const field = (name: string) => fieldIds.get(name)!
  const schemaResponse = await page.request.get(
    `/api/v1/tables/${tableId}/schema`,
  )
  const schema = (await schemaResponse.json()) as TableSchema
  const name = schema.table.displayFieldId!
  const view = schema.views[0]!
  const inserted = await create<{ rows: DataRow[] }>(
    page,
    `/api/v1/tables/${tableId}/rows`,
    {
      rows: Array.from({ length: 40 }, (_, index) => ({
        fields:
          index === 0
            ? {
                [name]: 'Row 0',
                [field('Status')]: 'Todo',
                [field('Tags')]: ['Alpha'],
              }
            : { [name]: `Row ${index}` },
      })),
    },
  )
  const rowPath = `/api/v1/tables/${tableId}/rows/${inserted.rows[0]!.id}`
  const value = async (fieldId: string) => {
    const response = await page.request.get(rowPath)
    expect(response.status()).toBe(200)
    return ((await response.json()) as DataRow).fields[fieldId]
  }
  const patches: Record<string, unknown>[] = []
  page.on('request', (request) => {
    if (request.method() === 'PATCH' && request.url().includes('/rows'))
      patches.push(
        (request.postDataJSON() as { fields: Record<string, unknown> }).fields,
      )
  })
  const nextPatch = () =>
    page.waitForResponse(
      (response) =>
        response.request().method() === 'PATCH' &&
        new URL(response.url()).pathname === rowPath,
    )
  const textbox = page.getByRole('textbox', {
    name: en.field.editor.value,
    exact: true,
  })
  const listbox = (label: string) =>
    page.getByRole('listbox', { name: label, exact: true })
  const option = (label: string, choice: string) =>
    listbox(label).locator(`[role="option"][data-choice="${choice}"]`)
  const leave = () =>
    page.locator('.grid-toolbar').click({ position: { x: 2, y: 2 } })

  await page.goto(`/t/${tableId}/v/${view.id}`)
  await expect(page.locator('revo-grid')).toBeVisible()

  // One click on the box toggles; a click elsewhere in the cell only selects.
  const doneCell = await gridCell(page, field('Done'), 0)
  await doneCell.click({ position: { x: 4, y: 4 } })
  await doneCell.locator('.field-check').waitFor()
  expect(patches).toEqual([])
  let saved = nextPatch()
  await doneCell.locator('.field-check').click()
  expect((await saved).status()).toBe(200)
  await expect.poll(() => value(field('Done'))).toBe(true)
  saved = nextPatch()
  await doneCell.locator('.field-check').click()
  expect((await saved).status()).toBe(200)
  await expect.poll(() => value(field('Done'))).toBe(false)
  expect(patches).toEqual([
    { [field('Done')]: true },
    { [field('Done')]: false },
  ])
  await expect(textbox).toHaveCount(0)
  await expect(page.getByRole('listbox')).toHaveCount(0)

  // One click opens the single-select list; picking commits and closes.
  await (await gridCell(page, field('Status'), 0)).click()
  await expect(listbox('Status')).toBeFocused()
  await expect(option('Status', 'Todo')).toHaveAttribute(
    'aria-selected',
    'true',
  )
  saved = nextPatch()
  await option('Status', 'Doing').click()
  expect((await saved).status()).toBe(200)
  await expect(listbox('Status')).toHaveCount(0)
  await expect.poll(() => value(field('Status'))).toBe('Doing')
  await (await gridCell(page, field('Status'), 0)).click()
  saved = nextPatch()
  await option('Status', '').click()
  expect((await saved).status()).toBe(200)
  await expect.poll(() => value(field('Status'))).toBe(null)
  expect(patches).toHaveLength(4)

  // Multi-select stays open for several changes and writes once on leave.
  await (await gridCell(page, field('Tags'), 0)).click()
  await expect(listbox('Tags')).toHaveAttribute('aria-multiselectable', 'true')
  await option('Tags', 'Gamma').click()
  await option('Tags', 'Alpha').click()
  await option('Tags', 'Beta').click()
  await expect(listbox('Tags')).toBeVisible()
  expect(patches).toHaveLength(4)
  saved = nextPatch()
  await leave()
  expect((await saved).status()).toBe(200)
  await expect(listbox('Tags')).toHaveCount(0)
  await expect.poll(() => value(field('Tags'))).toEqual(['Beta', 'Gamma'])
  expect(patches).toHaveLength(5)
  expect(patches[4]).toEqual({ [field('Tags')]: ['Beta', 'Gamma'] })

  // Escape cancels the open multi-select draft.
  await (await gridCell(page, field('Tags'), 0)).click()
  await option('Tags', 'Alpha').click()
  await page.keyboard.press('Escape')
  await expect(listbox('Tags')).toHaveCount(0)
  await leave()
  expect(patches).toHaveLength(5)
  expect(await value(field('Tags'))).toEqual(['Beta', 'Gamma'])

  await page.reload()
  const tagsCell = await gridCell(page, field('Tags'), 0)
  await expect(tagsCell).toContainText('Beta')
  await expect(tagsCell).toContainText('Gamma')
  await expect(tagsCell).not.toContainText('Alpha')
  await expect(await gridCell(page, field('Status'), 0)).not.toContainText(
    'Doing',
  )

  // A list opened on a bottom row stays inside the viewport.
  await (await gridCell(page, field('Status'), 39)).click()
  await expect(listbox('Status')).toBeVisible()
  const box = await page.locator('.grid-choice-popover').evaluate((element) => {
    const rect = element.getBoundingClientRect()
    return { top: rect.top, bottom: rect.bottom, height: innerHeight }
  })
  expect(box.top).toBeGreaterThanOrEqual(0)
  expect(box.bottom).toBeLessThanOrEqual(box.height)
  await expect(option('Status', 'Finished')).toBeInViewport()
  await page.keyboard.press('Escape')
  await expect(listbox('Status')).toHaveCount(0)
  expect(patches).toHaveLength(5)

  // zh-CN and dark theme: translated option text on dark theme surfaces.
  await setLocale(page, 'zh-CN')
  await setTheme(page, 'dark')
  try {
    await (await gridCell(page, field('Status'), 0)).click()
    await expect(listbox('Status')).toBeFocused()
    await expect(option('Status', '')).toHaveText(zh.field.editor.none)
    const luminance = await page
      .locator('.grid-choice-popover')
      .evaluate((element) => {
        const channels = (color: string) =>
          (color.match(/[\d.]+/g) ?? []).slice(0, 3).map(Number)
        const of = (color: string) => {
          const [r = 0, g = 0, b = 0] = channels(color)
          return (0.2126 * r + 0.7152 * g + 0.0722 * b) / 255
        }
        const style = getComputedStyle(element)
        return { bg: of(style.backgroundColor), fg: of(style.color) }
      })
    expect(luminance.bg).toBeLessThan(0.3)
    expect(luminance.fg).toBeGreaterThan(0.6)
    await page.keyboard.press('Escape')
    await expect(listbox('Status')).toHaveCount(0)
  } finally {
    await setTheme(page, 'light')
    await setLocale(page, 'en')
  }
  expect(patches).toHaveLength(5)
  expectBrowserClean(page, diagnostics)
})

test('first-click editing survives direct, sidebar and New Entry navigation', async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, 'Inline grid editing is a desktop interaction.')
  test.setTimeout(180_000)
  await page.setViewportSize({ width: 1440, height: 900 })
  const diagnostics = await observeBrowser(page)
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

  const { workspace } = await create<{ workspace: { id: string } }>(
    page,
    '/api/v1/workspaces',
    { name: `Edit activation ${Date.now()}` },
  )
  const { book } = await create<{ book: { id: string } }>(
    page,
    `/api/v1/workspaces/${workspace.id}/books`,
    { name: 'Edit activation book' },
  )
  const initialSchema = await create<TableSchema>(
    page,
    `/api/v1/books/${book.id}/tables`,
    { name: 'Edit activation table' },
  )
  const tableId = initialSchema.table.id
  const textResult = await create<FieldMutation>(
    page,
    `/api/v1/tables/${tableId}/fields`,
    { name: 'Note', uiType: 'text' },
  )
  const checkboxResult = await create<FieldMutation>(
    page,
    `/api/v1/tables/${tableId}/fields`,
    { name: 'Done', uiType: 'checkbox' },
  )
  if (!textResult.field || !checkboxResult.field)
    throw new Error('The editing fields were not returned')
  const schemaResponse = await page.request.get(
    `/api/v1/tables/${tableId}/schema`,
  )
  expect(schemaResponse.status()).toBe(200)
  const schema = (await schemaResponse.json()) as TableSchema
  const view = schema.views[0]
  if (!view) throw new Error('The grid view was not returned')
  const rowsPath = `/api/v1/tables/${tableId}/rows`
  const { rows } = await create<{ rows: DataRow[] }>(page, rowsPath, {
    rows: [
      {
        fields: {
          [textResult.field.id]: 'Before',
          [checkboxResult.field.id]: false,
        },
      },
    ],
  })
  const firstRow = rows[0]
  if (!firstRow) throw new Error('The initial row was not returned')

  const rowPath = (rowId: string) => `${rowsPath}/${rowId}`
  const rowPatches: string[] = []
  page.on('request', (request) => {
    const path = new URL(request.url()).pathname
    if (request.method() === 'PATCH' && path.startsWith(`${rowsPath}/`))
      rowPatches.push(path)
  })
  const value = async (rowId: string, fieldId: string) => {
    const response = await page.request.get(rowPath(rowId))
    expect(response.status()).toBe(200)
    return ((await response.json()) as DataRow).fields[fieldId]
  }
  const clickEditableTextAndCheckbox = async (
    rowIndex: number,
    rowId: string,
  ) => {
    const textPatchCount = rowPatches.length
    const cell = await gridCell(page, textResult.field!.id, rowIndex)
    await cell.click()
    const textbox = page.getByRole('textbox', {
      name: en.field.editor.value,
      exact: true,
    })
    await expect(textbox).toBeFocused()
    await page.keyboard.press('Escape')
    await expect(textbox).toHaveCount(0)
    expect(rowPatches).toHaveLength(textPatchCount)

    const checkboxCell = await gridCell(
      page,
      checkboxResult.field!.id,
      rowIndex,
    )
    const checkbox = checkboxCell.locator('.field-check')
    await expect(checkbox).toBeVisible()
    const expected = (await value(rowId, checkboxResult.field!.id)) !== true
    const patchCount = rowPatches.length
    const patch = page.waitForResponse(
      (response) =>
        response.request().method() === 'PATCH' &&
        new URL(response.url()).pathname === rowPath(rowId),
    )
    await checkbox.click()
    expect((await patch).status()).toBe(200)
    expect(rowPatches).toHaveLength(patchCount + 1)
    await expect
      .poll(() => value(rowId, checkboxResult.field!.id))
      .toBe(expected)
  }

  const viewPath = `/t/${tableId}/v/${view.id}`
  await page.goto(`/t/${tableId}`)
  await expect(page.locator('revo-grid')).toBeVisible()
  await expect.poll(() => new URL(page.url()).pathname).toBe(viewPath)
  await clickEditableTextAndCheckbox(0, firstRow.id)

  await page.goto(`/w/${workspace.id}/b/${book.id}`)
  const bookNode = page.locator(`[data-nav-id="${book.id}"]`)
  await expect(bookNode).toBeVisible()
  await bookNode.locator('button.book-toggle').click()
  const tableNode = page.locator(`[data-nav-id="${tableId}"]`)
  await expect(tableNode).toBeVisible()
  await tableNode.locator('button.table-toggle').click()
  await tableNode.locator(`[data-view-id="${view.id}"] a`).click()
  await expect(page).toHaveURL(new RegExp(`${viewPath}$`))
  await clickEditableTextAndCheckbox(0, firstRow.id)

  await page
    .locator('.grid-toolbar')
    .getByRole('button', {
      name: en.workspaceGrid.ux10.newEntry,
      exact: true,
    })
    .click()
  const dialog = page.locator('dialog.row-expand')
  await expect(dialog).toBeVisible()
  const createdResponse = page.waitForResponse(
    (response) =>
      response.request().method() === 'POST' &&
      new URL(response.url()).pathname === rowsPath,
  )
  await dialog
    .getByRole('button', { name: en.mobileRows.create, exact: true })
    .click()
  expect((await createdResponse).status()).toBe(201)
  await expect(page).toHaveURL(/\/r\/[1-9]\d*$/)
  const createdRowId = new URL(page.url()).pathname.split('/').at(-1)!
  await dialog
    .getByRole('button', { name: en.rowExpand.close, exact: true })
    .click()
  await expect(dialog).not.toBeVisible()
  await expect(page).toHaveURL(new RegExp(`${viewPath}$`))
  await clickEditableTextAndCheckbox(1, createdRowId)
  expectBrowserClean(page, diagnostics)
})
