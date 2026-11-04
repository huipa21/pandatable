import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { markRaw, type Component } from 'vue'
import { i18n, loadLocale } from '../../i18n'
import type { RowQueryRequest, TableSchema } from '../../api/types'
import { useAuthStore } from '../../stores/auth'
import { useRealtimeStore } from '../../stores/realtime'
import { navUser, tableSchema } from '../../test/navigation'
import { ThemeMedia } from '../../test/setup'
import DataGrid from './DataGrid.vue'
import { formatTsv } from './tsv'

const gridState = vi.hoisted(() => ({
  fieldId: '',
  value: 'changed',
  rowIndex: 0,
  rowCount: 1,
  visibleFieldIds: [] as string[],
  rowFields: [] as Record<string, unknown>[],
  range: { x: 0, y: 0, x1: 0, y1: 0, rgCol: 'rgCol', rgRow: 'rgRow' },
  fillRange: { x: 0, y: 0, x1: 0, y1: 0 },
  focus: { rowIndex: 0, colIndex: 0, colType: 'rgCol' },
  clipboard: new Map<string, string>(),
  editCell: vi.fn(),
  focusCells: vi.fn(),
  scrollRow: vi.fn(),
}))

vi.mock('@revolist/vue3-datagrid', async () => {
  const vue = await import('vue')
  return {
    useTemplates: () => ({
      template: () => () => null,
    }),
    default: vue.defineComponent({
      name: 'RevoGrid',
      props: ['source', 'columns', 'editors', 'readonly', 'useClipboard'],
      emits: [
        'created',
        'beforeedit',
        'viewportscroll',
        'afterfocus',
        'beforecopyregion',
        'beforecopyapply',
        'beforepasteapply',
        'beforecut',
        'clearregion',
        'beforerangeedit',
        'aftercolumnresize',
        'columndragend',
      ],
      setup(_props, { emit }) {
        const root = vue.ref<HTMLElement>()
        vue.onMounted(() => {
          const element = root.value as
            | (HTMLElement & {
                getSourceStore: () => Promise<{ set: () => void }>
                getSelectedRange: () => Promise<typeof gridState.range>
              })
            | undefined
          if (element) {
            element.getSourceStore = async () => ({ set: vi.fn() })
            element.getSelectedRange = async () => gridState.range
            Object.assign(element, {
              setCellEdit: gridState.editCell,
              setCellsFocus: gridState.focusCells,
              scrollToRow: gridState.scrollRow,
              scrollToColumnProp: vi.fn(),
            })
          }
          emit('created')
        })
        return () =>
          vue.h('div', { ref: root, 'data-testid': 'fake-grid' }, [
            vue.h(
              'button',
              {
                type: 'button',
                onClick: () =>
                  emit(
                    'beforeedit',
                    new CustomEvent('beforeedit', {
                      detail: {
                        prop: gridState.fieldId,
                        val: gridState.value,
                        rowIndex: gridState.rowIndex,
                      },
                    }),
                  ),
              },
              'commit',
            ),
            vue.h(
              'button',
              {
                type: 'button',
                onClick: () =>
                  emit(
                    'afterfocus',
                    new CustomEvent('afterfocus', {
                      detail: {
                        model: _props.source[gridState.focus.rowIndex],
                        column: _props.columns.filter(
                          (column: { pin?: string }) =>
                            (column.pin ?? 'rgCol') === gridState.focus.colType,
                        )[gridState.focus.colIndex],
                        ...gridState.focus,
                      },
                    }),
                  ),
              },
              'focus',
            ),
            vue.h(
              'button',
              {
                type: 'button',
                onClick: () => {
                  emit(
                    'beforecopyregion',
                    new CustomEvent('beforecopyregion', {
                      detail: gridState.range,
                      cancelable: true,
                    }),
                  )
                  emit(
                    'beforecopyapply',
                    new CustomEvent('beforecopyapply', {
                      detail: {
                        data: [],
                        event: {
                          setData: (type: string, value: string) =>
                            gridState.clipboard.set(type, value),
                        },
                      },
                      cancelable: true,
                    }),
                  )
                },
              },
              'copy',
            ),
            vue.h(
              'button',
              {
                type: 'button',
                onClick: () =>
                  emit(
                    'beforepasteapply',
                    new CustomEvent('beforepasteapply', {
                      detail: {
                        parsed: [],
                        raw: '',
                        dataText: gridState.value,
                        event: new Event('paste'),
                      },
                      cancelable: true,
                    }),
                  ),
              },
              'paste',
            ),
            vue.h(
              'button',
              {
                type: 'button',
                onClick: () =>
                  emit(
                    'beforerangeedit',
                    new CustomEvent('beforerangeedit', {
                      detail: {
                        data: {},
                        models: {},
                        type: 'rgRow',
                        oldRange: gridState.range,
                        newRange: gridState.fillRange,
                      },
                      cancelable: true,
                    }),
                  ),
              },
              'fill',
            ),
            vue.h(
              'button',
              {
                type: 'button',
                onClick: () => {
                  emit(
                    'beforecopyregion',
                    new CustomEvent('beforecopyregion', {
                      detail: gridState.range,
                      cancelable: true,
                    }),
                  )
                  emit(
                    'beforecut',
                    new CustomEvent('beforecut', {
                      detail: { event: {} as ClipboardEvent },
                      cancelable: true,
                    }),
                  )
                  emit(
                    'clearregion',
                    new CustomEvent('clearregion', {
                      detail: {} as DataTransfer,
                      cancelable: true,
                    }),
                  )
                },
              },
              'cut',
            ),
          ])
      },
    }),
    VGridVueEditor: (component: unknown) => component,
  }
})

let pinia: ReturnType<typeof createPinia>
let wrapper: VueWrapper | undefined

function schemaFor(kind: string, level: TableSchema['myLevel'] = 'owner') {
  const schema = tableSchema()
  schema.myLevel = level
  schema.fields[0]!.uiType = kind
  schema.fields.push({
    id: 'system-id',
    name: 'ID',
    description: '',
    uiType: 'id',
    options: {},
    isSystem: true,
    isRequired: true,
    isUnique: false,
    default: null,
    position: 1_000_000_000,
    version: 1,
  })
  return schema
}

function addField(
  schema: TableSchema,
  id: string,
  kind: string,
  options: Record<string, unknown> = {},
) {
  schema.fields.push({
    id,
    name: id,
    description: '',
    uiType: kind,
    options,
    isSystem: false,
    isRequired: false,
    isUnique: false,
    default: null,
    position:
      Math.max(
        ...schema.fields
          .filter((field) => !field.isSystem)
          .map((field) => field.position),
      ) + 1024,
    version: 1,
  })
}

beforeEach(async () => {
  pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().setUser(navUser)
  const media = new ThemeMedia()
  media.matches = true
  vi.stubGlobal('matchMedia', () => media)
  const dialogPrototype = window.HTMLDialogElement.prototype
  if (!dialogPrototype.showModal) {
    Object.defineProperty(dialogPrototype, 'showModal', {
      configurable: true,
      value(this: HTMLDialogElement) {
        this.setAttribute('open', '')
      },
    })
    Object.defineProperty(dialogPrototype, 'close', {
      configurable: true,
      value(this: HTMLDialogElement) {
        this.removeAttribute('open')
      },
    })
  }
  await loadLocale('en')
  await loadLocale('zh-CN')
  i18n.global.locale.value = 'en'
  gridState.fieldId = 'name'
  gridState.value = 'changed'
  gridState.rowIndex = 0
  gridState.rowCount = 1
  gridState.visibleFieldIds = ['name']
  gridState.rowFields = []
  gridState.range = { x: 0, y: 0, x1: 0, y1: 0, rgCol: 'rgCol', rgRow: 'rgRow' }
  gridState.fillRange = { x: 0, y: 0, x1: 0, y1: 0 }
  gridState.focus = { rowIndex: 0, colIndex: 0, colType: 'rgCol' }
  gridState.clipboard.clear()
  gridState.editCell.mockReset()
  gridState.focusCells.mockReset()
  gridState.scrollRow.mockReset()
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  disposePinia(pinia)
  vi.unstubAllGlobals()
})

