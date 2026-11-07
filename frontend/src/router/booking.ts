// Weline-only routes. Spread into the authenticated layout's children in
// router/index.ts with a single line, so upstream router changes merge cleanly.
import type { RouteRecordRaw } from 'vue-router'

export const bookingRoutes: RouteRecordRaw[] = [
  {
    path: 'booking',
    name: 'booking',
    component: () => import('@/views/booking/BookingView.vue')
    // TODO: add `meta: { permission: 'booking' }` once a booking permission exists.
  }
]
