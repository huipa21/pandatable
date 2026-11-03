<script setup lang="ts">
import {
  computed,
  defineAsyncComponent,
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  shallowRef,
  watch,
} from 'vue'
import { useI18n } from 'vue-i18n'
import { createResourceLabel, accessLevelLabel } from '../i18n'
import { useRoute, useRouter } from 'vue-router'
import { errorMessage, request, RequestError } from '../api/client'
import type {
  ExportViewQuery,
  FilterNode,
  FormConfig,
  GalleryConfig,
  ImportResult,
  KanbanConfig,
  ResourceKind,
  TableSchema,
  TableView,
  ViewColumns,
  ViewMutation,
} from '../api/types'
import { useAuthStore } from '../stores/auth'
import { useNavStore } from '../stores/nav'
import { useSharingStore } from '../stores/sharing'
import { useRealtimeStore } from '../stores/realtime'
import { useUndoStore, viewUndoStep } from '../stores/undo'
import AppIcon from '../components/AppIcon.vue'
import PresenceAvatars from '../components/presence/PresenceAvatars.vue'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const nav = useNavStore()
const sharing = useSharingStore()
const realtime = useRealtimeStore()
const undo = useUndoStore()
const schema = shallowRef<TableSchema | null>(null)
// Lazy so the field registry/editor stay out of the initial bundle (D-130).
const FieldPanel = defineAsyncComponent(
  () => import('../components/fields/FieldPanel.vue'),
)
const FilterBuilder = defineAsyncComponent(
  () => import('../components/filters/FilterBuilder.vue'),
)
const FormBuilder = defineAsyncComponent(
  () => import('../components/views/FormBuilder.vue'),
)
const FormView = defineAsyncComponent(
  () => import('../components/views/FormView.vue'),
)
const KanbanBuilder = defineAsyncComponent(
  () => import('../components/views/KanbanBuilder.vue'),
)
const KanbanView = defineAsyncComponent(
  () => import('../components/views/KanbanView.vue'),
)
const GalleryBuilder = defineAsyncComponent(
  () => import('../components/views/GalleryBuilder.vue'),
)
const GalleryView = defineAsyncComponent(
  () => import('../components/views/GalleryView.vue'),
)
const DataGrid = defineAsyncComponent(
  () => import('../components/grid/DataGrid.vue'),
)
const MobileCardList = defineAsyncComponent(
  () => import('../components/rows/MobileCardList.vue'),
)
const RowExpandDialog = defineAsyncComponent(
  () => import('../components/rows/RowExpandDialog.vue'),
)
const ImportDialog = defineAsyncComponent(
  () => import('../components/importing/ImportDialog.vue'),
)
const grid = ref<{
  refresh: () => Promise<void>
  exportQuery: () => ExportViewQuery
  toggleField?: (id: string, visible: boolean) => void
  navigateRow: (id: string, direction: -1 | 1) => Promise<string | null>
}>()
const rowWorkspace = ref<HTMLElement>()
const gridWorkspace = ref<HTMLElement>()
const fieldPanel = ref<{ openCreate: () => Promise<void> }>()
const fieldsOpen = ref(false)
const filterOpen = ref(false)
const fieldsVisited = ref(false)
const filterVisited = ref(false)
const fieldMenuError = ref('')
const createFieldPending = ref(false)
const kanban = ref<{
  refresh: () => Promise<void>
  navigateRow: (id: string, direction: -1 | 1) => Promise<string | null>
}>()
const gallery = ref<{
  refresh: () => Promise<void>
  navigateRow: (id: string, direction: -1 | 1) => Promise<string | null>
}>()
const rowNavigator = computed(() =>
  view.value?.type === 'grid'
    ? grid.value?.navigateRow
    : view.value?.type === 'kanban'
      ? kanban.value?.navigateRow
      : view.value?.type === 'gallery'
        ? gallery.value?.navigateRow
        : undefined,
)

function refreshVisibleRows() {
  void grid.value?.refresh()
  void kanban.value?.refresh()
  void gallery.value?.refresh()
}

