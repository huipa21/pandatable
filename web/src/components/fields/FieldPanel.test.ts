import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { i18n, loadLocale } from '../../i18n'
import { useAuthStore } from '../../stores/auth'
import { navigation, navUser, tableSchema } from '../../test/navigation'
import type { TableField, TableSchema } from '../../api/types'
import FieldPanel from './FieldPanel.vue'
import { draftFrom, patchBody } from './fieldForm'

let pinia: ReturnType<typeof createPinia>
beforeEach(async () => {
  pinia = createPinia()
  setActivePinia(pinia)
  await loadLocale('en')
  await loadLocale('zh-CN')
  i18n.global.locale.value = 'en'
  useAuthStore().setUser(navUser)
  HTMLDialogElement.prototype.showModal = function () {
    this.open = true
  }
  HTMLDialogElement.prototype.close = function () {
    this.open = false
  }
})
afterEach(() => {
  disposePinia(pinia)
  vi.unstubAllGlobals()
  i18n.global.locale.value = 'en'
})

const field = (patch: Partial<TableField>): TableField => ({
  id: 'f',
  name: 'Field',
  description: '',
  uiType: 'text',
  options: {},
  isSystem: false,
  isRequired: false,
  isUnique: false,
  default: null,
  position: 1024,
  version: 1,
  ...patch,
})

function schema(level: TableSchema['myLevel'] = 'owner'): TableSchema {
  const base = tableSchema()
  return {
    ...base,
    myLevel: level,
    fields: [
      base.fields[0]!,
      field({
        id: 'qty',
        name: 'Qty',
        uiType: 'integer',
        options: { size: 'int', thousandSep: true },
        isRequired: true,
        position: 2048,
        version: 3,
      }),
      field({
        id: 'status',
        name: 'Status',
        uiType: 'single_select',
        options: {
          choices: [
            { id: 'a', label: 'Open', color: 'green' },
            { id: 'b', label: 'Done', color: 'gray' },
          ],
        },
        default: 'Open',
        position: 3072,
        version: 2,
      }),
      field({
        id: 'sys',
        name: 'Created time',
        uiType: 'created_at',
        isSystem: true,
        position: 0,
      }),
    ],
  }
}

type Call = [string, RequestInit]
function stubFetch(...responses: Response[]) {
  const fetch = vi.fn((url: string) => {
    if (url === '/api/v1/nav')
      return Promise.resolve(Response.json(navigation()))
    const next = responses.shift()
    return next
      ? Promise.resolve(next)
      : Promise.reject(new TypeError('offline'))
  })
  vi.stubGlobal('fetch', fetch)
  return {
    fetch,
    calls: () =>
      (fetch.mock.calls as unknown as Call[]).filter(
        ([url]) => url !== '/api/v1/nav',
      ),
  }
}

const failure = (status: number, code: string, details?: object) =>
  Response.json(
    { error: { code, message: 'server text', details } },
    { status },
  )

async function setup(value = schema()) {
  const wrapper = mount(FieldPanel, {
    props: { schema: value },
    global: { plugins: [pinia, i18n] },
    attachTo: document.body,
  })
  await flushPromises()
  return wrapper
}

const buttonByText = (
  wrapper: Awaited<ReturnType<typeof setup>>,
  text: string,
) => wrapper.findAll('button').find((button) => button.text() === text)!

const row = (wrapper: Awaited<ReturnType<typeof setup>>, id: string) =>
  wrapper.get(`[data-field-id="${id}"]`)

async function fieldAction(
  wrapper: Awaited<ReturnType<typeof setup>>,
  id: string,
  action: string,
) {
  const tile = row(wrapper, id)
  await tile.get('summary').trigger('click')
  const button = tile
    .findAll('.field-menu button')
    .find((item) => item.text() === action)
  expect(button, `${action} action`).toBeDefined()
  await button!.trigger('click')
  await flushPromises()
}

async function selectEditType(
  wrapper: Awaited<ReturnType<typeof setup>>,
  id: string,
  kind: string,
) {
  await fieldAction(wrapper, id, 'Edit')
  const editor = wrapper.get('dialog[aria-label="Edit field"]')
  await editor.get('.field-type-selected').trigger('click')
  await editor.get(`[data-kind="${kind}"]`).trigger('click')
  await flushPromises()
}

async function openType(
  wrapper: Awaited<ReturnType<typeof setup>>,
  kind: string,
) {
  await buttonByText(wrapper, 'Add field').trigger('click')
  await wrapper.get(`[data-kind="${kind}"]`).trigger('click')
  await flushPromises()
}

it('creates and edits a checkbox icon without changing legacy options', async () => {
  const next = schema()
  next.fields.push(
    field({
      id: 'check',
      name: 'Approved',
      uiType: 'checkbox',
      options: { icon: 'heart' },
      position: 4096,
    }),
  )
  const { calls } = stubFetch(
    Response.json({
      field: next.fields.at(-1),
      schema: next,
      eventId: 'checkbox-created',
    }),
    Response.json({
      field: { ...next.fields.at(-1), options: { icon: 'star' } },
      schema: next,
      eventId: 'checkbox-edited',
    }),
  )
  const wrapper = await setup()
  try {
    await openType(wrapper, 'checkbox')
    const create = wrapper.get('dialog[aria-label="New field"]')
    await create.get('input').setValue('Approved')
    const options = create.findAll('[role="radio"]')
    expect(options).toHaveLength(7)
    expect(
      create
        .get('[role="radio"][aria-label="Check"]')
        .attributes('aria-checked'),
    ).toBe('true')
    await create.get('[role="radio"][aria-label="Heart"]').trigger('click')
    expect(
      create
        .get('[role="radio"][aria-label="Heart"]')
        .attributes('aria-checked'),
    ).toBe('true')
    await create.get('form').trigger('submit')
    await flushPromises()
    expect(JSON.parse(String(calls()[0]![1].body))).toMatchObject({
      uiType: 'checkbox',
      options: { icon: 'heart' },
    })

    await wrapper.setProps({ schema: next })
    await fieldAction(wrapper, 'check', 'Edit')
    const edit = wrapper.get('dialog[aria-label="Edit field"]')
    expect(
      edit.get('[role="radio"][aria-label="Heart"]').attributes('aria-checked'),
    ).toBe('true')
    await edit.get('[role="radio"][aria-label="Star"]').trigger('click')
    await edit.get('form').trigger('submit')
    await flushPromises()
    expect(JSON.parse(String(calls()[1]![1].body))).toEqual({
      options: { icon: 'star' },
    })

    const legacy = field({ uiType: 'checkbox', options: {} })
    const draft = draftFrom(legacy, {})
    draft.name = 'Renamed'
    expect(patchBody(legacy, draft, {})).toEqual({
      ok: true,
      value: { name: 'Renamed' },
    })
  } finally {
    wrapper.unmount()
  }
})

it('searches icon-bearing types and carries the name into advanced editors', async () => {
  stubFetch()
  const wrapper = await setup()
  try {
    await buttonByText(wrapper, 'Add field').trigger('click')
    const dialog = wrapper.get('dialog[aria-label="New field"]')
    expect(wrapper.find('.field-extra-actions').exists()).toBe(false)
    expect(dialog.findAll('[data-kind]')).toHaveLength(32)
    expect(dialog.findAll('[data-kind] svg')).toHaveLength(32)
    expect(dialog.find('.field-settings').exists()).toBe(false)
    await dialog.get('input').setValue('Related value')
    await dialog.get('input[type="search"]').setValue('look')
    expect(
      dialog.findAll('[data-kind]').map((item) => item.attributes('data-kind')),
    ).toEqual(['lookup'])
    await dialog.get('[data-kind="lookup"]').trigger('click')
    await flushPromises()
    const lookup = wrapper.get('dialog[aria-label="Create computed field"]')
    expect((lookup.element as HTMLDialogElement).open).toBe(true)
    expect((lookup.get('input').element as HTMLInputElement).value).toBe(
      'Related value',
    )
    expect(lookup.text()).toContain('Create a relation field')
    expect(
      lookup.get('button[type="submit"]').attributes('disabled'),
    ).toBeDefined()
    await lookup.get('.field-type-selected').trigger('click')
    await lookup.get('[data-kind="formula"]').trigger('click')
    await flushPromises()
    const formula = wrapper.get('dialog[aria-label="Formula field"]')
    expect((formula.get('input').element as HTMLInputElement).value).toBe(
      'Related value',
    )
    expect(
      formula.findAll('.formula-suggestions button').length,
    ).toBeGreaterThan(8)
    await formula.get('.field-type-selected').trigger('click')
    await formula.get('[data-kind="text"]').trigger('click')
    await flushPromises()
    expect(dialog.find('.field-type-list').exists()).toBe(false)
    expect(dialog.find('.field-settings').exists()).toBe(true)
    expect((dialog.get('input').element as HTMLInputElement).value).toBe(
      'Related value',
    )
  } finally {
    wrapper.unmount()
  }
})

