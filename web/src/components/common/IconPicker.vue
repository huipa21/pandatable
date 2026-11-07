<script setup lang="ts">
import { nextTick, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { BookIconName } from '../../api/types'
import AppIcon from '../AppIcon.vue'

const props = defineProps<{ modelValue: BookIconName | null }>()
const emit = defineEmits<{
  'update:modelValue': [value: BookIconName | null]
}>()
const { t } = useI18n()
const choices = [
  'panelClose',
  'panelOpen',
  'search',
  'sun',
  'moon',
  'undo',
  'redo',
  'table',
  'book',
  'view',
  'chevronDown',
  'chevronRight',
  'plus',
  'more',
  'fields',
  'filter',
  'group',
  'sort',
  'colour',
  'height',
  'trash',
  'eye',
  'expand',
  'fullscreen',
  'close',
  'duplicate',
  'chevronUp',
  'comments',
  'history',
  'compact',
  'edit',
  'calendar',
  'clock',
  'check',
  'link',
  'text',
  'number',
  'formula',
  'lookup',
  'rollup',
  'attachment',
  'mail',
  'star',
  'phone',
  'code',
  'user',
  'percent',
  'currency',
] satisfies readonly BookIconName[]
const options: readonly (BookIconName | null)[] = [null, ...choices]
const buttons = ref<HTMLButtonElement[]>([])

function setButton(element: unknown, index: number) {
  if (element instanceof HTMLButtonElement) buttons.value[index] = element
}

function focusOption(index: number) {
  const next = (index + options.length) % options.length
  emit('update:modelValue', options[next]!)
  void nextTick(() => buttons.value[next]?.focus())
}

function onKeydown(event: KeyboardEvent, index: number) {
  const delta =
    event.key === 'ArrowRight' || event.key === 'ArrowDown'
      ? 1
      : event.key === 'ArrowLeft' || event.key === 'ArrowUp'
        ? -1
        : 0
  if (!delta) return
  event.preventDefault()
  focusOption(index + delta)
}
</script>

<template>
  <div
    class="icon-picker"
    role="radiogroup"
    :aria-label="t('dashboard.ds06.iconLabel')"
  >
    <button
      v-for="(choice, index) in options"
      :key="choice ?? 'random'"
      :ref="(element) => setButton(element, index)"
      type="button"
      class="icon-picker-option"
      :class="{ selected: props.modelValue === choice }"
      role="radio"
      :aria-label="
        choice === null
          ? t('dashboard.ds06.randomIcon')
          : t('dashboard.ds06.iconOption', { icon: choice })
      "
      :aria-checked="props.modelValue === choice"
      :tabindex="props.modelValue === choice ? 0 : -1"
      @click="emit('update:modelValue', choice)"
      @keydown="onKeydown($event, index)"
    >
      <span v-if="choice === null" class="icon-picker-random">
        {{ t('dashboard.ds06.randomIconShort') }}
      </span>
      <AppIcon v-else :name="choice" :size="20" />
    </button>
  </div>
</template>
