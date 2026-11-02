import { expect, paceLogin, test } from './fixtures'
import { setLocale } from './app-shell'
import en from '../src/i18n/en.json' with { type: 'json' }
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
  await expect(page).toHaveURL(/\/(home|change-password)$/)
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
    if (!channels || channels.length !== 3) return 0
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
  const view = schema.views[0]!

  await page.goto(`/t/${tableId}/v/${view.id}`)
  await expect(page.getByText('Overdue task', { exact: true })).toBeVisible()
  await page.locator('details.grid-highlights > summary').click()
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

  await setStatusCondition(0, 'Overdue', 'red-2')
  await page
    .getByRole('button', { name: en.grid.addHighlightRule, exact: true })
    .click()
  const cellRule = rules.nth(1)
  const cellCondition = cellRule.locator('.filter-item').first()
  await cellRule
    .getByRole('button', { name: en.filter.addCondition, exact: true })
    .click()
  await cellCondition.getByRole('combobox').nth(0).selectOption(due.id)
  await cellCondition.getByRole('combobox').nth(1).selectOption('is_within')
  await cellCondition.getByRole('combobox').nth(2).selectOption('next:3')
  await cellRule
    .getByLabel(en.grid.highlightTarget.replace('{position}', '2'))
    .selectOption('cells')
  await cellRule
    .getByLabel(en.grid.highlightFields.replace('{position}', '2'))
    .selectOption(due.id)
  await cellRule
    .getByLabel(en.grid.highlightColor.replace('{position}', '2'))
    .selectOption('amber-3')

  await page
    .getByRole('button', { name: en.grid.addHighlightRule, exact: true })
    .click()
  await setStatusCondition(2, 'Overdue', 'blue-2')

  const schemaPath = `/api/v1/tables/${tableId}/schema`
  await expect
    .poll(async () => {
      const current = (await (
        await page.request.get(schemaPath)
      ).json()) as TableSchema
      return current.views[0]?.config.highlight?.length ?? 0
    })
    .toBe(3)

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
  const overdueID = ((await inserted.json()) as { rows: { id: string }[] })
    .rows[0]!.id
  const firstResult = await query()
  const overdueRow = firstResult.rows.find((row) => row.id === overdueID)
  expect(overdueRow?._hl_row).toBe('red-2')
  expect(overdueRow?._hl_cells?.[due.id]).toBe('amber-3')

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
    '.grid-highlight-cell[data-row-highlight="red-2"]',
  )
  const lightRowColors = await rowHighlight.first().evaluate((element) => {
    const style = getComputedStyle(element)
    return { background: style.backgroundColor, color: style.color }
  })
  expect(
    contrastRatio(lightRowColors.color, lightRowColors.background),
  ).toBeGreaterThanOrEqual(4.5)

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
    .poll(async () => {
      const current = (await (
        await page.request.get(schemaPath)
      ).json()) as TableSchema
      return current.views[0]?.config.highlight?.[0]?.color
    })
    .toBe('blue-2')
  let reorderedRow = (await query()).rows.find((row) => row.id === overdueID)
  expect(reorderedRow?._hl_row).toBe('blue-2')
  expect(reorderedRow?._hl_cells?.[due.id]).toBe('amber-3')

  await page
    .getByRole('button', {
      name: en.grid.moveHighlightDown.replace('{position}', '1'),
    })
    .click()
  await expect
    .poll(async () => {
      const current = (await (
        await page.request.get(schemaPath)
      ).json()) as TableSchema
      return current.views[0]?.config.highlight?.[0]?.color
    })
    .toBe('red-2')
  reorderedRow = (await query()).rows.find((row) => row.id === overdueID)
  expect(reorderedRow?._hl_row).toBe('red-2')

  await page.emulateMedia({ colorScheme: 'dark' })
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
})
