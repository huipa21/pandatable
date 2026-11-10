<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NButton, NDatePicker } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { timeKind } from '../../fields/batch2'
import { fieldErrorMessage } from '../../fields/messages'
import type { BoundField } from '../../fields/registry'
import {
  dateKind,
  type DateFormat,
  type DateTimeOptions,
} from '../../fields/temporal'
import {
  isCompleteTyped,
  normalizeTyped,
  typedPlaceholder,
  type TypedKind,
} from '../../fields/temporalTyping'
import TimeScroller from './TimeScroller.vue'
import {
  fail,
  type FieldEnv,
  type Reason,
  type Result,
  type WireValue,
} from '../../fields/types'

const props = withDefaults(
  defineProps<{
    field: BoundField
    value: WireValue
    env: FieldEnv
    label: string
    required?: boolean
    autoFocus?: boolean
    live?: boolean
    gridCommitOnLeave?: boolean
    initialDraft?: string
    defaultPlaceholder?: string
  }>(),
  {
    required: false,
    autoFocus: true,
    live: false,
    gridCommitOnLeave: false,
    initialDraft: undefined,
    defaultPlaceholder: undefined,
  },
)
const emit = defineEmits<{
  commit: [value: WireValue, retainFocus?: boolean]
  preview: [value: WireValue]
  invalidValue: []
  invalidDraft: [text: string, reason: Reason, message?: string]
  compositionFinished: []
  cancel: []
  focusout: [event: FocusEvent]
  draftChanged: []
}>()
const { t } = useI18n()
const host = ref<HTMLElement>()
const portal = ref<HTMLElement>()
const textInput = ref<HTMLInputElement>()
const portalDestination = ref<HTMLElement | string>('body')
const portalStyle = ref<Record<string, string>>({})
const show = ref(false)
const composing = ref(false)
const reason = ref<Reason | null>(null)
const requiredError = ref(false)
const options = computed(
  () =>
    props.field.options as Partial<DateTimeOptions> & {
      format?: '12h' | '24h'
    },
)
const dateFormats: Record<DateFormat, string> = {
  iso: 'yyyy-MM-dd',
  slash: 'yyyy/MM/dd',
  us: 'MM/dd/yyyy',
  eu: 'dd/MM/yyyy',
}
const dateFormat = computed(
  () => dateFormats[options.value.dateFormat ?? 'iso'],
)
const twelveHours = computed(() =>
  props.field.kind === 'time'
    ? options.value.format === '12h'
    : options.value.timeFormat === '12h',
)
const pickerFormat = computed(() => {
  if (props.field.kind === 'date') return dateFormat.value
  return `${dateFormat.value} ${twelveHours.value ? 'h:mm a' : 'HH:mm'}`
})
const pickerActions = computed<Array<'clear' | 'confirm'>>(() => {
  const actions: Array<'clear' | 'confirm'> = props.required ? [] : ['clear']
  if (props.field.kind !== 'date') actions.push('confirm')
  return actions
})
const error = computed(() =>
  requiredError.value
    ? t('errors.ERR_REQUIRED_FIELD')
    : reason.value
      ? fieldErrorMessage(reason.value)
      : '',
)
const typedKind = computed<TypedKind | null>(() =>
  props.field.kind === 'date' ||
  props.field.kind === 'datetime' ||
  props.field.kind === 'time'
    ? props.field.kind
    : null,
)
const placeholder = computed(() =>
  typedKind.value
    ? typedPlaceholder(typedKind.value, {
        dateFormat: options.value.dateFormat ?? 'iso',
        twelveHours: twelveHours.value,
        today: todayInFieldZone(),
      })
    : '',
)
const togglePanelLabel = computed(() =>
  props.field.kind === 'time'
    ? t('cellEditors.ces02.openTimePanel')
    : t('cellEditors.ces02.openCalendar'),
)

