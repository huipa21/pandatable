import type { Page } from '@playwright/test'
import {
  test,
  expect,
  allowHTTPFailure,
  expectHTTPFailure,
  observeBrowser,
  expectBrowserClean,
  paceLogin,
} from './fixtures'
import type { DataRow, TableSchema } from '../src/api/types'
import { setLocale, setTheme, signOut } from './app-shell'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})
type HeldWindow = Window & { held?: boolean; release?: () => void }
async function holdResponse(page: Page, path: string, method: string) {
  await page.evaluate(
    ({ path, method }) => {
      const state = window as HeldWindow
      const original = window.fetch
      let release!: () => void
      const pending = new Promise<void>((resolve) => {
        release = resolve
      })
      state.held = false
      state.release = () => {
        window.fetch = original
        release()
      }
      window.fetch = async (...args: Parameters<typeof fetch>) => {
        const response = await original(...args)
        if (
          new URL(response.url).pathname === path &&
          (args[1]?.method ?? 'GET') === method
        ) {
          state.held = true
          await pending
        }
        return response
      }
    },
    { path, method },
  )
}

test('AT-313 real 390x844 cards, full-screen exact editors, defaults, paging, ACL and stale responses', async ({
  page,
  browser,
}) => {
  test.setTimeout(150_000)
  await page.setViewportSize({ width: 390, height: 844 })
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
  const h = headers()
  const suffix = Date.now()
  const workspace = await page.request.post('/api/v1/workspaces', {
    headers: h,
    data: { name: `Mobile ${suffix}` },
  })
  expect(workspace.status()).toBe(201)
  const wid = (await workspace.json()).workspace.id as string
  const book = await page.request.post(`/api/v1/workspaces/${wid}/books`, {
    headers: h,
    data: { name: 'Mobile book' },
  })
  expect(book.status()).toBe(201)
  const bid = (await book.json()).book.id as string
  const table = await page.request.post(`/api/v1/books/${bid}/tables`, {
    headers: h,
    data: { name: 'Mobile table' },
  })
  expect(table.status()).toBe(201)
  const tid = (await table.json()).table.id as string
  const schemaOf = async () => {
    const response = await page.request.get(`/api/v1/tables/${tid}/schema`)
    expect(response.status()).toBe(200)
    return (await response.json()) as TableSchema
  }
  const initial = await schemaOf()
  const name = initial.fields.find(
    (field) => field.id === initial.table.displayFieldId,
  )!
  expect(
    (
      await page.request.patch(`/api/v1/fields/${name.id}`, {
        headers: { ...h, 'If-Match': String(name.version) },
        data: { isRequired: true, isUnique: true },
      })
    ).status(),
  ).toBe(200)
  const choices = {
    choices: [
      { id: 'a', label: 'Alpha', color: 'red' },
      { id: 'b', label: 'Beta', color: 'blue' },
    ],
  }
  const specs = [
    {
      name: 'Name',
      kind: 'text',
      options: {},
      input: '移动 row',
      value: '移动 row',
    },
    {
      name: 'Notes',
      kind: 'long_text',
      options: {},
      input: '第一行\nsecond line',
      value: '第一行\nsecond line',
    },
    {
      name: 'Big',
      kind: 'integer',
      options: { size: 'big' },
      input: '9223372036854775807',
      value: '9223372036854775807',
    },
    {
      name: 'Exact',
      kind: 'decimal',
      options: { precision: 38, scale: 6 },
      input: '123456789012345678901234567890.123456',
      value: '123456789012345678901234567890.123456',
    },
    {
      name: 'Float',
      kind: 'float',
      options: { size: 'real' },
      input: '1.23456789',
      value: 1.2345679,
    },
    { name: 'Check', kind: 'checkbox', options: {}, input: '', value: true },
    {
      name: 'Date',
      kind: 'date',
      options: {},
      input: '2026-10-27',
      value: '2026-10-27',
    },
    {
      name: 'Instant',
      kind: 'datetime',
      options: {},
      input: '2026-10-27T15:00:00.123456+08:00',
      value: '2026-10-27T07:00:00.123456Z',
    },
    {
      name: 'Stage',
      kind: 'single_select',
      options: choices,
      input: 'Beta',
      value: 'Beta',
    },
    {
      name: 'Tags',
      kind: 'multi_select',
      options: choices,
      input: '',
      value: ['Alpha', 'Beta'],
    },
    {
      name: 'Email',
      kind: 'email',
      options: {},
      input: 'valid@example.com',
      value: 'valid@example.com',
    },
    {
      name: 'URL',
      kind: 'url',
      options: {},
      input: 'https://example.com/a',
      value: 'https://example.com/a',
    },
  ]
  for (const spec of specs.slice(1))
    expect(
      (
        await page.request.post(`/api/v1/tables/${tid}/fields`, {
          headers: h,
          data: { name: spec.name, uiType: spec.kind, options: spec.options },
        })
      ).status(),
    ).toBe(201)
  expect(
    (
      await page.request.post(`/api/v1/tables/${tid}/fields`, {
        headers: h,
        data: { name: 'Defaulted', uiType: 'integer', default: 7 },
      })
    ).status(),
  ).toBe(201)
  const schema = await schemaOf()
  const defaulted = schema.fields.find((field) => field.name === 'Defaulted')!
  const base = `/t/${tid}/v/${schema.views[0]!.id}`
  const apiRows = `/api/v1/tables/${tid}/rows`
  const mutations: { method: string; fields?: Record<string, unknown> }[] = []
  page.on('request', (request) => {
    if (
      new URL(request.url()).pathname === apiRows &&
      request.method() === 'POST'
    )
      mutations.push({
        method: 'POST',
        fields: request.postDataJSON().rows[0].fields,
      })
  })
  await page.goto(base)
  await expect(page.locator('.mobile-rows')).toBeVisible()
  await expect(page.locator('revo-grid')).toHaveCount(0)
  await page.getByRole('button', { name: en.grid.addRow, exact: true }).click()
  const dialog = page.locator('dialog.row-expand')
  await expect(dialog).toBeVisible()
  expect(await dialog.boundingBox()).toMatchObject({
    x: 0,
    y: 0,
    width: 390,
    height: 844,
  })
  await expect(dialog.locator('.row-field')).toHaveCount(18)
  expectHTTPFailure(page, 'POST', apiRows, 400)
  await dialog
    .getByRole('button', { name: en.mobileRows.create, exact: true })
    .click()
  await expect(dialog.getByRole('alert')).toHaveText(
    en.errors.ERR_REQUIRED_FIELD,
  )
  for (const spec of specs) {
    const field = schema.fields.find((field) => field.name === spec.name)!
    const section = dialog.locator(`[data-field-id="${field.id}"]`)
    await section
      .getByRole('button', {
        name: en.rowExpand.edit.replace('{name}', spec.name),
        exact: true,
      })
      .click()
    if (spec.kind === 'checkbox') await section.locator('input').click()
    else if (spec.kind === 'single_select')
      await section.locator('select').selectOption(spec.input)
    else if (spec.kind === 'multi_select') {
      await section.locator('select').selectOption(['Beta', 'Alpha'])
      await section
        .getByRole('button', { name: en.field.editor.apply, exact: true })
        .click()
    } else {
      await section.locator('input, textarea').fill(spec.input)
      await section
        .getByRole('button', { name: en.field.editor.apply, exact: true })
        .click()
    }
    await expect(section.locator('.field-editor')).toHaveCount(0)
  }
  expect(mutations).toHaveLength(1)
  await dialog
    .getByRole('button', { name: en.mobileRows.create, exact: true })
    .click()
  await expect(page).toHaveURL(/\/r\/[1-9]\d*$/)
  const rowId = new URL(page.url()).pathname.split('/').at(-1)!
  const rowPath = `${base}/r/${rowId}`
  const apiRow = `${apiRows}/${rowId}`
  expect(mutations).toHaveLength(2)
  expect(mutations[1]!.fields).not.toHaveProperty(defaulted.id)
  const savedRow = (await (await page.request.get(apiRow)).json()) as DataRow
  for (const spec of specs)
    expect(
      savedRow.fields[
        schema.fields.find((field) => field.name === spec.name)!.id
      ],
    ).toEqual(spec.value)
  expect(savedRow.fields[defaulted.id]).toBe(7)
  const nameSection = dialog.locator(`[data-field-id="${name.id}"]`)
  await nameSection.getByRole('button').click()
  await nameSection.locator('input').fill('Saved mobile')
  await nameSection.locator('input').press('Enter')
  await expect(
    dialog.getByText(en.rowExpand.saved, { exact: true }),
  ).toBeVisible()
  await page.reload()
  await expect(nameSection).toContainText('Saved mobile')
  const bigSection = dialog.locator(
    `[data-field-id="${schema.fields.find((field) => field.name === 'Big')!.id}"]`,
  )
  await bigSection.getByRole('button').click()
  await bigSection.locator('input').fill('invalid')
  await bigSection
    .getByRole('button', { name: en.rowExpand.save, exact: true })
    .click()
  await expect(bigSection.getByRole('alert')).toHaveText(
    en.field.error.invalid_integer,
  )
  await bigSection.locator('input').press('Escape')
  await dialog.getByRole('button', { name: en.rowExpand.close }).click()
  await expect(page.locator('.mobile-card')).toHaveCount(1)
  await expect(page.locator('.card-title')).toContainText('Saved mobile')
  await expect(page.locator('.card-field')).toHaveCount(4)
  await page
    .getByRole('button', { name: en.mobileRows.options, exact: true })
    .click()
  const sheet = page.getByRole('dialog', {
    name: en.mobileRows.options,
    exact: true,
  })
  await sheet.getByRole('checkbox', { name: 'Name', exact: true }).uncheck()
  await page.keyboard.press('Escape')
  await expect(sheet).not.toBeVisible()
  await expect(
    page.getByRole('button', { name: en.mobileRows.options, exact: true }),
  ).toBeFocused()
  await expect(page.locator('.card-title')).toContainText('Saved mobile')
  await page.locator('.mobile-card').focus()
  await page.keyboard.press('Enter')
  await expect(dialog).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(dialog).not.toBeVisible()

  const seed = await page.request.post(apiRows, {
    headers: h,
    data: {
      rows: Array.from({ length: 300 }, (_, index) => ({
        fields: { [name.id]: `Paging ${index}` },
      })),
    },
  })
  expect(seed.status()).toBe(201)
  const deletable = (await seed.json()).rows[0].id as string
  await page.reload()
  await expect(page.locator('.mobile-card')).toHaveCount(100)
  for (let index = 0; index < 3; index++) {
    await page
      .getByRole('button', { name: en.mobileRows.next, exact: true })
      .click()
    await expect(page.locator('.mobile-rows')).toHaveAttribute(
      'data-page',
      String(index + 1),
    )
    await expect(page.locator('.mobile-card')).toHaveCount(
      index === 2 ? 1 : 100,
    )
    expect(
      Number(
        await page.locator('.mobile-rows').getAttribute('data-cached-rows'),
      ),
    ).toBeLessThanOrEqual(300)
  }
  await setTheme(page, 'dark')
  await setLocale(page, 'zh-CN')
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await expect(
    page.getByRole('heading', { name: zh.mobileRows.title, exact: true }),
  ).toBeVisible()
  await page.locator('.mobile-card').click()
  await expect(dialog).toBeVisible()
  expect(await dialog.boundingBox()).toMatchObject({
    x: 0,
    y: 0,
    width: 390,
    height: 844,
  })
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true)
  expect(
    await dialog.evaluate(
      (element) => element.scrollWidth <= element.clientWidth,
    ),
  ).toBe(true)
  await dialog.getByRole('button', { name: zh.rowExpand.close }).click()
  await setLocale(page, 'en')
  await setTheme(page, 'light')

  await holdResponse(page, apiRow, 'GET')
  await page
    .locator('.mobile-paging')
    .getByRole('button', { name: en.mobileRows.previous })
    .click()
  await page
    .getByRole('button', { name: en.mobileRows.previous, exact: true })
    .click()
  await page
    .getByRole('button', { name: en.mobileRows.previous, exact: true })
    .click()
  await page
    .getByRole('button', {
      name: en.rowExpand.open.replace('{id}', rowId),
      exact: true,
    })
    .click()
  await expect
    .poll(() => page.evaluate(() => (window as HeldWindow).held))
    .toBe(true)
  await dialog.getByRole('button', { name: en.rowExpand.close }).click()
  await page.evaluate(() => (window as HeldWindow).release?.())
  await expect(dialog).toHaveCount(0)
  await page
    .getByRole('button', {
      name: en.rowExpand.open.replace('{id}', rowId),
      exact: true,
    })
    .click()
  await nameSection.getByRole('button').click()
  await nameSection.locator('input').fill('Late committed')
  await holdResponse(page, apiRow, 'PATCH')
  await nameSection
    .getByRole('button', { name: en.rowExpand.save, exact: true })
    .click()
  await expect
    .poll(() => page.evaluate(() => (window as HeldWindow).held))
    .toBe(true)
  await dialog.getByRole('button', { name: en.rowExpand.close }).click()
  await page.evaluate(() => (window as HeldWindow).release?.())
  await expect(dialog).toHaveCount(0)
  expect((await (await page.request.get(apiRow)).json()).fields[name.id]).toBe(
    'Late committed',
  )
  await page
    .locator(`[data-row-id="${deletable}"] .card-selection input`)
    .check()
  await page
    .getByRole('button', { name: en.mobileRows.options, exact: true })
    .click()
  await sheet
    .getByRole('button', {
      name: en.grid.deleteRows.replace('{count}', '1'),
      exact: true,
    })
    .click()
  const deleteDialog = page.getByRole('dialog', {
    name: en.grid.confirmDeleteTitle,
    exact: true,
  })
  expect((await page.request.get(`${apiRows}/${deletable}`)).status()).toBe(200)
  await deleteDialog
    .getByRole('button', { name: en.grid.deleteConfirm, exact: true })
    .click()
  await expect(deleteDialog).not.toBeVisible()
  expect((await page.request.get(`${apiRows}/${deletable}`)).status()).toBe(404)

  const userResponse = await page.request.post('/api/v1/admin/users', {
    headers: h,
    data: {
      username: `m${suffix}`,
      displayName: 'Mobile reader',
      password: 'Temporary-password-123',
      role: 'user',
    },
  })
  expect(userResponse.status()).toBe(201)
  const user = (await userResponse.json()).user as {
    id: string
    username: string
  }
  const grantData = {
    resourceType: 'table',
    resourceId: tid,
    principalType: 'user',
    principalId: user.id,
  }
  const grant = await page.request.put('/api/v1/grants', {
    headers: h,
    data: { ...grantData, level: 'read' },
  })
  expect(grant.status()).toBe(201)
  const readerContext = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL,
    viewport: { width: 390, height: 844 },
    locale: 'en-US',
  })
  try {
    expect(
      (
        await readerContext.request.post('/api/v1/auth/login', {
          headers: h,
          data: { login: user.username, password: 'Temporary-password-123' },
        })
      ).status(),
    ).toBe(200)
    expect(
      (
        await readerContext.request.post('/api/v1/auth/password', {
          headers: h,
          data: {
            current: 'Temporary-password-123',
            new: 'Reader-password-123',
          },
        })
      ).status(),
    ).toBe(200)
    expect(
      (
        await readerContext.request.post('/api/v1/auth/login', {
          headers: h,
          data: { login: user.username, password: 'Reader-password-123' },
        })
      ).status(),
    ).toBe(200)
    const reader = await readerContext.newPage()
    const pendingQueries = new Set<import('@playwright/test').Request>()
    reader.on('request', (request) => {
      if (
        request.method() === 'POST' &&
        new URL(request.url()).pathname ===
          `/api/v1/views/${schema.views[0]!.id}/query`
      )
        pendingQueries.add(request)
    })
    reader.on('requestfinished', (request) => pendingQueries.delete(request))
    reader.on('requestfailed', (request) => pendingQueries.delete(request))
    const diagnostics = await observeBrowser(reader)
    await reader.goto(base)
    if ((await reader.locator('html').getAttribute('lang')) === 'zh-CN')
      await setLocale(reader, 'en')
    await expect(reader.locator('.mobile-card')).toHaveCount(100)
    await expect(reader.locator('.mobile-add, .card-selection')).toHaveCount(0)
    await reader
      .getByRole('button', {
        name: en.rowExpand.open.replace('{id}', rowId),
        exact: true,
      })
      .click()
    const readerDialog = reader.locator('dialog.row-expand')
    await expect(
      readerDialog.getByText(en.rowExpand.readOnly, { exact: true }),
    ).toBeVisible()
    await expect(readerDialog.locator('.row-field button')).toHaveCount(0)
    expect(
      (
        await page.request.put('/api/v1/grants', {
          headers: h,
          data: { ...grantData, level: 'write' },
        })
      ).status(),
    ).toBe(200)
    await reader.reload()
    const readerName = readerDialog.locator(`[data-field-id="${name.id}"]`)
    await readerName.getByRole('button').click()
    await readerName.locator('input').fill('Writer mobile')
    await readerName
      .getByRole('button', { name: en.rowExpand.save, exact: true })
      .click()
    await expect(
      readerDialog.getByText(en.rowExpand.saved, { exact: true }),
    ).toBeVisible()
    await readerDialog.getByRole('button', { name: en.rowExpand.close }).click()
    await reader
      .getByRole('button', { name: en.grid.addRow, exact: true })
      .click()
    await expect(
      readerDialog.getByRole('button', {
        name: en.mobileRows.create,
        exact: true,
      }),
    ).toBeVisible()
    expect(
      (
        await page.request.put('/api/v1/grants', {
          headers: h,
          data: { ...grantData, level: 'read' },
        })
      ).status(),
    ).toBe(200)
    // Realtime may deliver the downgrade first; the dialog then denies locally.
    allowHTTPFailure(reader, 'POST', apiRows, 403)
    await readerDialog
      .getByRole('button', { name: en.mobileRows.create, exact: true })
      .click()
    await expect(readerDialog.getByRole('alert')).toHaveText(
      en.errors.ERR_PERMISSION_DENIED,
    )
    await readerDialog.getByRole('button', { name: en.rowExpand.close }).click()
    await expect(reader.locator('.mobile-card')).toHaveCount(100)
    await expect.poll(() => pendingQueries.size).toBe(0)
    // Realtime revocation can refetch the open table before navigation.
    allowHTTPFailure(reader, 'GET', '/api/v1/tables/*/schema', 404)
    allowHTTPFailure(reader, 'POST', '/api/v1/views/*/query', 404)
    expect(
      (
        await page.request.delete(
          `/api/v1/grants/${(await grant.json()).grant.id}`,
          { headers: h },
        )
      ).status(),
    ).toBe(200)
    await reader.goto(rowPath)
    await expect(
      reader.getByRole('heading', { name: en.nav.removedTitle, exact: true }),
    ).toBeVisible()
    await expect(reader.locator('.mobile-card, dialog.row-expand')).toHaveCount(
      0,
    )
    expectBrowserClean(reader, diagnostics)
  } finally {
    await readerContext.close()
  }

  await page.goto(rowPath)
  await expect(dialog).toBeVisible()
  await dialog.getByRole('button', { name: en.rowExpand.close }).click()
  await holdResponse(page, apiRow, 'GET')
  await page
    .getByRole('button', {
      name: en.rowExpand.open.replace('{id}', rowId),
      exact: true,
    })
    .click()
  await expect
    .poll(() => page.evaluate(() => (window as HeldWindow).held))
    .toBe(true)
  await dialog.getByRole('button', { name: en.rowExpand.close }).click()
  await signOut(page)
  await expect(page).toHaveURL(/\/login$/)
  await page.evaluate(() => (window as HeldWindow).release?.())
  await expect(page.locator('dialog.row-expand, .mobile-card')).toHaveCount(0)
})
