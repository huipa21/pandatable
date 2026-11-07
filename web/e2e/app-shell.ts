import type { Locator, Page } from '@playwright/test'
import { expect } from './fixtures'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }

const exactly = (labels: string[]) =>
  new RegExp(
    `^(${labels.map((label) => label.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('|')})$`,
  )

// Action menus stay open after their dialogs so focus can return; an open
// dropdown may cover the next row's trigger, so close it first.
export async function openNodeActions(actions: Locator) {
  if ((await actions.getAttribute('open')) !== null) return
  const others = actions.page().locator('details.node-actions[open] > summary')
  while ((await others.count()) > 0) await others.first().click()
  await actions.locator('summary').click()
  await expect(actions).toHaveAttribute('open', '')
}

export const themeToggleName = exactly([
  en.common.useDark,
  en.common.useLight,
  zh.common.useDark,
  zh.common.useLight,
])

/** Selects a locale from the header language menu. */
export async function setLocale(page: Page, locale: 'en' | 'zh-CN') {
  const toggle = page.locator('.language-menu-trigger')
  await expect(toggle).toBeVisible()
  await toggle.click()
  const option = page
    .locator('.language-menu-item')
    .filter({ has: page.locator(`.language-menu-native[lang="${locale}"]`) })
  await expect(option).toBeVisible()
  if ((await option.getAttribute('aria-checked')) === 'true') {
    await page.keyboard.press('Escape')
  } else {
    await option.click()
  }
  await expect(page.locator('html')).toHaveAttribute('lang', locale)
  await expect(toggle).toHaveAttribute('aria-expanded', 'false')
}

/** Clicks the header theme toggle only when the effective theme differs. */
export async function setTheme(page: Page, theme: 'light' | 'dark') {
  const html = page.locator('html')
  const toggle = page.getByRole('button', { name: themeToggleName })
  await expect(toggle).toBeVisible()
  if ((await html.getAttribute('data-theme')) !== theme) await toggle.click()
  await expect(html).toHaveAttribute('data-theme', theme)
}

/** Opens the avatar menu (sidebar footer on desktop, header on mobile). */
export async function openAccountMenu(page: Page) {
  const button = page.locator('.avatar-button')
  await expect(button).toBeVisible()
  if ((await button.getAttribute('aria-expanded')) !== 'true')
    await button.click()
  const panel = page.locator('.user-menu-panel')
  await expect(panel).toBeVisible()
  return panel
}

export async function accountLink(page: Page, name: string) {
  const panel = await openAccountMenu(page)
  return panel.getByRole('link', { name, exact: true })
}

/** Returns an administration link from the avatar menu's admin submenu. */
export async function adminLink(page: Page, name: string) {
  const panel = await openAccountMenu(page)
  const submenu = panel.locator('details.user-submenu')
  await expect(submenu).toBeVisible()
  if ((await submenu.getAttribute('open')) === null)
    await submenu.locator('summary').click()
  return submenu.getByRole('link', { name, exact: true })
}

export async function signOut(page: Page) {
  const panel = await openAccountMenu(page)
  await panel
    .getByRole('button', {
      name: exactly([en.auth.signOut, zh.auth.signOut]),
    })
    .click()
}
