<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { errorMessage, request, RequestError } from '../../api/client'
import type { DataRow, RowPatchMutation, TableSchema } from '../../api/types'
import { bindField } from '../../fields/registry'
import { isWireValue, type FieldEnv, type WireValue } from '../../fields/types'
import { useAuthStore } from '../../stores/auth'
import { useNavStore } from '../../stores/nav'
import FieldEditor from '../fields/FieldEditor.vue'
import FieldRenderer from '../fields/FieldRenderer.vue'

const props = defineProps<{
  schema: TableSchema
  viewId: string
  rowId: string
  identity: string
}>()
const emit = defineEmits<{ close: []; saved: []; schemaChanged: [] }>()
const { t, locale } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const nav = useNavStore()
const dialog = ref<HTMLDialogElement>()
const row = ref<DataRow | null>(null)
const loading = ref(false)
const saving = ref(false)
const error = ref<unknown>(null)
const editing = ref<string | null>(null)
const saved = ref(false)
let controller: AbortController | undefined
let generation = 0
let disposed = false
const env = computed<FieldEnv>(() => ({
  timeZone: auth.user?.effectiveTimezone,
  locale: locale.value,
}))
const canWrite = computed(
  () =>
    props.identity === auth.user?.id &&
    (props.schema.myLevel === 'write' || props.schema.myLevel === 'owner'),
)
const fields = computed(() =>
  [...props.schema.fields]
    .sort((a, b) => a.position - b.position)
    .map((field) => {
      const value = row.value?.fields[field.id]
      return {
        field,
        result: bindField(field.uiType, field.options),
        value: isWireValue(value)
          ? { ok: true as const, value }
          : { ok: false as const },
      }
    }),
)
const message = computed(() =>
  typeof error.value === 'string'
    ? error.value
    : error.value
      ? errorMessage(error.value)
      : '',
)

function current(token: number) {
  return !disposed && token === generation && props.identity === auth.user?.id
}

function begin() {
  controller?.abort()
  controller = new AbortController()
  return {
    token: ++generation,
    signal: AbortSignal.any([controller.signal, AbortSignal.timeout(10_000)]),
  }
}

function failure(cause: unknown) {
  error.value = cause
  saved.value = false
  if (!(cause instanceof RequestError)) return
  if (cause.code === 'ERR_PASSWORD_CHANGE_REQUIRED') {
    row.value = null
    editing.value = null
    void router.replace('/change-password')
  } else if ([401, 403, 404].includes(cause.status)) {
    row.value = null
    editing.value = null
    if (cause.status === 401) {
      auth.setUser(null)
      void router.replace('/login')
    } else {
      void nav.refresh(true)
      emit('schemaChanged')
    }
  } else if (cause.code === 'ERR_SCHEMA_CHANGED') {
    row.value = null
    editing.value = null
    emit('schemaChanged')
  }
}

async function load() {
  const { token, signal } = begin()
  row.value = null
  editing.value = null
  error.value = null
  saved.value = false
  saving.value = false
  loading.value = true
  await nextTick()
  if (!current(token)) return
  if (!dialog.value?.open) dialog.value?.showModal()
  try {
    const result = await request<DataRow>(
      `/tables/${props.schema.table.id}/rows/${encodeURIComponent(props.rowId)}`,
      'GET',
      undefined,
      signal,
    )
    if (current(token)) row.value = result
  } catch (cause) {
    if (current(token)) failure(cause)
  } finally {
    if (current(token)) loading.value = false
  }
}

async function save(value: WireValue) {
  if (!canWrite.value || saving.value || !row.value || !editing.value) return
  const item = fields.value.find((item) => item.field.id === editing.value)
  if (!item?.result.ok || item.field.isSystem || item.result.value.readOnly)
    return
  const body = { fields: { [item.field.id]: value } }
  saved.value = false
  if (new TextEncoder().encode(JSON.stringify(body)).length > 4096) {
    error.value = t('rowExpand.tooLarge')
    return
  }
  const { token, signal } = begin()
  saving.value = true
  error.value = null
  try {
    const result = await request<RowPatchMutation>(
      `/tables/${props.schema.table.id}/rows/${encodeURIComponent(props.rowId)}`,
      'PATCH',
      body,
      signal,
    )
    if (!current(token)) return
    row.value = result.row
    editing.value = null
    saved.value = true
    emit('saved')
  } catch (cause) {
    if (current(token)) failure(cause)
  } finally {
    if (current(token)) saving.value = false
  }
}

