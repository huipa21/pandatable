import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { defineComponent, shallowReactive } from 'vue'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import type { TableSchema } from '../../api/types'
import { tableSchema } from '../../test/navigation'
import {
  applyColumnIntents,
  columnSaveDelay,
  isCompleteFilter,
  normalizeColumns,
  useViewColumns,
} from './viewColumns'

let wrapper: VueWrapper | undefined
beforeEach(() => vi.useFakeTimers())
afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

function fixture() {
  const schema = tableSchema()
  schema.fields.push(
    { ...schema.fields[0]!, id: 'other', name: 'Other', position: 2048 },
    {
      ...schema.fields[0]!,
      id: 'system',
      name: 'ID',
      isSystem: true,
      position: 1e9,
    },
  )
  schema.views[0]!.config = {
    fields: [
      { id: 'name', visible: true, width: 180 },
      { id: 'other', visible: true, width: 200 },
    ],
    frozen: 0,
  }
  return schema
}

function mounted(schema: TableSchema = fixture()) {
  const state = shallowReactive({ schema, identity: 'actor', canWrite: true })
  const updated = vi.fn()
  const failure = vi.fn()
  let columns!: ReturnType<typeof useViewColumns>
  wrapper = mount(
    defineComponent({
      setup() {
        columns = useViewColumns({
          schema: () => state.schema,
          viewId: () => 'v',
          identity: () => state.identity,
          canWrite: () => state.canWrite,
          updated,
          failure,
          message: () => 'translated failure',
        })
        return () => null
      },
    }),
  )
  return { state, columns, updated, failure }
}

async function debounce() {
  await vi.advanceTimersByTimeAsync(columnSaveDelay)
  await flushPromises()
}

it('drops unknown/deleted/duplicate fields, appends live fields and clamps frozen count', () => {
  const schema = fixture()
  const result = normalizeColumns(
    {
      fields: [
        { id: 'missing', visible: true },
        { id: 'other', visible: false, width: 300 },
        { id: 'other', visible: true },
      ],
      frozen: 5,
    },
    schema,
  )
  expect(result).toEqual({
    fields: [
      { id: 'other', visible: false, width: 300 },
      { id: 'name', visible: true },
      { id: 'system', visible: false },
    ],
    frozen: 1,
  })
})

it('keeps incomplete highlight filters out of persisted view config', () => {
  expect(isCompleteFilter({ field: 'name', cmp: 'is', value: '' })).toBe(false)
  expect(isCompleteFilter({ field: 'name', cmp: 'is', value: 'Overdue' })).toBe(
    true,
  )
  expect(isCompleteFilter({ field: 'name', cmp: 'is_empty' })).toBe(true)
  expect(isCompleteFilter({ op: 'and', items: [] })).toBe(false)
  expect(
    isCompleteFilter({
      op: 'and',
      items: [{ field: 'name', cmp: 'is', value: 'Overdue' }],
    }),
  ).toBe(true)
})

it('preserves the view query configuration with column edits', () => {
  const schema = fixture()
  schema.views[0]!.config.filter = {
    field: 'name',
    cmp: 'contains',
    value: 'keep me',
  }
  schema.views[0]!.config.rowHeight = 'tall'
  schema.views[0]!.config.sort = [
    { field: 'other', dir: 'desc' },
    { field: 'missing', dir: 'asc' },
    { field: 'other', dir: 'asc' },
  ]
  schema.views[0]!.config.groupBy = ['other', 'name']
  schema.views[0]!.config.aggregates = [{ fieldId: 'other', aggregate: 'sum' }]
  schema.views[0]!.config.highlight = [
    {
      id: 'hl-overdue',
      when: { field: 'name', cmp: 'is', value: 'Overdue' },
      target: { type: 'cells', fields: ['other'] },
      color: 'amber-3',
    },
  ]
  const normalized = normalizeColumns(schema.views[0]!.config, schema)
  expect(normalized.sort).toEqual([{ field: 'other', dir: 'desc' }])
  expect(normalized.filter).toEqual(schema.views[0]!.config.filter)
  expect(normalized.rowHeight).toBe('tall')
  expect(normalized.groupBy).toEqual(['other', 'name'])
  expect(normalized.aggregates).toEqual([
    { fieldId: 'other', aggregate: 'sum' },
  ])
  expect(normalized.highlight).toEqual(schema.views[0]!.config.highlight)
  const changed = applyColumnIntents(
    schema.views[0]!.config,
    [{ kind: 'visible', id: 'name', visible: false }],
    schema,
  )
  expect(changed.sort).toEqual([{ field: 'other', dir: 'desc' }])
  expect(changed.groupBy).toEqual(['other', 'name'])
  expect(changed.aggregates).toEqual([{ fieldId: 'other', aggregate: 'sum' }])
  expect(changed.highlight).toEqual(schema.views[0]!.config.highlight)
})

