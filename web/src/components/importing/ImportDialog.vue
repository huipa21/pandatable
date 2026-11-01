<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  analyzeImport,
  errorMessage,
  request,
  RequestError,
} from '../../api/client'
import type {
  ImportAnalysis,
  ImportColumn,
  ImportJob,
  ImportMapping,
  ImportResult,
  TableField,
  TableSchema,
} from '../../api/types'
import { fieldKindLabel } from '../../fields/messages'
import { isKind } from '../../fields/registry'
import { USER_KINDS } from '../fields/fieldForm'

const props = defineProps<{
  targetKind: 'book' | 'table'
  targetId: string
  schema?: TableSchema | null
}>()
const emit = defineEmits<{
  close: []
  completed: [result: ImportResult]
  openTable: [result: ImportResult]
}>()
const { t } = useI18n()
const dialog = ref<HTMLDialogElement>()
const selected = ref<File | null>(null)
const analysis = ref<ImportAnalysis | null>(null)
const mappings = ref<ImportMapping[]>([])
const choiceText = ref<Record<number, string>>({})
const name = ref('')
const hasHeader = ref(true)
const jobId = ref('')
const job = ref<ImportJob | null>(null)
const busy = ref(false)
const polling = ref(false)
const error = ref<unknown>(null)
let controller: AbortController | undefined
let timer: ReturnType<typeof setTimeout> | undefined
let disposed = false
let completed = false

const packageFile = computed(() => analysis.value?.format === 'package')
const active = computed(() => job.value?.status === 'running')
const kinds = USER_KINDS.filter(
  (kind) => kind !== 'autonumber' && kind !== 'attachment',
)
const formatLabel = computed(() => {
  switch (analysis.value?.format) {
    case 'csv':
      return t('import.formatCsv')
    case 'xlsx':
      return t('import.formatXlsx')
    case 'package':
      return t('import.formatPackage')
    default:
      return ''
  }
})
const invalidMapping = computed(() => {
  if (!analysis.value) return true
  if (props.targetKind === 'book') {
    if (!name.value.trim()) return true
    if (packageFile.value) return false
    const names = mappings.value
      .filter((mapping) => !mapping.skip)
      .map((mapping) => mapping.name.trim().toLocaleLowerCase())
    return (
      names.length === 0 ||
      names.some((value) => !value) ||
      new Set(names).size !== names.length
    )
  }
  const targets = mappings.value
    .map((mapping) => mapping.targetFieldId)
    .filter(Boolean)
  return targets.length === 0 || new Set(targets).size !== targets.length
})

onMounted(async () => {
  await nextTick()
  if (!disposed) dialog.value?.showModal()
})
onBeforeUnmount(() => {
  disposed = true
  controller?.abort()
  if (timer) clearTimeout(timer)
})

function chooseFile(event: Event) {
  const input = event.target as HTMLInputElement
  selected.value = input.files?.[0] ?? null
}

function label(kind: string) {
  return isKind(kind) ? fieldKindLabel(kind) : t('field.error.unknown_kind')
}

function relationTarget(field: TableField): string | undefined {
  const table = props.schema?.table.id
  const relation = props.schema?.relations.find(
    (item) => item.parentFieldId === field.id || item.childFieldId === field.id,
  )
  if (!relation) return undefined
  return relation.parentTableId === table
    ? relation.childTableId
    : relation.parentTableId
}

function writableFields(column: ImportColumn): TableField[] {
  return (props.schema?.fields ?? []).filter((field) => {
    if (
      field.isSystem ||
      ['formula', 'lookup', 'rollup', 'autonumber'].includes(field.uiType)
    )
      return false
    if (field.uiType !== 'link') return true
    return (
      packageFile.value &&
      column.uiType === 'link' &&
      !!column.relinkTargetId &&
      relationTarget(field) === column.relinkTargetId
    )
  })
}

