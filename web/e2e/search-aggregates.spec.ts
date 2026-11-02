import { expect, paceLogin, test } from './fixtures'
import { setLocale } from './app-shell'
import en from '../src/i18n/en.json' with { type: 'json' }
import type { TableSchema } from '../src/api/types'

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

async function signIn(page: import('@playwright/test').Page) {
  await paceLogin(page)
  await page.goto('/login')
  if ((await page.locator('html').getAttribute('lang')) === 'zh-CN')
    await setLocale(page, 'en')
  await page.getByLabel(en.auth.login, { exact: true }).fill('first-admin')
  await page
    .getByLabel(en.auth.password, { exact: true })
    .fill('Setup-password-123')
  await page.getByRole('button', { name: en.auth.signIn, exact: true }).click()
  await expect(page).toHaveURL(/\/(home|change-password)$/)
}

function aggregateValue(
  result: {
    aggregates: { fieldId: string; aggregate: string; value: unknown }[]
  },
  fieldId: string,
  aggregate: string,
) {
  const found = result.aggregates.find(
    (item) => item.fieldId === fieldId && item.aggregate === aggregate,
  )
  if (!found) throw new Error(`Missing ${aggregate} aggregate for ${fieldId}`)
  return found.value
}

test('AT-509/510 quick search and footer aggregates use visible filtered rows', async ({
  page,
}) => {
  test.setTimeout(150_000)
  test.skip(
    (page.viewportSize()?.width ?? 0) < 768,
    'production grid is desktop-only',
  )
  await signIn(page)

  const stamp = Date.now()
  const create = async (path: string, data: unknown) => {
    const response = await page.request.post(path, { headers: headers(), data })
    expect(response.status()).toBe(201)
    return response.json()
  }
  const workspace = (await create('/api/v1/workspaces', {
    name: `Search ${stamp}`,
  })) as { workspace: { id: string } }
  const book = (await create(
    `/api/v1/workspaces/${workspace.workspace.id}/books`,
    { name: `Search book ${stamp}` },
  )) as { book: { id: string } }
  const createdTable = (await create(`/api/v1/books/${book.book.id}/tables`, {
    name: `Search table ${stamp}`,
  })) as TableSchema
  const tableId = createdTable.table.id

  const createField = async (name: string, uiType: string) => {
    const result = (await create(`/api/v1/tables/${tableId}/fields`, {
      name,
      uiType,
    })) as { field: TableSchema['fields'][number] }
    return result.field
  }
  const note = await createField('Note', 'text')
  const amount = await createField('Amount', 'integer')
  const rate = await createField('Rate', 'integer')
  const counter = await createField('Counter', 'text')
  const privateField = await createField('Private', 'text')
  const schema = (await (
    await page.request.get(`/api/v1/tables/${tableId}/schema`)
  ).json()) as TableSchema
  const displayId = schema.table.displayFieldId!
  const view = schema.views[0]!

  const values = [
    { label: 'Acme Alpha', note: 'one', amount: 10, rate: 2, counter: 'x' },
    {
      label: 'Beta',
      note: '项目 Beta',
      amount: 20,
      rate: 4,
      counter: 'y',
      private: 'acme hidden',
    },
    {
      label: 'Gamma',
      note: '项目 Gamma',
      amount: 10,
      rate: null,
      counter: 'z',
    },
    { label: 'Empty', note: null, amount: 10, rate: null, counter: 'n' },
  ]
  const rows = await page.request.post(`/api/v1/tables/${tableId}/rows`, {
    headers: headers(),
    data: {
      rows: values.map((value) => ({
        fields: {
          [displayId]: value.label,
          [note.id]: value.note,
          [amount.id]: value.amount,
          [rate.id]: value.rate,
          [counter.id]: value.counter,
          [privateField.id]: value.private ?? null,
        },
      })),
    },
  })
  expect(rows.status()).toBe(201)

  const filter = { field: amount.id, cmp: 'eq', value: 10 }
  const config = await page.request.patch(`/api/v1/views/${view.id}`, {
    headers: { ...headers(), 'If-Match': String(view.version) },
    data: {
      config: {
        fields: schema.fields.map((field) => ({
          id: field.id,
          visible: !field.isSystem && field.id !== privateField.id,
        })),
        frozen: 0,
        filter,
        aggregates: [
          { fieldId: amount.id, aggregate: 'sum' },
          { fieldId: rate.id, aggregate: 'avg' },
          { fieldId: note.id, aggregate: 'count_empty' },
          { fieldId: displayId, aggregate: 'unique' },
          { fieldId: counter.id, aggregate: 'count' },
        ],
      },
    },
  })
  expect(config.status()).toBe(200)
  await page.setViewportSize({ width: 1280, height: 900 })

  const waitForAggregates = (search?: string) =>
    page.waitForResponse((response) => {
      const request = response.request()
      if (
        request.method() !== 'POST' ||
        new URL(response.url()).pathname !==
          `/api/v1/views/${view.id}/aggregates`
      )
        return false
      const body = request.postDataJSON() as { search?: string }
      return body.search === search
    })
  const initialAggregates = waitForAggregates()
  await page.goto(`/t/${tableId}/v/${view.id}`)
  const initial = (await (await initialAggregates).json()) as {
    aggregates: { fieldId: string; aggregate: string; value: unknown }[]
  }
  expect(aggregateValue(initial, amount.id, 'sum')).toBe('30')
  expect(aggregateValue(initial, rate.id, 'avg')).toBe('2')
  expect(aggregateValue(initial, note.id, 'count_empty')).toBe(1)
  expect(aggregateValue(initial, displayId, 'unique')).toBe(3)
  expect(aggregateValue(initial, counter.id, 'count')).toBe(3)

  const acmeQuery = page.waitForResponse((response) => {
    const request = response.request()
    if (
      request.method() !== 'POST' ||
      new URL(response.url()).pathname !== `/api/v1/views/${view.id}/query`
    )
      return false
    return (request.postDataJSON() as { search?: string }).search === 'acme'
  })
  const acmeAggregates = waitForAggregates('acme')
  await page.getByRole('searchbox', { name: en.grid.quickSearch }).fill('acme')
  const acmeResponse = await acmeQuery
  const acmeRows = (await acmeResponse.json()) as {
    rows: { fields: Record<string, unknown> }[]
  }
  expect(acmeRows.rows.map((row) => row.fields[displayId])).toEqual([
    'Acme Alpha',
  ])
  const acmeFooter = (await (await acmeAggregates).json()) as {
    aggregates: { fieldId: string; aggregate: string; value: unknown }[]
  }
  expect(aggregateValue(acmeFooter, amount.id, 'sum')).toBe('10')
  expect(aggregateValue(acmeFooter, rate.id, 'avg')).toBe('2')
  expect(aggregateValue(acmeFooter, note.id, 'count_empty')).toBe(0)
  expect(aggregateValue(acmeFooter, displayId, 'unique')).toBe(1)
  expect(aggregateValue(acmeFooter, counter.id, 'count')).toBe(1)

  const chineseQuery = page.waitForResponse((response) => {
    const request = response.request()
    return (
      request.method() === 'POST' &&
      new URL(response.url()).pathname === `/api/v1/views/${view.id}/query` &&
      (request.postDataJSON() as { search?: string }).search === '项目'
    )
  })
  await page.getByRole('searchbox', { name: en.grid.quickSearch }).fill('项目')
  const chineseRows = (await (await chineseQuery).json()) as {
    rows: { fields: Record<string, unknown> }[]
  }
  expect(chineseRows.rows.map((row) => row.fields[displayId])).toEqual([
    'Gamma',
  ])

  const amountAggregate = page.getByRole('combobox', {
    name: en.grid.aggregateForField.replace('{field}', 'Amount'),
  })
  await page
    .locator('summary')
    .filter({ hasText: en.grid.footerAggregates })
    .click()
  const changedAggregate = waitForAggregates('项目')
  await amountAggregate.selectOption('avg')
  await changedAggregate
  await expect(page.locator('.grid-aggregate-footer')).toContainText('Average')
})
