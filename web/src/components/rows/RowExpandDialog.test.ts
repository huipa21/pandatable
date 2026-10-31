import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { i18n, loadLocale } from '../../i18n'
import { useAuthStore } from '../../stores/auth'
import { useNavStore } from '../../stores/nav'
import { useRealtimeStore } from '../../stores/realtime'
import { navUser, tableSchema } from '../../test/navigation'
import type { DataRow } from '../../api/types'
import RowExpandDialog from './RowExpandDialog.vue'

let pinia: ReturnType<typeof createPinia>
let wrapper: VueWrapper | undefined
const id = '9007199254740993'
const original: DataRow = { id, fields: { name: 'Original' } }
let stored: DataRow

beforeEach(async () => {
  pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().setUser(navUser)
  await loadLocale('en')
  await loadLocale('zh-CN')
  i18n.global.locale.value = 'en'
  stored = structuredClone(original)
  Object.defineProperties(window.HTMLDialogElement.prototype, {
    showModal: {
      configurable: true,
      value(this: HTMLDialogElement) {
        this.setAttribute('open', '')
      },
    },
    close: {
      configurable: true,
      value(this: HTMLDialogElement) {
        this.removeAttribute('open')
      },
    },
  })
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  disposePinia(pinia)
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

async function setup(
  schema = tableSchema(),
  fetcher = vi.fn(async (_url: string, init?: RequestInit) => {
    if (init?.method === 'PATCH') {
      const body = JSON.parse(String(init.body)) as {
        fields: DataRow['fields']
      }
      stored = { id, fields: { ...stored.fields, ...body.fields } }
      return Response.json({ row: stored, eventId: 'event' })
    }
    return Response.json(stored)
  }),
  rowId: string | null = id,
) {
  vi.stubGlobal('fetch', fetcher)
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }],
  })
  await router.push(`/t/t/v/v/r/${id}`)
  wrapper = mount(RowExpandDialog, {
    props: { schema, viewId: 'v', rowId, identity: navUser.id },
    global: { plugins: [pinia, i18n, router], stubs: { CommentPanel: true } },
    attachTo: document.body,
  })
  await flushPromises()
  return { wrapper, fetcher, router }
}

function schemaWith(kind: string, options: Record<string, unknown> = {}) {
  const schema = tableSchema()
  schema.fields[0]!.uiType = kind
  schema.fields[0]!.options = options
  return schema
}

const textCases = [
  ['text', {}, '中文 text', '中文 text'],
  ['long_text', {}, 'line one\nline two', 'line one\nline two'],
  ['integer', { size: 'big' }, '9223372036854775807', '9223372036854775807'],
  [
    'decimal',
    { precision: 38, scale: 6 },
    '123456789012345678901234567890.123456',
    '123456789012345678901234567890.123456',
  ],
  ['float', { size: 'real' }, '1.23456789', 1.2345679],
  ['date', {}, '2026-10-27', '2026-10-27'],
  [
    'datetime',
    {},
    '2026-10-27T15:00:00.123456+08:00',
    '2026-10-27T07:00:00.123456Z',
  ],
  ['email', {}, 'valid@example.com', 'valid@example.com'],
  ['url', {}, 'https://example.com/a', 'https://example.com/a'],
] as const

it('reports field focus in the row dialog and clears it on blur', async () => {
  const { wrapper } = await setup()
  const setFocus = vi.spyOn(useRealtimeStore(), 'setFocus')
  const button = wrapper.get('.row-field button')
  await button.trigger('focusin')
  expect(setFocus).toHaveBeenLastCalledWith('t', { rowId: id, fieldId: 'name' })
  await button.trigger('focusout', { relatedTarget: document.body })
  expect(setFocus).toHaveBeenLastCalledWith('t', null)
})

