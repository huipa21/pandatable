import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { markRaw, nextTick, watch, type Component } from 'vue'
import type {
  CellTemplateProp,
  ColumnRegular,
  Providers,
} from '@revolist/revogrid'
import { i18n, loadLocale } from '../../i18n'
import type {
  RelationLink,
  RowQueryRequest,
  TableSchema,
  ViewColumns,
} from '../../api/types'
import { useAuthStore } from '../../stores/auth'
import { useRealtimeStore, type RealtimeEvent } from '../../stores/realtime'
import { useUndoStore } from '../../stores/undo'
import { navUser, tableSchema } from '../../test/navigation'
import { ThemeMedia } from '../../test/setup'
import DataGrid from './DataGrid.vue'
import GridSearchPopover from './GridSearchPopover.vue'
import { formatTsv } from './tsv'

const gridState = vi.hoisted(() => ({
  fieldId: '',
  value: 'changed',
  rowIndex: 0,
  rowCount: 1,
  viewportHeight: 100,
  scrollHeight: 100,
  scrollTop: 0,
  visibleFieldIds: [] as string[],
  rowFields: [] as Record<string, unknown>[],
  footerFailuresRemaining: 0,
  range: { x: 0, y: 0, x1: 0, y1: 0, rgCol: 'rgCol', rgRow: 'rgRow' },
  fillRange: { x: 0, y: 0, x1: 0, y1: 0 },
  focus: { rowIndex: 0, colIndex: 0, colType: 'rgCol' },
  clipboard: new Map<string, string>(),
  editCell: vi.fn(),
  focusCells: vi.fn(),
  scrollRow: vi.fn(),
  scrollColumn: vi.fn(),
  editorComponent: undefined as Component | undefined,
  renderHeaders: false,
  relationCellProps: undefined as Record<string, unknown> | undefined,
}))

vi.mock('@revolist/vue3-datagrid', async () => {
  const vue = await import('vue')
  return {
    useTemplates: () => ({
      template: (
        component: { name?: string; __name?: string },
        options?: { props?: (context: unknown) => Record<string, unknown> },
      ) => {
        const name = component.name ?? component.__name
        if (name === 'GridColumnHeader')
          return (_render: unknown, context: unknown) =>
            vue.h(component as Component, options?.props?.(context))
        if (name === 'GridRelationCell')
          return (_render: unknown, context: unknown) => {
            gridState.relationCellProps = options?.props?.(context)
            return null
          }
        return () => null
      },
    }),
    default: vue.defineComponent({
      name: 'RevoGrid',
      props: ['source', 'columns', 'editors', 'readonly', 'useClipboard'],
      emits: [
        'created',
        'beforeedit',
        'beforecellfocus',
        'beforeeditstart',
        'viewportscroll',
        'aftergridrender',
        'afterrender',
        'contentsizechanged',
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
      setup(_props, { emit, slots }) {
        const root = vue.ref<HTMLElement>()
        const viewport = vue.ref<HTMLElement>()
        const currentFocus = () => ({
          model: _props.source[gridState.focus.rowIndex],
          cell: { x: gridState.focus.colIndex, y: gridState.focus.rowIndex },
          rowType: 'rgRow',
          column: _props.columns.filter(
            (column: { pin?: string }) =>
              (column.pin ?? 'rgCol') === gridState.focus.colType,
          )[gridState.focus.colIndex],
          ...gridState.focus,
        })
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
            Object.defineProperty(element, 'source', {
              get: () => _props.source,
            })
            Object.assign(element, {
              setCellEdit: gridState.editCell,
              setCellsFocus: gridState.focusCells,
              scrollToRow: gridState.scrollRow,
              scrollToColumnProp: gridState.scrollColumn,
              getFocused: async () => currentFocus(),
            })
          }
          if (viewport.value) {
            Object.defineProperties(viewport.value, {
              clientHeight: { get: () => gridState.viewportHeight },
              scrollHeight: { get: () => gridState.scrollHeight },
              scrollTop: { get: () => gridState.scrollTop },
            })
          }
          emit('created')
        })
        return () =>
          vue.h(
            'div',
            {
              ref: root,
              'data-testid': 'fake-grid',
              'aria-rowcount': _props.source.length,
              onMousedown: (originalEvent: MouseEvent) =>
                emit(
                  'beforecellfocus',
                  new CustomEvent('beforecellfocus', {
                    detail: {
                      prop: gridState.fieldId,
                      model: _props.source[gridState.focus.rowIndex],
                      rowIndex: gridState.focus.rowIndex,
                      originalEvent,
                    },
                    cancelable: true,
                  }),
                ),
            },
            [
              ...(gridState.renderHeaders
                ? [
                    vue.h(
                      'div',
                      { 'data-testid': 'grid-headers' },
                      _props.columns.map(
                        (column: {
                          prop: string
                          columnTemplate?: (
                            render: typeof vue.h,
                            context: unknown,
                          ) => ReturnType<typeof vue.h>
                        }) =>
                          column.columnTemplate?.(vue.h, column) ??
                          vue.h('span', { 'data-column': column.prop }, '#'),
                      ),
                    ),
                  ]
                : []),
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
                        detail: currentFocus(),
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
              vue.h('revogr-viewport-scroll', { class: 'rgCol' }, [
                vue.h('div', { ref: viewport, class: 'vertical-inner' }, [
                  vue.h(
                    'revogr-data',
                    {},
                    _props.source.length
                      ? [
                          vue.h('div', {
                            class: 'rgRow',
                            'data-rgrow': _props.source.length - 1,
                          }),
                        ]
                      : [],
                  ),
                ]),
              ]),
              vue.h('div', {}, [
                ...(slots.default?.() ?? []),
                ...(slots.footer?.() ?? []),
              ]),
            ],
          )
      },
    }),
    VGridVueEditor: (component: Component) => {
      gridState.editorComponent = component
      return () => ({ render: () => '' })
    },
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
  gridState.footerFailuresRemaining = 0
  gridState.renderHeaders = false
  gridState.relationCellProps = undefined
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
  gridState.viewportHeight = 100
  gridState.scrollHeight = 100
  gridState.scrollTop = 0
  gridState.visibleFieldIds = ['name']
  gridState.rowFields = []
  gridState.range = { x: 0, y: 0, x1: 0, y1: 0, rgCol: 'rgCol', rgRow: 'rgRow' }
  gridState.fillRange = { x: 0, y: 0, x1: 0, y1: 0 }
  gridState.focus = { rowIndex: 0, colIndex: 0, colType: 'rgCol' }
  gridState.clipboard.clear()
  gridState.editCell.mockReset()
  gridState.focusCells.mockReset()
  gridState.scrollRow.mockReset()
  gridState.scrollRow.mockResolvedValue(undefined)
  gridState.scrollColumn.mockReset()
  gridState.scrollColumn.mockResolvedValue(undefined)
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
  offset = 0,
  limit = gridState.rowCount,
) {
  return {
    rows: Array.from(
      { length: Math.max(0, Math.min(limit, gridState.rowCount - offset)) },
      (_, index) => ({
        id: String(BigInt('9007199254740993') + BigInt(offset + index)),
        fields: gridState.rowFields[offset + index] ?? {
          [schema.fields[0]!.id]: 'before',
        },
      }),
    ),
    fieldIds,
    total: gridState.rowCount,
    schemaVersion: schema.schemaVersion,
  }
}

async function mountGrid(
  schema: TableSchema,
  identity = navUser.id,
  routePath = '/',
  attachToDocument = false,
) {
  const fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input)
    const body =
      typeof init?.body === 'string'
        ? (JSON.parse(init.body) as {
            fieldIds?: string[]
            aggregates?: { fieldId: string; aggregate: string }[]
            offset?: number
            limit?: number
          })
        : undefined
    if (path.endsWith('/query'))
      return Response.json(
        queryResponse(
          schema,
          body?.fieldIds ?? gridState.visibleFieldIds,
          body?.offset,
          body?.limit,
        ),
      )
    if (path.endsWith('/groups'))
      return Response.json({
        groups: gridState.rowCount
          ? [{ key: null, count: gridState.rowCount, aggregates: [] }]
          : [],
        schemaVersion: schema.schemaVersion,
      })
    if (path.endsWith('/aggregates'))
      if (gridState.footerFailuresRemaining > 0) {
        gridState.footerFailuresRemaining--
        return Response.json({ code: 'ERR_INTERNAL' }, { status: 500 })
      }
    if (path.endsWith('/aggregates'))
      return Response.json({
        aggregates: (body?.aggregates ?? []).map((aggregate) => ({
          ...aggregate,
          value: aggregate.aggregate.startsWith('percent_')
            ? '0.25'
            : ['earliest', 'latest'].includes(aggregate.aggregate)
              ? '2026-11-06'
              : 42,
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
        const previous = requestBody.rows.map(({ id, fields }) => {
          const index = Number(BigInt(id) - BigInt('9007199254740993'))
          const current = gridState.rowFields[index] ?? {
            [schema.fields[0]!.id]: 'before',
          }
          const before = Object.fromEntries(
            Object.keys(fields).map((fieldId) => [
              fieldId,
              current[fieldId] ?? null,
            ]),
          )
          return { id, fields: before }
        })
        const patched = requestBody.rows.map(({ id, fields }) => {
          const index = Number(BigInt(id) - BigInt('9007199254740993'))
          const next = {
            ...(gridState.rowFields[index] ?? {
              [schema.fields[0]!.id]: 'before',
            }),
            ...fields,
          }
          gridState.rowFields[index] = next
          return { id, fields: next }
        })
        return Response.json({ eventId: 'event', rows: patched, previous })
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
    if (init?.method === 'DELETE') {
      const requestBody = JSON.parse(String(init.body)) as { ids: string[] }
      const rows = requestBody.ids.map((id) => {
        const index = Number(BigInt(id) - BigInt('9007199254740993'))
        return {
          id,
          fields: gridState.rowFields[index] ?? {
            [schema.fields[0]!.id]: 'before',
          },
          restoreVersion: 'restore-version',
        }
      })
      gridState.rowCount -= rows.length
      return Response.json({ eventId: 'event', rows })
    }
    throw new Error(`Unexpected API request: ${path}`)
  })
  vi.stubGlobal('fetch', fetch)
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: DataGrid },
      { path: '/t/:tableId', component: DataGrid },
      { path: '/t/:tableId/v/:viewId', component: DataGrid },
    ],
  })
  await router.push(routePath)
  wrapper = mount(DataGrid, {
    props: { schema, viewId: 'v', identity },
    global: { plugins: [pinia, i18n, router] },
    attachTo: attachToDocument ? document.body : undefined,
  })
  await flushPromises()
  return fetch
}

async function openFooterPicker(fieldId: string) {
  await wrapper!
    .get(`.grid-summary-cell[data-field-id="${fieldId}"] button`)
    .trigger('click')
  return wrapper!.get('.grid-summary-menu')
}

async function chooseFooterAggregate(fieldId: string, aggregate: string) {
  const menu = await openFooterPicker(fieldId)
  await menu
    .get(`[role="option"][data-aggregate="${aggregate}"]`)
    .trigger('click')
  await flushPromises()
}

async function requestCellEdit() {
  const grid = wrapper!.getComponent({ name: 'RevoGrid' })
  const source = grid.props('source') as { __rowId: string }[]
  const model = source[gridState.focus.rowIndex]
  if (!model) throw new Error('Missing focused grid row')
  const event = new CustomEvent('beforeeditstart', {
    detail: {
      prop: gridState.fieldId,
      model,
      rowIndex: gridState.focus.rowIndex,
    },
    cancelable: true,
  })
  grid.vm.$emit('beforeeditstart', event)
  return event.defaultPrevented
}

async function selectRow(id: string) {
  const columns = wrapper!
    .getComponent({ name: 'RevoGrid' })
    .props('columns') as ColumnRegular[]
  const column = columns.find((item) => item.prop === '__selected')
  if (!column?.cellTemplate) throw new Error('Missing row selection column')
  let change: ((event: Event) => void) | undefined
  const render = ((tag: unknown, data?: unknown) => {
    if (
      tag === 'input' &&
      typeof data === 'object' &&
      data !== null &&
      'onChange' in data &&
      typeof data.onChange === 'function'
    )
      change = data.onChange as (event: Event) => void
    // The test invokes the captured handler and never mounts the Stencil VNode.
    return null as never
  }) as Parameters<NonNullable<ColumnRegular['cellTemplate']>>[0]
  const model = { __rowId: id, __rowNumber: 1 }
  column.cellTemplate(render, {
    prop: '__selected',
    model,
    column,
    rowIndex: 0,
    colIndex: 0,
    colType: 'colPinStart',
    type: 'rgRow',
    data: [model],
    providers: {} as Providers,
  } as CellTemplateProp)
  if (!change) return false
  const event = new Event('change')
  Object.defineProperty(event, 'target', { value: { checked: true } })
  change(event)
  await nextTick()
  return true
}

