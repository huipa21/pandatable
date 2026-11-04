/* eslint-disable vue/one-component-per-file -- inline DataGrid/FieldPanel stubs */
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { defineComponent, h, type PropType } from 'vue'
import type { TableSchema, TableView } from '../api/types'
import { i18n, loadLocale } from '../i18n'
import { useAuthStore } from '../stores/auth'
import { useNavStore } from '../stores/nav'
import { navigation, navUser, tableSchema } from '../test/navigation'
import { ThemeMedia } from '../test/setup'
import ResourcePage from './ResourcePage.vue'

let pinia: ReturnType<typeof createPinia>

beforeEach(async () => {
  pinia = createPinia()
  setActivePinia(pinia)
  await loadLocale('en')
  i18n.global.locale.value = 'en'
  useAuthStore().setUser(navUser)
  const nav = useNavStore()
  nav.tree = navigation()
  nav.loaded = true
  const media = new ThemeMedia()
  media.matches = true
  vi.stubGlobal('matchMedia', () => media)
})

afterEach(() => {
  disposePinia(pinia)
  vi.unstubAllGlobals()
})

it('keeps a newer saved view when a delayed schema refresh has an older view version', async () => {
  const initial = tableSchema()
  const newerView: TableView = {
    ...initial.views[0]!,
    version: 3,
    config: {
      ...initial.views[0]!.config,
      fields: [{ id: 'name', visible: false, width: 250 }],
    },
  }
  const delayed: TableSchema = {
    ...initial,
    schemaVersion: initial.schemaVersion + 1,
    views: [{ ...initial.views[0]!, version: 2 }],
  }
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValueOnce(Response.json(initial))
      .mockResolvedValueOnce(Response.json(delayed)),
  )
  const grid = defineComponent({
    props: {
      schema: { type: Object as PropType<TableSchema>, required: true },
    },
    emits: ['viewUpdated'],
    setup(props, { emit }) {
      return () =>
        h('section', [
          h(
            'button',
            {
              'data-testid': 'save-view',
              onClick: () => emit('viewUpdated', newerView),
            },
            'Save',
          ),
          h(
            'button',
            {
              'data-testid': 'stale-view',
              onClick: () => emit('viewUpdated', delayed.views[0]),
            },
            'Older response',
          ),
          h('span', { 'data-testid': 'view-version' }, [
            String(props.schema.views[0]?.version),
            ':',
            String(props.schema.views[0]?.config.fields[0]?.visible),
          ]),
        ])
    },
  })
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/t/:tableId/v/:viewId', component: ResourcePage }],
  })
  await router.push('/t/t/v/v')
  const wrapper = mount(ResourcePage, {
    global: { plugins: [pinia, i18n, router], stubs: { DataGrid: grid } },
  })
  try {
    await flushPromises()
    await wrapper.get('[data-testid="save-view"]').trigger('click')
    await wrapper.get('[data-testid="stale-view"]').trigger('click')
    useNavStore().revision++
    await flushPromises()
    expect(wrapper.get('[data-testid="view-version"]').text()).toBe('3:false')
  } finally {
    wrapper.unmount()
  }
})

it.each(['flush', 'save'] as const)(
  'blocks grid interaction and rejects a different view after navigation during %s',
  async (phase) => {
    const schema = tableSchema()
    schema.views.push({ ...schema.views[0]!, id: 'v2', name: 'Other' })
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => Response.json(schema)),
    )
    let release!: (saved: boolean) => void
    const deferred = () =>
      new Promise<boolean>((resolve) => {
        release = resolve
      })
    const flushColumnChanges = vi.fn(
      phase === 'flush' ? deferred : async () => true,
    )
    const reorderFields = vi.fn(phase === 'save' ? deferred : async () => true)
    const result = vi.fn()
    const grid = defineComponent({
      emits: ['toggleFields'],
      setup(_props, { emit, expose }) {
        expose({ flushColumnChanges, reorderFields })
        return () =>
          h('section', { 'data-testid': 'grid' }, [
            h(
              'button',
              {
                class: 'grid-fields-trigger',
                onClick: () => emit('toggleFields'),
              },
              'Fields',
            ),
          ])
      },
    })
    const panel = defineComponent({
      props: {
        prepareReorder: {
          type: Function as PropType<() => Promise<boolean>>,
          required: true,
        },
        persistReorder: {
          type: Function as PropType<(ids: string[]) => Promise<boolean>>,
          required: true,
        },
        finishReorder: {
          type: Function as PropType<() => void>,
          required: true,
        },
      },
      setup(props) {
        return () =>
          h(
            'button',
            {
              'data-testid': 'move-field',
              onClick: async () => {
                try {
                  if (await props.prepareReorder())
                    result(await props.persistReorder(['name']))
                } finally {
                  props.finishReorder()
                }
              },
            },
            'Move',
          )
      },
    })
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/t/:tableId/v/:viewId', component: ResourcePage }],
    })
    await router.push('/t/t/v/v')
    const wrapper = mount(ResourcePage, {
      global: {
        plugins: [pinia, i18n, router],
        stubs: { DataGrid: grid, FieldPanel: panel },
      },
    })
    try {
      await flushPromises()
      await wrapper.get('.grid-fields-trigger').trigger('click')
      await vi.dynamicImportSettled()
      await flushPromises()
      await wrapper.get('[data-testid="move-field"]').trigger('click')
      expect(flushColumnChanges).toHaveBeenCalledTimes(1)
      expect(
        wrapper.get('[data-testid="grid"]').attributes('inert'),
      ).toBeDefined()
      await flushPromises()
      await router.push('/t/t/v/v2')
      await flushPromises()
      release(true)
      await flushPromises()
      if (phase === 'flush') {
        expect(reorderFields).not.toHaveBeenCalled()
        expect(result).not.toHaveBeenCalled()
      } else {
        expect(reorderFields).toHaveBeenCalledWith(['name'])
        expect(result).toHaveBeenCalledWith(false)
      }
      expect(
        wrapper.get('[data-testid="grid"]').attributes('inert'),
      ).toBeUndefined()
    } finally {
      wrapper.unmount()
    }
  },
)
