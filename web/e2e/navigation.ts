import type { Browser, Page } from '@playwright/test'
import type { Components } from '@revolist/revogrid'
import {
  test,
  expect,
  observeBrowser,
  expectHTTPFailure,
  allowHTTPFailure,
  expectBrowserClean,
} from './fixtures'
import type {
  Navigation,
  RowQueryResult,
  TableSchema,
  ViewMutation,
  WorkspacePreference,
} from '../src/api/types'
import { adminLink, openNodeActions, setLocale, setTheme } from './app-shell'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }
import { verifySharing } from './sharing'
import { verifyFieldReader, verifyFields } from './fields'
import {
  observeViewQuerySchemaConflicts,
  openFieldPanel,
  sidebarView,
} from './table-controls'
import { formatTsv } from '../src/components/grid/tsv'

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

async function tree(page: Page, locale = en) {
  if (page.viewportSize()!.width < 768) {
    const drawer = page.locator('.nav-drawer')
    if (!(await drawer.isVisible()))
      await page
        .getByRole('button', { name: locale.nav.open, exact: true })
        .click()
    await expect(drawer).toBeVisible()
  }
  return page.getByRole('navigation', { name: locale.nav.title, exact: true })
}

async function closeDrawer(page: Page, locale = en) {
  if (
    page.viewportSize()!.width < 768 &&
    (await page.locator('.nav-drawer').isVisible())
  )
    await page
      .getByRole('button', { name: locale.nav.close, exact: true })
      .click()
}

export async function create(
  page: Page,
  kind: 'workspace' | 'book' | 'table',
  name: string,
  locale = en,
) {
  if (kind === 'workspace') {
    if (
      page.viewportSize()!.width >= 768 &&
      !(await page.locator('.sidebar-desktop .nav-tree').isVisible())
    )
      await page
        .getByRole('button', { name: locale.nav.expandSidebar, exact: true })
        .click()
    const nav = await tree(page, locale)
    await nav
      .locator('.workspace-heading')
      .getByRole('button', { name: locale.nav.create.workspace, exact: true })
      .click()
  } else {
    await page
      .locator('main')
      .getByRole('button', { name: locale.nav.create[kind], exact: true })
      .click()
  }
  const before = page.url()
  const dialog = page.getByRole('dialog', {
    name: locale.nav.dialogTitle,
    exact: true,
  })
  await expect(dialog).toBeVisible()
  await dialog
    .getByRole('textbox', { name: locale.nav.name, exact: true })
    .fill(name)
  await dialog
    .getByRole('button', { name: locale.admin.confirm, exact: true })
    .click()
  await expect(dialog).not.toBeVisible()
  const target =
    kind === 'workspace'
      ? /\/w\/[^/]+$/
      : kind === 'book'
        ? /\/w\/[^/]+\/b\/[^/]+$/
        : /\/t\/[^/]+\/v\/[^/]+$/
  // Workspace creation can start from another stable /w/:id route.
  await expect(page).toHaveURL(
    (url) => url.href !== before && target.test(url.pathname),
  )
}

async function startRename(page: Page, id: string, name: string, locale = en) {
  const nav = await tree(page, locale)
  const actions = nav.locator(`[data-nav-id="${id}"] > .node-actions`)
  await openNodeActions(actions)
  await actions
    .getByRole('button', { name: locale.nav.rename, exact: true })
    .click()
  const dialog = page.getByRole('dialog', {
    name: locale.nav.dialogTitle,
    exact: true,
  })
  await dialog
    .getByRole('textbox', { name: locale.nav.name, exact: true })
    .fill(name)
  return dialog
}

async function rename(page: Page, id: string, name: string, locale = en) {
  const dialog = await startRename(page, id, name, locale)
  await dialog
    .getByRole('button', { name: locale.admin.confirm, exact: true })
    .click()
  await expect(dialog).not.toBeVisible()
  await closeDrawer(page, locale)
}

async function noOverflow(page: Page) {
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true)
}

