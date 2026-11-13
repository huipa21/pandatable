<script setup lang="ts">
const props = withDefaults(defineProps<{ name: IconName; size?: number }>(), {
  size: 18,
})
</script>

<script lang="ts">
const paths = {
  activityClose: ['M4 4h16v16H4z', 'M15 4v16', 'M8 9l3 3-3 3'],
  activityOpen: ['M4 4h16v16H4z', 'M15 4v16', 'M11 9l-3 3 3 3'],
  panelClose: ['M4 4h16v16H4z', 'M9 4v16', 'M16 9l-3 3 3 3'],
  panelOpen: ['M4 4h16v16H4z', 'M9 4v16', 'M13 9l3 3-3 3'],
  language: [
    'M3 5h10M8 3v2M4 8h9M5 8l7 7M12 8l-7 7',
    'M16 20l3-10 3 10M17 17h4',
  ],
  search: ['M11 4a7 7 0 1 0 0 14 7 7 0 0 0 0-14z', 'M20 20l-4-4'],
  sun: [
    'M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8z',
    'M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4',
  ],
  moon: ['M20 14.5A8 8 0 0 1 9.5 4a8 8 0 1 0 10.5 10.5z'],
  undo: ['M9 14L4 9l5-5', 'M4 9h10a6 6 0 0 1 0 12h-3'],
  redo: ['M15 14l5-5-5-5', 'M20 9H10a6 6 0 0 0 0 12h3'],
  transfer: ['M7 3v18m0-18L4 6m3-3 3 3', 'M17 21V3m0 18 3-3m-3 3-3-3'],
  save: ['M4 3h13l4 4v14H3V3z', 'M7 3v6h10V3', 'M7 21v-8h10v8'],
  table: ['M4 5h16v14H4z', 'M4 10h16M4 15h16M10 5v14'],
  book: ['M5 4h11a3 3 0 0 1 3 3v13H8a3 3 0 0 1-3-3z', 'M5 17a3 3 0 0 1 3-3h11'],
  description: ['M5 3h10l4 4v14H5z', 'M15 3v5h4', 'M8 12h8M8 16h8'],
  view: ['M4 6h16M7 12h10M10 18h4'],
  'display-field': [
    'M4 4h10v16H4z',
    'M8 8h3M8 12h3M8 16h3',
    'M17 8l3 2-3 2M17 14l3 2-3 2',
  ],
  chevronDown: ['M6 9l6 6 6-6'],
  'chevron-down': ['M6 9l6 6 6-6'],
  chevronRight: ['M9 6l6 6-6 6'],
  plus: ['M12 5v14M5 12h14'],
  'insert-left': ['M5 5v14M19 5v14', 'M16 8l-4 4 4 4M12 12h7'],
  'insert-right': ['M5 5v14M19 5v14', 'M8 8l4 4-4 4M7 12h5'],
  more: ['M5 12h.01M12 12h.01M19 12h.01'],
  moreVertical: ['M12 5h.01M12 12h.01M12 19h.01'],
  grip: ['M8 5h.01M16 5h.01M8 12h.01M16 12h.01M8 19h.01M16 19h.01'],
  fields: ['M4 6h.01M4 12h.01M4 18h.01', 'M8 6h12M8 12h12M8 18h12'],
  filter: ['M3 5h18l-7 8v6l-4-2v-4z'],
  group: ['M4 5h16v4H4z', 'M5 9v10h14V9M10 13h4'],
  sort: ['M5 19v-5M10 19V9M15 19V5M20 19V3'],
  'sort-asc': ['M4 5h10M4 9h7M4 13h4', 'M18 20V4m-3 3 3-3 3 3'],
  'sort-desc': ['M4 5h10M4 9h7M4 13h4', 'M18 4v16m-3-3 3 3 3-3'],
  colour: ['M14 4l6 6-2 2-6-6zM12 6l-7 7-1 5 5-1 7-7'],
  height: ['M12 4v16M9 7l3-3 3 3M9 17l3 3 3-3'],
  trash: ['M4 7h16M9 4h6M6 7l1 13h10l1-13M10 11v6M14 11v6'],
  eye: [
    'M2 12s4-6 10-6 10 6 10 6-4 6-10 6S2 12 2 12z',
    'M12 9a3 3 0 1 0 0 6 3 3 0 0 0 0-6z',
  ],
  'eye-off': [
    'M3 3l18 18',
    'M10.6 10.6a2 2 0 0 0 2.8 2.8',
    'M9.9 5.2A11 11 0 0 1 12 5c6 0 10 7 10 7a16 16 0 0 1-3 3.8M6.2 6.2C3.6 7.8 2 12 2 12s4 7 10 7a10 10 0 0 0 3-.5',
  ],
  'circle-check': ['M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z', 'M8 12l3 3 5-6'],
  'square-check': ['M4 4h16v16H4z', 'M8 12l3 3 5-6'],
  expand: ['M14 4h6v6M20 4l-7 7M10 20H4v-6M4 20l7-7'],
  fullscreen: ['M8 4H4v4M16 4h4v4M20 16v4h-4M8 20H4v-4'],
  close: ['M6 6l12 12M18 6L6 18'],
  checkMark: ['M4 12l5 5L20 6'],
  duplicate: ['M9 9h11v11H9z', 'M15 9V4H4v11h5'],
  flag: ['M5 21V4', 'M5 4c5-4 9 4 14 0v11c-5 4-9-4-14 0'],
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
  heart: [
    'M20.8 4.6a5.5 5.5 0 0 0-7.8 0L12 5.7l-1.1-1.1a5.5 5.5 0 0 0-7.8 7.8l1.1 1.1L12 21l7.8-7.5 1.1-1.1a5.5 5.5 0 0 0-.1-7.8z',
  ],
  'thumbs-up': [
    'M7 10v11H4V10z',
    'M7 10l5-7 1 1v5h5a2 2 0 0 1 2 2l-1 7a3 3 0 0 1-3 3H7',
  ],
  phone: ['M5 3l4 5-2 3a14 14 0 0 0 6 6l3-2 5 4-2 3C10 22 2 14 2 5z'],
  code: ['M8 6l-6 6 6 6M16 6l6 6-6 6M14 3l-4 18'],
  user: ['M12 3a4 4 0 1 0 0 8 4 4 0 0 0 0-8zM4 21v-2a8 8 0 0 1 16 0v2'],
  percent: [
    'M5 19L19 5M6 4a2 2 0 1 0 0 4 2 2 0 0 0 0-4zM18 16a2 2 0 1 0 0 4 2 2 0 0 0 0-4z',
  ],
  currency: ['M12 2v20M17 6H9a4 4 0 0 0 0 8h6a4 4 0 0 1 0 8H5'],
  import: ['M12 3v11m-4-4 4 4 4-4M4 14v6h16v-6'],
  export: ['M12 14V3m-4 4 4-4 4 4M4 14v6h16v-6'],
  bold: ['M7 4h6a4 4 0 0 1 0 8H7z', 'M7 12h7a4 4 0 0 1 0 8H7z'],
  italic: ['M19 4h-9M14 20H5M15 4L9 20'],
  underline: ['M6 4v6a6 6 0 0 0 12 0V4', 'M4 21h16'],
  strikethrough: [
    'M16 5H10a3.5 3.5 0 0 0-2.5 6',
    'M14 12a4 4 0 0 1 0 8H7',
    'M4 12h16',
  ],
  'heading-1': ['M3 6v12M11 6v12M3 12h8', 'M16 10l3-2v10'],
  'heading-2': [
    'M3 6v12M11 6v12M3 12h8',
    'M15 10a2.5 2.5 0 0 1 5 0c0 2.5-5 4.5-5 8h5',
  ],
  'heading-3': ['M3 6v12M11 6v12M3 12h8', 'M15 8h5l-3 4a3 3 0 1 1-2 5.2'],
  quote: ['M3 12v6', 'M17 6H3M21 12H8M21 18H8'],
  'list-bullet': ['M4 6h.01M4 12h.01M4 18h.01', 'M9 6h11M9 12h11M9 18h11'],
  'list-ordered': [
    'M10 6h11M10 12h11M10 18h11',
    'M4 4h1v5M4 9h2',
    'M4 15a1 1 0 0 1 2 0c0 1-2 2-2 4h2',
  ],
  'list-check': ['M3 7l2 2 4-4M3 17l2 2 4-4', 'M13 6h8M13 12h8M13 18h8'],
  image: [
    'M3 5h18v14H3z',
    'M8.5 8a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3z',
    'M21 15l-5-5L5 19',
  ],
  'long-text': ['M4 6h16M4 10h16M4 14h16M4 18h10'],
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
