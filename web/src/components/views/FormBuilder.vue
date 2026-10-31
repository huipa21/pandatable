<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type {
  FormConfig,
  FormFieldConfig,
  TableSchema,
  TableView,
} from '../../api/types'
import { bindField } from '../../fields/registry'

const props = defineProps<{
  schema: TableSchema
  view: TableView
  busy?: boolean
}>()
const emit = defineEmits<{ save: [form: FormConfig] }>()
const { t } = useI18n()
const draft = ref<FormConfig>(emptyForm())

function emptyForm(): FormConfig {
  return {
    title: '',
    description: '',
    fields: [],
    afterSubmit: 'message',
  }
}

watch(
  () => [props.view.id, props.view.version, props.view.config.form] as const,
  () => {
    const form = props.view.config.form
    draft.value = form
      ? {
          ...form,
          fields: form.fields.map((field) => ({ ...field })),
        }
      : emptyForm()
  },
  { immediate: true },
)

const writableFields = computed(() =>
  [...props.schema.fields]
    .sort((left, right) => left.position - right.position)
    .filter((field) => {
      if (field.isSystem || field.uiType === 'link') return false
      const bound = bindField(field.uiType, field.options)
      return bound.ok && !bound.value.readOnly
    }),
)
const selectedIDs = computed(
  () => new Set(draft.value.fields.map((field) => field.id)),
)
const fieldByID = computed(
  () => new Map(props.schema.fields.map((field) => [field.id, field])),
)

function toggleField(id: string, selected: boolean) {
  if (selected) {
    if (selectedIDs.value.has(id)) return
    const field = fieldByID.value.get(id)
    if (!field) return
    draft.value.fields.push({
      id,
      label: field.name,
      help: '',
      required: false,
    })
  } else {
    draft.value.fields = draft.value.fields.filter((field) => field.id !== id)
  }
}

function moveField(index: number, offset: -1 | 1) {
  const target = index + offset
  if (target < 0 || target >= draft.value.fields.length) return
  const fields: FormFieldConfig[] = [...draft.value.fields]
  ;[fields[index], fields[target]] = [fields[target]!, fields[index]!]
  draft.value.fields = fields
}

function save() {
  if (props.busy) return
  emit('save', {
    ...draft.value,
    fields: draft.value.fields.map((field) => ({ ...field })),
  })
}
</script>

<template>
  <details class="form-builder">
    <summary>{{ t('form.builderTitle') }}</summary>
    <form @submit.prevent="save">
      <label>
        {{ t('form.title') }}
        <input v-model="draft.title" :disabled="busy" />
      </label>
      <label>
        {{ t('form.description') }}
        <textarea v-model="draft.description" :disabled="busy" />
      </label>
      <fieldset :disabled="busy">
        <legend>{{ t('form.availableFields') }}</legend>
        <label
          v-for="field in writableFields"
          :key="field.id"
          class="field-choice"
        >
          <input
            type="checkbox"
            :checked="selectedIDs.has(field.id)"
            @change="
              toggleField(field.id, ($event.target as HTMLInputElement).checked)
            "
          />
          {{ field.name }}
        </label>
        <p v-if="writableFields.length === 0">
          {{ t('form.noWritableFields') }}
        </p>
      </fieldset>
      <fieldset :disabled="busy">
        <legend>{{ t('form.selectedFields') }}</legend>
        <section
          v-for="(item, index) in draft.fields"
          :key="item.id"
          class="selected-field"
        >
          <h3>{{ fieldByID.get(item.id)?.name ?? item.label }}</h3>
          <label>
            {{ t('form.fieldLabel') }}
            <input v-model="item.label" />
          </label>
          <label>
            {{ t('form.fieldHelp') }}
            <textarea v-model="item.help" />
          </label>
          <label class="required-choice">
            <input v-model="item.required" type="checkbox" />
            {{ t('form.required') }}
          </label>
          <div class="field-order">
            <button
              type="button"
              :aria-label="t('form.moveUp', { name: item.label })"
              :disabled="index === 0"
              @click="moveField(index, -1)"
            >
              {{ t('nav.moveUp') }}
            </button>
            <button
              type="button"
              :aria-label="t('form.moveDown', { name: item.label })"
              :disabled="index === draft.fields.length - 1"
              @click="moveField(index, 1)"
            >
              {{ t('nav.moveDown') }}
            </button>
          </div>
        </section>
        <p v-if="draft.fields.length === 0">{{ t('form.noSelectedFields') }}</p>
      </fieldset>
      <label>
        {{ t('form.afterSubmit') }}
        <select v-model="draft.afterSubmit" :disabled="busy">
          <option value="message">{{ t('form.afterSubmitMessage') }}</option>
          <option value="new">{{ t('form.afterSubmitNew') }}</option>
        </select>
      </label>
      <button type="submit" :disabled="busy">{{ t('form.save') }}</button>
    </form>
  </details>
</template>

<style scoped>
.form-builder,
form,
fieldset,
.selected-field {
  display: grid;
  gap: 12px;
}
.form-builder {
  margin: 16px 0;
  padding: 12px;
  border: 1px solid var(--border);
  border-radius: 8px;
}
form,
fieldset {
  min-width: 0;
}
fieldset {
  padding: 12px;
  border: 1px solid var(--border);
}
label {
  display: grid;
  gap: 4px;
}
.field-choice,
.required-choice {
  display: flex;
  align-items: center;
  gap: 8px;
}
.selected-field {
  padding: 12px 0;
  border-top: 1px solid var(--border);
}
.field-order {
  display: flex;
  gap: 8px;
}
input,
textarea,
select,
button {
  min-width: 0;
  padding: 8px;
  color: var(--text);
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 4px;
}
textarea {
  min-height: 64px;
}
@media (max-width: 767px) {
  button,
  input,
  select {
    min-height: 44px;
    font-size: 16px;
  }
}
</style>
