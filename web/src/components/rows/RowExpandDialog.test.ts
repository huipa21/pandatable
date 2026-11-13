import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { i18n, loadLocale } from '../../i18n'
import { useAuthStore } from '../../stores/auth'
import { useNavStore } from '../../stores/nav'
import { useRealtimeStore } from '../../stores/realtime'
import { useUndoStore } from '../../stores/undo'
import { navUser, tableSchema } from '../../test/navigation'
import type { DataRow } from '../../api/types'
import FieldEditor from '../fields/FieldEditor.vue'
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

it.each(['create', 'edit'])(
  'passes schema requiredness to the %s row editor',
  async (mode) => {
    const schema = schemaWith('date')
    schema.fields[0]!.isRequired = true
    stored.fields.name = '2026-10-27'
    const { wrapper } = await setup(
      schema,
      undefined,
      mode === 'create' ? null : id,
    )
    expect(wrapper.findComponent(FieldEditor).props('required')).toBe(true)
  },
)

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
  const button = wrapper.get('.row-field')
  await button.trigger('focusin')
  expect(setFocus).toHaveBeenLastCalledWith('t', { rowId: id, fieldId: 'name' })
  await button.trigger('focusout', { relatedTarget: document.body })
  expect(setFocus).toHaveBeenLastCalledWith('t', null)
})

it('uses the display field as the title and searches every field by name and displayed value', async () => {
  const schema = tableSchema()
  schema.fields.push({
    ...schema.fields[0]!,
    id: 'notes',
    name: 'Notes',
    position: 2048,
  })
  stored.fields.notes = 'Hidden column content'
  const { wrapper } = await setup(schema)
  expect(wrapper.get('header h2').text()).toBe('Original')
  await wrapper.get('.row-search input').setValue('hidden COLUMN')
  expect(
    wrapper
      .findAll('.row-field')
      .filter((field) => field.isVisible())
      .map((field) => field.attributes('data-field-id')),
  ).toEqual(['notes'])
  await wrapper.get('.row-search input').setValue('name')
  expect(
    wrapper.findAll('.row-field').filter((field) => field.isVisible()),
  ).toHaveLength(1)
  await wrapper.get('.row-search input').setValue('missing')
  expect(wrapper.text()).toContain('No matching fields.')
})

it('uses the configured display field even when a checkbox is the first column', async () => {
  const schema = tableSchema()
  schema.fields.unshift({
    ...schema.fields[0]!,
    id: 'status',
    name: 'Status',
    uiType: 'checkbox',
    position: 0,
  })
  stored.fields.status = true
  const { wrapper } = await setup(schema)
  expect(wrapper.get('header h2').text()).toBe('Original')
  await wrapper.setProps({
    schema: { ...schema, table: { ...schema.table, displayFieldId: 'status' } },
  })
  expect(wrapper.get('header h2').text()).toBe('Original')
})

it('falls back to the row identity rather than a boolean title without a text field', async () => {
  stored.fields.name = true
  const { wrapper } = await setup(schemaWith('checkbox'))
  expect(wrapper.get('header h2').text()).toBe(`Row ${id}`)
})

it('keeps an existing field visible while editing a value matched by search', async () => {
  const { wrapper, fetcher } = await setup()
  await wrapper.get('.row-search input').setValue('Original')
  const field = wrapper.get('[data-field-id="name"]')
  const input = field.get('input')
  ;(input.element as HTMLInputElement).focus()
  await input.setValue('Changed draft')
  expect(field.isVisible()).toBe(true)
  expect(document.activeElement).toBe(input.element)
  expect((input.element as HTMLInputElement).value).toBe('Changed draft')
  expect(fetcher).toHaveBeenCalledTimes(1)
  await wrapper.get('.row-search input').setValue('missing')
  expect(field.isVisible()).toBe(false)
  await wrapper.get('.row-search input').setValue('name')
  expect(field.isVisible()).toBe(true)
  expect((input.element as HTMLInputElement).value).toBe('Changed draft')
})

it('keeps focus in the create search when an edited field becomes visible again', async () => {
  const schema = tableSchema()
  schema.fields.push({
    ...schema.fields[0]!,
    id: 'quantity',
    name: 'Quantity',
    position: 2048,
  })
  const { wrapper } = await setup(schema, vi.fn<typeof fetch>(), null)
  await wrapper.get('[data-field-id="name"] input').setValue('Draft')
  const search = wrapper.get('.row-search input')
  ;(search.element as HTMLInputElement).focus()
  await search.setValue('qua')
  await search.setValue('')
  expect(document.activeElement).toBe(search.element)
  expect(
    (wrapper.get('[data-field-id="name"] input').element as HTMLInputElement)
      .value,
  ).toBe('Draft')
})

it('matches create search against field names rather than default values', async () => {
  const schema = tableSchema()
  schema.fields[0]!.default = 'Quality default'
  schema.fields.push({
    ...schema.fields[0]!,
    id: 'quantity',
    name: 'Quantity',
    default: null,
    position: 2048,
  })
  const { wrapper } = await setup(schema, vi.fn<typeof fetch>(), null)
  await wrapper.get('.row-search input').setValue('qua')
  expect(
    wrapper
      .findAll('.row-field')
      .filter((field) => field.isVisible())
      .map((field) => field.attributes('data-field-id')),
  ).toEqual(['quantity'])
})

it('shows a fixed new-entry title, inline editors and no system or navigation controls', async () => {
  const schema = schemaWith('checkbox')
  schema.fields[0]!.default = true
  schema.fields.push(
    {
      ...schema.fields[0]!,
      id: 'quantity',
      name: 'Quantity',
      uiType: 'integer',
      default: 7,
      position: 2048,
    },
    ...['id', 'created_at', 'created_by', 'updated_at', 'updated_by'].map(
      (kind, index) => ({
        ...schema.fields[0]!,
        id: kind,
        name: kind,
        uiType: kind,
        default: null,
        isSystem: true,
        position: 3000 + index,
      }),
    ),
  )
  const { wrapper } = await setup(schema, vi.fn<typeof fetch>(), null)
  expect(wrapper.get('header h2').text()).toBe('New entry')
  expect(
    wrapper
      .get('[data-field-id="name"] [role="checkbox"]')
      .attributes('aria-checked'),
  ).toBe('true')
  expect(wrapper.findAll('.row-field')).toHaveLength(2)
  expect(wrapper.find('[aria-label="Previous row"]').exists()).toBe(false)
  expect(wrapper.find('[aria-label="Next row"]').exists()).toBe(false)
  expect(wrapper.find('[aria-label="Toggle compact view"]').exists()).toBe(
    false,
  )
  expect(wrapper.find('.row-field-value[role="button"]').exists()).toBe(false)
  expect(wrapper.text()).not.toContain('Apply each edited value')
  expect(wrapper.get('[aria-label="Save"]').attributes('title')).toBe('Save')
  await wrapper.get('.row-search input').setValue('qUa')
  expect(
    wrapper
      .findAll('.row-field')
      .filter((field) => field.isVisible())
      .map((field) => field.attributes('data-field-id')),
  ).toEqual(['quantity'])
  await wrapper.get('.row-search input').setValue('missing')
  expect(wrapper.text()).toContain('No matching fields.')
})

it.each(['button', 'Control', 'Meta'])(
  'saves only touched create fields with %s',
  async (method) => {
    const schema = tableSchema()
    schema.fields[0]!.default = 'Default name'
    schema.fields.push({
      ...schema.fields[0]!,
      id: 'quantity',
      name: 'Quantity',
      uiType: 'integer',
      default: 7,
      position: 2048,
    })
    const fetcher = vi.fn<typeof fetch>(async () =>
      Response.json({ rows: [{ id, fields: { name: 'Edited' } }] }),
    )
    const { wrapper } = await setup(schema, fetcher, null)
    const input = wrapper.get('[data-field-id="name"] input')
    expect(input.attributes('placeholder')).toBe('Default name')
    expect((input.element as HTMLInputElement).value).toBe('')
    await input.setValue('Edited')
    await input.trigger('keydown', {
      key: 'Enter',
      ctrlKey: true,
      isComposing: true,
    })
    expect(fetcher).not.toHaveBeenCalled()
    if (method === 'button')
      await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
    else
      await input.trigger('keydown', {
        key: 'Enter',
        ctrlKey: method === 'Control',
        metaKey: method === 'Meta',
      })
    await flushPromises()
    expect(JSON.parse(String(fetcher.mock.calls[0]?.[1]?.body))).toEqual({
      rows: [{ fields: { name: 'Edited' } }],
    })
    expect(wrapper.emitted('created')).toEqual([[id]])
  },
)