const media = window.matchMedia('(min-width: 768px)')
const desktop = ref(media.matches)
const newRow = ref(false)
const filterValue = shallowRef<FilterNode | null>(null)
const filterDraft = shallowRef<FilterNode | null>(null)
const filterApplied = ref(false)
function countConditions(node: FilterNode | null): number {
  if (!node) return 0
  return 'items' in node
    ? node.items.reduce((count, item) => count + countConditions(item), 0)
    : 1
}
const filterCount = computed(() => countConditions(filterValue.value))
const localViewConfig = shallowRef<ViewColumns | null>(null)
const hasLocalViewChanges = ref(false)
const viewError = ref('')
const viewBusy = ref(false)
const exportError = ref('')
const exportBusy = ref(false)
const importOpen = ref(false)
let exportController: AbortController | undefined
let exportGeneration = 0
const newViewName = ref('')
const newViewScope = ref<'shared' | 'personal'>('personal')
const newViewType = ref<'grid' | 'form' | 'kanban' | 'gallery'>('grid')
const viewNameDrafts = ref<Record<string, string>>({})
const resized = (event: MediaQueryListEvent) => {
  desktop.value = event.matches
  newRow.value = false
  fieldsOpen.value = false
  filterOpen.value = false
  createFieldPending.value = false
}
function dismissGridMenus(event: PointerEvent) {
  const target = event.target
  if (!(target instanceof Element) || target.closest('dialog[open]')) return
  if (!target.closest('.grid-field-popover, .grid-fields-trigger')) {
    fieldsOpen.value = false
    createFieldPending.value = false
  }
  if (!target.closest('.grid-filter-popover, .grid-filter-trigger'))
    filterOpen.value = false
}
function closeGridMenus(event: KeyboardEvent) {
  if (
    event.key !== 'Escape' ||
    document.querySelector('dialog[open]') ||
    (!fieldsOpen.value && !filterOpen.value)
  )
    return
  const trigger = fieldsOpen.value
    ? '.grid-fields-trigger'
    : '.grid-filter-trigger'
  fieldsOpen.value = false
  filterOpen.value = false
  createFieldPending.value = false
  gridWorkspace.value?.querySelector<HTMLButtonElement>(trigger)?.focus()
  event.preventDefault()
}
function toggleFieldsMenu() {
  fieldsOpen.value = !fieldsOpen.value
  if (fieldsOpen.value) fieldsVisited.value = true
  else createFieldPending.value = false
  filterOpen.value = false
}
function toggleFilterMenu() {
  filterOpen.value = !filterOpen.value
  if (filterOpen.value) filterVisited.value = true
  fieldsOpen.value = false
  createFieldPending.value = false
}
async function openFieldCreate() {
  fieldsOpen.value = true
  fieldsVisited.value = true
  filterOpen.value = false
  createFieldPending.value = true
  await nextTick()
  if (fieldPanel.value && createFieldPending.value) {
    createFieldPending.value = false
    await fieldPanel.value.openCreate()
  }
}
function toggleGridField(id: string, visible: boolean) {
  if (!grid.value?.toggleField) {
    fieldMenuError.value = t('grid.loading')
    return
  }
  fieldMenuError.value = ''
  grid.value.toggleField(id, visible)
}
watch(fieldPanel, (panel) => {
  if (!panel || !createFieldPending.value) return
  createFieldPending.value = false
  void panel.openCreate()
})
onMounted(() => {
  media.addEventListener('change', resized)
  document.addEventListener('pointerdown', dismissGridMenus)
  document.addEventListener('keydown', closeGridMenus)
})
const loading = ref(false)
const error = ref<unknown>(null)
let controller: AbortController | undefined
let generation = 0
const kind = computed<ResourceKind>(() =>
  route.params.tableId ? 'table' : route.params.bookId ? 'book' : 'workspace',
)
const node = computed(() => {
  const id =
    route.params.tableId ?? route.params.bookId ?? route.params.workspaceId
  return nav.flatItems.find(
    (item) =>
      item.kind === kind.value &&
      item.id === id &&
      (kind.value !== 'book' || item.parentId === route.params.wid),
  )
})
// The bare table route shows the default view without naming it in the URL.
const viewId = computed(() => {
  if (typeof route.params.viewId === 'string') return route.params.viewId
  const views = schema.value?.views ?? []
  return (views.find((item) => item.type === 'grid') ?? views[0])?.id
})
const displayField = computed(() =>
  schema.value?.fields.find(
    (field) => field.id === schema.value?.table.displayFieldId,
  ),
)
const view = computed(() =>
  schema.value?.views.find((item) => item.id === viewId.value),
)
const showRowDialog = computed(
  () =>
    (view.value?.type === 'grid' ||
      view.value?.type === 'kanban' ||
      view.value?.type === 'gallery') &&
    (typeof route.params.rowId === 'string' || newRow.value),
)
const canCreateSharedView = computed(
  () => schema.value?.myLevel === 'write' || schema.value?.myLevel === 'owner',
)
const canSaveAsPersonalView = computed(() => {
  const currentView = view.value
  return (
    currentView?.scope === 'shared' &&
    (currentView.locked || schema.value?.myLevel === 'read') &&
    (hasLocalViewChanges.value || filterApplied.value)
  )
})
const missing = computed(
  () =>
    nav.loaded &&
    !nav.error &&
    (!node.value || (schema.value && route.params.viewId && !view.value)),
)
const canShare = computed(
  () =>
    node.value &&
    !node.value.pathOnly &&
    node.value.level === 'owner' &&
    (kind.value !== 'table' || schema.value?.myLevel === 'owner'),
)

/** Applies a newer authoritative schema for the shown table only. */
function applySchema(next: TableSchema) {
  const shown = schema.value
  if (
    !shown ||
    next.table.id !== shown.table.id ||
    next.table.id !== node.value?.id ||
    next.schemaVersion < shown.schemaVersion
  )
    return
  for (const previousField of shown.fields) {
    if (!['single_select', 'multi_select'].includes(previousField.uiType))
      continue
    const updatedField = next.fields.find(
      (field) => field.id === previousField.id,
    )
    if (!updatedField || updatedField.uiType !== previousField.uiType) continue
    const updatedChoices = new Map(
      selectChoices(updatedField).map((choice) => [choice.id, choice.label]),
    )
    const renamedLabels = new Map<string, string>()
    for (const choice of selectChoices(previousField)) {
      const label = updatedChoices.get(choice.id)
      if (label && label !== choice.label)
        renamedLabels.set(choice.label, label)
    }
    if (!renamedLabels.size) continue
    filterValue.value = rewriteChoiceFilter(
      filterValue.value,
      previousField.id,
      renamedLabels,
    )
    filterDraft.value = rewriteChoiceFilter(
      filterDraft.value,
      previousField.id,
      renamedLabels,
    )
    if (localViewConfig.value?.filter) {
      localViewConfig.value = {
        ...localViewConfig.value,
        filter: rewriteChoiceFilter(
          localViewConfig.value.filter,
          previousField.id,
          renamedLabels,
        ),
      }
    }
  }
  schema.value = next
}

