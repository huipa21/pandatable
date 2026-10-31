import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { afterEach, beforeEach, expect, it } from 'vitest'
import { bindField } from '../../fields/registry'
import { i18n, loadLocale } from '../../i18n'
import { useRealtimeStore } from '../../stores/realtime'
import GridCellContent from '../grid/GridCellContent.vue'
import CellPresence from './CellPresence.vue'
import PresenceAvatars from './PresenceAvatars.vue'
import { presenceColor } from './colors'

let pinia: ReturnType<typeof createPinia>

beforeEach(async () => {
  pinia = createPinia()
  setActivePinia(pinia)
  await loadLocale('en')
  await loadLocale('zh-CN')
  i18n.global.locale.value = 'en'
})

afterEach(() => {
  disposePinia(pinia)
})

it('shows accessible collaborator avatars and matching cell outlines, excluding self', async () => {
  const realtime = useRealtimeStore()
  realtime.presence = {
    table: [
      {
        userId: 'alice',
        name: 'Alice',
        cell: { rowId: '1', fieldId: 'field' },
      },
      { userId: 'bob', name: 'Bob', cell: { rowId: '1', fieldId: 'field' } },
      { userId: 'chen', name: '陈', cell: { rowId: '2', fieldId: 'field' } },
    ],
  }
  const avatars = mount(PresenceAvatars, {
    props: { tableId: 'table', identity: 'alice' },
    global: { plugins: [pinia, i18n] },
  })
  const cell = mount(CellPresence, {
    props: {
      tableId: 'table',
      rowId: '1',
      fieldId: 'field',
      identity: 'alice',
    },
    global: { plugins: [pinia, i18n] },
  })
  try {
    expect(avatars.get('[role="list"]').attributes('aria-label')).toBe(
      'Collaborators viewing this table',
    )
    expect(avatars.findAll('[role="listitem"]')).toHaveLength(2)
    expect(avatars.get('[data-presence-user="bob"]').attributes('title')).toBe(
      'Bob is viewing this table',
    )
    expect(avatars.get('[data-presence-user="chen"]').text()).toBe('陈')
    expect(cell.get('[data-presence-user="bob"]').text()).toBe('Bob')
    expect(cell.find('[data-presence-user="alice"]').exists()).toBe(false)
    expect(cell.find('[data-presence-user="chen"]').exists()).toBe(false)
    const color = document.createElement('span')
    color.style.borderColor = presenceColor('bob')
    expect(
      (cell.get('.cell-presence').element as HTMLElement).style.borderColor,
    ).toBe(color.style.borderColor)
    i18n.global.locale.value = 'zh-CN'
    await nextTick()
    expect(avatars.get('[data-presence-user="bob"]').attributes('title')).toBe(
      'Bob 正在查看此表',
    )
    expect(cell.get('[data-presence-user="bob"]').attributes('title')).toBe(
      'Bob 正聚焦于此单元格',
    )
    realtime.presence = { table: [] }
    await nextTick()
    expect(avatars.find('[role="list"]').exists()).toBe(false)
    expect(cell.find('.cell-presence').exists()).toBe(false)
  } finally {
    avatars.unmount()
    cell.unmount()
  }
})

it('renders live presence for normal and invalid grid cells, but not group rows', async () => {
  const field = bindField('text', null)
  if (!field.ok) throw new Error('Failed to bind text field')
  const realtime = useRealtimeStore()
  const cell = mount(GridCellContent, {
    props: {
      field: field.value,
      value: 'Example',
      env: {},
      message: '',
      tableId: 'table',
      identity: 'alice',
      rowId: '1',
      fieldId: 'field',
    },
    global: { plugins: [pinia, i18n] },
  })
  try {
    expect(cell.find('.cell-presence').exists()).toBe(false)
    realtime.presence = {
      table: [
        { userId: 'bob', name: 'Bob', cell: { rowId: '1', fieldId: 'field' } },
      ],
    }
    await nextTick()
    expect(cell.get('.field-value').text()).toBe('Example')
    expect(cell.get('[data-presence-user="bob"]').text()).toBe('Bob')

    await cell.setProps({ message: 'Invalid value' })
    expect(cell.get('[role="alert"]').text()).toBe('Invalid value')
    expect(cell.findAll('[data-presence-user="bob"]')).toHaveLength(1)

    await cell.setProps({
      group: {
        header: true,
        path: 'group',
        label: 'Group',
        expanded: true,
        ariaLabel: 'Collapse group',
        toggle: () => undefined,
      },
    })
    expect(cell.get('.grid-group-toggle').text()).toContain('Group')
    expect(cell.find('.cell-presence').exists()).toBe(false)

    await cell.setProps({ group: undefined, message: '' })
    expect(cell.get('[data-presence-user="bob"]').text()).toBe('Bob')
    realtime.presence = { table: [] }
    await nextTick()
    expect(cell.find('.cell-presence').exists()).toBe(false)
  } finally {
    cell.unmount()
  }
})
