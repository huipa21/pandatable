<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { DataRow } from '../../api/types'
import { isWireValue, type FieldEnv, type WireValue } from '../../fields/types'
import FieldRenderer from '../fields/FieldRenderer.vue'
import RelationLinks from '../relations/RelationLinks.vue'
import type { CardField } from './cardFields'

defineProps<{
  items: CardField[]
  row: DataRow
  env: FieldEnv
  tableId: string
  identity: string
}>()
const { t } = useI18n()

function wireValue(value: unknown): WireValue {
  if (!isWireValue(value)) throw new Error('Unsupported card field value')
  return value
}
</script>

<template>
  <div v-for="item in items" :key="item.field.id" class="card-field">
    <span class="card-label">{{ item.field.name }}</span>
    <RelationLinks
      v-if="
        item.field.uiType === 'link' && isWireValue(row.fields[item.field.id])
      "
      :links="row.fields[item.field.id]"
    />
    <FieldRenderer
      v-else-if="item.bound.ok && isWireValue(row.fields[item.field.id])"
      :field="item.bound.value"
      :value="wireValue(row.fields[item.field.id])"
      :env="env"
      :table-id="tableId"
      :identity="identity"
      :links="false"
    />
    <span v-else>{{ t('grid.unsupportedField') }}</span>
  </div>
</template>

<style scoped>
.card-field {
  display: grid;
  gap: 2px;
  min-width: 0;
}
.card-label {
  color: var(--muted);
}
</style>
