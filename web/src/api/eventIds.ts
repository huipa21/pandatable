const ownEvents = new Map<string, number>()
const eventLifetime = 60_000
const maxRememberedEvents = 2048

export function newEventId(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(16))
  const id = Array.from(bytes, (byte) =>
    byte.toString(16).padStart(2, '0'),
  ).join('')
  const now = Date.now()
  for (const [key, expires] of ownEvents) {
    if (expires > now && ownEvents.size < maxRememberedEvents) break
    ownEvents.delete(key)
  }
  ownEvents.set(id, now + eventLifetime)
  return id
}

export function isOwnEvent(id: string | undefined): boolean {
  if (!id) return false
  const expires = ownEvents.get(id)
  if (expires === undefined) return false
  if (expires <= Date.now()) {
    ownEvents.delete(id)
    return false
  }
  return true
}