function queryResponse(
  schema: TableSchema,
  fieldIds = gridState.visibleFieldIds.length
    ? gridState.visibleFieldIds
    : [schema.fields[0]!.id],
) {
  return {
    rows: Array.from({ length: gridState.rowCount }, (_, index) => ({
      id: String(BigInt('9007199254740993') + BigInt(index)),
      fields: gridState.rowFields[index] ?? {
        [schema.fields[0]!.id]: 'before',
      },
    })),
    fieldIds,
    total: gridState.rowCount,
    schemaVersion: schema.schemaVersion,
  }
}

async function mountGrid(schema: TableSchema, identity = navUser.id) {
  const fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input)
    const body =
      typeof init?.body === 'string'
        ? (JSON.parse(init.body) as {
            fieldIds?: string[]
            aggregates?: { fieldId: string; aggregate: string }[]
          })
        : undefined
    if (path.endsWith('/query'))
      return Response.json(
        queryResponse(schema, body?.fieldIds ?? gridState.visibleFieldIds),
      )
    if (path.endsWith('/aggregates'))
      return Response.json({
        aggregates: (body?.aggregates ?? []).map((aggregate) => ({
          ...aggregate,
          value: 42,
        })),
        schemaVersion: schema.schemaVersion,
      })
    if (path === '/api/v1/views/v' && init?.method === 'PATCH') {
      const { config } = JSON.parse(String(init.body)) as {
        config: TableSchema['views'][number]['config']
      }
      return Response.json({
        view: {
          ...schema.views[0],
          config,
          version: schema.views[0]!.version + 1,
        },
        eventId: 'event',
      })
    }
    if (init?.method === 'PATCH') {
      const requestBody = JSON.parse(String(init.body)) as {
        fields?: Record<string, unknown>
        rows?: { id: string; fields: Record<string, unknown> }[]
      }
      if (requestBody.rows) {
        const patched = requestBody.rows.map(({ id, fields }) => {
          const index = Number(BigInt(id) - BigInt('9007199254740993'))
          const next = { ...(gridState.rowFields[index] ?? {}), ...fields }
          gridState.rowFields[index] = next
          return { id, fields: next }
        })
        return Response.json({ eventId: 'event', rows: patched })
      }
      return Response.json({
        row: {
          id: '9007199254740993',
          fields: { [schema.fields[0]!.id]: gridState.value },
        },
      })
    }
    if (init?.method === 'POST') {
      const requestBody = JSON.parse(String(init.body)) as {
        rows: { fields: Record<string, unknown> }[]
      }
      const rows = requestBody.rows.map(({ fields }, index) => {
        const id = String(
          BigInt('9007199254740993') + BigInt(gridState.rowCount + index),
        )
        return { id, fields }
      })
      gridState.rowCount += rows.length
      return Response.json({
        eventId: 'event',
        rows,
      })
    }
    if (init?.method === 'DELETE')
      return Response.json({ eventId: 'event', rows: [] })
    throw new Error(`Unexpected API request: ${path}`)
  })
  vi.stubGlobal('fetch', fetch)
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: DataGrid }],
  })
  wrapper = mount(DataGrid, {
    props: { schema, viewId: 'v', identity },
    global: { plugins: [pinia, i18n, router] },
  })
  await flushPromises()
  return fetch
}

it('keeps the add-field action outside the selectable grid columns', async () => {
  await mountGrid(schemaFor('text'))
  const columns = wrapper!
    .getComponent({ name: 'RevoGrid' })
    .props('columns') as { prop: string }[]
  expect(columns.map((column) => column.prop)).not.toContain('__add_field')
  const addField = wrapper!.get('button.grid-add-field')
  expect(addField.attributes('aria-label')).toBe('Add field')
  await addField.trigger('click')
  expect(wrapper!.emitted('createField')).toEqual([[]])
})

it('does not expose the add-field action to readers', async () => {
  await mountGrid(schemaFor('text', 'read'))
  expect(wrapper!.find('button.grid-add-field').exists()).toBe(false)
})

it('dismisses toolbar menus on outside pointer interactions', async () => {
  await mountGrid(schemaFor('text'))
  const root = wrapper!.element
  document.body.appendChild(root)
  try {
    const menu = wrapper!.get('details.grid-more').element as HTMLDetailsElement
    menu.open = true
    menu
      .querySelector('input')!
      .dispatchEvent(new MouseEvent('pointerdown', { bubbles: true }))
    expect(menu.open).toBe(true)
    document.body.dispatchEvent(
      new MouseEvent('pointerdown', { bubbles: true }),
    )
    expect(menu.open).toBe(false)
  } finally {
    root.remove()
  }
})

it('keeps an incomplete highlight draft while an earlier rule save finishes', async () => {
  const schema = schemaFor('text')
  schema.fields[0]!.filterOps = ['is']
  schema.views[0]!.config.highlight = [
    {
      id: 'saved',
      when: { field: 'name', cmp: 'is', value: 'Saved' },
      target: { type: 'row' },
      color: 'red-2',
    },
  ]
  const fetch = await mountGrid(markRaw(schema))
  await wrapper!
    .get(
      '.grid-highlight-rule .grid-highlight-controls label:last-child select',
    )
    .setValue('blue-2')
  await wrapper!.get('.grid-highlights > fieldset > button').trigger('click')
  expect(wrapper!.findAll('.grid-highlight-rule')).toHaveLength(2)

  const saves = () =>
    fetch.mock.calls.filter(
      ([path, init]) =>
        String(path) === '/api/v1/views/v' && init?.method === 'PATCH',
    )
  await vi.waitFor(() =>
    expect(wrapper!.emitted('viewUpdated')).toHaveLength(1),
  )
  const savedConfig = (
    JSON.parse(String(saves()[0]?.[1]?.body)) as {
      config: TableSchema['views'][number]['config']
    }
  ).config
  expect(savedConfig.highlight).toMatchObject([
    { id: 'saved', color: 'blue-2' },
  ])
  expect(wrapper!.findAll('.grid-highlight-rule')).toHaveLength(2)
  expect(wrapper!.get('.grid-highlights [role="status"]').text()).toContain(
    'Complete each condition',
  )

  const draft = wrapper!.findAll('.grid-highlight-rule')[1]!
  await draft.get('.filter-actions button').trigger('click')
  await draft.get('.filter-item input').setValue('New')
  await vi.waitFor(() => expect(saves()).toHaveLength(2))
  const completedConfig = (
    JSON.parse(String(saves()[1]?.[1]?.body)) as {
      config: TableSchema['views'][number]['config']
    }
  ).config
  expect(completedConfig.highlight).toHaveLength(2)
  expect(completedConfig.highlight?.[0]).toMatchObject({
    id: 'saved',
    color: 'blue-2',
  })
  expect(completedConfig.highlight?.[1]).toMatchObject({
    when: {
      op: 'and',
      items: [{ field: 'name', cmp: 'is', value: 'New' }],
    },
    target: { type: 'row' },
    color: 'red-2',
  })
})

