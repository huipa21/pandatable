<script setup lang="ts">
import RelationLinks from './RelationLinks.vue'
import type { HighlightColor, RelationLink } from '../../api/types'
import CellPresence from '../presence/CellPresence.vue'

defineProps<{
  links: unknown
  editable: boolean
  disabled: () => boolean
  open: () => void
  remove: (link: RelationLink) => void
  tableId: string
  rowId: string
  fieldId: string
  identity: string
  rowHighlight?: HighlightColor
  cellHighlight?: HighlightColor
}>()
</script>

<template>
  <div
    class="grid-highlight-cell"
    :data-row-highlight="rowHighlight"
    :data-cell-highlight="cellHighlight"
  >
    <RelationLinks
      :links="links"
      :editable="editable"
      :disabled="disabled()"
      @edit="open"
      @remove="remove"
    />
    <CellPresence
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
</style>
