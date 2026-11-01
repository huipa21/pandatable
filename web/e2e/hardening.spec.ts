import type { Locator, Page, Request } from '@playwright/test'
import {
  exerciseOnce,
  readMemorySample,
  scrollToLoaded,
  trackPage,
  validateGridView,
  waitForSubscription,
} from '../scripts/load-test.mjs'
import type {
  AuthUserMutation,
  DataRow,
  RowPatchMutation,
  TableSchema,
  ViewMutation,
} from '../src/api/types'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }
import { expect, paceLogin, test } from './fixtures'

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

async function login(
  page: Page,
  username = 'first-admin',
  password = 'Setup-password-123',
) {
  await paceLogin(page)
  const response = await page.request.post('/api/v1/auth/login', {
    headers: headers(),
    data: { login: username, password },
  })
  expect(response.status()).toBe(200)
  return ((await response.json()) as AuthUserMutation).user.id
}

async function fixture(page: Page, suffix: string, count: number) {
  const workspaceName = `Hardening workspace ${suffix}`
  const bookName = `Hardening book ${suffix}`
  const tableName = `Hardening table ${suffix}`
  const workspace = await page.request.post('/api/v1/workspaces', {
    headers: headers(),
    data: { name: workspaceName },
  })
  expect(workspace.status()).toBe(201)
  const workspaceId = (await workspace.json()).workspace.id as string
  const book = await page.request.post(
    `/api/v1/workspaces/${workspaceId}/books`,
    { headers: headers(), data: { name: bookName } },
  )
  expect(book.status()).toBe(201)
  const bookId = (await book.json()).book.id as string
  const table = await page.request.post(`/api/v1/books/${bookId}/tables`, {
    headers: headers(),
    data: { name: tableName },
  })
  expect(table.status()).toBe(201)
  const tableId = (await table.json()).table.id as string
  const schemaResponse = await page.request.get(
    `/api/v1/tables/${tableId}/schema`,
  )
  expect(schemaResponse.status()).toBe(200)
  const schema = (await schemaResponse.json()) as TableSchema
  const fieldId = schema.table.displayFieldId
  const viewId = schema.views[0]?.id
  if (!fieldId || !viewId) throw new Error('Fixture schema is incomplete')
  validateGridView(schema.views[0])
  const rows: DataRow[] = []
  for (let offset = 0; offset < count; offset += 200) {
    const response = await page.request.post(`/api/v1/tables/${tableId}/rows`, {
      headers: headers(),
      data: {
        rows: Array.from(
          { length: Math.min(200, count - offset) },
          (_, index) => ({
            fields: { [fieldId]: `Fixture ${offset + index}` },
          }),
        ),
      },
    })
    expect(response.status()).toBe(201)
    rows.push(...((await response.json()).rows as DataRow[]))
  }
  return {
    workspaceName,
    bookName,
    tableName,
    workspaceId,
    bookId,
    tableId,
    viewId,
    fieldId,
    rows,
  }
}

async function noPageOverflow(page: Page, path: string) {
  await expect
    .poll(
      () =>
        page.evaluate(
          () => document.documentElement.scrollWidth <= window.innerWidth,
        ),
      { message: `No horizontal page overflow on ${path}` },
    )
    .toBe(true)
}

