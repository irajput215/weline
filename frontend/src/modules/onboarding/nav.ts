import { Rocket } from 'lucide-vue-next'
import type { NavSection } from '@/components/layout/navigation'

export const nav: NavSection[] = [
  {
    label: 'wl.onboarding.section',
    permissions: ['wl_onboarding_setup'],
    items: [{ name: 'wl.onboarding.nav.start', path: '/x/onboarding', icon: Rocket, permission: 'wl_onboarding_setup' }]
  }
]
