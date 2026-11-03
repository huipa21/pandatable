<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { errorMessage, request, RequestError } from '../../api/client'
import type {
  DataRow,
  ExportViewQuery,
  FilterNode,
  RowMutation,
  RowGroupResult,
  RowQueryRequest,
  RowQueryResult,
  RowValue,
  TableSchema,
} from '../../api/types'
import { bindField } from '../../fields/registry'
import { isWireValue, type FieldEnv, type WireValue } from '../../fields/types'
import { userLabel } from '../../fields/userDirectory'
import { useAuthStore } from '../../stores/auth'
import { useNavStore } from '../../stores/nav'
import { useRealtimeStore, type RealtimeEvent } from '../../stores/realtime'
import { useUndoStore } from '../../stores/undo'
import FieldRenderer from '../fields/FieldRenderer.vue'
import RelationLinks from '../relations/RelationLinks.vue'
import CellPresence from '../presence/CellPresence.vue'
import { RemoteRowsWindow, remotePageSize } from '../grid/remoteWindow'
import { RealtimeRowSync } from '../grid/realtimeRows'
import { GroupedRowsWindow, type GroupNode } from '../grid/groupedRows'
import { normalizeColumns } from '../grid/viewColumns'

const props = defineProps<{
  schema: TableSchema
  viewId: string
  identity: string
  filter?: FilterNode | null
}>()
const emit = defineEmits<{
  expand: [rowId: string]
  add: []
  schemaChanged: []
}>()
const { t, locale } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const nav = useNavStore()
const realtime = useRealtimeStore()
const undo = useUndoStore()
const resolveUser = (id: string) =>
  userLabel(props.identity, props.schema.table.id, id)
const options = ref<HTMLDialogElement>()
const confirmation = ref<HTMLDialogElement>()
const heading = ref<HTMLElement>()
const error = ref<unknown>(null)
const initializing = ref(false)
const groupLoading = ref(false)
const saving = ref(false)
const page = ref(0)
const revision = ref(0)
const selected = ref(new Set<string>())
const override = ref<string[] | null>(null)
let remoteRows: RemoteRowsWindow | undefined
let groupedRows: GroupedRowsWindow | undefined
let generation = 0
let disposed = false
let mutation: AbortController | undefined
let realtimeReloadPending = false
const env = computed<FieldEnv>(() => ({
  timeZone: auth.user?.effectiveTimezone,
  locale: locale.value,
}))
const canWrite = computed(
  () =>
    props.identity === auth.user?.id &&
    (props.schema.myLevel === 'write' || props.schema.myLevel === 'owner'),
)
const fields = computed(() =>
  [...props.schema.fields]
    .sort((a, b) => a.position - b.position)
    .map((field) => ({
      ...field,
      bound: bindField(field.uiType, field.options),
    })),
)
const display = computed(() =>
  fields.value.find((field) => field.id === props.schema.table.displayFieldId),
)
const view = computed(() =>
  props.schema.views.find((candidate) => candidate.id === props.viewId),
)
const viewColumns = computed(() =>
  normalizeColumns(
    view.value?.config ?? { fields: [], frozen: 0 },
    props.schema,
  ),
)
const groupBy = computed(() => view.value?.config.groupBy ?? [])
const state = computed(() => {
  void revision.value
  return groupedRows?.snapshot() ?? remoteRows?.snapshot()
})
const visibleIds = computed(() => {
  void revision.value
  return (
    override.value ??
    remoteRows?.fieldIds ??
    (groupedRows
      ? viewColumns.value.fields
          .filter((field) => field.visible)
          .map((field) => field.id)
      : [])
  )
})
const preview = computed(() =>
  visibleIds.value
    .filter((id) => id !== display.value?.id)
    .slice(0, 4)
    .map((id) => fields.value.find((field) => field.id === id))
    .filter((field) => field !== undefined),
)
const cards = computed(() => {
  void revision.value
  const start = page.value * remotePageSize
  const count = Math.max(
    0,
    Math.min(remotePageSize, (remoteRows?.total ?? 0) - start),
  )
  return Array.from({ length: count }, (_, index) =>
    remoteRows?.rowAt(start + index),
  ).filter((row) => row !== null && row !== undefined)
})
type CardListEntry =
  | { kind: 'group'; key: string; group: GroupNode }
  | { kind: 'row'; key: string; row: DataRow }
