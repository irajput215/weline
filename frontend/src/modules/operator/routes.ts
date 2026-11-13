import type { RouteRecordRaw } from 'vue-router'

export const routes: RouteRecordRaw[] = [
  { path: 'x/operator', name: 'wl-operator', component: () => import('./views/OperatorView.vue'), meta: { permission: 'wl_operator_console' } }
]
