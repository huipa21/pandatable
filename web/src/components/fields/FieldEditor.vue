<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { BoundField } from '../../fields/registry'
import type { ChoiceOptions } from '../../fields/choice'
import {
  discardAttachment,
  errorMessage,
  uploadAttachment,
} from '../../api/client'
import type { AttachmentValue, DirectoryPrincipal } from '../../api/types'
import type { AttachmentOptions } from '../../fields/attachment'
import {
  queueUserLabels,
  searchTableUsers,
  userLabelDeleted,
} from '../../fields/userDirectory'
import { fieldErrorMessage } from '../../fields/messages'
import type { FieldEnv, Reason, Result, WireValue } from '../../fields/types'
import type { CheckboxOptions } from '../../fields/number'
import AppIcon from '../AppIcon.vue'
import CheckboxIcon from './CheckboxIcon.vue'
import LongTextDialog from './LongTextDialog.vue'
import TemporalEditor from './TemporalEditor.vue'

const props = withDefaults(
  defineProps<{
    field: BoundField
    value: WireValue
    env?: FieldEnv
    label?: string
    required?: boolean
    autoFocus?: boolean
    live?: boolean
    submitLabel?: string
    tableId?: string
    fieldId?: string
    preserveUploads?: boolean
    identity?: string
    resolveUser?: (id: string) => string | null
    gridCommitOnLeave?: boolean
    initialDraft?: string
    defaultPlaceholder?: string
  }>(),
  {
    env: () => ({}),
    label: undefined,
    required: false,
    autoFocus: true,
    live: false,
    submitLabel: undefined,
    tableId: undefined,
    fieldId: undefined,
    preserveUploads: false,
    identity: undefined,
    resolveUser: undefined,
    gridCommitOnLeave: false,
    initialDraft: undefined,
    defaultPlaceholder: undefined,
  },
)
const emit = defineEmits<{
  commit: [value: WireValue, retainFocus?: boolean]
  preview: [value: WireValue]
  invalidValue: []
  invalidDraft: [text: string, reason: Reason, message?: string]
  compositionFinished: []
  cancel: []
  focusout: [event: FocusEvent]
  draftChanged: []
}>()
const { t } = useI18n()

