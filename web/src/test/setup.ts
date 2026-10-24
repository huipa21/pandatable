import { vi } from 'vitest'

export class ThemeMedia extends EventTarget implements MediaQueryList {
  matches = false
  media = '(prefers-color-scheme: dark)'
  onchange: MediaQueryList['onchange'] = null
  private listeners = new Set<(event: MediaQueryListEvent) => void>()

  addListener(callback: (event: MediaQueryListEvent) => void) {
    this.listeners.add(callback)
  }

  removeListener(callback: (event: MediaQueryListEvent) => void) {
    this.listeners.delete(callback)
  }

  setDark(matches: boolean) {
    this.matches = matches
    const event = Object.assign(new Event('change'), {
      matches,
      media: this.media,
    })
    this.dispatchEvent(event)
    for (const listener of this.listeners) listener(event)
  }
}

export const themeMedia = new ThemeMedia()
vi.stubGlobal('matchMedia', () => themeMedia)
