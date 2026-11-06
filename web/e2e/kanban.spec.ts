import type { Locator, Page } from '@playwright/test'
import {
  expect,
  expectBrowserClean,
  observeBrowser,
  paceLogin,
  test,
} from './fixtures'
import { setLocale } from './app-shell'
import { openCreateView } from './table-controls'
import en from '../src/i18n/en.json' with { type: 'json' }
import type {
  AuthUser,
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

function column(page: Page, name: string): Locator {
  return page
    .locator('.kanban-column')
    .filter({ has: page.getByRole('heading', { name, exact: true }) })
}

async function english(page: Page) {
  if ((await page.locator('html').getAttribute('lang')) === 'zh-CN')
    await setLocale(page, 'en')
}

test('AT-901 persists card moves and updates another connected Kanban client', async ({
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
  const me = (await (
    await page.request.get('/api/v1/auth/me')
  ).json()) as AuthUser
  const suffix = `${Date.now()}-${isMobile ? 'mobile' : 'desktop'}`
  const workspace = await create<{ workspace: { id: string } }>(
    page,
    '/api/v1/workspaces',
    { name: `Kanban ${suffix}` },
  )
  const book = await create<{ book: { id: string } }>(
    page,
    `/api/v1/workspaces/${workspace.workspace.id}/books`,
    { name: 'Board book' },
  )
  const schema = await create<TableSchema>(
    page,
    `/api/v1/books/${book.book.id}/tables`,
    { name: 'Board tasks' },
  )
  const tableId = schema.table.id
  const titleId = schema.table.displayFieldId
  if (!titleId) throw new Error('Kanban table has no display field')
  const status = await create<{ field: TableField }>(
    page,
    `/api/v1/tables/${tableId}/fields`,
    {
      name: 'Status',
      uiType: 'single_select',
      options: {
        choices: [
          { id: 'todo', label: 'Todo', color: 'gray' },
          { id: 'done', label: 'Done', color: 'green' },
        ],
      },
    },
  )
  const assignee = await create<{ field: TableField }>(
    page,
    `/api/v1/tables/${tableId}/fields`,
    { name: 'Assignee', uiType: 'user' },
  )
  const notes = await create<{ field: TableField }>(
    page,
    `/api/v1/tables/${tableId}/fields`,
    { name: 'Notes', uiType: 'text' },
  )
  const inserted = await create<{ rows: DataRow[] }>(
    page,
    `/api/v1/tables/${tableId}/rows`,
    {
      rows: [
        {
          fields: {
            [titleId]: 'First task',
            [status.field.id]: 'Todo',
            [notes.field.id]: 'Visible detail',
          },
        },
        { fields: { [titleId]: 'Unassigned task' } },
      ],
    },
  )
  const row = inserted.rows[0]!
  await page.goto(`/t/${tableId}/v/${schema.views[0]!.id}`)
  await english(page)
  const createView = await openCreateView(page, tableId)
  await createView.getByLabel(en.nav.viewName, { exact: true }).fill('Board')
  await createView.getByLabel(en.nav.viewScopeLabel).selectOption('shared')
  await createView.getByLabel(en.form.viewType).selectOption('kanban')
  await createView.getByRole('button', { name: en.nav.createView }).click()
  await expect(page.locator('.kanban-settings')).toBeVisible()
  const builder = page.locator('.kanban-settings')
  await builder.locator('summary').click()
  await builder.getByLabel(en.kanban.groupField).selectOption(status.field.id)
  await builder
    .getByRole('checkbox', { name: 'Assignee', exact: true })
    .uncheck()
  await builder.getByRole('button', { name: en.kanban.save }).click()
  await expect(column(page, 'Todo')).toContainText('First task')
  await expect(column(page, 'Todo')).toContainText('Visible detail')
  await expect(column(page, en.kanban.empty)).toContainText('Unassigned task')
  await expect(column(page, 'Done')).toContainText(
    en.kanban.count.replace('{count}', '0'),
  )

  const boardURL = new URL(page.url()).pathname
  const peerContext = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL,
    locale: 'en-US',
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
    await peer.goto(boardURL)
    await english(peer)
    await expect(column(peer, 'Todo')).toContainText('First task')
    await expect
      .poll(() =>
        frames.some(
          (frame) => frame.t === 'subscribed' && frame.tableId === tableId,
        ),
      )
      .toBe(true)

    const card = column(page, 'Todo').locator(`[data-row-id="${row.id}"]`)
    const saved = page.waitForResponse(
      (response) =>
        response.request().method() === 'PATCH' &&
        new URL(response.url()).pathname ===
          `/api/v1/tables/${tableId}/rows/${row.id}`,
    )
    if (isMobile) {
      await card
        .getByRole('button', {
          name: en.kanban.moveCard.replace('{id}', row.id),
        })
        .click()
      await card
        .getByLabel(en.kanban.destination)
        .selectOption({ label: 'Done' })
      await card
        .getByRole('button', { name: en.kanban.confirmMove, exact: true })
        .click()
    } else {
      await card.dragTo(column(page, 'Done'))
    }
    expect((await saved).status()).toBe(200)
    await expect(column(page, 'Done')).toContainText('First task')
    await expect
      .poll(() =>
        frames.some(
          (frame) => frame.t === 'row.updated' && frame.tableId === tableId,
        ),
      )
      .toBe(true)
    await expect(column(peer, 'Done')).toContainText('First task')
    const persisted = await page.request.get(
      `/api/v1/tables/${tableId}/rows/${row.id}`,
    )
    expect(((await persisted.json()) as DataRow).fields[status.field.id]).toBe(
      'Done',
    )

    await builder
      .getByLabel(en.kanban.groupField)
      .selectOption(assignee.field.id)
    await builder.getByRole('button', { name: en.kanban.save }).click()
    await expect(column(page, en.kanban.empty)).toContainText('First task')
    await expect(column(peer, en.kanban.empty)).toContainText('First task')
    await page.getByLabel(en.kanban.searchUsers).fill('first-admin')
    await page
      .getByRole('button', { name: en.kanban.search, exact: true })
      .click()
    await page
      .getByRole('button', {
        name: en.kanban.addUser.replace('{name}', me.displayName),
      })
      .click()
    const unassigned = column(page, en.kanban.empty).locator(
      `[data-row-id="${row.id}"]`,
    )
    await unassigned
      .getByRole('button', { name: en.kanban.moveCard.replace('{id}', row.id) })
      .click()
    await unassigned
      .getByLabel(en.kanban.destination)
      .selectOption({ label: me.displayName })
    await unassigned
      .getByRole('button', { name: en.kanban.confirmMove, exact: true })
      .click()
    await expect(column(page, me.displayName)).toContainText('First task')
    await expect(column(peer, me.displayName)).toContainText('First task')
    const assigned = await page.request.get(
      `/api/v1/tables/${tableId}/rows/${row.id}`,
    )
    expect(((await assigned.json()) as DataRow).fields[assignee.field.id]).toBe(
      me.id,
    )
    await page.reload()
    await expect(column(page, me.displayName)).toContainText('First task')
    expectBrowserClean(peer, diagnostics)
  } finally {
    await peerContext.close()
  }
})
