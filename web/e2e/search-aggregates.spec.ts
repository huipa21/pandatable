import { expect, paceLogin, test } from './fixtures'
import { setLocale } from './app-shell'
import { openGridSearch } from './table-controls'
import type { Components } from '@revolist/revogrid'
import en from '../src/i18n/en.json' with { type: 'json' }
import type { TableSchema } from '../src/api/types'

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

async function signIn(page: import('@playwright/test').Page) {
  await paceLogin(page)
  await page.goto('/login')
  if ((await page.locator('html').getAttribute('lang')) === 'zh-CN')
    await setLocale(page, 'en')
  await page.getByLabel(en.auth.login, { exact: true }).fill('first-admin')
  await page
    .getByLabel(en.auth.password, { exact: true })
    .fill('Setup-password-123')
  await page.getByRole('button', { name: en.auth.signIn, exact: true }).click()
  await expect(page).toHaveURL(/\/(?:home|change-password)$/)
}

function aggregateValue(
  result: {
    aggregates: { fieldId: string; aggregate: string; value: unknown }[]
  },
  fieldId: string,
  aggregate: string,
) {
  const found = result.aggregates.find(
    (item) => item.fieldId === fieldId && item.aggregate === aggregate,
  )
  if (!found) throw new Error(`Missing ${aggregate} aggregate for ${fieldId}`)
  return found.value
}

