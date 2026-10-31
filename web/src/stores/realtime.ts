import { onScopeDispose, ref } from 'vue'
import { defineStore } from 'pinia'
import { isOwnEvent } from '../api/eventIds'

export interface PresenceCell {
  rowId: string
  fieldId: string
}

export interface PresenceUser {
  userId: string
  name: string
  cell?: PresenceCell
}

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
  users?: PresenceUser[]
}

type Listener = (event: RealtimeEvent) => void
type SocketStatus = 'idle' | 'connecting' | 'connected' | 'retrying'

interface LocalPresence {
  viewId: string
  cell: PresenceCell | null
  lastSent?: number
  timer?: ReturnType<typeof setTimeout>
}

const presenceInterval = 200
const presenceHeartbeat = 20_000

function isPresenceUser(value: unknown): value is PresenceUser {
  if (
    typeof value !== 'object' ||
    value === null ||
    !('userId' in value) ||
    typeof value.userId !== 'string' ||
    !('name' in value) ||
    typeof value.name !== 'string'
  )
    return false
  if (!('cell' in value) || value.cell === undefined) return true
  const cell = value.cell
  return (
    typeof cell === 'object' &&
    cell !== null &&
    'rowId' in cell &&
    typeof cell.rowId === 'string' &&
    'fieldId' in cell &&
    typeof cell.fieldId === 'string'
  )
}

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
  for (const item of [
    event.tableId,
    event.by,
    event.byName,
    event.eventId,
    event.viewId,
    event.code,
  ]) {
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
  if (
    event.t === 'presence' &&
    (!event.tableId ||
      !Array.isArray(event.users) ||
      event.users.some((user) => !isPresenceUser(user)))
  )
    throw new Error('Realtime message has invalid presence')
  return event
}

export const useRealtimeStore = defineStore('realtime', () => {
  const status = ref<SocketStatus>('idle')
  const error = ref<unknown>(null)
  const presence = ref<Record<string, PresenceUser[]>>({})
  const listeners = new Map<string, Set<Listener>>()
  const globalListeners = new Set<Listener>()
  const localPresence = new Map<string, LocalPresence>()
  const acknowledged = new Set<string>()
  let identity: string | null = null
  let socket: WebSocket | undefined
  let retry: ReturnType<typeof setTimeout> | undefined
  let heartbeat: ReturnType<typeof setInterval> | undefined
  let generation = 0
  let delay = 1000

  function send(type: string, tableId: string) {
    if (socket?.readyState === WebSocket.OPEN)
      socket.send(JSON.stringify({ t: type, tableId }))
  }

  function clearVisible(tableId: string) {
    if (!Object.hasOwn(presence.value, tableId)) return
    const next = { ...presence.value }
    delete next[tableId]
    presence.value = next
  }

  function clearConnectionPresence() {
    acknowledged.clear()
    presence.value = {}
    for (const state of localPresence.values()) {
      clearTimeout(state.timer)
      state.timer = undefined
      state.lastSent = undefined
    }
  }

  function flushPresence(tableId: string) {
    const state = localPresence.get(tableId)
    if (!state) return
    state.timer = undefined
    if (!acknowledged.has(tableId) || socket?.readyState !== WebSocket.OPEN)
      return
    socket.send(
      JSON.stringify({
        t: 'presence',
        tableId,
        ...(state.viewId ? { viewId: state.viewId } : {}),
        ...(state.cell ? { cell: state.cell } : {}),
      }),
    )
    state.lastSent = performance.now()
  }

  function queuePresence(tableId: string) {
    const state = localPresence.get(tableId)
    if (
      !state ||
      state.timer ||
      !acknowledged.has(tableId) ||
      socket?.readyState !== WebSocket.OPEN
    )
      return
    const wait =
      state.lastSent === undefined
        ? 0
        : Math.max(0, state.lastSent + presenceInterval - performance.now())
    if (wait === 0) flushPresence(tableId)
    else state.timer = setTimeout(() => flushPresence(tableId), wait)
  }

  function setView(tableId: string, viewId: string) {
    if (!listeners.has(tableId)) return
    let state = localPresence.get(tableId)
    if (!state) {
      state = { viewId, cell: null }
      localPresence.set(tableId, state)
    } else if (state.viewId === viewId) {
      return
    } else {
      state.viewId = viewId
      state.cell = null
    }
    const users = presence.value[tableId]
    if (users)
      presence.value = {
        ...presence.value,
        [tableId]: users.map(({ userId, name }) => ({ userId, name })),
      }
    queuePresence(tableId)
  }

  function setFocus(tableId: string, cell: PresenceCell | null) {
    const state = localPresence.get(tableId)
    if (!state) return
    if (
      state.cell?.rowId === cell?.rowId &&
      state.cell?.fieldId === cell?.fieldId
    )
      return
    state.cell = cell
    queuePresence(tableId)
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
      if (!listeners.has(event.tableId)) return
      acknowledged.add(event.tableId)
      queuePresence(event.tableId)
      for (const listener of listeners.get(event.tableId) ?? [])
        listener({ t: 'resync', tableId: event.tableId })
      return
    }
    if (event.t === 'sub.denied' && event.tableId) {
      acknowledged.delete(event.tableId)
      clearVisible(event.tableId)
      const state = localPresence.get(event.tableId)
      if (state) {
        clearTimeout(state.timer)
        state.timer = undefined
      }
    } else if (
      event.t === 'presence' &&
      event.tableId &&
      event.users &&
      acknowledged.has(event.tableId)
    ) {
      const state = localPresence.get(event.tableId)
      const sameView = !!state && (event.viewId ?? '') === state.viewId
      presence.value = {
        ...presence.value,
        [event.tableId]: sameView
          ? event.users
          : event.users.map(({ userId, name }) => ({ userId, name })),
      }
      error.value = null
    } else if (event.t === 'error') {
      error.value = new Error(`Realtime protocol error: ${event.code ?? ''}`)
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
      clearConnectionPresence()
      schedule(current)
    }
  }

  function stop() {
    generation++
    identity = null
    clearTimeout(retry)
    clearInterval(heartbeat)
    retry = undefined
    heartbeat = undefined
    clearConnectionPresence()
    localPresence.clear()
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
    if (identity) {
      heartbeat = setInterval(() => {
        for (const tableId of localPresence.keys()) queuePresence(tableId)
      }, presenceHeartbeat)
      open(generation)
    }
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
        acknowledged.delete(tableId)
        const state = localPresence.get(tableId)
        if (state) clearTimeout(state.timer)
        localPresence.delete(tableId)
        clearVisible(tableId)
        send('unsub', tableId)
      }
    }
  }

  function onGlobal(listener: Listener): () => void {
    globalListeners.add(listener)
    return () => globalListeners.delete(listener)
  }

  onScopeDispose(stop)
  return {
    status,
    error,
    presence,
    start,
    stop,
    subscribe,
    onGlobal,
    setView,
    setFocus,
  }
})