function captureRelationCell() {
  const grid = wrapper!.getComponent({ name: 'RevoGrid' })
  const columns = grid.props('columns') as ColumnRegular[]
  const column = columns.find((item) => item.prop === 'linked')
  const model = (grid.props('source') as Record<string, unknown>[])[0]
  if (!column?.cellTemplate || !model) throw new Error('Missing relation cell')
  const render = (() => null as never) as Parameters<
    NonNullable<ColumnRegular['cellTemplate']>
  >[0]
  column.cellTemplate(render, {
    prop: 'linked',
    value: model.linked,
    model,
    column,
    rowIndex: 0,
    colIndex: 1,
    colType: 'rgCol',
    type: 'rgRow',
    data: [model],
    providers: {} as Providers,
  } as CellTemplateProp)
  const props = gridState.relationCellProps
  if (
    !props ||
    typeof props.remove !== 'function' ||
    typeof props.disabled !== 'function'
  )
    throw new Error('Missing relation cell callbacks')
  const remove = props.remove
  const disabled = props.disabled
  return {
    editable: props.editable,
    disabled: () => Boolean(disabled()),
    remove: (link: RelationLink) => Promise.resolve(remove(link)),
  }
}

function gridRelationSchema(level: TableSchema['myLevel'] = 'owner') {
  const schema = schemaFor('text', level)
  addField(schema, 'linked', 'link')
  gridState.visibleFieldIds = ['name', 'linked']
  gridState.rowFields = [
    {
      name: 'before',
      linked: [
        { id: '2', label: 'Related row' },
        { id: '3', label: 'Another row' },
      ],
    },
  ]
  return schema
}

it('unlinks a grid tag, disables duplicate actions, and records the inverse', async () => {
  const schema = gridRelationSchema()
  const fetcher = await mountGrid(schema)
  let release!: (response: Response) => void
  fetcher.mockImplementationOnce(
    () =>
      new Promise<Response>((resolve) => {
        release = resolve
      }),
  )
  const cell = captureRelationCell()
  const pending = cell.remove({ id: '2', label: 'Related row' })
  await flushPromises()
  expect(cell.disabled()).toBe(true)
  const mutation = fetcher.mock.calls.find(([url]) =>
    String(url).endsWith('/links/linked'),
  )
  expect(mutation?.[0]).toBe(
    '/api/v1/tables/t/rows/9007199254740993/links/linked',
  )
  expect(JSON.parse(String(mutation?.[1]?.body))).toEqual({
    add: [],
    remove: ['2'],
  })
  release(
    Response.json({
      row: {
        id: '9007199254740993',
        fields: { name: 'before', linked: [{ id: '3', label: 'Another row' }] },
      },
      changes: [
        {
          tableId: 't',
          rowId: '9007199254740993',
          fieldId: 'linked',
          targetId: '2',
          action: 'unlink',
        },
      ],
    }),
  )
  await pending
  await flushPromises()

  const source = wrapper!.getComponent({ name: 'RevoGrid' }).props('source') as
    Record<string, unknown>[] | undefined
  expect(source?.[0]?.linked).toEqual([{ id: '3', label: 'Another row' }])
  expect(cell.disabled()).toBe(false)
  expect(useUndoStore().undoStack[0]?.steps).toEqual([
    {
      kind: 'link',
      tableId: 't',
      rowId: '9007199254740993',
      fieldId: 'linked',
      add: ['2'],
      remove: [],
    },
  ])
})

it('reports grid unlink failures and hides relation actions from readers', async () => {
  const schema = gridRelationSchema()
  const fetcher = await mountGrid(schema)
  fetcher.mockImplementationOnce(async () =>
    Response.json({ error: { code: 'ERR_INTERNAL' } }, { status: 500 }),
  )
  const cell = captureRelationCell()
  await cell.remove({ id: '2', label: 'Related row' })
  await flushPromises()
  expect(wrapper!.get('[role="alert"]').text()).toContain(
    i18n.global.t('errors.ERR_INTERNAL'),
  )
  expect(useUndoStore().undoStack).toHaveLength(0)
  expect(cell.disabled()).toBe(false)
  await wrapper!.setProps({ schema: { ...schema, myLevel: 'read' } })
  expect(captureRelationCell().editable).toBe(false)
})

it('allows editing when the route resolves its default view', async () => {
  const schema = schemaFor('text')
  await mountGrid(schema, navUser.id, `/t/${schema.table.id}`)

  expect(await requestCellEdit()).toBe(false)

  expect(gridState.editCell).not.toHaveBeenCalled()
})

it('rejects editing when the route names a different view', async () => {
  const schema = schemaFor('text')
  await mountGrid(schema, navUser.id, `/t/${schema.table.id}/v/other`)

  expect(await requestCellEdit()).toBe(true)
})

it('rejects editing after the schema version changes', async () => {
  const schema = schemaFor('text')
  await mountGrid(schema, navUser.id, `/t/${schema.table.id}`)
  await wrapper!.setProps({
    schema: { ...schema, schemaVersion: schema.schemaVersion + 1 },
  })

  expect(await requestCellEdit()).toBe(true)
})

it('shows visible selected-row actions only to writers', async () => {
  const schema = schemaFor('text')
  await mountGrid(schema)
  const id = '9007199254740993'
  expect(await selectRow(id)).toBe(true)
  expect(wrapper!.get('.grid-row-actions').text()).toContain('1 selected')
  expect(wrapper!.get('.grid-row-actions').text()).toContain('Edit selected')
  expect(wrapper!.get('.grid-row-actions').text()).toContain('Delete selected')

  await wrapper!.setProps({ schema: { ...schema, myLevel: 'read' } })
  expect(wrapper!.find('.grid-row-actions').exists()).toBe(false)
  wrapper!.unmount()
  wrapper = undefined
  await mountGrid(schemaFor('text', 'read'))
  expect(await selectRow(id)).toBe(false)
  expect(wrapper!.find('.grid-row-actions').exists()).toBe(false)
})

it('selects all matching rows across pages and clears them from the header', async () => {
  gridState.renderHeaders = true
  gridState.rowCount = 251
  const fetch = await mountGrid(schemaFor('text'))
  await selectRow('9007199254740993')
  expect(
    (wrapper!.get('.grid-select-all').element as HTMLInputElement)
      .indeterminate,
  ).toBe(true)
  fetch.mockClear()
  await wrapper!.get('.grid-select-all').setValue(true)
  await flushPromises()
  expect(wrapper!.get('.grid-row-actions').text()).toContain('251 selected')
  expect(
    (wrapper!.get('.grid-select-all').element as HTMLInputElement).checked,
  ).toBe(true)
  expect(
    fetch.mock.calls
      .filter(([path]) => String(path).endsWith('/query'))
      .map(([, init]) => JSON.parse(String(init?.body)).offset),
  ).toEqual([0, 100, 200])
  await wrapper!.get('.grid-select-all').setValue(false)
  expect(wrapper!.find('.grid-row-actions').exists()).toBe(false)
})

it('retains the selection and reports the existing limit for select all', async () => {
  gridState.renderHeaders = true
  gridState.rowCount = 1001
  await mountGrid(schemaFor('text'))
  await selectRow('9007199254740993')
  await wrapper!.get('.grid-select-all').setValue(true)
  await flushPromises()
  expect(wrapper!.get('.grid-row-actions').text()).toContain('1 selected')
  expect(wrapper!.get('[role="alert"]').text()).toContain(
    'Select no more than 1,000 rows',
  )
})

it('allows select all at the 1,000-row boundary', async () => {
  gridState.renderHeaders = true
  gridState.rowCount = 1000
  await mountGrid(schemaFor('text'))
  await wrapper!.get('.grid-select-all').setValue(true)
  await flushPromises()
  expect(wrapper!.get('.grid-row-actions').text()).toContain('1000 selected')
  expect(wrapper!.find('[role="alert"]').exists()).toBe(false)
})

it('reports a select-all query failure without changing the existing selection', async () => {
  gridState.renderHeaders = true
  gridState.rowCount = 2
  const fetch = await mountGrid(schemaFor('text'))
  await selectRow('9007199254740993')
  fetch.mockResolvedValueOnce(
    Response.json({ code: 'ERR_INTERNAL' }, { status: 500 }),
  )
  await wrapper!.get('.grid-select-all').setValue(true)
  await flushPromises()
  expect(wrapper!.get('.grid-row-actions').text()).toContain('1 selected')
  expect(wrapper!.get('[role="alert"]').text()).not.toBe('')
  expect(
    (wrapper!.get('.grid-select-all').element as HTMLInputElement).disabled,
  ).toBe(false)
})

it('does not render select all for readers', async () => {
  gridState.renderHeaders = true
  await mountGrid(schemaFor('text', 'read'))
  expect(wrapper!.find('.grid-select-all').exists()).toBe(false)
  expect(wrapper!.get('.grid-selection-header').text()).toBe('#')
})

it('checks the header when every row is selected individually', async () => {
  gridState.renderHeaders = true
  gridState.rowCount = 2
  await mountGrid(schemaFor('text'))
  await selectRow('9007199254740993')
  expect(
    (wrapper!.get('.grid-select-all').element as HTMLInputElement)
      .indeterminate,
  ).toBe(true)
  await selectRow('9007199254740994')
  expect(
    (wrapper!.get('.grid-select-all').element as HTMLInputElement).checked,
  ).toBe(true)
  await wrapper!.get('.grid-select-all').setValue(false)
  expect(wrapper!.find('.grid-row-actions').exists()).toBe(false)
})

it('highlights only the hovered row across pinned and scrolling regions', async () => {
  gridState.rowCount = 2
  await mountGrid(schemaFor('text'))
  const grid = wrapper!.get('[data-testid="fake-grid"]')
  grid.element
    .querySelectorAll('revogr-data')
    .forEach((region) => region.remove())
  const regions = [
    document.createElement('revogr-data'),
    document.createElement('revogr-data'),
  ]
  for (const region of regions) {
    region.innerHTML =
      '<div class="rgRow" data-rgrow="0"><span>First</span></div>' +
      '<div class="rgRow" data-rgrow="1"><span>Second</span></div>'
    grid.element.append(region)
  }
  await grid.get('[data-rgrow="0"] span').trigger('mouseover')
  expect(grid.findAll('.grid-row-hover')).toHaveLength(2)
  expect(grid.findAll('[data-rgrow="1"].grid-row-hover')).toHaveLength(0)
  await grid.get('[data-rgrow="1"] span').trigger('mouseover')
  expect(grid.findAll('[data-rgrow="0"].grid-row-hover')).toHaveLength(0)
  expect(grid.findAll('[data-rgrow="1"].grid-row-hover')).toHaveLength(2)
  await grid.trigger('mouseleave')
  expect(grid.findAll('.grid-row-hover')).toHaveLength(0)
})

it.each([
  { outcome: 'succeeds', rejected: false },
  { outcome: 'fails and the draft is discarded', rejected: true },
])(
  're-enables selected-row actions when a cell save $outcome',
  async ({ rejected }) => {
    const fetch = await mountGrid(schemaFor('text'))
    const original = fetch.getMockImplementation()!
    let release: ((response: Response) => void) | undefined
    fetch.mockImplementation((input, init) => {
      if (init?.method !== 'PATCH') return original(input, init)
      return new Promise<Response>((resolve) => {
        release = resolve
      })
    })
    const openEditor = () =>
      mount(gridState.editorComponent!, {
        props: {
          column: { prop: 'name' },
          prop: 'name',
          rowIndex: 0,
          val: 'before',
          save: (value: string) => {
            gridState.value = value
            ;(
              wrapper!.get('[data-testid="fake-grid"] button:first-child')
                .element as HTMLButtonElement
            ).click()
          },
          close: vi.fn(),
        },
        global: { plugins: [pinia, i18n] },
      })
    const editor = openEditor()
    await editor.get('input').setValue('saved')
    await editor
      .get('input')
      .trigger('focusout', { relatedTarget: document.body })
    await flushPromises()
    editor.unmount()
    expect(release).toBeDefined()
    expect(await selectRow('9007199254740993')).toBe(true)
    expect(
      wrapper!
        .get('.grid-row-actions')
        .findAll('button')
        .every((button) => button.attributes('disabled') !== undefined),
    ).toBe(true)
    expect(wrapper!.text()).toContain(i18n.global.t('grid.pendingCellDrafts'))

    release!(
      rejected
        ? Response.json(
            { error: { code: 'ERR_UNIQUE_CONFLICT' } },
            { status: 409 },
          )
        : Response.json({
            row: { id: '9007199254740993', fields: { name: 'saved' } },
          }),
    )
    await flushPromises()
    if (rejected) {
      expect(
        wrapper!
          .get('.grid-row-actions')
          .findAll('button')
          .every((button) => button.attributes('disabled') !== undefined),
      ).toBe(true)
      expect(wrapper!.text()).toContain(
        i18n.global.t('errors.ERR_UNIQUE_CONFLICT'),
      )
      const reopened = openEditor()
      expect((reopened.get('input').element as HTMLInputElement).value).toBe(
        'saved',
      )
      await reopened.get('input').trigger('keydown', { key: 'Escape' })
      reopened.unmount()
      await flushPromises()
    }
    expect(
      wrapper!
        .get('.grid-row-actions')
        .findAll('button')
        .every((button) => button.attributes('disabled') === undefined),
    ).toBe(true)
    expect(wrapper!.text()).not.toContain(
      i18n.global.t('grid.pendingCellDrafts'),
    )
  },
)