function choices(options: Record<string, unknown> | null): string {
  if (!Array.isArray(options?.choices)) return ''
  return options.choices
    .map((choice: unknown) =>
      typeof choice === 'object' &&
      choice !== null &&
      'label' in choice &&
      typeof choice.label === 'string'
        ? choice.label
        : '',
    )
    .filter(Boolean)
    .join('\n')
}

async function analyze() {
  if (!selected.value || busy.value) return
  error.value = null
  busy.value = true
  const own = new AbortController()
  controller = own
  try {
    const result = await analyzeImport(
      props.targetKind,
      props.targetId,
      selected.value,
      AbortSignal.any([own.signal, AbortSignal.timeout(600_000)]),
    )
    if (disposed || own.signal.aborted) return
    jobId.value = result.id
    analysis.value = result.analysis
    hasHeader.value = result.analysis.hasHeader
    name.value =
      result.analysis.tableName ?? selected.value.name.replace(/\.[^.]+$/, '')
    mappings.value = result.analysis.columns.map((column) => {
      const target = writableFields(column).find(
        (field) => field.id === column.targetFieldId,
      )
      choiceText.value[column.index] = choices(column.options)
      return {
        index: column.index,
        name: column.name,
        uiType: column.uiType,
        options: column.options ?? {},
        targetFieldId: target?.id ?? '',
        skip: props.targetKind === 'table' && !target,
        relink: target?.uiType === 'link',
      }
    })
  } catch (cause) {
    if (!disposed && !own.signal.aborted) error.value = cause
  } finally {
    if (controller === own) controller = undefined
    if (!disposed) busy.value = false
  }
}

function changeType(mapping: ImportMapping, column: ImportColumn) {
  if (mapping.uiType === column.uiType) {
    mapping.options = column.options ?? {}
    choiceText.value[column.index] = choices(column.options)
  } else if (mapping.uiType === 'integer') {
    mapping.options = { size: 'big' }
  } else if (mapping.uiType === 'decimal') {
    mapping.options = { precision: 38, scale: 6 }
  } else if (
    mapping.uiType === 'single_select' ||
    mapping.uiType === 'multi_select'
  ) {
    choiceText.value[column.index] = column.examples?.join('\n') ?? ''
    updateChoices(mapping)
  } else {
    mapping.options = {}
  }
}

function updateChoices(mapping: ImportMapping) {
  const labels = (choiceText.value[mapping.index] ?? '')
    .split(/\r?\n/)
    .map((value) => value.trim())
    .filter(Boolean)
  mapping.options = {
    choices: labels.map((label, index) => ({
      id: `c${index + 1}`,
      label,
      color: 'gray',
    })),
  }
}

function changeTarget(mapping: ImportMapping, column: ImportColumn) {
  const field = writableFields(column).find(
    (item) => item.id === mapping.targetFieldId,
  )
  mapping.skip = !field
  mapping.relink = field?.uiType === 'link'
}

function trackStartedImport() {
  job.value = {
    id: jobId.value,
    status: 'running',
    result: { rows: 0, invalidCells: 0 },
    hasReport: false,
  }
  void poll()
}

async function start() {
  if (!analysis.value || invalidMapping.value || busy.value) return
  error.value = null
  busy.value = true
  const own = new AbortController()
  controller = own
  try {
    await request(
      `/imports/${encodeURIComponent(jobId.value)}`,
      'POST',
      {
        name: name.value.trim(),
        hasHeader: hasHeader.value,
        columns: mappings.value,
      },
      AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
    )
    if (disposed || own.signal.aborted) return
    trackStartedImport()
  } catch (cause) {
    if (!disposed && !own.signal.aborted) {
      if (cause instanceof RequestError && cause.code !== 'ERR_IMPORT_STATE') {
        error.value = cause
      } else {
        trackStartedImport()
      }
    }
  } finally {
    if (controller === own) controller = undefined
    if (!disposed) busy.value = false
  }
}

