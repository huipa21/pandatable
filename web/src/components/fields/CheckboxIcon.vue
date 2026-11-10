<script setup lang="ts">
import { computed } from 'vue'
import type { CheckboxIcon } from '../../fields/number'
import AppIcon, { type IconName } from '../AppIcon.vue'

const props = withDefaults(
  defineProps<{ icon?: CheckboxIcon; checked: boolean; label?: string }>(),
  { icon: 'check', label: undefined },
)

const names: Record<Exclude<CheckboxIcon, 'check'>, IconName> = {
  square_check: 'square-check',
  circle_check: 'circle-check',
  star: 'star',
  heart: 'heart',
  thumbs_up: 'thumbs-up',
  flag: 'flag',
}
const name = computed(() =>
  props.icon === 'check' ? 'check' : names[props.icon],
)
</script>

<template>
  <span
    class="field-check"
    :class="{
      checked,
      'field-check-symbol': icon !== 'check',
      'field-check-box': icon === 'square_check' || icon === 'circle_check',
    }"
    :data-icon="icon"
    :role="label ? 'img' : undefined"
    :aria-label="label"
    :aria-hidden="label ? undefined : 'true'"
  >
    <AppIcon v-if="icon !== 'check'" :name="name" :size="16" />
  </span>
</template>

<style scoped>
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
.field-check:not(.field-check-symbol).checked::after {
  content: '\2713';
}
.field-check-symbol {
  border: 0;
  color: var(--muted);
}
.field-check-symbol.checked:not(.field-check-box) :deep(path) {
  fill: currentColor;
}
.field-check-box:not(.checked) :deep(path:last-child) {
  display: none;
}
</style>