const input = ref<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>()
const checkboxButton = ref<HTMLButtonElement>()
const checkboxChecked = ref(props.value === true)
const temporalEditor = ref<InstanceType<typeof TemporalEditor>>()
const longTextDialog = ref<InstanceType<typeof LongTextDialog>>()
const longTextOpen = ref(false)
let longTextSelection: {
  start: number
  end: number
  direction: 'forward' | 'backward' | 'none'
} | null = null
const temporal = computed(() =>
  ['date', 'datetime', 'time'].includes(props.field.kind),
)
const attachmentInput = ref<HTMLInputElement>()
const reason = ref<Reason | null>(null)
const requiredError = ref(false)
const draft = ref(
  props.initialDraft ??
    (props.field.readOnly
      ? ''
      : props.field.formatCopy(props.value, props.env)),
)
const composing = ref(false)
const selected = ref<string[]>(
  typeof props.value === 'string'
    ? [props.value]
    : Array.isArray(props.value)
      ? props.value.filter((item): item is string => typeof item === 'string')
      : [],
)
const single = ref(typeof props.value === 'string' ? props.value : '')
const selectedUsers = ref<string[]>(
  typeof props.value === 'string'
    ? [props.value]
    : Array.isArray(props.value)
      ? props.value.filter((item): item is string => typeof item === 'string')
      : [],
)
const attachmentFiles = ref<AttachmentValue[]>(
  Array.isArray(props.value)
    ? (props.value as unknown as AttachmentValue[]).map((file) => ({ ...file }))
    : [],
)
const attachmentError = ref('')
const uploadingAttachments = ref(false)
const attachmentPending = ref(false)
const dragOver = ref(false)
const stagedAttachmentIds = new Set<string>()
const removedAttachmentIds = new Set<string>()
const userQuery = ref('')
const userResults = ref<DirectoryPrincipal[]>([])
const userSearching = ref(false)
const userSearchFailed = ref(false)
const userPending = ref(false)
let unmounted = false
let userSearchGeneration = 0
let userSearchController: AbortController | undefined
const choices = computed(
  () => (props.field.options as ChoiceOptions).choices ?? [],
)
const multiple = computed(() => props.field.kind === 'multi_select')
const multipleUsers = computed(() => props.field.kind === 'multi_user')
const attachmentOptions = computed(
  () => props.field.options as AttachmentOptions,
)
const availableAttachmentSlots = computed(() =>
  Math.max(0, attachmentOptions.value.maxFiles - attachmentFiles.value.length),
)
const acceptAttribute = computed(() =>
  attachmentOptions.value.accept
    ? attachmentOptions.value.accept.replaceAll(', ', ',')
    : undefined,
)
const showSubmitButton = computed(() =>
  Boolean(
    !props.live &&
    (props.submitLabel ||
      (props.field.presentation === 'select' && multiple.value) ||
      props.field.presentation === 'attachment') &&
    !(props.field.presentation === 'user' && multipleUsers.value),
  ),
)
const placeholder = computed(() => {
  if (props.defaultPlaceholder !== undefined) return props.defaultPlaceholder
  switch (props.field.presentation) {
    case 'date':
      return t('field.editor.datePlaceholder')
    case 'datetime':
      return t('field.editor.datetimePlaceholder')
    default:
      return undefined
  }
})
const error = computed(() =>
  requiredError.value
    ? t('errors.ERR_REQUIRED_FIELD')
    : reason.value
      ? fieldErrorMessage(reason.value)
      : '',
)

onMounted(() => {
  if (props.autoFocus) (input.value ?? checkboxButton.value)?.focus()
})
watch(
  () => props.value,
  (value) => {
    if (props.field.presentation === 'checkbox')
      checkboxChecked.value = value === true
  },
)
async function discardTemporaryAttachments() {
  if (!props.tableId || !props.fieldId) return
  const ids = props.preserveUploads
    ? removedAttachmentIds
    : new Set([...stagedAttachmentIds, ...removedAttachmentIds])
  for (const id of ids)
    await discardTemporaryAttachment(props.tableId, props.fieldId, id)
}

async function discardTemporaryAttachment(
  tableId: string,
  fieldId: string,
  id: string,
) {
  try {
    await discardAttachment(tableId, fieldId, id)
  } catch (cause) {
    console.error('Failed to discard a temporary attachment', cause)
  }
}

onBeforeUnmount(() => {
  unmounted = true
  userSearchController?.abort()
  void discardTemporaryAttachments()
})
watch(
  [() => props.value, () => props.identity, () => props.tableId],
  ([value]) => {
    if (props.field.presentation !== 'user') return
    const ids =
      typeof value === 'string'
        ? [value]
        : Array.isArray(value)
          ? value.filter((item): item is string => typeof item === 'string')
          : []
    queueUserLabels(props.identity, props.tableId, ids)
  },
  { immediate: true, deep: true },
)

function finish(result: Result<WireValue>) {
  requiredError.value = result.ok && props.required && result.value === null
  if (requiredError.value) {
    if (props.live) emit('invalidValue')
    return false
  }
  if (!result.ok) {
    reason.value = result.reason
    if (props.live) emit('invalidValue')
    if (props.gridCommitOnLeave)
      emit('invalidDraft', currentDraft(), result.reason)
    return false
  }
  reason.value = null
  emit('commit', result.value)
  return true
}

function temporalCommit(value: WireValue, retainFocus?: boolean) {
  if (retainFocus) emit('commit', value, true)
  else emit('commit', value)
}

