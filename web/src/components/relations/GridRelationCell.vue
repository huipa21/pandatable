<script setup lang="ts">
import RelationLinks from './RelationLinks.vue'

defineProps<{
  links: unknown
  editable: boolean
  open: () => void
}>()
</script>

<template>
  <RelationLinks :links="links" :editable="editable" @edit="open" />
</template>
