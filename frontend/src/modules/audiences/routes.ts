import type { RouteRecordRaw } from 'vue-router'

export const routes: RouteRecordRaw[] = [
  {
    path: 'x/audiences',
    name: 'wl-audiences-segments',
    component: () => import('./views/SegmentsView.vue'),
    meta: { permission: 'wl_audiences_segments' }
  },
  {
    path: 'x/audiences/segments/new',
    name: 'wl-audiences-segment-new',
    component: () => import('./views/SegmentBuilderView.vue'),
    meta: { permission: 'wl_audiences_segments' }
  },
  {
    path: 'x/audiences/segments/:id',
    name: 'wl-audiences-segment',
    component: () => import('./views/SegmentBuilderView.vue'),
    meta: { permission: 'wl_audiences_segments' }
  },
  {
    path: 'x/audiences/retarget',
    name: 'wl-audiences-retarget',
    component: () => import('./views/RetargetView.vue'),
    meta: { permission: 'wl_audiences_segments' }
  },
  {
    path: 'x/audiences/history',
    name: 'wl-audiences-history',
    component: () => import('./views/HistoryView.vue'),
    meta: { permission: 'wl_audiences_segments' }
  }
]