let baselineValue = props.value
let baselineCopy = props.field.formatCopy(baselineValue, props.env)
let baselineWall: string | null = null
let fraction = ''
const pickerValue = ref<string | null>(null)
const draft = ref('')
const dirty = ref(false)
const panelPending = ref(false)
const browserGap = ref(false)
let panelInteracted = false
let panelTimeChanged = false
let panelWallTime: string | null = null
let panelFraction: string | undefined
let panelTimeError: Reason | null = null
let pointerInPanel = false
let clearing = false
let skipPickerUpdate = false

function localWallExists(text: string) {
  const match = /^(\d{4})-(\d{2})-(\d{2}) (\d{2}):(\d{2}):(\d{2})$/.exec(text)
  if (!match) return false
  const [, y, m, d, h, min, s] = match.map(Number)
  const date = new Date(0)
  date.setFullYear(y!, m! - 1, d!)
  date.setHours(h!, min!, s!, 0)
  return (
    date.getFullYear() === y &&
    date.getMonth() === m! - 1 &&
    date.getDate() === d &&
    date.getHours() === h &&
    date.getMinutes() === min &&
    date.getSeconds() === s
  )
}

function setBaseline(value: WireValue) {
  baselineValue = value
  baselineCopy = props.field.formatCopy(value, props.env)
  fraction = ''
  browserGap.value = false
  if (props.field.kind === 'date') {
    baselineWall = typeof value === 'string' ? value : null
  } else if (props.field.kind === 'datetime') {
    const match =
      /^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})(\.\d{1,6})?[+-]\d{2}:\d{2}(?::\d{2})?$/.exec(
        baselineCopy,
      )
    baselineWall = match?.[1] ?? null
    fraction = match?.[2] ?? ''
    browserGap.value = baselineWall !== null && !localWallExists(baselineWall)
  } else {
    const match = /^(\d{2}:\d{2}:\d{2})(\.\d{1,6})?$/.exec(baselineCopy)
    baselineWall = match?.[1] ?? null
    fraction = match?.[2] ?? ''
  }
  const fallback = baselineWall ? `${baselineWall.slice(0, 10)} 12:00:00` : null
  pickerValue.value = browserGap.value
    ? fallback && localWallExists(fallback)
      ? fallback
      : null
    : baselineWall
}

setBaseline(props.value)
function resetDraft(value: WireValue, initialDraft?: string) {
  const formatted = props.field.formatDisplay(value, props.env)
  draft.value = initialDraft ?? formatted
  dirty.value = initialDraft !== undefined && initialDraft !== formatted
}

resetDraft(props.value, props.initialDraft)

function positionPanel() {
  const rect = host.value?.getBoundingClientRect()
  if (!rect) return
  const panelHeight = props.field.kind === 'time' ? 270 : 360
  const panelWidth = props.field.kind === 'time' ? 260 : 320
  const below = rect.bottom + 4
  const top =
    below + panelHeight <= window.innerHeight
      ? below
      : Math.max(8, rect.top - panelHeight - 4)
  portalStyle.value = {
    position: 'fixed',
    top: `${top}px`,
    left: `${Math.max(8, Math.min(rect.left, window.innerWidth - panelWidth - 8))}px`,
    zIndex: '2000',
  }
}

function inputElement() {
  return textInput.value ?? null
}

watch(
  [() => props.value, () => props.initialDraft],
  ([value, initialDraft]) => {
    if (document.activeElement === inputElement()) return
    setBaseline(value)
    resetDraft(value, initialDraft)
    reason.value = null
    requiredError.value = false
    panelPending.value = false
    panelInteracted = false
    panelTimeChanged = false
    panelWallTime = null
    panelFraction = undefined
    panelTimeError = null
  },
  { deep: true },
)
watch(show, (visible) => {
  if (visible) void nextTick(positionPanel)
})

