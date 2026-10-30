import type { Browser, BrowserContext, Page } from '@playwright/test'
import { expect, observeBrowser, paceLogin, test } from './fixtures'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }
import type { TableSchema, ViewColumns, ViewMutation } from '../src/api/types'

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

async function signInOwner(page: Page) {
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

async function signInParticipant(
  browser: Browser,
  username: string,
): Promise<{ context: BrowserContext; page: Page }> {
  const context = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL,
    viewport: { width: 1280, height: 900 },
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
          new: 'Views-password-123',
        },
      })
    ).status(),
  ).toBe(200)
  await paceLogin()
  expect(
    (
      await context.request.post('/api/v1/auth/login', {
        headers: headers(),
        data: { login: username, password: 'Views-password-123' },
      })
    ).status(),
  ).toBe(200)
  return { context, page: await context.newPage() }
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
  return (await response.json()).user as { id: string; username: string }
}

async function createResource<T>(
  page: Page,
  path: string,
  data: unknown,
  status = 201,
): Promise<T> {
  const response = await page.request.post(path, { headers: headers(), data })
  expect(response.status()).toBe(status)
  return response.json()
}

async function setLanguageEnglish(page: Page) {
  if ((await page.locator('html').getAttribute('lang')) === 'zh-CN')
    await page
      .getByRole('button', { name: zh.common.languages.en, exact: true })
      .click()
}

async function viewActions(page: Page, name: string) {
  const row = page
    .locator('.view-links > div')
    .filter({ has: page.getByRole('link', { name, exact: true }) })
  const actions = row.locator('details')
  if ((await actions.getAttribute('open')) === null)
    await actions.locator('summary').click()
  return row
}

