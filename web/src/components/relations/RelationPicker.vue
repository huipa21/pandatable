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
import AppIcon from '../AppIcon.vue'

const PAGE_SIZE = 100
const SEARCH_DELAY = 250

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
const searchInput = ref<HTMLInputElement>()
const options = ref<RelationLink[]>([])
const search = ref('')
const offset = ref(0)
const total = ref(0)
const loading = ref(false)
const pendingId = ref<string | null>(null)
const error = ref<unknown>(null)
let target: { viewId: string; display: TableField | undefined } | undefined
let controller: AbortController | undefined
let generation = 0
let searchTimer: ReturnType<typeof setTimeout> | undefined

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
const linkedIds = computed(() => new Set(links.value.map((link) => link.id)))
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
const saving = computed(() => pendingId.value !== null)
const page = computed(() => Math.floor(offset.value / PAGE_SIZE) + 1)
const pageCount = computed(() =>
  Math.max(1, Math.ceil(total.value / PAGE_SIZE)),
)
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
    if (!target) {
      const targetSchema = await request<TableSchema>(
        `/tables/${encodeURIComponent(targetTableId.value)}/schema`,
        'GET',
        undefined,
        AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
      )
      if (!current(token, own.signal)) return
      const view = targetSchema.views.find((item) => item.type === 'grid')
      if (!view) throw new Error(t('relation.noRows'))
      target = {
        viewId: view.id,
        display: targetSchema.fields.find(
          (field) => field.id === targetSchema.table.displayFieldId,
        ),
      }
    }
    const { viewId, display } = target
    const term = search.value.trim()
    const result = await request<RowQueryResult>(
      `/views/${encodeURIComponent(viewId)}/query`,
      'POST',
      {
        offset: nextOffset,
        limit: PAGE_SIZE,
        includeCount: true,
        ...(display ? { fieldIds: [display.id] } : {}),
        ...(term
          ? { search: term, ...(display ? { searchField: display.id } : {}) }
          : {}),
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

function onSearchInput() {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    searchTimer = undefined
    void loadPage(0)
  }, SEARCH_DELAY)
}

function searchNow() {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = undefined
  void loadPage(0)
}

function applied(result: LinkMutation) {
  if (!Array.isArray(result.changes))
    throw new Error('Link mutation returned an unsupported response')
  if (props.identity === auth.user?.id)
    undo.record(linkUndoSteps(result.changes))
  emit('updated', result.row)
}

async function toggle(item: RelationLink) {
  if (saving.value) return
  if (!props.field || !props.rowId || !relation.value) {
    error.value = t('relation.invalid')
    return
  }
  const linked = linkedIds.value.has(item.id)
  const change = linked
    ? { add: [], remove: [item.id] }
    : {
        add: [item.id],
        remove: single.value
          ? links.value.map((link) => link.id).filter((id) => id !== item.id)
          : [],
      }
  pendingId.value = item.id
  error.value = null
  const path =
    `/tables/${encodeURIComponent(props.schema.table.id)}/rows/` +
    `${encodeURIComponent(props.rowId)}/links/${encodeURIComponent(props.field.id)}`
  try {
    applied(await request<LinkMutation>(path, 'POST', change))
  } catch (cause) {
    const relatedRowId =
      cause instanceof RequestError && cause.code === 'ERR_RELATION_CONFLICT'
        ? cause.details.relatedRowId
        : undefined
    if (!linked && relatedRowId) {
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
            target: item.label,
            existing: existingLabel,
          }),
        )
      ) {
        error.value = cause
        return
      }
      try {
        applied(
          await request<LinkMutation>(path, 'POST', {
            ...change,
            replace: true,
          }),
        )
      } catch (replacementError) {
        error.value = replacementError
      }
      return
    }
    error.value = cause
  } finally {
    pendingId.value = null
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
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = undefined
  dialog.value?.close()
  emit('close')
}

function onBackdrop(event: MouseEvent) {
  if (event.target === dialog.value) close()
}

watch(
  () => props.open,
  (open) => {
    if (!open) {
      controller?.abort()
      generation++
      if (searchTimer) clearTimeout(searchTimer)
      searchTimer = undefined
      dialog.value?.close()
      return
    }
    target = undefined
    search.value = ''
    options.value = []
    offset.value = 0
    total.value = 0
    error.value = null
    if (!dialog.value?.open) dialog.value?.showModal()
    searchInput.value?.focus()
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
  if (searchTimer) clearTimeout(searchTimer)
  dialog.value?.close()
})
</script>