function currentDraft() {
  if (temporal.value && temporalEditor.value)
    return temporalEditor.value.currentDraft()
  const element = input.value
  return element instanceof HTMLInputElement ||
    element instanceof HTMLTextAreaElement
    ? element.value
    : draft.value
}

function compositionEnded() {
  composing.value = false
  emit('compositionFinished')
}

function commitText() {
  if (temporal.value && temporalEditor.value)
    return temporalEditor.value.commitDraft()
  draft.value = currentDraft()
  return finish(props.field.parsePaste(draft.value, props.env))
}

function hasPendingDraft() {
  if (temporal.value) return temporalEditor.value?.hasPendingDraft() ?? false
  if (props.field.presentation === 'user') return userPending.value
  if (props.field.presentation === 'attachment')
    return (
      attachmentPending.value ||
      uploadingAttachments.value ||
      attachmentError.value !== ''
    )
  return false
}

function commitDraft() {
  if (props.field.presentation === 'user') return commitUsers()
  if (props.field.presentation === 'attachment') return commitAttachments()
  if (props.field.presentation === 'select') return commitSelect()
  if (props.field.presentation === 'checkbox')
    return finish(props.field.normalize(checkboxChecked.value))
  return commitText()
}

function contains(target: EventTarget | null) {
  return (
    (temporalEditor.value?.contains(target) ?? false) ||
    (target instanceof Node &&
      (longTextDialog.value?.contains(target) ?? false))
  )
}

function isPointerInPanel() {
  return (
    (temporalEditor.value?.isPointerInPanel() ?? false) ||
    (longTextDialog.value?.contains(document.activeElement) ?? false)
  )
}

defineExpose({
  commitDraft,
  hasPendingDraft,
  currentDraft,
  isComposing: () => temporalEditor.value?.isComposing() ?? composing.value,
  contains,
  isPointerInPanel,
})

function openLongTextDialog() {
  if (!(input.value instanceof HTMLTextAreaElement)) return
  longTextSelection = {
    start: input.value.selectionStart,
    end: input.value.selectionEnd,
    direction: input.value.selectionDirection,
  }
  longTextOpen.value = true
}

function restoreLongTextFocus() {
  longTextOpen.value = false
  if (!(input.value instanceof HTMLTextAreaElement)) return
  input.value.focus({ preventScroll: true })
  if (longTextSelection)
    input.value.setSelectionRange(
      longTextSelection.start,
      longTextSelection.end,
      longTextSelection.direction,
    )
}

function applyLongText(text: string) {
  longTextOpen.value = false
  draft.value = text
  if (input.value instanceof HTMLTextAreaElement) input.value.value = text
  previewText()
  if (props.gridCommitOnLeave) commitText()
  else emit('draftChanged')
}

function previewText() {
  if (!props.live) return
  const result = props.field.parsePaste(draft.value, props.env)
  requiredError.value = result.ok && props.required && result.value === null
  if (requiredError.value) {
    emit('invalidValue')
    return
  }
  if (!result.ok) {
    reason.value = result.reason
    emit('invalidValue')
    return
  }
  reason.value = null
  emit('preview', result.value)
}

function commitSelect() {
  return finish(
    props.field.normalize(multiple.value ? selected.value : single.value),
  )
}

function toggleCheckbox() {
  const checked = !checkboxChecked.value
  if (finish(props.field.normalize(checked))) checkboxChecked.value = checked
}

function commit() {
  commitDraft()
}

function commitAttachments() {
  if (uploadingAttachments.value || attachmentError.value) return false
  if (attachmentFiles.value.length > attachmentOptions.value.maxFiles) {
    attachmentError.value = t('field.editor.attachmentLimit', {
      count: attachmentOptions.value.maxFiles,
    })
    return false
  }
  const committed = finish(
    props.field.normalize(attachmentFiles.value as unknown as WireValue),
  )
  if (committed) {
    attachmentPending.value = false
    emit('draftChanged')
  }
  return committed
}

