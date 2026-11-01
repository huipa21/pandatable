<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { errorMessage, request, RequestError } from '../../api/client'
import type {
  DataRow,
  DirectoryPrincipal,
  RowGroupBucket,
  RowGroupResult,
  RowPatchMutation,
  RowQueryRequest,
  RowQueryResult,
  TableField,
  TableSchema,
  TableView,
} from '../../api/types'
import type { ChoiceOptions } from '../../fields/choice'
import { bindField } from '../../fields/registry'
import { isWireValue, type FieldEnv, type WireValue } from '../../fields/types'
import {
  searchTableUsers,
  userLabel,
  userLabelDeleted,
  userLabelFailed,
} from '../../fields/userDirectory'
import { useAuthStore } from '../../stores/auth'
import { useNavStore } from '../../stores/nav'
import { useRealtimeStore } from '../../stores/realtime'
import { patchUndoStep, useUndoStore } from '../../stores/undo'
import FieldRenderer from '../fields/FieldRenderer.vue'
import RelationLinks from '../relations/RelationLinks.vue'

const props = defineProps<{
  schema: TableSchema
  view: TableView
  identity: string
}>()
const emit = defineEmits<{ expand: [rowId: string]; schemaChanged: [] }>()
const { t, locale } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const nav = useNavStore()
const realtime = useRealtimeStore()
const undo = useUndoStore()
const columnsPerPage = 8
const cardsPerPage = 20

type GroupValue = string | null
interface Column {
  key: string
  value: GroupValue
  label: string
  count: number
  available: boolean
}

const groups = shallowRef<RowGroupBucket[]>([])
const rowsByColumn = shallowRef<Record<string, DataRow[]>>({})
const extraUsers = shallowRef<DirectoryPrincipal[]>([])
const searchResults = shallowRef<DirectoryPrincipal[]>([])
const searchQuery = ref('')
const searchHasMore = ref(false)
const searching = ref(false)
const columnPage = ref(0)
const cardPages = ref<Record<string, number>>({})
const loading = ref(false)
const saving = ref(false)
const loadError = ref('')
const actionError = ref('')
const searchError = ref('')
const movingRow = ref<string | null>(null)
const destination = ref('')
const dragging = shallowRef<DataRow | null>(null)
let disposed = false
let generation = 0
let readController: AbortController | undefined
let writeController: AbortController | undefined
let searchController: AbortController | undefined
let reloadTimer: ReturnType<typeof setTimeout> | undefined

const groupField = computed(() =>
  props.schema.fields.find(
    (field) => field.id === props.view.config.kanban?.groupField,
  ),
)
const displayField = computed(() =>
  props.schema.fields.find(
    (field) => field.id === props.schema.table.displayFieldId,
  ),
)
const displayBound = computed(() =>
  displayField.value
    ? bindField(displayField.value.uiType, displayField.value.options)
    : null,
)
const cardFields = computed(() =>
  (props.view.config.kanban?.fields ?? [])
    .filter((id) => id !== displayField.value?.id)
    .flatMap((id) => {
      const field = props.schema.fields.find((item) => item.id === id)
      return field
        ? [{ field, bound: bindField(field.uiType, field.options) }]
        : []
    }),
)
const fieldIds = computed(() =>
  Array.from(
    new Set(
      [
        displayField.value?.id,
        groupField.value?.id,
        ...(props.view.config.kanban?.fields ?? []),
      ].filter((id): id is string => !!id),
    ),
  ),
)
const env = computed<FieldEnv>(() => ({
  timeZone: auth.user?.effectiveTimezone,
  locale: locale.value,
}))
const canWrite = computed(
  () =>
    props.identity === auth.user?.id &&
    (props.schema.myLevel === 'write' || props.schema.myLevel === 'owner'),
)
const configured = computed(
  () =>
    !!groupField.value &&
    ['single_select', 'user'].includes(groupField.value.uiType),
)

