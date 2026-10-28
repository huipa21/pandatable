import type { Page } from '@playwright/test'
import { test, expect, expectHTTPFailure } from './fixtures'
import type {
  GrantLevel,
  GrantListing,
  PrincipalType,
  ResourceKind,
  TableSchema,
} from '../src/api/types'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})
const dialog = (page: Page, locale = en) =>
  page.getByRole('dialog', { name: locale.sharing.title, exact: true })
const direct = (page: Page, id: string) =>
  dialog(page).locator(`[data-principal-id="${id}"]`)
async function grants(
  page: Page,
  kind: ResourceKind,
  id: string,
): Promise<GrantListing> {
  const response = await page.request.get(
    `/api/v1/grants?resource=${kind}:${id}`,
  )
  expect(response.status()).toBe(200)
  return response.json()
}
async function open(page: Page, path: string, locale = en) {
  await page.goto(path)
  await page
    .locator('main')
    .getByRole('button', { name: locale.sharing.open, exact: true })
    .click()
  await expect(dialog(page, locale)).toBeVisible()
  await expect(
    dialog(page, locale).getByRole('heading', {
      name: locale.sharing.direct,
      exact: true,
    }),
  ).toBeVisible()
  return dialog(page, locale)
}
async function close(page: Page, locale = en) {
  await dialog(page, locale)
    .getByRole('button', { name: locale.sharing.close, exact: true })
    .click()
  await expect(dialog(page, locale)).not.toBeVisible()
}
async function reviewAdd(
  page: Page,
  name: string,
  id: string,
  level: GrantLevel,
  type: PrincipalType = 'user',
) {
  const modal = dialog(page)
  await modal
    .getByLabel(en.sharing.principalType, { exact: true })
    .selectOption(type)
  await modal.getByLabel(en.sharing.query, { exact: true }).fill(name)
  await modal
    .getByRole('button', { name: en.sharing.search, exact: true })
    .click()
  await modal
    .getByLabel(en.sharing.selectPrincipal, { exact: true })
    .selectOption(id)
  await modal.getByLabel(en.sharing.level, { exact: true }).selectOption(level)
  await modal
    .getByRole('button', { name: en.sharing.review, exact: true })
    .click()
  await expect(modal.locator('.share-review')).toBeVisible()
}
async function confirm(page: Page) {
  await dialog(page)
    .getByRole('button', { name: en.admin.confirm, exact: true })
    .click()
  await expect(dialog(page).locator('.share-review')).not.toBeVisible()
  await expect(dialog(page).getByRole('status')).toHaveCount(0)
}
async function add(
  page: Page,
  name: string,
  id: string,
  level: GrantLevel,
  type: PrincipalType = 'user',
) {
  await reviewAdd(page, name, id, level, type)
  await confirm(page)
  await expect(direct(page, id)).toContainText(en.nav.level[level])
}
async function remove(page: Page, id: string) {
  await direct(page, id)
    .getByRole('button', { name: en.sharing.remove, exact: true })
    .click()
  await confirm(page)
  await expect(direct(page, id)).toHaveCount(0)
}
async function schema(page: Page, id: string, level: GrantLevel) {
  const response = await page.request.get(`/api/v1/tables/${id}/schema`)
  expect(response.status()).toBe(200)
  expect(((await response.json()) as TableSchema).myLevel).toBe(level)
}
async function focus(page: Page) {
  await page.bringToFront()
  await page.evaluate(() => window.dispatchEvent(new Event('focus')))
}
async function noOverflow(page: Page) {
  expect(
    await page.evaluate(() => {
      const modal = document.querySelector<HTMLDialogElement>('.share-dialog')!
      return (
        document.documentElement.scrollWidth <= innerWidth &&
        modal.scrollWidth <= modal.clientWidth &&
        modal.getBoundingClientRect().height <= innerHeight
      )
    }),
  ).toBe(true)
}