it('omits system, relation, and attachment fields from the bulk editor', async () => {
  const schema = schemaFor('text')
  addField(schema, 'relation', 'link')
  addField(schema, 'uploads', 'attachment', { accept: '', maxFiles: 10 })
  await mountGrid(schema)
  expect(await selectRow('9007199254740993')).toBe(true)
  await wrapper!.get('.grid-row-actions button').trigger('click')

  const options = wrapper!
    .get('dialog[aria-label="Edit selected rows"] select')
    .findAll('option')
    .map((option) => option.attributes('value'))
  expect(options).toEqual(['', 'name'])
  expect(wrapper!.text()).toContain(
    'Relation and attachment fields are not available for bulk editing.',
  )
})

it('keeps the bulk temporal picker in its dialog and removes it when switching fields', async () => {
  const schema = schemaFor('date')
  addField(schema, 'text', 'text')
  await mountGrid(schema, navUser.id, '/', true)
  expect(await selectRow('9007199254740993')).toBe(true)
  await wrapper!.get('.grid-row-actions button').trigger('click')

  const dialog = wrapper!.get('dialog[aria-label="Edit selected rows"]')
  const field = dialog.get('select')
  await field.setValue('name')
  await flushPromises()
  expect(dialog.find('.temporal-picker-portal').exists()).toBe(true)
  await field.setValue('text')
  await flushPromises()
  expect(dialog.find('.temporal-picker-portal').exists()).toBe(false)
  expect(dialog.find('.field-editor input').exists()).toBe(true)
})

it('deletes only selected row IDs across pages and retains selection on cancel', async () => {
  gridState.rowCount = 205
  const fetch = await mountGrid(schemaFor('text'))
  const first = '9007199254740994'
  const later = String(BigInt('9007199254740993') + 150n)
  expect(await selectRow(first)).toBe(true)
  expect(await selectRow(later)).toBe(true)

  await wrapper!.get('.grid-row-actions button:last-child').trigger('click')
  const dialog = wrapper!.get('dialog[aria-label="Delete selected rows"]')
  await dialog.get('.grid-dialog-actions button:first-child').trigger('click')
  expect(wrapper!.get('.grid-row-actions').text()).toContain('2 selected')
  expect(
    fetch.mock.calls.filter(([, init]) => init?.method === 'DELETE'),
  ).toHaveLength(0)

  await wrapper!.get('.grid-row-actions button:last-child').trigger('click')
  await wrapper!
    .get(
      'dialog[aria-label="Delete selected rows"] .grid-dialog-actions button:last-child',
    )
    .trigger('click')
  await flushPromises()
  const deletion = fetch.mock.calls.find(
    ([, init]) => init?.method === 'DELETE',
  )
  expect(deletion).toBeDefined()
  expect(JSON.parse(String(deletion?.[1]?.body))).toEqual({
    ids: [first, later],
  })
  expect(useUndoStore().undoStack.at(-1)?.steps).toEqual([
    {
      kind: 'restore',
      tableId: 't',
      rows: expect.arrayContaining([
        expect.objectContaining({ id: first }),
        expect.objectContaining({ id: later }),
      ]),
    },
  ])
  expect(wrapper!.find('.grid-row-actions').exists()).toBe(false)
})

it('previews and atomically edits selected IDs with a typed value and guarded undo', async () => {
  gridState.rowCount = 205
  const schema = schemaFor('text')
  const fetch = await mountGrid(schema)
  const first = '9007199254740994'
  const later = String(BigInt('9007199254740993') + 150n)
  expect(await selectRow(first)).toBe(true)
  expect(await selectRow(later)).toBe(true)

  await wrapper!.get('.grid-row-actions button').trigger('click')
  let dialog = wrapper!.get('dialog[aria-label="Edit selected rows"]')
  await dialog.get('select').setValue(schema.fields[0]!.id)
  await dialog.get('input[aria-label="Field value"]').setValue('shared value')
  await dialog.get('button').trigger('click')
  expect(dialog.text()).toContain(
    'Set Name to “shared value” for 2 selected rows?',
  )
  expect(
    fetch.mock.calls.filter(([, init]) => init?.method === 'PATCH'),
  ).toHaveLength(0)
  await dialog.get('.grid-dialog-actions button:first-child').trigger('click')
  expect(wrapper!.get('.grid-row-actions').text()).toContain('2 selected')
  expect(
    fetch.mock.calls.filter(([, init]) => init?.method === 'PATCH'),
  ).toHaveLength(0)

  await wrapper!.get('.grid-row-actions button').trigger('click')
  dialog = wrapper!.get('dialog[aria-label="Edit selected rows"]')
  await dialog.get('select').setValue(schema.fields[0]!.id)
  await dialog.get('input[aria-label="Field value"]').setValue('shared value')
  await dialog.get('button').trigger('click')

  await dialog.get('.grid-dialog-actions button:last-child').trigger('click')
  await flushPromises()
  const patch = fetch.mock.calls.find(
    ([path, init]) =>
      String(path) === '/api/v1/tables/t/rows' && init?.method === 'PATCH',
  )
  expect(patch).toBeDefined()
  expect(JSON.parse(String(patch?.[1]?.body))).toEqual({
    rows: [
      { id: first, fields: { [schema.fields[0]!.id]: 'shared value' } },
      { id: later, fields: { [schema.fields[0]!.id]: 'shared value' } },
    ],
  })
  expect(useUndoStore().undoStack.at(-1)?.steps).toEqual([
    {
      kind: 'patch',
      tableId: 't',
      rows: [
        {
          id: first,
          fields: { [schema.fields[0]!.id]: 'before' },
          expected: { [schema.fields[0]!.id]: 'shared value' },
        },
        {
          id: later,
          fields: { [schema.fields[0]!.id]: 'before' },
          expected: { [schema.fields[0]!.id]: 'shared value' },
        },
      ],
    },
  ])
  expect(wrapper!.find('.grid-row-actions').exists()).toBe(false)
})

it('retains the selection and reports atomic bulk edit failures', async () => {
  const schema = schemaFor('text')
  const fetch = await mountGrid(schema)
  const fallback = fetch.getMockImplementation()
  if (!fallback) throw new Error('Missing fetch implementation')
  fetch.mockImplementation((input, init) => {
    const requestBody =
      typeof init?.body === 'string'
        ? (JSON.parse(init.body) as { rows?: unknown[] })
        : undefined
    if (
      String(input) === '/api/v1/tables/t/rows' &&
      init?.method === 'PATCH' &&
      requestBody?.rows
    )
      return Promise.resolve(
        Response.json(
          {
            error: {
              code: 'ERR_UNIQUE_CONFLICT',
              message: 'A unique value already exists.',
            },
          },
          { status: 409 },
        ),
      )
    return fallback(input, init)
  })
  const first = '9007199254740994'
  const later = String(BigInt('9007199254740993') + 150n)
  expect(await selectRow(first)).toBe(true)
  expect(await selectRow(later)).toBe(true)
  await wrapper!.get('.grid-row-actions button').trigger('click')
  const dialog = wrapper!.get('dialog[aria-label="Edit selected rows"]')
  await dialog.get('select').setValue(schema.fields[0]!.id)
  await dialog.get('input[aria-label="Field value"]').setValue('duplicate')
  await dialog.get('button').trigger('click')
  await dialog.get('.grid-dialog-actions button:last-child').trigger('click')
  await flushPromises()

  expect(dialog.get('[role="alert"]').text()).toContain(
    i18n.global.t('errors.ERR_UNIQUE_CONFLICT'),
  )
  expect(wrapper!.get('.grid-row-actions').text()).toContain('2 selected')
  expect(useUndoStore().undoStack).toHaveLength(0)
})

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

async function waitForAppendVisibility(visible: boolean) {
  await vi.waitFor(() =>
    expect(wrapper!.get('.grid-add-row').attributes('aria-hidden')).toBe(
      String(!visible),
    ),
  )
}

it('header menu sorts the view and hides a column and its summary without editing or resizing', async () => {
  const schema = schemaFor('text')
  addField(schema, 'other', 'text')
  gridState.visibleFieldIds = ['name', 'other']
  gridState.renderHeaders = true
  const fetch = await mountGrid(markRaw(schema), navUser.id, '/', true)
  const header = wrapper!.get('[aria-label="Column menu for Name"]')
  const grid = wrapper!.get('[data-testid="fake-grid"]')
  const mouseDown = vi.fn()
  grid.element.addEventListener('mousedown', mouseDown)
  await header.trigger('pointerdown')
  await header.trigger('mousedown')
  expect(mouseDown).not.toHaveBeenCalled()
  expect(gridState.editCell).not.toHaveBeenCalled()
  expect(
    wrapper!.getComponent({ name: 'RevoGrid' }).props('columns'),
  ).toHaveLength(3)

  await header.trigger('click')
  document.querySelector<HTMLButtonElement>('[data-action="sort-asc"]')!.click()
  await flushPromises()
  const saved = await wrapper!.vm.$.exposed?.flushColumnChanges()
  await nextTick()
  expect(
    saved,
    wrapper!
      .findAll('[role="alert"]')
      .map((item) => item.text())
      .join('; '),
  ).toBe(true)
  const save = fetch.mock.calls.find(
    ([url, init]) =>
      String(url).endsWith('/views/v') && init?.method === 'PATCH',
  )
  expect(save).toBeDefined()
  expect(JSON.parse(String(save![1]?.body)).config.sort).toEqual([
    { field: 'name', dir: 'asc' },
  ])
  expect(wrapper!.vm.$.exposed?.exportQuery().sorts).toEqual([
    { fieldId: 'name', direction: 'asc' },
  ])

  expect(
    wrapper!.find('.grid-summary-cell[data-field-id="other"]').exists(),
  ).toBe(true)
  await wrapper!.get('[aria-label="Column menu for other"]').trigger('click')
  expect(
    document.querySelector('.column-header-menu')?.getAttribute('aria-label'),
  ).toBe('Column menu for other')
  const hide = document.querySelector<HTMLButtonElement>(
    '[data-action="hide"]',
  )!
  expect(hide.disabled).toBe(false)
  hide.click()
  await flushPromises()
  const config = wrapper!
    .emitted('viewConfigChanged')
    ?.at(-1)?.[0] as ViewColumns
  expect(config.fields.find((field) => field.id === 'other')?.visible).toBe(
    false,
  )
  expect(wrapper!.find('[aria-label="Column menu for other"]').exists()).toBe(
    false,
  )
  expect(
    wrapper!.find('.grid-summary-cell[data-field-id="other"]').exists(),
  ).toBe(false)
  expect(gridState.editCell).not.toHaveBeenCalled()
})

it('header menu groups locally for readers and sends the grouping to bucket and row queries', async () => {
  const schema = schemaFor('text', 'read')
  gridState.renderHeaders = true
  const key = `pandatable:groups:${navUser.id}:v:name`
  localStorage.setItem(key, JSON.stringify(['[null]']))
  try {
    const fetch = await mountGrid(schema, navUser.id, '/', true)
    await wrapper!.get('[aria-label="Column menu for Name"]').trigger('click')
    document.querySelector<HTMLButtonElement>('[data-action="group"]')!.click()
    await flushPromises()
    const requests = fetch.mock.calls
      .filter(([url]) => /\/(groups|query)$/.test(String(url)))
      .map(([url, init]) => ({
        url: String(url),
        body: JSON.parse(String(init?.body)) as {
          groupBy?: string[]
          groupPath?: unknown[]
        },
      }))
    expect(
      requests.some(
        ({ url, body }) =>
          url.endsWith('/groups') && body.groupBy?.[0] === 'name',
      ),
    ).toBe(true)
    expect(
      requests.some(
        ({ url, body }) =>
          url.endsWith('/query') &&
          body.groupBy?.[0] === 'name' &&
          body.groupPath?.length === 1,
      ),
    ).toBe(true)
    expect(fetch.mock.calls.some(([, init]) => init?.method === 'PATCH')).toBe(
      false,
    )
  } finally {
    localStorage.removeItem(key)
  }
})

it('creates one blank row, disables repeats, and records the authoritative undo', async () => {
  gridState.rowCount = 0
  const fetch = await mountGrid(schemaFor('text'))
  await waitForAppendVisibility(true)
  const fallback = fetch.getMockImplementation()
  if (!fallback) throw new Error('Missing fetch implementation')
  let finishCreate: ((response: Response) => void) | undefined
  fetch.mockImplementation((input, init) => {
    if (String(input).endsWith('/tables/t/rows') && init?.method === 'POST')
      return new Promise<Response>((resolve) => {
        finishCreate = resolve
      })
    return fallback(input, init)
  })

  const button = wrapper!.get<HTMLButtonElement>('.grid-add-row')
  await button.trigger('click')
  expect(button.element.disabled).toBe(true)
  button.element.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  const creates = fetch.mock.calls.filter(
    ([path, init]) =>
      String(path).endsWith('/tables/t/rows') && init?.method === 'POST',
  )
  expect(creates).toHaveLength(1)
  expect(JSON.parse(String(creates[0]![1]?.body))).toEqual({
    rows: [{ fields: {} }],
  })
  expect(
    (wrapper!.getComponent({ name: 'RevoGrid' }).props('source') as unknown[])
      .length,
  ).toBe(0)

  gridState.rowCount = 1
  if (!finishCreate) throw new Error('Create request was not started')
  finishCreate(
    Response.json({
      eventId: 'event',
      rows: [{ id: '9007199254740993', fields: { name: 'server default' } }],
    }),
  )
  await flushPromises()
  expect(useUndoStore().undoStack.at(-1)?.steps).toEqual([
    { kind: 'delete', tableId: 't', ids: ['9007199254740993'] },
  ])
  expect(
    (wrapper!.getComponent({ name: 'RevoGrid' }).props('source') as unknown[])
      .length,
  ).toBe(1)
  expect(button.element.disabled).toBe(false)
  await vi.waitFor(() => expect(gridState.scrollRow).toHaveBeenCalledWith(0))
})

