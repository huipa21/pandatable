import type { Browser, BrowserContext, Page } from '@playwright/test'
import {
  allowHTTPFailure,
  expect,
  expectHTTPFailure,
  observeBrowser,
  paceLogin,
  test,
} from './fixtures'
import type {
  DataRow,
  TableField,
  TableSchema,
  TableView,
} from '../src/api/types'
import { accountLink, setLocale } from './app-shell'
import { openFieldPanel, sidebarView } from './table-controls'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

async function post<T>(page: Page, path: string, data: unknown): Promise<T> {
  const response = await page.request.post(path, { headers: headers(), data })
  expect(response.status(), path).toBe(201)
  return response.json()
}

async function login(page: Page) {
  await paceLogin(page)
  await page.goto('/login')
  if ((await page.locator('html').getAttribute('lang')) === 'zh-CN')
    await setLocale(page, 'en')
  await page.getByLabel(en.auth.login, { exact: true }).fill('first-admin')
  await page
    .getByLabel(en.auth.password, { exact: true })
    .fill('Setup-password-123')
  await page.getByRole('button', { name: en.auth.signIn, exact: true }).click()
  await expect(page).toHaveURL(/\/(?:home|w\/[^/]+)$/)
}

async function navigation(page: Page, isMobile: boolean) {
  if (isMobile && !(await page.locator('.nav-drawer').isVisible()))
    await page.getByRole('button', { name: en.nav.open, exact: true }).click()
  return page.getByRole('navigation', { name: en.nav.title, exact: true })
}

async function openTrash(page: Page) {
  await (await accountLink(page, en.trash.title)).click()
  await expect(page).toHaveURL(/\/trash$/)
  await expect(
    page.getByRole('heading', { name: en.trash.title }),
  ).toBeVisible()
}

async function restore(page: Page, id: string, isMobile: boolean) {
  const button = page.locator(`[data-trash-id="${id}"] button`)
  await expect(button).toBeVisible()
  await expect(button).toBeEnabled()
  if (isMobile) await button.tap()
  else await button.press('Enter')
  await expect(page.locator(`[data-trash-id="${id}"]`)).toHaveCount(0)
}

async function participant(browser: Browser, username: string) {
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
          new: 'Trash-password-123',
        },
      })
    ).status(),
  ).toBe(200)
  await paceLogin()
  expect(
    (
      await context.request.post('/api/v1/auth/login', {
        headers: headers(),
        data: { login: username, password: 'Trash-password-123' },
      })
    ).status(),
  ).toBe(200)
  return { context, page: await context.newPage() }
}

function subscriptions(page: Page) {
  const tables = new Set<string>()
  page.on('websocket', (socket) => {
    socket.on('framereceived', (frame) => {
      if (typeof frame.payload !== 'string') return
      const event = JSON.parse(frame.payload) as {
        t?: string
        tableId?: string
      }
      if (event.t === 'subscribed' && event.tableId) tables.add(event.tableId)
    })
  })
  return tables
}

