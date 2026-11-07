import type { Locator, Page } from '@playwright/test'
import type { Components } from '@revolist/revogrid'
import { expect, paceLogin, test } from './fixtures'
import type { DataRow, FieldMutation, TableSchema } from '../src/api/types'
import en from '../src/i18n/en.json' with { type: 'json' }

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

async function create<T>(page: Page, path: string, data: unknown): Promise<T> {
  const response = await page.request.post(path, { headers: headers(), data })
  expect(response.status()).toBe(201)
  return (await response.json()) as T
}

async function login(page: Page) {
  await paceLogin(page)
  await page.goto('/login')
  await page.getByLabel(en.auth.login, { exact: true }).fill('first-admin')
  await page
    .getByLabel(en.auth.password, { exact: true })
    .fill('Setup-password-123')
  await page.getByRole('button', { name: en.auth.signIn, exact: true }).click()
  await expect(page).toHaveURL(/\/w\/[^/]+$/)
}

async function selectRow(page: Page, id: string, index: number) {
  await page
    .locator('revo-grid')
    .evaluate(
      async (element, rowIndex) =>
        (element as HTMLElement & Components.RevoGrid).scrollToRow(rowIndex),
      index,
    )
  const checkbox = page.getByRole('checkbox', {
    name: en.grid.selectRow.replace('{id}', id),
    exact: true,
  })
  await expect(checkbox).toBeVisible()
  await checkbox.check()
}

async function openBulkEdit(page: Page, fieldName: string) {
  await page
    .getByRole('button', { name: en.grid.editRows, exact: true })
    .click()
  const dialog = page.getByRole('dialog', {
    name: en.grid.bulkEditTitle,
    exact: true,
  })
  await expect(dialog).toBeVisible()
  await dialog
    .getByRole('combobox', { name: en.grid.bulkEditField, exact: true })
    .selectOption({ label: fieldName })
  return dialog
}

async function previewAndApply(dialog: Locator) {
  await dialog
    .getByRole('button', { name: en.grid.previewBulkEdit, exact: true })
    .click()
  await expect(dialog.getByRole('status')).toContainText('2 selected rows')
  await dialog
    .getByRole('button', { name: en.grid.applyBulkEdit, exact: true })
    .click()
  await expect(dialog).toBeHidden()
}

