import type { Components } from '@revolist/revogrid'
import type { FilterNode, RowQueryResult, TableSchema } from '../src/api/types'
import { setLocale, setTheme } from './app-shell'
import { expect, paceLogin, test } from './fixtures'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }

test.use({ actionTimeout: 10_000 })

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

test('new table uses a compact, working field and filter toolbar', async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, 'The mobile card list retains its separate controls.')
  test.setTimeout(150_000)
  await page.setViewportSize({ width: 1440, height: 900 })
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

  const create = async (path: string, data: unknown) => {
    const response = await page.request.post(path, { headers: headers(), data })
    expect(response.status()).toBe(201)
    return response.json()
  }
  const stamp = Date.now()
  const workspace = (await create('/api/v1/workspaces', {
    name: `Table UI ${stamp}`,
  })) as { workspace: { id: string } }
  const book = (await create(
    `/api/v1/workspaces/${workspace.workspace.id}/books`,
    { name: `Table UI book ${stamp}` },
  )) as { book: { id: string } }
  const initial = (await create(`/api/v1/books/${book.book.id}/tables`, {
    name: `Table UI table ${stamp}`,
  })) as TableSchema
  const tableId = initial.table.id
  const viewId = initial.views[0]!.id
  const displayId = initial.table.displayFieldId!
  const schemaOf = async () => {
    const response = await page.request.get(`/api/v1/tables/${tableId}/schema`)
    expect(response.status()).toBe(200)
    return (await response.json()) as TableSchema
  }
  await page.goto(`/t/${tableId}/v/${viewId}`)

  const toolbar = page.locator('.grid-toolbar')
  await expect(toolbar).toBeVisible()
  for (const name of [
    en.grid.fields,
    en.grid.filter,
    en.grid.group,
    en.grid.sort,
  ])
    await expect(toolbar.getByText(name, { exact: true })).toBeVisible()
  await expect(
    toolbar.getByText(en.grid.highlightMenuLabel, { exact: true }),
  ).toBeVisible()
  await expect(
    page.getByRole('tab', { name: /^(data|details)$/i }),
  ).toHaveCount(0)
  await expect(page.locator('.table-management')).not.toHaveAttribute(
    'open',
    '',
  )
  await test.info().attach('new-table', {
    body: await page.screenshot(),
    contentType: 'image/png',
  })

  const grid = page.locator('revo-grid')
  const addField = page.locator('.grid-add-field')
  await expect(addField).toHaveAccessibleName(en.field.panel.add)
  await addField.click()
  const editor = page.getByRole('dialog', {
    name: en.field.panel.createTitle,
    exact: true,
  })
  await expect(editor).toBeVisible()
  await editor.getByLabel(en.field.panel.name, { exact: true }).fill('Notes')
  await editor.locator('[data-kind="long_text"]').click()
  await editor.getByText(en.field.panel.setDefault, { exact: true }).click()
  await editor
    .getByLabel(en.field.panel.defaultValue, { exact: true })
    .fill('New note')
  await editor.getByText(en.field.panel.addDescription, { exact: true }).click()
  await editor
    .getByLabel(en.field.panel.description, { exact: true })
    .fill('A description')
  const createdField = page.waitForResponse(
    (response) =>
      response.request().method() === 'POST' &&
      new URL(response.url()).pathname === `/api/v1/tables/${tableId}/fields`,
  )
  await editor.getByRole('button', { name: en.field.panel.create }).click()
  expect((await createdField).status()).toBe(201)
  await expect(editor).not.toBeVisible()
  const schema = await schemaOf()
  const notes = schema.fields.find((field) => field.name === 'Notes')!
  expect(notes).toMatchObject({
    uiType: 'long_text',
    default: 'New note',
    description: 'A description',
  })

  const panel = page.getByRole('region', { name: en.field.panel.title })
  await expect(panel).toBeVisible()
  await panel.getByLabel(en.field.panel.search).fill('Notes')
  await expect(panel.locator('.field-name')).toHaveText(['Notes'])
  await panel.getByLabel(en.field.panel.search).fill('')
  const showNotes = panel.getByLabel(
    en.field.panel.showField.replace('{name}', 'Notes'),
  )
  await showNotes.uncheck()
  await expect
    .poll(
      async () =>
        (await schemaOf()).views[0]!.config.fields.find(
          (field) => field.id === notes.id,
        )?.visible,
    )
    .toBe(false)
  await showNotes.check()
  await expect
    .poll(
      async () =>
        (await schemaOf()).views[0]!.config.fields.find(
          (field) => field.id === notes.id,
        )?.visible,
    )
    .toBe(true)
  await panel.getByRole('button', { name: en.field.panel.systemFields }).click()
  await expect(panel.locator('[data-system="true"]').first()).toBeVisible()
  await page.locator('.grid-fields-trigger').click()

  await page.setViewportSize({ width: 820, height: 900 })
  await grid.evaluate(async (element, id) => {
    await (element as HTMLElement & Components.RevoGrid).scrollToColumnProp(id)
  }, notes.id)
  const bounds = await addField.evaluate((button) => {
    const surface = button.parentElement!
    const grid = surface.querySelector('revo-grid')!
    const action = button.getBoundingClientRect()
    return {
      actionLeft: action.left,
      actionRight: action.right,
      gridRight: grid.getBoundingClientRect().right,
      surfaceRight: surface.getBoundingClientRect().right,
    }
  })
  expect(bounds.actionLeft).toBeGreaterThanOrEqual(bounds.gridRight - 1)
  expect(bounds.actionRight).toBeLessThanOrEqual(bounds.surfaceRight + 1)
  await addField.click()
  await expect(editor).toBeVisible()
  await editor.getByRole('button', { name: en.field.panel.cancel }).click()
  await page.locator('.grid-fields-trigger').click()
  await page.setViewportSize({ width: 1440, height: 900 })

  await page.getByRole('button', { name: en.grid.addRow, exact: true }).click()
  await expect
    .poll(async () => {
      const response = await page.request.post(
        `/api/v1/views/${viewId}/query`,
        {
          headers: headers(),
          data: { offset: 0, limit: 100, includeCount: true },
        },
      )
      expect(response.status()).toBe(200)
      return ((await response.json()) as RowQueryResult).rows.some(
        (row) => row.fields[notes.id] === 'New note',
      )
    })
    .toBe(true)
  await create(`/api/v1/tables/${tableId}/rows`, {
    rows: [
      { fields: { [displayId]: 'Alpha' } },
      { fields: { [displayId]: 'Beta' } },
    ],
  })
  await page.reload()
  await page.getByRole('button', { name: en.grid.filter, exact: true }).click()
  const filters = page.locator('.grid-filter-popover')
  await filters
    .getByRole('button', { name: en.filter.addCondition, exact: true })
    .click()
  const condition = filters.locator('.filter-item').first()
  await condition.locator('select').nth(0).selectOption(displayId)
  await condition.locator('select').nth(1).selectOption('contains')
  await condition.getByRole('textbox').fill('Alpha')
  const filteredQuery = page.waitForResponse((response) => {
    const request = response.request()
    return (
      request.method() === 'POST' &&
      new URL(response.url()).pathname === `/api/v1/views/${viewId}/query` &&
      Boolean((request.postDataJSON() as { filter?: FilterNode }).filter)
    )
  })
  await filters.getByRole('button', { name: en.filter.apply }).click()
  const filtered = await filteredQuery
  expect(filtered.status()).toBe(200)
  expect(
    ((await filtered.json()) as RowQueryResult).rows.map(
      (row) => row.fields[displayId],
    ),
  ).toEqual(['Alpha'])
  await expect(page.locator('.grid-filter-trigger .grid-count')).toHaveText('1')
  await test.info().attach('filter-menu', {
    body: await page.screenshot(),
    contentType: 'image/png',
  })

  await page.locator('.grid-groups summary').click()
  const grouping = page.locator('.grid-groups')
  await grouping.getByLabel(en.grid.addGroupField).selectOption(displayId)
  const groups = page.waitForResponse(
    (response) =>
      response.request().method() === 'POST' &&
      new URL(response.url()).pathname === `/api/v1/views/${viewId}/groups`,
  )
  await grouping.getByRole('button', { name: en.grid.addGroup }).click()
  expect((await groups).status()).toBe(200)
  await expect
    .poll(async () => (await schemaOf()).views[0]!.config.groupBy)
    .toEqual([displayId])
  await grouping
    .getByRole('button', {
      name: en.grid.removeGroupField.replace(
        '{name}',
        schema.fields.find((field) => field.id === displayId)!.name,
      ),
    })
    .click()
  await expect
    .poll(async () => (await schemaOf()).views[0]!.config.groupBy ?? [])
    .toEqual([])

  await page.locator('.grid-row-height summary').click()
  await page
    .locator('.grid-row-height')
    .getByRole('button', { name: en.grid.rowHeights.medium })
    .click()
  await expect
    .poll(() =>
      grid.evaluate(
        (element) => (element as HTMLElement & Components.RevoGrid).rowSize,
      ),
    )
    .toBe(48)
  await expect
    .poll(async () => (await schemaOf()).views[0]!.config.rowHeight)
    .toBe('medium')

  try {
    await setTheme(page, 'dark')
    await setLocale(page, 'zh-CN')
    await expect(
      page.getByRole('button', { name: zh.grid.fields, exact: true }),
    ).toBeVisible()
    await expect
      .poll(() =>
        page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
      )
      .toBe(true)
  } finally {
    await setLocale(page, 'en')
    await setTheme(page, 'light')
  }
})
