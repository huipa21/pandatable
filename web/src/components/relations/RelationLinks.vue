<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { RelationLink } from '../../api/types'

const props = defineProps<{
  links: unknown
  editable?: boolean
}>()
const emit = defineEmits<{ edit: [] }>()
const { t } = useI18n()
const values = () =>
  Array.isArray(props.links)
    ? props.links.filter(
        (value): value is RelationLink =>
          typeof value === 'object' &&
          value !== null &&
          'id' in value &&
          typeof value.id === 'string' &&
          'label' in value &&
          typeof value.label === 'string',
      )
    : []
</script>

<template>
  <span class="relation-links">
    <span v-for="link in values()" :key="link.id" class="relation-chip">
      {{ link.label }}
    </span>
    <span v-if="values().length === 0" class="relation-empty">
      {{ t('relation.empty') }}
    </span>
    <button
      v-if="editable"
      type="button"
      class="relation-edit"
      :aria-label="t('relation.edit')"
      @click.stop="emit('edit')"
    >
      {{ t('relation.edit') }}
    </button>
  </span>
</template>

<style scoped>
.relation-links {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  max-width: 100%;
  overflow: hidden;
}
.relation-chip {
  max-width: 160px;
  padding: 0 8px;
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.relation-empty {
  color: var(--muted);
}
.relation-edit {
  flex: 0 0 auto;
  padding: 0 6px;
  border: 0;
  background: transparent;
  color: var(--accent);
  cursor: pointer;
}
</style>
