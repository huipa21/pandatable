import type { Page } from '@playwright/test'
import { expect, expectHTTPFailure, paceLogin, test } from './fixtures'
import type {
  DataRow,
  TableField,
  TableSchema,
  TableView,
} from '../src/api/types'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

async function post<T>(page: Page, path: string, data: unknown): Promise<T> {
  const response = await page.request.post(path, { headers: headers(), data })
  expect(response.status(), path).toBe(201)
  return response.json()
}

async function login(page: Page) {
  await paceLogin(page)
  await page.goto('/login')
  if ((await page.locator('html').getAttribute('lang')) === 'zh-CN')
    await page
      .getByRole('button', { name: zh.common.languages.en, exact: true })
      .click()
  await page.getByLabel(en.auth.login, { exact: true }).fill('first-admin')
  await page
    .getByLabel(en.auth.password, { exact: true })
    .fill('Setup-password-123')
  await page.getByRole('button', { name: en.auth.signIn, exact: true }).click()
  await expect(page).toHaveURL(/\/home$/)
}

async function navigation(page: Page, isMobile: boolean) {
  if (isMobile && !(await page.locator('.nav-drawer').isVisible()))
    await page.getByRole('button', { name: en.nav.open, exact: true }).click()
  return page.getByRole('navigation', { name: en.nav.title, exact: true })
}

async function openTrash(page: Page, isMobile: boolean) {
  const nav = await navigation(page, isMobile)
  await nav.getByRole('link', { name: en.trash.title, exact: true }).click()
  await expect(page).toHaveURL(/\/trash$/)
  await expect(
    page.getByRole('heading', { name: en.trash.title }),
  ).toBeVisible()
}

async function restore(page: Page, id: string, isMobile: boolean) {
  const button = page.locator(`[data-trash-id="${id}"] button`)
  await expect(button).toBeVisible()
  await expect(button).toBeEnabled()
  if (isMobile) await button.tap()
  else await button.press('Enter')
  await expect(page.locator(`[data-trash-id="${id}"]`)).toHaveCount(0)
}

