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

  it('reports the latest focus at most every 200 ms, heartbeats, and stops on unsubscribe', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const store = useRealtimeStore()
    store.start('user')
    const remove = store.subscribe('table', vi.fn())
    store.setView('table', 'view-a')
    const socket = FakeSocket.sockets[0]!
    socket.open()
    expect(socket.sent.map((value) => JSON.parse(value))).toEqual([
      { t: 'sub', tableId: 'table' },
    ])
    socket.message({ t: 'subscribed', tableId: 'table' })
    expect(JSON.parse(socket.sent.at(-1)!)).toEqual({
      t: 'presence',
      tableId: 'table',
      viewId: 'view-a',
    })

    store.setFocus('table', { rowId: '1', fieldId: 'field' })
    store.setFocus('table', { rowId: '9007199254740993', fieldId: 'field' })
    await vi.advanceTimersByTimeAsync(199)
    expect(socket.sent).toHaveLength(2)
    await vi.advanceTimersByTimeAsync(1)
    expect(JSON.parse(socket.sent.at(-1)!)).toEqual({
      t: 'presence',
      tableId: 'table',
      viewId: 'view-a',
      cell: { rowId: '9007199254740993', fieldId: 'field' },
    })

    store.setView('table', 'view-b')
    await vi.advanceTimersByTimeAsync(200)
    expect(JSON.parse(socket.sent.at(-1)!)).toEqual({
      t: 'presence',
      tableId: 'table',
      viewId: 'view-b',
    })
    const beforeHeartbeat = socket.sent.length
    await vi.advanceTimersByTimeAsync(19_600)
    expect(socket.sent).toHaveLength(beforeHeartbeat + 1)
    expect(JSON.parse(socket.sent.at(-1)!)).toEqual({
      t: 'presence',
      tableId: 'table',
      viewId: 'view-b',
    })

    store.setFocus('table', { rowId: '2', fieldId: 'field' })
    remove()
    const afterUnsubscribe = socket.sent.length
    expect(JSON.parse(socket.sent.at(-1)!)).toEqual({
      t: 'unsub',
      tableId: 'table',
    })
    await vi.advanceTimersByTimeAsync(20_000)
    expect(socket.sent).toHaveLength(afterUnsubscribe)
    store.stop()
    disposePinia(pinia)
  })

  it('clears stale snapshots on view changes, disconnect, denial, and identity stop', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const store = useRealtimeStore()
    store.start('user')
    const remove = store.subscribe('table', vi.fn())
    store.setView('table', 'view-a')
    const socket = FakeSocket.sockets[0]!
    socket.open()
    socket.message({ t: 'subscribed', tableId: 'table' })
    const users = [
      {
        userId: 'bob',
        name: 'Bob',
        cell: { rowId: '1', fieldId: 'field' },
      },
    ]
    socket.message({ t: 'presence', tableId: 'table', viewId: 'view-a', users })
    expect(store.presence.table).toEqual(users)

    store.setView('table', 'view-b')
    expect(store.presence.table).toEqual([{ userId: 'bob', name: 'Bob' }])
    socket.message({ t: 'presence', tableId: 'table', viewId: 'view-a', users })
    expect(store.presence.table).toEqual([{ userId: 'bob', name: 'Bob' }])
    socket.close()
    expect(store.presence).toEqual({})
    await vi.advanceTimersByTimeAsync(1000)
    const next = FakeSocket.sockets[1]!
    next.open()
    next.message({ t: 'subscribed', tableId: 'table' })
    expect(JSON.parse(next.sent.at(-1)!)).toEqual({
      t: 'presence',
      tableId: 'table',
      viewId: 'view-b',
    })
    next.message({ t: 'presence', tableId: 'table', viewId: 'view-b', users })
    expect(store.presence.table).toEqual(users)
    next.message({ t: 'sub.denied', tableId: 'table', code: 'ERR_NOT_FOUND' })
    next.message({ t: 'presence', tableId: 'table', viewId: 'view-b', users })
    expect(store.presence).toEqual({})
    store.stop()
    remove()
    disposePinia(pinia)
  })

  it('rejects malformed presence snapshots rather than showing partial data', () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const store = useRealtimeStore()
    store.start('user')
    store.subscribe('table', vi.fn())
    store.setView('table', 'view')
    const socket = FakeSocket.sockets[0]!
    socket.open()
    socket.message({ t: 'subscribed', tableId: 'table' })
    socket.message({
      t: 'presence',
      tableId: 'table',
      users: [{ userId: 'bob', name: 'Bob', cell: { rowId: 1 } }],
    })
    expect(store.error).toBeInstanceOf(Error)
    expect(store.presence).toEqual({})
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
