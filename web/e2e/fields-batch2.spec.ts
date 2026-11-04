import type { Page } from '@playwright/test'
import { test, expect, paceLogin } from './fixtures'
import { create as createResource } from './navigation'
import { openFieldCellEditor, revealFieldColumn } from './fields'
import type {
  AdminSettings,
  RowQueryResult,
  TableField,
  TableSchema,
} from '../src/api/types'
import { restoreSettings } from './settings'
import { setLocale } from './app-shell'
import { openFieldPanel } from './table-controls'
import en from '../src/i18n/en.json' with { type: 'json' }

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

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

async function createTable(page: Page, suffix: string) {
  await createResource(page, 'workspace', `Batch2 W ${suffix}`)
  await createResource(page, 'book', `Batch2 B ${suffix}`)
  await createResource(page, 'table', `Batch2 T ${suffix}`)
  const parts = new URL(page.url()).pathname.split('/')
  const tableId = parts[2]
  const viewId = parts[4]
  if (!tableId || !viewId) throw new Error('Table route incomplete')
  return { tableId, viewId }
}

async function schemaOf(page: Page, tableId: string) {
  const response = await page.request.get(`/api/v1/tables/${tableId}/schema`)
  expect(response.status()).toBe(200)
  return (await response.json()) as TableSchema
}

async function queryRows(page: Page, viewId: string) {
  const response = await page.request.post(`/api/v1/views/${viewId}/query`, {
    headers: headers(),
    data: { offset: 0, limit: 100, includeCount: true },
  })
  expect(response.status()).toBe(200)
  return (await response.json()) as RowQueryResult
}

interface FieldSpec {
  name: string
  kind: string
  maxLength?: string
  scale?: string
  currency?: string
  displayDecimals?: string
  ratingMax?: string
  autoNumberPrefix?: string
  autoNumberPadding?: string
}

async function addField(page: Page, spec: FieldSpec) {
  const panel = await openFieldPanel(page)
  await panel
    .getByRole('button', { name: en.field.panel.add, exact: true })
    .click()
  const editor = page.getByRole('dialog', {
    name: en.field.panel.createTitle,
    exact: true,
  })
  await expect(editor).toBeVisible()
  await editor
    .getByRole('textbox', { name: en.field.panel.name, exact: true })
    .fill(spec.name)
  await editor.locator(`[data-kind="${spec.kind}"]`).click()
  if (spec.maxLength !== undefined)
    await editor
      .getByRole('textbox', { name: en.field.panel.maxLength, exact: true })
      .fill(spec.maxLength)
  if (spec.scale !== undefined)
    await editor
      .getByRole('textbox', { name: en.field.panel.scale, exact: true })
      .fill(spec.scale)
  if (spec.currency !== undefined)
    await editor
      .getByRole('textbox', { name: en.field.panel.currency, exact: true })
      .fill(spec.currency)
  if (spec.displayDecimals !== undefined)
    await editor
      .getByRole('textbox', {
        name: en.field.panel.displayDecimals,
        exact: true,
      })
      .fill(spec.displayDecimals)
  if (spec.ratingMax !== undefined)
    await editor
      .getByRole('textbox', { name: en.field.panel.ratingMax, exact: true })
      .fill(spec.ratingMax)
  if (spec.autoNumberPrefix !== undefined)
    await editor
      .getByRole('textbox', { name: en.field.panel.prefix, exact: true })
      .fill(spec.autoNumberPrefix)
  if (spec.autoNumberPadding !== undefined)
    await editor
      .getByRole('textbox', { name: en.field.panel.padding, exact: true })
      .fill(spec.autoNumberPadding)
  await editor
    .getByRole('button', { name: en.field.panel.create, exact: true })
    .click()
  await expect(editor).not.toBeVisible()
  await expect(
    page.getByRole('status').filter({ hasText: en.field.panel.saved }),
  ).toBeVisible()
  const field = (
    await schemaOf(page, new URL(page.url()).pathname.split('/')[2]!)
  ).fields.find((item) => item.name === spec.name)
  if (!field) throw new Error(`Created field ${spec.name} missing from schema`)
  await page.locator('.grid-fields-trigger').click()
  return field
}

async function saveGridText(
  page: Page,
  viewId: string,
  field: TableField,
  value: string,
) {
  await openFieldCellEditor(page, field.id)
  const input = page.getByRole('textbox', {
    name: en.field.editor.value,
    exact: true,
  })
  await expect(input).toBeVisible()
  await input.fill(value)
  const saved = page.waitForResponse(
    (response) =>
      response.request().method() === 'PATCH' &&
      response.url().includes('/api/v1/tables/') &&
      response.url().includes('/rows/'),
    { timeout: 10_000 },
  )
  await input.press(field.uiType === 'json' ? 'Control+Enter' : 'Enter')
  await page.locator('.grid-toolbar').click({ position: { x: 2, y: 2 } })
  expect((await saved).status()).toBe(200)
  await expect
    .poll(async () => (await queryRows(page, viewId)).rows[0]?.fields[field.id])
    .toBeTruthy()
  await expect(
    page.getByRole('status').filter({ hasText: en.grid.saving }),
  ).not.toBeVisible()
}