it('opens the localized History tab with keyboard navigation', async () => {
  const fetcher = vi.fn(async (url: string) => {
    if (url.endsWith('/history'))
      return Response.json({
        items: [
          {
            id: '1',
            authorId: navUser.id,
            authorName: 'Alice',
            action: 'update',
            changes: { name: { o: 'Original', n: 'Changed' } },
            createdAt: '2026-10-31T12:00:00Z',
          },
        ],
        hasMore: false,
      })
    return Response.json(stored)
  })
  const { wrapper } = await setup(tableSchema(), fetcher)
  const tabs = wrapper.findAll('[role="tab"]')
  expect(tabs.map((tab) => tab.text())).toEqual(['Details', 'History'])
  expect(tabs[0]!.attributes('aria-selected')).toBe('true')
  await wrapper
    .get('[role="tablist"]')
    .trigger('keydown', { key: 'ArrowRight' })
  await flushPromises()
  expect(tabs[1]!.attributes('aria-selected')).toBe('true')
  expect(wrapper.get('.history').text()).toContain('Original → Changed')
  expect(wrapper.get('.history').text()).toContain('Alice')
  i18n.global.locale.value = 'zh-CN'
  await flushPromises()
  expect(wrapper.get('.history').text()).toContain('历史记录')
  await wrapper.get('[role="tablist"]').trigger('keydown', { key: 'ArrowLeft' })
  expect(tabs[0]!.attributes('aria-selected')).toBe('true')
})

it('preserves active edits, identifies remote changes, and refreshes after cancel or save', async () => {
  class Socket {
    static OPEN = 1
    static current: Socket
    readyState = 0
    onopen: (() => void) | null = null
    onmessage: ((event: MessageEvent) => void) | null = null
    onclose: (() => void) | null = null

    constructor() {
      Socket.current = this
    }

    send() {}

    close() {
      this.readyState = 3
      this.onclose?.()
    }

    message(event: object) {
      this.onmessage?.(
        new MessageEvent('message', { data: JSON.stringify(event) }),
      )
    }
  }
  vi.stubGlobal('WebSocket', Socket)
  const { wrapper, fetcher } = await setup()
  const realtime = useRealtimeStore()
  realtime.start(navUser.id)
  const socket = Socket.current
  socket.readyState = Socket.OPEN
  socket.onopen?.()
  socket.message({ t: 'subscribed', tableId: 't' })
  await flushPromises()
  fetcher.mockClear()

  await wrapper.get('.row-field button').trigger('click')
  const editor = wrapper.get('.field-editor input')
  await editor.setValue('Local draft')
  stored.fields.name = 'Bob saved'
  socket.message({
    t: 'row.updated',
    tableId: 't',
    ids: [id],
    fields: ['name'],
    by: 'bob',
    byName: 'Bob',
  })
  await flushPromises()
  expect(wrapper.get('[role="status"]').text()).toContain('Changed by Bob')
  expect((editor.element as HTMLInputElement).value).toBe('Local draft')
  expect(fetcher).not.toHaveBeenCalled()
  await wrapper.get('fieldset > button').trigger('click')
  await flushPromises()
  expect(wrapper.text()).toContain('Bob saved')

  await wrapper.get('.row-field button').trigger('click')
  await wrapper.get('.field-editor input').setValue('Local wins')
  stored.fields.name = 'Bob again'
  socket.message({
    t: 'row.updated',
    tableId: 't',
    ids: [id],
    fields: ['name'],
    by: 'bob',
    byName: 'Bob',
  })
  await wrapper.get('.field-editor button').trigger('click')
  await flushPromises()
  expect(stored.fields.name).toBe('Local wins')
  expect(wrapper.text()).toContain('Local wins')
  realtime.stop()
})

