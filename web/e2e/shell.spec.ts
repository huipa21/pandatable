import { test, expect } from './fixtures'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }

test('real embedded shell, health, assets and history boundaries', async ({
  page,
  api,
}) => {
  const health = await api.get('/healthz')
  expect(health.status()).toBe(200)
  expect(await health.json()).toEqual({ status: 'ok' })
  const response = await page.goto('/')
  expect(response?.status()).toBe(200)
  await expect(page).toHaveURL(/\/login$/)
  await expect(
    page.getByRole('heading', { name: en.login.title }),
  ).toBeVisible()
  await expect(page.getByText(en.login.notice)).toBeVisible()
  const csp = response?.headers()['content-security-policy']
  expect(csp).toContain("script-src 'self'")
  expect(csp).not.toContain("'unsafe-eval'")
  expect(csp).not.toMatch(/script-src[^;]*'unsafe-inline'/)
  expect(response?.headers()['x-content-type-options']).toBe('nosniff')

  const scripts = await page
    .locator('script[src]')
    .evaluateAll((elements) =>
      elements.map((element) => element.getAttribute('src')),
    )
  expect(scripts.length).toBeGreaterThan(0)
  for (const src of scripts) {
    expect(src).toMatch(/^\/assets\/.*\.js$/)
    const asset = await api.get(src!)
    expect(asset.status()).toBe(200)
    expect(asset.headers()['content-type']).toMatch(/javascript/)
    expect(await asset.text()).not.toContain('<!doctype html>')
  }
  const index = await api.get('/')
  const deep = await api.get('/t/future/v/example')
  expect(deep.status()).toBe(200)
  expect(await deep.text()).toBe(await index.text())
  await page.goto('/t/future/v/example')
  await expect(page).toHaveURL(/\/login$/)
  await expect(
    page.getByRole('heading', { name: en.login.title }),
  ).toBeVisible()
  await page.goto('/not-yet-implemented')
  await expect(
    page.getByRole('heading', { name: en.nav.notFoundTitle }),
  ).toBeVisible()
  await page.reload()
  await expect(
    page.getByRole('heading', { name: en.nav.notFoundTitle }),
  ).toBeVisible()
  await page.getByRole('link', { name: en.common.backToLogin }).click()
  await expect(
    page.getByRole('heading', { name: en.login.title }),
  ).toBeVisible()

  for (const path of [
    '/assets/missing.js',
    '/assets/missing',
    '/api',
    '/api/v1/missing',
    '/healthz/missing',
    '/.env',
    '/missing.css',
  ]) {
    const missing = await api.get(path)
    expect(missing.status(), path).toBe(404)
    expect(missing.headers()['content-type']).toContain('application/json')
    expect(await missing.json()).toMatchObject({
      error: { code: 'ERR_NOT_FOUND' },
    })
  }
  const head = await api.head('/t/future/v/example')
  expect(head.status()).toBe(200)
  expect(await head.body()).toHaveLength(0)
  const post = await api.post('/t/future/v/example')
  expect(post.status()).toBe(405)
})

test('locale and all theme modes persist in the actual production shell', async ({
  page,
}) => {
  await page.goto('/login')
  const html = page.locator('html')
  await expect(html).toHaveAttribute('lang', 'en')
  await page
    .getByRole('button', { name: en.common.languages.zhCN, exact: true })
    .click()
  await expect(
    page.getByRole('heading', { name: zh.login.title }),
  ).toBeVisible()
  await expect(html).toHaveAttribute('lang', 'zh-CN')
  await page
    .getByRole('button', { name: zh.common.themes.dark, exact: true })
    .click()
  await expect(html).toHaveAttribute('data-theme', 'dark')
  await expect(page.locator('.welcome-card')).toHaveCSS(
    'background-color',
    'rgb(24, 24, 28)',
  )
  await page.reload()
  await expect(
    page.getByRole('heading', { name: zh.login.title }),
  ).toBeVisible()
  await expect(html).toHaveAttribute('data-theme', 'dark')
  await expect(
    page.getByRole('button', { name: zh.common.themes.dark, exact: true }),
  ).toHaveAttribute('aria-pressed', 'true')

  await page
    .getByRole('button', { name: zh.common.languages.en, exact: true })
    .click()
  await page
    .getByRole('button', { name: en.common.themes.light, exact: true })
    .click()
  await page.emulateMedia({ colorScheme: 'dark' })
  await expect(html).toHaveAttribute('data-theme', 'light')
  await expect(page.locator('.welcome-card')).toHaveCSS(
    'background-color',
    'rgb(255, 255, 255)',
  )
  await page.reload()
  await expect(html).toHaveAttribute('lang', 'en')
  await expect(html).toHaveAttribute('data-theme', 'light')
  await page
    .getByRole('button', { name: en.common.themes.system, exact: true })
    .click()
  await expect(html).toHaveAttribute('data-theme', 'dark')
  await page.reload()
  await expect(
    page.getByRole('button', { name: en.common.themes.system, exact: true }),
  ).toHaveAttribute('aria-pressed', 'true')
  await expect(html).toHaveAttribute('data-theme', 'dark')
  await page.emulateMedia({ colorScheme: 'light' })
  await expect(html).toHaveAttribute('data-theme', 'light')

  const controls = await page.locator('.preferences fieldset').all()
  expect(controls).toHaveLength(2)
  const language = await controls[0]!.boundingBox()
  const theme = await controls[1]!.boundingBox()
  expect(language).not.toBeNull()
  expect(theme).not.toBeNull()
  expect(Math.abs(language!.y - theme!.y)).toBeLessThan(2)
  expect(language!.x + language!.width).toBeLessThanOrEqual(theme!.x)
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true)
})
