<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { errorMessage } from '../../api/client'
import type { NavView } from '../../api/types'
import type { NavigationItem } from '../../stores/nav'
import {
  canManageView,
  useViewManagementStore,
  type ViewScope,
} from '../../stores/viewManagement'
import AppIcon from '../AppIcon.vue'
import { useNavActionMenu } from './useNavActionMenu'

const props = defineProps<{ table: NavigationItem; view: NavView }>()
const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const views = useViewManagementStore()
const root = ref<HTMLDetailsElement>()
const { close, onToggle: toggleMenu } = useNavActionMenu(root)
const selected = computed(() => {
  if (
    views.target?.tableId !== props.table.id ||
    views.target.viewId !== props.view.id
  )
    return undefined
  return views.schema?.views.find((item) => item.id === props.view.id)
})
const manageable = computed(
  () =>
    !!selected.value && canManageView(selected.value, views.schema?.myLevel),
)
const canShare = computed(
  () => views.schema?.myLevel === 'write' || views.schema?.myLevel === 'owner',
)
const index = computed(
  () =>
    views.schema?.views.findIndex((item) => item.id === props.view.id) ?? -1,
)

function onToggle() {
  toggleMenu()
  if (!root.value?.open) return
  if (views.busy) {
    if (
      views.target?.tableId !== props.table.id ||
      views.target.viewId !== props.view.id
    )
      close(true)
    return
  }
  const origin = route.fullPath
  void views.select(
    props.table.id,
    props.view.id,
    () => route.fullPath === origin,
  )
}

function edit(operation: 'rename' | 'delete') {
  close(true)
  views.beginEdit(operation)
}

async function duplicate(scope: ViewScope) {
  if (!selected.value) return
  const origin = route.fullPath
  const tableId = props.table.id
  const name = t('nav.viewCopyName', { name: selected.value.name })
  const result = await views.duplicate(name, scope)
  if (!result || route.fullPath !== origin) return
  close(true)
  await router.push(`/t/${tableId}/v/${result.view.id}`)
}

function move(direction: 'up' | 'down') {
  const ordered = views.schema?.views
  if (!ordered || index.value < 0) return
  const beforeId =
    direction === 'up'
      ? ordered[index.value - 1]?.id
      : (ordered[index.value + 2]?.id ?? null)
  if (beforeId === undefined) return
  void views.move(beforeId)
}
</script>

<template>
  <details ref="root" class="node-actions" @toggle="onToggle">
    <summary :title="t('workspaceGrid.ux05.viewActions', { name: view.name })">
      <AppIcon name="more" :size="16" />
      <span class="visually-hidden">
        {{ t('workspaceGrid.ux05.viewActions', { name: view.name }) }}
      </span>
    </summary>
    <div class="nav-actions">
      <p v-if="views.loading" role="status">
        {{ t('workspaceGrid.ux05.loading') }}
      </p>
      <p v-if="views.error" role="alert">{{ errorMessage(views.error) }}</p>
      <template v-if="views.loadError">
        <p role="alert">{{ errorMessage(views.loadError) }}</p>
        <button type="button" :disabled="views.loading" @click="views.retry()">
          {{ t('auth.retry') }}
        </button>
      </template>
      <template v-if="selected && !views.loadError">
        <span class="path-only">
          {{
            selected.scope === 'personal'
              ? t('nav.viewScope.personal')
              : t('nav.viewScope.shared')
          }}
        </span>
        <button
          v-if="manageable"
          type="button"
          :disabled="views.busy"
          @click="edit('rename')"
        >
          {{ t('nav.renameView') }}
        </button>
        <button
          type="button"
          :disabled="views.busy"
          @click="duplicate('personal')"
        >
          {{ t('nav.duplicatePersonalView') }}
        </button>
        <button
          v-if="manageable && canShare"
          type="button"
          :disabled="views.busy"
          @click="duplicate('shared')"
        >
          {{ t('nav.duplicateSharedView') }}
        </button>
        <button
          v-if="
            views.schema?.myLevel === 'owner' && selected.scope === 'shared'
          "
          type="button"
          :disabled="views.busy"
          @click="views.toggleLock()"
        >
          {{ selected.locked ? t('nav.unlockView') : t('nav.lockView') }}
        </button>
        <template v-if="manageable">
          <button
            type="button"
            :aria-label="t('nav.moveViewUp', { name: view.name })"
            :disabled="views.busy || index <= 0"
            @click="move('up')"
          >
            {{ t('nav.moveUp') }}
          </button>
          <button
            type="button"
            :aria-label="t('nav.moveViewDown', { name: view.name })"
            :disabled="
              views.busy || index >= (views.schema?.views.length ?? 0) - 1
            "
            @click="move('down')"
          >
            {{ t('nav.moveDown') }}
          </button>
          <button type="button" :disabled="views.busy" @click="edit('delete')">
            {{ t('nav.deleteView') }}
          </button>
        </template>
      </template>
    </div>
  </details>
</template>
