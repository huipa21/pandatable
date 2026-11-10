import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { i18n, loadLocale } from '../../i18n'
import type { ImportAnalysis, ImportJob } from '../../api/types'
import { tableSchema } from '../../test/navigation'
import ImportDialog from './ImportDialog.vue'

beforeEach(async () => {
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
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

const plain: ImportAnalysis = {
  format: 'xlsx',
  hasHeader: true,
  sampledRows: 1000,
  columns: [
    { index: 0, name: 'Name', uiType: 'text', options: {}, examples: ['甲'] },
    {
      index: 1,
      name: 'Amount',
      uiType: 'integer',
      options: { size: 'big' },
      examples: ['1'],
    },
  ],
}

function fetchImport(analysis: ImportAnalysis, job: ImportJob) {
  return vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.body instanceof FormData)
      return Response.json({ id: 'job-1', analysis }, { status: 201 })
    if (init?.method === 'POST')
      return Response.json({ id: 'job-1', status: 'running' }, { status: 202 })
    if (init?.method === 'DELETE') return Response.json({})
    return Response.json(job)
  })
}

const done: ImportJob = {
  id: 'job-1',
  status: 'done',
  result: {
    tableId: 'new-table',
    viewId: 'new-view',
    rows: 3,
    invalidCells: 1,
  },
  hasReport: true,
}

async function upload(wrapper: ReturnType<typeof mount>) {
  const input = wrapper.get<HTMLInputElement>('input[type=file]')
  Object.defineProperty(input.element, 'files', {
    configurable: true,
    value: [
      new File(['data'], 'sample.xlsx', {
        type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
      }),
    ],
  })
  await input.trigger('change')
  await wrapper.get('button').trigger('click')
  await flushPromises()
}

it('analyzes a file, lets the user adjust and skip fields, and downloads the report', async () => {
  const fetcher = fetchImport(plain, done)
  vi.stubGlobal('fetch', fetcher)
  const wrapper = mount(ImportDialog, {
    props: { targetKind: 'book', targetId: 'book-1' },
    global: { plugins: [i18n] },
  })
  try {
    await upload(wrapper)
    expect(wrapper.text()).toContain('XLSX detected; 1000 rows sampled.')
    const columns = wrapper.findAll('.import-column')
    await columns[0]!.get('select').setValue('single_select')
    await columns[0]!.get('textarea').setValue('甲\n乙')
    await columns[1]!.get('input[type=checkbox]').setValue(true)
    await wrapper
      .findAll('button')
      .find((button) => button.text() === 'Start import')!
      .trigger('click')
    await flushPromises()
    const start = fetcher.mock.calls.find(
      ([path, init]) =>
        String(path).endsWith('/imports/job-1') && init?.method === 'POST',
    )?.[1]
    expect(JSON.parse(String(start?.body))).toMatchObject({
      name: 'sample',
      hasHeader: true,
      columns: [
        {
          index: 0,
          uiType: 'single_select',
          options: {
            choices: [
              { id: 'c1', label: '甲', color: 'red' },
              { id: 'c2', label: '乙', color: 'orange' },
            ],
          },
        },
        { index: 1, skip: true },
      ],
    })

    expect(wrapper.text()).toContain('Import complete: 3 rows.')
    expect(wrapper.get('a[download]').attributes('href')).toBe(
      '/api/v1/imports/job-1/issues',
    )
    expect(wrapper.emitted('completed')).toEqual([[done.result]])
    i18n.global.locale.value = 'zh-CN'
    await flushPromises()
    expect(wrapper.text()).toContain('下载无效单元格报告')
    await wrapper
      .findAll('button')
      .find((button) => button.text() === '关闭')!
      .trigger('click')
    await flushPromises()
    expect(wrapper.emitted('close')).toHaveLength(1)
    expect(
      fetcher.mock.calls.some(([, init]) => init?.method === 'DELETE'),
    ).toBe(true)
  } finally {
    wrapper.unmount()
  }
})

it('rejects CSV files before uploading', async () => {
  const fetcher = vi.fn()
  vi.stubGlobal('fetch', fetcher)
  const wrapper = mount(ImportDialog, {
    props: { targetKind: 'book', targetId: 'book-1' },
    global: { plugins: [i18n] },
  })
  try {
    const input = wrapper.get<HTMLInputElement>('input[type=file]')
    expect(input.attributes('accept')).toBe('.xlsx')
    Object.defineProperty(input.element, 'files', {
      configurable: true,
      value: [new File(['data'], 'sample.csv', { type: 'text/csv' })],
    })
    await input.trigger('change')
    expect(wrapper.get('[role="alert"]').text()).toBe(
      'Only Excel (.xlsx) files are supported',
    )
    expect(wrapper.get('button:disabled').text()).toBe('Analyze file')
    expect(fetcher).not.toHaveBeenCalled()
  } finally {
    wrapper.unmount()
  }
})

