<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { BoundField } from '../../fields/registry'
import type { ChoiceOptions } from '../../fields/choice'
import { fieldErrorMessage } from '../../fields/messages'
import type { FieldEnv, Reason, Result, WireValue } from '../../fields/types'

const props = withDefaults(
  defineProps<{ field: BoundField; value: WireValue; env?: FieldEnv }>(),
  { env: () => ({}) },
)
const emit = defineEmits<{ commit: [value: WireValue]; cancel: [] }>()
const { t } = useI18n()

const input = ref<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>()
const reason = ref<Reason | null>(null)
const draft = ref(
  props.field.readOnly ? '' : props.field.formatCopy(props.value, props.env),
)
const selected = ref<string[]>(
  typeof props.value === 'string'
    ? [props.value]
    : Array.isArray(props.value)
      ? [...props.value]
      : [],
)
const single = ref(typeof props.value === 'string' ? props.value : '')
const choices = computed(
  () => (props.field.options as ChoiceOptions).choices ?? [],
)
const multiple = computed(() => props.field.kind === 'multi_select')
const placeholder = computed(() => {
  switch (props.field.presentation) {
    case 'date':
      return t('field.editor.datePlaceholder')
    case 'datetime':
      return t('field.editor.datetimePlaceholder')
    default:
      return undefined
  }
})
const error = computed(() =>
  reason.value ? fieldErrorMessage(reason.value) : '',
)

onMounted(() => input.value?.focus())

function finish(result: Result<WireValue>) {
  if (!result.ok) {
    reason.value = result.reason
    return
  }
  reason.value = null
  emit('commit', result.value)
}

const commitText = () => finish(props.field.parsePaste(draft.value, props.env))

function commitSelect() {
  finish(props.field.normalize(multiple.value ? selected.value : single.value))
}

function commitCheckbox(event: Event) {
  finish(props.field.normalize((event.target as HTMLInputElement).checked))
}

function keydown(event: KeyboardEvent) {
  if (event.isComposing) return
  if (event.key === 'Escape') {
    event.preventDefault()
    emit('cancel')
    return
  }
  if (event.key !== 'Enter') return
  if (props.field.presentation === 'checkbox') return
  const multiline = props.field.presentation === 'multiline'
  if (multiline && !(event.ctrlKey || event.metaKey)) return
  if (!multiline && event.shiftKey) return
  event.preventDefault()
  if (props.field.presentation === 'select') commitSelect()
  else commitText()
}
</script>

<template>
  <div v-if="!field.readOnly" class="field-editor" @keydown="keydown">
    <input
      v-if="field.presentation === 'checkbox'"
      ref="input"
      type="checkbox"
      :checked="value === true"
      :aria-label="t('field.editor.value')"
      @change="commitCheckbox"
    />
    <select
      v-else-if="field.presentation === 'select' && multiple"
      ref="input"
      v-model="selected"
      multiple
      :size="Math.min(8, Math.max(2, choices.length))"
      :aria-label="t('field.editor.value')"
      :aria-invalid="reason !== null"
    >
      <option v-for="choice in choices" :key="choice.id" :value="choice.label">
        {{ choice.label }}
      </option>
    </select>
    <select
      v-else-if="field.presentation === 'select'"
      ref="input"
      v-model="single"
      :aria-label="t('field.editor.value')"
      :aria-invalid="reason !== null"
      @change="commitSelect"
    >
      <option value="">{{ t('field.editor.none') }}</option>
      <option v-for="choice in choices" :key="choice.id" :value="choice.label">
        {{ choice.label }}
      </option>
    </select>
    <textarea
      v-else-if="field.presentation === 'multiline'"
      ref="input"
      v-model="draft"
      :aria-label="t('field.editor.value')"
      :aria-invalid="reason !== null"
      :placeholder="t('field.editor.multilineHint')"
    />
    <input
      v-else
      ref="input"
      v-model="draft"
      type="text"
      :inputmode="field.presentation === 'number' ? 'decimal' : undefined"
      :aria-label="t('field.editor.value')"
      :aria-invalid="reason !== null"
      :placeholder="placeholder"
    />
    <button
      v-if="field.presentation === 'select' && multiple"
      type="button"
      @click="commitSelect"
    >
      {{ t('field.editor.apply') }}
    </button>
    <p v-if="reason" class="field-editor-error" role="alert">{{ error }}</p>
  </div>
</template>

<style scoped>
.field-editor {
  display: grid;
  gap: 4px;
}
.field-editor input[type='text'],
.field-editor textarea,
.field-editor select {
  width: 100%;
  padding: 4px 6px;
  background: var(--surface);
  color: var(--text);
  border: 2px solid var(--accent);
}
.field-editor textarea {
  min-height: 90px;
}
.field-editor-error {
  margin: 0;
  color: #c0362c;
  font-size: 0.85em;
}
:global(:root[data-theme='dark']) .field-editor-error {
  color: #ff8a80;
}
</style>
