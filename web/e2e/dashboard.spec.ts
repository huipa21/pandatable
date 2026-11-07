import en from '../src/i18n/en.json' with { type: 'json' }
import type { Navigation } from '../src/api/types'
import {
  expect,
  expectBrowserClean,
  observeBrowser,
  paceLogin,
  test,
} from './fixtures'

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

test('DS-07: dashboard search, recency, row actions and mobile layout', async ({
  page,
  isMobile,
}) => {
  test.setTimeout(150_000)
  if (isMobile) await page.setViewportSize({ width: 390, height: 844 })
  const diagnostics = await observeBrowser(page)
  let createdWorkspace: string | undefined
  await paceLogin(page)
  await page.goto('/login')
  await page.getByLabel(en.auth.login, { exact: true }).fill('first-admin')
  await page
    .getByLabel(en.auth.password, { exact: true })
    .fill('Setup-password-123')
  await page.getByRole('button', { name: en.auth.signIn, exact: true }).click()

  try {
    await expect(page).toHaveURL(/\/home$/)
    await expect(
      page.locator('main').getByRole('heading', {
        name: en.workspaceGrid.ux02.workspaces,
        exact: true,
      }),
    ).toBeVisible()

    await page
      .locator('main')
      .getByRole('button', {
        name: en.nav.create.workspace,
        exact: true,
      })
      .click()
    const dialog = page.getByRole('dialog', {
      name: en.nav.dialogTitle,
      exact: true,
    })
    await expect(dialog).toBeVisible()
    await dialog
      .getByRole('textbox', { name: en.nav.name, exact: true })
      .fill(`DS07 dashboard ${Date.now()}`)
    await dialog
      .getByRole('textbox', {
        name: en.dashboard.ds06.descriptionLabel,
        exact: true,
      })
      .fill('東京チーム workspace')
    const workspaceOpen = page.waitForResponse((response) => {
      const url = new URL(response.url())
      return (
        url.pathname === '/api/v1/recent' &&
        response.request().method() === 'POST' &&
        response.status() === 204
      )
    })
    await dialog
      .getByRole('button', { name: en.admin.confirm, exact: true })
      .click()
    await expect(dialog).not.toBeVisible()
    await expect(page).toHaveURL(/\/w\/[^/]+$/)
    await workspaceOpen
    const workspaceId = new URL(page.url()).pathname.split('/')[2]
    if (!workspaceId)
      throw new Error('Workspace creation did not open a workspace')
    createdWorkspace = workspaceId

    await page.locator('.brand').click()
    await expect(page).toHaveURL(/\/home$/)
    const list = page.locator('.workspace-dashboard-list')
    await expect(list).toBeVisible()
    const navigationResponse = await page.request.get('/api/v1/nav')
    expect(navigationResponse.status()).toBe(200)
    const navigation = (await navigationResponse.json()) as Navigation
    const renderedIds = await list
      .locator('.workspace-dashboard-row')
      .evaluateAll((rows) =>
        rows
          .map((row) => row.getAttribute('data-nav-id'))
          .filter((id): id is string => id !== null)
          .sort(),
      )
    expect(renderedIds).toEqual(
      navigation.workspaces.map((workspace) => workspace.id).sort(),
    )
    const createdRow = list.locator(`[data-nav-id="${workspaceId}"]`)
    await expect(createdRow).toBeVisible()
    await expect(
      list.locator('.workspace-dashboard-row').first(),
    ).toHaveAttribute('data-nav-id', workspaceId)
    await expect(
      createdRow.locator('.workspace-dashboard-description'),
    ).toHaveAttribute('title', '東京チーム workspace')

    const search = page.getByRole('searchbox', {
      name: en.dashboard.ds07.searchLabel,
      exact: true,
    })
    await search.fill('東京チーム')
    await expect(createdRow).toBeVisible()
    await expect(list.locator('.workspace-dashboard-row')).toHaveCount(1)
    await search.fill('no matching workspace')
    await expect(
      page.getByText(en.dashboard.ds07.noMatches, { exact: true }),
    ).toBeVisible()
    await page
      .getByRole('button', { name: en.dashboard.ds07.clearSearch, exact: true })
      .click()
    await expect(search).toHaveValue('')
    await expect(createdRow).toBeVisible()

    const actions = createdRow.locator(':scope > .node-actions')
    await actions.locator('summary').click()
    await expect(actions).toContainText(en.dashboard.ds06.editWorkspace)
    await expect(actions).toContainText(en.sharing.open)
    await expect(actions).toContainText(en.workspaceGrid.ux02.setDefault)
    await expect(actions).toContainText(en.nav.delete)

    await page.goto(`/w/${workspaceId}`)
    await expect(page).toHaveURL(new RegExp(`/w/${workspaceId}$`))
    await page.goto('/home')
    await expect(page).toHaveURL(/\/home$/)

    await page.setViewportSize({ width: 390, height: 844 })
    await expect(createdRow).toBeVisible()
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true)
    expectBrowserClean(page, diagnostics)
  } finally {
    if (createdWorkspace) {
      const deleted = await page.request.delete(
        `/api/v1/workspaces/${createdWorkspace}`,
        { headers: headers() },
      )
      expect(deleted.status()).toBe(200)
    }
  }
})
