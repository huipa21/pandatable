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
  await page.goto('/home')
  await expect(page.locator('.breadcrumbs')).toHaveCount(0)

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
  const bookName = `Book ${'B'.repeat(65)}`
  const { book } = await create<{ book: { id: string } }>(
    `/api/v1/workspaces/${workspace.id}/books`,
    { name: bookName },
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

  for (const route of [
    {
      path: `/w/${workspace.id}`,
      names: [workspaceName],
      hrefs: [`/w/${workspace.id}`],
    },
    {
      path: `/w/${workspace.id}/b/${book.id}`,
      names: [workspaceName, bookName],
      hrefs: [`/w/${workspace.id}`, `/w/${workspace.id}/b/${book.id}`],
    },
    {
      path: `/t/${schema.table.id}`,
      names: [
        workspaceName,
        bookName,
        schema.table.name,
        schema.views[0]!.name,
      ],
      hrefs: [
        `/w/${workspace.id}`,
        `/w/${workspace.id}/b/${book.id}`,
        `/t/${schema.table.id}`,
        `/t/${schema.table.id}/v/${views[0]!.id}`,
      ],
    },
    {
      path: `/t/${schema.table.id}/v/${views[0]!.id}`,
      names: [
        workspaceName,
        bookName,
        schema.table.name,
        schema.views[0]!.name,
      ],
      hrefs: [
        `/w/${workspace.id}`,
        `/w/${workspace.id}/b/${book.id}`,
        `/t/${schema.table.id}`,
        `/t/${schema.table.id}/v/${views[0]!.id}`,
      ],
    },
  ]) {
    await page.goto(route.path)
    if (route.path === `/t/${schema.table.id}`)
      await expect(page).toHaveURL(
        new RegExp(`/t/${schema.table.id}/v/${views[0]!.id}$`),
      )
    const links = page.locator('.breadcrumbs a')
    await expect(links).toHaveCount(route.names.length)
    for (const [index, name] of route.names.entries()) {
      await expect(links.nth(index)).toHaveText(name)
      await expect(links.nth(index)).toHaveAttribute(
        'href',
        route.hrefs[index]!,
      )
      if (index === route.names.length - 1)
        await expect(links.nth(index)).toHaveAttribute('aria-current', 'page')
      else await expect(links.nth(index)).not.toHaveAttribute('aria-current')
    }
    const workspaceCrumb = page.locator('.breadcrumbs .breadcrumb-workspace')
    await expect(workspaceCrumb).toHaveText(workspaceName)
    await bounded(page, workspaceCrumb)
    const toggle = (await page.locator('.sidebar-toggle').boundingBox())!
    const crumb = (await workspaceCrumb.boundingBox())!
    expect(crumb.x).toBeGreaterThanOrEqual(toggle.x + toggle.width - 1)
    await expect(
      page.getByRole('heading', { name: en.auth.homeTitle, exact: true }),
    ).toHaveCount(0)
    if (!isMobile && route.path.startsWith('/t/')) {
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
    await page.setViewportSize({ width: 1280, height: 844 })
    const toolbar = page.locator('.grid-toolbar')
    for (const locale of ['en', 'zh-CN'] as const) {
      for (const theme of ['light', 'dark'] as const) {
        await setLocale(page, locale)
        await setTheme(page, theme)
        const labels = locale === 'en' ? en : zh
        const newEntry = toolbar.getByRole('button', {
          name: labels.gridPolish.gp10.new,
          exact: true,
        })
        await expect(newEntry).toBeVisible()
        await expect
          .poll(async () => (await toolbar.boundingBox())?.height)
          .toBeLessThanOrEqual(56)
        const actionBounds = (await toolbar
          .locator('.table-toolbar-actions')
          .boundingBox())!
        const entryBounds = (await newEntry.boundingBox())!
        expect(entryBounds.x).toBeGreaterThanOrEqual(
          actionBounds.x + actionBounds.width - 1,
        )
        await noPageOverflow(page)
      }
    }
    await setLocale(page, 'en')
    await setTheme(page, 'light')
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
      if (width === 390) await noPageOverflow(page)
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
      await expect(toolbar.locator('.schema-summary')).toHaveCount(0)
      await expect(toolbar.locator('.visibility-menu')).toBeVisible()
      await expect(toolbar.locator('.view-export')).toBeVisible()
      if (view.type === 'grid' && width >= 768) {
        await expect(toolbar.locator('.grid-format')).toBeVisible()
        await expect(toolbar.locator('.grid-search')).toBeVisible()
        await expect(toolbar.locator('.grid-more')).toHaveCount(0)
        await expect(toolbar.locator('.grid-column-options')).toHaveCount(0)
      }
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
          name: labels.gridPolish.gp06.import,
          exact: true,
        }),
      ).toBeVisible()
      await expect(
        exportMenu.getByRole('button', {
          name: labels.gridPolish.gp06.export,
          exact: true,
        }),
      ).toHaveCount(1)
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
          ['.grid-format > summary', '.grid-format-menu'],
          ['.grid-search > summary', '.grid-search-menu'],
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

        const searchMenu = toolbar.locator('.grid-search')
        const searchTrigger = searchMenu.locator(':scope > summary')
        await searchTrigger.focus()
        await page.keyboard.press('Enter')
        const searchInput = searchMenu.getByRole('searchbox', {
          name: labels.grid.quickSearch,
          exact: true,
        })
        await searchInput.focus()
        await page.keyboard.press('Escape')
        await expect(searchMenu).not.toHaveAttribute('open', '')
        await expect(searchTrigger).toBeFocused()
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
          .getByRole('button', {
            name: labels.gridPolish.gp06.export,
            exact: true,
          })
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