it('lists fields in position order with badges and gives readers no mutation controls', async () => {
  stubFetch()
  const wrapper = await setup(schema('read'))
  try {
    expect(wrapper.findAll('.field-name').map((item) => item.text())).toEqual([
      'Created time',
      'Name',
      'Qty',
      'Status',
    ])
    expect(row(wrapper, 'sys').text()).toContain('System')
    expect(row(wrapper, 'name').text()).toContain('Display field')
    expect(row(wrapper, 'qty').text()).toContain('Required')
    expect(row(wrapper, 'qty').text()).toContain('Integer')
    expect(wrapper.text()).toContain('Only table owners can change fields.')
    expect(wrapper.find('details').exists()).toBe(false)
    expect(wrapper.findAll('button')).toHaveLength(0)
  } finally {
    wrapper.unmount()
  }
})

it('searches and toggles visible fields in the compact menu without exposing system fields by default', async () => {
  stubFetch()
  const wrapper = mount(FieldPanel, {
    props: {
      schema: schema(),
      menu: true,
      columns: [
        { id: 'name', visible: true },
        { id: 'qty', visible: true },
        { id: 'status', visible: false },
        { id: 'sys', visible: false },
      ],
    },
    global: { plugins: [pinia, i18n] },
    attachTo: document.body,
  })
  try {
    expect(wrapper.find('[data-field-id="sys"]').exists()).toBe(false)
    await wrapper.get('input[type="search"]').setValue('qty')
    expect(wrapper.findAll('.field-name').map((item) => item.text())).toEqual([
      'Qty',
    ])
    await wrapper.get('input[aria-label="Show Qty"]').setValue(false)
    expect(wrapper.emitted('visibility')).toEqual([['qty', false]])
    await wrapper.get('input[type="search"]').setValue('')
    await wrapper
      .findAll('button')
      .find((button) => button.text() === 'System fields')!
      .trigger('click')
    expect(wrapper.find('[data-field-id="sys"]').exists()).toBe(true)
    await wrapper.get('input[aria-label="Show Created time"]').setValue(true)
    expect(wrapper.emitted('visibility')?.at(-1)).toEqual(['sys', true])
  } finally {
    wrapper.unmount()
  }
})

it('keeps new-field defaults and descriptions optional and saves their values', async () => {
  const next = { ...schema(), schemaVersion: 2 }
  const { calls } = stubFetch(
    Response.json(
      { field: next.fields[0], schema: next, eventId: 'e' },
      { status: 201 },
    ),
  )
  const wrapper = mount(FieldPanel, {
    props: { schema: schema(), menu: true },
    global: { plugins: [pinia, i18n] },
    attachTo: document.body,
  })
  try {
    await buttonByText(wrapper, 'Add field').trigger('click')
    const dialog = wrapper.get('dialog.field-dialog')
    await dialog.get('[data-kind="long_text"]').trigger('click')
    expect(dialog.get('.field-default').attributes('open')).toBeUndefined()
    expect(
      dialog.get('.field-description-editor').attributes('open'),
    ).toBeUndefined()
    await dialog.get('input').setValue('Notes')
    await dialog.get('.field-default summary').trigger('click')
    await dialog.get('.field-default input').setValue('Default note')
    await dialog.get('.field-description-editor summary').trigger('click')
    await dialog.get('.field-description-editor textarea').setValue('Details')
    await dialog.get('form').trigger('submit')
    await flushPromises()
    expect(JSON.parse(String(calls()[0]![1].body))).toMatchObject({
      name: 'Notes',
      uiType: 'long_text',
      default: 'Default note',
      description: 'Details',
    })
  } finally {
    wrapper.unmount()
  }
})

it('header menu creates and places a new field next to its anchor in the view', async () => {
  const initial = schema()
  const created = field({ id: 'new', name: 'New field', position: 4096 })
  const next = {
    ...initial,
    fields: [...initial.fields, created],
    schemaVersion: 2,
  }
  const { fetch } = stubFetch(
    Response.json({ field: created, schema: next, eventId: 'e' }),
  )
  const prepareReorder = vi.fn().mockResolvedValue(true)
  const persistReorder = vi.fn().mockResolvedValue(true)
  const finishReorder = vi.fn()
  let applySchema: (value: TableSchema) => void = () => {}
  const wrapper = mount(FieldPanel, {
    props: {
      schema: initial,
      menu: true,
      columns: [
        { id: 'name', visible: true },
        { id: 'qty', visible: false },
        { id: 'status', visible: true },
      ],
      prepareReorder,
      persistReorder,
      finishReorder,
      onUpdate: (value: TableSchema) => applySchema(value),
    },
    global: { plugins: [pinia, i18n] },
    attachTo: document.body,
  })
  applySchema = (value) => void wrapper.setProps({ schema: value })
  try {
    await wrapper.vm.$.exposed?.openCreate({
      anchorFieldId: 'name',
      side: 'right',
    })
    const dialog = wrapper.get('dialog.field-dialog')
    await dialog.get('[data-kind="text"]').trigger('click')
    await dialog.get('input').setValue('New field')
    await dialog.get('form').trigger('submit')
    await flushPromises()
    expect(prepareReorder).toHaveBeenCalledOnce()
    expect(prepareReorder.mock.invocationCallOrder[0]).toBeLessThan(
      fetch.mock.invocationCallOrder[0]!,
    )
    expect(persistReorder).toHaveBeenCalledWith([
      'name',
      'new',
      'qty',
      'status',
      'sys',
    ])
    expect(finishReorder).toHaveBeenCalledOnce()
  } finally {
    wrapper.unmount()
  }
})

it('header menu does not create a field if pending view changes cannot be saved', async () => {
  const initial = schema()
  const { calls } = stubFetch()
  const finishReorder = vi.fn()
  const wrapper = mount(FieldPanel, {
    props: {
      schema: initial,
      menu: true,
      columns: initial.fields.map((item) => ({ id: item.id, visible: true })),
      prepareReorder: vi.fn().mockResolvedValue(false),
      persistReorder: vi.fn().mockResolvedValue(true),
      finishReorder,
    },
    global: { plugins: [pinia, i18n] },
    attachTo: document.body,
  })
  try {
    await wrapper.vm.$.exposed?.openCreate({
      anchorFieldId: 'name',
      side: 'right',
    })
    const dialog = wrapper.get('dialog.field-dialog')
    await dialog.get('[data-kind="text"]').trigger('click')
    await dialog.get('input').setValue('New field')
    await dialog.get('form').trigger('submit')
    await flushPromises()
    expect(calls()).toHaveLength(0)
    expect(dialog.get('[role="alert"]').text()).toContain(
      'pending view changes',
    )
    expect(finishReorder).toHaveBeenCalledOnce()
  } finally {
    wrapper.unmount()
  }
})

it('header menu duplicates only field schema and reports a failed view placement', async () => {
  const initial = schema()
  const original = initial.fields.find((item) => item.id === 'status')!
  original.description = 'Stages'
  const copy = {
    ...original,
    id: 'copy',
    name: 'Status (copy)',
    position: 4096,
  }
  const next = {
    ...initial,
    fields: [...initial.fields, copy],
    schemaVersion: 2,
  }
  const { calls } = stubFetch(
    Response.json({ field: copy, schema: next, eventId: 'e' }),
  )
  let applySchema: (value: TableSchema) => void = () => {}
  const wrapper = mount(FieldPanel, {
    props: {
      schema: initial,
      menu: true,
      columns: initial.fields.map((item) => ({ id: item.id, visible: true })),
      prepareReorder: vi.fn().mockResolvedValue(true),
      persistReorder: vi.fn().mockResolvedValue(false),
      finishReorder: vi.fn(),
      onUpdate: (value: TableSchema) => applySchema(value),
    },
    global: { plugins: [pinia, i18n] },
    attachTo: document.body,
  })
  applySchema = (value) => void wrapper.setProps({ schema: value })
  try {
    await wrapper.vm.$.exposed?.duplicate(original.id)
    await flushPromises()
    expect(JSON.parse(String(calls()[0]![1].body))).toEqual({
      name: 'Status (copy)',
      description: 'Stages',
      uiType: 'single_select',
      options: original.options,
    })
    expect(wrapper.get('[role="alert"]').text()).toContain(
      'field was created, but its position could not be saved',
    )
  } finally {
    wrapper.unmount()
  }
})

