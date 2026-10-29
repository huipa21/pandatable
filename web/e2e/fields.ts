import type { Page } from '@playwright/test'
import type { Components } from '@revolist/revogrid'
import { expect, expectHTTPFailure } from './fixtures'
import type { TableField, TableSchema } from '../src/api/types'
import en from '../src/i18n/en.json' with { type: 'json' }

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

async function schemaOf(page: Page, tid: string) {
  const response = await page.request.get(`/api/v1/tables/${tid}/schema`)
  expect(response.status()).toBe(200)
  return (await response.json()) as TableSchema
}

const byName = (schema: TableSchema, name: string) =>
  schema.fields.find((field) => field.name === name)

function dialog(page: Page, name: string) {
  return page.getByRole('dialog', { name, exact: true })
}

async function fieldAction(page: Page, id: string, action: string) {
  const menu = page.locator(`[data-field-id="${id}"] details.field-actions`)
  if ((await menu.getAttribute('open')) === null)
    await menu.locator('summary').click()
  await menu.getByRole('button', { name: action, exact: true }).click()
}

async function save(
  page: Page,
  editor: ReturnType<typeof dialog>,
  label: string,
) {
  await editor.getByRole('button', { name: label, exact: true }).click()
  await expect(editor).not.toBeVisible()
  await expect(
    page.getByRole('status').filter({ hasText: en.field.panel.saved }),
  ).toBeVisible()
}

export async function openFieldCellEditor(
  page: Page,
  fieldId: string,
  rowIndex = 0,
) {
  const gridElement = page.locator('revo-grid')
  await gridElement.evaluate(
    async (element, { fieldId: targetId, row }) => {
      const grid = element as HTMLElement & Components.RevoGrid
      await grid.scrollToRow(row)
      const columns = await grid.getColumns()
      const column = columns.find((item) => item.prop === targetId)
      if (!column) throw new Error(`Column ${targetId} not found`)
      const type = (column.pin ?? 'rgCol') as
        'rgCol' | 'colPinStart' | 'colPinEnd'
      if (type === 'rgCol' && typeof grid.scrollToColumnProp === 'function')
        await grid.scrollToColumnProp(targetId)
      for (let attempt = 0; attempt < 30; attempt++) {
        const store = await grid.getColumnStore(type)
        const source = store.get('source')
        const items = store.get('items')
        const x = items.findIndex((index) => source[index]?.prop === targetId)
        if (x >= 0) {
          await grid.setCellsFocus({ x, y: row }, { x, y: row }, type)
          grid.focus()
          await new Promise<void>((resolve) =>
            requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
          )
          return
        }
        await new Promise<void>((resolve) =>
          requestAnimationFrame(() => resolve()),
        )
      }
      throw new Error(`Column ${targetId} did not become visible`)
    },
    { fieldId, row: rowIndex },
  )
  await page.keyboard.press('Enter')
}

export async function revealFieldColumn(page: Page, fieldId: string) {
  await page.locator('revo-grid').evaluate(async (element, targetId) => {
    const grid = element as HTMLElement & Components.RevoGrid
    if (typeof grid.scrollToColumnProp !== 'function')
      throw new Error('Grid column scrolling is unavailable')
    await grid.scrollToColumnProp(targetId)
    await new Promise<void>((resolve) =>
      requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
    )
  }, fieldId)
}

