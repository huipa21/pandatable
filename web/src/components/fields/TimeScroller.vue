<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import type { ComponentPublicInstance } from 'vue'
import { useI18n } from 'vue-i18n'

const props = withDefaults(
  defineProps<{
    value: string | null
    twelveHours: boolean
    clearable?: boolean
  }>(),
  { clearable: true },
)
const emit = defineEmits<{
  'update:value': [value: string]
  pick: [value: string]
  confirm: []
  clear: []
}>()
const { t } = useI18n()
const ROW_HEIGHT = 32
const selected = ref(props.value ?? currentTime())
const columns = ref<HTMLElement[]>([])

function currentTime() {
  const now = new Date()
  return [now.getHours(), now.getMinutes(), now.getSeconds()]
    .map((part) => String(part).padStart(2, '0'))
    .join(':')
}

function setColumnRef(
  index: number,
  element: Element | ComponentPublicInstance | null,
) {
  if (element instanceof HTMLElement) columns.value[index] = element
}

function selectionIndex(index: number) {
  const [hour, minute, second] = selected.value.split(':').map(Number)
  if (props.twelveHours && index === 0) return hour! % 12 || 12
  if (index === 0) return hour!
  if (index === 1) return minute!
  if (index === 2 && !props.twelveHours) return second!
  return hour! >= 12 ? 1 : 0
}

function scrollToSelection() {
  columns.value.forEach((column, index) => {
    const target =
      selectionIndex(index) - (props.twelveHours && index === 0 ? 1 : 0)
    column.scrollTop = target * ROW_HEIGHT
  })
}

function updateSelected(next: string, picked = false) {
  if (next === selected.value) {
    if (picked) emit('pick', next)
    return
  }
  selected.value = next
  emit('update:value', next)
  if (picked) emit('pick', next)
}

function setPart(index: number, value: number, picked = false) {
  let [hour, minute, second] = selected.value.split(':').map(Number)
  if (props.twelveHours && index === 2) {
    const isPM = hour! >= 12
    if (isPM !== (value === 1)) hour = (hour! % 12) + (value === 1 ? 12 : 0)
  } else if (index === 0) {
    hour = props.twelveHours ? (hour! >= 12 ? 12 : 0) + (value % 12) : value
  } else if (index === 1) minute = value
  else second = value
  updateSelected(
    `${String(hour).padStart(2, '0')}:${String(minute).padStart(2, '0')}:${String(second).padStart(2, '0')}`,
    picked,
  )
}

function handleKeydown(index: number, event: KeyboardEvent) {
  if (event.key !== 'ArrowUp' && event.key !== 'ArrowDown') return
  event.preventDefault()
  event.stopPropagation()
  const delta = event.key === 'ArrowUp' ? -1 : 1
  const count =
    index === 0
      ? props.twelveHours
        ? 12
        : 24
      : props.twelveHours && index === 2
        ? 2
        : 60
  const current =
    selectionIndex(index) - (props.twelveHours && index === 0 ? 1 : 0)
  const next = (current + delta + count) % count
  setPart(index, index === 0 && props.twelveHours ? next + 1 : next)
}

function handleScroll(index: number, event: Event) {
  const element = event.currentTarget
  if (!(element instanceof HTMLElement)) return
  const indexAtCenter = Math.round(element.scrollTop / ROW_HEIGHT)
  setPart(
    index,
    index === 0 && props.twelveHours ? indexAtCenter + 1 : indexAtCenter,
  )
}

const hourValues = computed(() =>
  Array.from({ length: props.twelveHours ? 12 : 24 }, (_, index) =>
    props.twelveHours ? index + 1 : index,
  ),
)
const minuteValues = Array.from({ length: 60 }, (_, index) => index)
const secondValues = minuteValues
const periodValues = [0, 1]
const displayedColumns = computed(() => {
  const result = [
    { label: t('cellEditors.ces02.hour'), values: hourValues.value },
    { label: t('cellEditors.ces02.minute'), values: minuteValues },
  ]
  if (!props.twelveHours)
    result.push({ label: t('cellEditors.ces02.second'), values: secondValues })
  else
    result.push({ label: t('cellEditors.ces02.period'), values: periodValues })
  return result
})