it('keeps invalid inline drafts and blocks submission until they are corrected', async () => {
  const fetcher = vi.fn<typeof fetch>(async () =>
    Response.json({ rows: [{ id, fields: { name: 9 } }] }),
  )
  const { wrapper } = await setup(schemaWith('integer'), fetcher, null)
  const input = wrapper.get('.field-editor input')
  await input.setValue('invalid')
  await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
  expect(fetcher).not.toHaveBeenCalled()
  expect(wrapper.get('dialog.row-expand > [role="alert"]').text()).toContain(
    'Correct the invalid field values',
  )
  await wrapper.get('.row-search input').setValue('missing')
  await wrapper.get('.row-search input').setValue('')
  expect((input.element as HTMLInputElement).value).toBe('invalid')
  await input.setValue('9')
  await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
  await flushPromises()
  expect(JSON.parse(String(fetcher.mock.calls[0]?.[1]?.body))).toEqual({
    rows: [{ fields: { name: 9 } }],
  })
})

it.each([
  ['date', '2026-10-27'],
  ['time', '13:45:00'],
  ['datetime', '2026-10-27T07:00:00Z'],
])(
  'keeps an untouched %s default out of the typed draft',
  async (kind, value) => {
    const schema = schemaWith(kind)
    schema.fields[0]!.default = value
    const fetcher = vi.fn<typeof fetch>(async () =>
      Response.json({ rows: [{ id, fields: {} }] }),
    )
    const { wrapper } = await setup(schema, fetcher, null)
    const input = wrapper.get('.field-editor input')
    expect(input.attributes('placeholder')).toBeTruthy()
    expect((input.element as HTMLInputElement).value).toBe('')
    await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
    await flushPromises()
    expect(JSON.parse(String(fetcher.mock.calls[0]?.[1]?.body))).toEqual({
      rows: [{ fields: {} }],
    })
  },
)

it('waits for attachment uploads, commits them from Save, and preserves them after creation', async () => {
  const attachment = {
    id: '0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b',
    name: 'note.txt',
    mime: 'text/plain',
    size: 4,
    thumb: null,
  }
  let release!: (response: Response) => void
  const fetcher = vi.fn<typeof fetch>(async (url, init) => {
    const path = String(url)
    if (path.endsWith('/attachments'))
      return new Promise<Response>((resolve) => {
        release = resolve
      })
    if (path.endsWith('/rows') && init?.method === 'POST')
      return Response.json({ rows: [{ id, fields: { name: [attachment] } }] })
    if (init?.method === 'GET') return Response.json(stored)
    return new Response(null, { status: 204 })
  })
  const { wrapper } = await setup(schemaWith('attachment'), fetcher, null)
  const upload = wrapper.get('.attachment-file-input')
  Object.defineProperty(upload.element, 'files', {
    configurable: true,
    value: [new File(['test'], attachment.name, { type: attachment.mime })],
  })
  await upload.trigger('change')
  await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
  expect(
    fetcher.mock.calls.some(([url]) => String(url).endsWith('/rows')),
  ).toBe(false)
  expect(wrapper.get('[role="alert"]').text()).toContain('Finish or correct')
  release(Response.json(attachment))
  await flushPromises()
  await wrapper.get('.row-search input').setValue('missing')
  await wrapper.get('.row-search input').setValue('')
  expect(wrapper.get('.field-attachment-list').text()).toContain('note.txt')
  await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
  await flushPromises()
  const post = fetcher.mock.calls.find(
    ([url, init]) => String(url).endsWith('/rows') && init?.method === 'POST',
  )
  expect(JSON.parse(String(post?.[1]?.body))).toEqual({
    rows: [{ fields: { name: [attachment] } }],
  })
  await wrapper.setProps({ rowId: id })
  await flushPromises()
  expect(
    fetcher.mock.calls.some(([url]) => String(url).endsWith('/discard')),
  ).toBe(false)
})

it('discards an upload that finishes after the new-entry editor unmounts', async () => {
  const attachment = {
    id: '0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b',
    name: 'late.txt',
    mime: 'text/plain',
    size: 4,
    thumb: null,
  }
  let release!: (response: Response) => void
  const fetcher = vi.fn<typeof fetch>(async (url) => {
    if (String(url).endsWith('/attachments'))
      return new Promise<Response>((resolve) => {
        release = resolve
      })
    if (String(url).endsWith('/discard'))
      return new Response(null, { status: 204 })
    throw new Error(`Unexpected request: ${String(url)}`)
  })
  const mounted = await setup(schemaWith('attachment'), fetcher, null)
  const upload = mounted.wrapper.get('.attachment-file-input')
  Object.defineProperty(upload.element, 'files', {
    configurable: true,
    value: [new File(['test'], attachment.name, { type: attachment.mime })],
  })
  await upload.trigger('change')
  mounted.wrapper.unmount()
  wrapper = undefined
  release(Response.json(attachment))
  await flushPromises()
  const discards = fetcher.mock.calls.filter(([url]) =>
    String(url).endsWith('/discard'),
  )
  expect(discards).toHaveLength(1)
  expect(String(discards[0]?.[0])).toContain(
    `/tables/t/fields/name/attachments/${attachment.id}/discard`,
  )
})

it('discards applied temporary attachments when a new-entry draft is closed', async () => {
  const attachment = {
    id: '0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b',
    name: 'note.txt',
    mime: 'text/plain',
    size: 4,
    thumb: null,
  }
  const fetcher = vi.fn<typeof fetch>(async (url) => {
    if (String(url).endsWith('/attachments')) return Response.json(attachment)
    if (String(url).endsWith('/discard'))
      return new Response(null, { status: 204 })
    throw new Error(`Unexpected request: ${String(url)}`)
  })
  const { wrapper } = await setup(schemaWith('attachment'), fetcher, null)
  const upload = wrapper.get('.attachment-file-input')
  Object.defineProperty(upload.element, 'files', {
    configurable: true,
    value: [new File(['test'], attachment.name, { type: attachment.mime })],
  })
  await upload.trigger('change')
  await flushPromises()
  expect(wrapper.find('.field-editor > button').exists()).toBe(false)
  await wrapper.get('[aria-label="Close row"]').trigger('click')
  await flushPromises()
  expect(
    fetcher.mock.calls.filter(([url]) => String(url).endsWith('/discard')),
  ).toHaveLength(1)
  expect(
    fetcher.mock.calls.some(([url]) => String(url).endsWith('/rows')),
  ).toBe(false)
})

it('does not silently create a row after an attachment upload fails', async () => {
  const fetcher = vi.fn<typeof fetch>(async (url) => {
    if (String(url).endsWith('/attachments'))
      return Response.json(
        { error: { code: 'ERR_VALIDATION' } },
        { status: 400 },
      )
    throw new Error(`Unexpected request: ${String(url)}`)
  })
  const { wrapper } = await setup(schemaWith('attachment'), fetcher, null)
  const upload = wrapper.get('.attachment-file-input')
  Object.defineProperty(upload.element, 'files', {
    configurable: true,
    value: [new File(['test'], 'note.txt', { type: 'text/plain' })],
  })
  await upload.trigger('change')
  await flushPromises()
  await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
  expect(wrapper.get('dialog.row-expand > [role="alert"]').text()).toContain(
    'Finish or correct',
  )
  expect(
    fetcher.mock.calls.some(([url]) => String(url).endsWith('/rows')),
  ).toBe(false)
})

it.each([false, true])(
  'saves pending multi-user selections and supports clearing the last user: %s',
  async (clear) => {
    const user = {
      id: '0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b',
      name: 'Alice',
      username: 'alice',
    }
    const fetcher = vi.fn<typeof fetch>(async (url, init) => {
      const path = String(url)
      if (path.includes('/directory/users?'))
        return Response.json({ items: [user], hasMore: false })
      if (path.endsWith('/directory/user-labels'))
        return Response.json({ items: [user] })
      if (path.endsWith('/rows') && init?.method === 'POST')
        return Response.json({ rows: [{ id, fields: {} }] })
      throw new Error(`Unexpected request: ${path}`)
    })
    const { wrapper } = await setup(schemaWith('multi_user'), fetcher, null)
    await wrapper.get('.field-user-editor input').setValue('ali')
    await flushPromises()
    await wrapper.get('.field-user-results button').trigger('click')
    if (clear) {
      expect(wrapper.find('.field-user-editor > button').exists()).toBe(false)
      await wrapper.get('.field-user-selected button').trigger('click')
    }
    await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
    await flushPromises()
    const post = fetcher.mock.calls.find(
      ([url, init]) => String(url).endsWith('/rows') && init?.method === 'POST',
    )
    expect(JSON.parse(String(post?.[1]?.body))).toEqual({
      rows: [{ fields: { name: clear ? null : [user.id] } }],
    })
  },
)

