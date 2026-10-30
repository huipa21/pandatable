import { expect, paceLogin, test } from './fixtures'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }
import type { RowSort, TableSchema } from '../src/api/types'

const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

async function signIn(page: import('@playwright/test').Page) {
  await paceLogin(page)
  await page.goto('/login')
  if ((await page.locator('html').getAttribute('lang')) === 'zh-CN')
    await page
      .getByRole('button', { name: zh.common.languages.en, exact: true })
      .click()
  await page.getByLabel(en.auth.login, { exact: true }).fill('first-admin')
  await page
    .getByLabel(en.auth.password, { exact: true })
    .fill('Setup-password-123')
  await page.getByRole('button', { name: en.auth.signIn, exact: true }).click()
  await expect(page).toHaveURL(/\/(home|change-password)$/)
}

function compareNullLast(
  left: string | null,
  right: string | null,
  direction: 1 | -1,
) {
  if (left === null) return right === null ? 0 : 1
  if (right === null) return -1
  return left < right ? -direction : left > right ? direction : 0
}

test('AT-503 ordered sorting is stable across query pages and grid scrolling', async ({
  page,
}, testInfo) => {
  test.skip(
    testInfo.project.name.toLowerCase().includes('mobile'),
    'The current row grid is desktop-only.',
  )
  test.setTimeout(180_000)
  await signIn(page)

  const stamp = Date.now()
  const create = async (path: string, data: unknown) => {
    const response = await page.request.post(path, { headers: headers(), data })
    expect(response.status()).toBe(201)
    return response.json()
  }
  const workspace = (await create('/api/v1/workspaces', {
    name: `Sort ${stamp}`,
  })) as { workspace: { id: string } }
  const book = (await create(
    `/api/v1/workspaces/${workspace.workspace.id}/books`,
    { name: `Sort book ${stamp}` },
  )) as { book: { id: string } }
  const schema = (await create(`/api/v1/books/${book.book.id}/tables`, {
    name: `Sort table ${stamp}`,
  })) as TableSchema
  const tableId = schema.table.id

  const createField = async (name: string, uiType: string) => {
    const result = (await create(`/api/v1/tables/${tableId}/fields`, {
      name,
      uiType,
    })) as { field: TableSchema['fields'][number] }
    return result.field
  }
  const status = await createField('Status', 'text')
  const due = await createField('Due', 'date')
  const values = Array.from({ length: 205 }, (_, index) => ({
    name: `Task ${String(index).padStart(3, '0')}`,
    status: index % 17 === 0 ? null : index % 2 === 0 ? 'Open' : 'Blocked',
    due:
      index % 11 === 0
        ? null
        : `2026-03-${String((index % 9) + 1).padStart(2, '0')}`,
  }))
  const createdResponse = await page.request.post(
    `/api/v1/tables/${tableId}/rows`,
    {
      headers: headers(),
      data: {
        rows: values.map((value) => ({
          fields: {
            [schema.table.displayFieldId!]: value.name,
            [status.id]: value.status,
            [due.id]: value.due,
          },
        })),
      },
    },
  )
  expect(createdResponse.status()).toBe(201)
  const created = (await createdResponse.json()) as {
    rows: { id: string }[]
  }
  const sorts: RowSort[] = [
    { fieldId: status.id, direction: 'asc' },
    { fieldId: due.id, direction: 'desc' },
  ]
  const expected = values
    .map((value, index) => ({ ...value, id: created.rows[index]!.id }))
    .sort(
      (left, right) =>
        compareNullLast(left.status, right.status, 1) ||
        compareNullLast(left.due, right.due, -1) ||
        (BigInt(left.id) < BigInt(right.id)
          ? -1
          : BigInt(left.id) > BigInt(right.id)
            ? 1
            : 0),
    )
  const paged: { id: string; fields: Record<string, unknown> }[] = []
  for (const offset of [0, 100, 200]) {
    const response = await page.request.post(
      `/api/v1/views/${schema.views[0]!.id}/query`,
      { headers: headers(), data: { offset, limit: 100, sorts } },
    )
    expect(response.status()).toBe(200)
    const result = (await response.json()) as {
      rows: { id: string; fields: Record<string, unknown> }[]
    }
    paged.push(...result.rows)
  }
  expect(paged.map((row) => row.id)).toEqual(expected.map((row) => row.id))
  expect(new Set(paged.map((row) => row.id)).size).toBe(values.length)

  await page.goto(`/t/${tableId}/v/${schema.views[0]!.id}`)
  await page.getByText(en.grid.sort, { exact: true }).click()
  const addField = page.getByLabel(en.grid.addSortField, { exact: true })
  await addField.selectOption(status.id)
  await page.getByRole('button', { name: en.grid.addSort, exact: true }).click()
  await addField.selectOption(due.id)
  await page.getByRole('button', { name: en.grid.addSort, exact: true }).click()
  const sortedQuery = page.waitForResponse((response) => {
    if (
      response.request().method() !== 'POST' ||
      new URL(response.url()).pathname !==
        `/api/v1/views/${schema.views[0]!.id}/query`
    )
      return false
    const request = response.request().postDataJSON() as {
      sorts?: RowSort[]
    }
    return (
      request.sorts?.length === 2 &&
      request.sorts[0]?.fieldId === status.id &&
      request.sorts[1]?.fieldId === due.id &&
      request.sorts[1]?.direction === 'desc'
    )
  })
  await page
    .getByLabel(en.grid.sortDirection.replace('{field}', 'Due'), {
      exact: true,
    })
    .selectOption('desc')
  const firstGridPage = await sortedQuery
  expect(firstGridPage.status()).toBe(200)
  expect(firstGridPage.request().postDataJSON()).toMatchObject({ sorts })

  await page.getByText(en.grid.sort, { exact: true }).click()
  const nextPage = page.waitForResponse(
    (response) => {
      if (
        response.request().method() !== 'POST' ||
        new URL(response.url()).pathname !==
          `/api/v1/views/${schema.views[0]!.id}/query`
      )
        return false
      const request = response.request().postDataJSON() as {
        offset?: number
        sorts?: RowSort[]
      }
      return request.offset === 200 && request.sorts?.length === 2
    },
    { timeout: 30_000 },
  )
  await page.locator('revo-grid').evaluate(async (element) => {
    const grid = element as HTMLElement & {
      scrollToRow(index: number): Promise<void>
    }
    await grid.scrollToRow(200)
  })
  expect((await nextPage).status()).toBe(200)
})
