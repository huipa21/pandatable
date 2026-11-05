<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useNavStore, type NavigationItem } from '../../stores/nav'
import { useNavLocation } from '../../composables/useNavLocation'
import AppIcon from '../AppIcon.vue'
import NavActions from './NavActions.vue'
import NavNode from './NavNode.vue'

const emit = defineEmits<{ navigate: [] }>()
const { t } = useI18n()
const nav = useNavStore()
const { workspace, routeWorkspaceId } = useNavLocation()
const collapsed = ref(new Set<string>())
let dragged: NavigationItem | null = null

watch(routeWorkspaceId, (id) => {
  if (id && collapsed.value.has(id)) {
    const next = new Set(collapsed.value)
    next.delete(id)
    collapsed.value = next
  }
})

function toggleWorkspace(id: string) {
  const next = new Set(collapsed.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  collapsed.value = next
}

async function move(item: NavigationItem, index: number) {
  try {
    await nav.reorder(item, index)
  } catch {
    // The store exposes the translated mutation failure beside the tree.
  }
}
function drop(target: NavigationItem) {
  const source = dragged
  dragged = null
  if (
    source &&
    source.canEdit &&
    target.canEdit &&
    source.kind !== 'workspace' &&
    source.kind === target.kind &&
    source.parentId === target.parentId &&
    source.id !== target.id &&
    !nav.busy
  )
    void move(source, target.index)
}
</script>

<template>
  <nav :aria-label="t('nav.title')" class="nav-tree" @dragend="dragged = null">
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
          <button
            class="workspace-toggle"
            :aria-label="t('nav.toggle', { name: item.name })"
            :aria-expanded="!collapsed.has(item.id)"
            :aria-controls="`workspace-children-${item.id}`"
            @click="toggleWorkspace(item.id)"
          >
            <AppIcon
              :name="collapsed.has(item.id) ? 'chevronRight' : 'chevronDown'"
              :size="14"
            />
          </button>
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
          <span v-if="item.pathOnly" class="path-only">{{
            t('nav.pathOnly')
          }}</span>
        </div>
        <NavActions :item="item" />
        <div
          v-show="!collapsed.has(item.id)"
          :id="`workspace-children-${item.id}`"
        >
          <div class="workspace-section">
            <span class="path-only">{{
              t('nav.bookCount', item.children.length)
            }}</span>
            <button
              v-if="item.canCreate"
              class="icon-button"
              :aria-label="t('nav.createBookIn', { name: item.name })"
              :title="t('nav.create.book')"
              :disabled="nav.busy"
              @click="
                nav.editor = {
                  operation: 'create',
                  kind: 'book',
                  parentId: item.id,
                }
              "
            >
              <AppIcon name="plus" :size="16" />
            </button>
          </div>
          <p
            v-if="item.children.length === 0 && !item.pathOnly"
            class="path-only"
          >
            {{ t('nav.emptyBooks') }}
          </p>
          <ul>
            <NavNode
              v-for="child in item.children"
              :key="child.id"
              :item="child"
              @navigate="emit('navigate')"
              @drag="dragged = $event"
              @drop="drop"
              @move="move"
            />
          </ul>
        </div>
      </li>
    </ul>
  </nav>
</template>