it('keeps the selected single user visible in the inline draft', async () => {
  const user = {
    id: '0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b',
    name: 'Alice',
    username: 'alice',
  }
  const fetcher = vi.fn<typeof fetch>(async (url, init) => {
    const path = String(url)
    if (path.includes('/directory/users?'))
      return Response.json({ items: [user], hasMore: false })
    if (path.endsWith('/directory/user-labels'))
      return Response.json({ items: [user] })
    if (path.endsWith('/rows') && init?.method === 'POST')
      return Response.json({ rows: [{ id, fields: { name: user.id } }] })
    throw new Error(`Unexpected request: ${path}`)
  })
  const { wrapper } = await setup(schemaWith('user'), fetcher, null)
  await wrapper.get('.field-user-editor input').setValue('ali')
  await flushPromises()
  await wrapper.get('.field-user-results button').trigger('click')
  await flushPromises()
  expect(wrapper.get('.field-user-selected').text()).toContain('Alice')
  await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
  await flushPromises()
  const post = fetcher.mock.calls.find(
    ([url, init]) => String(url).endsWith('/rows') && init?.method === 'POST',
  )
  expect(JSON.parse(String(post?.[1]?.body))).toEqual({
    rows: [{ fields: { name: user.id } }],
  })
})

it('stages an existing inline value on Enter and patches only with header Save', async () => {
  const { wrapper, fetcher } = await setup()
  expect(wrapper.find('.row-field-value[role="button"]').exists()).toBe(false)
  expect(wrapper.find('.row-field button').exists()).toBe(false)
  const input = wrapper.get('.field-editor input')
  await input.setValue('Updated')
  await input.trigger('keydown', { key: 'Enter' })
  expect(fetcher).toHaveBeenCalledTimes(1)
  await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
  await flushPromises()
  expect(JSON.parse(String(fetcher.mock.calls.at(-1)?.[1]?.body))).toEqual({
    fields: { name: 'Updated' },
  })
  expect(wrapper.emitted('saved')).toHaveLength(1)
})

it.each(['button', 'Control', 'Meta'])(
  'submits multiple staged fields atomically with %s and records one undo',
  async (method) => {
    const schema = tableSchema()
    schema.fields.push({
      ...schema.fields[0]!,
      id: 'notes',
      name: 'Notes',
      position: 2048,
    })
    stored.fields.notes = 'Previous notes'
    const { wrapper, fetcher } = await setup(schema)
    const save = wrapper.get('.row-actions [aria-label="Save"]')
    expect(save.attributes('disabled')).toBeDefined()
    expect(wrapper.find('.row-toolbar').exists()).toBe(false)
    expect(wrapper.find('header .row-search').exists()).toBe(true)
    expect(wrapper.findAll('.row-field-value[role="button"]')).toHaveLength(0)
    expect(wrapper.findAll('.field-editor > button')).toHaveLength(0)
    await wrapper.get('[data-field-id="name"] input').setValue('Updated')
    await wrapper.get('[data-field-id="notes"] input').setValue('New notes')
    await wrapper.get('[data-field-id="notes"] input').trigger('focusout', {
      relatedTarget: save.element,
    })
    expect(fetcher).toHaveBeenCalledTimes(1)
    expect(save.attributes('disabled')).toBeUndefined()
    if (method === 'button') await save.trigger('click')
    else
      await wrapper.get('.field-editor input').trigger('keydown', {
        key: 'Enter',
        ctrlKey: method === 'Control',
        metaKey: method === 'Meta',
      })
    await flushPromises()
    const patches = fetcher.mock.calls.filter(
      ([, init]) => init?.method === 'PATCH',
    )
    expect(patches).toHaveLength(1)
    expect(JSON.parse(String(patches[0]?.[1]?.body))).toEqual({
      fields: { name: 'Updated', notes: 'New notes' },
    })
    expect(save.attributes('disabled')).toBeDefined()
    expect(useUndoStore().undoStack).toHaveLength(1)
    expect(useUndoStore().undoStack[0]?.steps).toMatchObject([
      {
        kind: 'patch',
        rows: [{ id, fields: { name: 'Original', notes: 'Previous notes' } }],
      },
    ])
  },
)

it('guards row actions only while dirty and disables Save after reverting', async () => {
  const { wrapper, fetcher } = await setup()
  await wrapper.setProps({ navigateRow: vi.fn().mockResolvedValue('2') })
  const save = wrapper.get('.row-actions [aria-label="Save"]')
  const input = wrapper.get('.field-editor input')
  expect(save.attributes('disabled')).toBeDefined()
  await input.setValue('Changed')
  for (const label of [
    'Previous row',
    'Next row',
    'Duplicate row',
    'Delete row',
  ])
    expect(
      wrapper
        .get(`.row-actions [aria-label="${label}"]`)
        .attributes('disabled'),
      label,
    ).toBeDefined()
  await input.setValue('Original')
  expect(save.attributes('disabled')).toBeDefined()
  for (const label of [
    'Previous row',
    'Next row',
    'Duplicate row',
    'Delete row',
  ])
    expect(
      wrapper
        .get(`.row-actions [aria-label="${label}"]`)
        .attributes('disabled'),
      label,
    ).toBeUndefined()
  expect(fetcher).toHaveBeenCalledTimes(1)
})

it('blocks invalid existing drafts and preserves corrected values for a failed Save retry', async () => {
  stored.fields.name = 1
  const fetcher = vi.fn(async (_url: string, init?: RequestInit) => {
    if (init?.method !== 'PATCH') return Response.json(stored)
    return Response.json({ row: { id, fields: { name: 7 } } })
  })
  const { wrapper } = await setup(schemaWith('integer'), fetcher)
  const save = wrapper.get('.row-actions [aria-label="Save"]')
  const input = wrapper.get('.field-editor input')
  await input.setValue('invalid')
  expect(save.attributes('disabled')).toBeUndefined()
  await save.trigger('click')
  expect(fetcher).toHaveBeenCalledTimes(1)
  expect(wrapper.get('dialog.row-expand > [role="alert"]').text()).toContain(
    'invalid',
  )
  await input.setValue('7')
  await input.trigger('focusout', { relatedTarget: save.element })
  expect(fetcher).toHaveBeenCalledTimes(1)
  fetcher.mockResolvedValueOnce(
    Response.json({ error: { code: 'ERR_INTERNAL' } }, { status: 500 }),
  )
  await save.trigger('click')
  await flushPromises()
  expect((input.element as HTMLInputElement).value).toBe('7')
  expect(save.attributes('disabled')).toBeUndefined()
  expect(wrapper.emitted('saved')).toBeUndefined()
  await save.trigger('click')
  await flushPromises()
  expect(wrapper.emitted('saved')).toHaveLength(1)
  expect(
    fetcher.mock.calls.filter(([, init]) => init?.method === 'PATCH'),
  ).toHaveLength(2)
})

it('blocks clearing a required inline field before making a request', async () => {
  const schema = tableSchema()
  schema.fields[0]!.isRequired = true
  const { wrapper, fetcher } = await setup(schema)
  await wrapper.get('.field-editor input').setValue('')
  await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
  expect(fetcher).toHaveBeenCalledTimes(1)
  expect(wrapper.get('.field-editor input').attributes('aria-invalid')).toBe(
    'true',
  )
  expect(wrapper.get('.field-editor [role="alert"]').text()).toContain(
    'required',
  )
})

it('detects an unconfirmed existing datetime selection and preserves it from realtime reloads', async () => {
  const schema = schemaWith('datetime', { timezone: 'fixed', fixedTz: 'UTC' })
  stored.fields.name = '2026-03-01T14:15:12.123456Z'
  const subscribe = vi.spyOn(useRealtimeStore(), 'subscribe')
  const { wrapper, fetcher } = await setup(schema)
  const save = wrapper.get('.row-actions [aria-label="Save"]')
  expect(save.attributes('disabled')).toBeDefined()
  await wrapper.get('.temporal-editor-toggle').trigger('click')
  await flushPromises()
  const portal = wrapper.get('.temporal-picker-portal')
  const day = portal
    .findAll('.n-date-panel-date:not(.n-date-panel-date--excluded)')
    .find((element) => element.text() === '15')
  if (!day) throw new Error('Datetime day was not found')
  await day.trigger('pointerdown')
  await day.trigger('click')
  expect(save.attributes('disabled')).toBeUndefined()
  subscribe.mock.calls[0]?.[1]({ t: 'row.updated', tableId: 't', ids: [id] })
  await flushPromises()
  expect(fetcher).toHaveBeenCalledTimes(1)
  await save.trigger('click')
  await flushPromises()
  const patches = fetcher.mock.calls.filter(
    ([, init]) => init?.method === 'PATCH',
  )
  expect(patches).toHaveLength(1)
  expect(JSON.parse(String(patches[0]?.[1]?.body))).toEqual({
    fields: { name: '2026-03-15T14:15:12.123456Z' },
  })
})