function selectChoices(field: TableSchema['fields'][number]) {
  const choices = field.options.choices
  if (!Array.isArray(choices)) return []
  return choices.filter(
    (choice: unknown): choice is { id: string; label: string } =>
      typeof choice === 'object' &&
      choice !== null &&
      'id' in choice &&
      typeof choice.id === 'string' &&
      'label' in choice &&
      typeof choice.label === 'string',
  )
}

function rewriteChoiceFilter(
  filter: FilterNode | null,
  fieldId: string,
  labels: ReadonlyMap<string, string>,
): FilterNode | null {
  if (!filter) return null
  if ('op' in filter) {
    let changed = false
    const items = filter.items.map((item) => {
      const next = rewriteChoiceFilter(item, fieldId, labels)
      if (next !== item) changed = true
      return next ?? item
    })
    return changed ? { ...filter, items } : filter
  }
  if (filter.field !== fieldId) return filter
  const value = filter.value
  if (typeof value === 'string') {
    const renamed = labels.get(value)
    return renamed ? { ...filter, value: renamed } : filter
  }
  if (
    Array.isArray(value) &&
    value.every((item): item is string => typeof item === 'string')
  ) {
    let changed = false
    const renamed = value.map((item) => {
      const next = labels.get(item)
      if (next) changed = true
      return next ?? item
    })
    return changed ? { ...filter, value: renamed } : filter
  }
  return filter
}

// A silent load keeps the shown schema (and any open field editor) while
// refreshing; failures other than access changes leave it in place.
async function loadSchema(silent = false) {
  controller?.abort()
  const current = ++generation
  const own = new AbortController()
  controller = own
  const keep =
    silent && !!node.value && schema.value?.table.id === node.value.id
  if (!keep) schema.value = null
  error.value = null
  loading.value = false
  if (!node.value || kind.value !== 'table') return
  loading.value = !keep
  try {
    const result = await request<TableSchema>(
      `/tables/${node.value.id}/schema`,
      'GET',
      undefined,
      AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
    )
    if (current !== generation) return
    if (keep && schema.value) applySchema(result)
    else schema.value = result
  } catch (cause) {
    if (current !== generation || own.signal.aborted) return
    if (
      keep &&
      !(cause instanceof RequestError && [401, 403, 404].includes(cause.status))
    )
      return
    error.value = cause
    if (cause instanceof RequestError) {
      if (cause.code === 'ERR_UNAUTHENTICATED') {
        auth.setUser(null)
        await router.replace('/login')
      } else if (cause.code === 'ERR_PASSWORD_CHANGE_REQUIRED') {
        await router.replace('/change-password')
      } else if ([403, 404].includes(cause.status)) await nav.refresh(true)
    }
  } finally {
    if (current === generation) loading.value = false
  }
}
watch(
  [
    () => route.params.tableId,
    () => route.params.viewId,
    () => route.params.bookId,
    () => route.params.workspaceId,
    () => route.params.wid,
    () => node.value?.id,
  ],
  () => void loadSchema(),
  { immediate: true },
)
watch(
  [
    () => nav.revision,
    () => node.value?.id,
    () => node.value?.name,
    () => node.value?.level,
  ],
  ([revision, id, name, level], [oldRevision, oldId, oldName, oldLevel]) => {
    const metadataChanged =
      id === oldId && (name !== oldName || level !== oldLevel)
    if (metadataChanged || (revision !== oldRevision && !loading.value))
      void loadSchema(true)
  },
)
watch(
  () => auth.user?.id,
  () => void loadSchema(),
)
watch(
  [() => route.params.tableId, () => auth.user?.id],
  ([tableId, userId], _previous, onCleanup) => {
    if (typeof tableId !== 'string' || !userId) return
    onCleanup(
      realtime.subscribe(tableId, (event) => {
        if (
          ['schema.changed', 'view.changed', 'sub.denied', 'resync'].includes(
            event.t,
          )
        )
          void loadSchema(true)
      }),
    )
  },
  { immediate: true },
)
watch(
  [() => route.params.tableId, () => view.value?.id, () => auth.user?.id],
  ([tableId, viewId, userId]) => {
    if (typeof tableId === 'string' && userId)
      realtime.setView(tableId, viewId ?? '')
  },
  { immediate: true },
)
watch(
  [
    () => route.params.tableId,
    viewId,
    () => route.params.rowId,
    () => auth.user?.id,
  ],
  () => {
    newRow.value = false
    fieldsOpen.value = false
    filterOpen.value = false
    fieldsVisited.value = false
    filterVisited.value = false
    createFieldPending.value = false
    fieldMenuError.value = ''
  },
)
watch(
  () => view.value?.id,
  () => {
    filterApplied.value = false
    filterValue.value = view.value?.config.filter ?? null
    filterDraft.value = view.value?.config.filter ?? null
    localViewConfig.value = view.value?.config ?? null
    hasLocalViewChanges.value = false
  },
  { immediate: true },
)

watch([() => route.params.tableId, viewId, () => auth.user?.id], () => {
  exportGeneration++
  exportController?.abort()
  exportBusy.value = false
  exportError.value = ''
})

