import type { Browser, Page } from '@playwright/test'
import {
  test,
  expect,
  observeBrowser,
  expectHTTPFailure,
  expectBrowserClean,
} from './fixtures'
import type { Navigation, TableSchema } from '../src/api/types'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }
import { verifySharing } from './sharing'

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

async function tree(page: Page, locale = en) {
  if (page.viewportSize()!.width < 768) {
    const drawer = page.locator('.nav-drawer')
    if (!(await drawer.isVisible()))
      await page
        .getByRole('button', { name: locale.nav.open, exact: true })
        .click()
    await expect(drawer).toBeVisible()
  }
  return page.getByRole('navigation', { name: locale.nav.title, exact: true })
}

async function closeDrawer(page: Page, locale = en) {
  if (
    page.viewportSize()!.width < 768 &&
    (await page.locator('.nav-drawer').isVisible())
  )
    await page
      .getByRole('button', { name: locale.nav.close, exact: true })
      .click()
}

async function create(
  page: Page,
  kind: 'workspace' | 'book' | 'table',
  name: string,
  locale = en,
) {
  await page
    .locator('main')
    .getByRole('button', { name: locale.nav.create[kind], exact: true })
    .click()
  const dialog = page.getByRole('dialog', {
    name: locale.nav.dialogTitle,
    exact: true,
  })
  await expect(dialog).toBeVisible()
  await dialog
    .getByRole('textbox', { name: locale.nav.name, exact: true })
    .fill(name)
  await dialog
    .getByRole('button', { name: locale.admin.confirm, exact: true })
    .click()
  await expect(dialog).not.toBeVisible()
  await expect(page).toHaveURL(
    kind === 'workspace'
      ? /\/w\/[^/]+$/
      : kind === 'book'
        ? /\/w\/[^/]+\/b\/[^/]+$/
        : /\/t\/[^/]+\/v\/[^/]+$/,
  )
}

async function rename(
  page: Page,
  id: string,
  name: string,
  locale = en,
  expectSuccess = true,
) {
  const nav = await tree(page, locale)
  const actions = nav.locator(`[data-nav-id="${id}"] > .node-actions`)
  if (!((await actions.getAttribute('open')) !== null))
    await actions.locator('summary').click()
  await actions
    .getByRole('button', { name: locale.nav.rename, exact: true })
    .click()
  const dialog = page.getByRole('dialog', {
    name: locale.nav.dialogTitle,
    exact: true,
  })
  await dialog
    .getByRole('textbox', { name: locale.nav.name, exact: true })
    .fill(name)
  await dialog
    .getByRole('button', { name: locale.admin.confirm, exact: true })
    .click()
  if (expectSuccess) {
    await expect(dialog).not.toBeVisible()
    await closeDrawer(page, locale)
  }
}

async function noOverflow(page: Page) {
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true)
}