it('header menu duplicates a formula using its current referenced field names', async () => {
  const initial = schema()
  initial.fields[1]!.name = 'Units'
  const formula = field({
    id: 'formula',
    name: 'Double quantity',
    uiType: 'formula',
    options: {
      expr: '{Qty} * 2',
      ast: {
        kind: 'binary',
        children: [{ kind: 'field', fieldId: 'qty', position: 1 }],
      },
    },
    position: 4096,
  })
  initial.fields.push(formula)
  const copy = { ...formula, id: 'copy', name: 'Double quantity (copy)' }
  const next = {
    ...initial,
    fields: [...initial.fields, copy],
    schemaVersion: 2,
  }
  const { calls } = stubFetch(
    Response.json({ field: copy, schema: next, eventId: 'e' }),
  )
  let applySchema: (value: TableSchema) => void = () => {}
  const wrapper = mount(FieldPanel, {
    props: {
      schema: initial,
      menu: true,
      columns: initial.fields.map((item) => ({ id: item.id, visible: true })),
      prepareReorder: vi.fn().mockResolvedValue(true),
      persistReorder: vi.fn().mockResolvedValue(true),
      finishReorder: vi.fn(),
      onUpdate: (value: TableSchema) => applySchema(value),
    },
    global: { plugins: [pinia, i18n] },
    attachTo: document.body,
  })
  applySchema = (value) => void wrapper.setProps({ schema: value })
  try {
    await wrapper.vm.$.exposed?.duplicate(formula.id)
    expect(JSON.parse(String(calls()[0]![1].body)).options).toEqual({
      expr: '{Units} * 2',
    })
  } finally {
    wrapper.unmount()
  }
})

it('header menu opens the field editor focused on its description', async () => {
  const wrapper = await setup()
  try {
    await wrapper.vm.$.exposed?.openEditor('qty', { focus: 'description' })
    const description = wrapper.get(
      'dialog.field-dialog .field-description-editor textarea',
    )
    expect(document.activeElement).toBe(description.element)
    expect(
      (description.element.closest('details') as HTMLDetailsElement).open,
    ).toBe(true)
  } finally {
    wrapper.unmount()
  }
})

it('creates a select field with canonical options and a label default, then applies the returned schema', async () => {
  const next = { ...schema(), schemaVersion: 2 }
  const { calls } = stubFetch(
    Response.json(
      { field: next.fields[0], schema: next, eventId: 'e' },
      { status: 201 },
    ),
  )
  const wrapper = await setup()
  try {
    await buttonByText(wrapper, 'Add field').trigger('click')
    const dialog = wrapper.get('dialog.field-dialog')
    expect((dialog.element as HTMLDialogElement).open).toBe(true)
    await dialog.get('input').setValue('  Stage  ')
    await dialog.get('[data-kind="single_select"]').trigger('click')
    await buttonByText(wrapper, 'Add option').trigger('click')
    await buttonByText(wrapper, 'Add option').trigger('click')
    const labels = dialog.findAll('.field-choice input')
    await labels[0]!.setValue('Todo')
    await labels[1]!.setValue('Doing')
    await dialog.findAll('.choice-color-trigger')[1]!.trigger('click')
    await dialog.get('[role="option"][aria-label="Blue"]').trigger('click')
    const choices = dialog.findAll('.field-choice')
    const defaults = dialog.findAll('select').at(-1)!
    await defaults.setValue(defaults.findAll('option')[2]!.element.value)
    await dialog.get('form').trigger('submit')
    await flushPromises()
    expect(choices).toHaveLength(2)
    const [url, init] = calls()[0]!
    expect(url).toBe('/api/v1/tables/t/fields')
    expect(init.method).toBe('POST')
    expect(new Headers(init.headers).get('If-Match')).toBeNull()
    const body = JSON.parse(String(init.body))
    expect(body).toMatchObject({
      name: 'Stage',
      uiType: 'single_select',
      description: '',
      isRequired: false,
      isUnique: false,
      default: 'Doing',
    })

    expect(body.options.choices).toEqual([
      {
        id: expect.stringMatching(/^[A-Za-z0-9_-]{1,64}$/),
        label: 'Todo',
        color: 'red',
      },
      {
        id: expect.stringMatching(/^[A-Za-z0-9_-]{1,64}$/),
        label: 'Doing',
        color: 'blue',
      },
    ])
    expect(wrapper.emitted('update')).toEqual([[next]])
    expect((dialog.element as HTMLDialogElement).open).toBe(false)
    expect(wrapper.get('[role="status"]').text()).toBe('Field saved.')
  } finally {
    wrapper.unmount()
  }
})

it.each(['single_select', 'multi_select'])(
  'automatically colors and focuses new %s choice options',
  async (kind) => {
    stubFetch()
    const wrapper = await setup()
    try {
      await openType(wrapper, kind)
      const dialog = wrapper.get('dialog.field-dialog')
      for (let index = 0; index < 3; index++) {
        await buttonByText(wrapper, 'Add option').trigger('click')
        expect(document.activeElement).toBe(
          dialog.findAll('.field-choice input')[index]!.element,
        )
      }
      expect(
        dialog
          .findAll('.choice-color-trigger')
          .map((button) => button.attributes('aria-label')),
      ).toEqual([
        'Option 1 color: Red',
        'Option 2 color: Orange',
        'Option 3 color: Amber',
      ])

      await dialog.get('[aria-label="Remove option 2"]').trigger('click')
      await buttonByText(wrapper, 'Add option').trigger('click')
      expect(
        dialog
          .findAll('.choice-color-trigger')
          .map((button) => button.attributes('aria-label')),
      ).toEqual([
        'Option 1 color: Red',
        'Option 2 color: Amber',
        'Option 3 color: Orange',
      ])
      expect(document.activeElement).toBe(
        dialog.findAll('.field-choice input')[2]!.element,
      )
    } finally {
      wrapper.unmount()
    }
  },
)

it('navigates the choice color palette and closes it on Escape or outside click', async () => {
  stubFetch()
  const wrapper = await setup()
  try {
    await fieldAction(wrapper, 'status', 'Edit')
    const dialog = wrapper.get('dialog.field-dialog')
    const trigger = dialog.findAll('.choice-color-trigger')[0]!
    expect(trigger.attributes('aria-label')).toBe('Option 1 color: Green')
    await trigger.trigger('click')
    const palette = dialog.get('.choice-color-palette')
    expect(palette.attributes('role')).toBe('listbox')
    expect(palette.findAll('[role="option"]')).toHaveLength(13)
    const green = palette.get('[role="option"][aria-label="Green"]')
    expect(green.attributes('aria-selected')).toBe('true')
    expect(document.activeElement).toBe(green.element)

    await green.trigger('keydown', { key: 'ArrowUp' })
    const red = palette.get('[role="option"][aria-label="Red"]')
    expect(document.activeElement).toBe(red.element)
    expect(red.attributes('aria-selected')).toBe('false')
    await red.trigger('keydown', { key: 'ArrowRight' })
    const orange = palette.get('[role="option"][aria-label="Orange"]')
    expect(document.activeElement).toBe(orange.element)
    await orange.trigger('keydown', { key: 'ArrowDown' })
    const teal = palette.get('[role="option"][aria-label="Teal"]')
    expect(document.activeElement).toBe(teal.element)
    await teal.trigger('keydown', { key: 'ArrowLeft' })
    expect(document.activeElement).toBe(green.element)
    await green.trigger('keydown', { key: 'ArrowUp' })
    await red.trigger('keydown', { key: 'Enter' })
    expect(dialog.find('.choice-color-palette').exists()).toBe(false)
    expect(trigger.attributes('aria-label')).toBe('Option 1 color: Red')
    expect(document.activeElement).toBe(trigger.element)

    await trigger.trigger('click')
    await dialog
      .get('[role="option"][aria-label="Red"]')
      .trigger('keydown', { key: 'Escape' })
    expect(dialog.find('.choice-color-palette').exists()).toBe(false)
    expect(document.activeElement).toBe(trigger.element)

    await trigger.trigger('click')
    document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    await flushPromises()
    expect(dialog.find('.choice-color-palette').exists()).toBe(false)

    i18n.global.locale.value = 'zh-CN'
    await flushPromises()
    expect(trigger.attributes('aria-label')).toBe('选项 1 颜色：红色')
    await trigger.trigger('click')
    expect(dialog.find('[role="option"][aria-label="红色"]').exists()).toBe(
      true,
    )
  } finally {
    wrapper.unmount()
  }
})

