import type { RouteRecordRaw } from 'vue-router'

export const routes: RouteRecordRaw[] = [
  { path: 'x/reminders', name: 'wl-reminders', component: () => import('./views/RemindersView.vue'), meta: { permission: 'wl_reminders_rules' } }
]
