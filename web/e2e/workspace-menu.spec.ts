import type { Page } from '@playwright/test'
import type { WorkspacePreference } from '../src/api/types'
import en from '../src/i18n/en.json' with { type: 'json' }
import { expect, paceLogin, test } from './fixtures'

const preferencePath = '/api/v1/auth/workspace-preference'
const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})
const cleanupStates = new WeakMap<
  Page,
  { original: string; workspace?: string }
>()

test.afterEach(async ({ page }) => {
  const state = cleanupStates.get(page)
  if (!state) return

  const restored = await page.request.put(preferencePath, {
    headers: headers(),
    data: { workspaceId: state.original },
  })
  expect(restored.status()).toBe(200)
  if (state.workspace) {
    const deleted = await page.request.delete(
      `/api/v1/workspaces/${state.workspace}`,
      { headers: headers() },
    )
    expect(deleted.status()).toBe(200)
  }
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

async function tree(page: Page, mobile: boolean) {
  if (mobile) {
    const drawer = page.locator('.nav-drawer')
    if (!(await drawer.isVisible()))
      await page.getByRole('button', { name: en.nav.open, exact: true }).click()
    await expect(drawer).toBeVisible()
    return drawer.locator('.nav-tree')
  }
  return page.locator('.sidebar-desktop .nav-tree')
}

test('WMB-03: workspace menu actions are keyboard and mobile accessible', async ({
  page,
  isMobile,
}, testInfo) => {
  test.setTimeout(180_000)
  if (isMobile) await page.setViewportSize({ width: 390, height: 844 })
  await login(page)

  const original = (
    (await (
      await page.request.get(preferencePath)
    ).json()) as WorkspacePreference
  ).workspaceId
  expect(original).toBeTruthy()
  if (!original) throw new Error('Expected a preferred workspace after login')

  const state: { original: string; workspace?: string } = { original }
  cleanupStates.set(page, state)
  const prefix = `Workspace ${testInfo.project.name} ${Date.now()} `
  const name =
    `${prefix}${'Long '.repeat(Math.max(1, Math.floor((100 - prefix.length) / 5)))}`.slice(
      0,
      100,
    )
  const created = await page.request.post('/api/v1/workspaces', {
    headers: headers(),
    data: { name },
  })
  expect(created.status()).toBe(201)
  state.workspace = (
    (await created.json()) as { workspace: { id: string } }
  ).workspace.id

  await page.goto(`/w/${state.workspace}`)
  const sidebar = await tree(page, isMobile)
  const row = sidebar.locator(`[data-nav-id="${state.workspace}"]`)
  const workspaceLink = row.locator('.workspace-link')
  const actions = row.locator('.workspace-nav-menu')
  const trigger = actions.locator('summary')
  await expect(workspaceLink.locator('.app-icon')).toHaveCount(0)
  await expect(trigger).toHaveAttribute(
    'aria-label',
    en.wmb.workspaceActions.replace('{name}', name),
  )
  await expect(
    workspaceLink.evaluate((element) => {
      const style = getComputedStyle(element)
      return (
        style.textOverflow === 'ellipsis' &&
        element.scrollWidth > element.clientWidth
      )
    }),
  ).toBe(true)
  if (isMobile) {
    const target = await trigger.boundingBox()
    expect(target?.width).toBeGreaterThanOrEqual(44)
    expect(target?.height).toBeGreaterThanOrEqual(44)
  }
  expect(
    await workspaceLink.evaluate((element) =>
      element.nextElementSibling?.classList.contains('workspace-nav-menu'),
    ),
  ).toBe(true)

  await trigger.focus()
  await page.keyboard.press('Enter')
  const menu = actions.locator('.nav-actions')
  await expect(menu.getByRole('button')).toHaveText([
    en.nav.create.book,
    en.wmb.editWorkspace,
  ])
  await page.keyboard.press('Tab')
  await expect(menu.getByRole('button').nth(0)).toBeFocused()
  await page.keyboard.press('ArrowDown')
  await expect(menu.getByRole('button').nth(1)).toBeFocused()
  await page.keyboard.press('ArrowUp')
  await expect(menu.getByRole('button').nth(0)).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(trigger).toBeFocused()
  await expect(actions).not.toHaveAttribute('open', '')

  await trigger.click()
  await menu
    .getByRole('button', { name: en.nav.create.book, exact: true })
    .click()
  const dialog = page.locator('dialog.structure-dialog')
  await expect(dialog).toBeVisible()
  if (isMobile) await expect(page.locator('.nav-drawer')).toBeVisible()
  await dialog
    .getByLabel(en.nav.name, { exact: true })
    .fill('Menu-created book')
  await dialog
    .getByRole('button', { name: en.admin.confirm, exact: true })
    .click()
  await expect(page).toHaveURL(new RegExp(`/w/${state.workspace}/b/[^/]+$`))
  if (isMobile) await expect(page.locator('.nav-drawer')).not.toBeVisible()

  await page.goto(`/w/${state.workspace}`)
  const workspaceSidebar = await tree(page, isMobile)
  const editMenu = workspaceSidebar.locator(
    `[data-nav-id="${state.workspace}"] .workspace-nav-menu`,
  )
  await editMenu.locator('summary').click()
  await editMenu
    .getByRole('button', { name: en.wmb.editWorkspace, exact: true })
    .click()
  const editDialog = page.locator('dialog.structure-dialog')
  await expect(editDialog).toBeVisible()
  await expect(editDialog.getByLabel(en.nav.name, { exact: true })).toHaveValue(
    name,
  )
  if (isMobile) await expect(page.locator('.nav-drawer')).toBeVisible()
  await editDialog
    .getByRole('button', { name: en.admin.cancel, exact: true })
    .click()
})