it('provides click-accessible choice reorder controls and restores focus', async () => {
  stubFetch()
  const wrapper = await setup()
  try {
    await fieldAction(wrapper, 'status', 'Edit')
    const dialog = wrapper.get('dialog.field-dialog')
    const firstHandle = dialog.findAll('.choice-drag-handle')[0]!
    await firstHandle.trigger('click')
    let controls = dialog.get('.choice-move-controls')
    expect(
      controls.get('[aria-label="Move option 1 up"]').attributes('disabled'),
    ).toBeDefined()
    const down = controls.get('[aria-label="Move option 1 down"]')
    expect(document.activeElement).toBe(down.element)
    await down.trigger('click')
    expect(
      dialog
        .findAll('.field-choice')
        .map((choice) => choice.attributes('data-choice-id')),
    ).toEqual(['b', 'a'])
    expect(document.activeElement).toBe(firstHandle.element)

    await firstHandle.trigger('click')
    controls = dialog.get('.choice-move-controls')
    await controls.get('[aria-label="Move option 2 up"]').trigger('click')
    expect(
      dialog
        .findAll('.field-choice')
        .map((choice) => choice.attributes('data-choice-id')),
    ).toEqual(['a', 'b'])
    expect(document.activeElement).toBe(firstHandle.element)

    await firstHandle.trigger('click')
    await dialog
      .get('[aria-label="Move option 1 down"]')
      .trigger('keydown', { key: 'Escape' })
    expect(dialog.find('.choice-move-controls').exists()).toBe(false)
    expect(document.activeElement).toBe(firstHandle.element)
  } finally {
    wrapper.unmount()
  }
})

it('drags choices before a target and ignores no-op drops', async () => {
  stubFetch()
  const wrapper = await setup()
  const transfer = {
    effectAllowed: 'none',
    dropEffect: 'none',
    setData: vi.fn(),
    setDragImage: vi.fn(),
  }
  try {
    await fieldAction(wrapper, 'status', 'Edit')
    const dialog = wrapper.get('dialog.field-dialog')
    await buttonByText(wrapper, 'Add option').trigger('click')
    const last = dialog.findAll('.field-choice')[2]!
    const lastId = last.attributes('data-choice-id')
    await last.get('.choice-drag-handle').trigger('dragstart', {
      dataTransfer: transfer,
    })
    const first = dialog.findAll('.field-choice')[0]!
    await first.trigger('dragover', { dataTransfer: transfer, clientY: -1 })
    expect(first.classes()).toContain('choice-drop-before')
    await first.trigger('drop', { dataTransfer: transfer })
    expect(
      dialog
        .findAll('.field-choice')
        .map((choice) => choice.attributes('data-choice-id')),
    ).toEqual([lastId, 'a', 'b'])

    const middle = dialog.findAll('.field-choice')[1]!
    await middle.get('.choice-drag-handle').trigger('dragstart', {
      dataTransfer: transfer,
    })
    const end = dialog.findAll('.field-choice')[2]!
    await end.trigger('dragover', { dataTransfer: transfer, clientY: -1 })
    expect(end.classes()).not.toContain('choice-drop-before')
    await end.trigger('drop', { dataTransfer: transfer })
    expect(
      dialog
        .findAll('.field-choice')
        .map((choice) => choice.attributes('data-choice-id')),
    ).toEqual([lastId, 'a', 'b'])
  } finally {
    wrapper.unmount()
  }
})

it.each(['keyboard', 'drag'] as const)(
  'saves the chosen color and stable choice IDs after %s reordering',
  async (method) => {
    const next = { ...schema(), schemaVersion: 2 }
    const { calls } = stubFetch(
      Response.json({
        field: next.fields.find((item) => item.id === 'status'),
        schema: next,
        eventId: 'e',
      }),
    )
    const wrapper = await setup()
    try {
      await fieldAction(wrapper, 'status', 'Edit')
      const dialog = wrapper.get('dialog.field-dialog')
      await dialog.findAll('.choice-color-trigger')[0]!.trigger('click')
      await dialog.get('[role="option"][aria-label="Red"]').trigger('click')

      const firstHandle = dialog.findAll('.choice-drag-handle')[0]!
      if (method === 'keyboard') {
        ;(firstHandle.element as HTMLButtonElement).focus()
        await firstHandle.trigger('keydown', { key: 'ArrowDown', altKey: true })
        expect(document.activeElement).toBe(firstHandle.element)
      } else {
        const transfer = {
          effectAllowed: 'none',
          dropEffect: 'none',
          setData: vi.fn(),
          setDragImage: vi.fn(),
        }
        await firstHandle.trigger('dragstart', { dataTransfer: transfer })
        const target = dialog.findAll('.field-choice')[1]!
        await target.trigger('dragover', { dataTransfer: transfer, clientY: 1 })
        expect(target.classes()).toContain('choice-drop-after')
        await target.trigger('drop', { dataTransfer: transfer })
        expect(transfer.setDragImage).toHaveBeenCalled()
      }
      expect(
        dialog
          .findAll('.field-choice')
          .map((choice) => choice.attributes('data-choice-id')),
      ).toEqual(['b', 'a'])
      expect(calls()).toHaveLength(0)
      expect(
        (dialog.get('.field-default select').element as HTMLSelectElement)
          .value,
      ).toBe('a')

      await dialog.get('form').trigger('submit')
      await flushPromises()
      const [url, init] = calls()[0]!
      expect(url).toBe('/api/v1/fields/status')
      expect(init.method).toBe('PATCH')
      expect(new Headers(init.headers).get('If-Match')).toBe('2')
      expect(JSON.parse(String(init.body))).toEqual({
        options: {
          choices: [
            { id: 'b', label: 'Done', color: 'gray' },
            { id: 'a', label: 'Open', color: 'red' },
          ],
        },
      })
    } finally {
      wrapper.unmount()
    }
  },
)

it('creates an ID-bound formula through the editor with field and function suggestions', async () => {
  const next = schema()
  const formulaField = field({
    id: 'formula',
    name: 'Double quantity',
    uiType: 'formula',
    options: { expr: '{Qty} * 2' },
    position: 4096,
  })
  next.fields.push(formulaField)
  const { calls } = stubFetch(
    Response.json({ type: 'number' }),
    Response.json(
      { field: formulaField, schema: next, eventId: 'e' },
      { status: 201 },
    ),
  )
  const wrapper = await setup()
  try {
    await openType(wrapper, 'formula')
    const dialog = wrapper
      .findAll('dialog.field-dialog')
      .find((item) => item.attributes('aria-label') === 'Formula field')!
    await dialog.find('input').setValue('Double quantity')
    const expression = dialog.get('textarea')
    await expression.setValue('CON')
    expect(dialog.text()).toContain('CONCAT(')
    await expression.setValue('{Qty')
    const fieldSuggestion = dialog
      .findAll('button')
      .find((item) => item.text() === 'Qty')!
    await fieldSuggestion.trigger('click')
    expect((expression.element as HTMLTextAreaElement).value).toBe('{Qty}')
    await expression.setValue('{Qty} * 2')
    await dialog.get('form').trigger('submit')
    await flushPromises()
    const [check, create] = calls()
    expect(check![0]).toBe('/api/v1/formula:check')
    expect(JSON.parse(String(check![1].body))).toEqual({
      tableId: 't',
      expr: '{Qty} * 2',
    })
    expect(create![0]).toBe('/api/v1/tables/t/fields')
    expect(JSON.parse(String(create![1].body))).toMatchObject({
      name: 'Double quantity',
      uiType: 'formula',
      options: { expr: '{Qty} * 2' },
    })
    expect(wrapper.emitted('update')).toEqual([[next]])
  } finally {
    wrapper.unmount()
  }
})

it('shows formula validation positions inline without rendering server messages', async () => {
  const { calls } = stubFetch(
    Response.json({ error: { position: 8, message: 'private server detail' } }),
  )
  const wrapper = await setup()
  try {
    await openType(wrapper, 'formula')
    const dialog = wrapper
      .findAll('dialog.field-dialog')
      .find((item) => item.attributes('aria-label') === 'Formula field')!
    await dialog.get('textarea').setValue('{Qty} +')
    await new Promise((resolve) => setTimeout(resolve, 350))
    await flushPromises()
    expect(dialog.get('[role="alert"]').text()).toBe(
      'Invalid formula at position 8.',
    )
    expect(dialog.text()).not.toContain('private server detail')
    expect(calls()).toHaveLength(1)
  } finally {
    wrapper.unmount()
  }
})

it('rebinds the formula editor expression to renamed stable field references', async () => {
  const value = schema()
  value.fields[1] = { ...value.fields[1]!, name: 'Units' }
  value.fields.push(
    field({
      id: 'formula',
      name: 'Double quantity',
      uiType: 'formula',
      options: {
        expr: '{Qty} * 2',
        ast: {
          kind: 'binary',
          value: '*',
          position: 7,
          children: [
            { kind: 'field', value: 'Qty', fieldId: 'qty', position: 1 },
            { kind: 'number', value: '2', position: 9 },
          ],
        },
        fieldIds: ['qty'],
        resultType: 'number',
      },
      position: 4096,
    }),
  )
  stubFetch()
  const wrapper = await setup(value)
  try {
    const formula = row(wrapper, 'formula')
    await formula.get('summary').trigger('click')
    await formula
      .findAll('button')
      .find((button) => button.text() === 'Edit')!
      .trigger('click')
    const dialog = wrapper
      .findAll('dialog.field-dialog')
      .find((item) => item.attributes('aria-label') === 'Formula field')!
    expect((dialog.get('textarea').element as HTMLTextAreaElement).value).toBe(
      '{Units} * 2',
    )
  } finally {
    wrapper.unmount()
  }
})

