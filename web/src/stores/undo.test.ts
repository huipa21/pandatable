import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { loadLocale, i18n } from '../i18n'
import type { DataRow, RowValue, TableView } from '../api/types'
import { navUser, tableSchema } from '../test/navigation'
import { useAuthStore } from './auth'
import {
  linkUndoSteps,
  patchUndoStep,
  useUndoStore,
  viewUndoStep,
} from './undo'

let pinia: ReturnType<typeof createPinia>
let data: Map<string, DataRow>
let failDelete: boolean
let raceAfterRead: boolean

beforeEach(async () => {
  pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().setUser(navUser)
  await loadLocale('en')
  i18n.global.locale.value = 'en'
  data = new Map()
  failDelete = false
  raceAfterRead = false
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input)
      const method = init?.method ?? 'GET'
      const body = init?.body
        ? (JSON.parse(String(init.body)) as Record<string, unknown>)
        : {}
      if (path.endsWith('/history'))
        return Response.json({
          items: [{ authorName: 'Bob', changes: { name: { n: 'theirs' } } }],
          hasMore: false,
        })
      if (method === 'GET') {
        const row = data.get(path.split('/').at(-1)!)
        if (!row)
          return Response.json(
            { error: { code: 'ERR_NOT_FOUND' } },
            { status: 404 },
          )
        const response = structuredClone(row)
        if (raceAfterRead) {
          raceAfterRead = false
          row.fields.name = 'changed again'
        }
        return Response.json(response)
      }
      if (method === 'PATCH') {
        const rows = body.rows as {
          id: string
          fields: Record<string, RowValue>
          expected: Record<string, RowValue>
        }[]
        for (const patch of rows) {
          const current = data.get(patch.id)
          if (
            !current ||
            Object.entries(patch.expected).some(
              ([key, value]) =>
                JSON.stringify(current.fields[key]) !== JSON.stringify(value),
            )
          )
            return Response.json(
              { error: { code: 'ERR_UNDO_CONFLICT' } },
              { status: 409 },
            )
        }
        const updated = rows.map((patch) => {
          const row = data.get(patch.id)!
          row.fields = { ...row.fields, ...patch.fields }
          return structuredClone(row)
        })
        return Response.json({ rows: updated, eventId: 'event' })
      }
      if (method === 'DELETE') {
        if (failDelete)
          return Response.json(
            { error: { code: 'ERR_RESTRICT_VIOLATION' } },
            { status: 409 },
          )
        const rows = (body.ids as string[]).map((id) => {
          const row = structuredClone(data.get(id)!)
          data.delete(id)
          return row
        })
        return Response.json({ rows, eventId: 'event' })
      }
      if (path.endsWith('/rows:restore')) {
        const rows = body.rows as DataRow[]
        for (const row of rows) data.set(row.id, structuredClone(row))
        return Response.json({ rows, eventId: 'event' })
      }
      throw new Error(`Unexpected request: ${method} ${path}`)
    }),
  )
})