it('keeps the append action hidden until the logical end of a paged result', async () => {
  gridState.rowCount = 205
  gridState.scrollHeight = 500
  const fetch = await mountGrid(schemaFor('text'))
  await waitForAppendVisibility(false)
  const grid = wrapper!.getComponent({ name: 'RevoGrid' })
  expect((grid.props('source') as unknown[]).length).toBe(205)
  await wrapper!.get('.grid-add-row').trigger('click')
  expect(
    fetch.mock.calls.filter(
      ([path, init]) =>
        String(path).endsWith('/tables/t/rows') && init?.method === 'POST',
    ),
  ).toHaveLength(0)

  gridState.scrollTop = 400
  grid.vm.$emit(
    'viewportscroll',
    new CustomEvent('viewportscroll', {
      detail: { dimension: 'rgRow', coordinate: 204 * 32 },
    }),
  )
  await flushPromises()
  await waitForAppendVisibility(true)
  expect((grid.props('source') as unknown[]).length).toBe(205)
  const queries = fetch.mock.calls.filter(([path]) =>
    String(path).endsWith('/query'),
  )
  expect(
    queries.some(([, init]) => JSON.parse(String(init?.body)).offset === 200),
  ).toBe(true)
})

it('preserves the appended page when the grid resets its viewport during requery', async () => {
  gridState.rowCount = 450
  gridState.scrollHeight = 500
  const fetch = await mountGrid(schemaFor('text'))
  const grid = wrapper!.getComponent({ name: 'RevoGrid' })
  gridState.scrollTop = 400
  grid.vm.$emit(
    'viewportscroll',
    new CustomEvent('viewportscroll', {
      detail: { dimension: 'rgRow', coordinate: 449 * 32 },
    }),
  )
  await flushPromises()
  await waitForAppendVisibility(true)

  const stop = watch(
    () => (grid.props('source') as unknown[]).length,
    (count) => {
      if (count !== 451) return
      gridState.scrollTop = 0
      grid.vm.$emit(
        'viewportscroll',
        new CustomEvent('viewportscroll', {
          detail: { dimension: 'rgRow', coordinate: 0 },
        }),
      )
    },
    { flush: 'sync' },
  )
  try {
    await wrapper!.get('.grid-add-row').trigger('click')
    await flushPromises()
    await vi.waitFor(() =>
      expect(gridState.scrollRow).toHaveBeenCalledWith(450),
    )
    expect(
      fetch.mock.calls.filter(
        ([path, init]) =>
          String(path).endsWith('/query') &&
          (JSON.parse(String(init?.body)) as { offset: number }).offset === 0,
      ),
    ).toHaveLength(1)
  } finally {
    stop()
  }
})

it('keeps empty virtual slots while an append requery is pending', async () => {
  gridState.rowCount = 450
  gridState.scrollHeight = 500
  const schema = schemaFor('text')
  const fetch = await mountGrid(schema)
  const grid = wrapper!.getComponent({ name: 'RevoGrid' })
  gridState.scrollTop = 400
  grid.vm.$emit(
    'viewportscroll',
    new CustomEvent('viewportscroll', {
      detail: { dimension: 'rgRow', coordinate: 449 * 32 },
    }),
  )
  await flushPromises()
  await waitForAppendVisibility(true)

  const fallback = fetch.getMockImplementation()
  if (!fallback) throw new Error('Missing fetch implementation')
  let finishQuery: ((response: Response) => void) | undefined
  fetch.mockImplementation((input, init) => {
    if (String(input).endsWith('/query')) {
      const body = JSON.parse(String(init?.body)) as {
        offset: number
        includeCount: boolean
      }
      if (body.offset === 400 && body.includeCount)
        return new Promise<Response>((resolve) => {
          finishQuery = resolve
        })
    }
    return fallback(input, init)
  })
  await wrapper!.get('.grid-add-row').trigger('click')
  await vi.waitFor(() => expect(finishQuery).toBeDefined())
  await flushPromises()
  const source = grid.props('source') as { __rowId?: string }[]
  expect(source).toHaveLength(450)
  expect(source.every((row) => !row.__rowId)).toBe(true)
  await waitForAppendVisibility(false)

  if (!finishQuery) throw new Error('Requery was not started')
  finishQuery(Response.json(queryResponse(schema, ['name'], 400, 100)))
  await flushPromises()
  await vi.waitFor(() => expect(gridState.scrollRow).toHaveBeenCalledWith(450))
})

it('retains the appended-row reveal through a deferred realtime reload', async () => {
  let receive: ((event: RealtimeEvent) => void) | undefined
  vi.spyOn(useRealtimeStore(), 'subscribe').mockImplementation(
    (_table, listener) => {
      receive = listener
      return () => {}
    },
  )
  gridState.rowCount = 450
  gridState.scrollHeight = 500
  const schema = schemaFor('text')
  const fetch = await mountGrid(schema)
  const grid = wrapper!.getComponent({ name: 'RevoGrid' })
  gridState.scrollTop = 400
  grid.vm.$emit(
    'viewportscroll',
    new CustomEvent('viewportscroll', {
      detail: { dimension: 'rgRow', coordinate: 449 * 32 },
    }),
  )
  await flushPromises()
  await waitForAppendVisibility(true)

  const fallback = fetch.getMockImplementation()
  if (!fallback) throw new Error('Missing fetch implementation')
  const finishQueries: ((response: Response) => void)[] = []
  fetch.mockImplementation((input, init) => {
    if (String(input).endsWith('/query')) {
      const body = JSON.parse(String(init?.body)) as {
        offset: number
        includeCount: boolean
      }
      if (body.offset === 400 && body.includeCount)
        return new Promise<Response>((resolve) => finishQueries.push(resolve))
    }
    return fallback(input, init)
  })

  await wrapper!.get('.grid-add-row').trigger('click')
  await vi.waitFor(() => expect(finishQueries).toHaveLength(1))
  if (!receive) throw new Error('Realtime listener was not registered')
  receive({ t: 'row.created', tableId: 't' })
  finishQueries[0]!(Response.json(queryResponse(schema, ['name'], 400, 100)))
  await vi.waitFor(() => expect(finishQueries).toHaveLength(2))
  await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))
  gridState.scrollTop = 0
  grid.vm.$emit(
    'viewportscroll',
    new CustomEvent('viewportscroll', {
      detail: { dimension: 'rgRow', coordinate: 0 },
    }),
  )
  finishQueries[1]!(Response.json(queryResponse(schema, ['name'], 400, 100)))
  await flushPromises()
  await vi.waitFor(() => expect(gridState.scrollRow).toHaveBeenCalledWith(450))
})

it('waits for the viewport to be measurable before revealing an appended row', async () => {
  gridState.rowCount = 205
  gridState.scrollHeight = 500
  await mountGrid(schemaFor('text'))
  const grid = wrapper!.getComponent({ name: 'RevoGrid' })
  gridState.scrollTop = 400
  grid.vm.$emit(
    'viewportscroll',
    new CustomEvent('viewportscroll', {
      detail: { dimension: 'rgRow', coordinate: 204 * 32 },
    }),
  )
  await flushPromises()
  await waitForAppendVisibility(true)
  gridState.viewportHeight = 0
  gridState.scrollHeight = 0

  await wrapper!.get('.grid-add-row').trigger('click')
  await flushPromises()
  await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))
  expect(gridState.scrollRow).not.toHaveBeenCalled()
  await waitForAppendVisibility(false)

  gridState.viewportHeight = 100
  gridState.scrollHeight = 500
  gridState.scrollTop = 0
  grid.vm.$emit(
    'viewportscroll',
    new CustomEvent('viewportscroll', {
      detail: { dimension: 'rgRow', coordinate: 0 },
    }),
  )
  await vi.waitFor(() => expect(gridState.scrollRow).toHaveBeenCalledWith(205))
})

it('reveals an appended row at the end of an expanded multi-page group', async () => {
  const schema = schemaFor('text')
  addField(schema, 'status', 'text')
  schema.views[0]!.config.groupBy = ['status']
  const storageKey = `pandatable:groups:${navUser.id}:v:status`
  localStorage.setItem(storageKey, JSON.stringify(['[null]']))
  gridState.rowCount = 250
  gridState.scrollHeight = 500
  try {
    const fetch = await mountGrid(schema)
    const grid = wrapper!.getComponent({ name: 'RevoGrid' })
    gridState.scrollTop = 400
    grid.vm.$emit(
      'viewportscroll',
      new CustomEvent('viewportscroll', {
        detail: { dimension: 'rgRow', coordinate: 250 * 32 },
      }),
    )
    await flushPromises()
    await waitForAppendVisibility(true)
    const fallback = fetch.getMockImplementation()
    if (!fallback) throw new Error('Missing fetch implementation')
    let finishGroups: ((response: Response) => void) | undefined
    fetch.mockImplementation((input, init) =>
      String(input).endsWith('/groups')
        ? new Promise<Response>((resolve) => {
            finishGroups = resolve
          })
        : fallback(input, init),
    )
    await wrapper!.get('.grid-add-row').trigger('click')
    await vi.waitFor(() => expect(finishGroups).toBeDefined())
    await flushPromises()
    const pendingSource = grid.props('source') as {
      __rowId?: string
      __groupId?: string
    }[]
    expect(pendingSource).toHaveLength(251)
    expect(pendingSource.every((row) => !row.__rowId && !row.__groupId)).toBe(
      true,
    )
    if (!finishGroups) throw new Error('Grouped requery was not started')
    finishGroups(
      Response.json({
        groups: [{ key: null, count: 251, aggregates: [] }],
        schemaVersion: schema.schemaVersion,
      }),
    )
    await flushPromises()
    await vi.waitFor(() =>
      expect(gridState.scrollRow).toHaveBeenCalledWith(251),
    )
    const source = grid.props('source') as { __rowId?: string }[]
    expect(source).toHaveLength(252)
    expect(source.at(-1)?.__rowId).toBe(
      String(BigInt('9007199254740993') + 250n),
    )
  } finally {
    localStorage.removeItem(storageKey)
  }
})

it('waits for the final page to load before showing the append action', async () => {
  gridState.rowCount = 205
  gridState.scrollHeight = 500
  const schema = schemaFor('text')
  const fetch = await mountGrid(schema)
  await waitForAppendVisibility(false)
  const fallback = fetch.getMockImplementation()
  if (!fallback) throw new Error('Missing fetch implementation')
  let finishPage: ((response: Response) => void) | undefined
  fetch.mockImplementation((input, init) => {
    if (String(input).endsWith('/query')) {
      const body = JSON.parse(String(init?.body)) as { offset?: number }
      if (body.offset === 200)
        return new Promise<Response>((resolve) => {
          finishPage = resolve
        })
    }
    return fallback(input, init)
  })
  gridState.scrollTop = 400
  wrapper!.getComponent({ name: 'RevoGrid' }).vm.$emit(
    'viewportscroll',
    new CustomEvent('viewportscroll', {
      detail: { dimension: 'rgRow', coordinate: 204 * 32 },
    }),
  )
  await vi.waitFor(() => expect(finishPage).toBeDefined())
  await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))
  await waitForAppendVisibility(false)
  if (!finishPage) throw new Error('Final page request was not started')
  finishPage(Response.json(queryResponse(schema, ['name'], 200, 100)))
  await flushPromises()
  await waitForAppendVisibility(true)
})

it('removes and restores the append action with live table permissions', async () => {
  const schema = schemaFor('text', 'read')
  const fetch = await mountGrid(schema)
  expect(wrapper!.find('.grid-add-row').exists()).toBe(false)
  await wrapper!.setProps({ schema: { ...schema, myLevel: 'write' } })
  await waitForAppendVisibility(true)
  await wrapper!.setProps({ schema: { ...schema, myLevel: 'read' } })
  expect(wrapper!.find('.grid-add-row').exists()).toBe(false)
  expect(
    fetch.mock.calls.filter(
      ([path, init]) =>
        String(path).endsWith('/tables/t/rows') && init?.method === 'POST',
    ),
  ).toHaveLength(0)
})

