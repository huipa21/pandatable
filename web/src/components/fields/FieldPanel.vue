<script setup lang="ts">
// Field list and editor on the real table schema shell (D-130). Lazy-loaded;
// every mutation returns or reloads the authoritative schema.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { request, RequestError } from '../../api/client'
import type {
  FieldMutation,
  FormulaCheck,
  RelationMutation,
  TableField,
  TableSchema,
  TypeChangeDryRun,
  ViewColumns,
} from '../../api/types'
import { useAuthStore } from '../../stores/auth'
import { useNavStore } from '../../stores/nav'
import { isChoiceColor, nextChoiceColor } from '../../fields/choice'
import { fieldKindLabel } from '../../fields/messages'
import { CHECKBOX_ICONS, type CheckboxIcon } from '../../fields/number'
import { isKind, kindConstraints } from '../../fields/registry'
import type { FieldEnv, Kind } from '../../fields/types'
import AppIcon from '../AppIcon.vue'
import ChoiceColorPicker from './ChoiceColorPicker.vue'
import CheckboxIconPreview from './CheckboxIcon.vue'
import FieldTypePicker from './FieldTypePicker.vue'
import {
  createBody,
  dateFormatLabel,
  displayEligible,
  draftFrom,
  MAX_DESCRIPTION,
  mutationMessage,
  newChoice,
  patchBody,
  sizeLabel,
  STORAGE_OPTIONS,
  timeFormatLabel,
  timezoneLabel,
  USER_KINDS,
  type FieldDraft,
} from './fieldForm'

const MAX_CHOICES = 500

const props = defineProps<{
  schema: TableSchema
  menu?: boolean
  columns?: ViewColumns['fields']
  prepareReorder?: () => Promise<boolean>
  persistReorder?: (ids: string[]) => Promise<boolean>
  finishReorder?: () => void
}>()
const emit = defineEmits<{
  update: [schema: TableSchema]
  reload: []
  visibility: [id: string, visible: boolean]
  reorder: [ids: string[]]
}>()

const { t, locale } = useI18n()
const auth = useAuthStore()
const nav = useNavStore()
const env = computed<FieldEnv>(() => ({
  timeZone: auth.user?.effectiveTimezone,
  locale: locale.value,
}))

const owner = computed(() => props.schema.myLevel === 'owner')
const fields = computed(() =>
  [...props.schema.fields].sort((a, b) => a.position - b.position),
)
const userFields = computed(() => fields.value.filter((f) => !f.isSystem))
const fieldSearch = ref('')
const showSystemFields = ref(false)
const orderedFields = computed(() => {
  if (!props.menu) return fields.value
  const order = new Map(
    (props.columns ?? []).map((column, index) => [column.id, index]),
  )
  return [...fields.value].sort(
    (a, b) =>
      (order.get(a.id) ?? Number.MAX_SAFE_INTEGER) -
      (order.get(b.id) ?? Number.MAX_SAFE_INTEGER),
  )
})
const displayedFields = computed(() => {
  if (!props.menu) return orderedFields.value
  const query = fieldSearch.value.trim().toLocaleLowerCase()
  return orderedFields.value.filter(
    (field) =>
      (showSystemFields.value || !field.isSystem) &&
      field.name.toLocaleLowerCase().includes(query),
  )
})
function fieldVisible(field: TableField) {
  return (
    props.columns?.find((column) => column.id === field.id)?.visible ??
    !field.isSystem
  )
}
const allDisplayedFieldsVisible = computed(
  () =>
    displayedFields.value.length > 0 &&
    displayedFields.value.every(fieldVisible),
)
function toggleDisplayedFields(visible: boolean) {
  for (const field of displayedFields.value)
    if (fieldVisible(field) !== visible) emit('visibility', field.id, visible)
}

const mutationBusy = ref(false)
const reordering = ref(false)
const busy = computed(() => mutationBusy.value || reordering.value)
const error = ref<unknown>(null)
const status = ref('')
let controller: AbortController | undefined

const openMenuId = ref<string | null>(null)
const menuPosition = ref({ left: '0px', top: '0px' })
let menuDetails: HTMLDetailsElement | null = null
const pressedFieldId = ref<string | null>(null)
const draggedFieldId = ref<string | null>(null)
const dropTarget = ref<{ id: string; edge: 'before' | 'after' } | null>(null)
const draggedChoiceId = ref<string | null>(null)
const choiceDropTarget = ref<{ id: string; edge: 'before' | 'after' } | null>(
  null,
)
const choiceMoveStatus = ref('')
const openChoiceMoveId = ref<string | null>(null)
let choiceMoveAnchor: HTMLButtonElement | null = null
let pressedPointerId: number | null = null

const editor = ref<HTMLDialogElement>()
const typeChangeEditor = ref<HTMLDialogElement>()
const relationEditor = ref<HTMLDialogElement>()
const computedEditor = ref<HTMLDialogElement>()
const formulaEditor = ref<HTMLDialogElement>()
const remover = ref<HTMLDialogElement>()
/** The field being edited (null while creating); version is captured at open. */
const editing = ref<TableField | null>(null)
const typeChanging = ref<TableField | null>(null)
const typeChangeKind = ref<Kind>('text')
const typeChangePreview = ref<TypeChangeDryRun | null>(null)
const creating = ref(false)
const choosingType = ref(true)
const draft = ref<FieldDraft>(draftFrom(null, env.value))
const checkboxIconLabels = computed<Record<CheckboxIcon, string>>(() => ({
  check: t('gridPolish.gp11.check'),
  square_check: t('gridPolish.gp11.squareCheck'),
  circle_check: t('gridPolish.gp11.circleCheck'),
  star: t('gridPolish.gp11.star'),
  heart: t('gridPolish.gp11.heart'),
  thumbs_up: t('gridPolish.gp11.thumbsUp'),
  flag: t('gridPolish.gp11.flag'),
}))
const selectedCheckboxIcon = computed<CheckboxIcon>(
  () =>
    CHECKBOX_ICONS.find((icon) => icon === draft.value.checkboxIcon) ?? 'check',
)

function checkboxIconKeydown(event: KeyboardEvent, icon: CheckboxIcon) {
  const index = CHECKBOX_ICONS.indexOf(icon)
  let next: number
  switch (event.key) {
    case 'ArrowRight':
    case 'ArrowDown':
      next = (index + 1) % CHECKBOX_ICONS.length
      break
    case 'ArrowLeft':
    case 'ArrowUp':
      next = (index + CHECKBOX_ICONS.length - 1) % CHECKBOX_ICONS.length
      break
    case 'Home':
      next = 0
      break
    case 'End':
      next = CHECKBOX_ICONS.length - 1
      break
    default:
      return
  }
  event.preventDefault()
  draft.value.checkboxIcon = CHECKBOX_ICONS[next]!
  if (event.currentTarget instanceof HTMLButtonElement) {
    const radios =
      event.currentTarget.parentElement?.querySelectorAll<HTMLButtonElement>(
        '[role="radio"]',
      )
    radios?.[next]?.focus()
  }
}
const conflict = ref(false)
const deleting = ref<TableField | null>(null)
const relationCreating = ref(false)
const relationEditing = ref<TableField | null>(null)
const computedCreating = ref(false)
const computedEditing = ref<TableField | null>(null)
const formulaCreating = ref(false)
const formulaEditing = ref<TableField | null>(null)
const formulaDraft = ref({ name: '', description: '', expression: '' })
const formulaCheck = ref<FormulaCheck | null>(null)
const formulaFailure = ref<unknown>(null)
const formulaChecking = ref(false)
const formulaCursor = ref(0)
const computedDraft = ref({
  name: '',
  description: '',
  kind: 'lookup' as 'lookup' | 'rollup',
  relationFieldId: '',
  targetFieldId: '',
  aggregate: 'count',
})
const computedTargetFields = ref<TableField[]>([])
const computedTargetLoading = ref(false)
let computedController: AbortController | undefined
let formulaController: AbortController | undefined
let formulaTimer: ReturnType<typeof setTimeout> | undefined
const relationDraft = ref({
  name: '',
  description: '',
  reverseName: '',
  targetTableId: '',
  kind: 'one_many',
  sourceSide: 'child',
  onDelete: 'set_null',
})
const relationTargetLoading = ref(false)
let relationController: AbortController | undefined
const relationCardinality = computed({
  get: () =>
    relationDraft.value.kind === 'one_many'
      ? relationDraft.value.sourceSide === 'child'
        ? 'many_one'
        : 'one_many'
      : relationDraft.value.kind,
  set: (value: string) => {
    relationDraft.value.kind = value === 'many_one' ? 'one_many' : value
    relationDraft.value.sourceSide = value === 'many_one' ? 'child' : 'parent'
  },
})
const relationChoices = computed(() => [
  { value: 'many_many', label: t('relation.manyMany'), icon: 'group' as const },
  { value: 'one_many', label: t('relation.oneMany'), icon: 'fields' as const },
  { value: 'many_one', label: t('relation.manyOne'), icon: 'sort' as const },
  { value: 'one_one', label: t('relation.oneOne'), icon: 'link' as const },
])
const computedRelations = computed(() =>
  props.schema.relations.flatMap((relation) =>
    [
      ...(relation.parentTableId === props.schema.table.id
        ? [
            {
              fieldId: relation.parentFieldId,
              targetTableId: relation.childTableId,
            },
          ]
        : []),
      ...(relation.childTableId === props.schema.table.id
        ? [
            {
              fieldId: relation.childFieldId,
              targetTableId: relation.parentTableId,
            },
          ]
        : []),
    ].flatMap(({ fieldId, targetTableId }) => {
      const field = props.schema.fields.find(
        (item) => item.id === fieldId && item.uiType === 'link',
      )
      return field ? [{ field, targetTableId }] : []
    }),
  ),
)
const computedAggregates = computed(() => {
  const type = computedTargetFields.value.find(
    (field) => field.id === computedDraft.value.targetFieldId,
  )?.uiType
  const result = ['count', 'concat']
  if (
    ['integer', 'decimal', 'float', 'currency', 'percent', 'rating'].includes(
      type ?? '',
    )
  )
    result.push('sum', 'avg')
  if (
    [
      'text',
      'long_text',
      'email',
      'url',
      'single_select',
      'integer',
      'decimal',
      'float',
      'currency',
      'percent',
      'rating',
      'date',
      'datetime',
      'time',
      'autonumber',
    ].includes(type ?? '')
  )
    result.push('min', 'max')
  if (type === 'date' || type === 'datetime') result.push('earliest', 'latest')
  return result
})
const computedEditorDisabled = computed(
  () =>
    busy.value ||
    (!computedEditing.value &&
      (computedTargetLoading.value ||
        !computedRelations.value.length ||
        !computedTargetFields.value.length)),
)
const formulaFunctions = [
  'IF',
  'AND',
  'OR',
  'NOT',
  'SWITCH',
  'BLANK',
  'ISBLANK',
  'COALESCE',
  'ROUND',
  'ROUNDUP',
  'ROUNDDOWN',
  'ABS',
  'CEILING',
  'FLOOR',
  'MOD',
  'POWER',
  'SQRT',
  'MIN',
  'MAX',
  'SUM',
  'AVERAGE',
  'INT',
  'CONCAT',
  'LEN',
  'LEFT',
  'RIGHT',
  'MID',
  'UPPER',
  'LOWER',
  'TRIM',
  'SUBSTITUTE',
  'FIND',
  'REPLACE',
  'T',
  'VALUE',
  'REGEX_MATCH',
  'NOW',
  'TODAY',
  'YEAR',
  'MONTH',
  'DAY',
  'WEEKDAY',
  'HOUR',
  'MINUTE',
  'DATEADD',
  'DATETIME_DIFF',
  'DATETIME_FORMAT',
  'DATE',
]
const formulaSuggestions = computed(() => {
  const before = formulaDraft.value.expression.slice(0, formulaCursor.value)
  const open = before.lastIndexOf('{')
  if (open > before.lastIndexOf('}')) {
    const search = before.slice(open + 1).toLocaleLowerCase()
    return fields.value
      .filter(
        (field) =>
          field.uiType !== 'link' &&
          field.name.toLocaleLowerCase().includes(search),
      )
      .slice(0, 8)
      .map((field) => ({ kind: 'field' as const, value: field.name }))
  }
  const word = /[A-Za-z_][A-Za-z_0-9]*$/.exec(before)?.[0] ?? ''
  if (!word)
    return formulaFunctions.map((value) => ({
      kind: 'function' as const,
      value,
    }))
  return formulaFunctions
    .filter((name) => name.startsWith(word.toUpperCase()))
    .slice(0, 8)
    .map((value) => ({ kind: 'function' as const, value }))
})
const formulaSuggestionLabel = computed(() =>
  formulaSuggestions.value[0]?.kind === 'function'
    ? t('field.panel.formulaFunctions')
    : t('field.panel.formulaFields'),
)
function formulaSuggestionText(suggestion: {
  kind: 'field' | 'function'
  value: string
}) {
  return (
    suggestion.value +
    (suggestion.kind === 'function'
      ? t('field.panel.formulaFunctionSuffix')
      : '')
  )
}

