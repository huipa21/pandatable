import type { Page } from '@playwright/test'
import { writeFile } from 'node:fs/promises'
import { test, expect, expectHTTPFailure } from './fixtures'
import type { SystemSnapshot } from '../src/api/types'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }

export async function verifyDashboard(page: Page) {
  let release: (() => void) | undefined
  const gate = new Promise<void>((resolve) => {
    release = resolve
  })
  await page.route(
    '**/api/v1/admin/system',
    async (route) => {
      await gate
      await route.continue()
    },
    { times: 1 },
  )
  const initialResponse = page.waitForResponse(
    (r) =>
      new URL(r.url()).pathname === '/api/v1/admin/system' &&
      r.status() === 200,
  )
  await page
    .getByRole('link', { name: en.dashboard.title, exact: true })
    .click()
  await expect(
    page.getByText(en.dashboard.loading, { exact: true }),
  ).toBeVisible()
  release!()
  const first = (await (await initialResponse).json()) as SystemSnapshot
  expect(first.memory.value!.totalBytes).toBeGreaterThan(0)
  expect(first.memory.value!.usedBytes).toBeGreaterThan(0)
  expect(first.database.value!.sizeBytes).toBeGreaterThan(0)
  expect(first.database.value!.connections).toBeGreaterThan(0)
  expect(first.database.value!.sharedBuffersBytes).toBe(768 * 1024 * 1024)
  expect(first.disk.value!.freeBytes).toBeGreaterThan(0)
  expect(first.filesBytes.value).toBe(4096)
  expect(first.backupsBytes.value).toBe(2048)
  expect(first.appRssBytes.value).toBeGreaterThan(0)
  expect(first.container.status).toBe('ready')
  expect(first.container.value!.currentBytes).toBeGreaterThan(0)
  expect(first.container.value!.limitBytes).toBe(512 * 1024 * 1024)
  expect(first.versions.postgres.value).toMatch(/^18\.6/)
  expect(first.versions.migration.value).toBe(2)
  expect(first.versions.go).toBe('go1.27.0')
  expect(first.uptimeSeconds).toBeGreaterThan(0)
  expect(JSON.stringify(first)).not.toMatch(
    /postgres:\/\/|\/data|password|SETUP_TOKEN|\/sys\/|\/proc\//,
  )
  await expect(page.locator('[data-metric="files"] dd')).toHaveText('4 KiB')
  await expect(page.locator('[data-metric="backups"] dd')).toHaveText('2 KiB')
  await expect(page.locator('[data-metric="memoryTotal"]')).toHaveAttribute(
    'data-available',
    'true',
  )
  await expect(page.locator('[data-metric="dbSize"]')).toHaveAttribute(
    'data-available',
    'true',
  )
  const firstAt = first.collectedAt
  const refreshed = page.waitForResponse(
    (r) =>
      new URL(r.url()).pathname === '/api/v1/admin/system' &&
      r.status() === 200,
    { timeout: 10_000 },
  )
  const refreshMilliseconds = await page.evaluate(
    (previous) =>
      new Promise<number>((resolve, reject) => {
        const element = document.querySelector('[data-collected-at]')!
        const started = performance.now()
        const observer = new MutationObserver(() => {
          if (element.getAttribute('data-collected-at') !== previous) {
            clearTimeout(deadline)
            observer.disconnect()
            resolve(performance.now() - started)
          }
        })
        const deadline = setTimeout(() => {
          observer.disconnect()
          reject(
            new Error(
              'Dashboard did not render a fresh snapshot within ten seconds',
            ),
          )
        }, 10_000)
        observer.observe(element, {
          attributes: true,
          attributeFilter: ['data-collected-at'],
        })
      }),
    firstAt,
  )
  expect(refreshMilliseconds).toBeLessThanOrEqual(10_000)
  const second = (await (await refreshed).json()) as SystemSnapshot
  expect(second.collectedAt).not.toBe(firstAt)
  expect(second.cpuPercent.status).toBe('ready')
  expect(second.cpuPercent.value).toBeGreaterThan(0)
  expect(second.cpuPercent.value).toBeLessThanOrEqual(100)
  expect(second.filesBytes.collectedAt).toBe(first.filesBytes.collectedAt)
  expect(second.backupsBytes.collectedAt).toBe(first.backupsBytes.collectedAt)
  await page.screenshot({
    path: test.info().outputPath('dashboard-en.png'),
    fullPage: true,
  })
  await page
    .getByRole('button', { name: en.common.languages.zhCN, exact: true })
    .click()
  await expect(
    page.getByRole('heading', { name: zh.dashboard.title, exact: true }),
  ).toBeVisible()
  await expect(page.locator('[data-metric="memoryUsed"] dt')).toHaveText(
    zh.dashboard.used,
  )
  await expect(page.locator('[data-metric="dbSize"] dt')).toHaveText(
    zh.dashboard.dbSize,
  )
  await expect(page.locator('[data-metric="files"] dt')).toHaveText(
    zh.dashboard.files,
  )
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true)
  await page.screenshot({
    path: test.info().outputPath('dashboard-zh.png'),
    fullPage: true,
  })
  expectHTTPFailure(page, 'GET', '/api/v1/admin/system', 503)
  await page.route(
    '**/api/v1/admin/system',
    (route) =>
      route.fulfill({
        status: 503,
        contentType: 'application/json',
        body: JSON.stringify({ error: { code: 'ERR_SYSTEM_UNAVAILABLE' } }),
      }),
    { times: 1 },
  )
  await page
    .getByRole('button', { name: zh.dashboard.refresh, exact: true })
    .click()
  await expect(page.getByRole('alert')).toContainText(
    zh.errors.ERR_SYSTEM_UNAVAILABLE,
  )
  await expect(page.getByRole('alert')).toContainText(zh.dashboard.stale)
  await expect(page.locator('[data-metric="files"] dd')).toHaveText('4 KiB')
  await page.getByRole('button', { name: zh.auth.retry, exact: true }).click()
  await expect(page.getByRole('alert')).toHaveCount(0)
  await page
    .getByRole('button', { name: zh.common.languages.en, exact: true })
    .click()
  const observations = test.info().outputPath('dashboard-observations.json')
  await writeFile(
    observations,
    JSON.stringify({ first, second, refreshMilliseconds }, null, 2),
  )
  await test.info().attach('dashboard-observations', {
    path: observations,
    contentType: 'application/json',
  })
  await page.getByRole('link', { name: en.admin.menu, exact: true }).click()
}
