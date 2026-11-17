import { History } from 'lucide-vue-next'
import type { NavSection } from '@/components/layout/navigation'

export const nav: NavSection[] = [
  {
    label: 'wl.timeline.section',
    permissions: ['wl_timeline_contacts'],
    items: [{ name: 'wl.timeline.nav.timeline', path: '/x/timeline', icon: History, permission: 'wl_timeline_contacts' }]
  }
]
