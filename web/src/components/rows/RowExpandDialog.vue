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
import { patchUndoStep, useUndoStore } from '../../stores/undo'
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
const detailsTab = ref<HTMLButtonElement>()
const historyTab = ref<HTMLButtonElement>()
const commentsTab = ref<HTMLButtonElement>()
const activeTab = ref<'details' | 'comments' | 'history'>('details')
const search = ref('')
const compact = ref(false)
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
    editing.value !== null ||
    relationEditing.value !== null,
)
const row = ref<DataRow | null>(null)
const loading = ref(false)
const saving = ref(false)
const error = ref<unknown>(null)
const editing = ref<string | null>(null)
const changedBy = ref('')
const relationEditing = ref<string | null>(null)
const saved = ref(false)
const preserveUploads = ref(false)
const draft = ref<Record<string, WireValue>>({})
const blocked = ref(false)
const creating = computed(() => props.rowId === null || duplicating.value)
const title = computed(() => {
  const primary = fields.value.find((item) => !item.field.isSystem)
  const label =
    primary?.result.ok && primary.value.ok
      ? primary.result.value.formatDisplay(primary.value.value, env.value)
      : ''
  return (
    label ||
    (creating.value
      ? t('mobileRows.newRow')
      : t('rowExpand.title', { id: props.rowId }))
  )
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
      const value = creating.value
        ? Object.hasOwn(draft.value, field.id)
          ? draft.value[field.id]
          : field.default
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
const filteredFields = computed(() => {
  const query = search.value.trim().toLocaleLowerCase(locale.value)
  return fields.value.filter(
    (item) =>
      !query ||
      item.field.name.toLocaleLowerCase(locale.value).includes(query) ||
      (item.result.ok &&
        item.value.ok &&
        item.result.value
          .formatDisplay(item.value.value, env.value)
          .toLocaleLowerCase(locale.value)
          .includes(query)),
  )
})
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

function failure(cause: unknown) {
  error.value = cause
  saved.value = false
  preserveUploads.value = false
  draftCleanupStarted = false
  if (!(cause instanceof RequestError)) return
  if (cause.code === 'ERR_PASSWORD_CHANGE_REQUIRED') {
    blocked.value = true
    row.value = null
    draft.value = {}
    editing.value = null
    void router.replace('/change-password')
  } else if ([401, 403, 404].includes(cause.status)) {
    blocked.value = true
    row.value = null
    editing.value = null
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
    editing.value = null
    emit('schemaChanged')
  }
}

async function load(schemaChanged = false) {
  const { token, signal } = begin()
  row.value = null
  draft.value = {}
  duplicating.value = false
  copiedAttachments.clear()
  search.value = ''
  navigationNotice.value = ''
  blocked.value = false
  editing.value = null
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
    if (current(token)) row.value = result
  } catch (cause) {
    if (current(token)) failure(cause)
  } finally {
    if (current(token)) loading.value = false
  }
}

async function save(value: WireValue) {
  if (
    !canWrite.value ||
    saving.value ||
    (!row.value && !creating.value) ||
    !editing.value
  )
    return
  const item = fields.value.find((item) => item.field.id === editing.value)
  if (!item?.result.ok || item.field.isSystem || item.result.value.readOnly)
    return
  if (creating.value) {
    draft.value = { ...draft.value, [item.field.id]: value }
    preserveUploads.value = true
    await nextTick()
    editing.value = null
    error.value = null
    return
  }
  if (props.rowId === null) return
  const body = { fields: { [item.field.id]: value } }
  const previous = row.value?.fields[item.field.id]
  if (previous === undefined) {
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
    undo.record([
      patchUndoStep(
        props.schema.table.id,
        [
          {
            id: result.row.id,
            fields: result.previous ?? { [item.field.id]: previous },
          },
        ],
        [result.row],
      ),
    ])
    row.value = result.row
    preserveUploads.value = true
    await nextTick()
    editing.value = null
    saved.value = true
    emit('saved')
  } catch (cause) {
    if (current(token)) failure(cause)
  } finally {
    if (current(token)) saving.value = false
  }
}

async function create() {
  if (
    !creating.value ||
    !canWrite.value ||
    blocked.value ||
    saving.value ||
    editing.value
  )
    return
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
    draft.value = {}
    emit('created', created.id)
  } catch (cause) {
    if (current(token)) failure(cause)
  } finally {
    if (current(token)) saving.value = false
  }
}

function startEditing(id: string) {
  preserveUploads.value = false
  editing.value = id
  saved.value = false
  error.value = null
}

