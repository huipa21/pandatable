<script setup lang="ts">
import {
  computed,
  defineComponent,
  h,
  markRaw,
  onBeforeUnmount,
  onMounted,
  ref,
} from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import RevoGrid, { VGridVueEditor } from '@revolist/vue3-datagrid'
import type { EditorType } from '@revolist/vue3-datagrid'
import type {
  BeforeSaveDataDetails,
  ColumnRegular,
  Components,
  ViewPortScrollEvent,
} from '@revolist/revogrid'
import { errorMessage, request, RequestError } from '../../api/client'
import type {
  DataRow,
  RowMutation,
  RowQueryRequest,
  RowQueryResult,
  RowValue,
  TableField,
  TableSchema,
} from '../../api/types'
import { bindField, type BoundField } from '../../fields/registry'
import { fieldErrorMessage } from '../../fields/messages'
import type { FieldEnv, WireValue } from '../../fields/types'
import FieldRenderer from '../fields/FieldRenderer.vue'
import { useAuthStore } from '../../stores/auth'
import { useNavStore } from '../../stores/nav'
import { usePreferencesStore } from '../../stores/preferences'
import GridCellEditor from './GridCellEditor.vue'
import { RemoteRowsWindow, remotePageSize } from './remoteWindow'

const props = defineProps<{
  schema: TableSchema
  viewId: string
  identity: string
}>()
const emit = defineEmits<{ schemaChanged: [] }>()
const { t, locale } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const nav = useNavStore()
const preferences = usePreferencesStore()
const host = ref<{ $el: HTMLElement & Components.RevoGrid }>()
const ready = ref(false)
const disposed = ref(false)
const loading = ref(false)
const saving = ref(false)
const error = ref<unknown>(null)
const cellErrors = ref(new Map<string, string>())
const visibleFieldIds = ref<string[]>([])
const fieldOverride = ref<string[] | null>(null)
const selectedRows = ref(new Set<string>())
const deleteDialog = ref<HTMLDialogElement>()
const gridSource = markRaw([] as GridModel[])
const syncedPages = new Set<number>()
const mutationControllers = new Set<AbortController>()
const desktop = ref(window.matchMedia('(min-width: 768px)').matches)
const media = window.matchMedia('(min-width: 768px)')
const resized = (event: MediaQueryListEvent) => {
  desktop.value = event.matches
}
const fieldEnvironment = computed<FieldEnv>(() => ({
  timeZone: auth.user?.effectiveTimezone,
  locale: locale.value,
}))

const fieldById = computed(() => {
  const result = new Map<string, { field: TableField; bound: BoundField }>()
  for (const field of props.schema.fields) {
    const bound = bindField(field.uiType, field.options)
    if (bound.ok) result.set(field.id, { field, bound: bound.value })
  }
  return result
})
const gridError = computed(() =>
  typeof error.value === 'string'
    ? error.value
    : error.value instanceof Error
      ? errorMessage(error.value)
      : '',
)
const canWrite = computed(
  () =>
    props.identity === auth.user?.id &&
    (props.schema.myLevel === 'write' || props.schema.myLevel === 'owner'),
)
const allFields = computed(() =>
  [...props.schema.fields].sort(
    (left, right) => left.position - right.position,
  ),
)
const unsupportedField = computed(() =>
  props.schema.fields.some((field) => !fieldById.value.has(field.id)),
)

function fetchPage(
  offset: number,
  limit: number,
  includeCount: boolean,
  signal: AbortSignal,
): Promise<RowQueryResult> {
  if (props.identity !== auth.user?.id)
    throw new Error('Grid identity changed before query')
  const body: RowQueryRequest = {
    offset,
    limit,
    includeCount,
    ...(fieldOverride.value === null ? {} : { fieldIds: fieldOverride.value }),
  }
  return request<RowQueryResult>(
    `/views/${props.viewId}/query`,
    'POST',
    body,
    AbortSignal.any([signal, AbortSignal.timeout(10_000)]),
  ).then((result) => {
    if (result.schemaVersion !== props.schema.schemaVersion) {
      emit('schemaChanged')
      throw new Error('Grid schema changed during query')
    }
    if (fieldOverride.value === null) visibleFieldIds.value = result.fieldIds
    return result
  })
}

const rows = new RemoteRowsWindow(
  { fetch: fetchPage },
  () => {
    if (!ready.value || disposed.value) return
    if (rows.snapshot().cachedRows > 0) loading.value = false
    syncSource()
  },
  (cause) => {
    if (cause instanceof RequestError && [401, 403, 404].includes(cause.status))
      handleAccessFailure(cause)
    error.value = cause
    loading.value = false
  },
)

