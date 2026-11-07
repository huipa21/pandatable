<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { type NavigationItem, useNavStore } from '../../stores/nav'
import AppIcon from '../AppIcon.vue'
import NavActions from './NavActions.vue'
import ViewActions from './ViewActions.vue'

const props = defineProps<{ item: NavigationItem }>()
const emit = defineEmits<{
  navigate: []
  drag: [item: NavigationItem]
  drop: [item: NavigationItem]
  move: [item: NavigationItem, index: number]
}>()
const { t } = useI18n()
const nav = useNavStore()
const route = useRoute()
const expanded = ref(false)
const current = computed(
  () =>
    (props.item.kind === 'book' && route.params.bookId === props.item.id) ||
    (props.item.kind === 'table' &&
      route.params.tableId === props.item.id &&
      !route.params.viewId),
)
const routeRequiresExpansion = computed(() => {
  if (props.item.kind === 'book')
    return props.item.children.some(
      (child) => child.id === route.params.tableId,
    )
  return (
    props.item.kind === 'table' &&
    route.params.tableId === props.item.id &&
    typeof route.params.viewId === 'string'
  )
})
watch(
  () => props.item.id,
  () => {
    expanded.value = false
    if (routeRequiresExpansion.value) expanded.value = true
  },
)
watch(
  routeRequiresExpansion,
  (requiresExpansion) => {
    if (requiresExpansion) expanded.value = true
  },
  { immediate: true },
)
</script>

<template>
  <li :data-nav-id="item.id" :data-kind="item.kind">
    <div
      :class="['nav-row', `nav-row-${item.kind}`, { current }]"
      :draggable="item.canEdit && !nav.busy"
      @dragstart.stop="emit('drag', item)"
      @dragover.prevent
      @drop.prevent.stop="emit('drop', item)"
    >
      <template v-if="item.kind === 'book'">
        <button
          class="book-toggle"
          :aria-label="t('nav.toggle', { name: item.name })"
          :title="t('nav.toggle', { name: item.name })"
          :aria-expanded="expanded"
          @click="expanded = !expanded"
        >
          <AppIcon
            :name="expanded ? 'chevronDown' : 'chevronRight'"
            :size="14"
          />
        </button>
        <RouterLink
          class="nav-name book-link"
          :to="item.path"
          :aria-current="current ? 'page' : undefined"
          @click="emit('navigate')"
        >
          <AppIcon name="book" :size="16" />
          <span>{{ item.name }}</span>
        </RouterLink>
      </template>
      <template v-else>
        <button
          class="table-toggle"
          :aria-label="t('nav.showViews', { name: item.name })"
          :title="t('nav.showViews', { name: item.name })"
          :aria-expanded="expanded"
          @click="expanded = !expanded"
        >
          <AppIcon class="table-icon" name="table" :size="16" />
          <AppIcon
            class="dropdown-icon"
            :name="expanded ? 'chevronDown' : 'chevronRight'"
            :size="16"
          />
        </button>
        <RouterLink
          class="nav-name"
          :to="item.path"
          :aria-current="current ? 'page' : undefined"
          @click="emit('navigate')"
        >
          {{ item.name }}
        </RouterLink>
      </template>
      <span v-if="item.pathOnly" class="path-only">{{
        t('nav.pathOnly')
      }}</span>
    </div>
    <NavActions
      :item="item"
      @move="(node, index) => emit('move', node, index)"
    />
    <ul v-if="item.kind === 'book' && expanded && item.children.length">
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
    <ul v-if="item.kind === 'table' && expanded" class="view-list">
      <li v-if="!item.views.length" class="path-only">
        {{ t('nav.emptyViews') }}
      </li>
      <li v-for="view in item.views" :key="view.id" :data-view-id="view.id">
        <RouterLink
          class="nav-row"
          :to="`/t/${item.id}/v/${view.id}`"
          :aria-current="
            route.params.tableId === item.id && route.params.viewId === view.id
              ? 'page'
              : undefined
          "
          @click="emit('navigate')"
        >
          <AppIcon name="view" :size="14" />
          <span class="nav-name">{{ view.name }}</span>
        </RouterLink>
        <ViewActions :table="item" :view="view" />
      </li>
    </ul>
  </li>
</template>

<style scoped>
.nav-row-book .book-toggle {
  flex: 0 0 24px;
  justify-content: center;
}

.nav-row-book .book-link {
  display: inline-flex;
  flex: 1;
  align-items: center;
  gap: 4px;
}

.book-link span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 767px) {
  .nav-row-book .book-toggle {
    flex-basis: 44px;
    min-width: 44px;
    height: 44px;
  }
}
</style>
