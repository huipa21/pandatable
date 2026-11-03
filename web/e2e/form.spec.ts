import type { Browser, BrowserContext, Page } from '@playwright/test'
import { expect, paceLogin, test } from './fixtures'
import { setLocale } from './app-shell'
import { openTableOptions } from './table-controls'
import en from '../src/i18n/en.json' with { type: 'json' }
import type { TableSchema } from '../src/api/types'

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

async function signInOwner(page: Page) {
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

async function createUser(page: Page, username: string) {
  const response = await page.request.post('/api/v1/admin/users', {
    headers: headers(),
    data: {
      username,
      displayName: username,
      role: 'user',
      password: 'Temporary-password-123',
    },
  })
  expect(response.status()).toBe(201)
  return (await response.json()).user as { id: string }
}

async function setLanguageEnglish(page: Page) {
  if ((await page.locator('html').getAttribute('lang')) === 'zh-CN')
    await setLocale(page, 'en')
}

async function signInParticipant(
  browser: Browser,
  username: string,
): Promise<{ context: BrowserContext; page: Page }> {
  const context = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL,
    locale: 'en-US',
    viewport: { width: 390, height: 844 },
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
        data: {
          current: 'Temporary-password-123',
          new: 'Form-password-123',
        },
      })
    ).status(),
  ).toBe(200)
  await paceLogin()
  expect(
    (
      await context.request.post('/api/v1/auth/login', {
        headers: headers(),
        data: { login: username, password: 'Form-password-123' },
      })
    ).status(),
  ).toBe(200)
  return { context, page: await context.newPage() }
}