it('does not replace a draft with a realtime read that was already in flight', async () => {
  const subscribe = vi.spyOn(useRealtimeStore(), 'subscribe')
  const { wrapper, fetcher } = await setup()
  let release!: (response: Response) => void
  fetcher.mockImplementationOnce(
    () =>
      new Promise<Response>((resolve) => {
        release = resolve
      }),
  )
  subscribe.mock.calls[0]?.[1]({ t: 'row.updated', tableId: 't', ids: [id] })
  await wrapper.get('.field-editor input').setValue('Pending local value')
  release(Response.json({ id, fields: { name: 'Remote value' } }))
  await flushPromises()
  expect(
    (wrapper.get('.field-editor input').element as HTMLInputElement).value,
  ).toBe('Pending local value')
  expect(
    wrapper.get('.row-actions [aria-label="Save"]').attributes('disabled'),
  ).toBeUndefined()
})

it('opens the relation picker by clicking the linked value', async () => {
  const schema = tableSchema()
  schema.fields.push({
    ...schema.fields[0]!,
    id: 'linked',
    name: 'Linked',
    uiType: 'link',
    position: 2048,
  })
  stored.fields.linked = [{ id: '2', label: 'Related row' }]
  const { wrapper } = await setup(schema)
  const section = wrapper.get('[data-field-id="linked"]')
  expect(section.get('.relation-edit').attributes('aria-label')).toBe(
    'Edit links',
  )
  await section.get('.relation-chip').trigger('click')
  expect(wrapper.get('.relation-picker').attributes('open')).toBeDefined()
})

function relationSchema() {
  const schema = tableSchema()
  schema.fields.push({
    ...schema.fields[0]!,
    id: 'linked',
    name: 'Linked',
    uiType: 'link',
    options: {
      relationId: 'relation',
      targetTableId: 'target',
      side: 'parent',
    },
    position: 2048,
  })
  stored.fields.linked = [
    { id: '2', label: 'Related row' },
    { id: '3', label: 'Another row' },
  ]
  schema.relations = [
    {
      id: 'relation',
      kind: 'many_many',
      parentTableId: 't',
      childTableId: 'target',
      parentFieldId: 'linked',
      childFieldId: 'reverse',
      onDelete: 'set_null',
    },
  ]
  return schema
}

it('stages picker replacements with ordinary edits in one PATCH and preserves displaced-link undo', async () => {
  const schema = relationSchema()
  schema.relations[0]!.kind = 'one_one'
  stored.fields.linked = [{ id: '2', label: 'Related row' }]
  const previous = structuredClone(stored.fields)
  const changes = [
    {
      tableId: 't',
      rowId: id,
      fieldId: 'linked',
      targetId: '2',
      action: 'unlink',
    },
    {
      tableId: 't',
      rowId: '8',
      fieldId: 'linked',
      targetId: '4',
      action: 'unlink',
    },
    {
      tableId: 't',
      rowId: id,
      fieldId: 'linked',
      targetId: '4',
      action: 'link',
    },
  ]
  const fetcher = vi.fn(async (url: string, init?: RequestInit) => {
    if (url.endsWith('/tables/target/schema'))
      return Response.json(tableSchema())
    if (url.endsWith('/query'))
      return Response.json({
        rows: [{ id: '4', fields: { name: 'Target' } }],
        total: 1,
      })
    if (init?.method === 'PATCH')
      return Response.json({
        row: {
          id,
          fields: {
            name: 'Edited',
            linked: [{ id: '4', label: 'Authoritative target' }],
          },
        },
        previous,
        changes,
      })
    return Response.json(stored)
  })
  const { wrapper } = await setup(schema, fetcher)
  await wrapper.get('.field-editor input').setValue('Edited')
  await wrapper.get('[data-field-id="linked"] .relation-edit').trigger('click')
  await flushPromises()
  await wrapper
    .get('.relation-picker')
    .get('[aria-label="Link “Target”"]')
    .trigger('click')
  expect(wrapper.get('[data-field-id="linked"]').text()).toContain('Target')
  expect(wrapper.find('[aria-label="Unlink “Related row”"]').exists()).toBe(
    false,
  )
  expect(
    fetcher.mock.calls.filter(([url]) => url.includes('/links/')),
  ).toHaveLength(0)
  expect(
    fetcher.mock.calls.filter(([, init]) => init?.method === 'PATCH'),
  ).toHaveLength(0)
  expect(useUndoStore().undoStack).toHaveLength(0)
  await wrapper
    .get('.relation-picker')
    .get('.relation-footer > button')
    .trigger('click')
  await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
  await flushPromises()
  const patches = fetcher.mock.calls.filter(
    ([, init]) => init?.method === 'PATCH',
  )
  expect(patches).toHaveLength(1)
  expect(JSON.parse(String(patches[0]?.[1]?.body))).toEqual({
    fields: { name: 'Edited', linked: [{ id: '4', label: 'Target' }] },
  })
  expect(wrapper.get('[data-field-id="linked"]').text()).toContain(
    'Authoritative target',
  )
  expect(useUndoStore().undoStack).toHaveLength(1)
  expect(useUndoStore().undoStack[0]?.steps).toMatchObject([
    { kind: 'patch', rows: [{ id, fields: { name: 'Original' } }] },
    { kind: 'link', rowId: id, add: [], remove: ['4'] },
    { kind: 'link', rowId: '8', add: ['4'], remove: [] },
    { kind: 'link', rowId: id, add: ['2'], remove: [] },
  ])
})

it('stages only the chosen relation tag and records undo after header Save', async () => {
  const schema = relationSchema()
  const fetcher = vi.fn(async (_url: string, init?: RequestInit) => {
    if (init?.method !== 'PATCH') return Response.json(stored)
    stored = {
      ...stored,
      fields: { ...stored.fields, linked: [{ id: '3', label: 'Another row' }] },
    }
    return Response.json({
      row: stored,
      eventId: 'event',
      changes: [
        {
          tableId: 't',
          rowId: id,
          fieldId: 'linked',
          targetId: '2',
          action: 'unlink',
        },
      ],
    })
  })
  const { wrapper } = await setup(schema, fetcher)
  await wrapper
    .get('button[aria-label="Unlink “Related row”"]')
    .trigger('click')
  await flushPromises()
  expect(fetcher).toHaveBeenCalledTimes(1)
  expect(wrapper.emitted('saved')).toBeUndefined()
  expect(useUndoStore().undoStack).toHaveLength(0)
  await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
  await flushPromises()

  const mutation = fetcher.mock.calls.find(
    ([, init]) => init?.method === 'PATCH',
  )
  expect(mutation?.[0]).toBe(`/api/v1/tables/t/rows/${id}`)
  expect(JSON.parse(String(mutation?.[1]?.body))).toEqual({
    fields: { linked: [{ id: '3', label: 'Another row' }] },
  })
  expect(wrapper.find('[aria-label="Unlink “Related row”"]').exists()).toBe(
    false,
  )
  expect(wrapper.find('[aria-label="Unlink “Another row”"]').exists()).toBe(
    true,
  )
  expect(wrapper.get('.relation-picker').attributes('open')).toBeUndefined()
  expect(wrapper.emitted('saved')).toHaveLength(1)
  expect(useUndoStore().undoStack[0]?.steps).toEqual([
    {
      kind: 'link',
      tableId: 't',
      rowId: id,
      fieldId: 'linked',
      add: ['2'],
      remove: [],
    },
  ])
})

it('keeps the staged relation removal and reports a Save failure', async () => {
  const schema = relationSchema()
  const fetcher = vi.fn(async (_url: string, init?: RequestInit) =>
    init?.method === 'PATCH'
      ? Response.json({ error: { code: 'ERR_INTERNAL' } }, { status: 500 })
      : Response.json(stored),
  )
  const { wrapper } = await setup(schema, fetcher)
  await wrapper
    .get('button[aria-label="Unlink “Related row”"]')
    .trigger('click')
  await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
  await flushPromises()

  expect(wrapper.get('[role="alert"]').text()).toBe(
    i18n.global.t('errors.ERR_INTERNAL'),
  )
  expect(wrapper.find('[aria-label="Unlink “Related row”"]').exists()).toBe(
    false,
  )
  expect(wrapper.emitted('saved')).toBeUndefined()
  expect(useUndoStore().undoStack).toHaveLength(0)
})

