import type { RouteRecordRaw } from 'vue-router'

export const routes: RouteRecordRaw[] = [
  {
    path: 'x/ads',
    name: 'wl-ads-entrypoints',
    component: () => import('./views/EntryPointsView.vue'),
    meta: { permission: 'wl_ads_entrypoints' }
  },
  {
    path: 'x/ads/:id',
    name: 'wl-ads-entrypoint',
    component: () => import('./views/EntryPointView.vue'),
    meta: { permission: 'wl_ads_entrypoints' }
  }
]