const mobileEntries = computed<CardListEntry[]>(() => {
  void revision.value
  if (!groupedRows)
    return cards.value.map((row) => ({ kind: 'row', key: row.id, row }))
  const start = page.value * remotePageSize
  return groupedRows.entries
    .slice(start, start + remotePageSize)
    .flatMap<CardListEntry>((entry, index) => {
      if (entry.kind === 'group')
        return [
          {
            kind: 'group' as const,
            key: JSON.stringify(entry.group.path),
            group: entry.group,
          },
        ]
      const row = groupedRows?.rowAt(start + index)
      return row ? [{ kind: 'row' as const, key: row.id, row }] : []
    })
})
const loading = computed(
  () =>
    initializing.value ||
    groupLoading.value ||
    (!groupedRows &&
      (state.value?.total ?? 0) > 0 &&
      cards.value.length === 0 &&
      !error.value),
)
const pageCount = computed(() =>
  Math.max(1, Math.ceil((state.value?.total ?? 0) / remotePageSize)),
)

function current(token: number) {
  return !disposed && token === generation && props.identity === auth.user?.id
}
function wireValue(value: unknown): WireValue {
  if (!isWireValue(value)) throw new Error('Unsupported card field value')
  return value
}
function hasTitle(row: DataRow) {
  const value = display.value ? row.fields[display.value.id] : undefined
  return isWireValue(value) && value !== null
}
function hasRelationValue(field: { id: string; uiType: string }, row: DataRow) {
  return field.uiType === 'link' && isWireValue(row.fields[field.id])
}
function groupKeyLabel(key: RowValue) {
  if (key === null) return t('grid.groupEmpty')
  if (Array.isArray(key)) return key.map(String).join(', ')
  return String(key)
}
function groupTitle(group: GroupNode) {
  const field = props.schema.fields.find(
    (candidate) => candidate.id === groupBy.value[group.level],
  )
  return t('grid.groupHeader', {
    field: field?.name ?? '',
    value: groupKeyLabel(group.key),
    count: group.count,
  })
}
function groupAggregateText(aggregate: GroupNode['aggregates'][number]) {
  const field = props.schema.fields.find(
    (candidate) => candidate.id === aggregate.fieldId,
  )
  return t('grid.groupAggregate', {
    field: field?.name ?? '',
    aggregate: groupAggregateLabel(aggregate.aggregate),
    value: String(aggregate.value ?? ''),
  })
}
function groupAggregateLabel(aggregate: string) {
  switch (aggregate) {
    case 'count':
      return t('grid.aggregateLabels.count')
    case 'count_empty':
      return t('grid.aggregateLabels.count_empty')
    case 'count_filled':
      return t('grid.aggregateLabels.count_filled')
    case 'unique':
      return t('grid.aggregateLabels.unique')
    case 'sum':
      return t('grid.aggregateLabels.sum')
    case 'avg':
      return t('grid.aggregateLabels.avg')
    case 'min':
      return t('grid.aggregateLabels.min')
    case 'max':
      return t('grid.aggregateLabels.max')
    case 'earliest':
      return t('grid.aggregateLabels.earliest')
    case 'latest':
      return t('grid.aggregateLabels.latest')
    case 'percent_checked':
      return t('grid.aggregateLabels.percent_checked')
    default:
      return aggregate
  }
}
function groupActionLabel(group: GroupNode) {
  const field = props.schema.fields.find(
    (candidate) => candidate.id === groupBy.value[group.level],
  )
  const params = { field: field?.name ?? '', value: groupKeyLabel(group.key) }
  return group.expanded
    ? t('grid.collapseGroup', params)
    : t('grid.expandGroup', params)
}
async function toggleGroup(group: GroupNode) {
  if (!groupedRows || loading.value || saving.value) return
  const token = generation
  groupLoading.value = true
  error.value = null
  try {
    await groupedRows.toggle(group.path)
    if (current(token)) {
      page.value = Math.min(page.value, pageCount.value - 1)
      revision.value++
    }
  } catch (cause) {
    failure(cause, token)
  } finally {
    if (current(token)) groupLoading.value = false
  }
}

