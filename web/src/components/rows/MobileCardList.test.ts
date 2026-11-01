import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import type { RowQueryRequest, TableSchema } from '../../api/types'
import { i18n, loadLocale } from '../../i18n'
import { useAuthStore } from '../../stores/auth'
import { useNavStore } from '../../stores/nav'
import { useUndoStore } from '../../stores/undo'
import { navUser, tableSchema } from '../../test/navigation'
import MobileCardList from './MobileCardList.vue'

let pinia: ReturnType<typeof createPinia>
let wrapper: VueWrapper | undefined
beforeEach(async () => {
  pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().setUser(navUser)
  await loadLocale('en')
  await loadLocale('zh-CN')
  i18n.global.locale.value = 'en'
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

function provider(total = 1000) {
  return vi.fn(async (_url: string, init?: RequestInit) => {
    const body = JSON.parse(String(init?.body)) as RowQueryRequest
    const offset = body.offset ?? 0
    const fieldIds = body.fieldIds ?? [
      'extra0',
      'extra1',
      'extra2',
      'extra3',
      'extra4',
      'extra5',
    ]
    return Response.json({
      rows: Array.from(
        { length: Math.max(0, Math.min(100, total - offset)) },
        (_, index) => ({
          id: String(9007199254740993n + BigInt(offset + index)),
          fields: Object.fromEntries(
            fieldIds.map((id) => [
              id,
              id === 'name' ? `Title ${offset + index}` : `Value ${id}`,
            ]),
          ),
        }),
      ),
      fieldIds,
      total: body.includeCount ? total : null,
      schemaVersion: 1,
    })
  })
}
function schemaOf(): TableSchema {
  const schema = tableSchema()
  for (let index = 0; index < 6; index++)
    schema.fields.push({
      ...schema.fields[0]!,
      id: `extra${index}`,
      name: `Extra ${index}`,
      position: 2048 + index,
    })
  return schema
}
async function setup(fetcher = provider(), schema = schemaOf()) {
  vi.stubGlobal('fetch', fetcher)
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }],
  })
  await router.push('/t/t/v/v')
  wrapper = mount(MobileCardList, {
    props: { schema, viewId: 'v', identity: navUser.id },
    global: { plugins: [pinia, i18n, router] },
    attachTo: document.body,
  })
  await flushPromises()
  return { wrapper, router, fetcher }
}

describe('bounded mobile pages', () => {
  let mounted: Awaited<ReturnType<typeof setup>>

  beforeEach(async () => {
    mounted = await setup()
  })

  it('renders hidden display values plus four visible fields, exact IDs and no grid, with bounded pages', async () => {
    const { wrapper, fetcher } = mounted
    expect(wrapper.findAll('.mobile-card')).toHaveLength(100)
    expect(wrapper.get('.card-title').text()).toBe('Title 0')
    expect(
      wrapper.findAll('.mobile-card')[0]!.findAll('.card-field'),
    ).toHaveLength(4)
    expect(wrapper.findAll('.mobile-card')[0]!.text()).not.toContain('extra4')
    expect(wrapper.find('revo-grid').exists()).toBe(false)
    await wrapper.get('.mobile-card').trigger('click')
    expect(wrapper.emitted('expand')).toEqual([['9007199254740993']])
    for (let index = 0; index < 5; index++) {
      await wrapper.findAll('.mobile-paging button')[1]!.trigger('click')
      await flushPromises()
      expect(
        Number(wrapper.get('.mobile-rows').attributes('data-cached-rows')),
      ).toBeLessThanOrEqual(300)
      expect(wrapper.findAll('.mobile-card')).toHaveLength(100)
    }
    expect(wrapper.get('.card-title').text()).toBe('Title 500')
    expect(
      fetcher.mock.calls.every(
        ([, init]) => JSON.parse(String(init?.body)).limit === 100,
      ),
    ).toBe(true)
    await wrapper.findAll('.mobile-paging button')[0]!.trigger('click')
    await flushPromises()
    expect(wrapper.get('.card-title').text()).toBe('Title 400')
    await wrapper.get('.mobile-add').trigger('click')
    expect(wrapper.emitted('add')).toHaveLength(1)
    i18n.global.locale.value = 'zh-CN'
    await flushPromises()
    expect(wrapper.get('.mobile-toolbar h2').text()).toBe('数据行')
  })
})

