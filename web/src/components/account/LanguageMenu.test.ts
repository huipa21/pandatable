import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, disposePinia } from 'pinia'
import LanguageMenu from './LanguageMenu.vue'
import { i18n, loadLocale, supportedLocales } from '../../i18n'

let pinia: ReturnType<typeof createPinia>

beforeEach(async () => {
  localStorage.clear()
  await loadLocale('en')
  i18n.global.locale.value = 'en'
  pinia = createPinia()
})

afterEach(() => {
  disposePinia(pinia)
  localStorage.clear()
})

function mountMenu() {
  return mount(LanguageMenu, {
    attachTo: document.body,
    global: { plugins: [pinia, i18n] },
  })
}

it('lists supported locales by native and localized names and persists selection', async () => {
  const wrapper = mountMenu()
  try {
    const trigger = wrapper.get('.language-menu-trigger')
    expect(trigger.attributes('aria-label')).toBe('Language')
    expect(trigger.attributes('title')).toBe('Language')
    expect(trigger.attributes('aria-haspopup')).toBe('menu')
    expect(supportedLocales).toEqual(['en', 'zh-CN'])

    await trigger.trigger('click')
    const items = wrapper.findAll('[role="menuitemradio"]')
    expect(items).toHaveLength(2)
    const english = items[0]!
    const chinese = items[1]!
    expect(english.get('.language-menu-native').text()).toBe('English')
    expect(english.get('.language-menu-native').attributes('lang')).toBe('en')
    expect(english.text()).toContain('English')
    expect(english.attributes('aria-checked')).toBe('true')
    expect(chinese.get('.language-menu-native').text()).toBe('简体中文')
    expect(chinese.get('.language-menu-native').attributes('lang')).toBe(
      'zh-CN',
    )
    expect(chinese.text()).toContain('Chinese (Simplified)')
    expect(chinese.attributes('aria-checked')).toBe('false')

    await chinese.trigger('click')
    await vi.waitFor(() => expect(i18n.global.locale.value).toBe('zh-CN'))
    expect(localStorage.getItem('pandatable.locale')).toBe('zh-CN')
    expect(wrapper.find('[role="menu"]').exists()).toBe(false)
    expect(document.activeElement).toBe(trigger.element)

    await trigger.trigger('click')
    const translatedItems = wrapper.findAll('[role="menuitemradio"]')
    expect(
      translatedItems[0]!.get('.language-menu-names span:last-child').text(),
    ).toBe('英语')
    expect(
      translatedItems[1]!.get('.language-menu-names span:last-child').text(),
    ).toBe('简体中文')
    expect(translatedItems[1]!.attributes('aria-checked')).toBe('true')
  } finally {
    wrapper.unmount()
  }
})

it('supports arrow navigation and restores focus on Escape or outside click', async () => {
  const wrapper = mountMenu()
  try {
    const trigger = wrapper.get('.language-menu-trigger')
    await trigger.trigger('click')
    const items = wrapper.findAll('[role="menuitemradio"]')
    const english = items[0]!
    const chinese = items[1]!
    expect(document.activeElement).toBe(english.element)
    await english.trigger('keydown', { key: 'ArrowDown' })
    expect(document.activeElement).toBe(chinese.element)
    await chinese.trigger('keydown', { key: 'ArrowUp' })
    expect(document.activeElement).toBe(english.element)
    await english.trigger('keydown', { key: 'Escape' })
    await flushPromises()
    expect(wrapper.find('[role="menu"]').exists()).toBe(false)
    expect(document.activeElement).toBe(trigger.element)

    await trigger.trigger('click')
    document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    await flushPromises()
    expect(wrapper.find('[role="menu"]').exists()).toBe(false)
    expect(document.activeElement).toBe(trigger.element)
  } finally {
    wrapper.unmount()
  }
})
