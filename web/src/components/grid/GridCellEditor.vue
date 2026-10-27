<script setup lang="ts">
import { computed } from 'vue'
import type { EditorType } from '@revolist/vue3-datagrid'
import FieldEditor from '../fields/FieldEditor.vue'
import type { BoundField } from '../../fields/registry'
import type { FieldEnv, WireValue } from '../../fields/types'

type GridColumn = EditorType['column'] & {
  boundField: BoundField
  fieldEnv: FieldEnv
}
type GridEditorProps = Omit<EditorType, 'column'> & { column: GridColumn }

const props = defineProps<GridEditorProps>()
const field = computed(() => props.column.boundField)
const env = computed(() => props.column.fieldEnv)
const value = computed<WireValue>(() => {
  const normalized = field.value.normalize(props.val ?? null)
  return normalized.ok ? normalized.value : null
})

function commit(next: WireValue) {
  props.save(next)
}

function cancel() {
  props.close()
}
</script>

<template>
  <FieldEditor
    :field="field"
    :value="value"
    :env="env"
    @commit="commit"
    @cancel="cancel"
  />
</template>