function formulaExpression(field: TableField | null) {
  const expression = field?.options.expr
  const ast = field?.options.ast
  if (typeof expression !== 'string' || ast === null || typeof ast !== 'object')
    return typeof expression === 'string' ? expression : ''

  const references: { fieldId: string; position: number; name: string }[] = []
  const visit = (node: unknown) => {
    if (node === null || typeof node !== 'object') return
    const item = node as Record<string, unknown>
    if (
      item.kind === 'field' &&
      typeof item.fieldId === 'string' &&
      typeof item.position === 'number'
    ) {
      const current = props.schema.fields.find(
        (candidate) => candidate.id === item.fieldId,
      )
      if (current) {
        references.push({
          fieldId: item.fieldId,
          position: item.position,
          name: current.name,
        })
      }
    }
    if (Array.isArray(item.children)) item.children.forEach(visit)
  }
  visit(ast)

  const characters = Array.from(expression)
  for (const reference of references.sort(
    (left, right) => right.position - left.position,
  )) {
    const start = reference.position - 1
    if (characters[start] !== '{') continue
    const end = characters.indexOf('}', start)
    if (end < 0) continue
    characters.splice(start, end - start + 1, `{${reference.name}}`)
  }
  return characters.join('')
}

function computedAggregateLabel(aggregate: string) {
  switch (aggregate) {
    case 'count':
      return t('field.panel.aggregates.count')
    case 'sum':
      return t('field.panel.aggregates.sum')
    case 'avg':
      return t('field.panel.aggregates.avg')
    case 'min':
      return t('field.panel.aggregates.min')
    case 'max':
      return t('field.panel.aggregates.max')
    case 'earliest':
      return t('field.panel.aggregates.earliest')
    case 'latest':
      return t('field.panel.aggregates.latest')
    case 'concat':
      return t('field.panel.aggregates.concat')
    default:
      return aggregate
  }
}
const bookTables = computed(() => {
  for (const workspace of nav.tree.workspaces) {
    const book = workspace.books.find(
      (item) => item.id === props.schema.table.bookId,
    )
    if (book) return book.tables
  }
  return []
})
const targetTables = computed(() =>
  bookTables.value
    .filter((table) => table.level === 'owner')
    .map((table) => ({ id: table.id, name: table.name })),
)
const relationTargetTables = computed(() =>
  relationEditing.value
    ? bookTables.value.map((table) => ({ id: table.id, name: table.name }))
    : targetTables.value,
)
const relationTargetUnavailable = computed(
  () =>
    !!relationEditing.value &&
    !!relationDraft.value.targetTableId &&
    !relationTargetTables.value.some(
      (table) => table.id === relationDraft.value.targetTableId,
    ),
)
const computedRelationUnavailable = computed(
  () =>
    !!computedEditing.value &&
    !!computedDraft.value.relationFieldId &&
    !computedRelations.value.some(
      (item) => item.field.id === computedDraft.value.relationFieldId,
    ),
)
const computedTargetUnavailable = computed(
  () =>
    !!computedEditing.value &&
    !!computedDraft.value.targetFieldId &&
    !computedTargetFields.value.some(
      (field) => field.id === computedDraft.value.targetFieldId,
    ),
)
const computedAggregateUnavailable = computed(
  () =>
    !!computedEditing.value &&
    !!computedDraft.value.aggregate &&
    !computedAggregates.value.includes(computedDraft.value.aggregate),
)
const computedTargetDisabled = computed(
  () =>
    busy.value ||
    !!computedEditing.value ||
    computedTargetLoading.value ||
    !computedTargetFields.value.length,
)
const editingRelation = computed(() =>
  props.schema.relations.find(
    (relation) =>
      relation.parentFieldId === relationEditing.value?.id ||
      relation.childFieldId === relationEditing.value?.id,
  ),
)

const constraints = computed(() => kindConstraints(draft.value.kind))
const typeChangeKinds = computed(() =>
  USER_KINDS.filter(
    (kind) =>
      kind !== typeChanging.value?.uiType &&
      kind !== 'attachment' &&
      kind !== 'autonumber',
  ),
)
const canChangeType = (field: TableField) =>
  !field.isSystem &&
  !['link', 'lookup', 'rollup', 'formula', 'attachment', 'autonumber'].includes(
    field.uiType,
  )
const editableTypeKinds = computed(() => {
  const field = editing.value
  if (!field || !canChangeType(field)) return []
  return [
    field.uiType,
    ...USER_KINDS.filter(
      (kind) =>
        kind !== field.uiType && kind !== 'attachment' && kind !== 'autonumber',
    ),
  ]
})
const storageFixed = computed(() => editing.value !== null)
const systemEdit = computed(() => editing.value?.isSystem ?? false)
const sizes = computed(() =>
  draft.value.kind === 'float' ? ['real', 'double'] : ['small', 'int', 'big'],
)
const selectKind = computed(
  () =>
    draft.value.kind === 'single_select' || draft.value.kind === 'multi_select',
)
const showFill = computed(
  () =>
    !creating.value &&
    draft.value.isRequired &&
    !(editing.value?.isRequired ?? false),
)

function stable(field: TableField) {
  return props.schema.fields.find((item) => item.id === field.id) ?? null
}

const canSetDisplay = (field: TableField) =>
  displayEligible(field) && field.id !== props.schema.table.displayFieldId

async function mutate<T>(
  path: string,
  method: 'POST' | 'PATCH' | 'DELETE',
  body: unknown,
  version?: number,
  forReorder = false,
): Promise<T | null> {
  if (mutationBusy.value || (reordering.value && !forReorder)) return null
  mutationBusy.value = true
  error.value = null
  status.value = ''
  controller?.abort()
  const own = new AbortController()
  controller = own
  try {
    return await request<T>(
      path,
      method,
      body,
      AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
      version === undefined ? {} : { ifMatch: version },
    )
  } catch (cause) {
    if (own.signal.aborted && controller !== own) return null
    error.value = cause
    if (!(cause instanceof RequestError) || cause.status >= 500) {
      // The outcome is uncertain; reconcile with the authoritative schema.
      emit('reload')
    } else if (cause.code === 'ERR_VERSION_CONFLICT') {
      conflict.value = true
      emit('reload')
    } else if ([401, 403, 404].includes(cause.status)) {
      emit('reload')
      if (cause.status !== 401) void nav.refresh(true)
    }
    return null
  } finally {
    if (controller === own) {
      controller = undefined
      mutationBusy.value = false
    }
  }
}

async function openEditor(field: TableField | null) {
  clearChoiceDrag()
  closeChoiceMoveMenu()
  choiceMoveStatus.value = ''
  error.value = null
  status.value = ''
  conflict.value = false
  editing.value = field
  creating.value = field === null
  choosingType.value = field === null
  draft.value = draftFrom(field, env.value)
  await nextTick()
  editor.value?.showModal()
}

async function openTypeChange(field: TableField, kind: Kind) {
  error.value = null
  status.value = ''
  typeChanging.value = field
  typeChangePreview.value = null
  typeChangeKind.value = kind
  await nextTick()
  typeChangeEditor.value?.showModal()
}

function selectEditKind(kind: string) {
  const field = editing.value
  if (
    !field ||
    !canChangeType(field) ||
    !isKind(kind) ||
    !editableTypeKinds.value.includes(kind) ||
    kind === field.uiType
  )
    return
  if (
    JSON.stringify(draft.value) !== JSON.stringify(draftFrom(field, env.value))
  ) {
    error.value = t('field.panel.saveBeforeTypeChange')
    return
  }
  void openTypeChange(field, kind)
}

function closeTypeChange() {
  if (busy.value) return
  typeChangeEditor.value?.close()
  typeChanging.value = null
  typeChangePreview.value = null
  error.value = null
}

function typeChangeBody(field: TableField) {
  const options =
    (typeChangeKind.value === 'single_select' ||
      typeChangeKind.value === 'multi_select') &&
    (field.uiType === 'single_select' || field.uiType === 'multi_select')
      ? field.options
      : undefined
  return {
    uiType: typeChangeKind.value,
    ...(options === undefined ? {} : { options }),
  }
}

async function previewTypeChange() {
  const field = typeChanging.value
  if (!field) return
  typeChangePreview.value = null
  const result = await mutate<TypeChangeDryRun>(
    `/fields/${field.id}/convert:dry-run`,
    'POST',
    typeChangeBody(field),
    field.version,
  )
  if (result) typeChangePreview.value = result
}

async function saveTypeChange() {
  const field = typeChanging.value
  const preview = typeChangePreview.value
  if (!field || !preview) return
  const result = await mutate<FieldMutation>(
    `/fields/${field.id}/convert`,
    'POST',
    { ...typeChangeBody(field), setFailingToNull: preview.failing > 0 },
    field.version,
  )
  if (!result) return
  const updated = result.schema.fields.find((item) => item.id === field.id)
  if (!updated) {
    error.value = t('field.panel.configurationUnavailable')
    emit('reload')
    return
  }
  emit('update', result.schema)
  typeChangeEditor.value?.close()
  typeChanging.value = null
  typeChangePreview.value = null
  editing.value = updated
  draft.value = draftFrom(updated, env.value)
  status.value = t('field.panel.saved')
}

function closeEditor() {
  if (busy.value) return
  clearChoiceDrag()
  closeChoiceMoveMenu()
  editor.value?.close()
  editing.value = null
  creating.value = false
  error.value = null
}

async function loadReverseField(targetTableId: string, fieldId: string) {
  relationController?.abort()
  const own = new AbortController()
  relationController = own
  relationTargetLoading.value = true
  try {
    const target =
      targetTableId === props.schema.table.id
        ? props.schema
        : await request<TableSchema>(
            `/tables/${targetTableId}/schema`,
            'GET',
            undefined,
            AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
          )
    if (relationController !== own) return
    const reverse = target.fields.find((item) => item.id === fieldId)
    if (reverse) relationDraft.value.reverseName = reverse.name
    else error.value = t('field.panel.configurationUnavailable')
  } catch (cause) {
    if (!own.signal.aborted) error.value = cause
  } finally {
    if (relationController === own) {
      relationController = undefined
      relationTargetLoading.value = false
    }
  }
}

async function openRelationEditor(field: TableField | null = null) {
  error.value = null
  status.value = ''
  if (!nav.loaded && !(await nav.refresh())) {
    error.value = nav.error ?? new Error(t('relation.noTables'))
    return
  }
  relationEditing.value = field
  const relation = editingRelation.value
  const parent = relation?.parentFieldId === field?.id
  relationDraft.value = {
    name: field?.name ?? '',
    description: field?.description ?? '',
    reverseName: field ? '' : props.schema.table.name,
    targetTableId: field
      ? ((parent ? relation?.childTableId : relation?.parentTableId) ?? '')
      : (targetTables.value.find((table) => table.id === props.schema.table.id)
          ?.id ??
        targetTables.value[0]?.id ??
        ''),
    kind: field ? (relation?.kind ?? '') : 'one_many',
    sourceSide: field ? (parent ? 'parent' : 'child') : 'child',
    onDelete: field ? (relation?.onDelete ?? '') : 'set_null',
  }
  relationCreating.value = true
  await nextTick()
  relationEditor.value?.showModal()
  if (field && relation && relationDraft.value.targetTableId)
    void loadReverseField(
      relationDraft.value.targetTableId,
      parent ? relation.childFieldId : relation.parentFieldId,
    )
}