onMounted(async () => {
  const fullscreen = document.fullscreenElement
  const dialog = host.value?.closest('dialog')
  portalDestination.value =
    dialog?.querySelector<HTMLElement>(
      ':scope > [data-temporal-portal-root]',
    ) ??
    dialog ??
    (fullscreen instanceof HTMLElement ? fullscreen : document.body)
  await nextTick()
  const input = inputElement()
  if (props.autoFocus && input && !input.matches(':disabled')) {
    input.focus({ preventScroll: true })
    show.value = props.gridCommitOnLeave
  }
  window.addEventListener('resize', positionPanel)
  window.addEventListener('scroll', positionPanel, true)
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', positionPanel)
  window.removeEventListener('scroll', positionPanel, true)
})

function normalizeDayPeriod(text: string) {
  const locale = props.env.locale === 'zh-CN' ? 'zh-CN' : 'en'
  const formatter = new Intl.DateTimeFormat(locale, {
    hour: 'numeric',
    hour12: true,
    timeZone: 'UTC',
  })
  for (const [hour, replacement] of [
    [1, 'AM'],
    [13, 'PM'],
  ] as const) {
    const period = formatter
      .formatToParts(new Date(Date.UTC(2000, 0, 1, hour)))
      .find((part) => part.type === 'dayPeriod')?.value
    if (period) text = text.replace(period, replacement)
  }
  return text
}

function todayInFieldZone() {
  const parts = new Intl.DateTimeFormat('en-CA', {
    timeZone: props.env.timeZone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).formatToParts(new Date())
  return {
    year: Number(parts.find((part) => part.type === 'year')?.value),
    month: Number(parts.find((part) => part.type === 'month')?.value),
    day: Number(parts.find((part) => part.type === 'day')?.value),
  }
}

function typedContext() {
  return {
    dateFormat: options.value.dateFormat ?? 'iso',
    twelveHours: twelveHours.value,
    today: todayInFieldZone(),
  }
}

function parseText(text: string): Result<WireValue> {
  const kind = typedKind.value
  let normalized = kind
    ? (normalizeTyped(kind, text, typedContext()) ??
      normalizeDayPeriod(text.trim()))
    : normalizeDayPeriod(text.trim())
  if (kind === 'time' && /^\d{2}:\d{2}$/.test(normalized)) normalized += ':00'
  return props.field.parsePaste(normalized, props.env)
}

function syncPickerHighlight(text: string) {
  const kind = typedKind.value
  if (!kind || !isCompleteTyped(kind, text, typedContext())) return
  const normalized = normalizeTyped(kind, text, typedContext())
  if (normalized === null) return
  const result = props.field.parsePaste(normalized, props.env)
  if (!result.ok || result.value === null) return
  if (kind === 'date') {
    pickerValue.value = String(result.value)
    return
  }
  const copy = props.field.formatCopy(result.value, props.env)
  if (kind === 'time') {
    const match = /^(\d{2}:\d{2}:\d{2})(\.\d{1,6})?$/.exec(copy)
    if (!match) return
    pickerValue.value = match[1]!
    panelFraction = match[2] ?? ''
    return
  }
  const match = /^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})(\.\d{1,6})?/.exec(copy)
  if (!match) return
  const wall = match[1]!
  panelWallTime = wall.slice(11)
  panelFraction = match[2] ?? ''
  browserGap.value = !localWallExists(wall)
  pickerValue.value = browserGap.value ? `${wall.slice(0, 10)} 12:00:00` : wall
}

function pickerText(value: string) {
  if (props.field.kind !== 'datetime') return value
  const time =
    panelWallTime ??
    (browserGap.value && baselineWall && !panelTimeChanged
      ? baselineWall.slice(11)
      : null)
  return time === null ? value : `${value.slice(0, 10)} ${time}`
}

function fromPicker(value: string | null): Result<WireValue> {
  if (value === null) return props.field.normalize(null)
  if (props.field.kind === 'date') return props.field.normalize(value)
  if (props.field.kind === 'datetime' && panelTimeError)
    return fail(panelTimeError)
  value = pickerText(value)
  const selectedFraction = panelFraction ?? fraction
  if (value === baselineWall && selectedFraction === fraction)
    return props.field.normalize(baselineValue)
  if (props.field.kind === 'time')
    return props.field.normalize(value + selectedFraction)
  return props.field.parsePaste(value + selectedFraction, props.env)
}