async function poll() {
  if (!jobId.value || polling.value || disposed) return
  error.value = null
  polling.value = true
  const own = new AbortController()
  controller = own
  try {
    const result = await request<ImportJob>(
      `/imports/${encodeURIComponent(jobId.value)}`,
      'GET',
      undefined,
      AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
    )
    if (disposed || own.signal.aborted) return
    if (result.status === 'ready') {
      job.value = null
      error.value = new RequestError('ERR_IMPORT_NOT_STARTED', 409)
    } else if (result.status === 'running') {
      job.value = result
      timer = setTimeout(() => void poll(), 800)
    } else {
      job.value = result
      if (!completed) {
        completed = true
        emit('completed', result.result)
      }
    }
  } catch (cause) {
    if (!disposed && !own.signal.aborted) error.value = cause
  } finally {
    if (controller === own) controller = undefined
    if (!disposed) polling.value = false
  }
}

async function close() {
  if (busy.value || active.value) return
  if (jobId.value) {
    busy.value = true
    try {
      await request(`/imports/${encodeURIComponent(jobId.value)}`, 'DELETE')
    } catch (cause) {
      if (!(cause instanceof RequestError && cause.code === 'ERR_NOT_FOUND')) {
        error.value = cause
        busy.value = false
        return
      }
    }
    busy.value = false
  }
  dialog.value?.close()
  emit('close')
}
</script>

