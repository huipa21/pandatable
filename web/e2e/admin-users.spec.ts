import type { Page } from '@playwright/test'
import {
  test,
  expect,
  observeBrowser,
  expectHTTPFailure,
  expectBrowserClean,
} from './fixtures'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }
import { verifyDashboard } from './dashboard'
import { verifySettings, restoreSettings } from './settings'
import { verifyNavigation } from './navigation'

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

async function verifyGroups(page: Page, suffix: string) {
  const headers = { Origin: process.env.PT_E2E_BASE_URL!, 'X-PT-CSRF': '1' }
  const createdUsers: { id: string; username: string; displayName: string }[] =
    []
  for (const name of ['bob', 'carol']) {
    const response = await page.request.post('/api/v1/admin/users', {
      headers,
      data: {
        username: `${name}-${suffix}`,
        displayName: name === 'bob' ? 'Bob' : 'Carol',
        role: 'user',
        password: 'Temporary-password-123',
      },
    })
    expect(response.status()).toBe(201)
    createdUsers.push((await response.json()).user)
  }
  await page.getByRole('link', { name: en.groups.title, exact: true }).click()
  await expect(page.getByText(en.groups.empty, { exact: true })).toBeVisible()
  await page
    .getByRole('button', { name: en.groups.create, exact: true })
    .click()
  await page.getByLabel(en.groups.name, { exact: true }).fill(' team-a ')
  await page
    .getByLabel(en.groups.description, { exact: true })
    .fill('Team description\nSecond line')
  await page
    .getByRole('button', { name: en.admin.confirm, exact: true })
    .click()
  await expect(
    page.getByRole('heading', { name: 'team-a', exact: true }),
  ).toBeVisible()
  await expect(
    page.getByText(en.groups.emptyMembers, { exact: true }),
  ).toBeVisible()
  await noOverflow(page)
  const groupID = new URL(page.url()).pathname.split('/').at(-1)!
  for (const user of createdUsers) {
    await page
      .getByLabel(en.groups.selectUser, { exact: true })
      .selectOption(user.id)
    await page.getByRole('button', { name: en.groups.add, exact: true }).click()
    await expect(page.locator(`[data-member="${user.username}"]`)).toBeVisible()
  }
  await expect(page.locator('[data-member]')).toHaveCount(2)
  await page.screenshot({
    path: test.info().outputPath('groups-en.png'),
    fullPage: true,
  })
  const bob = createdUsers[0]!
  const carol = createdUsers[1]!
  await page
    .locator(`[data-member="${bob.username}"]`)
    .getByRole('link', { name: 'Bob', exact: true })
    .click()
  await expect(page).toHaveURL(new RegExp(`/admin/users/${bob.id}$`))
  await expect(
    page.getByRole('heading', { name: en.groups.memberships, exact: true }),
  ).toBeVisible()
  await expect(page.locator('[data-group="team-a"]')).toContainText('2 members')
  await noOverflow(page)
  await page.screenshot({
    path: test.info().outputPath('user-memberships.png'),
    fullPage: true,
  })
  await page.getByRole('link', { name: 'team-a', exact: true }).click()
  await page
    .locator(`[data-member="${carol.username}"]`)
    .getByRole('link', { name: 'Carol', exact: true })
    .click()
  await expect(page.locator('[data-group="team-a"]')).toBeVisible()
  await page.getByRole('link', { name: 'team-a', exact: true }).click()
  await page
    .getByRole('button', { name: en.common.languages.zhCN, exact: true })
    .click()
  await expect(
    page.getByRole('heading', { name: zh.groups.members, exact: true }),
  ).toBeVisible()
  await page
    .locator(`[data-member="${carol.username}"]`)
    .getByRole('button', { name: zh.groups.remove, exact: true })
    .click()
  await expect(page.locator('[data-member]')).toHaveCount(1)
  await page.getByRole('button', { name: zh.groups.edit, exact: true }).click()
  await page.getByLabel(zh.groups.name, { exact: true }).fill('团队-a')
  await page
    .getByLabel(zh.groups.description, { exact: true })
    .fill('更新后的描述')
  await noOverflow(page)
  await page
    .getByRole('button', { name: zh.admin.confirm, exact: true })
    .click()
  await expect(
    page.getByRole('heading', { name: '团队-a', exact: true }),
  ).toBeVisible()
  await expect(page.getByText('更新后的描述', { exact: true })).toBeVisible()
  await page.screenshot({
    path: test.info().outputPath('groups-zh.png'),
    fullPage: true,
  })
  const conflict = await page.request.post('/api/v1/admin/groups', {
    headers,
    data: { name: 'occupied-name' },
  })
  expect(conflict.status()).toBe(201)
  const conflictID = (await conflict.json()).group.id
  await page.getByRole('button', { name: zh.groups.edit, exact: true }).click()
  await page.getByLabel(zh.groups.name, { exact: true }).fill('occupied-name')
  expectHTTPFailure(page, 'PATCH', `/api/v1/admin/groups/${groupID}`, 409)
  await page
    .getByRole('button', { name: zh.admin.confirm, exact: true })
    .click()
  await expect(page.getByRole('alert')).toContainText(
    zh.errors.ERR_GROUP_NAME_CONFLICT,
  )
  await page.getByRole('button', { name: zh.admin.cancel, exact: true }).click()
  await page
    .getByRole('button', { name: zh.common.languages.en, exact: true })
    .click()
  await page
    .getByRole('link', { name: en.groups.backGroups, exact: true })
    .click()
  await page
    .getByLabel(en.groups.searchHint, { exact: true })
    .fill('no-such-group')
  await page.getByRole('button', { name: en.admin.search, exact: true }).click()
  await expect(page.getByText(en.groups.empty, { exact: true })).toBeVisible()
  await page.getByLabel(en.groups.searchHint, { exact: true }).fill('团队')
  await page.getByRole('button', { name: en.admin.search, exact: true }).click()
  await expect(page.locator('[data-group]')).toHaveCount(1)
  await page.getByRole('link', { name: '团队-a', exact: true }).click()
  await page
    .getByRole('button', { name: en.groups.delete, exact: true })
    .click()
  await noOverflow(page)
  await page.getByRole('button', { name: en.admin.cancel, exact: true }).click()
  await expect(page).toHaveURL(new RegExp(`/admin/groups/${groupID}$`))
  await page
    .getByRole('button', { name: en.groups.delete, exact: true })
    .click()
  await page
    .getByRole('button', { name: en.admin.confirm, exact: true })
    .click()
  await expect(page).toHaveURL(/\/admin\/groups$/)
  expect(
    (await page.request.get(`/api/v1/admin/groups/${groupID}`)).status(),
  ).toBe(404)
  expect(
    (await page.request.get(`/api/v1/admin/users/${bob.id}/groups`)).status(),
  ).toBe(200)
  expect(
    (
      await (
        await page.request.get(`/api/v1/admin/users/${bob.id}/groups`)
      ).json()
    ).total,
  ).toBe(0)
  expect(
    (
      await page.request.delete(`/api/v1/admin/groups/${conflictID}`, {
        headers,
      })
    ).status(),
  ).toBe(200)
  await page.goto(`/admin/users/${bob.id}`)
  await expect(page.getByText(en.groups.empty, { exact: true })).toBeVisible()
  await page
    .getByRole('link', { name: en.groups.backUsers, exact: true })
    .click()
  await expect(page).toHaveURL(/\/admin\/users$/)
  return createdUsers
}

