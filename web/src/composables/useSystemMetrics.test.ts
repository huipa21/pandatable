import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { useSystemMetrics } from './useSystemMetrics'

let wrapper: VueWrapper | undefined
let state: ReturnType<typeof useSystemMetrics>
let hidden = false

beforeEach(() => {
  hidden = false
  vi.useFakeTimers()
  vi.spyOn(document, 'hidden', 'get').mockImplementation(() => hidden)
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

function start() {
  wrapper = mount(
    defineComponent({
      setup() {
        state = useSystemMetrics()
        return () => null
      },
    }),
  )
}

it('fetches immediately, refreshes within ten seconds and retains explicitly stale data on failure', async () => {
  const fetch = vi
    .fn()
    .mockResolvedValue(Response.json({ collectedAt: 'first' }))
  vi.stubGlobal('fetch', fetch)
  start()
  await vi.advanceTimersByTimeAsync(0)
  expect(fetch).toHaveBeenCalledOnce()
  expect(state.snapshot.value?.collectedAt).toBe('first')
  await vi.advanceTimersByTimeAsync(9_999)
  expect(fetch).toHaveBeenCalledOnce()
  fetch.mockRejectedValueOnce(new TypeError('offline'))
  await vi.advanceTimersByTimeAsync(1)
  expect(fetch).toHaveBeenCalledTimes(2)
  expect(state.snapshot.value?.collectedAt).toBe('first')
  expect(state.error.value).toBeInstanceOf(TypeError)
  fetch.mockResolvedValue(Response.json({ collectedAt: 'recovered' }))
  await state.refresh()
  expect(state.error.value).toBeNull()
  expect(state.snapshot.value?.collectedAt).toBe('recovered')
  wrapper?.unmount()
  expect(vi.getTimerCount()).toBe(0)
  await vi.advanceTimersByTimeAsync(20_000)
  expect(fetch).toHaveBeenCalledTimes(3)
})

it('never overlaps, aborts hidden work and ignores stale results after resuming', async () => {
  let finish: ((response: Response) => void) | undefined
  const fetch = vi.fn((path: string, init: RequestInit) => {
    expect(path).toBe('/api/v1/admin/system')
    expect(init.signal).toBeInstanceOf(AbortSignal)
    return new Promise<Response>((resolve) => {
      finish = resolve
    })
  })
  vi.stubGlobal('fetch', fetch)
  start()
  await state.refresh()
  expect(fetch).toHaveBeenCalledOnce()
  const old = finish!
  const signal = fetch.mock.calls[0]![1].signal!
  hidden = true
  document.dispatchEvent(new Event('visibilitychange'))
  expect(signal.aborted).toBe(true)
  expect(vi.getTimerCount()).toBe(0)
  await vi.advanceTimersByTimeAsync(30_000)
  expect(fetch).toHaveBeenCalledOnce()
  hidden = false
  document.dispatchEvent(new Event('visibilitychange'))
  expect(fetch).toHaveBeenCalledTimes(2)
  finish!(Response.json({ collectedAt: 'new' }))
  await vi.advanceTimersByTimeAsync(0)
  old(Response.json({ collectedAt: 'old' }))
  await vi.advanceTimersByTimeAsync(0)
  expect(state.snapshot.value?.collectedAt).toBe('new')
  expect(state.error.value).toBeNull()
})

it('bounds a stalled request and exposes timeout rather than a successful empty snapshot', async () => {
  const fetch = vi.fn(
    (_path: string, init: RequestInit) =>
      new Promise<Response>((_resolve, reject) => {
        init.signal?.addEventListener('abort', () =>
          reject(new DOMException('Aborted', 'AbortError')),
        )
      }),
  )
  vi.stubGlobal('fetch', fetch)
  start()
  await vi.advanceTimersByTimeAsync(10_000)
  expect(fetch).toHaveBeenCalledOnce()
  expect(state.error.value).toBeInstanceOf(DOMException)
  expect(state.snapshot.value).toBeNull()
  await vi.advanceTimersByTimeAsync(10_000)
  expect(fetch).toHaveBeenCalledTimes(2)
})