function columnKey(value: GroupValue) {
  return JSON.stringify(value)
}

function userName(id: string) {
  const added = extraUsers.value.find((user) => user.id === id)
  const name = userLabel(props.identity, props.schema.table.id, id)
  if (userLabelDeleted(props.identity, props.schema.table.id, id))
    return t('field.user.deleted')
  if (userLabelFailed(props.identity, props.schema.table.id, id))
    return t('field.user.lookupFailed')
  return added?.name ?? name ?? t('kanban.unknownUser', { id: id.slice(0, 8) })
}

function makeColumns(field: TableField, buckets: RowGroupBucket[]): Column[] {
  const counts = new Map<string, number>()
  for (const bucket of buckets) {
    if (
      (bucket.key !== null && typeof bucket.key !== 'string') ||
      !Number.isSafeInteger(bucket.count) ||
      bucket.count < 0
    )
      throw new Error('Kanban query returned an invalid group')
    counts.set(columnKey(bucket.key), bucket.count)
  }
  const result: Column[] = []
  const seen = new Set<string>()
  const add = (value: GroupValue, label: string, available: boolean) => {
    const key = columnKey(value)
    if (seen.has(key)) return
    seen.add(key)
    result.push({ key, value, label, available, count: counts.get(key) ?? 0 })
  }
  add(null, t('kanban.empty'), !field.isRequired)
  if (field.uiType === 'single_select') {
    const bound = bindField(field.uiType, field.options)
    if (!bound.ok) throw new Error('Kanban grouping field options are invalid')
    for (const choice of (bound.value.options as ChoiceOptions).choices)
      add(choice.label, choice.label, true)
  }
  for (const bucket of buckets) {
    const value = bucket.key
    if (value === null || typeof value !== 'string') continue
    add(
      value,
      field.uiType === 'user'
        ? userName(value)
        : t('kanban.unknownChoice', { value }),
      field.uiType === 'user',
    )
  }
  if (field.uiType === 'user') {
    for (const user of extraUsers.value) add(user.id, userName(user.id), true)
  }
  return result
}

const columns = computed(() =>
  configured.value && groupField.value
    ? makeColumns(groupField.value, groups.value)
    : [],
)
const columnPageCount = computed(() =>
  Math.max(1, Math.ceil(columns.value.length / columnsPerPage)),
)
const visibleColumns = computed(() =>
  columns.value.slice(
    columnPage.value * columnsPerPage,
    (columnPage.value + 1) * columnsPerPage,
  ),
)

function cardsPageCount(column: Column) {
  return Math.max(1, Math.ceil(column.count / cardsPerPage))
}

function canNextCards(column: Column) {
  return (
    !loading.value &&
    (cardPages.value[column.key] ?? 0) + 1 < cardsPageCount(column)
  )
}

function hasTitle(row: DataRow) {
  const value = displayField.value
    ? row.fields[displayField.value.id]
    : undefined
  return isWireValue(value) && value !== null
}

function hasRelationValue(field: TableField, row: DataRow) {
  return field.uiType === 'link' && isWireValue(row.fields[field.id])
}

function current(token: number) {
  return !disposed && token === generation && props.identity === auth.user?.id
}

function handleFailure(cause: unknown) {
  if (!(cause instanceof RequestError)) return
  if (cause.code === 'ERR_PASSWORD_CHANGE_REQUIRED') {
    void router.replace('/change-password')
  } else if (cause.status === 401) {
    auth.setUser(null)
    void router.replace('/login')
  } else if ([403, 404].includes(cause.status)) {
    groups.value = []
    rowsByColumn.value = {}
    void nav.refresh(true)
    emit('schemaChanged')
  } else if (cause.code === 'ERR_SCHEMA_CHANGED') {
    emit('schemaChanged')
  }
}

