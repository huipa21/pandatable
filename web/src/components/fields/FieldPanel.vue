<script setup lang="ts">
// Field list and editor on the real table schema shell (D-130). Lazy-loaded;
// every mutation returns or reloads the authoritative schema.
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { request, RequestError } from '../../api/client'
import type {
  FieldMutation,
  FormulaCheck,
  RelationMutation,
  TableField,
  TableSchema,
  TypeChangeDryRun,
} from '../../api/types'
import { useAuthStore } from '../../stores/auth'
import { useNavStore } from '../../stores/nav'
import { CHOICE_COLORS } from '../../fields/choice'
import { fieldKindLabel } from '../../fields/messages'
import { isKind, kindConstraints } from '../../fields/registry'
import type { FieldEnv, Kind } from '../../fields/types'
import {
  colorLabel,
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

const props = defineProps<{ schema: TableSchema }>()
const emit = defineEmits<{
  update: [schema: TableSchema]
  reload: []
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

const busy = ref(false)
const error = ref<unknown>(null)
const status = ref('')
let controller: AbortController | undefined

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
const draft = ref<FieldDraft>(draftFrom(null, env.value))
const conflict = ref(false)
const deleting = ref<TableField | null>(null)
const relationCreating = ref(false)
const computedCreating = ref(false)
const formulaCreating = ref(false)
const formulaEditing = ref<TableField | null>(null)
const formulaDraft = ref({ name: '', description: '', expression: '' })
const formulaCheck = ref<FormulaCheck | null>(null)
const formulaFailure = ref<unknown>(null)
const formulaChecking = ref(false)
const formulaCursor = ref(0)
const computedDraft = ref({
  name: '',
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
  reverseName: '',
  targetTableId: '',
  kind: 'one_many',
  sourceSide: 'child',
  onDelete: 'set_null',
})
const computedRelations = computed(() =>
  props.schema.relations.flatMap((relation) => {
    const sourceFieldId =
      relation.parentTableId === props.schema.table.id
        ? relation.parentFieldId
        : relation.childTableId === props.schema.table.id
          ? relation.childFieldId
          : ''
    const targetTableId =
      relation.parentTableId === props.schema.table.id
        ? relation.childTableId
        : relation.childTableId === props.schema.table.id
          ? relation.parentTableId
          : ''
    const field = props.schema.fields.find(
      (item) => item.id === sourceFieldId && item.uiType === 'link',
    )
    return field ? [{ field, targetTableId }] : []
  }),
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
    computedTargetLoading.value ||
    !computedRelations.value.length ||
    !computedTargetFields.value.length,
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
  if (!word) return []
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
const targetTables = computed(() => {
  for (const workspace of nav.tree.workspaces) {
    const book = workspace.books.find(
      (item) => item.id === props.schema.table.bookId,
    )
    if (book)
      return book.tables
        .filter((table) => table.level === 'owner')
        .map((table) => ({ id: table.id, name: table.name }))
  }
  return []
})

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
): Promise<T | null> {
  if (busy.value) return null
  busy.value = true
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
      busy.value = false
    }
  }
}

async function openEditor(field: TableField | null) {
  error.value = null
  status.value = ''
  conflict.value = false
  editing.value = field
  creating.value = field === null
  draft.value = draftFrom(field, env.value)
  await nextTick()
  editor.value?.showModal()
}

async function openTypeChange(field: TableField) {
  error.value = null
  status.value = ''
  typeChanging.value = field
  typeChangePreview.value = null
  typeChangeKind.value = typeChangeKinds.value[0] ?? (USER_KINDS[0] as Kind)
  await nextTick()
  typeChangeEditor.value?.showModal()
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
  emit('update', result.schema)
  typeChangeEditor.value?.close()
  typeChanging.value = null
  typeChangePreview.value = null
  status.value = t('field.panel.saved')
}

function closeEditor() {
  if (busy.value) return
  editor.value?.close()
  editing.value = null
  creating.value = false
  error.value = null
}

async function openRelationEditor() {
  error.value = null
  status.value = ''
  if (!nav.loaded && !(await nav.refresh())) {
    error.value = nav.error ?? new Error(t('relation.noTables'))
    return
  }
  relationDraft.value = {
    name: '',
    reverseName: '',
    targetTableId:
      targetTables.value.find((table) => table.id === props.schema.table.id)
        ?.id ??
      targetTables.value[0]?.id ??
      '',
    kind: 'one_many',
    sourceSide: 'child',
    onDelete: 'set_null',
  }
  relationCreating.value = true
  await nextTick()
  relationEditor.value?.showModal()
}