function failure(cause: unknown, token: number) {
  if (!current(token)) return
  error.value = cause
  if (!(cause instanceof RequestError)) return
  if (cause.code === 'ERR_PASSWORD_CHANGE_REQUIRED') {
    remoteRows?.dispose()
    groupedRows?.dispose()
    void router.replace('/change-password')
  } else if ([401, 403, 404].includes(cause.status)) {
    remoteRows?.dispose()
    groupedRows?.dispose()
    selected.value.clear()
    if (cause.status === 401) {
      auth.setUser(null)
      void router.replace('/login')
    } else {
      void nav.refresh(true)
      emit('schemaChanged')
    }
  } else if (cause.code === 'ERR_SCHEMA_CHANGED') {
    remoteRows?.dispose()
    groupedRows?.dispose()
    emit('schemaChanged')
  }
}

async function refresh(anchor = page.value * remotePageSize) {
  mutation?.abort()
  remoteRows?.dispose()
  groupedRows?.dispose()
  remoteRows = undefined
  groupedRows = undefined
  const token = ++generation
  saving.value = false
  selected.value.clear()
  initializing.value = true
  if (!current(token)) {
    initializing.value = false
    return
  }
  error.value = null
  const schemaVersion = props.schema.schemaVersion
  const viewId = props.viewId
  const displayId = display.value?.id
  const fieldIds = override.value === null ? undefined : [...override.value]
  const filter = props.filter
  const query = (body: RowQueryRequest, signal: AbortSignal) =>
    request<RowQueryResult>(
      `/views/${viewId}/query`,
      'POST',
      body,
      AbortSignal.any([signal, AbortSignal.timeout(10_000)]),
    )
  const fetchRows = async (
    offset: number,
    limit: number,
    includeCount: boolean,
    signal: AbortSignal,
    groupPath?: RowValue[],
  ) => {
    const body = {
      offset,
      limit,
      includeCount,
      ...(fieldIds ? { fieldIds } : {}),
      ...(filter ? { filter } : {}),
      ...(groupPath?.length ? { groupPath } : {}),
    }
    let result = await query(body, signal)
    if (!current(token) || signal.aborted) return result
    const visible = result.fieldIds
    if (displayId && !visible.includes(displayId)) {
      const extra = await query(
        { ...body, fieldIds: [...visible, displayId] },
        signal,
      )
      if (extra.schemaVersion !== result.schemaVersion)
        throw new RequestError('ERR_SCHEMA_CHANGED', 409)
      result = { ...extra, fieldIds: visible }
    }
    if (
      current(token) &&
      !signal.aborted &&
      result.schemaVersion !== schemaVersion
    )
      throw new RequestError('ERR_SCHEMA_CHANGED', 409)
    return result
  }
  if (groupBy.value.length > 0) {
    groupedRows = new GroupedRowsWindow(
      {
        groups: async (path, signal) => {
          const result = await request<RowGroupResult>(
            `/views/${viewId}/groups`,
            'POST',
            { path, ...(filter ? { filter } : {}) },
            AbortSignal.any([signal, AbortSignal.timeout(10_000)]),
          )
          if (result.schemaVersion !== schemaVersion) {
            emit('schemaChanged')
            throw new RequestError('ERR_SCHEMA_CHANGED', 409)
          }
          return result.groups
        },
        rows: (path, offset, limit, includeCount, signal) =>
          fetchRows(offset, limit, includeCount, signal, path),
      },
      Math.max(1, Math.min(3, groupBy.value.length)),
      `pandatable:groups:${props.identity}:${props.viewId}`,
      () => {
        if (current(token)) revision.value++
      },
      (cause) => failure(cause, token),
    )
    await groupedRows.initialize()
    if (current(token)) {
      page.value = 0
      initializing.value = false
      revision.value++
    }
    return
  }
  const window = new RemoteRowsWindow(
    {
      fetch: (offset, limit, includeCount, signal) =>
        fetchRows(offset, limit, includeCount, signal),
    },
    () => {
      revision.value++
    },
    (cause) => failure(cause, token),
  )
  remoteRows = window
  await window.initialize(anchor)
  if (current(token)) {
    page.value = Math.max(0, window.snapshot().activePage)
    initializing.value = false
    revision.value++
  }
}

