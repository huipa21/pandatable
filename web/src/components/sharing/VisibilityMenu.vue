<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useSharingStore, type ShareTarget } from '../../stores/sharing'
import AppIcon from '../AppIcon.vue'

const props = withDefaults(
  defineProps<{ target: ShareTarget; placement?: 'toolbar' | 'utility' }>(),
  { placement: 'toolbar' },
)
const { t } = useI18n()
const sharing = useSharingStore()
const menu = ref<HTMLDetailsElement>()
const trigger = ref<HTMLElement>()

function close(focus = false) {
  if (!menu.value?.open) return
  menu.value.open = false
  if (focus) trigger.value?.focus()
}

function dismiss(event: PointerEvent) {
  const target = event.target
  if (target instanceof Node && !menu.value?.contains(target)) close()
}

function keydown(event: KeyboardEvent) {
  if (event.key !== 'Escape' || !menu.value?.open) return
  event.preventDefault()
  event.stopPropagation()
  close(true)
}

function share() {
  close()
  // The modal dialog restores focus to the element focused before it opened.
  trigger.value?.focus()
  launchedFor = props.target.id
  sharing.open({
    kind: props.target.kind,
    id: props.target.id,
    name: props.target.name,
  })
}

watch(
  () => sharing.target,
  async (current) => {
    if (current || launchedFor !== props.target.id) return
    launchedFor = null
    await nextTick()
    if (trigger.value?.isConnected) trigger.value.focus()
  },
)

onMounted(() => document.addEventListener('pointerdown', dismiss))
onBeforeUnmount(() => document.removeEventListener('pointerdown', dismiss))
</script>

<script lang="ts">
// Shared across instances so a remounted opener for the same target can take focus back.
let launchedFor: string | null = null
</script>

<template>
  <details
    ref="menu"
    class="visibility-menu"
    :class="`visibility-menu--${placement}`"
    @keydown="keydown"
  >
    <summary
      ref="trigger"
      :aria-label="t('grid.visibilityMenu.label')"
      :title="t('grid.visibilityMenu.label')"
    >
      <AppIcon name="eye" />
      <span>{{ t('grid.visibilityMenu.label') }}</span>
    </summary>
    <div
      class="visibility-menu-panel"
      role="group"
      :aria-label="t('grid.visibilityMenu.actions')"
    >
      <button type="button" @click="share">
        {{ t('sharing.open') }}
      </button>
    </div>
  </details>
</template>

<style scoped>
.visibility-menu {
  position: relative;
}

.visibility-menu > summary {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-height: 40px;
  padding: 8px 10px;
  color: var(--text);
  background: transparent;
  border: 1px solid transparent;
  border-radius: 6px;
  cursor: pointer;
  list-style: none;
}

.visibility-menu > summary::-webkit-details-marker {
  display: none;
}

.visibility-menu[open] > summary,
.visibility-menu > summary:hover {
  background: color-mix(in srgb, var(--accent) 10%, transparent);
}

.visibility-menu--utility > summary {
  min-height: 0;
  padding: 6px 10px;
  background: var(--panel-surface);
  border-color: var(--border);
}

.visibility-menu-panel {
  position: absolute;
  z-index: 5;
  top: 100%;
  left: 0;
  display: grid;
  gap: 4px;
  min-width: 160px;
  max-width: calc(100vw - 32px);
  max-height: 65vh;
  padding: 8px;
  overflow: auto;
  background: var(--panel-surface);
  color: var(--text);
  border: 1px solid var(--border);
  border-radius: 8px;
  box-shadow: 0 8px 24px rgb(0 0 0 / 14%);
}

.visibility-menu--utility .visibility-menu-panel {
  right: 0;
  left: auto;
}

.visibility-menu-panel button {
  min-height: 40px;
  text-align: start;
}
</style>
