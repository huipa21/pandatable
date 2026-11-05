<script setup lang="ts">
import { computed } from 'vue'
import type { HighlightColor } from '../../api/types'
import type { BoundField } from '../../fields/registry'
import type { FieldEnv, WireValue } from '../../fields/types'
import FieldRenderer from '../fields/FieldRenderer.vue'
import CellPresence from '../presence/CellPresence.vue'

const props = defineProps<{
  field: BoundField
  value: WireValue
  env: FieldEnv
  state?: () => {
    draft?: { text: string; pending: boolean }
    message: string
  }
  message?: string
  savingLabel?: string
  tableId: string
  identity: string
  rowId: string
  fieldId: string
  rowHighlight?: HighlightColor
  cellHighlight?: HighlightColor
  group?: {
    header: boolean
    path: string
    label: string
    expanded: boolean
    ariaLabel: string
    aggregate?: string
    toggle: () => void
  }
}>()
const cellState = computed(
  () => props.state?.() ?? { draft: undefined, message: props.message ?? '' },
)
// Choice drafts are label lists; render them as chips when they still parse.
const choiceDraft = computed(() => {
  const draft = cellState.value.draft
  if (!draft || props.field.presentation !== 'select') return undefined
  const parsed = props.field.parsePaste(draft.text, props.env)
  return parsed.ok ? { value: parsed.value } : undefined
})
</script>

<template>
  <div
    class="grid-highlight-cell"
    :data-row-highlight="rowHighlight"
    :data-cell-highlight="cellHighlight"
  >
    <button
      v-if="group?.header"
      type="button"
      class="grid-group-toggle"
      :data-group-path="group.path"
      :aria-label="group.ariaLabel"
      :aria-expanded="group.expanded"
      @click.stop="group.toggle"
    >
      <span class="grid-group-indicator" aria-hidden="true"></span>
      {{ group.label }}
      <span v-if="group.aggregate" class="grid-group-cell">{{
        group.aggregate
      }}</span>
    </button>
    <span v-else-if="group" class="grid-group-cell">{{
      group.aggregate ?? ''
    }}</span>
    <div
      v-else-if="cellState.message || cellState.draft"
      class="grid-cell-invalid"
      :aria-invalid="cellState.message ? 'true' : undefined"
    >
      <span v-if="cellState.draft && !choiceDraft" class="grid-cell-draft">{{
        cellState.draft.text
      }}</span>
      <FieldRenderer
        v-else
        :field="field"
        :value="choiceDraft ? choiceDraft.value : value"
        :env="env"
        :table-id="tableId"
        :identity="identity"
      />
      <span v-if="cellState.message" role="alert">{{ cellState.message }}</span>
      <span v-else-if="cellState.draft?.pending" role="status">{{
        savingLabel
      }}</span>
    </div>
    <FieldRenderer
      v-else-if="!group"
      :field="field"
      :value="value"
      :env="env"
      :table-id="tableId"
      :identity="identity"
    />
    <CellPresence
      v-if="!group"
      :table-id="tableId"
      :row-id="rowId"
      :field-id="fieldId"
      :identity="identity"
    />
  </div>
</template>

<style scoped>
.grid-highlight-cell {
  position: relative;
  min-height: 100%;
  padding-inline: 4px;
}
.grid-group-indicator {
  display: inline-block;
  width: 0;
  height: 0;
  margin-inline-end: 6px;
  border-block: 4px solid transparent;
  border-inline-start: 6px solid currentColor;
  transition: transform 100ms;
}
.grid-group-toggle[aria-expanded='true'] .grid-group-indicator {
  transform: rotate(90deg);
}
.grid-cell-draft {
  white-space: pre-wrap;
}
</style>