<template>
  <dialog
    ref="dialog"
    class="relation-picker"
    :aria-label="dialogTitle"
    @cancel.prevent="close"
    @click="onBackdrop"
  >
    <header class="relation-search">
      <AppIcon name="search" :size="18" />
      <input
        ref="searchInput"
        v-model="search"
        type="search"
        :placeholder="t('relation.searchPlaceholder')"
        :aria-label="t('relation.searchPlaceholder')"
        @input="onSearchInput"
        @keydown.enter.prevent="searchNow"
      />
      <span class="relation-linked-count" role="status">
        <AppIcon name="link" :size="14" />
        {{ t('relation.linkedCount', { count: links.length }) }}
      </span>
    </header>
    <p v-if="message" class="relation-error" role="alert">{{ message }}</p>
    <ul
      v-if="options.length"
      class="relation-options"
      :aria-busy="loading"
      :aria-label="dialogTitle"
    >
      <li
        v-for="item in options"
        :key="item.id"
        :class="{ linked: linkedIds.has(item.id) }"
      >
        <span class="relation-option-label">{{ item.label }}</span>
        <button
          type="button"
          class="relation-toggle"
          :class="{ linked: linkedIds.has(item.id) }"
          :aria-pressed="linkedIds.has(item.id)"
          :aria-label="
            linkedIds.has(item.id)
              ? t('relation.unlinkRecord', { label: item.label })
              : t('relation.linkRecord', { label: item.label })
          "
          :title="
            linkedIds.has(item.id) ? t('relation.unlink') : t('relation.link')
          "
          :disabled="saving"
          @click="toggle(item)"
        >
          <AppIcon
            :name="linkedIds.has(item.id) ? 'checkMark' : 'plus'"
            :size="16"
          />
        </button>
      </li>
    </ul>
    <p v-else-if="loading" class="relation-empty" role="status">
      {{ t('relation.loading') }}
    </p>
    <p v-else-if="!message" class="relation-empty">
      {{ search.trim() ? t('relation.noMatches') : t('relation.noRows') }}
    </p>
    <footer class="relation-footer">
      <button type="button" :disabled="saving" @click="close">
        {{ t('relation.close') }}
      </button>
      <nav
        v-if="pageCount > 1"
        class="relation-paging"
        :aria-label="t('relation.paging')"
      >
        <button
          type="button"
          :disabled="loading || offset === 0"
          @click="loadPage(Math.max(0, offset - PAGE_SIZE))"
        >
          {{ t('relation.previous') }}
        </button>
        <span>{{ t('relation.page', { page, pages: pageCount }) }}</span>
        <button
          type="button"
          :disabled="loading || page >= pageCount"
          @click="loadPage(offset + PAGE_SIZE)"
        >
          {{ t('relation.next') }}
        </button>
      </nav>
      <span class="relation-total">
        {{ t('relation.recordCount', { count: total }) }}
      </span>
    </footer>
  </dialog>
</template>

<style scoped>
.relation-picker {
  display: none;
  width: min(560px, calc(100vw - 32px));
  max-height: min(80vh, 640px);
  padding: 0;
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: 10px;
  background: var(--panel-surface);
  color: var(--text);
  box-shadow: 0 12px 32px rgb(0 0 0 / 18%);
}
.relation-picker[open] {
  display: flex;
  flex-direction: column;
}
.relation-search {
  display: flex;
  gap: 10px;
  align-items: center;
  padding: 10px 12px;
  border-bottom: 1px solid var(--border);
  background: var(--surface);
}
.relation-search input {
  flex: 1;
  min-width: 0;
  border: 0;
  outline: none;
  background: transparent;
  color: inherit;
  font: inherit;
}
.relation-linked-count {
  display: inline-flex;
  gap: 6px;
  align-items: center;
  padding: 4px 8px;
  border-radius: 6px;
  background: var(--border);
  white-space: nowrap;
  font-size: 0.875rem;
}
.relation-options {
  flex: 1;
  min-height: 0;
  margin: 0;
  padding: 0;
  overflow: auto;
  list-style: none;
}
.relation-options[aria-busy='true'] {
  opacity: 0.6;
}
.relation-options li {
  display: flex;
  gap: 12px;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px 8px 16px;
  border-bottom: 1px solid var(--border);
}
.relation-option-label {
  overflow-wrap: anywhere;
  color: var(--accent);
  font-weight: 600;
}
.relation-toggle {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border: 0;
  border-radius: 6px;
  background: var(--highlight-green-2-bg);
  color: var(--highlight-green-2-text);
  cursor: pointer;
}
.relation-toggle:hover:not(:disabled) {
  filter: brightness(0.95);
}
.relation-toggle.linked {
  background: var(--accent);
  color: var(--panel-surface);
}
.relation-toggle:disabled {
  cursor: default;
  opacity: 0.6;
}
.relation-empty,
.relation-error {
  margin: 0;
  padding: 16px;
}
.relation-error {
  color: var(--highlight-red-2-text);
}
.relation-footer {
  display: flex;
  gap: 8px;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  border-top: 1px solid var(--border);
  background: var(--surface);
}
.relation-footer > button,
.relation-paging button {
  padding: 4px 10px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--panel-surface);
  color: var(--text);
}
.relation-paging {
  display: flex;
  gap: 8px;
  align-items: center;
}
.relation-total {
  color: var(--muted);
  font-size: 0.875rem;
}
</style>