async function reachableControls(page: Page, path: string, scope?: Locator) {
  const viewport = page.viewportSize()
  if (!viewport) throw new Error('Responsive test requires a fixed viewport')
  const controls = scope
    ? scope.locator(
        ':is(button, a[href], input:not([type="hidden"]), select, textarea):visible',
      )
    : page.locator(
        ':is(header button, header a[href], main button, main a[href], ' +
          'main input:not([type="hidden"]), main select, main textarea):visible',
      )
  const visible = await controls.evaluateAll((elements) =>
    elements.map((element, index) => {
      element.scrollIntoView({ block: 'center', inline: 'nearest' })
      const scrolled = element.getBoundingClientRect()
      return {
        index,
        x: scrolled.x,
        width: scrolled.width,
        enabled:
          !element.matches(':disabled') &&
          !element.closest('[aria-disabled="true"]'),
      }
    }),
  )
  let enabled = 0
  for (const control of visible) {
    expect(
      control.x,
      `${path}: control ${control.index + 1} left edge`,
    ).toBeGreaterThanOrEqual(0)
    expect(
      control.x + control.width,
      `${path}: control ${control.index + 1} right edge`,
    ).toBeLessThanOrEqual(viewport.width)
    if (control.enabled) {
      enabled++
      await controls.nth(control.index).click({ trial: true, timeout: 5_000 })
    }
  }
  expect(visible.length, `${path}: visible controls`).toBeGreaterThan(0)
  expect(enabled, `${path}: enabled controls`).toBeGreaterThan(0)
}

async function english(page: Page) {
  if ((await page.locator('html').getAttribute('lang')) !== 'zh-CN') return
  await page
    .getByRole('button', { name: zh.common.languages.en, exact: true })
    .click()
  await expect(page.locator('html')).toHaveAttribute('lang', 'en')
}

async function usableDialog(page: Page, dialog: Locator, name: string) {
  await expect(dialog).toBeVisible()
  const viewport = page.viewportSize()
  if (!viewport) throw new Error('Responsive test requires a fixed viewport')
  const box = await dialog.evaluate((element) => {
    const rect = element.getBoundingClientRect()
    return {
      x: rect.x,
      y: rect.y,
      width: rect.width,
      height: rect.height,
      scrollWidth: element.scrollWidth,
      clientWidth: element.clientWidth,
    }
  })
  expect(box.x, `${name}: left edge`).toBeGreaterThanOrEqual(0)
  expect(box.x + box.width, `${name}: right edge`).toBeLessThanOrEqual(
    viewport.width,
  )
  expect(box.y, `${name}: top edge`).toBeGreaterThanOrEqual(0)
  expect(box.y + box.height, `${name}: bottom edge`).toBeLessThanOrEqual(
    viewport.height,
  )
  expect(box.scrollWidth, `${name}: horizontal overflow`).toBeLessThanOrEqual(
    box.clientWidth,
  )
  await reachableControls(page, name, dialog)
  await noPageOverflow(page, name)
}

async function usableMenus(page: Page, scope: Locator, name: string) {
  const summaries = scope.locator('details > summary:visible')
  await expect(summaries.first()).toBeVisible()
  const count = await summaries.count()
  for (let index = 0; index < count; index++) {
    const summary = summaries.nth(index)
    const menu = summary.locator('..')
    await summary.click()
    await expect(menu).toHaveAttribute('open', '')
    await reachableControls(page, `${name} ${index + 1}`, menu)
    await noPageOverflow(page, `${name} ${index + 1}`)
    await summary.click()
    await expect(menu).not.toHaveAttribute('open', '')
  }
}

async function closeDialogsAndRestoreEnglish(page: Page) {
  if (page.isClosed()) return
  for (let count = 0; count < 3; count++) {
    if ((await page.locator('dialog[open]').count()) === 0) break
    await page.keyboard.press('Escape')
  }
  await english(page)
}

async function inspectSharing(page: Page, locale: typeof en, search: boolean) {
  await page
    .locator('main')
    .getByRole('button', { name: locale.sharing.open, exact: true })
    .click()
  const sharing = page.getByRole('dialog', {
    name: locale.sharing.title,
    exact: true,
  })
  await expect(
    sharing.getByRole('heading', {
      name: locale.sharing.direct,
      exact: true,
    }),
  ).toBeVisible()
  await usableDialog(page, sharing, 'sharing dialog')
  if (search) {
    await sharing
      .getByLabel(locale.sharing.query, { exact: true })
      .fill('responsive-no-such-principal')
    await sharing
      .getByRole('button', { name: locale.sharing.search, exact: true })
      .click()
    await expect(
      sharing.getByText(locale.sharing.emptySearch, { exact: true }),
    ).toBeVisible()
  }
  await sharing
    .getByRole('button', { name: locale.sharing.close, exact: true })
    .click()
  await expect(sharing).not.toBeVisible()
}

