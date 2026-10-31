<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { errorMessage, request, RequestError } from '../../api/client'
import type {
  DataRow,
  LinkMutation,
  RelationLink,
  RowQueryResult,
  TableField,
  TableSchema,
} from '../../api/types'
import { bindField } from '../../fields/registry'
import { isWireValue, type FieldEnv } from '../../fields/types'
import { useAuthStore } from '../../stores/auth'
import { linkUndoSteps, useUndoStore } from '../../stores/undo'

const props = defineProps<{
  open: boolean
  schema: TableSchema
  field: TableField | null
  rowId: string | null
  links: unknown
  identity: string
}>()
const emit = defineEmits<{
  close: []
  updated: [row: DataRow]
}>()
const { t, locale } = useI18n()
const auth = useAuthStore()
const undo = useUndoStore()
const dialog = ref<HTMLDialogElement>()
const options = ref<RelationLink[]>([])
const selected = ref<string[]>([])
const offset = ref(0)
const total = ref(0)
const loading = ref(false)
const saving = ref(false)
const error = ref<unknown>(null)
let controller: AbortController | undefined
let generation = 0

const env = computed<FieldEnv>(() => ({
  timeZone: auth.user?.effectiveTimezone,
  locale: locale.value,
}))
const links = computed<RelationLink[]>(() =>
  Array.isArray(props.links)
    ? props.links.filter(
        (value): value is RelationLink =>
          typeof value === 'object' &&
          value !== null &&
          'id' in value &&
          typeof value.id === 'string' &&
          'label' in value &&
          typeof value.label === 'string',
      )
    : [],
)
const relation = computed(() => {
  const relationId = props.field?.options.relationId
  return typeof relationId === 'string'
    ? props.schema.relations.find((item) => item.id === relationId)
    : undefined
})
const targetTableId = computed(() => {
  const id = props.field?.options.targetTableId
  return typeof id === 'string' ? id : ''
})
const side = computed(() =>
  props.field?.options.side === 'child' ? 'child' : 'parent',
)
const single = computed(
  () =>
    relation.value?.kind === 'one_one' ||
    (relation.value?.kind === 'one_many' && side.value === 'child'),
)
const page = computed(() => Math.floor(offset.value / 100) + 1)
const pageCount = computed(() => Math.max(1, Math.ceil(total.value / 100)))
const dialogTitle = computed(() =>
  props.field ? t('relation.pickerTitle', { field: props.field.name }) : '',
)
const message = computed(() =>
  typeof error.value === 'string'
    ? error.value
    : error.value
      ? errorMessage(error.value)
      : '',
)

function current(token: number, signal: AbortSignal) {
  return (
    token === generation && !signal.aborted && props.identity === auth.user?.id
  )
}

function selectedOnPage(id: string) {
  return selected.value.includes(id)
}

function toggle(id: string, checked: boolean) {
  const next = selected.value.filter((value) => value !== id)
  selected.value = checked ? (single.value ? [id] : [...next, id]) : next
}

async function relatedRowLabel(id: string) {
  const row = await request<DataRow>(
    `/tables/${encodeURIComponent(props.schema.table.id)}/rows/${encodeURIComponent(id)}`,
    'GET',
    undefined,
    AbortSignal.timeout(10_000),
  )
  const display = props.schema.fields.find(
    (field) => field.id === props.schema.table.displayFieldId,
  )
  const bound = display ? bindField(display.uiType, display.options) : null
  const value = display ? row.fields[display.id] : undefined
  return bound?.ok && isWireValue(value)
    ? bound.value.formatDisplay(value, env.value).trim() ||
        t('relation.untitled', { id })
    : t('relation.untitled', { id })
}

async function loadPage(nextOffset = 0) {
  if (!props.field || !props.rowId || !targetTableId.value) {
    error.value = t('relation.invalid')
    return
  }
  if (!relation.value) {
    error.value = t('relation.invalid')
    return
  }
  controller?.abort()
  const own = new AbortController()
  controller = own
  const token = ++generation
  loading.value = true
  error.value = null
  try {
    const targetSchema = await request<TableSchema>(
      `/tables/${encodeURIComponent(targetTableId.value)}/schema`,
      'GET',
      undefined,
      AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
    )
    if (!current(token, own.signal)) return
    const view = targetSchema.views.find((item) => item.type === 'grid')
    if (!view) throw new Error(t('relation.noRows'))
    const display = targetSchema.fields.find(
      (field) => field.id === targetSchema.table.displayFieldId,
    )
    const result = await request<RowQueryResult>(
      `/views/${encodeURIComponent(view.id)}/query`,
      'POST',
      {
        offset: nextOffset,
        limit: 100,
        includeCount: true,
        ...(display ? { fieldIds: [display.id] } : {}),
      },
      AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
    )
    if (!current(token, own.signal)) return
    const bound = display ? bindField(display.uiType, display.options) : null
    options.value = result.rows.map((row) => {
      const value = display ? row.fields[display.id] : undefined
      const label =
        bound?.ok && isWireValue(value)
          ? bound.value.formatDisplay(value, env.value).trim()
          : ''
      return {
        id: row.id,
        label: label || t('relation.untitled', { id: row.id }),
      }
    })
    offset.value = nextOffset
    total.value = result.total ?? 0
  } catch (cause) {
    if (current(token, own.signal)) error.value = cause
  } finally {
    if (current(token, own.signal)) loading.value = false
    if (controller === own) controller = undefined
  }
}

