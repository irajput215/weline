import type { RouteRecordRaw } from 'vue-router'

export const routes: RouteRecordRaw[] = [
  {
    path: 'x/consent',
    name: 'wl-consent',
    component: () => import('./views/ConsentView.vue'),
    meta: { permission: 'wl_consent_records' }
  },
  {
    path: 'x/consent/settings',
    name: 'wl-consent-settings',
    component: () => import('./views/SettingsView.vue'),
    meta: { permission: 'wl_consent_settings' }
  }
]
