import { History, Repeat, UsersRound } from 'lucide-vue-next'
import type { NavSection } from '@/components/layout/navigation'

export const nav: NavSection[] = [
  {
    label: 'wl.audiences.section',
    permissions: ['wl_audiences_segments'],
    items: [
      { name: 'wl.audiences.nav.segments', path: '/x/audiences', icon: UsersRound, permission: 'wl_audiences_segments' },
      { name: 'wl.audiences.nav.retarget', path: '/x/audiences/retarget', icon: Repeat, permission: 'wl_audiences_segments' },
      { name: 'wl.audiences.nav.history', path: '/x/audiences/history', icon: History, permission: 'wl_audiences_segments' }
    ]
  }
]
