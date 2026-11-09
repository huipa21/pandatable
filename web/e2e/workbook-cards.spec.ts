import type { BrowserContext, Page } from '@playwright/test'
import type {
  AdminUserMutation,
  Book,
  Navigation,
  TableSchema,
} from '../src/api/types'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }
import { setLocale, setTheme } from './app-shell'
import { closeMobileNavigation, openShare, shareAction } from './sharing'
import {
  expect,
  expectBrowserClean,
  expectHTTPFailure,
  observeBrowser,
  paceLogin,
  test,
} from './fixtures'

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

async function createWorkspace(page: Page, name: string) {
  const response = await page.request.post('/api/v1/workspaces', {
    headers: headers(),
    data: { name },
  })
  expect(response.status()).toBe(201)
  return ((await response.json()) as { workspace: { id: string } }).workspace.id
}

async function createBook(
  page: Page,
  workspaceId: string,
  name: string,
  icon?: Book['icon'],
) {
  const response = await page.request.post(
    `/api/v1/workspaces/${workspaceId}/books`,
    { headers: headers(), data: { name, ...(icon ? { icon } : {}) } },
  )
  expect(response.status()).toBe(201)
  return ((await response.json()) as { book: Book }).book
}

async function verifyContextualSidebar(
  page: Page,
  mobile: boolean,
  workspaceId: string,
) {
  if (mobile)
    await page.getByRole('button', { name: en.nav.open, exact: true }).click()
  const sidebar = page.locator(
    mobile ? '.nav-drawer .nav-tree' : '.sidebar-desktop .nav-tree',
  )
  await expect(sidebar.locator('.workspace-root > li')).toHaveCount(2)
  await expect(
    sidebar.locator('.workspace-root [data-kind="dashboard"] .workspace-link'),
  ).toHaveAttribute('href', '/home')
  await expect(
    sidebar.locator('.workspace-root > li[data-kind="workspace"]'),
  ).toHaveAttribute('data-nav-id', workspaceId)
  await expect(
    sidebar.locator(
      '.workspace-toggle, .workspace-section, .workspace-root [data-kind="book"], .workspace-root [data-kind="table"]',
    ),
  ).toHaveCount(0)
  if (mobile) {
    await page.keyboard.press('Escape')
    await expect(page.locator('.nav-drawer')).not.toBeVisible()
  }
}

async function selectWorkspace(page: Page, id: string) {
  const mobile = await page.evaluate(
    () => window.matchMedia('(max-width: 767px)').matches,
  )
  if (new URL(page.url()).pathname !== '/home') {
    if (mobile && !(await page.locator('.nav-drawer').isVisible()))
      await page.getByRole('button', { name: en.nav.open, exact: true }).click()
    const sidebar = page.locator(mobile ? '.nav-drawer' : '.sidebar-desktop')
    await sidebar
      .locator('.workspace-root [data-kind="dashboard"] .workspace-link')
      .click()
    await expect(page).toHaveURL(/\/home$/)
    if (mobile) await expect(page.locator('.nav-drawer')).not.toBeVisible()
  }
  await page
    .locator(
      `.workspace-dashboard [data-nav-id="${id}"] > .workspace-dashboard-link`,
    )
    .click()
  await expect(page).toHaveURL(new RegExp(`/w/${id}$`))
  await verifyContextualSidebar(page, mobile, id)
}

async function verifyStaleRetry(page: Page, expectedCards: number) {
  let fail = true
  await page.route('**/api/v1/nav', async (route) => {
    if (!fail) {
      await route.continue()
      return
    }
    fail = false
    await route.fulfill({
      status: 503,
      contentType: 'application/json',
      body: JSON.stringify({ error: { code: 'ERR_INTERNAL' } }),
    })
  })
  expectHTTPFailure(page, 'GET', '/api/v1/nav', 503)
  await page.evaluate(() => window.dispatchEvent(new Event('focus')))
  const main = page.locator('main')
  const stale = main.locator('.workbook-stale')
  await expect(stale).toContainText(en.nav.stale)
  await expect(main.locator('.workbook-card')).toHaveCount(expectedCards)
  await expect(main.getByText(en.nav.emptyBooks, { exact: true })).toHaveCount(
    0,
  )
  await stale.getByRole('button', { name: en.auth.retry, exact: true }).click()
  await expect(stale).toHaveCount(0)
  await expect(main.locator('.workbook-card')).toHaveCount(expectedCards)
  if (expectedCards === 0)
    await expect(
      main.getByText(en.nav.emptyBooks, { exact: true }),
    ).toBeVisible()
  await page.unroute('**/api/v1/nav')
}

