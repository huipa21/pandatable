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
  expect(csp).toContain("script-src-attr 'none'")
  expect(csp).toContain("frame-src 'none'")
  expect(csp).toContain("form-action 'self'")
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

test('locale and theme toggles persist in the actual production shell', async ({
  page,
}) => {
  await page.goto('/login')
  const html = page.locator('html')
  await expect(html).toHaveAttribute('lang', 'en')
  // Before any choice the theme follows the operating system.
  await expect(html).toHaveAttribute('data-theme', 'light')
  await page.emulateMedia({ colorScheme: 'dark' })
  await expect(html).toHaveAttribute('data-theme', 'dark')
  await page.emulateMedia({ colorScheme: 'light' })
  await expect(html).toHaveAttribute('data-theme', 'light')

  const languageButton = page.getByRole('button', {
    name: en.common.language,
    exact: true,
  })
  await expect(languageButton).toHaveAttribute('aria-haspopup', 'menu')
  await expect(languageButton).toHaveAttribute('aria-expanded', 'false')
  await expect(languageButton).toHaveCSS('border-top-width', '0px')
  await languageButton.click()
  await expect(page.getByRole('menuitemradio')).toHaveCount(2)
  await expect(page.locator('.language-menu-native[lang="en"]')).toHaveText(
    en.common.nativeLanguages.en,
  )
  await expect(page.locator('.language-menu-native[lang="zh-CN"]')).toHaveText(
    en.common.nativeLanguages.zhCN,
  )
  await expect(
    page.getByRole('menuitemradio', {
      name: /English/,
    }),
  ).toHaveAttribute('aria-checked', 'true')
  await page.locator('.language-menu-native[lang="zh-CN"]').click()
  await expect(
    page.getByRole('heading', { name: zh.login.title }),
  ).toBeVisible()
  await expect(html).toHaveAttribute('lang', 'zh-CN')
  await page
    .getByRole('button', { name: zh.common.useDark, exact: true })
    .click()
  await expect(html).toHaveAttribute('data-theme', 'dark')
  await expect(page.locator('.welcome-card')).toHaveCSS(
    'background-color',
    'rgb(24, 24, 28)',
  )
  await expect(
    page.getByRole('button', { name: zh.common.useLight, exact: true }),
  ).toBeVisible()
  await page.reload()
  await expect(
    page.getByRole('heading', { name: zh.login.title }),
  ).toBeVisible()
  await expect(html).toHaveAttribute('data-theme', 'dark')
  await expect(
    page.getByRole('button', { name: zh.common.language, exact: true }),
  ).toBeVisible()
  await expect(
    page.getByRole('button', { name: zh.common.useLight, exact: true }),
  ).toBeVisible()

  await page
    .getByRole('button', { name: zh.common.language, exact: true })
    .click()
  await page.locator('.language-menu-native[lang="en"]').click()
  await expect(html).toHaveAttribute('lang', 'en')
  await page
    .getByRole('button', { name: en.common.useLight, exact: true })
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
  await expect(
    page.getByRole('button', { name: en.common.useDark, exact: true }),
  ).toBeVisible()

  const language = await page
    .locator('.preferences .language-menu-trigger')
    .boundingBox()
  const theme = await page
    .locator('.preferences')
    .getByRole('button', { name: en.common.useDark, exact: true })
    .boundingBox()
  expect(language).not.toBeNull()
  expect(theme).not.toBeNull()
  expect(
    Math.abs(
      language!.y + language!.height / 2 - (theme!.y + theme!.height / 2),
    ),
  ).toBeLessThan(2)
  expect(language!.x + language!.width).toBeLessThanOrEqual(theme!.x)
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true)

  await page.getByLabel(en.auth.login, { exact: true }).fill('first-admin')
  await page
    .getByLabel(en.auth.password, { exact: true })
    .fill('Setup-password-123')
  await page.getByRole('button', { name: en.auth.signIn, exact: true }).click()
  await expect(page).toHaveURL(/\/home$/)
  await page
    .getByRole('button', { name: en.common.language, exact: true })
    .click()
  await page.locator('.language-menu-native[lang="zh-CN"]').click()
  await expect(html).toHaveAttribute('lang', 'zh-CN')
  await page.reload()
  await expect(html).toHaveAttribute('lang', 'zh-CN')
  await expect(
    page.getByRole('button', { name: zh.common.language, exact: true }),
  ).toBeVisible()
})

test('language menu supports keyboard dismissal and fits a 390px viewport', async ({
  page,
}) => {
  await page.goto('/login')
  await page.setViewportSize({ width: 390, height: 844 })
  const trigger = page.getByRole('button', {
    name: en.common.language,
    exact: true,
  })

  await trigger.click()
  await page.keyboard.press('ArrowDown')
  await page.keyboard.press('Enter')
  await expect(page.locator('html')).toHaveAttribute('lang', 'zh-CN')
  await expect(
    page.getByRole('heading', { name: zh.login.title }),
  ).toBeVisible()

  const chineseTrigger = page.getByRole('button', {
    name: zh.common.language,
    exact: true,
  })
  await chineseTrigger.click()
  await page.keyboard.press('ArrowUp')
  await page.keyboard.press('Escape')
  await expect(chineseTrigger).toBeFocused()
  await expect(page.getByRole('menu')).toHaveCount(0)

  await chineseTrigger.click()
  const menu = page.getByRole('menu')
  const bounds = await menu.boundingBox()
  expect(bounds).not.toBeNull()
  expect(bounds!.x).toBeGreaterThanOrEqual(0)
  expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(390)
  expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(844)
  expect(
    await page.evaluate(
      () =>
        document.documentElement.scrollWidth <= window.innerWidth &&
        document.querySelector('.app-header')!.scrollWidth <=
          document.querySelector('.app-header')!.clientWidth,
    ),
  ).toBe(true)
})
