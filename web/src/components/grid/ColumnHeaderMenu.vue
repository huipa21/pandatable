<script lang="ts">
export type ColumnHeaderAction =
  | 'edit'
  | 'duplicate'
  | 'description'
  | 'hide'
  | 'display'
  | 'sort-asc'
  | 'sort-desc'
  | 'filter'
  | 'group'
  | 'insert-left'
  | 'insert-right'
  | 'delete'
</script>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { TableField } from '../../api/types'
import { displayEligible } from '../fields/fieldForm'
import AppIcon, { type IconName } from '../AppIcon.vue'

const props = defineProps<{
  field: TableField
  anchor: HTMLButtonElement
  owner: boolean
  displayFieldId: string | null
  groupBy: string[]
  groupPending: boolean
}>()
const emit = defineEmits<{
  select: [action: ColumnHeaderAction]
  close: []
}>()
const { t } = useI18n()
const menu = ref<HTMLElement>()
const position = ref({ left: '0px', top: '0px' })

interface MenuItem {
  action: ColumnHeaderAction
  icon: IconName
  label: string
  disabled?: string
}

const groups = computed<MenuItem[][]>(() => {
  const field = props.field
  const display = field.id === props.displayFieldId
  const link = field.uiType === 'link'
  const structure: MenuItem[] = props.owner
    ? [
        { action: 'edit', icon: 'edit', label: t('gridPolish.gp12.edit') },
        {
          action: 'duplicate',
          icon: 'duplicate',
          label: t('gridPolish.gp12.duplicate'),
          disabled:
            field.isSystem || link
              ? t('gridPolish.gp12.duplicateDisabled')
              : '',
        },
        {
          action: 'description',
          icon: 'description',
          label: t('gridPolish.gp12.description'),
        },
      ]
    : []
  const view: MenuItem[] = [
    {
      action: 'hide',
      icon: 'eye-off',
      label: t('gridPolish.gp12.hide'),
      disabled: display ? t('gridPolish.gp12.displayRequired') : '',
    },
  ]
  if (props.owner && !display)
    view.push({
      action: 'display',
      icon: 'display-field',
      label: t('gridPolish.gp12.display'),
      disabled: displayEligible(field)
        ? ''
        : t('gridPolish.gp12.displayDisabled'),
    })
  const sort: MenuItem[] = [
    {
      action: 'sort-asc',
      icon: 'sort-asc',
      label: t('gridPolish.gp12.sortAsc'),
      disabled: link ? t('gridPolish.gp12.linkUnsupported') : '',
    },
    {
      action: 'sort-desc',
      icon: 'sort-desc',
      label: t('gridPolish.gp12.sortDesc'),
      disabled: link ? t('gridPolish.gp12.linkUnsupported') : '',
    },
  ]
  const filter: MenuItem[] = [
    {
      action: 'filter',
      icon: 'filter',
      label: t('gridPolish.gp12.filter'),
      disabled:
        field.filterable === false || !field.filterOps?.length
          ? t('gridPolish.gp12.filterDisabled')
          : '',
    },
    {
      action: 'group',
      icon: 'group',
      label: t('gridPolish.gp12.group'),
      disabled: link
        ? t('gridPolish.gp12.linkUnsupported')
        : props.groupBy.includes(field.id)
          ? t('gridPolish.gp12.alreadyGrouped')
          : props.groupBy.length >= 3
            ? t('gridPolish.gp12.groupLimit')
            : props.groupPending
              ? t('gridPolish.gp12.groupPending')
              : '',
    },
  ]
  const insert: MenuItem[] = props.owner
    ? [
        {
          action: 'insert-left',
          icon: 'insert-left',
          label: t('gridPolish.gp12.insertLeft'),
        },
        {
          action: 'insert-right',
          icon: 'insert-right',
          label: t('gridPolish.gp12.insertRight'),
        },
      ]
    : []
  const danger: MenuItem[] = props.owner
    ? [
        {
          action: 'delete',
          icon: 'trash',
          label: t('gridPolish.gp12.delete'),
          disabled:
            field.isSystem || display
              ? t('gridPolish.gp12.deleteDisabled')
              : '',
        },
      ]
    : []
  return [structure, view, sort, filter, insert, danger].filter(
    (group) => group.length,
  )
})

