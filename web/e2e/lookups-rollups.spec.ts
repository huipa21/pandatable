import { allowHTTPFailure, expect, test, paceLogin } from './fixtures'
import type { DataRow, TableSchema } from '../src/api/types'
import { setLocale } from './app-shell'
import { openFieldTypeActions } from './table-controls'
import en from '../src/i18n/en.json' with { type: 'json' }
import type { Browser, Page } from '@playwright/test'

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

async function signIn(
  page: Page,
  username = 'first-admin',
  password = 'Setup-password-123',
) {
  await paceLogin(page)
  await page.goto('/login')
  if ((await page.locator('html').getAttribute('lang')) === 'zh-CN')
    await setLocale(page, 'en')
  await page.getByLabel(en.auth.login, { exact: true }).fill(username)
  await page.getByLabel(en.auth.password, { exact: true }).fill(password)
  await page.getByRole('button', { name: en.auth.signIn, exact: true }).click()
  await expect(page).toHaveURL(/\/(?:home|w\/[^/]+|change-password)$/)
}

async function createTable(page: Page, bookId: string, name: string) {
  const response = await page.request.post(`/api/v1/books/${bookId}/tables`, {
    headers: headers(),
    data: { name },
  })
  expect(response.status()).toBe(201)
  return (await response.json()) as TableSchema
}

async function createField(
  page: Page,
  tableId: string,
  name: string,
  uiType: string,
) {
  const response = await page.request.post(`/api/v1/tables/${tableId}/fields`, {
    headers: headers(),
    data: { name, uiType, options: {} },
  })
  expect(response.status()).toBe(201)
  return (await response.json()) as {
    field: TableSchema['fields'][number]
    schema: TableSchema
  }
}

async function createRows(
  page: Page,
  tableId: string,
  rows: Record<string, unknown>[],
) {
  const response = await page.request.post(`/api/v1/tables/${tableId}/rows`, {
    headers: headers(),
    data: { rows: rows.map((fields) => ({ fields })) },
  })
  expect(response.status()).toBe(201)
  return ((await response.json()) as { rows: DataRow[] }).rows
}

async function addComputedFromUI(
  page: Page,
  schema: TableSchema,
  kind: 'lookup' | 'rollup',
  relationFieldId: string,
  targetFieldId: string,
) {
  await page.goto(`/t/${schema.table.id}/v/${schema.views[0]!.id}`)
  const panel = await openFieldTypeActions(page)
  await panel.locator(`[data-kind="${kind}"]`).click()
  const dialog = page.getByRole('dialog', {
    name: en.field.panel.computedTitle,
    exact: true,
  })
  await expect(dialog).toBeVisible()
  await dialog
    .getByLabel(en.field.panel.name, { exact: true })
    .fill(kind === 'lookup' ? 'Project owner' : 'Total hours')
  await dialog
    .getByRole('combobox', {
      name: en.field.panel.computedRelation,
      exact: true,
    })
    .selectOption(relationFieldId)
  await dialog
    .getByRole('combobox', {
      name: en.field.panel.computedTarget,
      exact: true,
    })
    .selectOption(targetFieldId)
  if (kind === 'rollup')
    await dialog
      .getByRole('combobox', {
        name: en.field.panel.computedAggregate,
        exact: true,
      })
      .selectOption('sum')
  await dialog
    .getByRole('button', { name: en.field.panel.create, exact: true })
    .click()
  await expect(dialog).not.toBeVisible()
}

async function queryRows(page: Page, viewId: string) {
  const response = await page.request.post(`/api/v1/views/${viewId}/query`, {
    headers: headers(),
    data: {},
  })
  expect(response.status()).toBe(200)
  return ((await response.json()) as { rows: DataRow[] }).rows
}

