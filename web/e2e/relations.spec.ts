import { expect, test, paceLogin, expectHTTPFailure } from './fixtures'
import type { DataRow, TableSchema } from '../src/api/types'
import { setLocale } from './app-shell'
import { openFieldTypeActions, openGridMore } from './table-controls'
import en from '../src/i18n/en.json' with { type: 'json' }

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

async function login(page: import('@playwright/test').Page) {
  await paceLogin(page)
  await page.goto('/login')
  if ((await page.locator('html').getAttribute('lang')) === 'zh-CN')
    await setLocale(page, 'en')
  await page.getByLabel(en.auth.login, { exact: true }).fill('first-admin')
  await page
    .getByLabel(en.auth.password, { exact: true })
    .fill('Setup-password-123')
  await page.getByRole('button', { name: en.auth.signIn, exact: true }).click()
  await expect(page).toHaveURL(/\/home$/)
}

async function createWorkspace(page: import('@playwright/test').Page) {
  const response = await page.request.post('/api/v1/workspaces', {
    headers: headers(),
    data: { name: `Relations ${Date.now()}` },
  })
  expect(response.status()).toBe(201)
  return ((await response.json()) as { workspace: { id: string } }).workspace.id
}

async function createBook(
  page: import('@playwright/test').Page,
  workspaceId: string,
  name: string,
) {
  const response = await page.request.post(
    `/api/v1/workspaces/${workspaceId}/books`,
    { headers: headers(), data: { name } },
  )
  expect(response.status()).toBe(201)
  return ((await response.json()) as { book: { id: string } }).book.id
}

async function createTable(
  page: import('@playwright/test').Page,
  bookId: string,
  name: string,
) {
  const response = await page.request.post(`/api/v1/books/${bookId}/tables`, {
    headers: headers(),
    data: { name },
  })
  expect(response.status()).toBe(201)
  return (await response.json()) as TableSchema
}

async function createRows(
  page: import('@playwright/test').Page,
  schema: TableSchema,
  labels: string[],
) {
  const labelId = schema.table.displayFieldId
  if (!labelId) throw new Error('Table has no display field')
  const response = await page.request.post(
    `/api/v1/tables/${schema.table.id}/rows`,
    {
      headers: headers(),
      data: {
        rows: labels.map((label) => ({ fields: { [labelId]: label } })),
      },
    },
  )
  expect(response.status()).toBe(201)
  return ((await response.json()) as { rows: DataRow[] }).rows
}

async function getRow(
  page: import('@playwright/test').Page,
  tableId: string,
  rowId: string,
) {
  const response = await page.request.get(
    `/api/v1/tables/${tableId}/rows/${rowId}`,
  )
  expect(response.status()).toBe(200)
  return (await response.json()) as DataRow
}

async function createRelation(
  page: import('@playwright/test').Page,
  source: TableSchema,
  target: TableSchema,
  input: {
    name: string
    reverseName: string
    kind: 'one_one' | 'one_many' | 'many_many'
    sourceSide?: 'parent' | 'child'
    onDelete?: 'set_null' | 'cascade' | 'restrict'
  },
) {
  const response = await page.request.post(
    `/api/v1/tables/${source.table.id}/relations`,
    {
      headers: headers(),
      data: { ...input, targetTableId: target.table.id },
    },
  )
  expect(response.status()).toBe(201)
  return (await response.json()) as {
    relation: TableSchema['relations'][number]
    field: { id: string; name: string }
    schema: TableSchema
    eventId: string
  }
}

async function changeLinks(
  page: import('@playwright/test').Page,
  tableId: string,
  rowId: string,
  fieldId: string,
  add: string[],
) {
  const response = await page.request.post(
    `/api/v1/tables/${tableId}/rows/${rowId}/links/${fieldId}`,
    { headers: headers(), data: { add } },
  )
  expect(response.status()).toBe(200)
}