test('AT-511 configures a form, persists it, submits rows, and disables read-only mobile users', async ({
  page,
  browser,
}) => {
  test.setTimeout(240_000)
  await page.setViewportSize({ width: 1280, height: 900 })
  await signInOwner(page)
  const stamp = Date.now()
  const workspaceResponse = await page.request.post('/api/v1/workspaces', {
    headers: headers(),
    data: { name: `Forms ${stamp}` },
  })
  expect(workspaceResponse.status()).toBe(201)
  const workspace = (await workspaceResponse.json()).workspace as { id: string }
  const bookResponse = await page.request.post(
    `/api/v1/workspaces/${workspace.id}/books`,
    { headers: headers(), data: { name: `Form book ${stamp}` } },
  )
  expect(bookResponse.status()).toBe(201)
  const book = (await bookResponse.json()).book as { id: string }
  const tableResponse = await page.request.post(
    `/api/v1/books/${book.id}/tables`,
    { headers: headers(), data: { name: `Form table ${stamp}` } },
  )
  expect(tableResponse.status()).toBe(201)
  const schema = (await tableResponse.json()) as TableSchema
  const tableId = schema.table.id
  const fieldResponse = await page.request.post(
    `/api/v1/tables/${tableId}/fields`,
    { headers: headers(), data: { name: 'Subject', uiType: 'text' } },
  )
  expect(fieldResponse.status()).toBe(201)

  const writerName = `formwrite-${stamp}`
  const readerName = `formread-${stamp}`
  const writer = await createUser(page, writerName)
  const reader = await createUser(page, readerName)
  for (const grant of [
    { principalId: writer.id, level: 'write' },
    { principalId: reader.id, level: 'read' },
  ]) {
    const response = await page.request.put('/api/v1/grants', {
      headers: headers(),
      data: {
        resourceType: 'table',
        resourceId: tableId,
        principalType: 'user',
        ...grant,
      },
    })
    expect(response.status()).toBe(201)
  }

  await page.reload()
  await page.goto(`/t/${tableId}/v/${schema.views[0]!.id}`)
  await page.setViewportSize({ width: 390, height: 844 })
  await openTableOptions(page)
  await expect(page.locator('.view-create')).toBeVisible()
  const viewCreate = page.locator('.view-create')
  await viewCreate.getByLabel(en.nav.viewName, { exact: true }).fill('Intake')
  await viewCreate.getByLabel(en.nav.viewScopeLabel).selectOption('shared')
  await viewCreate.getByLabel(en.form.viewType).selectOption('form')
  await viewCreate.getByRole('button', { name: en.nav.createView }).click()
  await expect(page).toHaveURL(/\/t\/[^/]+\/v\/[^/]+$/)
  await page.locator('.table-management > summary').click()
  const builder = page.locator('.form-builder')
  await builder.locator('summary').click()
  await builder
    .getByLabel(en.form.title, { exact: true })
    .fill('Request intake')
  await builder
    .getByLabel(en.form.description, { exact: true })
    .fill('Tell us what you need.')
  await builder.getByRole('checkbox', { name: 'Subject', exact: true }).check()
  await builder.getByLabel(en.form.fieldLabel, { exact: true }).fill('Topic')
  await builder
    .getByLabel(en.form.fieldHelp, { exact: true })
    .fill('A short summary.')
  await builder.getByLabel(en.form.required, { exact: true }).check()
  await builder.getByLabel(en.form.afterSubmit).selectOption('message')
  await builder.getByRole('button', { name: en.form.save }).click()

  await expect(
    page.getByRole('heading', { name: 'Request intake' }),
  ).toBeVisible()
  await page.getByRole('button', { name: en.form.submit, exact: true }).click()
  expect(
    await page
      .getByRole('textbox', { name: 'Topic' })
      .evaluate((element) => !(element as HTMLInputElement).validity.valid),
  ).toBe(true)
  await page
    .getByRole('textbox', { name: 'Topic', exact: true })
    .fill('Owner request')
  await page.getByRole('button', { name: en.form.submit, exact: true }).click()
  await expect(page.getByRole('status')).toContainText(en.form.submitted)

  const schemaResponse = await page.request.get(
    `/api/v1/tables/${tableId}/schema`,
  )
  const reloadedSchema = (await schemaResponse.json()) as TableSchema
  const formView = reloadedSchema.views.find(
    (candidate) => candidate.type === 'form',
  )!
  expect(formView.config.form).toMatchObject({
    title: 'Request intake',
    description: 'Tell us what you need.',
    fields: [{ label: 'Topic', help: 'A short summary.', required: true }],
    afterSubmit: 'message',
  })
  const formFieldID = formView.config.form!.fields[0]!.id
  await page.reload()
  await expect(
    page.getByRole('heading', { name: 'Request intake' }),
  ).toBeVisible()
  const rowsResponse = await page.request.post(
    `/api/v1/views/${formView.id}/query`,
    { headers: headers(), data: {} },
  )
  expect(rowsResponse.status()).toBe(200)
  const ownerRows = (await rowsResponse.json()).rows as {
    fields: Record<string, unknown>
  }[]
  expect(ownerRows).toHaveLength(1)
  expect(ownerRows[0]!.fields[formFieldID]).toBe('Owner request')

  let writerContext: BrowserContext | undefined
  let readerContext: BrowserContext | undefined
  try {
    const writerSession = await signInParticipant(browser, writerName)
    writerContext = writerSession.context
    const writerPage = writerSession.page
    await writerPage.goto(`/t/${tableId}/v/${formView.id}`)
    await setLanguageEnglish(writerPage)
    await writerPage
      .getByRole('textbox', { name: 'Topic', exact: true })
      .fill('Writer request')
    await writerPage.getByRole('button', { name: en.form.submit }).click()
    await expect(writerPage.getByRole('status')).toContainText(
      en.form.submitted,
    )
    const writerRowsResponse = await writerPage.request.post(
      `/api/v1/views/${formView.id}/query`,
      { headers: headers(), data: {} },
    )
    expect(writerRowsResponse.status()).toBe(200)
    const writerRows = (await writerRowsResponse.json()).rows as {
      fields: Record<string, unknown>
    }[]
    expect(writerRows).toHaveLength(2)
    expect(writerRows[1]!.fields[formFieldID]).toBe('Writer request')
    expect(
      await writerPage.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true)

    const readerSession = await signInParticipant(browser, readerName)
    readerContext = readerSession.context
    const readerPage = readerSession.page
    await readerPage.goto(`/t/${tableId}/v/${formView.id}`)
    await setLanguageEnglish(readerPage)
    await expect(readerPage.getByText(en.form.readOnly)).toBeVisible()
    await expect(
      readerPage.getByRole('button', { name: en.form.submit }),
    ).toBeDisabled()
    expect(
      await readerPage.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true)
  } finally {
    await writerContext?.close()
    await readerContext?.close()
  }
})
