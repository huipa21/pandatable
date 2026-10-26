<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { createResourceLabel } from '../../i18n'
import { useRouter } from 'vue-router'
import { errorMessage } from '../../api/client'
import { useNavStore } from '../../stores/nav'

const { t } = useI18n()
const router = useRouter()
const nav = useNavStore()
const dialog = ref<HTMLDialogElement>()
const name = ref('')
const error = ref<unknown>(null)
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
  if (!action || nav.busy || (action.operation !== 'delete' && invalid.value))
    return
  try {
    const path = await nav.submit(action, name.value.trim())
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
              ? t('nav.rename')
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
      <p v-if="error" role="alert">{{ errorMessage(error) }}</p>
      <div class="nav-actions">
        <button
          type="submit"
          :disabled="nav.busy || (nav.editor.operation !== 'delete' && invalid)"
        >
          {{ t('admin.confirm') }}
        </button>
        <button type="button" :disabled="nav.busy" @click="close">
          {{ t('admin.cancel') }}
        </button>
      </div>
    </form>
  </dialog>
</template>
