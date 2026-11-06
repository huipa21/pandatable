<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { TableSchema, TableView } from '../../api/types'
import { accessLevelLabel } from '../../i18n'
import PresenceAvatars from '../presence/PresenceAvatars.vue'

const props = defineProps<{
  schema: TableSchema
  view?: TableView
  identity: string
  csvReady: boolean
  exportBusy: boolean
  exportError: string
  fillSummary?: boolean
}>()
const emit = defineEmits<{
  import: []
  export: [format: 'csv' | 'xlsx']
}>()
const { t } = useI18n()
const menu = ref<HTMLDetailsElement>()
const trigger = ref<HTMLElement>()
const displayField = computed(() =>
  props.schema.fields.find(
    (field) => field.id === props.schema.table.displayFieldId,
  ),
)

function close(focus = false) {
  if (!menu.value?.open) return
  menu.value.open = false
  if (focus) trigger.value?.focus()
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
    <dl
      class="schema-summary table-info"
      :class="{ 'table-info-fill': fillSummary }"
    >
      <div
        class="table-info-field"
        :title="`${t('nav.displayField')}: ${displayField?.name ?? t('nav.noDisplayField')}`"
      >
        <dt>{{ t('nav.displayField') }}</dt>
        <dd :title="displayField?.name ?? t('nav.noDisplayField')">
          {{ displayField?.name ?? t('nav.noDisplayField') }}
        </dd>
      </div>
      <div
        class="table-info-access"
        :title="`${t('nav.access')}: ${accessLevelLabel(schema.myLevel)}`"
      >
        <dt>{{ t('nav.access') }}</dt>
        <dd>{{ accessLevelLabel(schema.myLevel) }}</dd>
      </div>
    </dl>
    <button
      v-if="schema.myLevel === 'write' || schema.myLevel === 'owner'"
      type="button"
      :aria-label="t('import.append')"
      :title="t('import.append')"
      @click="emit('import')"
    >
      {{ t('import.title') }}
    </button>
    <details v-if="view" ref="menu" class="view-export" @keydown="keydown">
      <summary ref="trigger">{{ t('export.title') }}</summary>
      <div class="view-export-panel">
        <button
          v-if="view.type === 'grid'"
          type="button"
          :disabled="exportBusy || !csvReady"
          @click="emit('export', 'csv')"
        >
          {{ t('export.csv') }}
        </button>
        <button
          type="button"
          :disabled="exportBusy"
          @click="emit('export', 'xlsx')"
        >
          {{ t('export.package') }}
        </button>
        <p>{{ t('export.packageNotice') }}</p>
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

.table-info {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 6px;
  min-width: 0;
  max-width: 100%;
  margin: 0 2px;
  color: var(--muted);
  font-size: 0.75rem;
}

/* Zero intrinsic width: in the Grid toolbar the summary fills only the space
   left by the actions, so it never pushes them onto a second row (D-229). */
.table-info-fill {
  flex: 1 1 0;
  justify-content: flex-end;
  row-gap: 0;
  width: 0;
  overflow: hidden;
}

.table-info > div {
  display: flex;
  gap: 4px;
  min-width: 0;
}

.table-info-field {
  max-width: 140px;
}

.table-info dt {
  flex-shrink: 0;
}

.table-info-fill dt {
  flex-shrink: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.table-info dt::after {
  content: ':';
}

.table-info dd {
  min-width: 0;
  margin: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.table-toolbar-actions > button,
.view-export > summary {
  display: inline-flex;
  align-items: center;
  min-height: 40px;
  padding: 8px 6px;
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

.table-toolbar-actions > button:hover,
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