async function createReader(page: Page, browser: Browser, username: string) {
  const response = await page.request.post('/api/v1/admin/users', {
    headers: headers(),
    data: {
      username,
      displayName: 'Lookup reader',
      role: 'user',
      password: 'Temporary-password-123',
    },
  })
  expect(response.status()).toBe(201)
  const user = (await response.json()).user as { id: string }
  const context = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL!,
    locale: 'en-US',
  })
  const reader = await context.newPage()
  await signIn(reader, username, 'Temporary-password-123')
  if (new URL(reader.url()).pathname === '/change-password') {
    await reader
      .getByLabel(en.auth.currentPassword, { exact: true })
      .fill('Temporary-password-123')
    await reader
      .getByLabel(en.auth.newPassword, { exact: true })
      .fill('Reader-password-123')
    await reader
      .getByLabel(en.auth.confirmPassword, { exact: true })
      .fill('Reader-password-123')
    await reader
      .getByRole('button', { name: en.auth.changePassword, exact: true })
      .click()
    await expect(reader).toHaveURL(/\/login$/)
    await reader.getByLabel(en.auth.login, { exact: true }).fill(username)
    await reader
      .getByLabel(en.auth.password, { exact: true })
      .fill('Reader-password-123')
    await reader
      .getByRole('button', { name: en.auth.signIn, exact: true })
      .click()
    await expect(reader).toHaveURL(/\/home$/)
  }
  return { context, reader, user }
}

