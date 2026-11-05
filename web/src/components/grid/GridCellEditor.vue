<script setup lang="ts">
import { computed, nextTick, ref, type PropType } from 'vue'
import { useI18n } from 'vue-i18n'
import type { EditorType } from '@revolist/vue3-datagrid'
import FieldEditor from '../fields/FieldEditor.vue'
import type { BoundField } from '../../fields/registry'
import type { FieldEnv, Reason, WireValue } from '../../fields/types'
import { fieldErrorMessage } from '../../fields/messages'
import { userLabel } from '../../fields/userDirectory'
import { isGridScalar } from './inlineEditing'

const props = defineProps({
  column: { type: Object as PropType<EditorType['column']>, required: true },
  val: {
    type: [String, Number, Boolean, Array, Object] as PropType<WireValue>,
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
  initialDraft: { type: String, default: undefined },
  canCommit: { type: Function as PropType<() => boolean>, required: true },
  unavailableMessage: {
    type: Function as PropType<() => string>,
    required: true,
  },
  saveDraft: {
    type: Function as PropType<
      (text: string, pending: boolean, message?: string) => void
    >,
    required: true,
  },
  discardDraft: {
    type: Function as PropType<() => void>,
    required: true,
  },
})
const { t } = useI18n()
const root = ref<HTMLElement>()
const editor = ref<InstanceType<typeof FieldEditor>>()
const field = computed(() => props.boundField)
const env = computed(() => props.fieldEnv)
const value = computed<WireValue>(() => {
  const normalized = field.value.normalize(props.val ?? null)
  return normalized.ok ? normalized.value : null
})
const resolveUser = (id: string) => userLabel(props.identity, props.tableId, id)
const preserveUploads = ref(false)
const scalar = computed(() => isGridScalar(field.value))
const initialText =
  props.initialDraft ??
  (typeof props.val === 'string' && !field.value.normalize(props.val).ok
    ? props.val
    : field.value.formatCopy(value.value, env.value))
let finished = false
let leaving = false
let leaveAfterComposition = false

async function commit(next: WireValue) {
  if (scalar.value) {
    if (finished) return
    finished = true
    const text = editor.value?.currentDraft() ?? initialText
    if (!props.canCommit()) {
      props.saveDraft(text, false, props.unavailableMessage())
      props.close()
      return
    }
    props.saveDraft(text, true)
    props.save(next, leaving)
    if (leaving) props.close()
    return
  }
  if (field.value.presentation === 'attachment') {
    preserveUploads.value = true
    await nextTick()
  }
  props.save(next)
}

function cancel() {
  if (scalar.value) {
    finished = true
    props.discardDraft()
  }
  props.close()
}

function invalidDraft(text: string, reason: Reason) {
  if (leaving) props.saveDraft(text, false, fieldErrorMessage(reason))
}

function leave() {
  if (!scalar.value || finished || !editor.value) return true
  if (editor.value.isComposing()) {
    leaveAfterComposition = true
    return false
  }
  if (!props.canCommit()) {
    props.saveDraft(
      editor.value.currentDraft(),
      false,
      props.unavailableMessage(),
    )
    finished = true
    props.close()
    return true
  }
  leaving = true
  const valid = editor.value.commitDraft()
  leaving = false
  if (!valid) {
    finished = true
    props.close()
  }
  return true
}

function focusLeft(event: FocusEvent) {
  if (
    event.relatedTarget instanceof Node &&
    root.value?.contains(event.relatedTarget)
  )
    return
  leave()
}

function compositionFinished() {
  if (!leaveAfterComposition) return
  leaveAfterComposition = false
  queueMicrotask(leave)
}

function captureBeforeUnmount() {
  if (finished || !scalar.value || !editor.value) return
  const text = editor.value.currentDraft()
  if (text !== initialText || props.initialDraft !== undefined)
    props.saveDraft(text, false, t('errors.ERR_INTERNAL'))
  finished = true
}

defineExpose({
  leave,
  contains: (target: EventTarget | null) =>
    target instanceof Node && Boolean(root.value?.contains(target)),
  isComposing: () => editor.value?.isComposing() ?? false,
  captureBeforeUnmount,
})
</script>

<template>
  <div ref="root" @focusout="focusLeft">
    <span v-if="changedBy" class="remote-edit-indicator" role="status">
      {{ t('realtime.changedBy', { name: changedBy }) }}
    </span>
    <FieldEditor
      ref="editor"
      :field="field"
      :value="value"
      :env="env"
      :table-id="tableId"
      :field-id="fieldId"
      :preserve-uploads="preserveUploads"
      :identity="identity"
      :resolve-user="resolveUser"
      :grid-commit-on-leave="scalar"
      :initial-draft="scalar ? initialText : undefined"
      @commit="commit"
      @invalid-draft="invalidDraft"
      @composition-finished="compositionFinished"
      @cancel="cancel"
    />
  </div>
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