async function inspectImport(
  page: Page,
  locale: typeof en,
  mode: 'newTable' | 'append',
) {
  await page
    .locator('main')
    .getByRole('button', { name: locale.import[mode], exact: true })
    .click()
  const importing = page.getByRole('dialog', {
    name: locale.import.title,
    exact: true,
  })
  await usableDialog(page, importing, 'import dialog')
  await importing
    .getByLabel(locale.import.file, { exact: true })
    .setInputFiles({
      name: 'responsive.csv',
      mimeType: 'text/csv',
      buffer: Buffer.from('Name\nResponsive row\n'),
    })
  await importing
    .getByRole('button', { name: locale.import.analyze, exact: true })
    .click()
  await expect(
    importing.getByRole('heading', {
      name: locale.import.mapping,
      exact: true,
    }),
  ).toBeVisible()
  if (mode === 'newTable')
    await importing
      .getByLabel(locale.import.tableName, { exact: true })
      .fill('Responsive draft table')
  await usableDialog(page, importing, 'import mapping dialog')
  await importing
    .getByRole('button', { name: locale.import.close, exact: true })
    .click()
  await expect(importing).not.toBeVisible()
}

test('AT-806 main pages remain usable at 390px in both languages', async ({
  page,
}, testInfo) => {
  test.setTimeout(150_000)
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/login')
  await english(page)
  await expect(
    page.getByRole('heading', { name: en.login.title }),
  ).toBeVisible()
  await expect(page.getByLabel(en.auth.login, { exact: true })).toBeVisible()
  await page.waitForLoadState('networkidle')
  await noPageOverflow(page, '/login')
  await reachableControls(page, '/login')

  await login(page)
  const data = await fixture(page, `${testInfo.project.name}-${Date.now()}`, 1)
  const routes = [
    { path: '/home', en: en.auth.homeTitle, zh: zh.auth.homeTitle },
    { path: '/setup', en: en.setup.title, zh: zh.setup.title },
    {
      path: `/w/${data.workspaceId}`,
      en: data.workspaceName,
      zh: data.workspaceName,
    },
    {
      path: `/w/${data.workspaceId}/b/${data.bookId}`,
      en: data.bookName,
      zh: data.bookName,
    },
    {
      path: `/t/${data.tableId}/v/${data.viewId}`,
      en: data.tableName,
      zh: data.tableName,
    },
    { path: '/profile', en: en.profile.title, zh: zh.profile.title },
    {
      path: '/change-password',
      en: en.auth.changePassword,
      zh: zh.auth.changePassword,
    },
    { path: '/trash', en: en.trash.title, zh: zh.trash.title },
    {
      path: '/admin/dashboard',
      en: en.dashboard.title,
      zh: zh.dashboard.title,
    },
    { path: '/admin/users', en: en.admin.users, zh: zh.admin.users },
    { path: '/admin/groups', en: en.groups.title, zh: zh.groups.title },
    {
      path: '/admin/settings',
      en: en.settings.title,
      zh: zh.settings.title,
    },
    {
      path: '/admin/workspaces',
      en: en.workspaces.title,
      zh: zh.workspaces.title,
    },
    { path: '/admin/backups', en: en.backups.title, zh: zh.backups.title },
    {
      path: '/connection-error',
      en: en.auth.connectionError,
      zh: zh.auth.connectionError,
    },
    {
      path: '/not-yet-implemented',
      en: en.nav.notFoundTitle,
      zh: zh.nav.notFoundTitle,
    },
  ]
  try {
    for (const locale of ['en', 'zh'] as const) {
      if (locale === 'zh')
        await page
          .getByRole('button', { name: en.common.languages.zhCN, exact: true })
          .click()
      for (const route of routes) {
        await page.goto(route.path)
        await expect(page.locator('main h1')).toHaveText(route[locale])
        await page.waitForLoadState('networkidle')
        await noPageOverflow(page, route.path)
        if (route.path.startsWith('/t/')) {
          await expect(
            page.getByRole('region', {
              name: locale === 'en' ? en.mobileRows.title : zh.mobileRows.title,
              exact: true,
            }),
          ).toBeVisible()
        }
        await reachableControls(page, route.path)
        await noPageOverflow(page, route.path)
      }
    }
  } finally {
    if (!page.isClosed()) await english(page)
  }
})