async function addAttachmentFiles(files: FileList | File[]) {
  if (!props.tableId || !props.fieldId || uploadingAttachments.value) return
  const tableId = props.tableId
  const fieldId = props.fieldId
  const selectedFiles = Array.from(files)
  if (!selectedFiles.length) return
  attachmentPending.value = true
  emit('draftChanged')
  if (selectedFiles.length > availableAttachmentSlots.value) {
    attachmentError.value = t('field.editor.attachmentLimit', {
      count: attachmentOptions.value.maxFiles,
    })
    return
  }
  attachmentError.value = ''
  uploadingAttachments.value = true
  try {
    for (const file of selectedFiles) {
      if (unmounted) break
      try {
        const uploaded = await uploadAttachment(
          tableId,
          fieldId,
          file,
          AbortSignal.timeout(120_000),
        )
        if (unmounted) {
          await discardTemporaryAttachment(tableId, fieldId, uploaded.id)
          break
        }
        stagedAttachmentIds.add(uploaded.id)
        attachmentFiles.value = [...attachmentFiles.value, uploaded]
      } catch (cause) {
        attachmentError.value = errorMessage(cause)
        break
      }
    }
  } finally {
    uploadingAttachments.value = false
    if (props.live && !unmounted) commitAttachments()
    emit('draftChanged')
    if (attachmentInput.value) attachmentInput.value.value = ''
  }
}

function removeAttachment(file: AttachmentValue) {
  attachmentFiles.value = attachmentFiles.value.filter(
    (item) => item.id !== file.id,
  )
  if (stagedAttachmentIds.has(file.id)) stagedAttachmentIds.delete(file.id)
  removedAttachmentIds.add(file.id)
  attachmentPending.value = true
  emit('draftChanged')
  if (props.live) commitAttachments()
}

function dropAttachments(event: DragEvent) {
  dragOver.value = false
  if (event.dataTransfer?.files)
    void addAttachmentFiles(event.dataTransfer.files)
}

function attachmentInputChanged(event: Event) {
  const files = (event.target as HTMLInputElement).files
  if (files) void addAttachmentFiles(files)
}

function openAttachmentPicker() {
  attachmentInput.value?.click()
}

function userLabel(id: string) {
  if (userLabelDeleted(props.identity, props.tableId, id))
    return t('field.user.deleted')
  return (
    props.resolveUser?.(id) ?? t('field.user.unknown', { id: id.slice(0, 8) })
  )
}

function commitUsers() {
  const committed = finish(
    props.field.normalize(
      multipleUsers.value ? selectedUsers.value : single.value,
    ),
  )
  if (committed) {
    userPending.value = false
    emit('draftChanged')
  }
  return committed
}

function chooseUser(user: DirectoryPrincipal) {
  userSearchFailed.value = false
  if (!multipleUsers.value) {
    single.value = user.id
    selectedUsers.value = [user.id]
    commitUsers()
    return
  }
  if (!selectedUsers.value.includes(user.id)) {
    selectedUsers.value = [...selectedUsers.value, user.id]
    userPending.value = true
    emit('draftChanged')
    if (props.live) commitUsers()
  }
}

function removeUser(id: string) {
  selectedUsers.value = selectedUsers.value.filter((item) => item !== id)
  userPending.value = true
  emit('draftChanged')
  if (props.live) commitUsers()
}

function liveBlur(event: FocusEvent) {
  if (
    !props.live ||
    composing.value ||
    temporal.value ||
    ['user', 'attachment', 'select', 'checkbox'].includes(
      props.field.presentation,
    ) ||
    currentDraft() === props.field.formatCopy(props.value, props.env)
  )
    return
  const next = event.relatedTarget
  if (
    (next instanceof Node &&
      (event.currentTarget as HTMLElement).contains(next)) ||
    contains(next) ||
    isPointerInPanel()
  )
    return
  commitDraft()
}

