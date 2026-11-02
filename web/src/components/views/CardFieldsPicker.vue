<script setup lang="ts">
import type { TableField } from '../../api/types'

const props = defineProps<{
  fields: TableField[]
  selected: string[]
  legend: string
  busy?: boolean
}>()
const emit = defineEmits<{ change: [fields: string[]] }>()

function toggle(id: string, checked: boolean) {
  const remaining = props.selected.filter((item) => item !== id)
  emit('change', checked ? [...remaining, id] : remaining)
}
</script>

<template>
  <fieldset :disabled="busy">
    <legend>{{ legend }}</legend>
    <label v-for="field in fields" :key="field.id" class="field-choice">
      <input
        type="checkbox"
        :checked="selected.includes(field.id)"
        @change="toggle(field.id, ($event.target as HTMLInputElement).checked)"
      />
      {{ field.name }}
    </label>
  </fieldset>
</template>

<style scoped>
fieldset {
  display: grid;
  gap: 12px;
  min-width: 0;
  padding: 12px;
  border: 1px solid var(--border);
}
.field-choice {
  display: flex;
  align-items: center;
  gap: 8px;
}
</style>
