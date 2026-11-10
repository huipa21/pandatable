import { expect, paceLogin, test } from './fixtures'
import type { TableSchema } from '../src/api/types'
import en from '../src/i18n/en.json' with { type: 'json' }

test('grid header and row hover reveal selection and expand controls', async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, 'Hover controls belong to the desktop grid.')
  await page.setViewportSize({ width: 1440, height: 900 })
  await paceLogin(page)
  await page.goto('/login')
  await page.getByLabel(en.auth.login, { exact: true }).fill('first-admin')
  await page
    .getByLabel(en.auth.password, { exact: true })
    .fill('Setup-password-123')
  await page.getByRole('button', { name: en.auth.signIn, exact: true }).click()
  await expect(page).toHaveURL(/\/home$/)

  async function create<T>(path: string, data: unknown): Promise<T> {
    const response = await page.request.post(path, {
      headers: { Origin: process.env.PT_E2E_BASE_URL!, 'X-PT-CSRF': '1' },
      data,
    })
    expect(response.status()).toBe(201)
    return (await response.json()) as T
  }
  const { workspace } = await create<{ workspace: { id: string } }>(
    '/api/v1/workspaces',
    { name: `Hover ${Date.now()}` },
  )
  const { book } = await create<{ book: { id: string } }>(
    `/api/v1/workspaces/${workspace.id}/books`,
    { name: 'Hover book' },
  )
  const schema = await create<TableSchema>(`/api/v1/books/${book.id}/tables`, {
    name: 'Hover table',
  })
  await create(`/api/v1/tables/${schema.table.id}/fields`, {
    name: 'Notes',
    uiType: 'text',
  })
  await create(`/api/v1/tables/${schema.table.id}/rows`, {
    rows: ['First', 'Second'].map((name) => ({
      fields: { [schema.table.displayFieldId!]: name },
    })),
  })
  await page.goto(`/t/${schema.table.id}/v/${schema.views[0]!.id}`)
  const grid = page.locator('revo-grid')
  const first = grid.locator('.grid-row-number-cell').first()
  const second = grid.locator('.grid-row-number-cell').nth(1)
  const header = grid.locator('.grid-selection-header')
  const checkbox = first.locator('.grid-row-select')
  const expand = first.locator('.grid-row-expand')
  await expect(first).toContainText('1')
  await page.getByRole('toolbar').first().hover()
  await expect(checkbox).toHaveCSS('opacity', '0')
  await expect(expand).toHaveCSS('opacity', '0')
  await expect(first.locator('.grid-row-number')).toHaveCSS('opacity', '1')
  await expect(header.locator('input')).toHaveCSS('opacity', '0')

  for (const theme of ['light', 'dark']) {
    await page.locator('html').evaluate((element, value) => {
      element.setAttribute('data-theme', value)
    }, theme)
    const colors = await grid.evaluate((element) => ({
      header: getComputedStyle(element.querySelector('.rgHeaderCell')!)
        .backgroundColor,
      row: getComputedStyle(element.querySelector('revogr-data .rgCell')!)
        .backgroundColor,
    }))
    expect(colors.header).not.toBe(colors.row)
    await grid.getByText('First', { exact: true }).hover()
    await expect(checkbox).toHaveCSS('opacity', '1')
    await expect(expand).toHaveCSS('opacity', '1')
    await expect(first.locator('.grid-row-number')).toHaveCSS('opacity', '0')
    await expect(second.locator('.grid-row-select')).toHaveCSS('opacity', '0')
    const hovered = grid.locator('revogr-data .grid-row-hover')
    await expect(hovered).toHaveCount(2)
    await expect(hovered.first().locator('.rgCell').first()).not.toHaveCSS(
      'background-color',
      colors.row,
    )
    await page.getByRole('toolbar').first().hover()
    await expect(grid.locator('.grid-row-hover')).toHaveCount(0)
  }
  await header.hover()
  await expect(header.locator('.grid-row-number')).toHaveCSS('opacity', '0')
  const selectAll = header.getByRole('checkbox', {
    name: en.grid.selectAllRows,
  })
  await expect(selectAll).toHaveCSS('opacity', '1')
  await selectAll.check()
  await expect(page.locator('.grid-row-actions')).toContainText('2 selected')
  await expect(checkbox).toBeChecked()
  await selectAll.uncheck()
  await expect(page.locator('.grid-row-actions')).toHaveCount(0)

  await first.hover()
  await checkbox.check()
  await header.hover()
  await expect(selectAll).toHaveJSProperty('indeterminate', true)
  await first.hover()
  await expand.click()
  await expect(page.locator('dialog.row-expand')).toBeVisible()
  await expect(page.locator('dialog.row-expand header h2')).toHaveText('First')
})
