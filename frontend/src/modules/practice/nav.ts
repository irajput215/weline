import { Building2, CalendarClock, ClipboardList, MapPin, Stethoscope } from 'lucide-vue-next'
import type { NavSection } from '@/components/layout/navigation'

export const nav: NavSection[] = [
  {
    label: 'wl.practice.section',
    permissions: ['wl_practice_settings', 'wl_practice_locations', 'wl_practice_practitioners', 'wl_practice_services'],
    items: [
      { name: 'wl.practice.nav.profile', path: '/x/practice/profile', icon: Building2, permission: 'wl_practice_settings' },
      { name: 'wl.practice.nav.locations', path: '/x/practice/locations', icon: MapPin, permission: 'wl_practice_locations' },
      { name: 'wl.practice.nav.practitioners', path: '/x/practice/practitioners', icon: Stethoscope, permission: 'wl_practice_practitioners' },
      { name: 'wl.practice.nav.services', path: '/x/practice/services', icon: ClipboardList, permission: 'wl_practice_services' },
      { name: 'wl.practice.nav.hours', path: '/x/practice/hours', icon: CalendarClock, permission: 'wl_practice_settings' }
    ]
  }
]
