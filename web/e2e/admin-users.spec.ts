import type { Page } from '@playwright/test'
import { test, expect, observeBrowser } from './fixtures'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }

async function signIn(page: Page, login: string, password: string) {
  await page.goto('/login')
  await page.getByLabel(en.auth.login, { exact: true }).fill(login)
  await page.getByLabel(en.auth.password, { exact: true }).fill(password)
  await page.getByRole('button', { name: en.auth.signIn, exact: true }).click()
}

async function noOverflow(page: Page) {
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true)
}

test('AT-104..108: admin users, forced change, reset, disable and panel visibility', async ({
  page,
  browser,
  api,
}, testInfo) => {
  const username = `alice-${testInfo.project.name.startsWith('mobile') ? 'mobile' : 'desktop'}`
  const temporary = 'Temporary-password-123'
  const changed = 'Changed-password-123'
  const reset = 'Reset-password-123'
  const headers = { Origin: process.env.PT_E2E_BASE_URL!, 'X-PT-CSRF': '1' }
  const unauthenticated = await api.post('/api/v1/admin/users', {
    headers,
    data: {},
  })
  expect(unauthenticated.status()).toBe(401)
  await signIn(page, 'first-admin', 'Setup-password-123')
  await expect(page).toHaveURL(/\/home$/)
  await expect(
    page.getByRole('link', { name: en.admin.menu, exact: true }),
  ).toBeVisible()
  await page.getByRole('link', { name: en.admin.menu, exact: true }).click()
  await expect(page).toHaveURL(/\/admin\/users$/)
  await page.getByRole('button', { name: en.admin.create, exact: true }).click()
  await page
    .getByLabel(en.admin.username, { exact: true })
    .fill(` ${username.toUpperCase()} `)
  await page
    .getByLabel(en.admin.email, { exact: true })
    .fill(` ${username.toUpperCase()}@EXAMPLE.TEST `)
  await page.getByLabel(en.admin.displayName, { exact: true }).fill('Alice')
  await page
    .getByLabel(en.admin.temporaryPassword, { exact: true })
    .fill(temporary)
  await noOverflow(page)
  await page
    .getByRole('button', { name: en.admin.confirm, exact: true })
    .click()
  const card = page.locator(`[data-user="${username}"]`)
  await expect(card).toBeVisible()
  await expect(card).toContainText(`${username}@example.test`)

  // Real edit/role changes in both languages, without adding sign-in attempts.
  await page
    .getByRole('button', { name: en.common.languages.zhCN, exact: true })
    .click()
  await expect(
    page.getByRole('heading', { name: zh.admin.users }),
  ).toBeVisible()
  await card.getByRole('button', { name: zh.admin.edit, exact: true }).click()
  await page
    .getByLabel(zh.admin.displayName, { exact: true })
    .fill('Alice 更新')
  await page.getByLabel(zh.admin.role, { exact: true }).selectOption('admin')
  await noOverflow(page)
  await page
    .getByRole('button', { name: zh.admin.confirm, exact: true })
    .click()
  await expect(card).toContainText(zh.admin.administrator)
  await card.getByRole('button', { name: zh.admin.edit, exact: true }).click()
  await page.getByLabel(zh.admin.role, { exact: true }).selectOption('user')
  await page
    .getByRole('button', { name: zh.admin.confirm, exact: true })
    .click()
  await expect(card).toContainText(zh.admin.regularUser)
  await page
    .getByRole('button', { name: zh.common.languages.en, exact: true })
    .click()
  await page
    .getByLabel(en.admin.searchHint, { exact: true })
    .fill(username.toUpperCase())
  await page.getByRole('button', { name: en.admin.search, exact: true }).click()
  await expect(page.locator('.user-card')).toHaveCount(1)
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true)
  expect(
    await card.evaluate((element) => getComputedStyle(element).display),
  ).toBe(testInfo.project.name.startsWith('mobile') ? 'block' : 'flex')

  const userContext = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL,
    viewport: page.viewportSize(),
    locale: 'en-US',
  })
  const userPage = await userContext.newPage()
  const diagnostics = await observeBrowser(userPage)
  try {
    await signIn(userPage, username, temporary)
    await expect(userPage).toHaveURL(/\/change-password$/)
    await userPage.goto('/admin/users')
    await expect(userPage).toHaveURL(/\/change-password$/)
    const forcedAPI = await userContext.request.get('/api/v1/admin/users')
    expect(forcedAPI.status()).toBe(403)
    expect(await forcedAPI.json()).toMatchObject({
      error: { code: 'ERR_PASSWORD_CHANGE_REQUIRED' },
    })
    await userPage
      .getByLabel(en.auth.currentPassword, { exact: true })
      .fill(temporary)
    await userPage
      .getByLabel(en.auth.newPassword, { exact: true })
      .fill(changed)
    await userPage
      .getByLabel(en.auth.confirmPassword, { exact: true })
      .fill(changed)
    await noOverflow(userPage)
    await userPage
      .getByRole('button', { name: en.auth.changePassword, exact: true })
      .click()
    await expect(userPage).toHaveURL(/\/login$/)
    await expect(userPage.getByRole('alert')).toContainText(
      en.auth.passwordChanged,
    )
    await signIn(userPage, username, changed)
    await expect(userPage).toHaveURL(/\/home$/)
    await expect(
      userPage.getByRole('link', { name: en.admin.menu, exact: true }),
    ).toHaveCount(0)
    await expect(
      userPage.getByRole('link', { name: /sign.?up|register/i }),
    ).toHaveCount(0)
    await userPage.goto('/admin/users')
    await expect(userPage).toHaveURL(/\/home$/)
    await userPage.goto('/admin/future')
    await expect(userPage).toHaveURL(/\/home$/)
    const forbidden = await userContext.request.post('/api/v1/admin/users', {
      headers,
      data: {},
    })
    expect(forbidden.status()).toBe(403)
    expect(await forbidden.json()).toMatchObject({
      error: { code: 'ERR_PERMISSION_DENIED' },
    })

    await card
      .getByRole('button', { name: en.admin.reset, exact: true })
      .click()
    await page
      .getByLabel(en.admin.temporaryPassword, { exact: true })
      .fill(reset)
    await page
      .getByRole('button', { name: en.admin.confirm, exact: true })
      .click()
    await expect(page.getByRole('alert')).toContainText(en.admin.saved)
    const revoked = await userContext.request.get('/api/v1/auth/me')
    expect(revoked.status()).toBe(401)
    await signIn(userPage, username, reset)
    await expect(userPage).toHaveURL(/\/change-password$/)

    await card
      .getByRole('button', { name: en.admin.disable, exact: true })
      .click()
    await page
      .getByRole('button', { name: en.admin.confirm, exact: true })
      .click()
    await expect(card).toContainText(en.admin.disabled)
    await userPage.goto('/home')
    await expect(userPage).toHaveURL(/\/login$/)
    const denied = await userContext.request.post('/api/v1/auth/login', {
      headers,
      data: { login: username, password: reset },
    })
    expect(denied.status()).toBe(401)
    expect(await denied.json()).toMatchObject({
      error: { code: 'ERR_INVALID_CREDENTIALS' },
    })
    await page
      .getByLabel(en.admin.status, { exact: true })
      .selectOption('disabled')
    await expect(card).toBeVisible()
    await expect(card).toContainText('Alice 更新')
    await card
      .getByRole('button', { name: en.admin.enable, exact: true })
      .click()
    await page
      .getByRole('button', { name: en.admin.confirm, exact: true })
      .click()
    await expect(card).toHaveCount(0)
    await page
      .getByLabel(en.admin.status, { exact: true })
      .selectOption('active')
    await expect(card).toContainText(en.admin.active)
    expect(diagnostics.errors).toEqual([])
    expect(diagnostics.pendingFailures).toEqual([])
  } finally {
    await userContext.close()
  }
})