watch(
  [() => route.params.tableId, () => route.params.bookId, () => auth.user?.id],
  () => {
    importOpen.value = false
  },
)

async function imported() {
  await nav.refresh(true)
  if (kind.value === 'table') {
    await grid.value?.refresh()
    await kanban.value?.refresh()
    await gallery.value?.refresh()
  }
}

async function openImportedTable(result: ImportResult) {
  if (!result.tableId) return
  importOpen.value = false
  const path = result.viewId
    ? `/t/${result.tableId}/v/${result.viewId}`
    : `/t/${result.tableId}`
  await router.push(path)
}

async function downloadExport(format: 'csv' | 'xlsx') {
  const activeView = view.value
  if (!activeView || exportBusy.value || (format === 'csv' && !grid.value))
    return
  const current = ++exportGeneration
  const controller = new AbortController()
  exportController = controller
  exportBusy.value = true
  exportError.value = ''
  try {
    const body =
      format === 'csv'
        ? { format, ...grid.value!.exportQuery(), filter: filterValue.value }
        : { format }
    const result = await request<{ ticket: string }>(
      `/views/${encodeURIComponent(activeView.id)}/export`,
      'POST',
      body,
      AbortSignal.any([controller.signal, AbortSignal.timeout(10_000)]),
    )
    if (controller.signal.aborted || current !== exportGeneration) return
    if (
      typeof result.ticket !== 'string' ||
      !/^[A-Z2-7]{26,128}$/.test(result.ticket)
    )
      throw new RequestError('ERR_INTERNAL', 500)
    const link = document.createElement('a')
    link.href = `/api/v1/exports/${encodeURIComponent(result.ticket)}`
    link.hidden = true
    document.body.appendChild(link)
    link.click()
    link.remove()
  } catch (cause) {
    if (!controller.signal.aborted && current === exportGeneration)
      exportError.value = t('export.failed', { message: errorMessage(cause) })
  } finally {
    if (current === exportGeneration) {
      exportBusy.value = false
      exportController = undefined
    }
  }
}

function canManageView(item: TableView) {
  if (item.scope === 'personal') return true
  return (
    schema.value?.myLevel === 'owner' ||
    (schema.value?.myLevel === 'write' && !item.locked)
  )
}

function canMoveView(item: TableView, index: number, direction: 'up' | 'down') {
  const viewCount = schema.value?.views.length ?? 0
  return (
    canManageView(item) &&
    !viewBusy.value &&
    (direction === 'up' ? index > 0 : index < viewCount - 1)
  )
}

function applyFilter() {
  filterValue.value = filterDraft.value
  filterApplied.value = true
}

function replaceView(updated: TableView) {
  if (!schema.value) return
  schema.value = {
    ...schema.value,
    views: schema.value.views.map((item) =>
      item.id === updated.id ? updated : item,
    ),
  }
  void nav.refresh(true)
}

function viewRequestFailed(cause: unknown) {
  viewError.value = errorMessage(cause)
}

async function createView() {
  if (!schema.value || !newViewName.value.trim()) return
  viewBusy.value = true
  viewError.value = ''
  try {
    const result = await request<ViewMutation>(
      `/tables/${schema.value.table.id}/views`,
      'POST',
      {
        name: newViewName.value,
        scope: newViewScope.value,
        type: newViewType.value,
      },
    )
    schema.value = {
      ...schema.value,
      views: [...schema.value.views, result.view],
    }
    newViewName.value = ''
    newViewType.value = 'grid'
    void nav.refresh(true)
    await router.push(`/t/${schema.value.table.id}/v/${result.view.id}`)
  } catch (cause) {
    viewRequestFailed(cause)
  } finally {
    viewBusy.value = false
  }
}

async function saveForm(form: FormConfig) {
  if (!view.value || view.value.type !== 'form' || !canManageView(view.value))
    return
  const before = view.value
  viewBusy.value = true
  viewError.value = ''
  try {
    const result = await request<ViewMutation>(
      `/views/${view.value.id}`,
      'PATCH',
      { config: { form } },
      undefined,
      { ifMatch: view.value.version },
    )
    if (schema.value)
      undo.record([viewUndoStep(schema.value.table.id, before, result.view)])
    replaceView(result.view)
  } catch (cause) {
    viewRequestFailed(cause)
  } finally {
    viewBusy.value = false
  }
}

async function saveCardView(
  kind: 'kanban' | 'gallery',
  config: KanbanConfig | GalleryConfig,
) {
  if (
    !view.value ||
    view.value.type !== kind ||
    !schema.value ||
    !canManageView(view.value) ||
    viewBusy.value
  )
    return
  const before = view.value
  const tableId = schema.value.table.id
  const identity = auth.user?.id
  viewBusy.value = true
  viewError.value = ''
  try {
    const result = await request<ViewMutation>(
      `/views/${encodeURIComponent(before.id)}`,
      'PATCH',
      { config: { [kind]: config } },
      AbortSignal.timeout(10_000),
      { ifMatch: before.version },
    )
    if (
      schema.value?.table.id !== tableId ||
      view.value?.id !== before.id ||
      auth.user?.id !== identity
    )
      return
    undo.record([viewUndoStep(tableId, before, result.view)])
    replaceView(result.view)
  } catch (cause) {
    if (
      schema.value?.table.id === tableId &&
      view.value?.id === before.id &&
      auth.user?.id === identity
    ) {
      viewRequestFailed(cause)
      if (
        cause instanceof RequestError &&
        cause.code === 'ERR_VERSION_CONFLICT'
      )
        await loadSchema(true)
    }
  } finally {
    viewBusy.value = false
  }
}

