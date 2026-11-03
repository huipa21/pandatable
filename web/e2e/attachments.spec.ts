import { expect, expectHTTPFailure, paceLogin, test } from './fixtures'
import { create as createResource } from './navigation'
import { restoreSettings } from './settings'
import type {
  AdminSettings,
  AttachmentValue,
  RowQueryResult,
  TableSchema,
} from '../src/api/types'
import { setLocale } from './app-shell'
import { openFieldPanel } from './table-controls'
import en from '../src/i18n/en.json' with { type: 'json' }

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

async function login(page: import('@playwright/test').Page) {
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

async function addAttachmentField(
  page: import('@playwright/test').Page,
  tableId: string,
  name: string,
  maxFiles: number,
  accept = '',
) {
  const panel = await openFieldPanel(page)
  await panel
    .getByRole('button', { name: en.field.panel.add, exact: true })
    .click()
  const dialog = page.getByRole('dialog', {
    name: en.field.panel.createTitle,
    exact: true,
  })
  await dialog
    .getByRole('textbox', { name: en.field.panel.name, exact: true })
    .fill(name)
  await dialog
    .getByRole('combobox', { name: en.field.panel.kind, exact: true })
    .selectOption('attachment')
  await dialog
    .getByRole('textbox', {
      name: en.field.panel.attachmentMaxFiles,
      exact: true,
    })
    .fill(String(maxFiles))
  if (accept)
    await dialog
      .getByRole('textbox', {
        name: en.field.panel.attachmentAccept,
        exact: true,
      })
      .fill(accept)
  await dialog
    .getByRole('button', { name: en.field.panel.create, exact: true })
    .click()
  await expect(dialog).not.toBeVisible()
  const response = await page.request.get(`/api/v1/tables/${tableId}/schema`)
  expect(response.status()).toBe(200)
  const schema = (await response.json()) as TableSchema
  const field = schema.fields.find((item) => item.name === name)
  if (!field) throw new Error(`Created attachment field ${name} is missing`)
  await page.locator('.grid-fields-trigger').click()
  return field
}

async function queryRows(
  page: import('@playwright/test').Page,
  viewId: string,
) {
  const response = await page.request.post(`/api/v1/views/${viewId}/query`, {
    headers: headers(),
    data: { offset: 0, limit: 100, includeCount: true },
  })
  expect(response.status()).toBe(200)
  return (await response.json()) as RowQueryResult
}

async function openAttachmentEditor(
  page: import('@playwright/test').Page,
  viewId: string,
  fieldId: string,
  fieldName: string,
) {
  const row = (await queryRows(page, viewId)).rows[0]
  if (!row) throw new Error('Attachment test row is missing')
  await page
    .getByRole('button', {
      name: en.rowExpand.open.replace('{id}', row.id),
      exact: true,
    })
    .click()
  const field = page
    .locator('.row-expand')
    .locator(`[data-field-id="${fieldId}"]`)
  await field
    .getByRole('button', {
      name: en.rowExpand.edit.replace('{name}', fieldName),
      exact: true,
    })
    .click()
  await expect(field.locator('.field-attachment-editor')).toBeVisible()
}

async function dropFile(
  page: import('@playwright/test').Page,
  file: { name: string; mime: string; base64: string },
) {
  const transfer = await page.evaluateHandle((value) => {
    const bytes = Uint8Array.from(atob(value.base64), (char) =>
      char.charCodeAt(0),
    )
    const data = new DataTransfer()
    data.items.add(new File([bytes], value.name, { type: value.mime }))
    return data
  }, file)
  try {
    await page.locator('.field-attachment-editor').dispatchEvent('drop', {
      dataTransfer: transfer,
    })
  } finally {
    await transfer.dispose()
  }
}

async function createUser(
  page: import('@playwright/test').Page,
  username: string,
) {
  const response = await page.request.post('/api/v1/admin/users', {
    headers: headers(),
    data: {
      username,
      displayName: 'Attachment outsider',
      role: 'user',
      password: 'Temporary-password-123',
    },
  })
  expect(response.status()).toBe(201)
  return (await response.json()).user as { id: string }
}

async function signInAs(
  page: import('@playwright/test').Page,
  username: string,
) {
  await paceLogin(page)
  await page.goto('/login')
  if ((await page.locator('html').getAttribute('lang')) === 'zh-CN')
    await setLocale(page, 'en')
  await page.getByLabel(en.auth.login, { exact: true }).fill(username)
  await page
    .getByLabel(en.auth.password, { exact: true })
    .fill('Temporary-password-123')
  await page.getByRole('button', { name: en.auth.signIn, exact: true }).click()
  await expect(page).toHaveURL(/\/(home|change-password)$/)
  if (new URL(page.url()).pathname === '/change-password') {
    const changedPassword = 'Attachment-password-123'
    await page
      .getByLabel(en.auth.currentPassword, { exact: true })
      .fill('Temporary-password-123')
    await page
      .getByLabel(en.auth.newPassword, { exact: true })
      .fill(changedPassword)
    await page
      .getByLabel(en.auth.confirmPassword, { exact: true })
      .fill(changedPassword)
    await page
      .getByRole('button', { name: en.auth.changePassword, exact: true })
      .click()
    await expect(page).toHaveURL(/\/login$/)
    await page.getByLabel(en.auth.login, { exact: true }).fill(username)
    await page
      .getByLabel(en.auth.password, { exact: true })
      .fill(changedPassword)
    await page
      .getByRole('button', { name: en.auth.signIn, exact: true })
      .click()
  }
  await expect(page).toHaveURL(/\/home$/)
}

test('AT-321/322 attachment field upload, serving, row persistence, and configured size rejection', async ({
  page,
  browser,
  isMobile,
}) => {
  test.setTimeout(240_000)
  test.skip(isMobile, 'Attachment acceptance is desktop-only')
  await page.setViewportSize({ width: 1440, height: 900 })
  await login(page)
  const suffix = Date.now()
  await createResource(page, 'workspace', `Attachment W ${suffix}`)
  await createResource(page, 'book', `Attachment B ${suffix}`)
  await createResource(page, 'table', `Attachment T ${suffix}`)
  const path = new URL(page.url()).pathname.split('/')
  const tableId = path[2]!
  const viewId = path[4]!
  const field = await addAttachmentField(
    page,
    tableId,
    'Attachments',
    2,
    'image/*, application/pdf',
  )
  const limitField = await addAttachmentField(page, tableId, 'Limit check', 2)
  await page.getByRole('button', { name: en.grid.addRow, exact: true }).click()
  await expect.poll(async () => (await queryRows(page, viewId)).total).toBe(1)

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

  await openAttachmentEditor(page, viewId, field.id, field.name)
  const imageUpload = page.waitForResponse(
    (response) =>
      response.request().method() === 'POST' &&
      response
        .url()
        .includes(`/tables/${tableId}/fields/${field.id}/attachments`),
  )
  await page
    .locator('.field-attachment-editor input[type="file"]')
    .setInputFiles({
      name: 'pixel.png',
      mimeType: 'image/png',
      buffer: Buffer.from(pngBytes),
    })
  const imageResponse = await imageUpload
  expect(imageResponse.status()).toBe(201)
  const imageFile = (await imageResponse.json()) as AttachmentValue
  expect(imageFile.mime).toBe('image/png')
  expect(imageFile.thumb).toBe(`/api/v1/attachments/${imageFile.id}/thumbnail`)

  const pdfBase64 = Buffer.from(
    '%PDF-1.4\nattachment fixture\n%%EOF\n',
  ).toString('base64')
  const pdfUpload = page.waitForResponse(
    (response) =>
      response.request().method() === 'POST' &&
      response
        .url()
        .includes(`/tables/${tableId}/fields/${field.id}/attachments`),
  )
  await dropFile(page, {
    name: 'document.pdf',
    mime: 'application/pdf',
    base64: pdfBase64,
  })
  const pdfResponse = await pdfUpload
  expect(pdfResponse.status()).toBe(201)
  const pdfFile = (await pdfResponse.json()) as AttachmentValue
  expect(pdfFile.mime).toBe('application/pdf')
  expect(pdfFile.thumb).toBeNull()

  const save = page.waitForResponse(
    (response) =>
      response.request().method() === 'PATCH' &&
      response.url().includes(`/tables/${tableId}/rows/`),
  )
  await page
    .locator('.row-expand')
    .locator(`[data-field-id="${field.id}"]`)
    .getByRole('button', { name: en.rowExpand.save, exact: true })
    .click()
  expect((await save).status()).toBe(200)
  const rows = await queryRows(page, viewId)
  expect(rows.rows[0]?.fields[field.id]).toEqual([imageFile, pdfFile])
  await page.reload()
  const imageLinks = page.locator('.field-attachment[download="pixel.png"]')
  const pdfLinks = page.locator('.field-attachment[download="document.pdf"]')
  await expect(imageLinks).toHaveCount(2)
  await expect(imageLinks.first().locator('img')).toBeVisible()
  await expect(pdfLinks).toHaveCount(2)
  await expect(pdfLinks.first()).toBeVisible()
  const download = await page.request.get(`/api/v1/attachments/${pdfFile.id}`)
  expect(download.status()).toBe(200)
  expect(download.headers()['content-disposition']).toContain('attachment')
  const thumbnail = await page.request.get(
    `/api/v1/attachments/${imageFile.id}/thumbnail`,
  )
  expect(thumbnail.status()).toBe(200)
  expect(thumbnail.headers()['content-type']).toContain('image/png')

  const outsiderName = `attout-${suffix}`
  await createUser(page, outsiderName)
  const outsiderContext = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL!,
    locale: 'en-US',
  })
  const outsider = await outsiderContext.newPage()
  try {
    await signInAs(outsider, outsiderName)
    const denied = await outsider.request.get(
      `/api/v1/attachments/${pdfFile.id}`,
    )
    expect(denied.status()).toBe(404)
  } finally {
    await outsiderContext.close()
  }

  await page
    .locator('.row-expand')
    .getByRole('button', { name: en.rowExpand.close, exact: true })
    .click()
  const priorResponse = await page.request.get('/api/v1/admin/settings')
  expect(priorResponse.status()).toBe(200)
  const prior = (await priorResponse.json()) as AdminSettings
  try {
    const changed = await page.request.patch('/api/v1/admin/settings', {
      headers: headers(),
      data: { uploadMaxMb: 1 },
    })
    expect(changed.status()).toBe(200)
    await openAttachmentEditor(page, viewId, limitField.id, limitField.name)
    expectHTTPFailure(
      page,
      'POST',
      `/api/v1/tables/${tableId}/fields/${limitField.id}/attachments`,
      413,
    )
    const tooLarge = page.waitForResponse(
      (response) =>
        response.request().method() === 'POST' &&
        response
          .url()
          .includes(`/tables/${tableId}/fields/${limitField.id}/attachments`),
    )
    await page
      .locator('.field-attachment-editor input[type="file"]')
      .setInputFiles({
        name: 'over-limit.pdf',
        mimeType: 'application/pdf',
        buffer: Buffer.alloc((1 << 20) + 1, 0x78),
      })
    expect((await tooLarge).status()).toBe(413)
    await expect(page.getByRole('alert')).toContainText(
      en.errors.ERR_UPLOAD_TOO_LARGE,
    )
  } finally {
    await restoreSettings(page, prior)
  }
})
