<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { RelationLink } from '../../api/types'
import AppIcon from '../AppIcon.vue'
import { presenceColor } from '../presence/colors'

const props = defineProps<{
  links: unknown
  editable?: boolean
  disabled?: boolean
}>()
const emit = defineEmits<{ edit: []; remove: [link: RelationLink] }>()
const { t } = useI18n()
const values = computed(() =>
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
    : [],
)
</script>

<template>
  <span class="relation-links">
    <span v-if="values.length" class="relation-items">
      <span
        v-for="link in values"
        :key="link.id"
        class="relation-chip"
        :style="{ '--relation-color': presenceColor(link.id) }"
        :title="link.label"
      >
        <span class="relation-label">{{ link.label }}</span>
        <button
          v-if="editable"
          type="button"
          class="relation-remove"
          :aria-label="t('relation.unlinkRecord', { label: link.label })"
          :title="t('relation.unlink')"
          :disabled="disabled"
          @click.stop="emit('remove', link)"
        >
          <AppIcon name="close" :size="14" />
        </button>
      </span>
    </span>
    <span v-else class="relation-empty">
      {{ t('relation.empty') }}
    </span>
    <button
      v-if="editable"
      type="button"
      class="relation-edit"
      :aria-label="t('relation.edit')"
      :disabled="disabled"
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
  min-width: 0;
}
.relation-items {
  display: inline-flex;
  flex: 0 1 auto;
  align-items: center;
  gap: 4px;
  max-width: 100%;
  min-width: 0;
  overflow-x: auto;
  scrollbar-width: thin;
}
.relation-chip {
  display: inline-flex;
  flex: 0 1 auto;
  align-items: center;
  gap: 2px;
  max-width: 160px;
  min-width: 32px;
  padding: 0 4px 0 8px;
  border-radius: 10px;
  background: color-mix(in srgb, var(--relation-color) 28%, transparent);
  color: var(--text);
  white-space: nowrap;
}
.relation-label {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.relation-remove {
  display: inline-flex;
  flex: 0 0 20px;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  padding: 0;
  border: 0;
  border-radius: 50%;
  background: transparent;
  color: inherit;
  cursor: pointer;
}
.relation-remove:hover:not(:disabled),
.relation-remove:focus-visible {
  background: color-mix(in srgb, currentColor 14%, transparent);
}
.relation-remove:focus-visible,
.relation-edit:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: -2px;
}
.relation-empty {
  min-width: 0;
  overflow: hidden;
  color: var(--muted);
  text-overflow: ellipsis;
  white-space: nowrap;
}
.relation-edit {
  flex: 0 0 auto;
  padding: 0 6px;
  border: 0;
  background: transparent;
  color: var(--accent);
  cursor: pointer;
}
.relation-remove:disabled,
.relation-edit:disabled {
  opacity: 0.5;
  cursor: default;
}
</style>