interface GridModel {
  __rowId: string
  [key: string]: RowValue | string
}

function wireValue(value: unknown): WireValue {
  if (
    value === null ||
    typeof value === 'string' ||
    typeof value === 'number' ||
    typeof value === 'boolean' ||
    (Array.isArray(value) && value.every((item) => typeof item === 'string'))
  )
    return value as WireValue
  return null
}

function boundFor(id: string) {
  return fieldById.value.get(id)
}

const columns = computed<ColumnRegular[]>(() => {
  const definitions: ColumnRegular[] = [
    {
      prop: '__selected',
      name: '',
      size: 44,
      pin: 'colPinStart',
      readonly: true,
      cellTemplate: (render, { model }) =>
        model.__rowId
          ? render('input', {
              type: 'checkbox',
              checked: selectedRows.value.has(String(model.__rowId)),
              'aria-label': t('grid.selectRow', { id: model.__rowId }),
              onChange: (event: Event) =>
                selectRow(
                  String(model.__rowId),
                  (event.target as HTMLInputElement).checked,
                ),
            })
          : null,
    },
  ]
  for (const id of visibleFieldIds.value) {
    const item = boundFor(id)
    if (!item) continue
    const { field, bound } = item
    definitions.push({
      prop: field.id,
      name: field.name,
      size: 180,
      pin: field.uiType === 'id' ? 'colPinStart' : undefined,
      readonly: !canWrite.value || bound.readOnly,
      editor: bound.readOnly ? undefined : field.id,
      cellTemplate: (render, { model }) => {
        const rowId = String(model.__rowId ?? '')
        const value = wireValue(model[field.id])
        const message = cellErrors.value.get(`${rowId}:${field.id}`)
        const cell = render(FieldRenderer, {
          field: bound,
          value,
          env: fieldEnvironment.value,
        })
        return message
          ? render(
              'div',
              { class: 'grid-cell-invalid', 'aria-invalid': 'true' },
              [cell, render('span', { role: 'alert' }, message)],
            )
          : cell
      },
    })
  }
  return definitions
})

const editors = computed(() =>
  Object.fromEntries(
    visibleFieldIds.value.flatMap((id) => {
      const item = boundFor(id)
      if (!item || item.bound.readOnly) return []
      const editor = defineComponent({
        setup(editorProps: EditorType) {
          return () =>
            h(GridCellEditor, {
              ...editorProps,
              column: {
                ...editorProps.column,
                boundField: item.bound,
                fieldEnv: fieldEnvironment.value,
              },
            })
        },
      })
      return [[id, VGridVueEditor(editor)]]
    }),
  ),
)

function modelFor(row: DataRow | null): GridModel {
  return row ? { __rowId: row.id, ...row.fields } : { __rowId: '' }
}

function mutate<T>(
  path: string,
  method: 'POST' | 'PATCH' | 'DELETE',
  body: unknown,
) {
  const controller = new AbortController()
  mutationControllers.add(controller)
  return request<T>(
    path,
    method,
    body,
    AbortSignal.any([controller.signal, AbortSignal.timeout(10_000)]),
  ).finally(() => mutationControllers.delete(controller))
}

function syncSource() {
  if (gridSource.length !== rows.slots.length) {
    gridSource.length = rows.slots.length
    gridSource.fill(modelFor(null))
    syncedPages.clear()
  }
  const cachedPages = new Set(rows.snapshot().cachedPages)
  for (const page of syncedPages) {
    if (cachedPages.has(page)) continue
    const start = page * remotePageSize
    const end = Math.min(start + remotePageSize, gridSource.length)
    gridSource.fill(modelFor(null), start, end)
  }
  for (const page of cachedPages) {
    const start = page * remotePageSize
    const end = Math.min(start + remotePageSize, rows.slots.length)
    for (let index = start; index < end; index++) {
      gridSource[index] = modelFor(rows.rowAt(index))
    }
  }
  syncedPages.clear()
  for (const page of cachedPages) syncedPages.add(page)
  void refreshSource()
}

async function refreshSource() {
  try {
    const store = await host.value?.$el.getSourceStore('rgRow')
    if (store && !disposed.value) store.set('source', gridSource.slice())
  } catch (cause) {
    if (!disposed.value) {
      error.value = cause
      loading.value = false
    }
  }
}