it('saves highlight rules through the versioned view config', async () => {
  const fetch = vi.fn(async (_url: unknown, init?: RequestInit) =>
    Response.json({
      view: {
        ...fixture().views[0],
        version: 2,
        config: JSON.parse(String(init?.body)).config,
      },
    }),
  )
  vi.stubGlobal('fetch', fetch)
  const { columns } = mounted()
  columns.changeHighlights([
    {
      id: 'hl-overdue',
      when: { field: 'name', cmp: 'is', value: 'Overdue' },
      target: { type: 'row' },
      color: 'red-2',
    },
  ])
  await debounce()
  expect(fetch).toHaveBeenCalledTimes(1)
  expect(
    JSON.parse(String(fetch.mock.calls[0]?.[1]?.body)).config.highlight,
  ).toEqual([
    {
      id: 'hl-overdue',
      when: { field: 'name', cmp: 'is', value: 'Overdue' },
      target: { type: 'row' },
      color: 'red-2',
    },
  ])
  expect(columns.config.value.highlight?.[0]?.color).toBe('red-2')
})

it('saves aggregate changes without replacing unrelated view config', async () => {
  const schema = fixture()
  schema.views[0]!.config.sort = [{ field: 'other', dir: 'desc' }]
  schema.views[0]!.config.groupBy = ['other']
  schema.views[0]!.config.highlight = [
    {
      id: 'hl-overdue',
      when: { field: 'name', cmp: 'is', value: 'Overdue' },
      target: { type: 'row' },
      color: 'red-2',
    },
  ]
  const fetch = vi.fn(async (_url: unknown, init?: RequestInit) =>
    Response.json({
      view: {
        ...schema.views[0],
        version: 2,
        config: JSON.parse(String(init?.body)).config,
      },
      eventId: 'event',
    }),
  )
  vi.stubGlobal('fetch', fetch)
  const { columns } = mounted(schema)
  columns.changeAggregates([{ fieldId: 'other', aggregate: 'sum' }])
  await debounce()

  const request = fetch.mock.calls[0]?.[1]
  expect(request?.headers).toMatchObject({ 'If-Match': '1' })
  expect(JSON.parse(String(request?.body)).config).toMatchObject({
    sort: [{ field: 'other', dir: 'desc' }],
    groupBy: ['other'],
    aggregates: [{ fieldId: 'other', aggregate: 'sum' }],
    highlight: schema.views[0]!.config.highlight,
  })
  expect(columns.config.value.aggregates).toEqual([
    { fieldId: 'other', aggregate: 'sum' },
  ])
})

it('rebases only user intents against live fields and keeps unrelated server widths', () => {
  const schema = fixture()
  const result = applyColumnIntents(
    schema.views[0]!.config,
    [
      { kind: 'order', ids: ['system', 'other', 'name', 'deleted'] },
      { kind: 'visible', id: 'other', visible: false },
      { kind: 'width', id: 'deleted', width: 90 },
      { kind: 'width', id: 'name', width: 1500 },
      { kind: 'frozen', count: 2 },
    ],
    schema,
  )
  expect(result.fields.map((field) => field.id)).toEqual([
    'system',
    'other',
    'name',
  ])
  expect(result.fields.find((field) => field.id === 'name')?.width).toBe(1000)
  expect(result.fields.find((field) => field.id === 'other')?.width).toBe(200)
  expect(result.frozen).toBe(1)
})