export async function verifyDataGrid(owner: Page, tid: string, viewId: string) {
  const h = headers()
  const query = async (fieldIds?: string[]) => {
    const response = await owner.request.post(`/api/v1/views/${viewId}/query`, {
      headers: h,
      data: { offset: 0, limit: 100, includeCount: true, fieldIds },
    })
    expect(response.status()).toBe(200)
    return (await response.json()) as RowQueryResult
  }
  const panel = await openFieldPanel(owner)
  await panel
    .getByRole('button', { name: en.field.panel.add, exact: true })
    .click()
  const fieldDialog = owner.getByRole('dialog', {
    name: en.field.panel.createTitle,
    exact: true,
  })
  await fieldDialog
    .getByRole('textbox', { name: en.field.panel.name, exact: true })
    .fill('Email')
  await fieldDialog.locator('[data-kind="email"]').click()
  await fieldDialog
    .getByRole('button', { name: en.field.panel.create, exact: true })
    .click()
  await expect(fieldDialog).not.toBeVisible()
  await expect(
    owner.getByRole('status').filter({ hasText: en.field.panel.saved }),
  ).toBeVisible()
  await panel
    .getByRole('button', { name: en.field.panel.add, exact: true })
    .click()
  const noteDialog = owner.getByRole('dialog', {
    name: en.field.panel.createTitle,
    exact: true,
  })
  await noteDialog
    .getByRole('textbox', { name: en.field.panel.name, exact: true })
    .fill('Note')
  await noteDialog.locator('[data-kind="long_text"]').click()
  await noteDialog
    .getByRole('button', { name: en.field.panel.create, exact: true })
    .click()
  await expect(noteDialog).not.toBeVisible()
  await owner.locator('.grid-fields-trigger').click()
  const schemaResponse = await owner.request.get(`/api/v1/tables/${tid}/schema`)
  expect(schemaResponse.status()).toBe(200)
  const schema = (await schemaResponse.json()) as TableSchema
  const name = schema.fields.find((field) => field.name === 'Name')!
  const qty = schema.fields.find((field) => field.name === '数量')!
  const email = schema.fields.find((field) => field.name === 'Email')!
  const updatedBy = schema.fields.find(
    (field) => field.uiType === 'updated_by',
  )!

  const gridRegion = owner.getByRole('region', { name: en.grid.title })
  await expect(gridRegion).toBeVisible()
  await expect(
    owner.getByRole('button', { name: en.grid.addRow }),
  ).toBeEnabled()
  await owner.getByRole('button', { name: en.grid.addRow }).click()
  await expect.poll(async () => (await query()).total).toBe(1)
  let result = await query()
  expect(result.rows[0]?.fields[qty.id]).toBe(7)
  expect(result.fieldIds).toContain(name.id)
  expect(result.fieldIds).toContain(qty.id)
  expect(result.fieldIds).toContain(email.id)
  await expect(owner.getByText('7', { exact: true })).toBeVisible()

  const gridElement = owner.locator('revo-grid')
  const openEditor = async (fieldId: string, rowIndex = 0) => {
    const targetRow = (await query()).rows[rowIndex]
    const field = schema.fields.find((item) => item.id === fieldId)
    expect(targetRow).toBeDefined()
    expect(field).toBeDefined()
    await gridElement.evaluate(
      async (element, { row, fid }) => {
        const grid = element as HTMLElement & Components.RevoGrid
        const cols = await grid.getColumns()
        const colIndex = cols.findIndex((c) => c.prop === fid)
        if (colIndex < 0) throw new Error(`Column not found for ${fid}`)
        const column = cols[colIndex]!
        const pin = (column.pin ?? 'rgCol') as
          'rgCol' | 'colPinStart' | 'colPinEnd'
        const store = await grid.getColumnStore(pin)
        const source = store.get('source')
        const items = store.get('items')
        const x = items.findIndex((index) => source[index]?.prop === fid)
        const safeX = Math.max(0, x)
        await grid.setCellsFocus(
          { x: safeX, y: row },
          { x: safeX, y: row },
          pin,
        )
        grid.focus()
        await grid.setCellEdit(row, column.prop)
      },
      { row: rowIndex, fid: fieldId },
    )
    const ed = owner.getByRole('textbox', {
      name: en.field.editor.value,
      exact: true,
    })
    await expect(ed).toBeVisible()
  }
  const edit = async (fieldId: string, value: string) => {
    await openEditor(fieldId)
    const editor = owner.getByRole('textbox', {
      name: en.field.editor.value,
      exact: true,
    })
    await expect(editor).toBeVisible()
    await editor.fill(value)
    await editor.press('Enter')
    return editor
  }

  expectHTTPFailure(owner, 'POST', `/api/v1/tables/${tid}/rows`, 409)
  await owner.getByRole('button', { name: en.grid.addRow }).click()
  await expect(owner.getByRole('alert')).toContainText(
    en.errors.ERR_UNIQUE_CONFLICT,
  )
  expect((await query()).total).toBe(1)

  await openEditor(qty.id)
  let editor = owner.getByRole('textbox', {
    name: en.field.editor.value,
    exact: true,
  })
  await editor.fill('abc')
  await editor.press('Enter')
  await expect(
    owner.getByText(en.field.error.invalid_integer, { exact: true }),
  ).toBeVisible()
  expect((await query()).rows[0]?.fields[qty.id]).toBe(7)
  await editor.fill('8')
  await editor.press('Enter')
  await expect.poll(async () => (await query()).rows[0]?.fields[qty.id]).toBe(8)

  await owner.getByRole('button', { name: en.grid.addRow }).click()
  await expect.poll(async () => (await query()).total).toBe(2)
  result = await query()
  expect(result.rows[1]?.fields[qty.id]).toBe(7)
  await openEditor(qty.id, 1)
  editor = owner.getByRole('textbox', {
    name: en.field.editor.value,
    exact: true,
  })
  await editor.fill('9')
  await editor.press('Enter')
  await expect.poll(async () => (await query()).rows[1]?.fields[qty.id]).toBe(9)
  await owner.getByRole('button', { name: en.grid.addRow }).click()
  await expect.poll(async () => (await query()).total).toBe(3)

  await openEditor(email.id)
  editor = owner.getByRole('textbox', {
    name: en.field.editor.value,
    exact: true,
  })
  await editor.fill('x@')
  await editor.press('Enter')
  await expect(
    owner.getByText(en.field.error.invalid_email, { exact: true }),
  ).toBeVisible()
  expect((await query()).rows[0]?.fields[email.id]).toBeNull()
  await editor.fill('grid@example.test')
  await editor.press('Enter')
  await expect
    .poll(async () => (await query()).rows[0]?.fields[email.id])
    .toBe('grid@example.test')

  await edit(name.id, 'Persisted from the grid')
  await expect
    .poll(async () => (await query()).rows[0]?.fields[name.id])
    .toBe('Persisted from the grid')
  result = await query()
  expect(result.rows[0]?.fields[name.id]).toBe('Persisted from the grid')
  const me = (await (await owner.request.get('/api/v1/auth/me')).json()) as {
    id: string
  }

  const fieldsMenu = await openFieldPanel(owner)
  await fieldsMenu
    .getByRole('button', { name: en.field.panel.systemFields })
    .click()
  await fieldsMenu
    .getByRole('checkbox', {
      name: en.field.panel.showField.replace('{name}', updatedBy.name),
      exact: true,
    })
    .check()
  const updatedByHeader = owner
    .locator('revo-grid')
    .getByRole('columnheader', { name: updatedBy.name, exact: true })
  await expect(updatedByHeader).toBeVisible()
  await updatedByHeader.scrollIntoViewIfNeeded()
  result = await query([...result.fieldIds, updatedBy.id])
  expect(result.fieldIds).toContain(updatedBy.id)
  expect(result.rows[0]?.fields[updatedBy.id]).toBe(me.id)

  await expect
    .poll(async () => {
      const response = await owner.request.get(`/api/v1/tables/${tid}/schema`)
      expect(response.status()).toBe(200)
      const current = (await response.json()) as TableSchema
      return current.views
        .find((view) => view.id === viewId)
        ?.config.fields.find((field) => field.id === updatedBy.id)?.visible
    })
    .toBe(true)
  await owner.reload()
  await expect(owner.getByRole('region', { name: en.grid.title })).toBeVisible()
  await expect
    .poll(() =>
      gridElement.evaluate(
        async (element, id) =>
          (
            await (element as HTMLElement & Components.RevoGrid).getColumns()
          ).some((column) => column.prop === id),
        updatedBy.id,
      ),
    )
    .toBe(true)
  await expect
    .poll(async () => (await query()).rows[0]?.fields[name.id])
    .toBe('Persisted from the grid')

  await setTheme(owner, 'dark')
  await expect(owner.locator('html')).toHaveAttribute('data-theme', 'dark')
  await setLocale(owner, 'zh-CN')
  await expect(
    owner.getByRole('button', { name: zh.grid.addRow, exact: true }),
  ).toBeVisible()
  await setLocale(owner, 'en')
  await setTheme(owner, 'light')
  await expect(owner.locator('html')).toHaveAttribute('data-theme', 'light')

  const dateResponse = await owner.request.post(
    `/api/v1/tables/${tid}/fields`,
    {
      headers: h,
      data: { name: 'Date', uiType: 'date' },
    },
  )
  expect(dateResponse.status()).toBe(201)
  const stageResponse = await owner.request.post(
    `/api/v1/tables/${tid}/fields`,
    {
      headers: h,
      data: {
        name: 'Stage',
        uiType: 'single_select',
        options: {
          choices: [
            { id: 'todo', label: 'Todo', color: 'gray' },
            { id: 'done', label: 'Done', color: 'green' },
          ],
        },
      },
    },
  )
  expect(stageResponse.status()).toBe(201)
  const finalSchema = (await (
    await owner.request.get(`/api/v1/tables/${tid}/schema`)
  ).json()) as TableSchema
  const clipboardView = finalSchema.views.find((view) => view.id === viewId)
  const note = finalSchema.fields.find((field) => field.name === 'Note')
  if (!clipboardView || !note) throw new Error('Clipboard schema is incomplete')
  const clipboardIds = [name.id, qty.id, email.id, note.id]
  const clipboardIdSet = new Set(clipboardIds)
  const columnsById = new Map(
    clipboardView.config.fields.map((column) => [column.id, column]),
  )
  const clipboardColumns = clipboardIds.map((id) => {
    const column = columnsById.get(id)
    return column ? { ...column, visible: true } : { id, visible: true }
  })
  const clipboardUpdate = await owner.request.patch(`/api/v1/views/${viewId}`, {
    headers: { ...h, 'If-Match': String(clipboardView.version) },
    data: {
      config: {
        ...clipboardView.config,
        fields: [
          ...clipboardColumns,
          ...clipboardView.config.fields.filter(
            (column) => !clipboardIdSet.has(column.id),
          ),
        ],
        frozen: 0,
      },
    },
  })
  expect(clipboardUpdate.status()).toBe(200)
  expect(
    ((await clipboardUpdate.json()) as ViewMutation).view.config.fields
      .slice(0, clipboardIds.length)
      .map((column) => column.id),
  ).toEqual(clipboardIds)
  const date = finalSchema.fields.find((field) => field.name === 'Date')!
  const stage = finalSchema.fields.find((field) => field.name === 'Stage')!
  result = await query()
  const seed = await owner.request.patch(`/api/v1/tables/${tid}/rows`, {
    headers: h,
    data: {
      rows: [
        {
          id: result.rows[0]!.id,
          fields: { [date.id]: '2026-10-27', [stage.id]: 'Todo' },
        },
      ],
    },
  })
  expect(seed.status()).toBe(200)
  await owner.reload()
  await expect(owner.getByRole('region', { name: en.grid.title })).toBeVisible()
  await owner.context().grantPermissions(['clipboard-read', 'clipboard-write'])

  const grid = owner.locator('revo-grid')
  const columnIndex = async (fieldId: string) =>
    grid.evaluate(async (element, prop) => {
      const store = await (
        element as HTMLElement & Components.RevoGrid
      ).getColumnStore('rgCol')
      const source = store.get('source')
      return store
        .get('items')
        .findIndex((index) => source[index]?.prop === prop)
    }, fieldId)
  const focusRange = async (
    start: { x: number; y: number },
    end: { x: number; y: number },
  ) => {
    await grid.evaluate(
      async (element, { from, to }) =>
        (element as HTMLElement & Components.RevoGrid).setCellsFocus(
          from,
          to,
          'rgCol',
          'rgRow',
        ),
      { from: start, to: end },
    )
    await expect
      .poll(async () =>
        grid.evaluate(async (element) =>
          (element as HTMLElement & Components.RevoGrid).getSelectedRange(),
        ),
      )
      .toMatchObject({
        x: start.x,
        y: start.y,
        x1: end.x,
        y1: end.y,
      })
  }
  const focusCell = async (fieldId: string, rowIndex: number) => {
    const column = await columnIndex(fieldId)
    expect(column).toBeGreaterThanOrEqual(0)
    await focusRange({ x: column, y: rowIndex }, { x: column, y: rowIndex })
    return column
  }
  const pasteRequestBodies: unknown[] = []
  const clipboardPatch = (request: import('@playwright/test').Request) => {
    if (
      request.method() === 'PATCH' &&
      new URL(request.url()).pathname === `/api/v1/tables/${tid}/rows`
    )
      pasteRequestBodies.push(request.postDataJSON())
  }
  owner.on('request', clipboardPatch)
  const pasteX = await focusCell(name.id, 0)
  expect(
    await Promise.all([name.id, qty.id, email.id, note.id].map(columnIndex)),
  ).toEqual([pasteX, pasteX + 1, pasteX + 2, pasteX + 3])
  await owner.evaluate(
    (text) => navigator.clipboard.writeText(text),
    formatTsv([
      ['中文', '42', 'first@example.test', 'first line\nsaid "yes"'],
      ['quoted "text"', 'abc', 'x@', '多行\n内容'],
      ['last', '10', 'last@example.test', 'tail'],
    ]),
  )
  await owner.keyboard.press('Control+v')
  await expect
    .poll(async () => (await query()).rows[0]?.fields[name.id])
    .toBe('中文')
  result = await query()
  expect(pasteRequestBodies).toHaveLength(1)
  const pasted = pasteRequestBodies[0] as {
    rows: { id: string; fields: Record<string, unknown> }[]
  }
  expect(pasted.rows).toHaveLength(3)
  expect(
    pasted.rows.reduce(
      (count, row) => count + Object.keys(row.fields).length,
      0,
    ),
  ).toBe(10)
  expect(result.rows.map((row) => row.fields[qty.id])).toEqual([42, 9, 10])
  expect(result.rows[0]?.fields[note.id]).toBe('first line\nsaid "yes"')
  expect(result.rows[1]?.fields[email.id]).toBeNull()
  await expect(
    gridRegion
      .getByRole('alert')
      .filter({ hasText: 'Row 2, field 数量' })
      .first(),
  ).toBeVisible()
  await expect(
    gridRegion
      .getByRole('alert')
      .filter({ hasText: 'Row 2, field Email' })
      .first(),
  ).toBeVisible()

  await focusCell(name.id, 2)
  await owner.evaluate(
    (text) => navigator.clipboard.writeText(text),
    formatTsv(
      Array.from({ length: 10 }, (_, index) => [
        `paste row ${index}`,
        String(100 + index),
      ]),
    ),
  )
  await owner.keyboard.press('Control+v')
  const addRowsDialog = owner.getByRole('dialog', {
    name: en.grid.addRowsTitle,
    exact: true,
  })
  await expect(addRowsDialog).toContainText(
    en.grid.addRowsConfirm.replace('{count}', '9'),
  )
  await addRowsDialog
    .getByRole('button', {
      name: en.grid.addRowsAction.replace('{count}', '9'),
      exact: true,
    })
    .click()
  await expect.poll(async () => (await query()).total).toBe(12)
  result = await query()
  expect(result.rows[2]?.fields[qty.id]).toBe(100)
  expect(result.rows[11]?.fields[qty.id]).toBe(109)
  owner.off('request', clipboardPatch)

  await setTheme(owner, 'dark')
  await setLocale(owner, 'zh-CN')
  await expect(owner.locator('html')).toHaveAttribute('data-theme', 'dark')
  const dateColumn = await columnIndex(date.id)
  const stageColumn = await columnIndex(stage.id)
  expect(stageColumn).toBe(dateColumn + 1)
  await focusRange({ x: dateColumn, y: 0 }, { x: stageColumn, y: 0 })
  await owner.keyboard.press('Control+c')
  await expect
    .poll(() => owner.evaluate(() => navigator.clipboard.readText()))
    .toBe('2026-10-27\tTodo')
  await focusCell(stage.id, 0)
  await owner.keyboard.press('Control+x')
  await expect
    .poll(async () => (await query()).rows[0]?.fields[stage.id])
    .toBeNull()

  const nameColumn = await columnIndex(name.id)
  await focusRange({ x: nameColumn, y: 0 }, { x: nameColumn, y: 5 })
  await owner.keyboard.press('Control+d')
  await expect
    .poll(async () =>
      (await query()).rows
        .slice(0, 6)
        .every((row) => row.fields[name.id] === '中文'),
    )
    .toBe(true)

  await focusCell(name.id, 0)
  const handle = grid.locator(
    'revogr-overlay-selection[coltype="rgCol"][type="rgRow"] .autofill-handle',
  )
  await expect(handle).toBeVisible()
  const from = await handle.boundingBox()
  const to = await grid
    .locator(
      `revogr-data[col-type="rgCol"] .rgCell[data-rgrow="3"][data-rgcol="${nameColumn}"]`,
    )
    .boundingBox()
  expect(from).not.toBeNull()
  expect(to).not.toBeNull()
  await owner.mouse.move(from!.x + from!.width / 2, from!.y + from!.height / 2)
  await owner.mouse.down()
  await owner.mouse.move(to!.x + to!.width / 2, to!.y + to!.height / 2, {
    steps: 12,
  })
  await owner.mouse.up()
  await expect
    .poll(async () =>
      (await query()).rows
        .slice(0, 4)
        .every((row) => row.fields[name.id] === '中文'),
    )
    .toBe(true)

  await setLocale(owner, 'en')
  await setTheme(owner, 'light')
}