async function searchUsers() {
  userSearchController?.abort()
  userResults.value = []
  userSearchFailed.value = false
  const query = userQuery.value.trim()
  if (!props.tableId || [...query].length < 2) {
    userSearching.value = false
    return
  }
  const controller = new AbortController()
  userSearchController = controller
  const generation = ++userSearchGeneration
  userSearching.value = true
  try {
    const listing = await searchTableUsers(
      props.tableId,
      query,
      AbortSignal.any([controller.signal, AbortSignal.timeout(10_000)]),
    )
    if (generation === userSearchGeneration) userResults.value = listing.items
  } catch {
    if (generation === userSearchGeneration && !controller.signal.aborted)
      userSearchFailed.value = true
  } finally {
    if (generation === userSearchGeneration) userSearching.value = false
  }
}

function keydown(event: KeyboardEvent) {
  if (temporal.value) return
  if (event.isComposing || composing.value) {
    if (props.gridCommitOnLeave) event.stopPropagation()
    return
  }
  if (
    props.field.presentation === 'multiline' &&
    event.target === input.value &&
    (event.key === 'F2' ||
      ((event.ctrlKey || event.metaKey) &&
        event.shiftKey &&
        event.key.toLowerCase() === 'e'))
  ) {
    event.preventDefault()
    event.stopPropagation()
    openLongTextDialog()
    return
  }
  if (event.key === 'Escape') {
    event.preventDefault()
    emit('cancel')
    return
  }
  if (event.key !== 'Enter') return
  if (props.field.presentation === 'checkbox') return
  if (props.field.presentation === 'user') return
  if (props.field.presentation === 'attachment') return
  const multiline = props.field.presentation === 'multiline'
  if (multiline && !(event.ctrlKey || event.metaKey)) return
  if (!multiline && event.shiftKey) return
  event.preventDefault()
  if (props.field.presentation === 'select') commitSelect()
  else commitText()
}
</script>

