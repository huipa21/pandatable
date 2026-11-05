<script setup lang="ts">
import {
  computed,
  defineComponent,
  h,
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  shallowRef,
  watch,
  type PropType,
} from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import RevoGrid, { useTemplates, VGridVueEditor } from '@revolist/vue3-datagrid'
import type { EditorType } from '@revolist/vue3-datagrid'
import { ColumnMovePlugin } from '@revolist/revogrid'
import type {
  BeforeSaveDataDetails,
  BeforeRangeSaveDataDetails,
  CellTemplateProp,
  ColumnRegular,
  Components,
  ColumnDragEventData,
  EditorCtrCallable,
  FocusAfterRenderEvent,
  RangeArea,
  ViewPortScrollEvent,
} from '@revolist/revogrid'
import {
  discardAttachment,
  errorMessage,
  request,
  RequestError,
} from '../../api/client'
import type {
  DataRow,
  ExportViewQuery,
  FilterNode,
  HighlightColor,
  HighlightRule,
  RowMutation,
  RowPatchMutation,
  RowQueryRequest,
  RowQueryResult,
  RowAggregate,
  RowAggregateResult,
  RowGroupResult,
  RowSort,
  RowValue,
  TableField,
  TableSchema,
  TableView,
  ViewAggregate,
  ViewColumns,
} from '../../api/types'
import { bindField, type BoundField } from '../../fields/registry'
import { fieldErrorMessage } from '../../fields/messages'
import { isWireValue, type FieldEnv, type WireValue } from '../../fields/types'
import { useAuthStore } from '../../stores/auth'
import { useNavStore } from '../../stores/nav'
import { usePreferencesStore } from '../../stores/preferences'
import { useRealtimeStore, type RealtimeEvent } from '../../stores/realtime'
import {
  patchUndoStep,
  useUndoStore,
  viewUndoStep,
  type RowValues,
  type UndoStep,
} from '../../stores/undo'
import GridCellEditor from './GridCellEditor.vue'
import GridCellContent from './GridCellContent.vue'
import AppIcon from '../AppIcon.vue'
import FilterBuilder from '../filters/FilterBuilder.vue'
import GridRelationCell from '../relations/GridRelationCell.vue'
import RelationPicker from '../relations/RelationPicker.vue'
import { RemoteRowsWindow, remotePageSize } from './remoteWindow'
import { RealtimeRowSync } from './realtimeRows'
import { GroupedRowsWindow, type GroupNode } from './groupedRows'
import { isGridScalar } from './inlineEditing'
import { formatTsv, parseTsv, TsvError } from './tsv'
import {
  isCompleteFilter,
  useViewColumns,
  minColumnWidth,
  maxColumnWidth,
} from './viewColumns'

const props = defineProps<{
  schema: TableSchema
  viewId: string
  identity: string
  filter?: FilterNode | null
  fieldsOpen?: boolean
  filterOpen?: boolean
  filterCount?: number
}>()

function attachmentIds(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  return value.flatMap((item) =>
    typeof item === 'object' &&
    item !== null &&
    'id' in item &&
    typeof item.id === 'string'
      ? [item.id]
      : [],
  )
}
const emit = defineEmits<{
  schemaChanged: []
  expand: [rowId: string]
  viewUpdated: [view: TableView]
  viewConfigChanged: [config: ViewColumns, hasLocalChanges: boolean]
  toggleFields: []
  toggleFilter: []
  createField: []
}>()
const { t, locale } = useI18n()
const { template } = useTemplates()
const router = useRouter()
const auth = useAuthStore()
const nav = useNavStore()
const preferences = usePreferencesStore()
const realtime = useRealtimeStore()
const undo = useUndoStore()
const host = ref<{ $el: HTMLElement & Components.RevoGrid }>()
const toolbar = ref<HTMLElement>()
const ready = ref(false)
const disposed = ref(false)
const loading = ref(false)
const saving = ref(false)
const error = ref<unknown>(null)
const searchTerm = ref('')
const footerAggregates = shallowRef<RowAggregate[]>([])
const footerAggregateError = ref('')
const footerAggregateLoading = ref(false)
const cellErrors = ref(new Map<string, string>())
interface CellDraft {
  text: string
  pending: boolean
  revision: number
}
interface CellIntent {
  value: WireValue
  revision: number | undefined
  text: string | undefined
  outcome?: { success: true } | { message: string }
}
const cellDrafts = ref(new Map<string, CellDraft>())
const cellIntents = new Map<string, CellIntent>()
const cellControllers = new Set<AbortController>()
let cellDraftRevision = 0
let pendingCellWrites = 0
let cellWriteTail: Promise<void> = Promise.resolve()
const clipboardErrors = ref<string[]>([])
const clipboardMessage = ref('')
const selectedRows = ref(new Set<string>())
const deleteDialog = ref<HTMLDialogElement>()
const addRowsDialog = ref<HTMLDialogElement>()
const relationField = ref<TableField | null>(null)
const relationRow = ref<DataRow | null>(null)
const gridSource = shallowRef<GridModel[]>([])
const columnWidthDrafts = ref<Record<string, string>>({})
const frozenDraft = ref<string | null>(null)
type HighlightDraft = Omit<HighlightRule, 'when'> & { when: FilterNode | null }
const highlightColors: HighlightColor[] = [
  'red-2',
  'orange-2',
  'amber-3',
  'green-2',
  'blue-2',
  'purple-2',
]
const initialViewConfig = props.schema.views.find(
  (view) => view.id === props.viewId,
)?.config
let savedHighlightFingerprint = JSON.stringify(
  initialViewConfig?.highlight ?? [],
)
const syncedPages = new Set<number>()
const syncedRows: (DataRow | null)[] = []
const mutationControllers = new Set<AbortController>()
const activeEdit = shallowRef<{
  rowId: string
  fieldId: string
  changedBy: string
} | null>(null)
let editorBoundary: {
  rowId: string
  fieldId: string
  editor: InstanceType<typeof GridCellEditor>
} | null = null
const deferredRows = new Map<string, DataRow>()
let deferredRemoteChanges = false
let remoteReloadPending = false
let realtimeEpoch = 0
let initializationGeneration = 0
let initializing = false
let focused: FocusAfterRenderEvent | undefined
let footerController: AbortController | undefined
let footerGeneration = 0
let searchTimer: ReturnType<typeof setTimeout> | undefined
let copyRange: RangeArea | null = null
let copyColumnType: FocusAfterRenderEvent['colType'] = 'rgCol'
let cutPending = false
interface PastePlan {
  startRow: number
  addedRows: number
  patchRows: { id: string; index: number; fields: Record<string, RowValue> }[]
  insertRows: { fields: Record<string, RowValue> }[]
}
interface DisplayedFooterAggregate {
  key: string
  field: string
  aggregate: string
  value: string
}
const pendingPaste = ref<PastePlan | null>(null)
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
const gridEditorComponent = defineComponent({
  props: {
    column: {
      type: Object as PropType<EditorType['column']>,
      required: true,
    },
    val: {
      type: [String, Number, Boolean, Array, Object] as PropType<WireValue>,
      default: null,
    },
    save: {
      type: Function as PropType<EditorType['save']>,
      required: true,
    },
    close: {
      type: Function as PropType<EditorType['close']>,
      required: true,
    },
    prop: { type: String, required: true },
    rowIndex: { type: Number, required: true },
  },
  setup(editorProps) {
    const modelId = editorProps.column.model?.__rowId
    let rowId =
      typeof modelId === 'string'
        ? modelId
        : (rows.rowAt(editorProps.rowIndex)?.id ?? '')
    const fieldId = String(editorProps.column.prop)
    const origin = componentEditOrigin
    const cellEditor = ref<InstanceType<typeof GridCellEditor>>()
    const originalDraft = cellDrafts.value.get(`${rowId}:${fieldId}`)
    const originalIntent = originalDraft?.pending
      ? cellIntents.get(`${rowId}:${fieldId}`)
      : undefined
    onMounted(() => {
      const focusedId = focused?.model?.__rowId
      if (!rowId && typeof focusedId === 'string') rowId = focusedId
      if (rowId) {
        activeEdit.value = { rowId, fieldId, changedBy: '' }
        if (cellEditor.value)
          editorBoundary = { rowId, fieldId, editor: cellEditor.value }
        realtime.setFocus(props.schema.table.id, { rowId, fieldId })
      }
    })
    onBeforeUnmount(() => {
      cellEditor.value?.captureBeforeUnmount()
      if (editorBoundary?.editor === cellEditor.value) editorBoundary = null
      if (
        activeEdit.value?.rowId === rowId &&
        activeEdit.value.fieldId === fieldId
      ) {
        activeEdit.value = null
        if (!host.value?.$el.contains(document.activeElement))
          realtime.setFocus(props.schema.table.id, null)
        queueMicrotask(flushDeferredRealtime)
      }
    })
    return () => {
      const item = fieldById.value.get(String(editorProps.column.prop))
      if (!item) {
        error.value = new Error(t('grid.saveFailed'))
        return null
      }
      return h(GridCellEditor, {
        ...editorProps,
        key: `${editorProps.rowIndex}:${String(editorProps.column.prop)}`,
        ref: cellEditor,
        boundField: item.bound,
        fieldEnv: fieldEnvironment.value,
        tableId: props.schema.table.id,
        fieldId: item.field.id,
        identity: props.identity,
        initialDraft: originalDraft?.text,
        canCommit: () => editOriginCurrent(origin),
        unavailableMessage: () => editUnavailableMessage(origin),
        saveDraft: (text: string, pending: boolean, message?: string) => {
          if (sameEditScope(origin))
            storeCellDraft(rowId, fieldId, text, pending, message)
        },
        discardDraft: () => {
          if (sameEditScope(origin))
            discardEditorDraft(rowId, fieldId, originalIntent)
        },
        changedBy:
          activeEdit.value?.rowId === rowId &&
          activeEdit.value.fieldId === fieldId
            ? activeEdit.value.changedBy
            : '',
      })
    }
  },
})
// VGridVueEditor returns a callable factory although its declared type is a union.
const vueGridEditor = VGridVueEditor(gridEditorComponent) as EditorCtrCallable
const gridCellEditor: EditorCtrCallable = (column, save, close) => {
  const adapter = vueGridEditor(column, save, close)
  adapter.beforeDisconnect = () => {
    const rowId = column.model?.__rowId
    if (
      typeof rowId === 'string' &&
      editorBoundary?.rowId === rowId &&
      editorBoundary.fieldId === String(column.prop)
    )
      editorBoundary.editor.leave()
  }
  return adapter
}
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
interface EditOrigin {
  tableId: string
  viewId: string
  identity: string
  schemaVersion: number
  routeTableScoped: boolean
}

const componentEditOrigin = editOrigin()

function editOrigin(): EditOrigin {
  return {
    tableId: props.schema.table.id,
    viewId: props.viewId,
    identity: props.identity,
    schemaVersion: props.schema.schemaVersion,
    routeTableScoped:
      typeof router.currentRoute.value.params.tableId === 'string',
  }
}

function sameEditScope(origin: EditOrigin) {
  return (
    props.schema.table.id === origin.tableId &&
    props.viewId === origin.viewId &&
    props.identity === origin.identity &&
    auth.user?.id === origin.identity
  )
}

function editOriginCurrent(origin: EditOrigin) {
  const params = router.currentRoute.value.params
  return (
    !disposed.value &&
    sameEditScope(origin) &&
    canWrite.value &&
    props.schema.schemaVersion === origin.schemaVersion &&
    (!origin.routeTableScoped ||
      (params.tableId === origin.tableId && params.viewId === origin.viewId))
  )
}

function editUnavailableMessage(origin: EditOrigin) {
  return props.schema.schemaVersion !== origin.schemaVersion
    ? t('errors.ERR_SCHEMA_CHANGED')
    : t('errors.ERR_PERMISSION_DENIED')
}

function storeCellDraft(
  rowId: string,
  fieldId: string,
  text: string,
  pending: boolean,
  message?: string,
) {
  const key = `${rowId}:${fieldId}`
  cellDrafts.value.set(key, { text, pending, revision: ++cellDraftRevision })
  if (message) cellErrors.value.set(key, message)
  else cellErrors.value.delete(key)
}

function discardCellDraft(rowId: string, fieldId: string) {
  const key = `${rowId}:${fieldId}`
  cellDrafts.value.delete(key)
  cellErrors.value.delete(key)
}

function discardEditorDraft(
  rowId: string,
  fieldId: string,
  previous: CellIntent | undefined,
) {
  const key = `${rowId}:${fieldId}`
  if (
    !previous ||
    previous.revision === undefined ||
    previous.text === undefined
  ) {
    discardCellDraft(rowId, fieldId)
    return
  }
  const latest = cellIntents.get(key)
  if (latest && latest !== previous) return
  if (previous.outcome && 'success' in previous.outcome) {
    discardCellDraft(rowId, fieldId)
    return
  }
  cellDrafts.value.set(key, {
    text: previous.text,
    pending: !previous.outcome,
    revision: previous.revision,
  })
  if (previous.outcome) cellErrors.value.set(key, previous.outcome.message)
  else cellErrors.value.delete(key)
}
const canPersistView = computed(() => {
  if (props.identity !== auth.user?.id) return false
  const selected = props.schema.views.find((item) => item.id === props.viewId)
  return (
    selected?.scope === 'personal' ||
    (selected?.scope === 'shared' &&
      (props.schema.myLevel === 'owner' ||
        (props.schema.myLevel === 'write' && !selected.locked)))
  )
})
const columnState = useViewColumns({
  schema: () => props.schema,
  viewId: () => props.viewId,
  identity: () => (props.identity === auth.user?.id ? props.identity : ''),
  canWrite: () => canPersistView.value,
  failure: (cause) => {
    if (cause instanceof RequestError && [401, 403, 404].includes(cause.status))
      handleAccessFailure(cause)
  },
  updated: (view) => {
    const fingerprint = JSON.stringify(view.config.highlight ?? [])
    const highlightsChanged = fingerprint !== savedHighlightFingerprint
    savedHighlightFingerprint = fingerprint
    emit('viewUpdated', view)
    if (highlightsChanged) refreshHighlightedRows()
  },
  recorded: (before, after) =>
    undo.record([viewUndoStep(props.schema.table.id, before, after)]),
  message: (cause) =>
    t('grid.columnSaveFailed', { message: errorMessage(cause) }),
})
const rowSize = computed(() => {
  const sizes = { short: 32, medium: 48, tall: 64, extra: 96 }
  return sizes[columnState.config.value.rowHeight ?? 'short']
})
const rowHeightOptions = computed(() => [
  { value: 'short' as const, label: t('grid.rowHeights.short') },
  { value: 'medium' as const, label: t('grid.rowHeights.medium') },
  { value: 'tall' as const, label: t('grid.rowHeights.tall') },
  { value: 'extra' as const, label: t('grid.rowHeights.extra') },
])
watch(
  () => [columnState.config.value, columnState.localQuery.value] as const,
  ([config, hasLocalChanges]) =>
    emit('viewConfigChanged', config, hasLocalChanges),
  { deep: true, immediate: true },
)
const highlightDrafts = shallowRef<HighlightDraft[]>(
  (columnState.config.value.highlight ?? []).map((rule) => ({
    ...rule,
    target:
      rule.target.type === 'cells'
        ? { type: 'cells', fields: [...rule.target.fields] }
        : { type: 'row' },
  })),
)
const highlightableFields = computed(() =>
  [...props.schema.fields]
    .filter(
      (field) =>
        field.filterable !== false && (field.filterOps?.length ?? 0) > 0,
    )
    .sort((left, right) => left.position - right.position),
)
const highlightTargetFields = computed(() =>
  [...props.schema.fields].sort(
    (left, right) => left.position - right.position,
  ),
)
const highlightListOperators = new Set([
  'is_any_of',
  'is_none_of',
  'has_any_of',
  'has_all_of',
  'has_none_of',
  'is_exactly',
])