async function verifyLayout(page: Page, width: number) {
  await page.setViewportSize({ width, height: 900 })
  const main = page.locator('main')
  await expect(main.getByRole('searchbox')).toHaveCount(0)
  await expect(
    main.getByRole('button', { name: en.nav.create.book, exact: true }),
  ).toHaveCount(0)
  const cards = page.locator('main .workbook-card')
  await expect(cards).toHaveCount(3)
  const boxes = await cards.evaluateAll((elements) =>
    elements.map((element) => {
      const card = element.getBoundingClientRect()
      const name = element.querySelector('.workbook-card-name')
      if (!name) throw new Error('Workbook card name is missing')
      const text = name.getBoundingClientRect()
      return {
        left: card.left,
        right: card.right,
        top: card.top,
        bottom: card.bottom,
        nameLeft: text.left,
        nameRight: text.right,
        nameBottom: text.bottom,
        scrollWidth: element.scrollWidth,
        clientWidth: element.clientWidth,
      }
    }),
  )
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true)
  for (const [index, box] of boxes.entries()) {
    expect(box.left).toBeGreaterThanOrEqual(0)
    expect(box.right).toBeLessThanOrEqual(width)
    expect(box.nameLeft).toBeGreaterThanOrEqual(box.left)
    expect(box.nameRight).toBeLessThanOrEqual(box.right)
    expect(box.nameBottom).toBeLessThanOrEqual(box.bottom)
    expect(box.scrollWidth).toBeLessThanOrEqual(box.clientWidth)
    for (const other of boxes.slice(index + 1)) {
      expect(
        box.right <= other.left ||
          other.right <= box.left ||
          box.bottom <= other.top ||
          other.bottom <= box.top,
      ).toBe(true)
    }
  }
  if (width === 390) expect(boxes[1]!.top).toBeGreaterThan(boxes[0]!.top)
  else expect(boxes[1]!.top).toBe(boxes[0]!.top)
}

