import { expect, paceLogin, test } from './fixtures'
import { setLocale, setTheme } from './app-shell'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }
import type { HighlightRule, TableSchema } from '../src/api/types'

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
  await expect(page).toHaveURL(/\/(?:home|w\/[^/]+|change-password)$/)
}

function dateOffset(date: string, days: number) {
  const [year, month, day] = date.split('-').map(Number)
  return new Date(Date.UTC(year!, month! - 1, day! + days))
    .toISOString()
    .slice(0, 10)
}

function contrastRatio(foreground: string, background: string) {
  const luminance = (color: string) => {
    const channels = color.match(/\d+/g)?.slice(0, 3).map(Number)
    if (!channels || channels.length !== 3)
      throw new Error(`Unexpected color: ${color}`)
    const values = channels.map((channel) => {
      const normalized = channel / 255
      return normalized <= 0.04045
        ? normalized / 12.92
        : ((normalized + 0.055) / 1.055) ** 2.4
    })
    return 0.2126 * values[0]! + 0.7152 * values[1]! + 0.0722 * values[2]!
  }
  const left = luminance(foreground)
  const right = luminance(background)
  return (Math.max(left, right) + 0.05) / (Math.min(left, right) + 0.05)
}

async function expectPalettePrecedence(page: import('@playwright/test').Page) {
  const colors: HighlightRule['color'][] = [
    'red-2',
    'orange-2',
    'amber-3',
    'green-2',
    'blue-2',
    'purple-2',
  ]
  const fieldScope = await page
    .locator('.grid-highlight-cell .field-value')
    .first()
    .evaluate((field) =>
      field.getAttributeNames().filter((name) => name.startsWith('data-v-')),
    )
  expect(fieldScope.length).toBeGreaterThan(0)
  const { references, samples } = await page.evaluate(
    ({ palette, scope }) => {
      const container = document.createElement('div')
      container.style.cssText = 'position:fixed;left:-10000px;top:0'
      document.body.append(container)
      const sample = (row?: string, cell?: string) => {
        const element = document.createElement('div')
        element.className = 'grid-highlight-cell'
        if (row) element.dataset.rowHighlight = row
        if (cell) element.dataset.cellHighlight = cell
        const readOnly = document.createElement('span')
        readOnly.className = 'field-value field-readonly'
        readOnly.textContent = 'Read-only'
        const field = document.createElement('span')
        field.className = 'field-value'
        const link = document.createElement('a')
        link.className = 'field-link'
        link.href = '#'
        link.textContent = 'Link'
        const invalid = document.createElement('span')
        invalid.className = 'grid-cell-invalid'
        invalid.textContent = 'Invalid'
        for (const attribute of scope) {
          readOnly.setAttribute(attribute, '')
          field.setAttribute(attribute, '')
          link.setAttribute(attribute, '')
        }
        field.append(link)
        element.append(readOnly, field, invalid)
        container.append(element)
        const style = getComputedStyle(element)
        return {
          background: style.backgroundColor,
          color: style.color,
          readOnly: getComputedStyle(readOnly).color,
          link: getComputedStyle(link).color,
          invalid: getComputedStyle(invalid).color,
        }
      }
      try {
        const references = palette.map((color) => ({
          color,
          row: sample(color),
          cell: sample(undefined, color),
        }))
        const samples = palette.flatMap((row) =>
          palette.map((cell, index) => ({
            row,
            cell,
            actual: sample(row, cell),
            expected: references[index]!.cell,
          })),
        )
        return { references, samples }
      } finally {
        container.remove()
      }
    },
    { palette: colors, scope: fieldScope },
  )
  for (const reference of references) {
    expect(reference.row, reference.color).toEqual(reference.cell)
    expect(
      contrastRatio(reference.cell.color, reference.cell.background),
      reference.color,
    ).toBeGreaterThanOrEqual(4.5)
    expect(
      contrastRatio(reference.cell.readOnly, reference.cell.background),
      `${reference.color} read-only text`,
    ).toBeGreaterThanOrEqual(4.5)
    expect(
      contrastRatio(reference.cell.link, reference.cell.background),
      `${reference.color} link`,
    ).toBeGreaterThanOrEqual(4.5)
    expect(
      contrastRatio(reference.cell.invalid, reference.cell.background),
      `${reference.color} validation message`,
    ).toBeGreaterThanOrEqual(4.5)
  }
  for (const sample of samples)
    expect(sample.actual, `${sample.row} row / ${sample.cell} cell`).toEqual(
      sample.expected,
    )
}

