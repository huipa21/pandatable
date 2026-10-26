<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useNavStore, type NavigationItem } from '../../stores/nav'
import NavNode from './NavNode.vue'

const emit = defineEmits<{ navigate: [] }>()
const { t } = useI18n()
const nav = useNavStore()
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
    <h2>{{ t('nav.title') }}</h2>
    <div class="nav-actions">
      <button
        :disabled="nav.busy"
        @click="nav.editor = { operation: 'create', kind: 'workspace' }"
      >
        {{ t('nav.create.workspace') }}
      </button>
      <button :disabled="nav.loading" @click="nav.refresh(true)">
        {{ t('nav.refresh') }}
      </button>
    </div>
    <p v-if="nav.loading" role="status">{{ t('nav.loading') }}</p>
    <p v-if="nav.error && nav.loaded">{{ t('nav.stale') }}</p>
    <p v-if="nav.loaded && !nav.error && nav.items.length === 0">
      {{ t('nav.empty') }}
    </p>
    <ul v-if="nav.loaded">
      <NavNode
        v-for="item in nav.items"
        :key="item.id"
        :item="item"
        @navigate="emit('navigate')"
        @drag="dragged = $event"
        @drop="drop"
        @move="move"
      />
    </ul>
    <button class="trash-placeholder" disabled>
      {{ t('nav.trashUnavailable') }}
    </button>
  </nav>
</template>
