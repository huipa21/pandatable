<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { errorMessage, request, RequestError } from '../../api/client'
import type {
  FormConfig,
  RowPatchMutation,
  TableSchema,
  TableView,
} from '../../api/types'
import { bindField } from '../../fields/registry'
import type { FieldEnv, WireValue } from '../../fields/types'
import { userLabel } from '../../fields/userDirectory'
import { useAuthStore } from '../../stores/auth'
import { useNavStore } from '../../stores/nav'
import { useUndoStore } from '../../stores/undo'
import FieldEditor from '../fields/FieldEditor.vue'

const props = defineProps<{
  schema: TableSchema
  view: TableView
  identity: string
}>()
const emit = defineEmits<{ schemaChanged: [] }>()
const { t, locale } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const nav = useNavStore()
const undo = useUndoStore()
const emptyForm = (): FormConfig => ({
  title: '',
  description: '',
  fields: [],
  afterSubmit: 'message',
})
const form = computed(() => props.view.config.form ?? emptyForm())
const canSubmit = computed(
  () =>
    props.identity === auth.user?.id &&
    (props.schema.myLevel === 'write' || props.schema.myLevel === 'owner'),
)
const env = computed<FieldEnv>(() => ({
  timeZone: auth.user?.effectiveTimezone,
  locale: locale.value,
}))
const fields = computed(() =>
  form.value.fields.map((config) => {
    const field = props.schema.fields.find((item) => item.id === config.id)
    return {
      config,
      field,
      bound: field ? bindField(field.uiType, field.options) : null,
    }
  }),
)
function fieldSubmitLabel(uiType: string) {
  return uiType === 'attachment' || uiType === 'multi_select'
    ? t('form.applyField')
    : undefined
}
const draft = ref<Record<string, WireValue>>({})
const saving = ref(false)
const error = ref('')
const submitted = ref(false)
const editorGeneration = ref(0)
const preserveUploads = ref(false)
const invalidFields = ref(new Set<string>())
let controller: AbortController | undefined

function resetDraft() {
  const values: Record<string, WireValue> = {}
  for (const item of fields.value)
    if (item.field) values[item.field.id] = item.field.default
  draft.value = values
  invalidFields.value = new Set()
}

watch(
  () =>
    [props.view.id, props.view.version, props.schema.schemaVersion] as const,
  () => {
    resetDraft()
    error.value = ''
    submitted.value = false
    preserveUploads.value = false
  },
  { immediate: true },
)

function valueChanged(id: string, value: WireValue) {
  draft.value = { ...draft.value, [id]: value }
  const invalid = new Set(invalidFields.value)
  invalid.delete(id)
  invalidFields.value = invalid
  error.value = ''
  submitted.value = false
}

function markInvalid(id: string) {
  invalidFields.value = new Set(invalidFields.value).add(id)
}

function empty(value: WireValue | undefined) {
  return (
    value === undefined ||
    value === null ||
    (typeof value === 'string' && value.trim() === '') ||
    (Array.isArray(value) && value.length === 0) ||
    (typeof value === 'object' &&
      !Array.isArray(value) &&
      Object.keys(value).length === 0)
  )
}

function message(cause: unknown) {
  error.value = errorMessage(cause)
  submitted.value = false
  if (!(cause instanceof RequestError)) return
  if (cause.code === 'ERR_PASSWORD_CHANGE_REQUIRED') {
    void router.replace('/change-password')
    return
  }
  if (cause.status === 401) {
    auth.setUser(null)
    void router.replace('/login')
    void nav.refresh(true)
  } else if ([403, 404].includes(cause.status)) {
    void nav.refresh(true)
    emit('schemaChanged')
  }
}