function closeRelationEditor() {
  if (busy.value) return
  relationEditor.value?.close()
  relationCreating.value = false
  error.value = null
}

async function loadComputedTargetFields() {
  const relation = computedRelations.value.find(
    (item) => item.field.id === computedDraft.value.relationFieldId,
  )
  computedTargetFields.value = []
  computedDraft.value.targetFieldId = ''
  if (!relation) return
  computedController?.abort()
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
    computedDraft.value.targetFieldId = computedTargetFields.value[0]?.id ?? ''
  } catch (cause) {
    if (!controller.signal.aborted) error.value = cause
  } finally {
    if (computedController === controller) {
      computedController = undefined
      computedTargetLoading.value = false
    }
  }
}

async function openComputedEditor(kind: 'lookup' | 'rollup') {
  error.value = null
  status.value = ''
  computedCreating.value = true
  computedDraft.value = {
    name: '',
    kind,
    relationFieldId: computedRelations.value[0]?.field.id ?? '',
    targetFieldId: '',
    aggregate: kind === 'lookup' ? '' : 'count',
  }
  await loadComputedTargetFields()
  await nextTick()
  computedEditor.value?.showModal()
}

function closeComputedEditor() {
  if (busy.value) return
  computedController?.abort()
  computedEditor.value?.close()
  computedCreating.value = false
  computedTargetFields.value = []
  error.value = null
}

async function createComputedField() {
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
      uiType: computedDraft.value.kind,
      options,
    },
  )
  if (!result) return
  emit('update', result.schema)
  computedEditor.value?.close()
  computedCreating.value = false
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
  const result = await mutate<RelationMutation>(
    `/tables/${props.schema.table.id}/relations`,
    'POST',
    relationDraft.value,
  )
  if (!result) return
  emit('update', result.schema)
  relationEditor.value?.close()
  relationCreating.value = false
  status.value = t('field.panel.saved')
  void nav.refresh(true)
}

