<script setup lang="ts">
import { computed, nextTick, ref, useId, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon, { fieldIconName } from '../AppIcon.vue'
import type { QuickSearch, QuickSearchField } from './quickSearch'

const props = defineProps<{
  fields: QuickSearchField[]
  modelValue: QuickSearch
}>()
const emit = defineEmits<{ 'update:modelValue': [value: QuickSearch] }>()
const { t } = useI18n()
const id = useId()
const menu = ref<HTMLDetailsElement>()
const trigger = ref<HTMLElement>()
const input = ref<HTMLInputElement>()
const fieldButton = ref<HTMLButtonElement>()
const list = ref<HTMLUListElement>()
const listOpen = ref(false)
const activeIndex = ref(0)

const options = computed(() => [
  { id: null, name: t('gridPolish.gp10.allFields'), uiType: null },
  ...props.fields,
])
const selectedIndex = computed(() =>
  Math.max(
    0,
    options.value.findIndex((option) => option.id === props.modelValue.fieldId),
  ),
)
const selected = computed(() => options.value[selectedIndex.value]!)

function update(patch: Partial<QuickSearch>) {
  emit('update:modelValue', { ...props.modelValue, ...patch })
}

function optionId(index: number) {
  return `${id}-option-${index}`
}

function openList() {
  activeIndex.value = selectedIndex.value
  listOpen.value = true
  void nextTick(() => list.value?.focus())
}

function closeList(focusButton = true) {
  if (!listOpen.value) return
  listOpen.value = false
  if (focusButton) fieldButton.value?.focus()
}

function choose(index: number) {
  const option = options.value[index]
  if (!option) return
  listOpen.value = false
  if (option.id !== props.modelValue.fieldId) update({ fieldId: option.id })
  void nextTick(() => input.value?.focus())
}

function clear() {
  update({ term: '' })
  input.value?.focus()
}

function close() {
  if (!menu.value?.open) return
  listOpen.value = false
  menu.value.open = false
  trigger.value?.focus()
}

function listKeydown(event: KeyboardEvent) {
  const last = options.value.length - 1
  switch (event.key) {
    case 'ArrowDown':
      activeIndex.value = Math.min(last, activeIndex.value + 1)
      break
    case 'ArrowUp':
      activeIndex.value = Math.max(0, activeIndex.value - 1)
      break
    case 'Home':
      activeIndex.value = 0
      break
    case 'End':
      activeIndex.value = last
      break
    case 'Enter':
    case ' ':
      choose(activeIndex.value)
      break
    case 'Tab':
      closeList(false)
      return
    default:
      return
  }
  event.preventDefault()
  document
    .getElementById(optionId(activeIndex.value))
    ?.scrollIntoView?.({ block: 'nearest' })
}

function keydown(event: KeyboardEvent) {
  if (event.key !== 'Escape' || !menu.value?.open) return
  event.preventDefault()
  event.stopPropagation()
  if (listOpen.value) closeList()
  else close()
}

function toggled() {
  if (menu.value?.open) void nextTick(() => input.value?.focus())
  else listOpen.value = false
}

function listFocusout(event: FocusEvent) {
  const next = event.relatedTarget
  if (!(next instanceof Node) || !list.value?.contains(next)) closeList(false)
}

watch(
  () => props.fields.map((field) => field.id).join('\n'),
  () => {
    activeIndex.value = Math.min(activeIndex.value, options.value.length - 1)
  },
)

defineExpose({ close })
</script>

<template>
  <details
    ref="menu"
    class="grid-search"
    :data-active="modelValue.term ? '' : undefined"
    @toggle="toggled"
    @keydown="keydown"
  >
    <summary
      ref="trigger"
      class="grid-toolbar-button grid-search-trigger"
      :aria-label="t('grid.quickSearch')"
      :title="t('grid.quickSearch')"
    >
      <AppIcon name="search" />
      <span v-if="modelValue.term" class="grid-search-dot" aria-hidden="true" />
    </summary>
    <div class="grid-search-menu" role="search">
      <div class="grid-search-field">
        <span :id="`${id}-field-label`" class="grid-search-label">
          {{ t('gridPolish.gp10.field') }}
        </span>
        <button
          ref="fieldButton"
          type="button"
          class="grid-search-field-button"
          aria-haspopup="listbox"
          :aria-expanded="listOpen"
          :aria-controls="`${id}-fields`"
          :aria-labelledby="`${id}-field-label ${id}-field-value`"
          @click="listOpen ? closeList() : openList()"
        >
          <AppIcon
            :name="selected.uiType ? fieldIconName(selected.uiType) : 'fields'"
            :size="16"
          />
          <span :id="`${id}-field-value`" class="grid-search-field-name">{{
            selected.name
          }}</span>
          <AppIcon name="chevron-down" :size="16" />
        </button>
        <ul
          v-show="listOpen"
          :id="`${id}-fields`"
          ref="list"
          class="grid-search-options"
          role="listbox"
          tabindex="-1"
          :aria-labelledby="`${id}-field-label`"
          :aria-activedescendant="listOpen ? optionId(activeIndex) : undefined"
          @keydown="listKeydown"
          @focusout="listFocusout"
        >
          <li
            v-for="(option, index) in options"
            :id="optionId(index)"
            :key="option.id ?? ''"
            role="option"
            :aria-selected="index === selectedIndex"
            :data-field-id="option.id ?? undefined"
            :class="{ 'grid-search-option-active': index === activeIndex }"
            @mousedown.prevent
            @mouseenter="activeIndex = index"
            @click="choose(index)"
          >
            <AppIcon
              :name="option.uiType ? fieldIconName(option.uiType) : 'fields'"
              :size="16"
            />
            <span>{{ option.name }}</span>
          </li>
        </ul>
      </div>
      <div class="grid-search-keyword">
        <input
          ref="input"
          :value="modelValue.term"
          type="search"
          maxlength="512"
          :aria-label="t('grid.quickSearch')"
          :placeholder="t('grid.quickSearchPlaceholder')"
          @input="update({ term: ($event.target as HTMLInputElement).value })"
        />
        <button
          v-if="modelValue.term"
          type="button"
          class="grid-search-clear"
          :aria-label="t('gridPolish.gp10.clear')"
          :title="t('gridPolish.gp10.clear')"
          @click="clear"
        >
          <AppIcon name="close" :size="14" />
        </button>
      </div>
    </div>
  </details>
</template>

<style scoped>
.grid-search {
  position: static;
}

.grid-search > summary {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 5px;
  min-width: 40px;
  min-height: 40px;
  padding: 8px 6px;
  color: var(--text);
  background: transparent;
  border: 1px solid transparent;
  border-radius: 6px;
  cursor: pointer;
  list-style: none;
}

.grid-search > summary::-webkit-details-marker {
  display: none;
}

.grid-search[open] > summary,
.grid-search > summary:hover,
.grid-search[data-active] > summary {
  background: color-mix(in srgb, var(--accent) 10%, transparent);
}

.grid-search-dot {
  position: absolute;
  top: 7px;
  right: 6px;
  width: 7px;
  height: 7px;
  background: var(--accent);
  border-radius: 50%;
}

.grid-search-menu {
  position: absolute;
  z-index: 3;
  top: 100%;
  right: 0;
  display: grid;
  gap: 10px;
  width: min(340px, calc(100vw - 24px));
  max-height: 65vh;
  padding: 12px;
  color: var(--text);
  background: var(--panel-surface);
  border: 1px solid var(--border);
  border-radius: 8px;
  box-shadow: 0 8px 24px rgb(0 0 0 / 14%);
}

.grid-search-field {
  position: relative;
  display: grid;
  gap: 4px;
}

.grid-search-label {
  font-size: 0.875em;
  color: var(--muted);
}

.grid-search-field-button {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  min-height: 36px;
  text-align: start;
}

.grid-search-field-name {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.grid-search-options {
  position: absolute;
  z-index: 1;
  top: 100%;
  right: 0;
  left: 0;
  max-height: 240px;
  margin: 2px 0 0;
  padding: 4px;
  overflow: auto;
  list-style: none;
  background: var(--panel-surface);
  border: 1px solid var(--border);
  border-radius: 6px;
  box-shadow: 0 8px 24px rgb(0 0 0 / 14%);
}

.grid-search-options:focus {
  outline: 2px solid var(--accent);
  outline-offset: 1px;
}

.grid-search-options li {
  display: flex;
  align-items: center;
  gap: 6px;
  min-height: 32px;
  padding: 4px 8px;
  border-radius: 4px;
  cursor: pointer;
}

.grid-search-options li span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.grid-search-options li[aria-selected='true'] {
  color: var(--accent);
}

.grid-search-options .grid-search-option-active {
  background: color-mix(in srgb, var(--accent) 12%, transparent);
}

.grid-search-keyword {
  position: relative;
  display: flex;
  align-items: center;
}

.grid-search-keyword input {
  width: 100%;
  padding-right: 32px;
}

.grid-search-clear {
  position: absolute;
  right: 4px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  min-width: 0;
  height: 26px;
  min-height: 0;
  padding: 0;
  background: transparent;
  border: 0;
}
</style>