function canEditField(isSystem: boolean, readOnly: boolean) {
  return canWrite.value && !isSystem && !readOnly
}

async function discardDraftAttachments() {
  if (!creating.value || draftCleanupStarted) return
  draftCleanupStarted = true
  const ids = new Map<string, Set<string>>()
  for (const field of props.schema.fields) {
    if (field.uiType !== 'attachment') continue
    const value = draft.value[field.id]
    if (!Array.isArray(value)) continue
    for (const item of value) {
      if (
        typeof item === 'object' &&
        item !== null &&
        'id' in item &&
        typeof item.id === 'string' &&
        !copiedAttachments.has(item.id)
      )
        ids.set(field.id, new Set([...(ids.get(field.id) ?? []), item.id]))
    }
  }
  await Promise.all(
    [...ids].flatMap(([fieldId, attachmentIds]) =>
      [...attachmentIds].map((attachmentId) =>
        discardAttachment(props.schema.table.id, fieldId, attachmentId).catch(
          () => {},
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

function switchTab(tab: 'details' | 'comments' | 'history', focus = false) {
  activeTab.value = tab
  realtime.setFocus(props.schema.table.id, null)
  if (focus)
    void nextTick(() =>
      (tab === 'details'
        ? detailsTab.value
        : tab === 'comments'
          ? commentsTab.value
          : historyTab.value
      )?.focus(),
    )
}

function tabKey(event: KeyboardEvent) {
  if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
    event.preventDefault()
    const tabs = ['details', 'comments', 'history'] as const
    const index = tabs.indexOf(activeTab.value)
    switchTab(tabs[(index + (event.key === 'ArrowRight' ? 1 : 2)) % 3]!, true)
  } else if (event.key === 'Home') {
    event.preventDefault()
    switchTab('details', true)
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
    editing.value ||
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
  draft.value = values
  activeTab.value = 'details'
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
    editing.value ||
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
    editing.value ||
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
function relationUpdated(updated: DataRow) {
  row.value = updated
  saved.value = true
  emit('saved')
}

function flushRealtimeReload() {
  if (
    !realtimeReloadPending ||
    disposed ||
    blocked.value ||
    loading.value ||
    saving.value ||
    editing.value ||
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
        editing.value ||
        relationEditing.value
      ) {
        realtimeReloadPending = true
        return
      }
      row.value = result
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
    if (editing.value && (!fields || fields.includes(editing.value)))
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
      row.value = null
      draft.value = {}
      editing.value = null
      loading.value = false
      saving.value = false
      saved.value = false
      error.value = null
    }
  },
  { immediate: true },
)
watch(
  () => props.schema.myLevel,
  (next, previous) => {
    if (loading.value) {
      void load()
      return
    }
    const lostWrite =
      (previous === 'write' || previous === 'owner') &&
      next !== 'write' &&
      next !== 'owner' &&
      (!!editing.value || saving.value || !!relationEditing.value)
    generation++
    controller?.abort()
    if (realtimeController) {
      realtimeController.abort()
      realtimeController = undefined
      realtimeReloadPending = true
    }
    saving.value = false
    editing.value = null
    saved.value = false
    if (lostWrite) error.value = new RequestError('ERR_PERMISSION_DENIED', 403)
    if (creating.value) draft.value = {}
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
watch(editing, () => {
  changedBy.value = ''
})
watch(
  [
    () => props.schema.table.id,
    () => props.rowId,
    () => props.identity,
    () => auth.user?.id,
  ],
  () => {
    activeTab.value = props.focusComment ? 'comments' : 'details'
    realtime.setFocus(props.schema.table.id, null)
  },
)
watch([editing, saving, relationEditing, loading], flushRealtimeReload)
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
      'row-expand-compact': compact,
    }"
    :style="dialogStyle"
    :aria-label="creating ? title : t('rowExpand.title', { id: rowId })"
    @cancel.prevent="
      activeTab === 'details' ? close() : switchTab('details', true)
    "
  >
    <header>
      <h2>{{ title }}</h2>
      <div class="row-actions">
        <button
          type="button"
          :aria-label="t('rowExpand.previous')"
          :title="t('rowExpand.previous')"
          :disabled="creating || !navigateRow || actionsBlocked"
          @click="navigate(-1)"
        >
          <AppIcon name="chevronUp" />
        </button>
        <button
          type="button"
          :aria-label="t('rowExpand.next')"
          :title="t('rowExpand.next')"
          :disabled="creating || !navigateRow || actionsBlocked"
          @click="navigate(1)"
        >
          <AppIcon name="chevronDown" />
        </button>
        <button
          type="button"
          :aria-label="t('rowExpand.compact')"
          :title="t('rowExpand.compact')"
          :aria-pressed="compact"
          @click="compact = !compact"
        >
          <AppIcon name="compact" />
        </button>
        <button
          v-if="canWrite && !creating"
          type="button"
          :aria-label="t('rowExpand.duplicate')"
          :title="t('rowExpand.duplicate')"
          :disabled="
            !row || saving || editing !== null || relationEditing !== null
          "
          @click="duplicate"
        >
          <AppIcon name="duplicate" />
        </button>
        <button
          v-if="canWrite && !creating"
          type="button"
          :aria-label="t('rowExpand.delete')"
          :title="t('rowExpand.delete')"
          :disabled="
            !row || saving || editing !== null || relationEditing !== null
          "
          @click="deleteDialog?.showModal()"
        >
          <AppIcon name="trash" />
        </button>
        <button
          type="button"
          :aria-label="t('rowExpand.fullscreen')"
          :title="t('rowExpand.fullscreen')"
          :aria-pressed="fullscreen"
          :disabled="!workspace"
          @click="fullscreen = !fullscreen"
        >
          <AppIcon name="fullscreen" />
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
    <button v-if="error && !saving && !creating" type="button" @click="load()">
      {{ t('auth.retry') }}
    </button>
    <p v-if="saved" role="status">{{ t('rowExpand.saved') }}</p>
    <template v-if="row || (creating && canWrite && !blocked)">
      <div class="row-toolbar">
        <label class="row-search">
          <AppIcon name="search" :size="16" />
          <input
            v-model="search"
            type="search"
            :aria-label="t('rowExpand.search')"
            :placeholder="t('rowExpand.search')"
            :disabled="activeTab !== 'details'"
          />
        </label>
        <div
          v-if="row && rowId && !creating"
          class="row-tabs"
          role="tablist"
          :aria-label="t('history.tabs')"
          @keydown="tabKey"
        >
          <button
            :id="`row-details-tab-${rowId}`"
            ref="detailsTab"
            type="button"
            role="tab"
            :aria-selected="activeTab === 'details'"
            :aria-controls="`row-details-panel-${rowId}`"
            :tabindex="activeTab === 'details' ? 0 : -1"
            @click="switchTab('details')"
          >
            <AppIcon name="view" />
            <span class="row-tab-label">{{ t('history.details') }}</span>
          </button>
          <button
            :id="`row-comments-tab-${rowId}`"
            ref="commentsTab"
            type="button"
            role="tab"
            :aria-selected="activeTab === 'comments'"
            :aria-controls="`row-comments-panel-${rowId}`"
            :tabindex="activeTab === 'comments' ? 0 : -1"
            :title="t('comments.title')"
            @click="switchTab('comments')"
          >
            <AppIcon name="comments" />
            <span class="row-tab-label">{{ t('comments.title') }}</span>
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
            <span class="row-tab-label">{{ t('history.title') }}</span>
          </button>
        </div>
        <button
          v-if="activeTab !== 'details'"
          type="button"
          :aria-label="t('rowExpand.returnToFields')"
          :title="t('rowExpand.returnToFields')"
          @click="switchTab('details', true)"
        >
          <AppIcon name="close" />
        </button>
      </div>
      <div class="row-content">
        <div
          v-show="creating || activeTab === 'details'"
          :id="rowId ? `row-details-panel-${rowId}` : undefined"
          class="row-form"
          :role="rowId ? 'tabpanel' : undefined"
          :aria-labelledby="rowId ? `row-details-tab-${rowId}` : undefined"
        >
          <p v-if="!canWrite">{{ t('rowExpand.readOnly') }}</p>
          <p>{{ creating ? t('mobileRows.defaults') : t('rowExpand.hint') }}</p>
          <p v-if="creating && error">{{ t('mobileRows.uncertain') }}</p>
          <p v-if="duplicating" role="status">
            {{ t('rowExpand.duplicateDraft') }}
          </p>
          <p v-if="!filteredFields.length">{{ t('rowExpand.noFields') }}</p>
          <section
            v-for="item in filteredFields"
            :key="item.field.id"
            class="row-field"
            :aria-label="item.field.name"
            :data-field-id="item.field.id"
            @focusin="focusField(item.field.id)"
            @focusout="leaveField"
          >
            <CellPresence
              v-if="rowId"
              :table-id="schema.table.id"
              :row-id="rowId"
              :field-id="item.field.id"
              :identity="identity"
            />
            <h3>
              <AppIcon :name="fieldIconName(item.field.uiType)" :size="16" />{{
                item.field.name
              }}
            </h3>
            <p v-if="item.field.description">{{ item.field.description }}</p>
            <p v-if="creating && item.field.isSystem">
              {{ t('rowExpand.system') }}
            </p>
            <template
              v-else-if="
                item.field.uiType === 'link' && item.value.ok && !creating
              "
            >
              <RelationLinks
                :links="item.value.value"
                :editable="canWrite"
                @edit="relationEditing = item.field.id"
              />
            </template>
            <template v-else-if="item.result.ok && item.value.ok">
              <template v-if="editing === item.field.id && canWrite">
                <fieldset :disabled="saving">
                  <legend>{{ item.field.name }}</legend>
                  <p v-if="changedBy" role="status">
                    {{ t('realtime.changedBy', { name: changedBy }) }}
                  </p>
                  <FieldEditor
                    :field="item.result.value"
                    :value="item.value.value"
                    :env="env"
                    :table-id="schema.table.id"
                    :field-id="item.field.id"
                    :preserve-uploads="preserveUploads"
                    :identity="identity"
                    :resolve-user="resolveUser"
                    :submit-label="
                      creating ? t('field.editor.apply') : t('rowExpand.save')
                    "
                    @commit="save"
                    @cancel="editing = null"
                  />
                  <button type="button" @click="editing = null">
                    {{ t('admin.cancel') }}
                  </button>
                </fieldset>
              </template>
              <template v-else>
                <div class="row-field-value">
                  <FieldRenderer
                    :field="item.result.value"
                    :value="item.value.value"
                    :env="env"
                    :table-id="schema.table.id"
                    :identity="identity"
                    :resolve-user="resolveUser"
                  />
                  <button
                    v-if="
                      canEditField(
                        item.field.isSystem,
                        item.result.value.readOnly,
                      )
                    "
                    type="button"
                    :disabled="saving"
                    @click="startEditing(item.field.id)"
                  >
                    <AppIcon name="edit" :size="16" />
                    <span class="row-tab-label">{{
                      t('rowExpand.edit', { name: item.field.name })
                    }}</span>
                  </button>
                  <span
                    v-else-if="
                      item.field.isSystem || item.result.value.readOnly
                    "
                    v-text="t('rowExpand.system')"
                  />
                </div>
              </template>
            </template>
            <p v-else role="alert">{{ t('grid.unsupportedField') }}</p>
          </section>
        </div>
        <div
          v-if="row && rowId && !creating && !blocked"
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
      </div>
      <button
        v-if="creating"
        class="create-row"
        type="button"
        :disabled="saving || editing !== null"
        @click="create"
      >
        {{
          saving
            ? t('grid.saving')
            : duplicating
              ? t('rowExpand.saveDuplicate')
              : t('mobileRows.create')
        }}
      </button>
    </template>
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
    :links="relationField && row ? row.fields[relationField.id] : null"
    :identity="identity"
    @updated="relationUpdated"
    @close="relationEditing = null"
  />
</template>

<style scoped>
.row-expand {
  width: min(760px, calc(100vw - 32px));
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
  position: relative;
  flex: 1;
  min-height: 0;
  overflow: auto;
}
.row-form,
.row-overlay {
  padding: 16px 24px;
}
.row-overlay {
  min-height: 100%;
  background: var(--surface);
}
.row-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 16px;
  border-block: 1px solid var(--border);
}
.row-search {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  min-width: 0;
  max-width: 65%;
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
.row-field-value {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 8px;
  padding: 8px 10px;
  border: 1px solid var(--border);
  border-radius: 6px;
}
.row-field-value :deep(.field-value) {
  flex: 1;
  min-width: 0;
}
.row-field-value > button {
  flex-shrink: 0;
  display: inline-flex;
  padding: 2px;
  border: 0;
}
.row-field-value > span {
  font-size: 11px;
  color: var(--muted);
}
.row-expand-compact .row-field-value {
  border: 0;
  padding: 4px 0;
}
.row-expand-compact .row-field h3 {
  text-transform: uppercase;
}
.row-expand-compact .row-field h3 .app-icon {
  display: none;
}
.create-row {
  margin: 12px 24px;
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
@media (max-width: 767px) {
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
    position: sticky;
    z-index: 1;
    top: 0;
    padding: 12px 0;
    background: var(--surface);
    border-bottom: 1px solid var(--border);
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