test('AT-806 resource menus and structure dialogs at 390px in both languages', async ({
  page,
}, testInfo) => {
  test.setTimeout(150_000)
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/login')
  await english(page)
  await login(page)
  const data = await fixture(page, `menus-${testInfo.project.name}`, 1)
  try {
    for (const locale of [en, zh]) {
      if (locale === zh)
        await page
          .getByRole('button', { name: en.common.languages.zhCN, exact: true })
          .click()
      for (const target of [
        { path: '/home', kind: 'workspace' },
        { path: `/w/${data.workspaceId}`, kind: 'book' },
        { path: `/w/${data.workspaceId}/b/${data.bookId}`, kind: 'table' },
      ] as const) {
        await page.goto(target.path)
        await page
          .locator('main')
          .getByRole('button', {
            name: locale.nav.create[target.kind],
            exact: true,
          })
          .click()
        const dialog = page.getByRole('dialog', {
          name: locale.nav.dialogTitle,
          exact: true,
        })
        await dialog
          .getByRole('textbox', { name: locale.nav.name, exact: true })
          .fill('Responsive draft')
        await usableDialog(page, dialog, `${target.kind} dialog`)
        await dialog
          .getByRole('button', { name: locale.admin.cancel, exact: true })
          .click()
        await expect(dialog).not.toBeVisible()
        if (target.kind === 'book') await inspectSharing(page, locale, false)
        if (target.kind === 'table')
          await inspectImport(page, locale, 'newTable')
      }

      await page.goto(`/t/${data.tableId}/v/${data.viewId}`)
      await usableMenus(page, page.locator('main'), 'table menu')

      await page
        .getByRole('button', { name: locale.nav.open, exact: true })
        .click()
      const drawer = page.getByRole('dialog', {
        name: locale.nav.title,
        exact: true,
      })
      await usableDialog(page, drawer, 'navigation drawer')
      const actions = drawer.locator(
        `[data-nav-id="${data.workspaceId}"] > .node-actions`,
      )
      await actions.locator('summary').click()
      await reachableControls(page, 'workspace action menu', actions)
      await noPageOverflow(page, 'workspace action menu')
      await drawer
        .getByRole('button', { name: locale.nav.close, exact: true })
        .click()
      await expect(drawer).not.toBeVisible()
    }
  } finally {
    await closeDialogsAndRestoreEnglish(page)
  }
})

