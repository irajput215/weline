import { PlugZap } from 'lucide-vue-next'
import type { NavSection } from '@/components/layout/navigation'

export const nav: NavSection[] = [
  {
    label: 'wl.connectors.section',
    permissions: ['wl_connectors_settings'],
    items: [{ name: 'wl.connectors.nav.system', path: '/x/connectors', icon: PlugZap, permission: 'wl_connectors_settings' }]
  }
]
