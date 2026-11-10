import type { Locator, Page } from '@playwright/test'
import type { Components } from '@revolist/revogrid'
import { expect, paceLogin, test } from './fixtures'
import type { DataRow, FieldMutation, TableSchema } from '../src/api/types'
import { setLocale } from './app-shell'
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

async function rowValue(
  page: Page,
  tableId: string,
  rowId: string,
  fieldId: string,
) {
  const response = await page.request.get(
    `/api/v1/tables/${tableId}/rows/${rowId}`,
  )
  expect(response.status()).toBe(200)
  return ((await response.json()) as DataRow).fields[fieldId]
}

test('typed temporal values and calendar picks persist after reload', async ({
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

  const suffix = Date.now()
  const { workspace } = await create<{ workspace: { id: string } }>(
    page,
    '/api/v1/workspaces',
    { name: `Temporal typing ${suffix}` },
  )
  const { book } = await create<{ book: { id: string } }>(
    page,
    `/api/v1/workspaces/${workspace.id}/books`,
    { name: 'Temporal typing book' },
  )
  const schema = await create<TableSchema>(
    page,
    `/api/v1/books/${book.id}/tables`,
    { name: 'Temporal typing table' },
  )
  const addField = (name: string, uiType: 'date' | 'time') =>
    create<FieldMutation>(page, `/api/v1/tables/${schema.table.id}/fields`, {
      name,
      uiType,
    })
  const { field: date } = await addField('Date', 'date')
  const { field: time } = await addField('Time', 'time')
  const view = schema.views[0]
  if (!date || !time || !view)
    throw new Error('Temporal fields or view were not returned')

  const year = new Date().getFullYear()
  const { rows } = await create<{ rows: DataRow[] }>(
    page,
    `/api/v1/tables/${schema.table.id}/rows`,
    {
      rows: [
        { fields: {} },
        { fields: {} },
        { fields: { [date.id]: `${year}-11-14` } },
      ],
    },
  )
  const [timeRow, typedDateRow, pickedDateRow] = rows
  if (!timeRow || !typedDateRow || !pickedDateRow)
    throw new Error('Temporal test rows were not returned')

  await page.goto(`/t/${schema.table.id}/v/${view.id}`)
  const editor = page.getByRole('textbox', {
    name: en.field.editor.value,
    exact: true,
  })

  await (await gridCell(page, time.id, 0)).click()
  await expect(editor).toBeFocused()
  await page.keyboard.type('1345', { delay: 350 })
  await page.keyboard.press('Enter')
  await expect
    .poll(() => rowValue(page, schema.table.id, timeRow.id, time.id))
    .toBe('13:45:00')
  await page.reload()
  await expect(await gridCell(page, time.id, 0)).toContainText('13:45:00')

  await (await gridCell(page, date.id, 1)).click()
  await expect(editor).toBeFocused()
  await editor.fill('11/15')
  await page.keyboard.press('Enter')
  await expect
    .poll(() => rowValue(page, schema.table.id, typedDateRow.id, date.id))
    .toBe(`${year}-11-15`)
  await page.reload()
  await expect(await gridCell(page, date.id, 1)).toContainText(`${year}-11-15`)

  await (await gridCell(page, date.id, 2)).click()
  const calendar = page.locator('.temporal-picker-portal .n-date-panel--date')
  await expect(calendar).toBeVisible()
  await calendar
    .locator('.n-date-panel-date:not(.n-date-panel-date--excluded)')
    .filter({ hasText: /^15$/ })
    .click()
  await expect
    .poll(() => rowValue(page, schema.table.id, pickedDateRow.id, date.id))
    .toBe(`${year}-11-15`)
  await page.reload()
  await expect(await gridCell(page, date.id, 2)).toContainText(`${year}-11-15`)
})