function closeRelationEditor() {
  if (busy.value) return
  relationController?.abort()
  relationController = undefined
  relationTargetLoading.value = false
  relationEditor.value?.close()
  relationCreating.value = false
  relationEditing.value = null
  error.value = null
}

async function loadComputedTargetFields(selectedId = '') {
  computedController?.abort()
  computedController = undefined
  computedTargetLoading.value = false
  const relation = computedRelations.value.find(
    (item) => item.field.id === computedDraft.value.relationFieldId,
  )
  computedTargetFields.value = []
  computedDraft.value.targetFieldId = selectedId
  if (!relation) return
  const controller = new AbortController()
  computedController = controller
  computedTargetLoading.value = true
  error.value = null
  try {
    const target = await request<TableSchema>(
      `/tables/${relation.targetTableId}/schema`,
      'GET',
      undefined,
      AbortSignal.any([controller.signal, AbortSignal.timeout(10_000)]),
    )
    if (computedController !== controller) return
    computedTargetFields.value = target.fields.filter(
      (field) =>
        field.uiType !== 'link' &&
        field.uiType !== 'lookup' &&
        field.uiType !== 'rollup',
    )
    if (!computedEditing.value)
      computedDraft.value.targetFieldId =
        computedTargetFields.value[0]?.id ?? ''
  } catch (cause) {
    if (!controller.signal.aborted) error.value = cause
  } finally {
    if (computedController === controller) {
      computedController = undefined
      computedTargetLoading.value = false
    }
  }
}

async function openComputedEditor(
  kind: 'lookup' | 'rollup',
  field: TableField | null = null,
) {
  error.value = null
  status.value = ''
  computedEditing.value = field
  computedCreating.value = true
  computedDraft.value = {
    name: field?.name ?? '',
    description: field?.description ?? '',
    kind,
    relationFieldId:
      typeof field?.options.relationFieldId === 'string'
        ? field.options.relationFieldId
        : (computedRelations.value[0]?.field.id ?? ''),
    targetFieldId:
      typeof field?.options.targetFieldId === 'string'
        ? field.options.targetFieldId
        : '',
    aggregate:
      typeof field?.options.aggregate === 'string'
        ? field.options.aggregate
        : kind === 'lookup'
          ? ''
          : 'count',
  }
  await loadComputedTargetFields(computedDraft.value.targetFieldId)
  await nextTick()
  computedEditor.value?.showModal()
}

function closeComputedEditor() {
  if (busy.value) return
  computedController?.abort()
  computedEditor.value?.close()
  computedCreating.value = false
  computedEditing.value = null
  computedTargetFields.value = []
  error.value = null
}

async function saveComputedField() {
  const field = computedEditing.value
  if (field) {
    const metadata = draftFrom(field, env.value)
    metadata.name = computedDraft.value.name
    metadata.description = computedDraft.value.description
    const body = patchBody(field, metadata, env.value)
    if (!body.ok) {
      error.value = body.message
      return
    }
    const result = await mutate<FieldMutation>(
      `/fields/${field.id}`,
      'PATCH',
      body.value,
      field.version,
    )
    if (!result) return
    emit('update', result.schema)
    closeComputedEditor()
    status.value = t('field.panel.saved')
    return
  }
  const options = {
    relationFieldId: computedDraft.value.relationFieldId,
    targetFieldId: computedDraft.value.targetFieldId,
    ...(computedDraft.value.kind === 'rollup'
      ? { aggregate: computedDraft.value.aggregate }
      : {}),
  }
  const result = await mutate<FieldMutation>(
    `/tables/${props.schema.table.id}/fields`,
    'POST',
    {
      name: computedDraft.value.name,
      ...(computedDraft.value.description.trim()
        ? { description: computedDraft.value.description.trim() }
        : {}),
      uiType: computedDraft.value.kind,
      options,
    },
  )
  if (!result) return
  emit('update', result.schema)
  computedEditor.value?.close()
  computedCreating.value = false
  computedEditing.value = null
  status.value = t('field.panel.saved')
}

async function openFormulaEditor(field: TableField | null = null) {
  error.value = null
  status.value = ''
  formulaFailure.value = null
  formulaCheck.value = null
  formulaEditing.value = field
  formulaCreating.value = true
  formulaDraft.value = {
    name: field?.name ?? '',
    description: field?.description ?? '',
    expression: formulaExpression(field),
  }
  await nextTick()
  formulaEditor.value?.showModal()
  formulaEditor.value?.querySelector('textarea')?.focus()
}

function closeFormulaEditor() {
  if (busy.value) return
  if (formulaTimer) clearTimeout(formulaTimer)
  formulaTimer = undefined
  formulaController?.abort()
  formulaController = undefined
  formulaEditor.value?.close()
  formulaCreating.value = false
  formulaEditing.value = null
  formulaFailure.value = null
  formulaCheck.value = null
}

async function runFormulaCheck(): Promise<boolean> {
  if (formulaTimer) clearTimeout(formulaTimer)
  formulaTimer = undefined
  formulaController?.abort()
  const controller = new AbortController()
  formulaController = controller
  formulaChecking.value = true
  formulaFailure.value = null
  try {
    const result = await request<FormulaCheck>(
      '/formula:check',
      'POST',
      {
        tableId: props.schema.table.id,
        expr: formulaDraft.value.expression,
      },
      AbortSignal.any([controller.signal, AbortSignal.timeout(10_000)]),
    )
    if (formulaController !== controller) return false
    formulaCheck.value = result
    return result.error === undefined && result.type !== undefined
  } catch (cause) {
    if (!controller.signal.aborted) formulaFailure.value = cause
    return false
  } finally {
    if (formulaController === controller) {
      formulaController = undefined
      formulaChecking.value = false
    }
  }
}

function scheduleFormulaCheck() {
  formulaCheck.value = null
  formulaFailure.value = null
  formulaController?.abort()
  if (formulaTimer) clearTimeout(formulaTimer)
  formulaTimer = undefined
  if (!formulaDraft.value.expression.trim()) return
  formulaTimer = setTimeout(() => {
    formulaTimer = undefined
    void runFormulaCheck()
  }, 300)
}

function trackFormulaCursor(event: Event) {
  formulaCursor.value = (event.target as HTMLTextAreaElement).selectionStart
  scheduleFormulaCheck()
}

async function insertFormulaSuggestion(suggestion: {
  kind: 'field' | 'function'
  value: string
}) {
  const expression = formulaDraft.value.expression
  const before = expression.slice(0, formulaCursor.value)
  let start = formulaCursor.value
  let insertion: string
  if (suggestion.kind === 'field') {
    const open = before.lastIndexOf('{')
    start = open > before.lastIndexOf('}') ? open : formulaCursor.value
    insertion = `{${suggestion.value}}`
  } else {
    const word = /[A-Za-z_][A-Za-z_0-9]*$/.exec(before)?.[0]
    if (word) start -= word.length
    insertion = `${suggestion.value}(`
  }
  const end = formulaCursor.value
  formulaDraft.value.expression =
    expression.slice(0, start) + insertion + expression.slice(end)
  const cursor = start + insertion.length
  await nextTick()
  const textarea = formulaEditor.value?.querySelector('textarea')
  textarea?.focus()
  textarea?.setSelectionRange(cursor, cursor)
  formulaCursor.value = cursor
  scheduleFormulaCheck()
}

async function saveFormula() {
  if (busy.value || formulaChecking.value) return
  const valid = await runFormulaCheck()
  if (!valid) return
  const field = formulaEditing.value
  const result = field
    ? await mutate<FieldMutation>(
        `/fields/${field.id}`,
        'PATCH',
        {
          name: formulaDraft.value.name,
          description: formulaDraft.value.description,
          options: { expr: formulaDraft.value.expression },
        },
        field.version,
      )
    : await mutate<FieldMutation>(
        `/tables/${props.schema.table.id}/fields`,
        'POST',
        {
          name: formulaDraft.value.name,
          description: formulaDraft.value.description,
          uiType: 'formula',
          options: { expr: formulaDraft.value.expression },
        },
      )
  if (!result) return
  emit('update', result.schema)
  formulaEditor.value?.close()
  formulaCreating.value = false
  formulaEditing.value = null
  formulaCheck.value = null
  status.value = t('field.panel.saved')
}

async function createRelation() {
  const field = relationEditing.value
  if (field) {
    const metadata = draftFrom(field, env.value)
    metadata.name = relationDraft.value.name
    metadata.description = relationDraft.value.description
    const body = patchBody(field, metadata, env.value)
    if (!body.ok) {
      error.value = body.message
      return
    }
    const result = await mutate<FieldMutation>(
      `/fields/${field.id}`,
      'PATCH',
      body.value,
      field.version,
    )
    if (!result) return
    emit('update', result.schema)
    closeRelationEditor()
    status.value = t('field.panel.saved')
    return
  }
  const result = await mutate<RelationMutation>(
    `/tables/${props.schema.table.id}/relations`,
    'POST',
    {
      name: relationDraft.value.name,
      reverseName: relationDraft.value.reverseName,
      targetTableId: relationDraft.value.targetTableId,
      kind: relationDraft.value.kind,
      sourceSide: relationDraft.value.sourceSide,
      onDelete: relationDraft.value.onDelete,
    },
  )
  if (!result) return
  emit('update', result.schema)
  relationEditor.value?.close()
  relationCreating.value = false
  status.value = t('field.panel.saved')
  void nav.refresh(true)
}

function openFieldEditor(field: TableField) {
  if (field.uiType === 'formula') void openFormulaEditor(field)
  else if (field.uiType === 'link') void openRelationEditor(field)
  else if (field.uiType === 'lookup' || field.uiType === 'rollup')
    void openComputedEditor(field.uiType, field)
  else void openEditor(field)
}

// After a version conflict, reseed the open editor from the latest metadata.
watch(
  () => props.schema,
  () => {
    if (!conflict.value) return
    conflict.value = false
    if (typeChanging.value) {
      const latest = stable(typeChanging.value)
      typeChangePreview.value = null
      if (!latest || !canChangeType(latest)) {
        typeChangeEditor.value?.close()
        typeChanging.value = null
      } else {
        typeChanging.value = latest
        if (typeChangeKind.value === latest.uiType)
          typeChangeKind.value = typeChangeKinds.value[0] ?? 'text'
      }
    }
    if (relationEditing.value) {
      const latest = stable(relationEditing.value)
      if (!latest) closeRelationEditor()
      else {
        relationEditing.value = latest
        relationDraft.value.name = latest.name
        relationDraft.value.description = latest.description
      }
    }
    if (computedEditing.value) {
      const latest = stable(computedEditing.value)
      if (!latest) closeComputedEditor()
      else {
        computedEditing.value = latest
        computedDraft.value.name = latest.name
        computedDraft.value.description = latest.description
        computedDraft.value.relationFieldId =
          typeof latest.options.relationFieldId === 'string'
            ? latest.options.relationFieldId
            : ''
        computedDraft.value.aggregate =
          typeof latest.options.aggregate === 'string'
            ? latest.options.aggregate
            : ''
        void loadComputedTargetFields(
          typeof latest.options.targetFieldId === 'string'
            ? latest.options.targetFieldId
            : '',
        )
      }
    }
    if (formulaEditing.value) {
      const latest = stable(formulaEditing.value)
      if (!latest) closeFormulaEditor()
      else {
        formulaEditing.value = latest
        formulaDraft.value = {
          name: latest.name,
          description: latest.description,
          expression: formulaExpression(latest),
        }
        formulaCheck.value = null
      }
    }
    if (!editing.value) return
    const latest = stable(editing.value)
    clearChoiceDrag()
    closeChoiceMoveMenu()
    choiceMoveStatus.value = ''
    if (!latest) {
      editor.value?.close()
      editing.value = null
      return
    }
    editing.value = latest
    draft.value = draftFrom(latest, env.value)
  },
)

