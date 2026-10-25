import { createRouter, createWebHistory } from 'vue-router'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/login' },
    { path: '/setup', component: () => import('../pages/SetupPage.vue') },
    { path: '/login', component: () => import('../pages/LoginPage.vue') },
    {
      path: '/_spikes/revogrid',
      component: () => import('../pages/GridSpikePage.vue'),
    },
    {
      path: '/:pathMatch(.*)*',
      component: () => import('../pages/NotFoundPage.vue'),
    },
  ],
})

router.beforeEach(async (to) => {
  try {
    const response = await fetch('/api/v1/setup/status', { cache: 'no-store' })
    if (response.ok) {
      const status = (await response.json()) as { needed: boolean }
      if (status.needed && to.path !== '/setup') return '/setup'
    }
  } catch {
    return true
  }
  return true
})
