import type { Page } from '@playwright/test'
import {
  expect,
  expectBrowserClean,
  observeBrowser,
  paceLogin,
  test,
} from './fixtures'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }
import type {
  AttachmentValue,
  DataRow,
  TableField,
  TableSchema,
} from '../src/api/types'

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

async function create<T>(page: Page, path: string, data: unknown): Promise<T> {
  const response = await page.request.post(path, { headers: headers(), data })
  expect(response.status()).toBe(201)
  return response.json()
}

async function english(page: Page) {
  if ((await page.locator('html').getAttribute('lang')) === 'zh-CN')
    await page
      .getByRole('button', { name: zh.common.languages.en, exact: true })
      .click()
}

test('AT-902 displays persisted Gallery covers and selected card fields', async ({
  page,
  browser,
  isMobile,
}) => {
  test.setTimeout(240_000)
  await paceLogin(page)
  const login = await page.request.post('/api/v1/auth/login', {
    headers: headers(),
    data: { login: 'first-admin', password: 'Setup-password-123' },
  })
  expect(login.status()).toBe(200)
  const suffix = `${Date.now()}-${isMobile ? 'mobile' : 'desktop'}`
  const workspace = await create<{ workspace: { id: string } }>(
    page,
    '/api/v1/workspaces',
    { name: `Gallery ${suffix}` },
  )
  const book = await create<{ book: { id: string } }>(
    page,
    `/api/v1/workspaces/${workspace.workspace.id}/books`,
    { name: 'Gallery book' },
  )
  const schema = await create<TableSchema>(
    page,
    `/api/v1/books/${book.book.id}/tables`,
    { name: 'Gallery items' },
  )
  const tableId = schema.table.id
  const titleId = schema.table.displayFieldId
  if (!titleId) throw new Error('Gallery table has no display field')
  const photo = await create<{ field: TableField }>(
    page,
    `/api/v1/tables/${tableId}/fields`,
    { name: 'Photo', uiType: 'attachment' },
  )
  const notes = await create<{ field: TableField }>(
    page,
    `/api/v1/tables/${tableId}/fields`,
    { name: 'Notes', uiType: 'text' },
  )
  const hidden = await create<{ field: TableField }>(
    page,
    `/api/v1/tables/${tableId}/fields`,
    { name: 'Hidden detail', uiType: 'text' },
  )
  const pngBytes = await page.evaluate(async () => {
    const canvas = document.createElement('canvas')
    canvas.width = 2
    canvas.height = 2
    const context = canvas.getContext('2d')
    if (!context) throw new Error('Canvas 2D context is unavailable')
    context.fillStyle = '#d24b36'
    context.fillRect(0, 0, 2, 2)
    const blob = await new Promise<Blob>((resolve, reject) => {
      canvas.toBlob((value) =>
        value ? resolve(value) : reject(new Error('PNG encoding failed')),
      )
    })
    return Array.from(new Uint8Array(await blob.arrayBuffer()))
  })
  const upload = await page.request.post(
    `/api/v1/tables/${tableId}/fields/${photo.field.id}/attachments`,
    {
      headers: headers(),
      multipart: {
        file: {
          name: 'cover.png',
          mimeType: 'image/png',
          buffer: Buffer.from(pngBytes),
        },
      },
    },
  )
  expect(upload.status()).toBe(201)
  const image = (await upload.json()) as AttachmentValue
  expect(image.thumb).toBe(`/api/v1/attachments/${image.id}/thumbnail`)
  const inserted = await create<{ rows: DataRow[] }>(
    page,
    `/api/v1/tables/${tableId}/rows`,
    {
      rows: [
        {
          fields: {
            [titleId]: 'First item',
            [photo.field.id]: [image],
            [notes.field.id]: 'Visible detail',
            [hidden.field.id]: 'Hidden value',
          },
        },
        { fields: { [titleId]: 'No image' } },
      ],
    },
  )
  const row = inserted.rows[0]!
  const emptyRow = inserted.rows[1]!

  await page.goto(`/t/${tableId}/v/${schema.views[0]!.id}`)
  await english(page)
  const createView = page.locator('.view-create')
  await createView.getByLabel(en.nav.viewName, { exact: true }).fill('Gallery')
  await createView.getByLabel(en.nav.viewScopeLabel).selectOption('shared')
  await createView.getByLabel(en.form.viewType).selectOption('gallery')
  await createView.getByRole('button', { name: en.nav.createView }).click()
  const builder = page.locator('.gallery-settings')
  await expect(builder).toBeVisible()
  await builder.locator('summary').click()
  await builder.getByLabel(en.gallery.coverField).selectOption(photo.field.id)
  await builder.getByRole('checkbox', { name: 'Photo', exact: true }).uncheck()
  await builder
    .getByRole('checkbox', { name: 'Hidden detail', exact: true })
    .uncheck()
  await builder.getByRole('button', { name: en.gallery.save }).click()

  const card = page.locator(`.gallery-card[data-row-id="${row.id}"]`)
  await expect(card).toContainText('First item')
  await expect(card).toContainText('Visible detail')
  await expect(card).not.toContainText('Hidden value')
  const cover = card.locator('.gallery-cover img')
  await expect(cover).toHaveAttribute('src', image.thumb!)
  await expect
    .poll(() =>
      cover.evaluate((element: HTMLImageElement) => element.naturalWidth),
    )
    .toBeGreaterThan(0)
  const emptyCard = page.locator(`.gallery-card[data-row-id="${emptyRow.id}"]`)
  await expect(emptyCard).toContainText(en.gallery.noImage)
  const thumbnailResponse = await page.request.get(image.thumb!)
  expect(thumbnailResponse.status()).toBe(200)
  expect(thumbnailResponse.headers()['content-type']).toContain('image/png')

  const galleryURL = new URL(page.url()).pathname
  await card
    .getByRole('button', { name: en.gallery.openRow.replace('{id}', row.id) })
    .click()
  await expect(page.getByRole('dialog')).toBeVisible()
  await page.getByRole('button', { name: en.rowExpand.close }).click()
  await expect.poll(() => new URL(page.url()).pathname).toBe(galleryURL)

  await page.reload()
  await expect(card).toContainText('Visible detail')
  await expect(cover).toHaveAttribute('src', image.thumb!)
  const persistedSchema = (await (
    await page.request.get(`/api/v1/tables/${tableId}/schema`)
  ).json()) as TableSchema
  const persisted = persistedSchema.views.find((view) =>
    galleryURL.endsWith(`/${view.id}`),
  )?.config.gallery
  expect(persisted).toEqual({
    coverField: photo.field.id,
    fields: [titleId, notes.field.id],
  })

  const peerContext = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL,
    storageState: await page.context().storageState(),
    viewport: isMobile
      ? { width: 390, height: 844 }
      : { width: 1280, height: 900 },
  })
  const peer = await peerContext.newPage()
  const diagnostics = await observeBrowser(peer)
  const frames: { t?: string; tableId?: string }[] = []
  peer.on('websocket', (socket) => {
    socket.on('framereceived', (frame) => {
      if (typeof frame.payload === 'string')
        frames.push(
          JSON.parse(frame.payload) as { t?: string; tableId?: string },
        )
    })
  })
  try {
    await peer.goto(galleryURL)
    await english(peer)
    const peerCard = peer.locator(`.gallery-card[data-row-id="${row.id}"]`)
    await expect(peerCard).toContainText('Visible detail')
    await expect
      .poll(() =>
        frames.some(
          (frame) => frame.t === 'subscribed' && frame.tableId === tableId,
        ),
      )
      .toBe(true)
    const update = await page.request.patch(
      `/api/v1/tables/${tableId}/rows/${row.id}`,
      {
        headers: headers(),
        data: { fields: { [notes.field.id]: 'Updated detail' } },
      },
    )
    expect(update.status()).toBe(200)
    await expect(peerCard).toContainText('Updated detail')
    const bounds = await peerCard.boundingBox()
    expect(bounds).not.toBeNull()
    expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(
      peer.viewportSize()!.width,
    )
    expectBrowserClean(peer, diagnostics)
  } finally {
    await peerContext.close()
  }
})
