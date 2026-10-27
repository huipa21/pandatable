<script setup lang="ts">
import { computed, markRaw, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import RevoGrid, { VGridVueEditor } from '@revolist/vue3-datagrid'
import type {
  BeforeSaveDataDetails,
  BeforeRangeSaveDataDetails,
  ColumnRegular,
  Components,
  GroupingOptions,
  RangeArea,
  FocusAfterRenderEvent,
  ViewPortScrollEvent,
} from '@revolist/revogrid'
import { usePreferencesStore } from '../../stores/preferences'
import type { CellPatch, EditableField, WindowSource } from './model'
import { GridInputError, RowWindow, placeholder } from './window'
import { formatTsv, parseTsv, TsvError } from './tsv'
import type { WindowStats } from './window'
import NoteEditor from './NoteEditor.vue'

const props = defineProps<{ provider: WindowSource; grouped?: boolean }>()
const emit = defineEmits<{ stats: [value: WindowStats] }>()
const { t } = useI18n()
const preferences = usePreferencesStore()
const host = ref<{ $el: HTMLElement & Components.RevoGrid }>()
const error = ref<'load' | 'edit' | 'paste' | 'copy' | null>(null)
const errorMessage = computed(() =>
  error.value === 'load'
    ? t('spike.loadError')
    : error.value === 'copy'
      ? t('spike.copyError')
      : error.value === 'paste'
        ? t('spike.pasteError')
        : t('spike.editError'),
)
let ready = false
let disposed = false
let focused: FocusAfterRenderEvent | undefined
function focusChanged(event: CustomEvent<FocusAfterRenderEvent>) {
  focused = event.detail
}
const rows = new RowWindow(
  props.provider,
  () => {
    const stats = rows.snapshot()
    emit('stats', stats)
    if (!ready || disposed) return
    if (props.grouped) {
      if (
        !stats.pending &&
        stats.cachedRows === props.provider.total &&
        host.value
      ) {
        host.value.$el.source = rows.slots.slice()
      }
    } else {
      void refresh()
    }
  },
  (cause) => {
    console.error('Spike window request failed', cause)
    error.value = 'load'
  },
)
const source = markRaw(rows.slots)
const editors = { note: VGridVueEditor(NoteEditor) }
const grouping = computed<GroupingOptions | undefined>(() =>
  props.grouped
    ? {
        props: ['group'],
        expandedAll: true,
        groupLabelTemplate: (h, { name, expanded, colType }) =>
          colType !== 'colPinStart'
            ? null
            : h(
                'button',
                {
                  type: 'button',
                  'aria-expanded': String(expanded),
                  'aria-label': expanded
                    ? t('spike.collapse', { name })
                    : t('spike.expand', { name }),
                  class: 'spike-group-toggle',
                },
                name,
              ),
      }
    : undefined,
)
const columns = computed<ColumnRegular[]>(() => [
  {
    prop: 'id',
    name: t('spike.fields.id'),
    pin: 'colPinStart',
    size: 90,
    readonly: true,
  },
  {
    prop: 'name',
    name: t('spike.fields.name'),
    size: 230,
    readonly: ({ model }) => !model.id,
  },
  {
    prop: 'note',
    name: t('spike.fields.note'),
    size: 340,
    editor: 'note',
    readonly: ({ model }) => !model.id,
  },
  {
    prop: 'quantity',
    name: t('spike.fields.quantity'),
    size: 180,
    readonly: ({ model }) => !model.id,
    cellTemplate: (h, { model }) =>
      h(
        'span',
        { class: 'spike-quantity' },
        model.id ? String(model.quantity) : t('spike.loading'),
      ),
  },
  { prop: 'group', name: t('spike.fields.group'), size: 260, readonly: true },
])

async function refresh() {
  try {
    const store = await host.value?.$el.getSourceStore('rgRow')
    if (store && !disposed) store.set('source', rows.slots.slice())
  } catch (cause) {
    if (!disposed) {
      console.error('Spike grid refresh failed', cause)
      error.value = 'load'
    }
  }
}

function created() {
  ready = true
  rows.load(0)
}

function scroll(event: CustomEvent<ViewPortScrollEvent>) {
  if (event.detail.dimension === 'rgRow' && !props.grouped) {
    const index = Math.min(
      props.provider.total - 1,
      Math.max(0, Math.floor(event.detail.coordinate / 32)),
    )
    rows.load(index)
  }
}

function focusGrid(event: PointerEvent) {
  if (
    event.target instanceof Element &&
    !event.target.closest('input, textarea, button')
  ) {
    host.value?.$el.focus({ preventScroll: true })
  }
}

function patches(data: Record<string, unknown>, index: number): CellPatch[] {
  return Object.entries(data).flatMap(([field, value]) => {
    if (!['name', 'note', 'quantity'].includes(field)) return []
    const parsed = field === 'quantity' ? Number(value) : String(value ?? '')
    return [{ index, field: field as EditableField, value: parsed }]
  })
}

function apply(changes: CellPatch[]) {
  try {
    rows.apply(changes)
    error.value = null
  } catch (cause) {
    if (!(cause instanceof GridInputError)) throw cause
    console.warn('Spike edit rejected', cause.message)
    error.value = 'edit'
  }
}

function edit(event: CustomEvent<BeforeSaveDataDetails>) {
  event.preventDefault()
  if (props.grouped) return
  apply(
    patches({ [event.detail.prop]: event.detail.val }, event.detail.rowIndex),
  )
}

function rangeEdit(event: CustomEvent<BeforeRangeSaveDataDetails>) {
  event.preventDefault()
  if (props.grouped) return
  const changes = Object.entries(event.detail.data).flatMap(([index, data]) =>
    patches(data, Number(index)),
  )
  apply(changes)
}

function guardCopy(event: CustomEvent<RangeArea | null>) {
  const range = event.detail
  if (!range) return
  if (
    (range.y1 - range.y + 1) * (range.x1 - range.x + 1) > 5000 ||
    rows.slots.slice(range.y, range.y1 + 1).some((row) => !row.id)
  ) {
    event.preventDefault()
    error.value = 'copy'
  }
}

function copy(event: CustomEvent<{ event: DataTransfer; data?: string[][] }>) {
  event.preventDefault()
  event.detail.event.setData('text/plain', formatTsv(event.detail.data ?? []))
}

function paste(
  event: CustomEvent<{
    parsed: string[][]
    dataText: string
    raw: string
    event: ClipboardEvent
  }>,
) {
  try {
    event.detail.parsed = parseTsv(event.detail.dataText || event.detail.raw)
    if (
      !focused ||
      focused.colType !== 'rgCol' ||
      focused.rowIndex + event.detail.parsed.length > props.provider.total ||
      event.detail.parsed.some((row) => row.length + focused!.colIndex > 4)
    ) {
      throw new TsvError('Paste exceeds writable grid viewport bounds')
    }
  } catch (cause) {
    if (!(cause instanceof TsvError)) throw cause
    event.preventDefault()
    event.detail.event.preventDefault()
    console.warn('Spike TSV paste rejected', cause.message)
    error.value = 'paste'
  }
}

async function keydown(event: KeyboardEvent) {
  if (
    !(event.ctrlKey || event.metaKey) ||
    event.key.toLowerCase() !== 'd' ||
    event.target instanceof HTMLInputElement ||
    event.target instanceof HTMLTextAreaElement
  )
    return
  event.preventDefault()
  event.stopPropagation()
  if (props.grouped) return
  const grid = host.value?.$el
  const range = await grid?.getSelectedRange()
  if (!range || disposed) return
  if (
    (range.x1 - range.x + 1) * (range.y1 - range.y + 1) > 5000 ||
    !rows.slots[range.y]?.id
  ) {
    error.value = 'edit'
    return
  }
  const cols = await grid!.getColumnStore(range.colType)
  const changes: CellPatch[] = []
  for (let y = range.y + 1; y <= range.y1; y++) {
    for (let x = range.x; x <= range.x1; x++) {
      if (changes.length >= 5000) {
        error.value = 'edit'
        return
      }
      const prop = cols.get('source')[cols.get('items')[x] ?? -1]?.prop
      const first = rows.slots[range.y]
      if (
        first &&
        first !== placeholder &&
        prop &&
        ['name', 'note', 'quantity'].includes(String(prop))
      ) {
        changes.push(...patches({ [prop]: first[prop as EditableField] }, y))
      }
    }
  }
  if (!disposed) apply(changes)
}

async function jump(index: number) {
  await host.value?.$el.scrollToRow(index)
}
defineExpose({ jump })
onBeforeUnmount(() => {
  disposed = true
  rows.dispose()
  emit('stats', rows.snapshot())
})
</script>

<template>
  <section
    class="data-grid"
    @keydown.capture="keydown"
    @pointerdown="focusGrid"
  >
    <p v-if="error" role="alert">{{ errorMessage }}</p>
    <RevoGrid
      ref="host"
      :source="source"
      :columns="columns"
      :editors="editors"
      :theme="preferences.isDark ? 'darkCompact' : 'compact'"
      :row-size="32"
      :range="true"
      :resize="true"
      :readonly="grouped"
      :use-clipboard="!grouped"
      :grouping="grouping"
      :hide-attribution="true"
      @created="created"
      @viewportscroll="scroll"
      @beforeedit="edit"
      @afterfocus="focusChanged"
      @beforerangeedit="rangeEdit"
      @beforecopyregion="guardCopy"
      @beforecopyapply="copy"
      @beforepasteapply="paste"
    />
  </section>
</template>

<style>
.data-grid revo-grid {
  display: block;
  height: 440px;
  width: 100%;
}
.spike-quantity {
  border-radius: 5px;
  background: var(--border);
  color: var(--text);
  padding: 2px 8px;
}
</style>