it.each(textCases)(
  'saves and reloads %s using the existing codec and exact wire value',
  async (kind, options, input, expected) => {
    const schema = schemaWith(kind, options)
    stored.fields.name = null
    const { wrapper, fetcher } = await setup(schema)
    await wrapper.get('.row-field button').trigger('click')
    await wrapper
      .get('.field-editor input, .field-editor textarea')
      .setValue(input)
    await wrapper.get('.field-editor button').trigger('click')
    await flushPromises()
    expect(stored.fields.name).toEqual(expected)
    expect(wrapper.emitted('saved')).toHaveLength(1)
    expect(fetcher).toHaveBeenLastCalledWith(
      `/api/v1/tables/t/rows/${id}`,
      expect.objectContaining({
        method: 'PATCH',
        cache: 'no-store',
        credentials: 'same-origin',
        headers: {
          'Content-Type': 'application/json',
          'X-PT-CSRF': '1',
          'X-PT-Event-ID': expect.stringMatching(/^[0-9a-f]{32}$/),
        },
        body: JSON.stringify({ fields: { name: expected } }),
        signal: expect.any(AbortSignal),
      }),
    )

    await wrapper.setProps({ rowId: '9007199254740994' })
    await wrapper.setProps({ rowId: id })
    await flushPromises()
    await wrapper.get('.row-field button').trigger('click')
    expect(
      (
        wrapper.get('.field-editor input, .field-editor textarea')
          .element as HTMLInputElement
      ).value,
    ).not.toBe('')
  },
)

it.each(textCases)(
  'creates %s through the same editor without an intermediate write',
  async (kind, options, input, expected) => {
    const fetcher = vi.fn<typeof fetch>(async () =>
      Response.json({
        rows: [{ id, fields: { name: expected } }],
        eventId: 'event',
      }),
    )
    const { wrapper } = await setup(schemaWith(kind, options), fetcher, null)
    expect(fetcher).not.toHaveBeenCalled()
    await wrapper.get('.row-field button').trigger('click')
    await wrapper
      .get('.field-editor input, .field-editor textarea')
      .setValue(input)
    await wrapper.get('.field-editor button').trigger('click')
    expect(fetcher).not.toHaveBeenCalled()
    await wrapper.get('.create-row').trigger('click')
    await flushPromises()
    expect(fetcher).toHaveBeenCalledWith(
      '/api/v1/tables/t/rows',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ rows: [{ fields: { name: expected } }] }),
      }),
    )
    expect(wrapper.emitted('created')).toEqual([[id]])
    expect(wrapper.emitted('saved')).toBeUndefined()
  },
)

it.each([524288, 524289])(
  'enforces the exact %s-byte UTF-8 atomic create bound without splitting',
  async (bytes) => {
    const schema = schemaWith('long_text')
    schema.fields = Array.from({ length: 6 }, (_, index) => ({
      ...schema.fields[0]!,
      id: `notes${index}`,
      name: `Notes ${index}`,
      position: index,
    }))
    const fetcher = vi.fn<typeof fetch>(async () =>
      Response.json({ rows: [{ id, fields: {} }] }),
    )
    const { wrapper } = await setup(schema, fetcher, null)
    const body: { rows: { fields: Record<string, string> }[] } = {
      rows: [{ fields: {} }],
    }
    for (const field of schema.fields) body.rows[0]!.fields[field.id] = ''
    const overhead = new TextEncoder().encode(JSON.stringify(body)).length
    for (const [index, field] of schema.fields.entries()) {
      const section = wrapper.get(`[data-field-id="${field.id}"]`)
      await section.get('button').trigger('click')
      await section
        .get('textarea')
        .setValue('x'.repeat(index < 5 ? 100000 : bytes - overhead - 500000))
      await section.get('.field-editor button').trigger('click')
    }
    await wrapper.get('.create-row').trigger('click')
    await flushPromises()
    expect(fetcher).toHaveBeenCalledTimes(bytes === 524288 ? 1 : 0)
    if (bytes === 524288)
      expect(
        new TextEncoder().encode(String(fetcher.mock.calls[0]?.[1]?.body))
          .length,
      ).toBe(bytes)
    else expect(wrapper.get('[role="alert"]').text()).toContain('512 KiB')
  },
)

