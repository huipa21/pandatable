import { execFileSync } from 'node:child_process'
import { readFile } from 'node:fs/promises'
import path from 'node:path'
import type { Browser, Locator, Page } from '@playwright/test'
import {
  allowHTTPFailure,
  expect,
  expectBrowserClean,
  observeBrowser,
  paceLogin,
  test,
} from './fixtures'
import { setLocale } from './app-shell'
import { openExportMenu } from './table-controls'
import en from '../src/i18n/en.json' with { type: 'json' }
import type {
  DataRow,
  ImportJob,
  TableField,
  TableSchema,
  ViewMutation,
} from '../src/api/types'

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

async function loginOwner(page: Page) {
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

async function create<T>(page: Page, url: string, data: unknown): Promise<T> {
  const response = await page.request.post(url, { headers: headers(), data })
  expect(response.status(), await response.text()).toBe(201)
  return (await response.json()) as T
}

async function makeBook(page: Page, suffix: string) {
  const workspace = await create<{ workspace: { id: string } }>(
    page,
    '/api/v1/workspaces',
    { name: `Import ${suffix}` },
  )
  const book = await create<{ book: { id: string } }>(
    page,
    `/api/v1/workspaces/${workspace.workspace.id}/books`,
    { name: `Book ${suffix}` },
  )
  return { workspaceId: workspace.workspace.id, bookId: book.book.id }
}

async function makeTable(page: Page, bookId: string, name: string) {
  return create<TableSchema>(page, `/api/v1/books/${bookId}/tables`, { name })
}

async function makeField(
  page: Page,
  tableId: string,
  input: Record<string, unknown>,
) {
  return (
    await create<{ field: TableField }>(
      page,
      `/api/v1/tables/${tableId}/fields`,
      input,
    )
  ).field
}

async function makeRows(
  page: Page,
  tableId: string,
  fields: Record<string, unknown>[],
) {
  return (
    await create<{ rows: DataRow[] }>(page, `/api/v1/tables/${tableId}/rows`, {
      rows: fields.map((item) => ({ fields: item })),
    })
  ).rows
}

async function doneJob(page: Page) {
  return page.waitForResponse(
    async (response) => {
      if (
        response.request().method() !== 'GET' ||
        !/\/api\/v1\/imports\/[^/]+$/.test(new URL(response.url()).pathname) ||
        response.status() !== 200
      )
        return false
      const body = (await response.json()) as ImportJob
      return body.status === 'done' || body.status === 'failed'
    },
    { timeout: 240_000 },
  )
}

async function openBookImport(page: Page, workspaceId: string, bookId: string) {
  await page.goto(`/w/${workspaceId}/b/${bookId}`)
  await page
    .getByRole('button', { name: en.import.newTable, exact: true })
    .click()
  return page.locator('dialog.import-dialog')
}

async function closeImport(page: Page, dialog: Locator) {
  const discarded = page.waitForResponse((response) => {
    const path = new URL(response.url()).pathname
    return (
      response.request().method() === 'DELETE' &&
      /^\/api\/v1\/imports\/[^/]+$/.test(path)
    )
  })
  await dialog
    .getByRole('button', { name: en.import.close, exact: true })
    .click()
  const response = await discarded
  expect(response.status(), await response.text()).toBe(200)
  await expect(dialog).toBeHidden()
}

async function participant(
  browser: Browser,
  username: string,
  mobile: boolean,
) {
  const context = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL,
    locale: 'en-US',
    viewport: mobile
      ? { width: 390, height: 844 }
      : { width: 1280, height: 900 },
  })
  await paceLogin()
  expect(
    (
      await context.request.post('/api/v1/auth/login', {
        headers: headers(),
        data: { login: username, password: 'Temporary-password-123' },
      })
    ).status(),
  ).toBe(200)
  expect(
    (
      await context.request.post('/api/v1/auth/password', {
        headers: headers(),
        data: { current: 'Temporary-password-123', new: 'Import-password-123' },
      })
    ).status(),
  ).toBe(200)
  await paceLogin()
  expect(
    (
      await context.request.post('/api/v1/auth/login', {
        headers: headers(),
        data: { login: username, password: 'Import-password-123' },
      })
    ).status(),
  ).toBe(200)
  const page = await context.newPage()
  return { context, page, diagnostics: await observeBrowser(page) }
}

