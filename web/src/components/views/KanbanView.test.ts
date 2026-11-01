import { afterEach, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import type { DataRow } from '../../api/types'
import { invalidateUserLabels } from '../../fields/userDirectory'
import { i18n, loadLocale } from '../../i18n'
import { useAuthStore } from '../../stores/auth'
import { useRealtimeStore } from '../../stores/realtime'
import { useUndoStore } from '../../stores/undo'
import { navUser, tableSchema } from '../../test/navigation'
import KanbanView from './KanbanView.vue'

let wrapper: VueWrapper | undefined
let pinia: ReturnType<typeof createPinia>

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  if (pinia) disposePinia(pinia)
  invalidateUserLabels()
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

async function setup(
  level: 'read' | 'write' | 'owner' = 'owner',
  kind: 'single_select' | 'user' = 'single_select',
  choices = ['Todo', 'Done'],
) {
  await loadLocale('en')
  i18n.global.locale.value = 'en'
  pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().setUser(navUser)
  const schema = tableSchema()
  schema.myLevel = level
  schema.fields.push({
    ...schema.fields[0]!,
    id: 'status',
    name: kind === 'user' ? 'Assignee' : 'Status',
    uiType: kind,
    options:
      kind === 'user'
        ? {}
        : {
            choices: choices.map((label, index) => ({
              id: `choice-${index}`,
              label,
            })),
          },
    position: 2048,
  })
  const view = schema.views[0]!
  view.type = 'kanban'
  view.config.kanban = { groupField: 'status', fields: ['name', 'status'] }
  const userId = '00000000-0000-0000-0000-000000000001'
  const state = {
    rows: [
      {
        id: '1',
        fields: { name: 'First task', status: kind === 'user' ? null : 'Todo' },
      },
      { id: '2', fields: { name: 'Second task', status: null } },
      ...(kind === 'user'
        ? []
        : [{ id: '3', fields: { name: 'Old task', status: 'Old' } }]),
    ] as DataRow[],
    rejectPatch: false,
    omitPrevious: false,
    mismatchPatch: false,
  }
  const fetcher = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      const body = JSON.parse(String(init?.body ?? '{}')) as {
        groupPath?: (string | null)[]
        offset?: number
        limit?: number
        fields?: Record<string, string | null>
        ids?: string[]
      }
      if (url.endsWith('/groups')) {
        const counts = new Map<string | null, number>()
        for (const row of state.rows) {
          const key = row.fields.status
          if (key !== null && typeof key !== 'string')
            throw new Error('Invalid test group')
          counts.set(key, (counts.get(key) ?? 0) + 1)
        }
        return Response.json({
          groups: [...counts].map(([key, count]) => ({
            key,
            count,
            aggregates: [],
          })),
          schemaVersion: 1,
        })
      }
      if (url.endsWith('/query')) {
        const rows = state.rows.filter(
          (row) => row.fields.status === body.groupPath?.[0],
        )
        return Response.json({
          rows: rows.slice(
            body.offset ?? 0,
            (body.offset ?? 0) + (body.limit ?? 20),
          ),
          fieldIds: ['name', 'status'],
          total: null,
          schemaVersion: 1,
        })
      }
      if (url.includes('/directory/users?'))
        return Response.json({
          items: [{ id: userId, name: 'Bob', username: 'bob' }],
          hasMore: false,
        })
      if (url.endsWith('/directory/user-labels'))
        return Response.json({
          items: (body.ids ?? []).map((id) => ({ id, name: 'Bob' })),
        })
      if (url.includes('/rows/') && init?.method === 'PATCH') {
        if (state.rejectPatch)
          return Response.json(
            { error: { code: 'ERR_PERMISSION_DENIED' } },
            { status: 403 },
          )
        const row = state.rows.find((item) => url.endsWith(`/${item.id}`))
        if (!row || !body.fields) throw new Error('Missing test row')
        const previous = { status: row.fields.status }
        row.fields.status = body.fields.status ?? null
        const returnedRow = state.mismatchPatch
          ? { ...row, fields: { ...row.fields, status: previous.status } }
          : row
        return Response.json({
          row: returnedRow,
          ...(state.omitPrevious ? {} : { previous }),
          eventId: 'event',
        })
      }
      throw new Error(`Unexpected request: ${url}`)
    },
  )
  vi.stubGlobal('fetch', fetcher)
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/t/:tableId/v/:viewId', component: KanbanView }],
  })
  await router.push('/t/t/v/v')
  wrapper = mount(KanbanView, {
    props: { schema, view, identity: navUser.id },
    global: { plugins: [pinia, i18n, router] },
  })
  await flushPromises()
  return { wrapper, state, fetcher, userId }
}

function column(name: string) {
  const found = wrapper
    ?.findAll('.kanban-column')
    .find((item) => item.get('h3').text() === name)
  if (!found) throw new Error(`Missing column ${name}`)
  return found
}

