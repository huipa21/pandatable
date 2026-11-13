<script setup lang="ts">
import {
  computed,
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  toRaw,
  watch,
} from 'vue'
import AppIcon, { fieldIconName } from '../AppIcon.vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import {
  discardAttachment,
  errorMessage,
  request,
  RequestError,
} from '../../api/client'
import type {
  DataRow,
  RelationLink,
  RowMutation,
  RowPatchMutation,
  TableSchema,
} from '../../api/types'
import { bindField } from '../../fields/registry'
import { isWireValue, type FieldEnv, type WireValue } from '../../fields/types'
import { userLabel } from '../../fields/userDirectory'
import { useAuthStore } from '../../stores/auth'
import { useNavStore } from '../../stores/nav'
import { useRealtimeStore, type RealtimeEvent } from '../../stores/realtime'
import { linkUndoSteps, patchUndoStep, useUndoStore } from '../../stores/undo'
import FieldEditor from '../fields/FieldEditor.vue'
import FieldRenderer from '../fields/FieldRenderer.vue'
import RelationLinks from '../relations/RelationLinks.vue'
import RelationPicker from '../relations/RelationPicker.vue'
import CellPresence from '../presence/CellPresence.vue'
import CommentPanel from './CommentPanel.vue'
import HistoryPanel from './HistoryPanel.vue'

const rowPatchBodyLimit = 64 * 1024

const props = defineProps<{
  schema: TableSchema
  viewId: string
  rowId: string | null
  identity: string
  focusComment?: string
  workspace?: HTMLElement
  navigateRow?: (id: string, direction: -1 | 1) => Promise<string | null>
}>()
const emit = defineEmits<{
  close: []
  saved: []
  created: [rowId: string]
  schemaChanged: []
  navigate: [rowId: string]
}>()
const { t, locale } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const nav = useNavStore()
const realtime = useRealtimeStore()
const undo = useUndoStore()
const resolveUser = (id: string) =>
  userLabel(props.identity, props.schema.table.id, id)
const dialog = ref<HTMLDialogElement>()
const historyTab = ref<HTMLButtonElement>()
const commentsTab = ref<HTMLButtonElement>()
const activeTab = ref<'comments' | 'history'>('comments')
const activityExpanded = ref(true)
const search = ref('')
const fullscreen = ref(false)
const workspaceRect = ref<DOMRect>()
const duplicating = ref(false)
const navigating = ref(false)
const navigationNotice = ref('')
const deleteDialog = ref<HTMLDialogElement>()
const copiedAttachments = new Set<string>()
const actionsBlocked = computed(
  () =>
    saving.value ||
    navigating.value ||
    dirty.value ||
    relationEditing.value !== null,
)
const row = ref<DataRow | null>(null)
const loading = ref(false)
const saving = ref(false)
const error = ref<unknown>(null)
const changedBy = ref('')
const relationEditing = ref<string | null>(null)
const saved = ref(false)
const preserveUploads = ref(false)
const draft = ref<Record<string, WireValue>>({})
const invalidDraftFields = ref(new Set<string>())
const draftVersion = ref(0)
const pendingVersion = ref(0)
type DraftEditor = Pick<
  InstanceType<typeof FieldEditor>,
  'commitDraft' | 'hasPendingDraft' | 'isComposing'
>
const draftEditors = new Map<string, DraftEditor>()
const changedFields = computed(() =>
  Object.fromEntries(
    Object.entries(draft.value).filter(
      ([id, value]) => !sameFieldValue(id, value, row.value?.fields[id]),
    ),
  ),
)
const dirty = computed(() => {
  void pendingVersion.value
  return (
    Object.keys(changedFields.value).length > 0 ||
    invalidDraftFields.value.size > 0 ||
    hasPendingDrafts()
  )
})
const blocked = ref(false)
const creating = computed(() => props.rowId === null || duplicating.value)
const title = computed(() => {
  if (creating.value) return t('mobileRows.newRow')
  const primary =
    availableFields.value.find(
      (item) =>
        item.field.id === props.schema.table.displayFieldId &&
        item.field.uiType !== 'checkbox',
    ) ??
    availableFields.value.find((item) =>
      ['text', 'long_text'].includes(item.field.uiType),
    )
  const label =
    primary?.result.ok && primary.value.ok
      ? primary.result.value.formatDisplay(primary.value.value, env.value)
      : ''
  return label || t('rowExpand.title', { id: props.rowId })
})
const dialogStyle = computed(() => {
  const rect = workspaceRect.value
  if (!fullscreen.value || !rect) return undefined
  return {
    top: `${rect.top}px`,
    left: `${rect.left}px`,
    width: `${rect.width}px`,
    height: `${rect.height}px`,
  }
})
let controller: AbortController | undefined
let realtimeController: AbortController | undefined
let realtimeReloadPending = false
let generation = 0
let disposed = false
let draftCleanupStarted = false
let draftContext: { tableId: string; attachmentFieldIds: string[] } | undefined
const env = computed<FieldEnv>(() => ({
  timeZone: auth.user?.effectiveTimezone,
  locale: locale.value,
}))
const canWrite = computed(
  () =>
    props.identity === auth.user?.id &&
    (props.schema.myLevel === 'write' || props.schema.myLevel === 'owner'),
)
const ARRAY_VALUE_KINDS = new Set([
  'multi_select',
  'text_array',
  'int_array',
  'multi_user',
  'attachment',
])
function isFieldWireValue(kind: string, value: unknown): value is WireValue {
  if (!isWireValue(value)) return false
  if (kind === 'link')
    return (
      Array.isArray(value) &&
      value.every(
        (item) =>
          typeof item === 'object' &&
          item !== null &&
          'id' in item &&
          typeof item.id === 'string' &&
          'label' in item &&
          typeof item.label === 'string',
      )
    )
  if (value === null || kind === 'json' || kind === 'attachment') return true
  if (ARRAY_VALUE_KINDS.has(kind))
    return (
      Array.isArray(value) && value.every((item) => typeof item === 'string')
    )
  return typeof value !== 'object'
}
const fields = computed(() =>
  [...props.schema.fields]
    .sort((a, b) => a.position - b.position)
    .map((field) => {
      const value = Object.hasOwn(draft.value, field.id)
        ? draft.value[field.id]
        : creating.value
          ? field.default
          : row.value?.fields[field.id]
      return {
        field,
        result: bindField(field.uiType, field.options),
        value: isFieldWireValue(field.uiType, value)
          ? { ok: true as const, value }
          : { ok: false as const },
      }
    }),
)
type DialogField = (typeof fields.value)[number]
const availableFields = computed(() =>
  fields.value.filter((item) => !item.field.isSystem),
)
const filteredFields = computed(() => {
  const query = search.value.trim().toLocaleLowerCase(locale.value)
  return availableFields.value.filter((item) => {
    const storedValue = row.value?.fields[item.field.id]
    return (
      !query ||
      item.field.name.toLocaleLowerCase(locale.value).includes(query) ||
      (!creating.value &&
        item.result.ok &&
        isFieldWireValue(item.field.uiType, storedValue) &&
        item.result.value
          .formatDisplay(storedValue, env.value)
          .toLocaleLowerCase(locale.value)
          .includes(query))
    )
  })
})
const filteredFieldIds = computed(
  () => new Set(filteredFields.value.map((item) => item.field.id)),
)

