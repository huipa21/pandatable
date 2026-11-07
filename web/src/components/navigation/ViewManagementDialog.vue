<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { errorMessage } from '../../api/client'
import { useAuthStore } from '../../stores/auth'
import {
  useViewManagementStore,
  type ViewScope,
  type ViewType,
} from '../../stores/viewManagement'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const views = useViewManagementStore()
const dialog = ref<HTMLDialogElement>()
const nameInput = ref<HTMLInputElement>()
const name = ref('')
const scope = ref<ViewScope>('personal')
const type = ref<ViewType>('grid')
let opener: HTMLElement | null = null
let fallback: HTMLElement | null = null
let openedRoute = ''
let openedIdentity: string | undefined

const selected = computed(() =>
  views.schema?.views.find((item) => item.id === views.editor?.viewId),
)
const title = computed(() =>
  views.editor?.operation === 'rename'
    ? t('nav.renameView')
    : views.editor?.operation === 'delete'
      ? t('nav.deleteView')
      : t('nav.createView'),
)
const invalidName = computed(
  () =>
    [...name.value.trim()].length < 1 ||
    [...name.value.trim()].length > 100 ||
    /[\p{Cc}]/u.test(name.value),
)
const canCreateShared = computed(
  () => views.schema?.myLevel === 'write' || views.schema?.myLevel === 'owner',
)
const submitDisabled = computed(() => {
  const action = views.editor
  return (
    !action ||
    !views.schema ||
    views.busy ||
    views.loading ||
    (action.operation === 'create' &&
      scope.value === 'shared' &&
      !canCreateShared.value) ||
    (action.operation !== 'delete' && invalidName.value)
  )
})

watch(
  () => views.editor,
  async (action) => {
    if (action) {
      opener =
        document.activeElement instanceof HTMLElement
          ? document.activeElement
          : null
      fallback =
        opener
          ?.closest('[data-kind="table"]')
          ?.querySelector<HTMLElement>('.table-toggle') ?? null
      openedRoute = route.fullPath
      openedIdentity = auth.user?.id
      name.value =
        action.operation === 'rename' ? (selected.value?.name ?? '') : ''
      scope.value = 'personal'
      type.value = 'grid'
      await nextTick()
      if (views.editor === action && !dialog.value?.open)
        dialog.value?.showModal()
    } else if (dialog.value?.open) {
      dialog.value.close()
      await nextTick()
      if (route.fullPath === openedRoute && auth.user?.id === openedIdentity) {
        const destination = opener?.isConnected ? opener : fallback
        if (destination?.isConnected) destination.focus()
      }
    }
  },
)
watch(
  () => views.loading,
  async (loading) => {
    if (loading || !views.editor || views.editor.operation === 'delete') return
    await nextTick()
    if (dialog.value?.open && !nameInput.value?.disabled)
      nameInput.value?.focus()
  },
)
watch(() => route.fullPath, views.reset, { flush: 'sync' })

function close() {
  if (!views.busy) views.dismissEditor()
}

async function save() {
  const action = views.editor
  if (!action || submitDisabled.value) return
  const origin = route.fullPath
  const identity = auth.user?.id
  if (action.operation === 'create') {
    const result = await views.create(
      name.value.trim(),
      type.value,
      scope.value,
    )
    if (!result || route.fullPath !== origin || auth.user?.id !== identity)
      return
    views.dismissEditor()
    await nextTick()
    if (route.fullPath === origin && auth.user?.id === identity)
      await router.push(`/t/${action.tableId}/v/${result.view.id}`)
  } else if (action.operation === 'rename') {
    const result = await views.rename(name.value.trim())
    if (result && route.fullPath === origin && auth.user?.id === identity)
      views.dismissEditor()
  } else {
    const wasActive =
      route.params.tableId === action.tableId &&
      route.params.viewId === action.viewId
    if (
      !(await views.remove()) ||
      route.fullPath !== origin ||
      auth.user?.id !== identity
    )
      return
    const next = views.schema?.views[0]
    views.dismissEditor()
    await nextTick()
    if (wasActive && route.fullPath === origin && auth.user?.id === identity) {
      await router.replace(
        next ? `/t/${action.tableId}/v/${next.id}` : `/t/${action.tableId}`,
      )
      await nextTick()
      const tableToggle = document.querySelector<HTMLElement>(
        `[data-nav-id="${action.tableId}"] > .nav-row .table-toggle`,
      )
      if (
        route.params.tableId === action.tableId &&
        auth.user?.id === identity &&
        tableToggle?.isConnected
      )
        tableToggle.focus()
    }
  }
}
</script>

<template>
  <dialog
    ref="dialog"
    class="structure-dialog view-dialog"
    :aria-label="title"
    @cancel.prevent="close"
  >
    <form v-if="views.editor" @submit.prevent="save">
      <h2>{{ title }}</h2>
      <p v-if="views.loading" role="status">
        {{ t('workspaceGrid.ux05.loading') }}
      </p>
      <p v-if="views.editor.operation === 'delete' && selected">
        {{ t('nav.confirmDeleteView', { name: selected.name }) }}
      </p>
      <label
        v-else-if="views.editor.operation !== 'delete'"
        class="structure-name"
      >
        <span>{{ t('nav.viewName') }}</span>
        <input
          ref="nameInput"
          v-model="name"
          autofocus
          :disabled="views.busy || views.loading || !views.schema"
          :aria-label="t('nav.viewName')"
          aria-describedby="view-name-policy"
        />
        <span id="view-name-policy">{{ t('nav.namePolicy') }}</span>
      </label>
      <template v-if="views.editor.operation === 'create'">
        <label class="structure-name">
          <span>{{ t('nav.viewScopeLabel') }}</span>
          <select
            v-model="scope"
            :disabled="views.busy || views.loading || !views.schema"
          >
            <option value="personal">{{ t('nav.viewScope.personal') }}</option>
            <option value="shared" :disabled="!canCreateShared">
              {{ t('nav.viewScope.shared') }}
            </option>
          </select>
        </label>
        <label class="structure-name">
          <span>{{ t('form.viewType') }}</span>
          <select
            v-model="type"
            :disabled="views.busy || views.loading || !views.schema"
          >
            <option value="grid">{{ t('form.gridType') }}</option>
            <option value="form">{{ t('form.formType') }}</option>
            <option value="kanban">{{ t('kanban.viewType') }}</option>
            <option value="gallery">{{ t('gallery.viewType') }}</option>
          </select>
        </label>
      </template>
      <p v-if="views.error" role="alert">{{ errorMessage(views.error) }}</p>
      <template v-if="views.loadError">
        <p role="alert">{{ errorMessage(views.loadError) }}</p>
        <button type="button" :disabled="views.loading" @click="views.retry()">
          {{ t('auth.retry') }}
        </button>
      </template>
      <div class="nav-actions">
        <button type="submit" :disabled="submitDisabled">
          {{
            views.editor.operation === 'create'
              ? t('nav.createView')
              : t('admin.confirm')
          }}
        </button>
        <button type="button" :disabled="views.busy" @click="close">
          {{ t('admin.cancel') }}
        </button>
      </div>
    </form>
  </dialog>
</template>
