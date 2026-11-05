<script setup lang="ts">
import {
  computed,
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  useId,
  type CSSProperties,
} from 'vue'
import { useI18n } from 'vue-i18n'
import FieldRenderer from '../fields/FieldRenderer.vue'
import type { BoundField } from '../../fields/registry'
import type { ChoiceOptions } from '../../fields/choice'
import type { FieldEnv } from '../../fields/types'

const props = defineProps<{
  field: BoundField
  env: FieldEnv
  /** Labels currently stored in the cell, including unconfigured ones. */
  original: string[]
  /** Labels the editor starts with (a recovered draft or the stored value). */
  initial: string[]
  label: string
  error?: string
}>()
const emit = defineEmits<{
  pick: [labels: string[], keyboard: boolean]
  commit: []
  cancel: []
}>()
const { t } = useI18n()
const id = `grid-choice-${useId()}`
const list = ref<HTMLElement>()
const root = ref<HTMLElement>()
const popover = ref<HTMLElement>()
const placement = ref<CSSProperties>({})
const multiple = computed(() => props.field.kind === 'multi_select')
const selected = ref<string[]>([...props.initial])

interface ChoiceOption {
  key: string
  label: string | null
}
const options = computed<ChoiceOption[]>(() => {
  const choices = (props.field.options as ChoiceOptions).choices ?? []
  const result: ChoiceOption[] = []
  if (!multiple.value) result.push({ key: 'none', label: null })
  // Stored labels that are no longer configured stay visible (and removable).
  for (const label of new Set([...props.original, ...props.initial]))
    if (!choices.some((choice) => choice.label === label))
      result.push({ key: `unknown:${label}`, label })
  for (const choice of choices)
    result.push({ key: `choice:${choice.id}`, label: choice.label })
  return result
})
const isSelected = (option: ChoiceOption) =>
  option.label === null
    ? selected.value.length === 0
    : selected.value.includes(option.label)
const active = ref(
  Math.max(
    0,
    options.value.findIndex((option) => isSelected(option)),
  ),
)
const optionId = (index: number) => `${id}-${index}`

function scrollActive() {
  void nextTick(() =>
    document
      .getElementById(optionId(active.value))
      ?.scrollIntoView?.({ block: 'nearest' }),
  )
}

onMounted(() => {
  place()
  window.addEventListener('resize', place)
  window.addEventListener('scroll', reposition, true)
  list.value?.focus({ preventScroll: true })
  scrollActive()
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', place)
  window.removeEventListener('scroll', reposition, true)
})

function reposition(event: Event) {
  if (!popover.value?.contains(event.target as Node | null)) place()
}

// The grid viewport clips overflow, so the popover is viewport-fixed: below
// the cell when it fits, otherwise flipped to end at the cell's bottom edge.
function place() {
  const anchor = root.value?.getBoundingClientRect()
  const element = popover.value
  if (!anchor || !element) return
  const height = element.offsetHeight
  const width = Math.max(element.offsetWidth, anchor.width)
  const viewHeight = window.innerHeight
  const viewWidth = window.innerWidth
  let top = anchor.top
  if (top + height > viewHeight && anchor.bottom - height >= 0)
    top = anchor.bottom - height
  top = Math.max(0, Math.min(top, viewHeight - height))
  const left = Math.max(0, Math.min(anchor.left, viewWidth - width))
  placement.value = {
    position: 'fixed',
    top: `${top}px`,
    left: `${left}px`,
    minWidth: `${anchor.width}px`,
  }
  // A transformed ancestor would re-base fixed coordinates; correct for it.
  void nextTick(() => {
    const actual = popover.value?.getBoundingClientRect()
    if (!actual) return
    const dx = actual.left - left
    const dy = actual.top - top
    if (Math.abs(dx) < 0.5 && Math.abs(dy) < 0.5) return
    placement.value = {
      ...placement.value,
      top: `${top - dy}px`,
      left: `${left - dx}px`,
    }
  })
}

