<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { errorMessage, request, RequestError } from '../../api/client'
import type {
  DataRow,
  RowQueryRequest,
  RowQueryResult,
  TableSchema,
  TableView,
} from '../../api/types'
import { attachmentCover } from '../../fields/attachment'
import { bindField } from '../../fields/registry'
import { isWireValue, type FieldEnv, type WireValue } from '../../fields/types'
import { useAuthStore } from '../../stores/auth'
import { useNavStore } from '../../stores/nav'
import { useRealtimeStore } from '../../stores/realtime'
import FieldRenderer from '../fields/FieldRenderer.vue'
import CardFieldValues from './CardFieldValues.vue'
import { bindCardFields } from './cardFields'

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
const pageSize = 20
const maxOffset = 1_000_000
const maxPages = Math.floor(maxOffset / pageSize) + 1

const rows = shallowRef<DataRow[]>([])
const total = ref(0)
const page = ref(0)
const loading = ref(false)
const loadError = ref('')
const failedCovers = ref(new Set<string>())
let disposed = false
let generation = 0
let readController: AbortController | undefined
let reloadTimer: ReturnType<typeof setTimeout> | undefined

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
const coverField = computed(() =>
  props.schema.fields.find(
    (field) =>
      field.id === props.view.config.gallery?.coverField &&
      field.uiType === 'attachment',
  ),
)
const cardFields = computed(() =>
  bindCardFields(
    props.schema.fields,
    props.view.config.gallery?.fields ?? [],
    displayField.value?.id ?? null,
  ),
)
const fieldIds = computed(() =>
  Array.from(
    new Set(
      [
        displayField.value?.id,
        coverField.value?.id,
        ...(props.view.config.gallery?.fields ?? []),
      ].filter((id): id is string => !!id),
    ),
  ),
)
const env = computed<FieldEnv>(() => ({
  timeZone: auth.user?.effectiveTimezone,
  locale: locale.value,
}))
const pageCount = computed(() =>
  Math.max(1, Math.min(maxPages, Math.ceil(total.value / pageSize))),
)
const cards = computed(() =>
  rows.value.map((row) => {
    let cover: { name: string; thumb: string } | null = null
    let invalid = false
    if (coverField.value) {
      const result = attachmentCover(row.fields[coverField.value.id])
      if (result.ok) cover = result.value
      else invalid = true
    }
    return {
      row,
      cover,
      unavailable:
        invalid ||
        (cover !== null && failedCovers.value.has(`${row.id}:${cover.thumb}`)),
    }
  }),
)

function hasTitle(row: DataRow) {
  const value = displayField.value
    ? row.fields[displayField.value.id]
    : undefined
  return isWireValue(value) && value !== null
}

