import { devices } from '@playwright/test'
import en from '../src/i18n/en.json' with { type: 'json' }
import { test, expect } from './fixtures'

test('AT-101: first-run setup requires the logged token and signs in the first admin', async ({
  page,
  browser,
  api,
}) => {
  const token = process.env.PT_E2E_SETUP_TOKEN
  const baseURL = process.env.PT_E2E_BASE_URL
  expect(token, 'owned app startup token').toBeTruthy()
  expect(baseURL, 'owned app URL').toBeTruthy()
  const initialStatus = await api.get('/api/v1/setup/status')
  expect(initialStatus.status()).toBe(200)
  expect(await initialStatus.json()).toEqual({ needed: true })
  const setupErrorStatuses: number[] = []
  page.on('response', (response) => {
    if (
      new URL(response.url()).pathname === '/api/v1/setup' &&
      [401, 410].includes(response.status())
    ) {
      setupErrorStatuses.push(response.status())
    }
  })

  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/')
  await expect(page).toHaveURL(/\/setup$/)
  await expect(
    page.getByRole('heading', { name: en.setup.title }),
  ).toBeVisible()
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true)

  const mobileContext = await browser.newContext({
    ...devices['Pixel 7'],
    baseURL,
  })
  const mobilePage = await mobileContext.newPage()
  const mobileErrors: string[] = []
  mobilePage.on('pageerror', (error) => mobileErrors.push(error.message))
  mobilePage.on('console', (message) => {
    if (['warning', 'error'].includes(message.type()))
      mobileErrors.push(message.text())
  })
  try {
    await mobilePage.goto(`${baseURL}/w/not-yet-created`)
    await expect(mobilePage).toHaveURL(/\/setup$/)
    await expect(
      mobilePage.getByRole('heading', { name: en.setup.title }),
    ).toBeVisible()
    expect(
      await mobilePage.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true)
    expect(mobileErrors).toEqual([])
  } finally {
    await mobileContext.close()
  }

  const fillForm = async (setupToken: string) => {
    await page.getByRole('textbox', { name: en.setup.token }).fill(setupToken)
    await page
      .getByRole('textbox', { name: en.setup.username })
      .fill('First-Admin')
    await page
      .getByRole('textbox', { name: en.setup.email })
      .fill('owner@example.test')
    await page
      .getByRole('textbox', { name: en.setup.displayName })
      .fill('First Administrator')
    await page.getByLabel(en.setup.password).fill('Setup-password-123')
  }
  await fillForm('wrong-setup-token')
  await page.getByRole('button', { name: en.setup.submit }).click()
  await expect(page.getByRole('alert')).toContainText(en.errors.ERR_SETUP_TOKEN)
  expect(await (await api.get('/api/v1/setup/status')).json()).toEqual({
    needed: true,
  })
  expect(setupErrorStatuses).toEqual([401])

  await fillForm(token!)
  await page.getByRole('button', { name: en.setup.submit }).click()
  await expect(
    page.getByRole('heading', { name: en.setup.successTitle }),
  ).toBeVisible()
  await expect(page.getByText('first-admin · owner@example.test')).toBeVisible()
  const me = await page.evaluate(async () => {
    const response = await fetch('/api/v1/auth/me', { cache: 'no-store' })
    return { status: response.status, user: await response.json() }
  })
  expect(me.status).toBe(200)
  expect(me.user).toMatchObject({
    username: 'first-admin',
    email: 'owner@example.test',
    role: 'admin',
  })

  const completedStatus = await page.evaluate(async () => {
    const response = await fetch('/api/v1/setup/status', { cache: 'no-store' })
    return { status: response.status, body: await response.json() }
  })
  expect(completedStatus).toEqual({ status: 200, body: { needed: false } })
  const completedPost = await page.evaluate(async (setupToken) => {
    const response = await fetch('/api/v1/setup', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-PT-CSRF': '1' },
      body: JSON.stringify({
        token: setupToken,
        username: 'first-admin',
        email: 'owner@example.test',
        displayName: 'First Administrator',
        password: 'Setup-password-123',
        locale: 'en',
      }),
    })
    return { status: response.status, body: await response.json() }
  }, token)
  expect(completedPost.status).toBe(410)
  expect(completedPost.body).toMatchObject({
    error: { code: 'ERR_SETUP_COMPLETE' },
  })
  expect(setupErrorStatuses).toEqual([401, 410])
  await page.goto('/setup')
  await expect(page.getByRole('status')).toContainText(en.setup.alreadyComplete)
})
