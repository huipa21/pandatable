import { afterEach, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { nextTick } from 'vue'
import type { SystemMetric, SystemSnapshot } from '../api/types'
import { i18n, loadLocale } from '../i18n'
import AdminDashboardPage from './AdminDashboardPage.vue'

const replace = vi.fn()
vi.mock('vue-router', () => ({ useRouter: () => ({ replace }) }))
afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

const at = '2026-10-26T00:00:00Z'
function ready<T>(value: T): SystemMetric<T> {
  return { status: 'ready', collectedAt: at, value }
}
function fixture(): SystemSnapshot {
  return {
    collectedAt: at,
    cpuPercent: {
      status: 'unavailable',
      reason: 'initializing',
      collectedAt: at,
      value: null,
    },
    load: {
      status: 'unavailable',
      reason: 'unsupported',
      collectedAt: at,
      value: null,
    },
    memory: ready({ usedBytes: 1024, totalBytes: 4096, availableBytes: 3072 }),
    appRssBytes: ready(1024),
    container: ready({ currentBytes: 1024, limitBytes: null, unlimited: true }),
    disk: ready({ totalBytes: 8192, usedBytes: 4096, freeBytes: 4096 }),
    database: ready({
      sizeBytes: 4096,
      connections: 2,
      activeConnections: 1,
      sharedBuffersBytes: 4096,
    }),
    filesBytes: ready(0),
    backupsBytes: ready(2048),
    versions: {
      app: 'test-build',
      go: 'go1.27.0',
      postgres: ready('18.6'),
      migration: ready(2),
    },
    uptimeSeconds: 60,
  }
}

it('renders real zero, units, timestamps, unavailable/initial CPU and both locales; stale data stays visibly stale', async () => {
  await loadLocale('en')
  await loadLocale('zh-CN')
  i18n.global.locale.value = 'en'
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
  let finish: ((response: Response) => void) | undefined
  const fetch = vi.fn().mockImplementationOnce(
    () =>
      new Promise<Response>((resolve) => {
        finish = resolve
      }),
  )
  vi.stubGlobal('fetch', fetch)
  const wrapper = mount(AdminDashboardPage, {
    global: { plugins: [i18n, createPinia()] },
  })
  try {
    await nextTick()
    expect(wrapper.text()).toContain('Collecting system observations')
    finish!(Response.json(fixture()))
    await flushPromises()
    expect(wrapper.get('[data-metric="files"]').text()).toContain('0 B')
    expect(
      wrapper.get('[data-metric="cpu"]').attributes('data-available'),
    ).toBe('false')
    expect(wrapper.get('[data-metric="cpu"]').text()).toContain(
      'Waiting for a second',
    )
    expect(wrapper.get('[data-metric="containerLimit"]').text()).toContain(
      'No visible finite limit',
    )
    expect(wrapper.get('[data-metric="dbSize"]').text()).toContain('4 KiB')
    expect(wrapper.get('[data-metric="files"]').text()).toContain(
      'Directory scan',
    )
    fetch.mockRejectedValueOnce(new TypeError('offline'))
    await wrapper.get('.dashboard-heading button').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('stale')
    expect(wrapper.get('[data-metric="dbSize"]').text()).toContain('4 KiB')
    i18n.global.locale.value = 'zh-CN'
    await flushPromises()
    expect(wrapper.get('h1').text()).toBe('系统仪表盘')
    expect(wrapper.get('[data-metric="cpu"]').text()).toContain('等待第二次')
    expect(wrapper.get('[role="alert"]').text()).toContain('数据已过期')
  } finally {
    wrapper.unmount()
    i18n.global.locale.value = 'en'
  }
})