test('AT-806 table dialogs at 390px in both languages', async ({
  page,
}, testInfo) => {
  test.setTimeout(150_000)
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/login')
  await english(page)
  await login(page)
  const data = await fixture(page, `dialogs-${testInfo.project.name}`, 1)
  const row = data.rows[0]
  if (!row) throw new Error('Responsive fixture did not create its row')
  try {
    for (const locale of [en, zh]) {
      if (locale === zh)
        await page
          .getByRole('button', { name: en.common.languages.zhCN, exact: true })
          .click()
      await page.goto(`/t/${data.tableId}/v/${data.viewId}`)

      await inspectSharing(page, locale, true)
      await inspectImport(page, locale, 'append')

      const selectedRow = page.getByRole('checkbox', {
        name: locale.grid.selectRow.replace('{id}', row.id),
        exact: true,
      })
      await selectedRow.check()

      await page
        .getByRole('button', { name: locale.mobileRows.options, exact: true })
        .click()
      const options = page.getByRole('dialog', {
        name: locale.mobileRows.options,
        exact: true,
      })
      await usableDialog(page, options, 'row options dialog')
      await usableMenus(page, options, 'field menu')
      const addField = options.getByRole('button', {
        name: locale.field.panel.add,
        exact: true,
      })
      await expect(addField).toBeVisible()
      await addField.click()
      const field = page.getByRole('dialog', {
        name: locale.field.panel.createTitle,
        exact: true,
      })
      await field
        .getByRole('textbox', { name: locale.field.panel.name, exact: true })
        .fill('Responsive draft')
      await usableDialog(page, field, 'field dialog')
      await field
        .getByRole('button', { name: locale.field.panel.cancel, exact: true })
        .click()
      await expect(field).not.toBeVisible()

      for (const editor of [
        {
          open: locale.relation.add,
          title: locale.relation.createTitle,
        },
        {
          open: locale.field.panel.addFormula,
          title: locale.field.panel.formulaTitle,
        },
      ]) {
        await options
          .getByRole('button', { name: editor.open, exact: true })
          .click()
        const dialog = page.getByRole('dialog', {
          name: editor.title,
          exact: true,
        })
        await usableDialog(page, dialog, editor.title)
        await dialog
          .getByRole('button', { name: locale.field.panel.cancel, exact: true })
          .click()
        await expect(dialog).not.toBeVisible()
      }

      await options
        .getByRole('button', {
          name: locale.mobileRows.closeOptions,
          exact: true,
        })
        .click()
      await expect(options).not.toBeVisible()
      await page
        .getByRole('button', { name: locale.mobileRows.options, exact: true })
        .click()
      const deleteRows = options.getByRole('button', {
        name: locale.grid.deleteRows.replace('{count}', '1'),
        exact: true,
      })
      await expect(deleteRows).toBeEnabled()
      await deleteRows.click()
      await expect(options).not.toBeVisible()
      const confirmation = page.getByRole('dialog', {
        name: locale.grid.confirmDeleteTitle,
        exact: true,
      })
      await usableDialog(page, confirmation, 'row delete confirmation')
      await confirmation
        .getByRole('button', { name: locale.grid.cancel, exact: true })
        .click()
      await expect(confirmation).not.toBeVisible()
      await selectedRow.uncheck()

      await page
        .getByRole('button', { name: locale.grid.addRow, exact: true })
        .click()
      const draft = page.getByRole('dialog', {
        name: locale.mobileRows.newRow,
        exact: true,
      })
      await usableDialog(page, draft, 'new row dialog')
      await draft
        .getByRole('button', { name: locale.rowExpand.close, exact: true })
        .click()
      await expect(draft).not.toBeVisible()

      await page
        .getByRole('button', {
          name: locale.rowExpand.open.replace('{id}', row.id),
          exact: true,
        })
        .click()
      const expanded = page.getByRole('dialog', {
        name: locale.rowExpand.title.replace('{id}', row.id),
        exact: true,
      })
      await usableDialog(page, expanded, 'row dialog')
      const value = locale === en ? 'Responsive English' : 'Responsive Chinese'
      const name = expanded.locator(`[data-field-id="${data.fieldId}"]`)
      await name.getByRole('button').click()
      await name.locator('input').fill(value)
      await name.locator('input').press('Enter')
      await expect(
        expanded.getByText(locale.rowExpand.saved, { exact: true }),
      ).toBeVisible()
      const saved = await page.request.get(
        `/api/v1/tables/${data.tableId}/rows/${row.id}`,
      )
      expect(saved.status()).toBe(200)
      expect(((await saved.json()) as DataRow).fields[data.fieldId]).toBe(value)
      await expanded
        .getByRole('button', { name: locale.rowExpand.close, exact: true })
        .click()
      await expect(expanded).not.toBeVisible()
    }
  } finally {
    await closeDialogsAndRestoreEnglish(page)
  }
})

