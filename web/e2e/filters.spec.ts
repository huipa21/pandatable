import { expect, paceLogin, test } from './fixtures'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }
import type { FilterNode, TableSchema } from '../src/api/types'
import type { Page } from '@playwright/test'

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

const originalTimezones = new WeakMap<Page, string | null>()

test.afterEach(async ({ page }) => {
  const timezone = originalTimezones.get(page)
  if (timezone === undefined) return
  const response = await page.request.patch('/api/v1/auth/me', {
    headers: headers(),
    data: { timezone },
  })
  expect(response.status()).toBe(200)
  originalTimezones.delete(page)
})

async function signIn(page: Page) {
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
  await expect(page).toHaveURL(/\/(home|change-password)$/)
}

function dateInZone(timeZone: string) {
  return new Intl.DateTimeFormat('en-CA', {
    timeZone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).format(new Date())
}

function offsetDate(date: string, days: number) {
  const [year, month, day] = date.split('-').map(Number)
  const shifted = new Date(Date.UTC(year!, month! - 1, day! + days))
  return shifted.toISOString().slice(0, 10)
}

test('AT-501/502 nested filters use the actor timezone and stay transient', async ({
  page,
}) => {
  test.setTimeout(150_000)
  await signIn(page)

  const stamp = Date.now()
  const create = async (path: string, data: unknown) => {
    const response = await page.request.post(path, { headers: headers(), data })
    expect(response.status()).toBe(201)
    return response.json()
  }
  const workspace = (await create('/api/v1/workspaces', {
    name: `Filter ${stamp}`,
  })) as { workspace: { id: string } }
  const book = (await create(
    `/api/v1/workspaces/${workspace.workspace.id}/books`,
    { name: `Filter book ${stamp}` },
  )) as { book: { id: string } }
  const schema = (await create(`/api/v1/books/${book.book.id}/tables`, {
    name: `Filter table ${stamp}`,
  })) as TableSchema
  const tableId = schema.table.id

  const createField = async (
    name: string,
    uiType: string,
    options: Record<string, unknown> = {},
  ) => {
    const result = (await create(`/api/v1/tables/${tableId}/fields`, {
      name,
      uiType,
      options,
    })) as { field: TableSchema['fields'][number] }
    return result.field
  }
  const status = await createField('Status', 'single_select', {
    choices: [
      { id: 'open', label: 'Open', color: 'green' },
      { id: 'blocked', label: 'Blocked', color: 'red' },
      { id: 'closed', label: 'Closed', color: 'gray' },
    ],
  })
  const due = await createField('Due', 'date')
  const owner = await createField('Owner', 'user')
  const meResponse = await page.request.get('/api/v1/auth/me')
  expect(meResponse.status()).toBe(200)
  const me = (await meResponse.json()) as {
    id: string
    timezone: string | null
  }
  expect(me.timezone === null || typeof me.timezone === 'string').toBe(true)
  originalTimezones.set(page, me.timezone)

  const utcToday = new Date().toISOString().slice(0, 10)
  const timeZone = ['Pacific/Kiritimati', 'Pacific/Pago_Pago'].find(
    (zone) => dateInZone(zone) !== utcToday,
  )
  if (!timeZone)
    throw new Error('No timezone boundary is available for this test')
  const timezoneUpdate = await page.request.patch('/api/v1/auth/me', {
    headers: headers(),
    data: { timezone: timeZone },
  })
  expect(timezoneUpdate.status()).toBe(200)

  const localToday = dateInZone(timeZone)
  const otherUserResponse = await page.request.post('/api/v1/admin/users', {
    headers: headers(),
    data: {
      username: `filter-other-${stamp}`,
      displayName: 'Filter other',
      role: 'user',
      password: 'Temporary-password-123',
    },
  })
  expect(otherUserResponse.status()).toBe(201)
  const otherUser = (
    (await otherUserResponse.json()) as { user: { id: string } }
  ).user.id
  const values = [
    {
      name: 'Yesterday',
      status: 'Open',
      due: offsetDate(localToday, -1),
      owner: otherUser,
    },
    {
      name: 'Tomorrow owned',
      status: 'Blocked',
      due: offsetDate(localToday, 1),
      owner: me.id,
    },
    {
      name: 'Closed',
      status: 'Closed',
      due: offsetDate(localToday, -1),
      owner: me.id,
    },
    {
      name: 'Tomorrow',
      status: 'Open',
      due: offsetDate(localToday, 1),
      owner: otherUser,
    },
    { name: 'UTC boundary', status: 'Open', due: utcToday, owner: otherUser },
    {
      name: 'Local boundary',
      status: 'Open',
      due: localToday,
      owner: otherUser,
    },
  ]
  const rows = await page.request.post(`/api/v1/tables/${tableId}/rows`, {
    headers: headers(),
    data: {
      rows: values.map((value) => ({
        fields: {
          [schema.table.displayFieldId!]: value.name,
          [status.id]: value.status,
          [due.id]: value.due,
          [owner.id]: value.owner,
        },
      })),
    },
  })
  expect(rows.status()).toBe(201)

  await page.goto(`/t/${tableId}/v/${schema.views[0]!.id}`)
  const configBeforeFilter = (await (
    await page.request.get(`/api/v1/tables/${tableId}/schema`)
  ).json()) as TableSchema
  await page.getByText(en.grid.filter, { exact: true }).click()
  const root = page.locator('.filter-group').first()
  await root
    .getByRole('button', { name: en.filter.addCondition, exact: true })
    .click()
  const statusCondition = root.locator('.filter-item').first()
  await statusCondition.locator('select').nth(0).selectOption(status.id)
  await statusCondition.locator('select').nth(1).selectOption('is_any_of')
  await statusCondition
    .locator('select')
    .nth(2)
    .selectOption(['Open', 'Blocked'])

  await root
    .getByRole('button', { name: en.filter.addGroup, exact: true })
    .click()
  const nested = page.locator('.filter-group').nth(1)
  await nested.locator('select').nth(0).selectOption('or')
  const dueCondition = nested.locator(':scope > .filter-item').first()
  await dueCondition.locator('select').nth(0).selectOption(due.id)
  await dueCondition.locator('select').nth(1).selectOption('before')
  await dueCondition.locator('select').nth(2).selectOption('today')

  await nested
    .getByRole('button', { name: en.filter.addCondition, exact: true })
    .click()
  const ownerCondition = nested.locator(':scope > .filter-item').nth(1)
  await ownerCondition.locator('select').nth(0).selectOption(owner.id)
  await ownerCondition.locator('select').nth(2).selectOption('me')

  const queryResponse = page.waitForResponse((response) => {
    const request = response.request()
    if (
      request.method() !== 'POST' ||
      new URL(response.url()).pathname !==
        `/api/v1/views/${schema.views[0]!.id}/query`
    )
      return false
    return Boolean((request.postDataJSON() as { filter?: FilterNode }).filter)
  })
  await page.getByRole('button', { name: en.filter.apply, exact: true }).click()
  const response = await queryResponse
  expect(response.status()).toBe(200)
  const request = response.request().postDataJSON() as {
    filter: FilterNode
  }
  expect(request.filter).toMatchObject({
    op: 'and',
    items: [
      { field: status.id, cmp: 'is_any_of', value: ['Open', 'Blocked'] },
      {
        op: 'or',
        items: [
          { field: due.id, cmp: 'before', value: { rel: 'today' } },
          { field: owner.id, cmp: 'is', value: { rel: 'me' } },
        ],
      },
    ],
  })
  const result = (await response.json()) as {
    rows: { fields: Record<string, unknown> }[]
  }
  const actual = result.rows
    .map((row) => row.fields[schema.table.displayFieldId!])
    .sort()
  const expected = ['Yesterday', 'Tomorrow owned']
  if (utcToday < localToday) expected.push('UTC boundary')
  expect(actual).toEqual(expected.sort())

  const savedSchema = (await (
    await page.request.get(`/api/v1/tables/${tableId}/schema`)
  ).json()) as TableSchema
  expect(savedSchema.views[0]!.config).toEqual(
    configBeforeFilter.views[0]!.config,
  )
})