export async function verifyNavigation(
  admin: Page,
  alice: Page,
  browser: Browser,
  participants: { id: string; username: string }[],
  aliceUsername: string,
) {
  const h = headers()
  const aliceMe = await (await alice.request.get('/api/v1/auth/me')).json()
  expect(
    (
      await alice.request.patch('/api/v1/auth/me', {
        headers: h,
        data: { locale: 'en' },
      })
    ).status(),
  ).toBe(200)
  await alice.goto('/home')
  await expect(alice.locator('main')).toContainText(en.nav.empty)
  await create(alice, 'workspace', 'Workspace W')
  const wid = new URL(alice.url()).pathname.split('/')[2]!
  try {
    await expect(alice.locator('main')).toContainText(en.nav.emptyBooks)
    await create(alice, 'book', 'Book B')
    const bid = new URL(alice.url()).pathname.split('/')[4]!
    await expect(alice.locator('main')).toContainText(en.nav.emptyTables)
    await create(alice, 'table', 'Table T')
    await expect(alice.locator('.schema-summary')).toContainText('Name')
    await expect(
      alice.getByRole('navigation', { name: en.nav.views }),
    ).toContainText('Grid')
    const tid = new URL(alice.url()).pathname.split('/')[2]!
    const viewId = new URL(alice.url()).pathname.split('/')[4]!
    const schemaResponse = await alice.request.get(
      `/api/v1/tables/${tid}/schema`,
    )
    expect(schemaResponse.status()).toBe(200)
    const schema = (await schemaResponse.json()) as TableSchema
    expect(schema.myLevel).toBe('owner')
    expect(schema.fields.filter((field) => field.isSystem)).toHaveLength(5)
    expect(
      schema.fields.find((field) => field.id === schema.table.displayFieldId),
    ).toMatchObject({ name: 'Name', uiType: 'text' })
    expect(schema.views).toEqual([
      expect.objectContaining({
        id: viewId,
        name: 'Grid',
        type: 'grid',
        scope: 'shared',
      }),
    ])
    const nav = await tree(alice)
    await expect(nav.locator('[data-nav-id]')).toHaveCount(3)
    await expect(
      nav.locator(
        `[data-nav-id="${wid}"] [data-nav-id="${bid}"] [data-nav-id="${tid}"]`,
      ),
    ).toBeVisible()
    await expect(
      nav.getByRole('button', { name: en.nav.trashUnavailable }),
    ).toBeDisabled()
    if (alice.viewportSize()!.width < 768) {
      await alice.keyboard.press('Escape')
      await expect(alice.locator('.nav-drawer')).not.toBeVisible()
      await expect(
        alice.getByRole('button', { name: en.nav.open, exact: true }),
      ).toBeFocused()
    }
    await closeDrawer(alice)

    const siblingBookResponse = await alice.request.post(
      `/api/v1/workspaces/${wid}/books`,
      { headers: h, data: { name: 'Hidden sibling book' } },
    )
    expect(siblingBookResponse.status()).toBe(201)
    const siblingBook = (await siblingBookResponse.json()).book.id as string
    const siblingTableResponse = await alice.request.post(
      `/api/v1/books/${bid}/tables`,
      { headers: h, data: { name: 'Second table' } },
    )
    expect(siblingTableResponse.status()).toBe(201)
    const siblingTable = (await siblingTableResponse.json()).table.id as string
    await alice.reload()
    let ownerTree = await tree(alice)
    const tableActions = ownerTree.locator(
      `[data-nav-id="${tid}"] > .node-actions`,
    )
    await tableActions.locator('summary').click()
    await tableActions
      .getByRole('button', { name: en.nav.moveDown, exact: true })
      .click()
    await expect
      .poll(async () => {
        const value = (await (
          await alice.request.get('/api/v1/nav')
        ).json()) as Navigation
        return value.workspaces
          .find((w) => w.id === wid)!
          .books.find((b) => b.id === bid)!
          .tables.map((item) => item.id)
      })
      .toEqual([siblingTable, tid])
    if (alice.viewportSize()!.width >= 768) {
      await ownerTree
        .locator(`[data-nav-id="${tid}"] > .nav-row`)
        .dragTo(ownerTree.locator(`[data-nav-id="${siblingTable}"] > .nav-row`))
    } else {
      await tableActions
        .getByRole('button', { name: en.nav.moveUp, exact: true })
        .tap()
    }
    await expect
      .poll(async () => {
        const value = (await (
          await alice.request.get('/api/v1/nav')
        ).json()) as Navigation
        return value.workspaces
          .find((w) => w.id === wid)!
          .books.find((b) => b.id === bid)!
          .tables.map((item) => item.id)
      })
      .toEqual([tid, siblingTable])
    await closeDrawer(alice)
    await rename(alice, wid, '工作区 W')
    await rename(alice, bid, '工作簿 B')
    await alice
      .getByRole('button', { name: en.common.languages.zhCN, exact: true })
      .click()
    await rename(alice, tid, '数据表 T', zh)
    await expect(
      alice.getByRole('heading', { name: '数据表 T', exact: true }),
    ).toBeVisible()
    await expect(alice.locator('.breadcrumbs')).toContainText('工作区 W')
    await noOverflow(alice)
    await alice.screenshot({
      path: test.info().outputPath('navigation-zh.png'),
      fullPage: true,
    })
    await alice
      .getByRole('button', { name: zh.common.themes.dark, exact: true })
      .click()
    await expect(alice.locator('html')).toHaveAttribute('data-theme', 'dark')
    await noOverflow(alice)
    await alice
      .getByRole('button', { name: zh.common.themes.light, exact: true })
      .click()

    await admin.goto('/home')
    const adminTree = await tree(admin)
    await expect(adminTree.locator(`[data-nav-id="${wid}"]`)).toHaveCount(0)
    await closeDrawer(admin)
    const deniedTable = await admin.request.get(`/api/v1/tables/${tid}/schema`)
    expect(deniedTable.status()).toBe(404)
    expect(await deniedTable.json()).toMatchObject({
      error: { code: 'ERR_NOT_FOUND' },
    })
    await admin
      .getByRole('link', { name: en.workspaces.title, exact: true })
      .click()
    const inventory = admin.locator(`[data-workspace="${wid}"]`)
    await expect(inventory).toContainText('工作区 W')
    await expect(inventory).toContainText(aliceUsername)
    await expect(inventory.locator('a,button')).toHaveCount(0)

    const contexts = []
    const participantPages: Page[] = []
    const participantDiagnostics: Awaited<ReturnType<typeof observeBrowser>>[] =
      []
    try {
      for (const [index, participant] of participants.entries()) {
        const context = await browser.newContext({
          baseURL: process.env.PT_E2E_BASE_URL,
          viewport: alice.viewportSize(),
          isMobile: alice.viewportSize()!.width < 768,
          hasTouch: alice.viewportSize()!.width < 768,
        })
        contexts.push(context)
        const temporary = 'Temporary-password-123'
        const password = 'Navigation-password-123'
        expect(
          (
            await context.request.post('/api/v1/auth/login', {
              headers: h,
              data: { login: participant.username, password: temporary },
            })
          ).status(),
        ).toBe(200)
        expect(
          (
            await context.request.post('/api/v1/auth/password', {
              headers: h,
              data: { current: temporary, new: password },
            })
          ).status(),
        ).toBe(200)
        expect(
          (
            await context.request.post('/api/v1/auth/login', {
              headers: h,
              data: { login: participant.username, password },
            })
          ).status(),
        ).toBe(200)
        const grant = await alice.request.put('/api/v1/grants', {
          headers: h,
          data: {
            resourceType: index === 0 ? 'workspace' : 'table',
            resourceId: index === 0 ? wid : tid,
            principalType: 'user',
            principalId: participant.id,
            level: 'read',
          },
        })
        expect(grant.status()).toBe(201)
        const page = await context.newPage()
        participantPages.push(page)
        const diagnostics = await observeBrowser(page)
        participantDiagnostics.push(diagnostics)
        await page.goto(`/t/${tid}/v/${viewId}`)
        await page
          .getByRole('button', { name: en.common.languages.en, exact: true })
          .click()
        await expect(page.locator('.schema-summary')).toContainText(
          en.nav.level.read,
        )
        const visibleTree = await tree(page)
        await expect(visibleTree.locator('.node-actions')).toHaveCount(0)
        if (index === 1) {
          await expect(visibleTree.locator('[data-nav-id]')).toHaveCount(3)
          await expect(visibleTree.locator('.path-only')).toHaveCount(2)
          await expect(
            visibleTree.locator(
              `[data-nav-id="${siblingTable}"], [data-nav-id="${siblingBook}"]`,
            ),
          ).toHaveCount(0)
          await visibleTree
            .locator(`[data-nav-id="${wid}"] > .nav-row a`)
            .click()
          await expect(page.locator('main')).toContainText(en.nav.pathNotice)
          await expect(page.locator('main button')).toHaveCount(0)
          await page.goto('/home')
          await create(page, 'workspace', 'Carol W2')
          const carolWid = new URL(page.url()).pathname.split('/')[2]!
          const carolNav = (await (
            await context.request.get('/api/v1/nav')
          ).json()) as Navigation
          expect(
            carolNav.workspaces.find((w) => w.id === carolWid),
          ).toMatchObject({ level: 'owner', pathOnly: false })
          expect(
            (
              await context.request.delete(`/api/v1/workspaces/${carolWid}`, {
                headers: h,
              })
            ).status(),
          ).toBe(200)
        }
        await closeDrawer(page)
        expect(
          (
            await context.request.patch(`/api/v1/tables/${tid}`, {
              headers: h,
              data: { name: 'Denied' },
            })
          ).status(),
        ).toBe(403)
        await noOverflow(page)
        expect(diagnostics.errors).toEqual([])
        expect(diagnostics.pendingFailures).toEqual([])
      }
      await verifySharing(
        admin,
        alice,
        participantPages[0]!,
        participantPages[1]!,
        { wid, bid, tid, siblingTable, siblingBook },
        participants,
        aliceUsername,
      )
      participantPages.forEach((page, index) =>
        expectBrowserClean(page, participantDiagnostics[index]!),
      )
    } finally {
      for (const context of contexts) await context.close()
    }

    ownerTree = await tree(alice, zh)
    const actions = ownerTree.locator(`[data-nav-id="${tid}"] > .node-actions`)
    if ((await actions.getAttribute('open')) === null)
      await actions.locator('summary').click()
    await actions
      .getByRole('button', { name: zh.nav.delete, exact: true })
      .click()
    const dialog = alice.getByRole('dialog', { name: zh.nav.dialogTitle })
    await expect(dialog).toContainText(
      zh.nav.deleteNotice.replace('{name}', '数据表 T'),
    )
    await dialog
      .getByRole('button', { name: zh.admin.cancel, exact: true })
      .click()
    await actions
      .getByRole('button', { name: zh.nav.delete, exact: true })
      .click()
    await dialog
      .getByRole('button', { name: zh.admin.confirm, exact: true })
      .click()
    await expect(dialog).not.toBeVisible()
    await expect(ownerTree.locator(`[data-nav-id="${tid}"]`)).toHaveCount(0)
    await closeDrawer(alice, zh)
    await expect(
      alice.getByRole('heading', { name: zh.nav.removedTitle }),
    ).toBeVisible()
    expect(
      (await alice.request.get(`/api/v1/tables/${tid}/schema`)).status(),
    ).toBe(404)
    await alice
      .getByRole('button', { name: zh.common.languages.en, exact: true })
      .click()
    expectHTTPFailure(alice, 'PATCH', `/api/v1/tables/${siblingTable}`, 404)
    await alice.request.delete(`/api/v1/tables/${siblingTable}`, { headers: h })
    await rename(alice, siblingTable, 'Gone', en, false)
    await expect(alice.getByRole('dialog').getByRole('alert')).toContainText(
      en.errors.ERR_NOT_FOUND,
    )
    await alice
      .getByRole('dialog')
      .getByRole('button', { name: en.admin.cancel, exact: true })
      .click()
    await closeDrawer(alice)
  } finally {
    expect(
      (
        await alice.request.delete(`/api/v1/workspaces/${wid}`, { headers: h })
      ).status(),
    ).toBe(200)
    expect(
      (
        await alice.request.patch('/api/v1/auth/me', {
          headers: h,
          data: { locale: aliceMe.locale },
        })
      ).status(),
    ).toBe(200)
    await admin.goto('/admin/users')
    await admin
      .getByLabel(en.admin.searchHint, { exact: true })
      .fill(aliceUsername)
    await admin
      .getByRole('button', { name: en.admin.search, exact: true })
      .click()
  }
}