async function searchAndChooseUser(
  page: Page,
  viewId: string,
  field: TableField,
  username: string,
  applyMultiple: boolean,
) {
  await openFieldCellEditor(page, field.id)
  const editor = page.locator('.field-editor')
  const search = editor.getByRole('searchbox', {
    name: en.field.editor.userSearch,
    exact: true,
  })
  await search.fill(username)
  const result = editor.getByRole('button').filter({ hasText: username })
  await expect(result).toBeVisible()
  await expect(editor).not.toContainText('disabled')
  await result.click()
  if (applyMultiple)
    await editor
      .getByRole('button', { name: en.field.editor.apply, exact: true })
      .click()
  await expect
    .poll(async () => (await queryRows(page, viewId)).rows[0]?.fields[field.id])
    .toBeTruthy()
  await expect(
    page.getByRole('status').filter({ hasText: en.grid.saving }),
  ).not.toBeVisible()
}

async function createTestUser(
  page: Page,
  username: string,
  displayName: string,
) {
  const response = await page.request.post('/api/v1/admin/users', {
    headers: headers(),
    data: {
      username,
      displayName,
      role: 'user',
      password: 'Temporary-password-123',
    },
  })
  expect(response.status()).toBe(201)
  return (await response.json()).user as { id: string; username: string }
}

test('AT-320 batch-2 fields create through Field UI, grid persistence, active user picker and autonumber sequence', async ({
  page,
  isMobile,
}) => {
  test.setTimeout(240_000)
  test.skip(isMobile, 'AT-320 browser acceptance is desktop-only')
  await page.setViewportSize({ width: 1440, height: 900 })
  await login(page)
  const suffix = Date.now()
  const activeUsername = `fieldpick-${suffix}-active`
  const disabledUsername = `fieldpick-${suffix}-disabled`
  const activeUser = await createTestUser(page, activeUsername, 'Active Picker')
  const disabledUser = await createTestUser(
    page,
    disabledUsername,
    'Disabled Picker',
  )
  const disabled = await page.request.patch(
    `/api/v1/admin/users/${disabledUser.id}`,
    {
      headers: headers(),
      data: { status: 'disabled' },
    },
  )
  expect(disabled.status()).toBe(200)

  const { tableId, viewId } = await createTable(page, String(suffix))
  const activeSearch = await page.request.get(
    `/api/v1/tables/${tableId}/directory/users?q=fieldpick-${suffix}`,
  )
  expect(activeSearch.status()).toBe(200)
  expect(await activeSearch.json()).toMatchObject({
    items: [{ id: activeUser.id, username: activeUsername }],
    hasMore: false,
  })

  await page.getByRole('button', { name: en.grid.addRow, exact: true }).click()
  await expect.poll(async () => (await queryRows(page, viewId)).total).toBe(1)

  const specs: FieldSpec[] = [
    { name: 'Varchar', kind: 'text', maxLength: '12' },
    { name: 'Time', kind: 'time' },
    { name: 'Duration', kind: 'duration' },
    { name: 'Currency', kind: 'currency', scale: '2' },
    { name: 'Percent', kind: 'percent', displayDecimals: '2' },
    { name: 'Rating', kind: 'rating', ratingMax: '5' },
    { name: 'Phone', kind: 'phone' },
    { name: 'UUID', kind: 'uuid' },
    { name: 'JSON', kind: 'json' },
    { name: 'INET', kind: 'inet' },
    { name: 'CIDR', kind: 'cidr' },
    { name: 'TextArray', kind: 'text_array' },
    { name: 'IntArray', kind: 'int_array' },
    { name: 'User', kind: 'user' },
    { name: 'MultiUser', kind: 'multi_user' },
    {
      name: 'AutoNumber',
      kind: 'autonumber',
      autoNumberPrefix: 'ROW-',
      autoNumberPadding: '3',
    },
  ]
  const fields: Record<string, TableField> = {}
  for (const spec of specs) fields[spec.name] = await addField(page, spec)

  const values: [string, string][] = [
    ['Varchar', 'varchar-12'],
    ['Time', '13:20:30'],
    ['Duration', 'PT2H3M4.5S'],
    ['Currency', '1234.50'],
    ['Percent', '0.125'],
    ['Rating', '4'],
    ['Phone', '+1 (555) 010-1234'],
    ['UUID', '00000000-0000-7000-8000-000000000001'],
    ['JSON', '{"answer":42}'],
    ['INET', '192.0.2.10'],
    ['CIDR', '192.0.2.0/24'],
    ['TextArray', 'alpha, beta'],
    ['IntArray', '11, 12'],
  ]
  for (const [name, value] of values)
    await saveGridText(page, viewId, fields[name]!, value)
  await searchAndChooseUser(page, viewId, fields.User!, activeUsername, false)
  await searchAndChooseUser(
    page,
    viewId,
    fields.MultiUser!,
    activeUsername,
    true,
  )

  await openFieldCellEditor(page, fields.AutoNumber!.id)
  await expect(
    page.getByRole('textbox', { name: en.field.editor.value, exact: true }),
  ).not.toBeVisible()

  let result = await queryRows(page, viewId)
  const row = result.rows[0]!
  expect(row.fields[fields.Varchar!.id]).toBe('varchar-12')
  expect(row.fields[fields.Time!.id]).toBe('13:20:30')
  expect(row.fields[fields.Duration!.id]).toBe('PT2H3M4.5S')
  expect(row.fields[fields.Currency!.id]).toBe('1234.50')
  expect(row.fields[fields.Percent!.id]).toBe('0.1250')
  expect(row.fields[fields.Rating!.id]).toBe(4)
  expect(row.fields[fields.Phone!.id]).toBe('+1 (555) 010-1234')
  expect(row.fields[fields.UUID!.id]).toBe(
    '00000000-0000-7000-8000-000000000001',
  )
  expect(row.fields[fields.JSON!.id]).toEqual({ answer: 42 })
  expect(row.fields[fields.INET!.id]).toBe('192.0.2.10')
  expect(row.fields[fields.CIDR!.id]).toBe('192.0.2.0/24')
  expect(row.fields[fields.TextArray!.id]).toEqual(['alpha', 'beta'])
  expect(row.fields[fields.IntArray!.id]).toEqual(['11', '12'])
  expect(row.fields[fields.User!.id]).toBe(activeUser.id)
  expect(row.fields[fields.MultiUser!.id]).toEqual([activeUser.id])
  expect(row.fields[fields.AutoNumber!.id]).toBe('1')

  await page.reload()
  await expect(page.getByRole('region', { name: en.grid.title })).toBeVisible()
  result = await queryRows(page, viewId)
  expect(result.rows[0]?.fields).toMatchObject(row.fields)
  await revealFieldColumn(page, fields.User!.id)
  await expect(
    page.getByText('Active Picker', { exact: true }).first(),
  ).toBeVisible()
  await expect(page.getByText(/Disabled Picker/)).toHaveCount(0)

  await page.getByRole('button', { name: en.grid.addRow, exact: true }).click()
  await expect.poll(async () => (await queryRows(page, viewId)).total).toBe(2)
  result = await queryRows(page, viewId)
  expect(result.rows[1]?.fields[fields.AutoNumber!.id]).toBe('2')
})