function gbkFixture() {
  const parts: Buffer[] = [Buffer.from('Number,Day,Stage,Details\n', 'ascii')]
  const chinese = Buffer.from([0xd6, 0xd0, 0xce, 0xc4])
  for (let index = 0; index < 10_000; index++) {
    const number = index === 4321 ? 'bad-number' : String(index + 1)
    parts.push(
      Buffer.from(
        `${number},2026-01-${String((index % 28) + 1).padStart(2, '0')},${index % 2 ? 'Open' : 'Closed'},`,
        'ascii',
      ),
      chinese,
      Buffer.from(`-${index}\n`, 'ascii'),
    )
  }
  return Buffer.concat(parts)
}

test('AT-702–704 package restoration, GBK wizard and XLSX append permissions', async ({
  page,
  browser,
}, testInfo) => {
  test.setTimeout(300_000)
  await loginOwner(page)
  const suffix = `${testInfo.project.name}-${Date.now()}`
  const { workspaceId, bookId } = await makeBook(page, suffix)
  const projects = await makeTable(page, bookId, 'Projects')
  const source = await makeTable(page, bookId, 'Tasks')
  const amount = await makeField(page, source.table.id, {
    name: 'Amount',
    uiType: 'decimal',
    options: { precision: 12, scale: 2 },
    isRequired: true,
    default: 7,
  })
  const tags = await makeField(page, source.table.id, {
    name: 'Tags',
    uiType: 'multi_select',
    options: {
      choices: [
        { id: 'a', label: '甲', color: 'gray' },
        { id: 'b', label: '乙', color: 'gray' },
      ],
    },
  })
  const list = await makeField(page, source.table.id, {
    name: 'List',
    uiType: 'text_array',
  })
  const when = await makeField(page, source.table.id, {
    name: 'When',
    uiType: 'datetime',
  })
  const formula = await makeField(page, source.table.id, {
    name: 'Double',
    uiType: 'formula',
    options: { expr: '{Amount} * 2' },
  })
  const relation = await create<{
    relation: TableSchema['relations'][number]
  }>(page, `/api/v1/tables/${source.table.id}/relations`, {
    name: 'Project',
    reverseName: 'Tasks',
    targetTableId: projects.table.id,
    kind: 'one_many',
    sourceSide: 'child',
    onDelete: 'set_null',
  })
  const projectCount = await makeField(page, source.table.id, {
    name: 'Project count',
    uiType: 'rollup',
    options: {
      relationFieldId: relation.relation.childFieldId,
      targetFieldId: projects.table.displayFieldId,
      aggregate: 'count',
    },
  })
  const doubledCount = await makeField(page, source.table.id, {
    name: 'Doubled count',
    uiType: 'formula',
    options: { expr: '{Project count} * 2' },
  })
  await create(page, `/api/v1/tables/${source.table.id}/views`, {
    name: 'Second grid',
    scope: 'shared',
  })
  const computedView = (
    await create<ViewMutation>(
      page,
      `/api/v1/tables/${source.table.id}/views`,
      {
        name: 'Computed filter',
        scope: 'shared',
      },
    )
  ).view
  const configuredView = await page.request.patch(
    `/api/v1/views/${computedView.id}`,
    {
      headers: { ...headers(), 'If-Match': String(computedView.version) },
      data: {
        config: {
          ...computedView.config,
          filter: { field: projectCount.id, cmp: 'gte', value: 1 },
        },
      },
    },
  )
  expect(configuredView.status()).toBe(200)
  const projectRow = (
    await makeRows(page, projects.table.id, [
      { [projects.table.displayFieldId!]: '项目, A' },
    ])
  )[0]!
  const sourceRow = (
    await makeRows(page, source.table.id, [
      {
        [source.table.displayFieldId!]: '中文任务',
        [amount.id]: '12.50',
        [tags.id]: ['甲', '乙'],
        [list.id]: ['first, second', ' spaced '],
        [when.id]: '2026-10-24T03:00:00.123456Z',
      },
    ])
  )[0]!
  const linked = await page.request.post(
    `/api/v1/tables/${source.table.id}/rows/${sourceRow.id}/links/${relation.relation.childFieldId}`,
    { headers: headers(), data: { add: [projectRow.id] } },
  )
  expect(linked.status()).toBe(200)
  const originalSchema = (await (
    await page.request.get(`/api/v1/tables/${source.table.id}/schema`)
  ).json()) as TableSchema
  const originalRow = (await (
    await page.request.get(
      `/api/v1/tables/${source.table.id}/rows/${sourceRow.id}`,
    )
  ).json()) as DataRow

  await page.goto(`/t/${source.table.id}/v/${source.views[0]!.id}`)
  await openExportMenu(page)
  const download = page.waitForEvent('download')
  await page
    .getByRole('button', { name: en.export.package, exact: true })
    .click()
  const packageFile = await download
  expect(await packageFile.failure()).toBeNull()
  const packageBytes = await readFile((await packageFile.path())!)
  const dialog = await openBookImport(page, workspaceId, bookId)
  await dialog.locator('input[type=file]').setInputFiles({
    name: 'package.xlsx',
    mimeType:
      'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    buffer: packageBytes,
  })
  await dialog.getByRole('button', { name: en.import.analyze }).click()
  await expect(dialog.getByText(en.import.packageNotice)).toBeVisible()
  await dialog.getByLabel(en.import.tableName).fill('Restored tasks')
  await dialog
    .getByLabel(en.import.relink.replace('{table}', 'Projects'))
    .check()
  await expect(dialog.getByText(en.import.linkDependencyWarning)).toBeHidden()
  const packageFinished = doneJob(page)
  await dialog.getByRole('button', { name: en.import.start }).click()
  const packageResult = (await (await packageFinished).json()) as ImportJob
  expect(packageResult).toMatchObject({
    status: 'done',
    result: { rows: 1, invalidCells: 0 },
  })
  const restoredId = packageResult.result.tableId!
  const restored = (await (
    await page.request.get(`/api/v1/tables/${restoredId}/schema`)
  ).json()) as TableSchema
  const restoredRow = (await (
    await page.request.get(`/api/v1/tables/${restoredId}/rows/${sourceRow.id}`)
  ).json()) as DataRow
  expect(restored.views.map((view) => view.name).sort()).toEqual(
    originalSchema.views
      .filter((view) => view.scope === 'shared')
      .map((view) => view.name)
      .sort(),
  )
  for (const field of [amount, tags, list, when]) {
    const imported = restored.fields.find((item) => item.name === field.name)!
    expect(imported).toMatchObject({
      uiType: field.uiType,
      options: field.options,
      isRequired: field.isRequired,
      isUnique: field.isUnique,
      default: field.default,
    })
    expect(restoredRow.fields[imported.id]).toEqual(
      originalRow.fields[field.id],
    )
  }
  const importedFormula = restored.fields.find(
    (field) => field.name === 'Double',
  )!
  expect(importedFormula.uiType).toBe('formula')
  expect(restoredRow.fields[importedFormula.id]).toEqual(
    originalRow.fields[formula.id],
  )
  const importedLink = restored.fields.find(
    (field) => field.name === 'Project',
  )!
  expect(importedLink.uiType).toBe('link')
  expect(restoredRow.fields[importedLink.id]).toEqual(
    originalRow.fields[relation.relation.childFieldId],
  )
  for (const field of [projectCount, doubledCount]) {
    const imported = restored.fields.find((item) => item.name === field.name)!
    expect(restoredRow.fields[imported.id]).toEqual(
      originalRow.fields[field.id],
    )
  }
  await closeImport(page, dialog)

  const textFallback = await openBookImport(page, workspaceId, bookId)
  await textFallback.locator('input[type=file]').setInputFiles({
    name: 'package.xlsx',
    mimeType:
      'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    buffer: packageBytes,
  })
  await textFallback.getByRole('button', { name: en.import.analyze }).click()
  await expect(
    textFallback.getByText(en.import.linkDependencyWarning),
  ).toBeVisible()
  await textFallback.getByLabel(en.import.tableName).fill('Text fallback tasks')
  const fallbackFinished = doneJob(page)
  await textFallback.getByRole('button', { name: en.import.start }).click()
  const fallbackResult = (await (await fallbackFinished).json()) as ImportJob
  expect(fallbackResult).toMatchObject({
    status: 'done',
    result: {
      rows: 1,
      convertedLinks: ['Project'],
      unavailableViews: ['Computed filter'],
    },
  })
  await expect(
    textFallback.getByText(
      en.import.unavailableViews.replace('{views}', 'Computed filter'),
    ),
  ).toBeVisible()
  const fallbackId = fallbackResult.result.tableId!
  const fallbackSchema = (await (
    await page.request.get(`/api/v1/tables/${fallbackId}/schema`)
  ).json()) as TableSchema
  const fallbackRow = (await (
    await page.request.get(`/api/v1/tables/${fallbackId}/rows/${sourceRow.id}`)
  ).json()) as DataRow
  for (const field of [projectCount, doubledCount]) {
    const imported = fallbackSchema.fields.find(
      (item) => item.name === field.name,
    )!
    expect(imported).toMatchObject({
      options: { error: 'dependency_missing' },
    })
    expect(fallbackRow.fields[imported.id]).toBe('#ERROR')
  }
  const fallbackView = fallbackSchema.views.find(
    (view) => view.name === 'Computed filter',
  )!
  const fallbackCount = fallbackSchema.fields.find(
    (field) => field.name === 'Project count',
  )!
  expect(fallbackView.config.filter).toEqual({
    field: fallbackCount.id,
    cmp: 'gte',
    value: 1,
  })
  const unavailableQuery = await page.request.post(
    `/api/v1/views/${fallbackView.id}/query`,
    { headers: headers(), data: { filter: fallbackView.config.filter } },
  )
  expect(unavailableQuery.status()).toBe(400)
  await closeImport(page, textFallback)
  const queryPath = `/api/v1/views/${fallbackView.id}/query`
  allowHTTPFailure(page, 'POST', queryPath, 400)
  const failedViewQuery = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname === queryPath &&
      response.request().method() === 'POST' &&
      response.status() === 400,
  )
  await page.goto(`/t/${fallbackId}/v/${fallbackView.id}`)
  await failedViewQuery
  await page.locator('.grid-filter-trigger, .view-filter > summary').click()
  const filter = page.locator('.filter-group').first()
  await expect(filter.locator('.filter-item')).toContainText('Project count')
  await filter
    .getByRole('button', { name: en.filter.clear, exact: true })
    .click()
  const locallyRepairedQuery = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname === queryPath &&
      response.request().method() === 'POST' &&
      response.status() === 200,
  )
  await page.getByRole('button', { name: en.filter.apply, exact: true }).click()
  await locallyRepairedQuery
  const repairedView = await page.request.patch(
    `/api/v1/views/${fallbackView.id}`,
    {
      headers: { ...headers(), 'If-Match': String(fallbackView.version) },
      data: { config: { ...fallbackView.config, filter: null } },
    },
  )
  expect(repairedView.status()).toBe(200)
  const repairedQuery = await page.request.post(
    `/api/v1/views/${fallbackView.id}/query`,
    { headers: headers(), data: {} },
  )
  expect(repairedQuery.status()).toBe(200)

  await page.goto(`/t/${restoredId}/v/${packageResult.result.viewId!}`)
  await page.getByRole('button', { name: en.import.append }).click()
  const packageAppend = page.locator('dialog.import-dialog')
  await packageAppend.locator('input[type=file]').setInputFiles({
    name: 'package.xlsx',
    mimeType:
      'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    buffer: packageBytes,
  })
  await packageAppend.getByRole('button', { name: en.import.analyze }).click()
  await expect(
    packageAppend.getByText(en.import.packageAppendNotice),
  ).toBeVisible()
  await expect(
    packageAppend.getByText(en.import.packageDataWarning),
  ).toBeVisible()
  await expect(
    packageAppend.getByLabel(en.import.targetField.replace('{name}', 'Amount')),
  ).toHaveValue(restored.fields.find((field) => field.name === 'Amount')!.id)
  await closeImport(page, packageAppend)

  const gbk = await openBookImport(page, workspaceId, bookId)
  await gbk.locator('input[type=file]').setInputFiles({
    name: 'gbk.csv',
    mimeType: 'text/csv',
    buffer: gbkFixture(),
  })
  await gbk.getByRole('button', { name: en.import.analyze }).click()
  await expect(
    gbk.getByText(en.import.encoding.replace('{encoding}', 'gbk')),
  ).toBeVisible()
  for (const [field, type] of [
    ['Number', 'integer'],
    ['Day', 'date'],
    ['Stage', 'single_select'],
    ['Details', 'text'],
  ] as const) {
    await expect(
      gbk.getByLabel(en.import.columnType.replace('{name}', field)),
    ).toHaveValue(type)
  }
  await gbk.getByLabel(en.import.tableName).fill('GBK imported')
  await gbk
    .getByLabel(en.import.columnName.replace('{name}', 'Details'))
    .fill('Notes')
  await gbk
    .getByLabel(en.import.columnType.replace('{name}', 'Details'))
    .selectOption('long_text')
  const gbkFinished = doneJob(page)
  await gbk.getByRole('button', { name: en.import.start }).click()
  const gbkResult = (await (await gbkFinished).json()) as ImportJob
  expect(gbkResult).toMatchObject({
    status: 'done',
    result: { rows: 10_000, invalidCells: 1 },
    hasReport: true,
  })
  const reportDownload = page.waitForEvent('download')
  await gbk.getByRole('link', { name: en.import.downloadReport }).click()
  const report = await reportDownload
  expect(await report.failure()).toBeNull()
  expect((await readFile((await report.path())!)).toString('utf8')).toContain(
    '4323,Number,bad-number',
  )
  await closeImport(page, gbk)

  const destination = await makeTable(page, bookId, 'Append destination')
  const amountField = await makeField(page, destination.table.id, {
    name: 'Amount',
    uiType: 'integer',
    options: { size: 'big' },
  })
  const username = `import-${Date.now()}`
  const user = await create<{ user: { id: string } }>(
    page,
    '/api/v1/admin/users',
    {
      username,
      displayName: username,
      role: 'user',
      password: 'Temporary-password-123',
    },
  )
  const grant = (level: 'read' | 'write') =>
    page.request.put('/api/v1/grants', {
      headers: headers(),
      data: {
        resourceType: 'table',
        resourceId: destination.table.id,
        principalType: 'user',
        principalId: user.user.id,
        level,
      },
    })
  expect((await grant('read')).status()).toBe(201)
  const collaborator = await participant(
    browser,
    username,
    testInfo.project.name.includes('mobile'),
  )
  try {
    const viewPath = `/t/${destination.table.id}/v/${destination.views[0]!.id}`
    await collaborator.page.goto(viewPath)
    await expect(
      collaborator.page.getByRole('button', { name: en.import.append }),
    ).toHaveCount(0)
    await expect(collaborator.page.locator('.visibility-menu')).toHaveCount(0)
    await openExportMenu(collaborator.page)
    const readerDownload = collaborator.page.waitForEvent('download')
    await collaborator.page
      .getByRole('button', { name: en.export.package, exact: true })
      .click()
    expect(await (await readerDownload).failure()).toBeNull()
    const plainXlsx = await readFile(
      path.resolve('e2e', 'data', 'plain-import.xlsx'),
    )
    const denied = await collaborator.context.request.post(
      `/api/v1/tables/${destination.table.id}/imports/analyze`,
      {
        headers: headers(),
        multipart: {
          file: {
            name: 'plain-import.xlsx',
            mimeType:
              'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
            buffer: plainXlsx,
          },
        },
      },
    )
    expect(denied.status()).toBe(403)
    expect([200, 201]).toContain((await grant('write')).status())
    await collaborator.page.reload()
    await collaborator.page
      .getByRole('button', { name: en.import.append })
      .click()
    const append = collaborator.page.locator('dialog.import-dialog')
    await append.locator('input[type=file]').setInputFiles({
      name: 'plain-import.xlsx',
      mimeType:
        'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
      buffer: plainXlsx,
    })
    await append.getByRole('button', { name: en.import.analyze }).click()
    await expect(
      append.getByLabel(en.import.targetField.replace('{name}', 'Name')),
    ).toHaveValue(destination.table.displayFieldId!)
    await append
      .getByLabel(en.import.targetField.replace('{name}', 'Amount'))
      .selectOption(amountField.id)
    const appendFinished = doneJob(collaborator.page)
    await append.getByRole('button', { name: en.import.start }).click()
    const appendResult = (await (await appendFinished).json()) as ImportJob
    expect(appendResult).toMatchObject({
      status: 'done',
      result: { rows: 2, invalidCells: 1 },
      hasReport: true,
    })
    await expect(
      append.getByRole('link', { name: en.import.downloadReport }),
    ).toBeVisible()
    await closeImport(collaborator.page, append)
    expectBrowserClean(collaborator.page, collaborator.diagnostics)
  } finally {
    await collaborator.context.close()
  }
})