afterEach(() => {
  disposePinia(pinia)
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

function row(id: string, value: string): DataRow {
  return { id, fields: { name: value } }
}

function recordEdit(id: string, before: string, after: string) {
  const updated = row(id, after)
  data.set(id, updated)
  useUndoStore().record([
    patchUndoStep('table', [{ id, fields: { name: before } }], [updated]),
  ])
}

describe('per-tab undo history', () => {
  it('bounds history to 50 actions, reverses order and invalidates redo', async () => {
    const undo = useUndoStore()
    for (let index = 1; index <= 51; index++)
      recordEdit('1', `v${index - 1}`, `v${index}`)
    expect(undo.undoStack).toHaveLength(50)
    await undo.undo()
    expect(data.get('1')?.fields.name).toBe('v50')
    await undo.undo()
    expect(data.get('1')?.fields.name).toBe('v49')
    await undo.redo()
    expect(data.get('1')?.fields.name).toBe('v50')
    expect(undo.redoStack).toHaveLength(1)
    recordEdit('1', 'v50', 'new action')
    expect(undo.redoStack).toHaveLength(0)
    expect(
      patchUndoStep(
        'table',
        [{ id: '1', fields: { name: 'same' } }],
        [row('1', 'same')],
      ),
    ).toBeNull()
    undo.record([null])
    expect(undo.undoStack).toHaveLength(50)
  })

  it('splits oversized guarded patches while keeping one undoable action', async () => {
    const undo = useUndoStore()
    const large = '\u{1f600}'.repeat(100_000)
    const previous = Array.from({ length: 8 }, (_, index) => {
      const id = String(index)
      data.set(id, row(id, 'new'))
      return { id, fields: { name: large } }
    })
    undo.record([patchUndoStep('table', previous, [...data.values()])])
    expect(undo.undoStack).toHaveLength(1)
    expect(undo.undoStack[0]!.steps).toHaveLength(2)

    await undo.undo()
    expect([...data.values()].every((item) => item.fields.name === large)).toBe(
      true,
    )
    expect(undo.redoStack).toHaveLength(1)
    await undo.redo()
    expect([...data.values()].every((item) => item.fields.name === 'new')).toBe(
      true,
    )
    const patches = vi
      .mocked(fetch)
      .mock.calls.filter(([, init]) => init?.method === 'PATCH')
    expect(patches).toHaveLength(4)
    for (const [path, init] of patches) {
      expect(String(path)).toContain('/rows:guarded')
      expect(
        new TextEncoder().encode(String(init?.body)).length,
      ).toBeLessThanOrEqual(2 * 1024 * 1024)
    }
  })

  it.each([false, true])(
    'rebounds a confirmed conflict when newer values increase the payload (fail later chunk: %s)',
    async (failLaterChunk) => {
      const undo = useUndoStore()
      const large = '\u{1f600}'.repeat(100_000)
      const previous = Array.from({ length: 8 }, (_, index) => {
        const id = String(index)
        data.set(id, row(id, 'mine'))
        return { id, fields: { name: 'before' } }
      })
      undo.record([patchUndoStep('table', previous, [...data.values()])])
      for (const item of data.values()) item.fields.name = large
      const confirm = vi.fn(() => true)
      vi.stubGlobal('confirm', confirm)
      const fetcher = vi.mocked(fetch)
      const base = fetcher.getMockImplementation()
      if (!base) throw new Error('Missing fetch implementation')
      let patches = 0
      fetcher.mockImplementation(async (input, init) => {
        if (init?.method === 'PATCH' && ++patches === 3 && failLaterChunk)
          return Response.json(
            { error: { code: 'ERR_UNDO_CONFLICT' } },
            { status: 409 },
          )
        return base(input, init)
      })

      await undo.undo()
      expect(confirm).toHaveBeenCalledOnce()
      if (failLaterChunk) {
        expect(
          [...data.values()].every((item) => item.fields.name === large),
        ).toBe(true)
        expect(patches).toBe(4)
        expect(undo.undoStack).toHaveLength(1)
        expect(undo.inconsistent).toBe(false)
        expect(undo.error).toMatchObject({ code: 'ERR_UNDO_CONFLICT' })
      } else {
        expect(
          [...data.values()].every((item) => item.fields.name === 'before'),
        ).toBe(true)
        expect(undo.redoStack[0]!.steps).toHaveLength(2)
        await undo.redo()
        expect(
          [...data.values()].every((item) => item.fields.name === large),
        ).toBe(true)
      }
      for (const [, init] of fetcher.mock.calls.filter(
        ([, options]) => options?.method === 'PATCH',
      )) {
        expect(
          new TextEncoder().encode(String(init?.body)).length,
        ).toBeLessThanOrEqual(2 * 1024 * 1024)
      }
    },
  )

  it('keeps history on conflict cancellation and restores the overwritten value on redo', async () => {
    const undo = useUndoStore()
    recordEdit('1', 'before', 'mine')
    data.get('1')!.fields.name = 'theirs'
    const confirm = vi.fn(() => false)
    vi.stubGlobal('confirm', confirm)
    await undo.undo()
    expect(confirm).toHaveBeenCalledWith(
      'Bob changed a cell since your edit. Overwrite their value?',
    )
    expect(data.get('1')?.fields.name).toBe('theirs')
    expect(undo.undoStack).toHaveLength(1)
    expect(undo.redoStack).toHaveLength(0)
    expect(undo.error).toBeNull()

    confirm.mockReturnValue(true)
    await undo.undo()
    expect(data.get('1')?.fields.name).toBe('before')
    await undo.redo()
    expect(data.get('1')?.fields.name).toBe('theirs')
  })

  it('rejects another change between confirmation and the guarded write', async () => {
    const undo = useUndoStore()
    recordEdit('1', 'before', 'mine')
    data.get('1')!.fields.name = 'theirs'
    raceAfterRead = true
    vi.stubGlobal('confirm', () => true)
    await undo.undo()
    expect(data.get('1')?.fields.name).toBe('changed again')
    expect(undo.undoStack).toHaveLength(1)
    expect(undo.redoStack).toHaveLength(0)
    expect(undo.error).toMatchObject({ code: 'ERR_UNDO_CONFLICT' })
  })

  it('groups a paste affecting existing and new rows into one reversible action', async () => {
    const undo = useUndoStore()
    data.set('1', row('1', 'pasted'))
    data.set('2', {
      id: '2',
      fields: { name: 'new row', files: [{ id: 'file', name: 'file.pdf' }] },
      commentCount: 1,
    })
    undo.record([
      patchUndoStep(
        'table',
        [{ id: '1', fields: { name: 'old' } }],
        [data.get('1')!],
      ),
      { kind: 'delete', tableId: 'table', ids: ['2'] },
    ])
    await undo.undo()
    expect(data.get('1')?.fields.name).toBe('old')
    expect(data.has('2')).toBe(false)
    expect(undo.redoStack).toHaveLength(1)
    await undo.redo()
    expect(data.get('1')?.fields.name).toBe('pasted')
    expect(data.get('2')).toEqual({
      id: '2',
      fields: { name: 'new row', files: [{ id: 'file', name: 'file.pdf' }] },
      commentCount: 1,
    })
  })

  it('stores and sends compact versioned references for large deleted snapshots', async () => {
    const undo = useUndoStore()
    const huge = 'x'.repeat(9 * 1024 * 1024)
    undo.record([
      {
        kind: 'restore',
        tableId: 'table',
        rows: [
          {
            id: '1',
            fields: { name: huge },
            restoreVersion: '101',
            cascaded: [
              {
                tableId: 'child',
                rows: [
                  { id: '2', fields: { name: huge }, restoreVersion: '102' },
                ],
              },
            ],
          },
        ],
      },
    ])
    let payload: unknown
    vi.stubGlobal(
      'fetch',
      vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
        payload = JSON.parse(String(init?.body))
        return Response.json({ rows: [row('1', 'restored')], eventId: 'event' })
      }),
    )
    expect(JSON.stringify(undo.undoStack).length).toBeLessThan(256)
    await undo.undo()
    expect(payload).toEqual({
      rows: [
        {
          id: '1',
          restoreVersion: '101',
          cascaded: [
            {
              tableId: 'child',
              rows: [{ id: '2', restoreVersion: '102' }],
            },
          ],
        },
      ],
    })
    expect(undo.redoStack).toEqual([
      { steps: [{ kind: 'delete', tableId: 'table', ids: ['1'] }] },
    ])
  })

  it('compensates earlier steps and retains the action when a later step fails', async () => {
    const undo = useUndoStore()
    data.set('1', row('1', 'new'))
    data.set('2', row('2', 'inserted'))
    undo.record([
      patchUndoStep(
        'table',
        [{ id: '1', fields: { name: 'old' } }],
        [data.get('1')!],
      ),
      { kind: 'delete', tableId: 'table', ids: ['2'] },
    ])
    failDelete = true
    await undo.undo()
    expect(data.get('1')?.fields.name).toBe('new')
    expect(data.get('2')?.fields.name).toBe('inserted')
    expect(undo.undoStack).toHaveLength(1)
    expect(undo.inconsistent).toBe(false)
    expect(undo.error).toMatchObject({ code: 'ERR_RESTRICT_VIOLATION' })
  })

  it('clears history when identity changes', () => {
    recordEdit('1', 'before', 'after')
    const undo = useUndoStore()
    expect(undo.canUndo).toBe(true)
    useAuthStore().setUser(null)
    expect(undo.undoStack).toHaveLength(0)
    expect(undo.redoStack).toHaveLength(0)
  })

  it.each(['grid', 'form'] as const)(
    'undoes and redoes %s configuration using the current view version',
    async (type) => {
      const schema = tableSchema()
      const before = structuredClone(schema.views[0]!)
      before.type = type
      before.config = {
        fields: [{ id: 'name', visible: true }],
        frozen: 0,
        ...(type === 'form'
          ? {
              form: {
                title: 'Before',
                description: '',
                fields: [],
                afterSubmit: 'message' as const,
              },
            }
          : {}),
      }
      const after: TableView = structuredClone(before)
      after.version = 2
      if (type === 'form') after.config.form!.title = 'After'
      else after.config.fields[0]!.visible = false
      let current = structuredClone(after)
      const fetcher = vi.fn(
        async (_input: RequestInfo | URL, init?: RequestInit) => {
          if (init?.method === 'GET')
            return Response.json({ ...schema, views: [current] })
          expect(new Headers(init?.headers).get('If-Match')).toBe(
            String(current.version),
          )
          const body = JSON.parse(String(init?.body)) as {
            config: Partial<TableView['config']>
          }
          current = {
            ...current,
            version: current.version + 1,
            config: { ...current.config, ...body.config },
          }
          return Response.json({ view: current, eventId: 'event' })
        },
      )
      vi.stubGlobal('fetch', fetcher)
      const undo = useUndoStore()
      undo.record([viewUndoStep(schema.table.id, before, after)])
      await undo.undo()
      expect(current.config).toEqual(before.config)
      await undo.redo()
      expect(current.config).toEqual(after.config)
      expect(current.version).toBe(4)

      if (type === 'form') current.config.form!.title = 'Someone else'
      else current.config.fields[0]!.width = 210
      await undo.undo()
      expect(undo.error).toMatchObject({ code: 'ERR_VERSION_CONFLICT' })
      expect(undo.canUndo).toBe(true)
      expect(
        fetcher.mock.calls.filter(([, init]) => init?.method === 'PATCH'),
      ).toHaveLength(2)
    },
  )
})

it('reverses all actual link deltas, including a displaced one-to-one link', () => {
  expect(
    linkUndoSteps([
      {
        tableId: 't',
        rowId: '8',
        fieldId: 'f',
        targetId: '20',
        action: 'unlink',
      },
      {
        tableId: 't',
        rowId: '10',
        fieldId: 'f',
        targetId: '20',
        action: 'link',
      },
    ]),
  ).toEqual([
    {
      kind: 'link',
      tableId: 't',
      rowId: '10',
      fieldId: 'f',
      add: [],
      remove: ['20'],
    },
    {
      kind: 'link',
      tableId: 't',
      rowId: '8',
      fieldId: 'f',
      add: ['20'],
      remove: [],
    },
  ])
})