test('AT-806 form views work at 390px in both languages', async ({
  page,
}, testInfo) => {
  test.setTimeout(150_000)
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/login')
  await english(page)
  await login(page)
  const data = await fixture(page, `form-${testInfo.project.name}`, 1)
  try {
    await page.goto(`/t/${data.tableId}/v/${data.viewId}`)
    const create = page.locator('.view-create')
    await create.getByLabel(en.nav.viewName, { exact: true }).fill('Responsive')
    const type = create.getByRole('combobox', {
      name: en.form.viewType,
      exact: true,
    })
    await expect(type).toBeVisible()
    await type.selectOption('form')
    const [created] = await Promise.all([
      page.waitForResponse(
        (response) =>
          response.request().method() === 'POST' &&
          new URL(response.url()).pathname ===
            `/api/v1/tables/${data.tableId}/views`,
      ),
      create
        .getByRole('button', { name: en.nav.createView, exact: true })
        .click(),
    ])
    expect(created.status()).toBe(201)
    const viewId = ((await created.json()) as ViewMutation).view.id
    const path = `/t/${data.tableId}/v/${viewId}`
    await expect(page).toHaveURL(new RegExp(`${path}$`))

    for (const locale of [en, zh]) {
      if (locale === zh)
        await page
          .getByRole('button', { name: en.common.languages.zhCN, exact: true })
          .click()
      await page.goto(path)
      const builder = page.locator('.form-builder')
      await builder.locator('summary').click()
      await reachableControls(page, 'form builder', builder)
      await noPageOverflow(page, 'form builder')
      const title = locale === en ? 'Responsive English' : 'Responsive Chinese'
      await builder.getByLabel(locale.form.title, { exact: true }).fill(title)
      await builder.getByRole('checkbox', { name: 'Name', exact: true }).check()
      await builder
        .getByLabel(locale.form.fieldLabel, { exact: true })
        .fill('Responsive field')
      await builder.getByLabel(locale.form.required, { exact: true }).check()
      const [saved] = await Promise.all([
        page.waitForResponse(
          (response) =>
            response.request().method() === 'PATCH' &&
            new URL(response.url()).pathname === `/api/v1/views/${viewId}`,
        ),
        builder
          .getByRole('button', { name: locale.form.save, exact: true })
          .click(),
      ])
      expect(saved.status()).toBe(200)
      const form = page.locator('.form-view')
      await expect(form.getByRole('heading', { name: title })).toBeVisible()
      await reachableControls(page, 'form view', form)
      await noPageOverflow(page, 'form view')
      await form
        .getByRole('textbox', { name: 'Responsive field', exact: true })
        .fill(title)
      const [submitted] = await Promise.all([
        page.waitForResponse(
          (response) =>
            response.request().method() === 'POST' &&
            new URL(response.url()).pathname ===
              `/api/v1/views/${viewId}/form-submit`,
        ),
        form
          .getByRole('button', { name: locale.form.submit, exact: true })
          .click(),
      ])
      expect(submitted.ok()).toBe(true)
      expect(
        ((await submitted.json()) as RowPatchMutation).row.fields[data.fieldId],
      ).toBe(title)
      await expect(form.getByRole('status')).toHaveText(locale.form.submitted)
    }
  } finally {
    await closeDialogsAndRestoreEnglish(page)
  }
})

