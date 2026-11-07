import type { Page } from '@playwright/test'
import type { Components } from '@revolist/revogrid'
import { expect, expectHTTPFailure, paceLogin, test } from './fixtures'
import type { DataRow, TableSchema } from '../src/api/types'
import { setLocale } from './app-shell'
import { openFieldPanel } from './table-controls'
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

async function tables(page: Page, suffix: string) {
  const workspace = await page.request.post('/api/v1/workspaces', {
    headers: headers(),
    data: { name: `Undo ${suffix}` },
  })
  expect(workspace.status()).toBe(201)
  const workspaceId = (await workspace.json()).workspace.id as string
  const book = await page.request.post(
    `/api/v1/workspaces/${workspaceId}/books`,
    { headers: headers(), data: { name: 'Undo book' } },
  )
  expect(book.status()).toBe(201)
  const bookId = (await book.json()).book.id as string
  async function create(name: string) {
    const response = await page.request.post(`/api/v1/books/${bookId}/tables`, {
      headers: headers(),
      data: { name },
    })
    expect(response.status()).toBe(201)
    return (await response.json()) as TableSchema
  }
  return { source: await create('Rows'), target: await create('Projects') }
}

async function createRows(
  page: Page,
  schema: TableSchema,
  labels: string[],
): Promise<DataRow[]> {
  const fieldId = schema.table.displayFieldId
  if (!fieldId) throw new Error('Missing display field')
  const response = await page.request.post(
    `/api/v1/tables/${schema.table.id}/rows`,
    {
      headers: headers(),
      data: {
        rows: labels.map((label) => ({ fields: { [fieldId]: label } })),
      },
    },
  )
  expect(response.status()).toBe(201)
  return ((await response.json()) as { rows: DataRow[] }).rows
}

async function readRow(page: Page, tableId: string, rowId: string) {
  const response = await page.request.get(
    `/api/v1/tables/${tableId}/rows/${rowId}`,
  )
  expect(response.status()).toBe(200)
  return (await response.json()) as DataRow
}

async function focusRange(
  page: Page,
  fieldId: string,
  row: number,
  endRow = row,
) {
  const grid = page.locator('revo-grid')
  await grid.evaluate(
    async (element, { fieldId, row, endRow }) => {
      const grid = element as HTMLElement & Components.RevoGrid
      await grid.scrollToRow(row)
      const column = (await grid.getColumns()).find(
        (item) => item.prop === fieldId,
      )
      if (!column) throw new Error('Missing undo test column')
      const type = column.pin ?? 'rgCol'
      const store = await grid.getColumnStore(type)
      const source = store.get('source')
      const x = store
        .get('items')
        .findIndex((index) => source[index]?.prop === fieldId)
      if (x < 0) throw new Error('Undo test column is not visible')
      await grid.setCellsFocus({ x, y: row }, { x, y: endRow }, type)
      grid.focus()
    },
    { fieldId, row, endRow },
  )
  await expect
    .poll(() =>
      grid.evaluate(async (element) => {
        const range = await (
          element as HTMLElement & Components.RevoGrid
        ).getSelectedRange()
        return { y: range?.y, y1: range?.y1 }
      }),
    )
    .toEqual({ y: row, y1: endRow })
}

async function shortcut(page: Page, direction: 'undo' | 'redo') {
  await expect(
    page.getByRole('button', { name: en.undo[direction], exact: true }),
  ).toBeEnabled()
  await page
    .locator('revo-grid')
    .evaluate((element) => (element as HTMLElement).focus())
  await page.keyboard.press(direction === 'undo' ? 'Control+z' : 'Control+y')
}