function defaultPlaceholder(item: DialogField) {
  if (
    !creating.value ||
    Object.hasOwn(draft.value, item.field.id) ||
    item.field.default === null ||
    !item.result.ok ||
    !item.value.ok ||
    ['checkbox', 'select', 'user', 'attachment'].includes(
      item.result.value.presentation,
    )
  )
    return undefined
  return item.result.value.formatCopy(item.value.value, env.value) || undefined
}

const message = computed(() =>
  typeof error.value === 'string'
    ? error.value
    : error.value
      ? errorMessage(error.value)
      : '',
)

function current(token: number) {
  return !disposed && token === generation && props.identity === auth.user?.id
}

function begin() {
  controller?.abort()
  if (realtimeController) {
    realtimeReloadPending = true
    realtimeController.abort()
  }
  realtimeController = undefined
  controller = new AbortController()
  return {
    token: ++generation,
    signal: AbortSignal.any([controller.signal, AbortSignal.timeout(10_000)]),
  }
}

function resetDraft() {
  draft.value = {}
  invalidDraftFields.value.clear()
  draftEditors.clear()
  draftVersion.value++
  preserveUploads.value = false
}

function startDraftContext() {
  draftContext = {
    tableId: props.schema.table.id,
    attachmentFieldIds: props.schema.fields
      .filter((field) => field.uiType === 'attachment')
      .map((field) => field.id),
  }
  draftCleanupStarted = false
}

function failure(cause: unknown) {
  error.value = cause
  saved.value = false
  preserveUploads.value = false
  draftCleanupStarted = false
  if (!(cause instanceof RequestError)) return
  if (cause.code === 'ERR_PASSWORD_CHANGE_REQUIRED') {
    void discardDraftAttachments()
    draftContext = undefined
    blocked.value = true
    row.value = null
    resetDraft()
    void router.replace('/change-password')
  } else if ([401, 403, 404].includes(cause.status)) {
    blocked.value = true
    row.value = null
    if (cause.status === 401) {
      auth.setUser(null)
      void router.replace('/login')
    } else {
      void nav.refresh(true)
      emit('schemaChanged')
    }
  } else if (cause.code === 'ERR_SCHEMA_CHANGED') {
    blocked.value = true
    row.value = null
    emit('schemaChanged')
  }
}

async function load(schemaChanged = false) {
  const { token, signal } = begin()
  void discardDraftAttachments()
  row.value = null
  resetDraft()
  relationEditing.value = null
  duplicating.value = false
  copiedAttachments.clear()
  draftContext = undefined
  startDraftContext()
  search.value = ''
  navigationNotice.value = ''
  blocked.value = false
  error.value = schemaChanged
    ? new RequestError('ERR_SCHEMA_CHANGED', 409)
    : null
  saved.value = false
  saving.value = false
  loading.value = true
  navigating.value = false
  await nextTick()
  if (!current(token)) return
  if (!dialog.value?.open) dialog.value?.showModal()
  if (props.rowId === null) {
    loading.value = false
    return
  }
  try {
    const result = await request<DataRow>(
      `/tables/${props.schema.table.id}/rows/${encodeURIComponent(props.rowId)}`,
      'GET',
      undefined,
      signal,
    )
    if (current(token)) {
      row.value = result
      rememberAttachments()
    }
  } catch (cause) {
    if (current(token)) failure(cause)
  } finally {
    if (current(token)) loading.value = false
  }
}

