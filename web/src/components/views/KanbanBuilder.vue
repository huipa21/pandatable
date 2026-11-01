<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { KanbanConfig, TableSchema, TableView } from '../../api/types'

const props = defineProps<{
  schema: TableSchema
  view: TableView
  busy?: boolean
}>()
const emit = defineEmits<{ save: [config: KanbanConfig] }>()
const { t } = useI18n()
const draft = ref<KanbanConfig>({ groupField: null, fields: [] })
const dirty = ref(false)
let viewId = ''

watch(
  () => [props.view.id, props.view.version, props.view.config.kanban] as const,
  () => {
    const config = props.view.config.kanban ?? { groupField: null, fields: [] }
    if (
      viewId !== props.view.id ||
      !dirty.value ||
      JSON.stringify(draft.value) === JSON.stringify(config)
    ) {
      draft.value = {
        groupField: config.groupField,
        fields: [...config.fields],
      }
      dirty.value = false
    }
    viewId = props.view.id
  },
  { immediate: true },
)

const fields = computed(() =>
  [...props.schema.fields].sort((a, b) => a.position - b.position),
)
const groupingFields = computed(() =>
  fields.value.filter((field) =>
    ['single_select', 'user'].includes(field.uiType),
  ),
)

function toggleField(id: string, checked: boolean) {
  draft.value.fields = checked
    ? [...draft.value.fields.filter((item) => item !== id), id]
    : draft.value.fields.filter((item) => item !== id)
  dirty.value = true
}

function save() {
  if (props.busy) return
  emit('save', {
    groupField: draft.value.groupField,
    fields: [...draft.value.fields],
  })
}
</script>

<template>
  <details class="kanban-settings">
    <summary>{{ t('kanban.settings') }}</summary>
    <form @submit.prevent="save">
      <label>
        {{ t('kanban.groupField') }}
        <select
          v-model="draft.groupField"
          :disabled="busy"
          @change="dirty = true"
        >
          <option :value="null">{{ t('kanban.chooseField') }}</option>
          <option
            v-for="field in groupingFields"
            :key="field.id"
            :value="field.id"
          >
            {{ field.name }}
          </option>
        </select>
      </label>
      <fieldset :disabled="busy">
        <legend>{{ t('kanban.cardFields') }}</legend>
        <label v-for="field in fields" :key="field.id" class="field-choice">
          <input
            type="checkbox"
            :checked="draft.fields.includes(field.id)"
            @change="
              toggleField(field.id, ($event.target as HTMLInputElement).checked)
            "
          />
          {{ field.name }}
        </label>
      </fieldset>
      <button type="submit" :disabled="busy">{{ t('kanban.save') }}</button>
    </form>
  </details>
</template>

<style scoped>
.kanban-settings,
form,
fieldset {
  display: grid;
  gap: 12px;
}
.kanban-settings {
  margin: 16px 0;
  padding: 12px;
  border: 1px solid var(--border);
  border-radius: 8px;
}
fieldset {
  min-width: 0;
  padding: 12px;
  border: 1px solid var(--border);
}
label {
  display: grid;
  gap: 4px;
}
.field-choice {
  display: flex;
  align-items: center;
  gap: 8px;
}
select,
button {
  min-width: 0;
  padding: 8px;
  color: var(--text);
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 4px;
}
@media (max-width: 767px) {
  select,
  button {
    min-height: 44px;
    font-size: 16px;
  }
}
</style>
