import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { i18n, loadLocale } from '../../i18n'
import type { TableSchema } from '../../api/types'
import { useAuthStore } from '../../stores/auth'
import { navUser, tableSchema } from '../../test/navigation'
import { ThemeMedia } from '../../test/setup'
import DataGrid from './DataGrid.vue'

const gridState = vi.hoisted(() => ({
  fieldId: '',
  value: 'changed',
  rowIndex: 0,
}))

vi.mock('@revolist/vue3-datagrid', async () => {
  const vue = await import('vue')
  return {
    default: vue.defineComponent({
      props: ['source', 'columns', 'editors', 'readonly', 'useClipboard'],
      emits: ['created', 'beforeedit', 'viewportscroll'],
      setup(_props, { emit }) {
        const root = vue.ref<HTMLElement>()
        vue.onMounted(() => {
          const element = root.value as
            | (HTMLElement & {
                getSourceStore: () => Promise<{ set: () => void }>
              })
            | undefined
          if (element) element.getSourceStore = async () => ({ set: vi.fn() })
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

beforeEach(async () => {
  pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().setUser(navUser)
  const media = new ThemeMedia()
  media.matches = true
  vi.stubGlobal('matchMedia', () => media)
  await loadLocale('en')
  await loadLocale('zh-CN')
  i18n.global.locale.value = 'en'
  gridState.fieldId = 'name'
  gridState.value = 'changed'
  gridState.rowIndex = 0
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  disposePinia(pinia)
  vi.unstubAllGlobals()
})

function queryResponse(schema: TableSchema, fieldIds = ['name']) {
  return {
    rows: [
      { id: '9007199254740993', fields: { [schema.fields[0]!.id]: 'before' } },
    ],
    fieldIds,
    total: 1,
    schemaVersion: schema.schemaVersion,
  }
}

async function mountGrid(schema: TableSchema, identity = navUser.id) {
  const fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input)
    const body =
      typeof init?.body === 'string'
        ? (JSON.parse(init.body) as { fieldIds?: string[] })
        : undefined
    if (path.endsWith('/query'))
      return Response.json(queryResponse(schema, body?.fieldIds ?? ['name']))
    if (init?.method === 'PATCH')
      return Response.json({
        row: {
          id: '9007199254740993',
          fields: { [schema.fields[0]!.id]: gridState.value },
        },
      })
    if (init?.method === 'POST')
      return Response.json({
        eventId: 'event',
        rows: [{ id: '9007199254740994', fields: {} }],
      })
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
    body: JSON.stringify({ offset: 0, limit: 100, includeCount: true }),
  })
  const patch = fetch.mock.calls.find(([path]) =>
    String(path).endsWith('/9007199254740993'),
  )?.[1]
  expect(patch).toMatchObject({
    method: 'PATCH',
    body: JSON.stringify({ fields: { name: 'changed' } }),
  })
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
  expect(wrapper!.get('button').attributes('disabled')).toBeDefined()
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
