<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { TableField } from '../../api/types'
import AppIcon, { fieldIconName } from '../AppIcon.vue'

defineOptions({ name: 'GridColumnHeader' })

defineProps<{
  field: TableField
  isOpen: () => boolean
  open: (anchor: HTMLButtonElement) => void
}>()
const { t } = useI18n()
</script>

<template>
  <div class="grid-column-header">
    <AppIcon :name="fieldIconName(field.uiType)" :size="16" />
    <span class="grid-column-name">{{ field.name }}</span>
    <button
      type="button"
      class="grid-column-menu-trigger"
      :aria-label="t('gridPolish.gp12.menuFor', { name: field.name })"
      aria-haspopup="menu"
      :aria-expanded="isOpen()"
      @pointerdown.stop
      @mousedown.stop
      @touchstart.stop
      @dblclick.stop
      @keydown.stop
      @click.stop="open($event.currentTarget as HTMLButtonElement)"
    >
      <AppIcon name="chevron-down" :size="16" />
    </button>
  </div>
</template>

<style scoped>
.grid-column-header {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  min-width: 0;
  height: 100%;
}
.grid-column-header > .app-icon {
  flex: none;
  color: var(--muted);
}
.grid-column-name {
  overflow: hidden;
  flex: 1;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.grid-column-menu-trigger {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  padding: 0;
  opacity: 0;
  color: var(--text);
  background: transparent;
  border: 0;
  border-radius: 4px;
}
.grid-column-header:hover .grid-column-menu-trigger,
.grid-column-header:focus-within .grid-column-menu-trigger,
.grid-column-menu-trigger[aria-expanded='true'] {
  opacity: 1;
}
.grid-column-menu-trigger:hover,
.grid-column-menu-trigger:focus-visible {
  background: var(--surface);
}
@media (hover: none), (any-pointer: coarse) {
  .grid-column-menu-trigger {
    opacity: 1;
  }
}
</style>