function appHighWaterRSS(project: string): number {
  const engine = process.env.PT_ENGINE ?? 'podman'
  const id = execFileSync(
    engine,
    [
      'ps',
      '--filter',
      `label=com.docker.compose.project=${project}`,
      '--filter',
      'label=com.docker.compose.service=app',
      '--format',
      '{{.ID}}',
    ],
    { encoding: 'utf8' },
  ).trim()
  expect(id).toMatch(/^[a-f0-9]{12,64}$/i)
  const status = execFileSync(engine, ['exec', id, 'cat', '/proc/1/status'], {
    encoding: 'utf8',
  })
  const match = /^VmHWM:\s+(\d+)\s+kB$/m.exec(status)
  expect(match, 'Linux app process high-water RSS').not.toBeNull()
  return Number(match![1]) * 1024
}

test('AT-705 imports 200,000 CSV rows with app high-water RSS below 300 MB', async ({
  page,
}, testInfo) => {
  test.skip(
    testInfo.project.name.includes('mobile'),
    'The shared app RSS workload runs once.',
  )
  test.setTimeout(300_000)
  await loginOwner(page)
  const { workspaceId, bookId } = await makeBook(page, `large-${Date.now()}`)
  const project = process.env.COMPOSE_PROJECT_NAME
  expect(project).toBeTruthy()
  const limitBytes = 300_000_000
  const before = appHighWaterRSS(project!)
  const lines = ['Name,Amount']
  for (let index = 0; index < 200_000; index++)
    lines.push(`row-${index},${index}`)
  const dialog = await openBookImport(page, workspaceId, bookId)
  await dialog.locator('input[type=file]').setInputFiles({
    name: 'large.csv',
    mimeType: 'text/csv',
    buffer: Buffer.from(lines.join('\n') + '\n'),
  })
  await dialog.getByRole('button', { name: en.import.analyze }).click()
  await dialog.getByLabel(en.import.tableName).fill('Large imported')
  const finished = doneJob(page)
  await dialog.getByRole('button', { name: en.import.start }).click()
  const result = (await (await finished).json()) as ImportJob
  expect(result).toMatchObject({
    status: 'done',
    result: { rows: 200_000, invalidCells: 0 },
  })
  const peak = appHighWaterRSS(project!)
  await testInfo.attach('import-rss', {
    body: JSON.stringify({
      rows: result.result.rows,
      beforeBytes: before,
      peakBytes: peak,
      limitBytes,
    }),
    contentType: 'application/json',
  })
  expect(peak, 'app process peak RSS throughout its lifetime').toBeLessThan(
    limitBytes,
  )
  await closeImport(page, dialog)
})
