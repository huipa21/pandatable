<script setup lang="ts">
import RelationLinks from './RelationLinks.vue'
import type { HighlightColor } from '../../api/types'

defineProps<{
  links: unknown
  editable: boolean
  open: () => void
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
    <RelationLinks :links="links" :editable="editable" @edit="open" />
  </div>
</template>

<style scoped>
.grid-highlight-cell {
  min-height: 100%;
  padding-inline: 4px;
}
</style>