function validate(
  result: Result<WireValue>,
  text: string,
  committing: boolean,
) {
  if (!result.ok) {
    reason.value = result.reason
    requiredError.value = false
    if (committing) emit('invalidDraft', text, result.reason)
    if (!committing || props.live) emit('invalidValue')
    return false
  }
  if (props.required && result.value === null) {
    reason.value = null
    requiredError.value = true
    if (committing)
      emit(
        'invalidDraft',
        text,
        props.field.kind === 'time' ? 'invalid_time' : 'invalid_date',
        t('errors.ERR_REQUIRED_FIELD'),
      )
    if (!committing || props.live) emit('invalidValue')
    return false
  }
  reason.value = null
  requiredError.value = false
  return true
}

function currentDraft() {
  return dirty.value ? draft.value : baselineCopy
}

function commitResult(
  result: Result<WireValue>,
  text: string,
  retainFocus = false,
  preserveFocusedText = false,
) {
  if (!validate(result, text, true) || !result.ok) return false
  setBaseline(result.value)
  dirty.value = false
  panelPending.value = false
  draft.value =
    preserveFocusedText && document.activeElement === inputElement()
      ? text
      : props.field.formatDisplay(result.value, props.env)
  panelInteracted = false
  panelTimeChanged = false
  panelWallTime = null
  panelFraction = undefined
  panelTimeError = null
  emit('draftChanged')
  if (retainFocus) emit('commit', result.value, true)
  else emit('commit', result.value)
  return true
}

function commitDraft(retainFocus = false) {
  if (panelPending.value && props.field.kind === 'datetime') {
    const date = panelDateText()
    if (date && panelWallTime && !panelTimeError)
      recordPanelDraft(panelWallTime + (panelFraction ?? fraction))
  }
  const text = currentDraft()
  const unchangedWallTime =
    props.field.kind === 'datetime' &&
    baselineWall !== null &&
    text === baselineWall + fraction
  return commitResult(
    dirty.value && !unchangedWallTime
      ? parseText(text)
      : props.field.normalize(baselineValue),
    text,
    retainFocus,
    true,
  )
}

function previewDraft() {
  const result = parseText(draft.value)
  if (validate(result, draft.value, false) && result.ok)
    emit('preview', result.value)
}

function inputChanged(event: Event) {
  if (
    event.target !== inputElement() ||
    !(event.target instanceof HTMLInputElement)
  )
    return
  draft.value = event.target.value
  dirty.value = true
  panelInteracted = false
  panelTimeChanged = false
  panelWallTime = null
  panelFraction = undefined
  panelTimeError = null
  panelPending.value = false
  syncPickerHighlight(draft.value)
  if (props.live && !composing.value) previewDraft()
  emit('draftChanged')
}

function pickerUpdated(value: string | string[] | null) {
  if (skipPickerUpdate) {
    skipPickerUpdate = false
    return
  }
  if (Array.isArray(value)) {
    reason.value = 'invalid_type'
    emit('invalidValue')
    return
  }
  if (dirty.value && !panelInteracted) return
  pickerValue.value = value
  if (value === null) {
    clear()
    return
  }
  if (props.field.kind === 'date') {
    commitResult(fromPicker(value), value)
    return
  }
}

function pickerConfirmed(
  _timestamp: number | [number, number] | null,
  value: string | string[] | null,
) {
  if (Array.isArray(value)) {
    reason.value = 'invalid_type'
    emit('invalidValue')
    return
  }
  if (props.field.kind === 'datetime') {
    // Naive emits its browser-local formatted value after the confirmation.
    skipPickerUpdate = true
    queueMicrotask(() => {
      skipPickerUpdate = false
    })
  }
  const result = fromPicker(value)
  const text = result.ok
    ? props.field.formatCopy(result.value, props.env)
    : value === null
      ? ''
      : pickerText(value) + (panelFraction ?? fraction)
  if (!commitResult(result, text)) {
    draft.value = text
    dirty.value = true
    emit('draftChanged')
  } else show.value = false
}

