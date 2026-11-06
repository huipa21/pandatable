import { readFile } from 'node:fs/promises'
import type { Locator } from '@playwright/test'
import { expect, paceLogin, test } from './fixtures'
import { setLocale } from './app-shell'
import { openExportMenu, openGridMore } from './table-controls'
import en from '../src/i18n/en.json' with { type: 'json' }
import type { TableSchema } from '../src/api/types'

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

async function expectUnobscured(locator: Locator) {
  await expect(locator).toBeVisible()
  await expect
    .poll(() =>
      locator.evaluate((element) => {
        const bounds = element.getBoundingClientRect()
        const hit = document.elementFromPoint(
          bounds.x + bounds.width / 2,
          bounds.y + bounds.height / 2,
        )
        return hit !== null && (hit === element || element.contains(hit))
      }),
    )
    .toBe(true)
}

test('AT-701 current-view CSV and the AT-702 package export artifact', async ({
  page,
}, testInfo) => {
  test.setTimeout(150_000)
  await paceLogin(page)
  await page.goto('/login')
  if ((await page.locator('html').getAttribute('lang')) === 'zh-CN')
    await setLocale(page, 'en')
  await page.getByLabel(en.auth.login, { exact: true }).fill('first-admin')
  await page
    .getByLabel(en.auth.password, { exact: true })
    .fill('Setup-password-123')
  await page.getByRole('button', { name: en.auth.signIn, exact: true }).click()
  await expect(page).toHaveURL(/\/(?:w\/[^/]+|change-password)$/)

  const stamp = Date.now()
  const create = async (path: string, data: unknown) => {
    const response = await page.request.post(path, { headers: headers(), data })
    expect(response.status()).toBe(201)
    return response.json()
  }
  const workspace = (await create('/api/v1/workspaces', {
    name: `Export ${stamp}`,
  })) as { workspace: { id: string } }
  const book = (await create(
    `/api/v1/workspaces/${workspace.workspace.id}/books`,
    { name: `Export book ${stamp}` },
  )) as { book: { id: string } }
  const initial = (await create(`/api/v1/books/${book.book.id}/tables`, {
    name: `任务 ${stamp}`,
  })) as TableSchema
  const tableId = initial.table.id
  const field = async (name: string) =>
    (
      (await create(`/api/v1/tables/${tableId}/fields`, {
        name,
        uiType: 'text',
      })) as { field: TableSchema['fields'][number] }
    ).field
  const group = await field('Group')
  const status = await field('Status')
  const secret = await field('Secret')
  const schemaResponse = await page.request.get(
    `/api/v1/tables/${tableId}/schema`,
  )
  expect(schemaResponse.status()).toBe(200)
  const schema = (await schemaResponse.json()) as TableSchema
  const name = schema.fields.find(
    (item) => item.id === schema.table.displayFieldId,
  )!
  const view = schema.views[0]!
  const configured = await page.request.patch(`/api/v1/views/${view.id}`, {
    headers: { ...headers(), 'If-Match': String(view.version) },
    data: {
      config: {
        fields: schema.fields.map((item) => ({
          id: item.id,
          visible: !item.isSystem && item.id !== secret.id,
        })),
        frozen: 0,
        groupBy: [group.id],
        sort: [{ field: name.id, dir: 'desc' }],
        filter: { field: status.id, cmp: 'is', value: 'Open' },
      },
    },
  })
  expect(configured.status()).toBe(200)
  const rows = [
    { name: '中文,"引号"', group: 'B', status: 'Open' },
    { name: 'Alpha', group: 'A', status: 'Open' },
    { name: 'Excluded', group: 'A', status: 'Closed' },
    { name: 'Omega', group: 'A', status: 'Open' },
  ]
  const created = await page.request.post(`/api/v1/tables/${tableId}/rows`, {
    headers: headers(),
    data: {
      rows: rows.map((row) => ({
        fields: {
          [name.id]: row.name,
          [group.id]: row.group,
          [status.id]: row.status,
          [secret.id]: 'secret value',
        },
      })),
    },
  })
  expect(created.status()).toBe(201)
  await page.goto(`/t/${tableId}/v/${view.id}`)

  const mobile = testInfo.project.name.includes('mobile')
  if (mobile) {
    await page
      .getByRole('button', { name: en.mobileRows.options, exact: true })
      .click()
    const options = page.locator('dialog.mobile-sheet')
    await options
      .getByRole('checkbox', { name: 'Status', exact: true })
      .uncheck()
    await options
      .getByRole('button', { name: en.mobileRows.closeOptions, exact: true })
      .click()

    const filter = page.locator('.table-toolbar .view-filter')
    await filter.locator('summary').click()
    await expectUnobscured(filter.locator('.view-filter-panel'))
    await filter.locator('summary').click()

    const visibility = page.locator('.table-toolbar .visibility-menu')
    await visibility.locator('summary').click()
    await expectUnobscured(
      visibility.getByRole('button', { name: en.sharing.open, exact: true }),
    )
    await visibility.locator('summary').click()
  }
  await openExportMenu(page)
  await expect(page.locator('.transfer-menu')).toHaveCount(1)
  await expect(
    page.locator('.table-toolbar-actions .schema-summary'),
  ).toHaveCount(0)
  await expect(page.locator('.transfer-menu > summary')).toHaveAttribute(
    'aria-label',
    en.transfer.title,
  )
  const csvRequest = page.waitForRequest(
    (request) =>
      request.method() === 'POST' &&
      new URL(request.url()).pathname === `/api/v1/views/${view.id}/export`,
  )
  const csvDownload = page.waitForEvent('download')
  await page.getByRole('button', { name: en.export.csv, exact: true }).click()
  const csvPayload = (await csvRequest).postDataJSON() as {
    fieldIds: string[]
    groupBy: string[]
  }
  expect(csvPayload.groupBy).toEqual([group.id])
  expect(csvPayload.fieldIds).not.toContain(secret.id)
  if (mobile) expect(csvPayload.fieldIds).not.toContain(status.id)
  const csvFile = await csvDownload
  expect(csvFile.suggestedFilename()).toBe(`任务 ${stamp}.csv`)
  expect(await csvFile.failure()).toBeNull()
  const csv = await readFile((await csvFile.path())!)
  expect([...csv.subarray(0, 3)]).toEqual([0xef, 0xbb, 0xbf])
  const value = csv.subarray(3).toString('utf8')
  if (mobile) {
    expect(value).toBe(
      `${name.name},Group\r\nOmega,A\r\nAlpha,A\r\n"中文,""引号""",B\r\n`,
    )
  } else {
    expect(value).toBe(
      `${name.name},Group,Status\r\nOmega,A,Open\r\nAlpha,A,Open\r\n"中文,""引号""",B,Open\r\n`,
    )
    await openGridMore(page)
    await page.getByLabel(en.grid.quickSearch, { exact: true }).fill('Alpha')
    await openExportMenu(page)
    const searchRequest = page.waitForRequest(
      (request) =>
        request.method() === 'POST' &&
        new URL(request.url()).pathname === `/api/v1/views/${view.id}/export`,
    )
    const searchedDownload = page.waitForEvent('download')
    await page.getByRole('button', { name: en.export.csv, exact: true }).click()
    expect((await searchRequest).postDataJSON()).toMatchObject({
      search: 'Alpha',
    })
    const searched = await searchedDownload
    expect(await searched.failure()).toBeNull()
    const searchedCSV = await readFile((await searched.path())!)
    expect(searchedCSV.subarray(3).toString('utf8')).toBe(
      `${name.name},Group,Status\r\nAlpha,A,Open\r\n`,
    )
  }
  expect(value).not.toContain('secret value')
  expect(value).not.toContain('Excluded')

  const packageRequest = page.waitForRequest(
    (request) =>
      request.method() === 'POST' &&
      new URL(request.url()).pathname === `/api/v1/views/${view.id}/export`,
  )
  const packageDownload = page.waitForEvent('download')
  const packageButton = page.getByRole('button', {
    name: en.export.package,
    exact: true,
  })
  await expectUnobscured(packageButton)
  await packageButton.click()
  expect((await packageRequest).postDataJSON()).toEqual({ format: 'xlsx' })
  const packageFile = await packageDownload
  expect(packageFile.suggestedFilename()).toBe(`任务 ${stamp}.xlsx`)
  expect(await packageFile.failure()).toBeNull()
  const packageBytes = await readFile((await packageFile.path())!)
  expect([...packageBytes.subarray(0, 4)]).toEqual([0x50, 0x4b, 0x03, 0x04])
  expect(packageBytes.length).toBeGreaterThan(2_000)

  const secondView = (await create(`/api/v1/tables/${tableId}/views`, {
    name: 'Closed items',
    type: 'grid',
    scope: 'shared',
  })) as { view: TableSchema['views'][number] }
  const configuredSecond = await page.request.patch(
    `/api/v1/views/${secondView.view.id}`,
    {
      headers: { ...headers(), 'If-Match': String(secondView.view.version) },
      data: {
        config: {
          fields: schema.fields.map((item) => ({
            id: item.id,
            visible: item.id === name.id || item.id === status.id,
          })),
          frozen: 0,
          filter: { field: status.id, cmp: 'is', value: 'Closed' },
        },
      },
    },
  )
  expect(configuredSecond.status()).toBe(200)
  await page.goto(`/t/${tableId}/v/${secondView.view.id}`)
  await openExportMenu(page)
  const secondRequest = page.waitForRequest(
    (request) =>
      request.method() === 'POST' &&
      new URL(request.url()).pathname ===
        `/api/v1/views/${secondView.view.id}/export`,
  )
  const secondDownload = page.waitForEvent('download')
  await page.getByRole('button', { name: en.export.csv, exact: true }).click()
  const secondPayload = (await secondRequest).postDataJSON()
  expect(secondPayload).toMatchObject({
    filter: { field: status.id, cmp: 'is', value: 'Closed' },
  })
  expect(secondPayload.fieldIds).toEqual(
    mobile ? undefined : [name.id, status.id],
  )
  const secondFile = await secondDownload
  expect(await secondFile.failure()).toBeNull()
  expect(
    (await readFile((await secondFile.path())!)).subarray(3).toString(),
  ).toBe(`${name.name},Status\r\nExcluded,Closed\r\n`)

  const otherTable = (await create(`/api/v1/books/${book.book.id}/tables`, {
    name: `Other export ${stamp}`,
  })) as TableSchema
  await create(`/api/v1/tables/${otherTable.table.id}/rows`, {
    rows: [{ fields: { [otherTable.table.displayFieldId!]: 'Other table' } }],
  })
  await page.goto(`/t/${otherTable.table.id}/v/${otherTable.views[0]!.id}`)
  await openExportMenu(page)
  const otherRequest = page.waitForRequest(
    (request) =>
      request.method() === 'POST' &&
      new URL(request.url()).pathname ===
        `/api/v1/views/${otherTable.views[0]!.id}/export`,
  )
  const otherDownload = page.waitForEvent('download')
  await page.getByRole('button', { name: en.export.csv, exact: true }).click()
  const otherPayload = (await otherRequest).postDataJSON()
  expect(otherPayload.fieldIds).toEqual(
    mobile ? undefined : [otherTable.table.displayFieldId],
  )
  const otherFile = await otherDownload
  expect(await otherFile.failure()).toBeNull()
  const displayName = otherTable.fields.find(
    (item) => item.id === otherTable.table.displayFieldId,
  )!.name
  expect(
    (await readFile((await otherFile.path())!)).subarray(3).toString(),
  ).toBe(`${displayName}\r\nOther table\r\n`)
})
