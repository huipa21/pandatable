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
  text: ['M5 5h14M12 5v15M8 20h8'],
  number: ['M9 3L7 21M17 3l-2 18M4 9h17M3 15h17'],
  formula: ['M18 4H7l6 8-6 8h11'],
  lookup: [
    'M3 5h12M3 9h8M3 13h6',
    'M16 12a4 4 0 1 0 0 8 4 4 0 0 0 0-8zM19 19l3 3',
  ],
  rollup: ['M4 7h6v6H4zM14 14h6v6h-6zM14 4h6v6h-6zM10 10l4 4M10 7h4'],
  attachment: ['M8 13l7-7a3 3 0 0 1 4 4l-9 9a5 5 0 0 1-7-7l9-9M6 15l9-9'],
  mail: ['M3 5h18v14H3zM3 5l9 8 9-8'],
  star: ['M12 3l3 6 6 1-4.5 4.5 1 6.5-5.5-3-5.5 3 1-6.5L3 10l6-1z'],
  phone: ['M5 3l4 5-2 3a14 14 0 0 0 6 6l3-2 5 4-2 3C10 22 2 14 2 5z'],
  code: ['M8 6l-6 6 6 6M16 6l6 6-6 6M14 3l-4 18'],
  user: ['M12 3a4 4 0 1 0 0 8 4 4 0 0 0 0-8zM4 21v-2a8 8 0 0 1 16 0v2'],
  percent: [
    'M5 19L19 5M6 4a2 2 0 1 0 0 4 2 2 0 0 0 0-4zM18 16a2 2 0 1 0 0 4 2 2 0 0 0 0-4z',
  ],
  currency: ['M12 2v20M17 6H9a4 4 0 0 0 0 8h6a4 4 0 0 1 0 8H5'],
} as const
export type IconName = keyof typeof paths
export function fieldIconName(kind: string): IconName {
  if (kind === 'text') return 'text'
  if (kind === 'lookup' || kind === 'rollup' || kind === 'formula') return kind
  if (kind === 'attachment') return 'attachment'
  if (kind === 'email') return 'mail'
  if (kind === 'rating') return 'star'
  if (kind === 'phone') return 'phone'
  if (kind === 'json' || kind === 'uuid' || kind === 'inet' || kind === 'cidr')
    return 'code'
  if (kind === 'user' || kind === 'multi_user' || kind.endsWith('_by'))
    return 'user'
  if (kind === 'currency' || kind === 'percent') return kind
  if (['integer', 'decimal', 'float', 'autonumber'].includes(kind))
    return 'number'
  if (kind === 'date' || kind.endsWith('_at') || kind === 'datetime')
    return 'calendar'
  if (kind === 'time' || kind === 'duration') return 'clock'
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
