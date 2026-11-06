<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { createResourceLabel } from '../../i18n'
import { useNavStore, type NavigationItem } from '../../stores/nav'
import { useSharingStore } from '../../stores/sharing'
import { useViewManagementStore } from '../../stores/viewManagement'
import AppIcon from '../AppIcon.vue'
import { useNavActionMenu } from './useNavActionMenu'

const props = defineProps<{ item: NavigationItem }>()
const emit = defineEmits<{
  move: [item: NavigationItem, index: number]
}>()
const { t } = useI18n()
const route = useRoute()
const nav = useNavStore()
const sharing = useSharingStore()
const views = useViewManagementStore()
const root = ref<HTMLDetailsElement>()
const { close, onToggle } = useNavActionMenu(root)
const canShare = computed(
  () => !props.item.pathOnly && props.item.level === 'owner',
)
const canSetDefault = computed(
  () =>
    props.item.kind === 'workspace' &&
    !props.item.pathOnly &&
    props.item.level !== 'none',
)
const isDefault = computed(
  () => nav.preferenceLoaded && nav.preferredWorkspaceId === props.item.id,
)
const canCreateView = computed(
  () => props.item.kind === 'table' && props.item.level !== 'none',
)
const hasActions = computed(
  () =>
    props.item.canEdit ||
    props.item.canCreate ||
    canShare.value ||
    canSetDefault.value ||
    canCreateView.value,
)

async function setDefault() {
  close(true)
  try {
    await nav.setPreferredWorkspace(props.item.id)
  } catch {
    // The store exposes the failure and retry beside the tree.
  }
}

function createView() {
  const origin = route.fullPath
  close(true)
  void views.beginCreate(props.item.id, () => route.fullPath === origin)
}
</script>

<template>
  <details v-if="hasActions" ref="root" class="node-actions" @toggle="onToggle">
    <summary :title="t('nav.actions', { name: item.name })">
      <AppIcon name="more" :size="16" />
      <span class="visually-hidden">{{
        t('nav.actions', { name: item.name })
      }}</span>
    </summary>
    <div class="nav-actions">
      <button
        v-if="canSetDefault"
        :disabled="nav.preferenceBusy || isDefault"
        @click="setDefault"
      >
        {{ t('workspaceGrid.ux02.setDefault') }}
      </button>
      <button v-if="canShare" @click="sharing.open(item)">
        {{ t('sharing.open') }}
      </button>
      <button v-if="canCreateView" :disabled="views.busy" @click="createView">
        {{ t('nav.createView') }}
      </button>
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
</template>
