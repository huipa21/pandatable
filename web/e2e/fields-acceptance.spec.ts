import type { Page } from '@playwright/test'
import type { Components } from '@revolist/revogrid'
import { test, expect, expectHTTPFailure, paceLogin } from './fixtures'
import type { DataRow, RowQueryResult, TableSchema } from '../src/api/types'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }

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
  await page.goto('/login')
  if ((await page.locator('html').getAttribute('lang')) === 'zh-CN')
    await page
      .getByRole('button', { name: zh.common.languages.en, exact: true })
      .click()
  await page.getByLabel(en.auth.login, { exact: true }).fill(username)
  await page.getByLabel(en.auth.password, { exact: true }).fill(password)
  await page.getByRole('button', { name: en.auth.signIn, exact: true }).click()
  await expect(page).toHaveURL(/\/home$/)
}

async function createResource(
  page: Page,
  kind: 'workspace' | 'book' | 'table',
  name: string,
) {
  await page
    .locator('main')
    .getByRole('button', { name: en.nav.create[kind], exact: true })
    .click()
  const dialog = page.getByRole('dialog', {
    name: en.nav.dialogTitle,
    exact: true,
  })
  await expect(dialog).toBeVisible()
  await dialog
    .getByRole('textbox', { name: en.nav.name, exact: true })
    .fill(name)
  await dialog
    .getByRole('button', { name: en.admin.confirm, exact: true })
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

async function schemaOf(page: Page, tid: string) {
  const response = await page.request.get(`/api/v1/tables/${tid}/schema`)
  expect(response.status()).toBe(200)
  return (await response.json()) as TableSchema
}

async function queryOf(page: Page, viewId: string) {
  const response = await page.request.post(`/api/v1/views/${viewId}/query`, {
    headers: headers(),
    data: { offset: 0, limit: 100, includeCount: true },
  })
  expect(response.status()).toBe(200)
  return (await response.json()) as RowQueryResult
}

async function addFieldViaUI(
  page: Page,
  spec: {
    name: string
    kind: string
    size?: string
    precision?: string
    scale?: string
    choices?: string[]
  },
) {
  const panel = page.getByRole('region', { name: en.field.panel.title })
  await panel
    .getByRole('button', { name: en.field.panel.add, exact: true })
    .click()
  const editor = page.getByRole('dialog', {
    name: en.field.panel.createTitle,
    exact: true,
  })
  await expect(editor).toBeVisible()
  await editor
    .getByRole('textbox', { name: en.field.panel.name, exact: true })
    .fill(spec.name)
  await editor
    .getByRole('combobox', { name: en.field.panel.kind, exact: true })
    .selectOption(spec.kind)
  if (spec.size) {
    await editor
      .getByRole('combobox', { name: en.field.panel.size, exact: true })
      .selectOption(spec.size)
  }
  if (spec.precision) {
    await editor
      .getByRole('textbox', { name: en.field.panel.precision, exact: true })
      .fill(spec.precision)
  }
  if (spec.scale) {
    await editor
      .getByRole('textbox', { name: en.field.panel.scale, exact: true })
      .fill(spec.scale)
  }
  if (spec.choices) {
    for (const [index, label] of spec.choices.entries()) {
      await editor
        .getByRole('button', { name: en.field.panel.addChoice, exact: true })
        .click()
      await editor
        .getByRole('textbox', {
          name: en.field.panel.choiceLabel.replace('{n}', String(index + 1)),
          exact: true,
        })
        .fill(label)
    }
  }
  await editor
    .getByRole('button', { name: en.field.panel.create, exact: true })
    .click()
  await expect(editor).not.toBeVisible()
  await expect(
    page.getByRole('status').filter({ hasText: en.field.panel.saved }),
  ).toBeVisible()
}

async function openCellEditor(page: Page, field: { id: string }, rowIndex = 0) {
  const gridElement = page.locator('revo-grid')
  await gridElement.evaluate(
    async (element, { fieldId, row }) => {
      const grid = element as HTMLElement & Components.RevoGrid
      await grid.scrollToRow(row)
      const columns = await grid.getColumns()
      const column = columns.find((c) => c.prop === fieldId)
      if (!column) throw new Error(`Column ${fieldId} not found`)
      const type = (column.pin ?? 'rgCol') as
        'rgCol' | 'colPinStart' | 'colPinEnd'
      if (type === 'rgCol' && typeof grid.scrollToColumnProp === 'function') {
        await grid.scrollToColumnProp(fieldId)
      }
      const store = await grid.getColumnStore(type)
      const source = store.get('source')
      const items = store.get('items')
      const x = items.findIndex((index) => source[index]?.prop === fieldId)
      const safeX = Math.max(0, x)
      await grid.setCellsFocus({ x: safeX, y: row }, { x: safeX, y: row }, type)
      grid.focus()
      await grid.setCellEdit(row, fieldId)
    },
    { fieldId: field.id, row: rowIndex },
  )
}

test('AT-301..304 complete field acceptance: 12 M1 kinds, grid persistence, validation, constraints, system fields and authorship', async ({
  page,
  isMobile,
}) => {
  test.setTimeout(150_000)
  if (isMobile) {
    await page.goto('/login')
    await login(page)
    await expect(page).toHaveURL(/\/home$/)
    return
  }
  await page.setViewportSize({ width: 1440, height: 900 })
  await login(page)

  const suffix = Date.now()
  await createResource(page, 'workspace', `Acceptance W ${suffix}`)
  await createResource(page, 'book', `Acceptance B ${suffix}`)
  await createResource(page, 'table', `Acceptance T ${suffix}`)

  const parts = new URL(page.url()).pathname.split('/')
  const tableId = parts[2]
  const viewId = parts[4]
  if (!tableId || !viewId) throw new Error('Table route incomplete')

  // Unfreeze columns so scrolling is uniform across all columns.
  const initialSchema = await schemaOf(page, tableId)
  await page.request.patch(`/api/v1/views/${viewId}`, {
    headers: headers(),
    data: { config: { ...initialSchema.views[0]!.config, frozen: 0 } },
  })
  await page.reload()

  // Initial table starts with "Name" (text).
  // Owner adds the remaining 11 kinds through the Field UI (plus a custom text field to cover all 12 additions).
  await addFieldViaUI(page, { name: 'Notes', kind: 'long_text' })
  await addFieldViaUI(page, { name: 'Qty', kind: 'integer', size: 'int' })
  await addFieldViaUI(page, {
    name: 'Price',
    kind: 'decimal',
    precision: '10',
    scale: '2',
  })
  await addFieldViaUI(page, { name: 'Score', kind: 'float', size: 'real' })
  await addFieldViaUI(page, { name: 'Active', kind: 'checkbox' })
  await addFieldViaUI(page, { name: 'DueDate', kind: 'date' })
  await addFieldViaUI(page, { name: 'EventAt', kind: 'datetime' })
  await addFieldViaUI(page, {
    name: 'Stage',
    kind: 'single_select',
    choices: ['Todo', 'Done'],
  })
  await addFieldViaUI(page, {
    name: 'Tags',
    kind: 'multi_select',
    choices: ['Alpha', 'Beta'],
  })
  await addFieldViaUI(page, { name: 'Contact', kind: 'email' })
  await addFieldViaUI(page, { name: 'Website', kind: 'url' })
  await addFieldViaUI(page, { name: 'Code', kind: 'text' })

  const schema = await schemaOf(page, tableId)
  const fieldByName = (n: string) => schema.fields.find((f) => f.name === n)!

  const nameField = fieldByName('Name')
  const notesField = fieldByName('Notes')
  const qtyField = fieldByName('Qty')
  const priceField = fieldByName('Price')
  const scoreField = fieldByName('Score')
  const activeField = fieldByName('Active')
  const dueField = fieldByName('DueDate')
  const eventField = fieldByName('EventAt')
  const stageField = fieldByName('Stage')
  const tagsField = fieldByName('Tags')
  const contactField = fieldByName('Contact')
  const websiteField = fieldByName('Website')
  const codeField = fieldByName('Code')

  // Add a row through the grid.
  const gridRegion = page.getByRole('region', { name: en.grid.title })
  await expect(gridRegion).toBeVisible()
  const addRowBtn = page.getByRole('button', {
    name: en.grid.addRow,
    exact: true,
  })
  await addRowBtn.click()
  await expect(addRowBtn).toBeEnabled()
  await expect(
    page.locator('revo-grid').getByRole('row', { name: /Select row 1/ }),
  ).toBeVisible()
  await expect.poll(async () => (await queryOf(page, viewId)).total).toBe(1)

  // Enter values for all 12 kinds through the grid.
  // 1. Name (text)
  await openCellEditor(page, nameField)
  let textInput = page.getByRole('textbox', {
    name: en.field.editor.value,
    exact: true,
  })
  await expect(textInput).toBeVisible()
  await textInput.fill('Widget One')
  await textInput.press('Enter')
  await expect(textInput).not.toBeVisible()
  await expect
    .poll(
      async () => (await queryOf(page, viewId)).rows[0]?.fields[nameField.id],
    )
    .toBe('Widget One')

  // 2. Code (text)
  await openCellEditor(page, codeField)
  textInput = page.getByRole('textbox', {
    name: en.field.editor.value,
    exact: true,
  })
  await expect(textInput).toBeVisible()
  await textInput.fill('W-001')
  await textInput.press('Enter')
  await expect(textInput).not.toBeVisible()
  await expect
    .poll(
      async () => (await queryOf(page, viewId)).rows[0]?.fields[codeField.id],
    )
    .toBe('W-001')

  // 3. Notes (long_text)
  await openCellEditor(page, notesField)
  const textarea = page.getByRole('textbox', {
    name: en.field.editor.value,
    exact: true,
  })
  await expect(textarea).toBeVisible()
  await textarea.fill('First line of notes')
  await textarea.press('Control+Enter')
  await expect(textarea).not.toBeVisible()
  await expect
    .poll(
      async () => (await queryOf(page, viewId)).rows[0]?.fields[notesField.id],
    )
    .toBe('First line of notes')

  // 4. Qty (integer)
  await openCellEditor(page, qtyField)
  textInput = page.getByRole('textbox', {
    name: en.field.editor.value,
    exact: true,
  })
  await expect(textInput).toBeVisible()
  await textInput.fill('42')
  await textInput.press('Enter')
  await expect(textInput).not.toBeVisible()
  await expect
    .poll(
      async () => (await queryOf(page, viewId)).rows[0]?.fields[qtyField.id],
    )
    .toBe(42)

  // 5. Price (decimal)
  await openCellEditor(page, priceField)
  textInput = page.getByRole('textbox', {
    name: en.field.editor.value,
    exact: true,
  })
  await expect(textInput).toBeVisible()
  await textInput.fill('99.95')
  await textInput.press('Enter')
  await expect(textInput).not.toBeVisible()
  await expect
    .poll(
      async () => (await queryOf(page, viewId)).rows[0]?.fields[priceField.id],
    )
    .toBe('99.95')

  // 6. Score (float)
  await openCellEditor(page, scoreField)
  textInput = page.getByRole('textbox', {
    name: en.field.editor.value,
    exact: true,
  })
  await expect(textInput).toBeVisible()
  await textInput.fill('4.5')
  await textInput.press('Enter')
  await expect(textInput).not.toBeVisible()
  await expect
    .poll(
      async () => (await queryOf(page, viewId)).rows[0]?.fields[scoreField.id],
    )
    .toBe(4.5)

  // 7. Active (checkbox)
  await openCellEditor(page, activeField)
  const checkbox = page.getByRole('checkbox', {
    name: en.field.editor.value,
    exact: true,
  })
  await expect(checkbox).toBeVisible()
  await checkbox.check()
  await expect(checkbox).not.toBeVisible()
  await expect
    .poll(
      async () => (await queryOf(page, viewId)).rows[0]?.fields[activeField.id],
    )
    .toBe(true)

  // 8. DueDate (date)
  await openCellEditor(page, dueField)
  textInput = page.getByRole('textbox', {
    name: en.field.editor.value,
    exact: true,
  })
  await expect(textInput).toBeVisible()
  await textInput.fill('2026-11-20')
  await textInput.press('Enter')
  await expect(textInput).not.toBeVisible()
  await expect
    .poll(
      async () => (await queryOf(page, viewId)).rows[0]?.fields[dueField.id],
    )
    .toBe('2026-11-20')

  // 9. EventAt (datetime)
  await openCellEditor(page, eventField)
  textInput = page.getByRole('textbox', {
    name: en.field.editor.value,
    exact: true,
  })
  await expect(textInput).toBeVisible()
  await textInput.fill('2026-11-20T10:00:00Z')
  await textInput.press('Enter')
  await expect(textInput).not.toBeVisible()
  await expect
    .poll(
      async () => (await queryOf(page, viewId)).rows[0]?.fields[eventField.id],
    )
    .toBe('2026-11-20T10:00:00Z')

  // 10. Stage (single_select)
  await openCellEditor(page, stageField)
  const singleSelect = page.getByRole('combobox', {
    name: en.field.editor.value,
    exact: true,
  })
  await expect(singleSelect).toBeVisible()
  await singleSelect.selectOption({ label: 'Done' })
  await expect(singleSelect).not.toBeVisible()
  await expect
    .poll(
      async () => (await queryOf(page, viewId)).rows[0]?.fields[stageField.id],
    )
    .toBe('Done')

  // 11. Tags (multi_select)
  await openCellEditor(page, tagsField)
  const multiSelect = page.locator('.field-editor select[multiple]')
  await expect(multiSelect).toBeVisible()
  await multiSelect.selectOption(['Alpha'])
  await page
    .getByRole('button', { name: en.field.editor.apply, exact: true })
    .click()
  await expect(multiSelect).not.toBeVisible()
  await expect
    .poll(
      async () => (await queryOf(page, viewId)).rows[0]?.fields[tagsField.id],
    )
    .toEqual(['Alpha'])

  // 12. Contact (email)
  await openCellEditor(page, contactField)
  textInput = page.getByRole('textbox', {
    name: en.field.editor.value,
    exact: true,
  })
  await expect(textInput).toBeVisible()
  await textInput.fill('valid@example.test')
  await textInput.press('Enter')
  await expect(textInput).not.toBeVisible()
  await expect
    .poll(
      async () =>
        (await queryOf(page, viewId)).rows[0]?.fields[contactField.id],
    )
    .toBe('valid@example.test')

  // 13. Website (url)
  await openCellEditor(page, websiteField)
  textInput = page.getByRole('textbox', {
    name: en.field.editor.value,
    exact: true,
  })
  await expect(textInput).toBeVisible()
  await textInput.fill('https://pandatable.test')
  await textInput.press('Enter')
  await expect(textInput).not.toBeVisible()
  await expect
    .poll(
      async () =>
        (await queryOf(page, viewId)).rows[0]?.fields[websiteField.id],
    )
    .toBe('https://pandatable.test')

  // Verify all entered values persist in query results before reload.
  await expect
    .poll(async () => {
      const q = await queryOf(page, viewId)
      return q.rows[0]?.fields[qtyField.id]
    })
    .toBe(42)

  let rows = await queryOf(page, viewId)
  expect(rows.rows[0]!.fields[nameField.id]).toBe('Widget One')
  expect(rows.rows[0]!.fields[codeField.id]).toBe('W-001')
  expect(rows.rows[0]!.fields[notesField.id]).toBe('First line of notes')
  expect(rows.rows[0]!.fields[qtyField.id]).toBe(42)
  expect(rows.rows[0]!.fields[priceField.id]).toBe('99.95')
  expect(rows.rows[0]!.fields[scoreField.id]).toBe(4.5)
  expect(rows.rows[0]!.fields[activeField.id]).toBe(true)
  expect(rows.rows[0]!.fields[dueField.id]).toBe('2026-11-20')
  expect(rows.rows[0]!.fields[eventField.id]).toBe('2026-11-20T10:00:00Z')
  expect(rows.rows[0]!.fields[stageField.id]).toBe('Done')
  expect(rows.rows[0]!.fields[tagsField.id]).toEqual(['Alpha'])
  expect(rows.rows[0]!.fields[contactField.id]).toBe('valid@example.test')
  expect(rows.rows[0]!.fields[websiteField.id]).toBe('https://pandatable.test')

  // Reload page and confirm all values persist across reload (AT-301).
  await page.reload()
  await expect(gridRegion).toBeVisible()

  rows = await queryOf(page, viewId)
  expect(rows.rows[0]!.fields[nameField.id]).toBe('Widget One')
  expect(rows.rows[0]!.fields[codeField.id]).toBe('W-001')
  expect(rows.rows[0]!.fields[notesField.id]).toBe('First line of notes')
  expect(rows.rows[0]!.fields[qtyField.id]).toBe(42)
  expect(rows.rows[0]!.fields[priceField.id]).toBe('99.95')
  expect(rows.rows[0]!.fields[scoreField.id]).toBe(4.5)
  expect(rows.rows[0]!.fields[activeField.id]).toBe(true)
  expect(rows.rows[0]!.fields[dueField.id]).toBe('2026-11-20')
  expect(rows.rows[0]!.fields[eventField.id]).toBe('2026-11-20T10:00:00Z')
  expect(rows.rows[0]!.fields[stageField.id]).toBe('Done')
  expect(rows.rows[0]!.fields[tagsField.id]).toEqual(['Alpha'])
  expect(rows.rows[0]!.fields[contactField.id]).toBe('valid@example.test')
  expect(rows.rows[0]!.fields[websiteField.id]).toBe('https://pandatable.test')

  // AT-302: Validation checks for invalid integer and email values.
  // "abc" in integer cell shows cell error and saves nothing.
  await openCellEditor(page, qtyField)
  textInput = page.getByRole('textbox', {
    name: en.field.editor.value,
    exact: true,
  })
  await expect(textInput).toBeVisible()
  await textInput.fill('abc')
  await textInput.press('Enter')
  await expect(
    page.getByText(en.field.error.invalid_integer, { exact: true }),
  ).toBeVisible()
  rows = await queryOf(page, viewId)
  expect(rows.rows[0]!.fields[qtyField.id]).toBe(42)
  await page.keyboard.press('Escape')

  // "x@" in email cell shows cell error and saves nothing.
  await openCellEditor(page, contactField)
  textInput = page.getByRole('textbox', {
    name: en.field.editor.value,
    exact: true,
  })
  await expect(textInput).toBeVisible()
  await textInput.fill('x@')
  await textInput.press('Enter')
  await expect(
    page.getByText(en.field.error.invalid_email, { exact: true }),
  ).toBeVisible()
  rows = await queryOf(page, viewId)
  expect(rows.rows[0]!.fields[contactField.id]).toBe('valid@example.test')
  await page.keyboard.press('Escape')

  // Clean up created resources.
  await page.request.delete(`/api/v1/tables/${tableId}`, { headers: headers() })
})

test('AT-303: required and unique constraints enforcement and defaults on new rows', async ({
  page,
  isMobile,
}) => {
  test.setTimeout(150_000)
  if (isMobile) {
    await page.goto('/login')
    await login(page)
    await expect(page).toHaveURL(/\/home$/)
    return
  }
  await page.setViewportSize({ width: 1440, height: 900 })
  await login(page)

  const suffix = Date.now()
  await createResource(page, 'workspace', `Constraints W ${suffix}`)
  await createResource(page, 'book', `Constraints B ${suffix}`)
  await createResource(page, 'table', `Constraints T ${suffix}`)

  const parts = new URL(page.url()).pathname.split('/')
  const tableId = parts[2]
  const viewId = parts[4]
  if (!tableId || !viewId) throw new Error('Table route incomplete')

  // Unfreeze columns so scrolling is uniform.
  const initialSchema = await schemaOf(page, tableId)
  await page.request.patch(`/api/v1/views/${viewId}`, {
    headers: headers(),
    data: { config: { ...initialSchema.views[0]!.config, frozen: 0 } },
  })
  await page.reload()

  // Add 2 rows to the table before adding the target field.
  const gridRegion = page.getByRole('region', { name: en.grid.title })
  await expect(gridRegion).toBeVisible()
  await page.getByRole('button', { name: en.grid.addRow, exact: true }).click()
  await page.getByRole('button', { name: en.grid.addRow, exact: true }).click()
  await expect.poll(async () => (await queryOf(page, viewId)).total).toBe(2)

  // Add "Serial" text field via Field UI (both rows have empty cells).
  await addFieldViaUI(page, { name: 'Serial', kind: 'text' })
  let schema = await schemaOf(page, tableId)
  const serialField = schema.fields.find((f) => f.name === 'Serial')!

  const panel = page.getByRole('region', { name: en.field.panel.title })
  const fieldItem = panel.locator(`[data-field-id="${serialField.id}"]`)
  const actionsMenu = fieldItem.locator('details.field-actions')
  await actionsMenu.locator('summary').click()
  await actionsMenu
    .getByRole('button', { name: en.field.panel.edit, exact: true })
    .click()

  let editDialog = page.getByRole('dialog', {
    name: en.field.panel.editTitle,
    exact: true,
  })
  await expect(editDialog).toBeVisible()

  // Setting required on empty cells without default/fillEmpty fails with 409 ERR_REQUIRED_HAS_EMPTY (count = 2).
  expectHTTPFailure(page, 'PATCH', `/api/v1/fields/${serialField.id}`, 409)
  await editDialog
    .getByRole('checkbox', { name: en.field.panel.required, exact: true })
    .check()
  await editDialog
    .getByRole('button', { name: en.field.panel.save, exact: true })
    .click()
  await expect(editDialog.getByRole('alert')).toContainText(
    en.errors.ERR_REQUIRED_HAS_EMPTY.replace('{count}', '2'),
  )

  // User can abort (cancel).
  await editDialog
    .getByRole('button', { name: en.field.panel.cancel, exact: true })
    .click()
  await expect(editDialog).not.toBeVisible()

  // Opening edit again and providing default + fillEmpty succeeds.
  await actionsMenu.locator('summary').click()
  await actionsMenu
    .getByRole('button', { name: en.field.panel.edit, exact: true })
    .click()
  editDialog = page.getByRole('dialog', {
    name: en.field.panel.editTitle,
    exact: true,
  })
  await expect(editDialog).toBeVisible()
  await editDialog
    .getByRole('checkbox', { name: en.field.panel.required, exact: true })
    .check()
  await editDialog
    .getByRole('textbox', { name: en.field.panel.defaultValue, exact: true })
    .fill('DEF-VAL')
  await editDialog
    .getByRole('checkbox', { name: en.field.panel.fillEmpty, exact: true })
    .check()
  await editDialog
    .getByRole('button', { name: en.field.panel.save, exact: true })
    .click()
  await expect(editDialog).not.toBeVisible()
  await expect(
    page.getByRole('status').filter({ hasText: en.field.panel.saved }),
  ).toBeVisible()

  // Both rows now have "DEF-VAL".
  let rows = await queryOf(page, viewId)
  expect(rows.rows[0]!.fields[serialField.id]).toBe('DEF-VAL')
  expect(rows.rows[1]!.fields[serialField.id]).toBe('DEF-VAL')

  // Setting unique while duplicate values exist fails with 409 ERR_UNIQUE_HAS_DUPLICATES (1 duplicate).
  await actionsMenu.locator('summary').click()
  await actionsMenu
    .getByRole('button', { name: en.field.panel.edit, exact: true })
    .click()
  editDialog = page.getByRole('dialog', {
    name: en.field.panel.editTitle,
    exact: true,
  })
  await expect(editDialog).toBeVisible()
  expectHTTPFailure(page, 'PATCH', `/api/v1/fields/${serialField.id}`, 409)
  await editDialog
    .getByRole('checkbox', { name: en.field.panel.unique, exact: true })
    .check()
  await editDialog
    .getByRole('button', { name: en.field.panel.save, exact: true })
    .click()
  await expect(editDialog.getByRole('alert')).toContainText(
    en.errors.ERR_UNIQUE_HAS_DUPLICATES.replace('{count}', '1'),
  )
  await editDialog
    .getByRole('button', { name: en.field.panel.cancel, exact: true })
    .click()
  await expect(editDialog).not.toBeVisible()

  // Make row 2's Serial distinct so there are no duplicates.
  await openCellEditor(page, serialField, 1)
  let serialInput = page.getByRole('textbox', {
    name: en.field.editor.value,
    exact: true,
  })
  await expect(serialInput).toBeVisible()
  await serialInput.fill('SN-002')
  await serialInput.press('Enter')
  await expect(serialInput).not.toBeVisible()

  await expect
    .poll(
      async () => (await queryOf(page, viewId)).rows[1]?.fields[serialField.id],
    )
    .toBe('SN-002')

  // Now setting unique succeeds.
  await actionsMenu.locator('summary').click()
  await actionsMenu
    .getByRole('button', { name: en.field.panel.edit, exact: true })
    .click()
  editDialog = page.getByRole('dialog', {
    name: en.field.panel.editTitle,
    exact: true,
  })
  await expect(editDialog).toBeVisible()
  await editDialog
    .getByRole('checkbox', { name: en.field.panel.unique, exact: true })
    .check()
  await editDialog
    .getByRole('button', { name: en.field.panel.save, exact: true })
    .click()
  await expect(editDialog).not.toBeVisible()

  schema = await schemaOf(page, tableId)
  expect(schema.fields.find((f) => f.name === 'Serial')!.isUnique).toBe(true)

  // Unique constraint rejects duplicate write with translated error.
  expectHTTPFailure(
    page,
    'PATCH',
    `/api/v1/tables/${tableId}/rows/${rows.rows[1]!.id}`,
    409,
  )
  await openCellEditor(page, serialField, 1)
  serialInput = page.getByRole('textbox', {
    name: en.field.editor.value,
    exact: true,
  })
  await expect(serialInput).toBeVisible()
  await serialInput.fill('DEF-VAL')
  await serialInput.press('Enter')
  await expect(page.getByRole('alert').first()).toContainText(
    en.errors.ERR_UNIQUE_CONFLICT,
  )
  await page.keyboard.press('Escape')

  // Prepare row 1 to have unique "SN-001" so "DEF-VAL" is free for new row default.
  await page.request.patch(
    `/api/v1/tables/${tableId}/rows/${rows.rows[0]!.id}`,
    {
      headers: headers(),
      data: { fields: { [serialField.id]: 'SN-001' } },
    },
  )
  await page.reload()
  await expect(gridRegion).toBeVisible()

  // A default is applied to a newly added row.
  const addBtn = page.getByRole('button', { name: en.grid.addRow, exact: true })
  await addBtn.click()
  await expect(addBtn).toBeEnabled()
  await expect.poll(async () => (await queryOf(page, viewId)).total).toBe(3)
  rows = await queryOf(page, viewId)
  expect(rows.rows[2]!.fields[serialField.id]).toBe('DEF-VAL')

  await page.request.delete(`/api/v1/tables/${tableId}`, { headers: headers() })
})

test('AT-304: system fields hidden by default, revealed via Fields control, authorship updated_by tracking', async ({
  page,
  browser,
  isMobile,
}) => {
  test.setTimeout(150_000)
  if (isMobile) {
    await page.goto('/login')
    await login(page)
    await expect(page).toHaveURL(/\/home$/)
    return
  }
  await page.setViewportSize({ width: 1440, height: 900 })
  await login(page)

  const suffix = Date.now()
  await createResource(page, 'workspace', `System W ${suffix}`)
  await createResource(page, 'book', `System B ${suffix}`)
  await createResource(page, 'table', `System T ${suffix}`)

  const parts = new URL(page.url()).pathname.split('/')
  const tableId = parts[2]
  const viewId = parts[4]
  if (!tableId || !viewId) throw new Error('Table route incomplete')

  const gridElement = page.locator('revo-grid')
  await expect(gridElement).toBeVisible()

  const schema = await schemaOf(page, tableId)
  const systemFields = schema.fields.filter((f) => f.isSystem)

  // A new view hides the five system fields (id, created_at, created_by, updated_at, updated_by).
  for (const sys of systemFields) {
    await expect(
      gridElement.getByRole('columnheader', { name: sys.name, exact: true }),
    ).toHaveCount(0)
  }

  // The Fields control reveals them.
  const fieldsMenu = page.locator('details.grid-fields')
  await fieldsMenu.locator('summary').click()
  for (const sys of systemFields) {
    await fieldsMenu
      .getByRole('checkbox', { name: sys.name, exact: true })
      .check()
    const header = gridElement.getByRole('columnheader', {
      name: sys.name,
      exact: true,
    })
    await expect(header).toBeVisible()
  }
  await fieldsMenu.locator('summary').click()

  // Add a row created by Alice (first-admin).
  await page.getByRole('button', { name: en.grid.addRow, exact: true }).click()
  await expect.poll(async () => (await queryOf(page, viewId)).total).toBe(1)

  const me = (await (await page.request.get('/api/v1/auth/me')).json()) as {
    id: string
  }
  const nameField = schema.fields.find((f) => f.uiType === 'text')!
  const idField = schema.fields.find((f) => f.uiType === 'id')!
  const createdByField = schema.fields.find((f) => f.uiType === 'created_by')!
  const updatedByField = schema.fields.find((f) => f.uiType === 'updated_by')!

  const rows = await queryOf(page, viewId)
  const rowId = rows.rows[0]!.id

  const rowRes = await page.request.get(
    `/api/v1/tables/${tableId}/rows/${rowId}`,
  )
  const row = (await rowRes.json()) as DataRow
  expect(row.fields[idField.id]).toBe(rowId)
  expect(row.fields[createdByField.id]).toBe(me.id)
  expect(row.fields[updatedByField.id]).toBe(me.id)

  // Querying with system fieldIds explicitly returns them in view query too.
  const queryWithSys = await page.request.post(
    `/api/v1/views/${viewId}/query`,
    {
      headers: headers(),
      data: {
        fieldIds: [
          nameField.id,
          idField.id,
          createdByField.id,
          updatedByField.id,
        ],
      },
    },
  )
  const viewRows = (await queryWithSys.json()) as RowQueryResult
  expect(viewRows.rows[0]!.fields[idField.id]).toBe(rowId)
  expect(viewRows.rows[0]!.fields[createdByField.id]).toBe(me.id)
  expect(viewRows.rows[0]!.fields[updatedByField.id]).toBe(me.id)

  // Create second user as admin.
  const bobUsername = `bob-${suffix}`
  const bobRes = await page.request.post('/api/v1/admin/users', {
    headers: headers(),
    data: {
      username: bobUsername,
      displayName: 'Bob User',
      role: 'user',
      password: 'Temporary-password-123',
    },
  })
  expect(bobRes.status()).toBe(201)
  const bobUser = (await bobRes.json()).user as { id: string }

  // Grant Bob write access on the table.
  const grantRes = await page.request.put('/api/v1/grants', {
    headers: headers(),
    data: {
      resourceType: 'table',
      resourceId: tableId,
      principalType: 'user',
      principalId: bobUser.id,
      level: 'write',
    },
  })
  expect([200, 201]).toContain(grantRes.status())

  // Bob logs in in a separate browser context, changes temporary password, and edits the row.
  const bobContext = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL,
    viewport: { width: 1440, height: 900 },
    locale: 'en-US',
  })
  const bobPage = await bobContext.newPage()
  try {
    await paceLogin(bobPage)
    await bobPage.goto('/login')
    await bobPage.getByLabel(en.auth.login, { exact: true }).fill(bobUsername)
    await bobPage
      .getByLabel(en.auth.password, { exact: true })
      .fill('Temporary-password-123')
    await bobPage
      .getByRole('button', { name: en.auth.signIn, exact: true })
      .click()
    await expect(bobPage).toHaveURL(/\/change-password$/)
    await bobPage
      .getByLabel(en.auth.currentPassword, { exact: true })
      .fill('Temporary-password-123')
    await bobPage
      .getByLabel(en.auth.newPassword, { exact: true })
      .fill('Changed-password-123')
    await bobPage
      .getByLabel(en.auth.confirmPassword, { exact: true })
      .fill('Changed-password-123')
    await bobPage
      .getByRole('button', { name: en.auth.changePassword, exact: true })
      .click()
    await expect(bobPage).toHaveURL(/\/login$/)
    await bobPage.getByLabel(en.auth.login, { exact: true }).fill(bobUsername)
    await bobPage
      .getByLabel(en.auth.password, { exact: true })
      .fill('Changed-password-123')
    await bobPage
      .getByRole('button', { name: en.auth.signIn, exact: true })
      .click()
    await expect(bobPage).toHaveURL(/\/home$/)

    await bobPage.goto(`/t/${tableId}/v/${viewId}`)
    await expect(
      bobPage.getByRole('region', { name: en.grid.title }),
    ).toBeVisible()

    // Bob edits the cell in the grid.
    await openCellEditor(bobPage, nameField)
    const bobInput = bobPage.getByRole('textbox', {
      name: en.field.editor.value,
      exact: true,
    })
    await expect(bobInput).toBeVisible()
    await bobInput.fill('Edited by Bob')
    await bobInput.press('Enter')
    await expect(bobInput).not.toBeVisible()

    await expect
      .poll(
        async () =>
          (await queryOf(bobPage, viewId)).rows[0]?.fields[nameField.id],
      )
      .toBe('Edited by Bob')
  } finally {
    await bobContext.close()
  }

  // Back in Alice's browser, reload and verify authorship values.
  await page.reload()
  await expect(gridElement).toBeVisible()

  const updatedRowRes = await page.request.get(
    `/api/v1/tables/${tableId}/rows/${rowId}`,
  )
  const updatedRow = (await updatedRowRes.json()) as DataRow
  expect(updatedRow.fields[nameField.id]).toBe('Edited by Bob')
  expect(updatedRow.fields[createdByField.id]).toBe(me.id)
  expect(updatedRow.fields[updatedByField.id]).toBe(bobUser.id)

  // Clean up.
  await page.goto('/home')
  await page.request.delete(`/api/v1/tables/${tableId}`, { headers: headers() })
  await page.request.patch(`/api/v1/admin/users/${bobUser.id}`, {
    headers: headers(),
    data: { status: 'disabled' },
  })
})
