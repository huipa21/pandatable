<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { type NavigationItem, useNavStore } from '../../stores/nav'
import { useNavLocation } from '../../composables/useNavLocation'
import AppIcon from '../AppIcon.vue'
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
    <ul class="workspace-root">
      <li data-kind="dashboard">
        <div :class="['workspace-box', { current: $route.path === '/home' }]">
          <RouterLink
            class="workspace-link"
            to="/home"
            :aria-current="$route.path === '/home' ? 'page' : undefined"
            @click="emit('navigate')"
          >
            {{ t('workspaceGrid.ux02.workspaces') }}
          </RouterLink>
        </div>
      </li>
      <li
        v-if="routeWorkspaceId && workspace"
        :data-nav-id="workspace.id"
        data-kind="workspace"
      >
        <div class="workspace-box current">
          <RouterLink
            class="workspace-link"
            :to="workspace.path"
            :aria-current="$route.path === workspace.path ? 'page' : 'location'"
            @click="emit('navigate')"
          >
            <AppIcon
              v-if="workspace.bookIcon"
              :name="workspace.bookIcon"
              :size="16"
            />
            {{ workspace.name }}
          </RouterLink>
          <span v-if="workspace.pathOnly" class="path-only">
            {{ t('nav.pathOnly') }}
          </span>
        </div>
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