it.each([
  { kind: 'text_array', operator: 'has_any_of' },
  { kind: 'json', operator: 'path_equals' },
] as const)(
  'keeps incomplete $kind highlight filter values local',
  async ({ kind, operator }) => {
    vi.useFakeTimers()
    try {
      const schema = schemaFor(kind)
      schema.fields[0]!.filterOps = [operator]
      const fetch = await mountGrid(markRaw(schema))
      await wrapper!
        .get('.grid-highlights > fieldset > button')
        .trigger('click')
      const draft = wrapper!.get('.grid-highlight-rule')
      await draft
        .get(
          kind === 'text_array'
            ? '.filter-actions button:nth-child(2)'
            : '.filter-actions button',
        )
        .trigger('click')
      const inputs = draft.findAll('.filter-item input')
      if (kind === 'json') await inputs[1]!.setValue('value')

      const saves = () =>
        fetch.mock.calls.filter(
          ([path, init]) =>
            String(path) === '/api/v1/views/v' && init?.method === 'PATCH',
        )
      await vi.advanceTimersByTimeAsync(350)
      expect(saves()).toHaveLength(0)
      expect(wrapper!.get('.grid-highlights [role="status"]').text()).toContain(
        'Complete each condition',
      )

      await inputs[0]!.setValue(
        kind === 'json' ? 'project.name' : 'first,second',
      )
      await vi.advanceTimersByTimeAsync(350)
      expect(saves()).toHaveLength(1)
      const config = (
        JSON.parse(String(saves()[0]?.[1]?.body)) as {
          config: TableSchema['views'][number]['config']
        }
      ).config
      const condition = {
        field: 'name',
        cmp: operator,
        value:
          kind === 'json'
            ? { path: ['project', 'name'], value: 'value' }
            : ['first', 'second'],
      }
      expect(config.highlight?.[0]?.when).toEqual({
        op: 'and',
        items:
          kind === 'json' ? [condition] : [{ op: 'and', items: [condition] }],
      })
    } finally {
      vi.useRealTimers()
    }
  },
)

it.each([
  { kind: 'text', operator: 'is_empty' },
  { kind: 'text', operator: 'is_not_empty' },
  { kind: 'checkbox', operator: 'is_checked' },
  { kind: 'checkbox', operator: 'is_not_checked' },
] as const)(
  'serializes a value-less $operator highlight condition for the server',
  async ({ kind, operator }) => {
    vi.useFakeTimers()
    try {
      const schema = schemaFor(kind)
      schema.fields[0]!.filterOps = [operator]
      const fetch = await mountGrid(markRaw(schema))
      await wrapper!
        .get('.grid-highlights > fieldset > button')
        .trigger('click')
      await wrapper!
        .get('.grid-highlight-rule .filter-actions button:nth-child(2)')
        .trigger('click')
      await vi.advanceTimersByTimeAsync(350)
      const save = fetch.mock.calls.find(
        ([path, init]) =>
          String(path) === '/api/v1/views/v' && init?.method === 'PATCH',
      )
      expect(save).toBeDefined()
      const config = (
        JSON.parse(String(save?.[1]?.body)) as {
          config: TableSchema['views'][number]['config']
        }
      ).config
      expect(config.highlight?.[0]?.when).toEqual({
        op: 'and',
        items: [
          {
            op: 'and',
            items: [{ field: 'name', cmp: operator, value: null }],
          },
        ],
      })
    } finally {
      vi.useRealTimers()
    }
  },
)

it.each([
  { kind: 'single_select', operator: 'is_any_of' },
  { kind: 'single_select', operator: 'is_none_of' },
  { kind: 'text_array', operator: 'has_any_of' },
  { kind: 'text_array', operator: 'has_all_of' },
  { kind: 'text_array', operator: 'has_none_of' },
  { kind: 'text_array', operator: 'is_exactly' },
] as const)(
  'does not save an empty $operator highlight operand',
  async ({ kind, operator }) => {
    vi.useFakeTimers()
    try {
      const schema = schemaFor(kind)
      schema.fields[0]!.filterOps = [operator]
      schema.views[0]!.config.highlight = [
        {
          id: 'empty-list',
          when: {
            op: 'and',
            items: [{ field: 'name', cmp: operator, value: [] }],
          },
          target: { type: 'row' },
          color: 'red-2',
        },
      ]
      const fetch = await mountGrid(markRaw(schema))
      await wrapper!
        .get(
          '.grid-highlight-rule .grid-highlight-controls label:last-child select',
        )
        .setValue('blue-2')
      await vi.advanceTimersByTimeAsync(350)
      expect(
        fetch.mock.calls.filter(
          ([path, init]) =>
            String(path) === '/api/v1/views/v' && init?.method === 'PATCH',
        ),
      ).toHaveLength(0)
      expect(wrapper!.get('.grid-highlights [role="status"]').text()).toContain(
        'Complete each condition',
      )
    } finally {
      vi.useRealTimers()
    }
  },
)

it.each([
  { kind: 'single_select', operator: 'is_none_of' },
  { kind: 'multi_select', operator: 'has_all_of' },
  { kind: 'multi_select', operator: 'has_none_of' },
  { kind: 'multi_select', operator: 'is_exactly' },
] as const)(
  'waits for a list selection for a new $operator highlight condition',
  async ({ kind, operator }) => {
    vi.useFakeTimers()
    try {
      const schema = schemaFor(kind)
      schema.fields[0]!.filterOps = [operator]
      schema.fields[0]!.options = {
        choices: [{ label: 'first' }, { label: 'second' }],
      }
      const fetch = await mountGrid(markRaw(schema))
      await wrapper!
        .get('.grid-highlights > fieldset > button')
        .trigger('click')
      await wrapper!
        .get('.grid-highlight-rule .filter-actions button')
        .trigger('click')
      const saves = () =>
        fetch.mock.calls.filter(
          ([path, init]) =>
            String(path) === '/api/v1/views/v' && init?.method === 'PATCH',
        )
      await vi.advanceTimersByTimeAsync(350)
      expect(saves()).toHaveLength(0)
      expect(wrapper!.get('.grid-highlights [role="status"]').text()).toContain(
        'Complete each condition',
      )

      await wrapper!
        .get('.grid-highlight-rule .filter-item select[multiple]')
        .setValue(['first', 'second'])
      await vi.advanceTimersByTimeAsync(350)
      expect(saves()).toHaveLength(1)
      const config = (
        JSON.parse(String(saves()[0]?.[1]?.body)) as {
          config: TableSchema['views'][number]['config']
        }
      ).config
      expect(config.highlight?.[0]?.when).toEqual({
        op: 'and',
        items: [{ field: 'name', cmp: operator, value: ['first', 'second'] }],
      })
    } finally {
      vi.useRealTimers()
    }
  },
)

it('keeps an incomplete highlight range local until both bounds are present', async () => {
  vi.useFakeTimers()
  try {
    const schema = schemaFor('integer')
    schema.fields[0]!.filterOps = ['between']
    schema.views[0]!.config.highlight = [
      {
        id: 'range',
        when: { field: 'name', cmp: 'between', value: ['1'] },
        target: { type: 'row' },
        color: 'red-2',
      },
    ]
    const fetch = await mountGrid(markRaw(schema))
    await wrapper!
      .get(
        '.grid-highlight-rule .grid-highlight-controls label:last-child select',
      )
      .setValue('blue-2')
    await vi.advanceTimersByTimeAsync(350)
    const saves = () =>
      fetch.mock.calls.filter(
        ([path, init]) =>
          String(path) === '/api/v1/views/v' && init?.method === 'PATCH',
      )
    expect(saves()).toHaveLength(0)
    expect(wrapper!.get('.grid-highlights [role="status"]').text()).toContain(
      'Complete each condition',
    )

    await wrapper!
      .findAll('.grid-highlight-rule .filter-item input')[1]!
      .setValue('10')
    await vi.advanceTimersByTimeAsync(350)
    expect(saves()).toHaveLength(1)
    const config = (
      JSON.parse(String(saves()[0]?.[1]?.body)) as {
        config: TableSchema['views'][number]['config']
      }
    ).config
    expect(config.highlight?.[0]?.when).toEqual({
      op: 'and',
      items: [{ field: 'name', cmp: 'between', value: ['1', '10'] }],
    })
  } finally {
    vi.useRealTimers()
  }
})

