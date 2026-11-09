import { readFile } from 'node:fs/promises'
import { inflateRawSync } from 'node:zlib'
import type { Locator } from '@playwright/test'
import { expect, paceLogin, test } from './fixtures'
import { setLocale } from './app-shell'
import { openExportMenu } from './table-controls'
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

function zipEntry(bytes: Buffer, entryName: string) {
  let end = bytes.length - 22
  while (end >= 0 && bytes.readUInt32LE(end) !== 0x06054b50) end--
  if (end < 0) throw new Error('XLSX archive directory is missing.')
  const entries = bytes.readUInt16LE(end + 10)
  let offset = bytes.readUInt32LE(end + 16)
  for (let index = 0; index < entries; index++) {
    if (bytes.readUInt32LE(offset) !== 0x02014b50)
      throw new Error('XLSX archive entry is invalid.')
    const method = bytes.readUInt16LE(offset + 10)
    const compressedSize = bytes.readUInt32LE(offset + 20)
    const nameLength = bytes.readUInt16LE(offset + 28)
    const extraLength = bytes.readUInt16LE(offset + 30)
    const commentLength = bytes.readUInt16LE(offset + 32)
    const name = bytes.toString('utf8', offset + 46, offset + 46 + nameLength)
    if (name === entryName) {
      const localOffset = bytes.readUInt32LE(offset + 42)
      const localNameLength = bytes.readUInt16LE(localOffset + 26)
      const localExtraLength = bytes.readUInt16LE(localOffset + 28)
      const dataOffset = localOffset + 30 + localNameLength + localExtraLength
      const compressed = bytes.subarray(dataOffset, dataOffset + compressedSize)
      if (method === 0) return compressed
      if (method === 8) return inflateRawSync(compressed)
      throw new Error(`Unsupported XLSX compression method: ${method}.`)
    }
    offset += 46 + nameLength + extraLength + commentLength
  }
  throw new Error(`XLSX archive entry ${entryName} is missing.`)
}

test('GP-06 XLSX-only transfer menu and the AT-702 package export artifact', async ({
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
  await expect(page).toHaveURL(/\/(?:home|change-password)$/)

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
    en.gridPolish.gp06.trigger,
  )
  await expect(page.locator('.transfer-menu > summary')).toHaveText(
    en.gridPolish.gp06.trigger,
  )
  const menuItems = page.locator('.view-export-panel button')
  await expect(menuItems).toHaveCount(2)
  await expect(menuItems).toHaveText([
    en.gridPolish.gp06.import,
    en.gridPolish.gp06.export,
  ])

  const packageRequest = page.waitForRequest(
    (request) =>
      request.method() === 'POST' &&
      new URL(request.url()).pathname === `/api/v1/views/${view.id}/export`,
  )
  const packageDownload = page.waitForEvent('download')
  const packageButton = page.getByRole('button', {
    name: en.gridPolish.gp06.export,
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
  const workbook = zipEntry(packageBytes, 'xl/workbook.xml').toString('utf8')
  expect(workbook).toMatch(/<sheet[^>]+name="Data"/)
  expect(workbook).toMatch(/<sheet[^>]+name="_meta"/)
})
