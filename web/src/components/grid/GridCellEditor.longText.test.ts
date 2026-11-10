import {
  afterAll,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { EditorType } from '@revolist/vue3-datagrid'
import { bindField } from '../../fields/registry'
import type { WireValue } from '../../fields/types'
import { i18n, loadLocale } from '../../i18n'
import FieldEditor from '../fields/FieldEditor.vue'
import GridCellEditor from './GridCellEditor.vue'

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

afterAll(() => {
  HTMLDialogElement.prototype.showModal = nativeShowModal
  HTMLDialogElement.prototype.close = nativeClose
})

function mountEditor(value: WireValue, initialDraft?: string) {
  const field = bindField('long_text', {})
  if (!field.ok) throw new Error(field.reason)
  const save = vi.fn()
  const close = vi.fn()
  const saveDraft = vi.fn()
  const discardDraft = vi.fn()
  const wrapper = mount(GridCellEditor, {
    props: {
      column: { prop: 'field' } as EditorType['column'],
      val: value,
      save,
      close,
      boundField: field.value,
      fieldEnv: { timeZone: 'UTC', locale: 'en' },
      tableId: 'table',
      fieldId: 'field',
      identity: 'alice',
      initialDraft,
      canCommit: () => true,
      unavailableMessage: () => 'Unavailable',
      saveDraft,
      discardDraft,
    },
    global: { plugins: [i18n] },
    attachTo: document.body,
  })
  return { wrapper, save, close, saveDraft, discardDraft }
}

function dialogTextarea() {
  const textarea = document.querySelector<HTMLTextAreaElement>(
    'dialog.long-text-dialog[open] .long-text-dialog__textarea',
  )
  if (!textarea) throw new Error('Long-text dialog was not mounted')
  return textarea
}

describe('grid long text editor', () => {
  it('keeps focus within the dialog and commits its applied draft', async () => {
    const { wrapper, save, close } = mountEditor('stored')
    const textarea = wrapper.get('.field-editor textarea')
    await textarea.setValue('inline draft')
    ;(textarea.element as HTMLTextAreaElement).focus()
    await wrapper.get('.field-expand').trigger('click')
    await flushPromises()
    const dialog = dialogTextarea()

    expect(wrapper.findComponent(FieldEditor).vm.isPointerInPanel()).toBe(true)
    await textarea.trigger('focusout', { relatedTarget: dialog })
    expect(save).not.toHaveBeenCalled()
    expect(close).not.toHaveBeenCalled()

    dialog.value = 'dialog draft'
    dialog.dispatchEvent(new Event('input', { bubbles: true }))
    dialog.dispatchEvent(
      new KeyboardEvent('keydown', {
        key: 'Enter',
        ctrlKey: true,
        bubbles: true,
      }),
    )

    expect(save).toHaveBeenCalledExactlyOnceWith('dialog draft', false)
    expect(close).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('discards dialog changes and restores the inline draft and focus', async () => {
    const { wrapper, save, close } = mountEditor('stored', 'original draft')
    const textarea = wrapper.get('.field-editor textarea')
    ;(textarea.element as HTMLTextAreaElement).focus()
    ;(textarea.element as HTMLTextAreaElement).setSelectionRange(2, 6)
    await wrapper.get('.field-expand').trigger('click')
    await flushPromises()
    const dialog = dialogTextarea()
    dialog.value = 'discarded dialog draft'
    dialog.dispatchEvent(new Event('input', { bubbles: true }))
    dialog.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }),
    )
    await flushPromises()

    const discard = document.querySelector<HTMLButtonElement>(
      '.long-text-dialog__confirm button',
    )
    if (!discard) throw new Error('Discard confirmation was not mounted')
    discard.click()

    expect((textarea.element as HTMLTextAreaElement).value).toBe(
      'original draft',
    )
    expect(document.activeElement).toBe(textarea.element)
    expect((textarea.element as HTMLTextAreaElement).selectionStart).toBe(2)
    expect((textarea.element as HTMLTextAreaElement).selectionEnd).toBe(6)
    expect(save).not.toHaveBeenCalled()
    expect(close).not.toHaveBeenCalled()
    expect(wrapper.find('.field-editor').exists()).toBe(true)
    wrapper.unmount()
  })
})