async function initialize(anchor = 0) {
  loading.value = true
  error.value = null
  cellErrors.value.clear()
  try {
    await rows.initialize(anchor)
    if (!disposed.value && !error.value) loading.value = false
  } catch (cause) {
    if (!disposed.value) {
      error.value = cause
      loading.value = false
    }
  }
}

function created() {
  ready.value = true
  void initialize()
}

function scroll(event: CustomEvent<ViewPortScrollEvent>) {
  if (event.detail.dimension !== 'rgRow') return
  const index = Math.max(0, Math.floor(event.detail.coordinate / 32))
  rows.load(index)
  loading.value = rows.snapshot().cachedRows === 0 && rows.total > 0
}

function selectRow(id: string, selected: boolean) {
  const next = new Set(selectedRows.value)
  if (selected) {
    if (next.size >= 1000) {
      error.value = t('grid.selectionLimit')
      return
    }
    next.add(id)
  } else next.delete(id)
  selectedRows.value = next
}

function handleAccessFailure(failure: RequestError) {
  if (failure.status === 401 || failure.code === 'ERR_UNAUTHENTICATED') {
    auth.setUser(null)
    void routerReplaceLogin()
  } else {
    void nav.refresh(true)
  }
}

async function routerReplaceLogin() {
  await router.replace('/login')
}

async function edit(event: CustomEvent<BeforeSaveDataDetails>) {
  event.preventDefault()
  if (!canWrite.value || disposed.value) return
  if (typeof event.detail.prop !== 'string') return
  const item = boundFor(event.detail.prop)
  const row = rows.rowAt(event.detail.rowIndex)
  if (!item || !row || item.bound.readOnly) return
  const normalized = item.bound.normalize(event.detail.val)
  const key = `${row.id}:${item.field.id}`
  if (!normalized.ok) {
    cellErrors.value.set(key, fieldErrorMessage(normalized.reason))
    return
  }
  saving.value = true
  error.value = null
  try {
    const result = await mutate<{ row: DataRow }>(
      `/tables/${props.schema.table.id}/rows/${row.id}`,
      'PATCH',
      { fields: { [item.field.id]: normalized.value } },
    )
    if (disposed.value || props.identity !== auth.user?.id) return
    rows.updateRow(event.detail.rowIndex, result.row)
    cellErrors.value.delete(key)
    selectedRows.value.delete(row.id)
  } catch (cause) {
    if (cause instanceof RequestError && [401, 403, 404].includes(cause.status))
      handleAccessFailure(cause)
    cellErrors.value.set(key, errorMessage(cause))
    error.value = cause
  } finally {
    if (!disposed.value) saving.value = false
  }
}

async function addRow() {
  if (!canWrite.value || saving.value) return
  saving.value = true
  error.value = null
  const anchor = rows.total
  try {
    await mutate<RowMutation>(`/tables/${props.schema.table.id}/rows`, 'POST', {
      rows: [{ fields: {} }],
    })
    await initialize(anchor)
  } catch (cause) {
    if (cause instanceof RequestError && [401, 403, 404].includes(cause.status))
      handleAccessFailure(cause)
    error.value = cause
  } finally {
    if (!disposed.value) saving.value = false
  }
}

function toggleField(id: string, visible: boolean) {
  const next = new Set(fieldOverride.value ?? visibleFieldIds.value)
  if (visible) next.add(id)
  else next.delete(id)
  fieldOverride.value = allFields.value
    .filter((field) => next.has(field.id))
    .map((field) => field.id)
  visibleFieldIds.value = [...fieldOverride.value]
  void initialize()
}

function selectedField(id: string) {
  return (fieldOverride.value ?? visibleFieldIds.value).includes(id)
}

function askDelete() {
  if (selectedRows.value.size > 0) deleteDialog.value?.showModal()
}

async function deleteSelected() {
  const ids = [...selectedRows.value]
  if (!ids.length || ids.length > 1000) return
  saving.value = true
  error.value = null
  const anchor = Math.max(
    0,
    Math.min(
      Math.max(0, rows.total - ids.length - 1),
      rows.snapshot().activePage * remotePageSize,
    ),
  )
  try {
    await mutate<RowMutation>(
      `/tables/${props.schema.table.id}/rows`,
      'DELETE',
      { ids },
    )
    selectedRows.value.clear()
    deleteDialog.value?.close()
    await initialize(anchor)
  } catch (cause) {
    if (cause instanceof RequestError && [401, 403, 404].includes(cause.status))
      handleAccessFailure(cause)
    error.value = cause
  } finally {
    if (!disposed.value) saving.value = false
  }
}

onMounted(() => media.addEventListener('change', resized))

