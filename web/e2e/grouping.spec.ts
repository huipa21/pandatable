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

test('AT-504 groups three levels with counts, sums, collapse state, and sticky mobile headers', async ({
  page,
}) => {
  test.setTimeout(180_000)
  await signIn(page)
  const stamp = Date.now()
  const create = async (path: string, data: unknown) => {
    const response = await page.request.post(path, { headers: headers(), data })
    expect(response.status()).toBe(201)
    return response.json()
  }
  const workspace = (await create('/api/v1/workspaces', {
    name: `Grouping ${stamp}`,
  })) as { workspace: { id: string } }
  const book = (await create(
    `/api/v1/workspaces/${workspace.workspace.id}/books`,
    { name: `Grouping book ${stamp}` },
  )) as { book: { id: string } }
  const schema = (await create(`/api/v1/books/${book.book.id}/tables`, {
    name: `Grouping table ${stamp}`,
  })) as TableSchema
  const tableId = schema.table.id
  const createField = async (name: string, uiType: string) => {
    const result = (await create(`/api/v1/tables/${tableId}/fields`, {
      name,
      uiType,
    })) as { field: TableSchema['fields'][number] }
    return result.field
  }
  const project = await createField('Project', 'text')
  const status = await createField('Status', 'text')
  const owner = await createField('Owner', 'text')
  const hours = await createField('Hours', 'integer')
  const values = [
    {
      name: 'North Alice 1',
      project: 'North',
      status: 'Open',
      owner: 'Alice',
      hours: 4,
    },
    {
      name: 'North Alice 2',
      project: 'North',
      status: 'Open',
      owner: 'Alice',
      hours: 5,
    },
    {
      name: 'North empty',
      project: 'North',
      status: 'Open',
      owner: null,
      hours: 3,
    },
    {
      name: 'North blocked',
      project: 'North',
      status: 'Blocked',
      owner: 'Bob',
      hours: 7,
    },
    {
      name: 'South 1',
      project: 'South',
      status: 'Open',
      owner: 'Bob',
      hours: 2,
    },
    {
      name: 'South 2',
      project: 'South',
      status: 'Open',
      owner: 'Bob',
      hours: 3,
    },
  ]
  const created = await page.request.post(`/api/v1/tables/${tableId}/rows`, {
    headers: headers(),
    data: {
      rows: values.map((value) => ({
        fields: {
          [schema.table.displayFieldId!]: value.name,
          [project.id]: value.project,
          [status.id]: value.status,
          [owner.id]: value.owner,
          [hours.id]: value.hours,
        },
      })),
    },
  })
  expect(created.status()).toBe(201)

  const view = schema.views[0]!
  const configured = await page.request.patch(`/api/v1/views/${view.id}`, {
    headers: { ...headers(), 'If-Match': String(view.version) },
    data: {
      config: {
        fields: view.config.fields,
        frozen: view.config.frozen,
        groupBy: [project.id, status.id, owner.id],
        aggregates: [{ fieldId: hours.id, aggregate: 'sum' }],
      },
    },
  })
  expect(configured.status()).toBe(200)
  const path = `/t/${tableId}/v/${view.id}`
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.goto(path)

  await expect(
    page.getByRole('button', { name: 'Expand Project: North', exact: false }),
  ).toContainText('North (4)')
  await expect(
    page.getByRole('button', { name: 'Expand Project: South', exact: false }),
  ).toContainText('South (2)')
  await expect(page.getByText('Hours Sum: 19')).toBeVisible()

  await page
    .getByRole('button', { name: 'Expand Project: North', exact: false })
    .click()
  await expect(page.getByText('Status: Open (3)')).toBeVisible()
  await expect(page.getByText('Hours Sum: 12')).toBeVisible()
  await expect(page.getByText('Status: Blocked (1)')).toBeVisible()

  await page
    .getByRole('button', { name: 'Expand Status: Open', exact: false })
    .click()
  await expect(page.getByText('Owner: Alice (2)')).toBeVisible()
  await expect(page.getByText('Owner: (empty) (1)')).toBeVisible()
  await expect(page.getByText('Hours Sum: 9')).toBeVisible()
  await expect(page.getByText('Hours Sum: 3')).toBeVisible()

  const leafRequest = page.waitForRequest((request) => {
    if (
      request.method() !== 'POST' ||
      new URL(request.url()).pathname !== `/api/v1/views/${view.id}/query`
    )
      return false
    const body = request.postDataJSON() as { groupPath?: unknown[] }
    return (
      body.groupPath?.length === 3 &&
      body.groupPath[0] === 'North' &&
      body.groupPath[1] === 'Open' &&
      body.groupPath[2] === null
    )
  })
  await page
    .getByRole('button', { name: 'Expand Owner: (empty)', exact: false })
    .click()
  expect((await leafRequest).postDataJSON()).toMatchObject({
    groupPath: ['North', 'Open', null],
  })
  await expect(page.getByText('North empty', { exact: true })).toBeVisible()
  await page
    .getByRole('button', { name: 'Collapse Owner: (empty)', exact: false })
    .click()
  await expect(page.getByText('North empty', { exact: true })).toHaveCount(0)
  await page
    .getByRole('button', { name: 'Collapse Project: North', exact: false })
    .click()
  await expect(page.getByText('Status: Open (3)')).toHaveCount(0)

  const me = (await (await page.request.get('/api/v1/auth/me')).json()) as {
    id: string
  }
  await page.evaluate(
    (key) => localStorage.removeItem(key),
    `pandatable:groups:${me.id}:${view.id}`,
  )
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto(path)
  const mobileGroups = page.locator('.mobile-group-header')
  await expect(mobileGroups).toHaveCount(2)
  await expect(mobileGroups.first().locator('..')).toHaveCSS(
    'position',
    'sticky',
  )
  await mobileGroups
    .first()
    .getByRole('button', { name: 'Expand Project: North', exact: false })
    .click()
  await expect(page.getByText('Status: Open (3)')).toBeVisible()
  await page
    .getByRole('button', { name: 'Expand Status: Open', exact: false })
    .click()
  await expect(page.getByText('Owner: (empty) (1)')).toBeVisible()
})
