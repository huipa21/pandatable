<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { onBeforeRouteLeave } from 'vue-router'
import { errorMessage } from '../api/client'
import {
  useResourceSearch,
  type ResourceSearchItem,
} from '../composables/useResourceSearch'
import { useAuthStore } from '../stores/auth'
import { useNavStore, type NavigationItem } from '../stores/nav'
import AppIcon from '../components/AppIcon.vue'
import ResourceSearchInput from '../components/ResourceSearchInput.vue'
import NavActions from '../components/navigation/NavActions.vue'

interface DashboardWorkspace extends ResourceSearchItem {
  navigation: NavigationItem
}

const { t } = useI18n()
const auth = useAuthStore()
const nav = useNavStore()
const query = ref('')
const resolving = ref(false)
const resolveError = ref<unknown>(null)
let controller: AbortController | undefined
let active = true

const workspaces = computed<DashboardWorkspace[]>(() =>
  nav.items.map((item, index) => ({
    id: item.id,
    name: item.name,
    description: item.pathOnly ? '' : item.description,
    lastOpenedAt: item.lastOpenedAt,
    index,
    navigation: item,
  })),
)
const orderedWorkspaces = useResourceSearch(
  workspaces,
  query,
  () => nav.preferredWorkspaceId,
)
const hasQuery = computed(() => query.value.trim().length > 0)
const showEmptyState = computed(
  () =>
    nav.loaded &&
    !nav.error &&
    !resolving.value &&
    !resolveError.value &&
    !orderedWorkspaces.value.length &&
    !hasQuery.value,
)
const showNoMatches = computed(
  () =>
    nav.loaded &&
    !nav.error &&
    !resolving.value &&
    !resolveError.value &&
    hasQuery.value &&
    !orderedWorkspaces.value.length,
)

async function resolve() {
  const userId = auth.user?.id
  if (!userId || resolving.value || !active) return
  const own = new AbortController()
  controller = own
  resolving.value = true
  resolveError.value = null
  const current = () =>
    active && !own.signal.aborted && auth.user?.id === userId
  try {
    await nav.resolvePreference(own.signal)
    if (!current()) return
    await nav.refresh(true)
  } catch (cause) {
    if (current()) resolveError.value = cause
  } finally {
    if (controller === own) resolving.value = false
  }
}

function openCreateDialog() {
  nav.editor = { operation: 'create', kind: 'workspace' }
}

function actionItem(workspace: DashboardWorkspace): NavigationItem {
  return {
    ...workspace.navigation,
    canCreate: false,
    childKind: null,
  }
}

watch(
  () => auth.user?.id,
  (id) => {
    controller?.abort()
    resolving.value = false
    resolveError.value = null
    query.value = ''
    if (id && active) void resolve()
  },
  { immediate: true },
)
onBeforeRouteLeave(() => {
  active = false
  controller?.abort()
})
onBeforeUnmount(() => {
  active = false
  controller?.abort()
})
</script>

