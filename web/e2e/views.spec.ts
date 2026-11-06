import type { Browser, BrowserContext, Page } from '@playwright/test'
import { expect, observeBrowser, paceLogin, test } from './fixtures'
import { setLocale } from './app-shell'
import {
  openCreateView,
  openTableOptions,
  sidebarTable,
  sidebarView,
} from './table-controls'
import en from '../src/i18n/en.json' with { type: 'json' }

test.use({ actionTimeout: 10_000 })
import type { TableSchema, ViewColumns, ViewMutation } from '../src/api/types'

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
  await expect(page).toHaveURL(/\/(?:home|w\/[^/]+|change-password)$/)
}

async function signInParticipant(
  browser: Browser,
  username: string,
): Promise<{ context: BrowserContext; page: Page }> {
  const context = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL,
    locale: 'en-US',
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
    await setLocale(page, 'en')
}

async function viewActions(page: Page, tableId: string, name: string) {
  const row = (await sidebarView(page, tableId, name)).locator('..')
  const actions = row.locator(':scope > .node-actions')
  if ((await actions.getAttribute('open')) === null)
    await actions.locator(':scope > summary').click()
  return row
}

test('UX-05 creates every supported view under the selected sidebar table', async ({
  page,
  isMobile,
}) => {
  test.setTimeout(240_000)
  await signInOwner(page)
  const stamp = Date.now()
  const workspace = await createResource<{ workspace: { id: string } }>(
    page,
    '/api/v1/workspaces',
    { name: `View navigation ${stamp}` },
  )
  const book = await createResource<{ book: { id: string } }>(
    page,
    `/api/v1/workspaces/${workspace.workspace.id}/books`,
    { name: `View navigation book ${stamp}` },
  )
  const source = await createResource<TableSchema>(
    page,
    `/api/v1/books/${book.book.id}/tables`,
    { name: 'Source table' },
  )
  const target = await createResource<TableSchema>(
    page,
    `/api/v1/books/${book.book.id}/tables`,
    { name: 'Target table' },
  )
  const sourcePath = `/t/${source.table.id}/v/${source.views[0]!.id}`
  await page.goto(sourcePath)
  const table = await sidebarTable(page, target.table.id)
  const summary = table.locator(':scope > .node-actions > summary')
  const interaction = isMobile ? 'touch' : 'keyboard'
  const cancelled = await openCreateView(page, target.table.id, interaction)
  await cancelled
    .getByRole('button', { name: en.admin.cancel, exact: true })
    .click()
  await expect(summary).toBeFocused()
  await expect(page).toHaveURL(new RegExp(`${sourcePath}$`))
  await expect(page.locator('.resource-page .view-create')).toHaveCount(0)
  await expect(page.locator('.resource-page .view-links')).toHaveCount(0)

  for (const type of ['grid', 'form', 'kanban', 'gallery'] as const) {
    const name = `Sidebar ${type}`
    const dialog = await openCreateView(page, target.table.id, interaction)
    await dialog.getByLabel(en.nav.viewName, { exact: true }).fill(name)
    await dialog.getByLabel(en.nav.viewScopeLabel).selectOption('shared')
    await dialog.getByLabel(en.form.viewType).selectOption(type)
    const [response] = await Promise.all([
      page.waitForResponse(
        (result) =>
          result.request().method() === 'POST' &&
          new URL(result.url()).pathname ===
            `/api/v1/tables/${target.table.id}/views`,
      ),
      dialog
        .getByRole('button', { name: en.nav.createView, exact: true })
        .click(),
    ])
    expect(response.status()).toBe(201)
    const created = (await response.json()) as ViewMutation
    expect(created.view.type).toBe(type)
    await expect(page).toHaveURL(
      new RegExp(`/t/${target.table.id}/v/${created.view.id}$`),
    )
    await expect(
      await sidebarView(page, target.table.id, name),
    ).toHaveAttribute('aria-current', 'page')
    await page.reload()
    await expect(await sidebarView(page, target.table.id, name)).toBeVisible()
  }
})

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
    await openTableOptions(writerPage)
    await expect(writerPage.locator('.schema-summary')).toContainText(
      en.nav.level.write,
    )

    let createForm = await openCreateView(writerPage, tableId)
    await createForm
      .getByLabel(en.nav.viewName, { exact: true })
      .fill('Writer personal')
    await createForm
      .getByLabel(en.nav.viewScopeLabel)
      .selectOption('personal', { timeout: 10_000 })
    const initialUrl = writerPage.url()
    await createForm.getByRole('button', { name: en.nav.createView }).click()
    await expect(writerPage).not.toHaveURL(initialUrl)
    createForm = await openCreateView(writerPage, tableId)
    await createForm
      .getByLabel(en.nav.viewName, { exact: true })
      .fill('Shared board')
    await createForm
      .getByLabel(en.nav.viewScopeLabel)
      .selectOption('shared', { timeout: 10_000 })
    const personalUrl = writerPage.url()
    await createForm.getByRole('button', { name: en.nav.createView }).click()
    await expect(writerPage).not.toHaveURL(personalUrl)
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
    const readerCreate = await openCreateView(readerPage, tableId)
    await expect(
      readerCreate.getByRole('option', { name: en.nav.viewScope.shared }),
    ).toHaveAttribute('disabled', '')
    await readerCreate
      .getByLabel(en.nav.viewName, { exact: true })
      .fill('Reader personal')
    await readerCreate
      .getByRole('button', { name: en.nav.createView, exact: true })
      .click()
    await expect(readerPage).not.toHaveURL(new RegExp(`/v/${shared.id}$`))
    const readerPrivateSchema = (await (
      await readerPage.request.get(`/api/v1/tables/${tableId}/schema`)
    ).json()) as TableSchema
    const readerPrivate = readerPrivateSchema.views.find(
      (item) => item.name === 'Reader personal',
    )!
    const ownerSchema = (await (
      await page.request.get(`/api/v1/tables/${tableId}/schema`)
    ).json()) as TableSchema
    expect(ownerSchema.views.some((item) => item.id === readerPrivate.id)).toBe(
      false,
    )
    expect(
      ownerSchema.views.some((item) => item.id === privateWriterView.id),
    ).toBe(false)
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
    const lockRow = await viewActions(ownerPage, tableId, 'Shared board')
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
    await readerPage.locator('.grid-filter-trigger').click()
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
    await expect(readerPage).not.toHaveURL(new RegExp(`/v/${shared.id}$`))
    readerSchemaResponse = await readerPage.request.get(
      `/api/v1/tables/${tableId}/schema`,
    )
    readerSchema = (await readerSchemaResponse.json()) as TableSchema
    const savedPersonal = readerSchema.views.find(
      (item) =>
        item.scope === 'personal' &&
        item.name === en.nav.viewCopyName.replace('{name}', 'Shared board'),
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
    const unlockRow = await viewActions(ownerPage, tableId, 'Shared board')
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
    const sharedRow = await viewActions(writerPage, tableId, 'Shared board')
    await sharedRow.getByRole('button', { name: en.nav.renameView }).click()
    const renameDialog = writerPage.getByRole('dialog', {
      name: en.nav.renameView,
    })
    await renameDialog
      .getByLabel(en.nav.viewName, { exact: true })
      .fill('Renamed board')
    await renameDialog
      .getByRole('button', { name: en.admin.confirm, exact: true })
      .click()
    await expect(
      await sidebarView(writerPage, tableId, 'Renamed board'),
    ).toBeVisible()
    const renamedRow = await viewActions(writerPage, tableId, 'Renamed board')
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
    const movedRow = await viewActions(writerPage, tableId, 'Renamed board')
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
    await expect(writerPage).toHaveURL(
      new RegExp(`/v/${duplicateMutation.view.id}$`),
    )

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
    const copyRow = await viewActions(writerPage, tableId, copy.name)
    await copyRow.getByRole('button', { name: en.nav.deleteView }).click()
    const deleteDialog = writerPage.getByRole('dialog', {
      name: en.nav.deleteView,
    })
    await deleteDialog
      .getByRole('button', { name: en.admin.confirm, exact: true })
      .click()
    await expect(deleteDialog).not.toBeVisible()
    await expect(writerPage).not.toHaveURL(new RegExp(`/v/${copy.id}$`))
    await expect(
      writerPage.locator(`[data-nav-id="${tableId}"] > .nav-row .table-toggle`),
    ).toBeFocused()
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