it('preserves relative-user, bigint, and datetime filter values when preparing highlight saves', async () => {
  vi.useFakeTimers()
  try {
    const schema = schemaFor('text')
    schema.fields[0]!.filterOps = ['is_empty']
    addField(schema, 'amount', 'integer')
    addField(schema, 'due', 'datetime')
    addField(schema, 'owner', 'user')
    schema.fields.find((field) => field.id === 'amount')!.filterOps = ['gte']
    schema.fields.find((field) => field.id === 'due')!.filterOps = ['before']
    schema.fields.find((field) => field.id === 'owner')!.filterOps = [
      'has_any_of',
    ]
    schema.views[0]!.config.highlight = [
      {
        id: 'precise',
        when: {
          op: 'and',
          items: [
            { field: 'name', cmp: 'is_empty' },
            { field: 'amount', cmp: 'gte', value: '9007199254740993' },
            {
              field: 'due',
              cmp: 'before',
              value: '2026-03-29T01:15:12.123456Z',
            },
            { field: 'owner', cmp: 'has_any_of', value: { rel: 'me' } },
          ],
        },
        target: { type: 'row' },
        color: 'red-2',
      },
    ]
    const fetch = await mountGrid(markRaw(schema))
    await wrapper!
      .get(
        '.grid-highlight-rule .grid-highlight-controls label:last-child select',
      )
      .setValue('blue-2')
    await vi.advanceTimersByTimeAsync(350)
    const save = fetch.mock.calls.find(
      ([path, init]) =>
        String(path) === '/api/v1/views/v' && init?.method === 'PATCH',
    )
    expect(save).toBeDefined()
    const config = (
      JSON.parse(String(save?.[1]?.body)) as {
        config: TableSchema['views'][number]['config']
      }
    ).config
    expect(config.highlight?.[0]?.when).toEqual({
      op: 'and',
      items: [
        { field: 'name', cmp: 'is_empty', value: null },
        { field: 'amount', cmp: 'gte', value: '9007199254740993' },
        {
          field: 'due',
          cmp: 'before',
          value: '2026-03-29T01:15:12.123456Z',
        },
        { field: 'owner', cmp: 'has_any_of', value: { rel: 'me' } },
      ],
    })
  } finally {
    vi.useRealTimers()
  }
})

it('preserves valid empty JSON arrays in highlight equality conditions', async () => {
  vi.useFakeTimers()
  try {
    const schema = schemaFor('json')
    schema.fields[0]!.filterOps = ['is']
    schema.views[0]!.config.highlight = [
      {
        id: 'empty-json-array',
        when: { field: 'name', cmp: 'is', value: [] },
        target: { type: 'row' },
        color: 'red-2',
      },
    ]
    const fetch = await mountGrid(markRaw(schema))
    expect(wrapper!.find('.grid-highlights [role="status"]').exists()).toBe(
      false,
    )
    await wrapper!
      .get(
        '.grid-highlight-rule .grid-highlight-controls label:last-child select',
      )
      .setValue('blue-2')
    await vi.advanceTimersByTimeAsync(350)
    const save = fetch.mock.calls.find(
      ([path, init]) =>
        String(path) === '/api/v1/views/v' && init?.method === 'PATCH',
    )
    expect(save).toBeDefined()
    const config = (
      JSON.parse(String(save?.[1]?.body)) as {
        config: TableSchema['views'][number]['config']
      }
    ).config
    expect(config.highlight?.[0]?.when).toEqual({
      field: 'name',
      cmp: 'is',
      value: [],
    })
  } finally {
    vi.useRealTimers()
  }
})

it('labels the highlight menu and its rule help in both locales', async () => {
  const schema = schemaFor('text')
  schema.fields[0]!.filterOps = ['is']
  await mountGrid(schema)
  const summary = wrapper!.get('.grid-highlights > summary')
  expect(summary.attributes('aria-label')).toBe('Highlight')
  expect(summary.text()).toBe('Highlight')
  expect(summary.find('svg').exists()).toBe(true)
  expect(wrapper!.get('.grid-highlights').text()).toContain(
    'The first matching rule wins for each row and each cell.',
  )
  expect(wrapper!.get('.grid-highlights').text()).toContain(
    'A cell match overrides the row color.',
  )

  await wrapper!.get('.grid-highlights > fieldset > button').trigger('click')
  expect(
    wrapper!.get('.grid-highlight-controls label:last-child').text(),
  ).toContain('Color for rule 1')
  i18n.global.locale.value = 'zh-CN'
  await flushPromises()
  expect(summary.attributes('aria-label')).toBe('高亮')
  expect(summary.text()).toBe('高亮')
  expect(wrapper!.get('.grid-highlights').text()).toContain(
    '每一行和每个单元格分别使用第一条匹配的规则',
  )
  expect(wrapper!.get('.grid-highlights').text()).toContain(
    '单元格匹配会覆盖行颜色',
  )
  expect(
    wrapper!.get('.grid-highlight-controls label:last-child').text(),
  ).toContain('规则 1 的颜色')
})

it.each([
  { level: 'read', scope: 'shared', locked: false, visible: false },
  { level: 'write', scope: 'shared', locked: true, visible: false },
  { level: 'read', scope: 'personal', locked: false, visible: true },
  { level: 'owner', scope: 'shared', locked: true, visible: true },
] as const)(
  'keeps highlight editing permission for $level / $scope / locked=$locked',
  async ({ level, scope, locked, visible }) => {
    const schema = schemaFor('text', level)
    schema.views[0]!.scope = scope
    schema.views[0]!.locked = locked
    await mountGrid(schema)
    expect(wrapper!.find('.grid-highlights').exists()).toBe(visible)
  },
)

it('discards local highlight drafts when view-edit permissions change', async () => {
  const schema = schemaFor('text')
  schema.fields[0]!.filterOps = ['is']
  schema.views[0]!.config.highlight = [
    {
      id: 'saved',
      when: { field: 'name', cmp: 'is', value: 'Saved' },
      target: { type: 'row' },
      color: 'red-2',
    },
  ]
  await mountGrid(markRaw(schema))
  await wrapper!.get('.grid-highlights > fieldset > button').trigger('click')
  expect(wrapper!.findAll('.grid-highlight-rule')).toHaveLength(2)

  await wrapper!.setProps({ schema: markRaw({ ...schema, myLevel: 'read' }) })
  expect(wrapper!.find('.grid-highlights').exists()).toBe(false)
  await wrapper!.setProps({ schema: markRaw(schema) })
  expect(wrapper!.findAll('.grid-highlight-rule')).toHaveLength(1)
  expect(
    (
      wrapper!.get('.grid-highlight-rule .filter-item input')
        .element as HTMLInputElement
    ).value,
  ).toBe('Saved')
})

