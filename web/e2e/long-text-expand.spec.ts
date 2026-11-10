import { expect, paceLogin, test } from './fixtures'
import { setLocale } from './app-shell'
import type { Components } from '@revolist/revogrid'
import type { Page } from '@playwright/test'
import type { FieldMutation, TableSchema } from '../src/api/types'
import en from '../src/i18n/en.json' with { type: 'json' }

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

async function create<T>(page: Page, path: string, data: unknown) {
  const response = await page.request.post(path, { headers: headers(), data })
  expect(response.status()).toBe(201)
  return (await response.json()) as T
}

async function gridCell(page: Page, fieldId: string, row: number) {
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

test('long-text expand applies markdown in the grid and draft in the row dialog', async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, 'Long-text grid editing is a desktop interaction.')
  test.setTimeout(150_000)
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

  const stamp = Date.now()
  const { workspace } = await create<{ workspace: { id: string } }>(
    page,
    '/api/v1/workspaces',
    { name: `Long text ${stamp}` },
  )
  const { book } = await create<{ book: { id: string } }>(
    page,
    `/api/v1/workspaces/${workspace.id}/books`,
    { name: 'Long text book' },
  )
  const table = await create<TableSchema>(
    page,
    `/api/v1/books/${book.id}/tables`,
    { name: 'Long text table' },
  )
  const tableId = table.table.id
  const view = table.views[0]
  if (!view || !table.table.displayFieldId)
    throw new Error('The table is missing its grid view or display field')
  const { field: notes } = await create<FieldMutation>(
    page,
    `/api/v1/tables/${tableId}/fields`,
    { name: 'Notes', uiType: 'long_text' },
  )
  if (!notes) throw new Error('Long-text field creation returned no field')
  const inserted = await page.request.post(`/api/v1/tables/${tableId}/rows`, {
    headers: headers(),
    data: {
      rows: [
        {
          fields: {
            [table.table.displayFieldId]: 'Long text row',
            [notes.id]: 'original grid text',
          },
        },
      ],
    },
  })
  expect(inserted.status()).toBe(201)
  const query = await page.request.post(`/api/v1/views/${view.id}/query`, {
    headers: headers(),
    data: { offset: 0, limit: 10, includeCount: true },
  })
  expect(query.status()).toBe(200)
  const row = ((await query.json()) as { rows: { id: string }[] }).rows[0]
  if (!row) throw new Error('The inserted row was not returned')

  await page.goto(`/t/${tableId}/v/${view.id}`)
  await expect(page.locator('revo-grid')).toBeVisible()
  const cell = await gridCell(page, notes.id, 0)
  await cell.click()
  const inline = page.getByRole('textbox', {
    name: en.field.editor.value,
    exact: true,
  })
  await expect(inline).toHaveValue('original grid text')
  await page
    .getByRole('button', {
      name: en.cellEditors.ces05.expand.replace(
        '{field}',
        en.field.editor.value,
      ),
      exact: true,
    })
    .click()
  const expanded = page.locator('dialog.long-text-dialog')
  await expect(expanded).toBeVisible()
  const expandedArea = expanded.locator('.long-text-dialog__textarea')
  await expandedArea.fill('grid text')
  await expandedArea.evaluate((element) => {
    const textarea = element as HTMLTextAreaElement
    textarea.setSelectionRange(0, textarea.value.length)
    textarea.dispatchEvent(new Event('select', { bubbles: true }))
  })
  await expanded
    .getByRole('button', {
      name: new RegExp(en.cellEditors.ces04.commands.bold),
    })
    .click()
  const gridSave = page.waitForResponse(
    (response) =>
      response.request().method() === 'PATCH' &&
      new URL(response.url()).pathname ===
        `/api/v1/tables/${tableId}/rows/${row.id}`,
  )
  await expanded
    .getByRole('button', { name: en.cellEditors.ces04.close, exact: true })
    .click()
  expect((await gridSave).status()).toBe(200)
  await expect(cell).toContainText('**grid text**')
  await page.reload()
  await expect(await gridCell(page, notes.id, 0)).toContainText('**grid text**')

  await page
    .getByRole('button', {
      name: en.rowExpand.open.replace('{id}', row.id),
      exact: true,
    })
    .click()
  const rowDialog = page.locator('dialog.row-expand')
  const section = rowDialog.locator(`[data-field-id="${notes.id}"]`)
  await section.locator('.row-field-value').click()
  const rowEditor = section.locator('.field-editor textarea')
  await expect(rowEditor).toHaveValue('**grid text**')
  await section
    .getByRole('button', {
      name: en.cellEditors.ces05.expand.replace(
        '{field}',
        en.field.editor.value,
      ),
      exact: true,
    })
    .click()
  const rowTextDialog = page.locator('dialog.long-text-dialog')
  await expect(rowTextDialog).toBeVisible()
  const rowText = rowTextDialog.locator('.long-text-dialog__textarea')
  await rowText.fill('row dialog draft')
  await rowText.press('Control+Enter')
  await expect(rowEditor).toHaveValue('row dialog draft')
  await section
    .getByRole('button', { name: en.rowExpand.save, exact: true })
    .click()
  await expect(section.locator('.field-editor')).toHaveCount(0)
  const persisted = await page.request.get(
    `/api/v1/tables/${tableId}/rows/${row.id}`,
  )
  expect(persisted.status()).toBe(200)
  expect(
    ((await persisted.json()) as { fields: Record<string, string> }).fields[
      notes.id
    ],
  ).toBe('row dialog draft')
})