watch(
  () => props.value,
  (value) => {
    selected.value = value ?? currentTime()
  },
)
watch([selected, () => props.twelveHours], () => {
  void nextTick(scrollToSelection)
})
onMounted(() => void nextTick(scrollToSelection))

function labelFor(index: number, value: number) {
  if (props.twelveHours && index === 2)
    return value === 1 ? t('cellEditors.ces02.pm') : t('cellEditors.ces02.am')
  return String(value).padStart(2, '0')
}
</script>

<template>
  <div class="time-scroller" data-time-scroller>
    <div class="time-scroller-columns">
      <div
        v-for="(column, columnIndex) in displayedColumns"
        :key="column.label"
        :ref="(element) => setColumnRef(columnIndex, element)"
        class="time-scroller-column"
        role="listbox"
        :aria-label="column.label"
        @scroll.passive="handleScroll(columnIndex, $event)"
      >
        <button
          v-for="optionValue in column.values"
          :key="optionValue"
          type="button"
          class="time-scroller-option"
          role="option"
          :aria-selected="selectionIndex(columnIndex) === optionValue"
          :data-time-value="optionValue"
          @click="setPart(columnIndex, optionValue, true)"
          @keydown="handleKeydown(columnIndex, $event)"
        >
          {{ labelFor(columnIndex, optionValue) }}
        </button>
      </div>
    </div>
    <div class="time-scroller-actions">
      <button
        v-if="clearable"
        type="button"
        class="time-scroller-clear"
        @click="emit('clear')"
      >
        {{ t('cellEditors.ces02.clear') }}
      </button>
      <button
        type="button"
        class="time-scroller-confirm"
        data-temporal-confirm
        @click="emit('confirm')"
      >
        {{ t('cellEditors.ces02.confirm') }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.time-scroller {
  min-width: 240px;
  padding: 8px;
  color: var(--n-text-color, #222);
  background: var(--n-color, #fff);
  border: 1px solid var(--n-border-color, #ddd);
  border-radius: 6px;
  box-shadow: 0 3px 12px rgb(0 0 0 / 16%);
}

.time-scroller-columns {
  display: flex;
  justify-content: center;
  gap: 4px;
}

.time-scroller-column {
  box-sizing: border-box;
  display: flex;
  width: 56px;
  height: 160px;
  flex-direction: column;
  overflow-y: auto;
  overscroll-behavior: contain;
  scroll-snap-type: y mandatory;
  scrollbar-width: thin;
  padding-block: 64px;
}

.time-scroller-option {
  min-height: 32px;
  flex: 0 0 32px;
  border: 0;
  border-radius: 4px;
  color: inherit;
  background: transparent;
  cursor: pointer;
  scroll-snap-align: center;
}

.time-scroller-option[aria-selected='true'] {
  color: var(--n-primary-color, #2080f0);
  background: var(--n-primary-color-suppl, #e6f3ff);
  font-weight: 600;
}

.time-scroller-option:focus-visible,
.time-scroller-actions button:focus-visible {
  outline: 2px solid var(--n-primary-color, #2080f0);
  outline-offset: 1px;
}

.time-scroller-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 8px;
}

.time-scroller-actions button {
  min-height: 30px;
  padding: 4px 10px;
  border: 1px solid var(--n-border-color, #ddd);
  border-radius: 4px;
  color: inherit;
  background: transparent;
  cursor: pointer;
}

.time-scroller-confirm {
  color: var(--n-primary-color, #fff) !important;
  background: var(--n-primary-color, #2080f0) !important;
}
</style>
