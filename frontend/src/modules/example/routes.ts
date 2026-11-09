import type { RouteRecordRaw } from 'vue-router'

export const routes: RouteRecordRaw[] = [
  {
    path: 'x/example/notes',
    name: 'wl-example-notes',
    component: () => import('./views/NotesView.vue'),
    meta: { permission: 'wl_example_notes' }
  }
]