async function refresh() {
  readController?.abort()
  const controller = new AbortController()
  readController = controller
  const token = ++generation
  if (
    !configured.value ||
    !groupField.value ||
    props.identity !== auth.user?.id
  ) {
    groups.value = []
    rowsByColumn.value = {}
    loading.value = false
    return
  }
  const field = groupField.value
  const version = props.schema.schemaVersion
  const filter = props.view.config.filter
  const signal = AbortSignal.any([
    controller.signal,
    AbortSignal.timeout(10_000),
  ])
  loading.value = true
  loadError.value = ''
  try {
    const result = await request<RowGroupResult>(
      `/views/${encodeURIComponent(props.view.id)}/groups`,
      'POST',
      { path: [], ...(filter ? { filter } : {}) },
      signal,
    )
    if (!current(token)) return
    if (result.schemaVersion !== version)
      throw new RequestError('ERR_SCHEMA_CHANGED', 409)
    const nextColumns = makeColumns(field, result.groups)
    const page = Math.min(
      columnPage.value,
      Math.max(0, Math.ceil(nextColumns.length / columnsPerPage) - 1),
    )
    const selected = nextColumns.slice(
      page * columnsPerPage,
      (page + 1) * columnsPerPage,
    )
    const nextPages = { ...cardPages.value }
    const entries = await Promise.all(
      selected.map(async (column): Promise<[string, DataRow[]]> => {
        const cardPage = Math.min(
          nextPages[column.key] ?? 0,
          cardsPageCount(column) - 1,
        )
        nextPages[column.key] = cardPage
        if (column.count === 0) return [column.key, []]
        const body: RowQueryRequest = {
          offset: cardPage * cardsPerPage,
          limit: cardsPerPage,
          includeCount: false,
          groupPath: [column.value],
          fieldIds: fieldIds.value,
          ...(filter ? { filter } : {}),
        }
        const response = await request<RowQueryResult>(
          `/views/${encodeURIComponent(props.view.id)}/query`,
          'POST',
          body,
          signal,
        )
        if (response.schemaVersion !== version)
          throw new RequestError('ERR_SCHEMA_CHANGED', 409)
        return [column.key, response.rows]
      }),
    )
    if (!current(token)) return
    columnPage.value = page
    cardPages.value = nextPages
    groups.value = result.groups
    rowsByColumn.value = Object.fromEntries(entries)
  } catch (cause) {
    if (!current(token) || controller.signal.aborted) return
    handleFailure(cause)
    loadError.value = errorMessage(cause)
  } finally {
    if (current(token)) loading.value = false
  }
}

function scheduleRefresh() {
  clearTimeout(reloadTimer)
  reloadTimer = setTimeout(() => void refresh(), 100)
}

const unsubscribe = realtime.subscribe(props.schema.table.id, (event) => {
  if (event.t === 'sub.denied') {
    readController?.abort()
    generation++
    loading.value = false
    const failure = new RequestError('ERR_PERMISSION_DENIED', 403)
    handleFailure(failure)
    loadError.value = errorMessage(failure)
    return
  }
  if (
    [
      'row.updated',
      'row.created',
      'row.deleted',
      'row.bulk',
      'resync',
    ].includes(event.t)
  )
    scheduleRefresh()
})

watch(
  () =>
    [
      props.view.id,
      props.view.config.kanban?.groupField,
      JSON.stringify(props.view.config.kanban?.fields),
      JSON.stringify(props.view.config.filter),
      props.schema.schemaVersion,
    ] as const,
  () => {
    columnPage.value = 0
    cardPages.value = {}
    groups.value = []
    rowsByColumn.value = {}
    extraUsers.value = []
    searchController?.abort()
    searchResults.value = []
    searchHasMore.value = false
    searchError.value = ''
    searching.value = false
    movingRow.value = null
    void refresh()
  },
  { immediate: true },
)

function changeColumns(offset: -1 | 1) {
  columnPage.value = Math.max(
    0,
    Math.min(columnPageCount.value - 1, columnPage.value + offset),
  )
  void refresh()
}

