import { ShieldCheck, SlidersHorizontal } from 'lucide-vue-next'
import type { NavSection } from '@/components/layout/navigation'

export const nav: NavSection[] = [
  {
    label: 'wl.consent.section',
    permissions: ['wl_consent_records', 'wl_consent_settings'],
    items: [
      { name: 'wl.consent.nav.records', path: '/x/consent', icon: ShieldCheck, permission: 'wl_consent_records' },
      { name: 'wl.consent.nav.settings', path: '/x/consent/settings', icon: SlidersHorizontal, permission: 'wl_consent_settings' }
    ]
  }
]
