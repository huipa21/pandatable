<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { type NavigationItem, useNavStore } from '../../stores/nav'
import { useNavLocation } from '../../composables/useNavLocation'
import AppIcon from '../AppIcon.vue'
import NavActions from './NavActions.vue'
import NavNode from './NavNode.vue'

const emit = defineEmits<{ navigate: [] }>()
const { t } = useI18n()
const nav = useNavStore()
const { workspace, routeWorkspaceId, activeBook } = useNavLocation()
const draggedItem = ref<NavigationItem | null>(null)

function reorder(item: NavigationItem, index: number) {
  void nav.reorder(item, index)
}

function drop(item: NavigationItem) {
  const source = draggedItem.value
  draggedItem.value = null
  if (source && source.kind === item.kind && source.parentId === item.parentId)
    reorder(source, item.index)
}
</script>

<template>
  <nav :aria-label="t('nav.title')" class="nav-tree">
    <div class="workspace-heading">
      <h2>{{ t('workspaceGrid.ux02.workspaces') }}</h2>
      <button
        class="icon-button"
        :aria-label="t('nav.create.workspace')"
        :title="t('nav.create.workspace')"
        :disabled="nav.busy"
        @click="nav.editor = { operation: 'create', kind: 'workspace' }"
      >
        <AppIcon name="plus" :size="16" />
      </button>
    </div>
    <p v-if="nav.loading && !nav.loaded" role="status">
      {{ t('nav.loading') }}
    </p>
    <p v-if="nav.error && nav.loaded" class="path-only">{{ t('nav.stale') }}</p>
    <p v-if="nav.preferenceError && nav.preferenceLoaded" class="path-only">
      {{ t('workspaceGrid.ux02.preferenceStale') }}
    </p>
    <p
      v-if="
        nav.loaded && !nav.error && !nav.preferenceBusy && !nav.items.length
      "
      class="path-only"
    >
      {{ t('nav.empty') }}
    </p>
    <ul v-if="nav.loaded" class="workspace-root">
      <li
        v-for="item in nav.items"
        :key="item.id"
        :data-nav-id="item.id"
        data-kind="workspace"
      >
        <div :class="['workspace-box', { current: workspace?.id === item.id }]">
          <RouterLink
            class="workspace-link"
            :to="item.path"
            :aria-current="
              workspace?.id === item.id
                ? routeWorkspaceId === item.id
                  ? $route.path === item.path
                    ? 'page'
                    : 'location'
                  : undefined
                : undefined
            "
            @click="emit('navigate')"
          >
            {{ item.name }}
          </RouterLink>
          <span
            v-if="nav.preferenceLoaded && nav.preferredWorkspaceId === item.id"
            class="workspace-default"
            role="img"
            :aria-label="
              t('workspaceGrid.ux02.defaultMarker', { name: item.name })
            "
            :title="t('workspaceGrid.ux02.defaultMarker', { name: item.name })"
          >
            {{ t('workspaceGrid.ux02.default') }}
          </span>
          <span v-if="item.pathOnly" class="path-only">
            {{ t('nav.pathOnly') }}
          </span>
        </div>
        <NavActions :item="item" />
      </li>
    </ul>
    <ul
      v-if="activeBook"
      class="workbook-root"
      :data-context-book="activeBook.id"
    >
      <NavNode
        :item="activeBook"
        @navigate="emit('navigate')"
        @drag="draggedItem = $event"
        @drop="drop"
        @move="reorder"
      />
    </ul>
  </nav>
</template>

<style scoped>
.workbook-root {
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--border);
}
</style>