test('WG-07: selected rows support typed atomic edits, delete and undo', async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, 'Selected-row actions are part of the desktop grid.')
  test.setTimeout(150_000)
  await page.setViewportSize({ width: 1440, height: 900 })
  await login(page)

  const suffix = Date.now()
  const { workspace } = await create<{ workspace: { id: string } }>(
    page,
    '/api/v1/workspaces',
    { name: `Bulk actions ${suffix}` },
  )
  const { book } = await create<{ book: { id: string } }>(
    page,
    `/api/v1/workspaces/${workspace.id}/books`,
    { name: 'Bulk actions book' },
  )
  const initial = await create<TableSchema>(
    page,
    `/api/v1/books/${book.id}/tables`,
    { name: 'Bulk actions table' },
  )
  const tableId = initial.table.id
  const choices = {
    choices: [
      { id: 'todo', label: 'Todo', color: 'gray' },
      { id: 'finished', label: 'Finished', color: 'green' },
    ],
  }
  const fields = new Map<string, string>()
  for (const data of [
    { name: 'Quantity', uiType: 'integer' },
    { name: 'Enabled', uiType: 'checkbox' },
    { name: 'Stage', uiType: 'single_select', options: choices },
    { name: 'Due', uiType: 'date' },
  ]) {
    const result = await create<FieldMutation>(
      page,
      `/api/v1/tables/${tableId}/fields`,
      data,
    )
    if (!result.field) throw new Error(`Missing ${data.name} field`)
    fields.set(data.name, result.field.id)
  }
  const schemaResponse = await page.request.get(
    `/api/v1/tables/${tableId}/schema`,
  )
  expect(schemaResponse.status()).toBe(200)
  const schema = (await schemaResponse.json()) as TableSchema
  const nameField = schema.table.displayFieldId
  const view = schema.views[0]
  if (!nameField || !view) throw new Error('The table is missing its grid view')
  const inserted = await create<{ rows: DataRow[] }>(
    page,
    `/api/v1/tables/${tableId}/rows`,
    {
      rows: Array.from({ length: 151 }, (_, index) => ({
        fields: {
          [nameField]: `Row ${index}`,
          [fields.get('Quantity')!]: index,
          [fields.get('Enabled')!]: true,
          [fields.get('Stage')!]: 'Todo',
          [fields.get('Due')!]: '2026-01-01',
        },
      })),
    },
  )
  const first = inserted.rows[1]
  const later = inserted.rows[150]
  const untouched = inserted.rows[2]
  if (!first || !later || !untouched) throw new Error('Missing seeded rows')
  const readRow = async (id: string) => {
    const response = await page.request.get(
      `/api/v1/tables/${tableId}/rows/${id}`,
    )
    expect(response.status()).toBe(200)
    return (await response.json()) as DataRow
  }
  const rowActions = async (ids: string[]) => {
    await selectRow(page, ids[0]!, 1)
    await selectRow(page, ids[1]!, 150)
  }
  const patchBodies: unknown[] = []
  page.on('request', (request) => {
    if (
      request.method() === 'PATCH' &&
      new URL(request.url()).pathname === `/api/v1/tables/${tableId}/rows`
    )
      patchBodies.push(request.postDataJSON())
  })

  await page.goto(`/t/${tableId}/v/${view.id}`)
  await expect(page.locator('revo-grid')).toBeVisible()
  await rowActions([first.id, later.id])

  let dialog = await openBulkEdit(page, 'Name')
  await dialog
    .getByLabel(en.field.editor.value, { exact: true })
    .fill('Shared value')
  await dialog
    .getByRole('button', { name: en.grid.previewBulkEdit, exact: true })
    .click()
  await dialog
    .getByRole('button', { name: en.grid.cancel, exact: true })
    .click()
  await expect(
    page.getByRole('button', { name: en.grid.editRows }),
  ).toBeVisible()
  expect(patchBodies).toHaveLength(0)

  dialog = await openBulkEdit(page, 'Name')
  await dialog
    .getByLabel(en.field.editor.value, { exact: true })
    .fill('Shared value')
  await previewAndApply(dialog)
  expect(patchBodies.at(-1)).toEqual({
    rows: [
      { id: first.id, fields: { [nameField]: 'Shared value' } },
      { id: later.id, fields: { [nameField]: 'Shared value' } },
    ],
  })
  expect((await readRow(first.id)).fields[nameField]).toBe('Shared value')
  expect((await readRow(later.id)).fields[nameField]).toBe('Shared value')

  await rowActions([first.id, later.id])
  dialog = await openBulkEdit(page, 'Quantity')
  await dialog.getByLabel(en.field.editor.value, { exact: true }).fill('42')
  await previewAndApply(dialog)
  expect((await readRow(later.id)).fields[fields.get('Quantity')!]).toBe(42)

  await rowActions([first.id, later.id])
  dialog = await openBulkEdit(page, 'Enabled')
  const checkbox = dialog.getByLabel(en.field.editor.value, { exact: true })
  await checkbox.check()
  await checkbox.uncheck()
  await previewAndApply(dialog)
  expect((await readRow(later.id)).fields[fields.get('Enabled')!]).toBe(false)

  await rowActions([first.id, later.id])
  dialog = await openBulkEdit(page, 'Stage')
  await dialog
    .getByLabel(en.field.editor.value, { exact: true })
    .selectOption({ label: 'Finished' })
  await previewAndApply(dialog)
  expect((await readRow(later.id)).fields[fields.get('Stage')!]).toBe(
    'Finished',
  )

  await rowActions([first.id, later.id])
  dialog = await openBulkEdit(page, 'Stage')
  await dialog
    .getByLabel(en.field.editor.value, { exact: true })
    .selectOption({ label: en.field.editor.none })
  await previewAndApply(dialog)
  expect((await readRow(later.id)).fields[fields.get('Stage')!]).toBeNull()

  await rowActions([first.id, later.id])
  dialog = await openBulkEdit(page, 'Due')
  await dialog
    .getByLabel(en.field.editor.value, { exact: true })
    .fill('2026-06-15')
  await previewAndApply(dialog)
  expect((await readRow(later.id)).fields[fields.get('Due')!]).toBe(
    '2026-06-15',
  )
  expect((await readRow(untouched.id)).fields[nameField]).toBe('Row 2')
  expect(patchBodies).toHaveLength(6)

  await rowActions([first.id, later.id])
  await page
    .getByRole('button', {
      name: en.grid.deleteRows.replace('{count}', '2'),
      exact: true,
    })
    .click()
  const deleteDialog = page.getByRole('dialog', {
    name: en.grid.confirmDeleteTitle,
    exact: true,
  })
  await deleteDialog
    .getByRole('button', { name: en.grid.cancel, exact: true })
    .click()
  await expect(
    page.getByRole('button', { name: en.grid.editRows }),
  ).toBeVisible()
  const deleteRequests: unknown[] = []
  page.on('request', (request) => {
    if (
      request.method() === 'DELETE' &&
      new URL(request.url()).pathname === `/api/v1/tables/${tableId}/rows`
    )
      deleteRequests.push(request.postDataJSON())
  })
  await page
    .getByRole('button', {
      name: en.grid.deleteRows.replace('{count}', '2'),
      exact: true,
    })
    .click()
  await page
    .getByRole('dialog', { name: en.grid.confirmDeleteTitle, exact: true })
    .getByRole('button', { name: en.grid.deleteConfirm, exact: true })
    .click()
  await expect.poll(() => deleteRequests.length).toBe(1)
  expect(deleteRequests[0]).toEqual({ ids: [first.id, later.id] })
  await expect
    .poll(async () =>
      (
        await page.request.get(`/api/v1/tables/${tableId}/rows/${first.id}`)
      ).status(),
    )
    .toBe(404)
  await page.getByRole('button', { name: en.undo.undo, exact: true }).click()
  await expect
    .poll(async () =>
      (
        await page.request.get(`/api/v1/tables/${tableId}/rows/${first.id}`)
      ).status(),
    )
    .toBe(200)
  await expect
    .poll(async () =>
      (
        await page.request.get(`/api/v1/tables/${tableId}/rows/${later.id}`)
      ).status(),
    )
    .toBe(200)
  await expect
    .poll(async () => (await readRow(first.id)).fields[nameField])
    .toBe('Shared value')
  await expect
    .poll(async () => (await readRow(later.id)).fields[nameField])
    .toBe('Shared value')
})
