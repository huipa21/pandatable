<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { createResourceLabel } from '../../i18n'
import { useNavStore, type NavigationItem } from '../../stores/nav'

defineProps<{ item: NavigationItem }>()
const emit = defineEmits<{
  navigate: []
  drag: [item: NavigationItem]
  drop: [item: NavigationItem]
  move: [item: NavigationItem, index: number]
}>()
const { t } = useI18n()
const nav = useNavStore()
const expanded = ref(true)
</script>

<template>
  <li :data-nav-id="item.id" :data-kind="item.kind">
    <div
      class="nav-row"
      :draggable="item.canEdit && item.kind !== 'workspace' && !nav.busy"
      @dragstart.stop="emit('drag', item)"
      @dragover.prevent
      @drop.prevent.stop="emit('drop', item)"
    >
      <button
        v-if="item.children.length"
        class="tree-toggle"
        :aria-label="t('nav.toggle', { name: item.name })"
        :aria-expanded="expanded"
        @click="expanded = !expanded"
      >
        {{ expanded ? t('nav.expanded') : t('nav.collapsed') }}
      </button>
      <RouterLink :to="item.path" @click="emit('navigate')">
        {{ item.name }}
      </RouterLink>
      <span v-if="item.pathOnly" class="path-only">{{
        t('nav.pathOnly')
      }}</span>
    </div>
    <details v-if="item.canEdit || item.canCreate" class="node-actions">
      <summary>{{ t('nav.actions', { name: item.name }) }}</summary>
      <div class="nav-actions">
        <button
          v-if="item.canCreate && item.childKind"
          :disabled="nav.busy"
          @click="
            nav.editor = {
              operation: 'create',
              kind: item.childKind,
              parentId: item.id,
            }
          "
        >
          {{ createResourceLabel(item.childKind) }}
        </button>
        <button
          v-if="item.canEdit"
          :disabled="nav.busy"
          @click="
            nav.editor = {
              operation: 'rename',
              kind: item.kind,
              id: item.id,
              name: item.name,
            }
          "
        >
          {{ t('nav.rename') }}
        </button>
        <button
          v-if="item.canEdit"
          :disabled="nav.busy"
          @click="
            nav.editor = {
              operation: 'delete',
              kind: item.kind,
              id: item.id,
              name: item.name,
            }
          "
        >
          {{ t('nav.delete') }}
        </button>
        <template v-if="item.canEdit && item.kind !== 'workspace'">
          <button
            :disabled="nav.busy || item.index === 0"
            @click="emit('move', item, item.index - 1)"
          >
            {{ t('nav.moveUp') }}
          </button>
          <button
            :disabled="nav.busy || item.index === item.siblingCount - 1"
            @click="emit('move', item, item.index + 1)"
          >
            {{ t('nav.moveDown') }}
          </button>
        </template>
      </div>
    </details>
    <ul v-if="item.children.length && expanded">
      <NavNode
        v-for="child in item.children"
        :key="child.id"
        :item="child"
        @navigate="emit('navigate')"
        @drag="emit('drag', $event)"
        @drop="emit('drop', $event)"
        @move="(node, index) => emit('move', node, index)"
      />
    </ul>
  </li>
</template>