test('UX-05 table trash revokes all descendant views and restore preserves them', async ({
  page,
  browser,
  isMobile,
}) => {
  test.setTimeout(240_000)
  test.skip(
    isMobile,
    'The multi-session descendant-access check uses desktop clients.',
  )
  const ownerSubscriptions = subscriptions(page)
  await login(page)
  const suffix = Date.now()
  const workspace = await post<{ workspace: { id: string } }>(
    page,
    '/api/v1/workspaces',
    { name: `View trash workspace ${suffix}` },
  )
  const book = await post<{ book: { id: string } }>(
    page,
    `/api/v1/workspaces/${workspace.workspace.id}/books`,
    { name: 'View trash book' },
  )
  const schema = await post<TableSchema>(
    page,
    `/api/v1/books/${book.book.id}/tables`,
    { name: 'View trash table' },
  )
  const tableId = schema.table.id
  const shared = await post<{ view: TableView }>(
    page,
    `/api/v1/tables/${tableId}/views`,
    { name: 'Retained shared', scope: 'shared', type: 'grid' },
  )
  const ownerPrivate = await post<{ view: TableView }>(
    page,
    `/api/v1/tables/${tableId}/views`,
    { name: 'Owner private', scope: 'personal', type: 'grid' },
  )
  const fieldId = schema.table.displayFieldId!
  const row = (
    await post<{ rows: DataRow[] }>(page, `/api/v1/tables/${tableId}/rows`, {
      rows: [{ fields: { [fieldId]: 'Retained descendant data' } }],
    })
  ).rows[0]!
  const users: { id: string; username: string }[] = []
  for (const index of [1, 2]) {
    const username = `trash-view-${suffix}-${index}`
    const created = await post<{ user: { id: string; username: string } }>(
      page,
      '/api/v1/admin/users',
      {
        username,
        displayName: username,
        role: 'user',
        password: 'Temporary-password-123',
      },
    )
    users.push(created.user)
    expect(
      (
        await page.request.put('/api/v1/grants', {
          headers: headers(),
          data: {
            resourceType: 'table',
            resourceId: tableId,
            principalType: 'user',
            principalId: created.user.id,
            level: 'read',
          },
        })
      ).status(),
    ).toBe(201)
  }

  const contexts: BrowserContext[] = []
  try {
    const first = await participant(browser, users[0]!.username)
    const second = await participant(browser, users[1]!.username)
    contexts.push(first.context, second.context)
    const firstDiagnostics = await observeBrowser(first.page)
    const secondDiagnostics = await observeBrowser(second.page)
    const firstSubscriptions = subscriptions(first.page)
    const secondSubscriptions = subscriptions(second.page)
    const firstPrivate = await post<{ view: TableView }>(
      first.page,
      `/api/v1/tables/${tableId}/views`,
      { name: 'First private', scope: 'personal', type: 'grid' },
    )
    const secondPrivate = await post<{ view: TableView }>(
      second.page,
      `/api/v1/tables/${tableId}/views`,
      { name: 'Second private', scope: 'personal', type: 'grid' },
    )
    const allViews = [
      shared.view,
      ownerPrivate.view,
      firstPrivate.view,
      secondPrivate.view,
    ]
    const actors = [
      { page, own: ownerPrivate.view, subscribed: ownerSubscriptions },
      {
        page: first.page,
        own: firstPrivate.view,
        subscribed: firstSubscriptions,
      },
      {
        page: second.page,
        own: secondPrivate.view,
        subscribed: secondSubscriptions,
      },
    ]
    for (const actor of actors) {
      await actor.page.goto(`/t/${tableId}/v/${actor.own.id}`)
      if ((await actor.page.locator('html').getAttribute('lang')) === 'zh-CN')
        await setLocale(actor.page, 'en')
      await expect.poll(() => actor.subscribed.has(tableId)).toBe(true)
      await expect(
        await sidebarView(actor.page, tableId, actor.own.name),
      ).toBeVisible()
      const before = (await (
        await actor.page.request.get(`/api/v1/tables/${tableId}/schema`)
      ).json()) as TableSchema
      expect(before.views.map((view) => view.id)).toContain(shared.view.id)
      expect(before.views.map((view) => view.id)).toContain(actor.own.id)
      for (const other of allViews.filter(
        (view) => view.scope === 'personal' && view.id !== actor.own.id,
      ))
        expect(before.views.map((view) => view.id)).not.toContain(other.id)
      allowHTTPFailure(
        actor.page,
        'GET',
        `/api/v1/tables/${tableId}/schema`,
        404,
      )
      allowHTTPFailure(actor.page, 'POST', '/api/v1/views/*/query', 404)
    }

    const tableNode = page.locator(
      `[data-nav-id="${tableId}"][data-kind="table"]`,
    )
    const actions = tableNode.locator(':scope > .node-actions')
    await actions.locator(':scope > summary').click()
    await actions
      .getByRole('button', { name: en.nav.delete, exact: true })
      .click()
    const confirmation = page.getByRole('dialog', { name: en.nav.dialogTitle })
    await confirmation
      .getByRole('button', { name: en.admin.confirm, exact: true })
      .click()
    await expect(confirmation).not.toBeVisible()

    for (const actor of actors) {
      await expect(
        actor.page.locator(`[data-nav-id="${tableId}"][data-kind="table"]`),
      ).toHaveCount(0)
      await expect(
        actor.page.getByRole('heading', { name: en.nav.removedTitle }),
      ).toBeVisible()
      expect(
        (
          await actor.page.request.get(`/api/v1/tables/${tableId}/schema`)
        ).status(),
      ).toBe(404)
      for (const view of allViews) {
        expect(
          (
            await actor.page.request.post(`/api/v1/views/${view.id}/query`, {
              headers: headers(),
              data: { offset: 0, limit: 100, includeCount: true },
            })
          ).status(),
        ).toBe(404)
        expect(
          (
            await actor.page.request.patch(`/api/v1/views/${view.id}`, {
              headers: { ...headers(), 'If-Match': String(view.version) },
              data: { name: 'Unavailable' },
            })
          ).status(),
        ).toBe(404)
        expect(
          (
            await actor.page.request.post(
              `/api/v1/views/${view.id}/duplicate`,
              {
                headers: headers(),
                data: { name: 'Unavailable copy', scope: 'personal' },
              },
            )
          ).status(),
        ).toBe(404)
        expect(
          (
            await actor.page.request.post(`/api/v1/views/${view.id}/move`, {
              headers: { ...headers(), 'If-Match': String(view.version) },
              data: { beforeId: null },
            })
          ).status(),
        ).toBe(404)
        expect(
          (
            await actor.page.request.delete(`/api/v1/views/${view.id}`, {
              headers: { ...headers(), 'If-Match': String(view.version) },
            })
          ).status(),
        ).toBe(404)
      }
      expect(
        (
          await actor.page.request.post(`/api/v1/tables/${tableId}/views`, {
            headers: headers(),
            data: { name: 'Unavailable', scope: 'personal' },
          })
        ).status(),
      ).toBe(404)
      for (const view of [shared.view, actor.own]) {
        await actor.page.goto(`/t/${tableId}/v/${view.id}`)
        await expect(
          actor.page.getByRole('heading', { name: en.nav.removedTitle }),
        ).toBeVisible()
      }
    }

    await openTrash(page)
    await restore(page, tableId, false)
    for (const actor of actors) {
      const restored = await actor.page.request.get(
        `/api/v1/tables/${tableId}/schema`,
      )
      expect(restored.status()).toBe(200)
      const restoredSchema = (await restored.json()) as TableSchema
      expect(restoredSchema.views.map((view) => view.id)).toContain(
        shared.view.id,
      )
      expect(restoredSchema.views.map((view) => view.id)).toContain(
        actor.own.id,
      )
      for (const other of allViews.filter(
        (view) => view.scope === 'personal' && view.id !== actor.own.id,
      ))
        expect(restoredSchema.views.map((view) => view.id)).not.toContain(
          other.id,
        )
      const query = await actor.page.request.post(
        `/api/v1/views/${actor.own.id}/query`,
        {
          headers: headers(),
          data: { offset: 0, limit: 100, includeCount: true },
        },
      )
      expect(query.status()).toBe(200)
      expect(((await query.json()) as { rows: DataRow[] }).rows).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            id: row.id,
            fields: { [fieldId]: 'Retained descendant data' },
          }),
        ]),
      )
      await actor.page.goto(`/t/${tableId}/v/${actor.own.id}`)
      await expect(
        await sidebarView(actor.page, tableId, actor.own.name),
      ).toBeVisible()
    }
    expect(firstDiagnostics.errors).toEqual([])
    expect(secondDiagnostics.errors).toEqual([])
  } finally {
    for (const context of contexts) await context.close()
  }
})