it.each([
  ['ERR_REQUIRED_FIELD', 400],
  ['ERR_UNIQUE_CONFLICT', 409],
])(
  'reports %s on blank creation without an optimistic row',
  async (code, status) => {
    i18n.global.mergeLocaleMessage('en', {
      workspaceGrid: {
        ux09: {
          blankCreateHint: 'Use New to provide the required or unique values.',
        },
      },
    })
    gridState.rowCount = 0
    const fetch = await mountGrid(schemaFor('text'))
    await waitForAppendVisibility(true)
    const fallback = fetch.getMockImplementation()
    if (!fallback) throw new Error('Missing fetch implementation')
    fetch.mockImplementation((input, init) =>
      String(input).endsWith('/tables/t/rows') && init?.method === 'POST'
        ? Promise.resolve(Response.json({ error: { code } }, { status }))
        : fallback(input, init),
    )
    await wrapper!.get('.grid-add-row').trigger('click')
    await flushPromises()
    const message =
      code === 'ERR_REQUIRED_FIELD'
        ? i18n.global.t('errors.ERR_REQUIRED_FIELD')
        : i18n.global.t('errors.ERR_UNIQUE_CONFLICT')
    expect(wrapper!.get('[role="alert"]').text()).toContain(message)
    expect(wrapper!.get('[role="alert"]').text()).toContain(
      'Use New to provide the required or unique values.',
    )
    expect(wrapper!.find('dialog[open]').exists()).toBe(false)
    expect(useUndoStore().undoStack).toHaveLength(0)
    expect(
      (wrapper!.getComponent({ name: 'RevoGrid' }).props('source') as unknown[])
        .length,
    ).toBe(0)
  },
)

it('surfaces a network failure without creating an optimistic row or undo step', async () => {
  gridState.rowCount = 0
  const fetch = await mountGrid(schemaFor('text'))
  await waitForAppendVisibility(true)
  const fallback = fetch.getMockImplementation()
  if (!fallback) throw new Error('Missing fetch implementation')
  fetch.mockImplementation((input, init) =>
    String(input).endsWith('/tables/t/rows') && init?.method === 'POST'
      ? Promise.reject(new TypeError('Network unavailable'))
      : fallback(input, init),
  )
  await wrapper!.get('.grid-add-row').trigger('click')
  await flushPromises()
  expect(wrapper!.get('[role="alert"]').text()).toContain(
    i18n.global.t('errors.ERR_INTERNAL'),
  )
  expect(gridState.rowCount).toBe(0)
  expect(useUndoStore().undoStack).toHaveLength(0)
})

it('hides the action when the authoritative query fails', async () => {
  const schema = schemaFor('text')
  const fetch = await mountGrid(schema)
  await waitForAppendVisibility(true)
  const fallback = fetch.getMockImplementation()
  if (!fallback) throw new Error('Missing fetch implementation')
  fetch.mockImplementation((input, init) =>
    String(input).endsWith('/query')
      ? Promise.resolve(
          Response.json({ error: { code: 'ERR_INTERNAL' } }, { status: 500 }),
        )
      : fallback(input, init),
  )
  await wrapper!.setProps({
    filter: { field: 'name', cmp: 'is', value: 'matching' },
  })
  await flushPromises()
  await waitForAppendVisibility(false)
  expect(wrapper!.get('[role="alert"]').text()).toContain(
    i18n.global.t('errors.ERR_INTERNAL'),
  )
})

it('hides the action if a later page fails to load at the logical end', async () => {
  gridState.rowCount = 205
  gridState.scrollHeight = 500
  const fetch = await mountGrid(schemaFor('text'))
  await waitForAppendVisibility(false)
  const fallback = fetch.getMockImplementation()
  if (!fallback) throw new Error('Missing fetch implementation')
  fetch.mockImplementation((input, init) => {
    if (String(input).endsWith('/query')) {
      const body = JSON.parse(String(init?.body)) as { offset?: number }
      if (body.offset === 200)
        return Promise.resolve(
          Response.json({ error: { code: 'ERR_INTERNAL' } }, { status: 500 }),
        )
    }
    return fallback(input, init)
  })
  gridState.scrollTop = 400
  wrapper!.getComponent({ name: 'RevoGrid' }).vm.$emit(
    'viewportscroll',
    new CustomEvent('viewportscroll', {
      detail: { dimension: 'rgRow', coordinate: 204 * 32 },
    }),
  )
  await vi.waitFor(() =>
    expect(wrapper!.get('[role="alert"]').text()).toContain(
      i18n.global.t('errors.ERR_INTERNAL'),
    ),
  )
  await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))
  await waitForAppendVisibility(false)
})

it('reconciles a successful create if write access is revoked in flight', async () => {
  gridState.rowCount = 0
  const schema = schemaFor('text')
  const fetch = await mountGrid(schema)
  await waitForAppendVisibility(true)
  const fallback = fetch.getMockImplementation()
  if (!fallback) throw new Error('Missing fetch implementation')
  let finishCreate: ((response: Response) => void) | undefined
  fetch.mockImplementation((input, init) => {
    if (String(input).endsWith('/tables/t/rows') && init?.method === 'POST')
      return new Promise<Response>((resolve) => {
        finishCreate = resolve
      })
    return fallback(input, init)
  })
  await wrapper!.get('.grid-add-row').trigger('click')
  await wrapper!.setProps({ schema: { ...schema, myLevel: 'read' } })
  expect(wrapper!.find('.grid-add-row').exists()).toBe(false)
  gridState.rowCount = 1
  if (!finishCreate) throw new Error('Create request was not started')
  finishCreate(
    Response.json({
      eventId: 'event',
      rows: [{ id: '9007199254740993', fields: { name: 'default' } }],
    }),
  )
  await flushPromises()
  expect(useUndoStore().undoStack.at(-1)?.steps).toEqual([
    { kind: 'delete', tableId: 't', ids: ['9007199254740993'] },
  ])
  expect(
    (wrapper!.getComponent({ name: 'RevoGrid' }).props('source') as unknown[])
      .length,
  ).toBe(1)
  expect(
    fetch.mock.calls.filter(
      ([path, init]) =>
        String(path).endsWith('/tables/t/rows') && init?.method === 'POST',
    ),
  ).toHaveLength(1)
})

it('requeries a filtered view without inserting a fake visible row', async () => {
  i18n.global.mergeLocaleMessage('en', {
    workspaceGrid: {
      ux09: {
        createdMayBeHidden:
          'Row created. Filters, sorting, or collapsed groups may affect where it appears.',
      },
    },
  })
  gridState.rowCount = 0
  const schema = schemaFor('text')
  const fetch = await mountGrid(schema)
  const fallback = fetch.getMockImplementation()
  if (!fallback) throw new Error('Missing fetch implementation')
  fetch.mockImplementation((input, init) =>
    String(input).endsWith('/query')
      ? Promise.resolve(
          Response.json({
            rows: [],
            fieldIds: ['name'],
            total: 0,
            schemaVersion: schema.schemaVersion,
          }),
        )
      : fallback(input, init),
  )
  await wrapper!.setProps({
    filter: { field: 'name', cmp: 'is', value: 'matching' },
  })
  await flushPromises()
  await waitForAppendVisibility(true)
  await wrapper!.get('.grid-add-row').trigger('click')
  await flushPromises()
  expect(gridState.rowCount).toBe(1)
  expect(
    (wrapper!.getComponent({ name: 'RevoGrid' }).props('source') as unknown[])
      .length,
  ).toBe(0)
  expect(wrapper!.text()).toContain(
    'Row created. Filters, sorting, or collapsed groups may affect where it appears.',
  )
})

