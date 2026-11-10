import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { bindField } from '../../fields/registry'
import type { FieldEnv, WireValue } from '../../fields/types'
import { i18n, loadLocale } from '../../i18n'
import FieldEditor from './FieldEditor.vue'
import TemporalEditor from './TemporalEditor.vue'

const cleanup: Array<() => void> = []

beforeEach(async () => {
  await loadLocale('en')
  await loadLocale('zh-CN')
  i18n.global.locale.value = 'en'
})

afterEach(() => {
  for (const unmount of cleanup.splice(0)) unmount()
  vi.unstubAllEnvs()
})

function mountTemporal(
  kind: string,
  value: WireValue,
  options: unknown = {},
  env: FieldEnv = { timeZone: 'Asia/Shanghai', locale: 'en' },
) {
  const field = bindField(kind, options)
  if (!field.ok) throw new Error(field.reason)
  const wrapper = mount(TemporalEditor, {
    props: {
      field: field.value,
      value,
      env,
      label: 'Value',
      gridCommitOnLeave: true,
    },
    global: { plugins: [i18n] },
    attachTo: document.body,
  })
  cleanup.push(() => wrapper.unmount())
  return wrapper
}

function panel(wrapper: ReturnType<typeof mountTemporal>) {
  const element = Array.from(
    document.querySelectorAll<HTMLElement>('.temporal-picker-portal'),
  ).find((candidate) => wrapper.vm.contains(candidate))
  if (!element) throw new Error('Temporal picker portal was not mounted')
  return element
}

async function clickDay(element: HTMLElement, day: number) {
  const target = Array.from(
    element.querySelectorAll<HTMLElement>(
      '.n-date-panel-date:not(.n-date-panel-date--excluded)',
    ),
  ).find((item) => item.textContent?.trim() === String(day))
  if (!target) throw new Error(`Calendar day ${day} was not found`)
  target.dispatchEvent(new Event('pointerdown', { bubbles: true }))
  target.click()
  await flushPromises()
}