function changeCards(column: Column, offset: -1 | 1) {
  cardPages.value = {
    ...cardPages.value,
    [column.key]: Math.max(
      0,
      Math.min(
        cardsPageCount(column) - 1,
        (cardPages.value[column.key] ?? 0) + offset,
      ),
    ),
  }
  void refresh()
}

async function searchUsers() {
  searchController?.abort()
  const controller = new AbortController()
  searchController = controller
  searching.value = true
  searchError.value = ''
  try {
    const result = await searchTableUsers(
      props.schema.table.id,
      searchQuery.value,
      AbortSignal.any([controller.signal, AbortSignal.timeout(10_000)]),
    )
    if (disposed || controller.signal.aborted) return
    searchResults.value = result.items
    searchHasMore.value = result.hasMore
  } catch (cause) {
    if (disposed || controller.signal.aborted) return
    handleFailure(cause)
    searchError.value = errorMessage(cause)
  } finally {
    if (!disposed && searchController === controller) searching.value = false
  }
}

function addUser(user: DirectoryPrincipal) {
  if (!extraUsers.value.some((item) => item.id === user.id))
    extraUsers.value = [...extraUsers.value, user]
  searchResults.value = []
  searchHasMore.value = false
  void refresh()
}

async function move(row: DataRow, column: Column) {
  const field = groupField.value
  if (!field || !canWrite.value || saving.value || !column.available) return
  const before = row.fields[field.id]
  if (before === undefined) {
    actionError.value = t('grid.saveFailed')
    return
  }
  if (before === column.value) {
    movingRow.value = null
    return
  }
  const bound = bindField(field.uiType, field.options)
  const normalized = bound.ok ? bound.value.normalize(column.value) : bound
  if (!normalized.ok) {
    actionError.value = errorMessage(new RequestError('ERR_VALIDATION', 400))
    return
  }
  const controller = new AbortController()
  writeController = controller
  saving.value = true
  actionError.value = ''
  try {
    const result = await request<RowPatchMutation>(
      `/tables/${encodeURIComponent(props.schema.table.id)}/rows/${encodeURIComponent(row.id)}`,
      'PATCH',
      { fields: { [field.id]: normalized.value } },
      AbortSignal.any([controller.signal, AbortSignal.timeout(10_000)]),
    )
    if (disposed || props.identity !== auth.user?.id) return
    if (
      result.row?.id !== row.id ||
      !result.row.fields ||
      typeof result.row.fields !== 'object' ||
      Array.isArray(result.row.fields) ||
      result.row.fields[field.id] !== normalized.value ||
      !result.previous ||
      typeof result.previous !== 'object' ||
      Array.isArray(result.previous)
    ) {
      actionError.value = t('grid.saveFailed')
      return
    }
    undo.record([
      patchUndoStep(
        props.schema.table.id,
        [{ id: row.id, fields: result.previous }],
        [result.row],
      ),
    ])
    movingRow.value = null
  } catch (cause) {
    if (disposed || controller.signal.aborted) return
    handleFailure(cause)
    actionError.value = errorMessage(cause)
  } finally {
    if (!disposed) {
      saving.value = false
      await refresh()
    }
  }
}

function startMove(row: DataRow) {
  movingRow.value = row.id
  const value = groupField.value ? row.fields[groupField.value.id] : null
  destination.value = columnKey(typeof value === 'string' ? value : null)
}

function submitMove(row: DataRow) {
  const column = columns.value.find((item) => item.key === destination.value)
  if (column) void move(row, column)
}

function startDrag(event: DragEvent, row: DataRow) {
  if (!canWrite.value || saving.value) {
    event.preventDefault()
    return
  }
  dragging.value = row
  event.dataTransfer?.setData('text/plain', row.id)
  if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move'
}