it('dismisses toolbar menus on outside pointer interactions', async () => {
  await mountGrid(schemaFor('text'))
  const root = wrapper!.element
  document.body.appendChild(root)
  try {
    const menu = wrapper!.get('details.grid-format')
      .element as HTMLDetailsElement
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

it('shows localized feedback for invalid Format widths without persisting them', async () => {
  await mountGrid(schemaFor('text'))
  const widthInput = wrapper!.get('.grid-column-control input[type="number"]')
  await widthInput.setValue('1001')
  await widthInput.trigger('change')
  expect(widthInput.attributes('aria-invalid')).toBe('true')
  expect(widthInput.attributes('aria-describedby')).toBeTruthy()
  expect(wrapper!.get('.grid-column-width-error').text()).toBe(
    'Enter a whole number from 60 to 1000.',
  )
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
  expect(editors.name).toBeDefined()
  const editor = mount(gridState.editorComponent!, {
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

it('keeps an editor mounted when an outside pointer interrupts IME composition', async () => {
  await mountGrid(schemaFor('text'))
  const save = vi.fn()
  const editor = mount(gridState.editorComponent!, {
    props: {
      column: { prop: 'name' },
      prop: 'name',
      rowIndex: 0,
      val: 'before',
      save,
      close: vi.fn(),
    },
    global: { plugins: [pinia, i18n] },
  })
  const outside = document.createElement('button')
  const clicked = vi.fn()
  outside.addEventListener('click', clicked)
  document.body.append(editor.element, outside)
  try {
    const input = editor.get('input')
    await input.setValue('中文')
    await input.trigger('compositionstart')
    const pointer = new MouseEvent('pointerdown', {
      bubbles: true,
      cancelable: true,
    })
    const click = new MouseEvent('click', { bubbles: true, cancelable: true })
    expect(outside.dispatchEvent(pointer)).toBe(false)
    expect(outside.dispatchEvent(click)).toBe(false)
    expect(clicked).not.toHaveBeenCalled()
    expect(save).not.toHaveBeenCalled()

    await input.trigger('compositionend')
    await flushPromises()
    expect(save).toHaveBeenCalledExactlyOnceWith('中文', true)
    expect(
      outside.dispatchEvent(
        new MouseEvent('click', { bubbles: true, cancelable: true }),
      ),
    ).toBe(true)
    expect(clicked).toHaveBeenCalledOnce()
  } finally {
    editor.unmount()
    outside.remove()
  }
})

it('activates a scalar on a single click without activating drags, modifiers, or links', async () => {
  await mountGrid(schemaFor('text'))
  document.body.appendChild(wrapper!.element)
  const grid = wrapper!.get('[data-testid="fake-grid"]')
  try {
    await grid.trigger('mousedown', { button: 0, clientX: 20, clientY: 20 })
    await grid.trigger('mouseup', { button: 0, clientX: 20, clientY: 20 })
    await flushPromises()
    expect(gridState.editCell).toHaveBeenCalledExactlyOnceWith(0, 'name')

    gridState.editCell.mockClear()
    await grid.trigger('mousedown', { button: 0, clientX: 20, clientY: 20 })
    await grid.trigger('mousemove', { buttons: 1, clientX: 40, clientY: 20 })
    await grid.trigger('mouseup', { button: 0, clientX: 40, clientY: 20 })
    await grid.trigger('mousedown', {
      button: 0,
      shiftKey: true,
      clientX: 20,
      clientY: 20,
    })
    await grid.trigger('mouseup', { button: 0, clientX: 20, clientY: 20 })
    const link = document.createElement('a')
    grid.element.appendChild(link)
    link.dispatchEvent(
      new MouseEvent('mousedown', {
        bubbles: true,
        button: 0,
        clientX: 20,
        clientY: 20,
      }),
    )
    link.dispatchEvent(
      new MouseEvent('mouseup', {
        bubbles: true,
        button: 0,
        clientX: 20,
        clientY: 20,
      }),
    )
    await flushPromises()
    expect(gridState.editCell).not.toHaveBeenCalled()
  } finally {
    wrapper!.element.remove()
  }
})

it.each([
  ['reader', 'text', 'read'],
  ['system field', 'id', 'owner'],
  ['reader select', 'single_select', 'read'],
  ['reader checkbox', 'checkbox', 'read'],
] as const)(
  'does not single-click activate a %s',
  async (_label, kind, level) => {
    const fetch = await mountGrid(schemaFor(kind, level))
    document.body.appendChild(wrapper!.element)
    try {
      const grid = wrapper!.get('[data-testid="fake-grid"]')
      const box = document.createElement('span')
      box.className = 'field-check'
      grid.element.appendChild(box)
      for (const target of [grid.element, box]) {
        for (const type of ['mousedown', 'mouseup'])
          target.dispatchEvent(
            new MouseEvent(type, {
              bubbles: true,
              button: 0,
              clientX: 20,
              clientY: 20,
            }),
          )
        await flushPromises()
      }
      expect(gridState.editCell).not.toHaveBeenCalled()
      expect(
        fetch.mock.calls.some(([, init]) => init?.method === 'PATCH'),
      ).toBe(false)
    } finally {
      wrapper!.element.remove()
    }
  },
)

it.each(['single_select', 'multi_select'])(
  'opens a writable %s editor on a single click',
  async (kind) => {
    await mountGrid(schemaFor(kind))
    document.body.appendChild(wrapper!.element)
    try {
      const grid = wrapper!.get('[data-testid="fake-grid"]')
      await grid.trigger('mousedown', { button: 0, clientX: 20, clientY: 20 })
      await grid.trigger('mouseup', { button: 0, clientX: 20, clientY: 20 })
      await flushPromises()
      expect(gridState.editCell).toHaveBeenCalledExactlyOnceWith(0, 'name')
    } finally {
      wrapper!.element.remove()
    }
  },
)

it('toggles a checkbox once per click on its box through the cell pipeline', async () => {
  gridState.rowFields = [{ name: false }]
  const fetch = await mountGrid(schemaFor('checkbox'))
  document.body.appendChild(wrapper!.element)
  const grid = wrapper!.get('[data-testid="fake-grid"]')
  const box = document.createElement('span')
  box.className = 'field-check'
  grid.element.appendChild(box)
  const click = async (target: Element) => {
    for (const type of ['mousedown', 'mouseup'])
      target.dispatchEvent(
        new MouseEvent(type, {
          bubbles: true,
          button: 0,
          clientX: 20,
          clientY: 20,
        }),
      )
    await flushPromises()
  }
  const patches = () =>
    fetch.mock.calls
      .filter(([, init]) => init?.method === 'PATCH')
      .map(([, init]) => JSON.parse(String(init?.body)) as unknown)
  try {
    // Selecting the cell outside the box does not toggle.
    await click(grid.element)
    expect(patches()).toEqual([])

    // The fake PATCH echoes gridState.value as the stored checkbox value.
    gridState.value = true as unknown as string
    await click(box)
    expect(patches()).toEqual([{ fields: { name: true } }])

    gridState.value = false as unknown as string
    await click(box)
    expect(patches()).toEqual([
      { fields: { name: true } },
      { fields: { name: false } },
    ])
    expect(gridState.editCell).not.toHaveBeenCalled()
  } finally {
    wrapper!.element.remove()
  }
})

function clickCheckBox() {
  const grid = wrapper!.get('[data-testid="fake-grid"]')
  let box = grid.element.querySelector('.field-check')
  if (!box) {
    box = document.createElement('span')
    box.className = 'field-check'
    grid.element.appendChild(box)
  }
  for (const type of ['mousedown', 'mouseup'])
    box.dispatchEvent(
      new MouseEvent(type, {
        bubbles: true,
        button: 0,
        clientX: 20,
        clientY: 20,
      }),
    )
  return flushPromises()
}

it('records a checkbox toggle as one guarded undo step', async () => {
  gridState.rowFields = [{ name: false }]
  gridState.value = true as unknown as string
  await mountGrid(schemaFor('checkbox'))
  document.body.appendChild(wrapper!.element)
  try {
    const undo = useUndoStore()
    const before = undo.undoStack.length
    await clickCheckBox()
    expect(undo.undoStack).toHaveLength(before + 1)
    expect(undo.undoStack.at(-1)?.steps).toEqual([
      {
        kind: 'patch',
        tableId: tableSchema().table.id,
        rows: [
          {
            id: '9007199254740993',
            fields: { name: false },
            expected: { name: true },
          },
        ],
      },
    ])
  } finally {
    wrapper!.element.remove()
  }
})

it('shows a rejected toggle honestly and keeps the stored value', async () => {
  gridState.rowFields = [{ name: false }]
  const fetch = await mountGrid(schemaFor('checkbox'))
  const original = fetch.getMockImplementation()!
  let reject = true
  fetch.mockImplementation(async (input, init) => {
    if (init?.method === 'PATCH' && reject)
      return Response.json(
        { error: { code: 'ERR_PERMISSION_DENIED' } },
        { status: 403 },
      )
    return original(input, init)
  })
  document.body.appendChild(wrapper!.element)
  try {
    const undo = useUndoStore()
    const before = undo.undoStack.length
    await clickCheckBox()
    await flushPromises()
    expect(wrapper!.text()).toContain(
      i18n.global.t('errors.ERR_PERMISSION_DENIED'),
    )
    expect(undo.undoStack).toHaveLength(before)
    // The cell still holds false, so the next toggle again requests true.
    reject = false
    gridState.value = true as unknown as string
    await clickCheckBox()
    const patches = fetch.mock.calls
      .filter(([, init]) => init?.method === 'PATCH')
      .map(([, init]) => JSON.parse(String(init?.body)) as unknown)
    expect(patches).toEqual([
      { fields: { name: true } },
      { fields: { name: true } },
    ])
    expect(wrapper!.text()).not.toContain(
      i18n.global.t('errors.ERR_PERMISSION_DENIED'),
    )
  } finally {
    wrapper!.element.remove()
  }
})

it('flips the newest intended checkbox value for rapid successive toggles', async () => {
  gridState.rowFields = [{ name: false }]
  const fetch = await mountGrid(schemaFor('checkbox'))
  const original = fetch.getMockImplementation()!
  const releases: ((response: Response) => void)[] = []
  fetch.mockImplementation((input, init) => {
    if (init?.method !== 'PATCH') return original(input, init)
    return new Promise<Response>((resolve) => releases.push(resolve))
  })
  document.body.appendChild(wrapper!.element)
  const grid = wrapper!.get('[data-testid="fake-grid"]')
  const box = document.createElement('span')
  box.className = 'field-check'
  grid.element.appendChild(box)
  try {
    for (let index = 0; index < 2; index++)
      for (const type of ['mousedown', 'mouseup'])
        box.dispatchEvent(
          new MouseEvent(type, {
            bubbles: true,
            button: 0,
            clientX: 20,
            clientY: 20,
          }),
        )
    await flushPromises()
    releases.shift()!(
      Response.json({
        row: { id: '9007199254740993', fields: { name: true } },
        previous: { name: false },
      }),
    )
    await flushPromises()
    releases.shift()!(
      Response.json({
        row: { id: '9007199254740993', fields: { name: false } },
        previous: { name: true },
      }),
    )
    await flushPromises()
    expect(
      fetch.mock.calls
        .filter(([, init]) => init?.method === 'PATCH')
        .map(([, init]) => JSON.parse(String(init?.body)) as unknown),
    ).toEqual([{ fields: { name: true } }, { fields: { name: false } }])
  } finally {
    wrapper!.element.remove()
  }
})

it('serializes successive cell saves and keeps the last authoritative response', async () => {
  const fetch = await mountGrid(schemaFor('text'))
  const original = fetch.getMockImplementation()!
  let releaseFirst: ((response: Response) => void) | undefined
  fetch.mockImplementation((input, init) => {
    if (init?.method !== 'PATCH') return original(input, init)
    const value = (
      JSON.parse(String(init.body)) as { fields: { name: string } }
    ).fields.name
    if (value === 'first')
      return new Promise<Response>((resolve) => {
        releaseFirst = resolve
      })
    return Promise.resolve(
      Response.json({
        row: { id: '9007199254740993', fields: { name: value } },
        previous: { name: 'first' },
      }),
    )
  })
  const commit = wrapper!.get('[data-testid="fake-grid"] button:first-child')
  gridState.value = 'first'
  await commit.trigger('click')
  await flushPromises()
  gridState.value = 'second'
  await commit.trigger('click')
  await flushPromises()
  expect(
    fetch.mock.calls.filter(([, init]) => init?.method === 'PATCH'),
  ).toHaveLength(1)
  expect(releaseFirst).toBeDefined()
  releaseFirst!(
    Response.json({
      row: { id: '9007199254740993', fields: { name: 'first' } },
      previous: { name: 'before' },
    }),
  )
  await flushPromises()
  const patches = fetch.mock.calls.filter(
    ([, init]) => init?.method === 'PATCH',
  )
  expect(patches).toHaveLength(2)
  expect(JSON.parse(String(patches[1]![1]?.body))).toEqual({
    fields: { name: 'second' },
  })
  const source = wrapper!
    .getComponent({ name: 'RevoGrid' })
    .props('source') as { name: string }[]
  expect(source[0]?.name).toBe('second')
})

it('applies a locally saved row after the editor closes without reloading the window', async () => {
  const fetch = await mountGrid(schemaFor('text'))
  const queriesBefore = fetch.mock.calls.filter(([input]) =>
    String(input).endsWith('/query'),
  ).length
  const editor = mount(gridState.editorComponent!, {
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
    gridState.value = 'after'
    await wrapper!
      .get('[data-testid="fake-grid"] button:first-child')
      .trigger('click')
    await flushPromises()
    const sourceWhileEditing = wrapper!
      .getComponent({ name: 'RevoGrid' })
      .props('source') as { name: string }[]
    expect(sourceWhileEditing[0]?.name).toBe('before')
  } finally {
    editor.unmount()
  }
  await flushPromises()
  const source = wrapper!
    .getComponent({ name: 'RevoGrid' })
    .props('source') as { name: string }[]
  expect(source[0]?.name).toBe('after')
  expect(
    fetch.mock.calls.filter(([input]) => String(input).endsWith('/query')),
  ).toHaveLength(queriesBefore)
})

it.each(['row.updated', 'comment.created'])(
  'refreshes again when %s arrives during window initialization',
  async (eventType) => {
    let receive: ((event: RealtimeEvent) => void) | undefined
    vi.spyOn(useRealtimeStore(), 'subscribe').mockImplementation(
      (_table, listener) => {
        receive = listener
        return () => {}
      },
    )
    const schema = schemaFor('text')
    const fetch = await mountGrid(schema)
    const original = fetch.getMockImplementation()!
    let resolveReload: ((response: Response) => void) | undefined
    let reloadSignal: AbortSignal | undefined
    fetch.mockImplementation((input, init) => {
      if (String(input).endsWith('/query') && !resolveReload) {
        reloadSignal = init?.signal ?? undefined
        return new Promise<Response>((resolve) => {
          resolveReload = resolve
        })
      }
      return original(input, init)
    })

    expect(receive).toBeDefined()
    receive!({ t: 'resync', tableId: 't' })
    await flushPromises()
    expect(resolveReload).toBeDefined()
    receive!({
      t: eventType,
      tableId: 't',
      ids: ['9007199254740993'],
    })
    expect(reloadSignal?.aborted).toBe(false)
    gridState.rowFields = [{ name: 'remote' }]
    resolveReload!(
      Response.json({
        ...queryResponse(schema),
        rows: [{ id: '9007199254740993', fields: { name: 'stale' } }],
      }),
    )
    await flushPromises()
    const source = wrapper!
      .getComponent({ name: 'RevoGrid' })
      .props('source') as { name: string }[]
    expect(source[0]?.name).toBe('remote')
  },
)

it('reconciles collaborator changes that overlap a local editor save', async () => {
  let receive: ((event: RealtimeEvent) => void) | undefined
  vi.spyOn(useRealtimeStore(), 'subscribe').mockImplementation(
    (_table, listener) => {
      receive = listener
      return () => {}
    },
  )
  const fetch = await mountGrid(schemaFor('text'))
  const original = fetch.getMockImplementation()!
  let resolveSave: ((response: Response) => void) | undefined
  fetch.mockImplementation((input, init) => {
    if (init?.method === 'PATCH')
      return new Promise<Response>((resolve) => {
        resolveSave = resolve
      })
    return original(input, init)
  })
  const editor = mount(gridState.editorComponent!, {
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
    gridState.value = 'local'
    await wrapper!
      .get('[data-testid="fake-grid"] button:first-child')
      .trigger('click')
    await flushPromises()
    expect(resolveSave).toBeDefined()
    gridState.rowFields = [{ name: 'remote latest' }]
    receive!({
      t: 'row.updated',
      tableId: 't',
      ids: ['9007199254740993'],
      fields: ['name'],
    })
    await flushPromises()
    resolveSave!(
      Response.json({
        row: { id: '9007199254740993', fields: { name: 'local' } },
        previous: { name: 'before' },
      }),
    )
    await flushPromises()
  } finally {
    editor.unmount()
  }
  await flushPromises()
  const source = wrapper!
    .getComponent({ name: 'RevoGrid' })
    .props('source') as { name: string }[]
  expect(source[0]?.name).toBe('remote latest')
})

it('refreshes realtime changes received while an uncached viewport loads', async () => {
  let receive: ((event: RealtimeEvent) => void) | undefined
  vi.spyOn(useRealtimeStore(), 'subscribe').mockImplementation(
    (_table, listener) => {
      receive = listener
      return () => {}
    },
  )
  gridState.rowCount = 1100
  const schema = schemaFor('text')
  const fetch = await mountGrid(schema)
  const original = fetch.getMockImplementation()!
  const pending = new Map<number, (response: Response) => void>()
  fetch.mockImplementation((input, init) => {
    const body = JSON.parse(String(init?.body ?? '{}')) as {
      offset?: number
      includeCount?: boolean
    }
    if (
      String(input).endsWith('/query') &&
      !body.includeCount &&
      body.offset !== undefined &&
      body.offset >= 900
    )
      return new Promise<Response>((resolve) => {
        pending.set(body.offset!, resolve)
      })
    return original(input, init)
  })
  wrapper!.findComponent({ name: 'RevoGrid' }).vm.$emit(
    'viewportscroll',
    new CustomEvent('viewportscroll', {
      detail: { dimension: 'rgRow', coordinate: 32_000 },
    }),
  )
  await flushPromises()
  expect(pending.has(900)).toBe(true)
  expect(pending.has(1000)).toBe(true)
  const rowId = String(BigInt('9007199254740993') + 1000n)
  gridState.rowFields[1000] = { name: 'remote latest' }
  receive!({ t: 'row.updated', tableId: 't', ids: [rowId] })
  pending.get(900)!(Response.json(queryResponse(schema, ['name'], 900, 100)))
  await flushPromises()
  const source = wrapper!
    .getComponent({ name: 'RevoGrid' })
    .props('source') as { name?: string; __rowId?: string }[]
  expect(source[1000]).toMatchObject({ __rowId: rowId, name: 'remote latest' })
  pending.get(1000)!(Response.json(queryResponse(schema, ['name'], 1000, 100)))
})

it.each(['permission', 'identity', 'view'] as const)(
  'does not dispatch queued writes after a %s change',
  async (change) => {
    const schema = schemaFor('text')
    const fetch = await mountGrid(schema)
    const original = fetch.getMockImplementation()!
    let releaseFirst: ((response: Response) => void) | undefined
    fetch.mockImplementation((input, init) => {
      if (init?.method !== 'PATCH') return original(input, init)
      return new Promise<Response>((resolve) => {
        releaseFirst = resolve
      })
    })
    const commit = wrapper!.get('[data-testid="fake-grid"] button:first-child')
    gridState.value = 'first'
    await commit.trigger('click')
    await flushPromises()
    gridState.value = 'second'
    await commit.trigger('click')
    await flushPromises()
    if (change === 'permission')
      await wrapper!.setProps({ schema: { ...schema, myLevel: 'read' } })
    else if (change === 'identity')
      useAuthStore().setUser({ ...navUser, id: 'bob' })
    else await wrapper!.setProps({ viewId: 'other' })
    expect(releaseFirst).toBeDefined()
    releaseFirst!(
      Response.json({
        row: { id: '9007199254740993', fields: { name: 'first' } },
      }),
    )
    await flushPromises()
    expect(
      fetch.mock.calls.filter(([, init]) => init?.method === 'PATCH'),
    ).toHaveLength(1)
  },
)

it('retains the active draft without saving when write permission is revoked', async () => {
  const schema = schemaFor('text')
  const fetch = await mountGrid(schema)
  const save = vi.fn()
  const close = vi.fn()
  const open = () =>
    mount(gridState.editorComponent!, {
      props: {
        column: { prop: 'name' },
        prop: 'name',
        rowIndex: 0,
        val: 'before',
        save,
        close,
      },
      global: { plugins: [pinia, i18n] },
    })
  const editor = open()
  await editor.get('input').setValue('permission draft')
  await wrapper!.setProps({ schema: { ...schema, myLevel: 'read' } })
  expect(save).not.toHaveBeenCalled()
  expect(close).toHaveBeenCalledOnce()
  expect(wrapper!.text()).toContain(
    i18n.global.t('errors.ERR_PERMISSION_DENIED'),
  )
  expect(
    fetch.mock.calls.filter(([, init]) => init?.method === 'PATCH'),
  ).toHaveLength(0)
  editor.unmount()
  await wrapper!.setProps({ schema })
  const reopened = open()
  expect((reopened.get('input').element as HTMLInputElement).value).toBe(
    'permission draft',
  )
  await reopened.get('input').trigger('keydown', { key: 'Escape' })
  reopened.unmount()
})

it('retains and reopens the raw draft after a rejected server save', async () => {
  const fetch = await mountGrid(schemaFor('text'))
  const original = fetch.getMockImplementation()!
  fetch.mockImplementation((input, init) =>
    init?.method === 'PATCH'
      ? Promise.resolve(
          Response.json(
            { error: { code: 'ERR_UNIQUE_CONFLICT' } },
            { status: 409 },
          ),
        )
      : original(input, init),
  )
  const open = () =>
    mount(gridState.editorComponent!, {
      props: {
        column: { prop: 'name' },
        prop: 'name',
        rowIndex: 0,
        val: 'before',
        save: (value: string) => {
          gridState.value = value
          ;(
            wrapper!.get('[data-testid="fake-grid"] button:first-child')
              .element as HTMLButtonElement
          ).click()
        },
        close: vi.fn(),
      },
      global: { plugins: [pinia, i18n] },
    })
  const editor = open()
  await editor.get('input').setValue('duplicate')
  await editor
    .get('input')
    .trigger('focusout', { relatedTarget: document.body })
  await flushPromises()
  expect(wrapper!.text()).toContain(i18n.global.t('errors.ERR_UNIQUE_CONFLICT'))
  editor.unmount()
  const reopened = open()
  expect((reopened.get('input').element as HTMLInputElement).value).toBe(
    'duplicate',
  )
  await reopened.get('input').trigger('keydown', { key: 'Escape' })
  expect(wrapper!.find('.grid-validation-summary').exists()).toBe(false)
  reopened.unmount()
})

it('preserves an earlier pending draft when a later edit is cancelled', async () => {
  const fetch = await mountGrid(schemaFor('text'))
  const original = fetch.getMockImplementation()!
  let release: ((response: Response) => void) | undefined
  fetch.mockImplementation((input, init) => {
    if (init?.method !== 'PATCH') return original(input, init)
    return new Promise<Response>((resolve) => {
      release = resolve
    })
  })
  const open = () =>
    mount(gridState.editorComponent!, {
      props: {
        column: { prop: 'name' },
        prop: 'name',
        rowIndex: 0,
        val: 'before',
        save: (value: string) => {
          gridState.value = value
          ;(
            wrapper!.get('[data-testid="fake-grid"] button:first-child')
              .element as HTMLButtonElement
          ).click()
        },
        close: vi.fn(),
      },
      global: { plugins: [pinia, i18n] },
    })
  const first = open()
  await first.get('input').setValue('first draft')
  await first.get('input').trigger('focusout', { relatedTarget: document.body })
  await flushPromises()
  first.unmount()
  const second = open()
  await second.get('input').setValue('cancel this')
  await second.get('input').trigger('keydown', { key: 'Escape' })
  second.unmount()
  expect(release).toBeDefined()
  release!(
    Response.json({ error: { code: 'ERR_UNIQUE_CONFLICT' } }, { status: 409 }),
  )
  await flushPromises()
  const recovered = open()
  expect((recovered.get('input').element as HTMLInputElement).value).toBe(
    'first draft',
  )
  expect(wrapper!.text()).toContain(i18n.global.t('errors.ERR_UNIQUE_CONFLICT'))
  recovered.unmount()
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
  addField(schema, 'hidden', 'text')
  schema.views[0]!.config.fields.push({ id: 'hidden', visible: false })
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

  await chooseFooterAggregate('amount', 'sum')
  const request = fetch.mock.calls
    .filter(([path]) => String(path).endsWith('/aggregates'))
    .at(-1)
  expect(request?.[1]?.method).toBe('POST')
  expect(JSON.parse(String(request?.[1]?.body))).toMatchObject({
    search: 'acme',
    aggregates: [{ fieldId: 'amount', aggregate: 'sum' }],
  })
  expect(
    wrapper!
      .findAll('.grid-summary-cell[data-field-id]')
      .map((cell) => cell.attributes('data-field-id')),
  ).toEqual(['name', 'amount'])
  expect(
    wrapper!.get('.grid-summary-cell[data-field-id="amount"] output').text(),
  ).toContain('42')
})

it('scopes quick search to one visible searchable field and falls back to all fields', async () => {
  const schema = schemaFor('text')
  addField(schema, 'comment', 'text')
  addField(schema, 'files', 'attachment')
  const fetch = await mountGrid(markRaw(schema))
  const popover = wrapper!.getComponent(GridSearchPopover)
  expect(popover.props('fields').map((field) => field.id)).toEqual([
    'name',
    'comment',
  ])
  const lastQuery = () =>
    fetch.mock.calls
      .filter(([path]) => String(path).endsWith('/query'))
      .map(([, init]) => JSON.parse(String(init?.body)) as RowQueryRequest)
      .at(-1)
  const settle = async () => {
    await new Promise((resolve) => setTimeout(resolve, 300))
    await flushPromises()
  }

  popover.vm.$emit('update:modelValue', { term: 'us', fieldId: 'comment' })
  await settle()
  expect(lastQuery()).toMatchObject({ search: 'us', searchField: 'comment' })
  expect(wrapper!.vm.$.exposed?.exportQuery()).toMatchObject({
    search: 'us',
    searchField: 'comment',
  })

  wrapper!.vm.$.exposed?.toggleField('comment', false)
  await settle()
  expect(popover.props('modelValue')).toEqual({ term: 'us', fieldId: null })
  expect(lastQuery()?.search).toBe('us')
  expect(lastQuery()?.searchField).toBeUndefined()
  await vi.waitFor(() =>
    expect(wrapper!.emitted('viewUpdated')).toHaveLength(1),
  )

  popover.vm.$emit('update:modelValue', { term: 'us', fieldId: 'name' })
  const respond = fetch.getMockImplementation()!
  let rejected = 0
  fetch.mockImplementation(async (path, init) => {
    const body = typeof init?.body === 'string' ? init.body : ''
    if (String(path).endsWith('/query') && body.includes('searchField')) {
      rejected += 1
      return Response.json(
        {
          error: {
            code: 'ERR_VALIDATION',
            message: 'invalid',
            details: { field: 'searchField' },
          },
        },
        { status: 400 },
      )
    }
    return respond(path, init)
  })
  await settle()
  await settle()
  expect(rejected).toBeGreaterThan(0)
  expect(popover.props('modelValue')).toEqual({ term: 'us', fieldId: null })
  expect(lastQuery()?.searchField).toBeUndefined()
  expect(wrapper!.find('[role="alert"]').exists()).toBe(false)
})

it('tracks the public footer column scroll event while keeping pinned cells fixed', async () => {
  const schema = schemaFor('text')
  addField(schema, 'amount', 'integer')
  schema.views[0]!.config.frozen = 1
  await mountGrid(schema)
  const grid = wrapper!.getComponent({ name: 'RevoGrid' })
  grid.vm.$emit(
    'viewportscroll',
    new CustomEvent('viewportscroll', {
      detail: { dimension: 'rgCol', coordinate: 260 },
    }),
  )
  await nextTick()
  expect(
    (wrapper!.get('.grid-summary-track-scrollable').element as HTMLElement)
      .style.transform,
  ).toBe('translateX(-260px)')
  expect(
    (wrapper!.get('.grid-summary-track-pinned').element as HTMLElement).style
      .transform,
  ).toBe('')
  expect(
    (
      wrapper!.get('.grid-summary-cell[data-field-id="name"]')
        .element as HTMLElement
    ).style.left,
  ).toBe('104px')
})

it('reclamps footer scrolling when the viewport becomes wider', async () => {
  const schema = schemaFor('text')
  addField(schema, 'amount', 'integer')
  schema.views[0]!.config.fields = [
    { id: 'name', visible: true, width: 400 },
    { id: 'amount', visible: true, width: 400 },
  ]
  await mountGrid(schema)
  let viewportWidth = 500
  Object.defineProperty(
    wrapper!.get('.grid-summary-viewport').element,
    'clientWidth',
    {
      configurable: true,
      get: () => viewportWidth,
    },
  )
  const grid = wrapper!.getComponent({ name: 'RevoGrid' })
  grid.vm.$emit(
    'viewportscroll',
    new CustomEvent('viewportscroll', {
      detail: { dimension: 'rgCol', coordinate: 700 },
    }),
  )
  await nextTick()
  const track = wrapper!.get('.grid-summary-track-scrollable')
    .element as HTMLElement
  expect(track.style.transform).toBe('translateX(-404px)')

  viewportWidth = 700
  grid.vm.$emit('contentsizechanged')
  await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))
  await nextTick()
  expect(track.style.transform).toBe('translateX(-204px)')
})

it('reveals an offscreen footer trigger using the public grid scroll API', async () => {
  const schema = schemaFor('text')
  addField(schema, 'amount', 'integer')
  schema.views[0]!.config.frozen = 1
  await mountGrid(schema)
  const viewport = wrapper!.get('.grid-summary-viewport').element
  const trigger = wrapper!.get(
    '.grid-summary-cell[data-field-id="amount"] button',
  )
  vi.spyOn(viewport, 'getBoundingClientRect').mockReturnValue(
    new DOMRect(0, 0, 500, 32),
  )
  vi.spyOn(trigger.element, 'getBoundingClientRect').mockReturnValue(
    new DOMRect(650, 0, 180, 32),
  )

  await trigger.trigger('focus')
  expect(gridState.scrollColumn).toHaveBeenCalledWith('amount')
})

it('resets footer scrolling when the desktop grid is removed and restored', async () => {
  await mountGrid(schemaFor('text'))
  const media = window.matchMedia('(min-width: 768px)')
  if (!(media instanceof ThemeMedia)) throw new Error('Missing media mock')
  wrapper!.getComponent({ name: 'RevoGrid' }).vm.$emit(
    'viewportscroll',
    new CustomEvent('viewportscroll', {
      detail: { dimension: 'rgCol', coordinate: 150 },
    }),
  )
  await nextTick()
  expect(
    (wrapper!.get('.grid-summary-track-scrollable').element as HTMLElement)
      .style.transform,
  ).toBe('translateX(-150px)')

  media.setDark(false)
  await nextTick()
  expect(wrapper!.find('[data-testid="fake-grid"]').exists()).toBe(false)
  expect(wrapper!.find('.grid-summary-bar').exists()).toBe(false)

  media.setDark(true)
  await flushPromises()
  expect(
    (wrapper!.get('.grid-summary-track-scrollable').element as HTMLElement)
      .style.transform,
  ).toBe('translateX(-0px)')
  wrapper!.getComponent({ name: 'RevoGrid' }).vm.$emit(
    'viewportscroll',
    new CustomEvent('viewportscroll', {
      detail: { dimension: 'rgCol', coordinate: 75 },
    }),
  )
  await nextTick()
  expect(
    (wrapper!.get('.grid-summary-track-scrollable').element as HTMLElement)
      .style.transform,
  ).toBe('translateX(-75px)')
})

it('keeps an unconfigured footer empty and removes its config when None is selected', async () => {
  gridState.rowCount = 3
  const schema = schemaFor('text')
  addField(schema, 'amount', 'integer', { size: 'int' })
  const fetch = await mountGrid(schema)
  expect(wrapper!.get('.grid-summary-record-count').text()).toBe('3 records')
  const cell = wrapper!.get('.grid-summary-cell[data-field-id="amount"]')
  const trigger = cell.get<HTMLButtonElement>('button')
  expect(cell.find('output').exists()).toBe(false)
  expect(trigger.element.tabIndex).toBe(0)
  expect(trigger.attributes('aria-expanded')).toBe('false')
  expect(trigger.get('.grid-summary-hint').text()).toBe('Summary')
  expect(
    fetch.mock.calls.filter(([path]) => String(path).endsWith('/aggregates')),
  ).toHaveLength(0)

  await chooseFooterAggregate('amount', 'sum')
  expect(cell.get('output').text()).toContain('Sum')
  expect(cell.get('output').text()).toContain('42')
  expect(trigger.attributes('aria-describedby')).toBe(
    cell.get('output').attributes('id'),
  )

  await chooseFooterAggregate('amount', 'none')
  expect(cell.find('output').exists()).toBe(false)
  const config = wrapper!
    .emitted('viewConfigChanged')
    ?.at(-1)?.[0] as ViewColumns
  expect(config.aggregates ?? []).not.toContainEqual({
    fieldId: 'amount',
    aggregate: 'sum',
  })
})

it('supports footer listbox keyboard navigation, selection, and focus return', async () => {
  await mountGrid(schemaFor('text'), navUser.id, '/', true)
  const trigger = wrapper!.get<HTMLButtonElement>(
    '.grid-summary-cell[data-field-id="name"] button',
  )
  trigger.element.focus()
  const menu = await openFooterPicker('name')
  expect(document.activeElement).toBe(menu.element)
  expect(menu.attributes('role')).toBe('listbox')
  expect(menu.get('[data-aggregate="none"]').attributes('aria-selected')).toBe(
    'true',
  )
  await menu.trigger('keydown', { key: 'ArrowDown' })
  expect(menu.attributes('aria-activedescendant')).toBe(
    menu.get('[data-aggregate="count"]').attributes('id'),
  )
  await menu.trigger('keydown', { key: 'Enter' })
  await flushPromises()
  expect(wrapper!.find('.grid-summary-menu').exists()).toBe(false)
  expect(document.activeElement).toBe(trigger.element)
  expect(
    wrapper!.get('.grid-summary-cell[data-field-id="name"] output').text(),
  ).toContain('42')

  const reopened = await openFooterPicker('name')
  await reopened.trigger('keydown', { key: 'End' })
  await reopened.trigger('keydown', { key: 'ArrowUp' })
  expect(reopened.attributes('aria-activedescendant')).toBe(
    reopened.get('[data-aggregate="percent_filled"]').attributes('id'),
  )
  await reopened.trigger('keydown', { key: 'Home' })
  expect(reopened.attributes('aria-activedescendant')).toBe(
    reopened.get('[data-aggregate="none"]').attributes('id'),
  )
  await reopened.trigger('keydown', { key: 'Escape' })
  expect(wrapper!.find('.grid-summary-menu').exists()).toBe(false)
  expect(document.activeElement).toBe(trigger.element)
})

it.each([
  { label: 'reader', level: 'read', locked: false },
  { label: 'writer of a locked view', level: 'write', locked: true },
] as const)(
  'keeps footer changes local for a $label',
  async ({ level, locked }) => {
    const schema = schemaFor('text', level)
    schema.views[0]!.locked = locked
    const fetch = await mountGrid(schema)

    await chooseFooterAggregate('name', 'count')
    expect(
      wrapper!.get('.grid-summary-cell[data-field-id="name"] output').text(),
    ).toContain('42')
    await wrapper!.vm.$.exposed?.flushColumnChanges()
    expect(
      fetch.mock.calls.filter(
        ([path, init]) =>
          String(path).endsWith('/views/v') && init?.method === 'PATCH',
      ),
    ).toHaveLength(0)
    expect(wrapper!.emitted('viewConfigChanged')?.at(-1)?.[1]).toBe(true)

    await chooseFooterAggregate('name', 'none')
    expect(
      wrapper!.find('.grid-summary-cell[data-field-id="name"] output').exists(),
    ).toBe(false)
  },
)

it('offers type-aware footer operations, renders API values, and clears config', async () => {
  const schema = schemaFor('text')
  addField(schema, 'amount', 'integer', { size: 'int' })
  addField(schema, 'created', 'date')
  addField(schema, 'checked', 'checkbox')
  addField(schema, 'files', 'attachment')
  const fetch = await mountGrid(schema)

  const checkboxMenu = await openFooterPicker('checked')
  expect(
    checkboxMenu
      .findAll('[role="option"]')
      .map((option) => option.attributes('data-aggregate')),
  ).toEqual(['none', 'count', 'percent_checked'])
  await checkboxMenu.get('[data-aggregate="none"]').trigger('click')
  expect(
    wrapper!.find('.grid-summary-cell[data-field-id="files"] button').exists(),
  ).toBe(false)
  expect(
    wrapper!
      .find('.grid-summary-cell[data-field-id="files"] .grid-summary-hint')
      .exists(),
  ).toBe(false)

  for (const aggregate of [
    'count',
    'count_empty',
    'count_filled',
    'unique',
    'percent_empty',
    'percent_filled',
    'percent_unique',
  ]) {
    const menu = await openFooterPicker('name')
    expect(menu.find(`[data-aggregate="${aggregate}"]`).exists()).toBe(true)
    await menu.get(`[data-aggregate="${aggregate}"]`).trigger('click')
    await flushPromises()
    const request = fetch.mock.calls
      .filter(([path]) => String(path).endsWith('/aggregates'))
      .at(-1)
    const body = JSON.parse(String(request?.[1]?.body)) as {
      aggregates: { fieldId: string; aggregate: string }[]
    }
    expect(body.aggregates).toContainEqual({ fieldId: 'name', aggregate })
    expect(
      wrapper!.get('.grid-summary-cell[data-field-id="name"] output').text(),
    ).toContain(aggregate.startsWith('percent_') ? '25%' : '42')
  }

  for (const aggregate of ['sum', 'avg']) {
    const menu = await openFooterPicker('amount')
    expect(menu.find(`[data-aggregate="${aggregate}"]`).exists()).toBe(true)
    await menu.get(`[data-aggregate="${aggregate}"]`).trigger('click')
    await flushPromises()
    expect(
      wrapper!.get('.grid-summary-cell[data-field-id="amount"] output').text(),
    ).toContain('42')
  }

  for (const aggregate of ['earliest', 'latest']) {
    const menu = await openFooterPicker('created')
    expect(menu.find(`[data-aggregate="${aggregate}"]`).exists()).toBe(true)
    await menu.get(`[data-aggregate="${aggregate}"]`).trigger('click')
    await flushPromises()
    expect(
      wrapper!.get('.grid-summary-cell[data-field-id="created"] output').text(),
    ).toContain('2026')
  }

  await chooseFooterAggregate('name', 'none')
  expect(
    wrapper!.find('.grid-summary-cell[data-field-id="name"] output').exists(),
  ).toBe(false)
  const lastRequest = fetch.mock.calls
    .filter(([path]) => String(path).endsWith('/aggregates'))
    .at(-1)
  expect(
    JSON.parse(String(lastRequest?.[1]?.body)).aggregates,
  ).not.toContainEqual({ fieldId: 'name', aggregate: 'percent_unique' })
})

it('shows footer aggregate failures and retries the server request explicitly', async () => {
  const schema = schemaFor('text')
  gridState.footerFailuresRemaining = 1
  const fetch = await mountGrid(schema)
  await chooseFooterAggregate('name', 'count')
  expect(wrapper!.get('[role="alert"]').text()).toContain(
    'Footer aggregates could not be loaded.',
  )

  await wrapper!.get('[role="alert"] button').trigger('click')
  await flushPromises()
  expect(wrapper!.find('[role="alert"]').exists()).toBe(false)
  expect(
    fetch.mock.calls.filter(([path]) => String(path).endsWith('/aggregates')),
  ).toHaveLength(2)
  expect(
    wrapper!.get('.grid-summary-cell[data-field-id="name"] output').text(),
  ).toContain('42')
})

it('ignores a stale footer aggregate reply after a newer function is selected', async () => {
  const schema = schemaFor('text')
  const fetch = await mountGrid(schema)
  let resolveFirst!: (response: Response) => void
  const firstReply = new Promise<Response>((resolve) => {
    resolveFirst = resolve
  })
  fetch.mockImplementationOnce(async () => firstReply)
  await chooseFooterAggregate('name', 'count')
  await chooseFooterAggregate('name', 'unique')
  expect(
    wrapper!.get('.grid-summary-cell[data-field-id="name"] output').text(),
  ).toContain('42')

  resolveFirst(
    Response.json({
      aggregates: [{ fieldId: 'name', aggregate: 'count', value: 99 }],
      schemaVersion: schema.schemaVersion,
    }),
  )
  await flushPromises()
  expect(
    wrapper!.get('.grid-summary-cell[data-field-id="name"] output').text(),
  ).toContain('42')
  expect(
    wrapper!.get('.grid-summary-cell[data-field-id="name"] output').text(),
  ).not.toContain('99')
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
  expect(wrapper!.findAll('input[type="checkbox"]')).toHaveLength(0)
  wrapper!.vm.$.exposed?.toggleField('system-id', true)
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
    .get('[data-testid="fake-grid"]')
    .findAll('button')
    .find((button) => button.text() === 'cut')!
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

it('uses the live grid focus for consecutive keyboard jumps across evicted pages', async () => {
  gridState.rowCount = 401
  await mountGrid(schemaFor('text'))
  await wrapper!
    .findAll('button')
    .find((button) => button.text() === 'focus')!
    .trigger('click')
  gridState.focusCells.mockImplementation(
    async (
      from: { x: number; y: number },
      _to: { x: number; y: number },
      type: string,
    ) => {
      gridState.focus = {
        rowIndex: from.y,
        colIndex: from.x,
        colType: type,
      }
    },
  )

  await wrapper!
    .get('.data-grid')
    .trigger('keydown', { key: 'End', ctrlKey: true })
  await flushPromises()
  expect(gridState.focusCells).toHaveBeenLastCalledWith(
    { x: 0, y: 400 },
    { x: 0, y: 400 },
    'rgCol',
  )

  await wrapper!
    .get('.data-grid')
    .trigger('keydown', { key: 'Home', ctrlKey: true })
  await flushPromises()
  expect(gridState.focusCells).toHaveBeenLastCalledWith(
    { x: 0, y: 0 },
    { x: 0, y: 0 },
    'rgCol',
  )
})

it('edits and expands the focused row immediately after a cross-page jump', async () => {
  gridState.rowCount = 401
  await mountGrid(schemaFor('text'))
  await wrapper!
    .findAll('button')
    .find((button) => button.text() === 'focus')!
    .trigger('click')
  gridState.focusCells.mockImplementation(
    async (
      from: { x: number; y: number },
      _to: { x: number; y: number },
      type: string,
    ) => {
      gridState.focus = {
        rowIndex: from.y,
        colIndex: from.x,
        colType: type,
      }
    },
  )

  await wrapper!
    .get('.data-grid')
    .trigger('keydown', { key: 'End', ctrlKey: true })
  await flushPromises()
  await wrapper!.get('.data-grid').trigger('keydown', { key: 'F2' })
  expect(gridState.editCell).toHaveBeenLastCalledWith(400, 'name')
  await wrapper!.get('.data-grid').trigger('keydown', { key: ' ' })
  expect(wrapper!.emitted('expand')?.at(-1)).toEqual([
    String(BigInt('9007199254740993') + 400n),
  ])
})

it('keeps an in-flight keyboard jump when the current cell is temporarily unloaded', async () => {
  const schema = schemaFor('text')
  gridState.rowCount = 401
  const fetch = await mountGrid(schema)
  const fallback = fetch.getMockImplementation()
  if (!fallback) throw new Error('Missing fetch implementation')
  let finishTarget: ((response: Response) => void) | undefined
  fetch.mockImplementation((input, init) => {
    if (
      String(input).endsWith('/query') &&
      (JSON.parse(String(init?.body)) as RowQueryRequest).offset === 400
    )
      return new Promise<Response>((resolve) => {
        finishTarget = resolve
      })
    return fallback(input, init)
  })
  await wrapper!
    .findAll('button')
    .find((button) => button.text() === 'focus')!
    .trigger('click')

  await wrapper!
    .get('.data-grid')
    .trigger('keydown', { key: 'End', ctrlKey: true })
  await flushPromises()
  if (!finishTarget) throw new Error('Target page was not requested')
  await wrapper!
    .get('.data-grid')
    .trigger('keydown', { key: 'Home', ctrlKey: true })
  await flushPromises()
  finishTarget(Response.json(queryResponse(schema, ['name'], 400, 100)))
  await flushPromises()
  expect(gridState.focusCells).toHaveBeenLastCalledWith(
    { x: 0, y: 400 },
    { x: 0, y: 400 },
    'rgCol',
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