it('omits untouched defaults, permits explicit null, and preserves drafts after failed creation', async () => {
  const schema = tableSchema()
  schema.fields[0]!.default = 'Default name'
  schema.fields.push({
    ...schema.fields[0]!,
    id: 'untouched',
    name: 'Untouched',
    default: 'Physical default',
  })
  const fetcher = vi.fn<typeof fetch>(async () =>
    Response.json({ error: { code: 'ERR_REQUIRED_FIELD' } }, { status: 400 }),
  )
  const { wrapper } = await setup(schema, fetcher, null)
  expect(wrapper.text()).toContain('Default name')
  await wrapper.get('.row-field button').trigger('click')
  await wrapper.get('input').setValue('')
  await wrapper.get('.field-editor button').trigger('click')
  await wrapper.get('.create-row').trigger('click')
  await flushPromises()
  expect(JSON.parse(String(fetcher.mock.calls[0]?.[1]?.body))).toEqual({
    rows: [{ fields: { name: null } }],
  })
  expect(wrapper.emitted('created')).toBeUndefined()
  expect(wrapper.get('[role="alert"]').text()).toContain('required field')
  expect(wrapper.text()).toContain('Creation has not been confirmed')
  await wrapper.get('.row-field button').trigger('click')
  expect((wrapper.get('input').element as HTMLInputElement).value).toBe('')
})

it.each(['view', 'schema', 'identity', 'permission', 'close'])(
  'ignores late creation after %s changes',
  async (change) => {
    let release!: (response: Response) => void
    const fetcher = vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          release = resolve
        }),
    )
    const { wrapper } = await setup(tableSchema(), fetcher, null)
    await wrapper.get('.create-row').trigger('click')
    if (change === 'view') await wrapper.setProps({ viewId: 'other' })
    else if (change === 'schema')
      await wrapper.setProps({
        schema: { ...tableSchema(), schemaVersion: 2 },
      })
    else if (change === 'identity')
      useAuthStore().setUser({ ...navUser, id: 'other' })
    else if (change === 'permission')
      await wrapper.setProps({
        schema: { ...tableSchema(), myLevel: 'read' },
      })
    else await wrapper.get('dialog').trigger('cancel')
    release(Response.json({ rows: [{ id, fields: {} }] }))
    await flushPromises()
    expect(wrapper.emitted('created')).toBeUndefined()
  },
)
it.each(['checkbox', 'single_select', 'multi_select'])(
  'uses the native %s editor',
  async (kind) => {
    const schema = schemaWith(
      kind,
      kind === 'checkbox'
        ? {}
        : {
            choices: [
              { id: 'a', label: 'Alpha' },
              { id: 'b', label: 'Beta' },
            ],
          },
    )
    stored.fields.name = kind === 'checkbox' ? false : null
    const { wrapper } = await setup(schema)
    await wrapper.get('.row-field button').trigger('click')
    if (kind === 'checkbox') await wrapper.get('input').setValue(true)
    else
      await wrapper
        .get('select')
        .setValue(kind === 'single_select' ? 'Beta' : ['Beta', 'Alpha'])
    if (kind === 'multi_select')
      await wrapper.get('.field-editor button').trigger('click')
    await flushPromises()
    expect(stored.fields.name).toEqual(
      kind === 'checkbox'
        ? true
        : kind === 'single_select'
          ? 'Beta'
          : ['Alpha', 'Beta'],
    )
  },
)

