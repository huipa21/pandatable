<script setup lang="ts">
import {
  computed,
  defineAsyncComponent,
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
  FilterNode,
  ResourceKind,
  TableSchema,
  TableView,
  ViewColumns,
  ViewMutation,
} from '../api/types'
import { useAuthStore } from '../stores/auth'
import { tablePath, useNavStore, type NavigationItem } from '../stores/nav'
import { useSharingStore } from '../stores/sharing'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const nav = useNavStore()
const sharing = useSharingStore()
const schema = shallowRef<TableSchema | null>(null)
// Lazy so the field registry/editor stay out of the initial bundle (D-130).
const FieldPanel = defineAsyncComponent(
  () => import('../components/fields/FieldPanel.vue'),
)
const FilterBuilder = defineAsyncComponent(
  () => import('../components/filters/FilterBuilder.vue'),
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
const grid = ref<{ refresh: () => Promise<void> }>()
const media = window.matchMedia('(min-width: 768px)')
const desktop = ref(media.matches)
const newRow = ref(false)
const filterValue = shallowRef<FilterNode | null>(null)
const filterDraft = shallowRef<FilterNode | null>(null)
const filterApplied = ref(false)
const localViewConfig = shallowRef<ViewColumns | null>(null)
const hasLocalViewChanges = ref(false)
const viewError = ref('')
const viewBusy = ref(false)
const newViewName = ref('')
const newViewScope = ref<'shared' | 'personal'>('personal')
const viewNameDrafts = ref<Record<string, string>>({})
const resized = (event: MediaQueryListEvent) => {
  desktop.value = event.matches
  newRow.value = false
}
onMounted(() => media.addEventListener('change', resized))
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
const breadcrumbs = computed(() => {
  const items: NavigationItem[] = []
  let current = node.value
  while (current) {
    items.unshift(current)
    const parentId: string | null = current.parentId
    current = nav.flatItems.find((item) => item.id === parentId)
  }
  return items
})
const displayField = computed(() =>
  schema.value?.fields.find(
    (field) => field.id === schema.value?.table.displayFieldId,
  ),
)
const view = computed(() =>
  schema.value?.views.find((item) => item.id === route.params.viewId),
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
  schema.value = next
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
    if (!route.params.viewId && result.views.length)
      await router.replace(tablePath(result.table.id, result.views))
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
    () => node.value?.name,
    () => node.value?.level,
  ],
  () => void loadSchema(),
  { immediate: true },
)
watch(
  () => nav.revision,
  () => {
    if (!loading.value) void loadSchema(true)
  },
)
watch(
  () => auth.user?.id,
  () => void loadSchema(),
)
watch(
  [
    () => route.params.tableId,
    () => route.params.viewId,
    () => route.params.rowId,
    () => auth.user?.id,
  ],
  () => {
    newRow.value = false
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
      { name: newViewName.value, scope: newViewScope.value },
    )
    schema.value = {
      ...schema.value,
      views: [...schema.value.views, result.view],
    }
    newViewName.value = ''
    void nav.refresh(true)
    await router.push(`/t/${schema.value.table.id}/v/${result.view.id}`)
  } catch (cause) {
    viewRequestFailed(cause)
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
  media.removeEventListener('change', resized)
})
</script>

<template>
  <section class="resource-page">
    <nav
      v-if="!missing && !error"
      class="breadcrumbs"
      :aria-label="t('nav.breadcrumbs')"
    >
      <RouterLink to="/home">{{ t('auth.homeTitle') }}</RouterLink>
      <template v-for="item in breadcrumbs" :key="item.id">
        <span aria-hidden="true">{{ t('nav.separator') }}</span>
        <RouterLink :to="item.path">{{ item.name }}</RouterLink>
        <span v-if="item.pathOnly" class="path-only">{{
          t('nav.pathOnly')
        }}</span>
      </template>
    </nav>
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
      <h1>{{ schema?.table.name ?? node.name }}</h1>
      <button v-if="canShare" @click="sharing.open(node)">
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
      </template>
      <template v-else-if="schema">
        <dl class="schema-summary">
          <dt>{{ t('nav.displayField') }}</dt>
          <dd>{{ displayField?.name ?? t('nav.noDisplayField') }}</dd>
          <dt>{{ t('nav.access') }}</dt>
          <dd>{{ accessLevelLabel(schema.myLevel) }}</dd>
        </dl>
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
                v-if="schema.myLevel === 'owner' && item.scope === 'shared'"
                type="button"
                :disabled="viewBusy"
                @click="toggleViewLock(item)"
              >
                {{ item.locked ? t('nav.unlockView') : t('nav.lockView') }}
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
          <button type="submit" :disabled="viewBusy || !newViewName.trim()">
            {{ t('nav.createView') }}
          </button>
        </form>
        <p v-if="viewError" role="alert">{{ viewError }}</p>
        <p v-if="schema.views.length === 0">{{ t('nav.emptyViews') }}</p>
        <details v-if="view" class="view-filter">
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
        <FieldPanel
          v-if="desktop"
          :key="schema.table.id"
          :schema="schema"
          @update="applySchema"
          @reload="loadSchema(true)"
        />
        <DataGrid
          v-if="view && desktop"
          ref="grid"
          :key="`${schema.table.id}:${view.id}:${schema.schemaVersion}:${auth.user?.id ?? ''}`"
          :schema="schema"
          :view-id="view.id"
          :identity="auth.user?.id ?? ''"
          :filter="filterValue"
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
            (id) => router.push(`/t/${schema!.table.id}/v/${view!.id}/r/${id}`)
          "
        />
        <MobileCardList
          v-else-if="view"
          ref="grid"
          :key="`${schema.table.id}:${view.id}:${auth.user?.id ?? ''}`"
          :schema="schema"
          :view-id="view.id"
          :identity="auth.user?.id ?? ''"
          :filter="filterValue"
          @schema-changed="loadSchema(true)"
          @expand="
            (id) => router.push(`/t/${schema!.table.id}/v/${view!.id}/r/${id}`)
          "
          @add="newRow = true"
        >
          <FieldPanel
            :schema="schema"
            @update="applySchema"
            @reload="loadSchema(true)"
          />
        </MobileCardList>
        <RowExpandDialog
          v-if="view && (typeof route.params.rowId === 'string' || newRow)"
          :schema="schema"
          :view-id="view.id"
          :row-id="
            typeof route.params.rowId === 'string' ? route.params.rowId : null
          "
          :identity="auth.user?.id ?? ''"
          @close="
            () => {
              newRow = false
              router.replace(`/t/${schema!.table.id}/v/${view!.id}`)
            }
          "
          @saved="grid?.refresh()"
          @created="
            (id) => {
              newRow = false
              grid?.refresh()
              router.replace(`/t/${schema!.table.id}/v/${view!.id}/r/${id}`)
            }
          "
          @schema-changed="loadSchema(true)"
        />
      </template>
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
