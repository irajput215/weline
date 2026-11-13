import type { RouteRecordRaw } from 'vue-router'

export const routes: RouteRecordRaw[] = [
  { path: 'x/onboarding', name: 'wl-onboarding', component: () => import('./views/OnboardingView.vue'), meta: { permission: 'wl_onboarding_setup' } }
]
