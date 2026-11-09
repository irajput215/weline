import { CalendarCheck, CalendarClock, Globe } from 'lucide-vue-next'
import type { NavSection } from '@/components/layout/navigation'

export const nav: NavSection[] = [
  {
    label: 'wl.booking.section',
    permissions: ['wl_booking_appointments', 'wl_booking_availability'],
    items: [
      { name: 'wl.booking.nav.appointments', path: '/x/booking/appointments', icon: CalendarCheck, permission: 'wl_booking_appointments' },
      { name: 'wl.booking.nav.availability', path: '/x/booking/availability', icon: CalendarClock, permission: 'wl_booking_availability' },
      { name: 'wl.booking.nav.online', path: '/x/booking/online', icon: Globe, permission: 'wl_booking_availability' }
    ]
  }
]