async function renameView(item: TableView) {
  if (!canManageView(item)) return
  const name = viewNameDrafts.value[item.id] ?? item.name
  viewBusy.value = true
  viewError.value = ''
  try {
    const result = await request<ViewMutation>(
      `/views/${item.id}`,
      'PATCH',
      { name },
      undefined,
      { ifMatch: item.version },
    )
    replaceView(result.view)
  } catch (cause) {
    viewRequestFailed(cause)
  } finally {
    viewBusy.value = false
  }
}

async function duplicateView(item: TableView, scope: 'shared' | 'personal') {
  if (scope === 'shared' && !canCreateSharedView.value) return
  viewBusy.value = true
  viewError.value = ''
  try {
    const result = await request<ViewMutation>(
      `/views/${item.id}/duplicate`,
      'POST',
      {
        name: t('nav.viewCopyName', { name: item.name }),
        scope,
      },
    )
    if (schema.value) {
      schema.value = {
        ...schema.value,
        views: [...schema.value.views, result.view],
      }
      void nav.refresh(true)
      await router.push(`/t/${schema.value.table.id}/v/${result.view.id}`)
    }
  } catch (cause) {
    viewRequestFailed(cause)
  } finally {
    viewBusy.value = false
  }
}

async function saveAsPersonalView() {
  if (!schema.value || !view.value || view.value.scope !== 'shared') return
  viewBusy.value = true
  viewError.value = ''
  try {
    const config = localViewConfig.value
    if (!config) throw new RequestError('ERR_INTERNAL', 500)
    if (filterApplied.value) config.filter = filterValue.value
    const result = await request<ViewMutation>(
      `/views/${view.value.id}/duplicate`,
      'POST',
      {
        name: t('nav.viewCopyName', { name: view.value.name }),
        scope: 'personal',
        config,
      },
    )
    schema.value = {
      ...schema.value,
      views: [...schema.value.views, result.view],
    }
    void nav.refresh(true)
    await router.push(`/t/${schema.value.table.id}/v/${result.view.id}`)
  } catch (cause) {
    viewRequestFailed(cause)
  } finally {
    viewBusy.value = false
  }
}

async function toggleViewLock(item: TableView) {
  if (schema.value?.myLevel !== 'owner' || item.scope !== 'shared') return
  viewBusy.value = true
  viewError.value = ''
  try {
    const result = await request<ViewMutation>(
      `/views/${item.id}`,
      'PATCH',
      { locked: !item.locked },
      undefined,
      { ifMatch: item.version },
    )
    replaceView(result.view)
  } catch (cause) {
    viewRequestFailed(cause)
  } finally {
    viewBusy.value = false
  }
}

async function moveView(item: TableView, beforeId: string | null) {
  if (!canManageView(item)) return
  viewBusy.value = true
  viewError.value = ''
  try {
    const result = await request<ViewMutation>(
      `/views/${item.id}/move`,
      'POST',
      { beforeId },
      undefined,
      { ifMatch: item.version },
    )
    replaceView(result.view)
    if (schema.value)
      schema.value = {
        ...schema.value,
        views: [...schema.value.views].sort(
          (left, right) =>
            left.position - right.position || left.id.localeCompare(right.id),
        ),
      }
  } catch (cause) {
    viewRequestFailed(cause)
  } finally {
    viewBusy.value = false
  }
}

async function deleteView(item: TableView) {
  if (
    !canManageView(item) ||
    !window.confirm(t('nav.confirmDeleteView', { name: item.name }))
  )
    return
  viewBusy.value = true
  viewError.value = ''
  try {
    await request(`/views/${item.id}`, 'DELETE', undefined, undefined, {
      ifMatch: item.version,
    })
    if (!schema.value) return
    const wasActive = view.value?.id === item.id
    const views = schema.value.views.filter(
      (candidate) => candidate.id !== item.id,
    )
    schema.value = { ...schema.value, views }
    void nav.refresh(true)
    if (wasActive) {
      const next = views[0]
      if (next) await router.replace(`/t/${schema.value.table.id}/v/${next.id}`)
      else await router.replace(`/t/${schema.value.table.id}`)
    }
  } catch (cause) {
    viewRequestFailed(cause)
  } finally {
    viewBusy.value = false
  }
}
onBeforeUnmount(() => {
  generation++
  controller?.abort()
  exportGeneration++
  exportController?.abort()
  media.removeEventListener('change', resized)
  document.removeEventListener('pointerdown', dismissGridMenus)
  document.removeEventListener('keydown', closeGridMenus)
})
</script>

