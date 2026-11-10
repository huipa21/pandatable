import { afterEach, beforeEach, expect, it } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { i18n, loadLocale } from '../../i18n'
import type { TableField } from '../../api/types'
import { tableSchema } from '../../test/navigation'
import ColumnHeaderMenu from './ColumnHeaderMenu.vue'

let wrapper: VueWrapper | undefined
let anchor: HTMLButtonElement

beforeEach(async () => {
  await loadLocale('en')
  i18n.global.locale.value = 'en'
  anchor = document.createElement('button')
  document.body.appendChild(anchor)
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  anchor.remove()
})

async function openMenu(
  overrides: Partial<{
    field: TableField
    owner: boolean
    displayFieldId: string | null
    groupBy: string[]
    groupPending: boolean
  }> = {},
) {
  const field = {
    ...tableSchema().fields[0]!,
    filterable: true,
    filterOps: ['contains'] as TableField['filterOps'],
  }
  wrapper = mount(ColumnHeaderMenu, {
    props: {
      field,
      anchor,
      owner: true,
      displayFieldId: null,
      groupBy: [],
      groupPending: false,
      ...overrides,
    },
    global: { plugins: [i18n] },
    attachTo: document.body,
  })
  await flushPromises()
  return document.querySelector<HTMLElement>('.column-header-menu')!
}

it('shows twelve owner header menu actions and five reader actions', async () => {
  let menu = await openMenu()
  expect(menu.querySelectorAll('[role="menuitem"]')).toHaveLength(12)
  expect(menu.querySelectorAll('[role="separator"]')).toHaveLength(5)
  wrapper!.unmount()
  wrapper = undefined
  menu = await openMenu({ owner: false })
  expect(
    Array.from(menu.querySelectorAll<HTMLElement>('[role="menuitem"]')).map(
      (item) => item.dataset.action,
    ),
  ).toEqual(['hide', 'sort-asc', 'sort-desc', 'filter', 'group'])
})

it('disables unsupported header menu actions and protects the display field', async () => {
  const field = {
    ...tableSchema().fields[0]!,
    uiType: 'link',
    filterable: true,
    filterOps: ['is_empty'] as TableField['filterOps'],
  }
  const menu = await openMenu({ field, displayFieldId: field.id })
  const duplicate = menu.querySelector<HTMLButtonElement>(
    '[data-action="duplicate"]',
  )!
  expect(duplicate.disabled).toBe(true)
  expect(duplicate.title).toContain('link')
  expect(
    menu.querySelector<HTMLButtonElement>('[data-action="hide"]')!.disabled,
  ).toBe(true)
  expect(
    menu.querySelector<HTMLButtonElement>('[data-action="delete"]')!.disabled,
  ).toBe(true)
  expect(menu.querySelector('[data-action="display"]')).toBeNull()
})

it('navigates the header menu with arrows and Home/End and restores focus on Escape', async () => {
  const menu = await openMenu()
  const items = Array.from(
    menu.querySelectorAll<HTMLButtonElement>(
      '[role="menuitem"]:not(:disabled)',
    ),
  )
  expect(document.activeElement).toBe(items[0])
  items[0]!.dispatchEvent(
    new KeyboardEvent('keydown', { key: 'End', bubbles: true }),
  )
  expect(document.activeElement).toBe(items.at(-1))
  items
    .at(-1)!
    .dispatchEvent(
      new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }),
    )
  expect(document.activeElement).toBe(items[0])
  items[0]!.dispatchEvent(
    new KeyboardEvent('keydown', { key: 'ArrowUp', bubbles: true }),
  )
  expect(document.activeElement).toBe(items.at(-1))
  items
    .at(-1)!
    .dispatchEvent(new KeyboardEvent('keydown', { key: 'Home', bubbles: true }))
  expect(document.activeElement).toBe(items[0])
  items[0]!.dispatchEvent(
    new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }),
  )
  expect(wrapper!.emitted('close')).toHaveLength(1)
  expect(document.activeElement).toBe(anchor)
})

it('dismisses the header menu on outside pointer and scroll events', async () => {
  await openMenu()
  document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }))
  expect(wrapper!.emitted('close')).toHaveLength(1)
  document.dispatchEvent(new Event('scroll'))
  expect(wrapper!.emitted('close')).toHaveLength(2)
})