it('enforces the 20-rule boundary without saving an incomplete replacement', async () => {
  const schema = schemaFor('text')
  schema.fields[0]!.filterOps = ['is']
  schema.views[0]!.config.highlight = Array.from(
    { length: 20 },
    (_, index) => ({
      id: `saved-${index}`,
      when: { field: 'name', cmp: 'is', value: `Value ${index}` },
      target: { type: 'row' },
      color: 'red-2',
    }),
  )
  const fetch = await mountGrid(markRaw(schema))
  const add = wrapper!.get('.grid-highlights > fieldset > button')
  expect(add.attributes('disabled')).toBeDefined()
  expect(wrapper!.get('.grid-highlights').text()).toContain(
    'A view can have up to 20 highlight rules.',
  )
  await wrapper!
    .get('button[aria-label="Remove highlight rule 20"]')
    .trigger('click')
  expect(add.attributes('disabled')).toBeUndefined()
  await add.trigger('click')
  expect(wrapper!.findAll('.grid-highlight-rule')).toHaveLength(20)
  expect(add.attributes('disabled')).toBeDefined()

  await vi.waitFor(() =>
    expect(wrapper!.emitted('viewUpdated')).toHaveLength(1),
  )
  const save = fetch.mock.calls.find(
    ([path, init]) =>
      String(path) === '/api/v1/views/v' && init?.method === 'PATCH',
  )
  const config = (
    JSON.parse(String(save?.[1]?.body)) as {
      config: TableSchema['views'][number]['config']
    }
  ).config
  expect(config.highlight).toHaveLength(19)
  expect(config.highlight?.map((rule) => rule.id)).not.toContain('saved-19')
  expect(wrapper!.findAll('.grid-highlight-rule')).toHaveLength(20)
})

it('keeps highlight drafts through a version conflict and preserves concurrent view settings', async () => {
  const schema = schemaFor('text')
  schema.fields[0]!.filterOps = ['is']
  schema.views[0]!.config.highlight = [
    {
      id: 'saved',
      when: { field: 'name', cmp: 'is', value: 'Saved' },
      target: { type: 'row' },
      color: 'red-2',
    },
  ]
  const latest = structuredClone(schema)
  latest.views[0]!.version = 7
  latest.views[0]!.config.rowHeight = 'tall'
  const fetch = await mountGrid(markRaw(schema))
  const fallback = fetch.getMockImplementation()!
  let patches = 0
  fetch.mockImplementation(async (input, init) => {
    const path = String(input)
    if (path === '/api/v1/views/v' && init?.method === 'PATCH') {
      patches++
      if (patches === 1)
        return Response.json(
          { error: { code: 'ERR_VERSION_CONFLICT' } },
          { status: 409 },
        )
      const { config } = JSON.parse(String(init.body)) as {
        config: TableSchema['views'][number]['config']
      }
      return Response.json({
        view: { ...latest.views[0], version: 8, config },
        eventId: 'event',
      })
    }
    if (path === '/api/v1/tables/t/schema') return Response.json(latest)
    return fallback(input, init)
  })

  await wrapper!
    .get(
      '.grid-highlight-rule .grid-highlight-controls label:last-child select',
    )
    .setValue('blue-2')
  await wrapper!.get('.grid-highlights > fieldset > button').trigger('click')
  await vi.waitFor(
    () => expect(wrapper!.emitted('viewUpdated')).toHaveLength(2),
    { timeout: 5000 },
  )
  const saves = () =>
    fetch.mock.calls.filter(
      ([path, init]) =>
        String(path) === '/api/v1/views/v' && init?.method === 'PATCH',
    )
  expect(saves()).toHaveLength(2)
  expect(saves()[0]?.[1]?.headers).toMatchObject({ 'If-Match': '1' })
  expect(saves()[1]?.[1]?.headers).toMatchObject({ 'If-Match': '7' })
  const retried = (
    JSON.parse(String(saves()[1]?.[1]?.body)) as {
      config: TableSchema['views'][number]['config']
    }
  ).config
  expect(retried.rowHeight).toBe('tall')
  expect(retried.highlight).toMatchObject([{ id: 'saved', color: 'blue-2' }])
  expect(wrapper!.find('[role="alert"]').exists()).toBe(true)
  expect(wrapper!.findAll('.grid-highlight-rule')).toHaveLength(2)

  const draft = wrapper!.findAll('.grid-highlight-rule')[1]!
  await draft.get('.filter-actions button').trigger('click')
  await draft.get('.filter-item input').setValue('New')
  await vi.waitFor(() => expect(saves()).toHaveLength(3))
  const completed = (
    JSON.parse(String(saves()[2]?.[1]?.body)) as {
      config: TableSchema['views'][number]['config']
    }
  ).config
  expect(saves()[2]?.[1]?.headers).toMatchObject({ 'If-Match': '8' })
  expect(completed.rowHeight).toBe('tall')
  expect(completed.highlight).toHaveLength(2)
})

it('reports grid focus and clears it when focus leaves the grid', async () => {
  await mountGrid(schemaFor('text', 'read'))
  const setFocus = vi.spyOn(useRealtimeStore(), 'setFocus')
  await wrapper!
    .findAll('button')
    .find((button) => button.text() === 'focus')!
    .trigger('click')
  expect(setFocus).toHaveBeenLastCalledWith('t', {
    rowId: '9007199254740993',
    fieldId: 'name',
  })
  await wrapper!.get('[data-testid="fake-grid"]').trigger('focusout', {
    relatedTarget: document.body,
  })
  expect(setFocus).toHaveBeenLastCalledWith('t', null)
})

it('keeps row ordinals separate from bigint identity and exposes view-ordered navigation', async () => {
  gridState.rowCount = 3
  await mountGrid(schemaFor('text'))
  const engine = wrapper!.findComponent({ name: 'RevoGrid' })
  const source = engine.props('source') as {
    __rowId: string
    __rowNumber: number
  }[]
  expect(source.slice(0, 3).map((row) => row.__rowNumber)).toEqual([1, 2, 3])
  expect(source[0]!.__rowId).toBe('9007199254740993')
  expect((engine.props('columns') as { name: string }[])[0]!.name).toBe('#')
  const grid = wrapper!.vm as unknown as {
    navigateRow: (id: string, direction: -1 | 1) => Promise<string | null>
  }
  expect(await grid.navigateRow(source[0]!.__rowId, 1)).toBe(source[1]!.__rowId)
  expect(await grid.navigateRow(source[0]!.__rowId, -1)).toBeNull()
  await expect(grid.navigateRow('missing', 1)).rejects.toThrow(
    'not in the loaded view',
  )
})

it('keeps presence on a cell while its editor receives focus', async () => {
  await mountGrid(schemaFor('text'))
  const setFocus = vi.spyOn(useRealtimeStore(), 'setFocus')
  const editors = wrapper!
    .findComponent({ name: 'RevoGrid' })
    .props('editors') as Record<string, Component>
  const editor = mount(editors.name!, {
    props: {
      column: { prop: 'name' },
      prop: 'name',
      rowIndex: 0,
      val: 'before',
      save: vi.fn(),
      close: vi.fn(),
    },
    global: { plugins: [pinia, i18n] },
  })
  try {
    expect(setFocus).toHaveBeenLastCalledWith('t', {
      rowId: '9007199254740993',
      fieldId: 'name',
    })
    setFocus.mockClear()
    await wrapper!.get('[data-testid="fake-grid"]').trigger('focusout', {
      relatedTarget: document.body,
    })
    expect(setFocus).not.toHaveBeenCalled()
  } finally {
    editor.unmount()
    setFocus.mockRestore()
  }
})