export async function verifySharing(
  admin: Page,
  alice: Page,
  bob: Page,
  carol: Page,
  ids: {
    wid: string
    bid: string
    tid: string
    siblingTable: string
    siblingBook: string
  },
  participants: { id: string; username: string }[],
  aliceUsername: string,
) {
  const h = headers()
  const { wid, bid, tid, siblingTable, siblingBook } = ids
  const b = participants[0]!
  const c = participants[1]!
  const me = (await (await alice.request.get('/api/v1/auth/me')).json()) as {
    id: string
  }
  const workspacePath = `/w/${wid}`
  const bookPath = `/w/${wid}/b/${bid}`
  const tablePath = `/t/${tid}`
  const aliceAdminCalls: string[] = []
  const capture = (request: { url(): string }) => {
    if (new URL(request.url()).pathname.startsWith('/api/v1/admin/'))
      aliceAdminCalls.push(request.url())
  }
  alice.on('request', capture)
  let groupId: string | undefined
  let transientGroup: string | undefined
  try {
    await alice
      .getByRole('button', { name: zh.common.languages.en, exact: true })
      .click()
    // Native dialog stacks above the mobile drawer and returns keyboard focus.
    if (alice.viewportSize()!.width < 768)
      await alice
        .getByRole('button', { name: en.nav.open, exact: true })
        .click()
    const actions = alice.locator(`[data-nav-id="${wid}"] > .node-actions`)
    if ((await actions.getAttribute('open')) === null)
      await actions.locator('summary').click()
    const trigger = actions.getByRole('button', {
      name: en.sharing.open,
      exact: true,
    })
    if (alice.viewportSize()!.width < 768) await trigger.tap()
    else await trigger.click()
    await expect(dialog(alice)).toBeVisible()
    await alice.keyboard.press('Tab')
    await alice.keyboard.press('Shift+Tab')
    expect(
      await dialog(alice).evaluate((element) =>
        element.contains(document.activeElement),
      ),
    ).toBe(true)
    await noOverflow(alice)
    await alice.keyboard.press('Escape')
    await expect(dialog(alice)).not.toBeVisible()
    await expect(trigger).toBeFocused()
    if (alice.viewportSize()!.width < 768) await alice.keyboard.press('Escape')

    await open(alice, workspacePath)
    const own = (await grants(alice, 'workspace', wid)).items.find(
      (item) => item.principalId === me.id,
    )!
    await direct(alice, me.id)
      .getByRole('button', { name: en.sharing.remove, exact: true })
      .click()
    await dialog(alice)
      .getByRole('button', { name: en.admin.cancel, exact: true })
      .click()
    expect((await grants(alice, 'workspace', wid)).items).toContainEqual(
      expect.objectContaining({ id: own.id }),
    )
    await direct(alice, me.id)
      .getByRole('button', { name: en.sharing.remove, exact: true })
      .click()
    expectHTTPFailure(alice, 'DELETE', `/api/v1/grants/${own.id}`, 409)
    await dialog(alice)
      .getByRole('button', { name: en.admin.confirm, exact: true })
      .click()
    await expect(dialog(alice).getByRole('alert')).toContainText(
      en.errors.ERR_LAST_OWNER,
    )
    await dialog(alice)
      .getByRole('button', { name: en.admin.cancel, exact: true })
      .click()
    await direct(alice, me.id).getByRole('combobox').selectOption('write')
    await direct(alice, me.id)
      .getByRole('button', { name: en.sharing.change, exact: true })
      .click()
    expectHTTPFailure(alice, 'PUT', '/api/v1/grants', 409)
    await dialog(alice)
      .getByRole('button', { name: en.admin.confirm, exact: true })
      .click()
    await expect(dialog(alice).getByRole('alert')).toContainText(
      en.errors.ERR_LAST_OWNER,
    )
    await dialog(alice)
      .getByRole('button', { name: en.admin.cancel, exact: true })
      .click()
    // AT-204: a real picker replaces the existing workspace read grant.
    await add(alice, b.username, b.id, 'read')
    await close(alice)
    await schema(bob, tid, 'read')
    await focus(bob)
    await expect(bob.locator('.schema-summary')).toContainText(
      en.nav.level.read,
    )
    await expect(
      bob
        .locator('main')
        .getByRole('button', { name: en.sharing.open, exact: true }),
    ).toHaveCount(0)
    expect(
      (
        await bob.request.get(
          `/api/v1/directory/users?resource=workspace:${wid}&q=alice`,
        )
      ).status(),
    ).toBe(403)
    expect(
      (
        await admin.request.get(
          `/api/v1/directory/users?resource=workspace:${wid}&q=alice`,
        )
      ).status(),
    ).toBe(404)

    // AT-205/206: actual schema levels and structure denial, not row/editor proxies.
    await open(alice, bookPath)
    await add(alice, b.username, b.id, 'write')
    await close(alice)
    await schema(bob, tid, 'write')
    expect(
      (
        await bob.request.post(`/api/v1/books/${bid}/tables`, {
          headers: h,
          data: { name: 'Denied' },
        })
      ).status(),
    ).toBe(403)
    await focus(bob)
    await expect(bob.locator('.schema-summary')).toContainText(
      en.nav.level.write,
    )
    await open(alice, bookPath)
    await remove(alice, b.id)
    await close(alice)
    await open(alice, tablePath)
    await add(alice, b.username, b.id, 'write')
    await close(alice)
    await schema(bob, tid, 'write')
    await schema(bob, siblingTable, 'read')
    await open(alice, workspacePath)
    await add(alice, b.username, b.id, 'write')
    await close(alice)
    await open(alice, tablePath)
    await direct(alice, b.id).getByRole('combobox').selectOption('read')
    await expect(direct(alice, b.id)).toContainText(
      en.sharing.inheritedHigher.replace('{level}', en.nav.level.write),
    )
    await direct(alice, b.id)
      .getByRole('button', { name: en.sharing.change, exact: true })
      .click()
    await confirm(alice)
    expect(
      (await grants(alice, 'table', tid)).items.find(
        (item) => item.principalId === b.id,
      )?.inheritedLevel,
    ).toBe('write')
    await schema(bob, tid, 'write')
    await remove(alice, b.id)
    await schema(bob, tid, 'write')
    // AT-208 already verified table-only read; now remove it before the group case.
    await remove(alice, c.id)
    await close(alice)
    expect(
      (await carol.request.get(`/api/v1/tables/${tid}/schema`)).status(),
    ).toBe(404)

    const groupResponse = await admin.request.post('/api/v1/admin/groups', {
      headers: h,
      data: { name: `sharing-team-${b.username}` },
    })
    expect(groupResponse.status()).toBe(201)
    const group = (await groupResponse.json()).group as {
      id: string
      name: string
    }
    groupId = group.id
    expect(
      (
        await admin.request.post(`/api/v1/admin/groups/${group.id}/members`, {
          headers: h,
          data: { userId: c.id },
        })
      ).status(),
    ).toBe(201)
    await open(alice, bookPath)
    await add(alice, group.name, group.id, 'write', 'group')
    await close(alice)
    await schema(carol, tid, 'write')
    await carol.goto(tablePath)
    await expect(carol.locator('.schema-summary')).toContainText(
      en.nav.level.write,
    )
    expectHTTPFailure(carol, 'POST', '/api/v1/views/*/query', 404)
    expectHTTPFailure(carol, 'GET', '/api/v1/grants', 404)
    const revokedAt = Date.now()
    expect(
      (
        await admin.request.delete(
          `/api/v1/admin/groups/${group.id}/members/${c.id}`,
          { headers: h },
        )
      ).status(),
    ).toBe(200)
    expect(
      (await carol.request.get(`/api/v1/tables/${tid}/schema`)).status(),
    ).toBe(404)
    expect(Date.now() - revokedAt).toBeLessThan(5000)
    await focus(carol)
    await expect(
      carol.getByRole('heading', { name: en.nav.removedTitle, exact: true }),
    ).toBeVisible()
    await open(alice, bookPath)
    await remove(alice, group.id)
    await close(alice)

    // AT-209: book ownership differs from parent-structure ownership.
    await open(alice, bookPath)
    await add(alice, b.username, b.id, 'owner')
    await close(alice)
    await open(alice, workspacePath)
    await remove(alice, b.id)
    await close(alice)
    await bob.goto(bookPath)
    await expect(
      bob
        .locator('main')
        .getByRole('button', { name: en.sharing.open, exact: true }),
    ).toBeVisible()
    expect(
      (
        await bob.request.get(`/api/v1/grants?resource=workspace:${wid}`)
      ).status(),
    ).toBe(403)
    expect(
      (
        await bob.request.get(`/api/v1/grants?resource=book:${siblingBook}`)
      ).status(),
    ).toBe(404)
    expect(
      (
        await bob.request.patch(`/api/v1/books/${bid}`, {
          headers: h,
          data: { name: 'Denied' },
        })
      ).status(),
    ).toBe(403)
    await open(bob, bookPath)
    await add(bob, c.username, c.id, 'write')
    await close(bob)
    await open(bob, tablePath)
    await add(bob, c.username, c.id, 'owner')
    await close(bob)
    await open(bob, bookPath)
    await remove(bob, c.id)
    await close(bob)
    await carol.goto(tablePath)
    await expect(carol.locator('.schema-summary')).toContainText(
      en.nav.level.owner,
    )
    expect(
      (
        await carol.request.patch(`/api/v1/tables/${tid}`, {
          headers: h,
          data: { name: 'Denied' },
        })
      ).status(),
    ).toBe(403)
    await open(carol, tablePath)
    await add(carol, b.username, b.id, 'read')
    await direct(carol, c.id)
      .getByRole('button', { name: en.sharing.remove, exact: true })
      .click()
    expectHTTPFailure(carol, 'GET', '/api/v1/grants', 404)
    await dialog(carol)
      .getByRole('button', { name: en.admin.confirm, exact: true })
      .click()
    await expect(dialog(carol).getByRole('alert')).toContainText(
      en.errors.ERR_NOT_FOUND,
    )
    await close(carol)
    await expect(
      carol.getByRole('heading', { name: en.nav.removedTitle, exact: true }),
    ).toBeVisible()

    // AT-210/211: another active direct user owner permits Alice's downgrade.
    await open(alice, workspacePath)
    await add(alice, b.username, b.id, 'owner')
    await close(alice)
    await open(bob, workspacePath)
    await add(bob, c.username, c.id, 'owner')
    await close(bob)
    await open(alice, workspacePath)
    await direct(alice, me.id).getByRole('combobox').selectOption('read')
    await direct(alice, me.id)
      .getByRole('button', { name: en.sharing.change, exact: true })
      .click()
    expectHTTPFailure(alice, 'GET', '/api/v1/grants', 403)
    await dialog(alice)
      .getByRole('button', { name: en.admin.confirm, exact: true })
      .click()
    await expect(dialog(alice).getByRole('alert')).toContainText(
      en.errors.ERR_PERMISSION_DENIED,
    )
    await close(alice)
    await expect(
      alice
        .locator('main')
        .getByRole('button', { name: en.sharing.open, exact: true }),
    ).toHaveCount(0)
    await open(bob, workspacePath)
    await add(bob, aliceUsername, me.id, 'owner')
    await close(bob)
    await focus(alice)
    await expect(
      alice
        .locator('main')
        .getByRole('button', { name: en.sharing.open, exact: true }),
    ).toBeVisible()

    // A real principal can disappear between search and confirmation.
    const disposable = await admin.request.post('/api/v1/admin/groups', {
      headers: h,
      data: { name: `vanishing-${b.username}` },
    })
    expect(disposable.status()).toBe(201)
    const vanished = (await disposable.json()).group as {
      id: string
      name: string
    }
    transientGroup = vanished.id
    await open(alice, workspacePath)
    await reviewAdd(alice, vanished.name, vanished.id, 'read', 'group')
    expect(
      (
        await admin.request.delete(`/api/v1/admin/groups/${vanished.id}`, {
          headers: h,
        })
      ).status(),
    ).toBe(200)
    transientGroup = undefined
    expectHTTPFailure(alice, 'PUT', '/api/v1/grants', 404)
    await dialog(alice)
      .getByRole('button', { name: en.admin.confirm, exact: true })
      .click()
    await expect(dialog(alice).getByRole('alert')).toContainText(
      en.errors.ERR_PRINCIPAL_NOT_FOUND,
    )
    await dialog(alice)
      .getByRole('button', { name: en.admin.cancel, exact: true })
      .click()
    await close(alice)

    const created = await admin.request.post('/api/v1/admin/users', {
      headers: h,
      data: {
        username: `picker-${b.username}`,
        displayName: 'Picker target',
        role: 'user',
        password: 'Temporary-password-123',
      },
    })
    expect(created.status()).toBe(201)
    const target = (await created.json()).user as {
      id: string
      username: string
    }
    await open(alice, workspacePath)
    await reviewAdd(alice, target.username, target.id, 'read')
    expect(
      (
        await admin.request.patch(`/api/v1/admin/users/${target.id}`, {
          headers: h,
          data: { status: 'disabled' },
        })
      ).status(),
    ).toBe(200)
    expectHTTPFailure(alice, 'PUT', '/api/v1/grants', 409)
    await dialog(alice)
      .getByRole('button', { name: en.admin.confirm, exact: true })
      .click()
    await expect(dialog(alice).getByRole('alert')).toContainText(
      en.errors.ERR_PRINCIPAL_DISABLED,
    )
    await dialog(alice)
      .getByRole('button', { name: en.admin.cancel, exact: true })
      .click()
    await dialog(alice)
      .getByLabel(en.sharing.query, { exact: true })
      .fill(target.username)
    await dialog(alice)
      .getByRole('button', { name: en.sharing.search, exact: true })
      .click()
    await expect(dialog(alice)).toContainText(en.sharing.emptySearch)
    await noOverflow(alice)
    await alice.screenshot({
      path: test.info().outputPath('sharing-en-light.png'),
      fullPage: true,
    })
    await close(alice)
    await alice
      .getByRole('button', { name: en.common.languages.zhCN, exact: true })
      .click()
    await alice
      .getByRole('button', { name: zh.common.themes.dark, exact: true })
      .click()
    await open(alice, tablePath, zh)
    await expect(dialog(alice, zh)).toContainText(zh.sharing.inherited)
    await expect(alice.locator('html')).toHaveAttribute('data-theme', 'dark')
    await noOverflow(alice)
    await alice.screenshot({
      path: test.info().outputPath('sharing-zh-dark.png'),
      fullPage: true,
    })
    await close(alice, zh)
    await alice
      .getByRole('button', { name: zh.common.themes.light, exact: true })
      .click()
    // Preserve T1.11's Chinese-locale continuation and independent admin inventory.
    expect(
      (await admin.request.get(`/api/v1/tables/${tid}/schema`)).status(),
    ).toBe(404)
    expect(aliceAdminCalls).toEqual([])
  } finally {
    alice.off('request', capture)
    for (const id of [groupId, transientGroup]) {
      if (id)
        expect(
          (
            await admin.request.delete(`/api/v1/admin/groups/${id}`, {
              headers: h,
            })
          ).status(),
        ).toBe(200)
    }
  }
}