test('AT-407/408/411 lookup, rollups and cross-table masking', async ({
  page,
  browser,
  isMobile,
}) => {
  test.setTimeout(150_000)
  test.skip(isMobile, 'Computed-field grid coverage is desktop-only.')
  await page.setViewportSize({ width: 1440, height: 900 })
  await signIn(page)

  const stamp = Date.now()
  const workspaceResponse = await page.request.post('/api/v1/workspaces', {
    headers: headers(),
    data: { name: `Computed ${stamp}` },
  })
  expect(workspaceResponse.status()).toBe(201)
  const workspaceId = (
    (await workspaceResponse.json()) as { workspace: { id: string } }
  ).workspace.id
  const bookResponse = await page.request.post(
    `/api/v1/workspaces/${workspaceId}/books`,
    { headers: headers(), data: { name: 'Computed book' } },
  )
  expect(bookResponse.status()).toBe(201)
  const bookId = ((await bookResponse.json()) as { book: { id: string } }).book
    .id
  const projects = await createTable(page, bookId, 'Projects')
  const tasks = await createTable(page, bookId, 'Tasks')

  const ownerField = await createField(page, projects.table.id, 'Owner', 'text')
  const hoursField = await createField(page, tasks.table.id, 'Hours', 'integer')
  const dayField = await createField(page, tasks.table.id, 'Day', 'date')
  const relationResponse = await page.request.post(
    `/api/v1/tables/${tasks.table.id}/relations`,
    {
      headers: headers(),
      data: {
        name: 'Project',
        reverseName: 'Tasks',
        targetTableId: projects.table.id,
        kind: 'one_many',
        sourceSide: 'child',
        onDelete: 'set_null',
      },
    },
  )
  expect(relationResponse.status()).toBe(201)
  const relation = (await relationResponse.json()) as {
    relation: TableSchema['relations'][number]
    schema: TableSchema
  }
  const projectRows = await createRows(page, projects.table.id, [
    {
      [projects.table.displayFieldId!]: 'Panda',
      [ownerField.field.id]: 'Alice',
    },
  ])
  const taskRows = await createRows(page, tasks.table.id, [
    {
      [tasks.table.displayFieldId!]: 'Build A',
      [hoursField.field.id]: 4,
      [dayField.field.id]: '2026-01-02',
    },
    {
      [tasks.table.displayFieldId!]: 'Build B',
      [hoursField.field.id]: 6,
      [dayField.field.id]: '2026-01-05',
    },
  ])
  for (const row of taskRows) {
    const response = await page.request.post(
      `/api/v1/tables/${tasks.table.id}/rows/${row.id}/links/${relation.relation.childFieldId}`,
      { headers: headers(), data: { add: [projectRows[0]!.id] } },
    )
    expect(response.status()).toBe(200)
  }

  const taskSchemaResponse = await page.request.get(
    `/api/v1/tables/${tasks.table.id}/schema`,
  )
  const taskSchema = (await taskSchemaResponse.json()) as TableSchema
  await addComputedFromUI(
    page,
    taskSchema,
    'lookup',
    relation.relation.childFieldId,
    ownerField.field.id,
  )
  const projectSchemaResponse = await page.request.get(
    `/api/v1/tables/${projects.table.id}/schema`,
  )
  const projectSchema = (await projectSchemaResponse.json()) as TableSchema
  await addComputedFromUI(
    page,
    projectSchema,
    'rollup',
    relation.relation.parentFieldId,
    hoursField.field.id,
  )
  // The open grid may query mid-change and retry after a stale-schema 409.
  for (const schema of [projectSchema, taskSchema])
    allowHTTPFailure(
      page,
      'POST',
      `/api/v1/views/${schema.views[0]!.id}/query`,
      409,
    )

  const totals = [
    ['count', hoursField.field.id],
    ['avg', hoursField.field.id],
    ['min', hoursField.field.id],
    ['max', hoursField.field.id],
    ['earliest', dayField.field.id],
    ['latest', dayField.field.id],
    ['concat', tasks.table.displayFieldId!],
  ] as const
  for (const [aggregate, targetFieldId] of totals) {
    const response = await page.request.post(
      `/api/v1/tables/${projects.table.id}/fields`,
      {
        headers: headers(),
        data: {
          name: `AT ${aggregate}`,
          uiType: 'rollup',
          options: {
            relationFieldId: relation.relation.parentFieldId,
            targetFieldId,
            aggregate,
          },
        },
      },
    )
    expect(response.status()).toBe(201)
  }
  const finalProjectSchema = (await (
    await page.request.get(`/api/v1/tables/${projects.table.id}/schema`)
  ).json()) as TableSchema
  const projectQuery = await queryRows(page, finalProjectSchema.views[0]!.id)
  const projectRow = projectQuery[0]!
  const value = (name: string) => {
    const field = finalProjectSchema.fields.find((item) => item.name === name)
    if (!field) throw new Error(`Missing computed field ${name}`)
    return projectRow.fields[field.id]
  }
  expect(value('Total hours')).toBe(10)
  expect(value('AT count')).toBe(2)
  expect(value('AT avg')).toBe(5)
  expect(value('AT min')).toBe(4)
  expect(value('AT max')).toBe(6)
  expect(value('AT earliest')).toBe('2026-01-02')
  expect(value('AT latest')).toBe('2026-01-05')
  expect(value('AT concat')).toBe('Build A, Build B')

  const lookupField = taskSchema.fields.find((field) => field.uiType === 'link')
  expect(lookupField).toBeDefined()
  const finalTaskSchema = (await (
    await page.request.get(`/api/v1/tables/${tasks.table.id}/schema`)
  ).json()) as TableSchema
  const lookup = finalTaskSchema.fields.find(
    (field) => field.name === 'Project owner',
  )
  if (!lookup) throw new Error('Lookup field was not created')
  const taskQuery = await queryRows(page, finalTaskSchema.views[0]!.id)
  expect(taskQuery[0]!.fields[lookup.id]).toEqual(['Alice'])
  const patch = await page.request.patch(
    `/api/v1/tables/${projects.table.id}/rows/${projectRows[0]!.id}`,
    {
      headers: headers(),
      data: { fields: { [ownerField.field.id]: 'Bob' } },
    },
  )
  expect(patch.status()).toBe(200)
  expect(
    (await queryRows(page, finalTaskSchema.views[0]!.id))[0]!.fields[lookup.id],
  ).toEqual(['Bob'])

  const username = `lookup-${stamp}`
  const { context, reader, user } = await createReader(page, browser, username)
  try {
    const grant = await page.request.put('/api/v1/grants', {
      headers: headers(),
      data: {
        resourceType: 'table',
        resourceId: tasks.table.id,
        principalType: 'user',
        principalId: user.id,
        level: 'read',
      },
    })
    expect(grant.status()).toBe(201)
    await reader.goto(`/t/${tasks.table.id}/v/${finalTaskSchema.views[0]!.id}`)
    await expect(reader.locator('revo-grid')).toBeVisible()
    await expect(reader.getByText('🔒', { exact: true }).first()).toBeVisible()
    await expect(
      reader.getByText('Panda', { exact: true }).first(),
    ).toBeVisible()
    const masked = await queryRows(reader, finalTaskSchema.views[0]!.id)
    expect(masked[0]!.fields[lookup.id]).toBe('🔒')
  } finally {
    await context.close()
  }
})