function requestRealtimeReload() {
  if (saving.value || initializing.value) {
    realtimeReloadPending = true
    return
  }
  realtimeReloadPending = false
  void refresh()
}

const rowSync = new RealtimeRowSync({
  window: () =>
    remoteRows || groupedRows
      ? {
          snapshot: () => (groupedRows ?? remoteRows)!.snapshot(),
          indexOfRow: (id) => (groupedRows ?? remoteRows)!.indexOfRow(id),
          invalidateOffscreen: (ids) => remoteRows?.invalidateOffscreen(ids),
        }
      : undefined,
  epoch: () => generation,
  fetch: async (ids, signal) => {
    const fieldIds = [...visibleIds.value]
    if (display.value && !fieldIds.includes(display.value.id))
      fieldIds.push(display.value.id)
    const result = await request<RowQueryResult>(
      `/views/${props.viewId}/query`,
      'POST',
      {
        ids,
        limit: ids.length,
        includeCount: false,
        fieldIds,
        ...(props.filter ? { filter: props.filter } : {}),
      } satisfies RowQueryRequest,
      AbortSignal.any([signal, AbortSignal.timeout(10_000)]),
    )
    if (result.schemaVersion !== props.schema.schemaVersion) {
      emit('schemaChanged')
      throw new RequestError('ERR_SCHEMA_CHANGED', 409)
    }
    return result
  },
  merge: (row) => {
    const window = groupedRows ?? remoteRows
    const index = window?.indexOfRow(row.id) ?? -1
    if (index >= 0) window?.updateRow(index, row)
  },
  reload: requestRealtimeReload,
  needsReload: () =>
    !!groupedRows || !!props.filter || !!view.value?.config.sort?.length,
  failed: (cause) => failure(cause, generation),
})

function handleRealtime(event: RealtimeEvent) {
  if (event.t === 'row.updated' && event.ids) {
    if (initializing.value) requestRealtimeReload()
    else rowSync.update(event.ids)
  } else if (
    ['comment.created', 'comment.updated', 'comment.deleted'].includes(
      event.t,
    ) &&
    event.ids
  ) {
    if (initializing.value) requestRealtimeReload()
    else rowSync.update(event.ids)
  } else if (
    ['row.created', 'row.deleted', 'row.bulk', 'resync'].includes(event.t) ||
    (event.t === 'view.changed' && event.viewId === props.viewId)
  ) {
    requestRealtimeReload()
  }
}

watch(
  [() => props.schema.table.id, () => props.identity, () => auth.user?.id],
  ([tableId, identity, userId], _previous, onCleanup) => {
    if (!tableId || !identity || identity !== userId) return
    onCleanup(realtime.subscribe(tableId, handleRealtime))
  },
  { immediate: true },
)
watch([saving, initializing], ([isSaving, isInitializing]) => {
  if (!isSaving && !isInitializing && realtimeReloadPending)
    requestRealtimeReload()
})

async function move(delta: number) {
  const next = page.value + delta
  if (saving.value || loading.value || next < 0 || next >= pageCount.value)
    return
  page.value = next
  selected.value.clear()
  if (groupedRows) groupedRows.load(next * remotePageSize)
  else remoteRows?.load(next * remotePageSize)
  revision.value++
  await nextTick()
  heading.value?.focus()
}

function toggleField(id: string, checked: boolean) {
  const next = new Set(visibleIds.value)
  if (checked) next.add(id)
  else next.delete(id)
  override.value = viewColumns.value.fields
    .filter((field) => next.has(field.id))
    .map((field) => field.id)
  void refresh()
}

function askDelete() {
  options.value?.close()
  confirmation.value?.showModal()
}