onBeforeUnmount(() => {
  disposed.value = true
  media.removeEventListener('change', resized)
  for (const controller of mutationControllers) controller.abort()
  mutationControllers.clear()
  rows.dispose()
})
</script>

<template>
  <section class="data-grid" :aria-label="t('grid.title')">
    <div class="grid-toolbar">
      <button
        type="button"
        :disabled="!desktop || !canWrite || saving"
        @click="addRow"
      >
        {{ t('grid.addRow') }}
      </button>
      <button
        type="button"
        :disabled="!desktop || !canWrite || saving || selectedRows.size === 0"
        @click="askDelete"
      >
        {{ t('grid.deleteRows', { count: selectedRows.size }) }}
      </button>
      <details v-if="desktop" class="grid-fields">
        <summary>{{ t('grid.fields') }}</summary>
        <fieldset>
          <legend>{{ t('grid.visibleFields') }}</legend>
          <label v-for="field in allFields" :key="field.id">
            <input
              type="checkbox"
              :checked="selectedField(field.id)"
              @change="
                toggleField(
                  field.id,
                  ($event.target as HTMLInputElement).checked,
                )
              "
            />
            <span>{{ field.name }}</span>
          </label>
        </fieldset>
      </details>
      <span v-if="saving" role="status">{{ t('grid.saving') }}</span>
      <span v-else-if="loading" role="status">{{ t('grid.loading') }}</span>
    </div>
    <p v-if="error" role="alert">{{ gridError }}</p>
    <p v-if="cellErrors.size > 0" class="grid-validation-summary" role="alert">
      {{ [...cellErrors.values()].join(' ') }}
    </p>
    <button v-if="error" type="button" @click="() => initialize()">
      {{ t('auth.retry') }}
    </button>
    <p v-if="rows.total === 0 && !loading && !error" role="status">
      {{ t('grid.empty') }}
    </p>
    <p v-if="!desktop" class="grid-mobile-notice" role="status">
      {{ t('grid.mobileNotice') }}
    </p>
    <RevoGrid
      v-if="desktop"
      ref="host"
      :source="gridSource"
      :columns="columns"
      :editors="editors"
      :theme="preferences.isDark ? 'darkCompact' : 'compact'"
      :row-size="32"
      :range="false"
      :resize="true"
      :readonly="!canWrite"
      :use-clipboard="false"
      :hide-attribution="true"
      @created="created"
      @viewportscroll="scroll"
      @beforeedit="edit"
    />
    <p v-if="unsupportedField" role="alert">
      {{ t('grid.unsupportedField') }}
    </p>
    <dialog ref="deleteDialog" :aria-label="t('grid.confirmDeleteTitle')">
      <h2>{{ t('grid.confirmDeleteTitle') }}</h2>
      <p>{{ t('grid.confirmDelete', { count: selectedRows.size }) }}</p>
      <div class="grid-dialog-actions">
        <button type="button" :disabled="saving" @click="deleteDialog?.close()">
          {{ t('grid.cancel') }}
        </button>
        <button type="button" :disabled="saving" @click="deleteSelected">
          {{ t('grid.deleteConfirm') }}
        </button>
      </div>
    </dialog>
  </section>
</template>

<style>
.data-grid {
  margin-top: 16px;
}
.data-grid revo-grid {
  display: block;
  height: min(65vh, 680px);
  min-height: 320px;
  width: 100%;
}
.grid-toolbar,
.grid-dialog-actions {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
  margin-bottom: 8px;
}
.grid-fields {
  position: relative;
}
.grid-fields fieldset {
  position: absolute;
  z-index: 2;
  top: 100%;
  left: 0;
  min-width: 220px;
  max-height: 50vh;
  overflow: auto;
  background: var(--surface);
  color: var(--text);
  border: 1px solid var(--border);
}
.grid-fields label {
  display: flex;
  gap: 8px;
  padding: 4px;
}
.grid-cell-invalid {
  display: grid;
  color: #c0362c;
}
.grid-cell-invalid [role='alert'] {
  font-size: 11px;
  white-space: normal;
}
.data-grid button,
.data-grid summary {
  padding: 6px 10px;
  background: var(--surface);
  color: var(--text);
  border: 1px solid var(--border);
  border-radius: 4px;
  cursor: pointer;
}
.data-grid button:disabled {
  cursor: default;
  opacity: 0.55;
}
@media (max-width: 767px) {
  .grid-mobile-notice {
    display: block;
  }
}
.grid-mobile-notice {
  display: none;
}
</style>