test('AT-326 currency default snapshots by field and formats each currency', async ({
  page,
  isMobile,
}) => {
  test.setTimeout(180_000)
  test.skip(isMobile, 'AT-326 browser acceptance is desktop-only')
  await page.setViewportSize({ width: 1440, height: 900 })
  await login(page)
  const settingsResponse = await page.request.get('/api/v1/admin/settings')
  expect(settingsResponse.status()).toBe(200)
  const previous = (await settingsResponse.json()) as AdminSettings
  try {
    const setDefault = async (defaultCurrency: string) => {
      const response = await page.request.patch('/api/v1/admin/settings', {
        headers: headers(),
        data: { defaultCurrency },
      })
      expect(response.status()).toBe(200)
    }
    await setDefault('CNY')
    const { tableId, viewId } = await createTable(
      page,
      `Currency ${Date.now()}`,
    )
    await page
      .getByRole('button', { name: en.grid.addRow, exact: true })
      .click()
    await expect.poll(async () => (await queryRows(page, viewId)).total).toBe(1)

    const cny = await addField(page, {
      name: 'CNYAmount',
      kind: 'currency',
      scale: '2',
    })
    expect(cny.options).toMatchObject({ currency: 'CNY', scale: 2 })
    await saveGridText(page, viewId, cny, '1234.50')
    await expect(page.getByText(/¥/).first()).toBeVisible()

    await openFieldPanel(page)
    const cnyActions = page.locator(
      `[data-field-id="${cny.id}"] details.field-actions`,
    )
    await cnyActions.locator('summary').click()
    await cnyActions
      .getByRole('button', { name: en.field.panel.edit, exact: true })
      .click()
    const edit = page.getByRole('dialog', {
      name: en.field.panel.editTitle,
      exact: true,
    })
    await edit
      .getByRole('textbox', { name: en.field.panel.currency, exact: true })
      .fill('USD')
    await edit
      .getByRole('button', { name: en.field.panel.save, exact: true })
      .click()
    await expect(edit).not.toBeVisible()
    let schema = await schemaOf(page, tableId)
    const usd = schema.fields.find((field) => field.id === cny.id)!
    expect(usd.options).toMatchObject({ currency: 'USD', scale: 2 })
    await expect(page.getByText(/\$1,234\.50/).first()).toBeVisible()

    await setDefault('EUR')
    const eur = await addField(page, {
      name: 'EURAmount',
      kind: 'currency',
      scale: '2',
    })
    expect(eur.options).toMatchObject({ currency: 'EUR', scale: 2 })
    schema = await schemaOf(page, tableId)
    expect(
      schema.fields.find((field) => field.id === cny.id)?.options,
    ).toMatchObject({
      currency: 'USD',
      scale: 2,
    })
    await saveGridText(page, viewId, eur, '2345.60')
    await expect(page.getByText(/€2,345\.60/).first()).toBeVisible()
  } finally {
    await restoreSettings(page, previous)
  }
})
