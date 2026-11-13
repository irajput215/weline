import type { RouteRecordRaw } from 'vue-router'

export const routes: RouteRecordRaw[] = [
  { path: 'x/audit', name: 'wl-audit', component: () => import('./views/AuditView.vue'), meta: { permission: 'wl_audit_events' } }
]