it('offers native bounded options for real field visibility without filter/group/highlight controls', async () => {
  const { wrapper, fetcher } = await setup()
  expect(wrapper.vm.$.exposed?.exportQuery()).toEqual({ groupBy: [] })
  await wrapper.get('.mobile-toolbar button').trigger('click')
  expect(wrapper.get('dialog.mobile-sheet').attributes('open')).toBeDefined()
  expect(wrapper.findAll('.field-choice')).toHaveLength(7)
  await wrapper.get('.field-choice input').setValue(true)
  await flushPromises()
  const bodies = fetcher.mock.calls.map(
    ([, init]) => JSON.parse(String(init?.body)) as RowQueryRequest,
  )
  expect(bodies.at(-1)?.fieldIds).toContain('name')
  expect(wrapper.vm.$.exposed?.exportQuery().fieldIds).toContain('name')
  expect(wrapper.get('dialog.mobile-sheet').attributes('open')).toBeDefined()
  expect(wrapper.text()).not.toMatch(/Filter|Group|Highlight/)
})

it('keeps grouped mobile visibility and exported overrides in view order', async () => {
  const schema = schemaOf()
  schema.views[0]!.config = {
    fields: [
      'extra1',
      'name',
      'extra0',
      'extra2',
      'extra3',
      'extra4',
      'extra5',
    ].map((id) => ({ id, visible: ['extra1', 'name', 'extra2'].includes(id) })),
    frozen: 0,
    groupBy: ['extra1'],
  }
  const rows = provider(1)
  const fetcher = vi.fn(async (url: string, init?: RequestInit) =>
    url.endsWith('/groups')
      ? Response.json({
          groups: [{ key: 'G', count: 1, aggregates: [] }],
          schemaVersion: 1,
        })
      : rows(url, init),
  )
  const { wrapper } = await setup(fetcher, schema)
  await wrapper.get('.mobile-toolbar button').trigger('click')
  const choices = wrapper.findAll('.field-choice input')
  expect((choices[0]!.element as HTMLInputElement).checked).toBe(true)
  expect((choices[1]!.element as HTMLInputElement).checked).toBe(false)
  expect((choices[2]!.element as HTMLInputElement).checked).toBe(true)
  expect((choices[3]!.element as HTMLInputElement).checked).toBe(true)
  await choices[3]!.setValue(false)
  await flushPromises()
  expect(wrapper.vm.$.exposed?.exportQuery()).toEqual({
    fieldIds: ['extra1', 'name'],
    groupBy: ['extra1'],
  })
})

it('keeps empty/query failures honest and retries explicitly', async () => {
  const empty = await setup(provider(0))
  expect(empty.wrapper.findAll('.mobile-card')).toHaveLength(0)
  expect(empty.wrapper.text()).toContain('No rows yet')
  empty.wrapper.unmount()
  const fetcher = provider()
  fetcher.mockRejectedValueOnce(new TypeError('secret'))
  const { wrapper } = await setup(fetcher)
  expect(wrapper.get('[role="alert"]').text()).not.toContain('secret')
  expect(wrapper.findAll('.mobile-card')).toHaveLength(0)
  await wrapper.get('[role="alert"] button').trigger('click')
  await flushPromises()
  expect(wrapper.findAll('.mobile-card')).toHaveLength(100)
})

it.each([
  [401, 'ERR_UNAUTHENTICATED', '/login'],
  [403, 'ERR_PASSWORD_CHANGE_REQUIRED', '/change-password'],
  [403, 'ERR_PERMISSION_DENIED', '/t/t/v/v'],
  [404, 'ERR_NOT_FOUND', '/t/t/v/v'],
  [409, 'ERR_SCHEMA_CHANGED', '/t/t/v/v'],
])(
  'clears content on %s/%s and uses existing auth/schema boundaries',
  async (status, code, path) => {
    vi.spyOn(useNavStore(), 'refresh').mockResolvedValue(true)
    const { wrapper, router } = await setup(
      vi.fn(async () => Response.json({ error: { code } }, { status })),
    )
    expect(wrapper.findAll('.mobile-card')).toHaveLength(0)
    expect(wrapper.get('[role="alert"]').text()).not.toBe('')
    expect(router.currentRoute.value.path).toBe(path)
  },
)

it('has no creation/selection/mutation controls for readers', async () => {
  const schema = schemaOf()
  schema.myLevel = 'read'
  const { wrapper } = await setup(provider(), schema)
  expect(wrapper.find('.mobile-add').exists()).toBe(false)
  expect(wrapper.find('.card-selection').exists()).toBe(false)
})