function startEditing(id: string) {
  editing.value = id
  saved.value = false
  error.value = null
}

function close() {
  generation++
  controller?.abort()
  dialog.value?.close()
  emit('close')
}

watch(
  [
    () => props.schema.table.id,
    () => props.viewId,
    () => props.rowId,
    () => props.schema.schemaVersion,
    () => props.identity,
    () => auth.user?.id,
  ],
  () => {
    if (props.identity === auth.user?.id) void load()
    else {
      generation++
      controller?.abort()
      row.value = null
      editing.value = null
      loading.value = false
      saving.value = false
      saved.value = false
      error.value = null
    }
  },
  { immediate: true },
)
onBeforeUnmount(() => {
  disposed = true
  generation++
  controller?.abort()
  dialog.value?.close()
})
</script>

<template>
  <dialog
    ref="dialog"
    class="row-expand"
    :aria-label="t('rowExpand.title', { id: rowId })"
    @cancel.prevent="close"
  >
    <header>
      <h2>{{ t('rowExpand.title', { id: rowId }) }}</h2>
      <button type="button" @click="close">{{ t('rowExpand.close') }}</button>
    </header>
    <p v-if="loading" role="status">{{ t('grid.loading') }}</p>
    <p v-if="message" role="alert">{{ message }}</p>
    <button v-if="error && !saving" type="button" @click="load">
      {{ t('auth.retry') }}
    </button>
    <p v-if="saved" role="status">{{ t('rowExpand.saved') }}</p>
    <template v-if="row">
      <p v-if="!canWrite">{{ t('rowExpand.readOnly') }}</p>
      <p>{{ t('rowExpand.hint') }}</p>
      <section
        v-for="item in fields"
        :key="item.field.id"
        class="row-field"
        :aria-label="item.field.name"
        :data-field-id="item.field.id"
      >
        <h3>{{ item.field.name }}</h3>
        <p v-if="item.field.description">{{ item.field.description }}</p>
        <template v-if="item.result.ok && item.value.ok">
          <template v-if="editing === item.field.id && canWrite">
            <fieldset :disabled="saving">
              <legend>{{ item.field.name }}</legend>
              <FieldEditor
                :field="item.result.value"
                :value="item.value.value"
                :env="env"
                :submit-label="t('rowExpand.save')"
                @commit="save"
                @cancel="editing = null"
              />
              <button type="button" @click="editing = null">
                {{ t('admin.cancel') }}
              </button>
            </fieldset>
          </template>
          <template v-else>
            <FieldRenderer
              :field="item.result.value"
              :value="item.value.value"
              :env="env"
            />
            <button
              v-if="
                canWrite && !item.field.isSystem && !item.result.value.readOnly
              "
              type="button"
              :disabled="saving"
              @click="startEditing(item.field.id)"
            >
              {{ t('rowExpand.edit', { name: item.field.name }) }}
            </button>
            <span
              v-else-if="item.field.isSystem || item.result.value.readOnly"
              v-text="t('rowExpand.system')"
            />
          </template>
        </template>
        <p v-else role="alert">{{ t('grid.unsupportedField') }}</p>
      </section>
    </template>
  </dialog>
</template>

<style scoped>
.row-expand {
  width: min(700px, calc(100vw - 32px));
  max-height: calc(100dvh - 48px);
  overflow: auto;
  padding: 20px;
  border: 1px solid var(--border);
  border-radius: 12px;
  background: var(--surface);
  color: var(--text);
}
.row-expand::backdrop {
  background: #0008;
}
header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
h2,
h3 {
  margin: 0;
  overflow-wrap: anywhere;
}
.row-field {
  display: grid;
  gap: 8px;
  padding: 16px 0;
  border-top: 1px solid var(--border);
}
fieldset {
  min-width: 0;
}
button {
  padding: 6px 12px;
  color: var(--text);
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 4px;
  cursor: pointer;
}
button:disabled {
  cursor: wait;
  opacity: 0.6;
}
.row-field :deep(.field-value) {
  overflow-wrap: anywhere;
  white-space: pre-wrap;
}
</style>
