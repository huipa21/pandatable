import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { i18n, loadLocale } from '../i18n'
import { useAuthStore } from '../stores/auth'
import { useNavStore } from '../stores/nav'
import { navUser } from '../test/navigation'
import HomePage from './HomePage.vue'

let pinia: ReturnType<typeof createPinia>

beforeEach(async () => {
  pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().setUser(navUser)
  await loadLocale('en')
  i18n.global.locale.value = 'en'
})

afterEach(() => {
  disposePinia(pinia)
  vi.restoreAllMocks()
})

async function setup() {
  const page = { template: '<div />' }
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/home', component: HomePage },
      { path: '/w/:workspaceId', component: page },
      { path: '/t/:tableId/v/:viewId/r/:rowId', component: page },
    ],
  })
  await router.push('/home')
  const wrapper = mount(
    { template: '<RouterView />' },
    { global: { plugins: [pinia, i18n, router] } },
  )
  return { router, wrapper }
}

it('resolves and replaces home with the preferred workspace without trapping Back', async () => {
  const nav = useNavStore()
  const resolve = vi.spyOn(nav, 'resolvePreference').mockResolvedValue('w2')
  const refresh = vi.spyOn(nav, 'refresh').mockResolvedValue(true)
  const { router, wrapper } = await setup()
  try {
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/w/w2')
    expect(resolve).toHaveBeenCalledTimes(1)
    expect(refresh).toHaveBeenCalledWith(true)
    await router.push('/t/t/v/v/r/42')
    await router.push('/home')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/w/w2')
    router.back()
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/t/t/v/v/r/42')
  } finally {
    wrapper.unmount()
  }
})

it('shows a localized error and retries resolution rather than showing an empty chooser', async () => {
  const nav = useNavStore()
  vi.spyOn(nav, 'resolvePreference')
    .mockRejectedValueOnce(new TypeError('private server detail'))
    .mockResolvedValueOnce('w')
  vi.spyOn(nav, 'refresh').mockResolvedValue(true)
  const { router, wrapper } = await setup()
  try {
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/home')
    expect(wrapper.get('[role="alert"]').text()).not.toContain(
      'private server detail',
    )
    expect(wrapper.get('[role="alert"]').text()).toContain(
      'Unable to open your default workspace',
    )
    expect(wrapper.text()).not.toContain('Choose a workspace')
    await loadLocale('zh-CN')
    i18n.global.locale.value = 'zh-CN'
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain(
      '无法打开你的默认工作区',
    )
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/w/w')
  } finally {
    wrapper.unmount()
  }
})

it('does not replace an explicit deep link when resolution finishes during its route guard', async () => {
  const nav = useNavStore()
  let releaseResolve!: (id: string) => void
  let releaseGuard!: () => void
  const resolve = vi.spyOn(nav, 'resolvePreference').mockImplementation(
    () =>
      new Promise<string>((done) => {
        releaseResolve = done
      }),
  )
  const refresh = vi.spyOn(nav, 'refresh').mockResolvedValue(true)
  const { router, wrapper } = await setup()
  try {
    expect(wrapper.get('[role="status"]').text()).toBe(
      'Opening your default workspace',
    )
    expect(wrapper.text()).not.toContain('Choose a workspace')
    router.beforeEach((to) => {
      if (to.path.startsWith('/t/'))
        return new Promise<void>((done) => {
          releaseGuard = done
        })
    })
    const navigation = router.push('/t/t/v/v/r/42')
    await flushPromises()
    expect(resolve.mock.calls[0]![0]?.aborted).toBe(true)
    releaseResolve('w')
    await flushPromises()
    releaseGuard()
    await navigation
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/t/t/v/v/r/42')
    expect(refresh).not.toHaveBeenCalled()
  } finally {
    wrapper.unmount()
  }
})

it('keeps home retryable when refreshing the resolved workspace fails', async () => {
  const nav = useNavStore()
  vi.spyOn(nav, 'resolvePreference').mockResolvedValue('w')
  vi.spyOn(nav, 'refresh').mockImplementation(async () => {
    nav.error = new TypeError('offline')
    return false
  })
  const { router, wrapper } = await setup()
  try {
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/home')
    expect(wrapper.find('[role="alert"] button').exists()).toBe(true)
  } finally {
    wrapper.unmount()
  }
})
