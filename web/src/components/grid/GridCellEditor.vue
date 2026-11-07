<script setup lang="ts">
import { computed, nextTick, ref, type PropType } from 'vue'
import { useI18n } from 'vue-i18n'
import type { EditorType } from '@revolist/vue3-datagrid'
import FieldEditor from '../fields/FieldEditor.vue'
import type { BoundField } from '../../fields/registry'
import type { FieldEnv, Reason, WireValue } from '../../fields/types'
import { fieldErrorMessage } from '../../fields/messages'
import { userLabel } from '../../fields/userDirectory'
import { choiceLabels } from '../../fields/choice'
import GridChoiceEditor from './GridChoiceEditor.vue'
import { isGridChoice, isGridScalar } from './inlineEditing'

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
const choice = isGridChoice(field.value)
const choiceEditor = ref<InstanceType<typeof GridChoiceEditor>>()
const choiceError = ref('')
// Typing on a focused cell opens RevoGrid's editor with the key as `val`; the
// row model holds the stored labels, including choices no longer configured.
const model = props.column.model as Record<string, unknown> | undefined
const storedChoices = choiceLabels(
  (model && Object.hasOwn(model, props.fieldId)
    ? model[props.fieldId]
    : props.val) as WireValue,
)
const recoveredChoices = (() => {
  if (!choice || props.initialDraft === undefined) return undefined
  const parsed = field.value.parsePaste(props.initialDraft, env.value)
  return parsed.ok ? choiceLabels(parsed.value) : undefined
})()
const choiceListLabel = (() => {
  const name = (props.column.column as { name?: unknown } | undefined)?.name
  return typeof name === 'string' && name ? name : t('field.editor.value')
})()
const initialText =
  props.initialDraft ??
  (typeof props.val === 'string' && !field.value.normalize(props.val).ok
    ? props.val
    : field.value.formatCopy(value.value, env.value))
let finished = false
let leaving = false
let leaveAfterComposition = false
let committedText: string | undefined
let baselineText = initialText

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
    committedText = text
    baselineText = text
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
  if (scalar.value || choice) {
    finished = true
    committedText = undefined
    props.discardDraft()
  }
  const grid = gridHost()
  props.close()
  if (choice) restoreGridFocus(grid)
}

function gridHost() {
  return root.value?.closest<HTMLElement>('revo-grid') ?? null
}

/** Keeps keyboard navigation on the grid after the focused list unmounts. */
function restoreGridFocus(grid: HTMLElement | null) {
  queueMicrotask(() => {
    if (
      grid?.isConnected &&
      (!document.activeElement ||
        document.activeElement === document.body ||
        !document.activeElement.isConnected)
    )
      grid.focus({ preventScroll: true })
  })
}

const sameChoices = (left: string[], right: string[]) =>
  left.length === right.length && left.every((label) => right.includes(label))

/**
 * Commits a choice selection through the shared cell pipeline. `leave` is the
 * editor boundary (outside click/Tab/unmount), `pointer` an option click and
 * `enter` a keyboard confirmation, which moves focus like scalar Enter.
 */
function commitChoice(labels: string[], mode: 'leave' | 'pointer' | 'enter') {
  if (finished) return true
  const grid = gridHost()
  const done = () => {
    finished = true
    if (mode !== 'enter') props.close()
    restoreGridFocus(grid)
  }
  if (sameChoices(labels, storedChoices)) {
    if (props.initialDraft !== undefined) props.discardDraft()
    finished = true
    props.close()
    restoreGridFocus(grid)
    return true
  }
  const text = labels.join(', ')
  if (!props.canCommit()) {
    props.saveDraft(text, false, props.unavailableMessage())
    finished = true
    props.close()
    restoreGridFocus(grid)
    return true
  }
  const multiple = field.value.kind === 'multi_select'
  const normalized = field.value.normalize(
    multiple ? labels : (labels[0] ?? ''),
  )
  if (!normalized.ok) {
    const message = fieldErrorMessage(normalized.reason)
    if (mode !== 'leave') {
      choiceError.value = message
      return false
    }
    props.saveDraft(text, false, message)
    done()
    return true
  }
  props.saveDraft(text, true)
  if (mode === 'enter') {
    finished = true
    props.save(normalized.value, false)
    restoreGridFocus(grid)
    return true
  }
  props.save(normalized.value, true)
  done()
  return true
}

function choicePicked(labels: string[], keyboard: boolean) {
  commitChoice(labels, keyboard ? 'enter' : 'pointer')
}

function choiceConfirmed() {
  if (choiceEditor.value) commitChoice(choiceEditor.value.selection(), 'enter')
}

function invalidDraft(text: string, reason: Reason) {
  if (leaving) props.saveDraft(text, false, fieldErrorMessage(reason))
}

function leave() {
  if (choice) {
    if (!finished && choiceEditor.value)
      commitChoice(choiceEditor.value.selection(), 'leave')
    return true
  }
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

function draftChanged() {
  // RevoGrid can retain an editor after overlapping requests to open a cell.
  if (
    scalar.value &&
    finished &&
    committedText !== undefined &&
    editor.value?.currentDraft() !== committedText
  ) {
    finished = false
    committedText = undefined
  }
}

function captureBeforeUnmount() {
  if (choice && !finished && choiceEditor.value) {
    const labels = choiceEditor.value.selection()
    if (!sameChoices(labels, storedChoices))
      props.saveDraft(labels.join(', '), false, t('errors.ERR_INTERNAL'))
    finished = true
    return
  }
  if (finished || !scalar.value || !editor.value) return
  const text = editor.value.currentDraft()
  if (text !== baselineText || props.initialDraft !== undefined)
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
  <div ref="root" @focusout="focusLeft" @input="draftChanged">
    <span v-if="changedBy" class="remote-edit-indicator" role="status">
      {{ t('realtime.changedBy', { name: changedBy }) }}
    </span>
    <GridChoiceEditor
      v-if="choice"
      ref="choiceEditor"
      :field="field"
      :env="env"
      :original="storedChoices"
      :initial="recoveredChoices ?? storedChoices"
      :label="choiceListLabel"
      :error="choiceError"
      @pick="choicePicked"
      @commit="choiceConfirmed"
      @cancel="cancel"
    />
    <FieldEditor
      v-else
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