<template>
  <div
    v-if="!field.readOnly"
    class="field-editor"
    @keydown="keydown"
    @focusout="liveBlur"
  >
    <button
      v-if="field.presentation === 'checkbox'"
      ref="checkboxButton"
      type="button"
      class="field-checkbox"
      role="checkbox"
      :aria-checked="checkboxChecked"
      :aria-label="label ?? t('field.editor.value')"
      :aria-required="required || undefined"
      @click="toggleCheckbox"
      @keydown.space.prevent.stop="toggleCheckbox"
      @keydown.enter.prevent.stop="toggleCheckbox"
    >
      <CheckboxIcon
        :icon="(field.options as CheckboxOptions).icon"
        :checked="checkboxChecked"
      />
    </button>
    <TemporalEditor
      v-else-if="temporal"
      ref="temporalEditor"
      :field="field"
      :value="value"
      :env="env"
      :label="label ?? t('field.editor.value')"
      :required="required"
      :auto-focus="autoFocus"
      :live="live"
      :grid-commit-on-leave="gridCommitOnLeave"
      :initial-draft="initialDraft"
      :default-placeholder="defaultPlaceholder"
      @commit="temporalCommit"
      @preview="emit('preview', $event)"
      @invalid-value="emit('invalidValue')"
      @invalid-draft="
        (text, why, message) => emit('invalidDraft', text, why, message)
      "
      @composition-finished="emit('compositionFinished')"
      @cancel="emit('cancel')"
      @focusout="emit('focusout', $event)"
      @draft-changed="emit('draftChanged')"
    />
    <select
      v-else-if="field.presentation === 'select' && multiple"
      ref="input"
      v-model="selected"
      multiple
      :size="Math.min(8, Math.max(2, choices.length))"
      :aria-label="label ?? t('field.editor.value')"
      :aria-invalid="reason !== null || requiredError"
      :required="required"
      @change="live ? commitSelect() : undefined"
    >
      <option v-for="choice in choices" :key="choice.id" :value="choice.label">
        {{ choice.label }}
      </option>
    </select>
    <select
      v-else-if="field.presentation === 'select'"
      ref="input"
      v-model="single"
      :aria-label="label ?? t('field.editor.value')"
      :aria-invalid="reason !== null || requiredError"
      :required="required"
      @change="commitSelect"
    >
      <option value="">{{ t('field.editor.none') }}</option>
      <option v-for="choice in choices" :key="choice.id" :value="choice.label">
        {{ choice.label }}
      </option>
    </select>
    <div
      v-else-if="field.presentation === 'multiline'"
      class="field-long-text"
      :class="{ 'field-long-text--grid': gridCommitOnLeave }"
    >
      <textarea
        ref="input"
        v-model="draft"
        :aria-label="label ?? t('field.editor.value')"
        :aria-invalid="reason !== null || requiredError"
        :placeholder="defaultPlaceholder ?? t('field.editor.multilineHint')"
        :required="required"
        @input="previewText"
        @compositionstart="composing = true"
        @compositionend="compositionEnded"
      />
      <button
        type="button"
        class="field-expand"
        :aria-label="
          t('cellEditors.ces05.expand', {
            field: label ?? t('field.editor.value'),
          })
        "
        @mousedown.prevent
        @click="openLongTextDialog"
      >
        <AppIcon name="expand" />
      </button>
    </div>
    <div v-else-if="field.presentation === 'user'" class="field-user-editor">
      <input
        ref="input"
        v-model="userQuery"
        type="search"
        :aria-label="t('field.editor.userSearch')"
        :placeholder="t('field.editor.userSearchPlaceholder')"
        :disabled="!tableId"
        @input="searchUsers"
      />
      <p v-if="userSearching">{{ t('field.editor.searchingUsers') }}</p>
      <p v-if="userSearchFailed" role="alert">
        {{ t('field.editor.userSearchFailed') }}
      </p>
      <ul v-if="userResults.length" class="field-user-results">
        <li v-for="user in userResults" :key="user.id">
          <button type="button" @click="chooseUser(user)">
            {{ user.name }}
            <span>
              {{ t('field.editor.username', { username: user.username }) }}
            </span>
          </button>
        </li>
      </ul>
      <ul v-if="selectedUsers.length" class="field-user-selected">
        <li v-for="id in selectedUsers" :key="id">
          <span>{{ userLabel(id) }}</span>
          <button
            v-if="multipleUsers"
            type="button"
            :aria-label="t('field.editor.removeUser', { name: userLabel(id) })"
            @click="removeUser(id)"
          >
            {{ t('field.panel.remove') }}
          </button>
        </li>
      </ul>
      <button
        v-if="multipleUsers && !live"
        type="button"
        :disabled="selectedUsers.length === 0 && !userPending"
        @click="commitUsers"
      >
        {{ submitLabel ?? t('field.editor.apply') }}
      </button>
    </div>
    <div
      v-else-if="field.presentation === 'attachment'"
      class="field-attachment-editor"
      :class="{ 'drag-over': dragOver }"
      @dragover.prevent="dragOver = true"
      @dragleave.prevent="dragOver = false"
      @drop.prevent="dropAttachments"
    >
      <input
        ref="attachmentInput"
        class="attachment-file-input"
        type="file"
        :accept="acceptAttribute"
        multiple
        :disabled="uploadingAttachments || availableAttachmentSlots === 0"
        @change="attachmentInputChanged"
      />
      <button
        type="button"
        :disabled="uploadingAttachments || availableAttachmentSlots === 0"
        @click="openAttachmentPicker"
      >
        {{ t('field.editor.addAttachments') }}
      </button>
      <span>{{ t('field.editor.dropAttachments') }}</span>
      <p v-if="uploadingAttachments" role="status">
        {{ t('field.editor.uploadingAttachments') }}
      </p>
      <ul class="field-attachment-list">
        <li v-for="file in attachmentFiles" :key="file.id">
          <img v-if="file.thumb" :src="file.thumb" :alt="file.name" />
          <span>{{ file.name }}</span>
          <button
            type="button"
            :aria-label="
              t('field.editor.removeAttachment', { name: file.name })
            "
            :disabled="uploadingAttachments"
            @click="removeAttachment(file)"
          >
            {{ t('field.panel.remove') }}
          </button>
        </li>
      </ul>
      <p v-if="attachmentError" class="field-editor-error" role="alert">
        {{ attachmentError }}
      </p>
    </div>
    <input
      v-else
      ref="input"
      v-model="draft"
      type="text"
      :inputmode="field.presentation === 'number' ? 'decimal' : undefined"
      :aria-label="label ?? t('field.editor.value')"
      :aria-invalid="reason !== null || requiredError"
      :placeholder="placeholder"
      :required="required"
      @input="previewText"
      @compositionstart="composing = true"
      @compositionend="compositionEnded"
    />
    <button v-if="showSubmitButton" type="button" @click="commit">
      {{ submitLabel ?? t('field.editor.apply') }}
    </button>
    <p v-if="reason || requiredError" class="field-editor-error" role="alert">
      {{ error }}
    </p>
    <LongTextDialog
      v-if="field.presentation === 'multiline'"
      ref="longTextDialog"
      :open="longTextOpen"
      :value="draft"
      :field-name="label ?? t('field.editor.value')"
      @apply="applyLongText"
      @cancel="restoreLongTextFocus"
    />
  </div>