function allowDrop(event: DragEvent, column: Column) {
  if (dragging.value && canWrite.value && !saving.value && column.available)
    event.preventDefault()
}

function drop(event: DragEvent, column: Column) {
  event.preventDefault()
  const row = dragging.value
  dragging.value = null
  if (row) void move(row, column)
}

function wireValue(value: unknown): WireValue {
  if (!isWireValue(value)) throw new Error('Unsupported Kanban field value')
  return value
}

onBeforeUnmount(() => {
  disposed = true
  generation++
  clearTimeout(reloadTimer)
  readController?.abort()
  writeController?.abort()
  searchController?.abort()
  unsubscribe()
})

defineExpose({ refresh })
</script>

<template>
  <section class="kanban-view" :aria-label="t('kanban.viewType')">
    <p v-if="!configured">{{ t('kanban.notConfigured') }}</p>
    <template v-else>
      <p v-if="!canWrite">{{ t('kanban.readOnly') }}</p>
      <form
        v-if="groupField?.uiType === 'user'"
        class="user-search"
        @submit.prevent="searchUsers"
      >
        <label>
          {{ t('kanban.searchUsers') }}
          <input v-model="searchQuery" :disabled="searching" />
        </label>
        <button type="submit" :disabled="searching">
          {{ t('kanban.search') }}
        </button>
        <button
          v-for="user in searchResults"
          :key="user.id"
          type="button"
          @click="addUser(user)"
        >
          {{ t('kanban.addUser', { name: user.name }) }}
        </button>
        <p v-if="searchHasMore">{{ t('kanban.moreUsers') }}</p>
        <p v-if="searchError" role="alert">{{ searchError }}</p>
      </form>
      <p v-if="loading" role="status">{{ t('kanban.loading') }}</p>
      <p v-if="loadError" role="alert">{{ loadError }}</p>
      <p v-if="actionError" role="alert">{{ actionError }}</p>
      <button v-if="loadError" type="button" @click="refresh">
        {{ t('auth.retry') }}
      </button>
      <nav
        v-if="columnPageCount > 1"
        class="page-controls"
        :aria-label="
          t('kanban.columnsPage', {
            page: columnPage + 1,
            total: columnPageCount,
          })
        "
      >
        <button
          type="button"
          :disabled="loading || columnPage === 0"
          @click="changeColumns(-1)"
        >
          {{ t('kanban.previousColumns') }}
        </button>
        <span>{{
          t('kanban.columnsPage', {
            page: columnPage + 1,
            total: columnPageCount,
          })
        }}</span>
        <button
          type="button"
          :disabled="loading || columnPage + 1 >= columnPageCount"
          @click="changeColumns(1)"
        >
          {{ t('kanban.nextColumns') }}
        </button>
      </nav>
      <div class="kanban-columns">
        <section
          v-for="column in visibleColumns"
          :key="column.key"
          class="kanban-column"
          :data-column-key="column.key"
          @dragover="allowDrop($event, column)"
          @drop="drop($event, column)"
        >
          <h3>{{ column.label }}</h3>
          <p>{{ t('kanban.count', { count: column.count }) }}</p>
          <article
            v-for="row in rowsByColumn[column.key] ?? []"
            :key="row.id"
            class="kanban-card"
            :data-row-id="row.id"
            :draggable="canWrite && !saving"
            @dragstart="startDrag($event, row)"
            @dragend="dragging = null"
          >
            <button
              type="button"
              class="card-title"
              @click="emit('expand', row.id)"
            >
              <FieldRenderer
                v-if="displayField && displayBound?.ok && hasTitle(row)"
                :field="displayBound.value"
                :value="wireValue(row.fields[displayField.id])"
                :env="env"
                :table-id="schema.table.id"
                :identity="identity"
                :links="false"
              />
              <span v-else>{{ t('mobileRows.untitled', { id: row.id }) }}</span>
            </button>
            <div
              v-for="item in cardFields"
              :key="item.field.id"
              class="card-field"
            >
              <span class="card-label">{{ item.field.name }}</span>
              <RelationLinks
                v-if="hasRelationValue(item.field, row)"
                :links="row.fields[item.field.id]"
              />
              <FieldRenderer
                v-else-if="
                  item.bound.ok && isWireValue(row.fields[item.field.id])
                "
                :field="item.bound.value"
                :value="wireValue(row.fields[item.field.id])"
                :env="env"
                :table-id="schema.table.id"
                :identity="identity"
                :links="false"
              />
              <span v-else>{{ t('grid.unsupportedField') }}</span>
            </div>
            <button
              v-if="canWrite && movingRow !== row.id"
              type="button"
              :aria-label="t('kanban.moveCard', { id: row.id })"
              :disabled="saving"
              @click="startMove(row)"
            >
              {{ t('kanban.move') }}
            </button>
            <form
              v-if="canWrite && movingRow === row.id"
              @submit.prevent="submitMove(row)"
            >
              <label>
                {{ t('kanban.destination') }}
                <select v-model="destination" :disabled="saving">
                  <option
                    v-for="target in columns"
                    :key="target.key"
                    :value="target.key"
                    :disabled="!target.available"
                  >
                    {{ target.label }}
                  </option>
                </select>
              </label>
              <button type="submit" :disabled="saving">
                {{ t('kanban.confirmMove') }}
              </button>
              <button
                type="button"
                :disabled="saving"
                @click="movingRow = null"
              >
                {{ t('kanban.cancelMove') }}
              </button>
            </form>
          </article>
          <nav
            v-if="cardsPageCount(column) > 1"
            class="page-controls"
            :aria-label="
              t('kanban.cardsPage', {
                page: (cardPages[column.key] ?? 0) + 1,
                total: cardsPageCount(column),
              })
            "
          >
            <button
              type="button"
              :aria-label="t('kanban.previousCards', { column: column.label })"
              :disabled="loading || (cardPages[column.key] ?? 0) === 0"
              @click="changeCards(column, -1)"
            >
              {{ t('nav.moveUp') }}
            </button>
            <span>{{
              t('kanban.cardsPage', {
                page: (cardPages[column.key] ?? 0) + 1,
                total: cardsPageCount(column),
              })
            }}</span>
            <button
              type="button"
              :aria-label="t('kanban.nextCards', { column: column.label })"
              :disabled="!canNextCards(column)"
              @click="changeCards(column, 1)"
            >
              {{ t('nav.moveDown') }}
            </button>
          </nav>
        </section>
      </div>
    </template>
  </section>