describe('temporal editor', () => {
  it('round-trips a calendar date without a time-zone shift', async () => {
    const wrapper = mountTemporal('date', '2026-03-01')
    await flushPromises()
    expect((wrapper.get('input').element as HTMLInputElement).value).toBe(
      '2026-03-01',
    )
    expect(panel(wrapper).querySelector('.n-date-panel')).not.toBeNull()
    await clickDay(panel(wrapper), 1)
    expect(wrapper.emitted('commit')).toEqual([['2026-03-01']])
  })

  it.each([
    ['slash', '2026/03/01'],
    ['us', '03/01/2026'],
    ['eu', '01/03/2026'],
  ])('displays the configured %s date format', async (dateFormat, expected) => {
    const wrapper = mountTemporal('date', '2026-03-01', { dateFormat })
    await flushPromises()
    expect((wrapper.get('input').element as HTMLInputElement).value).toBe(
      expected,
    )
    await clickDay(panel(wrapper), 15)
    expect(wrapper.emitted('commit')).toEqual([['2026-03-15']])
  })

  it('confirms a fixed-zone datetime using its wall time and exact precision', async () => {
    const wrapper = mountTemporal('datetime', '2026-03-01T14:15:12.123456Z', {
      timezone: 'fixed',
      fixedTz: 'America/New_York',
    })
    await flushPromises()
    expect((wrapper.get('input').element as HTMLInputElement).value).toBe(
      '2026-03-01 09:15',
    )
    await clickDay(panel(wrapper), 15)
    expect(wrapper.emitted('commit')).toBeUndefined()
    const confirm = panel(wrapper).querySelector<HTMLButtonElement>(
      '[data-temporal-confirm]',
    )
    if (!confirm) throw new Error('Datetime confirm button was not found')
    confirm.click()
    await flushPromises()
    expect(wrapper.emitted('commit')).toEqual([['2026-03-15T13:15:12.123456Z']])
  })

  it('confirms a time scroller selection without discarding seconds or fractions', async () => {
    const wrapper = mountTemporal('time', '09:15:12.123456')
    await flushPromises()
    const popup = panel(wrapper)
    const columns = popup.querySelectorAll<HTMLElement>('.time-scroller-column')
    const minute = Array.from(
      columns[1]?.querySelectorAll<HTMLElement>('.time-scroller-option') ?? [],
    ).find((item) => item.textContent?.trim() === '35')
    if (!minute) throw new Error('Time picker minute was not found')
    minute.click()
    await flushPromises()
    expect(wrapper.get<HTMLInputElement>('input').element.value).toBe(
      '09:35:12.123456',
    )
    expect(wrapper.emitted('commit')).toBeUndefined()
    const confirm = popup.querySelector<HTMLButtonElement>(
      '.time-scroller-confirm',
    )
    if (!confirm) throw new Error('Time picker confirm button was not found')
    confirm.click()
    await flushPromises()
    expect(wrapper.emitted('commit')).toEqual([['09:35:12.123456']])
  })

  it('supports 12-hour picks and arrow-key navigation within a column', async () => {
    const wrapper = mountTemporal('time', '09:15:12', { format: '12h' })
    await flushPromises()
    const popup = panel(wrapper)
    const hour = Array.from(
      popup.querySelectorAll<HTMLButtonElement>(
        '.time-scroller-column:first-child .time-scroller-option',
      ),
    ).find((item) => item.textContent?.trim() === '09')
    if (!hour) throw new Error('Time scroller hour was not found')
    const pressDown = () =>
      hour.dispatchEvent(
        new KeyboardEvent('keydown', {
          key: 'ArrowDown',
          bubbles: true,
          cancelable: true,
        }),
      )
    pressDown()
    await flushPromises()
    expect(hour.getAttribute('aria-selected')).toBe('false')
    let selectedHour = Array.from(
      popup.querySelectorAll<HTMLButtonElement>(
        '.time-scroller-column:first-child .time-scroller-option',
      ),
    ).find((item) => item.textContent?.trim() === '10')
    expect(selectedHour?.getAttribute('aria-selected')).toBe('true')
    pressDown()
    await flushPromises()
    selectedHour = Array.from(
      popup.querySelectorAll<HTMLButtonElement>(
        '.time-scroller-column:first-child .time-scroller-option',
      ),
    ).find((item) => item.textContent?.trim() === '11')
    expect(selectedHour?.getAttribute('aria-selected')).toBe('true')

    const pm = Array.from(
      popup.querySelectorAll<HTMLButtonElement>(
        '.time-scroller-column:nth-child(3) .time-scroller-option',
      ),
    ).find((item) => item.textContent?.trim() === 'PM')
    if (!pm) throw new Error('Time scroller PM option was not found')
    pm.click()
    await flushPromises()
    expect(wrapper.get<HTMLInputElement>('input').element.value).toBe(
      '11:15:12 PM',
    )
  })

  it.each(['datetime', 'time'] as const)(
    'commits an unconfirmed %s panel draft through the editor API',
    async (kind) => {
      const wrapper = mountTemporal(
        kind,
        kind === 'datetime' ? '2026-03-01T14:15:12.123456Z' : '09:15:12.123456',
        kind === 'datetime'
          ? { timezone: 'fixed', fixedTz: 'America/New_York' }
          : {},
      )
      await flushPromises()
      expect(wrapper.vm.hasPendingDraft()).toBe(false)
      const popup = panel(wrapper)
      if (kind === 'datetime') await clickDay(popup, 15)
      else {
        const minute = Array.from(
          popup.querySelectorAll<HTMLElement>(
            '.time-scroller-column:nth-child(2) .time-scroller-option',
          ),
        ).find((item) => item.textContent?.trim() === '35')
        if (!minute) throw new Error('Time picker minute was not found')
        minute.click()
        await flushPromises()
      }
      expect(wrapper.vm.hasPendingDraft()).toBe(true)
      expect(wrapper.vm.commitDraft()).toBe(true)
      expect(wrapper.emitted('commit')).toEqual([
        [
          kind === 'datetime'
            ? '2026-03-15T13:15:12.123456Z'
            : '09:35:12.123456',
        ],
      ])
      expect(wrapper.vm.hasPendingDraft()).toBe(false)
    },
  )

  it('accepts typed HH:mm and emits the existing canonical time wire shape', async () => {
    const wrapper = mountTemporal('time', null)
    await flushPromises()
    await wrapper.get('input').setValue('09:35')
    await wrapper.get('input').trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('commit')).toEqual([['09:35:00']])
  })

  it('keeps human-paced time typing and the caret stable between keys', async () => {
    const wrapper = mountTemporal('time', null)
    await flushPromises()
    const input = wrapper.get<HTMLInputElement>('input').element
    let typed = ''

    for (const character of '13:45') {
      const start = input.selectionStart ?? input.value.length
      const end = input.selectionEnd ?? start
      typed += character
      input.setRangeText(character, start, end, 'end')
      input.dispatchEvent(
        new InputEvent('input', {
          bubbles: true,
          inputType: 'insertText',
          data: character,
        }),
      )
      await new Promise((resolve) => setTimeout(resolve, 300))
      await flushPromises()
      expect(input.value).toBe(typed)
      expect(input.selectionStart).toBe(typed.length)
    }

    await wrapper.get('input').trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('commit')).toEqual([['13:45:00']])
  })

  it('does not carry an escaped time draft into a new editor instance', async () => {
    const first = mountTemporal('time', null)
    await flushPromises()
    await first.get('input').setValue('13:45')
    await first.get('input').trigger('keydown', { key: 'Escape' })
    first.unmount()

    const second = mountTemporal('time', null)
    await flushPromises()
    expect(second.get<HTMLInputElement>('input').element.value).toBe('')
  })

  it('shows a typed format hint and opens a form panel on request', async () => {
    const field = bindField('date', {})
    if (!field.ok) throw new Error(field.reason)
    const wrapper = mount(TemporalEditor, {
      props: {
        field: field.value,
        value: null,
        env: { timeZone: 'UTC', locale: 'en' },
        label: 'Date',
        autoFocus: false,
      },
      global: { plugins: [i18n] },
      attachTo: document.body,
    })
    cleanup.push(() => wrapper.unmount())
    await flushPromises()
    expect(wrapper.get<HTMLInputElement>('input').element.placeholder).toBe(
      'YYYY-MM-DD',
    )
    expect(panel(wrapper).querySelector('.n-date-panel')).toBeNull()

    await wrapper.get('input').trigger('keydown', {
      key: 'ArrowDown',
      altKey: true,
    })
    await flushPromises()
    expect(panel(wrapper).querySelector('.n-date-panel')).not.toBeNull()
    expect(
      wrapper.get('.temporal-editor-toggle').attributes('aria-expanded'),
    ).toBe('true')
  })

  it('resets from a changed value while the input is unfocused', async () => {
    const field = bindField('date', {})
    if (!field.ok) throw new Error(field.reason)
    const wrapper = mount(TemporalEditor, {
      props: {
        field: field.value,
        value: '2026-03-01',
        env: { timeZone: 'UTC', locale: 'en' },
        label: 'Date',
        autoFocus: false,
      },
      global: { plugins: [i18n] },
      attachTo: document.body,
    })
    cleanup.push(() => wrapper.unmount())
    await flushPromises()
    await wrapper.setProps({ value: '2026-03-15' })
    await flushPromises()
    expect(wrapper.get<HTMLInputElement>('input').element.value).toBe(
      '2026-03-15',
    )
    expect(wrapper.vm.currentDraft()).toBe('2026-03-15')
  })

  it('does not recommit an Enter edit when the form input later blurs', async () => {
    const field = bindField('date', {})
    if (!field.ok) throw new Error(field.reason)
    const wrapper = mount(TemporalEditor, {
      props: {
        field: field.value,
        value: null,
        env: { timeZone: 'UTC', locale: 'en' },
        label: 'Date',
        autoFocus: false,
      },
      global: { plugins: [i18n] },
      attachTo: document.body,
    })
    cleanup.push(() => wrapper.unmount())
    await flushPromises()
    const input = wrapper.get<HTMLInputElement>('input').element
    input.focus()
    await wrapper.get('input').setValue('2026-11-15')
    await wrapper.get('input').trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('commit')).toEqual([['2026-11-15']])

    input.blur()
    await flushPromises()
    expect(wrapper.emitted('commit')).toHaveLength(1)
  })

  it('commits compact typed dates through the existing paste parser', async () => {
    const wrapper = mountTemporal('date', null)
    await flushPromises()
    await wrapper.get('input').setValue('20261115')
    await wrapper.get('input').trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('commit')).toEqual([['2026-11-15']])
  })

  it('retains invalid typed time text after Enter', async () => {
    const wrapper = mountTemporal('time', null)
    await flushPromises()
    await wrapper.get('input').setValue('25:00')
    await wrapper.get('input').trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('commit')).toBeUndefined()
    expect(wrapper.emitted('invalidDraft')?.[0]?.[0]).toBe('25:00')
    expect(wrapper.get<HTMLInputElement>('input').element.value).toBe('25:00')
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
  })

  it('keeps a live typed date while its panel highlight is synchronized', async () => {
    const wrapper = mountTemporal('date', null)
    await wrapper.setProps({ live: true, autoFocus: false })
    await flushPromises()
    await wrapper.get('input').setValue('2026-10-27')
    expect(wrapper.emitted('preview')).toEqual([['2026-10-27']])
    expect(wrapper.emitted('commit')).toBeUndefined()
    expect(wrapper.vm.currentDraft()).toBe('2026-10-27')
    expect(wrapper.vm.commitDraft()).toBe(true)
    expect(wrapper.emitted('commit')).toEqual([['2026-10-27']])
  })

  it('keeps an unconfirmed datetime draft visible after leaving the picker', async () => {
    const wrapper = mountTemporal('datetime', null)
    await wrapper.setProps({ live: true, autoFocus: false })
    await flushPromises()
    const input = wrapper.get<HTMLInputElement>('input')
    input.element.focus()
    const text = '2026-10-27T15:00:00.123456+08:00'
    await input.setValue(text)
    input.element.blur()
    await flushPromises()
    expect(input.element.value).toBe(text)
    expect(wrapper.vm.currentDraft()).toBe(text)
    expect(wrapper.emitted('commit')).toBeUndefined()
    expect(wrapper.vm.commitDraft()).toBe(true)
    expect(wrapper.emitted('commit')).toEqual([['2026-10-27T07:00:00.123456Z']])
  })

  it('closes the picker and lets a valid typed Enter reach grid navigation', async () => {
    const wrapper = mountTemporal('date', null)
    const onKeydown = vi.fn()
    document.body.addEventListener('keydown', onKeydown)
    try {
      await flushPromises()
      await wrapper.get('input').setValue('2026-10-27')
      await wrapper.get('input').trigger('keydown', { key: 'Enter' })
      expect(wrapper.emitted('commit')).toEqual([['2026-10-27']])
      expect(panel(wrapper).querySelector('.n-date-panel')).toBeNull()
      expect(onKeydown).toHaveBeenCalledOnce()
    } finally {
      document.body.removeEventListener('keydown', onKeydown)
    }
  })

  it('keeps invalid typed Enter inside the picker', async () => {
    const wrapper = mountTemporal('date', null)
    const onKeydown = vi.fn()
    document.body.addEventListener('keydown', onKeydown)
    try {
      await flushPromises()
      await wrapper.get('input').setValue('not a date')
      await wrapper.get('input').trigger('keydown', { key: 'Enter' })
      expect(wrapper.emitted('commit')).toBeUndefined()
      expect(wrapper.emitted('invalidDraft')).toHaveLength(1)
      expect(onKeydown).not.toHaveBeenCalled()
    } finally {
      document.body.removeEventListener('keydown', onKeydown)
    }
  })

  it('uses the configured 12-hour time format', async () => {
    const wrapper = mountTemporal('time', '23:05:07', { format: '12h' })
    await flushPromises()
    expect((wrapper.get('input').element as HTMLInputElement).value).toBe(
      '11:05:07 PM',
    )
    await wrapper.get('input').setValue('12:35 AM')
    await wrapper.get('input').trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('commit')).toEqual([['00:35:00']])
  })

  it('clears an optional date immediately', async () => {
    const wrapper = mountTemporal('date', '2026-03-01')
    await flushPromises()
    const clear = panel(wrapper).querySelector<HTMLButtonElement>(
      '.n-date-panel-actions button',
    )
    if (!clear) throw new Error('Date picker clear button was not found')
    clear.click()
    await flushPromises()
    expect(wrapper.emitted('commit')).toEqual([[null]])
  })

  it('preserves a wall time that falls in the browser time zone DST gap', async () => {
    vi.stubEnv('TZ', 'America/Los_Angeles')
    const wrapper = mountTemporal(
      'datetime',
      '2026-03-07T18:30:45.123456Z',
      {},
      { timeZone: 'Asia/Shanghai', locale: 'en' },
    )
    await flushPromises()
    expect((wrapper.get('input').element as HTMLInputElement).value).toBe(
      '2026-03-08 02:30',
    )
    expect(wrapper.get('[role="status"]').text()).toBe(
      i18n.global.t('gridPolish.gp08.browserGap'),
    )
    await clickDay(panel(wrapper), 15)
    const confirm = panel(wrapper).querySelector<HTMLButtonElement>(
      '[data-temporal-confirm]',
    )
    if (!confirm) throw new Error('Datetime confirm button was not found')
    confirm.click()
    await flushPromises()
    expect(wrapper.emitted('commit')).toEqual([['2026-03-14T18:30:45.123456Z']])
  })

  it('preserves the field-zone wall time when selecting a browser DST-gap date', async () => {
    vi.stubEnv('TZ', 'America/Los_Angeles')
    const wrapper = mountTemporal(
      'datetime',
      '2026-03-06T18:30:45.123456Z',
      {},
      { timeZone: 'Asia/Shanghai', locale: 'en' },
    )
    await flushPromises()
    await clickDay(panel(wrapper), 8)
    const confirm = panel(wrapper).querySelector<HTMLButtonElement>(
      '[data-temporal-confirm]',
    )
    if (!confirm) throw new Error('Datetime confirm button was not found')
    confirm.click()
    await flushPromises()
    expect(wrapper.emitted('commit')).toEqual([['2026-03-07T18:30:45.123456Z']])
    expect((wrapper.get('input').element as HTMLInputElement).value).toBe(
      '2026-03-08 02:30',
    )
    expect(wrapper.find('[role="status"]').exists()).toBe(true)
  })

  it('keeps a changed panel time when the chosen date is in the browser DST gap', async () => {
    vi.stubEnv('TZ', 'America/Los_Angeles')
    const wrapper = mountTemporal(
      'datetime',
      '2026-03-06T17:30:45.123456Z',
      {},
      { timeZone: 'Asia/Shanghai', locale: 'en' },
    )
    await flushPromises()
    const popup = panel(wrapper)
    const timeInput = popup.querySelector<HTMLInputElement>(
      '.n-date-panel-header .n-time-picker input',
    )
    if (!timeInput) throw new Error('Datetime panel time input was not found')
    timeInput.click()
    await flushPromises()
    const hour = Array.from(
      popup.querySelectorAll<HTMLElement>(
        '.n-time-picker-col:first-child .n-time-picker-col__item',
      ),
    ).find((item) => item.textContent?.trim() === '02')
    if (!hour) throw new Error('Datetime panel hour was not found')
    hour.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    hour.click()
    await flushPromises()
    await clickDay(popup, 8)
    expect(timeInput.value).toBe('02:30')
    expect(wrapper.find('[role="status"]').exists()).toBe(true)
    const confirm = popup.querySelector<HTMLButtonElement>(
      '[data-temporal-confirm]',
    )
    if (!confirm) throw new Error('Datetime confirm button was not found')
    confirm.click()
    await flushPromises()
    expect(wrapper.emitted('commit')).toEqual([['2026-03-07T18:30:45.123456Z']])
  })

  it('accepts a field-zone time in the browser DST gap after choosing the date', async () => {
    vi.stubEnv('TZ', 'America/Los_Angeles')
    const wrapper = mountTemporal(
      'datetime',
      '2026-03-06T17:30:45.123456Z',
      {},
      { timeZone: 'Asia/Shanghai', locale: 'en' },
    )
    await flushPromises()
    const popup = panel(wrapper)
    await clickDay(popup, 8)
    const timeInput = popup.querySelector<HTMLInputElement>(
      '.n-date-panel-header .n-time-picker input',
    )
    if (!timeInput) throw new Error('Datetime panel time input was not found')
    timeInput.click()
    await flushPromises()
    const hour = Array.from(
      popup.querySelectorAll<HTMLElement>(
        '.n-time-picker-col:first-child .n-time-picker-col__item',
      ),
    ).find((item) => item.textContent?.trim() === '02')
    if (!hour) throw new Error('Datetime panel hour was not found')
    hour.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    hour.click()
    await flushPromises()
    expect(timeInput.value).toBe('02:30')
    expect(wrapper.find('[role="status"]').exists()).toBe(true)
    const confirm = popup.querySelector<HTMLButtonElement>(
      '[data-temporal-confirm]',
    )
    if (!confirm) throw new Error('Datetime confirm button was not found')
    confirm.click()
    await flushPromises()
    expect(wrapper.emitted('commit')).toEqual([['2026-03-07T18:30:45.123456Z']])
  })

  it('uses the configured date and 12-hour formats in the datetime panel', async () => {
    vi.stubEnv('TZ', 'UTC')
    const wrapper = mountTemporal(
      'datetime',
      '2026-03-01T03:15:12Z',
      { dateFormat: 'us', timeFormat: '12h' },
      { timeZone: 'Asia/Shanghai', locale: 'en' },
    )
    await flushPromises()
    const popup = panel(wrapper)
    const dateInput = popup.querySelector<HTMLInputElement>(
      '.n-date-panel-date-input input',
    )
    const timeInput = popup.querySelector<HTMLInputElement>(
      '.n-date-panel-header .n-time-picker input',
    )
    if (!dateInput || !timeInput)
      throw new Error('Datetime panel inputs were not found')
    expect(dateInput.value).toBe('03/01/2026')
    timeInput.click()
    await flushPromises()
    const columns = popup.querySelectorAll<HTMLElement>('.n-time-picker-col')
    const pm = Array.from(
      columns[2]?.querySelectorAll<HTMLElement>('.n-time-picker-col__item') ??
        [],
    ).find((item) => item.textContent?.trim() === 'PM')
    if (!pm) throw new Error('Datetime panel PM selection was not found')
    pm.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    pm.click()
    await flushPromises()
    expect(timeInput.value).toBe('11:15 PM')
    const confirm = popup.querySelector<HTMLButtonElement>(
      '[data-temporal-confirm]',
    )
    if (!confirm) throw new Error('Datetime confirm button was not found')
    confirm.click()
    await flushPromises()
    expect(wrapper.emitted('commit')).toEqual([['2026-03-01T15:15:12Z']])
  })

  it('retains invalid datetime panel input when committing on leave', async () => {
    const wrapper = mountTemporal('datetime', '2026-03-01T03:15:12Z')
    await flushPromises()
    const timeInput = panel(wrapper).querySelector<HTMLInputElement>(
      '.n-date-panel-header .n-time-picker input',
    )
    if (!timeInput) throw new Error('Datetime panel time input was not found')
    timeInput.value = 'bad'
    timeInput.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    expect(wrapper.vm.commitDraft()).toBe(false)
    expect(wrapper.emitted('commit')).toBeUndefined()
    expect(wrapper.emitted('invalidDraft')?.[0]?.[0]).toBe('2026-03-01 bad')
  })

  it('reports an invalid picker confirmation to live forms and retains its draft', async () => {
    vi.stubEnv('TZ', 'UTC')
    const field = bindField('datetime', {
      timezone: 'fixed',
      fixedTz: 'America/New_York',
    })
    if (!field.ok) throw new Error(field.reason)
    const wrapper = mount(FieldEditor, {
      props: {
        field: field.value,
        value: '2026-03-07T07:30:45Z',
        env: { timeZone: 'UTC', locale: 'en' },
        live: true,
        label: 'Date and time',
      },
      global: { plugins: [i18n] },
      attachTo: document.body,
    })
    cleanup.push(() => wrapper.unmount())
    await flushPromises()
    const popup = document.querySelector<HTMLElement>('.temporal-picker-portal')
    if (!popup) throw new Error('Temporal picker portal was not mounted')
    await wrapper.get('.temporal-editor-toggle').trigger('click')
    await flushPromises()
    await clickDay(popup, 8)
    const confirm = popup.querySelector<HTMLButtonElement>(
      '[data-temporal-confirm]',
    )
    if (!confirm) throw new Error('Datetime confirm button was not found')
    confirm.click()
    await flushPromises()
    expect(wrapper.emitted('invalidDraft')?.[0]?.slice(0, 2)).toEqual([
      '2026-03-08 02:30:45',
      'nonexistent_local_time',
    ])
    expect(wrapper.get('[role="alert"]').text()).toContain('daylight-saving')
    expect(wrapper.emitted('invalidValue')).toHaveLength(1)
    expect(wrapper.emitted('commit')).toBeUndefined()
    expect((wrapper.get('input').element as HTMLInputElement).value).toBe(
      '2026-03-08 02:30:45',
    )
    expect(wrapper.vm.currentDraft()).toBe('2026-03-08 02:30:45')
    expect(wrapper.vm.commitDraft()).toBe(false)
  })

  it('preserves the later instant in a daylight-saving overlap when unchanged', async () => {
    const wire = '2026-11-01T06:30:45.123456Z'
    const wrapper = mountTemporal('datetime', wire, {
      timezone: 'fixed',
      fixedTz: 'America/New_York',
    })
    await flushPromises()
    await wrapper.get('input').trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('commit')).toEqual([[wire, true]])
  })

  it('reports invalid live drafts and prevents clearing required values', async () => {
    const field = bindField('time', {})
    if (!field.ok) throw new Error(field.reason)
    const wrapper = mount(FieldEditor, {
      props: {
        field: field.value,
        value: null,
        env: { timeZone: 'UTC', locale: 'en' },
        required: true,
        live: true,
        autoFocus: false,
        label: 'Time',
      },
      global: { plugins: [i18n] },
      attachTo: document.body,
    })
    cleanup.push(() => wrapper.unmount())
    await flushPromises()
    const input = wrapper.get('input')
    expect(input.attributes('required')).toBeDefined()
    await input.setValue('bad')
    expect(wrapper.emitted('invalidValue')).toHaveLength(1)
    await input.setValue('09:35')
    expect(wrapper.emitted('preview')).toEqual([['09:35:00']])
    await input.setValue('')
    await input.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('commit')).toBeUndefined()
    expect(wrapper.get('[role="alert"]').text()).toBe(
      i18n.global.t('errors.ERR_REQUIRED_FIELD'),
    )
  })

  it('restores a live form value when Escape cancels a typed edit', async () => {
    const field = bindField('time', {})
    if (!field.ok) throw new Error(field.reason)
    const wrapper = mount(FieldEditor, {
      props: {
        field: field.value,
        value: '09:15:00',
        env: { timeZone: 'UTC', locale: 'en' },
        live: true,
        autoFocus: false,
        label: 'Time',
      },
      global: { plugins: [i18n] },
      attachTo: document.body,
    })
    cleanup.push(() => wrapper.unmount())
    await flushPromises()
    const input = wrapper.get('input')
    await input.setValue('10:30')
    expect(wrapper.emitted('preview')).toEqual([['10:30:00']])
    await input.trigger('keydown', { key: 'Escape' })
    expect(wrapper.emitted('preview')).toEqual([['10:30:00'], ['09:15:00']])
    expect((input.element as HTMLInputElement).value).toBe('09:15:00')
  })
})