it('shows live fields in metadata order and resolves system user labels through the table directory', async () => {
  const schema = tableSchema()
  for (const [index, kind] of [
    'id',
    'created_at',
    'created_by',
    'updated_at',
    'updated_by',
  ].entries()) {
    schema.fields.push({
      ...schema.fields[0]!,
      id: kind,
      uiType: kind,
      name: kind,
      isSystem: true,
      position: 1e9 + index,
    })
    stored.fields[kind] = kind.endsWith('_by')
      ? '0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b'
      : kind === 'id'
        ? id
        : '2026-10-27T00:00:00.123456Z'
  }
  schema.fields.push({
    ...schema.fields[0]!,
    id: 'hidden',
    name: 'Hidden',
    uiType: 'single_select',
    options: { choices: [] },
    position: 2048,
  })
  stored.fields.hidden = 'Removed'
  const { wrapper, fetcher } = await setup(schema)
  expect(wrapper.findAll('.row-field')).toHaveLength(7)
  expect(wrapper.findAll('.row-field h3').map((item) => item.text())).toEqual([
    'Name',
    'Hidden',
    'id',
    'created_at',
    'created_by',
    'updated_at',
    'updated_by',
  ])
  expect(wrapper.findAll('.row-field button')).toHaveLength(2)
  expect(wrapper.get('.chip-unknown').text()).toBe('Removed')
  expect(fetcher).toHaveBeenCalledWith(
    '/api/v1/tables/t/directory/user-labels',
    expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({
        ids: ['0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b'],
      }),
    }),
  )
  i18n.global.locale.value = 'zh-CN'
  await flushPromises()
  expect(wrapper.text()).toContain('只读系统字段')
})

it.each([
  ['integer', 'not an integer'],
  ['email', 'invalid@'],
])('does not persist codec-invalid %s', async (kind, input) => {
  stored.fields.name = null
  const { wrapper, fetcher } = await setup(schemaWith(kind))
  await wrapper.get('.row-field button').trigger('click')
  await wrapper.get('.field-editor input').setValue(input)
  await wrapper.get('.field-editor button').trigger('click')
  await flushPromises()
  expect(wrapper.get('.field-editor [role="alert"]').text()).not.toBe('')
  expect(fetcher).toHaveBeenCalledTimes(1)
  expect(wrapper.emitted('saved')).toBeUndefined()
})

it('rejects UTF-8 oversized single-row bodies without truncation or a request', async () => {
  stored.fields.name = null
  const { wrapper, fetcher } = await setup(schemaWith('long_text'))
  await wrapper.get('.row-field button').trigger('click')
  await wrapper.get('textarea').setValue('中'.repeat(22000))
  await wrapper.get('.field-editor button').trigger('click')
  await flushPromises()
  expect(wrapper.get('[role="alert"]').text()).toContain('65,536')
  expect(fetcher).toHaveBeenCalledTimes(1)
})

it.each([65536, 65537])(
  'enforces the exact %i-byte single-row boundary',
  async (bytes) => {
    stored.fields.name = null
    const { wrapper, fetcher } = await setup(schemaWith('long_text'))
    const overhead = new TextEncoder().encode(
      JSON.stringify({ fields: { name: '' } }),
    ).length
    await wrapper.get('.row-field button').trigger('click')
    await wrapper.get('textarea').setValue('x'.repeat(bytes - overhead))
    await wrapper.get('.field-editor button').trigger('click')
    await flushPromises()
    expect(fetcher).toHaveBeenCalledTimes(bytes === 65536 ? 2 : 1)
    expect(wrapper.emitted('saved')?.length ?? 0).toBe(bytes === 65536 ? 1 : 0)
  },
)

it('keeps failed writes explicit, preserves the editor, and never reports success', async () => {
  const fetcher = vi.fn(async (_url: string, init?: RequestInit) =>
    init?.method === 'PATCH'
      ? Response.json(
          {
            error: {
              code: 'ERR_UNIQUE_CONFLICT',
              message: 'secret database details',
            },
          },
          { status: 409 },
        )
      : Response.json(stored),
  )
  const { wrapper } = await setup(tableSchema(), fetcher)
  await wrapper.get('.row-field button').trigger('click')
  await wrapper.get('input').setValue('Conflict')
  await wrapper.get('.field-editor button').trigger('click')
  await flushPromises()
  expect(wrapper.get('[role="alert"]').text()).toContain('unique field')
  expect(wrapper.text()).not.toContain('secret')
  expect(wrapper.find('.field-editor').exists()).toBe(true)
  expect(wrapper.emitted('saved')).toBeUndefined()
})