it('expands the focused loaded row with Space but leaves native controls and IME alone', async () => {
  await mountGrid(schemaFor('text', 'read'))
  await wrapper!
    .findAll('button')
    .find((button) => button.text() === 'focus')!
    .trigger('click')
  await wrapper!
    .get('.data-grid')
    .trigger('keydown', { key: ' ', isComposing: true })
  expect(wrapper!.emitted('expand')).toBeUndefined()
  await wrapper!.get('.data-grid').trigger('keydown', { key: ' ' })
  expect(wrapper!.emitted('expand')).toEqual([['9007199254740993']])
  await wrapper!.get('button').trigger('keydown', { key: ' ' })
  expect(wrapper!.emitted('expand')).toHaveLength(1)
})

it('queries the real view and persists a cell edit without coercing bigint row IDs', async () => {
  const schema = schemaFor('text')
  const fetch = await mountGrid(schema)
  await wrapper!.get('[data-testid="fake-grid"] button').trigger('click')
  await flushPromises()

  const query = fetch.mock.calls.find(([path]) =>
    String(path).endsWith('/query'),
  )?.[1]
  expect(query).toMatchObject({
    method: 'POST',
    body: JSON.stringify({
      offset: 0,
      limit: 100,
      includeCount: true,
      sorts: [],
    }),
  })
  const patch = fetch.mock.calls.find(([path]) =>
    String(path).endsWith('/9007199254740993'),
  )?.[1]
  expect(patch).toMatchObject({
    method: 'PATCH',
    body: JSON.stringify({ fields: { name: 'changed' } }),
  })
})

it('sends transient quick search and renders configured footer aggregates', async () => {
  const schema = schemaFor('text')
  addField(schema, 'amount', 'integer')
  schema.fields.find((field) => field.id === 'amount')!.options = {
    size: 'int',
  }
  const fetch = await mountGrid(schema)

  await wrapper!.get('input[type="search"]').setValue('acme')
  await new Promise((resolve) => setTimeout(resolve, 300))
  await flushPromises()
  const query = fetch.mock.calls
    .filter(([path]) => String(path).endsWith('/query'))
    .map(([, init]) => JSON.parse(String(init?.body)) as RowQueryRequest)
    .at(-1)
  expect(query?.search).toBe('acme')
  expect(wrapper!.vm.$.exposed?.exportQuery()).toMatchObject({
    fieldIds: ['name', 'amount'],
    sorts: [],
    groupBy: [],
    search: 'acme',
  })

  await wrapper!
    .get(
      `select[aria-label="${i18n.global.t('grid.aggregateForField', { field: 'amount' })}"]`,
    )
    .setValue('sum')
  await flushPromises()
  const request = fetch.mock.calls
    .filter(([path]) => String(path).endsWith('/aggregates'))
    .at(-1)
  expect(request?.[1]?.method).toBe('POST')
  expect(JSON.parse(String(request?.[1]?.body))).toMatchObject({
    search: 'acme',
    aggregates: [{ fieldId: 'amount', aggregate: 'sum' }],
  })
  expect(wrapper!.get('.grid-aggregate-footer').text()).toContain('42')
})

it('adds, prioritizes, changes, and removes transient sort criteria', async () => {
  const schema = schemaFor('text')
  addField(schema, 'status', 'text')
  addField(schema, 'due', 'date')
  const fetch = await mountGrid(schema)
  const controls = wrapper!.get('details.grid-sorts')
  const addFieldSelect = controls.get(
    `select[aria-label="${i18n.global.t('grid.addSortField')}"]`,
  )
  const addCriterion = () =>
    controls
      .findAll('button')
      .find((button) => button.text() === i18n.global.t('grid.addSort'))!
      .trigger('click')

  await addFieldSelect.setValue('status')
  await addCriterion()
  await flushPromises()
  await controls
    .get(
      `select[aria-label="${i18n.global.t('grid.sortFieldPosition', { position: 1 })}"]`,
    )
    .setValue('status')
  await controls
    .get(
      `select[aria-label="${i18n.global.t('grid.sortDirection', { field: 'status' })}"]`,
    )
    .setValue('desc')
  await flushPromises()

  await addFieldSelect.setValue('due')
  await addCriterion()
  await flushPromises()
  await controls
    .get(
      `select[aria-label="${i18n.global.t('grid.sortDirection', { field: 'due' })}"]`,
    )
    .setValue('asc')
  await controls
    .get(
      `button[aria-label="${i18n.global.t('grid.moveSortUp', { field: 'due' })}"]`,
    )
    .trigger('click')
  await flushPromises()

  const requests = () =>
    fetch.mock.calls
      .filter(([path]) => String(path).endsWith('/query'))
      .map(([, init]) => JSON.parse(String(init?.body)) as RowQueryRequest)
  expect(requests().at(-1)?.sorts).toEqual([
    { fieldId: 'due', direction: 'asc' },
    { fieldId: 'status', direction: 'desc' },
  ])
  expect(wrapper!.vm.$.exposed?.exportQuery().sorts).toEqual([
    { fieldId: 'due', direction: 'asc' },
    { fieldId: 'status', direction: 'desc' },
  ])
  await controls
    .get(
      `button[aria-label="${i18n.global.t('grid.removeSortField', { field: 'status' })}"]`,
    )
    .trigger('click')
  await flushPromises()
  expect(requests().at(-1)?.sorts).toEqual([
    { fieldId: 'due', direction: 'asc' },
  ])
})

it('shows codec validation in the cell and never sends an invalid integer write', async () => {
  const schema = schemaFor('integer')
  schema.fields[0]!.options = { size: 'int' }
  gridState.value = 'abc'
  const fetch = await mountGrid(schema)
  await wrapper!.get('[data-testid="fake-grid"] button').trigger('click')
  await flushPromises()

  expect(wrapper!.get('[role="alert"]').text()).toContain(
    i18n.global.t('field.error.invalid_integer'),
  )
  expect(fetch.mock.calls.some(([, init]) => init?.method === 'PATCH')).toBe(
    false,
  )
})

it('keeps read access read-only and explicitly fetches a selected system field', async () => {
  const schema = schemaFor('text', 'read')
  const fetch = await mountGrid(schema)
  expect(wrapper!.find('.grid-add-row').exists()).toBe(false)
  const fieldCheckboxes = wrapper!.findAll('input[type="checkbox"]')
  expect(fieldCheckboxes).toHaveLength(2)
  await fieldCheckboxes[1]!.setValue(true)
  await flushPromises()

  const queryBodies = fetch.mock.calls
    .filter(([path]) => String(path).endsWith('/query'))
    .map(
      ([, init]) =>
        JSON.parse(String(init?.body)) as {
          fieldIds?: string[]
          includeCount?: boolean
        },
    )
  expect(queryBodies.at(-1)?.fieldIds).toEqual(['name', 'system-id'])
  expect(queryBodies.at(-1)?.includeCount).toBe(true)
})

