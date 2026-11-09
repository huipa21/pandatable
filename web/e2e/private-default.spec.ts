import type { Page } from '@playwright/test'
import type { WorkspacePreference } from '../src/api/types'
import en from '../src/i18n/en.json' with { type: 'json' }
import { expect, paceLogin, test } from './fixtures'

const preferencePath = '/api/v1/auth/workspace-preference'
const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})
const cleanupStates = new WeakMap<
  Page,
  { original: string; workspace?: string }
>()

test.afterEach(async ({ page }) => {
  const state = cleanupStates.get(page)
  if (!state) return

  const restored = await page.request.put(preferencePath, {
    headers: headers(),
    data: { workspaceId: state.original },
  })
  expect(restored.status()).toBe(200)
  if (state.workspace) {
    const deleted = await page.request.delete(
      `/api/v1/workspaces/${state.workspace}`,
      { headers: headers() },
    )
    expect(deleted.status()).toBe(200)
  }
})

async function login(page: Page) {
  await paceLogin(page)
  await page.goto('/login')
  await page.getByLabel(en.auth.login, { exact: true }).fill('first-admin')
  await page
    .getByLabel(en.auth.password, { exact: true })
    .fill('Setup-password-123')
  await page.getByRole('button', { name: en.auth.signIn, exact: true }).click()
  await expect(page).toHaveURL(/\/(?:home|w\/[^/]+)$/)
}

async function sidebarTree(page: Page, mobile: boolean) {
  if (mobile) {
    const drawer = page.locator('.nav-drawer')
    if (!(await drawer.isVisible()))
      await page.getByRole('button', { name: en.nav.open, exact: true }).click()
    await expect(drawer).toBeVisible()
    return drawer.locator('.nav-tree')
  }
  return page.locator('.sidebar-desktop .nav-tree')
}

test('WMB-02: private workspaces stay out of navigation and direct routes', async ({
  page,
  browser,
  isMobile,
}, testInfo) => {
  test.setTimeout(180_000)
  if (isMobile) await page.setViewportSize({ width: 390, height: 844 })
  await login(page)

  const original = (
    (await (
      await page.request.get(preferencePath)
    ).json()) as WorkspacePreference
  ).workspaceId
  expect(original).toBeTruthy()
  if (!original) throw new Error('Expected an initial preferred workspace')

  const state: { original: string; workspace?: string } = { original }
  cleanupStates.set(page, state)
  const name = `Private workspace ${testInfo.project.name} ${Date.now()}`
  const created = await page.request.post('/api/v1/workspaces', {
    headers: headers(),
    data: { name },
  })
  expect(created.status()).toBe(201)
  state.workspace = (
    (await created.json()) as { workspace: { id: string } }
  ).workspace.id

  const username = `private-${Date.now()}`
  const userResponse = await page.request.post('/api/v1/admin/users', {
    headers: headers(),
    data: {
      username,
      displayName: 'Private workspace reader',
      role: 'user',
      password: 'Temporary-password-123',
    },
  })
  expect(userResponse.status()).toBe(201)

  const userContext = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL,
    viewport: isMobile
      ? { width: 390, height: 844 }
      : { width: 1440, height: 900 },
    isMobile,
    hasTouch: isMobile,
    locale: 'en-US',
  })
  const userPage = await userContext.newPage()
  try {
    await paceLogin()
    let signedIn = await userContext.request.post('/api/v1/auth/login', {
      headers: headers(),
      data: { login: username, password: 'Temporary-password-123' },
    })
    expect(signedIn.status()).toBe(200)
    const passwordChanged = await userContext.request.post(
      '/api/v1/auth/password',
      {
        headers: headers(),
        data: {
          current: 'Temporary-password-123',
          new: 'Changed-password-123',
        },
      },
    )
    expect(passwordChanged.status()).toBe(200)
    await paceLogin()
    signedIn = await userContext.request.post('/api/v1/auth/login', {
      headers: headers(),
      data: { login: username, password: 'Changed-password-123' },
    })
    expect(signedIn.status()).toBe(200)

    await userPage.goto('/home')
    await expect(userPage.locator('.workspace-dashboard')).toBeVisible()
    await expect(userPage.locator('.workspace-dashboard')).not.toContainText(
      name,
    )
    const tree = await sidebarTree(userPage, isMobile)
    await expect(
      tree.locator(`[data-nav-id="${state.workspace}"]`),
    ).toHaveCount(0)
    await expect(tree).not.toContainText(name)
    if (isMobile)
      await userPage.getByRole('button', { name: en.nav.close }).click()

    await userPage.getByRole('button', { name: en.search.open }).click()
    const search = userPage.getByRole('combobox', { name: en.search.input })
    await search.fill(name)
    await expect(userPage.locator('#search-results')).not.toContainText(name)

    await userPage.goto(`/w/${state.workspace}`)
    await expect(
      userPage.getByRole('heading', { name: en.nav.removedTitle }),
    ).toBeVisible()
    await expect(userPage.locator('.resource-page')).not.toContainText(name)
  } finally {
    await userContext.close()
  }
})