async function deleteSelected() {
  if (!canWrite.value || saving.value || !selected.value.size) return
  const token = generation
  const ids = [...selected.value]
  mutation = new AbortController()
  saving.value = true
  error.value = null
  try {
    const result = await request<RowMutation>(
      `/tables/${props.schema.table.id}/rows`,
      'DELETE',
      { ids },
      AbortSignal.any([mutation.signal, AbortSignal.timeout(10_000)]),
    )
    if (!current(token)) return
    if (result.rows.length !== ids.length)
      throw new Error('Row deletion returned an unsupported response')
    undo.record([
      { kind: 'restore', tableId: props.schema.table.id, rows: result.rows },
    ])
    confirmation.value?.close()
    await refresh()
  } catch (cause) {
    failure(cause, token)
  } finally {
    if (current(token)) saving.value = false
  }
}

watch(
  [
    () => props.schema.table.id,
    () => props.viewId,
    () => props.schema.schemaVersion,
    () => props.schema.myLevel,
    () => props.identity,
    () => auth.user?.id,
    () => JSON.stringify(props.filter),
    () => JSON.stringify(view.value?.config.groupBy ?? []),
    () => JSON.stringify(view.value?.config.aggregates ?? []),
  ],
  () => {
    confirmation.value?.close()
    override.value = null
    page.value = 0
    void refresh(0)
  },
  { immediate: true },
)
watch(
  [
    () => props.schema.table.id,
    () => props.viewId,
    () => props.identity,
    () => auth.user?.id,
  ],
  () => options.value?.close(),
)
defineExpose({
  refresh: () => refresh(),
  navigateRow: async (id: string, direction: -1 | 1) => {
    const window = groupedRows ?? remoteRows
    const index = window?.indexOfRow(id) ?? -1
    if (!window || index < 0)
      throw new Error(t('rowExpand.navigationUnavailable'))
    for (
      let target = index + direction;
      target >= 0 && target < window.total;
      target += direction
    ) {
      if (!(await window.ensureLoaded(target)))
        throw new Error(t('grid.loadError'))
      const row = window.rowAt(target)
      if (row) return row.id
    }
    return null
  },
  exportQuery: (): ExportViewQuery => ({
    ...(override.value === null ? {} : { fieldIds: [...override.value] }),
    groupBy: [...groupBy.value],
  }),
})
onBeforeUnmount(() => {
  disposed = true
  generation++
  mutation?.abort()
  rowSync.dispose()
  remoteRows?.dispose()
  groupedRows?.dispose()
  options.value?.close()
  confirmation.value?.close()
})
</script>