async function save() {
  if (busy.value) return
  const field = editing.value
  const body = field
    ? patchBody(field, draft.value, env.value)
    : createBody(draft.value, env.value)
  if (!body.ok) {
    error.value = body.message
    return
  }
  const result = field
    ? await mutate<FieldMutation>(
        `/fields/${field.id}`,
        'PATCH',
        body.value,
        field.version,
      )
    : await mutate<FieldMutation>(
        `/tables/${props.schema.table.id}/fields`,
        'POST',
        body.value,
      )
  if (!result) return
  emit('update', result.schema)
  closeChoiceMoveMenu()
  editor.value?.close()
  editing.value = null
  creating.value = false
  status.value = t('field.panel.saved')
}

function reorderCandidates(field: TableField) {
  return displayedFields.value.filter(
    (item) =>
      !item.isSystem &&
      (!props.menu || fieldVisible(item) || item.id === field.id),
  )
}

function moveNeighbor(field: TableField, offset: number) {
  const candidates = reorderCandidates(field)
  const index = candidates.findIndex((item) => item.id === field.id)
  return index < 0 ? undefined : candidates[index + offset]
}

function reorderPlan(
  field: TableField,
  target: TableField,
  edge: 'before' | 'after',
) {
  const candidates = reorderCandidates(field).map((item) => item.id)
  const sourceIndex = candidates.indexOf(field.id)
  const targetIndex = candidates.indexOf(target.id)
  if (sourceIndex < 0 || targetIndex < 0 || field.id === target.id) return null

  const moved = [...candidates]
  moved.splice(sourceIndex, 1)
  const insertion = moved.indexOf(target.id) + (edge === 'after' ? 1 : 0)
  moved.splice(insertion, 0, field.id)
  if (moved.every((id, index) => id === candidates[index])) return null

  const candidateIds = new Set(candidates)
  let next = 0
  const ids = orderedFields.value.map((item) =>
    candidateIds.has(item.id) ? moved[next++]! : item.id,
  )
  const schemaIds = userFields.value.map((item) => item.id)
  const schemaIndex = schemaIds.indexOf(field.id)
  const remaining = schemaIds.filter((id) => id !== field.id)
  const schemaTarget = remaining.indexOf(target.id)
  if (schemaIndex < 0 || schemaTarget < 0) return null
  return {
    ids,
    currentSchemaIndex: schemaIndex,
    schemaIndex: schemaTarget + (edge === 'after' ? 1 : 0),
    position: insertion + 1,
    total: moved.length,
  }
}

async function reorderField(
  field: TableField,
  target: TableField,
  edge: 'before' | 'after',
) {
  if (!owner.value || busy.value || field.isSystem || target.isSystem) return
  if (!reorderPlan(field, target, edge)) return

  const reorderView = props.menu && props.columns !== undefined
  reordering.value = true
  error.value = null
  status.value = t('field.panel.savingOrder')
  let prepared = false
  try {
    if (reorderView && props.prepareReorder) {
      prepared = true
      if (!(await props.prepareReorder())) {
        error.value = t('field.panel.finishViewChanges')
        return
      }
      await nextTick()
    }

    const current = stable(field)
    const destination = stable(target)
    const plan =
      current && destination && reorderPlan(current, destination, edge)
    if (!current || !plan) {
      error.value = t('field.panel.orderChanged')
      return
    }
    if (plan.schemaIndex !== plan.currentSchemaIndex) {
      const result = await mutate<FieldMutation>(
        `/fields/${current.id}`,
        'PATCH',
        { index: plan.schemaIndex },
        current.version,
        true,
      )
      if (!result) return
      emit('update', result.schema)
      await nextTick()
    }
    if (reorderView) {
      status.value = t('field.panel.savingOrder')
      if (props.persistReorder) {
        if (!(await props.persistReorder(plan.ids))) {
          error.value = t('field.panel.viewOrderFailed')
          return
        }
      } else emit('reorder', plan.ids)
    }
    status.value = t('field.panel.moved', {
      name: current.name,
      position: plan.position,
      total: plan.total,
    })
  } catch (cause) {
    error.value = cause
  } finally {
    if (prepared) props.finishReorder?.()
    if (error.value) status.value = ''
    reordering.value = false
  }
}

function move(field: TableField, offset: -1 | 1) {
  const target = moveNeighbor(field, offset)
  if (target) void reorderField(field, target, offset < 0 ? 'before' : 'after')
}

async function setDisplay(field: TableField) {
  const result = await mutate<{ eventId: string }>(
    `/tables/${props.schema.table.id}`,
    'PATCH',
    { displayFieldId: field.id },
  )
  if (!result) return
  status.value = t('field.panel.displaySet')
  emit('reload')
}

async function askDelete(field: TableField) {
  error.value = null
  deleting.value = field
  await nextTick()
  remover.value?.showModal()
}

function closeDelete() {
  if (busy.value) return
  remover.value?.close()
  deleting.value = null
  error.value = null
}

async function confirmDelete() {
  const field = deleting.value
  if (!field) return
  const result = await mutate<FieldMutation>(
    `/fields/${field.id}`,
    'DELETE',
    undefined,
    field.version,
  )
  if (!result) return
  emit('update', result.schema)
  remover.value?.close()
  deleting.value = null
  status.value = t('field.panel.deleted')
}

function closeFieldMenu(restoreFocus = false) {
  const summary = menuDetails?.querySelector<HTMLElement>('summary')
  openMenuId.value = null
  menuDetails = null
  if (restoreFocus && summary?.isConnected) summary.focus()
}

function positionFieldMenu() {
  const summary = menuDetails?.querySelector<HTMLElement>('summary')
  const menu = menuDetails?.querySelector<HTMLElement>('.field-menu')
  if (!summary || !menu) return
  const anchor = summary.getBoundingClientRect()
  const size = menu.getBoundingClientRect()
  const width = size.width || 180
  const height = size.height || 160
  const left = Math.max(
    8,
    Math.min(anchor.right - width, window.innerWidth - width - 8),
  )
  const below = anchor.bottom + 4
  const top =
    below + height <= window.innerHeight - 8
      ? below
      : Math.max(8, anchor.top - height - 4)
  menuPosition.value = { left: `${left}px`, top: `${top}px` }
}

async function toggleFieldMenu(field: TableField, event: MouseEvent) {
  if (busy.value) return
  if (openMenuId.value === field.id) {
    closeFieldMenu()
    return
  }
  const details = (event.currentTarget as HTMLElement).closest('details')
  if (!(details instanceof HTMLDetailsElement)) return
  closeFieldMenu()
  menuDetails = details
  openMenuId.value = field.id
  await nextTick()
  positionFieldMenu()
}

function clearPress(event?: PointerEvent) {
  if (
    event &&
    pressedPointerId !== null &&
    event.pointerId !== pressedPointerId
  )
    return
  pressedPointerId = null
  pressedFieldId.value = null
}

function clearDrag() {
  clearPress()
  draggedFieldId.value = null
  dropTarget.value = null
}

function pressField(field: TableField, event: PointerEvent) {
  if (
    !owner.value ||
    busy.value ||
    field.isSystem ||
    event.button !== 0 ||
    event.isPrimary === false
  )
    return
  pressedPointerId = event.pointerId
  pressedFieldId.value = field.id
}

function startFieldDrag(field: TableField, event: DragEvent) {
  if (!owner.value || busy.value || field.isSystem || !event.dataTransfer) {
    event.preventDefault()
    clearDrag()
    return
  }
  closeFieldMenu()
  draggedFieldId.value = field.id
  dropTarget.value = null
  event.dataTransfer.effectAllowed = 'move'
  event.dataTransfer.setData('text/plain', field.id)
  const tile = (event.currentTarget as HTMLElement).closest('li')
  if (tile) event.dataTransfer.setDragImage(tile, 16, 16)
}

function overField(field: TableField, event: DragEvent) {
  const source = fields.value.find((item) => item.id === draggedFieldId.value)
  if (!source) return
  const bounds = (event.currentTarget as HTMLElement).getBoundingClientRect()
  const edge =
    event.clientY < bounds.top + bounds.height / 2 ? 'before' : 'after'
  if (!reorderPlan(source, field, edge)) {
    dropTarget.value = null
    return
  }
  event.preventDefault()
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
  dropTarget.value = { id: field.id, edge }
}

function leaveField(event: DragEvent) {
  const current = event.currentTarget as HTMLElement
  if (
    event.relatedTarget instanceof Node &&
    current.contains(event.relatedTarget)
  )
    return
  dropTarget.value = null
}

function dropField(field: TableField, event: DragEvent) {
  const source = fields.value.find((item) => item.id === draggedFieldId.value)
  const target = dropTarget.value
  if (!source || !target || target.id !== field.id) {
    clearDrag()
    return
  }
  event.preventDefault()
  clearDrag()
  void reorderField(source, field, target.edge)
}

function dismissFieldMenu(event: PointerEvent) {
  if (
    menuDetails &&
    event.target instanceof Node &&
    !menuDetails.contains(event.target)
  )
    closeFieldMenu()
}

function fieldKeydown(event: KeyboardEvent) {
  if (event.key !== 'Escape') return
  if (draggedChoiceId.value) {
    clearChoiceDrag()
    event.preventDefault()
    event.stopPropagation()
    return
  }
  if (openChoiceMoveId.value) {
    closeChoiceMoveMenu(true)
    event.preventDefault()
    event.stopPropagation()
    return
  }
  if (draggedFieldId.value || pressedFieldId.value) clearDrag()
  if (!openMenuId.value || dialogOpen.value) return
  event.preventDefault()
  event.stopPropagation()
  closeFieldMenu(true)
}

function fieldWindowBlur() {
  clearDrag()
  clearChoiceDrag()
  closeChoiceMoveMenu()
  closeFieldMenu()
}

function addChoice() {
  if (draft.value.choices.length >= MAX_CHOICES) return
  const choice = newChoice()
  choice.color = nextChoiceColor(
    draft.value.choices.map((item) => item.color).filter(isChoiceColor),
  )
  draft.value.choices.push(choice)
  choiceMoveStatus.value = ''
  void nextTick(() => {
    editor.value
      ?.querySelector<HTMLInputElement>(`[data-choice-id="${choice.id}"] input`)
      ?.focus()
  })
}

function removeChoice(index: number) {
  clearChoiceDrag()
  closeChoiceMoveMenu()
  choiceMoveStatus.value = ''
  const [removed] = draft.value.choices.splice(index, 1)
  if (removed)
    draft.value.defaultChoices = draft.value.defaultChoices.filter(
      (id) => id !== removed.id,
    )
}

function clearChoiceDrag() {
  draggedChoiceId.value = null
  choiceDropTarget.value = null
}

function closeChoiceMoveMenu(restoreFocus = false) {
  const anchor = choiceMoveAnchor
  choiceMoveAnchor = null
  openChoiceMoveId.value = null
  if (restoreFocus && anchor)
    void nextTick(() => {
      if (anchor.isConnected) anchor.focus()
    })
}

function toggleChoiceMoveMenu(id: string, event: MouseEvent) {
  if (openChoiceMoveId.value === id) {
    closeChoiceMoveMenu()
    return
  }
  const anchor = event.currentTarget
  if (!(anchor instanceof HTMLButtonElement)) return
  choiceMoveAnchor = anchor
  openChoiceMoveId.value = id
  void nextTick(() => {
    if (openChoiceMoveId.value === id)
      anchor.parentElement
        ?.querySelector<HTMLButtonElement>(
          '.choice-move-controls button:not(:disabled)',
        )
        ?.focus()
  })
}

