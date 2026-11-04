<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon, { fieldIconName } from '../AppIcon.vue'
import { fieldKindLabel } from '../../fields/messages'
import { isKind } from '../../fields/registry'
import { USER_KINDS } from './fieldForm'

const props = defineProps<{
  modelValue: string
  expanded?: boolean
  disabled?: boolean
}>()
const emit = defineEmits<{ 'update:modelValue': [kind: string] }>()
const { t } = useI18n()
const open = ref(props.expanded ?? false)
const search = ref('')
watch(
  () => props.expanded,
  (expanded) => {
    open.value = expanded ?? false
  },
)
const kinds = ['link', 'lookup', 'rollup', 'formula', ...USER_KINDS] as const
const label = (kind: string) =>
  isKind(kind) ? fieldKindLabel(kind) : t('field.panel.linkType')
const filtered = computed(() =>
  kinds.filter((kind) =>
    label(kind)
      .toLocaleLowerCase()
      .includes(search.value.trim().toLocaleLowerCase()),
  ),
)
function select(kind: string) {
  emit('update:modelValue', kind)
  open.value = false
  search.value = ''
}
</script>

<template>
  <div class="field-type-picker">
    <button
      v-if="!open"
      type="button"
      class="field-type-selected"
      :disabled="disabled"
      :aria-label="t('field.panel.kind')"
      :aria-expanded="open"
      @click="open = true"
    >
      <AppIcon :name="fieldIconName(modelValue)" />
      <span>{{ label(modelValue) }}</span>
      <AppIcon name="chevronDown" />
    </button>
    <div v-else class="field-type-options">
      <label class="field-type-search">
        <AppIcon name="search" />
        <input
          v-model="search"
          type="search"
          :aria-label="t('field.panel.searchTypes')"
          :placeholder="t('field.panel.searchTypes')"
          :disabled="disabled"
        />
      </label>
      <div class="field-type-list" :aria-label="t('field.panel.kind')">
        <button
          v-for="kind in filtered"
          :key="kind"
          type="button"
          :data-kind="kind"
          :disabled="disabled"
          :aria-pressed="modelValue === kind"
          @click="select(kind)"
        >
          <AppIcon :name="fieldIconName(kind)" />
          <span>{{ label(kind) }}</span>
        </button>
        <p v-if="!filtered.length">{{ t('field.panel.noMatches') }}</p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.field-type-picker {
  min-width: 0;
}
.field-type-selected,
.field-type-list button {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  text-align: start;
}
.field-type-selected span {
  flex: 1;
}
.field-type-options {
  border: 1px solid var(--border, #dedee5);
  border-radius: 8px;
  overflow: hidden;
}
.field-type-search {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--border, #dedee5);
}
.field-type-search input {
  min-width: 0;
  width: 100%;
  border: 0;
  background: transparent;
}
.field-type-list {
  max-height: 290px;
  overflow-y: auto;
  overscroll-behavior: contain;
  padding: 6px;
}
.field-type-list button {
  min-height: 36px;
  border: 0;
  background: transparent;
  border-radius: 4px;
  padding: 8px;
}
.field-type-list button:hover,
.field-type-list button:focus-visible,
.field-type-list button[aria-pressed='true'] {
  background: var(--surface-hover, #f3f3f6);
}
.field-type-list p {
  padding: 8px;
}
</style>