it('pastes quoted multiline and Chinese TSV, reporting invalid cells while patching valid cells once', async () => {
  const schema = schemaFor('text')
  addField(schema, 'note', 'long_text')
  addField(schema, 'qty', 'integer')
  schema.fields.find((field) => field.id === 'qty')!.options = { size: 'big' }
  addField(schema, 'email', 'email')
  gridState.visibleFieldIds = ['name', 'note', 'qty', 'email']
  gridState.rowCount = 3
  gridState.rowFields = Array.from({ length: 3 }, () => ({
    name: 'before',
    note: 'before',
    qty: 1,
    email: 'before@example.com',
  }))
  gridState.focus = { rowIndex: 0, colIndex: 0, colType: 'rgCol' }
  gridState.value = formatTsv([
    ['中文', 'line one\nline two', '42', 'valid@example.com'],
    ['quoted "text"', '多行\n内容', 'abc', 'x@'],
    ['last', 'tail', '9007199254740993', 'last@example.com'],
  ])
  const fetch = await mountGrid(schema)
  await wrapper!
    .get('[data-testid="fake-grid"] button:nth-child(2)')
    .trigger('click')
  await wrapper!
    .get('[data-testid="fake-grid"] button:nth-child(4)')
    .trigger('click')
  await flushPromises()

  const patches = fetch.mock.calls.filter(
    ([, init]) => init?.method === 'PATCH',
  )
  expect(patches).toHaveLength(1)
  const requestBody = JSON.parse(String(patches[0]?.[1]?.body)) as {
    rows: { id: string; fields: Record<string, unknown> }[]
  }
  expect(requestBody.rows).toHaveLength(3)
  expect(
    requestBody.rows.reduce(
      (count, row) => count + Object.keys(row.fields).length,
      0,
    ),
  ).toBe(10)
  expect(requestBody.rows[0]?.fields).toEqual({
    name: '中文',
    note: 'line one\nline two',
    qty: '42',
    email: 'valid@example.com',
  })
  expect(requestBody.rows[1]?.fields).toEqual({
    name: 'quoted "text"',
    note: '多行\n内容',
  })
  expect(wrapper!.text()).toContain('Row 2, field qty:')
  expect(wrapper!.text()).toContain('Row 2, field email:')
})

it('confirms and creates exactly the rows needed when paste extends beyond the table', async () => {
  const schema = schemaFor('text')
  gridState.rowCount = 3
  gridState.rowFields = Array.from({ length: 3 }, () => ({ name: 'before' }))
  gridState.focus = { rowIndex: 2, colIndex: 0, colType: 'rgCol' }
  gridState.value = formatTsv(
    Array.from({ length: 10 }, (_, index) => [`new ${index + 1}`]),
  )
  const fetch = await mountGrid(schema)
  await wrapper!
    .get('[data-testid="fake-grid"] button:nth-child(2)')
    .trigger('click')
  await wrapper!
    .get('[data-testid="fake-grid"] button:nth-child(4)')
    .trigger('click')

  const dialog = wrapper!.get('dialog[open]')
  expect(dialog.text()).toContain('Add 9 rows to complete this paste?')
  await dialog.get('button:last-child').trigger('click')
  await flushPromises()

  const patch = fetch.mock.calls.find(([, init]) => init?.method === 'PATCH')
  const insert = fetch.mock.calls.find(
    ([, init]) =>
      init?.method === 'POST' && String(init.body).includes('"rows"'),
  )
  expect(patch).toBeDefined()
  expect(
    (
      JSON.parse(String(insert?.[1]?.body)) as {
        rows: { fields: Record<string, unknown> }[]
      }
    ).rows,
  ).toHaveLength(9)
  expect(gridState.rowCount).toBe(12)
})

it('does not add blank rows when every pasted cell in new rows is invalid', async () => {
  const schema = schemaFor('integer')
  gridState.rowCount = 1
  gridState.rowFields = [{ name: 7 }]
  gridState.focus = { rowIndex: 1, colIndex: 0, colType: 'rgCol' }
  gridState.value = 'not an integer'
  const fetch = await mountGrid(schema)
  await wrapper!
    .get('[data-testid="fake-grid"] button:nth-child(2)')
    .trigger('click')
  await wrapper!
    .get('[data-testid="fake-grid"] button:nth-child(4)')
    .trigger('click')
  await flushPromises()

  expect(
    fetch.mock.calls.some(
      ([path, init]) =>
        init?.method === 'POST' && String(path).endsWith('/rows'),
    ),
  ).toBe(false)
  expect(gridState.rowCount).toBe(1)
  expect(wrapper!.text()).toContain('Row 2, field Name:')
  expect(wrapper!.find('dialog[open]').exists()).toBe(false)
})

it('copies codec-formatted select labels and dates as plain TSV and safe HTML', async () => {
  const schema = schemaFor('single_select')
  schema.fields[0]!.options = {
    choices: [{ id: 'active', label: 'Active', color: 'green' }],
  }
  addField(schema, 'date', 'date')
  gridState.visibleFieldIds = ['name', 'date']
  gridState.rowFields = [{ name: 'Active', date: '2026-10-27' }]
  gridState.range = {
    x: 0,
    y: 0,
    x1: 1,
    y1: 0,
    rgCol: 'rgCol',
    rgRow: 'rgRow',
  }
  await mountGrid(schema)
  await wrapper!
    .get('[data-testid="fake-grid"] button:nth-child(3)')
    .trigger('click')

  expect(gridState.clipboard.get('text/plain')).toBe('Active\t2026-10-27')
  expect(gridState.clipboard.get('text/html')).toContain(
    '<table><tbody><tr><td>Active</td><td>2026-10-27</td></tr></tbody></table>',
  )
})

it('cuts editable cells through a bulk patch without changing row IDs', async () => {
  const schema = schemaFor('text')
  gridState.rowFields = [{ name: 'cut me' }]
  const fetch = await mountGrid(schema)
  await wrapper!
    .get('[data-testid="fake-grid"] button:last-child')
    .trigger('click')
  await flushPromises()

  const patch = fetch.mock.calls.find(([, init]) => init?.method === 'PATCH')
  expect(patch).toBeDefined()
  expect(JSON.parse(String(patch?.[1]?.body))).toEqual({
    rows: [{ id: '9007199254740993', fields: { name: null } }],
  })
})

it('fills down with Ctrl+D and the vertical fill handle through one bulk patch each', async () => {
  const schema = schemaFor('text')
  gridState.rowCount = 6
  gridState.rowFields = Array.from({ length: 6 }, (_, index) => ({
    name: index === 0 ? 'top' : `row ${index}`,
  }))
  gridState.range = {
    x: 0,
    y: 0,
    x1: 0,
    y1: 5,
    rgCol: 'rgCol',
    rgRow: 'rgRow',
  }
  const fetch = await mountGrid(schema)
  await wrapper!.get('[data-testid="fake-grid"]').trigger('keydown', {
    key: 'd',
    ctrlKey: true,
  })

  await flushPromises()
  let patchCalls = fetch.mock.calls.filter(
    ([, init]) => init?.method === 'PATCH',
  )
  expect(patchCalls).toHaveLength(1)
  expect(
    (
      JSON.parse(String(patchCalls[0]?.[1]?.body)) as {
        rows: { fields: Record<string, unknown> }[]
      }
    ).rows.every((row) => row.fields.name === 'top'),
  ).toBe(true)

  gridState.range = { x: 0, y: 0, x1: 0, y1: 0, rgCol: 'rgCol', rgRow: 'rgRow' }
  gridState.fillRange = { x: 0, y: 0, x1: 0, y1: 3 }
  await wrapper!
    .get('[data-testid="fake-grid"] button:nth-child(5)')
    .trigger('click')
  await flushPromises()
  patchCalls = fetch.mock.calls.filter(([, init]) => init?.method === 'PATCH')
  expect(patchCalls).toHaveLength(2)
  expect(
    (
      JSON.parse(String(patchCalls[1]?.[1]?.body)) as {
        rows: { fields: Record<string, unknown> }[]
      }
    ).rows,
  ).toHaveLength(3)

  gridState.fillRange = { x: 0, y: 0, x1: 1, y1: 3 }
  await wrapper!
    .get('[data-testid="fake-grid"] button:nth-child(5)')
    .trigger('click')
  await flushPromises()
  expect(
    fetch.mock.calls.filter(([, init]) => init?.method === 'PATCH'),
  ).toHaveLength(2)
  expect(wrapper!.text()).toContain('only works downward')
})