async function submit() {
  if (!canSubmit.value || saving.value) return
  if (invalidFields.value.size > 0) {
    error.value = t('form.invalidFields')
    return
  }
  for (const item of fields.value) {
    if (
      item.config.required &&
      item.field &&
      empty(draft.value[item.field.id])
    ) {
      error.value = t('form.requiredField', {
        label: item.config.label || item.field.name,
      })
      return
    }
  }
  const body = { fields: draft.value }
  if (new TextEncoder().encode(JSON.stringify(body)).length > 64 * 1024) {
    error.value = t('form.submitTooLarge')
    return
  }
  controller?.abort()
  controller = new AbortController()
  saving.value = true
  error.value = ''
  submitted.value = false
  try {
    const result = await request<RowPatchMutation>(
      `/views/${encodeURIComponent(props.view.id)}/form-submit`,
      'POST',
      body,
      AbortSignal.any([controller.signal, AbortSignal.timeout(10_000)]),
    )
    if (!result.row?.id)
      throw new Error('Form submission returned an unsupported response')
    if (props.identity === auth.user?.id)
      undo.record([
        {
          kind: 'delete',
          tableId: props.schema.table.id,
          ids: [result.row.id],
        },
      ])
    preserveUploads.value = true
    submitted.value = true
    if (form.value.afterSubmit === 'new') {
      resetDraft()
      editorGeneration.value++
      submitted.value = false
      await nextTick()
      preserveUploads.value = false
    }
  } catch (cause) {
    message(cause)
  } finally {
    saving.value = false
  }
}

const resolveUser = (id: string) =>
  userLabel(props.identity, props.schema.table.id, id)

onBeforeUnmount(() => controller?.abort())
</script>

<template>
  <section class="form-view" :aria-label="view.name">
    <h2>{{ form.title || view.name }}</h2>
    <p v-if="form.description" class="form-description">
      {{ form.description }}
    </p>
    <p v-if="!canSubmit" class="read-only">{{ t('form.readOnly') }}</p>
    <p v-if="error" role="alert">{{ error }}</p>
    <p v-if="submitted" role="status">{{ t('form.submitted') }}</p>
    <form @submit.prevent="submit">
      <fieldset :disabled="!canSubmit || saving">
        <section
          v-for="item in fields"
          :key="`${editorGeneration}:${item.config.id}`"
          class="form-field"
        >
          <template
            v-if="item.field && item.bound?.ok && !item.bound.value.readOnly"
          >
            <h3>
              {{ item.config.label || item.field.name }}
              <span v-if="item.config.required" class="required-mark">
                {{ t('form.required') }}
              </span>
            </h3>
            <p v-if="item.config.help" class="field-help">
              {{ item.config.help }}
            </p>
            <FieldEditor
              :field="item.bound.value"
              :value="draft[item.field.id] ?? item.field.default"
              :env="env"
              :label="item.config.label || item.field.name"
              :required="item.config.required"
              :live="true"
              :auto-focus="false"
              :submit-label="fieldSubmitLabel(item.field.uiType)"
              :table-id="schema.table.id"
              :field-id="item.field.id"
              :identity="identity"
              :resolve-user="resolveUser"
              :preserve-uploads="preserveUploads"
              @preview="valueChanged(item.field.id, $event)"
              @commit="valueChanged(item.field.id, $event)"
              @invalid-value="markInvalid(item.field.id)"
            />
          </template>
          <p v-else role="alert">{{ t('grid.unsupportedField') }}</p>
        </section>
        <button type="submit" :disabled="!canSubmit || saving">
          {{ saving ? t('grid.saving') : t('form.submit') }}
        </button>
      </fieldset>
    </form>
  </section>
</template>

<style scoped>
.form-view {
  width: min(760px, 100%);
  margin: 20px auto;
  padding: clamp(16px, 4vw, 32px);
  border: 1px solid var(--border);
  border-radius: 12px;
  background: var(--surface);
  color: var(--text);
}
.form-view h2,
.form-view h3 {
  margin: 0;
  overflow-wrap: anywhere;
}
.form-description,
.field-help {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.form-field {
  display: grid;
  gap: 8px;
  padding: 16px 0;
  border-top: 1px solid var(--border);
}
fieldset {
  display: grid;
  gap: 12px;
  min-width: 0;
  padding: 0;
  border: 0;
}
button {
  min-height: 44px;
  padding: 8px 14px;
  color: var(--text);
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 4px;
  cursor: pointer;
}
button:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}
@media (max-width: 767px) {
  .form-view {
    margin: 12px 0;
    border-radius: 8px;
  }
  :deep(.field-editor input),
  :deep(.field-editor select),
  :deep(.field-editor textarea),
  :deep(.field-editor button) {
    min-height: 44px;
    font-size: 16px;
  }
}
</style>