it('patches only changed members with If-Match and keeps storage settings fixed', async () => {
  const next = schema()
  const { calls } = stubFetch(
    Response.json({ field: next.fields[1], schema: next, eventId: 'e' }),
  )
  const wrapper = await setup()
  try {
    await fieldAction(wrapper, 'qty', 'Edit')
    const dialog = wrapper.get('dialog.field-dialog')
    expect(dialog.find('select[disabled]').exists()).toBe(true)
    expect(dialog.text()).toContain(
      'Storage settings are fixed after creation.',
    )
    await dialog.get('input').setValue('数量')
    const defaultInput = dialog.findAll('input').at(-1)!
    await defaultInput.setValue('42')
    await dialog.get('form').trigger('submit')
    await flushPromises()
    const [url, init] = calls()[0]!
    expect(url).toBe('/api/v1/fields/qty')
    expect(init.method).toBe('PATCH')
    expect(new Headers(init.headers).get('If-Match')).toBe('3')
    expect(new Headers(init.headers).get('X-PT-CSRF')).toBe('1')
    expect(JSON.parse(String(init.body))).toEqual({ name: '数量', default: 42 })
  } finally {
    wrapper.unmount()
  }
})

it('previews a type change and aborts without sending the conversion request', async () => {
  const { calls } = stubFetch(
    Response.json({
      total: 3,
      convertible: 2,
      failing: 1,
      samples: [{ id: '4', value: 'x' }],
    }),
  )
  const wrapper = await setup()
  try {
    await selectEditType(wrapper, 'qty', 'text')
    const dialog = wrapper
      .findAll('dialog.field-dialog')
      .find((item) => item.attributes('aria-label') === 'Change field type')!
    await dialog
      .findAll('button')
      .find((button) => button.text() === 'Preview conversion')!
      .trigger('click')
    await flushPromises()
    expect(dialog.text()).toContain(
      '3 non-empty values: 2 convertible, 1 failing.',
    )
    expect(dialog.text()).toContain('Row 4: x')
    await dialog
      .findAll('button')
      .find((button) => button.text() === 'Abort')!
      .trigger('click')
    const request = calls()[0]!
    expect(request[0]).toBe('/api/v1/fields/qty/convert:dry-run')
    expect(new Headers(request[1].headers).get('If-Match')).toBe('3')
    expect(JSON.parse(String(request[1].body))).toEqual({ uiType: 'text' })
    expect(calls()).toHaveLength(1)
  } finally {
    wrapper.unmount()
  }
})

it('converts only after preview and explicitly selects NULL for failing values', async () => {
  const next = schema()
  next.fields[1] = {
    ...next.fields[1]!,
    uiType: 'text',
    isRequired: false,
    version: 4,
  }
  const { calls } = stubFetch(
    Response.json({
      total: 3,
      convertible: 2,
      failing: 1,
      samples: [{ id: '4', value: 'x' }],
    }),
    Response.json({
      field: next.fields[1],
      schema: next,
      eventId: 'type-event',
    }),
  )
  const wrapper = await setup()
  try {
    await selectEditType(wrapper, 'qty', 'text')
    const dialog = wrapper
      .findAll('dialog.field-dialog')
      .find((item) => item.attributes('aria-label') === 'Change field type')!
    await dialog
      .findAll('button')
      .find((button) => button.text() === 'Preview conversion')!
      .trigger('click')
    await flushPromises()
    expect(dialog.text()).toContain(
      'Required will be turned off so failed values can become NULL.',
    )
    await dialog
      .findAll('button')
      .find(
        (button) => button.text() === 'Convert, set failing values to NULL',
      )!
      .trigger('click')
    await flushPromises()
    const conversion = calls()[1]!
    expect(conversion[0]).toBe('/api/v1/fields/qty/convert')
    expect(conversion[1].method).toBe('POST')
    expect(new Headers(conversion[1].headers).get('If-Match')).toBe('3')
    expect(JSON.parse(String(conversion[1].body))).toEqual({
      uiType: 'text',
      setFailingToNull: true,
    })
    expect(wrapper.emitted('update')?.[0]?.[0]).toEqual(next)
  } finally {
    wrapper.unmount()
  }
})

it('rejects an invalid default locally with a localized reason and sends nothing', async () => {
  const { calls } = stubFetch()
  const wrapper = await setup()
  try {
    await fieldAction(wrapper, 'qty', 'Edit')
    const dialog = wrapper.get('dialog.field-dialog')
    await dialog.findAll('input').at(-1)!.setValue('12.5')
    await dialog.get('form').trigger('submit')
    await flushPromises()
    expect(calls()).toHaveLength(0)
    expect(dialog.get('[role="alert"]').text()).toMatch(/^Default value: /)
  } finally {
    wrapper.unmount()
  }
})

it('reloads on a version conflict and reseeds the open editor from the newer field', async () => {
  const { calls } = stubFetch(failure(409, 'ERR_VERSION_CONFLICT'))
  const wrapper = await setup()
  try {
    await fieldAction(wrapper, 'qty', 'Edit')
    const dialog = wrapper.get('dialog.field-dialog')
    await dialog.get('input').setValue('Count')
    await dialog.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.emitted('reload')).toHaveLength(1)
    expect(dialog.get('[role="alert"]').text()).toContain('changed elsewhere')
    const newer = schema()
    newer.fields[1] = { ...newer.fields[1]!, name: 'Units', version: 4 }
    await wrapper.setProps({ schema: newer })
    expect((dialog.get('input').element as HTMLInputElement).value).toBe(
      'Units',
    )
    expect(calls()).toHaveLength(1)
  } finally {
    wrapper.unmount()
  }
})

it.each(['removed', 'reloaded'] as const)(
  'clears choice reorder controls when a conflicting field is %s',
  async (change) => {
    stubFetch(failure(409, 'ERR_VERSION_CONFLICT'))
    const wrapper = await setup()
    try {
      await fieldAction(wrapper, 'status', 'Edit')
      const dialog = wrapper.get('dialog.field-dialog')
      await dialog.findAll('.field-choice input')[0]!.setValue('Opened')
      await dialog.get('form').trigger('submit')
      await flushPromises()
      expect(wrapper.emitted('reload')).toHaveLength(1)

      await dialog.findAll('.choice-drag-handle')[0]!.trigger('click')
      expect(dialog.find('.choice-move-controls').exists()).toBe(true)
      const newer = schema()
      if (change === 'removed')
        newer.fields = newer.fields.filter((item) => item.id !== 'status')
      else {
        const status = newer.fields.find((item) => item.id === 'status')!
        status.options = {
          choices: [{ id: 'b', label: 'Done', color: 'gray' }],
        }
      }
      await wrapper.setProps({ schema: newer })
      expect(dialog.find('.choice-move-controls').exists()).toBe(false)
      const escape = new KeyboardEvent('keydown', {
        key: 'Escape',
        bubbles: true,
        cancelable: true,
      })
      document.dispatchEvent(escape)
      expect(escape.defaultPrevented).toBe(false)
      expect((dialog.element as HTMLDialogElement).open).toBe(
        change === 'reloaded',
      )
    } finally {
      wrapper.unmount()
    }
  },
)

it('localizes preflight counts and validation reasons in both locales without server text', async () => {
  stubFetch(
    failure(409, 'ERR_REQUIRED_HAS_EMPTY', { count: 3 }),
    failure(400, 'ERR_VALIDATION', { reason: 'storage_change' }),
  )
  const wrapper = await setup()
  try {
    await fieldAction(wrapper, 'status', 'Edit')
    const dialog = wrapper.get('dialog.field-dialog')
    const required = dialog
      .findAll('label.field-check')
      .find((label) => label.text() === 'Required')!
    await required.get('input').setValue(true)
    expect(dialog.text()).toContain(
      'Fill existing empty values with the default',
    )
    await dialog.get('form').trigger('submit')
    await flushPromises()
    expect(dialog.get('[role="alert"]').text()).toBe(
      '3 existing rows are empty. Fill them first or choose a default to fill them.',
    )
    i18n.global.locale.value = 'zh-CN'
    await dialog.get('form').trigger('submit')
    await flushPromises()
    expect(dialog.get('[role="alert"]').text()).toBe(
      '存储设置在创建后不可更改。',
    )
    expect(wrapper.text()).not.toContain('server text')
  } finally {
    wrapper.unmount()
  }
})