async function openPicker(
  page: import('@playwright/test').Page,
  fieldName: string,
  rowIndex = 0,
) {
  await page
    .getByRole('button', { name: en.relation.edit, exact: true })
    .nth(rowIndex)
    .click()
  const title = en.relation.pickerTitle.replace('{field}', fieldName)
  const picker = page.getByRole('dialog', { name: title, exact: true })
  await expect(picker).toBeVisible()
  return picker
}

test('AT-401..406 relation fields, linked rows, replacement, delete policies and scope', async ({
  page,
  isMobile,
}) => {
  test.setTimeout(150_000)
  test.skip(isMobile, 'The relation grid flow is exercised on desktop.')
  await page.setViewportSize({ width: 1440, height: 900 })
  await login(page)

  const workspaceId = await createWorkspace(page)
  const bookId = await createBook(page, workspaceId, 'Relation book')
  const otherBookId = await createBook(page, workspaceId, 'Other book')
  const projects = await createTable(page, bookId, 'Projects')
  const tasks = await createTable(page, bookId, 'Tasks')
  const employees = await createTable(page, bookId, 'Employees')
  const left = await createTable(page, bookId, 'Left records')
  const right = await createTable(page, bookId, 'Right records')
  const tags = await createTable(page, bookId, 'Tags')
  const cascadeParents = await createTable(page, bookId, 'Cascade parents')
  const cascadeChildren = await createTable(page, bookId, 'Cascade children')
  const restrictParents = await createTable(page, bookId, 'Restricted parents')
  const restrictChildren = await createTable(
    page,
    bookId,
    'Restricted children',
  )
  const outside = await createTable(page, otherBookId, 'Outside')

  await page.goto(`/t/${tasks.table.id}/v/${tasks.views[0]!.id}`)
  const fieldPanel = await openFieldTypeActions(page)
  await fieldPanel.locator('[data-kind="link"]').click()
  const relationDialog = page.getByRole('dialog', {
    name: en.relation.createTitle,
    exact: true,
  })
  await expect(relationDialog).toBeVisible()
  const targetSelect = relationDialog.getByRole('combobox', {
    name: en.relation.target,
    exact: true,
  })
  const targetIds = await targetSelect
    .locator('option')
    .evaluateAll((options) =>
      options.map((option) => (option as HTMLOptionElement).value),
    )
  expect(targetIds).toContain(tasks.table.id)
  expect(targetIds).toContain(projects.table.id)
  expect(targetIds).not.toContain(outside.table.id)
  await targetSelect.selectOption(projects.table.id)
  await relationDialog
    .getByRole('textbox', { name: en.relation.name, exact: true })
    .fill('Project')
  await relationDialog
    .getByText(en.field.panel.advancedSettings, { exact: true })
    .click()
  await relationDialog
    .getByRole('textbox', { name: en.relation.reverseName, exact: true })
    .fill('Tasks')
  await relationDialog
    .getByRole('radio', { name: en.relation.manyOne, exact: true })
    .check()
  await relationDialog
    .getByRole('button', { name: en.field.panel.create, exact: true })
    .click()
  await expect(relationDialog).not.toBeVisible()

  const taskSchemaResponse = await page.request.get(
    `/api/v1/tables/${tasks.table.id}/schema`,
  )
  const taskSchema = (await taskSchemaResponse.json()) as TableSchema
  const projectSchemaResponse = await page.request.get(
    `/api/v1/tables/${projects.table.id}/schema`,
  )
  const projectSchema = (await projectSchemaResponse.json()) as TableSchema
  const projectField = taskSchema.fields.find(
    (field) => field.uiType === 'link' && field.name === 'Project',
  )!
  const taskField = projectSchema.fields.find(
    (field) => field.uiType === 'link' && field.name === 'Tasks',
  )!
  const projectRows = await createRows(page, projects, ['Project A'])
  const taskRows = await createRows(page, tasks, ['Task A', 'Task B'])

  await page.goto(`/t/${tasks.table.id}/v/${tasks.views[0]!.id}`)
  await expect(page.locator('revo-grid')).toBeVisible()
  const picker = await openPicker(page, 'Project', 0)
  await picker.getByRole('checkbox', { name: 'Project A', exact: true }).check()
  await picker
    .getByRole('button', { name: en.relation.save, exact: true })
    .click()
  await expect(picker).not.toBeVisible()
  const linkedTask = await getRow(page, tasks.table.id, taskRows[0]!.id)
  expect(linkedTask.fields[projectField.id]).toEqual([
    { id: projectRows[0]!.id, label: 'Project A' },
  ])
  await changeLinks(page, tasks.table.id, taskRows[1]!.id, projectField.id, [
    projectRows[0]!.id,
  ])
  const linkedProject = await getRow(
    page,
    projects.table.id,
    projectRows[0]!.id,
  )
  expect(linkedProject.fields[taskField.id]).toEqual([
    { id: taskRows[0]!.id, label: 'Task A' },
    { id: taskRows[1]!.id, label: 'Task B' },
  ])

  const self = await createRelation(page, employees, employees, {
    name: 'Manager',
    reverseName: 'Reports',
    kind: 'one_many',
    sourceSide: 'child',
  })
  const employeeRows = await createRows(page, employees, [
    'Manager A',
    'Report A',
  ])
  await changeLinks(
    page,
    employees.table.id,
    employeeRows[1]!.id,
    self.relation.childFieldId,
    [employeeRows[0]!.id],
  )
  const manager = await getRow(page, employees.table.id, employeeRows[0]!.id)
  expect(manager.fields[self.relation.parentFieldId]).toEqual([
    { id: employeeRows[1]!.id, label: 'Report A' },
  ])

  const oneToOne = await createRelation(page, left, right, {
    name: 'Right',
    reverseName: 'Left',
    kind: 'one_one',
  })
  const leftRows = await createRows(page, left, ['Left A', 'Left B'])
  const rightRows = await createRows(page, right, ['Right A'])
  await page.goto(`/t/${left.table.id}/v/${left.views[0]!.id}`)
  await expect(page.locator('revo-grid')).toBeVisible()
  const firstPicker = await openPicker(page, 'Right', 0)
  await firstPicker
    .getByRole('checkbox', { name: 'Right A', exact: true })
    .check()
  await firstPicker
    .getByRole('button', { name: en.relation.save, exact: true })
    .click()
  await expect(firstPicker).not.toBeVisible()

  const secondPicker = await openPicker(page, 'Right', 1)
  await secondPicker
    .getByRole('checkbox', { name: 'Right A', exact: true })
    .check()
  let replacementMessage = ''
  page.once('dialog', async (dialog) => {
    replacementMessage = dialog.message()
    await dialog.accept()
  })
  expectHTTPFailure(
    page,
    'POST',
    `/api/v1/tables/${left.table.id}/rows/${leftRows[1]!.id}/links/${oneToOne.relation.parentFieldId}`,
    409,
  )
  await secondPicker
    .getByRole('button', { name: en.relation.save, exact: true })
    .click()
  await expect(secondPicker).not.toBeVisible()
  expect(replacementMessage).toContain('Right A')
  expect(replacementMessage).toContain('Left A')
  expect(
    (await getRow(page, left.table.id, leftRows[0]!.id)).fields[
      oneToOne.relation.parentFieldId
    ],
  ).toEqual([])
  expect(
    (await getRow(page, left.table.id, leftRows[1]!.id)).fields[
      oneToOne.relation.parentFieldId
    ],
  ).toEqual([{ id: rightRows[0]!.id, label: 'Right A' }])

  const manyToMany = await createRelation(page, tasks, tags, {
    name: 'Tags',
    reverseName: 'Tasks',
    kind: 'many_many',
    onDelete: 'cascade',
  })
  const tagRows = await createRows(page, tags, ['Tag A', 'Tag B', 'Tag C'])
  await changeLinks(
    page,
    tasks.table.id,
    taskRows[0]!.id,
    manyToMany.relation.parentFieldId,
    tagRows.map((row) => row.id),
  )
  await changeLinks(
    page,
    tags.table.id,
    tagRows[0]!.id,
    manyToMany.relation.childFieldId,
    [taskRows[1]!.id],
  )
  expect(
    (await getRow(page, tags.table.id, tagRows[0]!.id)).fields[
      manyToMany.relation.childFieldId
    ],
  ).toEqual([
    { id: taskRows[0]!.id, label: 'Task A' },
    { id: taskRows[1]!.id, label: 'Task B' },
  ])

  const deleteProject = await page.request.delete(
    `/api/v1/tables/${projects.table.id}/rows`,
    { headers: headers(), data: { ids: [projectRows[0]!.id] } },
  )
  expect(deleteProject.status()).toBe(200)
  expect(
    (await getRow(page, tasks.table.id, taskRows[0]!.id)).fields[
      projectField.id
    ],
  ).toEqual([])
  const cascade = await createRelation(page, cascadeChildren, cascadeParents, {
    name: 'Parent',
    reverseName: 'Children',
    kind: 'one_many',
    sourceSide: 'child',
    onDelete: 'cascade',
  })
  const cascadeParentRows = await createRows(page, cascadeParents, [
    'Delete me',
  ])
  const cascadeChildRows = await createRows(page, cascadeChildren, ['Child'])
  await changeLinks(
    page,
    cascadeChildren.table.id,
    cascadeChildRows[0]!.id,
    cascade.relation.childFieldId,
    [cascadeParentRows[0]!.id],
  )
  const cascadeDelete = await page.request.delete(
    `/api/v1/tables/${cascadeParents.table.id}/rows`,
    { headers: headers(), data: { ids: [cascadeParentRows[0]!.id] } },
  )
  expect(cascadeDelete.status()).toBe(200)
  const missingCascadeChild = await page.request.get(
    `/api/v1/tables/${cascadeChildren.table.id}/rows/${cascadeChildRows[0]!.id}`,
  )
  expect(missingCascadeChild.status()).toBe(404)

  const restrict = await createRelation(
    page,
    restrictChildren,
    restrictParents,
    {
      name: 'Parent',
      reverseName: 'Children',
      kind: 'one_many',
      sourceSide: 'child',
      onDelete: 'restrict',
    },
  )
  const restrictedParentRows = await createRows(page, restrictParents, [
    'Keep me',
  ])
  const restrictedChildRows = await createRows(page, restrictChildren, [
    'Linked child',
  ])
  await changeLinks(
    page,
    restrictChildren.table.id,
    restrictedChildRows[0]!.id,
    restrict.relation.childFieldId,
    [restrictedParentRows[0]!.id],
  )
  await page.goto(
    `/t/${restrictParents.table.id}/v/${restrictParents.views[0]!.id}`,
  )
  await expect(page.locator('revo-grid')).toBeVisible()
  const selectRow = en.grid.selectRow.replace(
    '{id}',
    restrictedParentRows[0]!.id,
  )
  const rowSelector = page.getByRole('checkbox', {
    name: selectRow,
    exact: true,
  })
  await rowSelector.focus()
  await rowSelector.press('Space')
  await openGridMore(page)
  await page
    .getByRole('button', {
      name: en.grid.deleteRows.replace('{count}', '1'),
      exact: true,
    })
    .click()
  const deleteDialog = page.getByRole('dialog', {
    name: en.grid.confirmDeleteTitle,
    exact: true,
  })
  expectHTTPFailure(
    page,
    'DELETE',
    `/api/v1/tables/${restrictParents.table.id}/rows`,
    409,
  )
  await deleteDialog
    .getByRole('button', { name: en.grid.deleteConfirm, exact: true })
    .click()
  await expect(page.getByRole('alert')).toContainText(
    en.errors.ERR_RESTRICT_VIOLATION,
  )

  const crossBook = await page.request.post(
    `/api/v1/tables/${tasks.table.id}/relations`,
    {
      headers: headers(),
      data: {
        name: 'Outside relation',
        reverseName: 'Tasks',
        targetTableId: outside.table.id,
        kind: 'one_many',
        sourceSide: 'child',
      },
    },
  )
  expect(crossBook.status()).toBe(400)
})