it('reader and identity mismatch never expose write controls', async () => {
  const schema = tableSchema()
  schema.myLevel = 'read'
  const { wrapper, fetcher } = await setup(schema)
  expect(wrapper.findAll('.row-field button')).toHaveLength(0)
  useAuthStore().setUser({ ...navUser, id: 'someone-else' })
  await flushPromises()
  expect(wrapper.findAll('.row-field')).toHaveLength(0)
  expect(fetcher).toHaveBeenCalledTimes(1)
})

it('does not re-fetch on identical schema refreshes, but updates permission controls', async () => {
  const { wrapper, fetcher } = await setup()
  await wrapper.setProps({ schema: { ...tableSchema(), myLevel: 'read' } })
  expect(fetcher).toHaveBeenCalledTimes(1)
  expect(wrapper.findAll('.row-field button')).toHaveLength(0)
  await wrapper.setProps({ schema: tableSchema() })
  expect(wrapper.findAll('.row-field button')).toHaveLength(1)
  expect(fetcher).toHaveBeenCalledTimes(1)
})

it('reports lost write permission when a realtime ACL refresh interrupts an edit', async () => {
  const { wrapper, fetcher } = await setup()
  await wrapper.get('.row-field button').trigger('click')
  await wrapper.get('.field-editor input').setValue('Unsaved draft')
  await wrapper.setProps({ schema: { ...tableSchema(), myLevel: 'read' } })
  expect(wrapper.get('[role="alert"]').text()).toContain(
    'You do not have permission',
  )
  expect(wrapper.find('.field-editor').exists()).toBe(false)
  expect(fetcher).toHaveBeenCalledTimes(1)
})

it('restarts an in-flight read on a permission change without leaving perpetual loading', async () => {
  let release!: (response: Response) => void
  const fetcher = vi.fn(async () => Response.json(stored))
  fetcher.mockImplementationOnce(
    () =>
      new Promise<Response>((resolve) => {
        release = resolve
      }),
  )
  const { wrapper } = await setup(tableSchema(), fetcher)
  await wrapper.setProps({ schema: { ...tableSchema(), myLevel: 'read' } })
  await flushPromises()
  release(Response.json({ id, fields: { name: 'Late private value' } }))
  await flushPromises()
  expect(wrapper.text()).toContain('Original')
  expect(wrapper.text()).not.toContain('Late private value')
  expect(wrapper.find('[role="status"]').exists()).toBe(false)
  expect(wrapper.findAll('.row-field button')).toHaveLength(0)
})

it.each([
  [401, 'ERR_UNAUTHENTICATED', '/login'],
  [403, 'ERR_PASSWORD_CHANGE_REQUIRED', '/change-password'],
])('uses the existing auth route for %s/%s', async (status, code, path) => {
  const { wrapper, router } = await setup(
    tableSchema(),
    vi.fn(async () => Response.json({ error: { code } }, { status })),
  )
  await flushPromises()
  expect(router.currentRoute.value.path).toBe(path)
  expect(wrapper.findAll('.row-field')).toHaveLength(0)
})

it('reports unsupported DTO shapes rather than inventing empty values', async () => {
  stored.fields.name = { nested: 'unsupported' }
  const { wrapper } = await setup()
  expect(wrapper.get('[role="alert"]').text()).not.toBe('')
  expect(wrapper.findAll('.row-field button')).toHaveLength(0)
})

it.each([400, 403, 404])(
  'shows safe row read failure %s without row content',
  async (status) => {
    vi.spyOn(useNavStore(), 'refresh').mockResolvedValue(true)
    const code =
      status === 400
        ? 'ERR_VALIDATION'
        : status === 403
          ? 'ERR_PERMISSION_DENIED'
          : 'ERR_NOT_FOUND'
    const { wrapper } = await setup(
      tableSchema(),
      vi.fn(async () => Response.json({ error: { code } }, { status })),
    )
    expect(wrapper.get('[role="alert"]').text()).not.toBe('')
    expect(wrapper.findAll('.row-field')).toHaveLength(0)
  },
)

