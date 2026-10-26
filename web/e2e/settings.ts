import type { Page } from '@playwright/test'
import { test, expect, expectHTTPFailure } from './fixtures'
import type { AdminSettings } from '../src/api/types'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }

export async function verifySettings(page: Page, suffix: string) {
  const before = await page.request.get('/api/v1/admin/settings')
  expect(before.status()).toBe(200)
  const previous = (await before.json()) as AdminSettings
  const appName = `Panda Team ${suffix} <plain>`
  let release!: () => void
  const gate = new Promise<void>((resolve) => {
    release = resolve
  })
  await page.route(
    '**/api/v1/admin/settings',
    async (route) => {
      await gate
      await route.continue()
    },
    { times: 1 },
  )
  await page.getByRole('link', { name: en.settings.title, exact: true }).click()
  await expect(
    page.getByText(en.settings.loading, { exact: true }),
  ).toBeVisible()
  release()
  await expect(
    page.getByLabel(en.settings.appName, { exact: true }),
  ).toHaveValue(previous.appName)
  const columns = await page
    .locator('.settings-form')
    .evaluate(
      (element) =>
        getComputedStyle(element).gridTemplateColumns.trim().split(/\s+/)
          .length,
    )
  expect(columns).toBe(page.viewportSize()!.width < 768 ? 1 : 2)
  await page
    .getByLabel(en.settings.appName, { exact: true })
    .fill(` ${appName} `)
  await page
    .getByLabel(en.settings.defaultTimezone, { exact: true })
    .selectOption('Europe/London')
  await page
    .getByLabel(en.settings.defaultCurrency, { exact: true })
    .selectOption('USD')
  await page
    .getByLabel(en.settings.defaultLocale, { exact: true })
    .selectOption('zh-CN')
  await page.getByLabel(en.settings.uploadMaxMb, { exact: true }).fill('7')
  await page
    .getByLabel(en.settings.historyRetentionDays, { exact: true })
    .fill('60')
  await page
    .getByLabel(en.settings.trashRetentionDays, { exact: true })
    .fill('14')
  await page
    .getByRole('button', { name: en.settings.save, exact: true })
    .click()
  await expect(page.getByRole('alert')).toContainText(en.settings.saved)
  await expect(page.locator('.brand')).toContainText(appName)
  // verifyGroups already made an explicit English local choice.
  await expect(page.locator('html')).toHaveAttribute('lang', 'en')
  await page.reload()
  await expect(page.locator('.brand')).toContainText(appName)
  await expect(page).toHaveTitle(appName)
  await expect(
    page.getByLabel(en.settings.uploadMaxMb, { exact: true }),
  ).toHaveValue('7')
  await expect(
    page.getByLabel(en.settings.defaultTimezone, { exact: true }),
  ).toHaveValue('Europe/London')
  const persisted = await page.request.get('/api/v1/admin/settings')
  expect(await persisted.json()).toEqual({
    appName,
    defaultTimezone: 'Europe/London',
    defaultCurrency: 'USD',
    defaultLocale: 'zh-CN',
    uploadMaxMb: 7,
    historyRetentionDays: 60,
    trashRetentionDays: 14,
  })
  const me = await page.request.get('/api/v1/auth/me')
  expect((await me.json()).effectiveTimezone).toBe('Europe/London')
  expect(await (await page.request.get('/api/v1/meta/app')).json()).toEqual({
    appName,
    defaultLocale: 'zh-CN',
  })
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true)
  await page.screenshot({
    path: test.info().outputPath('settings-en.png'),
    fullPage: true,
  })
  await page
    .getByRole('button', { name: en.common.languages.zhCN, exact: true })
    .click()
  await expect(
    page.getByRole('heading', { name: zh.settings.title, exact: true }),
  ).toBeVisible()
  await expect(
    page.getByText(zh.settings.deferredNotice, { exact: true }),
  ).toBeVisible()
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true)
  await page.screenshot({
    path: test.info().outputPath('settings-zh.png'),
    fullPage: true,
  })
  await page
    .getByRole('button', { name: zh.common.languages.en, exact: true })
    .click()

  await page.route(
    '**/api/v1/admin/settings',
    (route) =>
      route.fulfill({
        status: 500,
        contentType: 'application/json',
        body: JSON.stringify({ error: { code: 'ERR_INTERNAL' } }),
      }),
    { times: 1 },
  )
  expectHTTPFailure(page, 'GET', '/api/v1/admin/settings', 500)
  await page
    .getByRole('button', { name: en.settings.reload, exact: true })
    .click()
  await expect(page.getByRole('alert')).toContainText(en.errors.ERR_INTERNAL)
  await page.getByRole('button', { name: en.auth.retry, exact: true }).click()
  await expect(page.getByRole('alert')).toHaveCount(0)
  await expect(
    page.getByLabel(en.settings.appName, { exact: true }),
  ).toHaveValue(appName)

  let releaseWorkspaces!: () => void
  const workspaceGate = new Promise<void>((resolve) => {
    releaseWorkspaces = resolve
  })
  await page.route(
    '**/api/v1/admin/workspaces?*',
    async (route) => {
      await workspaceGate
      await route.continue()
    },
    { times: 1 },
  )
  await page
    .getByRole('link', { name: en.workspaces.title, exact: true })
    .click()
  await expect(
    page.getByText(en.workspaces.loading, { exact: true }),
  ).toBeVisible()
  releaseWorkspaces()
  await expect(
    page.getByText(en.workspaces.empty, { exact: true }),
  ).toBeVisible()
  expect(
    await (await page.request.get('/api/v1/admin/workspaces')).json(),
  ).toEqual({ items: [], total: 0 })
  await page.getByLabel(en.workspaces.searchHint, { exact: true }).fill('%_')
  await page.getByRole('button', { name: en.admin.search, exact: true }).click()
  await expect(
    page.getByText(en.workspaces.empty, { exact: true }),
  ).toBeVisible()
  await expect(page.locator('.admin-workspaces a')).toHaveCount(0)
  await page
    .getByRole('button', { name: en.common.languages.zhCN, exact: true })
    .click()
  await expect(
    page.getByRole('heading', { name: zh.workspaces.title, exact: true }),
  ).toBeVisible()
  await expect(
    page.getByText(zh.workspaces.notice, { exact: true }),
  ).toBeVisible()
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true)
  await page.screenshot({
    path: test.info().outputPath('workspaces-empty-zh.png'),
    fullPage: true,
  })
  await page
    .getByRole('button', { name: zh.common.languages.en, exact: true })
    .click()
  await page.route(
    '**/api/v1/admin/workspaces?*',
    (route) =>
      route.fulfill({
        status: 500,
        contentType: 'application/json',
        body: JSON.stringify({ error: { code: 'ERR_INTERNAL' } }),
      }),
    { times: 1 },
  )
  expectHTTPFailure(page, 'GET', '/api/v1/admin/workspaces', 500)
  await page.getByRole('button', { name: en.admin.search, exact: true }).click()
  await expect(page.getByRole('alert')).toContainText(en.errors.ERR_INTERNAL)
  await page.getByRole('button', { name: en.auth.retry, exact: true }).click()
  await expect(page.getByRole('alert')).toHaveCount(0)
  await expect(
    page.getByText(en.workspaces.empty, { exact: true }),
  ).toBeVisible()
  await page.getByRole('link', { name: en.admin.menu, exact: true }).click()
  return { previous, appName }
}