async function save() {
  if (saving.value) return
  if (!props.field || !props.rowId || !relation.value) {
    error.value = t('relation.invalid')
    return
  }
  const currentIDs = links.value.map((link) => link.id)
  const change = {
    add: selected.value.filter((id) => !currentIDs.includes(id)),
    remove: currentIDs.filter((id) => !selected.value.includes(id)),
  }
  if (!change.add.length && !change.remove.length) {
    close()
    return
  }
  saving.value = true
  error.value = null
  const path =
    `/tables/${encodeURIComponent(props.schema.table.id)}/rows/` +
    `${encodeURIComponent(props.rowId)}/links/${encodeURIComponent(props.field.id)}`
  try {
    const result = await request<LinkMutation>(path, 'POST', change)
    if (!Array.isArray(result.changes))
      throw new Error('Link mutation returned an unsupported response')
    if (props.identity === auth.user?.id)
      undo.record(linkUndoSteps(result.changes))
    emit('updated', result.row)
    finishClose()
  } catch (cause) {
    if (
      cause instanceof RequestError &&
      cause.code === 'ERR_RELATION_CONFLICT'
    ) {
      const selectedItem = options.value.find((item) =>
        change.add.includes(item.id),
      )
      const relatedRowId = cause.details.relatedRowId
      if (selectedItem && relatedRowId) {
        let existingLabel: string
        try {
          existingLabel = await relatedRowLabel(relatedRowId)
        } catch (lookupError) {
          error.value = lookupError
          return
        }
        if (
          !window.confirm(
            t('relation.replaceConfirm', {
              target: selectedItem.label,
              existing: existingLabel,
            }),
          )
        ) {
          error.value = cause
          return
        }
        try {
          const result = await request<LinkMutation>(path, 'POST', {
            ...change,
            replace: true,
          })
          if (!Array.isArray(result.changes))
            throw new Error('Link mutation returned an unsupported response', {
              cause,
            })
          if (props.identity === auth.user?.id)
            undo.record(linkUndoSteps(result.changes))
          emit('updated', result.row)
          finishClose()
          return
        } catch (replacementError) {
          error.value = replacementError
          return
        }
      }
    }
    error.value = cause
  } finally {
    saving.value = false
  }
}

function close() {
  if (saving.value) return
  finishClose()
}

function finishClose() {
  generation++
  controller?.abort()
  controller = undefined
  dialog.value?.close()
  emit('close')
}

watch(
  () => props.open,
  (open) => {
    if (!open) {
      controller?.abort()
      generation++
      dialog.value?.close()
      return
    }
    selected.value = links.value.map((link) => link.id)
    options.value = []
    offset.value = 0
    total.value = 0
    if (!dialog.value?.open) dialog.value?.showModal()
    void loadPage()
  },
)
watch(
  () => props.identity,
  () => {
    if (props.open) close()
  },
)
onBeforeUnmount(() => {
  controller?.abort()
  dialog.value?.close()
})
</script>

<template>
  <dialog
    ref="dialog"
    class="relation-picker"
    :aria-label="dialogTitle"
    @cancel.prevent="close"
  >
    <h2>{{ dialogTitle }}</h2>
    <p v-if="loading" role="status">{{ t('relation.loading') }}</p>
    <p v-if="message" role="alert">{{ message }}</p>
    <ul v-if="!loading && options.length" class="relation-options">
      <li v-for="item in options" :key="item.id">
        <label>
          <input
            type="checkbox"
            :checked="selectedOnPage(item.id)"
            :disabled="saving"
            @change="
              toggle(item.id, ($event.target as HTMLInputElement).checked)
            "
          />
          <span>{{ item.label }}</span>
        </label>
      </li>
    </ul>
    <p v-else-if="!loading && !message">{{ t('relation.noRows') }}</p>
    <nav
      v-if="pageCount > 1"
      class="relation-paging"
      :aria-label="t('relation.paging')"
    >
      <button
        type="button"
        :disabled="loading || saving || offset === 0"
        @click="loadPage(Math.max(0, offset - 100))"
      >
        {{ t('relation.previous') }}
      </button>
      <span role="status">{{
        t('relation.page', { page, pages: pageCount })
      }}</span>
      <button
        type="button"
        :disabled="loading || saving || page >= pageCount"
        @click="loadPage(offset + 100)"
      >
        {{ t('relation.next') }}
      </button>
    </nav>
    <div class="relation-actions">
      <button type="button" :disabled="saving" @click="close">
        {{ t('relation.cancel') }}
      </button>
      <button
        type="button"
        :disabled="saving || loading || !!message"
        @click="save"
      >
        {{ saving ? t('relation.saving') : t('relation.save') }}
      </button>
    </div>
  </dialog>
</template>

<style scoped>
.relation-picker {
  width: min(460px, calc(100vw - 32px));
  max-height: min(80vh, 640px);
  overflow: auto;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface);
  color: var(--text);
}
.relation-options {
  max-height: 48vh;
  margin: 0;
  padding: 0;
  overflow: auto;
  list-style: none;
}
.relation-options label {
  display: flex;
  gap: 8px;
  align-items: center;
  padding: 8px;
}
.relation-options label span {
  overflow-wrap: anywhere;
}
.relation-actions,
.relation-paging {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  align-items: center;
  margin-top: 12px;
}
.relation-picker button {
  padding: 6px 10px;
  border: 1px solid var(--border);
  border-radius: 4px;
  background: var(--surface);
  color: var(--text);
}
</style>
