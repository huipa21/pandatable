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
  format: 'csv',
  encoding: 'gbk',
  delimiter: ',',
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
    value: [new File(['data'], 'sample.csv', { type: 'text/csv' })],
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
    expect(wrapper.text()).toContain('CSV detected; 1000 rows sampled.')
    expect(wrapper.text()).toContain('Encoding: gbk')
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
              { id: 'c1', label: '甲', color: 'gray' },
              { id: 'c2', label: '乙', color: 'gray' },
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

it('explains that package append does not restore fields or views', async () => {
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
    expect(wrapper.text()).toContain(
      'Rows are appended; package fields and shared views are not restored.',
    )
    expect(wrapper.text()).not.toContain('Restored shared views are unlocked.')
    expect(wrapper.text()).toContain(
      'Attachment file names mapped to attachment fields cannot be restored',
    )
    i18n.global.locale.value = 'zh-CN'
    await flushPromises()
    expect(wrapper.text()).toContain(
      '将追加数据行；不会还原数据包中的字段和共享视图。',
    )
    expect(wrapper.text()).toContain('Excel 时间戳可能损失精度。')
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
    result: { ...done.result, convertedLinks: ['Other'] },
  })
  vi.stubGlobal('fetch', fetcher)
  const wrapper = mount(ImportDialog, {
    props: { targetKind: 'book', targetId: 'b' },
    global: { plugins: [i18n] },
  })
  try {
    await upload(wrapper)
    expect(wrapper.text()).toContain(
      'Attachment file names mapped to attachment fields cannot be restored and are reported as invalid; Excel timestamps may lose precision.',
    )
    expect(wrapper.text()).toContain('Restored shared views are unlocked.')
    i18n.global.locale.value = 'zh-CN'
    await flushPromises()
    expect(wrapper.text()).toContain('还原后的共享视图处于未锁定状态。')
    expect(wrapper.text()).toContain(
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
  } finally {
    wrapper.unmount()
  }
})
