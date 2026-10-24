import { afterEach, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { i18n, loadLocale } from '../i18n'
import { ThemeMedia } from '../test/setup'
import GridSpikePage from './GridSpikePage.vue'

afterEach(() => vi.restoreAllMocks())

it('keeps the engine opt-in, excludes mobile, and removes its responsive listener', async () => {
  await loadLocale('en')
  i18n.global.locale.value = 'en'
  const media = new ThemeMedia()
  const add = vi.spyOn(media, 'addEventListener')
  const remove = vi.spyOn(media, 'removeEventListener')
  vi.spyOn(window, 'matchMedia').mockReturnValue(media)
  const wrapper = mount(GridSpikePage, {
    global: { plugins: [i18n], stubs: { RouterLink: true } },
  })
  expect(wrapper.find('[role="status"]').text()).toContain('Below 768 px')
  expect(wrapper.find('revo-grid').exists()).toBe(false)
  media.setDark(true)
  await nextTick()
  expect(wrapper.find('button').text()).toBe('Mount prototype')
  expect(wrapper.find('revo-grid').exists()).toBe(false)
  wrapper.unmount()
  expect(add).toHaveBeenCalledOnce()
  expect(remove).toHaveBeenCalledWith('change', add.mock.calls[0]![1])
})
