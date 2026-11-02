<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { GalleryConfig, TableSchema, TableView } from '../../api/types'
import CardFieldsPicker from './CardFieldsPicker.vue'

const props = defineProps<{
  schema: TableSchema
  view: TableView
  busy?: boolean
}>()
const emit = defineEmits<{ save: [config: GalleryConfig] }>()
const { t } = useI18n()
const draft = ref<GalleryConfig>({ coverField: null, fields: [] })
const dirty = ref(false)
let viewId = ''

watch(
  () => [props.view.id, props.view.version, props.view.config.gallery] as const,
  () => {
    const config = props.view.config.gallery ?? { coverField: null, fields: [] }
    if (
      viewId !== props.view.id ||
      !dirty.value ||
      JSON.stringify(draft.value) === JSON.stringify(config)
    ) {
      draft.value = {
        coverField: config.coverField,
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
const coverFields = computed(() =>
  fields.value.filter((field) => field.uiType === 'attachment'),
)

function setFields(fields: string[]) {
  draft.value.fields = fields
  dirty.value = true
}

function save() {
  if (props.busy) return
  emit('save', {
    coverField: draft.value.coverField,
    fields: [...draft.value.fields],
  })
}
</script>

<template>
  <details class="gallery-settings">
    <summary>{{ t('gallery.settings') }}</summary>
    <form @submit.prevent="save">
      <label>
        {{ t('gallery.coverField') }}
        <select
          v-model="draft.coverField"
          :disabled="busy"
          @change="dirty = true"
        >
          <option :value="null">{{ t('gallery.noCoverField') }}</option>
          <option
            v-for="field in coverFields"
            :key="field.id"
            :value="field.id"
          >
            {{ field.name }}
          </option>
        </select>
      </label>
      <CardFieldsPicker
        :fields="fields"
        :selected="draft.fields"
        :legend="t('gallery.cardFields')"
        :busy="busy"
        @change="setFields"
      />
      <button type="submit" :disabled="busy">{{ t('gallery.save') }}</button>
    </form>
  </details>
</template>

<style scoped>
.gallery-settings,
form {
  display: grid;
  gap: 12px;
}
.gallery-settings {
  margin: 16px 0;
  padding: 12px;
  border: 1px solid var(--border);
  border-radius: 8px;
}
label {
  display: grid;
  gap: 4px;
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