function clear() {
  if (props.required) {
    validate(props.field.normalize(null), '', true)
    return
  }
  if (clearing) return
  clearing = true
  queueMicrotask(() => {
    clearing = false
  })
  commitResult(props.field.normalize(null), '')
}

function cancel() {
  const value = baselineValue
  setBaseline(value)
  draft.value = props.field.formatDisplay(value, props.env)
  dirty.value = false
  panelPending.value = false
  reason.value = null
  requiredError.value = false
  show.value = false
  panelInteracted = false
  panelTimeChanged = false
  panelWallTime = null
  panelFraction = undefined
  panelTimeError = null
  emit('draftChanged')
  if (props.live) emit('preview', value)
  emit('cancel')
}

function panelTimeUpdated(value: string) {
  pickerValue.value = value
  const result = fromPicker(value)
  if (!result.ok) {
    reason.value = result.reason
    return
  }
  reason.value = null
  requiredError.value = false
  draft.value = props.field.formatDisplay(result.value, props.env)
  dirty.value = true
  panelPending.value = true
  emit('draftChanged')
}

function panelTimePicked() {
  void nextTick(() => {
    const input = inputElement()
    if (!input) return
    input.focus({ preventScroll: true })
    input.setSelectionRange(input.value.length, input.value.length)
  })
}

function parsePanelTime(text: string) {
  text = normalizeDayPeriod(text.trim())
  if (/^\d{2}:\d{2}$/.test(text)) text += ':00'
  return timeKind.parsePaste(
    text,
    { format: twelveHours.value ? '12h' : '24h' },
    props.env,
  )
}

function panelDateText() {
  const input = portal.value?.querySelector<HTMLInputElement>(
    '.n-date-panel-date-input input',
  )
  const text = input?.value.trim() ?? ''
  const parsed = dateKind.parsePaste(
    text,
    { dateFormat: options.value.dateFormat ?? 'iso' },
    props.env,
  )
  return parsed.ok && typeof parsed.value === 'string' ? parsed.value : text
}

function recordPanelDraft(time: string) {
  const date = panelDateText()
  draft.value = date ? `${date} ${time}` : time
  dirty.value = true
  emit('draftChanged')
}

function rememberPanelTime() {
  if (panelWallTime !== null) return
  if (!panelTimeChanged) {
    if (dirty.value) {
      const parsed = parseText(draft.value)
      if (parsed.ok && parsed.value !== null) {
        const copy = props.field.formatCopy(parsed.value, props.env)
        const match = /^\d{4}-\d{2}-\d{2} (\d{2}:\d{2}:\d{2})(\.\d{1,6})?/.exec(
          copy,
        )
        if (match) {
          panelWallTime = match[1]!
          panelFraction = match[2] ?? ''
          return
        }
      }
    } else if (baselineWall) {
      panelWallTime = baselineWall.slice(11)
      return
    }
  }
  const input = portal.value?.querySelector<HTMLInputElement>(
    '.n-date-panel-header .n-time-picker input',
  )
  if (!input?.value && !baselineWall && !panelTimeChanged) {
    const now = new Date()
    panelWallTime = [now.getHours(), now.getMinutes(), now.getSeconds()]
      .map((part) => String(part).padStart(2, '0'))
      .join(':')
    return
  }
  const parsed = parsePanelTime(input?.value ?? '')
  if (parsed.ok && typeof parsed.value === 'string') {
    panelWallTime =
      parsed.value.slice(0, 6) +
      (baselineWall?.slice(17, 19) ?? parsed.value.slice(6, 8))
    panelTimeError = null
  } else panelTimeError = 'invalid_time'
}

