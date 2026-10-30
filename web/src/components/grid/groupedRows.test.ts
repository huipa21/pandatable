import { beforeEach, expect, it, vi } from 'vitest'
import type { RowGroupBucket, RowQueryResult, RowValue } from '../../api/types'
import { GroupedRowsWindow } from './groupedRows'

beforeEach(() => {
  window.localStorage.clear()
})

it('loads nested buckets and leaf rows on demand and persists per-user expansion', async () => {
  const groups = vi.fn(async (path: RowValue[]): Promise<RowGroupBucket[]> => {
    if (path.length === 0)
      return [
        {
          key: 'North',
          count: 3,
          aggregates: [{ fieldId: 'hours', aggregate: 'sum', value: 12 }],
        },
        { key: 'South', count: 1, aggregates: [] },
      ]
    if (JSON.stringify(path) === JSON.stringify(['North']))
      return [{ key: 'Open', count: 3, aggregates: [] }]
    if (JSON.stringify(path) === JSON.stringify(['North', 'Open']))
      return [
        { key: 'Alice', count: 2, aggregates: [] },
        { key: null, count: 1, aggregates: [] },
      ]
    return []
  })
  const rows = vi.fn(
    async (
      path: RowValue[],
      offset: number,
      limit: number,
      includeCount: boolean,
    ): Promise<RowQueryResult> => {
      const owner = path.at(-1)
      const data = owner === 'Alice' ? ['row-1', 'row-2'] : ['row-empty']
      return {
        rows: data
          .slice(offset, offset + limit)
          .map((id) => ({ id, fields: {} })),
        fieldIds: [],
        total: includeCount ? data.length : null,
        schemaVersion: 1,
      }
    },
  )
  const key = 'pandatable:groups:alice:view-a'
  const grouped = new GroupedRowsWindow(
    { groups, rows },
    3,
    key,
    () => {},
    (error) => {
      throw error
    },
  )

  await grouped.initialize()
  expect(grouped.entries.map((entry) => entry.kind)).toEqual(['group', 'group'])
  expect(grouped.total).toBe(2)
  await grouped.toggle(['North'])
  await grouped.toggle(['North', 'Open'])
  expect(grouped.entries.map((entry) => entry.kind)).toEqual([
    'group',
    'group',
    'group',
    'group',
    'group',
  ])
  expect(grouped.entries[3]).toMatchObject({
    kind: 'group',
    group: { key: null, count: 1 },
  })
  await grouped.toggle(['North', 'Open', null])
  expect(grouped.total).toBe(6)
  expect(grouped.rowAt(4)?.id).toBe('row-empty')
  expect(grouped.indexOfRow('row-empty')).toBe(4)
  expect(rows).toHaveBeenCalledWith(
    ['North', 'Open', null],
    0,
    100,
    true,
    expect.any(AbortSignal),
  )
  expect(JSON.parse(window.localStorage.getItem(key) ?? '[]')).toEqual([
    '["North"]',
    '["North","Open"]',
    '["North","Open",null]',
  ])
  await grouped.toggle(['North'])
  expect(grouped.total).toBe(2)
  grouped.dispose()
})
