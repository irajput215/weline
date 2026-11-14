import type { RouteRecordRaw } from 'vue-router'

export const routes: RouteRecordRaw[] = [
  { path: 'x/timeline', name: 'wl-timeline', component: () => import('./views/TimelineView.vue'), meta: { permission: 'wl_timeline_contacts' } }
]