async function save() {
  if (
    creating.value ||
    !canWrite.value ||
    blocked.value ||
    loading.value ||
    saving.value ||
    !props.rowId ||
    !row.value
  )
    return
  const pendingValid = flushPendingDrafts()
  if (invalidDraftFields.value.size) {
    error.value = t('gridPolish.gp05.invalid')
    return
  }
  if (!pendingValid || hasPendingDrafts()) {
    error.value = t('gridPolish.gp05.applyPending')
    return
  }
  const changes = changedFields.value
  if (!Object.keys(changes).length) return
  const body = { fields: changes }
  const previous = Object.fromEntries(
    Object.keys(changes).map((id) => [id, row.value!.fields[id]!]),
  )
  if (Object.values(previous).some((value) => value === undefined)) {
    error.value = new RequestError('ERR_SCHEMA_CHANGED', 409)
    return
  }
  saved.value = false
  if (
    new TextEncoder().encode(JSON.stringify(body)).length > rowPatchBodyLimit
  ) {
    error.value = t('rowExpand.tooLarge')
    return
  }
  const { token, signal } = begin()
  saving.value = true
  error.value = null
  try {
    const result = await request<RowPatchMutation>(
      `/tables/${props.schema.table.id}/rows/${encodeURIComponent(props.rowId)}`,
      'PATCH',
      body,
      signal,
    )
    if (!current(token)) return
    const previousFields = result.previous ?? previous
    const scalarPrevious = result.changes
      ? Object.fromEntries(
          Object.entries(previousFields).filter(([id]) =>
            props.schema.fields.some(
              (field) => field.id === id && field.uiType !== 'link',
            ),
          ),
        )
      : previousFields
    undo.record([
      patchUndoStep(
        props.schema.table.id,
        [
          {
            id: result.row.id,
            fields: scalarPrevious,
          },
        ],
        [result.row],
      ),
      ...linkUndoSteps(result.changes ?? []),
    ])
    row.value = result.row
    preserveUploads.value = true
    await nextTick()
    draft.value = {}
    invalidDraftFields.value.clear()
    draftVersion.value++
    rememberAttachments()
    startDraftContext()
    await nextTick()
    preserveUploads.value = false
    saved.value = true
    emit('saved')
  } catch (cause) {
    if (current(token)) failure(cause)
  } finally {
    if (current(token)) saving.value = false
  }
}

function isDraftEditor(editor: unknown): editor is DraftEditor {
  return (
    typeof editor === 'object' &&
    editor !== null &&
    'commitDraft' in editor &&
    typeof editor.commitDraft === 'function' &&
    'hasPendingDraft' in editor &&
    typeof editor.hasPendingDraft === 'function' &&
    'isComposing' in editor &&
    typeof editor.isComposing === 'function'
  )
}

function setDraftEditor(fieldId: string, editor: unknown) {
  if (editor === null) draftEditors.delete(fieldId)
  else if (isDraftEditor(editor)) draftEditors.set(fieldId, editor)
  else throw new TypeError('Field editor does not expose its draft API')
}

function updateDraft(fieldId: string, value: WireValue) {
  if (!canWrite.value || blocked.value || saving.value) return
  const item = fields.value.find((field) => field.field.id === fieldId)
  if (
    !item ||
    item.field.isSystem ||
    (item.field.uiType !== 'link' &&
      (!item.result.ok || item.result.value.readOnly))
  )
    return
  const next = { ...draft.value }
  if (
    !creating.value &&
    sameFieldValue(fieldId, value, row.value?.fields[fieldId])
  )
    delete next[fieldId]
  else next[fieldId] = value
  draft.value = next
  invalidDraftFields.value.delete(fieldId)
  error.value = null
  saved.value = false
}

function sameFieldValue(fieldId: string, value: unknown, previous: unknown) {
  const field = props.schema.fields.find((item) => item.id === fieldId)
  if (field?.uiType === 'link') {
    const ids = (links: unknown) =>
      Array.isArray(links)
        ? links.map((link: RelationLink) => link.id).sort()
        : links
    return JSON.stringify(ids(value)) === JSON.stringify(ids(previous))
  }
  return JSON.stringify(value) === JSON.stringify(previous)
}

function rememberAttachments() {
  copiedAttachments.clear()
  for (const field of props.schema.fields) {
    if (field.uiType !== 'attachment') continue
    const value = row.value?.fields[field.id]
    if (!Array.isArray(value)) continue
    for (const file of value)
      if (
        typeof file === 'object' &&
        file !== null &&
        'id' in file &&
        typeof file.id === 'string'
      )
        copiedAttachments.add(file.id)
  }
}

function hasPendingDrafts() {
  return [...draftEditors.values()].some((editor) => editor.hasPendingDraft())
}

function flushPendingDrafts() {
  let valid = true
  for (const editor of draftEditors.values())
    if (
      editor.isComposing() ||
      (editor.hasPendingDraft() && !editor.commitDraft())
    )
      valid = false
  return valid
}

async function create() {
  if (
    !creating.value ||
    !canWrite.value ||
    blocked.value ||
    loading.value ||
    saving.value
  )
    return
  const pendingValid = flushPendingDrafts()
  if (invalidDraftFields.value.size) {
    error.value = t('gridPolish.gp05.invalid')
    return
  }
  if (!pendingValid || hasPendingDrafts()) {
    error.value = t('gridPolish.gp05.applyPending')
    return
  }
  const body = { rows: [{ fields: draft.value }] }
  if (
    Object.keys(draft.value).length > 5000 ||
    new TextEncoder().encode(JSON.stringify(body)).length > 512 * 1024
  ) {
    error.value = t('mobileRows.createLimit')
    return
  }
  const { token, signal } = begin()
  saving.value = true
  error.value = null
  try {
    const result = await request<RowMutation>(
      `/tables/${props.schema.table.id}/rows`,
      'POST',
      body,
      signal,
    )
    if (!current(token)) return
    const created = result.rows[0]
    if (result.rows.length !== 1 || !created || !/^[1-9]\d*$/.test(created.id))
      throw new Error('Row creation returned an unsupported response')
    undo.record([
      { kind: 'delete', tableId: props.schema.table.id, ids: [created.id] },
    ])
    preserveUploads.value = true
    draft.value = {}
    draftContext = undefined
    emit('created', created.id)
  } catch (cause) {
    if (current(token)) failure(cause)
  } finally {
    if (current(token)) saving.value = false
  }
}