</template>

<style scoped>
.field-editor {
  display: grid;
  gap: 4px;
}
.field-checkbox {
  display: inline-grid;
  width: fit-content;
  padding: 0;
  place-items: center;
  border: 0;
  background: transparent;
  cursor: pointer;
}
.field-checkbox:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 3px;
}
.field-editor input[type='text'],
.field-editor textarea,
.field-editor select,
.field-attachment-editor {
  width: 100%;
  padding: 4px 6px;
  background: var(--surface);
  color: var(--text);
  border: 2px solid var(--accent);
}
.field-editor textarea {
  min-height: 90px;
}
.field-long-text {
  position: relative;
}
.field-long-text textarea {
  padding-right: 30px;
}
.field-expand {
  position: absolute;
  top: 4px;
  right: 4px;
  display: grid;
  width: 24px;
  height: 24px;
  padding: 4px;
  place-items: center;
  border: 0;
  border-radius: 4px;
  background: var(--surface);
  color: var(--muted);
  cursor: pointer;
}
.field-expand:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 1px;
}
.field-long-text--grid .field-expand {
  opacity: 0;
  pointer-events: none;
}
.field-long-text--grid:hover .field-expand,
.field-long-text--grid:focus-within .field-expand {
  opacity: 1;
  pointer-events: auto;
}
.field-user-results,
.field-user-selected,
.field-attachment-list {
  display: grid;
  gap: 4px;
  max-height: 180px;
  margin: 0;
  padding: 0;
  overflow: auto;
  list-style: none;
}
.attachment-file-input {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  clip-path: inset(50%);
}
.field-attachment-editor {
  display: grid;
  gap: 6px;
  border: 1px dashed var(--border);
  padding: 8px;
}
.field-attachment-editor.drag-over {
  border-color: var(--accent);
  background: color-mix(in srgb, var(--accent) 10%, var(--surface));
}
.field-attachment-list li {
  display: flex;
  gap: 6px;
  align-items: center;
  min-width: 0;
}
.field-attachment-list li span {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.field-attachment-list img {
  width: 48px;
  height: 36px;
  object-fit: cover;
}
.field-user-results button,
.field-user-selected li {
  display: flex;
  justify-content: space-between;
  width: 100%;
  padding: 4px 6px;
  background: var(--surface);
  color: var(--text);
  border: 1px solid var(--border);
}
.field-editor-error {
  margin: 0;
  color: #c0362c;
  font-size: 0.85em;
}
:global(:root[data-theme='dark']) .field-editor-error {
  color: #ff8a80;
}
</style>