test('AT-608: reverse-order undo and redo preserve rows, links, comments and view config', async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, 'Range paste and fill-down are desktop grid actions.')
  test.setTimeout(180_000)
  await page.setViewportSize({ width: 1440, height: 900 })
  await login(page)
  const { source, target } = await tables(page, `desktop-${Date.now()}`)
  const extra = await page.request.post(
    `/api/v1/tables/${source.table.id}/fields`,
    { headers: headers(), data: { name: 'Extra', uiType: 'text' } },
  )
  expect(extra.status()).toBe(201)
  const relationResponse = await page.request.post(
    `/api/v1/tables/${source.table.id}/relations`,
    {
      headers: headers(),
      data: {
        name: 'Projects',
        reverseName: 'Rows',
        targetTableId: target.table.id,
        kind: 'many_many',
      },
    },
  )
  expect(relationResponse.status()).toBe(201)
  const relation = (await relationResponse.json()) as {
    schema: TableSchema
    relation: TableSchema['relations'][number]
  }
  const schema = relation.schema
  const name = schema.table.displayFieldId
  const extraField = schema.fields.find((field) => field.name === 'Extra')
  if (!name || !extraField) throw new Error('Missing undo test fields')
  const original = await createRows(page, schema, ['A', 'B', 'C', 'D', 'E'])
  const project = (await createRows(page, target, ['Apollo']))[0]!
  const rowsPath = `/api/v1/tables/${schema.table.id}/rows`
  const relationPath = (rowId: string) =>
    `${rowsPath}/${rowId}/links/${relation.relation.parentFieldId}`
  expect(
    (
      await page.request.post(relationPath(original[2]!.id), {
        headers: headers(),
        data: { add: [project.id] },
      })
    ).status(),
  ).toBe(200)
  expect(
    (
      await page.request.post(`${rowsPath}/${original[2]!.id}/comments`, {
        headers: headers(),
        data: { body: 'Retained comment' },
      })
    ).status(),
  ).toBe(201)
  const viewId = schema.views[0]!.id
  const schemaOf = async () =>
    (await (
      await page.request.get(`/api/v1/tables/${schema.table.id}/schema`)
    ).json()) as TableSchema
  const visible = async () =>
    (await schemaOf()).views
      .find((view) => view.id === viewId)!
      .config.fields.find((field) => field.id === extraField.id)?.visible
  const values = async () =>
    Promise.all(
      original.map(
        async (row) =>
          (await readRow(page, schema.table.id, row.id)).fields[name],
      ),
    )
  await page.goto(`/t/${schema.table.id}/v/${viewId}`)
  await expect(page.locator('revo-grid')).toBeVisible()
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])

  const menu = await openFieldPanel(page)
  await menu
    .getByLabel(en.field.panel.showField.replace('{name}', 'Extra'), {
      exact: true,
    })
    .uncheck()
  await expect.poll(visible).toBe(false)
  await page.locator('.grid-fields-trigger').click()
  await page.getByRole('button', { name: en.grid.addRow, exact: true }).click()
  const query = async () => {
    const response = await page.request.post(`/api/v1/views/${viewId}/query`, {
      headers: headers(),
      data: { limit: 100, includeCount: true },
    })
    expect(response.status()).toBe(200)
    return (await response.json()) as { rows: DataRow[]; total: number }
  }
  await expect.poll(async () => (await query()).total).toBe(6)
  const inserted = (await query()).rows.find(
    (row) => !original.some((item) => item.id === row.id),
  )
  if (!inserted) throw new Error('Inserted row is missing')

  await focusRange(page, name, 0)
  await page.locator('revo-grid').evaluate(async (element, fieldId) => {
    await (element as HTMLElement & Components.RevoGrid).setCellEdit(0, fieldId)
  }, name)
  const editor = page.getByRole('textbox', {
    name: en.field.editor.value,
    exact: true,
  })
  await editor.fill('Edited A')
  await editor.press('Enter')
  await expect
    .poll(
      async () =>
        (await readRow(page, schema.table.id, original[0]!.id)).fields[name],
    )
    .toBe('Edited A')

  await focusRange(page, name, 1)
  await page.evaluate(() => navigator.clipboard.writeText('Paste B\nPaste C'))
  await page.keyboard.press('Control+v')
  await expect
    .poll(values)
    .toEqual(['Edited A', 'Paste B', 'Paste C', 'D', 'E'])
  await focusRange(page, name, 0, 2)
  await page.keyboard.press('Control+d')
  await expect
    .poll(values)
    .toEqual(['Edited A', 'Edited A', 'Edited A', 'D', 'E'])

  for (const row of original.slice(2))
    await page
      .getByRole('checkbox', {
        name: en.grid.selectRow.replace('{id}', row.id),
        exact: true,
      })
      .check()
  const deleteSelected = page.getByRole('button', {
    name: en.grid.deleteRows.replace('{count}', '3'),
    exact: true,
  })
  await expect(deleteSelected).toBeEnabled()
  await deleteSelected.click()
  const deleteDialog = page.getByRole('dialog', {
    name: en.grid.confirmDeleteTitle,
    exact: true,
  })
  await deleteDialog
    .getByRole('button', { name: en.grid.deleteConfirm, exact: true })
    .click()
  await expect
    .poll(async () =>
      (await page.request.get(`${rowsPath}/${original[2]!.id}`)).status(),
    )
    .toBe(404)

  await page
    .getByRole('button', { name: en.relation.edit, exact: true })
    .first()
    .click()
  const picker = page.getByRole('dialog', {
    name: en.relation.pickerTitle.replace('{field}', 'Projects'),
    exact: true,
  })
  await picker.getByLabel('Apollo', { exact: true }).check()
  await picker
    .getByRole('button', { name: en.relation.save, exact: true })
    .click()
  await expect
    .poll(
      async () =>
        (await readRow(page, schema.table.id, original[0]!.id)).fields[
          relation.relation.parentFieldId
        ],
    )
    .toEqual([{ id: project.id, label: 'Apollo' }])

  await shortcut(page, 'undo')
  await expect
    .poll(
      async () =>
        (await readRow(page, schema.table.id, original[0]!.id)).fields[
          relation.relation.parentFieldId
        ],
    )
    .toEqual([])
  await shortcut(page, 'undo')
  await expect
    .poll(async () =>
      (await page.request.get(`${rowsPath}/${original[2]!.id}`)).status(),
    )
    .toBe(200)
  const restored = await readRow(page, schema.table.id, original[2]!.id)
  expect(restored.id).toBe(original[2]!.id)
  expect(restored.commentCount).toBe(1)
  expect(restored.fields[relation.relation.parentFieldId]).toEqual([
    { id: project.id, label: 'Apollo' },
  ])
  const comments = await page.request.get(
    `${rowsPath}/${original[2]!.id}/comments`,
  )
  expect(
    ((await comments.json()) as { items: { body: string }[] }).items[0]?.body,
  ).toBe('Retained comment')
  await shortcut(page, 'undo')
  await expect
    .poll(values)
    .toEqual(['Edited A', 'Paste B', 'Paste C', 'D', 'E'])
  await shortcut(page, 'undo')
  await expect.poll(values).toEqual(['Edited A', 'B', 'C', 'D', 'E'])
  await shortcut(page, 'undo')
  await expect.poll(values).toEqual(['A', 'B', 'C', 'D', 'E'])
  await shortcut(page, 'undo')
  await expect
    .poll(async () =>
      (await page.request.get(`${rowsPath}/${inserted.id}`)).status(),
    )
    .toBe(404)
  await shortcut(page, 'undo')
  await expect.poll(visible).toBe(true)

  await shortcut(page, 'redo')
  await expect.poll(visible).toBe(false)
  await shortcut(page, 'redo')
  await expect
    .poll(async () =>
      (await page.request.get(`${rowsPath}/${inserted.id}`)).status(),
    )
    .toBe(200)
  await shortcut(page, 'redo')
  await expect.poll(values).toEqual(['Edited A', 'B', 'C', 'D', 'E'])
  await shortcut(page, 'redo')
  await expect
    .poll(values)
    .toEqual(['Edited A', 'Paste B', 'Paste C', 'D', 'E'])
  await shortcut(page, 'redo')
  await expect
    .poll(values)
    .toEqual(['Edited A', 'Edited A', 'Edited A', 'D', 'E'])
  await shortcut(page, 'redo')
  await expect
    .poll(async () =>
      (await page.request.get(`${rowsPath}/${original[2]!.id}`)).status(),
    )
    .toBe(404)
  await shortcut(page, 'redo')
  await expect
    .poll(
      async () =>
        (await readRow(page, schema.table.id, original[0]!.id)).fields[
          relation.relation.parentFieldId
        ],
    )
    .toEqual([{ id: project.id, label: 'Apollo' }])
})