<template>
  <section class="home-page workspace-dashboard">
    <header class="dashboard-heading">
      <h1>{{ t('workspaceGrid.ux02.workspaces') }}</h1>
      <button :disabled="nav.busy" @click="openCreateDialog">
        {{ t('nav.create.workspace') }}
      </button>
    </header>

    <ResourceSearchInput
      v-model="query"
      :label="t('dashboard.ds07.searchLabel')"
      :placeholder="t('dashboard.ds07.searchPlaceholder')"
      :clear-label="t('dashboard.ds07.clearSearch')"
    />

    <p v-if="(!nav.loaded || resolving) && !nav.error" role="status">
      {{ resolving ? t('dashboard.ds07.resolving') : t('nav.loading') }}
    </p>
    <div v-if="resolveError" class="dashboard-error" role="alert">
      <h2>{{ t('dashboard.ds07.resolveError') }}</h2>
      <p>{{ errorMessage(resolveError) }}</p>
      <button :disabled="resolving" @click="resolve">
        {{ t('auth.retry') }}
      </button>
    </div>
    <div v-if="!nav.loaded && nav.error" class="dashboard-error" role="alert">
      <h2>{{ t('nav.unavailable') }}</h2>
      <p>{{ errorMessage(nav.error) }}</p>
      <button :disabled="nav.loading" @click="nav.refresh(true)">
        {{ t('auth.retry') }}
      </button>
    </div>
    <div
      v-else-if="nav.loaded && nav.error"
      class="dashboard-stale"
      role="status"
    >
      <p>{{ t('nav.stale') }}</p>
      <button :disabled="nav.loading" @click="nav.refresh(true)">
        {{ t('auth.retry') }}
      </button>
    </div>

    <ul
      v-if="nav.loaded"
      class="workspace-dashboard-list"
      role="list"
      :aria-label="t('workspaceGrid.ux02.workspaces')"
    >
      <li
        v-for="workspace in orderedWorkspaces"
        :key="workspace.id"
        class="workspace-dashboard-row"
        :data-nav-id="workspace.id"
        data-kind="workspace"
      >
        <RouterLink
          :to="workspace.navigation.path"
          class="workspace-dashboard-link"
        >
          <span class="workspace-dashboard-icon">
            <AppIcon
              v-if="workspace.navigation.bookIcon"
              :name="workspace.navigation.bookIcon"
              :size="24"
            />
          </span>
          <span class="workspace-dashboard-name">{{ workspace.name }}</span>
          <span
            v-if="!workspace.navigation.pathOnly && workspace.description"
            class="workspace-dashboard-description"
            tabindex="0"
            :title="workspace.description"
          >
            {{ workspace.description }}
          </span>
        </RouterLink>
        <span
          v-if="
            nav.preferenceLoaded && nav.preferredWorkspaceId === workspace.id
          "
          class="workspace-dashboard-default"
          role="img"
          :aria-label="
            t('workspaceGrid.ux02.defaultMarker', { name: workspace.name })
          "
          :title="
            t('workspaceGrid.ux02.defaultMarker', { name: workspace.name })
          "
        >
          {{ t('workspaceGrid.ux02.default') }}
        </span>
        <span
          v-if="workspace.navigation.pathOnly"
          class="workspace-dashboard-path-only"
        >
          {{ t('nav.pathOnly') }}
        </span>
        <NavActions :item="actionItem(workspace)" />
      </li>
      <li v-if="showEmptyState" class="workspace-dashboard-empty">
        <p>{{ t('nav.empty') }}</p>
        <button :disabled="nav.busy" @click="openCreateDialog">
          {{ t('nav.create.workspace') }}
        </button>
      </li>
      <li v-else-if="showNoMatches" class="workspace-dashboard-empty">
        <p>{{ t('dashboard.ds07.noMatches') }}</p>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.workspace-dashboard {
  width: min(100%, 1120px);
  margin: 0 auto;
}

.dashboard-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.dashboard-heading h1 {
  margin: 8px 0 16px;
  font-size: clamp(26px, 4vw, 36px);
}

.workspace-dashboard-list {
  display: grid;
  gap: 8px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.workspace-dashboard-row {
  position: relative;
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
  min-height: 56px;
  padding: 8px 10px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 8px;
}

.workspace-dashboard-link {
  display: flex;
  flex: 1;
  align-items: center;
  gap: 12px;
  min-width: 0;
  color: inherit;
  text-decoration: none;
}

.workspace-dashboard-link:hover .workspace-dashboard-name {
  color: var(--accent);
}

.workspace-dashboard-icon {
  display: grid;
  flex: 0 0 32px;
  place-items: center;
  color: var(--accent);
}

.workspace-dashboard-name {
  flex: 0 1 auto;
  min-width: 0;
  font-weight: 650;
  overflow-wrap: anywhere;
}

.workspace-dashboard-description {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  color: var(--muted);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.workspace-dashboard-description:focus {
  overflow: visible;
  white-space: normal;
  overflow-wrap: anywhere;
}

.workspace-dashboard-default,
.workspace-dashboard-path-only {
  flex: 0 0 auto;
  color: var(--muted);
  font-size: 12px;
}

.workspace-dashboard-default {
  color: var(--accent);
}

.workspace-dashboard-empty {
  padding: 20px 12px;
  text-align: center;
}

.dashboard-stale,
.dashboard-error {
  margin: 12px 0;
}

:deep(.workspace-dashboard-row > .node-actions) {
  position: relative;
  flex: 0 0 auto;
  color: var(--muted);
}

:deep(.workspace-dashboard-row > .node-actions > summary) {
  width: 36px;
  height: 36px;
}

:deep(.workspace-dashboard-row > .node-actions > .nav-actions) {
  position: absolute;
  z-index: 30;
  top: 38px;
  right: 0;
  flex-direction: column;
  align-items: stretch;
  min-width: 170px;
  margin: 0;
  padding: 6px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 8px;
  box-shadow: 0 6px 24px rgb(0 0 0 / 12%);
}

@media (max-width: 520px) {
  .workspace-dashboard-row {
    gap: 6px;
    padding: 8px 6px;
  }

  .workspace-dashboard-link {
    gap: 6px;
  }

  .workspace-dashboard-icon {
    flex-basis: 26px;
  }

  .workspace-dashboard-name {
    flex: 0 1 45%;
  }
}
</style>
