<script setup lang="ts">
const props = defineProps<{
  modelValue: string
  label: string
  placeholder: string
  clearLabel: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

function update(event: Event) {
  emit('update:modelValue', (event.currentTarget as HTMLInputElement).value)
}
</script>

<template>
  <div class="resource-search">
    <label class="resource-search-field">
      <span class="visually-hidden">{{ props.label }}</span>
      <input
        type="search"
        :value="props.modelValue"
        :placeholder="props.placeholder"
        @input="update"
      />
    </label>
    <button
      v-if="props.modelValue"
      class="resource-search-clear"
      type="button"
      :aria-label="props.clearLabel"
      @click="emit('update:modelValue', '')"
    >
      {{ props.clearLabel }}
    </button>
  </div>
</template>

<style scoped>
.resource-search {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  min-width: 0;
  margin: 0 0 16px;
}

.resource-search-field {
  flex: 1 1 0;
  min-width: 0;
}

.resource-search input {
  box-sizing: border-box;
  width: 100%;
  min-height: 42px;
  padding: 8px 12px;
  color: var(--text);
  background: var(--panel-surface);
  border: 1px solid var(--border);
  border-radius: 6px;
}

.resource-search input:focus-visible,
.resource-search-clear:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 2px;
}

.resource-search-clear {
  flex: 0 0 auto;
  min-height: 42px;
  white-space: nowrap;
}
</style>