function dismissChoiceMoveMenu(event: PointerEvent) {
  if (
    choiceMoveAnchor &&
    (!(event.target instanceof Node) ||
      !choiceMoveAnchor.parentElement?.contains(event.target))
  )
    closeChoiceMoveMenu()
}

function choiceMoveFocusOut(event: FocusEvent) {
  if (
    choiceMoveAnchor &&
    (!(event.relatedTarget instanceof Node) ||
      !choiceMoveAnchor.parentElement?.contains(event.relatedTarget))
  )
    closeChoiceMoveMenu()
}

function choiceDropIndex(
  sourceId: string,
  targetId: string,
  edge: 'before' | 'after',
) {
  const choices = draft.value.choices
  const source = choices.findIndex((choice) => choice.id === sourceId)
  const target = choices.findIndex((choice) => choice.id === targetId)
  if (source < 0 || target < 0 || source === target) return null
  const boundary = target + (edge === 'after' ? 1 : 0)
  const index = boundary - (source < boundary ? 1 : 0)
  return index === source ? null : index
}

function moveChoice(id: string, index: number) {
  const choices = draft.value.choices
  const current = choices.findIndex((choice) => choice.id === id)
  if (current < 0 || index < 0 || index >= choices.length || current === index)
    return
  const [choice] = choices.splice(current, 1)
  if (!choice) return
  choices.splice(index, 0, choice)
  choiceMoveStatus.value = t('gridPolish.gp09.moved', {
    n: index + 1,
    total: choices.length,
  })
}

function choiceKeydown(id: string, event: KeyboardEvent) {
  if (!event.altKey || !['ArrowUp', 'ArrowDown'].includes(event.key)) return
  event.preventDefault()
  event.stopPropagation()
  if (busy.value) return
  const index = draft.value.choices.findIndex((choice) => choice.id === id)
  if (index < 0) return
  const handle = event.currentTarget
  closeChoiceMoveMenu()
  moveChoice(id, index + (event.key === 'ArrowUp' ? -1 : 1))
  if (handle instanceof HTMLButtonElement) void nextTick(() => handle.focus())
}

function stepChoice(id: string, direction: -1 | 1) {
  const index = draft.value.choices.findIndex((choice) => choice.id === id)
  if (!busy.value && index >= 0) moveChoice(id, index + direction)
  closeChoiceMoveMenu(true)
}

function startChoiceDrag(id: string, event: DragEvent) {
  if (busy.value || !event.dataTransfer) {
    event.preventDefault()
    clearChoiceDrag()
    return
  }
  closeChoiceMoveMenu()
  draggedChoiceId.value = id
  choiceDropTarget.value = null
  event.dataTransfer.effectAllowed = 'move'
  event.dataTransfer.setData('text/plain', id)
  const row = (event.currentTarget as HTMLElement).closest('.field-choice')
  if (row) event.dataTransfer.setDragImage(row, 16, 16)
}

function overChoice(id: string, event: DragEvent) {
  const source = draggedChoiceId.value
  if (!source || busy.value) return
  const bounds = (event.currentTarget as HTMLElement).getBoundingClientRect()
  const edge =
    event.clientY < bounds.top + bounds.height / 2 ? 'before' : 'after'
  if (choiceDropIndex(source, id, edge) === null) {
    choiceDropTarget.value = null
    return
  }
  event.preventDefault()
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
  choiceDropTarget.value = { id, edge }
}

function leaveChoice(event: DragEvent) {
  const current = event.currentTarget as HTMLElement
  if (
    event.relatedTarget instanceof Node &&
    current.contains(event.relatedTarget)
  )
    return
  choiceDropTarget.value = null
}

function dropChoice(id: string, event: DragEvent) {
  const source = draggedChoiceId.value
  const target = choiceDropTarget.value
  if (source && target?.id === id && !busy.value) {
    const index = choiceDropIndex(source, id, target.edge)
    if (index !== null) {
      event.preventDefault()
      moveChoice(source, index)
    }
  }
  clearChoiceDrag()
}

function toggleDefaultChoice(id: string, checked: boolean) {
  const others = draft.value.defaultChoices.filter((item) => item !== id)
  draft.value.defaultChoices = checked ? [...others, id] : others
}

function changeKind() {
  clearChoiceDrag()
  closeChoiceMoveMenu()
  const kind = draft.value.kind
  const name = draft.value.name
  const description = draft.value.description
  draft.value = { ...draftFrom(null, env.value), kind, name, description }
  if (kind === 'float') draft.value.size = 'double'
}

async function selectCreateKind(kind: string) {
  if (busy.value) return
  const name = relationCreating.value
    ? relationDraft.value.name
    : computedCreating.value
      ? computedDraft.value.name
      : formulaCreating.value
        ? formulaDraft.value.name
        : draft.value.name
  const description = formulaCreating.value
    ? formulaDraft.value.description
    : draft.value.description
  if (
    creating.value &&
    isKind(kind) &&
    !['lookup', 'rollup', 'formula'].includes(kind)
  ) {
    draft.value.kind = kind
    changeKind()
    choosingType.value = false
    return
  }
  closeEditor()
  closeRelationEditor()
  closeComputedEditor()
  closeFormulaEditor()
  if (kind === 'link') {
    await openRelationEditor()
    if (!relationCreating.value) return
    relationDraft.value.name = name
  } else if (kind === 'lookup' || kind === 'rollup') {
    await openComputedEditor(kind)
    computedDraft.value.name = name
  } else if (kind === 'formula') {
    await openFormulaEditor()
    formulaDraft.value.name = name
    formulaDraft.value.description = description
  } else if (isKind(kind)) {
    await openEditor(null)
    draft.value.name = name
    draft.value.description = description
    draft.value.kind = kind
    changeKind()
    choosingType.value = false
    return
  } else return
}

const message = computed(() =>
  typeof error.value === 'string' ? error.value : mutationMessage(error.value),
)
const dialogOpen = computed(
  () =>
    editing.value !== null ||
    typeChanging.value !== null ||
    creating.value ||
    deleting.value !== null ||
    relationCreating.value ||
    computedCreating.value ||
    formulaCreating.value,
)
const formulaStatus = computed(() => {
  if (formulaCheck.value?.error)
    return t('field.panel.formulaInvalid', {
      position: formulaCheck.value.error.position,
    })
  if (formulaFailure.value) return mutationMessage(formulaFailure.value)
  return ''
})

watch(displayedFields, (visible) => {
  if (
    openMenuId.value &&
    !visible.some((field) => field.id === openMenuId.value)
  )
    closeFieldMenu()
  if (
    draggedFieldId.value &&
    !visible.some((field) => field.id === draggedFieldId.value)
  )
    clearDrag()
})

onMounted(() => {
  document.addEventListener('pointerdown', dismissFieldMenu, true)
  document.addEventListener('pointerdown', dismissChoiceMoveMenu, true)
  document.addEventListener('keydown', fieldKeydown, true)
  document.addEventListener('scroll', positionFieldMenu, true)
  window.addEventListener('resize', positionFieldMenu)
  window.addEventListener('pointerup', clearPress)
  window.addEventListener('pointercancel', clearPress)
  window.addEventListener('dragend', clearDrag)
  window.addEventListener('drop', clearDrag)
  window.addEventListener('dragend', clearChoiceDrag)
  window.addEventListener('drop', clearChoiceDrag)
  window.addEventListener('blur', fieldWindowBlur)
})

onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', dismissFieldMenu, true)
  document.removeEventListener('pointerdown', dismissChoiceMoveMenu, true)
  document.removeEventListener('keydown', fieldKeydown, true)
  document.removeEventListener('scroll', positionFieldMenu, true)
  window.removeEventListener('resize', positionFieldMenu)
  window.removeEventListener('pointerup', clearPress)
  window.removeEventListener('pointercancel', clearPress)
  window.removeEventListener('dragend', clearDrag)
  window.removeEventListener('drop', clearDrag)
  window.removeEventListener('dragend', clearChoiceDrag)
  window.removeEventListener('drop', clearChoiceDrag)
  window.removeEventListener('blur', fieldWindowBlur)
  relationController?.abort()
  computedController?.abort()
  formulaController?.abort()
  if (formulaTimer) clearTimeout(formulaTimer)
  const own = controller
  controller = undefined
  own?.abort()
})
defineExpose({ openCreate: () => openEditor(null) })
</script>