function choose(index: number, keyboard: boolean) {
  const option = options.value[index]
  if (!option) return
  active.value = index
  if (!multiple.value) {
    emit('pick', option.label === null ? [] : [option.label], keyboard)
    return
  }
  if (option.label === null) return
  const label = option.label
  selected.value = selected.value.includes(label)
    ? selected.value.filter((item) => item !== label)
    : [...selected.value, label]
}

function move(index: number) {
  active.value = Math.max(0, Math.min(options.value.length - 1, index))
  scrollActive()
}

function keydown(event: KeyboardEvent) {
  // Tab reaches RevoGrid so it moves focus and the leave boundary commits.
  if (event.key === 'Tab' || event.isComposing) return
  event.stopPropagation()
  const handled = () => event.preventDefault()
  switch (event.key) {
    case 'ArrowDown':
      handled()
      move(active.value + 1)
      break
    case 'ArrowUp':
      handled()
      move(active.value - 1)
      break
    case 'Home':
      handled()
      move(0)
      break
    case 'End':
      handled()
      move(options.value.length - 1)
      break
    case ' ':
      handled()
      choose(active.value, true)
      break
    case 'Enter':
      handled()
      if (multiple.value) emit('commit')
      else choose(active.value, true)
      break
    case 'Escape':
      handled()
      emit('cancel')
      break
  }
}

defineExpose({ selection: () => [...selected.value] })
</script>

<template>
  <div ref="root" class="grid-choice-editor" @dblclick.stop @touchstart.stop>
    <div ref="popover" class="grid-choice-popover" :style="placement">
      <div
        :id="id"
        ref="list"
        class="grid-choice-list"
        role="listbox"
        tabindex="0"
        :aria-label="label"
        :aria-multiselectable="multiple ? 'true' : undefined"
        :aria-activedescendant="optionId(active)"
        :aria-invalid="error ? 'true' : undefined"
        @keydown="keydown"
        @mousedown.prevent
      >
        <div
          v-for="(option, index) in options"
          :id="optionId(index)"
          :key="option.key"
          class="grid-choice-option"
          :class="{ active: index === active }"
          role="option"
          :aria-selected="isSelected(option) ? 'true' : 'false'"
          :data-choice="option.label ?? ''"
          @click="choose(index, false)"
        >
          <span class="grid-choice-mark" aria-hidden="true" />
          <span v-if="option.label === null" class="grid-choice-none">{{
            t('field.editor.none')
          }}</span>
          <FieldRenderer
            v-else
            :field="field"
            :value="multiple ? [option.label] : option.label"
            :env="env"
          />
        </div>
      </div>
      <p v-if="error" class="grid-choice-error" role="alert">{{ error }}</p>
    </div>
  </div>
</template>

<style scoped>
.grid-choice-editor {
  position: relative;
  min-width: 100%;
  height: 100%;
}
.grid-choice-popover {
  position: absolute;
  z-index: 20;
  top: 0;
  left: 0;
  min-width: 100%;
  max-width: 320px;
  background: var(--surface);
  color: var(--text);
  border: 2px solid var(--accent);
  box-shadow: 0 4px 12px rgb(0 0 0 / 18%);
}
.grid-choice-list {
  display: grid;
  max-height: 240px;
  padding: 2px;
  overflow: auto;
  outline: none;
}
.grid-choice-option {
  display: flex;
  gap: 6px;
  align-items: center;
  min-height: 26px;
  padding: 2px 6px;
  cursor: pointer;
  white-space: nowrap;
}
.grid-choice-option:hover,
.grid-choice-option.active {
  background: color-mix(in srgb, var(--accent) 14%, var(--surface));
}
.grid-choice-list:focus-visible .grid-choice-option.active {
  outline: 2px solid var(--accent);
  outline-offset: -2px;
}
.grid-choice-mark {
  flex: 0 0 1em;
  color: var(--accent);
}
.grid-choice-option[aria-selected='true'] .grid-choice-mark::after {
  content: '\2713';
}
.grid-choice-none {
  color: var(--muted);
}
.grid-choice-error {
  margin: 0;
  padding: 2px 6px;
  white-space: normal;
  color: #c0362c;
  font-size: 0.85em;
}
:global(:root[data-theme='dark']) .grid-choice-error {
  color: #ff8a80;
}
</style>
