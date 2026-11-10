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
import { createResourceLabel } from '../i18n'
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
import { useRealtimeStore } from '../stores/realtime'
import { useUndoStore, viewUndoStep } from '../stores/undo'
import {
  canManageView as isViewManageable,
  useViewManagementStore,
} from '../stores/viewManagement'
import AppIcon from '../components/AppIcon.vue'
import TableHistoryControls from '../components/navigation/TableHistoryControls.vue'
import TableToolbarActions from '../components/navigation/TableToolbarActions.vue'
import VisibilityMenu from '../components/sharing/VisibilityMenu.vue'
import { useResourceSearch } from '../composables/useResourceSearch'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const nav = useNavStore()
const realtime = useRealtimeStore()
const undo = useUndoStore()
const viewManagement = useViewManagementStore()
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
  flushColumnChanges?: () => Promise<boolean>
  reorderFields?: (ids: string[]) => Promise<boolean>
  navigateRow: (id: string, direction: -1 | 1) => Promise<string | null>
}>()
const rowWorkspace = ref<HTMLElement>()
const gridWorkspace = ref<HTMLElement>()
const gridToolbarBottom = ref(0)
const mobileFilter = ref<HTMLDetailsElement>()
type FieldPlacement = { anchorFieldId: string; side: 'left' | 'right' }
type HeaderFieldAction =
  | { kind: 'edit'; id: string; options?: { focus?: 'description' } }
  | { kind: 'duplicate' | 'delete' | 'display'; id: string }
  | { kind: 'insert'; id: string; side: 'left' | 'right' }