function hasIncompleteHighlightValues(node: FilterNode | null): boolean {
  if (!node) return false
  if ('items' in node) return node.items.some(hasIncompleteHighlightValues)
  const value = node.value
  if (node.cmp === 'path_equals')
    return (
      typeof value !== 'object' ||
      value === null ||
      Array.isArray(value) ||
      !Array.isArray(value.path) ||
      value.path.length === 0
    )
  if (node.cmp === 'between') return !Array.isArray(value) || value.length !== 2
  if (!highlightListOperators.has(node.cmp)) return false
  if (
    node.cmp === 'has_any_of' &&
    typeof value === 'object' &&
    value !== null &&
    !Array.isArray(value) &&
    value.rel === 'me'
  )
    return false
  return !Array.isArray(value) || value.length === 0
}

const highlightIncomplete = computed(() =>
  highlightDrafts.value.some(
    (rule) =>
      !isCompleteFilter(rule.when) ||
      hasIncompleteHighlightValues(rule.when) ||
      (rule.target.type === 'cells' && rule.target.fields.length === 0),
  ),
)
watch(
  () =>
    [
      columnState.config.value.highlight,
      props.viewId,
      props.identity,
      columnState.canPersist.value,
    ] as const,
  (
    [rules, viewId, identity, canPersist],
    [, previousViewId, previousIdentity, previousCanPersist],
  ) => {
    if (
      highlightIncomplete.value &&
      viewId === previousViewId &&
      identity === previousIdentity &&
      canPersist === previousCanPersist
    )
      return
    highlightDrafts.value = cloneHighlightDrafts(rules ?? [])
  },
)
let draggedHighlightIndex: number | null = null

function cloneHighlightDrafts(rules = highlightDrafts.value): HighlightDraft[] {
  return rules.map((rule) => ({
    ...rule,
    target:
      rule.target.type === 'cells'
        ? { type: 'cells', fields: [...rule.target.fields] }
        : { type: 'row' },
  }))
}

function selectedFooterAggregate(fieldId: string) {
  return (
    columnState.config.value.aggregates?.find(
      (item) => item.fieldId === fieldId,
    )?.aggregate ?? 'none'
  )
}

function changeFooterAggregate(fieldId: string, value: string) {
  const field = footerAggregateFields.value.find(
    (item) => item.field.id === fieldId,
  )
  if (
    !field ||
    (value !== 'none' &&
      !field.aggregates.some((aggregate) => aggregate === value))
  )
    return
  const aggregates: ViewAggregate[] = (
    columnState.config.value.aggregates ?? []
  ).filter((item) => item.fieldId !== fieldId)
  if (value !== 'none') aggregates.push({ fieldId, aggregate: value })
  columnState.changeAggregates(aggregates)
}

function serializeHighlightFilter(node: FilterNode): FilterNode {
  if ('items' in node)
    return { ...node, items: node.items.map(serializeHighlightFilter) }
  return node.value === undefined && isCompleteFilter(node)
    ? { ...node, value: null }
    : node
}

function persistHighlightDrafts() {
  if (highlightIncomplete.value) return
  const rules: HighlightRule[] = []
  for (const rule of cloneHighlightDrafts()) {
    if (rule.when === null) return
    if (rule.target.type === 'cells' && rule.target.fields.length === 0) return
    rules.push({
      ...rule,
      when: serializeHighlightFilter(rule.when),
      target:
        rule.target.type === 'cells'
          ? { type: 'cells', fields: [...rule.target.fields] }
          : { type: 'row' },
    })
  }
  columnState.changeHighlights(rules)
}

function updateHighlightWhen(id: string, when: FilterNode | null) {
  highlightDrafts.value = highlightDrafts.value.map((rule) =>
    rule.id === id ? { ...rule, when } : rule,
  )
  persistHighlightDrafts()
}

function updateHighlightTarget(id: string, type: string) {
  highlightDrafts.value = highlightDrafts.value.map((rule) => {
    if (rule.id !== id) return rule
    if (type === 'cells') {
      const current = rule.target.type === 'cells' ? rule.target.fields : []
      const fields = current.length
        ? [...current]
        : highlightTargetFields.value[0]
          ? [highlightTargetFields.value[0].id]
          : []
      return {
        ...rule,
        target: { type: 'cells', fields },
      }
    }
    return { ...rule, target: { type: 'row' } }
  })
  persistHighlightDrafts()
}

function updateHighlightFields(id: string, event: Event) {
  const fields = Array.from(
    (event.target as HTMLSelectElement).selectedOptions,
    (option) => option.value,
  )
  highlightDrafts.value = highlightDrafts.value.map((rule) =>
    rule.id === id && rule.target.type === 'cells'
      ? { ...rule, target: { type: 'cells', fields } }
      : rule,
  )
  persistHighlightDrafts()
}

function updateHighlightColor(id: string, color: HighlightColor) {
  highlightDrafts.value = highlightDrafts.value.map((rule) =>
    rule.id === id ? { ...rule, color } : rule,
  )
  persistHighlightDrafts()
}

function addHighlightRule() {
  if (highlightDrafts.value.length >= 20 || !highlightableFields.value.length)
    return
  highlightDrafts.value = [
    ...highlightDrafts.value,
    {
      id: `hl-${crypto.randomUUID()}`,
      when: null,
      target: { type: 'row' },
      color: 'red-2',
    },
  ]
}

function startHighlightDrag(index: number, event: DragEvent) {
  draggedHighlightIndex = index
  event.dataTransfer?.setData('text/plain', String(index))
  if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move'
}

function highlightColorLabel(color: HighlightColor) {
  switch (color) {
    case 'red-2':
      return t('grid.highlightColors.red')
    case 'orange-2':
      return t('grid.highlightColors.orange')
    case 'amber-3':
      return t('grid.highlightColors.amber')
    case 'green-2':
      return t('grid.highlightColors.green')
    case 'blue-2':
      return t('grid.highlightColors.blue')
    case 'purple-2':
      return t('grid.highlightColors.purple')
  }
}

function removeHighlightRule(index: number) {
  highlightDrafts.value = highlightDrafts.value.filter(
    (_, current) => current !== index,
  )
  persistHighlightDrafts()
}

function moveHighlightRule(from: number, to: number) {
  if (from < 0 || from >= highlightDrafts.value.length) return
  if (to < 0 || to >= highlightDrafts.value.length || from === to) return
  const next = [...highlightDrafts.value]
  const [rule] = next.splice(from, 1)
  if (!rule) return
  next.splice(to, 0, rule)
  highlightDrafts.value = next
  persistHighlightDrafts()
}

function dropHighlightRule(target: number) {
  if (draggedHighlightIndex === null) return
  moveHighlightRule(draggedHighlightIndex, target)
  draggedHighlightIndex = null
}
const visibleFieldIds = computed(() =>
  columnState.config.value.fields
    .filter((field) => field.visible)
    .map((field) => field.id),
)
const footerAggregateFields = computed(() =>
  visibleFieldIds.value.flatMap((id) => {
    const item = fieldById.value.get(id)
    if (!item) return []
    const aggregates = item.bound
      .aggregates()
      .filter((aggregate) => aggregate !== 'none')
    return aggregates.length ? [{ ...item, aggregates }] : []
  }),
)
function displayedFooterAggregates(): DisplayedFooterAggregate[] {
  const result: DisplayedFooterAggregate[] = []
  for (const aggregate of footerAggregates.value) {
    const item = fieldById.value.get(aggregate.fieldId)
    if (!item || !visibleFieldIds.value.includes(aggregate.fieldId)) continue
    result.push({
      key: `${aggregate.fieldId}:${aggregate.aggregate}`,
      field: item.field.name,
      aggregate: groupAggregateLabel(aggregate.aggregate),
      value: formatFooterAggregateValue(
        aggregate.value,
        aggregate.aggregate,
        item.bound,
      ),
    })
  }
  return result
}
const columnFields = computed(() =>
  columnState.config.value.fields.flatMap((column) => {
    const field = props.schema.fields.find((field) => field.id === column.id)
    return field ? [{ ...column, name: field.name }] : []
  }),
)
const sortableFields = computed(() =>
  [...props.schema.fields]
    .filter((field) => field.uiType !== 'link')
    .sort((left, right) => left.position - right.position),
)
const sortCriteria = ref<RowSort[]>(
  (columnState.config.value.sort ?? []).map(({ field, dir }) => ({
    fieldId: field,
    direction: dir,
  })),
)
const availableSortFields = computed(() =>
  sortableFields.value.filter(
    (field) => !sortCriteria.value.some((sort) => sort.fieldId === field.id),
  ),
)
const sortFieldToAdd = ref('')
const unsupportedField = computed(() =>
  props.schema.fields.some(
    (field) => field.uiType !== 'link' && !fieldById.value.has(field.id),
  ),
)

function fetchPage(
  offset: number,
  limit: number,
  includeCount: boolean,
  signal: AbortSignal,
  groupPath?: RowValue[],
): Promise<RowQueryResult> {
  if (props.identity !== auth.user?.id)
    throw new Error('Grid identity changed before query')
  const body: RowQueryRequest = {
    offset,
    limit,
    includeCount,
    sorts: sortCriteria.value,
    ...(props.filter ? { filter: props.filter } : {}),
    ...(searchTerm.value ? { search: searchTerm.value } : {}),
    ...(groupPath?.length ? { groupPath } : {}),
    ...(columnState.localQuery.value
      ? { fieldIds: visibleFieldIds.value }
      : {}),
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
    return result
  })
}

const remoteRows = new RemoteRowsWindow(
  { fetch: fetchPage },
  () => {
    if (!ready.value || disposed.value) return
    if (rows.snapshot().cachedRows > 0) loading.value = false
    syncSource()
    if (!loading.value && !initializing && remoteReloadPending)
      queueMicrotask(flushDeferredRealtime)
  },
  (cause) => {
    if (cause instanceof RequestError && [401, 403, 404].includes(cause.status))
      handleAccessFailure(cause)
    error.value = cause
    loading.value = false
  },
)

const groupBy = computed(() => columnState.config.value.groupBy ?? [])
const groupedMode = computed(() => groupBy.value.length > 0)
const groupableFields = computed(() =>
  [...props.schema.fields]
    .filter((field) => field.uiType !== 'link')
    .sort((left, right) => left.position - right.position),
)
const availableGroupFields = computed(() =>
  groupableFields.value.filter((field) => !groupBy.value.includes(field.id)),
)
const groupFieldToAdd = ref('')
const groupEditDisabled = computed(
  () => !columnState.canPersist.value || columnState.groupPending.value,
)
const groupAddDisabled = computed(
  () => groupEditDisabled.value || groupBy.value.length >= 3,
)
function addGroupField() {
  if (
    groupAddDisabled.value ||
    !availableGroupFields.value.some(
      (field) => field.id === groupFieldToAdd.value,
    )
  )
    return
  columnState.change({
    kind: 'group',
    ids: [...groupBy.value, groupFieldToAdd.value],
  })
  groupFieldToAdd.value = ''
}
function removeGroupField(id: string) {
  if (groupEditDisabled.value) return
  columnState.change({
    kind: 'group',
    ids: groupBy.value.filter((fieldId) => fieldId !== id),
  })
}
function createGroupedRows() {
  return new GroupedRowsWindow(
    {
      groups: async (path, signal) => {
        const result = await request<RowGroupResult>(
          `/views/${props.viewId}/groups`,
          'POST',
          {
            path,
            ...(props.filter ? { filter: props.filter } : {}),
            ...(searchTerm.value ? { search: searchTerm.value } : {}),
            ...(columnState.localQuery.value
              ? { fieldIds: visibleFieldIds.value }
              : {}),
          },
          AbortSignal.any([signal, AbortSignal.timeout(10_000)]),
        )
        if (result.schemaVersion !== props.schema.schemaVersion) {
          emit('schemaChanged')
          throw new RequestError('ERR_SCHEMA_CHANGED', 409)
        }
        return result.groups
      },
      rows: (path, offset, limit, includeCount, signal) =>
        fetchPage(offset, limit, includeCount, signal, path),
    },
    Math.max(1, Math.min(3, groupBy.value.length)),
    `pandatable:groups:${props.identity}:${props.viewId}:${groupBy.value.join(',')}`,
    () => {
      if (!ready.value || disposed.value) return
      if (groupedRows.snapshot().cachedRows > 0) loading.value = false
      syncSource()
      if (!loading.value && !initializing && remoteReloadPending)
        queueMicrotask(flushDeferredRealtime)
    },
    (cause) => {
      if (
        cause instanceof RequestError &&
        [401, 403, 404].includes(cause.status)
      )
        handleAccessFailure(cause)
      error.value = cause
      loading.value = false
    },
  )
}
let groupedRows = createGroupedRows()