it('moves, sets the display field and deletes with the field version', async () => {
  const next = schema()
  const { calls } = stubFetch(
    Response.json({ field: next.fields[1], schema: next, eventId: 'e' }),
    Response.json({ table: next.table, eventId: 'e' }),
    Response.json({ schema: next, eventId: 'e' }),
  )
  const wrapper = await setup()
  try {
    expect(buttonByText(wrapper, 'Move up').exists()).toBe(true)
    await fieldAction(wrapper, 'qty', 'Move down')
    await fieldAction(wrapper, 'qty', 'Edit')
    const editor = wrapper.get('dialog[aria-label="Edit field"]')
    await editor.get('.field-advanced-settings summary').trigger('click')
    await editor
      .findAll('button')
      .find((button) => button.text() === 'Use as display field')!
      .trigger('click')
    await flushPromises()
    expect(wrapper.emitted('reload')).toHaveLength(1)
    expect(wrapper.get('[role="status"]').text()).toBe('Display field updated.')
    await editor
      .findAll('button')
      .find((button) => button.text() === 'Cancel')!
      .trigger('click')
    await fieldAction(wrapper, 'qty', 'Delete')
    const remover = wrapper.findAll('dialog.field-dialog').at(-1)!
    expect(remover.text()).toContain('“Qty”')
    await remover.get('form').trigger('submit')
    await flushPromises()
    const [move, display, remove] = calls()
    expect(move![0]).toBe('/api/v1/fields/qty')
    expect(JSON.parse(String(move![1].body))).toEqual({ index: 2 })
    expect(new Headers(move![1].headers).get('If-Match')).toBe('3')
    expect(display![0]).toBe('/api/v1/tables/t')
    expect(JSON.parse(String(display![1].body))).toEqual({
      displayFieldId: 'qty',
    })
    expect(remove![1].method).toBe('DELETE')
    expect(new Headers(remove![1].headers).get('If-Match')).toBe('3')
    expect(wrapper.get('[role="status"]').text()).toBe('Field deleted.')
    expect(wrapper.emitted('update')).toHaveLength(2)
    expect(row(wrapper, 'sys').text()).not.toContain('Delete')
    expect(row(wrapper, 'name').text()).not.toContain('Use as display field')
  } finally {
    wrapper.unmount()
  }
})

it('creates a same-book relation from the owner field panel', async () => {
  const next = schema()
  const relationField = field({
    id: 'relation-field',
    name: 'Reports',
    uiType: 'link',
    options: { relationId: 'relation-id', targetTableId: 't', side: 'parent' },
    position: 4096,
  })
  next.fields.push(relationField)
  next.relations = [
    {
      id: 'relation-id',
      kind: 'one_many',
      parentTableId: 't',
      childTableId: 't',
      parentFieldId: 'relation-field',
      childFieldId: 'reverse-field',
      onDelete: 'set_null',
    },
  ]
  const { calls } = stubFetch(
    Response.json(
      {
        relation: next.relations[0],
        field: relationField,
        schema: next,
        eventId: 'relation-event',
      },
      { status: 201 },
    ),
  )
  const wrapper = await setup()
  try {
    await openType(wrapper, 'link')
    const dialog = wrapper.get('dialog[aria-label="Create relation"]')
    await flushPromises()
    expect(
      dialog
        .get('select')
        .findAll('option')
        .map((option) => option.attributes('value')),
    ).toEqual(['t'])
    await dialog.get('input').setValue('Manager')
    await dialog.get('.field-advanced-settings input').setValue('Reports')
    await dialog.get('select').setValue('t')
    await dialog.get('form').trigger('submit')
    await flushPromises()
    const [url, init] = calls()[0]!
    expect(url).toBe('/api/v1/tables/t/relations')
    expect(init.method).toBe('POST')
    expect(JSON.parse(String(init.body))).toEqual({
      name: 'Manager',
      reverseName: 'Reports',
      targetTableId: 't',
      kind: 'one_many',
      sourceSide: 'child',
      onDelete: 'set_null',
    })
    expect(wrapper.emitted('update')?.[0]?.[0]).toEqual(next)
  } finally {
    wrapper.unmount()
  }
})

it.each([
  ['many_many', 'many_many', 'parent'],
  ['one_many', 'one_many', 'parent'],
  ['many_one', 'one_many', 'child'],
  ['one_one', 'one_one', 'parent'],
])(
  'maps the %s relation choice to the existing API',
  async (choice, kind, sourceSide) => {
    const { calls } = stubFetch(
      Response.json({ schema: schema(), eventId: 'relation-event' }),
    )
    const wrapper = await setup()
    try {
      await openType(wrapper, 'link')
      const dialog = wrapper.get('dialog[aria-label="Create relation"]')
      await dialog.get('input').setValue('Related records')
      await dialog.get(`input[type="radio"][value="${choice}"]`).setValue(true)
      await dialog.get('form').trigger('submit')
      await flushPromises()
      expect(JSON.parse(String(calls()[0]![1].body))).toMatchObject({
        kind,
        sourceSide,
        reverseName: 'Table',
      })
    } finally {
      wrapper.unmount()
    }
  },
)

it('configures a lookup from an existing same-book relation', async () => {
  const next = schema()
  const relationField = field({
    id: 'relation-field',
    name: 'Project',
    uiType: 'link',
    options: {},
    position: 4096,
  })
  const lookup = field({
    id: 'lookup-field',
    name: 'Project name',
    uiType: 'lookup',
    options: { relationFieldId: 'relation-field', targetFieldId: 'name' },
    position: 5120,
  })
  next.fields.push(relationField)
  next.relations = [
    {
      id: 'relation-id',
      kind: 'one_many',
      parentTableId: 't',
      childTableId: 't',
      parentFieldId: 'relation-field',
      childFieldId: 'reverse-field',
      onDelete: 'set_null',
    },
  ]
  const created = { ...next, fields: [...next.fields, lookup] }
  const { calls } = stubFetch(
    Response.json(next),
    Response.json({ field: lookup, schema: created, eventId: 'lookup-event' }),
  )
  const wrapper = await setup(next)
  try {
    await openType(wrapper, 'lookup')
    const dialog = wrapper.get('dialog[aria-label="Create computed field"]')
    await flushPromises()
    await dialog.get('input').setValue('Project name')
    await dialog.findAll('select')[1]!.setValue('name')
    await dialog.get('form').trigger('submit')
    await flushPromises()
    expect(calls().map(([url]) => url)).toEqual([
      '/api/v1/tables/t/schema',
      '/api/v1/tables/t/fields',
    ])
    expect(JSON.parse(String(calls()[1]![1].body))).toEqual({
      name: 'Project name',
      uiType: 'lookup',
      options: { relationFieldId: 'relation-field', targetFieldId: 'name' },
    })
    expect(wrapper.emitted('update')?.[0]?.[0]).toEqual(created)
  } finally {
    wrapper.unmount()
  }
})

it('reconciles permission loss and uncertain outcomes with an authoritative reload', async () => {
  const { fetch } = stubFetch(failure(403, 'ERR_PERMISSION_DENIED'))
  const wrapper = await setup()
  try {
    await fieldAction(wrapper, 'qty', 'Move down')
    await flushPromises()
    expect(wrapper.emitted('reload')).toHaveLength(1)
    expect(fetch).toHaveBeenCalledWith('/api/v1/nav', expect.anything())
    expect(wrapper.get('[role="alert"]').text()).toContain('permission')
    await fieldAction(wrapper, 'qty', 'Move down')
    await flushPromises()
    expect(wrapper.emitted('reload')).toHaveLength(2)
  } finally {
    wrapper.unmount()
  }
})

it('aborts an in-flight mutation on unmount', async () => {
  let signal: AbortSignal | undefined
  vi.stubGlobal(
    'fetch',
    vi.fn((_url: string, init: RequestInit) => {
      signal = init.signal ?? undefined
      return new Promise(() => {})
    }),
  )
  const wrapper = await setup()
  await fieldAction(wrapper, 'qty', 'Move down')
  wrapper.unmount()
  expect(signal?.aborted).toBe(true)
})

it('keeps select defaults across label edits by choice id', () => {
  const status = schema().fields[2]!
  const draft = draftFrom(status, {})
  draft.choices[0]!.label = 'Opened'
  const body = patchBody(status, draft, {})
  expect(body).toEqual({
    ok: true,
    value: {
      options: {
        choices: [
          { id: 'a', label: 'Opened', color: 'green' },
          { id: 'b', label: 'Done', color: 'gray' },
        ],
      },
      default: 'Opened',
    },
  })
})

