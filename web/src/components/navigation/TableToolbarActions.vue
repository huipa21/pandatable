<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { TableSchema, TableView } from '../../api/types'
import PresenceAvatars from '../presence/PresenceAvatars.vue'
import AppIcon from '../AppIcon.vue'

const props = defineProps<{
  schema: TableSchema
  view?: TableView
  identity: string
  csvReady: boolean
  exportBusy: boolean
  exportError: string
}>()
const emit = defineEmits<{
  import: []
  export: [format: 'csv' | 'xlsx']
}>()
const { t } = useI18n()
const menu = ref<HTMLDetailsElement>()
const trigger = ref<HTMLElement>()
const canImport = computed(
  () => props.schema.myLevel === 'write' || props.schema.myLevel === 'owner',
)

function close(focus = false) {
  if (!menu.value?.open) return
  menu.value.open = false
  if (focus) trigger.value?.focus()
}

function importTable() {
  close(true)
  emit('import')
}

function exportTable(format: 'csv' | 'xlsx') {
  close(true)
  emit('export', format)
}

function dismiss(event: PointerEvent) {
  const target = event.target
  if (target instanceof Node && !menu.value?.contains(target)) close()
}

function keydown(event: KeyboardEvent) {
  if (event.key !== 'Escape' || !menu.value?.open) return
  event.preventDefault()
  event.stopPropagation()
  close(true)
}

watch(
  [() => props.schema.table.id, () => props.view?.id, () => props.identity],
  () => close(),
)
onMounted(() => document.addEventListener('pointerdown', dismiss))
onBeforeUnmount(() => document.removeEventListener('pointerdown', dismiss))
</script>

<template>
  <div class="table-toolbar-actions">
    <details
      v-if="view || canImport"
      ref="menu"
      class="view-export transfer-menu"
      @keydown="keydown"
    >
      <summary
        ref="trigger"
        :aria-label="t('transfer.title')"
        :title="t('transfer.title')"
      >
        <AppIcon name="duplicate" :size="16" />
        <span class="visually-hidden">{{ t('transfer.title') }}</span>
      </summary>
      <div class="view-export-panel">
        <button v-if="canImport" type="button" @click="importTable">
          {{ t('import.append') }}
        </button>
        <button
          v-if="view?.type === 'grid'"
          type="button"
          :disabled="exportBusy || !csvReady"
          @click="exportTable('csv')"
        >
          {{ t('export.csv') }}
        </button>
        <button
          v-if="view"
          type="button"
          :disabled="exportBusy"
          @click="exportTable('xlsx')"
        >
          {{ t('export.package') }}
        </button>
        <p v-if="view">{{ t('export.packageNotice') }}</p>
      </div>
    </details>
    <PresenceAvatars :table-id="schema.table.id" :identity="identity" />
    <span v-if="exportBusy" class="table-export-feedback" role="status">
      {{ t('export.preparing') }}
    </span>
    <span v-else-if="exportError" class="table-export-feedback" role="alert">
      {{ exportError }}
    </span>
  </div>
</template>

<style scoped>
.table-toolbar-actions {
  display: flex;
  flex: 1 1 auto;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
  min-width: 0;
  max-width: 100%;
}

.view-export > summary {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  min-height: 40px;
  padding: 8px;
  color: var(--text);
  background: transparent;
  border: 1px solid transparent;
  border-radius: 6px;
  cursor: pointer;
  list-style: none;
}

.view-export > summary::-webkit-details-marker {
  display: none;
}

.view-export[open] > summary,
.view-export > summary:hover {
  background: color-mix(in srgb, var(--accent) 10%, transparent);
}

.view-export-panel {
  position: absolute;
  z-index: 15;
  top: 100%;
  right: 0;
  display: grid;
  gap: 8px;
  width: min(320px, 100%);
  max-height: 65vh;
  padding: 12px;
  overflow: auto;
  color: var(--text);
  background: var(--panel-surface);
  border: 1px solid var(--border);
  border-radius: 8px;
  box-shadow: 0 8px 24px rgb(0 0 0 / 14%);
}

.view-export-panel button {
  min-height: 40px;
  text-align: start;
}

.view-export-panel p {
  margin: 0;
}

.table-export-feedback {
  max-width: 100%;
  overflow-wrap: anywhere;
}
</style>
