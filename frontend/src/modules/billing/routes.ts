import type { RouteRecordRaw } from 'vue-router'

export const routes: RouteRecordRaw[] = [
  { path: 'x/billing', name: 'wl-billing', component: () => import('./views/BillingView.vue'), meta: { permission: 'wl_billing_plan' } }
]