it('keeps metadata types read-only while allowing inferred types to be changed', async () => {
  const analysis: ImportAnalysis = {
    ...plain,
    metaVersion: 'v2',
    columns: [
      { ...plain.columns[0]!, typeSource: 'meta' },
      { ...plain.columns[1]!, typeSource: 'inferred' },
    ],
  }
  vi.stubGlobal('fetch', fetchImport(analysis, done))
  const wrapper = mount(ImportDialog, {
    props: { targetKind: 'book', targetId: 'book-1' },
    global: { plugins: [i18n] },
  })
  try {
    await upload(wrapper)
    expect(wrapper.text()).toContain('Field types from workbook')
    expect(wrapper.text()).not.toContain('Type for Name')
    expect(wrapper.text()).toContain('Type for Amount')
    expect(wrapper.text()).toContain('Name for Name')
    expect(
      wrapper.find('.import-column input:not([type="checkbox"])').exists(),
    ).toBe(true)
  } finally {
    wrapper.unmount()
  }
})

it('submits editable v2 fallback types without allowing metadata or structural types to change', async () => {
  const analysis: ImportAnalysis = {
    format: 'package',
    metaVersion: 'v2',
    hasHeader: true,
    sampledRows: 1,
    columns: [
      {
        index: 1,
        name: 'Meta',
        uiType: 'text',
        options: {},
        typeSource: 'meta',
      },
      {
        index: 2,
        name: 'Fallback',
        uiType: 'integer',
        options: { size: 'big' },
        typeSource: 'manifest',
      },
      {
        index: 3,
        name: 'Inferred',
        uiType: 'text',
        options: {},
        typeSource: 'inferred',
      },
      {
        index: 4,
        name: 'Files',
        uiType: 'attachment',
        options: {},
        typeSource: 'manifest',
      },
    ],
  }
  const fetcher = fetchImport(analysis, done)
  vi.stubGlobal('fetch', fetcher)
  const wrapper = mount(ImportDialog, {
    props: { targetKind: 'book', targetId: 'book-1' },
    global: { plugins: [i18n] },
  })
  try {
    await upload(wrapper)
    const columns = wrapper.findAll('.import-column')
    expect(columns[0]!.find('select').exists()).toBe(false)
    expect(columns[3]!.find('select').exists()).toBe(false)
    await columns[1]!.get('select').setValue('text')
    await columns[2]!.get('select').setValue('long_text')
    await wrapper
      .findAll('button')
      .find((button) => button.text() === 'Start import')!
      .trigger('click')
    await flushPromises()
    const start = fetcher.mock.calls.find(
      ([path, init]) =>
        String(path).endsWith('/imports/job-1') && init?.method === 'POST',
    )?.[1]
    expect(JSON.parse(String(start?.body)).columns).toMatchObject([
      { index: 1, uiType: 'text' },
      { index: 2, uiType: 'text', options: {} },
      { index: 3, uiType: 'long_text', options: {} },
      { index: 4, uiType: 'attachment' },
    ])
  } finally {
    wrapper.unmount()
  }
})

it('preserves imported fallback choice colors when editing other choices', async () => {
  const analysis: ImportAnalysis = {
    format: 'package',
    metaVersion: 'v2',
    hasHeader: true,
    sampledRows: 1,
    columns: [
      {
        index: 1,
        name: 'Status',
        uiType: 'single_select',
        options: {
          choices: [{ id: 'existing', label: 'Existing', color: 'pink' }],
        },
        typeSource: 'manifest',
      },
    ],
  }
  const fetcher = fetchImport(analysis, done)
  vi.stubGlobal('fetch', fetcher)
  const wrapper = mount(ImportDialog, {
    props: { targetKind: 'book', targetId: 'book-1' },
    global: { plugins: [i18n] },
  })
  try {
    await upload(wrapper)
    await wrapper.get('.import-column textarea').setValue('Existing\nNew')
    await wrapper
      .findAll('button')
      .find((button) => button.text() === 'Start import')!
      .trigger('click')
    await flushPromises()
    const start = fetcher.mock.calls.find(
      ([path, init]) =>
        String(path).endsWith('/imports/job-1') && init?.method === 'POST',
    )?.[1]
    expect(JSON.parse(String(start?.body)).columns[0].options.choices).toEqual([
      { id: 'existing', label: 'Existing', color: 'pink' },
      { id: 'c1', label: 'New', color: 'red' },
    ])
  } finally {
    wrapper.unmount()
  }
})

