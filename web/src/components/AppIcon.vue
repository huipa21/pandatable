<script setup lang="ts">
const props = withDefaults(defineProps<{ name: IconName; size?: number }>(), {
  size: 18,
})
</script>

<script lang="ts">
const paths = {
  panelClose: ['M4 4h16v16H4z', 'M9 4v16', 'M16 9l-3 3 3 3'],
  panelOpen: ['M4 4h16v16H4z', 'M9 4v16', 'M13 9l3 3-3 3'],
  search: ['M11 4a7 7 0 1 0 0 14 7 7 0 0 0 0-14z', 'M20 20l-4-4'],
  sun: [
    'M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8z',
    'M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4',
  ],
  moon: ['M20 14.5A8 8 0 0 1 9.5 4a8 8 0 1 0 10.5 10.5z'],
  undo: ['M9 14L4 9l5-5', 'M4 9h10a6 6 0 0 1 0 12h-3'],
  redo: ['M15 14l5-5-5-5', 'M20 9H10a6 6 0 0 0 0 12h3'],
  table: ['M4 5h16v14H4z', 'M4 10h16M4 15h16M10 5v14'],
  book: ['M5 4h11a3 3 0 0 1 3 3v13H8a3 3 0 0 1-3-3z', 'M5 17a3 3 0 0 1 3-3h11'],
  view: ['M4 6h16M7 12h10M10 18h4'],
  chevronDown: ['M6 9l6 6 6-6'],
  chevronRight: ['M9 6l6 6-6 6'],
  plus: ['M12 5v14M5 12h14'],
  more: ['M5 12h.01M12 12h.01M19 12h.01'],
} as const
export type IconName = keyof typeof paths
</script>

<template>
  <svg
    class="app-icon"
    aria-hidden="true"
    :width="props.size"
    :height="props.size"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="2"
    stroke-linecap="round"
    stroke-linejoin="round"
  >
    <path v-for="d in paths[props.name]" :key="d" :d="d" />
  </svg>
</template>