it('coalesces resize/order/hide/freeze and sends one versioned bounded config save', async () => {
  const fetch = vi.fn(async (_url: unknown, init?: RequestInit) =>
    Response.json({
      view: {
        ...fixture().views[0],
        version: 2,
        config: JSON.parse(String(init?.body)).config,
      },
      eventId: 'event',
    }),
  )
  vi.stubGlobal('fetch', fetch)
  const { columns } = mounted()
  columns.change({ kind: 'width', id: 'name', width: 250 })
  columns.change({ kind: 'order', ids: ['other', 'name', 'system'] })
  columns.change({ kind: 'visible', id: 'system', visible: true })
  columns.change({ kind: 'frozen', count: 2 })
  expect(fetch).not.toHaveBeenCalled()
  await debounce()
  expect(fetch).toHaveBeenCalledTimes(1)
  expect(fetch.mock.calls[0]?.[0]).toBe('/api/v1/views/v')
  expect(fetch.mock.calls[0]?.[1]?.headers).toMatchObject({
    'If-Match': '1',
    'X-PT-CSRF': '1',
  })
  expect(JSON.parse(String(fetch.mock.calls[0]?.[1]?.body))).toEqual({
    config: columns.config.value,
  })
  expect(columns.saving.value).toBe(false)
})

it.each(['reader', 'locked'])(
  'keeps %s column changes strictly local without saved state',
  async (mode) => {
    const fetch = vi.fn()
    vi.stubGlobal('fetch', fetch)
    const schema = fixture()
    if (mode === 'locked') schema.views[0]!.locked = true
    const { columns, state } = mounted(schema)
    state.canWrite = false
    await flushPromises()
    columns.change({ kind: 'visible', id: 'name', visible: false })
    columns.change({ kind: 'width', id: 'other', width: 350 })
    columns.change({ kind: 'frozen', count: 1 })
    await debounce()
    expect(fetch).not.toHaveBeenCalled()
    expect(columns.canPersist.value).toBe(false)
    expect(
      columns.config.value.fields.find((field) => field.id === 'other')?.width,
    ).toBe(350)
    expect(columns.config.value.frozen).toBe(1)
  },
)

it('allows an owner to save edits to a locked shared view', async () => {
  const fetch = vi.fn(async (_url: unknown, init?: RequestInit) =>
    Response.json({
      view: {
        ...fixture().views[0],
        locked: true,
        version: 2,
        config: JSON.parse(String(init?.body)).config,
      },
    }),
  )
  vi.stubGlobal('fetch', fetch)
  const schema = fixture()
  schema.views[0]!.locked = true
  const { columns } = mounted(schema)
  columns.change({ kind: 'visible', id: 'name', visible: false })
  await debounce()
  expect(fetch).toHaveBeenCalledTimes(1)
  expect(columns.canPersist.value).toBe(true)
})

it('allows a reader to persist edits to their own personal view', async () => {
  const fetch = vi.fn(async (_url: unknown, init?: RequestInit) =>
    Response.json({
      view: {
        ...fixture().views[0],
        scope: 'personal',
        version: 2,
        config: JSON.parse(String(init?.body)).config,
      },
    }),
  )
  vi.stubGlobal('fetch', fetch)
  const schema = fixture()
  schema.views[0]!.scope = 'personal'
  const { columns, state } = mounted(schema)
  state.canWrite = true
  columns.change({ kind: 'visible', id: 'name', visible: false })
  await debounce()
  expect(fetch).toHaveBeenCalledTimes(1)
})

it('does not block more intents during a save and serializes the next version', async () => {
  let resolve!: (response: Response) => void
  const fetch = vi
    .fn()
    .mockImplementationOnce(
      () =>
        new Promise<Response>((done) => {
          resolve = done
        }),
    )
    .mockImplementation(async (_url: unknown, init?: RequestInit) =>
      Response.json({
        view: {
          ...fixture().views[0],
          version: 3,
          config: JSON.parse(String(init?.body)).config,
        },
      }),
    )
  vi.stubGlobal('fetch', fetch)
  const { columns } = mounted()
  columns.change({ kind: 'width', id: 'name', width: 300 })
  await vi.advanceTimersByTimeAsync(columnSaveDelay)
  expect(columns.saving.value).toBe(true)
  columns.change({ kind: 'visible', id: 'other', visible: false })
  const config = applyColumnIntents(
    fixture().views[0]!.config,
    [{ kind: 'width', id: 'name', width: 300 }],
    fixture(),
  )
  resolve(
    Response.json({ view: { ...fixture().views[0], version: 2, config } }),
  )
  await flushPromises()
  await debounce()
  expect(fetch).toHaveBeenCalledTimes(2)
  expect(fetch.mock.calls[1]?.[1]?.headers['If-Match']).toBe('2')
  expect(
    columns.config.value.fields.find((field) => field.id === 'other')?.visible,
  ).toBe(false)
})

