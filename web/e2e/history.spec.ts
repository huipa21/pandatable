import type { Components } from '@revolist/revogrid'
import { test, expect, paceLogin } from './fixtures'
import type { DataRow, HistoryListing, TableSchema } from '../src/api/types'
import { setLocale } from './app-shell'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

test('AT-607: edits, paste and links appear in the localized row timeline', async ({
  page,
  isMobile,
}) => {
  test.setTimeout(180_000)
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
  const workspace = await page.request.post('/api/v1/workspaces', {
    headers: h,
    data: { name: `History ${isMobile ? 'mobile' : 'desktop'}` },
  })
  expect(workspace.status()).toBe(201)
  const workspaceId = (await workspace.json()).workspace.id as string
  const book = await page.request.post(
    `/api/v1/workspaces/${workspaceId}/books`,
    { headers: h, data: { name: 'History book' } },
  )
  expect(book.status()).toBe(201)
  const bookId = (await book.json()).book.id as string
  const createTable = async (name: string) => {
    const response = await page.request.post(`/api/v1/books/${bookId}/tables`, {
      headers: h,
      data: { name },
    })
    expect(response.status()).toBe(201)
    return (await response.json()) as TableSchema
  }
  const tasks = await createTable('Tasks')
  const projects = await createTable('Projects')
  const relationResponse = await page.request.post(
    `/api/v1/tables/${tasks.table.id}/relations`,
    {
      headers: h,
      data: {
        name: 'Projects',
        reverseName: 'Tasks',
        targetTableId: projects.table.id,
        kind: 'many_many',
      },
    },
  )
  expect(relationResponse.status()).toBe(201)
  const relation = (await relationResponse.json()) as {
    schema: TableSchema
    relation: TableSchema['relations'][number]
  }
  const schema = relation.schema
  const taskName = schema.table.displayFieldId
  const projectName = projects.table.displayFieldId
  if (!taskName || !projectName) throw new Error('Missing display fields')
  const createRows = async (
    tableId: string,
    fieldId: string,
    labels: string[],
  ) => {
    const response = await page.request.post(`/api/v1/tables/${tableId}/rows`, {
      headers: h,
      data: { rows: labels.map((label) => ({ fields: { [fieldId]: label } })) },
    })
    expect(response.status()).toBe(201)
    return ((await response.json()) as { rows: DataRow[] }).rows
  }
  const tasksRows = await createRows(tasks.table.id, taskName, [
    'Original A',
    'Original B',
  ])
  const project = (
    await createRows(projects.table.id, projectName, ['Apollo'])
  )[0]!
  const first = tasksRows[0]!
  const second = tasksRows[1]!
  const base = `/t/${tasks.table.id}/v/${schema.views[0]!.id}`
  const rowPath = `/api/v1/tables/${tasks.table.id}/rows/${first.id}`
  await page.goto(`${base}/r/${first.id}`)
  const dialog = page.locator('dialog.row-expand')
  await expect(dialog).toBeVisible()
  const nameSection = dialog.locator(`[data-field-id="${taskName}"]`)
  await nameSection.locator('.field-editor input').fill('Edited A')
  await dialog
    .getByRole('button', { name: en.gridPolish.gp05.save, exact: true })
    .click()
  await expect(
    dialog.getByText(en.rowExpand.saved, { exact: true }),
  ).toBeVisible()
  await expect(nameSection.locator('.field-editor input')).toHaveValue(
    'Edited A',
  )
  await dialog.getByRole('button', { name: en.rowExpand.close }).click()

  if (isMobile) {
    const response = await page.request.patch(
      `/api/v1/tables/${tasks.table.id}/rows`,
      {
        headers: h,
        data: {
          rows: [
            { id: first.id, fields: { [taskName]: 'Pasted A' } },
            { id: second.id, fields: { [taskName]: 'Pasted B' } },
          ],
        },
      },
    )
    expect(response.status()).toBe(200)
  } else {
    await page.goto(base)
    const grid = page.locator('revo-grid')
    await expect(grid).toBeVisible()
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
    // Rows and shell navigation load asynchronously; refocus until it sticks.
    await expect(async () => {
      const focused = await grid.evaluate(async (element, fieldId) => {
        const grid = element as HTMLElement & Components.RevoGrid
        await grid.scrollToRow(0)
        const column = (await grid.getColumns()).find(
          (item) => item.prop === fieldId,
        )!
        const type = column.pin ?? 'rgCol'
        const store = await grid.getColumnStore(type)
        const source = store.get('source')
        const x = store
          .get('items')
          .findIndex((index) => source[index]?.prop === fieldId)
        if (x < 0) throw new Error('History paste column is not visible')
        await grid.setCellsFocus({ x, y: 0 }, { x, y: 0 }, type)
        grid.focus()
        return (await grid.getFocused())?.model?.__rowId
      }, taskName)
      expect(focused).toBe(first.id)
    }).toPass({ timeout: 10_000 })
    await page.evaluate(() =>
      navigator.clipboard.writeText('Pasted A\nPasted B'),
    )
    await page.keyboard.press('Control+v')
    await expect
      .poll(async () => {
        const one = await page.request.get(rowPath)
        const two = await page.request.get(
          `/api/v1/tables/${tasks.table.id}/rows/${second.id}`,
        )
        return [
          ((await one.json()) as DataRow).fields[taskName],
          ((await two.json()) as DataRow).fields[taskName],
        ]
      })
      .toEqual(['Pasted A', 'Pasted B'])
  }

  const linkPath = `${rowPath}/links/${relation.relation.parentFieldId}`
  for (const data of [{ add: [project.id] }, { remove: [project.id] }]) {
    const response = await page.request.post(linkPath, { headers: h, data })
    expect(response.status()).toBe(200)
  }
  const historyResponse = await page.request.get(`${rowPath}/history`)
  expect(historyResponse.status()).toBe(200)
  const history = (await historyResponse.json()) as HistoryListing
  expect(history.items.map((item) => item.action)).toEqual([
    'unlink',
    'link',
    'update',
    'update',
    'insert',
  ])
  const reverseResponse = await page.request.get(
    `/api/v1/tables/${projects.table.id}/rows/${project.id}/history`,
  )
  expect(reverseResponse.status()).toBe(200)
  const reverse = (await reverseResponse.json()) as HistoryListing
  expect(reverse.items.slice(0, 2).map((item) => item.action)).toEqual([
    'unlink',
    'link',
  ])
  expect(reverse.items[0]!.changes[relation.relation.childFieldId]!.o).toEqual([
    { id: first.id, label: 'Pasted A' },
  ])

  await page.goto(`${base}/r/${first.id}`)
  const activity = dialog.locator('.row-activity')
  await expect(activity).toBeVisible()
  await activity
    .getByRole('tab', { name: en.history.title, exact: true })
    .click()
  await expect(nameSection.locator('.field-editor input')).toHaveValue(
    'Pasted A',
  )
  const timeline = activity.locator('.history')
  await expect(timeline).toContainText(history.items[0]!.authorName)
  await expect(timeline).toContainText('Original A → Edited A')
  await expect(timeline).toContainText('Edited A → Pasted A')
  await expect(timeline).toContainText(`Apollo (row ${project.id})`)
  await expect(timeline).toContainText(en.history.actions.link)
  await expect(timeline).toContainText(en.history.actions.unlink)
  await dialog.getByRole('button', { name: en.rowExpand.close }).click()
  await setLocale(page, 'zh-CN')
  await page.goto(`${base}/r/${first.id}`)
  await activity
    .getByRole('tab', { name: zh.history.title, exact: true })
    .click()
  await expect(timeline).toContainText(zh.history.actions.link)
  await expect(timeline).toContainText(`Apollo（行 ${project.id}）`)
})