async function loadFooterAggregates() {
  footerController?.abort()
  footerController = undefined
  const generation = ++footerGeneration
  const aggregates = columnState.config.value.aggregates ?? []
  if (!aggregates.length) {
    footerAggregates.value = []
    footerAggregateError.value = ''
    footerAggregateLoading.value = false
    return
  }
  const controller = new AbortController()
  footerController = controller
  footerAggregateLoading.value = true
  footerAggregateError.value = ''
  footerAggregates.value = []
  try {
    const result = await request<RowAggregateResult>(
      `/views/${props.viewId}/aggregates`,
      'POST',
      {
        aggregates,
        fieldIds: visibleFieldIds.value,
        ...(props.filter ? { filter: props.filter } : {}),
        ...(searchTerm.value ? { search: searchTerm.value } : {}),
      },
      AbortSignal.any([controller.signal, AbortSignal.timeout(10_000)]),
    )
    if (disposed.value || generation !== footerGeneration) return
    if (result.schemaVersion !== props.schema.schemaVersion) {
      emit('schemaChanged')
      throw new RequestError('ERR_SCHEMA_CHANGED', 409)
    }
    footerAggregates.value = result.aggregates
  } catch (cause) {
    if (
      disposed.value ||
      generation !== footerGeneration ||
      controller.signal.aborted
    )
      return
    if (cause instanceof RequestError && [401, 403, 404].includes(cause.status))
      handleAccessFailure(cause)
    footerAggregateError.value = errorMessage(cause)
  } finally {
    if (generation === footerGeneration) {
      footerAggregateLoading.value = false
      footerController = undefined
    }
  }
}

const rows = {
  get total() {
    return groupedMode.value ? groupedRows.total : remoteRows.total
  },
  snapshot: () =>
    groupedMode.value ? groupedRows.snapshot() : remoteRows.snapshot(),
  initialize: (anchor = 0) =>
    groupedMode.value
      ? groupedRows.initialize()
      : remoteRows.initialize(anchor),
  load: (index: number) =>
    groupedMode.value ? groupedRows.load(index) : remoteRows.load(index),
  ensureLoaded: (index: number) =>
    groupedMode.value
      ? groupedRows.ensureLoaded(index)
      : remoteRows.ensureLoaded(index),
  rowAt: (index: number) =>
    groupedMode.value ? groupedRows.rowAt(index) : remoteRows.rowAt(index),
  indexOfRow: (id: string) =>
    groupedMode.value ? groupedRows.indexOfRow(id) : remoteRows.indexOfRow(id),
  updateRow: (index: number, row: DataRow) =>
    groupedMode.value
      ? groupedRows.updateRow(index, row)
      : remoteRows.updateRow(index, row),
  invalidateOffscreen: (ids: readonly string[]) => {
    if (!groupedMode.value) remoteRows.invalidateOffscreen(ids)
  },
  dispose: () => {
    groupedRows.dispose()
    remoteRows.dispose()
  },
}

function requestRealtimeReload() {
  remoteReloadPending = true
  flushDeferredRealtime()
}

function flushDeferredRealtime() {
  if (
    disposed.value ||
    !ready.value ||
    activeEdit.value ||
    saving.value ||
    loading.value ||
    initializing
  )
    return
  if (remoteReloadPending) {
    remoteReloadPending = false
    deferredRows.clear()
    deferredRemoteChanges = false
    void initialize(
      Math.max(0, rows.snapshot().activePage) * remotePageSize,
      true,
    )
    return
  }
  for (const row of deferredRows.values()) {
    const index = rows.indexOfRow(row.id)
    if (index >= 0) rows.updateRow(index, row)
  }
  deferredRows.clear()
  deferredRemoteChanges = false
}

