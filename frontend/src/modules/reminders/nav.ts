import { BellRing } from 'lucide-vue-next'
import type { NavSection } from '@/components/layout/navigation'

export const nav: NavSection[] = [
  {
    label: 'wl.reminders.section',
    permissions: ['wl_reminders_rules', 'wl_reminders_recalls'],
    items: [{ name: 'wl.reminders.nav.reminders', path: '/x/reminders', icon: BellRing, permission: 'wl_reminders_rules' }]
  }
]
