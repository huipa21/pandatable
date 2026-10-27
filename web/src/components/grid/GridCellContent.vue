<script setup lang="ts">
import type { BoundField } from '../../fields/registry'
import type { FieldEnv, WireValue } from '../../fields/types'
import FieldRenderer from '../fields/FieldRenderer.vue'

defineProps<{
  field: BoundField
  value: WireValue
  env: FieldEnv
  message: string
}>()
</script>

<template>
  <div v-if="message" class="grid-cell-invalid" aria-invalid="true">
    <FieldRenderer :field="field" :value="value" :env="env" />
    <span role="alert">{{ message }}</span>
  </div>
  <FieldRenderer v-else :field="field" :value="value" :env="env" />
</template>
