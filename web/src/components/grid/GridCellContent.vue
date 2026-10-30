<script setup lang="ts">
import type { HighlightColor } from '../../api/types'
import type { BoundField } from '../../fields/registry'
import type { FieldEnv, WireValue } from '../../fields/types'
import FieldRenderer from '../fields/FieldRenderer.vue'

defineProps<{
  field: BoundField
  value: WireValue
  env: FieldEnv
  message: string
  tableId: string
  identity: string
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
    <div v-else-if="message" class="grid-cell-invalid" aria-invalid="true">
      <FieldRenderer
        :field="field"
        :value="value"
        :env="env"
        :table-id="tableId"
        :identity="identity"
      />
      <span role="alert">{{ message }}</span>
    </div>
    <FieldRenderer
      v-else-if="!group"
      :field="field"
      :value="value"
      :env="env"
      :table-id="tableId"
      :identity="identity"
    />
  </div>
</template>

<style scoped>
.grid-highlight-cell {
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
</style>