<template>
  <section class="resource-page">
    <p v-if="(!nav.loaded && !nav.error) || loading" role="status">
      {{ t('nav.loading') }}
    </p>
    <div v-else-if="!nav.loaded && nav.error" role="alert">
      <h1>{{ t('nav.unavailable') }}</h1>
      <p>{{ errorMessage(nav.error) }}</p>
      <button :disabled="nav.loading" @click="nav.refresh(true)">
        {{ t('auth.retry') }}
      </button>
    </div>
    <div v-else-if="missing">
      <h1>{{ t('nav.removedTitle') }}</h1>
      <p>{{ t('errors.ERR_NOT_FOUND') }}</p>
      <RouterLink to="/home">{{ t('auth.homeTitle') }}</RouterLink>
    </div>
    <div v-else-if="error" role="alert">
      <h1>{{ t('nav.unavailable') }}</h1>
      <p>{{ errorMessage(error) }}</p>
      <button @click="loadSchema()">{{ t('auth.retry') }}</button>
    </div>
    <template v-else-if="node">
      <header v-if="kind !== 'table'" class="resource-heading">
        <h1>{{ schema?.table.name ?? node.name }}</h1>
      </header>
      <button v-if="canShare && kind !== 'table'" @click="sharing.open(node)">
        {{ t('sharing.open') }}
      </button>
      <p v-if="node.pathOnly">{{ t('nav.pathNotice') }}</p>
      <template v-if="kind !== 'table'">
        <p v-if="node.children.length === 0">
          {{
            kind === 'workspace' ? t('nav.emptyBooks') : t('nav.emptyTables')
          }}
        </p>
        <ul class="resource-list">
          <li v-for="child in node.children" :key="child.id">
            <RouterLink :to="child.path">{{ child.name }}</RouterLink>
            <span v-if="child.pathOnly">{{ t('nav.pathOnly') }}</span>
          </li>
        </ul>
        <button
          v-if="node.canCreate && node.childKind"
          :disabled="nav.busy"
          @click="
            nav.editor = {
              operation: 'create',
              kind: node.childKind,
              parentId: node.id,
            }
          "
        >
          {{ createResourceLabel(node.childKind) }}
        </button>
        <button
          v-if="kind === 'book' && node.canCreate"
          type="button"
          @click="importOpen = true"
        >
          {{ t('import.newTable') }}
        </button>
      </template>

      <template v-else-if="schema">
        <h1 class="visually-hidden">{{ schema.table.name }}</h1>
        <div class="table-utility">
          <PresenceAvatars
            :table-id="node.id"
            :identity="auth.user?.id ?? ''"
          />
          <button v-if="canShare" type="button" @click="sharing.open(node)">
            {{ t('sharing.open') }}
          </button>
          <details class="table-management">
            <summary>
              <AppIcon name="table" />
              {{ t('nav.tableOptions') }}
            </summary>
            <div class="table-management-panel">
              <dl class="schema-summary">
                <dt>{{ t('nav.displayField') }}</dt>
                <dd>{{ displayField?.name ?? t('nav.noDisplayField') }}</dd>
                <dt>{{ t('nav.access') }}</dt>
                <dd>{{ accessLevelLabel(schema.myLevel) }}</dd>
              </dl>
              <button
                v-if="schema.myLevel === 'write' || schema.myLevel === 'owner'"
                type="button"
                @click="importOpen = true"
              >
                {{ t('import.append') }}
              </button>
              <details v-if="view" class="view-export">
                <summary>{{ t('export.title') }}</summary>
                <button
                  v-if="view.type === 'grid'"
                  type="button"
                  :disabled="exportBusy || !grid"
                  @click="downloadExport('csv')"
                >
                  {{ t('export.csv') }}
                </button>
                <button
                  type="button"
                  :disabled="exportBusy"
                  @click="downloadExport('xlsx')"
                >
                  {{ t('export.package') }}
                </button>
                <p>{{ t('export.packageNotice') }}</p>
                <p v-if="exportBusy" role="status">
                  {{ t('export.preparing') }}
                </p>
                <p v-if="exportError" role="alert">{{ exportError }}</p>
              </details>
              <nav class="view-links" :aria-label="t('nav.views')">
                <div v-for="(item, index) in schema.views" :key="item.id">
                  <RouterLink
                    :to="`/t/${schema.table.id}/v/${item.id}`"
                    :aria-current="view?.id === item.id ? 'page' : undefined"
                  >
                    {{ item.name }}
                  </RouterLink>
                  <span v-if="item.scope === 'shared'">{{
                    t('nav.viewScope.shared')
                  }}</span>
                  <span v-else>{{ t('nav.viewScope.personal') }}</span>
                  <button
                    type="button"
                    :aria-label="t('nav.moveViewUp', { name: item.name })"
                    :disabled="!canMoveView(item, index, 'up')"
                    @click="moveView(item, schema.views[index - 1]!.id)"
                  >
                    {{ t('nav.moveUp') }}
                  </button>
                  <button
                    type="button"
                    :aria-label="t('nav.moveViewDown', { name: item.name })"
                    :disabled="!canMoveView(item, index, 'down')"
                    @click="moveView(item, schema.views[index + 2]?.id ?? null)"
                  >
                    {{ t('nav.moveDown') }}
                  </button>
                  <button
                    v-if="item.scope === 'shared' && !canManageView(item)"
                    type="button"
                    :disabled="viewBusy"
                    @click="duplicateView(item, 'personal')"
                  >
                    {{ t('nav.duplicatePersonalView') }}
                  </button>
                  <details v-if="canManageView(item)">
                    <summary>{{ t('nav.viewActions') }}</summary>
                    <form @submit.prevent="renameView(item)">
                      <label>
                        {{ t('nav.viewName') }}
                        <input
                          v-model="viewNameDrafts[item.id]"
                          :aria-label="t('nav.viewName')"
                          :placeholder="item.name"
                        />
                      </label>
                      <button type="submit" :disabled="viewBusy">
                        {{ t('nav.renameView') }}
                      </button>
                    </form>
                    <button
                      type="button"
                      :disabled="viewBusy"
                      @click="duplicateView(item, 'personal')"
                    >
                      {{ t('nav.duplicatePersonalView') }}
                    </button>
                    <button
                      v-if="canCreateSharedView"
                      type="button"
                      :disabled="viewBusy"
                      @click="duplicateView(item, 'shared')"
                    >
                      {{ t('nav.duplicateSharedView') }}
                    </button>
                    <button
                      v-if="
                        schema.myLevel === 'owner' && item.scope === 'shared'
                      "
                      type="button"
                      :disabled="viewBusy"
                      @click="toggleViewLock(item)"
                    >
                      {{
                        item.locked ? t('nav.unlockView') : t('nav.lockView')
                      }}
                    </button>
                    <button
                      type="button"
                      :disabled="viewBusy"
                      @click="deleteView(item)"
                    >
                      {{ t('nav.deleteView') }}
                    </button>
                  </details>
                </div>
              </nav>
              <form class="view-create" @submit.prevent="createView">
                <label>
                  {{ t('nav.viewName') }}
                  <input v-model="newViewName" :disabled="viewBusy" required />
                </label>
                <label>
                  {{ t('nav.viewScopeLabel') }}
                  <select v-model="newViewScope" :disabled="viewBusy">
                    <option value="personal">
                      {{ t('nav.viewScope.personal') }}
                    </option>
                    <option value="shared" :disabled="!canCreateSharedView">
                      {{ t('nav.viewScope.shared') }}
                    </option>
                  </select>
                </label>
                <label>
                  {{ t('form.viewType') }}
                  <select v-model="newViewType" :disabled="viewBusy">
                    <option value="grid">{{ t('form.gridType') }}</option>
                    <option value="form">{{ t('form.formType') }}</option>
                    <option value="kanban">{{ t('kanban.viewType') }}</option>
                    <option value="gallery">{{ t('gallery.viewType') }}</option>
                  </select>
                </label>
                <button
                  type="submit"
                  :disabled="viewBusy || !newViewName.trim()"
                >
                  {{ t('nav.createView') }}
                </button>
              </form>
              <p v-if="viewError" role="alert">{{ viewError }}</p>
              <p v-if="schema.views.length === 0">{{ t('nav.emptyViews') }}</p>
            </div>
          </details>
        </div>
        <details v-if="view?.type === 'grid' && !desktop" class="view-filter">
          <summary>{{ t('grid.filter') }}</summary>
          <FilterBuilder
            :key="`${schema.table.id}:${view.id}`"
            v-model="filterDraft"
            :schema="schema"
          />
          <button type="button" @click="applyFilter">
            {{ t('filter.apply') }}
          </button>
        </details>
        <button
          v-if="canSaveAsPersonalView"
          type="button"
          :disabled="viewBusy"
          @click="saveAsPersonalView"
        >
          {{ t('nav.saveAsPersonalView') }}
        </button>
        <FormBuilder
          v-if="view?.type === 'form' && canManageView(view)"
          :schema="schema"
          :view="view"
          :busy="viewBusy"
          @save="saveForm"
        />
        <KanbanBuilder
          v-if="view?.type === 'kanban' && canManageView(view)"
          :schema="schema"
          :view="view"
          :busy="viewBusy"
          @save="(config) => saveCardView('kanban', config)"
        />
        <GalleryBuilder
          v-if="view?.type === 'gallery' && canManageView(view)"
          :schema="schema"
          :view="view"
          :busy="viewBusy"
          @save="(config) => saveCardView('gallery', config)"
        />
        <FieldPanel
          v-if="desktop && view?.type !== 'grid'"
          :key="schema.table.id"
          :schema="schema"
          @update="applySchema"
          @reload="loadSchema(true)"
        />
        <div ref="rowWorkspace" class="row-workspace">
          <div
            v-if="view?.type === 'grid' && desktop"
            ref="gridWorkspace"
            class="grid-workspace"
          >
            <DataGrid
              ref="grid"
              :key="`${schema.table.id}:${view.id}:${schema.schemaVersion}:${auth.user?.id ?? ''}`"
              :schema="schema"
              :view-id="view.id"
              :identity="auth.user?.id ?? ''"
              :filter="filterValue"
              :fields-open="fieldsOpen"
              :filter-open="filterOpen"
              :filter-count="filterCount"
              @toggle-fields="toggleFieldsMenu"
              @toggle-filter="toggleFilterMenu"
              @create-field="openFieldCreate"
              @schema-changed="loadSchema(true)"
              @view-updated="
                (updated) => {
                  if (schema)
                    schema.views = schema.views.map((item) =>
                      item.id === updated.id ? updated : item,
                    )
                }
              "
              @view-config-changed="
                (config, hasLocalChanges) => {
                  localViewConfig = config
                  hasLocalViewChanges = hasLocalChanges
                }
              "
              @expand="
                (id) =>
                  router.push(`/t/${schema!.table.id}/v/${view!.id}/r/${id}`)
              "
            />
            <div v-show="fieldsOpen" class="grid-field-popover">
              <FieldPanel
                v-if="fieldsVisited"
                :key="schema.table.id"
                ref="fieldPanel"
                :schema="schema"
                :columns="localViewConfig?.fields"
                menu
                @visibility="toggleGridField"
                @update="applySchema"
                @reload="loadSchema(true)"
              />
              <p v-if="fieldMenuError" role="alert">{{ fieldMenuError }}</p>
            </div>
            <div v-show="filterOpen" class="grid-filter-popover">
              <FilterBuilder
                v-if="filterVisited"
                :key="`${schema.table.id}:${view.id}`"
                v-model="filterDraft"
                :schema="schema"
                compact
              />
              <button type="button" @click="applyFilter">
                {{ t('filter.apply') }}
              </button>
            </div>
          </div>
          <MobileCardList
            v-else-if="view?.type === 'grid'"
            ref="grid"
            :key="`${schema.table.id}:${view.id}:${auth.user?.id ?? ''}`"
            :schema="schema"
            :view-id="view.id"
            :identity="auth.user?.id ?? ''"
            :filter="filterValue"
            @schema-changed="loadSchema(true)"
            @expand="
              (id) =>
                router.push(`/t/${schema!.table.id}/v/${view!.id}/r/${id}`)
            "
            @add="newRow = true"
          >
            <FieldPanel
              :schema="schema"
              @update="applySchema"
              @reload="loadSchema(true)"
            />
          </MobileCardList>
          <FormView
            v-else-if="view?.type === 'form'"
            :key="`${schema.table.id}:${view.id}:${auth.user?.id ?? ''}`"
            :schema="schema"
            :view="view"
            :identity="auth.user?.id ?? ''"
            @schema-changed="loadSchema(true)"
          />
          <KanbanView
            v-else-if="view?.type === 'kanban'"
            ref="kanban"
            :key="`${schema.table.id}:${view.id}:${schema.schemaVersion}:${auth.user?.id ?? ''}`"
            :schema="schema"
            :view="view"
            :identity="auth.user?.id ?? ''"
            @schema-changed="loadSchema(true)"
            @expand="
              (id) =>
                router.push(`/t/${schema!.table.id}/v/${view!.id}/r/${id}`)
            "
          />
          <GalleryView
            v-else-if="view?.type === 'gallery'"
            ref="gallery"
            :key="`${schema.table.id}:${view.id}:${schema.schemaVersion}:${auth.user?.id ?? ''}`"
            :schema="schema"
            :view="view"
            :identity="auth.user?.id ?? ''"
            @schema-changed="loadSchema(true)"
            @expand="
              (id) =>
                router.push(`/t/${schema!.table.id}/v/${view!.id}/r/${id}`)
            "
          />
          <RowExpandDialog
            v-if="showRowDialog && view"
            :schema="schema"
            :view-id="view.id"
            :row-id="
              typeof route.params.rowId === 'string' ? route.params.rowId : null
            "
            :identity="auth.user?.id ?? ''"
            :workspace="rowWorkspace"
            :navigate-row="rowNavigator"
            :focus-comment="
              typeof route.query.comment === 'string'
                ? route.query.comment
                : undefined
            "
            @close="
              () => {
                newRow = false
                router.replace(`/t/${schema!.table.id}/v/${view!.id}`)
              }
            "
            @saved="refreshVisibleRows"
            @navigate="
              (id) =>
                router.replace(`/t/${schema!.table.id}/v/${view!.id}/r/${id}`)
            "
            @created="
              (id) => {
                newRow = false
                refreshVisibleRows()
                router.replace(`/t/${schema!.table.id}/v/${view!.id}/r/${id}`)
              }
            "
            @schema-changed="loadSchema(true)"
          />
        </div>
      </template>
      <ImportDialog
        v-if="importOpen && (kind === 'book' || kind === 'table')"
        :key="`${kind}:${node.id}`"
        :target-kind="kind"
        :target-id="node.id"
        :schema="schema"
        @close="importOpen = false"
        @completed="imported"
        @open-table="openImportedTable"
      />
    </template>
    <button
      v-else-if="nav.error"
      :disabled="nav.loading"
      @click="nav.refresh(true)"
    >
      {{ t('auth.retry') }}
    </button>
  </section>