const rowSync = new RealtimeRowSync({
  window: () => (ready.value && !disposed.value ? rows : undefined),
  epoch: () => realtimeEpoch,
  fetch: async (ids, signal) => {
    const result = await request<RowQueryResult>(
      `/views/${props.viewId}/query`,
      'POST',
      {
        ids,
        limit: ids.length,
        includeCount: false,
        fieldIds: visibleFieldIds.value,
        sorts: sortCriteria.value,
        ...(props.filter ? { filter: props.filter } : {}),
        ...(searchTerm.value ? { search: searchTerm.value } : {}),
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
    if (activeEdit.value || saving.value) {
      deferredRows.set(row.id, row)
      deferredRemoteChanges = true
      return
    }
    const index = rows.indexOfRow(row.id)
    if (index >= 0) rows.updateRow(index, row)
  },
  reload: requestRealtimeReload,
  needsReload: () =>
    groupedMode.value ||
    sortCriteria.value.length > 0 ||
    !!props.filter ||
    !!searchTerm.value,
  failed: (cause) => {
    if (cause instanceof RequestError && [401, 403, 404].includes(cause.status))
      handleAccessFailure(cause)
    error.value = cause
  },
})

function handleRealtime(event: RealtimeEvent) {
  if (event.t === 'row.updated' && event.ids) {
    if (!ready.value) {
      requestRealtimeReload()
      return
    }
    const edit = activeEdit.value
    if (edit && event.ids.includes(edit.rowId)) {
      const fields = event.changes?.[edit.rowId] ?? event.fields
      if (!fields || fields.includes(edit.fieldId))
        activeEdit.value = {
          ...edit,
          changedBy: event.byName || event.by || t('realtime.unknownEditor'),
        }
    }
    if (initializing || loading.value) {
      requestRealtimeReload()
      return
    }
    rowSync.update(event.ids)
    void loadFooterAggregates()
  } else if (
    ['comment.created', 'comment.updated', 'comment.deleted'].includes(
      event.t,
    ) &&
    event.ids
  ) {
    if (!ready.value) requestRealtimeReload()
    else if (initializing || loading.value) requestRealtimeReload()
    else rowSync.update(event.ids)
  } else if (
    ['row.created', 'row.deleted', 'row.bulk', 'resync'].includes(event.t) ||
    (event.t === 'view.changed' && event.viewId === props.viewId)
  ) {
    requestRealtimeReload()
  }
}

const removeRealtimeListener = realtime.subscribe(
  props.schema.table.id,
  handleRealtime,
)

interface GridModel {
  __rowId?: string
  __rowNumber?: number
  __groupId?: string
  __commentCount?: number
  _hl_row?: HighlightColor
  _hl_cells?: Record<string, HighlightColor>
  [key: string]: RowValue | string | undefined
}

const emptyModel: GridModel = { __rowId: '', __groupId: '' }
const groupNodes = new Map<string, GroupNode>()

function groupPathKey(path: RowValue[]) {
  return JSON.stringify(path)
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

function groupActionLabel(group: GroupNode) {
  const field = props.schema.fields.find(
    (candidate) => candidate.id === groupBy.value[group.level],
  )
  const params = {
    field: field?.name ?? '',
    value: groupKeyLabel(group.key),
  }
  return group.expanded
    ? t('grid.collapseGroup', params)
    : t('grid.expandGroup', params)
}

async function toggleGroup(group: GroupNode) {
  try {
    await groupedRows.toggle(group.path)
  } catch (cause) {
    error.value = cause
  }
}

function modelForGroup(group: GroupNode): GridModel {
  const id = groupPathKey(group.path)
  groupNodes.set(id, group)
  const model: GridModel = { __rowId: '', __groupId: id }
  const firstFieldId = visibleFieldIds.value[0]
  if (firstFieldId) model[firstFieldId] = null
  for (const aggregate of group.aggregates)
    model[aggregate.fieldId] = aggregate.value
  return model
}

function wireValue(value: unknown): WireValue {
  if (isWireValue(value)) return value
  return null
}

function boundFor(id: string) {
  return fieldById.value.get(id)
}

function supportedColumn(id: string) {
  return (
    fieldById.value.has(id) ||
    props.schema.fields.some(
      (field) => field.id === id && field.uiType === 'link',
    )
  )
}

function openRelationPicker(field: TableField, rowId: string) {
  if (!canWrite.value) return
  const index = rows.indexOfRow(rowId)
  const row = index < 0 ? null : rows.rowAt(index)
  if (!row) return
  relationField.value = field
  relationRow.value = row
}

function updateRelationRow(row: DataRow) {
  const index = rows.indexOfRow(row.id)
  if (index >= 0) rows.updateRow(index, row)
  relationRow.value = row
  if (columnState.config.value.highlight?.length) void refreshHighlightedRows()
}

function closeRelationPicker() {
  relationField.value = null
  relationRow.value = null
}

const columns = computed<ColumnRegular[]>(() => {
  const definitions: ColumnRegular[] = [
    {
      prop: '__selected',
      name: '#',
      size: 104,
      pin: 'colPinStart',
      readonly: true,
      cellTemplate: (render, { model }) =>
        model.__rowId
          ? render(
              'span',
              {
                class: 'grid-highlight-cell grid-row-number-cell',
                'data-row-highlight': rowHighlight(model),
              },
              [
                render(
                  'span',
                  { class: 'grid-row-number' },
                  String(model.__rowNumber ?? ''),
                ),
                render('input', {
                  type: 'checkbox',
                  checked: selectedRows.value.has(String(model.__rowId)),
                  'aria-label': t('grid.selectRow', { id: model.__rowId }),
                  onChange: (event: Event) =>
                    selectRow(
                      String(model.__rowId),
                      (event.target as HTMLInputElement).checked,
                    ),
                }),
                render(
                  'button',
                  {
                    type: 'button',
                    class: 'grid-row-expand',
                    'aria-label': t('rowExpand.open', { id: model.__rowId }),
                    onClick: () => emit('expand', String(model.__rowId)),
                  },
                  render(
                    'svg',
                    {
                      width: 18,
                      height: 18,
                      viewBox: '0 0 24 24',
                      fill: 'none',
                      stroke: 'currentColor',
                      'stroke-width': 2,
                      'aria-hidden': 'true',
                    },
                    [
                      render('path', {
                        d: 'M14 4h6v6M20 4l-7 7M10 20H4v-6M4 20l7-7',
                      }),
                    ],
                  ),
                ),
                typeof model.__commentCount === 'number' &&
                model.__commentCount > 0
                  ? render(
                      'span',
                      {
                        class: 'grid-comment-count',
                        'aria-label': t('comments.count', {
                          count: model.__commentCount,
                        }),
                      },
                      String(model.__commentCount),
                    )
                  : null,
              ],
            )
          : null,
    },
  ]
  for (const [index, id] of visibleFieldIds.value.entries()) {
    const relationField = props.schema.fields.find(
      (field) => field.id === id && field.uiType === 'link',
    )
    if (relationField) {
      definitions.push({
        prop: relationField.id,
        name: relationField.name,
        size:
          columnState.config.value.fields.find((column) => column.id === id)
            ?.width ?? 180,
        minSize: minColumnWidth,
        maxSize: maxColumnWidth,
        pin:
          index < columnState.config.value.frozen ? 'colPinStart' : undefined,
        readonly: true,
        cellTemplate: template<CellTemplateProp>(GridRelationCell, {
          props: (context) => ({
            links: context.value,
            editable: canWrite.value,
            rowHighlight: rowHighlight(context.model),
            cellHighlight: cellHighlight(context.model, relationField.id),
            tableId: props.schema.table.id,
            rowId: String(context.model.__rowId ?? ''),
            fieldId: relationField.id,
            identity: props.identity,
            open: () =>
              openRelationPicker(
                relationField,
                String(context.model.__rowId ?? ''),
              ),
          }),
          key: (context) =>
            `${String(context.model.__rowId)}:${relationField.id}`,
        }),
      })
      continue
    }
    const item = boundFor(id)
    if (!item) continue
    const { field, bound } = item
    const fieldCellTemplate = template<CellTemplateProp>(GridCellContent, {
      props: (context) => {
        const groupId = context.model.__groupId
        const group =
          typeof groupId === 'string' ? groupNodes.get(groupId) : undefined
        const aggregate = group?.aggregates.find(
          (candidate) => candidate.fieldId === field.id,
        )
        return {
          field: bound,
          value: wireValue(context.value),
          env: fieldEnvironment.value,
          tableId: props.schema.table.id,
          identity: props.identity,
          rowId: String(context.model.__rowId ?? ''),
          fieldId: field.id,
          rowHighlight: rowHighlight(context.model),
          cellHighlight: cellHighlight(context.model, field.id),
          state: () => {
            const key = `${String(context.model.__rowId)}:${field.id}`
            return {
              draft: cellDrafts.value.get(key),
              message: cellErrors.value.get(key) ?? '',
            }
          },
          savingLabel: t('grid.saving'),
          group: group
            ? {
                header: field.id === visibleFieldIds.value[0],
                path: JSON.stringify(group.path),
                label: groupTitle(group),
                expanded: group.expanded,
                ariaLabel: groupActionLabel(group),
                aggregate: aggregate
                  ? t('grid.groupAggregate', {
                      field: field.name,
                      aggregate: groupAggregateLabel(aggregate.aggregate),
                      value: String(aggregate.value ?? ''),
                    })
                  : undefined,
                toggle: () => void toggleGroup(group),
              }
            : undefined,
        }
      },
      key: (context) =>
        `${String(context.model.__groupId ?? context.model.__rowId)}:${field.id}`,
    })
    definitions.push({
      prop: field.id,
      name: field.name,
      size:
        columnState.config.value.fields.find((column) => column.id === id)
          ?.width ?? 180,
      minSize: minColumnWidth,
      maxSize: maxColumnWidth,
      pin: index < columnState.config.value.frozen ? 'colPinStart' : undefined,
      readonly: !canWrite.value || bound.readOnly,
      editor: bound.readOnly ? undefined : field.id,
      cellTemplate: fieldCellTemplate,
    })
  }
  return definitions
})

const canCreateField = computed(
  () => props.identity === auth.user?.id && props.schema.myLevel === 'owner',
)
const addFieldOffset = computed(() =>
  columns.value.reduce((width, column) => width + (column.size ?? 180), 0),
)

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

function formatFooterAggregateValue(
  value: unknown,
  aggregate: string,
  field: BoundField,
): string {
  if (value === null || value === undefined) return t('grid.aggregateNoValue')
  if (['count', 'count_empty', 'count_filled', 'unique'].includes(aggregate))
    return String(value)
  if (aggregate === 'percent_checked')
    return new Intl.NumberFormat(locale.value, {
      style: 'percent',
      maximumFractionDigits: 2,
    }).format(Number(value))
  if (!isWireValue(value)) return String(value)
  return field.formatDisplay(value, fieldEnvironment.value)
}

const editors = computed(() =>
  Object.fromEntries(
    visibleFieldIds.value.flatMap((id) => {
      const item = boundFor(id)
      if (!item || item.bound.readOnly) return []
      return [[id, gridCellEditor]]
    }),
  ),
)

function modelFor(row: DataRow | null, index: number): GridModel {
  if (!row) return emptyModel
  return {
    __rowId: row.id,
    __rowNumber: index + 1,
    __groupId: '',
    __commentCount: row.commentCount,
    ...(row._hl_row ? { _hl_row: row._hl_row } : {}),
    ...(row._hl_cells ? { _hl_cells: row._hl_cells } : {}),
    ...row.fields,
  }
}

function rowHighlight(model: GridModel): HighlightColor | undefined {
  return model._hl_row
}

function cellHighlight(
  model: GridModel,
  fieldId: string,
): HighlightColor | undefined {
  return model._hl_cells?.[fieldId]
}

function mutate<T>(
  path: string,
  method: 'POST' | 'PATCH' | 'DELETE',
  body: unknown,
  signal?: AbortSignal,
) {
  const controller = new AbortController()
  mutationControllers.add(controller)
  const signals = [controller.signal, AbortSignal.timeout(10_000)]
  if (signal) signals.push(signal)
  return request<T>(path, method, body, AbortSignal.any(signals)).finally(() =>
    mutationControllers.delete(controller),
  )
}

function clipboardColumn(
  index: number,
  columnType: FocusAfterRenderEvent['colType'] = focused?.colType ?? 'rgCol',
) {
  const column = columns.value.filter((candidate) =>
    columnType === 'rgCol' ? !candidate.pin : candidate.pin === columnType,
  )[index]
  const prop = column?.prop
  return typeof prop === 'string' ? fieldById.value.get(prop) : undefined
}

function requestBodyFits(body: unknown) {
  return new TextEncoder().encode(JSON.stringify(body)).byteLength <= 512 * 1024
}

function clipboardFailure(message = t('grid.clipboardUnavailable')) {
  clipboardMessage.value = message
}

function rowIndexForVirtual(index: number) {
  const anchor = focused
  const id = anchor?.model?.__rowId
  if (!Number.isInteger(index)) return -1
  if (typeof id !== 'string') return index
  if (!anchor) return -1
  const anchorIndex = rows.indexOfRow(id)
  return anchorIndex < 0 ? -1 : anchorIndex + index - anchor.rowIndex
}

function rangeFits(range: RangeArea) {
  const startRow = rowIndexForVirtual(range.y)
  const endRow = rowIndexForVirtual(range.y1)
  const cellCount = (range.y1 - range.y + 1) * (range.x1 - range.x + 1)
  return (
    range.x >= 0 &&
    startRow >= 0 &&
    range.x1 >= range.x &&
    range.y1 >= range.y &&
    endRow < rows.total &&
    cellCount <= 5000 &&
    Array.from({ length: range.y1 - range.y + 1 }, (_, index) =>
      rows.rowAt(startRow + index),
    ).every(Boolean)
  )
}

function escapeClipboardHtml(value: string) {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;')
    .replace(/\r\n|\r|\n/g, '<br>')
}

function beforeCopyRegion(event: CustomEvent<RangeArea | null>) {
  copyRange = null
  const range = event.detail
  if (!range) return
  if (!rangeFits(range)) {
    event.preventDefault()
    clipboardFailure(t('grid.copyLimit'))
    return
  }
  clipboardMessage.value = ''
  copyRange = range
  copyColumnType =
    (range as RangeArea & { colType?: FocusAfterRenderEvent['colType'] })
      .colType ??
    focused?.colType ??
    'rgCol'
}

function copy(event: CustomEvent<{ event: DataTransfer; data?: string[][] }>) {
  event.preventDefault()
  const range = copyRange
  if (!range || !rangeFits(range)) {
    clipboardFailure(t('grid.copyLimit'))
    return
  }
  const matrix = Array.from(
    { length: range.y1 - range.y + 1 },
    (_, rowOffset) => {
      const row = rows.rowAt(rowIndexForVirtual(range.y + rowOffset))
      return Array.from(
        { length: range.x1 - range.x + 1 },
        (_, columnOffset) => {
          const item = clipboardColumn(range.x + columnOffset, copyColumnType)
          if (!row || !item) return ''
          return item.bound.formatCopy(
            wireValue(row.fields[item.field.id]),
            fieldEnvironment.value,
          )
        },
      )
    },
  )
  event.detail.event.setData('text/plain', formatTsv(matrix))
  event.detail.event.setData(
    'text/html',
    `<table><tbody>${matrix
      .map(
        (row) =>
          `<tr>${row.map((cell) => `<td>${escapeClipboardHtml(cell)}</td>`).join('')}</tr>`,
      )
      .join('')}</tbody></table>`,
  )
}

async function patchRows(
  patchRows: { id: string; index: number; fields: Record<string, RowValue> }[],
  record = true,
) {
  if (!patchRows.length) return null
  const body = {
    rows: patchRows.map(({ id, fields }) => ({ id, fields })),
  }
  if (
    patchRows.length > 1000 ||
    patchRows.reduce(
      (count, row) => count + Object.keys(row.fields).length,
      0,
    ) > 5000 ||
    !requestBodyFits(body)
  ) {
    clipboardFailure(t('grid.pasteLimit'))
    return false as const
  }
  try {
    const before: RowValues[] = patchRows.map((patch) => {
      const current = rows.rowAt(patch.index)
      if (!current || current.id !== patch.id)
        throw new Error(t('grid.saveFailed'))
      const fields: Record<string, RowValue> = {}
      for (const id of Object.keys(patch.fields)) {
        const value = current.fields[id]
        if (value === undefined) throw new Error(t('grid.saveFailed'))
        fields[id] = value
      }
      return { id: patch.id, fields }
    })
    const result = await mutate<RowMutation>(
      `/tables/${props.schema.table.id}/rows`,
      'PATCH',
      body,
    )
    if (disposed.value || props.identity !== auth.user?.id) return false
    if (
      result.rows.length !== patchRows.length ||
      result.rows.some((row, index) => row.id !== patchRows[index]?.id)
    )
      throw new Error(t('grid.saveFailed'))
    const step = patchUndoStep(
      props.schema.table.id,
      result.previous ?? before,
      result.rows,
    )
    if (record) undo.record([step])
    result.rows.forEach((row, index) => {
      const patch = patchRows[index]!
      rows.updateRow(patch.index, row)
      for (const fieldId of Object.keys(patch.fields))
        cellErrors.value.delete(`${row.id}:${fieldId}`)
    })
    if (columnState.config.value.highlight?.length)
      await refreshHighlightedRows()
    return step
  } catch (cause) {
    if (cause instanceof RequestError && [401, 403, 404].includes(cause.status))
      handleAccessFailure(cause)
    error.value = cause
    return false as const
  }
}

function planPaste(
  matrix: string[][],
  startRow: number,
  startColumn: number,
  columnType: FocusAfterRenderEvent['colType'],
) {
  const columnCount = columns.value.filter((candidate) =>
    columnType === 'rgCol' ? !candidate.pin : candidate.pin === columnType,
  ).length
  const existing = new Map<
    number,
    { id: string; index: number; fields: Record<string, RowValue> }
  >()
  const newFields = new Map<number, Record<string, RowValue>>()
  const issues: string[] = []
  let lastWritableRow = -1

  for (let rowOffset = 0; rowOffset < matrix.length; rowOffset++) {
    const rowIndex = startRow + rowOffset
    for (
      let columnOffset = 0;
      columnOffset < matrix[rowOffset]!.length;
      columnOffset++
    ) {
      const columnIndex = startColumn + columnOffset
      if (columnIndex >= columnCount) {
        clipboardFailure(t('grid.pasteLimit'))
        return null
      }
      const item = clipboardColumn(columnIndex, columnType)
      if (!item || item.bound.readOnly) continue
      const result = item.bound.parsePaste(
        matrix[rowOffset]![columnOffset]!,
        fieldEnvironment.value,
      )
      const target = rows.rowAt(rowIndex)
      if (rowIndex < rows.total && !target) {
        clipboardFailure()
        return null
      }
      if (!result.ok) {
        const message = t('grid.pasteInvalidCell', {
          row: rowIndex + 1,
          field: item.field.name,
          reason: fieldErrorMessage(result.reason),
        })
        issues.push(message)
        if (target)
          cellErrors.value.set(`${target.id}:${item.field.id}`, message)
        continue
      }

      lastWritableRow = Math.max(lastWritableRow, rowIndex)
      if (rowIndex < rows.total) {
        const patch = existing.get(rowIndex) ?? {
          id: target!.id,
          index: rowIndex,
          fields: {},
        }
        patch.fields[item.field.id] = result.value
        existing.set(rowIndex, patch)
      } else {
        const fields = newFields.get(rowIndex) ?? {}
        fields[item.field.id] = result.value
        newFields.set(rowIndex, fields)
      }
    }
  }

  const addedRows = Math.max(0, lastWritableRow + 1 - rows.total)
  if (addedRows > 1000) {
    clipboardFailure(t('grid.pasteLimit'))
    return null
  }
  const insertRows = Array.from({ length: addedRows }, (_, index) => ({
    fields: newFields.get(rows.total + index) ?? {},
  }))
  const patchRows = [...existing.values()].sort(
    (left, right) => left.index - right.index,
  )
  const insertBody = { rows: insertRows }
  const patchBody = {
    rows: patchRows.map(({ id, fields }) => ({ id, fields })),
  }
  if (
    patchRows.length > 1000 ||
    insertRows.reduce(
      (count, row) => count + Object.keys(row.fields).length,
      0,
    ) > 5000 ||
    !requestBodyFits(insertBody) ||
    !requestBodyFits(patchBody)
  ) {
    clipboardFailure(t('grid.pasteLimit'))
    return null
  }
  clipboardErrors.value = issues
  return { startRow, addedRows, patchRows, insertRows } satisfies PastePlan
}

async function savePaste(plan: PastePlan) {
  if (!canWrite.value || props.identity !== auth.user?.id || saving.value)
    return
  saving.value = true
  error.value = null
  try {
    const patch = await patchRows(plan.patchRows, false)
    if (patch === false) return
    const steps: (UndoStep | null)[] = [patch]
    if (plan.addedRows > 0) {
      if (disposed.value || props.identity !== auth.user?.id) {
        undo.record(steps)
        return
      }
      try {
        const inserted = await mutate<RowMutation>(
          `/tables/${props.schema.table.id}/rows`,
          'POST',
          { rows: plan.insertRows },
        )
        if (inserted.rows.length !== plan.insertRows.length)
          throw new Error(t('grid.saveFailed'))
        steps.push({
          kind: 'delete',
          tableId: props.schema.table.id,
          ids: inserted.rows.map((row) => row.id),
        })
      } catch (cause) {
        if (
          cause instanceof RequestError &&
          [401, 403, 404].includes(cause.status)
        )
          handleAccessFailure(cause)
        error.value = cause
        undo.record(steps)
        if (plan.patchRows.length > 0) clipboardFailure(t('grid.partialPaste'))
        return
      }
      undo.record(steps)
      if (disposed.value || props.identity !== auth.user?.id) return
      await initialize(plan.startRow, true)
    } else undo.record(steps)
  } finally {
    if (!disposed.value) saving.value = false
  }
}

function paste(
  event: CustomEvent<{
    parsed: string[][]
    dataText: string
    raw: string
    event: ClipboardEvent
  }>,
) {
  event.preventDefault()
  clipboardMessage.value = ''
  clipboardErrors.value = []
  if (!canWrite.value || !focused) {
    clipboardFailure()
    return
  }
  let matrix: string[][]
  try {
    matrix = parseTsv(event.detail.dataText || event.detail.raw)
  } catch (cause) {
    if (!(cause instanceof TsvError)) throw cause
    clipboardFailure(t('grid.pasteInvalidTsv'))
    return
  }
  const startRow = rowIndexForVirtual(focused.rowIndex)
  if (startRow < 0) {
    clipboardFailure()
    return
  }
  const rowEnd = startRow + matrix.length
  if (rowEnd > 1_000_001) {
    clipboardFailure(t('grid.pasteLimit'))
    return
  }
  const plan = planPaste(matrix, startRow, focused.colIndex, focused.colType)
  if (!plan) return
  if (plan.addedRows > 0) {
    pendingPaste.value = plan
    addRowsDialog.value?.showModal()
  } else {
    void savePaste(plan)
  }
}

async function confirmPasteRows() {
  const plan = pendingPaste.value
  pendingPaste.value = null
  addRowsDialog.value?.close()
  if (plan) await savePaste(plan)
}

function cancelPasteRows() {
  pendingPaste.value = null
  addRowsDialog.value?.close()
}

function beforeCut(event: CustomEvent<{ event: ClipboardEvent }>) {
  cutPending = canWrite.value
  if (!cutPending) event.preventDefault()
}

async function clearCutRange(event: CustomEvent<DataTransfer>) {
  if (!cutPending) return
  event.preventDefault()
  cutPending = false
  const range = copyRange
  copyRange = null
  await clearCells(range, copyColumnType)
}

async function clearCells(
  range: RangeArea | null,
  columnType: FocusAfterRenderEvent['colType'],
) {
  if (!canWrite.value || saving.value) return
  if (!range || !rangeFits(range)) {
    clipboardFailure(t('grid.copyLimit'))
    return
  }
  const patchMap = new Map<
    number,
    { id: string; index: number; fields: Record<string, RowValue> }
  >()
  for (let x = range.x; x <= range.x1; x++) {
    const item = clipboardColumn(x, columnType)
    if (!item || item.bound.readOnly) continue
    const empty = item.bound.parsePaste('', fieldEnvironment.value)
    if (!empty.ok) {
      clipboardFailure(fieldErrorMessage(empty.reason))
      return
    }
    for (let y = range.y; y <= range.y1; y++) {
      const rowIndex = rowIndexForVirtual(y)
      const target = rows.rowAt(rowIndex)
      if (!target) {
        clipboardFailure()
        return
      }
      const patch = patchMap.get(rowIndex) ?? {
        id: target.id,
        index: rowIndex,
        fields: {},
      }
      patch.fields[item.field.id] = empty.value
      patchMap.set(rowIndex, patch)
    }
  }
  saving.value = true
  clipboardMessage.value = ''
  try {
    await patchRows([...patchMap.values()])
  } finally {
    if (!disposed.value) saving.value = false
  }
}

async function fillDown(
  oldRange: RangeArea,
  newRange: RangeArea,
  keyboard = false,
  columnType: FocusAfterRenderEvent['colType'] = focused?.colType ?? 'rgCol',
) {
  clipboardErrors.value = []
  const oldStart = rowIndexForVirtual(oldRange.y)
  const newStart = rowIndexForVirtual(newRange.y)
  const newEnd = rowIndexForVirtual(newRange.y1)
  if (
    !canWrite.value ||
    oldStart < 0 ||
    newStart !== oldStart ||
    newEnd < 0 ||
    oldRange.x !== newRange.x ||
    oldRange.x1 !== newRange.x1 ||
    newRange.y !== oldRange.y ||
    (keyboard ? newRange.y1 <= oldRange.y : newRange.y1 <= oldRange.y1) ||
    newEnd >= rows.total
  ) {
    clipboardFailure(t('grid.fillDownLimit'))
    return
  }
  const rowCount = newEnd - oldStart
  const columnCount = newRange.x1 - newRange.x + 1
  if (rowCount < 1 || rowCount > 1000 || rowCount * columnCount > 5000) {
    clipboardFailure(t('grid.fillDownLimit'))
    return
  }
  const first = rows.rowAt(oldStart)
  if (!first) {
    clipboardFailure()
    return
  }
  const fields: Record<string, RowValue> = {}
  for (let x = oldRange.x; x <= oldRange.x1; x++) {
    const item = clipboardColumn(x, columnType)
    if (!item || item.bound.readOnly) continue
    fields[item.field.id] = wireValue(first.fields[item.field.id])
  }
  if (Object.keys(fields).length === 0) return
  const patches: {
    id: string
    index: number
    fields: Record<string, RowValue>
  }[] = []
  for (let y = oldStart + 1; y <= newEnd; y++) {
    const row = rows.rowAt(y)
    if (!row) {
      clipboardFailure()
      return
    }
    patches.push({ id: row.id, index: y, fields: { ...fields } })
  }
  saving.value = true
  clipboardMessage.value = ''
  try {
    await patchRows(patches)
  } finally {
    if (!disposed.value) saving.value = false
  }
}

function rangeEdit(event: CustomEvent<BeforeRangeSaveDataDetails>) {
  event.preventDefault()
  const oldRange = event.detail.oldRange
  const newRange = event.detail.newRange
  if (!oldRange || !newRange) {
    clipboardFailure(t('grid.fillDownLimit'))
    return
  }
  void fillDown(oldRange, newRange)
}

async function keydown(event: KeyboardEvent) {
  if (
    event.defaultPrevented ||
    event.isComposing ||
    (event.target instanceof Element &&
      event.target.closest(
        'input, textarea, select, button, summary, [contenteditable="true"]',
      ))
  )
    return
  const shortcut = event.ctrlKey || event.metaKey
  const stop = () => {
    event.preventDefault()
    event.stopPropagation()
  }
  if (!shortcut && ['Delete', 'Backspace'].includes(event.key)) {
    stop()
    if (focusedRowIndex() < 0) return
    const range = await host.value?.$el.getSelectedRange()
    await clearCells(
      range ?? null,
      range?.colType ?? focused?.colType ?? 'rgCol',
    )
    return
  }
  if (!shortcut && ['F2', 'Enter'].includes(event.key)) {
    stop()
    const rowIndex = focusedRowIndex()
    const column =
      focused?.column?.prop ??
      clipboardColumn(focused?.colIndex ?? -1)?.field.id
    const item = typeof column === 'string' ? boundFor(column) : undefined
    if (canWrite.value && item && !item.bound.readOnly && rowIndex >= 0)
      await host.value?.$el.setCellEdit(rowIndex, item.field.id)
    return
  }
  if (['Home', 'End', 'PageUp', 'PageDown'].includes(event.key)) {
    stop()
    let rowIndex = focusedRowIndex()
    const prop =
      focused?.column?.prop ??
      clipboardColumn(focused?.colIndex ?? -1)?.field.id
    let fieldIndex = visibleFieldIds.value.indexOf(String(prop))
    if (rowIndex < 0 || !visibleFieldIds.value.length) return
    if (event.key === 'Home') {
      fieldIndex = 0
      if (shortcut) rowIndex = 0
    } else if (event.key === 'End') {
      fieldIndex = visibleFieldIds.value.length - 1
      if (shortcut) rowIndex = rows.total - 1
    } else {
      const page = Math.max(
        1,
        Math.floor(
          ((host.value?.$el.clientHeight ?? 352) - 32) / rowSize.value,
        ),
      )
      rowIndex = Math.max(
        0,
        Math.min(
          rows.total - 1,
          rowIndex + (event.key === 'PageDown' ? page : -page),
        ),
      )
    }
    await focusGlobalCell(rowIndex, Math.max(0, fieldIndex))
    return
  }
  if (event.key === ' ' && !event.ctrlKey && !event.metaKey && !event.altKey) {
    event.preventDefault()
    event.stopPropagation()
    const index = focusedRowIndex()
    const row = rows.rowAt(index)
    const column =
      focused?.column?.prop ??
      clipboardColumn(focused?.colIndex ?? -1)?.field.id
    const item = typeof column === 'string' ? boundFor(column) : undefined
    if (row && props.identity === auth.user?.id) {
      if (item?.field.uiType === 'checkbox') {
        if (canWrite.value) {
          const value = item.bound.normalize(row.fields[item.field.id] !== true)
          if (!value.ok) {
            error.value = fieldErrorMessage(value.reason)
            return
          }
          saving.value = true
          try {
            await patchRows([
              { id: row.id, index, fields: { [item.field.id]: value.value } },
            ])
          } finally {
            if (!disposed.value) saving.value = false
          }
        }
      } else emit('expand', row.id)
    }
    return
  }
  if (
    !(event.ctrlKey || event.metaKey) ||
    event.key.toLowerCase() !== 'd' ||
    (event.target instanceof Element &&
      event.target.closest(
        'input, textarea, select, button, [contenteditable="true"]',
      ))
  )
    return
  event.preventDefault()
  event.stopPropagation()
  const range = await host.value?.$el.getSelectedRange()
  if (range)
    await fillDown(
      range,
      range,
      true,
      range.colType ?? focused?.colType ?? 'rgCol',
    )
}

let keyboardGeneration = 0
function focusedRowIndex() {
  const id = focused?.model?.__rowId
  return typeof id === 'string' ? rows.indexOfRow(id) : -1
}

async function focusGlobalCell(rowIndex: number, fieldIndex: number) {
  const own = ++keyboardGeneration
  const identity = props.identity
  const id = visibleFieldIds.value[fieldIndex]
  if (!id || rowIndex < 0 || rowIndex >= rows.total) return
  if (!(await rows.ensureLoaded(rowIndex))) return
  if (
    own !== keyboardGeneration ||
    disposed.value ||
    identity !== auth.user?.id ||
    !rows.rowAt(rowIndex)
  )
    return
  const column = columns.value.find((column) => column.prop === id)
  if (!column) return
  const type = column.pin ?? 'rgCol'
  const x = columns.value
    .filter((candidate) => (candidate.pin ?? 'rgCol') === type)
    .findIndex((candidate) => candidate.prop === id)
  await nextTick()
  await host.value?.$el.scrollToRow(rowIndex)
  if (type === 'rgCol') await host.value?.$el.scrollToColumnProp(id)
  await nextTick()
  await host.value?.$el.setCellsFocus(
    { x, y: rowIndex },
    { x, y: rowIndex },
    type,
  )
}

function documentKeydown(event: KeyboardEvent) {
  if (
    focused &&
    event.target instanceof Element &&
    !event.target.closest('.data-grid')
  )
    void keydown(event)
}

function dismissToolbarMenus(event: PointerEvent) {
  const target = event.target
  if (!(target instanceof Element) || target.closest('dialog[open]')) return
  for (const menu of toolbar.value?.querySelectorAll<HTMLDetailsElement>(
    ':scope > details[open]',
  ) ?? [])
    if (!menu.contains(target)) menu.open = false
}

function syncSource() {
  if (groupedMode.value) {
    groupNodes.clear()
    let rowNumber = 0
    gridSource.value = groupedRows.entries.map((entry, index) =>
      entry.kind === 'group'
        ? modelForGroup(entry.group)
        : modelFor(rows.rowAt(index), rowNumber++),
    )
    return
  }
  let source = gridSource.value
  let changed = false
  if (source.length !== remoteRows.slots.length) {
    source = Array.from({ length: remoteRows.slots.length }, () => emptyModel)
    syncedRows.length = remoteRows.slots.length
    syncedRows.fill(null)
    syncedPages.clear()
    changed = true
  }
  const cachedPages = new Set(remoteRows.snapshot().cachedPages)
  for (const page of syncedPages) {
    if (cachedPages.has(page)) continue
    const start = page * remotePageSize
    const end = Math.min(start + remotePageSize, source.length)
    for (let index = start; index < end; index++) {
      if (!syncedRows[index]) continue
      source[index] = emptyModel
      syncedRows[index] = null
      changed = true
    }
  }
  for (const page of cachedPages) {
    const start = page * remotePageSize
    const end = Math.min(start + remotePageSize, remoteRows.slots.length)
    for (let index = start; index < end; index++) {
      const row = remoteRows.rowAt(index)
      if (syncedRows[index] === row) continue
      source[index] = modelFor(row, index)
      syncedRows[index] = row
      changed = true
    }
  }
  syncedPages.clear()
  for (const page of cachedPages) syncedPages.add(page)
  if (changed) gridSource.value = source.slice()
}

async function initialize(anchor = 0, preserveCellErrors = false) {
  const generation = ++initializationGeneration
  initializing = true
  realtimeEpoch++
  loading.value = true
  error.value = null
  if (!preserveCellErrors)
    for (const key of cellErrors.value.keys())
      if (!cellDrafts.value.has(key)) cellErrors.value.delete(key)
  void loadFooterAggregates()
  try {
    await rows.initialize(anchor)
    if (!disposed.value && generation === initializationGeneration) {
      initializing = false
      loading.value = false
      if (!error.value) flushDeferredRealtime()
    }
  } catch (cause) {
    if (!disposed.value && generation === initializationGeneration) {
      initializing = false
      error.value = cause
      loading.value = false
    }
  }
}

function refreshHighlightedRows() {
  if (disposed.value) return Promise.resolve()
  const page = Math.max(0, rows.snapshot().activePage)
  return initialize(page * remotePageSize, true)
}

function created() {
  ready.value = true
  void initialize()
}

interface CellPointerActivation {
  rowId: string
  fieldId: string
  origin: EditOrigin
  x: number
  y: number
  dragged: boolean
}
let cellPointerActivation: CellPointerActivation | null = null

function beforeCellFocus(event: CustomEvent<BeforeSaveDataDetails>) {
  const rowId = event.detail.model?.__rowId
  const fieldId = event.detail.prop
  if (
    editorBoundary &&
    (editorBoundary.rowId !== rowId || editorBoundary.fieldId !== fieldId) &&
    !editorBoundary.editor.leave()
  ) {
    cellPointerActivation = null
    event.preventDefault()
    return
  }
  cellPointerActivation = null
  const original = event.detail.originalEvent
  if (
    !(original instanceof MouseEvent) ||
    original.type !== 'mousedown' ||
    original.button !== 0 ||
    original.shiftKey ||
    original.ctrlKey ||
    original.metaKey ||
    original.altKey ||
    !(original.target instanceof Element) ||
    original.target.closest(
      'a, button, input, textarea, select, [contenteditable="true"], .autofill-handle',
    ) ||
    typeof rowId !== 'string' ||
    !rowId ||
    event.detail.model?.__groupId ||
    typeof fieldId !== 'string' ||
    !editOriginCurrent(componentEditOrigin)
  )
    return
  const item = boundFor(fieldId)
  if (!item || !isGridScalar(item.bound) || rows.indexOfRow(rowId) < 0) return
  cellPointerActivation = {
    rowId,
    fieldId,
    origin: componentEditOrigin,
    x: original.clientX,
    y: original.clientY,
    dragged: false,
  }
}

function trackCellPointer(event: MouseEvent) {
  if (
    cellPointerActivation &&
    Math.hypot(
      event.clientX - cellPointerActivation.x,
      event.clientY - cellPointerActivation.y,
    ) > 5
  )
    cellPointerActivation.dragged = true
}

async function activateCell(event: MouseEvent) {
  const candidate = cellPointerActivation
  cellPointerActivation = null
  const grid = host.value?.$el
  if (
    !candidate ||
    candidate.dragged ||
    event.button !== 0 ||
    Math.hypot(event.clientX - candidate.x, event.clientY - candidate.y) > 5 ||
    !(event.target instanceof Node) ||
    !grid?.contains(event.target) ||
    !editOriginCurrent(candidate.origin)
  )
    return
  const range = await grid.getSelectedRange()
  if (range && (range.x !== range.x1 || range.y !== range.y1)) return
  const item = boundFor(candidate.fieldId)
  const index = rows.indexOfRow(candidate.rowId)
  if (
    !editOriginCurrent(candidate.origin) ||
    !item ||
    !isGridScalar(item.bound) ||
    index < 0
  )
    return
  await grid.setCellEdit(index, candidate.fieldId)
}

function finishCellPointer(event: MouseEvent) {
  void activateCell(event).catch((cause: unknown) => {
    if (!disposed.value) error.value = cause
  })
}

function leaveEditorOnPointer(event: PointerEvent) {
  if (
    editorBoundary &&
    !editorBoundary.editor.contains(event.target) &&
    !editorBoundary.editor.leave()
  ) {
    event.preventDefault()
    event.stopPropagation()
  }
}

function preventCompositionClick(event: MouseEvent) {
  if (
    editorBoundary?.editor.isComposing() &&
    !editorBoundary.editor.contains(event.target)
  ) {
    event.preventDefault()
    event.stopPropagation()
  }
}

function beforeEditStart(event: CustomEvent<BeforeSaveDataDetails>) {
  const rowId = event.detail.model?.__rowId
  const item =
    typeof event.detail.prop === 'string'
      ? boundFor(event.detail.prop)
      : undefined
  if (
    !editOriginCurrent(componentEditOrigin) ||
    event.detail.model?.__groupId ||
    typeof rowId !== 'string' ||
    rows.indexOfRow(rowId) < 0 ||
    !item ||
    item.bound.readOnly
  )
    event.preventDefault()
}

function focusChanged(event: CustomEvent<FocusAfterRenderEvent>) {
  focused = event.detail
  const rowId = event.detail.model?.__rowId
  const fieldId = event.detail.column?.prop
  realtime.setFocus(
    props.schema.table.id,
    typeof rowId === 'string' &&
      typeof fieldId === 'string' &&
      props.schema.fields.some((field) => field.id === fieldId)
      ? { rowId, fieldId }
      : null,
  )
}

function focusLeft(event: FocusEvent) {
  const next = event.relatedTarget
  if (
    !activeEdit.value &&
    (!(next instanceof Node) || !host.value?.$el.contains(next))
  )
    realtime.setFocus(props.schema.table.id, null)
}

function scroll(event: CustomEvent<ViewPortScrollEvent>) {
  if (event.detail.dimension !== 'rgRow') return
  const index = Math.max(0, Math.floor(event.detail.coordinate / rowSize.value))
  rows.load(index)
  loading.value =
    !groupedMode.value && rows.snapshot().cachedRows === 0 && rows.total > 0
}

function updateSortCriteria(next: RowSort[]) {
  sortCriteria.value = next
  if (ready.value) void initialize(0)
}

function addSort() {
  const fieldId = sortFieldToAdd.value
  if (!availableSortFields.value.some((field) => field.id === fieldId)) return
  updateSortCriteria([...sortCriteria.value, { fieldId, direction: 'asc' }])
  sortFieldToAdd.value = ''
}

function changeSortField(index: number, fieldId: string) {
  if (
    !sortableFields.value.some((field) => field.id === fieldId) ||
    sortCriteria.value.some(
      (sort, current) => current !== index && sort.fieldId === fieldId,
    )
  )
    return
  updateSortCriteria(
    sortCriteria.value.map((sort, current) =>
      current === index ? { ...sort, fieldId } : sort,
    ),
  )
}

function changeSortDirection(index: number, direction: string) {
  if (direction !== 'asc' && direction !== 'desc') return
  updateSortCriteria(
    sortCriteria.value.map((sort, current) =>
      current === index ? { ...sort, direction } : sort,
    ),
  )
}

function moveSort(index: number, direction: -1 | 1) {
  const neighbor = index + direction
  if (neighbor < 0 || neighbor >= sortCriteria.value.length) return
  const next = [...sortCriteria.value]
  ;[next[index], next[neighbor]] = [next[neighbor]!, next[index]!]
  updateSortCriteria(next)
}

function removeSort(index: number) {
  updateSortCriteria(
    sortCriteria.value.filter((_, current) => current !== index),
  )
}

function sortFieldOptions(index: number) {
  const selected = sortCriteria.value[index]?.fieldId
  return sortableFields.value.filter(
    (field) =>
      field.id === selected ||
      !sortCriteria.value.some(
        (sort, current) => current !== index && sort.fieldId === field.id,
      ),
  )
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
  const origin = componentEditOrigin
  if (!editOriginCurrent(origin)) return
  if (event.detail.model?.__groupId) return
  if (typeof event.detail.prop !== 'string') return
  const item = boundFor(event.detail.prop)
  const rowId = event.detail.model?.__rowId ?? focused?.model?.__rowId
  const rowIndex =
    typeof rowId === 'string' ? rows.indexOfRow(rowId) : event.detail.rowIndex
  const row = rows.rowAt(rowIndex)
  if (!item || item.bound.readOnly) return
  if (!row) {
    error.value = new Error(t('grid.saveFailed'))
    return
  }
  const normalized = item.bound.normalize(event.detail.val)
  const key = `${row.id}:${item.field.id}`
  if (!normalized.ok) {
    cellErrors.value.set(key, fieldErrorMessage(normalized.reason))
    const draft = cellDrafts.value.get(key)
    if (draft) draft.pending = false
    return
  }
  const previousIntent = cellIntents.get(key)
  const currentValue = previousIntent
    ? previousIntent.value
    : (deferredRows.get(row.id) ?? row).fields[item.field.id]
  if (
    currentValue !== undefined &&
    JSON.stringify(currentValue) === JSON.stringify(normalized.value)
  ) {
    const draft = cellDrafts.value.get(key)
    if (previousIntent && draft && previousIntent.revision !== undefined) {
      draft.revision = previousIntent.revision
      draft.pending = true
    } else if (!previousIntent) discardCellDraft(row.id, item.field.id)
    return
  }
  const draftAtCommit = cellDrafts.value.get(key)
  const revision = draftAtCommit?.revision
  const intent: CellIntent = {
    value: normalized.value,
    revision,
    text: draftAtCommit?.text,
  }
  cellIntents.set(key, intent)
  const predecessor = cellWriteTail
  let release = () => {}
  cellWriteTail = new Promise<void>((resolve) => {
    release = resolve
  })
  pendingCellWrites++
  saving.value = true
  error.value = null
  const controller = new AbortController()
  cellControllers.add(controller)
  try {
    await predecessor
    if (!editOriginCurrent(origin)) {
      if (sameEditScope(origin)) {
        intent.outcome = { message: editUnavailableMessage(origin) }
        const draft = cellDrafts.value.get(key)
        if (draft && draft.revision === revision) {
          draft.pending = false
          cellErrors.value.set(key, editUnavailableMessage(origin))
        }
      }
      return
    }
    const currentIndex = rows.indexOfRow(row.id)
    const currentRow =
      deferredRows.get(row.id) ??
      (currentIndex >= 0 ? rows.rowAt(currentIndex) : null) ??
      row
    if (!currentRow || currentRow.fields[item.field.id] === undefined)
      throw new Error(t('grid.saveFailed'))
    error.value = null
    const result = await mutate<RowPatchMutation>(
      `/tables/${origin.tableId}/rows/${row.id}`,
      'PATCH',
      { fields: { [item.field.id]: normalized.value } },
      controller.signal,
    )
    if (!editOriginCurrent(origin)) {
      if (sameEditScope(origin))
        intent.outcome = { message: editUnavailableMessage(origin) }
      return
    }
    if (result.row.id !== row.id) throw new Error(t('grid.saveFailed'))
    intent.outcome = { success: true }
    const before = result.previous ?? {
      [item.field.id]: currentRow.fields[item.field.id]!,
    }
    undo.record([
      patchUndoStep(
        origin.tableId,
        [{ id: row.id, fields: before }],
        [result.row],
      ),
    ])
    if (activeEdit.value || pendingCellWrites > 1)
      deferredRows.set(row.id, result.row)
    else {
      deferredRows.delete(row.id)
      const index = rows.indexOfRow(row.id)
      if (index >= 0) rows.updateRow(index, result.row)
    }
    if (columnState.config.value.highlight?.length) remoteReloadPending = true
    const draft = cellDrafts.value.get(key)
    if (!draft || draft.revision === revision) {
      cellDrafts.value.delete(key)
      cellErrors.value.delete(key)
    }
    selectedRows.value.delete(row.id)
  } catch (cause) {
    if (disposed.value || !sameEditScope(origin)) return
    if (!editOriginCurrent(origin)) {
      intent.outcome = { message: editUnavailableMessage(origin) }
      const draft = cellDrafts.value.get(key)
      if (draft && draft.revision === revision) {
        draft.pending = false
        cellErrors.value.set(key, editUnavailableMessage(origin))
      }
      return
    }
    intent.outcome = { message: errorMessage(cause) }
    if (cause instanceof RequestError && [401, 403, 404].includes(cause.status))
      handleAccessFailure(cause)
    if (item.field.uiType === 'attachment') {
      const currentIds = new Set(attachmentIds(row.fields[item.field.id]))
      for (const id of attachmentIds(normalized.value)) {
        if (!currentIds.has(id))
          void discardAttachment(origin.tableId, item.field.id, id).catch(
            () => {},
          )
      }
    }
    const draft = cellDrafts.value.get(key)
    if (!draft || draft.revision === revision) {
      if (draft) draft.pending = false
      cellErrors.value.set(key, errorMessage(cause))
    }
    error.value = cause
  } finally {
    cellControllers.delete(controller)
    if (cellIntents.get(key) === intent) cellIntents.delete(key)
    pendingCellWrites--
    release()
    if (!disposed.value && pendingCellWrites === 0) saving.value = false
  }
}

async function addRow() {
  if (!canWrite.value || saving.value) return
  saving.value = true
  error.value = null
  const anchor = rows.total
  try {
    const result = await mutate<RowMutation>(
      `/tables/${props.schema.table.id}/rows`,
      'POST',
      {
        rows: [{ fields: {} }],
      },
    )
    if (result.rows.length !== 1) throw new Error(t('grid.saveFailed'))
    undo.record([
      {
        kind: 'delete',
        tableId: props.schema.table.id,
        ids: [result.rows[0]!.id],
      },
    ])
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
  columnState.change({ kind: 'visible', id, visible })
}

async function reorderFields(ids: string[]) {
  if (
    !props.identity ||
    props.identity !== auth.user?.id ||
    !columnState.canPersist.value
  )
    return false
  columnState.change({ kind: 'order', ids })
  return columnState.flush()
}

function selectedField(id: string) {
  return visibleFieldIds.value.includes(id)
}

function moveColumn(id: string, direction: number) {
  const ids = columnFields.value.map((field) => field.id)
  const index = ids.indexOf(id)
  const next = ids.indexOf(columnNeighbor(id, direction) ?? '')
  if (index < 0 || next < 0) return
  ;[ids[index], ids[next]] = [ids[next]!, ids[index]!]
  columnState.change({ kind: 'order', ids })
}

function columnNeighbor(id: string, direction: number) {
  const field = columnFields.value.find((field) => field.id === id)
  const fields = field?.visible
    ? columnFields.value.filter((field) => field.visible)
    : columnFields.value
  return fields[fields.findIndex((field) => field.id === id) + direction]?.id
}

function commitColumnWidth(id: string, event: Event) {
  columnState.change({
    kind: 'width',
    id,
    width: Number((event.target as HTMLInputElement).value),
  })
  delete columnWidthDrafts.value[id]
}

function commitFrozen(event: Event) {
  columnState.change({
    kind: 'frozen',
    count: Number((event.target as HTMLInputElement).value),
  })
  frozenDraft.value = null
}

function resizeColumns(event: CustomEvent<Record<number, ColumnRegular>>) {
  for (const column of Object.values(event.detail)) {
    const id = String(column.prop)
    if (!supportedColumn(id) || typeof column.size !== 'number') continue
    columnState.change({ kind: 'width', id, width: column.size })
  }
}

function dragColumns(event: CustomEvent<ColumnDragEventData>) {
  const ids = event.detail.columns
    .map((column) => String(column.prop))
    .filter((id) => supportedColumn(id))
  const affected = new Set(ids)
  let index = 0
  const order = columnState.config.value.fields.map((column) =>
    affected.has(column.id) ? ids[index++]! : column.id,
  )
  columnState.change({ kind: 'order', ids: order })
}

watch(
  () =>
    [
      props.schema.table.id,
      props.schema.schemaVersion,
      props.schema.myLevel,
      props.viewId,
      props.identity,
      auth.user?.id,
      router.currentRoute.value.params.tableId,
      router.currentRoute.value.params.viewId,
    ] as const,
  () => {
    if (editOriginCurrent(componentEditOrigin)) return
    cellPointerActivation = null
    for (const controller of cellControllers) controller.abort()
    if (!sameEditScope(componentEditOrigin)) {
      cellDrafts.value.clear()
      cellErrors.value.clear()
      return
    }
    editorBoundary?.editor.leave()
    for (const [key, draft] of cellDrafts.value) {
      if (!draft.pending) continue
      draft.pending = false
      cellErrors.value.set(key, editUnavailableMessage(componentEditOrigin))
    }
  },
  { flush: 'sync' },
)

watch(
  () => visibleFieldIds.value.join(','),
  () => {
    if (ready.value) {
      focused = undefined
      realtime.setFocus(props.schema.table.id, null)
      void initialize(rows.snapshot().activePage * remotePageSize)
    }
  },
)
watch(
  () => JSON.stringify(props.filter),
  () => {
    if (ready.value) void initialize(0)
  },
)
watch(
  () => JSON.stringify(groupBy.value),
  () => {
    const previous = groupedRows
    groupedRows = createGroupedRows()
    previous.dispose()
    if (ready.value) void initialize(0)
  },
)
watch(searchTerm, () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    searchTimer = undefined
    if (ready.value) void initialize(0)
  }, 250)
})
watch(
  saving,
  (isSaving) => {
    realtimeEpoch++
    if (rowSync.hasWork() || deferredRemoteChanges) {
      deferredRows.clear()
      deferredRemoteChanges = false
      remoteReloadPending = true
    }
    if (!isSaving) flushDeferredRealtime()
  },
  { flush: 'sync' },
)
watch(
  () => JSON.stringify(columnState.config.value.aggregates ?? []),
  () => {
    if (ready.value) void loadFooterAggregates()
  },
)
watch(
  () => props.viewId,
  () => {
    sortCriteria.value = (columnState.config.value.sort ?? []).map(
      ({ field, dir }) => ({ fieldId: field, direction: dir }),
    )
    if (ready.value) void initialize(0)
  },
)

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
    const result = await mutate<RowMutation>(
      `/tables/${props.schema.table.id}/rows`,
      'DELETE',
      { ids },
    )
    if (result.rows.length !== ids.length) throw new Error(t('grid.saveFailed'))
    undo.record([
      { kind: 'restore', tableId: props.schema.table.id, rows: result.rows },
    ])
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

onMounted(() => {
  media.addEventListener('change', resized)
  document.addEventListener('keydown', documentKeydown, true)
  document.addEventListener('pointerdown', leaveEditorOnPointer, true)
  document.addEventListener('click', preventCompositionClick, true)
  document.addEventListener('mousemove', trackCellPointer)
  document.addEventListener('mouseup', finishCellPointer)
  document.addEventListener('pointerdown', dismissToolbarMenus)
})

defineExpose({
  refresh: () => initialize(rows.snapshot().activePage * remotePageSize),
  navigateRow: async (id: string, direction: -1 | 1) => {
    const index = rows.indexOfRow(id)
    if (index < 0) throw new Error(t('rowExpand.navigationUnavailable'))
    for (
      let target = index + direction;
      target >= 0 && target < rows.total;
      target += direction
    ) {
      if (!(await rows.ensureLoaded(target)))
        throw new Error(t('grid.loadError'))
      const row = rows.rowAt(target)
      if (row) return row.id
    }
    return null
  },
  toggleField,
  flushColumnChanges: columnState.flush,
  reorderFields,
  exportQuery: (): ExportViewQuery => ({
    fieldIds: [...visibleFieldIds.value],
    sorts: sortCriteria.value.map((sort) => ({ ...sort })),
    groupBy: [...groupBy.value],
    ...(searchTerm.value ? { search: searchTerm.value } : {}),
  }),
})

onBeforeUnmount(() => {
  disposed.value = true
  editorBoundary?.editor.captureBeforeUnmount()
  editorBoundary = null
  cellPointerActivation = null
  realtime.setFocus(props.schema.table.id, null)
  keyboardGeneration++
  clearTimeout(searchTimer)
  footerController?.abort()
  footerGeneration++
  media.removeEventListener('change', resized)
  document.removeEventListener('keydown', documentKeydown, true)
  document.removeEventListener('pointerdown', leaveEditorOnPointer, true)
  document.removeEventListener('click', preventCompositionClick, true)
  document.removeEventListener('mousemove', trackCellPointer)
  document.removeEventListener('mouseup', finishCellPointer)
  document.removeEventListener('pointerdown', dismissToolbarMenus)
  for (const controller of cellControllers) controller.abort()
  cellControllers.clear()
  for (const controller of mutationControllers) controller.abort()
  mutationControllers.clear()
  removeRealtimeListener()
  rowSync.dispose()
  deferredRows.clear()
  rows.dispose()
})
</script>

<template>
  <section
    class="data-grid"
    :aria-label="t('grid.title')"
    @keydown.capture="keydown"
  >
    <div ref="toolbar" class="grid-toolbar">
      <button
        v-if="desktop"
        type="button"
        class="grid-toolbar-button grid-fields-trigger"
        :aria-expanded="!!fieldsOpen"
        @click="emit('toggleFields')"
      >
        <AppIcon name="fields" />
        {{ t('grid.fields') }}
      </button>
      <button
        v-if="desktop"
        type="button"
        class="grid-toolbar-button grid-filter-trigger"
        :class="{ 'grid-toolbar-active': filterCount }"
        :aria-expanded="!!filterOpen"
        @click="emit('toggleFilter')"
      >
        <AppIcon name="filter" />
        {{ t('grid.filter') }}
        <span v-if="filterCount" class="grid-count">{{ filterCount }}</span>
      </button>
      <details v-if="desktop" class="grid-groups">
        <summary>
          <AppIcon name="group" />
          {{ t('grid.group') }}
          <span v-if="groupBy.length" class="grid-count">{{
            groupBy.length
          }}</span>
        </summary>
        <fieldset>
          <legend>{{ t('grid.groupFields') }}</legend>
          <ol class="grid-group-fields">
            <li v-for="id in groupBy" :key="id">
              <span>{{
                groupableFields.find((field) => field.id === id)?.name ?? id
              }}</span>
              <button
                type="button"
                :disabled="groupEditDisabled"
                :aria-label="
                  t('grid.removeGroupField', {
                    name:
                      groupableFields.find((field) => field.id === id)?.name ??
                      id,
                  })
                "
                @click="removeGroupField(id)"
              >
                {{ t('grid.remove') }}
              </button>
            </li>
          </ol>
          <label>
            <span>{{ t('grid.addGroupField') }}</span>
            <select v-model="groupFieldToAdd" :disabled="groupAddDisabled">
              <option value="">{{ t('grid.selectSortField') }}</option>
              <option
                v-for="field in availableGroupFields"
                :key="field.id"
                :value="field.id"
              >
                {{ field.name }}
              </option>
            </select>
          </label>
          <button
            type="button"
            :disabled="groupAddDisabled || !groupFieldToAdd"
            @click="addGroupField"
          >
            {{ t('grid.addGroup') }}
          </button>
          <p v-if="!columnState.canPersist.value">
            {{ t('grid.groupReadOnly') }}
          </p>
          <p v-if="columnState.groupPending.value" role="status">
            {{ t('grid.groupPending') }}
          </p>
          <button
            v-if="columnState.groupPending.value && !columnState.saving.value"
            type="button"
            @click="columnState.cancelGrouping"
          >
            {{ t('grid.cancelGroupChange') }}
          </button>
        </fieldset>
      </details>
      <details v-if="desktop" class="grid-sorts">
        <summary>
          <AppIcon name="sort" />
          {{ t('grid.sort') }}
        </summary>
        <fieldset>
          <legend>{{ t('grid.sortPriority') }}</legend>
          <ol>
            <li
              v-for="(sort, index) in sortCriteria"
              :key="sort.fieldId"
              class="grid-sort-criterion"
            >
              <label>
                {{ t('grid.sortFieldPosition', { position: index + 1 }) }}
                <select
                  :aria-label="
                    t('grid.sortFieldPosition', { position: index + 1 })
                  "
                  :value="sort.fieldId"
                  @change="
                    changeSortField(
                      index,
                      ($event.target as HTMLSelectElement).value,
                    )
                  "
                >
                  <option
                    v-for="field in sortFieldOptions(index)"
                    :key="field.id"
                    :value="field.id"
                  >
                    {{ field.name }}
                  </option>
                </select>
              </label>
              <label>
                {{ t('grid.sortDirectionLabel') }}
                <select
                  :aria-label="
                    t('grid.sortDirection', {
                      field:
                        sortableFields.find(
                          (field) => field.id === sort.fieldId,
                        )?.name ?? '',
                    })
                  "
                  :value="sort.direction"
                  @change="
                    changeSortDirection(
                      index,
                      ($event.target as HTMLSelectElement).value,
                    )
                  "
                >
                  <option value="asc">{{ t('grid.ascending') }}</option>
                  <option value="desc">{{ t('grid.descending') }}</option>
                </select>
              </label>
              <button
                type="button"
                :disabled="index === 0"
                :aria-label="
                  t('grid.moveSortUp', {
                    field:
                      sortableFields.find((field) => field.id === sort.fieldId)
                        ?.name ?? '',
                  })
                "
                @click="moveSort(index, -1)"
              >
                {{ t('grid.moveUp') }}
              </button>
              <button
                type="button"
                :disabled="index === sortCriteria.length - 1"
                :aria-label="
                  t('grid.moveSortDown', {
                    field:
                      sortableFields.find((field) => field.id === sort.fieldId)
                        ?.name ?? '',
                  })
                "
                @click="moveSort(index, 1)"
              >
                {{ t('grid.moveDown') }}
              </button>
              <button
                type="button"
                :aria-label="
                  t('grid.removeSortField', {
                    field:
                      sortableFields.find((field) => field.id === sort.fieldId)
                        ?.name ?? '',
                  })
                "
                @click="removeSort(index)"
              >
                {{ t('grid.remove') }}
              </button>
            </li>
          </ol>
          <label>
            {{ t('grid.addSortField') }}
            <select
              v-model="sortFieldToAdd"
              :aria-label="t('grid.addSortField')"
            >
              <option value="">{{ t('grid.selectSortField') }}</option>
              <option
                v-for="field in availableSortFields"
                :key="field.id"
                :value="field.id"
              >
                {{ field.name }}
              </option>
            </select>
          </label>
          <button
            type="button"
            :disabled="!sortFieldToAdd || !availableSortFields.length"
            @click="addSort"
          >
            {{ t('grid.addSort') }}
          </button>
        </fieldset>
      </details>
      <details
        v-if="desktop && columnState.canPersist.value"
        class="grid-highlights"
      >
        <summary :aria-label="t('grid.highlightMenuLabel')">
          <AppIcon name="colour" />
          {{ t('grid.highlightMenuLabel') }}
        </summary>
        <fieldset>
          <legend>{{ t('grid.highlightRules') }}</legend>
          <p>{{ t('grid.highlightDescription') }}</p>
          <p>{{ t('grid.highlightOrderHelp') }}</p>
          <ol class="grid-highlight-rules">
            <li
              v-for="(rule, index) in highlightDrafts"
              :key="rule.id"
              class="grid-highlight-rule"
              @dragover.prevent
              @drop.prevent="dropHighlightRule(index)"
            >
              <h3
                draggable="true"
                @dragstart="startHighlightDrag(index, $event)"
                @dragend="draggedHighlightIndex = null"
              >
                {{ t('grid.highlightRule', { position: index + 1 }) }}
              </h3>
              <div class="grid-highlight-controls">
                <label>
                  <span>{{
                    t('grid.highlightTarget', { position: index + 1 })
                  }}</span>
                  <select
                    :value="rule.target.type"
                    @change="
                      updateHighlightTarget(
                        rule.id,
                        ($event.target as HTMLSelectElement).value,
                      )
                    "
                  >
                    <option value="row">
                      {{ t('grid.highlightRowTarget') }}
                    </option>
                    <option value="cells">
                      {{ t('grid.highlightCellsTarget') }}
                    </option>
                  </select>
                </label>
                <label v-if="rule.target.type === 'cells'">
                  <span>{{
                    t('grid.highlightFields', { position: index + 1 })
                  }}</span>
                  <select
                    multiple
                    :aria-label="
                      t('grid.highlightFields', { position: index + 1 })
                    "
                    :value="rule.target.fields"
                    @change="updateHighlightFields(rule.id, $event)"
                  >
                    <option
                      v-for="field in highlightTargetFields"
                      :key="field.id"
                      :value="field.id"
                    >
                      {{ field.name }}
                    </option>
                  </select>
                </label>
                <label>
                  <span>{{
                    t('grid.highlightColor', { position: index + 1 })
                  }}</span>
                  <select
                    :value="rule.color"
                    @change="
                      updateHighlightColor(
                        rule.id,
                        ($event.target as HTMLSelectElement)
                          .value as HighlightColor,
                      )
                    "
                  >
                    <option
                      v-for="color in highlightColors"
                      :key="color"
                      :value="color"
                    >
                      {{ highlightColorLabel(color) }}
                    </option>
                  </select>
                </label>
              </div>
              <fieldset>
                <legend>
                  {{ t('grid.highlightCondition') }}
                  {{ index + 1 }}
                </legend>
                <FilterBuilder
                  :schema="schema"
                  :model-value="rule.when"
                  @update:model-value="updateHighlightWhen(rule.id, $event)"
                />
              </fieldset>
              <div class="grid-highlight-actions">
                <button
                  type="button"
                  :disabled="index === 0"
                  :aria-label="
                    t('grid.moveHighlightUp', { position: index + 1 })
                  "
                  @click="moveHighlightRule(index, index - 1)"
                >
                  {{ t('grid.moveUp') }}
                </button>
                <button
                  type="button"
                  :disabled="index === highlightDrafts.length - 1"
                  :aria-label="
                    t('grid.moveHighlightDown', { position: index + 1 })
                  "
                  @click="moveHighlightRule(index, index + 1)"
                >
                  {{ t('grid.moveDown') }}
                </button>
                <button
                  type="button"
                  :aria-label="
                    t('grid.removeHighlightRule', { position: index + 1 })
                  "
                  @click="removeHighlightRule(index)"
                >
                  {{ t('grid.remove') }}
                </button>
              </div>
            </li>
          </ol>
          <p v-if="highlightIncomplete" role="status">
            {{ t('grid.highlightIncomplete') }}
          </p>
          <p v-if="highlightDrafts.length >= 20" role="status">
            {{ t('grid.highlightLimit') }}
          </p>
          <button
            type="button"
            :disabled="
              highlightDrafts.length >= 20 || !highlightableFields.length
            "
            @click="addHighlightRule"
          >
            {{ t('grid.addHighlightRule') }}
          </button>
        </fieldset>
      </details>
      <slot name="after-highlight" />
      <details v-if="desktop" class="grid-row-height">
        <summary :aria-label="t('grid.rowHeight')" :title="t('grid.rowHeight')">
          <AppIcon name="height" />
        </summary>
        <fieldset>
          <legend>{{ t('grid.rowHeight') }}</legend>
          <button
            v-for="height in rowHeightOptions"
            :key="height.value"
            type="button"
            :aria-pressed="
              (columnState.config.value.rowHeight ?? 'short') === height.value
            "
            @click="
              columnState.change({ kind: 'height', height: height.value })
            "
          >
            {{ height.label }}
          </button>
        </fieldset>
      </details>
      <details v-if="desktop" class="grid-more">
        <summary :aria-label="t('grid.more')" :title="t('grid.more')">
          <AppIcon name="more" />
        </summary>
        <div class="grid-more-menu">
          <label class="grid-quick-search">
            <span>{{ t('grid.quickSearch') }}</span>
            <input
              v-model="searchTerm"
              type="search"
              maxlength="512"
              :aria-label="t('grid.quickSearch')"
              :placeholder="t('grid.quickSearchPlaceholder')"
            />
          </label>
          <button
            type="button"
            :disabled="!canWrite || saving || selectedRows.size === 0"
            @click="askDelete"
          >
            {{ t('grid.deleteRows', { count: selectedRows.size }) }}
          </button>
          <details class="grid-column-options">
            <summary>{{ t('grid.columnOptions') }}</summary>
            <fieldset>
              <legend>{{ t('grid.visibleFields') }}</legend>
              <div
                v-for="field in columnFields"
                :key="field.id"
                class="grid-column-control"
              >
                <label>
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
                <input
                  type="number"
                  :min="minColumnWidth"
                  :max="maxColumnWidth"
                  :value="columnWidthDrafts[field.id] ?? field.width ?? 180"
                  :aria-label="t('grid.columnWidth', { name: field.name })"
                  @input="
                    columnWidthDrafts[field.id] = (
                      $event.target as HTMLInputElement
                    ).value
                  "
                  @change="commitColumnWidth(field.id, $event)"
                />
                <button
                  type="button"
                  :disabled="!columnNeighbor(field.id, -1)"
                  :aria-label="t('grid.columnLeft', { name: field.name })"
                  @click="moveColumn(field.id, -1)"
                >
                  {{ t('grid.moveLeft') }}
                </button>
                <button
                  type="button"
                  :disabled="!columnNeighbor(field.id, 1)"
                  :aria-label="t('grid.columnRight', { name: field.name })"
                  @click="moveColumn(field.id, 1)"
                >
                  {{ t('grid.moveRight') }}
                </button>
              </div>
              <label>
                <span>{{ t('grid.frozenColumns') }}</span>
                <input
                  type="number"
                  min="0"
                  :max="visibleFieldIds.length"
                  :value="frozenDraft ?? columnState.config.value.frozen"
                  @input="
                    frozenDraft = ($event.target as HTMLInputElement).value
                  "
                  @change="commitFrozen"
                />
              </label>
            </fieldset>
          </details>
          <details class="grid-aggregate-settings">
            <summary>{{ t('grid.footerAggregates') }}</summary>
            <fieldset>
              <legend>{{ t('grid.footerAggregates') }}</legend>
              <label
                v-for="item in footerAggregateFields"
                :key="item.field.id"
                class="grid-aggregate-control"
              >
                <span>{{ item.field.name }}</span>
                <select
                  :aria-label="
                    t('grid.aggregateForField', { field: item.field.name })
                  "
                  :value="selectedFooterAggregate(item.field.id)"
                  @change="
                    changeFooterAggregate(
                      item.field.id,
                      ($event.target as HTMLSelectElement).value,
                    )
                  "
                >
                  <option value="none">
                    {{ t('grid.aggregateLabels.none') }}
                  </option>
                  <option
                    v-for="aggregate in item.aggregates"
                    :key="aggregate"
                    :value="aggregate"
                  >
                    {{ groupAggregateLabel(aggregate) }}
                  </option>
                </select>
              </label>
              <p v-if="footerAggregateFields.length === 0" role="status">
                {{ t('grid.noFooterAggregateFields') }}
              </p>
            </fieldset>
          </details>
        </div>
      </details>
      <span v-if="!columnState.canPersist.value" role="status">{{
        t('grid.columnsLocal')
      }}</span>
      <span v-else-if="columnState.saving.value" role="status">{{
        t('grid.columnsSaving')
      }}</span>
      <span v-if="saving" role="status">{{ t('grid.saving') }}</span>
      <span v-else-if="loading" role="status">{{ t('grid.loading') }}</span>
    </div>
    <p v-if="columnState.failure.value" role="alert">
      {{ columnState.failure.value }}
    </p>
    <button
      v-if="columnState.retryable.value"
      type="button"
      @click="columnState.retry"
    >
      {{ t('grid.columnsRetry') }}
    </button>
    <p v-if="error" role="alert">{{ gridError }}</p>
    <p v-if="cellErrors.size > 0" class="grid-validation-summary" role="alert">
      {{ [...cellErrors.values()].join(' ') }}
    </p>
    <p v-if="clipboardMessage" class="grid-validation-summary" role="alert">
      {{ clipboardMessage }}
    </p>
    <p
      v-if="clipboardErrors.length"
      class="grid-validation-summary"
      role="alert"
    >
      {{ clipboardErrors.join('\n') }}
    </p>
    <button v-if="error" type="button" @click="() => initialize()">
      {{ t('auth.retry') }}
    </button>
    <p
      v-if="rows.total === 0 && !loading && !error"
      class="visually-hidden"
      role="status"
    >
      {{ t('grid.empty') }}
    </p>
    <p v-if="!desktop" class="grid-mobile-notice" role="status">
      {{ t('grid.mobileNotice') }}
    </p>
    <div
      class="grid-surface"
      :class="{
        'grid-is-empty': rows.total === 0 && !loading && !error,
        'grid-has-field-action': desktop && canCreateField,
      }"
    >
      <RevoGrid
        v-if="desktop"
        ref="host"
        tabindex="0"
        :source="gridSource"
        :columns="columns"
        :editors="editors"
        :plugins="[ColumnMovePlugin]"
        :theme="preferences.isDark ? 'darkCompact' : 'compact'"
        :row-size="rowSize"
        :range="true"
        :resize="true"
        :readonly="!canWrite"
        :use-clipboard="true"
        :hide-attribution="true"
        @created="created"
        @viewportscroll="scroll"
        @beforecellfocus="beforeCellFocus"
        @beforeeditstart="beforeEditStart"
        @beforeedit="edit"
        @afterfocus="focusChanged"
        @focusout="focusLeft"
        @beforecopyregion="beforeCopyRegion"
        @beforecopyapply="copy"
        @beforepasteapply="paste"
        @beforecut="beforeCut"
        @clearregion="clearCutRange"
        @beforerangeedit="rangeEdit"
        @aftercolumnresize="resizeColumns"
        @columndragend="dragColumns"
      />
      <button
        v-if="desktop && canCreateField"
        type="button"
        class="grid-add-field"
        :style="{ left: `min(${addFieldOffset}px, calc(100% - 52px))` }"
        :aria-label="t('field.panel.add')"
        :title="t('field.panel.add')"
        @click="emit('createField')"
      >
        <AppIcon name="plus" />
      </button>
      <button
        v-if="desktop && canWrite"
        type="button"
        class="grid-add-row"
        :disabled="saving"
        :aria-label="t('grid.addRow')"
        @click="addRow"
      >
        <AppIcon name="plus" />
      </button>
    </div>
    <p v-if="footerAggregateError" role="alert">
      {{ t('grid.footerAggregateFailed', { message: footerAggregateError }) }}
    </p>
    <footer
      v-if="displayedFooterAggregates().length || footerAggregateLoading"
      class="grid-aggregate-footer"
      :aria-label="t('grid.footerAggregates')"
      aria-live="polite"
    >
      <p v-if="footerAggregateLoading" role="status">
        {{ t('grid.footerAggregatesLoading') }}
      </p>
      <ul v-else>
        <li
          v-for="aggregate in displayedFooterAggregates()"
          :key="aggregate.key"
        >
          <span>{{ aggregate.field }}</span>
          <span>{{ aggregate.aggregate }}</span>
          <output>{{ aggregate.value }}</output>
        </li>
      </ul>
    </footer>
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
    <dialog ref="addRowsDialog" :aria-label="t('grid.addRowsTitle')">
      <h2>{{ t('grid.addRowsTitle') }}</h2>
      <p>
        {{ t('grid.addRowsConfirm', { count: pendingPaste?.addedRows ?? 0 }) }}
      </p>
      <div class="grid-dialog-actions">
        <button type="button" :disabled="saving" @click="cancelPasteRows">
          {{ t('grid.cancel') }}
        </button>
        <button type="button" :disabled="saving" @click="confirmPasteRows">
          {{ t('grid.addRowsAction', { count: pendingPaste?.addedRows ?? 0 }) }}
        </button>
      </div>
    </dialog>
    <RelationPicker
      :open="relationField !== null"
      :schema="schema"
      :field="relationField"
      :row-id="relationRow?.id ?? null"
      :links="
        relationField && relationRow
          ? relationRow.fields[relationField.id]
          : null
      "
      :identity="identity"
      @updated="updateRelationRow"
      @close="closeRelationPicker"
    />
  </section>
