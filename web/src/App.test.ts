import { afterEach, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, disposePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { NConfigProvider, darkTheme, dateZhCN, zhCN } from 'naive-ui'
import App from './App.vue'
import LoginPage from './pages/LoginPage.vue'
import { i18n, loadLocale } from './i18n'
import { usePreferencesStore } from './stores/preferences'

afterEach(() => localStorage.clear())

it('AT-003: header toggles update the login surface and Naive UI configuration', async () => {
  const pinia = createPinia()
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/login', component: LoginPage }],
  })
  await loadLocale('en')
  i18n.global.locale.value = 'en'
  await router.push('/login')
  const wrapper = mount(App, {
    global: { plugins: [pinia, i18n, router] },
  })
  try {
    expect(wrapper.find('h1').text()).toBe('Welcome to PandaTable')
    const language = wrapper.get('.preferences .language-toggle')
    expect(language.text()).toBe('中文')
    await language.trigger('click')
    await flushPromises()
    await vi.waitFor(() =>
      expect(wrapper.find('h1').text()).toBe('欢迎使用 PandaTable'),
    )
    expect(wrapper.get('.preferences .language-toggle').text()).toBe('English')
    await wrapper.get('[aria-label="切换到深色主题"]').trigger('click')
    expect(usePreferencesStore(pinia).theme).toBe('dark')
    expect(wrapper.find('[aria-label="切换到浅色主题"]').exists()).toBe(true)
    const provider = wrapper.findComponent(NConfigProvider)
    expect(provider.props('theme')).toEqual(darkTheme)
    expect(provider.props('locale')).toEqual(zhCN)
    expect(provider.props('dateLocale')).toEqual(dateZhCN)
    expect(localStorage.getItem('pandatable.locale')).toBe('zh-CN')
    expect(localStorage.getItem('pandatable.theme')).toBe('dark')
  } finally {
    wrapper.unmount()
    disposePinia(pinia)
  }
})
