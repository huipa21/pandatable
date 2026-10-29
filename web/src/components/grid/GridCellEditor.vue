<script setup lang="ts">
import { computed, type PropType } from 'vue'
import type { EditorType } from '@revolist/vue3-datagrid'
import FieldEditor from '../fields/FieldEditor.vue'
import type { BoundField } from '../../fields/registry'
import type { FieldEnv, WireValue } from '../../fields/types'
import { userLabel } from '../../fields/userDirectory'

const props = defineProps({
  column: { type: Object as PropType<EditorType['column']>, required: true },
  val: {
    type: [String, Number, Boolean, Array] as PropType<WireValue>,
    default: null,
  },
  save: { type: Function as PropType<EditorType['save']>, required: true },
  close: { type: Function as PropType<EditorType['close']>, required: true },
  boundField: { type: Object as PropType<BoundField>, required: true },
  fieldEnv: { type: Object as PropType<FieldEnv>, required: true },
  tableId: { type: String, required: true },
  identity: { type: String, required: true },
})
const field = computed(() => props.boundField)
const env = computed(() => props.fieldEnv)
const value = computed<WireValue>(() => {
  const normalized = field.value.normalize(props.val ?? null)
  return normalized.ok ? normalized.value : null
})
const resolveUser = (id: string) => userLabel(props.identity, props.tableId, id)

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
    :table-id="tableId"
    :identity="identity"
    :resolve-user="resolveUser"
    @commit="commit"
    @cancel="cancel"
  />
</template>