</template>

<style>
.data-grid {
  min-width: 0;
}
.grid-surface {
  position: relative;
  min-height: min(65vh, 680px);
  background: var(--grid-canvas);
}
.data-grid revo-grid {
  position: relative;
  z-index: 0;
  display: block;
  height: min(65vh, 680px);
  min-height: 320px;
  width: 100%;
}
.data-grid .grid-has-field-action revo-grid {
  width: calc(100% - 52px);
}
.data-grid .grid-is-empty revo-grid {
  height: 40px;
  min-height: 0;
}
.data-grid .grid-add-field {
  position: absolute;
  z-index: 1;
  top: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 52px;
  height: 40px;
  padding: 0;
  color: var(--muted);
  background: var(--panel-surface);
  border: 0;
  border-right: 1px solid var(--border);
  border-left: 1px solid var(--border);
  border-radius: 0;
}
.data-grid .grid-add-row {
  display: flex;
  align-items: center;
  width: min(300px, 100%);
  min-height: 38px;
  padding: 8px 14px;
  text-align: left;
  background: var(--panel-surface);
  border: 0;
  border-bottom: 1px solid var(--border);
  border-radius: 0;
}
.grid-toolbar,
.grid-dialog-actions {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
  margin-bottom: 8px;
}
.grid-toolbar {
  position: relative;
  z-index: 1;
  gap: 2px;
  min-height: 48px;
  margin-bottom: 0;
  border-bottom: 1px solid var(--border);
}
.grid-toolbar > details > summary,
.data-grid .grid-toolbar-button {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-height: 40px;
  padding: 8px 10px;
  color: var(--text);
  background: transparent;
  border: 1px solid transparent;
  border-radius: 6px;
  cursor: pointer;
  list-style: none;
}
.grid-toolbar > details > summary::-webkit-details-marker {
  display: none;
}
.grid-toolbar > details[open] > summary,
.grid-toolbar > details > summary:hover,
.data-grid .grid-toolbar-button:hover,
.data-grid .grid-toolbar-active {
  background: color-mix(in srgb, var(--accent) 10%, transparent);
}
.grid-count {
  min-width: 22px;
  padding: 2px 6px;
  color: var(--accent);
  background: color-mix(in srgb, var(--accent) 14%, transparent);
  border-radius: 5px;
  text-align: center;
}
.grid-quick-search {
  display: grid;
  gap: 6px;
}
.grid-quick-search input {
  max-width: min(260px, 40vw);
}
.grid-groups,
.grid-sorts,
.grid-highlights,
.grid-row-height,
.grid-more {
  position: relative;
}
.grid-groups > fieldset,
.grid-sorts > fieldset,
.grid-highlights > fieldset,
.grid-row-height > fieldset,
.grid-more-menu {
  position: absolute;
  z-index: 3;
  top: 100%;
  left: 0;
  min-width: min(460px, calc(100vw - 32px));
  max-width: calc(100vw - 32px);
  max-height: 65vh;
  margin: 0;
  padding: 14px;
  overflow: auto;
  background: var(--panel-surface);
  color: var(--text);
  border: 1px solid var(--border);
  border-radius: 8px;
  box-shadow: 0 8px 24px rgb(0 0 0 / 14%);
}
.grid-row-height > fieldset {
  display: grid;
  gap: 4px;
  min-width: 150px;
}
.grid-row-height button[aria-pressed='true'] {
  color: var(--accent);
  border-color: var(--accent);
}
.grid-more-menu {
  display: grid;
  gap: 10px;
}
.grid-more-menu details > summary {
  cursor: pointer;
}
.grid-more-menu fieldset {
  min-width: 0;
  margin: 8px 0 0;
  padding: 8px;
  border: 1px solid var(--border);
}
.grid-group-fields {
  display: grid;
  gap: 6px;
  padding-left: 24px;
}
.grid-group-fields li {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.grid-column-control {
  display: grid;
  grid-template-columns: minmax(100px, 1fr) 75px auto auto;
  align-items: center;
  gap: 4px;
}
.grid-column-control > input {
  width: 75px;
}
.grid-column-options label,
.grid-groups label,
.grid-sorts label,
.grid-aggregate-control {
  display: flex;
  gap: 8px;
  padding: 4px;
}
.grid-aggregate-control {
  justify-content: space-between;
}
.grid-aggregate-control select {
  min-width: 150px;
}
.grid-aggregate-footer {
  overflow-x: auto;
  border-top: 1px solid var(--border);
  color: var(--text);
}
.grid-aggregate-footer ul {
  display: flex;
  gap: 20px;
  list-style: none;
  margin: 0;
  padding: 8px;
}
.grid-aggregate-footer li {
  display: flex;
  gap: 6px;
  white-space: nowrap;
}
.grid-sorts ol {
  display: grid;
  gap: 4px;
  padding-left: 28px;
}
.grid-highlight-rules {
  display: grid;
  gap: 8px;
  padding-left: 28px;
}
.grid-highlight-rule {
  border: 1px solid var(--border);
  padding: 8px;
}
.grid-highlight-rule h3 {
  cursor: grab;
}
.grid-highlight-rule h3:active {
  cursor: grabbing;
}
.grid-highlight-controls {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 8px;
}
.grid-highlight-controls label {
  display: grid;
}
.grid-highlight-cell {
  display: flex;
  width: 100%;
  min-height: 32px;
  align-items: center;
  box-sizing: border-box;
}
.grid-highlight-cell[data-row-highlight],
.grid-highlight-cell[data-cell-highlight] {
  --muted: currentColor;
  --accent: currentColor;
}
.grid-highlight-cell[data-row-highlight='red-2']:not([data-cell-highlight]),
.grid-highlight-cell[data-cell-highlight='red-2'] {
  background-color: var(--highlight-red-2-bg);
  color: var(--highlight-red-2-text);
}
.grid-highlight-cell[data-row-highlight='orange-2']:not([data-cell-highlight]),
.grid-highlight-cell[data-cell-highlight='orange-2'] {
  background-color: var(--highlight-orange-2-bg);
  color: var(--highlight-orange-2-text);
}
.grid-highlight-cell[data-row-highlight='amber-3']:not([data-cell-highlight]),
.grid-highlight-cell[data-cell-highlight='amber-3'] {
  background-color: var(--highlight-amber-3-bg);
  color: var(--highlight-amber-3-text);
}
.grid-highlight-cell[data-row-highlight='green-2']:not([data-cell-highlight]),
.grid-highlight-cell[data-cell-highlight='green-2'] {
  background-color: var(--highlight-green-2-bg);
  color: var(--highlight-green-2-text);
}
.grid-highlight-cell[data-row-highlight='blue-2']:not([data-cell-highlight]),
.grid-highlight-cell[data-cell-highlight='blue-2'] {
  background-color: var(--highlight-blue-2-bg);
  color: var(--highlight-blue-2-text);
}
.grid-highlight-cell[data-row-highlight='purple-2']:not([data-cell-highlight]),
.grid-highlight-cell[data-cell-highlight='purple-2'] {
  background-color: var(--highlight-purple-2-bg);
  color: var(--highlight-purple-2-text);
}
.grid-highlight-cell[data-cell-highlight='red-2'],
.grid-highlight-cell[data-cell-highlight='orange-2'],
.grid-highlight-cell[data-cell-highlight='amber-3'],
.grid-highlight-cell[data-cell-highlight='green-2'],
.grid-highlight-cell[data-cell-highlight='blue-2'],
.grid-highlight-cell[data-cell-highlight='purple-2'] {
  position: relative;
  z-index: 1;
}
.grid-highlight-cell[data-row-highlight] .grid-cell-invalid,
.grid-highlight-cell[data-cell-highlight] .grid-cell-invalid {
  color: var(--highlight-red-2-text);
}
.grid-highlight-cell[data-row-highlight] .relation-edit,
.grid-highlight-cell[data-cell-highlight] .relation-edit {
  text-decoration: underline;
}
.grid-sort-criterion {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px;
}
.grid-cell-invalid {
  display: grid;
  color: #c0362c;
}
.grid-cell-invalid [role='alert'] {
  font-size: 11px;
  white-space: normal;
}
.grid-validation-summary {
  white-space: pre-wrap;
}
.data-grid .grid-group-toggle {
  width: 100%;
  overflow: hidden;
  text-align: left;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 600;
}
.data-grid .grid-group-cell {
  white-space: nowrap;
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
.grid-mobile-notice {
  display: none;
}
@media (max-width: 767px) {
  .grid-mobile-notice {
    display: block;
  }
}
</style>
