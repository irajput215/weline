import type { RouteRecordRaw } from 'vue-router'

export const routes: RouteRecordRaw[] = [
  {
    path: 'x/booking/appointments',
    name: 'wl-booking-appointments',
    component: () => import('./views/AppointmentsView.vue'),
    meta: { permission: 'wl_booking_appointments' }
  },
  {
    path: 'x/booking/availability',
    name: 'wl-booking-availability',
    component: () => import('./views/AvailabilityView.vue'),
    meta: { permission: 'wl_booking_availability' }
  },
  {
    path: 'x/booking/online',
    name: 'wl-booking-online',
    component: () => import('./views/OnlineView.vue'),
    meta: { permission: 'wl_booking_availability' }
  }
]
