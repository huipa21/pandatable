import type { WindowSource } from '../components/grid/model'

export function syntheticSource(
  total: number,
  label: (id: number) => string,
  group: (index: number) => string,
): WindowSource {
  return {
    total,
    fetch(offset, limit, signal) {
      return new Promise((resolve, reject) => {
        const abort = () => {
          clearTimeout(timer)
          signal.removeEventListener('abort', abort)
          reject(new DOMException('Synthetic request cancelled', 'AbortError'))
        }
        // Deliberately varied synthetic latency, not a database measurement.
        const timer = setTimeout(
          () => {
            signal.removeEventListener('abort', abort)
            resolve(
              Array.from({ length: limit }, (_, i) => ({
                id: offset + i + 1,
                name: label(offset + i + 1),
                note: '',
                quantity: (offset + i) % 100,
                group: group((offset + i) % 2),
              })),
            )
          },
          offset % 300 === 0 ? 120 : 40,
        )
        signal.addEventListener('abort', abort, { once: true })
        if (signal.aborted) abort()
      })
    },
  }
}
