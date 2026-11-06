import type { Locator, Page } from '@playwright/test'
import type { TableSchema } from '../src/api/types'
import { setLocale, setTheme } from './app-shell'
import { expect, expectHTTPFailure, paceLogin, test } from './fixtures'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }

test.use({ actionTimeout: 10_000 })

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

async function bounded(page: Page, locator: Locator) {
  await expect(locator).toBeVisible()
  const bounds = await locator.boundingBox()
  expect(bounds).not.toBeNull()
  expect(bounds!.width).toBeGreaterThan(0)
  expect(bounds!.x).toBeGreaterThanOrEqual(0)
  expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(
    page.viewportSize()!.width + 1,
  )
}

async function noPageOverflow(page: Page) {
  await expect
    .poll(() =>
      page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
    )
    .toBe(true)
}

test('UX-06 consolidates the header across routes, views and narrow layouts', async ({
  page,
  isMobile,
}) => {
  test.setTimeout(240_000)
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

  const create = async <T>(path: string, data: unknown): Promise<T> => {
    const response = await page.request.post(path, { headers: headers(), data })
    expect(response.status(), await response.text()).toBe(201)
    return response.json()
  }
  const stamp = Date.now()
  const workspaceName = `Workspace ${stamp} ${'W'.repeat(60)}`
  const otherName = `Other workspace ${stamp}`
  const { workspace } = await create<{ workspace: { id: string } }>(
    '/api/v1/workspaces',
    { name: workspaceName },
  )
  const other = await create<{ workspace: { id: string } }>(
    '/api/v1/workspaces',
    { name: otherName },
  )
  const { book } = await create<{ book: { id: string } }>(
    `/api/v1/workspaces/${workspace.id}/books`,
    { name: `Book ${'B'.repeat(65)}` },
  )
  const schema = await create<TableSchema>(`/api/v1/books/${book.id}/tables`, {
    name: `Table ${'T'.repeat(65)}`,
  })
  const views: { type: 'grid' | 'form' | 'kanban' | 'gallery'; id: string }[] =
    [{ type: 'grid', id: schema.views[0]!.id }]
  for (const type of ['form', 'kanban', 'gallery'] as const) {
    const result = await create<{ view: { id: string } }>(
      `/api/v1/tables/${schema.table.id}/views`,
      { name: `Header ${type}`, type, scope: 'shared' },
    )
    views.push({ type, id: result.view.id })
  }

  for (const path of [
    `/w/${workspace.id}`,
    `/w/${workspace.id}/b/${book.id}`,
    `/t/${schema.table.id}/v/${views[0]!.id}`,
  ]) {
    await page.goto(path)
    const workspaceCrumb = page.locator('.breadcrumbs .breadcrumb-workspace')
    await expect(workspaceCrumb).toHaveText(workspaceName)
    await bounded(page, workspaceCrumb)
    const toggle = (await page.locator('.sidebar-toggle').boundingBox())!
    const crumb = (await workspaceCrumb.boundingBox())!
    expect(crumb.x).toBeGreaterThanOrEqual(toggle.x + toggle.width - 1)
    await expect(
      page.getByRole('heading', { name: en.auth.homeTitle, exact: true }),
    ).toHaveCount(0)
    if (!isMobile && path.startsWith('/t/')) {
      await expect(
        page.getByRole('button', { name: en.grid.addRow, exact: true }),
      ).toBeEnabled()
      await expect
        .poll(
          async () =>
            (await page.locator('.grid-toolbar').boundingBox())?.height,
        )
        .toBeLessThanOrEqual(56)
    }
  }
  if (!isMobile) {
    await page.getByRole('button', { name: en.nav.collapseSidebar }).click()
    await bounded(page, page.locator('.breadcrumbs .breadcrumb-workspace'))
    await expect(page.locator('.avatar-button')).toBeVisible()
  }
  await page.goto(`/w/${other.workspace.id}`)
  await expect(page.locator('.breadcrumbs .breadcrumb-workspace')).toHaveText(
    otherName,
  )
  if (!isMobile)
    await page.getByRole('button', { name: en.nav.expandSidebar }).click()

  for (const [width, locale, theme] of [
    [820, 'en', 'light'],
    [390, 'zh-CN', 'dark'],
  ] as const) {
    if (isMobile && width === 820) continue
    await page.setViewportSize({ width, height: 844 })
    await setLocale(page, locale)
    await setTheme(page, theme)
    const labels = locale === 'en' ? en : zh
    for (const view of views) {
      await page.goto(`/t/${schema.table.id}/v/${view.id}`)
      const workspaceCrumb = page.locator('.breadcrumbs .breadcrumb-workspace')
      await expect(workspaceCrumb).toHaveText(workspaceName)
      await bounded(page, workspaceCrumb)
      await expect(
        page.locator('.breadcrumbs a[aria-current="page"]'),
      ).toBeVisible()
      const toolbar =
        view.type === 'grid' && width >= 768
          ? page.locator('.grid-toolbar')
          : page.locator('.table-toolbar')
      await expect(toolbar).toBeVisible()
      await expect(
        toolbar.getByRole('button', { name: labels.undo.undo }),
      ).toBeVisible()
      await expect(
        toolbar.getByRole('button', { name: labels.undo.redo }),
      ).toBeVisible()
      await expect(
        toolbar.getByRole('button', {
          name: labels.import.append,
          exact: true,
        }),
      ).toBeVisible()
      await expect(toolbar.locator('.schema-summary')).toContainText(
        labels.nav.level.owner,
      )
      await expect(toolbar.locator('.visibility-menu')).toBeVisible()
      await expect(toolbar.locator('.view-export')).toBeVisible()
      await expect(
        page.locator('.table-management, .view-subheader'),
      ).toHaveCount(0)
      await expect(
        page.getByText('Views and table options', { exact: true }),
      ).toHaveCount(0)
      await expect(
        page.getByText('视图和数据表选项', { exact: true }),
      ).toHaveCount(0)
      await noPageOverflow(page)

      const exportMenu = toolbar.locator('.view-export')
      const exportTrigger = exportMenu.locator('summary')
      await exportTrigger.focus()
      await page.keyboard.press('Enter')
      await bounded(page, exportMenu.locator('.view-export-panel'))
      await expect(
        exportMenu.getByRole('button', {
          name: labels.export.csv,
          exact: true,
        }),
      ).toHaveCount(view.type === 'grid' ? 1 : 0)
      await page.keyboard.press('Escape')
      await expect(exportTrigger).toBeFocused()

      await page.keyboard.press('Enter')
      const visibility = toolbar.locator('.visibility-menu')
      const visibilityTrigger = visibility.locator('summary')
      await visibilityTrigger.focus()
      await page.keyboard.press('Enter')
      await expect(exportMenu).not.toHaveAttribute('open', '')
      await bounded(page, visibility.locator('.visibility-menu-panel'))
      await page.keyboard.press('Escape')
      await expect(visibilityTrigger).toBeFocused()

      if (view.type !== 'grid') continue
      if (width >= 768) {
        const undo = toolbar.getByRole('button', { name: en.undo.undo })
        const redo = toolbar.getByRole('button', { name: en.undo.redo })
        await page
          .getByRole('button', { name: en.grid.addRow, exact: true })
          .click()
        await expect(undo).toBeEnabled()
        await undo.click()
        await expect(redo).toBeEnabled()
        await redo.click()
        await expect(undo).toBeEnabled()

        await toolbar.locator('.grid-fields-trigger').focus()
        await page.keyboard.press('Enter')
        await expect(page.locator('.grid-field-popover')).toBeVisible()
        await toolbar.locator('.grid-groups > summary').focus()
        await page.keyboard.press('Enter')
        await expect(page.locator('.grid-field-popover')).not.toBeVisible()
        await toolbar.locator('.grid-filter-trigger').focus()
        await page.keyboard.press('Enter')
        await expect(toolbar.locator('.grid-groups')).not.toHaveAttribute(
          'open',
          '',
        )
        await expect(page.locator('.grid-filter-popover')).toBeVisible()
        await page.keyboard.press('Enter')

        for (const [trigger, panel] of [
          ['.grid-fields-trigger', '.grid-field-popover'],
          ['.grid-filter-trigger', '.grid-filter-popover'],
          ['.grid-groups > summary', '.grid-groups > fieldset'],
          ['.grid-sorts > summary', '.grid-sorts > fieldset'],
          ['.grid-highlights > summary', '.grid-highlights > fieldset'],
          ['.grid-row-height > summary', '.grid-row-height > fieldset'],
          ['.grid-more > summary', '.grid-more-menu'],
        ] as const) {
          await toolbar.locator(trigger).click()
          const popover = page.locator(panel)
          await bounded(page, popover)
          const bounds = (await popover.boundingBox())!
          const toolbarBounds = (await toolbar.boundingBox())!
          expect(bounds.y).toBeGreaterThanOrEqual(
            toolbarBounds.y + toolbarBounds.height - 1,
          )
          await toolbar.locator(trigger).click()
        }
      } else {
        const filter = toolbar.locator('.view-filter')
        await filter.locator('summary').click()
        await bounded(page, filter.locator('.view-filter-panel'))
        await noPageOverflow(page)
        await filter.locator('summary').click()

        let release!: () => void
        const pending = new Promise<void>((resolve) => {
          release = resolve
        })
        const path = `/api/v1/views/${view.id}/export`
        expectHTTPFailure(page, 'POST', path, 503)
        await page.route(`**${path}`, async (route) => {
          await pending
          await route.fulfill({
            status: 503,
            contentType: 'application/json',
            body: JSON.stringify({ error: { code: 'ERR_INTERNAL' } }),
          })
        })
        await exportTrigger.click()
        await exportMenu
          .getByRole('button', { name: labels.export.package, exact: true })
          .click()
        await expect(
          toolbar.locator('.table-export-feedback[role="status"]'),
        ).toBeVisible()
        await exportTrigger.focus()
        await page.keyboard.press('Escape')
        release()
        await expect(
          toolbar.locator('.table-export-feedback[role="alert"]'),
        ).toBeVisible()
        await page.unroute(`**${path}`)
      }
    }
  }
})