it('disables unlinking while saving and ignores a late result after permission is revoked', async () => {
  const schema = relationSchema()
  let release!: (response: Response) => void
  const fetcher = vi.fn(async (_url: string, init?: RequestInit) =>
    init?.method === 'PATCH'
      ? new Promise<Response>((resolve) => {
          release = resolve
        })
      : Response.json(stored),
  )
  const { wrapper } = await setup(schema, fetcher)
  await wrapper
    .get('button[aria-label="Unlink “Related row”"]')
    .trigger('click')
  await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
  expect(
    wrapper
      .get('button[aria-label="Unlink “Another row”"]')
      .attributes('disabled'),
  ).toBeDefined()
  expect(
    wrapper.get('button[aria-label="Edit links"]').attributes('disabled'),
  ).toBeDefined()

  await wrapper.setProps({ schema: { ...schema, myLevel: 'read' } })
  release(
    Response.json({
      row: { ...stored, fields: { ...stored.fields, linked: [] } },
      changes: [
        {
          tableId: 't',
          rowId: id,
          fieldId: 'linked',
          targetId: '2',
          action: 'unlink',
        },
      ],
    }),
  )
  await flushPromises()

  expect(wrapper.find('.relation-remove').exists()).toBe(false)
  expect(wrapper.find('.relation-edit').exists()).toBe(false)
  expect(wrapper.get('.relation-chip').text()).toBe('Related row')
  expect(wrapper.get('[role="alert"]').text()).toContain(
    'You do not have permission',
  )
  expect(wrapper.emitted('saved')).toBeUndefined()
  expect(useUndoStore().undoStack).toHaveLength(0)
})

it('keeps fields visible alongside activity tabs and preserves the active draft', async () => {
  const { wrapper } = await setup()
  await wrapper.get('.field-editor input').setValue('Unsaved')
  await wrapper.get('[id^="row-comments-tab"]').trigger('click')
  expect(wrapper.get('.row-form').isVisible()).toBe(true)
  expect(wrapper.get('[id^="row-comments-panel"]').isVisible()).toBe(true)
  await wrapper.get('[aria-label="Collapse activity"]').trigger('click')
  expect(wrapper.get('.row-activity').isVisible()).toBe(false)
  expect(wrapper.get('.row-expand').classes()).toContain(
    'row-expand-no-activity',
  )
  expect(wrapper.get('.row-form').isVisible()).toBe(true)
  expect(
    (wrapper.get('.field-editor input').element as HTMLInputElement).value,
  ).toBe('Unsaved')
  await wrapper.get('[aria-label="Expand activity"]').trigger('click')
  await wrapper.get('[id^="row-comments-tab"]').trigger('click')
  expect(wrapper.get('.row-form').isVisible()).toBe(true)
  expect(wrapper.get('.row-activity').isVisible()).toBe(true)
  expect(wrapper.get('.row-expand').classes()).not.toContain(
    'row-expand-no-activity',
  )
  expect(wrapper.emitted('close')).toBeUndefined()
})

it('opens notification deep links directly in the comments overlay', async () => {
  const { wrapper } = await setup()
  await wrapper.get('[aria-label="Collapse activity"]').trigger('click')
  await wrapper.setProps({ focusComment: 'comment-1' })
  expect(
    wrapper.get('[id^="row-comments-tab"]').attributes('aria-selected'),
  ).toBe('true')
  expect(wrapper.get('.row-form').isVisible()).toBe(true)
  expect(wrapper.get('.row-activity').isVisible()).toBe(true)
})

it('omits compact presentation and toggles fullscreen only inside the table workspace', async () => {
  const { wrapper } = await setup()
  const workspace = document.createElement('div')
  vi.spyOn(workspace, 'getBoundingClientRect').mockReturnValue(
    new DOMRect(220, 120, 900, 500),
  )
  await wrapper.setProps({ workspace })
  expect(wrapper.find('[aria-label="Toggle compact view"]').exists()).toBe(
    false,
  )
  const fullscreenButton = wrapper.get('[aria-label="Toggle table fullscreen"]')
  const expandPath = fullscreenButton.get('svg path').attributes('d')
  await wrapper.get('[aria-label="Toggle table fullscreen"]').trigger('click')
  expect(fullscreenButton.get('svg path').attributes('d')).not.toBe(expandPath)
  expect(fullscreenButton.attributes('aria-pressed')).toBe('true')
  const dialog = wrapper.get('dialog.row-expand').element as HTMLDialogElement
  expect([
    dialog.style.left,
    dialog.style.top,
    dialog.style.width,
    dialog.style.height,
  ]).toEqual(['220px', '120px', '900px', '500px'])
  await wrapper.get('[aria-label="Toggle table fullscreen"]').trigger('click')
  expect(dialog.style.width).toBe('')
  expect(fullscreenButton.get('svg path').attributes('d')).toBe(expandPath)
  expect(fullscreenButton.attributes('aria-pressed')).toBe('false')
})

it('keeps duplication entirely local until Save duplicate and generates a new identity', async () => {
  const schema = tableSchema()
  schema.fields.push({
    ...schema.fields[0]!,
    id: 'id',
    name: 'ID',
    uiType: 'id',
    isSystem: true,
    position: 3,
  })
  stored.fields.id = id
  const { wrapper, fetcher } = await setup(schema)
  fetcher.mockClear()
  await wrapper.get('[aria-label="Duplicate row"]').trigger('click')
  expect(fetcher).not.toHaveBeenCalled()
  expect(wrapper.text()).toContain('Unsaved duplicate')
  await wrapper.get('.field-editor input').setValue('Copied name')
  expect(fetcher).not.toHaveBeenCalled()
  fetcher.mockResolvedValueOnce(
    Response.json({ rows: [{ id: '2', fields: { name: 'Copied name' } }] }),
  )
  await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
  await flushPromises()
  expect(JSON.parse(String(fetcher.mock.calls[0]?.[1]?.body))).toEqual({
    rows: [{ fields: { name: 'Copied name' } }],
  })
  expect(wrapper.emitted('created')).toEqual([['2']])
})

it('closing a duplicate never inserts a row', async () => {
  const { wrapper, fetcher } = await setup()
  fetcher.mockClear()
  await wrapper.get('[aria-label="Duplicate row"]').trigger('click')
  await wrapper.get('[aria-label="Close row"]').trigger('click')
  expect(fetcher).not.toHaveBeenCalled()
  expect(wrapper.emitted('close')).toHaveLength(1)
})

it.each([
  ['datetime', 'button', false],
  ['datetime', 'button', true],
  ['datetime', 'shortcut', false],
  ['time', 'button', false],
  ['time', 'shortcut', false],
] as const)(
  'saves an unconfirmed %s picker selection using the %s (outside month: %s)',
  async (kind, method, outsideMonth) => {
    const schema = schemaWith(
      kind,
      kind === 'datetime'
        ? { timezone: 'fixed', fixedTz: 'America/New_York' }
        : {},
    )
    stored.fields.name =
      kind === 'datetime' ? '2026-03-01T14:15:12.123456Z' : '09:15:12.123456'
    const fetcher = vi.fn<typeof fetch>(async (_url, init) => {
      if (init?.method === 'GET') return Response.json(stored)
      return Response.json({ rows: [{ id: '2', fields: {} }] })
    })
    const { wrapper } = await setup(schema, fetcher)
    await wrapper.get('[aria-label="Duplicate row"]').trigger('click')
    const input = wrapper.get('.temporal-editor input')
    ;(input.element as HTMLInputElement).focus()
    await wrapper.get('.temporal-editor-toggle').trigger('click')
    await flushPromises()
    const portal = wrapper
      .get('dialog.row-expand')
      .element.querySelector<HTMLElement>('.temporal-picker-portal')
    if (!portal) throw new Error('Temporal picker portal was not mounted')
    const selector =
      kind === 'datetime'
        ? outsideMonth
          ? '.n-date-panel-date--excluded:not(.n-date-panel-date--disabled)'
          : '.n-date-panel-date:not(.n-date-panel-date--excluded)'
        : '.time-scroller-column:nth-child(2) .time-scroller-option'
    const selected = Array.from(
      portal.querySelectorAll<HTMLElement>(selector),
    ).find(
      (element) =>
        element.textContent?.trim() ===
        (kind === 'time' ? '35' : outsideMonth ? '1' : '15'),
    )
    if (!selected) throw new Error(`${kind} picker option was not found`)
    selected.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    selected.click()
    await flushPromises()

    if (method === 'button')
      await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
    else
      portal.dispatchEvent(
        new KeyboardEvent('keydown', {
          key: 'Enter',
          ctrlKey: true,
          bubbles: true,
        }),
      )
    await flushPromises()
    const post = fetcher.mock.calls.find(([, init]) => init?.method === 'POST')
    expect(JSON.parse(String(post?.[1]?.body))).toEqual({
      rows: [
        {
          fields: {
            name:
              kind === 'datetime'
                ? outsideMonth
                  ? '2026-04-01T13:15:12.123456Z'
                  : '2026-03-15T13:15:12.123456Z'
                : '09:35:12.123456',
          },
        },
      ],
    })
  },
)