/** Real Field UI/API flows on a freshly created table owned by `owner` (T1.14). */
export async function verifyFields(owner: Page, tid: string) {
  const h = headers()
  const panel = owner.getByRole('region', { name: en.field.panel.title })
  if (owner.viewportSize()!.width < 768)
    await owner
      .getByRole('button', { name: en.mobileRows.options, exact: true })
      .click()
  await expect(panel).toBeVisible()
  const initial = await schemaOf(owner, tid)
  const name = byName(initial, 'Name')!

  // Integer with required/unique/default on the empty physical table.
  await panel
    .getByRole('button', { name: en.field.panel.add, exact: true })
    .click()
  let editor = dialog(owner, en.field.panel.createTitle)
  await expect(editor).toBeVisible()
  await editor
    .getByRole('textbox', { name: en.field.panel.name, exact: true })
    .fill('Qty')
  await editor
    .getByRole('combobox', { name: en.field.panel.kind, exact: true })
    .selectOption('integer')
  await editor
    .getByRole('checkbox', { name: en.field.panel.required, exact: true })
    .check()
  await editor
    .getByRole('checkbox', { name: en.field.panel.unique, exact: true })
    .check()
  await editor
    .getByRole('textbox', { name: en.field.panel.defaultValue, exact: true })
    .fill('7')
  await save(owner, editor, en.field.panel.create)
  let schema = await schemaOf(owner, tid)
  const qty = byName(schema, 'Qty')!
  expect(qty).toMatchObject({
    uiType: 'integer',
    isRequired: true,
    isUnique: true,
    default: 7,
    options: { size: 'int', thousandSep: true },
    isSystem: false,
  })
  expect(schema.schemaVersion).toBeGreaterThan(initial.schemaVersion)
  const row = panel.locator(`[data-field-id="${qty.id}"]`)
  await expect(row).toContainText(en.field.panel.required)
  await expect(row).toContainText(en.field.panel.unique)
  // Schema DTOs never expose physical names.
  expect(JSON.stringify(schema)).not.toMatch(
    /pg_column|pgColumn|"c_[a-z2-7]{10}"/,
  )

  // Single select with closed palette choices and a label default.
  await panel
    .getByRole('button', { name: en.field.panel.add, exact: true })
    .click()
  editor = dialog(owner, en.field.panel.createTitle)
  await editor
    .getByRole('textbox', { name: en.field.panel.name, exact: true })
    .fill('Stage')
  await editor
    .getByRole('combobox', { name: en.field.panel.kind, exact: true })
    .selectOption('single_select')
  for (const [index, label] of ['Todo', 'Doing'].entries()) {
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
  await editor
    .getByRole('combobox', {
      name: en.field.panel.choiceColor.replace('{n}', '2'),
      exact: true,
    })
    .selectOption('blue')
  await editor
    .getByRole('combobox', { name: en.field.panel.defaultValue, exact: true })
    .selectOption({ label: 'Todo' })
  await save(owner, editor, en.field.panel.create)
  schema = await schemaOf(owner, tid)
  let stage = byName(schema, 'Stage')!
  expect(stage.default).toBe('Todo')
  expect(
    (stage.options as { choices: { label: string; color: string }[] }).choices,
  ).toEqual([
    expect.objectContaining({ label: 'Todo', color: 'gray' }),
    expect.objectContaining({ label: 'Doing', color: 'blue' }),
  ])

  // A choice label edit rewrites the stored default atomically (D-128).
  await fieldAction(owner, stage.id, en.field.panel.edit)
  editor = dialog(owner, en.field.panel.editTitle)
  await editor
    .getByRole('textbox', {
      name: en.field.panel.choiceLabel.replace('{n}', '1'),
      exact: true,
    })
    .fill('To do')
  await save(owner, editor, en.field.panel.save)
  schema = await schemaOf(owner, tid)
  stage = byName(schema, 'Stage')!
  expect(stage.default).toBe('To do')

  // AT-202: rename through the Field UI keeps id and data identity.
  await fieldAction(owner, qty.id, en.field.panel.edit)
  editor = dialog(owner, en.field.panel.editTitle)
  await expect(editor).toContainText(en.field.panel.storageFixed)
  await expect(
    editor.getByRole('combobox', { name: en.field.panel.size, exact: true }),
  ).toBeDisabled()
  await editor
    .getByRole('textbox', { name: en.field.panel.name, exact: true })
    .fill('数量')
  await save(owner, editor, en.field.panel.save)
  schema = await schemaOf(owner, tid)
  const renamed = schema.fields.find((field) => field.id === qty.id)!
  expect(renamed).toMatchObject({ name: '数量', uiType: 'integer', default: 7 })
  expect(renamed.version).toBeGreaterThan(qty.version)
  await expect(row.locator('.field-name')).toHaveText('数量')

  // Active-name uniqueness is enforced server-side and localized.
  expectHTTPFailure(owner, 'POST', `/api/v1/tables/${tid}/fields`, 409)
  await panel
    .getByRole('button', { name: en.field.panel.add, exact: true })
    .click()
  editor = dialog(owner, en.field.panel.createTitle)
  await editor
    .getByRole('textbox', { name: en.field.panel.name, exact: true })
    .fill('name')
  await editor
    .getByRole('button', { name: en.field.panel.create, exact: true })
    .click()
  await expect(editor.getByRole('alert')).toHaveText(
    en.errors.ERR_FIELD_NAME_CONFLICT,
  )
  await editor
    .getByRole('button', { name: en.field.panel.cancel, exact: true })
    .click()
  await expect(editor).not.toBeVisible()

  // A stale If-Match is rejected without side effects.
  const stale = await owner.request.patch(`/api/v1/fields/${qty.id}`, {
    headers: { ...h, 'If-Match': String(qty.version) },
    data: { name: 'Stale' },
  })
  expect(stale.status()).toBe(409)
  expect(await stale.json()).toMatchObject({
    error: { code: 'ERR_VERSION_CONFLICT' },
  })

  // Display field, reorder and soft delete through the menu.
  await fieldAction(owner, qty.id, en.field.panel.setDisplay)
  await expect(row).toContainText(en.field.panel.display)
  await expect(owner.locator('.schema-summary')).toContainText('数量')
  await fieldAction(owner, stage.id, en.field.panel.moveUp)
  await expect
    .poll(async () =>
      (await schemaOf(owner, tid)).fields
        .filter((field: TableField) => !field.isSystem)
        .sort((a, b) => a.position - b.position)
        .map((field) => field.name),
    )
    .toEqual(['Name', 'Stage', '数量'])
  await fieldAction(owner, stage.id, en.field.panel.delete)
  const remover = dialog(owner, en.field.panel.confirm)
  await expect(remover).toContainText('Stage')
  await remover
    .getByRole('button', { name: en.field.panel.confirm, exact: true })
    .click()
  await expect(remover).not.toBeVisible()
  await expect(panel.locator(`[data-field-id="${stage.id}"]`)).toHaveCount(0)
  schema = await schemaOf(owner, tid)
  expect(byName(schema, 'Stage')).toBeUndefined()
  expect(schema.table.displayFieldId).toBe(qty.id)
  expect(
    (
      await owner.request.patch(`/api/v1/fields/${stage.id}`, {
        headers: { ...h, 'If-Match': String(stage.version + 1) },
        data: { name: 'Back' },
      })
    ).status(),
  ).toBe(404)

  // System fields stay read-only apart from name/description.
  const system = schema.fields.find((field) => field.isSystem)!
  const systemPatch = await owner.request.patch(`/api/v1/fields/${system.id}`, {
    headers: { ...h, 'If-Match': String(system.version) },
    data: { isRequired: true },
  })
  expect(systemPatch.status()).toBe(400)
  expect(await systemPatch.json()).toMatchObject({
    error: { code: 'ERR_VALIDATION', details: { reason: 'system_field' } },
  })
  // Restore the Name display field so later navigation assertions still hold.
  expect(
    (
      await owner.request.patch(`/api/v1/tables/${tid}`, {
        headers: h,
        data: { displayFieldId: name.id },
      })
    ).status(),
  ).toBe(200)
  await owner.reload()
  await expect(owner.locator('.schema-summary')).toContainText('Name')
  return { qty: { id: qty.id, version: renamed.version } }
}

/** A non-owner sees the read-only field list and is denied field mutations. */
export async function verifyFieldReader(
  page: Page,
  tid: string,
  field: { id: string; version: number },
) {
  const h = headers()
  const panel = page.getByRole('region', { name: en.field.panel.title })
  if (page.viewportSize()!.width < 768)
    await page
      .getByRole('button', { name: en.mobileRows.options, exact: true })
      .click()
  await expect(panel).toContainText(en.field.panel.readOnly)
  await expect(panel).toContainText('数量')
  await expect(panel.getByRole('button')).toHaveCount(0)
  await expect(panel.locator('details')).toHaveCount(0)
  const create = await page.request.post(`/api/v1/tables/${tid}/fields`, {
    headers: h,
    data: { name: 'Denied', uiType: 'text' },
  })
  expect(create.status()).toBe(403)
  const rename = await page.request.patch(`/api/v1/fields/${field.id}`, {
    headers: { ...h, 'If-Match': String(field.version) },
    data: { name: 'Denied' },
  })
  expect(rename.status()).toBe(403)
  expect(await rename.json()).toMatchObject({
    error: { code: 'ERR_PERMISSION_DENIED' },
  })
  const display = await page.request.patch(`/api/v1/tables/${tid}`, {
    headers: h,
    data: { displayFieldId: field.id },
  })
  expect(display.status()).toBe(403)
  if (page.viewportSize()!.width < 768)
    await page
      .getByRole('button', { name: en.mobileRows.closeOptions, exact: true })
      .click()
}
