import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { EditorType } from '@revolist/vue3-datagrid'
import { bindField } from '../../fields/registry'
import type { WireValue } from '../../fields/types'
import { i18n, loadLocale } from '../../i18n'
import GridCellEditor from './GridCellEditor.vue'

beforeEach(async () => {
  await loadLocale('en')
  await loadLocale('zh-CN')
  i18n.global.locale.value = 'en'
})

function mountEditor(
  kind: string,
  value: WireValue,
  options: unknown = {},
  initialDraft?: string,
) {
  const field = bindField(kind, options)
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
  })
  return { wrapper, save, close, saveDraft, discardDraft }
}

describe('grid scalar editor lifecycle', () => {
  it('commits once on leave and ignores the resulting blur', async () => {
    const { wrapper, save, close, saveDraft } = mountEditor('text', 'before')
    const input = wrapper.get('input')
    await input.setValue('after')
    expect(wrapper.vm.leave()).toBe(true)
    await input.trigger('focusout', { relatedTarget: document.body })
    expect(saveDraft).toHaveBeenCalledExactlyOnceWith('after', true)
    expect(save).toHaveBeenCalledExactlyOnceWith('after', true)
    expect(close).toHaveBeenCalledOnce()
  })

  it('does not double-save Enter or save after Escape', async () => {
    const entered = mountEditor('text', 'before')
    await entered.wrapper.get('input').setValue('after')
    await entered.wrapper.get('input').trigger('keydown', { key: 'Enter' })
    await entered.wrapper.get('input').trigger('focusout', {
      relatedTarget: document.body,
    })
    expect(entered.save).toHaveBeenCalledExactlyOnceWith('after', false)

    const cancelled = mountEditor('text', 'before', {}, 'recoverable')
    await cancelled.wrapper.get('input').setValue('discard')
    await cancelled.wrapper.get('input').trigger('keydown', { key: 'Escape' })
    await cancelled.wrapper.get('input').trigger('focusout', {
      relatedTarget: document.body,
    })
    expect(cancelled.save).not.toHaveBeenCalled()
    expect(cancelled.discardDraft).toHaveBeenCalledOnce()
    expect(cancelled.close).toHaveBeenCalledOnce()
  })

  it('retains invalid raw text and restores it when reopened', async () => {
    const invalid = mountEditor('integer', 7)
    await invalid.wrapper.get('input').setValue('abc')
    expect(invalid.wrapper.vm.leave()).toBe(true)
    expect(invalid.save).not.toHaveBeenCalled()
    expect(invalid.close).toHaveBeenCalledOnce()
    expect(invalid.saveDraft).toHaveBeenCalledExactlyOnceWith(
      'abc',
      false,
      i18n.global.t('field.error.invalid_integer'),
    )
    const reopened = mountEditor('integer', 7, {}, 'abc')
    expect(
      (reopened.wrapper.get('input').element as HTMLInputElement).value,
    ).toBe('abc')
  })

  it('keeps invalid Enter feedback in the editor until the user leaves', async () => {
    const invalid = mountEditor('integer', 7)
    const input = invalid.wrapper.get('input')
    await input.setValue('abc')
    await input.trigger('keydown', { key: 'Enter' })
    expect(invalid.wrapper.get('[role="alert"]').text()).toBe(
      i18n.global.t('field.error.invalid_integer'),
    )
    expect(invalid.save).not.toHaveBeenCalled()
    expect(invalid.saveDraft).not.toHaveBeenCalled()
    expect(invalid.close).not.toHaveBeenCalled()

    expect(invalid.wrapper.vm.leave()).toBe(true)
    expect(invalid.saveDraft).toHaveBeenCalledExactlyOnceWith(
      'abc',
      false,
      i18n.global.t('field.error.invalid_integer'),
    )
    expect(invalid.close).toHaveBeenCalledOnce()
  })

  it('keeps focus moves inside the editor and IME composition from committing', async () => {
    const { wrapper, save, close } = mountEditor('text', 'before')
    const input = wrapper.get('input')
    const inside = document.createElement('button')
    wrapper.element.appendChild(inside)
    await input.trigger('focusout', { relatedTarget: inside })
    expect(save).not.toHaveBeenCalled()

    await input.trigger('compositionstart')
    ;(input.element as HTMLInputElement).value = '中文'
    await input.trigger('input')
    await input.trigger('keydown', { key: 'Enter', isComposing: true })
    await input.trigger('keydown', { key: 'Escape', isComposing: true })
    expect(wrapper.vm.leave()).toBe(false)
    expect(save).not.toHaveBeenCalled()
    expect(close).not.toHaveBeenCalled()
    await input.trigger('compositionend')
    await flushPromises()
    expect(save).toHaveBeenCalledExactlyOnceWith('中文', true)
  })

  it('allows multiline Enter and commits the complete value on leave', async () => {
    const { wrapper, save } = mountEditor('long_text', 'one')
    const textarea = wrapper.get('textarea')
    await textarea.setValue('one\n中文')
    await textarea.trigger('keydown', { key: 'Enter' })
    expect(save).not.toHaveBeenCalled()
    wrapper.vm.leave()
    expect(save).toHaveBeenCalledExactlyOnceWith('one\n中文', true)
  })

  it.each([
    ['integer', { size: 'big' }, '9223372036854775807', '9223372036854775807'],
    [
      'decimal',
      { precision: 38, scale: 6 },
      '123456789012345678901234567890.123456',
      '123456789012345678901234567890.123456',
    ],
    [
      'datetime',
      {},
      '2026-10-28 12:34:56.123456+00:00',
      '2026-10-28T12:34:56.123456Z',
    ],
  ])(
    'preserves exact %s wire values on leave',
    async (kind, options, text, wire) => {
      const { wrapper, save } = mountEditor(kind, null, options)
      await wrapper.get('input').setValue(text)
      wrapper.vm.leave()
      expect(save).toHaveBeenCalledExactlyOnceWith(wire, true)
    },
  )
})