it.each([
  ['date', '2026-03-01'],
  ['datetime', '2026-03-01T14:15:12Z'],
  ['time', '09:15:12'],
] as const)(
  'keeps the %s default untouched when its picker is opened without a selection',
  async (kind, defaultValue) => {
    const schema = schemaWith(kind)
    schema.fields[0]!.default = defaultValue
    const fetcher = vi.fn<typeof fetch>(async () =>
      Response.json({ rows: [{ id: '2', fields: {} }] }),
    )
    const { wrapper } = await setup(schema, fetcher, null)
    const input = wrapper.get('.temporal-editor input')
    ;(input.element as HTMLInputElement).focus()
    await wrapper.get('.temporal-editor-toggle').trigger('click')
    await flushPromises()
    const save = wrapper.get('.row-actions [aria-label="Save"]')
    await save.trigger('pointerdown')
    await save.trigger('click')
    await flushPromises()
    expect(fetcher).toHaveBeenCalledTimes(1)
    expect(JSON.parse(String(fetcher.mock.calls[0]?.[1]?.body))).toEqual({
      rows: [{ fields: {} }],
    })
  },
)

it('saves a selected datetime instead of omitting an untouched database default', async () => {
  const schema = schemaWith('datetime', { timezone: 'fixed', fixedTz: 'UTC' })
  schema.fields[0]!.default = '2026-03-01T14:15:12Z'
  const fetcher = vi.fn<typeof fetch>(async () =>
    Response.json({ rows: [{ id: '2', fields: {} }] }),
  )
  const { wrapper } = await setup(schema, fetcher, null)
  const input = wrapper.get('.temporal-editor input')
  ;(input.element as HTMLInputElement).focus()
  await wrapper.get('.temporal-editor-toggle').trigger('click')
  await flushPromises()
  const portal = wrapper
    .get('dialog.row-expand')
    .element.querySelector<HTMLElement>('.temporal-picker-portal')
  const day = Array.from(
    portal?.querySelectorAll<HTMLElement>(
      '.n-date-panel-date:not(.n-date-panel-date--excluded)',
    ) ?? [],
  ).find((element) => element.textContent?.trim() === '15')
  if (!day) throw new Error('Datetime picker day was not found')
  day.dispatchEvent(new Event('pointerdown', { bubbles: true }))
  day.click()
  await flushPromises()
  const save = wrapper.get('.row-actions [aria-label="Save"]')
  await save.trigger('pointerdown')
  await save.trigger('click')
  await flushPromises()
  const body = JSON.parse(String(fetcher.mock.calls[0]?.[1]?.body)) as {
    rows: { fields: { name?: string } }[]
  }
  expect(body.rows[0]?.fields.name).toMatch(
    /^\d{4}-\d{2}-15T\d{2}:\d{2}:\d{2}(?:\.\d{1,6})?Z$/,
  )
})

it.each(['keyboard', 'pointer'])(
  'retains a pending datetime selection when another field receives %s focus',
  async (method) => {
    const schema = schemaWith('datetime', {
      timezone: 'fixed',
      fixedTz: 'America/New_York',
    })
    schema.fields.push({
      ...tableSchema().fields[0]!,
      id: 'notes',
      name: 'Notes',
      position: 2048,
    })
    stored.fields.name = '2026-03-01T14:15:12.123456Z'
    stored.fields.notes = null
    const fetcher = vi.fn<typeof fetch>(async (_url, init) => {
      if (init?.method === 'GET') return Response.json(stored)
      return Response.json({ rows: [{ id: '2', fields: {} }] })
    })
    const { wrapper } = await setup(schema, fetcher)
    await wrapper.get('[aria-label="Duplicate row"]').trigger('click')
    const input = wrapper.get('[data-field-id="name"] .temporal-editor input')
    ;(input.element as HTMLInputElement).focus()
    await wrapper.get('.temporal-editor-toggle').trigger('click')
    await flushPromises()
    const portal = wrapper
      .get('dialog.row-expand')
      .element.querySelector<HTMLElement>('.temporal-picker-portal')
    const day = Array.from(
      portal?.querySelectorAll<HTMLElement>(
        '.n-date-panel-date:not(.n-date-panel-date--excluded)',
      ) ?? [],
    ).find((element) => element.textContent?.trim() === '15')
    if (!day) throw new Error('Datetime picker day was not found')
    day.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    day.click()
    await flushPromises()
    const notes = wrapper.get('[data-field-id="notes"] input')
    if (method === 'pointer') await notes.trigger('pointerdown')
    ;(notes.element as HTMLInputElement).focus()
    await notes.trigger('click')
    await flushPromises()
    const save = wrapper.get('.row-actions [aria-label="Save"]')
    await save.trigger('pointerdown')
    await save.trigger('click')
    await flushPromises()
    const post = fetcher.mock.calls.find(([, init]) => init?.method === 'POST')
    const body = JSON.parse(String(post?.[1]?.body)) as {
      rows: { fields: { name?: string } }[]
    }
    expect(body.rows[0]?.fields.name).toBe('2026-03-15T13:15:12.123456Z')
  },
)

it('blocks creation for invalid pending picker input and saves it after correction', async () => {
  const schema = schemaWith('datetime', {
    timezone: 'fixed',
    fixedTz: 'America/New_York',
  })
  stored.fields.name = '2026-03-01T14:15:12.123456Z'
  const fetcher = vi.fn<typeof fetch>(async (_url, init) => {
    if (init?.method === 'GET') return Response.json(stored)
    return Response.json({ rows: [{ id: '2', fields: {} }] })
  })
  const { wrapper } = await setup(schema, fetcher)
  await wrapper.get('[aria-label="Duplicate row"]').trigger('click')
  const input = wrapper.get('.temporal-editor input')
  ;(input.element as HTMLInputElement).focus()
  await wrapper.get('.temporal-editor-toggle').trigger('click')
  await flushPromises()
  const portal = wrapper
    .get('dialog.row-expand')
    .element.querySelector<HTMLElement>('.temporal-picker-portal')
  const day = Array.from(
    portal?.querySelectorAll<HTMLElement>(
      '.n-date-panel-date:not(.n-date-panel-date--excluded)',
    ) ?? [],
  ).find((element) => element.textContent?.trim() === '15')
  const time = portal?.querySelector<HTMLInputElement>(
    '.n-date-panel-header .n-time-picker input',
  )
  if (!day || !time) throw new Error('Datetime picker controls were not found')
  day.dispatchEvent(new Event('pointerdown', { bubbles: true }))
  day.click()
  await flushPromises()
  time.value = '25:00'
  time.dispatchEvent(new Event('input', { bubbles: true }))
  await flushPromises()
  const save = wrapper.get('.row-actions [aria-label="Save"]')
  await save.trigger('click')
  expect(fetcher.mock.calls.some(([, init]) => init?.method === 'POST')).toBe(
    false,
  )
  expect(wrapper.get('dialog.row-expand > [role="alert"]').text()).not.toBe('')
  ;(input.element as HTMLInputElement).focus()
  if (
    wrapper.get('.temporal-editor-toggle').attributes('aria-expanded') !==
    'true'
  )
    await wrapper.get('.temporal-editor-toggle').trigger('click')
  await flushPromises()
  const correctedDay = Array.from(
    portal?.querySelectorAll<HTMLElement>(
      '.n-date-panel-date:not(.n-date-panel-date--excluded)',
    ) ?? [],
  ).find((element) => element.textContent?.trim() === '15')
  const correctedTime = portal?.querySelector<HTMLInputElement>(
    '.n-date-panel-header .n-time-picker input',
  )
  if (!correctedDay || !correctedTime)
    throw new Error('Datetime picker controls were not reopened')
  correctedDay.dispatchEvent(new Event('pointerdown', { bubbles: true }))
  correctedDay.click()
  await flushPromises()
  correctedTime.value = '09:35'
  correctedTime.dispatchEvent(new Event('input', { bubbles: true }))
  await flushPromises()
  await save.trigger('click')
  await flushPromises()
  const post = fetcher.mock.calls.find(([, init]) => init?.method === 'POST')
  const body = JSON.parse(String(post?.[1]?.body)) as {
    rows: { fields: { name?: string } }[]
  }
  expect(body.rows[0]?.fields.name).toBe('2026-03-15T13:35:12.123456Z')
})