<template>
  <section
    class="mobile-rows"
    :aria-label="t('mobileRows.title')"
    :data-cached-rows="state?.cachedRows ?? 0"
    :data-page="page"
  >
    <header class="mobile-toolbar">
      <h2 ref="heading" tabindex="-1">{{ t('mobileRows.title') }}</h2>
      <button type="button" @click="options?.showModal()">
        {{ t('mobileRows.options') }}
      </button>
    </header>
    <p v-if="loading" role="status">{{ t('grid.loading') }}</p>
    <div v-if="error" role="alert">
      <p>{{ errorMessage(error) }}</p>
      <button type="button" :disabled="saving" @click="refresh()">
        {{ t('auth.retry') }}
      </button>
    </div>
    <template v-else-if="!loading && identity === auth.user?.id">
      <p v-if="state?.total === 0">{{ t('grid.empty') }}</p>
      <ul class="mobile-card-list">
        <li
          v-for="entry in mobileEntries"
          :key="entry.key"
          :class="entry.kind === 'group' ? 'mobile-group-item' : undefined"
          :data-row-id="entry.kind === 'row' ? entry.row.id : undefined"
        >
          <div
            v-if="entry.kind === 'group'"
            class="mobile-group-header"
            :data-level="entry.group.level"
          >
            <button
              type="button"
              :data-group-path="JSON.stringify(entry.group.path)"
              :aria-label="groupActionLabel(entry.group)"
              :aria-expanded="entry.group.expanded"
              @click="toggleGroup(entry.group)"
            >
              <span class="mobile-group-indicator" aria-hidden="true"></span>
              {{ groupTitle(entry.group) }}
            </button>
            <span
              v-for="aggregate in entry.group.aggregates"
              :key="`${aggregate.fieldId}:${aggregate.aggregate}`"
            >
              {{ groupAggregateText(aggregate) }}
            </span>
          </div>
          <template v-else>
            <button
              type="button"
              class="mobile-card"
              :aria-label="t('rowExpand.open', { id: entry.row.id })"
              :aria-describedby="`row-title-${entry.row.id}`"
              @click="emit('expand', entry.row.id)"
            >
              <span :id="`row-title-${entry.row.id}`" class="card-title">
                <FieldRenderer
                  v-if="display?.bound.ok && hasTitle(entry.row)"
                  :field="display.bound.value"
                  :value="wireValue(entry.row.fields[display.id])"
                  :env="env"
                  :table-id="schema.table.id"
                  :identity="identity"
                  :resolve-user="resolveUser"
                  :links="false"
                />
                <span v-else>{{
                  t('mobileRows.untitled', { id: entry.row.id })
                }}</span>
                <CellPresence
                  v-if="display"
                  :table-id="schema.table.id"
                  :row-id="entry.row.id"
                  :field-id="display.id"
                  :identity="identity"
                />
              </span>
              <span
                v-if="entry.row.commentCount"
                class="mobile-comment-count"
                :aria-label="
                  t('comments.count', { count: entry.row.commentCount })
                "
              >
                {{ entry.row.commentCount }}
              </span>
              <span v-for="field in preview" :key="field.id" class="card-field">
                <span class="card-label">{{ field.name }}</span>
                <RelationLinks
                  v-if="hasRelationValue(field, entry.row)"
                  :links="entry.row.fields[field.id]"
                />
                <FieldRenderer
                  v-else-if="
                    field.bound.ok && isWireValue(entry.row.fields[field.id])
                  "
                  :field="field.bound.value"
                  :value="wireValue(entry.row.fields[field.id])"
                  :env="env"
                  :table-id="schema.table.id"
                  :identity="identity"
                  :resolve-user="resolveUser"
                  :links="false"
                />
                <span v-else>{{ t('grid.unsupportedField') }}</span>
                <CellPresence
                  :table-id="schema.table.id"
                  :row-id="entry.row.id"
                  :field-id="field.id"
                  :identity="identity"
                />
              </span>
            </button>
            <label v-if="canWrite" class="card-selection">
              <input
                v-model="selected"
                type="checkbox"
                :value="entry.row.id"
                :disabled="saving"
                :aria-label="t('grid.selectRow', { id: entry.row.id })"
              />
              {{ t('grid.selectRow', { id: entry.row.id }) }}
            </label>
          </template>
        </li>
      </ul>
    </template>
    <nav class="mobile-paging" :aria-label="t('mobileRows.paging')">
      <button
        type="button"
        :disabled="page === 0 || loading || saving || !!error"
        @click="move(-1)"
      >
        {{ t('mobileRows.previous') }}
      </button>
      <span role="status">{{
        t('mobileRows.page', {
          page: page + 1,
          pages: pageCount,
          total: state?.total ?? 0,
        })
      }}</span>
      <button
        type="button"
        :disabled="page + 1 >= pageCount || loading || saving || !!error"
        @click="move(1)"
      >
        {{ t('mobileRows.next') }}
      </button>
    </nav>
    <button
      v-if="canWrite"
      type="button"
      class="mobile-add"
      :disabled="saving"
      :aria-label="t('grid.addRow')"
      @click="emit('add')"
    >
      {{ t('mobileRows.plus') }}
    </button>
    <dialog
      ref="options"
      class="mobile-sheet"
      :aria-label="t('mobileRows.options')"
      @click="
        (event) => {
          if (event.target === options) options.close()
        }
      "
    >
      <header class="mobile-toolbar">
        <h2>{{ t('mobileRows.options') }}</h2>
        <button type="button" @click="options?.close()">
          {{ t('mobileRows.closeOptions') }}
        </button>
      </header>
      <button type="button" :disabled="saving || loading" @click="refresh()">
        {{ t('mobileRows.refresh') }}
      </button>
      <button
        v-if="canWrite"
        type="button"
        :disabled="!selected.size || saving"
        @click="askDelete"
      >
        {{ t('grid.deleteRows', { count: selected.size }) }}
      </button>
      <fieldset :disabled="saving || loading">
        <legend>{{ t('grid.visibleFields') }}</legend>
        <label v-for="field in fields" :key="field.id" class="field-choice">
          <input
            type="checkbox"
            :checked="visibleIds.includes(field.id)"
            @change="
              toggleField(field.id, ($event.target as HTMLInputElement).checked)
            "
          />
          {{ field.name }}
        </label>
      </fieldset>
      <slot />
    </dialog>
    <dialog
      ref="confirmation"
      class="mobile-sheet"
      :aria-label="t('grid.confirmDeleteTitle')"
      @cancel="
        ($event) => {
          if (saving) $event.preventDefault()
        }
      "
    >
      <h2>{{ t('grid.confirmDeleteTitle') }}</h2>
      <p>{{ t('grid.confirmDelete', { count: selected.size }) }}</p>
      <p v-if="error" role="alert">{{ errorMessage(error) }}</p>
      <button type="button" :disabled="saving" @click="deleteSelected">
        {{ t('grid.deleteConfirm') }}
      </button>
      <button type="button" :disabled="saving" @click="confirmation?.close()">
        {{ t('grid.cancel') }}
      </button>
    </dialog>
  </section>