<template>
  <dialog
    ref="dialog"
    class="import-dialog"
    :aria-label="t('import.title')"
    @cancel.prevent="close"
  >
    <h2>{{ t('import.title') }}</h2>
    <p>
      {{ targetKind === 'book' ? t('import.newTable') : t('import.append') }}
    </p>
    <template v-if="!analysis">
      <label class="import-field">
        <span>{{ t('import.file') }}</span>
        <input
          type="file"
          accept=".csv,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
          :disabled="busy"
          @change="chooseFile"
        />
      </label>
      <p>{{ t('import.limits') }}</p>
      <button type="button" :disabled="!selected || busy" @click="analyze">
        {{ t('import.analyze') }}
      </button>
      <p v-if="busy" role="status">{{ t('import.analyzing') }}</p>
    </template>
    <template v-else-if="!job">
      <p>
        {{
          t('import.analyzed', {
            format: formatLabel,
            count: analysis.sampledRows,
          })
        }}
      </p>
      <p v-if="analysis.encoding">
        {{ t('import.encoding', { encoding: analysis.encoding }) }}
      </p>
      <p v-if="packageFile">{{ t('import.packageNotice') }}</p>
      <label v-if="targetKind === 'book'" class="import-field">
        <span>{{ t('import.tableName') }}</span>
        <input v-model="name" maxlength="100" />
      </label>
      <label class="import-field">
        <input v-model="hasHeader" type="checkbox" :disabled="packageFile" />
        {{ t('import.header') }}
      </label>
      <h3>{{ t('import.mapping') }}</h3>
      <div class="import-columns">
        <section
          v-for="(column, position) in analysis.columns"
          :key="column.index"
          class="import-column"
        >
          <strong>{{ column.name }}</strong>
          <span>{{ label(column.uiType) }}</span>
          <p v-if="column.examples?.length" class="import-examples">
            {{ t('import.examples', { values: column.examples.join(' · ') }) }}
          </p>
          <template v-if="targetKind === 'table'">
            <label class="import-field">
              <span>{{ t('import.targetField', { name: column.name }) }}</span>
              <select
                v-model="mappings[position]!.targetFieldId"
                @change="changeTarget(mappings[position]!, column)"
              >
                <option value="">{{ t('import.skip') }}</option>
                <option
                  v-for="field in writableFields(column)"
                  :key="field.id"
                  :value="field.id"
                >
                  {{
                    t('import.fieldOption', {
                      name: field.name,
                      type: label(field.uiType),
                    })
                  }}
                </option>
              </select>
            </label>
          </template>
          <template v-else-if="!packageFile">
            <label class="import-field">
              <input v-model="mappings[position]!.skip" type="checkbox" />
              {{ t('import.skip') }}
            </label>
            <template v-if="!mappings[position]!.skip">
              <label class="import-field">
                <span>{{ t('import.columnName', { name: column.name }) }}</span>
                <input v-model="mappings[position]!.name" maxlength="100" />
              </label>
              <label class="import-field">
                <span>{{ t('import.columnType', { name: column.name }) }}</span>
                <select
                  v-model="mappings[position]!.uiType"
                  @change="changeType(mappings[position]!, column)"
                >
                  <option v-for="kind in kinds" :key="kind" :value="kind">
                    {{ label(kind) }}
                  </option>
                </select>
              </label>
              <label
                v-if="
                  ['single_select', 'multi_select'].includes(
                    mappings[position]!.uiType,
                  )
                "
                class="import-field"
              >
                <span>{{ t('import.choiceLabels') }}</span>
                <textarea
                  v-model="choiceText[column.index]"
                  rows="3"
                  @input="updateChoices(mappings[position]!)"
                />
              </label>
            </template>
          </template>
          <template v-else-if="column.uiType === 'link'">
            <label v-if="column.relinkTargetId" class="import-field">
              <input v-model="mappings[position]!.relink" type="checkbox" />
              {{ t('import.relink', { table: column.linkTarget ?? '' }) }}
            </label>
            <p v-else>
              {{ t('import.linkAsText', { table: column.linkTarget ?? '' }) }}
            </p>
          </template>
        </section>
      </div>
      <p v-if="invalidMapping">{{ t('import.mappingRequired') }}</p>
      <button type="button" :disabled="invalidMapping || busy" @click="start">
        {{ t('import.start') }}
      </button>
    </template>
    <template v-else>
      <p v-if="active" role="status">
        {{ t('import.progress', { count: job.result.rows }) }}
      </p>
      <template v-else>
        <p v-if="job.status === 'done'" role="status">
          {{ t('import.done', { count: job.result.rows }) }}
        </p>
        <p v-else role="alert">
          {{ t('import.failed', { count: job.result.rows }) }}
          {{
            errorMessage(new RequestError(job.errorCode ?? 'ERR_INTERNAL', 400))
          }}
        </p>
        <p>
          {{ t('import.invalidCells', { count: job.result.invalidCells }) }}
        </p>
        <p v-if="job.result.convertedLinks?.length">
          {{
            t('import.convertedLinks', {
              fields: job.result.convertedLinks.join(', '),
            })
          }}
        </p>
        <a
          v-if="job.hasReport"
          :href="`/api/v1/imports/${encodeURIComponent(jobId)}/issues`"
          download
        >
          {{ t('import.downloadReport') }}
        </a>
        <button
          v-if="
            targetKind === 'book' && job.status === 'done' && job.result.tableId
          "
          type="button"
          @click="emit('openTable', job.result)"
        >
          {{ t('import.openTable') }}
        </button>
      </template>
      <button
        v-if="error && active"
        type="button"
        :disabled="polling"
        @click="poll"
      >
        {{ t('import.retryStatus') }}
      </button>
    </template>
    <p v-if="error" role="alert">{{ errorMessage(error) }}</p>
    <div class="import-actions">
      <button type="button" :disabled="busy || active" @click="close">
        {{ t('import.close') }}
      </button>
    </div>
  </dialog>
</template>

<style scoped>
.import-dialog {
  width: min(840px, calc(100vw - 24px));
  max-height: calc(100dvh - 24px);
  overflow: auto;
  color: var(--text);
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 12px;
  padding: 20px;
}

.import-columns {
  display: grid;
  gap: 12px;
}

.import-column {
  display: grid;
  gap: 6px;
  padding: 12px;
  border: 1px solid var(--border);
  border-radius: 6px;
  overflow-wrap: anywhere;
}

.import-field {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.import-field input:not([type='checkbox']),
.import-field select,
.import-field textarea {
  max-width: 100%;
}

.import-examples {
  margin: 0;
  overflow-wrap: anywhere;
}

.import-actions {
  display: flex;
  gap: 8px;
  margin-top: 16px;
}
</style>
