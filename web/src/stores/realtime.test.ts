import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { newEventId } from '../api/eventIds'
import { useRealtimeStore } from './realtime'

class FakeSocket {
  static OPEN = 1
  static sockets: FakeSocket[] = []
  readyState = 0
  sent: string[] = []
  onopen: (() => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  onerror: (() => void) | null = null
  onclose: (() => void) | null = null

  constructor(readonly url: string) {
    FakeSocket.sockets.push(this)
  }

  open() {
    this.readyState = FakeSocket.OPEN
    this.onopen?.()
  }

  message(value: unknown) {
    this.onmessage?.(
      new MessageEvent('message', { data: JSON.stringify(value) }),
    )
  }

  send(value: string) {
    this.sent.push(value)
  }

  close() {
    this.readyState = 3
    this.onclose?.()
  }
}

describe('realtime connection', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    FakeSocket.sockets = []
    vi.stubGlobal('WebSocket', FakeSocket)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it('resubscribes and resyncs after backoff, suppressing the originating event', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const store = useRealtimeStore()
    const listener = vi.fn()
    const remove = store.subscribe('table', listener)
    store.start('user')
    const first = FakeSocket.sockets[0]!
    first.open()
    expect(first.sent.map((value) => JSON.parse(value))).toEqual([
      { t: 'sub', tableId: 'table' },
    ])
    first.message({ t: 'subscribed', tableId: 'table' })
    expect(listener).toHaveBeenLastCalledWith({ t: 'resync', tableId: 'table' })
    const own = newEventId()
    listener.mockClear()
    first.message({
      t: 'row.updated',
      tableId: 'table',
      ids: ['1'],
      by: 'user',
      eventId: own,
    })
    expect(listener).not.toHaveBeenCalled()
    first.message({
      t: 'row.updated',
      tableId: 'table',
      ids: ['1'],
      by: 'another-user',
      eventId: own,
    })
    expect(listener).toHaveBeenCalledOnce()
    listener.mockClear()
    first.message({
      t: 'row.updated',
      tableId: 'table',
      ids: ['1'],
      eventId: 'other',
    })
    expect(listener).toHaveBeenCalledOnce()

    first.close()
    expect(store.status).toBe('retrying')
    await vi.advanceTimersByTimeAsync(999)
    expect(FakeSocket.sockets).toHaveLength(1)
    await vi.advanceTimersByTimeAsync(1)
    const failed = FakeSocket.sockets[1]!
    failed.close()
    await vi.advanceTimersByTimeAsync(1999)
    expect(FakeSocket.sockets).toHaveLength(2)
    await vi.advanceTimersByTimeAsync(1)
    const reconnected = FakeSocket.sockets[2]!
    reconnected.open()
    expect(reconnected.sent.map((value) => JSON.parse(value))).toEqual([
      { t: 'sub', tableId: 'table' },
    ])
    listener.mockClear()
    reconnected.message({ t: 'subscribed', tableId: 'table' })
    expect(listener).toHaveBeenCalledWith({ t: 'resync', tableId: 'table' })
    remove()
    expect(reconnected.sent.map((value) => JSON.parse(value)).at(-1)).toEqual({
      t: 'unsub',
      tableId: 'table',
    })
    store.stop()
    disposePinia(pinia)
  })

  it('surfaces invalid protocol messages and forwards revocation', () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const store = useRealtimeStore()
    const listener = vi.fn()
    store.onGlobal(listener)
    store.start('user')
    const socket = FakeSocket.sockets[0]!
    socket.open()
    socket.message({ t: 'session.revoked' })
    expect(listener).toHaveBeenCalledWith({ t: 'session.revoked' })
    socket.message({ ids: ['1'] })
    expect(store.error).toBeInstanceOf(Error)
    expect(store.status).toBe('retrying')
    store.stop()
    disposePinia(pinia)
  })

  it('creates event IDs when secure-context-only randomUUID is unavailable', () => {
    const getRandomValues = vi.fn((bytes: Uint8Array) => bytes.fill(7))
    vi.stubGlobal('crypto', { getRandomValues })
    expect(newEventId()).toBe('07'.repeat(16))
    expect(getRandomValues).toHaveBeenCalledOnce()
  })
})