export async function verifyNavigation(
  admin: Page,
  alice: Page,
  browser: Browser,
  participants: { id: string; username: string }[],
  aliceUsername: string,
) {
  const h = headers()
  const aliceMe = await (await alice.request.get('/api/v1/auth/me')).json()
  expect(
    (
      await alice.request.patch('/api/v1/auth/me', {
        headers: h,
        data: { locale: 'en' },
      })
    ).status(),
  ).toBe(200)
  await alice.goto('/home')
  const starter = (await (
    await alice.request.get('/api/v1/auth/workspace-preference')
  ).json()) as WorkspacePreference
  expect(starter.workspaceId).toBeTruthy()
  await expect(alice).toHaveURL(new RegExp(`/w/${starter.workspaceId}$`))
  await expect(alice.locator('main h1')).toHaveText('Default')
  await create(alice, 'workspace', 'Workspace W')
  const wid = new URL(alice.url()).pathname.split('/')[2]!
  try {
    await expect(alice.locator('main')).toContainText(en.nav.emptyBooks)
    await create(alice, 'book', 'Book B')
    const bid = new URL(alice.url()).pathname.split('/')[4]!
    await expect(alice.locator('main')).toContainText(en.nav.emptyTables)
    await create(alice, 'table', 'Table T')
    await expect(alice.locator('.schema-summary')).toContainText('Name')
    const tid = new URL(alice.url()).pathname.split('/')[2]!
    const viewId = new URL(alice.url()).pathname.split('/')[4]!
    await expect(await sidebarView(alice, tid, 'Grid')).toHaveAttribute(
      'href',
      `/t/${tid}/v/${viewId}`,
    )
    await closeDrawer(alice)
    const schemaResponse = await alice.request.get(
      `/api/v1/tables/${tid}/schema`,
    )
    expect(schemaResponse.status()).toBe(200)
    const schema = (await schemaResponse.json()) as TableSchema
    expect(schema.myLevel).toBe('owner')
    expect(schema.fields.filter((field) => field.isSystem)).toHaveLength(5)
    expect(
      schema.fields.find((field) => field.id === schema.table.displayFieldId),
    ).toMatchObject({ name: 'Name', uiType: 'text' })
    expect(schema.views).toEqual([
      expect.objectContaining({
        id: viewId,
        name: 'Grid',
        type: 'grid',
        scope: 'shared',
      }),
    ])
    // Field-driven view saves can race a grid query under snapshot isolation.
    const checkQueryConflicts = observeViewQuerySchemaConflicts(alice, viewId)
    const fieldFixture = await verifyFields(alice, tid)
    const nav = await tree(alice)
    await expect(nav.locator('[data-nav-id]')).toHaveCount(4)
    await expect(
      nav.locator(`[data-nav-id="${starter.workspaceId}"]`),
    ).toHaveCount(1)
    await expect(
      nav.locator(
        `[data-nav-id="${wid}"] [data-nav-id="${bid}"] [data-nav-id="${tid}"]`,
      ),
    ).toBeVisible()
    await expect(nav.locator('.book-toggle')).toHaveAttribute(
      'aria-expanded',
      'true',
    )
    if (alice.viewportSize()!.width < 768) {
      await alice.keyboard.press('Escape')
      await expect(alice.locator('.nav-drawer')).not.toBeVisible()
      await expect(
        alice.getByRole('button', { name: en.nav.open, exact: true }),
      ).toBeFocused()
    }
    await closeDrawer(alice)

    const siblingBookResponse = await alice.request.post(
      `/api/v1/workspaces/${wid}/books`,
      { headers: h, data: { name: 'Hidden sibling book' } },
    )
    expect(siblingBookResponse.status()).toBe(201)
    const siblingBook = (await siblingBookResponse.json()).book.id as string
    const siblingTableResponse = await alice.request.post(
      `/api/v1/books/${bid}/tables`,
      { headers: h, data: { name: 'Second table' } },
    )
    expect(siblingTableResponse.status()).toBe(201)
    const siblingTable = (await siblingTableResponse.json()).table.id as string
    await alice.reload()
    let ownerTree = await tree(alice)
    const tableActions = ownerTree.locator(
      `[data-nav-id="${tid}"] > .node-actions`,
    )
    await openNodeActions(tableActions)
    await tableActions
      .getByRole('button', { name: en.nav.moveDown, exact: true })
      .click()
    await expect
      .poll(async () => {
        const value = (await (
          await alice.request.get('/api/v1/nav')
        ).json()) as Navigation
        return value.workspaces
          .find((w) => w.id === wid)!
          .books.find((b) => b.id === bid)!
          .tables.map((item) => item.id)
      })
      .toEqual([siblingTable, tid])
    if (alice.viewportSize()!.width >= 768) {
      // The API poll can observe the new order before the client refresh
      // finishes; drag only once the rendered tree is idle and reordered.
      await expect(
        tableActions.getByRole('button', { name: en.nav.moveUp, exact: true }),
      ).toBeEnabled()
      await expect(
        ownerTree.locator(`[data-nav-id="${tid}"] > .nav-row`),
      ).toHaveAttribute('draggable', 'true')
      await ownerTree
        .locator(`[data-nav-id="${tid}"] > .nav-row`)
        .dragTo(ownerTree.locator(`[data-nav-id="${siblingTable}"] > .nav-row`))
    } else {
      await tableActions
        .getByRole('button', { name: en.nav.moveUp, exact: true })
        .tap()
    }
    await expect
      .poll(async () => {
        const value = (await (
          await alice.request.get('/api/v1/nav')
        ).json()) as Navigation
        return value.workspaces
          .find((w) => w.id === wid)!
          .books.find((b) => b.id === bid)!
          .tables.map((item) => item.id)
      })
      .toEqual([tid, siblingTable])
    await closeDrawer(alice)
    await rename(alice, wid, '工作区 W')
    await rename(alice, bid, '工作簿 B')
    await setLocale(alice, 'zh-CN')
    await rename(alice, tid, '数据表 T', zh)
    await expect(
      alice.getByRole('heading', { name: '数据表 T', exact: true }),
    ).toBeVisible()
    await expect(alice.locator('.breadcrumbs')).toContainText('工作区 W')
    await noOverflow(alice)
    await alice.screenshot({
      path: test.info().outputPath('navigation-zh.png'),
      fullPage: true,
    })
    await setTheme(alice, 'dark')
    await expect(alice.locator('html')).toHaveAttribute('data-theme', 'dark')
    await noOverflow(alice)
    await setTheme(alice, 'light')

    await admin.goto('/home')
    await expect(admin).toHaveURL(/\/w\/[^/]+$/)
    const adminTree = await tree(admin)
    await expect(adminTree.locator(`[data-nav-id="${wid}"]`)).toHaveCount(0)
    await closeDrawer(admin)
    const deniedTable = await admin.request.get(`/api/v1/tables/${tid}/schema`)
    expect(deniedTable.status()).toBe(404)
    expect(await deniedTable.json()).toMatchObject({
      error: { code: 'ERR_NOT_FOUND' },
    })
    await (await adminLink(admin, en.workspaces.title)).click()
    // Every account now owns a starter workspace (UX-01), so a long run can
    // push this workspace past the first inventory page; search for it.
    await admin
      .getByLabel(en.workspaces.searchHint, { exact: true })
      .fill('工作区 W')
    await admin
      .locator('main')
      .getByRole('button', { name: en.admin.search, exact: true })
      .click()
    const inventory = admin.locator(`[data-workspace="${wid}"]`)
    await expect(inventory).toContainText('工作区 W')
    await expect(inventory).toContainText(aliceUsername)
    await expect(inventory.locator('a,button')).toHaveCount(0)

    const contexts = []
    const participantPages: Page[] = []
    const participantDiagnostics: Awaited<ReturnType<typeof observeBrowser>>[] =
      []
    try {
      for (const [index, participant] of participants.entries()) {
        const context = await browser.newContext({
          baseURL: process.env.PT_E2E_BASE_URL,
          locale: 'en-US',
          viewport: alice.viewportSize(),
          isMobile: alice.viewportSize()!.width < 768,
          hasTouch: alice.viewportSize()!.width < 768,
        })
        contexts.push(context)
        const temporary = 'Temporary-password-123'
        const password = 'Navigation-password-123'
        expect(
          (
            await context.request.post('/api/v1/auth/login', {
              headers: h,
              data: { login: participant.username, password: temporary },
            })
          ).status(),
        ).toBe(200)
        expect(
          (
            await context.request.post('/api/v1/auth/password', {
              headers: h,
              data: { current: temporary, new: password },
            })
          ).status(),
        ).toBe(200)
        expect(
          (
            await context.request.post('/api/v1/auth/login', {
              headers: h,
              data: { login: participant.username, password },
            })
          ).status(),
        ).toBe(200)
        const grant = await alice.request.put('/api/v1/grants', {
          headers: h,
          data: {
            resourceType: index === 0 ? 'workspace' : 'table',
            resourceId: index === 0 ? wid : tid,
            principalType: 'user',
            principalId: participant.id,
            level: 'read',
          },
        })
        expect(grant.status()).toBe(201)
        const page = await context.newPage()
        participantPages.push(page)
        const diagnostics = await observeBrowser(page)
        participantDiagnostics.push(diagnostics)
        await page.goto(`/t/${tid}/v/${viewId}`)
        await setLocale(page, 'en')
        await expect(page.locator('.schema-summary')).toContainText(
          en.nav.level.read,
        )
        await verifyFieldReader(page, tid, fieldFixture.qty)
        const visibleTree = await tree(page)
        // Readers may choose a readable workspace as their own default only.
        await expect(
          visibleTree
            .locator(
              `[data-nav-id="${wid}"] > .node-actions > .nav-actions > button`,
            )
            .filter({ hasNotText: en.workspaceGrid.ux02.setDefault }),
        ).toHaveCount(0)
        if (index === 1) {
          // The admin-created participant also owns a starter workspace.
          await expect(visibleTree.locator('[data-nav-id]')).toHaveCount(4)
          await expect(
            visibleTree.locator('.path-only', { hasText: en.nav.pathOnly }),
          ).toHaveCount(2)
          await expect(
            visibleTree.locator(
              `[data-nav-id="${siblingTable}"], [data-nav-id="${siblingBook}"]`,
            ),
          ).toHaveCount(0)
          await visibleTree
            .locator(`[data-nav-id="${wid}"] > .workspace-box a`)
            .click()
          await expect(page.locator('main')).toContainText(en.nav.pathNotice)
          await expect(page.locator('main button')).toHaveCount(0)
          await page.goto('/home')
          await expect(page).toHaveURL(/\/w\/[^/]+$/)
          await create(page, 'workspace', 'Carol W2')
          const carolWid = new URL(page.url()).pathname.split('/')[2]!
          const carolNav = (await (
            await context.request.get('/api/v1/nav')
          ).json()) as Navigation
          expect(
            carolNav.workspaces.find((w) => w.id === carolWid),
          ).toMatchObject({ level: 'owner', pathOnly: false })
          expect(
            (
              await context.request.delete(`/api/v1/workspaces/${carolWid}`, {
                headers: h,
              })
            ).status(),
          ).toBe(200)
        }
        await closeDrawer(page)
        expect(
          (
            await context.request.patch(`/api/v1/tables/${tid}`, {
              headers: h,
              data: { name: 'Denied' },
            })
          ).status(),
        ).toBe(403)
        await noOverflow(page)
        expect(diagnostics.errors).toEqual([])
        expect(diagnostics.pendingFailures).toEqual([])
      }
      await verifySharing(
        admin,
        alice,
        participantPages[0]!,
        participantPages[1]!,
        { wid, bid, tid, siblingTable, siblingBook },
        participants,
        aliceUsername,
      )
      participantPages.forEach((page, index) =>
        expectBrowserClean(page, participantDiagnostics[index]!),
      )
    } finally {
      for (const context of contexts) await context.close()
    }

    ownerTree = await tree(alice, zh)
    const actions = ownerTree.locator(`[data-nav-id="${tid}"] > .node-actions`)
    await openNodeActions(actions)
    await actions
      .getByRole('button', { name: zh.nav.delete, exact: true })
      .click()
    const dialog = alice.getByRole('dialog', { name: zh.nav.dialogTitle })
    await expect(dialog).toContainText(
      zh.nav.deleteNotice.replace('{name}', '数据表 T'),
    )
    await dialog
      .getByRole('button', { name: zh.admin.cancel, exact: true })
      .click()
    await actions
      .getByRole('button', { name: zh.nav.delete, exact: true })
      .click()
    allowHTTPFailure(alice, 'GET', `/api/v1/tables/${tid}/schema`, 404)
    await dialog
      .getByRole('button', { name: zh.admin.confirm, exact: true })
      .click()
    await expect(dialog).not.toBeVisible()
    await expect(ownerTree.locator(`[data-nav-id="${tid}"]`)).toHaveCount(0)
    await closeDrawer(alice, zh)
    await expect(
      alice.getByRole('heading', { name: zh.nav.removedTitle }),
    ).toBeVisible()
    expect(
      (await alice.request.get(`/api/v1/tables/${tid}/schema`)).status(),
    ).toBe(404)
    await setLocale(alice, 'en')
    const staleRename = await startRename(alice, siblingTable, 'Gone')
    expectHTTPFailure(alice, 'PATCH', `/api/v1/tables/${siblingTable}`, 404)
    expect(
      (
        await alice.request.delete(`/api/v1/tables/${siblingTable}`, {
          headers: h,
        })
      ).status(),
    ).toBe(200)
    await staleRename
      .getByRole('button', { name: en.admin.confirm, exact: true })
      .click()
    await expect(staleRename.getByRole('alert')).toContainText(
      en.errors.ERR_NOT_FOUND,
    )
    await staleRename
      .getByRole('button', { name: en.admin.cancel, exact: true })
      .click()
    await closeDrawer(alice)
    await checkQueryConflicts()
  } finally {
    expect(
      (
        await alice.request.delete(`/api/v1/workspaces/${wid}`, { headers: h })
      ).status(),
    ).toBe(200)
    expect(
      (
        await alice.request.patch('/api/v1/auth/me', {
          headers: h,
          data: { locale: aliceMe.locale },
        })
      ).status(),
    ).toBe(200)
    await admin.goto('/admin/users')
    await admin
      .getByLabel(en.admin.searchHint, { exact: true })
      .fill(aliceUsername)
    await admin
      .locator('main')
      .getByRole('button', { name: en.admin.search, exact: true })
      .click()
  }
}
