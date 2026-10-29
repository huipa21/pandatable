<script setup lang="ts">
import { computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { BoundField } from '../../fields/registry'
import { choiceLabels, type ChoiceOptions } from '../../fields/choice'
import { safeHref } from '../../fields/text'
import type { FieldEnv, WireValue } from '../../fields/types'
import type { AttachmentValue } from '../../api/types'
import {
  queueUserLabels,
  userLabel,
  userLabelFailed,
} from '../../fields/userDirectory'

const props = withDefaults(
  defineProps<{
    field: BoundField
    value: WireValue
    env?: FieldEnv
    links?: boolean
    tableId?: string
    identity?: string
    /** Caller-supplied display name lookup for user ids (D-120). */
    resolveUser?: (id: string) => string | null
  }>(),
  {
    env: () => ({}),
    links: true,
    tableId: undefined,
    identity: undefined,
    resolveUser: undefined,
  },
)
const { t } = useI18n()

const text = computed(() => props.field.formatDisplay(props.value, props.env))
const href = computed(() => safeHref(props.field.kind, props.value))
const attachments = computed<AttachmentValue[]>(() =>
  props.field.kind === 'attachment' && Array.isArray(props.value)
    ? (props.value as unknown as AttachmentValue[])
    : [],
)
const chips = computed(() => {
  const choices = (props.field.options as ChoiceOptions).choices ?? []
  return choiceLabels(props.value).map((label) => {
    const choice = choices.find((item) => item.label === label)
    return { label, color: choice?.color ?? null }
  })
})
const userIds = computed(() =>
  typeof props.value === 'string'
    ? [props.value]
    : Array.isArray(props.value)
      ? props.value.filter(
          (value): value is string => typeof value === 'string',
        )
      : [],
)
watch(
  [userIds, () => props.identity, () => props.tableId],
  ([ids]) => {
    if (props.field.presentation === 'user')
      queueUserLabels(props.identity, props.tableId, ids)
  },
  { immediate: true },
)
const users = computed(() =>
  userIds.value.map((id) => {
    const name =
      props.resolveUser?.(id) ?? userLabel(props.identity, props.tableId, id)
    return {
      id,
      label: name ?? t('field.user.unknown', { id: id.slice(0, 8) }),
      failed: userLabelFailed(props.identity, props.tableId, id),
    }
  }),
)
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
      <span
        v-for="user in users"
        :key="user.id"
        class="field-user"
        :title="user.failed ? t('field.user.lookupFailed') : undefined"
      >
        {{ user.label }}
      </span>
    </template>
    <span
      v-else-if="field.presentation === 'attachment'"
      class="field-attachments"
    >
      <a
        v-for="attachment in attachments"
        :key="attachment.id"
        class="field-attachment"
        :href="`/api/v1/attachments/${encodeURIComponent(attachment.id)}`"
        :download="attachment.name"
      >
        <img
          v-if="attachment.thumb"
          :src="attachment.thumb"
          :alt="attachment.name"
          loading="lazy"
        />
        <span>{{ attachment.name }}</span>
      </a>
    </span>
    <a
      v-else-if="href && links"
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
.field-attachments {
  display: inline-flex;
  gap: 6px;
  max-width: 100%;
  overflow: hidden;
}
.field-attachment {
  display: inline-flex;
  gap: 4px;
  align-items: center;
  min-width: 0;
  color: var(--accent);
}
.field-attachment span {
  overflow: hidden;
  text-overflow: ellipsis;
}
.field-attachment img {
  flex: 0 0 48px;
  width: 48px;
  height: 36px;
  object-fit: cover;
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