test('AT-505 configures ordered rules, highlights cells, and uses readable theme tokens', async ({
  page,
}, testInfo) => {
  test.skip(
    testInfo.project.name.toLowerCase().includes('mobile'),
    'The production data grid is desktop-only.',
  )
  test.setTimeout(180_000)
  await signIn(page)
  await page.setViewportSize({ width: 1280, height: 900 })

  const stamp = Date.now()
  const create = async (path: string, data: unknown) => {
    const response = await page.request.post(path, { headers: headers(), data })
    expect(response.status()).toBe(201)
    return response.json()
  }
  const workspace = (await create('/api/v1/workspaces', {
    name: `Highlight ${stamp}`,
  })) as { workspace: { id: string } }
  const book = (await create(
    `/api/v1/workspaces/${workspace.workspace.id}/books`,
    { name: `Highlight book ${stamp}` },
  )) as { book: { id: string } }
  const schema = (await create(`/api/v1/books/${book.book.id}/tables`, {
    name: `Highlight table ${stamp}`,
  })) as TableSchema
  const tableId = schema.table.id
  const createdField = async (name: string, uiType: string) => {
    const result = (await create(`/api/v1/tables/${tableId}/fields`, {
      name,
      uiType,
    })) as { field: TableSchema['fields'][number] }
    return result.field
  }
  const status = await createdField('Status', 'text')
  const due = await createdField('Due', 'date')
  const today = await page.evaluate(() =>
    new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Shanghai' }).format(
      new Date(),
    ),
  )
  const inserted = await page.request.post(`/api/v1/tables/${tableId}/rows`, {
    headers: headers(),
    data: {
      rows: [
        {
          fields: {
            [schema.table.displayFieldId!]: 'Overdue task',
            [status.id]: 'Overdue',
            [due.id]: dateOffset(today, 1),
          },
        },
        {
          fields: {
            [schema.table.displayFieldId!]: 'Open task',
            [status.id]: 'Open',
            [due.id]: dateOffset(today, 1),
          },
        },
      ],
    },
  })
  expect(inserted.status()).toBe(201)
  const schemaPath = `/api/v1/tables/${tableId}/schema`
  const readView = async () => {
    const response = await page.request.get(schemaPath)
    expect(response.status()).toBe(200)
    return ((await response.json()) as TableSchema).views[0]!
  }
  const view = await readView()
  const configured = await page.request.patch(`/api/v1/views/${view.id}`, {
    headers: { ...headers(), 'If-Match': String(view.version) },
    data: {
      config: {
        ...view.config,
        rowHeight: 'medium',
        filter: { field: status.id, cmp: 'is_not_empty', value: null },
        sort: [{ field: status.id, dir: 'asc' }],
      },
    },
  })
  expect(configured.status()).toBe(200)
  const baseline = (
    (await configured.json()) as { view: TableSchema['views'][number] }
  ).view.config
  const preservedConfig = {
    fields: baseline.fields,
    frozen: baseline.frozen,
    filter: baseline.filter,
    rowHeight: baseline.rowHeight,
    sort: baseline.sort,
  }
  const savedOrder = async () =>
    (await readView()).config.highlight?.map((rule) => rule.color) ?? []

  await page.goto(`/t/${tableId}/v/${view.id}`)
  await expect(page.getByText('Overdue task', { exact: true })).toBeVisible()
  await setTheme(page, 'light')
  const menu = page.locator('details.grid-highlights')
  const trigger = menu.locator('summary')
  await expect(trigger).toBeVisible()
  await expect(trigger).toHaveAccessibleName(en.grid.highlightMenuLabel)
  await expect(trigger.locator('svg')).toHaveCount(1)
  await trigger.click()
  await expect(menu.getByText(en.grid.highlightDescription)).toBeVisible()
  await expect(menu.getByText(en.grid.highlightOrderHelp)).toBeVisible()
  await page
    .getByRole('button', { name: en.grid.addHighlightRule, exact: true })
    .click()

  const rules = page.locator('.grid-highlight-rule')
  const setStatusCondition = async (
    index: number,
    value: string,
    color: HighlightRule['color'],
  ) => {
    const rule = rules.nth(index)
    const condition = rule.locator('.filter-item').first()
    await rule
      .getByRole('button', { name: en.filter.addCondition, exact: true })
      .click()
    await condition.getByRole('combobox').nth(0).selectOption(status.id)
    await condition.getByRole('combobox').nth(1).selectOption('is')
    await condition.getByRole('textbox').fill(value)
    await rule
      .getByLabel(en.grid.highlightColor.replace('{position}', `${index + 1}`))
      .selectOption(color)
  }
  const setDueCondition = async (
    index: number,
    color: HighlightRule['color'],
  ) => {
    const rule = rules.nth(index)
    const condition = rule.locator('.filter-item').first()
    await rule
      .getByRole('button', { name: en.filter.addCondition, exact: true })
      .click()
    await condition.getByRole('combobox').nth(0).selectOption(due.id)
    await condition.getByRole('combobox').nth(1).selectOption('is_within')
    await condition.getByRole('combobox').nth(2).selectOption('next:3')
    await rule
      .getByLabel(en.grid.highlightTarget.replace('{position}', `${index + 1}`))
      .selectOption('cells')
    await rule
      .getByLabel(en.grid.highlightFields.replace('{position}', `${index + 1}`))
      .selectOption(due.id)
    await rule
      .getByLabel(en.grid.highlightColor.replace('{position}', `${index + 1}`))
      .selectOption(color)
  }

  await expect(menu.getByText(en.grid.highlightIncomplete)).toBeVisible()
  expect(await savedOrder()).toEqual([])
  await setStatusCondition(0, 'Overdue', 'red-2')
  await page
    .getByRole('button', { name: en.grid.addHighlightRule, exact: true })
    .click()
  await setDueCondition(1, 'amber-3')

  await page
    .getByRole('button', { name: en.grid.addHighlightRule, exact: true })
    .click()
  await setStatusCondition(2, 'Overdue', 'blue-2')
  await page
    .getByRole('button', { name: en.grid.addHighlightRule, exact: true })
    .click()
  await setDueCondition(3, 'green-2')
  await expect
    .poll(savedOrder)
    .toEqual(['red-2', 'amber-3', 'blue-2', 'green-2'])
  await page
    .getByRole('button', { name: en.grid.addHighlightRule, exact: true })
    .click()
  const emptyRule = rules.nth(4)
  await emptyRule
    .getByRole('button', { name: en.filter.addCondition, exact: true })
    .click()
  const emptyCondition = emptyRule.locator('.filter-item').first()
  await emptyCondition.getByRole('combobox').nth(0).selectOption(status.id)
  await emptyCondition.getByRole('combobox').nth(1).selectOption('is_empty')
  await emptyRule
    .getByLabel(en.grid.highlightColor.replace('{position}', '5'))
    .selectOption('purple-2')
  await expect
    .poll(savedOrder)
    .toEqual(['red-2', 'amber-3', 'blue-2', 'green-2', 'purple-2'])
  expect((await readView()).config.highlight?.[4]?.when).toEqual({
    op: 'and',
    items: [{ field: status.id, cmp: 'is_empty', value: null }],
  })
  await emptyRule
    .getByRole('button', {
      name: en.grid.removeHighlightRule.replace('{position}', '5'),
      exact: true,
    })
    .click()
  await expect
    .poll(savedOrder)
    .toEqual(['red-2', 'amber-3', 'blue-2', 'green-2'])

  const query = async () => {
    const response = await page.request.post(`/api/v1/views/${view.id}/query`, {
      headers: headers(),
      data: { limit: 100 },
    })
    expect(response.status()).toBe(200)
    return (await response.json()) as {
      rows: {
        id: string
        _hl_row?: string
        _hl_cells?: Record<string, string>
      }[]
    }
  }
  const insertedRows = ((await inserted.json()) as { rows: { id: string }[] })
    .rows
  const overdueID = insertedRows[0]!.id
  const openID = insertedRows[1]!.id
  const firstResult = await query()
  const overdueRow = firstResult.rows.find((row) => row.id === overdueID)
  expect(overdueRow?._hl_row).toBe('red-2')
  expect(overdueRow?._hl_cells?.[due.id]).toBe('amber-3')
  const openRow = firstResult.rows.find((row) => row.id === openID)
  expect(openRow?._hl_row).toBeUndefined()
  expect(openRow?._hl_cells?.[due.id]).toBe('amber-3')

  const statusInput = rules.nth(0).locator('.filter-item input')
  await statusInput.fill('Delayed')
  await expect
    .poll(
      async () =>
        (await query()).rows.find((row) => row.id === overdueID)?._hl_row,
    )
    .toBe('blue-2')
  await statusInput.fill('Overdue')
  await expect
    .poll(
      async () =>
        (await query()).rows.find((row) => row.id === overdueID)?._hl_row,
    )
    .toBe('red-2')

  const cellHighlight = page.locator(
    '.grid-highlight-cell[data-cell-highlight="amber-3"]',
  )
  await expect(cellHighlight.first()).toBeVisible()
  const lightColors = await cellHighlight.first().evaluate((element) => {
    const style = getComputedStyle(element)
    return { background: style.backgroundColor, color: style.color }
  })
  expect(
    contrastRatio(lightColors.color, lightColors.background),
  ).toBeGreaterThanOrEqual(4.5)
  const rowHighlight = page.locator(
    '.grid-highlight-cell[data-row-highlight="red-2"]:not([data-cell-highlight])',
  )
  await expect(rowHighlight.first()).toBeVisible()
  const lightRowColors = await rowHighlight.first().evaluate((element) => {
    const style = getComputedStyle(element)
    return { background: style.backgroundColor, color: style.color }
  })
  expect(
    contrastRatio(lightRowColors.color, lightRowColors.background),
  ).toBeGreaterThanOrEqual(4.5)
  await expectPalettePrecedence(page)

  await expect(rules.nth(2).locator('h3')).toHaveAttribute('draggable', 'true')
  await page.evaluate(() => {
    const source = document
      .querySelectorAll<HTMLElement>('.grid-highlight-rule')
      .item(2)
      ?.querySelector('h3')
    const target = document
      .querySelectorAll<HTMLElement>('.grid-highlight-rule')
      .item(0)
    if (!source || !target) throw new Error('Highlight reorder targets missing')
    const dataTransfer = new DataTransfer()
    source.dispatchEvent(
      new DragEvent('dragstart', { bubbles: true, dataTransfer }),
    )
    target.dispatchEvent(
      new DragEvent('dragover', {
        bubbles: true,
        cancelable: true,
        dataTransfer,
      }),
    )
    target.dispatchEvent(
      new DragEvent('drop', { bubbles: true, cancelable: true, dataTransfer }),
    )
    source.dispatchEvent(
      new DragEvent('dragend', { bubbles: true, dataTransfer }),
    )
  })
  await expect
    .poll(savedOrder)
    .toEqual(['blue-2', 'red-2', 'amber-3', 'green-2'])
  let reorderedRow = (await query()).rows.find((row) => row.id === overdueID)
  expect(reorderedRow?._hl_row).toBe('blue-2')
  expect(reorderedRow?._hl_cells?.[due.id]).toBe('amber-3')
  const overlappingCell = page.locator(
    '.grid-highlight-cell[data-row-highlight="blue-2"][data-cell-highlight="amber-3"]',
  )
  await expect(overlappingCell).toBeVisible()
  await expect(overlappingCell).toHaveCSS(
    'background-color',
    lightColors.background,
  )
  await expect(overlappingCell).toHaveCSS('color', lightColors.color)

  await page
    .getByRole('button', {
      name: en.grid.moveHighlightDown.replace('{position}', '1'),
    })
    .click()
  await expect
    .poll(savedOrder)
    .toEqual(['red-2', 'blue-2', 'amber-3', 'green-2'])
  reorderedRow = (await query()).rows.find((row) => row.id === overdueID)
  expect(reorderedRow?._hl_row).toBe('red-2')
  expect(reorderedRow?._hl_cells?.[due.id]).toBe('amber-3')

  await page
    .getByRole('button', {
      name: en.grid.moveHighlightUp.replace('{position}', '4'),
      exact: true,
    })
    .click()
  await expect
    .poll(savedOrder)
    .toEqual(['red-2', 'blue-2', 'green-2', 'amber-3'])
  reorderedRow = (await query()).rows.find((row) => row.id === overdueID)
  expect(reorderedRow?._hl_row).toBe('red-2')
  expect(reorderedRow?._hl_cells?.[due.id]).toBe('green-2')
  await page
    .getByRole('button', {
      name: en.grid.moveHighlightDown.replace('{position}', '3'),
      exact: true,
    })
    .click()
  await expect
    .poll(savedOrder)
    .toEqual(['red-2', 'blue-2', 'amber-3', 'green-2'])
  await page
    .getByRole('button', {
      name: en.grid.removeHighlightRule.replace('{position}', '2'),
      exact: true,
    })
    .click()
  await expect.poll(savedOrder).toEqual(['red-2', 'amber-3', 'green-2'])
  await page
    .getByRole('button', {
      name: en.grid.moveHighlightDown.replace('{position}', '1'),
      exact: true,
    })
    .click()
  await expect.poll(savedOrder).toEqual(['amber-3', 'red-2', 'green-2'])
  expect((await readView()).config).toMatchObject(preservedConfig)

  await page.reload()
  await expect(page.getByText('Overdue task', { exact: true })).toBeVisible()
  await expect(trigger).toHaveAccessibleName(en.grid.highlightMenuLabel)
  await trigger.click()
  await expect(rules).toHaveCount(3)
  for (const [index, color] of ['amber-3', 'red-2', 'green-2'].entries())
    await expect(
      rules
        .nth(index)
        .getByLabel(
          en.grid.highlightColor.replace('{position}', `${index + 1}`),
        ),
    ).toHaveValue(color)
  await expect(
    rules.nth(0).getByLabel(en.grid.highlightTarget.replace('{position}', '1')),
  ).toHaveValue('cells')
  await expect(
    rules.nth(0).getByLabel(en.grid.highlightFields.replace('{position}', '1')),
  ).toHaveValues([due.id])
  await expect(rules.nth(1).locator('.filter-item input')).toHaveValue(
    'Overdue',
  )
  reorderedRow = (await query()).rows.find((row) => row.id === overdueID)
  expect(reorderedRow?._hl_row).toBe('red-2')
  expect(reorderedRow?._hl_cells?.[due.id]).toBe('amber-3')
  await expect(
    page.locator(
      '.grid-highlight-cell[data-row-highlight="red-2"][data-cell-highlight="amber-3"]',
    ),
  ).toBeVisible()
  expect((await readView()).config).toMatchObject(preservedConfig)

  await setLocale(page, 'zh-CN')
  await expect(trigger).toBeVisible()
  await expect(trigger).toHaveAccessibleName(zh.grid.highlightMenuLabel)
  if ((await menu.getAttribute('open')) === null) await trigger.click()
  await expect(menu.getByText(zh.grid.highlightDescription)).toBeVisible()
  await expect(menu.getByText(zh.grid.highlightOrderHelp)).toBeVisible()
  await expect(
    rules.nth(0).getByLabel(zh.grid.highlightColor.replace('{position}', '1')),
  ).toBeVisible()
  await setLocale(page, 'en')

  await setTheme(page, 'dark')
  const darkCell = page
    .locator('.grid-highlight-cell[data-cell-highlight="amber-3"]')
    .first()
  await expect
    .poll(async () =>
      darkCell.evaluate((element) => getComputedStyle(element).backgroundColor),
    )
    .toBe('rgb(89, 69, 23)')
  const darkColors = await darkCell.evaluate((element) => {
    const style = getComputedStyle(element)
    return { background: style.backgroundColor, color: style.color }
  })
  expect(
    contrastRatio(darkColors.color, darkColors.background),
  ).toBeGreaterThanOrEqual(4.5)
  const darkRowColors = await rowHighlight.first().evaluate((element) => {
    const style = getComputedStyle(element)
    return { background: style.backgroundColor, color: style.color }
  })
  expect(
    contrastRatio(darkRowColors.color, darkRowColors.background),
  ).toBeGreaterThanOrEqual(4.5)
  await expectPalettePrecedence(page)
  await setTheme(page, 'light')
})
