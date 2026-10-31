<script setup lang="ts">
import { computed, nextTick, ref, type PropType } from 'vue'
import { useI18n } from 'vue-i18n'
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
  fieldId: { type: String, required: true },
  identity: { type: String, required: true },
  changedBy: { type: String, default: '' },
})
const { t } = useI18n()
const field = computed(() => props.boundField)
const env = computed(() => props.fieldEnv)
const value = computed<WireValue>(() => {
  const normalized = field.value.normalize(props.val ?? null)
  return normalized.ok ? normalized.value : null
})
const resolveUser = (id: string) => userLabel(props.identity, props.tableId, id)
const preserveUploads = ref(false)

async function commit(next: WireValue) {
  if (field.value.presentation === 'attachment') {
    preserveUploads.value = true
    await nextTick()
  }
  props.save(next)
}

function cancel() {
  props.close()
}
</script>

<template>
  <span v-if="changedBy" class="remote-edit-indicator" role="status">
    {{ t('realtime.changedBy', { name: changedBy }) }}
  </span>
  <FieldEditor
    :field="field"
    :value="value"
    :env="env"
    :table-id="tableId"
    :field-id="fieldId"
    :preserve-uploads="preserveUploads"
    :identity="identity"
    :resolve-user="resolveUser"
    @commit="commit"
    @cancel="cancel"
  />
</template>

<style scoped>
.remote-edit-indicator {
  display: block;
  padding: 0.25rem 0.4rem;
  color: var(--pt-text, inherit);
  background: var(--pt-surface, Canvas);
  border: 1px solid currentColor;
  font-size: 0.75rem;
}
</style>