test('AT-506–508 manages private/shared views, locked overrides, ordering, and version conflicts', async ({
  page,
  browser,
  isMobile,
}) => {
  test.setTimeout(240_000)
  test.skip(isMobile, 'The view config editor is covered on desktop.')
  await page.setViewportSize({ width: 1280, height: 900 })
  await signInOwner(page)
  const stamp = Date.now()
  const writerName = `viewswrite-${stamp}`
  const readerName = `viewsread-${stamp}`
  const writer = await createUser(page, writerName)
  const reader = await createUser(page, readerName)
  const workspace = await createResource<{ workspace: { id: string } }>(
    page,
    '/api/v1/workspaces',
    { name: `Views ${stamp}` },
  )
  const book = await createResource<{ book: { id: string } }>(
    page,
    `/api/v1/workspaces/${workspace.workspace.id}/books`,
    { name: `Views book ${stamp}` },
  )
  const schema = await createResource<TableSchema>(
    page,
    `/api/v1/books/${book.book.id}/tables`,
    { name: `Views table ${stamp}` },
  )
  const tableId = schema.table.id
  const initialView = schema.views[0]!
  const statusFieldResponse = await createResource<{
    field: TableSchema['fields'][number]
  }>(page, `/api/v1/tables/${tableId}/fields`, {
    name: 'Status',
    uiType: 'text',
  })
  const statusField = statusFieldResponse.field
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

  let writerContext: BrowserContext | undefined
  let readerContext: BrowserContext | undefined
  try {
    const writerSession = await signInParticipant(browser, writerName)
    writerContext = writerSession.context
    const writerPage = writerSession.page
    const writerDiagnostics = await observeBrowser(writerPage)
    await setLanguageEnglish(writerPage)
    await writerPage.goto(`/t/${tableId}/v/${initialView.id}`)
    await expect(writerPage.locator('.schema-summary')).toContainText(
      en.nav.level.write,
    )

    const createForm = writerPage.locator('.view-create')
    await createForm
      .getByLabel(en.nav.viewName, { exact: true })
      .fill('Writer personal')
    await createForm.locator('select').selectOption('personal')
    await createForm.getByRole('button', { name: en.nav.createView }).click()
    await expect(writerPage).toHaveURL(/\/t\/[^/]+\/v\/[^/]+$/)

    await createForm
      .getByLabel(en.nav.viewName, { exact: true })
      .fill('Shared board')
    await createForm.locator('select').selectOption('shared')
    await createForm.getByRole('button', { name: en.nav.createView }).click()
    const writerSchemaResponse = await writerPage.request.get(
      `/api/v1/tables/${tableId}/schema`,
    )
    expect(writerSchemaResponse.status()).toBe(200)
    let writerSchema = (await writerSchemaResponse.json()) as TableSchema
    const shared = writerSchema.views.find(
      (item) => item.name === 'Shared board',
    )!
    const privateWriterView = writerSchema.views.find(
      (item) => item.name === 'Writer personal',
    )!

    const readerSession = await signInParticipant(browser, readerName)
    readerContext = readerSession.context
    const readerPage = readerSession.page
    const readerDiagnostics = await observeBrowser(readerPage)
    await setLanguageEnglish(readerPage)
    await readerPage.goto(`/t/${tableId}/v/${shared.id}`)
    await expect(readerPage.locator('.schema-summary')).toContainText(
      en.nav.level.read,
    )
    let readerSchemaResponse = await readerPage.request.get(
      `/api/v1/tables/${tableId}/schema`,
    )
    let readerSchema = (await readerSchemaResponse.json()) as TableSchema
    expect(
      readerSchema.views.some((item) => item.id === privateWriterView.id),
    ).toBe(false)
    expect(readerSchema.views.some((item) => item.id === shared.id)).toBe(true)
    expect(
      (
        await readerPage.request.post(`/api/v1/tables/${tableId}/views`, {
          headers: headers(),
          data: { name: 'Forbidden shared', scope: 'shared' },
        })
      ).status(),
    ).toBe(403)

    const ownerPage = page
    await ownerPage.goto(`/t/${tableId}/v/${shared.id}`)
    const lockRow = await viewActions(ownerPage, 'Shared board')
    await lockRow.getByRole('button', { name: en.nav.lockView }).click()
    await expect(
      lockRow.getByRole('button', { name: en.nav.unlockView }),
    ).toBeVisible()
    const lockedResponse = await ownerPage.request.get(
      `/api/v1/tables/${tableId}/schema`,
    )
    const lockedSchema = (await lockedResponse.json()) as TableSchema
    const lockedView = lockedSchema.views.find((item) => item.id === shared.id)!
    expect(lockedView.locked).toBe(true)
    expect(
      (
        await writerPage.request.patch(`/api/v1/views/${shared.id}`, {
          headers: { ...headers(), 'If-Match': String(lockedView.version) },
          data: { name: 'Writer cannot rename locked view' },
        })
      ).status(),
    ).toBe(403)

    await readerPage.goto(`/t/${tableId}/v/${shared.id}`)
    await readerPage.locator('.view-filter > summary').click()
    const filter = readerPage.locator('.filter-group').first()
    await filter
      .getByRole('button', { name: en.filter.addCondition, exact: true })
      .click()
    const condition = filter.locator('.filter-item').first()
    await condition.locator('select').nth(0).selectOption(statusField.id)
    await condition.locator('select').nth(1).selectOption('is')
    await condition.getByRole('textbox').fill('Temporary')
    await readerPage.getByRole('button', { name: en.filter.apply }).click()
    const savePersonal = readerPage.getByRole('button', {
      name: en.nav.saveAsPersonalView,
    })
    await expect(savePersonal).toBeVisible()
    await savePersonal.click()
    await expect(readerPage).toHaveURL(/\/t\/[^/]+\/v\/[^/]+$/)
    readerSchemaResponse = await readerPage.request.get(
      `/api/v1/tables/${tableId}/schema`,
    )
    readerSchema = (await readerSchemaResponse.json()) as TableSchema
    const savedPersonal = readerSchema.views.find(
      (item) => item.scope === 'personal' && item.id !== initialView.id,
    )!
    expect(savedPersonal.config.filter).toMatchObject({
      op: 'and',
      items: [
        {
          field: statusField.id,
          cmp: 'is',
          value: 'Temporary',
        },
      ],
    })
    writerSchema = (await (
      await writerPage.request.get(`/api/v1/tables/${tableId}/schema`)
    ).json()) as TableSchema
    expect(
      writerSchema.views.some((item) => item.id === savedPersonal.id),
    ).toBe(false)

    await ownerPage.goto(`/t/${tableId}/v/${shared.id}`)
    const unlockRow = await viewActions(ownerPage, 'Shared board')
    await expect(
      unlockRow.getByRole('button', { name: en.nav.unlockView }),
    ).toBeVisible()
    const unlockResponsePromise = ownerPage.waitForResponse(
      (response) => response.request().method() === 'PATCH',
      { timeout: 10_000 },
    )
    await unlockRow.getByRole('button', { name: en.nav.unlockView }).click()
    const unlockResponse = await unlockResponsePromise
    expect(unlockResponse.status()).toBe(200)
    expect(unlockResponse.url()).toContain(shared.id)
    expect(((await unlockResponse.json()) as ViewMutation).view.locked).toBe(
      false,
    )
    await expect(
      unlockRow.getByRole('button', { name: en.nav.lockView }),
    ).toBeVisible()
    const currentResponse = await ownerPage.request.get(
      `/api/v1/tables/${tableId}/schema`,
    )
    const currentSchema = (await currentResponse.json()) as TableSchema
    const current = currentSchema.views.find((item) => item.id === shared.id)!
    expect(current.locked).toBe(false)

    const makeConfig = (width: number): ViewColumns => ({
      ...current.config,
      fields: current.config.fields.map((field) =>
        field.id === schema.table.displayFieldId ? { ...field, width } : field,
      ),
    })
    const contenders = [
      { client: ownerPage.request, config: makeConfig(250) },
      { client: writerPage.request, config: makeConfig(300) },
    ]
    const conflictResponses = await Promise.all(
      contenders.map(({ client, config }) =>
        client.patch(`/api/v1/views/${shared.id}`, {
          headers: { ...headers(), 'If-Match': String(current.version) },
          data: { config },
        }),
      ),
    )
    expect(
      conflictResponses.map((response) => response.status()).sort(),
    ).toEqual([200, 409])
    const conflictIndex = conflictResponses.findIndex(
      (response) => response.status() === 409,
    )
    const conflict = contenders[conflictIndex]!
    const refreshedResponse = await conflict.client.get(
      `/api/v1/tables/${tableId}/schema`,
    )
    const refreshedSchema = (await refreshedResponse.json()) as TableSchema
    const refreshed = refreshedSchema.views.find(
      (item) => item.id === shared.id,
    )!
    const reapplied = await conflict.client.patch(
      `/api/v1/views/${shared.id}`,
      {
        headers: { ...headers(), 'If-Match': String(refreshed.version) },
        data: { config: conflict.config },
      },
    )
    expect(reapplied.status()).toBe(200)
    const finalSchema = (await (
      await conflict.client.get(`/api/v1/tables/${tableId}/schema`)
    ).json()) as TableSchema
    expect(
      finalSchema.views
        .find((item) => item.id === shared.id)!
        .config.fields.find((field) => field.id === schema.table.displayFieldId)
        ?.width,
    ).toBe(
      conflict.config.fields.find(
        (field) => field.id === schema.table.displayFieldId,
      )?.width,
    )

    await writerPage.goto(`/t/${tableId}/v/${shared.id}`)
    const sharedRow = await viewActions(writerPage, 'Shared board')
    await sharedRow
      .getByLabel(en.nav.viewName, { exact: true })
      .fill('Renamed board')
    await sharedRow.getByRole('button', { name: en.nav.renameView }).click()
    await expect(
      writerPage.getByRole('link', { name: 'Renamed board', exact: true }),
    ).toBeVisible()
    const renamedRow = await viewActions(writerPage, 'Renamed board')
    const moveUpLabel = en.nav.moveViewUp.replace('{name}', 'Renamed board')
    await renamedRow.getByRole('button', { name: moveUpLabel }).click()
    await expect
      .poll(async () => {
        const latest = (await (
          await writerPage.request.get(`/api/v1/tables/${tableId}/schema`)
        ).json()) as TableSchema
        const moved = latest.views.find((item) => item.id === shared.id)!
        const personal = latest.views.find(
          (item) => item.id === privateWriterView.id,
        )!
        return moved.position < personal.position
      })
      .toBe(true)
    const movedRow = await viewActions(writerPage, 'Renamed board')
    const duplicateResponsePromise = writerPage.waitForResponse(
      (response) =>
        response.request().method() === 'POST' &&
        response.url().endsWith(`/api/v1/views/${shared.id}/duplicate`),
      { timeout: 10_000 },
    )
    await movedRow
      .getByRole('button', { name: en.nav.duplicatePersonalView })
      .click()
    const duplicateResponse = await duplicateResponsePromise
    expect(duplicateResponse.status()).toBe(201)
    const duplicateMutation = (await duplicateResponse.json()) as ViewMutation
    expect(duplicateMutation.view.scope).toBe('personal')
    await expect(writerPage).toHaveURL(/\/t\/[^/]+\/v\/[^/]+$/)

    const duplicateSchema = (await (
      await writerPage.request.get(`/api/v1/tables/${tableId}/schema`)
    ).json()) as TableSchema
    expect(
      duplicateSchema.views.some(
        (item) => item.id === duplicateMutation.view.id,
      ),
      JSON.stringify(
        duplicateSchema.views.map(({ id, scope, name }) => ({
          id,
          scope,
          name,
        })),
      ),
    ).toBe(true)
    const writerCopies = duplicateSchema.views.filter(
      (item) => item.scope === 'personal' && item.id !== privateWriterView.id,
    )
    expect(writerCopies.length).toBeGreaterThan(0)
    const copy = writerCopies[0]!
    writerPage.on('dialog', (dialog) => dialog.accept())
    const copyRow = await viewActions(writerPage, copy.name)
    await copyRow.getByRole('button', { name: en.nav.deleteView }).click()
    await expect
      .poll(async () => {
        const res = await writerPage.request.get(
          `/api/v1/tables/${tableId}/schema`,
        )
        const latest = (await res.json()) as TableSchema
        return latest.views.some((item) => item.id === copy.id)
      })
      .toBe(false)

    expect(writerDiagnostics.errors).toEqual([])
    expect(readerDiagnostics.errors).toEqual([])
  } finally {
    await writerContext?.close()
    await readerContext?.close()
  }
})