it.each(['view', 'schema', 'identity', 'unmount'])(
  'ignores late page results after %s changes even when fetch ignores abort',
  async (change) => {
    let release!: (response: Response) => void
    const fetcher = provider()
    fetcher.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          release = resolve
        }),
    )
    const { wrapper } = await setup(fetcher)
    if (change === 'view') await wrapper.setProps({ viewId: 'v2' })
    else if (change === 'schema')
      await wrapper.setProps({ schema: { ...schemaOf(), schemaVersion: 2 } })
    else if (change === 'identity')
      useAuthStore().setUser({ ...navUser, id: 'other' })
    else wrapper.unmount()
    release(
      Response.json({
        rows: [{ id: '1', fields: { name: 'Late secret' } }],
        total: 1,
        fieldIds: ['name'],
        schemaVersion: 1,
      }),
    )
    await flushPromises()
    expect(wrapper.text()).not.toContain('Late secret')
  },
)

it('deletes only confirmed selected page rows and does not invent success on failure', async () => {
  const fetcher = provider()
  const delegate = fetcher.getMockImplementation()!
  fetcher.mockImplementation(async (url, init) =>
    init?.method === 'DELETE'
      ? Response.json(
          { error: { code: 'ERR_PERMISSION_DENIED' } },
          { status: 403 },
        )
      : delegate(url, init),
  )
  vi.spyOn(useNavStore(), 'refresh').mockResolvedValue(true)
  const { wrapper } = await setup(fetcher)
  await wrapper.get('.card-selection input').setValue(true)
  await wrapper.get('.mobile-toolbar button').trigger('click')
  await wrapper.findAll('dialog.mobile-sheet button')[2]!.trigger('click')
  expect(fetcher.mock.calls.some(([, init]) => init?.method === 'DELETE')).toBe(
    false,
  )
  await wrapper
    .findAll('dialog.mobile-sheet')[1]!
    .findAll('button')[0]!
    .trigger('click')
  await flushPromises()
  expect(wrapper.get('[role="alert"]').text()).not.toBe('')
  expect(wrapper.findAll('.mobile-card')).toHaveLength(0)
  expect(useUndoStore().canUndo).toBe(false)
})

it('records the authoritative snapshot for a successful mobile deletion', async () => {
  const fetcher = provider()
  const delegate = fetcher.getMockImplementation()!
  const deleted = { id: '9007199254740993', fields: { name: 'Title 0' } }
  fetcher.mockImplementation(async (url, init) =>
    init?.method === 'DELETE'
      ? Response.json({ rows: [deleted], eventId: 'event' })
      : delegate(url, init),
  )
  const { wrapper } = await setup(fetcher)
  await wrapper.get('.card-selection input').setValue(true)
  await wrapper.get('.mobile-toolbar button').trigger('click')
  await wrapper.findAll('dialog.mobile-sheet button')[2]!.trigger('click')
  await wrapper
    .findAll('dialog.mobile-sheet')[1]!
    .findAll('button')[0]!
    .trigger('click')
  await flushPromises()
  expect(useUndoStore().undoStack).toEqual([
    { steps: [{ kind: 'restore', tableId: 't', rows: [deleted] }] },
  ])
})

it.each(['view', 'schema', 'identity', 'unmount'])(
  'ignores a late delete after %s changes',
  async (change) => {
    let release!: (response: Response) => void
    const fetcher = provider()
    const delegate = fetcher.getMockImplementation()!
    fetcher.mockImplementation(async (url, init) => {
      if (init?.method === 'DELETE')
        return new Promise<Response>((resolve) => {
          release = resolve
        })
      return delegate(url, init)
    })
    const { wrapper } = await setup(fetcher)
    await wrapper.get('.card-selection input').setValue(true)
    await wrapper.get('.mobile-toolbar button').trigger('click')
    await wrapper.findAll('dialog.mobile-sheet button')[2]!.trigger('click')
    await wrapper
      .findAll('dialog.mobile-sheet')[1]!
      .findAll('button')[0]!
      .trigger('click')
    if (change === 'view') await wrapper.setProps({ viewId: 'other' })
    else if (change === 'schema')
      await wrapper.setProps({ schema: { ...schemaOf(), schemaVersion: 2 } })
    else if (change === 'identity')
      useAuthStore().setUser({ ...navUser, id: 'other' })
    else wrapper.unmount()
    await flushPromises()
    const reads = fetcher.mock.calls.length
    release(Response.json({ rows: [], eventId: 'event' }))
    await flushPromises()
    expect(fetcher).toHaveBeenCalledTimes(reads)
    expect(wrapper.find('dialog[open]').exists()).toBe(false)
  },
)