function panelTimeInput(input: HTMLInputElement) {
  rememberPanelTime()
  const seconds = panelWallTime?.slice(6, 8) ?? baselineWall?.slice(17, 19)
  panelTimeChanged = true
  const parsed = parsePanelTime(input.value)
  if (parsed.ok && typeof parsed.value === 'string') {
    panelWallTime =
      parsed.value.slice(0, 6) + (seconds ?? parsed.value.slice(6))
    panelTimeError = null
    recordPanelDraft(panelWallTime + (panelFraction ?? fraction))
  } else {
    panelWallTime = null
    panelTimeError = 'invalid_time'
    recordPanelDraft(input.value)
    if (props.live) emit('invalidValue')
  }
}

function selectPanelTime(item: HTMLElement) {
  if (item.classList.contains('n-time-picker-col__item--disabled')) return
  // Keep the intended wall time even when the browser normalizes a DST gap.
  rememberPanelTime()
  panelTimeChanged = true
  const column = item.closest('.n-time-picker-col')
  const columns = column?.parentElement
    ? Array.from(column.parentElement.children)
    : []
  const index = column ? columns.indexOf(column) : -1
  const formatted = panelWallTime
    ? timeKind.formatDisplay(
        panelWallTime,
        { format: twelveHours.value ? '12h' : '24h' },
        props.env,
      )
    : ''
  const match = /^(\d{2}):(\d{2}):(\d{2})(?: (AM|PM))?$/.exec(formatted)
  if (!match || index < 0 || index > (twelveHours.value ? 2 : 1)) {
    panelTimeError = 'invalid_time'
    return
  }
  let [, hour, minute, second, period] = match
  const selected = item.textContent?.trim() ?? ''
  if (index === 0)
    hour = twelveHours.value && selected === '00' ? '12' : selected
  else if (index === 1) minute = selected
  else period = selected
  const parsed = parsePanelTime(
    `${hour}:${minute}:${second}${period ? ` ${period}` : ''}`,
  )
  if (parsed.ok && typeof parsed.value === 'string') {
    panelWallTime = parsed.value
    panelTimeError = null
  } else panelTimeError = 'invalid_time'
}

function syncPanelTime() {
  queueMicrotask(() => {
    void nextTick(() => {
      if (!panelWallTime) return
      const date = panelDateText()
      if (/^\d{4}-\d{2}-\d{2}$/.test(date))
        browserGap.value = !localWallExists(`${date} ${panelWallTime}`)
      if (dirty.value || panelPending.value)
        recordPanelDraft(panelWallTime + (panelFraction ?? fraction))
      const timeInput = portal.value?.querySelector<HTMLInputElement>(
        '.n-date-panel-header .n-time-picker input',
      )
      if (!timeInput) return
      const formatted = timeKind.formatDisplay(
        panelWallTime,
        { format: twelveHours.value ? '12h' : '24h' },
        props.env,
      )
      const match = /^(\d{2}):(\d{2}):\d{2}(?: (AM|PM))?$/.exec(formatted)
      if (!match) return
      if (!twelveHours.value) timeInput.value = `${match[1]}:${match[2]}`
      else {
        const formatter = new Intl.DateTimeFormat(
          props.env.locale === 'zh-CN' ? 'zh-CN' : 'en',
          { hour: 'numeric', hour12: true, timeZone: 'UTC' },
        )
        const period = formatter
          .formatToParts(
            new Date(Date.UTC(2000, 0, 1, match[3] === 'PM' ? 13 : 1)),
          )
          .find((part) => part.type === 'dayPeriod')?.value
        timeInput.value = `${Number(match[1])}:${match[2]} ${period ?? match[3]}`
      }
    })
  })
}

function panelPointerDown() {
  panelInteracted = true
  pointerInPanel = true
  setTimeout(() => {
    pointerInPanel = false
  }, 0)
}

function panelClick(event: MouseEvent) {
  if (!(event.target instanceof Element)) return
  const item = event.target.closest<HTMLElement>(
    '.n-time-picker-col__item:not(.n-time-picker-col__item--disabled)',
  )
  const day = event.target.closest(
    '.n-date-panel-date:not(.n-date-panel-date--disabled)',
  )
  if (props.field.kind !== 'datetime') return
  if (item) selectPanelTime(item)
  else if (day) rememberPanelTime()
  else return
  panelPending.value = true
  emit('draftChanged')
  syncPanelTime()
}