test('load harness waits for the grid source before scrolling', async ({
  page,
}) => {
  await page.setContent(
    '<revo-grid style="display:block;width:100%;height:300px">' +
      '<revogr-data col-type="colPinStart"><button></button></revogr-data>' +
      '</revo-grid>',
  )
  const grid = page.locator('revo-grid')
  await grid.evaluate(
    (element, rowLabel) => {
      const source = () =>
        element.hasAttribute('data-test-source-ready')
          ? Array.from({ length: 301 }, (_, index) =>
              index === 300 ? { __rowId: 'test-row' } : {},
            )
          : []
      const button = element.querySelector('button')
      if (!button) throw new Error('Missing test row button')
      button.setAttribute('aria-label', rowLabel)
      Object.assign(element, {
        async getSource() {
          element.setAttribute('data-test-source-read', 'true')
          return source()
        },
        async scrollToRow(target: number) {
          element.setAttribute('data-test-source-read', 'true')
          const rowCount = source().length
          if (rowCount <= target)
            throw new Error('Load harness scrolled before grid initialization')
          element.setAttribute('data-test-scroll-count', String(rowCount))
        },
      })
    },
    en.rowExpand.open.replace('{id}', 'test-row'),
  )
  const result = scrollToLoaded(page, 300, 'test-row').then(
    () => null,
    (error: unknown) => error,
  )
  try {
    await expect(grid).toHaveAttribute('data-test-source-read', 'true')
  } finally {
    await grid.evaluate((element) =>
      element.setAttribute('data-test-source-ready', 'true'),
    )
  }
  expect(await result).toBeNull()
  await expect(grid).toHaveAttribute('data-test-scroll-count', '301')
})

test('load harness recovers when the target viewport resets', async ({
  page,
}) => {
  test.setTimeout(30_000)
  await page.setContent(
    '<revo-grid style="display:block;width:100%;height:300px">' +
      '<revogr-data col-type="colPinStart" style="display:block;height:100%">' +
      '<div class="rgRow" data-rgrow="0" style="height:32px"><button></button></div>' +
      '</revogr-data></revo-grid>',
  )
  const grid = page.locator('revo-grid')
  await grid.evaluate(
    (element, rowLabel) => {
      const row = element.querySelector('.rgRow')
      const button = element.querySelector('button')
      if (!row || !button) throw new Error('Missing test row')
      let scrolls = 0
      let loaded = false
      Object.assign(element, {
        async getSource() {
          return Array.from({ length: 301 }, (_, index) =>
            index === 300 && loaded ? { __rowId: 'test-row' } : {},
          )
        },
        async scrollToRow(target: number) {
          scrolls++
          element.setAttribute('data-test-scroll-count', String(scrolls))
          row.setAttribute('data-rgrow', String(target))
          if (scrolls > 1) {
            loaded = true
            button.setAttribute('aria-label', rowLabel)
          }
        },
      })
    },
    en.rowExpand.open.replace('{id}', 'test-row'),
  )
  const result = scrollToLoaded(page, 300, 'test-row').then(
    () => null,
    (error: unknown) => error,
  )
  await expect(grid).toHaveAttribute('data-test-scroll-count', '1')
  await page.waitForTimeout(700)
  await expect(grid).toHaveAttribute('data-test-scroll-count', '1')
  await grid
    .locator('.rgRow')
    .evaluate((row) => row.setAttribute('data-rgrow', '0'))
  expect(await result).toBeNull()
  await expect(grid).toHaveAttribute('data-test-scroll-count', '2')
})

