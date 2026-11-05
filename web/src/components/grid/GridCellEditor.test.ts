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
  attachTo?: Element,
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
    attachTo,
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

const choices = {
  choices: [
    { id: 'a', label: 'Alpha', color: 'red' },
    { id: 'b', label: 'Beta', color: 'blue' },
    { id: 'c', label: 'Gamma', color: 'green' },
  ],
}

function option(
  wrapper: ReturnType<typeof mountEditor>['wrapper'],
  label: string,
) {
  return wrapper.get(`[role="option"][data-choice="${label}"]`)
}

describe('grid choice editor (UX-08)', () => {
  it('opens a focused single-select list and commits a picked label once', async () => {
    const { wrapper, save, close, saveDraft } = mountEditor(
      'single_select',
      'Alpha',
      choices,
      undefined,
      document.body,
    )
    try {
      const list = wrapper.get('[role="listbox"]')
      expect(document.activeElement).toBe(list.element)
      expect(option(wrapper, 'Alpha').attributes('aria-selected')).toBe('true')
      expect(list.attributes('aria-multiselectable')).toBeUndefined()
      await option(wrapper, 'Beta').trigger('click')
      expect(saveDraft).toHaveBeenCalledExactlyOnceWith('Beta', true)
      expect(save).toHaveBeenCalledExactlyOnceWith('Beta', true)
      expect(close).toHaveBeenCalledOnce()
      expect(wrapper.vm.leave()).toBe(true)
      expect(save).toHaveBeenCalledOnce()
    } finally {
      wrapper.unmount()
    }
  })

  it('clears a single select through the no-selection option', async () => {
    const { wrapper, save } = mountEditor('single_select', 'Alpha', choices)
    await option(wrapper, '').trigger('click')
    expect(save).toHaveBeenCalledExactlyOnceWith(null, true)
  })

  it('does not write when the picked or left selection is unchanged', async () => {
    const picked = mountEditor('single_select', 'Alpha', choices)
    await option(picked.wrapper, 'Alpha').trigger('click')
    expect(picked.save).not.toHaveBeenCalled()
    expect(picked.close).toHaveBeenCalledOnce()

    const left = mountEditor('multi_select', ['Alpha'], choices)
    expect(left.wrapper.vm.leave()).toBe(true)
    expect(left.save).not.toHaveBeenCalled()
    expect(left.close).toHaveBeenCalledOnce()
  })

  it('keeps a multi-select open across several changes and commits once on leave', async () => {
    const { wrapper, save, close, saveDraft } = mountEditor(
      'multi_select',
      ['Alpha'],
      choices,
    )
    expect(
      wrapper.get('[role="listbox"]').attributes('aria-multiselectable'),
    ).toBe('true')
    await option(wrapper, 'Gamma').trigger('click')
    await option(wrapper, 'Alpha').trigger('click')
    await option(wrapper, 'Beta').trigger('click')
    expect(save).not.toHaveBeenCalled()
    expect(close).not.toHaveBeenCalled()
    expect(option(wrapper, 'Alpha').attributes('aria-selected')).toBe('false')
    expect(option(wrapper, 'Gamma').attributes('aria-selected')).toBe('true')

    const inside = option(wrapper, 'Beta').element
    await wrapper.get('[role="listbox"]').trigger('focusout', {
      relatedTarget: inside,
    })
    expect(save).not.toHaveBeenCalled()

    expect(wrapper.vm.leave()).toBe(true)
    await wrapper.get('[role="listbox"]').trigger('focusout', {
      relatedTarget: document.body,
    })
    expect(saveDraft).toHaveBeenCalledExactlyOnceWith('Gamma, Beta', true)
    // The codec stores labels in configured choice order.
    expect(save).toHaveBeenCalledExactlyOnceWith(['Beta', 'Gamma'], true)
    expect(close).toHaveBeenCalledOnce()
  })

  it('cancels a multi-select draft with Escape', async () => {
    const { wrapper, save, close, discardDraft } = mountEditor(
      'multi_select',
      ['Alpha'],
      choices,
    )
    await option(wrapper, 'Beta').trigger('click')
    await wrapper.get('[role="listbox"]').trigger('keydown', { key: 'Escape' })
    expect(wrapper.vm.leave()).toBe(true)
    expect(save).not.toHaveBeenCalled()
    expect(discardDraft).toHaveBeenCalledOnce()
    expect(close).toHaveBeenCalledOnce()
  })

  it('navigates options with the keyboard without leaking keys to the grid', async () => {
    const single = mountEditor('single_select', null, choices)
    const parent = vi.fn()
    single.wrapper.element.parentElement?.addEventListener('keydown', parent)
    const list = single.wrapper.get('[role="listbox"]')
    expect(list.attributes('aria-activedescendant')).toBe(
      option(single.wrapper, '').attributes('id'),
    )
    await list.trigger('keydown', { key: 'ArrowDown' })
    await list.trigger('keydown', { key: 'ArrowDown' })
    await list.trigger('keydown', { key: 'ArrowUp' })
    expect(list.attributes('aria-activedescendant')).toBe(
      option(single.wrapper, 'Alpha').attributes('id'),
    )
    await list.trigger('keydown', { key: 'End' })
    await list.trigger('keydown', { key: 'Enter' })
    expect(single.save).toHaveBeenCalledExactlyOnceWith('Gamma', false)
    expect(parent).not.toHaveBeenCalled()

    const multi = mountEditor('multi_select', null, choices)
    const multiList = multi.wrapper.get('[role="listbox"]')
    await multiList.trigger('keydown', { key: ' ' })
    await multiList.trigger('keydown', { key: 'ArrowDown' })
    await multiList.trigger('keydown', { key: ' ' })
    expect(multi.save).not.toHaveBeenCalled()
    await multiList.trigger('keydown', { key: 'Enter' })
    expect(multi.save).toHaveBeenCalledExactlyOnceWith(['Alpha', 'Beta'], false)
  })

  it('keeps unconfigured stored labels visible and never silently replaces them', async () => {
    const kept = mountEditor('multi_select', ['Retired'], choices)
    const retired = option(kept.wrapper, 'Retired')
    expect(retired.attributes('aria-selected')).toBe('true')
    expect(retired.find('.chip-unknown').exists()).toBe(true)
    expect(kept.wrapper.vm.leave()).toBe(true)
    expect(kept.save).not.toHaveBeenCalled()

    const invalid = mountEditor('multi_select', ['Retired'], choices)
    await option(invalid.wrapper, 'Alpha').trigger('click')
    expect(invalid.wrapper.vm.leave()).toBe(true)
    expect(invalid.save).not.toHaveBeenCalled()
    expect(invalid.saveDraft).toHaveBeenCalledExactlyOnceWith(
      'Retired, Alpha',
      false,
      i18n.global.t('field.error.unknown_choice'),
    )

    const removed = mountEditor('multi_select', ['Retired', 'Alpha'], choices)
    await option(removed.wrapper, 'Retired').trigger('click')
    removed.wrapper.vm.leave()
    expect(removed.save).toHaveBeenCalledExactlyOnceWith(['Alpha'], true)

    const single = mountEditor('single_select', 'Retired', choices)
    expect(option(single.wrapper, 'Retired').attributes('aria-selected')).toBe(
      'true',
    )
    single.wrapper.vm.leave()
    expect(single.save).not.toHaveBeenCalled()
  })

  it('restores a recoverable choice draft and reports unavailable writes', async () => {
    const reopened = mountEditor(
      'multi_select',
      ['Alpha'],
      choices,
      'Alpha, Beta',
    )
    expect(option(reopened.wrapper, 'Beta').attributes('aria-selected')).toBe(
      'true',
    )
    reopened.wrapper.vm.leave()
    expect(reopened.save).toHaveBeenCalledExactlyOnceWith(
      ['Alpha', 'Beta'],
      true,
    )

    const field = bindField('single_select', choices)
    if (!field.ok) throw new Error(field.reason)
    const save = vi.fn()
    const saveDraft = vi.fn()
    const close = vi.fn()
    const wrapper = mount(GridCellEditor, {
      props: {
        column: { prop: 'field' } as EditorType['column'],
        val: 'Alpha',
        save,
        close,
        boundField: field.value,
        fieldEnv: { timeZone: 'UTC', locale: 'en' },
        tableId: 'table',
        fieldId: 'field',
        identity: 'alice',
        canCommit: () => false,
        unavailableMessage: () => 'Unavailable',
        saveDraft,
        discardDraft: vi.fn(),
      },
      global: { plugins: [i18n] },
    })
    await wrapper.get('[role="option"][data-choice="Beta"]').trigger('click')
    expect(save).not.toHaveBeenCalled()
    expect(saveDraft).toHaveBeenCalledExactlyOnceWith(
      'Beta',
      false,
      'Unavailable',
    )
    expect(close).toHaveBeenCalledOnce()
  })

  it('prefers the stored row value over a typed key that opened the editor', () => {
    const field = bindField('single_select', choices)
    if (!field.ok) throw new Error(field.reason)
    const wrapper = mount(GridCellEditor, {
      props: {
        column: {
          prop: 'field',
          model: { field: 'Gamma' },
        } as unknown as EditorType['column'],
        val: 'x',
        save: vi.fn(),
        close: vi.fn(),
        boundField: field.value,
        fieldEnv: { timeZone: 'UTC', locale: 'en' },
        tableId: 'table',
        fieldId: 'field',
        identity: 'alice',
        canCommit: () => true,
        unavailableMessage: () => 'Unavailable',
        saveDraft: vi.fn(),
        discardDraft: vi.fn(),
      },
      global: { plugins: [i18n] },
    })
    expect(
      wrapper
        .get('[role="option"][data-choice="Gamma"]')
        .attributes('aria-selected'),
    ).toBe('true')
  })
})
