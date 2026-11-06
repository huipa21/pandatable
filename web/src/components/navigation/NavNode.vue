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
// Workbooks start open; a table's view list is a dropdown opened on demand.
const expanded = ref(props.item.kind === 'book')
const current = computed(
  () =>
    props.item.kind === 'table' &&
    route.params.tableId === props.item.id &&
    !route.params.viewId,
)
watch(
  [() => route.params.tableId, () => route.params.viewId],
  ([tableId, viewId]) => {
    if (
      props.item.kind === 'table' &&
      tableId === props.item.id &&
      typeof viewId === 'string'
    )
      expanded.value = true
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
      <button
        v-if="item.kind === 'book'"
        class="book-toggle"
        :aria-expanded="expanded"
        @click="expanded = !expanded"
      >
        <AppIcon :name="expanded ? 'chevronDown' : 'chevronRight'" :size="14" />
        <AppIcon name="book" :size="16" />
        <span class="nav-name">{{ item.name }}</span>
      </button>
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