<template>
  <section
    class="field-panel"
    :class="{ 'field-panel-menu': menu }"
    :aria-label="t('field.panel.title')"
  >
    <div v-if="!menu" class="field-panel-head">
      <h2>{{ t('field.panel.title') }}</h2>
      <div v-if="owner" class="field-panel-actions">
        <button type="button" :disabled="busy" @click="openEditor(null)">
          {{ t('field.panel.add') }}
        </button>
      </div>
    </div>
    <div v-else class="field-menu-search">
      <AppIcon name="search" />
      <input
        v-model="fieldSearch"
        type="search"
        :disabled="busy"
        :aria-label="t('field.panel.search')"
        :placeholder="t('field.panel.search')"
      />
      <input
        type="checkbox"
        :checked="allDisplayedFieldsVisible"
        :disabled="busy || displayedFields.length === 0"
        :aria-label="t('field.panel.showMatching')"
        @change="
          toggleDisplayedFields(($event.target as HTMLInputElement).checked)
        "
      />
    </div>
    <p v-if="!owner" class="field-read-only">
      {{ t('field.panel.readOnly') }}
    </p>
    <p class="field-status" role="status" aria-atomic="true">{{ status }}</p>
    <p v-if="error && !dialogOpen" role="alert">{{ message }}</p>
    <p v-if="menu && !displayedFields.length" class="field-menu-empty">
      {{ t('field.panel.noMatches') }}
    </p>
    <ul class="field-list">
      <li
        v-for="field in displayedFields"
        :key="field.id"
        :data-field-id="field.id"
        :data-system="field.isSystem ? 'true' : undefined"
        :class="{
          'field-tile-pressed': pressedFieldId === field.id,
          'field-tile-dragging': draggedFieldId === field.id,
          'field-drop-before':
            dropTarget?.id === field.id && dropTarget.edge === 'before',
          'field-drop-after':
            dropTarget?.id === field.id && dropTarget.edge === 'after',
        }"
        @dragover="overField(field, $event)"
        @dragleave="leaveField"
        @drop="dropField(field, $event)"
      >
        <div class="field-line">
          <input
            v-if="menu"
            type="checkbox"
            :checked="fieldVisible(field)"
            :disabled="busy"
            :aria-label="t('field.panel.showField', { name: field.name })"
            @change="
              emit(
                'visibility',
                field.id,
                ($event.target as HTMLInputElement).checked,
              )
            "
          />
          <button
            v-if="owner && !field.isSystem"
            type="button"
            class="field-drag-handle"
            :aria-label="t('field.panel.dragField', { name: field.name })"
            :disabled="busy"
            :draggable="!busy"
            @pointerdown="pressField(field, $event)"
            @dragstart="startFieldDrag(field, $event)"
            @dragend="clearDrag"
          >
            <span class="field-name">{{ field.name }}</span>
          </button>
          <span v-else class="field-name">{{ field.name }}</span>
          <span class="field-kind">{{
            field.uiType === 'link'
              ? t('field.kind.link')
              : isKind(field.uiType)
                ? fieldKindLabel(field.uiType)
                : field.uiType
          }}</span>
          <span v-if="field.isSystem && !menu" class="field-badge">
            {{ t('field.panel.system') }}
          </span>
          <span
            v-if="field.id === schema.table.displayFieldId && !menu"
            class="field-badge"
          >
            {{ t('field.panel.display') }}
          </span>
          <span v-if="field.isRequired && !menu" class="field-badge">
            {{ t('field.panel.required') }}
          </span>
          <span v-if="field.isUnique && !menu" class="field-badge">
            {{ t('field.panel.unique') }}
          </span>
          <details
            v-if="owner"
            class="node-actions field-actions"
            :open="openMenuId === field.id"
          >
            <summary
              :aria-label="t('field.panel.actions', { name: field.name })"
              :aria-expanded="openMenuId === field.id"
              :aria-disabled="busy"
              @click.prevent="toggleFieldMenu(field, $event)"
            >
              <AppIcon name="more" />
            </summary>
            <div class="field-menu" :style="menuPosition">
              <button
                type="button"
                :disabled="busy"
                @click="
                  () => {
                    closeFieldMenu(true)
                    openFieldEditor(field)
                  }
                "
              >
                {{ t('field.panel.edit') }}
              </button>
              <template v-if="!field.isSystem">
                <button
                  type="button"
                  :disabled="busy"
                  @click="
                    () => {
                      closeFieldMenu(true)
                      askDelete(field)
                    }
                  "
                >
                  {{ t('field.panel.delete') }}
                </button>
                <button
                  type="button"
                  :disabled="busy || !moveNeighbor(field, -1)"
                  @click="
                    () => {
                      closeFieldMenu(true)
                      move(field, -1)
                    }
                  "
                >
                  {{ t('field.panel.moveUp') }}
                </button>
                <button
                  type="button"
                  :disabled="busy || !moveNeighbor(field, 1)"
                  @click="
                    () => {
                      closeFieldMenu(true)
                      move(field, 1)
                    }
                  "
                >
                  {{ t('field.panel.moveDown') }}
                </button>
              </template>
            </div>
          </details>
        </div>
        <p v-if="field.description && !menu" class="field-description">
          {{ field.description }}
        </p>
      </li>
    </ul>
    <footer v-if="menu" class="field-menu-footer">
      <button
        type="button"
        :disabled="busy"
        :aria-pressed="showSystemFields"
        @click="showSystemFields = !showSystemFields"
      >
        <AppIcon name="eye" />
        {{ t('field.panel.systemFields') }}
      </button>
      <button
        v-if="owner"
        type="button"
        :disabled="busy"
        @click="openEditor(null)"
      >
        <AppIcon name="plus" />
        {{ t('field.panel.add') }}
      </button>
    </footer>

    <dialog
      ref="editor"
      class="field-dialog"
      :aria-label="
        creating ? t('field.panel.createTitle') : t('field.panel.editTitle')
      "
      @cancel.prevent="closeEditor"
    >
      <form
        v-if="creating || editing"
        class="field-create-form"
        @submit.prevent="save"
      >
        <h2>
          {{
            creating ? t('field.panel.createTitle') : t('field.panel.editTitle')
          }}
        </h2>
        <label>
          <span :class="{ 'visually-hidden': menu }">{{
            t('field.panel.name')
          }}</span>
          <input
            v-model="draft.name"
            autofocus
            maxlength="400"
            required
            :placeholder="menu ? t('field.panel.namePlaceholder') : undefined"
            :disabled="busy"
          />
        </label>
        <FieldTypePicker
          :model-value="draft.kind"
          :expanded="creating && choosingType"
          :disabled="busy || (!!editing && !canChangeType(editing))"
          :allowed-kinds="creating ? undefined : editableTypeKinds"
          @update:model-value="
            (kind) => (creating ? selectCreateKind(kind) : selectEditKind(kind))
          "
        />
        <p v-if="systemEdit">{{ t('field.panel.systemNotice') }}</p>
        <fieldset v-else-if="!creating || !choosingType" class="field-settings">
          <legend>{{ t('field.panel.settings') }}</legend>
          <p v-if="storageFixed && STORAGE_OPTIONS[draft.kind]">
            {{ t('field.panel.storageFixed') }}
          </p>
          <label v-if="draft.kind === 'text'">
            <span>{{ t('field.panel.maxLength') }}</span>
            <input
              v-model="draft.maxLength"
              inputmode="numeric"
              :disabled="busy || storageFixed"
            />
          </label>
          <label v-if="draft.kind === 'integer' || draft.kind === 'float'">
            <span>{{ t('field.panel.size') }}</span>
            <select v-model="draft.size" :disabled="busy || storageFixed">
              <option v-for="size in sizes" :key="size" :value="size">
                {{ sizeLabel(size) }}
              </option>
            </select>
          </label>
          <template v-if="draft.kind === 'decimal'">
            <label>
              <span>{{ t('field.panel.precision') }}</span>
              <input
                v-model="draft.precision"
                inputmode="numeric"
                :disabled="busy || storageFixed"
              />
            </label>
            <label>
              <span>{{ t('field.panel.scale') }}</span>
              <input
                v-model="draft.scale"
                inputmode="numeric"
                :disabled="busy || storageFixed"
              />
            </label>
          </template>
          <label v-if="draft.kind === 'float' || draft.kind === 'percent'">
            <span>{{ t('field.panel.displayDecimals') }}</span>
            <input
              v-model="draft.displayDecimals"
              inputmode="numeric"
              :disabled="busy"
            />
          </label>
          <template v-if="draft.kind === 'currency'">
            <label>
              <span>{{ t('field.panel.currency') }}</span>
              <input
                v-model="draft.currency"
                maxlength="3"
                autocapitalize="characters"
                :placeholder="t('field.panel.defaultCurrency')"
                :disabled="busy"
              />
            </label>
            <label>
              <span>{{ t('field.panel.scale') }}</span>
              <input
                v-model="draft.scale"
                inputmode="numeric"
                :disabled="busy || storageFixed"
              />
            </label>
          </template>
          <label v-if="draft.kind === 'rating'">
            <span>{{ t('field.panel.ratingMax') }}</span>
            <input
              v-model="draft.ratingMax"
              inputmode="numeric"
              :disabled="busy || storageFixed"
            />
          </label>
          <div v-if="draft.kind === 'checkbox'" class="checkbox-icon-setting">
            <span>{{ t('gridPolish.gp11.icon') }}</span>
            <div
              class="checkbox-icon-picker"
              role="radiogroup"
              :aria-label="t('gridPolish.gp11.icon')"
            >
              <button
                v-for="icon in CHECKBOX_ICONS"
                :key="icon"
                type="button"
                role="radio"
                :aria-label="checkboxIconLabels[icon]"
                :title="checkboxIconLabels[icon]"
                :aria-checked="selectedCheckboxIcon === icon"
                :tabindex="selectedCheckboxIcon === icon ? 0 : -1"
                :disabled="busy"
                @click="draft.checkboxIcon = icon"
                @keydown="checkboxIconKeydown($event, icon)"
              >
                <CheckboxIconPreview :icon="icon" :checked="true" />
              </button>
            </div>
            <div class="checkbox-icon-preview">
              <span>{{ t('gridPolish.gp11.preview') }}</span>
              <CheckboxIconPreview
                :icon="selectedCheckboxIcon"
                :checked="true"
                :label="t('field.checkbox.checked')"
              />
              <CheckboxIconPreview
                :icon="selectedCheckboxIcon"
                :checked="false"
                :label="t('field.checkbox.unchecked')"
              />
            </div>
          </div>
          <label
            v-if="['integer', 'decimal', 'float'].includes(draft.kind)"
            class="field-check"
          >
            <input
              v-model="draft.thousandSep"
              type="checkbox"
              :disabled="busy"
            />
            <span>{{ t('field.panel.thousandSep') }}</span>
          </label>
          <label v-if="draft.kind === 'date' || draft.kind === 'datetime'">
            <span>{{ t('field.panel.dateFormat') }}</span>
            <select v-model="draft.dateFormat" :disabled="busy">
              <option
                v-for="format in ['iso', 'slash', 'us', 'eu']"
                :key="format"
                :value="format"
              >
                {{ dateFormatLabel(format) }}
              </option>
            </select>
          </label>
          <template v-if="draft.kind === 'datetime' || draft.kind === 'time'">
            <label>
              <span>{{ t('field.panel.timeFormat') }}</span>
              <select v-model="draft.timeFormat" :disabled="busy">
                <option
                  v-for="format in ['24h', '12h']"
                  :key="format"
                  :value="format"
                >
                  {{ timeFormatLabel(format) }}
                </option>
              </select>
            </label>
            <label v-if="draft.kind === 'datetime'">
              <span>{{ t('field.panel.timezone') }}</span>
              <select v-model="draft.timezone" :disabled="busy">
                <option
                  v-for="zone in ['user', 'fixed']"
                  :key="zone"
                  :value="zone"
                >
                  {{ timezoneLabel(zone) }}
                </option>
              </select>
            </label>
            <label
              v-if="draft.kind === 'datetime' && draft.timezone === 'fixed'"
            >
              <span>{{ t('field.panel.fixedTz') }}</span>
              <input v-model="draft.fixedTz" maxlength="64" :disabled="busy" />
            </label>
          </template>
          <label v-if="draft.kind === 'duration'">
            <span>{{ t('field.panel.durationFormat') }}</span>
            <select v-model="draft.durationFormat" :disabled="busy">
              <option value="h:mm">
                {{ t('field.panel.durationFormats.hoursMinutes') }}
              </option>
              <option value="h:mm:ss">
                {{ t('field.panel.durationFormats.hoursMinutesSeconds') }}
              </option>
              <option value="human">
                {{ t('field.panel.durationFormats.human') }}
              </option>
            </select>
          </label>
          <label v-if="draft.kind === 'uuid'" class="field-check">
            <input
              v-model="draft.autoGenerate"
              type="checkbox"
              :disabled="busy"
            />
            <span>{{ t('field.panel.autoGenerateUUID') }}</span>
          </label>
          <template v-if="draft.kind === 'autonumber'">
            <label>
              <span>{{ t('field.panel.prefix') }}</span>
              <input
                v-model="draft.autoNumberPrefix"
                maxlength="32"
                :disabled="busy"
              />
            </label>
            <label>
              <span>{{ t('field.panel.padding') }}</span>
              <input
                v-model="draft.autoNumberPadding"
                inputmode="numeric"
                :disabled="busy"
              />
            </label>
          </template>
          <template v-if="draft.kind === 'attachment'">
            <label>
              <span>{{ t('field.panel.attachmentAccept') }}</span>
              <input
                v-model="draft.attachmentAccept"
                :placeholder="t('field.panel.attachmentAcceptHint')"
                :disabled="busy"
              />
            </label>
            <label>
              <span>{{ t('field.panel.attachmentMaxFiles') }}</span>
              <input
                v-model="draft.attachmentMaxFiles"
                inputmode="numeric"
                :disabled="busy"
              />
            </label>
          </template>
          <fieldset v-if="selectKind" class="field-choices">
            <legend>{{ t('field.panel.choices') }}</legend>
            <div
              v-for="(choice, index) in draft.choices"
              :key="choice.id"
              class="field-choice"
              :data-choice-id="choice.id"
              :class="{
                'choice-dragging': draggedChoiceId === choice.id,
                'choice-drop-before':
                  choiceDropTarget?.id === choice.id &&
                  choiceDropTarget.edge === 'before',
                'choice-drop-after':
                  choiceDropTarget?.id === choice.id &&
                  choiceDropTarget.edge === 'after',
              }"
              @dragover="overChoice(choice.id, $event)"
              @dragleave="leaveChoice"
              @drop="dropChoice(choice.id, $event)"
            >
              <div class="choice-reorder" @focusout="choiceMoveFocusOut">
                <button
                  type="button"
                  class="choice-drag-handle"
                  :aria-label="
                    t('gridPolish.gp09.reorderChoice', { n: index + 1 })
                  "
                  :title="t('gridPolish.gp09.reorderChoice', { n: index + 1 })"
                  :aria-expanded="openChoiceMoveId === choice.id"
                  :disabled="busy"
                  :draggable="!busy"
                  @click="toggleChoiceMoveMenu(choice.id, $event)"
                  @keydown="choiceKeydown(choice.id, $event)"
                  @dragstart="startChoiceDrag(choice.id, $event)"
                  @dragend="clearChoiceDrag"
                >
                  <AppIcon name="grip" :size="18" />
                </button>
                <div
                  v-if="openChoiceMoveId === choice.id"
                  class="choice-move-controls"
                  role="group"
                  :aria-label="
                    t('gridPolish.gp09.reorderChoice', { n: index + 1 })
                  "
                >
                  <button
                    type="button"
                    :aria-label="t('gridPolish.gp09.moveUp', { n: index + 1 })"
                    :disabled="busy || index === 0"
                    @click="stepChoice(choice.id, -1)"
                  >
                    {{ t('field.panel.moveUp') }}
                  </button>
                  <button
                    type="button"
                    :aria-label="
                      t('gridPolish.gp09.moveDown', { n: index + 1 })
                    "
                    :disabled="busy || index === draft.choices.length - 1"
                    @click="stepChoice(choice.id, 1)"
                  >
                    {{ t('field.panel.moveDown') }}
                  </button>
                </div>
              </div>
              <ChoiceColorPicker
                v-model="choice.color"
                :option-number="index + 1"
                :disabled="busy"
              />
              <input
                v-model="choice.label"
                maxlength="400"
                :aria-label="t('field.panel.choiceLabel', { n: index + 1 })"
                :disabled="busy"
              />
              <button
                type="button"
                class="choice-remove"
                :aria-label="t('field.panel.removeChoice', { n: index + 1 })"
                :title="t('field.panel.removeChoice', { n: index + 1 })"
                :disabled="busy"
                @click="removeChoice(index)"
              >
                <AppIcon name="close" :size="16" />
              </button>
            </div>
            <span class="visually-hidden" role="status" aria-live="polite">
              {{ choiceMoveStatus }}
            </span>
            <button
              type="button"
              :disabled="busy || draft.choices.length >= MAX_CHOICES"
              @click="addChoice"
            >
              {{ t('field.panel.addChoice') }}
            </button>
          </fieldset>
          <label v-if="constraints.required" class="field-check">
            <input
              v-model="draft.isRequired"
              type="checkbox"
              :disabled="busy"
            />
            <span>{{ t('field.panel.required') }}</span>
          </label>
          <label v-if="constraints.unique" class="field-check">
            <input v-model="draft.isUnique" type="checkbox" :disabled="busy" />
            <span>{{ t('field.panel.unique') }}</span>
          </label>
          <details
            v-if="constraints.default"
            class="field-default"
            :open="!creating"
          >
            <summary>{{ t('field.panel.setDefault') }}</summary>
            <label v-if="draft.kind === 'checkbox'">
              <span>{{ t('field.panel.defaultValue') }}</span>
              <select v-model="draft.defaultBool" :disabled="busy">
                <option value="">{{ t('field.panel.noDefault') }}</option>
                <option value="true">{{ t('field.panel.checked') }}</option>
                <option value="false">{{ t('field.panel.unchecked') }}</option>
              </select>
            </label>
            <label v-else-if="draft.kind === 'single_select'">
              <span>{{ t('field.panel.defaultValue') }}</span>
              <select
                :value="draft.defaultChoices[0] ?? ''"
                :disabled="busy"
                @change="
                  (event) => {
                    const id = (event.target as HTMLSelectElement).value
                    draft.defaultChoices = id ? [id] : []
                  }
                "
              >
                <option value="">{{ t('field.panel.noDefault') }}</option>
                <option
                  v-for="choice in draft.choices"
                  :key="choice.id"
                  :value="choice.id"
                >
                  {{ choice.label }}
                </option>
              </select>
            </label>
            <fieldset
              v-else-if="draft.kind === 'multi_select'"
              class="field-choices"
            >
              <legend>{{ t('field.panel.defaultValue') }}</legend>
              <label
                v-for="choice in draft.choices"
                :key="choice.id"
                class="field-check"
              >
                <input
                  type="checkbox"
                  :checked="draft.defaultChoices.includes(choice.id)"
                  :disabled="busy"
                  @change="
                    (event) =>
                      toggleDefaultChoice(
                        choice.id,
                        (event.target as HTMLInputElement).checked,
                      )
                  "
                />
                <span>{{ choice.label }}</span>
              </label>
            </fieldset>
            <label v-else>
              <span>{{ t('field.panel.defaultValue') }}</span>
              <input
                v-model="draft.defaultText"
                maxlength="20000"
                :placeholder="
                  draft.kind === 'date'
                    ? t('field.editor.datePlaceholder')
                    : draft.kind === 'datetime'
                      ? t('field.editor.datetimePlaceholder')
                      : undefined
                "
                :disabled="busy"
              />
            </label>
            <p v-if="draft.defaultArrayJson">
              {{ t('field.panel.arrayDefaultJSON') }}
            </p>
          </details>
          <label v-if="showFill" class="field-check">
            <input v-model="draft.fillEmpty" type="checkbox" :disabled="busy" />
            <span>{{ t('field.panel.fillEmpty') }}</span>
          </label>
        </fieldset>
        <details
          v-if="!creating || !choosingType"
          class="field-description-editor"
          :open="!creating"
        >
          <summary>{{ t('field.panel.addDescription') }}</summary>
          <label>
            <span>{{ t('field.panel.description') }}</span>
            <textarea
              v-model="draft.description"
              rows="2"
              :maxlength="MAX_DESCRIPTION * 2"
              :disabled="busy"
            />
          </label>
        </details>
        <details
          v-if="editing && canSetDisplay(editing)"
          class="field-advanced-settings"
        >
          <summary>{{ t('field.panel.advancedSettings') }}</summary>
          <button type="button" :disabled="busy" @click="setDisplay(editing)">
            {{ t('field.panel.setDisplay') }}
          </button>
        </details>
        <p v-if="status" role="status">{{ status }}</p>
        <p v-if="error" role="alert">{{ message }}</p>
        <div class="nav-actions">
          <button type="button" :disabled="busy" @click="closeEditor">
            {{ t('field.panel.cancel') }}
          </button>
          <button
            v-if="!creating || !choosingType"
            type="submit"
            :disabled="busy"
          >
            {{
              busy
                ? t('field.panel.saving')
                : creating
                  ? t('field.panel.create')
                  : t('field.panel.save')
            }}
          </button>
        </div>
      </form>
    </dialog>

    <dialog
      ref="typeChangeEditor"
      class="field-dialog"
      :aria-label="t('field.panel.typeChangeTitle')"
      @cancel.prevent="closeTypeChange"
    >
      <form v-if="typeChanging" @submit.prevent="previewTypeChange">
        <h2>{{ t('field.panel.typeChangeTitle') }}</h2>
        <p>{{ t('field.panel.typeChangeHint') }}</p>
        <dl class="field-kind-fixed">
          <dt>{{ t('field.panel.name') }}</dt>
          <dd>{{ typeChanging.name }}</dd>
        </dl>
        <label>
          <span>{{ t('field.panel.targetType') }}</span>
          <select
            v-model="typeChangeKind"
            :disabled="busy"
            @change="
              () => {
                typeChangePreview = null
                error = null
              }
            "
          >
            <option v-for="kind in typeChangeKinds" :key="kind" :value="kind">
              {{ fieldKindLabel(kind) }}
            </option>
          </select>
        </label>
        <p v-if="typeChangePreview" role="status" class="type-change-summary">
          {{
            t('field.panel.conversionSummary', {
              total: typeChangePreview.total,
              convertible: typeChangePreview.convertible,
              failing: typeChangePreview.failing,
            })
          }}
        </p>
        <ul
          v-if="typeChangePreview?.samples.length"
          class="type-change-samples"
        >
          <li v-for="sample in typeChangePreview.samples" :key="sample.id">
            {{
              t('field.panel.conversionSample', {
                id: sample.id,
                value: sample.value,
              })
            }}
          </li>
        </ul>
        <p v-if="typeChanging.isRequired && typeChangePreview?.failing">
          {{ t('field.panel.requiredCleared') }}
        </p>
        <p v-if="error" role="alert">{{ message }}</p>
        <div class="nav-actions">
          <button type="submit" :disabled="busy">
            {{
              busy ? t('field.panel.saving') : t('field.panel.checkConversion')
            }}
          </button>
          <button
            v-if="typeChangePreview?.failing === 0"
            type="button"
            :disabled="busy"
            @click="saveTypeChange"
          >
            {{ t('field.panel.convert') }}
          </button>
          <button
            v-else-if="typeChangePreview"
            type="button"
            :disabled="busy"
            @click="saveTypeChange"
          >
            {{ t('field.panel.convertNull') }}
          </button>
          <button type="button" :disabled="busy" @click="closeTypeChange">
            {{ t('field.panel.abortConversion') }}
          </button>
        </div>
      </form>
    </dialog>

    <dialog
      ref="formulaEditor"
      class="field-dialog"
      :aria-label="t('field.panel.formulaTitle')"
      @cancel.prevent="closeFormulaEditor"
    >
      <form
        v-if="formulaCreating"
        class="field-create-form"
        @submit.prevent="saveFormula"
      >
        <h2 class="visually-hidden">{{ t('field.panel.formulaTitle') }}</h2>
        <label>
          <span>{{ t('field.panel.name') }}</span>
          <input
            v-model="formulaDraft.name"
            autofocus
            required
            maxlength="400"
            :placeholder="t('field.panel.namePlaceholder')"
            :disabled="busy"
          />
        </label>
        <FieldTypePicker
          model-value="formula"
          :disabled="busy || !!formulaEditing"
          @update:model-value="selectCreateKind"
        />
        <label>
          <span>{{ t('field.panel.formulaExpression') }}</span>
          <textarea
            :value="formulaDraft.expression"
            rows="6"
            maxlength="16384"
            spellcheck="false"
            :disabled="busy"
            @input="
              (event) => {
                formulaDraft.expression = (
                  event.target as HTMLTextAreaElement
                ).value
                trackFormulaCursor(event)
              }
            "
            @keyup="
              (event) =>
                (formulaCursor = (event.target as HTMLTextAreaElement)
                  .selectionStart)
            "
            @click="
              (event) =>
                (formulaCursor = (event.target as HTMLTextAreaElement)
                  .selectionStart)
            "
          />
        </label>
        <section
          class="formula-suggestions"
          :aria-label="formulaSuggestionLabel"
        >
          <template v-if="formulaSuggestions.length">
            <button
              v-for="suggestion in formulaSuggestions"
              :key="`${suggestion.kind}:${suggestion.value}`"
              type="button"
              @click="insertFormulaSuggestion(suggestion)"
            >
              <AppIcon
                :name="suggestion.kind === 'function' ? 'formula' : 'fields'"
              />
              {{ formulaSuggestionText(suggestion) }}
            </button>
          </template>
          <span v-else>{{ t('field.panel.formulaNoSuggestions') }}</span>
        </section>
        <p v-if="formulaChecking" role="status">
          {{ t('field.panel.formulaCheck') }}
        </p>
        <p v-else-if="formulaCheck?.type" role="status">
          {{ t('field.panel.formulaType', { type: formulaCheck.type }) }}
        </p>
        <p v-if="formulaStatus" role="alert">{{ formulaStatus }}</p>
        <p v-if="error" role="alert">{{ message }}</p>
        <details class="field-description-editor" :open="!!formulaEditing">
          <summary>{{ t('field.panel.addDescription') }}</summary>
          <label>
            <span>{{ t('field.panel.description') }}</span>
            <textarea
              v-model="formulaDraft.description"
              rows="2"
              :maxlength="MAX_DESCRIPTION * 2"
              :disabled="busy"
            />
          </label>
        </details>
        <details
          v-if="formulaEditing && canSetDisplay(formulaEditing)"
          class="field-advanced-settings"
        >
          <summary>{{ t('field.panel.advancedSettings') }}</summary>
          <button
            type="button"
            :disabled="busy"
            @click="setDisplay(formulaEditing)"
          >
            {{ t('field.panel.setDisplay') }}
          </button>
        </details>
        <p v-if="status" role="status">{{ status }}</p>
        <div class="nav-actions">
          <button type="button" :disabled="busy" @click="closeFormulaEditor">
            {{ t('field.panel.cancel') }}
          </button>
          <button
            type="submit"
            :disabled="
              busy || formulaChecking || !formulaDraft.expression.trim()
            "
          >
            {{
              busy
                ? t('field.panel.saving')
                : formulaEditing
                  ? t('field.panel.save')
                  : t('field.panel.create')
            }}
          </button>
        </div>
      </form>
    </dialog>

    <dialog
      ref="computedEditor"
      class="field-dialog"
      :aria-label="
        computedEditing
          ? t('field.panel.editTitle')
          : t('field.panel.computedTitle')
      "
      @cancel.prevent="closeComputedEditor"
    >
      <form
        v-if="computedCreating"
        class="field-create-form"
        @submit.prevent="saveComputedField"
      >
        <h2 class="visually-hidden">{{ t('field.panel.computedTitle') }}</h2>
        <label>
          <span>{{ t('field.panel.name') }}</span>
          <input
            v-model="computedDraft.name"
            autofocus
            required
            maxlength="400"
            :placeholder="t('field.panel.namePlaceholder')"
            :disabled="busy"
          />
        </label>
        <FieldTypePicker
          :model-value="computedDraft.kind"
          :disabled="busy || !!computedEditing"
          @update:model-value="selectCreateKind"
        />
        <p v-if="computedEditing">{{ t('field.panel.definitionFixed') }}</p>
        <div class="field-computed-columns">
          <label>
            <span>{{ t('field.panel.computedRelation') }}</span>
            <select
              v-model="computedDraft.relationFieldId"
              required
              :disabled="busy || !!computedEditing || !computedRelations.length"
              @change="loadComputedTargetFields()"
            >
              <option
                v-if="computedRelationUnavailable"
                :value="computedDraft.relationFieldId"
              >
                {{ t('field.panel.optionUnavailable') }}
              </option>
              <option
                v-for="item in computedRelations"
                :key="item.field.id"
                :value="item.field.id"
              >
                {{ item.field.name }}
              </option>
            </select>
          </label>
          <label>
            <span>{{ t('field.panel.computedTarget') }}</span>
            <select
              v-model="computedDraft.targetFieldId"
              required
              :disabled="computedTargetDisabled"
              @change="
                computedDraft.aggregate = computedAggregates.includes(
                  computedDraft.aggregate,
                )
                  ? computedDraft.aggregate
                  : (computedAggregates[0] ?? 'count')
              "
            >
              <option
                v-if="computedTargetUnavailable"
                :value="computedDraft.targetFieldId"
              >
                {{ t('field.panel.optionUnavailable') }}
              </option>
              <option
                v-for="field in computedTargetFields"
                :key="field.id"
                :value="field.id"
              >
                {{ field.name }}
              </option>
            </select>
          </label>
        </div>
        <label v-if="computedDraft.kind === 'rollup'">
          <span>{{ t('field.panel.computedAggregate') }}</span>
          <select
            v-model="computedDraft.aggregate"
            :disabled="busy || !!computedEditing"
          >
            <option
              v-if="computedAggregateUnavailable"
              :value="computedDraft.aggregate"
            >
              {{ computedAggregateLabel(computedDraft.aggregate) }}
            </option>
            <option
              v-for="aggregate in computedAggregates"
              :key="aggregate"
              :value="aggregate"
            >
              {{ computedAggregateLabel(aggregate) }}
            </option>
          </select>
        </label>
        <p v-if="!computedRelations.length" role="alert">
          {{
            computedEditing
              ? t('field.panel.configurationUnavailable')
              : t('field.panel.computedNoRelations')
          }}
        </p>
        <p
          v-else-if="
            !computedTargetLoading && !computedTargetFields.length && !error
          "
          role="alert"
        >
          {{
            computedEditing
              ? t('field.panel.configurationUnavailable')
              : t('field.panel.computedNoFields')
          }}
        </p>
        <p v-if="computedTargetLoading" role="status">
          {{ t('field.panel.computedLoading') }}
        </p>
        <details class="field-description-editor" :open="!!computedEditing">
          <summary>{{ t('field.panel.addDescription') }}</summary>
          <label>
            <span>{{ t('field.panel.description') }}</span>
            <textarea
              v-model="computedDraft.description"
              rows="2"
              :maxlength="MAX_DESCRIPTION * 2"
              :disabled="busy"
            />
          </label>
        </details>
        <details
          v-if="computedEditing && canSetDisplay(computedEditing)"
          class="field-advanced-settings"
        >
          <summary>{{ t('field.panel.advancedSettings') }}</summary>
          <button
            type="button"
            :disabled="busy"
            @click="setDisplay(computedEditing)"
          >
            {{ t('field.panel.setDisplay') }}
          </button>
        </details>
        <p v-if="status" role="status">{{ status }}</p>
        <p v-if="error" role="alert">{{ message }}</p>
        <div class="nav-actions">
          <button type="button" :disabled="busy" @click="closeComputedEditor">
            {{ t('field.panel.cancel') }}
          </button>
          <button type="submit" :disabled="computedEditorDisabled">
            {{
              busy
                ? t('field.panel.saving')
                : computedEditing
                  ? t('field.panel.save')
                  : t('field.panel.create')
            }}
          </button>
        </div>
      </form>
    </dialog>

    <dialog
      ref="relationEditor"
      class="field-dialog"
      :aria-label="
        relationEditing ? t('field.panel.editTitle') : t('relation.createTitle')
      "
      @cancel.prevent="closeRelationEditor"
    >
      <form
        v-if="relationCreating"
        class="field-create-form"
        @submit.prevent="createRelation"
      >
        <h2 class="visually-hidden">{{ t('relation.createTitle') }}</h2>
        <label>
          <span class="visually-hidden">{{ t('relation.name') }}</span>
          <input
            v-model="relationDraft.name"
            required
            maxlength="100"
            :placeholder="t('field.panel.namePlaceholder')"
            :disabled="busy"
          />
        </label>
        <FieldTypePicker
          model-value="link"
          :disabled="busy || !!relationEditing"
          @update:model-value="selectCreateKind"
        />
        <p v-if="relationEditing">{{ t('field.panel.definitionFixed') }}</p>
        <fieldset class="field-relation-types">
          <legend>{{ t('relation.kind') }}</legend>
          <label
            v-for="choice in relationChoices"
            :key="choice.value"
            :class="{ selected: relationCardinality === choice.value }"
          >
            <AppIcon :name="choice.icon" />
            <span>{{ choice.label }}</span>
            <input
              v-model="relationCardinality"
              type="radio"
              name="relation-cardinality"
              :value="choice.value"
              :disabled="busy || !!relationEditing"
            />
          </label>
        </fieldset>
        <label>
          <span>{{ t('relation.target') }}</span>
          <select
            v-model="relationDraft.targetTableId"
            required
            :disabled="busy || !!relationEditing || !targetTables.length"
          >
            <option
              v-if="relationTargetUnavailable"
              :value="relationDraft.targetTableId"
            >
              {{ t('field.panel.optionUnavailable') }}
            </option>
            <option
              v-for="table in relationTargetTables"
              :key="table.id"
              :value="table.id"
            >
              {{ table.name }}
            </option>
          </select>
        </label>
        <details class="field-advanced-settings" :open="!!relationEditing">
          <summary>{{ t('field.panel.advancedSettings') }}</summary>
          <label>
            <span>{{ t('relation.reverseName') }}</span>
            <input
              v-model="relationDraft.reverseName"
              required
              maxlength="100"
              :placeholder="
                relationEditing ? t('field.panel.optionUnavailable') : undefined
              "
              :disabled="busy || !!relationEditing"
            />
          </label>
          <label>
            <span>{{ t('relation.onDelete') }}</span>
            <select
              v-model="relationDraft.onDelete"
              :disabled="busy || !!relationEditing"
            >
              <option value="set_null">{{ t('relation.setNull') }}</option>
              <option value="cascade">{{ t('relation.cascade') }}</option>
              <option value="restrict">{{ t('relation.restrict') }}</option>
            </select>
          </label>
        </details>
        <details v-if="relationEditing" class="field-description-editor" open>
          <summary>{{ t('field.panel.addDescription') }}</summary>
          <label>
            <span>{{ t('field.panel.description') }}</span>
            <textarea
              v-model="relationDraft.description"
              rows="2"
              :maxlength="MAX_DESCRIPTION * 2"
              :disabled="busy"
            />
          </label>
        </details>
        <p v-if="relationEditing && !editingRelation" role="alert">
          {{ t('field.panel.configurationUnavailable') }}
        </p>
        <p v-else-if="!relationEditing && !targetTables.length" role="alert">
          {{ t('relation.noTables') }}
        </p>
        <p v-if="relationTargetLoading" role="status">
          {{ t('field.panel.computedLoading') }}
        </p>
        <p v-if="status" role="status">{{ status }}</p>
        <p v-if="error" role="alert">{{ message }}</p>
        <div class="nav-actions">
          <button type="button" :disabled="busy" @click="closeRelationEditor">
            {{ t('field.panel.cancel') }}
          </button>
          <button
            type="submit"
            :disabled="busy || (!relationEditing && !targetTables.length)"
          >
            {{
              busy
                ? t('field.panel.saving')
                : relationEditing
                  ? t('field.panel.save')
                  : t('field.panel.create')
            }}
          </button>
        </div>
      </form>
    </dialog>

    <dialog
      ref="remover"
      class="field-dialog"
      :aria-label="t('field.panel.confirm')"
      @cancel.prevent="closeDelete"
    >
      <form v-if="deleting" @submit.prevent="confirmDelete">
        <h2>{{ t('field.panel.confirm') }}</h2>
        <p v-if="deleting.uiType === 'link'">
          {{ t('relation.confirmDelete', { name: deleting.name }) }}
        </p>
        <p v-else>
          {{ t('field.panel.confirmDelete', { name: deleting.name }) }}
        </p>
        <p v-if="error" role="alert">{{ message }}</p>
        <div class="nav-actions">
          <button type="submit" :disabled="busy">
            {{ t('field.panel.confirm') }}
          </button>
          <button type="button" :disabled="busy" @click="closeDelete">
            {{ t('field.panel.cancel') }}
          </button>
        </div>
      </form>
    </dialog>
  </section>
