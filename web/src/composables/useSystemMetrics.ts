import { onBeforeUnmount, onMounted, ref, shallowRef } from 'vue'
import { request } from '../api/client'
import type { SystemSnapshot } from '../api/types'

export function useSystemMetrics() {
  const snapshot = shallowRef<SystemSnapshot | null>(null)
  const loading = ref(false)
  const error = shallowRef<unknown>(null)
  let controller: AbortController | undefined
  let timer: ReturnType<typeof setInterval> | undefined
  let deadline: ReturnType<typeof setTimeout> | undefined
  let generation = 0
  let mounted = false

  async function refresh() {
    if (!mounted || document.hidden || controller) return
    const current = ++generation
    const active = new AbortController()
    controller = active
    loading.value = true
    deadline = setTimeout(() => active.abort(), 10_000)
    try {
      const result = await request<SystemSnapshot>(
        '/admin/system',
        'GET',
        undefined,
        active.signal,
      )
      if (current !== generation) return
      snapshot.value = result
      error.value = null
    } catch (cause) {
      if (current === generation) error.value = cause
    } finally {
      if (current === generation) {
        clearTimeout(deadline)
        controller = undefined
        loading.value = false
      }
    }
  }

  function stop() {
    generation++
    clearInterval(timer)
    clearTimeout(deadline)
    timer = undefined
    controller?.abort()
    controller = undefined
    loading.value = false
  }

  function visibility() {
    stop()
    if (!document.hidden && mounted) {
      timer = setInterval(() => void refresh(), 10_000)
      void refresh()
    }
  }

  onMounted(() => {
    mounted = true
    document.addEventListener('visibilitychange', visibility)
    visibility()
  })
  onBeforeUnmount(() => {
    mounted = false
    document.removeEventListener('visibilitychange', visibility)
    stop()
  })
  return { snapshot, loading, error, refresh }
}