</template>

<style scoped>
.mobile-rows {
  min-width: 0;
  padding-bottom: 80px;
}
.mobile-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
h2 {
  font-size: 18px;
  overflow-wrap: anywhere;
}
button,
.field-choice,
.card-selection {
  min-height: 44px;
  font-size: 16px;
}
.mobile-card-list {
  display: grid;
  gap: 12px;
  padding: 0;
  list-style: none;
}
.mobile-card-list li {
  min-width: 0;
  border: 1px solid var(--border);
  border-radius: 8px;
}
.mobile-card-list .mobile-group-item {
  position: sticky;
  top: 0;
  z-index: 1;
  background: var(--surface);
}
.mobile-group-header {
  display: grid;
  gap: 4px;
  padding: 8px;
  overflow-wrap: anywhere;
}
.mobile-group-header button {
  min-height: 44px;
  text-align: start;
  font-weight: 700;
}
.mobile-group-indicator {
  display: inline-block;
  width: 0;
  height: 0;
  margin-inline-end: 6px;
  border-block: 4px solid transparent;
  border-inline-start: 6px solid currentColor;
  transition: transform 100ms;
}
.mobile-group-header button[aria-expanded='true'] .mobile-group-indicator {
  transform: rotate(90deg);
}
.mobile-card {
  display: grid;
  width: 100%;
  gap: 8px;
  text-align: start;
  padding: 12px;
}
.card-title,
.card-field {
  position: relative;
}
.card-title {
  font-weight: 700;
}
.card-field {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 2fr);
  gap: 8px;
}
.card-label {
  color: var(--muted);
}
.card-title,
.card-label,
.card-field > span {
  min-width: 0;
  overflow-wrap: anywhere;
}
.mobile-card :deep(.field-value) {
  max-height: 4.5em;
  overflow-wrap: anywhere;
  white-space: pre-wrap;
}
.card-selection,
.field-choice {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px;
  overflow-wrap: anywhere;
}
input[type='checkbox'] {
  width: 24px;
  height: 24px;
  flex-shrink: 0;
}
.mobile-paging {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.mobile-paging span {
  text-align: center;
}
.mobile-rows .mobile-add {
  position: fixed;
  z-index: 2;
  right: 20px;
  bottom: max(20px, env(safe-area-inset-bottom));
  width: 56px;
  height: 56px;
  border-radius: 50%;
  background: var(--accent);
  color: var(--surface);
  font-size: 32px;
}
.mobile-sheet {
  position: fixed;
  inset: auto 0 0;
  width: 100%;
  max-width: 100%;
  max-height: 85dvh;
  margin: 0;
  padding: 16px 16px max(16px, env(safe-area-inset-bottom));
  overflow: auto;
  border: 1px solid var(--border);
  border-radius: 12px 12px 0 0;
  color: var(--text);
  background: var(--surface);
}
.mobile-sheet::backdrop {
  background: #0008;
}
fieldset {
  min-width: 0;
  margin-top: 16px;
}
</style>