function saveShortcut(event: KeyboardEvent) {
  if (
    (event.target instanceof Element &&
      event.target.closest('dialog') !== dialog.value) ||
    event.isComposing ||
    event.key !== 'Enter' ||
    !(event.ctrlKey || event.metaKey) ||
    event.altKey
  )
    return
  event.preventDefault()
  event.stopPropagation()
  if (creating.value) void create()
  else void save()
}

function canEditField(item: DialogField) {
  return (
    canWrite.value &&
    !item.field.isSystem &&
    item.result.ok &&
    !item.result.value.readOnly
  )
}

function activateRelation(id: string) {
  if (
    !canWrite.value ||
    blocked.value ||
    saving.value ||
    navigating.value ||
    props.schema.fields.find((field) => field.id === id)?.isSystem
  )
    return
  relationEditing.value = id
}

function removeRelationLink(fieldId: string, link: RelationLink) {
  if (!canWrite.value || saving.value || creating.value || !row.value) return
  const item = fields.value.find((item) => item.field.id === fieldId)
  if (!item?.value.ok || !Array.isArray(item.value.value)) return
  updateDraft(
    fieldId,
    item.value.value.filter(
      (value) =>
        typeof value === 'object' &&
        value !== null &&
        'id' in value &&
        value.id !== link.id,
    ),
  )
}

async function discardDraftAttachments() {
  const context = draftContext
  if (!context || draftCleanupStarted) return
  draftCleanupStarted = true
  const ids = new Map<string, Set<string>>()
  for (const fieldId of context.attachmentFieldIds) {
    const value = draft.value[fieldId]
    if (!Array.isArray(value)) continue
    for (const item of value) {
      if (
        typeof item === 'object' &&
        item !== null &&
        'id' in item &&
        typeof item.id === 'string' &&
        !copiedAttachments.has(item.id)
      )
        ids.set(fieldId, new Set([...(ids.get(fieldId) ?? []), item.id]))
    }
  }
  await Promise.all(
    [...ids].flatMap(([fieldId, attachmentIds]) =>
      [...attachmentIds].map((attachmentId) =>
        discardAttachment(context.tableId, fieldId, attachmentId).catch(
          (cause: unknown) =>
            console.error('Failed to discard a temporary attachment', cause),
        ),
      ),
    ),
  )
}

function close() {
  void discardDraftAttachments()
  generation++
  controller?.abort()
  dialog.value?.close()
  emit('close')
}

function switchTab(tab: 'comments' | 'history', focus = false) {
  activeTab.value = tab
  activityExpanded.value = true
  realtime.setFocus(props.schema.table.id, null)
  if (focus)
    void nextTick(() =>
      (tab === 'comments' ? commentsTab.value : historyTab.value)?.focus(),
    )
}

function tabKey(event: KeyboardEvent) {
  if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
    event.preventDefault()
    const tabs = ['comments', 'history'] as const
    const index = tabs.indexOf(activeTab.value)
    switchTab(tabs[(index + 1) % 2]!, true)
  } else if (event.key === 'Home') {
    event.preventDefault()
    switchTab('comments', true)
  } else if (event.key === 'End') {
    event.preventDefault()
    switchTab('history', true)
  }
}

function duplicate() {
  if (
    !row.value ||
    !canWrite.value ||
    saving.value ||
    dirty.value ||
    relationEditing.value
  )
    return
  const values: Record<string, WireValue> = {}
  copiedAttachments.clear()
  for (const item of fields.value) {
    if (
      !item.field.isSystem &&
      item.result.ok &&
      !item.result.value.readOnly &&
      item.value.ok
    ) {
      values[item.field.id] = structuredClone(toRaw(item.value.value))
      if (item.field.uiType === 'attachment' && Array.isArray(item.value.value))
        for (const attachment of item.value.value)
          if (
            typeof attachment === 'object' &&
            attachment !== null &&
            'id' in attachment &&
            typeof attachment.id === 'string'
          )
            copiedAttachments.add(attachment.id)
    }
  }
  duplicating.value = true
  startDraftContext()
  draft.value = values
  invalidDraftFields.value.clear()
  preserveUploads.value = false
  search.value = ''
  saved.value = false
  error.value = null
}

async function navigate(direction: -1 | 1) {
  if (
    !props.rowId ||
    creating.value ||
    !props.navigateRow ||
    navigating.value ||
    saving.value ||
    dirty.value ||
    relationEditing.value
  )
    return
  const token = generation
  navigating.value = true
  navigationNotice.value = ''
  error.value = null
  try {
    const id = await props.navigateRow(props.rowId, direction)
    if (!current(token)) return
    if (id) emit('navigate', id)
    else
      navigationNotice.value =
        direction === -1 ? t('rowExpand.firstRow') : t('rowExpand.lastRow')
  } catch (cause) {
    if (current(token)) failure(cause)
  } finally {
    if (current(token)) navigating.value = false
  }
}

async function deleteRow() {
  if (
    !props.rowId ||
    creating.value ||
    !canWrite.value ||
    saving.value ||
    dirty.value ||
    relationEditing.value
  )
    return
  const { token, signal } = begin()
  saving.value = true
  error.value = null
  try {
    const result = await request<RowMutation>(
      `/tables/${props.schema.table.id}/rows`,
      'DELETE',
      { ids: [props.rowId] },
      signal,
    )
    if (!current(token)) return
    if (result.rows.length !== 1 || result.rows[0]?.id !== props.rowId)
      throw new Error(t('grid.saveFailed'))
    undo.record([
      { kind: 'restore', tableId: props.schema.table.id, rows: result.rows },
    ])
    deleteDialog.value?.close()
    emit('saved')
    close()
  } catch (cause) {
    if (current(token)) {
      deleteDialog.value?.close()
      failure(cause)
    }
  } finally {
    if (current(token)) saving.value = false
  }
}

