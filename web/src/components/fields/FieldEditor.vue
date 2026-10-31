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
import { queueUserLabels, searchTableUsers } from '../../fields/userDirectory'
import { fieldErrorMessage } from '../../fields/messages'
import type { FieldEnv, Reason, Result, WireValue } from '../../fields/types'

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
  },
)
const emit = defineEmits<{
  commit: [value: WireValue]
  preview: [value: WireValue]
  invalidValue: []
  cancel: []
}>()
const { t } = useI18n()

const input = ref<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>()
const attachmentInput = ref<HTMLInputElement>()
const reason = ref<Reason | null>(null)
const draft = ref(
  props.field.readOnly ? '' : props.field.formatCopy(props.value, props.env),
)
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
const dragOver = ref(false)
const stagedAttachmentIds = new Set<string>()
const removedAttachmentIds = new Set<string>()
const userQuery = ref('')
const userResults = ref<DirectoryPrincipal[]>([])
const userSearching = ref(false)
const userSearchFailed = ref(false)
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
    (props.submitLabel ||
      (props.field.presentation === 'select' && multiple.value) ||
      props.field.presentation === 'attachment') &&
    !(props.field.presentation === 'user' && multipleUsers.value),
  ),
)
const placeholder = computed(() => {
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
  reason.value ? fieldErrorMessage(reason.value) : '',
)

onMounted(() => {
  if (props.autoFocus) input.value?.focus()
})
async function discardTemporaryAttachments() {
  if (!props.tableId || !props.fieldId) return
  const ids = props.preserveUploads
    ? removedAttachmentIds
    : new Set([...stagedAttachmentIds, ...removedAttachmentIds])
  for (const id of ids)
    await discardAttachment(props.tableId, props.fieldId, id).catch(() => {})
}

onBeforeUnmount(() => {
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
  if (!result.ok) {
    reason.value = result.reason
    return
  }
  reason.value = null
  emit('commit', result.value)
}

const commitText = () => finish(props.field.parsePaste(draft.value, props.env))

function previewText() {
  if (!props.live) return
  const result = props.field.parsePaste(draft.value, props.env)
  if (!result.ok) {
    reason.value = result.reason
    emit('invalidValue')
    return
  }
  reason.value = null
  emit('preview', result.value)
}

function commitSelect() {
  finish(props.field.normalize(multiple.value ? selected.value : single.value))
}

function commitCheckbox(event: Event) {
  finish(props.field.normalize((event.target as HTMLInputElement).checked))
}

function commit() {
  if (props.field.presentation === 'user') commitUsers()
  else if (props.field.presentation === 'attachment') commitAttachments()
  else if (props.field.presentation === 'select') commitSelect()
  else if (props.field.presentation === 'checkbox')
    finish(
      props.field.normalize(
        input.value instanceof HTMLInputElement && input.value.checked,
      ),
    )
  else commitText()
}

function commitAttachments() {
  if (attachmentFiles.value.length > attachmentOptions.value.maxFiles) {
    attachmentError.value = t('field.editor.attachmentLimit', {
      count: attachmentOptions.value.maxFiles,
    })
    return
  }
  attachmentError.value = ''
  finish(props.field.normalize(attachmentFiles.value as unknown as WireValue))
}

async function addAttachmentFiles(files: FileList | File[]) {
  if (!props.tableId || !props.fieldId || uploadingAttachments.value) return
  const selectedFiles = Array.from(files)
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
      try {
        const uploaded = await uploadAttachment(
          props.tableId,
          props.fieldId,
          file,
          AbortSignal.timeout(120_000),
        )
        stagedAttachmentIds.add(uploaded.id)
        attachmentFiles.value = [...attachmentFiles.value, uploaded]
      } catch (cause) {
        attachmentError.value = errorMessage(cause)
        break
      }
    }
  } finally {
    uploadingAttachments.value = false
    if (attachmentInput.value) attachmentInput.value.value = ''
  }
}

function removeAttachment(file: AttachmentValue) {
  attachmentFiles.value = attachmentFiles.value.filter(
    (item) => item.id !== file.id,
  )
  if (stagedAttachmentIds.has(file.id)) stagedAttachmentIds.delete(file.id)
  removedAttachmentIds.add(file.id)
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
  return (
    props.resolveUser?.(id) ?? t('field.user.unknown', { id: id.slice(0, 8) })
  )
}

function commitUsers() {
  finish(
    props.field.normalize(
      multipleUsers.value ? selectedUsers.value : single.value,
    ),
  )
}

function chooseUser(user: DirectoryPrincipal) {
  userSearchFailed.value = false
  if (!multipleUsers.value) {
    finish(props.field.normalize(user.id))
    return
  }
  if (!selectedUsers.value.includes(user.id))
    selectedUsers.value = [...selectedUsers.value, user.id]
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
  if (event.isComposing) return
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
  <div v-if="!field.readOnly" class="field-editor" @keydown="keydown">
    <input
      v-if="field.presentation === 'checkbox'"
      ref="input"
      type="checkbox"
      :checked="value === true"
      :aria-label="label ?? t('field.editor.value')"
      :required="required"
      @change="commitCheckbox"
    />
    <select
      v-else-if="field.presentation === 'select' && multiple"
      ref="input"
      v-model="selected"
      multiple
      :size="Math.min(8, Math.max(2, choices.length))"
      :aria-label="label ?? t('field.editor.value')"
      :aria-invalid="reason !== null"
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
      :aria-invalid="reason !== null"
      :required="required"
      @change="commitSelect"
    >
      <option value="">{{ t('field.editor.none') }}</option>
      <option v-for="choice in choices" :key="choice.id" :value="choice.label">
        {{ choice.label }}
      </option>
    </select>
    <textarea
      v-else-if="field.presentation === 'multiline'"
      ref="input"
      v-model="draft"
      :aria-label="label ?? t('field.editor.value')"
      :aria-invalid="reason !== null"
      :placeholder="t('field.editor.multilineHint')"
      :required="required"
      @input="previewText"
    />
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
            @click="selectedUsers = selectedUsers.filter((item) => item !== id)"
          >
            {{ t('field.panel.remove') }}
          </button>
        </li>
      </ul>
      <button
        v-if="multipleUsers"
        type="button"
        :disabled="selectedUsers.length === 0"
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
      :aria-invalid="reason !== null"
      :placeholder="placeholder"
      :required="required"
      @input="previewText"
    />
    <button v-if="showSubmitButton" type="button" @click="commit">
      {{ submitLabel ?? t('field.editor.apply') }}
    </button>
    <p v-if="reason" class="field-editor-error" role="alert">{{ error }}</p>
  </div>
</template>

<style scoped>
.field-editor {
  display: grid;
  gap: 4px;
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
