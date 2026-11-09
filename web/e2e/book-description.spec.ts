import type { Page } from '@playwright/test'
import en from '../src/i18n/en.json' with { type: 'json' }
import { expect, paceLogin, test } from './fixtures'

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

let workspaceId: string | undefined

test.afterEach(async ({ page }) => {
  if (!workspaceId) return
  const response = await page.request.delete(
    `/api/v1/workspaces/${workspaceId}`,
    { headers: headers() },
  )
  expect(response.status()).toBe(200)
  workspaceId = undefined
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

async function workspaceTree(page: Page, mobile: boolean) {
  if (mobile) {
    const drawer = page.locator('.nav-drawer')
    if (!(await drawer.isVisible()))
      await page.getByRole('button', { name: en.nav.open, exact: true }).click()
    await expect(drawer).toBeVisible()
    return drawer.locator('.nav-tree')
  }
  return page.locator('.sidebar-desktop .nav-tree')
}

async function createBookFromMenu(
  page: Page,
  mobile: boolean,
  name: string,
  description?: string,
  checkLimit = false,
) {
  await page.goto(`/w/${workspaceId}`)
  const tree = await workspaceTree(page, mobile)
  const menu = tree.locator(
    `[data-nav-id="${workspaceId}"] .workspace-nav-menu`,
  )
  await menu.locator('summary').click()
  await menu
    .getByRole('button', { name: en.nav.create.book, exact: true })
    .click()

  const dialog = page.locator('dialog.structure-dialog')
  await expect(dialog).toBeVisible()
  await dialog.getByLabel(en.nav.name, { exact: true }).fill(name)
  if (description !== undefined) {
    const field = dialog.getByLabel(en.wmb.bookDescription, { exact: true })
    if (checkLimit) {
      await field.fill('漢'.repeat(501))
      await expect(
        dialog.getByRole('button', { name: en.admin.confirm, exact: true }),
      ).toBeDisabled()
      await expect(dialog.locator('#workspace-description-counter')).toHaveText(
        '501/500 characters',
      )
    }
    await field.fill(description)
  }
  await dialog
    .getByRole('button', { name: en.admin.confirm, exact: true })
    .click()
  await expect(page).toHaveURL(new RegExp(`/w/${workspaceId}/b/[^/]+$`))
  const bookId = new URL(page.url()).pathname.split('/').at(-1)
  if (!bookId) throw new Error('Expected the new book URL to contain its ID')
  return bookId
}

test('WMB-06: create, display, edit and clear an optional book description', async ({
  page,
  isMobile,
}, testInfo) => {
  test.setTimeout(180_000)
  if (isMobile) await page.setViewportSize({ width: 390, height: 844 })
  await login(page)

  const created = await page.request.post('/api/v1/workspaces', {
    headers: headers(),
    data: { name: `WMB06 ${Date.now()}` },
  })
  expect(created.status()).toBe(201)
  workspaceId = ((await created.json()) as { workspace: { id: string } })
    .workspace.id

  const nameOnly = `Name only ${testInfo.project.name}`
  await createBookFromMenu(page, isMobile, nameOnly)

  const describedName = `Described ${testInfo.project.name}`
  const description = `${'第一行说明'.repeat(20)}\n第二行：项目细节`
  const bookId = await createBookFromMenu(
    page,
    isMobile,
    describedName,
    description,
    true,
  )

  await page.goto(`/w/${workspaceId}`)
  const card = page.locator(
    `.workbook-card[href="/w/${workspaceId}/b/${bookId}"]`,
  )
  const cardDescription = card.locator('.workbook-card-description')
  await expect(cardDescription).toHaveAttribute('title', description)
  await expect(cardDescription).toHaveText(description)
  await expect
    .poll(() =>
      cardDescription.evaluate((element) => {
        const style = getComputedStyle(element)
        return (
          style.whiteSpace === 'nowrap' &&
          style.textOverflow === 'ellipsis' &&
          element.scrollWidth > element.clientWidth
        )
      }),
    )
    .toBe(true)

  await card.click()
  const tree = await workspaceTree(page, isMobile)
  const bookMenu = tree.locator(`[data-nav-id="${bookId}"] .node-actions`)
  await bookMenu.locator('summary').click()
  await bookMenu
    .getByRole('button', { name: en.wmb.editBook, exact: true })
    .click()
  const dialog = page.locator('dialog.structure-dialog')
  await expect(dialog).toBeVisible()
  await expect(dialog.getByLabel(en.nav.name, { exact: true })).toHaveValue(
    describedName,
  )
  const descriptionField = dialog.getByLabel(en.wmb.bookDescription, {
    exact: true,
  })
  await expect(descriptionField).toHaveValue(description)
  await descriptionField.fill('')
  await dialog
    .getByRole('button', { name: en.admin.confirm, exact: true })
    .click()
  await expect(dialog).not.toBeVisible()

  await page.goto(`/w/${workspaceId}`)
  await expect(
    page.locator(
      `.workbook-card[href="/w/${workspaceId}/b/${bookId}"] .workbook-card-description`,
    ),
  ).toHaveCount(0)
})