</template>

<style scoped>
.resource-heading {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
}

.table-utility {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.table-management {
  position: relative;
  z-index: 4;
}

.table-management > summary {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  background: var(--panel-surface);
  border: 1px solid var(--border);
  border-radius: 6px;
  cursor: pointer;
  list-style: none;
}

.table-management > summary::-webkit-details-marker {
  display: none;
}

.table-management-panel {
  position: absolute;
  right: 0;
  z-index: 5;
  width: min(460px, calc(100vw - 32px));
  max-height: 70vh;
  padding: 16px;
  overflow: auto;
  background: var(--panel-surface);
  border: 1px solid var(--border);
  border-radius: 10px;
  box-shadow: 0 8px 24px rgb(0 0 0 / 14%);
}

.table-management-panel .view-links,
.table-management-panel .view-create {
  display: grid;
  gap: 10px;
  margin-top: 16px;
}

.table-management-panel .view-links > div {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.table-management-panel .view-create label {
  display: grid;
  gap: 4px;
}

.grid-workspace {
  position: relative;
  min-width: 0;
}

.grid-field-popover,
.grid-filter-popover {
  position: absolute;
  z-index: 10;
  top: 48px;
  max-height: min(75vh, 700px);
  overflow: auto;
  color: var(--text);
  background: var(--panel-surface);
  border: 1px solid var(--border);
  border-radius: 10px;
  box-shadow: 0 8px 24px rgb(0 0 0 / 14%);
}

.grid-field-popover {
  left: 0;
  width: min(390px, 100%);
}

.grid-filter-popover {
  left: min(100px, 5%);
  width: min(720px, 95%);
  padding: 18px;
}

.grid-filter-popover > button {
  margin-top: 14px;
  color: var(--panel-surface);
  background: var(--accent);
  border-color: var(--accent);
}
</style>
