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
  fields: ['M4 6h.01M4 12h.01M4 18h.01', 'M8 6h12M8 12h12M8 18h12'],
  filter: ['M3 5h18l-7 8v6l-4-2v-4z'],
  group: ['M4 5h16v4H4z', 'M5 9v10h14V9M10 13h4'],
  sort: ['M5 19v-5M10 19V9M15 19V5M20 19V3'],
  colour: ['M14 4l6 6-2 2-6-6zM12 6l-7 7-1 5 5-1 7-7'],
  height: ['M12 4v16M9 7l3-3 3 3M9 17l3 3 3-3'],
  trash: ['M4 7h16M9 4h6M6 7l1 13h10l1-13M10 11v6M14 11v6'],
  eye: [
    'M2 12s4-6 10-6 10 6 10 6-4 6-10 6S2 12 2 12z',
    'M12 9a3 3 0 1 0 0 6 3 3 0 0 0 0-6z',
  ],
  expand: ['M14 4h6v6M20 4l-7 7M10 20H4v-6M4 20l7-7'],
  fullscreen: ['M8 4H4v4M16 4h4v4M20 16v4h-4M8 20H4v-4'],
  close: ['M6 6l12 12M18 6L6 18'],
  duplicate: ['M9 9h11v11H9z', 'M15 9V4H4v11h5'],
  chevronUp: ['M6 15l6-6 6 6'],
  comments: ['M4 4h16v12H9l-5 4z'],
  history: ['M4 8a9 9 0 1 1-1 8M4 3v5h5', 'M12 7v5l3 2'],
  compact: ['M4 5h16M4 9h16M4 13h16M4 17h16'],
  edit: ['M4 20l1-5L16 4l4 4L9 19zM14 6l4 4'],
  calendar: ['M4 6h16v14H4zM8 3v6M16 3v6M4 11h16'],
  clock: ['M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM12 7v5l3 2'],
  check: ['M4 4h16v16H4zM8 12l3 3 5-6'],
  link: ['M10 8l2-2a4 4 0 0 1 6 6l-2 2M14 16l-2 2a4 4 0 0 1-6-6l2-2M9 15l6-6'],
} as const
export type IconName = keyof typeof paths
export function fieldIconName(kind: string): IconName {
  if (kind === 'date' || kind.endsWith('_at') || kind === 'datetime')
    return 'calendar'
  if (kind === 'time') return 'clock'
  if (kind === 'checkbox') return 'check'
  if (kind === 'link' || kind === 'url') return 'link'
  if (kind.includes('select') || kind.includes('array')) return 'fields'
  return 'view'
}
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
