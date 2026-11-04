import type { Page } from '@playwright/test'
import { expect } from './fixtures'
import en from '../src/i18n/en.json' with { type: 'json' }

export async function openFieldPanel(page: Page) {
  const panel = page.getByRole('region', {
    name: en.field.panel.title,
    exact: true,
  })
  if ((page.viewportSize()?.width ?? 0) < 768) {
    if (!(await panel.isVisible()))
      await page
        .getByRole('button', { name: en.mobileRows.options, exact: true })
        .click()
  } else {
    const trigger = page.locator('.grid-fields-trigger')
    await expect(trigger).toBeVisible()
    if ((await trigger.getAttribute('aria-expanded')) !== 'true')
      await trigger.click()
  }
  await expect(panel).toBeVisible()
  return panel
}

export async function openFieldTypeActions(page: Page) {
  const panel = await openFieldPanel(page)
  await panel
    .getByRole('button', { name: en.field.panel.add, exact: true })
    .click()
  return page.getByRole('dialog', {
    name: en.field.panel.createTitle,
    exact: true,
  })
}

export async function openTableOptions(page: Page) {
  const options = page.locator('.table-management')
  await expect(options).toBeVisible()
  if ((await options.getAttribute('open')) === null)
    await options.locator(':scope > summary').click()
  return options
}

export async function openGridMore(page: Page) {
  const more = page.locator('.grid-more')
  await expect(more).toBeVisible()
  if ((await more.getAttribute('open')) === null)
    await more.locator(':scope > summary').click()
  return more
}
