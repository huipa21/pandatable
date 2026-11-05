<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { createResourceLabel } from '../../i18n'
import { useNavStore, type NavigationItem } from '../../stores/nav'
import { useSharingStore } from '../../stores/sharing'
import AppIcon from '../AppIcon.vue'

const props = defineProps<{ item: NavigationItem }>()
const emit = defineEmits<{
  move: [item: NavigationItem, index: number]
}>()
const { t } = useI18n()
const nav = useNavStore()
const sharing = useSharingStore()
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
const root = ref<HTMLDetailsElement>()

function close() {
  root.value?.removeAttribute('open')
}

function onOutside(event: Event) {
  const target = event.target as Element
  if (root.value?.contains(target)) return
  // Dialogs opened from the menu return focus to it when they close.
  if (target.closest?.('dialog, [role="dialog"], .n-modal-container')) return
  close()
}

function onKey(event: KeyboardEvent) {
  if (event.key === 'Escape' && root.value?.contains(event.target as Node)) {
    event.preventDefault()
    close()
    root.value.querySelector('summary')?.focus()
  }
}

async function setDefault() {
  close()
  root.value?.querySelector('summary')?.focus()
  try {
    await nav.setPreferredWorkspace(props.item.id)
  } catch {
    // The store exposes the failure and retry beside the tree.
  }
}

function listen(on: boolean) {
  if (on) {
    document.addEventListener('pointerdown', onOutside, true)
    document.addEventListener('keydown', onKey)
  } else {
    document.removeEventListener('pointerdown', onOutside, true)
    document.removeEventListener('keydown', onKey)
  }
}

// Only one action menu is open at a time so dropdowns never overlap.
function onToggle() {
  const open = root.value?.open ?? false
  listen(open)
  if (!open) return
  for (const other of document.querySelectorAll('details.node-actions[open]'))
    if (other !== root.value) other.removeAttribute('open')
}

onBeforeUnmount(() => listen(false))
</script>

<template>
  <details
    v-if="item.canEdit || item.canCreate || canShare || canSetDefault"
    ref="root"
    class="node-actions"
    @toggle="onToggle"
  >
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
