<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRealtimeStore } from '../../stores/realtime'
import { presenceColor } from './colors'

const props = defineProps<{
  tableId: string
  rowId: string
  fieldId: string
  identity: string
}>()
const realtime = useRealtimeStore()
const { t } = useI18n()
const collaborators = computed(() =>
  (realtime.presence[props.tableId] ?? []).filter(
    (user) =>
      user.userId !== props.identity &&
      user.cell?.rowId === props.rowId &&
      user.cell.fieldId === props.fieldId,
  ),
)
</script>

<template>
  <span
    v-if="rowId && collaborators.length"
    class="cell-presence"
    :data-presence-row="rowId"
    :data-presence-field="fieldId"
    :style="{ borderColor: presenceColor(collaborators[0]!.userId) }"
  >
    <span
      v-for="user in collaborators"
      :key="user.userId"
      class="cell-presence-name"
      :data-presence-user="user.userId"
      :aria-label="t('presence.focusedCell', { name: user.name })"
      :title="t('presence.focusedCell', { name: user.name })"
      :style="{ backgroundColor: presenceColor(user.userId) }"
    >
      {{ user.name }}
    </span>
  </span>
</template>

<style scoped>
.cell-presence {
  position: absolute;
  z-index: 3;
  inset: 0;
  display: flex;
  align-items: flex-start;
  justify-content: flex-end;
  overflow: hidden;
  box-sizing: border-box;
  border: 2px solid;
  pointer-events: none;
}
.cell-presence-name {
  overflow: hidden;
  max-width: 100%;
  padding: 1px 3px;
  color: #fff;
  font-size: 10px;
  line-height: 1.2;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
