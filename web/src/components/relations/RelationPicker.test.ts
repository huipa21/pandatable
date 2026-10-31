import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { i18n, loadLocale } from '../../i18n'
import type { DataRow, TableSchema } from '../../api/types'
import { useAuthStore } from '../../stores/auth'
import { useUndoStore } from '../../stores/undo'
import { navUser, tableSchema } from '../../test/navigation'
import RelationPicker from './RelationPicker.vue'

let pinia: ReturnType<typeof createPinia>
let wrapper: VueWrapper | undefined

beforeEach(async () => {
  pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().setUser(navUser)
  await loadLocale('en')
  await loadLocale('zh-CN')
  i18n.global.locale.value = 'en'
  HTMLDialogElement.prototype.showModal = function () {
    this.open = true
  }
  HTMLDialogElement.prototype.close = function () {
    this.open = false
  }
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  disposePinia(pinia)
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

it('confirms one-to-one replacement with both row labels and persists replacement', async () => {
  const schema = tableSchema()
  const field = {
    id: 'relation-field',
    name: 'Target',
    description: '',
    uiType: 'link',
    options: {
      relationId: 'relation',
      targetTableId: 'target',
      side: 'parent',
    },
    isSystem: false,
    isRequired: false,
    isUnique: false,
    default: null,
    position: 2048,
    version: 1,
  }
  schema.fields.push(field)
  schema.relations = [
    {
      id: 'relation',
      kind: 'one_one',
      parentTableId: 't',
      childTableId: 'target',
      parentFieldId: field.id,
      childFieldId: 'reverse-field',
      onDelete: 'set_null',
    },
  ]
  const targetSchema: TableSchema = {
    ...tableSchema(),
    table: {
      ...tableSchema().table,
      id: 'target',
      bookId: 'b',
      name: 'Target table',
      displayFieldId: 'target-name',
    },
    fields: [
      {
        ...tableSchema().fields[0]!,
        id: 'target-name',
        name: 'Name',
        position: 1024,
      },
    ],
    views: [{ ...tableSchema().views[0]!, id: 'target-view' }],
  }
  const replacement: DataRow = {
    id: '10',
    fields: { [field.id]: [{ id: '20', label: 'Right A' }] },
  }
  const fetcher = vi.fn(async (url: string, init?: RequestInit) => {
    if (url === '/api/v1/tables/target/schema')
      return Response.json(targetSchema)
    if (url === '/api/v1/views/target-view/query')
      return Response.json({
        rows: [{ id: '20', fields: { 'target-name': 'Right A' } }],
        fieldIds: ['target-name'],
        total: 1,
        schemaVersion: 1,
      })
    if (url === '/api/v1/tables/t/rows/8') {
      return Response.json({
        id: '8',
        fields: { name: 'Left A' },
      })
    }
    if (
      url.endsWith('/links/relation-field') &&
      !init?.body?.toString().includes('"replace":true')
    ) {
      return Response.json(
        {
          error: {
            code: 'ERR_RELATION_CONFLICT',
            message: 'server text',
            details: { relatedRowId: '8' },
          },
        },
        { status: 409 },
      )
    }
    if (url.endsWith('/links/relation-field'))
      return Response.json({
        row: replacement,
        changes: [
          {
            tableId: 't',
            rowId: '8',
            fieldId: 'relation-field',
            targetId: '20',
            action: 'unlink',
          },
          {
            tableId: 't',
            rowId: '10',
            fieldId: 'relation-field',
            targetId: '20',
            action: 'link',
          },
        ],
        eventId: 'event',
      })
    throw new Error(`Unexpected request: ${url}`)
  })
  const confirm = vi.fn(() => true)
  vi.stubGlobal('fetch', fetcher)
  vi.stubGlobal('confirm', confirm)
  wrapper = mount(RelationPicker, {
    props: {
      open: false,
      schema,
      field,
      rowId: '10',
      links: [],
      identity: navUser.id,
    },
    global: { plugins: [pinia, i18n] },
    attachTo: document.body,
  })
  await wrapper.setProps({ open: true })
  await flushPromises()

  const picker = wrapper.get('dialog')
  expect(fetcher.mock.calls.map(([url]) => url)).toEqual([
    '/api/v1/tables/target/schema',
    '/api/v1/views/target-view/query',
  ])
  await picker.get('input[type="checkbox"]').setValue(true)
  await picker
    .findAll('button')
    .find((button) => button.text() === 'Save links')!
    .trigger('click')
  await flushPromises()

  expect(confirm).toHaveBeenCalledWith(
    '“Right A” is already linked to “Left A”. Replace that link?',
  )
  const linkCalls = fetcher.mock.calls.filter(([url]) =>
    url.endsWith('/links/relation-field'),
  )
  expect(linkCalls).toHaveLength(2)
  expect(JSON.parse(String(linkCalls[0]![1]?.body))).toEqual({
    add: ['20'],
    remove: [],
  })
  expect(JSON.parse(String(linkCalls[1]![1]?.body))).toEqual({
    add: ['20'],
    remove: [],
    replace: true,
  })
  expect(wrapper.emitted('updated')?.[0]?.[0]).toEqual(replacement)
  expect(useUndoStore().undoStack[0]?.steps).toEqual([
    {
      kind: 'link',
      tableId: 't',
      rowId: '10',
      fieldId: 'relation-field',
      add: [],
      remove: ['20'],
    },
    {
      kind: 'link',
      tableId: 't',
      rowId: '8',
      fieldId: 'relation-field',
      add: ['20'],
      remove: [],
    },
  ])
})
