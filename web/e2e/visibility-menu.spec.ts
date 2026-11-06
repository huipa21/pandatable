import type { Page } from '@playwright/test'
import { expect, paceLogin, test } from './fixtures'
import { setLocale, setTheme } from './app-shell'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }
import type { TableSchema } from '../src/api/types'

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

async function signIn(page: Page) {
  await paceLogin(page)
  await page.goto('/login')
  if ((await page.locator('html').getAttribute('lang')) === 'zh-CN')
    await setLocale(page, 'en')
  await page.getByLabel(en.auth.login, { exact: true }).fill('first-admin')
  await page
    .getByLabel(en.auth.password, { exact: true })
    .fill('Setup-password-123')
  await page.getByRole('button', { name: en.auth.signIn, exact: true }).click()
  await expect(page).toHaveURL(/\/(?:home|w\/[^/]+|change-password)$/)
}

test('Visibility menu hosts table sharing on desktop Grid, mobile and non-Grid views', async ({
  page,
  browser,
}, testInfo) => {
  test.skip(
    testInfo.project.name.toLowerCase().includes('mobile'),
    'The desktop project covers both widths explicitly.',
  )
  test.setTimeout(180_000)
  await page.setViewportSize({ width: 1280, height: 900 })
  await signIn(page)
  const stamp = Date.now()
  const post = async (path: string, data: unknown) => {
    const response = await page.request.post(path, { headers: headers(), data })
    expect(response.status()).toBe(201)
    return response.json()
  }
  const { workspace } = (await post('/api/v1/workspaces', {
    name: `Visibility ${stamp}`,
  })) as { workspace: { id: string } }
  const { book } = (await post(`/api/v1/workspaces/${workspace.id}/books`, {
    name: `Visibility book ${stamp}`,
  })) as { book: { id: string } }
  const schema = (await post(`/api/v1/books/${book.id}/tables`, {
    name: `Visibility table ${stamp}`,
  })) as TableSchema
  const tableId = schema.table.id
  const gridPath = `/t/${tableId}/v/${schema.views[0]!.id}`
  const formView = (await post(`/api/v1/tables/${tableId}/views`, {
    name: 'Intake',
    type: 'form',
    scope: 'shared',
  })) as { view: { id: string } }

  await page.goto(gridPath)
  const main = page.locator('main')
  const toolbar = main.locator('.grid-toolbar')
  const visibility = toolbar.locator('.visibility-menu')
  await expect(visibility).toBeVisible()
  const order = await toolbar.evaluate((element) =>
    [...element.children].map((child) => child.className.split(' ')[0]),
  )
  const highlight = order.indexOf('grid-highlights')
  expect(highlight).toBeGreaterThanOrEqual(0)
  expect(order[highlight + 1]).toBe('visibility-menu')
  expect(order[highlight + 2]).toBe('grid-row-height')
  const trigger = visibility.getByLabel(en.grid.visibilityMenu.label, {
    exact: true,
  })
  await expect(trigger.locator('svg.app-icon')).toHaveCount(1)
  await expect(main.locator('.table-toolbar > button')).toHaveCount(0)
  await expect(main.locator('.table-toolbar .visibility-menu')).toHaveCount(0)

  const share = visibility.getByRole('button', {
    name: en.sharing.open,
    exact: true,
  })
  await trigger.focus()
  await page.keyboard.press('Enter')
  await expect(share).toBeVisible()
  const box = (await visibility
    .locator('.visibility-menu-panel')
    .boundingBox())!
  expect(box.x + box.width).toBeLessThanOrEqual(1280)
  await page.keyboard.press('Escape')
  await expect(share).toBeHidden()
  await expect(trigger).toBeFocused()

  await trigger.click()
  await expect(share).toBeVisible()
  await main.locator('.data-grid').click({ position: { x: 600, y: 400 } })
  await expect(share).toBeHidden()

  await trigger.click()
  await share.click()
  const dialog = page.getByRole('dialog', { name: en.sharing.title })
  await expect(dialog).toBeVisible()
  await expect(share).toBeHidden()
  await expect(
    dialog.getByRole('heading', { name: en.sharing.direct, exact: true }),
  ).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(dialog).toBeHidden()
  await expect(trigger).toBeFocused()

  await setLocale(page, 'zh-CN')
  await setTheme(page, 'dark')
  const zhTrigger = visibility.getByLabel(zh.grid.visibilityMenu.label, {
    exact: true,
  })
  await zhTrigger.click()
  await expect(
    visibility.getByRole('button', { name: zh.sharing.open, exact: true }),
  ).toBeVisible()
  await page.keyboard.press('Escape')
  await setTheme(page, 'light')
  await setLocale(page, 'en')

  // Narrow screens and non-Grid views keep the menu in the table utility area.
  for (const [path, width] of [
    [gridPath, 390],
    [`/t/${tableId}/v/${formView.view.id}`, 1280],
  ] as const) {
    await page.setViewportSize({ width, height: 844 })
    await page.goto(path)
    const utility = main.locator('.table-toolbar .visibility-menu')
    await expect(utility).toBeVisible()
    await expect(main.locator('.grid-toolbar .visibility-menu')).toHaveCount(0)
    await utility.locator('summary').click()
    const panel = utility.locator('.visibility-menu-panel')
    const bounds = (await panel.boundingBox())!
    expect(bounds.x).toBeGreaterThanOrEqual(0)
    expect(bounds.x + bounds.width).toBeLessThanOrEqual(width)
    await utility.getByRole('button', { name: en.sharing.open }).click()
    await expect(dialog).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(dialog).toBeHidden()
  }

  // Write access and admin status without target ownership do not grant Share.
  await page.setViewportSize({ width: 1280, height: 900 })
  const userResponse = await page.request.post('/api/v1/admin/users', {
    headers: headers(),
    data: {
      username: `vis-${stamp}`,
      displayName: `vis-${stamp}`,
      role: 'admin',
      password: 'Temporary-password-123',
    },
  })
  expect(userResponse.status()).toBe(201)
  const user = (await userResponse.json()).user as { id: string }
  const grant = await page.request.put('/api/v1/grants', {
    headers: headers(),
    data: {
      resourceType: 'table',
      resourceId: tableId,
      principalType: 'user',
      principalId: user.id,
      level: 'write',
    },
  })
  expect(grant.status()).toBe(201)
  const context = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL,
    locale: 'en-US',
    viewport: { width: 1280, height: 900 },
  })
  try {
    for (const [password, next] of [
      ['Temporary-password-123', 'Visibility-password-123'],
      ['Visibility-password-123', null],
    ] as const) {
      await paceLogin()
      expect(
        (
          await context.request.post('/api/v1/auth/login', {
            headers: headers(),
            data: { login: `vis-${stamp}`, password },
          })
        ).status(),
      ).toBe(200)
      if (next)
        expect(
          (
            await context.request.post('/api/v1/auth/password', {
              headers: headers(),
              data: { current: password, new: next },
            })
          ).status(),
        ).toBe(200)
    }
    const writer = await context.newPage()
    await writer.goto(gridPath)
    await expect(writer.locator('main .grid-toolbar')).toBeVisible()
    await expect(writer.locator('main .visibility-menu')).toHaveCount(0)
    await expect(
      writer
        .locator('main')
        .getByRole('button', { name: en.sharing.open, exact: true }),
    ).toHaveCount(0)
  } finally {
    await context.close()
  }
})
