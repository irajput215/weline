import { Megaphone } from 'lucide-vue-next'
import type { NavSection } from '@/components/layout/navigation'

export const nav: NavSection[] = [
  {
    label: 'wl.ads.section',
    permissions: ['wl_ads_entrypoints'],
    items: [{ name: 'wl.ads.nav.entrypoints', path: '/x/ads', icon: Megaphone, permission: 'wl_ads_entrypoints' }]
  }
]