function wireValue(value: unknown): WireValue {
  if (!isWireValue(value)) throw new Error('Unsupported Gallery field value')
  return value
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
    rows.value = []
    total.value = 0
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
  if (!props.identity || props.identity !== auth.user?.id) {
    rows.value = []
    total.value = 0
    loading.value = false
    return
  }
  const body: RowQueryRequest = {
    offset: page.value * pageSize,
    limit: pageSize,
    includeCount: true,
    fieldIds: fieldIds.value,
    ...(props.view.config.filter ? { filter: props.view.config.filter } : {}),
  }
  loading.value = true
  loadError.value = ''
  try {
    const result = await request<RowQueryResult>(
      `/views/${encodeURIComponent(props.view.id)}/query`,
      'POST',
      body,
      AbortSignal.any([controller.signal, AbortSignal.timeout(10_000)]),
    )
    if (!current(token)) return
    if (result.schemaVersion !== props.schema.schemaVersion)
      throw new RequestError('ERR_SCHEMA_CHANGED', 409)
    if (
      !Array.isArray(result.rows) ||
      result.rows.length > pageSize ||
      result.total === null ||
      !Number.isSafeInteger(result.total) ||
      result.total < 0
    )
      throw new RequestError('ERR_INTERNAL', 500)
    const lastPage = Math.max(0, Math.ceil(result.total / pageSize) - 1)
    if (page.value > lastPage) {
      page.value = lastPage
      await refresh()
      return
    }
    rows.value = result.rows
    total.value = result.total
    failedCovers.value = new Set()
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

watch(
  [() => props.schema.table.id, () => props.identity, () => auth.user?.id],
  ([tableId, identity, userId], _previous, onCleanup) => {
    if (!tableId || !identity || identity !== userId) return
    onCleanup(
      realtime.subscribe(tableId, (event) => {
        if (event.t === 'sub.denied') {
          readController?.abort()
          generation++
          loading.value = false
          const failure = new RequestError('ERR_PERMISSION_DENIED', 403)
          handleFailure(failure)
          loadError.value = errorMessage(failure)
        } else if (
          [
            'row.updated',
            'row.created',
            'row.deleted',
            'row.bulk',
            'resync',
          ].includes(event.t)
        ) {
          scheduleRefresh()
        }
      }),
    )
  },
  { immediate: true },
)

watch(
  () =>
    [
      props.view.id,
      JSON.stringify(props.view.config.gallery),
      JSON.stringify(props.view.config.filter),
      JSON.stringify(props.view.config.sort),
      props.schema.schemaVersion,
      props.identity,
      auth.user?.id,
    ] as const,
  () => {
    page.value = 0
    rows.value = []
    total.value = 0
    failedCovers.value = new Set()
    void refresh()
  },
  { immediate: true },
)

function changePage(direction: -1 | 1) {
  page.value = Math.max(
    0,
    Math.min(pageCount.value - 1, page.value + direction),
  )
  void refresh()
}

function coverFailed(rowId: string, thumbnail: string) {
  failedCovers.value.add(`${rowId}:${thumbnail}`)
}

onBeforeUnmount(() => {
  disposed = true
  generation++
  clearTimeout(reloadTimer)
  readController?.abort()
})

defineExpose({ refresh })
</script>

<template>
  <section class="gallery-view" :aria-label="t('gallery.viewType')">
    <p v-if="loading" role="status">{{ t('gallery.loading') }}</p>
    <div v-if="loadError" role="alert">
      <p>{{ t('gallery.loadFailed', { message: loadError }) }}</p>
      <button type="button" @click="refresh">{{ t('auth.retry') }}</button>
    </div>
    <p v-if="total > maxPages * pageSize">
      {{ t('gallery.pagingLimit', { count: maxPages * pageSize }) }}
    </p>
    <p v-if="!loading && !loadError && rows.length === 0">
      {{ t('gallery.empty') }}
    </p>
    <div class="gallery-cards" :aria-busy="loading">
      <article
        v-for="card in cards"
        :key="card.row.id"
        class="gallery-card"
        :data-row-id="card.row.id"
      >
        <div class="gallery-cover">
          <img
            v-if="card.cover && !card.unavailable"
            :src="card.cover.thumb"
            :alt="card.cover.name"
            loading="lazy"
            @error="coverFailed(card.row.id, card.cover.thumb)"
          />
          <span v-else-if="card.unavailable">{{
            t('gallery.coverUnavailable')
          }}</span>
          <span v-else>{{ t('gallery.noImage') }}</span>
        </div>
        <h3 class="card-title">
          <FieldRenderer
            v-if="displayField && displayBound?.ok && hasTitle(card.row)"
            :field="displayBound.value"
            :value="wireValue(card.row.fields[displayField.id])"
            :env="env"
            :table-id="schema.table.id"
            :identity="identity"
            :links="false"
          />
          <span v-else>{{
            t('mobileRows.untitled', { id: card.row.id })
          }}</span>
        </h3>
        <CardFieldValues
          :items="cardFields"
          :row="card.row"
          :env="env"
          :table-id="schema.table.id"
          :identity="identity"
        />
        <button
          type="button"
          :aria-label="t('gallery.openRow', { id: card.row.id })"
          @click="emit('expand', card.row.id)"
        >
          {{ t('gallery.open') }}
        </button>
      </article>
    </div>
    <nav
      v-if="pageCount > 1"
      class="page-controls"
      :aria-label="t('gallery.page', { page: page + 1, total: pageCount })"
    >
      <button
        type="button"
        :disabled="loading || page === 0"
        @click="changePage(-1)"
      >
        {{ t('gallery.previous') }}
      </button>
      <span>{{ t('gallery.page', { page: page + 1, total: pageCount }) }}</span>
      <button
        type="button"
        :disabled="loading || page + 1 >= pageCount"
        @click="changePage(1)"
      >
        {{ t('gallery.next') }}
      </button>
    </nav>
  </section>
</template>

<style scoped>
.gallery-view {
  min-width: 0;
  margin: 16px 0;
}
.gallery-cards {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 220px), 1fr));
  gap: 12px;
}
.gallery-card {
  display: grid;
  align-content: start;
  gap: 8px;
  min-width: 0;
  padding: 10px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 8px;
}
.gallery-cover {
  display: grid;
  width: 100%;
  aspect-ratio: 4 / 3;
  overflow: hidden;
  place-items: center;
  background: var(--background);
  color: var(--muted);
}
.gallery-cover img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.card-title {
  min-width: 0;
  margin: 0;
}
.page-controls {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  margin-top: 12px;
}
button {
  min-width: 0;
  padding: 8px;
  color: var(--text);
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 4px;
}
@media (max-width: 767px) {
  button {
    min-height: 44px;
    font-size: 16px;
  }
}
</style>