it.each(['Delete', 'Backspace'])(
  'clears a writable range with %s through ordinary bounded codecs',
  async (key) => {
    const schema = schemaFor('text')
    addField(schema, 'check', 'checkbox')
    gridState.rowCount = 2
    gridState.rowFields = [
      { name: 'first', check: true },
      { name: 'second', check: true },
    ]
    gridState.range = {
      x: 0,
      y: 0,
      x1: 1,
      y1: 1,
      rgCol: 'rgCol',
      rgRow: 'rgRow',
    }
    const fetch = await mountGrid(schema)
    await wrapper!
      .findAll('button')
      .find((button) => button.text() === 'focus')!
      .trigger('click')
    await wrapper!.get('.data-grid').trigger('keydown', { key })
    await flushPromises()
    const patch = fetch.mock.calls.find(([, init]) => init?.method === 'PATCH')
    expect(JSON.parse(String(patch?.[1]?.body))).toEqual({
      rows: [
        { id: '9007199254740993', fields: { name: null, check: false } },
        { id: '9007199254740994', fields: { name: null, check: false } },
      ],
    })
  },
)

it('skips read-only/system columns on clear and never writes for readers', async () => {
  const schema = schemaFor('text', 'read')
  const fetch = await mountGrid(schema)
  await wrapper!
    .findAll('button')
    .find((button) => button.text() === 'focus')!
    .trigger('click')
  await wrapper!.get('.data-grid').trigger('keydown', { key: 'Delete' })
  await flushPromises()
  expect(fetch.mock.calls.some(([, init]) => init?.method === 'PATCH')).toBe(
    false,
  )
  schema.myLevel = 'owner'
  schema.views[0]!.config = {
    fields: [
      { id: 'system-id', visible: true },
      { id: 'name', visible: true },
    ],
    frozen: 0,
  }
  await wrapper!.setProps({ schema: { ...schema } })
  await flushPromises()
  await wrapper!
    .findAll('button')
    .find((button) => button.text() === 'focus')!
    .trigger('click')
  gridState.range = {
    x: 0,
    y: 0,
    x1: 1,
    y1: 0,
    rgCol: 'rgCol',
    rgRow: 'rgRow',
  }
  await wrapper!.get('.data-grid').trigger('keydown', { key: 'Delete' })
  await flushPromises()
  const patch = fetch.mock.calls.find(([, init]) => init?.method === 'PATCH')
  expect(JSON.parse(String(patch?.[1]?.body)).rows[0].fields).toEqual({
    name: null,
  })
})

it('starts Enter/F2 editing but leaves native editor Escape and IME behavior untouched', async () => {
  await mountGrid(schemaFor('text'))
  await wrapper!
    .findAll('button')
    .find((button) => button.text() === 'focus')!
    .trigger('click')
  await wrapper!.get('.data-grid').trigger('keydown', { key: 'F2' })
  await wrapper!.get('.data-grid').trigger('keydown', { key: 'Enter' })
  expect(gridState.editCell).toHaveBeenNthCalledWith(1, 0, 'name')
  expect(gridState.editCell).toHaveBeenCalledTimes(2)
  await wrapper!.get('input').trigger('keydown', { key: 'F2' })
  await wrapper!
    .get('.data-grid')
    .trigger('keydown', { key: 'F2', isComposing: true })
  expect(gridState.editCell).toHaveBeenCalledTimes(2)
})

it('maps Home/End and Ctrl/Cmd+Home/End through loaded row identity, not viewport indexes', async () => {
  const schema = schemaFor('text')
  addField(schema, 'other', 'text')
  gridState.rowCount = 50
  const fetch = await mountGrid(schema)
  Object.assign(gridState.focus, {
    rowIndex: 3,
    colIndex: 0,
    model: { __rowId: String(BigInt('9007199254740993') + 45n) },
    column: { prop: 'name' },
  })
  await wrapper!
    .findAll('button')
    .find((button) => button.text() === 'focus')!
    .trigger('click')
  for (const [key, ctrlKey, metaKey, expectedRow, expectedX] of [
    ['Home', false, false, 45, 0],
    ['End', false, false, 45, 1],
    ['Home', true, false, 0, 0],
    ['End', false, true, 49, 1],
  ] as const) {
    await wrapper!
      .get('.data-grid')
      .trigger('keydown', { key, ctrlKey, metaKey })
    await flushPromises()
    expect(gridState.focusCells).toHaveBeenLastCalledWith(
      { x: expectedX, y: expectedRow },
      { x: expectedX, y: expectedRow },
      'rgCol',
    )
  }
  expect(fetch.mock.calls.every(([, init]) => init?.method !== 'PATCH')).toBe(
    true,
  )
})

it('pages by the selected row height', async () => {
  const schema = schemaFor('text')
  schema.views[0]!.config.rowHeight = 'medium'
  gridState.rowCount = 50
  await mountGrid(schema)
  Object.defineProperty(
    wrapper!.get('[data-testid="fake-grid"]').element,
    'clientHeight',
    {
      configurable: true,
      value: 512,
    },
  )
  Object.assign(gridState.focus, {
    rowIndex: 3,
    colIndex: 0,
    model: { __rowId: String(BigInt('9007199254740993') + 3n) },
    column: { prop: 'name' },
  })
  await wrapper!
    .findAll('button')
    .find((button) => button.text() === 'focus')!
    .trigger('click')
  await wrapper!.get('.data-grid').trigger('keydown', { key: 'PageDown' })
  await flushPromises()
  expect(gridState.focusCells).toHaveBeenLastCalledWith(
    { x: 0, y: 13 },
    { x: 0, y: 13 },
    'rgCol',
  )
})

it('coalesces resize/drag/hide/freeze into a column-only save with If-Match', async () => {
  vi.useFakeTimers()
  try {
    const schema = schemaFor('text')
    addField(schema, 'other', 'text')
    const fetch = await mountGrid(schema)
    const grid = wrapper!.findComponent({ name: 'RevoGrid' })
    grid.vm.$emit(
      'aftercolumnresize',
      new CustomEvent('aftercolumnresize', {
        detail: { 0: { prop: 'name', size: 260 } },
      }),
    )
    grid.vm.$emit(
      'columndragend',
      new CustomEvent('columndragend', {
        detail: { columns: [{ prop: 'other' }, { prop: 'name' }] },
      }),
    )
    await wrapper!.get('input[aria-label="Width of Name"]').setValue(275)
    const frozen = wrapper!.get('input[type="number"][min="0"]')
    ;(frozen.element as HTMLInputElement).value = '2'
    await frozen.trigger('input')
    grid.vm.$emit(
      'aftercolumnresize',
      new CustomEvent('aftercolumnresize', {
        detail: { 0: { prop: 'other', size: 220 } },
      }),
    )
    await flushPromises()
    expect((frozen.element as HTMLInputElement).value).toBe('2')
    await frozen.trigger('change')
    await vi.advanceTimersByTimeAsync(350)
    await flushPromises()
    const saves = fetch.mock.calls.filter(
      ([path]) => String(path) === '/api/v1/views/v',
    )
    expect(saves).toHaveLength(1)
    expect(saves[0]?.[1]?.headers).toMatchObject({ 'If-Match': '1' })
    const config = (
      JSON.parse(String(saves[0]?.[1]?.body)) as {
        config: TableSchema['views'][number]['config']
      }
    ).config
    expect(
      config.fields.filter((field) => field.visible).map((field) => field.id),
    ).toEqual(['other', 'name'])
    expect(config.fields.find((field) => field.id === 'name')?.width).toBe(275)
    expect(config.frozen).toBe(2)
  } finally {
    vi.useRealTimers()
  }
})