</template>

<style scoped>
.kanban-view {
  min-width: 0;
  margin: 16px 0;
}
.user-search,
.page-controls,
.kanban-card form {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}
.user-search label,
.kanban-card label {
  display: grid;
  gap: 4px;
}
.kanban-columns {
  display: grid;
  grid-auto-columns: minmax(240px, 1fr);
  grid-auto-flow: column;
  gap: 12px;
  min-width: 0;
  overflow-x: auto;
  padding: 8px 0;
}
.kanban-column {
  min-width: 0;
  padding: 12px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 8px;
}
.kanban-column h3,
.kanban-column p {
  margin: 0 0 8px;
}
.kanban-card {
  display: grid;
  gap: 8px;
  margin-bottom: 8px;
  padding: 10px;
  background: var(--background);
  border: 1px solid var(--border);
  border-radius: 6px;
}
.card-title {
  font-weight: 600;
  text-align: left;
}
.card-field {
  display: grid;
  gap: 2px;
  min-width: 0;
}
.card-label {
  color: var(--muted);
}
button,
input,
select {
  min-width: 0;
  padding: 8px;
  color: var(--text);
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 4px;
}
@media (max-width: 767px) {
  button,
  input,
  select {
    min-height: 44px;
    font-size: 16px;
  }
}
</style>
