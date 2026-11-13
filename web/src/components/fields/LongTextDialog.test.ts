import {
  afterAll,
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest'
import { nextTick } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { i18n, loadLocale } from '../../i18n'
import LongTextDialog from './LongTextDialog.vue'

const mounted: Array<() => void> = []
const nativeShowModal = HTMLDialogElement.prototype.showModal
const nativeClose = HTMLDialogElement.prototype.close

beforeAll(() => {
  HTMLDialogElement.prototype.showModal = function () {
    this.setAttribute('open', '')
  }
  HTMLDialogElement.prototype.close = function () {
    this.removeAttribute('open')
  }
})

beforeEach(async () => {
  await loadLocale('en')
  i18n.global.locale.value = 'en'
})

afterEach(() => {
  for (const unmount of mounted.splice(0)) unmount()
})

afterAll(() => {
  HTMLDialogElement.prototype.showModal = nativeShowModal
  HTMLDialogElement.prototype.close = nativeClose
})

function mountDialog(value = 'hello') {
  const wrapper = mount(LongTextDialog, {
    props: { open: true, value, fieldName: 'Notes' },
    global: { plugins: [i18n] },
    attachTo: document.body,
  })
  mounted.push(() => wrapper.unmount())
  return wrapper
}

function getTextarea() {
  const textarea = document.querySelector<HTMLTextAreaElement>(
    '.long-text-dialog__textarea',
  )
  if (!textarea) throw new Error('Long-text dialog textarea was not mounted')
  return textarea
}

function getDialog() {
  const dialog = document.querySelector<HTMLDialogElement>('.long-text-dialog')
  if (!dialog) throw new Error('Long-text dialog was not mounted')
  vi.spyOn(dialog, 'getBoundingClientRect').mockReturnValue(
    new DOMRect(100, 100, 600, 400),
  )
  return dialog
}

function pointerClick(
  start: Element,
  end: Element,
  startPosition: { clientX: number; clientY: number },
  endPosition = startPosition,
) {
  start.dispatchEvent(
    new MouseEvent('pointerdown', { bubbles: true, ...startPosition }),
  )
  end.dispatchEvent(new MouseEvent('click', { bubbles: true, ...endPosition }))
}

describe('long-text dialog', () => {
  it('seeds and focuses the textarea, then applies Bold to its selection', async () => {
    mountDialog()
    await flushPromises()
    const input = getTextarea()
    expect(input.value).toBe('hello')
    expect(document.activeElement).toBe(input)

    input.setSelectionRange(0, 5)
    input.dispatchEvent(new Event('select', { bubbles: true }))
    const bold = document.querySelector<HTMLButtonElement>(
      '.long-text-dialog__toolbar button[aria-label^="Bold"]',
    )
    if (!bold) throw new Error('Bold toolbar button was not mounted')
    bold.click()
    await nextTick()

    expect(input.value).toBe('**hello**')
    expect(bold.getAttribute('aria-pressed')).toBe('true')
  })

  it('applies with Ctrl+Enter', async () => {
    const shortcut = mountDialog('shortcut text')
    await flushPromises()
    getTextarea().dispatchEvent(
      new KeyboardEvent('keydown', {
        bubbles: true,
        key: 'Enter',
        ctrlKey: true,
      }),
    )
    expect(shortcut.emitted('apply')).toEqual([['shortcut text']])
  })

  it('applies when the close button is clicked', async () => {
    const wrapper = mountDialog('close text')
    await flushPromises()
    document
      .querySelector<HTMLButtonElement>('.long-text-dialog__close')
      ?.click()
    expect(wrapper.emitted('apply')).toEqual([['close text']])
  })

  it('applies when the dialog backdrop is clicked', async () => {
    const wrapper = mountDialog('backdrop text')
    await flushPromises()
    const dialog = getDialog()
    pointerClick(dialog, dialog, { clientX: 10, clientY: 10 })
    expect(wrapper.emitted('apply')).toEqual([['backdrop text']])
  })

  it('does not apply on dialog chrome or selection drags out of the textarea', async () => {
    const wrapper = mountDialog()
    await flushPromises()
    const dialog = getDialog()
    const textarea = getTextarea()
    textarea.value = 'draft selection'
    textarea.dispatchEvent(new Event('input', { bubbles: true }))

    pointerClick(textarea, textarea, { clientX: 200, clientY: 200 })
    pointerClick(dialog, dialog, { clientX: 110, clientY: 200 })
    pointerClick(textarea, dialog, { clientX: 200, clientY: 200 })
    pointerClick(
      textarea,
      dialog,
      { clientX: 200, clientY: 200 },
      { clientX: 10, clientY: 10 },
    )
    pointerClick(
      dialog,
      dialog,
      { clientX: 10, clientY: 10 },
      { clientX: 110, clientY: 200 },
    )

    expect(dialog.open).toBe(true)
    expect(textarea.value).toBe('draft selection')
    expect(wrapper.emitted('apply')).toBeUndefined()
    expect(wrapper.emitted('cancel')).toBeUndefined()
    textarea.dispatchEvent(
      new KeyboardEvent('keydown', { bubbles: true, key: 'Escape' }),
    )
    await nextTick()
    expect(document.body.textContent).toContain('Discard changes?')
  })

  it('cancels on clean Escape', async () => {
    const clean = mountDialog()
    await flushPromises()
    getTextarea().dispatchEvent(
      new KeyboardEvent('keydown', { bubbles: true, key: 'Escape' }),
    )
    expect(clean.emitted('cancel')).toEqual([[]])
  })

  it('confirms before discarding dirty changes on Escape', async () => {
    const dirty = mountDialog()
    await flushPromises()
    const textarea = getTextarea()
    textarea.value = 'changed'
    textarea.dispatchEvent(new Event('input', { bubbles: true }))
    textarea.dispatchEvent(
      new KeyboardEvent('keydown', { bubbles: true, key: 'Escape' }),
    )
    await nextTick()
    expect(document.body.textContent).toContain('Discard changes?')
    const discard = [...document.querySelectorAll('button')].find(
      (button) => button.textContent?.trim() === 'Discard',
    )
    if (!discard) throw new Error('Discard button was not mounted')
    discard.click()
    expect(dirty.emitted('cancel')).toEqual([[]])
  })

  it('contains its toolbar and stops keydown from bubbling out', async () => {
    const wrapper = mountDialog()
    await flushPromises()
    const button = document.querySelector('.long-text-dialog__toolbar button')
    if (!button) throw new Error('Toolbar button was not mounted')
    expect(wrapper.vm.contains(button)).toBe(true)

    const bubbled = vi.fn()
    document.body.addEventListener('keydown', bubbled)
    getTextarea().dispatchEvent(
      new KeyboardEvent('keydown', { bubbles: true, key: 'x' }),
    )
    document.body.removeEventListener('keydown', bubbled)
    expect(bubbled).not.toHaveBeenCalled()
  })
})