const fieldPanel = ref<{
  openCreate: (placement?: FieldPlacement) => Promise<void>
  openEditor: (id: string, options?: { focus?: 'description' }) => Promise<void>
  duplicate: (id: string) => Promise<void>
  askDelete: (id: string) => Promise<void>
  setDisplay: (id: string) => Promise<void>
}>()
const filterPanel = ref<{ addConditionForField: (id: string) => boolean }>()
const pendingHeaderAction = shallowRef<HeaderFieldAction | null>(null)
const pendingFilterField = ref<string | null>(null)
const filterMenuError = ref('')
const fieldsOpen = ref(false)
const filterOpen = ref(false)
const fieldsVisited = ref(false)
const filterVisited = ref(false)
const fieldMenuError = ref('')
const fieldReordering = ref(false)
let reorderOrigin:
  { tableId: string; viewId: string; identity: string } | undefined
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
let personalSaveGeneration = 0
let toolbarObserver: ResizeObserver | undefined
function measureGridToolbar() {
  const workspace = gridWorkspace.value
  const toolbar = workspace?.querySelector('.grid-toolbar')
  gridToolbarBottom.value =
    workspace && toolbar
      ? Math.max(
          0,
          toolbar.getBoundingClientRect().bottom -
            workspace.getBoundingClientRect().top,
        )
      : 0
}
function observeGridToolbar() {
  toolbarObserver?.disconnect()
  const toolbar = gridWorkspace.value?.querySelector('.grid-toolbar')
  if (toolbar && typeof ResizeObserver !== 'undefined') {
    toolbarObserver = new ResizeObserver(measureGridToolbar)
    toolbarObserver.observe(toolbar)
  }
  measureGridToolbar()
}
watch([gridWorkspace, grid], observeGridToolbar, { flush: 'post' })
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
  if (!target.closest('.view-filter') && mobileFilter.value)
    mobileFilter.value.open = false
}
function closeGridMenus(event: KeyboardEvent) {
  if (
    event.key !== 'Escape' ||
    document.querySelector('dialog[open]') ||
    (!fieldsOpen.value && !filterOpen.value && !mobileFilter.value?.open)
  )
    return
  if (mobileFilter.value?.open) {
    mobileFilter.value.open = false
    mobileFilter.value.querySelector<HTMLElement>('summary')?.focus()
  } else {
    const trigger = fieldsOpen.value
      ? '.grid-fields-trigger'
      : '.grid-filter-trigger'
    gridWorkspace.value?.querySelector<HTMLButtonElement>(trigger)?.focus()
  }
  fieldsOpen.value = false
  filterOpen.value = false
  createFieldPending.value = false
  event.preventDefault()
}
function closeToolbarMenus(toolbar: Element | null | undefined) {
  for (const menu of toolbar?.querySelectorAll<HTMLDetailsElement>(
    ':scope > details[open], .table-toolbar-actions > details[open], .grid-toolbar-end > details[open]',
  ) ?? [])
    menu.open = false
}
function activateToolbarMenu(event: MouseEvent) {
  if (!(event.target instanceof Element)) return
  const menu = event.target.closest('summary')?.parentElement
  if (!(menu instanceof HTMLDetailsElement) || menu.open) return
  const toolbar = menu.closest('.grid-toolbar, .table-toolbar')
  if (
    !toolbar ||
    (menu.parentElement !== toolbar &&
      !menu.parentElement?.classList.contains('table-toolbar-actions') &&
      !menu.parentElement?.classList.contains('grid-toolbar-end'))
  )
    return
  closeToolbarMenus(toolbar)
  if (toolbar.classList.contains('grid-toolbar')) {
    fieldsOpen.value = false
    filterOpen.value = false
    createFieldPending.value = false
  }
}
function toggleFieldsMenu() {
  fieldsOpen.value = !fieldsOpen.value
  if (fieldsOpen.value) {
    fieldsVisited.value = true
    closeToolbarMenus(gridWorkspace.value?.querySelector('.grid-toolbar'))
  } else createFieldPending.value = false
  filterOpen.value = false
}
function toggleFilterMenu() {
  filterOpen.value = !filterOpen.value
  if (filterOpen.value) {
    filterVisited.value = true
    closeToolbarMenus(gridWorkspace.value?.querySelector('.grid-toolbar'))
  }
  fieldsOpen.value = false
  createFieldPending.value = false
}
async function openFieldCreate() {
  closeToolbarMenus(gridWorkspace.value?.querySelector('.grid-toolbar'))
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
function runHeaderFieldAction() {
  const action = pendingHeaderAction.value
  const panel = fieldPanel.value
  if (!action || !panel || !fieldsOpen.value) return
  pendingHeaderAction.value = null
  switch (action.kind) {
    case 'edit':
      void panel.openEditor(action.id, action.options)
      break
    case 'duplicate':
      void panel.duplicate(action.id)
      break
    case 'delete':
      void panel.askDelete(action.id)
      break
    case 'display':
      void panel.setDisplay(action.id)
      break
    case 'insert':
      void panel.openCreate({ anchorFieldId: action.id, side: action.side })
      break
  }
}
async function openHeaderFieldAction(action: HeaderFieldAction) {
  if (schema.value?.myLevel !== 'owner') return
  closeToolbarMenus(gridWorkspace.value?.querySelector('.grid-toolbar'))
  fieldsOpen.value = true
  fieldsVisited.value = true
  filterOpen.value = false
  createFieldPending.value = false
  pendingHeaderAction.value = action
  await nextTick()
  runHeaderFieldAction()
}
function runPendingFilter() {
  const fieldId = pendingFilterField.value
  if (!fieldId || !filterPanel.value || !filterOpen.value) return
  pendingFilterField.value = null
  filterMenuError.value = filterPanel.value.addConditionForField(fieldId)
    ? ''
    : t('gridPolish.gp12.fieldUnavailable')
}
async function openFilterForField(id: string) {
  closeToolbarMenus(gridWorkspace.value?.querySelector('.grid-toolbar'))
  fieldsOpen.value = false
  filterOpen.value = true
  filterVisited.value = true
  createFieldPending.value = false
  filterMenuError.value = ''
  pendingFilterField.value = id
  await nextTick()
  runPendingFilter()
}
watch(
  fieldsOpen,
  (open) => {
    if (!open) pendingHeaderAction.value = null
  },
  { flush: 'sync' },
)
watch(
  filterOpen,
  (open) => {
    if (!open) pendingFilterField.value = null
  },
  { flush: 'sync' },
)
function toggleGridField(id: string, visible: boolean) {
  if (!grid.value?.toggleField) {
    fieldMenuError.value = t('grid.loading')
    return
  }
  fieldMenuError.value = ''
  grid.value.toggleField(id, visible)
}
function isReorderOriginCurrent() {
  return (
    !!reorderOrigin &&
    route.params.tableId === reorderOrigin.tableId &&
    schema.value?.table.id === reorderOrigin.tableId &&
    view.value?.id === reorderOrigin.viewId &&
    auth.user?.id === reorderOrigin.identity
  )
}
async function prepareFieldReorder() {
  const current = grid.value
  const tableId = schema.value?.table.id
  const viewId = view.value?.id
  const identity = auth.user?.id
  if (!current?.flushColumnChanges || !tableId || !viewId || !identity)
    return false
  fieldReordering.value = true
  reorderOrigin = { tableId, viewId, identity }
  const saved = await current.flushColumnChanges()
  return saved && isReorderOriginCurrent()
}
async function persistFieldReorder(ids: string[]) {
  if (!isReorderOriginCurrent() || !grid.value?.reorderFields) return false
  const saved = await grid.value.reorderFields(ids)
  return saved && isReorderOriginCurrent()
}
function finishFieldReorder() {
  reorderOrigin = undefined
  fieldReordering.value = false
}
watch(fieldPanel, (panel) => {
  if (!panel) return
  if (createFieldPending.value) {
    createFieldPending.value = false
    void panel.openCreate()
  }
  runHeaderFieldAction()
})
watch(filterPanel, runPendingFilter)
onMounted(() => {
  media.addEventListener('change', resized)
  window.addEventListener('resize', measureGridToolbar)
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
const searchableWorkbooks = computed(() =>
  kind.value === 'workspace'
    ? (node.value?.children ?? []).map((book, index) => ({ ...book, index }))
    : [],
)
const orderedWorkbooks = useResourceSearch(searchableWorkbooks, ref(''))
const showEmptyBooksHint = computed(
  () =>
    kind.value === 'workspace' &&
    node.value?.children.length === 0 &&
    !nav.error &&
    node.value?.canCreate === true,
)
const defaultViewId = computed(() => {
  const views = schema.value?.views ?? []
  return (views.find((item) => item.type === 'grid') ?? views[0])?.id
})
// The bare table route resolves to its default view before URL normalization.
const viewId = computed(() =>
  typeof route.params.viewId === 'string'
    ? route.params.viewId
    : defaultViewId.value,
)
const view = computed(() =>
  schema.value?.views.find((item) => item.id === viewId.value),
)
watch(
  [() => route.params.tableId, () => route.params.viewId, () => view.value?.id],
  ([tableId, routeViewId, resolvedViewId]) => {
    if (
      typeof tableId === 'string' &&
      typeof routeViewId !== 'string' &&
      typeof resolvedViewId === 'string' &&
      schema.value?.table.id === tableId
    )
      void router.replace({
        path: `/t/${tableId}/v/${resolvedViewId}`,
        query: route.query,
        hash: route.hash,
      })
  },
  { immediate: true },
)
const canCreateRow = computed(
  () =>
    !!auth.user?.id &&
    view.value?.type === 'grid' &&
    (schema.value?.myLevel === 'write' || schema.value?.myLevel === 'owner'),
)
const showRowDialog = computed(
  () =>
    (view.value?.type === 'grid' ||
      view.value?.type === 'kanban' ||
      view.value?.type === 'gallery') &&
    (typeof route.params.rowId === 'string' || newRow.value),
)

function openNewRow() {
  if (!canCreateRow.value || typeof route.params.rowId === 'string') return
  closeToolbarMenus(gridWorkspace.value?.querySelector('.grid-toolbar'))
  fieldsOpen.value = false
  filterOpen.value = false
  createFieldPending.value = false
  if (mobileFilter.value) mobileFilter.value.open = false
  newRow.value = true
}

function closeRowDialog() {
  newRow.value = false
  if (typeof route.params.rowId === 'string' && schema.value && view.value)
    void router.replace(`/t/${schema.value.table.id}/v/${view.value.id}`)
}

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
  const previousViews = new Map(shown.views.map((item) => [item.id, item]))
  next = {
    ...next,
    views: next.views.map((item) => {
      const previous = previousViews.get(item.id)
      return previous && previous.version > item.version ? previous : item
    }),
  }
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
  (next, previous) => {
    const defaultId = defaultViewId.value
    if (
      defaultId &&
      schema.value?.table.id === next[0] &&
      next[1] !== previous[1] &&
      next.every((value, index) => index === 1 || value === previous[index]) &&
      (next[1] ?? defaultId) === (previous[1] ?? defaultId)
    )
      return
    void loadSchema()
  },
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
    if (mobileFilter.value) mobileFilter.value.open = false
    fieldsVisited.value = false
    filterVisited.value = false
    createFieldPending.value = false
    fieldMenuError.value = ''
    personalSaveGeneration++
    viewBusy.value = false
    viewError.value = ''
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
  return isViewManageable(item, schema.value?.myLevel)
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

async function saveAsPersonalView() {
  if (
    !schema.value ||
    !view.value ||
    view.value.scope !== 'shared' ||
    viewBusy.value ||
    viewManagement.busy
  )
    return
  const tableId = schema.value.table.id
  const source = view.value
  const origin = route.fullPath
  const identity = auth.user?.id
  const current = ++personalSaveGeneration
  const stillCurrent = () =>
    current === personalSaveGeneration &&
    route.fullPath === origin &&
    auth.user?.id === identity &&
    schema.value?.table.id === tableId &&
    view.value?.id === source.id
  viewBusy.value = true
  viewError.value = ''
  try {
    const localConfig = localViewConfig.value
    if (!localConfig) throw new RequestError('ERR_INTERNAL', 500)
    const config = {
      ...localConfig,
      ...(filterApplied.value ? { filter: filterValue.value } : {}),
    }
    const loaded = await viewManagement.select(tableId, source.id, stillCurrent)
    if (!loaded) {
      if (stillCurrent())
        viewRequestFailed(
          viewManagement.loadError ?? new RequestError('ERR_INTERNAL', 500),
        )
      return
    }
    const result = await viewManagement.duplicate(
      t('nav.viewCopyName', { name: source.name }),
      'personal',
      config,
    )
    if (!result) {
      if (stillCurrent())
        viewRequestFailed(
          viewManagement.error ??
            viewManagement.loadError ??
            new RequestError('ERR_INTERNAL', 500),
        )
      return
    }
    if (stillCurrent()) await router.push(`/t/${tableId}/v/${result.view.id}`)
  } catch (cause) {
    if (stillCurrent()) viewRequestFailed(cause)
  } finally {
    if (current === personalSaveGeneration) viewBusy.value = false
  }
}
onBeforeUnmount(() => {
  personalSaveGeneration++
  generation++
  controller?.abort()
  exportGeneration++
  exportController?.abort()
  toolbarObserver?.disconnect()
  media.removeEventListener('change', resized)
  window.removeEventListener('resize', measureGridToolbar)
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
    <div v-else-if="kind === 'workspace' && !node && nav.error" role="alert">
      <h1>{{ t('nav.unavailable') }}</h1>
      <p>{{ t('nav.stale') }}</p>
      <p>{{ errorMessage(nav.error) }}</p>
      <button :disabled="nav.loading" @click="nav.refresh(true)">
        {{ t('auth.retry') }}
      </button>
    </div>
    <template v-else-if="node">
      <div
        v-if="kind === 'workspace' && nav.error"
        class="workbook-stale"
        role="status"
      >
        <p>{{ t('nav.stale') }}</p>
        <button :disabled="nav.loading" @click="nav.refresh(true)">
          {{ t('auth.retry') }}
        </button>
      </div>
      <header v-if="kind !== 'table'" class="resource-heading">
        <h1>{{ schema?.table.name ?? node.name }}</h1>
      </header>
      <p v-if="node.pathOnly">{{ t('nav.pathNotice') }}</p>
      <template v-if="kind !== 'table'">
        <p
          v-if="
            node.children.length === 0 && (kind !== 'workspace' || !nav.error)
          "
        >
          {{
            kind === 'workspace' ? t('nav.emptyBooks') : t('nav.emptyTables')
          }}
        </p>
        <p v-if="showEmptyBooksHint">
          {{ t('wmb.emptyBooksHint') }}
        </p>
        <ul
          v-if="kind === 'workspace' && orderedWorkbooks.length"
          class="workbook-grid"
        >
          <li v-for="child in orderedWorkbooks" :key="child.id">
            <RouterLink :to="child.path" class="workbook-card">
              <span class="workbook-card-icon">
                <AppIcon
                  v-if="child.bookIcon !== null"
                  :name="child.bookIcon"
                  :size="28"
                />
              </span>
              <span class="workbook-card-content">
                <span class="workbook-card-name">{{ child.name }}</span>
                <span
                  v-if="!child.pathOnly && child.description"
                  class="workbook-card-description"
                  :title="child.description"
                >
                  {{ child.description }}
                </span>
                <span v-if="child.pathOnly" class="workbook-card-context">
                  {{ t('nav.pathOnly') }}
                </span>
              </span>
            </RouterLink>
          </li>
        </ul>
        <ul v-else-if="kind === 'book'" class="resource-list">
          <li v-for="child in node.children" :key="child.id">
            <RouterLink :to="child.path">{{ child.name }}</RouterLink>
            <span v-if="child.pathOnly">{{ t('nav.pathOnly') }}</span>
          </li>
        </ul>
        <button
          v-if="kind !== 'workspace' && node.canCreate && node.childKind"
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
        <div
          v-if="view?.type !== 'grid' || !desktop"
          class="table-toolbar"
          role="toolbar"
          :aria-label="t('nav.viewTools')"
          @click.capture="activateToolbarMenu"
        >
          <TableHistoryControls />
          <details
            v-if="view?.type === 'grid'"
            ref="mobileFilter"
            class="view-filter"
          >
            <summary>{{ t('grid.filter') }}</summary>
            <div class="view-filter-panel">
              <FilterBuilder
                :key="`${schema.table.id}:${view.id}`"
                v-model="filterDraft"
                :schema="schema"
              />
              <button type="button" @click="applyFilter">
                {{ t('filter.apply') }}
              </button>
            </div>
          </details>
          <VisibilityMenu v-if="canShare" :target="node" placement="utility" />
          <div class="table-toolbar-end">
            <TableToolbarActions
              :schema="schema"
              :view="view"
              :identity="auth.user?.id ?? ''"
              :export-busy="exportBusy"
              :export-error="exportError"
              @import="importOpen = true"
              @export="downloadExport"
            />
          </div>
        </div>
        <button
          v-if="canSaveAsPersonalView"
          type="button"
          :disabled="viewBusy || viewManagement.busy"
          @click="saveAsPersonalView"
        >
          {{ t('nav.saveAsPersonalView') }}
        </button>
        <p v-if="viewError" role="alert">{{ viewError }}</p>
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
            @click.capture="activateToolbarMenu"
          >
            <DataGrid
              ref="grid"
              :key="`${schema.table.id}:${view.id}:${schema.schemaVersion}:${auth.user?.id ?? ''}`"
              :schema="schema"
              :view-id="view.id"
              :identity="auth.user?.id ?? ''"
              :inert="fieldReordering ? '' : undefined"
              :aria-busy="fieldReordering"
              :filter="filterValue"
              :fields-open="fieldsOpen"
              :filter-open="filterOpen"
              :filter-count="filterCount"
              @toggle-fields="toggleFieldsMenu"
              @toggle-filter="toggleFilterMenu"
              @create-field="openFieldCreate"
              @edit-field="
                (id, options) =>
                  openHeaderFieldAction({ kind: 'edit', id, options })
              "
              @duplicate-field="
                (id) => openHeaderFieldAction({ kind: 'duplicate', id })
              "
              @insert-field="
                (id, side) =>
                  openHeaderFieldAction({ kind: 'insert', id, side })
              "
              @delete-field="
                (id) => openHeaderFieldAction({ kind: 'delete', id })
              "
              @set-display="
                (id) => openHeaderFieldAction({ kind: 'display', id })
              "
              @filter-field="openFilterForField"
              @schema-changed="loadSchema(true)"
              @view-updated="
                (updated) => {
                  if (schema)
                    schema.views = schema.views.map((item) =>
                      item.id === updated.id && updated.version >= item.version
                        ? updated
                        : item,
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
            >
              <template #toolbar-leading>
                <TableHistoryControls />
              </template>
              <template #after-highlight>
                <VisibilityMenu v-if="canShare && node" :target="node" />
              </template>
              <template #toolbar-actions>
                <TableToolbarActions
                  :schema="schema"
                  :view="view"
                  :identity="auth.user?.id ?? ''"
                  :export-busy="exportBusy"
                  :export-error="exportError"
                  @import="importOpen = true"
                  @export="downloadExport"
                />
              </template>
              <template #toolbar-end>
                <button
                  v-if="canCreateRow"
                  class="grid-toolbar-button new-entry"
                  type="button"
                  @click="openNewRow"
                >
                  <AppIcon name="plus" />
                  {{ t('gridPolish.gp10.new') }}
                </button>
              </template>
            </DataGrid>
            <div
              v-show="fieldsOpen"
              class="grid-field-popover"
              :style="{ top: `${gridToolbarBottom}px` }"
            >
              <FieldPanel
                v-if="fieldsVisited"
                :key="schema.table.id"
                ref="fieldPanel"
                :schema="schema"
                :columns="localViewConfig?.fields"
                :prepare-reorder="prepareFieldReorder"
                :persist-reorder="persistFieldReorder"
                :finish-reorder="finishFieldReorder"
                menu
                @visibility="toggleGridField"
                @update="applySchema"
                @reload="loadSchema(true)"
              />
              <p v-if="fieldMenuError" role="alert">{{ fieldMenuError }}</p>
            </div>
            <div
              v-show="filterOpen"
              class="grid-filter-popover"
              :style="{ top: `${gridToolbarBottom}px` }"
            >
              <FilterBuilder
                v-if="filterVisited"
                :key="`${schema.table.id}:${view.id}`"
                ref="filterPanel"
                v-model="filterDraft"
                :schema="schema"
                compact
              />
              <p v-if="filterMenuError" role="alert">{{ filterMenuError }}</p>
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
            @add="openNewRow"
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
            @close="closeRowDialog"
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

.workbook-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 220px), 1fr));
  gap: 16px;
  margin: 20px 0;
  padding: 0;
  list-style: none;
}

.workbook-grid > li {
  min-width: 0;
}

.workbook-card {
  display: flex;
  align-items: center;
  gap: 14px;
  min-width: 0;
  min-height: 100px;
  height: 100%;
  padding: 16px;
  color: var(--text);
  text-decoration: none;
  background: var(--panel-surface);
  border: 1px solid var(--border);
  border-radius: 10px;
}

.workbook-card:hover {
  border-color: var(--accent);
}

.workbook-card:focus-visible {
  outline: 3px solid var(--accent);
  outline-offset: 3px;
}

.workbook-card-icon {
  display: grid;
  flex: 0 0 48px;
  place-items: center;
  width: 48px;
  height: 48px;
  color: var(--accent);
  background: color-mix(in srgb, var(--accent) 12%, var(--panel-surface));
  border-radius: 10px;
}

.workbook-card-content {
  display: grid;
  flex: 1 1 0;
  grid-template-columns: minmax(0, 1fr);
  min-width: 0;
  gap: 6px;
}

.workbook-card-name {
  min-width: 0;
  overflow-wrap: anywhere;
  font-weight: 600;
}

.workbook-card-description {
  min-width: 0;
  overflow: hidden;
  color: var(--muted);
  font-size: 0.875rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.workbook-card-context {
  color: var(--muted);
  font-size: 0.875rem;
}

.table-toolbar {
  position: relative;
  z-index: 3;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
  min-height: 48px;
  margin-bottom: 8px;
  border-bottom: 1px solid var(--border);
}

.table-toolbar-end {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
  min-width: 0;
  max-width: 100%;
  margin-left: auto;
}

/* Mirrors DataGrid's scoped `.grid-toolbar-button` (Fields, Filter). */
.new-entry {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  min-height: 40px;
  padding: 8px 6px;
  color: var(--text);
  background: transparent;
  border: 1px solid transparent;
  border-radius: 6px;
  cursor: pointer;
  white-space: nowrap;
}

.new-entry:hover {
  background: color-mix(in srgb, var(--accent) 10%, transparent);
}

.new-entry:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 2px;
}

.view-filter > summary {
  display: inline-flex;
  align-items: center;
  min-height: 40px;
  padding: 8px 10px;
  background: transparent;
  border: 1px solid transparent;
  border-radius: 6px;
  cursor: pointer;
  list-style: none;
}

.view-filter > summary::-webkit-details-marker {
  display: none;
}

.view-filter-panel {
  position: absolute;
  top: 100%;
  left: 0;
  z-index: 15;
  width: min(720px, 100%);
  max-height: 70vh;
  padding: 16px;
  overflow: auto;
  color: var(--text);
  background: var(--panel-surface);
  border: 1px solid var(--border);
  border-radius: 10px;
  box-shadow: 0 8px 24px rgb(0 0 0 / 14%);
}

.view-filter-panel > button {
  margin-top: 14px;
}

.grid-workspace {
  position: relative;
  min-width: 0;
}

.grid-field-popover,
.grid-filter-popover {
  position: absolute;
  z-index: 10;
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