it('deletes only after confirmation and surfaces deletion failure without closing the row', async () => {
  const { wrapper, fetcher } = await setup()
  fetcher.mockClear()
  await wrapper.get('.row-actions [aria-label="Delete row"]').trigger('click')
  expect(fetcher).not.toHaveBeenCalled()
  expect(wrapper.get('.row-delete-dialog').attributes('open')).toBeDefined()
  fetcher.mockResolvedValueOnce(
    Response.json(
      { error: { code: 'ERR_RESTRICT_VIOLATION' } },
      { status: 409 },
    ),
  )
  await wrapper.get('.row-delete-dialog button').trigger('click')
  await flushPromises()
  expect(wrapper.get('[role="alert"]').text()).not.toBe('')
  expect(wrapper.emitted('close')).toBeUndefined()
  fetcher.mockResolvedValueOnce(Response.json({ rows: [stored] }))
  await wrapper.get('.row-actions [aria-label="Delete row"]').trigger('click')
  await wrapper.get('.row-delete-dialog button').trigger('click')
  await flushPromises()
  expect(fetcher.mock.calls.at(-1)?.[1]?.method).toBe('DELETE')
  expect(JSON.parse(String(fetcher.mock.calls.at(-1)?.[1]?.body))).toEqual({
    ids: [id],
  })
  expect(wrapper.emitted('saved')).toHaveLength(1)
  expect(wrapper.emitted('close')).toHaveLength(1)
})

it('navigates by row identity, reports boundaries and disables navigation during edits', async () => {
  const { wrapper } = await setup()
  const navigateRow = vi
    .fn()
    .mockResolvedValueOnce('2')
    .mockResolvedValueOnce(null)
  await wrapper.setProps({ navigateRow })
  await wrapper.get('[aria-label="Next row"]').trigger('click')
  await flushPromises()
  expect(navigateRow).toHaveBeenCalledWith(id, 1)
  expect(wrapper.emitted('navigate')).toEqual([['2']])
  await wrapper.get('[aria-label="Previous row"]').trigger('click')
  await flushPromises()
  expect(wrapper.text()).toContain('This is the first row.')
  await wrapper.get('.field-editor input').setValue('Draft')
  expect(
    wrapper.get('[aria-label="Next row"]').attributes('disabled'),
  ).toBeDefined()
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
  expect(tabs.map((tab) => tab.text())).toEqual(['Comments', 'History'])
  expect(tabs[0]!.attributes('aria-selected')).toBe('true')
  await wrapper
    .get('[role="tablist"]')
    .trigger('keydown', { key: 'ArrowRight' })
  expect(tabs[1]!.attributes('aria-selected')).toBe('true')
  await flushPromises()
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
  await wrapper.get('.field-editor input').setValue('Original')
  await flushPromises()
  expect(wrapper.text()).toContain('Bob saved')

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
  await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
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
    await wrapper
      .get('.field-editor input, .field-editor textarea')
      .setValue(input)
    await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
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
    await wrapper
      .get('.field-editor input, .field-editor textarea')
      .setValue(input)
    expect(fetcher).not.toHaveBeenCalled()
    await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
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
      await section
        .get('textarea')
        .setValue('x'.repeat(index < 5 ? 100000 : bytes - overhead - 500000))
    }
    await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
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
  const input = wrapper.get('[data-field-id="name"] input')
  expect(input.attributes('placeholder')).toBe('Default name')
  await input.setValue('Changed')
  await input.setValue('')
  await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
  await flushPromises()
  expect(JSON.parse(String(fetcher.mock.calls[0]?.[1]?.body))).toEqual({
    rows: [{ fields: { name: null } }],
  })
  expect(wrapper.emitted('created')).toBeUndefined()
  expect(wrapper.get('[role="alert"]').text()).toContain('required field')
  expect(wrapper.text()).toContain('Creation has not been confirmed')
  expect((input.element as HTMLInputElement).value).toBe('')
})

