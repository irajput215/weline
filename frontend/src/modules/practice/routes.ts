import type { RouteRecordRaw } from 'vue-router'

export const routes: RouteRecordRaw[] = [
  {
    path: 'x/practice/profile',
    name: 'wl-practice-profile',
    component: () => import('./views/ProfileView.vue'),
    meta: { permission: 'wl_practice_settings' }
  },
  {
    path: 'x/practice/locations',
    name: 'wl-practice-locations',
    component: () => import('./views/LocationsView.vue'),
    meta: { permission: 'wl_practice_locations' }
  },
  {
    path: 'x/practice/practitioners',
    name: 'wl-practice-practitioners',
    component: () => import('./views/PractitionersView.vue'),
    meta: { permission: 'wl_practice_practitioners' }
  },
  {
    path: 'x/practice/services',
    name: 'wl-practice-services',
    component: () => import('./views/ServicesView.vue'),
    meta: { permission: 'wl_practice_services' }
  },
  {
    path: 'x/practice/hours',
    name: 'wl-practice-hours',
    component: () => import('./views/HoursView.vue'),
    meta: { permission: 'wl_practice_settings' }
  }
]