test('AT-203, AT-325 and AT-610 restore deleted resources and retained data', async ({
  page,
  isMobile,
}) => {
  test.setTimeout(150_000)
  await login(page)
  const suffix = Date.now()
  const workspace = await post<{ workspace: { id: string } }>(
    page,
    '/api/v1/workspaces',
    { name: `Trash workspace ${suffix}` },
  )
  const wid = workspace.workspace.id
  const book = await post<{ book: { id: string } }>(
    page,
    `/api/v1/workspaces/${wid}/books`,
    { name: `Trash book ${suffix}` },
  )
  const bid = book.book.id
  const schema = await post<TableSchema>(page, `/api/v1/books/${bid}/tables`, {
    name: `Trash table ${suffix}`,
  })
  const tid = schema.table.id
  const grid = schema.views[0]!
  const createdField = await post<{ field: TableField }>(
    page,
    `/api/v1/tables/${tid}/fields`,
    {
      name: 'Retained notes',
      uiType: 'text',
      isRequired: true,
      default: 'seed',
    },
  )
  const field = createdField.field
  const createdView = await post<{ view: TableView }>(
    page,
    `/api/v1/tables/${tid}/views`,
    { name: 'Restorable view', type: 'grid', scope: 'shared' },
  )
  const view = createdView.view
  const createdRows = await post<{ rows: DataRow[] }>(
    page,
    `/api/v1/tables/${tid}/rows`,
    { rows: [{ fields: { [field.id]: 'original value' } }] },
  )
  const row = createdRows.rows[0]!

  await page.goto(`/t/${tid}/v/${grid.id}`)
  const panel = await openFieldPanel(page)
  const fieldActions = panel.locator(`[data-field-id="${field.id}"] details`)
  await fieldActions.locator('summary').click()
  await fieldActions
    .getByRole('button', { name: en.field.panel.delete, exact: true })
    .click()
  const deleteField = page.getByRole('dialog', { name: en.field.panel.confirm })
  await deleteField
    .getByRole('button', { name: en.field.panel.confirm, exact: true })
    .click()
  await expect(deleteField).not.toBeVisible()
  await expect(panel.locator(`[data-field-id="${field.id}"]`)).toHaveCount(0)
  if (isMobile)
    await page
      .getByRole('button', { name: en.mobileRows.closeOptions, exact: true })
      .click()
  const hidden = await page.request.get(`/api/v1/tables/${tid}/rows/${row.id}`)
  expect(hidden.status()).toBe(200)
  expect(((await hidden.json()) as DataRow).fields).not.toHaveProperty(field.id)
  await post(page, `/api/v1/tables/${tid}/rows`, { rows: [{ fields: {} }] })

  await openTrash(page)
  const fieldCard = page.locator(`[data-trash-id="${field.id}"]`)
  await expect(fieldCard).toContainText('Retained notes')
  await expect(fieldCard).toContainText(`Trash workspace ${suffix}`)
  expect(await fieldCard.locator('time').count()).toBe(2)
  expectHTTPFailure(
    page,
    'POST',
    `/api/v1/trash/field/${field.id}/restore`,
    409,
  )
  await fieldCard.getByRole('button').click()
  await expect(page.getByRole('alert')).toContainText('1 empty rows')
  const optional = page.getByRole('button', {
    name: en.trash.restoreOptionalItem.replace('{name}', 'Retained notes'),
  })
  await expect(optional).toBeEnabled()
  if (isMobile) await optional.tap()
  else await optional.press('Enter')
  await expect(fieldCard).toHaveCount(0)
  const restoredRow = await page.request.get(
    `/api/v1/tables/${tid}/rows/${row.id}`,
  )
  expect(restoredRow.status()).toBe(200)
  expect(((await restoredRow.json()) as DataRow).fields[field.id]).toBe(
    'original value',
  )

  const nav = await navigation(page, isMobile)
  const tableNode = nav.locator(`[data-nav-id="${tid}"]`)
  await expect(tableNode).toBeVisible()
  const actions = tableNode.locator(':scope > .node-actions')
  await actions.locator('summary').click()
  await actions
    .getByRole('button', { name: en.nav.delete, exact: true })
    .click()
  const deleteTable = page.getByRole('dialog', { name: en.nav.dialogTitle })
  await deleteTable
    .getByRole('button', { name: en.admin.confirm, exact: true })
    .click()
  await expect(deleteTable).not.toBeVisible()
  await expect(tableNode).toHaveCount(0)
  if (isMobile && (await page.locator('.nav-drawer').isVisible()))
    await page.getByRole('button', { name: en.nav.close, exact: true }).click()
  expect(
    (await page.request.get(`/api/v1/tables/${tid}/schema`)).status(),
  ).toBe(404)
  await page.getByRole('button', { name: en.trash.refresh }).click()
  await restore(page, tid, isMobile)
  const tableRow = await page.request.get(
    `/api/v1/tables/${tid}/rows/${row.id}`,
  )
  expect(tableRow.status()).toBe(200)
  expect(((await tableRow.json()) as DataRow).fields[field.id]).toBe(
    'original value',
  )

  const deleteView = await page.request.delete(`/api/v1/views/${view.id}`, {
    headers: { ...headers(), 'If-Match': String(view.version) },
  })
  expect(deleteView.status()).toBe(200)
  await page.getByRole('button', { name: en.trash.refresh }).click()
  await restore(page, view.id, isMobile)

  for (const [kind, id] of [
    ['books', bid],
    ['workspaces', wid],
  ]) {
    const deleted = await page.request.delete(`/api/v1/${kind}/${id}`, {
      headers: headers(),
    })
    expect(deleted.status()).toBe(200)
    await page.getByRole('button', { name: en.trash.refresh }).click()
    await restore(page, id!, isMobile)
  }
  const finalSchema = await page.request.get(`/api/v1/tables/${tid}/schema`)
  expect(finalSchema.status()).toBe(200)
  expect(
    ((await finalSchema.json()) as TableSchema).views.map((item) => item.id),
  ).toContain(view.id)
  await setLocale(page, 'zh-CN')
  await expect(
    page.getByRole('heading', { name: zh.trash.title }),
  ).toBeVisible()
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true)
})