test('load harness scrolls, edits, receives peer events, and reads both metrics', async ({
  page,
  browser,
  isMobile,
}, testInfo) => {
  test.skip(isMobile, 'Load harness drives the desktop grid')
  test.setTimeout(240_000)
  await page.setViewportSize({ width: 1440, height: 900 })
  const adminId = await login(page)
  const data = await fixture(page, `load-smoke-${testInfo.project.name}`, 600)
  const username = `load-smoke-${Date.now()}`
  const created = await page.request.post('/api/v1/admin/users', {
    headers: headers(),
    data: {
      username,
      displayName: 'Load smoke',
      role: 'user',
      password: 'Temporary-password-123',
    },
  })
  expect(created.status()).toBe(201)
  const peerId = ((await created.json()) as AuthUserMutation).user.id
  const grant = await page.request.put('/api/v1/grants', {
    headers: headers(),
    data: {
      resourceType: 'table',
      resourceId: data.tableId,
      principalType: 'user',
      principalId: peerId,
      level: 'write',
    },
  })
  expect(grant.status()).toBe(201)
  const peerContext = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL,
    viewport: { width: 1440, height: 900 },
    locale: 'en-US',
  })
  const admin = {
    userId: adminId,
    subscribed: false,
    stopping: false,
    failure: '',
    received: 0,
  }
  const peer = {
    userId: peerId,
    subscribed: false,
    stopping: false,
    failure: '',
    received: 0,
  }
  let targetQueries = 0
  const countTargetQuery = (request: Request) => {
    if (
      request.method() !== 'POST' ||
      new URL(request.url()).pathname !== `/api/v1/views/${data.viewId}/query`
    )
      return
    const body = request.postDataJSON() as { offset?: number }
    if (body.offset === 300) targetQueries++
  }
  try {
    const peerPage = await peerContext.newPage()
    await login(peerPage, username, 'Temporary-password-123')
    const changed = await peerPage.request.post('/api/v1/auth/password', {
      headers: headers(),
      data: {
        current: 'Temporary-password-123',
        new: 'Changed-password-123',
      },
    })
    expect(changed.status()).toBe(200)
    expect(await login(peerPage, username, 'Changed-password-123')).toBe(peerId)
    trackPage(page, admin, data.tableId)
    trackPage(peerPage, peer, data.tableId)
    const path = `/t/${data.tableId}/v/${data.viewId}`
    await Promise.all([page.goto(path), peerPage.goto(path)])
    await Promise.all([waitForSubscription(admin), waitForSubscription(peer)])
    const grid = page.locator('revo-grid')
    await expect
      .poll(
        () =>
          grid.evaluate(async (element) => {
            const component = element as HTMLElement & {
              getSource?: () => Promise<unknown[]>
            }
            return component.getSource
              ? (await component.getSource()).length
              : 0
          }),
        { timeout: 20_000 },
      )
      .toBeGreaterThan(300)
    page.on('request', countTargetQuery)
    await grid.evaluate((element) => {
      const component = element as HTMLElement & {
        scrollToRow: (index: number) => Promise<void>
      }
      let reset = false
      element.addEventListener('viewportscroll', (event) => {
        const detail = (
          event as CustomEvent<{ dimension: string; coordinate: number }>
        ).detail
        if (reset || detail.dimension !== 'rgRow' || detail.coordinate < 9600)
          return
        reset = true
        element.setAttribute('data-test-viewport-reset', 'true')
        void component.scrollToRow(0)
      })
    })
    for (const [rowIndex, scrollIndex, value] of [
      [0, 300, 'load-smoke'],
      [9, 500, 'load-smoke-2'],
      [0, 400, 'load-smoke-3'],
    ] as const) {
      const row = data.rows[rowIndex]
      if (!row) throw new Error('Fixture did not create the load-test row')
      await exerciseOnce(rowIndex === 9 ? peerPage : page, {
        tableId: data.tableId,
        fieldId: data.fieldId,
        rowIndex,
        rowId: row.id,
        scrollIndex,
        value,
      })
      if (scrollIndex === 300) {
        await expect(grid).toHaveAttribute('data-test-viewport-reset', 'true')
        await expect.poll(() => targetQueries).toBeGreaterThanOrEqual(2)
      }
      const saved = await page.request.get(
        `/api/v1/tables/${data.tableId}/rows/${row.id}`,
      )
      expect(saved.status()).toBe(200)
      expect(((await saved.json()) as DataRow).fields[data.fieldId]).toBe(value)
    }
    await expect.poll(() => admin.received).toBeGreaterThan(0)
    await expect.poll(() => peer.received).toBeGreaterThan(0)
    expect(admin.failure).toBe('')
    expect(peer.failure).toBe('')
    const metrics = await page.request.get('/api/v1/admin/system')
    expect(metrics.status()).toBe(200)
    const sample = readMemorySample(await metrics.json(), 0)
    expect(sample.appRssBytes).toBeGreaterThan(0)
    expect(sample.hostUsedBytes).toBeGreaterThan(0)
  } finally {
    page.off('request', countTargetQuery)
    admin.stopping = true
    peer.stopping = true
    await peerContext.close()
  }
})
