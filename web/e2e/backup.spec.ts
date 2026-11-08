import { execFileSync, spawnSync } from 'node:child_process'
import { readFileSync } from 'node:fs'
import { inflateRawSync } from 'node:zlib'
import { expect, expectHTTPFailure, paceLogin, test } from './fixtures'
import type { BackupListing, BackupRecord } from '../src/api/types'
import { setLocale } from './app-shell'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }

function ownedApp() {
  const engine = process.env.PT_E2E_ENGINE
  const id = process.env.PT_E2E_APP_CONTAINER
  const project = process.env.COMPOSE_PROJECT_NAME
  if (
    (engine !== 'podman' && engine !== 'docker') ||
    !id?.match(/^[a-f0-9]{12,64}$/i) ||
    !project?.match(/^pt-e2e-[a-z2-7]+$/)
  )
    throw new Error('An owned E2E app container is required')
  const label = execFileSync(
    engine,
    [
      'inspect',
      '--format',
      '{{index .Config.Labels "com.docker.compose.project"}}',
      id,
    ],
    { encoding: 'utf8' },
  ).trim()
  expect(label).toBe(project)
  return {
    exec: (...args: string[]) =>
      execFileSync(engine, ['exec', '--user', '10001:10001', id, ...args]),
    exists: (name: string) => {
      const result = spawnSync(engine, [
        'exec',
        '--user',
        '10001:10001',
        id,
        'test',
        '-e',
        name,
      ])
      if (result.error || (result.status !== 0 && result.status !== 1))
        throw new Error(
          `Unable to check owned backup file: ${result.error ?? result.status}`,
        )
      return result.status === 0
    },
    write: (name: string, value: string) =>
      execFileSync(
        engine,
        ['exec', '-i', '--user', '10001:10001', id, 'tee', name],
        {
          input: value,
        },
      ),
  }
}

function zipEntries(buffer: Buffer) {
  const end = buffer.lastIndexOf(Buffer.from([0x50, 0x4b, 0x05, 0x06]))
  if (end < 0) throw new Error('ZIP end-of-directory record is missing')
  const count = buffer.readUInt16LE(end + 10)
  let offset = buffer.readUInt32LE(end + 16)
  const files = new Map<string, Buffer>()
  for (let index = 0; index < count; index++) {
    if (buffer.readUInt32LE(offset) !== 0x02014b50)
      throw new Error('Invalid ZIP central directory')
    const method = buffer.readUInt16LE(offset + 10)
    const size = buffer.readUInt32LE(offset + 20)
    const nameLength = buffer.readUInt16LE(offset + 28)
    const extraLength = buffer.readUInt16LE(offset + 30)
    const commentLength = buffer.readUInt16LE(offset + 32)
    const local = buffer.readUInt32LE(offset + 42)
    const name = buffer.toString('utf8', offset + 46, offset + 46 + nameLength)
    if (buffer.readUInt32LE(local) !== 0x04034b50)
      throw new Error('Invalid ZIP local header')
    const dataStart =
      local +
      30 +
      buffer.readUInt16LE(local + 26) +
      buffer.readUInt16LE(local + 28)
    const data = buffer.subarray(dataStart, dataStart + size)
    if (method !== 0 && method !== 8)
      throw new Error('Unsupported ZIP compression')
    files.set(name, method === 8 ? inflateRawSync(data) : data)
    offset += 46 + nameLength + extraLength + commentLength
  }
  return files
}