function measureWorkspace() {
  workspaceRect.value = props.workspace?.getBoundingClientRect()
}
let workspaceObserver: ResizeObserver | undefined
onMounted(() => {
  window.addEventListener('resize', measureWorkspace)
  window.addEventListener('scroll', measureWorkspace, true)
  if (typeof ResizeObserver !== 'undefined') {
    workspaceObserver = new ResizeObserver(measureWorkspace)
    if (props.workspace) workspaceObserver.observe(props.workspace)
  }
  measureWorkspace()
})
watch(
  () => props.workspace,
  (workspace, previous) => {
    if (previous) workspaceObserver?.unobserve(previous)
    if (workspace) workspaceObserver?.observe(workspace)
    measureWorkspace()
  },
  { flush: 'post' },
)
watch(fullscreen, measureWorkspace)
watch(
  () => props.focusComment,
  (id) => {
    if (id) switchTab('comments')
  },
  { immediate: true },
)
function flushRealtimeReload() {
  if (
    !realtimeReloadPending ||
    disposed ||
    blocked.value ||
    loading.value ||
    saving.value ||
    dirty.value ||
    relationEditing.value ||
    !row.value ||
    props.rowId === null
  )
    return
  realtimeReloadPending = false
  realtimeController?.abort()
  const requestController = new AbortController()
  realtimeController = requestController
  const token = generation
  const rowId = props.rowId
  const signal = AbortSignal.any([
    requestController.signal,
    AbortSignal.timeout(10_000),
  ])
  void request<DataRow>(
    `/tables/${props.schema.table.id}/rows/${encodeURIComponent(rowId)}`,
    'GET',
    undefined,
    signal,
  )
    .then((result) => {
      if (
        !current(token) ||
        requestController.signal.aborted ||
        props.rowId !== rowId
      )
        return
      if (
        loading.value ||
        saving.value ||
        dirty.value ||
        relationEditing.value
      ) {
        realtimeReloadPending = true
        return
      }
      row.value = result
      draftVersion.value++
      changedBy.value = ''
      rememberAttachments()
    })
    .catch((cause: unknown) => {
      if (current(token) && !requestController.signal.aborted) failure(cause)
    })
    .finally(() => {
      if (realtimeController === requestController)
        realtimeController = undefined
    })
}

function handleRealtime(event: RealtimeEvent) {
  const rowId = props.rowId
  if (!rowId) return
  if (event.t === 'row.updated' && event.ids?.includes(rowId)) {
    const fields = event.changes?.[rowId] ?? event.fields
    if (
      dirty.value &&
      (!fields || fields.some((id) => Object.hasOwn(draft.value, id)))
    )
      changedBy.value = event.byName || event.by || t('realtime.unknownEditor')
  } else if (
    !(event.t === 'row.deleted' && event.ids?.includes(rowId)) &&
    event.t !== 'row.bulk' &&
    event.t !== 'resync'
  )
    return
  realtimeReloadPending = true
  flushRealtimeReload()
}

watch(
  [
    () => props.schema.table.id,
    () => props.viewId,
    () => props.rowId,
    () => props.schema.schemaVersion,
    () => props.identity,
    () => auth.user?.id,
  ],
  (next, previous) => {
    if (props.identity === auth.user?.id)
      void load(
        creating.value && previous.length > 0 && next[3] !== previous[3],
      )
    else {
      generation++
      controller?.abort()
      void discardDraftAttachments()
      draftContext = undefined
      row.value = null
      resetDraft()
      relationEditing.value = null
      loading.value = false
      saving.value = false
      relationEditing.value = null
      saved.value = false
      error.value = null
    }
  },
  { immediate: true },
)
watch(
  () => props.schema.myLevel,
  (next, previous) => {
    const lostWrite =
      (previous === 'write' || previous === 'owner') &&
      next !== 'write' &&
      next !== 'owner' &&
      (dirty.value || saving.value || !!relationEditing.value || creating.value)
    if (loading.value) {
      void load()
      if (lostWrite)
        error.value = new RequestError('ERR_PERMISSION_DENIED', 403)
      return
    }
    generation++
    controller?.abort()
    if (realtimeController) {
      realtimeController.abort()
      realtimeController = undefined
      realtimeReloadPending = true
    }
    saving.value = false
    saved.value = false
    if (lostWrite) error.value = new RequestError('ERR_PERMISSION_DENIED', 403)
    if (creating.value || lostWrite) {
      void discardDraftAttachments()
      resetDraft()
      copiedAttachments.clear()
      startDraftContext()
    }
    flushRealtimeReload()
  },
)
watch(
  [
    () => props.schema.table.id,
    () => props.rowId,
    () => props.identity,
    () => auth.user?.id,
  ],
  ([tableId, rowId, identity, userId], _previous, onCleanup) => {
    realtimeController?.abort()
    realtimeController = undefined
    realtimeReloadPending = false
    changedBy.value = ''
    if (rowId && identity === userId)
      onCleanup(realtime.subscribe(tableId, handleRealtime))
  },
  { immediate: true },
)
watch(
  [
    () => props.schema.table.id,
    () => props.rowId,
    () => props.identity,
    () => auth.user?.id,
  ],
  () => {
    activeTab.value = 'comments'
    activityExpanded.value = true
    realtime.setFocus(props.schema.table.id, null)
  },
)
watch([dirty, saving, relationEditing, loading], flushRealtimeReload)
onBeforeUnmount(() => {
  workspaceObserver?.disconnect()
  window.removeEventListener('resize', measureWorkspace)
  window.removeEventListener('scroll', measureWorkspace, true)
  deleteDialog.value?.close()
  realtime.setFocus(props.schema.table.id, null)
  void discardDraftAttachments()
  disposed = true
  generation++
  controller?.abort()
  realtimeController?.abort()
  dialog.value?.close()
})
const relationField = computed(
  () =>
    props.schema.fields.find((field) => field.id === relationEditing.value) ??
    null,
)
const relationLinks = computed(() => {
  const item = fields.value.find(
    (item) => item.field.id === relationEditing.value,
  )
  return item?.value.ok ? item.value.value : null
})