// After a version conflict, reseed the open editor from the latest metadata.
watch(
  () => props.schema,
  () => {
    if (!conflict.value) return
    conflict.value = false
    if (!editing.value) return
    const latest = stable(editing.value)
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
  editor.value?.close()
  editing.value = null
  creating.value = false
  status.value = t('field.panel.saved')
}

async function move(field: TableField, offset: number) {
  const index = userFields.value.findIndex((item) => item.id === field.id)
  if (index < 0) return
  const result = await mutate<FieldMutation>(
    `/fields/${field.id}`,
    'PATCH',
    { index: index + offset },
    field.version,
  )
  if (result) emit('update', result.schema)
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

function closeMenu(event: Event) {
  ;(event.currentTarget as HTMLElement | null)
    ?.closest('details')
    ?.removeAttribute('open')
}

function addChoice() {
  if (draft.value.choices.length < MAX_CHOICES)
    draft.value.choices.push(newChoice())
}

function removeChoice(index: number) {
  const [removed] = draft.value.choices.splice(index, 1)
  if (removed)
    draft.value.defaultChoices = draft.value.defaultChoices.filter(
      (id) => id !== removed.id,
    )
}

function toggleDefaultChoice(id: string, checked: boolean) {
  const others = draft.value.defaultChoices.filter((item) => item !== id)
  draft.value.defaultChoices = checked ? [...others, id] : others
}

function changeKind() {
  const kind = draft.value.kind
  const name = draft.value.name
  const description = draft.value.description
  draft.value = { ...draftFrom(null, env.value), kind, name, description }
  if (kind === 'float') draft.value.size = 'double'
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

onBeforeUnmount(() => {
  computedController?.abort()
  formulaController?.abort()
  if (formulaTimer) clearTimeout(formulaTimer)
  const own = controller
  controller = undefined
  own?.abort()
})
</script>

<template>
  <section class="field-panel" :aria-label="t('field.panel.title')">
    <div class="field-panel-head">
      <h2>{{ t('field.panel.title') }}</h2>
      <div v-if="owner" class="field-panel-actions">
        <button type="button" :disabled="busy" @click="openEditor(null)">
          {{ t('field.panel.add') }}
        </button>
        <button type="button" :disabled="busy" @click="openRelationEditor">
          {{ t('relation.add') }}
        </button>
        <button
          type="button"
          :disabled="busy || !computedRelations.length"
          @click="openComputedEditor('lookup')"
        >
          {{ t('field.panel.addLookup') }}
        </button>
        <button
          type="button"
          :disabled="busy || !computedRelations.length"
          @click="openComputedEditor('rollup')"
        >
          {{ t('field.panel.addRollup') }}
        </button>
        <button type="button" :disabled="busy" @click="openFormulaEditor()">
          {{ t('field.panel.addFormula') }}
        </button>
      </div>
    </div>
    <p v-if="!owner">{{ t('field.panel.readOnly') }}</p>
    <p class="field-status" role="status">{{ status }}</p>
    <p v-if="error && !dialogOpen" role="alert">{{ message }}</p>
    <ul class="field-list">
      <li
        v-for="field in fields"
        :key="field.id"
        :data-field-id="field.id"
        :data-system="field.isSystem ? 'true' : undefined"
      >
        <div class="field-line">
          <span class="field-name">{{ field.name }}</span>
          <span class="field-kind">{{
            field.uiType === 'link'
              ? t('field.kind.link')
              : isKind(field.uiType)
                ? fieldKindLabel(field.uiType)
                : field.uiType
          }}</span>
          <span v-if="field.isSystem" class="field-badge">
            {{ t('field.panel.system') }}
          </span>
          <span
            v-if="field.id === schema.table.displayFieldId"
            class="field-badge"
          >
            {{ t('field.panel.display') }}
          </span>
          <span v-if="field.isRequired" class="field-badge">
            {{ t('field.panel.required') }}
          </span>
          <span v-if="field.isUnique" class="field-badge">
            {{ t('field.panel.unique') }}
          </span>
          <details v-if="owner" class="node-actions field-actions">
            <summary>
              {{ t('field.panel.actions', { name: field.name }) }}
            </summary>
            <div class="field-menu">
              <button
                v-if="field.uiType === 'formula'"
                type="button"
                :disabled="busy"
                @click="
                  (event) => {
                    closeMenu(event)
                    openFormulaEditor(field)
                  }
                "
              >
                {{ t('field.panel.edit') }}
              </button>
              <button
                v-else-if="!['link', 'lookup', 'rollup'].includes(field.uiType)"
                type="button"
                :disabled="busy"
                @click="
                  (event) => {
                    closeMenu(event)
                    openEditor(field)
                  }
                "
              >
                {{ t('field.panel.edit') }}
              </button>
              <button
                v-if="canChangeType(field)"
                type="button"
                :disabled="busy"
                @click="
                  (event) => {
                    closeMenu(event)
                    openTypeChange(field)
                  }
                "
              >
                {{ t('field.panel.changeType') }}
              </button>
              <template v-if="!field.isSystem">
                <button
                  type="button"
                  :disabled="busy || userFields[0]?.id === field.id"
                  @click="
                    (event) => {
                      closeMenu(event)
                      move(field, -1)
                    }
                  "
                >
                  {{ t('field.panel.moveUp') }}
                </button>
                <button
                  type="button"
                  :disabled="busy || userFields.at(-1)?.id === field.id"
                  @click="
                    (event) => {
                      closeMenu(event)
                      move(field, 1)
                    }
                  "
                >
                  {{ t('field.panel.moveDown') }}
                </button>
                <button
                  v-if="canSetDisplay(field)"
                  type="button"
                  :disabled="busy"
                  @click="
                    (event) => {
                      closeMenu(event)
                      setDisplay(field)
                    }
                  "
                >
                  {{ t('field.panel.setDisplay') }}
                </button>
                <button
                  type="button"
                  :disabled="busy"
                  @click="
                    (event) => {
                      closeMenu(event)
                      askDelete(field)
                    }
                  "
                >
                  {{ t('field.panel.delete') }}
                </button>
              </template>
            </div>
          </details>
        </div>
        <p v-if="field.description" class="field-description">
          {{ field.description }}
        </p>
      </li>
    </ul>

    <dialog
      ref="editor"
      class="field-dialog"
      :aria-label="
        creating ? t('field.panel.createTitle') : t('field.panel.editTitle')
      "
      @cancel.prevent="closeEditor"
    >
      <form v-if="creating || editing" @submit.prevent="save">
        <h2>
          {{
            creating ? t('field.panel.createTitle') : t('field.panel.editTitle')
          }}
        </h2>
        <label>
          <span>{{ t('field.panel.name') }}</span>
          <input
            v-model="draft.name"
            autofocus
            maxlength="400"
            :disabled="busy"
          />
        </label>
        <label v-if="creating">
          <span>{{ t('field.panel.kind') }}</span>
          <select v-model="draft.kind" :disabled="busy" @change="changeKind">
            <option v-for="kind in USER_KINDS" :key="kind" :value="kind">
              {{ fieldKindLabel(kind) }}
            </option>
          </select>
        </label>
        <dl v-else class="field-kind-fixed">
          <dt>{{ t('field.panel.kind') }}</dt>
          <dd>{{ fieldKindLabel(draft.kind) }}</dd>
        </dl>
        <label>
          <span>{{ t('field.panel.description') }}</span>
          <textarea
            v-model="draft.description"
            rows="2"
            :maxlength="MAX_DESCRIPTION * 2"
            :disabled="busy"
          />
        </label>
        <p v-if="systemEdit">{{ t('field.panel.systemNotice') }}</p>
        <fieldset v-else class="field-settings">
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
            <label v-if="draft.kind === 'datetime'">
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
            >
              <input
                v-model="choice.label"
                maxlength="400"
                :aria-label="t('field.panel.choiceLabel', { n: index + 1 })"
                :disabled="busy"
              />
              <select
                v-model="choice.color"
                :aria-label="t('field.panel.choiceColor', { n: index + 1 })"
                :disabled="busy"
              >
                <option
                  v-for="color in CHOICE_COLORS"
                  :key="color"
                  :value="color"
                >
                  {{ colorLabel(color) }}
                </option>
              </select>
              <button
                type="button"
                :aria-label="t('field.panel.removeChoice', { n: index + 1 })"
                :disabled="busy"
                @click="removeChoice(index)"
              >
                {{ t('field.panel.remove') }}
              </button>
            </div>
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
          <template v-if="constraints.default">
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
          </template>
          <label v-if="showFill" class="field-check">
            <input v-model="draft.fillEmpty" type="checkbox" :disabled="busy" />
            <span>{{ t('field.panel.fillEmpty') }}</span>
          </label>
        </fieldset>
        <p v-if="error" role="alert">{{ message }}</p>
        <div class="nav-actions">
          <button type="submit" :disabled="busy">
            {{
              busy
                ? t('field.panel.saving')
                : creating
                  ? t('field.panel.create')
                  : t('field.panel.save')
            }}
          </button>
          <button type="button" :disabled="busy" @click="closeEditor">
            {{ t('field.panel.cancel') }}
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
      <form v-if="formulaCreating" @submit.prevent="saveFormula">
        <h2>{{ t('field.panel.formulaTitle') }}</h2>
        <label>
          <span>{{ t('field.panel.name') }}</span>
          <input
            v-model="formulaDraft.name"
            autofocus
            required
            maxlength="400"
            :disabled="busy"
          />
        </label>
        <label>
          <span>{{ t('field.panel.description') }}</span>
          <textarea
            v-model="formulaDraft.description"
            rows="2"
            :maxlength="MAX_DESCRIPTION * 2"
            :disabled="busy"
          />
        </label>
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
        <p>{{ t('field.panel.formulaHint') }}</p>
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
        <div class="nav-actions">
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
          <button type="button" :disabled="busy" @click="closeFormulaEditor">
            {{ t('field.panel.cancel') }}
          </button>
        </div>
      </form>
    </dialog>

    <dialog
      ref="computedEditor"
      class="field-dialog"
      :aria-label="t('field.panel.computedTitle')"
      @cancel.prevent="closeComputedEditor"
    >
      <form v-if="computedCreating" @submit.prevent="createComputedField">
        <h2>{{ t('field.panel.computedTitle') }}</h2>
        <label>
          <span>{{ t('field.panel.name') }}</span>
          <input
            v-model="computedDraft.name"
            autofocus
            required
            maxlength="400"
            :disabled="busy"
          />
        </label>
        <label>
          <span>{{ t('field.panel.computedRelation') }}</span>
          <select
            v-model="computedDraft.relationFieldId"
            required
            :disabled="busy || !computedRelations.length"
            @change="loadComputedTargetFields"
          >
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
            :disabled="
              busy || computedTargetLoading || !computedTargetFields.length
            "
            @change="
              computedDraft.aggregate = computedAggregates.includes(
                computedDraft.aggregate,
              )
                ? computedDraft.aggregate
                : (computedAggregates[0] ?? 'count')
            "
          >
            <option
              v-for="field in computedTargetFields"
              :key="field.id"
              :value="field.id"
            >
              {{ field.name }}
            </option>
          </select>
        </label>
        <label v-if="computedDraft.kind === 'rollup'">
          <span>{{ t('field.panel.computedAggregate') }}</span>
          <select v-model="computedDraft.aggregate" :disabled="busy">
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
          {{ t('field.panel.computedNoRelations') }}
        </p>
        <p
          v-else-if="
            !computedTargetLoading && !computedTargetFields.length && !error
          "
          role="alert"
        >
          {{ t('field.panel.computedNoFields') }}
        </p>
        <p v-if="computedTargetLoading" role="status">
          {{ t('field.panel.computedLoading') }}
        </p>
        <p v-if="error" role="alert">{{ message }}</p>
        <div class="nav-actions">
          <button type="submit" :disabled="computedEditorDisabled">
            {{ busy ? t('relation.saving') : t('relation.create') }}
          </button>
          <button type="button" :disabled="busy" @click="closeComputedEditor">
            {{ t('field.panel.cancel') }}
          </button>
        </div>
      </form>
    </dialog>

    <dialog
      ref="relationEditor"
      class="field-dialog"
      :aria-label="t('relation.createTitle')"
      @cancel.prevent="closeRelationEditor"
    >
      <form v-if="relationCreating" @submit.prevent="createRelation">
        <h2>{{ t('relation.createTitle') }}</h2>
        <label>
          <span>{{ t('relation.target') }}</span>
          <select
            v-model="relationDraft.targetTableId"
            required
            :disabled="busy || !targetTables.length"
          >
            <option
              v-for="table in targetTables"
              :key="table.id"
              :value="table.id"
            >
              {{ table.name }}
            </option>
          </select>
        </label>
        <label>
          <span>{{ t('relation.name') }}</span>
          <input
            v-model="relationDraft.name"
            required
            maxlength="100"
            :disabled="busy"
          />
        </label>
        <label>
          <span>{{ t('relation.reverseName') }}</span>
          <input
            v-model="relationDraft.reverseName"
            required
            maxlength="100"
            :disabled="busy"
          />
        </label>
        <label>
          <span>{{ t('relation.kind') }}</span>
          <select v-model="relationDraft.kind" :disabled="busy">
            <option value="one_one">{{ t('relation.oneOne') }}</option>
            <option value="one_many">{{ t('relation.oneMany') }}</option>
            <option value="many_many">{{ t('relation.manyMany') }}</option>
          </select>
        </label>
        <label v-if="relationDraft.kind === 'one_many'">
          <span>{{ t('relation.thisTableSide') }}</span>
          <select v-model="relationDraft.sourceSide" :disabled="busy">
            <option value="child">{{ t('relation.manySide') }}</option>
            <option value="parent">{{ t('relation.oneSide') }}</option>
          </select>
        </label>
        <label>
          <span>{{ t('relation.onDelete') }}</span>
          <select v-model="relationDraft.onDelete" :disabled="busy">
            <option value="set_null">{{ t('relation.setNull') }}</option>
            <option value="cascade">{{ t('relation.cascade') }}</option>
            <option value="restrict">{{ t('relation.restrict') }}</option>
          </select>
        </label>
        <p v-if="!targetTables.length" role="alert">
          {{ t('relation.noTables') }}
        </p>
        <p v-if="error" role="alert">{{ message }}</p>
        <div class="nav-actions">
          <button type="submit" :disabled="busy || !targetTables.length">
            {{ busy ? t('relation.saving') : t('relation.create') }}
          </button>
          <button type="button" :disabled="busy" @click="closeRelationEditor">
            {{ t('field.panel.cancel') }}
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
