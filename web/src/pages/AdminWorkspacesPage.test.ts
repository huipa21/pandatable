import { afterEach, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { i18n, loadLocale } from '../i18n'
import AdminWorkspacesPage from './AdminWorkspacesPage.vue'

const replace = vi.fn()
vi.mock('vue-router', () => ({ useRouter: () => ({ replace }) }))
afterEach(() => {
  vi.unstubAllGlobals()
  replace.mockReset()
})

it('renders read-only owner/orphan/size metadata, paging/search/empty and both locales', async () => {
  await loadLocale('en')
  await loadLocale('zh-CN')
  i18n.global.locale.value = 'en'
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(
      Response.json({
        items: [
          {
            id: 'workspace',
            name: '<Workspace>',
            orphaned: true,
            attachmentBytes: 4113,
            ownerTotal: 52,
            owners: [
              {
                id: 'u',
                type: 'user',
                name: 'Alice',
                username: 'alice',
                status: 'disabled',
              },
              {
                id: 'g',
                type: 'group',
                name: 'Group',
                username: null,
                status: null,
              },
            ],
          },
        ],
        total: 60,
      }),
    )
    .mockImplementation(() =>
      Promise.resolve(Response.json({ items: [], total: 0 })),
    )
  vi.stubGlobal('fetch', fetch)
  const wrapper = mount(AdminWorkspacesPage, {
    global: { plugins: [createPinia(), i18n] },
  })
  try {
    await flushPromises()
    expect(wrapper.text()).toContain('<Workspace>')
    expect(wrapper.text()).toContain('4,113 bytes')
    expect(wrapper.text()).toContain('Disabled')
    expect(wrapper.text()).toContain('does not prevent orphan')
    expect(wrapper.text()).toContain('50 more owners')
    expect(wrapper.findAll('a')).toHaveLength(0)
    await wrapper.get('.pagination button:last-child').trigger('click')
    await flushPromises()
    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining('offset=50'),
      expect.anything(),
    )
    expect(wrapper.text()).toContain('No live workspaces')
    await wrapper.get('input').setValue('%_')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(fetch).toHaveBeenLastCalledWith(
      expect.stringContaining('q=%25_&offset=0'),
      expect.anything(),
    )
    i18n.global.locale.value = 'zh-CN'
    await flushPromises()
    expect(wrapper.get('h1').text()).toBe('工作区')
    expect(wrapper.text()).toContain('不会授予数据访问权限')
  } finally {
    wrapper.unmount()
    i18n.global.locale.value = 'en'
  }
})

it('shows retry on error and rejects late responses after unmount', async () => {
  await loadLocale('en')
  i18n.global.locale.value = 'en'
  let finish!: (response: Response) => void
  const fetch = vi
    .fn()
    .mockRejectedValueOnce(new TypeError('offline'))
    .mockImplementationOnce(
      () =>
        new Promise<Response>((resolve) => {
          finish = resolve
        }),
    )
  vi.stubGlobal('fetch', fetch)
  const wrapper = mount(AdminWorkspacesPage, {
    global: { plugins: [createPinia(), i18n] },
  })
  await flushPromises()
  expect(wrapper.get('[role="alert"]').text()).toContain('Try again')
  await wrapper.get('[role="alert"] button').trigger('click')
  const options = fetch.mock.calls[1]![1] as RequestInit
  wrapper.unmount()
  expect(options.signal?.aborted).toBe(true)
  finish(Response.json({ items: [], total: 0 }))
  await flushPromises()
  expect(replace).not.toHaveBeenCalled()
})