test('AT-706/708: admin backup lifecycle, ZIP download and localized disk guard', async ({
  page,
  api,
}) => {
  test.setTimeout(120_000)
  page.setDefaultTimeout(10_000)
  const app = ownedApp()
  const attachment = '/data/files/t39-browser-attachment'
  const sparse = '/data/files/t39-browser-disk-guard'
  try {
    expect((await api.get('/api/v1/admin/backups')).status()).toBe(401)
    await paceLogin(page)
    await page.goto('/login')
    if ((await page.locator('html').getAttribute('lang')) === 'zh-CN')
      await setLocale(page, 'en')
    await page.getByLabel(en.auth.login, { exact: true }).fill('first-admin')
    await page
      .getByLabel(en.auth.password, { exact: true })
      .fill('Setup-password-123')
    await page
      .getByRole('button', { name: en.auth.signIn, exact: true })
      .click()
    await expect(page).toHaveURL(/\/home$/)
    app.write(attachment, 'browser backup fixture')
    await page.goto('/admin/backups')
    await expect(
      page.getByRole('heading', { name: en.backups.title }),
    ).toBeVisible()
    await expect(page.getByText(/^Available disk space:/)).toBeVisible()

    const started = page.waitForResponse(
      (response) =>
        response.request().method() === 'POST' &&
        new URL(response.url()).pathname === '/api/v1/admin/backups',
    )
    await page
      .getByRole('button', { name: en.backups.create, exact: true })
      .click()
    const startResponse = await started
    expect(startResponse.status()).toBe(202)
    const record = (await startResponse.json()) as BackupRecord
    expect(record.status).toBe('running')
    expect(record.fileName).toMatch(/^pandatable-\d{8}-\d{6}\.zip$/)
    const card = page.locator(`[data-backup="${record.id}"]`)
    await expect(card).toContainText('Status: Done', { timeout: 60_000 })
    await expect(card).toContainText('Size:')
    const [download] = await Promise.all([
      page.waitForEvent('download'),
      card
        .getByRole('link', { name: en.backups.download, exact: true })
        .click(),
    ])
    expect(await download.failure()).toBeNull()
    expect(download.suggestedFilename()).toBe(record.fileName)
    const downloadPath = await download.path()
    if (!downloadPath) throw new Error('Downloaded archive is unavailable')
    const archive = readFileSync(downloadPath)
    const entries = zipEntries(archive)
    expect(entries.get('db.dump')?.subarray(0, 5).toString()).toBe('PGDMP')
    expect(entries.has('files/')).toBe(true)
    expect(entries.get('files/t39-browser-attachment')?.toString()).toBe(
      'browser backup fixture',
    )
    const metadata = JSON.parse(entries.get('backup.json')!.toString())
    expect(metadata).toEqual(
      expect.objectContaining({
        appVersion: expect.any(String),
        schemaMigration: expect.any(Number),
        createdAt: expect.any(String),
        pgVersion: expect.stringMatching(/^18\./),
        fileCount: expect.any(Number),
      }),
    )
    const listing = (await (
      await page.request.get('/api/v1/admin/backups')
    ).json()) as BackupListing
    expect(listing.items.find((item) => item.id === record.id)?.sizeBytes).toBe(
      archive.length,
    )
    const stored = `/data/backups/${record.fileName}`
    expect(app.exists(stored)).toBe(true)
    await card
      .getByRole('button', {
        name: en.backups.deleteItem.replace('{name}', record.fileName),
        exact: true,
      })
      .click()
    await expect(card).toContainText('Permanently delete')
    await card
      .getByRole('button', { name: en.backups.confirmDelete, exact: true })
      .click()
    await expect(card).toHaveCount(0)
    expect(app.exists(stored)).toBe(false)
    expect(
      (await page.request.get(`/api/v1/admin/backups/${record.id}`)).status(),
    ).toBe(404)

    const after = (await (
      await page.request.get('/api/v1/admin/backups')
    ).json()) as BackupListing
    expect(Number.isSafeInteger(after.freeBytes)).toBe(true)
    app.exec('truncate', '-s', String(after.freeBytes + 1), sparse)
    expectHTTPFailure(page, 'POST', '/api/v1/admin/backups', 409)
    await page
      .getByRole('button', { name: en.backups.create, exact: true })
      .click()
    await expect(page.getByRole('alert')).toContainText(
      en.errors.ERR_BACKUP_DISK_SPACE,
    )
    await setLocale(page, 'zh-CN')
    await expect(page.getByRole('alert')).toContainText(
      zh.errors.ERR_BACKUP_DISK_SPACE,
    )
    const refused = (await (
      await page.request.get('/api/v1/admin/backups')
    ).json()) as BackupListing
    expect(refused.items).toHaveLength(0)
  } finally {
    app.exec('rm', '-f', '--', attachment, sparse)
  }
})
