<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, useId, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  CHOICE_COLORS,
  isChoiceColor,
  type ChoiceColor,
} from '../../fields/choice'
import AppIcon from '../AppIcon.vue'
import { colorLabel } from './fieldForm'

const props = defineProps<{
  modelValue: string
  optionNumber: number
  disabled: boolean
}>()
const emit = defineEmits<{ 'update:modelValue': [color: ChoiceColor] }>()
const { t } = useI18n()
const paletteId = `choice-colors-${useId()}`
const root = ref<HTMLElement>()
const trigger = ref<HTMLButtonElement>()
const buttons = ref<HTMLButtonElement[]>([])
const open = ref(false)
const active = ref(0)
const columns = 5

const swatches: Record<ChoiceColor, string> = {
  gray: '#8a8f98',
  red: '#e5484d',
  orange: '#f76b15',
  amber: '#ffb224',
  yellow: '#f5d90a',
  lime: '#99d52a',
  green: '#30a46c',
  teal: '#12a594',
  cyan: '#05a2c2',
  blue: '#0090ff',
  indigo: '#3e63dd',
  purple: '#8e4ec6',
  pink: '#d6409f',
}
const colorName = computed(() =>
  isChoiceColor(props.modelValue)
    ? colorLabel(props.modelValue)
    : props.modelValue,
)
const currentSwatch = computed(() =>
  isChoiceColor(props.modelValue) ? swatches[props.modelValue] : 'transparent',
)

function setButton(element: unknown, index: number) {
  if (element instanceof HTMLButtonElement) buttons.value[index] = element
}

function focusOption(index: number) {
  active.value = Math.max(0, Math.min(CHOICE_COLORS.length - 1, index))
  void nextTick(() => buttons.value[active.value]?.focus())
}

function close(restoreFocus = false) {
  if (!open.value) return
  open.value = false
  document.removeEventListener('pointerdown', dismiss, true)
  buttons.value = []
  if (restoreFocus) void nextTick(() => trigger.value?.focus())
}

function dismiss(event: PointerEvent) {
  if (!(event.target instanceof Node) || !root.value?.contains(event.target))
    close()
}

function focusOut(event: FocusEvent) {
  if (
    open.value &&
    (!(event.relatedTarget instanceof Node) ||
      !root.value?.contains(event.relatedTarget))
  )
    close()
}

function toggle() {
  if (open.value) {
    close()
    return
  }
  active.value = Math.max(
    0,
    CHOICE_COLORS.findIndex((color) => color === props.modelValue),
  )
  open.value = true
  document.addEventListener('pointerdown', dismiss, true)
  focusOption(active.value)
}

function choose(color: ChoiceColor) {
  emit('update:modelValue', color)
  close(true)
}

function keydown(event: KeyboardEvent) {
  if (!open.value || event.isComposing) return
  if (event.key === 'Escape') {
    event.preventDefault()
    event.stopPropagation()
    close(true)
    return
  }
  if (!(event.target instanceof Node) || !root.value?.contains(event.target))
    return
  if (event.target === trigger.value) return
  let next = active.value
  switch (event.key) {
    case 'ArrowLeft':
      next--
      break
    case 'ArrowRight':
      next++
      break
    case 'ArrowUp':
      next -= columns
      break
    case 'ArrowDown':
      next += columns
      break
    case 'Home':
      next = 0
      break
    case 'End':
      next = CHOICE_COLORS.length - 1
      break
    case 'Enter':
    case ' ':
      event.preventDefault()
      event.stopPropagation()
      choose(CHOICE_COLORS[active.value]!)
      return
    default:
      return
  }
  event.preventDefault()
  event.stopPropagation()
  focusOption(next)
}

watch(
  () => props.disabled,
  (disabled) => {
    if (disabled) close()
  },
)
onBeforeUnmount(() =>
  document.removeEventListener('pointerdown', dismiss, true),
)
</script>

<template>
  <div
    ref="root"
    class="choice-color-picker"
    @focusout="focusOut"
    @keydown="keydown"
  >
    <button
      ref="trigger"
      type="button"
      class="choice-color-trigger"
      :disabled="disabled"
      :aria-label="
        t('gridPolish.gp09.colorButton', { n: optionNumber, color: colorName })
      "
      aria-haspopup="listbox"
      :aria-expanded="open"
      :aria-controls="open ? paletteId : undefined"
      @click="toggle"
    >
      <span
        class="choice-color-swatch"
        :style="{ backgroundColor: currentSwatch }"
        aria-hidden="true"
      />
      <AppIcon name="chevron-down" :size="14" />
    </button>
    <div
      v-if="open"
      :id="paletteId"
      class="choice-color-palette"
      role="listbox"
      :aria-label="t('field.panel.choiceColor', { n: optionNumber })"
    >
      <button
        v-for="(color, index) in CHOICE_COLORS"
        :key="color"
        :ref="(element) => setButton(element, index)"
        type="button"
        class="choice-color-option"
        role="option"
        :style="{ backgroundColor: swatches[color] }"
        :aria-label="colorLabel(color)"
        :aria-selected="modelValue === color"
        :title="colorLabel(color)"
        :tabindex="active === index ? 0 : -1"
        @focus="active = index"
        @click="choose(color)"
      >
        <AppIcon v-if="modelValue === color" name="checkMark" :size="18" />
      </button>
    </div>
  </div>
</template>

<style scoped>
.choice-color-picker {
  position: relative;
  flex: 0 0 auto;
}
.choice-color-trigger {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 5px 6px;
}
.choice-color-swatch {
  width: 20px;
  height: 20px;
  border: 1px solid rgb(0 0 0 / 20%);
  border-radius: 4px;
}
.choice-color-palette {
  position: absolute;
  z-index: 20;
  top: calc(100% + 4px);
  left: 0;
  display: grid;
  grid-template-columns: repeat(5, 32px);
  gap: 4px;
  padding: 8px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 8px;
  box-shadow: 0 6px 24px rgb(0 0 0 / 18%);
}
.choice-color-option {
  display: grid;
  width: 32px;
  height: 32px;
  padding: 0;
  place-items: center;
  color: white;
  border: 1px solid rgb(0 0 0 / 20%);
  border-radius: 4px;
  cursor: pointer;
}
.choice-color-option svg {
  filter: drop-shadow(0 1px 2px rgb(0 0 0 / 80%));
}
.choice-color-option:focus-visible {
  outline: 2px solid var(--text);
  outline-offset: 2px;
}
</style>