it('blocks invalid imported choices until the field type changes', async () => {
  const analysis: ImportAnalysis = {
    format: 'package',
    metaVersion: 'v2',
    hasHeader: true,
    sampledRows: 1,
    columns: [
      {
        index: 1,
        name: 'Status',
        uiType: 'single_select',
        options: {
          choices: [{ id: 'existing', label: 'Existing', color: 'invalid' }],
        },
        typeSource: 'manifest',
      },
    ],
  }
  vi.stubGlobal('fetch', fetchImport(analysis, done))
  const wrapper = mount(ImportDialog, {
    props: { targetKind: 'book', targetId: 'book-1' },
    global: { plugins: [i18n] },
  })
  try {
    await upload(wrapper)
    await wrapper.get('.import-column textarea').setValue('Existing\nNew')
    const start = wrapper
      .findAll('button')
      .find((button) => button.text() === 'Start import')!
    expect(start.attributes('disabled')).toBeDefined()
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    await wrapper.get('.import-column select').setValue('text')
    expect(start.attributes('disabled')).toBeUndefined()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
  } finally {
    wrapper.unmount()
  }
})

it('prefills append mappings and reports partially committed failures', async () => {
  const schema = tableSchema()
  const analysis: ImportAnalysis = {
    ...plain,
    columns: [
      { ...plain.columns[0]!, targetFieldId: 'name' },
      { ...plain.columns[1]!, targetFieldId: 'missing' },
    ],
  }
  const failed: ImportJob = {
    id: 'job-1',
    status: 'failed',
    result: { tableId: 't', rows: 5000, invalidCells: 2 },
    errorCode: 'ERR_IMPORT_ROWS_LIMIT',
    hasReport: true,
  }
  const fetcher = fetchImport(analysis, failed)
  vi.stubGlobal('fetch', fetcher)
  const wrapper = mount(ImportDialog, {
    props: { targetKind: 'table', targetId: 't', schema },
    global: { plugins: [i18n] },
  })
  try {
    await upload(wrapper)
    const selects = wrapper.findAll('.import-column select')
    expect((selects[0]!.element as HTMLSelectElement).value).toBe('name')
    expect((selects[1]!.element as HTMLSelectElement).value).toBe('')
    await wrapper
      .findAll('button')
      .find((button) => button.text() === 'Start import')!
      .trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('5000 rows were committed.')
    expect(wrapper.text()).toContain('500,000-row import limit')
    expect(wrapper.emitted('completed')).toEqual([[failed.result]])
    expect(wrapper.find('a[download]').exists()).toBe(true)
  } finally {
    wrapper.unmount()
  }
})

it('maps package append fields without the obsolete package notice', async () => {
  const analysis: ImportAnalysis = {
    ...plain,
    format: 'package',
    columns: [{ ...plain.columns[0]!, index: 1, targetFieldId: 'name' }],
  }
  vi.stubGlobal('fetch', fetchImport(analysis, done))
  const wrapper = mount(ImportDialog, {
    props: { targetKind: 'table', targetId: 't', schema: tableSchema() },
    global: { plugins: [i18n] },
  })
  try {
    await upload(wrapper)
    expect(wrapper.text()).not.toContain(
      'Rows are appended; package fields and shared views are not restored.',
    )
    expect(wrapper.text()).not.toContain('Restored shared views are unlocked.')
    expect(wrapper.text()).not.toContain(
      'Attachment file names mapped to attachment fields cannot be restored',
    )
    i18n.global.locale.value = 'zh-CN'
    await flushPromises()
    expect(wrapper.text()).not.toContain(
      '将追加数据行；不会还原数据包中的字段和共享视图。',
    )
    expect(wrapper.text()).not.toContain('Excel 时间戳可能损失精度。')
  } finally {
    wrapper.unmount()
  }
})

it.each([
  ['finished', done, 'Import complete: 3 rows.'],
  [
    'not started',
    { ...done, status: 'ready' as const },
    'The import did not start. Review the mapping and try again.',
  ],
])(
  'recovers the job status if starting %s loses its response',
  async (_label, status, message) => {
    const fetcher = vi.fn(
      async (_input: RequestInfo | URL, init?: RequestInit) => {
        if (init?.body instanceof FormData)
          return Response.json(
            { id: 'job-1', analysis: plain },
            { status: 201 },
          )
        if (init?.method === 'POST') throw new TypeError('connection lost')
        return Response.json(status)
      },
    )
    vi.stubGlobal('fetch', fetcher)
    const wrapper = mount(ImportDialog, {
      props: { targetKind: 'book', targetId: 'book-1' },
      global: { plugins: [i18n] },
    })
    try {
      await upload(wrapper)
      await wrapper
        .findAll('button')
        .find((button) => button.text() === 'Start import')!
        .trigger('click')
      await flushPromises()
      expect(wrapper.text()).toContain(message)
      expect(wrapper.emitted('completed')?.length ?? 0).toBe(
        status.status === 'done' ? 1 : 0,
      )
    } finally {
      wrapper.unmount()
    }
  },
)

