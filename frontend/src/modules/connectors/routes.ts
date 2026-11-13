import type { RouteRecordRaw } from 'vue-router'

export const routes: RouteRecordRaw[] = [
  {
    path: 'x/connectors',
    name: 'wl-connectors',
    component: () => import('./views/ConnectedSystemView.vue'),
    meta: { permission: 'wl_connectors_settings' }
  }
]
