import { beforeEach, describe, expect, it } from 'vitest'
import { config, mount } from '@vue/test-utils'
import { i18n, loadLocale } from '../../i18n'
import { bindField, type BoundField } from '../../fields/registry'
import { fieldErrorMessage, fieldKindLabel } from '../../fields/messages'
import { KINDS } from '../../fields/registry'
import type { Reason } from '../../fields/types'
import FieldRenderer from './FieldRenderer.vue'
import FieldEditor from './FieldEditor.vue'

config.global.plugins = [i18n]

beforeEach(async () => {
  await loadLocale('en')
  await loadLocale('zh-CN')
  i18n.global.locale.value = 'en'
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
    expect((input.element as HTMLInputElement).value).toBe(
      '2026-03-08 01:30:00-05:00',
    )
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

  it('toggles checkboxes immediately', async () => {
    const wrapper = mount(FieldEditor, {
      props: { field: field('checkbox'), value: false },
    })
    await wrapper.get('input').setValue(true)
    expect(wrapper.emitted('commit')).toEqual([[true]])
    await wrapper.get('input').trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('commit')).toEqual([[true]])
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