function stageRelation(links: RelationLink[]) {
  if (relationField.value)
    updateDraft(relationField.value.id, links as unknown as WireValue)
}

function focusField(fieldId: string) {
  if (props.rowId && !creating.value)
    realtime.setFocus(props.schema.table.id, { rowId: props.rowId, fieldId })
}

function leaveField(event: FocusEvent) {
  const next = event.relatedTarget
  if (
    !(next instanceof Node) ||
    !(event.currentTarget as HTMLElement).contains(next)
  )
    realtime.setFocus(props.schema.table.id, null)
}
</script>

<template>
  <dialog
    ref="dialog"
    class="row-expand"
    :class="{
      'row-expand-fullscreen': fullscreen,
      'row-expand-no-activity': !creating && !activityExpanded,
    }"
    :style="dialogStyle"
    :aria-label="creating ? title : t('rowExpand.title', { id: rowId })"
    @keydown.capture="saveShortcut"
    @cancel.prevent="close"
  >
    <header>
      <h2>{{ title }}</h2>
      <label class="row-search">
        <AppIcon name="search" :size="16" />
        <input
          v-model="search"
          type="search"
          :aria-label="t('rowExpand.search')"
          :placeholder="t('rowExpand.search')"
        />
      </label>
      <div class="row-actions">
        <button
          v-if="!creating"
          type="button"
          :aria-label="t('rowExpand.previous')"
          :title="t('rowExpand.previous')"
          :disabled="!navigateRow || actionsBlocked"
          @click="navigate(-1)"
        >
          <AppIcon name="chevronUp" />
        </button>
        <button
          v-if="!creating"
          type="button"
          :aria-label="t('rowExpand.next')"
          :title="t('rowExpand.next')"
          :disabled="!navigateRow || actionsBlocked"
          @click="navigate(1)"
        >
          <AppIcon name="chevronDown" />
        </button>
        <button
          v-if="canWrite && !creating"
          type="button"
          :aria-label="t('rowExpand.duplicate')"
          :title="t('rowExpand.duplicate')"
          :disabled="!row || actionsBlocked"
          @click="duplicate"
        >
          <AppIcon name="duplicate" />
        </button>
        <button
          v-if="canWrite && !creating"
          type="button"
          :aria-label="t('rowExpand.delete')"
          :title="t('rowExpand.delete')"
          :disabled="!row || actionsBlocked"
          @click="deleteDialog?.showModal()"
        >
          <AppIcon name="trash" />
        </button>
        <button
          v-if="canWrite"
          type="button"
          :aria-label="t('gridPolish.gp05.save')"
          :title="t('gridPolish.gp05.save')"
          aria-keyshortcuts="Control+Enter Meta+Enter"
          :disabled="blocked || loading || saving || (!creating && !dirty)"
          @click="creating ? create() : save()"
        >
          <AppIcon name="save" />
        </button>
        <button
          v-if="!creating"
          type="button"
          :aria-label="
            activityExpanded
              ? t('rowExpand.collapseActivity')
              : t('rowExpand.expandActivity')
          "
          :title="
            activityExpanded
              ? t('rowExpand.collapseActivity')
              : t('rowExpand.expandActivity')
          "
          :aria-expanded="activityExpanded"
          :aria-controls="`row-activity-${rowId}`"
          @click="activityExpanded = !activityExpanded"
        >
          <AppIcon
            :name="activityExpanded ? 'activityClose' : 'activityOpen'"
          />
        </button>
        <button
          type="button"
          :aria-label="t('rowExpand.fullscreen')"
          :title="t('rowExpand.fullscreen')"
          :aria-pressed="fullscreen"
          :disabled="!workspace"
          @click="fullscreen = !fullscreen"
        >
          <AppIcon :name="fullscreen ? 'exitFullscreen' : 'fullscreen'" />
        </button>
        <button
          type="button"
          :aria-label="t('rowExpand.close')"
          :title="t('rowExpand.close')"
          @click="close"
        >
          <AppIcon name="close" />
        </button>
      </div>
    </header>
    <p v-if="navigationNotice" role="status">{{ navigationNotice }}</p>
    <p v-if="loading" role="status">{{ t('grid.loading') }}</p>
    <p v-if="message" role="alert">{{ message }}</p>
    <button
      v-if="error && !saving && !creating && !row"
      type="button"
      @click="load()"
    >
      {{ t('auth.retry') }}
    </button>
    <p v-if="saved" role="status">{{ t('rowExpand.saved') }}</p>
    <template v-if="row || (creating && canWrite && !blocked)">
      <div class="row-content">
        <div class="row-form">
          <p v-if="!canWrite">{{ t('rowExpand.readOnly') }}</p>
          <p v-if="changedBy" role="status">
            {{ t('realtime.changedBy', { name: changedBy }) }}
          </p>
          <p v-if="creating && error">{{ t('mobileRows.uncertain') }}</p>
          <p v-if="duplicating" role="status">
            {{ t('rowExpand.duplicateDraft') }}
          </p>
          <p v-if="!filteredFields.length">{{ t('rowExpand.noFields') }}</p>
          <section
            v-for="item in availableFields"
            v-show="filteredFieldIds.has(item.field.id)"
            :key="item.field.id"
            class="row-field"
            :class="{ 'row-field-create': creating }"
            :aria-label="item.field.name"
            :data-field-id="item.field.id"
            @focusin="focusField(item.field.id)"
            @focusout="leaveField"
          >
            <CellPresence
              v-if="rowId && !creating"
              :table-id="schema.table.id"
              :row-id="rowId"
              :field-id="item.field.id"
              :identity="identity"
            />
            <h3>
              <AppIcon :name="fieldIconName(item.field.uiType)" :size="16" />
              <span :class="{ 'row-field-label-create': creating }">{{
                item.field.name
              }}</span>
            </h3>
            <div class="row-field-content">
              <p v-if="item.field.description">{{ item.field.description }}</p>
              <template
                v-if="
                  item.field.uiType === 'link' && item.value.ok && !creating
                "
              >
                <div
                  class="row-field-value"
                  :class="{ 'row-field-editable': canWrite }"
                  @click="activateRelation(item.field.id)"
                >
                  <RelationLinks
                    :links="item.value.value"
                    :editable="canWrite && !item.field.isSystem"
                    :disabled="saving"
                    @edit="activateRelation(item.field.id)"
                    @remove="removeRelationLink(item.field.id, $event)"
                  />
                </div>
              </template>
              <template v-else-if="item.result.ok && item.value.ok">
                <fieldset
                  v-if="canEditField(item)"
                  class="row-field-editor"
                  :data-field-type="item.field.uiType"
                  :disabled="saving"
                >
                  <legend class="row-tab-label">{{ item.field.name }}</legend>
                  <FieldEditor
                    :key="`${creating}-${draftVersion}-${item.field.id}`"
                    :ref="(editor) => setDraftEditor(item.field.id, editor)"
                    :field="item.result.value"
                    :value="
                      defaultPlaceholder(item) === undefined
                        ? item.value.value
                        : null
                    "
                    :env="env"
                    :default-placeholder="defaultPlaceholder(item)"
                    :label="item.field.name"
                    :required="item.field.isRequired"
                    :auto-focus="false"
                    :live="true"
                    :table-id="schema.table.id"
                    :field-id="item.field.id"
                    :preserve-uploads="
                      preserveUploads || Object.hasOwn(draft, item.field.id)
                    "
                    :identity="identity"
                    :resolve-user="resolveUser"
                    @draft-changed="pendingVersion++"
                    @preview="updateDraft(item.field.id, $event)"
                    @invalid-value="invalidDraftFields.add(item.field.id)"
                    @invalid-draft="invalidDraftFields.add(item.field.id)"
                    @commit="updateDraft(item.field.id, $event)"
                    @cancel="close"
                  />
                </fieldset>
                <div v-else class="row-field-value">
                  <FieldRenderer
                    :field="item.result.value"
                    :value="item.value.value"
                    :env="env"
                    :table-id="schema.table.id"
                    :identity="identity"
                    :resolve-user="resolveUser"
                  />
                  <span
                    v-if="item.field.isSystem || item.result.value.readOnly"
                    v-text="t('rowExpand.system')"
                  />
                </div>
              </template>
              <p v-else role="alert">{{ t('grid.unsupportedField') }}</p>
            </div>
          </section>
        </div>
        <aside
          v-if="row && rowId && !creating && !blocked"
          v-show="activityExpanded"
          :id="`row-activity-${rowId}`"
          class="row-activity"
        >
          <div
            class="row-tabs"
            role="tablist"
            :aria-label="t('history.tabs')"
            @keydown="tabKey"
          >
            <button
              :id="`row-comments-tab-${rowId}`"
              ref="commentsTab"
              type="button"
              role="tab"
              :aria-selected="activeTab === 'comments'"
              :aria-controls="`row-comments-panel-${rowId}`"
              :tabindex="activeTab === 'comments' ? 0 : -1"
              @click="switchTab('comments')"
            >
              <AppIcon name="comments" />
              {{ t('comments.title') }}
            </button>
            <button
              :id="`row-history-tab-${rowId}`"
              ref="historyTab"
              type="button"
              role="tab"
              :aria-selected="activeTab === 'history'"
              :aria-controls="`row-history-panel-${rowId}`"
              :tabindex="activeTab === 'history' ? 0 : -1"
              @click="switchTab('history')"
            >
              <AppIcon name="history" />
              {{ t('history.title') }}
            </button>
          </div>
          <div
            v-show="activeTab === 'comments'"
            :id="`row-comments-panel-${rowId}`"
            class="row-overlay"
            role="tabpanel"
            :aria-labelledby="`row-comments-tab-${rowId}`"
          >
            <CommentPanel
              :table-id="schema.table.id"
              :view-id="viewId"
              :row-id="rowId"
              :identity="identity"
              :level="schema.myLevel"
              :focus-comment="focusComment"
              @changed="emit('saved')"
              @schema-changed="emit('schemaChanged')"
            />
          </div>
          <HistoryPanel
            v-if="
              row && rowId && !creating && activeTab === 'history' && !blocked
            "
            :id="`row-history-panel-${rowId}`"
            class="row-overlay"
            role="tabpanel"
            :aria-labelledby="`row-history-tab-${rowId}`"
            :table-id="schema.table.id"
            :row-id="rowId"
            :identity="identity"
            :fields="schema.fields"
            @schema-changed="emit('schemaChanged')"
          />
        </aside>
      </div>
    </template>
    <div data-temporal-portal-root />
  </dialog>
  <dialog
    ref="deleteDialog"
    class="row-delete-dialog"
    :aria-label="t('rowExpand.delete')"
    @cancel.prevent="!saving && deleteDialog?.close()"
  >
    <h2>{{ t('rowExpand.delete') }}</h2>
    <p>{{ t('rowExpand.confirmDelete', { title }) }}</p>
    <button type="button" :disabled="saving" @click="deleteRow">
      {{ t('rowExpand.delete') }}
    </button>
    <button type="button" :disabled="saving" @click="deleteDialog?.close()">
      {{ t('admin.cancel') }}
    </button>
  </dialog>
  <RelationPicker
    :open="relationField !== null"
    :schema="schema"
    :field="relationField"
    :row-id="row?.id ?? null"
    :links="relationLinks"
    :identity="identity"
    :draft-mode="true"
    @draft-changed="stageRelation"
    @close="relationEditing = null"
  />