it.each(['row', 'schema', 'identity', 'unmount'])(
  'ignores a late read after %s changes, even when fetch ignores abort',
  async (change) => {
    let release!: (response: Response) => void
    const fetcher = vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          release = resolve
        }),
    )
    const { wrapper } = await setup(tableSchema(), fetcher)
    const oldRelease = release
    if (change === 'row') await wrapper.setProps({ rowId: '2' })
    else if (change === 'schema')
      await wrapper.setProps({ schema: { ...tableSchema(), schemaVersion: 2 } })
    else if (change === 'identity')
      useAuthStore().setUser({ ...navUser, id: 'other' })
    else wrapper.unmount()
    oldRelease(Response.json(original))
    await flushPromises()
    expect(wrapper.text()).not.toContain('Original')
    expect(wrapper.emitted('saved')).toBeUndefined()
  },
)

it('ignores a late successful PATCH on row and identity changes', async () => {
  let release!: (response: Response) => void
  const fetcher = vi.fn(async (_url: string, init?: RequestInit) => {
    if (init?.method === 'PATCH')
      return new Promise<Response>((resolve) => {
        release = resolve
      })
    return Response.json(stored)
  })
  const { wrapper } = await setup(tableSchema(), fetcher)
  await wrapper.get('.row-field button').trigger('click')
  await wrapper.get('input').setValue('Late secret')
  await wrapper.get('.field-editor button').trigger('click')
  await wrapper.setProps({ rowId: '2' })
  useAuthStore().setUser({ ...navUser, id: 'other' })
  release(Response.json({ row: { id, fields: { name: 'Late secret' } } }))
  await flushPromises()
  expect(wrapper.text()).not.toContain('Late secret')
  expect(wrapper.emitted('saved')).toBeUndefined()
})

it('cancels the current draft and closes with native Escape cancellation', async () => {
  const { wrapper, fetcher } = await setup()
  await wrapper.get('.row-field button').trigger('click')
  await wrapper.get('input').setValue('Unsaved')
  await wrapper.get('input').trigger('keydown', { key: 'Escape' })
  expect(wrapper.find('.field-editor').exists()).toBe(false)
  await wrapper.get('dialog').trigger('cancel')
  expect(wrapper.emitted('close')).toHaveLength(1)
  expect(fetcher).toHaveBeenCalledTimes(1)
})

it.each(['table', 'view', 'schema', 'permission', 'identity', 'unmount'])(
  'ignores an existing-row write after %s changes even when fetch ignores abort',
  async (change) => {
    let release!: (response: Response) => void
    const fetcher = vi.fn(async (_url: string, init?: RequestInit) => {
      if (init?.method === 'PATCH')
        return new Promise<Response>((resolve) => {
          release = resolve
        })
      return Response.json(stored)
    })
    const { wrapper } = await setup(tableSchema(), fetcher)
    await wrapper.get('.row-field button').trigger('click')
    await wrapper.get('input').setValue('Late secret')
    await wrapper.get('.field-editor button').trigger('click')
    if (change === 'table')
      await wrapper.setProps({
        schema: {
          ...tableSchema(),
          table: { ...tableSchema().table, id: 'other' },
        },
      })
    else if (change === 'view') await wrapper.setProps({ viewId: 'other' })
    else if (change === 'schema')
      await wrapper.setProps({ schema: { ...tableSchema(), schemaVersion: 2 } })
    else if (change === 'permission')
      await wrapper.setProps({ schema: { ...tableSchema(), myLevel: 'read' } })
    else if (change === 'identity')
      useAuthStore().setUser({ ...navUser, id: 'other' })
    else wrapper.unmount()
    release(Response.json({ row: { id, fields: { name: 'Late secret' } } }))
    await flushPromises()
    expect(wrapper.text()).not.toContain('Late secret')
    expect(wrapper.emitted('saved')).toBeUndefined()
  },
)
