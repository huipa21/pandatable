import { test, expect, paceLogin } from './fixtures'
import { setLocale, setTheme } from './app-shell'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }
import { verifyFields } from './fields'
import { create as createResource, verifyDataGrid } from './navigation'
import { observeViewQuerySchemaConflicts } from './table-controls'

test('T1.16/T1.17 production grid persists rows, clipboard edits, and mobile fallback', async ({
  page,
  isMobile,
}) => {
  test.setTimeout(150_000)
  if (!isMobile) await page.setViewportSize({ width: 1440, height: 900 })
  await paceLogin(page)
  await page.goto('/login')
  if ((await page.locator('html').getAttribute('lang')) === 'zh-CN')
    await setLocale(page, 'en')
  await page.getByLabel(en.auth.login, { exact: true }).fill('first-admin')
  await page
    .getByLabel(en.auth.password, { exact: true })
    .fill('Setup-password-123')
  await page.getByRole('button', { name: en.auth.signIn, exact: true }).click()
  await expect(page).toHaveURL(/\/w\/[^/]+$/)

  const suffix = Date.now()
  await createResource(page, 'workspace', `Grid workspace ${suffix}`)
  await createResource(page, 'book', `Grid book ${suffix}`)
  await createResource(page, 'table', `Grid table ${suffix}`)
  if (isMobile) {
    await expect(page.locator('revo-grid')).toHaveCount(0)
    await expect(
      page.getByRole('region', { name: en.mobileRows.title, exact: true }),
    ).toBeVisible()
    await setTheme(page, 'dark')
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
    await setLocale(page, 'zh-CN')
    await expect(
      page.getByRole('region', { name: zh.mobileRows.title, exact: true }),
    ).toBeVisible()
    await expect(page.locator('revo-grid')).toHaveCount(0)
    await setLocale(page, 'en')
    await setTheme(page, 'light')
    return
  }
  const parts = new URL(page.url()).pathname.split('/')
  const tableId = parts[2]
  const viewId = parts[4]
  if (!tableId || !viewId) throw new Error('Created table route is incomplete')

  // Field-driven view saves can race a grid query under snapshot isolation.
  const checkQueryConflicts = observeViewQuerySchemaConflicts(page, viewId)
  await verifyFields(page, tableId)
  await verifyDataGrid(page, tableId, viewId)
  await checkQueryConflicts()
})
