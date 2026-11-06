import type { Page } from '@playwright/test'
import { allowHTTPFailure, expect } from './fixtures'
import en from '../src/i18n/en.json' with { type: 'json' }

export function observeViewQuerySchemaConflicts(page: Page, viewId: string) {
  const path = `/api/v1/views/${viewId}/query`
  const conflicts: Promise<string>[] = []
  allowHTTPFailure(page, 'POST', path, 409)
  page.on('response', (response) => {
    if (
      response.request().method() === 'POST' &&
      new URL(response.url()).pathname === path &&
      response.status() === 409
    )
      conflicts.push(
        response
          .json()
          .then((body: { error: { code: string } }) => body.error.code),
      )
  })
  return async () => {
    const observed = [...conflicts]
    expect(await Promise.all(observed)).toEqual(
      observed.map(() => 'ERR_SCHEMA_CHANGED'),
    )
  }
}

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

export async function openExportMenu(page: Page) {
  const menu = page.locator('.view-export')
  await expect(menu).toBeVisible()
  if ((await menu.getAttribute('open')) === null)
    await menu.locator(':scope > summary').click()
  return menu
}

export async function sidebarTable(page: Page, tableId: string) {
  if (
    (page.viewportSize()?.width ?? 0) < 768 &&
    !(await page.locator('.nav-drawer').isVisible())
  )
    await page.getByRole('button', { name: en.nav.open, exact: true }).click()
  const nav = page.getByRole('navigation', { name: en.nav.title, exact: true })
  const table = nav.locator(`[data-nav-id="${tableId}"][data-kind="table"]`)
  await expect(table).toBeVisible()
  return table
}

export async function openCreateView(
  page: Page,
  tableId: string,
  interaction: 'pointer' | 'keyboard' | 'touch' = 'pointer',
) {
  const table = await sidebarTable(page, tableId)
  const actions = table.locator(':scope > .node-actions')
  const activate = async (target: ReturnType<Page['locator']>) => {
    if (interaction === 'touch') await target.tap()
    else if (interaction === 'keyboard') {
      await target.focus()
      await target.press('Enter')
    } else await target.click()
  }
  if ((await actions.getAttribute('open')) === null)
    await activate(actions.locator(':scope > summary'))
  await activate(
    actions.getByRole('button', { name: en.nav.createView, exact: true }),
  )
  const dialog = page.getByRole('dialog', {
    name: en.nav.createView,
    exact: true,
  })
  await expect(dialog).toBeVisible()
  await expect(
    dialog.getByLabel(en.nav.viewName, { exact: true }),
  ).toBeEnabled()
  return dialog
}

export async function sidebarView(page: Page, tableId: string, name: string) {
  const table = await sidebarTable(page, tableId)
  const toggle = table.locator(':scope > .nav-row .table-toggle')
  if ((await toggle.getAttribute('aria-expanded')) !== 'true')
    await toggle.click()
  const link = table
    .locator(':scope > .view-list')
    .getByRole('link', { name, exact: true })
  await expect(link).toBeVisible()
  return link
}

export async function openGridMore(page: Page) {
  const more = page.locator('.grid-more')
  await expect(more).toBeVisible()
  if ((await more.getAttribute('open')) === null)
    await more.locator(':scope > summary').click()
  return more
}
