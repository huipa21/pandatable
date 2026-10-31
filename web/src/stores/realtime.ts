import { onScopeDispose, ref } from 'vue'
import { defineStore } from 'pinia'
import { isOwnEvent } from '../api/eventIds'

export interface RealtimeEvent {
  t: string
  tableId?: string
  ids?: string[]
  fields?: string[]
  changes?: Record<string, string[]>
  by?: string
  byName?: string
  eventId?: string
  schemaVersion?: number
  viewId?: string
  version?: number
  code?: string
}

type Listener = (event: RealtimeEvent) => void
type SocketStatus = 'idle' | 'connecting' | 'connected' | 'retrying'

function parseEvent(raw: unknown): RealtimeEvent {
  if (typeof raw !== 'string') throw new Error('Realtime message is not text')
  const value: unknown = JSON.parse(raw)
  if (
    typeof value !== 'object' ||
    value === null ||
    !('t' in value) ||
    typeof value.t !== 'string'
  )
    throw new Error('Realtime message has no type')
  const event = value as RealtimeEvent
  for (const item of [event.tableId, event.by, event.byName, event.eventId]) {
    if (item !== undefined && typeof item !== 'string')
      throw new Error('Realtime message has invalid identifiers')
  }
  for (const list of [event.ids, event.fields]) {
    if (
      list !== undefined &&
      (!Array.isArray(list) || list.some((item) => typeof item !== 'string'))
    )
      throw new Error('Realtime message has invalid row or field IDs')
  }
  if (
    event.changes !== undefined &&
    (typeof event.changes !== 'object' ||
      event.changes === null ||
      Array.isArray(event.changes) ||
      Object.values(event.changes).some(
        (fields) =>
          !Array.isArray(fields) ||
          fields.some((field) => typeof field !== 'string'),
      ))
  )
    throw new Error('Realtime message has invalid cell changes')
  return event
}

export const useRealtimeStore = defineStore('realtime', () => {
  const status = ref<SocketStatus>('idle')
  const error = ref<unknown>(null)
  const listeners = new Map<string, Set<Listener>>()
  const globalListeners = new Set<Listener>()
  let identity: string | null = null
  let socket: WebSocket | undefined
  let retry: ReturnType<typeof setTimeout> | undefined
  let generation = 0
  let delay = 1000

  function send(type: string, tableId: string) {
    if (socket?.readyState === WebSocket.OPEN)
      socket.send(JSON.stringify({ t: type, tableId }))
  }

  function schedule(current: number) {
    if (!identity || current !== generation || retry) return
    status.value = 'retrying'
    const wait = delay
    delay = Math.min(delay * 2, 30_000)
    retry = setTimeout(() => {
      retry = undefined
      open(current)
    }, wait)
  }

  function dispatch(event: RealtimeEvent) {
    if (event.by === identity && isOwnEvent(event.eventId)) return
    if (event.t === 'subscribed' && event.tableId) {
      for (const listener of listeners.get(event.tableId) ?? [])
        listener({ t: 'resync', tableId: event.tableId })
      return
    }
    if (event.tableId) {
      for (const listener of listeners.get(event.tableId) ?? []) listener(event)
    }
    for (const listener of globalListeners) listener(event)
  }

  function open(current: number) {
    if (!identity || current !== generation) return
    status.value = 'connecting'
    let next: WebSocket
    try {
      const scheme = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      next = new WebSocket(`${scheme}//${window.location.host}/api/v1/ws`)
    } catch (cause) {
      error.value = cause
      schedule(current)
      return
    }
    socket = next
    next.onopen = () => {
      if (current !== generation || socket !== next) return
      status.value = 'connected'
      error.value = null
      delay = 1000
      for (const tableId of listeners.keys()) send('sub', tableId)
    }
    next.onmessage = (message) => {
      if (current !== generation || socket !== next) return
      let event: RealtimeEvent
      try {
        event = parseEvent(message.data)
      } catch (cause) {
        error.value = cause
        next.close(1002, 'Invalid realtime message')
        return
      }
      dispatch(event)
    }
    next.onerror = () => {
      if (current === generation && socket === next)
        error.value = new Error('Realtime connection failed')
    }
    next.onclose = () => {
      if (current !== generation || socket !== next) return
      socket = undefined
      schedule(current)
    }
  }

  function stop() {
    generation++
    identity = null
    clearTimeout(retry)
    retry = undefined
    const previous = socket
    socket = undefined
    previous?.close(1000, 'Client stopped')
    status.value = 'idle'
    error.value = null
    delay = 1000
  }

  function start(userId: string | null) {
    if (identity === userId) return
    stop()
    identity = userId
    if (identity) open(generation)
  }

  function subscribe(tableId: string, listener: Listener): () => void {
    let group = listeners.get(tableId)
    if (!group) {
      group = new Set()
      listeners.set(tableId, group)
      send('sub', tableId)
    }
    group.add(listener)
    return () => {
      group.delete(listener)
      if (group.size === 0 && listeners.get(tableId) === group) {
        listeners.delete(tableId)
        send('unsub', tableId)
      }
    }
  }

  function onGlobal(listener: Listener): () => void {
    globalListeners.add(listener)
    return () => globalListeners.delete(listener)
  }

  onScopeDispose(stop)
  return { status, error, start, stop, subscribe, onGlobal }
})