test('AT-509/510 quick search and footer aggregates use visible filtered rows', async ({
  page,
  browser,
}) => {
  test.setTimeout(150_000)
  test.skip(
    (page.viewportSize()?.width ?? 0) < 768,
    'production grid is desktop-only',
  )
  await signIn(page)

  const stamp = Date.now()
  const create = async (path: string, data: unknown) => {
    const response = await page.request.post(path, { headers: headers(), data })
    expect(response.status()).toBe(201)
    return response.json()
  }
  const workspace = (await create('/api/v1/workspaces', {
    name: `Search ${stamp}`,
  })) as { workspace: { id: string } }
  const book = (await create(
    `/api/v1/workspaces/${workspace.workspace.id}/books`,
    { name: `Search book ${stamp}` },
  )) as { book: { id: string } }
  const createdTable = (await create(`/api/v1/books/${book.book.id}/tables`, {
    name: `Search table ${stamp}`,
  })) as TableSchema
  const tableId = createdTable.table.id

  const createField = async (name: string, uiType: string) => {
    const result = (await create(`/api/v1/tables/${tableId}/fields`, {
      name,
      uiType,
    })) as { field: TableSchema['fields'][number] }
    return result.field
  }
  const note = await createField('Note', 'text')
  const amount = await createField('Amount', 'integer')
  const rate = await createField('Rate', 'integer')
  const counter = await createField('Counter', 'text')
  const privateField = await createField('Private', 'text')
  const schema = (await (
    await page.request.get(`/api/v1/tables/${tableId}/schema`)
  ).json()) as TableSchema
  const displayId = schema.table.displayFieldId!
  const view = schema.views[0]!

  const values = [
    { label: 'Acme Alpha', note: 'one', amount: 10, rate: 2, counter: 'x' },
    {
      label: 'Beta',
      note: '项目 Beta',
      amount: 20,
      rate: 4,
      counter: 'y',
      private: 'acme hidden',
    },
    {
      label: 'Gamma',
      note: '项目 Gamma',
      amount: 10,
      rate: null,
      counter: 'z',
    },
    { label: 'Empty', note: null, amount: 10, rate: null, counter: 'n' },
  ]
  const rows = await page.request.post(`/api/v1/tables/${tableId}/rows`, {
    headers: headers(),
    data: {
      rows: values.map((value) => ({
        fields: {
          [displayId]: value.label,
          [note.id]: value.note,
          [amount.id]: value.amount,
          [rate.id]: value.rate,
          [counter.id]: value.counter,
          [privateField.id]: value.private ?? null,
        },
      })),
    },
  })
  expect(rows.status()).toBe(201)

  const filter = { field: amount.id, cmp: 'eq', value: 10 }
  const config = await page.request.patch(`/api/v1/views/${view.id}`, {
    headers: { ...headers(), 'If-Match': String(view.version) },
    data: {
      config: {
        fields: schema.fields.map((field) => ({
          id: field.id,
          visible: !field.isSystem && field.id !== privateField.id,
          width: field.id === displayId ? 180 : 260,
        })),
        frozen: 1,
        filter,
        aggregates: [
          { fieldId: amount.id, aggregate: 'sum' },
          { fieldId: rate.id, aggregate: 'avg' },
          { fieldId: note.id, aggregate: 'count_empty' },
          { fieldId: displayId, aggregate: 'unique' },
          { fieldId: counter.id, aggregate: 'count' },
        ],
      },
    },
  })
  expect(config.status()).toBe(200)
  await page.setViewportSize({ width: 1280, height: 900 })

  const waitForAggregates = (search?: string) =>
    page.waitForResponse((response) => {
      const request = response.request()
      if (
        request.method() !== 'POST' ||
        new URL(response.url()).pathname !==
          `/api/v1/views/${view.id}/aggregates`
      )
        return false
      const body = request.postDataJSON() as { search?: string }
      return body.search === search
    })
  const initialAggregates = waitForAggregates()
  await page.goto(`/t/${tableId}/v/${view.id}`)
  const initial = (await (await initialAggregates).json()) as {
    aggregates: { fieldId: string; aggregate: string; value: unknown }[]
  }
  expect(aggregateValue(initial, amount.id, 'sum')).toBe('30')
  expect(aggregateValue(initial, rate.id, 'avg')).toBe('2')
  expect(aggregateValue(initial, note.id, 'count_empty')).toBe(1)
  expect(aggregateValue(initial, displayId, 'unique')).toBe(3)
  expect(aggregateValue(initial, counter.id, 'count')).toBe(3)

  const grid = page.locator('revo-grid')
  const gridWorkspace = page.locator('.grid-workspace')
  const surface = page.locator('.grid-surface')
  const summaryBar = page.locator('.grid-summary-bar')
  const summaryCells = page.locator('.grid-summary-cell[data-field-id]')
  await expect(summaryCells).toHaveCount(5)
  await expect(
    page.locator(`.grid-summary-cell[data-field-id="${privateField.id}"]`),
  ).toHaveCount(0)
  await expect(
    page.locator(`.grid-summary-cell[data-field-id="${amount.id}"] output`),
  ).toContainText('30')
  await expect(grid).toHaveAttribute('aria-rowcount', '3')
  const lastRow = grid.locator('revogr-data .rgRow[data-rgrow="2"]').first()
  const appendRow = page.getByRole('button', {
    name: en.grid.addRow,
    exact: true,
  })
  await expect(lastRow).toBeVisible()
  await expect(appendRow).toBeVisible()
  await expect
    .poll(async () => {
      const [workspaceBox, surfaceBox, barBox, rowBox, appendBox] =
        await Promise.all([
          gridWorkspace.boundingBox(),
          surface.boundingBox(),
          summaryBar.boundingBox(),
          lastRow.boundingBox(),
          appendRow.boundingBox(),
        ])
      if (!workspaceBox || !surfaceBox || !barBox || !rowBox || !appendBox)
        return Infinity
      return Math.max(
        Math.abs(barBox.y + barBox.height - (surfaceBox.y + surfaceBox.height)),
        Math.abs(
          barBox.y + barBox.height - (workspaceBox.y + workspaceBox.height),
        ),
        Math.abs(appendBox.y - (rowBox.y + rowBox.height)),
      )
    })
    .toBeLessThanOrEqual(2)
  const [barBox, appendBox] = await Promise.all([
    summaryBar.boundingBox(),
    appendRow.boundingBox(),
  ])
  if (!barBox || !appendBox) throw new Error('Footer geometry is unavailable')
  expect(barBox.y - (appendBox.y + appendBox.height)).toBeGreaterThan(20)

  const expectAligned = async (fieldName: string, fieldId: string) => {
    const headerBox = await grid
      .getByRole('columnheader', { name: fieldName, exact: true })
      .boundingBox()
    const summaryBox = await page
      .locator(`.grid-summary-cell[data-field-id="${fieldId}"]`)
      .boundingBox()
    if (!headerBox || !summaryBox)
      throw new Error(`Missing ${fieldName} column geometry`)
    const footerTransform = await page
      .locator(`.grid-summary-cell[data-field-id="${fieldId}"]`)
      .evaluate((element) => getComputedStyle(element.parentElement!).transform)
    const horizontalScroll = await grid
      .locator('revogr-scroll-virtual.horizontal')
      .evaluate((element) => ({
        scrollLeft: (element as HTMLElement).scrollLeft,
        scrollWidth: (element as HTMLElement).scrollWidth,
        clientWidth: (element as HTMLElement).clientWidth,
      }))
    expect(
      Math.abs(summaryBox.x - headerBox.x),
      `${fieldName} summary x=${summaryBox.x}, header x=${headerBox.x}, footer transform=${footerTransform}, horizontal=${JSON.stringify(horizontalScroll)}`,
    ).toBeLessThanOrEqual(2)
    expect(
      Math.abs(summaryBox.width - headerBox.width),
      `${fieldName} summary width=${summaryBox.width}, header width=${headerBox.width}`,
    ).toBeLessThanOrEqual(2)
  }
  await expectAligned('Amount', amount.id)
  const amountHeader = grid.getByRole('columnheader', {
    name: 'Amount',
    exact: true,
  })
  const amountBeforeResize = await amountHeader.boundingBox()
  const resizeHandle = await amountHeader.locator('.resizable-r').boundingBox()
  if (!amountBeforeResize || !resizeHandle)
    throw new Error('Amount column resize handle is unavailable')
  const resizedConfigSaved = page.waitForResponse((response) => {
    const request = response.request()
    return (
      request.method() === 'PATCH' &&
      new URL(response.url()).pathname === `/api/v1/views/${view.id}`
    )
  })
  await page.mouse.move(
    resizeHandle.x + resizeHandle.width / 2,
    resizeHandle.y + resizeHandle.height / 2,
  )
  await page.mouse.down()
  await page.mouse.move(
    resizeHandle.x + 36,
    resizeHandle.y + resizeHandle.height / 2,
    { steps: 6 },
  )
  await page.mouse.up()
  await expect
    .poll(async () => (await amountHeader.boundingBox())?.width ?? 0)
    .toBeGreaterThan(amountBeforeResize.width + 20)
  await expectAligned('Amount', amount.id)
  await resizedConfigSaved
  const frozenSummary = page.locator(
    `.grid-summary-cell[data-field-id="${displayId}"]`,
  )
  const frozenBefore = await frozenSummary.boundingBox()
  await grid.evaluate(async (element, fieldId) => {
    await (element as HTMLElement & Components.RevoGrid).scrollToColumnProp(
      fieldId,
    )
  }, counter.id)
  await expect
    .poll(() =>
      grid
        .locator('revogr-scroll-virtual.horizontal')
        .evaluate((element) => element.scrollLeft > 0),
    )
    .toBe(true)
  await expectAligned('Counter', counter.id)
  const frozenAfter = await frozenSummary.boundingBox()
  if (!frozenBefore || !frozenAfter)
    throw new Error('Frozen summary geometry is unavailable')
  expect(Math.abs(frozenAfter.x - frozenBefore.x)).toBeLessThanOrEqual(2)

  const acmeQuery = page.waitForResponse((response) => {
    const request = response.request()
    if (
      request.method() !== 'POST' ||
      new URL(response.url()).pathname !== `/api/v1/views/${view.id}/query`
    )
      return false
    return (request.postDataJSON() as { search?: string }).search === 'acme'
  })
  const acmeAggregates = waitForAggregates('acme')
  await openGridSearch(page)
  await page.getByRole('searchbox', { name: en.grid.quickSearch }).fill('acme')
  const acmeResponse = await acmeQuery
  const acmeRows = (await acmeResponse.json()) as {
    rows: { fields: Record<string, unknown> }[]
  }
  expect(acmeRows.rows.map((row) => row.fields[displayId])).toEqual([
    'Acme Alpha',
  ])
  const acmeFooter = (await (await acmeAggregates).json()) as {
    aggregates: { fieldId: string; aggregate: string; value: unknown }[]
  }
  expect(aggregateValue(acmeFooter, amount.id, 'sum')).toBe('10')
  expect(aggregateValue(acmeFooter, rate.id, 'avg')).toBe('2')
  expect(aggregateValue(acmeFooter, note.id, 'count_empty')).toBe(0)
  expect(aggregateValue(acmeFooter, displayId, 'unique')).toBe(1)
  expect(aggregateValue(acmeFooter, counter.id, 'count')).toBe(1)

  const chineseQuery = page.waitForResponse((response) => {
    const request = response.request()
    return (
      request.method() === 'POST' &&
      new URL(response.url()).pathname === `/api/v1/views/${view.id}/query` &&
      (request.postDataJSON() as { search?: string }).search === '项目'
    )
  })
  await page.getByRole('searchbox', { name: en.grid.quickSearch }).fill('项目')
  const chineseRows = (await (await chineseQuery).json()) as {
    rows: { fields: Record<string, unknown> }[]
  }
  expect(chineseRows.rows.map((row) => row.fields[displayId])).toEqual([
    'Gamma',
  ])

  const amountAggregate = page.getByRole('button', {
    name: en.grid.aggregateForField.replace('{field}', 'Amount'),
    exact: true,
  })
  await amountAggregate.focus()
  await expect
    .poll(async () => {
      const [gridBox, pinnedBox, amountBox] = await Promise.all([
        grid.boundingBox(),
        frozenSummary.boundingBox(),
        amountAggregate.boundingBox(),
      ])
      if (!gridBox || !pinnedBox || !amountBox) return Infinity
      return Math.max(
        0,
        pinnedBox.x + pinnedBox.width - amountBox.x,
        amountBox.x + amountBox.width - (gridBox.x + gridBox.width),
      )
    })
    .toBeLessThanOrEqual(2)
  await expectAligned('Amount', amount.id)
  const amountMenu = page.getByRole('listbox', {
    name: en.grid.aggregateForField.replace('{field}', 'Amount'),
  })
  const aggregateConfigSaved = page.waitForResponse((response) => {
    const request = response.request()
    return (
      request.method() === 'PATCH' &&
      new URL(response.url()).pathname === `/api/v1/views/${view.id}`
    )
  })
  const changedAggregate = waitForAggregates('项目')
  await amountAggregate.click()
  await amountMenu
    .getByRole('option', { name: en.grid.aggregateLabels.avg, exact: true })
    .click()
  await changedAggregate
  await aggregateConfigSaved
  await expect(
    page.locator(`.grid-summary-cell[data-field-id="${amount.id}"] output`),
  ).toContainText('Average')

  const clearedConfigSaved = page.waitForResponse((response) => {
    const request = response.request()
    return (
      request.method() === 'PATCH' &&
      new URL(response.url()).pathname === `/api/v1/views/${view.id}`
    )
  })
  const clearedAggregates = waitForAggregates('项目')
  await amountAggregate.click()
  await amountMenu
    .getByRole('option', { name: en.grid.aggregateLabels.none, exact: true })
    .click()
  const clearedConfig = (await clearedConfigSaved).request().postDataJSON() as {
    config: TableSchema['views'][number]['config']
  }
  await clearedAggregates
  expect(clearedConfig.config.aggregates ?? []).not.toContainEqual(
    expect.objectContaining({ fieldId: amount.id }),
  )
  await expect(
    page.locator(`.grid-summary-cell[data-field-id="${amount.id}"] output`),
  ).toHaveCount(0)
  const hint = page.locator(
    `.grid-summary-cell[data-field-id="${amount.id}"] .grid-summary-hint`,
  )
  await page.mouse.move(0, 0)
  await grid.focus()
  await expect
    .poll(() => hint.evaluate((element) => getComputedStyle(element).opacity))
    .toBe('0')
  await amountAggregate.hover()
  await expect
    .poll(() => hint.evaluate((element) => getComputedStyle(element).opacity))
    .toBe('1')
  await page.mouse.move(0, 0)
  await amountAggregate.focus()
  await expect
    .poll(() => hint.evaluate((element) => getComputedStyle(element).opacity))
    .toBe('1')
  await amountAggregate.press('Enter')
  await expect(amountMenu).toBeFocused()
  await amountMenu.press('ArrowDown')
  await expect(amountMenu).toHaveAttribute(
    'aria-activedescendant',
    (await amountMenu
      .getByRole('option', { name: en.grid.aggregateLabels.count, exact: true })
      .getAttribute('id')) as string,
  )
  await amountMenu.press('Escape')
  await expect(amountAggregate).toBeFocused()
  await amountAggregate.press('Enter')
  await amountMenu.press('Tab')
  await expect(amountMenu).toHaveCount(0)
  await expect(
    page.getByRole('button', {
      name: en.grid.aggregateForField.replace('{field}', 'Rate'),
      exact: true,
    }),
  ).toBeFocused()

  const touchContext = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL,
    storageState: await page.context().storageState(),
    hasTouch: true,
    viewport: { width: 1280, height: 900 },
  })
  try {
    const touchPage = await touchContext.newPage()
    await touchPage.goto(`/t/${tableId}/v/${view.id}`)
    expect(
      await touchPage.evaluate(() => matchMedia('(hover: none)').matches),
    ).toBe(true)
    const touchHint = touchPage.locator(
      `.grid-summary-cell[data-field-id="${amount.id}"] .grid-summary-hint`,
    )
    await expect
      .poll(() =>
        touchHint.evaluate((element) => getComputedStyle(element).opacity),
      )
      .toBe('1')
    await touchPage
      .getByRole('button', {
        name: en.grid.aggregateForField.replace('{field}', 'Amount'),
        exact: true,
      })
      .tap()
    await expect(
      touchPage.getByRole('listbox', {
        name: en.grid.aggregateForField.replace('{field}', 'Amount'),
      }),
    ).toBeVisible()
  } finally {
    await touchContext.close()
  }

  const updateView = async (
    update: (config: TableSchema['views'][number]['config']) => void,
  ) => {
    const currentSchema = (await (
      await page.request.get(`/api/v1/tables/${tableId}/schema`)
    ).json()) as TableSchema
    const currentView = currentSchema.views.find(
      (candidate) => candidate.id === view.id,
    )
    if (!currentView) throw new Error('Footer test view is missing')
    const nextConfig = structuredClone(currentView.config)
    update(nextConfig)
    const response = await page.request.patch(`/api/v1/views/${view.id}`, {
      headers: { ...headers(), 'If-Match': String(currentView.version) },
      data: { config: nextConfig },
    })
    expect(response.status()).toBe(200)
  }

  await updateView((config) => {
    const counterIndex = config.fields.findIndex(
      (field) => field.id === counter.id,
    )
    const amountIndex = config.fields.findIndex(
      (field) => field.id === amount.id,
    )
    const [counterColumn] = config.fields.splice(counterIndex, 1)
    if (!counterColumn || amountIndex < 0)
      throw new Error('Footer test columns are missing')
    config.fields.splice(amountIndex, 0, counterColumn)
  })
  await page.reload()
  await expect(summaryCells).toHaveCount(5)
  expect(
    await summaryCells.evaluateAll((cells) =>
      cells.map((cell) => cell.getAttribute('data-field-id')),
    ),
  ).toEqual([displayId, note.id, counter.id, amount.id, rate.id])
  await grid.evaluate(async (element, fieldId) => {
    await (element as HTMLElement & Components.RevoGrid).scrollToColumnProp(
      fieldId,
    )
  }, counter.id)
  await expectAligned('Counter', counter.id)

  await updateView((config) => {
    config.groupBy = [counter.id]
  })
  await page.reload()
  await expect(summaryCells).toHaveCount(5)
  await expect(page.locator('.grid-add-row')).toBeVisible()

  await updateView((config) => {
    config.filter = { field: amount.id, cmp: 'eq', value: 999 }
  })
  await page.reload()
  await expect(summaryCells).toHaveCount(5)
  await expect(page.locator('.grid-add-row')).toBeVisible()

  const moreRows = await page.request.post(`/api/v1/tables/${tableId}/rows`, {
    headers: headers(),
    data: {
      rows: Array.from({ length: 40 }, (_, index) => ({
        fields: { [displayId]: `Extra ${index}`, [amount.id]: 10 },
      })),
    },
  })
  expect(moreRows.status()).toBe(201)
  await updateView((config) => {
    config.filter = null
    config.groupBy = []
  })
  await page.reload()
  await expect(grid).toHaveAttribute('aria-rowcount', '44')
  await grid.evaluate(async (element) => {
    await (element as HTMLElement & Components.RevoGrid).scrollToRow(35)
  })
  await expect
    .poll(() =>
      grid
        .locator('revogr-viewport-scroll.rgCol .vertical-inner')
        .evaluate((element) => element.scrollTop),
    )
    .toBeGreaterThan(0)
  await grid.evaluate(async (element, fieldId) => {
    await (element as HTMLElement & Components.RevoGrid).scrollToColumnProp(
      fieldId,
    )
  }, counter.id)
  await expect
    .poll(() =>
      grid
        .locator('revogr-scroll-virtual.horizontal')
        .evaluate((element) => element.scrollLeft),
    )
    .toBeGreaterThan(0)
  await expectAligned('Counter', counter.id)
  await expect
    .poll(async () => {
      const [currentSurface, currentBar] = await Promise.all([
        surface.boundingBox(),
        summaryBar.boundingBox(),
      ])
      if (!currentSurface || !currentBar) return Infinity
      return Math.abs(
        currentBar.y +
          currentBar.height -
          (currentSurface.y + currentSurface.height),
      )
    })
    .toBeLessThanOrEqual(2)
})
