<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { BoundField } from '../../fields/registry'
import { choiceLabels, type ChoiceOptions } from '../../fields/choice'
import { safeHref } from '../../fields/text'
import type { FieldEnv, WireValue } from '../../fields/types'

const props = withDefaults(
  defineProps<{
    field: BoundField
    value: WireValue
    env?: FieldEnv
    /** Caller-supplied display name lookup for user ids (D-120). */
    resolveUser?: (id: string) => string | null
  }>(),
  { env: () => ({}), resolveUser: undefined },
)
const { t } = useI18n()

const text = computed(() => props.field.formatDisplay(props.value, props.env))
const href = computed(() => safeHref(props.field.kind, props.value))
const chips = computed(() => {
  const choices = (props.field.options as ChoiceOptions).choices ?? []
  return choiceLabels(props.value).map((label) => {
    const choice = choices.find((item) => item.label === label)
    return { label, color: choice?.color ?? null }
  })
})
const user = computed(() => {
  if (typeof props.value !== 'string') return null
  const name = props.resolveUser?.(props.value) ?? null
  return name ?? t('field.user.unknown', { id: props.value.slice(0, 8) })
})
</script>

<template>
  <span
    class="field-value"
    :class="[
      `field-${field.presentation}`,
      { 'field-readonly': field.readOnly },
    ]"
  >
    <template v-if="field.presentation === 'checkbox'">
      <span
        class="field-check"
        :class="{ checked: value === true }"
        role="img"
        :aria-label="
          value === true
            ? t('field.checkbox.checked')
            : t('field.checkbox.unchecked')
        "
      />
    </template>
    <template v-else-if="field.presentation === 'select'">
      <span
        v-for="chip in chips"
        :key="chip.label"
        class="field-chip"
        :class="chip.color ? `chip-${chip.color}` : 'chip-unknown'"
        :title="chip.color ? undefined : t('field.select.unknown')"
        v-text="chip.label"
      />
    </template>
    <template v-else-if="field.presentation === 'user'">
      <span v-if="user !== null" class="field-user">{{ user }}</span>
    </template>
    <a
      v-else-if="href"
      class="field-link"
      :href="href"
      target="_blank"
      rel="noopener noreferrer nofollow"
      v-text="text"
    />
    <template v-else>{{ text }}</template>
  </span>
</template>

<style scoped>
.field-value {
  display: inline-flex;
  gap: 4px;
  align-items: center;
  max-width: 100%;
  overflow: hidden;
  color: var(--text);
  text-overflow: ellipsis;
  white-space: nowrap;
}
.field-multiline {
  white-space: pre-wrap;
}
.field-number {
  justify-content: flex-end;
  font-variant-numeric: tabular-nums;
}
.field-readonly {
  color: var(--muted);
}
.field-link {
  color: var(--accent);
}
.field-check {
  display: inline-grid;
  width: 16px;
  height: 16px;
  place-items: center;
  border: 1px solid var(--border);
  border-radius: 3px;
}
.field-check.checked {
  border-color: var(--accent);
  color: var(--accent);
}
.field-check.checked::after {
  content: '\2713';
}
.field-chip {
  --chip: #8a8f98;
  padding: 0 8px;
  border-radius: 10px;
  background: color-mix(in srgb, var(--chip) 24%, transparent);
  color: var(--text);
}
.chip-unknown {
  border: 1px dashed var(--border);
  background: transparent;
  color: var(--muted);
}
.chip-red {
  --chip: #e5484d;
}
.chip-orange {
  --chip: #f76b15;
}
.chip-amber {
  --chip: #ffb224;
}
.chip-yellow {
  --chip: #f5d90a;
}
.chip-lime {
  --chip: #99d52a;
}
.chip-green {
  --chip: #30a46c;
}
.chip-teal {
  --chip: #12a594;
}
.chip-cyan {
  --chip: #05a2c2;
}
.chip-blue {
  --chip: #0090ff;
}
.chip-indigo {
  --chip: #3e63dd;
}
.chip-purple {
  --chip: #8e4ec6;
}
.chip-pink {
  --chip: #d6409f;
}
</style>
