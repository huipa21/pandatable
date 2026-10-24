<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { EditorType } from '@revolist/vue3-datagrid'

const props = defineProps<EditorType>()
const { t } = useI18n()
const input = ref<HTMLTextAreaElement>()
const value = ref(String(props.val ?? props.model?.[props.column.prop] ?? ''))
onMounted(() => input.value?.focus())

function keydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    event.preventDefault()
    event.stopPropagation()
    props.close()
  } else if (event.key === 'Enter' && !event.shiftKey) {
    event.preventDefault()
    event.stopPropagation()
    props.save(value.value)
  }
}
</script>

<template>
  <textarea
    ref="input"
    v-model="value"
    class="spike-note-editor"
    :aria-label="t('spike.noteEditor')"
    :placeholder="t('spike.noteHint')"
    maxlength="10000"
    @keydown="keydown"
  />
</template>

<style scoped>
.spike-note-editor {
  width: 100%;
  min-height: 90px;
  padding: 8px;
  background: var(--surface);
  color: var(--text);
  border: 2px solid var(--accent);
}
</style>
