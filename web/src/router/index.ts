import { createRouter, createWebHistory } from 'vue-router'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/login' },
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