function placeMenu() {
  if (!menu.value) return
  const anchor = props.anchor.getBoundingClientRect()
  const bounds = menu.value.getBoundingClientRect()
  const left = Math.max(
    8,
    Math.min(anchor.left, window.innerWidth - bounds.width - 8),
  )
  const below = anchor.bottom + 4
  const top =
    below + bounds.height <= window.innerHeight - 8
      ? below
      : Math.max(8, anchor.top - bounds.height - 4)
  position.value = { left: `${left}px`, top: `${top}px` }
}

function close(restoreFocus = false) {
  emit('close')
  if (restoreFocus && props.anchor.isConnected) props.anchor.focus()
}

function outside(event: PointerEvent) {
  const path = event.composedPath()
  if (!path.includes(props.anchor) && menu.value && !path.includes(menu.value))
    close()
}

function scrolled(event: Event) {
  if (event.target instanceof Node && menu.value?.contains(event.target)) return
  close()
}

function keydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    event.preventDefault()
    event.stopPropagation()
    close(true)
    return
  }
  if (event.key === 'Tab') {
    close()
    return
  }
  const buttons = Array.from(
    menu.value?.querySelectorAll<HTMLButtonElement>(
      '[role="menuitem"]:not(:disabled)',
    ) ?? [],
  )
  const current = buttons.indexOf(document.activeElement as HTMLButtonElement)
  let next: number
  switch (event.key) {
    case 'ArrowDown':
      next = (current + 1) % buttons.length
      break
    case 'ArrowUp':
      next = (current + buttons.length - 1) % buttons.length
      break
    case 'Home':
      next = 0
      break
    case 'End':
      next = buttons.length - 1
      break
    default:
      return
  }
  event.preventDefault()
  buttons[next]?.focus()
}

function choose(item: MenuItem) {
  if (item.disabled) return
  emit('select', item.action)
  close()
}

onMounted(async () => {
  document.addEventListener('pointerdown', outside, true)
  document.addEventListener('scroll', scrolled, true)
  window.addEventListener('resize', placeMenu)
  await nextTick()
  placeMenu()
  menu.value
    ?.querySelector<HTMLButtonElement>('[role="menuitem"]:not(:disabled)')
    ?.focus()
})
onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', outside, true)
  document.removeEventListener('scroll', scrolled, true)
  window.removeEventListener('resize', placeMenu)
})
</script>

<template>
  <Teleport to="body">
    <div
      ref="menu"
      class="column-header-menu"
      role="menu"
      :aria-label="t('gridPolish.gp12.menuFor', { name: field.name })"
      :style="position"
      @keydown="keydown"
    >
      <template v-for="(group, index) in groups" :key="index">
        <div v-if="index" class="column-header-separator" role="separator" />
        <button
          v-for="item in group"
          :key="item.action"
          type="button"
          role="menuitem"
          :data-action="item.action"
          :disabled="!!item.disabled"
          :title="item.disabled || undefined"
          @click="choose(item)"
        >
          <AppIcon :name="item.icon" :size="16" />
          {{ item.label }}
        </button>
      </template>
    </div>
  </Teleport>
</template>

<style scoped>
.column-header-menu {
  position: fixed;
  z-index: 1000;
  width: 230px;
  max-width: calc(100vw - 16px);
  max-height: calc(100vh - 16px);
  overflow: auto;
  padding: 4px;
  color: var(--text);
  background: var(--panel-surface);
  border: 1px solid var(--border);
  border-radius: 6px;
  box-shadow: 0 8px 24px #0003;
}
.column-header-menu button {
  display: flex;
  align-items: center;
  gap: 9px;
  width: 100%;
  padding: 7px 8px;
  color: inherit;
  text-align: left;
  background: transparent;
  border: 0;
  border-radius: 4px;
}
.column-header-menu button:not(:disabled):hover,
.column-header-menu button:focus-visible {
  background: var(--surface);
}
.column-header-menu button:disabled {
  color: var(--muted);
  cursor: not-allowed;
}
.column-header-separator {
  height: 1px;
  margin: 4px 0;
  background: var(--border);
}
</style>
