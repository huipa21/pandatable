import { afterEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { nextTick } from 'vue'
import type { AuthUser, BackupRecord } from '../api/types'
import { i18n, loadLocale } from '../i18n'
import { useAuthStore } from '../stores/auth'
import AdminBackupsPage from './AdminBackupsPage.vue'

const replace = vi.fn()
vi.mock('vue-router', () => ({ useRouter: () => ({ replace }) }))

const admin: AuthUser = {
  id: 'admin',
  username: 'admin',
  email: null,
  displayName: 'Admin',
  role: 'admin',
  mustChangePassword: false,
  locale: 'en',
  theme: 'system',
  timezone: null,
  effectiveTimezone: 'UTC',
  avatarColor: '#123456',
}

const running: BackupRecord = {
  id: 'backup-1',
  fileName: 'pandatable-20260620-120000.zip',
  sizeBytes: null,
  status: 'running',
  createdBy: 'admin',
  createdByName: 'Admin',
  createdAt: '2026-06-20T12:00:00Z',
  finishedAt: null,
  progress: { stage: 'archiving', percent: 45 },
}

async function setup() {
  await loadLocale('en')
  await loadLocale('zh-CN')
  i18n.global.locale.value = 'en'
  const pinia = createPinia()
  const auth = useAuthStore(pinia)
  auth.setUser(admin)
  return {
    auth,
    wrapper: mount(AdminBackupsPage, { global: { plugins: [pinia, i18n] } }),
  }
}

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
  replace.mockReset()
  i18n.global.locale.value = 'en'
})

it('creates, polls, downloads, translates, and confirms deletion', async () => {
  let items: BackupRecord[] = []
  const fetch = vi.fn((_path: string, options: RequestInit) => {
    if (options.method === 'POST') {
      items = [running]
      return Promise.resolve(Response.json(running, { status: 202 }))
    }
    if (options.method === 'DELETE') {
      items = []
      return Promise.resolve(Response.json({ eventId: 'event' }))
    }
    return Promise.resolve(Response.json({ items, freeBytes: 12000 }))
  })
  vi.stubGlobal('fetch', fetch)
  const { wrapper } = await setup()
  try {
    await flushPromises()
    expect(wrapper.text()).toContain('No backups yet.')
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    await wrapper
      .findAll('button')
      .find((button) => button.text() === 'Create backup')!
      .trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('Archiving files: 45%')
    expect(
      wrapper
        .findAll('button')
        .find((button) => button.text() === 'Create backup')!
        .attributes('disabled'),
    ).toBeDefined()
    items = [
      {
        ...running,
        status: 'done',
        sizeBytes: 800,
        progress: undefined,
        finishedAt: '2026-06-20T12:01:00Z',
      },
    ]
    await vi.advanceTimersByTimeAsync(1500)
    await flushPromises()
    expect(wrapper.get('a[download]').attributes('href')).toBe(
      '/api/v1/admin/backups/backup-1/download',
    )
    i18n.global.locale.value = 'zh-CN'
    await nextTick()
    expect(wrapper.get('h1').text()).toBe('备份')
    i18n.global.locale.value = 'en'
    await nextTick()
    await wrapper.get('button[aria-label^="Delete backup"]').trigger('click')
    expect(wrapper.text()).toContain('Permanently delete')
    await wrapper
      .findAll('button')
      .find((button) => button.text() === 'Delete backup')!
      .trigger('click')
    await flushPromises()
    expect(fetch).toHaveBeenCalledWith(
      '/api/v1/admin/backups/backup-1',
      expect.objectContaining({ method: 'DELETE' }),
    )
    expect(wrapper.text()).toContain('No backups yet.')
  } finally {
    wrapper.unmount()
  }
})

it('shows localized disk-space errors without claiming a backup was created', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn((_path: string, options: RequestInit) =>
      Promise.resolve(
        options.method === 'POST'
          ? Response.json(
              { error: { code: 'ERR_BACKUP_DISK_SPACE' } },
              { status: 409 },
            )
          : Response.json({ items: [], freeBytes: 1 }),
      ),
    ),
  )
  const { wrapper } = await setup()
  try {
    await flushPromises()
    await wrapper
      .findAll('button')
      .find((button) => button.text() === 'Create backup')!
      .trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain(
      'Not enough disk space',
    )
    expect(wrapper.text()).toContain('No backups yet.')
    i18n.global.locale.value = 'zh-CN'
    await nextTick()
    expect(wrapper.get('[role="alert"]').text()).toContain('磁盘空间不足')
  } finally {
    wrapper.unmount()
  }
})

it('shows failed jobs with a localized reason and no download', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve(
        Response.json({
          items: [
            {
              ...running,
              status: 'failed',
              progress: undefined,
              errorCode: 'ERR_BACKUP_FAILED',
              finishedAt: '2026-06-20T12:01:00Z',
            },
          ],
          freeBytes: 12000,
        }),
      ),
    ),
  )
  const { wrapper } = await setup()
  try {
    await flushPromises()
    expect(wrapper.text()).toContain('Status: Failed')
    expect(wrapper.get('[role="alert"]').text()).toContain('The backup failed')
    expect(wrapper.find('a[download]').exists()).toBe(false)
    i18n.global.locale.value = 'zh-CN'
    await nextTick()
    expect(wrapper.get('[role="alert"]').text()).toContain('备份失败')
  } finally {
    wrapper.unmount()
  }
})

it('aborts and clears a previous user’s pending list on logout', async () => {
  let release!: (response: Response) => void
  let signal: AbortSignal | undefined
  vi.stubGlobal(
    'fetch',
    vi.fn((_path: string, options: RequestInit) => {
      signal = options.signal ?? undefined
      return new Promise<Response>((resolve) => {
        release = resolve
      })
    }),
  )
  const { auth, wrapper } = await setup()
  try {
    expect(signal?.aborted).toBe(false)
    auth.setUser(null)
    expect(signal?.aborted).toBe(true)
    release(Response.json({ items: [running], freeBytes: 100 }))
    await flushPromises()
    expect(wrapper.text()).not.toContain(running.fileName)
    expect(wrapper.text()).not.toContain('Available disk space')
  } finally {
    wrapper.unmount()
  }
})