test('GP-10.3 places field-scoped search immediately left of New', async ({
  page,
}) => {
  test.setTimeout(120_000)
  test.skip(
    (page.viewportSize()?.width ?? 0) < 768,
    'production grid is desktop-only',
  )
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
  const { workspace } = await create<{ workspace: { id: string } }>(
    '/api/v1/workspaces',
    { name: `Search UI ${stamp}` },
  )
  const { book } = await create<{ book: { id: string } }>(
    `/api/v1/workspaces/${workspace.id}/books`,
    { name: `Search UI book ${stamp}` },
  )
  const created = await create<TableSchema>(`/api/v1/books/${book.id}/tables`, {
    name: `Search UI table ${stamp}`,
  })
  const tableId = created.table.id
  const { field: comment } = await create<{
    field: TableSchema['fields'][number]
  }>(`/api/v1/tables/${tableId}/fields`, { name: 'Comment', uiType: 'text' })
  const schema = (await (
    await page.request.get(`/api/v1/tables/${tableId}/schema`)
  ).json()) as TableSchema
  const nameField = schema.fields.find(
    (field) => field.id === schema.table.displayFieldId,
  )!
  await create(`/api/v1/tables/${tableId}/rows`, {
    rows: [
      { fields: { [nameField.id]: 'Zebra name', [comment.id]: 'plain' } },
      { fields: { [nameField.id]: 'Plain', [comment.id]: 'zebra comment' } },
    ],
  })

  await page.setViewportSize({ width: 1280, height: 844 })
  await page.goto(`/t/${tableId}/v/${schema.views[0]!.id}`)
  const toolbar = page.locator('.grid-toolbar')
  const end = toolbar.locator('.grid-toolbar-end')
  const search = end.locator(':scope > .grid-search')
  const trigger = search.locator(':scope > summary')
  const newEntry = end.getByRole('button', {
    name: en.gridPolish.gp10.new,
    exact: true,
  })
  await expect(trigger).toBeVisible()
  await expect(newEntry).toBeVisible()
  expect(
    await search.evaluate((element) =>
      element.nextElementSibling?.classList.contains('new-entry'),
    ),
  ).toBe(true)
  const searchBounds = (await trigger.boundingBox())!
  const newBounds = (await newEntry.boundingBox())!
  expect(searchBounds.x + searchBounds.width).toBeLessThanOrEqual(
    newBounds.x + 1,
  )
  expect(newBounds.x - (searchBounds.x + searchBounds.width)).toBeLessThan(16)

  const style = (locator: Locator) =>
    locator.evaluate((element) => {
      const computed = getComputedStyle(element)
      return {
        height: element.getBoundingClientRect().height,
        fontFamily: computed.fontFamily,
        fontSize: computed.fontSize,
        fontWeight: computed.fontWeight,
      }
    })
  expect(await style(newEntry)).toEqual(
    await style(toolbar.locator('.grid-fields-trigger')),
  )

  await trigger.click()
  const menu = search.locator('.grid-search-menu')
  await expect(menu).toBeVisible()
  await menu.locator('.grid-search-field-button').click()
  await expect(
    menu.getByRole('option', { name: en.gridPolish.gp10.allFields }),
  ).toHaveAttribute('aria-selected', 'true')
  await menu.getByRole('option', { name: nameField.name, exact: true }).click()
  const scoped = page.waitForResponse((response) => {
    const request = response.request()
    if (
      request.method() !== 'POST' ||
      !new URL(response.url()).pathname.endsWith('/query')
    )
      return false
    const body = request.postDataJSON() as {
      search?: string
      searchField?: string
    }
    return body.search === 'zebra' && body.searchField === nameField.id
  })
  await menu
    .getByRole('searchbox', { name: en.grid.quickSearch, exact: true })
    .fill('zebra')
  const result = (await (await scoped).json()) as {
    rows: { fields: Record<string, unknown> }[]
  }
  expect(result.rows.map((row) => row.fields[nameField.id])).toEqual([
    'Zebra name',
  ])
  await expect(trigger.locator('.grid-search-dot')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(search).not.toHaveAttribute('open', '')
  await expect(trigger).toBeFocused()
})
