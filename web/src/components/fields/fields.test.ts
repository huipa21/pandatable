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
import { config, flushPromises, mount } from '@vue/test-utils'
import { i18n, loadLocale } from '../../i18n'
import { bindField, type BoundField } from '../../fields/registry'
import { fieldErrorMessage, fieldKindLabel } from '../../fields/messages'
import { KINDS } from '../../fields/registry'
import type { Reason } from '../../fields/types'
import FieldRenderer from './FieldRenderer.vue'
import FieldEditor from './FieldEditor.vue'
import { invalidateUserLabels } from '../../fields/userDirectory'

config.global.plugins = [i18n]

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
  await loadLocale('zh-CN')
  i18n.global.locale.value = 'en'
})
afterEach(() => vi.unstubAllGlobals())

afterAll(() => {
  HTMLDialogElement.prototype.showModal = nativeShowModal
  HTMLDialogElement.prototype.close = nativeClose
})

function field(kind: string, options: unknown = null): BoundField {
  const bound = bindField(kind, options)
  if (!bound.ok) throw new Error(bound.reason)
  return bound.value
}

const choices = {
  choices: [
    { id: 'a', label: 'Alpha', color: 'red' },
    { id: 'b', label: 'Beta' },
  ],
}

describe('FieldRenderer', () => {
  it('can render safe links as plain text inside a card button', () => {
    const wrapper = mount(FieldRenderer, {
      props: {
        field: field('url'),
        value: 'https://example.com',
        links: false,
      },
    })
    expect(wrapper.find('a').exists()).toBe(false)
    expect(wrapper.text()).toBe('https://example.com')
  })
  it('renders text by interpolation only', () => {
    const wrapper = mount(FieldRenderer, {
      props: { field: field('text'), value: '<img src=x onerror=alert(1)>' },
    })
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.text()).toBe('<img src=x onerror=alert(1)>')
  })

  it('links only safe URLs and email addresses', () => {
    const safe = mount(FieldRenderer, {
      props: { field: field('url'), value: 'https://example.com/x' },
    })
    const link = safe.get('a')
    expect(link.attributes()).toMatchObject({
      href: 'https://example.com/x',
      target: '_blank',
      rel: 'noopener noreferrer nofollow',
    })
    const unsafe = mount(FieldRenderer, {
      props: { field: field('url'), value: 'javascript:alert(1)' },
    })
    expect(unsafe.find('a').exists()).toBe(false)
    expect(unsafe.text()).toBe('javascript:alert(1)')
    const mail = mount(FieldRenderer, {
      props: { field: field('email'), value: 'a@example.com' },
    })
    expect(mail.get('a').attributes('href')).toBe('mailto:a@example.com')
    const mailQuery = mount(FieldRenderer, {
      props: {
        field: field('email'),
        value: 'a?subject=other&body=x#tag@example.com',
      },
    })
    expect(mailQuery.get('a').attributes('href')).toBe(
      'mailto:a%3Fsubject%3Dother%26body%3Dx%23tag@example.com',
    )
  })

  it('renders choices, unknown labels and checkboxes', () => {
    const wrapper = mount(FieldRenderer, {
      props: {
        field: field('multi_select', choices),
        value: ['Alpha', 'Gone'],
      },
    })
    const chips = wrapper.findAll('.field-chip')
    expect(chips.map((chip) => chip.text())).toEqual(['Alpha', 'Gone'])
    expect(chips[0]?.classes()).toContain('chip-red')
    expect(chips[1]?.classes()).toContain('chip-unknown')
    expect(chips[1]?.attributes('title')).toBe(
      'This option is no longer configured',
    )
    const box = mount(FieldRenderer, {
      props: { field: field('checkbox'), value: true },
    })
    expect(box.get('[role="img"]').attributes('aria-label')).toBe('Checked')
    expect(box.get('.field-check').classes()).toContain('checked')
    expect(box.find('.field-check svg').exists()).toBe(false)
  })

  it('renders a checked checkbox star and its unchecked outline', async () => {
    const wrapper = mount(FieldRenderer, {
      props: { field: field('checkbox', { icon: 'star' }), value: true },
    })
    const icon = wrapper.get('.field-check')
    expect(icon.attributes('data-icon')).toBe('star')
    expect(icon.classes()).toContain('checked')
    expect(icon.get('svg path').attributes('d')).toMatch(/^M12 3l3 6/)
    expect(icon.attributes('aria-label')).toBe('Checked')

    await wrapper.setProps({ value: false })
    expect(icon.classes()).not.toContain('checked')
    expect(icon.find('svg path').exists()).toBe(true)
    expect(icon.attributes('aria-label')).toBe('Not checked')
  })

  it('formats numbers and datetimes for display', () => {
    const big = mount(FieldRenderer, {
      props: {
        field: field('integer', { size: 'big' }),
        value: '9223372036854775807',
      },
    })
    expect(big.text()).toBe('9,223,372,036,854,775,807')
    const time = mount(FieldRenderer, {
      props: {
        field: field('datetime', { timeFormat: '12h', dateFormat: 'us' }),
        value: '2026-07-04T16:05:00Z',
        env: { timeZone: 'Asia/Shanghai' },
      },
    })
    expect(time.text()).toBe('07/05/2026 12:05 AM')
  })

  it('resolves system users through the caller and localizes unknown users', async () => {
    const id = '0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b'
    const known = mount(FieldRenderer, {
      props: {
        field: field('created_by'),
        value: id,
        resolveUser: () => 'Alice',
      },
    })

    expect(known.text()).toBe('Alice')
    const unknown = mount(FieldRenderer, {
      props: { field: field('updated_by'), value: id },
    })
    expect(unknown.text()).toBe('Unknown user (0190a1b2)')
    expect(unknown.classes()).toContain('field-readonly')
    i18n.global.locale.value = 'zh-CN'
    await unknown.vm.$nextTick()
    expect(unknown.text()).toBe('未知用户（0190a1b2）')
  })

  it('refreshes cached authorship as a localized deleted user after navigation changes', async () => {
    const id = '0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b'
    let removed = false
    const fetch = vi.fn(async () =>
      Response.json({
        items: [
          removed ? { id, name: '', deleted: true } : { id, name: 'Alice' },
        ],
      }),
    )
    vi.stubGlobal('fetch', fetch)
    invalidateUserLabels()
    const wrapper = mount(FieldRenderer, {
      props: {
        field: field('created_by'),
        value: id,
        identity: 'viewer',
        tableId: 'table',
      },
    })
    try {
      await flushPromises()
      expect(wrapper.text()).toBe('Alice')
      removed = true
      invalidateUserLabels()
      await flushPromises()
      expect(wrapper.text()).toBe('Deleted user')
      i18n.global.locale.value = 'zh-CN'
      await wrapper.vm.$nextTick()
      expect(wrapper.text()).toBe('已删除的用户')
    } finally {
      wrapper.unmount()
    }
  })

  it('does not restore a deleted name from an in-flight stale directory response', async () => {
    const id = '0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b'
    let finishOld!: (response: Response) => void
    let finishNew!: (response: Response) => void
    const fetch = vi
      .fn()
      .mockImplementationOnce(
        () => new Promise<Response>((resolve) => (finishOld = resolve)),
      )
      .mockImplementationOnce(
        () => new Promise<Response>((resolve) => (finishNew = resolve)),
      )
    vi.stubGlobal('fetch', fetch)
    invalidateUserLabels()
    const wrapper = mount(FieldRenderer, {
      props: {
        field: field('created_by'),
        value: id,
        identity: 'viewer',
        tableId: 'table',
      },
    })
    try {
      await flushPromises()
      expect(fetch).toHaveBeenCalledTimes(1)
      invalidateUserLabels()
      await flushPromises()
      expect(fetch).toHaveBeenCalledTimes(2)
      finishNew(Response.json({ items: [{ id, name: '', deleted: true }] }))
      await flushPromises()
      expect(wrapper.text()).toBe('Deleted user')
      finishOld(Response.json({ items: [{ id, name: 'Alice' }] }))
      await flushPromises()
      expect(wrapper.text()).toBe('Deleted user')
    } finally {
      wrapper.unmount()
    }
  })
})