test('AT-203, AT-325 and AT-610 restore deleted resources and retained data', async ({
  page,
  isMobile,
}) => {
  test.setTimeout(150_000)
  await login(page)
  const suffix = Date.now()
  const workspace = await post<{ workspace: { id: string } }>(
    page,
    '/api/v1/workspaces',
    { name: `Trash workspace ${suffix}` },
  )
  const wid = workspace.workspace.id
  const book = await post<{ book: { id: string } }>(
    page,
    `/api/v1/workspaces/${wid}/books`,
    { name: `Trash book ${suffix}` },
  )
  const bid = book.book.id
  const schema = await post<TableSchema>(page, `/api/v1/books/${bid}/tables`, {
    name: `Trash table ${suffix}`,
  })
  const tid = schema.table.id
  const grid = schema.views[0]!
  const createdField = await post<{ field: TableField }>(
    page,
    `/api/v1/tables/${tid}/fields`,
    {
      name: 'Retained notes',
      uiType: 'text',
      isRequired: true,
      default: 'seed',
    },
  )
  const field = createdField.field
  const createdView = await post<{ view: TableView }>(
    page,
    `/api/v1/tables/${tid}/views`,
    { name: 'Restorable view', type: 'grid', scope: 'shared' },
  )
  const view = createdView.view
  const createdRows = await post<{ rows: DataRow[] }>(
    page,
    `/api/v1/tables/${tid}/rows`,
    { rows: [{ fields: { [field.id]: 'original value' } }] },
  )
  const row = createdRows.rows[0]!

  await page.goto(`/t/${tid}/v/${grid.id}`)
  if (isMobile)
    await page
      .getByRole('button', { name: en.mobileRows.options, exact: true })
      .click()
  const panel = page.getByRole('region', { name: en.field.panel.title })
  const fieldActions = panel.locator(`[data-field-id="${field.id}"] details`)
  await fieldActions.locator('summary').click()
  await fieldActions
    .getByRole('button', { name: en.field.panel.delete, exact: true })
    .click()
  const deleteField = page.getByRole('dialog', { name: en.field.panel.confirm })
  await deleteField
    .getByRole('button', { name: en.field.panel.confirm, exact: true })
    .click()
  await expect(deleteField).not.toBeVisible()
  await expect(panel.locator(`[data-field-id="${field.id}"]`)).toHaveCount(0)
  if (isMobile)
    await page
      .getByRole('button', { name: en.mobileRows.closeOptions, exact: true })
      .click()
  const hidden = await page.request.get(`/api/v1/tables/${tid}/rows/${row.id}`)
  expect(hidden.status()).toBe(200)
  expect(((await hidden.json()) as DataRow).fields).not.toHaveProperty(field.id)
  await post(page, `/api/v1/tables/${tid}/rows`, { rows: [{ fields: {} }] })

  await openTrash(page, isMobile)
  const fieldCard = page.locator(`[data-trash-id="${field.id}"]`)
  await expect(fieldCard).toContainText('Retained notes')
  await expect(fieldCard).toContainText(`Trash workspace ${suffix}`)
  expect(await fieldCard.locator('time').count()).toBe(2)
  expectHTTPFailure(
    page,
    'POST',
    `/api/v1/trash/field/${field.id}/restore`,
    409,
  )
  await fieldCard.getByRole('button').click()
  await expect(page.getByRole('alert')).toContainText('1 empty rows')
  const optional = page.getByRole('button', {
    name: en.trash.restoreOptionalItem.replace('{name}', 'Retained notes'),
  })
  await expect(optional).toBeEnabled()
  if (isMobile) await optional.tap()
  else await optional.press('Enter')
  await expect(fieldCard).toHaveCount(0)
  const restoredRow = await page.request.get(
    `/api/v1/tables/${tid}/rows/${row.id}`,
  )
  expect(restoredRow.status()).toBe(200)
  expect(((await restoredRow.json()) as DataRow).fields[field.id]).toBe(
    'original value',
  )

  const nav = await navigation(page, isMobile)
  const tableNode = nav.locator(`[data-nav-id="${tid}"]`)
  await expect(tableNode).toBeVisible()
  const actions = tableNode.locator(':scope > .node-actions')
  await actions.locator('summary').click()
  await actions
    .getByRole('button', { name: en.nav.delete, exact: true })
    .click()
  const deleteTable = page.getByRole('dialog', { name: en.nav.dialogTitle })
  await deleteTable
    .getByRole('button', { name: en.admin.confirm, exact: true })
    .click()
  await expect(deleteTable).not.toBeVisible()
  await expect(tableNode).toHaveCount(0)
  if (isMobile && (await page.locator('.nav-drawer').isVisible()))
    await page.getByRole('button', { name: en.nav.close, exact: true }).click()
  expect(
    (await page.request.get(`/api/v1/tables/${tid}/schema`)).status(),
  ).toBe(404)
  await page.getByRole('button', { name: en.trash.refresh }).click()
  await restore(page, tid, isMobile)
  const tableRow = await page.request.get(
    `/api/v1/tables/${tid}/rows/${row.id}`,
  )
  expect(tableRow.status()).toBe(200)
  expect(((await tableRow.json()) as DataRow).fields[field.id]).toBe(
    'original value',
  )

  const deleteView = await page.request.delete(`/api/v1/views/${view.id}`, {
    headers: { ...headers(), 'If-Match': String(view.version) },
  })
  expect(deleteView.status()).toBe(200)
  await page.getByRole('button', { name: en.trash.refresh }).click()
  await restore(page, view.id, isMobile)

  for (const [kind, id] of [
    ['books', bid],
    ['workspaces', wid],
  ]) {
    const deleted = await page.request.delete(`/api/v1/${kind}/${id}`, {
      headers: headers(),
    })
    expect(deleted.status()).toBe(200)
    await page.getByRole('button', { name: en.trash.refresh }).click()
    await restore(page, id!, isMobile)
  }
  const finalSchema = await page.request.get(`/api/v1/tables/${tid}/schema`)
  expect(finalSchema.status()).toBe(200)
  expect(
    ((await finalSchema.json()) as TableSchema).views.map((item) => item.id),
  ).toContain(view.id)
  await page
    .getByRole('button', { name: en.common.languages.zhCN, exact: true })
    .click()
  await expect(
    page.getByRole('heading', { name: zh.trash.title }),
  ).toBeVisible()
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true)
})