it('shows empty and unavailable groups and persists a dragged card', async () => {
  const { state, fetcher } = await setup()
  expect(column('Unassigned').text()).toContain('Second task')
  expect(column('Unavailable choice: Old').text()).toContain('Old task')
  expect(column('Done').text()).toContain('0 cards')
  const card = column('Todo').get('[data-row-id="1"]')
  await card.trigger('dragstart', {
    dataTransfer: { setData: vi.fn(), effectAllowed: '' },
  })
  await column('Done').trigger('dragover')
  await column('Done').trigger('drop')
  await flushPromises()
  expect(state.rows[0]!.fields.status).toBe('Done')
  expect(column('Done').text()).toContain('First task')
  expect(fetcher).toHaveBeenCalledWith(
    '/api/v1/tables/t/rows/1',
    expect.objectContaining({
      method: 'PATCH',
      body: JSON.stringify({ fields: { status: 'Done' } }),
    }),
  )
})

it('exposes an accessible move control and keeps failed writes visibly unchanged', async () => {
  const { state } = await setup()
  state.rejectPatch = true
  await wrapper!.get('[aria-label="Move card 1"]').trigger('click')
  await wrapper!.get('.kanban-card form select').setValue('"Done"')
  await wrapper!.get('.kanban-card form').trigger('submit')
  await flushPromises()
  expect(state.rows[0]!.fields.status).toBe('Todo')
  expect(wrapper!.get('[role="alert"]').text()).toContain('permission')
  expect(column('Todo').text()).toContain('First task')
})

it.each(['omitPrevious', 'mismatchPatch'] as const)(
  'reports %s responses and reloads without creating an unsafe undo',
  async (invalidResponse) => {
    const { state } = await setup()
    state[invalidResponse] = true
    await wrapper!.get('[aria-label="Move card 1"]').trigger('click')
    await wrapper!.get('.kanban-card form select').setValue('"Done"')
    await wrapper!.get('.kanban-card form').trigger('submit')
    await flushPromises()
    expect(wrapper!.get('[role="alert"]').text()).toBe(
      i18n.global.t('grid.saveFailed'),
    )
    expect(state.rows[0]!.fields.status).toBe('Done')
    expect(column('Done').text()).toContain('First task')
    expect(useUndoStore().undoStack).toHaveLength(0)
  },
)

it('disables moves for readers and refreshes on realtime invalidation', async () => {
  const { state } = await setup('read')
  expect(wrapper!.find('[aria-label="Move card 1"]').exists()).toBe(false)
  state.rows[0]!.fields.status = 'Done'
  vi.useFakeTimers()
  useRealtimeStore().invalidateLocal('t')
  await vi.advanceTimersByTimeAsync(100)
  await flushPromises()
  expect(column('Done').text()).toContain('First task')
})

it('searches for a user destination and assigns a card without dragging', async () => {
  const { state, userId } = await setup('write', 'user')
  await wrapper!.get('.user-search input').setValue('Bob')
  await wrapper!.get('.user-search').trigger('submit')
  await flushPromises()
  await wrapper!.get('button[aria-label="Move card 1"]').trigger('click')
  await wrapper!.get('.user-search button[type="button"]').trigger('click')
  await flushPromises()
  await wrapper!
    .get('.kanban-card form select')
    .setValue(JSON.stringify(userId))
  await wrapper!.get('.kanban-card form').trigger('submit')
  await flushPromises()
  expect(state.rows[0]!.fields.status).toBe(userId)
  expect(column('Bob').text()).toContain('First task')
})

it('bounds card and column pages without losing later records', async () => {
  const { state, fetcher } = await setup('owner', 'single_select', [
    'Todo',
    'Done',
    'Extra 1',
    'Extra 2',
    'Extra 3',
    'Extra 4',
    'Extra 5',
    'Extra 6',
  ])
  state.rows = Array.from({ length: 25 }, (_, index) => ({
    id: String(index + 1),
    fields: { name: `Task ${index + 1}`, status: 'Todo' },
  }))
  useRealtimeStore().invalidateLocal('t')
  await new Promise((resolve) => setTimeout(resolve, 150))
  await flushPromises()
  expect(column('Todo').findAll('.kanban-card')).toHaveLength(20)
  const next = column('Todo').get('[aria-label="Next cards in Todo"]')
  expect(next.attributes('disabled')).toBeUndefined()
  await next.trigger('click')
  await flushPromises()
  expect(
    fetcher.mock.calls
      .filter(([url]) => String(url).endsWith('/query'))
      .map(([, init]) => JSON.parse(String(init?.body))),
  ).toContainEqual(expect.objectContaining({ groupPath: ['Todo'], offset: 20 }))
  await vi.waitFor(() =>
    expect(column('Todo').findAll('.kanban-card')).toHaveLength(5),
  )
  expect(column('Todo').text()).toContain('Task 25')
  await wrapper!
    .findAll('button')
    .find((button) => button.text() === 'Next columns')!
    .trigger('click')
  await flushPromises()
  expect(column('Extra 6').exists()).toBe(true)
})