test('AT-104..110/112/201/208/213 and scoped AT-111: admin and real navigation', async ({
  page,
  browser,
  api,
}, testInfo) => {
  test.setTimeout(150_000)
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
  await verifyDashboard(page)
  const participants = await verifyGroups(
    page,
    testInfo.project.name.startsWith('mobile') ? 'mobile' : 'desktop',
  )
  const branding = await verifySettings(page, testInfo.project.name)
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
    isMobile: testInfo.project.name.startsWith('mobile'),
    hasTouch: testInfo.project.name.startsWith('mobile'),
  })
  const userPage = await userContext.newPage()
  const diagnostics = await observeBrowser(userPage)
  try {
    await userPage.goto('/login')
    await expect(userPage.locator('.brand')).toContainText(branding.appName)
    await expect(userPage.locator('html')).toHaveAttribute('lang', 'zh-CN')
    await expect(
      userPage.getByRole('link', { name: zh.settings.title, exact: true }),
    ).toHaveCount(0)
    await userPage
      .getByRole('button', { name: zh.common.languages.en, exact: true })
      .click()
    await userPage.reload()
    await expect(userPage.locator('html')).toHaveAttribute('lang', 'en')
    await expect(userPage.locator('.brand')).toContainText(branding.appName)
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
    await userPage.reload()
    await expect(userPage.locator('.brand')).toContainText(branding.appName)
    await expect(userPage).toHaveTitle(branding.appName)
    const profile = await userContext.request.get('/api/v1/auth/me')
    expect(await profile.json()).toMatchObject({
      locale: 'zh-CN',
      effectiveTimezone: 'Europe/London',
    })
    await expect(
      userPage.getByRole('link', { name: en.admin.menu, exact: true }),
    ).toHaveCount(0)
    await expect(
      userPage.getByRole('link', { name: en.groups.title, exact: true }),
    ).toHaveCount(0)
    await expect(
      userPage.getByRole('link', { name: en.dashboard.title, exact: true }),
    ).toHaveCount(0)
    await expect(
      userPage.getByRole('link', { name: /sign.?up|register/i }),
    ).toHaveCount(0)
    await userPage.goto('/admin/users')
    await expect(userPage).toHaveURL(/\/home$/)
    await userPage.goto('/admin/future')
    await expect(userPage).toHaveURL(/\/home$/)
    for (const path of [
      '/admin/dashboard',
      '/admin/settings',
      '/admin/workspaces',
      '/admin/groups',
      '/admin/groups/missing',
      '/admin/users/missing',
    ]) {
      await userPage.goto(path)
      await expect(userPage).toHaveURL(/\/home$/)
    }
    expect(
      (await userContext.request.get('/api/v1/admin/groups')).status(),
    ).toBe(403)
    for (const path of ['/admin/settings', '/admin/workspaces']) {
      await expect(userPage.locator(`a[href="${path}"]`)).toHaveCount(0)
      expect((await userContext.request.get(`/api/v1${path}`)).status()).toBe(
        403,
      )
    }
    expect(
      (
        await userContext.request.patch('/api/v1/admin/settings', {
          headers,
          data: { appName: 'Denied' },
        })
      ).status(),
    ).toBe(403)
    const systemDenied = await userContext.request.get('/api/v1/admin/system')
    expect(systemDenied.status()).toBe(403)
    expect(await systemDenied.json()).toMatchObject({
      error: { code: 'ERR_PERMISSION_DENIED' },
    })
    const forbidden = await userContext.request.post('/api/v1/admin/users', {
      headers,
      data: {},
    })
    expect(forbidden.status()).toBe(403)
    expect(await forbidden.json()).toMatchObject({
      error: { code: 'ERR_PERMISSION_DENIED' },
    })

    await verifyNavigation(page, userPage, browser, participants, username)

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
    expectBrowserClean(userPage, diagnostics)
  } finally {
    await userContext.close()
    await restoreSettings(page, branding.previous)
  }
})
