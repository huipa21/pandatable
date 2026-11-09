<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { createResourceLabel } from '../../i18n'
import { useRouter } from 'vue-router'
import { errorMessage, RequestError } from '../../api/client'
import { useNavStore, type ResourceDetails } from '../../stores/nav'
import type { BookIconName } from '../../api/types'
import IconPicker from '../common/IconPicker.vue'

const { t } = useI18n()
const router = useRouter()
const nav = useNavStore()
const dialog = ref<HTMLDialogElement>()
const name = ref('')
const icon = ref<BookIconName | null>(null)
const description = ref('')
const error = ref<unknown>(null)
const workspace = computed(() => {
  const action = nav.editor
  if (!action || action.kind !== 'workspace' || !action.id) return undefined
  return nav.tree.workspaces.find((item) => item.id === action.id)
})
const workspaceEditor = computed(
  () => nav.editor?.kind === 'workspace' && nav.editor.operation !== 'delete',
)
const bookEditor = computed(
  () => nav.editor?.kind === 'book' && nav.editor.operation !== 'delete',
)
const descriptionEditor = computed(
  () => workspaceEditor.value || bookEditor.value,
)
const descriptionLength = computed(() => [...description.value].length)
const descriptionInvalid = computed(() => descriptionLength.value > 500)
const inlineError = computed(() => {
  if (
    error.value instanceof RequestError &&
    error.value.code === 'ERR_VALIDATION'
  )
    return workspaceEditor.value
      ? t('dashboard.ds06.workspaceValidationError')
      : bookEditor.value
        ? t('wmb.bookValidationError')
        : errorMessage(error.value)
  return errorMessage(error.value)
})
const submitDisabled = computed(() => {
  const action = nav.editor
  return (
    nav.busy ||
    (action?.operation !== 'delete' &&
      action !== null &&
      (invalid.value || (descriptionEditor.value && descriptionInvalid.value)))
  )
})
const resourceDetails = computed<ResourceDetails | undefined>(() => {
  const action = nav.editor
  if (!action || action.operation === 'delete') return undefined
  if (action.kind === 'workspace') {
    const details: ResourceDetails = {}
    if (action.operation === 'create') {
      if (icon.value !== null) details.icon = icon.value
      if (description.value !== '') details.description = description.value
    } else {
      if (icon.value !== (workspace.value?.icon ?? null))
        details.icon = icon.value
      if (description.value !== (workspace.value?.description ?? ''))
        details.description = description.value
    }
    return details
  }
  if (action.kind !== 'book') return undefined
  const currentDescription =
    action.id === undefined
      ? ''
      : (nav.flatItems.find(
          (item) => item.id === action.id && item.kind === 'book',
        )?.description ?? '')
  if (action.operation === 'create')
    return description.value === ''
      ? undefined
      : { description: description.value }
  return description.value === currentDescription
    ? undefined
    : { description: description.value }
})
const invalid = computed(
  () =>
    [...name.value.trim()].length < 1 ||
    [...name.value.trim()].length > 100 ||
    /[\p{Cc}]/u.test(name.value),
)
watch(
  () => nav.editor,
  async (action) => {
    error.value = null
    name.value = action?.name ?? ''
    icon.value =
      action?.kind === 'workspace' ? (workspace.value?.icon ?? null) : null
    description.value =
      action?.kind === 'workspace'
        ? (workspace.value?.description ?? '')
        : action?.kind === 'book'
          ? (nav.flatItems.find(
              (item) => item.id === action.id && item.kind === 'book',
            )?.description ?? '')
          : ''
    await nextTick()
    if (action) dialog.value?.showModal()
    else dialog.value?.close()
  },
)
function close() {
  if (!nav.busy) nav.editor = null
}
async function save() {
  const action = nav.editor
  if (
    !action ||
    nav.busy ||
    (action.operation !== 'delete' &&
      (invalid.value || (descriptionEditor.value && descriptionInvalid.value)))
  )
    return
  try {
    const path = await nav.submit(
      action,
      name.value.trim(),
      resourceDetails.value,
    )
    nav.editor = null
    if (path) await router.push(path)
  } catch (cause) {
    error.value = cause
  }
}
</script>

<template>
  <dialog
    ref="dialog"
    class="structure-dialog"
    :aria-label="t('nav.dialogTitle')"
    @cancel.prevent="close"
  >
    <form v-if="nav.editor" @submit.prevent="save">
      <h2>
        {{
          nav.editor.operation === 'create'
            ? createResourceLabel(nav.editor.kind)
            : nav.editor.operation === 'rename'
              ? nav.editor.kind === 'workspace'
                ? t('dashboard.ds06.editWorkspace')
                : nav.editor.kind === 'book'
                  ? t('wmb.editBook')
                  : t('nav.rename')
              : t('nav.delete')
        }}
      </h2>
      <p v-if="nav.editor.operation === 'delete'">
        {{ t('nav.deleteNotice', { name: nav.editor.name }) }}
      </p>
      <label v-else class="structure-name">
        <span>{{ t('nav.name') }}</span>
        <input
          v-model="name"
          autofocus
          :aria-label="t('nav.name')"
          :disabled="nav.busy"
          :aria-describedby="'structure-name-policy'"
        />
        <span id="structure-name-policy">{{ t('nav.namePolicy') }}</span>
      </label>
      <template v-if="workspaceEditor">
        <div class="workspace-icon-field">
          <span>{{ t('dashboard.ds06.iconLabel') }}</span>
          <IconPicker v-model="icon" />
        </div>
      </template>
      <template v-if="descriptionEditor">
        <label class="structure-name workspace-description-field">
          <span>{{
            bookEditor
              ? t('wmb.bookDescription')
              : t('dashboard.ds06.descriptionLabel')
          }}</span>
          <textarea
            v-model="description"
            :disabled="nav.busy"
            :aria-label="
              bookEditor
                ? t('wmb.bookDescription')
                : t('dashboard.ds06.descriptionLabel')
            "
            :aria-invalid="descriptionInvalid"
            :aria-describedby="
              descriptionInvalid
                ? 'workspace-description-counter workspace-description-error'
                : 'workspace-description-counter'
            "
            rows="3"
          />
          <span id="workspace-description-counter">
            {{
              t('dashboard.ds06.descriptionCounter', {
                count: descriptionLength,
              })
            }}
          </span>
          <span
            v-if="descriptionInvalid"
            id="workspace-description-error"
            class="field-error"
          >
            {{ t('dashboard.ds06.descriptionTooLong') }}
          </span>
        </label>
      </template>
      <p v-if="error" role="alert">{{ inlineError }}</p>
      <div class="nav-actions">
        <button type="submit" :disabled="submitDisabled">
          {{ t('admin.confirm') }}
        </button>
        <button type="button" :disabled="nav.busy" @click="close">
          {{ t('admin.cancel') }}
        </button>
      </div>
    </form>
  </dialog>
</template>