it('restores authoritative config after rejection and reapplies retained intent only on successful retry', async () => {
  const schema = fixture()
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(
      Response.json({ error: { code: 'ERR_VALIDATION' } }, { status: 400 }),
    )
    .mockResolvedValueOnce(Response.json(schema))
    .mockImplementation(async (_url: unknown, init?: RequestInit) =>
      Response.json({
        view: {
          ...schema.views[0],
          version: 2,
          config: JSON.parse(String(init?.body)).config,
        },
      }),
    )
  vi.stubGlobal('fetch', fetch)
  const { columns } = mounted()
  columns.change({ kind: 'width', id: 'name', width: 300 })
  await debounce()
  expect(columns.failure.value).toBe('translated failure')
  expect(columns.config.value.fields[0]?.width).toBe(180)
  expect(columns.retryable.value).toBe(true)
  columns.retry()
  await flushPromises()
  expect(columns.config.value.fields[0]?.width).toBe(300)
  expect(fetch).toHaveBeenCalledTimes(3)
})

it('reloads a conflict, drops removed fields, preserves concurrent changes and retries once', async () => {
  const latest = fixture()
  latest.fields = latest.fields.filter((field) => field.id !== 'other')
  latest.views[0]!.version = 7
  latest.views[0]!.config = {
    fields: [{ id: 'name', visible: true, width: 480 }],
    frozen: 0,
  }
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(
      Response.json(
        { error: { code: 'ERR_VERSION_CONFLICT' } },
        { status: 409 },
      ),
    )
    .mockResolvedValueOnce(Response.json(latest))
    .mockImplementation(async (_url: unknown, init?: RequestInit) =>
      Response.json({
        view: {
          ...latest.views[0],
          version: 8,
          config: JSON.parse(String(init?.body)).config,
        },
      }),
    )
  vi.stubGlobal('fetch', fetch)
  const { columns } = mounted()
  columns.change({ kind: 'visible', id: 'system', visible: true })
  columns.change({ kind: 'width', id: 'other', width: 90 })
  await debounce()
  expect(
    columns.config.value.fields.some((field) => field.id === 'other'),
  ).toBe(false)
  expect(
    columns.config.value.fields.find((field) => field.id === 'system')?.visible,
  ).toBe(false)
  await debounce()
  expect(fetch).toHaveBeenCalledTimes(3)
  expect(fetch.mock.calls[2]?.[1]?.headers['If-Match']).toBe('7')
  expect(
    columns.config.value.fields.find((field) => field.id === 'system')?.visible,
  ).toBe(true)
  expect(columns.config.value.fields[0]?.width).toBe(480)
})

it.each(['unmount', 'identity', 'permission'])(
  'ignores a late response after %s even when fetch ignores cancellation',
  async (mode) => {
    let resolve!: (response: Response) => void
    const fetch = vi.fn(
      () =>
        new Promise<Response>((done) => {
          resolve = done
        }),
    )
    vi.stubGlobal('fetch', fetch)
    const { columns, state, updated } = mounted()
    columns.change({ kind: 'width', id: 'name', width: 300 })
    await vi.advanceTimersByTimeAsync(columnSaveDelay)
    if (mode === 'unmount') {
      wrapper!.unmount()
      wrapper = undefined
    } else if (mode === 'identity') state.identity = ''
    else state.canWrite = false
    await flushPromises()
    resolve(
      Response.json({
        view: {
          ...fixture().views[0],
          version: 2,
          config: columns.config.value,
        },
      }),
    )
    await flushPromises()
    expect(updated).not.toHaveBeenCalled()
    expect(fetch).toHaveBeenCalledTimes(1)
  },
)

it('surfaces reload failure and never leaves optimistic column values', async () => {
  const fetch = vi.fn().mockRejectedValue(new Error('offline'))
  vi.stubGlobal('fetch', fetch)
  const { columns, failure } = mounted()
  columns.change({ kind: 'width', id: 'name', width: 300 })
  await debounce()
  expect(columns.config.value.fields[0]?.width).toBe(180)
  expect(columns.retryable.value).toBe(true)
  expect(failure).toHaveBeenCalled()
})
