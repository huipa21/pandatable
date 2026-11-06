<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useUndoStore } from '../../stores/undo'
import AppIcon from '../AppIcon.vue'

const { t } = useI18n()
const undo = useUndoStore()
</script>

<template>
  <div class="table-history">
    <button
      class="icon-button"
      :aria-label="t('undo.undo')"
      :title="t('undo.undo')"
      :disabled="!undo.canUndo"
      aria-keyshortcuts="Control+Z Meta+Z"
      @click="undo.undo"
    >
      <AppIcon name="undo" />
    </button>
    <button
      class="icon-button"
      :aria-label="t('undo.redo')"
      :title="t('undo.redo')"
      :disabled="!undo.canRedo"
      aria-keyshortcuts="Control+Y Meta+Shift+Z"
      @click="undo.redo"
    >
      <AppIcon name="redo" />
    </button>
  </div>
</template>

<style scoped>
.table-history {
  display: flex;
  align-items: center;
  flex-shrink: 0;
  gap: 2px;
}
</style>