function panelInput(event: Event) {
  if (props.field.kind !== 'datetime' || !(event.target instanceof Element))
    return
  if (
    event.target instanceof HTMLInputElement &&
    event.target.closest('.n-time-picker')
  )
    panelTimeInput(event.target)
  else if (event.target.closest('.n-date-panel-date-input')) rememberPanelTime()
  else return
  panelPending.value = true
  emit('draftChanged')
  syncPanelTime()
}

function updateShow(value: boolean) {
  show.value = value && !inputElement()?.matches(':disabled')
}

function confirmPanel() {
  if (commitDraft()) show.value = false
}

function keydown(event: KeyboardEvent, inPanel = false) {
  if (event.isComposing || composing.value) {
    event.stopPropagation()
    return
  }
  if (!inPanel && event.target !== inputElement()) return
  if (inPanel) panelInteracted = true
  if (event.altKey && event.key === 'ArrowDown') {
    event.preventDefault()
    show.value = true
    return
  }
  if (event.key === 'Escape') {
    event.preventDefault()
    event.stopPropagation()
    cancel()
    return
  }
  if (event.key !== 'Enter' || event.shiftKey) return
  event.preventDefault()
  if (inPanel || panelInteracted) {
    event.stopPropagation()
    confirmPanel()
    return
  }
  const hadDraft = dirty.value || panelPending.value
  if (commitDraft(!hadDraft && props.gridCommitOnLeave)) {
    show.value = false
    if (!hadDraft) event.stopPropagation()
  } else event.stopPropagation()
}

function togglePanel() {
  show.value = !show.value
  if (show.value && dirty.value && props.field.kind !== 'time')
    syncPickerHighlight(draft.value)
}

function focusLeft(event: FocusEvent) {
  if (contains(event.relatedTarget) || pointerInPanel) return
  if (!props.gridCommitOnLeave && (dirty.value || panelPending.value))
    commitDraft()
  show.value = false
}

function panelFocusOut(event: FocusEvent) {
  focusLeft(event)
  emit('focusout', event)
}

function contains(target: EventTarget | null) {
  return (
    target instanceof Node &&
    Boolean(host.value?.contains(target) || portal.value?.contains(target))
  )
}

function compositionEnded() {
  composing.value = false
  if (props.live) previewDraft()
  emit('compositionFinished')
}

defineExpose({
  commitDraft,
  hasPendingDraft: () => dirty.value || panelPending.value,
  currentDraft,
  isComposing: () => composing.value,
  contains,
  isPointerInPanel: () => pointerInPanel,
})
</script>

