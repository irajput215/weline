import type { RouteRecordRaw } from 'vue-router'

export const routes: RouteRecordRaw[] = [
  { path: 'x/ai', name: 'wl-ai', component: () => import('./views/AiView.vue'), meta: { permission: 'wl_ai_runs' } }
]
