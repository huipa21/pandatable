import { expect, paceLogin, test } from './fixtures'
import type { AuthUser, DataRow, TableSchema } from '../src/api/types'
import { setLocale } from './app-shell'
import { openFieldTypeActions } from './table-controls'
import en from '../src/i18n/en.json' with { type: 'json' }
import type { Page } from '@playwright/test'

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
  await expect(page).toHaveURL(/\/(?:home|w\/[^/]+|change-password)$/)
}

async function createFormula(
  page: Page,
  tableId: string,
  name: string,
  expr: string,
) {
  const response = await page.request.post(`/api/v1/tables/${tableId}/fields`, {
    headers: headers(),
    data: { name, uiType: 'formula', options: { expr } },
  })
  expect(response.status()).toBe(201)
  return (await response.json()) as {
    field: TableSchema['fields'][number]
    schema: TableSchema
  }
}

async function queryRows(page: Page, viewId: string) {
  const response = await page.request.post(`/api/v1/views/${viewId}/query`, {
    headers: headers(),
    data: {},
  })
  expect(response.status()).toBe(200)
  return ((await response.json()) as { rows: DataRow[] }).rows
}

test('AT-409 formula editor, query-time expressions and stable field references', async ({
  page,
}) => {
  test.setTimeout(150_000)
  await signIn(page)

  const stamp = Date.now()
  const workspaceResponse = await page.request.post('/api/v1/workspaces', {
    headers: headers(),
    data: { name: `Formula ${stamp}` },
  })
  expect(workspaceResponse.status()).toBe(201)
  const workspaceId = (
    (await workspaceResponse.json()) as { workspace: { id: string } }
  ).workspace.id
  const bookResponse = await page.request.post(
    `/api/v1/workspaces/${workspaceId}/books`,
    { headers: headers(), data: { name: 'Formula book' } },
  )
  expect(bookResponse.status()).toBe(201)
  const bookId = ((await bookResponse.json()) as { book: { id: string } }).book
    .id
  const tableResponse = await page.request.post(
    `/api/v1/books/${bookId}/tables`,
    { headers: headers(), data: { name: 'Formula table' } },
  )
  expect(tableResponse.status()).toBe(201)
  const schema = (await tableResponse.json()) as TableSchema
  const tableId = schema.table.id

  const createField = async (name: string, uiType: string) => {
    const response = await page.request.post(
      `/api/v1/tables/${tableId}/fields`,
      { headers: headers(), data: { name, uiType, options: {} } },
    )
    expect(response.status()).toBe(201)
    return (await response.json()) as {
      field: TableSchema['fields'][number]
      schema: TableSchema
    }
  }
  const price = await createField('Price', 'decimal')
  const qty = await createField('Qty', 'integer')
  const status = await createField('Status', 'text')
  const due = await createField('Due', 'date')
  const first = await createField('First', 'text')
  const last = await createField('Last', 'text')
  const total = await createFormula(page, tableId, 'Total', '{Price} * {Qty}')
  const completed = await createFormula(
    page,
    tableId,
    'Completed',
    'IF({Status}="Done","✓","")',
  )
  const days = await createFormula(
    page,
    tableId,
    'Days until due',
    'DATETIME_DIFF(TODAY(), {Due}, "days")',
  )

  await page.goto(`/t/${tableId}/v/${schema.views[0]!.id}`)
  const panel = await openFieldTypeActions(page)
  await panel.locator('[data-kind="formula"]').click()
  const dialog = page.getByRole('dialog', {
    name: en.field.panel.formulaTitle,
    exact: true,
  })
  await expect(dialog).toBeVisible()
  await dialog
    .getByRole('textbox', { name: en.field.panel.name, exact: true })
    .fill('Full name')
  const expression = dialog.getByLabel(en.field.panel.formulaExpression, {
    exact: true,
  })
  await expression.fill('CON')
  await dialog.getByRole('button', { name: 'CONCAT(', exact: true }).click()
  await expression.fill('CONCAT({Fi')
  await dialog.getByRole('button', { name: 'First', exact: true }).click()
  await expression.fill('CONCAT({First}, " ", {Last})')
  await expect(dialog.getByRole('status')).toContainText(
    en.field.panel.formulaType.replace('{type}', 'text'),
  )
  await dialog
    .getByRole('button', { name: en.field.panel.create, exact: true })
    .click()
  await expect(dialog).not.toBeVisible()
  const formulaSchema = (await (
    await page.request.get(`/api/v1/tables/${tableId}/schema`)
  ).json()) as TableSchema
  const fullName = formulaSchema.fields.find(
    (field) => field.name === 'Full name',
  )!

  const newField = await openFieldTypeActions(page)
  await newField.locator('[data-kind="formula"]').click()
  const invalidDialog = page.getByRole('dialog', {
    name: en.field.panel.formulaTitle,
    exact: true,
  })
  await invalidDialog
    .getByRole('textbox', { name: en.field.panel.name, exact: true })
    .fill('Invalid formula')
  await invalidDialog
    .getByLabel(en.field.panel.formulaExpression, { exact: true })
    .fill('{Price} +')
  await expect(invalidDialog.getByRole('alert')).toContainText(
    en.field.panel.formulaInvalid.replace('{position}', '10'),
  )
  await invalidDialog
    .getByRole('button', { name: en.field.panel.cancel, exact: true })
    .click()

  const meResponse = await page.request.get('/api/v1/auth/me')
  expect(meResponse.status()).toBe(200)
  const me = (await meResponse.json()) as AuthUser
  const formatter = new Intl.DateTimeFormat('en-CA', {
    timeZone: me.effectiveTimezone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  })
  const todayInActorZone = () => formatter.format(new Date())
  const today = todayInActorZone()
  const rowResponse = await page.request.post(
    `/api/v1/tables/${tableId}/rows`,
    {
      headers: headers(),
      data: {
        rows: [
          {
            fields: {
              [schema.table.displayFieldId!]: 'Formula example',
              [price.field.id]: 12.5,
              [qty.field.id]: 4,
              [status.field.id]: 'Done',
              [due.field.id]: today,
              [first.field.id]: 'Ada',
              [last.field.id]: 'Lovelace',
            },
          },
        ],
      },
    },
  )
  expect(rowResponse.status()).toBe(201)

  const populatedSchema = (await (
    await page.request.get(`/api/v1/tables/${tableId}/schema`)
  ).json()) as TableSchema
  const readValues = async () => {
    const rows = await queryRows(page, populatedSchema.views[0]!.id)
    return rows[0]!.fields
  }
  const beforeQuery = todayInActorZone()
  let values = await readValues()
  const afterQuery = todayInActorZone()
  const dayDifference = (date: string) =>
    (Date.parse(`${date}T00:00:00Z`) - Date.parse(`${today}T00:00:00Z`)) /
    86_400_000
  const observedDays = values[days.field.id]
  if (typeof observedDays !== 'number')
    throw new Error('DATETIME_DIFF did not return a number')
  expect(values[total.field.id]).toBe(50)
  expect(values[completed.field.id]).toBe('✓')
  expect([dayDifference(beforeQuery), dayDifference(afterQuery)]).toContain(
    observedDays,
  )
  expect(values[fullName.id]).toBe('Ada Lovelace')

  const rename = await page.request.patch(`/api/v1/fields/${price.field.id}`, {
    headers: { ...headers(), 'If-Match': String(price.field.version) },
    data: { name: 'Unit price' },
  })
  expect(rename.status()).toBe(200)
  values = await readValues()
  expect(values[total.field.id]).toBe(50)
})