test('AT-609: a collaborator conflict can be cancelled or confirmed', async ({
  page,
  browser,
  isMobile,
}) => {
  test.setTimeout(180_000)
  await login(page)
  const { source } = await tables(
    page,
    `conflict-${Date.now()}-${isMobile ? 'mobile' : 'desktop'}`,
  )
  const fieldId = source.table.displayFieldId
  if (!fieldId) throw new Error('Missing display field')
  const field = source.fields.find((item) => item.id === fieldId)!
  const row = (await createRows(page, source, ['Before']))[0]!
  const username = `undo-bob-${Date.now()}-${isMobile ? 'm' : 'd'}`
  const created = await page.request.post('/api/v1/admin/users', {
    headers: headers(),
    data: {
      username,
      displayName: 'Bob',
      role: 'user',
      password: 'Temporary-password-123',
    },
  })
  expect(created.status()).toBe(201)
  const userId = (await created.json()).user.id as string
  expect(
    (
      await page.request.put('/api/v1/grants', {
        headers: headers(),
        data: {
          resourceType: 'table',
          resourceId: source.table.id,
          principalType: 'user',
          principalId: userId,
          level: 'write',
        },
      })
    ).status(),
  ).toBe(201)
  const bob = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL,
    locale: 'en-US',
  })
  try {
    await paceLogin(page)
    expect(
      (
        await bob.request.post('/api/v1/auth/login', {
          headers: headers(),
          data: { login: username, password: 'Temporary-password-123' },
        })
      ).status(),
    ).toBe(200)
    expect(
      (
        await bob.request.post('/api/v1/auth/password', {
          headers: headers(),
          data: {
            current: 'Temporary-password-123',
            new: 'Changed-password-123',
          },
        })
      ).status(),
    ).toBe(200)
    await paceLogin(page)
    expect(
      (
        await bob.request.post('/api/v1/auth/login', {
          headers: headers(),
          data: { login: username, password: 'Changed-password-123' },
        })
      ).status(),
    ).toBe(200)
    const base = `/t/${source.table.id}/v/${source.views[0]!.id}`
    await page.goto(`${base}/r/${row.id}`)
    const dialog = page.locator('dialog.row-expand')
    await expect(dialog).toBeVisible()
    const section = dialog.locator(`[data-field-id="${fieldId}"]`)
    await section
      .getByRole('button', {
        name: en.rowExpand.edit.replace('{name}', field.name),
        exact: true,
      })
      .click()
    await section.locator('.field-editor input').fill('Alice edit')
    await section
      .getByRole('button', { name: en.rowExpand.save, exact: true })
      .click()
    await expect
      .poll(
        async () =>
          (await readRow(page, source.table.id, row.id)).fields[fieldId],
      )
      .toBe('Alice edit')
    await dialog
      .getByRole('button', { name: en.rowExpand.close, exact: true })
      .click()
    const path = `/api/v1/tables/${source.table.id}/rows`
    expect(
      (
        await bob.request.patch(`${path}/${row.id}`, {
          headers: headers(),
          data: { fields: { [fieldId]: 'Bob edit' } },
        })
      ).status(),
    ).toBe(200)
    let prompt = ''
    expectHTTPFailure(page, 'PATCH', `${path}:guarded`, 409)
    page.once('dialog', async (dialog) => {
      prompt = dialog.message()
      await dialog.dismiss()
    })
    if (isMobile)
      await page
        .getByRole('button', { name: en.undo.undo, exact: true })
        .click()
    else await shortcut(page, 'undo')
    await expect.poll(() => prompt).toContain('Bob')
    await expect
      .poll(
        async () =>
          (await readRow(page, source.table.id, row.id)).fields[fieldId],
      )
      .toBe('Bob edit')
    await expect(
      page.getByRole('button', { name: en.undo.undo, exact: true }),
    ).toBeEnabled()
    await expect(
      page.getByRole('button', { name: en.undo.redo, exact: true }),
    ).toBeDisabled()

    expectHTTPFailure(page, 'PATCH', `${path}:guarded`, 409)
    page.once('dialog', (dialog) => dialog.accept())
    if (isMobile)
      await page
        .getByRole('button', { name: en.undo.undo, exact: true })
        .click()
    else await shortcut(page, 'undo')
    await expect
      .poll(
        async () =>
          (await readRow(page, source.table.id, row.id)).fields[fieldId],
      )
      .toBe('Before')
    await page.getByRole('button', { name: en.undo.redo, exact: true }).click()
    await expect
      .poll(
        async () =>
          (await readRow(page, source.table.id, row.id)).fields[fieldId],
      )
      .toBe('Bob edit')
    await setLocale(page, 'zh-CN')
    await expect(
      page.getByRole('button', { name: zh.undo.undo, exact: true }),
    ).toBeVisible()
    await expect(
      page.getByRole('button', { name: zh.undo.redo, exact: true }),
    ).toBeVisible()
  } finally {
    await bob.close()
  }
})