describe('FieldEditor', () => {
  it('renders nothing for read-only system kinds', () => {
    for (const kind of [
      'id',
      'created_at',
      'created_by',
      'updated_at',
      'updated_by',
    ]) {
      const wrapper = mount(FieldEditor, {
        props: { field: field(kind), value: null },
      })
      expect(wrapper.find('input, textarea, select').exists()).toBe(false)
    }
  })

  it('commits parsed values and shows localized errors', async () => {
    const wrapper = mount(FieldEditor, {
      props: {
        field: field('decimal', { precision: 5, scale: 2 }),
        value: '1.50',
      },
    })
    const input = wrapper.get('input')
    expect((input.element as HTMLInputElement).value).toBe('1.50')
    await input.setValue('1.234')
    await input.trigger('keydown', { key: 'Enter' })
    expect(wrapper.get('[role="alert"]').text()).toBe(
      'Too many decimal places for this field.',
    )
    expect(input.attributes('aria-invalid')).toBe('true')
    expect(wrapper.emitted('commit')).toBeUndefined()
    await input.setValue('1,23.4')
    await input.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('commit')).toEqual([['123.40']])
    await input.trigger('keydown', { key: 'Escape' })
    expect(wrapper.emitted('cancel')).toHaveLength(1)
  })

  it('edits datetimes in the display zone and rejects DST gaps', async () => {
    const wrapper = mount(FieldEditor, {
      props: {
        field: field('datetime'),
        value: '2026-03-08T06:30:00Z',
        env: { timeZone: 'America/New_York' },
      },
    })
    const input = wrapper.get('input')
    expect((input.element as HTMLInputElement).value).toBe('2026-03-08 01:30')
    await input.setValue('2026-03-08 02:30')
    await input.trigger('keydown', { key: 'Enter' })
    expect(wrapper.get('[role="alert"]').text()).toContain('daylight-saving')
    await input.setValue('2026-03-08 03:30')
    await input.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('commit')).toEqual([['2026-03-08T07:30:00Z']])
  })

  it('commits selects in choice order and long text only with Ctrl+Enter', async () => {
    const multi = mount(FieldEditor, {
      props: { field: field('multi_select', choices), value: ['Beta'] },
    })
    const select = multi.get('select')
    for (const option of select.findAll('option'))
      (option.element as HTMLOptionElement).selected = true
    await select.trigger('change')
    await multi.get('button').trigger('click')
    expect(multi.emitted('commit')).toEqual([[['Alpha', 'Beta']]])
    const single = mount(FieldEditor, {
      props: { field: field('single_select', choices), value: null },
    })
    await single.get('select').setValue('Beta')
    expect(single.emitted('commit')).toEqual([['Beta']])
    const long = mount(FieldEditor, {
      props: { field: field('long_text'), value: 'a' },
    })
    const area = long.get('textarea')
    await area.setValue('a\r\nb')
    await area.trigger('keydown', { key: 'Enter' })
    expect(long.emitted('commit')).toBeUndefined()
    await area.trigger('keydown', { key: 'Enter', ctrlKey: true })
    expect(long.emitted('commit')).toEqual([['a\nb']])
  })

  it.each([false, true])(
    'applies expanded long text with grid mode %s',
    async (gridCommitOnLeave) => {
      const wrapper = mount(FieldEditor, {
        attachTo: document.body,
        props: {
          field: field('long_text'),
          value: 'stored value',
          label: 'Notes',
          gridCommitOnLeave,
        },
      })
      const area = wrapper.get('textarea')
      await area.setValue('current draft')
      const expand = wrapper.get('.field-expand')
      expect(expand.attributes('aria-label')).toBe('Expand Notes')
      await expand.trigger('click')
      await flushPromises()

      const dialogArea = document.querySelector<HTMLTextAreaElement>(
        'dialog.long-text-dialog[open] .long-text-dialog__textarea',
      )
      expect(dialogArea?.value).toBe('current draft')
      if (!dialogArea) throw new Error('Long-text dialog was not mounted')
      dialogArea.value = 'applied draft'
      dialogArea.dispatchEvent(new Event('input', { bubbles: true }))
      dialogArea.dispatchEvent(
        new KeyboardEvent('keydown', {
          key: 'Enter',
          ctrlKey: true,
          bubbles: true,
        }),
      )

      expect((area.element as HTMLTextAreaElement).value).toBe('applied draft')
      if (gridCommitOnLeave) {
        expect(wrapper.emitted('commit')).toEqual([['applied draft']])
      } else {
        expect(wrapper.emitted('draftChanged')).toHaveLength(1)
        expect(wrapper.emitted('commit')).toBeUndefined()
      }
      wrapper.unmount()
    },
  )

  it('keeps opening-modal blur inside a live field and restores its draft on discard', async () => {
    const wrapper = mount(FieldEditor, {
      attachTo: document.body,
      props: { field: field('long_text'), value: 'stored value', live: true },
    })
    const area = wrapper.get('textarea')
    await area.setValue('inline draft')
    const inline = area.element as HTMLTextAreaElement
    inline.focus()
    inline.setSelectionRange(2, 6)
    wrapper
      .get('.field-expand')
      .element.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    // Native modal focus transfer can blur the opener before setting a target.
    inline.dispatchEvent(new FocusEvent('focusout', { bubbles: true }))
    expect(wrapper.emitted('commit')).toBeUndefined()
    await flushPromises()

    const expanded = document.querySelector<HTMLTextAreaElement>(
      'dialog.long-text-dialog[open] .long-text-dialog__textarea',
    )
    if (!expanded) throw new Error('Long-text dialog was not mounted')
    expanded.value = 'discard this draft'
    expanded.dispatchEvent(new Event('input', { bubbles: true }))
    expanded.dispatchEvent(
      new KeyboardEvent('keydown', { bubbles: true, key: 'Escape' }),
    )
    await flushPromises()
    expect(wrapper.emitted('commit')).toBeUndefined()
    expect(wrapper.emitted('cancel')).toBeUndefined()
    const discard = [...document.querySelectorAll('dialog button')].find(
      (button) => button.textContent?.trim() === 'Discard',
    ) as HTMLButtonElement | undefined
    if (!discard) throw new Error('Discard button was not mounted')
    discard.click()
    await flushPromises()

    expect(document.querySelector('dialog.long-text-dialog[open]')).toBeNull()
    expect(inline.value).toBe('inline draft')
    expect([inline.selectionStart, inline.selectionEnd]).toEqual([2, 6])
    expect(document.activeElement).toBe(inline)
    expect(wrapper.emitted('commit')).toBeUndefined()
    expect(wrapper.emitted('cancel')).toBeUndefined()
    wrapper.unmount()
  })

  it('opens the long text dialog by shortcut outside IME composition', async () => {
    const wrapper = mount(FieldEditor, {
      attachTo: document.body,
      props: { field: field('long_text'), value: 'stored value' },
    })
    const area = wrapper.get('textarea')
    ;(area.element as HTMLTextAreaElement).focus()

    await area.trigger('compositionstart')
    await area.trigger('keydown', { key: 'F2' })
    expect(document.querySelector('dialog.long-text-dialog[open]')).toBeNull()
    await area.trigger('compositionend')
    await area.trigger('keydown', { key: 'F2' })
    await flushPromises()
    expect(
      document.querySelector('dialog.long-text-dialog[open]'),
    ).not.toBeNull()

    const dialogArea = document.querySelector<HTMLTextAreaElement>(
      'dialog.long-text-dialog[open] .long-text-dialog__textarea',
    )
    if (!dialogArea) throw new Error('Long-text dialog was not mounted')
    dialogArea.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }),
    )
    await flushPromises()
    await area.trigger('keydown', {
      key: 'e',
      ctrlKey: true,
      shiftKey: true,
    })
    await flushPromises()
    expect(
      document.querySelector('dialog.long-text-dialog[open]'),
    ).not.toBeNull()
    wrapper.unmount()
  })

  it('toggles checkboxes immediately', async () => {
    const wrapper = mount(FieldEditor, {
      props: { field: field('checkbox', { icon: 'heart' }), value: false },
    })
    const button = wrapper.get('button[role="checkbox"]')
    expect(button.attributes('aria-checked')).toBe('false')
    expect(button.get('.field-check').attributes('data-icon')).toBe('heart')
    await button.trigger('keydown', { key: ' ' })
    expect(wrapper.emitted('commit')).toEqual([[true]])
    expect(button.attributes('aria-checked')).toBe('true')
    await button.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('commit')).toEqual([[true], [false]])
    expect(button.attributes('aria-checked')).toBe('false')
    await button.trigger('click')
    expect(wrapper.emitted('commit')).toEqual([[true], [false], [true]])
  })

  it('does not commit during IME composition', async () => {
    const wrapper = mount(FieldEditor, {
      props: { field: field('text'), value: null },
    })
    await wrapper.get('input').setValue('中文')
    await wrapper
      .get('input')
      .trigger('keydown', { key: 'Enter', isComposing: true })
    expect(wrapper.emitted('commit')).toBeUndefined()
    await wrapper
      .get('input')
      .trigger('keydown', { key: 'Escape', isComposing: true })
    expect(wrapper.emitted('cancel')).toBeUndefined()
  })

  it('keeps non-grid text drafts local when focus leaves', async () => {
    const wrapper = mount(FieldEditor, {
      props: { field: field('text'), value: 'before' },
    })
    await wrapper.get('input').setValue('draft')
    await wrapper
      .get('input')
      .trigger('focusout', { relatedTarget: document.body })
    expect(wrapper.emitted('commit')).toBeUndefined()
    expect((wrapper.get('input').element as HTMLInputElement).value).toBe(
      'draft',
    )
  })
})

describe('field messages', () => {
  it('has a translation for every kind and reason in both locales', () => {
    const reasons: Reason[] = [
      'unknown_kind',
      'invalid_options',
      'invalid_type',
      'invalid_text',
      'too_long',
      'invalid_integer',
      'invalid_decimal',
      'invalid_number',
      'imprecise_number',
      'out_of_range',
      'too_many_decimals',
      'invalid_boolean',
      'invalid_date',
      'invalid_datetime',
      'nonexistent_local_time',
      'timezone_required',
      'unknown_choice',
      'duplicate_choice',
      'invalid_email',
      'invalid_url',
      'url_scheme',
      'read_only',
    ]
    for (const locale of ['en', 'zh-CN'] as const) {
      i18n.global.locale.value = locale
      for (const reason of reasons)
        expect(fieldErrorMessage(reason)).not.toMatch(/^field\./)
      for (const kind of KINDS)
        expect(fieldKindLabel(kind)).not.toMatch(/^field\./)
    }
  })
})
