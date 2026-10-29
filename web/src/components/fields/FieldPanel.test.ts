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
    await dialog.get('select').setValue('single_select')
    await buttonByText(wrapper, 'Add option').trigger('click')
    await buttonByText(wrapper, 'Add option').trigger('click')
    const labels = dialog.findAll('.field-choice input')
    await labels[0]!.setValue('Todo')
    await labels[1]!.setValue('Doing')
    await dialog.findAll('.field-choice select')[1]!.setValue('blue')
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
        color: 'gray',
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
    await buttonByText(wrapper, 'Add formula').trigger('click')
    const dialog = wrapper
      .findAll('dialog.field-dialog')
      .find((item) => item.attributes('aria-label') === 'Formula field')!
    await dialog.find('input').setValue('Double quantity')
    const expression = dialog.findAll('textarea')[1]!
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
    await buttonByText(wrapper, 'Add formula').trigger('click')
    const dialog = wrapper
      .findAll('dialog.field-dialog')
      .find((item) => item.attributes('aria-label') === 'Formula field')!
    await dialog.findAll('textarea')[1]!.setValue('{Qty} +')
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
    expect(
      (dialog.findAll('textarea')[1]!.element as HTMLTextAreaElement).value,
    ).toBe('{Units} * 2')
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
    await row(wrapper, 'qty').findAll('button')[0]!.trigger('click')
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

it('rejects an invalid default locally with a localized reason and sends nothing', async () => {
  const { calls } = stubFetch()
  const wrapper = await setup()
  try {
    await row(wrapper, 'qty').findAll('button')[0]!.trigger('click')
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
    await row(wrapper, 'qty').findAll('button')[0]!.trigger('click')
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

it('localizes preflight counts and validation reasons in both locales without server text', async () => {
  stubFetch(
    failure(409, 'ERR_REQUIRED_HAS_EMPTY', { count: 3 }),
    failure(400, 'ERR_VALIDATION', { reason: 'storage_change' }),
  )
  const wrapper = await setup()
  try {
    await row(wrapper, 'status').findAll('button')[0]!.trigger('click')
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
    const qty = row(wrapper, 'qty')
    expect(buttonByText(wrapper, 'Move up').exists()).toBe(true)
    await qty
      .findAll('button')
      .find((b) => b.text() === 'Move down')!
      .trigger('click')
    await flushPromises()
    await qty
      .findAll('button')
      .find((b) => b.text() === 'Use as display field')!
      .trigger('click')
    await flushPromises()
    expect(wrapper.emitted('reload')).toHaveLength(1)
    expect(wrapper.get('[role="status"]').text()).toBe('Display field updated.')
    await qty
      .findAll('button')
      .find((b) => b.text() === 'Delete')!
      .trigger('click')
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
    await buttonByText(wrapper, 'Add relation').trigger('click')
    const dialog = wrapper.get('dialog[aria-label="Create relation"]')
    await flushPromises()
    expect(
      dialog
        .get('select')
        .findAll('option')
        .map((option) => option.attributes('value')),
    ).toEqual(['t'])
    await dialog.get('input').setValue('Manager')
    await dialog.findAll('input')[1]!.setValue('Reports')
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
    await buttonByText(wrapper, 'Add lookup').trigger('click')
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
    const qty = row(wrapper, 'qty')
    await qty
      .findAll('button')
      .find((b) => b.text() === 'Move down')!
      .trigger('click')
    await flushPromises()
    expect(wrapper.emitted('reload')).toHaveLength(1)
    expect(fetch).toHaveBeenCalledWith('/api/v1/nav', expect.anything())
    expect(wrapper.get('[role="alert"]').text()).toContain('permission')
    await qty
      .findAll('button')
      .find((b) => b.text() === 'Move down')!
      .trigger('click')
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
  await row(wrapper, 'qty')
    .findAll('button')
    .find((b) => b.text() === 'Move down')!
    .trigger('click')
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