it('prevents duplicate creation and retains editable drafts after unique and network errors', async () => {
  let release!: (response: Response) => void
  const fetcher = vi
    .fn<typeof fetch>()
    .mockImplementationOnce(
      () =>
        new Promise<Response>((resolve) => {
          release = resolve
        }),
    )
    .mockRejectedValueOnce(new TypeError('Network unavailable'))
    .mockResolvedValueOnce(
      Response.json({ rows: [{ id, fields: { name: 'Draft' } }] }),
    )
  const { wrapper } = await setup(tableSchema(), fetcher, null)
  const field = wrapper.get('.row-field')
  await field.get('input').setValue('Draft')
  await field.get('input').trigger('blur')
  expect(fetcher).not.toHaveBeenCalled()
  const create = wrapper.get('.row-actions [aria-label="Save"]')
  await create.trigger('click')
  create.element.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  expect(fetcher).toHaveBeenCalledTimes(1)
  expect(create.attributes('disabled')).toBeDefined()
  release(
    Response.json({ error: { code: 'ERR_UNIQUE_CONFLICT' } }, { status: 409 }),
  )
  await flushPromises()
  expect(wrapper.get('[role="alert"]').text()).toContain('unique field')
  expect(wrapper.emitted('created')).toBeUndefined()
  expect((field.get('input').element as HTMLInputElement).value).toBe('Draft')
  await create.trigger('click')
  await flushPromises()
  expect(wrapper.get('[role="alert"]').text()).not.toBe('')
  expect(wrapper.emitted('created')).toBeUndefined()
  expect((field.get('input').element as HTMLInputElement).value).toBe('Draft')
  await create.trigger('click')
  await flushPromises()
  expect(fetcher).toHaveBeenCalledTimes(3)
  expect(wrapper.emitted('created')).toEqual([[id]])
  for (const [, init] of fetcher.mock.calls)
    expect(JSON.parse(String(init?.body))).toEqual({
      rows: [{ fields: { name: 'Draft' } }],
    })
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
    await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
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

it.each(['table', 'view', 'schema', 'permission', 'identity'])(
  'clears create editors and invalid drafts after %s changes',
  async (change) => {
    const schema = schemaWith('integer', { size: 'big' })
    const fetcher = vi.fn<typeof fetch>(async () =>
      Response.json({ rows: [{ id, fields: {} }] }),
    )
    const { wrapper } = await setup(schema, fetcher, null)
    const input = wrapper.get('.field-editor input')
    await input.setValue('17')
    await input.setValue('invalid')

    if (change === 'table')
      await wrapper.setProps({
        schema: { ...schema, table: { ...schema.table, id: 'other' } },
      })
    else if (change === 'view') await wrapper.setProps({ viewId: 'other' })
    else if (change === 'schema')
      await wrapper.setProps({ schema: { ...schema, schemaVersion: 2 } })
    else if (change === 'permission')
      await wrapper.setProps({ schema: { ...schema, myLevel: 'write' } })
    else {
      useAuthStore().setUser({ ...navUser, id: 'other' })
      await flushPromises()
      useAuthStore().setUser(navUser)
    }
    await flushPromises()

    expect(
      (wrapper.get('.field-editor input').element as HTMLInputElement).value,
    ).toBe('')
    await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
    await flushPromises()
    expect(fetcher).toHaveBeenCalledTimes(1)
    expect(JSON.parse(String(fetcher.mock.calls[0]?.[1]?.body))).toEqual({
      rows: [{ fields: {} }],
    })
  },
)

it.each(['table', 'view', 'schema', 'permission', 'identity'])(
  'discards applied draft attachments from the original context after %s changes',
  async (change) => {
    const attachment = {
      id: '0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b',
      name: 'note.txt',
      mime: 'text/plain',
      size: 4,
      thumb: null,
    }
    const fetcher = vi.fn<typeof fetch>(async (url) => {
      if (String(url).endsWith('/attachments')) return Response.json(attachment)
      if (String(url).endsWith('/discard'))
        return new Response(null, { status: 204 })
      throw new Error(`Unexpected request: ${String(url)}`)
    })
    const schema = schemaWith('attachment')
    const { wrapper } = await setup(schema, fetcher, null)
    const upload = wrapper.get('.attachment-file-input')
    Object.defineProperty(upload.element, 'files', {
      configurable: true,
      value: [new File(['test'], attachment.name, { type: attachment.mime })],
    })
    await upload.trigger('change')
    await flushPromises()
    expect(wrapper.find('.field-editor > button').exists()).toBe(false)
    if (change === 'table')
      await wrapper.setProps({
        schema: { ...schema, table: { ...schema.table, id: 'other' } },
      })
    else if (change === 'view') await wrapper.setProps({ viewId: 'other' })
    else if (change === 'schema')
      await wrapper.setProps({
        schema: { ...schema, schemaVersion: 2, fields: [] },
      })
    else if (change === 'permission')
      await wrapper.setProps({ schema: { ...schema, myLevel: 'read' } })
    else useAuthStore().setUser({ ...navUser, id: 'other' })
    await flushPromises()
    const discards = fetcher.mock.calls.filter(([url]) =>
      String(url).endsWith('/discard'),
    )
    expect(discards).toHaveLength(1)
    expect(String(discards[0]?.[0])).toContain(
      `/tables/t/fields/name/attachments/${attachment.id}/discard`,
    )
  },
)

it('does not discard copied attachments when a duplicate draft is closed', async () => {
  const attachment = {
    id: '0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b',
    name: 'note.txt',
    mime: 'text/plain',
    size: 4,
    thumb: null,
  }
  stored.fields.name = [attachment]
  const fetcher = vi.fn<typeof fetch>(async (_url, init) => {
    if (init?.method === 'GET') return Response.json(stored)
    throw new Error('Unexpected mutation')
  })
  const { wrapper } = await setup(schemaWith('attachment'), fetcher)
  await wrapper.get('[aria-label="Duplicate row"]').trigger('click')
  await wrapper.get('[aria-label="Close row"]').trigger('click')
  await flushPromises()
  expect(
    fetcher.mock.calls.some(([url]) => String(url).endsWith('/discard')),
  ).toBe(false)
})

it.each(['checkbox', 'single_select', 'multi_select'])(
  'uses the %s editor',
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
    if (kind === 'checkbox')
      await wrapper.get('.field-editor [role="checkbox"]').trigger('click')
    else
      await wrapper
        .get('select')
        .setValue(kind === 'single_select' ? 'Beta' : ['Beta', 'Alpha'])
    await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
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

it('shows non-system fields in metadata order and omits system fields and their directory requests', async () => {
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
  expect(wrapper.findAll('.row-field')).toHaveLength(2)
  expect(wrapper.findAll('.row-field h3').map((item) => item.text())).toEqual([
    'Name',
    'Hidden',
  ])
  expect(wrapper.findAll('.row-field-editor')).toHaveLength(2)
  expect(wrapper.find('[data-field-id="hidden"] select').exists()).toBe(true)
  expect(fetcher).not.toHaveBeenCalledWith(
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
  expect(wrapper.text()).not.toContain('只读系统字段')
  await wrapper.get('.row-search input').setValue('created')
  expect(
    wrapper.findAll('.row-field').filter((field) => field.isVisible()),
  ).toHaveLength(0)
})

it.each([
  ['integer', 'not an integer'],
  ['email', 'invalid@'],
])('does not persist codec-invalid %s', async (kind, input) => {
  stored.fields.name = null
  const { wrapper, fetcher } = await setup(schemaWith(kind))
  await wrapper.get('.field-editor input').setValue(input)
  await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
  await flushPromises()
  expect(wrapper.get('.field-editor [role="alert"]').text()).not.toBe('')
  expect(fetcher).toHaveBeenCalledTimes(1)
  expect(wrapper.emitted('saved')).toBeUndefined()
})

it('rejects UTF-8 oversized single-row bodies without truncation or a request', async () => {
  stored.fields.name = null
  const { wrapper, fetcher } = await setup(schemaWith('long_text'))
  await wrapper.get('textarea').setValue('中'.repeat(22000))
  await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
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
    await wrapper.get('textarea').setValue('x'.repeat(bytes - overhead))
    await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
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
  await wrapper.get('.field-editor input').setValue('Conflict')
  await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
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
  expect(wrapper.findAll('.row-field-value[role="button"]')).toHaveLength(0)
  useAuthStore().setUser({ ...navUser, id: 'someone-else' })
  await flushPromises()
  expect(wrapper.findAll('.row-field')).toHaveLength(0)
  expect(fetcher).toHaveBeenCalledTimes(1)
})

it('does not re-fetch on identical schema refreshes, but updates permission controls', async () => {
  const { wrapper, fetcher } = await setup()
  await wrapper.setProps({ schema: { ...tableSchema(), myLevel: 'read' } })
  expect(fetcher).toHaveBeenCalledTimes(1)
  expect(wrapper.findAll('.row-field-value[role="button"]')).toHaveLength(0)
  await wrapper.setProps({ schema: tableSchema() })
  expect(wrapper.findAll('.row-field-editor')).toHaveLength(1)
  expect(fetcher).toHaveBeenCalledTimes(1)
})

it('reports lost write permission when a realtime ACL refresh interrupts an edit', async () => {
  const { wrapper, fetcher } = await setup()
  await wrapper.get('.field-editor input').setValue('Unsaved draft')
  await wrapper.setProps({ schema: { ...tableSchema(), myLevel: 'read' } })
  expect(wrapper.get('[role="alert"]').text()).toContain(
    'You do not have permission',
  )
  expect(wrapper.find('.field-editor').exists()).toBe(false)
  expect(fetcher).toHaveBeenCalledTimes(1)
})

it('reports lost write permission while a blank new entry is open', async () => {
  const { wrapper } = await setup(tableSchema(), undefined, null)
  await wrapper.setProps({ schema: { ...tableSchema(), myLevel: 'read' } })
  expect(wrapper.get('[role="alert"]').text()).toContain(
    'You do not have permission',
  )
  expect(wrapper.find('button[aria-label="Save"]').exists()).toBe(false)
})

it.each([false, true])(
  'removes temporal editors cleanly when new-entry write access is revoked (fullscreen: %s)',
  async (fullscreen) => {
    const schema = tableSchema()
    schema.fields.push(
      { ...schema.fields[0]!, id: 'date', uiType: 'date', options: {} },
      { ...schema.fields[0]!, id: 'datetime', uiType: 'datetime', options: {} },
    )
    const consoleError = vi.spyOn(console, 'error')
    const { wrapper } = await setup(schema, undefined, null)
    if (fullscreen) {
      const workspace = document.createElement('div')
      vi.spyOn(workspace, 'getBoundingClientRect').mockReturnValue(
        new DOMRect(220, 120, 900, 500),
      )
      await wrapper.setProps({ workspace })
      await wrapper
        .get('[aria-label="Toggle table fullscreen"]')
        .trigger('click')
    }
    expect(
      wrapper
        .get('dialog.row-expand')
        .classes()
        .includes('row-expand-fullscreen'),
    ).toBe(fullscreen)
    const portalRoot = wrapper.get('[data-temporal-portal-root]')
    expect(portalRoot.findAll('.temporal-picker-portal')).toHaveLength(2)
    await wrapper.setProps({ schema: { ...schema, myLevel: 'read' } })
    await flushPromises()
    expect(portalRoot.findAll('.temporal-picker-portal')).toHaveLength(0)
    expect(wrapper.get('[role="alert"]').text()).toContain(
      'You do not have permission',
    )
    expect(consoleError).not.toHaveBeenCalled()
  },
)

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
  expect(wrapper.text()).not.toContain(i18n.global.t('grid.loading'))
  expect(wrapper.findAll('.row-field-value[role="button"]')).toHaveLength(0)
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
  expect(wrapper.findAll('.row-field-value[role="button"]')).toHaveLength(0)
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
  await wrapper.get('.field-editor input').setValue('Late secret')
  await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
  await wrapper.setProps({ rowId: '2' })
  useAuthStore().setUser({ ...navUser, id: 'other' })
  release(Response.json({ row: { id, fields: { name: 'Late secret' } } }))
  await flushPromises()
  expect(wrapper.text()).not.toContain('Late secret')
  expect(wrapper.emitted('saved')).toBeUndefined()
})

it('cancels the current draft and closes with native Escape cancellation', async () => {
  const { wrapper, fetcher } = await setup()
  await wrapper.get('.field-editor input').setValue('Unsaved')
  await wrapper.get('.field-editor input').trigger('keydown', { key: 'Escape' })
  expect(wrapper.get('dialog.row-expand').attributes('open')).toBeUndefined()
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
    await wrapper.get('.field-editor input').setValue('Late secret')
    await wrapper.get('.row-actions [aria-label="Save"]').trigger('click')
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
    if (change !== 'unmount')
      expect(wrapper.text()).not.toContain('Late secret')
    expect(wrapper.emitted('saved')).toBeUndefined()
  },
)