<template>
  <div
    ref="host"
    class="temporal-editor"
    @focusout="focusLeft"
    @input="inputChanged"
    @keydown.capture="keydown($event)"
    @compositionstart.capture="composing = true"
    @compositionend.capture="compositionEnded"
  >
    <div class="temporal-editor-control">
      <input
        ref="textInput"
        class="temporal-editor-input"
        type="text"
        :value="draft"
        :placeholder="placeholder"
        :aria-label="label"
        :aria-invalid="Boolean(error)"
        :aria-required="required || undefined"
        :required="required"
        autocomplete="off"
        spellcheck="false"
      />
      <button
        type="button"
        class="temporal-editor-toggle"
        :aria-label="togglePanelLabel"
        :aria-expanded="show"
        @click="togglePanel"
      >
        <svg
          v-if="field.kind === 'time'"
          viewBox="0 0 24 24"
          aria-hidden="true"
          focusable="false"
        >
          <circle cx="12" cy="12" r="9" />
          <path d="M12 7v5l3 2" />
        </svg>
        <svg v-else viewBox="0 0 24 24" aria-hidden="true" focusable="false">
          <rect x="3.5" y="5" width="17" height="16" rx="2" />
          <path d="M7.5 3v4M16.5 3v4M4 9h16" />
        </svg>
      </button>
    </div>
    <p v-if="error" class="temporal-editor-error" role="alert">{{ error }}</p>
    <p v-if="browserGap" class="temporal-editor-warning" role="status">
      {{ t('gridPolish.gp08.browserGap') }}
    </p>
    <Teleport :to="portalDestination">
      <div
        ref="portal"
        class="temporal-picker-portal"
        :style="portalStyle"
        @pointerdown.capture="panelPointerDown"
        @click.capture="panelClick"
        @input.capture="panelInput"
        @keydown.capture="keydown($event, true)"
        @focusout="panelFocusOut"
      >
        <NDatePicker
          v-if="show && field.kind !== 'time'"
          :type="field.kind === 'date' ? 'date' : 'datetime'"
          :panel="true"
          :formatted-value="pickerValue"
          :value-format="
            field.kind === 'date' ? 'yyyy-MM-dd' : 'yyyy-MM-dd HH:mm:ss'
          "
          :format="pickerFormat"
          :date-format="dateFormat"
          :time-picker-format="twelveHours ? 'h:mm a' : 'HH:mm'"
          :time-picker-props="{ use12Hours: twelveHours }"
          :actions="pickerActions"
          :clearable="!required"
          :show="show"
          :year-range="[1, 9999]"
          @update:show="updateShow"
          @update:formatted-value="pickerUpdated"
          @confirm="pickerConfirmed"
          @clear="clear"
        >
          <template #confirm="{ onConfirm, disabled, text }">
            <NButton
              data-temporal-confirm
              size="tiny"
              type="primary"
              :disabled="disabled"
              @click="onConfirm"
            >
              {{ text }}
            </NButton>
          </template>
        </NDatePicker>
        <TimeScroller
          v-else-if="show"
          :value="pickerValue"
          :twelve-hours="twelveHours"
          :clearable="!required"
          @update:value="panelTimeUpdated"
          @pick="panelTimePicked"
          @confirm="confirmPanel"
          @clear="clear"
        />
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.temporal-editor {
  display: grid;
  gap: 4px;
  min-width: 0;
}

.temporal-editor-control {
  position: relative;
  min-width: 0;
}

.temporal-editor-input {
  box-sizing: border-box;
  width: 100%;
  min-height: 34px;
  padding: 6px 38px 6px 10px;
  border: 1px solid var(--n-border-color, #d9d9d9);
  border-radius: 4px;
  color: var(--n-text-color, #222);
  background: var(--n-color, #fff);
  font: inherit;
}

.temporal-editor-input:focus {
  border-color: var(--n-primary-color, #2080f0);
  outline: 2px solid
    color-mix(in srgb, var(--n-primary-color, #2080f0) 20%, transparent);
  outline-offset: 0;
}

.temporal-editor-input[aria-invalid='true'] {
  border-color: #c0362c;
}

.temporal-editor-toggle {
  position: absolute;
  top: 0;
  right: 0;
  display: grid;
  width: 34px;
  height: 100%;
  place-items: center;
  border: 0;
  color: var(--n-text-color-3, #666);
  background: transparent;
  cursor: pointer;
}

.temporal-editor-toggle svg {
  width: 18px;
  height: 18px;
  fill: none;
  stroke: currentColor;
  stroke-linecap: round;
  stroke-linejoin: round;
  stroke-width: 1.7;
}

.temporal-editor-toggle:focus-visible {
  outline: 2px solid var(--n-primary-color, #2080f0);
  outline-offset: -2px;
}

.temporal-picker-portal {
  width: max-content;
  max-width: calc(100vw - 16px);
}
.temporal-editor-error,
.temporal-editor-warning {
  margin: 0;
  font-size: 0.85em;
}
.temporal-editor-error {
  color: #c0362c;
}
:global(:root[data-theme='dark']) .temporal-editor-error {
  color: #ff8a80;
}
</style>
