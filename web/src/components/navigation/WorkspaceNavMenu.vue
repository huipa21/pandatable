<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useNavStore, type NavigationItem } from '../../stores/nav'
import AppIcon from '../AppIcon.vue'
import { useNavActionMenu } from './useNavActionMenu'

const props = defineProps<{ workspace: NavigationItem }>()
const { t } = useI18n()
const nav = useNavStore()
const root = ref<HTMLDetailsElement>()
const { close, onToggle } = useNavActionMenu(root)

function createBook() {
  close()
  nav.editor = {
    operation: 'create',
    kind: 'book',
    parentId: props.workspace.id,
  }
}

function editWorkspace() {
  close()
  nav.editor = {
    operation: 'rename',
    kind: 'workspace',
    id: props.workspace.id,
    name: props.workspace.name,
  }
}

function onMenuKey(event: KeyboardEvent) {
  if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return
  const buttons = [
    ...(root.value?.querySelectorAll<HTMLButtonElement>(
      '.nav-actions button:not(:disabled)',
    ) ?? []),
  ]
  if (buttons.length === 0) return

  const index = buttons.findIndex((button) => button === event.target)
  const offset = event.key === 'ArrowDown' ? 1 : -1
  const next =
    index < 0
      ? event.key === 'ArrowDown'
        ? 0
        : buttons.length - 1
      : (index + offset + buttons.length) % buttons.length
  event.preventDefault()
  buttons[next]?.focus()
}
</script>

<template>
  <details
    v-if="workspace.canCreate || workspace.canEdit"
    ref="root"
    class="node-actions workspace-nav-menu"
    @toggle="onToggle"
    @keydown="onMenuKey"
  >
    <summary
      :aria-label="t('wmb.workspaceActions', { name: workspace.name })"
      :title="t('wmb.workspaceActions', { name: workspace.name })"
    >
      <AppIcon name="moreVertical" :size="16" />
    </summary>
    <div class="nav-actions">
      <button
        v-if="workspace.canCreate"
        :disabled="nav.busy"
        @click="createBook"
      >
        {{ t('nav.create.book') }}
      </button>
      <button
        v-if="workspace.canEdit"
        :disabled="nav.busy"
        @click="editWorkspace"
      >
        {{ t('wmb.editWorkspace') }}
      </button>
    </div>
  </details>
</template>