test('UX-04: workbook cards respect icons, access, navigation and responsive states', async ({
  page,
  browser,
  isMobile,
}, testInfo) => {
  test.setTimeout(300_000)
  await paceLogin(page)
  const login = await page.request.post('/api/v1/auth/login', {
    headers: headers(),
    data: { login: 'first-admin', password: 'Setup-password-123' },
  })
  expect(login.status()).toBe(200)
  const suffix = `${Date.now()}-${testInfo.project.name.startsWith('mobile') ? 'm' : 'd'}`
  const workspaceIds: string[] = []
  let readerContext: BrowserContext | undefined
  try {
    const firstWorkspaceName = `UX04 first ${suffix}`
    const firstWorkspace = await createWorkspace(page, firstWorkspaceName)
    workspaceIds.push(firstWorkspace)
    const otherWorkspace = await createWorkspace(page, `UX04 other ${suffix}`)
    workspaceIds.push(otherWorkspace)
    const latinName = `UX04${'LongWorkbookName'.repeat(5)}`
    const chineseName = '工作簿长名称项目'.repeat(10)
    const fallbackBook = await createBook(page, otherWorkspace, latinName)
    const customBook = await createBook(
      page,
      otherWorkspace,
      chineseName,
      'star',
    )
    const recentBook = await createBook(page, otherWorkspace, 'Planner')

    await page.goto(`/w/${firstWorkspace}`)
    await setLocale(page, 'en')
    await setTheme(page, 'light')
    await verifyContextualSidebar(page, isMobile, firstWorkspace)
    const main = page.locator('main')
    await expect(
      main.getByText(en.nav.emptyBooks, { exact: true }),
    ).toBeVisible()
    await expect(
      main.getByText(en.wmb.emptyBooksHint, { exact: true }),
    ).toBeVisible()
    await expect(main.locator('.workbook-card')).toHaveCount(0)
    await expect(main.getByRole('searchbox')).toHaveCount(0)
    await expect(
      main.getByRole('button', { name: en.nav.create.book, exact: true }),
    ).toHaveCount(0)
    await expect(
      main.getByRole('button', { name: en.nav.create.workspace, exact: true }),
    ).toHaveCount(0)
    await expect(main).not.toContainText(en.auth.homeTitle)
    await expect(main).not.toContainText(en.nav.chooseWorkspace)
    await verifyStaleRetry(page, 0)

    await expect(
      main.getByRole('button', { name: en.sharing.open, exact: true }),
    ).toHaveCount(0)
    await openShare(page, 'workspace', firstWorkspace, en)
    const sharing = page.getByRole('dialog', {
      name: en.sharing.title,
      exact: true,
    })
    await expect(sharing).toBeVisible()
    await expect(
      sharing.getByRole('heading', { name: en.sharing.direct, exact: true }),
    ).toBeVisible()
    await sharing
      .getByRole('button', { name: en.sharing.close, exact: true })
      .click()
    await closeMobileNavigation(page, en)
    await selectWorkspace(page, firstWorkspace)

    if (isMobile)
      await page.getByRole('button', { name: en.nav.open, exact: true }).click()
    await page
      .locator(
        `${isMobile ? '.nav-drawer' : '.sidebar-desktop'} .nav-tree [data-nav-id="${firstWorkspace}"] .workspace-nav-menu summary`,
      )
      .click()
    await page
      .getByRole('button', { name: en.nav.create.book, exact: true })
      .click()
    const structure = page.getByRole('dialog', {
      name: en.nav.dialogTitle,
      exact: true,
    })
    await structure
      .getByRole('textbox', { name: en.nav.name })
      .fill('First book')
    const creation = page.waitForResponse(
      (response) =>
        response.request().method() === 'POST' &&
        new URL(response.url()).pathname ===
          `/api/v1/workspaces/${firstWorkspace}/books`,
    )
    await structure.getByRole('button', { name: en.admin.confirm }).click()
    const createdResponse = await creation
    expect(createdResponse.status()).toBe(201)
    const firstBook = ((await createdResponse.json()) as { book: Book }).book
    await expect(page).toHaveURL(
      new RegExp(`/w/${firstWorkspace}/b/${firstBook.id}$`),
    )
    await selectWorkspace(page, firstWorkspace)
    await expect(main.locator('.workbook-card')).toHaveCount(1)
    await expect(
      main.getByRole('link', { name: 'First book' }),
    ).toHaveAttribute('href', `/w/${firstWorkspace}/b/${firstBook.id}`)

    const tableResponse = await page.request.post(
      `/api/v1/books/${firstBook.id}/tables`,
      { headers: headers(), data: { name: 'Shared table' } },
    )
    expect(tableResponse.status()).toBe(201)
    const table = ((await tableResponse.json()) as TableSchema).table
    const hiddenBook = await createBook(page, firstWorkspace, 'Hidden sibling')

    await selectWorkspace(page, otherWorkspace)
    const cards = main.locator('.workbook-card')
    await expect(cards).toHaveCount(3)
    await expect(main).not.toContainText('First book')
    const response = await page.request.get('/api/v1/nav')
    expect(response.status()).toBe(200)
    const tree = (await response.json()) as Navigation
    const orderedBooks = tree.workspaces.find(
      (workspace) => workspace.id === otherWorkspace,
    )?.books
    if (!orderedBooks) throw new Error('Expected the accessible workspace')
    expect(
      await cards.evaluateAll((items) =>
        items.map((item) => item.getAttribute('href')),
      ),
    ).toEqual(orderedBooks.map((book) => `/w/${otherWorkspace}/b/${book.id}`))

    const fallbackCard = main.getByRole('link', {
      name: latinName,
      exact: true,
    })
    const customCard = main.getByRole('link', {
      name: chineseName,
      exact: true,
    })
    await expect(customCard).toHaveAttribute(
      'href',
      `/w/${otherWorkspace}/b/${customBook.id}`,
    )

    const recordOpen = page.waitForResponse((response) => {
      const request = response.request()
      return (
        request.method() === 'POST' &&
        new URL(response.url()).pathname === '/api/v1/recent' &&
        !!request.postData()?.includes(recentBook.id)
      )
    })
    await main.getByRole('link', { name: 'Planner', exact: true }).click()
    expect((await recordOpen).status()).toBe(204)
    await expect(page).toHaveURL(
      new RegExp(`/w/${otherWorkspace}/b/${recentBook.id}$`),
    )
    await page.goBack()
    await expect(page).toHaveURL(new RegExp(`/w/${otherWorkspace}$`))
    await expect(cards.first()).toHaveAttribute(
      'href',
      `/w/${otherWorkspace}/b/${recentBook.id}`,
    )

    const fallbackMarkup = await fallbackCard.locator('svg').innerHTML()
    expect(fallbackMarkup).toContain('<path')
    await expect(customCard.locator('svg path').first()).toHaveAttribute(
      'd',
      'M12 3l3 6 6 1-4.5 4.5 1 6.5-5.5-3-5.5 3 1-6.5L3 10l6-1z',
    )
    const override = fallbackBook.fallbackIcon === 'star' ? 'calendar' : 'star'
    const setIcon = await page.request.patch(
      `/api/v1/books/${fallbackBook.id}`,
      {
        headers: headers(),
        data: { icon: override },
      },
    )
    expect(setIcon.status()).toBe(200)
    await page.reload()
    await expect(fallbackCard).toBeVisible()
    expect(await fallbackCard.locator('svg').innerHTML()).not.toBe(
      fallbackMarkup,
    )
    await expect(customCard.locator('svg path').first()).toHaveAttribute(
      'd',
      'M12 3l3 6 6 1-4.5 4.5 1 6.5-5.5-3-5.5 3 1-6.5L3 10l6-1z',
    )
    const clearIcon = await page.request.patch(
      `/api/v1/books/${fallbackBook.id}`,
      { headers: headers(), data: { icon: null } },
    )
    expect(clearIcon.status()).toBe(200)
    await page.reload()
    await expect(fallbackCard).toBeVisible()
    expect(await fallbackCard.locator('svg').innerHTML()).toBe(fallbackMarkup)

    await page.keyboard.press('Tab')
    await fallbackCard.focus()
    await expect(fallbackCard).toBeFocused()
    const focus = await fallbackCard.evaluate((element) => ({
      visible: element.matches(':focus-visible'),
      width: Number.parseFloat(getComputedStyle(element).outlineWidth),
    }))
    expect(focus.visible).toBe(true)
    expect(focus.width).toBeGreaterThanOrEqual(2)
    await page.keyboard.press('Enter')
    await expect(page).toHaveURL(
      new RegExp(`/w/${otherWorkspace}/b/${fallbackBook.id}$`),
    )
    await selectWorkspace(page, firstWorkspace)
    await expect(main.locator('.workbook-card')).toHaveCount(2)
    await expect(main).not.toContainText(latinName)
    await selectWorkspace(page, otherWorkspace)
    await expect(main.locator('.workbook-card')).toHaveCount(3)
    await expect(main).not.toContainText('First book')
    await verifyStaleRetry(page, 3)

    for (const width of isMobile ? [390] : [1440, 900, 390])
      await verifyLayout(page, width)
    await setLocale(page, 'zh-CN')
    await setTheme(page, 'dark')
    await verifyLayout(page, 390)
    await expect(main.getByRole('link', { name: chineseName })).toBeVisible()
    await setLocale(page, 'en')
    await setTheme(page, 'light')

    const username = `ux04-${suffix}`
    const userResponse = await page.request.post('/api/v1/admin/users', {
      headers: headers(),
      data: {
        username,
        displayName: 'Workbook card reader',
        role: 'user',
        password: 'Temporary-password-123',
      },
    })
    expect(userResponse.status()).toBe(201)
    const user = ((await userResponse.json()) as AdminUserMutation).user
    for (const [resourceType, resourceId] of [
      ['workspace', otherWorkspace],
      ['table', table.id],
    ] as const) {
      const grant = await page.request.put('/api/v1/grants', {
        headers: headers(),
        data: {
          resourceType,
          resourceId,
          principalType: 'user',
          principalId: user.id,
          level: 'read',
        },
      })
      expect(grant.status()).toBe(201)
    }

    readerContext = await browser.newContext({
      baseURL: process.env.PT_E2E_BASE_URL,
      viewport: isMobile
        ? { width: 390, height: 844 }
        : { width: 1440, height: 900 },
      isMobile,
      hasTouch: isMobile,
      locale: 'en-US',
    })
    const reader = await readerContext.newPage()
    const diagnostics = await observeBrowser(reader)
    await paceLogin()
    const initialLogin = await readerContext.request.post(
      '/api/v1/auth/login',
      {
        headers: headers(),
        data: { login: username, password: 'Temporary-password-123' },
      },
    )
    expect(initialLogin.status()).toBe(200)
    const password = await readerContext.request.post('/api/v1/auth/password', {
      headers: headers(),
      data: { current: 'Temporary-password-123', new: 'Changed-password-123' },
    })
    expect(password.status()).toBe(200)
    await paceLogin()
    const readerLogin = await readerContext.request.post('/api/v1/auth/login', {
      headers: headers(),
      data: { login: username, password: 'Changed-password-123' },
    })
    expect(readerLogin.status()).toBe(200)

    await reader.goto(`/w/${otherWorkspace}`)
    const readerMain = reader.locator('main')
    await expect(readerMain.locator('.workbook-card')).toHaveCount(3)
    await expect(
      await shareAction(reader, 'workspace', otherWorkspace),
    ).toHaveCount(0)
    await closeMobileNavigation(reader)
    await selectWorkspace(reader, otherWorkspace)
    await expect(
      readerMain.getByRole('button', { name: en.nav.create.book, exact: true }),
    ).toHaveCount(0)
    expect(
      await readerMain
        .getByRole('link', { name: latinName, exact: true })
        .locator('svg')
        .innerHTML(),
    ).toBe(fallbackMarkup)
    await expect(
      readerMain
        .getByRole('link', { name: chineseName, exact: true })
        .locator('svg path')
        .first(),
    ).toHaveAttribute(
      'd',
      'M12 3l3 6 6 1-4.5 4.5 1 6.5-5.5-3-5.5 3 1-6.5L3 10l6-1z',
    )

    await reader.goto(`/w/${firstWorkspace}`)
    await setLocale(reader, 'zh-CN')
    await setTheme(reader, 'dark')
    await expect(readerMain).toContainText(zh.nav.pathNotice)
    await expect(readerMain.locator('.workbook-card')).toHaveCount(1)
    const pathCard = readerMain.locator('.workbook-card')
    await expect(pathCard).toContainText('First book')
    await expect(pathCard).toContainText(zh.nav.pathOnly)
    await expect(readerMain).not.toContainText('Hidden sibling')
    await expect(readerMain.getByRole('button')).toHaveCount(0)
    const readerNav = (await (
      await readerContext.request.get('/api/v1/nav')
    ).json()) as Navigation
    const visibleBooks = readerNav.workspaces.find(
      (workspace) => workspace.id === firstWorkspace,
    )?.books
    expect(visibleBooks?.map((book) => book.id)).toEqual([firstBook.id])
    await pathCard.click()
    await expect(reader).toHaveURL(
      new RegExp(`/w/${firstWorkspace}/b/${firstBook.id}$`),
    )
    await expect(readerMain).toContainText(zh.nav.pathNotice)
    await expect(readerMain.getByRole('button')).toHaveCount(0)
    await expect(
      readerMain.getByRole('link', { name: 'Shared table' }),
    ).toBeVisible()
    const deniedRecent = reader.waitForResponse((response) => {
      const request = response.request()
      return (
        request.method() === 'POST' &&
        new URL(response.url()).pathname === '/api/v1/recent' &&
        !!request.postData()?.includes(hiddenBook.id)
      )
    })
    expectHTTPFailure(reader, 'POST', '/api/v1/recent', 404)
    await reader.goto(`/w/${firstWorkspace}/b/${hiddenBook.id}`)
    expect((await deniedRecent).status()).toBe(404)
    await expect(readerMain.getByRole('heading', { level: 1 })).toHaveText(
      zh.nav.removedTitle,
    )
    expectBrowserClean(reader, diagnostics)
  } finally {
    await readerContext?.close()
    for (const workspaceId of workspaceIds) {
      const deleted = await page.request.delete(
        `/api/v1/workspaces/${workspaceId}`,
        { headers: headers() },
      )
      expect(deleted.status()).toBe(200)
    }
  }
})
