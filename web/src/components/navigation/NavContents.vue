<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useNavStore, type NavigationItem } from '../../stores/nav'
import { useNavLocation } from '../../composables/useNavLocation'
import AppIcon from '../AppIcon.vue'
import NavActions from './NavActions.vue'
import NavNode from './NavNode.vue'

const emit = defineEmits<{ navigate: [] }>()
const { t } = useI18n()
const nav = useNavStore()
const { workspace } = useNavLocation()
let dragged: NavigationItem | null = null

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
    <p v-if="nav.loading && !nav.loaded" role="status">
      {{ t('nav.loading') }}
    </p>
    <p v-if="nav.error && nav.loaded" class="path-only">{{ t('nav.stale') }}</p>
    <ul v-if="nav.loaded && workspace" class="workspace-root">
      <li :data-nav-id="workspace.id" data-kind="workspace">
        <div class="workspace-box">
          <RouterLink
            class="workspace-link"
            :to="workspace.path"
            @click="emit('navigate')"
          >
            {{ workspace.name }}
          </RouterLink>
          <span v-if="workspace.pathOnly" class="path-only">{{
            t('nav.pathOnly')
          }}</span>
        </div>
        <NavActions :item="workspace" />
        <div class="workspace-section">
          <span class="path-only">{{
            t('nav.bookCount', workspace.children.length)
          }}</span>
          <button
            v-if="workspace.canCreate"
            class="icon-button"
            :aria-label="t('nav.createBookIn', { name: workspace.name })"
            :title="t('nav.create.book')"
            :disabled="nav.busy"
            @click="
              nav.editor = {
                operation: 'create',
                kind: 'book',
                parentId: workspace.id,
              }
            "
          >
            <AppIcon name="plus" :size="16" />
          </button>
        </div>
        <p
          v-if="workspace.children.length === 0 && !workspace.pathOnly"
          class="path-only"
        >
          {{ t('nav.emptyBooks') }}
        </p>
        <ul>
          <NavNode
            v-for="item in workspace.children"
            :key="item.id"
            :item="item"
            @navigate="emit('navigate')"
            @drag="dragged = $event"
            @drop="drop"
            @move="move"
          />
        </ul>
      </li>
    </ul>
    <RouterLink
      v-else
      class="workspace-box workspace-placeholder"
      to="/home"
      @click="emit('navigate')"
    >
      {{ t('nav.selectWorkspace') }}
    </RouterLink>
  </nav>
</template>