it.each([
  {
    uiType: 'text_array',
    options: {},
    default: ['value, with a comma', ' padded '],
  },
  { uiType: 'time', options: { format: '12h' }, default: '13:30:00' },
  { uiType: 'date', options: { dateFormat: 'us' }, default: '2026-03-16' },
  {
    uiType: 'datetime',
    options: {
      dateFormat: 'eu',
      timeFormat: '12h',
      timezone: 'fixed',
      fixedTz: 'UTC',
    },
    default: '2026-03-16T13:30:00.123456Z',
  },
  {
    uiType: 'currency',
    options: { currency: 'USD', scale: 2 },
    default: '12.30',
  },
])('does not rewrite an unchanged $uiType default on rename', (spec) => {
  const original = field(spec)
  const draft = draftFrom(original, { timeZone: 'UTC', locale: 'en' })
  draft.name = 'Renamed'
  expect(patchBody(original, draft, { timeZone: 'UTC', locale: 'en' })).toEqual(
    {
      ok: true,
      value: { name: 'Renamed' },
    },
  )
})

it('edits text-array defaults containing commas without losing elements', () => {
  const original = field({
    uiType: 'text_array',
    default: ['value, with a comma', ' padded '],
  })
  const draft = draftFrom(original, {})
  expect(draft.defaultArrayJson).toBe(true)
  expect(draft.defaultText).toBe('["value, with a comma"," padded "]')
  draft.defaultText = '["value, with a comma","new"]'
  expect(patchBody(original, draft, {})).toEqual({
    ok: true,
    value: { default: ['value, with a comma', 'new'] },
  })
  draft.defaultText = '[invalid]'
  expect(patchBody(original, draft, {}).ok).toBe(false)
})

it('preserves a time field format when editing metadata', async () => {
  const value = schema()
  value.fields.push(
    field({
      id: 'time',
      name: 'Time',
      uiType: 'time',
      options: { format: '12h' },
      default: '13:30:00',
      position: 4096,
    }),
  )
  const { calls } = stubFetch(
    Response.json({ field: value.fields.at(-1), schema: value, eventId: 'e' }),
  )
  const wrapper = await setup(value)
  try {
    await fieldAction(wrapper, 'time', 'Edit')
    const editor = wrapper.get('dialog[open]')
    const format = editor
      .findAll('label')
      .find((label) => label.text().includes('Time format'))!
      .get('select')
    expect((format.element as HTMLSelectElement).value).toBe('12h')
    await editor.get('input').setValue('Meeting time')
    await editor.get('form').trigger('submit')
    await flushPromises()
    expect(JSON.parse(String(calls()[0]![1].body))).toEqual({
      name: 'Meeting time',
    })
  } finally {
    wrapper.unmount()
  }
})

it('opens one anchored action menu, dismisses it, and restores keyboard focus', async () => {
  const { calls } = stubFetch()
  const wrapper = mount(FieldPanel, {
    props: { schema: schema(), menu: true },
    global: { plugins: [pinia, i18n] },
    attachTo: document.body,
  })
  try {
    const first = row(wrapper, 'name').get('details')
    const second = row(wrapper, 'qty').get('details')
    expect(first.attributes('open')).toBeUndefined()
    expect(first.get('summary').attributes('aria-expanded')).toBe('false')
    expect(first.text()).not.toContain('Change type')
    await first.get('summary').trigger('click')
    expect(first.attributes('open')).toBeDefined()
    expect(first.get('summary').attributes('aria-expanded')).toBe('true')
    await second.get('summary').trigger('click')
    expect(first.attributes('open')).toBeUndefined()
    expect(second.attributes('open')).toBeDefined()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(second.attributes('open')).toBeUndefined()
    expect(document.activeElement).toBe(second.get('summary').element)
    await second.get('summary').trigger('click')
    document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    await flushPromises()
    expect(second.attributes('open')).toBeUndefined()
    await wrapper.get('input[aria-label="Show Qty"]').setValue(false)
    expect(wrapper.emitted('visibility')).toEqual([['qty', false]])
    expect(wrapper.find('.field-tile-pressed').exists()).toBe(false)
    expect(calls()).toHaveLength(0)
  } finally {
    wrapper.unmount()
  }
})

it('shows press feedback and reorders filtered visible slots without moving hidden slots', async () => {
  const value = schema()
  value.fields = [
    value.fields[0]!,
    field({ id: 'a', name: 'Alpha first', position: 2048 }),
    field({ id: 'hidden', name: 'Hidden', position: 3072 }),
    field({ id: 'c', name: 'Alpha third', position: 4096 }),
    field({ id: 'other', name: 'Other', position: 5120 }),
    field({
      id: 'sys',
      name: 'Created time',
      uiType: 'created_at',
      isSystem: true,
      position: 1_000_000_000,
    }),
  ]
  const next = {
    ...value,
    schemaVersion: value.schemaVersion + 1,
    fields: value.fields.map((item) =>
      item.id === 'a' ? { ...item, position: 4500, version: 2 } : item,
    ),
  }
  const { calls } = stubFetch(
    Response.json({
      field: next.fields.find((item) => item.id === 'a'),
      schema: next,
      eventId: 'e',
    }),
  )
  const wrapper = mount(FieldPanel, {
    props: {
      schema: value,
      menu: true,
      columns: value.fields.map((item) => ({
        id: item.id,
        visible: item.id !== 'hidden' && !item.isSystem,
      })),
    },
    global: { plugins: [pinia, i18n] },
    attachTo: document.body,
  })
  const transfer = {
    effectAllowed: 'none',
    dropEffect: 'none',
    setData: vi.fn(),
    setDragImage: vi.fn(),
  }
  try {
    await wrapper.get('input[type="search"]').setValue('Alpha')
    const first = row(wrapper, 'a')
    const last = row(wrapper, 'c')
    const handle = first.get('.field-drag-handle')
    const down = new MouseEvent('pointerdown', { bubbles: true, button: 0 })
    Object.defineProperties(down, {
      pointerId: { value: 7 },
      isPrimary: { value: true },
    })
    handle.element.dispatchEvent(down)
    await flushPromises()
    expect(first.classes()).toContain('field-tile-pressed')
    const cancel = new Event('pointercancel')
    Object.defineProperty(cancel, 'pointerId', { value: 7 })
    window.dispatchEvent(cancel)
    await flushPromises()
    expect(first.classes()).not.toContain('field-tile-pressed')

    await handle.trigger('dragstart', { dataTransfer: transfer })
    expect(first.classes()).toContain('field-tile-dragging')
    await last.trigger('dragover', { dataTransfer: transfer, clientY: -1 })
    await last.trigger('drop', { dataTransfer: transfer })
    await flushPromises()
    expect(calls()).toHaveLength(0)
    expect(wrapper.emitted('reorder')).toBeUndefined()

    await handle.trigger('dragstart', { dataTransfer: transfer })
    await last.trigger('dragover', { dataTransfer: transfer, clientY: 1 })
    expect(last.classes()).toContain('field-drop-after')
    await last.trigger('drop', { dataTransfer: transfer })
    await flushPromises()
    expect(transfer.setDragImage).toHaveBeenCalled()
    expect(JSON.parse(String(calls()[0]![1].body))).toEqual({ index: 3 })
    expect(wrapper.emitted('update')?.[0]?.[0]).toEqual(next)
    expect(wrapper.emitted('reorder')).toEqual([
      [['name', 'c', 'hidden', 'a', 'other', 'sys']],
    ])
    expect(wrapper.get('.field-status').text()).toBe(
      'Moved Alpha first to position 2 of 2.',
    )
    expect(wrapper.find('.field-tile-dragging').exists()).toBe(false)
    expect(wrapper.find('.field-drop-after').exists()).toBe(false)
  } finally {
    wrapper.unmount()
  }
})

it('reorders the active view without a no-op schema write when their orders differ', async () => {
  const { calls } = stubFetch()
  const wrapper = mount(FieldPanel, {
    props: {
      schema: schema(),
      menu: true,
      columns: [
        { id: 'status', visible: true },
        { id: 'qty', visible: true },
        { id: 'name', visible: true },
        { id: 'sys', visible: false },
      ],
    },
    global: { plugins: [pinia, i18n] },
    attachTo: document.body,
  })
  try {
    await fieldAction(wrapper, 'status', 'Move down')
    expect(calls()).toHaveLength(0)
    expect(wrapper.emitted('update')).toBeUndefined()
    expect(wrapper.emitted('reorder')).toEqual([
      [['qty', 'status', 'name', 'sys']],
    ])
  } finally {
    wrapper.unmount()
  }
})

