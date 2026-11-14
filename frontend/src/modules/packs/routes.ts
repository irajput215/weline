import type { RouteRecordRaw } from 'vue-router'

export const routes: RouteRecordRaw[] = [
  { path: 'x/packs', name: 'wl-packs', component: () => import('./views/PacksView.vue'), meta: { permission: 'wl_packs_install' } }
]