it('offers only eligible package re-linking and keeps other links as text', async () => {
  const analysis: ImportAnalysis = {
    format: 'package',
    tableName: 'Package',
    hasHeader: true,
    sampledRows: 1,
    columns: [
      {
        index: 1,
        name: 'Project',
        uiType: 'link',
        options: {},
        linkTarget: 'Projects',
        relinkTargetId: 'target',
      },
      {
        index: 2,
        name: 'Other',
        uiType: 'link',
        options: {},
        linkTarget: 'Unavailable',
      },
    ],
  }
  const fetcher = fetchImport(analysis, {
    ...done,
    result: {
      ...done.result,
      convertedLinks: ['Other'],
      unavailableViews: ['Computed filter', 'Summary'],
    },
  })
  vi.stubGlobal('fetch', fetcher)
  const wrapper = mount(ImportDialog, {
    props: { targetKind: 'book', targetId: 'b' },
    global: { plugins: [i18n] },
  })
  try {
    await upload(wrapper)
    expect(wrapper.text()).not.toContain(
      'Attachment file names mapped to attachment fields cannot be restored and are reported as invalid; Excel timestamps may lose precision.',
    )
    expect(wrapper.text()).not.toContain('Restored shared views are unlocked.')
    expect(wrapper.text()).toContain(
      'Links imported as text cause dependent lookup, rollup, and formula fields to show errors until repaired.',
    )
    i18n.global.locale.value = 'zh-CN'
    await flushPromises()
    expect(wrapper.text()).not.toContain('还原后的共享视图处于未锁定状态。')
    expect(wrapper.text()).toContain(
      '关联字段转为文本后，依赖它们的查找、汇总和公式字段会显示错误，直到修复。',
    )
    expect(wrapper.text()).not.toContain(
      '映射到附件字段的文件名无法恢复，将报告为无效；Excel 时间戳可能损失精度。',
    )
    i18n.global.locale.value = 'en'
    await flushPromises()
    const columns = wrapper.findAll('.import-column')
    await columns[0]!.get('input[type=checkbox]').setValue(true)
    expect(columns[1]!.text()).toContain('this field will become text')
    await wrapper
      .findAll('button')
      .find((button) => button.text() === 'Start import')!
      .trigger('click')
    await flushPromises()
    const start = fetcher.mock.calls.find(
      ([path, init]) =>
        String(path).endsWith('/imports/job-1') && init?.method === 'POST',
    )?.[1]
    expect(JSON.parse(String(start?.body)).columns).toMatchObject([
      { index: 1, relink: true, skip: false },
      { index: 2, relink: false, skip: false },
    ])
    expect(wrapper.text()).toContain('Imported as text: Other.')
    expect(wrapper.text()).toContain(
      'Views to review: Computed filter, Summary.',
    )
    i18n.global.locale.value = 'zh-CN'
    await flushPromises()
    expect(wrapper.text()).toContain(
      '需要检查的视图：Computed filter, Summary。',
    )
  } finally {
    wrapper.unmount()
  }
})

it('removes the dependency warning when every package link is re-linked', async () => {
  const analysis: ImportAnalysis = {
    format: 'package',
    tableName: 'Package',
    hasHeader: true,
    sampledRows: 1,
    columns: [
      {
        index: 1,
        name: 'Project',
        uiType: 'link',
        options: {},
        linkTarget: 'Projects',
        relinkTargetId: 'target',
      },
    ],
  }
  vi.stubGlobal('fetch', fetchImport(analysis, done))
  const wrapper = mount(ImportDialog, {
    props: { targetKind: 'book', targetId: 'b' },
    global: { plugins: [i18n] },
  })
  try {
    await upload(wrapper)
    const warning = 'Links imported as text cause dependent lookup'
    expect(wrapper.text()).toContain(warning)
    await wrapper.find('.import-column input[type=checkbox]').setValue(true)
    expect(wrapper.text()).not.toContain(warning)
    await wrapper.find('.import-column input[type=checkbox]').setValue(false)
    expect(wrapper.text()).toContain(warning)
  } finally {
    wrapper.unmount()
  }
})