it('waits for pending view changes before moving and releases the guard on failure', async () => {
  const { calls } = stubFetch()
  let release!: (saved: boolean) => void
  const prepareReorder = vi.fn(
    () =>
      new Promise<boolean>((resolve) => {
        release = resolve
      }),
  )
  const persistReorder = vi.fn(async () => true)
  const finishReorder = vi.fn()
  const wrapper = mount(FieldPanel, {
    props: {
      schema: schema(),
      menu: true,
      columns: [
        { id: 'name', visible: true },
        { id: 'qty', visible: true },
        { id: 'status', visible: true },
        { id: 'sys', visible: false },
      ],
      prepareReorder,
      persistReorder,
      finishReorder,
    },
    global: { plugins: [pinia, i18n] },
    attachTo: document.body,
  })
  try {
    await fieldAction(wrapper, 'name', 'Move down')
    expect(prepareReorder).toHaveBeenCalledTimes(1)
    expect(
      row(wrapper, 'qty').get('input[type="checkbox"]').attributes('disabled'),
    ).toBeDefined()
    expect(calls()).toHaveLength(0)
    release(false)
    await flushPromises()
    expect(calls()).toHaveLength(0)
    expect(persistReorder).not.toHaveBeenCalled()
    expect(finishReorder).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[role="alert"]').text()).toBe(
      'Save or retry pending view changes before moving fields.',
    )
    expect(wrapper.get('[role="status"]').text()).toBe('')
    expect(
      row(wrapper, 'qty').get('input[type="checkbox"]').attributes('disabled'),
    ).toBeUndefined()
  } finally {
    wrapper.unmount()
  }
})

it('rechecks the order after pending view changes finish', async () => {
  const { calls } = stubFetch()
  let release!: (saved: boolean) => void
  const prepareReorder = vi.fn(
    () =>
      new Promise<boolean>((resolve) => {
        release = resolve
      }),
  )
  const persistReorder = vi.fn(async () => true)
  const finishReorder = vi.fn()
  const wrapper = mount(FieldPanel, {
    props: {
      schema: schema(),
      menu: true,
      columns: [
        { id: 'name', visible: true },
        { id: 'qty', visible: true },
        { id: 'status', visible: true },
      ],
      prepareReorder,
      persistReorder,
      finishReorder,
    },
    global: { plugins: [pinia, i18n] },
    attachTo: document.body,
  })
  try {
    await fieldAction(wrapper, 'name', 'Move down')
    await wrapper.setProps({
      columns: [
        { id: 'qty', visible: true },
        { id: 'name', visible: true },
        { id: 'status', visible: true },
      ],
    })
    release(true)
    await flushPromises()
    expect(calls()).toHaveLength(0)
    expect(persistReorder).not.toHaveBeenCalled()
    expect(finishReorder).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[role="alert"]').text()).toBe(
      'Field order changed. Try moving the field again.',
    )
  } finally {
    wrapper.unmount()
  }
})

it('surfaces a failed view-order save rather than announcing a completed move', async () => {
  const next = schema()
  const { calls } = stubFetch(
    Response.json({ field: next.fields[0], schema: next, eventId: 'e' }),
  )
  const prepareReorder = vi.fn(async () => true)
  const persistReorder = vi.fn(async () => false)
  const finishReorder = vi.fn()
  const wrapper = mount(FieldPanel, {
    props: {
      schema: schema(),
      menu: true,
      columns: [
        { id: 'name', visible: true },
        { id: 'qty', visible: true },
        { id: 'status', visible: true },
        { id: 'sys', visible: false },
      ],
      prepareReorder,
      persistReorder,
      finishReorder,
    },
    global: { plugins: [pinia, i18n] },
    attachTo: document.body,
  })
  try {
    await fieldAction(wrapper, 'name', 'Move down')
    expect(calls()).toHaveLength(1)
    expect(persistReorder).toHaveBeenCalledWith([
      'qty',
      'name',
      'status',
      'sys',
    ])
    expect(finishReorder).toHaveBeenCalledTimes(1)
    expect(wrapper.emitted('update')).toEqual([[next]])
    expect(wrapper.get('[role="alert"]').text()).toBe(
      'The field order could not be saved in this view. Retry the view save, or reload and try again.',
    )
    expect(wrapper.get('[role="status"]').text()).toBe('')
  } finally {
    wrapper.unmount()
  }
})

it('does not discard an edited draft when a different type is chosen', async () => {
  const { calls } = stubFetch()
  const wrapper = await setup()
  try {
    await fieldAction(wrapper, 'qty', 'Edit')
    const editor = wrapper.get('dialog[aria-label="Edit field"]')
    await editor.get('input').setValue('Unsaved name')
    await editor.get('.field-type-selected').trigger('click')
    await editor.get('[data-kind="text"]').trigger('click')
    await flushPromises()
    expect(editor.get('[role="alert"]').text()).toBe(
      'Save or discard field changes before changing its type.',
    )
    expect(
      (
        wrapper.get('dialog[aria-label="Change field type"]')
          .element as HTMLDialogElement
      ).open,
    ).toBe(false)
    expect((editor.get('input').element as HTMLInputElement).value).toBe(
      'Unsaved name',
    )
    expect(calls()).toHaveLength(0)
  } finally {
    wrapper.unmount()
  }
})

it('edits relation metadata in its creation layout without changing its definition', async () => {
  const value = schema()
  value.fields.push(
    field({
      id: 'link',
      name: 'Manager',
      description: 'Existing description',
      uiType: 'link',
      options: { relationId: 'r', targetTableId: 't', side: 'child' },
      position: 4096,
    }),
    field({
      id: 'reverse',
      name: 'Reports',
      uiType: 'link',
      options: { relationId: 'r', targetTableId: 't', side: 'parent' },
      position: 5120,
    }),
  )
  value.relations = [
    {
      id: 'r',
      kind: 'one_many',
      parentTableId: 't',
      childTableId: 't',
      parentFieldId: 'reverse',
      childFieldId: 'link',
      onDelete: 'restrict',
    },
  ]
  const { calls } = stubFetch(
    Response.json({ field: value.fields[4], schema: value, eventId: 'e' }),
  )
  const wrapper = await setup(value)
  try {
    await fieldAction(wrapper, 'link', 'Edit')
    const editor = wrapper.get('dialog[open]')
    expect(
      editor.get('.field-type-selected').attributes('disabled'),
    ).toBeDefined()
    expect(
      editor.get('input[type="radio"][value="many_one"]').element,
    ).toHaveProperty('checked', true)
    expect(
      (editor.get('.field-advanced-settings input').element as HTMLInputElement)
        .value,
    ).toBe('Reports')
    expect(editor.text()).toContain(
      'This field configuration cannot be changed after creation.',
    )
    expect(editor.text()).not.toContain('Use as display field')
    await editor.get('input:not([type="radio"])').setValue('Supervisor')
    await editor.get('textarea').setValue('Updated description')
    await editor.get('form').trigger('submit')
    await flushPromises()
    expect(calls()[0]![0]).toBe('/api/v1/fields/link')
    expect(JSON.parse(String(calls()[0]![1].body))).toEqual({
      name: 'Supervisor',
      description: 'Updated description',
    })
  } finally {
    wrapper.unmount()
  }
})

it('prefills computed settings as read-only while allowing metadata edits', async () => {
  const value = schema()
  value.fields.push(
    field({
      id: 'link',
      name: 'Project',
      uiType: 'link',
      options: { relationId: 'r', targetTableId: 't', side: 'child' },
      position: 4096,
    }),
    field({
      id: 'lookup',
      name: 'Project quantity',
      description: 'Original',
      uiType: 'lookup',
      options: { relationFieldId: 'link', targetFieldId: 'qty' },
      position: 5120,
    }),
  )
  value.relations = [
    {
      id: 'r',
      kind: 'one_many',
      parentTableId: 't',
      childTableId: 't',
      parentFieldId: 'reverse',
      childFieldId: 'link',
      onDelete: 'set_null',
    },
  ]
  const { calls } = stubFetch(
    Response.json(value),
    Response.json({ field: value.fields.at(-1), schema: value, eventId: 'e' }),
  )
  const wrapper = await setup(value)
  try {
    await fieldAction(wrapper, 'lookup', 'Edit')
    const editor = wrapper.get('dialog[open]')
    const selectors = editor.findAll('.field-computed-columns select')
    expect(selectors.map((item) => item.attributes('disabled'))).toEqual([
      '',
      '',
    ])
    expect((selectors[0]!.element as HTMLSelectElement).value).toBe('link')
    expect((selectors[1]!.element as HTMLSelectElement).value).toBe('qty')
    expect((editor.get('textarea').element as HTMLTextAreaElement).value).toBe(
      'Original',
    )
    await editor.get('input').setValue('Project units')
    await editor.get('textarea').setValue('Revised')
    await editor.get('form').trigger('submit')
    await flushPromises()
    expect(calls().map(([url]) => url)).toEqual([
      '/api/v1/tables/t/schema',
      '/api/v1/fields/lookup',
    ])
    expect(JSON.parse(String(calls()[1]![1].body))).toEqual({
      name: 'Project units',
      description: 'Revised',
    })
  } finally {
    wrapper.unmount()
  }
})
