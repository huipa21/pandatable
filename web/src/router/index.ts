import { createRouter, createWebHistory } from 'vue-router'
import { request } from '../api/client'
import type { SetupStatus } from '../api/types'
import { useAuthStore } from '../stores/auth'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/login' },
    { path: '/setup', component: () => import('../pages/SetupPage.vue') },
    { path: '/login', component: () => import('../pages/LoginPage.vue') },
    { path: '/home', component: () => import('../pages/HomePage.vue') },
    {
      path: '/change-password',
      component: () => import('../pages/ChangePasswordPage.vue'),
    },
    {
      path: '/admin/dashboard',
      component: () => import('../pages/AdminDashboardPage.vue'),
    },
    {
      path: '/admin/users',
      component: () => import('../pages/AdminUsersPage.vue'),
    },
    {
      path: '/admin/users/:userId',
      component: () => import('../pages/AdminGroupsPage.vue'),
    },
    {
      path: '/admin/groups/:id?',
      component: () => import('../pages/AdminGroupsPage.vue'),
    },
    {
      path: '/connection-error',
      component: () => import('../pages/ConnectionErrorPage.vue'),
    },
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
  if (to.path === '/connection-error') return true
  try {
    const status = await request<SetupStatus>('/setup/status')
    if (status.needed) return to.path === '/setup' ? true : '/setup'
    const auth = useAuthStore()
    await auth.refresh()
    if (auth.user?.mustChangePassword) {
      return to.path === '/change-password' ? true : '/change-password'
    }
    if (to.path === '/setup') return true
    const admin = to.path === '/admin' || to.path.startsWith('/admin/')
    if (
      !auth.user &&
      (admin || ['/home', '/change-password'].includes(to.path))
    )
      return '/login'
    if (admin && auth.user?.role !== 'admin') return '/home'
    if (to.path === '/login' && auth.user) return '/home'
  } catch {
    return '/connection-error'
  }
  return true
})