</template>

<style scoped>
.checkbox-icon-setting,
.checkbox-icon-preview {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}
.checkbox-icon-picker {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 2px;
}
.checkbox-icon-picker button {
  display: inline-grid;
  width: 32px;
  height: 32px;
  padding: 0;
  place-items: center;
  background: var(--surface);
  border: 1px solid var(--border);
}
.checkbox-icon-picker button[aria-checked='true'] {
  border-color: var(--accent);
  background: color-mix(in srgb, var(--accent) 12%, var(--surface));
}
.checkbox-icon-preview {
  color: var(--muted);
}
.field-choice {
  position: relative;
}
.choice-reorder {
  position: relative;
  flex: 0 0 auto;
}
.choice-move-controls {
  position: absolute;
  z-index: 20;
  top: calc(100% + 4px);
  left: 0;
  display: grid;
  min-width: 120px;
  gap: 4px;
  padding: 6px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 8px;
  box-shadow: 0 6px 24px rgb(0 0 0 / 18%);
}
.choice-move-controls button {
  text-align: start;
  white-space: nowrap;
}
.choice-drag-handle,
.choice-remove {
  flex: 0 0 auto;
  padding: 5px;
  color: var(--muted);
  background: transparent;
  border: 1px solid transparent;
}
.choice-drag-handle {
  cursor: grab;
}
.choice-drag-handle:active {
  cursor: grabbing;
}
.choice-dragging {
  outline: 1px dashed var(--accent);
  opacity: 0.5;
}
.choice-drop-before::before,
.choice-drop-after::after {
  position: absolute;
  z-index: 2;
  right: 0;
  left: 0;
  height: 3px;
  border-radius: 2px;
  background: var(--accent);
  content: '';
  pointer-events: none;
}
.choice-drop-before::before {
  top: -5px;
}
.choice-drop-after::after {
  bottom: -5px;
}
</style>