</template>

<style scoped>
.row-expand {
  width: min(1120px, calc(100vw - 32px));
  height: min(780px, calc(100dvh - 48px));
  max-height: calc(100dvh - 48px);
  overflow: hidden;
  padding: 0;
  border: 1px solid var(--border);
  border-radius: 12px;
  background: var(--surface);
  color: var(--text);
}
.row-expand[open] {
  display: flex;
  flex-direction: column;
}
.row-expand-no-activity {
  width: min(740px, calc((100vw - 32px) * 0.66));
}
.row-expand-fullscreen {
  position: fixed;
  max-width: none;
  max-height: none;
  margin: 0;
  border-radius: 0;
}
.row-expand-fullscreen::backdrop {
  background: transparent;
}
.row-content {
  display: flex;
  position: relative;
  flex: 1;
  min-height: 0;
  overflow: auto;
}
.row-form,
.row-overlay {
  padding: 16px 24px;
}
.row-form {
  flex: 1;
  min-width: 0;
  overflow: auto;
}
.row-activity {
  flex: 0 0 34%;
  min-width: 0;
  overflow: auto;
  border-left: 1px solid var(--border);
}
.row-overlay {
  min-height: 100%;
  background: var(--surface);
}
.row-search {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  min-width: 0;
  max-width: 220px;
}
.row-search input {
  width: 100%;
  min-width: 0;
  border: 0;
  background: transparent;
  color: var(--text);
}
.row-tab-label {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip-path: inset(50%);
}
.row-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 4px;
}
.row-actions button,
.row-tabs button {
  display: inline-flex;
  padding: 6px;
}
.row-expand::backdrop {
  background: #0008;
}
header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 12px 16px;
}
header h2 {
  min-width: 0;
  font-size: 16px;
  line-height: 1.4;
  flex: 1;
}
.row-tabs {
  display: flex;
  gap: 8px;
  margin: 0 0;
  border-bottom: 1px solid var(--border);
}
.row-tabs [aria-selected='true'] {
  font-weight: 700;
  border-bottom-width: 3px;
}
h2,
h3 {
  margin: 0;
  overflow-wrap: anywhere;
}
.row-field {
  position: relative;
  display: grid;
  gap: 8px;
  padding: 16px 0;
}
.row-field h3 {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 500;
  color: var(--muted);
}
.row-field-create {
  grid-template-columns: minmax(8rem, 3fr) minmax(0, 7fr);
  align-items: start;
  column-gap: 16px;
}
.row-field-create h3 {
  padding-top: 8px;
}
.row-field-label-create::after {
  content: ':';
}
.row-field-content {
  min-width: 0;
}
.row-field-content > p {
  margin: 0 0 8px;
}
.row-field-editor {
  margin: 0;
  padding: 0;
  border: 0;
}
.row-field-value {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 8px;
  padding: 8px 10px;
  border: 1px solid var(--border);
  border-radius: 6px;
}
.row-field-editable {
  min-height: 36px;
  cursor: pointer;
}
.row-field-editable:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 2px;
}
.row-field-value :deep(.field-value) {
  flex: 1;
  min-width: 0;
}
.row-field-value > span {
  font-size: 11px;
  color: var(--muted);
}
.row-delete-dialog {
  max-width: min(420px, calc(100vw - 32px));
  padding: 20px;
  background: var(--surface);
  color: var(--text);
  border: 1px solid var(--border);
  border-radius: 8px;
}
fieldset {
  min-width: 0;
}
button {
  padding: 6px 12px;
  color: var(--text);
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 4px;
  cursor: pointer;
}
button:disabled {
  cursor: wait;
  opacity: 0.6;
}
.row-field :deep(.field-value) {
  overflow-wrap: anywhere;
  white-space: pre-wrap;
}
@media (max-width: 479px) {
  .row-field-create {
    grid-template-columns: minmax(0, 1fr);
  }
}
@media (max-width: 767px) {
  .row-content {
    display: block;
  }
  .row-form {
    overflow: visible;
  }
  .row-activity {
    border-left: 0;
    border-top: 1px solid var(--border);
  }
  .row-expand {
    width: 100%;
    max-width: 100%;
    height: 100dvh;
    max-height: 100dvh;
    margin: 0;
    padding: 0 0 max(16px, env(safe-area-inset-bottom));
    border: 0;
    border-radius: 0;
  }
  header {
    flex-wrap: wrap;
    position: sticky;
    z-index: 1;
    top: 0;
    padding: 12px 0;
    background: var(--surface);
    border-bottom: 1px solid var(--border);
  }
  .row-actions {
    width: 100%;
  }
  button,
  :deep(.field-editor input),
  :deep(.field-editor select),
  :deep(.field-editor button) {
    min-height: 44px;
    font-size: 16px;
  }
  .row-field p {
    overflow-wrap: anywhere;
  }
  :deep(.field-editor input[type='checkbox']) {
    min-width: 44px;
  }
}
</style>
