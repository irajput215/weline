import { History } from 'lucide-vue-next'
import type { NavSection } from '@/components/layout/navigation'

export const nav: NavSection[] = [
  {
    label: 'wl.audit.section',
    permissions: ['wl_audit_events'],
    items: [{ name: 'wl.audit.nav.events', path: '/x/audit', icon: History, permission: 'wl_audit_events' }]
  }
]
