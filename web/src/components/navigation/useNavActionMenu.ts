import { onBeforeUnmount, type Ref } from 'vue'

export function useNavActionMenu(root: Ref<HTMLDetailsElement | undefined>) {
  function close(focus = false) {
    root.value?.removeAttribute('open')
    if (focus) root.value?.querySelector('summary')?.focus()
  }

  function onOutside(event: Event) {
    const target = event.target
    if (!(target instanceof Element) || root.value?.contains(target)) return
    if (target.closest('dialog, [role="dialog"], .n-modal-container')) return
    close()
  }

  function onKey(event: KeyboardEvent) {
    if (event.key === 'Escape' && root.value?.contains(event.target as Node)) {
      event.preventDefault()
      close(true)
    }
  }

  function listen(on: boolean) {
    if (on) {
      document.addEventListener('pointerdown', onOutside, true)
      document.addEventListener('keydown', onKey)
    } else {
      document.removeEventListener('pointerdown', onOutside, true)
      document.removeEventListener('keydown', onKey)
    }
  }

  function onToggle() {
    const open = root.value?.open ?? false
    listen(open)
    if (!open) return
    for (const other of document.querySelectorAll('details.node-actions[open]'))
      if (other !== root.value) other.removeAttribute('open')
  }

  onBeforeUnmount(() => listen(false))
  return { close, onToggle }
}
