import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import type { RowComment } from '../../api/types'
import { i18n, loadLocale } from '../../i18n'
import { useAuthStore } from '../../stores/auth'
import { useRealtimeStore, type RealtimeEvent } from '../../stores/realtime'
import { navUser } from '../../test/navigation'
import CommentPanel from './CommentPanel.vue'

let pinia: ReturnType<typeof createPinia>
let wrapper: VueWrapper | undefined
let comments: RowComment[]
let onEvent: ((event: RealtimeEvent) => void) | undefined

const bob = { id: 'bob-id', username: 'bob', name: 'Bob' }

beforeEach(async () => {
  pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().setUser(navUser)
  await loadLocale('en')
  i18n.global.locale.value = 'en'
  comments = []
  onEvent = undefined
  vi.spyOn(useRealtimeStore(), 'subscribe').mockImplementation(
    (_table, listener) => {
      onEvent = listener
      return () => {
        onEvent = undefined
      }
    },
  )
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  disposePinia(pinia)
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

async function setup(focusComment?: string) {
  const fetcher = vi.fn<typeof fetch>(async (input, init) => {
    const url = String(input)
    if (url.includes('/directory/mentions')) {
      return Response.json({ items: [bob], hasMore: false })
    }
    if (url.endsWith('/comments') && init?.method === 'POST') {
      const body = (JSON.parse(String(init.body)) as { body: string }).body
      comments.unshift({
        id: 'comment-1',
        authorId: navUser.id,
        authorName: 'Alice',
        body,
        mentions: [bob],
        createdAt: '2026-10-31T12:00:00Z',
        editedAt: null,
      })
      return Response.json(
        { comment: comments[0], eventId: 'event' },
        { status: 201 },
      )
    }
    if (url.includes('/comments') && init?.method === 'GET') {
      return Response.json({ items: comments, hasMore: false })
    }
    throw new Error(`Unexpected request: ${url}`)
  })
  vi.stubGlobal('fetch', fetcher)
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }],
  })
  await router.push('/t/t/v/v/r/1')
  wrapper = mount(CommentPanel, {
    props: {
      tableId: 't',
      viewId: 'v',
      rowId: '1',
      identity: navUser.id,
      level: 'read',
      focusComment,
    },
    global: { plugins: [pinia, i18n, router] },
    attachTo: document.body,
  })
  await flushPromises()
  return { wrapper, fetcher, router }
}

it('offers scoped mention choices, posts as a reader, and links only notified mentions', async () => {
  const { wrapper, fetcher } = await setup()
  const input = wrapper.get('textarea')
  await input.setValue('@bo')
  await vi.waitFor(() =>
    expect(wrapper.findAll('[role="option"]')).toHaveLength(1),
  )
  expect(fetcher).toHaveBeenCalledWith(
    '/api/v1/tables/t/directory/mentions?q=bo',
    expect.objectContaining({ method: 'GET' }),
  )
  expect(wrapper.get('[role="option"]').text()).toContain('Bob (@bob)')
  await wrapper.get('[role="option"]').trigger('click')
  expect((input.element as HTMLTextAreaElement).value).toBe('@bob ')
  await input.setValue('@bob please check @stranger')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(comments[0]?.body).toBe('@bob please check @stranger')
  expect(wrapper.get('.comment-body a').text()).toBe('@bob')
  expect(wrapper.get('.comment-body').text()).toContain(
    '@stranger (not notified)',
  )
  expect(wrapper.emitted('changed')).toHaveLength(1)
})

it('focuses the requested comment and respects author and owner controls', async () => {
  comments = [
    {
      id: 'comment-2',
      authorId: bob.id,
      authorName: 'Bob',
      body: '@alice please check',
      mentions: [{ id: navUser.id, username: 'alice', name: 'Alice' }],
      createdAt: '2026-10-31T12:00:00Z',
      editedAt: null,
    },
  ]
  const { wrapper, fetcher } = await setup('comment-2')
  expect(document.activeElement?.id).toBe('comment-comment-2')
  expect(fetcher).toHaveBeenCalledWith(
    '/api/v1/tables/t/rows/1/comments?focus=comment-2',
    expect.objectContaining({ method: 'GET' }),
  )
  expect(wrapper.find('.comment-list .comment-actions button').exists()).toBe(
    false,
  )
  await wrapper.setProps({ level: 'owner' })
  const actions = wrapper
    .findAll('.comment-list button')
    .map((button) => button.text())
  expect(actions).toEqual(['Delete comment'])
  comments[0]!.authorId = navUser.id
  onEvent?.({ t: 'comment.updated', ids: ['1'], tableId: 't' })
  await flushPromises()
  expect(
    wrapper.findAll('.comment-list button').map((button) => button.text()),
  ).toEqual(['Edit comment', 'Delete comment'])
})

it('keeps a failed draft and reports the error', async () => {
  const { wrapper } = await setup()
  vi.stubGlobal(
    'fetch',
    vi.fn<typeof fetch>(async () =>
      Response.json({ error: { code: 'ERR_INTERNAL' } }, { status: 500 }),
    ),
  )
  await wrapper.get('textarea').setValue('Draft')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe(
    'Draft',
  )
  expect(wrapper.text()).toContain('The comment may not have been saved')
})
